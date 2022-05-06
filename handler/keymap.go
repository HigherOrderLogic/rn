package handler

import (
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
)

type keyMappingHandler struct {
	tui.Component
	inner    tui.Handler
	mappings map[term.KeyComb]term.KeyComb
}

// WithMapping takes a handler and a set of event mappings to provide
// key and event mapping to override default handler event handler.
func WithMapping(
	inner tui.Handler, mappings map[term.KeyComb]term.KeyComb,
) tui.Handler {
	return keyMappingHandler{inner, inner, mappings}
}

// Handle finds a mapping and overwrites event or delegates the event to
// underlying handler.
func (k keyMappingHandler) Handle(ev term.Event) (bool, bool) {
	if ev.Type != term.EventKey {
		return k.inner.Handle(ev)
	}
	mapped, ok := k.mappings[ev.KeyComb()]
	if ok {
		ev = term.Event{
			Type: term.EventKey,
			Ch:   mapped.Ch,
			Mod:  mapped.Mod,
			Key:  mapped.Key,
		}
	}
	return k.inner.Handle(ev)
}

// Cursor delegates call to underlying handler.
func (k keyMappingHandler) Cursor() (term.Coordinates, bool) {
	return k.inner.Cursor()
}

// Man returns remapped Manual from underlying handler.
func (k keyMappingHandler) Man() tui.Manual {
	m := k.inner.Man()
	for from, to := range k.mappings {
		m.Keys[to] = m.Keys[from]
	}
	return m
}
