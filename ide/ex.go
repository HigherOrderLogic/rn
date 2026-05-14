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

package ide

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	fileexplorercomp "unstable.build/go-tui/component/fileexplorer"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	thandler "unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/ideshell"
	"unstable.build/go-tui/ide/ideshell/workspaceshell"
	"unstable.build/go-tui/ide/idetask"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/ide/vctrl"
	tterm "unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/term/vte/vtereservoir"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
)

const (
	commandHistoryDocumentID = "command-history:ex-command-history"
	shellHistoryDocumentID   = "shell-history:ex-shell-history"
	reissuePadding           = 10 * time.Millisecond
	// fileExplorerURI is the pseudo-URI used to identify the file
	// explorer's in-memory buffer. Exposed so that infrastructure
	// that snapshots open files (session restore, history tracking)
	// can skip it instead of trying to re-open it as a regular tab.
	fileExplorerURI = "memory:///fexplorer"
)

var (
	errInvalidSetCursor    = errors.New("cannot set cursor on this buffer")
	errEventStreamNotReady = errors.New("event stream not ready to publish")
	errInvalidTab          = errors.New("expected exactly one argument with the position")
)

// ErrFlushPendingQuit is returned by :q and :wq when one or more
// buffers have an outstanding async save in flight. The user can
// wait for the save to complete, cancel it with :writecancel, or
// force the quit via :q! / :writeforcequit!.
var ErrFlushPendingQuit = errors.New(
	"save in progress; wait, run :writecancel, or use :q! / :wq")

type pluginHandler interface {
	browserapi.Floating
	OnFocusChange(bool)
}

type macroRecorder interface {
	IsRecording() bool
	RegisterID() string
}

// ex implements a tui.Handler by wrapping an editor.Component and
// providing an ex editor type of interface.
type ex struct {
	config     text.Config
	comp       text.Component
	clip       clipboard.Register
	executor   schemeapi.Executor
	ed         text.Editor
	parser     syntaxapi.Parser
	wsExecutor *workspaceshell.Executor
	// extensionsExecutor tracks extension binaries launched by the
	// IDE's extension runner. It backs the "extensions-process" REPL
	// command so users can list, signal, or stop extension PIDs the
	// same way they would workspace processes.
	extensionsExecutor *workspaceshell.Executor
	storage            storageapi.Service
	workspaceURI       workspaceapi.URI
	closed             bool
	reservoir          *vtereservoir.Facility
	// initialReservoirCapacity preserves the configured pool size so
	// that setExecutor can re-create the reservoir without racing
	// against the asynchronous initCap (Capacity() returns the
	// momentary length of the pool, which is usually 0 right after
	// extension load).
	initialReservoirCapacity int
	// do not use directly, use notifications below instead
	// which is able to dispatch cross-workspace cues.
	container            *notifications.Container
	notifications        browserapi.Notifications
	emulatorConfig       vte.Config
	newEmulatorHandler   func([]string) (vtereservoir.VTE, error)
	tm                   browser.TabManager
	newPluginHandler     func(...string) (pluginHandler, error)
	workspace            workspace.Workspace
	tasks                *idetask.Manager
	dispatchOnPreview    map[string]PreviewFunc
	filepathCompleter    command.Completer
	sequencer            thandler.Sequencer
	publishEvent         func(term.Event) bool
	cancelPartialReissue func()
	ctxPartialReissue    context.Context
	reissueEvent         term.Event
	cmd                  *command.Prompt
	syncCommandPrompt    bool
	// commandEditor, when non-nil, drives the command Prompt's modal
	// edit mode. It is set by the workspace handler to a bare vi or
	// modeless handler (no aux/status/icons bars) chosen from the
	// active editor mode configuration. Tests that construct ex
	// directly leave it nil and fall back to wrapping e.ed.Edit.
	commandEditor     command.Editor
	pluginWaitTimeout time.Duration
	// use floating windows functionality without having to work around focus commands
	// and how to se cmd.Window correctly.
	cmdV             handler.Virtual[*browser.Component]
	cmdWin           browser.Window
	fullscreenID     uint64
	exit             bool
	forceExit        bool
	height           int
	width            int
	isPromptDispatch bool
	macro            macroRecorder

	companionTerminal    vtereservoir.VTE
	companionTerminalWin browser.Window
	companionShell       *ideshell.Handler
	companionShellURI    workspaceapi.URI

	fileExplorerWin    browser.Window
	fileExplorerTarget browser.Window

	// fileExplorerHandler is created lazily on the first :fexplorer
	// invocation and reused across toggles. Recreating it would call
	// e.ed.Edit a second time on the same memory URI, which would
	// re-subscribe the vi editor's per-file fold/location/git
	// commands and fail with "command already registered".
	fileExplorerHandler *fileExplorerHandler

	// sched serializes UI-thread work (callbacks from async flush
	// goroutines, etc.). Set by the workspace handler after newEx.
	// When nil, async completion callbacks run inline on the
	// goroutine that delivered the result, which is unsafe for
	// production UI but acceptable in tests that don't drive a
	// real event loop.
	sched func(func()) bool
	// flusher owns the per-URI cancel/awaiter state for async
	// Flush/ForceFlush/Reload/Overwrite operations. It serialises
	// completion notifications through e.sched so all map mutations
	// happen on the UI goroutine. Created in init().
	flusher *flusher
}

// PreviewFunc is a function used to preview commands.
// The first argument returns a component to render alongside the command
// prompt and the function is used to cancel any mutable effects.
type PreviewFunc = func(string, ...string) (component.Responsive, func(), bool)

func newEx(
	ed text.Editor, m workspace.Workspace,
	storage storageapi.Service,
	notifications *notisManager,
	uri workspaceapi.URI,
	emulatorConfig vte.Config,
	pluginBarConfig plugin.BarConfig,
	publishEvent func(term.Event) bool,
	initialVTECapacity int,
	clip clipboard.Register,
	macro macroRecorder,
	dispatchOnPreview map[string]PreviewFunc,
	tm browser.TabManager,
	parser syntaxapi.Parser,
	opts ...text.Option,
) (e *ex, err error) {
	e = new(ex)
	err = e.init(ed, m, storage, notifications, uri,
		emulatorConfig, pluginBarConfig, publishEvent, initialVTECapacity, clip, macro,
		dispatchOnPreview, tm, parser, opts...)
	if err != nil {
		return
	}
	return
}

// init initializes this ex with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (e *ex) init(
	ed text.Editor, m workspace.Workspace,
	storage storageapi.Service,
	notifications *notisManager,
	uri workspaceapi.URI,
	emulatorConfig vte.Config,
	pluginBarConfig plugin.BarConfig,
	publishEvent func(term.Event) bool,
	initialVTECapacity int,
	clip clipboard.Register,
	macro macroRecorder,
	dispatchOnPreview map[string]PreviewFunc,
	tm browser.TabManager,
	parser syntaxapi.Parser,
	opts ...text.Option,
) (err error) {
	err = e.doInit(ed, m, storage, notifications, uri,
		emulatorConfig, publishEvent, clip, opts...)
	if err != nil {
		return
	}
	e.parser = parser
	err = e.comp.Init(ed, m, e.config)
	if err != nil {
		return
	}
	if tm == nil {
		tm = e.Browser()
	}
	e.tm = tm
	e.comp.SubscribeWindow((*windowSubscriber)(e))
	if initialVTECapacity != 0 {
		e.initialReservoirCapacity = initialVTECapacity
		e.reservoir = vtereservoir.New(e.Browser(), e.Browser(),
			e.workspace, e.executor, e.tm, e.emulatorConfig, initialVTECapacity)
	}
	e.newEmulatorHandler = func(cmdAndArgs []string) (
		vtereservoir.VTE, error,
	) {
		if e.reservoir != nil && argsMatchEmulatorShell(cmdAndArgs, e.emulatorConfig.CommandAndArgs) {
			e.log(log.TraceLevel, "getting vte instance from reservoir")
			return e.reservoir.Get()
		}
		cfg := e.emulatorConfig
		if len(cmdAndArgs) != 0 {
			cfg.CommandAndArgs = cmdAndArgs
		}
		v, err := vte.NewHandler(e.Browser(), e.Browser(),
			e.workspace, e.executor, e.tm, cfg)
		if err != nil {
			return nil, err
		}
		return vteAdapter{v}, nil
	}
	pluginOpts := []plugin.Option{
		plugin.WithVTEConfig(e.emulatorConfig),
		plugin.WithFrame(e.config.Frame),
		plugin.WithFrameCharSet(e.config.FocusFrameCharSet),
		plugin.WithFrameAttr(e.config.FocusFrameAttr),
		plugin.WithBarConfig(pluginBarConfig),
	}
	e.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return plugin.New(e.Browser(), e.Browser(), e.executor, e.workspace,
			e.tm, args, e.width, pluginOpts...)
	}
	e.dispatchOnPreview = dispatchOnPreview
	e.macro = macro
	e.filepathCompleter = command.FilePathCompleter(e.workspace)
	// sched is the event-loop scheduler; callers (production wires
	// emulatorConfig.ScheduleNextTick; tests must install a
	// serializing scheduler) must supply a non-nil value.
	if emulatorConfig.ScheduleNextTick == nil {
		panic("ide.ex: emulatorConfig.ScheduleNextTick must not be nil")
	}
	e.sched = emulatorConfig.ScheduleNextTick
	e.flusher = newFlusher(&e.comp, e.notifications, e.sched)
	e.tasks = idetask.NewManager(&e.comp, tm, m,
		emulatorConfig.ScheduleNextTick, pluginOpts...)
	e.tasks.SetFrameAttr(e.config.FrameAttr)
	e.comp.SubscribeWindow(e.tasks)
	return
}

func (e *ex) subscribeCommands() error {
	var ret error
	for name, man := range exCommands {
		// Name is only defined as a key to exCommands
		man.man.Name = name
		err := e.comp.SubscribeCommand(man.man, text.FuncCommandHandler(
			func(ctx context.Context, cmd textapi.Command) error {
				return man.handler(e, ctx, cmd.Args...)
			}, func(ctx context.Context, cmd textapi.Command) (
				iterator.Iterator[string], string, error,
			) {
				if man.completer == nil {
					return iterator.FromSlice[string](nil), "", nil
				}
				return man.completer(e, ctx, cmd)
			}))
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("subscribe command: %w", err))
		}
	}
	return ret
}

func (e *ex) setExecutor(
	exe schemeapi.Executor,
	wsExec *workspaceshell.Executor,
	extExec *workspaceshell.Executor,
) {
	e.executor = exe
	e.wsExecutor = wsExec
	e.extensionsExecutor = extExec
	if e.reservoir != nil {
		_ = e.reservoir.Close()
		e.reservoir = vtereservoir.New(e.Browser(), e.Browser(),
			e.workspace, e.executor, e.tm, e.emulatorConfig,
			e.initialReservoirCapacity)
	}
}

func (e *ex) completeReadFile(
	ctx context.Context, args []string,
) (iterator.Iterator[string], string, error) {
	// `:readfile` auto-completion works the same as the `:edit` command.
	return e.filepathCompleter.Complete(ctx, args)
}

func (e *ex) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "ide.ex").Logf(level, msg, args...)
}

func (e *ex) Interrupt(ctx context.Context) error {
	payload, _ := term.PayloadFromContext(ctx)
	ev := term.Event{Type: term.EventInterrupt, Raw: payload, Context: ctx}
	if !e.publishEvent(ev) {
		return errEventStreamNotReady
	}
	return nil
}

func (e *ex) doInit(
	ed text.Editor, m workspace.Workspace,
	storage storageapi.Service,
	n *notisManager,
	uri workspaceapi.URI,
	emulatorConfig vte.Config,
	publishEvent func(term.Event) bool,
	clip clipboard.Register,
	opts ...text.Option,
) (err error) {
	e.executor = m
	e.pluginWaitTimeout = 3 * time.Second
	e.clip = clip
	e.workspace = m
	e.container = notifications.New(&e.comp, n.cfg)
	e.notifications = n.new(uri, e.container)
	e.publishEvent = publishEvent
	e.storage = storage
	e.workspaceURI = uri
	e.emulatorConfig = emulatorConfig

	e.config = text.DefaultConfig()

	opts = append(opts, text.WithNotifications(e.notifications))
	for _, o := range opts {
		o(&e.config)
	}
	// Wire the event-loop scheduler into text.Config so async flush
	// completion can run dispatchFlush / resetTabProperties on the
	// UI goroutine instead of racing with Draw.
	if e.config.ScheduleNextTick == nil {
		e.config.ScheduleNextTick = emulatorConfig.ScheduleNextTick
	}

	seqInterests := make([]thandler.Sequence, 0,
		len(e.config.CommandSequenceBindings))
	for seq := range e.config.CommandSequenceBindings {
		seqInterests = append(seqInterests, seq)
	}
	e.sequencer.Init(seqInterests, e.config.SequencerTimeout)

	e.ed = ed
	e.cleanPartialReissueState()
	e.cmdV.C = browser.NewComponent(e.config.Config)
	return
}

func (e *ex) Wait() {
	if e.cmd != nil {
		e.cmd.Wait()
	}
}

// Complete satisfies command.Completer for command.Handler.
func (e *ex) Complete(ctx context.Context, args []string) (
	iterator.Iterator[string], string, error,
) {
	if len(args) == 0 {
		return nil, "", errors.New("missing command")
	}
	uri, h, ok := e.handlerInFocus()
	scmd := textapi.Command{
		Name:     args[0],
		Args:     args[1:],
		Resource: h,
		URI:      uri,
		Window:   e.invokeWindow(),
	}
	if ok {
		scmd.Cursor.Content = h.CursorAtScroll()
		scmd.Cursor.Window, _, _ = h.Cursor()
	}
	it, newArg, err := e.comp.CompleteCommand(ctx, scmd)
	if err != nil {
		e.setError(fmt.Errorf("complete command: %v", err))
	}
	return it, newArg, err
}

// Dispatch satisfies command.Dispatcher for command.Handler.
func (e *ex) Dispatch(command string, args ...string) bool {
	e.isPromptDispatch = true
	quit, err := e.runCommand(command, args)
	if err != nil {
		e.setError(err)
	}
	e.isPromptDispatch = false
	return quit
}

// Preview satisfies command.Dispatcher for command.Handler.
func (e *ex) Preview(command string, args ...string) (component.Responsive, func(), bool) {
	if e.dispatchOnPreview == nil {
		return nil, nil, false
	}
	do, ok := e.dispatchOnPreview[command]
	if !ok {
		return nil, nil, false
	}
	return do(command, args...)
}

func (e *ex) handlerInFocus() (workspaceapi.URI, text.Handler, bool) {
	content, _ := e.invokeWindow().Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return workspaceapi.URI{}, nil, false
	}
	ret, ok := t.Handler().(text.Handler)
	if !ok {
		return workspaceapi.URI{}, nil, false
	}
	return t.URI(), ret, true
}

// focusTab returns the focused tab (and its URI) for use with the
// async FlushTab/ReloadTab/OverwriteTab APIs. Unlike handlerInFocus,
// it does not unwrap the tab's inner text.Handler — those APIs need
// the *browser.Tab so they can locate its FlusherCloser.
func (e *ex) focusTab() (workspaceapi.URI, browserapi.Handler, bool) {
	content, _ := e.invokeWindow().Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return workspaceapi.URI{}, nil, false
	}
	return t.URI(), t, true
}

func (e *ex) moveFocusCursor(line int) error {
	_, h, ok := e.handlerInFocus()
	if !ok {
		return errInvalidSetCursor
	}
	// silently correct invalid line numbers
	// NOTE: this won't work if we implement +/- relative
	// line go to i.e. :+1, :-20
	if line < 0 {
		line = 0
	}
	ok = h.SetCursorAtScroll(term.Coordinates{Y: line})
	if !ok {
		return errors.New("could not set cursor to position")
	}
	return nil
}

func (e *ex) tabrename(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("expected at least one argument with new tab name")
	}

	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return errors.New("cannot rename companion terminal")
	}
	content, _ := win.Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return errors.New("cannot rename non tab")
	}
	var attrs term.Attributes
	if len(args) > 1 {
		attrs.Fg = tcell.GetColor(args[1])
		if len(args) > 2 {
			attrs.Bg = tcell.GetColor(args[2])
		}
	}
	return e.Browser().SetTabName(t.URI(), args[0], attrs)
}

func (e *ex) tabprevious(_ context.Context, args ...string) error {
	b := e.comp.Browser()
	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.PreviousTab(e.invokeWindow())
	return nil
}

func (e *ex) tabnext(_ context.Context, args ...string) error {
	b := e.comp.Browser()
	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.NextTab(e.invokeWindow())
	return nil
}

func (e *ex) tabfocus(_ context.Context, args ...string) error {
	if len(args) < 1 {
		return errInvalidTab
	}
	idxStr := args[0]
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return errInvalidTab
	}
	if idx == 0 {
		return errors.New("the first tab is 1")
	}
	b := e.comp.Browser()
	b.SetContentToTab(e.invokeWindow(), idx-1)
	return nil
}

func (e *ex) tabclose(_ context.Context, args ...string) error {
	b := e.comp.Browser()
	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	content, err := win.Content()
	if err != nil {
		return fmt.Errorf("get window content: %w", err)
	}
	if tab, ok := content.(*browser.Tab); ok && e.tabIsDirty(tab) {
		e.openCloseDirtyTabsPrompt(
			fmt.Sprintf("File '%s' has changes pending to be written. "+
				"Close and discard changes?", tab.URI().Name()),
			func() error {
				b.RemoveWindowContent(win)
				return nil
			},
		)
		return nil
	}
	// on focus dispatch to vte.Handler via Close
	b.RemoveWindowContent(win)
	return nil
}

func (e *ex) tabcloseall(_ context.Context, args ...string) error {
	b := e.comp.Browser()
	dirty := e.dirtyTabCount(e.comp.Tabs())
	if dirty > 0 {
		e.openCloseDirtyTabsPrompt(
			fmt.Sprintf("There are %d tabs with changes pending to be written. "+
				"Close and discard changes?", dirty),
			func() error {
				b.RemoveAllTabs()
				return nil
			},
		)
		return nil
	}
	// on focus dispatch to vte.Handler via Close
	b.RemoveAllTabs()
	return nil
}

func (e *ex) tabcloseinactive(_ context.Context, args ...string) error {
	b := e.comp.Browser()
	inactive := e.inactiveTabs()
	dirty := e.dirtyTabCount(inactive)
	if dirty > 0 {
		e.openCloseDirtyTabsPrompt(
			fmt.Sprintf("There are %d inactive tabs with changes pending to be written. "+
				"Close and discard changes?", dirty),
			func() error {
				if removed := b.RemoveInactiveTabs(); !removed {
					return errors.New("no inactive tabs left")
				}
				return nil
			},
		)
		return nil
	}
	if removed := b.RemoveInactiveTabs(); !removed {
		return errors.New("no inactive tabs left")

	}
	return nil
}

func (e *ex) tabIsDirty(tab *browser.Tab) bool {
	dirty, ok := e.comp.IsDirty(tab.URI())
	return ok && dirty
}

func (e *ex) dirtyTabCount(tabs []*browser.Tab) int {
	var dirty int
	for _, tab := range tabs {
		if e.tabIsDirty(tab) {
			dirty++
		}
	}
	return dirty
}

func (e *ex) inactiveTabs() []*browser.Tab {
	var inactive []*browser.Tab
	for _, tab := range e.comp.Tabs() {
		if _, ok := tab.Window(); ok {
			continue
		}
		inactive = append(inactive, tab)
	}
	return inactive
}

func (e *ex) closeFocusWindow(_ context.Context, args ...string) error {
	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	// on focus dispatch to vte.Handler via Close
	return win.Close()
}

func (e *ex) windowcloseall(_ context.Context, args ...string) error {
	return e.comp.Browser().CloseOtherWindows(e.invokeWindow())
}

func (e *ex) flushCloseIgnoreNonFlushed(_ context.Context, args ...string) error {
	if h, ok := e.fileExplorerHandlerInFocus(); ok {
		return h.forceFlush()
	}
	// :writeforcequit! — exit immediately and let any in-flight or
	// newly-started saves complete in the background.
	e.forceExit = true
	e.exit = true
	uri, t, ok := e.focusTab()
	if !ok {
		return nil
	}
	_ = e.flusher.forceFlush(uri, t)
	return nil
}

func (e *ex) flushClose(_ context.Context, args ...string) error {
	if h, ok := e.fileExplorerHandlerInFocus(); ok {
		return h.flush()
	}
	uri, t, ok := e.focusTab()
	if !ok {
		// nothing to flush; behave like :q
		e.forceExit = false
		e.exit = true
		return nil
	}
	// :wq — start an async flush; on success, request exit.
	return e.flusher.flushAndThen(uri, t, false /* force */, func() {
		e.forceExit = false
		e.exit = true
	})
}

func (e *ex) flush(ctx context.Context, args ...string) error {
	if h, ok := e.fileExplorerHandlerInFocus(); ok {
		return h.flush()
	}
	if h, ok := e.terminalInFocus(); ok {
		var name string
		var err error
		if len(args) == 0 {
			name, err = e.nextTerminalSessionName(ctx, h)
		} else {
			name, err = normalizeTerminalSessionName(args)
		}
		if err != nil {
			return err
		}
		return e.saveTerminalSession(ctx, name, h)
	}
	uri, t, ok := e.focusTab()
	if !ok {
		return textapi.ErrInvalidSave
	}
	return e.flusher.flush(uri, t)
}

func (e *ex) forceFlush(ctx context.Context, args ...string) error {
	if h, ok := e.fileExplorerHandlerInFocus(); ok {
		return h.forceFlush()
	}
	if h, ok := e.terminalInFocus(); ok {
		var name string
		var err error
		if len(args) == 0 {
			name, err = e.nextTerminalSessionName(ctx, h)
		} else {
			name, err = normalizeTerminalSessionName(args)
		}
		if err != nil {
			return err
		}
		return e.saveTerminalSession(ctx, name, h)
	}
	uri, t, ok := e.focusTab()
	if !ok {
		return textapi.ErrInvalidSave
	}
	return e.flusher.forceFlush(uri, t)
}

func (e *ex) flushAll(_ context.Context, args ...string) (ret error) {
	for _, t := range e.comp.Tabs() {
		if err := e.flusher.flush(t.URI(), t); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (e *ex) forceFlushAll(_ context.Context, args ...string) (ret error) {
	for _, t := range e.comp.Tabs() {
		if err := e.flusher.forceFlush(t.URI(), t); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (e *ex) forcequit(_ context.Context, args ...string) error {
	e.forceExit = true
	e.exit = true
	return nil
}
func (e *ex) quit(_ context.Context, args ...string) error {
	if e.flusher.inFlightCount() > 0 {
		return ErrFlushPendingQuit
	}
	e.forceExit = false
	e.exit = true
	return nil
}

// writeCancel cancels the in-flight flush for the focused tab, if
// any. The underlying scheme call (e.g. gRPC Rename) cannot be
// aborted; its result will be discarded when it eventually returns.
func (e *ex) writeCancel(_ context.Context, args ...string) error {
	uri, _, ok := e.focusTab()
	if !ok {
		return workspace.ErrNoFlushInProgress
	}
	return e.flusher.cancel(uri)
}

// waitInflight blocks until all in-flight async Flush/ForceFlush/
// Reload awaiter goroutines have completed and their completion
// callbacks have been dispatched through sched. This is intended
// for tests and for IDE shutdown — production UI code should
// never need to wait on this directly because the UI keeps
// responding while saves are in flight.
func (e *ex) waitInflight() {
	e.flusher.wait()
}

func (e *ex) dispatchCommand(cmd string, args ...string) (err error) {
	uri, h, ok := e.handlerInFocus()
	scmd := textapi.Command{
		Name:     cmd,
		Args:     args,
		Resource: h,
		URI:      uri,
		Window:   e.invokeWindow(),
	}
	if ok {
		scmd.Cursor.Content = h.CursorAtScroll()
		scmd.Cursor.Window, _, _ = h.Cursor()
	}
	var handled bool
	handled, err = e.comp.DispatchCommand(context.Background(), scmd)
	if err != nil {
		return
	}
	if !handled {
		target, ok := e.config.CommandAliases[cmd]
		if !ok && e.workspace == nil {
			err = fmt.Errorf("unknown command or alias %q or cannot run on an empty workspace", cmd)
		} else if !ok {
			err = fmt.Errorf("unknown command or command alias %q", cmd)
		} else if e.workspace == nil {
			err = fmt.Errorf("cannot run %q (alias of %v) on an empty workspace",
				cmd, target)
		} else {
			err = fmt.Errorf("%s is aliased to an unknown command %v", cmd, target)
		}
	}
	return
}

func (e *ex) editFileURI(uri workspaceapi.URI, win browser.Window, readOnly bool) (
	*browser.Tab, error,
) {
	return e.editFileURILocal(uri, win, readOnly)
}

func (e *ex) editFileURILocal(uri workspaceapi.URI, win browser.Window, readOnly bool) (
	*browser.Tab, error,
) {
	var h browserapi.Handler
	var err error
	if readOnly {
		h, err = e.comp.OpenReadOnly(uri)
	} else {
		h, err = e.comp.Open(uri)
	}
	if err != nil {
		return nil, err
	}
	if win.Closed() {
		win, _ = e.comp.Focus()
	}
	err = win.SetContent(h)
	if err == browserapi.ErrTabNotFree {
		err = nil
	}
	return h.(*browser.Tab), err
}

func (e *ex) parseURIOrWorkspaceURI(path string) (workspaceapi.URI, error) {
	uri, err := workspaceapi.ParseURI(path)
	if err != nil {
		uri, err = e.workspace.URI(path)
		e.log(log.TraceLevel, "URI (%s): %s, %v", path, uri.String(), err)
	}
	return uri, err
}

func (e *ex) editFiles(_ context.Context, args ...string) error {
	return e.editFilesReadOnly(false, args...)
}

func (e *ex) viewFiles(_ context.Context, args ...string) error {
	return e.editFilesReadOnly(true, args...)
}

func (e *ex) editFilesReadOnly(readOnly bool, args ...string) error {
	if len(args) == 0 {
		return errors.New("expected at least one file name")
	}

	if e.invokeWindow() == e.companionTerminalWin {
		if err := e.toggleCompanionTerminal(); err != nil {
			return fmt.Errorf("toggle companion terminal: %v", err)
		}
	}

	for _, arg := range args {
		// attempt to parse URI otherwise expect local file path
		uri, err := e.parseURIOrWorkspaceURI(arg)
		if err != nil {
			return err
		}
		_, err = e.editFileURI(uri, e.invokeWindow(), readOnly)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *ex) tabcopypath(_ context.Context, args ...string) error {
	absolute := len(args) > 0 && args[0] == "absolute"
	t, ok := e.comp.FocusTab()
	if !ok {
		return errors.New("not a tab")
	}
	if _, ok := t.Handler().(text.Handler); !ok {
		return errors.New("not a file")
	}

	uri := t.URI()
	path := e.copyPath(uri, absolute)

	err := e.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: path})
	if err != nil {
		return fmt.Errorf("clipboard copy: %v", err)
	}

	_, _ = e.notifications.Notify(browserapi.LevelSuccess,
		"file path copied to clipboard")

	return nil
}

func (e *ex) tabcopylocation(_ context.Context, args ...string) error {
	absolute := len(args) > 0 && args[0] == "absolute"
	uri, h, ok := e.handlerInFocus()
	if !ok {
		return errors.New("not a file")
	}

	location := fmt.Sprintf("%s:%d", e.copyPath(uri, absolute), h.CursorAtScroll().Y+1)
	err := e.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: location})
	if err != nil {
		return fmt.Errorf("clipboard copy: %v", err)
	}

	_, _ = e.notifications.Notify(browserapi.LevelSuccess,
		"file location copied to clipboard")

	return nil
}

func (e *ex) copyPath(uri workspaceapi.URI, absolute bool) string {
	if absolute {
		return uri.Path()
	}
	path := workspaceapi.RelPath(e.workspaceURI, uri)
	if path == "" || path == "." || path == uri.String() {
		path = uri.Path()
	}
	return path
}

func (e *ex) reloadfile(_ context.Context, args ...string) error {
	focus := e.invokeWindow()
	content, err := focus.Content()
	if err != nil {
		return fmt.Errorf("get window content: %w", err)
	}
	t, ok := content.(*browser.Tab)
	if !ok {
		return textapi.ErrInvalidReload
	}
	return e.flusher.reload(t.URI(), t)
}

func (e *ex) splitDirectionChange(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects argument 'horizontal', 'h', 'vertical', 'v'")
	}

	b := e.comp.Browser()
	switch args[0] {
	case "horizontal", "h":
		b.SetDefaultSplit(browserapi.OrientationBottom)
		_, _ = e.notifications.Notify(browserapi.LevelInfo, "changed split direction to horizontal")
	case "vertical", "v":
		b.SetDefaultSplit(browserapi.OrientationRight)
		_, _ = e.notifications.Notify(browserapi.LevelInfo, "changed split direction to vertical")
	}
	return nil
}

func (e *ex) windownewHandler(h browserapi.Handler, orientation browserapi.Orientation) {
	eb := e.comp.Browser()
	win := e.invokeWindow()
	eb.Split(orientation, win, h)
}

func (e *ex) invokeWindow() browser.Window {
	ret, _ := e.comp.Focus()
	return ret
}

func (e *ex) windowfocus(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}

	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}

	switch args[0] {
	case "right":
		e.comp.Browser().FocusRight()
	case "down":
		e.comp.Browser().FocusDown()
	case "left":
		e.comp.Browser().FocusLeft()
	case "up":
		e.comp.Browser().FocusUp()
	default:
		return fmt.Errorf("invalid argument %q", args[0])
	}
	return nil
}

func (e *ex) moveWindow(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}

	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}

	var ok bool
	switch args[0] {
	case "right":
		ok = e.comp.Browser().SwapContentRight()
		if ok {
			e.comp.Browser().FocusRight()
		}
	case "down":
		ok = e.comp.Browser().SwapContentDown()
		if ok {
			e.comp.Browser().FocusDown()
		}
	case "left":
		ok = e.comp.Browser().SwapContentLeft()
		if ok {
			e.comp.Browser().FocusLeft()
		}
	case "up":
		ok = e.comp.Browser().SwapContentUp()
		if ok {
			e.comp.Browser().FocusUp()
		}
	default:
		return fmt.Errorf("invalid argument %q", args[0])
	}
	if !ok {
		return errors.New("cannot move window in this direction")
	}
	return nil
}

func (e *ex) moveTab(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}

	var err error
	switch args[0] {
	case "right":
		err = e.comp.Browser().MoveTabRight(e.invokeWindow())
	case "left":
		err = e.comp.Browser().MoveTabLeft(e.invokeWindow())
	default:
		var idx int
		idx, err = strconv.Atoi(args[0])
		if err != nil {
			return errInvalidTab
		}
		if idx == 0 {
			return errors.New("the first tab is 1")
		}
		err = e.comp.Browser().MoveTabTo(e.invokeWindow(), idx-1)
	}
	return err
}

var defaultConvertTabIcon = ''

func (e *ex) convertTab(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument with tab name")
	}

	var icon rune
	name := args[0]
	if len(args) > 1 {
		runeArgs := []rune(args[1])
		if len(runeArgs) > 0 {
			icon = runeArgs[0]
		}
	}
	if icon == 0 {
		icon = defaultConvertTabIcon
	}
	win := e.invokeWindow()
	tab, ok := e.comp.Browser().NewTabFromContent(icon, name, win)
	if !ok {
		return errors.New("window content is already a tab")
	}

	if win == e.companionTerminalWin {
		_ = e.toggleCompanionTerminal()
		e.companionTerminal = nil // force re-open next time
		other, _ := e.comp.Focus()
		return other.SetContent(tab)
	}

	if win.IsFloating() {
		err := win.Close()
		if err == nil {
			other, _ := e.comp.Focus()
			return other.SetContent(tab)
		}
		return err
	}
	return nil
}

func (e *ex) windowresize(_ context.Context, args ...string) error {
	if (len(args) == 1 && args[0] != "reset") || len(args) == 0 {
		return errors.New("invalid arguments")
	}

	var ok bool
	switch args[0] {
	case "reset":
		ok = e.comp.Browser().ResetWindowSize()
	case "min":
		switch args[1] {
		case "height":
			ok = e.comp.Browser().SetMinWindowHeight()
		case "width":
			ok = e.comp.Browser().SetMinWindowWidth()
		default:
			return fmt.Errorf("invalid second argument %q", args[1])
		}
	case "max":
		switch args[1] {
		case "height":
			ok = e.comp.Browser().SetMaxWindowHeight()
		case "width":
			ok = e.comp.Browser().SetMaxWindowWidth()
		default:
			return fmt.Errorf("invalid second argument %q", args[1])
		}
	case "increase":
		switch args[1] {
		case "height":
			ok = e.comp.Browser().IncreaseWindowHeight()
		case "width":
			ok = e.comp.Browser().IncreaseWindowWidth()
		default:
			return fmt.Errorf("invalid second argument %q", args[1])
		}
	case "decrease":
		switch args[1] {
		case "height":
			ok = e.comp.Browser().DecreaseWindowHeight()
		case "width":
			ok = e.comp.Browser().DecreaseWindowWidth()
		default:
			return fmt.Errorf("invalid second argument %q", args[1])
		}
	default:
		return fmt.Errorf("invalid first argument %q", args[0])
	}
	if !ok {
		return errors.New("could not resize this window in this way")
	}
	return nil
}

func (e *ex) sendNotificationInfo(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	_, err := e.notifications.Notify(browserapi.LevelInfo, strings.Join(args, " "))
	return err
}

func (e *ex) sendNotificationSuccess(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	_, err := e.notifications.Notify(browserapi.LevelSuccess, strings.Join(args, " "))
	return err
}

func (e *ex) sendNotificationWarning(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	_, err := e.notifications.Notify(browserapi.LevelWarn, strings.Join(args, " "))
	return err
}

func (e *ex) sendNotificationError(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	_, err := e.notifications.Notify(browserapi.LevelError, strings.Join(args, " "))
	return err
}

func (e *ex) windowtogglemaximize(_ context.Context, args ...string) error {
	if e.fullscreenID != 0 {
		e.comp.Browser().ResetWindowSize()
		e.fullscreenID = 0
		return nil
	}
	win := e.invokeWindow()
	e.comp.Browser().SetMaxWindowHeight()
	e.comp.Browser().SetMaxWindowWidth()
	e.fullscreenID = win.WindowID()
	return nil
}

func (e *ex) closeNotifications(_ context.Context, args ...string) error {
	e.container.CloseAll()
	return nil
}

func (e *ex) pauseNotifications(_ context.Context, args ...string) error {
	e.container.PauseAll()
	return nil
}

func (e *ex) resumeNotifications(_ context.Context, args ...string) error {
	e.container.ResumeAll()
	return nil
}

func (e *ex) pasteFromClipboard(_ context.Context, args ...string) error {
	handler := e.focusHandler()
	data, err := e.clip.Paste(clipboard.DefaultRegisterID)
	if err != nil {
		return fmt.Errorf("clipboard paste: %w", err)
	}
	if len(data.Text) == 0 {
		_, _ = e.Browser().Notify(browserapi.LevelInfo, "nothing to paste")
		return nil
	}

	handler.Handle(term.Event{
		Type: term.EventPasteStart,
	})
	for _, ch := range data.Text {
		raw := []byte(string(ch))
		handler.Handle(term.Event{
			Type: term.EventKey,
			Ch:   ch,
			Raw:  raw,
		})
	}
	handler.Handle(term.Event{
		Type: term.EventPasteEnd,
	})
	return nil
}

func (e *ex) copyToClipboard(_ context.Context, args ...string) error {
	handler := e.focusHandler()
	data, ok := handler.Selection()
	if !ok {
		_, _ = e.Browser().Notify(browserapi.LevelInfo, "nothing to copy")
		return nil
	}
	err := e.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: data})
	if err != nil {
		_, _ = e.Browser().Notify(browserapi.LevelError,
			"failed to copy to clipboard: %v", err)
		err = fmt.Errorf("clipboard copy: %w", err)
		return err
	}
	_, _ = e.Browser().Notify(browserapi.LevelSuccess, "copied to clipboard")
	return nil
}

func (e *ex) defaultcolors(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument 'background'")
	}

	var attrs term.Attributes
	attrs.Bg = tcell.GetColor(args[0])

	if len(args) == 2 {
		attrs.Fg = tcell.GetColor(args[1])
	}

	content, _ := e.invokeWindow().Content()
	emh, ok := content.(vtereservoir.VTE)
	if ok {
		emh.SetDefaultAttributes(attrs)
		return nil
	}
	t, ok := content.(*browser.Tab)
	if !ok {
		return errors.New("cannot change colors of this window")
	}
	th, ok := t.Handler().(text.Handler)
	if ok {
		th.SetDefaultAttributes(attrs)
		return nil
	}
	emh, ok = t.Handler().(vtereservoir.VTE)
	if ok {
		emh.SetDefaultAttributes(attrs)
		return nil
	}
	return errors.New("cannot change colors of this window")
}

func (e *ex) readfile(_ context.Context, args ...string) error {
	if len(args) != 1 {
		return errors.New("expected one file name")
	}

	// get desired uri to read
	filename := args[0]
	uri, err := e.parseURIOrWorkspaceURI(filename)
	if err != nil {
		return err
	}

	// get current editor handler to be used for content insertion
	_, handler, ok := e.handlerInFocus()
	if !ok {
		return errors.New("read file cannot get handler in focus")
	}

	// insert the file contents into the current cursor position
	err = e.comp.ReadFile(uri, handler)
	if err != nil {
		return err
	}

	return err
}

func (e *ex) executePlugin(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return e.toggleCompanionTerminal()
	}
	// Args reach this handler already unquoted by the command prompt.
	// The plugin handler stitches them back together into a command line
	// that the VTE will re-tokenise via mvdan.cc/sh — so individual args
	// containing whitespace or shell metacharacters must be re-quoted to
	// survive that round trip as a single argument.
	h, err := e.newPluginHandler(reshellQuoteArgs(args)...)
	if err != nil {
		return err
	}
	cfg := browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	}
	// there can be multiple floating windows open
	// so instead of matching windows on tabclose,
	// we set a handler that closes the window if the handler
	// is closed.
	ph := &pluginAdapter{pluginHandler: h}
	// mimic same behavior as tab
	ph.OnFocusChange(false)
	win, err := e.comp.Floating(ph, cfg)
	if err != nil {
		_ = h.Close()
		return err
	}
	ph.win = win
	return nil
}

func (e *ex) executePluginWait(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("expected at least one argument")
	}
	ch := make(chan error)
	watcher := workspaceapi.ChanProcessWatcher(ch)
	start := time.Now()
	e.log(log.DebugLevel, "starting command %v", args)
	cfg := e.emulatorConfig
	cfg.Watcher = watcher
	// See executePlugin: re-quote so the VTE's shell.Fields call
	// preserves argument boundaries that contain whitespace.
	cfg.CommandAndArgs = reshellQuoteArgs(args)
	// use set executor so we can inject plugin vars
	v, err := vte.NewHandler(e.Browser(), e.Browser(),
		e.workspace, e.executor, e.tm, cfg)
	if err != nil {
		return err
	}
	h := vteAdapter{v}

	handleError := func(err error) error {
		if err == nil {
			_, _ = e.notifications.Notify(browserapi.LevelSuccess,
				fmt.Sprintf("%s: done in %s", args[0], time.Since(start).
					Truncate(time.Millisecond)))
			return err
		}

		// collect stdout/stderr from plugin handler
		const width, height = 50, 6
		var w term.StringWriter
		w.Init(width, height)
		h.Resize(width, height)
		h.Draw(&w)
		_ = w.Flush()
		err = fmt.Errorf("%v: %s", err, w.String())
		return err
	}

	// if command is part of an alias chain, then we want to wait for it
	// to finish so we can use the return value of this command to short-circuit
	// if there's an error.
	_, isAliasCtx := text.IsAliasContext(ctx)
	if isAliasCtx {
		defer h.Close() //nolint:errcheck
		select {
		case err = <-ch:
			err = handleError(err)
			return err
		case <-time.After(e.pluginWaitTimeout):
			return errors.New("command was taking too long and so it was canceled")
		}
	}

	go debug.CapturePanicReport(func
	//nolint:errcheck
	() {

		defer h.Close()
		err := <-ch
		err = handleError(err)
		if err != nil {
			_, _ = e.notifications.Notify(browserapi.LevelError,
				fmt.Sprintf("%s: %s", args[0], err))
		}

	})
	return nil
}

// reshellQuoteArgs re-applies POSIX-style shell quoting to args so that a
// downstream consumer that re-joins them with spaces and re-runs a shell
// tokenizer (mvdan.cc/sh's Fields) recovers the original argument
// boundaries even when individual values contain whitespace or shell
// metacharacters. Used by the plugin executor whose VTE pipeline does
// exactly that round-trip.
//
// Quoting uses double quotes (or no quotes when unnecessary) rather than
// single quotes so that variable expansion of values like
// "$RUNE_DATADIR/worktrees/foo" — which alias bodies routinely embed —
// is still performed by shell.Fields downstream. Single-quoting would
// suppress that expansion, leaving a literal "$RUNE_DATADIR" in argv
// (RUNE-AGENT/worktreenew regression).
func reshellQuoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = shellFieldsQuote(a)
	}
	return out
}

// shellFieldsQuote wraps s so that shell.Fields(s, os.Getenv) returns it
// as a single argument while preserving POSIX-style $VAR / ${VAR}
// expansion. Strings that do not contain Layer 1 / shell metacharacters
// are returned verbatim. Values that need quoting are wrapped in double
// quotes, escaping the few characters that are still special inside
// double-quoted regions: `\`, `"`, and backtick.
func shellFieldsQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !needsShellFieldsQuote(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '"' || c == '`' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	b.WriteByte('"')
	return b.String()
}

func needsShellFieldsQuote(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r',
			'\\', '"', '\'', '`',
			'|', '&', ';', '(', ')', '<', '>',
			'*', '?', '[', ']', '#', '~', '=':
			return true
		}
	}
	return false
}

func (e *ex) keydump(_ context.Context, _ ...string) error {
	h := browser.Keydump(e.clip, &e.comp)
	cfg := browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	}
	_, err := e.comp.Floating(h, cfg)
	return err
}

func (e *ex) newTask(_ context.Context, args ...string) error {
	const errExpect = "command expects at least four arguments: " +
		"name, alignment, a separator '--' and the command to run"
	if len(args) < 4 {
		return errors.New(errExpect)
	}
	sysArgs, cmdAndArgs, found := strings.Cut(strings.Join(args, " "), "--")
	if !found {
		return errors.New(errExpect)
	}

	sysArgv := strings.Split(sysArgs, " ")
	cmdAndArgv := strings.Split(cmdAndArgs, " ")

	// cleanup splitting via --
	cmdAndArgv = cmdAndArgv[1:]
	sysArgv = sysArgv[:len(sysArgv)-1]

	if len(sysArgv) < 2 || len(cmdAndArgv) == 0 {
		return errors.New(errExpect)
	}

	e.log(log.DebugLevel, "newtask called with args %+v, sysArgs: %+v, cmdAndArgv: %+v",
		args, sysArgv, cmdAndArgv)

	t := idetask.Task{
		Name: sysArgv[0],
		Cmd:  cmdAndArgv[0],
		Args: cmdAndArgv[1:],
	}
	if len(sysArgv) > 2 {
		t.Filter = sysArgv[2]
	}
	switch sysArgv[1] {
	case "right":
		t.MinimizeAlignment = component.AlignmentRight
	case "left":
		t.MinimizeAlignment = component.AlignmentLeft
	default:
		return fmt.Errorf("invalid orientation argument %q", sysArgv[1])
	}

	err := e.tasks.RunTask(t)
	if errors.Is(err, idetask.ErrTaskExists) {
		return e.openReplaceTaskPrompt(t)
	}
	return err
}

func (e *ex) newTaskTab(ctx context.Context, args ...string) error {
	const errExpect = "command expects at least three arguments: " +
		"name, alignment, a separator '--' and the command to run"
	if len(args) < 3 {
		return errors.New(errExpect)
	}
	sysArgs, cmdAndArgs, found := strings.Cut(strings.Join(args, " "), "--")
	if !found {
		return errors.New(errExpect)
	}

	sysArgv := strings.Split(sysArgs, " ")
	cmdAndArgv := strings.Split(cmdAndArgs, " ")

	// cleanup splitting via --
	cmdAndArgv = cmdAndArgv[1:]
	sysArgv = sysArgv[:len(sysArgv)-1]

	if len(sysArgv) < 1 || len(cmdAndArgv) == 0 {
		return errors.New(errExpect)
	}

	e.log(log.DebugLevel, "newtasktab called with args %+v, sysArgs: %+v, cmdAndArgv: %+v",
		args, sysArgv, cmdAndArgv)

	t := idetask.Task{
		Name: sysArgv[0],
		Cmd:  cmdAndArgv[0],
		Args: cmdAndArgv[1:],
	}
	if len(sysArgv) > 1 {
		t.Filter = sysArgv[1]
	}

	err := e.tasks.RunTask(t)
	if errors.Is(err, idetask.ErrTaskExists) {
		return e.openReplaceTaskPrompt(t)
	}
	if err != nil {
		return err
	}

	ok := e.tasks.FocusTask(args[0])
	if !ok {
		return errors.New("could not convert task to tab")
	}
	return e.convertTab(ctx, args[0])
}

func (e *ex) stopTask(_ context.Context, args ...string) error {
	if len(args) != 1 {
		return errors.New("expected one argument with the name of the task to stop")
	}

	return e.tasks.StopTask(args[0])
}

func (e *ex) focusTask(_ context.Context, args ...string) error {
	if len(args) != 1 {
		return errors.New("expected one argument with the name of the task to focus")
	}
	if !e.tasks.FocusTask(args[0]) {
		return fmt.Errorf("task %q does not exist", args[0])
	}
	return nil
}

func (e *ex) completeTasks(
	ctx context.Context, cmd textapi.Command,
) (iterator.Iterator[string], string, error) {
	if len(cmd.Args) > 1 {
		return iterator.FromSlice[string](nil), "", nil
	}
	tasks := e.tasks.ListTasks()
	names := make([]string, len(tasks))
	for i, task := range tasks {
		names[i] = task.Name
	}
	// ensure that order is deterministic
	sort.Strings(names)

	return iterator.FromSlice(names), "", nil
}

func (e *ex) toggleCompanionTerminal() error {
	// if window is open and shell didn't exit, then close. If shell exited
	// then most likely what the user really wants is to open a new one
	// and the reason why the companion terminal is not nil is because
	// it wan't cleaned up properly.
	if e.companionTerminalWin != nil && !e.companionTerminal.IsComplete() {
		e.companionTerminalWin.Close()
		e.companionTerminalWin = nil
		return nil
	}

	width := int(float64(e.width) * 0.8)
	height := int(float64(e.height) * 0.8)

	if e.companionTerminal == nil || e.companionTerminal.IsComplete() {
		// if user exits via 'exit' command, then we must close the previous
		// terminal emulator and open a new one
		if e.companionTerminal != nil && e.companionTerminal.IsComplete() {
			_ = e.companionTerminal.Close()
		}
		var err error
		e.companionTerminal, err = e.newEmulatorHandler(nil)
		if err != nil {
			return err
		}
		e.companionTerminal.OnFocusChange(false)
	}
	cfg := browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	}
	cth := companionTerminalHandler{
		vth:      e.companionTerminal,
		Floating: browser.StaticFloating(e.companionTerminal, width, height),
	}
	win, err := e.comp.Floating(cth, cfg)
	if err != nil {
		return err
	}
	e.companionTerminalWin = win
	return nil
}

func (e *ex) initFileExplorer() error {
	prev, _ := e.comp.Focus()
	if !prev.Closed() {
		e.fileExplorerTarget = prev
	}

	if e.fileExplorerHandler == nil {
		rootURI, err := e.workspace.URI(".")
		if err != nil {
			return fmt.Errorf("file explorer: resolve root: %w", err)
		}
		buf := cell.NewBuffer()
		ignore, err := vctrl.LoadGitignore(e.workspace)
		if err != nil {
			e.log(log.WarnLevel,
				"file explorer: load gitignore: %v", err)
			ignore = vctrl.NopMatcher(false)
		}
		comp, err := fileexplorercomp.New(buf, e.workspace, rootURI, fileexplorercomp.Config{
			Icons:       e.config.Icons,
			IndentWidth: e.config.Tabspaces,
			IndentAttr:  e.config.FileExplorerIndentAttr,
			IconAttr:    e.config.FileExplorerIconAttr,
			Ignore:      ignore,
		})
		if err != nil {
			return fmt.Errorf("file explorer: create component: %w", err)
		}
		uri, err := workspaceapi.ParseURI(fileExplorerURI)
		if err != nil {
			return fmt.Errorf("file explorer: parse uri: %w", err)
		}
		if existing, ok := e.comp.Resource(uri); ok {
			if err := e.comp.RemoveTab(existing); err != nil {
				return fmt.Errorf("file explorer: remove stale tab: %w", err)
			}
		}
		ed, err := e.ed.Edit(context.Background(), uri, buf, false, false)
		if err != nil {
			return fmt.Errorf("file explorer: open editor: %w", err)
		}
		wrapped, err := newFileExplorerHandler(
			exFileExplorerHost{ex: e}, comp, buf, ed, uri, e.fileExplorerTarget,
		)
		if err != nil {
			return fmt.Errorf("file explorer: wrap component: %w", err)
		}
		e.fileExplorerHandler = wrapped
		if err := e.comp.SubscribeEvents(
			fileExplorerFSEvents, wrapped.fsEventHandler(),
		); err != nil {
			return fmt.Errorf(
				"file explorer: subscribe fs events: %w", err)
		}
	} else {
		// Keep the target in sync with the latest focus.
		e.fileExplorerHandler.SetTargetWindow(e.fileExplorerTarget)
	}

	win, err := e.comp.SplitRoot(component.AlignmentLeft, e.fileExplorerHandler)
	if err != nil {
		return fmt.Errorf("file explorer: create root split window: %w", err)
	}
	e.fileExplorerWin = win
	e.fileExplorerHandler.SetWindow(win)
	e.fileExplorerHandler.syncWidth()
	_, _ = e.comp.SetFocus(prev)
	return nil
}

func (e *ex) fexplorer(_ context.Context, args ...string) error {
	if e.fileExplorerWin == nil || e.fileExplorerWin.Closed() {
		e.fileExplorerWin = nil
		if err := e.initFileExplorer(); err != nil {
			return err
		}
		if e.fileExplorerWin == nil || e.fileExplorerWin.Closed() {
			return errors.New("file explorer is not available")
		}
		_, _ = e.comp.SetFocus(e.fileExplorerWin)
		return nil
	}
	focus, _ := e.comp.Focus()
	if focus != e.fileExplorerWin {
		if !focus.Closed() {
			e.fileExplorerTarget = focus
		}
		_, _ = e.comp.SetFocus(e.fileExplorerWin)
	} else {
		prev := e.fileExplorerTarget
		_ = e.fileExplorerWin.Close()
		e.fileExplorerWin = nil
		// Closing the explorer with unflushed buffer edits
		// implicitly discards them. If an FS event arrived
		// while the user was editing, replay it now so the
		// next open shows the up-to-date tree.
		if e.fileExplorerHandler != nil {
			e.fileExplorerHandler.onWindowClosed()
		}
		if prev != nil && !prev.Closed() {
			_, _ = e.comp.SetFocus(prev)
		}
	}
	return nil
}

func (e *ex) terminalnewtab(_ context.Context, args ...string) error {
	h, err := e.newEmulatorHandler(args)
	if err != nil {
		return err
	}

	uri := h.URI()
	t, err := e.comp.Tab(uri, e.config.Icons.Terminal, h.Title(), h)
	if err != nil {
		_ = h.Close()
		return fmt.Errorf("wm.Tab: %s", err)
	}

	tab := t.(*browser.Tab)
	tab.Subscribe((*tabSubscriber)(e))

	win := e.invokeWindow()
	if err := win.SetContent(tab); err != nil {
		_ = tab.Close()
		return err
	}
	return nil
}

func (e *ex) shellnewtab(_ context.Context, args ...string) error {
	if e.companionShell == nil {
		shellCfg := ideshell.Config{
			Storage:           e.storage,
			HistoryDocumentID: shellHistoryDocumentID,
			MaxHistory:        e.config.ShellMaxHistory,
		}
		if e.commandEditor != nil {
			shellCfg.EditModeKey = command.DefaultConfig().EditModeKey
			shellCfg.Editor = e.commandEditor
		}
		h, registry := ideshell.New(
			e.emulatorConfig.ScheduleNextTick, e,
			shellCfg,
		)
		if e.wsExecutor != nil {
			e.wsExecutor.RegisterProcessCommand(registry)
		}
		if e.extensionsExecutor != nil {
			e.extensionsExecutor.RegisterExtensionsProcessCommand(registry)
		}

		router := text.NewREPLHandler(&e.comp)
		for _, cmd := range e.comp.REPLCommands() {
			if err := registry.RegisterREPLCommand(cmd, router); err != nil {
				_ = h.Close()
				return fmt.Errorf("register repl command %q: %w", cmd.Name, err)
			}
		}

		workspaceURI, err := e.workspace.URI(".")
		if err != nil {
			_ = h.Close()
			return fmt.Errorf("workspace uri: %w", err)
		}

		uri, err := workspaceapi.ParseURI("shell:///")
		if err == nil {
			uri, err = workspaceapi.WithPath(uri, workspaceURI.Path())
			if workspaceURI.Host() != "" {
				uri, err = workspaceapi.ParseURI(
					fmt.Sprintf("shell://%s%s", workspaceURI.Host(), workspaceURI.Path()),
				)
			}
		}
		if err != nil {
			_ = h.Close()
			return fmt.Errorf("parse shell uri: %w", err)
		}
		e.companionShell = h
		e.companionShellURI = uri
	}

	t, err := e.comp.Tab(e.companionShellURI, e.config.Icons.Shell, "shell", e.companionShell)
	if err != nil {
		return fmt.Errorf("wm.Tab: %s", err)
	}

	tab := t.(*browser.Tab)

	win := e.invokeWindow()
	if err := win.SetContent(tab); err != nil {
		content, cerr := win.Content()
		if cerr == nil {
			if curr, ok := content.(*browser.Tab); ok && curr == tab {
				if line := strings.TrimSpace(strings.Join(args, " ")); line != "" {
					e.companionShell.Submit(line)
				}
				return nil
			}
		}
		return err
	}
	tab.Subscribe((*tabSubscriber)(e))
	if line := strings.TrimSpace(strings.Join(args, " ")); line != "" {
		e.companionShell.Submit(line)
	}
	return nil
}

func (e *ex) terminalnew(_ context.Context, args ...string) error {
	h, err := e.newEmulatorHandler(args)
	if err != nil {
		return err
	}

	win := e.invokeWindow()
	if err := win.SetContent(h); err != nil {
		_ = h.Close()
		return err
	}
	return nil
}

func (e *ex) terminalneworsplit(_ context.Context, args ...string) error {
	t, err := e.newEmulatorHandler(args)
	if err != nil {
		return err
	}
	win := e.invokeWindow()
	content, _ := win.Content()
	_, vok := content.(vtereservoir.VTE)
	_, tok := content.(*browser.Tab)
	if !vok && !tok {
		err = win.SetContent(t)
	} else {
		_, err = e.comp.Split(browserapi.OrientationDefault, win, t)
	}
	if err != nil {
		_ = t.Close()
		return err
	}
	return nil
}

func (e *ex) panic(_ context.Context, args ...string) error {
	panic("this could be a panic")
}

func (e *ex) windownew(_ context.Context, args ...string) error {
	orientation := browserapi.OrientationDefault
	if len(args) != 0 {
		switch args[0] {
		case "right":
			orientation = browserapi.OrientationRight
		case "down":
			orientation = browserapi.OrientationBottom
		case "left":
			orientation = browserapi.OrientationLeft
		case "up":
			orientation = browserapi.OrientationTop
		default:
			return fmt.Errorf("invalid orientation argument %q", args[0])
		}
	}
	e.windownewHandler(nil, orientation)
	return nil
}

func (e *ex) echo(_ context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("expected one argument with the sequence of keys")
	}
	var keys []echoKey
	for _, arg := range args {
		sub, err := parseEchoKeys(arg)
		if err != nil {
			return fmt.Errorf("invalid syntax: %v", err)
		}
		keys = append(keys, sub...)
	}
	var err error
	keys, err = e.expandEchoRegisters(keys, nil)
	if err != nil {
		return err
	}
	ok := true
	for i := 0; i < len(keys); i++ {
		keyComb := keys[i]
		if keyComb.instructWait {
			ok = ok && e.publishEvent(term.Event{
				Type: term.EventInterrupt,
				UserFunc: func() {
					e.Wait()
				},
			})
			continue
		}
		if keyComb.instructPrompt {
			e.openCommandPrompt()
			continue
		}
		ok = ok && e.publishEvent(term.Event{
			Type: term.EventKey,
			Ch:   keyComb.Ch,
			Mod:  keyComb.Mod,
			Key:  keyComb.Key,
		})
	}
	if !ok {
		return errors.New("could not publish all events to the event loop")
	}
	return nil
}

func (e *ex) expandEchoRegisters(keys []echoKey, stack []string) ([]echoKey, error) {
	ret := make([]echoKey, 0, len(keys))
	for _, key := range keys {
		if key.instructReg == "" {
			ret = append(ret, key)
			continue
		}
		if e.macro != nil && e.macro.IsRecording() && e.macro.RegisterID() == key.instructReg {
			return nil, fmt.Errorf("cannot echo register %q while it is actively being recorded", key.instructReg)
		}
		for _, seen := range stack {
			if seen == key.instructReg {
				cycle := append(append([]string{}, stack...), key.instructReg)
				return nil, fmt.Errorf("recursive register expansion detected: %s", strings.Join(cycle, " -> "))
			}
		}
		data, err := e.clip.Paste(key.instructReg)
		if err != nil {
			return nil, fmt.Errorf("expand register %q: paste register %q: %w", key.instructReg, key.instructReg, err)
		}
		regKeys, err := parseEchoKeys(data.Text)
		if err != nil {
			return nil, fmt.Errorf("expand register %q: parse register %q: %w", key.instructReg, key.instructReg, err)
		}
		expanded, err := e.expandEchoRegisters(regKeys, append(stack, key.instructReg))
		if err != nil {
			return nil, fmt.Errorf("expand register %q: %w", key.instructReg, err)
		}
		ret = append(ret, expanded...)
	}
	return ret, nil
}

func (e *ex) runCommand(cmd string, args []string) (quit bool, err error) {
	if len(args) == 0 {
		line, cerr := strconv.Atoi(cmd)
		if cerr == nil {
			err = e.moveFocusCursor(line - 1)
			return
		}
		return e.exit, e.dispatchCommand(cmd)
	}

	return e.exit, e.dispatchCommand(cmd, args...)
}

func (e *ex) setError(err error) {
	_, _ = e.notifications.Notify(browserapi.LevelError, fmt.Sprintf("%s", err))
}

func (e *ex) handleCommandEvent(ev term.Event) bool {
	if ev.KeyComb() == e.config.CommandEvent {
		e.openCommandPrompt()
		return true
	}
	return false
}

func (e *ex) onFocusChange(inFocus bool) {
	handler, _ := e.comp.Browser().Focus().Content()
	onFocusChangeHandler(handler, inFocus)
}

func (e *ex) handleEvent(ev term.Event) (
	exit, handled bool,
) {
	if ev.Type == term.EventMouse {
		_, handled = e.comp.Browser().Handle(ev)
		return
	}

	if inFocus := ev.Type == term.EventFocus; inFocus || ev.Type == term.EventUnfocus {
		e.log(log.DebugLevel, "focus event: inFocus: %t", inFocus)
		handler, _ := e.invokeWindow().Content()
		onFocusChangeHandler(handler, inFocus)
		handled = true
		return
	}

	if ev.Type != term.EventKey {
		e.log(log.DebugLevel, "delegating non-key event: %+v", ev)
		_, handled = e.comp.Browser().Handle(ev)
		return
	}

	// If ex is configured with non character
	// command mode trigger event, then this takes
	// precedence over any other event
	if e.config.CommandEvent.Key != 0 || (ev.Ch != 0 && ev.Mod != 0) {
		handled = e.handleCommandEvent(ev)
		if handled {
			return
		}
	}

	keyComb := ev.KeyComb()

	if e.cancelPartialReissue == nil {
		// allow handler to take precedence over key bindings (i.e. vi is in
		// insert mode and some key bindings shouldn't apply)
		// but only do it when the previous event didn't match (i.e. vi `ma`
		// should bind to creating bookmark 'a', so 'a' shouldn't be
		// delegated to handler, otherwise it'll handle it and switch to insert mode.
		_, handled = e.comp.Browser().Handle(ev)
		if handled {
			return
		}
	}

	var seq thandler.Sequence
	var match thandler.SequenceMatchResult
	// err nil indicates that match is still valid as timer hasn't expired
	// and it was not canceled yet or simply it hasn't even started and
	// this is first event in sequence.
	if e.ctxPartialReissue.Err() == nil {
		seq, match = e.sequencer.Sequence(keyComb)
	}

	var cmdsAndArgs [][]string
	var ok bool
	switch match {
	case thandler.SequenceMatch:
		cmdsAndArgs, ok = e.config.CommandSequenceBindings[seq]
		if !ok {
			panic("key sequencer matched but no command configured")
		}
		if e.cancelPartialReissue != nil {
			e.cancelPartialReissue()
			e.cleanPartialReissueState()
		}
	case thandler.SequencePartialMatch:
		if e.cancelPartialReissue == nil {
			// this is not a re-issue of a partial command, so set
			// timer to re-issue if user doesn't complete sequence,
			// and if timer expires
			ctx := context.Background()
			e.ctxPartialReissue, e.cancelPartialReissue = context.WithTimeout(ctx,
				e.config.SequencerTimeout+reissuePadding)
			e.reissueEvent = ev
			waitCtx := e.ctxPartialReissue
			reissueEvent := e.reissueEvent
			go debug.CapturePanicReport(func() {
				<-waitCtx.Done()
				if waitCtx.Err() == context.DeadlineExceeded {
					// timer expired, reissue event because
					// user didn't send a matching key combination.
					e.publishEvent(reissueEvent)
				}
			})
			return
		}
		// this is a re-issue so process normally
		_, handled = e.comp.Browser().Handle(ev)
		if handled {
			return
		}
	default:
		if e.cancelPartialReissue != nil {
			err := e.ctxPartialReissue.Err()
			e.cancelPartialReissue()
			e.cleanPartialReissueState()
			if err == nil {
				// issue previous event right before this next one
				// since we know now it's not a match.
				e.log(log.TraceLevel, "no sequence match: re-dispatching previous event %q",
					e.reissueEvent.KeyComb())
				_, _ = e.comp.Browser().Handle(e.reissueEvent)
			}
			_, handled = e.comp.Browser().Handle(ev)
			if handled {
				return
			}
		}
		cmdsAndArgs, _ = e.comp.CommandKeyBinding(keyComb)
	}

	// if match is a single "" command, then this is effectively unsetting
	// a key binding.
	if len(cmdsAndArgs) == 1 && len(cmdsAndArgs[0]) == 1 && cmdsAndArgs[0][0] == "" {
		cmdsAndArgs = nil
	}

	// dispatch command or sequence of commands
	for _, cmdAndArgs := range cmdsAndArgs {
		quit, err := e.runCommand(cmdAndArgs[0], cmdAndArgs[1:])
		if err != nil {
			e.setError(err)
		}
		handled = true
		if quit {
			return quit, handled
		}
	}

	// If ex is configured with character
	// command mode trigger event (i.e. ':')
	// then we assume that the underlying editor is
	// a modal editor, and so does not handle
	// the command trigger event in its "initial" mode
	// (in vi terms, this would be normal mode).
	handled = e.handleCommandEvent(ev)
	if handled {
		return
	}
	return
}

func (e *ex) resetCommandList(cmd *command.Prompt) {
	// commands can be registered dynamicall via Editor.Register:
	// compile a new list every time we switch to command mode
	commands := e.comp.Commands()
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	cmd.Reset(commands)
}

func (e *ex) openCommandPrompt() {
	e.newCommandPrompt(func(cmd *command.Prompt) {
		e.resetCommandList(cmd)
	})
}

func (e *ex) openCommandHistoryPrompt(_ context.Context, _ ...string) error {
	e.newCommandPrompt(func(cmd *command.Prompt) {
		cmd.ResetHistory()
	})
	return nil
}

func (e *ex) newCommandPrompt(reset func(*command.Prompt)) {
	commandCfg := command.DefaultConfig()
	commandCfg.NoMarkdown = false
	commandCfg.MaxHistory = e.config.CommandMaxHistory
	commandCfg.HistoryCycleKey = e.config.CommandEvent
	commandCfg.HistoryToggleKey = e.config.CommandHistoryKey
	commandCfg.MatchedTextAttr = e.config.CommandOverlay.MatchedTextAttr
	commandCfg.FocusElementAttr = e.config.CommandOverlay.FocusElementAttr
	commandCfg.ElementAttr = e.config.CommandOverlay.ElementAttr
	commandCfg.ManualAttr = e.config.CommandOverlay.ManualAttr
	commandCfg.DocumentID = commandHistoryDocumentID
	commandCfg.FrameCharSet = e.config.FrameCharSet
	commandCfg.FrameAttr = e.config.FrameAttr
	commandCfg.ShowManualAfter = e.config.CommandOverlay.ShowManualAfter
	commandCfg.ShowProgressHint = e.config.CommandOverlay.ShowProgressHint
	commandCfg.Sync = e.syncCommandPrompt
	if e.commandEditor != nil {
		commandCfg.Editor = e.commandEditor
	} else {
		commandCfg.Editor = commandPromptEditor{ed: e.ed}
	}
	cmd := command.NewPrompt(e.storage, e, e, e, []command.Manual{}, commandCfg)

	commandHandler := browser.FuncFloating(
		browser.FuncHandler(
			handler.WithComponent(cmd,
				component.WithBackground(
					cmd, term.Cell{
						Attributes: term.Attributes{
							Bg:    e.config.CommandOverlay.ElementAttr.Bg,
							Attrs: e.config.CommandOverlay.ElementAttr.Attrs,
						},
					},
				),
			), func() error {
				err := cmd.Close()
				if cmd == e.cmd {
					e.cmd = nil
				}
				return err
			}),
		cmd.Dimensions,
	)
	reset(cmd)

	e.cmdWin = e.cmdV.C.Floating(commandHandler,
		browserapi.FloatingConfig{
			Offset:    term.Coordinates{Y: int(float64(e.height) * 0.2)},
			Alignment: component.AlignmentHorizontallyCentered,
		})
	e.cmd = cmd
}

func (e *ex) handlePrompt(ev term.Event) (exit, handled bool) {
	// special handling of paste on prompt via key binding
	// which is the only keybinding that we want to enable while
	// prompt is active.
	if ev.Type != term.EventKey {
		_, handled = e.cmdV.Handle(ev)
		return
	}

	cmdsAndArgs, ok := e.comp.CommandKeyBinding(ev.KeyComb())
	if ok && len(cmdsAndArgs[0]) == 1 && cmdsAndArgs[0][0] == cmdClipboardPaste {
		err := e.pasteFromClipboard(context.Background())
		if err != nil {
			_, _ = e.Browser().Notify(browserapi.LevelError, "%v", err)
		}
		return
	}

	_, handled = e.cmdV.Handle(ev)
	return
}

// Handle satisfies tui.Handler.
func (e *ex) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = e.container.Handle(ev)
	if handled {
		return
	}
	if e.cmd != nil {
		_, handled = e.handlePrompt(ev)
	} else {
		_, handled = e.handleEvent(ev)
		currFocus, _ := e.comp.Focus()
		if e.fullscreenID != 0 && currFocus.WindowID() != e.fullscreenID {
			_ = e.windowtogglemaximize(context.Background())
		}
	}
	return e.exit, handled
}

// Cursor satisfies tui.Handler.
func (e *ex) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	return e.focusHandler().Cursor()
}

// Selection satisfies tui.Handler.
func (e *ex) Selection() (string, bool) {
	return e.focusHandler().Selection()
}

// Resize satisfies tui.Component
func (e *ex) Resize(width, height int) {
	e.height = height
	e.width = width
	e.tasks.SetMaxWidthHeight(width, height)
	if e.reservoir != nil {
		e.reservoir.Resize(width, height)
	}
	e.container.Resize(width, height)
	// if a top bar is added we don't reposition
	// command window until the next resize, but that's
	// acceptable because bars are added once
	offset := e.comp.WindowManagerPosition()
	e.cmdV.Move(offset)

	// set correct width and height for dynamically resized
	// components
	width = max(0, width-offset.X)
	height = max(0, height-offset.Y)
	e.cmdV.Resize(width, height)
}

// Draw satisfies tui.Component
func (e *ex) Draw(w term.Writer) {
	if e.cmd != nil {
		// temporarily disable auto-dimming based on focus so we
		// can pass a DimWriter below and dim everything.
		prev := e.comp.SetDim(false)
		defer e.comp.SetDim(prev)

		if e.config.Config.Dim {
			e.container.Draw(tterm.DimWriter(w))
		} else {
			e.container.Draw(w)
		}
		vw := component.VirtualWriter{
			Writer: w,
			Offset: e.cmdV.Position(),
			Height: e.cmdV.Height(),
			Width:  e.cmdV.Width(),
		}
		e.cmdV.C.DrawWindow(e.cmdWin, &vw)
	} else {
		e.container.Draw(w)
	}
}

// Editor returns the underlying Editor implementation.
func (e *ex) Editor() text.Editor {
	return &e.comp
}

// commandPromptEditor adapts a text.Editor to the command.Editor
// interface expected by command.Prompt for its modal edit mode.
// It calls text.Editor.Edit with a bare context (no
// withAuxiliaryBars), so the editor returns just the buffer view
// with no status / icons / aux bar chrome. That keeps the visible
// cursor coordinates aligned with the prompt's own coordinates.
// Production flows replace this with a bare vi.New /
// modeless.NewHandler adapter (see workspace_handler.go).
type commandPromptEditor struct {
	ed text.Editor
}

func (c commandPromptEditor) Edit(buf *cell.Buffer) command.EditHandler {
	uri := workspaceapi.RandomURI("memory")
	h, err := c.ed.Edit(context.Background(), uri, buf, false, false)
	if err != nil {
		// text.Editor implementations used here are in-process and
		// do not return errors for in-memory buffers.
		panic(fmt.Errorf("command prompt editor: %w", err))
	}
	h.SetWrap(true)
	h.ShowCommandBar(false)
	return h
}

// Browser returns the underlying browser.Browser implementaiton.
func (e *ex) Browser() browser.Browser {
	return &e.comp
}

// Close closes the resources associated with this browser.
func (e *ex) Close() (ret error) {
	if e.closed {
		return nil
	}
	e.closed = true
	e.sequencer.Reset()
	if err := e.saveWorkspaceLayout(context.Background()); err != nil {
		ret = multierror.Append(ret, err)
	}
	if err := e.saveOpenTaskSessions(context.Background()); err != nil {
		ret = multierror.Append(ret, err)
	}
	if err := e.saveOpenTerminalSessions(context.Background()); err != nil {
		ret = multierror.Append(ret, err)
	}
	if err := e.comp.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if e.reservoir != nil {
		if err := e.reservoir.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if e.cancelPartialReissue != nil {
		e.cancelPartialReissue()
		e.cleanPartialReissueState()
	}
	// we do not call Close on terminal when window is closed
	if e.companionTerminal != nil {
		_ = e.companionTerminal.Close()
		e.companionTerminal = nil
	}
	if e.companionShell != nil {
		_ = e.companionShell.Close()
		e.companionShell = nil
		e.companionShellURI = workspaceapi.URI{}
	}
	if e.fileExplorerHandler != nil {
		if err := e.fileExplorerHandler.closeEditor(); err != nil {
			ret = multierror.Append(ret, err)
		}
		e.fileExplorerHandler = nil
	}
	if e.cmd != nil {
		var err error
		if e.cmdWin != nil && !e.cmdWin.Closed() {
			err = e.cmdWin.Close()
		} else {
			err = e.cmd.Close()
		}
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if err := e.container.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	return ret
}

func (e *ex) cleanPartialReissueState() {
	e.cancelPartialReissue = nil
	e.ctxPartialReissue = context.Background()
}

func (e *ex) focusHandler() tui.Handler {
	if e.cmd != nil && !e.isPromptDispatch {
		return &e.cmdV
	}
	return e.comp.Browser()
}

type pluginAdapter struct {
	pluginHandler
	win browser.Window
}

type workspaceExecutorAdapter struct {
	e schemeapi.Executor
}

func (a workspaceExecutorAdapter) Start(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	return a.e.StartCommand(ctx, cmd)
}

func (a workspaceExecutorAdapter) Signal(
	pid workspaceapi.Pid, sig syscall.Signal,
) error {
	return a.e.Signal(pid, sig)
}

func (a workspaceExecutorAdapter) Close() error {
	return a.e.Close()
}

func (h *pluginAdapter) Close() error {
	if !h.win.Closed() {
		_ = h.win.Close()
	}

	// ephemeral handlers are closed when focus changes
	// in component. So adding the following line here
	// should handle all cases
	defer h.pluginHandler.OnFocusChange(false)

	return h.pluginHandler.Close()
}

type tabSubscriber ex

func (e *tabSubscriber) OnFocus(t *browser.Tab) {
	onFocusChangeTab(t, true)
}

func (e *tabSubscriber) OnFree(t *browser.Tab) {
	if e.companionShell != nil && t.URI().String() == e.companionShellURI.String() {
		e.companionShell = nil
		e.companionShellURI = workspaceapi.URI{}
	}
	onFocusChangeTab(t, false)
}

type windowSubscriber ex

func (e *windowSubscriber) OnFocus(prev, curr thandler.Window) {
	onFocusChange(prev, false)
	onFocusChange(curr, true)
}

func onFocusChange(win thandler.Window, isInFocus bool) {
	if win == (thandler.Window{}) {
		return
	}

	onFocusChangeHandler(win.Content(), isInFocus)
}

func onFocusChangeHandler(handler tui.Handler, isInFocus bool) {
	if t, ok := handler.(*browser.Tab); ok {
		onFocusChangeTab(t, isInFocus)
		return
	}

	// if it's not a tab, it can be either an internal browser type
	// (through handler.Window, which doesn't unwrap) or directly through browser
	// which doesn unwrap the internal browser handler.
	attempts := make([]tui.Handler, 1, 2)
	attempts[0] = handler

	internal, ok := handler.(interface{ Content() browserapi.Handler })
	if ok {
		attempts = append(attempts, internal.Content())
	}
	for _, content := range attempts {
		if t, ok := content.(*pluginAdapter); ok {
			t.pluginHandler.OnFocusChange(isInFocus)
			return
		}
		if t, ok := content.(companionTerminalHandler); ok {
			t.vth.OnFocusChange(isInFocus)
			return
		}
	}
}

func onFocusChangeTab(t *browser.Tab, isInFocus bool) {
	emulator, ok := t.Handler().(vtereservoir.VTE)
	if !ok {
		return
	}
	emulator.OnFocusChange(isInFocus)
}

var _ component.Scrollable = vteAdapter{}

// argsMatchEmulatorShell reports whether the requested
// terminal command is satisfied by the configured shell, in which
// case the warm reservoir can serve it. An empty argument list
// always falls back to the default shell so it matches.
func argsMatchEmulatorShell(cmdAndArgs, configured []string) bool {
	if len(cmdAndArgs) == 0 {
		return true
	}
	return slices.Equal(cmdAndArgs, configured)
}

// adapts vte.Handler to vtereservoir.VTE
type vteAdapter struct {
	*vte.Handler
}

func (v vteAdapter) SetDefaultAttributes(attr term.Attributes) {
	v.Handler.SetDefaultAttributes(attr)
}

func (v vteAdapter) IsComplete() bool {
	return v.Component().IsComplete()
}

func (v vteAdapter) URI() workspaceapi.URI {
	return v.Component().URI()
}

func (v vteAdapter) Title() string {
	return v.Component().Title()
}

func (v vteAdapter) UsedAlternateBuffer() bool {
	return v.Component().UsedAlternateBuffer()
}

func (v vteAdapter) ClearPrimaryBuffer() bool {
	return v.Component().ClearPrimaryBuffer()
}

var _ component.Scrollable = companionTerminalHandler{}
var _ vtereservoir.VTE = companionTerminalHandler{}

func (e *ex) fileExplorerHandlerInFocus() (*fileExplorerHandler, bool) {
	if e.fileExplorerWin == nil || e.fileExplorerWin.Closed() {
		return nil, false
	}
	focus, _ := e.comp.Focus()
	if focus != e.fileExplorerWin {
		return nil, false
	}
	content, err := focus.Content()
	if err != nil {
		return nil, false
	}
	h, ok := content.(*fileExplorerHandler)
	return h, ok
}

// Aids in ensure that Close is not called when window is closed:
// session should remain open as long as this workspace is not closed.
// Also ensures that we can identify companionTerminal on window focus
// change to deliver on focus calls to underlying vte.Handler
type companionTerminalHandler struct {
	browser.Floating
	vth vtereservoir.VTE
}

// SeekUp satisfies component.Scrollable.
func (c companionTerminalHandler) SeekUp() bool {
	return c.vth.SeekUp()
}

// SeekDown satisfies component.Scrollable.
func (c companionTerminalHandler) SeekDown() bool {
	return c.vth.SeekDown()
}

// SeekOffset satisfies component.Scrollable.
func (c companionTerminalHandler) SeekOffset() int {
	return c.vth.SeekOffset()
}

// MaxSeekOffset satisfies component.Scrollable.
func (c companionTerminalHandler) MaxSeekOffset() int {
	return c.vth.MaxSeekOffset()
}

func (c companionTerminalHandler) OnFocusChange(inFocus bool) {
	c.vth.OnFocusChange(inFocus)
}

func (c companionTerminalHandler) SetDefaultAttributes(attr term.Attributes) {
	c.vth.SetDefaultAttributes(attr)
}

func (c companionTerminalHandler) Snapshot() (vte.Snapshot, error) {
	return c.vth.Snapshot()
}

func (c companionTerminalHandler) RestoreFromSnapshot(snapshot vte.Snapshot) error {
	return c.vth.RestoreFromSnapshot(snapshot)
}

func (c companionTerminalHandler) IsComplete() bool {
	return c.vth.IsComplete()
}

func (c companionTerminalHandler) URI() workspaceapi.URI {
	return c.vth.URI()
}

func (c companionTerminalHandler) Title() string {
	return c.vth.Title()
}

func (c companionTerminalHandler) UsedAlternateBuffer() bool {
	return c.vth.UsedAlternateBuffer()
}

func (c companionTerminalHandler) ClearPrimaryBuffer() bool {
	return c.vth.ClearPrimaryBuffer()
}

func (c companionTerminalHandler) Close() error {
	return nil
}
