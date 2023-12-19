package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	"unstable.build/go-tui/component"

	"unstable.build/go-tui/term"
	termpb "unstable.build/go-tui/term/rpc"
)

const (
	defaultRPCTimeout = 1000 * time.Millisecond
	smtgWrongCopy     = `

          ___
         /___/\_               
        _\   \/_/\__           
      __\       \/_/\          
      \   __    __ \ \         
     __\  \_\   \_\ \ \   __   
    /_/\\   __   __  \ \_/_/\  
    \_\/_\__\/\__\/\__\/_\_\/  
       \_\/_/\       /_\_\/    
          \_\/       \_\/      
    

Uh, Houston, we've had a problem
`
)

// Client satisfies Handler by talking to a remote handler over GRPC.
//
// Note that Draw,Resize and Cursor are conflated into one RPC. This
// Client relies on the fact that runtime first Resizes, then calls Draw,
// and then gets the Cursor.
type Client struct {
	width, height int
	cursor        struct {
		term.Coordinates
		show  bool
		style term.CursorStyle
	}

	errors chan error
	client HandlerClient
	resp   struct {
		*HandleResponse
		height, width int
	}
}

// NewClient allocates storage for a new Client and initializes it.
func NewClient(pbClient HandlerClient) *Client {
	ret := new(Client)
	ret.Init(pbClient)
	return ret
}

// Init initialies this Client with pbClient and the given interrupt func.
func (c *Client) Init(pbClient HandlerClient) {
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
		c.log(log.ErrorLevel, "handler.Client error: %s: %s", call, err)
	}
}

func (c *Client) log(level log.Level, msg string, args ...interface{}) {
	log.
		WithField(logging.KeyClass, "handler.Client").Logf(level, msg, args...)
}

// Resize satisfies tui.Handler
func (c *Client) Resize(width, height int) {
	c.width, c.height = width, height
}

func (c *Client) doDraw(w term.Writer, resp *DrawResponse) {
	for y, row := range resp.GetRows() {
		for x, c := range row.Cells {
			cell := c.ToModel()
			w.SetCell(term.Coordinates{X: x, Y: y}, cell)
		}
	}
}

func (c *Client) setNewHandleResponse(comp tui.Component) {
	comp.Resize(c.width, c.height)
	draw := NewDrawResponse(comp, c.width, c.height)
	c.resp.HandleResponse = &HandleResponse{Draw: draw}
	c.resp.width = c.width
	c.resp.height = c.height
	c.cursor.show = false
}

// Draw satisfies tui.Handler
func (c *Client) Draw(w term.Writer) {
	// NOTE conflating Draw+Handle at the proto level
	// is pointless if we call Handle on every Draw, but
	// this is leftovers from having a client circuit breaker
	// which we will be able to re-introduce once interrupts
	// with context are implemented.
	// This effectively fixes Interrupts handled in bundle
	// with other events not triggering a second call to Draw
	// which is what would wind up calling the server's Handle/Draw
	// method.
	c.Handle(term.Event{Type: term.EventInterrupt})
	c.doDraw(w, c.resp.HandleResponse.GetDraw())
}

// Handle satisfies tui.Handler
func (c *Client) Handle(ev term.Event) (exit, handled bool) {
	exit, handled, err := c.handle(ev)
	if err != nil {
		c.setNewHandleResponse(component.NewStringWithConfig(smtgWrongCopy,
			component.StringConfig{Alignment: component.SpanAlignmentCentered}))
	}
	return
}

func (c *Client) handle(ev term.Event) (exit, handled bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	protoEv := termpb.Event{}
	err = protoEv.FromModel(ev)
	if err != nil {
		c.collectError("Handle", err)
		// remote handler is sending bad events so eagerly close
		exit = true
		return
	}

	drawReq := DrawRequest{Width: int32(c.width), Height: int32(c.height)}

	req := HandleRequest{Event: &protoEv, Draw: &drawReq}
	resp, err := c.client.Handle(ctx, &req)
	runtime.KeepAlive(c)
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
	c.cursor.style = term.CursorStyle(resp.Draw.Cursor.Style)
	c.resp.HandleResponse = resp
	c.resp.width = int(drawReq.Width)
	c.resp.height = int(drawReq.Height)

	return
}

// Cursor satisfies tui.Handler
func (c *Client) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return c.cursor.Coordinates, c.cursor.style, c.cursor.show
}

// Man satisfies tui.Handler
func (c *Client) Man() tui.Manual {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	req := ManRequest{}

	resp, err := c.client.Man(ctx, &req)
	runtime.KeepAlive(c)
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

// Close satisfies browser.Handler
// Close closes this client and all associated resources.
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRPCTimeout)
	defer cancel()

	req := CloseRequest{}
	_, err := c.client.Close(ctx, &req)
	runtime.KeepAlive(c)
	if err != nil {
		return fmt.Errorf("proto.HandlerClient.Close: %w", err)
	}
	return nil
}

type dimensions struct {
	width  int
	height int
}

// Server serves a tui.Handler implementation over GRPC.
type Server struct {
	UnimplementedHandlerServer
	handler    tui.Handler
	dimensions atomic.Value
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
	s.dimensions.Store(dimensions{})
}

func (s *Server) draw(ctx context.Context, in *DrawRequest) (
	*DrawResponse, error,
) {
	dim := s.dimensions.Load().(dimensions)
	if int(in.Width) != dim.width || int(in.Height) != dim.height {
		s.handler.Resize(int(in.Width), int(in.Height))
	}
	s.dimensions.Store(dimensions{width: int(in.Width), height: int(in.Height)})
	cursor, style, show := s.handler.Cursor()
	res := NewDrawResponse(s.handler, int(in.Width), int(in.Height))
	res.Cursor.Position.X = int32(cursor.X)
	res.Cursor.Position.Y = int32(cursor.Y)
	res.Cursor.Show = show
	res.Cursor.Style = int32(style)
	return res, nil
}

func (s *Server) log(level log.Level, msg string, args ...interface{}) {
	log.
		WithField(logging.KeyClass, "handler.Server").Logf(level, msg, args...)
}

// Handle is an RPC that handles request to an underlying Handler's
// Handle over RPC.
func (s *Server) Handle(ctx context.Context, req *HandleRequest) (
	*HandleResponse, error,
) {
	pev := req.GetEvent()
	if pev == nil {
		return nil, errors.New("missing Event field in HandleRequest")
	}
	ev, err := pev.ToModel()
	if err != nil {
		return nil, err
	}

	s.log(log.TraceLevel, "handler.Server.Handle(%v)", ev)

	var exit, handled bool
	if ev.Type != term.EventInterrupt {
		exit, handled = s.handler.Handle(ev)
	}

	resp, err := s.draw(ctx, req.GetDraw())
	if err != nil {
		return nil, err
	}

	return &HandleResponse{
		Quit:    exit,
		Handled: handled,
		Draw:    resp,
	}, nil
}

// Man is an RPC that handles request to an underlying
// Handler's Man over RPC.
func (s *Server) Man(context.Context, *ManRequest) (
	*ManResponse, error,
) {
	man := s.handler.Man()
	protoMan := new(termpb.Manual)
	protoMan.FromModel(man)
	return &ManResponse{Man: protoMan}, nil
}

// Close is an RPC that handles request to an underlying
// Handler's Close if it implements it, otherwise it ignores request.
func (s *Server) Close(ctx context.Context, req *CloseRequest) (
	*CloseResponse, error,
) {
	closer, ok := s.handler.(io.Closer)
	if ok {
		err := closer.Close()
		if err != nil {
			return nil, fmt.Errorf("error Close: %v", err)
		}
	}
	s.log(log.DebugLevel, "handler.Server.Close: %v", ok)
	return &CloseResponse{}, nil
}
