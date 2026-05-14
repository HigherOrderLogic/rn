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

package ide

import (
	"context"
	"net"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi/browserrpc"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"google.golang.org/grpc"
	"unstable.build/go-tui/browser"
	tbrowserrpc "unstable.build/go-tui/browser/browserrpc"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
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
	mu        sync.Locker
	Handler   browserapi.Handler
	Component tui.Component
	close     func()
}

func (h *safeHandler) Resize(width, height int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Component.Resize(width, height)
}
func (h *safeHandler) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Component.Draw(w)
}
func (h *safeHandler) Handle(ev term.Event) (exit, handled bool) {
	h.mu.Lock()
	exit, handled = h.Handler.Handle(ev)
	// workaround search.List non-determinism
	if w, ok := h.Handler.(interface{ Wait() }); ok {
		w.Wait()
	}
	h.mu.Unlock()
	// addWorkspace runs Phase B/C asynchronously: a goroutine
	// builds the workspace and then schedules install via
	// scheduleNextTick. After the test has released h.mu the
	// install goroutine can acquire it and finish, so wait here for
	// any pending workspaces to install before the next test step.
	if drainer, ok := h.Handler.(interface{ drainPendingWorkspaces() }); ok {
		drainer.drainPendingWorkspaces()
	}
	// Drain any in-flight async save/reload completions started by
	// the just-processed event. The IDE locker is released above so
	// awaiter goroutines can complete their scheduled callbacks
	// (cfg.scheduleNextTick spawns a goroutine that re-acquires
	// h.mu before running cb).
	if drainer, ok := h.Handler.(interface{ waitInflight() }); ok {
		drainer.waitInflight()
	}
	if drainer, ok := h.Handler.(interface{ drainSched() }); ok {
		drainer.drainSched()
	}
	return
}
func (h *safeHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Cursor()
}

func (h *safeHandler) Selection() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Selection()
}

func (h *safeHandler) Close() error {
	if h.close != nil {
		h.close()
		return nil
	}
	return h.Handler.Close()
}

func newTestRPCBrowser(t *testing.T,
	destructor *func(),
	clip clipboard.Register,
	otherOpts ...text.Option,
) browserConstructor {
	return func(ed text.Editor, opts ...text.Option) (
		tui.Handler, browser.Browser, error,
	) {
		ex := new(ex)
		notifications := newWorkspaceNotifications(
			storagestub.NewInMemoryService(), notificationsConfig(),
			&workspaceManagerMock{workspace: ex})
		uri, err := workspaceapi.ParseURI("memory:///")
		require.NoError(t, err)
		svc := storagestub.NewInMemoryService()
		opts = append(opts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
		opts = append(opts, text.WithCommandKeyBinding(term.KeyComb{Ch: 'w', Mod: term.ModCtrl},
			[][]string{{"tabclose"}}))
		opts = append(opts, text.WithCommandKeyBinding(term.KeyComb{Ch: 'l', Mod: term.ModCtrl},
			[][]string{{"tabnext"}}))
		opts = append(opts, text.WithCommandKeyBinding(term.KeyComb{Ch: 'h', Mod: term.ModCtrl},
			[][]string{{"tabprevious"}}))
		opts = append(opts, otherOpts...)
		ex.syncCommandPrompt = true
		err = ex.init(ed, &testLoader{}, svc, notifications, uri,
			vte.DefaultConfig(), plugin.DefaultBarConfig(),
			nopPublishEvent, 0, clip, nil, nil, nil, nil, opts...)
		if err != nil {
			return nil, nil, err
		}
		ex.subscribeCommands()
		lis, err := net.Listen("tcp", ":0")
		require.NoError(t, err)

		var serverMutex sync.Mutex
		grpcServer := grpc.NewServer()
		server := tbrowserrpc.NewServer(ex.Browser(), &serverMutex)
		server.SetSyncMode()
		browserrpc.RegisterWindowManagerServer(grpcServer, server)
		browserrpc.RegisterNotificationsServer(grpcServer, server)
		browserrpc.RegisterResourceOpenerServer(grpcServer, server)

		go grpcServer.Serve(lis)

		conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
		require.NoError(t, err)

		bc := browserrpc.NewClient(context.Background(), conn)
		var closeOnce sync.Once
		close := func() {
			closeOnce.Do(func() {
				serverMutex.Lock()
				defer serverMutex.Unlock()
				bc.Close()
				server.Stop()
				grpcServer.Stop()
				ex.Close()
			})
		}
		h := &safeHandler{Component: ex, Handler: ex, mu: &serverMutex, close: close}
		*destructor = close
		return h, browsertest.BrowserFromAPIBrowser(bc), nil
	}
}

func TestIntegrationRPCBrowserDraw(t *testing.T) {
	var destructor func()
	constructor := newTestRPCBrowser(t, &destructor, clipboard.NewInMemory())
	testBrowserHandlerDraw(t, constructor)
	destructor()
}

func TestIntegrationCopyToClipboard(t *testing.T) {
	var destructor func()
	clip := clipboard.NewInMemory()
	constructor := newTestRPCBrowser(t, &destructor, clip,
		text.WithCommandKeyBinding(
			term.KeyComb{Ch: 'h', Mod: term.ModCtrl}, [][]string{{cmdClipboardPaste}}),
		text.WithFloatingNoMaxSize(false),
	)
	testCopyToClipboard(t, clip, constructor)
	destructor()
}

func TestRPCBrowserCloseLeak(t *testing.T) {
	var destructor func()
	_, b, err := newTestRPCBrowser(t,
		&destructor, clipboard.NewInMemory())(
		texttest.NopEditor())
	require.NoError(t, err)
	defer destructor()

	focus, err := b.Focus()
	require.NoError(t, err)

	win, err := b.Split(browserapi.OrientationLeft, focus, browsertest.NewTestHandler())
	require.NoError(t, err)

	require.NoError(t, win.Close())
}
