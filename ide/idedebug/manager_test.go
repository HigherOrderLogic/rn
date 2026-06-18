// Copyright 2026 Unstable Build, LLC.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the
// Free Software Foundation, either version 3 of the License, or (at your
// option) any later version.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
// General Public License for more details.
//
// See <https://www.gnu.org/licenses/> for a copy of the license.

package idedebug

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/go-dap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/debugapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// fakeSubscriber is a no-op EventSubscriber for tests that do
// not exercise the event stream.
type fakeSubscriber struct{}

func (fakeSubscriber) OnEvent(dap.EventMessage) {}
func (fakeSubscriber) OnClose(string)           {}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	uri, err := workspaceapi.ParseURI("file:///tmp/test")
	require.NoError(t, err)
	return New(uri, nil, nil, Config{})
}

// TestSessionIDRequired verifies that every method returns
// ErrSessionNotFound when called with an unknown session ID.
func TestSessionIDRequired(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	t.Cleanup(func() { _ = m.Close() })

	ctx := context.Background()
	_, err := m.Threads(ctx, "missing")
	assert.ErrorIs(t, err, debugapi.ErrSessionNotFound)

	_, err = m.Continue(ctx, "missing", &dap.ContinueArguments{})
	assert.ErrorIs(t, err, debugapi.ErrSessionNotFound)

	err = m.Terminate(ctx, "missing", &dap.TerminateArguments{})
	assert.ErrorIs(t, err, debugapi.ErrSessionNotFound)
}

// TestCreateSessionNoAdapter verifies CreateSession errors with
// ErrNoAdapterConfigured when the requested langID is not in
// Config.Adapters.
func TestCreateSessionNoAdapter(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	t.Cleanup(func() { _ = m.Close() })

	_, _, err := m.CreateSession(context.Background(), "unknown",
		debugapi.ClientCapabilities{}, fakeSubscriber{})
	assert.True(t, errors.Is(err, debugapi.ErrNoAdapterConfigured),
		"expected ErrNoAdapterConfigured, got %v", err)
}

// TestManagerCloseTerminatesAdapterAndGoroutines locks in the RUNE-180
// contract that Manager.Close cancels the manager ctx, cancels each
// session ctx (so exec.CommandContext kills the adapter subprocess),
// and waits for watchSession goroutines to exit.
func TestManagerCloseTerminatesAdapterAndGoroutines(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)

	// Inject a session mirroring startSession's invariants instead of
	// launching a real DAP adapter (which would need a fake binary):
	// a *debugServer parented on m.ctx with an open conn, a watcher
	// channel, and a watchSession goroutine tracked by m.wg.
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })

	watcher := make(chan error, 1)
	srv := newDebugServer(m.ctx, debugConfig{langID: "test", command: "test"},
		"/bin/test", nil, m.rootURI,
		debugapi.ClientCapabilities{}, fakeSubscriber{})
	srv.conn = a
	srv.watcher = watcher
	srv.alive = true

	const sessionID = "test-session"
	m.mu.Lock()
	m.sessions[sessionID] = srv
	m.mu.Unlock()

	m.wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer m.wg.Done()
		defer close(done)
		m.watchSession(sessionID, &srv.cfg, srv)
	}()

	select {
	case <-done:
		t.Fatal("watchSession exited before Close")
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, m.Close())

	require.Error(t, m.ctx.Err(),
		"Manager.Close must cancel m.ctx")
	require.Error(t, srv.ctx.Err(),
		"Manager.Close must cancel each debugServer ctx so the "+
			"adapter subprocess is killed")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchSession goroutine did not exit after Manager.Close")
	}
}

// TestManagerConcurrentSessionAccess drives the session-map accessors
// (sessionFor via the debugapi methods, removeSession, direct inserts)
// and Close concurrently to prove m.mu serialises every read and write
// of m.sessions. It is a -race regression guard: if any path touches
// m.sessions without the lock, the detector fires.
func TestManagerConcurrentSessionAccess(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)

	const workers = 8
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range workers {
		sessionID := newTestSessionID(i)
		wg.Go(func() {
			for range 50 {
				srv := newDebugServer(m.ctx, debugConfig{langID: "test", command: "test"},
					"/bin/test", nil, m.rootURI,
					debugapi.ClientCapabilities{}, fakeSubscriber{})
				m.mu.Lock()
				m.sessions[sessionID] = srv
				m.mu.Unlock()

				_, _ = m.Threads(ctx, sessionID)
				_ = m.Terminate(ctx, "missing", &dap.TerminateArguments{})

				m.removeSession(sessionID, srv)
			}
		})
	}

	wg.Go(func() {
		_ = m.Close()
	})

	wg.Wait()
}

func newTestSessionID(i int) string {
	return "session-" + string(rune('a'+i))
}

// TestSubstituteAddr asserts that adapter argv placeholders expand
// to the bound endpoint: {addr} to host:port and {host}/{port} to
// the split components.
func TestSubstituteAddr(t *testing.T) {
	t.Parallel()
	t.Run("addr placeholder", func(t *testing.T) {
		t.Parallel()
		got := substituteAddr([]string{"dap", "--listen={addr}"}, "127.0.0.1:5555")
		assert.Equal(t, []string{"dap", "--listen=127.0.0.1:5555"}, got)
	})
	t.Run("host and port placeholders", func(t *testing.T) {
		t.Parallel()
		got := substituteAddr(
			[]string{"-m", "debugpy.adapter", "--host", "{host}", "--port", "{port}"},
			"127.0.0.1:5555")
		assert.Equal(t,
			[]string{"-m", "debugpy.adapter", "--host", "127.0.0.1", "--port", "5555"},
			got)
	})
}

// captureRequestArgs injects a session whose cfg carries the given
// launch/attach templates, invokes fn (Launch or Attach), reads the
// single DAP request it writes over the wire, and returns its parsed
// Arguments object together with the request command.
func captureRequestArgs(
	t *testing.T, cfg debugConfig,
	fn func(m *Manager, sessionID string) error,
) (command string, args map[string]any) {
	t.Helper()
	m := newTestManager(t)
	t.Cleanup(func() { _ = m.Close() })

	ideEnd, adapterEnd := net.Pipe()
	t.Cleanup(func() { _ = ideEnd.Close(); _ = adapterEnd.Close() })

	srv := newDebugServer(m.ctx, cfg, "/bin/test", nil, m.rootURI,
		debugapi.ClientCapabilities{}, fakeSubscriber{})
	srv.conn = ideEnd
	srv.alive = true

	const sessionID = "capture-session"
	m.mu.Lock()
	m.sessions[sessionID] = srv
	m.mu.Unlock()

	errCh := make(chan error, 1)
	go func() { errCh <- fn(m, sessionID) }()

	reader := bufio.NewReader(adapterEnd)
	msg, err := dap.ReadProtocolMessage(reader)
	require.NoError(t, err)
	require.NoError(t, <-errCh)

	switch req := msg.(type) {
	case *dap.LaunchRequest:
		require.NoError(t, json.Unmarshal(req.Arguments, &args))
		return req.Command, args
	case *dap.AttachRequest:
		require.NoError(t, json.Unmarshal(req.Arguments, &args))
		return req.Command, args
	default:
		t.Fatalf("unexpected request type: %T", msg)
		return "", nil
	}
}

// TestLaunchArgs asserts that Launch builds the DAP launch payload
// from the configured template (with placeholder substitution and
// typed overlays). With no template, only the SDK-typed overlays are
// sent; adapter-specific keys (e.g. Delve's mode/outputMode) come
// from the language package's debugger config.
func TestLaunchArgs(t *testing.T) {
	t.Parallel()
	launchReq := func(args debugapi.LaunchRequestArguments) func(*Manager, string) error {
		return func(m *Manager, sessionID string) error {
			return m.Launch(context.Background(), sessionID, args)
		}
	}

	t.Run("delve template from config", func(t *testing.T) {
		t.Parallel()
		cfg := debugConfig{
			langID:  "go",
			command: "dlv",
			launchArgs: map[string]string{
				"mode":       "debug",
				"outputMode": "remote",
			},
		}
		cmd, args := captureRequestArgs(t, cfg,
			launchReq(debugapi.LaunchRequestArguments{
				Program:     "/tmp/app",
				Args:        []string{"-v"},
				Cwd:         "/tmp",
				Env:         map[string]string{"K": "V"},
				StopOnEntry: true,
			}))
		assert.Equal(t, "launch", cmd)
		assert.Equal(t, map[string]any{
			"mode":        "debug",
			"outputMode":  "remote",
			"program":     "/tmp/app",
			"cwd":         "/tmp",
			"stopOnEntry": true,
			"noDebug":     false,
			"args":        []any{"-v"},
			"env":         map[string]any{"K": "V"},
		}, args)
	})

	t.Run("no template sends only typed overlays", func(t *testing.T) {
		t.Parallel()
		cmd, args := captureRequestArgs(t, debugConfig{langID: "go", command: "dlv"},
			launchReq(debugapi.LaunchRequestArguments{
				Program:     "/tmp/app",
				StopOnEntry: true,
			}))
		assert.Equal(t, "launch", cmd)
		// With no template only the SDK-typed overlays are present;
		// no host-side adapter defaults (mode/outputMode) are injected.
		assert.Equal(t, map[string]any{
			"program":     "/tmp/app",
			"stopOnEntry": true,
			"noDebug":     false,
		}, args)
	})

	t.Run("debugpy template with overlays", func(t *testing.T) {
		t.Parallel()
		cfg := debugConfig{
			langID:  "python",
			command: "python",
			launchArgs: map[string]string{
				"request": "launch",
				"console": "internalConsole",
				"type":    "python",
			},
		}
		cmd, args := captureRequestArgs(t, cfg,
			launchReq(debugapi.LaunchRequestArguments{
				Program:     "/tmp/main.py",
				Args:        []string{"--flag"},
				Cwd:         "/work",
				Env:         map[string]string{"PYTHONPATH": "/x"},
				StopOnEntry: false,
			}))
		assert.Equal(t, "launch", cmd)
		// Template keys plus typed overlays; no Delve-only defaults
		// (mode/outputMode) leak into the debugpy payload.
		assert.Equal(t, map[string]any{
			"request":     "launch",
			"console":     "internalConsole",
			"type":        "python",
			"program":     "/tmp/main.py",
			"cwd":         "/work",
			"stopOnEntry": false,
			"noDebug":     false,
			"args":        []any{"--flag"},
			"env":         map[string]any{"PYTHONPATH": "/x"},
		}, args)
	})

	t.Run("placeholder substitution", func(t *testing.T) {
		t.Parallel()
		cfg := debugConfig{
			langID:     "python",
			command:    "python",
			launchArgs: map[string]string{"program": "{program}"},
		}
		cmd, args := captureRequestArgs(t, cfg,
			launchReq(debugapi.LaunchRequestArguments{Program: "/tmp/main.py"}))
		assert.Equal(t, "launch", cmd)
		// {program} in the template is substituted, then the typed
		// overlay sets the same key to the identical value.
		assert.Equal(t, map[string]any{
			"program":     "/tmp/main.py",
			"stopOnEntry": false,
			"noDebug":     false,
		}, args)
	})
}

// TestAttachArgs asserts that Attach builds the DAP attach payload
// from the configured template. With no template, only the
// SDK-typed overlays (processId/program) are sent; adapter-specific
// keys (e.g. Delve's mode) come from the language package config.
func TestAttachArgs(t *testing.T) {
	t.Parallel()
	attachReq := func(args debugapi.AttachRequestArguments) func(*Manager, string) error {
		return func(m *Manager, sessionID string) error {
			return m.Attach(context.Background(), sessionID, args)
		}
	}

	t.Run("delve template from config", func(t *testing.T) {
		t.Parallel()
		cfg := debugConfig{
			langID:     "go",
			command:    "dlv",
			attachArgs: map[string]string{"mode": "local"},
		}
		cmd, args := captureRequestArgs(t, cfg,
			attachReq(debugapi.AttachRequestArguments{PID: 4321, Program: "/tmp/app"}))
		assert.Equal(t, "attach", cmd)
		assert.Equal(t, map[string]any{
			"mode":      "local",
			"processId": float64(4321),
			"program":   "/tmp/app",
		}, args)
	})

	t.Run("no template sends only typed overlays", func(t *testing.T) {
		t.Parallel()
		cmd, args := captureRequestArgs(t, debugConfig{langID: "go", command: "dlv"},
			attachReq(debugapi.AttachRequestArguments{PID: 4321, Program: "/tmp/app"}))
		assert.Equal(t, "attach", cmd)
		// No template: only the SDK-typed overlays, with no host-side
		// adapter defaults (mode) injected.
		assert.Equal(t, map[string]any{
			"processId": float64(4321),
			"program":   "/tmp/app",
		}, args)
	})

	t.Run("debugpy template with overlays", func(t *testing.T) {
		t.Parallel()
		cfg := debugConfig{
			langID:  "python",
			command: "python",
			attachArgs: map[string]string{
				"request": "attach",
				"type":    "python",
			},
		}
		cmd, args := captureRequestArgs(t, cfg,
			attachReq(debugapi.AttachRequestArguments{PID: 99}))
		assert.Equal(t, "attach", cmd)
		// Program is empty so no program overlay is added.
		assert.Equal(t, map[string]any{
			"request":   "attach",
			"type":      "python",
			"processId": float64(99),
		}, args)
	})

	t.Run("dotted keys nest into objects", func(t *testing.T) {
		t.Parallel()
		// debugpy attach requires a nested connect object; the flat
		// template expresses it via dotted keys.
		cfg := debugConfig{
			langID:  "python",
			command: "python",
			attachArgs: map[string]string{
				"request":      "attach",
				"type":         "python",
				"connect.host": "127.0.0.1",
				"connect.port": "5688",
			},
		}
		cmd, args := captureRequestArgs(t, cfg,
			attachReq(debugapi.AttachRequestArguments{}))
		assert.Equal(t, "attach", cmd)
		assert.Equal(t, map[string]any{
			"request": "attach",
			"type":    "python",
			"connect": map[string]any{
				"host": "127.0.0.1",
				"port": "5688",
			},
		}, args)
	})
}
