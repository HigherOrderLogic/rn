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

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"unstable.build/go-tui/debug"
)

var _ textapi.EventHandler = (*eventStreamClient)(nil)

const (
	eventChanBuffer             = 100
)

type eventStreamClient struct {
	ctx       context.Context
	cancelCtx func()
	stream    textrpc.Editor_SubscribeEventServer
	ch        chan *textrpc.EditorEvent
}

func newEventStreamClient(
	ctx context.Context, stream textrpc.Editor_SubscribeEventServer, locker sync.Locker,
) *eventStreamClient {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan *textrpc.EditorEvent, eventChanBuffer)
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
