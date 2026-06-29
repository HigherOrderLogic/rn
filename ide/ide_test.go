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
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/docmarshal/docbson"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/term/vte/vtereservoir"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
)

func TestIDEInitializationIntegration(t *testing.T) {
	t.Run("does not panic with sample config", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		err := os.WriteFile(configFile.Name(), []byte(sampleConfig), 0666)
		require.NoError(t, err)

		cwdURI, err := workspaceapi.CurrentUserHostURI(".")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(cwdURI.String(), configFile.Name(), dir, newTestStorage(t, dir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("does not panic with empty config", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		err := os.WriteFile(configFile.Name(), []byte("{}"), 0666)
		require.NoError(t, err)

		cwdURI, err := workspaceapi.CurrentUserHostURI(".")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(cwdURI.String(), configFile.Name(),
			dir, newTestStorage(t, dir), WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("takes a non-URI as a workspace", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		err := os.WriteFile(configFile.Name(), []byte("{}"), 0666)
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(".", configFile.Name(), dir, newTestStorage(t, dir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		// addWorkspace launches the workspace install (Phase B) in a
		// goroutine; closeResources must wait for that to finish
		// before tearing down the workspace.Manager, otherwise an
		// in-flight vtereservoir StartCommand races with the manager
		// closing the file scheme's open *os.Files.
		i.WaitWorkspaces()
		assert.NoError(t, i.closeResources())
	})

	t.Run("creates non-existing directories for log file", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		configData := fmt.Sprintf("log_path: %s/bla/bla/bla/debug.log", dir)
		err = os.WriteFile(configFile.Name(), []byte(configData), 0666)
		require.NoError(t, err)

		i := new(IDE)
		err = i.init(".", configFile.Name(), dir, newTestStorage(t, dir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("is able to initialize without a cwd", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init("", configFile.Name(), dir, newTestStorage(t, dir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.root)
		assert.NoError(t, i.closeResources())
	})

	t.Run("init shader is run when passed WithInitShader option", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		initShader := new(mockShader)

		dataDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dataDir)
		})

		i := new(IDE)
		err = i.init("", configFile.Name(), dataDir, newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)),
			WithInitShader(
				func(_ term.Attributes, _ component.FrameCharSet) shader.Shader {
					return initShader
				},
				30, 1*time.Second,
			))
		require.NoError(t, err)

		i.initRunning()
		i.root.Draw(&term.NoopWriter{})

		assert.True(t, initShader.called)
		assert.NoError(t, i.closeResources())
	})

	t.Run("init shader factory receives attrs set via SetDefaultAttributes before Ready", func(t *testing.T) {
		// Documents RUNE-203 contract: callers MUST invoke
		// SetDefaultAttributes before Ready() so the init-shader
		// factory observes the configured GUI theme background.
		// Calling SetDefaultAttributes after Ready() leaves the
		// shader runner painting with the stale config defAttr.
		configFile, _ := makeTestFiles(t)

		dataDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dataDir)
		})

		var captured term.Attributes
		i, err := New("", configFile.Name(), dataDir, newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)),
			WithInitShader(
				func(attr term.Attributes, _ component.FrameCharSet) shader.Shader {
					captured = attr
					return new(mockShader)
				},
				30, 1*time.Second,
			))
		require.NoError(t, err)

		wantAttr := term.Attributes{
			Fg: term.ColorWhite,
			Bg: term.ColorBlack,
		}
		i.SetDefaultAttributes(wantAttr)

		_ = i.Ready()

		assert.Equal(t, wantAttr, captured,
			"init shader factory must observe attrs set via "+
				"SetDefaultAttributes prior to Ready()")
		assert.NoError(t, i.closeResources())
	})
}

func TestOpen(t *testing.T) {
	t.Parallel()
	assertURI := func(t *testing.T, i *IDE, expected workspaceapi.URI) {
		ex := i.workspaceHandler.exHandler(i.workspaceHandler.focusHandler())
		uri, _, ok := ex.handlerInFocus()
		require.True(t, ok)
		assert.Equal(t, expected, uri)
	}

	t.Run("empty workspace", func(t *testing.T) {
		t.Parallel()
		file, config := makeTestFiles(t)
		dataDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dataDir) })
		i, err := New("", config.Name(), dataDir, newTestStorage(t, dataDir), WithPublishEvent(nopPublishEvent))
		require.NoError(t, err)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)
	})

	t.Run("a workspace", func(t *testing.T) {
		t.Parallel()
		file, config := makeTestFiles(t)
		dataDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dataDir) })
		i, err := New(os.TempDir(), config.Name(), dataDir, newTestStorage(t, dataDir), WithPublishEvent(nopPublishEvent))
		require.NoError(t, err)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)
	})

	t.Run("syntax enabled, empty workspace", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(
			release.Package{Name: "go", Latest: "3"},
		)
		bundles := idepkgtest.MakeBundles(
			[]release.Bundle{
				{Package: "go", Version: "3"},
			},
		)
		file, config := makeTestFiles(t)
		dataDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dataDir) })
		rm := idepkgtest.NewReleaseManager(pkgs, bundles)
		i, err := New("", config.Name(), dataDir, newTestStorage(t, dataDir), WithReleaseManager(rm), WithPublishEvent(nopPublishEvent))
		require.NoError(t, err)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		logrus.SetLevel(logrus.TraceLevel)

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)

		// allow for syntax to unpack things
		time.Sleep(200 * time.Millisecond)
	})
}

// TestHomeWorkspaceDoesNotStartExtensions is an end-to-end guard that a
// configured extension is never started on the home/empty workspace. The home
// workspace deliberately runs no extensions because they recursively walk the
// workspace root for .gitignore files at startup, which is ruinously expensive
// when the root is the user's home directory.
func TestHomeWorkspaceDoesNotStartExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(
		"editor:\n  mode: modal\n"+
			"workspace:\n  home: "+dir+"\n"+
			"extensions:\n  rune-agent:\n    path: rune-agent-bin\n"), 0o644))

	recorder := &recordingRunner{}
	mu := new(sync.Mutex)
	i, err := New("", configPath, dir, newTestStorage(t, dir),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(recordingExtensionsRunner{runner: recorder}),
		WithLocker(mu),
		WithScheduleNextTick(func(fn func()) bool {
			fn()
			return true
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })
	_ = i.Ready()
	i.WaitWorkspaces()

	// The home runner is built and its extensions, if any, are started from a
	// background goroutine; give that path time to run so a regression that
	// reintroduces the start is caught rather than racing past the assertion.
	time.Sleep(200 * time.Millisecond)

	assert.Empty(t, recorder.runCalls(),
		"no extension may be started on the home workspace")
}

// TestWonAliasIntegration is an end-to-end test that wires the IDE
// through real configuration to verify the `won` alias from the user's
// `~/.runedev/config.yaml`:
//
//	command:
//	  aliases:
//	    won:
//	      command: workspaceopen
//	      completer:
//	        - '{history}'
//	        - '{file}'
//
// The test goes through the real configuration loader, the real
// command alias parser, the real workspace history (backed by
// localstorage on a temp dir), and the real text.Component completion
// path. After dispatching `:won <repoA>` once, querying the alias
// completion again must surface "<repoA>" as the first match — proving
// that `{history}` is wired correctly all the way from the YAML
// completer chain through search.History.HistoryIterator.
func TestWonAliasIntegration(t *testing.T) {
	dataDir := t.TempDir()
	repoA := t.TempDir()
	repoB := t.TempDir()

	// Real config file with the `won` alias in YAML form, identical to
	// what the user has in ~/.runedev/config.yaml.
	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
editor:
  mode: modal
command:
  show_manual: false
  key: ":"
  aliases:
    won:
      command: workspaceopen
      completer:
        - '{history}'
        - '{file}'
`), 0666))

	// Initial cwd workspace. We use repoB (different from repoA) so
	// the file-based completer's results are clearly distinguishable
	// from the history entries.
	// Seed repoB with a file so we can Open it as the initial
	// workspace; without an opened workspace the IDE root forwards
	// events differently and the command prompt would not even be
	// reachable from the empty root handler.
	repoBFile := filepath.Join(repoB, "seed.txt")
	require.NoError(t, os.WriteFile(repoBFile, nil, 0666))

	mu := new(sync.Mutex)
	i, err := New(repoB, configPath, dataDir, newTestStorage(t, dataDir),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
		WithLocker(mu),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })
	root := i.Ready()

	repoBURI, err := workspaceapi.CurrentUserHostURI(repoBFile)
	require.NoError(t, err)
	mu.Lock()
	require.NoError(t, i.Open(repoBURI))
	mu.Unlock()

	// Sanity: alias is wired through configuration.
	wh := i.workspaceHandler
	aliases := i.ideConfig.commandAliases()
	wonAlias, ok := aliases["won"]
	require.True(t, ok, "won alias must be registered from YAML config")
	require.Len(t, wonAlias.Completers, 2,
		"won alias must have two completer factories ({history} + {file})")

	// Dispatch the alias by driving keyboard input through the IDE
	// root handler — the same code path a real user takes. This goes
	// through the command Prompt, which is what records the entered
	// command line into search.History on Enter.
	// term.ParseKeys requires `<space>` rather than literal spaces;
	// the alias name has none, but the command itself needs the
	// space token between `won` and the path argument.
	wonInvocation := ":won<space>" + repoA + "<enter>"
	keys, err := term.ParseKeys(wonInvocation)
	require.NoError(t, err)
	root.Resize(80, 24)
	for _, k := range keys {
		mu.Lock()
		root.Handle(term.Event{Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key})
		mu.Unlock()
	}

	// Wait for any async completion machinery to settle (the
	// dispatch path runs the alias handler on a goroutine and
	// records history when it returns).
	wh.focusEx().Wait()

	// Now ask the focused text.Component to complete the same alias
	// with no partial last arg. This is exactly the call the prompt
	// issues when the user types `:won ` (alias + space).
	ex := wh.exHandler(wh.focusHandler())
	require.NotNil(t, ex, "expected a focused ex handler after dispatch")
	mu.Lock()
	it, _, err := ex.comp.CompleteCommand(t.Context(),
		textapi.Command{Name: "won", Args: []string{""}})
	mu.Unlock()
	require.NoError(t, err)
	defer func() { _ = it.Close() }()

	got, err := iterator.ToSlice(t.Context(), it)
	require.NoError(t, err)
	require.NotEmpty(t, got,
		"won completion must surface at least the prior `won %s` history entry, "+
			"got nothing — `{history}` is not wired correctly", repoA)

	// History must come first per chain order in the YAML config. The
	// HistoryCompleter strips the alias-name prefix, so the entry the
	// user sees back is just the arg that was passed (the repoA path).
	assert.Equal(t, repoA, got[0],
		"first completion must be the prior `won` argument from history; "+
			"got %q. full result: %v", got[0], got)
}

// TestE2EIssueImplementAliasChainOrdering reproduces the user-reported
// `issue-implement` failure. The alias chains a nested alias (standing
// in for `worktreenew`) followed by two `extensionready` steps:
//
//	issue-implement:
//	  command:
//	    - worktreelike $1
//	    - extensionready dummy agent $1
//	    - extensionready dummy chatskill issue-implement $1
//
// `worktreelike` is itself an alias whose body runs a single command
// that records its execution (the stand-in for the real worktree
// creation; we do not need a git worktree to exercise the ordering
// bug). The three steps must run in submission order: the nested alias
// first, then `agent`, then `chatskill`.
//
// The bug: ex.dispatchCommand dispatches each expanded alias step via
// text.Component.DispatchCommand, which only resolves subscribed
// commands — not alias names. So a step whose name is itself an alias
// (`worktreelike`) is silently dropped: its body never runs. In the
// real config that means the worktree workspace is never created and
// the `extensionready` steps run against the wrong workspace.
func TestE2EIssueImplementAliasChainOrdering(t *testing.T) {
	dataDir := t.TempDir()
	repo := t.TempDir()
	seed := filepath.Join(repo, "seed.txt")
	require.NoError(t, os.WriteFile(seed, nil, 0o666))

	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
editor:
  mode: modal
command:
  show_manual: false
  key: ":"
  aliases:
    worktreelike:
      command:
        - recordfirst $1
    issue-implement:
      command:
        - worktreelike $1
        - extensionready dummy agent $1
        - extensionready dummy chatskill issue-implement $1
`), 0o666))

	mu := new(sync.Mutex)
	// Mirror the production event loop: a single consumer runs
	// scheduled callbacks in enqueue order (run.go reads one
	// EventInterrupt at a time from a single channel). A goroutine
	// per call would let two dispatches race for mu and reorder, which
	// production never does.
	scheduleNextTick, _ := newTestScheduler(mu)
	runner := newPerIDReadyRunner("dummy")
	runnerFn := func(
		_ workspaceapi.URI,
		_ map[extensionapi.Permission]extension.ResourceRegistrar,
		_ string, _ browser.Notifications,
		_, _ schemeapi.Executor, _ extension.Grantor, _ text.Editor,
		_ ideauthorizer.PromptOpener, _ storageapi.Service,
		_ func(func()) bool) (extension.Runner, error) {
		return runner, nil
	}

	i, err := New(repo, configPath, dataDir, newTestStorage(t, dataDir),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(runnerFn)),
		WithScheduleNextTick(scheduleNextTick),
		WithLocker(mu),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })
	root := i.Ready()
	mu.Lock()
	root.Resize(80, 24)
	mu.Unlock()
	i.WaitWorkspaces()

	seedURI, err := workspaceapi.CurrentUserHostURI(seed)
	require.NoError(t, err)
	mu.Lock()
	require.NoError(t, i.Open(seedURI))
	mu.Unlock()

	// The dummy extension registers the follow-up commands the alias
	// dispatches; each records its name (plus the nested-alias step).
	var orderMu sync.Mutex
	var order []string
	record := func(name string) text.CommandHandler {
		return text.FuncCommandHandler(
			func(context.Context, textapi.Command) error {
				orderMu.Lock()
				order = append(order, name)
				orderMu.Unlock()
				return nil
			}, nil)
	}
	require.NoError(t, i.workspaceHandler.subscribeCommand(
		textapi.CommandManual{Name: "recordfirst"}, record("first")))
	require.NoError(t, i.workspaceHandler.subscribeCommand(
		textapi.CommandManual{Name: "agent"}, record("agent")))
	require.NoError(t, i.workspaceHandler.subscribeCommand(
		textapi.CommandManual{Name: "chatskill"}, record("chatskill")))

	invocation := ":issue-implement<space>my-branch<enter>"
	keys, err := term.ParseKeys(invocation)
	require.NoError(t, err)
	for _, k := range keys {
		mu.Lock()
		root.Handle(term.Event{Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key})
		mu.Unlock()
	}
	i.workspaceHandler.focusEx().Wait()

	// Let the extension become ready so the queued extensionready
	// follow-up commands dispatch.
	runner.release("dummy")

	require.Eventually(t, func() bool {
		orderMu.Lock()
		defer orderMu.Unlock()
		return len(order) == 3
	}, 5*time.Second, 20*time.Millisecond,
		"all three alias steps must run: the nested worktreelike alias, "+
			"then agent, then chatskill")

	orderMu.Lock()
	defer orderMu.Unlock()
	assert.Equal(t, []string{"first", "agent", "chatskill"}, order,
		"alias steps must dispatch in submission order; a missing "+
			"\"first\" means the nested worktreelike alias was dropped "+
			"instead of expanded")
}

// TestE2EExtensionReadyChainOrderingNoWorktree is the same scenario
// without the leading nested alias, isolating the extensionready
// ordering on a single workspace:
//
//	issue-implement:
//	  command:
//	    - extensionready dummy agent $1
//	    - extensionready dummy chatskill issue-implement $1
//
// With no pending workspace reservation, both steps run against the
// focused workspace. `chatskill` must dispatch after `agent` — the
// per-extension extensionready queue must preserve submission order
// even when both follow-up commands wait on the same extension.
func TestE2EExtensionReadyChainOrderingNoWorktree(t *testing.T) {
	dataDir := t.TempDir()
	repo := t.TempDir()
	seed := filepath.Join(repo, "seed.txt")
	require.NoError(t, os.WriteFile(seed, nil, 0o666))

	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
editor:
  mode: modal
command:
  show_manual: false
  key: ":"
  aliases:
    issue-implement:
      command:
        - extensionready dummy agent $1
        - extensionready dummy chatskill issue-implement $1
`), 0o666))

	mu := new(sync.Mutex)
	scheduleNextTick, _ := newTestScheduler(mu)
	runner := newPerIDReadyRunner("dummy")
	runnerFn := func(
		_ workspaceapi.URI,
		_ map[extensionapi.Permission]extension.ResourceRegistrar,
		_ string, _ browser.Notifications,
		_, _ schemeapi.Executor, _ extension.Grantor, _ text.Editor,
		_ ideauthorizer.PromptOpener, _ storageapi.Service,
		_ func(func()) bool) (extension.Runner, error) {
		return runner, nil
	}

	i, err := New(repo, configPath, dataDir, newTestStorage(t, dataDir),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(runnerFn)),
		WithScheduleNextTick(scheduleNextTick),
		WithLocker(mu),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })
	root := i.Ready()
	mu.Lock()
	root.Resize(80, 24)
	mu.Unlock()
	i.WaitWorkspaces()

	seedURI, err := workspaceapi.CurrentUserHostURI(seed)
	require.NoError(t, err)
	mu.Lock()
	require.NoError(t, i.Open(seedURI))
	mu.Unlock()

	var orderMu sync.Mutex
	var order []string
	record := func(name string) text.CommandHandler {
		return text.FuncCommandHandler(
			func(context.Context, textapi.Command) error {
				orderMu.Lock()
				order = append(order, name)
				orderMu.Unlock()
				return nil
			}, nil)
	}
	require.NoError(t, i.workspaceHandler.subscribeCommand(
		textapi.CommandManual{Name: "agent"}, record("agent")))
	require.NoError(t, i.workspaceHandler.subscribeCommand(
		textapi.CommandManual{Name: "chatskill"}, record("chatskill")))

	invocation := ":issue-implement<space>my-branch<enter>"
	keys, err := term.ParseKeys(invocation)
	require.NoError(t, err)
	for _, k := range keys {
		mu.Lock()
		root.Handle(term.Event{Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key})
		mu.Unlock()
	}
	i.workspaceHandler.focusEx().Wait()

	runner.release("dummy")

	require.Eventually(t, func() bool {
		orderMu.Lock()
		defer orderMu.Unlock()
		return len(order) == 2
	}, 5*time.Second, 20*time.Millisecond,
		"both extensionready follow-up commands must run")

	orderMu.Lock()
	defer orderMu.Unlock()
	assert.Equal(t, []string{"agent", "chatskill"}, order,
		"chatskill must dispatch after agent; the per-extension "+
			"extensionready queue must preserve submission order")
}

// TestE2EWorkspaceReloadRestoresLayoutAndTerminalOutput drives the
// full real IDE — real config, real file scheme, real vte handler —
// through the same flow that surfaced the original
// :workspacereload DeadlineExceeded bug, and asserts that the
// post-reload IDE preserves both the workspace layout and the
// captured terminal output.
//
// Flow:
//  1. cwd workspace points to a temp dir on disk; auto_restore is on
//     so the reload skips the restore-prompt.
//  2. Open a test file (left window).
//  3. windownew right creates a fresh empty window on the right.
//  4. terminalnew opens a real vte.Handler in that right window
//     running `echo abc`.
//  5. After the terminal output settles, the test snapshots the
//     layout topology and the textual content of the terminal cells.
//  6. :workspacereload is dispatched the same way a user would
//     dispatch it — through the command prompt.
//  7. After the workspace re-installs, the layout must be the same
//     vertical split, the right window must still hold a vte
//     handler, and its snapshot must still contain "abc".
func TestE2EWorkspaceReloadRestoresLayoutAndTerminalOutput(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()

	// The command key is Ctrl+backslash. Tests pick that combination
	// instead of `:` because, after opening a real vte terminal,
	// keyboard focus lands on the vte and a plain `:` would be eaten
	// by the shell — `<c-\\>` reliably opens the command prompt no
	// matter which handler is in focus.
	//
	// auto_restore: true prevents the post-reload "Do you want to
	// restore the previous session?" prompt from interposing itself
	// between the reload and the test assertions.
	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
editor:
  mode: modal
command:
  key: "<c-\\\\>"
workspace:
  auto_restore: true
`), 0o666))

	testFile := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello world\n"), 0o644))

	mu := new(sync.Mutex)
	// Scheduler that mirrors the production event loop: scheduled
	// callbacks run on a fresh goroutine while holding mu, so async
	// addWorkspace / reload installs can land back into IDE state.
	scheduleNextTick := func(fn func()) bool {
		go func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		}()
		return true
	}

	i, err := New(dir, configPath, dataDir, newTestStorage(t, dataDir),
		WithLocker(mu),
		WithScheduleNextTick(scheduleNextTick),
		WithPublishEvent(func(term.Event) bool { return true }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })

	root := i.Ready()
	mu.Lock()
	root.Resize(80, 24)
	mu.Unlock()
	i.WaitWorkspaces()

	sendKeys := func(t *testing.T, seq string) {
		t.Helper()
		keys, err := term.ParseKeys(seq)
		require.NoError(t, err)
		for _, k := range keys {
			mu.Lock()
			root.Handle(term.Event{
				Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key,
			})
			mu.Unlock()
			i.WaitInflight()
		}
	}

	// 1) Open the file (left window).
	// 2) Open an empty window on the right.
	// 3) Open a terminal in that right window running `echo abc`.
	sendKeys(t,
		"<c-\\\\>edit<space>"+testFile+"<enter>"+
			"<c-\\\\>windownew<space>right<enter>"+
			"<c-\\\\>terminalnew<space>echo<space>abc<enter>",
	)

	// Find the terminal we just created and wait for "abc" to land
	// in its active cells.
	type windowSnapshot struct {
		windowID  uint64
		cellsText string
	}
	snapshotTerminalWindow := func() (windowSnapshot, bool) {
		mu.Lock()
		defer mu.Unlock()
		ex := i.workspaceHandler.focusEx()
		var found windowSnapshot
		var ok bool
		ex.comp.Browser().IterateWindows(func(win browser.Window) {
			if ok {
				return
			}
			content, cerr := win.Content()
			if cerr != nil {
				return
			}
			vte, isVTE := content.(vtereservoir.VTE)
			if !isVTE {
				return
			}
			snap, sErr := vte.Snapshot()
			if sErr != nil {
				return
			}
			found = windowSnapshot{
				windowID:  win.WindowID(),
				cellsText: term.CellsToString(snap.ActiveCells()),
			}
			ok = true
		})
		return found, ok
	}
	var before windowSnapshot
	require.Eventually(t, func() bool {
		snap, ok := snapshotTerminalWindow()
		if !ok {
			return false
		}
		if !strings.Contains(snap.cellsText, "abc") {
			return false
		}
		before = snap
		return true
	}, 10*time.Second, 50*time.Millisecond,
		"terminal did not produce `abc` before workspacereload")

	// Capture the layout structure before reload — must remain
	// identical after reload.
	mu.Lock()
	layoutBefore := i.workspaceHandler.focusEx().comp.Browser().TileLayout()
	mu.Unlock()
	require.Equal(t, tcomponent.SplitOrientationVertical, layoutBefore.Split,
		"pre-reload layout must be a single vertical split "+
			"(left file / right terminal)")
	require.Len(t, layoutBefore.Children, 2,
		"pre-reload layout must have exactly two leaves")
	rightLeafBefore := layoutBefore.Children[1]
	require.Equal(t, before.windowID, rightLeafBefore.WindowID,
		"the terminal must live in the right leaf of the pre-reload layout")

	// 4) Drive :workspacereload through the command prompt — the
	// same path a real user takes.
	sendKeys(t, "<c-\\\\>workspacereload<enter>")

	// reload tears down and re-adds the workspace asynchronously
	// through addWorkspace.
	i.WaitWorkspaces()
	i.WaitInflight()

	require.Eventually(t, func() bool {
		mu.Lock()
		ex := i.workspaceHandler.focusEx()
		layout := ex.comp.Browser().TileLayout()
		mu.Unlock()
		return len(layout.Children) == 2
	}, 30*time.Second, 50*time.Millisecond,
		"workspace reload did not restore the split layout")

	// Layout must be preserved: still one vertical split with two
	// leaves.
	mu.Lock()
	layoutAfter := i.workspaceHandler.focusEx().comp.Browser().TileLayout()
	mu.Unlock()
	require.Equal(t, tcomponent.SplitOrientationVertical, layoutAfter.Split,
		"post-reload layout must remain a vertical split")
	require.Len(t, layoutAfter.Children, 2,
		"post-reload layout must still have two leaves")

	// The right window's terminal must be re-created with the same
	// captured output. RestoreTileLayout allocates fresh window IDs,
	// so we do not require the leaves' WindowIDs to match pre-reload
	// — only that a vte still lives in the workspace and that its
	// snapshot contains "abc".
	var after windowSnapshot
	require.Eventually(t, func() bool {
		snap, ok := snapshotTerminalWindow()
		if !ok {
			return false
		}
		if !strings.Contains(snap.cellsText, "abc") {
			return false
		}
		after = snap
		return true
	}, 10*time.Second, 50*time.Millisecond,
		"post-reload terminal must still contain `abc`")

	// The restored terminal must live in the right leaf of the
	// post-reload layout.
	rightLeafAfter := layoutAfter.Children[1]
	require.NotZero(t, rightLeafAfter.WindowID,
		"post-reload right leaf must reference a concrete window")
	require.Equal(t, rightLeafAfter.WindowID, after.windowID,
		"the restored terminal must live in the right leaf of the post-reload layout")
}

func TestE2EClipboardPasteIntoNoEchoTerminalRead(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()

	script := filepath.Join(dir, "read-secret.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf 'password: '
stty -echo
IFS= read -r secret
stty echo
printf '\nRESULT:%s\n' "$secret"
`), 0o755))

	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
clipboard: memory
editor:
  mode: modal
command:
  key: "<c-\\\\>"
  key_bindings:
    <m-v>: clipboardpaste
`), 0o666))

	mu := new(sync.Mutex)
	scheduleNextTick := func(fn func()) bool {
		go debug.CapturePanicReport(func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		})
		return true
	}
	i, err := New(dir, configPath, dataDir, newTestStorage(t, dataDir),
		WithLocker(mu),
		WithScheduleNextTick(scheduleNextTick),
		WithPublishEvent(func(term.Event) bool { return true }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })

	root := i.Ready()
	mu.Lock()
	root.Resize(80, 24)
	mu.Unlock()
	i.WaitWorkspaces()

	sendKeys := func(t *testing.T, seq string) {
		t.Helper()
		keys, err := term.ParseKeys(seq)
		require.NoError(t, err)
		for _, k := range keys {
			mu.Lock()
			root.Handle(term.Event{
				Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key,
			})
			mu.Unlock()
			i.WaitInflight()
		}
	}

	snapshotTerminalText := func() (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		ex := i.workspaceHandler.focusEx()
		var text string
		var ok bool
		ex.comp.Browser().IterateWindows(func(win browser.Window) {
			if ok {
				return
			}
			content, err := win.Content()
			if err != nil {
				return
			}
			vte, isVTE := content.(vtereservoir.VTE)
			if !isVTE {
				return
			}
			snap, err := vte.Snapshot()
			if err != nil {
				return
			}
			text = term.CellsToString(snap.ActiveCells())
			ok = true
		})
		return text, ok
	}

	sendKeys(t, "<c-\\\\>terminalnew<space>"+script+"<enter>")
	var promptText string
	require.Eventually(t, func() bool {
		text, ok := snapshotTerminalText()
		promptText = text
		return ok && strings.Contains(text, "password:")
	}, 10*time.Second, 50*time.Millisecond,
		"terminal did not reach password prompt; screen was:\n%s", promptText)

	require.NoError(t, i.workspaceHandler.clip.Copy(
		clipboard.DefaultRegisterID,
		clipboard.Data{Text: "s3cr3t"},
	))
	sendKeys(t, "<m-v><enter>")

	var resultText string
	require.Eventually(t, func() bool {
		text, ok := snapshotTerminalText()
		resultText = text
		return ok && strings.Contains(text, "RESULT:s3cr3t")
	}, 10*time.Second, 50*time.Millisecond,
		"terminal did not receive pasted secret; screen was:\n%s", resultText)
}

// TestE2EFileExplorerRefreshDoesNotClobberClipboard reproduces the bug
// where switching git branches (which changes files under the workspace
// root and fires FS-watcher events) clobbers the user's system clipboard
// with the file explorer's directory listing.
//
// The explorer reuses the standard editor, whose copy-on-delete
// subscriber (text.WithCopyDelete) copies any deleted buffer content to
// the default register. When an FS event drives refreshTree ->
// Component.Refresh -> rewriteBufferFromTree, the old tree text is
// deleted from the shared cell.Buffer and was being copied into the
// default register. A programmatic refresh must not touch the clipboard;
// only a real user delete should.
func TestE2EFileExplorerRefreshDoesNotClobberClipboard(t *testing.T) {
	rawDir := t.TempDir()
	dir, err := filepath.EvalSymlinks(rawDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "alpha.go"), []byte("package a\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.go"), []byte("package b\n"), 0o644))

	dataDir := t.TempDir()
	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
clipboard: memory
editor:
  mode: modal
command:
  key: "<c-\\\\>"
`), 0o666))

	mu := new(sync.Mutex)
	scheduleNextTick := func(fn func()) bool {
		go debug.CapturePanicReport(func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		})
		return true
	}

	i, err := New(dir, configPath, dataDir, newTestStorage(t, dataDir),
		WithLocker(mu),
		WithScheduleNextTick(scheduleNextTick),
		WithPublishEvent(func(term.Event) bool { return true }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })

	root := i.Ready()
	mu.Lock()
	root.Resize(80, 24)
	mu.Unlock()
	i.WaitWorkspaces()

	// Open the file explorer via the real :fexplorer command path.
	mu.Lock()
	ex := i.workspaceHandler.focusEx()
	require.NoError(t, ex.fexplorer(context.Background()))
	require.NotNil(t, ex.fileExplorerWin, "explorer window should be open")
	explorer := ex.fileExplorerHandler
	require.NotNil(t, explorer)
	mu.Unlock()

	// Prime the default register with a sentinel the user "copied"
	// earlier. A programmatic refresh must leave this untouched.
	const sentinel = "user-copied-sentinel"
	mu.Lock()
	require.NoError(t, ex.clip.Copy(
		clipboard.DefaultRegisterID, clipboard.Data{Text: sentinel}))
	mu.Unlock()

	// Add a sibling file on disk and drive the explorer's real
	// FS-event handler, exactly as the workspace watcher does when a
	// branch switch changes the working tree.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "gamma.go"), []byte("package g\n"), 0o644))
	gammaURI, err := workspaceapi.ParseURI("file://" + filepath.Join(dir, "gamma.go"))
	require.NoError(t, err)

	mu.Lock()
	explorer.onFSEvent(context.Background(),
		textapi.Event{Type: textapi.EventTypeCreate, URI: gammaURI})
	mu.Unlock()
	i.WaitInflight()

	mu.Lock()
	data, err := ex.clip.Paste(clipboard.DefaultRegisterID)
	mu.Unlock()
	require.NoError(t, err)
	require.Equal(t, sentinel, data.Text,
		"file explorer FS-driven refresh must not clobber the clipboard; "+
			"got:\n%s", data.Text)
}

// TestE2EFileExplorerEnterOpensFileAfterReload reproduces the bug
// where, after opening files and running :workspacereload, re-opening
// the file explorer and pressing <enter> on a file does nothing.
//
// A reload discards the old ex and restores the previous session's
// files into windows. The freshly re-opened explorer captures a
// different window as its target, so opening an already-restored file
// hit browser.Window.SetContent's ErrTabNotFree (the tab is already
// rendered in its restored window). editFileURILocal swallowed that
// error, so pressing <enter> silently did nothing. The fix focuses
// the window that already owns the tab.
func TestE2EFileExplorerEnterOpensFileAfterReload(t *testing.T) {
	rawDir := t.TempDir()
	dir, err := filepath.EvalSymlinks(rawDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "alpha.txt"), []byte("alpha\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.txt"), []byte("beta\n"), 0o644))

	dataDir := t.TempDir()
	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
editor:
  mode: modal
command:
  key: "<c-\\\\>"
workspace:
  auto_restore: true
`), 0o666))

	mu := new(sync.Mutex)
	scheduleNextTick := func(fn func()) bool {
		go debug.CapturePanicReport(func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		})
		return true
	}

	i, err := New(dir, configPath, dataDir, newTestStorage(t, dataDir),
		WithLocker(mu),
		WithScheduleNextTick(scheduleNextTick),
		WithPublishEvent(func(term.Event) bool { return true }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })

	root := i.Ready()
	mu.Lock()
	root.Resize(120, 40)
	mu.Unlock()
	i.WaitWorkspaces()

	sendKeys := func(t *testing.T, seq string) {
		t.Helper()
		keys, err := term.ParseKeys(seq)
		require.NoError(t, err)
		for _, k := range keys {
			mu.Lock()
			root.Handle(term.Event{
				Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key,
			})
			mu.Unlock()
			i.WaitInflight()
		}
	}

	focusedURI := func() string {
		mu.Lock()
		defer mu.Unlock()
		ex := i.workspaceHandler.focusEx()
		win, _ := ex.comp.Focus()
		if win == nil {
			return ""
		}
		content, cerr := win.Content()
		if cerr != nil || content == nil {
			return ""
		}
		tab, ok := content.(*browser.Tab)
		if !ok {
			return ""
		}
		return tab.URI().String()
	}

	// Open alpha.txt on the left, split a second window on the right
	// and open beta.txt there, then open the file explorer. The
	// explorer's target becomes the right window, distinct from the
	// window that holds alpha.txt.
	sendKeys(t, "<c-\\\\>edit<space>"+filepath.Join(dir, "alpha.txt")+"<enter>")
	sendKeys(t, "<c-\\\\>windownew<space>right<enter>")
	sendKeys(t, "<c-\\\\>edit<space>"+filepath.Join(dir, "beta.txt")+"<enter>")
	sendKeys(t, "<c-\\\\>fexplorer<enter>")

	// Reload the workspace. It tears down the ex and restores the two
	// files asynchronously.
	sendKeys(t, "<c-\\\\>workspacereload<enter>")
	i.WaitWorkspaces()
	i.WaitInflight()
	require.Eventually(t, func() bool {
		mu.Lock()
		ex := i.workspaceHandler.focusEx()
		var files int
		if ex != nil {
			for _, tab := range ex.comp.Browser().Tabs() {
				if strings.HasSuffix(tab.URI().String(), ".txt") {
					files++
				}
			}
		}
		mu.Unlock()
		return files == 2
	}, 30*time.Second, 50*time.Millisecond,
		"workspace reload did not restore both files")

	// Re-open the explorer. Its target window is not the window that
	// holds alpha.txt, so opening alpha.txt must focus alpha.txt's
	// existing window rather than silently doing nothing.
	sendKeys(t, "<c-\\\\>fexplorer<enter>")

	// The explorer renders the workspace tree; the cursor starts on
	// the first entry. Walk down until alpha.txt is focused. Each
	// <enter> on a directory expands/collapses it; on a file it opens
	// it. With a small flat tree alpha.txt is reached within a few
	// rows.
	var opened bool
	for range 8 {
		sendKeys(t, "<enter>")
		if strings.HasSuffix(focusedURI(), "/alpha.txt") {
			opened = true
			break
		}
		sendKeys(t, "<down>")
	}
	require.True(t, opened,
		"pressing enter on alpha.txt in the file explorer after a "+
			"workspacereload must focus the window showing alpha.txt; "+
			"focusedURI=%q", focusedURI())
}

type mockShader struct {
	called bool
	frames []int
}

func (s *mockShader) Shade(frame, total int, in [][]term.Cell) {
	s.called = true
	s.frames = append(s.frames, frame)
}

func makeTestFiles(t *testing.T) (*os.File, *os.File) {
	configFile, err := os.CreateTemp("", "six_ide_test.*.yaml")
	require.NoError(t, err)

	_, err = configFile.WriteString("{}")
	require.NoError(t, err)

	require.NoError(t, configFile.Close())

	file, err := os.CreateTemp("", "six_ide_test.*.go")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	t.Cleanup(func() {
		_ = os.Remove(configFile.Name())
		_ = os.Remove(file.Name())
	})

	return configFile, file
}

// newTestStorage returns the localstorage flavor every IDE test uses.
// Tests pass it as the storage argument to ide.New / IDE.init.
func newTestStorage(t *testing.T, dataDir string) storageapi.Service {
	t.Helper()
	return localstorage.New(context.Background(), dataDir, docbson.Marshaler())
}

func testRunnerFn(
	uri workspaceapi.URI,
	res map[extensionapi.Permission]extension.ResourceRegistrar,
	dataDir string, n browser.Notifications,
	exec, extExec schemeapi.Executor,
	grantor extension.Grantor,
	editor text.Editor,
	promptOpener ideauthorizer.PromptOpener, storage storageapi.Service,
	scheduleNextTick func(func()) bool) (extension.Runner, error) {
	return testRunner{}, nil
}

type testRunner struct {
}

func (r testRunner) Run(extensionID, path string, config config.Config) error {
	return nil
}

func (r testRunner) Close() error {
	return nil
}

func (r testRunner) WaitReady(ctx context.Context, id string) error {
	return nil
}

// TestIDEExoMisconfigurationFallsBackToDefault is an end-to-end
// guard against exoeditor.New panics when the user's config selects
// `editor.mode = "exo"` but does not supply both required fields
// (`editor.exo.command` containing {file}, and `editor.exo.goto`).
// validateExo rewrites the mode back to "modal" so the IDE boots
// with the built-in modal editor; this test asserts that the
// rewrite actually happens at the config layer so the workspace
// handler never reaches exoeditor.New on a misconfigured input.
//
// Reproduces the panic chain that motivated this guard:
//
//	exoeditor.New: command is required
//	exoeditor.New: invalid gotoTemplate: ...
//
// Either panic would crash the IDE on startup when a user
// previously experimented with `editor.mode = "exo"` and removed
// only part of the exo block.
func TestIDEExoMisconfigurationFallsBackToDefault(t *testing.T) {
	cases := []struct {
		name   string
		exo    string // YAML body inserted under editor:exo
		hasKey bool   // when false, omit the exo block entirely
	}{
		{
			name:   "no exo block at all",
			hasKey: false,
		},
		{
			name: "empty command, valid goto",
			exo: `    command: ""
    goto: "<esc>:{line}<enter>{col}|"`,
			hasKey: true,
		},
		{
			name: "command without {file}, valid goto",
			exo: `    command: "vim"
    goto: "<esc>:{line}<enter>{col}|"`,
			hasKey: true,
		},
		{
			name: "valid command, empty goto",
			exo: `    command: "vim {file}"
    goto: ""`,
			hasKey: true,
		},
		{
			name:   "valid command, missing goto field",
			exo:    `    command: "vim {file}"`,
			hasKey: true,
		},
		{
			name: "valid command, invalid goto",
			exo: `    command: "vim {file}"
    goto: "<bogus-key>"`,
			hasKey: true,
		},
		{
			name: "empty command, empty goto",
			exo: `    command: ""
    goto: ""`,
			hasKey: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configFile, _ := makeTestFiles(t)
			cfg := "editor:\n  mode: exo\n"
			if tc.hasKey {
				cfg += "  exo:\n" + tc.exo + "\n"
			}
			require.NoError(t,
				os.WriteFile(configFile.Name(), []byte(cfg), 0666))

			dir, err := os.MkdirTemp("", "")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })

			cwdURI, err := workspaceapi.CurrentUserHostURI(".")
			require.NoError(t, err)

			// Use init() rather than New() so we can introspect
			// the post-load ideConfig before any workspace
			// handler reaches exoeditor.New. init() must not panic
			// for any of these inputs: validateExo rewrites
			// the mode back to "modal" before the workspace
			// handler instantiates the editor.
			i := new(IDE)
			require.NotPanics(t, func() {
				err = i.init(cwdURI.String(),
					configFile.Name(), dir, newTestStorage(t, dir),
					WithPublishEvent(nopPublishEvent),
					WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
					WithLocker(new(sync.Mutex)))
			}, "IDE init must not panic for misconfigured exo; "+
				"validateExo must rewrite editor.mode to a "+
				"safe fallback before reaching exoeditor.New")
			require.NoError(t, err,
				"IDE init must still succeed for "+
					"misconfigured exo; validateExo "+
					"surfaces a non-fatal config error and "+
					"the IDE boots with the fallback mode")

			assert.NotEqual(t, "exo", i.ideConfig.editorMode(),
				"after validateExo, editor.mode must not "+
					"remain exo; got %q",
				i.ideConfig.editorMode())
			assert.Equal(t, "modal", i.ideConfig.editorMode(),
				"validateExo falls back to the safe "+
					"default mode (modal); a different "+
					"value means the validator regressed "+
					"or a new code path skipped the "+
					"rewrite")

			assert.NoError(t, i.closeResources())
		})
	}
}

// TestIDEExoWellFormedConfigDoesNotFallBack guards against an
// over-eager validateExo that would rewrite legitimate exo
// configurations back to "modal". This is the positive
// counterexample to TestIDEExoMisconfigurationFallsBackToDefault.
func TestIDEExoWellFormedConfigDoesNotFallBack(t *testing.T) {
	configFile, _ := makeTestFiles(t)
	const cfg = `editor:
  mode: exo
  exo:
    command: "vim {file}"
    goto: "<esc>:{line}<enter>{col}|"
    quit: "<esc>:qa!<enter>"
`
	require.NoError(t,
		os.WriteFile(configFile.Name(), []byte(cfg), 0666))

	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	cwdURI, err := workspaceapi.CurrentUserHostURI(".")
	require.NoError(t, err)

	i := new(IDE)
	require.NotPanics(t, func() {
		err = i.init(cwdURI.String(),
			configFile.Name(), dir, newTestStorage(t, dir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
	})
	require.NoError(t, err)

	assert.Equal(t, "exo", i.ideConfig.editorMode(),
		"a complete exo config (command + goto) must be "+
			"preserved through validateExo")
	assert.Equal(t, "vim {file}", i.ideConfig.exoCommand())
	assert.Equal(t, "<esc>:{line}<enter>{col}|", i.ideConfig.exoGoto())

	assert.NoError(t, i.closeResources())
}

// TestE2EExoUserQuitAutoClosesTab boots a real IDE through ide.New
// with a working exo section, opens a file, types `:q<enter>` into
// the embedded editor and asserts that the tab is removed
// automatically once the editor process exits.
//
// The auto-close chain in production is: vim exits -> vte's
// comp.Run returns -> Handler publishes a term.EventNone via the
// host EventPublisher -> the host event loop calls root.Handle with
// that event -> WindowManager routes it to the focused Tab ->
// vte.Handler.Handle returns exit=true (because e.exit is set) ->
// Tab.Handle calls Component.RemoveTab(self), which drops the tab
// from c.buffers and tears the window down. This test stands in for
// the host event loop by intercepting the publish via
// WithPublishEvent and re-dispatching the event through root.Handle
// under the IDE locker.
func TestE2EExoUserQuitAutoClosesTab(t *testing.T) {
	bin, err := exec.LookPath("nvim")
	if err != nil {
		bin, err = exec.LookPath("vim")
		if err != nil {
			t.Skip("neither nvim nor vim available")
		}
	}

	dir := t.TempDir()
	canonical, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	dir = canonical
	dataDir := t.TempDir()

	configPath := filepath.Join(dataDir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, fmt.Appendf(nil, `
editor:
  mode: exo
  exo:
    command: %s "+call cursor({line}, {col})" {file}
    goto: "<esc>:{line}<enter>{col}|"
    quit: "<esc>:q!<enter>"
command:
  key: "<c-\\\\>"
`, bin), 0o666))

	relFile := "exo.txt"
	filePath := filepath.Join(dir, relFile)
	require.NoError(t, os.WriteFile(filePath, []byte("hello\n"), 0o644))

	mu := new(sync.Mutex)
	scheduleNextTick := func(fn func()) bool {
		go func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		}()
		return true
	}

	var rootRef atomic.Pointer[tui.Handler]
	publish := func(ev term.Event) bool {
		if ev.Type != term.EventNone {
			return true
		}
		rp := rootRef.Load()
		if rp == nil {
			return true
		}
		r := *rp
		scheduleNextTick(func() {
			r.Handle(ev)
		})
		return true
	}

	i, err := New(dir, configPath, dataDir, newTestStorage(t, dataDir),
		WithLocker(mu),
		WithScheduleNextTick(scheduleNextTick),
		WithBell(func() {}),
		WithStreamingOpen(true),
		WithPublishEvent(publish),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })

	root := i.Ready()
	rootRef.Store(&root)
	mu.Lock()
	root.Resize(20, 8)
	mu.Unlock()
	i.WaitWorkspaces()

	sendKeys := func(seq string) {
		t.Helper()
		keys, err := term.ParseKeys(seq)
		require.NoError(t, err)
		for _, k := range keys {
			ev := term.Event{
				Type: term.EventKey, Ch: k.Ch, Mod: k.Mod, Key: k.Key,
			}
			switch {
			case k.Key == term.KeyEsc:
				ev.Raw = []byte{0x1b}
			case k.Key == term.KeyEnter:
				ev.Raw = []byte{0x0d}
			case k.Key == term.KeySpace:
				ev.Raw = []byte{' '}
			case k.Mod == term.ModCtrl && k.Ch == '\\':
				ev.Raw = []byte{0x1c}
			case k.Ch != 0:
				ev.Raw = []byte(string(k.Ch))
			}
			mu.Lock()
			root.Handle(ev)
			mu.Unlock()
			i.WaitInflight()
		}
	}

	tabCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(i.workspaceHandler.focusEx().comp.Tabs())
	}

	sendKeys(`<c-\\>edit<space>` + relFile + `<enter>`)

	require.Eventually(t, func() bool {
		return tabCount() > 0
	}, 10*time.Second, 100*time.Millisecond,
		"exo file tab must be open before quitting the editor")

	// Wait for the streaming-open swap to land so the tab's
	// handler is the exo editor rather than the deferHandler /
	// streamload pair. Without this gate the publish dispatch
	// below can race text.(*Component).openFileTabStreaming's
	// deferred sh.Close() with streamload.Handle (RUNE-205).
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		tabs := i.workspaceHandler.focusEx().comp.Tabs()
		if len(tabs) == 0 {
			return false
		}
		_, streaming := tabs[0].Handler().(interface {
			Swap(text.Handler)
		})
		return !streaming
	}, 10*time.Second, 50*time.Millisecond,
		"streaming-open swap must complete before quitting the editor")

	// Give the editor time to finish startup so `:q` is interpreted
	// in normal mode rather than swallowed by an init-time prompt.
	time.Sleep(1500 * time.Millisecond)

	sendKeys(`<esc>:q<enter>`)

	require.Eventually(t, func() bool {
		return tabCount() == 0
	}, 10*time.Second, 100*time.Millisecond,
		"exo tab must auto-close after :q<enter> exits %s; if "+
			"this assertion fails the vte exit publish -> "+
			"root.Handle -> Tab.Handle -> RemoveTab chain is "+
			"broken", bin)

	handlertest.RunHandlerSequence(t, &lockedHandler{Handler: root, mu: mu},
		20, 8, []handlertest.SequenceTestCase{{
			InputSequence: "",
			Expected: `┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		}})
}

// lockedHandler serializes Handle/Draw/Resize/Cursor on the shared
// IDE locker so handlertest.RunHandlerSequence does not race
// scheduled callbacks that the test scheduler runs under the same
// mutex.
type lockedHandler struct {
	tui.Handler
	mu sync.Locker
}

func (h *lockedHandler) Handle(ev term.Event) (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Handle(ev)
}

func (h *lockedHandler) Draw(w term.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Draw(w)
}

func (h *lockedHandler) Resize(width, height int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Handler.Resize(width, height)
}

func (h *lockedHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Handler.Cursor()
}

// minimalStarTutorial is a self-contained Starlark tutorial that
// renders a single floating window. It is enough for the runner to
// install an overlay once dispatched.
const minimalStarTutorial = `
def run():
    floating_window(title="welcome", text="hello")
tutorial(entry=run)
`

// TestIDEStartingTutorialDispatchesOnReady verifies that
// WithStartingTutorial schedules a `:tutorial start <name>` dispatch on
// the event loop once the IDE is ready, and that an unknown name is a
// no-op.
func TestIDEStartingTutorialDispatchesOnReady(t *testing.T) {
	cases := []struct {
		name           string
		starting       string
		withInitShader bool
		wantScheduled  bool
		wantActive     string
	}{
		{
			name:          "known tutorial runs",
			starting:      "basics",
			wantScheduled: true,
			wantActive:    "basics",
		},
		{
			name:           "known tutorial deferred until init shader finishes",
			starting:       "basics",
			withInitShader: true,
			wantScheduled:  true,
			wantActive:     "basics",
		},
		{
			name:          "unknown tutorial is a no-op",
			starting:      "missing",
			wantScheduled: false,
		},
		{
			name:          "empty starting tutorial is a no-op",
			starting:      "",
			wantScheduled: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configFile, _ := makeTestFiles(t)
			dataDir := t.TempDir()

			mu := new(sync.Mutex)
			var scheduled []func()
			scheduleNextTick := func(fn func()) bool {
				scheduled = append(scheduled, fn)
				return true
			}

			opts := []Option{
				WithPublishEvent(nopPublishEvent),
				WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
				WithLocker(mu),
				WithScheduleNextTick(scheduleNextTick),
				WithStarlarkTutorial("basics", minimalStarTutorial),
			}
			if tc.starting != "" {
				opts = append(opts, WithStartingTutorial(tc.starting))
			}
			if tc.withInitShader {
				opts = append(opts, WithInitShader(
					func(_ term.Attributes, _ component.FrameCharSet) shader.Shader {
						return new(mockShader)
					},
					30, 1*time.Second,
				))
			}

			i, err := New("", configFile.Name(), dataDir,
				newTestStorage(t, dataDir), opts...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = i.Close() })

			// Capture the deferred init-shader timer so the test can
			// fire it deterministically instead of waiting in real time.
			var afterDuration time.Duration
			var afterCb func()
			i.options.afterFunc = func(d time.Duration, fn func()) *time.Timer {
				afterDuration = d
				afterCb = fn
				return nil
			}

			root := i.Ready()
			mu.Lock()
			root.Resize(80, 24)
			mu.Unlock()
			i.WaitWorkspaces()

			if !tc.wantScheduled {
				// With no valid tutorial the IDE lands on the home
				// workspace, so maybeOpenHomePrompt now schedules the
				// home prompt instead. Running it must not start a
				// tutorial.
				for _, fn := range scheduled {
					mu.Lock()
					fn()
					mu.Unlock()
				}
				assert.Nil(t, i.tutorial.overlay,
					"no tutorial overlay should be active")
				return
			}

			if tc.withInitShader {
				assert.Empty(t, scheduled,
					"tutorial dispatch must be deferred until the init shader finishes")
				require.NotNil(t, afterCb,
					"a deferred timer should be registered for the init shader")
				assert.Equal(t, 1*time.Second+tutorialInitShaderBuffer, afterDuration,
					"tutorial delay must be the init shader duration plus the buffer")
				afterCb()
			}

			require.Len(t, scheduled, 1,
				"exactly one tutorial dispatch should be scheduled")
			mu.Lock()
			scheduled[0]()
			mu.Unlock()

			assert.NotNil(t, i.tutorial.overlay,
				"tutorial overlay should be active after dispatch")
			assert.Equal(t, tc.wantActive, i.tutorial.activeName)
		})
	}
}

// TestIDEHomePromptOpensOnReady verifies that landing on the home
// workspace with no first-run tutorial pre-opens the command prompt with
// the workspaceopen command, and that the prompt is not pre-opened when a
// tutorial is configured or a real workspace is focused.
func TestIDEHomePromptOpensOnReady(t *testing.T) {
	t.Run("home workspace opens prompt", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		dataDir := t.TempDir()

		mu := new(sync.Mutex)
		scheduleNextTick, drain := newTestScheduler(mu)

		i, err := New("", configFile.Name(), dataDir,
			newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(mu),
			WithScheduleNextTick(scheduleNextTick),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = i.Close() })

		root := i.Ready()
		mu.Lock()
		root.Resize(80, 24)
		mu.Unlock()
		i.WaitWorkspaces()
		drain()

		require.True(t, i.workspaceHandler.focusEx().home,
			"expected the home workspace to be focused")
		mu.Lock()
		cmd := i.workspaceHandler.focusEx().cmd
		mu.Unlock()
		assert.NotNil(t, cmd,
			"the command prompt should be open after the dispatch")
	})

	t.Run("user-opened prompt cancels the pre-open", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		dataDir := t.TempDir()

		mu := new(sync.Mutex)
		scheduleNextTick, drain := newTestScheduler(mu)

		i, err := New("", configFile.Name(), dataDir,
			newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(mu),
			WithScheduleNextTick(scheduleNextTick),
			WithInitShader(
				func(_ term.Attributes, _ component.FrameCharSet) shader.Shader {
					return new(mockShader)
				},
				30, 1*time.Second,
			),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = i.Close() })

		// Capture the deferred init-shader timer so the test can fire it
		// after the user has opened their own prompt.
		var afterCb func()
		i.options.afterFunc = func(_ time.Duration, fn func()) *time.Timer {
			afterCb = fn
			return nil
		}

		root := i.Ready()
		mu.Lock()
		root.Resize(80, 24)
		mu.Unlock()
		i.WaitWorkspaces()
		drain()

		require.True(t, i.workspaceHandler.focusEx().home,
			"expected the home workspace to be focused")
		require.NotNil(t, afterCb,
			"the pre-open must be deferred behind the init shader")

		// The user opens the command prompt themselves before the
		// deferred pre-open fires.
		mu.Lock()
		i.workspaceHandler.focusEx().openCommandPrompt()
		userCmd := i.workspaceHandler.focusEx().cmd
		mu.Unlock()
		require.NotNil(t, userCmd,
			"the user-opened prompt must be active")

		// Fire the deferred timer and drain the scheduled dispatch.
		afterCb()
		drain()

		mu.Lock()
		cmd := i.workspaceHandler.focusEx().cmd
		mu.Unlock()
		assert.Same(t, userCmd, cmd,
			"the pre-open must not replace the prompt the user opened")
	})

	t.Run("starting tutorial skips prompt", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		dataDir := t.TempDir()

		mu := new(sync.Mutex)
		scheduleNextTick, drain := newTestScheduler(mu)

		i, err := New("", configFile.Name(), dataDir,
			newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(mu),
			WithScheduleNextTick(scheduleNextTick),
			WithStarlarkTutorial("basics", minimalStarTutorial),
			WithStartingTutorial("basics"),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = i.Close() })

		root := i.Ready()
		mu.Lock()
		root.Resize(80, 24)
		mu.Unlock()
		i.WaitWorkspaces()
		drain()

		mu.Lock()
		overlay := i.tutorial.overlay
		cmd := i.workspaceHandler.focusEx().cmd
		mu.Unlock()
		assert.NotNil(t, overlay,
			"the scheduled dispatch should start the tutorial, not the prompt")
		assert.Nil(t, cmd,
			"the home prompt must not be pre-opened when a tutorial runs")
	})

	t.Run("real workspace skips prompt", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		dataDir := t.TempDir()
		repo := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(repo, "seed.txt"), nil, 0666))

		mu := new(sync.Mutex)
		scheduleNextTick, drain := newTestScheduler(mu)

		i, err := New(repo, configFile.Name(), dataDir,
			newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(mu),
			WithScheduleNextTick(scheduleNextTick),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = i.Close() })

		root := i.Ready()
		mu.Lock()
		root.Resize(80, 24)
		mu.Unlock()
		i.WaitWorkspaces()
		drain()

		mu.Lock()
		isHome := i.workspaceHandler.focusEx().home
		cmd := i.workspaceHandler.focusEx().cmd
		mu.Unlock()
		require.False(t, isHome,
			"expected a real workspace to be focused, not home")
		assert.Nil(t, cmd,
			"the home prompt must not be pre-opened off the home workspace")
	})

	t.Run("WithoutHomePrompt skips prompt", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		dataDir := t.TempDir()

		mu := new(sync.Mutex)
		scheduleNextTick, drain := newTestScheduler(mu)

		i, err := New("", configFile.Name(), dataDir,
			newTestStorage(t, dataDir),
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(mu),
			WithScheduleNextTick(scheduleNextTick),
			WithoutHomePrompt(),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = i.Close() })

		root := i.Ready()
		mu.Lock()
		root.Resize(80, 24)
		mu.Unlock()
		i.WaitWorkspaces()
		drain()

		mu.Lock()
		isHome := i.workspaceHandler.focusEx().home
		cmd := i.workspaceHandler.focusEx().cmd
		mu.Unlock()
		require.True(t, isHome,
			"expected the home workspace to be focused")
		assert.Nil(t, cmd,
			"WithoutHomePrompt must suppress the home prompt")
	})
}

// TestCloseDoesNotCloseBorrowedStorage verifies that closing an IDE does
// not propagate Close to the storage service it borrowed from the caller.
//
// The storage handle is owned by the embedder (cmd/rune's bootstrap handler
// and main, which create it via localstorage.New and close it once at
// shutdown). It is shared: the bootstrap flow builds a pre-config IDE and a
// configured IDE over the same storage, and the configured IDE's LLM router
// keeps reading aliases from it. If IDE.Close closes the borrowed storage,
// closing the pre-config IDE during the bootstrap swap tears the storage
// down underneath the live configured IDE, so the next read fails with
// "firstmover: Partition on closed Service" (observed on fresh installs as
// the rune-agent extension failing to start after :workspacereload).
func TestCloseDoesNotCloseBorrowedStorage(t *testing.T) {
	t.Parallel()
	_, config := makeTestFiles(t)
	dataDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dataDir) })

	store := &closeCountingService{Service: newTestStorage(t, dataDir)}
	t.Cleanup(func() { _ = store.Service.Close() })

	i, err := New("", config.Name(), dataDir, store,
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
		WithLocker(new(sync.Mutex)))
	require.NoError(t, err)
	_ = i.Ready()
	i.WaitWorkspaces()

	require.NoError(t, i.Close())

	assert.Equal(t, int32(0), store.closeCount.Load(),
		"IDE.Close must not close the borrowed shared storage")

	// The borrowed storage must remain usable after the IDE is closed:
	// a partition must not error with "Partition on closed Service".
	_, perr := store.Partition("after-close")
	require.NoError(t, perr,
		"borrowed storage must stay usable after IDE.Close")
	require.NoError(t, store.Set(context.Background(), "probe",
		map[string]any{"k": "v"}))
}

// closeCountingService wraps a storageapi.Service and counts Close calls so
// the test can assert that a borrowed service is not closed by the IDE.
type closeCountingService struct {
	storageapi.Service
	closeCount atomic.Int32
}

func (s *closeCountingService) Close() error {
	s.closeCount.Add(1)
	return s.Service.Close()
}

// TestSharedStorageSurvivesPreIDEClose reproduces the fresh-install bootstrap
// swap: cmd/rune builds a pre-config IDE and a configured IDE over the same
// borrowed storage, then closes the pre-config IDE. Closing the first IDE must
// not tear down the storage that the second IDE still uses, otherwise the
// configured IDE's next storage read fails with
// "firstmover: Partition on closed Service".
func TestSharedStorageSurvivesPreIDEClose(t *testing.T) {
	t.Parallel()
	_, config := makeTestFiles(t)
	dataDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dataDir) })

	shared := newTestStorage(t, dataDir)
	t.Cleanup(func() { _ = shared.Close() })

	opts := []Option{
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
		WithLocker(new(sync.Mutex)),
	}

	preIDE, err := New("", config.Name(), dataDir, shared, opts...)
	require.NoError(t, err)
	_ = preIDE.Ready()
	preIDE.WaitWorkspaces()

	configuredIDE, err := New("", config.Name(), dataDir, shared, opts...)
	require.NoError(t, err)
	_ = configuredIDE.Ready()
	configuredIDE.WaitWorkspaces()
	t.Cleanup(func() { _ = configuredIDE.Close() })

	require.NoError(t, preIDE.Close())

	// The configured IDE (and the caller) must still be able to partition
	// the shared storage after the pre-config IDE has been closed. Partition
	// is the operation that fails with "firstmover: Partition on closed
	// Service" when the shared root has been torn down; it is the path the
	// host's storagerpc bridge and the LLM router's alias store exercise.
	_, perr := shared.Partition("after-pre-ide-close")
	require.NoError(t, perr,
		"shared storage must stay partitionable after pre-config IDE.Close")
}

// TestIDEOpenDoesNotReadProtectedDirs asserts that opening the IDE on the
// user's home never reads into the macOS TCC-protected directories
// (~/Library, ~/Documents, ~/Desktop, ~/Downloads); any such read would
// trigger a system permission prompt.
func TestIDEOpenDoesNotReadProtectedDirs(t *testing.T) {
	usr, err := user.Current()
	require.NoError(t, err)
	if usr.HomeDir == "" {
		t.Skip("no home directory for current user")
	}
	home := filepath.Clean(usr.HomeDir)

	forbidden := []string{
		filepath.Join(home, "Library"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Downloads"),
	}

	tracker := &readTracker{home: home, forbidden: forbidden}
	const scheme = "trackhome"
	homeURI := scheme + "://" + home

	configFile, _ := makeTestFiles(t)
	dataDir := t.TempDir()

	var mu sync.Mutex
	i, err := New(homeURI, configFile.Name(), dataDir, newTestStorage(t, dataDir),
		WithLocker(&mu),
		WithScheduleNextTick(func(fn func()) bool {
			go func() {
				mu.Lock()
				defer mu.Unlock()
				fn()
			}()
			return true
		}),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
		WithScheme(scheme, tracker.newScheme),
	)
	require.NoError(t, err)

	_ = i.Ready()
	i.WaitWorkspaces()

	i.workspaceHandler.mu.Lock()
	var cwd workspace.Workspace
	for _, wh := range i.workspaceHandler.workspaces {
		if wh != nil && wh.cwd != nil {
			cwd = wh.cwd
			break
		}
	}
	i.workspaceHandler.mu.Unlock()
	require.NotNil(t, cwd, "cwd workspace must be installed")

	// LoadGitignore is the workspace-open tree walk run by the FS
	// monitor and task manager; drive it directly so the assertion is
	// deterministic rather than racing those detached goroutines.
	tracker.reset()
	_, err = vctrl.LoadGitignore(cwd)
	require.NoError(t, err)

	require.NoError(t, i.closeResources())

	reads := tracker.snapshot()
	require.NotEmpty(t, reads, "LoadGitignore must walk the workspace root")
	for _, p := range reads {
		for _, root := range forbidden {
			assert.Falsef(t, p == root || strings.HasPrefix(p, root+string(filepath.Separator)),
				"opening the IDE must not read protected dir: %q", p)
		}
	}
}

type readTracker struct {
	home      string
	forbidden []string

	mu    sync.Mutex
	reads []string
}

func (rt *readTracker) newScheme(
	ctx context.Context, cfg config.Config, _ workspaceapi.URI,
) (schemeapi.Scheme, error) {
	uri, err := workspaceapi.ParseURI("file://" + rt.home)
	if err != nil {
		return nil, err
	}
	inner, err := workspace.NewFileScheme(ctx, cfg, uri)
	if err != nil {
		return nil, err
	}
	return &trackingScheme{Scheme: inner, rt: rt}, nil
}

func (rt *readTracker) record(abs string) {
	rt.mu.Lock()
	rt.reads = append(rt.reads, abs)
	rt.mu.Unlock()
}

func (rt *readTracker) snapshot() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make([]string, len(rt.reads))
	copy(out, rt.reads)
	return out
}

func (rt *readTracker) reset() {
	rt.mu.Lock()
	rt.reads = nil
	rt.mu.Unlock()
}

type trackingScheme struct {
	schemeapi.Scheme
	rt *readTracker
}

func (s *trackingScheme) abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(s.rt.home, p)
}

func (s *trackingScheme) ReadDir(path string) ([]fs.DirEntry, error) {
	abs := s.abs(path)
	s.rt.record(abs)
	if abs == s.rt.home {
		// Synthesize a home listing of only the protected dirs so the
		// walk stays bounded: a correct matcher prunes all four and
		// recurses nowhere, while any descent records a violation.
		entries := make([]fs.DirEntry, len(s.rt.forbidden))
		for i, root := range s.rt.forbidden {
			entries[i] = protectedDirEntry(filepath.Base(root))
		}
		return entries, nil
	}
	return s.Scheme.ReadDir(path)
}

func (s *trackingScheme) Open(filename string) (workspaceapi.File, error) {
	s.rt.record(s.abs(filename))
	return s.Scheme.Open(filename)
}

func (s *trackingScheme) OpenFile(
	filename string, flag int, perm fs.FileMode,
) (workspaceapi.File, error) {
	s.rt.record(s.abs(filename))
	return s.Scheme.OpenFile(filename, flag, perm)
}

func (s *trackingScheme) Stat(filename string) (fs.FileInfo, error) {
	s.rt.record(s.abs(filename))
	return s.Scheme.Stat(filename)
}

func (s *trackingScheme) Lstat(filename string) (fs.FileInfo, error) {
	s.rt.record(s.abs(filename))
	return s.Scheme.Lstat(filename)
}

func (s *trackingScheme) Watch(
	string, chan<- schemeapi.EventInfo, ...schemeapi.Event,
) (int, error) {
	return 0, nil
}

func (s *trackingScheme) StopWatch(int) error { return nil }

type protectedDirEntry string

func (e protectedDirEntry) Name() string               { return string(e) }
func (e protectedDirEntry) IsDir() bool                { return true }
func (e protectedDirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (e protectedDirEntry) Info() (fs.FileInfo, error) { return protectedDirInfo(e), nil }

type protectedDirInfo string

func (i protectedDirInfo) Name() string       { return string(i) }
func (i protectedDirInfo) Size() int64        { return 0 }
func (i protectedDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (i protectedDirInfo) ModTime() time.Time { return time.Time{} }
func (i protectedDirInfo) IsDir() bool        { return true }
func (i protectedDirInfo) Sys() any           { return nil }
