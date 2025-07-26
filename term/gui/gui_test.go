// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
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
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
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

		input.pressedKeys[ebiten.KeyEnter] = struct{}{}
		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		expectedIterationID++
		input.pressedKeys[ebiten.KeyEnter] = struct{}{}
		input.pressedKeys[ebiten.KeyMeta] = struct{}{}
		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)

		delete(input.pressedKeys, ebiten.KeyMeta)
		delete(input.pressedKeys, ebiten.KeyEnter)
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

		input.pressedKeys[ebiten.KeyEnter] = struct{}{}
		require.NoError(t, gui.Update())
		require.Equal(t, 1, called)

		expectedIterationID++
		input.pressedKeys[ebiten.KeyEnter] = struct{}{}
		input.pressedKeys[ebiten.KeyMeta] = struct{}{}
		require.NoError(t, gui.Update())
		require.Equal(t, 2, called)

		delete(input.pressedKeys, ebiten.KeyMeta)
		delete(input.pressedKeys, ebiten.KeyEnter)
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
		require.Equal(t, 1, drawCalled)
		assert.Equal(t, 1, userFnCalled)

		// test that interrupts continue to work work after a UserFunc interrupt
		go gui.consumeEvents()
		gui.updateChan <- term.Event{
			Type: term.EventInterrupt,
		}
		for { // wait until event has been queued
			gui.mu.Lock()
			lenPendingEvents := len(gui.pendingEvents)
			gui.mu.Unlock()
			if lenPendingEvents == 1 {
				break
			}
		}
		require.NoError(t, gui.Update())
		require.Equal(t, 2, drawCalled)
	})

	t.Run("returns ErrHandlerExited if handler exits", func(t *testing.T) {
		mock := mockHandler{
			assertEvent: func(ev term.Event) (bool, bool) {
				return true, true
			},
		}
		gui, input := newTestGUI(t, &mock)

		input.pressedKeys[ebiten.KeyEnter] = struct{}{}
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

func newTestGUI(t *testing.T, mock *mockHandler) (*GUI, *mockInputManager) {
	gui, err := New(mock)
	require.NoError(t, err)

	ret := &mockInputManager{pressedKeys: make(map[ebiten.Key]struct{})}
	gui.input.input = ret
	gui.input.keyPressDelay = 0
	gui.input.keyPressRepeat = 0

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

func (m *mockHandler) Man() tui.Manual {
	return tui.Manual{}
}
