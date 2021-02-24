package editor

import (
	"fmt"
	"net"
	_ "net/http/pprof"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
)

const testingShutdownWait = 500 * time.Millisecond

type groupEventHandler struct {
	h  *browser.TestHandler
	wg *sync.WaitGroup
}

func (h *groupEventHandler) Handle(ev term.Event) (handled bool) {
	defer h.wg.Done()
	h.h.Handle(ev)
	return
}

// used to emulate term event loop synchronization
type safeHandler struct {
	mu *sync.Mutex
	tui.Handler
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
	return h.Handler.Handle(ev)
}
func (h *safeHandler) Cursor() (pos term.Coordinates, show bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Cursor()
}
func (h *safeHandler) Man() tui.Manual {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Man()
}

func newTestRPCBrowser(t *testing.T,
	destructor *func(),
) browserConstructor {
	return func(ed Editor, opts ...Option) (tui.Handler, browser.Browser, error) {
		var logger *log.Logger
		// // uncomment to debug
		// logger = log.New()
		// logger.SetLevel(log.TraceLevel)

		b := newTestBrowserHandler()
		err := b.Init(ed, opts...)
		if err != nil {
			return nil, nil, err
		}

		lis, err := net.Listen("tcp", ":0")
		require.NoError(t, err)

		broker := proto.NewDialBroker()

		var serverMutex sync.Mutex
		grpcServer := grpc.NewServer()
		server := browser.NewServer(broker, b.Browser(), &serverMutex)
		server.Logger = logger
		proto.RegisterWindowManagerServer(grpcServer, server)
		proto.RegisterMessengerServer(grpcServer, server)
		proto.RegisterKeyMapperServer(grpcServer, server)
		proto.RegisterResourceOpenerServer(grpcServer, server)
		proto.RegisterEventSubscriberServer(grpcServer, server)

		go grpcServer.Serve(lis)

		conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
		require.NoError(t, err)

		bc := browser.NewClient(broker, conn)
		bc.Logger = logger
		h := &safeHandler{Handler: b, mu: &serverMutex}
		*destructor = func() {
			bc.Close()
			server.Close()
			grpcServer.Stop()
			broker.Close()
		}
		return h, bc, nil
	}
}

func TestIntegrationRPCBrowserDraw(t *testing.T) {
	var destructor func()
	constructor := newTestRPCBrowser(t, &destructor)
	testBrowserHandlerDraw(t, constructor)
	destructor()
}

func TestRPCBrowserCloseLeak(t *testing.T) {
	var destructor func()
	_, b, err := newTestRPCBrowser(t, &destructor)(&testEditor{}, WithFilepath(""))
	require.NoError(t, err)
	defer destructor()

	win, err := b.SplitVerticalLeft(browser.NewTestHandler())
	require.NoError(t, err)

	require.NoError(t, win.Close())

	// NOTE: to reason about window/handler resource leaks
	// uncomment next line and analyze running goroutines
	// goleak.VerifyNone(t)
}

// NOTE: run go test -race in order for this test to be useful.
func TestClientSynchronizeHandlers(t *testing.T) {
	var destructor func()
	h, b, err := newTestRPCBrowser(t, &destructor)(&testEditor{}, WithFilepath(""))
	require.NoError(t, err)
	defer destructor()
	var wg sync.WaitGroup

	h1 := browser.TestHandler{}

	subs := []term.Event{
		{Type: term.EventKey, Ch: 'A'},
		{Type: term.EventKey, Ch: 'J'},
		{Type: term.EventKey, Ch: 'H'},
		{Type: term.EventKey, Ch: 'B'},
	}

	for _, ev := range subs {
		h := &groupEventHandler{wg: &wg, h: &h1}
		err = b.Subscribe(ev, h)
		require.NoError(t, err)
	}

	for _, ev := range subs {
		wg.Add(1)
		_, handled := h.Handle(ev)
		assert.True(t, handled)
	}

	wg.Wait()
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if exitCode == 0 {
		ignoreOpenCensus := goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start")
		// this is to give time to server to close resources
		time.Sleep(testingShutdownWait)
		err := goleak.Find(ignoreOpenCensus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goleak: Leaks on successful test run: %v\n", err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
