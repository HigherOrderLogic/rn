package editor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
)

var (
	commandBarAttr      = term.Attributes{Bg: term.ColorWhite, Fg: term.ColorBlack}
	errInvalidSetCursor = errors.New("Cannot set cursor on this buffer")
)

type mode int8

const (
	modeDefault mode = iota
	modeCommand
)

// Ex implements a tui.Handler by wrapping an editor.Component and
// providing an ex editor type of interface.
type Ex struct {
	comp       Component
	commandBuf *cell.Buffer
	cmdVirt    handler.Virtual
	mode       mode
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

// Init initializes this Ex with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (e *Ex) Init(ed Editor, opts ...Option) (err error) {
	e.commandBuf = cell.NewBuffer()
	e.cmdVirt = browser.NewMessageSpan(e.commandBuf, commandBarAttr)
	e.mode = modeDefault

	config := DefaultConfig()
	for _, o := range opts {
		o(&config)
	}
	err = e.comp.Init(ed, config)
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

func (e *Ex) runSingleCommand(cmd string) (quit bool, err error) {
	b := e.comp.Browser()
	switch cmd {
	case "bprev":
		b.UpdateWindowTabPrev(b.Focus())
	case "bnext":
		b.UpdateWindowTabNext(b.Focus())
	case "bclose":
		b.RemoveWindowContent(b.Focus())
	case "bcloseAll":
		b.RemoveAllTabs()
	case "close":
		err = b.Focus().Close()
	case "wq", "wq!":
		quit = true
		fallthrough
	case "w", "w!":
		err = e.comp.Flush(b.Focus())
	case "q!", "q":
		quit = true
	default:
		line, cerr := strconv.Atoi(cmd)
		if cerr == nil {
			err = e.moveFocusCursor(line - 1)
			return
		}
		name, h, ok := e.handlerInFocus()
		var handled bool
		if ok {
			handled = e.comp.DispatchCommand(cmd, h, name)
		}
		if !handled {
			err = fmt.Errorf("Unknown command: %s", cmd)
		}
	}
	return
}

func (e *Ex) runCommand(cmd string) (quit bool, err error) {
	parts := strings.Split(cmd, " ")
	if len(parts) == 1 {
		return e.runSingleCommand(parts[0])
	}

	switch parts[0] {
	case "e":
		var h browser.Handler
		h, err = e.comp.OpenFileTab(parts[1], "")
		if err != nil {
			return
		}
		e.comp.Browser().Focus().SetContent(h)
	default:
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
		var err error
		quit, err = e.runCommand(e.commandBuf.String())
		e.setNormalMode()
		if err != nil {
			e.setError(err)
		}
	case term.KeyEsc:
		e.setNormalMode()
	case term.KeyBackspace, term.KeyBackspace2:
		cols := e.commandBuf.Columns(0)
		if cols == 0 {
			e.setNormalMode()
			return
		}
		e.commandBuf.DeleteCell(term.Coordinates{X: cols - 1})
	case term.KeySpace:
		ev.Ch = ' '
		fallthrough
	default:
		if ev.Type == term.EventKey && ev.Ch != 0 {
			e.commandBuf.WriteString(string(ev.Ch))
		} else {
			handled = false
		}
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
		quit, err := e.runCommand(cmd)
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
	e.commandBuf.Reset()
	e.mode = modeDefault
}

func (e *Ex) setCommandMode() {
	e.mode = modeCommand
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

// Cursor satisfies tui.Handler.
func (e *Ex) Cursor() (pos term.Coordinates, show bool) {
	if e.mode == modeCommand {
		pos := e.cmdVirt.Position()
		pos.X += len(e.commandBuf.String())
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
	browser.ResizeMessageSpan(&e.cmdVirt, width, height)
	e.comp.Resize(width, height)
}

// Draw satisfies tui.Component
func (e *Ex) Draw(w term.Writer) {
	e.comp.Draw(w)

	if e.mode == modeCommand {
		e.cmdVirt.Draw(w)
	}
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
	return e.comp.Close()
}
