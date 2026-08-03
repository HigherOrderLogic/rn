// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package gui

import (
	"context"
	"sync"
	"testing"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
)

func TestUpdate(t *testing.T) {
	t.Run("passes iteration in Draw context to root handler", func(t *testing.T) {
		var called int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			actualIteration, ok := tui.IterationFromContext(w.Context())
			require.True(t, ok)
			assert.Equal(t, int64(0), actualIteration)
			called++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)
	})

	t.Run("Update DOES call Draw if ebiten calls Layout with DIFFERENT height/width", func(t *testing.T) {
		var called int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			called++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		gui.Layout(1600, 900)

		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)
	})

	t.Run("Update DOES NOT calls Draw if ebiten calls Layout with SAME height/width", func(t *testing.T) {
		var called int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			called++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		gui.Layout(defaultWidth, defaultHeight)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)
	})

	t.Run("delegates events to handler", func(t *testing.T) {
		var expectedIterationID int64
		var called int
		mock := mockHandler{
			assertDraw: func(w term.Writer) {
				actualIteration, ok := tui.IterationFromContext(w.Context())
				require.True(t, ok)
				assert.Equal(t, expectedIterationID, actualIteration)
			},
			assertEvent: func(ev term.Event) (exit, handled bool) {
				called++
				assert.Equal(t, term.EventKey, ev.Type)
				assert.Equal(t, term.KeyEnter, ev.Key)
				return
			},
		}
		gui, input := newTestGUI(t, &mock)

		input.events = action(press(ebiten.KeyEnter))
		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		expectedIterationID++
		input.events = action(press(ebiten.KeyEnter, ebiten.KeyModSuper))
		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)

		input.events = nil
		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)
	})

	t.Run("delegates events to handler", func(t *testing.T) {
		var expectedIterationID int64
		var called int
		mock := mockHandler{
			assertDraw: func(w term.Writer) {
				actualIteration, ok := tui.IterationFromContext(w.Context())
				require.True(t, ok)
				assert.Equal(t, expectedIterationID, actualIteration)
			},
			assertEvent: func(ev term.Event) (exit, handled bool) {
				called++
				assert.Equal(t, term.EventKey, ev.Type)
				assert.Equal(t, term.KeyEnter, ev.Key)
				return
			},
		}
		gui, input := newTestGUI(t, &mock)

		input.events = action(press(ebiten.KeyEnter))
		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		expectedIterationID++
		input.events = action(press(ebiten.KeyEnter, ebiten.KeyModSuper))
		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)

		input.events = nil
		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)
	})

	t.Run("process interrupts by calling Draw, with reset context", func(t *testing.T) {
		var called int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			if called != 0 {
				_, ok := tui.IterationFromContext(w.Context())
				require.False(t, ok)

				_, ok = term.PayloadFromContext(w.Context())
				require.False(t, ok)
			}
			called++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		// simulate publish
		gui.pendingEvents = append(gui.pendingEvents, term.Event{Type: term.EventInterrupt})

		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)
	})

	t.Run("if interrupt contains .Raw payload, this is passed along in next call to Draw", func(t *testing.T) {
		var called int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			if called != 0 {
				payload, ok := term.PayloadFromContext(w.Context())
				require.True(t, ok)
				assert.Equal(t, "X1234", string(payload))
			}

			called++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		// simulate publish
		gui.pendingEvents = append(gui.pendingEvents, term.Event{
			Type: term.EventInterrupt,
			Raw:  []byte("X1234"),
		})

		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)
	})

	t.Run("if interrupt contains iteration ID, this is passed along in next call to Draw", func(t *testing.T) {
		var called int
		var actualDrawContext context.Context
		mock := mockHandler{assertDraw: func(w term.Writer) {
			if called != 0 {
				actualIterationID, ok := tui.IterationFromContext(w.Context())
				require.True(t, ok)
				assert.Equal(t, int64(0), actualIterationID)
			}

			actualDrawContext = w.Context()
			called++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		payload, ok := term.PayloadFromContext(actualDrawContext)
		require.True(t, ok)

		// simulate publish
		gui.pendingEvents = append(gui.pendingEvents, term.Event{
			Type: term.EventInterrupt,
			Raw:  payload,
		})

		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)
	})

	t.Run("if interrupt without iteration ID, mixed with regular event, calls Draw twice one with, one without iterationID", func(t *testing.T) {
		var called int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			switch called {
			case 0:
				actualIterationID, ok := tui.IterationFromContext(w.Context())
				require.True(t, ok)
				assert.Equal(t, int64(0), actualIterationID)
			case 1:
				_, ok := tui.IterationFromContext(w.Context())
				require.False(t, ok)
			case 2:
				actualIterationID, ok := tui.IterationFromContext(w.Context())
				require.True(t, ok)
				assert.Equal(t, int64(1), actualIterationID)
			}
			called++
		}, assertEvent: func(ev term.Event) (bool, bool) {
			return false, true
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		gui.pendingEvents = append(gui.pendingEvents, term.Event{
			Type: term.EventInterrupt,
		})
		gui.pendingEvents = append(gui.pendingEvents, term.Event{
			Type: term.EventKey,
			Ch:   'a',
			Raw:  []byte{'a'},
		})

		require.NoError(t, gui.Update())
		require.Equal(t, 3, called)
	})

	t.Run("if interrupt contains user function this is called before next call to Draw", func(t *testing.T) {
		var drawCalled int
		var userFnCalled int
		mock := mockHandler{assertDraw: func(w term.Writer) {
			if drawCalled != 0 {
				assert.Equal(t, 1, userFnCalled)
			}
			drawCalled++
		}}
		gui, _ := newTestGUI(t, &mock)

		require.NoError(t, gui.Update())
		require.Equal(t, 1, drawCalled)

		// simulate publish
		gui.pendingEvents = append(gui.pendingEvents, term.Event{
			Type: term.EventInterrupt,
			UserFunc: func() {
				userFnCalled++
			},
		})

		require.NoError(t, gui.Update())
		require.Equal(t, 2, drawCalled)
		assert.Equal(t, 1, userFnCalled)

		gui.updateChan <- term.Event{
			Type: term.EventInterrupt,
		}
		require.NoError(t, gui.Update())
		require.Equal(t, 3, drawCalled)
	})

	t.Run("returns ErrHandlerExited if handler exits", func(t *testing.T) {
		mock := mockHandler{
			assertEvent: func(ev term.Event) (bool, bool) {
				return true, true
			},
		}
		gui, input := newTestGUI(t, &mock)

		input.events = action(press(ebiten.KeyEnter))
		require.Equal(t, ErrHandlerExited, gui.Update())
	})
}

func TestRootHandlerSynchronization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var handler *mockHandler
	handler = &mockHandler{
		assertDraw: func(_ term.Writer) {
			_ = handler.width
			_ = handler.height
		},
		assertEvent: func(ev term.Event) (bool, bool) {
			_ = handler.width
			_ = handler.height
			return false, false
		},
	}
	gui, err := New(handler, WithLocker(&mu))
	require.NoError(t, err)

	// simulates a write, with Resize
	go func() {
		for {
			select {
			case <-ctx.Done():
			default:
			}
			mu.Lock()
			handler.Resize(105, 47)
			mu.Unlock()
		}
	}()

	// read via gui.Draw, Update, and Layout
	for n := 0; n < 1000; n++ {
		gui.Layout(1000, 1000)
		gui.Update()
		gui.Draw(ebiten.NewImage(1000, 1000))
	}

	// running with -race should result in no data races
}

func TestLayout(t *testing.T) {
	t.Run("does not panic on layout 0 width and height", func(t *testing.T) {
		mock := mockHandler{}
		gui, _ := newTestGUI(t, &mock)
		assert.NotPanics(t, func() {
			gui.Layout(0, 0)
		})
	})

	t.Run("resizes underlying handler if width/height are different", func(t *testing.T) {
		mock := mockHandler{}
		gui, _ := newTestGUI(t, &mock)

		mock.width = 0
		mock.height = 0
		gui.Layout(1200, 900)
		assert.NotZero(t, mock.width)
		assert.NotZero(t, mock.height)
	})

	t.Run("does not resize underlying handler if width/height are the same", func(t *testing.T) {
		mock := mockHandler{}
		gui, _ := newTestGUI(t, &mock)

		mock.width = 0
		mock.height = 0
		gui.Layout(defaultWidth, defaultHeight)
		assert.Zero(t, mock.width)
		assert.Zero(t, mock.height)
	})
}

// TestSetFontUnknownFamilyDoesNotPanic is a regression test for
// RUNE-51. Attempting to switch to a font family that either does not
// exist on the system or produces degenerate metrics must surface an
// error through SetFont instead of crashing the process in
// cell.NewBufferWriter, and must leave the GUI in a usable state so
// subsequent draws still work.
func TestSetFontUnknownFamilyDoesNotPanic(t *testing.T) {
	mock := mockHandler{}
	gui, _ := newTestGUI(t, &mock)

	previousWriter := gui.writer
	require.NotNil(t, previousWriter)

	var err error
	assert.NotPanics(t, func() {
		err = gui.SetFont("this-font-family-does-not-exist-RUNE-51")
	})
	assert.Error(t, err,
		"SetFont must return an error for an unknown family")

	// The GUI must stay functional: subsequent resizes and draws must
	// not panic, which means the font manager was restored to a
	// known-good state by the normal reload path.
	assert.NotPanics(t, func() {
		gui.resize(1200, 900, gui.fontManager.DeviceScale())
	})
}
func newTestGUI(t *testing.T, mock *mockHandler) (*GUI, *mockInputManager) {
	gui, err := New(mock)
	require.NoError(t, err)

	ret := &mockInputManager{}
	gui.input.input = ret

	return gui, ret
}

type mockHandler struct {
	width, height int
	assertDraw    func(term.Writer)
	assertEvent   func(term.Event) (bool, bool)
}

func (m *mockHandler) Resize(width, height int) {
	m.width, m.height = width, height
}

func (m *mockHandler) Draw(w term.Writer) {
	m.assertDraw(w)
}

func (m *mockHandler) Handle(ev term.Event) (exit, handled bool) {
	return m.assertEvent(ev)
}

func (m *mockHandler) Cursor() (c term.Coordinates, s term.CursorStyle, show bool) {
	return
}

func (m *mockHandler) Selection() (string, bool) {
	return "", false
}
