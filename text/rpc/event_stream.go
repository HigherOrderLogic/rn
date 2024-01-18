package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	textapi "unstable.build/go-tui/api/text"
)

var _ textapi.EventHandler = (*eventStreamClient)(nil)

const handleReceiveMessageTimeout = 1 * time.Second

type eventStreamClient struct {
	stream Editor_SubscribeServer
	quit   atomic.Bool
}

func newEventStreamClient(
	stream Editor_SubscribeServer, locker sync.Locker,
) *eventStreamClient {
	ret := &eventStreamClient{stream: stream}
	return ret
}

func (e *eventStreamClient) Handle(ctx context.Context, ev textapi.Event) bool {
	e.log(log.TraceLevel, "handle %v", ev.Type)
	if e.quit.Load() {
		e.log(log.TraceLevel, "unsubscribing")
		return true
	}
	protoEv := toProto(ev)

	// do not unlock I/O mutex here, as it might introduce
	// race conditions and violate invariants that are quite hard
	// to debug.

	err := e.stream.Send(&protoEv)
	if err != nil {
		e.log(log.ErrorLevel, "stream send: %v", err)
		return true
	}
	return false
}

func (e *eventStreamClient) waitForUnsubscribe() error {
	defer e.quit.Store(true)

	// do not unlock here, Server should already have unlocked
	req, err := e.stream.Recv()
	if err != nil {
		return fmt.Errorf("stream receive: %v", err)
	}
	if !req.GetUnsubscribe() {
		e.log(log.WarnLevel, "received message non-unsubscribe request")
	}

	// wait for CloseSend
	if _, err = e.stream.Recv(); err != io.EOF {
		return fmt.Errorf("stream receive: %v", err)
	}

	return nil
}

func (e *eventStreamClient) log(level log.Level, msg string, args ...interface{}) {
	log.WithFields(log.Fields{logging.KeyClass: "textpb.eventStreamClient"}).
		Logf(level, msg, args...)
}

type eventStreamServer struct {
	stream    Editor_SubscribeClient
	parentCtx context.Context
	handler   textapi.EventHandler
}

func newEventStreamServer(
	parentCtx context.Context, stream Editor_SubscribeClient,
	handler textapi.EventHandler,
) eventStreamServer {
	return eventStreamServer{
		stream:    stream,
		parentCtx: parentCtx,
		handler:   handler,
	}
}

func (s eventStreamServer) log(level log.Level, msg string, args ...interface{}) {
	log.WithFields(log.Fields{logging.KeyClass: "textpb.eventStreamServer"}).
		Logf(level, msg, args...)
}

func (s eventStreamServer) receiveEvents(c *Client) {
	defer s.log(log.TraceLevel, "done receiving events")
	for {
		protoEv, err := s.stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) && status.Code(err) != codes.Canceled {
				s.log(log.ErrorLevel, "stream recv error: %v", err)
			}
			break
		}

		var ev textapi.Event
		if err := fromProto(&ev, protoEv); err != nil {
			s.log(log.ErrorLevel, "decode proto event: %v", err)
			continue
		}

		// set a timeout to prevent a deadlock via client stream buffer exhaustion
		ctx, cancel := context.WithTimeout(s.parentCtx, handleReceiveMessageTimeout)
		s.log(log.TraceLevel, "handle %v", ev.Type)
		exit := s.handler.Handle(ctx, ev)
		cancel()
		if err := ctx.Err(); err != nil {
			s.log(log.ErrorLevel, "could not dispatch event %v in time: %v", ev.Type, err)
			select {
			case <-s.parentCtx.Done():
				return
			default:
			}
		}
		if exit {
			break
		}
	}

	// do not attempt to send unsubscribe if client is closing
	select {
	case <-s.parentCtx.Done():
		return
	default:
	}

	req := EditorSubscribeRequest{Unsubscribe: true}
	s.log(log.TraceLevel, "send unsubscribe")
	err := s.stream.Send(&req)
	if err != nil {
		s.log(log.ErrorLevel, "send unsubscribe: %v", err)
	}
	if err := s.stream.CloseSend(); err != nil {
		s.log(log.ErrorLevel, "stream close send: %v", err)
	}
	// keep alive until we're done streaming events
	runtime.KeepAlive(c)
}
