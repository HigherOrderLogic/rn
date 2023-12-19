package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// Frame is a proxy handler that simply draws a frame around
// the underlying handler.
type Frame struct {
	component.Frame
	handler tui.Handler
}

// NewFrame allocates storage for a new Frame and initializes it.
func NewFrame(handler tui.Handler) (f *Frame) {
	f = new(Frame)
	f.Init(handler)
	return
}

// Init initializes this Frame with the given underlying handler
// and frame attributes.
func (f *Frame) Init(handler tui.Handler) {
	f.handler = handler
	f.Frame.Init(handler)
}

// Handle delegates the event to the underlying handler.
func (f *Frame) Handle(ev term.Event) (bool, bool) {
	if ev.Type == term.EventMouse {
		content := f.Frame.ContentPosition()
		ev.MouseX -= content.X
		ev.MouseY -= content.Y
		if ev.MouseX < 0 {
			ev.MouseX = 0
		}
		if ev.MouseY < 0 {
			ev.MouseY = 0
		}
	}
	return f.handler.Handle(ev)
}

// Cursor returns the underlying handler's cursor position
// with the frame offset.
func (f *Frame) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	pos, style, show = f.handler.Cursor()
	content := f.Frame.ContentPosition()
	pos.X += content.X
	pos.Y += content.Y
	return
}

// Man just delegates Man call to underlying handler.
func (f *Frame) Man() tui.Manual {
	return f.handler.Man()
}
