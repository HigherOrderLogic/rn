package plugin

import (
	"sync"

	bproto "github.com/ernestrc/blue/rpc"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	// PermissionBrowserWindowManager requests access to a browser's window manager.
	PermissionBrowserWindowManager Permission = "_PermBrowserWindowManager"
	// PermissionBrowserKeyMapper requests access to a browser's key mapper.
	PermissionBrowserKeyMapper = "_PermBrowserKeyMapper"
	// PermissionBrowserResourceOpener requests access to open new files.
	PermissionBrowserResourceOpener = "_PermBrowserResourceOpener"
	// PermissionBrowserMessenger requests access to send messages to the UI.
	PermissionBrowserMessenger = "_PermBrowserMessenger"
	// PermissionBrowserEventSubscriber requests access subscribe to term events.
	PermissionBrowserEventSubscriber = "_PermBrowserEventSubscriber"
	// PermissionBrowserEventPublisher requests access to publish term events.
	// This is useful if your plugin handler does async updates to its state, as
	// it enables interrupting the main event loop to redraw components.
	PermissionBrowserEventPublisher = "_PermBrowserEventPublisher"
	// PermissionBrowserStorage requests access to storage.
	PermissionBrowserStorage = "_PermBrowserStorage"
)

type browserResourceServer struct {
	mu  sync.Mutex
	b   browser.Browser
	srv proto.MuxServer
}

func newBrowserResourceServer(b browser.Browser) *browserResourceServer {
	ret := new(browserResourceServer)
	ret.b = b
	return ret
}

func (s *browserResourceServer) Serve(
	pluginID string, grantID uint32, broker proto.MuxBroker,
	l *log.Logger, lock sync.Locker,
) {
	broker.AcceptAndServe(grantID, func(opts []grpc.ServerOption) proto.MuxServer {
		s.mu.Lock()
		defer s.mu.Unlock()
		// NOTE: unfortunately all plugins must share the same browser.Server,
		// because it's internal state is not shared.
		if s.srv == nil {
			var srv proto.MuxServer
			if l != nil && l.IsLevelEnabled(log.TraceLevel) {
				srv = proto.LoggingGRPCServer(l, opts...)
			} else {
				srv = proto.GRPCServer(opts...)
			}
			grpc := srv.GRPC()
			s.srv = srv
			server := new(browser.Server)
			server.Init(broker, s.b, lock, interruptWindowServer)
			server.Logger = l
			rpcServer := interruptBrowserServer(server, term.Interrupt)
			proto.RegisterWindowManagerServer(grpc, rpcServer)
			proto.RegisterKeyMapperServer(grpc, rpcServer)
			proto.RegisterResourceOpenerServer(grpc, rpcServer)
			proto.RegisterMessengerServer(grpc, rpcServer)
			proto.RegisterEventSubscriberServer(grpc, rpcServer)
			proto.RegisterEventPublisherServer(grpc, rpcServer)
			bproto.RegisterDocumentStoreServer(grpc, rpcServer)
		}
		return s.srv
	})
}

// BrowserResources returns a map of Permission to a ResourceServer
// capable of serving each of the b Browser's resources.
func BrowserResources(b browser.Browser) map[Permission]ResourceServer {
	s := newBrowserResourceServer(b)
	return map[Permission]ResourceServer{
		PermissionBrowserWindowManager:   s,
		PermissionBrowserKeyMapper:       s,
		PermissionBrowserResourceOpener:  s,
		PermissionBrowserMessenger:       s,
		PermissionBrowserEventSubscriber: s,
		PermissionBrowserEventPublisher:  s,
		PermissionBrowserStorage:         s,
	}
}

func dialBrowser(token uint32, broker proto.MuxBroker) (
	browser.Browser, error,
) {
	if c, ok := clients.Load(token); ok {
		return c.(browser.Browser), nil
	}
	conn, err := broker.Dial(token)
	if err != nil {
		return nil, err
	}
	c := browser.NewClient(broker, conn)
	c.Logger = &pluginLogger
	clients.Store(token, c)
	return c, nil
}

// WindowManager acquires the browser's WindowManager
// resource with the given token.
func WindowManager(token uint32, broker proto.MuxBroker) (
	browser.WindowManager, error,
) {
	return dialBrowser(token, broker)
}

// KeyMapper acquires the browser's KeyMapper
// resource with the given token.
func KeyMapper(token uint32, broker proto.MuxBroker) (
	browser.KeyMapper, error,
) {
	return dialBrowser(token, broker)
}

// ResourceOpener acquires the browser's ResourceOpener
// resource with the given token.
func ResourceOpener(token uint32, broker proto.MuxBroker) (
	browser.ResourceOpener, error,
) {
	return dialBrowser(token, broker)
}

// Messenger acquires the browser's Messenger
// resource with the given token.
func Messenger(token uint32, broker proto.MuxBroker) (
	browser.Messenger, error,
) {
	return dialBrowser(token, broker)
}

// EventSubscriber acquires the browser's EventSubscriber
// resource with the given token.
func EventSubscriber(token uint32, broker proto.MuxBroker) (
	browser.EventSubscriber, error,
) {
	return dialBrowser(token, broker)
}

// EventPublisher acquires the browser's EventPublisher
// resource with the given token.
func EventPublisher(token uint32, broker proto.MuxBroker) (
	browser.EventPublisher, error,
) {
	return dialBrowser(token, broker)
}

// Storage acquires the browser's Storage
// resource with the given token.
func Storage(token uint32, broker proto.MuxBroker) (
	browser.Storage, error,
) {
	return dialBrowser(token, broker)
}
