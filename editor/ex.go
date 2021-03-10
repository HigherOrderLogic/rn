package editor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/component/search"
	"github.com/ernestrc/go-tui/term"
)

var (
	commandBarAttr      = term.Attributes{Bg: term.ColorWhite, Fg: term.ColorBlack}
	errInvalidSetCursor = errors.New("Cannot set cursor on this buffer")
	exCommands          = map[string]func(*Ex, ...string) (bool, error){
		"buffer_prev":      (*Ex).previousBuffer,
		"buffer_next":      (*Ex).nextBuffer,
		"buffer_close":     (*Ex).closeBuffer,
		"buffer_close_all": (*Ex).closeAllBuffers,
		"close":            (*Ex).closeFocusWindow,
		"write_quit":       (*Ex).flushCloseIgnoreNonFlushed,
		"write_quit!":      (*Ex).flushCloseIgnoreNonFlushed,
		"write":            (*Ex).forceFlush,
		"write!":           (*Ex).forceFlush,
		"quit!":            (*Ex).forceQuit,
		"quit":             (*Ex).forceQuit,
		"edit":             (*Ex).openFileTab,
	}
)

type mode int8

const (
	modeDefault mode = iota
	modeCommand

	commandOverlayHeight = 14
	commandOverlayWidth  = 50
)

// Ex implements a tui.Handler by wrapping an editor.Component and
// providing an ex editor type of interface.
type Ex struct {
	config  Config
	comp    Component
	command struct {
		argsMode bool

		component.Virtual
		cell.Buffer

		search.List
		component.Frame
		component.Overlay
	}
	mode mode
}

// NewEx allocates storage for a new Ex and initializes it.
func NewEx(ed Editor, opts ...Option) (e *Ex, err error) {
	e = new(Ex)
	err = e.Init(ed, opts...)
	if err != nil {
		return
	}
	return
}

var (
	defaultSearchListConfig = search.ListConfig{
		Algo:          search.FuzzyMatch,
		Interrupt:     term.Interrupt,
		CaseSensitive: false,
	}
)

// Init initializes this Ex with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (e *Ex) Init(ed Editor, opts ...Option) (err error) {
	e.command.Buffer.Init()
	e.mode = modeDefault

	// overlay buffer over the search list so we can
	// stop the search for multiple argument commands
	// but we can display arguments
	var s component.Scroll
	s.InitWithBuffer(&e.command.Buffer)
	e.command.Virtual.C = &s
	e.command.Virtual.Resize(commandOverlayWidth, commandOverlayHeight)

	e.config = DefaultConfig()
	for _, o := range opts {
		o(&e.config)
	}
	cfg := defaultSearchListConfig
	attr := term.Attributes{}

	var commandOverlay tui.Component
	if e.config.Frame {
		commandOverlay = &e.command.Frame
		e.command.Frame.Init(&e.command.List)
	} else {
		attr = term.Attributes{Fg: term.AttrReverse, Bg: term.AttrReverse}
		cfg.MatchedTextAttr = &term.Attributes{Bg: term.AttrReverse, Fg: term.ColorRed}
		cfg.CountAttr = &term.Attributes{Bg: term.AttrReverse, Fg: term.ColorRed | term.AttrBold}
		cfg.SearchBaseAttr = &term.Attributes{Bg: term.AttrReverse, Fg: term.AttrReverse}
		cfg.FocusElementAttr = &term.Attributes{Bg: term.AttrReverse, Fg: term.ColorRed | term.AttrBold}
		cfg.ElementAttr = &term.Attributes{Bg: term.AttrReverse, Fg: term.AttrReverse}
		commandOverlay = &e.command.List
	}

	e.command.List.Init(cfg)
	e.command.Overlay.Init(&e.comp, commandOverlay, attr, component.SpanConfig{
		PadVertical:   -commandOverlayHeight,
		PadHorizontal: -commandOverlayWidth,
	})
	err = e.comp.Init(ed, e.config)
	return
}

func (e *Ex) handlerInFocus() (string, Handler, bool) {
	focus, _ := e.comp.Focus()
	content, _ := focus.Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return "", nil, false
	}

	return t.ID(), t.Handler().(Handler), true
}

func (e *Ex) moveFocusCursor(line int) error {
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

func (e *Ex) previousBuffer(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.UpdateWindowTabPrev(b.Focus())
	return false, nil
}

func (e *Ex) nextBuffer(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.UpdateWindowTabNext(b.Focus())
	return false, nil
}

func (e *Ex) closeBuffer(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.RemoveWindowContent(b.Focus())
	return false, nil
}

func (e *Ex) closeAllBuffers(args ...string) (bool, error) {
	b := e.comp.Browser()
	b.RemoveAllTabs()
	return false, nil
}

func (e *Ex) closeFocusWindow(args ...string) (bool, error) {
	b := e.comp.Browser()
	return false, b.Focus().Close()
}

func (e *Ex) flushCloseIgnoreNonFlushed(args ...string) (bool, error) {
	b := e.comp.Browser()
	return true, e.comp.Flush(b.Focus())
}

func (e *Ex) forceFlush(args ...string) (bool, error) {
	b := e.comp.Browser()
	return false, e.comp.Flush(b.Focus())
}

func (e *Ex) forceQuit(args ...string) (bool, error) {
	return true, nil
}

func (e *Ex) runSingleCommand(cmd string) (quit bool, err error) {
	fnCmd, ok := exCommands[cmd]
	if ok {
		return fnCmd(e)
	}

	line, cerr := strconv.Atoi(cmd)
	if cerr == nil {
		err = e.moveFocusCursor(line - 1)
		return
	}
	name, h, ok := e.handlerInFocus()
	var handled bool
	if ok {
		handled = e.comp.DispatchCommand(h, name, cmd)
	}
	if !handled {
		err = fmt.Errorf("Unknown command: %s", cmd)
	}
	return
}

func (e *Ex) openFileTab(args ...string) (bool, error) {
	var h browser.Handler
	h, err := e.comp.OpenFileTab(args[0], "")
	if err != nil {
		return false, err
	}
	return false, e.comp.Browser().Focus().SetContent(h)
}

func (e *Ex) runCommand(cmd string, cmdAndArgs string) (quit bool, err error) {
	parts := strings.Split(cmdAndArgs, " ")
	if len(parts) == 1 {
		if len(cmd) == 0 {
			cmd = cmdAndArgs
		}
		return e.runSingleCommand(cmd)
	}

	fnCmd, ok := exCommands[cmd]
	if ok {
		return fnCmd(e, parts[1:]...)
	}

	name, h, ok := e.handlerInFocus()
	var handled bool
	if ok {
		handled = e.comp.DispatchCommand(h, name, cmd, parts[1:]...)
	}
	if !handled {
		err = fmt.Errorf("Unknown command: %s", cmd)
	}
	return
}

func (e *Ex) setError(err error) {
	e.comp.Browser().SetMessage("Error: %s", err)
}

func (e *Ex) handleCommand(ev term.Event) (quit, handled bool) {
	handled = true

	switch ev.Key {
	case term.KeyEnter:
		command, _ := e.command.List.Focus()
		var err error
		quit, err = e.runCommand(string(command), e.command.Buffer.String())
		e.setNormalMode()
		if err != nil {
			e.setError(err)
		}
	case term.KeyEsc:
		e.setNormalMode()
	case term.KeyArrowDown:
		e.command.List.FocusDown()
	case term.KeyArrowUp:
		e.command.List.FocusUp()
	case term.KeySpace:
		ev.Ch = ' '
		handled = false
	case term.KeyBackspace, term.KeyBackspace2:
		cols := e.command.Buffer.Columns(0)
		if cols == 0 {
			e.setNormalMode()
			return
		}
		e.command.List.SearchQueryDelete()
		_, r, _ := e.command.Buffer.DeleteCell(term.Coordinates{X: cols - 1})
		if r == ' ' {
			e.command.argsMode = false
		}
	default:
		handled = false
	}

	if handled {
		return
	}

	if ev.Ch == 0 {
		return
	}

	if ev.Ch == ' ' {
		e.command.argsMode = true
	}

	e.command.Buffer.WriteString(string([]rune{ev.Ch}))

	if !e.command.argsMode {
		e.command.List.SearchQueryWrite(ev.Ch)
	}
	return
}

func (e *Ex) handleCommandEvent(ev term.Event) bool {
	if ev == e.comp.config.CommandEvent {
		e.setCommandMode()
		return true
	}
	return false
}

func (e *Ex) handleProxy(ev term.Event) (
	exit, handled bool,
) {
	// If Ex is configured with non character
	// command mode trigger event, then this takes
	// precedence over any other event
	if e.comp.config.CommandEvent.Ch == 0 {
		handled = e.handleCommandEvent(ev)
		if handled {
			return
		}
	}

	mev, cmd, _ := e.comp.KeyMapping(ev)
	if cmd != "" {
		// command mappings take precedence over ev subscriptions
		// or other ex key mappings
		quit, err := e.runCommand(cmd, cmd)
		if err != nil {
			e.setError(err)
		}
		return quit, true
	}

	browser := e.comp.Browser()
	switch mev.Key {
	case term.KeyCtrlA:
		browser.RemoveAllTabs()
	case term.KeyCtrlW:
		browser.RemoveWindowContent(browser.Focus())
	case term.KeyCtrlL:
		browser.UpdateWindowTabNext(browser.Focus())
	case term.KeyCtrlH:
		browser.UpdateWindowTabPrev(browser.Focus())
	default:
		exit, handled = browser.Handle(mev)
		if handled {
			return
		}

		handled = e.comp.Publish(mev)
		if handled {
			return
		}
		// If ex is configured with character
		// command mode trigger event (i.e. ':')
		// then we assume that the underlying editor is
		// a modal editor, and so does not handle
		// the command trigger event in its "initial" mode
		// (in vi terms, this would be normal mode).
		handled = e.handleCommandEvent(mev)
		return
	}

	return false, true
}

func (e *Ex) setNormalMode() {
	e.command.Buffer.Reset()
	e.command.List.SearchReset()
	e.command.argsMode = false
	e.mode = modeDefault
}

func (e *Ex) setCommandMode() {
	e.mode = modeCommand

	// commands can be registered dynamicall via Editor.Register:
	// compile a new list every time we switch to command mode
	e.command.List.DataReset()
	for cmd := range exCommands {
		e.command.List.PushSync([]byte(cmd))
	}
	for _, cmd := range e.comp.Commands() {
		e.command.List.PushSync([]byte(cmd))
	}
}

// Handle satisfies tui.Handler.
func (e *Ex) Handle(ev term.Event) (bool, bool) {
	switch e.mode {
	case modeDefault:
		return e.handleProxy(ev)
	case modeCommand:
		return e.handleCommand(ev)
	default:
		panic(fmt.Sprintf("unknown mode: %+v", e.mode))
	}
}

func (e *Ex) overlayPosition() (pos term.Coordinates) {
	pos = e.command.Overlay.ContentOffset()
	if e.config.Frame {
		pos.Y++
		pos.X++
	}
	return
}

// Cursor satisfies tui.Handler.
func (e *Ex) Cursor() (pos term.Coordinates, show bool) {
	if e.mode == modeCommand {
		pos := e.command.Virtual.Position()
		pos.X += len(e.command.Buffer.String())
		return pos, true
	}
	return e.comp.Browser().Cursor()
}

// Man satisfies tui.Handler.
func (e *Ex) Man() tui.Manual {
	panic("TODO")
}

// Resize satisfies tui.Component
func (e *Ex) Resize(width, height int) {
	// internally resizes e.comp
	e.command.Overlay.Resize(width, height)

	pos := e.overlayPosition()
	e.command.Virtual.Move(pos)
}

// Draw satisfies tui.Component
func (e *Ex) Draw(w term.Writer) {
	if e.mode == modeCommand {
		e.command.Overlay.Draw(w)
		e.command.Virtual.Draw(w)
		return
	}
	e.comp.Draw(w)
}

// Editor returns the underlying Editor implementation.
func (e *Ex) Editor() Editor {
	return &e.comp
}

// Browser returns the underlying browser.Browser implementaiton.
func (e *Ex) Browser() browser.Browser {
	return &e.comp
}

// Close closes the resources associated with this browser.
func (e *Ex) Close() error {
	err1 := e.command.List.Close()
	err2 := e.comp.Close()
	if err2 != nil {
		return err2
	}
	return err1
}
