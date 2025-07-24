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

package handlerrpc

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"unstable.build/go-tui"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/termrpc"
)

// Handler adds Close to a floating handler.
type Handler interface {
	tui.Handler
	io.Closer
}

// ServerStream implements a tui.Handler (+handler.Floating) server over
// a grpc.ClientStream.
type ServerStream[T StreamMessage] struct {
	stream  grpc.ClientStream
	handler Handler
	newT    func() T
	height  atomic.Int32
	width   atomic.Int32
}

// NewServerStream allocates storage for a new Handler server
// and initializes it with the given grpc stream, windowID
// and type parameter constructor.
func NewServerStream[T StreamMessage](
	stream grpc.ClientStream, handler Handler,
	newT func() T,
) *ServerStream[T] {
	return &ServerStream[T]{
		stream:  stream,
		handler: handler,
		newT:    newT,
	}
}

// ReceiveMessages receives messages over the grpc stream
// and responds accordingly.
func (c *ServerStream[T]) ReceiveMessages() {
	defer func() {
		// ensure that client doesn't block in case of panic
		err := c.stream.CloseSend()
		c.log(log.TraceLevel, "closing connection send: %v", err)
	}()
	for {
		var recvMsg ServerMessage
		err := c.stream.RecvMsg(&recvMsg)
		if err != nil {
			if errors.Is(err, io.EOF) || status.Code(err) == codes.Canceled {
				c.log(log.DebugLevel, "stream is closing")
			} else {
				c.log(log.ErrorLevel, "stream receive: %v", err)
			}
			return
		}

		c.log(log.TraceLevel, "received client stream %p message %+v", c, &recvMsg)

		sendMsg := c.newT()
		switch recvMsg.GetType() {
		case MessageType_Draw:
			ctx := context.Background()
			var resp DrawStreamResponse
			width := int(c.width.Load())
			height := int(c.height.Load())
			w := newDrawResponseWriter(ctx, width, height)
			c.handler.Draw(w)
			resp.Rows = w.rows

			sendMsg.SetDraw(&resp)
			err = c.stream.SendMsg(sendMsg)

		case MessageType_Resize:
			resize := recvMsg.GetResize()
			width := int(resize.GetWidth())
			height := int(resize.GetHeight())
			c.width.Store(int32(width))
			c.height.Store(int32(height))
			c.handler.Resize(width, height)
			var resp ResizeStreamResponse
			sendMsg.SetResize(&resp)
			err = c.stream.SendMsg(sendMsg)

		case MessageType_Handle:
			var ev term.Event
			ev, err = recvMsg.GetHandle().GetEvent().ToModel()
			if err == nil {
				exit, handled := c.handler.Handle(ev)
				var resp HandleStreamResponse
				resp.Handled = handled
				resp.Quit = exit
				sendMsg.SetHandle(&resp)
				err = c.stream.SendMsg(sendMsg)
			}

		case MessageType_Cursor:
			coordinates, style, show := c.handler.Cursor()
			var resp CursorStreamResponse
			var respCoordinates termrpc.Coordinates
			respCoordinates.FromModel(coordinates)
			resp.Position = &respCoordinates
			resp.Style = int32(style)
			resp.Show = show
			sendMsg.SetCursor(&resp)
			err = c.stream.SendMsg(sendMsg)

		case MessageType_Selection:
			selection, ok := c.handler.Selection()
			var resp SelectionStreamResponse
			resp.Text = selection
			resp.Ok = ok
			sendMsg.SetSelection(&resp)
			err = c.stream.SendMsg(sendMsg)

		case MessageType_Man:
			man := c.handler.Man()
			var resp ManStreamResponse
			var respMan termrpc.Manual
			err = respMan.FromModel(man)
			if err == nil {
				resp.Man = &respMan
				err = resp.Man.FromModel(man)
				if err == nil {
					sendMsg.SetMan(&resp)
					err = c.stream.SendMsg(sendMsg)
				}
			}

		case MessageType_Dimensions:
			h, ok := c.handler.(handler.Floating)
			if ok {
				width, height := h.Dimensions()
				var resp DimensionsStreamResponse
				resp.Width = int32(width)
				resp.Height = int32(height)
				sendMsg.SetDimensions(&resp)
				err = c.stream.SendMsg(sendMsg)
			} else {
				err = errors.New("called dimensions on non-floating handler")
			}

		case MessageType_Close:
			err := c.handler.Close()
			if err != nil {
				c.log(log.ErrorLevel, "close handler: %v", err)
			}
			var resp CloseStreamResponse
			sendMsg.SetClose(&resp)
			err = c.stream.SendMsg(sendMsg)
			if err != nil {
				c.log(log.ErrorLevel, "send close stream response: %v", err)
			}
			c.log(log.TraceLevel, "received close and sent response message")
			return

		case MessageType_Response, MessageType_Request:
			err = errors.New("stream received invalid message: req/resp")
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.log(log.ErrorLevel, "%s", err.Error())
			}
			return
		}
	}
}

func (s *ServerStream[T]) log(level log.Level, msg string, args ...interface{}) {
	log.WithField(logging.KeyClass, "handlerrpc.ServerStream").Logf(level, msg, args...)
}
