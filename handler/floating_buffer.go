package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// FloatingBuffer wraps a tui.Handler and uses the contents of buffer
// do determine the best dimensions for the given handler.
func FloatingBuffer(h tui.Handler, buffer *cell.Buffer) Floating {
	return floatingBuffer{h: h, Floating: component.FloatingBuffer(h, buffer)}
}

type floatingBuffer struct {
	h tui.Handler
	component.Floating
}

func (f floatingBuffer) Handle(ev term.Event) (exit, handled bool) {
	return f.h.Handle(ev)
}

func (f floatingBuffer) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return f.h.Cursor()
}

func (f floatingBuffer) Man() tui.Manual {
	return f.h.Man()
}
