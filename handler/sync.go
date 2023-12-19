package handler

import (
	"sync"

	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

type hsync struct {
	mu sync.Locker
	h  tui.Handler
}

// Sync wraps a tui.Handler to provide access synchronization with mu.
func Sync(mu sync.Locker, h tui.Handler) tui.Handler {
	return hsync{mu: mu, h: h}
}

func (s hsync) Resize(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h.Resize(width, height)
}

func (s hsync) Draw(w term.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h.Draw(w)
}

func (s hsync) Handle(ev term.Event) (exit, handled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h.Handle(ev)
}

func (s hsync) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h.Cursor()
}

func (s hsync) Man() tui.Manual {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h.Man()
}
