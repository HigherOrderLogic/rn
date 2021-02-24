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
	client *Client, callOnUnmount bool,
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

	if callOnUnmount {
		require.NoError(t, cliRes.(*handlerServerResource).h.(Handler).OnUnmount())
	}

	// unfortunately gracefulshutdowns are asynchronous
	// because they wait on grpc connection to be shutdown fist by
	// server
	time.Sleep(asyncResultsSleepDuration)

	assertClientServersEqual(t, 0, client)
}

func expectSplit(
	t *testing.T, mockCC *proto.MockClientConnInterface,
	handlerID, windowID uint64, rpc string,
) {
	mockCC.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq(rpc),
			gomock.Eq(&proto.SplitRequest{HandlerId: handlerID}),
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

func TestClientMergeKeyMap(t *testing.T) {
	fixture := make(map[term.Event]term.Event)
	fixture[key1] = key2
	fixture[key2] = key3
	fixture[key3] = key1

	t.Run("maps term mappings to a proto merge key map request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		out := new(proto.MergeKeyMapResponse)

		expectedReq := []*proto.Mapping{
			{From: &protoKey1, To: &protoKey2},
			{From: &protoKey2, To: &protoKey3},
			{From: &protoKey3, To: &protoKey1},
		}

		mockCC.EXPECT().
			Invoke(gomock.Any(),
				gomock.Eq("/proto.KeyMapper/MergeKeyMap"), gomock.Any(),
				gomock.Eq(out)).
			DoAndReturn(func(ctx context.Context,
				method string, args interface{},
				reply interface{}, opts ...grpc.CallOption) error {
				// validate mappings with finer grained control
				req, ok := args.(*proto.MergeKeyMapRequest)
				require.True(t, ok)

				assert.ElementsMatch(t, expectedReq, req.Mappings)
				return nil
			}).
			Times(1)

		err := client.MergeKeyMap(fixture)
		require.NoError(t, err)
	})

	t.Run("bubbles up grpc error to caller", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		expectInvokeError(mockCC)

		err := client.MergeKeyMap(fixture)
		assertInvokeError(t, err)
	})

	t.Run("sends request anyway if map is nil or empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)
		mockCC.EXPECT().
			Invoke(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).
			Times(2)

		{
			err := client.MergeKeyMap(make(map[term.Event]term.Event))
			assert.NoError(t, err)
		}

		{
			err := client.MergeKeyMap(nil)
			assert.NoError(t, err)
		}
	})
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

func expectResourceOpen(mockCC *proto.MockClientConnInterface, myResource string) {
	in := &proto.OpenResourceRequest{Resource: myResource}
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

		myResource := "fjkelwjfeklw"

		expectResourceOpen(mockCC, myResource)

		_, err := client.Open(myResource)
		require.NoError(t, err)
	})

	t.Run("bubbles up rpc error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)

		expectInvokeError(mockCC)

		_, err := client.Open("")
		require.Error(t, err)
		require.Contains(t, err.Error(), "woopsie")
	})
}

func TestClientSubscribe(t *testing.T) {
	t.Run("bubbles up rpc error and so stops event handler resources", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		prototest.ExpectBrokerServe(t, 1, mockBroker)
		expectInvokeError(mockCC)

		err := client.Subscribe(term.Event{}, nil)
		assertInvokeError(t, err)

		assertClientServersEqual(t, 0, client)
		assertClientClientsEqual(t, 0, client)
	})

	t.Run("sends event subscribe request to server", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		prototest.ExpectBrokerServe(t, 1, mockBroker)

		in := &proto.SubscribeRequest{Ev: &proto.Event{Char: 'a'}, HandlerId: 1}
		out := new(proto.SubscribeResponse)

		mockCC.EXPECT().
			Invoke(gomock.Any(),
				gomock.Eq("/proto.EventSubscriber/Subscribe"),
				gomock.Eq(in), gomock.Eq(out)).
			Times(1)

		h := NewTestHandler()
		err := client.Subscribe(term.Event{Ch: 'a'}, handlerToEventHandler{h})
		require.NoError(t, err)

		assertClientHandlerExitClose(t, h, nil, client, false)
	})
}

func TestClientPublish(t *testing.T) {
	t.Run("bubbles up rpc error and so stops event handler resources", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)
		mockCC.EXPECT().
			Invoke(gomock.Any(),
				gomock.Eq("/proto.EventPublisher/Publish"),
				gomock.Any(), gomock.Any()).
			Times(1).
			Return(errors.New("uRich"))

		err := client.PublishInterrupt()
		require.Error(t, err)
	})

	t.Run("sends interrupt event publish request to server", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, _ := newMockedClient(ctrl)
		ev := proto.Event{Type: proto.Event_TypeInterrupt}
		in := &proto.PublishRequest{Ev: &ev}
		out := new(proto.PublishResponse)

		mockCC.EXPECT().
			Invoke(gomock.Any(),
				gomock.Eq("/proto.EventPublisher/Publish"),
				gomock.Eq(in), gomock.Eq(out)).
			Times(1)

		err := client.PublishInterrupt()
		require.NoError(t, err)
	})
}

func TestClientSplitHorizontalBelow(t *testing.T) {
	rpc := "/proto.WindowManager/SplitHorizontalBelow"
	testClientSplit(t, (WindowManager).SplitHorizontalBelow, rpc)
}

func TestClientSplitVerticalRight(t *testing.T) {
	rpc := "/proto.WindowManager/SplitVerticalRight"
	testClientSplit(t, (WindowManager).SplitVerticalRight, rpc)
}

func TestClientSplitHorizontalAbove(t *testing.T) {
	rpc := "/proto.WindowManager/SplitHorizontalAbove"
	testClientSplit(t, (WindowManager).SplitHorizontalAbove, rpc)
}

func TestClientSplitVerticalLeft(t *testing.T) {
	rpc := "/proto.WindowManager/SplitVerticalLeft"
	testClientSplit(t, (WindowManager).SplitVerticalLeft, rpc)
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
	split func(WindowManager, Handler) (Window, error),
	rpc string,
) {
	t.Run("serves handler and dials to window", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		windowID := uint64(99)
		prototest.ExpectBrokerServe(t, 1, mockBroker)
		expectSplit(t, mockCC, 1, windowID, rpc)
		mockWinConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, uint32(windowID))
		quitCh := prototest.ExpectMonitorConn(mockWinConn)

		win, err := split(client, NewTestHandler())
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

		win, err := split(client, NewTestHandler())
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
		expectSplit(t, mockCC, 1, windowID, rpc)
		prototest.ExpectBrokerDialError(t, ctrl, mockBroker, uint32(windowID))

		win, err := split(client, NewTestHandler())
		require.Error(t, err)
		assert.Nil(t, win)

		assertClientServersEqual(t, 0, client)
		assertClientClientsEqual(t, 0, client)
	})

	t.Run("gracefully closes resources if handler server is called OnUnmount", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client, mockCC, mockBroker := newMockedClient(ctrl)

		windowID := uint64(63)
		brokerID := uint64(1)
		prototest.ExpectBrokerServe(t, uint32(brokerID), mockBroker)
		expectSplit(t, mockCC, brokerID, windowID, rpc)
		mockWinConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, uint32(windowID))
		quitCh := prototest.ExpectMonitorConn(mockWinConn)

		h := NewTestHandler()
		var wg sync.WaitGroup
		h.OnUnmountCallback = func() error {
			wg.Done()
			return nil
		}
		win, err := split(client, h)
		require.NoError(t, err)

		wg.Add(1)
		mockWinConn.EXPECT().
			Invoke(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
		mockWinConn.EXPECT().Close().Times(1).
			DoAndReturn(func() error {
				h := client.servers[uint64(brokerID)].(*handlerServerResource).h.(Handler)
				err := prototest.ExpectSignalExit(mockWinConn, quitCh, nil)()
				// server would call this asynchronously
				go h.OnUnmount()
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
		expectSplit(t, mockCC, handlerID, windowID,
			"/proto.WindowManager/SplitVerticalRight")
		mockWinConn := prototest.ExpectBrokerDial(t, ctrl, mockBroker, uint32(windowID))
		quitCh := prototest.ExpectMonitorConn(mockWinConn)

		_, err := client.SplitVerticalRight(NewTestHandler())
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

func TestClientSetContent(t *testing.T) {
	/* tested via ex integration tests */
}
