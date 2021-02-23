package browser

import (
	"context"
	"fmt"

	"github.com/ernestrc/go-tui/proto"
	log "github.com/sirupsen/logrus"
)

// WindowClient satisfies Window by talking to a
// remote window over GRPC.
type windowClient struct {
	brokerID      uint64
	logger        *log.Logger
	pbClient      proto.WindowClient
	browserClient *Client
	onClose       func()
}

func newWindowClient(
	brokerID uint64, browserClient *Client,
	pbClient proto.WindowClient,
) *windowClient {
	ret := new(windowClient)
	ret.browserClient = browserClient
	ret.pbClient = pbClient
	ret.brokerID = brokerID
	return ret
}

func (w *windowClient) Content() (Handler, error) {
	ctx := context.Background()

	req := proto.WindowContentRequest{}
	res, err := w.pbClient.Content(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("pbClient.Content: %v", err)
	}
	return Token{ID: res.GetHandlerId()}, err
}

func (w *windowClient) SetContent(h Handler) error {
	ctx := context.Background()

	brokerID := w.browserClient.serveHandler(h)

	req := proto.WindowSetContentRequest{HandlerId: brokerID}
	_, err := w.pbClient.SetContent(ctx, &req)
	if err != nil {
		reason := fmt.Sprintf("error on call to SetContent: %v", err)
		w.browserClient.safeForceCloseHandler(brokerID, reason)
		return fmt.Errorf("pbClient.SetContent: %v", err)
	}
	return nil
}

func (w *windowClient) onWindowClosed(fn func()) {
	// window client cannot truly hook into when window
	// is closed remotely (we do that at the browser client level
	// via grpc conn monitor goroutine). For that we would
	// need a new RPC but there's no real use-case yet.
	panic("onWindowClosed not implemented on window client")
}

func (w *windowClient) id() uint64 {
	panic("id not implemented on window client")
}

func (w *windowClient) Close() (err error) {
	ctx := context.Background()
	req := proto.WindowCloseRequest{}
	_, _ = w.pbClient.Close(ctx, &req)

	// now we're ready to finally close window client connection and remove
	reason := "windowClient.Close"
	connErr := w.browserClient.safeForceCloseWindow(w.brokerID, reason)
	if connErr != nil {
		err = connErr
	}
	return
}

// satisfies proto.WindowServer
type windowServer struct {
	win Window
	s   *Server
}

// NewWindowServer returns a proto.WindowServer.
func NewWindowServer(s *Server, win Window) proto.WindowServer {
	ret := new(windowServer)
	ret.win = win
	ret.s = s
	return ret
}

func (s *windowServer) Content(
	ctx context.Context, req *proto.WindowContentRequest,
) (*proto.WindowContentResponse, error) {
	s.s.browser.Lock()
	defer s.s.browser.Unlock()

	content, err := s.win.Content()
	if err != nil {
		return nil, fmt.Errorf("windowServer.Content: %v", err)
	}
	handlerID := s.s.ensureAvailable(content)
	return &proto.WindowContentResponse{HandlerId: handlerID}, nil
}

func (s *windowServer) SetContent(
	ctx context.Context, req *proto.WindowSetContentRequest,
) (*proto.WindowSetContentResponse, error) {
	client, err := s.s.getContentHandler(req.GetHandlerId())
	if err != nil {
		return nil, fmt.Errorf("failed to dial to remote handler: %v", err)
	}

	s.s.browser.Lock()
	err = s.win.SetContent(client)
	s.s.browser.Unlock()
	if err != nil {
		reason := fmt.Sprintf("error on windowServer.SetContent: %v", err)
		s.s.safeForceCloseHandler(req.GetHandlerId(), reason)
		return nil, fmt.Errorf("error on Window.SetContent: %v", err)
	}

	return new(proto.WindowSetContentResponse), nil
}

func (s *windowServer) Close(
	ctx context.Context, req *proto.WindowCloseRequest,
) (*proto.WindowCloseResponse, error) {
	err := s.s.safeForceCloseWindow(s.win.id(), "received WindowCloseRequest")
	if err != nil {
		return nil, err
	}
	return new(proto.WindowCloseResponse), nil
}
