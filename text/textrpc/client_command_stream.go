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
	"log/slog"
	"sync"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
)

type commandClientStream struct {
	log           *slog.Logger
	ctx           context.Context
	handleCommand chan string
	stream        serverStream
	completers    sync.Map
	counter       int64
}

// subset of Editor_SubscribeCommandServer
type serverStream interface {
	RecvMsg(any) error
	Send(*textrpc.ServerCommandMessage) error
}

func newCommandClientStream(
	ctx context.Context, stream serverStream,
) *commandClientStream {
	log := slog.Default().With("struct", "textrpc.commandClientStream")
	return &commandClientStream{
		log:           log,
		stream:        stream,
		ctx:           ctx,
		handleCommand: make(chan string),
	}
}

func (c *commandClientStream) receiveMessages() error {
	for {
		var msg textrpc.ClientCommandMessage
		err := c.stream.RecvMsg(&msg)
		if err != nil {
			return fmt.Errorf("receive stream message: %w", err)
		}

		switch msg.GetType() {
		case textrpc.ClientCommandMessage_Handle:
			select {
			case c.handleCommand <- msg.GetHandle().GetError():
			case <-c.ctx.Done():
				return c.ctx.Err()
			}
		case textrpc.ClientCommandMessage_CompleteValue:
			complete := msg.GetCompleteValue()
			id := complete.GetId()
			if id == 0 {
				c.log.Warn("received complete value message with invalid id")
				continue
			}
			val, ok := c.completers.Load(id)
			if !ok {
				c.log.Warn("received complete message for unknown stream")
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
		case textrpc.ClientCommandMessage_CompleteDone:
			done := msg.GetCompleteDone()
			id := done.GetId()
			if id == 0 {
				c.log.Warn("received complete done message with invalid id")
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
			c.log.Warn("received extraneous message type", "type", msg.GetType())
		}
	}
}

func (c *commandClientStream) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) error {
	var cursorContent, cursorWindow termrpc.Coordinates
	cursorContent.FromModel(cmd.Cursor.Content)
	cursorWindow.FromModel(cmd.Cursor.Window)

	req := textrpc.HandleCommandRequest{
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

	var reqMsg textrpc.ServerCommandMessage
	reqMsg.Type = textrpc.ServerCommandMessage_Handle
	reqMsg.Handle = &req

	if err := c.stream.Send(&reqMsg); err != nil {
		return fmt.Errorf("send complete request: %w", err)
	}

	return nil
}

func (c *commandClientStream) Complete(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	c.counter++ // start with 1, so 0 is a missing ID error
	id := c.counter

	var req textrpc.CompleteCommandRequest
	req.Id = id
	req.Name = cmd.Name
	req.Args = cmd.Args
	// the rest of fields are not propagated, since textapi.CommandHandler
	// doesn't have he same signature as text.CommandHandler.

	var reqMsg textrpc.ServerCommandMessage
	reqMsg.Type = textrpc.ServerCommandMessage_Complete
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
