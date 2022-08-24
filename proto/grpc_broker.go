package proto

import (
	"github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// used to adapt to plugin.GRPCBroker to MuxBroker
type grpcBroker struct {
	*plugin.GRPCBroker
	logger *log.Logger
}

func (p grpcBroker) Dial(ID uint32) (
	conn MuxConn, err error,
) {
	conn, err = p.GRPCBroker.Dial(ID)
	if err != nil {
		return
	}

	if p.logger != nil && p.logger.IsLevelEnabled(log.TraceLevel) {
		conn = loggingConn{p.logger, conn}
	}
	return
}
func (p grpcBroker) AcceptAndServe(
	ID uint32, srv func(opts []grpc.ServerOption) MuxServer,
) {
	p.GRPCBroker.AcceptAndServe(ID, func(opts []grpc.ServerOption) *grpc.Server {
		return srv(opts).GRPC()
	})
}

func (p grpcBroker) Cleanup(ID uint32) error {
	return nil
}

// GRPCBroker adapts the signature of plugin.GRPCBroker to satisfy MuxBroker.
// It also wraps the connections returned by Dial, with a trace-level logging
// connection.
func GRPCBroker(broker *plugin.GRPCBroker, logger *log.Logger) MuxBroker {
	return grpcBroker{broker, logger}
}
