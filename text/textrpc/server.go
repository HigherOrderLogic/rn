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
	"sync"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/text"
)

var (
	errHandlerNotFound = errors.New("handler not found")
)

// Server serves an Editor over GRPC.
type Server struct {
	textrpc.UnimplementedEditorServer

	ctx       context.Context
	cancelCtx func()

	editor struct {
		browser.Notifications
		text.Editor
		sync.Locker
	}
}

// NewServer allocates storage for a new Server and initializes it.
func NewServer(
	b browser.Notifications, editor text.Editor, lock sync.Locker,
) *Server {
	ret := new(Server)
	ret.Init(b, editor, lock)
	return ret
}

// Init initializes this Server with broker and browser.
func (s *Server) Init(
	b browser.Notifications, editor text.Editor, lock sync.Locker,
) {
	s.editor.Editor = editor
	s.editor.Locker = lock
	s.editor.Notifications = b
	s.ctx, s.cancelCtx = context.WithCancel(context.Background())
}

// Edit satisfies EditorServer
func (s *Server) Edit(ctx context.Context, in *textrpc.EditRequest) (
	*textrpc.EditResponse, error,
) {
	uri, err := NewURIFromProto(in.GetResourceName())
	if err != nil {
		return nil, err
	}

	err = s.editHandler(uri,
		func(resource workspaceapi.URI) (text.Handler, error) {
			buf := EditRequestToBuffer(in)
			return s.editor.Edit(uri, buf, in.GetReadOnly(), in.GetRecovered())
		})
	if err != nil {
		return nil, err
	}

	return &textrpc.EditResponse{}, nil
}

// Editor satisfies EditorServer
func (s *Server) Editor(ctx context.Context, in *textrpc.EditorRequest) (
	*textrpc.EditorResponse, error,
) {
	uri, err := NewURIFromProto(in.GetResourceName())
	if err != nil {
		return nil, err
	}

	err = s.editHandler(uri,
		func(resource workspaceapi.URI) (text.Handler, error) {
			return s.editor.Editor.Editor(uri)
		})
	if err != nil {
		return nil, err
	}

	return &textrpc.EditorResponse{}, nil
}

// SubscribeEvent satisfies EditorServer
func (s *Server) SubscribeEvent(stream textrpc.Editor_SubscribeEventServer) error {
	defer s.log(log.TraceLevel, "stream event completed: stream=%p", stream)

	var req textrpc.SubscribeEventRequest
	err := stream.RecvMsg(&req)
	s.log(log.TraceLevel, "Subscribe: received request: %v: %v", req.GetType(), err)
	if err != nil {
		return fmt.Errorf("receive stream request: %v", err)
	}

	var evTypes []textapi.EventType
	for _, ev := range req.GetType() {
		evType, err := protoTypeToModel(ev)
		if err != nil {
			return err
		}
		evTypes = append(evTypes, evType)
	}

	handler := newEventStreamClient(s.ctx, stream, s.editor)
	defer handler.Close()

	s.editor.Lock()
	err = s.editor.SubscribeEvents(evTypes, handler)
	s.editor.Unlock()
	if err != nil {
		return fmt.Errorf("subscribe editor events: %v", err)
	}

	s.log(log.TraceLevel, "waiting for unsubscribe: stream=%p", stream)
	defer s.log(log.TraceLevel, "unsubscribed subscriber: stream=%p", stream)

	err = handler.waitForUnsubscribe()
	s.unsubscribeClient(handler)
	return err
}

// SubscribeCommand satisfies EditorServer
func (s *Server) SubscribeCommand(srv textrpc.Editor_SubscribeCommandServer) error {
	var msg textrpc.ClientCommandMessage
	err := srv.RecvMsg(&msg)
	if err != nil {
		return fmt.Errorf("receive subscribe command request: %w", err)
	}
	req := msg.GetRequest()
	if msg.GetType() != textrpc.ClientCommandMessage_Request || req == nil {
		return errors.New("receive subscribe command request: missing request")
	}

	clientStream := newCommandClientStream(s.ctx, srv)
	man := makeStdMan(req.GetCommand())

	s.editor.Lock()
	// NOTE this unlock here causes a race towards the first SendMsg.
	// In practice, this is not a problem, since for a command to be dispatched
	// the user needs to type it first, which gives plenty of time for this
	// goroutine to proceed and schedule the response below via SendMsg.
	err = s.editor.SubscribeCommand(man, clientStream)
	s.editor.Unlock()
	if err != nil {
		return err
	}

	go debug.CapturePanicReport(func() {
		for {
			select {
			case errMsg := <-clientStream.handleCommand:
				if errMsg != "" {
					s.editor.Lock()
					_, err := s.editor.Notify(browserapi.LevelError, errMsg)
					if err != nil {
						s.log(log.ErrorLevel, "%s", errMsg)
						s.log(log.WarnLevel, "notify: %v", err)
					}
					s.editor.Unlock()
				}
			case <-s.ctx.Done():
				return
			}
		}
	})

	resp := textrpc.SubscribeCommandResponse{}
	respMsg := textrpc.ServerCommandMessage{
		Type: textrpc.ServerCommandMessage_Response, Response: &resp}
	if err := srv.SendMsg(&respMsg); err != nil {
		return fmt.Errorf("send bar install response: %w", err)
	}

	err = clientStream.receiveMessages()

	s.editor.Lock()
	defer s.editor.Unlock()

	// replace the command handler in place so command doesn't "disappear"
	if uerr := s.editor.UnsubscribeCommand(man.Name); uerr != nil {
		err = multierror.Append(err, uerr)
	}
	if uerr := s.editor.SubscribeCommand(man,
		text.FuncCommandHandler(func(context.Context, textapi.Command) error {
			return fmt.Errorf("command %q was unsubscribed", man.Name)
		}, nil)); uerr != nil {
		err = multierror.Append(err, uerr)
	}

	return err
}

// SetLocationList satisfies EditorServer
func (s *Server) SetLocationList(ctx context.Context, in *textrpc.SetLocationListRequest) (
	*textrpc.SetLocationListResponse, error,
) {
	locs := in.GetLocations()
	id := in.GetListId()
	pri := in.GetPriority()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("SetLocationList", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	h.SetLocationList(textapi.LocationPriority(pri),
		id, text.LocationSlice(getLocations(locs)))
	return new(textrpc.SetLocationListResponse), nil
}

// MoveToNextLocation satisfies EditorServer
func (s *Server) MoveToNextLocation(ctx context.Context, in *textrpc.MoveToLocationRequest) (
	res *textrpc.MoveToLocationResponse, err error,
) {
	return s.moveToLocation(ctx, in, true)
}

// MoveToPrevLocation satisfies EditorServer
func (s *Server) MoveToPrevLocation(ctx context.Context, in *textrpc.MoveToLocationRequest) (
	res *textrpc.MoveToLocationResponse, err error,
) {
	return s.moveToLocation(ctx, in, false)
}

// SetDefaultAttributes satisfies EditorServer
func (s *Server) SetDefaultAttributes(ctx context.Context, in *textrpc.SetDefaultAttributesRequest) (
	*textrpc.SetDefaultAttributesResponse, error,
) {
	attrs := in.GetAttributes()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("SetDefaultAttributes", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	h.SetDefaultAttributes(attrs.ToModel())
	return new(textrpc.SetDefaultAttributesResponse), nil
}

// SetCursor satisfies EditorServer
func (s *Server) SetCursor(ctx context.Context, in *textrpc.SetCursorRequest) (
	*textrpc.SetCursorResponse, error,
) {
	pos := in.GetPos()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("SetCursorAtScroll", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	h.SetCursorAtScroll(pos.ToModel())
	return new(textrpc.SetCursorResponse), nil
}

// Cursor satisfies EditorServer
func (s *Server) Cursor(ctx context.Context, in *textrpc.CursorRequest) (
	*textrpc.CursorResponse, error,
) {
	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("Cursor", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	pos := h.CursorAtScroll()
	var protoPos termrpc.Coordinates
	protoPos.FromModel(pos)

	return &textrpc.CursorResponse{Pos: &protoPos}, nil
}

// EditCell satisfies EditorServer
func (s *Server) EditCell(ctx context.Context, in *textrpc.EditCellRequest) (
	*textrpc.EditCellResponse, error,
) {
	start := in.GetStart().ToModel()
	end := in.GetEnd().ToModel()
	str := in.GetStr()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("Edit", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	from, to, old := h.CellEditor().Edit(ctx, start, end, str)
	var protoFrom, protoTo termrpc.Coordinates
	protoFrom.FromModel(from)
	protoTo.FromModel(to)

	res := &textrpc.EditCellResponse{
		From: &protoFrom,
		To:   &protoTo,
		Old:  old,
	}
	return res, nil
}

// RawCells satisfies EditorServer
func (s *Server) RawCells(ctx context.Context, in *textrpc.RawCellsRequest) (
	*textrpc.RawCellsResponse, error,
) {
	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("RawCells", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	cells := h.CellView().RawCells()
	return NewRawCellsResponse(cells), nil
}

// Close closes all resources associated with this server.
func (s *Server) Close() (err error) {
	s.cancelCtx()
	return nil
}

func (s *Server) unsubscribeClient(handler *eventStreamClient) {
	s.editor.Lock()
	defer s.editor.Unlock()

	_, err := s.editor.UnsubscribeEvents(handler)
	if err != nil {
		s.log(log.ErrorLevel, "unsubscribe client from all events: %v", err)
	}
}

func (s *Server) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "text.Server").Logf(level, msg, args...)
}

func (s *Server) editHandler(
	resource workspaceapi.URI, get func(workspaceapi.URI) (text.Handler, error),
) error {
	s.editor.Lock()
	defer s.editor.Unlock()

	_, err := get(resource)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) getHandler(call string, uri *textrpc.URI) (text.Handler, bool) {
	muri, err := NewURIFromProto(uri)
	if err != nil {
		return nil, false
	}
	h, err := s.editor.Editor.Editor(muri)
	s.log(log.TraceLevel,
		"(%p editor.Server): %s: get handler with uri %s: %p %v",
		s, call, uri, h, err)
	return h, err == nil
}

func (s *Server) moveToLocation(
	ctx context.Context, in *textrpc.MoveToLocationRequest, next bool,
) (res *textrpc.MoveToLocationResponse, err error) {
	id := in.GetListId()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("moveToLocation", in.GetResourceName())
	if !ok {
		return nil, errHandlerNotFound
	}

	if next {
		ok = h.MoveToNextLocation(id)
	} else {
		ok = h.MoveToPrevLocation(id)
	}
	if !ok {
		err = errors.New("could not move to location")
		return
	}

	res = new(textrpc.MoveToLocationResponse)
	return res, nil
}

func makeStdMan(rpcMan *textrpc.CommandManual) textapi.CommandManual {
	var cmds []textapi.CommandManual
	for _, cmd := range rpcMan.GetCommands() {
		cmds = append(cmds, makeStdMan(cmd))
	}
	return textapi.CommandManual{
		Name:     rpcMan.GetName(),
		Summary:  rpcMan.GetSummary(),
		Synopsis: rpcMan.GetSynopsis(),
		Commands: cmds,
	}
}

func getLocations(locs []*textrpc.SetLocationListRequest_Location) (ret []textapi.Location) {
	for _, loc := range locs {
		ret = append(ret, textapi.Location{
			Attr:    loc.GetAttr().ToModel(),
			From:    loc.GetFrom().ToModel(),
			To:      loc.GetTo().ToModel(),
			Message: loc.GetMsg(),
		})
	}
	return
}
