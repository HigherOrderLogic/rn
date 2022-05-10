package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
)

const (
	commandHistoryDocumentID = "ex-command-history"
)

var (
	commandBarAttr      = term.Attributes{Bg: term.ColorWhite, Fg: term.ColorBlack}
	errInvalidSetCursor = errors.New("Cannot set cursor on this buffer")
	exCommands          = map[string]func(*ex, ...string) (bool, error){
		"bufferPrev":             (*ex).previousBuffer,
		"bufferNext":             (*ex).nextBuffer,
		"bufferClose":            (*ex).closeBuffer,
		"bufferCloseAll":         (*ex).closeAllBuffers,
		"close":                  (*ex).closeFocusWindow,
		"writeQuit":              (*ex).flushCloseIgnoreNonFlushed,
		"writeForceQuit!":        (*ex).flushCloseIgnoreNonFlushed,
		"write":                  (*ex).forceFlush,
		"forceWrite!":            (*ex).forceFlush,
		"forceQuit!":             (*ex).forceQuit,
		"quit":                   (*ex).forceQuit,
		"edit":                   (*ex).editFile,
		"reload":                 (*ex).reloadFile,
		"changeSplitOrientation": (*ex).splitDirectionChange,
		"splitWindow":            (*ex).newWindow,
		"newWindow":              (*ex).newWindow,
		"focusNextWindow":        (*ex).focusNextWindow,
		"focusPrevWindow":        (*ex).focusPrevWindow,
		"focusAboveWindow":       (*ex).focusAboveWindow,
		"focusBelowWindow":       (*ex).focusBelowWindow,
	}
	exDefaultBindings = map[term.KeyComb]string{
		{Key: term.KeyCtrlA}: "bufferCloseAll",
		{Key: term.KeyCtrlW}: "bufferClose",
		{Key: term.KeyCtrlL}: "bufferNext",
		{Key: term.KeyCtrlH}: "bufferPrev",
		{Key: term.KeyCtrlN}: "newWindow",
		{Ch: 'H'}:            "focusPrevWindow",
		{Ch: 'L'}:            "focusNextWindow",
		{Ch: 'J'}:            "focusBelowWindow",
		{Ch: 'K'}:            "focusAboveWindow",
		{Key: term.KeyCtrlO}: "changeSplitOrientation",
		{Key: term.KeyCtrlQ}: "close",
	}
)

type mode int8

const (
	modeDefault mode = iota
	modeCommand
)

// used to abstract workspace.Manager
type workspaceIfc interface {
	workspace.ResourceOpener // needed by text.Component
	URI(string) (workspace.URI, error)
}

// ex implements a tui.Handler by wrapping an editor.Component and
// providing an ex editor type of interface.
type ex struct {
	config          text.Config
	comp            text.Component
	ed              text.Editor
	workspace       workspaceIfc
	sequencer       handler.Sequencer
	cmdOverride     func([]string) (bool, bool, error)
	enabledCommands []string
	cmd             commandListHandler
	overlay         component.Overlay
	mode            mode
	nextSplit       bool
}

func newEx(
	ed text.Editor, m workspaceIfc, enabledCommands []string,
	commandOverride func([]string) (bool, bool, error),
	opts ...text.Option,
) (e *ex, err error) {
	e = new(ex)
	err = e.init(ed, m, enabledCommands, commandOverride, opts...)
	if err != nil {
		return
	}
	return
}

// Init initializes this ex with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (e *ex) init(
	ed text.Editor, m workspaceIfc, enabledCommands []string,
	commandOverride func([]string) (bool, bool, error),
	opts ...text.Option,
) (err error) {
	err = e.doInit(ed, m, enabledCommands, commandOverride, opts...)
	if err != nil {
		return
	}
	err = e.comp.Init(ed, m, e.config)
	if err != nil {
		return
	}
	e.cmd.loadHistory()
	return nil
}

// init is used for internal testing
func (e *ex) doInit(
	ed text.Editor, m workspaceIfc, enabledCommands []string,
	commandOverride func([]string) (bool, bool, error),
	opts ...text.Option,
) (err error) {
	e.mode = modeDefault
	e.workspace = m
	e.cmdOverride = commandOverride
	e.enabledCommands = enabledCommands

	e.config = text.DefaultConfig()

	for ev, cmd := range exDefaultBindings {
		opts = append(opts, text.WithCommandKeyBinding(ev, cmd))
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

	if e.config.CommandOverlay.Width <= 0 || e.config.CommandOverlay.Height <= 0 {
		msg := fmt.Sprintf("invalid CommandOverlay dimensions: %v",
			e.config.CommandOverlay)
		panic(msg)
	}

	e.cmd.init(e.Browser(), e.config.CommandMaxHistory,
		e.config.CommandOverlay, e.config.CommandEvent,
		func(command string, cmdAndArgs string) bool {
			quit, err := e.runCommand(string(command), cmdAndArgs)
			e.setProxyMode()
			if err != nil {
				e.setError(err)
			}
			return quit
		})

	var commandOverlay tui.Component
	if e.config.CommandOverlay.Frame {
		commandOverlay = component.NewFrame(&e.cmd)
	} else {
		commandOverlay = &e.cmd
	}

	e.overlay.Init(&e.comp, commandOverlay,
		e.config.CommandOverlay.ElementAttr,
		component.SpanConfig{
			PadVertical:      -e.config.CommandOverlay.Height,
			PadHorizontal:    -e.config.CommandOverlay.Width,
			ContentAlignment: component.SpanAlignmentCentered,
		})

	e.ed = ed
	return
}

func (e *ex) handlerInFocus() (workspace.URI, text.Handler, bool) {
	focus, _ := e.comp.Focus()
	content, _ := focus.Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return workspace.URI{}, nil, false
	}
	ret, ok := t.Handler().(text.Handler)
	if !ok {
		return workspace.URI{}, nil, false
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

func (e *ex) previousBuffer(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.EditWindowTabPrev(b.Focus())
	return false, nil
}

func (e *ex) nextBuffer(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.EditWindowTabNext(b.Focus())
	return false, nil
}

func (e *ex) closeBuffer(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.RemoveWindowContent(b.Focus())
	return false, nil
}

func (e *ex) closeAllBuffers(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.RemoveAllTabs()
	return false, nil
}

func (e *ex) closeFocusWindow(args ...string) (bool, error) {
	b := e.comp.Browser()
	return false, b.Focus().Close()
}

func (e *ex) flushCloseIgnoreNonFlushed(args ...string) (bool, error) {
	b := e.comp.Browser()
	return true, e.comp.Flush(b.Focus())
}

func (e *ex) forceFlush(args ...string) (bool, error) {
	b := e.comp.Browser()
	return false, e.comp.Flush(b.Focus())
}

func (e *ex) forceQuit(args ...string) (bool, error) {
	return true, nil
}

func (e *ex) dispatchCommand(cmd string, args ...string) (err error) {
	uri, h, ok := e.handlerInFocus()
	scmd := text.Command{
		Name:     cmd,
		Args:     args,
		Resource: h,
		URI:      uri,
	}
	if ok {
		scmd.Cursor.Content, _ = e.ed.Cursor(h)
		scmd.Cursor.Window, _ = h.Cursor()
	}
	handled := e.comp.DispatchCommand(scmd)
	if !handled {
		err = fmt.Errorf("Unknown command: '%s'", cmd)
	}
	return
}

func (e *ex) runSingleCommand(cmd string) (quit bool, err error) {
	fnCmd, ok := exCommands[cmd]
	if ok {
		return fnCmd(e)
	}

	line, cerr := strconv.Atoi(cmd)
	if cerr == nil {
		err = e.moveFocusCursor(line - 1)
		return
	}
	return false, e.dispatchCommand(cmd)
}

func (e *ex) editFileURI(uri workspace.URI) error {
	h, err := e.comp.Open(uri)
	if err != nil {
		return err
	}
	err = e.comp.Browser().Focus().SetContent(h)
	if err == browser.ErrTabNotFree {
		err = nil
	}
	return err
}

func (e *ex) editFile(args ...string) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("expected file name")
	}
	// attempt to parse URI otherwise expect local file path
	uri, err := workspace.ParseURI(args[0])
	if err != nil {
		uri, err = e.workspace.URI(args[0])
		if err != nil {
			return false, err
		}
	}
	return false, e.editFileURI(uri)
}

func (e *ex) reloadFile(args ...string) (bool, error) {
	b := e.comp.Browser()
	focus := b.Focus()
	uri, _, ok := e.handlerInFocus()
	if !ok {
		return false, errors.New("not a file")
	}
	b.RemoveWindowContent(focus)
	return false, e.editFileURI(uri)
}

func (e *ex) splitDirectionChange(args ...string) (bool, error) {
	e.nextSplit = !e.nextSplit
	b := e.comp.Browser()
	if e.nextSplit {
		b.SetDefaultSplit(browser.OrientationBottom)
		b.SetMessage("changed split direction to horizontal")
	} else {
		b.SetDefaultSplit(browser.OrientationRight)
		b.SetMessage("changed split direction to vertical")
	}
	return false, nil
}

func (e *ex) newWindowHandler(h browser.Handler) {
	eb := e.comp.Browser()
	eb.Split(browser.OrientationDefault, h)
}

func (e *ex) focusNextWindow(args ...string) (bool, error) {
	e.comp.Browser().FocusRight()
	return false, nil
}

func (e *ex) focusPrevWindow(args ...string) (bool, error) {
	e.comp.Browser().FocusLeft()
	return false, nil
}

func (e *ex) focusAboveWindow(args ...string) (bool, error) {
	e.comp.Browser().FocusUp()
	return false, nil
}

func (e *ex) focusBelowWindow(args ...string) (bool, error) {
	e.comp.Browser().FocusDown()
	return false, nil
}

func (e *ex) newWindow(args ...string) (bool, error) {
	e.newWindowHandler(nil)
	return false, nil
}

func (e *ex) runCommand(cmd string, cmdAndArgs string) (quit bool, err error) {
	parts := strings.Split(cmdAndArgs, " ")
	// check to workaround default :<number> command to go to line:
	// cmd is empty because it didn't match any command in the list
	// but default behaviour is to move cursor to line
	if cmd != "" {
		parts[0] = cmd // cmdAndArgs contains fuzzy completed command
	}

	if e.cmdOverride != nil {
		quit, handled, err := e.cmdOverride(parts)
		if quit || handled || err != nil {
			return quit, err
		}
	}

	if len(parts) == 1 {
		return e.runSingleCommand(parts[0])
	}

	fnCmd, ok := exCommands[parts[0]]
	if ok {
		return fnCmd(e, parts[1:]...)
	}

	return false, e.dispatchCommand(parts[0], parts[1:]...)
}

func (e *ex) setError(err error) {
	e.comp.Browser().SetMessage("Error: %s", err)
}

func (e *ex) handleCommandEvent(ev term.Event) bool {
	if ev.KeyComb() == e.config.CommandEvent {
		e.setCommandMode()
		return true
	}
	return false
}

func (e *ex) handleProxy(ev term.Event) (
	exit, handled bool,
) {
	// If ex is configured with non character
	// command mode trigger event, then this takes
	// precedence over any other event
	if e.config.CommandEvent.Ch == 0 {
		handled = e.handleCommandEvent(ev)
		if handled {
			return
		}
	}

	b := e.comp.Browser()

	// if focus handle handled event, then that takes precedence
	_, handled = b.Handle(ev)
	if handled {
		return
	}

	cmd, ok := e.comp.KeyMapping(ev.KeyComb())
	seq, match := e.sequencer.Handle(ev)
	if match {
		cmd, ok = e.config.CommandSequenceBindings[seq]
		if !ok {
			panic("key sequencer matched but no command configured")
		}
	}

	// dispatch bound event command or sequence command
	if cmd != "" {
		quit, err := e.runCommand(cmd, cmd)
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

func (e *ex) setProxyMode() {
	e.cmd.reset()
	e.mode = modeDefault
}

func (e *ex) setCommandMode() {
	e.mode = modeCommand

	// commands can be registered dynamicall via Editor.Register:
	// compile a new list every time we switch to command mode
	var commands []string
	for _, cmd := range e.enabledCommands {
		commands = append(commands, cmd)
	}
	for _, cmd := range e.comp.Commands() {
		commands = append(commands, cmd)
	}
	e.cmd.dataReset(commands)
}

// Handle satisfies tui.Handler.
func (e *ex) Handle(ev term.Event) (bool, bool) {
	switch e.mode {
	case modeDefault:
		return e.handleProxy(ev)
	case modeCommand:
		quit, handled := e.cmd.Handle(ev)
		if quit {
			// hack to signal proc exit
			if !handled {
				return true, true
			}
			e.setProxyMode()
		}
		return false, handled
	default:
		panic(fmt.Sprintf("unknown mode: %+v", e.mode))
	}
}

func (e *ex) overlayPosition() (pos term.Coordinates) {
	pos = e.overlay.ContentOffset()
	if e.config.CommandOverlay.Frame {
		pos.Y++
		pos.X++
	}
	return
}

// Cursor satisfies tui.Handler.
func (e *ex) Cursor() (pos term.Coordinates, show bool) {
	if e.mode == modeCommand {
		overlay := e.overlayPosition()
		pos, show = e.cmd.Cursor()
		pos.X += overlay.X
		pos.Y += overlay.Y
		return
	}
	return e.comp.Browser().Cursor()
}

// Man satisfies tui.Handler.
func (e *ex) Man() tui.Manual {
	panic("TODO")
}

// Resize satisfies tui.Component
func (e *ex) Resize(width, height int) {
	e.overlay.Resize(width, height)
}

// Draw satisfies tui.Component
func (e *ex) Draw(w term.Writer) {
	if e.mode == modeCommand {
		e.overlay.Draw(w)
		return
	}
	e.comp.Draw(w)
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
	if err := e.cmd.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := e.comp.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	return
}
