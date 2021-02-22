package handler

import (
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
)

type keyExit struct {
	tui.Component
	key term.Event
}

// KeyExit wraps a tui.Component which exits upon receiveing key.
func KeyExit(c tui.Component, key term.Event) tui.Handler {
	return &keyExit{Component: c, key: key}
}

func (e *keyExit) Handle(ev term.Event) (exit, handled bool) {
	exit = e.key == ev
	handled = exit
	return
}

func (e *keyExit) Cursor() (pos term.Coordinates, show bool) {
	return
}

func (e *keyExit) Man() tui.Manual {
	return tui.Manual{}
}
