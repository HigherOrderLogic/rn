package browser

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/util"
	log "github.com/sirupsen/logrus"
)

// Server serves a Browser over GRPC.
type Server struct {
	document.Server

	Logger *log.Logger

	broker proto.MuxBroker

	failureTimeout time.Duration

	// handler client resources are created on calls to Subscribe,
	// Split* and SetContent. They are destroyed when OnUnmount is dispatched to handler server
	// and so if server is closed, client connection
	// Connections are also monitored and cleaned if irrecoverable errors are found.
	clients map[uint64]io.Closer

	// window servers are created on calls to Split* and Focus. They are destroyed
	// when window is closed, either remotely, or locally (via onWindowClosed hook).
	servers map[uint64]io.Closer

	// Handlers opened by Open
	// NOTE this map is never cleaned up: this implementation is decoupled from the
	// editor Close hooks so we don't know when a browser client is no longer
	// referencing a valid Handler.
	opened map[uint32]Handler

	browser struct {
		Browser
		sync.Locker
	}

	windowServer func(*Server, Window) proto.WindowServer
}

// browserServerHandler wraps a handler.Client to satisfy browser.Handler
// and provide a hook on calls to OnUnmount. We could rely solely on the remote browser
// server to close and our monitor goroutine to clean up, but we do not trust the remote
// browser necessarily.
type browserServerHandler struct {
	Handler
	s         *Server
	handlerID uint64
}

func (s browserServerHandler) gracefulShutdown() {
	time.Sleep(gracefulShutdownWait)
	s.s.safeForceCloseHandler(s.handlerID, "Handle()(exit=true)")
}

func (s browserServerHandler) Handle(ev term.Event) (bool, bool) {
	exit, handled := s.Handler.Handle(ev)
	if exit {
		go s.gracefulShutdown()
	}
	return exit, handled
}

// NewServer allocates storage for a new Server and initializes it.
func NewServer(
	broker proto.MuxBroker, browser Browser, lock sync.Locker,
) *Server {
	ret := new(Server)
	ret.Init(broker, browser, lock, NewWindowServer)
	return ret
}

// Init initializes this Server with broker and browser.
func (s *Server) Init(
	broker proto.MuxBroker, browser Browser, lock sync.Locker,
	windowServer func(*Server, Window) proto.WindowServer,
) {
	s.broker = broker
	s.browser.Browser = browser
	s.browser.Locker = lock
	s.clients = make(map[uint64]io.Closer)
	s.servers = make(map[uint64]io.Closer)
	s.opened = make(map[uint32]Handler)
	s.failureTimeout = defaultFailureTimeout
	s.windowServer = windowServer
	s.Server.Init(browser, nil)
}

func (s *Server) consumeErrors(ctx context.Context, handlerID uint64, ch <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-ch:
			err = fmt.Errorf("handler.Client %d error: %v", handlerID, err)
			s.tryLog("%v", err)
			msgErr := s.setBrowserMessage(err.Error())
			if msgErr != nil {
				s.tryLog("error calling browser.SetMessage upon handler.Client"+
					" error: %v: %v", msgErr, err)
			}
		}
	}
}

func (s *Server) dialHandler(handlerID uint64) (handlerCloser, error) {
	s.tryLog("(%p browser.Server): dialing handlerID: %d", s, handlerID)
	handlerConn, err := s.broker.Dial(uint32(handlerID))
	if err != nil {
		return nil, err
	}

	pbClient := proto.NewHandlerClient(handlerConn)
	cc := handler.NewClient(pbClient)
	cc.Logger = s.Logger
	client := newIOWaitUnlockHandler(cc, s.browser.Locker)

	ctx, cancelFn := context.WithCancel(context.Background())

	go proto.MonitorConnection(ctx, s.failureTimeout, handlerConn, func(reason string) {
		s.safeForceCloseHandler(handlerID, reason)
	})

	go s.consumeErrors(ctx, handlerID, cc.Errors())

	s.browser.Lock()
	defer s.browser.Unlock()

	s.clients[uint64(handlerID)] = &handlerClientResource{
		handlerConn:   handlerConn,
		client:        client,
		cancelMonitor: cancelFn,
	}

	return client, nil
}

func (s *Server) serveWindow(win Window) uint64 {
	if res, ok := s.servers[win.id()]; ok {
		return res.(*windowServerResource).brokerID
	}

	brokerID, srv := proto.AcceptAndServe(s.broker, s.Logger,
		func(windowBrokerID uint32, srv proto.MuxServer) {
			winSrv := s.windowServer(s, win)
			proto.RegisterWindowServer(srv.GRPC(), winSrv)
		})

	s.servers[win.id()] = &windowServerResource{
		srv:      srv,
		win:      win,
		brokerID: uint64(brokerID),
	}

	win.onWindowClosed(func() {
		s.forceCloseWindow(win.id(), "underlying window called onWindowClosed callback")
	})
	return uint64(brokerID)
}

func (s *Server) getClients() map[uint64]io.Closer {
	return s.clients
}

func (s *Server) getServers() map[uint64]io.Closer {
	return s.servers
}

func (s *Server) forceCloseWindow(winID uint64, reason string) error {
	s.tryLog("browser.Server.forceCloseWindow(%d, reason=%s)", winID, reason)
	_, err := proto.ForceCloseResource(winID, s.getServers, s.Logger, nopLocker{})
	return err
}

func (s *Server) safeForceCloseWindow(winID uint64, reason string) error {
	s.tryLog("browser.Server.safeForceCloseWindow(%d, reason=%s)", winID, reason)
	_, err := proto.ForceCloseResource(winID, s.getServers, s.Logger, &s.browser)
	return err
}

func (s *Server) tryLog(msg string, args ...interface{}) {
	if s.Logger == nil {
		return
	}
	s.Logger.Debugf(msg, args...)
}

func (s *Server) safeForceCloseHandler(brokerID uint64, reason string) error {
	s.tryLog("browser.Server.safeForceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(brokerID, s.getClients, s.Logger, &s.browser)
	return err
}
func (s *Server) forceCloseHandler(brokerID uint64, reason string) error {
	s.tryLog("browser.Server.forceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(brokerID, s.getClients, s.Logger, nopLocker{})
	return err
}

func (s *Server) getContentHandler(handlerID uint64) (Handler, error) {
	s.browser.Lock()
	if handlerID < math.MaxUint32 {
		h, ok := s.opened[uint32(handlerID)]
		s.browser.Unlock()
		if ok {
			s.tryLog("(%p browser.Server): using return of Open/Content handler for handlerID: %d",
				s, handlerID)
			return h, nil
		}
	}

	var cc handlerCloser
	s.browser.Lock()
	res, ok := s.clients[handlerID]
	s.browser.Unlock()
	if ok {
		s.tryLog("(%p browser.Server): found cached client for handlerID: %d", s, handlerID)
		cc = res.(*handlerClientResource).client
	} else {
		var err error
		cc, err = s.dialHandler(handlerID)
		if err != nil {
			return nil, err
		}
	}

	bHandler := browserServerHandler{
		handlerID: handlerID,
		Handler:   cc,
		s:         s,
	}
	return bHandler, nil
}

// SplitVerticalRight satisfies proto.BrowserServer
func (s *Server) split(
	ctx context.Context, req interface{ GetHandlerId() uint64 },
	split func(WindowManager, Handler) (Window, error),
) (*proto.SplitResponse, error) {
	handlerID := req.GetHandlerId()
	handler, err := s.getContentHandler(handlerID)
	if err != nil {
		return nil, err
	}

	s.browser.Lock()
	defer s.browser.Unlock()
	win, err := split(s.browser, handler)
	if err != nil {
		reason := fmt.Sprintf("failed to create window: %s", err.Error())
		s.forceCloseHandler(handlerID, reason)
		return nil, err
	}

	windowID := s.serveWindow(win)
	res := &proto.SplitResponse{WindowId: windowID}
	return res, nil
}

// SplitVerticalRight satisfies proto.BrowserServer
func (s *Server) SplitVerticalRight(
	ctx context.Context, req *proto.SplitRequest,
) (*proto.SplitResponse, error) {
	return s.split(ctx, req, (WindowManager).SplitVerticalRight)
}

// SplitVerticalLeft satisfies proto.BrowserServer
func (s *Server) SplitVerticalLeft(
	ctx context.Context, req *proto.SplitRequest,
) (*proto.SplitResponse, error) {
	return s.split(ctx, req, (WindowManager).SplitVerticalLeft)
}

// SplitHorizontalAbove satisfies proto.BrowserServer
func (s *Server) SplitHorizontalAbove(
	ctx context.Context, req *proto.SplitRequest,
) (*proto.SplitResponse, error) {
	return s.split(ctx, req, (WindowManager).SplitHorizontalAbove)
}

// SplitHorizontalBelow satisfies proto.BrowserServer
func (s *Server) SplitHorizontalBelow(
	ctx context.Context, req *proto.SplitRequest,
) (*proto.SplitResponse, error) {
	return s.split(ctx, req, (WindowManager).SplitHorizontalBelow)
}

// MergeKeyMap satisfies proto.BrowserServer
func (s *Server) MergeKeyMap(
	ctx context.Context, req *proto.MergeKeyMapRequest,
) (*proto.MergeKeyMapResponse, error) {
	m := make(map[term.Event]term.Event)

	for _, mapping := range req.GetMappings() {
		from, err := mapping.From.ToModel()
		if err != nil {
			return nil, err
		}
		to, err := mapping.To.ToModel()
		if err != nil {
			return nil, err
		}
		m[from] = to
	}

	s.browser.Lock()
	defer s.browser.Unlock()

	err := s.browser.MergeKeyMap(m)
	if err != nil {
		return nil, err
	}
	return new(proto.MergeKeyMapResponse), nil
}

func (s *Server) setBrowserMessage(msg string) error {
	s.browser.Lock()
	defer s.browser.Unlock()

	return s.browser.SetMessage(msg)
}

// SetMessage satisfies proto.BrowserServer
func (s *Server) SetMessage(
	ctx context.Context, req *proto.SetMessageRequest,
) (*proto.SetMessageResponse, error) {
	msg := util.SanitizeLine(req.GetMsg())
	err := s.setBrowserMessage(msg)
	if err != nil {
		return nil, err
	}
	return new(proto.SetMessageResponse), nil
}

// ensureAvailable stores h for future calls to SetContent or Split methods
// so that these calls do not dial to remote handler, but rather use local
// handler referenced by the return id of this method.
func (s *Server) ensureAvailable(h Handler) uint64 {
	// if the content happens to be an active client
	// then there's no need to store in opened, as
	// future calls to SetContent/Split methods will used the cached
	// client in clients
	if bsh, ok := h.(browserServerHandler); ok {
		if _, ok = s.clients[bsh.handlerID]; ok {
			return bsh.handlerID
		}
	}

	handlerID := s.broker.NextId()
	s.opened[handlerID] = h
	s.tryLog("(%p browser.Server): stored handler with ID %d", s, handlerID)
	return uint64(handlerID)
}

// Open satisfies proto.BrowserServer
func (s *Server) Open(
	ctx context.Context, req *proto.OpenResourceRequest,
) (*proto.OpenResourceResponse, error) {
	resource := util.SanitizeResourceName(req.GetResource())

	s.browser.Lock()
	defer s.browser.Unlock()

	h, err := s.browser.Open(resource)
	if err != nil {
		return nil, err
	}

	handlerID := s.ensureAvailable(h)
	return &proto.OpenResourceResponse{HandlerId: handlerID}, nil
}

// Subscribe satisfies proto.BrowserServer
func (s *Server) Subscribe(
	ctx context.Context, req *proto.SubscribeRequest,
) (*proto.SubscribeResponse, error) {
	handlerID := req.GetHandlerId()
	handler, err := s.dialHandler(handlerID)
	if err != nil {
		return nil, err
	}
	h := serverEventHandler{handlerID: handlerID, s: s, h: handler}

	ev, err := req.GetEv().ToModel()
	if err != nil {
		return nil, err
	}

	s.browser.Lock()
	defer s.browser.Unlock()
	err = s.browser.Subscribe(ev, h)
	if err != nil {
		reason := fmt.Sprintf("failed to subscribe: %v", err)
		s.forceCloseHandler(handlerID, reason)
		return nil, err
	}

	return new(proto.SubscribeResponse), nil
}

// Publish satisfies proto.BrowserServer
func (s *Server) Publish(
	ctx context.Context, req *proto.PublishRequest,
) (*proto.PublishResponse, error) {
	ev, err := req.GetEv().ToModel()
	if err != nil {
		return nil, err
	}

	// NOTE: for now it's the only event allowed
	if ev.Type != term.EventInterrupt {
		return nil, fmt.Errorf("invalid event type: %v", ev.Type)
	}

	s.browser.Lock()
	defer s.browser.Unlock()
	err = s.browser.PublishInterrupt()
	if err != nil {
		return nil, err
	}

	return new(proto.PublishResponse), nil
}

// Focus satisfies proto.BrowserServer
func (s *Server) Focus(
	ctx context.Context, req *proto.FocusRequest,
) (*proto.FocusResponse, error) {
	s.browser.Lock()
	defer s.browser.Unlock()
	win, err := s.browser.Focus()

	if err != nil {
		return nil, err
	}

	windowID := s.serveWindow(win)
	res := &proto.FocusResponse{
		WindowId: windowID,
	}

	return res, nil
}

// FloatingWindow satisfies proto.BrowserServer
func (s *Server) FloatingWindow(
	ctx context.Context, req *proto.FloatingWindowRequest,
) (*proto.FloatingWindowResponse, error) {
	at := req.GetAt().ToModel()
	height := int(req.GetHeight())
	width := int(req.GetWidth())
	resp, err := s.split(ctx, req, func(wm WindowManager, h Handler) (Window, error) {
		return wm.FloatingWindow(h, at, height, width)
	})
	if err != nil {
		return nil, err
	}
	return &proto.FloatingWindowResponse{WindowId: resp.GetWindowId()}, nil
}

// Close closes all resources associated with this server.
func (s *Server) Close() (err error) {
	s.browser.Lock()
	defer s.browser.Unlock()

	for _, res := range s.clients {
		resErr := res.Close()
		if resErr != nil {
			err = resErr
		}
	}
	for _, res := range s.servers {
		resErr := res.Close()
		if resErr != nil {
			err = resErr
		}
	}

	s.browser.Close()
	s.clients = nil
	s.servers = nil
	s.opened = nil
	return err
}
