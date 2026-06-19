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

package main

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
)

// fakeNotifications records notify and progress calls for assertions.
type fakeNotifications struct {
	mu       sync.Mutex
	openID   string
	notifs   []string
	progress []progressSample
	nextID   int
}

type progressSample struct {
	id       string
	message  string
	progress int64
	total    int64
}

func newFakeNotifications() *fakeNotifications {
	return &fakeNotifications{openID: "notif-1", nextID: 1}
}

func (n *fakeNotifications) Notify(_ browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifs = append(n.notifs, msg)
	return n.openID, nil
}

func (n *fakeNotifications) NotifyOnce(level browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	return n.Notify(level, msg, args...)
}

func (n *fakeNotifications) UpdateNotificationProgress(id, message string, progress, total int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.progress = append(n.progress, progressSample{id, message, progress, total})
	return nil
}

func (n *fakeNotifications) lastProgress() progressSample {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.progress) == 0 {
		return progressSample{}
	}
	return n.progress[len(n.progress)-1]
}

func (n *fakeNotifications) progressMessages() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.progress))
	for i, p := range n.progress {
		out[i] = p.message
	}
	return out
}

// assertMonotonicProgress checks the displayed fraction never decreases,
// so the bar cannot move backward across steps with different totals.
func assertMonotonicProgress(t *testing.T, n *fakeNotifications) {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	prev := 0.0
	for _, p := range n.progress {
		require.NotZero(t, p.total)
		frac := float64(p.progress) / float64(p.total)
		assert.GreaterOrEqual(t, frac, prev,
			"progress fraction regressed at %q (%d/%d)", p.message, p.progress, p.total)
		prev = frac
	}
}

func TestDetectProject(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeFS)
		want  projectKind
	}{
		{"empty", func(*fakeFS) {}, kindNone},
		{"pyproject", func(f *fakeFS) { f.addFile("pyproject.toml") }, kindProject},
		{
			"pyproject wins over requirements and venv",
			func(f *fakeFS) {
				f.addFile("pyproject.toml")
				f.addFile("requirements.txt")
				f.addDir(".venv")
			},
			kindProject,
		},
		{"requirements txt", func(f *fakeFS) { f.addFile("requirements.txt") }, kindRequirements},
		{"requirements lock", func(f *fakeFS) { f.addFile("requirements.lock") }, kindRequirements},
		{"requirements in", func(f *fakeFS) { f.addFile("requirements.in") }, kindRequirements},
		{"venv only", func(f *fakeFS) { f.addDir(".venv") }, kindVenvOnly},
		{"script only", func(f *fakeFS) { f.addEntry("main.py", false) }, kindScript},
		{"non-python files only", func(f *fakeFS) { f.addEntry("README.md", false) }, kindNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFakeFS()
			tc.setup(fs)
			assert.Equal(t, tc.want, detectProject(context.Background(), fs))
		})
	}
}

func TestEnsureEnvironmentCommands(t *testing.T) {
	cases := []struct {
		name      string
		kind      projectKind
		setupFS   func(*fakeFS)
		responses map[string]scriptedCmd
		wantCalls []string
		wantTotal int64
		wantSync  string
	}{
		{
			name: "project runs uv sync",
			kind: kindProject,
			responses: map[string]scriptedCmd{
				"uv python find": {},
				"uv sync":        {},
			},
			wantCalls: []string{"uv python find", "uv sync"},
			wantTotal: 3,
			wantSync:  "Syncing project dependencies",
		},
		{
			name:    "requirements creates venv then pip install",
			kind:    kindRequirements,
			setupFS: func(f *fakeFS) { f.addFile("requirements.txt") },
			responses: map[string]scriptedCmd{
				"uv python find":                     {},
				"uv venv":                            {},
				"uv pip install -r requirements.txt": {},
			},
			wantCalls: []string{"uv python find", "uv venv", "uv pip install -r requirements.txt"},
			wantTotal: 3,
			wantSync:  "Installing requirements",
		},
		{
			name: "venv only verifies interpreter",
			kind: kindVenvOnly,
			responses: map[string]scriptedCmd{
				"uv python find": {},
			},
			wantCalls: []string{"uv python find", "uv python find"},
			wantTotal: 3,
			wantSync:  "Verifying virtual environment",
		},
		{
			name: "script only guards interpreter",
			kind: kindScript,
			responses: map[string]scriptedCmd{
				"uv python find": {},
			},
			wantCalls: []string{"uv python find"},
			wantTotal: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFakeFS()
			if tc.setupFS != nil {
				tc.setupFS(fs)
			}
			ex := newFakeExecutor()
			for k, v := range tc.responses {
				ex.respond(k, v)
			}
			notify := newFakeNotifications()
			err := ensureEnvironment(context.Background(), "uv", ex, notify, tc.kind, fs)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCalls, ex.callsSnapshot())

			last := notify.lastProgress()
			assert.Equal(t, tc.wantTotal, last.total)
			assert.Equal(t, tc.wantTotal, last.progress,
				"final progress must reach total")
			assert.Equal(t, "Python environment ready", last.message)
			assertMonotonicProgress(t, notify)

			msgs := notify.progressMessages()
			assert.Equal(t, "Finding Python interpreter", msgs[0])
			if tc.wantSync != "" {
				assert.Contains(t, msgs, tc.wantSync)
			}
		})
	}
}

func TestEnsureEnvironmentInstallsInterpreterOnFirstRun(t *testing.T) {
	fs := newFakeFS()
	ex := newFakeExecutor()
	ex.respond("uv python find", scriptedCmd{err: assertErr})
	ex.respond("uv python install --default", scriptedCmd{})
	ex.respond("uv sync", scriptedCmd{})
	notify := newFakeNotifications()
	err := ensureEnvironment(context.Background(), "uv", ex, notify, kindProject, fs)
	require.NoError(t, err)
	assert.Equal(t, []string{"uv python find", "uv python install --default", "uv sync"}, ex.callsSnapshot())

	last := notify.lastProgress()
	assert.Equal(t, int64(4), last.total, "installing an interpreter adds a step")
	assert.Equal(t, int64(4), last.progress)
	assert.Contains(t, notify.progressMessages(), "Installing Python interpreter")
	assertMonotonicProgress(t, notify)
}

var assertErr = &interpreterMissingError{}

type interpreterMissingError struct{}

func (*interpreterMissingError) Error() string { return "no interpreter" }
