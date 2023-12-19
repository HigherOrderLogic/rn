package rpc

import (
	"context"
	"runtime"

	"unstable.build/go-tui"
	"unstable.build/go-tui/browser"
	handlerpb "unstable.build/go-tui/handler/rpc"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/term"
)

var _ browser.Floating = (*floatingClientImpl)(nil)

type floatingClientImpl struct {
	conn          proto.MuxConn
	cancelFn      func()
	client        *handlerpb.Client
	fc            FloatingClient
	errorCh       chan error
	width, height int
}

func newFloatingClient(
	handler *handlerpb.Client, fc FloatingClient,
	cancelFn func(), conn proto.MuxConn,
) *floatingClientImpl {
	ret := &floatingClientImpl{
		client:   handler,
		fc:       fc,
		errorCh:  make(chan error),
		cancelFn: cancelFn,
		conn:     conn,
	}
	runtime.SetFinalizer(ret, func(*floatingClientImpl) {
		cancelFn()
		conn.Close()
		runtime.SetFinalizer(ret, nil)
	})
	return ret
}

func (f *floatingClientImpl) Resize(width, height int) {
	f.width = width
	f.height = width
	f.client.Resize(width, height)
	runtime.KeepAlive(f)
}
func (f *floatingClientImpl) Draw(w term.Writer) {
	f.client.Draw(w)
	runtime.KeepAlive(f)
}

func (f *floatingClientImpl) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = f.client.Handle(ev)
	runtime.KeepAlive(f)
	return exit, handled
}

func (f *floatingClientImpl) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	pos, style, show = f.client.Cursor()
	runtime.KeepAlive(f)
	return pos, style, show
}

func (f *floatingClientImpl) Man() tui.Manual {
	man := f.client.Man()
	runtime.KeepAlive(f)
	return man
}

func (f *floatingClientImpl) Dimensions() (width, height int) {
	req := DimensionsRequest{}
	ctx := context.Background()
	resp, err := f.fc.Dimensions(ctx, &req)
	runtime.KeepAlive(f)
	if err != nil {
		select {
		case f.errorCh <- err:
		default:
		}
		// best effort
		return f.width, f.height
	}
	return int(resp.GetWidth()), int(resp.GetHeight())
}

func (f *floatingClientImpl) Close() error {
	err := f.client.Close()
	f.cancelFn()
	// it might already be closed by conn monitoring
	_ = f.conn.Close()
	runtime.SetFinalizer(f, nil)
	return err
}
