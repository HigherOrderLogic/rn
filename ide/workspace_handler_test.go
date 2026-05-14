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

package ide

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	_ "net/http/pprof"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release/docrelease"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/docmarshal/doctoml"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/idetask"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/term/vte/vtereservoir"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacetest"
)

func TestFileCommandRegistryIntegration(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	uri1, err := workspaceapi.ParseURI("memory://" + dir)
	require.NoError(t, err)

	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.drainPendingWorkspaces()

	h := newSafeHandler(m)
	// jumptolocation is registered on a per-file basis, so the following tests
	// file-level subscriptions across a file's lifecycle.
	cases := []handlertest.SequenceTestCase{
		{":edit dakar.md>igentleman>driver>gentleman<:write>/gentleman>:jumptolocation next search>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│▐entleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next search>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐entleman                   │
│driver                      │
│gentleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":tabclose>:edit dakar.md>/gentleman>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│▐entleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next search>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐entleman                   │
│driver                      │
│gentleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":foldexpandall>", // this fails if not installed correctly
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐entleman                   │
│driver                      │
│gentleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, h, 30, 9, cases)

	require.NoError(t, m.Close())
}

// TestFileExplorerEnterDelegatesIntegration exercises the real
// :fexplorer integration path with both real text.Editor
// implementations (vi-style modal and modeless). Regression
// coverage for RUNE-138: when the file explorer's inner editor
// reports IsSearchMode() == true, pressing <Enter> must delegate
// to the inner editor (committing the search) rather than be
// captured by the outer fileExplorerHandler as expand-or-open.
//
// The IsSearchMode() signal must propagate from the leaf editor
// handler (vi.Vi or modeless.editorHandler) up through every
// wrapper that text.Editor.Edit installs — text.Publisher's
// cursorPublisher, fold/location/indent/comment/git command
// wrappers, status/aux/icons bars — and reach
// fileExplorerHandler.ed.IsSearchMode(). Because text.Handler
// declares IsSearchMode(), this propagation happens through
// interface embedding on each wrapper.
//
// vi has a key-driven inline search ('/'), so for modal we drive
// the bug repro end-to-end: '/findme<Enter>' must finish the
// search and not toggle the tree. modeless does not have a
// key-driven inline search (text search in modeless is surfaced
// through a separate fuzzy_search extension command), so we
// instead assert the non-search-mode behavior is preserved end-
// to-end: <Enter> still toggles the tree exactly as before.
func TestFileExplorerEnterDelegatesIntegration(t *testing.T) {
	cases := []struct {
		name string
		mode string
	}{
		{name: "modal", mode: editorModeModal},
		{name: "modeless", mode: editorModeModeless},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, "findme.txt"), []byte("hi"), 0o644))
			require.NoError(t, os.MkdirAll(
				filepath.Join(dir, "subdir"), 0o755))
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, "subdir", "child.txt"),
				[]byte("hi"), 0o644))

			cfg := defaultConfigWithWrap(false)
			editorCfg := cfg.cfg["editor"].(map[string]any)
			editorCfg["mode"] = tc.mode
			cfg.cfg["editor"] = editorCfg
			require.Equal(t, tc.mode, cfg.editorMode())

			uri, err := workspaceapi.ParseURI("file://" + dir)
			require.NoError(t, err)
			m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir,
				nopShutdownShaderConfig())
			require.NoError(t, m.addOrCreateWorkspace(uri))
			m.drainPendingWorkspaces()
			t.Cleanup(func() { _ = m.Close() })

			h := newSafeHandler(m)
			h.Resize(40, 12)

			ex := m.focusEx()
			require.NotNil(t, ex)
			// ex.fexplorer constructs newFileExplorerHandler, which the
			// async FS-watcher goroutine spawned by Phase C reads from
			// under m.mu. Take m.mu while building the explorer so its
			// internal state is published before the watcher dispatches
			// a Create event into eventApplies.
			m.mu.Lock()
			err = ex.fexplorer(context.Background())
			m.mu.Unlock()
			require.NoError(t, err)
			require.NotNil(t, ex.fileExplorerWin,
				"file explorer must be open")
			require.NotNil(t, ex.fileExplorerHandler)

			explorer := ex.fileExplorerHandler
			require.False(t, explorer.ed.IsSearchMode(),
				"IsSearchMode must propagate through the full "+
					"wrapper chain and report false initially")

			beforeRows := explorer.ed.CellView().Rows()
			require.Greater(t, beforeRows, 0)

			switch tc.mode {
			case editorModeModal:
				// Bug repro: enter search mode and type a query.
				_, handled := h.Handle(term.Event{
					Type: term.EventKey, Ch: '/',
				})
				require.True(t, handled,
					"'/' must enter vi search mode")
				require.True(t, explorer.ed.IsSearchMode(),
					"vi must be in search mode after '/'")

				for _, r := range "findme" {
					_, handled = h.Handle(term.Event{
						Type: term.EventKey, Ch: r,
					})
					require.True(t, handled,
						"typed char %q must be handled", r)
				}
				require.True(t, explorer.ed.IsSearchMode(),
					"vi must still be in search mode while "+
						"typing the query")

				_, handled = h.Handle(term.Event{
					Type: term.EventKey, Key: term.KeyEnter,
				})
				require.True(t, handled,
					"<Enter> must be handled")
				// Bug regression: <Enter> in search mode must
				// commit the search and exit search mode rather
				// than be captured by the outer file explorer.
				require.False(t, explorer.ed.IsSearchMode(),
					"<Enter> in search mode must commit the "+
						"inner search and exit search mode")
				require.Equal(t, beforeRows,
					explorer.ed.CellView().Rows(),
					"<Enter> in search mode must not expand "+
						"or collapse a tree node")
				for _, tab := range ex.comp.Browser().Tabs() {
					require.NotEqual(t,
						"file://"+filepath.Join(dir, "findme.txt"),
						tab.URI().String(),
						"<Enter> in search mode must not open "+
							"a file from the explorer")
				}

			case editorModeModeless:
				// modeless has no key-driven inline search in the
				// explorer; verify the non-search-mode behavior
				// still holds end-to-end through the full
				// wrapper chain. With the cursor on the first
				// (directory) row, <Enter> must expand the tree
				// — i.e. row count increases.
				_, handled := h.Handle(term.Event{
					Type: term.EventKey, Key: term.KeyEnter,
				})
				require.True(t, handled,
					"<Enter> must be handled")
				require.False(t, explorer.ed.IsSearchMode(),
					"modeless must not be in search mode after "+
						"<Enter> on a non-search context")
				require.NotEqual(t, beforeRows,
					explorer.ed.CellView().Rows(),
					"<Enter> outside search mode must still "+
						"toggle the tree (regression guard)")
			}
		})
	}
}

// TestFileExplorerReactsToFilesystemChangesIntegration is a
// black-box regression test for the cached file explorer becoming
// stale after the user closes it, creates a new file, and re-opens
// it. The fileExplorerHandler is cached across :fexplorer toggles,
// so without the FS-watcher subscription the second open would
// render the on-disk snapshot from when the explorer was first
// constructed and miss the newly-written file.
//
// The flow exercised end-to-end through the production handler
// chain (driven by handlertest.RunHandlerSequence with exact-frame
// assertions on each step) is:
//
//  1. Open the explorer via :fexplorer; alpha.go and beta.go must
//     render in the tree.
//  2. Close the explorer via :fexplorer (toggle).
//  3. Open and write gamma.go via :edit / :write so the FS watcher
//     fires a create event under the workspace root. vi
//     transiently creates `.gamma.go.swp` while the editor is
//     open, but the explorer's gitignore-derived ignore matcher
//     (RUNE-143) hides `*.swp` so the swap file does not pollute
//     the rendered tree.
//  4. Re-open the explorer via :fexplorer; the rendered tree must
//     now include gamma.go.
//
// The watcher delivery is asynchronous, so step 4's frame is
// asserted via require.Eventually that calls RunHandlerSequence
// with no input keys (just redrawing) until the expected frame is
// produced.
func TestFileExplorerReactsToFilesystemChangesIntegration(t *testing.T) {
	// Resolve symlinks so the workspace URI matches the canonical
	// path emitted by the FS watcher. On macOS t.TempDir() returns
	// /var/folders/... but the kqueue watcher reports
	// /private/var/folders/...; without canonicalisation the
	// explorer's prefix check would reject every event.
	rawDir := t.TempDir()
	dir, err := filepath.EvalSymlinks(rawDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "alpha.go"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.go"), []byte("b"), 0o644))

	uri, err := workspaceapi.ParseURI("file://" + dir)
	require.NoError(t, err)
	// Use a separate dataDir so the pkgmanager's .db/ scratch
	// directory doesn't appear inside the workspace and pollute
	// the rendered tree.
	m := newTestWorkspaceManagerHandlerWithDirs(t,
		defaultConfigWithWrap(false), dir, t.TempDir(),
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri))
	m.drainPendingWorkspaces()
	t.Cleanup(func() { _ = m.Close() })

	h := newSafeHandler(m)
	const width, height = 30, 9

	openedFrame := strings.Join([]string{
		"┌────────────────────────────┐",
		"│                            │",
		"┌────────────┐┌──────────────┤",
		"│ ▐ alpha.go ││              │",
		"│ o beta.go  ││workspaceWallp│",
		"│            ││              │",
		"├────────────┘└──────────────┤",
		"│1 1  2 2                    │",
		"└─────━━━────────────────────┘",
	}, "\n")
	closedFrame := strings.Join([]string{
		"┌────────────────────────────┐",
		"│                            │",
		"├────────────────────────────┤",
		"│                            │",
		"│     workspaceWallpaper     │",
		"│                            │",
		"├────────────────────────────┤",
		"│1 1  2 2                    │",
		"└─────━━━────────────────────┘",
	}, "\n")
	editorFrame := strings.Join([]string{
		"┌━━━━━━━━━━──────────────────┐",
		"│o gamma.go                  │",
		"├────────────────────────────┤",
		"│hell▐                       │",
		"│                            │",
		"│                      NORMAL│",
		"├────────────────────────────┤",
		"│1 1  2 2                    │",
		"└─────━━━────────────────────┘",
	}, "\n")
	// Final frame after re-opening the explorer: the on-disk
	// gamma.go entry must have been picked up by the FS-watcher
	// subscription and rendered in the tree. The buffer pane on
	// the right still shows the gamma.go editor opened in step 3.
	reopenedFrame := strings.Join([]string{
		"┌────────────────────────────┐",
		"│o gamma.go                  │",
		"┌────────────┐┌──────────────┤",
		"│ ▐ alpha.go ││hello         │",
		"│ o beta.go  ││              │",
		"│ o gamma.go ││        NORMAL│",
		"├────────────┘└──────────────┤",
		"│1 1  2 2                    │",
		"└─────━━━────────────────────┘",
	}, "\n")

	// Steps 1–3: drive the production handler chain with
	// RunHandlerSequence and assert the exact rendered frame at
	// each step. The toggle is invoked via :fexplorer rather than
	// a <tab> key binding because the test config's empty
	// workspace pre-focus does not propagate <tab> to the
	// command-key dispatcher; TestFileExplorerToggleViaTabKey
	// covers the <tab>-binding code path with a focused editor.
	//
	//  1. Open the file explorer: both pre-existing files must
	//     render in the tree.
	//  2. Close the file explorer with the toggle command.
	//  3. :edit gamma.go, type "hello", :write — flushes the new
	//     file to disk under the workspace root which fires the
	//     FS watcher.
	handlertest.RunHandlerSequence(t, h, width, height,
		[]handlertest.SequenceTestCase{
			{
				InputSequence: "<c-\\\\>fexplorer<enter>",
				Expected:      openedFrame,
			},
			{
				InputSequence: "<c-\\\\>fexplorer<enter>",
				Expected:      closedFrame,
			},
			{
				InputSequence: "<c-\\\\>edit<space>gamma.go<enter>" +
					"ihello<esc><c-\\\\>write<enter>",
				Expected: editorFrame,
			},
		})

	// Step 4: re-open the explorer; the rendered frame must now
	// include gamma.go alongside the pre-existing entries. The FS
	// watcher delivers its create event on a separate goroutine
	// (kqueue on macOS, inotify on Linux) with platform-dependent
	// latency, so retry the final RunHandlerSequence under
	// require.Eventually until the FS-watcher subscription has
	// rendered gamma.go in the tree. The explorer toggle in step
	// 2 closed the window, so the first iteration opens it; every
	// subsequent iteration is a no-op redraw (empty
	// InputSequence) because re-issuing the toggle would close
	// the just-opened explorer.
	require.Eventually(t, func() bool {
		fakeT := &testing.T{}
		input := "<c-\\\\>fexplorer<enter>"
		if m.focusEx().fileExplorerWin != nil {
			input = ""
		}
		handlertest.RunHandlerSequence(fakeT, h, width, height,
			[]handlertest.SequenceTestCase{{
				InputSequence: input,
				Expected:      reopenedFrame,
			}})
		return !fakeT.Failed()
	}, 5*time.Second, 25*time.Millisecond,
		"reopened explorer must render gamma.go after the FS "+
			"watcher delivers the create event for the new file")
}

// TestGitlinkIntegration exercises the :gitlink command end-to-end
// against a real on-disk git repository, using the same editor wiring
// production uses (newBuiltinModal/ModelessEditor →
// vctrlcmd.SubscribeGitCommands). It is a regression guard for a nil
// pointer dereference at vctrlcmd/remote_web_link.go:195: vi.Editor
// did not seed its viConfig with defaults, so the notifications
// interface forwarded into copyRemoteURL was nil and Notify panicked
// the moment :gitlink completed successfully.
//
// The test asserts that:
//   - :gitlink does not panic;
//   - the resulting URL is copied to the clipboard;
//   - a success notification is rendered on screen with the URL of
//     the file currently open at the cursor position.
func TestGitlinkIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}

	for _, mode := range []string{editorModeModal, editorModeModeless} {
		t.Run(mode, func(t *testing.T) {
			dir, commit, relFile := setupGitlinkRepo(t)

			cfg := defaultConfigWithWrap(false)
			editorCfg := cfg.cfg["editor"].(map[string]any)
			editorCfg["mode"] = mode
			cfg.cfg["editor"] = editorCfg
			require.Equal(t, mode, cfg.editorMode())

			uri, err := workspaceapi.ParseURI("file://" + dir)
			require.NoError(t, err)
			m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir,
				nopShutdownShaderConfig())
			require.NoError(t, m.addOrCreateWorkspace(uri))
			m.drainPendingWorkspaces()
			t.Cleanup(func() { _ = m.Close() })

			h := newSafeHandler(m)

			expectedURL := fmt.Sprintf(
				"https://github.com/unstablebuild/gitproj/blob/%s/%s#L1",
				commit, relFile)

			// Open the file via :edit so the full vi/modeless
			// editor wrapper chain (SubscribeGitCommands included)
			// is installed for the file's text.Handler — exactly
			// the path that panicked in production. Then run
			// :gitlink. The notification box wraps "copied <url>"
			// at 11 columns (15 cols wide minus borders/padding)
			// and spans the full editor height, occluding the
			// status row. Asserting the rendered framebuffer
			// guarantees the notification reached Draw, not just
			// Notify.
			cases := []handlertest.SequenceTestCase{{
				InputSequence: `<c-\\>edit<space>` + relFile +
					`<enter><c-\\>gitlink<enter>`,
				Expected: strings.Join([]string{
					"┌━━━━━━━━━━━━━━──────────────────────────────────────────────────┌─────────────┐",
					"│o guasacaca.md                                                  │ copied      │",
					"├────────────────────────────────────────────────────────────────│ https://git │",
					"│▐i                                                              │ hub.com/uns │",
					"│                                                                │ tablebuild/ │",
					"│                                                                │ gitproj/blo │",
					"│                                                                │ b/d8b96a860 │",
					"│                                                                │ 305b0c87524 │",
					"│                                                                │ da7eaae159f │",
					"├────────────────────────────────────────────────────────────────│ 25dd233db/r ┤",
					"│1 1  2 2                                                                      │",
					"└─────━━━──────────────────────────────────────────────────────────────────────┘",
				}, "\n"),
			}}
			require.NotPanics(t, func() {
				handlertest.RunHandlerSequence(t, h, 80, 12, cases)
			})

			paste, err := m.clip.Paste(clipboard.DefaultRegisterID)
			require.NoError(t, err)
			assert.Equal(t, expectedURL, paste.Text,
				":gitlink must copy the web URL of the file "+
					"under the cursor to the clipboard")
		})
	}
}

// setupGitlinkRepo creates a git repo in a fresh temp dir with a
// single committed file under recipes/ and a github origin remote.
// Author/committer dates are pinned so the resulting commit hash is
// deterministic across runs and across machines.
func setupGitlinkRepo(t *testing.T) (dir, commit, relFile string) {
	t.Helper()
	dir = t.TempDir()
	// EvalSymlinks because git returns canonical paths on macOS
	// (/private/var/folders/...) and the workspace path must match
	// for vctrl.RelPath to succeed.
	canonical, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	dir = canonical

	relFile = filepath.Join("recipes", "guasacaca.md")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "recipes"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, relFile), []byte("hi\n"), 0o644))

	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
	)
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"remote", "add", "origin",
			"https://github.com/unstablebuild/gitproj.git"},
		{"add", "."},
		{"commit", "-q", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = gitEnv
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	commit = strings.TrimSpace(string(out))
	return
}

func TestSetTabNameWithAttrIntegration(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	uri1, err := workspaceapi.ParseURI("file://" + dir)
	require.NoError(t, err)
	cfg := defaultConfigWithWrap(false)
	var wg sync.WaitGroup
	cfg.ringBell = func() {
		wg.Done()
	}
	m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir,
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.drainPendingWorkspaces()
	m.tabAttentionNameSuffix = "*"

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{InputSequence: "<c-\\\\>terminalnewtab<enter>" +
			"<c-\\\\>tabrename<space>terminal<enter>", // avoid dynamic tty name
			Expected: `┌━━━━━━━━━━──────────────────┐
│$ terminal                  │
├────────────────────────────┤
│▐                           │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
	}
	// safeHandler internally serializes Resize/Draw/Handle on
	// m.mu, so no manual lock is required here. The default
	// scheduleNextTick stub installed by
	// newTestWorkspaceManagerHandlerWithManagerAndExtensions
	// dispatches scheduled callbacks under m.mu on a fresh
	// goroutine, so any host-scheduled work is already
	// serialized with handler input through the same lock.
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)
	keys, err := term.ParseKeys("sleep<space>2<space>&&<space>printf<space>'\\\\a'<enter>")
	require.NoError(t, err)
	wg.Add(1)
	for _, key := range keys {
		ev := term.Event{
			Type: term.EventKey,
			Ch:   key.Ch,
			Mod:  key.Mod,
			Key:  key.Key,
		}
		if ev.Ch != 0 {
			ev.Raw = []byte(string(ev.Ch))
		} else if ev.Key == term.KeySpace {
			ev.Raw = []byte(" ")
		} else if ev.Key == term.KeyEnter {
			ev.Raw = []byte{0x0d, 0x0a}
		} else if ev.Mod == term.ModShift && ev.Ch == '7' {
			ev.Raw = []byte("&")
		}
		_, handled := h.Handle(ev)
		require.True(t, handled, "%s", ev.KeyComb().String())
	}
	keys, err = term.ParseKeys("<c-\\\\>workspacefocus<space>1<enter>")
	require.NoError(t, err)
	for _, key := range keys {
		ev := term.Event{
			Type: term.EventKey,
			Ch:   key.Ch,
			Mod:  key.Mod,
			Key:  key.Key,
		}
		_, handled := h.Handle(ev)
		require.True(t, handled, "%s", ev.KeyComb().String())
	}
	wg.Wait()
	// SetTabName routes the attention attribute through
	// f.parent.scheduleNextTick, which under the default test stub
	// dispatches on a fresh goroutine. The bell fires from the vte
	// parser path before that scheduled tick runs, so wg.Wait above
	// only proves the bell was rung — it does not guarantee the
	// attention attr propagated to workspaces[1] yet. Block here
	// until the bar tab actually reflects the attention attr so the
	// final render is deterministic.
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(m.workspaces) < 2 || m.workspaces[1] == nil {
			return false
		}
		return m.workspaces[1].attentionAttr != (term.Attributes{})
	}, 5*time.Second, 5*time.Millisecond,
		"workspace attention attr never propagated after bell")
	cases = []handlertest.SequenceTestCase{
		{InputSequence: "",
			Expected: `┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1 1  2 2*                   │
└━━━─────────────────────────┘`},
	}
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)
	require.NoError(t, m.Close())
}

func TestCrossWorkspaceNotifications(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	uri1, err := workspaceapi.ParseURI("memory://" + dir)
	require.NoError(t, err)
	uri2, err := workspaceapi.ParseURI("memory:///b")
	require.NoError(t, err)

	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.drainPendingWorkspaces()
	require.NoError(t, m.addOrCreateWorkspace(uri2))
	m.drainPendingWorkspaces()

	m.workspaces[0].notifications.Notify(browserapi.LevelError, "sh")

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{"",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1 #  2 #                    │
└─────###────────────────────┘`},
	}
	handlertest.RunHandlerSequenceWriter(t, newWriterForAttrTesting(30, 9),
		h, 30, 9, cases)

	require.NoError(t, m.Close())
}

func TestCustomLocations(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	uri1, err := workspaceapi.ParseURI("memory://" + dir)
	require.NoError(t, err)

	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.drainPendingWorkspaces()

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{":edit dakar.md>igentleman<:locationcreate mylist>a>driver<:locationcreate mylist>a>gentleman<:write>:locationcreate mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│gentlema▐                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation previous mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation previous mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentlema▐                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":locationdelete mork>",
			`┌━━━━━━━━━━────┌─────────────┐
│o dakar.md    │ there's no  │
├──────────────│ location    │
│gentlema▐     │ at the      │
│driver        │ given       │
│gentleman     │ cursor      │
│              │ position    │
│              │ for given   │
└──────────────│ location    │`},
		{":noticloseall>:locationdelete mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentlema▐                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│gentlema▐                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":locationdeleteall mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{"k:locationtoggle mylist>j:locationtoggle mylist>:jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentl▐man                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":locationtoggle mylist>:jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentl▐man                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentl▐man                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, h, 30, 9, cases)

	require.NoError(t, m.Close())
}

func TestApostropheMarkJump(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	uri1, err := workspaceapi.ParseURI("memory://" + dir)
	require.NoError(t, err)

	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.drainPendingWorkspaces()

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		// Create file with leading spaces on a line, set mark at column 6
		{InputSequence: "<c-\\\\>edit<space>test.md<enter>i<space><space><space>hello<enter>world<esc><c-\\\\>write<enter>k$ma",
			Expected: `┌━━━━━━━━━───────────────────┐
│o test.md                   │
├────────────────────────────┤
│   hell▐                    │
│world                       │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		// Move to next line
		{InputSequence: "j",
			Expected: `┌━━━━━━━━━───────────────────┐
│o test.md                   │
├────────────────────────────┤
│   hello                    │
│worl▐                       │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		// 'a jumps to first non-blank character on marked line
		{InputSequence: "'a",
			Expected: `┌━━━━━━━━━───────────────────┐
│o test.md                   │
├────────────────────────────┤
│   ▐ello                    │
│world                       │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		// Move away again
		{InputSequence: "j",
			Expected: `┌━━━━━━━━━───────────────────┐
│o test.md                   │
├────────────────────────────┤
│   hello                    │
│wor▐d                       │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		// `a jumps to exact mark position (column 6)
		{InputSequence: "`a",
			Expected: `┌━━━━━━━━━───────────────────┐
│o test.md                   │
├────────────────────────────┤
│   hell▐                    │
│world                       │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
	}
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)

	require.NoError(t, m.Close())
}

func TestOpenFilesinEmptyWorkspace(t *testing.T) {
	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), "",
		nopShutdownShaderConfig())

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{"<c-\\\\>edit<space>dakar.md<enter>",
			`┌━━━━━━━━━━──────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐                           │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1                           │
└━───────────────────────────┘`},
		{"<c-\\\\>tabclose<enter>",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1                           │
└━───────────────────────────┘`},
	}
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)

	require.NoError(t, m.Close())
}

func TestEditFileURIRedirectsToWorkspaceWithOpenFile(t *testing.T) {
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()
	sharedDir := t.TempDir()
	sharedPath := filepath.Join(sharedDir, "shared.txt")
	require.NoError(t, os.WriteFile(sharedPath, []byte("shared"), 0o666))

	cfg := defaultConfigWithWrap(false)
	m := newTestWorkspaceManagerHandlerWithDir(t, cfg, tmp1, nopShutdownShaderConfig())
	defer func() {
		require.NoError(t, m.Close())
	}()

	uri2, err := workspaceapi.ParseURI("file://" + tmp2)
	require.NoError(t, err)
	require.NoError(t, m.addOrCreateWorkspace(uri2))
	m.drainPendingWorkspaces()

	sharedURI, err := workspaceapi.ParseURI("file://" + sharedPath)
	require.NoError(t, err)

	openTab, err := m.workspaces[1].ex.editFileURI(
		sharedURI, m.workspaces[1].ex.invokeWindow(), false)
	require.NoError(t, err)
	m.workspaces[1].ex.Wait()

	require.True(t, m.switchToWorkspace(0))
	redirectedTab, err := m.workspaces[0].ex.editFileURI(
		sharedURI, m.workspaces[0].ex.invokeWindow(), false)
	require.NoError(t, err)
	m.workspaces[0].ex.Wait()
	m.workspaces[1].ex.Wait()

	assert.Equal(t, 1, m.focus)
	assert.Same(t, openTab, redirectedTab)
	assert.Empty(t, m.workspaces[0].ex.comp.Tabs())

	focusTab, ok := m.workspaces[1].ex.comp.FocusTab()
	require.True(t, ok)
	assert.Same(t, openTab, focusTab)
	assert.True(t, focusTab.URI().Equal(sharedURI))
	assert.Len(t, m.workspaces[1].ex.comp.Tabs(), 1)
	_, ok = redirectedTab.Window()
	assert.True(t, ok)
}

func TestReadfileCrossWorkspaceIntegration(t *testing.T) {
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()

	currentPath := filepath.Join(tmp1, "current.txt")
	require.NoError(t, os.WriteFile(currentPath, []byte("current\n"), 0o666))
	foreignPath := filepath.Join(tmp2, "foreign.txt")
	require.NoError(t, os.WriteFile(foreignPath, []byte("foreign contents"), 0o666))

	cfg := defaultConfigWithWrap(false)
	cfg.scheduleNextTick = func(fn func()) bool {
		fn()
		return true
	}
	m := newTestWorkspaceManagerHandlerWithDir(t, cfg, "", nopShutdownShaderConfig())
	defer func() {
		require.NoError(t, m.Close())
	}()

	uri1, err := workspaceapi.ParseURI("file://" + tmp1)
	require.NoError(t, err)
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.drainPendingWorkspaces()

	uri2, err := workspaceapi.ParseURI("file://" + tmp2)
	require.NoError(t, err)
	require.NoError(t, m.addOrCreateWorkspace(uri2))
	m.drainPendingWorkspaces()

	require.True(t, m.switchToWorkspace(0))
	currentURI, err := workspaceapi.ParseURI("file://" + currentPath)
	require.NoError(t, err)
	_, err = m.workspaces[0].ex.editFileURI(currentURI, m.workspaces[0].ex.invokeWindow(), false)
	require.NoError(t, err)
	m.workspaces[0].ex.Wait()

	foreignURI, err := workspaceapi.ParseURI("file://" + foreignPath)
	require.NoError(t, err)
	require.NoError(t, m.workspaces[0].ex.readfile(context.Background(), foreignURI.String()))
	m.workspaces[0].ex.Wait()

	assert.Equal(t, 0, m.focus)
	_, h, ok := m.workspaces[0].ex.handlerInFocus()
	require.True(t, ok)
	assert.Equal(t, "current\nforeign contents", term.CellsToString(h.CellView().RawCells()))
	assert.Empty(t, m.workspaces[1].ex.comp.Tabs())
	_, ok = m.workspaces[1].ex.comp.FocusTab()
	assert.False(t, ok)
}

func TestCrossWorkspaceOpenRoutingIntegration(t *testing.T) {
	type integrationCase struct {
		name           string
		startWorkspace int
		setup          func(t *testing.T, m *testWorkspaceManagerHandler, tmp1, tmp2 string)
		sequences      []handlertest.SequenceTestCase
	}

	newManager := func(t *testing.T, tmp1, tmp2 string) *testWorkspaceManagerHandler {
		cfg := defaultConfigWithWrap(false)
		cfg.cfg["browser"] = map[string]any{
			"workspace_bar":      "number",
			"focus_tab_attr":     map[string]any{"bg": "blue"},
			"non_focus_tab_attr": map[string]any{"bg": "default"},
			"window_manager": map[string]any{
				"no_max_size": false,
			},
		}
		m := newTestWorkspaceManagerHandlerWithDir(t, cfg, "", nopShutdownShaderConfig())

		uri1, err := workspaceapi.ParseURI("file://" + tmp1)
		require.NoError(t, err)
		require.NoError(t, m.addOrCreateWorkspace(uri1))
		m.drainPendingWorkspaces()

		uri2, err := workspaceapi.ParseURI("file://" + tmp2)
		require.NoError(t, err)
		require.NoError(t, m.addOrCreateWorkspace(uri2))
		m.drainPendingWorkspaces()

		return m
	}

	setAliases := func(m *testWorkspaceManagerHandler, aliases map[string]text.CommandAlias) {
		for _, wh := range m.workspaces {
			if wh == nil || wh.ex == nil {
				continue
			}
			if wh.ex.config.CommandAliases == nil {
				wh.ex.config.CommandAliases = make(map[string]text.CommandAlias)
			}
			maps.Copy(wh.ex.config.CommandAliases, aliases)
		}
	}

	tests := []integrationCase{
		{
			name:           "local file stays in current workspace",
			startWorkspace: 0,
			setup: func(t *testing.T, m *testWorkspaceManagerHandler, tmp1, tmp2 string) {
				require.NoError(t, os.WriteFile(filepath.Join(tmp1, "local.go"), []byte("package main\n"), 0o666))
				setAliases(m, map[string]text.CommandAlias{
					"openCase": {Commands: []string{"edit local.go"}},
					"to1":      {Commands: []string{"workspacefocus 1"}},
					"to2":      {Commands: []string{"workspacefocus 2"}},
				})
			},
			sequences: []handlertest.SequenceTestCase{
				{
					InputSequence: "<c-\\\\>openCase<enter>",
					Expected: `┌━━━━━━━━━━──────────────────┐
│o ········                  │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 ·  2 2                    │
└━━━─────────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to2<enter>",
					Expected: `┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to1<enter>",
					Expected: `┌━━━━━━━━━━──────────────────┐
│o ········                  │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 ·  2 2                    │
└━━━─────────────────────────┘`,
				},
			},
		},
		{
			name:           "unopened foreign file switches to owning workspace",
			startWorkspace: 0,
			setup: func(t *testing.T, m *testWorkspaceManagerHandler, tmp1, tmp2 string) {
				ownedPath := filepath.Join(tmp2, "owned.go")
				require.NoError(t, os.WriteFile(ownedPath, []byte("package main\n"), 0o666))
				setAliases(m, map[string]text.CommandAlias{
					"openCase": {Commands: []string{fmt.Sprintf("edit file://%s", ownedPath)}},
					"to1":      {Commands: []string{"workspacefocus 1"}},
					"to2":      {Commands: []string{"workspacefocus 2"}},
				})
			},
			sequences: []handlertest.SequenceTestCase{
				{
					InputSequence: "<c-\\\\>openCase<enter>",
					Expected: `┌━━━━━━━━━━──────────────────┐
│o ········                  │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to1<enter>",
					Expected: `┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1 ·  2 2                    │
└━━━─────────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to2<enter>",
					Expected: `┌━━━━━━━━━━──────────────────┐
│o ········                  │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
			},
		},
		{
			name:           "existing foreign tab is focused in owning workspace",
			startWorkspace: 0,
			setup: func(t *testing.T, m *testWorkspaceManagerHandler, tmp1, tmp2 string) {
				sharedPath := filepath.Join(t.TempDir(), "shared.go")
				require.NoError(t, os.WriteFile(sharedPath, []byte("package main\n"), 0o666))
				sharedURI, err := workspaceapi.ParseURI("file://" + sharedPath)
				require.NoError(t, err)
				_, err = m.workspaces[1].ex.editFileURI(sharedURI, m.workspaces[1].ex.invokeWindow(), false)
				require.NoError(t, err)
				m.workspaces[1].ex.Wait()
				setAliases(m, map[string]text.CommandAlias{
					"openCase": {Commands: []string{fmt.Sprintf("edit file://%s", sharedPath)}},
					"to1":      {Commands: []string{"workspacefocus 1"}},
					"to2":      {Commands: []string{"workspacefocus 2"}},
				})
			},
			sequences: []handlertest.SequenceTestCase{
				{
					InputSequence: "<c-\\\\>openCase<enter>",
					Expected: `┌━━━━━━━━━━━─────────────────┐
│o ·········                 │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to1<enter>",
					Expected: `┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1 ·  2 2                    │
└━━━─────────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to2<enter>",
					Expected: `┌━━━━━━━━━━━─────────────────┐
│o ·········                 │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
			},
		},
		{
			name:           "focused owner opens locally without reroute",
			startWorkspace: 1,
			setup: func(t *testing.T, m *testWorkspaceManagerHandler, tmp1, tmp2 string) {
				ownedPath := filepath.Join(tmp2, "self.go")
				require.NoError(t, os.WriteFile(ownedPath, []byte("package main\n"), 0o666))
				setAliases(m, map[string]text.CommandAlias{
					"openCase": {Commands: []string{fmt.Sprintf("edit file://%s", ownedPath)}},
					"to1":      {Commands: []string{"workspacefocus 1"}},
					"to2":      {Commands: []string{"workspacefocus 2"}},
				})
			},
			sequences: []handlertest.SequenceTestCase{
				{
					InputSequence: "<c-\\\\>openCase<enter>",
					Expected: `┌━━━━━━━━━───────────────────┐
│o ·······                   │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to1<enter>",
					Expected: `┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1 ·  2 2                    │
└━━━─────────────────────────┘`,
				},
				{
					InputSequence: "<c-\\\\>to2<enter>",
					Expected: `┌━━━━━━━━━───────────────────┐
│o ·······                   │
├────────────────────────────┤
│▐ackage main                │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 ·                    │
└─────━━━────────────────────┘`,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp1 := t.TempDir()
			tmp2 := t.TempDir()
			m := newManager(t, tmp1, tmp2)
			t.Cleanup(func() {
				require.NoError(t, m.Close())
			})
			tc.setup(t, m, tmp1, tmp2)
			require.True(t, m.switchToWorkspace(tc.startWorkspace))

			h := newSafeHandler(m)
			writer := term.NewStringWriter(30, 9)
			writer.BackgroundCh = '·'
			handlertest.RunHandlerSequenceWriter(t, writer, h, 30, 9, tc.sequences)
		})
	}
}

func TestWorkspaceConfig(t *testing.T) {
	mockConfig := map[string]any{
		"1": "2",
		"2": map[string]any{
			"dos": "2",
			"two": "2",
		},
	}
	uri, err := workspaceapi.ParseURI("memory:///tmp")
	require.NoError(t, err)

	t.Run("passes default scheme config to SchemeFunc", func(t *testing.T) {
		cfg := defaultCfg()
		manager := workspace.NewManager(cfg.workspace())
		workspaceConfig := cfg.cfg["workspace"].(map[string]any)
		workspaceConfig[workspace.MemoryScheme] = mockConfig

		passed := make(map[string]any)
		manager.RegisterScheme(workspace.MemoryScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (
				schemeapi.Scheme, error,
			) {
				cfg.Iterate(func(k string, v any) {
					passed[k] = v
				})
				return workspace.NewMemoryScheme(ctx, cfg, uri)
			})

		m := newTestWorkspaceManagerHandlerWithManager(t, manager, uri, cfg)
		defer m.Close()

		assert.EqualValues(t, mockConfig, passed)
	})

	t.Run("notifies user if config decode fails but does not hard error", func(t *testing.T) {
		cfg := defaultCfg()
		manager := workspace.NewManager(cfg.workspace())
		workspaceConfig := cfg.cfg["workspace"].(map[string]any)
		workspaceConfig[workspace.MemoryScheme] = mockConfig

		passed := make(map[string]any)
		manager.RegisterScheme(workspace.MemoryScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (
				schemeapi.Scheme, error,
			) {
				cfg.Iterate(func(k string, v any) {
					passed[k] = v
				})
				return workspace.NewMemoryScheme(ctx, cfg, uri)
			})

		homeURI, err := workspaceapi.ParseURI("memory:///home")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		runner := FuncExtensionsRunner(testRunnerFn)

		m := new(testWorkspaceManagerHandler)
		m.workspaceManagerHandler = new(workspaceManagerHandler)

		mu := new(sync.Mutex)
		if cfg.scheduleNextTick == nil {
			// Mirror the production host event loop
			// (gui.Update / tui.Run): dispatched UserFuncs run
			// while holding the same locker the IDE was wired
			// with, on a fresh goroutine to keep the scheduler
			// non-reentrant.
			cfg.scheduleNextTick = func(fn func()) bool {
				go func() {
					mu.Lock()
					defer mu.Unlock()
					fn()
				}()
				return true
			}
		}
		releaseManager := docrelease.NewManager(document.NewInMemoryService())
		shRunner := new(shaderRunner)
		shRunner.init(
			handler.Nop(), term.NopInterrupter(), term.Attributes{},
			nopShutdownShaderConfig(), loadingShaderConfig{}, openShaderConfig{},
			component.FrameCharSetDefault())
		storage := localstorage.New(context.Background(), dir, doctoml.Marshaler())
		err = m.workspaceManagerHandler.init(&uri, homeURI, manager,
			notificationsConfig(), cfg, storage, dir,
			func(term.Event) bool {
				return true
			}, runner, mu, nil,
			func() (ideConfig, error) { return cfg, errors.New("boom") },
			".sixrc", 0, 0, '1', 0, 0, true, nil, releaseManager, shRunner, 0, nil)
		require.NoError(t, err)
		defer m.Close()
		m.drainPendingWorkspaces()

		assert.EqualValues(t, mockConfig, passed)

		cases := []handlertest.SequenceTestCase{
			{"",
				`┌────┌─────────────┐
│    │ Config      │
├────│ decode      │
│    │ error: boom │
│    └─────────────┘
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
		}
		h := &safeHandler{Component: m, Handler: m, mu: m.mu}
		handlertest.TestHandlerSequence(t, h, 20, 10, cases)
	})

	t.Run("does not reload workspace config", func(t *testing.T) {
		cfg := defaultCfg()
		manager := workspace.NewManager(cfg.workspace())
		workspaceConfig := cfg.cfg["workspace"].(map[string]any)
		workspaceConfig[workspace.MemoryScheme] = mockConfig

		passed := make(map[string]any)
		manager.RegisterScheme(workspace.MemoryScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (
				schemeapi.Scheme, error,
			) {
				cfg.Iterate(func(k string, v any) {
					passed[k] = v
				})
				return workspace.NewMemoryScheme(ctx, cfg, uri)
			})

		m := newTestWorkspaceManagerHandlerWithManager(t, manager, uri, cfg)
		assert.EqualValues(t, mockConfig, passed)

		m.reloadConfig = func() (ideConfig, error) {
			cfg := defaultCfg()
			cfg.cfg["workspace"].(map[string]any)["1"] = "!!!!"
			return cfg, nil
		}

		require.Equal(t, 0, m.height)
		require.Equal(t, 0, m.width)
		m.mu.Lock()
		require.NoError(t, m.commandReloadWorkspace())
		m.mu.Unlock()
		m.drainPendingWorkspaces()
		assert.EqualValues(t, mockConfig, passed)

		require.NoError(t, m.Close())
	})
}

func TestWorkspaceExtensions(t *testing.T) {
	t.Run("calls extension runner with user extensions", func(t *testing.T) {
		cfg := defaultCfg()
		cfg.cfg = map[string]any{
			"command":            map[string]any{},
			"show_manual_after":  "1h",
			"show_progress_hint": false,
			"extensions": map[string]any{
				"git": map[string]any{
					"path": "myPath",
					"config": map[string]any{
						"a": "b",
					},
				},
			},
		}
		manager := workspace.NewManager(cfg.workspace())

		manager.RegisterScheme(workspace.MemoryScheme, workspace.NewMemoryScheme)

		uri, err := workspaceapi.ParseURI("memory:///tmp")
		require.NoError(t, err)

		// extension.Runner.Run is called asynchronously
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		var uris [2]string
		var i atomic.Int32
		var wg sync.WaitGroup
		runner := FuncExtensionsRunner(
			func(_uri workspaceapi.URI,
				res map[extensionapi.Permission]extension.ResourceRegistrar,
				s string, noti browser.Notifications,
				exec, extExec schemeapi.Executor,
				grantor extension.Grantor,
				editor text.Editor,
				promptOpener ideauthorizer.PromptOpener, storage storageapi.Service,
				scheduleNextTick func(func()) bool,
			) (extension.Runner, error) {
				defer wg.Done()
				assert.NotNil(t, res)
				assert.Equal(t, dir, s)
				assert.NotNil(t, noti)
				assert.NotNil(t, exec)
				assert.NotNil(t, grantor)
				assert.NotNil(t, promptOpener)
				assert.NotNil(t, storage)
				assert.NotNil(t, scheduleNextTick)
				i := i.Add(1)
				uris[i-1] = _uri.String()
				return fnRunner{fn: func(extensionID, path string, cfg config.Config) error {
					assert.Equal(t, "myPath", path)
					assert.Equal(t, "git", extensionID)
					assert.Equal(t, config.MapConfig(map[string]any{"a": "b"}), cfg)
					return nil
				},
				}, nil
			})
		wg.Add(2)
		m := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
			&uri, cfg, runner, nil, dir, nil, nopShutdownShaderConfig())
		defer m.Close()

		wg.Wait()
		assert.ElementsMatch(t, []string{"memory:///home", "memory:///tmp"}, uris)
	})

	t.Run("calls extension runner with built-in extensions", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		cfg := defaultCfg()
		cfg.cfg = map[string]any{}
		manager := workspace.NewManager(cfg.workspace())

		manager.RegisterScheme(workspace.MemoryScheme, workspace.NewMemoryScheme)

		uri, err := workspaceapi.ParseURI("memory:///tmp")
		require.NoError(t, err)

		extensions := map[string]Extension{
			"myID": {
				ID:         "myID",
				CmdAndArgs: "myPath2",
				Config:     config.MapConfig(map[string]any{"a": "b"}),
			},
		}

		// extension.Runner.Run is called asynchronously
		var uris [2]string
		var wg sync.WaitGroup
		var i atomic.Int32
		runner := FuncExtensionsRunner(
			func(_uri workspaceapi.URI,
				res map[extensionapi.Permission]extension.ResourceRegistrar,
				s string, noti browser.Notifications,
				executor, extExec schemeapi.Executor,
				grantor extension.Grantor,
				editor text.Editor,
				promptOpener ideauthorizer.PromptOpener, storage storageapi.Service,
				scheduleNextTick func(func()) bool) (extension.Runner, error) {
				defer wg.Done()
				assert.NotNil(t, res)
				assert.Equal(t, dir, s)
				assert.NotNil(t, noti)
				assert.NotNil(t, executor)
				assert.NotNil(t, grantor)
				assert.NotNil(t, promptOpener)
				assert.NotNil(t, storage)
				assert.NotNil(t, scheduleNextTick)
				i := i.Add(1)
				uris[i-1] = _uri.String()
				return fnRunner{fn: func(extensionID, path string, cfg config.Config) error {
					assert.Equal(t, "myID", extensionID)
					assert.Equal(t, "myPath2", path)
					assert.Equal(t, config.MapConfig(map[string]any{"a": "b"}), cfg)
					return nil
				},
				}, nil
			})
		wg.Add(2)
		m := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
			&uri, cfg, runner, extensions, dir, nil, nopShutdownShaderConfig())
		defer m.Close()

		wg.Wait()
		assert.ElementsMatch(t, []string{"memory:///home", "memory:///tmp"}, uris)
	})
}

func TestWorkspaceManagerHandlerDraw(t *testing.T) {
	fn := func(t *testing.T) tui.Handler {
		m := newTestWorkspaceManagerHandler(t, defaultCfg(), nil, nopShutdownShaderConfig())
		t.Cleanup(func() { m.Close() })
		h := newSafeHandler(m)
		return h
	}

	cases := []handlertest.SequenceTestCase{
		{"",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
		{":edit",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
┌──────────────────┐
│edit▐             │
│                  │
└──────────────────┘`},
		{":edit /tmp/12345aZZ>ihello<yyp",
			`┌━━━━━━━━━━━───────┐
│o 12345aZZ*       │
├──────────────────┤
│hell▐             │
│hello             │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		{":edit /tmp/12345aZZ>ihello<yyp:workspacerelo>", // un-saved
			`┌━━━━━━━━━━────────┐
│o 12345aZZ        │
├──────────────────┤
│▐                 │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		{":edit /tmp/12345aZZ>ihello<yyp:w>:workspacerelo>", // saved
			`┌━━━━━━━━━━────────┐
│o 12345aZZ        │
├──────────────────┤
│hell▐             │
│hello             │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		{":edit memory\\:///12345aZZ>ihello<yyp:w>:workspacerelo>", // full uri
			`┌━━━━━━━━━━────────┐
│o 12345aZZ        │
├──────────────────┤
│hell▐             │
│hello             │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		{":woc>:wonew memory\\:///tmp2>:edit 12345aZZ>:w>:woc>:wonew  memory\\:///tmp2>:noticloseall>", // prompt
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Do you want to  │
│  restore the     │
│  previous        │
│                  │
│  Yes        No   │
└──────────────────┘`},
		// prompt resets cache (use file scheme to avoid needing
		// to use ':' to indicate memory scheme)
		{":woc>:wonew memory\\:///tmp2>:edit 12345aZZ>:w>:woc>:wonew  memory\\:///tmp2>y:noticloseall>",
			`┌━━━━━━━━━━────────┐
│o 12345aZZ        │
├──────────────────┤
│▐                 │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		{":woc>:wonew memory\\:///tmp2>edit 12345aZZ>:w>:woc>:wonew  memory\\:///tmp2>n:woc>:wonew  memory\\:///tmp2>:noticloseall>", // prompt no: resets cache
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
		{":wof 3>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 1  3            │
└─────━────────────┘`},
		{":woc>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1                 │
└━─────────────────┘`},
		{":q!>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
		{":woc>:woc>",
			`┌────┌─────────────┐
│    │ workspace   │
├────│ tab is      │
│    │ empty       │
│work└─────────────┘
│                  │
│                  │
├──────────────────┤
│1                 │
└━─────────────────┘`},
		{":wofo 100>",
			`┌────┌─────────────┐
│    │ invalid     │
├────│ workspace:  │
│    │ there's     │
│    │ only 9      │
│work│ workspaces  │
│    └─────────────┘
│                  │
│                  │
└──────────────────┘`},
		{":addBlaBla>", // workspacenew should work on a workspace, use next avail
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 1  2 2          │
└─────━━━──────────┘`},
		{"123456789",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 1  9            │
└─────━────────────┘`},
		{"2:wonew>", // uses tmp dir as workspace in the absence of a uri
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 1  2 2          │
└─────━━━──────────┘`},
		{"2:wofo>",
			`┌────┌─────────────┐
│    │ invalid     │
├────│ arguments.  │
│    │ Expecting   │
│work│ 1 argument  │
│    │ with        │
│    │ workspace   │
├────│ number      ┤
│1 1  2            │
└─────━────────────┘`},
		{":wofo 3>:addBlaBla>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 1  3 3          │
└─────━━━──────────┘`},
		{":wofo 4>:workspacenew memory\\:///>", // can give path as arg to workspacenew
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 1  4 4          │
└─────━━━──────────┘`},
		{":wofo 4>:workspacenew memory\\:///tmp2>:edit memory\\:///tmp2/12>:workspacerelo>", // reloads non-primary workspace
			`┌━━━━──────────────┐
│o 12              │
├──────────────────┤
│▐                 │
│                  │
│                  │
│            NORMAL│
├──────────────────┤
│1 1  4 4          │
└─────━━━──────────┘`},
		{":workspacerename bla>:wofo 4>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1 bla  4          │
└───────━──────────┘`},
	}
	handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
}

func TestWorkspaceManagerClosePromptIntegration(t *testing.T) {
	t.Run("prompts on quit if files are clean, user continues", func(t *testing.T) {
		m := newTestWorkspaceManagerHandler(t, defaultCfg(), []string{}, nopShutdownShaderConfig())

		cases := []handlertest.SequenceTestCase{
			{":quit>",
				`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Are you sure    │
│  you want to     │
│  exit?           │
│                  │
│  Yes        No   │
└──────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 20, 10, cases)

		m.mu.Lock()
		exit, handled := m.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
		assert.True(t, exit)
		assert.True(t, handled)
		m.mu.Unlock()

		require.NoError(t, m.Close())
	})

	t.Run("single prompt when running quite more than once", func(t *testing.T) {
		m := newTestWorkspaceManagerHandler(t, defaultCfg(), []string{}, nopShutdownShaderConfig())

		cases := []handlertest.SequenceTestCase{
			{":quit>",
				`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Are you sure    │
│  you want to     │
│  exit?           │
│                  │
│  Yes        No   │
└──────────────────┘`},
			{":quit>",
				`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Are you sure    │
│  you want to     │
│  exit?           │
│                  │
│  Yes        No   │
└──────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 20, 10, cases)
		exit, _ := h.Handle(term.Event{Type: term.EventKey, Ch: '3'})
		assert.True(t, exit)
		require.NoError(t, m.Close())
	})

	t.Run("open prompt, close it, open again", func(t *testing.T) {
		m := newTestWorkspaceManagerHandler(t, defaultCfg(), []string{}, nopShutdownShaderConfig())

		cases := []handlertest.SequenceTestCase{
			{":quit>",
				`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Are you sure    │
│  you want to     │
│  exit?           │
│                  │
│  Yes        No   │
└──────────────────┘`},
			{"n",
				`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
			{":quit>",
				`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Are you sure    │
│  you want to     │
│  exit?           │
│                  │
│  Yes        No   │
└──────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 20, 10, cases)
		require.NoError(t, m.Close())
	})

	t.Run("prompts on quit if files are dirty, user continues", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		m := newTestWorkspaceManagerHandlerWithDir(t, defaultCfg(), dir, nopShutdownShaderConfig())
		require.NoError(t, m.openFile("1234", true))
		require.NoError(t, m.openFile("4567", false))

		cases := []handlertest.SequenceTestCase{
			{"ihola <",
				`┌━━━━━━━───────────┐
│o 1234*  o 4567   │
├──────────────────┤
│hola▐             │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
			{":quit>",
				`┌──────────────────┐
│o 1234*  o 4567   │
├──────────────────┤
│                  │
│  There are open  │
│  files with      │
│  changes         │
│                  │
│  Yes        No   │
└──────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 20, 10, cases)

		m.mu.Lock()
		exit, handled := m.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
		assert.True(t, exit)
		assert.True(t, handled)
		m.mu.Unlock()

		require.NoError(t, m.Close())
	})

	t.Run("prompts on quit if files are dirty, user backs down", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		m := newTestWorkspaceManagerHandlerWithDir(t, defaultCfg(), dir,
			nopShutdownShaderConfig())
		require.NoError(t, m.openFile("1234", true))
		require.NoError(t, m.openFile("4567", false))

		cases := []handlertest.SequenceTestCase{
			{"ihola <",
				`┌━━━━━━━───────────┐
│o 1234*  o 4567   │
├──────────────────┤
│hola▐             │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
			{":quit>",
				`┌──────────────────┐
│o 1234*  o 4567   │
├──────────────────┤
│                  │
│  There are open  │
│  files with      │
│  changes         │
│                  │
│  Yes        No   │
└──────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 20, 10, cases)

		m.mu.Lock()

		exit, handled := m.Handle(term.Event{Type: term.EventKey, Ch: 'n'})
		assert.False(t, exit)
		assert.True(t, handled)

		exit, handled = m.Handle(term.Event{Type: term.EventNone})
		assert.False(t, exit)
		assert.True(t, handled)

		m.mu.Unlock()

		require.NoError(t, m.Close())
	})

	for _, cmd := range []string{"forcequit!", "writeforcequit!"} {
		t.Run(fmt.Sprintf("does not prompt on %s", cmd), func(t *testing.T) {
			dir, err := os.MkdirTemp("", "")
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = os.RemoveAll(dir)
			})
			m := newTestWorkspaceManagerHandlerWithDir(t, defaultCfg(), dir,
				nopShutdownShaderConfig())
			require.NoError(t, m.openFile("1234", true))
			require.NoError(t, m.openFile("4567", false))

			cases := []handlertest.SequenceTestCase{
				{"ihola <",
					`┌━━━━━━━───────────┐
│o 1234*  o 4567   │
├──────────────────┤
│hola▐             │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
			}

			h := newSafeHandler(m)
			handlertest.TestHandlerSequence(t, h, 20, 10, cases)

			m.mu.Lock()
			exit, handled := m.Handle(term.Event{
				Ch:  testCommandKey.Ch,
				Mod: testCommandKey.Mod,
				Key: testCommandKey.Key, Type: term.EventKey,
			})
			assert.False(t, exit)
			assert.True(t, handled)

			for i, ch := range cmd {
				exit, handled := m.Handle(term.Event{Type: term.EventKey, Ch: ch})
				assert.False(t, exit)
				assert.True(t, handled, i)
			}

			// sut
			exit, handled = m.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
			assert.True(t, exit)
			assert.True(t, handled)

			m.mu.Unlock()

			require.NoError(t, m.Close())
		})
	}

	t.Run(
		"exit shader runs on exit and dirty exit prompt and closes when rejecting or dismissing",
		func(t *testing.T) {
			var mockShutdownShader mockShader
			mockShutdownShaderFn := func(term.Attributes) shader.Shader {
				return &mockShutdownShader
			}
			shutdownShaderCfg := shutdownShaderConfig{
				shader:   mockShutdownShaderFn,
				fps:      30,
				duration: 1 * time.Second,
			}

			m := newTestWorkspaceManagerHandler(t, defaultCfg(), []string{},
				shutdownShaderCfg)

			// m.shaderRunner.shader.Draw will use the shutdown shader only if the quit
			// dialog has been opened, so here mockShader won't Shade.
			m.shaderRunner.shader.Draw(&term.NoopWriter{})
			assert.False(t, mockShutdownShader.called)

			cases := []handlertest.SequenceTestCase{
				{":quit>",
					`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Are you sure    │
│  you want to     │
│  exit?           │
│                  │
│  Yes        No   │
└──────────────────┘`},
			}

			// Runs the shader when invoking the prompt.
			h := newSafeHandler(m)
			handlertest.TestHandlerSequence(t, h, 20, 10, cases)

			// m.shaderRunner.shader.Draw will use the shutdown shader only if the quit
			// dialog has been opened, so here mockShader won't Shade.
			m.shaderRunner.shader.Draw(&term.NoopWriter{})
			assert.True(t, mockShutdownShader.called)

			// Reset variables
			mockShutdownShader.called = false

			m.mu.Lock()

			// Cancel shader when answering "No" to exit prompt.
			exit, handled := m.Handle(term.Event{Type: term.EventKey, Ch: 'n'})
			assert.False(t, exit)
			assert.True(t, handled)
			assert.False(t, mockShutdownShader.called)

			m.mu.Unlock()

			require.NoError(t, m.Close())
		})
}

func TestWorkspaceManagerHandlerDrawWithInitialFiles(t *testing.T) {
	// re-use storage
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	for _, wrap := range []bool{false, true} {

		t.Run(fmt.Sprintf("wrap=%v", wrap), func(t *testing.T) {

			t.Run("initial files via openFile", func(t *testing.T) {
				m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(wrap),
					dir, nopShutdownShaderConfig())
				require.NoError(t, m.openFile("1234", true))
				require.NoError(t, m.openFile("4567", false))

				cases := []handlertest.SequenceTestCase{
					{"",
						`┌━━━━━━────────────┐
│o 1234  o 4567    │
├──────────────────┤
│▐                 │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
				}
				h := newSafeHandler(m)
				handlertest.TestHandlerSequence(t, h, 20, 10, cases)

				require.NoError(t, m.Close())
			})

			t.Run("initial files from restore previous session prompt", func(t *testing.T) {
				m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(wrap),
					dir, nopShutdownShaderConfig())

				cases := []handlertest.SequenceTestCase{
					{"",
						`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│  Do you want to  │
│  restore the     │
│  previous        │
│                  │
│  Yes        No   │
└──────────────────┘`},
					{"y",
						`┌────────━━━━━━────┐
│o 1234  o 4567    │
├──────────────────┤
│▐                 │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
				}
				h := newSafeHandler(m)
				handlertest.TestHandlerSequence(t, h, 20, 10, cases)
				require.NoError(t, m.Close())
			})

			t.Run("initial files from auto restore", func(t *testing.T) {
				cfg := defaultConfigWithWrap(wrap)
				cfg.cfg["workspace"].(map[string]any)["auto_restore"] = true
				m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir, nopShutdownShaderConfig())

				cases := []handlertest.SequenceTestCase{
					{"",
						`┌────────━━━━━━────┐
│o 1234  o 4567    │
├──────────────────┤
│▐                 │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
				}
				h := newSafeHandler(m)
				handlertest.TestHandlerSequence(t, h, 20, 10, cases)
				require.NoError(t, m.Close())
			})

			// do not re-use config across runners,
			// as they're loaded async and causes a data race
			newCfg := func() ideConfig {
				cfg := defaultConfigWithWrap(wrap)
				cfg.cfg["workspace"].(map[string]any)["auto_restore"] = true
				return cfg
			}

			t.Run("position is restored on close and open again", func(t *testing.T) {
				dir, err := os.MkdirTemp("", "")
				require.NoError(t, err)
				t.Cleanup(func() {
					_ = os.RemoveAll(dir)
				})
				manager := workspace.NewManager(config.NopConfig())
				require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
					workspace.NewMemoryScheme))
				uri, err := workspaceapi.ParseURI(fmt.Sprintf("memory:///%s", dir))
				require.NoError(t, err)
				runner := FuncExtensionsRunner(testRunnerFn)

				m1 := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
					&uri, newCfg(), runner, nil, dir, nil, nopShutdownShaderConfig())

				cases := []handlertest.SequenceTestCase{
					{":edit 1234>ih3ll0\nw1rld <:write>:edit 4567>ihello\nworld <:write>:notificationcloseall>",
						`┌────────━━━━━━────┐
│o 1234  o 4567    │
├──────────────────┤
│hello             │
│world▐            │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
				}
				h := newSafeHandler(m1)
				handlertest.TestHandlerSequence(t, h, 20, 10, cases)
				require.NoError(t, m1.Close())

				m2 := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
					&uri, newCfg(), runner, nil, dir, nil, nopShutdownShaderConfig())

				cases = []handlertest.SequenceTestCase{
					{"",
						`┌────────━━━━━━────┐
│o 1234  o 4567    │
├──────────────────┤
│hello             │
│world▐            │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
					{"i\na\nb\nc\nd\ne\nf<:write>",
						`┌────────━━━━━━────┐
│o 1234  o 4567    │
├──────────────────┤
│b                 │
│c                 │
│d                 │
│e                 │
│▐                 │
│            NORMAL│
└──────────────────┘`},
				}
				h2 := newSafeHandler(m2)
				handlertest.TestHandlerSequence(t, h2, 20, 10, cases)
				require.NoError(t, m2.Close())

				m3 := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
					&uri, newCfg(), runner, nil, dir, nil, nopShutdownShaderConfig())

				cases = []handlertest.SequenceTestCase{
					{"",
						`┌────────━━━━━━────┐
│o 1234  o 4567    │
├──────────────────┤
│b                 │
│c                 │
│d                 │
│e                 │
│▐                 │
│            NORMAL│
└──────────────────┘`},
				}
				h3 := newSafeHandler(m3)
				handlertest.TestHandlerSequence(t, h3, 20, 10, cases)
				require.NoError(t, m3.Close())
			})

			t.Run("position is restored on workspacereload", func(t *testing.T) {
				dir, err := os.MkdirTemp("", "")
				require.NoError(t, err)
				t.Cleanup(func() {
					_ = os.RemoveAll(dir)
				})
				m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(wrap), dir, nopShutdownShaderConfig())

				cases := []handlertest.SequenceTestCase{
					{":edit A>ih3ll0\nw1rld <:write>:edit B>ihello\nworld <:write>:notificationcloseall>",
						`┌─────━━━──────────┐
│o A  o B          │
├──────────────────┤
│hello             │
│world▐            │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
					{":workspacerelo>",
						`┌─────━━━──────────┐
│o A  o B          │
├──────────────────┤
│hello             │
│world▐            │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
					{"i\na\nb\nc\nd\ne\nf<:write>",
						`┌─────━━━──────────┐
│o A  o B          │
├──────────────────┤
│b                 │
│c                 │
│d                 │
│e                 │
│▐                 │
│            NORMAL│
└──────────────────┘`},
					{":workspacerelo>",
						`┌─────━━━──────────┐
│o A  o B          │
├──────────────────┤
│b                 │
│c                 │
│d                 │
│e                 │
│▐                 │
│            NORMAL│
└──────────────────┘`},
				}
				h := newSafeHandler(m)
				handlertest.TestHandlerSequence(t, h, 20, 10, cases)
				require.NoError(t, m.Close())
			})
		})
	}
}

func TestWorkspaceManagerRestoresOpenTerminalSessions(t *testing.T) {
	t.Run("workspace close and reopen restores terminal tabs and windows", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		manager := workspace.NewManager(config.NopConfig())
		require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
			workspace.NewMemoryScheme))
		const terminalWorkspaceScheme = "terminaltest"
		require.NoError(t, manager.RegisterScheme(terminalWorkspaceScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (schemeapi.Scheme, error) {
				return newTerminalSessionTestScheme(ctx, cfg, uri)
			}))
		uri, err := workspaceapi.ParseURI(terminalWorkspaceScheme + ":///workspace")
		require.NoError(t, err)
		runner := FuncExtensionsRunner(testRunnerFn)
		cfg := defaultConfigWithWrap(false)
		cfg.cfg["workspace"] = map[string]any{"auto_restore": true}
		cfg.ringBell = func() {}

		m := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
			&uri, cfg, runner, nil, dir, nil, nopShutdownShaderConfig())
		m.mu.Lock()
		ex1 := m.exHandler(m.focusHandler())
		fileURI, err := ex1.workspace.URI("restored.txt")
		require.NoError(t, err)
		fileTab, err := ex1.editFileURI(fileURI, ex1.invokeWindow(), false)
		require.NoError(t, err)
		_, ok := fileTab.Handler().(text.Handler)
		require.True(t, ok)
		require.NoError(t, ex1.newTask(context.Background(), "persisted-task", "right", "--", "echo", "ok"))
		require.NoError(t, ex1.windownew(context.Background(), "right"))
		require.NoError(t, ex1.terminalnewtab(context.Background(), "tab terminal"))
		require.NoError(t, ex1.windownew(context.Background(), "down"))
		require.NoError(t, ex1.terminalnew(context.Background(), "window terminal"))
		require.Len(t, ex1.comp.Tabs(), 2)
		require.NoError(t, m.commandCloseWorkspace())
		require.Equal(t, 0, m.workspaceCount)

		require.NoError(t, m.addWorkspace(uri, true, false, -1))
		m.mu.Unlock()
		m.waitForWorkspace(t, uri)
		m.mu.Lock()
		m.Resize(80, 24)
		ex2 := m.exHandler(m.focusHandler())
		tabs := ex2.comp.Tabs()
		require.Len(t, tabs, 2)
		var terminalTabs int
		for _, tab := range tabs {
			if _, ok := tab.Handler().(vtereservoir.VTE); ok {
				terminalTabs++
			}
		}
		require.Equal(t, 1, terminalTabs)

		var terminalWindows, tabTerminalWindows, ephemeralTerminalWindows, fileWindows int
		var taskWindows, minimizedTaskWindows int
		ex2.comp.Browser().IterateWindows(func(win browser.Window) {
			content, err := win.Content()
			require.NoError(t, err)
			switch content := content.(type) {
			case vtereservoir.VTE:
				ephemeralTerminalWindows++
				terminalWindows++
			case *browser.Tab:
				if _, ok := content.Handler().(vtereservoir.VTE); ok {
					tabTerminalWindows++
					terminalWindows++
				} else if _, ok := content.Handler().(text.Handler); ok {
					fileWindows++
				}
			case *idetask.Task:
				taskWindows++
				_, minimized := win.IsMinimized()
				if minimized {
					minimizedTaskWindows++
				}
			}
		})
		require.Equal(t, 1, tabTerminalWindows)
		require.Equal(t, 1, ephemeralTerminalWindows)
		require.Equal(t, 2, terminalWindows)
		require.Equal(t, 1, fileWindows)
		require.Equal(t, 1, taskWindows)
		require.Equal(t, 1, minimizedTaskWindows)
		require.Equal(t, tcomponent.SplitOrientationVertical,
			ex2.comp.Browser().TileLayout().Split)
		require.Len(t, ex2.comp.Browser().TileLayout().Children, 2)
		require.Equal(t, tcomponent.SplitOrientationHorizontal,
			ex2.comp.Browser().TileLayout().Children[1].Split)
		require.Len(t, ex2.comp.Browser().TileLayout().Children[1].Children, 2)

		m.mu.Unlock()
		require.NoError(t, m.Close())
	})

	t.Run("workspace reload preserves deep layout with minimized task", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		manager := workspace.NewManager(config.NopConfig())
		require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
			workspace.NewMemoryScheme))
		const terminalWorkspaceScheme = "terminaltest"
		require.NoError(t, manager.RegisterScheme(terminalWorkspaceScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (schemeapi.Scheme, error) {
				return newTerminalSessionTestScheme(ctx, cfg, uri)
			}))
		uri, err := workspaceapi.ParseURI(terminalWorkspaceScheme + ":///workspace")
		require.NoError(t, err)
		runner := FuncExtensionsRunner(testRunnerFn)

		cfg := defaultConfigWithWrap(false)
		cfg.cfg["workspace"] = map[string]any{"auto_restore": true}
		cfg.ringBell = func() {}
		m := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
			&uri, cfg, runner, nil, dir, nil, nopShutdownShaderConfig())

		wantLayoutBeforeReload := `┌──────────────━━━━━━━━━━━━────────────────────────────────────────────────────┐
│o nested.txt  o middle.txt                                                    │
├┌────────────────────────┐┌────────────────────────┐┌─────────────────────────┤
││                        ││▐                       ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        │└─────────────────────────┘
││                        ││                        │┌───────────┐┌────────────┐
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                  NORMAL││           ││      NORMAL│
└└────────────────────────┘└────────────────────────┘└───────────┘└────────────┘`
		// After workspacereload, focus is no longer persisted via the
		// layout: WindowManager.RestoreTileLayout picks the first leaf
		// it finds in the new tree as the active focus. Here that is
		// the empty top-left tile (which has no editor and therefore
		// no cursor), so the visible cursor that was present in
		// wantLayoutBeforeReload disappears from the rendered frame.
		wantLayoutAfterReload := `┌──────────────────────────────────────────────────────────────────────────────┐
│o nested.txt  o middle.txt                                                    │
├┌────────────────────────┐┌────────────────────────┐┌─────────────────────────┤
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        ││                         │
││                        ││                        │└─────────────────────────┘
││                        ││                        │┌───────────┐┌────────────┐
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                        ││           ││            │
││                        ││                  NORMAL││           ││      NORMAL│
└└────────────────────────┘└────────────────────────┘└───────────┘└────────────┘`
		wantTaskFocus := `┌──────────────────────────────────────────────────────────────────────────────┐
│o nested.txt  o middle.txt                                                    │
├────────────────────────┐┌─────────────────────────┐┌─────────────────────────┤
│                        ││                         ││                         │
┌──────────────────────────────┐                    ││                         │
│ ▀        sleep 1000        0s│                    ││                         │
│                              │                    ││                         │
│                              │                    ││                         │
│                              │                    ││                         │
│                              │                    ││                         │
│                              │                    ││                         │
│                              │                    ││                         │
│                              │                    │└─────────────────────────┘
│                              │                    │┌───────────┐┌────────────┐
│                              │                    ││           ││            │
│                              │                    ││           ││            │
│                              │                    ││           ││            │
│                              │                    ││           ││            │
│                              │                    ││           ││            │
│                              │                    ││           ││            │
└──────────────────────────────┘                    ││           ││            │
│                        ││                         ││           ││            │
│                        ││                   NORMAL││           ││      NORMAL│
└────────────────────────┘└─────────────────────────┘└───────────┘└────────────┘`
		cases := []handlertest.SequenceTestCase{
			{
				InputSequence: "<c-\\\\>tasknew<space>sleeper<space>left<space>--<space>sleep<space>1000<enter>" +
					"<c-\\\\>windownew<space>right<enter>" +
					"<c-\\\\>windownew<space>right<enter>" +
					"<c-\\\\>windownew<space>down<enter>" +
					"<c-\\\\>windownew<space>right<enter>" +
					"<c-\\\\>edit<space>nested.txt<enter>" +
					"<c-\\\\>windowfocus<space>left<enter>" +
					"<c-\\\\>windowfocus<space>left<enter>" +
					"<c-\\\\>edit<space>middle.txt<enter>",
				Expected: wantLayoutBeforeReload,
			},
			{
				InputSequence: "<c-\\\\>workspacereload<enter>",
				Expected:      wantLayoutAfterReload,
			},
			{
				InputSequence: "<c-\\\\>taskfocus<space>sleeper<enter>",
				Expected:      wantTaskFocus,
			},
		}

		h := newSafeHandler(m)
		handlertest.RunHandlerSequence(t, h, 80, 24, cases)

		require.NoError(t, m.Close())
	})

	t.Run("workspace reload skips empty floating windows", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		cfg := defaultConfigWithWrap(false)
		cfg.cfg["workspace"] = map[string]any{"auto_restore": true}
		cfg.ringBell = func() {}
		m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir, nopShutdownShaderConfig())

		ex1 := m.exHandler(m.focusHandler())
		fileURI, err := ex1.workspace.URI("restored-with-floating.txt")
		require.NoError(t, err)
		_, err = ex1.editFileURI(fileURI, ex1.invokeWindow(), false)
		require.NoError(t, err)
		_, err = ex1.comp.Floating(
			browser.NopFloatingHandler(handler.StaticFloating(handler.Nop(), 10, 4)),
			browserapi.FloatingConfig{Alignment: component.AlignmentCentered},
		)
		require.NoError(t, err)
		require.Equal(t, 1, ex1.comp.Browser().FloatingWindows())

		require.NoError(t, m.commandReloadWorkspace())
		m.drainPendingWorkspaces()
		m.Resize(80, 24)
		ex2 := m.exHandler(m.focusHandler())
		require.Equal(t, 0, ex2.comp.Browser().FloatingWindows())
		tabs := ex2.comp.Tabs()
		require.Len(t, tabs, 1)
		require.Equal(t, "restored-with-floating.txt", tabs[0].URI().Name())

		require.NoError(t, m.Close())
	})

	t.Run("manager close and reopen restores terminal tab", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		manager := workspace.NewManager(config.NopConfig())
		require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
			workspace.NewMemoryScheme))
		const terminalWorkspaceScheme = "terminaltest"
		require.NoError(t, manager.RegisterScheme(terminalWorkspaceScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (schemeapi.Scheme, error) {
				return newTerminalSessionTestScheme(ctx, cfg, uri)
			}))
		uri, err := workspaceapi.ParseURI(terminalWorkspaceScheme + ":///workspace")
		require.NoError(t, err)
		runner := FuncExtensionsRunner(testRunnerFn)
		newCfg := func() ideConfig {
			cfg := defaultConfigWithWrap(false)
			cfg.cfg["workspace"] = map[string]any{"auto_restore": true}
			cfg.ringBell = func() {}
			return cfg
		}

		m1 := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
			&uri, newCfg(), runner, nil, dir, nil, nopShutdownShaderConfig())
		m1.mu.Lock()
		ex1 := m1.exHandler(m1.focusHandler())
		require.NoError(t, ex1.terminalnewtab(context.Background()))
		require.Len(t, ex1.comp.Tabs(), 1)
		m1.mu.Unlock()
		require.NoError(t, m1.Close())

		m2 := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
			&uri, newCfg(), runner, nil, dir, nil, nopShutdownShaderConfig())
		defer m2.Close()
		m2.mu.Lock()
		defer m2.mu.Unlock()
		ex2 := m2.exHandler(m2.focusHandler())
		tabs := ex2.comp.Tabs()
		require.Len(t, tabs, 1)
		_, ok := tabs[0].Handler().(vtereservoir.VTE)
		require.True(t, ok)
	})
}

type terminalSessionTestScheme struct {
	schemeapi.Scheme
	uri workspaceapi.URI
}

func newTerminalSessionTestScheme(
	ctx context.Context,
	cfg config.Config,
	uri workspaceapi.URI,
) (schemeapi.Scheme, error) {
	memURI, err := workspaceapi.ParseURI("memory://" + uri.Path())
	if err != nil {
		return nil, err
	}
	base, err := workspace.NewMemoryScheme(ctx, cfg, memURI)
	if err != nil {
		return nil, err
	}
	return &terminalSessionTestScheme{Scheme: base, uri: uri}, nil
}

func (s *terminalSessionTestScheme) URI(path string) (workspaceapi.URI, error) {
	return s.Scheme.URI(path)
}

func (s *terminalSessionTestScheme) Root() string {
	return s.Scheme.Root()
}

func (s *terminalSessionTestScheme) Chroot(path string) (schemeapi.Scheme, error) {
	base, err := s.Scheme.Chroot(path)
	if err != nil {
		return nil, err
	}
	uri, err := s.URI(path)
	if err != nil {
		return nil, err
	}
	return &terminalSessionTestScheme{Scheme: base, uri: uri}, nil
}

func (s *terminalSessionTestScheme) NewPty(context.Context) (workspaceapi.Pty, error) {
	return workspaceapi.Pty{
		Master: workspacetest.NewFile(),
		Slave:  workspacetest.NewFile(),
	}, nil
}

func (s *terminalSessionTestScheme) SetPtySize(workspaceapi.Pty, int, int) error {
	return nil
}

func (s *terminalSessionTestScheme) StartCommand(context.Context, workspaceapi.Cmd) (workspaceapi.Pid, error) {
	return 0, nil
}

func (s *terminalSessionTestScheme) Signal(workspaceapi.Pid, syscall.Signal) error {
	return nil
}

func TestInitializeNoCwd(t *testing.T) {
	m := newTestWorkspaceManagerHandlerWithDir(t,
		defaultConfigWithWrap(false), "", nopShutdownShaderConfig())

	cases := []handlertest.SequenceTestCase{
		{"",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1                 │
└━─────────────────┘`},
	}
	h := newSafeHandler(m)
	handlertest.TestHandlerSequence(t, h, 20, 10, cases)

	require.NoError(t, m.Close())
}

func TestInitializeNotifications(t *testing.T) {
	m := newTestWorkspaceManagerHandlerWithDir(t,
		defaultConfigWithWrap(false), "", nopShutdownShaderConfig())

	cases := []handlertest.SequenceTestCase{
		{"",
			`┌────┌─────────────┐
│    │ 6:14am      │
├────└─────────────┘
│                  │
│workspaceWallpaper│
│                  │
│                  │
├──────────────────┤
│1                 │
└━─────────────────┘`},
	}
	h := newSafeHandler(m)
	m.notifications.current().NotifyOnce(browserapi.LevelWarn, "6:14am")
	handlertest.TestHandlerSequence(t, h, 20, 10, cases)

	require.NoError(t, m.Close())
}

// recordingNotifications wraps a browserapi.Notifications and captures every
// (level, formatted-msg) pair seen. Used by the auto-save integration test
// to assert which notifications the autoSaver surfaces, without depending
// on UI rendering.
type recordingNotifications struct {
	mu       sync.Mutex
	inner    browserapi.Notifications
	captured []capturedNote
}

type capturedNote struct {
	level browserapi.NotificationLevel
	msg   string
}

// feedAutoSaveSequence feeds the legacy handlertest sequence syntax used by
// other integration tests in this file: ':' opens the modal prompt, '>' is
// Enter, '<' is Esc, and bare runes are typed verbatim. It bypasses the
// rendered-output assertion of TestHandlerSequence which is irrelevant
// here.
func feedAutoSaveSequence(t *testing.T, h tui.Handler, seq string) {
	t.Helper()
	for _, r := range seq {
		switch r {
		case ':':
			h.Handle(term.Event{Mod: term.ModCtrl, Ch: '\\', Type: term.EventKey})
		case ' ':
			h.Handle(term.Event{Key: term.KeySpace, Type: term.EventKey})
		case '>':
			h.Handle(term.Event{Key: term.KeyEnter, Type: term.EventKey})
		case '<':
			h.Handle(term.Event{Key: term.KeyEsc, Type: term.EventKey})
		default:
			h.Handle(term.Event{Ch: r, Type: term.EventKey})
		}
	}
}

func (r *recordingNotifications) Notify(level browserapi.NotificationLevel,
	msg string, args ...any) (string, error) {
	r.mu.Lock()
	r.captured = append(r.captured,
		capturedNote{level: level, msg: fmt.Sprintf(msg, args...)})
	r.mu.Unlock()
	return r.inner.Notify(level, msg, args...)
}

func (r *recordingNotifications) NotifyOnce(level browserapi.NotificationLevel,
	msg string, args ...any) (string, error) {
	r.mu.Lock()
	r.captured = append(r.captured,
		capturedNote{level: level, msg: fmt.Sprintf(msg, args...)})
	r.mu.Unlock()
	return r.inner.NotifyOnce(level, msg, args...)
}

func (r *recordingNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return r.inner.UpdateNotificationProgress(id, message, progress, total)
}

func (r *recordingNotifications) snapshot() []capturedNote {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]capturedNote, len(r.captured))
	copy(out, r.captured)
	return out
}

// integrationFlusher is a stub autoSaverFlusher used by
// TestAutoSaveIntegration. It records flush attempts and returns the
// error configured by the test, isolating the integration to the
// wiring (config → subscription → debounce → flush → notification)
// without exercising real disk I/O, which would race with the
// per-workspace filesystem-event dispatcher under -race.
type integrationFlusher struct {
	mu     sync.Mutex
	called bool
	err    error
}

func (f *integrationFlusher) Resource(uri workspaceapi.URI) (browserapi.Handler, bool) {
	return integrationHandler{uri: uri}, true
}

func (f *integrationFlusher) FlushTab(
	_ context.Context, _ browserapi.Handler,
) (<-chan error, error) {
	f.mu.Lock()
	f.called = true
	err := f.err
	f.mu.Unlock()
	if err != nil {
		// pre-flight error (e.g. ErrInvalidSave)
		return nil, err
	}
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch, nil
}

func (f *integrationFlusher) flushed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called
}

type integrationHandler struct{ uri workspaceapi.URI }

func (integrationHandler) Resize(_, _ int)                          {}
func (integrationHandler) Draw(_ term.Writer)                       {}
func (integrationHandler) Handle(_ term.Event) (exit, handled bool) { return false, false }
func (integrationHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, 0, false
}
func (integrationHandler) Selection() (string, bool) { return "", false }
func (integrationHandler) Close() error              { return nil }

func TestAutoSaveIntegration(t *testing.T) {
	// Shorten the auto-save delay so tests don't have to wait the
	// production 2s. The factory hook lets us wrap the workspace's
	// notifications channel with a recorder.
	prevDelay := defaultAutoSaveDelay
	defaultAutoSaveDelay = 10 * time.Millisecond
	t.Cleanup(func() { defaultAutoSaveDelay = prevDelay })

	cases := []struct {
		name       string
		flushErr   error
		wantNotify bool
		wantNotMsg string
	}{
		{
			name:       "writable file is flushed after idle delay",
			flushErr:   nil,
			wantNotify: false,
		},
		{
			name:       "read-only file surfaces a warning notification",
			flushErr:   workspaceapi.ErrFileIsNotWritable,
			wantNotify: true,
			wantNotMsg: "auto-save skipped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			filename := "doc.txt"

			cfg := defaultConfigWithWrap(false)
			editorCfg := cfg.cfg["editor"].(map[string]any)
			editorCfg["auto_save"] = true
			cfg.cfg["editor"] = editorCfg

			// Replace the autoSaver's component with a stub flusher
			// that records flush attempts and returns the test's
			// configured error. This isolates the integration to
			// the wiring (config → subscription → debounced timer
			// → flush call → notification) without exercising real
			// disk I/O, which would race with the per-workspace
			// filesystem-event dispatcher under -race.
			stub := &integrationFlusher{
				err: tc.flushErr,
			}
			var rec *recordingNotifications
			prevFactory := autoSaverFactory
			// The autoSaver assumes all map mutations happen on the
			// editor's event-loop goroutine. In production this is
			// guaranteed because scheduleNextTick re-posts the flush
			// callback as a term.EventInterrupt, which the event loop
			// serializes with edit handling. defaultCfg's
			// scheduleNextTick runs callbacks inline, so we replace
			// it with a queue and drain inside the polling loop below
			// so flushURI always runs on the test goroutine.
			schedQ := newQueueSched()
			autoSaverFactory = func(_ autoSaverFlusher,
				notif browserapi.Notifications,
				_ func(func()) bool, _ time.Duration,
			) *autoSaver {
				rec = &recordingNotifications{inner: notif}
				return newAutoSaver(stub, rec, schedQ.sched,
					defaultAutoSaveDelay)
			}
			t.Cleanup(func() { autoSaverFactory = prevFactory })

			uri, err := workspaceapi.ParseURI("memory://" + dir)
			require.NoError(t, err)
			m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir,
				nopShutdownShaderConfig())
			require.NoError(t, m.addOrCreateWorkspace(uri))
			m.drainPendingWorkspaces()
			t.Cleanup(func() { _ = m.Close() })

			require.NotNil(t, rec, "autoSaver was not constructed; "+
				"check editor.auto_save config wiring")

			h := newSafeHandler(m)
			h.Resize(30, 9)
			feedAutoSaveSequence(t, h, ":edit "+filename+">iHello<")

			require.Eventually(t, func() bool {
				schedQ.drainAll()
				if !stub.flushed() {
					return false
				}
				if tc.wantNotify {
					for _, n := range rec.snapshot() {
						if strings.Contains(n.msg, tc.wantNotMsg) {
							return true
						}
					}
					return false
				}
				return true
			}, 2*time.Second, 5*time.Millisecond)

			assert.True(t, stub.flushed(),
				"autoSaver did not invoke flush after edit")
			if tc.wantNotify {
				var found bool
				for _, n := range rec.snapshot() {
					if strings.Contains(n.msg, tc.wantNotMsg) {
						assert.Equal(t, browserapi.LevelWarn, n.level)
						found = true
						break
					}
				}
				assert.True(t, found,
					"expected notification containing %q, got %v",
					tc.wantNotMsg, rec.snapshot())
			} else {
				for _, n := range rec.snapshot() {
					assert.NotContains(t, n.msg, "auto-save",
						"unexpected auto-save notification: %v", n)
				}
			}
		})
	}
}

func TestNoBar(t *testing.T) {
	cfg := defaultConfigWithWrap(false)
	cfg.cfg["browser"] = map[string]any{"workspace_bar": false}
	m := newTestWorkspaceManagerHandlerWithDir(t,
		cfg, "", nopShutdownShaderConfig())

	cases := []handlertest.SequenceTestCase{
		{"",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
		{":workspacefocus 2>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│workspaceWallpaper│
│                  │
│                  │
│                  │
└──────────────────┘`},
	}
	h := newSafeHandler(m)
	handlertest.TestHandlerSequence(t, h, 20, 10, cases)

	require.NoError(t, m.Close())
}

func TestSwitchToWorkspaceComplete(t *testing.T) {
	m := newTestWorkspaceManagerHandlerWithDirs(t, defaultConfigWithWrap(false),
		"/tmp", "", nopShutdownShaderConfig())

	cases := []handlertest.SequenceTestCase{
		{":wofo ",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
┌──────────────────────────────────────┐
│workspacefocus ▐                      │
│1 memory:///tmp                       │
│2                                     │
│3                                     │
│4                                     │
│5                                     │
│6                                     │
│7                                     │
│8                                     │
│9                                     │
└──────────────────────────────────────┘`},
	}
	h := newSafeHandler(m)
	handlertest.TestHandlerSequence(t, h, 40, 20, cases)

	require.NoError(t, m.Close())
}

func TestMoveWorkspace(t *testing.T) {
	m := newTestWorkspaceManagerHandlerWithDirs(t, defaultConfigWithWrap(false),
		"/tmp", "", nopShutdownShaderConfig())

	cases := []handlertest.SequenceTestCase{
		{":womo ",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
┌──────────────────────────────────────┐
│workspacemove ▐                       │
│1                                     │
│2                                     │
│3                                     │
│4                                     │
│5                                     │
│6                                     │
│7                                     │
│8                                     │
│9                                     │
└──────────────────────────────────────┘`},
	}
	h := newSafeHandler(m)
	handlertest.TestHandlerSequence(t, h, 40, 20, cases)

	cases = []handlertest.SequenceTestCase{
		{"right>",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│          workspaceWallpaper          │
│                                      │
└──────────────────────────────────────┘`},
		{":wonew memory\\:///tmp2>",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│          workspaceWallpaper          │
├──────────────────────────────────────┤
│2 2  3 3                              │
└─────━━━──────────────────────────────┘`},
		{":womo 1>",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│          workspaceWallpaper          │
├──────────────────────────────────────┤
│1 1  2 2                              │
└━━━───────────────────────────────────┘`},
		{":wofo 1>:womo left>",
			`┌────────────────────────┌─────────────┐
│                        │ workspace   │
├────────────────────────│ is already  │
│          workspaceWallp│ at the      │
├────────────────────────│ first slot  ┤
│1 1  2 2                              │
└━━━───────────────────────────────────┘`},
		{":noticloseall>:womo 9>:womo right>",
			`┌────────────────────────┌─────────────┐
│                        │ workspace   │
├────────────────────────│ is already  │
│          workspaceWallp│ at the      │
├────────────────────────│ last slot   ┤
│2 2  9 9                              │
└─────━━━──────────────────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, h, 40, 7, cases)

	require.NoError(t, m.Close())
}

func TestExternalCommands(t *testing.T) {
	// FIXME: unblock CI, working on it here:
	// https://git.unstable.build/unstablebuild/go-tui/pulls/107
	if ci := os.Getenv("CI"); ci == "true" {
		t.SkipNow()
	}

	t.Run("happy path", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		dir2, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			os.RemoveAll(dir)
			os.RemoveAll(dir2)
		})

		m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
			nopShutdownShaderConfig())
		err = m.subscribeCommand(textapi.CommandManual{Name: "ramon"},
			text.FuncCommandHandler(func(context.Context, textapi.Command) error {
				return nil
			}, func(ctx context.Context, cmd textapi.Command) (
				iterator.Iterator[string], string, error,
			) {
				return iterator.FromSlice([]string{"wasup", "wasep"}), "", nil
			}))
		require.NoError(t, err)

		cases := []handlertest.SequenceTestCase{
			// '_' simulates sleeps; we can't and shouldn't
			// enable sync command prompt from here
			{":ramo w__", // existing workspace
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
┌────────────────────────────┐
│ramon w▐                    │
│wasep                       │
│wasup                       │
└────────────────────────────┘
│                            │
│                            │
└────────────────────────────┘`},
			{fmt.Sprintf(":workspacenew %s>:ramo w__", dir2), // new workspace
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
┌────────────────────────────┐
│ramon w▐                    │
│wasep                       │
│wasup                       │
└────────────────────────────┘
│                            │
│                            │
└────────────────────────────┘`},
			{":wofo 8>:ramo w__", // empty workspace
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
┌────────────────────────────┐
│ramon w▐                    │
│wasep                       │
│wasup                       │
└────────────────────────────┘
│                            │
│                            │
└────────────────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 30, 15, cases)

		require.NoError(t, m.Close())
	})

	t.Run("is goroutine safe", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		dir2, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			os.RemoveAll(dir)
			os.RemoveAll(dir2)
		})

		m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
			nopShutdownShaderConfig())

		const n = 50
		var wg sync.WaitGroup
		var errs [n]error

		wg.Add(n)
		for i := range n {
			go func(i int) {
				defer wg.Done()
				errs[i] = m.subscribeCommand(textapi.CommandManual{Name: "cmd" + strconv.Itoa(i)},
					text.FuncCommandHandler(func(context.Context, textapi.Command) error {
						return nil
					}, func(ctx context.Context, cmd textapi.Command) (
						iterator.Iterator[string], string, error,
					) {
						return iterator.FromSlice([]string{strconv.Itoa(i), strconv.Itoa(i + 1000)}), "", nil
					}))
			}(i)
		}
		wg.Wait()
		for _, err := range errs {
			require.NoError(t, err)
		}

		cases := []handlertest.SequenceTestCase{
			{":c0 1", // existing workspace
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
┌────────────────────────────┐
│cmd0 1▐                     │
│1000                        │
│                            │
└────────────────────────────┘
│                            │
│                            │
└────────────────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 30, 15, cases)

		require.NoError(t, m.Close())
	})
}

func TestExternalEvents(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		dir2, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			os.RemoveAll(dir)
			os.RemoveAll(dir2)
		})

		evsk := []textapi.EventType{
			textapi.EventTypeOpen,
			textapi.EventTypeFlush,
			textapi.EventTypeEdit,
			textapi.EventTypeClose,
		}
		var open, flush, edit, close atomic.Int64
		m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
			nopShutdownShaderConfig())
		sub := text.FuncEventHandler(func(_ context.Context, ev textapi.Event) bool {
			switch ev.Type {
			case textapi.EventTypeOpen:
				open.Add(1)
			case textapi.EventTypeFlush:
				flush.Add(1)
			case textapi.EventTypeEdit:
				edit.Add(1)
			case textapi.EventTypeClose:
				close.Add(1)
			}
			return false
		})
		m.mu.Lock()
		err = m.SubscribeEvents(evsk, sub)
		m.mu.Unlock()
		require.NoError(t, err)

		cases := []handlertest.SequenceTestCase{
			{":edit a>", // existing workspace
				`┌━━━─────────────────────────┐
│o a                         │
├────────────────────────────┤
│▐                           │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
			{"iabc<:write>", // edit + flush (async, drained by testEx.Handle wrapper)
				`┌━━━─────────────────────────┐
│o a                         │
├────────────────────────────┤
│ab▐                         │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
			{":tabclose>", // close
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`},
			{fmt.Sprintf(":workspacenew %s>:edit b>", dir2), // new workspace
				`┌━━━─────────────────────────┐
│o b                         │
├────────────────────────────┤
│▐                           │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
			{"iabc<:write>", // edit + flush
				`┌━━━─────────────────────────┐
│o b                         │
├────────────────────────────┤
│ab▐                         │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
			{":tabclose>", // close
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
			{":wofo 8>:edit c>", // empty workspace
				`┌━━━─────────────────────────┐
│o c                         │
├────────────────────────────┤
│▐                           │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2  8                 │
└──────────━─────────────────┘`},
			{"iabc<:write>", // edit + flush
				`┌━━━─────────────────────────┐
│o c                         │
├────────────────────────────┤
│ab▐                         │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2  8                 │
└──────────━─────────────────┘`},
			{":tabclose>", // close
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  8                 │
└──────────━─────────────────┘`},
		}
		h := newSafeHandler(m)
		handlertest.TestHandlerSequence(t, h, 30, 15, cases)

		assert.Equal(t, 3, int(open.Load()))
		assert.Equal(t, 3, int(flush.Load()))
		assert.Equal(t, 9, int(edit.Load()))
		assert.Equal(t, 3, int(close.Load()))

		m.mu.Lock()
		ok, err := m.UnsubscribeEvents(sub)
		m.mu.Unlock()
		require.NoError(t, err)
		require.True(t, ok)

		cases = []handlertest.SequenceTestCase{
			{":wofo 2>:woc>:wofo 1>",
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`},
			{":edit a>",
				`┌━━━─────────────────────────┐
│o a                         │
├────────────────────────────┤
│▐bc                         │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
			{"iabc<:write>",
				`┌━━━─────────────────────────┐
│o a                         │
├────────────────────────────┤
│ab▐abc                      │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
			{":tabclose>",
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`},
			{fmt.Sprintf(":workspacenew %s>:edit b>", dir2), // new workspace
				`┌━━━─────────────────────────┐
│o b                         │
├────────────────────────────┤
│▐bc                         │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
			{"iabc<:write>",
				`┌━━━─────────────────────────┐
│o b                         │
├────────────────────────────┤
│ab▐abc                      │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
			{":tabclose>",
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
			{":wofo 8>:edit c>",
				`┌━━━─────────────────────────┐
│o c                         │
├────────────────────────────┤
│▐bc                         │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2  8                 │
└──────────━─────────────────┘`},
			{"iabc<:write>",
				`┌━━━─────────────────────────┐
│o c                         │
├────────────────────────────┤
│ab▐abc                      │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1 1  2 2  8                 │
└──────────━─────────────────┘`},
			{":tabclose>",
				`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  8                 │
└──────────━─────────────────┘`},
		}

		handlertest.TestHandlerSequence(t, h, 30, 15, cases)
		assert.Equal(t, 3, int(open.Load()))
		assert.Equal(t, 3, int(flush.Load()))
		assert.Equal(t, 9, int(edit.Load()))
		assert.Equal(t, 3, int(close.Load()))

		require.NoError(t, m.Close())
	})
}

func TestWorkspaceCommands(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	dir2, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	dir3, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
		os.RemoveAll(dir2)
		os.RemoveAll(dir3)
	})

	uri1, err := workspaceapi.ParseURI("memory://" + dir)
	require.NoError(t, err)
	uri2, err := workspaceapi.ParseURI("file://" + dir2)
	require.NoError(t, err)
	uri3, err := workspaceapi.ParseURI("file://" + dir3)
	require.NoError(t, err)

	abcCmd := textapi.CommandManual{Name: "tttt"}
	xyzCmd := textapi.CommandManual{Name: "xyz"}

	var abc, xyz atomic.Int64
	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), dir,
		nopShutdownShaderConfig())
	sub := text.FuncCommandHandler(func(_ context.Context, cmd textapi.Command) error {
		switch cmd.Name {
		case "tttt":
			abc.Add(1)
		case "xyz":
			xyz.Add(1)
		default:
			return errors.New("not cool, man")
		}
		return nil
	}, func(ctx context.Context, cmd textapi.Command) (
		iterator.Iterator[string], string, error,
	) {
		return iterator.Empty[string](), "", nil
	})

	m.mu.Lock()
	err = m.SubscribeCommandForWorkspace(uri1, xyzCmd, sub)
	require.NoError(t, err)
	err = m.SubscribeCommandForWorkspace(uri1, abcCmd, sub)
	require.NoError(t, err)

	require.NoError(t, m.addOrCreateWorkspace(uri2))
	require.NoError(t, m.addOrCreateWorkspace(uri3))
	// Release mu so the install goroutines (Phase B) can lock it
	// when they reach the rollback path / WaitGroup, then drain
	// pending workspaces, then re-acquire mu.
	m.mu.Unlock()
	m.drainPendingWorkspaces()
	m.mu.Lock()

	err = m.SubscribeCommandForWorkspace(uri2, xyzCmd, sub)
	require.NoError(t, err)
	err = m.SubscribeCommandForWorkspace(uri2, abcCmd, sub)
	require.NoError(t, err)
	m.mu.Unlock()

	cases := []handlertest.SequenceTestCase{
		{":workspacefocus 1>:xyz>:tttt>",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  3 3               │
└━━━─────────────────────────┘`},
		{":workspacefocus 2>:tttt>:xyz>",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  3 3               │
└─────━━━────────────────────┘`},
		{":workspacefocus 3>:tttt>:xyz>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias "xyz" │
│              └─────────────┘
│              ┌─────────────┐
│     workspace│ unknown     │
│              │ command or  │
│              │ command     │
│              │ alias       │
│              │ "tttt"      │
├──────────────└─────────────┤
│1 1  2 2  3 3               │
└──────────━━━───────────────┘`},
	}
	h := newSafeHandler(m)
	handlertest.TestHandlerSequence(t, h, 30, 15, cases)

	assert.Equal(t, 2, int(xyz.Load()))
	assert.Equal(t, 2, int(abc.Load()))

	err = m.UnsubscribeCommandForWorkspace(uri1, "tttt")
	require.NoError(t, err)

	err = m.UnsubscribeCommandForWorkspace(uri2, "xyz")
	require.NoError(t, err)

	cases = []handlertest.SequenceTestCase{
		{":noticloseall>:workspacefocus 1>:xyz>:tttt>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias       │
│              │ "tttt"      │
│              └─────────────┘
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  3 3               │
└━━━─────────────────────────┘`},
		{":noticloseall>:workspacefocus 2>:tttt>:xyz>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias "xyz" │
│              └─────────────┘
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  3 3               │
└─────━━━────────────────────┘`},
		{":noticloseall>:workspacefocus 3>:tttt>:xyz>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias "xyz" │
│              └─────────────┘
│              ┌─────────────┐
│     workspace│ unknown     │
│              │ command or  │
│              │ command     │
│              │ alias       │
│              │ "tttt"      │
├──────────────└─────────────┤
│1 1  2 2  3 3               │
└──────────━━━───────────────┘`},
	}

	handlertest.TestHandlerSequence(t, h, 30, 15, cases)
	assert.Equal(t, 3, int(xyz.Load()))
	assert.Equal(t, 3, int(abc.Load()))

	err = m.UnsubscribeCommandForWorkspace(uri2, "tttt")
	require.NoError(t, err)

	err = m.UnsubscribeCommandForWorkspace(uri1, "xyz")
	require.NoError(t, err)

	cases = []handlertest.SequenceTestCase{
		{":noticloseall>:workspacefocus 1>:xyz>:tttt>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias       │
│              │ "tttt"      │
│              └─────────────┘
│     workspace┌─────────────┐
│              │ unknown     │
│              │ command or  │
│              │ command     │
│              │ alias "xyz" │
├──────────────└─────────────┤
│1 1  2 2  3 3               │
└━━━─────────────────────────┘`},
		{":noticloseall>:workspacefocus 2>:tttt>:xyz>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias "xyz" │
│              └─────────────┘
│              ┌─────────────┐
│     workspace│ unknown     │
│              │ command or  │
│              │ command     │
│              │ alias       │
│              │ "tttt"      │
├──────────────└─────────────┤
│1 1  2 2  3 3               │
└─────━━━────────────────────┘`},
		{":noticloseall>:workspacefocus 3>:tttt>:xyz>",
			`┌──────────────┌─────────────┐
│              │ unknown     │
├──────────────│ command or  │
│              │ command     │
│              │ alias "xyz" │
│              └─────────────┘
│              ┌─────────────┐
│     workspace│ unknown     │
│              │ command or  │
│              │ command     │
│              │ alias       │
│              │ "tttt"      │
├──────────────└─────────────┤
│1 1  2 2  3 3               │
└──────────━━━───────────────┘`},
	}

	handlertest.TestHandlerSequence(t, h, 30, 15, cases)
	assert.Equal(t, 3, int(xyz.Load()))
	assert.Equal(t, 3, int(abc.Load()))

	require.NoError(t, m.Close())
}

func TestComponentOnTabsClickIntegration(t *testing.T) {
	// setup
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	manager := workspace.NewManager(config.NopConfig())
	require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
		workspace.NewMemoryScheme))

	var uri *workspaceapi.URI
	if dir != "" {
		var err error
		uri = new(workspaceapi.URI)
		*uri, err = workspaceapi.ParseURI(fmt.Sprintf("memory://%s", dir))
		require.NoError(t, err)
	}
	runner := FuncExtensionsRunner(testRunnerFn)
	cfg := defaultCfg()
	var called int
	m := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
		uri, cfg, runner, nil, dir,
		func(i int) bool {
			called++
			return true
		}, nopShutdownShaderConfig())
	m.Resize(20, 8)

	// sut
	require.Equal(t, 0, called)

	m.mu.Lock()

	_, handled := m.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
	assert.True(t, handled)
	assert.Equal(t, 1, called)

	_, handled = m.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseY: 7})
	assert.False(t, handled)
	assert.Equal(t, 1, called)

	m.mu.Unlock()

	require.NoError(t, m.Close())
}

// TestWorkspaceBarTabClickIntegration is a black-box regression suite
// for the bottom workspace bar's mouse routing. Each case builds a
// (possibly sparse) workspace layout via the manager's internal
// helpers, dispatches a single term.EventMouse/MouseLeft event at
// chosen bar coordinates, and asserts the resulting screen with
// handlertest.RunHandlerSequence. Verification is purely from the
// rendered Draw output, never via private fields.
//
// The bar config sets focus_tab_attr to {bg: blue} so the focused tab's
// name cells render as the writer's BackgroundCh ('·'). That makes
// which slot gained focus directly observable in the expected string.
//
// Bar layout in numbers mode renders one bar tab per visible workspace
// as "<icon> <name>" with a two-space separator. With single-digit
// names each tab spans 5 columns (icon + space + name + separator).
func TestWorkspaceBarTabClickIntegration(t *testing.T) {
	const (
		width  = 30
		height = 9
		// barY is the on-screen Y of the workspace bar's tab row. The
		// bar sits one row above the bottom border.
		barY = height - 2
	)

	// screen builds the expected Draw output. The bar string encodes
	// the focused slot via '·' on the focused name (see writer setup).
	// hlStart/hlLen describe the focus-frame highlight rendered on the
	// bottom border row over the focused tab's cell columns.
	screen := func(bar string, hlStart, hlLen int) string {
		// Build the bottom border: '└' + 28 box chars + '┘'.
		bottom := []rune("└────────────────────────────┘")
		for i := 0; i < hlLen; i++ {
			bottom[1+hlStart+i] = '━'
		}
		return `┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
` + bar + `
` + string(bottom)
	}

	type clickCase struct {
		name string
		// filled[i] indicates whether slot i should be populated.
		filled []bool
		// focusSlot is the slot to focus before the click.
		focusSlot int
		// mouseX is the X column of the click on the bar row.
		mouseX int
		// expected is the rendered screen after the click.
		expected string
	}

	cases := []clickCase{
		// Dense layout (3 filled): "1 1  2 2  3 3". Each filled tab
		// is 5 cells wide (icon + space + name + 2-space sep), so
		// tab 0 spans X∈[0,5], tab 1 spans X∈[6,10], tab 2 spans
		// X∈[11,15].
		{
			name:      "dense_click_first_tab_focuses_slot_1",
			filled:    []bool{true, true, true},
			focusSlot: 2, mouseX: 1,
			expected: screen("│1 ·  2 2  3 3               │", 0, 3),
		},
		{
			name:      "dense_click_middle_tab_focuses_slot_2",
			filled:    []bool{true, true, true},
			focusSlot: 0, mouseX: 7,
			expected: screen("│1 1  2 ·  3 3               │", 5, 3),
		},
		{
			name:      "dense_click_last_tab_focuses_slot_3",
			filled:    []bool{true, true, true},
			focusSlot: 0, mouseX: 12,
			expected: screen("│1 1  2 2  3 ·               │", 10, 3),
		},
		{
			name:      "dense_click_far_past_last_tab_keeps_focus",
			filled:    []bool{true, true, true},
			focusSlot: 0, mouseX: width - 2,
			expected: screen("│1 ·  2 2  3 3               │", 0, 3),
		},

		// Single empty middle slot — the original RUNE-126 bug. Bar
		// renders "1 1  3 3"; widths 5/5 → tab 0 X∈[0,5], tab 1
		// X∈[6,10]. Clicking inside tab 1 must focus slot 3, not 2.
		{
			name:      "middle_gap_click_first_visible_tab_focuses_slot_1",
			filled:    []bool{true, false, true},
			focusSlot: 2, mouseX: 1,
			expected: screen("│1 ·  3 3                    │", 0, 3),
		},
		{
			name:      "middle_gap_click_second_visible_tab_focuses_slot_3",
			filled:    []bool{true, false, true},
			focusSlot: 0, mouseX: 7,
			expected: screen("│1 1  3 ·                    │", 5, 3),
		},
		{
			name:      "middle_gap_click_past_last_visible_tab_keeps_focus",
			filled:    []bool{true, false, true},
			focusSlot: 0, mouseX: 15,
			expected: screen("│1 ·  3 3                    │", 0, 3),
		},

		// Two consecutive empty middle slots: bar renders "1 1  4 4".
		{
			name:      "double_gap_click_second_visible_tab_focuses_slot_4",
			filled:    []bool{true, false, false, true},
			focusSlot: 0, mouseX: 7,
			expected: screen("│1 1  4 ·                    │", 5, 3),
		},

		// Focused empty middle slot: bar renders "1 1  2    3 3".
		// The focused-but-empty entry has no icon, so its width is
		// only 3 cells (sep + name) → widths 5/3/5 → tab spans
		// X∈[0,5], X∈[6,8], X∈[9,13].
		// Clicking the left or right filled tab moves focus off the
		// empty slot, whiy slot, which then disappears from the bar — the bar
		// collapses back to the dense two-tab layout.
		{
			name:      "focused_empty_middle_click_left_filled_focuses_slot_1",
			filled:    []bool{true, false, true},
			focusSlot: 1, mouseX: 1,
			expected: screen("│1 ·  3 3                    │", 0, 3),
		},
		{
			name:      "focused_empty_middle_click_right_filled_focuses_slot_3",
			filled:    []bool{true, false, true},
			focusSlot: 1, mouseX: 10,
			expected: screen("│1 1  3 ·                    │", 5, 3),
		},

		// Focused empty first slot: bar renders "1  2 2  3 3" with
		// widths 3/5/5 → tab 0 X∈[0,3], tab 1 X∈[4,8], tab 2 X∈[9,13].
		// Clicking tab 1 or tab 2 unfocuses slot 0, so its empty
		// entry vanishes and the bar collapses left.
		{
			name:      "focused_empty_first_click_second_visible_focuses_slot_2",
			filled:    []bool{false, true, true},
			focusSlot: 0, mouseX: 5,
			expected: screen("│2 ·  3 3                    │", 0, 3),
		},
		{
			name:      "focused_empty_first_click_third_visible_focuses_slot_3",
			filled:    []bool{false, true, true},
			focusSlot: 0, mouseX: 10,
			expected: screen("│2 2  3 ·                    │", 5, 3),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfigWithWrap(false)
			// Configure attrs so the focused tab's name renders as
			// '·' (the writer's BackgroundCh) and the unfocused
			// tabs render as their literal name characters.
			cfg.cfg["browser"] = map[string]any{
				"workspace_bar":      "number",
				"focus_tab_attr":     map[string]any{"bg": "blue"},
				"non_focus_tab_attr": map[string]any{"bg": "default"},
			}
			m := newTestWorkspaceManagerHandlerWithDir(t, cfg, "",
				nopShutdownShaderConfig())
			t.Cleanup(func() { require.NoError(t, m.Close()) })

			// addWorkspace is async; the default scheduleNextTick
			// stub serialises Phase C under m.mu on a fresh
			// goroutine. Drive each install fully (Phase B+C)
			// before invoking the next internal helper so the
			// subsequent switch/close calls observe a consistent
			// h.workspaces snapshot.
			for range tc.filled {
				uri, err := workspaceapi.ParseURI("file://" + t.TempDir())
				require.NoError(t, err)
				require.NoError(t, m.addOrCreateWorkspace(uri))
				m.drainPendingWorkspaces()
			}
			m.mu.Lock()
			for i, f := range tc.filled {
				if f {
					continue
				}
				require.True(t, m.switchToWorkspace(i))
				_, _, err := m.closeWorkspace()
				require.NoError(t, err)
			}
			require.True(t, m.switchToWorkspace(tc.focusSlot))
			m.mu.Unlock()

			// Black-box verification: route the mouse click through
			// the full handler chain and assert the rendered screen
			// via RunHandlerSequence with no key input.
			h := newSafeHandler(m)
			writer := term.NewStringWriter(width, height)
			writer.BackgroundCh = '·'
			h.Resize(width, height)
			h.Handle(term.Event{
				Type: term.EventMouse, Key: term.MouseLeft,
				MouseX: tc.mouseX, MouseY: barY,
			})
			handlertest.RunHandlerSequenceWriter(t, writer, h,
				width, height, []handlertest.SequenceTestCase{
					{InputSequence: "", Expected: tc.expected},
				})
		})
	}
}

func TestWorkspaceManagerCreateWorkspace(t *testing.T) {
	m := newTestWorkspaceManagerHandler(t, defaultCfg(), nil, nopShutdownShaderConfig())
	t.Cleanup(func() { m.Close() })

	// create new temp dir, with consistent name, so test below works
	const (
		tempDir  = "/tmp/TestWorkspaceManagerCreateWorkspace"
		tempDir2 = "/tmp/TestWorkspaceManagerCreateWorkspace2"
	)
	for _, tempDir := range []string{tempDir, tempDir2} {
		err := os.MkdirAll(tempDir, 0600)
		require.NoError(t, err)
		err = os.RemoveAll(tempDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			os.RemoveAll(tempDir)
		})
	}

	cases := []handlertest.SequenceTestCase{
		{fmt.Sprintf(":workspacenew file\\://%s>", tempDir),
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
┌────────────────────────────┐
│                            │
│  workspace with URI        │
│  file:///tmp/TestWorkspac  │
│  eManagerCreateWorkspace   │
│  does not exist. Do you    │
│  want to create it?        │
│                            │
│                            │
│      Yes          No       │
└────────────────────────────┘
│                            │
│                            │
│                            │
└────────────────────────────┘`},
		{"y>",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
		{fmt.Sprintf(":workspacenew %s>", tempDir2), // not fully specified
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
┌────────────────────────────┐
│                            │
│  workspace with URI        │
│  file:///tmp/TestWorkspac  │
│  eManagerCreateWorkspace2  │
│  does not exist. Do you    │
│  want to create it?        │
│                            │
│                            │
│      Yes          No       │
└────────────────────────────┘
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2                    │
└─────━━━────────────────────┘`},
		{"y>",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│     workspaceWallpaper     │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2  3 3               │
└──────────━━━───────────────┘`},
	}

	h := newSafeHandler(m)
	handlertest.TestHandlerSequence(t, h, 30, 20, cases)

	// test that they indeed exist
	for _, tempDir := range []string{tempDir, tempDir2} {
		fs, err := os.Stat(tempDir)
		require.NoError(t, err)
		require.True(t, fs.IsDir())
	}
}

// TestWorkspaceManagerCreateWorkspaceQuotedPath guards the fix for RUNE-120:
// the modal command prompt must respect bash-style quoting/escaping so that
// directory paths containing spaces survive `:` dispatch unchanged. Each case
// drives a separate fresh manager so the variants can be asserted independently.
func TestWorkspaceManagerCreateWorkspaceQuotedPath(t *testing.T) {
	parent := t.TempDir()
	// The escape variant exercises raw backslash space escapes which only
	// guard whitespace; shell metacharacters such as parens still need to be
	// quoted. Keep the directory name space-only so the test focuses on the
	// space-escaping behaviour without dragging in unrelated metacharacter
	// quoting concerns.
	dirEscape := filepath.Join(parent, "Unstable Build escape")
	dirSingle := filepath.Join(parent, "Unstable Build (single)")
	dirDouble := filepath.Join(parent, "Unstable Build (double)")
	for _, d := range []string{dirEscape, dirSingle, dirDouble} {
		require.NoError(t, os.MkdirAll(d, 0700))
	}

	cases := []struct {
		name string
		// raw is the buffer the prompt should receive after the
		// leading "workspacenew ". feedLiteral emits each rune as a
		// literal key event; spaces are sent as KeySpace events to
		// match the runtime keypress path.
		raw  string
		path string
	}{
		{
			name: "backslash-escaped spaces",
			raw:  strings.ReplaceAll(dirEscape, " ", `\ `),
			path: dirEscape,
		},
		{
			name: "single-quoted path",
			raw:  fmt.Sprintf("'%s'", dirSingle),
			path: dirSingle,
		},
		{
			name: "double-quoted path",
			raw:  fmt.Sprintf(`"%s"`, dirDouble),
			path: dirDouble,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestWorkspaceManagerHandler(t, defaultCfg(), nil,
				nopShutdownShaderConfig())
			t.Cleanup(func() { m.Close() })
			m.forceSyncCommandPrompt = true
			m.Resize(30, 20)

			h := newSafeHandler(m)
			// open the modal command prompt (Ctrl+\\, see defaultCfg).
			h.Handle(term.Event{Type: term.EventKey,
				Mod: term.ModCtrl, Ch: '\\'})
			feedLiteral(t, h, "workspacenew ")
			feedLiteral(t, h, tc.raw)
			h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})

			require.Equal(t, 2, m.workspaceCount,
				"expected the new workspace to be registered alongside the default one")

			uri, err := workspaceapi.CurrentUserHostURI(tc.path)
			require.NoError(t, err)
			var found bool
			for _, w := range m.workspaces {
				if w == nil {
					continue
				}
				if w.uri == uri {
					found = true
					break
				}
			}
			require.True(t, found,
				"expected to find workspace registered at %s", uri)
		})
	}
}

// feedLiteral writes each rune in s to h as a regular key event. Space is
// translated to KeySpace to match the runtime keypress path.
func feedLiteral(t *testing.T, h tui.Handler, s string) {
	t.Helper()
	for _, r := range s {
		switch r {
		case ' ':
			h.Handle(term.Event{Type: term.EventKey, Key: term.KeySpace})
		default:
			h.Handle(term.Event{Type: term.EventKey, Ch: r})
		}
	}
}

func newTestWorkspaceManagerHandlerWithManager(
	t *testing.T, manager *workspace.Manager,
	uri workspaceapi.URI, cfg ideConfig,
) *testWorkspaceManagerHandler {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
		&uri, cfg, FuncExtensionsRunner(testRunnerFn), nil, dir, nil,
		nopShutdownShaderConfig())
}

func newTestWorkspaceManagerHandlerWithManagerAndExtensions(
	t *testing.T, manager *workspace.Manager,
	uri *workspaceapi.URI, cfg ideConfig, runner ExtensionsRunner,
	extensions map[string]Extension, dir string,
	onTabsClick func(int) bool,
	shutdownShaderCfg shutdownShaderConfig,
) *testWorkspaceManagerHandler {
	homeURI, err := workspaceapi.ParseURI("memory:///home")
	require.NoError(t, err)

	m := new(testWorkspaceManagerHandler)
	m.workspaceManagerHandler = new(workspaceManagerHandler)
	// ensure that command manual is never shown
	cfg.cfg["command"] = defaultCfg().cfg["command"]

	shRunner := new(shaderRunner)
	shRunner.init(handler.Nop(), term.NopInterrupter(), term.Attributes{},
		shutdownShaderCfg, loadingShaderConfig{}, openShaderConfig{},
		component.FrameCharSetDefault())

	mu := new(sync.Mutex)
	// schedTracker counts scheduled-but-not-yet-run callbacks
	// emitted by the default cfg.scheduleNextTick stub below.
	// Tests drain by waiting on Cond until the count hits zero.
	// Using a WaitGroup here trips the race detector because
	// schedWG.Add can race with schedWG.Wait.
	var schedMu sync.Mutex
	schedCond := sync.NewCond(&schedMu)
	var schedCount int
	// trackSched is true when we install the default stub below;
	// only then should m.schedWG point to the tracked WaitGroup.
	trackSched := false
	if cfg.scheduleNextTick == nil {
		// Default scheduleNextTick stub mirrors the host event
		// loop's UserFunc dispatch (gui.Update at
		// term/gui/gui.go:248): fn runs on a fresh goroutine
		// while holding mu, the same locker init receives.
		// We also track every scheduled callback via schedCond so
		// tests can drain queued callbacks before asserting on
		// Draw output (see testWorkspaceManagerHandler.drainSched).
		cfg.scheduleNextTick = func(fn func()) bool {
			schedMu.Lock()
			schedCount++
			schedMu.Unlock()
			go func() {
				defer func() {
					schedMu.Lock()
					schedCount--
					if schedCount == 0 {
						schedCond.Broadcast()
					}
					schedMu.Unlock()
				}()
				mu.Lock()
				defer mu.Unlock()
				fn()
			}()
			return true
		}
		trackSched = true
	}

	notiConfig := notificationsConfig()
	releaseManager := docrelease.NewManager(document.NewInMemoryService())
	storage := localstorage.New(context.Background(), dir, doctoml.Marshaler())
	err = m.workspaceManagerHandler.init(uri, homeURI, manager,
		notiConfig, cfg, storage, dir, func(term.Event) bool {
			return true
		}, runner, mu, extensions,
		func() (ideConfig, error) { return cfg, nil },
		".sixrc", 0, 0, '1', 0, 0, true, onTabsClick, releaseManager, shRunner, 0, nil)

	require.NoError(t, err)
	if uri != nil {
		// addWorkspace is now async: init kicks off Phase B in a
		// goroutine and Phase C lands the install via
		// scheduleNextTick. Tests built on top of this helper expect
		// the boot workspace to be installed by the time they start
		// interacting with the handler, so block here until that has
		// happened (or fail with a clear deadline).
		m.waitForWorkspace(t, *uri)
	}
	if trackSched {
		m.schedDrain = func() {
			schedMu.Lock()
			for schedCount > 0 {
				schedCond.Wait()
			}
			schedMu.Unlock()
		}
	}
	return m
}

// waitForWorkspace blocks until uri has been installed into
// h.workspaces (i.e. Phase C ran and the pending entry was drained).
// It is the test counterpart to the async addWorkspace contract:
// production code returns to the event loop immediately while Phase B
// runs, so tests that immediately read m.focusHandler() / m.workspaces
// must wait first.
func (m *testWorkspaceManagerHandler) waitForWorkspace(
	t *testing.T, uri workspaceapi.URI,
) {
	t.Helper()
	const timeout = 10 * time.Second
	const interval = 5 * time.Millisecond
	deadline := time.Now().Add(timeout)
	for {
		m.mu.Lock()
		_, installed := m.findInstalledSlot(uri)
		_, pending := m.pending[uri.String()]
		m.mu.Unlock()
		if installed {
			return
		}
		if !pending && time.Now().After(deadline) {
			t.Fatalf("waitForWorkspace: %s never installed and "+
				"no pending entry within %s", uri.String(), timeout)
		}
		if time.Now().After(deadline) {
			t.Fatalf("waitForWorkspace: %s still pending after %s",
				uri.String(), timeout)
		}
		time.Sleep(interval)
	}
}

func newTestWorkspaceManagerHandlerWithDir(
	t *testing.T, cc ideConfig, dir string,
	shutdownShaderCfg shutdownShaderConfig,
) *testWorkspaceManagerHandler {
	manager := workspace.NewManager(config.NopConfig())
	require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
		workspace.NewMemoryScheme))
	require.NoError(t, manager.RegisterScheme(workspace.FileScheme,
		workspace.NewFileScheme))

	var uri *workspaceapi.URI
	if dir != "" {
		var err error
		uri = new(workspaceapi.URI)
		*uri, err = workspaceapi.ParseURI(fmt.Sprintf("memory://%s", dir))
		require.NoError(t, err)
	}
	dataDir := dir
	if dataDir == "" {
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			os.RemoveAll(dir)
		})
		dataDir = dir
	}
	runner := FuncExtensionsRunner(testRunnerFn)
	return newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
		uri, cc, runner, nil, dataDir, nil, shutdownShaderCfg)
}

// newTestWorkspaceManagerHandlerWithDirs is like newTestWorkspaceManagerHandlerWithDir
// but allows the workspace dir and the pkgmanager dataDir to be specified
// independently. This is useful for tests that need the workspace URI to be a
// stable path (e.g. "/tmp") but must not share the system temp dir as the
// pkgmanager data dir, to avoid interfering with other tests and stale
// package-manager state.
func newTestWorkspaceManagerHandlerWithDirs(
	t *testing.T, cc ideConfig, dir, dataDir string,
	shutdownShaderCfg shutdownShaderConfig,
) *testWorkspaceManagerHandler {
	manager := workspace.NewManager(config.NopConfig())
	require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
		workspace.NewMemoryScheme))
	require.NoError(t, manager.RegisterScheme(workspace.FileScheme,
		workspace.NewFileScheme))

	var uri *workspaceapi.URI
	if dir != "" {
		var err error
		uri = new(workspaceapi.URI)
		*uri, err = workspaceapi.ParseURI(fmt.Sprintf("memory://%s", dir))
		require.NoError(t, err)
	}
	if dataDir == "" {
		d, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(d) })
		dataDir = d
	}
	runner := FuncExtensionsRunner(testRunnerFn)
	return newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
		uri, cc, runner, nil, dataDir, nil, shutdownShaderCfg)
}

func newTestWorkspaceManagerHandler(
	t *testing.T, cc ideConfig, filenames []string,
	shutdownShaderCfg shutdownShaderConfig,
) *testWorkspaceManagerHandler {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return newTestWorkspaceManagerHandlerWithDir(t, cc, dir, shutdownShaderCfg)
}

// deterministic usage of search list
type testWorkspaceManagerHandler struct {
	*workspaceManagerHandler
	forceSyncCommandPrompt bool
	// schedDrain blocks until every callback dispatched through the
	// default test scheduleNextTick stub has finished. Tests that
	// assert post-:write or post-:reload state should call drainSched
	// (via safeHandler or directly) to ensure scheduled completion
	// callbacks have run.
	schedDrain func()
}

func (t *testWorkspaceManagerHandler) drainSched() {
	if t == nil || t.schedDrain == nil {
		return
	}
	t.schedDrain()
}

func (t *testWorkspaceManagerHandler) enableSyncCommandPrompt() {
	if t == nil || t.workspaceManagerHandler == nil {
		return
	}
	if t.empty != nil {
		t.empty.syncCommandPrompt = true
	}
	for _, w := range t.workspaces {
		if w != nil && w.ex != nil {
			w.ex.syncCommandPrompt = true
		}
	}
	if h := t.focusHandler(); h != nil {
		if ex, ok := h.(*ex); ok {
			ex.syncCommandPrompt = true
		}
		if wh, ok := h.(*workspaceHandler); ok && wh.ex != nil {
			wh.ex.syncCommandPrompt = true
		}
	}
}

// mimic ide.IDE
func (t *testWorkspaceManagerHandler) Close() error {
	t.workspaceManagerHandler.mu.Lock()
	defer t.workspaceManagerHandler.mu.Unlock()

	return t.workspaceManagerHandler.Close()
}

func (t *testWorkspaceManagerHandler) Handle(ev term.Event) (bool, bool) {
	if t.forceSyncCommandPrompt {
		t.enableSyncCommandPrompt()
	}
	quit, handle := t.workspaceManagerHandler.Handle(ev)
	if t.forceSyncCommandPrompt {
		t.enableSyncCommandPrompt()
	}
	handler := t.workspaceManagerHandler.focusHandler()
	ex, ok := handler.(*ex)
	if !ok {
		ex = handler.(*workspaceHandler).ex
	}
	ex.Wait()
	return quit, handle
}

// drainPendingWorkspaces blocks until h.pending is empty. The caller
// must NOT hold h.mu; we acquire it briefly each iteration to read
// the pending map and release it so the install goroutine queued by
// scheduleNextTick can run.
func (t *testWorkspaceManagerHandler) drainPendingWorkspaces() {
	t.workspaceManagerHandler.pendingWG.Wait()
}

func defaultCfg() ideConfig {
	return ideConfig{cfg: map[string]any{
		"clipboard": "memory",
		"command": map[string]any{
			"show_manual_after":  "1h",
			"show_progress_hint": false,
			"key":                "<c-\\\\>", // see handlertest.TestHandlerIsolated
			"key_bindings": map[string]any{
				"1": "workspacefocus 1",
				"2": "workspacefocus 2",
				"3": "workspacefocus 3",
				"4": "workspacefocus 4",
				"5": "workspacefocus 5",
				"6": "workspacefocus 6",
				"7": "workspacefocus 7",
				"8": "workspacefocus 8",
				"9": "workspacefocus 9",
				"0": "workspacefocus 10",
			},
			"aliases": map[string]any{
				"addBlaBla": "workspacenew memory:///blabla",
				"w":         "write!",
			},
		},
		"workspace": map[string]any{
			"wallpaper":    "workspaceWallpaper",
			"auto_restore": false,
		},
		"browser": map[string]any{
			"workspace_bar": "number",
			"window_manager": map[string]any{
				"no_max_size": false,
			},
		},
		"notifications": map[string]any{
			"progress_bar": false,
		},
	},
		configPath: "not-empty",
	}
}

func defaultConfigWithWrap(wrap bool) ideConfig {
	ret := defaultCfg()
	ret.cfg["editor"] = map[string]any{
		"modal": map[string]any{
			"wrap": wrap,
		},
	}
	return ret
}

type fnRunner struct {
	fn func(extensionID, path string, config config.Config) error
}

func (f fnRunner) Run(extensionID, path string, config config.Config) error {
	return f.fn(extensionID, path, config)
}

func (f fnRunner) Close() error {
	return nil
}

func newSafeHandler(m *testWorkspaceManagerHandler) *safeHandler {
	// Force the command Prompt into sync mode so completion runs
	// inline on the test goroutine. The async path spawns a raw
	// goroutine that iterates the prompt's history slice without
	// holding the harness lock; in production the single-threaded
	// event loop serialises everything so this is safe, but tests
	// drive Handle from the test goroutine while the prompt is still
	// iterating, which races with the next History.Add. See the data
	// race fixed for TestWorkspaceManagerHandlerDraw.
	m.forceSyncCommandPrompt = true
	return &safeHandler{
		Component: m,
		Handler:   m, mu: m.mu,
	}
}

func newWriterForAttrTesting(width, height int) *term.StringWriter {
	writer := term.NewStringWriter(width, height)
	writer.ForegroundCh = '#'
	return writer
}

// TestCloseWorkspaceRemovesClosedWorkspaceFromManager guards against a
// regression where closing a workspace via the IDE left the workspace
// rooted in workspace.Manager. Each closed workspace would keep its scheme
// (and the file/watcher state owned by the scheme) alive forever, which
// caused steady memory growth on workspace open/close cycles.
func TestCloseWorkspaceRemovesClosedWorkspaceFromManager(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	manager := workspace.NewManager(config.NopConfig())
	require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
		workspace.NewMemoryScheme))

	uri, err := workspaceapi.ParseURI(fmt.Sprintf("memory:///%s", dir))
	require.NoError(t, err)

	runner := FuncExtensionsRunner(testRunnerFn)
	m := newTestWorkspaceManagerHandlerWithManagerAndExtensions(t, manager,
		&uri, defaultCfg(), runner, nil, dir, nil, nopShutdownShaderConfig())
	t.Cleanup(func() { _ = m.Close() })

	// Sanity: workspace was added to the Manager.
	require.True(t, manager.HasWorkspace(uri),
		"workspace should be registered with the Manager after init")

	require.NoError(t, m.commandCloseWorkspace())
	require.Equal(t, 0, m.workspaceCount)

	require.False(t, manager.HasWorkspace(uri),
		"closing a workspace must remove it from workspace.Manager so its scheme can be GC'd")
}
