package editor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/proto"
	log "github.com/sirupsen/logrus"
)

var (
	errHandlerNotFound = errors.New("handler not found")
)

// Server serves an Editor over GRPC.
type Server struct {
	Logger *log.Logger

	broker proto.MuxBroker

	// editor handlers currently open
	nameToID    map[string]uint32
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

func (s *serverEventHandler) Handle(ev Event) bool {
	if s.exitNext {
		return true
	}

	brokerID, ok := s.s.nameToID[ev.ResourceName]
	if !ok {
		s.s.tryLog("(%p editor.Server): could NOT dispatch event: handler with resource name %s not found",
			s.s, ev.ResourceName)
		return true
	}
	ev.Resource = browser.Token{ID: uint64(brokerID)}

	// do not hold mutex while waiting for I/O
	s.s.editor.Unlock()
	defer s.s.editor.Lock()

	// cleaning up upon exit=true is performed via quitCallback
	// of eventHandlerClient so there's no need to check for exit here.
	return s.eventHandlerClient.Handle(ev)
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
	s.nameToID = make(map[string]uint32)
	s.idToHandler = make(map[uint32]Handler)
	s.clients = make(map[uint64]io.Closer)
	s.failureTimeout = defaultFailureTimeout
	s.errChan = make(chan error)

	go func() {
		s.editor.Locker.Lock()
		defer s.editor.Locker.Unlock()
		s.editor.SubscribeEditor(EventTypeClose, s)
		s.editor.SubscribeEditor(EventTypeOpen, s)
	}()
}

func (s *Server) cleanResource(name string) {
	// allow other subscribers to take action first
	// TODO this breaks if client opens and closes quickly
	time.Sleep(gracefulShutdownWait)

	s.editor.Lock()
	defer s.editor.Unlock()

	handlerID, ok := s.nameToID[name]
	if !ok {
		s.tryLog("(%p editor.Server): could not find handler with resource name %s",
			s, name)
	} else {
		delete(s.nameToID, name)
		delete(s.idToHandler, handlerID)
		s.tryLog("(%p editor.Server): cleaned handler with resource name %s",
			s, name)
	}
}

// Handle satisfies editor.Editor so server can consume EventypeClose and Open events.
func (s *Server) Handle(ev Event) bool {
	switch ev.Type {
	case EventTypeOpen:
		_, ok := s.nameToID[ev.ResourceName]
		if !ok {
			s.addNextHandlerResource(ev.ResourceName, ev.Resource)
		}
	case EventTypeClose:
		go s.cleanResource(ev.ResourceName)
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
			s.tryLog("%v", err)
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

func (s *Server) tryLog(msg string, args ...interface{}) {
	if s.Logger == nil {
		return
	}
	s.Logger.Debugf(msg, args...)
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

	s.tryLog("editor.Server.safeForceCloseHandler(%d, reason=%s)", brokerID, reason)
	_, err := proto.ForceCloseResource(uint64(brokerID), s.getClients, s.Logger, s.editor.Locker)
	return err
}

func (s *Server) dialHandler(handlerID uint32) (EventHandler, error) {
	s.editor.Lock()
	res, ok := s.clients[uint64(handlerID)]
	s.editor.Unlock()
	if ok {
		s.tryLog("(%p editor.Server): found cached client for handlerID: %d", s, handlerID)
		return res.(*handlerClientResource).client, nil
	}

	s.tryLog("(%p editor.Server): dialing handlerID: %d", s, handlerID)
	handlerConn, err := s.broker.Dial(handlerID)
	if err != nil {
		return nil, err
	}

	client := newEventHandlerClient(handlerConn, func() {
		s.safeForceCloseHandler(handlerID, "editorEventHandlerClient.onExit")
	})
	client.logger = s.Logger

	ctx, cancelFn := context.WithCancel(context.Background())
	go proto.MonitorConnection(ctx, s.failureTimeout, handlerConn, func(reason string) {
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

func (s *Server) addNextHandlerResource(name string, h Handler) uint32 {
	handlerID := s.broker.NextId()
	s.nameToID[name] = handlerID
	s.idToHandler[handlerID] = h
	s.tryLog("(%p editor.Server): stored handler with name, ID: %s,%d", s, name, handlerID)
	return handlerID
}

// Edit satisfies proto.EditorServer
func (s *Server) Edit(ctx context.Context, in *proto.EditRequest) (
	*proto.EditResponse, error,
) {
	s.editor.Lock()
	defer s.editor.Unlock()

	resourceName := in.GetResourceName()
	buf := proto.EditRequestToBuffer(in)
	h, err := s.editor.Edit(resourceName, buf)
	if err != nil {
		return nil, err
	}

	handlerID, ok := s.nameToID[resourceName]
	if !ok {
		handlerID = s.addNextHandlerResource(resourceName, h)
	}

	res := &proto.EditResponse{
		HandlerId: handlerID,
	}

	return res, nil
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

	evType, err := protoTypeToModel(in.GetType())
	if err != nil {
		return nil, err
	}

	s.editor.Lock()
	err = s.editor.SubscribeEditor(evType, handler)
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

	commander := FuncCommandHandler(func(cmd Command) bool {
		return handler.Handle(Event{
			Type:         eventTypeCommand,
			Content:      cmd.Name,
			Resource:     cmd.Resource,
			ResourceName: cmd.ResourceName,
			Start:        cmd.Cursor,
		})
	})

	cmd := in.GetCommand()

	s.editor.Lock()
	err = s.editor.Register(cmd, commander)
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

	h, ok := s.idToHandler[handlerID]
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

	h, ok := s.idToHandler[handlerID]
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

	h, ok := s.idToHandler[handlerID]
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

	h, ok := s.idToHandler[handlerID]
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

// Insert satisfies proto.EditorServer
func (s *Server) Insert(ctx context.Context, in *proto.InsertRequest) (
	*proto.InsertResponse, error,
) {
	handlerID := in.GetHandlerId()
	at := in.GetAt().ToModel()
	str := in.GetStr()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.idToHandler[handlerID]
	if !ok {
		return nil, errHandlerNotFound
	}

	from, to, err := s.editor.Writer(h).Insert(at, str)
	if err != nil {
		return nil, err
	}

	var protoFrom, protoTo proto.Coordinates
	protoFrom.FromModel(from)
	protoTo.FromModel(to)

	res := &proto.InsertResponse{
		From: &protoFrom,
		To:   &protoTo,
	}
	return res, nil
}

// Delete satisfies proto.EditorServer
func (s *Server) Delete(ctx context.Context, in *proto.DeleteRequest) (
	*proto.DeleteResponse, error,
) {
	handlerID := in.GetHandlerId()
	from := in.GetFrom().ToModel()
	to := in.GetTo().ToModel()

	s.editor.Lock()
	defer s.editor.Unlock()

	h, ok := s.idToHandler[handlerID]
	if !ok {
		return nil, errHandlerNotFound
	}

	start, end, str, err := s.editor.Writer(h).Delete(from, to)
	if err != nil {
		return nil, err
	}

	var protoStart, protoEnd proto.Coordinates
	protoStart.FromModel(start)
	protoEnd.FromModel(end)

	res := &proto.DeleteResponse{
		Start: &protoStart,
		End:   &protoEnd,
		Str:   str,
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

	h, ok := s.idToHandler[handlerID]
	if !ok {
		return nil, errHandlerNotFound
	}

	cells, err := s.editor.Reader(h).RawCells()
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
