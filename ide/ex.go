package ide

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ernestrc/blue/document"
	"github.com/ernestrc/blue/iterator"
	"github.com/ernestrc/blue/logging"
	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	schemeapi "unstable.build/go-tui/api/scheme"
	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/plugin"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/emulator"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
)

const (
	commandHistoryDocumentID  = "ex-command-history"
	reissuePadding            = 10 * time.Millisecond
	cmdEdit                   = "edit"
	cmdChangeSplitOrientation = "changeSplitOrientation"
	cmdSplitWindowTerminal    = "splitWindowTerminal"
	cmdTerminalTab            = "newTerminal"
	cmdSplitWindow            = "splitWindow"
	cmdNewWindow              = "newWindow"
)

var (
	commandBarAttr      = term.Attributes{Bg: term.ColorWhite, Fg: term.ColorBlack}
	errInvalidSetCursor = errors.New("Cannot set cursor on this buffer")
	// TODO remove all default bindings
	exDefaultBindings = map[term.KeyComb]string{
		{Key: term.KeyCtrlW}: "tabClose",
		{Key: term.KeyCtrlL}: "tabNext",
		{Key: term.KeyCtrlH}: "tabPrev",
	}
	errEventStreamNotReady = errors.New("event stream not ready to publish")
)

type workspaceLoader interface {
	workspace.Loader
	workspace.Directory
	schemeapi.Terminal
	schemeapi.Executor
}

// ex implements a tui.Handler by wrapping an editor.Component and
// providing an ex editor type of interface.
type ex struct {
	config               text.Config
	comp                 text.Component
	ed                   text.Editor
	storage              document.Service
	emulatorConfig       emulator.Config
	workspace            workspaceLoader
	sequencer            handler.Sequencer
	publishEvent         func(term.Event) bool
	cancelPartialReissue func()
	ctxPartialReissue    context.Context
	reissueEvent         term.Event
	cmd                  *command.Prompt
	// use floating windows functionality without having to work around focus commands
	// and how to se cmd.Window correctly.
	cmdBrowser browser.Component
	cmdV       handler.Virtual
	cmdWin     browser.Window
	fullscreen browserapi.Handler
	exit       bool
	forceExit  bool
	height     int
	width      int

	companionTerminal    *emulator.Handler
	companionTerminalWin browser.Window
}

func newEx(
	ed text.Editor, m workspaceLoader,
	storage document.Service,
	emulatorConfig emulator.Config,
	publishEvent func(term.Event) bool,
	opts ...text.Option,
) (e *ex, err error) {
	e = new(ex)
	err = e.init(ed, m, storage, emulatorConfig, publishEvent, opts...)
	if err != nil {
		return
	}
	return
}

// init initializes this ex with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (e *ex) init(
	ed text.Editor, m workspaceLoader,
	storage document.Service,
	emulatorConfig emulator.Config,
	publishEvent func(term.Event) bool,
	opts ...text.Option,
) (err error) {
	err = e.doInit(ed, m, storage, emulatorConfig, publishEvent, opts...)
	if err != nil {
		return
	}
	err = e.comp.Init(ed, m, e.config)
	if err != nil {
		return
	}
	return
}

func (e *ex) subscribeCommands() error {
	var ret error
	for name, man := range exCommands {
		name := name
		man := man
		// Name is only defined as a key to exCommands
		man.man.Name = name
		err := e.comp.SubscribeCommand(man.man, text.FuncCommandCompleter(
			func(ctx context.Context, cmd textapi.Command) (bool, error) {
				return false, man.handler(e, cmd.Args...)
			}, func(ctx context.Context, args []string) (
				iterator.Iterator[string], string, error,
			) {
				return e.completeCommand(ctx, name, args)
			}))
		if err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	return ret
}

func (e *ex) completeEdit(
	ctx context.Context, args []string,
) (iterator.Iterator[string], string, error) {
	if len(args) == 0 || args[len(args)-1] == "" {
		it, err := workspace.ListFiles(ctx, e.workspace, ".")
		if err != nil {
			return nil, "", err
		}
		return it, "", nil
	}

	var modifiedLast string
	last := args[len(args)-1]

	// take ~ as the home of the user using the editor.
	// rather than the home directory of the user at the workspace.
	// do not always expand without making sure that we are not
	// erasing trailing /, which prevents user from editing files
	// in folders.
	var err error
	if strings.Contains(last, "~") {
		last, err = workspaceapi.ExpandPath(last, user.Current,
			func() (string, error) {
				// do not really expand to cwd,
				// let parseURIOrWorkspaceURI take care of that
				return ".", nil
			})
		if err != nil {
			return nil, "", fmt.Errorf("expand path: %v", err)
		}
		modifiedLast = last
	}

	uri, err := e.parseURIOrWorkspaceURI(last)
	if err != nil {
		return nil, "", err
	}

	it, err := workspace.ListFiles(ctx, e.workspace, uri.Path())
	if err != nil {
		return nil, "", err
	}
	return it, modifiedLast, nil
}

func (e *ex) log(level log.Level, msg string, args ...interface{}) {
	log.WithField(logging.KeyClass, "ex").Logf(level, msg, args...)
}

func (e *ex) completeCommand(
	ctx context.Context, cmd string, args []string,
) (iterator.Iterator[string], string, error) {
	e.log(log.DebugLevel, "complete command: %s %v", cmd, args)
	switch cmd {
	case cmdEdit:
		return e.completeEdit(ctx, args)
	case cmdSplitWindow, cmdNewWindow, cmdSplitWindowTerminal:
		return iterator.FromSlice([]string{"right", "left", "top", "bottom"}), "", nil
	case cmdChangeSplitOrientation:
		return iterator.FromSlice([]string{"horizontal", "vertical"}), "", nil
	default:
		return iterator.FromSlice[string](nil), "", nil
	}
}

func (e *ex) Interrupt() error {
	if !e.publishEvent(term.Event{Type: term.EventInterrupt}) {
		return errEventStreamNotReady
	}
	return nil
}

func (e *ex) doInit(
	ed text.Editor, m workspaceLoader,
	storage document.Service,
	emulatorConfig emulator.Config,
	publishEvent func(term.Event) bool,
	opts ...text.Option,
) (err error) {
	e.workspace = m
	e.publishEvent = publishEvent
	e.storage = storage
	e.emulatorConfig = emulatorConfig

	e.config = text.DefaultConfig()

	for ev, cmd := range exDefaultBindings {
		opts = append(opts, text.WithCommandKeyBinding(ev, []string{cmd}))
	}

	for _, o := range opts {
		o(&e.config)
	}

	seqInterests := make([]handler.Sequence, 0,
		len(e.config.CommandSequenceBindings))
	for seq := range e.config.CommandSequenceBindings {
		seqInterests = append(seqInterests, seq)
	}
	e.sequencer.Init(seqInterests, e.config.SequencerTimeout)

	e.ed = ed
	e.cleanPartialReissueState()
	e.cmdBrowser.Init(e.config.Config)
	e.cmdV.C = &e.cmdBrowser
	return
}

func (e *ex) Wait() {
	if e.cmd != nil {
		e.cmd.Wait()
	}
}

// Complete satisfies command.Completer for command.Handler.
func (e *ex) Complete(ctx context.Context, cmd string, args ...string) (
	iterator.Iterator[string], string,
) {
	it, newArg, err := e.comp.CompleteCommand(ctx, cmd, args...)
	if err != nil {
		e.setError(fmt.Errorf("complete command: %v", err))
		return iterator.FromSlice[string](nil), ""
	}
	return it, newArg
}

// Dispatch satisfies command.Dispatcher for command.Handler.
func (e *ex) Dispatch(command string, args ...string) bool {
	quit, err := e.runCommand(command, args)
	if err != nil {
		e.setError(err)
	}
	return quit
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
	return e.comp.SetCursor(h, term.Coordinates{Y: line})
}

func (e *ex) previousBuffer(args ...string) error {
	b := e.comp.Browser()
	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.PreviousTab(e.invokeWindow())
	return nil
}

func (e *ex) nextBuffer(args ...string) error {
	b := e.comp.Browser()
	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.NextTab(e.invokeWindow())
	return nil
}

func (e *ex) closeBuffer(args ...string) error {
	b := e.comp.Browser()
	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.RemoveWindowContent(win)
	return nil
}

func (e *ex) closeAllBuffers(args ...string) error {
	b := e.comp.Browser()
	b.RemoveAllTabs()
	return nil
}

func (e *ex) closeFocusWindow(args ...string) error {
	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	return win.Close()
}

func (e *ex) flushCloseIgnoreNonFlushed(args ...string) error {
	e.forceExit = true
	e.exit = true
	return e.comp.Flush(e.invokeWindow())
}

func (e *ex) flushClose(args ...string) error {
	e.forceExit = false
	e.exit = true
	return e.comp.Flush(e.invokeWindow())
}

func (e *ex) forceFlush(args ...string) error {
	return e.comp.Flush(e.invokeWindow())
}

func (e *ex) forceQuit(args ...string) error {
	e.forceExit = true
	e.exit = true
	return nil
}
func (e *ex) quit(args ...string) error {
	e.forceExit = false
	e.exit = true
	return nil
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
		scmd.Cursor.Content, _ = e.ed.Cursor(h)
		scmd.Cursor.Window, _, _ = h.Cursor()
	}
	var handled bool
	handled, err = e.comp.DispatchCommand(scmd)
	if err != nil {
		return
	}
	if !handled {
		target, ok := e.config.CommandAliases[cmd]
		if !ok && e.workspace == nil {
			err = fmt.Errorf("Unknown command or alias %q or cannot run on an empty workspace", cmd)
		} else if !ok {
			err = fmt.Errorf("Unknown command or command alias %q", cmd)
		} else if e.workspace == nil {
			err = fmt.Errorf("Cannot run %q (alias of %v) on an empty workspace",
				cmd, target)
		} else {
			err = fmt.Errorf("%s is aliased to an unknown command %v", cmd, target)
		}
	}
	return
}

func (e *ex) editFileURI(uri workspaceapi.URI, win browser.Window) (*browser.Tab, error) {
	e.log(log.DebugLevel, "edit: %s", uri.String())

	h, err := e.comp.Open(uri)
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
	e.log(log.TraceLevel, "parse uri (%s): %s, %v", path, uri.String(), err)
	if err != nil {
		uri, err = e.workspace.URI(path)
		e.log(log.TraceLevel, "URI (%s): %s, %v", path, uri.String(), err)
	}
	return uri, err
}

func (e *ex) editFiles(args ...string) error {
	if len(args) == 0 {
		return errors.New("expected at least one file name")
	}

	for _, arg := range args {
		// attempt to parse URI otherwise expect local file path
		uri, err := e.parseURIOrWorkspaceURI(arg)
		if err != nil {
			return err
		}
		_, err = e.editFileURI(uri, e.invokeWindow())
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *ex) reloadFile(args ...string) error {
	b := e.comp.Browser()
	focus := e.invokeWindow()
	uri, _, ok := e.handlerInFocus()
	if !ok {
		return errors.New("not a file")
	}
	b.RemoveWindowContent(focus)
	_, err := e.editFileURI(uri, e.invokeWindow())
	return err
}

func (e *ex) splitDirectionChange(args ...string) error {
	if len(args) == 0 {
		return errors.New("expecting argument 'horizontal', 'h', 'vertical', 'v'")
	}

	b := e.comp.Browser()
	switch args[0] {
	case "horizontal", "h":
		b.SetDefaultSplit(browserapi.OrientationBottom)
		b.Notify(notifications.LevelInfo, "changed split direction to horizontal")
	case "vertical", "v":
		b.SetDefaultSplit(browserapi.OrientationRight)
		b.Notify(notifications.LevelInfo, "changed split direction to vertical")
	}
	return nil
}

func (e *ex) newWindowHandler(h browserapi.Handler, orientation browserapi.Orientation) {
	eb := e.comp.Browser()
	win := e.invokeWindow()
	eb.Split(orientation, win, h)
}

func (e *ex) onCloseCommandPrompt() error {
	err := e.cmd.Close()
	e.cmd = nil
	return err
}

func (e *ex) invokeWindow() browser.Window {
	ret, _ := e.comp.Focus()
	return ret
}

func (e *ex) focusNextWindow(args ...string) error {
	e.comp.Browser().FocusRight()
	return nil
}

func (e *ex) focusPrevWindow(args ...string) error {
	e.comp.Browser().FocusLeft()
	return nil
}

func (e *ex) focusAboveWindow(args ...string) error {
	e.comp.Browser().FocusUp()
	return nil
}

func (e *ex) focusBelowWindow(args ...string) error {
	e.comp.Browser().FocusDown()
	return nil
}

func (e *ex) closeNotifications(args ...string) error {
	e.comp.CloseNotifications()
	return nil
}

func (e *ex) toggleFullscreen(args ...string) error {
	if e.fullscreen != nil {
		e.fullscreen = nil
		e.comp.Resize(e.width, e.height)
		return nil
	}
	content, err := e.invokeWindow().Content()
	if err != nil {
		return fmt.Errorf("window get content: %v", err)
	}
	e.fullscreen = content
	e.fullscreen.Resize(e.width, e.height)
	return nil
}

func (e *ex) pauseNotifications(args ...string) error {
	e.comp.PauseNotifications()
	return nil
}

func (e *ex) resumeNotifications(args ...string) error {
	e.comp.ResumeNotifications()
	return nil
}

func (e *ex) executePlugin(args ...string) error {
	if len(args) == 0 {
		return e.toggleCompanionTerminal()
	}
	h, err := plugin.Handler(e.Browser(), e.Browser(), e.workspace, e.workspace,
		e.emulatorConfig, strings.Join(args, " "), e.width,
		e.config.Frame, e.config.FocusFrameCharSet, e.config.FocusFrameAttr)
	if err != nil {
		return err
	}
	cfg := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
	}
	// there can be multiple floating windows open
	// so instead of matching windows on tabClose,
	// we set a handler that closes the window if the handler
	// is closed.
	var win browser.Window
	win, err = e.comp.Floating(browser.FuncFloatingHandler(h, func() error {
		if !win.Closed() {
			_ = win.Close()
		}
		return h.Close()
	}), cfg)
	if err != nil {
		_ = h.Close()
		return err
	}
	return nil
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
		cfg := e.emulatorConfig
		cfg.WidthHint = width
		cfg.HeightHint = width
		e.companionTerminal, err = e.newEmulator("", cfg)
		if err != nil {
			return err
		}
	}
	cfg := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
	}
	// do not call Close on terminal when window is closed: session should remain
	// open as long as this workspace is not closed.
	floating := browser.StaticFloating(browser.NopHandler(e.companionTerminal), width, height)
	win, err := e.comp.Floating(floating, cfg)
	if err != nil {
		return err
	}
	e.companionTerminalWin = win
	return nil
}

func (e *ex) newEmulator(initialCmd string, cfg emulator.Config) (*emulator.Handler, error) {
	h, err := emulator.New(e.Browser(), e.Browser(),
		e.workspace, e.workspace, cfg, initialCmd)
	if err != nil {
		err = fmt.Errorf("new emulator: %s", err)
		return nil, err
	}
	return h, nil
}

func (e *ex) newEmulatorTab(initialCmd string) (*browser.Tab, error) {
	cfg := e.emulatorConfig
	h, err := e.newEmulator(initialCmd, cfg)
	if err != nil {
		return nil, err
	}

	uri, err := h.URI()
	if err != nil {
		_ = h.Close()
		return nil, err
	}
	t, err := e.comp.Tab(uri, h.Title(), h)
	if err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("wm.Tab: %s", err)
	}

	return t.(*browser.Tab), nil
}

func (e *ex) splitWindowTerminal(args ...string) error {
	orientation := browserapi.OrientationDefault
	if len(args) != 0 {
		switch args[0] {
		case "right":
			orientation = browserapi.OrientationRight
		case "bottom":
			orientation = browserapi.OrientationBottom
		case "left":
			orientation = browserapi.OrientationLeft
		case "top":
			orientation = browserapi.OrientationTop
		default:
			return fmt.Errorf("invalid orientation argument %q", args[0])
		}
	}
	t, err := e.newEmulatorTab("")
	if err != nil {
		return err
	}
	win := e.invokeWindow()
	if _, err := e.comp.Split(orientation, win, t); err != nil {
		_ = t.Close()
		return err
	}
	return nil
}

func (e *ex) newTerminalTab(args ...string) error {
	t, err := e.newEmulatorTab("")
	if err != nil {
		return err
	}
	win := e.invokeWindow()
	if err := win.SetContent(t); err != nil {
		_ = t.Close()
		return err
	}
	return nil
}

func (e *ex) panic(args ...string) error {
	panic("this could be a panic")
}

func (e *ex) newWindow(args ...string) error {
	orientation := browserapi.OrientationDefault
	if len(args) != 0 {
		switch args[0] {
		case "right":
			orientation = browserapi.OrientationRight
		case "bottom":
			orientation = browserapi.OrientationBottom
		case "left":
			orientation = browserapi.OrientationLeft
		case "top":
			orientation = browserapi.OrientationTop
		default:
			return fmt.Errorf("invalid orientation argument %q", args[0])
		}
	}
	e.newWindowHandler(nil, orientation)
	return nil
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
	e.comp.Browser().Notify(notifications.LevelError, "%s", err)
}

func (e *ex) handleCommandEvent(ev term.Event) bool {
	if ev.KeyComb() == e.config.CommandEvent {
		e.openCommandPrompt()
		return true
	}
	return false
}

func isWindowControlCommand(cmdAndArgs []string) bool {
	cmd := cmdAndArgs[0]
	_, ok := windowControlCommands[cmd]
	return ok
}

func (e *ex) handleEvent(ev term.Event) (
	exit, handled bool,
) {
	if ev.Type == term.EventMouse {
		if e.fullscreen != nil {
			_, handled = e.fullscreen.Handle(ev)
		} else {
			_, handled = e.comp.Browser().Handle(ev)
		}
		return
	}

	// If ex is configured with non character
	// command mode trigger event, then this takes
	// precedence over any other event
	if e.config.CommandEvent.Ch == 0 {
		handled = e.handleCommandEvent(ev)
		if handled {
			return
		}
	}

	var seq handler.Sequence
	var match handler.SequenceMatchResult
	// err nil indicates that match is still valid as timer hasn't expired
	// and it was not canceled yet or simply it hasn't even started and
	// this is first event in sequence.
	if e.ctxPartialReissue.Err() == nil {
		seq, match = e.sequencer.Sequence(ev.KeyComb())
	}

	var cmdAndArgs []string
	var ok bool
	switch match {
	case handler.SequenceMatch:
		cmdAndArgs, ok = e.config.CommandSequenceBindings[seq]
		if !ok {
			panic("key sequencer matched but no command configured")
		}
		if e.cancelPartialReissue != nil {
			e.cancelPartialReissue()
			e.cleanPartialReissueState()
		}
	case handler.SequencePartialMatch:
		if e.cancelPartialReissue == nil {
			// this is not a re-issue of a partial command, so set
			// timer to re-issue if user doesn't complete sequence,
			// and if timer expires
			ctx := context.Background()
			e.ctxPartialReissue, e.cancelPartialReissue = context.WithTimeout(ctx,
				e.config.SequencerTimeout+reissuePadding)
			e.reissueEvent = ev
			go func(ctx context.Context, ev term.Event) {
				<-ctx.Done()
				if ctx.Err() == context.DeadlineExceeded {
					// timer expired, reissue event because
					// user didn't send a matching key combination.
					e.publishEvent(ev)
				}
			}(e.ctxPartialReissue, e.reissueEvent)
			return
		}
		// this is a re-issue so continue processing
	default:
		if e.cancelPartialReissue != nil {
			err := e.ctxPartialReissue.Err()
			e.cancelPartialReissue()
			e.cleanPartialReissueState()
			if err == nil {
				// issue previous event right before this next one
				// since we know now it's not a match.
				_, _ = e.comp.Browser().Handle(e.reissueEvent)
			}
		}
		cmdAndArgs, _ = e.comp.KeyMapping(ev.KeyComb())
	}

	// first dispatch window control commands
	if len(cmdAndArgs) != 0 && isWindowControlCommand(cmdAndArgs) {
		// make sure that the command is applied to the right window
		// and it needs to be set here to differentiate between
		// runCommand being called from command prompt
		// or from a key mapping event
		quit, err := e.runCommand(cmdAndArgs[0], cmdAndArgs[1:])
		if err != nil {
			e.setError(err)
		}
		return quit, true
	}

	if match != handler.SequenceMatch {
		// then the focus handler takes precedence
		b := e.comp.Browser()
		_, handled = b.Handle(ev)
		if handled {
			return
		}
	}

	// finally dispatch user event command or sequence command
	if len(cmdAndArgs) != 0 {
		quit, err := e.runCommand(cmdAndArgs[0], cmdAndArgs[1:])
		if err != nil {
			e.setError(err)
		}
		return quit, true
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
	var commands []text.CommandManual
	for _, cmd := range e.comp.Commands() {
		commands = append(commands, cmd)
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	cmd.Reset(commands)
}

func (e *ex) openCommandPrompt() {
	commandCfg := command.DefaultConfig()
	commandCfg.MaxHistory = e.config.CommandMaxHistory
	commandCfg.HistoryKey = e.config.CommandEvent
	commandCfg.MatchedTextAttr = e.config.CommandOverlay.MatchedTextAttr
	commandCfg.FocusElementAttr = e.config.CommandOverlay.FocusElementAttr
	commandCfg.ElementAttr = e.config.CommandOverlay.ElementAttr
	commandCfg.DocumentID = commandHistoryDocumentID
	commandCfg.FrameCharSet = e.config.FrameCharSet
	commandCfg.FrameAttr = e.config.FrameAttr
	commandCfg.ShowManualAfter = e.config.CommandOverlay.ShowManualAfter
	cmd := command.NewPrompt(e.storage, e, e, e, []text.CommandManual{}, commandCfg)

	var commandHandler browser.Floating = cmd
	commandHandler = browser.FuncFloating(
		browser.FuncHandler(
			handler.WithComponent(cmd,
				component.WithBackground(
					cmd, term.Cell{Bg: e.config.CommandOverlay.ElementAttr.Bg},
				),
			), e.onCloseCommandPrompt),
		cmd.Dimensions,
	)
	e.resetCommandList(cmd)

	e.cmdWin = e.cmdBrowser.Floating(commandHandler,
		component.FloatingConfig{
			Offset:    term.Coordinates{Y: int(float64(e.height) * 0.2)},
			Alignment: component.SpanAlignmentHorizontallyCentered,
		})
	e.cmd = cmd
}

// Handle satisfies tui.Handler.
func (e *ex) Handle(ev term.Event) (exit, handled bool) {
	if e.cmd != nil {
		_, handled = e.cmdV.Handle(ev)
	} else {
		_, handled = e.handleEvent(ev)
	}
	return e.exit, handled
}

// Cursor satisfies tui.Handler.
func (e *ex) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	if e.cmd != nil {
		return e.cmdV.Cursor()
	}
	if e.fullscreen != nil {
		return e.fullscreen.Cursor()
	}
	return e.comp.Browser().Cursor()
}

// Man satisfies tui.Handler.
func (e *ex) Man() tui.Manual {
	panic("TODO")
}

// Resize satisfies tui.Component
func (e *ex) Resize(width, height int) {
	e.height = height
	e.width = width
	e.comp.Resize(width, height)
	if e.fullscreen != nil {
		e.fullscreen.Resize(width, height)
	}
	// if a top bar is added we don't reposition
	// command window until the next resize, but that's
	// acceptable because bars are added once
	offset := e.comp.WindowManagerPosition()
	e.cmdV.Move(offset)

	// set correct width and height for dynamically resized
	// components
	width -= offset.X
	height -= offset.Y
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
			if e.fullscreen != nil {
				e.fullscreen.Draw(term.DimWriter(w))
			} else {
				e.comp.Draw(term.DimWriter(w))
			}
		} else {
			if e.fullscreen != nil {
				e.fullscreen.Draw(w)
			} else {
				e.comp.Draw(w)
			}
		}
		w = component.VirtualWriter(w,
			e.cmdV.Position(), e.cmdV.Height(), e.cmdV.Width())
		e.cmdBrowser.DrawWindow(e.cmdWin, w)
	} else if e.fullscreen != nil {
		e.fullscreen.Draw(w)
	} else {
		e.comp.Draw(w)
	}
}

// Editor returns the underlying Editor implementation.
func (e *ex) Editor() text.Editor {
	return &e.comp
}

// Browser returns the underlying browser.Browser implementaiton.
func (e *ex) Browser() browser.Browser {
	return &e.comp
}

func (e *ex) cleanPartialReissueState() {
	e.cancelPartialReissue = nil
	e.ctxPartialReissue = context.Background()
}

// Close closes the resources associated with this browser.
func (e *ex) Close() (ret error) {
	e.sequencer.Reset()
	if err := e.comp.Close(); err != nil {
		ret = multierr.Append(ret, err)
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
	if e.cmd != nil {
		if err := e.cmd.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	return
}

type commandAll struct {
	man     textapi.CommandManual
	handler func(*ex, ...string) error
}
