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

package vte

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/term/vte/vtetest"
	"unstable.build/go-tui/workspace"
)

// this is the timeout to wait for the shell to stop updating the
// internal state of the vte, to call a test case "complete", so
// assertions can run. The slower the host of the tests, the longer
// this timeout should be.
var defaultWaitForIdleVte = 100 * time.Millisecond

func init() {
	if os.Getenv("CI") == "true" {
		defaultWaitForIdleVte = 150 * time.Millisecond
	}
}

// TestMain isolates HOME for the entire package's child processes so
// vim invocations in integration tests (e.g. TestHandlerIntegration)
// write their .viminfo and .viminf[a-z].tmp lock files into a
// throwaway directory. A real $HOME left over from prior crashes can
// already carry the full a-z spinner of viminf*.tmp files, which
// triggers E929 (too many viminfo temp files) and prevents the
// editor banner from appearing at all — the test then fails on a
// golden screen mismatch rather than a vim error.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "vte-home")
	if err != nil {
		panic(err)
	}
	prev, hadPrev := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		panic(err)
	}
	code := m.Run()
	if hadPrev {
		_ = os.Setenv("HOME", prev)
	} else {
		_ = os.Unsetenv("HOME")
	}
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func TestHandlerIntegration(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"",
			`$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"ls",
			`$ ls▐               
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"^^echo bla>",
			`$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `},
		{"vi>ihello",
			`hello▐              
~                   
~                   
~                   
~                   
~                   
~                   
~                   
~                   
-- INSERT --        `},
		{"<:quit!>",
			`$ echo bla          
bla                 
$ vi                
$ ▐                 
                    
                    
                    
                    
                    
                    `},
	}

	cfg := DefaultConfig()

	// vi needs quite a bit of tiem to exit
	waitForIdleVte := defaultWaitForIdleVte * 4

	testSequence(t, cfg, waitForIdleVte, cases)
}

func TestHandlerEnvIntegration(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"",
			`$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		{"echo \\$MYENV>",
			`$ echo $MYENV       
hello               
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `},
	}

	os.Setenv("MYENV", "hello")
	defer os.Setenv("MYENV", "")
	cfg := DefaultConfig()

	waitForIdleVte := defaultWaitForIdleVte * 4
	testSequence(t, cfg, waitForIdleVte, cases)
}

func TestHandlerCloseExit(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"exit>",
			`$ exit              
exit                
                    
                    
                    
                    
                    
                    
                    
                    `},
	}

	cfg := DefaultConfig()

	handler, _ := testSequence(t, cfg, defaultWaitForIdleVte, cases)
	exit, handled := handler.Handle(term.Event{})
	require.True(t, exit)
	assert.False(t, handled)

	_, _, show := handler.Cursor()
	assert.False(t, show)
}

// TestHandlerPublishesEventOnPtyExit pins the auto-close behaviour
// browser.Tab.Handle relies on: when the underlying pty child dies
// (e.g. the user types :q in an embedded vim, or exit in a shell),
// the host event loop must receive at least one event so it routes a
// Handle call to the tab. Without the wake-up the dead vte sits
// black until the user presses another key.
func TestHandlerPublishesEventOnPtyExit(t *testing.T) {
	t.Parallel()
	if ci := os.Getenv("CI"); ci == "true" {
		t.SkipNow()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	uri, err := workspaceapi.CurrentUserHostURI(os.TempDir())
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	t.Cleanup(func() { scheme.Close() })

	ps1 := os.Getenv("PS1")
	os.Setenv("PS1", "$ ")
	t.Cleanup(func() { os.Setenv("PS1", ps1) })

	pub := &exitWakePublisher{}
	cfg := DefaultConfig()
	cfg.WidthHint = 20
	cfg.HeightHint = 10
	cfg.CommandAndArgs = []string{"sh"}
	handler, err := NewHandler(pub, nopNotifications{}, scheme, scheme, nopTabManager{}, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { handler.Close() })

	handler.Resize(20, 10)

	for _, b := range []byte("exit\n") {
		handler.Handle(term.Event{Type: term.EventKey, Ch: rune(b), Raw: []byte{b}})
	}

	require.Eventually(t, func() bool {
		return pub.SawEventNone()
	}, 5*time.Second, 10*time.Millisecond,
		"vte.Handler must publish at least one event after the "+
			"pty child exits so the host event loop can route a "+
			"Handle call and trigger tab auto-close without "+
			"further user input")
}

type exitWakePublisher struct {
	mu      sync.Mutex
	sawNone bool
}

func (p *exitWakePublisher) PublishEvent(ev term.Event) error {
	if ev.Type == term.EventNone {
		p.mu.Lock()
		p.sawNone = true
		p.mu.Unlock()
	}
	return nil
}

func (p *exitWakePublisher) SawEventNone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sawNone
}

// TestHandlerMouseSelection drives press/drag/release sequences over
// the vte handler and asserts the resulting Selection() contents. The
// leftward and same-cell cases pin the user-reported bug where the
// first (leftmost) cell of a leftward drag was excluded from the
// selection.
func TestHandlerMouseSelection(t *testing.T) {
	t.Parallel()

	// All cases echo "hello" so it lands on row 1. The buffer reports
	// Columns(1)=5, which clamps any to.X past column 5.
	const helloRow = 1
	type mev struct {
		key  term.Key
		x, y int
	}
	left := func(x, y int) mev { return mev{term.MouseLeft, x, y} }
	rel := func(x, y int) mev { return mev{term.MouseRelease, x, y} }

	cases := []struct {
		desc   string
		events []mev
		want   string
	}{
		{
			desc:   "rightward drag from col 0 to col 4",
			events: []mev{left(0, helloRow), left(4, helloRow), rel(4, helloRow)},
			want:   "hello",
		},
		{
			desc:   "leftward drag from col 5 to col 0 includes first cell",
			events: []mev{left(5, helloRow), left(0, helloRow), rel(0, helloRow)},
			want:   "hello",
		},
		{
			desc:   "press and drag on same cell selects that cell",
			events: []mev{left(0, helloRow), left(0, helloRow), rel(0, helloRow)},
			want:   "h",
		},
		{
			desc:   "press right, drag one cell left",
			events: []mev{left(4, helloRow), left(3, helloRow), rel(3, helloRow)},
			want:   "lo",
		},
		{
			desc:   "drag past row's last column clamps to Columns(y)",
			events: []mev{left(4, helloRow), left(5, helloRow), rel(5, helloRow)},
			want:   "o",
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			cfg := DefaultConfig()
			handler, _ := testSequence(t, cfg, defaultWaitForIdleVte,
				[]vtetest.Case{{"echo hello>",
					`$ echo hello        
hello               
$ ▐                 
                    
                    
                    
                    
                    
                    
                    `}})
			for _, ev := range tc.events {
				handler.Handle(term.Event{
					Type: term.EventMouse, Key: ev.key,
					MouseX: ev.x, MouseY: ev.y,
				})
			}
			sel, ok := handler.Selection()
			require.True(t, ok)
			assert.Equal(t, tc.want, sel)
		})
	}
}

func TestResetPrimaryBuffer(t *testing.T) {
	t.Parallel()
	t.Run("non modal", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo bla>echo bla>",
				`$ echo bla          
bla                 
$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

		handler.ClearPrimaryBuffer()
		time.Sleep(defaultWaitForIdleVte)

		cases = []vtetest.Case{
			{"echo XXX>",
				`                    
$ echo XXX          
XXX                 
$ ▐                 
                    
                    
                    
                    
                    
                    `},
		}

		vtetest.TestSequence(t, handler, 20, 10, defaultWaitForIdleVte, ch, cases)
	})

	t.Run("on modal mode", func(t *testing.T) {
		t.Parallel()
		cases := []vtetest.Case{
			{"echo bla>echo bla><",
				`$ echo bla          
bla                 
$ echo bla          
bla                 
$ ▐                 
                    
                    
                    
                    
                    `},
		}
		cfg := DefaultConfig()
		cfg.Modal = true
		handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

		handler.ClearPrimaryBuffer()
		time.Sleep(defaultWaitForIdleVte)

		cases = []vtetest.Case{
			{"echo XXX><kv0yjP",
				`                    
$ echo XXX          
XXX                 
$ XX▐               
                    
                    
                    
                    
                    
                    `},
		}

		vtetest.TestSequence(t, handler, 20, 10, defaultWaitForIdleVte, ch, cases)
	})
}

func TestHandlerResizeViIntegration(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"echo 'a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk'",
			`> b                 
> c                 
> d                 
> e                 
> f                 
> g                 
> h                 
> i                 
> j                 
> k'▐               `},
		{">",
			`c                   
d                   
e                   
f                   
g                   
h                   
i                   
j                   
k                   
$ ▐                 `},
	}

	cfg := DefaultConfig()
	cfg.Modal = true
	handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

	// test same width/height resize, which simulates window manager
	// calling Resize on every children after a window re-configuration.
	handler.Resize(20, 10)

	cases = []vtetest.Case{
		{"",
			`c                   
d                   
e                   
f                   
g                   
h                   
i                   
j                   
k                   
$ ▐                 `},
	}

	vtetest.TestCases(t, handler, 20, 10, defaultWaitForIdleVte, ch, cases)
}

func testSequence(t *testing.T, cfg Config, timeout time.Duration, cases []vtetest.Case) (
	*Handler, chan struct{},
) {
	shell := "sh" // all systems were this runs should have sh
	return testSequenceShell(t, cfg, timeout, shell, cases)
}

func testSequenceShell(t *testing.T, cfg Config, timeout time.Duration, shell string, cases []vtetest.Case) (
	*Handler, chan struct{},
) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	temp := os.TempDir()

	uri, err := workspaceapi.CurrentUserHostURI(temp)
	require.NoError(t, err)

	scheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)

	ps1 := os.Getenv("PS1")
	os.Setenv("PS1", "$ ")

	ch := make(chan struct{}, 50 /* big enough for the max length sequence of events */)
	cfg.WidthHint = 20
	cfg.HeightHint = 10
	cfg.CommandAndArgs = []string{shell}
	handler, err := NewHandler(chanEventPublisher{ch}, nopNotifications{},
		scheme, scheme, nopTabManager{}, cfg)
	require.NoError(t, err)

	if ci := os.Getenv("CI"); ci == "true" {
		// the version of sh running on the CI docker containers
		// doesn't support bell (neither ctrl+g or ctrl+a + <-)
		t.SkipNow()
	}

	t.Cleanup(func() {
		handler.Close()
		scheme.Close()
		cancel()
		os.Setenv("PS1", ps1)
	})

	vtetest.TestSequence(t, handler, cfg.WidthHint, cfg.HeightHint,
		timeout, ch, cases)

	return handler, ch
}

type chanEventPublisher struct {
	ch chan struct{}
}

func (p chanEventPublisher) PublishEvent(term.Event) error {
	p.ch <- struct{}{}
	return nil
}

type nopNotifications struct {
}

func (nopNotifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	return "", nil
}

func (nopNotifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	return "", nil
}

func (n nopNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return nil
}

// TestHandlerHandleReturnsHandledWithoutPtyEcho pins the regression
// where a slow pty round-trip (e.g. an SSH workspace pty whose output
// arrives via workspacerpc) caused vte.Handler.Handle to return
// handled=false even though the keypress had already been written to
// the pty. The IDE sequencer would then treat the unhandled key as a
// candidate for sequence matching ("g" is a prefix of "gg"/"gf") and
// re-issue it on timeout, surfacing duplicated input ("g" -> "gg").
func TestHandlerHandleReturnsHandledWithoutPtyEcho(t *testing.T) {
	t.Parallel()

	// Use a real shell only to keep parity with other handler tests:
	// what we exercise is the Handle return contract, not echo.
	cases := []vtetest.Case{
		{"",
			`$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
	}
	cfg := DefaultConfig()
	handler, _ := testSequence(t, cfg, defaultWaitForIdleVte, cases)

	// Force the pty-echo wait window to effectively zero so the test
	// reproducibly exercises the path where Handle returns BEFORE
	// any update arrives from the pty. This mirrors the production
	// failure mode where an SSH workspace pty round-trips bytes via
	// workspacerpc and the echo arrives well after handleTimeout.
	handler.handleTimeout = time.Nanosecond

	// Drive Handle directly with a key event and assert handled=true
	// returns even if no pty update arrives within handleTimeout. The
	// underlying contract: writing to the pty is the moment the event
	// is "consumed" by vte.Handler.
	exit, handled := handler.Handle(term.Event{
		Type: term.EventKey,
		Ch:   'g',
		Raw:  []byte("g"),
	})
	assert.False(t, exit)
	assert.True(t, handled,
		"vte.Handler.Handle must return handled=true once the event "+
			"is written to the pty, regardless of whether the pty "+
			"echo round-trip completed within handleTimeout. "+
			"Otherwise, callers chaining into a key sequencer "+
			"(e.g. ide/ex) duplicate input on slow remote ptys.")
}

func TestHandlerPasteEndWritesBufferedInput(t *testing.T) {
	t.Parallel()

	handler := &Handler{
		comp: &Component{
			parserHandler: &parserHandler{useAlt: true},
		},
	}

	handled, raw := handler.handleInput(term.Event{Type: term.EventPasteStart})
	assert.True(t, handled)
	assert.Empty(t, raw)

	handled, raw = handler.handleInput(term.Event{
		Type: term.EventKey,
		Ch:   's',
		Raw:  []byte("secret\n"),
	})
	assert.True(t, handled)
	assert.Empty(t, raw)

	handled, raw = handler.handleInput(term.Event{Type: term.EventPasteEnd})
	assert.False(t, handled)
	assert.Equal(t, []byte("secret\r"), raw)
}

// TestHandlerCoalescesInterruptsDuringHandle pins that vte.Handler
// suppresses publisher-bound EventInterrupts for the duration of a
// Handle call, so a single keystroke that drives the embedded program
// to flush in multiple stages produces at most one publish, not one
// per stage. Terminal sessions rely on this to avoid redraw thrash.
func TestHandlerCoalescesInterruptsDuringHandle(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"", `$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
	}
	cfg := DefaultConfig()
	handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

	drain(ch)
	exit, handled := handler.Handle(term.Event{
		Type: term.EventKey, Ch: 'a', Raw: []byte("a"),
	})
	assert.False(t, exit)
	assert.True(t, handled)
	time.Sleep(defaultWaitForIdleVte)

	assert.LessOrEqual(t, len(ch), 1,
		"a single Handle call must publish at most one EventInterrupt; "+
			"got %d. The sema-gated publisher plus the post-write wait "+
			"in Handle exist to keep terminal redraws cheap.", len(ch))
}

func drain(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// TestHandlerCtrlCInterruptsForegroundProgram pins the regression where
// pressing ctrl-c on a foreground program running in the primary buffer
// (e.g. a blocking `sleep`) failed to interrupt it. The handler must
// write the raw ETX byte (0x03) to the pty so the kernel line
// discipline delivers SIGINT to the foreground process group, returning
// control to the shell prompt.
func TestHandlerCtrlCInterruptsForegroundProgram(t *testing.T) {
	t.Parallel()
	cases := []vtetest.Case{
		{"",
			`$ ▐                 
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
		// start a foreground program that blocks; the shell prompt
		// must not return until the program is interrupted.
		{"sleep 30>",
			`$ sleep 30          
▐                   
                    
                    
                    
                    
                    
                    
                    
                    `},
	}
	cfg := DefaultConfig()
	handler, ch := testSequence(t, cfg, defaultWaitForIdleVte, cases)

	drain(ch)

	// ctrl-c with the raw ETX byte the gui input layer produces.
	exit, handled := handler.Handle(term.Event{
		Type: term.EventKey, Mod: term.ModCtrl, Ch: 'c', Raw: []byte{0x03},
	})
	assert.False(t, exit)
	assert.True(t, handled)

	w := term.NewStringWriter(20, 10)
	require.Eventually(t, func() bool {
		require.NoError(t, w.Clear(term.Attributes{}))
		handler.Draw(w)
		require.NoError(t, w.Flush())
		// after the interrupt the shell must print a fresh prompt
		// below the interrupted command line.
		return strings.Count(w.String(), "$ ") >= 2
	}, 5*time.Second, 20*time.Millisecond,
		"ctrl-c must interrupt the foreground sleep and return the "+
			"shell prompt; screen was:\n%s", w.String())
}
