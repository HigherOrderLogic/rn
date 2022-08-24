package text

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/handler"
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

type nopLocker struct{}

func (l nopLocker) Lock()   {}
func (l nopLocker) Unlock() {}

func newTestServer(t *testing.T, ctrl *gomock.Controller) (*proto.MockMuxBroker, *MockEditor, *Server) {
	broker := proto.NewMockMuxBroker(ctrl)
	ed := NewMockEditor(ctrl)
	expectInitialServerSubscribe(t, ed)
	s := NewServer(broker, ed, new(sync.Mutex))
	broker.EXPECT().Cleanup(gomock.Any()).AnyTimes()
	return broker, ed, s
}

func expectEdit(t *testing.T, mock *MockEditor, resource workspace.URI, content string) {
	mock.EXPECT().Edit(gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(_uri workspace.URI, buf *cell.Buffer) (tui.Handler, error) {
			assert.Equal(t, resource, _uri)
			assert.Equal(t, content, buf.String())
			return handler.NewTestHandler(), nil
		})
}

func callServerEdit(
	t *testing.T, ctx context.Context, broker *proto.MockMuxBroker,
	s *Server, nextID uint32, uri workspace.URI, content string,
) {
	broker.EXPECT().NextId().Return(nextID).Times(1)

	buf := cell.NewBuffer()
	buf.WriteString(content)
	req := proto.NewEditRequest(uri, buf)

	res, err := s.Edit(ctx, &req)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, nextID, res.GetHandlerId())
}

func TestServerEdit(t *testing.T) {
	nextID := uint32(99)
	ctx := context.Background()
	resource, err := workspace.ParseURI("file:///ULaptopNotLinux:@")
	require.NoError(t, err)
	bufContent1 := "ULaptopWillLinux:)"

	t.Run("Edit is propagated to underlying Editor", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		broker, mock, s := newTestServer(t, ctrl)
		expectEdit(t, mock, resource, bufContent1)
		callServerEdit(t, ctx, broker, s, nextID, resource, bufContent1)
	})

	t.Run("relative path is converted to absolute", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		broker, mock, s := newTestServer(t, ctrl)
		require.NoError(t, err)
		expectEdit(t, mock, resource, bufContent1)
		callServerEdit(t, ctx, broker, s, nextID, resource, bufContent1)
	})

	t.Run("bubbles up underlying's Editor Edit errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		_, mock, s := newTestServer(t, ctrl)

		mock.EXPECT().Edit(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("NOLINUX")).
			Times(1)

		req := proto.NewEditRequest(resource, cell.NewBuffer())

		res, err := s.Edit(ctx, &req)
		require.Error(t, err)
		require.Nil(t, res)
	})
}

func waitForMonitoringExit(quitCh chan struct{}) {
	<-quitCh
	time.Sleep(asyncResultsSleepDuration)
}

func expectHandlerInvokeExit(t *testing.T, handlerConn *proto.MockMuxConn) {
	handlerConn.EXPECT().
		Invoke(gomock.Any(), gomock.Eq("/proto.EditorEventHandler/Handle"),
			gomock.Any(),
			gomock.Any()).
		DoAndReturn(func(ctx context.Context,
			method string, args interface{},
			reply interface{}, opts ...grpc.CallOption) error {
			res, ok := reply.(*proto.EditorEventHandleResponse)
			require.True(t, ok)

			res.Quit = true
			return nil
		}).
		Times(1)
}

func assertServerHandlerExitClose(
	t *testing.T, handlerConn *proto.MockMuxConn,
	h EventHandler, s *Server, quitCh chan struct{},
	broker *proto.MockMuxBroker,
) {
	resource := &TestHandler{}
	expectHandlerInvokeExit(t, handlerConn)

	handlerConn.EXPECT().Close().Times(1).
		DoAndReturn(prototest.ExpectSignalExit(handlerConn, quitCh, nil))

	broker.EXPECT().NextId().Return(uint32(88)).Times(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// force server to store resource name and make an ID
	s.Handle(ctx, Event{Type: EventTypeOpen, URI: uri, Resource: resource})

	s.editor.Lock()
	h.Handle(ctx, Event{Type: EventTypeClose, URI: uri, Resource: resource})
	s.editor.Unlock()

	waitForMonitoringExit(quitCh)

	s.editor.Lock()
	defer s.editor.Unlock()
	assert.Equal(t, 0, len(s.clients))
}

func TestServerSubscribe(t *testing.T) {
	ctx := context.Background()
	t.Run("propagates subscribe with multiple event types", func(t *testing.T) {
		nextID := uint32(12)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker, mock, s := newTestServer(t, ctrl)
		expectedEvTypes := []EventType{EventTypeFlush, EventTypeFocus}

		var actualEvTypes []EventType
		var wg sync.WaitGroup
		mock.EXPECT().SubscribeEditorEvents(gomock.Any(), gomock.Any()).
			DoAndReturn(func(evs []EventType, _h EventHandler) error {
				defer wg.Done()
				actualEvTypes = evs
				return nil
			})
		req := proto.EditorSubscribeRequest{
			HandlerId: nextID,
			Type: []proto.EditorEvent_Type{
				proto.EditorEvent_TypeFlush,
				proto.EditorEvent_TypeFocus,
			},
		}
		conn := prototest.ExpectBrokerDial(t, ctrl, broker, nextID)
		quitCh := prototest.ExpectMonitorConn(conn)

		wg.Add(1)
		res, err := s.Subscribe(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)

		wg.Wait()

		conn.EXPECT().Close().Times(1).
			DoAndReturn(prototest.ExpectSignalExit(conn, quitCh, nil))
		conn.EXPECT().Invoke(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes()

		assert.Equal(t, expectedEvTypes, actualEvTypes)
		assert.NoError(t, s.Close())
		waitForMonitoringExit(quitCh)
	})

	t.Run("cleans resources when handler subscriber returns exit=true", func(t *testing.T) {
		nextID := uint32(12222)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker, mock, s := newTestServer(t, ctrl)
		expectedEvTypes := []EventType{EventTypeFlush}

		var actualEvTypes []EventType
		var h EventHandler
		var wg sync.WaitGroup
		mock.EXPECT().SubscribeEditorEvents(gomock.Any(), gomock.Any()).
			DoAndReturn(func(evs []EventType, _h EventHandler) error {
				defer wg.Done()
				actualEvTypes = evs
				h = _h
				return nil
			})
		conn := prototest.ExpectBrokerDial(t, ctrl, broker, nextID)
		quitCh := prototest.ExpectMonitorConn(conn)

		req := proto.EditorSubscribeRequest{
			HandlerId: nextID,
			Type:      []proto.EditorEvent_Type{proto.EditorEvent_TypeFlush},
		}

		wg.Add(1)
		res, err := s.Subscribe(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)

		wg.Wait()

		require.NotNil(t, h)
		assert.Equal(t, expectedEvTypes, actualEvTypes)
		assertServerHandlerExitClose(t, conn, h, s, quitCh, broker)
	})
}

func assertEqualLocations(t *testing.T, loc, expected LocationList) {
	var locations, expectedLocations []Location
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
	resource, err := workspace.ParseURI("file:///go-tui")
	require.NoError(t, err)

	t.Run("calls underlying editor SetLocationList", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker, mock, s := newTestServer(t, ctrl)

		content := "main"
		nextID := uint32(232)
		expectEdit(t, mock, resource, content)
		callServerEdit(t, ctx, broker, s, nextID, resource, content)

		locs := LocationSlice([]Location{{Message: "wsb: hold AMC", To: term.Coordinates{X: 3}}})
		mock.EXPECT().SetLocationList(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

		req := makeLocationListRequest(nextID, locID, locs)
		res, err := s.SetLocationList(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
	})

	t.Run("is threadsafe", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker := proto.NewMockMuxBroker(ctrl)
		ed := &testEditor{}
		c, err := NewComponent(ed, &testWorkspace{}, Config{})
		require.NoError(t, err)
		s := NewServer(broker, c, new(sync.Mutex))

		content := "main"
		nextID := uint32(232)
		callServerEdit(t, ctx, broker, s, nextID, resource, content)

		locs := []Location{
			{From: term.Coordinates{X: 0, Y: 0}, To: term.Coordinates{X: 3, Y: 0}},
			{From: term.Coordinates{X: 1, Y: 4}, To: term.Coordinates{X: 2, Y: 4}},
			{From: term.Coordinates{X: 0, Y: 5}, To: term.Coordinates{X: 0, Y: 6}},
		}

		var wg sync.WaitGroup
		n := 100
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				l := LocationSlice(locs)

				req := makeLocationListRequest(nextID, locID, l)
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

		h, ok := s.idToHandler[nextID]
		if !assert.True(t, ok) {
			return
		}

		l, ok := h.(*testEditorHandler)
		if !assert.True(t, ok) {
			return
		}
		assertEqualLocations(t, LocationSlice(locs), l.locationList)
	})
}

func TestServerSetCursor(t *testing.T) {
	t.Run("calls underlying editor SetCursor", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker, mock, s := newTestServer(t, ctrl)

		resource, err := workspace.ParseURI("file:///SetCursorer")
		require.NoError(t, err)
		content := "Oh my"
		nextID := uint32(12888)
		expectEdit(t, mock, resource, content)
		callServerEdit(t, ctx, broker, s, nextID, resource, content)

		pos := term.Coordinates{X: 4, Y: 5}
		mock.EXPECT().SetCursor(gomock.Any(), gomock.Eq(pos)).Return(nil).Times(1)

		var protoPos proto.Coordinates
		protoPos.FromModel(pos)

		req := proto.SetCursorRequest{HandlerId: nextID, Pos: &protoPos}
		res, err := s.SetCursor(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
	})
}

func TestServerCursor(t *testing.T) {
	t.Run("calls underlying editor Cursor", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker, mock, s := newTestServer(t, ctrl)

		resource, err := workspace.ParseURI("file:///Cursorer")
		require.NoError(t, err)
		nextID := uint32(12888)
		expectEdit(t, mock, resource, "")
		callServerEdit(t, ctx, broker, s, nextID, resource, "")

		pos := term.Coordinates{X: 4, Y: 5}
		mock.EXPECT().Cursor(gomock.Any()).Return(pos, nil).Times(1)

		req := proto.CursorRequest{HandlerId: nextID}
		res, err := s.Cursor(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)

		assert.Equal(t, pos, res.GetPos().ToModel())
	})
}
