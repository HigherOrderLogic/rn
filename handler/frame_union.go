package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// FrameUnion wraps a component.FrameUnion to satisfy tui.Handler by
// dispatching mouse events to FrameUnion nodes and returning the
// correct main Cursor offset.
//
// See component.FrameUnion for more details.
type FrameUnion struct {
	main tui.Handler
	component.FrameUnion
}

// NewFrameUnion allocates storage for a new FrameUnion and initializes it.
func NewFrameUnion(main tui.Handler) *FrameUnion {
	ret := new(FrameUnion)
	ret.Init(main)
	return ret
}

// Init initializes this frame union with main.
func (u *FrameUnion) Init(main tui.Handler) {
	u.main = main
	u.FrameUnion.Init(main)
}

// UnionTop stacks top on top of the main component. This
// method panics if top is nil.
func (u *FrameUnion) UnionTop(top tui.Handler, height int) {
	u.FrameUnion.UnionTop(top, height)
}

// UnionBottom stacks bottom under of the main component. This
// method panics if bottom is nil.
func (u *FrameUnion) UnionBottom(bottom tui.Handler, height int) {
	u.FrameUnion.UnionBottom(bottom, height)
}

// UnionLeft stacks left to the left of the main component. This
// method panics if left is nil.
func (u *FrameUnion) UnionLeft(left tui.Handler, width int) {
	u.FrameUnion.UnionLeft(left, width)
}

// UnionRight stacks right to the right of the main component. This
// method panics if right is nil.
func (u *FrameUnion) UnionRight(right tui.Handler, width int) {
	u.FrameUnion.UnionRight(right, width)
}

// Resize satisfies tui.Handler.
func (u *FrameUnion) Resize(width, height int) {
	u.FrameUnion.Resize(width, height)
}

// Draw satisfies tui.Handler.
func (u *FrameUnion) Draw(w term.Writer) {
	u.FrameUnion.Draw(w)
}

// Handle delegates ev to main handler, unless event is a mouse event,
// in which case it's delegated to the component at ev.MouseX and ev.MouseY.
func (u *FrameUnion) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventMouse {
		return u.main.Handle(ev)
	}

	c, ok := u.FrameUnion.ComponentAt(term.Coordinates{X: ev.MouseX, Y: ev.MouseY})
	if !ok {
		return
	}
	handler := c.C.(tui.Handler)
	ev.MouseX -= c.Position().X
	ev.MouseY -= c.Position().Y
	if ev.MouseX < 0 {
		ev.MouseX = 0
	}
	if ev.MouseY < 0 {
		ev.MouseY = 0
	}
	return handler.Handle(ev)
}

// Cursor returns the main component's cursor position.
func (u *FrameUnion) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	o := u.FrameUnion.MainPosition()
	pos, style, show = u.main.Cursor()
	pos.X += o.X
	pos.Y += o.Y
	return
}

// Man satisfies tui.Handler.
func (u *FrameUnion) Man() tui.Manual {
	return tui.Manual{}
}
