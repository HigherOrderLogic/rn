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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"unstable.build/go-tui/cmd/extension_python/pyshim"
	"unstable.build/go-tui/extension/langext"
)

// initRootHarness runs initializeProjectRoot against an empty workspace
// dir on the real filesystem with a scripted executor, returning the
// executor for call assertions.
func initRootHarness(t *testing.T, dataDir string, cfg config.Config) *fakeExecutor {
	t.Helper()
	work := t.TempDir()
	fs := realFS{root: work}
	ex := newFakeExecutor()
	ex.respond("uv python find", scriptedCmd{})
	ex.respond("uv python install --default", scriptedCmd{})
	ex.respond("uvx --from debugpy==1.8.17 python -c ", scriptedCmd{})
	root := langext.Root{Dir: work, URI: "file://" + work}
	err := initializeProjectRoot(context.Background(), fs, ex,
		newFakeNotifications(), &captureLSP{},
		fakeInstaller{fs: fs, root: dataDir}, cfg, dataDir, root)
	require.NoError(t, err)
	return ex
}

func TestInitializeProjectRootInstallsShims(t *testing.T) {
	dataDir := t.TempDir()
	initRootHarness(t, dataDir, config.NopConfig())
	assert.FileExists(t, filepath.Join(pyshim.Dir(dataDir), "python"))
	assert.FileExists(t, filepath.Join(pyshim.Dir(dataDir), "python3"))
}

func TestInitializeProjectRootPrewarmsDebugpy(t *testing.T) {
	cfg := config.JSONFromMap(map[string]any{"debugpy": "1.8.17"})
	ex := initRootHarness(t, t.TempDir(), cfg)
	assert.Eventually(t, func() bool {
		for _, call := range ex.callsSnapshot() {
			if strings.HasPrefix(call, "uvx --from debugpy==1.8.17") {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "background uvx prewarm must run")
}

func TestInitializeProjectRootSkipsPrewarmWithoutPin(t *testing.T) {
	ex := initRootHarness(t, t.TempDir(), config.NopConfig())
	time.Sleep(50 * time.Millisecond)
	for _, call := range ex.callsSnapshot() {
		assert.NotContains(t, call, "uvx", "no prewarm must run without a debugpy pin")
	}
}

func TestDebugpyPin(t *testing.T) {
	t.Run("nil config returns empty", func(t *testing.T) {
		assert.Equal(t, "", debugpyPin(nil, newFakeNotifications()))
	})

	t.Run("absent key returns empty", func(t *testing.T) {
		notify := newFakeNotifications()
		assert.Equal(t, "", debugpyPin(config.NopConfig(), notify))
		assert.Empty(t, notify.notifs)
	})

	t.Run("pin returned", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{"debugpy": "1.8.17"})
		assert.Equal(t, "1.8.17", debugpyPin(cfg, newFakeNotifications()))
	})

	t.Run("non-string warns and returns empty", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{"debugpy": 1.8})
		notify := newFakeNotifications()
		assert.Equal(t, "", debugpyPin(cfg, notify))
		assert.NotEmpty(t, notify.notifs)
	})
}
