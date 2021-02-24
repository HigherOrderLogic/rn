package editor

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/proto"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func newClientServerIntegration(
	t *testing.T, h *MockEventHandler,
	serverQuitCallback, clientQuitCallback func(),
) (*eventHandlerClient, func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	server := newEventHandlerServer(h, serverQuitCallback)
	proto.RegisterEditorEventHandlerServer(grpcServer, server)

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	client := newEventHandlerClient(conn, clientQuitCallback)

	closeFn := func() {
		client.Close()
		grpcServer.Stop()
		conn.Close()
	}

	return client, closeFn
}

func consumeError(t *testing.T, wg *sync.WaitGroup, client *eventHandlerClient) {
	select {
	case err := <-client.errors():
		wg.Done()
		assert.NoError(t, err)
	case <-client.quitChan:
	}
}

func TestEventHandlerRPC(t *testing.T) {
	ev := Event{
		Type:         EventTypeFlush,
		ResourceName: "myResourceName",
		Resource:     browser.Token{ID: 1},
	}

	t.Run("asynchronously dispatches events to remote event handler", func(t *testing.T) {
		var wg sync.WaitGroup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		h := NewMockEventHandler(ctrl)

		client, closeFn := newClientServerIntegration(t, h, func() {}, func() {})
		defer closeFn()

		go consumeError(t, &wg, client)

		h.EXPECT().Handle(gomock.Eq(ev)).DoAndReturn(func(ev Event) bool {
			wg.Done()
			return false
		}).Times(1)

		wg.Add(1)
		exit := client.Handle(ev)
		assert.False(t, exit)

		wg.Wait()
	})

	t.Run("client/server call exit callback if remote handler returns true to a Handle call", func(t *testing.T) {
		var wg sync.WaitGroup

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		h := NewMockEventHandler(ctrl)

		client, closeFn := newClientServerIntegration(t, h, wg.Done, wg.Done)
		defer closeFn()

		go consumeError(t, &wg, client)

		h.EXPECT().Handle(gomock.Eq(ev)).Return(true).Times(1)

		wg.Add(2)
		exit := client.Handle(ev)
		assert.False(t, exit)

		wg.Wait()
	})

	t.Run("dispatches errors through error channel", func(t *testing.T) {
		var wg sync.WaitGroup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		h := NewMockEventHandler(ctrl)

		client, closeFn := newClientServerIntegration(t, h, func() {}, func() {})
		defer closeFn()

		go func() {
			defer wg.Done()
			err := <-client.errors()
			assert.Error(t, err)
		}()

		// should cause failure
		client.conn.(io.Closer).Close()
		time.Sleep(gracefulShutdownWait)

		wg.Add(1)
		exit := client.Handle(ev)
		assert.False(t, exit)

		wg.Wait()
	})
}
