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
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser/browserrpc"
	"unstable.build/go-tui/debug"
	termrpc "unstable.build/go-tui/term/termrpc"
)

type commandClientStream struct {
	ctx           context.Context
	handleCommand chan string
	stream        serverStream
	completers    sync.Map
	counter       int64
}

// subset of Editor_SubscribeCommandServer
type serverStream interface {
	RecvMsg(any) error
	Send(*ServerCommandMessage) error
}

func newCommandClientStream(
	ctx context.Context, stream serverStream,
) *commandClientStream {
	return &commandClientStream{
		stream:        stream,
		ctx:           ctx,
		handleCommand: make(chan string),
	}
}

func (c *commandClientStream) receiveMessages() error {
	for {
		var msg ClientCommandMessage
		err := c.stream.RecvMsg(&msg)
		if err != nil {
			return fmt.Errorf("receive stream message: %w", err)
		}

		switch msg.GetType() {
		case ClientCommandMessage_Handle:
			select {
			case c.handleCommand <- msg.GetHandle().GetError():
			case <-c.ctx.Done():
				return c.ctx.Err()
			}
		case ClientCommandMessage_CompleteValue:
			complete := msg.GetCompleteValue()
			id := complete.GetId()
			if id == 0 {
				c.log(log.WarnLevel, "received complete value message with invalid id")
				continue
			}
			val, ok := c.completers.Load(id)
			if !ok {
				c.log(log.WarnLevel, "received complete message for unknown stream")
				continue
			}
			chanCtx := val.(chanCtx)
			chanValue := chanValue{
				val: complete.GetValue(),
			}
			select {
			case chanCtx.ch <- chanValue:
			case <-chanCtx.ctx.Done():
				continue
			case <-c.ctx.Done():
				return c.ctx.Err()
			}
		case ClientCommandMessage_CompleteDone:
			done := msg.GetCompleteDone()
			id := done.GetId()
			if id == 0 {
				c.log(log.WarnLevel, "received complete done message with invalid id")
				continue
			}
			val, ok := c.completers.LoadAndDelete(id)
			if !ok {
				continue
			}
			chanCtx := val.(chanCtx)
			errStr := done.GetError()
			if errStr == "" {
				close(chanCtx.ch)
				continue
			}

			chanValue := chanValue{
				err: errors.New(errStr),
			}
			select {
			case chanCtx.ch <- chanValue:
				continue
			case <-chanCtx.ctx.Done():
				continue
			case <-c.ctx.Done():
				return c.ctx.Err()
			}
		default:
			c.log(log.WarnLevel, "received extraneous message type: %d", msg.GetType())
		}
	}
}

func (c *commandClientStream) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) error {
	var cursorContent, cursorWindow termrpc.Coordinates
	cursorContent.FromModel(cmd.Cursor.Content)
	cursorWindow.FromModel(cmd.Cursor.Window)

	req := HandleCommandRequest{
		Name:          cmd.Name,
		Args:          cmd.Args,
		CursorContent: &cursorContent,
		CursorWindow:  &cursorWindow,
	}
	if cmd.Window != nil {
		req.WindowId = cmd.Window.WindowID()
	}
	if cmd.URI != (workspaceapi.URI{}) {
		req.ResourceName = NewURI(cmd.URI)
	}

	var reqMsg ServerCommandMessage
	reqMsg.Type = ServerCommandMessage_Handle
	reqMsg.Handle = &req

	if err := c.stream.Send(&reqMsg); err != nil {
		return fmt.Errorf("send complete request: %w", err)
	}

	return nil
}

func (c *commandClientStream) Complete(ctx context.Context, cmd string, args []string) (
	iterator.Iterator[string], string, error,
) {
	c.counter++ // start with 1, so 0 is a missing ID error
	id := c.counter

	var req CompleteCommandRequest
	req.Id = id
	req.Name = cmd
	req.Args = args

	var reqMsg ServerCommandMessage
	reqMsg.Type = ServerCommandMessage_Complete
	reqMsg.Complete = &req

	if err := c.stream.Send(&reqMsg); err != nil {
		return nil, "", fmt.Errorf("send complete request: %w", err)
	}

	ctx, cancelCtx := context.WithCancel(ctx)
	ch := make(chan chanValue)
	c.completers.Store(id, chanCtx{ctx: ctx, ch: ch})

	return iterator.FromFunc(func(ctx context.Context) (string, bool, error) {
		select {
		case next, ok := <-ch:
			if !ok {
				return "", false, nil
			}
			if next.err != nil {
				close(ch) // force next to return !ok, in case of poor impls
				return "", false, next.err
			}
			return next.val, true, nil
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}, func() error {
		cancelCtx()
		return nil
	}), "", nil
}

func (c *commandClientStream) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "textrpc.commandClientStream").Logf(level, msg, args...)
}

// subset of Editor_SubscribeCommandClient
type clientStream interface {
	RecvMsg(any) error
	Send(*ClientCommandMessage) error
	CloseSend() error
}

type commandServerStream struct {
	ctx       context.Context
	cancelCtx func()
	stream    clientStream
	sendChan  chan *ClientCommandMessage
	h         textapi.CommandHandler
}

func newCommandServerStream(
	ctx context.Context,
	stream clientStream,
	h textapi.CommandHandler,
) *commandServerStream {
	ctx, cancelCtx := context.WithCancel(ctx)
	return &commandServerStream{
		stream:    stream,
		h:         h,
		cancelCtx: cancelCtx,
		ctx:       ctx,
		sendChan:  make(chan *ClientCommandMessage),
	}
}

func (s *commandServerStream) sendMessages() {
	defer func() {
		_ = s.stream.CloseSend()
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.sendChan:
			err := s.stream.Send(msg)
			if err != nil {
				s.log(log.ErrorLevel,
					"unable to send stream message back, terminating stream: %v", err)
				s.cancelCtx()
				return
			}
		}
	}
}

func (s *commandServerStream) receiveMessages() {
	go debug.CapturePanicReport(s.sendMessages)
	defer s.cancelCtx()
	for {
		var reqMsg ServerCommandMessage
		err := s.stream.RecvMsg(&reqMsg)
		if err != nil {
			if !errors.Is(err, io.EOF) && status.Code(err) != codes.Canceled {
				s.log(log.ErrorLevel, "receive server command message: %v", err)
			}
			return
		}
		tpe := reqMsg.GetType()
		switch tpe {
		case ServerCommandMessage_Handle:
			s.handleCommand(reqMsg.GetHandle())
		case ServerCommandMessage_Complete:
			err = s.handleComplete(reqMsg.GetComplete())
			if err != nil {
				var resp CompleteCommandDone
				resp.Error = err.Error()
				resp.Id = reqMsg.GetComplete().GetId()
				msg := &ClientCommandMessage{
					Type:         ClientCommandMessage_CompleteDone,
					CompleteDone: &resp,
				}
				select {
				case <-s.ctx.Done():
					return
				case s.sendChan <- msg:
				}
			}
		default:
			s.log(log.ErrorLevel, "extraneous server command message: %d", tpe)
			return
		}
	}
}

func (s *commandServerStream) handleCommand(
	req *HandleCommandRequest,
) {
	err := s.doHandleCommand(req)
	var res HandleCommandResponse
	if err != nil {
		res.Error = err.Error()
	}

	var respMsg ClientCommandMessage
	respMsg.Type = ClientCommandMessage_Handle
	respMsg.Handle = &res
	select {
	case s.sendChan <- &respMsg:
	case <-s.ctx.Done():
	}
}

func (s *commandServerStream) doHandleCommand(req *HandleCommandRequest) error {
	var cmd textapi.Command
	err := s.commandFromProto(&cmd, req)
	if err != nil {
		return fmt.Errorf("command from protobuf: %w", err)
	}

	// NOTE: commands cancels are not currently being propagated
	// from client to server. We must add an extra message that
	// we handle here to do so.
	return s.h.HandleCommand(s.ctx, cmd)
}

func (s *commandServerStream) handleComplete(req *CompleteCommandRequest) error {
	if req.Name == "" {
		return errors.New("invalid complete request: missing command name")
	}
	if req.Id == 0 {
		return errors.New("invalid complete request: missing request id")
	}

	s.log(log.TraceLevel, "streaming completer's iterator for %s %v",
		req.Name, req.Args)
	defer s.log(log.TraceLevel, "done streaming completer's iterator for %s %v",
		req.Name, req.Args)

	ctx := s.ctx
	completer, err := s.h.Complete(ctx, req.Name, req.Args)
	if err != nil {
		return err
	}

	id := req.Id
	go debug.CapturePanicReport(func() {
		var resp CompleteCommandDone
		err := s.streamValues(id, completer)
		if err != nil {
			resp.Error = err.Error()
		}
		resp.Id = id
		msg := &ClientCommandMessage{
			Type:         ClientCommandMessage_CompleteDone,
			CompleteDone: &resp,
		}
		select {
		case <-s.ctx.Done():
		case s.sendChan <- msg:
		}
	})

	return nil
}

func (s *commandServerStream) streamValues(
	id int64,
	completer iterator.Iterator[string],
) error {
	for {
		next, ok := completer.Next(s.ctx)
		if !ok {
			return completer.Err()
		}
		resp := CompleteCommandValue{Id: id, Value: next}
		respMsg := &ClientCommandMessage{
			CompleteValue: &resp,
			Type:          ClientCommandMessage_CompleteValue,
		}
		select {
		case s.sendChan <- respMsg:
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
}

func (s *commandServerStream) commandFromProto(
	e *textapi.Command, pe *HandleCommandRequest,
) (err error) {
	if pe.ResourceName != nil {
		e.URI, err = NewURIFromProto(pe.GetResourceName())
		if err != nil {
			return
		}
		e.Resource = Token{
			URI: e.URI,
		}
	}
	e.Cursor.Window = pe.GetCursorWindow().ToModel()
	e.Cursor.Content = pe.GetCursorContent().ToModel()
	e.Args = pe.GetArgs()
	e.Name = pe.GetName()
	e.Window = browserrpc.NewWindow(uint64(pe.GetWindowId()))
	return err
}

func (s *commandServerStream) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "textrpc.commandServerStream").Logf(level, msg, args...)
}

// enables canceling the iterator from two different places:
// iterator.Close, which produces a ctx.Err() error
// and when the producer is done sending values
// via the corresponding proto message, in which case the channel
// is closed and we gracefully terminate the iterator.
type chanCtx struct {
	ctx context.Context
	ch  chan chanValue
}

type chanValue struct {
	val string
	err error
}
