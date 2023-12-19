package finder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/blue/document"
	"github.com/ernestrc/blue/iterator"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	browserextension "unstable.build/go-tui/api/browser/extension"
	"unstable.build/go-tui/api/config"
	storageextension "unstable.build/go-tui/api/storage/extension"
	textapi "unstable.build/go-tui/api/text"
	textextension "unstable.build/go-tui/api/text/extension"
	workspaceapi "unstable.build/go-tui/api/workspace"
	workspaceextension "unstable.build/go-tui/api/workspace/extension"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler/search"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/term"
)

const (
	readerBufferSize    = 64 * 1024
	defaultStoreTimeout = 5 * time.Second
	defaultMaxHistory   = 2000
)

func Permissions() []extension.Permission {
	return []extension.Permission{
		extension.Permission(extension.PermissionBrowserResourceOpener),
		extension.Permission(extension.PermissionBrowserEventPublisher),
		extension.Permission(extension.PermissionBrowserNotifications),
		extension.PermissionStorage,
		extension.Permission(extension.PermissionEditor),
		extension.Permission(extension.PermissionFileSystem),
		extension.Permission(extension.PermissionExecute),
	}
}

type fuzzyFinderHandler struct {
	s                    document.Service
	f                    browserapi.ResourceOpener
	p                    browserapi.EventPublisher
	m                    browserapi.Notifications
	ed                   textapi.Editor
	fs                   workspaceapi.FileSystem
	executor             workspaceapi.Executor
	invokeWindow         browserapi.Window
	historyKey           term.KeyComb
	mu                   sync.Mutex
	cmdStr               string
	getResource          func(workspaceapi.FileSystem, string) (workspaceapi.URI, term.Coordinates)
	workspaceFallback    func(workspaceapi.FileSystem, context.Context) (iterator.Iterator[string], error)
	pid                  workspaceapi.Pid
	ctx                  context.Context
	cancelCtx            func()
	waitChan             chan error
	height               int
	list                 search.List
	background           tui.Component
	listHandler          tui.Handler
	killed               bool
	useWorkspaceFallback bool
	cancelScan           func()

	history search.History
}

func (h *fuzzyFinderHandler) execCommand(command string) (
	*os.File, *os.File, workspaceapi.Pid, error,
) {
	shell := os.Getenv("SHELL")
	if len(shell) == 0 {
		shell = "sh"
	}
	return h.execCommandWith(shell, command)
}

// Watch satisfies workspaceapi.Watcher which is employed
// to wait for the underlying command to execute.
func (h *fuzzyFinderHandler) Watch() chan error {
	return h.waitChan
}

func (h *fuzzyFinderHandler) execCommandWith(shell string, commandStr string) (
	*os.File, *os.File, workspaceapi.Pid, error,
) {
	cmd := workspaceapi.Cmd{
		Path:    shell,
		Args:    []string{"-c", commandStr},
		Watcher: h,
	}
	stderr, stdout, err := h.setPipes(&cmd)
	if err != nil {
		return nil, nil, 0, err
	}
	pid, err := h.executor.Start(cmd)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to create command: %w", err)
	}
	return stderr, stdout, pid, nil
}

func (h *fuzzyFinderHandler) setPipes(
	cmd *workspaceapi.Cmd,
) (stderr, stdout *os.File, err error) {
	stdout, stdoutWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, stderrWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdout = stdoutWrite
	cmd.Stderr = stderrWrite
	return stderr, stdout, nil
}

// KillCommand kills the process for the given command
func (h *fuzzyFinderHandler) killCommand() error {
	h.mu.Unlock()
	defer h.mu.Lock()

	return h.executor.Signal(h.pid, syscall.SIGKILL)
}

func (h *fuzzyFinderHandler) readCommand(ctx context.Context, datachan chan<- []byte, src io.Reader, cancelScan func()) {
	defer cancelScan()
	defer close(datachan)
	reader := bufio.NewReaderSize(src, readerBufferSize)
	for {
		data, err := reader.ReadBytes('\n')
		if len(data) > 0 {
			select {
			case datachan <- data[:len(data)-1]:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Error(err)
			}
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

func (h *fuzzyFinderHandler) open(resource workspaceapi.URI) (browserapi.Handler, error) {
	h.mu.Unlock()
	defer h.mu.Lock()
	return h.f.Open(resource)
}

func (h *fuzzyFinderHandler) setContent(
	resource workspaceapi.URI, b browserapi.Handler, pos term.Coordinates,
) error {
	h.mu.Unlock()
	defer h.mu.Lock()
	err := h.invokeWindow.SetContent(b)
	if err != nil && !errors.Is(err, browserapi.ErrTabNotFree) {
		return err
	}
	if h.ed == nil {
		log.Info("could not set cursor position because host did not grant extension.PermissionEditor")
		return nil
	}

	hed, err := h.ed.Editor(resource)
	if err != nil {
		return err
	}

	return h.ed.SetCursor(hed, pos)
}

func (h *fuzzyFinderHandler) notifyError(msg string, args ...interface{}) error {
	// allow browser messenger permission to be denied
	if h.m == nil {
		return nil
	}

	h.mu.Unlock()
	defer h.mu.Lock()

	return h.m.Notify(notifications.LevelError, msg, args...)
}

func (h *fuzzyFinderHandler) openResource(searchQuery, data string) {
	resource, pos := h.getResource(h.fs, data)
	handler, err := h.open(resource)
	if err != nil {
		merr := h.notifyError("Open: %v", err)
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

func (h *fuzzyFinderHandler) doScanDataViaWorkspaceAPI(
	ctx context.Context, datachan chan<- []byte,
) error {
	defer close(datachan)
	log.Debugf("using workspace API to get resource iterator")

	it, err := h.workspaceFallback(h.fs, ctx)
	if err != nil {
		return fmt.Errorf("workspace API fallback: %v", err)
	}

	for {
		resource, ok := it.Next()
		if !ok {
			break
		}
		select {
		case datachan <- []byte(resource):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := it.Err(); err != nil {
		return err
	}
	return nil
}

func (h *fuzzyFinderHandler) scanDataViaWorkspaceAPI(
	ctx context.Context, datachan chan<- []byte,
) {
	err := h.doScanDataViaWorkspaceAPI(ctx, datachan)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("scan data: %v", err)

		// setMessage unlocks locker to prevent deadlock by I/O wait
		h.mu.Lock()
		defer h.mu.Unlock()

		merr := h.notifyError("failed to scan: %v", err)
		if merr != nil {
			log.Errorf("error setting message: %v", merr)
		}
	}
}

func (h *fuzzyFinderHandler) scanData() {
	start := time.Now()
	defer func() {
		if log.IsLevelEnabled(log.DebugLevel) {
			log.Debugf("Done iterating over data in %s", time.Since(start))
		}
	}()

	ctx := h.ctx
	ctx, cancelScan := context.WithCancel(ctx)

	h.mu.Lock()
	h.cancelScan = cancelScan
	datachan := h.list.Push(ctx)
	h.mu.Unlock()

	if h.useWorkspaceFallback {
		h.scanDataViaWorkspaceAPI(ctx, datachan)
		return
	}

	log.Debugf("using resource list command: %s", h.cmdStr)

	stderr, stdout, exec, err := h.execCommand(h.cmdStr)
	if err != nil {
		log.Debugf("fallback to scan data via workspace API: %v", err)
		h.scanDataViaWorkspaceAPI(ctx, datachan)
		return
	}

	h.mu.Lock()
	h.pid = exec
	h.mu.Unlock()

	go h.readCommand(ctx, datachan, stdout, cancelScan)
	go func() {
		data, err := ioutil.ReadAll(stderr)
		if err != nil {
			log.Errorf("failed to read from stderr: %v", err)
		}
		log.Debugf("stderr: %s", string(data))
	}()

	log.Debugf("waiting for command to be done")
	err = <-h.waitChan
	log.Debugf("command is done: %v", err)

	h.mu.Lock()
	defer h.mu.Unlock()

	killed := h.killed
	h.pid = 0

	if !killed && err != nil {
		merr := h.notifyError("failed to execute '%s': %v", h.cmdStr, err)
		if merr != nil {
			log.Errorf("error setting message: %v", merr)
		}
	}

}

func (h *fuzzyFinderHandler) initGrants(
	broker proto.MuxBroker, grants []extension.Grant,
	historyDocumentID string, maxHistory int,
) (err error) {
	for _, grant := range grants {
		switch grant.Permission {
		case extension.Permission(extension.PermissionFileSystem):
			h.fs, err = workspaceextension.FileSystem(grant, broker)
		case extension.Permission(extension.PermissionExecute):
			h.executor, err = workspaceextension.Executor(grant, broker)
		case extension.Permission(extension.PermissionEditor):
			h.ed, err = textextension.Editor(grant, broker)
		case extension.Permission(extension.PermissionBrowserNotifications):
			h.m, err = browserextension.Notifications(grant, broker)
		case extension.Permission(extension.PermissionBrowserEventPublisher):
			h.p, err = browserextension.EventPublisher(grant, broker)
		case extension.Permission(extension.PermissionBrowserResourceOpener):
			h.f, err = browserextension.ResourceOpener(grant, broker)
		case extension.PermissionStorage:
			h.s, err = storageextension.Storage(grant, broker)
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
		return errors.New("extension is missing critical permissions")
	}
	return
}

// New returns a tui.Handler that employs a search.List
// to interactively search the command's stdout lines.
//
// The given context is passed back to the fallback
// function along with any values stored with it.
func New(
	ctx context.Context, grants []extension.Grant, broker proto.MuxBroker,
	invokeWindow browserapi.Window, cfg config.Config,
	historyKey term.KeyComb, historyDocumentID string, command string,
	fallback func(workspaceapi.FileSystem, context.Context) (iterator.Iterator[string], error),
	getResource func(exec workspaceapi.FileSystem, line string) (workspaceapi.URI, term.Coordinates),
) (browserapi.Handler, error) {
	h := new(fuzzyFinderHandler)
	maxHistory, err := cfg.GetInt("history")
	if err != nil {
		if err != config.ErrNotFound {
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
	h.useWorkspaceFallback = command == ""
	h.workspaceFallback = fallback

	h.ctx, h.cancelCtx = context.WithCancel(ctx)
	h.waitChan = make(chan error)

	listConfig := h.getListConfig(cfg)
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

	log.Debugf("useWorkspaceFallback set to %v", h.useWorkspaceFallback)

	go h.scanData()

	return h, nil
}

func (h *fuzzyFinderHandler) getListConfig(c config.Config) search.ListConfig {
	caseSensitive, err := c.GetBool("case_sensitive")
	if err != nil {
		if err != config.ErrNotFound {
			log.Errorf("failed to load 'case_sensitive' from config: %v", err)
		}
		caseSensitive = true
	}
	algoStr, err := c.GetString("algo")
	if err != nil && err != config.ErrNotFound {
		log.Errorf("failed to load 'algo' from config: %v", err)
	}
	algo := search.FuzzyMatch
	if algoStr == "equal" {
		algo = search.EqualMatch
	} else if algoStr == "contains" {
		algo = search.ContainsMatch
	}

	cfg := search.ListConfig{
		Algo:          algo,
		Interrupter:   h.p,
		CaseSensitive: caseSensitive,
	}

	matchedTextAttr, err := config.GetAttributes(c, "matched_text_attr")
	if err != nil && err != config.ErrNotFound {
		log.Errorf("failed to load 'matched_text_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'matched_text_attr' from config: %v", matchedTextAttr)
		cfg.MatchedTextAttr = &matchedTextAttr
	}

	countAttr, err := config.GetAttributes(c, "count_attr")
	if err != nil && err != config.ErrNotFound {
		log.Errorf("failed to load 'count_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'count_attr' from config: %v", countAttr)
		cfg.CountAttr = &countAttr
	}

	textAttr, err := config.GetAttributes(c, "element_attr")
	if err != nil && err != config.ErrNotFound {
		log.Errorf("failed to load 'element_attr' from config: %v", err)
	} else if err == nil {
		log.Tracef("loaded 'element_attr' from config: %v", textAttr)
		cfg.ElementAttr = &textAttr
	}

	focusAttr, err := config.GetAttributes(c, "focus_element_attr")
	if err != nil && err != config.ErrNotFound {
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

	if ev.KeyComb().Key == term.KeyCtrlC {
		if h.cancelScan != nil {
			h.cancelScan()
		}
		if h.pid != 0 {
			_ = h.killCommand()
		}
	}

	exit, handled = h.listHandler.Handle(ev)

	log.Tracef("fuzzyFinderHandler.Handle(%#v): %v", ev, handled)

	return
}

func (h *fuzzyFinderHandler) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	return h.listHandler.Cursor()
}

func (h *fuzzyFinderHandler) Man() tui.Manual {
	return h.listHandler.Man()
}

func (h *fuzzyFinderHandler) Close() error {
	h.cancelCtx()

	log.Tracef("fuzzyFinderHandler.Close(): %#v", h.pid)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.pid != 0 {
		h.killed = true
		_ = h.killCommand()
		h.pid = 0
	}
	return h.list.Close()
}
