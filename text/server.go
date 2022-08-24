package text

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

var (
	errHandlerNotFound = errors.New("handler not found")
)

// Server serves an Editor over GRPC.
type Server struct {
	proto.UnimplementedEditorServer
	Logger *log.Logger

	broker proto.MuxBroker

	// editor handlers currently open
	uriToID     map[string]uint32
	idToHandler map[uint32]Handler

	clients map[uint64]io.Closer

	editor struct {
		Editor
		sync.Locker
	}

	failureTimeout time.Duration
	errChan        chan error
}

// helps map real Handlers with token.Handler
type serverEventHandler struct {
	*eventHandlerClient
	s *Server

	// used to unsubscribe when client could have closed due to a
	// network issue or because remote server closed.
	exitNext bool
}

func (s *serverEventHandler) Handle(ctx context.Context, ev Event) bool {
	if s.exitNext {
		return true
	}

	// only overwrite resource if this event is for a particular resource
	if ev.URI != (workspace.URI{}) {
		brokerID, ok := s.s.uriToID[ev.URI.String()]
		if !ok {
			s.s.tryLog(log.WarnLevel, "(%p editor.Server): could NOT"+
				"dispatch event %#v: handler with resource name %s not found",
				s.s, ev.Type, ev.URI)
			return true
		}
		token := browser.Token{ID: uint64(brokerID)}
		ev.Resource = Token{Token: token, resource: ev.URI}
	}

	// do not hold mutex while waiting for I/O
	s.s.editor.Unlock()
	defer s.s.editor.Lock()

	// cleaning up upon exit=true is performed via quitCallback
	// of eventHandlerClient so there's no need to check for exit here.
	return s.eventHandlerClient.Handle(ctx, ev)
}

// NewServer allocates storage for a new Server and initializes it.
func NewServer(
	broker proto.MuxBroker, editor Editor, lock sync.Locker,
) *Server {
	ret := new(Server)
	ret.Init(broker, editor, lock)
	return ret
}

// Init initializes this Server with broker and browser.
func (s *Server) Init(
	broker proto.MuxBroker, editor Editor, lock sync.Locker,
) {
	s.broker = broker
	s.editor.Editor = editor
	s.editor.Locker = lock
	s.uriToID = make(map[string]uint32)
	s.idToHandler = make(map[uint32]Handler)
	s.clients = make(map[uint64]io.Closer)
	s.failureTimeout = defaultFailureTimeout
	s.errChan = make(chan error)

	evs := []EventType{EventTypeClose, EventTypeOpen}
	s.editor.SubscribeEditorEvents(evs, s)
}

func (s *Server) cleanResource(resource workspace.URI) {
	name := resource.String()
	handlerID, ok := s.uriToID[name]
	if !ok {
		s.tryLog(log.DebugLevel,
			"(%p editor.Server): could not find handler with resource name %s",
			s, name)
	} else {
		delete(s.uriToID, name)
		delete(s.idToHandler, handlerID)
		s.tryLog(log.DebugLevel,
			"(%p editor.Server): cleaned handler %d with resource name %s",
			s, handlerID, name)
	}
}

// Handle satisfies editor.Editor so server can consume EventypeClose and Open events.
func (s *Server) Handle(ctx context.Context, ev Event) bool {
	switch ev.Type {
	case EventTypeOpen:
		_, ok := s.uriToID[ev.URI.String()]
		if !ok {
			s.addNextHandlerResource(ev.URI, ev.Resource)
		}
	case EventTypeClose:
		go func() {
			// wait for other events to be dispatched before cleaning resources
			<-ctx.Done()
			s.editor.Lock()
			defer s.editor.Unlock()

			s.cleanResource(ev.URI)
		}()
	}

	return false
}

func (s *Server) consumeErrors(ctx context.Context, ch <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-ch:
			err = fmt.Errorf("eventHandlerClient error: %v", err)
			s.tryLog(log.DebugLevel, "editor.Server: consumeErrors: %s", err)
			select {
			case s.errChan <- err:
			default:
			}
		}
	}
}

// Errors return an channel of errors produced when performing
// asynchronous operations. It is optional to consume this errors.
func (s *Server) Errors() <-chan error {
	return s.errChan
}

func (s *Server) tryLog(level log.Level, msg string, args ...interface{}) {
	if s.Logger == nil {
		return
	}
	s.Logger.Logf(level, msg, args...)
}

func (s *Server) getClients() map[uint64]io.Closer {
	return s.clients
}

func (s *Server) safeForceCloseHandler(brokerID uint32, reason string) error {
	s.editor.Lock()
	res, ok := s.clients[uint64(brokerID)]
	if ok {
		// make sure that next Handle unsubscribes
		res.(*handlerClientResource).client.exitNext = true
	}
	s.editor.Unlock()

	s.tryLog(log.TraceLevel,
		"editor.Server.safeForceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(s.broker, uint64(brokerID), s.getClients,
		s.editor.Locker)
	return err
}

func (s *Server) dialHandler(handlerID uint32) (EventHandler, error) {
	s.editor.Lock()
	res, ok := s.clients[uint64(handlerID)]
	s.editor.Unlock()
	if ok {
		s.tryLog(log.DebugLevel,
			"(%p editor.Server): found cached client for handlerID: %d",
			s, handlerID)
		return res.(*handlerClientResource).client, nil
	}

	s.tryLog(log.TraceLevel,
		"(%p editor.Server): dialing handlerID: %d", s, handlerID)
	handlerConn, err := s.broker.Dial(handlerID)
	if err != nil {
		return nil, err
	}

	client := newEventHandlerClient(handlerConn, func() {
		s.safeForceCloseHandler(handlerID, "editorEventHandlerClient.onExit")
	})
	client.logger = s.Logger

	ctx, cancelFn := context.WithCancel(context.Background())
	go proto.MonitorConnection(ctx, s.failureTimeout, handlerConn,
		func(reason string) {
			reason = fmt.Sprintf("editor.MonitorConnection(handler): %s", reason)
			s.safeForceCloseHandler(handlerID, reason)
		})
	go s.consumeErrors(ctx, client.errors())

	s.editor.Lock()
	defer s.editor.Unlock()

	h := &serverEventHandler{s: s, eventHandlerClient: client}

	s.clients[uint64(handlerID)] = &handlerClientResource{
		handlerConn:   handlerConn,
		client:        h,
		cancelMonitor: cancelFn,
	}

	return h, nil
}

func (s *Server) addNextHandlerResource(resource workspace.URI, h Handler) uint32 {
	handlerID := s.broker.NextId()
	s.uriToID[resource.String()] = handlerID
	s.idToHandler[handlerID] = h
	s.tryLog(log.TraceLevel,
		"(%p editor.Server): stored handler with name='%s', id=%d",
		s, resource.String(), handlerID)
	return handlerID
}

func (s *Server) ensureAvailable(resource workspace.URI, h Handler) uint32 {
	handlerID, ok := s.uriToID[resource.String()]
	if !ok {
		handlerID = s.addNextHandlerResource(resource, h)
	}
	return handlerID
}

func (s *Server) editHandler(
	resource workspace.URI, get func(workspace.URI) (Handler, error),
) (uint32, error) {
	s.editor.Lock()
	defer s.editor.Unlock()

	h, err := get(resource)
	if err != nil {
		return 0, err
	}

	handlerID := s.ensureAvailable(resource, h)

	return handlerID, nil
}

// Edit satisfies proto.EditorServer
func (s *Server) Edit(ctx context.Context, in *proto.EditRequest) (
	*proto.EditResponse, error,
) {
	uri, err := proto.NewURIFromProto(in.GetResourceName())
	if err != nil {
		return nil, err
	}

	handlerID, err := s.editHandler(uri,
		func(resource workspace.URI) (Handler, error) {
			buf := proto.EditRequestToBuffer(in)
			return s.editor.Edit(uri, buf)
		})
	if err != nil {
		return nil, err
	}

	return &proto.EditResponse{HandlerId: handlerID}, nil
}

// Editor satisfies proto.EditorServer
func (s *Server) Editor(ctx context.Context, in *proto.EditorRequest) (
	*proto.EditorResponse, error,
) {
	uri, err := proto.NewURIFromProto(in.GetResourceName())
	if err != nil {
		return nil, err
	}

	handlerID, err := s.editHandler(uri,
		func(resource workspace.URI) (Handler, error) {
			return s.editor.Editor.Editor(uri)
		})
	if err != nil {
		return nil, err
	}

	return &proto.EditorResponse{HandlerId: handlerID}, nil
}

// Subscribe satisfies proto.EditorServer
func (s *Server) Subscribe(ctx context.Context, in *proto.EditorSubscribeRequest) (
	*proto.EditorSubscribeResponse, error,
) {
	handlerID := in.GetHandlerId()
	handler, err := s.dialHandler(handlerID)
	if err != nil {
		return nil, err
	}

	var evTypes []EventType
	for _, ev := range in.GetType() {
		evType, err := protoTypeToModel(ev)
		if err != nil {
			return nil, err
		}
		evTypes = append(evTypes, evType)
	}

	s.editor.Lock()
	err = s.editor.SubscribeEditorEvents(evTypes, handler)
	s.editor.Unlock()
	if err != nil {
		reason := fmt.Sprintf("failed to subscribe: %v", err)
		s.safeForceCloseHandler(handlerID, reason)
		return nil, err
	}

	return new(proto.EditorSubscribeResponse), nil
}

// Register satisfies proto.EditorServer
func (s *Server) Register(ctx context.Context, in *proto.RegisterCommandRequest) (
	*proto.RegisterCommandResponse, error,
) {
	handlerID := in.GetHandlerId()
	handler, err := s.dialHandler(handlerID)
	if err != nil {
		return nil, err
	}

	commander := FuncCommandHandler(func(ctx context.Context, cmd Command) bool {
		return handler.Handle(ctx, Event{
			Type:     eventTypeCommand,
			Content:  cmd.Name,
			Resource: cmd.Resource,
			URI:      cmd.URI,
			Start:    cmd.Cursor.Content,
			From:     cmd.Cursor.Window,
			cmdArgs:  cmd.Args,
		})
	})

	cmd := in.GetCommand()

	s.editor.Lock()
	err = s.editor.SubscribeCommand(cmd, commander)
	s.editor.Unlock()
	if err != nil {
		reason := fmt.Sprintf("failed to register command '%s': %v", cmd, err)
		s.safeForceCloseHandler(handlerID, reason)
		return nil, err
	}

	return new(proto.RegisterCommandResponse), nil
}

func getLocations(locs []*proto.SetLocationListRequest_Location) (ret []Location) {
	for _, loc := range locs {
		ret = append(ret, Location{
			Attr:    loc.GetAttr().ToModel(),
			From:    loc.GetFrom().ToModel(),
			To:      loc.GetTo().ToModel(),
			Message: loc.GetMsg(),
		})
	}
	return
}

// SetLocationList satisfies proto.EditorServer
func (s *Server) SetLocationList(ctx context.Context, in *proto.SetLocationListRequest) (
	*proto.SetLocationListResponse, error,
) {
	handlerID := in.GetHandlerId()
	locs := in.GetLocations()
	id := in.GetListId()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("SetLocationList", handlerID)
	if !ok {
		return nil, errHandlerNotFound
	}

	err := s.editor.SetLocationList(h, id, LocationSlice(getLocations(locs)))
	if err != nil {
		return nil, err
	}

	return new(proto.SetLocationListResponse), nil
}

// MoveToNextLocation satisfies proto.EditorServer
func (s *Server) MoveToNextLocation(ctx context.Context, in *proto.MoveToLocationRequest) (
	res *proto.MoveToLocationResponse, err error,
) {
	return s.moveToLocation(ctx, in, true)
}

// MoveToPrevLocation satisfies proto.EditorServer
func (s *Server) MoveToPrevLocation(ctx context.Context, in *proto.MoveToLocationRequest) (
	res *proto.MoveToLocationResponse, err error,
) {
	return s.moveToLocation(ctx, in, false)
}

// SetCursor satisfies proto.EditorServer
func (s *Server) SetCursor(ctx context.Context, in *proto.SetCursorRequest) (
	*proto.SetCursorResponse, error,
) {
	handlerID := in.GetHandlerId()
	pos := in.GetPos()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("SetCursor", handlerID)
	if !ok {
		return nil, errHandlerNotFound
	}

	err := s.editor.SetCursor(h, pos.ToModel())
	if err != nil {
		return nil, err
	}

	return new(proto.SetCursorResponse), nil
}

// Cursor satisfies proto.EditorServer
func (s *Server) Cursor(ctx context.Context, in *proto.CursorRequest) (
	*proto.CursorResponse, error,
) {
	handlerID := in.GetHandlerId()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("Cursor", handlerID)
	if !ok {
		return nil, errHandlerNotFound
	}

	pos, err := s.editor.Cursor(h)
	if err != nil {
		return nil, err
	}

	var protoPos proto.Coordinates
	protoPos.FromModel(pos)

	return &proto.CursorResponse{Pos: &protoPos}, nil
}

func (s *Server) moveToLocation(
	ctx context.Context, in *proto.MoveToLocationRequest, next bool,
) (res *proto.MoveToLocationResponse, err error) {
	handlerID := in.GetHandlerId()
	id := in.GetListId()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("moveToLocation", handlerID)
	if !ok {
		return nil, errHandlerNotFound
	}

	if next {
		err = s.editor.MoveToNextLocation(h, id)
	} else {
		err = s.editor.MoveToPrevLocation(h, id)
	}

	if err != nil {
		return nil, err
	}

	res = new(proto.MoveToLocationResponse)
	return res, nil
}

// EditCell satisfies proto.EditorServer
func (s *Server) EditCell(ctx context.Context, in *proto.EditCellRequest) (
	*proto.EditCellResponse, error,
) {
	handlerID := in.GetHandlerId()
	start := in.GetStart().ToModel()
	end := in.GetEnd().ToModel()
	str := in.GetStr()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("Edit", handlerID)
	if !ok {
		return nil, errHandlerNotFound
	}

	from, to, old, err := s.editor.CellEditor(h).Edit(start, end, str)
	if err != nil {
		return nil, err
	}

	var protoFrom, protoTo proto.Coordinates
	protoFrom.FromModel(from)
	protoTo.FromModel(to)

	res := &proto.EditCellResponse{
		From: &protoFrom,
		To:   &protoTo,
		Old:  old,
	}
	return res, nil
}

// RawCells satisfies proto.EditorServer
func (s *Server) RawCells(ctx context.Context, in *proto.RawCellsRequest) (
	*proto.RawCellsResponse, error,
) {
	handlerID := in.GetHandlerId()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.getHandler("RawCells", handlerID)
	if !ok {
		return nil, errHandlerNotFound
	}

	cells, err := s.editor.CellView(h).RawCells()
	if err != nil {
		return nil, err
	}

	return proto.NewRawCellsResponse(cells), nil
}

// Close closes all resources associated with this server.
func (s *Server) Close() (err error) {
	s.editor.Lock()
	defer s.editor.Unlock()

	for _, res := range s.clients {
		resErr := res.Close()
		if resErr != nil {
			err = resErr
		}
	}

	s.clients = nil

	return err
}

func (s *Server) getHandler(call string, handlerID uint32) (Handler, bool) {
	h, ok := s.idToHandler[handlerID]
	s.tryLog(log.TraceLevel,
		"(%p editor.Server): %s: found handler with id %d: %p",
		s, call, handlerID, h)
	return h, ok
}
