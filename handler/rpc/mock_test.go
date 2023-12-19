package rpc

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
	termpb "unstable.build/go-tui/term/rpc"
)

type mockHandlerClient struct {
	handledCh chan term.Event
	quitCh    chan struct{}
	rpcError  error
	remote    tui.Handler
}

func (c *mockHandlerClient) Handle(
	ctx context.Context, in *HandleRequest, opts ...grpc.CallOption,
) (*HandleResponse, error) {
	if c.rpcError != nil {
		return nil, c.rpcError
	}

	resp := new(HandleResponse)
	ev, err := in.GetEvent().ToModel()
	if err != nil {
		return nil, err
	}

	width, height := int(in.GetDraw().GetWidth()), int(in.GetDraw().GetHeight())
	c.remote.Resize(width, height)
	pos, style, show := c.remote.Cursor()
	resp.Draw = NewDrawResponse(c.remote, width, height)
	resp.Draw.Cursor = &DrawResponse_Cursor{
		Position: &termpb.Coordinates{
			X: int32(pos.X),
			Y: int32(pos.Y),
		},
		Show:  show,
		Style: int32(style),
	}

	// mimic server
	if ev.Type == term.EventInterrupt {
		return resp, err
	}

	resp.Quit, resp.Handled = c.remote.Handle(ev)
	if c.handledCh != nil {
		select {
		case c.handledCh <- ev:
		case <-c.quitCh:
		}
	}

	return resp, nil
}

func (c *mockHandlerClient) Man(
	ctx context.Context, in *ManRequest, opts ...grpc.CallOption,
) (*ManResponse, error) {
	if c.rpcError != nil {
		return nil, c.rpcError
	}

	man := c.remote.Man()
	protoMan := termpb.Manual{}
	protoMan.FromModel(man)

	return &ManResponse{Man: &protoMan}, nil
}

func (c *mockHandlerClient) Close(
	ctx context.Context, in *CloseRequest, opts ...grpc.CallOption,
) (*CloseResponse, error) {
	if c.rpcError != nil {
		return nil, c.rpcError
	}
	if closer, ok := c.remote.(io.Closer); ok {
		err := closer.Close()
		if err != nil {
			return nil, err
		}
	}
	return &CloseResponse{}, nil
}
