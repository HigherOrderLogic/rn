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
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// realFS is a minimal workspaceapi.FileSystem backed by the OS and
// rooted at a workspace directory. Relative paths resolve against root,
// so the extension's detection and URI logic run against a real tree
// while the test controls the root. Only the methods the extension uses
// (URI, Stat, ReadDir) are implemented; the rest report not-supported.
type realFS struct{ root string }

func (f realFS) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(f.root, p)
}

func (f realFS) URI(p string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI("file://" + f.resolve(p))
}

func (f realFS) Stat(p string) (os.FileInfo, error) { return os.Stat(f.resolve(p)) }

func (f realFS) ReadDir(p string) ([]os.DirEntry, error) { return os.ReadDir(f.resolve(p)) }

func (f realFS) OpenFile(string, int, os.FileMode) (workspaceapi.File, error) {
	return nil, errors.New("realFS: OpenFile not supported")
}
func (f realFS) Remove(string) error { return errors.New("realFS: Remove not supported") }
func (f realFS) MkdirAll(string, os.FileMode) error {
	return errors.New("realFS: MkdirAll not supported")
}

// captureLSP is a fake semanticapi.LSP that records the InitializeParams
// it received and otherwise no-ops every request. The env+LSP suite
// asserts against the captured params instead of running a real server.
type captureLSP struct {
	noopLSP
	mu         sync.Mutex
	initParams *semanticapi.InitializeParams
	initCount  int
}

func (l *captureLSP) Initialize(
	_ context.Context, p semanticapi.InitializeParams,
) (semanticapi.InitializeResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := p
	l.initParams = &cp
	l.initCount++
	return semanticapi.InitializeResult{}, nil
}

func (l *captureLSP) captured() (semanticapi.InitializeParams, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.initParams == nil {
		return semanticapi.InitializeParams{}, l.initCount
	}
	return *l.initParams, l.initCount
}

// scenarioEnv is the result of running the extension against a scenario:
// the workspace directory, the executor rooted there, and the captured
// LSP/notifications for assertions.
type scenarioEnv struct {
	dir     string
	exec    realExecutor
	lsp     *captureLSP
	notify  *fakeNotifications
	manuals []textapi.CommandManual
}

// loadScenario copies testdata/<name> into a fresh temp directory so each
// run is isolated and uv side effects do not leak into the source tree.
// For the venv_only scenario it pre-seeds a populated .venv from
// venv-packages.txt, simulating a workspace that already has a virtual
// environment the extension must reuse without re-installing.
func loadScenario(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", name)
	dir := t.TempDir()
	copyTree(t, src, dir)

	pkgs := filepath.Join(dir, "venv-packages.txt")
	if data, err := os.ReadFile(pkgs); err == nil {
		require.NoError(t, os.Remove(pkgs))
		seedVenv(t, dir, string(data))
	}
	return dir
}

// copyTree recursively copies src into dst, preserving relative layout.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dst, 0o755))
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyTree(t, s, d)
			continue
		}
		data, err := os.ReadFile(s)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(d, data, 0o644))
	}
}

// seedVenv creates a .venv in dir and installs the given requirements
// into it via real uv, leaving the workspace in the kindVenvOnly state
// the extension must detect and reuse.
func seedVenv(t *testing.T, dir, requirements string) {
	t.Helper()
	ctx := context.Background()
	ex := newDirExecutor(dir)
	require.NoError(t, runUV(ctx, "uv", ex, "venv"))
	reqFile := filepath.Join(dir, ".venv-seed-requirements.txt")
	require.NoError(t, os.WriteFile(reqFile, []byte(requirements), 0o644))
	require.NoError(t, runUV(ctx, "uv", ex, "pip", "install", "-r", reqFile))
	require.NoError(t, os.Remove(reqFile))
}

// runExtensionOnScenario loads the named scenario, runs the extension's
// full bring-up (detection, uv env, LSP init, REPL registration) against
// a real FileSystem and Executor with a fake LSP and Notifications, and
// returns the resulting environment for assertions.
func runExtensionOnScenario(t *testing.T, name string) scenarioEnv {
	t.Helper()
	dir := loadScenario(t, name)
	ex := newDirExecutor(dir)
	lsp := &captureLSP{}
	notify := newFakeNotifications()
	env := scenarioEnv{dir: dir, exec: ex, lsp: lsp, notify: notify}

	ext := &pyExtension{}
	err := ext.extendWorkspaceWith(context.Background(),
		realFS{root: dir},
		ex,
		notify,
		lsp,
		"",
		nil,
		func(m textapi.CommandManual, _ textapi.REPLHandler) error {
			env.manuals = append(env.manuals, m)
			return nil
		},
	)
	require.NoError(t, err)
	return env
}

// runPython runs the scenario's main.py through the synced environment
// via `uv run` and returns its stdout, so all scenarios are verified to
// produce identical results regardless of how the env was expressed.
func runPython(t *testing.T, dir string) string {
	t.Helper()
	out, err := runUVCaptureDir(t, dir, "run", "python", "main.py")
	require.NoError(t, err)
	return out
}

// runUVCaptureDir runs `uv <args>` in dir through the real executor and
// returns trimmed stdout. It reuses the handler's capture path so the
// e2e suite exercises the same code that backs the `python` REPL command.
func runUVCaptureDir(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	h := &pyHandler{exec: newDirExecutor(dir), notify: newFakeNotifications(), cwd: dir}
	return h.runUVCapture(context.Background(), args...)
}

// allScenarios lists the testdata scenario directories driven by the
// env+LSP and REPL suites. Add a folder here to cover a new in-the-wild
// environment shape.
var allScenarios = []string{"pyproject", "requirements", "venv_only"}
