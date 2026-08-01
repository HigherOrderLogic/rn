// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package ide_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/ide/pkgtrust"
)

// quitTutorialSrc parks on a floating_window step. Floating windows
// swallow every key that is not Enter/Esc/Space, which is what made
// the IDE unquittable mid-tutorial.
const quitTutorialSrc = `
def run():
    floating_window(title="welcome", text="hello there")
    floating_window(title="second", text="almost done")
tutorial(entry=run)
`

// waitTutorialSrc parks on a wait_command step, which passes keys
// through to the IDE root. It exercises the other half of the quit
// path: an exit decided by the root under a live tutorial overlay.
const waitTutorialSrc = `
def run():
    wait_command(command="edit")
    floating_window(title="done", text="finished up")
tutorial(entry=run)
`

// newQuitE2EIDE builds a real IDE with <m-q> bound to quit and the
// tutorials above registered under the `tutorials:` config key, then
// returns the root handler wrapped the way the host event loop drives
// it.
func newQuitE2EIDE(t *testing.T) (e2eLockedHandler, *ide.IDE) {
	t.Helper()
	dir := t.TempDir()

	tutPath := filepath.Join(dir, "basics.star")
	require.NoError(t, os.WriteFile(tutPath, []byte(quitTutorialSrc), 0o600))
	waitPath := filepath.Join(dir, "waiting.star")
	require.NoError(t, os.WriteFile(waitPath, []byte(waitTutorialSrc), 0o600))

	cfgPath := filepath.Join(dir, "rune.yaml")
	require.NoError(t, os.WriteFile(cfgPath, fmt.Appendf(nil, `
editor:
  mode: modal
command:
  key: ":"
  key_bindings:
    "<m-q>": quit
tutorials:
  basics: %s
  waiting: %s
`, tutPath, waitPath), 0o600))

	var mu sync.Mutex
	tracker := newSchedTracker(hostScheduleNextTick(&mu))
	i, err := ide.New(dir, cfgPath, dir, pkgtrust.NewStore(dir, nil),
		newE2EStorage(t, dir),
		ide.WithLocker(&mu),
		ide.WithScheduleNextTick(tracker.Schedule),
		ide.WithPublishEvent(func(term.Event) bool { return true }),
	)
	require.NoError(t, err)

	h := e2eLockedHandler{Handler: i.Ready(), mu: &mu, ide: i, sched: tracker}
	h.Resize(40, 14)
	i.WaitWorkspaces()
	return h, i
}

func e2eRender(t *testing.T, h e2eLockedHandler) string {
	t.Helper()
	w := term.NewStringWriter(40, 14)
	h.Draw(w)
	require.NoError(t, w.Flush())
	return w.String()
}

// e2eWaitFor polls the rendered frame until it contains want. A
// tutorial step becomes active on its own goroutine, so the frame it
// paints is not observable synchronously after the start command.
func e2eWaitFor(t *testing.T, h e2eLockedHandler, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		last = e2eRender(t, h)
		if strings.Contains(last, want) {
			return
		}
		h.Handle(term.Event{Type: term.EventInterrupt})
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in frame:\n%s", want, last)
}

// e2eWaitGone is e2eWaitFor's inverse: it polls until unwanted has
// disappeared from the frame, asserting no exit is reported meanwhile.
func e2eWaitGone(t *testing.T, h e2eLockedHandler, unwanted string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		last = e2eRender(t, h)
		if !strings.Contains(last, unwanted) {
			return
		}
		exit, _ := h.Handle(term.Event{Type: term.EventInterrupt})
		require.False(t, exit, "the IDE must not exit on its own")
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q to disappear from frame:\n%s", unwanted, last)
}

func e2eFeed(t *testing.T, h e2eLockedHandler, seq string) (exit bool) {
	t.Helper()
	keys, err := term.ParseKeys(seq)
	require.NoError(t, err)
	for _, k := range keys {
		quit, _ := h.Handle(term.Event{
			Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key})
		exit = exit || quit
	}
	return exit
}

// TestE2EQuit drives a real IDE end to end and pins that the quit
// chord is always escapable, including while a tutorial overlay is
// parked on a floating_window step, and that a tutorial finishing on
// its own never exits the IDE.
func TestE2EQuit(t *testing.T) {
	t.Parallel()

	t.Run("quit chord exits with no tutorial running", func(t *testing.T) {
		t.Parallel()
		h, i := newQuitE2EIDE(t)
		t.Cleanup(func() { _ = i.Close() })

		require.False(t, e2eFeed(t, h, "<m-q>"),
			"quit must open the confirm prompt before exiting")
		e2eWaitFor(t, h, "Yes")

		assert.True(t, e2eFeed(t, h, "y"),
			"answering Yes must exit the IDE")
	})

	t.Run("quit chord exits while a tutorial is parked on a floating window",
		func(t *testing.T) {
			t.Parallel()
			h, i := newQuitE2EIDE(t)
			t.Cleanup(func() { _ = i.Close() })

			require.False(t, e2eFeed(t, h, ":tutorial<space>start<space>basics<enter>"))
			e2eWaitFor(t, h, "hello there")

			require.False(t, e2eFeed(t, h, "<m-q>"),
				"quit must open the confirm prompt before exiting")
			frame := e2eRender(t, h)
			assert.NotContains(t, frame, "hello there",
				"the tutorial must be torn down so the confirm prompt is visible")
			e2eWaitFor(t, h, "Yes")

			assert.True(t, e2eFeed(t, h, "y"),
				"answering Yes must exit the IDE")
		})

	t.Run("typed :quit exits while a tutorial is waiting for a command",
		func(t *testing.T) {
			t.Parallel()
			h, i := newQuitE2EIDE(t)
			t.Cleanup(func() { _ = i.Close() })

			require.False(t, e2eFeed(t, h, ":tutorial<space>start<space>waiting<enter>"))

			// A wait_command step passes keys through, so the quit is
			// decided by the IDE root underneath a live overlay. Its
			// exit signal has to survive both tutorial frames.
			require.False(t, e2eFeed(t, h, ":quit<enter>"),
				"quit must open the confirm prompt before exiting")
			e2eWaitFor(t, h, "Yes")

			assert.True(t, e2eFeed(t, h, "y"),
				"answering Yes must exit the IDE")
		})

	t.Run("tutorial finishing does not exit the IDE", func(t *testing.T) {
		t.Parallel()
		h, i := newQuitE2EIDE(t)
		t.Cleanup(func() { _ = i.Close() })

		require.False(t, e2eFeed(t, h, ":tutorial<space>start<space>basics<enter>"))
		e2eWaitFor(t, h, "hello there")

		require.False(t, e2eFeed(t, h, "<enter>"),
			"advancing a tutorial step must not exit the IDE")
		e2eWaitFor(t, h, "almost done")

		require.False(t, e2eFeed(t, h, "<enter>"),
			"the tutorial's final step must not exit the IDE")
		// The script's finish publishes an interrupt in production;
		// the overlay is cleared when the event loop delivers it.
		e2eWaitGone(t, h, "almost done")

		// The IDE is still alive and still quittable.
		require.False(t, e2eFeed(t, h, "<m-q>"))
		e2eWaitFor(t, h, "Yes")
		assert.True(t, e2eFeed(t, h, "y"))
	})
}
