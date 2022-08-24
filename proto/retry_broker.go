package proto

import (
	context "context"
	"net"

	"github.com/ernestrc/blue/retry"
	grpc "google.golang.org/grpc"
)

type retryBroker struct {
	b MuxBroker
	s retry.Strategy
}

// WithRetryBroker wraps a MuxBroker with retry.Strategy retries.
func WithRetryBroker(b MuxBroker, strategy retry.Strategy) MuxBroker {
	return retryBroker{b: b, s: strategy}
}

func (b retryBroker) NextId() uint32 {
	return b.b.NextId()
}

func (b retryBroker) Accept(id uint32) (lis net.Listener, err error) {
	ctx := context.Background()
	retry.Retry(ctx, b.s, func(ctx context.Context) bool {
		lis, err = b.b.Accept(id)
		if err != nil {
			return true
		}
		return false
	})
	return
}

func (b retryBroker) AcceptAndServe(ID uint32, srv func(opts []grpc.ServerOption) MuxServer) {
	lis, err := b.Accept(ID)
	if err != nil {
		panic(err)
	}
	server := srv([]grpc.ServerOption{})
	go server.Serve(lis)
}

func (b retryBroker) Dial(ID uint32) (conn MuxConn, err error) {
	ctx := context.Background()
	retry.Retry(ctx, b.s, func(ctx context.Context) bool {
		conn, err = b.b.Dial(ID)
		if err != nil {
			return true
		}
		return false
	})
	return
}

func (b retryBroker) Cleanup(ID uint32) error {
	return b.b.Cleanup(ID)
}

func (b retryBroker) Close() error {
	return b.b.Close()
}
