package browser

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// without access to underlying stream (i.e. SendClose),
// waiting a prudent amount of time for all rpcs to finish
// is the best we can do. See https://github.com/grpc/grpc-go/issues/1714
const gracefulShutdownWait = 100 * time.Millisecond

var _ Browser = (*Client)(nil)

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
	f       proto.ResourceOpenerClient
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

func (c browserClientHandler) gracefulShutdown(reason string) {
	time.Sleep(gracefulShutdownWait)
	c.c.safeForceCloseHandler(c.handlerID, reason)
}

func (c browserClientHandler) Close() error {
	go c.gracefulShutdown("Close")
	return c.Handler.Close()
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
	c.f = proto.NewResourceOpenerClient(cc)
	c.p = proto.NewEventPublisherClient(cc)
	c.broker = broker
	c.clients = make(map[uint64]io.Closer)
	c.servers = make(map[uint64]io.Closer)
	c.failureTimeout = defaultFailureTimeout
}

func (c *Client) serveHandler(h Handler) (uint64, bool) {
	var brokerID uint64
	var srv proto.MuxServer

	tokenHandler, ok := h.(Token)
	if ok {
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
	return brokerID, !ok
}

func (c *Client) dialWindow(windowID uint64) (Window, error) {
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

func toProtoOrientation(o Orientation) proto.Orientation {
	switch o {
	case OrientationDefault:
		return proto.Orientation_Default
	case OrientationTop:
		return proto.Orientation_Top
	case OrientationBottom:
		return proto.Orientation_Bottom
	case OrientationLeft:
		return proto.Orientation_Left
	case OrientationRight:
		return proto.Orientation_Right
	default:
		panic("invalid orientation")
	}
}

func (c *Client) split(split clientSplit, o Orientation, h Handler) (Window, error) {
	handlerID, created := c.serveHandler(h)
	req := proto.SplitRequest{HandlerId: handlerID, Orientation: toProtoOrientation(o)}
	ctx := context.Background()
	res, err := split(c.wm, ctx, &req)
	if err != nil {
		if created {
			reason := fmt.Sprintf("browser.Split: %v", err)
			c.safeForceCloseHandler(handlerID, reason)
		}
		return nil, err
	}
	win, err := c.dialWindow(res.GetWindowId())
	if err != nil {
		if created {
			reason := fmt.Sprintf("error dialing to window: %v", err)
			c.safeForceCloseHandler(handlerID, reason)
		}
		return nil, err
	}
	return win, nil
}

// Split satisfies Browser.
func (c *Client) Split(o Orientation, h Handler) (Window, error) {
	return c.split((proto.WindowManagerClient).Split, o, h)
}

// Bar satisfies Browser.
func (c *Client) Bar(o Orientation, h tui.Handler) error {
	handlerID, created := c.serveHandler(NopHandler(h))
	req := proto.BarRequest{HandlerId: handlerID, Orientation: toProtoOrientation(o)}
	ctx := context.Background()
	_, err := c.wm.Bar(ctx, &req)
	if err != nil {
		if created {
			reason := fmt.Sprintf("browser.Bar: %v", err)
			c.safeForceCloseHandler(handlerID, reason)
		}
		return err
	}
	return nil
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
func (c *Client) Open(resource workspace.URI) (Handler, error) {
	ctx := context.Background()
	req := proto.OpenResourceRequest{Resource: resource.String()}

	res, err := c.f.Open(ctx, &req)
	if err != nil {
		return nil, err
	}

	return Token{ID: uint64(res.GetHandlerId())}, err
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
	return c.dialWindow(res.GetWindowId())
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

// Floating satisfies browser.WindowManager
func (c *Client) Floating(
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

		fres, err := cc.Floating(ctx, &freq)
		if err != nil {
			return nil, err
		}
		return &proto.SplitResponse{WindowId: fres.GetWindowId()}, nil
	}, OrientationDefault, h)
}

// Tab satisfies browser.WindowManager
func (c *Client) Tab(uri workspace.URI, name string, h Handler) (Handler, error) {
	handlerID, created := c.serveHandler(h)
	req := proto.TabRequest{
		HandlerId:    handlerID,
		ResourceId:   uri.String(),
		ResourceName: name,
	}
	ctx := context.Background()
	res, err := c.wm.Tab(ctx, &req)
	if err != nil {
		if created {
			reason := fmt.Sprintf("browser.Tab: %v", err)
			c.safeForceCloseHandler(handlerID, reason)
		}
		return nil, err
	}
	return Token{ID: uint64(res.GetTabHandlerId())}, err
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
