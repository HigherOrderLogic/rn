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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
)

func fakeLoginShell(t *testing.T, lines ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("login-shell resolution is POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fakeshell")
	var body strings.Builder
	body.WriteString("#!/bin/sh\n")
	for _, l := range lines {
		body.WriteString("echo " + l + "\n")
	}
	if err := os.WriteFile(path, []byte(body.String()), 0o755); err != nil {
		t.Fatalf("write fake shell: %v", err)
	}
	return path
}

func TestResolveLoginPath(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		want    string
		wantErr bool
	}{
		{
			name:  "ignores leading banner lines",
			lines: []string{"welcome-banner", "", "/login/bin:/usr/bin"},
			want:  "/login/bin:/usr/bin",
		},
		{
			name:  "single line",
			lines: []string{"/usr/bin"},
			want:  "/usr/bin",
		},
		{
			name:    "empty output is an error",
			lines:   []string{""},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHELL", fakeLoginShell(t, tc.lines...))

			got, err := resolveLoginPath()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got PATH %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveLoginPath: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got PATH %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSetupManagedBinPathPrependsBinDirOnce asserts that setupManagedBinPath
// creates the managed dirs and prepends the bin dir to PATH exactly once.
func TestSetupManagedBinPathPrependsBinDirOnce(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PATH", "/usr/bin:/usr/local/bin")

	if err := setupRuneBinPATH(dataDir); err != nil {
		t.Fatalf("setupManagedBinPath: %v", err)
	}

	binDir := filepath.Join(dataDir, "bin")
	want := binDir + ":/usr/bin:/usr/local/bin"
	if got := os.Getenv("PATH"); got != want {
		t.Fatalf("got PATH %q, want %q", got, want)
	}

	for _, sub := range []string{"bin", "lib"} {
		if fi, err := os.Stat(filepath.Join(dataDir, sub)); err != nil || !fi.IsDir() {
			t.Fatalf("managed dir %q not created: %v", sub, err)
		}
	}
}

// TestStartLoginPathResolveObservesResolvedPATH asserts that, after the
// background resolve is joined, PATH is the managed bin dir followed by the
// resolved login PATH and a gui.env value expanding $PATH observes it.
// Regression for the gui.env PATH startup race.
func TestStartLoginPathResolveObservesResolvedPATH(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("RUNE_DATADIR", dataDir)
	t.Setenv("SHELL", fakeLoginShell(t, "/login/bin:/usr/bin"))

	t.Setenv("PATH", "/min/gui/path")
	if err := setupRuneBinPATH(dataDir); err != nil {
		t.Fatalf("setupManagedBinPath: %v", err)
	}
	if err := <-startLoginShellPATHResolve(dataDir); err != nil {
		t.Fatalf("startLoginShellPATHResolve: %v", err)
	}

	binDir := filepath.Join(dataDir, "bin")
	want := binDir + ":/login/bin:/usr/bin"
	if got := os.Getenv("PATH"); got != want {
		t.Fatalf("got PATH %q, want %q", got, want)
	}
	if got := os.Getenv("PATH"); strings.Contains(got, "/min/gui/path") {
		t.Fatalf("minimal launch PATH leaked into resolved PATH: %q", got)
	}

	expanded := evalVar("$RUNE_DATADIR/lib/foo/bin:$PATH")
	if !strings.Contains(expanded, filepath.Join(dataDir, "lib", "foo", "bin")) {
		t.Fatalf("RUNE_DATADIR not expanded in gui.env value: %q", expanded)
	}
	if !strings.Contains(expanded, "/login/bin") {
		t.Fatalf("gui.env $PATH did not observe resolved login PATH: %q", expanded)
	}
	if !strings.Contains(expanded, binDir) {
		t.Fatalf("gui.env $PATH missing managed bin dir: %q", expanded)
	}
}

// TestStartLoginPathResolvePropagatesError asserts that a resolution
// failure is delivered on the channel rather than swallowed.
func TestStartLoginPathResolvePropagatesError(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("SHELL", fakeLoginShell(t, "" /* empty PATH output */))

	if err := <-startLoginShellPATHResolve(dataDir); err == nil {
		t.Fatal("expected resolution error, got nil")
	}
}

func TestApplyGUIEnvVarsSemantics(t *testing.T) {
	resetGUIEnvBaseline()
	t.Setenv("RUNE_TEST_BASE", "base-value")

	env := config.JSONFromMap(map[string]any{
		"STRING_EXPAND": "$RUNE_TEST_BASE/sub",
		"NUMBER":        42,
		"BOOLEAN":       true,
	})
	require.NoError(t, applyGUIEnvVars(env))

	assert.Equal(t, "base-value/sub", os.Getenv("STRING_EXPAND"),
		"string values expand via os.Expand")
	assert.Equal(t, "42", os.Getenv("NUMBER"),
		"non-string values use fmt.Sprintf(%%v)")
	assert.Equal(t, "true", os.Getenv("BOOLEAN"))
}

func TestApplyGUIEnvVarsUpdatesLiveEnv(t *testing.T) {
	resetGUIEnvBaseline()
	require.Empty(t, os.Getenv("RUNE_LIVE_APPLY_TEST"))

	env := config.JSONFromMap(map[string]any{"RUNE_LIVE_APPLY_TEST": "set"})
	require.NoError(t, applyGUIEnvVars(env))
	assert.Equal(t, "set", os.Getenv("RUNE_LIVE_APPLY_TEST"))
	t.Cleanup(func() { _ = os.Unsetenv("RUNE_LIVE_APPLY_TEST") })
}

// TestApplyGUIEnvVarsNoPATHDuplication asserts that applying a self-referential
// PATH value twice does not accumulate duplicate entries, because both applies
// expand against the stable pre-gui.env baseline.
func TestApplyGUIEnvVarsNoPATHDuplication(t *testing.T) {
	resetGUIEnvBaseline()
	t.Setenv("PATH", "/usr/bin:/bin")

	env := config.JSONFromMap(map[string]any{"PATH": "/extra/bin:$PATH"})
	require.NoError(t, applyGUIEnvVars(env))
	first := os.Getenv("PATH")
	assert.Equal(t, "/extra/bin:/usr/bin:/bin", first)

	require.NoError(t, applyGUIEnvVars(env))
	second := os.Getenv("PATH")
	assert.Equal(t, first, second,
		"repeated apply must not duplicate PATH entries")
	assert.Equal(t, 1, strings.Count(second, "/extra/bin"))
}

func TestApplyGUIEnvVarsWithLookup(t *testing.T) {
	lookup := func(key string) string {
		if key == "CUSTOM" {
			return "custom-value"
		}
		return ""
	}
	env := config.JSONFromMap(map[string]any{"OUT": "$CUSTOM/x"})
	require.NoError(t, applyGUIEnvVarsWithLookup(env, lookup))
	assert.Equal(t, "custom-value/x", os.Getenv("OUT"))
	t.Cleanup(func() { _ = os.Unsetenv("OUT") })
}

func resetGUIEnvBaseline() {
	guiEnvBaselineMu.Lock()
	guiEnvBaseline = nil
	guiEnvBaselineDone = false
	guiEnvBaselineMu.Unlock()
}
