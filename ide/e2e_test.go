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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide"
)

// hostScheduleNextTick mirrors a host event loop's UserFunc dispatch:
// fn runs on a fresh goroutine while holding mu, exactly like
// gui.Update does before invoking ev.UserFunc(). Tests use this to
// preserve the production contract that scheduled callbacks observe
// IDE state under the host lock.
func hostScheduleNextTick(mu sync.Locker) func(func()) bool {
	return func(fn func()) bool {
		go func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		}()
		return true
	}
}

// schedTracker wraps a scheduler with a pending-callback counter so
// the test can wait for every queued scheduled callback to actually
// run, not just the IDE flusher's onDone. The IDE flusher's
// WaitInflight only blocks for awaiter goroutines; text/component's
// dispatchFlush is scheduled independently and otherwise has no
// observable completion handle in tests.
type schedTracker struct {
	inner   func(func()) bool
	muCount sync.Mutex
	cond    *sync.Cond
	pending int
}

func newSchedTracker(inner func(func()) bool) *schedTracker {
	s := &schedTracker{inner: inner}
	s.cond = sync.NewCond(&s.muCount)
	return s
}

func (s *schedTracker) Schedule(fn func()) bool {
	s.muCount.Lock()
	s.pending++
	s.muCount.Unlock()
	return s.inner(func() {
		defer func() {
			s.muCount.Lock()
			s.pending--
			if s.pending == 0 {
				s.cond.Broadcast()
			}
			s.muCount.Unlock()
		}()
		fn()
	})
}

func (s *schedTracker) Wait() {
	s.muCount.Lock()
	for s.pending > 0 {
		s.cond.Wait()
	}
	s.muCount.Unlock()
}

// e2eLockedHandler mirrors the way the production event loop drives
// the IDE handler: every Handle/Draw acquires mu before delegating
// and releases it afterwards so scheduled callbacks (spawned by
// hostScheduleNextTick) can run in between. After each Handle, the
// wrapper waits for in-flight async saves/reloads so the next
// Draw/Handle observes the post-completion state.
type e2eLockedHandler struct {
	tui.Handler
	mu    *sync.Mutex
	ide   *ide.IDE
	sched *schedTracker
}

func (h e2eLockedHandler) Handle(ev term.Event) (bool, bool) {
	h.mu.Lock()
	quit, handled := h.Handler.Handle(ev)
	h.mu.Unlock()
	h.ide.WaitInflight()
	// WaitInflight only waits for the IDE flusher's awaiter
	// goroutines. text/component.dispatchFlush is scheduled
	// independently through the same scheduler and would otherwise
	// race the next Draw — wait for the scheduler to drain too.
	if h.sched != nil {
		h.sched.Wait()
	}
	return quit, handled
}

func (h e2eLockedHandler) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Draw(w)
}

func (h e2eLockedHandler) Resize(width, height int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Resize(width, height)
}

func (h e2eLockedHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Cursor()
}

func (h e2eLockedHandler) Selection() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Selection()
}

func TestE2E(t *testing.T) {
	t.Parallel()
	t.Run("sed arg substitution and single quote grouping works", func(t *testing.T) {
		t.Parallel()
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		config, err := os.CreateTemp(dir, "bcd")
		require.NoError(t, err)

		file, err := os.Create(filepath.Join(dir, "e2e.go"))
		require.NoError(t, err)

		// Drop a tiny test-local sed wrapper into the workspace dir so
		// the alias does not depend on whether gsed/GNU sed happens
		// to be installed on the host. /usr/bin/sed -i.bak is portable
		// between BSD (macOS) and GNU sed.
		sedStub := filepath.Join(dir, "sedstub")
		require.NoError(t, os.WriteFile(sedStub, []byte(
			"#!/bin/sh\n"+
				"/usr/bin/sed -i.bak \"$2\" \"$3\"\n"+
				"rm -f \"$3.bak\"\n",
		), 0o755))

		_, err = config.Seek(0, 0)
		require.NoError(t, err)
		_, err = config.WriteString(fmt.Sprintf(`
editor:
  mode: modal
command:
  key: ":"
  aliases:
    sed: "!! %s -i $1 %%"
`, sedStub))
		require.NoError(t, err)

		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		var mu sync.Mutex
		tracker := newSchedTracker(hostScheduleNextTick(&mu))
		i, err := ide.New(dir, config.Name(), dir,
			ide.WithLocker(&mu),
			ide.WithScheduleNextTick(tracker.Schedule),
			ide.WithPublishEvent(func(term.Event) bool { return true }),
		)
		require.NoError(t, err)

		handler := i.Ready()
		// addWorkspace is async; wait for the cwd workspace install
		// to land before driving keyboard input or opening files.
		i.WaitWorkspaces()
		mu.Lock()
		require.NoError(t, i.Open(uri))
		mu.Unlock()
		cases := []handlertest.SequenceTestCase{
			{"ia<space>bc<space>abc<space>ab<space>c<space>abc<esc>:write<enter>",
				`┌━━━━━━━━──────────┐
│o e2e.go          │
├──────────────────┤
│a bc abc ab c ab▐ │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
			{":sed<space>'s/c<space>a/C<space>A/g'<enter>" +
				":notificationcloseall<enter>:reloadfile!<enter>",
				`┌━━━━━━━━──────────┐
│o e2e.go          │
├──────────────────┤
│a bC AbC Ab C Ab▐ │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		}

		// Drive the sequence through a wrapper that locks per
		// Handle/Draw and drains in-flight async saves/reloads
		// between turns. Holding mu across the whole sequence
		// would deadlock with the host scheduler (which spawns
		// goroutines that acquire mu themselves).
		handlertest.RunHandlerSequence(t, e2eLockedHandler{
			Handler: handler, mu: &mu, ide: i, sched: tracker,
		}, 20, 10, cases)
	})

	t.Run("plugins executed via ! and !! get auth env vars", func(t *testing.T) {
		t.Parallel()
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		config, err := os.CreateTemp(dir, "bcd")
		require.NoError(t, err)

		_, err = config.Seek(0, 0)
		require.NoError(t, err)
		_, err = config.WriteString(`
editor:
  mode: modal
command:
  key: ":"
`)
		require.NoError(t, err)

		var mu sync.Mutex
		runner, err := extensionv2.NewRunner(context.Background(),
			&mu, dir,
			extensionv2.WithSocketEnv("IDETEST_SOCKET"),
			extensionv2.WithDataDirEnv("IDETEST_DATADIR"),
			extensionv2.WithAuthCertEnv("IDETEST_CERT"),
			extensionv2.WithAuthTokenEnv("IDETEST_TOKEN"),
		)
		require.NoError(t, err)
		i, err := ide.New(dir, config.Name(), dir,
			ide.WithExtensionsRunner(runner),
			ide.WithLocker(&mu),
			ide.WithPublishEvent(func(term.Event) bool { return true }),
		)
		require.NoError(t, err)

		handler := i.Ready()

		filename1, err := filepath.Abs(filepath.Join(dir, "ide.env"))
		require.NoError(t, err)
		file1, err := os.Create(filename1)
		require.NoError(t, err)
		require.NoError(t, file1.Close())

		filename2, err := filepath.Abs(filepath.Join(dir, "ide2.env"))
		require.NoError(t, err)
		file2, err := os.Create(filename2)
		require.NoError(t, err)
		require.NoError(t, file2.Close())

		// allow for extension servers to be ready
		time.Sleep(5 * time.Second)

		keys, err := term.ParseKeys(
			fmt.Sprintf(`:!<space>sh<space>-c<space>"env<space>|grep<space>IDETEST<space>|<space>tee<space>%s"<enter>`, filename1) +
				fmt.Sprintf(`:!!<space>sh<space>-c<space>"env<space>|grep<space>IDETEST<space>|<space>tee<space>%s"<enter>`, filename2),
		)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		for _, key := range keys {
			_, handled := handler.Handle(term.Event{Ch: key.Ch, Mod: key.Mod, Key: key.Key, Type: term.EventKey})
			require.True(t, handled, key.String())
		}

		assertAuthVarsPresent(t, filename1)
		assertAuthVarsPresent(t, filename2)
	})

	t.Run("plugins executed via ! and !! cwd is the workspce", func(t *testing.T) {
		t.Parallel()
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		config, err := os.CreateTemp(dir, "bcd")
		require.NoError(t, err)

		_, err = config.Seek(0, 0)
		require.NoError(t, err)
		_, err = config.WriteString(`
editor:
  mode: modal
command:
  key: ":"
`)
		require.NoError(t, err)

		var mu sync.Mutex
		runner, err := extensionv2.NewRunner(context.Background(),
			&mu, dir,
		)
		require.NoError(t, err)
		i, err := ide.New(dir, config.Name(), dir,
			ide.WithExtensionsRunner(runner),
			ide.WithLocker(&mu),
			ide.WithScheduleNextTick(hostScheduleNextTick(&mu)),
			ide.WithPublishEvent(func(term.Event) bool { return true }),
		)
		require.NoError(t, err)

		handler := i.Ready()
		i.WaitWorkspaces()

		filename1, err := filepath.Abs(filepath.Join(dir, "ide.cwd"))
		require.NoError(t, err)
		file1, err := os.Create(filename1)
		require.NoError(t, err)
		require.NoError(t, file1.Close())

		// allow for extension runner to be ready
		time.Sleep(5 * time.Second)

		keys, err := term.ParseKeys(
			fmt.Sprintf(`:!<space>sh<space>-c<space>"pwd<space>|<space>tee<space>%s"<enter>`, filename1),
		)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		for _, key := range keys {
			_, handled := handler.Handle(term.Event{Ch: key.Ch, Mod: key.Mod, Key: key.Key, Type: term.EventKey})
			require.True(t, handled, key.String())
		}

		assertCwdVar(t, filename1, dir)
	})
}

func assertCwdVar(t *testing.T, filename string, expectedValue string) {
	t.Helper()

	var actual string
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(filename)
		if err != nil {
			return false
		}
		actual = strings.TrimSuffix(strings.Trim(string(data), " "), "\n")
		return actual == expectedValue
	}, 5*time.Second, 50*time.Millisecond,
		"expected %q in %s, got %q", expectedValue, filename, actual)
}

func assertAuthVarsPresent(t *testing.T, filename string) {
	t.Helper()

	var vars []string
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(filename)
		if err != nil {
			return false
		}
		content := strings.TrimSuffix(strings.Trim(string(data), " "), "\n")
		if content == "" {
			vars = nil
		} else {
			vars = strings.Split(content, "\n")
		}
		return len(vars) == 4
	}, 5*time.Second, 50*time.Millisecond,
		"expected auth env vars in %s, got %v", filename, vars)

	assert.Equal(t, 4, len(vars))
	for _, v := range vars {
		kv := strings.Split(v, "=")
		switch kv[0] {
		case "IDETEST_SOCKET",
			"IDETEST_DATADIR",
			"IDETEST_TOKEN":
			require.Len(t, kv, 2)
			assert.NotZero(t, kv[1])
		case "IDETEST_CERT":
		default:
			t.Errorf("extraneous idetest env var: %q", kv[0])
		}
	}
}
