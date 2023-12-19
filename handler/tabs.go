package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

var _ tui.Handler = (*Tabs)(nil)

// Tabs add mouse handling to component.Tabs.
type Tabs struct {
	component.Tabs

	OnClick func(int)
}

// NewTabs returns a Tabs component which handles mouse events.
func NewTabs() *Tabs {
	t := new(Tabs)
	t.Init()
	return t
}

// Init initializes this Tabs with the given underlying handler
// and frame attributes.
func (f *Tabs) Init() {
	f.Tabs.Init()
}

// Handle delegates the event to the underlying handler.
func (f *Tabs) Handle(ev term.Event) (quit, handled bool) {
	if ev.Type != term.EventMouse || ev.Key != term.MouseLeft {
		return
	}
	mousePos := term.Coordinates{X: ev.MouseX, Y: ev.MouseY}
	idx, ok := f.Tabs.TabAt(mousePos)
	if !ok {
		return
	}

	handled = true

	f.SetFocus(idx)
	if f.OnClick != nil {
		f.OnClick(idx)
	}
	return
}

// Cursor returns the underlying handler's cursor position
// with the frame offset.
func (f *Tabs) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, term.CursorStyleDefault, false
}

// Man just delegates Man call to underlying handler.
func (f *Tabs) Man() tui.Manual {
	panic("TODO")
}
