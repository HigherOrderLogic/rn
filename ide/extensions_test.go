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

package ide

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/docmarshal/docbson"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/term"
	"gopkg.in/yaml.v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/idepkg"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
	"unstable.build/go-tui/ide/pkgshell"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
)

func TestStartInstalledExtensions(t *testing.T) {
	t.Parallel()

	t.Run("starts on workspace runners but not home", func(t *testing.T) {
		t.Parallel()
		cfg := configWithExtension(t, "rune-agent", "rune-agent-bin")
		home := &recordingRunner{}
		wsRunner := &recordingRunner{}
		ws := &workspaceHandler{}
		ws.Extensions.Store(extension.Runner(wsRunner))

		h := &workspaceManagerHandler{
			mu:           new(sync.Mutex),
			homeRunner:   home,
			reloadConfig: func() (ideConfig, error) { return cfg, nil },
		}
		h.workspaces[0] = ws
		h.workspaceCount = 1

		started := h.startInstalledExtensions([]string{"rune-agent"})
		assert.True(t, started)

		assert.Empty(t, home.runCalls(),
			"home runner must not start user extensions")

		wsCalls := wsRunner.runCalls()
		require.Len(t, wsCalls, 1)
		assert.Equal(t, "rune-agent", wsCalls[0].id)
		assert.Equal(t, "rune-agent-bin", wsCalls[0].path)
	})

	t.Run("ignores ids absent from merged config", func(t *testing.T) {
		t.Parallel()
		cfg := configWithExtension(t, "rune-agent", "rune-agent-bin")
		home := &recordingRunner{}
		h := &workspaceManagerHandler{
			mu:           new(sync.Mutex),
			homeRunner:   home,
			reloadConfig: func() (ideConfig, error) { return cfg, nil },
		}

		started := h.startInstalledExtensions([]string{"other-ext"})
		assert.False(t, started)
		assert.Empty(t, home.runCalls())
	})

	t.Run("no ids is a no-op", func(t *testing.T) {
		t.Parallel()
		home := &recordingRunner{}
		h := &workspaceManagerHandler{
			mu:         new(sync.Mutex),
			homeRunner: home,
			reloadConfig: func() (ideConfig, error) {
				t.Fatal("reloadConfig must not be called with no ids")
				return ideConfig{}, nil
			},
		}
		assert.False(t, h.startInstalledExtensions(nil))
		assert.Empty(t, home.runCalls())
	})

	t.Run("already-running runner still counts as started", func(t *testing.T) {
		t.Parallel()
		cfg := configWithExtension(t, "rune-agent", "rune-agent-bin")
		wsRunner := &recordingRunner{
			runErr: fmt.Errorf("extension %q: %w", "rune-agent",
				extensionv2.ErrExtensionAlreadyRunning),
		}
		ws := &workspaceHandler{}
		ws.Extensions.Store(extension.Runner(wsRunner))
		h := &workspaceManagerHandler{
			mu:           new(sync.Mutex),
			reloadConfig: func() (ideConfig, error) { return cfg, nil },
		}
		h.workspaces[0] = ws
		h.workspaceCount = 1
		started := h.startInstalledExtensions([]string{"rune-agent"})
		assert.True(t, started)
		assert.Len(t, wsRunner.runCalls(), 1)
	})
}

func TestStartUserExtensionIdempotent(t *testing.T) {
	t.Parallel()
	p := configWithExtension(t, "rune-agent", "rune-agent-bin").
		extensions()["rune-agent"]
	path, pconfig := extensionRunArgs(p)

	t.Run("already running treated as success", func(t *testing.T) {
		t.Parallel()
		runner := &recordingRunner{
			runErr: fmt.Errorf("extension %q: %w", "rune-agent",
				extensionv2.ErrExtensionAlreadyRunning),
		}
		assert.NoError(t, startUserExtension(runner, "rune-agent", path, pconfig))
	})

	t.Run("other error propagates", func(t *testing.T) {
		t.Parallel()
		runner := &recordingRunner{runErr: errors.New("boom")}
		err := startUserExtension(runner, "rune-agent", path, pconfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})
}

func TestAfterPackageConfigMerge(t *testing.T) {
	t.Parallel()

	newHandler := func(t *testing.T, hook func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error)) (
		*workspaceManagerHandler, *recordingRunner,
	) {
		t.Helper()
		cfg := configWithExtension(t, "rune-agent", "rune-agent-bin")
		wsRunner := &recordingRunner{}
		ws := &workspaceHandler{}
		ws.Extensions.Store(extension.Runner(wsRunner))
		h := &workspaceManagerHandler{
			mu:                     new(sync.Mutex),
			homeRunner:             &recordingRunner{},
			reloadConfig:           func() (ideConfig, error) { return cfg, nil },
			packageConfigMergeHook: hook,
		}
		h.workspaces[0] = ws
		h.workspaceCount = 1
		return h, wsRunner
	}

	t.Run("gui.env only invokes stored hook, no extension start", func(t *testing.T) {
		t.Parallel()
		var hookCalls int
		h, wsRunner := newHandler(t, func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error) {
			hookCalls++
			return idepkg.ConfigMergeResult{LiveApplied: true}, nil
		})
		event := idepkg.ConfigMergeEvent{Diff: mustYAMLDoc(t, "gui:\n  env:\n    FOO: bar\n")}

		result, err := h.afterPackageConfigMerge(event)
		require.NoError(t, err)
		assert.True(t, result.LiveApplied)
		assert.Equal(t, 1, hookCalls)
		assert.Empty(t, wsRunner.runCalls())
	})

	t.Run("extensions only starts extension and still invokes hook", func(t *testing.T) {
		t.Parallel()
		var hookCalls int
		h, wsRunner := newHandler(t, func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error) {
			hookCalls++
			return idepkg.ConfigMergeResult{}, nil
		})
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t, "extensions:\n  rune-agent:\n    path: rune-agent-bin\n"),
		}

		result, err := h.afterPackageConfigMerge(event)
		require.NoError(t, err)
		assert.True(t, result.LiveApplied)
		assert.Equal(t, 1, hookCalls)
		require.Len(t, wsRunner.runCalls(), 1)
		assert.Equal(t, "rune-agent", wsRunner.runCalls()[0].id)
	})

	t.Run("both env and extensions live-apply", func(t *testing.T) {
		t.Parallel()
		h, wsRunner := newHandler(t, func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error) {
			return idepkg.ConfigMergeResult{LiveApplied: true}, nil
		})
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t,
				"gui:\n  env:\n    FOO: bar\nextensions:\n  rune-agent:\n    path: rune-agent-bin\n"),
		}
		result, err := h.afterPackageConfigMerge(event)
		require.NoError(t, err)
		assert.True(t, result.LiveApplied)
		require.Len(t, wsRunner.runCalls(), 1)
	})

	t.Run("stored hook error propagates", func(t *testing.T) {
		t.Parallel()
		h, _ := newHandler(t, func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error) {
			return idepkg.ConfigMergeResult{}, errors.New("boom")
		})
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t, "extensions:\n  rune-agent:\n    path: rune-agent-bin\n"),
		}
		result, err := h.afterPackageConfigMerge(event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		// extension start still flips LiveApplied even when the gui.env hook errors.
		assert.True(t, result.LiveApplied)
	})

	t.Run("nil stored hook still starts extension", func(t *testing.T) {
		t.Parallel()
		h, wsRunner := newHandler(t, nil)
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t, "extensions:\n  rune-agent:\n    path: rune-agent-bin\n"),
		}
		result, err := h.afterPackageConfigMerge(event)
		require.NoError(t, err)
		assert.True(t, result.LiveApplied)
		require.Len(t, wsRunner.runCalls(), 1)
	})

	t.Run("tutorials added invoke tutorialsInstalled and still run hook", func(t *testing.T) {
		t.Parallel()
		var hookCalls int
		h, _ := newHandler(t, func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error) {
			hookCalls++
			return idepkg.ConfigMergeResult{}, nil
		})
		var gotNames []string
		h.tutorialsInstalled = func(names []string) (bool, error) {
			gotNames = names
			return true, nil
		}
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t, "tutorials:\n  go-intro: go-intro.star\n"),
		}
		result, err := h.afterPackageConfigMerge(event)
		require.NoError(t, err)
		assert.True(t, result.LiveApplied)
		assert.Equal(t, 1, hookCalls)
		assert.Equal(t, []string{"go-intro"}, gotNames)
	})

	t.Run("no tutorials added does not invoke tutorialsInstalled", func(t *testing.T) {
		t.Parallel()
		h, _ := newHandler(t, nil)
		var called bool
		h.tutorialsInstalled = func([]string) (bool, error) {
			called = true
			return true, nil
		}
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t, "extensions:\n  rune-agent:\n    path: rune-agent-bin\n"),
		}
		_, err := h.afterPackageConfigMerge(event)
		require.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("tutorialsInstalled error is joined onto returned error", func(t *testing.T) {
		t.Parallel()
		h, _ := newHandler(t, nil)
		h.tutorialsInstalled = func([]string) (bool, error) {
			return false, errors.New("tutorial boom")
		}
		event := idepkg.ConfigMergeEvent{
			Diff: mustYAMLDoc(t, "tutorials:\n  go-intro: go-intro.star\n"),
		}
		_, err := h.afterPackageConfigMerge(event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tutorial boom")
	})
}

// TestPkgInstallStartsExtensionWithPackageEnv is the black-box regression for
// the env-before-spawn contract. It drives the real `pkg install <pkg>` shell
// command against a real package manager. The installed package's config.yaml
// adds both a gui.env block and an extensions entry. The IDE's gui.env hook
// (modeled on cmd/rune's guiEnvLiveApplyHook) applies the env via os.Setenv,
// and the IDE then starts the newly-configured extension on the home runner —
// a stubbed extension.Runner that records os.Getenv at the moment Run is
// invoked, exactly where a real extension subprocess would snapshot its
// environment. The recorded value must be the package's env, which only holds
// if the env was applied before the extension was started.
func TestPkgInstallStartsExtensionWithPackageEnv(t *testing.T) {
	const (
		envKey = "RUNE_TEST_PKG_EXT_ENV"
		envVal = "from-package"
		extID  = "pkgext"
		pkgID  = "envextpkg"
	)
	t.Setenv(envKey, "")

	dir := t.TempDir()
	extPath := filepath.Join(dir, "pkgext-bin")

	configFile, err := os.CreateTemp(dir, "config.*.yaml")
	require.NoError(t, err)
	_, err = configFile.WriteString("editor:\n  mode: modal\n")
	require.NoError(t, err)
	require.NoError(t, configFile.Close())
	configPath := configFile.Name()

	pkgs := idepkgtest.MakePackages(release.Package{Name: pkgID, Latest: "1"})
	bundles := idepkgtest.MakeBundles([]release.Bundle{{Package: pkgID, Version: "1"}})
	rm := idepkgtest.NewReleaseManager(pkgs, bundles)
	rm.SetMissProgressComplete(true)
	rm.SetTarball(pkgID, makeEnvExtPkgTarball(t, envKey, envVal, extID, extPath))

	recorder := &recordingRunner{captureEnv: envKey}

	// envHook mirrors cmd/rune's guiEnvLiveApplyHook: when the merged diff
	// touches gui.env it applies the package environment to the live process.
	envHook := func(event idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error) {
		if !event.TouchesPath("gui", "env") {
			return idepkg.ConfigMergeResult{}, nil
		}
		if err := os.Setenv(envKey, envVal); err != nil {
			return idepkg.ConfigMergeResult{}, err
		}
		return idepkg.ConfigMergeResult{LiveApplied: true}, nil
	}

	m := newPkgInstallExtHandler(t, configPath, rm,
		recordingExtensionsRunner{runner: recorder}, envHook)
	defer m.Close()

	// User extensions never start on the home workspace, so open a real
	// workspace for the freshly-installed extension to start on.
	wsURI, err := workspaceapi.CurrentUserHostURI(dir)
	require.NoError(t, err)
	require.NoError(t, m.addWorkspace(wsURI, true, false, -1))
	m.drainPendingWorkspaces()

	h := pkgshell.New(pkgshell.Config{
		Manager:       m.pkgmanager.pkg,
		UpdateChecker: m.pkgmanager.uc,
	})
	_, err = h.HandleCommand(context.Background(), repl.Command{
		Name: pkgshell.CommandName,
		Args: []string{"install", pkgID},
	}, repl.NopProgressWriter())
	require.NoError(t, err)

	// The merged config on disk must carry the package's extension entry.
	merged, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), extID,
		"install must merge the extensions entry into the user config")

	var calls []runCall
	require.Eventually(t, func() bool {
		calls = recorder.runCalls()
		return len(calls) > 0
	}, 10*time.Second, 20*time.Millisecond,
		"installing a package with an extensions entry must start the extension")

	var started *runCall
	for i := range calls {
		if calls[i].id == extID {
			started = &calls[i]
			break
		}
	}
	require.NotNil(t, started, "extension %q was not started, calls: %+v", extID, calls)
	assert.Equal(t, envVal, started.envVal,
		"extension must be started after the package gui.env was applied so it "+
			"inherits the package environment")
}

// TestPkgInstallRegistersTutorialLive is the black-box regression for the
// install-time tutorial registration contract, mirroring
// TestPkgInstallStartsExtensionWithPackageEnv for the extensions path. It
// drives a real `pkg install <pkg>` through a real IDE against a real package
// manager. The installed package's config.yaml adds a `tutorials:` entry whose
// .star file ships on disk. Before this feature a freshly-installed tutorial
// was invisible until restart because `tutorial start <name>` resolves only
// from the live tutorialRunner.tutorials map; the install must now register it
// live, so the tutorial is startable without a restart.
func TestPkgInstallRegistersTutorialLive(t *testing.T) {
	const (
		pkgID    = "tutpkg"
		tutName  = "go-intro"
		tutorial = "def run():\n    floating_window(title=\"hi\", text=\"hello\")\n" +
			"tutorial(entry=run)\n"
	)

	dir := t.TempDir()
	tutPath := filepath.Join(dir, "go-intro.star")
	require.NoError(t, os.WriteFile(tutPath, []byte(tutorial), 0o644))

	configPath := filepath.Join(dir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("editor:\n  mode: modal\n"), 0o644))

	wsFile := filepath.Join(dir, "seed.go")
	require.NoError(t, os.WriteFile(wsFile, nil, 0o644))

	pkgs := idepkgtest.MakePackages(release.Package{Name: pkgID, Latest: "1"})
	bundles := idepkgtest.MakeBundles([]release.Bundle{{Package: pkgID, Version: "1"}})
	rm := idepkgtest.NewReleaseManager(pkgs, bundles)
	rm.SetMissProgressComplete(true)
	rm.SetTarball(pkgID, makeTutorialPkgTarball(t, map[string]string{tutName: tutPath}))

	mu := new(sync.Mutex)
	i, err := New(dir, configPath, dir, newTestStorage(t, dir),
		WithReleaseManager(rm),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
		WithLocker(mu),
		// A synchronous scheduler runs onTutorialsInstalled's registration
		// and prompt closure inline on the install goroutine's scheduleFn
		// call, so the test observes the live registration deterministically.
		WithScheduleNextTick(func(fn func()) bool {
			fn()
			return true
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })
	_ = i.Ready()

	uri, err := workspaceapi.CurrentUserHostURI(wsFile)
	require.NoError(t, err)
	mu.Lock()
	require.NoError(t, i.Open(uri))
	i.root.Resize(80, 24)
	mu.Unlock()

	require.False(t, tutorialRegistered(i, mu, tutName),
		"tutorial must not be registered before install")

	h := pkgshell.New(pkgshell.Config{
		Manager:       i.workspaceHandler.pkgmanager.pkg,
		UpdateChecker: i.workspaceHandler.pkgmanager.uc,
	})
	mu.Lock()
	_, err = h.HandleCommand(context.Background(), repl.Command{
		Name: pkgshell.CommandName,
		Args: []string{"install", pkgID},
	}, repl.NopProgressWriter())
	mu.Unlock()
	require.NoError(t, err)

	merged, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), tutName,
		"install must merge the tutorials entry into the user config")

	require.Eventually(t, func() bool {
		return tutorialRegistered(i, mu, tutName)
	}, 10*time.Second, 20*time.Millisecond,
		"installing a package with a tutorials entry must register the tutorial live")
}

// TestPkgInstallMultipleTutorialsPromptsOnce asserts that when a single install
// adds more than one tutorial, every tutorial is registered live but the user
// is prompted exactly once (for the first tutorial). Prompting per tutorial
// would stack overlapping floating windows on top of each other.
func TestPkgInstallMultipleTutorialsPromptsOnce(t *testing.T) {
	const pkgID = "multitutpkg"
	tutNames := []string{"alpha-intro", "beta-intro"}
	tutorialSrc := "def run():\n    floating_window(title=\"hi\", text=\"hello\")\n" +
		"tutorial(entry=run)\n"

	dir := t.TempDir()
	tutorials := make(map[string]string, len(tutNames))
	for _, name := range tutNames {
		p := filepath.Join(dir, name+".star")
		require.NoError(t, os.WriteFile(p, []byte(tutorialSrc), 0o644))
		tutorials[name] = p
	}

	configPath := filepath.Join(dir, "rune.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("editor:\n  mode: modal\n"), 0o644))

	wsFile := filepath.Join(dir, "seed.go")
	require.NoError(t, os.WriteFile(wsFile, nil, 0o644))

	pkgs := idepkgtest.MakePackages(release.Package{Name: pkgID, Latest: "1"})
	bundles := idepkgtest.MakeBundles([]release.Bundle{{Package: pkgID, Version: "1"}})
	rm := idepkgtest.NewReleaseManager(pkgs, bundles)
	rm.SetMissProgressComplete(true)
	rm.SetTarball(pkgID, makeTutorialPkgTarball(t, tutorials))

	mu := new(sync.Mutex)
	i, err := New(dir, configPath, dir, newTestStorage(t, dir),
		WithReleaseManager(rm),
		WithPublishEvent(nopPublishEvent),
		WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
		WithLocker(mu),
		WithScheduleNextTick(func(fn func()) bool {
			fn()
			return true
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = i.Close() })
	_ = i.Ready()

	uri, err := workspaceapi.CurrentUserHostURI(wsFile)
	require.NoError(t, err)
	mu.Lock()
	require.NoError(t, i.Open(uri))
	i.root.Resize(80, 24)
	mu.Unlock()

	require.Equal(t, 0, countFloatingWindows(i, mu),
		"no prompt should be open before install")

	h := pkgshell.New(pkgshell.Config{
		Manager:       i.workspaceHandler.pkgmanager.pkg,
		UpdateChecker: i.workspaceHandler.pkgmanager.uc,
	})
	mu.Lock()
	_, err = h.HandleCommand(context.Background(), repl.Command{
		Name: pkgshell.CommandName,
		Args: []string{"install", pkgID},
	}, repl.NopProgressWriter())
	mu.Unlock()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return tutorialRegistered(i, mu, tutNames[0]) &&
			tutorialRegistered(i, mu, tutNames[1])
	}, 10*time.Second, 20*time.Millisecond,
		"every installed tutorial must be registered live")

	assert.Equal(t, 1, countFloatingWindows(i, mu),
		"installing multiple tutorials must open exactly one prompt, not one per tutorial")
}

func countFloatingWindows(i *IDE, mu sync.Locker) int {
	mu.Lock()
	defer mu.Unlock()
	n := 0
	i.Browser().IterateWindows(func(w browser.Window) {
		if w.IsFloating() {
			n++
		}
	})
	return n
}

func tutorialRegistered(i *IDE, mu sync.Locker, name string) bool {
	mu.Lock()
	defer mu.Unlock()
	return i.tutorial.has(name)
}

// makeTutorialPkgTarball builds a gzipped tar of a package whose config.yaml
// adds a tutorials entry for each name->path in tutorials. Installing it
// exercises the real config-merge and live tutorial-registration path.
func makeTutorialPkgTarball(t *testing.T, tutorials map[string]string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("tutorials:\n")
	for name, tutPath := range tutorials {
		b.WriteString("  " + name + ": " + tutPath + "\n")
	}
	configYAML := b.String()
	files := map[string]string{
		"config.yaml":    configYAML,
		"lib/readme.txt": "# lib placeholder\n",
	}
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gzw.Close())
	return buf.Bytes()
}

// recordingRunner is a fake extension.Runner that records the Run calls it
// receives. runErr, when set, is returned from every Run call.
type recordingRunner struct {
	mu     sync.Mutex
	calls  []runCall
	runErr error
	// captureEnv, when non-empty, snapshots os.Getenv(captureEnv) at the
	// moment Run is invoked, mirroring how a real extension subprocess
	// inherits os.Environ() at fork time.
	captureEnv string
}

type runCall struct {
	id     string
	path   string
	cfg    config.Config
	envVal string
}

func (r *recordingRunner) Run(id, path string, cfg config.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := runCall{id: id, path: path, cfg: cfg}
	if r.captureEnv != "" {
		call.envVal = os.Getenv(r.captureEnv)
	}
	r.calls = append(r.calls, call)
	return r.runErr
}

func (r *recordingRunner) Close() error { return nil }

func (r *recordingRunner) WaitReady(context.Context, string) error { return nil }

func (r *recordingRunner) runCalls() []runCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]runCall, len(r.calls))
	copy(out, r.calls)
	return out
}

var _ extension.Runner = (*recordingRunner)(nil)

// recordingExtensionsRunner is an ExtensionsRunner whose
// WorkspaceExtensionsRunner always hands back the same recordingRunner, so a
// test can observe the Run calls the IDE makes against the home/workspace
// runner it builds internally.
type recordingExtensionsRunner struct{ runner *recordingRunner }

func (r recordingExtensionsRunner) WorkspaceExtensionsRunner(
	_ workspaceapi.URI, _ map[extensionapi.Permission]extension.ResourceRegistrar,
	_ *ideauthorizer.Authorizer,
	_ string, _ browser.Notifications, _, _ schemeapi.Executor,
	_ extension.Grantor, _ text.Editor,
	_ ideauthorizer.PromptOpener, _ storageapi.Service,
	_ func(func()) bool,
) (extension.Runner, error) {
	return r.runner, nil
}

// configWithExtension returns an ideConfig declaring a single user extension
// with the given id and path.
func configWithExtension(t *testing.T, id, path string) ideConfig {
	t.Helper()
	return ideConfig{
		cfg: map[string]any{
			"extensions": map[string]any{
				id: map[string]any{"path": path},
			},
		},
		errors: map[string]error{},
	}
}

func mustYAMLDoc(t *testing.T, s string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(s), &doc))
	return &doc
}

// makeEnvExtPkgTarball builds a gzipped tar of a package whose config.yaml adds
// both a gui.env block (the env the extension needs) and an extensions entry
// pointing at extPath. Installing it exercises the real config-merge path.
func makeEnvExtPkgTarball(t *testing.T, envKey, envVal, extID, extPath string) []byte {
	t.Helper()
	configYAML := "gui:\n  env:\n    " + envKey + ": " + envVal + "\n" +
		"extensions:\n  " + extID + ":\n    path: " + extPath + "\n"
	files := map[string]string{
		"config.yaml":    configYAML,
		"lib/readme.txt": "# lib placeholder\n",
	}
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gzw.Close())
	return buf.Bytes()
}

// newPkgInstallExtHandler builds a testWorkspaceManagerHandler wired like the
// production IDE for the pkg-install path: a real release manager, a real
// on-disk config at configPath that reloadConfig re-reads (so the post-merge
// hook observes the merged extensions entry), the given ExtensionsRunner, and
// the given gui.env merge hook.
func newPkgInstallExtHandler(
	t *testing.T, configPath string, releaseManager release.Manager,
	runner ExtensionsRunner,
	mergeHook func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error),
) *testWorkspaceManagerHandler {
	t.Helper()

	cfg := defaultCfg()
	cfg.configPath = configPath
	cfg.scheduleNextTick = func(fn func()) bool {
		fn()
		return true
	}

	homeURI, err := workspaceapi.ParseURI("file:///tmp")
	require.NoError(t, err)

	m := new(testWorkspaceManagerHandler)
	m.workspaceManagerHandler = new(workspaceManagerHandler)
	m.packageConfigMergeHook = mergeHook
	m.tutorialsInstalled = func([]string) (bool, error) { return false, nil }

	mu := new(sync.Mutex)
	interrupter := term.NopInterrupter()
	shRunner := new(shaderRunner)
	shRunner.init(handler.Nop(), interrupter, term.Attributes{},
		nopShutdownShaderConfig(), loadingShaderConfig{}, openShaderConfig{},
		component.FrameCharSetDefault())

	dir := t.TempDir()

	manager := workspace.NewManager(cfg.workspace(), inlineSchedule)
	manager.RegisterScheme(workspace.FileScheme, workspace.NewFileScheme)

	notiCfg := notificationsConfig()
	storage := localstorage.New(context.Background(), dir, docbson.Marshaler())

	reload := func() (ideConfig, error) {
		ret := defaultCfg()
		ret.configPath = configPath
		ret.storage = cfg.storage
		if err := loadFileConfig(&ret, configPath); err != nil {
			return ret, err
		}
		return ret, nil
	}

	require.NoError(t, m.workspaceManagerHandler.init(nil, homeURI, manager,
		notiCfg, cfg, storage, dir,
		func(term.Event) bool { return true },
		runner, mu, nil, reload,
		".sixrc", 0, 0, '1', 0, 0, true, nil, releaseManager,
		shRunner, 0, nil, false, false, newCommandObserverRegistry()))
	return m
}
