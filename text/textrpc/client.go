// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package textrpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	grpc "google.golang.org/grpc"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser/browserrpc"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
	termrpc "unstable.build/go-tui/term/termrpc"
	"unstable.build/go-tui/text"
)

const (
	defaultTimeout = 4 * time.Second
)

var _ text.Handler = Token{}

// Token wraps a browser.Token to satisfy editor.Handler.
type Token struct {
	browserrpc.Token
	workspaceapi.URI
}

// Resource satisfies text.Handler
func (t Token) Resource() workspaceapi.URI {
	return t.URI
}

// SetWrap satisfies text.Handler
func (t Token) SetWrap(wrap bool) {
}

// SetCursorAtScroll satisfies text.Handler
func (t Token) SetCursorAtScroll(term.Coordinates) bool {
	return false
}

// ShowCommandBar satisfies text.Handler.
func (t Token) ShowCommandBar(show bool) {
}

// SeekUp satisfies text.Handler.
func (t Token) SeekUp() bool {
	return false
}

// SeekDown satisfies text.Handler.
func (t Token) SeekDown() bool {
	return false
}

// SeekOffset satisfies text.Handler.
func (t Token) SeekOffset() int {
	return 0
}

// MaxSeekOffset satisfies text.Handler.
func (t Token) MaxSeekOffset() int {
	return 0
}

var _ textapi.Editor = (*Client)(nil)

// Client satisfies text.Editor by calling a remote editor over grpc.
type Client struct {
	browser         *browserrpc.Client
	cc              grpc.ClientConnInterface
	ed              EditorClient
	clientCtx       context.Context
	clientCancelCtx func()
}

// NewClient allocates storage for a new Client and initializes it.
func NewClient(ctx context.Context, cc grpc.ClientConnInterface) *Client {
	ret := new(Client)
	ret.Init(ctx, cc)
	return ret
}

// Init initializes this Client with broker and client.
func (c *Client) Init(ctx context.Context, cc grpc.ClientConnInterface) {
	c.ed = NewEditorClient(cc)
	c.cc = cc
	c.browser = browserrpc.NewClient(ctx, cc)
	c.clientCtx, c.clientCancelCtx = context.WithCancel(ctx)
}

// Edit requests editor server to edit buf.
func (c *Client) Edit(file workspaceapi.URI, buf *cell.Buffer) (textapi.Handler, error) {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	req := NewEditRequest(file, buf)

	_, err := c.ed.Edit(ctx, &req)
	if err != nil {
		return nil, err
	}

	return Token{URI: file}, nil
}

// Editor satisfies text.Editor
func (c *Client) Editor(file workspaceapi.URI) (textapi.Handler, error) {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	req := EditorRequest{ResourceName: NewURI(file)}

	_, err := c.ed.Editor(ctx, &req)
	if err != nil {
		return nil, err
	}

	return Token{URI: file}, nil
}

// SubscribeEvents requests the editor server to subscribe sub to ev.
func (c *Client) SubscribeEvents(
	evs []textapi.EventType, h textapi.EventHandler,
) error {
	stream, err := c.ed.SubscribeEvent(c.clientCtx)
	if err != nil {
		return err
	}

	var req SubscribeEventRequest
	for _, ev := range evs {
		req.Type = append(req.Type, protoType(textapi.Event{Type: ev}))
	}

	err = stream.Send(&req)
	if err != nil {
		return fmt.Errorf("stream send request: %v", err)
	}

	handler := newEventStreamServer(c.clientCtx, stream, h)
	go debug.CapturePanicReport(handler.receiveEvents)

	return nil
}

// SubscribeCommand requests the editor server to register cmd with h.
func (c *Client) SubscribeCommand(man textapi.CommandManual, h textapi.CommandHandler) error {
	stream, err := c.ed.SubscribeCommand(c.clientCtx)
	if err != nil {
		return err
	}
	rpcMan := makeProtoManual(man)
	req := SubscribeCommandRequest{Command: &rpcMan}
	sendMsg := ClientCommandMessage{Request: &req, Type: ClientCommandMessage_Request}

	if err := stream.Send(&sendMsg); err != nil {
		return fmt.Errorf("send subscribe command request: %w", err)
	}

	var recvMsg ServerCommandMessage
	err = stream.RecvMsg(&recvMsg)
	if err != nil {
		return fmt.Errorf("send subscribe command request: %w", err)
	}

	if recvMsg.GetType() != ServerCommandMessage_Response || recvMsg.GetResponse() == nil {
		return errors.New("recv subscribe command response: nil response")
	}

	srvStream := newCommandServerStream(c.clientCtx, stream, h)
	go debug.CapturePanicReport(srvStream.receiveMessages)

	return nil
}

func makeLocationListRequest(
	uri workspaceapi.URI, priority textapi.LocationPriority,
	listID string, l textapi.LocationList,
) SetLocationListRequest {
	req := SetLocationListRequest{
		ResourceName: NewURI(uri),
		ListId:       listID,
		Priority:     uint32(priority),
	}

	for loc, ok := l.Current(); ok; loc, ok = l.Next() {
		var from, to termrpc.Coordinates
		var attr termrpc.Attributes
		from.FromModel(loc.From)
		to.FromModel(loc.To)
		attr.FromModel(loc.Attr)
		req.Locations = append(req.Locations, &SetLocationListRequest_Location{
			From: &from,
			To:   &to,
			Attr: &attr,
			Msg:  loc.Message,
		})
	}
	return req // nolint:govet
}

// SetLocationList requests the editor server to set l as the new location list for h.
// Note that h is expected to be the return valu of Edit or a dispatched event, delivered
// via an EventHandler.
func (c *Client) SetLocationList(
	h textapi.Handler, pri textapi.LocationPriority, ID string, l textapi.LocationList,
) error {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	token := h.(Token)
	req := makeLocationListRequest(token.URI, pri, ID, l)
	_, err := c.ed.SetLocationList(ctx, &req)
	return err
}

func (c *Client) moveToLocation(h textapi.Handler, ID string, next bool) (err error) {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	token := h.(Token)
	req := MoveToLocationRequest{ResourceName: NewURI(token.URI), ListId: ID}
	if next {
		_, err = c.ed.MoveToNextLocation(ctx, &req)
	} else {
		_, err = c.ed.MoveToPrevLocation(ctx, &req)
	}
	return err
}

// MoveToPrevLocation requests the editor server to move cursor to the previous location
// in location list identified by ID.
func (c *Client) MoveToPrevLocation(h textapi.Handler, ID string) error {
	err := c.moveToLocation(h, ID, false)
	return err
}

// MoveToNextLocation requests the editor server to move cursor to the next location
// in location list identified by ID.
func (c *Client) MoveToNextLocation(h textapi.Handler, ID string) error {
	err := c.moveToLocation(h, ID, true)
	return err
}

// SetCursor requests the editor server to move cursor to pos
func (c *Client) SetCursor(h textapi.Handler, pos term.Coordinates) error {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	token := h.(Token)
	var protoPos termrpc.Coordinates
	protoPos.FromModel(pos)
	req := SetCursorRequest{Pos: &protoPos, ResourceName: NewURI(token.URI)}
	_, err := c.ed.SetCursor(ctx, &req)
	return err
}

// Cursor requests the editor server to move cursor to pos
func (c *Client) Cursor(h textapi.Handler) (term.Coordinates, error) {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	token := h.(Token)
	req := CursorRequest{ResourceName: NewURI(token.URI)}
	res, err := c.ed.Cursor(ctx, &req)
	if err != nil {
		return term.Coordinates{}, err
	}
	return res.GetPos().ToModel(), nil
}

// CellEditor satisfies text.Editor.
func (c *Client) CellEditor(h textapi.Handler) textapi.CellEditor {
	token := h.(Token)
	return clientWriter{client: c, uri: token.URI}
}

// CellView satisfies text.Editor.
func (c *Client) CellView(h textapi.Handler) textapi.CellView {
	token := h.(Token)
	return clientView{client: c, uri: token.URI}
}

// SetDefaultAttributes satisfies text.Editor.
func (c *Client) SetDefaultAttributes(h textapi.Handler, attrs term.Attributes) error {
	ctx, cancel := c.ctxWithTimeout()
	defer cancel()

	token := h.(Token)
	var rpcAttrs termrpc.Attributes
	rpcAttrs.FromModel(attrs)
	req := SetDefaultAttributesRequest{
		ResourceName: NewURI(token.URI),
		Attributes:   &rpcAttrs,
	}
	_, err := c.ed.SetDefaultAttributes(ctx, &req)
	return err
}

// Close closes all resources associated with this client.
func (c *Client) Close() (ret error) {
	if c.clientCancelCtx != nil {
		c.clientCancelCtx()
		c.clientCancelCtx = nil
		c.cc = nil
	}
	return ret
}

func (c *Client) ctxWithTimeout() (context.Context, func()) {
	ctx, cancel := context.WithTimeout(c.clientCtx, defaultTimeout)
	return ctx, cancel
}

func makeProtoManual(man textapi.CommandManual) CommandManual {
	var cmds []*CommandManual
	for _, cmd := range man.Commands {
		childManual := new(CommandManual)
		*childManual = makeProtoManual(cmd)
		cmds = append(cmds, childManual)
	}
	ret := CommandManual{
		Name:     man.Name,
		Summary:  man.Summary,
		Synopsis: man.Synopsis,
		Commands: cmds,
	}
	return ret // nolint:govet
}
