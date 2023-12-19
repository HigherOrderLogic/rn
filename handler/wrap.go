package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

type wrapHandler struct {
	h  tui.Handler
	fn func(term.Event) (bool, bool)
}

// Wrap wraps a tui.Handler with a fn that gets called
// instead of Handle called on h. The rest of tui.Handler
// methods are delegated directly to h, so if h.Handle needs to be
// called, it's the client's responsibility to do so.
//
// Additionally to the usual term.Events, term.EventResize events
// are also dispatched as events to fn, after Resize has been called on h.
func Wrap(h tui.Handler, fn func(term.Event) (bool, bool)) tui.Handler {
	return wrapHandler{h: h, fn: fn}
}

func (n wrapHandler) Resize(width, height int) {
	n.h.Resize(width, height)
	n.fn(term.Event{Type: term.EventResize, Width: width, Height: height})
}

func (n wrapHandler) Draw(w term.Writer) {
	n.h.Draw(w)
}

func (n wrapHandler) Handle(ev term.Event) (exit, handled bool) {
	return n.fn(ev)
}

func (n wrapHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return n.h.Cursor()
}

func (n wrapHandler) Man() tui.Manual {
	return n.Man()
}
