package plugin

import (
	"context"
	"errors"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testGranteePbClient struct {
	mu                 sync.Mutex
	err                error
	fixturePermissions []*proto.Permission
	sleepPermissions   time.Duration
	healthChan         chan struct{}
	onShutdownChan     chan struct{}

	_permissions *proto.PermRequest
	_onGrant     *proto.OnPermGrantRequest
	_shutdown    *proto.ShutdownRequest
	_health      *proto.HealthRequest
}

func (c *testGranteePbClient) permissions() (proto.PermRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._permissions == nil {
		return proto.PermRequest{}, false
	}
	return *c._permissions, true
}
func (c *testGranteePbClient) onGrant() (proto.OnPermGrantRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._onGrant == nil {
		return proto.OnPermGrantRequest{}, false
	}
	return *c._onGrant, true
}
func (c *testGranteePbClient) shutdown() (proto.ShutdownRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._shutdown == nil {
		return proto.ShutdownRequest{}, false

	}
	return *c._shutdown, true
}
func (c *testGranteePbClient) health() (proto.HealthRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c._health == nil {
		return proto.HealthRequest{}, false

	}
	return *c._health, true
}

func (c *testGranteePbClient) Permissions(
	ctx context.Context, in *proto.PermRequest, opts ...grpc.CallOption,
) (*proto.PermResponse, error) {
	c.mu.Lock()
	c._permissions = in
	c.mu.Unlock()
	permissions := new(proto.PermResponse)
	if c.err != nil {
		return nil, c.err
	}
	permissions.Perms = c.fixturePermissions
	time.Sleep(c.sleepPermissions)
	return permissions, nil
}

func (c *testGranteePbClient) OnGrant(
	ctx context.Context, in *proto.OnPermGrantRequest, opts ...grpc.CallOption,
) (*proto.OnPermGrantResponse, error) {
	c.mu.Lock()
	c._onGrant = in
	c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	return new(proto.OnPermGrantResponse), nil
}

func (c *testGranteePbClient) Shutdown(
	ctx context.Context, in *proto.ShutdownRequest, opts ...grpc.CallOption,
) (*proto.ShutdownResponse, error) {
	c.mu.Lock()
	c._shutdown = in
	c.mu.Unlock()
	if c.onShutdownChan != nil {
		go func(ch chan struct{}) {
			ch <- struct{}{}
		}(c.onShutdownChan)
	}
	if c.err != nil {
		return nil, c.err
	}
	return new(proto.ShutdownResponse), nil
}

func (c *testGranteePbClient) Health(
	ctx context.Context, in *proto.HealthRequest, opts ...grpc.CallOption,
) (*proto.HealthResponse, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.healthChan != nil {
		// wait for test harness signal to respond
		_, ok := <-c.healthChan
		// if not closed, then hold
		if ok {
			c._health = in
		}
	}
	return new(proto.HealthResponse), nil
}

func TestUnitClient(t *testing.T) {
	t.Run("permissions sends permissions", func(t *testing.T) {
		mockpbClient := &testGranteePbClient{}
		mockpbClient.fixturePermissions =
			[]*proto.Permission{&proto.Permission{Id: "ballz"}}

		client := newGranteeClient(nil, mockpbClient)

		perms, err := client.permissions(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, mockpbClient.fixturePermissions, perms)
	})

	t.Run("permissions bubbles up error", func(t *testing.T) {
		myErr := errors.New("Hubble was perfect")
		mockpbClient := &testGranteePbClient{err: myErr}
		client := newGranteeClient(nil, mockpbClient)

		_, err := client.permissions(context.Background(), nil)
		require.Equal(t, myErr, err)
	})

	t.Run("sendGrants sends grants", func(t *testing.T) {
		mockpbClient := &testGranteePbClient{}
		client := newGranteeClient(nil, mockpbClient)

		granted := []*proto.PermissionGrant{&proto.PermissionGrant{Id: "shits", GrantId: uint32(1234)}}
		denied := []*proto.Permission{&proto.Permission{Id: "poops"}}

		err := client.sendGrants(context.Background(), denied, granted)
		require.NoError(t, err)

		onGrant, ok := mockpbClient.onGrant()
		require.True(t, ok)
		assert.Equal(t, onGrant.GetDenied(), denied)
		assert.Equal(t, onGrant.GetGranted(), granted)
	})

	t.Run("sendGrants bubbles up error", func(t *testing.T) {
		myErr := errors.New("Hubble was perfect")
		mockpbClient := &testGranteePbClient{err: myErr}
		client := newGranteeClient(nil, mockpbClient)

		err := client.sendGrants(context.Background(), nil, nil)
		require.Equal(t, myErr, err)
	})

	t.Run("health send a health request", func(t *testing.T) {
		mockpbClient := &testGranteePbClient{}
		client := newGranteeClient(nil, mockpbClient)

		err := client.health(context.Background())
		require.NoError(t, err)
		require.NotNil(t, mockpbClient.health)
	})

	t.Run("health bubbles up error", func(t *testing.T) {
		myErr := errors.New("Atza is perfect")
		mockpbClient := &testGranteePbClient{err: myErr}
		client := newGranteeClient(nil, mockpbClient)

		err := client.health(context.Background())
		require.Equal(t, myErr, err)
	})

	t.Run("Close send a shutdown request", func(t *testing.T) {
		mockpbClient := &testGranteePbClient{}
		client := newGranteeClient(nil, mockpbClient)

		err := client.shutdown("")
		require.NoError(t, err)
		require.NotNil(t, mockpbClient.shutdown)
	})

	t.Run("Close bubbles up shutdown error", func(t *testing.T) {
		myErr := errors.New("Atza is perfect")
		mockpbClient := &testGranteePbClient{err: myErr}
		client := newGranteeClient(nil, mockpbClient)

		err := client.shutdown("")
		require.Equal(t, myErr, err)
	})
}

type granteeMock struct {
	err                  error
	onConnected          int
	cfgs                 []Config
	onGrant, onDenied    []Permission
	onHealth, onShutdown bool
}

func (g *granteeMock) Connected(b proto.MuxBroker, cfg Config) {
	g.cfgs = append(g.cfgs, cfg)
	g.onConnected++
}
func (g *granteeMock) PermissionGranted(grants []Grant) {
	for _, grant := range grants {
		g.onGrant = append(g.onGrant, grant.Permission)
	}
}
func (g *granteeMock) PermissionDenied(perms []Permission) {
	g.onDenied = append(g.onDenied, perms...)
}
func (g *granteeMock) Shutdown(reason string) error {
	if g.err != nil {
		return g.err
	}
	if g.onShutdown {
		return errors.New("called shutdown twice")
	}
	g.onShutdown = true
	return nil
}
func (g *granteeMock) Health() error {
	if g.err != nil {
		return g.err
	}
	g.onHealth = true
	return nil
}

func setupIntTest(
	t *testing.T, granteeMock Grantee, perms []Permission,
) (client *granteeClient, closeFn func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	server := newGranteeServer(nil, granteeMock, perms, time.Duration(0))
	server.(*granteeServer).osExit = nil
	proto.RegisterGranteeServer(grpcServer, server)

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	client = newGranteeClient(nil, proto.NewGranteeClient(conn))
	closeFn = func() {
		client.shutdown("test harness")
		grpcServer.Stop()
	}
	return
}

func TestIntegrationPluginClientServer(t *testing.T) {
	t.Run("permissions request triggers Grantee OnConnected", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("write"), Permission("read")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		protoPerms, err := client.permissions(context.Background(), nil)
		require.NoError(t, err)

		assert.Equal(t, []*proto.Permission{
			&proto.Permission{Id: "write"}, &proto.Permission{Id: "read"},
		}, protoPerms)
		assert.Equal(t, 1, grantee.onConnected)
	})

	t.Run("permissions request plugin config passes onto grantee", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("wasup")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		in := map[string]interface{}{
			"viz":    true,
			"hubble": 1,
			"sonicd": "more",
			"longboard": map[string]interface{}{
				"raven": math.MaxFloat64,
			},
		}
		_, err := client.permissions(context.Background(), MapConfig(in))
		require.NoError(t, err)

		require.Len(t, grantee.cfgs, 1)

		out := grantee.cfgs[0]
		viz, err := out.GetBool("viz")
		assert.NoError(t, err)
		assert.True(t, viz)

		sonicd, err := out.GetString("sonicd")
		assert.NoError(t, err)
		assert.Equal(t, "more", sonicd)

		longboard, err := out.GetConfig("longboard")
		require.NoError(t, err)

		raven, err := longboard.GetFloat("raven")
		assert.NoError(t, err)
		assert.Equal(t, math.MaxFloat64, raven)

		hubble, err := out.GetInt("hubble")
		assert.NoError(t, err)
		assert.Equal(t, 1, hubble)
	})

	t.Run("permissions request twice does not trigger Grantee OnConnected twice", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("append")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		_, err := client.permissions(context.Background(), nil)
		require.NoError(t, err)
		_, err = client.permissions(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, 1, grantee.onConnected)
	})

	t.Run("sendGrants request triggers Grantee OnPermissionGranted/Denied", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("append"), Permission("read")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		denied := []*proto.Permission{&proto.Permission{Id: "append"}}
		granted := []*proto.PermissionGrant{&proto.PermissionGrant{Id: "read", GrantId: 1}}
		err := client.sendGrants(context.Background(), denied, granted)
		require.NoError(t, err)

		assert.Equal(t, []Permission{Permission("read")}, grantee.onGrant)
		assert.Equal(t, []Permission{Permission("append")}, grantee.onDenied)
	})

	t.Run("sendGrants request DOES NOT trigger Grantee OnPermissionGranted/Denied for permissions that were not requested", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("write")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		denied := []*proto.Permission{&proto.Permission{Id: "garbage"}}
		granted := []*proto.PermissionGrant{
			&proto.PermissionGrant{Id: "write", GrantId: 1},
			&proto.PermissionGrant{Id: "trash", GrantId: 2},
		}
		err := client.sendGrants(context.Background(), denied, granted)
		require.NoError(t, err)

		assert.Equal(t, []Permission{Permission("write")}, grantee.onGrant)
		assert.Equal(t, []Permission(nil), grantee.onDenied)
	})

	t.Run("health request triggers Grantee Health", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("append")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		err := client.health(context.Background())
		require.NoError(t, err)

		assert.True(t, grantee.onHealth)
	})

	t.Run("health path bubbles up error returned by Grantee", func(t *testing.T) {
		myErr := errors.New("oopsie daisy")
		grantee := granteeMock{err: myErr}
		perms := []Permission{Permission("append")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		err := client.health(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "oopsie daisy")
	})

	t.Run("Close request triggers Grantee OnShutdown", func(t *testing.T) {
		grantee := granteeMock{}
		perms := []Permission{Permission("append")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		err := client.shutdown("sut")
		require.NoError(t, err)

		assert.True(t, grantee.onShutdown)
	})

	t.Run("Close path bubbles up error returned by Grantee", func(t *testing.T) {
		myErr := errors.New("oh bollocks")
		grantee := granteeMock{err: myErr}
		perms := []Permission{Permission("append")}
		client, closeFn := setupIntTest(t, &grantee, perms)
		defer closeFn()

		err := client.shutdown("sut")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "oh bollocks")
	})
}
