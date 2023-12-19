package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// TestHandler is a handler used to test composite handlers. Each event
// processed by this handler increments the Ch rune to the next rune.
type TestHandler struct {
	component.TestComponent
	tui.Manual
	CursorPos      term.Coordinates
	CursorStyle    term.CursorStyle
	Exit           bool
	Handled        bool
	HandleOverride func(term.Event) (bool, bool)
}

// NewTestHandler will allocate storage for a new handler and initialize it
func NewTestHandler() (t *TestHandler) {
	t = new(TestHandler)
	t.Ch = 'A'
	return
}

// Handle the next Event
func (t *TestHandler) Handle(ev term.Event) (bool, bool) {
	if t.HandleOverride != nil {
		return t.HandleOverride(ev)
	}
	// signal that we handled the event
	t.Ch++
	return t.Exit, t.Handled
}

// Cursor returns the set CursorPos.
func (t *TestHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	if t.CursorPos == (term.Coordinates{}) {
		return term.Coordinates{X: -1, Y: -1}, 0, false
	}
	return t.CursorPos, t.CursorStyle, true
}

// Man returns set Manual.
func (t *TestHandler) Man() tui.Manual {
	return t.Manual
}
