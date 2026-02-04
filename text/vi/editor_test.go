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

package vi

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

func TestEditorDispatchFocus(t *testing.T) {
	cwd, err := workspaceapi.ParseURI("file:///")
	require.NoError(t, err)
	ed := Editor(
		// test wrapping/unwrapping of ifcs
		WithWorkspaceCommandRegistry(cwd, texttest.NopWorkspaceRegistry()),
	)
	content := "Clement"
	uri, err := workspaceapi.ParseURI("file:///Jolie")
	require.NoError(t, err)

	var h textapi.Handler
	ed.SubscribeEvents([]textapi.EventType{textapi.EventTypeOpen},
		text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			assert.Equal(t, textapi.EventTypeOpen, ev.Type)
			assert.Equal(t, content, ev.Content)
			assert.Equal(t, uri, ev.URI)
			h = ev.Resource
			return false
		}))

	var focusCalled int
	ed.SubscribeEvents([]textapi.EventType{textapi.EventTypeFocus},
		text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			focusCalled++
			assert.Equal(t, textapi.EventTypeFocus, ev.Type)
			assert.Equal(t, uri, ev.URI)
			assert.Equal(t, h, ev.Resource)
			return false
		}))

	buf := cell.NewBuffer()
	buf.WriteString(content)
	_, err = ed.Edit(uri, buf, false, false)
	require.NoError(t, err)

	assert.Equal(t, 1, focusCalled)
}

func TestEditorDispatchScroll(t *testing.T) {
	cfg := text.StatusBarConfig{
		Publisher: &texttest.TestEditor{},
		ScheduleNextTick: func(cb func()) bool {
			cb()
			return true
		},
	}
	ed := Editor(WithStatusBarConfig(true, cfg))
	buf := cell.NewBuffer()
	buf.WriteString("Atzari\nSurinach")
	h, err := ed.Edit(workspaceapi.URI{}, buf, false, false)
	require.NoError(t, err)

	h.Resize(2, 2)
	h.Handle(term.Event{Type: term.EventKey, Ch: 'j'})

	at := term.Coordinates{X: -1}
	ed.SubscribeEvents([]textapi.EventType{textapi.EventTypeScroll},
		text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			at = ev.Start
			return false
		}))

	h.Handle(term.Event{Type: term.EventKey, Ch: 'k'})
	assert.Equal(t, term.Coordinates{}, at)

	at = term.Coordinates{X: -1}
	h.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
	assert.Equal(t, term.Coordinates{Y: 1}, at)

	at = term.Coordinates{X: -1}
	h.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
	assert.Equal(t, term.Coordinates{X: -1}, at)
}

func TestEditorDispatchCursor(t *testing.T) {
	t.Run("regular handler-driven changes to cursor", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("file:///tmp/zsh.sh")
		require.NoError(t, err)
		ed := Editor()
		buf := cell.NewBuffer()
		buf.WriteString("Matias\nGiordano\n")
		h, err := ed.Edit(uri, buf, false, false)
		require.NoError(t, err)

		// should scroll as well, but changes in cursorAtScroll is what we are expecting
		h.Resize(2, 1)

		windowCursor := term.Coordinates{X: -1}
		scrollCursor := term.Coordinates{X: -1}
		ed.SubscribeEvents([]textapi.EventType{textapi.EventTypeCursor},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				windowCursor = ev.Start
				scrollCursor = ev.From
				assert.Equal(t, ev.URI.String(), "file:///tmp/zsh.sh")
				assert.Equal(t, ev.Resource, h)
				return false
			}))

		h.Handle(term.Event{Type: term.EventKey, Ch: 'k'})
		assert.Equal(t, term.Coordinates{X: -1}, windowCursor)
		assert.Equal(t, term.Coordinates{X: -1}, scrollCursor)

		h.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		assert.Equal(t, term.Coordinates{Y: 0}, windowCursor)
		assert.Equal(t, term.Coordinates{Y: 1}, scrollCursor)

		windowCursor = term.Coordinates{X: -1}
		scrollCursor = term.Coordinates{X: -1}
		h.Handle(term.Event{Type: term.EventKey, Ch: 'l'})
		assert.Equal(t, term.Coordinates{X: 1}, windowCursor)
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, scrollCursor)
	})

	t.Run("api-driven changes to cursor", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("file:///tmp/zsh.sh")
		require.NoError(t, err)
		ed := Editor()
		buf := cell.NewBuffer()
		buf.WriteString("Matias\nGiordano\n")
		h, err := ed.Edit(uri, buf, false, false)
		require.NoError(t, err)
		h.Resize(2, 1)

		windowCursor := term.Coordinates{X: -1}
		scrollCursor := term.Coordinates{X: -1}
		ed.SubscribeEvents([]textapi.EventType{textapi.EventTypeCursor},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				windowCursor = ev.Start
				scrollCursor = ev.From
				assert.Equal(t, ev.URI.String(), "file:///tmp/zsh.sh")
				assert.Equal(t, ev.Resource, h)
				return false
			}))

		require.True(t, h.SetCursorAtScroll(term.Coordinates{Y: 1}))
		assert.Equal(t, term.Coordinates{Y: 0}, windowCursor)
		assert.Equal(t, term.Coordinates{Y: 1}, scrollCursor)

		windowCursor = term.Coordinates{X: -1}
		scrollCursor = term.Coordinates{X: -1}
		h.SetLocationList(textapi.LocationPriorityInfo, "id",
			textapi.LocationSlice([]textapi.Location{{}, {From: term.Coordinates{Y: 1}}}))
		h.MoveToNextLocation("id")
		assert.Equal(t, term.Coordinates{Y: 0}, windowCursor)
		assert.Equal(t, term.Coordinates{Y: 0}, scrollCursor)

		windowCursor = term.Coordinates{X: -1}
		scrollCursor = term.Coordinates{X: -1}
		h.MoveToPrevLocation("id")
		assert.Equal(t, term.Coordinates{Y: 0}, windowCursor)
		assert.Equal(t, term.Coordinates{Y: 1}, scrollCursor)
	})
}

func TestEditorSetCursor(t *testing.T) {
	uri, err := workspaceapi.ParseURI("file:///tmp/zsh.sh")
	require.NoError(t, err)

	for _, wrap := range []bool{true, false} {
		t.Run(fmt.Sprintf("wrap: %v, does not return error if cursor already at position", wrap),
			func(t *testing.T) {
				ed := Editor(WithWrap(wrap), WithAutoCenter(true))
				h, err := ed.Edit(uri, cell.NewBuffer(), false, false)
				require.NoError(t, err)
				if wrap {
					h.Resize(1, 1) // just not 0, 0
				}

				h.SetCursorAtScroll(term.Coordinates{})
			})

		t.Run(fmt.Sprintf("wrap: %v, sets cursor at position", wrap),
			func(t *testing.T) {
				buf := cell.NewBuffer()
				buf.WriteString("a")
				ed := Editor(WithWrap(wrap), WithAutoCenter(true))
				h, err := ed.Edit(uri, buf, false, false)
				require.NoError(t, err)
				h.Resize(1, 1)

				h.SetCursorAtScroll(term.Coordinates{X: 1})
				pos := h.CursorAtScroll()
				require.NoError(t, err)
				assert.Equal(t, term.Coordinates{X: 1}, pos)
			})

		t.Run(fmt.Sprintf("wrap: %v, should be robust against Resize", wrap),
			func(t *testing.T) {
				buf := cell.NewBuffer()
				buf.WriteString("aaaaaaaaaaaaaaaa\nbb\nc\nd\ne")
				ed := Editor(WithWrap(wrap), WithAutoCenter(true))
				h, err := ed.Edit(uri, buf, false, false)
				require.NoError(t, err)
				cursor := h.(interface{ CursorReference() *text.Cursor }).CursorReference()

				h.SetCursorAtScroll(term.Coordinates{Y: 3})

				assert.Equal(t, term.Coordinates{}, cursor.Coordinates())
				assert.Equal(t, term.Coordinates{}, cursor.CursorAtScroll())

				h.Resize(1, 1)
				assert.Equal(t, term.Coordinates{}, cursor.Coordinates())
				assert.Equal(t, term.Coordinates{Y: 3}, cursor.CursorAtScroll())

				h.SetCursorAtScroll(term.Coordinates{Y: 4})
				require.NoError(t, err)

				h.Resize(10, 10)
				assert.Equal(t, term.Coordinates{Y: 0}, cursor.Coordinates())
				assert.Equal(t, term.Coordinates{Y: 4}, cursor.CursorAtScroll())

				h.Resize(2, 2)
				assert.Equal(t, term.Coordinates{Y: 0}, cursor.Coordinates())
				assert.Equal(t, term.Coordinates{Y: 4}, cursor.CursorAtScroll())
			})
	}
}
