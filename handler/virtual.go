package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// Virtual wraps another tui.Handler to provide cursor event coordinates.
type Virtual struct {
	component.Virtual
}

// Handle tui.Handler
func (v *Virtual) Handle(ev term.Event) (bool, bool) {
	if ev.Type == term.EventMouse {
		offset := v.Position()
		ev.MouseX -= offset.X
		ev.MouseY -= offset.Y
	}
	return v.C.(tui.Handler).Handle(ev)
}

// Cursor tui.Handler
func (v *Virtual) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	pos, style, show = v.C.(tui.Handler).Cursor()
	offset := v.Position()
	pos.X += offset.X
	pos.Y += offset.Y
	return
}

// Man tui.Handler
func (v *Virtual) Man() tui.Manual {
	return v.C.(tui.Handler).Man()
}
