package browser

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/proto"
	prototest "github.com/ernestrc/go-tui/proto/test"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const asyncResultsSleepDuration = 300 * time.Millisecond

func newServerWithNoBroker(ctrl *gomock.Controller) (
	*Server, *MockBrowser,
) {
	mock := NewMockBrowser(ctrl)
	s := NewServer(nil, mock, new(sync.Mutex))
	return s, mock
}

func newTestServer(ctrl *gomock.Controller, mu *sync.Mutex) (
	*Server, *MockBrowser, *proto.MockMuxBroker,
) {
	mockBrowser := NewMockBrowser(ctrl)
	mockBroker := proto.NewMockMuxBroker(ctrl)
	s := NewServer(mockBroker, mockBrowser, mu)
	return s, mockBrowser, mockBroker
}

func TestServerSetMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("delegates SetMessage to underlying Browser", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		s, mock := newServerWithNoBroker(ctrl)

		mock.EXPECT().SetMessage(gomock.Eq("blah")).Return(nil)

		req := proto.SetMessageRequest{Msg: "blah"}
		res, err := s.SetMessage(ctx, &req)
		require.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("bubbles up SetMessage Browser error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		s, mock := newServerWithNoBroker(ctrl)

		mock.EXPECT().SetMessage(gomock.Any()).Return(errors.New("oopsie daisy"))

		_, err := s.SetMessage(ctx, new(proto.SetMessageRequest))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "oopsie")
	})
}

func assertHandlerStored(t *testing.T, nextID uint32, s *Server, expected Handler) {
	h, ok := s.opened[nextID]
	require.True(t, ok)
	assert.Equal(t, expected, h)
}

func TestServerOpen(t *testing.T) {
	ctx := context.Background()

	t.Run("delegates Open to underlying Browser", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		s, mock, broker := newTestServer(ctrl, new(sync.Mutex))
		uri, err := workspace.ParseURI("file:///tmp/coronavirus.sql")
		require.NoError(t, err)

		h := NewTestHandler()
		mock.EXPECT().Open(gomock.Eq(uri)).Return(h, nil)
		broker.EXPECT().NextId().Return(uint32(1))

		req := proto.OpenResourceRequest{Resource: "file:///tmp/coronavirus.sql"}
		res, err := s.Open(ctx, &req)
		require.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("bubbles up Open Browser error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		s, mock, _ := newTestServer(ctrl, new(sync.Mutex))

		mock.EXPECT().Open(gomock.Any()).Return(nil, errors.New("oopsie daisy"))

		req := proto.OpenResourceRequest{Resource: "file:///a"}
		_, err := s.Open(ctx, &req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "oopsie")
	})

	t.Run("stores handler for use with Split/SetContent methods", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		s, mock, broker := newTestServer(ctrl, new(sync.Mutex))

		nextID := uint32(10)
		h := NewTestHandler()
		mock.EXPECT().Open(gomock.Any()).Return(h, nil)
		broker.EXPECT().NextId().Return(nextID)

		req := proto.OpenResourceRequest{Resource: "file:///Caliu"}
		res, err := s.Open(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assertHandlerStored(t, nextID, s, h)

		sreq := proto.SplitRequest{
			HandlerId:   uint64(nextID),
			Orientation: proto.Orientation_Right,
		}
		windowID := uint32(999)
		prototest.ExpectBrokerServe(t, windowID, broker)
		mockWindow := NewMockWindow(ctrl)
		mockWindow.EXPECT().onWindowClosed(gomock.Any()).AnyTimes()
		mockWindow.EXPECT().id().AnyTimes().Return(uint64(0))

		mock.EXPECT().Split(gomock.Eq(OrientationRight), gomock.Any()).Return(mockWindow, nil)
		_, err = s.Split(ctx, &sreq)
		require.NoError(t, err)
	})
}

func insertDrawResponse(t *testing.T, quit bool) func(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	return func(ctx context.Context,
		method string, args interface{},
		reply interface{}, opts ...grpc.CallOption) error {
		// validate mappings with finer grained control
		res, ok := reply.(*proto.HandleResponse)
		require.True(t, ok)

		res.Draw = proto.NewDrawResponse(component.String(""), 0, 0)
		res.Quit = quit
		res.Handled = true
		return nil
	}
}

func expectHandlerInvoke(t *testing.T, handlerConn *proto.MockMuxConn, protoEv *proto.Event) {
	handlerConn.EXPECT().
		Invoke(gomock.Any(), gomock.Eq("/proto.Handler/Handle"),
			gomock.Eq(&proto.HandleRequest{Event: protoEv, Draw: &proto.DrawRequest{}}),
			gomock.Any()).
		Times(1).
		DoAndReturn(insertDrawResponse(t, false))
}

func expectHandlerInvokeExit(t *testing.T, handlerConn *proto.MockMuxConn) {
	handlerConn.EXPECT().
		Invoke(gomock.Any(), gomock.Eq("/proto.Handler/Handle"),
			gomock.Any(),
			gomock.Any()).
		DoAndReturn(insertDrawResponse(t, true)).
		Times(1)
}

func TestServerPublish(t *testing.T) {
	ctx := context.Background()

	t.Run("handles interrupt event by calling interrupt handler", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, mock, _ := newTestServer(ctrl, &mu)
		req := proto.PublishRequest{Ev: &proto.Event{Type: proto.Event_TypeInterrupt}}

		mock.EXPECT().PublishInterrupt().Times(1)

		res, err := s.Publish(ctx, &req)
		require.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("handles EventNone event by calling interrupt handler", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, mock, _ := newTestServer(ctrl, &mu)
		req := proto.PublishRequest{Ev: &proto.Event{Type: proto.Event_TypeNone}}

		mock.EXPECT().PublishEventNone().Times(1)

		res, err := s.Publish(ctx, &req)
		require.NoError(t, err)
		assert.NotNil(t, res)
	})

	t.Run("rejects any event other than an interrupt event", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, _, _ := newTestServer(ctrl, &mu)
		req := proto.PublishRequest{Ev: &proto.Event{Type: proto.Event_TypeKey}}

		res, err := s.Publish(ctx, &req)
		require.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("handles interrupt handler error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, mock, _ := newTestServer(ctrl, &mu)
		req := proto.PublishRequest{Ev: &proto.Event{Type: proto.Event_TypeInterrupt}}

		mock.EXPECT().PublishInterrupt().Return(errors.New("uRock"))

		res, err := s.Publish(ctx, &req)
		require.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("handles event none handler error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, mock, _ := newTestServer(ctrl, &mu)
		req := proto.PublishRequest{Ev: &proto.Event{Type: proto.Event_TypeNone}}

		mock.EXPECT().PublishEventNone().Return(errors.New("uRock"))

		res, err := s.Publish(ctx, &req)
		require.Error(t, err)
		assert.Nil(t, res)
	})
}

func assertServerClientsEqual(t *testing.T, expected int, s *Server) {
	s.browser.Lock()
	defer s.browser.Unlock()
	assert.Equal(t, expected, len(s.clients))
}

func assertServerServersEqual(t *testing.T, expected int, s *Server) {
	s.browser.Lock()
	defer s.browser.Unlock()
	assert.Equal(t, expected, len(s.servers))
}

func TestServerSplitHorizontalAbove(t *testing.T) {
	testServerSplit(t, OrientationTop, proto.Orientation_Top)
}

func TestServerSplitDefault(t *testing.T) {
	testServerSplit(t, OrientationDefault, proto.Orientation_Default)
}

func TestServerSplitHorizontalBelow(t *testing.T) {
	testServerSplit(t, OrientationBottom, proto.Orientation_Bottom)
}

func TestServerSplitVerticalLeft(t *testing.T) {
	testServerSplit(t, OrientationLeft, proto.Orientation_Left)
}

func TestServerSplitVerticalRight(t *testing.T) {
	testServerSplit(t, OrientationRight, proto.Orientation_Right)
}

func waitForMonitoringExit(quitCh chan struct{}) {
	<-quitCh
	time.Sleep(asyncResultsSleepDuration)
}

func assertServerHandlerExitClose(
	t *testing.T, handlerConn *proto.MockMuxConn,
	h tui.Handler, s *Server,
	termEv term.Event, quitCh chan struct{},
) {
	expectHandlerInvokeExit(t, handlerConn)

	handlerConn.EXPECT().Close().Times(1).
		DoAndReturn(prototest.ExpectSignalExit(handlerConn, quitCh, nil))

	s.browser.Lock()
	exit, _ := h.Handle(termEv)
	s.browser.Unlock()
	assert.True(t, exit)

	waitForMonitoringExit(quitCh)

	assertServerClientsEqual(t, 0, s)
}

func testServerSplit(t *testing.T, expectedSplit Orientation, split proto.Orientation) {
	ctx := context.Background()
	handlerID := uint32(21)
	windowID := uint32(111111)
	req := proto.SplitRequest{
		HandlerId:   uint64(handlerID),
		Orientation: split,
	}
	protoEv := proto.Event{
		Key:    proto.Event_MouseMiddle,
		Mod:    proto.Event_Motion,
		MouseX: 10,
		MouseY: 1393291,
	}
	termEv := term.Event{
		Key:    term.MouseMiddle,
		Mod:    term.ModMotion,
		MouseX: 10,
		MouseY: 1393291,
	}

	t.Run("dials to remote handler and exposes window server for client to dial into", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, mock, mockBroker := newTestServer(ctrl, &mu)

		// store onWindowClosed callback
		var callback func()
		mockWindow := NewMockWindow(ctrl)
		mockWindow.EXPECT().id().AnyTimes().Return(uint64(0))
		mockWindow.EXPECT().onWindowClosed(gomock.Any()).DoAndReturn(func(fn func()) {
			callback = fn
		}).Times(1)

		var h Handler
		handlerConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, handlerID)
		quitCh := prototest.ExpectMonitorConn(handlerConn)
		mock.EXPECT().Split(gomock.Eq(expectedSplit), gomock.Any()).
			DoAndReturn(func(_ Orientation, _h Handler) (Window, error) {
				h = _h
				return mockWindow, nil
			})
		prototest.ExpectBrokerServe(t, windowID, mockBroker)

		res, err := s.Split(ctx, &req)
		require.NoError(t, err)
		assert.NotNil(t, res)

		// verify that handler works
		expectHandlerInvoke(t, handlerConn, &protoEv)
		s.browser.Lock()
		exit, handled := h.Handle(termEv)
		s.browser.Unlock()
		assert.False(t, exit)
		assert.True(t, handled)

		time.Sleep(asyncResultsSleepDuration)

		// verify that handler is closeable by its handlerId
		handlerConn.EXPECT().Close().Times(1).
			DoAndReturn(prototest.ExpectSignalExit(handlerConn, quitCh, nil))
		require.NoError(t, s.safeForceCloseHandler(uint64(handlerID), ""))
		assertServerClientsEqual(t, 0, s)
		waitForMonitoringExit(quitCh)

		assertServerServersEqual(t, 1, s)

		// verify that resources are cleaned upon call to onWindowClosed callback
		callback()
		assertServerServersEqual(t, 0, s)
	})

	t.Run("bubbles up dial error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, _, mockBroker := newTestServer(ctrl, &mu)

		prototest.ExpectBrokerDialError(t, ctrl, mockBroker, handlerID)

		res, err := s.Split(ctx, &req)
		require.Error(t, err)
		assert.Nil(t, res)

		assertServerClientsEqual(t, 0, s)
		assertServerServersEqual(t, 0, s)
	})

	t.Run("bubbles up browser split error and so closes handler connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		var mu sync.Mutex
		s, mock, mockBroker := newTestServer(ctrl, &mu)

		handlerConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, handlerID)
		quitCh := prototest.ExpectMonitorConn(handlerConn)
		mock.EXPECT().Split(gomock.Eq(expectedSplit), gomock.Any()).
			Return(nil, errors.New("woopsie"))
		handlerConn.EXPECT().Close().Times(1).
			DoAndReturn(prototest.ExpectSignalExit(handlerConn, quitCh, nil))

		res, err := s.Split(ctx, &req)
		require.Error(t, err)
		assert.Nil(t, res)
		waitForMonitoringExit(quitCh)

		assertServerClientsEqual(t, 0, s)
		assertServerServersEqual(t, 0, s)
	})

	assertNoLeaks(t)
}

func TestServerSetContent(t *testing.T) {
	/* tested via ex integration tests */
}
