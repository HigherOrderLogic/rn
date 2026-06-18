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
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
)

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

// TestE2E_UV_ProjectSync drives ensureEnvironment against a real uv in a
// fresh pyproject workspace and asserts the .venv is created.
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

	fs := newFakeFS().addFile("pyproject.toml")
	kind := detectProject(context.Background(), fs)
	require.Equal(t, kindProject, kind)

	ex := newDirExecutor(dir)
	err := ensureEnvironment(context.Background(), "uv", ex, newFakeNotifications(), kind, fs)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, ".venv"))
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "uv sync should create a .venv directory")
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

		got := resolvePyTool(context.Background(), realFS{root: dataDir}, realExecutor{}, dataDir, name)
		assert.Equal(t, bin, got)
	}

	missing := resolvePyTool(context.Background(), realFS{root: dataDir}, realExecutor{}, dataDir, "absent")
	assert.Empty(t, missing)
}
