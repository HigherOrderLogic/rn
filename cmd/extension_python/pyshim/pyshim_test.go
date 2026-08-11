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

package pyshim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// osFS is a workspaceapi.FileSystem backed directly by the OS. Shim
// tests always use absolute paths rooted in a temp dir, so no
// workspace-root resolution is needed.
type osFS struct{}

func (osFS) URI(p string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI("file://" + p)
}

func (osFS) OpenFile(p string, flag int, mode os.FileMode) (workspaceapi.File, error) {
	f, err := os.OpenFile(p, flag, mode)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (osFS) Remove(p string) error                     { return os.Remove(p) }
func (osFS) Stat(p string) (os.FileInfo, error)        { return os.Stat(p) }
func (osFS) ReadDir(p string) ([]os.DirEntry, error)   { return os.ReadDir(p) }
func (osFS) MkdirAll(p string, perm os.FileMode) error { return os.MkdirAll(p, perm) }

func TestWrite(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, Write(osFS{}, dataDir))

	for _, name := range []string{"python", "python3"} {
		p := filepath.Join(Dir(dataDir), name)
		info, err := os.Stat(p)
		require.NoError(t, err, "shim %s must exist", name)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "shim %s mode", name)

		data, err := os.ReadFile(p)
		require.NoError(t, err)
		body := string(data)
		assert.True(t, strings.HasPrefix(body, "#!/bin/sh\n"), "shim %s shebang", name)
		assert.Contains(t, body, FallbackPath(dataDir),
			"shim %s must embed the managed-interpreter fallback path", name)
	}
}

func TestWriteIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, Write(osFS{}, dataDir))
	first, err := os.ReadFile(filepath.Join(Dir(dataDir), "python3"))
	require.NoError(t, err)

	require.NoError(t, Write(osFS{}, dataDir))
	second, err := os.ReadFile(filepath.Join(Dir(dataDir), "python3"))
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

// TestWriteReplacesSymlink covers migration of existing installs
// where uv's interpreter symlinks occupy python/bin: the shim must
// replace the link with a regular file without writing through it into
// the managed interpreter.
func TestWriteReplacesSymlink(t *testing.T) {
	dataDir := t.TempDir()
	binDir := Dir(dataDir)
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	target := filepath.Join(dataDir, "python", "python", "cpython", "bin", "python3.12")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("managed-interpreter"), 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(binDir, "python3")))
	require.NoError(t, os.Symlink(target, filepath.Join(binDir, "python")))

	require.NoError(t, Write(osFS{}, dataDir))

	for _, name := range []string{"python", "python3"} {
		info, err := os.Lstat(filepath.Join(binDir, name))
		require.NoError(t, err)
		assert.Zero(t, info.Mode()&os.ModeSymlink,
			"shim %s must be a regular file, not a symlink", name)
	}
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "managed-interpreter", string(data),
		"managed interpreter must not be clobbered through the old symlink")
}

// fakeInterpreter writes an executable script at path that prints tag
// followed by its arguments, standing in for a real python binary.
func fakeInterpreter(t *testing.T, path, tag string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	body := "#!/bin/sh\necho \"" + tag + " $@\"\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
}

// runShim executes the shim through sh with a controlled environment so
// an activated venv or dev PATH cannot leak into the assertion.
func runShim(t *testing.T, shim, cwd string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{shim}, args...)...)
	cmd.Dir = cwd
	cmd.Env = append([]string{"PATH=/usr/bin:/bin"}, env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		exit = exitErr.ExitCode()
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), exit
}

func TestShimResolution(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, Write(osFS{}, dataDir))
	shim := filepath.Join(Dir(dataDir), "python3")

	t.Run("nearest enclosing project venv wins", func(t *testing.T) {
		root := t.TempDir()
		fakeInterpreter(t, filepath.Join(root, ".venv", "bin", "python"), "venv-root")
		fakeInterpreter(t, filepath.Join(root, "a", ".venv", "bin", "python"), "venv-a")
		cwd := filepath.Join(root, "a", "b")
		require.NoError(t, os.MkdirAll(cwd, 0o755))

		out, _, exit := runShim(t, shim, cwd, nil, "-V", "extra")
		assert.Zero(t, exit)
		assert.Equal(t, "venv-a -V extra", out)
	})

	t.Run("activated VIRTUAL_ENV wins over project venv", func(t *testing.T) {
		root := t.TempDir()
		fakeInterpreter(t, filepath.Join(root, ".venv", "bin", "python"), "venv-project")
		venv := t.TempDir()
		fakeInterpreter(t, filepath.Join(venv, "bin", "python"), "venv-activated")

		out, _, exit := runShim(t, shim, root, []string{"VIRTUAL_ENV=" + venv})
		assert.Zero(t, exit)
		assert.Equal(t, "venv-activated", out)
	})

	t.Run("no venv falls back to managed interpreter", func(t *testing.T) {
		fakeInterpreter(t, FallbackPath(dataDir), "managed")
		cwd := t.TempDir()

		out, _, exit := runShim(t, shim, cwd, nil, "script.py")
		assert.Zero(t, exit)
		assert.Equal(t, "managed script.py", out)
	})

	t.Run("nothing available fails with guidance", func(t *testing.T) {
		bare := t.TempDir()
		require.NoError(t, Write(osFS{}, bare))
		bareShim := filepath.Join(Dir(bare), "python3")

		_, stderr, exit := runShim(t, bareShim, t.TempDir(), nil)
		assert.Equal(t, 127, exit)
		assert.Contains(t, stderr, "rune: no Python found")
	})
}
