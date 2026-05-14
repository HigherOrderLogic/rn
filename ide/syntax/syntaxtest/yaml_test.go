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

package syntaxtest

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

func newInstalledYAMLPkgManager(t *testing.T) *mockPkgManager {
	return newInstalledPkgManagerWithFiles(t,
		"yaml/tree-sitter.so",
		"yaml/highlights.scm",
		"yaml/indents.scm",
		"yaml/folds.scm",
		"yaml/locals.scm",
	)
}

func newYAMLTestCase(
	t *testing.T,
	pkgs syntax.PkgManager,
	width, height int,
	interrupt func(context.Context) error,
) (*sync.Mutex, *text.Component, func()) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)

	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)

	mu := new(sync.Mutex)
	scfg := syntax.DefaultConfig()
	scfg.ScheduleNextTick = func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		return true
	}
	scfg.ReparseOnErrors = false
	scfg.StrictErrors = true

	ed := vi.Editor(
		vi.WithIndents(text.IndentConfig{"yaml": text.IndentRuneSpace}),
		vi.WithStatusBarConfig(true, text.StatusBarConfig{
			Publisher:        texttest.NopEditor(),
			ScheduleNextTick: scfg.ScheduleNextTick,
		}),
	)
	w := workspace.NewSchemeWorkspace(uri, scheme)
	tcfg := text.DefaultConfig()
	tcfg.ScheduleNextTick = func(fn func()) bool { fn(); return true }
	tcfg.Syntax = scfg
	tcfg.PkgManager = pkgs
	tcfg.EventPublisher = func(ev term.Event) bool {
		interrupt(context.Background())
		return true
	}
	comp, err := text.NewComponent(ed, w, tcfg)
	require.NoError(t, err)

	comp.Browser().Resize(width, height)

	return mu, comp, func() {
		_ = comp.Close()
		_ = scheme.Close()
	}
}

func TestYAMLEnterUsesSpaceIndentIntegration(t *testing.T) {
	pkgs := newInstalledYAMLPkgManager(t)

	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}

	const width, height = 40, 12
	mu, comp, cleanup := newYAMLTestCase(t, pkgs, width, height, ready)
	defer cleanup()

	const yamlContent = "root:\n  child:"

	wg.Add(1)
	mu.Lock()
	_, h := newEditFileName(t, comp, yamlContent, "config.yaml")
	mu.Unlock()
	wg.Wait()

	keys, err := term.ParseKeys("GA<enter>x<esc>")
	require.NoError(t, err)
	for _, key := range keys {
		ev := term.Event{Type: term.EventKey, Ch: key.Ch, Key: key.Key, Mod: key.Mod}
		_, handled := h.Handle(ev)
		require.True(t, handled, "%s", ev.KeyComb().String())
	}

	actual := h.CellView().String()
	assert.Equal(t, "root:\n  child:\n    x", actual)
	assert.NotContains(t, actual, "\t")
}

// TestYAMLShiftRightUsesSpaceIndentIntegration verifies that vi `>>` on a
// YAML file uses space indentation, not a tab, matching the configured
// indent material for the yaml language.
func TestYAMLShiftRightUsesSpaceIndentIntegration(t *testing.T) {
	pkgs := newInstalledYAMLPkgManager(t)

	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}

	const width, height = 40, 12
	mu, comp, cleanup := newYAMLTestCase(t, pkgs, width, height, ready)
	defer cleanup()

	const yamlContent = "root:\nchild:"

	wg.Add(1)
	mu.Lock()
	_, h := newEditFileName(t, comp, yamlContent, "config.yaml")
	mu.Unlock()
	wg.Wait()

	// Move to last line and run `>>` to indent it one level.
	keys, err := term.ParseKeys(`G\>\>`)
	require.NoError(t, err)
	for _, key := range keys {
		ev := term.Event{Type: term.EventKey, Ch: key.Ch, Key: key.Key, Mod: key.Mod}
		_, handled := h.Handle(ev)
		require.True(t, handled, "%s", ev.KeyComb().String())
	}

	actual := h.CellView().String()
	assert.NotContains(t, actual, "\t", "YAML shift-right must not introduce tabs")
	assert.Contains(t, actual, "    child:")
}

// TestYAMLVisualShiftRightUsesSpaceIndentIntegration verifies that visual-mode
// `>` on a YAML selection uses space indentation.
func TestYAMLVisualShiftRightUsesSpaceIndentIntegration(t *testing.T) {
	pkgs := newInstalledYAMLPkgManager(t)

	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}

	const width, height = 40, 12
	mu, comp, cleanup := newYAMLTestCase(t, pkgs, width, height, ready)
	defer cleanup()

	const yamlContent = "root:\nchild:\nother:"

	wg.Add(1)
	mu.Lock()
	_, h := newEditFileName(t, comp, yamlContent, "config.yaml")
	mu.Unlock()
	wg.Wait()

	// Visual-line select last two lines and indent once.
	keys, err := term.ParseKeys(`jVj\>`)
	require.NoError(t, err)
	for _, key := range keys {
		ev := term.Event{Type: term.EventKey, Ch: key.Ch, Key: key.Key, Mod: key.Mod}
		_, handled := h.Handle(ev)
		require.True(t, handled, "%s", ev.KeyComb().String())
	}

	actual := h.CellView().String()
	assert.NotContains(t, actual, "\t", "YAML visual shift-right must not introduce tabs")
	assert.Contains(t, actual, "    child:")
	assert.Contains(t, actual, "    other:")
}
