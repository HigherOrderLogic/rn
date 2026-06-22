// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
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

package workspace

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

func newTestFileScheme(uri workspaceapi.URI) (*fileScheme, error) {
	ret := new(fileScheme)
	ret.osStat = func(path string) (os.FileInfo, error) {
		if strings.Contains(path, ".txt") || strings.Contains(path, ".md") {
			return testFileInfo{name: path, isDir: false}, nil
		}
		return testFileInfo{name: path, isDir: true}, nil
	}
	ret.getUser = func() (*user.User, error) {
		return &user.User{Username: "git", HomeDir: "/home/git"}, nil
	}
	ret.lookupUser = func(username string) (*user.User, error) {
		return &user.User{Username: username, HomeDir: fmt.Sprintf("/home/%s", username)}, nil
	}
	err := ret.init(config.NopConfig(), uri)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func TestNewScheme(t *testing.T) {
	tsuite := []struct {
		desc         string
		workspaceURI string
		expectedErr  string
	}{
		{"no port no user workspace absolute returns error", "file://ernest.photography", "invalid file URI"},
		{"port no user workspace absolute returns error", "file://ernest.photography:4222", "invalid file URI"},
		{"port user workspace absolute returns error", "file://ernie@ernest.photography:4222", "invalid file URI"},
		{"port user workspace absolute slash returns error", "file://ernie@ernest.photography:4222/", "invalid file URI"},
		{"no port user workspace absolute slash returns error", "file://ernie@ernest.photography/", "invalid file URI"},
		{"different scheme returns error", "ssh:///tmp", "invalid file URI"},
		{"no host regular folder success", "file:///tmp", ""},
		{"root", "file:///", ""},
		{"file uri should return error", "file:///tmp/file.txt",
			"workspaceapi.URI does not refer to a directory: file:///tmp/file.txt"}, // newTestFileScheme sets osStat based on file name
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			workspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			// sut
			_, err = newTestFileScheme(workspaceURI)
			if tcase.expectedErr != "" {
				assert.EqualError(t, err, tcase.expectedErr)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}

func TestFileSchemeURI(t *testing.T) {
	tsuite := []struct {
		desc         string
		workspaceURI string
		inPath       string
		expectedOut  string
		expectedErr  string
	}{
		{"absolute root", "file:///", "/",
			"file:///", ""},
		{"absolute root, non-root workspace", "file:///home/ernicles", "/",
			"file:///", ""},
		{"absolute root 2", "file:///var", "/tmp/file",
			"file:///tmp/file", ""},
		{"absolute folder is equal to workspace", "file:///home/ernicles", "/home/ernicles",
			"file:///home/ernicles", ""},
		{"absolute folder file", "file:///home/ernicles", "/home/ernicles/file.txt",
			"file:///home/ernicles/file.txt", ""},
		{"absolute other folder file", "file:///home/ernicles", "/home/git/file.txt",
			"file:///home/git/file.txt", ""},
		{"relative file", "file:///home/ernicles", "file.txt",
			"file:///home/ernicles/file.txt", ""},
		{"relative file to home, non workspace", "file:///home/src/blue", "~/file.txt",
			"file:///home/git/file.txt", ""},
		{"relative file to home, workspace", "file:///home/git/", "~/file.txt",
			"file:///home/git/file.txt", ""},
		{"relative upwards workspace tree", "file:///home/git/src/blue", "../../",
			"file:///home/git/src/blue/../../", ""},
		{"relative upwards workspace tree ending slash", "file:///home/git/src/blue/", "../../",
			"file:///home/git/src/blue/../../", ""},
		{"relative upwards workspace tree ./ path", "file:///home/git/src/blue", "./../../file.txt",
			"file:///home/git/src/blue/./../../file.txt", ""},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			workspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			s, err := newTestFileScheme(workspaceURI)
			require.NoError(t, err)
			defer s.Close()

			// sut
			actualOut, actualErr := s.URI(tcase.inPath)
			if tcase.expectedErr != "" {
				assert.Error(t, actualErr)
				assert.Nil(t, actualOut)
			} else {
				assert.NoError(t, actualErr)

				expectedURI, err := workspaceapi.ParseURI(tcase.expectedOut)
				require.NoError(t, err)
				assert.Equal(t, expectedURI.String(), actualOut.String())
			}

		})
	}
}

func TestFileAssumptions(t *testing.T) {
	t.Run("Write overwrites data", func(t *testing.T) {
		f, err := os.CreateTemp("", "")
		name := f.Name()
		n, err := f.Write([]byte("12345"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		f, err = os.OpenFile(name, os.O_RDWR, 0)
		require.NoError(t, err)
		n, err = f.Write([]byte("ZZ"))
		require.NoError(t, err)
		assert.Equal(t, 2, n)

		nn, err := f.Seek(0, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(0), nn)

		data, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "ZZ345", string(data))
	})

	t.Run("Read uses write offset", func(t *testing.T) {
		f, err := os.CreateTemp("", "")
		name := f.Name()
		n, err := f.Write([]byte("12345"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		f, err = os.OpenFile(name, os.O_RDWR, 0)
		require.NoError(t, err)

		n, err = f.Write([]byte("ZZ"))
		require.NoError(t, err)
		assert.Equal(t, 2, n)

		data, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "345", string(data))
	})
}

func TestStartCommand(t *testing.T) {
	t.Run("Cmd.Dir makes command run on that directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		ch := make(chan error)
		ctx := context.Background()

		// If Dir is passed runs there
		cmd := workspaceapi.Cmd{
			Path:    "/bin/sh",
			Args:    []string{"-c", "pwd"},
			Dir:     "/bin",
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)

		err = <-ch
		assert.Equal(t, err, nil)

		assert.Equal(t, stderr.String(), "")
		assert.Equal(t, stdout.String(), "/bin\n")

		stderr.Reset()
		stdout.Reset()

	})

	t.Run("local command inherits live process env", func(t *testing.T) {
		// Documents why os.Setenv is sufficient for future local launches:
		// StartCommand seeds the child env from the live process environment
		// at launch time, so a gui.env live-apply via os.Setenv is observed
		// by subsequently launched local commands.
		t.Setenv("RUNE_FILE_SCHEME_ENV_TEST", "inherited")

		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Path:    "/bin/sh",
			Args:    []string{"-c", "printf %s \"$RUNE_FILE_SCHEME_ENV_TEST\""},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)

		require.NoError(t, <-ch)
		assert.Equal(t, "", stderr.String())
		assert.Equal(t, "inherited", stdout.String())
	})

	t.Run("omitting Cmd.Dir makes command run on workspace dir", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Path:    "/bin/sh",
			Args:    []string{"-c", "pwd"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)

		err = <-ch
		assert.Equal(t, err, nil)

		assert.Equal(t, stderr.String(), "")
		assert.Equal(t, stdout.String(), tmpDir+"\n")
	})

	t.Run("empty Cmd.Path resolves to host's $SHELL", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		t.Setenv("SHELL", "/bin/sh")

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			// Empty Path — protocol contract for "use the user's
			// login shell on the executor's host".
			Args:    []string{"-c", "echo hello"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)
		require.NoError(t, <-ch)

		assert.Equal(t, "hello\n", stdout.String(),
			"empty Path should be resolved to /bin/sh from $SHELL")
	})

	t.Run("empty Cmd.Path falls back when $SHELL unset", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		t.Setenv("SHELL", "")

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Args:    []string{"-c", "echo ok"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)
		require.NoError(t, <-ch)

		assert.Equal(t, "ok\n", stdout.String(),
			"empty Path with unset $SHELL must still launch a shell (/bin/sh fallback)")
	})

	t.Run("empty Cmd.Path falls back when $SHELL is not absolute", func(t *testing.T) {
		// Some misbehaving environments set SHELL to a bare name
		// like "zsh" that may not resolve via PATH inside the
		// stripped exec environment we run with. The fallback
		// guards against that footgun.
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		t.Setenv("SHELL", "zsh") // not absolute

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Args:    []string{"-c", "echo fallback"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)
		require.NoError(t, <-ch)

		assert.Equal(t, "fallback\n", stdout.String(),
			"empty Path with non-absolute $SHELL must fall back to /bin/sh")
	})

	t.Run("empty Cmd.Path defaults Args to --login -i", func(t *testing.T) {
		// vte.Component leaves Path AND Args empty when the user
		// hasn't configured a shell; the fileScheme owns the
		// login-shell defaults so the same Cmd is transport-agnostic
		// (works locally and over workspacessh+workspacerpc).
		//
		// We point SHELL at a shell-script wrapper that prints its
		// own argv. That gives us a portable way to verify the
		// resolved args without depending on a real shell's
		// runtime behaviour.
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		bin := filepath.Join(tmpDir, "fake-shell")
		require.NoError(t, os.WriteFile(bin,
			[]byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755))
		t.Setenv("SHELL", bin)

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			// Empty Path AND empty Args.
			Stdout:  &stdout,
			Watcher: workspaceapi.ChanProcessWatcher(ch),
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)
		require.NoError(t, <-ch)

		assert.Equal(t, "--login\n-i\n", stdout.String(),
			"empty Args must default to [--login, -i] so empty-cmd "+
				"Cmds produce a usable login shell")
	})

	t.Run("empty Cmd.Path with zsh exports ZDOTDIR from config", func(t *testing.T) {
		// The fileScheme reads zdotdir from the workspace config
		// at init time. That keeps the resolution local to the
		// host that will actually run the shell — for SSH
		// workspaces the remote rune sees its own config, so the
		// IDE host's ZDOTDIR doesn't leak across the wire.
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		// Bypass newTestFileScheme so we can pass a non-nop config.
		s := new(fileScheme)
		s.osStat = os.Stat
		s.getUser = func() (*user.User, error) {
			return &user.User{Username: "git", HomeDir: "/home/git"}, nil
		}
		s.lookupUser = func(name string) (*user.User, error) {
			return &user.User{Username: name, HomeDir: "/home/" + name}, nil
		}
		require.NoError(t, s.init(
			config.MapConfig(map[string]any{"zdotdir": "/zdot/dir"}),
			uri,
		))

		// We can't rely on zsh being installed in CI, so we point
		// SHELL at a script named "zsh" (so filepath.Base of the
		// resolved shell is "zsh") that just trampolines into
		// /bin/sh. The fileScheme only injects ZDOTDIR when the
		// resolved binary's basename matches "zsh", which is what
		// we want to verify here.
		bin := filepath.Join(tmpDir, "zsh")
		require.NoError(t, os.WriteFile(bin,
			[]byte("#!/bin/sh\nexec /bin/sh \"$@\"\n"), 0o755))
		t.Setenv("SHELL", bin)

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Args:    []string{"-c", "echo ZDOTDIR=$ZDOTDIR"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)
		require.NoError(t, <-ch)

		assert.Equal(t, "ZDOTDIR=/zdot/dir\n", stdout.String(),
			"fileScheme should export ZDOTDIR when the resolved "+
				"shell is zsh and zdotdir is set in config")
	})

	t.Run("empty Cmd.Path without zsh leaves ZDOTDIR untouched", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s := new(fileScheme)
		s.osStat = os.Stat
		s.getUser = func() (*user.User, error) {
			return &user.User{Username: "git", HomeDir: "/home/git"}, nil
		}
		s.lookupUser = func(name string) (*user.User, error) {
			return &user.User{Username: name, HomeDir: "/home/" + name}, nil
		}
		require.NoError(t, s.init(
			config.MapConfig(map[string]any{"zdotdir": "/zdot/dir"}),
			uri,
		))

		t.Setenv("SHELL", "/bin/sh")
		// Make sure the parent's ZDOTDIR is unset so the test
		// only sees what fileScheme adds (or doesn't add).
		t.Setenv("ZDOTDIR", "")

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Args:    []string{"-c", "echo ZDOTDIR=${ZDOTDIR:-unset}"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)
		require.NoError(t, <-ch)

		assert.Equal(t, "ZDOTDIR=unset\n", stdout.String(),
			"non-zsh shells must not receive the configured ZDOTDIR; "+
				"got %q", stdout.String())
	})

	// Reproduces the bug from RUNE-184: a Cmd.Path beginning with ~
	// was forwarded verbatim to fork/exec because StartCommand
	// didn't expand it, even though every other path-taking
	// fileScheme method (OpenFile, Stat, ReadDir, ...) does. The
	// expansion must live in fileScheme because that's the only
	// layer that knows the executor host's getUser; for remote
	// schemes the same code runs on the remote rune so ~ resolves
	// to the *remote* user's home.
	t.Run("Cmd.Path expands ~ via the executor's user", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		// Use tmpDir as the fake user home so we can place a real
		// script at "~/script.sh".
		fakeHome := tmpDir
		script := filepath.Join(fakeHome, "script.sh")
		require.NoError(t, os.WriteFile(script,
			[]byte("#!/bin/sh\necho ok\n"), 0o755))

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s := new(fileScheme)
		s.osStat = os.Stat
		s.getUser = func() (*user.User, error) {
			return &user.User{Username: "git", HomeDir: fakeHome}, nil
		}
		s.lookupUser = func(name string) (*user.User, error) {
			return &user.User{Username: name, HomeDir: fakeHome}, nil
		}
		require.NoError(t, s.init(config.NopConfig(), uri))

		var stdout bytes.Buffer
		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Path:    "~/script.sh",
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err,
			"~-prefixed Cmd.Path must be expanded by fileScheme "+
				"before fork/exec; otherwise the kernel sees a "+
				"literal '~/script.sh' and reports 'no such file "+
				"or directory'")
		require.NoError(t, <-ch)
		assert.Equal(t, "ok\n", stdout.String())
	})
}

func TestResolveLoginShell(t *testing.T) {
	t.Run("absolute SHELL pointing at non-existent file falls back", func(t *testing.T) {
		// Reproduces the user-reported "fork/exec /usr/bin/bash: no
		// such file or directory" error: $SHELL points at a path
		// that exists in some context (e.g. a login shell) but not
		// in the rune-x process's view of the filesystem.
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		bogus := filepath.Join(tmpDir, "definitely-not-here")
		t.Setenv("SHELL", bogus)

		got := resolveLoginShell()
		assert.NotEqual(t, bogus, got,
			"resolveLoginShell must not return a $SHELL that doesn't "+
				"exist on disk; that produces a confusing fork/exec "+
				"error far from this code")
		// Whatever it picked must itself be executable so the
		// caller can actually run it.
		assert.True(t, isExecutableFile(got),
			"fallback %q must itself be executable", got)
	})

	t.Run("absolute SHELL pointing at a directory falls back", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		t.Setenv("SHELL", tmpDir) // directory, not a file

		got := resolveLoginShell()
		assert.NotEqual(t, tmpDir, got)
		assert.True(t, isExecutableFile(got))
	})

	t.Run("SHELL with stray whitespace is trimmed", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		bin := filepath.Join(tmpDir, "fake-shell")
		require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

		t.Setenv("SHELL", "  "+bin+"\n")

		got := resolveLoginShell()
		assert.Equal(t, bin, got,
			"resolveLoginShell must trim whitespace from $SHELL; "+
				"some sshd-spawned environments propagate a trailing "+
				"newline that breaks fork/exec")
	})

	t.Run("absolute existing executable SHELL is returned", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		bin := filepath.Join(tmpDir, "good-shell")
		require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

		t.Setenv("SHELL", bin)
		assert.Equal(t, bin, resolveLoginShell())
	})
}

func TestFileSchemeCloseClosesTrackedFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	uri, err := workspaceapi.ParseURI("file://" + tmpDir)
	require.NoError(t, err)

	s, err := newTestFileScheme(uri)
	require.NoError(t, err)

	path1 := tmpDir + "/a.txt"
	path2 := tmpDir + "/b.txt"
	require.NoError(t, os.WriteFile(path1, []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(path2, []byte("world"), 0644))

	f1, err := s.OpenFile(path1, os.O_RDONLY, 0)
	require.NoError(t, err)
	f2, err := s.OpenFile(path2, os.O_RDONLY, 0)
	require.NoError(t, err)

	// Sanity: both files tracked before close.
	var countBefore int
	s.files.Range(func(_, _ any) bool { countBefore++; return true })
	assert.Equal(t, 2, countBefore)

	require.NoError(t, s.Close())

	// After Close, tracked files should have been removed and underlying files
	// closed (a second close on the underlying *os.File returns an error).
	var countAfter int
	s.files.Range(func(_, _ any) bool { countAfter++; return true })
	assert.Equal(t, 0, countAfter)

	// Closing again should be a no-op (idempotent) for the wrapped file.
	assert.NoError(t, f1.Close())
	assert.NoError(t, f2.Close())
}

func TestOpenFileClosesSchemeOnCallerClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/a.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	f, err := OpenFile(path, os.O_RDONLY, 0)
	require.NoError(t, err)

	owned, ok := f.(*ownedSchemeFile)
	require.True(t, ok, "OpenFile should return an ownedSchemeFile")

	// Closing the returned file should also close the owning scheme's context,
	// which we can observe via the wrapped fileSchemeFile deregistering itself.
	inner, ok := owned.File.(*fileSchemeFile)
	require.True(t, ok, "wrapped file should be a fileSchemeFile")
	scheme := inner.p
	var tracked int
	scheme.files.Range(func(_, _ any) bool { tracked++; return true })
	require.Equal(t, 1, tracked)

	require.NoError(t, f.Close())

	// After Close, scheme should no longer track the file and its context
	// should be cancelled.
	tracked = 0
	scheme.files.Range(func(_, _ any) bool { tracked++; return true })
	assert.Equal(t, 0, tracked)
	select {
	case <-scheme.ctx.Done():
	default:
		t.Fatal("scheme context should be cancelled after close")
	}

	// Closing again should remain a no-op.
	assert.NoError(t, f.Close())
}

func TestReadFileClosesFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/a.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	data, err := ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
	// Calling ReadFile many times should not accumulate open FDs; if it did
	// we'd eventually hit EMFILE. This is a coarse smoke-check.
	for i := range 1024 {
		_, err := ReadFile(path)
		require.NoError(t, err, "iteration %d", i)
	}
}
