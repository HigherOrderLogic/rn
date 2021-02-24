package editor

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	gracefulShutdownWait  = 400 * time.Millisecond
	defaultFailureTimeout = 5 * time.Second
)

// Client satisfies editor.Editor by calling a remote editor over grpc.
type Client struct {
	Logger *log.Logger

	// resources invariant
	mu sync.Mutex

	broker proto.MuxBroker
	cc     grpc.ClientConnInterface
	ed     proto.EditorClient

	// event handler server resources. event handler servers are created on
	// calls to Subscribe.
	servers map[uint64]io.Closer
}

// NewClient allocates storage for a new Client and initializes it.
func NewClient(
	broker proto.MuxBroker, cc grpc.ClientConnInterface,
) *Client {
	ret := new(Client)
	ret.Init(broker, cc)
	return ret
}

// Init initializes this Client with broker and client.
func (c *Client) Init(
	broker proto.MuxBroker, cc grpc.ClientConnInterface,
) {
	c.ed = proto.NewEditorClient(cc)
	c.cc = cc
	c.broker = broker
	c.servers = make(map[uint64]io.Closer)
}

func (c *Client) tryLog(msg string, args ...interface{}) {
	if c.Logger == nil {
		return
	}
	c.Logger.Debugf(msg, args...)
}

func (c *Client) getServers() map[uint64]io.Closer {
	return c.servers
}

func (c *Client) safeForceCloseHandler(brokerID uint32, reason string) error {
	c.tryLog("editor.Client.safeForceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(uint64(brokerID), c.getServers, c.Logger, &c.mu)
	return err
}

func (c *Client) serveHandler(h EventHandler) uint32 {
	brokerID, srv := proto.AcceptAndServe(c.broker, c.Logger,
		func(handlerID uint32, srv proto.MuxServer) {
			s := newEventHandlerServer(h, func() {
				time.Sleep(gracefulShutdownWait)
				c.safeForceCloseHandler(handlerID, "editorEventHandlerServer.onExit")
			})
			s.logger = c.Logger
			proto.RegisterEditorEventHandlerServer(srv.GRPC(), s)
		})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers[uint64(brokerID)] = &handlerServerResource{h: h, srv: srv}
	return brokerID
}

// Edit requests editor server to edit buf.
func (c *Client) Edit(name string, buf *cell.Buffer) (Handler, error) {
	ctx := context.Background()
	req := proto.BufferToEditRequest(buf)
	req.ResourceName = name

	res, err := c.ed.Edit(ctx, &req)
	if err != nil {
		return nil, err
	}

	return browser.Token{ID: uint64(res.GetHandlerId())}, nil
}

// SubscribeEditor requests the editor server to subscribe sub to ev.
func (c *Client) SubscribeEditor(evType EventType, h EventHandler) error {
	ctx := context.Background()

	handlerID := c.serveHandler(h)

	protoType := Event{Type: evType}.protoType()
	req := proto.EditorSubscribeRequest{Type: protoType, HandlerId: handlerID}
	_, err := c.ed.Subscribe(ctx, &req)
	if err != nil {
		reason := fmt.Sprintf("editor.Client.Subscribe: %v", err)
		c.safeForceCloseHandler(handlerID, reason)
		return err
	}

	return nil
}

// Register requests the editor server to register cmd with h.
func (c *Client) Register(cmd string, h CommandHandler) error {
	ctx := context.Background()

	// re-use EventHandler logic
	handlerID := c.serveHandler(FuncEventHandler(func(ev Event) bool {
		cmd := Command{
			Name:         ev.Content,
			ResourceName: ev.ResourceName,
			Resource:     ev.Resource,
			Cursor:       ev.Start,
		}
		return h.HandleCommand(cmd)
	}))

	req := proto.RegisterCommandRequest{Command: cmd, HandlerId: handlerID}
	_, err := c.ed.Register(ctx, &req)
	if err != nil {
		reason := fmt.Sprintf("editor.Client.Register: %v", err)
		c.safeForceCloseHandler(handlerID, reason)
		return err
	}

	return nil
}

func makeLocationListRequest(
	handlerID uint32, listID string, l LocationList,
) proto.SetLocationListRequest {
	req := proto.SetLocationListRequest{
		HandlerId: handlerID,
		ListId:    listID,
	}

	for loc, ok := l.Current(); ok; loc, ok = l.Next() {
		var from, to proto.Coordinates
		var attr proto.Attributes
		from.FromModel(loc.From)
		to.FromModel(loc.To)
		attr.FromModel(loc.Attr)
		req.Locations = append(req.Locations, &proto.SetLocationListRequest_Location{
			From: &from,
			To:   &to,
			Attr: &attr,
			Msg:  loc.Message,
		})
	}
	return req
}

// SetLocationList requests the editor server to set l as the new location list for h.
// Note that h is expected to be the return valu of Edit or a dispatched event, delivered
// via an EventHandler.
func (c *Client) SetLocationList(h Handler, ID string, l LocationList) error {
	ctx := context.Background()
	token, ok := h.(browser.Token)
	if !ok {
		panic("SetLocationList: invalid Handler argument")
	}
	req := makeLocationListRequest(uint32(token.ID), ID, l)
	_, err := c.ed.SetLocationList(ctx, &req)
	return err
}

func (c *Client) moveToLocation(h Handler, ID string, next bool) (err error) {
	ctx := context.Background()
	token, ok := h.(browser.Token)
	if !ok {
		panic("MoveToNextLocation: invalid Handler argument")
	}
	req := proto.MoveToLocationRequest{HandlerId: uint32(token.ID), ListId: ID}
	if next {
		_, err = c.ed.MoveToNextLocation(ctx, &req)
	} else {
		_, err = c.ed.MoveToPrevLocation(ctx, &req)
	}
	return err
}

// MoveToPrevLocation requests the editor server to move cursor to the previous location
// in location list identified by ID.
func (c *Client) MoveToPrevLocation(h Handler, ID string) error {
	return c.moveToLocation(h, ID, false)
}

// MoveToNextLocation requests the editor server to move cursor to the next location
// in location list identified by ID.
func (c *Client) MoveToNextLocation(h Handler, ID string) error {
	return c.moveToLocation(h, ID, true)
}

// SetCursor requests the editor server to move cursor to pos
func (c *Client) SetCursor(h Handler, pos term.Coordinates) error {
	ctx := context.Background()
	token, ok := h.(browser.Token)
	if !ok {
		panic("SetCursor: invalid Handler argument")
	}
	var protoPos proto.Coordinates
	protoPos.FromModel(pos)
	req := proto.SetCursorRequest{Pos: &protoPos, HandlerId: uint32(token.ID)}
	_, err := c.ed.SetCursor(ctx, &req)
	return err
}

// Cursor requests the editor server to move cursor to pos
func (c *Client) Cursor(h Handler) (term.Coordinates, error) {
	ctx := context.Background()
	token, ok := h.(browser.Token)
	if !ok {
		panic("Cursor: invalid Handler argument")
	}
	req := proto.CursorRequest{HandlerId: uint32(token.ID)}
	res, err := c.ed.Cursor(ctx, &req)
	if err != nil {
		return term.Coordinates{}, err
	}
	return res.GetPos().ToModel(), nil
}

// Writer satisfies editor.Editor.
func (c *Client) Writer(h Handler) Writer {
	token, ok := h.(browser.Token)
	if !ok {
		panic("SetLocationList: invalid Handler argument")
	}
	return clientWriter{client: c, handlerID: uint32(token.ID)}
}

// Reader satisfies editor.Editor.
func (c *Client) Reader(h Handler) Reader {
	token, ok := h.(browser.Token)
	if !ok {
		panic("SetLocationList: invalid Handler argument")
	}
	return clientReader{client: c, handlerID: uint32(token.ID)}
}

// Close closes all resources associated with this client.
func (c *Client) Close() (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if closer, ok := c.cc.(io.Closer); ok {
		ccErr := closer.Close()
		if ccErr != nil {
			err = ccErr
		}
	}
	for _, res := range c.servers {
		resErr := res.Close()
		if resErr != nil {
			err = resErr
		}
	}

	c.servers = nil

	return err
}
