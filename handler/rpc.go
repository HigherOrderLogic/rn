package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

const defaultRPCTimeout = 1000 * time.Millisecond

type clientCloser interface {
	proto.HandlerClient
	io.Closer
}

// Client satisfies Handler by talking to a remote handler over GRPC.
//
// Note that Draw,Resize and Cursor are conflated into one RPC. This
// Client relies on the fact that runtime first Resizes, then calls Draw,
// and then gets the Cursor.
type Client struct {
	Logger *log.Logger

	width, height int
	cursor        struct {
		term.Coordinates
		show bool
	}

	errors chan error
	client proto.HandlerClient
	resp   struct {
		*proto.HandleResponse
		height, width int
	}
}

// NewClient allocates storage for a new Client and initializes it.
func NewClient(pbClient proto.HandlerClient) *Client {
	ret := new(Client)
	ret.Init(pbClient)
	return ret
}

// Init initialies this Client with pbClient and the given interrupt func.
func (c *Client) Init(pbClient proto.HandlerClient) {
	c.client = pbClient
	// NOTE client breaker is a great concept but interruptDraw is global
	// so if there are multiple client breakers, it's hard to figure out when
	// to re-issue redraw request to avoid endless loop. works
	// c.client = withClientBreaker(c.client, interruptDraw, interruptHandle, c.Logger)

	c.errors = make(chan error)
}

// Errors returns a channel which receives RPC errors.
// The tui.Handler API is not designed for remote handlers, so errors must
// be bubbled up asynchronously.
func (c *Client) Errors() <-chan error {
	return c.errors
}

func (c *Client) collectError(call string, err error) {
	select {
	case c.errors <- err:
	default:
		if c.Logger != nil {
			c.Logger.Errorf("handler.Client error: %s: %s", call, err)
		}
	}
}

// Resize satisfies tui.Handler
func (c *Client) Resize(width, height int) {
	c.width, c.height = width, height
}

func (c *Client) doDraw(w term.Writer, resp *proto.DrawResponse) {
	for y, row := range resp.GetRows() {
		for x, c := range row.Cells {
			cell := c.ToModel()
			w.SetCell(term.Coordinates{X: x, Y: y}, cell)
		}
	}
}

func (c *Client) setNewHandleResponse(comp tui.Component) {
	comp.Resize(c.width, c.height)
	draw := proto.NewDrawResponse(comp, c.width, c.height)
	c.resp.HandleResponse = &proto.HandleResponse{Draw: draw}
	c.resp.width = c.width
	c.resp.height = c.height
	c.cursor.show = false
}

// Draw satisfies tui.Handler
func (c *Client) Draw(w term.Writer) {
	if c.resp.HandleResponse == nil {
		c.Handle(term.Event{Type: term.EventInterrupt})
	}

	if c.height != c.resp.height || c.width != c.resp.width {
		c.setNewHandleResponse(component.StringCentered(loadingCopy))
	}

	c.doDraw(w, c.resp.HandleResponse.GetDraw())

	c.resp.HandleResponse = nil
}

// Handle satisfies tui.Handler
func (c *Client) Handle(ev term.Event) (exit, handled bool) {
	exit, handled, err := c.handle(ev)
	if err != nil {
		c.setNewHandleResponse(component.StringCentered(smtgWrongCopy))
	}
	return
}

func (c *Client) handle(ev term.Event) (exit, handled bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	protoEv := proto.Event{}
	err = protoEv.FromModel(ev)
	if err != nil {
		c.collectError("Handle", err)
		// remote handler is sending bad events so eagerly close
		exit = true
		return
	}

	drawReq := proto.DrawRequest{Width: int32(c.width), Height: int32(c.height)}

	req := proto.HandleRequest{Event: &protoEv, Draw: &drawReq}
	resp, err := c.client.Handle(ctx, &req)
	if err != nil {
		c.collectError("Handle", err)
		return c.resp.GetQuit(), false, err
	}

	exit = resp.GetQuit()
	handled = resp.GetHandled()

	if resp.GetDraw() == nil ||
		resp.GetDraw().GetCursor() == nil ||
		resp.GetDraw().GetCursor().GetPosition() == nil {
		err = errors.New("invalid Cursor from server's Draw response")
		c.collectError("Draw", err)
		return
	}

	c.cursor.Coordinates.X = int(resp.Draw.Cursor.Position.X)
	c.cursor.Coordinates.Y = int(resp.Draw.Cursor.Position.Y)
	c.cursor.show = resp.Draw.Cursor.Show
	c.resp.HandleResponse = resp
	c.resp.width = int(drawReq.Width)
	c.resp.height = int(drawReq.Height)

	return
}

// Cursor satisfies tui.Handler
func (c *Client) Cursor() (pos term.Coordinates, show bool) {
	return c.cursor.Coordinates, c.cursor.show
}

// Man satisfies tui.Handler
func (c *Client) Man() tui.Manual {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	req := proto.ManRequest{}

	resp, err := c.client.Man(ctx, &req)
	if err != nil {
		c.collectError("Man", err)
		return tui.Manual{}
	}

	man := resp.GetMan()
	if man == nil {
		c.collectError("Man", errors.New("missing Man field in ManResponse"))
		return tui.Manual{}
	}

	tuiMan, err := man.ToModel()
	if err != nil {
		c.collectError("Man", err)
		return tui.Manual{}
	}

	return tuiMan
}

func tryLog(logger *log.Logger, msg string, args ...interface{}) {
	if logger != nil {
		logger.Debugf(msg, args...)
	}
}

// OnUnmount satisfies browser.Handler
func (c *Client) OnUnmount() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	req := proto.OnUnmountRequest{}

	_, err := c.client.OnUnmount(ctx, &req)
	if err != nil {
		c.collectError("OnUnmount", err)
		return err
	}
	return nil
}

// Close closes this client and all associated resources.
func (c *Client) Close() error {
	return nil
}

// Server serves a tui.Handler implementation over GRPC.
type Server struct {
	handler tui.Handler
	Logger  *log.Logger
}

// NewServer allocates storage for a new Server and initializes it.
func NewServer(handler tui.Handler) *Server {
	ret := new(Server)
	ret.Init(handler)
	return ret
}

// Init initializes this Server to serve handler.
func (s *Server) Init(handler tui.Handler) {
	s.handler = handler
}

func (s *Server) draw(ctx context.Context, in *proto.DrawRequest) (
	*proto.DrawResponse, error,
) {
	s.handler.Resize(int(in.Width), int(in.Height))
	cursor, show := s.handler.Cursor()
	res := proto.NewDrawResponse(s.handler, int(in.Width), int(in.Height))
	res.Cursor.Position.X = int32(cursor.X)
	res.Cursor.Position.Y = int32(cursor.Y)
	res.Cursor.Show = show
	return res, nil
}

// Handle is an RPC that handles request to an underlying Handler's
// Handle over RPC.
func (s *Server) Handle(ctx context.Context, req *proto.HandleRequest) (
	*proto.HandleResponse, error,
) {
	pev := req.GetEvent()
	if pev == nil {
		return nil, errors.New("missing Event field in HandleRequest")
	}
	ev, err := pev.ToModel()
	if err != nil {
		return nil, err
	}

	var exit, handled bool
	if ev.Type != term.EventInterrupt {
		exit, handled = s.handler.Handle(ev)
	}

	resp, err := s.draw(ctx, req.GetDraw())
	if err != nil {
		return nil, err
	}

	return &proto.HandleResponse{
		Quit:    exit,
		Handled: handled,
		Draw:    resp,
	}, nil
}

// Man is an RPC that handles request to an underlying
// Handler's Man over RPC.
func (s *Server) Man(context.Context, *proto.ManRequest) (
	*proto.ManResponse, error,
) {
	man := s.handler.Man()
	protoMan := new(proto.Manual)
	protoMan.FromModel(man)
	return &proto.ManResponse{Man: protoMan}, nil
}

// OnUnmount is an RPC that handles request to an underlying
// Handler's OnUnmount if it implements it, otherwise it ignores request.
func (s *Server) OnUnmount(context.Context, *proto.OnUnmountRequest) (
	*proto.OnUnmountResponse, error,
) {
	unmounter, ok := s.handler.(interface{ OnUnmount() error })
	if ok {
		err := unmounter.OnUnmount()
		if err != nil {
			return nil, fmt.Errorf("error OnUnmount: %v", err)
		}
	}
	tryLog(s.Logger, "handler.Server.OnUnmount: %v", ok)
	return &proto.OnUnmountResponse{}, nil
}
