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

package idelsp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// TestEventTypeClose_evictsFilesCache locks in the fix for RUNE-195:
// Manager.files retained the full text of every URI ever opened in a
// language-supported buffer because EventTypeClose only emitted
// textDocument/didClose without touching m.files. The handler must
// drop the cached entry so long-running sessions do not accumulate
// one full file copy per URI.
func TestEventTypeClose_evictsFilesCache(t *testing.T) {
	t.Parallel()
	workspaceURI := makeURI(t, "file:///workspace")
	m := New(workspaceURI, nil, nil, nil, nil, nil,
		Config{NoInitializeServer: true})
	t.Cleanup(func() { _ = m.Close() })

	openURI := makeURI(t, "file:///workspace/open.go")
	m.mu.Lock()
	m.files[openURI.String()] = newFile(openURI, "package open\n", "go")
	m.mu.Unlock()

	require.Len(t, m.files, 1)

	err := m.handle(textapi.Event{
		Type: textapi.EventTypeClose,
		URI:  openURI,
	})
	require.NoError(t, err)

	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Empty(t, m.files,
		"EventTypeClose must evict the cached file entry")
}

// TestManagerConcurrentStateAccess drives the file/server/pendingOpens
// accessors and Close concurrently to prove m.mu serialises every read
// and write of the manager's maps. With NoInitializeServer the open/close
// events stay in-process (no language server is spawned), so this is a
// pure -race regression guard for the locking around m.files,
// m.pendingOpens, and m.servers.
func TestManagerConcurrentStateAccess(t *testing.T) {
	t.Parallel()
	workspaceURI := makeURI(t, "file:///workspace")
	m := New(workspaceURI, nil, nil, nil, nil, nil,
		Config{NoInitializeServer: true})

	const workers = 8
	files := []workspaceapi.URI{
		makeURI(t, "file:///workspace/a.go"),
		makeURI(t, "file:///workspace/b.go"),
		makeURI(t, "file:///workspace/c.go"),
	}

	var wg sync.WaitGroup
	for i := range workers {
		uri := files[i%len(files)]
		wg.Go(func() {
			for range 50 {
				_ = m.handle(textapi.Event{Type: textapi.EventTypeOpen, URI: uri,
					Content: "package a\n"})
				m.mu.Lock()
				m.files[uri.String()] = newFile(uri, "package a\n", "go")
				m.mu.Unlock()
				_, _ = m.getFile(uri.String())
				_ = m.allServers()
				_ = m.handle(textapi.Event{Type: textapi.EventTypeClose, URI: uri})
			}
		})
	}

	wg.Go(func() {
		_ = m.Close()
	})

	wg.Wait()
}

// TestManagerMaxRetriesPropagatesFromConfig locks in the fix for Bug B
// in RUNE-132: Config.MaxRetries was read for default-filling but
// never assigned to Manager.maxRetries, so retry.LimitStrategy
// silently received zero and watchServer never restarted a crashed
// language server.
func TestManagerMaxRetriesPropagatesFromConfig(t *testing.T) {
	uri := makeURI(t, "file:///workspace")

	t.Run("explicit value is propagated", func(t *testing.T) {
		m := New(uri, nil, nil, nil, nil, nil, Config{MaxRetries: 7})
		t.Cleanup(func() { _ = m.Close() })
		assert.Equal(t, uint(7), m.maxRetries)
	})

	t.Run("zero falls back to default", func(t *testing.T) {
		m := New(uri, nil, nil, nil, nil, nil, Config{})
		t.Cleanup(func() { _ = m.Close() })
		assert.Equal(t, uint(3), m.maxRetries)
	})
}

// TestDidChangeWatchedFiles_invalidatesOnDelete locks in the fix for
// RUNE-AGENT-72: a workspace/didChangeWatchedFiles batch that contains
// a Deleted event must call Callback.InvalidateAllPending so subsequent
// WaitFileProcessed calls block until gopls re-publishes diagnostics
// for unrelated files in the same package. Non-deletion batches must
// not trigger the invalidation.
func TestDidChangeWatchedFiles_invalidatesOnDelete(t *testing.T) {
	t.Parallel()
	uri := makeURI(t, "file:///workspace")

	t.Run("deletion event triggers InvalidateAllPending", func(t *testing.T) {
		t.Parallel()
		callback := &testCallback{}
		m := New(uri, nil, nil, nil, nil, nil,
			Config{Callback: callback, NoInitializeServer: true})
		t.Cleanup(func() { _ = m.Close() })

		err := m.DidChangeWatchedFiles(context.Background(),
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{URI: "file:///workspace/a.go", Type: semanticapi.FileChangeTypeChanged},
					{URI: "file:///workspace/b.go", Type: semanticapi.FileChangeTypeDeleted},
				},
			})
		require.NoError(t, err)

		callback.mu.Lock()
		got := callback.invalidateAllPendingCount
		callback.mu.Unlock()
		assert.Equal(t, 1, got)
	})

	t.Run("rename-as-delete+create triggers InvalidateAllPending", func(t *testing.T) {
		t.Parallel()
		callback := &testCallback{}
		m := New(uri, nil, nil, nil, nil, nil,
			Config{Callback: callback, NoInitializeServer: true})
		t.Cleanup(func() { _ = m.Close() })

		err := m.DidChangeWatchedFiles(context.Background(),
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{URI: "file:///workspace/old.go", Type: semanticapi.FileChangeTypeDeleted},
					{URI: "file:///workspace/new.go", Type: semanticapi.FileChangeTypeCreated},
				},
			})
		require.NoError(t, err)

		callback.mu.Lock()
		got := callback.invalidateAllPendingCount
		callback.mu.Unlock()
		assert.Equal(t, 1, got)
	})

	t.Run("add+change only does not trigger InvalidateAllPending", func(t *testing.T) {
		t.Parallel()
		callback := &testCallback{}
		m := New(uri, nil, nil, nil, nil, nil,
			Config{Callback: callback, NoInitializeServer: true})
		t.Cleanup(func() { _ = m.Close() })

		err := m.DidChangeWatchedFiles(context.Background(),
			semanticapi.DidChangeWatchedFilesParams{
				Changes: []semanticapi.FileEvent{
					{URI: "file:///workspace/a.go", Type: semanticapi.FileChangeTypeCreated},
					{URI: "file:///workspace/b.go", Type: semanticapi.FileChangeTypeChanged},
				},
			})
		require.NoError(t, err)

		callback.mu.Lock()
		got := callback.invalidateAllPendingCount
		callback.mu.Unlock()
		assert.Equal(t, 0, got)
	})
}

// TestDidChangeWatchedFiles_marksOpenFilePending locks in the fix for
// RUNE-AGENT-72 (round 2): when an apply_patch-style Changed event
// arrives for a file that is also open in the editor, the manager
// must still mark the URI as pending so a subsequent
// WaitFileProcessed blocks until gopls re-publishes diagnostics.
// Previously fileDidChangeOOB short-circuited for open files,
// causing check_file_errors to return a stale pre-patch snapshot.
func TestDidChangeWatchedFiles_marksOpenFilePending(t *testing.T) {
	t.Parallel()
	uri := makeURI(t, "file:///workspace")
	callback := &testCallback{}
	m := New(uri, nil, nil, nil, nil, nil,
		Config{Callback: callback, NoInitializeServer: true})
	t.Cleanup(func() { _ = m.Close() })

	// Simulate the file being open in the editor.
	openURI := makeURI(t, "file:///workspace/open.go")
	m.mu.Lock()
	m.files[openURI.String()] = newFile(openURI, "package open\n", "go")
	m.mu.Unlock()

	// An apply_patch-style Changed event arrives for the open file.
	err := m.DidChangeWatchedFiles(context.Background(),
		semanticapi.DidChangeWatchedFilesParams{
			Changes: []semanticapi.FileEvent{
				{URI: openURI.String(), Type: semanticapi.FileChangeTypeChanged},
			},
		})
	require.NoError(t, err)

	// The callback must have received an out-of-band change marked
	// as open so a subsequent WaitFileProcessed blocks for a newer
	// version rather than releasing on a stale push.
	callback.mu.Lock()
	got := callback.fileDidChangeCalls
	callback.mu.Unlock()
	require.Len(t, got, 1)
	assert.Equal(t, openURI.String(), got[0].uri)
	assert.True(t, got[0].open)
	assert.True(t, got[0].oob)
}

// TestWatchServerRestartsOnConnLoss locks in the fix that watchServer
// listens on srv.conn.Done() in addition to the process watcher.
// Closing the IDE-side jsonrpc2 conn while the gopls process is still
// alive must trigger a SIGKILL of the orphan plus a fresh server.
func TestWatchServerRestartsOnConnLoss(t *testing.T) {
	t.Parallel()
	goplsBin := findGopls(t)
	tmpDir := setupTestWorkspace(t, "testdata")

	uri := makeURI(t, "file://"+tmpDir)
	scheme := newTestScheme()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var ready sync.Once
	var wg sync.WaitGroup
	callback := &testCallback{
		onProgress: readyOnProgress(&ready, &wg),
	}

	mgr := New(
		uri, scheme, scheme,
		&stubPkgManager{bin: goplsBin},
		nil, nil,
		Config{Callback: callback, MaxRetries: 3, NoInitializeServer: true},
	)
	t.Cleanup(func() { _ = mgr.Close() })

	params := autoInitParams(uri.String())
	initOpts, err := json.Marshal(map[string]any{
		"langID":  "go",
		"command": "gopls serve",
	})
	require.NoError(t, err)
	params.InitializeOptions = initOpts

	wg.Add(1)
	_, err = mgr.Initialize(ctx, params)
	require.NoError(t, err)
	wg.Wait()

	mgr.mu.Lock()
	origSrv := mgr.servers["go"].(*langServer)
	mgr.mu.Unlock()
	require.NotNil(t, origSrv)
	origPid := origSrv.pid

	// Tear down the jsonrpc2 connection without touching the
	// child process. Closing the IDE-side stdin/stdout breaks
	// the readIncoming goroutine, which closes the conn's done
	// channel.
	require.NoError(t, origSrv.stdin.Close())
	require.NoError(t, origSrv.stdout.Close())

	require.Eventually(t, func() bool {
		select {
		case <-origSrv.conn.Done():
			return true
		default:
			return false
		}
	}, 5*time.Second, 50*time.Millisecond,
		"original conn never reported Done")

	// The manager must now kill the orphan process and start a new server.
	var newSrv *langServer
	require.Eventually(t, func() bool {
		mgr.mu.Lock()
		s, _ := mgr.servers["go"].(*langServer)
		mgr.mu.Unlock()
		if s != nil && s != origSrv {
			newSrv = s
			return true
		}
		return false
	}, 15*time.Second, 200*time.Millisecond,
		"server did not restart after conn loss")

	assert.NotEqual(t, origPid, newSrv.pid,
		"new server must have a different pid")
	assert.Equal(t, origSrv.params, newSrv.params,
		"InitializeParams must be preserved across conn-loss restart")

	// Sanity check the new server actually responds.
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	var raw semanticapi.DocumentSymbolResult
	_ = newSrv.call(pingCtx, "workspace/symbol",
		semanticapi.WorkspaceSymbolParams{Query: ""}, &raw)
}

// TestManagerCloseTerminatesGopls locks in the RUNE-180 contract that
// Manager.Close propagates cancellation down to spawned language-server
// processes (m.ctx → langServer.ctx → exec.CommandContext-bound child).
func TestManagerCloseTerminatesGopls(t *testing.T) {
	t.Parallel()
	goplsBin := findGopls(t)
	tmpDir := setupTestWorkspace(t, "testdata")

	uri := makeURI(t, "file://"+tmpDir)
	scheme := newTestScheme()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var ready sync.Once
	var wg sync.WaitGroup
	callback := &testCallback{
		onProgress: readyOnProgress(&ready, &wg),
	}

	mgr := New(
		uri, scheme, scheme,
		&stubPkgManager{bin: goplsBin},
		nil, nil,
		Config{Callback: callback, MaxRetries: 3, NoInitializeServer: true},
	)

	params := autoInitParams(uri.String())
	initOpts, err := json.Marshal(map[string]any{
		"langID":  "go",
		"command": "gopls serve",
	})
	require.NoError(t, err)
	params.InitializeOptions = initOpts

	wg.Add(1)
	_, err = mgr.Initialize(ctx, params)
	require.NoError(t, err)
	wg.Wait()

	mgr.mu.Lock()
	srv := mgr.servers["go"].(*langServer)
	mgr.mu.Unlock()
	require.NotNil(t, srv)
	require.NotNil(t, srv.watcher,
		"langServer must have a process watcher so close can be observed")

	require.NoError(t, mgr.Close())

	require.Error(t, mgr.ctx.Err(),
		"Manager.Close must cancel m.ctx so handleEvs / watchServer exit")

	select {
	case <-srv.watcher:
	case <-time.After(10 * time.Second):
		t.Fatal("gopls child did not exit within 10s of Manager.Close: " +
			"manager teardown is not propagating cancellation to the child")
	}
}
