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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/idetask"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/term/vte/vtereservoir"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
)

const (
	commandHistoryDocumentID = "ex-command-history"
	reissuePadding           = 10 * time.Millisecond
)

var (
	errInvalidSetCursor    = errors.New("cannot set cursor on this buffer")
	errEventStreamNotReady = errors.New("event stream not ready to publish")
	errInvalidTab          = errors.New("expected exactly one argument with the tab position")
)

type pluginHandler interface {
	browserapi.Floating
	OnFocusChange(bool)
}

// ex implements a tui.Handler by wrapping an editor.Component and
// providing an ex editor type of interface.
type ex struct {
	config               text.Config
	comp                 text.Component
	clip                 clipboard.Register
	ed                   text.Editor
	storage              document.Service
	reservoir            *vtereservoir.Facility
	notifications        notifier
	emulatorConfig       vte.Config
	newEmulatorHandler   func(string) (vtereservoir.VTE, error)
	newPluginHandler     func(...string) (pluginHandler, error)
	workspace            workspace.Workspace
	tasks                *idetask.Manager
	dispatchOnPreview    map[string]func() func()
	filepathCompleter    command.Completer
	sequencer            handler.Sequencer
	publishEvent         func(term.Event) bool
	cancelPartialReissue func()
	ctxPartialReissue    context.Context
	reissueEvent         term.Event
	cmd                  *command.Prompt
	syncCommandPrompt    bool
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

	companionTerminal    vtereservoir.VTE
	companionTerminalWin browser.Window
}

func newEx(
	ed text.Editor, m workspace.Workspace,
	storage document.Service,
	notifications notifier,
	emulatorConfig vte.Config,
	publishEvent func(term.Event) bool,
	initialVTECapacity int,
	clip clipboard.Register,
	dispatchOnPreview map[string]func() func(),
	opts ...text.Option,
) (e *ex, err error) {
	e = new(ex)
	err = e.init(ed, m, storage, notifications,
		emulatorConfig, publishEvent, initialVTECapacity, clip,
		dispatchOnPreview, opts...)
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
	storage document.Service,
	notifications notifier,
	emulatorConfig vte.Config,
	publishEvent func(term.Event) bool,
	initialVTECapacity int,
	clip clipboard.Register,
	dispatchOnPreview map[string]func() func(),
	opts ...text.Option,
) (err error) {
	err = e.doInit(ed, m, storage, notifications,
		emulatorConfig, publishEvent, clip, opts...)
	if err != nil {
		return
	}
	err = e.comp.Init(ed, m, e.config)
	if err != nil {
		return
	}
	e.comp.SubscribeWindow((*windowSubscriber)(e))
	if initialVTECapacity != 0 {
		e.reservoir = vtereservoir.New(e.Browser(), e.Browser(),
			e.workspace, e.workspace, e.Browser(), e.emulatorConfig, initialVTECapacity)
	}
	e.newEmulatorHandler = func(initialCmd string) (vtereservoir.VTE, error) {
		if initialCmd == "" && e.reservoir != nil {
			e.log(log.TraceLevel, "getting vte instance from reservoir")
			return e.reservoir.Get()
		}
		v, err := vte.NewHandler(e.Browser(), e.Browser(),
			e.workspace, e.workspace, e.Browser(), e.emulatorConfig, initialCmd)
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
		plugin.WithBarAttr(e.config.FocusFrameAttr),
	}
	e.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return plugin.New(e.Browser(), e.Browser(), e.workspace, e.workspace,
			e.Browser(), strings.Join(args, " "), e.width, pluginOpts...)
	}
	e.dispatchOnPreview = dispatchOnPreview
	e.filepathCompleter = command.FilePathCompleter(e.workspace)
	e.tasks = idetask.NewManager(e.Browser(), m, pluginOpts...)
	e.comp.SubscribeWindow(e.tasks)
	return
}

func (e *ex) subscribeCommands() error {
	var ret error
	for name, man := range exCommands {
		name := name
		man := man
		// Name is only defined as a key to exCommands
		man.man.Name = name
		err := e.comp.SubscribeCommand(man.man, text.FuncCommandHandler(
			func(ctx context.Context, cmd textapi.Command) error {
				return man.handler(e, cmd.Args...)
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

func (e *ex) completeReadFile(
	ctx context.Context, args []string,
) (iterator.Iterator[string], string, error) {
	// `:readfile` auto-completion works the same as the `:edit` command.
	return e.filepathCompleter.Complete(ctx, args)
}

func (e *ex) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "ide.ex").Logf(level, msg, args...)
}

func (e *ex) Interrupt(ctx context.Context) error {
	payload, _ := term.PayloadFromContext(ctx)
	if !e.publishEvent(term.Event{Type: term.EventInterrupt, Raw: payload}) {
		return errEventStreamNotReady
	}
	return nil
}

func (e *ex) doInit(
	ed text.Editor, m workspace.Workspace,
	storage document.Service,
	n notifier,
	emulatorConfig vte.Config,
	publishEvent func(term.Event) bool,
	clip clipboard.Register,
	opts ...text.Option,
) (err error) {
	e.clip = clip
	e.workspace = m
	e.notifications = n
	e.publishEvent = publishEvent
	e.storage = storage
	e.emulatorConfig = emulatorConfig

	e.config = text.DefaultConfig()

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
func (e *ex) Preview(command string, args ...string) (func(), bool) {
	if e.dispatchOnPreview == nil {
		return nil, false
	}
	cancel, ok := e.dispatchOnPreview[command]
	if !ok {
		return nil, false
	}
	cancelTrigger := cancel()
	e.Dispatch(command, args...)
	return cancelTrigger, true
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
	ok = h.SetCursorAtScroll(term.Coordinates{Y: line})
	if !ok {
		return errors.New("could not set cursor to position")
	}
	return nil
}

func (e *ex) tabrename(args ...string) error {
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

func (e *ex) tabprevious(args ...string) error {
	b := e.comp.Browser()
	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.PreviousTab(e.invokeWindow())
	return nil
}

func (e *ex) tabnext(args ...string) error {
	b := e.comp.Browser()
	if e.invokeWindow() == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	b.NextTab(e.invokeWindow())
	return nil
}

func (e *ex) tabfocus(args ...string) error {
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

func (e *ex) tabclose(args ...string) error {
	b := e.comp.Browser()
	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	// on focus dispatch to vte.Handler via Close
	b.RemoveWindowContent(win)
	return nil
}

func (e *ex) tabcloseall(args ...string) error {
	b := e.comp.Browser()
	// on focus dispatch to vte.Handler via Close
	b.RemoveAllTabs()
	return nil
}

func (e *ex) tabcloseinactive(args ...string) error {
	b := e.comp.Browser()
	if removed := b.RemoveInactiveTabs(); !removed {
		return errors.New("no inactive tabs left")

	}
	return nil
}

func (e *ex) closeFocusWindow(args ...string) error {
	win := e.invokeWindow()
	if win == e.companionTerminalWin {
		return e.toggleCompanionTerminal()
	}
	// on focus dispatch to vte.Handler via Close
	return win.Close()
}

func (e *ex) windowcloseall(args ...string) error {
	return e.comp.Browser().CloseOtherWindows(e.invokeWindow())
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

func (e *ex) flush(args ...string) error {
	return e.comp.Flush(e.invokeWindow())
}

func (e *ex) forceFlush(args ...string) error {
	return e.comp.ForceFlush(e.invokeWindow())
}

func (e *ex) flushAll(args ...string) (ret error) {
	for _, t := range e.comp.Tabs() {
		if err := e.comp.FlushTab(t); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (e *ex) forceFlushAll(args ...string) (ret error) {
	for _, t := range e.comp.Tabs() {
		if err := e.comp.ForceFlushTab(t); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (e *ex) forcequit(args ...string) error {
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
		scmd.Cursor.Content = h.CursorAtScroll()
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

func (e *ex) editFiles(args ...string) error {
	return e.editFilesReadOnly(false, args...)
}

func (e *ex) viewFiles(args ...string) error {
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

func (e *ex) tabcopypath(args ...string) error {
	absolute := len(args) > 0 && args[0] == "absolute"
	t, ok := e.comp.FocusTab()
	if !ok {
		return errors.New("not a tab")
	}
	if _, ok := t.Handler().(text.Handler); !ok {
		return errors.New("not a file")
	}

	uri := t.URI()
	var path string
	if absolute {
		path = uri.Path()
	} else {
		path = uri.Name()
	}

	err := e.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: path})
	if err != nil {
		return fmt.Errorf("clipboard copy: %v", err)
	}

	e.notifications.Notify(notifications.LevelSuccess,
		"file path copied to clipboard")

	return nil
}

func (e *ex) reloadfile(args ...string) error {
	focus := e.invokeWindow()
	return e.comp.Reload(focus)
}

func (e *ex) splitDirectionChange(args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects argument 'horizontal', 'h', 'vertical', 'v'")
	}

	b := e.comp.Browser()
	switch args[0] {
	case "horizontal", "h":
		b.SetDefaultSplit(browserapi.OrientationBottom)
		e.notifications.Notify(notifications.LevelInfo, "changed split direction to horizontal")
	case "vertical", "v":
		b.SetDefaultSplit(browserapi.OrientationRight)
		e.notifications.Notify(notifications.LevelInfo, "changed split direction to vertical")
	}
	return nil
}

func (e *ex) windownewHandler(h browserapi.Handler, orientation browserapi.Orientation) {
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

func (e *ex) windowfocus(args ...string) error {
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

func (e *ex) moveWindow(args ...string) error {
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

func (e *ex) moveTab(args ...string) error {
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

func (e *ex) windowresize(args ...string) error {
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

func (e *ex) sendNotificationInfo(args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	e.notifications.Notify(notifications.LevelInfo, strings.Join(args, " "))
	return nil
}

func (e *ex) sendNotificationSuccess(args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	e.notifications.Notify(notifications.LevelSuccess, strings.Join(args, " "))
	return nil
}

func (e *ex) sendNotificationWarning(args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	e.notifications.Notify(notifications.LevelWarn, strings.Join(args, " "))
	return nil
}

func (e *ex) sendNotificationError(args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}
	e.notifications.Notify(notifications.LevelError, strings.Join(args, " "))
	return nil
}

func (e *ex) windowtogglemaximize(args ...string) error {
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

func (e *ex) closeNotifications(args ...string) error {
	e.notifications.CloseAll()
	return nil
}

func (e *ex) pauseNotifications(args ...string) error {
	e.notifications.PauseAll()
	return nil
}

func (e *ex) resumeNotifications(args ...string) error {
	e.notifications.ResumeAll()
	return nil
}

func (e *ex) pasteFromClipboard(args ...string) error {
	handler := e.focusHandler()
	data, err := e.clip.Paste(clipboard.DefaultRegisterID)
	if err != nil {
		return fmt.Errorf("clipboard paste: %w", err)
	}
	if len(data.Text) == 0 {
		_, _ = e.Browser().Notify(notifications.LevelInfo, "nothing to paste")
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

func (e *ex) copyToClipboard(args ...string) error {
	handler := e.focusHandler()
	data, ok := handler.Selection()
	if !ok {
		_, _ = e.Browser().Notify(notifications.LevelInfo, "nothing to copy")
		return nil
	}
	err := e.clip.Copy(clipboard.DefaultRegisterID, clipboard.Data{Text: data})
	if err != nil {
		_, _ = e.Browser().Notify(notifications.LevelError,
			"failed to copy to clipboard: %v", err)
		err = fmt.Errorf("clipboard copy: %w", err)
		return err
	}
	_, _ = e.Browser().Notify(notifications.LevelSuccess, "copied to clipboard")
	return nil
}

func (e *ex) defaultcolors(args ...string) error {
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

func (e *ex) readfile(args ...string) error {
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

func (e *ex) executePlugin(args ...string) error {
	if len(args) == 0 {
		return e.toggleCompanionTerminal()
	}
	for i, arg := range args {
		if arg != "%" {
			continue
		}
		content, _ := e.invokeWindow().Content()
		th, ok := content.(*browser.Tab)
		if !ok {
			continue
		}
		args[i] = th.URI().Path()
	}
	h, err := e.newPluginHandler(args...)
	if err != nil {
		return err
	}
	cfg := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
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

func (e *ex) newTask(args ...string) error {
	const errExpect = "command expects at least four arguments: " +
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
		t.MinimizeAlignment = component.SpanAlignmentRight
	case "left":
		t.MinimizeAlignment = component.SpanAlignmentLeft
	default:
		return fmt.Errorf("invalid orientation argument %q", sysArgv[1])
	}

	err := e.tasks.RunTask(t)
	if errors.Is(err, idetask.ErrTaskExists) {
		return e.openReplaceTaskPrompt(t)
	}
	return err
}

func (e *ex) stopTask(args ...string) error {
	if len(args) != 1 {
		return errors.New("expected one argument with the name of the task to stop")
	}

	return e.tasks.StopTask(args[0])
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
		e.companionTerminal, err = e.newEmulatorHandler("")
		if err != nil {
			return err
		}
		e.companionTerminal.OnFocusChange(false)
	}
	cfg := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
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

func (e *ex) terminalnewtab(args ...string) error {
	var initialCmd string
	if len(args) > 0 {
		initialCmd = args[0]
	}
	h, err := e.newEmulatorHandler(initialCmd)
	if err != nil {
		return err
	}

	uri := h.URI()
	if err != nil {
		_ = h.Close()
		return err
	}
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

func (e *ex) terminalnew(args ...string) error {
	var initialCmd string
	if len(args) > 0 {
		initialCmd = args[0]
	}
	h, err := e.newEmulatorHandler(initialCmd)
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

func (e *ex) terminalneworsplit(args ...string) error {
	var initialCmd string
	if len(args) > 0 {
		initialCmd = args[0]
	}
	t, err := e.newEmulatorHandler(initialCmd)
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

func (e *ex) panic(args ...string) error {
	panic("this could be a panic")
}

func (e *ex) windownew(args ...string) error {
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

func (e *ex) echo(args ...string) error {
	sequence := strings.Join(args, " ")
	if sequence == "" {
		return errors.New("expected one argument with the sequence of keys")
	}
	keys, err := parseEchoKeys(sequence)
	if err != nil {
		return fmt.Errorf("invalid syntax: %v", err)
	}
	ok := true
	for _, keyComb := range keys {
		if keyComb.instructWait {
			ok = ok && e.publishEvent(term.Event{
				Type: term.EventInterrupt,
				UserFunc: func() {
					e.Wait()
				},
			})
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
	e.notifications.Notify(notifications.LevelError, fmt.Sprintf("%s", err))
}

func (e *ex) handleCommandEvent(ev term.Event) bool {
	if ev.KeyComb() == e.config.CommandEvent {
		e.openCommandPrompt()
		return true
	}
	return false
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

	var seq handler.Sequence
	var match handler.SequenceMatchResult
	// err nil indicates that match is still valid as timer hasn't expired
	// and it was not canceled yet or simply it hasn't even started and
	// this is first event in sequence.
	if e.ctxPartialReissue.Err() == nil {
		seq, match = e.sequencer.Sequence(keyComb)
	}

	var cmdsAndArgs [][]string
	var ok bool
	switch match {
	case handler.SequenceMatch:
		cmdsAndArgs, ok = e.config.CommandSequenceBindings[seq]
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
		// this is a re-issue so continue processing
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
	if handled {
		return false, handled
	}

	// if event is not bound to command, the finally dispatch to focus handler
	if match != handler.SequenceMatch {
		e.log(log.TraceLevel, "no sequence match: re-dispatching current event %q",
			ev.KeyComb())
		_, handled = e.comp.Browser().Handle(ev)
		if handled {
			return
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
	commandCfg := command.DefaultConfig()
	commandCfg.MaxHistory = e.config.CommandMaxHistory
	commandCfg.HistoryKey = e.config.CommandEvent
	commandCfg.MatchedTextAttr = e.config.CommandOverlay.MatchedTextAttr
	commandCfg.FocusElementAttr = e.config.CommandOverlay.FocusElementAttr
	commandCfg.ElementAttr = e.config.CommandOverlay.ElementAttr
	commandCfg.ManualAttr = e.config.CommandOverlay.ManualAttr
	commandCfg.DocumentID = commandHistoryDocumentID
	commandCfg.FrameCharSet = e.config.FrameCharSet
	commandCfg.FrameAttr = e.config.FrameAttr
	commandCfg.ShowManualAfter = e.config.CommandOverlay.ShowManualAfter
	commandCfg.Sync = e.syncCommandPrompt
	promptStorage := document.WithPartition(e.storage, "cprompt")
	cmd := command.NewPrompt(promptStorage, e, e, e, []command.Manual{}, commandCfg)

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
			), e.onCloseCommandPrompt),
		cmd.Dimensions,
	)
	e.resetCommandList(cmd)

	e.cmdWin = e.cmdV.C.Floating(commandHandler,
		component.FloatingConfig{
			Offset:    term.Coordinates{Y: int(float64(e.height) * 0.2)},
			Alignment: component.SpanAlignmentHorizontallyCentered,
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
		err := e.pasteFromClipboard()
		if err != nil {
			_, _ = e.Browser().Notify(notifications.LevelError, "%v", err)
		}
		return
	}

	_, handled = e.cmdV.Handle(ev)
	return
}

// Handle satisfies tui.Handler.
func (e *ex) Handle(ev term.Event) (exit, handled bool) {
	if e.cmd != nil {
		_, handled = e.handlePrompt(ev)
	} else {
		_, handled = e.handleEvent(ev)
		currFocus, _ := e.comp.Focus()
		if e.fullscreenID != 0 && currFocus.WindowID() != e.fullscreenID {
			_ = e.windowtogglemaximize()
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

// Man satisfies tui.Handler.
func (e *ex) Man() tui.Manual {
	panic("TODO")
}

// Resize satisfies tui.Component
func (e *ex) Resize(width, height int) {
	e.height = height
	e.width = width
	e.tasks.SetMaxWidthHeight(width, height)
	if e.reservoir != nil {
		e.reservoir.Resize(width, height)
	}
	e.comp.Resize(width, height)
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
			e.comp.Draw(term.DimWriter(w))
		} else {
			e.comp.Draw(w)
		}
		vw := component.VirtualWriter{
			Writer: w,
			Offset: e.cmdV.Position(),
			Height: e.cmdV.Height(),
			Width:  e.cmdV.Width(),
		}
		e.cmdV.C.DrawWindow(e.cmdWin, &vw)
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

// Close closes the resources associated with this browser.
func (e *ex) Close() (ret error) {
	e.sequencer.Reset()
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
	if e.cmd != nil {
		if err := e.cmd.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
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
	onFocusChangeTab(t, false)
}

type windowSubscriber ex

func (e *windowSubscriber) OnFocus(prev, curr handler.Window) {
	onFocusChange(prev, false)
	onFocusChange(curr, true)
}

func onFocusChange(win handler.Window, isInFocus bool) {
	if win == (handler.Window{}) {
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

func (c companionTerminalHandler) Close() error {
	return nil
}
