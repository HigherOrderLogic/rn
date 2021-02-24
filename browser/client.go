package browser

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const gracefulShutdownWait = 100 * time.Millisecond

// Client satisfies Browser by talking to a browser server over RPC.
type Client struct {
	Logger *log.Logger

	// resources invariant
	mu sync.Mutex

	// time this client waits for a grpc connection transient failure
	// to recover before we shutdown connection.
	failureTimeout time.Duration

	broker  proto.MuxBroker
	cc      grpc.ClientConnInterface
	storage document.Client
	wm      proto.WindowManagerClient
	msg     proto.MessengerClient
	mp      proto.KeyMapperClient
	f       proto.ResourceOpenerClient
	s       proto.EventSubscriberClient
	p       proto.EventPublisherClient

	// window client resources. windowClients are created on
	// calls to Split* or Focus. They are destroyed when client
	// calls Close method, or server Closes window.
	// The latter is monitored via a separate goroutine.
	clients map[uint64]io.Closer

	// handler server resources. handler servers are created on
	// calls to Split or SetContent (if handler is not return of Open).
	// this map is cleaned up only when handler returns true to a call to Handle.
	servers map[uint64]io.Closer
}

type browserClientHandler struct {
	handlerID uint64
	Handler
	c *Client
}

func (c browserClientHandler) gracefulShutdown() {
	time.Sleep(gracefulShutdownWait)
	c.c.safeForceCloseHandler(c.handlerID, "Handle()(exit=true)")
}

func (c browserClientHandler) Handle(ev term.Event) (bool, bool) {
	exit, handled := c.Handler.Handle(ev)
	if exit {
		go c.gracefulShutdown()
	}
	return exit, handled
}

// NewClient allocates storage for a new Client and initializes it.
func NewClient(
	broker proto.MuxBroker, cc grpc.ClientConnInterface,
) *Client {
	ret := new(Client)
	ret.Init(broker, cc)
	return ret
}

func (c *Client) tryLog(msg string, args ...interface{}) {
	if c.Logger == nil {
		return
	}
	c.Logger.Debugf(msg, args...)
}

// Init initializes this Client with broker and client.
func (c *Client) Init(
	broker proto.MuxBroker, cc grpc.ClientConnInterface,
) {
	c.storage.Init(cc)
	c.wm = proto.NewWindowManagerClient(cc)
	c.msg = proto.NewMessengerClient(cc)
	c.cc = cc
	c.mp = proto.NewKeyMapperClient(cc)
	c.f = proto.NewResourceOpenerClient(cc)
	c.s = proto.NewEventSubscriberClient(cc)
	c.p = proto.NewEventPublisherClient(cc)
	c.broker = broker
	c.clients = make(map[uint64]io.Closer)
	c.servers = make(map[uint64]io.Closer)
	c.failureTimeout = defaultFailureTimeout
}

func (c *Client) serveHandler(h Handler) uint64 {
	var brokerID uint64
	var srv proto.MuxServer

	if tokenHandler, ok := h.(Token); ok {
		brokerID = tokenHandler.ID
	} else {
		var brokerID32 uint32
		brokerID32, srv = proto.AcceptAndServe(c.broker, c.Logger,
			func(handlerID uint32, srv proto.MuxServer) {
				h = browserClientHandler{
					Handler:   h,
					c:         c,
					handlerID: uint64(handlerID),
				}
				hsrv := handler.NewServer(h)
				hsrv.Logger = c.Logger
				proto.RegisterHandlerServer(srv.GRPC(), hsrv)
			})
		brokerID = uint64(brokerID32)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers[brokerID] = &handlerServerResource{h: h, srv: srv, brokerID: brokerID}
	return brokerID
}

func (c *Client) dialWindow(windowID uint64, handlerID int) (Window, error) {
	c.mu.Lock()
	res, ok := c.clients[windowID]
	c.mu.Unlock()
	if ok {
		return res.(*windowClientResource).client, nil
	}

	winConn, err := c.broker.Dial(uint32(windowID))
	if err != nil {
		return nil, err
	}

	cc := proto.NewWindowClient(winConn)
	client := newWindowClient(windowID, c, cc)
	client.logger = c.Logger

	ctx, cancelFn := context.WithCancel(context.Background())

	go proto.MonitorConnection(ctx, c.failureTimeout, winConn, func(reason string) {
		// only applies when connection is closed remotely
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.clients, uint64(windowID))
	})

	c.mu.Lock()
	defer c.mu.Unlock()
	c.clients[uint64(windowID)] = &windowClientResource{
		winConn:       winConn,
		cancelMonitor: cancelFn,
		client:        client,
	}

	return client, nil
}

func (c *Client) getClients() map[uint64]io.Closer {
	return c.clients
}

func (c *Client) getServers() map[uint64]io.Closer {
	return c.servers
}

func (c *Client) forceCloseHandler(brokerID uint64, reason string) error {
	c.tryLog("browser.Client.forceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(brokerID, c.getServers, c.Logger, nopLocker{})
	return err
}

func (c *Client) safeForceCloseHandler(brokerID uint64, reason string) error {
	c.tryLog("browser.Client.safeForceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(brokerID, c.getServers, c.Logger, &c.mu)
	return err
}

func (c *Client) safeForceCloseWindow(brokerID uint64, reason string) error {
	c.tryLog("browser.Client.safeForceCloseWindow(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(brokerID, c.getClients, c.Logger, &c.mu)
	return err
}

type clientSplit func(cc proto.WindowManagerClient,
	ctx context.Context, req *proto.SplitRequest,
	opts ...grpc.CallOption) (*proto.SplitResponse, error)

func (c *Client) split(split clientSplit, h Handler) (Window, error) {
	handlerID := c.serveHandler(h)
	req := proto.SplitRequest{HandlerId: handlerID}
	ctx := context.Background()
	res, err := split(c.wm, ctx, &req)
	if err != nil {
		reason := fmt.Sprintf("error on call to split: %v", err)
		c.safeForceCloseHandler(handlerID, reason)
		return nil, err
	}
	win, err := c.dialWindow(res.GetWindowId(), int(handlerID))
	if err != nil {
		reason := fmt.Sprintf("error dialing to window: %v", err)
		c.safeForceCloseHandler(handlerID, reason)
		return nil, err
	}
	return win, nil
}

// SplitVerticalRight satisfies Browser.
func (c *Client) SplitVerticalRight(h Handler) (Window, error) {
	return c.split((proto.WindowManagerClient).SplitVerticalRight, h)
}

// SplitVerticalLeft satisfies Browser.
func (c *Client) SplitVerticalLeft(h Handler) (Window, error) {
	return c.split((proto.WindowManagerClient.SplitVerticalLeft), h)
}

// SplitHorizontalAbove satisfies Browser.
func (c *Client) SplitHorizontalAbove(h Handler) (Window, error) {
	return c.split((proto.WindowManagerClient.SplitHorizontalAbove), h)
}

// SplitHorizontalBelow satisfies Browser.
func (c *Client) SplitHorizontalBelow(h Handler) (Window, error) {
	return c.split((proto.WindowManagerClient.SplitHorizontalBelow), h)
}

// MergeKeyMap satisfies Browser.
func (c *Client) MergeKeyMap(m map[term.Event]term.Event) error {
	ctx := context.Background()
	req := proto.MergeKeyMapRequest{}

	for from, to := range m {
		protoFrom, protoTo := new(proto.Event), new(proto.Event)
		err := protoFrom.FromModel(from)
		if err != nil {
			return err
		}
		err = protoTo.FromModel(to)
		if err != nil {
			return err
		}
		req.Mappings = append(req.Mappings, &proto.Mapping{
			From: protoFrom,
			To:   protoTo,
		})
	}

	_, err := c.mp.MergeKeyMap(ctx, &req)
	return err
}

// SetMessage satisfies Browser.
func (c *Client) SetMessage(msg string, args ...interface{}) error {
	msg = fmt.Sprintf(msg, args...)

	ctx := context.Background()
	req := proto.SetMessageRequest{Msg: msg}

	_, err := c.msg.SetMessage(ctx, &req)
	return err
}

// Open satisfies Browser.
func (c *Client) Open(resource string) (Handler, error) {
	ctx := context.Background()
	req := proto.OpenResourceRequest{Resource: resource}

	res, err := c.f.Open(ctx, &req)
	if err != nil {
		return nil, err
	}

	return Token{ID: uint64(res.GetHandlerId())}, err
}

// Subscribe satisfies Browser.
func (c *Client) Subscribe(ev term.Event, h EventHandler) error {
	ctx := context.Background()

	protoEv := new(proto.Event)
	err := protoEv.FromModel(ev)
	if err != nil {
		return err
	}

	evh := newClientEventHandler(c, h)
	handlerID := c.serveHandler(evh)
	evh.setHandlerID(handlerID)

	req := proto.SubscribeRequest{Ev: protoEv, HandlerId: handlerID}

	_, err = c.s.Subscribe(ctx, &req)
	if err != nil {
		reason := fmt.Sprintf("error on call to Subscribe: %v", err)
		c.safeForceCloseHandler(handlerID, reason)
	}
	return err
}

// PublishInterrupt satisfies Browser.
func (c *Client) PublishInterrupt() error {
	ctx := context.Background()
	protoEv := new(proto.Event)
	err := protoEv.FromModel(term.Event{Type: term.EventInterrupt})
	if err != nil {
		return err
	}
	req := proto.PublishRequest{Ev: protoEv}

	_, err = c.p.Publish(ctx, &req)
	return err
}

// Focus satisfies Browser.
func (c *Client) Focus() (Window, error) {
	ctx := context.Background()
	req := proto.FocusRequest{}
	res, err := c.wm.Focus(ctx, &req)
	if err != nil {
		return nil, err
	}
	return c.dialWindow(res.GetWindowId(), -1)
}

// Create satisfies browser.Storage
func (c *Client) Create(
	ctx context.Context, ID string, doc interface{},
) error {
	return c.storage.Create(ctx, ID, doc)
}

// Set satisfies browser.Storage
func (c *Client) Set(
	ctx context.Context, ID string, doc interface{},
) error {
	return c.storage.Set(ctx, ID, doc)
}

// Update satisfies browser.Storage
func (c *Client) Update(
	ctx context.Context, ID string, updates []document.Update,
) error {
	return c.storage.Update(ctx, ID, updates)
}

// Get satisfies browser.Storage
func (c *Client) Get(
	ctx context.Context, ID string, doc interface{},
) error {
	return c.storage.Get(ctx, ID, doc)
}

// Delete satisfies browser.Storage
func (c *Client) Delete(
	ctx context.Context, ID string,
) error {
	return c.storage.Delete(ctx, ID)
}

// List satisfies browser.Storage
func (c *Client) List(
	ctx context.Context, filters []document.Filter,
) (document.Iterator, error) {
	return c.storage.List(ctx, filters)
}

// FloatingWindow satisfies browser.WindowManager
func (c *Client) FloatingWindow(
	h Handler, at term.Coordinates, height, width int,
) (Window, error) {
	var atProto proto.Coordinates
	atProto.FromModel(at)

	freq := proto.FloatingWindowRequest{
		At:     &atProto,
		Height: int32(height),
		Width:  int32(width),
	}

	return c.split(func(cc proto.WindowManagerClient,
		ctx context.Context, req *proto.SplitRequest,
		opts ...grpc.CallOption) (*proto.SplitResponse, error) {

		freq.HandlerId = req.GetHandlerId()

		fres, err := cc.FloatingWindow(ctx, &freq)
		if err != nil {
			return nil, err
		}
		return &proto.SplitResponse{WindowId: fres.GetWindowId()}, nil
	}, h)
}

// Close closes all resources associated with this Client.
// This client should not be used after this method is called.
func (c *Client) Close() (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, res := range c.clients {
		resErr := res.Close()
		if resErr != nil {
			err = resErr
		}
	}
	for _, res := range c.servers {
		resErr := res.Close()
		if resErr != nil {
			err = resErr
		}
	}
	if closer, ok := c.cc.(io.Closer); ok {
		ccErr := closer.Close()
		if ccErr != nil {
			err = ccErr
		}
	}
	c.clients = nil
	c.servers = nil
	return
}
