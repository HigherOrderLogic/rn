package plugin

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ernestrc/go-tui/proto"
	"github.com/hashicorp/go-plugin"
)

const (
	defDurationGracefulShutServer = 5 * time.Second
	defDurationGracefulShutClient = 300 * time.Millisecond
)

type granteeServer struct {
	mu        sync.Mutex
	req       []Permission
	broker    proto.MuxBroker
	grantee   Grantee
	connected bool
	keepAlive chan struct{}
	osExit    func(int)

	durationGracefulShut time.Duration
	keepAliveTimeout     time.Duration
}

func newGranteeServer(
	broker proto.MuxBroker, grantee Grantee, req []Permission,
	keepAlive time.Duration,
) proto.GranteeServer {
	ret := new(granteeServer)
	ret.broker = broker
	ret.grantee = grantee
	ret.req = req
	ret.durationGracefulShut = defDurationGracefulShutServer
	ret.osExit = os.Exit
	if keepAlive != time.Duration(0) {
		ret.keepAlive = make(chan struct{})
		ret.keepAliveTimeout = keepAlive * 2
	}
	return ret
}

func forceStopTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}

	select {
	case <-timer.C:
	default:
	}
}

func (s *granteeServer) monitorKeepAlive() {
	t := time.NewTimer(s.keepAliveTimeout)

	s.mu.Lock()
	ch := s.keepAlive
	s.mu.Unlock()

	for {
		select {
		case <-t.C:
			s.doShutdown("lost connectivity to host: failed to send a health check in time")
		case <-ch:
			forceStopTimer(t)
			t.Reset(s.keepAliveTimeout)
		}
	}
}

func (s *granteeServer) Permissions(ctx context.Context, req *proto.PermRequest) (
	*proto.PermResponse, error,
) {
	resp := new(proto.PermResponse)
	for _, perm := range s.req {
		resp.Perms = append(resp.Perms, &proto.Permission{Id: string(perm)})
	}

	var cfg jsonMap
	protoCfg := req.GetConfig()
	if protoCfg == nil {
		cfg.mapConfig = make(map[string]interface{})
	} else {
		err := cfg.UnmarshalText(protoCfg)
		if err != nil {
			return nil, fmt.Errorf("could not decode incoming plugin config: %v", err)
		}
	}

	s.mu.Lock()
	connected := s.connected
	s.mu.Unlock()

	if !connected {
		s.grantee.Connected(s.broker, cfg)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.connected = true
		if s.keepAlive != nil {
			go s.monitorKeepAlive()
		}
	}

	return resp, nil
}

func (s *granteeServer) OnGrant(ctx context.Context, req *proto.OnPermGrantRequest) (
	*proto.OnPermGrantResponse, error,
) {
	/* only trigger OnPermission* for permissions that were actually requested */

	var denied []Permission
	var granted []Grant
	for _, den := range req.Denied {
		for _, requested := range s.req {
			if string(requested) == den.Id {
				denied = append(denied, Permission(den.Id))
			}
		}
	}
	for _, gr := range req.Granted {
		for _, requested := range s.req {
			if string(requested) == gr.Id {
				granted = append(granted, Grant{
					Token: gr.GrantId, Permission: Permission(gr.Id),
				})
			}
		}
	}

	if len(granted) != 0 {
		s.grantee.PermissionGranted(granted)
	}
	if len(denied) != 0 {
		s.grantee.PermissionDenied(denied)
	}

	return new(proto.OnPermGrantResponse), nil
}

func (s *granteeServer) doShutdown(reason string) error {
	if err := s.grantee.Shutdown(reason); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.keepAlive != nil {
		close(s.keepAlive)
		s.keepAlive = nil
	}
	if s.osExit != nil {
		go func() {
			time.Sleep(s.durationGracefulShut)
			s.osExit(0)
		}()
	}
	return nil
}

func (s *granteeServer) Shutdown(ctx context.Context, in *proto.ShutdownRequest) (
	*proto.ShutdownResponse, error,
) {
	err := s.doShutdown(in.GetReason())
	if err != nil {
		return nil, err
	}
	return new(proto.ShutdownResponse), nil
}

func (s *granteeServer) Health(context.Context, *proto.HealthRequest) (
	*proto.HealthResponse, error,
) {
	if err := s.grantee.Health(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.keepAlive != nil {
		select {
		case s.keepAlive <- struct{}{}:
		}
	}
	return new(proto.HealthResponse), nil
}

type granteeClient struct {
	mBroker proto.MuxBroker
	client  proto.GranteeClient

	pClient *plugin.Client
}

func newGranteeClient(
	broker proto.MuxBroker, client proto.GranteeClient,
) *granteeClient {
	ret := new(granteeClient)
	ret.client = client
	ret.mBroker = broker
	return ret
}

func (c *granteeClient) broker() proto.MuxBroker {
	return c.mBroker
}

func (c *granteeClient) permissions(ctx context.Context, config Config) (
	perms []*proto.Permission, err error,
) {
	var req proto.PermRequest
	if config == nil {
		req.Config = []byte("{}")
	} else {
		mConfig := toInternalConfig(config)
		jsonConfig := jsonMap{mConfig}
		req.Config, err = jsonConfig.MarshalText()
		if err != nil {
			err = fmt.Errorf("could not marshal config: %v", err)
			return
		}
	}

	resp, err := c.client.Permissions(ctx, &req)
	if err != nil {
		return nil, err
	}
	return resp.GetPerms(), nil
}

func (c *granteeClient) sendGrants(
	ctx context.Context,
	denied []*proto.Permission,
	granted []*proto.PermissionGrant,
) error {
	req := new(proto.OnPermGrantRequest)

	for _, dn := range denied {
		req.Denied = append(req.Denied, dn)
	}

	for _, gr := range granted {
		req.Granted = append(req.Granted, gr)
	}

	_, err := c.client.OnGrant(ctx, req)
	return err
}

func (c *granteeClient) health(ctx context.Context) error {
	req := proto.HealthRequest{}
	_, err := c.client.Health(ctx, &req)
	return err
}

func (c *granteeClient) bindPluginClient(pc *plugin.Client) {
	if c.pClient != nil {
		panic("trying to bind to two clients")
	}
	c.pClient = pc
}

func (c *granteeClient) shutdown(reason string) error {
	if c.pClient != nil {
		defer func() {
			c.pClient.Kill()
			c.pClient = nil
		}()
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), defDurationGracefulShutClient)
	defer cancelFn()

	req := proto.ShutdownRequest{Reason: reason}
	_, err := c.client.Shutdown(ctx, &req)
	return err
}
