package proto

import (
	context "context"
	"io"
	"net"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// MuxConn is a MuxBroker connection
type MuxConn interface {
	grpc.ClientConnInterface
	GetState() connectivity.State
	WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool
	io.Closer
}

type MuxServer interface {
	GracefulStop()
	Serve(lis net.Listener) error
	Stop()
	GRPC() *grpc.Server
}

// MuxBroker allows a client or server to multiplex over connections.
type MuxBroker interface {
	// NextId returns the next id to be used to Serve/Dial.
	// The returned value must always be > 0.
	NextId() uint32
	Accept(id uint32) (net.Listener, error)
	AcceptAndServe(ID uint32, srv func(opts []grpc.ServerOption) MuxServer)
	Dial(ID uint32) (conn MuxConn, err error)
	Cleanup(ID uint32) error
	Close() error
}
