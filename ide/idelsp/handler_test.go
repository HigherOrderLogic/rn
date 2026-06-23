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

package idelsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestCallbackHandler_ShowMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		msgType       semanticapi.MessageType
		message       string
		expectedLevel browserapi.NotificationLevel
	}{
		{
			name:          "error message",
			msgType:       semanticapi.MessageTypeError,
			message:       "something failed",
			expectedLevel: browserapi.LevelError,
		},
		{
			name:          "warning message",
			msgType:       semanticapi.MessageTypeWarning,
			message:       "might be wrong",
			expectedLevel: browserapi.LevelWarn,
		},
		{
			name:          "info message",
			msgType:       semanticapi.MessageTypeInfo,
			message:       "all good",
			expectedLevel: browserapi.LevelInfo,
		},
		{
			name:          "log message",
			msgType:       semanticapi.MessageTypeLog,
			message:       "debug info",
			expectedLevel: browserapi.LevelInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			notif := &mockNotifications{}
			h := NewCallbackHandler(
				notif, nil, nil, nil, nil,
				"",
				CallbackHandlerConfig{},
			)
			ctx := ContextWithMetadata(
				t.Context(),
				Metadata{ServerName: "test-server"},
			)
			err := h.ShowMessage(ctx,
				semanticapi.ShowMessageParams{
					Type:    tt.msgType,
					Message: tt.message,
				},
			)
			require.NoError(t, err)
			require.Len(t, notif.notified, 1)
			assert.Equal(t,
				tt.expectedLevel, notif.notified[0].level,
			)
			assert.Equal(t,
				"test-server: "+tt.message,
				notif.notified[0].msg,
			)
		})
	}

	t.Run("no metadata omits prefix", func(t *testing.T) {
		t.Parallel()
		notif := &mockNotifications{}
		h := NewCallbackHandler(
			notif, nil, nil, nil, nil,
			"",
			CallbackHandlerConfig{},
		)
		err := h.ShowMessage(t.Context(),
			semanticapi.ShowMessageParams{
				Type:    semanticapi.MessageTypeInfo,
				Message: "bare message",
			},
		)
		require.NoError(t, err)
		require.Len(t, notif.notified, 1)
		assert.Equal(t,
			"bare message", notif.notified[0].msg,
		)
	})
}

func TestCallbackHandler_LogMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		msgType semanticapi.MessageType
		message string
	}{
		{
			name:    "error",
			msgType: semanticapi.MessageTypeError,
			message: "err msg",
		},
		{
			name:    "warning",
			msgType: semanticapi.MessageTypeWarning,
			message: "warn msg",
		},
		{
			name:    "info",
			msgType: semanticapi.MessageTypeInfo,
			message: "info msg",
		},
		{
			name:    "debug",
			msgType: semanticapi.MessageTypeDebug,
			message: "debug msg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewCallbackHandler(
				nil, nil, nil, nil, nil,
				"",
				CallbackHandlerConfig{},
			)
			err := h.LogMessage(t.Context(),
				semanticapi.LogMessageParams{
					Type:    tt.msgType,
					Message: tt.message,
				},
			)
			require.NoError(t, err)
		})
	}
}

func TestCallbackHandler_PublishDiagnostics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		diagnostics       []semanticapi.Diagnostic
		expectedLocations []textapi.Location
		expectedPriority  textapi.LocationPriority
	}{
		{
			name:             "empty clears list",
			diagnostics:      nil,
			expectedPriority: textapi.LocationPriorityInfo,
		},
		{
			name: "single error",
			diagnostics: []semanticapi.Diagnostic{
				{
					Range: semanticapi.Range{
						Start: semanticapi.Position{
							Line: 5, Character: 0,
						},
						End: semanticapi.Position{
							Line: 5, Character: 10,
						},
					},
					Severity: semanticapi.DiagnosticSeverityError,
					Message:  "undefined variable",
				},
			},
			expectedLocations: []textapi.Location{
				{
					From:    term.Coordinates{X: 0, Y: 5},
					To:      term.Coordinates{X: 10, Y: 5},
					Message: "undefined variable",
					Attr: term.Attributes{
						Bg: term.ColorRed,
					},
					Icon: "✖",
				},
			},
			expectedPriority: textapi.LocationPriorityError,
		},
		{
			name: "mixed severities uses highest",
			diagnostics: []semanticapi.Diagnostic{
				{
					Range: semanticapi.Range{
						Start: semanticapi.Position{
							Line: 1, Character: 0,
						},
						End: semanticapi.Position{
							Line: 1, Character: 5,
						},
					},
					Severity: semanticapi.DiagnosticSeverityWarning,
					Message:  "unused var",
				},
				{
					Range: semanticapi.Range{
						Start: semanticapi.Position{
							Line: 3, Character: 0,
						},
						End: semanticapi.Position{
							Line: 3, Character: 8,
						},
					},
					Severity: semanticapi.DiagnosticSeverityError,
					Message:  "syntax error",
				},
			},
			expectedLocations: []textapi.Location{
				{
					From:    term.Coordinates{X: 0, Y: 1},
					To:      term.Coordinates{X: 5, Y: 1},
					Message: "unused var",
					Attr: term.Attributes{
						Bg: term.ColorYellow,
					},
					Icon: "▲",
				},
				{
					From:    term.Coordinates{X: 0, Y: 3},
					To:      term.Coordinates{X: 8, Y: 3},
					Message: "syntax error",
					Attr: term.Attributes{
						Bg: term.ColorRed,
					},
					Icon: "✖",
				},
			},
			expectedPriority: textapi.LocationPriorityError,
		},
		{
			name: "compiler inline diagnostic classified",
			diagnostics: []semanticapi.Diagnostic{
				{
					Range: semanticapi.Range{
						Start: semanticapi.Position{
							Line: 10, Character: 0,
						},
						End: semanticapi.Position{
							Line: 10, Character: 3,
						},
					},
					Severity: semanticapi.DiagnosticSeverityInformation,
					Source:   "compiler",
					Message:  "can inline Add",
				},
			},
			expectedLocations: []textapi.Location{
				{
					From:    term.Coordinates{X: 0, Y: 10},
					To:      term.Coordinates{X: 3, Y: 10},
					Message: "Inline: can inline Add",
					Icon:    "󰁔",
					Attr: term.Attributes{
						Bg: term.GetColor("indigo"),
					},
				},
			},
			expectedPriority: textapi.LocationPriorityInfo,
		},
		{
			name: "compiler escape diagnostic classified",
			diagnostics: []semanticapi.Diagnostic{
				{
					Range: semanticapi.Range{
						Start: semanticapi.Position{
							Line: 5, Character: 2,
						},
						End: semanticapi.Position{
							Line: 5, Character: 8,
						},
					},
					Severity: semanticapi.DiagnosticSeverityInformation,
					Source:   "compiler",
					Message:  "a escapes to heap",
				},
			},
			expectedLocations: []textapi.Location{
				{
					From:    term.Coordinates{X: 2, Y: 5},
					To:      term.Coordinates{X: 8, Y: 5},
					Message: "Escape: a escapes to heap",
					Icon:    "󰁝",
					Attr: term.Attributes{
						Bg: term.GetColor("darkmagenta"),
					},
				},
			},
			expectedPriority: textapi.LocationPriorityInfo,
		},
		{
			name: "compiler error severity not classified",
			diagnostics: []semanticapi.Diagnostic{
				{
					Range: semanticapi.Range{
						Start: semanticapi.Position{
							Line: 1, Character: 0,
						},
						End: semanticapi.Position{
							Line: 1, Character: 5,
						},
					},
					Severity: semanticapi.DiagnosticSeverityError,
					Source:   "compiler",
					Message:  "cannot inline: too complex",
				},
			},
			expectedLocations: []textapi.Location{
				{
					From:    term.Coordinates{X: 0, Y: 1},
					To:      term.Coordinates{X: 5, Y: 1},
					Message: "cannot inline: too complex",
					Attr: term.Attributes{
						Bg: term.ColorRed,
					},
					Icon: "✖",
				},
			},
			expectedPriority: textapi.LocationPriorityError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uri, err := workspaceapi.ParseURI("file:///tmp/test.go")
			require.NoError(t, err)
			ed := &mockEditor{
				handler: &mockEditorHandler{uri: uri},
			}
			h := NewCallbackHandler(
				nil, nil, nil, ed, nil,
				"",
				CallbackHandlerConfig{},
			)
			err = h.PublishDiagnostics(t.Context(),
				semanticapi.PublishDiagnosticsParams{
					URI:         "file:///tmp/test.go",
					Diagnostics: tt.diagnostics,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, "lsp-diagnostics", ed.locID)
			assert.Equal(t, tt.expectedPriority, ed.locPriority)
			assert.Equal(t, tt.expectedLocations, ed.locations)
		})
	}
}

// TestCallbackHandler_PublishDiagnostics_BySource asserts that
// diagnostics published by distinct servers for the same URI
// accumulate (rather than overwrite) in the central cache and are
// merged into a single "lsp-diagnostics" location list. This is the
// multi-server (e.g. ty + ruff) payoff: each backend's set stays
// independently overridable, but they coexist in one list so
// next/prev-diagnostic navigation treats them uniformly.
func TestCallbackHandler_PublishDiagnostics_BySource(t *testing.T) {
	t.Parallel()
	uri, err := workspaceapi.ParseURI("file:///tmp/main.py")
	require.NoError(t, err)
	ed := &mockEditor{handler: &mockEditorHandler{uri: uri}}
	h := NewCallbackHandler(nil, nil, nil, ed, nil, "", CallbackHandlerConfig{})

	tyDiag := semanticapi.Diagnostic{
		Range: semanticapi.Range{
			Start: semanticapi.Position{Line: 1, Character: 0},
			End:   semanticapi.Position{Line: 1, Character: 4},
		},
		Severity: semanticapi.DiagnosticSeverityError,
		Source:   "ty",
		Message:  "type error",
	}
	ruffDiag := semanticapi.Diagnostic{
		Range: semanticapi.Range{
			Start: semanticapi.Position{Line: 2, Character: 0},
			End:   semanticapi.Position{Line: 2, Character: 6},
		},
		Severity: semanticapi.DiagnosticSeverityWarning,
		Source:   "ruff",
		Message:  "unused import",
	}

	tyCtx := ContextWithMetadata(t.Context(), Metadata{ServerName: "ty"})
	ruffCtx := ContextWithMetadata(t.Context(), Metadata{ServerName: "ruff"})

	require.NoError(t, h.PublishDiagnostics(tyCtx, semanticapi.PublishDiagnosticsParams{
		URI:         "file:///tmp/main.py",
		Diagnostics: []semanticapi.Diagnostic{tyDiag},
	}))
	require.NoError(t, h.PublishDiagnostics(ruffCtx, semanticapi.PublishDiagnosticsParams{
		URI:         "file:///tmp/main.py",
		Diagnostics: []semanticapi.Diagnostic{ruffDiag},
	}))

	// The merged snapshot must contain both servers' diagnostics.
	got := h.Diagnostics()
	require.Len(t, got["file:///tmp/main.py"], 2)
	assert.ElementsMatch(t,
		[]semanticapi.Diagnostic{tyDiag, ruffDiag},
		got["file:///tmp/main.py"])

	// Both servers' diagnostics land in one merged "lsp-diagnostics"
	// location list, never under per-server source ids.
	ed.mu.Lock()
	merged := ed.locationsByID["lsp-diagnostics"]
	_, hasTyID := ed.locationsByID["lsp-diagnostics:ty"]
	_, hasRuffID := ed.locationsByID["lsp-diagnostics:ruff"]
	ed.mu.Unlock()
	assert.False(t, hasTyID, "must not use a per-server source id")
	assert.False(t, hasRuffID, "must not use a per-server source id")
	// The merged list carries both fully-rendered locations: ty's
	// error (red/✖) and ruff's warning (yellow/▲). Map iteration order
	// across servers is unspecified, so match the set, not a sequence.
	wantTyLoc := textapi.Location{
		From:    term.Coordinates{X: 0, Y: 1},
		To:      term.Coordinates{X: 4, Y: 1},
		Message: "type error",
		Attr:    term.Attributes{Bg: term.ColorRed},
		Icon:    "✖",
	}
	wantRuffLoc := textapi.Location{
		From:    term.Coordinates{X: 0, Y: 2},
		To:      term.Coordinates{X: 6, Y: 2},
		Message: "unused import",
		Attr:    term.Attributes{Bg: term.ColorYellow},
		Icon:    "▲",
	}
	assert.ElementsMatch(t,
		[]textapi.Location{wantTyLoc, wantRuffLoc}, merged)

	// Clearing one server's diagnostics must leave the other intact.
	require.NoError(t, h.PublishDiagnostics(tyCtx, semanticapi.PublishDiagnosticsParams{
		URI:         "file:///tmp/main.py",
		Diagnostics: nil,
	}))
	got = h.Diagnostics()
	require.Len(t, got["file:///tmp/main.py"], 1)
	assert.Equal(t, ruffDiag, got["file:///tmp/main.py"][0])

	// The merged location list must now contain only ruff's entry.
	ed.mu.Lock()
	mergedAfterClear := ed.locationsByID["lsp-diagnostics"]
	ed.mu.Unlock()
	assert.Equal(t, []textapi.Location{wantRuffLoc}, mergedAfterClear)
}

func TestCallbackHandler_PublishDiagnostics_IconConfig(t *testing.T) {
	t.Parallel()
	uri, err := workspaceapi.ParseURI("file:///tmp/test.go")
	require.NoError(t, err)
	ed := &mockEditor{
		handler: &mockEditorHandler{uri: uri},
	}
	h := NewCallbackHandler(
		nil, nil, nil, ed, nil,
		"",
		CallbackHandlerConfig{
			Icons: IconSet{
				IconDiagnosticError:       "E",
				IconDiagnosticWarning:     "W",
				IconDiagnosticInformation: "I",
				IconDiagnosticHint:        "H",
				IconCompilerInline:        ">",
			},
		},
	)
	err = h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
		URI: "file:///tmp/test.go",
		Diagnostics: []semanticapi.Diagnostic{
			{
				Range: semanticapi.Range{
					End: semanticapi.Position{Character: 1},
				},
				Severity: semanticapi.DiagnosticSeverityError,
				Message:  "error",
			},
			{
				Range: semanticapi.Range{
					End: semanticapi.Position{Character: 1},
				},
				Severity: semanticapi.DiagnosticSeverityWarning,
				Message:  "warning",
			},
			{
				Range: semanticapi.Range{
					End: semanticapi.Position{Character: 1},
				},
				Severity: semanticapi.DiagnosticSeverityInformation,
				Message:  "info",
			},
			{
				Range: semanticapi.Range{
					End: semanticapi.Position{Character: 1},
				},
				Severity: semanticapi.DiagnosticSeverityHint,
				Message:  "hint",
			},
			{
				Range: semanticapi.Range{
					End: semanticapi.Position{Character: 1},
				},
				Severity: semanticapi.DiagnosticSeverityInformation,
				Source:   "compiler",
				Message:  "can inline Add",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, ed.locations, 5)
	assert.Equal(t, "E", ed.locations[0].Icon)
	assert.Equal(t, "W", ed.locations[1].Icon)
	assert.Equal(t, "I", ed.locations[2].Icon)
	assert.Equal(t, "H", ed.locations[3].Icon)
	assert.Equal(t, ">", ed.locations[4].Icon)
}

func TestCallbackHandler_Diagnostics(t *testing.T) {
	t.Parallel()

	uri, err := workspaceapi.ParseURI("file:///tmp/test.go")
	require.NoError(t, err)
	ed := &mockEditor{handler: &mockEditorHandler{uri: uri}}
	h := NewCallbackHandler(
		nil, nil, nil, ed, nil, "", CallbackHandlerConfig{},
	)

	// Empty source initially.
	assert.Empty(t, h.Diagnostics())

	// Publish diagnostics for one URI.
	diags := []semanticapi.Diagnostic{
		{
			Range: semanticapi.Range{
				Start: semanticapi.Position{Line: 1, Character: 0},
				End:   semanticapi.Position{Line: 1, Character: 5},
			},
			Severity: semanticapi.DiagnosticSeverityError,
			Message:  "boom",
		},
	}
	err = h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
		URI:         "file:///tmp/test.go",
		Diagnostics: diags,
	})
	require.NoError(t, err)
	got := h.Diagnostics()
	require.Len(t, got, 1)
	require.Len(t, got["file:///tmp/test.go"], 1)
	assert.Equal(t, "boom", got["file:///tmp/test.go"][0].Message)

	// Mutating the returned snapshot must not leak into internal state.
	got["file:///tmp/test.go"][0].Message = "tampered"
	delete(got, "file:///tmp/test.go")
	got2 := h.Diagnostics()
	require.Len(t, got2, 1)
	assert.Equal(t, "boom", got2["file:///tmp/test.go"][0].Message)

	// Publishing an empty slice clears the entry for that URI.
	err = h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
		URI:         "file:///tmp/test.go",
		Diagnostics: nil,
	})
	require.NoError(t, err)
	assert.Empty(t, h.Diagnostics())
}

func TestCallbackHandler_WaitFileProcessed(t *testing.T) {
	t.Parallel()

	newHandler := func() *CallbackHandler {
		return NewCallbackHandler(
			nil, nil, nil, nil, nil,
			"",
			CallbackHandlerConfig{
				ScheduleNextTick: func(func()) bool {
					return true
				},
			},
		)
	}

	t.Run("no pending changes returns immediately", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		require.NoError(t, h.WaitFileProcessed(t.Context(), "file:///tmp/test.go"))
	})

	t.Run("matching version unblocks wait", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"
		h.FileDidChange(uri, 2, true, false)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		select {
		case err := <-done:
			t.Fatalf("wait returned too early: %v", err)
		case <-time.After(20 * time.Millisecond):
		}

		err := h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 2,
		})
		require.NoError(t, err)

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for matching diagnostics version")
		}
	})

	t.Run("stale version does not unblock newer wait", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"
		h.FileDidChange(uri, 2, true, false)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		err := h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 1,
		})
		require.NoError(t, err)

		select {
		case err := <-done:
			t.Fatalf("wait returned on stale version: %v", err)
		case <-time.After(20 * time.Millisecond):
		}

		err = h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 2,
		})
		require.NoError(t, err)

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for current diagnostics version")
		}
	})

	t.Run("context cancellation stops wait", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"
		h.FileDidChange(uri, 2, true, false)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(ctx, uri)
		}()

		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for cancellation")
		}
	})

	t.Run("unversioned change waits for any diagnostics push", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"
		h.FileDidChange(uri, 0, false, true)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		select {
		case err := <-done:
			t.Fatalf("wait returned too early: %v", err)
		case <-time.After(20 * time.Millisecond):
		}

		// A diagnostics push with version 0 should unblock.
		err := h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 0,
		})
		require.NoError(t, err)

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for unversioned diagnostics")
		}
	})

	t.Run("unversioned change unblocked by versioned diagnostics", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"
		h.FileDidChange(uri, 0, false, true)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		select {
		case err := <-done:
			t.Fatalf("wait returned too early: %v", err)
		case <-time.After(20 * time.Millisecond):
		}

		// A versioned diagnostics push should also clear unversioned.
		err := h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 5,
		})
		require.NoError(t, err)

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for diagnostics")
		}
	})

	t.Run("fallback timeout expires when no diagnostics arrive", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"
		h.FileDidChange(uri, 0, false, true)

		// Use a context without a deadline to exercise the fallback.
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := h.WaitFileProcessed(ctx, uri)
		elapsed := time.Since(start)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		// Should have respected the caller's shorter timeout,
		// not the 5s fallback.
		require.Less(t, elapsed, 500*time.Millisecond)
	})

	t.Run("tracked file OOB change blocks on stale push until newer version", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"

		// Bring the file to a fully processed open state at version 1.
		h.FileDidChange(uri, 1, true, false)
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 1,
		}))
		require.NoError(t, h.WaitFileProcessed(t.Context(), uri))

		// Out-of-band (watched-file) change on the still-open file.
		h.FileDidChange(uri, 1, true, true)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		// A stale push at the already-awaited version must not release.
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 1,
		}))
		select {
		case err := <-done:
			t.Fatalf("wait returned on stale push: %v", err)
		case <-time.After(30 * time.Millisecond):
		}

		// The IDE reload sends a strictly newer versioned didChange,
		// and gopls re-publishes for it.
		h.FileDidChange(uri, 2, true, false)
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 2,
		}))
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for newer version after OOB change")
		}
	})

	t.Run("untracked file OOB change releases on first push", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"

		// No prior sent version: a watched-file change on a closed file.
		h.FileDidChange(uri, 0, false, true)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		select {
		case err := <-done:
			t.Fatalf("wait returned too early: %v", err)
		case <-time.After(20 * time.Millisecond):
		}

		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 0,
		}))
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for first push on untracked OOB change")
		}
	})

	t.Run("open file with no tracked version uses version floor", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"

		// An open-but-unedited file: the editor open path never calls
		// FileDidChange, so the handler has no entry, yet gopls has
		// already published diagnostics for the open version (1).
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 1,
		}))

		// Out-of-band change reports the editor's current version (1)
		// as the floor to surpass.
		h.FileDidChange(uri, 1, true, true)

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uri)
		}()

		// A stale re-publish for the version gopls already holds must
		// NOT release the wait.
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 1,
		}))
		select {
		case err := <-done:
			t.Fatalf("wait returned on stale push at version floor: %v", err)
		case <-time.After(30 * time.Millisecond):
		}

		// The reload's versioned didChange + its diagnostics push
		// surpasses the floor and releases the wait.
		h.FileDidChange(uri, 2, true, false)
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 2,
		}))
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for resync past version floor")
		}
	})

	t.Run("tracked file OOB change returns on caller deadline without newer version", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uri := "file:///tmp/test.go"

		h.FileDidChange(uri, 1, true, false)
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uri,
			Version: 1,
		}))
		h.FileDidChange(uri, 1, true, true)

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := h.WaitFileProcessed(ctx, uri)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), 500*time.Millisecond)
	})

	t.Run("InvalidateAllPending blocks wait for unrelated URI until next publish", func(t *testing.T) {
		t.Parallel()
		h := newHandler()
		uA := "file:///tmp/a.go"
		uB := "file:///tmp/b.go"

		// Register both URIs and bring them to a fully processed state
		// so a plain WaitFileProcessed would return immediately.
		h.FileDidChange(uA, 1, true, false)
		h.FileDidChange(uB, 1, true, false)
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uA,
			Version: 1,
		}))
		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uB,
			Version: 1,
		}))
		// Sanity: without invalidation the wait returns immediately.
		require.NoError(t, h.WaitFileProcessed(t.Context(), uB))

		// Invalidate all tracked URIs (simulates a delete event in
		// DidChangeWatchedFiles).
		h.InvalidateAllPending()

		done := make(chan error, 1)
		go func() {
			done <- h.WaitFileProcessed(context.Background(), uB)
		}()

		// The wait must block until the next publishDiagnostics push
		// for uB. A push for an unrelated URI must not release it.
		select {
		case err := <-done:
			t.Fatalf("wait returned too early: %v", err)
		case <-time.After(50 * time.Millisecond):
		}

		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uA,
			Version: 2,
		}))
		select {
		case err := <-done:
			t.Fatalf("wait returned after unrelated push: %v", err)
		case <-time.After(20 * time.Millisecond):
		}

		require.NoError(t, h.PublishDiagnostics(t.Context(), semanticapi.PublishDiagnosticsParams{
			URI:     uB,
			Version: 2,
		}))
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for diagnostics after invalidation")
		}
	})
}

func TestClassifyCompilerDiagnostic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		msg      string
		wantIcon string
		wantMsg  string
		wantAttr term.Attributes
	}{
		{
			msg: "can inline Add", wantIcon: "󰁔",
			wantMsg:  "Inline: can inline Add",
			wantAttr: term.Attributes{Bg: term.GetColor("indigo")},
		},
		{
			msg: "inlining call to Add", wantIcon: "󰁔",
			wantMsg:  "Inline: inlining call to Add",
			wantAttr: term.Attributes{Bg: term.GetColor("indigo")},
		},
		{
			msg: "a escapes to heap", wantIcon: "󰁝",
			wantMsg:  "Escape: a escapes to heap",
			wantAttr: term.Attributes{Bg: term.GetColor("darkmagenta")},
		},
		{
			msg: "moved to heap: x", wantIcon: "󰁝",
			wantMsg:  "Escape: moved to heap: x",
			wantAttr: term.Attributes{Bg: term.GetColor("darkmagenta")},
		},
		{
			msg: "leaking param: x", wantIcon: "󰁝",
			wantMsg:  "Escape: leaking param: x",
			wantAttr: term.Attributes{Bg: term.GetColor("darkmagenta")},
		},
		{
			msg: "a does not escape", wantIcon: "󰁝",
			wantMsg:  "Escape: a does not escape",
			wantAttr: term.Attributes{Bg: term.GetColor("darkmagenta")},
		},
		{
			msg: "Found IsInBounds", wantIcon: "󰅪",
			wantMsg:  "Bounds: Found IsInBounds",
			wantAttr: term.Attributes{Bg: term.GetColor("rebeccapurple")},
		},
		{
			msg: "isInBounds", wantIcon: "󰅪",
			wantMsg:  "Bounds: isInBounds",
			wantAttr: term.Attributes{Bg: term.GetColor("rebeccapurple")},
		},
		{
			msg: "nilcheck", wantIcon: "∅",
			wantMsg:  "Nilcheck: nilcheck",
			wantAttr: term.Attributes{Bg: term.GetColor("blueviolet")},
		},
		{
			msg: "unknown compiler message", wantIcon: "⚙",
			wantMsg:  "unknown compiler message",
			wantAttr: term.Attributes{Bg: term.GetColor("darkslateblue")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			t.Parallel()
			icon, msg, attr := classifyCompilerDiagnostic(tt.msg, DefaultIconSet())
			assert.Equal(t, tt.wantIcon, icon)
			assert.Equal(t, tt.wantMsg, msg)
			assert.Equal(t, tt.wantAttr, attr)
		})
	}
}

func TestCallbackHandler_Progress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(*CallbackHandler)
		token       semanticapi.ProgressToken
		value       string
		checkNotif  func(*testing.T, *mockNotifications)
		checkUpdate func(*testing.T, *mockNotifications)
	}{
		{
			name: "begin creates notification",
			token: semanticapi.ProgressToken{
				StringValue: "tok1",
			},
			value: `{"kind":"begin","title":"Loading"}`,
			checkNotif: func(
				t *testing.T, n *mockNotifications,
			) {
				require.Len(t, n.notified, 1)
				assert.Equal(t,
					browserapi.LevelInfo,
					n.notified[0].level,
				)
				assert.Equal(t,
					"Loading", n.notified[0].msg,
				)
			},
		},
		{
			name: "begin with message appends",
			token: semanticapi.ProgressToken{
				StringValue: "tok2",
			},
			value: `{"kind":"begin","title":"Build","message":"starting"}`,
			checkNotif: func(
				t *testing.T, n *mockNotifications,
			) {
				require.Len(t, n.notified, 1)
				assert.Equal(t,
					"Build: starting",
					n.notified[0].msg,
				)
			},
		},
		{
			name: "report updates progress",
			setup: func(h *CallbackHandler) {
				h.mu.Lock()
				h.progress["tok3"] = "notif-1"
				h.mu.Unlock()
			},
			token: semanticapi.ProgressToken{
				StringValue: "tok3",
			},
			value: `{"kind":"report","message":"50%","percentage":50}`,
			checkUpdate: func(
				t *testing.T, n *mockNotifications,
			) {
				require.Len(t, n.progUpds, 1)
				assert.Equal(t,
					"notif-1", n.progUpds[0].id,
				)
				assert.Equal(t,
					int64(50), n.progUpds[0].progress,
				)
			},
		},
		{
			name: "end removes and completes",
			setup: func(h *CallbackHandler) {
				h.mu.Lock()
				h.progress["tok4"] = "notif-2"
				h.mu.Unlock()
			},
			token: semanticapi.ProgressToken{
				StringValue: "tok4",
			},
			value: `{"kind":"end"}`,
			checkUpdate: func(
				t *testing.T, n *mockNotifications,
			) {
				require.Len(t, n.progUpds, 1)
				assert.Equal(t,
					int64(100), n.progUpds[0].progress,
				)
			},
		},
		{
			name: "integer token",
			token: semanticapi.ProgressToken{
				IntegerValue: 42,
				IsInteger:    true,
			},
			value: `{"kind":"begin","title":"Indexing"}`,
			checkNotif: func(
				t *testing.T, n *mockNotifications,
			) {
				require.Len(t, n.notified, 1)
				assert.Equal(t,
					"Indexing", n.notified[0].msg,
				)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			notif := &mockNotifications{}
			h := NewCallbackHandler(
				notif, nil, nil, nil, nil,
				"",
				CallbackHandlerConfig{},
			)
			if tt.setup != nil {
				tt.setup(h)
			}
			err := h.Progress(t.Context(),
				semanticapi.ProgressParams{
					Token: tt.token,
					Value: json.RawMessage(tt.value),
				},
			)
			require.NoError(t, err)
			if tt.checkNotif != nil {
				tt.checkNotif(t, notif)
			}
			if tt.checkUpdate != nil {
				tt.checkUpdate(t, notif)
			}
		})
	}
}

func TestCallbackHandler_LogTrace(t *testing.T) {
	t.Parallel()
	h := NewCallbackHandler(
		nil, nil, nil, nil, nil,
		"",
		CallbackHandlerConfig{},
	)
	err := h.LogTrace(t.Context(),
		semanticapi.LogTraceParams{
			Message: "trace msg",
			Verbose: "verbose detail",
		},
	)
	require.NoError(t, err)
}

func TestCallbackHandler_ShowDocument(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		params     semanticapi.ShowDocumentParams
		hasEditor  bool
		wantCursor *term.Coordinates
	}{
		{
			name: "opens resource and notifies",
			params: semanticapi.ShowDocumentParams{
				URI: "file:///tmp/foo.go",
			},
		},
		{
			name: "sets cursor on selection",
			params: semanticapi.ShowDocumentParams{
				URI: "file:///tmp/foo.go",
				Selection: &semanticapi.Range{
					Start: semanticapi.Position{
						Line: 10, Character: 5,
					},
					End: semanticapi.Position{
						Line: 10, Character: 15,
					},
				},
			},
			hasEditor: true,
			wantCursor: &term.Coordinates{
				X: 5, Y: 10,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			notif := &mockNotifications{}
			opener := &mockResourceOpener{}
			uri, err := workspaceapi.ParseURI(tt.params.URI)
			require.NoError(t, err)
			var ed Editor
			if tt.hasEditor {
				ed = &mockEditor{
					handler: &mockEditorHandler{uri: uri},
				}
			}
			h := NewCallbackHandler(
				notif, nil, opener, ed, nil,
				"",
				CallbackHandlerConfig{},
			)
			ctx := ContextWithMetadata(
				t.Context(),
				Metadata{ServerName: "test-server"},
			)
			result, err := h.ShowDocument(ctx, tt.params)
			require.NoError(t, err)
			assert.True(t, result.Success)
			require.Len(t, notif.notified, 1)
			assert.Contains(t,
				notif.notified[0].msg, "test-server: see",
			)
			if tt.wantCursor != nil {
				me := ed.(*mockEditor)
				require.NotNil(t, me.cursorSet)
				assert.Equal(t, *tt.wantCursor, *me.cursorSet)
			}
		})
	}
}

func TestCallbackHandler_ShowDocument_HTTP(t *testing.T) {
	t.Parallel()

	t.Run("HTTP URL opens floating HTML handler", func(t *testing.T) {
		t.Parallel()
		wm := &mockWindowManager{}
		opener := &mockResourceOpener{}
		h := NewCallbackHandler(
			&mockNotifications{}, wm, opener, nil, nil,
			"",
			CallbackHandlerConfig{},
		)
		result, err := h.ShowDocument(t.Context(),
			semanticapi.ShowDocumentParams{
				URI: "http://localhost:8080/asm",
			},
		)
		require.NoError(t, err)
		assert.True(t, result.Success)
		wm.mu.Lock()
		assert.Equal(t, 1, wm.floatingCalls)
		wm.mu.Unlock()
		opener.mu.Lock()
		assert.Empty(t, opener.opened)
		opener.mu.Unlock()
	})

	t.Run("HTTPS URL opens floating HTML handler", func(t *testing.T) {
		t.Parallel()
		wm := &mockWindowManager{}
		opener := &mockResourceOpener{}
		h := NewCallbackHandler(
			&mockNotifications{}, wm, opener, nil, nil,
			"",
			CallbackHandlerConfig{},
		)
		result, err := h.ShowDocument(t.Context(),
			semanticapi.ShowDocumentParams{
				URI: "https://pkg.go.dev/fmt",
			},
		)
		require.NoError(t, err)
		assert.True(t, result.Success)
		wm.mu.Lock()
		assert.Equal(t, 1, wm.floatingCalls)
		wm.mu.Unlock()
		opener.mu.Lock()
		assert.Empty(t, opener.opened)
		opener.mu.Unlock()
	})

	t.Run("file URL uses resource opener", func(t *testing.T) {
		t.Parallel()
		wm := &mockWindowManager{}
		opener := &mockResourceOpener{}
		notif := &mockNotifications{}
		h := NewCallbackHandler(
			notif, wm, opener, nil, nil,
			"",
			CallbackHandlerConfig{},
		)
		ctx := ContextWithMetadata(
			t.Context(),
			Metadata{ServerName: "gopls"},
		)
		result, err := h.ShowDocument(ctx,
			semanticapi.ShowDocumentParams{
				URI: "file:///tmp/foo.go",
			},
		)
		require.NoError(t, err)
		assert.True(t, result.Success)
		wm.mu.Lock()
		assert.Equal(t, 0, wm.floatingCalls)
		wm.mu.Unlock()
		opener.mu.Lock()
		assert.Len(t, opener.opened, 1)
		opener.mu.Unlock()
	})
}

func TestCallbackHandler_WorkDoneProgressCreate(
	t *testing.T,
) {
	t.Parallel()
	tests := []struct {
		name  string
		token semanticapi.ProgressToken
		key   string
	}{
		{
			name: "string token",
			token: semanticapi.ProgressToken{
				StringValue: "my-token",
			},
			key: "my-token",
		},
		{
			name: "integer token",
			token: semanticapi.ProgressToken{
				IntegerValue: 99,
				IsInteger:    true,
			},
			key: "99",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewCallbackHandler(
				nil, nil, nil, nil, nil,
				"",
				CallbackHandlerConfig{},
			)
			err := h.WorkDoneProgressCreate(
				t.Context(),
				semanticapi.WorkDoneProgressCreateParams{
					Token: tt.token,
				},
			)
			require.NoError(t, err)
			h.mu.Lock()
			_, ok := h.progress[tt.key]
			h.mu.Unlock()
			assert.True(t, ok)
		})
	}
}

func TestCallbackHandler_ApplyEdit(t *testing.T) {
	t.Parallel()
	uri, err := workspaceapi.ParseURI("file:///tmp/test.go")
	require.NoError(t, err)

	tests := []struct {
		name          string
		params        semanticapi.ApplyWorkspaceEditParams
		applied       bool
		expectedEdits []mockCellEdit
	}{
		{
			name: "empty edit succeeds",
			params: semanticapi.ApplyWorkspaceEditParams{
				Edit: semanticapi.WorkspaceEdit{},
			},
			applied: true,
		},
		{
			name: "changes applies text edits",
			params: semanticapi.ApplyWorkspaceEditParams{
				Edit: semanticapi.WorkspaceEdit{
					Changes: map[string][]semanticapi.TextEdit{
						"file:///tmp/test.go": {
							{
								Range: semanticapi.Range{
									Start: semanticapi.Position{
										Line: 0, Character: 0,
									},
									End: semanticapi.Position{
										Line: 0, Character: 3,
									},
								},
								NewText: "hello",
							},
						},
					},
				},
			},
			applied: true,
			expectedEdits: []mockCellEdit{
				{
					start: term.Coordinates{X: 0, Y: 0},
					end:   term.Coordinates{X: 3, Y: 0},
					text:  "hello",
				},
			},
		},
		{
			name: "document changes with text edit",
			params: semanticapi.ApplyWorkspaceEditParams{
				Edit: semanticapi.WorkspaceEdit{
					DocumentChanges: []semanticapi.DocumentChange{
						{
							TextDocumentEdit: &semanticapi.TextDocumentEdit{
								TextDocument: semanticapi.VersionedTextDocumentIdentifier{
									URI: "file:///tmp/test.go",
								},
								Edits: []semanticapi.TextEdit{
									{
										Range: semanticapi.Range{
											Start: semanticapi.Position{
												Line: 1, Character: 0,
											},
											End: semanticapi.Position{
												Line: 1, Character: 5,
											},
										},
										NewText: "world",
									},
								},
							},
						},
					},
				},
			},
			applied: true,
			expectedEdits: []mockCellEdit{
				{
					start: term.Coordinates{X: 0, Y: 1},
					end:   term.Coordinates{X: 5, Y: 1},
					text:  "world",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ed := &mockEditor{
				handler: &mockEditorHandler{uri: uri},
			}
			h := NewCallbackHandler(
				nil, nil, nil, ed,
				newTestScheme(),
				"",
				CallbackHandlerConfig{},
			)
			result, err := h.ApplyEdit(t.Context(), tt.params)
			require.NoError(t, err)
			assert.Equal(t, tt.applied, result.Applied)
			assert.Equal(t, tt.expectedEdits, ed.cellEdits)
		})
	}
}

// TestCallbackHandler_ApplyEdit_OpenFileUsesCellEditor verifies that
// when the target file is already open in the editor, applyEdit routes
// the edits through the cell editor and does NOT write to disk (the
// editor owns persistence) nor open the resource.
func TestCallbackHandler_ApplyEdit_OpenFileUsesCellEditor(t *testing.T) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "open-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	path := filepath.Join(tmpDir, "main.go")
	uriStr := "file://" + path
	initial := "package main\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0644))

	uri, err := workspaceapi.ParseURI(uriStr)
	require.NoError(t, err)

	cases := []struct {
		name   string
		params semanticapi.ApplyWorkspaceEditParams
		want   mockCellEdit
	}{
		{
			name: "document changes",
			params: semanticapi.ApplyWorkspaceEditParams{
				Edit: semanticapi.WorkspaceEdit{
					DocumentChanges: []semanticapi.DocumentChange{{
						TextDocumentEdit: &semanticapi.TextDocumentEdit{
							TextDocument: semanticapi.VersionedTextDocumentIdentifier{URI: uriStr},
							Edits: []semanticapi.TextEdit{{
								Range: semanticapi.Range{
									Start: semanticapi.Position{Line: 0, Character: 0},
									End:   semanticapi.Position{Line: 0, Character: 7},
								},
								NewText: "pkg",
							}},
						},
					}},
				},
			},
			want: mockCellEdit{
				start: term.Coordinates{X: 0, Y: 0},
				end:   term.Coordinates{X: 7, Y: 0},
				text:  "pkg",
			},
		},
		{
			name: "changes map",
			params: semanticapi.ApplyWorkspaceEditParams{
				Edit: semanticapi.WorkspaceEdit{
					Changes: map[string][]semanticapi.TextEdit{
						uriStr: {{
							Range: semanticapi.Range{
								Start: semanticapi.Position{Line: 2, Character: 0},
								End:   semanticapi.Position{Line: 2, Character: 4},
							},
							NewText: "type",
						}},
					},
				},
			},
			want: mockCellEdit{
				start: term.Coordinates{X: 0, Y: 2},
				end:   term.Coordinates{X: 4, Y: 2},
				text:  "type",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(initial), 0644))
			ed := &mockEditor{handler: &mockEditorHandler{uri: uri}}
			opener := &mockResourceOpener{}
			h := NewCallbackHandler(
				nil, nil, opener, ed,
				newTestScheme(), "",
				CallbackHandlerConfig{},
			)
			result, err := h.ApplyEdit(t.Context(), tc.params)
			require.NoError(t, err)
			assert.True(t, result.Applied)

			assert.Equal(t, []mockCellEdit{tc.want}, ed.cellEdits)

			opener.mu.Lock()
			assert.Empty(t, opener.opened, "open file must not be re-opened")
			opener.mu.Unlock()

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, initial, string(got),
				"open file must not be written behind the editor buffer")
		})
	}
}

// TestCallbackHandler_ApplyEdit_ClosedFileWritesDisk verifies that when
// the target file is NOT open, applyEdit applies the edits directly on
// disk via the file system without opening the resource or touching a
// cell editor — avoiding a dirty buffer racing the FS watcher.
func TestCallbackHandler_ApplyEdit_ClosedFileWritesDisk(t *testing.T) {
	t.Parallel()
	goModURITmpl := "file://%s"
	initial := "module example.com/x\n\ngo 1.21\n\nrequire foo v1.0.0\n"
	want := "module example.com/x\n\ngo 1.21\n\nrequire foo v1.2.3\n"

	upgradeEdit := semanticapi.TextEdit{
		Range: semanticapi.Range{
			Start: semanticapi.Position{Line: 4, Character: 12},
			End:   semanticapi.Position{Line: 4, Character: 18},
		},
		NewText: "v1.2.3",
	}

	build := func(uriStr string) []semanticapi.ApplyWorkspaceEditParams {
		return []semanticapi.ApplyWorkspaceEditParams{
			{Edit: semanticapi.WorkspaceEdit{
				DocumentChanges: []semanticapi.DocumentChange{{
					TextDocumentEdit: &semanticapi.TextDocumentEdit{
						TextDocument: semanticapi.VersionedTextDocumentIdentifier{URI: uriStr},
						Edits:        []semanticapi.TextEdit{upgradeEdit},
					},
				}},
			}},
			{Edit: semanticapi.WorkspaceEdit{
				Changes: map[string][]semanticapi.TextEdit{uriStr: {upgradeEdit}},
			}},
		}
	}
	names := []string{"document changes", "changes map"}
	for i, name := range names {
		t.Run(name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "closed-*")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
			goModPath := filepath.Join(tmpDir, "go.mod")
			require.NoError(t, os.WriteFile(goModPath, []byte(initial), 0644))
			uriStr := fmt.Sprintf(goModURITmpl, goModPath)

			ed := &mockEditor{
				handler:   &mockEditorHandler{},
				editorErr: fmt.Errorf("not open"),
			}
			opener := &mockResourceOpener{}
			h := NewCallbackHandler(
				nil, nil, opener, ed,
				newTestScheme(), "",
				CallbackHandlerConfig{},
			)
			result, err := h.ApplyEdit(t.Context(), build(uriStr)[i])
			require.NoError(t, err)
			assert.True(t, result.Applied)

			assert.Empty(t, ed.cellEdits,
				"closed file must not go through the cell editor")
			opener.mu.Lock()
			assert.Empty(t, opener.opened,
				"closed file must not be force-opened")
			opener.mu.Unlock()

			got, err := os.ReadFile(goModPath)
			require.NoError(t, err)
			assert.Equal(t, want, string(got))
		})
	}
}

func TestCallbackHandler_ApplyEdit_CreateFile(
	t *testing.T,
) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	newFile := filepath.Join(tmpDir, "new.go")

	h := NewCallbackHandler(
		nil, nil, nil, nil, newTestScheme(),
		"",
		CallbackHandlerConfig{},
	)
	result, err := h.ApplyEdit(t.Context(),
		semanticapi.ApplyWorkspaceEditParams{
			Edit: semanticapi.WorkspaceEdit{
				DocumentChanges: []semanticapi.DocumentChange{
					{
						CreateFile: &semanticapi.CreateFile{
							Kind: "create",
							URI:  "file://" + newFile,
						},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	_, err = os.Stat(newFile)
	require.NoError(t, err)
}

func TestCallbackHandler_ApplyEdit_DeleteFile(
	t *testing.T,
) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	target := filepath.Join(tmpDir, "del.go")
	require.NoError(t, os.WriteFile(
		target, []byte("x"), 0644,
	))

	h := NewCallbackHandler(
		nil, nil, nil, nil, newTestScheme(),
		"",
		CallbackHandlerConfig{},
	)
	result, err := h.ApplyEdit(t.Context(),
		semanticapi.ApplyWorkspaceEditParams{
			Edit: semanticapi.WorkspaceEdit{
				DocumentChanges: []semanticapi.DocumentChange{
					{
						DeleteFile: &semanticapi.DeleteFile{
							Kind: "delete",
							URI:  "file://" + target,
						},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	_, err = os.Stat(target)
	require.True(t, os.IsNotExist(err))
}

func TestCallbackHandler_ApplyEdit_RenameFile(
	t *testing.T,
) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	oldFile := filepath.Join(tmpDir, "old.go")
	newFile := filepath.Join(tmpDir, "new.go")
	require.NoError(t, os.WriteFile(
		oldFile, []byte("content"), 0644,
	))

	h := NewCallbackHandler(
		nil, nil, nil, nil, newTestScheme(),
		"",
		CallbackHandlerConfig{},
	)
	result, err := h.ApplyEdit(t.Context(),
		semanticapi.ApplyWorkspaceEditParams{
			Edit: semanticapi.WorkspaceEdit{
				DocumentChanges: []semanticapi.DocumentChange{
					{
						RenameFile: &semanticapi.RenameFile{
							Kind:   "rename",
							OldURI: "file://" + oldFile,
							NewURI: "file://" + newFile,
						},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	_, err = os.Stat(oldFile)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(newFile)
	require.NoError(t, err)
}

func TestCallbackHandler_ApplyEdit_IgnoreIfExists(
	t *testing.T,
) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	existing := filepath.Join(tmpDir, "exists.go")
	require.NoError(t, os.WriteFile(
		existing, []byte("original"), 0644,
	))

	h := NewCallbackHandler(
		nil, nil, nil, nil, newTestScheme(),
		"",
		CallbackHandlerConfig{},
	)
	result, err := h.ApplyEdit(t.Context(),
		semanticapi.ApplyWorkspaceEditParams{
			Edit: semanticapi.WorkspaceEdit{
				DocumentChanges: []semanticapi.DocumentChange{
					{
						CreateFile: &semanticapi.CreateFile{
							Kind: "create",
							URI:  "file://" + existing,
							Options: &semanticapi.CreateFileOptions{
								IgnoreIfExists: true,
							},
						},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	data, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
}

func TestCallbackHandler_ApplyEdit_DeleteIgnoreIfNotExists(
	t *testing.T,
) {
	t.Parallel()
	h := NewCallbackHandler(
		nil, nil, nil, nil, newTestScheme(),
		"",
		CallbackHandlerConfig{},
	)
	result, err := h.ApplyEdit(t.Context(),
		semanticapi.ApplyWorkspaceEditParams{
			Edit: semanticapi.WorkspaceEdit{
				DocumentChanges: []semanticapi.DocumentChange{
					{
						DeleteFile: &semanticapi.DeleteFile{
							Kind: "delete",
							URI:  "file:///nonexistent/path.go",
							Options: &semanticapi.DeleteFileOptions{
								IgnoreIfNotExists: true,
							},
						},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	assert.True(t, result.Applied)
}

func TestCallbackHandler_WorkspaceFolders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rootURI string
		want    []semanticapi.WorkspaceFolder
	}{
		{
			name:    "standard path",
			rootURI: "file:///home/user/project",
			want: []semanticapi.WorkspaceFolder{
				{
					URI:  "file:///home/user/project",
					Name: "project",
				},
			},
		},
		{
			name:    "nested path",
			rootURI: "file:///a/b/c",
			want: []semanticapi.WorkspaceFolder{
				{
					URI:  "file:///a/b/c",
					Name: "c",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewCallbackHandler(
				nil, nil, nil, nil, nil,
				tt.rootURI,
				CallbackHandlerConfig{},
			)
			folders, err := h.WorkspaceFolders(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, folders)
		})
	}
}

func TestCallbackHandler_Configuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		config   config.Config
		items    []semanticapi.ConfigurationItem
		expected []string
	}{
		{
			name:   "nil config returns null",
			config: nil,
			items: []semanticapi.ConfigurationItem{
				{Section: "gopls"},
			},
			expected: []string{"null"},
		},
		{
			name: "returns map as JSON",
			config: config.MapConfig(map[string]any{
				"lsp": map[string]any{
					"servers": map[string]any{
						"gopls": map[string]any{
							"usePlaceholders": true,
						},
					},
				},
			}),
			items: []semanticapi.ConfigurationItem{
				{Section: "gopls"},
			},
			expected: []string{
				`{"usePlaceholders":true}`,
			},
		},
		{
			name: "missing key returns null",
			config: config.MapConfig(map[string]any{
				"lsp": map[string]any{},
			}),
			items: []semanticapi.ConfigurationItem{
				{Section: "missing"},
			},
			expected: []string{"null"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewCallbackHandler(
				nil, nil, nil, nil, nil,
				"",
				CallbackHandlerConfig{
					Config: tt.config,
				},
			)
			results, err := h.Configuration(
				t.Context(),
				semanticapi.ConfigurationParams{
					Items: tt.items,
				},
			)
			require.NoError(t, err)
			require.Len(t, results, len(tt.expected))
			for i, exp := range tt.expected {
				assert.JSONEq(t,
					exp, string(results[i]),
				)
			}
		})
	}
}

func TestCallbackHandler_RegisterUnregister(
	t *testing.T,
) {
	t.Parallel()
	h := NewCallbackHandler(
		nil, nil, nil, nil, nil,
		"",
		CallbackHandlerConfig{},
	)
	assert.NoError(t, h.RegisterCapability(
		t.Context(), semanticapi.RegistrationParams{},
	))
	assert.NoError(t, h.UnregisterCapability(
		t.Context(), semanticapi.UnregistrationParams{},
	))
}

func TestCallbackHandler_Refresh(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		call  func(context.Context, *CallbackHandler) error
		check func(*testing.T, *mockRefresher)
	}{
		{
			name: "CodeLensRefresh",
			call: func(
				ctx context.Context, h *CallbackHandler,
			) error {
				return h.CodeLensRefresh(ctx)
			},
			check: func(t *testing.T, r *mockRefresher) {
				assert.Equal(t, 1, r.codeLens)
			},
		},
		{
			name: "SemanticTokensRefresh",
			call: func(
				ctx context.Context, h *CallbackHandler,
			) error {
				return h.SemanticTokensRefresh(ctx)
			},
			check: func(t *testing.T, r *mockRefresher) {
				assert.Equal(t, 1, r.semantic)
			},
		},
		{
			name: "InlayHintRefresh",
			call: func(
				ctx context.Context, h *CallbackHandler,
			) error {
				return h.InlayHintRefresh(ctx)
			},
			check: func(t *testing.T, r *mockRefresher) {
				assert.Equal(t, 1, r.inlayHints)
			},
		},
		{
			name: "DiagnosticRefresh",
			call: func(
				ctx context.Context, h *CallbackHandler,
			) error {
				return h.DiagnosticRefresh(ctx)
			},
			check: func(t *testing.T, r *mockRefresher) {
				assert.Equal(t, 1, r.diagnostics)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &mockRefresher{}
			h := NewCallbackHandler(
				nil, nil, nil, nil, nil,
				"",
				CallbackHandlerConfig{Refresher: r},
			)
			err := tt.call(t.Context(), h)
			require.NoError(t, err)
			tt.check(t, r)
		})
	}
}

func TestCallbackHandler_NopRefresherDefault(
	t *testing.T,
) {
	t.Parallel()
	h := NewCallbackHandler(
		nil, nil, nil, nil, nil,
		"",
		CallbackHandlerConfig{},
	)
	assert.NoError(t, h.CodeLensRefresh(t.Context()))
	assert.NoError(t,
		h.SemanticTokensRefresh(t.Context()),
	)
	assert.NoError(t, h.InlayHintRefresh(t.Context()))
	assert.NoError(t, h.DiagnosticRefresh(t.Context()))
}

func TestProgressTokenKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		token semanticapi.ProgressToken
		want  string
	}{
		{
			name: "string token",
			token: semanticapi.ProgressToken{
				StringValue: "abc",
			},
			want: "abc",
		},
		{
			name: "integer token",
			token: semanticapi.ProgressToken{
				IntegerValue: 42,
				IsInteger:    true,
			},
			want: "42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t,
				tt.want, progressTokenKey(tt.token),
			)
		})
	}
}

func TestDiagnosticSeverityToAttr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		severity semanticapi.DiagnosticSeverity
		wantBg   term.Color
		wantIcon string
	}{
		{
			name:     "error is red",
			severity: semanticapi.DiagnosticSeverityError,
			wantBg:   term.ColorRed,
			wantIcon: "✖",
		},
		{
			name:     "warning is yellow",
			severity: semanticapi.DiagnosticSeverityWarning,
			wantBg:   term.ColorYellow,
			wantIcon: "▲",
		},
		{
			name:     "info is blue",
			severity: semanticapi.DiagnosticSeverityInformation,
			wantBg:   term.ColorBlue,
			wantIcon: "◉",
		},
		{
			name:     "hint is gray",
			severity: semanticapi.DiagnosticSeverityHint,
			wantBg:   term.ColorGray,
			wantIcon: "󰌵",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			attr, icon := diagnosticSeverityToAttr(tt.severity, DefaultIconSet())
			style := term.Style(attr)
			assert.Equal(t, tt.wantBg, style.Bg)
			assert.Equal(t, tt.wantIcon, icon)
		})
	}
}

func TestClassifyCompilerDiagnostic_Icons(t *testing.T) {
	t.Parallel()
	icons := IconSet{
		IconCompilerInline:   "I",
		IconCompilerEscape:   "E",
		IconCompilerBounds:   "B",
		IconCompilerNilcheck: "N",
		IconCompilerDefault:  "C",
	}
	tests := []struct {
		name string
		msg  string
		icon string
	}{
		{name: "inline", msg: "can inline Add", icon: "I"},
		{name: "escape", msg: "a escapes to heap", icon: "E"},
		{name: "bounds", msg: "Found IsInBounds", icon: "B"},
		{name: "nilcheck", msg: "removed nilcheck", icon: "N"},
		{name: "default", msg: "optimized something", icon: "C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			icon, _, _ := classifyCompilerDiagnostic(tt.msg, icons)
			assert.Equal(t, tt.icon, icon)
		})
	}
}

type mockNotifications struct {
	mu        sync.Mutex
	notified  []mockNotification
	progUpds  []mockProgressUpdate
	nextID    int
	notifyErr error
}

type mockNotification struct {
	level browserapi.NotificationLevel
	msg   string
}

type mockProgressUpdate struct {
	id       string
	message  string
	progress int64
	total    int64
}

func (m *mockNotifications) Notify(
	level browserapi.NotificationLevel,
	msg string, args ...any,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.notifyErr != nil {
		return "", m.notifyErr
	}
	m.nextID++
	id := "notif-" + string(rune('0'+m.nextID))
	formatted := msg
	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}
	m.notified = append(m.notified, mockNotification{
		level: level, msg: formatted,
	})
	return id, nil
}

func (m *mockNotifications) NotifyOnce(
	level browserapi.NotificationLevel,
	msg string, args ...any,
) (string, error) {
	return m.Notify(level, msg, args...)
}

func (m *mockNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progUpds = append(m.progUpds, mockProgressUpdate{
		id: id, message: message,
		progress: progress, total: total,
	})
	return nil
}

type mockEditor struct {
	mu          sync.Mutex
	handler     *mockEditorHandler
	locations   []textapi.Location
	locPriority textapi.LocationPriority
	locID       string
	cursorSet   *term.Coordinates
	cellEdits   []mockCellEdit
	editorErr   error
	locationErr error
	// locationsByID records the latest location list per source id so
	// tests can assert that diagnostics from distinct servers
	// accumulate under distinct source ids.
	locationsByID map[string][]textapi.Location
}

func (m *mockEditor) Editor(
	_ workspaceapi.URI,
) (textapi.Handler, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.editorErr != nil {
		return nil, m.editorErr
	}
	return m.handler, nil
}

func (m *mockEditor) SetLocationList(
	_ textapi.Handler, priority textapi.LocationPriority,
	id string, list textapi.LocationList,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locationErr != nil {
		return m.locationErr
	}
	m.locPriority = priority
	m.locID = id
	m.locations = nil
	if list != nil {
		for loc, ok := list.Current(); ok; loc, ok = list.Next() {
			m.locations = append(m.locations, loc)
		}
	}
	if m.locationsByID == nil {
		m.locationsByID = make(map[string][]textapi.Location)
	}
	m.locationsByID[id] = append([]textapi.Location(nil), m.locations...)
	return nil
}

func (m *mockEditor) SetCursor(
	_ textapi.Handler, c term.Coordinates,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cursorSet = &c
	return nil
}

func (m *mockEditor) CellEditor(
	_ textapi.Handler,
) textapi.CellEditor {
	return &mockCellEditor{editor: m}
}

type mockEditorHandler struct {
	uri workspaceapi.URI
}

func (m *mockEditorHandler) Resource() workspaceapi.URI {
	return m.uri
}

func (m *mockEditorHandler) Resize(_, _ int)    {}
func (m *mockEditorHandler) Draw(_ term.Writer) {}
func (m *mockEditorHandler) Handle(
	_ term.Event,
) (bool, bool) {
	return false, false
}
func (m *mockEditorHandler) Cursor() (
	term.Coordinates, term.CursorStyle, bool,
) {
	return term.Coordinates{}, 0, false
}
func (m *mockEditorHandler) Selection() (string, bool) {
	return "", false
}
func (m *mockEditorHandler) Close() error {
	return nil
}

type mockCellEdit struct {
	start, end term.Coordinates
	text       string
}

type mockCellEditor struct {
	editor *mockEditor
}

func (c *mockCellEditor) Edit(
	_ context.Context,
	start, end term.Coordinates, str string,
) (term.Coordinates, term.Coordinates, string, error) {
	c.editor.mu.Lock()
	defer c.editor.mu.Unlock()
	c.editor.cellEdits = append(
		c.editor.cellEdits,
		mockCellEdit{start: start, end: end, text: str},
	)
	return start, end, "", nil
}

type mockWindowManager struct {
	mu            sync.Mutex
	floatingCalls int
}

func (m *mockWindowManager) Floating(
	_ browserapi.Floating, _ browserapi.FloatingConfig,
) (browserapi.Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.floatingCalls++
	return &mockWindow{}, nil
}

func (m *mockWindowManager) CloseWindow(
	_ browserapi.Window,
) error {
	return nil
}

type mockWindow struct{}

func (m *mockWindow) WindowID() uint64 { return 0 }

type mockResourceOpener struct {
	mu     sync.Mutex
	opened []workspaceapi.URI
	openFn func(workspaceapi.URI)
}

func (m *mockResourceOpener) Open(
	uri workspaceapi.URI,
) (browserapi.Handler, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = append(m.opened, uri)
	if m.openFn != nil {
		m.openFn(uri)
	}
	return nil, nil
}

type mockRefresher struct {
	mu          sync.Mutex
	codeLens    int
	semantic    int
	inlayHints  int
	diagnostics int
}

func (m *mockRefresher) RefreshCodeLens(
	_ context.Context,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codeLens++
	return nil
}

func (m *mockRefresher) RefreshSemanticTokens(
	_ context.Context,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.semantic++
	return nil
}

func (m *mockRefresher) RefreshInlayHints(
	_ context.Context,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inlayHints++
	return nil
}

func (m *mockRefresher) RefreshDiagnostics(
	_ context.Context,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.diagnostics++
	return nil
}
