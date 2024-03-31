package ide

import (
	"context"
	"net"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/blue/document"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	browserpb "unstable.build/go-tui/browser/rpc"
	browsertest "unstable.build/go-tui/browser/test"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	texttest "unstable.build/go-tui/text/test"
)

const testingShutdownWait = 500 * time.Millisecond

func nopPublishEvent(term.Event) bool {
	return true
}

type groupEventHandler struct {
	h  *browsertest.TestHandler
	wg *sync.WaitGroup
}

func (h *groupEventHandler) Handle(ev term.Event) (handled bool) {
	defer h.wg.Done()
	h.h.Handle(ev)
	return
}

// used to emulate term event loop synchronization
type safeHandler struct {
	mu      *sync.Mutex
	Handler *ex
}

func (h *safeHandler) Resize(width, height int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Resize(width, height)
}
func (h *safeHandler) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Draw(w)
}
func (h *safeHandler) Handle(ev term.Event) (exit, handled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	exit, handled = h.Handler.Handle(ev)
	// workaround search.List non-determinism
	h.Handler.Wait()
	return
}
func (h *safeHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Cursor()
}
func (h *safeHandler) Man() tui.Manual {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Man()
}

func (h *safeHandler) Close() error {
	return h.Handler.Close()
}

func newTestRPCBrowser(t *testing.T,
	destructor *func(),
) browserConstructor {
	return func(ed text.Editor, opts ...text.Option) (
		tui.Handler, browser.Browser, error,
	) {
		opts = append(opts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
		opts = append(opts, text.WithCommandKeyBinding(term.KeyComb{Key: term.KeyCtrlW}, []string{"closeTab"}))
		opts = append(opts, text.WithCommandKeyBinding(term.KeyComb{Key: term.KeyCtrlL}, []string{"nextTab"}))
		opts = append(opts, text.WithCommandKeyBinding(term.KeyComb{Key: term.KeyCtrlH}, []string{"previousTab"}))
		ex := new(ex)
		err := ex.init(ed, &testLoader{}, document.NewInMemoryService(),
			vte.DefaultConfig(), nopPublishEvent, opts...)
		if err != nil {
			return nil, nil, err
		}
		ex.subscribeCommands()
		lis, err := net.Listen("tcp", ":0")
		require.NoError(t, err)

		broker := proto.NewUnixGRPCBroker("", "", "")

		var serverMutex sync.Mutex
		grpcServer := grpc.NewServer()
		server := browserpb.NewServer(broker, ex.Browser(), &serverMutex)
		server.SetSyncMode()
		browserpb.RegisterWindowManagerServer(grpcServer, server)
		browserpb.RegisterNotificationsServer(grpcServer, server)
		browserpb.RegisterResourceOpenerServer(grpcServer, server)

		go grpcServer.Serve(lis)

		conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
		require.NoError(t, err)

		bc := browserpb.NewClient(context.Background(), broker, conn)
		h := &safeHandler{Handler: ex, mu: &serverMutex}
		*destructor = func() {
			serverMutex.Lock()
			defer serverMutex.Unlock()
			bc.Close()
			server.Stop()
			grpcServer.Stop()
			broker.Close()
			ex.Close()
		}
		return h, browsertest.BrowserFromAPIBrowser(bc), nil
	}
}

func TestIntegrationRPCBrowserDraw(t *testing.T) {
	var destructor func()
	constructor := newTestRPCBrowser(t, &destructor)
	testBrowserHandlerDraw(t, constructor)
	destructor()
}

func TestRPCBrowserCloseLeak(t *testing.T) {
	uri, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	var destructor func()
	_, b, err := newTestRPCBrowser(t, &destructor)(texttest.NopEditor(), text.WithFile(uri))
	require.NoError(t, err)
	defer destructor()

	focus, err := b.Focus()
	require.NoError(t, err)

	win, err := b.Split(browserapi.OrientationLeft, focus, browsertest.NewTestHandler())
	require.NoError(t, err)

	require.NoError(t, win.Close())
}

func TestMain(m *testing.M) {
	log.SetLevel(log.ErrorLevel)
	goleak.VerifyTestMain(m)
}
