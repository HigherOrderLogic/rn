// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package textrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/debug"
)

var _ textapi.EventHandler = (*eventStreamClient)(nil)

const (
	handleReceiveMessageTimeout = 3 * time.Second
	eventChanBuffer             = 100
)

type eventStreamClient struct {
	ctx       context.Context
	cancelCtx func()
	stream    Editor_SubscribeEventServer
	ch        chan *EditorEvent
}

func newEventStreamClient(
	ctx context.Context, stream Editor_SubscribeEventServer, locker sync.Locker,
) *eventStreamClient {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan *EditorEvent, eventChanBuffer)
	ret := &eventStreamClient{
		stream:    stream,
		ctx:       ctx,
		cancelCtx: cancel,
		ch:        ch,
	}
	go debug.CapturePanicReport(func() {
		ret.sendMessages(ctx)
	})
	return ret
}

func (e *eventStreamClient) sendMessages(ctx context.Context) {
	for {
		select {
		case ev := <-e.ch:
			err := e.stream.Send(ev)
			if err != nil {
				e.log(log.ErrorLevel, "stop sending messages: stream send: %v", err)
				return
			}
		case <-ctx.Done():
			e.log(log.TraceLevel, "stop sending messages: %v", ctx.Err())
			return
		}
	}
}

func (e *eventStreamClient) Handle(ctx context.Context, ev textapi.Event) bool {
	e.log(log.TraceLevel, "handle %v", ev.Type)
	protoEv := toProto(ev)

	// do not unlock I/O mutex here, as it might introduce
	// race conditions and violate invariants that are quite hard
	// to debug.

	select {
	case e.ch <- &protoEv:
	case <-e.ctx.Done():
		e.log(log.TraceLevel, "unsubscribing")
		return true
	default:
		e.log(log.ErrorLevel, "event stream is lagging behind: dropping messages")
	}
	return false
}

func (e *eventStreamClient) Close() error {
	e.cancelCtx()
	return nil
}

func (e *eventStreamClient) waitForUnsubscribe() error {
	req, err := e.stream.Recv()
	if err != nil {
		return fmt.Errorf("stream receive: %v", err)
	}
	if !req.GetUnsubscribe() {
		e.log(log.WarnLevel, "received message non-unsubscribe request")
	}

	// wait for CloseSend
	if _, err = e.stream.Recv(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("stream receive: %v", err)
	}

	return nil
}

func (e *eventStreamClient) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{logging.KeyClass: "textrpc.eventStreamClient"}).
		Logf(level, msg, args...)
}

type eventStreamServer struct {
	stream    Editor_SubscribeEventClient
	parentCtx context.Context
	handler   textapi.EventHandler
}

func newEventStreamServer(
	parentCtx context.Context, stream Editor_SubscribeEventClient,
	handler textapi.EventHandler,
) eventStreamServer {
	return eventStreamServer{
		stream:    stream,
		parentCtx: parentCtx,
		handler:   handler,
	}
}

func (s eventStreamServer) log(level log.Level, msg string, args ...interface{}) {
	log.WithFields(log.Fields{logging.KeyClass: "textrpc.eventStreamServer"}).
		Logf(level, msg, args...)
}

func (s eventStreamServer) receiveEvents() {
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
		if err := ctx.Err(); err != nil {
			s.log(log.ErrorLevel, "could not dispatch event %v in time: %v", ev.Type, err)
			select {
			case <-s.parentCtx.Done():
				cancel()
				return
			default:
			}
		}
		cancel()
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

	req := SubscribeEventRequest{Unsubscribe: true}
	s.log(log.TraceLevel, "send unsubscribe")
	err := s.stream.Send(&req)
	if err != nil {
		s.log(log.ErrorLevel, "send unsubscribe: %v", err)
	}
	if err := s.stream.CloseSend(); err != nil {
		s.log(log.ErrorLevel, "stream close send: %v", err)
	}
}
