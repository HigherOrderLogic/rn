package handler

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
)

const testHandlerManualDesc = "remote SUPER plugin"
const drawDispatchWaitTime = 20 * time.Millisecond

var testHandlerKeys tui.KeyMap

func init() {
	testHandlerKeys = make(tui.KeyMap)
	ev1 := term.KeyComb{Key: term.KeyCtrlSpace}
	ev2 := term.KeyComb{Mod: term.ModAlt, Ch: '@'}
	testHandlerKeys[ev1] = tui.EventDesc{ID: "sup", Description: "media soup"}
	testHandlerKeys[ev2] = tui.EventDesc{ID: "hiperio", Description: "is dead; or is it?"}
}

func testHandler() tui.Handler {
	ret := NewTestHandler()
	ret.Manual.Summary = testHandlerManualDesc
	ret.Manual.Keys = testHandlerKeys
	return ret
}

func newStubClient(t *testing.T) *Client {
	return &Client{
		client: &mockHandlerClient{
			remote: testHandler(),
		},
	}
}

func assertTestManual(t *testing.T, man tui.Manual, msg ...interface{}) {
	expected := tui.Manual{
		Summary: testHandlerManualDesc,
		Keys:    testHandlerKeys,
	}
	assert.Equal(t, expected, man)
}

func testRPCHandlerManual(t *testing.T, rpcHandler *Client) {
	assertTestManual(t, rpcHandler.Man(), rpcHandler.errors)
}

func testRPCHandlerCursor(t *testing.T, rpcHandler *Client) {
	// force call to underlying Cursor on the server side
	rpcHandler.Draw(term.NewStringWriter(0, 0))

	// give some time for the client to asynchronously receive it
	time.Sleep(drawDispatchWaitTime)

	// then force collect cursor response
	rpcHandler.Draw(term.NewStringWriter(0, 0))

	pos, ok := rpcHandler.Cursor()
	require.False(t, ok)

	assert.Equal(t, term.Coordinates{X: -1, Y: -1}, pos)
}

type testResizeHandler struct {
	TestHandler
	width, height int
}

func (h *testResizeHandler) Resize(width, height int) {
	if h.width == width && h.height == height {
		panic("called redundant Resized")
	}
	h.width = width
	h.height = height
}

func TestClientHandlerDraw(t *testing.T) {
	stubClient := newStubClient(t)
	defer stubClient.Close()

	t.Run("draw", func(t *testing.T) {
		cases := []testutil.HandlerSequenceTestCase{
			{"",
				`AAAA
AAAA
AAAA
AAAA`}, {"j",
				`BBBB
BBBB
BBBB
BBBB`},
		}

		testutil.TestHandlerSequence(t, stubClient, 4, 4, cases)
	})

	t.Run("calls resize only when dimensions have changed", func(t *testing.T) {
		h := new(testResizeHandler)
		client, cleanup := newServerClient(t, h)
		defer client.Close()
		defer cleanup()

		client.Resize(10, 6)
		client.Handle(term.Event{Ch: 'h'})
		client.Handle(term.Event{Ch: 'j'})

		client.Resize(10, 6)
		client.Handle(term.Event{Ch: 'k'})
	})
}

func TestUnitClientHandlerManual(t *testing.T) {
	stubClient := newStubClient(t)
	defer stubClient.Close()
	testRPCHandlerManual(t, stubClient)
}

func TestUnitClientHandlerCursor(t *testing.T) {
	stubClient := newStubClient(t)
	defer stubClient.Close()
	testRPCHandlerCursor(t, stubClient)
}

func TestClientHandleErrors(t *testing.T) {
	myErr := errors.New("functional programming is overrated")
	stubClient := NewClient(&mockHandlerClient{
		remote:   testHandler(),
		rpcError: myErr,
	})
	defer stubClient.Close()
	errChan := stubClient.Errors()

	go func() {
		_ = stubClient.Man()
	}()

	assert.Equal(t, myErr, <-errChan)
}

func newServerClient(t *testing.T, testHandler tui.Handler) (*Client, func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	proto.RegisterHandlerServer(grpcServer, NewServer(testHandler))

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	return NewClient(proto.NewHandlerClient(conn)), func() {
		conn.Close()
		grpcServer.Stop()
	}
}

func TestIntegrationClientHandlerDraw(t *testing.T) {
	serverClient, closeFn := newServerClient(t, testHandler())
	defer closeFn()
	defer serverClient.Close()

	cases := []testutil.HandlerSequenceTestCase{
		{"",
			`AAAA
AAAA
AAAA
AAAA`},
		{"j",
			`BBBB
BBBB
BBBB
BBBB`},
		{"",
			`BBBB
BBBB
BBBB
BBBB`},
	}

	width, height := 4, 4
	writer := term.NewStringWriter(width, height)
	serverClient.Resize(width, height)

	for _, tcase := range cases {
		err := writer.Clear(term.Attributes{Fg: 0, Bg: 0})
		require.NoError(t, err)

		for _, r := range tcase.InputSequence {
			serverClient.Handle(term.Event{Ch: r, Type: term.EventKey})
			time.Sleep(drawDispatchWaitTime)
		}

		serverClient.Draw(writer)
		time.Sleep(drawDispatchWaitTime)

		err = writer.Flush()
		require.NoError(t, err)

		out := writer.String()
		assert.Equal(t, tcase.Expected, out, serverClient.errors)
	}
}

func TestIntegrationClientHandlerManual(t *testing.T) {
	serverClient, closeFn := newServerClient(t, testHandler())
	defer closeFn()
	defer serverClient.Close()
	testRPCHandlerManual(t, serverClient)
}

func TestIntegrationClientHandlerCursor(t *testing.T) {
	serverClient, closeFn := newServerClient(t, testHandler())
	defer closeFn()
	defer serverClient.Close()
	testRPCHandlerCursor(t, serverClient)
}

func TestMain(m *testing.M) {
	ignoreOpenCensus := goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start")
	goleak.VerifyTestMain(m, ignoreOpenCensus)
}
