package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui/proto"
	prototest "github.com/ernestrc/go-tui/proto/test"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
)

var (
	key1 = term.Event{Type: term.EventKey, Key: term.KeyCtrlBackslash}
	key2 = term.Event{Type: term.EventKey,
		Mod: term.ModAlt, Key: term.KeyBackspace}
	key3      = term.Event{Type: term.EventMouse, MouseX: 10, MouseY: 11111}
	protoKey1 = proto.Event{
		Type: proto.Event_TypeKey,
		Key:  proto.Event_Ctrl4,
	}
	protoKey2 = proto.Event{
		Type: proto.Event_TypeKey,
		Key:  proto.Event_CtrlH,
		Mod:  proto.Event_Alt,
	}
	protoKey3 = proto.Event{
		Type:   proto.Event_TypeMouse,
		MouseX: 10,
		MouseY: 11111,
	}
)

func assertNoLeaks(t *testing.T) {
	ignoreOpenCensus := goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start")
	goleak.VerifyNone(t, ignoreOpenCensus)
}

func newMockedClient(ctrl *gomock.Controller) (
	client *Client,
	mockCC *proto.MockClientConnInterface,
	mockMux *proto.MockMuxBroker,
) {
	mockCC = proto.NewMockClientConnInterface(ctrl)
	mockMux = proto.NewMockMuxBroker(ctrl)
	client = NewClient(mockMux, mockCC)

	mockMux.EXPECT().Cleanup(gomock.Any()).AnyTimes()
	return
}

func expectInvokeRPC(mockCC *proto.MockClientConnInterface) {
	mockCC.EXPECT().
		Invoke(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).
		Times(1).
		Return(nil)
}

func expectInvokeError(mockCC *proto.MockClientConnInterface) {
	mockCC.EXPECT().
		Invoke(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).
		Times(1).
		Return(errors.New("woopsie"))
}

func assertInvokeError(t *testing.T, err error) {
	require.Error(t, err)
	assert.Contains(t, err.Error(), "woopsie")
}

func assertClientHandlerExitClose(
	t *testing.T,
	h *TestHandler, mockWinConn *proto.MockMuxConn,
	client *Client, callClose bool,
) {
	assertClientServersEqual(t, 1, client)

	h.Exit = true

	client.mu.Lock()
	cliRes := client.servers[1]

	exit, handled := cliRes.(*handlerServerResource).h.
		Handle(term.Event{Type: term.EventKey, Key: term.KeyCtrlBackslash})
	client.mu.Unlock()
	assert.True(t, exit)
	assert.True(t, handled)

	if callClose {
		require.NoError(t, cliRes.(*handlerServerResource).h.(Handler).Close())
	}

	// unfortunately gracefulshutdowns are asynchronous
	// because they wait on grpc connection to be shutdown fist by
	// server
	time.Sleep(asyncResultsSleepDuration)

	assertClientServersEqual(t, 0, client)
}

func expectSplit(
	t *testing.T, mockCC *proto.MockClientConnInterface,
	handlerID, windowID uint64, orientation proto.Orientation,
) {
	mockCC.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq("/proto.WindowManager/Split"),
			gomock.Eq(&proto.SplitRequest{Orientation: orientation, HandlerId: handlerID}),
			gomock.Any()).
		DoAndReturn(func(
			ctx context.Context, method string, args interface{},
			reply interface{}, opts ...grpc.CallOption) error {
			splitRes, ok := reply.(*proto.SplitResponse)
			require.True(t, ok)
			splitRes.WindowId = windowID
			return nil
		}).
		Times(1)
}

func expectFocus(
	t *testing.T, mockCC *proto.MockClientConnInterface,
	handlerID, windowID uint64,
) {
	mockCC.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq("/proto.WindowManager/Focus"),
			gomock.Eq(&proto.FocusRequest{}),
			gomock.Any()).
		DoAndReturn(func(
			ctx context.Context, method string, args interface{},
			reply interface{}, opts ...grpc.CallOption) error {
			focusRes, ok := reply.(*proto.FocusResponse)
			require.True(t, ok)
			focusRes.WindowId = windowID
			return nil
		}).
		Times(1)
}

func expectWindowClose(
	t *testing.T, mockWinConn *proto.MockMuxConn, quitCh chan struct{},
) {
	mockWinConn.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq("/proto.Window/Close"),
			gomock.Eq(&proto.WindowCloseRequest{}),
			gomock.Eq(&proto.WindowCloseResponse{})).
		Times(1)
	mockWinConn.EXPECT().Close().Times(1).
		DoAndReturn(prototest.ExpectSignalExit(mockWinConn, quitCh, nil))
}

func TestClientSetMessage(t *testing.T) {
	t.Run("invokes the pbclient rpc", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		myMsg, arg1, arg2 := "oh la la: %s %d", "obla di obla da", 5
		in := &proto.SetMessageRequest{Msg: fmt.Sprintf(myMsg, arg1, arg2)}
		out := new(proto.SetMessageResponse)

		mockCC.EXPECT().
			Invoke(gomock.Any(),
				gomock.Eq("/proto.Messenger/SetMessage"),
				gomock.Eq(in), gomock.Eq(out)).
			Times(1)

		err := client.SetMessage(myMsg, arg1, arg2)
		require.NoError(t, err)
	})
	t.Run("bubbles up rpc error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		expectInvokeError(mockCC)

		err := client.SetMessage("")
		assertInvokeError(t, err)
	})
}

func expectResourceOpen(mockCC *proto.MockClientConnInterface, myResource workspace.URI) {
	in := &proto.OpenResourceRequest{Resource: myResource.String()}
	out := new(proto.OpenResourceResponse)
	mockCC.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq("/proto.ResourceOpener/Open"),
			gomock.Eq(in), gomock.Eq(out)).
		Times(1)
}

func TestClientOpen(t *testing.T) {
	t.Run("invokes the pbclient rpc", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		myResource, err := workspace.ParseURI("file:///fjkelwjfeklw")
		require.NoError(t, err)

		expectResourceOpen(mockCC, myResource)

		_, err = client.Open(myResource)
		require.NoError(t, err)
	})

	t.Run("bubbles up rpc error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		expectInvokeError(mockCC)

		_, err := client.Open(workspace.URI{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "woopsie")
	})
}

func TestClientPublish(t *testing.T) {
	tsuite := []struct {
		rpc string
		fn  func(*Client) error
		ev  *proto.Event
	}{
		{"PublishInterrupt", (*Client).PublishInterrupt, &proto.Event{Type: proto.Event_TypeInterrupt}},
		{"PublishEventNone", (*Client).PublishEventNone, &proto.Event{Type: proto.Event_TypeNone}},
	}
	for _, tcase := range tsuite {
		t.Run(fmt.Sprintf("%s bubbles up rpc error and so stops event handler resources", tcase.rpc),
			func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				client, mockCC, _ := newMockedClient(ctrl)
				mockCC.EXPECT().
					Invoke(gomock.Any(),
						gomock.Eq("/proto.EventPublisher/Publish"),
						gomock.Any(), gomock.Any()).
					Times(1).
					Return(errors.New("uRich"))

				err := tcase.fn(client)
				require.Error(t, err)
			})

		t.Run(fmt.Sprintf("%s sends interrupt event publish request to server", tcase.rpc),
			func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				client, mockCC, _ := newMockedClient(ctrl)
				ev := tcase.ev
				in := &proto.PublishRequest{Ev: ev}
				out := new(proto.PublishResponse)

				mockCC.EXPECT().
					Invoke(gomock.Any(),
						gomock.Eq("/proto.EventPublisher/Publish"),
						gomock.Eq(in), gomock.Eq(out)).
					Times(1)

				err := tcase.fn(client)
				require.NoError(t, err)
			})
	}
}

func TestClientSplitHorizontalBelow(t *testing.T) {
	testClientSplit(t, OrientationBottom, proto.Orientation_Bottom)
}

func TestClientSplitVerticalRight(t *testing.T) {
	testClientSplit(t, OrientationRight, proto.Orientation_Right)
}

func TestClientSplitHorizontalAbove(t *testing.T) {
	testClientSplit(t, OrientationTop, proto.Orientation_Top)
}

func TestClientSplitDefault(t *testing.T) {
	testClientSplit(t, OrientationDefault, proto.Orientation_Default)
}

func TestClientSplitVerticalLeft(t *testing.T) {
	testClientSplit(t, OrientationLeft, proto.Orientation_Left)
}

func assertClientClientsEqual(t *testing.T, expected int, c *Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Equal(t, expected, len(c.clients))
}

func assertClientServersEqual(t *testing.T, expected int, c *Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Equal(t, expected, len(c.servers))
}

func testClientSplit(
	t *testing.T,
	split Orientation,
	expectedSplit proto.Orientation,
) {
	t.Run("serves handler and dials to window", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		windowID := uint64(99)
		prototest.ExpectBrokerServe(t, 1, mockBroker)
		expectSplit(t, mockCC, 1, windowID, expectedSplit)
		mockWinConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, uint32(windowID))
		quitCh := prototest.ExpectMonitorConn(mockWinConn)

		win, err := client.Split(split, NewTestHandler())
		require.NoError(t, err)
		assertClientServersEqual(t, 1, client)
		assertClientClientsEqual(t, 1, client)

		// caches window clients
		expectFocus(t, mockCC, 1, windowID)
		_, err = client.Focus()
		require.NoError(t, err)
		assertClientServersEqual(t, 1, client)
		assertClientClientsEqual(t, 1, client)

		expectWindowClose(t, mockWinConn, quitCh)
		require.NoError(t, win.Close())
		time.Sleep(asyncResultsSleepDuration)

		assertClientClientsEqual(t, 0, client)
	})

	t.Run("bubbles up rpc error and so stops handler server", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		prototest.ExpectBrokerServe(t, 1, mockBroker)
		expectInvokeError(mockCC)

		win, err := client.Split(split, NewTestHandler())
		assertInvokeError(t, err)
		assert.Nil(t, win)

		assertClientServersEqual(t, 0, client)
		assertClientClientsEqual(t, 0, client)
	})

	t.Run("bubbles up dial to window error and so stops handler server", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		windowID := uint64(99)
		prototest.ExpectBrokerServe(t, 1, mockBroker)
		expectSplit(t, mockCC, 1, windowID, expectedSplit)
		prototest.ExpectBrokerDialError(t, ctrl, mockBroker, uint32(windowID))

		win, err := client.Split(split, NewTestHandler())
		require.Error(t, err)
		assert.Nil(t, win)

		assertClientServersEqual(t, 0, client)
		assertClientClientsEqual(t, 0, client)
	})

	t.Run("gracefully closes resources if handler server is called Close", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		windowID := uint64(63)
		brokerID := uint64(1)
		prototest.ExpectBrokerServe(t, uint32(brokerID), mockBroker)
		expectSplit(t, mockCC, brokerID, windowID, expectedSplit)
		mockWinConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, uint32(windowID))
		quitCh := prototest.ExpectMonitorConn(mockWinConn)

		h := NewTestHandler()
		var wg sync.WaitGroup
		h.CloseCallback = func() error {
			wg.Done()
			return nil
		}
		win, err := client.Split(split, h)
		require.NoError(t, err)

		wg.Add(1)
		mockWinConn.EXPECT().
			Invoke(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
		mockWinConn.EXPECT().Close().Times(1).
			DoAndReturn(func() error {
				h := client.servers[uint64(brokerID)].(*handlerServerResource).h.(Handler)
				err := prototest.ExpectSignalExit(mockWinConn, quitCh, nil)()
				// server would call this asynchronously
				go h.Close()
				return err
			})
		require.NoError(t, win.Close())

		wg.Wait()
		// monitor goroutine might not have called WaitForStateChange yet
		// because it's run asynchronously
		time.Sleep(asyncResultsSleepDuration)
	})

	assertNoLeaks(t)
}

func TestClientClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client, mockCC, mockBroker := newMockedClient(ctrl)

	for i := 0; i < 10; i++ {
		windowID := uint64(i)
		handlerID := uint64(i)
		prototest.ExpectBrokerServe(t, uint32(handlerID), mockBroker)
		expectSplit(t, mockCC, handlerID, windowID, proto.Orientation_Right)
		mockWinConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, uint32(windowID))
		quitCh := prototest.ExpectMonitorConn(mockWinConn)

		_, err := client.Split(OrientationRight, NewTestHandler())
		require.NoError(t, err)
		assertClientServersEqual(t, i+1, client)
		assertClientClientsEqual(t, i+1, client)

		if i%2 == 0 {
			mockWinConn.EXPECT().Close().Times(1).
				DoAndReturn(prototest.ExpectSignalExit(mockWinConn, quitCh, nil))
		} else {
			mockWinConn.EXPECT().Close().Times(1).
				DoAndReturn(prototest.ExpectSignalExit(mockWinConn, quitCh, errors.New("let's see")))
		}
	}

	err := client.Close()
	require.Error(t, err)
	assert.Contains(t, "let's see", err.Error())
	assertNoLeaks(t)
}

/* tested via ex integration tests */
func TestClientSetContent(t *testing.T) {
}
func TestClientFocus(t *testing.T) {
}
