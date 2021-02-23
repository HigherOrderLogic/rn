package plugin

import (
	"context"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
)

type interruptWindow struct {
	srv           proto.WindowServer
	interruptDraw func()
}

func interruptWindowServer(s *browser.Server, win browser.Window) proto.WindowServer {
	return &interruptWindow{
		srv:           browser.NewWindowServer(s, win),
		interruptDraw: term.Interrupt,
	}
}

func (w *interruptWindow) SetContent(
	ctx context.Context, req *proto.WindowSetContentRequest,
) (*proto.WindowSetContentResponse, error) {
	defer w.interruptDraw()
	return w.srv.SetContent(ctx, req)
}

func (w *interruptWindow) Content(
	ctx context.Context, req *proto.WindowContentRequest,
) (*proto.WindowContentResponse, error) {
	return w.srv.Content(ctx, req)
}

func (w *interruptWindow) Close(
	ctx context.Context, req *proto.WindowCloseRequest,
) (*proto.WindowCloseResponse, error) {
	defer w.interruptDraw()
	return w.srv.Close(ctx, req)
}
