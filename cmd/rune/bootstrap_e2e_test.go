// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/handler/handlertest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"

	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/term/gui"
	"unstable.build/go-tui/text"
)

// TestBootstrapE2ESurfacesOAuthURLInWaitPrompt is the black-box e2e
// test for the bootstrap login flow: it constructs the bootstrap
// handler the way runGUI does, drives it through the Welcome →
// Vim-mode → Sign-in prompts with real term.Events, and
// asserts that once the apiclient publishes the OAuth URL on its
// LoginSession.URL channel, that URL ends up rendered inside the
// "Follow the instructions in your browser" wait prompt.
//
// The contract under test is the full plumbing:
//
//	bootstrapHandler.startLogin
//	  → apiclient.Client.Login(ctx)
//	    → context-stashed urlCh
//	      → tokenSourceRefresh publishes the URL into ctx's chan
//	        → bootstrap_handler URL watcher schedules mountLoginWaitPrompt
//	          → preIDE.Prompt renders the URL on screen
//
// If any link in that chain breaks (the ctx key, the channel send,
// the scheduleNextTick wiring, or the wait-prompt mount), the
// rendered frame will not contain the URL and this test fails.
func TestBootstrapE2ESurfacesOAuthURLInWaitPrompt(t *testing.T) {
	// Fake ox-api endpoint: every request 404s so the auth config
	// fetch falls back to the builtin defaults and the OAuth flow
	// reaches the OpenBrowser callback with a synthesized URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dataDir := t.TempDir()
	configPath := dataDir + "/config.yaml"

	restoreFlags := overrideBootstrapFlags(t, bootstrapFlagOverrides{
		httpAddress:    srv.URL,
		dataPath:       dataDir,
		configPath:     configPath,
		websiteAddress: "https://rune.test",
	})
	t.Cleanup(restoreFlags)

	browserURLCh := make(chan *url.URL, 1)
	openBrowser := func(u *url.URL) error {
		select {
		case browserURLCh <- u:
		default:
		}
		return nil
	}

	mu := new(sync.Mutex)
	publishEvent, stopPump := newBootstrapPublishPump(mu)

	checkoutURL, signupURL := mustResolveBootstrapURLs("https://rune.test")
	root, err := newBootstrapHandler(
		dataDir, configPath, "" /* workspace */, "" /* zdotDir */, nil, /* filenames */
		nil /* launchCmd */, ide.FuncExtensionsRunner(testE2EExtensionsRunner),
		mu, publishEvent,
		checkoutURL, signupURL,
		openBrowser, clipboard.NewInMemory(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	t.Cleanup(stopPump)

	const width, height = 80, 30
	wrapped := &bootstrapE2ELocked{Handler: root, mu: mu}
	wrapped.Resize(width, height)

	// Welcome → Vim-mode → Sign-in. Each key matches the per-prompt
	// binding tables in bootstrap_handler.go. The brief pauses give
	// the publish-channel pumper a chance to drain the scheduled-tick
	// callbacks that mount each successor prompt before the next key
	// arrives.
	for _, ch := range []rune{'g', 'v', 'l'} {
		wrapped.Handle(term.Event{Type: term.EventKey, Ch: ch})
		time.Sleep(50 * time.Millisecond)
	}

	var oauthURL *url.URL
	select {
	case oauthURL = <-browserURLCh:
		require.NotEmpty(t, oauthURL.String(),
			"OpenBrowser hook must receive a non-empty URL")
	case <-time.After(10 * time.Second):
		t.Fatal("OpenBrowser hook was never invoked; OAuth flow stalled before publishing the URL")
	}

	// The bootstrap wait prompt embeds the URL in a backtick-quoted
	// span that wraps at the prompt's width budget, so the exact
	// string may have soft-wraps. Assert on host + path fragments
	// that are too specific to appear by coincidence.
	wantHost := oauthURL.Host
	wantPath := oauthURL.Path
	require.NotEmpty(t, wantHost, "OAuth URL must have a host")

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return containsAll(frame,
			"Follow the instructions",
			wantHost,
			wantPath,
		)
	}, 10*time.Second, 50*time.Millisecond,
		"expected the bootstrap wait prompt to render with the OAuth URL "+
			"(host=%q path=%q)", wantHost, wantPath)

	frame := handlertest.DrawHandler(wrapped, width, height)
	assert.Contains(t, frame, "Follow the instructions",
		"wait prompt header must render")
	assert.Contains(t, frame, wantHost,
		"wait prompt must inline the OAuth URL's host so the user can copy it")
	assert.Contains(t, frame, wantPath,
		"wait prompt must inline the OAuth URL's path so the user can copy it")
}

type bootstrapFlagOverrides struct {
	httpAddress    string
	dataPath       string
	configPath     string
	websiteAddress string
}

// TestBootstrapE2EFontSizeKeybindingDuringBootstrap pins the fix for
// the "unknown command or command alias \"guifontsize\"" bug. rune.star
// is loaded by the pre-config IDE, so its GUI keybinding <m-=> ->
// "guifontsize increase" is live during the first-run bootstrap flow.
// The guifontsize command, however, is registered only by
// subscribeGUICommands, which the configured IDE gets via
// setupConfiguredIDE — the pre-config IDE never did, so dispatching the
// keybinding surfaced "unknown command or command alias". runGUI now
// calls setupPreIDE on the first-run branch, which this test exercises.
//
// The test drives the real path end to end: build the pre-config
// bootstrap handler, attach a real GUI, run setupPreIDE, then feed the
// actual <m-=> key event through the handler and assert the resulting
// frame does not carry the "unknown command" error.
func TestBootstrapE2EFontSizeKeybindingDuringBootstrap(t *testing.T) {
	dataDir := t.TempDir()
	configPath := dataDir + "/config.yaml"

	mu := new(sync.Mutex)
	publishEvent, stopPump := newBootstrapPublishPump(mu)

	checkoutURL, signupURL := mustResolveBootstrapURLs("https://rune.test")
	root, err := newBootstrapHandler(
		dataDir, configPath, "" /* workspace */, "" /* zdotDir */, nil, /* filenames */
		nil /* launchCmd */, ide.FuncExtensionsRunner(testE2EExtensionsRunner),
		mu, publishEvent,
		checkoutURL, signupURL,
		func(*url.URL) error { return nil }, clipboard.NewInMemory(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	t.Cleanup(stopPump)
	require.NotNil(t, root.preIDE, "fresh data dir must build the pre-config IDE")
	require.Nil(t, root.realIDE, "fresh data dir must not build the configured IDE")

	// Attach a real GUI: the guifontsize handler calls
	// g.IncreaseFontSize() at dispatch time, so a nil GUI would panic
	// rather than exercise the command path under test.
	g, err := gui.New(root)
	require.NoError(t, err)
	root.attachGUI(g, false)
	require.NoError(t, root.setupPreIDE(),
		"setupPreIDE must register the pre-config IDE GUI commands")

	wrapped := &bootstrapE2ELocked{Handler: root, mu: mu}

	// A successful guifontsize dispatch calls g.IncreaseFontSize, which
	// rescales the cell grid and resizes the pre-config IDE to a new
	// column/row count. If the command were unregistered (the bug), the
	// keybinding would surface "unknown command" and leave the pre-config
	// IDE size untouched. So the resize is a race-free, drawing-free
	// signal that the command actually ran.
	beforeW, beforeH := root.preIDE.Size()

	// <m-=> is bound to "guifontsize increase" in rune.star.
	wrapped.Handle(keyEv('=', term.ModMeta))

	afterW, afterH := root.preIDE.Size()
	require.NotEqual(t, [2]int{beforeW, beforeH}, [2]int{afterW, afterH},
		"pressing <m-=> during bootstrap must dispatch the registered "+
			"guifontsize command and rescale the pre-config IDE, not "+
			"surface \"unknown command\"")
}

func overrideBootstrapFlags(t *testing.T, o bootstrapFlagOverrides) func() {
	t.Helper()
	prevHTTP := *flagHTTPAddress
	prevData := *flagDataPath
	prevConfig := *flagConfigPath
	prevWebsite := *flagWebsiteAddress
	*flagHTTPAddress = o.httpAddress
	*flagDataPath = o.dataPath
	*flagConfigPath = o.configPath
	*flagWebsiteAddress = o.websiteAddress
	return func() {
		*flagHTTPAddress = prevHTTP
		*flagDataPath = prevData
		*flagConfigPath = prevConfig
		*flagWebsiteAddress = prevWebsite
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}

// newBootstrapPublishPump wires the publish-event hook the way runGUI
// does: a pumper goroutine runs EventInterrupt UserFuncs under the
// bootstrap locker. Register the returned stop func with t.Cleanup
// after the handler's Close cleanup so it runs before it (LIFO),
// matching the runtime's pump-then-handler teardown order.
//
// stop fences off further publishes before closing the pump channel:
// the prompt-frame shader keeps interrupting at its own FPS well past
// a fast test's lifetime, and a late publish into a closed channel
// would panic the suite.
func newBootstrapPublishPump(
	mu *sync.Mutex,
) (publish func(term.Event) bool, stop func()) {
	publishCh := make(chan term.Event, 256)
	var publishMu sync.Mutex
	stopped := false
	publish = func(ev term.Event) bool {
		publishMu.Lock()
		defer publishMu.Unlock()
		if stopped {
			return false
		}
		select {
		case publishCh <- ev:
			return true
		default:
			return false
		}
	}
	pumperDone := make(chan struct{})
	go debug.CapturePanicReport(func() {
		defer close(pumperDone)
		for ev := range publishCh {
			if ev.Type == term.EventInterrupt && ev.UserFunc != nil {
				mu.Lock()
				ev.UserFunc()
				mu.Unlock()
			}
		}
	})
	stop = func() {
		publishMu.Lock()
		stopped = true
		publishMu.Unlock()
		close(publishCh)
		<-pumperDone
	}
	return publish, stop
}

// bootstrapE2ELocked serializes Handle/Draw/Resize on the bootstrap
// handler's locker so the pumper goroutine's UserFunc execution
// cannot race the test's Handle calls. Mirrors lockedHandler in
// ide_test.go.
type bootstrapE2ELocked struct {
	tui.Handler
	mu sync.Locker
}

func (h *bootstrapE2ELocked) Handle(ev term.Event) (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Handle(ev)
}

func (h *bootstrapE2ELocked) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Draw(w)
}

func (h *bootstrapE2ELocked) Resize(width, height int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Resize(width, height)
}

func (h *bootstrapE2ELocked) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Cursor()
}

func (h *bootstrapE2ELocked) Selection() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Selection()
}

// TestBootstrapE2ESignUpReopensLoginPrompt pins the fix for the
// "Sign up locks the user out" bug: pressing the Sign up button in
// the login choice prompt opens the signup URL in the user's browser
// and then must re-mount the Sign in / Sign up choice prompt so the
// user can come back to Rune after completing signup on the website.
//
// The bug was that openLoginPrompt was invoked synchronously inside
// the OnSelect callback, racing the SDK's prompt-teardown of the
// just-selected choice. The fresh prompt mount got torn down by the
// outgoing prompt's close logic, leaving the bootstrap with no
// visible UI and no way to retry sign in.
//
// The fix is to schedule the re-mount via scheduleNextTick so it
// runs on a later event-loop iteration, after the original prompt
// has fully closed. This test drives Welcome → Vim-mode →
// Sign-up via real keystrokes, observes the signup URL on the
// OpenBrowser hook, and asserts the choice prompt is visible again
// in the rendered frame.
func TestBootstrapE2ESignUpReopensLoginPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dataDir := t.TempDir()
	configPath := dataDir + "/config.yaml"

	restoreFlags := overrideBootstrapFlags(t, bootstrapFlagOverrides{
		httpAddress:    srv.URL,
		dataPath:       dataDir,
		configPath:     configPath,
		websiteAddress: "https://rune.test",
	})
	t.Cleanup(restoreFlags)

	browserURLCh := make(chan *url.URL, 1)
	openBrowser := func(u *url.URL) error {
		select {
		case browserURLCh <- u:
		default:
		}
		return nil
	}

	mu := new(sync.Mutex)
	publishEvent, stopPump := newBootstrapPublishPump(mu)

	checkoutURL, signupURL := mustResolveBootstrapURLs("https://rune.test")
	root, err := newBootstrapHandler(
		dataDir, configPath, "", "", nil,
		nil, ide.FuncExtensionsRunner(testE2EExtensionsRunner),
		mu, publishEvent,
		checkoutURL, signupURL,
		openBrowser, clipboard.NewInMemory(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	t.Cleanup(stopPump)

	const width, height = 80, 30
	wrapped := &bootstrapE2ELocked{Handler: root, mu: mu}
	wrapped.Resize(width, height)

	// Welcome → Vim-mode. 'v' enables vim mode; control then
	// advances to the login choice prompt.
	for _, ch := range []rune{'g', 'v'} {
		wrapped.Handle(term.Event{Type: term.EventKey, Ch: ch})
		time.Sleep(50 * time.Millisecond)
	}

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return containsAll(frame, "Sign in", "Sign up")
	}, 5*time.Second, 50*time.Millisecond,
		"login choice prompt must be visible after the vim-mode prompt advances")

	wrapped.Handle(term.Event{Type: term.EventKey, Ch: 's'})

	select {
	case got := <-browserURLCh:
		require.Equal(t, signupURL, got.String(),
			"Sign up must open the signup URL through the test hook, "+
				"not launch the host's real browser")
	case <-time.After(5 * time.Second):
		t.Fatal("Sign up did not open the signup URL through the test hook")
	}

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return containsAll(frame, "Sign in", "Sign up")
	}, 5*time.Second, 50*time.Millisecond,
		"after Sign up opens the browser, the login choice prompt must be re-mounted "+
			"so the user can come back to Rune; otherwise the bootstrap is stuck "+
			"with no visible UI")
}

// TestBootstrapE2EEscReopensBootstrapPrompt pins the fix for the
// "Esc kills the bootstrap" bug: dismissing any bootstrap prompt
// with Esc must reopen that prompt. Before the fix,
// clearOnClosePromptHandler deleted the prompt-dedup entry only
// after running the close callback, so the guardedPromptChain's
// reopen was deduped against the dying window and the user was left
// staring at an empty pre-config IDE with no way to continue.
func TestBootstrapE2EEscReopensBootstrapPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dataDir := t.TempDir()
	configPath := dataDir + "/config.yaml"

	restoreFlags := overrideBootstrapFlags(t, bootstrapFlagOverrides{
		httpAddress:    srv.URL,
		dataPath:       dataDir,
		configPath:     configPath,
		websiteAddress: "https://rune.test",
	})
	t.Cleanup(restoreFlags)

	mu := new(sync.Mutex)
	publishEvent, stopPump := newBootstrapPublishPump(mu)

	checkoutURL, signupURL := mustResolveBootstrapURLs("https://rune.test")
	root, err := newBootstrapHandler(
		dataDir, configPath, "", "", nil,
		nil, ide.FuncExtensionsRunner(testE2EExtensionsRunner),
		mu, publishEvent,
		checkoutURL, signupURL,
		func(*url.URL) error { return nil }, clipboard.NewInMemory(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	t.Cleanup(stopPump)

	const width, height = 80, 30
	wrapped := &bootstrapE2ELocked{Handler: root, mu: mu}
	wrapped.Resize(width, height)

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return strings.Contains(frame, "Welcome to Rune")
	}, 5*time.Second, 50*time.Millisecond,
		"welcome prompt must be visible at bootstrap start")

	wrapped.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return strings.Contains(frame, "Welcome to Rune")
	}, 5*time.Second, 50*time.Millisecond,
		"Esc on the welcome prompt must reopen it; an empty screen "+
			"leaves the user with a dead installation")

	// Advance to the vim-mode prompt and make sure Esc reopens
	// mid-chain prompts too.
	wrapped.Handle(term.Event{Type: term.EventKey, Ch: 'g'})

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return strings.Contains(frame, "Choose your key bindings")
	}, 5*time.Second, 50*time.Millisecond,
		"vim-mode prompt must be visible after the welcome prompt advances")

	wrapped.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})

	require.Eventually(t, func() bool {
		frame := handlertest.DrawHandler(wrapped, width, height)
		return strings.Contains(frame, "Choose your key bindings")
	}, 5*time.Second, 50*time.Millisecond,
		"Esc on the vim-mode prompt must reopen it")
}

func testE2EExtensionsRunner(
	_ workspaceapi.URI,
	_ map[extensionapi.Permission]extension.ResourceRegistrar,
	_ string,
	_ browser.Notifications,
	_, _ schemeapi.Executor,
	_ extension.Grantor,
	_ text.Editor,
	_ ideauthorizer.PromptOpener,
	_ storageapi.Service,
	_ func(func()) bool,
) (extension.Runner, error) {
	return nopE2ERunner{}, nil
}

type nopE2ERunner struct{}

func (nopE2ERunner) Run(string, string, config.Config) error { return nil }
func (nopE2ERunner) Close() error                            { return nil }
func (n nopE2ERunner) WaitReady(ctx context.Context, id string) error {
	return nil
}
