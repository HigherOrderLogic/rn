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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// fakeFileInfo satisfies os.FileInfo for paths scripted into fakeFS.
type fakeFileInfo struct {
	name string
	dir  bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o755 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }

// fakeDirEntry satisfies os.DirEntry for ReadDir results.
type fakeDirEntry struct {
	name string
	dir  bool
}

func (e fakeDirEntry) Name() string               { return e.name }
func (e fakeDirEntry) IsDir() bool                { return e.dir }
func (e fakeDirEntry) Type() os.FileMode          { return 0 }
func (e fakeDirEntry) Info() (os.FileInfo, error) { return fakeFileInfo{name: e.name, dir: e.dir}, nil }

// fakeFS implements workspaceapi.FileSystem for resolver tests.
type fakeFS struct {
	files   map[string]bool
	dirs    map[string]bool
	entries []os.DirEntry
}

func newFakeFS() *fakeFS {
	return &fakeFS{files: map[string]bool{}, dirs: map[string]bool{}}
}

func (f *fakeFS) addFile(p string) *fakeFS { f.files[p] = true; return f }
func (f *fakeFS) addDir(p string) *fakeFS  { f.dirs[p] = true; return f }
func (f *fakeFS) addEntry(name string, dir bool) *fakeFS {
	f.entries = append(f.entries, fakeDirEntry{name: name, dir: dir})
	return f
}

func (f *fakeFS) URI(p string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI("file://" + p)
}

func (f *fakeFS) OpenFile(_ string, _ int, _ os.FileMode) (workspaceapi.File, error) {
	return nil, errors.New("not supported")
}

func (f *fakeFS) Remove(_ string) error { return errors.New("not supported") }
func (f *fakeFS) MkdirAll(_ string, _ os.FileMode) error {
	return errors.New("not supported")
}

func (f *fakeFS) Stat(name string) (os.FileInfo, error) {
	if f.files[name] {
		return fakeFileInfo{name: name, dir: false}, nil
	}
	if f.dirs[name] {
		return fakeFileInfo{name: name, dir: true}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: os.ErrNotExist}
}

func (f *fakeFS) ReadDir(_ string) ([]os.DirEntry, error) {
	return f.entries, nil
}

// scriptedCmd records the stdout/stderr payload and exit error returned
// by the fake executor for a matching command key.
type scriptedCmd struct {
	stdout string
	stderr string
	err    error
}

// fakeExecutor implements workspaceapi.Executor. It dispatches on the
// space-joined (cmd.Path, cmd.Args...) key. Unknown commands error from
// Start.
type fakeExecutor struct {
	mu        sync.Mutex
	responses map[string]scriptedCmd
	calls     []string
	nextPid   workspaceapi.Pid
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{responses: map[string]scriptedCmd{}, nextPid: 1}
}

func (e *fakeExecutor) respond(key string, r scriptedCmd) *fakeExecutor {
	e.responses[key] = r
	return e
}

func (e *fakeExecutor) callKey(cmd workspaceapi.Cmd) string {
	return strings.Join(append([]string{cmd.Path}, cmd.Args...), " ")
}

func (e *fakeExecutor) Start(_ context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
	e.mu.Lock()
	key := e.callKey(cmd)
	e.calls = append(e.calls, key)
	resp, ok := e.responses[key]
	pid := e.nextPid
	e.nextPid++
	e.mu.Unlock()

	if !ok {
		return 0, fmt.Errorf("fakeExecutor: no scripted response for %q", key)
	}
	if cmd.Stdout != nil && resp.stdout != "" {
		_, _ = io.Copy(cmd.Stdout, bytes.NewBufferString(resp.stdout))
	}
	if cmd.Stderr != nil && resp.stderr != "" {
		_, _ = io.Copy(cmd.Stderr, bytes.NewBufferString(resp.stderr))
	}
	if cmd.Watcher != nil {
		ch := cmd.Watcher.WatchProcess()
		go func(err error) {
			if ch != nil {
				ch <- err
			}
		}(resp.err)
	}
	return pid, nil
}

func (e *fakeExecutor) Signal(_ workspaceapi.Pid, _ syscall.Signal) error { return nil }
func (e *fakeExecutor) Close() error                                      { return nil }

func (e *fakeExecutor) callsSnapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.calls))
	copy(out, e.calls)
	return out
}

func TestResolvePyTool(t *testing.T) {
	for _, name := range []string{"ty", "ruff", "uv"} {
		t.Run(name+"/resolves DataDir/bin", func(t *testing.T) {
			fs := newFakeFS().
				addFile("/Users/u/.rune/bin/" + name)
			ex := newFakeExecutor()
			got := resolvePyTool(context.Background(), fs, ex, "/Users/u/.rune", name)
			assert.Equal(t, "/Users/u/.rune/bin/"+name, got)
			assert.Empty(t, ex.callsSnapshot())
		})

		t.Run(name+"/directory at install path is ignored", func(t *testing.T) {
			fs := newFakeFS().addDir("/Users/u/.rune/bin/" + name)
			ex := newFakeExecutor()
			got := resolvePyTool(context.Background(), fs, ex, "/Users/u/.rune", name)
			assert.Equal(t, "", got)
			assert.Empty(t, ex.callsSnapshot())
		})

		t.Run(name+"/missing returns empty without probing", func(t *testing.T) {
			fs := newFakeFS()
			ex := newFakeExecutor()
			got := resolvePyTool(context.Background(), fs, ex, "/Users/u/.rune", name)
			assert.Equal(t, "", got)
			assert.Empty(t, ex.callsSnapshot())
		})
	}
}
