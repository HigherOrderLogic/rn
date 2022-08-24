package plugin

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui/proto"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type nopBroker struct {
	mu      sync.Mutex
	nextID  uint32
	_closed bool
}

func (b *nopBroker) Cleanup(uint32) error {
	return nil
}

func (b *nopBroker) NextId() uint32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	return b.nextID
}

func (b *nopBroker) Accept(ID uint32) (net.Listener, error) {
	return nil, nil
}

func (b *nopBroker) AcceptAndServe(
	ID uint32, srv func(opts []grpc.ServerOption) proto.MuxServer,
) {
}

func (b *nopBroker) Dial(ID uint32) (
	conn proto.MuxConn, err error,
) {
	return nil, nil
}

func (b *nopBroker) closed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b._closed
}

func (b *nopBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b._closed {
		panic("called Close multiple times")
	}
	b._closed = true
	return nil
}

func newTestManager(grantor Grantor, opts ...Option) (*Manager, *testGranteePbClient, *nopBroker) {
	m := new(Manager)

	opts = append([]Option{
		WithHandshakeTimeout(500 * time.Millisecond),
		WithHealthTimeout(500 * time.Millisecond),
		WithHealthRetries(0),
	}, opts...)
	mockpb := &testGranteePbClient{
		healthChan: make(chan struct{}),
	}

	m.builder = func(pluginID, path string, grantor Grantor, logger *log.Logger) (*granteeClient, error) {
		return newGranteeClient(m.broker, mockpb), nil
	}
	m.Init(grantor, opts...)
	m.broker = &nopBroker{}

	return m, mockpb, m.broker.(*nopBroker)
}

func testRunAndWait(t *testing.T, mgr *Manager, pbClient *testGranteePbClient) {
	cfg := mapConfig(make(map[string]interface{}))
	err := mgr.Run("myId", "/here/is/my/plugin", cfg)
	require.NoError(t, err)

	pbClient.healthChan <- struct{}{}
	close(pbClient.healthChan) // only test once
}

func assertShutdown(t *testing.T, pbClient *testGranteePbClient, broker *nopBroker) string {
	<-pbClient.onShutdownChan
	sht, ok := pbClient.shutdown()
	assert.True(t, ok)
	return sht.Reason
}

func TestManagerRun(t *testing.T) {
	t.Run("should start plugin and proceed to handshake", func(t *testing.T) {
		grantor := mockGrantor{}
		mgr, pbClient, _ := newTestManager(&grantor)
		defer mgr.Close()

		pbClient.fixturePermissions = []*proto.Permission{{Id: "read"}, {Id: "read"}}
		testRunAndWait(t, mgr, pbClient)

		assert.NotNil(t, pbClient.permissions)
		require.NotNil(t, pbClient.onGrant)

		grant, ok := pbClient.onGrant()
		require.True(t, ok)

		assert.Len(t, grant.Denied, 0)
		require.Len(t, grant.Granted, 1)

		expected := []*proto.PermissionGrant{
			{Id: "read", GrantId: 1},
		}
		assert.Equal(t, expected, grant.Granted)

		srvs := grantor.servers()
		require.Len(t, srvs, 1)
		require.Len(t, srvs[0].served(), 1)
		assert.Equal(t, uint32(1), srvs[0].served()[0])

		// verify that shutdown was not called
		_, ok = pbClient.shutdown()
		require.False(t, ok)

		stat, ok := mgr.Stat("myId")
		require.True(t, ok)
		require.NotZero(t, stat.ActiveAt)
		require.Len(t, stat.Errors, 0)

		stats := mgr.Stats()
		status, ok := stats["myId"]
		require.True(t, ok)
		require.NotZero(t, status.ActiveAt)
		require.Len(t, status.Errors, 0)
	})

	t.Run("Stop should call a plugin's shutdown", func(t *testing.T) {
		mgr, pbClient, broker := newTestManager(&mockGrantor{})
		defer mgr.Close()

		pbClient.onShutdownChan = make(chan struct{})

		testRunAndWait(t, mgr, pbClient)
		require.NoError(t, mgr.Stop("myId"))

		assertShutdown(t, pbClient, broker)

		// should return error because it had been already stopped
		require.Error(t, mgr.Stop("myId"))
	})

	t.Run("double start plugin with same ID should error", func(t *testing.T) {
		mgr, _, _ := newTestManager(&mockGrantor{})
		defer mgr.Close()

		err := mgr.Run("red", "", nil)
		require.NoError(t, err)

		err = mgr.Run("red", "", nil)
		require.Error(t, err)
	})

	t.Run("should shutdown plugin if errors upon call to get permissions", func(t *testing.T) {
		mgr, pbClient, broker := newTestManager(&mockGrantor{})
		defer mgr.Close()

		pbClient.fixturePermissions =
			[]*proto.Permission{{Id: "read"}}
		pbClient.err = errors.New("woopsie")
		pbClient.onShutdownChan = make(chan struct{})

		err := mgr.Run("blue", "/here/is/my/plugin", nil)
		require.NoError(t, err)

		reason := assertShutdown(t, pbClient, broker)
		assert.Contains(t, reason, "woopsie")
	})

	t.Run("should shutdown plugin if fails to respond to handshake in time", func(t *testing.T) {
		mgr, pbClient, broker := newTestManager(&mockGrantor{})
		defer mgr.Close()

		pbClient.fixturePermissions =
			[]*proto.Permission{{Id: "read"}}
		pbClient.sleepPermissions = 2 * time.Second
		pbClient.onShutdownChan = make(chan struct{})

		err := mgr.Run("blue", "/here/is/my/plugin", nil)
		require.NoError(t, err)

		reason := assertShutdown(t, pbClient, broker)
		assert.Contains(t, reason, "deadline exceeded")
	})

	t.Run("should shutdown plugin if fails to respond to first health requests in time", func(t *testing.T) {
		mgr, pbClient, broker := newTestManager(&mockGrantor{})
		defer mgr.Close()

		pbClient.fixturePermissions =
			[]*proto.Permission{{Id: "read"}}
		pbClient.onShutdownChan = make(chan struct{})

		err := mgr.Run("green", "/here/is/my/plugin", nil)
		require.NoError(t, err)

		time.Sleep(mgr.config.handshakeTimeout + 50*time.Millisecond)

		reason := assertShutdown(t, pbClient, broker)
		assert.Contains(t, reason, "health check")
	})

	t.Run("should shutdown plugin if fails to respond to subsequent health checks", func(t *testing.T) {
		mgr, pbClient, broker := newTestManager(&mockGrantor{},
			WithHealthTimeout(250*time.Millisecond), WithHealthRetries(3))
		defer mgr.Close()

		pbClient.fixturePermissions =
			[]*proto.Permission{{Id: "read"}}
		pbClient.onShutdownChan = make(chan struct{})

		err := mgr.Run("yellow", "/here/is/my/plugin", nil)
		require.NoError(t, err)

		// respond 2 times correctly
		pbClient.healthChan <- struct{}{}
		pbClient.healthChan <- struct{}{}
		defer close(pbClient.healthChan)

		// exhaust retries
		sleepyTime := 250*3*time.Millisecond + (50 * time.Millisecond)
		time.Sleep(sleepyTime)

		reason := assertShutdown(t, pbClient, broker)
		assert.Contains(t, reason, "exhausted health check retries")

		// check errors in status
		stat, ok := mgr.Stat("yellow")
		assert.True(t, ok)
		require.Len(t, stat.Errors, mgr.config.healthRetries+1, mgr.clients)
		assert.Contains(t, stat.Errors[0].Error(), "deadline")
	})

	t.Run("should wait for plugin Shutdown before returning from a call to Close", func(t *testing.T) {
		mgr, pbClient, broker := newTestManager(&mockGrantor{})

		pbClient.fixturePermissions = []*proto.Permission{{Id: "read"}}
		pbClient.onShutdownChan = make(chan struct{})

		pluginIDs := []string{"green", "blue"}
		for _, id := range pluginIDs {
			err := mgr.Run(id, "/here/is/my/plugin", nil)
			require.NoError(t, err)
		}

		time.Sleep(mgr.config.handshakeTimeout + 50*time.Millisecond)

		var wg sync.WaitGroup
		wg.Add(len(pluginIDs))
		go func() {
			for range pluginIDs {
				<-pbClient.onShutdownChan
				_, ok := pbClient.shutdown()
				assert.True(t, ok)
				wg.Done()
			}
		}()
		wg.Wait()

		require.NoError(t, mgr.Close())
		assert.True(t, broker.closed())
	})
}
