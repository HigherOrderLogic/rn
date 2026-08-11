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
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"unstable.build/go-tui/extension/langext"
	"unstable.build/go-tui/ide/idelsp"
	"unstable.build/go-tui/workspace"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type startedProcess struct {
	path   string
	args   []string
	env    []string
	stderr *synchronizedBuffer
}

type recordingScheme struct {
	schemeapi.Scheme
	mu      sync.Mutex
	started []startedProcess
}

func (s *recordingScheme) Start(ctx context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
	return s.StartCommand(ctx, cmd)
}

func (s *recordingScheme) StartCommand(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	stderr := &synchronizedBuffer{}
	if cmd.Stderr == nil {
		cmd.Stderr = stderr
	}
	pid, err := s.Scheme.StartCommand(ctx, cmd)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.started = append(s.started, startedProcess{
		path: cmd.Path, args: append([]string{}, cmd.Args...),
		env: append([]string{}, cmd.Env...), stderr: stderr,
	})
	s.mu.Unlock()
	return pid, nil
}

func (s *recordingScheme) startedProcess(path string) (startedProcess, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.started) - 1; i >= 0; i-- {
		if s.started[i].path == path {
			return s.started[i], true
		}
	}
	return startedProcess{}, false
}

type successfulExecutor struct {
	mu      sync.Mutex
	nextPid workspaceapi.Pid
}

func (e *successfulExecutor) Start(
	_ context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	e.mu.Lock()
	e.nextPid++
	pid := e.nextPid
	e.mu.Unlock()
	if cmd.Watcher != nil {
		cmd.Watcher.WatchProcess() <- nil
	}
	return pid, nil
}

func (*successfulExecutor) Signal(workspaceapi.Pid, syscall.Signal) error { return nil }
func (*successfulExecutor) Close() error                                  { return nil }

type e2eInstaller map[string]string

func (i e2eInstaller) FindInstalledExecutable(_ context.Context, name string) (string, error) {
	if path := i[name]; path != "" {
		return path, nil
	}
	return "", os.ErrNotExist
}

func TestE2EPythonLoggingConfigReachesServers(t *testing.T) {
	tyBin := findVersionedTool(t, "ty", "0.0.51")
	ruffBin := findVersionedTool(t, "ruff", "0.15.18")
	dir := t.TempDir()
	rootURI := "file://" + dir
	uri, err := workspaceapi.ParseURI(rootURI)
	require.NoError(t, err)

	baseScheme, err := workspace.NewFileScheme(t.Context(), config.NopConfig(), uri)
	require.NoError(t, err)
	scheme := &recordingScheme{Scheme: baseScheme}
	mgr := idelsp.New(uri, scheme, scheme, nil, nil, nil,
		idelsp.Config{MaxRetries: 1})
	t.Cleanup(func() { require.NoError(t, mgr.Close()) })

	cfg := config.JSONFromMap(map[string]any{
		"debug": map[string]any{"log_level": "debug"},
	})
	installer := e2eInstaller{"ty": tyBin, "ruff": ruffBin, "uv": "uv"}
	err = initializeProjectRoot(t.Context(), scheme, &successfulExecutor{}, newFakeNotifications(),
		mgr, installer, cfg, langext.Root{Dir: dir, URI: rootURI})
	require.NoError(t, err)

	ty, ok := scheme.startedProcess(tyBin)
	require.True(t, ok, "ty process was not started")
	assert.Equal(t, []string{"server"}, ty.args)
	ruff, ok := scheme.startedProcess(ruffBin)
	require.True(t, ok, "ruff process was not started")
	assert.Equal(t, []string{"server", "-v"}, ruff.args)
	require.Eventually(t, func() bool {
		return strings.Contains(ty.stderr.String(), "log_level: Some(") &&
			strings.Contains(ty.stderr.String(), "Debug,")
	}, 5*time.Second, 20*time.Millisecond,
		"ty did not report the forwarded debug initialization option")
	require.Eventually(t, func() bool {
		return ruff.stderr.String() != ""
	}, 5*time.Second, 20*time.Millisecond, "ruff did not write verbose logs to stderr")
}

func findVersionedTool(t *testing.T, name, version string) string {
	t.Helper()
	bin, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found, skipping python logging e2e test", name)
	}
	out, err := exec.Command(bin, "--version").CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), version)
	return bin
}

// realExecutor spawns real subprocesses, used by the e2e tests against
// an installed uv. The watcher receives the process exit error. dir, when
// set, is the default working directory for commands that do not specify
// their own, mirroring the workspace executor being rooted at the
// workspace directory in production.
type realExecutor struct{ dir string }

func (e realExecutor) Start(ctx context.Context, c workspaceapi.Cmd) (workspaceapi.Pid, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	if cmd.Dir == "" {
		cmd.Dir = e.dir
	}
	if c.Env != nil {
		cmd.Env = c.Env
	}
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := workspaceapi.Pid(cmd.Process.Pid)
	go func() {
		err := cmd.Wait()
		if c.Watcher != nil {
			c.Watcher.WatchProcess() <- err
		}
	}()
	return pid, nil
}

func (realExecutor) Signal(workspaceapi.Pid, syscall.Signal) error { return nil }
func (realExecutor) Close() error                                  { return nil }

func findUV(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not found, skipping python e2e test")
	}
}

// TestE2E_UV_ProjectSync drives the full extension bring-up against a
// real uv in a fresh pyproject workspace and asserts the .venv is created
// by the eager workspace-root sync and that the language server is
// initialized exactly once rooted there.
func TestE2E_UV_ProjectSync(t *testing.T) {
	findUV(t)

	dir := t.TempDir()
	pyproject := `[project]
name = "rune-py-e2e"
version = "0.0.0"
requires-python = ">=3.9"
dependencies = []
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644))

	env := runExtensionOnDir(t, dir)

	info, err := os.Stat(filepath.Join(dir, ".venv"))
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "uv sync should create a .venv directory")

	assertShimResolvesVenv(t, env)

	params, count := env.lsp.captured()
	require.Equal(t, 1, count, "the workspace-root project must initialize exactly once")
	assert.Equal(t, "file://"+dir, params.RootURI)
}

// TestE2E_PyHandler_PythonList runs the `python list` REPL subcommand
// against a real uv and asserts output is produced.
func TestE2E_PyHandler_PythonList(t *testing.T) {
	findUV(t)

	dir := t.TempDir()
	_, handler := newPyHandler(newDirExecutor(dir), newFakeNotifications(), dir)
	it, err := handler.HandleCommand(
		context.Background(),
		repl.Command{Name: "python", Args: []string{"list"}},
		repl.NopProgressWriter(),
	)
	require.NoError(t, err)
	out, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	require.Len(t, out, 1)
}

// newDirExecutor returns a realExecutor rooted at dir, so commands that
// do not set Cmd.Dir run in the workspace directory.
func newDirExecutor(dir string) realExecutor { return realExecutor{dir: dir} }

// TestE2E_ResolvePyTool resolves a tool from <dataDir>/bin/<name> against
// a real filesystem.
func TestE2E_ResolvePyTool(t *testing.T) {
	dataDir := t.TempDir()
	binDir := filepath.Join(dataDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	for _, name := range []string{"uv", "ty", "ruff"} {
		bin := filepath.Join(binDir, name)
		require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

		got := resolvePyTool(context.Background(), realFS{root: dataDir}, realExecutor{},
			fakeInstaller{fs: realFS{root: dataDir}, root: dataDir}, name)
		assert.Equal(t, bin, got)
	}

	missing := resolvePyTool(context.Background(), realFS{root: dataDir}, realExecutor{},
		fakeInstaller{fs: realFS{root: dataDir}, root: dataDir}, "absent")
	assert.Empty(t, missing)
}
