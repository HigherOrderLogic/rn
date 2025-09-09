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

package browserrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler/handlerrpc"
	"unstable.build/go-tui/term"
	termrpc "unstable.build/go-tui/term/termrpc"
)

var _ browserapi.Browser = (*Client)(nil)

// Client satisfies Browser by talking to a browser server over RPC.
type Client struct {
	cc  grpc.ClientConnInterface
	wm  WindowManagerClient
	msg NotificationsClient
	f   ResourceOpenerClient
	p   EventPublisherClient

	clientCtx       context.Context
	clientCancelCtx func()
}

// NewClient allocates storage for a new Client and initializes it.
func NewClient(ctx context.Context, cc grpc.ClientConnInterface) *Client {
	ret := new(Client)
	ret.Init(ctx, cc)
	return ret
}

// Init initializes this Client with the given client and parent context.
func (c *Client) Init(ctx context.Context, cc grpc.ClientConnInterface) {
	c.wm = NewWindowManagerClient(cc)
	c.msg = NewNotificationsClient(cc)
	c.cc = cc
	c.f = NewResourceOpenerClient(cc)
	c.p = NewEventPublisherClient(cc)
	c.clientCtx, c.clientCancelCtx = context.WithCancel(ctx)
}

// CloseWindow satisfies browserapi.Browser.
func (c *Client) CloseWindow(win browserapi.Window) error {
	req := WindowCloseRequest{WindowId: win.WindowID()}

	_, err := c.wm.CloseWindow(c.clientCtx, &req)
	return err
}

// SetWindowContent satisfies browserapi.Browser.
func (c *Client) SetWindowContent(win browserapi.Window, h browserapi.Handler) error {
	stream, err := c.wm.SetContent(c.clientCtx)
	if err != nil {
		return fmt.Errorf("new set content stream: %w", err)
	}

	var uri string
	if token, ok := h.(Token); ok {
		uri = token.URI
	}
	req := WindowSetContentRequest{
		Uri:      uri,
		WindowId: win.WindowID(),
	}
	sendMsg := WindowSetContentMessage{
		Type:    handlerrpc.MessageType_Request,
		Request: &req,
	}
	if err := stream.SendMsg(&sendMsg); err != nil {
		return fmt.Errorf("send set content request: %w", err)
	}

	var recvMsg handlerrpc.ServerMessage
	if err := stream.RecvMsg(&recvMsg); err != nil {
		if strings.Contains(err.Error(), browserapi.ErrTabNotFree.Error()) {
			return browserapi.ErrTabNotFree
		}
		return fmt.Errorf("recv set content response: %w", err)
	}

	if recvMsg.GetResponse() == nil {
		return fmt.Errorf("recv nil set content response: %v", &recvMsg)
	}

	if uri != "" {
		return nil
	}

	server := handlerrpc.NewServerStream(
		stream, h, func() *WindowSetContentMessage {
			return new(WindowSetContentMessage)
		})
	go debug.CapturePanicReport(server.ReceiveMessages)

	return nil
}

// Split satisfies Browser.
func (c *Client) Split(
	o browserapi.Orientation, win browserapi.Window, h browserapi.Handler,
) (browserapi.Window, error) {
	stream, err := c.wm.Split(c.clientCtx)
	if err != nil {
		return nil, fmt.Errorf("new split stream: %w", err)
	}

	var uri string
	if token, ok := h.(Token); ok {
		uri = token.URI
	}

	req := SplitRequest{
		Uri:         uri,
		Orientation: toProtoOrientation(o),
		WindowId:    win.WindowID(),
	}
	sendMsg := SplitWindowMessage{
		Type:    handlerrpc.MessageType_Request,
		Request: &req,
	}

	if err := stream.SendMsg(&sendMsg); err != nil {
		return nil, fmt.Errorf("send split request: %w", err)
	}

	var recvMsg handlerrpc.ServerMessage
	if err := stream.RecvMsg(&recvMsg); err != nil {
		return nil, fmt.Errorf("recv split response: %w", err)
	}

	if recvMsg.GetResponse() == nil {
		return nil, fmt.Errorf("recv nil split response: %v", &recvMsg)
	}

	windowID := int(recvMsg.GetResponse().GetWindowId())
	if uri != "" {
		return newWindowClient(uint64(windowID)), nil
	}

	server := handlerrpc.NewServerStream[*SplitWindowMessage](
		stream, h, func() *SplitWindowMessage {
			return new(SplitWindowMessage)
		})
	go debug.CapturePanicReport(server.ReceiveMessages)

	return newWindowClient(uint64(windowID)), nil
}

// Bar satisfies Browser.
func (c *Client) Bar(config browserapi.BarConfig, h tui.Handler) error {
	if _, ok := h.(Token); ok {
		return errors.New("cannot install a resource handler as a bar")
	}

	stream, err := c.wm.Bar(c.clientCtx)
	if err != nil {
		return fmt.Errorf("new bar stream: %w", err)
	}

	req := BarRequest{
		Orientation: toProtoOrientation(config.Orientation),
		Size:        uint32(config.Size),
		Frame:       toProtoFrame(config.Frame),
	}
	sendMsg := BarMessage{
		Type:    handlerrpc.MessageType_Request,
		Request: &req,
	}

	if err := stream.SendMsg(&sendMsg); err != nil {
		return fmt.Errorf("send bar request: %w", err)
	}

	var recvMsg handlerrpc.ServerMessage
	if err := stream.RecvMsg(&recvMsg); err != nil {
		return fmt.Errorf("recv bar response: %w", err)
	}

	if recvMsg.GetResponse() == nil {
		return fmt.Errorf("recv nil bar response: %v", &recvMsg)
	}

	server := handlerrpc.NewServerStream(
		stream, browserapi.NopHandler(h), func() *BarMessage {
			return new(BarMessage)
		})
	go debug.CapturePanicReport(server.ReceiveMessages)

	return nil
}

// Notify satisfies Browser.
func (c *Client) Notify(level notifications.Level, msg string, args ...interface{}) error {
	msg = fmt.Sprintf(msg, args...)

	ctx := context.Background()
	req := NotifyRequest{Level: uint32(level), Msg: msg}

	_, err := c.msg.Notify(ctx, &req)
	return err
}

// NotifyOnce satisfies Browser.
func (c *Client) NotifyOnce(level notifications.Level, msg string, args ...interface{}) error {
	msg = fmt.Sprintf(msg, args...)

	ctx := context.Background()
	req := NotifyRequest{Level: uint32(level), Msg: msg}

	_, err := c.msg.NotifyOnce(ctx, &req)
	return err
}

// Open satisfies Browser.
func (c *Client) Open(resource workspaceapi.URI) (browserapi.Handler, error) {
	ctx := context.Background()
	req := OpenResourceRequest{Resource: resource.String()}

	res, err := c.f.Open(ctx, &req)
	if err != nil {
		return nil, err
	}

	return Token{URI: res.GetUri()}, err
}

// PublishEventNone satisfies Browser.
func (c *Client) PublishEventNone() error {
	ctx := context.Background()
	protoEv := new(termrpc.Event)
	err := protoEv.FromModel(term.Event{Type: term.EventNone})
	if err != nil {
		return err
	}
	req := PublishRequest{Ev: protoEv}

	_, err = c.p.Publish(ctx, &req)
	return err
}

// Interrupt satisfies Browser.
func (c *Client) Interrupt(ctx context.Context) error {
	payload, _ := term.PayloadFromContext(ctx)
	protoEv := new(termrpc.Event)
	err := protoEv.FromModel(term.Event{Type: term.EventInterrupt, Raw: payload})
	if err != nil {
		return err
	}
	req := PublishRequest{Ev: protoEv}

	_, err = c.p.Publish(ctx, &req)
	return err
}

// Focus satisfies Browser.
func (c *Client) Focus() (browserapi.Window, error) {
	var req FocusRequest
	res, err := c.wm.Focus(c.clientCtx, &req)
	if err != nil {
		return nil, err
	}
	win := newWindowClient(res.GetWindowId())
	return win, err
}

// Floating satisfies browser.WindowManager
func (c *Client) Floating(
	h browserapi.Floating, cfg component.FloatingConfig,
) (browserapi.Window, error) {
	if _, ok := h.(Token); ok {
		return nil, errors.New("cannot install a resource handler in a floating window")
	}
	stream, err := c.wm.Floating(c.clientCtx)
	if err != nil {
		return nil, fmt.Errorf("new floating stream: %w", err)
	}

	var atProto termrpc.Coordinates
	atProto.FromModel(cfg.Offset)

	req := FloatingWindowRequest{
		Offset:    &atProto,
		Alignment: uint32(cfg.Alignment),
	}
	sendMsg := FloatingWindowMessage{
		Type:    handlerrpc.MessageType_Request,
		Request: &req,
	}

	if err := stream.SendMsg(&sendMsg); err != nil {
		return nil, fmt.Errorf("send floating request: %w", err)
	}

	var recvMsg handlerrpc.ServerMessage
	if err := stream.RecvMsg(&recvMsg); err != nil {
		return nil, fmt.Errorf("recv floating response: %w", err)
	}

	if recvMsg.GetResponse() == nil {
		return nil, fmt.Errorf("recv nil floating response: %v", &recvMsg)
	}

	windowID := int(recvMsg.GetResponse().GetWindowId())
	server := handlerrpc.NewServerStream[*FloatingWindowMessage](
		stream, h, func() *FloatingWindowMessage {
			return new(FloatingWindowMessage)
		})
	go debug.CapturePanicReport(server.ReceiveMessages)

	return newWindowClient(uint64(windowID)), nil
}

// Tab satisfies browser.WindowManager
func (c *Client) Tab(
	uri workspaceapi.URI, icon rune, name string, h browserapi.Handler,
) (browserapi.Handler, error) {
	if _, ok := h.(Token); ok {
		return nil, errors.New("cannot create a tab from a remote resource")
	}
	stream, err := c.wm.Tab(c.clientCtx)
	if err != nil {
		return nil, fmt.Errorf("new floating stream: %w", err)
	}
	uriStr := uri.String()
	req := TabRequest{
		ResourceId:   uriStr,
		ResourceName: name,
		ResourceIcon: string(icon),
	}
	sendMsg := TabMessage{
		Type:    handlerrpc.MessageType_Request,
		Request: &req,
	}

	if err := stream.SendMsg(&sendMsg); err != nil {
		return nil, fmt.Errorf("send floating request: %w", err)
	}

	var recvMsg handlerrpc.ServerMessage
	if err := stream.RecvMsg(&recvMsg); err != nil {
		return nil, fmt.Errorf("recv floating response: %w", err)
	}

	if recvMsg.GetResponse() == nil {
		return nil, fmt.Errorf("recv nil floating response: %v", &recvMsg)
	}

	server := handlerrpc.NewServerStream(stream, h, func() *TabMessage {
		return new(TabMessage)
	})
	go debug.CapturePanicReport(server.ReceiveMessages)

	return Token{URI: uriStr}, err
}

// Close closes all resources associated with this Client.
// This client should not be used after this method is called.
func (c *Client) Close() (err error) {
	if closer, ok := c.cc.(io.Closer); ok {
		err = closer.Close()
	}
	c.cc = nil
	if c.clientCancelCtx != nil {
		c.clientCancelCtx()
		c.clientCancelCtx = nil
	}
	return
}

func toProtoOrientation(o browserapi.Orientation) Orientation {
	switch o {
	case browserapi.OrientationDefault:
		return Orientation_Default
	case browserapi.OrientationTop:
		return Orientation_Top
	case browserapi.OrientationBottom:
		return Orientation_Bottom
	case browserapi.OrientationLeft:
		return Orientation_Left
	case browserapi.OrientationRight:
		return Orientation_Right
	default:
		panic("invalid orientation")
	}
}

func toProtoFrame(o browserapi.BarFrame) BarRequest_Frame {
	switch o {
	case browserapi.BarFrameDefault:
		return BarRequest_Default
	case browserapi.BarFrameAlways:
		return BarRequest_Always
	case browserapi.BarFrameNever:
		return BarRequest_Never
	default:
		panic("invalid orientation")
	}
}
