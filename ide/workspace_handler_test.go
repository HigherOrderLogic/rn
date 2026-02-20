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
	"os"
	"strconv"
	"sync/atomic"
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
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
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

	h := newSafeHandler(m)
	// jumptolocation is registered on a per-file basis, so the following tests
	// file-level subscriptions across a file's lifecycle.
	cases := []handlertest.SequenceTestCase{
		{":edit dakar.md>igentleman>driver>gentleman<:write>/gentleman>:jumptolocation next search>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│▐entleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next search>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐entleman                   │
│driver                      │
│gentleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":tabclose>:edit dakar.md>/gentleman>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│▐entleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next search>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐entleman                   │
│driver                      │
│gentleman                   │
│       searching 'gentleman'│
│                      NORMAL│
└────────────────────────────┘`},
		{":foldexpandall>", // this fails if not installed correctly
			`┌────────────────────────────┐
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

func TestSetTabNameWithAttrIntegration(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	uri1, err := workspaceapi.ParseURI("file://" + dir)
	require.NoError(t, err)
	cfg := defaultConfigWithWrap(false)
	var mu sync.Mutex
	cfg.scheduleNextTick = func(cb func()) bool {
		mu.Lock()
		defer mu.Unlock()
		cb()
		return true
	}
	var wg sync.WaitGroup
	cfg.ringBell = func() {
		wg.Done()
	}
	m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir,
		nopShutdownShaderConfig())
	require.NoError(t, m.addOrCreateWorkspace(uri1))
	m.tabAttentionNameSuffix = "*"

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{InputSequence: "<c-\\\\>terminalnewtab<enter>" +
			"<c-\\\\>tabrename<space>terminal<enter>", // avoid dynamic tty name
			Expected: `┌────────────────────────────┐
│$ terminal                  │
├────────────────────────────┤
│▐                           │
│                            │
│                            │
├────────────────────────────┤
│1 1  2 2                    │
└────────────────────────────┘`},
	}
	mu.Lock()
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)
	mu.Unlock()
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
└────────────────────────────┘`},
	}
	mu.Lock()
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)
	mu.Unlock()
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
	require.NoError(t, m.addOrCreateWorkspace(uri2))

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
└────────────────────────────┘`},
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

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{":edit dakar.md>igentleman<:locationcreate mylist>a>driver<:locationcreate mylist>a>gentleman<:write>:locationcreate mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│gentlema▐                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation previous mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation previous mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentlema▐                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":locationdelete mork>",
			`┌──────────────┌─────────────┐
│o dakar.md    │ there's no  │
├──────────────│ location    │
│gentlema▐     │ at the      │
│driver        │ given       │
│gentleman     │ cursor      │
│              │ position    │
│              │ for given   │
└──────────────│ location    │`},
		{":noticloseall>:locationdelete mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentlema▐                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│driver                      │
│gentlema▐                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":locationdeleteall mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{"k:locationtoggle mylist>j:locationtoggle mylist>:jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentl▐man                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentleman                   │
│drive▐                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":locationtoggle mylist>:jumptolocation next mylist>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│gentl▐man                   │
│driver                      │
│gentleman                   │
│                            │
│                      NORMAL│
└────────────────────────────┘`},
		{":jumptolocation next mylist>",
			`┌────────────────────────────┐
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

func TestOpenFilesinEmptyWorkspace(t *testing.T) {
	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), "",
		nopShutdownShaderConfig())

	h := newSafeHandler(m)
	cases := []handlertest.SequenceTestCase{
		{"<c-\\\\>edit<space>dakar.md<enter>",
			`┌────────────────────────────┐
│o dakar.md                  │
├────────────────────────────┤
│▐                           │
│                            │
│                      NORMAL│
├────────────────────────────┤
│1                           │
└────────────────────────────┘`},
		{"<c-\\\\>tabclose<enter>",
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
│                            │
│     workspaceWallpaper     │
│                            │
├────────────────────────────┤
│1                           │
└────────────────────────────┘`},
	}
	handlertest.RunHandlerSequence(t, h, 30, 9, cases)

	require.NoError(t, m.Close())
}

func TestWorkspaceConfig(t *testing.T) {
	mockConfig := map[string]interface{}{
		"1": "2",
		"2": map[string]interface{}{
			"dos": "2",
			"two": "2",
		},
	}
	uri, err := workspaceapi.ParseURI("memory:///tmp")
	require.NoError(t, err)

	t.Run("passes default scheme config to SchemeFunc", func(t *testing.T) {
		cfg := defaultCfg()
		manager := workspace.NewManager(cfg.workspace())
		workspaceConfig := cfg.cfg["workspace"].(map[string]interface{})
		workspaceConfig[workspace.MemoryScheme] = mockConfig

		passed := make(map[string]interface{})
		manager.RegisterScheme(workspace.MemoryScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (
				schemeapi.Scheme, error,
			) {
				cfg.Iterate(func(k string, v interface{}) {
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
		workspaceConfig := cfg.cfg["workspace"].(map[string]interface{})
		workspaceConfig[workspace.MemoryScheme] = mockConfig

		passed := make(map[string]interface{})
		manager.RegisterScheme(workspace.MemoryScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (
				schemeapi.Scheme, error,
			) {
				cfg.Iterate(func(k string, v interface{}) {
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
			cfg.scheduleNextTick = func(fn func()) bool {
				mu.Lock()
				defer mu.Unlock()
				fn()
				return true
			}
		}
		releaseManager := docrelease.NewManager(document.NewInMemoryService())
		shRunner := new(shaderRunner)
		shRunner.init(
			handler.Nop(), term.NopInterrupter(), term.Attributes{},
			nopShutdownShaderConfig(), component.FrameCharSetDefault())
		err = m.workspaceManagerHandler.init(&uri, homeURI, manager,
			notificationsConfig(), cfg, dir,
			func(term.Event) bool {
				return true
			}, runner, mu, nil,
			func() (ideConfig, error) { return cfg, errors.New("boom") },
			".sixrc", 0, 0, '1', 0, 0, true, nil, releaseManager, shRunner, 0, nil)
		require.NoError(t, err)
		defer m.Close()

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
		workspaceConfig := cfg.cfg["workspace"].(map[string]interface{})
		workspaceConfig[workspace.MemoryScheme] = mockConfig

		passed := make(map[string]interface{})
		manager.RegisterScheme(workspace.MemoryScheme,
			func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (
				schemeapi.Scheme, error,
			) {
				cfg.Iterate(func(k string, v interface{}) {
					passed[k] = v
				})
				return workspace.NewMemoryScheme(ctx, cfg, uri)
			})

		m := newTestWorkspaceManagerHandlerWithManager(t, manager, uri, cfg)
		assert.EqualValues(t, mockConfig, passed)

		m.reloadConfig = func() (ideConfig, error) {
			cfg := defaultCfg()
			cfg.cfg["workspace"].(map[string]interface{})["1"] = "!!!!"
			return cfg, nil
		}

		require.Equal(t, 0, m.height)
		require.Equal(t, 0, m.width)
		m.mu.Lock()
		require.NoError(t, m.commandReloadWorkspace())
		m.mu.Unlock()
		assert.EqualValues(t, mockConfig, passed)

		require.NoError(t, m.Close())
	})
}

func TestWorkspaceExtensions(t *testing.T) {
	t.Run("calls extension runner with user extensions", func(t *testing.T) {
		cfg := defaultCfg()
		cfg.cfg = map[string]interface{}{
			"command":           map[string]interface{}{},
			"show_manual_after": "1h",
			"extensions": map[string]interface{}{
				"git": map[string]interface{}{
					"path": "myPath",
					"config": map[string]interface{}{
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
		var uris [2]string
		var i atomic.Int32
		var wg sync.WaitGroup
		runner := FuncExtensionsRunner(
			func(_uri workspaceapi.URI,
				res map[extensionapi.Permission]extension.ResourceRegistrar,
				s string, noti browser.Notifications,
				exec schemeapi.Executor,
			) (extension.Runner, error) {
				defer wg.Done()
				i := i.Add(1)
				uris[i-1] = _uri.String()
				return fnRunner{fn: func(extensionID, path string, cfg config.Config) error {
					assert.Equal(t, "myPath", path)
					assert.Equal(t, "git", extensionID)
					assert.Equal(t, config.MapConfig(map[string]interface{}{"a": "b"}), cfg)
					return nil
				},
				}, nil
			})
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
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
		cfg.cfg = map[string]interface{}{}
		manager := workspace.NewManager(cfg.workspace())

		manager.RegisterScheme(workspace.MemoryScheme, workspace.NewMemoryScheme)

		uri, err := workspaceapi.ParseURI("memory:///tmp")
		require.NoError(t, err)

		extensions := map[string]Extension{
			"myID": {
				ID:         "myID",
				CmdAndArgs: "myPath2",
				Config:     config.MapConfig(map[string]interface{}{"a": "b"}),
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
				executor schemeapi.Executor) (extension.Runner, error) {
				defer wg.Done()
				i := i.Add(1)
				uris[i-1] = _uri.String()
				return fnRunner{fn: func(extensionID, path string, cfg config.Config) error {
					assert.Equal(t, "myID", extensionID)
					assert.Equal(t, "myPath2", path)
					assert.Equal(t, config.MapConfig(map[string]interface{}{"a": "b"}), cfg)
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
│  Do you want     │
│  to restore      │
│  the previous    │
│  session?        │
│                  │
└──────────────────┘`},
		// prompt resets cache (use file scheme to avoid needing
		// to use ':' to indicate memory scheme)
		{":woc>:wonew memory\\:///tmp2>:edit 12345aZZ>:w>:woc>:wonew  memory\\:///tmp2>y:noticloseall>",
			`┌──────────────────┐
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
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
└──────────────────┘`},
		{":wofo 4>:workspacenew memory\\:///tmp2>:edit memory\\:///tmp2/12>:workspacerelo>", // reloads non-primary workspace
			`┌──────────────────┐
│o 12              │
├──────────────────┤
│▐                 │
│                  │
│                  │
│            NORMAL│
├──────────────────┤
│1 1  4 4          │
└──────────────────┘`},
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
└──────────────────┘`},
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
│                  │
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
│                  │
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
│                  │
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
│                  │
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
				`┌──────────────────┐
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
│  There are       │
│  open files      │
│  with changes    │
│  pending to be   │
│  written. Are    │
│  you sure you    │
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
				`┌──────────────────┐
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
│  There are       │
│  open files      │
│  with changes    │
│  pending to be   │
│  written. Are    │
│  you sure you    │
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
					`┌──────────────────┐
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
│                  │
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
						`┌──────────────────┐
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
│  Do you want     │
│  to restore      │
│  the previous    │
│  session?        │
│                  │
└──────────────────┘`},
					{"y",
						`┌──────────────────┐
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
				cfg.cfg["workspace"].(map[string]interface{})["auto_restore"] = true
				m := newTestWorkspaceManagerHandlerWithDir(t, cfg, dir, nopShutdownShaderConfig())

				cases := []handlertest.SequenceTestCase{
					{"",
						`┌──────────────────┐
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
				cfg.cfg["workspace"].(map[string]interface{})["auto_restore"] = true
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
						`┌──────────────────┐
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
└──────────────────┘`},
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
└──────────────────┘`},
	}
	h := newSafeHandler(m)
	m.notifications.current().NotifyOnce(browserapi.LevelWarn, "6:14am")
	handlertest.TestHandlerSequence(t, h, 20, 10, cases)

	require.NoError(t, m.Close())
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
	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), "/tmp",
		nopShutdownShaderConfig())

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
	m := newTestWorkspaceManagerHandlerWithDir(t, defaultConfigWithWrap(false), "/tmp",
		nopShutdownShaderConfig())

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
│left                                  │
│right                                 │
│1                                     │
│2                                     │
│3                                     │
│4                                     │
│5                                     │
│6                                     │
│7                                     │
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
└──────────────────────────────────────┘`},
		{":womo 1>",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│          workspaceWallpaper          │
├──────────────────────────────────────┤
│1 1  2 2                              │
└──────────────────────────────────────┘`},
		{":wofo 1>:womo left>",
			`┌────────────────────────┌─────────────┐
│                        │ workspace   │
├────────────────────────│ is already  │
│          workspaceWallp│ at the      │
├────────────────────────│ first slot  ┤
│1 1  2 2                              │
└──────────────────────────────────────┘`},
		{":noticloseall>:womo 9>:womo right>",
			`┌────────────────────────┌─────────────┐
│                        │ workspace   │
├────────────────────────│ is already  │
│          workspaceWallp│ at the      │
├────────────────────────│ last slot   ┤
│2 2  9 9                              │
└──────────────────────────────────────┘`},
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
│wasup                       │
│wasep                       │
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
│wasup                       │
│wasep                       │
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
│wasup                       │
│wasep                       │
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
		for i := 0; i < n; i++ {
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
				`┌────────────────────────────┐
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
			{"iabc<:write>", // edit + flush
				`┌────────────────────────────┐
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
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
			{"iabc<:write>", // edit + flush
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
			{":wofo 8>:edit c>", // empty workspace
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
			{"iabc<:write>", // edit + flush
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
				`┌────────────────────────────┐
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
				`┌────────────────────────────┐
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
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
			{"iabc<:write>",
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
			{":wofo 8>:edit c>",
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
			{"iabc<:write>",
				`┌────────────────────────────┐
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
└────────────────────────────┘`},
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
┌────────────────────────────┐
│                            │
│                            │
│  workspace with URI        │
│  file:///tmp/TestWorkspac  │
│  eManagerCreateWorkspace   │
│  does not exist. Do you    │
│  want to create it?        │
│                            │
│                            │
│     Yes            No      │
│                            │
└────────────────────────────┘
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
└────────────────────────────┘`},
		{fmt.Sprintf(":workspacenew %s>", tempDir2), // not fully specified
			`┌────────────────────────────┐
│                            │
├────────────────────────────┤
┌────────────────────────────┐
│                            │
│                            │
│  workspace with URI        │
│  file:///tmp/TestWorkspac  │
│  eManagerCreateWorkspace2  │
│   does not exist. Do you   │
│  want to create it?        │
│                            │
│                            │
│     Yes            No      │
│                            │
└────────────────────────────┘
│                            │
├────────────────────────────┤
│1 1  2 2                    │
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
│1 1  2 2  3 3               │
└────────────────────────────┘`},
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
		shutdownShaderCfg, component.FrameCharSetDefault())

	mu := new(sync.Mutex)
	if cfg.scheduleNextTick == nil {
		cfg.scheduleNextTick = func(fn func()) bool {
			mu.Lock()
			defer mu.Unlock()
			fn()
			return true
		}
	}

	notiConfig := notificationsConfig()
	releaseManager := docrelease.NewManager(document.NewInMemoryService())
	err = m.workspaceManagerHandler.init(uri, homeURI, manager,
		notiConfig, cfg, dir, func(term.Event) bool {
			return true
		}, runner, mu, extensions,
		func() (ideConfig, error) { return cfg, nil },
		".sixrc", 0, 0, '1', 0, 0, true, onTabsClick, releaseManager, shRunner, 0, nil)

	require.NoError(t, err)
	return m
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
}

// mimic ide.IDE
func (t *testWorkspaceManagerHandler) Close() error {
	t.workspaceManagerHandler.mu.Lock()
	defer t.workspaceManagerHandler.mu.Unlock()

	return t.workspaceManagerHandler.Close()
}

func (t *testWorkspaceManagerHandler) Handle(ev term.Event) (bool, bool) {
	quit, handle := t.workspaceManagerHandler.Handle(ev)
	handler := t.workspaceManagerHandler.focusHandler()
	ex, ok := handler.(*ex)
	if !ok {
		ex = handler.(*workspaceHandler).ex
	}
	ex.Wait()
	return quit, handle
}

func defaultCfg() ideConfig {
	return ideConfig{cfg: map[string]interface{}{
		"clipboard": "memory",
		"command": map[string]interface{}{
			"show_manual_after": "1h",
			"key":               "<c-\\\\>", // see handlertest.TestHandlerIsolated
			"key_bindings": map[string]interface{}{
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
			"aliases": map[string]interface{}{
				"addBlaBla": "workspacenew memory:///blabla",
				"w":         "write!",
			},
		},
		"workspace": map[string]interface{}{
			"wallpaper":    "workspaceWallpaper",
			"auto_restore": false,
		},
		"browser": map[string]interface{}{
			"workspace_bar": "number",
			"window_manager": map[string]interface{}{
				"no_max_size": false,
			},
		},
		"notifications": map[string]interface{}{
			"progress_bar": false,
		},
	},
		scheduleNextTick: nil,
	}
}

func defaultConfigWithWrap(wrap bool) ideConfig {
	ret := defaultCfg()
	ret.cfg["editor"] = map[string]interface{}{
		"modal": map[string]interface{}{
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
