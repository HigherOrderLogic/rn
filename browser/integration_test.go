package browser

import (
	"net"
	"sync"
	"testing"

	bproto "github.com/ernestrc/blue/rpc"
	"github.com/ernestrc/go-tui/proto"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func newClientServerIntegration(
	t *testing.T, h Browser,
) (*Client, func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	broker := proto.NewDialBroker()
	mutex := new(sync.Mutex)

	grpcServer := grpc.NewServer()
	rpcServer := NewServer(broker, h, mutex)
	proto.RegisterWindowManagerServer(grpcServer, rpcServer)
	proto.RegisterResourceOpenerServer(grpcServer, rpcServer)
	proto.RegisterMessengerServer(grpcServer, rpcServer)
	proto.RegisterEventPublisherServer(grpcServer, rpcServer)
	bproto.RegisterDocumentStoreServer(grpcServer, rpcServer)

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	client := NewClient(broker, conn)

	closeFn := func() {
		client.Close()
		rpcServer.Close()
		grpcServer.Stop()
		conn.Close()
	}

	return client, closeFn
}

func TestIntegrationSetFocus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockBrowser(ctrl)
	client, cleanup := newClientServerIntegration(t, mock)
	defer cleanup()

	win1 := NopWindow()
	win2 := NopWindow()

	mock.EXPECT().Focus().Return(win1, nil)
	resWin1, err := client.Focus()
	require.NoError(t, err)

	mock.EXPECT().Split(gomock.Any(), gomock.Any()).Return(win2, nil)
	resWin2, err := client.Split(OrientationDefault, nil)
	require.NoError(t, err)

	mock.EXPECT().SetFocus(gomock.Any()).Return(win2, nil)
	resPrev, err := client.SetFocus(resWin1)
	require.NoError(t, err)
	assert.Equal(t, resWin2, resPrev)

	mock.EXPECT().SetFocus(gomock.Any()).Return(win1, nil)
	resPrev, err = client.SetFocus(resWin2)
	require.NoError(t, err)
	assert.Equal(t, resWin1, resPrev)

	mock.EXPECT().Close()
}
