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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/term"

	wmbrowser "unstable.build/go-tui/browser"
	"unstable.build/go-tui/ide/ideplan"
)

func TestOptionToChoiceMapping(t *testing.T) {
	cases := []struct {
		option string
		want   string
	}{
		{optVimYes, editorModal},
		{optVimNo, editorModeless},
		{"unknown", editorModal}, // default fallback
	}
	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			require.Equal(t, tc.want, optionToChoice(tc.option))
		})
	}
}

// TestRenderOverride pins that the vim-mode choice maps to the modal
// override and the standard-editor choice maps to the modeless override
// (which switches editor.mode to modeless), and that an unknown choice
// is an error.
func TestRenderOverride(t *testing.T) {
	yes, err := renderOverride(editorModal)
	require.NoError(t, err)
	require.NotContains(t, yes, "mode: modeless",
		"vim mode must not switch the editor into modeless")

	no, err := renderOverride(editorModeless)
	require.NoError(t, err)
	require.Contains(t, no, "mode: modeless",
		"declining vim mode must switch the editor into modeless")

	_, err = renderOverride("bogus")
	require.Error(t, err)
}

// TestGuardedPromptChainReopensOnUnadvancedClose proves that an Esc
// (or any dismissal that does not call OnSelect) re-opens the same
// prompt, while a normal OnSelect-then-Close sequence does not, and
// neither does a close fired by the pre-config IDE's own teardown.
func TestGuardedPromptChainReopensOnUnadvancedClose(t *testing.T) {
	t.Run("esc reopens", func(t *testing.T) {
		var reopened int
		g := &guardedPromptChain{closing: func() bool { return false }}
		_ = g.onClose(func() { reopened++ })()
		require.Equal(t, 1, reopened, "Esc-equivalent close must re-open")
	})

	t.Run("select does not reopen", func(t *testing.T) {
		var reopened int
		var advanced int
		g := &guardedPromptChain{closing: func() bool { return false }}
		// Simulate the normal selection-then-close ordering: the
		// SDK calls OnSelect first (inside Handle), which marks
		// advanced, then Close → OnClose.
		g.onSelect(func(_ int, _ string) { advanced++ })(0, "")
		_ = g.onClose(func() { reopened++ })()
		require.Equal(t, 1, advanced)
		require.Equal(t, 0, reopened, "selection must not re-open")
	})

	t.Run("closing pre-config IDE does not reopen", func(t *testing.T) {
		var reopened int
		g := &guardedPromptChain{closing: func() bool { return true }}
		_ = g.onClose(func() { reopened++ })()
		require.Equal(t, 0, reopened,
			"teardown-driven close must not re-open the prompt")
	})
}

// TestShouldSwallowBootstrapEvent enumerates the dangerous keys that
// must not reach the pre-config IDE while the bootstrap flow is in
// progress, plus a handful of events that must pass through untouched.
func TestShouldSwallowBootstrapEvent(t *testing.T) {
	cases := []struct {
		name string
		ev   term.Event
		want bool
	}{
		// Dangerous: ':' opens the command prompt (configurable
		// activation key from default rune.star).
		{"colon opens command prompt", keyEv(':', 0), true},

		// Dangerous: default quit / close-window / close-tab
		// bindings from rune.star and override_modeless.star.
		{"meta-q quit", keyEv('q', term.ModMeta), true},
		{"meta-w windowclose", keyEv('w', term.ModMeta), true},
		{"alt-w tabclose", keyEv('w', term.ModAlt), true},
		{"ctrl-w tabclose", keyEv('w', term.ModCtrl), true},
		{"meta-shift-w", keyEv('w', term.ModMeta|term.ModShift), true},

		// Pass-through: arrow keys (prompt navigation), mouse,
		// resize, normal text input, Enter (used for one-button
		// welcome screen), Esc (the prompt's own close path, which
		// the guarded close callback re-opens).
		{"arrow left", namedKeyEv(term.KeyArrowLeft), false},
		{"enter", namedKeyEv(term.KeyEnter), false},
		{"esc", namedKeyEv(term.KeyEsc), false},
		{"plain rune", keyEv('a', 0), false},
		{"resize", term.Event{Type: term.EventResize, Width: 80, Height: 24}, false},
		{"mouse", term.Event{Type: term.EventMouse}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldSwallowBootstrapEvent(tc.ev))
		})
	}
}

func keyEv(ch rune, mod term.Modifier) term.Event {
	return term.Event{Type: term.EventKey, Ch: ch, Mod: mod}
}

func namedKeyEv(k term.Key) term.Event {
	return term.Event{Type: term.EventKey, Key: k}
}

// recordingNotifier captures notify calls so tests can assert the
// copy-URL helper surfaces success/failure.
type recordingNotifier struct {
	calls []struct {
		level browserapi.NotificationLevel
		msg   string
	}
}

func (r *recordingNotifier) Notify(
	level browserapi.NotificationLevel, format string, args ...any,
) (string, error) {
	r.calls = append(r.calls, struct {
		level browserapi.NotificationLevel
		msg   string
	}{level: level, msg: format})
	return "", nil
}
func (r *recordingNotifier) NotifyOnce(
	level browserapi.NotificationLevel, format string, args ...any,
) (string, error) {
	return r.Notify(level, format, args...)
}
func (r *recordingNotifier) UpdateNotificationProgress(string, string, int64, int64) error {
	return nil
}

// TestCopyBootstrapURLReturnsNotification pins that copyBootstrapURL
// writes the OAuth URL to the injected clipboard and reports success,
// never touching the developer's real system clipboard.
func TestCopyBootstrapURLReturnsNotification(t *testing.T) {
	clip := clipboard.NewInMemory()
	bh := &bootstrapHandler{clip: clip}

	level, msg := bh.copyBootstrapURL("https://example.test/auth?x=1")
	require.NotEmpty(t, msg, "copyBootstrapURL must produce a notification message")
	require.Equal(t, browserapi.LevelSuccess, level,
		"in-memory clipboard copy must succeed")
	data, err := clip.Paste(clipboard.DefaultRegisterID)
	require.NoError(t, err)
	require.Equal(t, "https://example.test/auth?x=1", data.Text,
		"the OAuth URL must be written to the injected clipboard, "+
			"never the developer's real system clipboard")
}

// closeRecordingWindow stands in for wmbrowser.Window in tests. The
// embedded interface lets the struct satisfy wmbrowser.Window without
// reimplementing every method.
type closeRecordingWindow struct {
	wmbrowser.Window
	closed bool
}

func (w *closeRecordingWindow) Close() error {
	w.closed = true
	return nil
}

// TestHandleLoginDoneClosesWaitWindowBeforeRefresh pins the fix for
// the "stuck on Follow browser instructions" bug: the token refresh
// performs synchronous HTTP I/O, so it must not run inside an event
// loop tick. The wait window's close tick must reach the event loop
// while refresh is still in flight; otherwise the user sees the wait
// prompt indefinitely after a successful sign-in.
func TestHandleLoginDoneClosesWaitWindowBeforeRefresh(t *testing.T) {
	win := &closeRecordingWindow{}
	coord := &loginCoord{window: win}

	var mu sync.Mutex
	var ticks []func()
	scheduleTick := func(fn func()) bool {
		mu.Lock()
		ticks = append(ticks, fn)
		mu.Unlock()
		return true
	}

	refreshStarted := make(chan struct{})
	refreshUnblock := make(chan struct{})
	refresh := func(ctx context.Context) (ideplan.Decision, error) {
		close(refreshStarted)
		<-refreshUnblock
		return ideplan.Decision{Status: ideplan.StatusActive}, nil
	}

	done := make(chan struct{})
	go func() {
		handleLoginDone(loginDoneArgs{
			ok:            true,
			coord:         coord,
			scheduleTick:  scheduleTick,
			decide:        refresh,
			onSwap:        func() error { return nil },
			onUpgrade:     func() error { return nil },
			onLoginPrompt: func() error { return nil },
			notifyError:   func(string, error) {},
		})
		close(done)
	}()

	select {
	case <-refreshStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh never started")
	}

	// At this point handleLoginDone is blocked inside refresh.
	// The close-window tick must have already been scheduled so the
	// event loop can drain it while we are still in flight.
	mu.Lock()
	ticksWhileRefreshing := append([]func(){}, ticks...)
	mu.Unlock()
	require.NotEmpty(t, ticksWhileRefreshing,
		"window-close tick must be scheduled before refresh blocks")

	ticksWhileRefreshing[0]()
	require.True(t, win.closed,
		"first scheduled tick must close the wait window")

	close(refreshUnblock)
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(ticks), 2,
		"refresh-success path must schedule a follow-up tick")
}

// TestHandleLoginDoneCancelledShortCircuits ensures that a cancelled
// login still closes the wait window via the first tick but skips
// refresh and any follow-up UI transition.
func TestHandleLoginDoneCancelledShortCircuits(t *testing.T) {
	win := &closeRecordingWindow{}
	coord := &loginCoord{window: win, cancelled: true}

	var ticks []func()
	scheduleTick := func(fn func()) bool {
		ticks = append(ticks, fn)
		return true
	}

	refreshCalled := false
	handleLoginDone(loginDoneArgs{
		ok:           true,
		coord:        coord,
		scheduleTick: scheduleTick,
		decide: func(ctx context.Context) (ideplan.Decision, error) {
			refreshCalled = true
			return ideplan.Decision{}, nil
		},
		onSwap:        func() error { t.Fatal("onSwap must not run when cancelled"); return nil },
		onUpgrade:     func() error { t.Fatal("onUpgrade must not run when cancelled"); return nil },
		onLoginPrompt: func() error { t.Fatal("onLoginPrompt must not run when cancelled"); return nil },
		notifyError:   func(string, error) { t.Fatal("notifyError must not run when cancelled") },
	})

	require.False(t, refreshCalled, "refresh must not run when cancelled")
	require.Len(t, ticks, 1, "only the close-window tick should be scheduled")
	ticks[0]()
	require.True(t, win.closed)
}

// TestHandleLoginDoneRefreshErrorFallsBackToLoginPrompt verifies that
// a refresh failure routes to openLoginPrompt rather than crashing or
// leaving the wait prompt visible.
func TestHandleLoginDoneRefreshErrorFallsBackToLoginPrompt(t *testing.T) {
	win := &closeRecordingWindow{}
	coord := &loginCoord{window: win}

	var ticks []func()
	scheduleTick := func(fn func()) bool {
		ticks = append(ticks, fn)
		return true
	}

	loginPromptCalls := 0
	handleLoginDone(loginDoneArgs{
		ok:           true,
		coord:        coord,
		scheduleTick: scheduleTick,
		decide: func(ctx context.Context) (ideplan.Decision, error) {
			return ideplan.Decision{}, errors.New("refresh boom")
		},
		onSwap:        func() error { t.Fatal("onSwap must not run on refresh error"); return nil },
		onUpgrade:     func() error { t.Fatal("onUpgrade must not run on refresh error"); return nil },
		onLoginPrompt: func() error { loginPromptCalls++; return nil },
		notifyError:   func(string, error) {},
	})

	require.Len(t, ticks, 2,
		"expected close-window tick + login-prompt fallback tick")
	for _, fn := range ticks {
		fn()
	}
	require.True(t, win.closed)
	require.Equal(t, 1, loginPromptCalls)
}

// TestHandleLoginDoneCallbackErrorNotifiesUser proves that a failure
// inside any post-login callback (onSwap, onUpgrade, onLoginPrompt)
// reaches the user via notifyError instead of being swallowed into
// the log. The bootstrap user is staring at the prompt and would
// otherwise see nothing happen.
func TestHandleLoginDoneCallbackErrorNotifiesUser(t *testing.T) {
	win := &closeRecordingWindow{}
	coord := &loginCoord{window: win}

	var ticks []func()
	scheduleTick := func(fn func()) bool {
		ticks = append(ticks, fn)
		return true
	}

	swapErr := errors.New("swap exploded")
	var notified []struct {
		ctx string
		err error
	}
	handleLoginDone(loginDoneArgs{
		ok:           true,
		coord:        coord,
		scheduleTick: scheduleTick,
		decide: func(ctx context.Context) (ideplan.Decision, error) {
			return ideplan.Decision{Status: ideplan.StatusActive}, nil
		},
		onSwap:        func() error { return swapErr },
		onUpgrade:     func() error { t.Fatal("onUpgrade must not run on active swap"); return nil },
		onLoginPrompt: func() error { t.Fatal("onLoginPrompt must not run on active swap"); return nil },
		notifyError: func(ctx string, err error) {
			notified = append(notified, struct {
				ctx string
				err error
			}{ctx, err})
		},
	})

	for _, fn := range ticks {
		fn()
	}
	require.Len(t, notified, 1, "swap failure must reach notifyError")
	require.Equal(t, "finish bootstrap", notified[0].ctx)
	require.ErrorIs(t, notified[0].err, swapErr)
}
