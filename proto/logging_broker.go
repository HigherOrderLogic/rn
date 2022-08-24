package proto

import (
	"net"

	log "github.com/sirupsen/logrus"
	grpc "google.golang.org/grpc"
)

type loggingBroker struct {
	root   MuxBroker
	logger *log.Logger
}

// LoggingBroker wraps other to provide trace-level logging.
func LoggingBroker(other MuxBroker, logger *log.Logger) MuxBroker {
	return loggingBroker{root: other, logger: logger}
}

func (b loggingBroker) NextId() uint32 {
	ret := b.root.NextId()
	b.logger.Tracef("loggingBroker: NextId(): %v", ret)
	return ret
}

func (b loggingBroker) Accept(id uint32) (net.Listener, error) {
	lis, err := b.root.Accept(id)
	b.logger.Tracef("loggingBroker: Accept(%d): (%v, %v)", id, lis, err)
	return lis, err
}

func (b loggingBroker) AcceptAndServe(ID uint32, srv func(opts []grpc.ServerOption) MuxServer) {
	b.root.AcceptAndServe(ID, srv)
	b.logger.Tracef("loggingBroker: AcceptAndServe(%d)", ID)
}

func (b loggingBroker) Dial(ID uint32) (conn MuxConn, err error) {
	conn, err = b.root.Dial(ID)
	b.logger.Tracef("loggingBroker: Dial(%d): %v", ID, err)
	return
}

func (b loggingBroker) Cleanup(ID uint32) (err error) {
	err = b.root.Cleanup(ID)
	b.logger.Tracef("loggingBroker: Cleanup(%d): %v", ID, err)
	return err
}

func (b loggingBroker) Close() error {
	err := b.root.Close()
	b.logger.Tracef("loggingBroker: Close(): %v", err)
	return err
}
