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

package extensionv2

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/pkgtrust"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/workspace"
)

// nopTrustVerifier is a TrustVerifier that trusts nothing; e2e tests exercise
// prompt flows for unverified extensions.
type nopTrustVerifier struct{}

func (nopTrustVerifier) VerifyExtensionEntrypoint(string) (string, bool) { return "", false }

func TestPluginPermissionPromptE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	runectl := findRunectl(t)
	var err error
	runectl, err = filepath.EvalSymlinks(runectl)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dataDir := t.TempDir()
	workspaceDir := filepath.Join("/tmp", fmt.Sprintf("rune-e2e-%d", os.Getpid()))
	_ = os.RemoveAll(workspaceDir)
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(workspaceDir) })
	uri, err := workspaceapi.ParseURI("file://" + workspaceDir)
	require.NoError(t, err)
	execScheme, err := workspace.NewFileScheme(ctx, config.MapConfig(map[string]any{}), uri)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, execScheme.Close()) })

	baseRunner, err := NewRunner(ctx, new(sync.Mutex), dataDir)
	require.NoError(t, err)

	cases := []struct {
		name           string
		cmd            workspaceapi.Cmd
		wantPrompts    int
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "direct runectl",
			cmd: workspaceapi.Cmd{
				Path: runectl,
				Args: []string{"wm", "focus"},
			},
			wantPrompts: 1,
			wantContains: []string{
				fmt.Sprintf("Program %s", runectl),
				"with args [wm focus]",
				"wants to **manage the window manager**.",
			},
			wantNotContain: []string{"running inside"},
		},
		{
			name: "shell runs runectl",
			cmd: workspaceapi.Cmd{
				Path: "/bin/sh",
				Args: []string{"-c", fmt.Sprintf("%q wm focus", runectl)},
			},
			wantPrompts: 1,
			wantContains: []string{
				fmt.Sprintf("Program %s", runectl),
				"with args [wm focus]",
				"running inside /bin/sh [-c",
				"wants to **manage the window manager**.",
			},
			wantNotContain: []string{"Program /bin/sh with args [-c"},
		},
		{
			name: "shell runs runectl twice",
			cmd: workspaceapi.Cmd{
				Path: "/bin/sh",
				Args: []string{"-c", fmt.Sprintf("%q wm focus && %q wm focus", runectl, runectl)},
			},
			wantPrompts: 2,
			wantContains: []string{
				fmt.Sprintf("Program %s", runectl),
				"with args [wm focus]",
				"running inside /bin/sh with args:\n\n```",
				"wants to **manage the window manager**.",
			},
			wantNotContain: []string{"Program /bin/sh with args [-c"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := newE2EPromptOpener(ideauthorizer.PromptOptionYes)
			storage := storagestub.NewInMemoryService()
			authorizer, err := ideauthorizer.NewAuthorizer(
				texttest.NopEditor(), prompt, storage,
				func(fn func()) bool {
					fn()
					return true
				},
				nil, pkgtrust.NewStore(t.TempDir(), nil), ideauthorizer.Config{},
			)
			require.NoError(t, err)
			runner, err := baseRunner.WorkspaceExtensionsRunner(
				uri,
				extension.BrowserResources(e2eBrowser{}, func(term.Event) bool { return true }),
				authorizer,
				nopTrustVerifier{},
				dataDir,
				dataDir,
				e2eBrowser{},
				execScheme,
				execScheme,
				extension.GrantAll(),
				texttest.NopEditor(),
				prompt,
				storage,
				func(fn func()) bool {
					fn()
					return true
				},
			)
			require.NoError(t, err)
			defer func() { require.NoError(t, runner.Close()) }()

			watcher := newE2EProcessWatcher()
			cmd := tc.cmd
			cmd.Watcher = watcher
			_, err = runner.(schemeapi.Executor).StartCommand(context.Background(), cmd)
			require.NoError(t, err)
			err = watcher.wait(10 * time.Second)
			require.NoError(t, err)

			message := prompt.message(t, tc.wantPrompts)
			for _, want := range tc.wantContains {
				assert.Contains(t, message, want)
			}
			for _, want := range tc.wantNotContain {
				assert.NotContains(t, message, want)
			}
		})
	}
}

type e2eProcessWatcher struct {
	done chan error
}

func newE2EProcessWatcher() *e2eProcessWatcher {
	return &e2eProcessWatcher{done: make(chan error, 1)}
}

func (w *e2eProcessWatcher) WatchProcess() chan error {
	return w.done
}

func (w *e2eProcessWatcher) wait(timeout time.Duration) error {
	select {
	case err := <-w.done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for command")
	}
}

type e2ePromptOpener struct {
	option string

	mu       sync.Mutex
	messages []string
}

func newE2EPromptOpener(option string) *e2ePromptOpener {
	return &e2ePromptOpener{option: option}
}

func (p *e2ePromptOpener) Prompt(
	message string, options []string,
	bindings []term.KeyComb,
	promptHandler handler.PromptHandler,
) browser.Window {
	p.mu.Lock()
	p.messages = append(p.messages, message)
	p.mu.Unlock()
	for i, opt := range options {
		if opt == p.option {
			promptHandler.OnSelect(i, opt)
			return browsertest.NopWindow()
		}
	}
	panic("test prompt option was not provided")
}

func (p *e2ePromptOpener) message(t *testing.T, wantPrompts int) string {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	require.Len(t, p.messages, wantPrompts)
	return p.messages[0]
}

type e2eBrowser struct{}

func (e2eBrowser) Focus() (browser.Window, error) { return browsertest.NopWindow(), nil }

func (e2eBrowser) SetFocus(browser.Window) (browser.Window, error) {
	return browsertest.NopWindow(), nil
}

func (e2eBrowser) Window(uint64) (browser.Window, bool) { return browsertest.NopWindow(), true }

func (e2eBrowser) IterateWindows(func(browser.Window)) {}

func (e2eBrowser) Split(
	browserapi.Orientation, browser.Window, browserapi.Handler,
) (browser.Window, error) {
	return browsertest.NopWindow(), nil
}

func (e2eBrowser) Floating(
	browser.Floating, browserapi.FloatingConfig,
) (browser.Window, error) {
	return browsertest.NopWindow(), nil
}

func (e2eBrowser) Bar(browserapi.BarConfig, tui.Handler) error { return nil }

func (e2eBrowser) SetTabName(workspaceapi.URI, string, term.Attributes) error { return nil }

func (e2eBrowser) Tab(
	workspaceapi.URI, rune, string, browserapi.Handler,
) (browserapi.Handler, error) {
	return browsertest.NewTestHandler(), nil
}

func (e2eBrowser) SetWindowContent(browser.Window, browserapi.Handler) error { return nil }

func (e2eBrowser) CloseWindow(browser.Window) error { return nil }

func (e2eBrowser) Open(workspaceapi.URI) (browserapi.Handler, error) {
	return browsertest.NewTestHandler(), nil
}

func (e2eBrowser) Resource(workspaceapi.URI) (browserapi.Handler, bool) {
	return browsertest.NewTestHandler(), true
}

func (e2eBrowser) PublishEvent(term.Event) error { return nil }

func (e2eBrowser) Close() error { return nil }

func (e2eBrowser) Notify(browserapi.NotificationLevel, string, ...any) (string, error) {
	return "", nil
}

func (e2eBrowser) NotifyOnce(browserapi.NotificationLevel, string, ...any) (string, error) {
	return "", nil
}

func (e2eBrowser) UpdateNotificationProgress(string, string, int64, int64) error {
	return nil
}

// findRunectl locates the runectl binary or skips the test
// when none is available on the system. Mirrors findGopls /
// findDlv used elsewhere in the repo: the e2e suite is a
// development-time integration test, not a hard CI gate, so
// missing tooling should skip rather than fail the run.
func findRunectl(t *testing.T) string {
	t.Helper()
	if bin, err := exec.LookPath("runectl"); err == nil {
		return bin
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("HOME"), ".rune", "bin", "runectl"),
		filepath.Join(os.Getenv("HOME"), "go", "bin", "runectl"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("runectl not found in PATH, ~/.rune/bin, or ~/go/bin; " +
		"build it via `make -C cmd/runectl build` to enable this test")
	return ""
}
