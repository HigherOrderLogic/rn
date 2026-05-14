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
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

const asyncResultsSleepDuration = 300 * time.Millisecond
const locID = "errors"

type nopLocker struct{}

func (l nopLocker) Lock()   {}
func (l nopLocker) Unlock() {}

func newTestServer(t *testing.T, ctrl *gomock.Controller) (*texttest.MockEditor, *Server) {
	ed := texttest.NewMockEditor(ctrl)
	s := NewServer(nopNotifications{}, ed, new(sync.Mutex))
	return ed, s
}

func expectEdit(t *testing.T, mock *texttest.MockEditor, resource workspaceapi.URI, content string, readOnly, recovered bool) {
	mock.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(_ context.Context, _uri workspaceapi.URI, buf *cell.Buffer, _readOnly, _recovered bool) (text.Handler, error) {
			assert.Equal(t, resource, _uri)
			assert.Equal(t, content, buf.String())
			assert.Equal(t, readOnly, _readOnly)
			assert.Equal(t, recovered, _recovered)
			return texttest.NewTestHandler(), nil
		})
}

func expectEditor(
	t *testing.T, ctrl *gomock.Controller,
	mock *texttest.MockEditor, resource workspaceapi.URI,
) *texttest.MockHandler {
	ret := texttest.NewMockHandler(ctrl)
	mock.EXPECT().Editor(gomock.Any()).AnyTimes().
		DoAndReturn(func(_uri workspaceapi.URI) (text.Handler, error) {
			assert.Equal(t, resource, _uri)
			return ret, nil
		})
	return ret
}

func callServerEdit(
	t *testing.T, ctx context.Context,
	s *Server, uri workspaceapi.URI, content string,
	readOnly, recovered bool,
) {
	buf := cell.NewBuffer()
	buf.WriteString(content)
	req := NewEditRequest(uri, buf, readOnly, recovered)

	res, err := s.Edit(ctx, &req)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestServerEdit(t *testing.T) {
	ctx := context.Background()
	resource, err := workspaceapi.ParseURI("file:///ULaptopNotLinux:@")
	require.NoError(t, err)
	bufContent1 := "ULaptopWillLinux:)"

	t.Run("Edit is propagated to underlying Editor", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock, s := newTestServer(t, ctrl)
		expectEdit(t, mock, resource, bufContent1, false, false)
		callServerEdit(t, ctx, s, resource, bufContent1, false, false)
	})

	t.Run("relative path is converted to absolute", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock, s := newTestServer(t, ctrl)
		require.NoError(t, err)
		expectEdit(t, mock, resource, bufContent1, false, false)
		callServerEdit(t, ctx, s, resource, bufContent1, false, false)
	})

	t.Run("bubbles up underlying's Editor Edit errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock, s := newTestServer(t, ctrl)

		mock.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("NOLINUX")).
			Times(1)

		req := NewEditRequest(resource, cell.NewBuffer(), false, false)

		res, err := s.Edit(ctx, &req)
		require.Error(t, err)
		require.Nil(t, res)
	})
}

func assertEqualLocations(t *testing.T, loc, expected text.LocationList) {
	var locations, expectedLocations []textapi.Location
	for ok := true; ok; _, ok = loc.Prev() {

	}
	for ok := true; ok; _, ok = expected.Prev() {

	}
	for n, ok := loc.Current(); ok; n, ok = loc.Next() {
		locations = append(locations, n)
	}
	for n, ok := expected.Current(); ok; n, ok = expected.Next() {
		expectedLocations = append(expectedLocations, n)
	}
	assert.EqualValues(t, expectedLocations, locations)
}

func TestServerSetLocationList(t *testing.T) {
	resource, err := workspaceapi.ParseURI("file:///go-tui")
	require.NoError(t, err)

	t.Run("calls underlying editor SetLocationList", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, s := newTestServer(t, ctrl)

		content := "main"
		expectEdit(t, mock, resource, content, false, false)
		callServerEdit(t, ctx, s, resource, content, false, false)

		locs := text.LocationSlice([]textapi.Location{{
			Message: "wsb: hold AMC",
			To:      term.Coordinates{X: 3},
			Icon:    "!",
		}})
		h := expectEditor(t, ctrl, mock, resource)
		h.EXPECT().SetLocationList(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(func(pri textapi.LocationPriority, ID string, l text.LocationList) {
				assert.Equal(t, textapi.LocationPriorityInfo, pri)
				assert.Equal(t, locID, ID)
				assertEqualLocations(t, l, locs)
			})

		req := makeLocationListRequest(resource, textapi.LocationPriorityInfo, locID, locs)
		res, err := s.SetLocationList(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
	})

	t.Run("is threadsafe", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ed := texttest.NopEditor()
		c, err := text.NewComponent(ed, &testLoader{}, text.Config{
			Config: browser.Config{
				Wallpaper: browser.NopWallpaper(),
			},
			ScheduleNextTick: func(fn func()) bool { fn(); return true },
		})
		require.NoError(t, err)
		s := NewServer(nopNotifications{}, c, new(sync.Mutex))

		content := "main"
		callServerEdit(t, ctx, s, resource, content, false, false)

		locs := []textapi.Location{
			{From: term.Coordinates{X: 0, Y: 0}, To: term.Coordinates{X: 3, Y: 0}},
			{From: term.Coordinates{X: 1, Y: 4}, To: term.Coordinates{X: 2, Y: 4}},
			{From: term.Coordinates{X: 0, Y: 5}, To: term.Coordinates{X: 0, Y: 6}},
		}

		var wg sync.WaitGroup
		n := 100
		wg.Add(n)
		for range n {
			go func() {
				defer wg.Done()
				l := text.LocationSlice(locs)

				req := makeLocationListRequest(resource, textapi.LocationPriorityWarning, locID, l)
				res, err := s.SetLocationList(ctx, &req)
				if !assert.NoError(t, err) {
					return
				}
				if !assert.NotNil(t, res) {
					return
				}
			}()
		}

		wg.Wait()
	})
}

func TestServerSetCursor(t *testing.T) {
	t.Run("calls underlying editor SetCursor", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, s := newTestServer(t, ctrl)

		resource, err := workspaceapi.ParseURI("file:///SetCursorer")
		require.NoError(t, err)
		content := "Oh my"
		expectEdit(t, mock, resource, content, true, true)
		callServerEdit(t, ctx, s, resource, content, true, true)

		pos := term.Coordinates{X: 4, Y: 5}
		h := expectEditor(t, ctrl, mock, resource)
		h.EXPECT().SetCursorAtScroll(gomock.Eq(pos)).Times(1)

		var protoPos termrpc.Coordinates
		protoPos.FromModel(pos)

		req := textrpc.SetCursorRequest{ResourceName: NewURI(resource), Pos: &protoPos}
		res, err := s.SetCursor(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
	})
}

func TestServerSubscribeCommandStopsNotificationHelperOnDisconnect(t *testing.T) {
	ed := newRecordingCommandEditor()
	notifications := &recordingNotifications{notified: make(chan string, 1)}
	s := NewServer(notifications, ed, nopLocker{})
	stream := newTestSubscribeCommandServer()

	done := make(chan error, 1)
	go func() {
		done <- s.SubscribeCommand(stream)
	}()

	select {
	case <-ed.subscribed:
	case <-time.After(time.Second):
		t.Fatal("SubscribeCommand did not register command handler")
	}

	stream.recv <- &textrpc.ClientCommandMessage{
		Type:   textrpc.ClientCommandMessage_Handle,
		Handle: &textrpc.HandleCommandResponse{Error: "first error"},
	}

	select {
	case errMsg := <-notifications.notified:
		assert.Equal(t, "first error", errMsg)
	case <-time.After(time.Second):
		t.Fatal("SubscribeCommand helper did not process command response")
	}

	stream.closeRecv()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorContains(t, err, "receive stream message")
	case <-time.After(time.Second):
		t.Fatal("SubscribeCommand did not return after client disconnect")
	}

	select {
	case ed.handler.handleCommand <- "late error":
		t.Fatal("SubscribeCommand helper still receives command responses after disconnect")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServerSubscribeCommandUnsubscribesPlaceholderBeforeResubscribe(t *testing.T) {
	ed := newStrictRecordingCommandEditor()
	s := NewServer(nopNotifications{}, ed, nopLocker{})

	first := newTestSubscribeCommandServer()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- s.SubscribeCommand(first)
	}()

	select {
	case <-ed.subscribed:
	case <-time.After(time.Second):
		t.Fatal("first SubscribeCommand did not register command handler")
	}

	first.closeRecv()
	select {
	case err := <-firstDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "receive stream message")
	case <-time.After(time.Second):
		t.Fatal("first SubscribeCommand did not return after disconnect")
	}

	second := newTestSubscribeCommandServer()
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- s.SubscribeCommand(second)
	}()

	select {
	case <-second.sent:
	case <-time.After(time.Second):
		t.Fatal("second SubscribeCommand did not send subscribe response")
	}

	second.closeRecv()
	select {
	case err := <-secondDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "receive stream message")
	case <-time.After(time.Second):
		t.Fatal("second SubscribeCommand did not return after disconnect")
	}

	assert.Equal(t, []string{
		"unsubscribe:test-command",
		"subscribe:test-command",
		"unsubscribe:test-command",
		"subscribe:test-command",
		"unsubscribe:test-command",
		"subscribe:test-command",
		"unsubscribe:test-command",
		"subscribe:test-command",
	}, ed.calls)
}

func TestServerSubscribeCommandReturnsUnexpectedUnsubscribeError(t *testing.T) {
	ed := &failingUnsubscribeCommandEditor{err: errors.New("boom")}
	s := NewServer(nopNotifications{}, ed, nopLocker{})

	err := s.SubscribeCommand(newTestSubscribeCommandServer())
	require.Error(t, err)
	assert.ErrorContains(t, err, `unsubscribe existing command "test-command": boom`)
}

func TestServerSubscribeREPLCommandUnregistersBeforeResubscribe(t *testing.T) {
	ed := newStrictRecordingREPLEditor()
	s := NewServer(nopNotifications{}, ed, nopLocker{})

	first := newTestSubscribeREPLCommandServer()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- s.SubscribeREPLCommand(first)
	}()

	select {
	case <-first.sent:
	case <-time.After(time.Second):
		t.Fatal("first SubscribeREPLCommand did not send subscribe response")
	}

	first.closeRecv()
	select {
	case err := <-firstDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "receive repl stream message")
	case <-time.After(time.Second):
		t.Fatal("first SubscribeREPLCommand did not return after disconnect")
	}

	second := newTestSubscribeREPLCommandServer()
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- s.SubscribeREPLCommand(second)
	}()

	select {
	case <-second.sent:
	case <-time.After(time.Second):
		t.Fatal("second SubscribeREPLCommand did not send subscribe response")
	}

	second.closeRecv()
	select {
	case err := <-secondDone:
		require.Error(t, err)
		assert.ErrorContains(t, err, "receive repl stream message")
	case <-time.After(time.Second):
		t.Fatal("second SubscribeREPLCommand did not return after disconnect")
	}

	assert.Equal(t, []string{
		"unregister:test-repl-command",
		"register:test-repl-command",
		"unregister:test-repl-command",
		"register:test-repl-command",
	}, ed.calls)
}

func TestServerSubscribeREPLCommandReturnsUnexpectedUnregisterError(t *testing.T) {
	ed := &failingUnregisterREPLEditor{err: errors.New("boom")}
	s := NewServer(nopNotifications{}, ed, nopLocker{})

	err := s.SubscribeREPLCommand(newTestSubscribeREPLCommandServer())
	require.Error(t, err)
	assert.ErrorContains(t, err, `unregister existing repl command "test-repl-command": boom`)
}

func TestServerSubscribeEventCleansUpOnUnsubscribeEOFAndClose(t *testing.T) {
	t.Run("normal unsubscribe", func(t *testing.T) {
		ed := texttest.NopEditor()
		s := NewServer(nopNotifications{}, ed, nopLocker{})
		stream := newTestSubscribeEventServer(
			&textrpc.SubscribeEventRequest{Type: []textrpc.EditorEvent_Type{textrpc.EditorEvent_TypeOpen}},
			&textrpc.SubscribeEventRequest{Unsubscribe: true},
		)

		err := s.SubscribeEvent(stream)
		require.NoError(t, err)
		assert.Empty(t, ed.Subscribers()[textapi.EventTypeOpen])
	})

	t.Run("client eof without unsubscribe", func(t *testing.T) {
		ed := texttest.NopEditor()
		s := NewServer(nopNotifications{}, ed, nopLocker{})
		stream := newTestSubscribeEventServer(
			&textrpc.SubscribeEventRequest{Type: []textrpc.EditorEvent_Type{textrpc.EditorEvent_TypeOpen}},
		)

		err := s.SubscribeEvent(stream)
		require.Error(t, err)
		assert.ErrorContains(t, err, "stream receive")
		assert.Empty(t, ed.Subscribers()[textapi.EventTypeOpen])
	})

	t.Run("server close stops sender", func(t *testing.T) {
		subscribed := make(chan struct{})
		ed := texttest.NopEditorWithCallback(func() {
			close(subscribed)
		})
		s := NewServer(nopNotifications{}, ed, nopLocker{})
		stream := newBlockingTestSubscribeEventServer(
			&textrpc.SubscribeEventRequest{Type: []textrpc.EditorEvent_Type{textrpc.EditorEvent_TypeOpen}},
		)

		done := make(chan error, 1)
		go func() {
			done <- s.SubscribeEvent(stream)
		}()

		select {
		case <-subscribed:
		case <-time.After(time.Second):
			t.Fatal("SubscribeEvent did not register event handler")
		}

		require.NoError(t, s.Close())
		stream.cancel()
		select {
		case err := <-done:
			require.Error(t, err)
			assert.ErrorContains(t, err, "stream receive")
		case <-time.After(time.Second):
			t.Fatal("SubscribeEvent did not return after server close")
		}
		assert.Empty(t, ed.Subscribers()[textapi.EventTypeOpen])
	})
}

func TestServerCursor(t *testing.T) {
	t.Run("calls underlying editor Cursor", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, s := newTestServer(t, ctrl)

		resource, err := workspaceapi.ParseURI("file:///Cursorer")
		require.NoError(t, err)
		expectEdit(t, mock, resource, "", false, false)
		callServerEdit(t, ctx, s, resource, "", false, false)

		pos := term.Coordinates{X: 4, Y: 5}
		h := expectEditor(t, ctrl, mock, resource)
		h.EXPECT().CursorAtScroll().Return(pos).Times(1)

		req := textrpc.CursorRequest{ResourceName: NewURI(resource)}
		res, err := s.Cursor(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)

		assert.Equal(t, pos, res.GetPos().ToModel())
	})
}

type recordingCommandEditor struct {
	texttest.TestEditor
	subscribed chan struct{}
	once       sync.Once
	handler    *commandClientStream
}

func newRecordingCommandEditor() *recordingCommandEditor {
	return &recordingCommandEditor{
		subscribed: make(chan struct{}),
	}
}

func (e *recordingCommandEditor) SubscribeCommand(_ textapi.CommandManual, h text.CommandHandler) error {
	if stream, ok := h.(*commandClientStream); ok {
		e.handler = stream
	}
	e.once.Do(func() {
		close(e.subscribed)
	})
	return nil
}

func (e *recordingCommandEditor) UnsubscribeCommand(string) error {
	return nil
}

type strictRecordingCommandEditor struct {
	texttest.TestEditor
	mu         sync.Mutex
	subscribed chan struct{}
	once       sync.Once
	registered map[string]struct{}
	calls      []string
	handler    *commandClientStream
}

func newStrictRecordingCommandEditor() *strictRecordingCommandEditor {
	return &strictRecordingCommandEditor{
		subscribed: make(chan struct{}),
		registered: make(map[string]struct{}),
	}
}

func (e *strictRecordingCommandEditor) SubscribeCommand(cmd textapi.CommandManual, h text.CommandHandler) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, "subscribe:"+cmd.Name)
	if _, ok := e.registered[cmd.Name]; ok {
		return errors.New("command already registered")
	}
	e.registered[cmd.Name] = struct{}{}
	if stream, ok := h.(*commandClientStream); ok {
		e.handler = stream
	}
	e.once.Do(func() {
		close(e.subscribed)
	})
	return nil
}

func (e *strictRecordingCommandEditor) UnsubscribeCommand(cmd string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, "unsubscribe:"+cmd)
	if _, ok := e.registered[cmd]; !ok {
		return text.ErrCommandNotRegistered
	}
	delete(e.registered, cmd)
	return nil
}

type failingUnsubscribeCommandEditor struct {
	texttest.TestEditor
	err error
}

func (e *failingUnsubscribeCommandEditor) SubscribeCommand(textapi.CommandManual, text.CommandHandler) error {
	return nil
}

func (e *failingUnsubscribeCommandEditor) UnsubscribeCommand(string) error {
	return e.err
}

type strictRecordingREPLEditor struct {
	texttest.TestEditor
	mu         sync.Mutex
	registered map[string]struct{}
	calls      []string
}

func newStrictRecordingREPLEditor() *strictRecordingREPLEditor {
	return &strictRecordingREPLEditor{registered: make(map[string]struct{})}
}

func (e *strictRecordingREPLEditor) RegisterREPLCommand(
	cmd textapi.CommandManual, _ textapi.REPLHandler,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, "register:"+cmd.Name)
	if _, ok := e.registered[cmd.Name]; ok {
		return errors.New("command already registered")
	}
	e.registered[cmd.Name] = struct{}{}
	return nil
}

func (e *strictRecordingREPLEditor) UnregisterREPLCommand(cmd string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, "unregister:"+cmd)
	if _, ok := e.registered[cmd]; !ok {
		return text.ErrCommandNotRegistered
	}
	delete(e.registered, cmd)
	return nil
}

type failingUnregisterREPLEditor struct {
	texttest.TestEditor
	err error
}

func (e *failingUnregisterREPLEditor) RegisterREPLCommand(textapi.CommandManual, textapi.REPLHandler) error {
	return nil
}

func (e *failingUnregisterREPLEditor) UnregisterREPLCommand(string) error {
	return e.err
}

type testSubscribeREPLCommandServer struct {
	ctx  context.Context
	recv chan *textrpc.ClientREPLCommandMessage
	sent chan *textrpc.ServerREPLCommandMessage
}

func newTestSubscribeREPLCommandServer() *testSubscribeREPLCommandServer {
	stream := &testSubscribeREPLCommandServer{
		ctx:  context.Background(),
		recv: make(chan *textrpc.ClientREPLCommandMessage, 1),
		sent: make(chan *textrpc.ServerREPLCommandMessage, 1),
	}
	stream.recv <- &textrpc.ClientREPLCommandMessage{
		Type: textrpc.ClientREPLCommandMessage_Request,
		Request: &textrpc.SubscribeREPLCommandRequest{Command: &textrpc.CommandManual{
			Name: "test-repl-command",
		}},
	}
	return stream
}

func (s *testSubscribeREPLCommandServer) closeRecv() {
	close(s.recv)
}

func (s *testSubscribeREPLCommandServer) Recv() (*textrpc.ClientREPLCommandMessage, error) {
	msg, ok := <-s.recv
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

func (s *testSubscribeREPLCommandServer) Send(msg *textrpc.ServerREPLCommandMessage) error {
	s.sent <- msg
	return nil
}

func (s *testSubscribeREPLCommandServer) Context() context.Context {
	return s.ctx
}

func (s *testSubscribeREPLCommandServer) SendMsg(msg any) error {
	serverMsg := msg.(*textrpc.ServerREPLCommandMessage)
	return s.Send(serverMsg)
}

func (s *testSubscribeREPLCommandServer) RecvMsg(msg any) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	clientMsg := msg.(*textrpc.ClientREPLCommandMessage)
	proto.Merge(clientMsg, next)
	return nil
}

func (s *testSubscribeREPLCommandServer) SetHeader(metadata.MD) error {
	return nil
}

func (s *testSubscribeREPLCommandServer) SendHeader(metadata.MD) error {
	return nil
}

func (s *testSubscribeREPLCommandServer) SetTrailer(metadata.MD) {}

type recordingNotifications struct {
	notified chan string
}

func (n *recordingNotifications) Notify(_ browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	message := fmt.Sprintf(msg, args...)
	select {
	case n.notified <- message:
	default:
	}
	return "", nil
}

func (n *recordingNotifications) NotifyOnce(level browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	return n.Notify(level, msg, args...)
}

func (n *recordingNotifications) UpdateNotificationProgress(id, message string, progress, total int64) error {
	return nil
}

type testSubscribeCommandServer struct {
	ctx  context.Context
	recv chan *textrpc.ClientCommandMessage
	sent chan *textrpc.ServerCommandMessage
}

func newTestSubscribeCommandServer() *testSubscribeCommandServer {
	stream := &testSubscribeCommandServer{
		ctx:  context.Background(),
		recv: make(chan *textrpc.ClientCommandMessage, 1),
		sent: make(chan *textrpc.ServerCommandMessage, 1),
	}
	stream.recv <- &textrpc.ClientCommandMessage{
		Type: textrpc.ClientCommandMessage_Request,
		Request: &textrpc.SubscribeCommandRequest{Command: &textrpc.CommandManual{
			Name: "test-command",
		}},
	}
	return stream
}

func (s *testSubscribeCommandServer) closeRecv() {
	close(s.recv)
}

func (s *testSubscribeCommandServer) Recv() (*textrpc.ClientCommandMessage, error) {
	msg, ok := <-s.recv
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

func (s *testSubscribeCommandServer) Send(msg *textrpc.ServerCommandMessage) error {
	s.sent <- msg
	return nil
}

func (s *testSubscribeCommandServer) Context() context.Context {
	return s.ctx
}

func (s *testSubscribeCommandServer) SendMsg(msg any) error {
	serverMsg := msg.(*textrpc.ServerCommandMessage)
	return s.Send(serverMsg)
}

func (s *testSubscribeCommandServer) RecvMsg(msg any) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	clientMsg := msg.(*textrpc.ClientCommandMessage)
	proto.Merge(clientMsg, next)
	return nil
}

func (s *testSubscribeCommandServer) SetHeader(metadata.MD) error {
	return nil
}

func (s *testSubscribeCommandServer) SendHeader(metadata.MD) error {
	return nil
}

func (s *testSubscribeCommandServer) SetTrailer(metadata.MD) {}

type testSubscribeEventServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	recv   chan *textrpc.SubscribeEventRequest
	sent   chan *textrpc.EditorEvent
}

func newTestSubscribeEventServer(msgs ...*textrpc.SubscribeEventRequest) *testSubscribeEventServer {
	return newTestSubscribeEventServerWithClose(true, msgs...)
}

func newBlockingTestSubscribeEventServer(msgs ...*textrpc.SubscribeEventRequest) *testSubscribeEventServer {
	return newTestSubscribeEventServerWithClose(false, msgs...)
}

func newTestSubscribeEventServerWithClose(closeRecv bool, msgs ...*textrpc.SubscribeEventRequest) *testSubscribeEventServer {
	ctx, cancel := context.WithCancel(context.Background())
	stream := &testSubscribeEventServer{
		ctx:    ctx,
		cancel: cancel,
		recv:   make(chan *textrpc.SubscribeEventRequest, len(msgs)),
		sent:   make(chan *textrpc.EditorEvent, 1),
	}
	for _, msg := range msgs {
		stream.recv <- msg
	}
	if closeRecv {
		close(stream.recv)
	}
	return stream
}

func (s *testSubscribeEventServer) Recv() (*textrpc.SubscribeEventRequest, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case msg, ok := <-s.recv:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	}
}

func (s *testSubscribeEventServer) Send(msg *textrpc.EditorEvent) error {
	s.sent <- msg
	return nil
}

func (s *testSubscribeEventServer) Context() context.Context {
	return s.ctx
}

func (s *testSubscribeEventServer) SendMsg(msg any) error {
	event := msg.(*textrpc.EditorEvent)
	return s.Send(event)
}

func (s *testSubscribeEventServer) RecvMsg(msg any) error {
	next, err := s.Recv()
	if err != nil {
		return err
	}
	req := msg.(*textrpc.SubscribeEventRequest)
	proto.Reset(req)
	proto.Merge(req, next)
	return nil
}

func (s *testSubscribeEventServer) SetHeader(metadata.MD) error {
	return nil
}

func (s *testSubscribeEventServer) SendHeader(metadata.MD) error {
	return nil
}

func (s *testSubscribeEventServer) SetTrailer(metadata.MD) {}
