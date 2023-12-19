package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

var _ tui.Handler = (*Proxy)(nil)

// Proxy satisfies tui.Handler by taking a pointer to a tui.Handler
// and dereferencing on each method call. Caller is responsible for
// initializing Ptr correctly.
type Proxy struct {
	Target tui.Handler
}

// Resize satisfies tui.Handler.
func (i *Proxy) Resize(width, height int) {
	i.Target.Resize(width, height)
}

// Draw satisfies tui.Handler.
func (i *Proxy) Draw(w term.Writer) {
	i.Target.Draw(w)
}

// Handle satisfies tui.Handler.
func (i *Proxy) Handle(ev term.Event) (exit, handled bool) {
	return i.Target.Handle(ev)
}

// Cursor satisfies tui.Handler.
func (i *Proxy) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return i.Target.Cursor()
}

// Man satisfies tui.Handler.
func (i *Proxy) Man() tui.Manual {
	return i.Target.Man()
}
