package finder

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler/search"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

const (
	readerBufferSize    = 64 * 1024
	defaultStoreTimeout = 5 * time.Second
	defaultMaxHistory   = 20
)

func Permissions() []plugin.Permission {
	return []plugin.Permission{
		plugin.PermissionBrowserResourceOpener,
		plugin.PermissionBrowserEventPublisher,
		plugin.PermissionBrowserMessenger,
		plugin.PermissionBrowserStorage,
		plugin.PermissionEditor,
		plugin.PermissionWorkspace,
	}
}

type fuzzyFinderHandler struct {
	s            browser.Storage
	f            browser.ResourceOpener
	p            browser.EventPublisher
	m            browser.Messenger
	ed           text.Editor
	executor     workspace.Workspace
	invokeWindow browser.Window
	historyKey   term.KeyComb
	mu           sync.Mutex
	cmdStr       string
	getResource  func(workspace.Workspace, string) (workspace.URI, term.Coordinates)
	pid          workspace.Pid
	quitChan     chan struct{}
	height       int
	list         search.List
	background   tui.Component
	listHandler  tui.Handler
	killed       bool

	history search.History
}

func (h *fuzzyFinderHandler) execCommand(command string) (workspace.Pid, error) {
	shell := os.Getenv("SHELL")
	if len(shell) == 0 {
		shell = "sh"
	}
	return h.execCommandWith(shell, command)
}

// ExecCommandWith executes the given command with the specified shell
func (h *fuzzyFinderHandler) execCommandWith(shell string, command string) (workspace.Pid, error) {
	cmd, err := h.executor.Command(shell, "-c", command)
	if err != nil {
		return 0, fmt.Errorf("failed to create command: %w", err)
	}
	return cmd, nil
}

// KillCommand kills the process for the given command
func (h *fuzzyFinderHandler) killCommand() error {
	return h.executor.Signal(h.pid, syscall.SIGKILL)
}

func (h *fuzzyFinderHandler) readCommand(src io.Reader) {
	h.mu.Lock()
	datachan := h.list.Push()
	h.mu.Unlock()

	defer close(datachan)

	reader := bufio.NewReaderSize(src, readerBufferSize)
	for {
		data, err := reader.ReadBytes('\n')
		if len(data) > 0 {
			select {
			case datachan <- data[:len(data)-1]:
			case <-h.quitChan:
				return
			}
		}
		if err != nil {
			break
		}
	}
}

func (h *fuzzyFinderHandler) addSearchHistory(searchQuery string) {
	if h.s == nil {
		log.Debug("Storage permission not granted; ignoring history feature")
		return
	}

	if searchQuery == "" {
		log.Trace("skipping persisting of empty query")
		return
	}

	h.mu.Unlock()
	defer h.mu.Lock()

	err := h.history.Add(searchQuery)
	if err != nil {
		log.Errorf("error adding search history: %v", err)
	} else {
		log.Debugf("added %q to query history: %v", searchQuery, h.history)
	}
}

func (h *fuzzyFinderHandler) open(resource workspace.URI) (browser.Handler, error) {
	h.mu.Unlock()
	defer h.mu.Lock()
	return h.f.Open(resource)
}

func (h *fuzzyFinderHandler) setContent(
	resource workspace.URI, b browser.Handler, pos term.Coordinates,
) error {
	h.mu.Unlock()
	defer h.mu.Lock()
	err := h.invokeWindow.SetContent(b)
	if err != nil && err != browser.ErrTabNotFree {
		return err
	}
	if h.ed == nil {
		log.Info("could not set cursor position because host did not grant plugin.PermissionEditor")
		return nil
	}

	hed, err := h.ed.Editor(resource)
	if err != nil {
		return err
	}

	return h.ed.SetCursor(hed, pos)
}

func (h *fuzzyFinderHandler) setMessage(msg string, args ...interface{}) error {
	// allow browser messenger permission to be denied
	if h.m == nil {
		return nil
	}

	h.mu.Unlock()
	defer h.mu.Lock()

	return h.m.SetMessage(msg, args...)
}

func (h *fuzzyFinderHandler) openResource(searchQuery, data string) {
	resource, pos := h.getResource(h.executor, data)
	handler, err := h.open(resource)
	if err != nil {
		merr := h.setMessage("Open: %v", err)
		if merr != nil {
			log.Errorf("error setting message: %v", merr)
		}
		log.Errorf("error opening new resource: %v", err)
		return
	}

	err = h.setContent(resource, handler, pos)
	if err != nil {
		log.Errorf("error SetContent: %v", err)
		return
	}
}

func (h *fuzzyFinderHandler) publishInterrupt() {
	err := h.p.PublishInterrupt()
	if err != nil {
		log.Printf("failed to publish interrupt: %v", err)
	}
}

func (h *fuzzyFinderHandler) scanData() {
	exec, err := h.execCommand(h.cmdStr)
	h.mu.Lock()
	h.pid = exec
	h.mu.Unlock()
	if err != nil {
		log.Error(err)
		return
	}

	out, err := h.executor.StdoutPipe(h.pid)
	if err != nil {
		log.Errorf("command stdout failed; %v", err)
		return
	}
	err = h.executor.Start(h.pid)
	if err != nil {
		log.Errorf("command start failed; %v", err)
		return
	}

	h.readCommand(out)

	err = h.executor.Wait(h.pid)

	h.mu.Lock()
	defer h.mu.Unlock()

	killed := h.killed
	h.pid = 0

	if !killed && err != nil {
		merr := h.setMessage("failed to execute '%s': %v", h.cmdStr, err)
		if merr != nil {
			log.Errorf("error setting message: %v", merr)
		}
	}

}

func (h *fuzzyFinderHandler) initGrants(
	broker proto.MuxBroker, grants []plugin.Grant,
	historyDocumentID string, maxHistory int,
) (err error) {
	for _, grant := range grants {
		switch grant.Permission {
		case plugin.PermissionWorkspace:
			h.executor, err = plugin.Workspace(grant.Token, broker)
		case plugin.PermissionEditor:
			h.ed, err = plugin.Editor(grant.Token, broker)
		case plugin.PermissionBrowserMessenger:
			h.m, err = plugin.Messenger(grant.Token, broker)
		case plugin.PermissionBrowserEventPublisher:
			h.p, err = plugin.EventPublisher(grant.Token, broker)
		case plugin.PermissionBrowserResourceOpener:
			h.f, err = plugin.ResourceOpener(grant.Token, broker)
		case plugin.PermissionBrowserStorage:
			h.s, err = plugin.Storage(grant.Token, broker)
			if err == nil {
				h.history.Init(h.s, historyDocumentID, maxHistory)
				err = h.history.Load()
			}
		}
		if err != nil {
			return
		}
	}
	if h.f == nil || h.p == nil {
		log.Fatalf("This plugin cannot function with granted permissions. Exiting now.")
	}
	return
}

// New returns a tui.Handler that employs a search.List
// to interactively search the command's stdout lines.
func New(
	grants []plugin.Grant, broker proto.MuxBroker,
	invokeWindow browser.Window, config plugin.Config,
	historyKey term.KeyComb, historyDocumentID string, command string,
	getResource func(exec workspace.Workspace, line string) (workspace.URI, term.Coordinates),
) (tui.Handler, error) {
	h := new(fuzzyFinderHandler)
	maxHistory, err := config.GetInt("history")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Errorf("failed to load 'history' from config: %v", err)
		}
		maxHistory = defaultMaxHistory
	} else {
		log.Tracef("loaded 'history' from config: %v", maxHistory)
	}
	err = h.initGrants(broker, grants, historyDocumentID, maxHistory)
	if err != nil {
		return nil, err
	}

	h.invokeWindow = invokeWindow
	h.historyKey = historyKey
	h.getResource = getResource
	h.cmdStr = command
	log.Printf("using resource list command: %s", h.cmdStr)

	h.quitChan = make(chan struct{})

	listConfig := h.getListConfig(config)
	h.list.Init(listConfig)
	h.listHandler = search.Handler(&h.list, func(item string) {
		searchQuery := h.list.Buffer().String()
		h.openResource(searchQuery, item)
		h.addSearchHistory(searchQuery)
	})

	var defCell term.Cell
	if listConfig.ElementAttr != nil {
		defCell.Bg = listConfig.ElementAttr.Bg
		defCell.Fg = listConfig.ElementAttr.Fg
	}
	h.background = component.WithBackground(h.listHandler, defCell)

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

	matchedTextAttr, err := plugin.GetAttributes(config, "matched_text_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'matched_text_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'matchied_text_attr' from config: %v", matchedTextAttr)
		cfg.MatchedTextAttr = &matchedTextAttr
	}

	countAttr, err := plugin.GetAttributes(config, "count_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'count_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'count_attr' from config: %v", countAttr)
		cfg.CountAttr = &countAttr
	}

	textAttr, err := plugin.GetAttributes(config, "element_attr")
	if err != nil && err != plugin.ErrNotFound {
		log.Errorf("failed to load 'element_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'element_attr' from config: %v", textAttr)
		cfg.ElementAttr = &textAttr
	}

	focusAttr, err := plugin.GetAttributes(config, "focus_element_attr")
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
	h.background.Resize(width, height)
}

func (h *fuzzyFinderHandler) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.background.Draw(w)
}

func (h *fuzzyFinderHandler) writeLastSearchQuery() {
	search := h.history.Next()
	if search == "" {
		log.Debugf("no search queries stored")
		return
	}
	h.list.Buffer().Reset()
	h.list.Buffer().WriteString(search)
}

func (h *fuzzyFinderHandler) Handle(ev term.Event) (exit, handled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Type != term.EventKey {
		return
	}

	if ev.KeyComb() == h.historyKey {
		h.writeLastSearchQuery()
		handled = true
		return
	}

	exit, handled = h.listHandler.Handle(ev)

	log.Tracef("fuzzyFinderHandler.Handle(%#v): %v", ev, handled)

	return
}

func (h *fuzzyFinderHandler) Cursor() (pos term.Coordinates, show bool) {
	return h.listHandler.Cursor()
}

func (h *fuzzyFinderHandler) Man() tui.Manual {
	return h.listHandler.Man()
}

func (h *fuzzyFinderHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Tracef("fuzzyFinderHandler.Close(): %#v", h.pid)

	if h.pid != 0 {
		h.killed = true
		_ = h.killCommand()
	}
	close(h.quitChan)
	h.list.Close()
	return nil
}
