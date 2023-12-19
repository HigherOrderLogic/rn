package search

import (
	"unstable.build/go-tui"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
)

type simpleHandler struct {
	*List
	ed tui.Handler
	fn func(string)
}

// Handler wraps a List to satisfy tui.Handler.
// It handles enter key by calling fn with the element in focus,
// if there's an element in focus at all.
// It handles esc key by exiting and handles arrow keys up/down
// by scrolling up and down the list.
func Handler(l *List, fn func(string)) tui.Handler {
	// this must be set for List searchBar Responsive logic to make sense
	const wrap = true

	buf := l.Buffer()
	clipboard := clipboard.NewInMemory()
	attr := term.Attributes{} // do not matter for Handler's purpouse
	ed, _ := text.NewSimpleEditor(clipboard, wrap, true, attr, attr).
		Edit(workspaceapi.RandomURI("search"), buf)
	ret := simpleHandler{List: l, fn: fn, ed: ed}
	return ret
}

func (s simpleHandler) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventKey {
		return
	}

	switch ev.Key {
	case term.KeyEnter, term.KeyTab:
		s.Wait()
		item, ok := s.Focus()
		if ok {
			handled = true
			exit = true
			s.fn(string(item.data))
		}
	case term.KeyCtrlC:
		s.Cancel()
	case term.KeyEsc:
		handled = true
		exit = true
	case term.KeyCtrlJ, term.KeyArrowDown:
		handled = s.FocusDown()
	case term.KeyCtrlK, term.KeyArrowUp:
		handled = s.FocusUp()
	default:
		_, handled = s.ed.Handle(ev)
	}

	return
}

func (s simpleHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	c, style, ok := s.ed.Cursor()
	if s.cfg.bottomSearchBar {
		c.Y += (s.List.height - s.List.inputHeight())
	}
	return c, style, ok
}

func (s simpleHandler) Man() tui.Manual {
	panic("TODO")
}

func (s simpleHandler) Resize(width, height int) {
	s.List.Resize(width, height)

	inputHeight := s.InputHeight()
	s.ed.Resize(width, inputHeight)
}
