package editor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ernestrc/go-tui/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	defaultClientTimeout    = 4 * time.Second
	handleBackpressureThres = 128
)

type eventHandlerClient struct {
	conn         grpc.ClientConnInterface
	pb           proto.EditorEventHandlerClient
	evChan       chan proto.EditorEvent
	errChan      chan error
	quitChan     chan struct{}
	quitCallback func()
	logger       *log.Logger
}

func newEventHandlerClient(
	cc grpc.ClientConnInterface, quitCallback func(),
) *eventHandlerClient {
	ret := new(eventHandlerClient)
	ret.pb = proto.NewEditorEventHandlerClient(cc)
	ret.errChan = make(chan error)
	ret.quitChan = make(chan struct{})
	ret.evChan = make(chan proto.EditorEvent, handleBackpressureThres)
	ret.quitCallback = quitCallback
	ret.conn = cc

	go ret.pipelineEvents()

	return ret
}

func (c *eventHandlerClient) errors() <-chan error {
	return c.errChan
}

func (c *eventHandlerClient) handleError(err error) {
	if c.logger != nil {
		c.logger.Errorf("editor.eventHandlerClient.Handle error: %v", err)
	}

	select {
	case c.errChan <- err:
	default:
	}
}

func (c *eventHandlerClient) pipelineEvents() {
	var protoEv proto.EditorEvent
	for {
		select {
		case <-c.quitChan:
			return
		case protoEv = <-c.evChan:
		}

		ctx := context.Background()
		ctx, cancelFn := context.WithTimeout(ctx, defaultClientTimeout)
		req := proto.EditorEventHandleRequest{Event: &protoEv}
		resp, err := c.pb.Handle(ctx, &req)
		cancelFn()
		if err != nil {
			c.handleError(err)
		} else if resp.GetQuit() {
			c.quitCallback()
			return
		}
	}
}

func (c *eventHandlerClient) Handle(ev Event) bool {
	protoEv := ev.toProto()
	c.evChan <- protoEv

	return false
}

func (c *eventHandlerClient) Close() error {
	close(c.quitChan)
	// if Handle is called after close, then we want to panic
	// to indicate programmer error.
	close(c.evChan)
	return nil
}

type eventHandlerServer struct {
	handler EventHandler
	logger  *log.Logger
	onExit  func()
}

func newEventHandlerServer(
	handler EventHandler, onExit func(),
) *eventHandlerServer {
	ret := new(eventHandlerServer)
	ret.handler = handler
	ret.onExit = onExit
	return ret
}

func (s *eventHandlerServer) Handle(
	ctx context.Context, req *proto.EditorEventHandleRequest,
) (*proto.EditorEventHandleResponse, error) {
	protoEv := req.GetEvent()
	if protoEv == nil {
		return nil, errors.New("invalid handle request: missing event property")
	}

	var ev Event
	err := ev.fromProto(protoEv)
	if err != nil {
		err = fmt.Errorf("failed to decode proto event: %s", err)
		return nil, err
	}

	quit := s.handler.Handle(ev)

	resp := &proto.EditorEventHandleResponse{Quit: quit}

	if quit {
		s.onExit()
	}
	return resp, nil
}
