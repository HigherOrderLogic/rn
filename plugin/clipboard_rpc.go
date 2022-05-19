package plugin

import (
	"context"
	"io"
	"sync"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui/proto"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
)

const (
	defaultFailureTimeout = 5 * time.Second
)

type clipboardServer struct {
	proto.UnimplementedClipboardServer
	broker                proto.MuxBroker
	mu                    sync.Mutex
	clients               map[uint64]io.Closer
	c                     ClipboardSetter
	failureTimeout        time.Duration
	logger                *log.Logger
	defaultRegisterServer *clipboardRegisterServer
}

type clipboardRegisterClient struct {
	cc            proto.MuxConn
	c             proto.ClipboardRegisterClient
	cancelMonitor func()
	hook          func() error
}

func (c *clipboardRegisterClient) Paste() (string, error) {
	ctx := context.Background()
	req := proto.ClipboardPasteRequest{}

	res, err := c.c.Paste(ctx, &req)
	if err != nil {
		return "", err
	}
	return res.GetData(), nil
}

func (c *clipboardRegisterClient) Copy(data string) error {
	ctx := context.Background()
	req := proto.ClipboardCopyRequest{Data: data}

	_, err := c.c.Copy(ctx, &req)
	if err != nil {
		return err
	}
	return nil
}

func (c *clipboardRegisterClient) Close() (ret error) {
	if c.cancelMonitor == nil {
		return nil
	}
	if c.hook != nil {
		if err := c.hook(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	c.cancelMonitor()
	if err := c.cc.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	c.cancelMonitor = nil
	return
}

func newClipboardServer(
	logger *log.Logger, broker proto.MuxBroker, c Clipboard,
) *clipboardServer {
	ret := new(clipboardServer)
	ret.broker = broker
	ret.c = c
	ret.logger = logger
	ret.failureTimeout = defaultFailureTimeout
	ret.clients = make(map[uint64]io.Closer)
	// to satisfy ClipboardRegister
	ret.defaultRegisterServer = &clipboardRegisterServer{}
	return ret
}

func (s *clipboardServer) getClients() map[uint64]io.Closer {
	return s.clients
}

func (s *clipboardServer) dialRegister(handlerID uint32) (ClipboardRegister, error) {
	handlerConn, err := s.broker.Dial(handlerID)
	if err != nil {
		return nil, err
	}

	c := proto.NewClipboardRegisterClient(handlerConn)
	client := &clipboardRegisterClient{c: c, cc: handlerConn}

	ctx, cancelFn := context.WithCancel(context.Background())
	go proto.MonitorConnection(ctx, s.failureTimeout, handlerConn, func(reason string) {
		_, _ = proto.ForceCloseResource(uint64(handlerID), s.getClients, s.logger, &s.mu)
		s.mu.Lock()
		defer s.mu.Unlock()
		if client.hook != nil {
			client.hook()
		}
	})

	client.cancelMonitor = cancelFn

	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[uint64(handlerID)] = client

	return client, nil
}

func (s *clipboardServer) SetRegister(ctx context.Context, req *proto.SetRegisterRequest) (
	*proto.SetRegisterResponse, error,
) {
	handlerID := req.GetHandlerId()
	register, err := s.dialRegister(uint32(handlerID))
	if err != nil {
		return nil, err
	}

	registerID := req.GetRegisterId()
	err = s.c.SetRegister(registerID, register)
	if err != nil {
		return nil, err
	}

	return new(proto.SetRegisterResponse), nil
}

func (s *clipboardServer) Close() (ret error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.clients {
		err := c.Close()
		if err != nil {
			ret = err
		}
	}

	return
}

type clipboardClient struct {
	mu        sync.Mutex
	registers map[string]*clipboardRegisterServer

	broker         proto.MuxBroker
	cc             grpc.ClientConnInterface
	c              proto.ClipboardClient
	logger         *log.Logger
	remoteRegister *clipboardRegisterClient
}

type clipboardRegisterServer struct {
	proto.UnimplementedClipboardRegisterServer
	srv proto.MuxServer
	r   ClipboardRegister
}

func (c *clipboardRegisterServer) Copy(
	ctx context.Context, req *proto.ClipboardCopyRequest,
) (res *proto.ClipboardCopyResponse, err error) {
	data := req.GetData()
	err = c.r.Copy(data)
	if err != nil {
		return
	}
	res = new(proto.ClipboardCopyResponse)
	return
}

func (c *clipboardRegisterServer) Paste(
	ctx context.Context, req *proto.ClipboardPasteRequest,
) (res *proto.ClipboardPasteResponse, err error) {
	res = new(proto.ClipboardPasteResponse)
	res.Data, err = c.r.Paste()
	if err != nil {
		res = nil
		return
	}
	return
}

func (c *clipboardRegisterServer) Close() error {
	if c.srv != nil {
		c.srv.Stop()
	}
	return nil
}

func newClipboardClient(
	logger *log.Logger, broker proto.MuxBroker, cc proto.MuxConn,
) Clipboard {
	ret := new(clipboardClient)
	ret.broker = broker
	ret.cc = cc
	ret.logger = logger
	ret.c = proto.NewClipboardClient(cc)
	ret.registers = make(map[string]*clipboardRegisterServer)

	c := proto.NewClipboardRegisterClient(cc)
	ret.remoteRegister = &clipboardRegisterClient{c: c, cc: cc}

	return ret
}

func (c *clipboardClient) serveClipboardRegister(
	r ClipboardRegister,
) (*clipboardRegisterServer, uint32) {
	rs := &clipboardRegisterServer{r: r}
	brokerID, srv := proto.AcceptAndServe(c.broker, c.logger,
		func(handlerID uint32, srv proto.MuxServer) {
			proto.RegisterClipboardRegisterServer(srv.GRPC(), rs)
		})
	rs.srv = srv
	return rs, brokerID
}

func (c *clipboardClient) register(registerID string) (ClipboardRegister, error) {
	panic("this should not be called")
}

func (c *clipboardClient) Paste() (string, error) {
	return c.remoteRegister.Paste()
}

func (c *clipboardClient) Copy(data string) error {
	return c.remoteRegister.Copy(data)
}

func (c *clipboardClient) SetRegister(registerID string, r ClipboardRegister) error {
	c.mu.Lock()
	if c, ok := c.registers[registerID]; ok {
		_ = c.Close()
	}
	c.mu.Unlock()
	ctx := context.Background()
	cc, handlerID := c.serveClipboardRegister(r)
	req := proto.SetRegisterRequest{HandlerId: uint64(handlerID), RegisterId: registerID}
	_, err := c.c.SetRegister(ctx, &req)
	if err != nil {
		_ = cc.Close()
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.registers[registerID] = cc

	return nil
}

func (c *clipboardClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, cc := range c.registers {
		_ = cc.Close()
	}
	if closer, ok := c.cc.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
