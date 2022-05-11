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
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

// Server serves a Browser over GRPC.
type Server struct {
	proto.UnimplementedEventPublisherServer
	proto.UnimplementedMessengerServer
	proto.UnimplementedResourceOpenerServer
	proto.UnimplementedWindowManagerServer
	document.Server

	Logger *log.Logger

	broker proto.MuxBroker

	failureTimeout time.Duration

	// handler client resources are created on calls to Split* and SetContent.
	// They are destroyed when Close is dispatched to handler server
	// and so if server is closed, client connection
	// Connections are also monitored and cleaned if irrecoverable errors are found.
	clients map[uint64]io.Closer

	// window servers are created on calls to Split* and Focus. They are destroyed
	// when window is closed, either remotely, or locally (via onWindowClosed hook).
	servers         map[uint64]io.Closer
	brokerIDToWinID map[uint64]uint64

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
// and provide a hook on calls to Close. We could rely solely on the remote browser
// server to close and our monitor goroutine to clean up, but we do not trust the remote
// browser necessarily.
type browserServerHandler struct {
	Handler
	s         *Server
	handlerID uint64
}

func (s browserServerHandler) gracefulShutdown(reason string) {
	time.Sleep(gracefulShutdownWait)
	s.s.safeForceCloseHandler(s.handlerID, reason)
}

func (s browserServerHandler) Close() error {
	go s.gracefulShutdown("Close")
	return s.Handler.Close()
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
	s.brokerIDToWinID = make(map[uint64]uint64)
	s.opened = make(map[uint32]Handler)
	s.failureTimeout = defaultFailureTimeout
	s.windowServer = windowServer
	s.Server.Init(browser, nil)
}

func (s *Server) consumeErrors(
	ctx context.Context, handlerID uint64, ch <-chan error,
) {
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

func (s *Server) dialHandler(handlerID uint64) (Handler, error) {
	s.tryLog("(%p browser.Server): dialing handlerID: %d", s, handlerID)
	handlerConn, err := s.broker.Dial(uint32(handlerID))
	if err != nil {
		return nil, err
	}

	pbClient := proto.NewHandlerClient(handlerConn)
	pbClient = newIOWaitUnlockHandlerClient(pbClient, s.browser.Locker)
	cc := handler.NewClient(pbClient)
	cc.Logger = s.Logger

	ctx, cancelFn := context.WithCancel(context.Background())

	go proto.MonitorConnection(ctx, s.failureTimeout, handlerConn,
		func(reason string) {
			s.safeForceCloseHandler(uint64(handlerID),
				fmt.Sprintf("browser.MonitorConnection(handler): %s", reason))
		})

	go s.consumeErrors(ctx, handlerID, cc.Errors())

	s.browser.Lock()
	defer s.browser.Unlock()

	s.clients[uint64(handlerID)] = &handlerClientResource{
		handlerConn:   handlerConn,
		client:        cc,
		cancelMonitor: cancelFn,
	}

	return cc, nil
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
	s.brokerIDToWinID[uint64(brokerID)] = win.id()

	win.onWindowClosed(func() {
		s.forceCloseWindow(win.id(),
			"underlying window called onWindowClosed callback")
		delete(s.brokerIDToWinID, uint64(brokerID))
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

func (s *Server) getContentHandler(handlerID uint64) (Handler, bool, error) {
	if handlerID < math.MaxUint32 {
		s.browser.Lock()
		h, ok := s.opened[uint32(handlerID)]
		s.browser.Unlock()
		if ok {
			s.tryLog("(%p browser.Server): using return of Open/Content handler for handlerID: %d",
				s, handlerID)
			return h, false, nil
		}
	}

	var ret Handler
	s.browser.Lock()
	res, ok := s.clients[handlerID]
	s.browser.Unlock()
	if ok {
		s.tryLog("(%p browser.Server): found cached client for handlerID: %d", s, handlerID)
		ret = res.(*handlerClientResource).client
	} else {
		var err error
		ret, err = s.dialHandler(handlerID)
		if err != nil {
			return nil, false, err
		}
	}

	bHandler := browserServerHandler{
		handlerID: handlerID,
		Handler:   ret,
		s:         s,
	}
	return bHandler, !ok, nil
}

func (s *Server) newRemoteResource(
	ctx context.Context, handlerID uint64,
	action func(WindowManager, Handler) (Window, error),
) (uint64, error) {
	handler, created, err := s.getContentHandler(handlerID)
	if err != nil {
		return 0, err
	}

	s.browser.Lock()
	defer s.browser.Unlock()
	win, err := action(s.browser, handler)
	if err != nil {
		if created {
			reason := fmt.Sprintf("failed to create resource: %s", err.Error())
			s.forceCloseHandler(handlerID, reason)
		}
		return 0, err
	}
	if win == nil {
		return 0, nil
	}

	return s.serveWindow(win), nil
}

func protoToModelOrientation(p proto.Orientation) (o Orientation) {
	switch p {
	case proto.Orientation_Default:
		o = OrientationDefault
	case proto.Orientation_Top:
		o = OrientationTop
	case proto.Orientation_Bottom:
		o = OrientationBottom
	case proto.Orientation_Left:
		o = OrientationLeft
	case proto.Orientation_Right:
		o = OrientationRight
	}
	return
}

// Split satisfies proto.BrowserServer
func (s *Server) Split(
	ctx context.Context, req *proto.SplitRequest,
) (*proto.SplitResponse, error) {
	windowID, err := s.newRemoteResource(ctx, req.GetHandlerId(),
		func(wm WindowManager, h Handler) (Window, error) {
			return wm.Split(protoToModelOrientation(req.GetOrientation()), h)
		})
	if err != nil {
		return nil, err
	}
	return &proto.SplitResponse{WindowId: windowID}, nil
}

// Bar satisfies proto.BrowserServer
func (s *Server) Bar(
	ctx context.Context, req *proto.BarRequest,
) (*proto.BarResponse, error) {
	handlerID := req.GetHandlerId()
	handler, created, err := s.getContentHandler(handlerID)
	if err != nil {
		return nil, err
	}

	s.browser.Lock()
	defer s.browser.Unlock()
	err = s.browser.Bar(protoToModelOrientation(req.GetOrientation()), handler)
	if err != nil {
		if created {
			reason := fmt.Sprintf("failed to create window: %s", err.Error())
			s.forceCloseHandler(handlerID, reason)
		}
		return nil, err
	}
	return new(proto.BarResponse), nil
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
	uri, err := workspace.ParseURI(req.GetResource())
	if err != nil {
		return nil, err
	}

	s.browser.Lock()
	defer s.browser.Unlock()

	h, err := s.browser.Open(uri)
	if err != nil {
		return nil, err
	}

	handlerID := s.ensureAvailable(h)
	return &proto.OpenResourceResponse{HandlerId: handlerID}, nil
}

// Publish satisfies proto.BrowserServer
func (s *Server) Publish(
	ctx context.Context, req *proto.PublishRequest,
) (*proto.PublishResponse, error) {
	ev, err := req.GetEv().ToModel()
	if err != nil {
		return nil, err
	}

	var fn func() error
	switch ev.Type {
	case term.EventInterrupt:
		fn = s.browser.PublishInterrupt
	case term.EventNone:
		fn = s.browser.PublishEventNone
	default:
		return nil, fmt.Errorf("invalid event type: %v", ev.Type)
	}

	s.browser.Lock()
	defer s.browser.Unlock()
	err = fn()
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

// SetFocus satisfies proto.BrowserServer
func (s *Server) SetFocus(
	ctx context.Context, req *proto.SetFocusRequest,
) (*proto.FocusResponse, error) {
	s.browser.Lock()
	defer s.browser.Unlock()

	winID, ok0 := s.brokerIDToWinID[req.GetWindowId()]
	winIfc, ok1 := s.servers[winID]
	win, ok2 := winIfc.(*windowServerResource)
	if !ok0 || !ok1 || !ok2 {
		return nil, fmt.Errorf("cannot find window with windowID: %d: FIXME remove: %#v",
			req.GetWindowId(), s.servers)
	}

	prev, err := s.browser.SetFocus(win.win)
	if err != nil {
		return nil, err
	}

	windowID := s.serveWindow(prev)
	res := &proto.FocusResponse{
		WindowId: windowID,
	}

	return res, nil
}

// Floating satisfies proto.BrowserServer
func (s *Server) Floating(
	ctx context.Context, req *proto.FloatingWindowRequest,
) (*proto.FloatingWindowResponse, error) {
	at := req.GetAt().ToModel()
	height := int(req.GetHeight())
	width := int(req.GetWidth())
	windowID, err := s.newRemoteResource(ctx, req.GetHandlerId(),
		func(wm WindowManager, h Handler) (Window, error) {
			return wm.Floating(h, at, height, width)
		})
	if err != nil {
		return nil, err
	}
	return &proto.FloatingWindowResponse{WindowId: windowID}, nil
}

// Tab satisfies proto.BrowserServer
func (s *Server) Tab(
	ctx context.Context, req *proto.TabRequest,
) (*proto.TabResponse, error) {
	name := req.GetResourceName()
	id := req.GetResourceId()
	uri, err := workspace.ParseURI(id)
	if err != nil {
		return nil, err
	}
	var tab Handler
	_, err = s.newRemoteResource(ctx, req.GetHandlerId(),
		func(wm WindowManager, h Handler) (w Window, err error) {
			tab, err = wm.Tab(uri, name, h)
			return nil, err
		})
	if err != nil {
		return nil, err
	}
	handlerID := s.ensureAvailable(tab)
	return &proto.TabResponse{TabHandlerId: handlerID}, nil
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
