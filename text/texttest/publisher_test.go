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

package texttest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

func newEdit(content string) (*cell.Buffer, *component.Scroll, *text.Cursor) {
	buf := cell.NewBuffer()
	buf.WriteString(content)
	scroll := component.NewScroll(buf)
	cursor := text.NewCursor(scroll, nil)
	cursor.RightInclusiveSemantics = true
	return buf, scroll, cursor
}

func newMock(ctrl *gomock.Controller) *MockHandler {
	mock := NewMockHandler(ctrl)
	mock.EXPECT().Resize(gomock.Any(), gomock.Any()).AnyTimes()
	return mock
}

func TestPublisher(t *testing.T) {
	uri, err := workspaceapi.ParseURI("file:///Teamshares")
	require.NoError(t, err)
	t.Run("publishes EventTypeOpen when PublishEdit is called", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		pub := text.NewPublisher()
		content := "1. She interviews"

		var eventHandler textapi.Handler
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeOpen},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				assert.Equal(t, textapi.EventTypeOpen, ev.Type)
				assert.Equal(t, content, ev.Content)
				assert.Equal(t, uri, ev.URI)
				eventHandler = ev.Resource
				return false
			}))

		buf, _, cursor := newEdit(content)
		returnedHandler := pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		assert.Equal(t, eventHandler, returnedHandler)
	})

	t.Run("publishes EventTypeFocus when PublishEdit is called", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		pub := text.NewPublisher()
		content := "2. She gets hired"

		var eventHandler textapi.Handler
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeFocus},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				assert.Equal(t, textapi.EventTypeFocus, ev.Type)
				assert.Equal(t, uri, ev.URI)
				eventHandler = ev.Resource
				return false
			}))

		buf, _, cursor := newEdit(content)
		returnedHandler := pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		assert.Equal(t, eventHandler, returnedHandler)
	})

	t.Run("subscribes to multiple events", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		pub := text.NewPublisher()
		content := "3. SHE GOT HIRED, I KNEW IT!!!"

		var eventHandler textapi.Handler
		var fired int
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeOpen, textapi.EventTypeFocus},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				fired++
				assert.Equal(t, uri, ev.URI)
				eventHandler = ev.Resource
				return false
			}))

		buf, _, cursor := newEdit(content)
		returnedHandler := pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		assert.Equal(t, eventHandler, returnedHandler)
		assert.Equal(t, 2, fired)
	})

	t.Run("dispatches EventTypeCursor when given cursor changes on Handle", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pub := text.NewPublisher()
		mock := newMock(ctrl)
		buf, scroll, cursor := newEdit("a\nb\n\nc")

		h := pub.PublishEdit(uri, buf, mock, cursor)
		scroll.Resize(2, 2)
		h.Resize(2, 2)

		var called bool
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeCursor},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				require.Equal(t, textapi.EventTypeCursor, ev.Type)
				assert.Equal(t, uri, ev.URI)
				assert.Equal(t, h, ev.Resource)
				assert.Equal(t, term.Coordinates{Y: 1}, ev.Start)
				assert.Equal(t, term.Coordinates{Y: 2}, ev.From)
				called = true
				return false
			}))

		mock.EXPECT().Handle(gomock.Any()).
			DoAndReturn(func(ev term.Event) (bool, bool) {
				require.Equal(t, term.Event{Type: term.EventInterrupt}, ev)
				require.True(t, cursor.MoveDown())
				require.True(t, cursor.MoveDown())
				return false, true
			}).Times(1)

		exit, handled := h.Handle(term.Event{Type: term.EventInterrupt})
		assert.False(t, exit)
		assert.True(t, handled)
		assert.True(t, called)
	})

	t.Run("dispatches EventTypeSelection when cursor changes and selection is on", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pub := text.NewPublisher()
		mock := newMock(ctrl)
		buf, scroll, cursor := newEdit("aa\nbb\n")

		h := pub.PublishEdit(uri, buf, mock, cursor)
		scroll.Resize(2, 2)
		h.Resize(2, 2)

		var called bool
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeSelection},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				require.Equal(t, textapi.EventTypeSelection, ev.Type)
				assert.Equal(t, uri, ev.URI)
				assert.Equal(t, h, ev.Resource)
				assert.Equal(t, term.Coordinates{Y: 0}, ev.Start)
				assert.Equal(t, term.Coordinates{Y: 1, X: 1}, ev.End)
				assert.Equal(t, "aa\nb", ev.Content)
				called = true
				return false
			}))

		mock.EXPECT().Handle(gomock.Any()).
			DoAndReturn(func(ev term.Event) (bool, bool) {
				require.Equal(t, term.Event{Type: term.EventInterrupt}, ev)
				require.True(t, cursor.MoveDown())
				return false, true
			}).Times(1)

		require.True(t, cursor.Select())

		exit, handled := h.Handle(term.Event{Type: term.EventInterrupt})
		assert.False(t, exit)
		assert.True(t, handled)
		assert.True(t, called)
	})

	t.Run("dispatches EventTypeSelection when selection goes from on to off", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pub := text.NewPublisher()
		mock := newMock(ctrl)
		buf, scroll, cursor := newEdit("aa\nbb\n")

		h := pub.PublishEdit(uri, buf, mock, cursor)
		scroll.Resize(2, 2)
		h.Resize(2, 2)

		var called int
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeSelection},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				require.Equal(t, textapi.EventTypeSelection, ev.Type)
				assert.Equal(t, uri, ev.URI)
				assert.Equal(t, h, ev.Resource)
				if called == 0 {
					assert.Equal(t, term.Coordinates{Y: 0}, ev.Start)
					assert.Equal(t, term.Coordinates{Y: 1, X: 1}, ev.End)
					assert.Equal(t, "aa\nb", ev.Content)
				} else {
					assert.Equal(t, term.Coordinates{Y: 0}, ev.Start)
					assert.Equal(t, term.Coordinates{Y: 1, X: 1}, ev.End)
					assert.Equal(t, "", ev.Content)
				}
				called++
				return false
			}))

		var times int
		mock.EXPECT().Handle(gomock.Any()).
			DoAndReturn(func(ev term.Event) (bool, bool) {
				require.Equal(t, term.Event{Type: term.EventInterrupt}, ev)
				if times == 0 {
					require.True(t, cursor.MoveDown())
				}
				if times == 1 {
					require.True(t, cursor.Unselect())
				}
				times++
				return false, true
			}).Times(2)

		require.True(t, cursor.Select())

		exit, handled := h.Handle(term.Event{Type: term.EventInterrupt})
		assert.False(t, exit)
		assert.True(t, handled)

		exit, handled = h.Handle(term.Event{Type: term.EventInterrupt})
		assert.False(t, exit)
		assert.True(t, handled)

		assert.Equal(t, 2, called)
	})

	t.Run("dispatches EventTypeScroll when scroll seeks", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		pub := text.NewPublisher()
		buf, scroll, cursor := newEdit("\n\n")

		h := pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		h.Resize(2, 2)
		require.True(t, scroll.SeekDown())

		at := term.Coordinates{X: -1}
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeScroll},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				require.Equal(t, textapi.EventTypeScroll, ev.Type)
				assert.Equal(t, uri, ev.URI)
				assert.Equal(t, h, ev.Resource)
				at = ev.Start
				return false
			}))

		require.True(t, scroll.SeekUp())
		assert.Equal(t, term.Coordinates{}, at)

		at = term.Coordinates{X: -1}
		require.True(t, scroll.SeekDown())
		assert.Equal(t, term.Coordinates{Y: 1}, at)
	})

	t.Run("dispatches EventTypeEdit when buffer content is inserted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		pub := text.NewPublisher()
		buf, _, cursor := newEdit("\n\n")

		h := pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		h.Resize(2, 2)

		var called bool
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeEdit},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				called = true
				require.Equal(t, textapi.EventTypeEdit, ev.Type)
				assert.Equal(t, term.Coordinates{Y: 2}, ev.Start, "Start")
				assert.Equal(t, term.Coordinates{Y: 2}, ev.End, "End")
				assert.Equal(t, term.Coordinates{Y: 2}, ev.From, "From")
				assert.Equal(t, term.Coordinates{Y: 2, X: 9}, ev.To, "To")
				assert.Equal(t, uri, ev.URI)
				assert.Equal(t, h, ev.Resource)
				assert.Equal(t, "blah\tbleh", ev.Content)
				return false
			}))

		buf.WriteString("blah\tbleh")
		require.True(t, called)
	})

	t.Run("dispatches EventTypeEdit when buffer content is deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		pub := text.NewPublisher()
		buf, _, cursor := newEdit("aaa\nbbb")

		h := pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		h.Resize(2, 2)

		var called bool
		pub.SubscribeEvents([]textapi.EventType{textapi.EventTypeEdit},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				called = true
				require.Equal(t, textapi.EventTypeEdit, ev.Type)
				assert.Equal(t, term.Coordinates{}, ev.Start, "Start")
				assert.Equal(t, term.Coordinates{Y: 1}, ev.End, "End")
				assert.Equal(t, term.Coordinates{}, ev.From, "From")
				assert.Equal(t, term.Coordinates{}, ev.To, "To")
				assert.Equal(t, uri, ev.URI)
				assert.Equal(t, h, ev.Resource)
				assert.Equal(t, "", ev.Content)
				return false
			}))

		buf.DeleteLine(term.Coordinates{}, term.Coordinates{})
		require.True(t, called)
	})

	t.Run("unsubscribes one handler", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		pub := text.NewPublisher()
		content := "1. She interviews"

		var one, two, three int
		handler1 := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			one++
			return false
		})
		handler2 := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			two++
			return false
		})
		handler3 := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			three++
			return false
		})
		evs := []textapi.EventType{textapi.EventTypeOpen, textapi.EventTypeEdit}
		pub.SubscribeEvents(evs, handler1)
		pub.SubscribeEvents(evs, handler2)
		pub.SubscribeEvents(evs, handler3)

		buf, _, cursor := newEdit(content)
		pub.PublishEdit(uri, buf, newMock(ctrl), cursor)
		buf.DeleteLine(term.Coordinates{}, term.Coordinates{})
		buf.DeleteLine(term.Coordinates{}, term.Coordinates{})
		assert.Equal(t, 3, one)
		assert.Equal(t, 3, two)
		assert.Equal(t, 3, three)

		assert.True(t, pub.UnsubscribeEvents(handler1))
		buf.DeleteLine(term.Coordinates{}, term.Coordinates{})
		assert.Equal(t, 3, one)
		assert.Equal(t, 4, two)
		assert.Equal(t, 4, three)

		// make sure that two remains subscribed to other events
		buf.DeleteLine(term.Coordinates{}, term.Coordinates{})
		assert.Equal(t, 3, one)
		assert.Equal(t, 5, two)
		assert.Equal(t, 5, three)
	})
}
