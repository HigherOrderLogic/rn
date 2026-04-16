// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package finder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/handler/search"
	"unstable.build/go-tui/text/modeless"
)

const (
	readerBufferSize  = 64 * 1024
	defaultMaxHistory = 2000
)

// Permissions returns the permissions required by this extension.
func Permissions() []extensionapi.Permission {
	return []extensionapi.Permission{
		extensionapi.Permission(extensionapi.PermissionBrowserResourceOpener),
		extensionapi.Permission(extensionapi.PermissionInterrupt),
		extensionapi.Permission(extensionapi.PermissionNotifications),
		extensionapi.Permission(extensionapi.PermissionBrowserWindowManager),
		extensionapi.PermissionStorage,
		extensionapi.Permission(extensionapi.PermissionEditor),
		extensionapi.Permission(extensionapi.PermissionFileSystem),
		extensionapi.Permission(extensionapi.PermissionExecute),
	}
}

// Clients contains the workspace clients required by NewV2.
type Clients struct {
	Storage        storageapi.Service
	ResourceOpener browserapi.ResourceOpener
	WindowManager  browserapi.WindowManager
	Interrupter    term.Interrupter
	Notifications  browserapi.Notifications
	Editor         textapi.Editor
	FileSystem     workspaceapi.FileSystem
	Executor       workspaceapi.Executor
}

// RedispatchHandler is a browser handler that can handle repeated command
// invocations while its split window is already open.
type RedispatchHandler interface {
	browserapi.Handler
	Redispatch(context.Context, textapi.Command) error
}

// NewV2 returns a tui handler that uses native extensionv2 workspace clients.
func NewV2(
	ctx context.Context, clients Clients, invokeWindow browserapi.Window, cfg config.Config,
	historyKey term.KeyComb, historyDocumentID string, command string,
	fallback func(workspaceapi.FileSystem, context.Context) (iterator.Iterator[string], error),
	getResource func(exec workspaceapi.FileSystem, line string) (workspaceapi.URI, term.Coordinates, bool),
) (RedispatchHandler, error) {
	maxHistory, err := cfg.GetInt("history")
	if err != nil {
		if err != config.ErrNotFound {
			log.Errorf("failed to load 'history' from config: %v", err)
		}
		maxHistory = defaultMaxHistory
	} else {
		log.Tracef("loaded 'history' from config: %v", maxHistory)
	}

	h := &fuzzyFinderHandler{
		s:                    clients.Storage,
		f:                    clients.ResourceOpener,
		wm:                   clients.WindowManager,
		p:                    clients.Interrupter,
		m:                    clients.Notifications,
		ed:                   clients.Editor,
		fs:                   clients.FileSystem,
		executor:             clients.Executor,
		invokeWindow:         invokeWindow,
		historyKey:           historyKey,
		getResource:          getResource,
		cmdStr:               command,
		useWorkspaceFallback: command == "",
		workspaceFallback:    fallback,
		waitChan:             make(chan error),
	}
	if h.f == nil || h.p == nil {
		return nil, errors.New("extension is missing critical permissions")
	}
	if h.s != nil {
		h.history.Init(h.s, historyDocumentID, maxHistory)
		if err := h.history.Load(); err != nil {
			return nil, err
		}
	}

	h.ctx, h.cancelCtx = context.WithCancel(context.Background())

	listConfig := h.getListConfig(cfg)
	h.list.Init(listConfig)
	ed, _ := modeless.Editor(modeless.WithWrap(true)).
		Edit(workspaceapi.RandomURI("search"), h.list.Buffer(), false, false)
	h.listHandler = search.Handler(&h.list, ed, func(item string) {
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

type fuzzyFinderHandler struct {
	s                    storageapi.Service
	f                    browserapi.ResourceOpener
	wm                   browserapi.WindowManager
	p                    term.Interrupter
	m                    browserapi.Notifications
	ed                   textapi.Editor
	fs                   workspaceapi.FileSystem
	executor             workspaceapi.Executor
	invokeWindow         browserapi.Window
	historyKey           term.KeyComb
	mu                   sync.Mutex
	cmdStr               string
	getResource          func(workspaceapi.FileSystem, string) (workspaceapi.URI, term.Coordinates, bool)
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

func (h *fuzzyFinderHandler) execCommand(ctx context.Context, command string) (
	*os.File, *os.File, workspaceapi.Pid, error,
) {
	shell := os.Getenv("SHELL")
	if len(shell) == 0 {
		shell = "sh"
	}
	return h.execCommandWith(ctx, shell, command)
}

// Watch satisfies workspaceapi.Watcher which is employed
// to wait for the underlying command to execute.
func (h *fuzzyFinderHandler) WatchProcess() chan error {
	return h.waitChan
}

func (h *fuzzyFinderHandler) execCommandWith(
	ctx context.Context, shell string, commandStr string,
) (*os.File, *os.File, workspaceapi.Pid, error) {
	cmd := workspaceapi.Cmd{
		Path:    shell,
		Args:    []string{"-c", commandStr},
		Watcher: h,
	}
	stderr, stdout, err := h.setPipes(&cmd)
	if err != nil {
		return nil, nil, 0, err
	}
	pid, err := h.executor.Start(ctx, cmd)
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

	err := h.history.Add(searchQuery)
	if err != nil {
		log.Errorf("error adding search history: %v", err)
	} else {
		log.Tracef("added %q to query history", searchQuery)
	}
}

func (h *fuzzyFinderHandler) open(resource workspaceapi.URI) (browserapi.Handler, error) {
	return h.f.Open(resource)
}

func (h *fuzzyFinderHandler) setContent(
	resource workspaceapi.URI, b browserapi.Handler, pos term.Coordinates,
) error {
	err := h.wm.SetWindowContent(h.invokeWindow, b)
	if err != nil && !errors.Is(err, browserapi.ErrTabNotFree) {
		return err
	}
	if h.ed == nil {
		log.Info("could not set cursor position because host did not grant extensionapi.PermissionEditor")
		return nil
	}

	hed, err := h.ed.Editor(resource)
	if err != nil {
		return err
	}

	return h.ed.SetCursor(hed, pos)
}

func (h *fuzzyFinderHandler) notifyError(msg string, args ...any) error {
	// allow browser messenger permission to be denied
	if h.m == nil {
		return nil
	}

	_, err := h.m.Notify(browserapi.LevelError, msg, args...)
	return err
}

func (h *fuzzyFinderHandler) openResource(searchQuery, data string) {
	resource, pos, ok := h.getResource(h.fs, data)
	if !ok {
		_ = h.notifyError("line does not conform to file:location format")
		return
	}
	if resource == (workspaceapi.URI{}) {
		log.Warnf("trying to open a line with a parse error")
		return
	}
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
		resource, ok := it.Next(ctx)
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
	return it.Close()
}

func (h *fuzzyFinderHandler) scanDataViaWorkspaceAPI(
	ctx context.Context, datachan chan<- []byte, cancelScan func(),
) {
	defer cancelScan()
	err := h.doScanDataViaWorkspaceAPI(ctx, datachan)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("scan data: %v", err)

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
		h.scanDataViaWorkspaceAPI(ctx, datachan, cancelScan)
		return
	}

	log.Debugf("using resource list command: %s", h.cmdStr)

	stderr, stdout, exec, err := h.execCommand(ctx, h.cmdStr)
	if err != nil {
		log.Debugf("fallback to scan data via workspace API: %v", err)
		h.scanDataViaWorkspaceAPI(ctx, datachan, cancelScan)
		return
	}

	h.mu.Lock()
	h.pid = exec
	h.mu.Unlock()

	go h.readCommand(ctx, datachan, stdout, cancelScan)
	go func() {
		data, err := io.ReadAll(stderr)
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
	switch algoStr {
	case "equal":
		algo = search.EqualMatch
	case "contains":
		algo = search.ContainsMatch
	}

	cfg := search.ListConfig{
		Algo: algo,
		Interrupter: term.FuncInterrupter(func(ctx context.Context) error {
			err := h.p.Interrupt(ctx)
			if err != nil {
				log.Errorf("interrupt: %v", err)
			}
			return err
		}),
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

func (h *fuzzyFinderHandler) Redispatch(ctx context.Context, cmd textapi.Command) error {
	h.writeLastSearchQuery()
	return nil
}

func (h *fuzzyFinderHandler) Handle(ev term.Event) (exit, handled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Type != term.EventKey {
		return
	}

	comb := ev.KeyComb()
	if comb == h.historyKey {
		h.writeLastSearchQuery()
		handled = true
		return
	}

	if comb.Ch == 'c' && comb.Mod == term.ModCtrl {
		h.killed = true
		if h.cancelScan != nil {
			h.cancelScan()
		}
	}

	exit, handled = h.listHandler.Handle(ev)

	log.Tracef("fuzzyFinderHandler.Handle(%#v): %v", ev, handled)

	return
}

func (h *fuzzyFinderHandler) Selection() (string, bool) {
	return h.listHandler.Selection()
}

func (h *fuzzyFinderHandler) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	return h.listHandler.Cursor()
}

func (h *fuzzyFinderHandler) Close() (ret error) {
	h.cancelCtx()

	log.Tracef("fuzzyFinderHandler.Close(): %#v", h.pid)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.killed = true
	if h.cancelScan != nil {
		h.cancelScan()
	}
	if err := h.list.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if h.s != nil {
		if err := h.s.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return ret
}
