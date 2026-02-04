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

package textrpc

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi/browserrpc"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	tbrowserrpc "unstable.build/go-tui/browser/browserrpc"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/workspace"
)

func TestClientServerIntegration(t *testing.T) {
	uri, err := workspaceapi.ParseURI("file:///test")
	require.NoError(t, err)
	t.Run("client through server calls underlying editor Edit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, nopLocker{})

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		expectEdit(t, ed, uri, "hero", false, false)
		buf := cell.NewBuffer()
		buf.WriteString("hero")

		_, err := client.Edit(uri, buf, false, false)
		require.NoError(t, err)
	})

	t.Run("client passes readOnly and recover params", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, nopLocker{})

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		expectEdit(t, ed, uri, "hero", true, true)
		buf := cell.NewBuffer()
		buf.WriteString("hero")
		_, err := client.Edit(uri, buf, true, true)
		require.NoError(t, err)
	})

	t.Run("client through server calls underlying Editor", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, nopLocker{})

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		ed.EXPECT().Editor(gomock.Any()).Times(1).
			DoAndReturn(func(_uri workspaceapi.URI) (tui.Handler, error) {
				assert.Equal(t, _uri, uri)
				return handler.NewTestHandler(), nil
			})
		_, err := client.Editor(uri)
		require.NoError(t, err)
	})

	t.Run("underlying editor Edito errors bubble up to client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, nopLocker{})

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		ed.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("The Upsetter")).
			Times(1)

		_, err := client.Edit(uri, cell.NewBuffer(), false, false)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "The Upsetter"))
	})

	t.Run("client through server calls underlying editor Subscribe", func(t *testing.T) {
		str1 := "Granola Lola"
		tsuite := []struct {
			name       string
			evType     textapi.EventType
			trigger    func(t *testing.T, resourceName string, ed text.Editor, buf *cell.Buffer)
			start, end *term.Coordinates
			content    *string
		}{
			{
				"Edit->EventTypeOpen",
				textapi.EventTypeOpen,
				func(t *testing.T, resourceName string, ed text.Editor, buf *cell.Buffer) {
					ed.Edit(uri, buf, false, false)
				}, nil, nil, nil,
			},
			{
				"Edit->EventTypeEdit",
				textapi.EventTypeEdit,
				func(t *testing.T, resourceName string, ed text.Editor, buf *cell.Buffer) {
					ed.Edit(uri, buf, false, false)
					buf.WriteString(str1)
				}, &term.Coordinates{}, &term.Coordinates{}, &str1,
			},
			{
				"Edit->EventTypeEdit",
				textapi.EventTypeEdit,
				func(t *testing.T, resourceName string, ed text.Editor, buf *cell.Buffer) {
					buf.WriteString(str1)
					ed.Edit(uri, buf, false, false)
					buf.DeleteRow(0)
				}, &term.Coordinates{}, &term.Coordinates{Y: 1}, nil,
			},
			{
				"Handle->EventTypeCursor",
				textapi.EventTypeCursor,
				func(t *testing.T, resourceName string, ed text.Editor, buf *cell.Buffer) {
					buf.WriteString(str1)
					h, err := ed.Edit(uri, buf, false, false)
					assert.NoError(t, err)
					h.Handle(term.Event{Ch: 'l'})
				}, &term.Coordinates{}, &term.Coordinates{}, nil,
			},
			{
				"Handle->EventTypeSelection",
				textapi.EventTypeSelection,
				func(t *testing.T, resourceName string, ed text.Editor, buf *cell.Buffer) {
					buf.WriteString(str1)
					h, err := ed.Edit(uri, buf, false, false)
					assert.NoError(t, err)
					h.Handle(term.Event{Ch: 'v'})
				}, &term.Coordinates{}, &term.Coordinates{}, nil,
			},
		}

		for i, _tcase := range tsuite {
			tcase := _tcase
			t.Run(tcase.name, func(t *testing.T) {
				var wg sync.WaitGroup
				ed := texttest.NopEditorWithCallback(wg.Done)
				s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

				client, closeFn := setupIntTest(t, s)
				defer closeFn()

				wg.Add(1)
				err := client.SubscribeEvents([]textapi.EventType{tcase.evType},
					text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
						defer wg.Done()
						if tcase.start != nil {
							assert.Equal(t, *tcase.start, ev.Start)
						}
						if tcase.end != nil {
							assert.Equal(t, *tcase.end, ev.End)
						}
						if tcase.content != nil {
							assert.Equal(t, *tcase.content, ev.Content)
						}
						return false
					}))
				require.NoError(t, err)

				// wait for subscribe callback
				wg.Wait()

				// proceed to trigger
				wg.Add(1)

				buf := cell.NewBuffer()
				tcase.trigger(t, strconv.Itoa(i), ed, buf)
				wg.Wait()

				s.editor.Lock()
				defer s.editor.Unlock()

				assert.NoError(t, s.Close())
			})
		}
	})

	t.Run("calls unsubscribe if event stream completes", func(t *testing.T) {
		var wg sync.WaitGroup
		ed := texttest.NopEditorWithCallback(wg.Done)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		wg.Add(1)
		err := client.SubscribeEvents([]textapi.EventType{textapi.EventTypeOpen},
			text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				defer wg.Done()
				return true
			}))
		require.NoError(t, err)

		// wait for subscribe callback
		wg.Wait()

		// proceed to trigger
		ed.Edit(uri, cell.NewBuffer(), false, false)

		// wg panics if Done called but not added
		wg.Wait()

		// close
		s.editor.Lock()
		defer s.editor.Unlock()

		assert.NoError(t, s.Close())
	})

	t.Run("event handler drops messages if event handler server is not processing events", func(t *testing.T) {
		var wg sync.WaitGroup
		ed := texttest.NopEditorWithCallback(wg.Done)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		var mu sync.Mutex
		evs := make(map[textapi.EventType]textapi.Event)

		wg.Add(1)
		err := client.SubscribeEvents([]textapi.EventType{
			textapi.EventTypeOpen,
			textapi.EventTypeClose,
			textapi.EventTypeFlush,
			textapi.EventTypeEdit,
			textapi.EventTypeScroll,
			textapi.EventTypeFocus,
			textapi.EventTypeUnfocus,
			textapi.EventTypeCursor,
			textapi.EventTypeSelection,
		}, text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			mu.Lock()
			defer mu.Unlock()
			evs[ev.Type] = ev
			return false
		}))
		require.NoError(t, err)

		// wait for subscribe callback, which also uses wg
		wg.Wait()

		subs := ed.Subscribers()
		require.Len(t, subs, 9)
		require.Len(t, subs[textapi.EventTypeOpen], 1)
		handler := subs[textapi.EventTypeOpen][0]

		// proceed to trigger, should not deadlock
		mu.Lock() // try to deadlock
		for i := 0; i < 10000; i++ {
			tpe := textapi.EventType(i % 9)
			handler.Handle(context.Background(), textapi.Event{URI: uri, Type: tpe})
		}

		mu.Unlock()
		// there's no deterministic way to know how many msgs will
		// be buffered by the underlying transport, so this is the only way
		time.Sleep(5 * time.Second)
		mu.Lock()

		require.Len(t, evs, 9)
		s.editor.Lock()
		defer s.editor.Unlock()

		assert.NoError(t, s.Close())
	})

	t.Run("SetLocationList sets the location list of the remote editor", func(t *testing.T) {
		var wg sync.WaitGroup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		expectEdit(t, ed, uri, "", false, false)
		h, err := client.Edit(uri, cell.NewBuffer(), false, false)
		require.NoError(t, err)

		l := text.LocationSlice([]textapi.Location{loc2})

		mock := expectEditor(t, ctrl, ed, uri)
		mock.EXPECT().SetLocationList(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(pri textapi.LocationPriority, id string, ll text.LocationList) {
				defer wg.Done()
				assertLocation(t, ll, 0, loc2)
				assert.Equal(t, locID, id)
				assertLocationListLen(t, ll, 1)
				assert.Equal(t, textapi.LocationPriorityError, pri)
			}).Times(1)

		wg.Add(1)
		err = client.SetLocationList(h, textapi.LocationPriorityError, locID, l)
		require.NoError(t, err)

		wg.Wait()
	})

	t.Run("SetDefaultAttributes sets the default attrs of the remote editor", func(t *testing.T) {
		var wg sync.WaitGroup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		expectEdit(t, ed, uri, "", false, false)
		h, err := client.Edit(uri, cell.NewBuffer(), false, false)
		require.NoError(t, err)

		expectedAttrs := term.Attributes{
			Attrs: tcell.AttrUnderline | tcell.AttrBold,
			Fg:    tcell.ColorWhite,
			Bg:    tcell.ColorNavy,
		}

		mock := expectEditor(t, ctrl, ed, uri)
		mock.EXPECT().SetDefaultAttributes(gomock.Any()).
			DoAndReturn(func(attrs term.Attributes) {
				defer wg.Done()
				assert.Equal(t, expectedAttrs, attrs)
			}).Times(1)

		wg.Add(1)
		err = client.SetDefaultAttributes(h, expectedAttrs)
		require.NoError(t, err)

		wg.Wait()
	})

	t.Run("Writer returns a Writer that is able to modify underlying buffer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, nopLocker{})

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		buf := cell.NewBuffer()
		expectEdit(t, ed, uri, "", false, false)
		h, err := client.Edit(uri, buf, false, false)
		require.NoError(t, err)

		w := client.CellEditor(h)
		at := term.Coordinates{X: 1}

		mock := expectEditor(t, ctrl, ed, uri)
		mock.EXPECT().CellEditor().Return(buf.Editor()).Times(1)
		from, to, _, err := w.Edit(context.Background(), at, at, "el\nAridio")

		require.NoError(t, err)
		require.Equal(t, term.Coordinates{}, from)
		require.Equal(t, term.Coordinates{X: 6, Y: 1}, to)
		require.Equal(t, " el\nAridio", buf.String())

		mock.EXPECT().CellEditor().Return(buf.Editor()).Times(1)
		start, end, str, err := w.Edit(
			context.Background(), term.Coordinates{}, term.Coordinates{Y: 1}, "")

		require.NoError(t, err)
		assert.Equal(t, term.Coordinates{}, start)
		assert.Equal(t, term.Coordinates{}, end)
		assert.Equal(t, " el\n", str)
		assert.Equal(t, "Aridio", buf.String())
	})

	t.Run("Reader returns a Reader that is able to read underlying buffer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, nopLocker{})

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		buf := cell.NewBuffer()
		buf.WriteString("guacamole")
		expectEdit(t, ed, uri, "guacamole", false, false)
		h, err := client.Edit(uri, buf, false, false)
		require.NoError(t, err)

		mock := expectEditor(t, ctrl, ed, uri)
		mock.EXPECT().CellView().Return(buf.View()).Times(2)

		r := client.CellView(h)
		cells, err := r.RawCells()
		require.NoError(t, err)
		assert.Equal(t, "guacamole", term.CellsToString(cells))

		buf.WriteString("\npollos hermanos")

		cells, err = r.RawCells()
		require.NoError(t, err)
		assert.Equal(t, "guacamole\npollos hermanos", term.CellsToString(cells))
	})

	t.Run("dispatches commands to subscribed command handler", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))
		var wg sync.WaitGroup

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		var (
			dispatched       textapi.Command
			subscribed       textapi.CommandManual
			subscribedTimes  int
			subscribedClient text.CommandHandler
		)
		handler := textapi.FuncCommandHandler(func(_ context.Context, man textapi.Command) error {
			dispatched = man
			wg.Done()
			return nil
		}, nil)

		man := textapi.CommandManual{
			Name:     "bla",
			Synopsis: "blabla",
			Commands: []textapi.CommandManual{
				{
					Name:     "ble",
					Synopsis: "bleble",
				},
			},
		}
		ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).DoAndReturn(
			func(man textapi.CommandManual, h text.CommandHandler) error {
				subscribed = man
				subscribedTimes++
				subscribedClient = h
				return nil
			})
		err := client.SubscribeCommand(man, handler)
		require.NoError(t, err)

		require.Equal(t, 1, subscribedTimes)
		assert.Equal(t, "bla", subscribed.Name)
		assert.Equal(t, "blabla", subscribed.Synopsis)
		require.Len(t, subscribed.Commands, 1)
		assert.Equal(t, "ble", subscribed.Commands[0].Name)
		assert.Equal(t, "bleble", subscribed.Commands[0].Synopsis)
		require.NotNil(t, subscribedClient)

		// dispatch
		resource := texttest.NewTestHandler()
		resource.URI = uri
		command := textapi.Command{
			Name:     "bla",
			Args:     []string{"ble"},
			URI:      uri,
			Resource: resource,
			Window:   browserrpc.NewWindow(199),
		}
		command.Cursor.Content = term.Coordinates{X: 1, Y: 2}
		command.Cursor.Window = term.Coordinates{X: 3, Y: 4}
		s.editor.Lock()

		wg.Add(1)
		err = subscribedClient.HandleCommand(context.Background(), command)
		s.editor.Unlock()
		require.NoError(t, err)

		wg.Wait()
		assert.Equal(t, "bla", dispatched.Name)
		require.Len(t, dispatched.Args, 1)
		assert.Equal(t, "ble", dispatched.Args[0])
		assert.Equal(t, "file:///test", dispatched.URI.String())
		assert.Equal(t, uint64(199), dispatched.Window.WindowID())
		assert.Equal(t, "file:///test", dispatched.Resource.Resource().String())

		waitForReplaceCommand(t, &wg, ed, client, "bla")
	})

	t.Run("handles command handler error by notifying user", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		noti := browsertest.NewMockNotifications(ctrl)
		s := NewServer(noti, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		var (
			subscribedClient text.CommandHandler
			wg               sync.WaitGroup
		)
		handler := textapi.FuncCommandHandler(func(_ context.Context, man textapi.Command) error {
			return errors.New("boom")
		}, nil)

		man := textapi.CommandManual{Name: "bla"}
		ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).DoAndReturn(
			func(man textapi.CommandManual, h text.CommandHandler) error {
				subscribedClient = h
				return nil
			})
		err := client.SubscribeCommand(man, handler)
		require.NoError(t, err)

		require.NotNil(t, subscribedClient)

		// dispatch
		resource := texttest.NewTestHandler()
		resource.URI = uri
		command := textapi.Command{Name: "bla"}
		wg.Add(1)
		noti.EXPECT().Notify(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(func(browserapi.NotificationLevel, string, ...any) (string, error) {
				wg.Done()
				return "", nil
			})
		s.editor.Lock()
		err = subscribedClient.HandleCommand(context.Background(), command)
		s.editor.Unlock()
		require.NoError(t, err)

		wg.Wait() // notify was called
		waitForReplaceCommand(t, &wg, ed, client, "bla")
	})

	t.Run("returns subscribe error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		handler := textapi.FuncCommandHandler(func(_ context.Context, man textapi.Command) error {
			return nil
		}, nil)

		man := textapi.CommandManual{Name: "bla"}
		ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).DoAndReturn(
			func(man textapi.CommandManual, h text.CommandHandler) error {
				return errors.New("boom")
			})
		err := client.SubscribeCommand(man, handler)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("completes commands", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		var (
			subscribedClient text.CommandHandler
			dispatched       string
			dispatchedArgs   []string
			dispatchedTimes  int
		)
		expectedIterator := iterator.FromSlice([]string{"a", "b", "c"})
		handler := textapi.FuncCommandHandler(
			func(_ context.Context, man textapi.Command) error {
				return nil
			}, func(_ context.Context, cmd string, args []string) (
				iterator.Iterator[string], error,
			) {
				dispatched = cmd
				dispatchedArgs = args
				dispatchedTimes++
				return expectedIterator, nil
			})

		man := textapi.CommandManual{Name: "bla"}
		ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).DoAndReturn(
			func(man textapi.CommandManual, h text.CommandHandler) error {
				subscribedClient = h
				return nil
			})
		err := client.SubscribeCommand(man, handler)
		require.NoError(t, err)

		require.NotNil(t, subscribedClient)

		// complete
		s.editor.Lock()
		it, _, err := subscribedClient.Complete(context.Background(), textapi.Command{Name: "bla", Args: []string{"ble"}})
		s.editor.Unlock()
		require.NoError(t, err)

		actualIterator, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Equal(t, actualIterator, []string{"a", "b", "c"})

		require.Equal(t, 1, dispatchedTimes)
		assert.Equal(t, "bla", dispatched)
		require.Len(t, dispatchedArgs, 1)
		assert.Equal(t, "ble", dispatchedArgs[0])

		var wg sync.WaitGroup
		waitForReplaceCommand(t, &wg, ed, client, "bla")
	})

	t.Run("returns complete error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		var (
			subscribedClient text.CommandHandler
		)
		handler := textapi.FuncCommandHandler(
			func(_ context.Context, man textapi.Command) error {
				return nil
			}, func(_ context.Context, cmd string, args []string) (
				iterator.Iterator[string], error,
			) {
				return nil, errors.New("boom")
			})

		man := textapi.CommandManual{Name: "bla"}
		ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).DoAndReturn(
			func(man textapi.CommandManual, h text.CommandHandler) error {
				subscribedClient = h
				return nil
			})
		err := client.SubscribeCommand(man, handler)
		require.NoError(t, err)

		require.NotNil(t, subscribedClient)

		s.editor.Lock()
		it, _, err := subscribedClient.Complete(context.Background(),
			textapi.Command{Name: "bla", Args: []string{"ble"}})
		s.editor.Unlock()
		require.NoError(t, err)

		_, err = iterator.ToSlice(context.Background(), it)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")

		var wg sync.WaitGroup
		waitForReplaceCommand(t, &wg, ed, client, "bla")
	})

	t.Run("returns iterator error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		s := NewServer(nopNotifications{}, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, s)
		defer closeFn()

		var (
			subscribedClient text.CommandHandler
		)
		handler := textapi.FuncCommandHandler(
			func(_ context.Context, man textapi.Command) error {
				return nil
			}, func(_ context.Context, cmd string, args []string) (
				iterator.Iterator[string], error,
			) {
				return iterator.Error[string](errors.New("boom")), nil
			})

		man := textapi.CommandManual{Name: "bla"}
		ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).DoAndReturn(
			func(man textapi.CommandManual, h text.CommandHandler) error {
				subscribedClient = h
				return nil
			})
		err := client.SubscribeCommand(man, handler)
		require.NoError(t, err)

		require.NotNil(t, subscribedClient)

		s.editor.Lock()
		it, _, err := subscribedClient.Complete(context.Background(),
			textapi.Command{Name: "bla", Args: []string{"ble"}})
		s.editor.Unlock()
		require.NoError(t, err)

		_, err = iterator.ToSlice(context.Background(), it)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")

		var wg sync.WaitGroup
		waitForReplaceCommand(t, &wg, ed, client, "bla")
	})
}

func waitForReplaceCommand(
	t *testing.T, wg *sync.WaitGroup,
	ed *texttest.MockEditor, client *Client, expectCmd string,
) {
	wg.Add(1)
	ed.EXPECT().UnsubscribeCommand(gomock.Any()).DoAndReturn(
		func(cmd string) error {
			assert.Equal(t, expectCmd, cmd)
			wg.Done()
			return nil
		})
	ed.EXPECT().SubscribeCommand(gomock.Any(), gomock.Any()).Return(nil)
	require.NoError(t, client.Close())
	wg.Wait()
}

func TestRPCTab(t *testing.T) {
	testTabIntegration(t, func(ed text.Editor, mu *sync.Mutex) (*text.Component, browserapi.WindowManager, error) {
		c, err := newTestComponentErr(ed)
		if err != nil {
			return nil, nil, err
		}

		s := tbrowserrpc.NewServer(c, mu)
		s.SetSyncMode()

		client, closeFn := setupWmIntTest(t, s)
		t.Cleanup(func() {
			mu.Lock()
			s.Stop()
			mu.Unlock()
			closeFn()
		})

		return c, client, err
	})
}

func assertLocation(t *testing.T, l text.LocationList, idx int, loca textapi.Location) {
	resetLocationList(l)
	var i int
	for loc, ok := l.Current(); ok; loc, ok = l.Next() {
		if idx == i {
			assert.Equal(t, loca, loc)
			return
		}
		i++
	}
}
func assertLocationListLen(t *testing.T, l text.LocationList, length int) {
	resetLocationList(l)
	var i int
	for _, ok := l.Current(); ok; _, ok = l.Next() {
		i++
	}
	assert.Equal(t, length, i)
}

func resetLocationList(l text.LocationList) {
	for {
		_, ok := l.Prev()
		if !ok {
			break
		}
	}
}

func testTabIntegration(t *testing.T,
	constructor func(ed text.Editor, mu *sync.Mutex) (*text.Component, browserapi.WindowManager, error)) {
	t.Run("switches to a tab upon call to SetContent", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{"",
				`┌──────────────────┐
│x $$  x ##        │
├──────────────────┤
│##################│
│##################│
│##################│
│##################│
│##################│
│##################│
└──────────────────┘`},
		}

		fn := func(t *testing.T) tui.Handler {
			var mu sync.Mutex
			c, wm, err := constructor(texttest.NopEditor(), &mu)
			require.NoError(t, err)

			resource1, err := workspaceapi.ParseURI("file:///a")
			require.NoError(t, err)
			resource2, err := workspaceapi.ParseURI("file:///b")
			require.NoError(t, err)
			b1 := browsertest.NewTestHandler()
			b1.TestHandler.Ch = '$'
			_, err = wm.Tab(resource1, 'x', "$$", b1)
			require.NoError(t, err)

			b2 := browsertest.NewTestHandler()
			b2.TestHandler.Ch = '#'
			t2, err := wm.Tab(resource2, 'x', "##", b2)
			require.NoError(t, err)

			win, err := wm.Focus()
			require.NoError(t, err)

			require.NoError(t, wm.SetWindowContent(win, t2))
			return handler.Sync(&mu, handler.NopFromComponent(c.Browser()))
		}
		handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
	})
}

type testLoader struct {
	content       string
	flusherCloser *testFlusherCloser
	expectError   error
}

type testFlusherCloser struct {
	closeFn   func() error
	flushFn   func() error
	lastFlush time.Time
}

func (t *testFlusherCloser) Close() error {
	if t.closeFn != nil {
		return t.closeFn()
	}
	return nil
}
func (t *testFlusherCloser) Reload() error {
	return nil
}
func (t *testFlusherCloser) Flush() error {
	t.lastFlush = time.Now()
	if t.flushFn != nil {
		return t.flushFn()
	}
	return nil
}

func (t *testFlusherCloser) ForceFlush() error {
	panic("unimplemented")
}

func (t *testFlusherCloser) LastFlush() time.Time {
	return t.lastFlush
}

func (t *testLoader) Remove(string) error {
	return nil
}

func (t *testLoader) Load(
	file workspaceapi.URI, buf *cell.Buffer, swapDir workspaceapi.URI, readOnly bool,
) (workspace.FlusherCloser, error) {
	if t.expectError != nil {
		return nil, t.expectError
	}
	if t.flusherCloser != nil {
		return t.flusherCloser, nil
	}
	if t.content != "" {
		buf.WriteString(t.content)
	}
	return &testFlusherCloser{}, nil
}

func (t *testLoader) Recover(
	file, swapFilePath workspaceapi.URI, buf *cell.Buffer, force bool,
) (workspace.FlusherCloser, error) {
	return t.Load(file, buf, workspaceapi.URI{}, false)
}
func (t *testLoader) URI(path string) (workspaceapi.URI, error) {
	panic("unused")
}

func (t *testLoader) OpenFile(path string, flag int, perm os.FileMode) (
	workspaceapi.File, error,
) {
	panic("unused")
}

func (t *testLoader) Stat(path string) (os.FileInfo, error) {
	panic("unused")
}

func (t *testLoader) ReadDir(name string) ([]os.DirEntry, error) {
	panic("unused")
}

func newTestComponentErr(ed text.Editor) (*text.Component, error) {
	cfg := text.DefaultConfig()
	c, err := text.NewComponent(ed, &testLoader{}, cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func doSetupIntTest(
	t *testing.T, register func(*grpc.Server),
) (conn *grpc.ClientConn, closeFn func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	register(grpcServer)

	go grpcServer.Serve(lis)

	conn, err = grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	closeFn = func() {
		grpcServer.Stop()
		lis.Close()
	}
	return
}

func setupIntTest(
	t *testing.T, s *Server,
) (*Client, func()) {
	conn, closeFn := doSetupIntTest(t, func(grpcServer *grpc.Server) {
		textrpc.RegisterEditorServer(grpcServer, s)
	})
	client := NewClient(context.Background(), conn)
	return client, func() {
		client.Close()
		closeFn()
	}
}

func setupWmIntTest(
	t *testing.T, s *tbrowserrpc.Server,
) (*browserrpc.Client, func()) {
	conn, closeFn := doSetupIntTest(t, func(grpcServer *grpc.Server) {
		browserrpc.RegisterWindowManagerServer(grpcServer, s)
	})
	client := browserrpc.NewClient(context.Background(), conn)
	return client, func() {
		client.Close()
		closeFn()
	}
}

type nopNotifications struct{}

func (n nopNotifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	return "", nil
}
func (n nopNotifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	return "", nil
}

func (n nopNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return nil
}
