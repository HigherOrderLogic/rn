package handler

import (
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
)

type keyExit struct {
	tui.Component
	key term.KeyComb
}

// KeyExit wraps a tui.Component which exits upon receiveing key.
func KeyExit(c tui.Component, key term.KeyComb) tui.Handler {
	return &keyExit{Component: c, key: key}
}

func (e *keyExit) Handle(ev term.Event) (exit, handled bool) {
	exit = e.key == ev.KeyComb()
	handled = exit
	return
}

func (e *keyExit) Cursor() (pos term.Coordinates, show bool) {
	return
}

func (e *keyExit) Man() tui.Manual {
	return tui.Manual{}
}
