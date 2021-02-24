package finder

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component/search"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

const (
	readerBufferSize        = 64 * 1024
	defaultStoreTimeout     = 5 * time.Second
	searchHistoryDocumentID = "search-history"
	defaultMaxHistory       = 20
)

type fuzzyFinderHandler struct {
	s            browser.Storage
	f            browser.ResourceOpener
	p            browser.EventPublisher
	m            browser.Messenger
	invokeWindow browser.Window
	invokeKey    term.Event
	mu           sync.Mutex
	cmdStr       string
	getResource  func(string) string
	exec         *exec.Cmd
	quitChan     chan struct{}
	height       int
	list         search.List
	killed       bool

	history struct {
		max     int
		Queries []string
	}
}

func execCommand(command string, setpgid bool) *exec.Cmd {
	shell := os.Getenv("SHELL")
	if len(shell) == 0 {
		shell = "sh"
	}
	return execCommandWith(shell, command, setpgid)
}

// ExecCommandWith executes the given command with the specified shell
func execCommandWith(shell string, command string, setpgid bool) *exec.Cmd {
	cmd := exec.Command(shell, "-c", command)
	if setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}

// KillCommand kills the process for the given command
func (h *fuzzyFinderHandler) killCommand() error {
	return syscall.Kill(-h.exec.Process.Pid, syscall.SIGKILL)
}

func (h *fuzzyFinderHandler) readCommand(src io.Reader) {
	h.mu.Lock()
	datachan := h.list.Push()
	h.mu.Unlock()

	defer close(datachan)

	reader := bufio.NewReaderSize(src, readerBufferSize)
	for {
		data, err := reader.ReadBytes(byte('\n'))
		if len(data) > 0 {
			select {
			case datachan <- data:
			case <-h.quitChan:
				return
			}
		}
		if err != nil {
			break
		}
	}
}

func (h *fuzzyFinderHandler) getSearchHistory() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
	defer cancel()

	err := h.s.Get(ctx, searchHistoryDocumentID, &h.history)
	if err == document.ErrNotFound {
		err = nil
	}
	if err == nil {
		log.Debugf("retrieved query history; %#v", h.history.Queries)
	}

	return err
}

func (h *fuzzyFinderHandler) addSearchHistory(searchQuery string) error {
	h.history.Queries = append(h.history.Queries, searchQuery)
	if len(h.history.Queries) > h.history.max {
		h.history.Queries = h.history.Queries[1:]
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultStoreTimeout)
	defer cancel()

	h.mu.Unlock()
	defer h.mu.Lock()

	return h.s.Set(ctx, searchHistoryDocumentID, &h.history)
}

func (h *fuzzyFinderHandler) open(resource string) (browser.Handler, error) {
	h.mu.Unlock()
	defer h.mu.Lock()
	return h.f.Open(resource)
}

func (h *fuzzyFinderHandler) setContent(b browser.Handler) error {
	h.mu.Unlock()
	defer h.mu.Lock()
	return h.invokeWindow.SetContent(b)
}

func (h *fuzzyFinderHandler) setMessage(msg string, args ...interface{}) error {
	h.mu.Unlock()
	defer h.mu.Lock()
	return h.m.SetMessage(msg, args...)
}

func (h *fuzzyFinderHandler) openResource(searchQuery, data string) {
	resource := h.getResource(data)
	buf, err := h.open(resource)
	if err != nil {
		merr := h.setMessage("Open: %v", err)
		if merr != nil {
			log.Errorf("error setting message: %v", merr)
		}
		log.Errorf("error opening new resource: %v", err)
		return
	}

	err = h.setContent(buf)
	if err != nil {
		log.Errorf("error SetContent: %v", err)
		return
	}

	// optinally store query for history browsing
	if h.s != nil {
		err := h.addSearchHistory(searchQuery)
		if err != nil {
			log.Errorf("error adding search history: %v", err)
		} else {
			log.Debugf("added %s to query history", searchQuery)
		}
	}
}

func (h *fuzzyFinderHandler) publishInterrupt() {
	err := h.p.PublishInterrupt()
	if err != nil {
		log.Printf("failed to publish interrupt: %v", err)
	}
}

func (h *fuzzyFinderHandler) scanData() {
	h.mu.Lock()
	h.exec = execCommand(h.cmdStr, true)
	exec := h.exec
	h.mu.Unlock()

	out, err := exec.StdoutPipe()
	if err != nil {
		log.Errorf("command stdout failed; %v", err)
		return
	}
	err = exec.Start()
	if err != nil {
		log.Errorf("command start failed; %v", err)
		return
	}

	go h.readCommand(out)

	err = exec.Wait()
	h.mu.Lock()
	killed := h.killed
	h.exec = nil
	h.mu.Unlock()

	if !killed && err != nil {
		merr := h.m.SetMessage("failed to execute '%s': %v", h.cmdStr, err)
		if merr != nil {
			log.Errorf("error setting message: %v", merr)
		}
	}

}

func (h *fuzzyFinderHandler) initGrants(
	broker proto.MuxBroker, grants []plugin.Grant,
) (err error) {
	for _, grant := range grants {
		switch grant.Permission {
		case plugin.PermissionBrowserMessenger:
			h.m, err = plugin.Messenger(grant.Token, broker)
		case plugin.PermissionBrowserEventPublisher:
			h.p, err = plugin.EventPublisher(grant.Token, broker)
		case plugin.PermissionBrowserResourceOpener:
			h.f, err = plugin.ResourceOpener(grant.Token, broker)
		case plugin.PermissionBrowserStorage:
			h.s, err = plugin.Storage(grant.Token, broker)
			if err == nil {
				err = h.getSearchHistory()
			}
		}
		if err != nil {
			return
		}
	}
	return
}

// New returns a tui.Handler that employs a search.List
// to interactively search the command's stdout lines.
func New(
	grants []plugin.Grant, broker proto.MuxBroker,
	invokeWindow browser.Window, config plugin.Config,
	invokeKey term.Event, command string,
	getResource func(line string) string,
) (tui.Handler, error) {
	h := new(fuzzyFinderHandler)
	err := h.initGrants(broker, grants)
	if err != nil {
		return nil, err
	}

	h.invokeWindow = invokeWindow
	h.invokeKey = invokeKey
	h.getResource = getResource
	h.cmdStr = command
	log.Printf("using resource list command: %s", h.cmdStr)

	h.quitChan = make(chan struct{})

	listConfig := h.getListConfig(config)
	h.list.Init(listConfig)

	h.history.max, err = config.GetInt("history")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Errorf("failed to load 'history' from config: %v", err)
		}
		h.history.max = defaultMaxHistory
	} else {
		log.Tracef("loaded 'history' from config: %v", h.history.max)
	}

	go h.scanData()

	return h, nil
}

func (h *fuzzyFinderHandler) getListConfig(config plugin.Config) search.ListConfig {
	caseSensitive, err := config.GetBool("case_sensitive")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Errorf("failed to load 'case_sensitive' from config: %v", err)
		}
		caseSensitive = true
	}
	algoStr, err := config.GetString("algo")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'algo' from config: %v", err)
	}
	algo := search.FuzzyMatch
	if algoStr == "equal" {
		algo = search.EqualMatch
	}

	cfg := search.ListConfig{
		Algo:          algo,
		Interrupt:     h.publishInterrupt,
		CaseSensitive: caseSensitive,
	}

	searchBaseAttr, err := config.GetAttributes("search_base_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'search_base_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'search_base_attr' from config: %v", searchBaseAttr)
		cfg.SearchBaseAttr = &searchBaseAttr
	}

	matchedTextAttr, err := config.GetAttributes("match_text_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'match_base_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'match_base_attr' from config: %v", matchedTextAttr)
		cfg.MatchedTextAttr = &matchedTextAttr
	}

	countAttr, err := config.GetAttributes("count_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'count_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'count_attr' from config: %v", countAttr)
		cfg.CountAttr = &countAttr
	}

	textAttr, err := config.GetAttributes("element_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'element_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'element_attr' from config: %v", textAttr)
		cfg.ElementAttr = &textAttr
	}

	focusAttr, err := config.GetAttributes("focus_element_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'focus_element_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'focus_element_attr' from config: %v", focusAttr)
		cfg.FocusElementAttr = &focusAttr
	}

	return cfg
}

func (h *fuzzyFinderHandler) Resize(width, height int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.height = height
	h.list.Resize(width, height)
}

func (h *fuzzyFinderHandler) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.list.Draw(w)
}

func (h *fuzzyFinderHandler) writeLastSearchQuery() {
	if len(h.history.Queries) == 0 {
		log.Debugf("no search queries stored")
		return
	}
	search := h.history.Queries[0]
	h.history.Queries = h.history.Queries[1:]
	h.list.Search(search)
}

func (h *fuzzyFinderHandler) Handle(ev term.Event) (exit, handled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Type != term.EventKey {
		return
	}

	if ev == h.invokeKey {
		h.writeLastSearchQuery()
		handled = true
		return
	}

	switch ev.Key {
	case term.KeyEnter:
		item, ok := h.list.Focus()
		if ok {
			handled = true
			exit = true
			h.openResource(h.list.SearchQueryString(), string(item))
		}
	case term.KeyEsc:
		exit = true
	case term.KeyArrowDown:
		handled = h.list.FocusDown()
	case term.KeyArrowUp:
		handled = h.list.FocusUp()
	case term.KeySpace:
		ev.Ch = ' '
	case term.KeyBackspace:
		fallthrough
	case term.KeyBackspace2:
		handled = h.list.SearchQueryDelete()
	}

	if ev.Ch != 0 {
		h.list.SearchQueryWrite(ev.Ch)
		handled = true
	}

	return
}

func (h *fuzzyFinderHandler) Cursor() (pos term.Coordinates, show bool) {
	return term.Coordinates{X: h.list.SearchQueryLen()}, true
}

func (h *fuzzyFinderHandler) Man() tui.Manual {
	return tui.Manual{}
}

func (h *fuzzyFinderHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.exec != nil {
		h.killed = true
		_ = h.killCommand()
	}
	close(h.quitChan)
	h.list.Close()
	return nil
}
