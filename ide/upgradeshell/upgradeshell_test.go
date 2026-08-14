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

package upgradeshell

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/ide/ideupgrade"
)

// promptWindowManager answers the upgrade prompt by feeding the given
// key to the floating handler as soon as it is shown. A zero key
// leaves the prompt unanswered.
type promptWindowManager struct {
	key rune
}

func (m *promptWindowManager) Floating(
	h browserapi.Floating, _ browserapi.FloatingConfig,
) (browserapi.Window, error) {
	if m.key != 0 {
		h.Resize(80, 20)
		h.Handle(term.Event{Type: term.EventKey, Ch: m.key})
	}
	return nil, nil
}

func (m *promptWindowManager) Focus() (browserapi.Window, error) { return nil, nil }
func (m *promptWindowManager) Split(
	browserapi.Orientation, browserapi.Window, browserapi.Handler,
) (browserapi.Window, error) {
	return nil, nil
}
func (m *promptWindowManager) Bar(browserapi.BarConfig, tui.Handler) error { return nil }
func (m *promptWindowManager) Tab(
	workspaceapi.URI, rune, string, browserapi.Handler,
) (browserapi.Handler, error) {
	return nil, nil
}
func (m *promptWindowManager) SetWindowContent(browserapi.Window, browserapi.Handler) error {
	return nil
}
func (m *promptWindowManager) CloseWindow(browserapi.Window) error { return nil }

type recordingProgressWriter struct {
	mu    sync.Mutex
	units []string
}

func (w *recordingProgressWriter) Progress(_, _ int64, units string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.units = append(w.units, units)
}

func (w *recordingProgressWriter) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, len(w.units))
	copy(out, w.units)
	return out
}

// newHandler stands up a Handler backed by a TLS manifest endpoint
// that responds with status and body.
func newHandler(
	t *testing.T, status int, body any, key rune,
) *Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/"+runtime.GOOS+"-"+runtime.GOARCH+"/manifest.json",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			if body != nil {
				require.NoError(t, json.NewEncoder(w).Encode(body))
			}
		})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	mgr, err := ideupgrade.New(ideupgrade.Config{
		CurrentVersion:   "v0.1.0",
		Arch:             runtime.GOOS + "-" + runtime.GOARCH,
		ManifestURL:      srv.URL,
		Storage:          storagestub.NewInMemoryService(),
		HTTPClient:       srv.Client(),
		WindowManager:    &promptWindowManager{key: key},
		ScheduleNextTick: func(fn func()) bool { fn(); return true },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	return New(Config{Manager: mgr})
}

func availableManifest() map[string]any {
	return map[string]any{
		"version":  "v9.9.9",
		"url":      "https://example.invalid/rune-v9.9.9.tar.gz",
		"sha256":   "abc123",
		"filename": "rune-v9.9.9.tar.gz",
	}
}

func TestHandleCommand(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    any
		key     rune
		args    []string
		want    string
		wantErr string
	}{
		{
			name:   "up to date",
			status: http.StatusNotFound,
			want:   "Rune is up to date (**v0.1.0**)",
		},
		{
			name:    "check fails",
			status:  http.StatusInternalServerError,
			wantErr: "status 500",
		},
		{
			name:   "remind later",
			status: http.StatusOK,
			body:   availableManifest(),
			key:    'l',
			want:   "Rune **v9.9.9** is available. Reminder postponed.",
		},
		{
			name:   "skip version",
			status: http.StatusOK,
			body:   availableManifest(),
			key:    's',
			want:   "Skipping Rune **v9.9.9**.",
		},
		{
			name:   "help",
			status: http.StatusNotFound,
			args:   []string{"help"},
			want:   usageMarkdown(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(t, tc.status, tc.body, tc.key)
			pw := &recordingProgressWriter{}
			got, err := h.upgrade(context.Background(),
				repl.Command{Name: CommandName, Args: tc.args}, pw)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestHandleCommandReportsCheckProgress pins the console feedback the
// user gets while the manifest fetch is in flight: without it the
// command looks hung for the duration of the request.
func TestHandleCommandReportsCheckProgress(t *testing.T) {
	h := newHandler(t, http.StatusNotFound, nil, 0)
	pw := &recordingProgressWriter{}
	_, err := h.upgrade(context.Background(),
		repl.Command{Name: CommandName}, pw)
	require.NoError(t, err)
	require.Equal(t, []string{"checking for updates"}, pw.snapshot())
}

// TestHandleCommandUpgradeNowForwardsProgress covers the "Upgrade Now"
// branch. The test binary is not part of a managed install, so the
// upgrade is refused before any download — which is enough to prove
// the branch reaches Manager.Upgrade with the console's writer.
func TestHandleCommandUpgradeNowForwardsProgress(t *testing.T) {
	h := newHandler(t, http.StatusOK, availableManifest(), 'y')
	pw := &recordingProgressWriter{}
	_, err := h.upgrade(context.Background(),
		repl.Command{Name: CommandName}, pw)
	require.Error(t, err)
	var notSupported *ideupgrade.ErrUpgradeNotSupported
	require.ErrorAs(t, err, &notSupported)
}

func TestNewPanicsWithoutManager(t *testing.T) {
	require.Panics(t, func() { New(Config{}) })
}

func TestHandleCommandReturnsOutputComponent(t *testing.T) {
	h := newHandler(t, http.StatusNotFound, nil, 0)
	it, err := h.HandleCommand(context.Background(),
		repl.Command{Name: CommandName}, repl.NopProgressWriter())
	require.NoError(t, err)
	v, ok := it.Next(context.Background())
	require.True(t, ok)
	require.NotNil(t, v)
}
