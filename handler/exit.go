package handler

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

type keyExit struct {
	tui.Component
	key term.KeyComb
	cb  func()
}

// KeyExit wraps a tui.Component which exits upon receiveing key.
func KeyExit(c tui.Component, key term.KeyComb) tui.Handler {
	return &keyExit{Component: c, key: key}
}

// KeyExitCallback wraps a tui.Component which exits and calls cb upon receiveing key.
func KeyExitCallback(c tui.Component, key term.KeyComb, cb func()) tui.Handler {
	return &keyExit{Component: c, key: key, cb: cb}
}

func (e *keyExit) Handle(ev term.Event) (exit, handled bool) {
	exit = e.key == ev.KeyComb()
	handled = exit
	if e.cb != nil {
		e.cb()
	}
	return
}

func (e *keyExit) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return
}

func (e *keyExit) Man() tui.Manual {
	return tui.Manual{}
}
