package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

// HandlerSequenceTestCase represents an input sequence and
// the result expected draw string representation.
type HandlerSequenceTestCase struct {
	InputSequence string
	Expected      string
}

// HandlerTestCase represents an event and the result
// expected draw string representation.
type HandlerTestCase struct {
	Event    term.Event
	Expected string
}

func handleTestCase(
	t *testing.T, i int, w *term.StringWriter,
	h tui.Handler, tcase HandlerSequenceTestCase,
	width, height int,
) {
	err := w.Clear(term.Attributes{Fg: 0, Bg: 0})
	require.NoError(t, err)

	var shouldSleep time.Duration
	var escapeNext bool
	for _, r := range tcase.InputSequence {
		if escapeNext {
			escapeNext = false
			h.Handle(term.Event{Ch: r, Type: term.EventKey})
			continue
		}
		switch r {
		case ':':
			h.Handle(term.Event{Key: term.KeyCtrlBackslash, Type: term.EventKey})
		case '`':
			h.Handle(term.Event{Key: term.KeyCtrlV, Type: term.EventKey})
		case '_':
			shouldSleep += 100
		case ' ':
			h.Handle(term.Event{Key: term.KeySpace, Type: term.EventKey})
		case '^':
			h.Handle(term.Event{Key: term.KeyBackspace, Type: term.EventKey})
		case '#':
			h.Handle(term.Event{Key: term.KeyCtrlH, Type: term.EventKey})
		case '$':
			h.Handle(term.Event{Key: term.KeyCtrlL, Type: term.EventKey})
		case '>':
			h.Handle(term.Event{Key: term.KeyEnter, Type: term.EventKey})
		case '<':
			h.Handle(term.Event{Key: term.KeyEsc, Type: term.EventKey})
		case '✌':
			h.Handle(term.Event{Key: term.KeyTab, Type: term.EventKey})
		case '⬇':
			h.Handle(term.Event{Key: term.KeyArrowDown, Type: term.EventKey})
		case '⬆':
			h.Handle(term.Event{Key: term.KeyArrowUp, Type: term.EventKey})
		case '\\':
			escapeNext = true
		default:
			h.Handle(term.Event{Ch: r, Type: term.EventKey})
		}
	}

	// this is a hack for async handlers
	if shouldSleep != 0 {
		// wait until all events have been dispatched
		time.Sleep(shouldSleep * time.Millisecond)
		// force a draw and wait for the draw response to arrive
		h.Draw(term.NewStringWriter(width, height))
		time.Sleep(shouldSleep * time.Millisecond)
	}
	h.Draw(w)

	cursor, _, ok := h.Cursor()
	if ok {
		w.SetCursor(cursor)
	}

	err = w.Flush()
	require.NoError(t, err)

	out := w.String()
	assert.Equal(t, tcase.Expected, out, "test case %d", i)
}

// TestHandlerIsolated is a helper function that drives
// a set of HandlerSequenceTestCase and its results in an isolated fashion:
// fn will be called on every test case.
//
// See TestHandlerSequence for more details.
func TestHandlerIsolated(
	t *testing.T, fn func(t *testing.T) tui.Handler, width, height int,
	cases []HandlerSequenceTestCase,
) {
	writer := term.NewStringWriter(width, height)

	for i, tcase := range cases {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			handler := fn(t)
			handler.Resize(width, height)
			handleTestCase(t, i, writer, handler, tcase, width, height)
		})
	}
}

// TestHandlerSequence is a helper function that drives
// a set of HandlerSequenceTestCase and its results.
//
// Certain key events are encoded in characters. For instance, a '>' character
// signals term.KeyEnter and '<' character signals term.KeyEsc.
func TestHandlerSequence(
	t *testing.T, handler tui.Handler, width, height int,
	cases []HandlerSequenceTestCase,
) {
	writer := term.NewStringWriter(width, height)
	handler.Resize(width, height)

	for i, tcase := range cases {
		handleTestCase(t, i, writer, handler, tcase, width, height)
	}
}

// TestHandler is a helper function that tests a handler against
// a sequence of HandlerTestCase.
func TestHandler(
	t *testing.T, handler tui.Handler,
	cases []HandlerTestCase, w *term.StringWriter,
) {
	var err error

	for _, tcase := range cases {
		if err = w.Clear(term.Attributes{}); err != nil {
			t.Fatal(err)
		}

		handler.Handle(tcase.Event)

		handler.Draw(w)

		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}

		// for readability, we expected strings are written starting with \n
		expected := strings.TrimLeft(tcase.Expected, "\n")
		assert.Equal(t, expected, w.String())
	}
}
