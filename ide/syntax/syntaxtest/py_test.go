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
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

func newInstalledPythonPkgManager(t *testing.T) *mockPkgManager {
	return newInstalledPkgManagerWithFiles(t,
		"python/tree-sitter.so",
		"python/highlights.scm",
		"python/indents.scm",
		"python/folds.scm",
		"python/locals.scm",
	)
}

func newPythonTestCase(
	t *testing.T,
	pkgs syntax.PkgManager, width, height int,
	interrupt func(context.Context) error,
) (*sync.Mutex, *text.Component, func()) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)

	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)

	mu := new(sync.Mutex)
	cfg := syntax.DefaultConfig()
	cfg.ScheduleNextTick = func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		return true
	}
	cfg.ReparseOnErrors = false
	cfg.StrictErrors = true

	ed := vi.Editor(
		vi.WithIndents(text.IndentConfig{"python": text.IndentRuneSpace}),
		vi.WithStatusBarConfig(true, text.StatusBarConfig{
			Publisher:        texttest.NopEditor(),
			ScheduleNextTick: cfg.ScheduleNextTick,
		}),
	)
	w := workspace.NewSchemeWorkspace(uri, scheme, inlineSchedule)
	tcfg := text.DefaultConfig()
	tcfg.ScheduleNextTick = func(fn func()) bool { fn(); return true }
	tcfg.Syntax = cfg
	tcfg.PkgManager = pkgs
	tcfg.EventPublisher = func(ev term.Event) bool {
		interrupt(context.Background())
		return true
	}
	tcfg.FocusTabIconAttr = term.Attributes{Fg: term.ColorWhite}
	comp, err := text.NewComponent(ed, w, tcfg)
	require.NoError(t, err)

	comp.Browser().Resize(width, height)

	return mu, comp, func() {
		_ = comp.Close()
		_ = scheme.Close()
	}
}

func newPythonTree(t *testing.T) (*syntax.Tree, func()) {
	pkgs := newInstalledPythonPkgManager(t)
	_, tree, cleanup := newPythonTreeWithContent(t, pkgs, pyFileContent)
	return tree, cleanup
}

func newPythonTreeWithContent(
	t *testing.T, pkgs syntax.PkgManager, content string,
) (*text.Component, *syntax.Tree, func()) {
	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}
	const width, height = 30, 15
	mu, comp, cleanup := newPythonTestCase(t, pkgs, width, height, ready)

	wg.Add(1)
	mu.Lock()
	_, h := newEditFileName(t, mu, comp, content, strconv.Itoa(int(i.Add(1)))+".py")
	mu.Unlock()
	wg.Wait()

	cref, ok := h.(*text.StatusBar)
	require.True(t, ok)
	tree, ok := cref.Buffer().View().(*syntax.Tree)
	require.True(t, ok)

	return comp, tree, cleanup
}

func TestPythonTreeStateIntegration(t *testing.T) {
	tree, cleanup := newPythonTree(t)

	it := tree.State()
	defer it.Close()
	var actual syntax.State
	for {
		s, ok := it.Next(context.Background())
		require.True(t, ok)
		actual = s
		if s.Progress == 1 {
			break
		}
	}
	assert.Equal(t, syntax.State{
		LangID:     "python",
		Folds:      true,
		Indents:    true,
		Highlights: true,
		Progress:   1,
	}, actual)

	require.NoError(t, tree.Close())
	cleanup()
}

func TestPythonTreeQueryIntegration(t *testing.T) {
	t.Run("locals.scm definitions resolve to the expected captures", func(t *testing.T) {
		_, tree, cleanup := newPythonTreeWithContent(t, newInstalledPythonPkgManager(t), pyFileContent)

		it, err := tree.Query("locals.scm", "local.definition.function")
		require.NoError(t, err)

		matches, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Equal(t, []syntax.Match{
			{CaptureName: "local.definition.function", LineString: "def greet(name):", Line: 5},
			{CaptureName: "local.definition.function", LineString: "    def __init__(self, prefix):", Line: 13},
		}, matches)

		require.NoError(t, tree.Close())
		cleanup()
	})
}

func TestPythonTreeCommentCoverageIntegration(t *testing.T) {
	content := "x = 1\n# alpha\n# beta\ny = 2\n"
	_, tree, cleanup := newPythonTreeWithContent(t, newInstalledPythonPkgManager(t), content)

	ranges, ok := tree.CommentCoverage(term.Range{
		Start: term.Coordinates{Y: 1, X: 0},
		End:   term.Coordinates{Y: 2, X: len("# beta")},
	})
	require.True(t, ok)
	assert.Equal(t, []term.Range{
		{Start: term.Coordinates{Y: 1, X: 0}, End: term.Coordinates{Y: 1, X: len("# alpha")}},
		{Start: term.Coordinates{Y: 2, X: 0}, End: term.Coordinates{Y: 2, X: len("# beta")}},
	}, ranges)

	require.NoError(t, tree.Close())
	cleanup()
}

func TestPythonTreeFoldsIntegration(t *testing.T) {
	tree, cleanup := newPythonTree(t)

	it, ok := tree.Folds()
	require.True(t, ok)

	actual, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)

	expected := []term.Range{
		{Start: term.Coordinates{X: 0, Y: 2}, End: term.Coordinates{X: 35, Y: 2}},
		{Start: term.Coordinates{X: 0, Y: 5}, End: term.Coordinates{X: 18, Y: 9}},
		{Start: term.Coordinates{X: 0, Y: 0}, End: term.Coordinates{X: 9, Y: 0}},
		{Start: term.Coordinates{X: 0, Y: 2}, End: term.Coordinates{X: 35, Y: 2}},
		{Start: term.Coordinates{X: 9, Y: 5}, End: term.Coordinates{X: 15, Y: 5}},
		{Start: term.Coordinates{X: 14, Y: 6}, End: term.Coordinates{X: 23, Y: 6}},
		{Start: term.Coordinates{X: 4, Y: 7}, End: term.Coordinates{X: 25, Y: 8}},
		{Start: term.Coordinates{X: 18, Y: 7}, End: term.Coordinates{X: 21, Y: 7}},
		{Start: term.Coordinates{X: 13, Y: 8}, End: term.Coordinates{X: 25, Y: 8}},
		{Start: term.Coordinates{X: 0, Y: 12}, End: term.Coordinates{X: 28, Y: 14}},
		{Start: term.Coordinates{X: 4, Y: 13}, End: term.Coordinates{X: 28, Y: 14}},
		{Start: term.Coordinates{X: 16, Y: 13}, End: term.Coordinates{X: 30, Y: 13}},
	}
	assert.ElementsMatch(t, expected, actual)

	require.NoError(t, tree.Close())
	cleanup()
}

func TestPythonTreeHighlightsIntegration(t *testing.T) {
	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}
	const width, height = 30, 15
	pkgs := newInstalledPythonPkgManager(t)
	mu, comp, cleanup := newPythonTestCase(t, pkgs, width, height, ready)
	defer cleanup()
	w := newWriter(width, height)

	wg.Add(1)
	mu.Lock()
	newEditFileName(t, mu, comp, pyFileContent, "highlights.py")
	mu.Unlock()
	wg.Wait()

	cases := []handlertest.SingleTestCase{
		{term.Event{Ch: 'g', Type: term.EventKey}, `
┌###############─────────────┐
│# #############             │
├────────────────────────────┤
│    message = ######### + na│
│    ### i in #####(#):      │
│        #####(message, i)   │
│    ###### message          │
│                            │
│                            │
│##### Greeter:              │
│    ### __init__(self, prefi│
│        self.prefix = prefix│
│                            │
│                      NORMAL│
└────────────────────────────┘`},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)
}

func TestPythonTreeIndentsIntegration(t *testing.T) {
	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}
	const width, height = 30, 15
	pkgs := newInstalledPythonPkgManager(t)
	mu, comp, cleanup := newPythonTestCase(t, pkgs, width, height, ready)
	defer cleanup()
	w := newWriter(width, height)

	wg.Add(1)
	mu.Lock()
	_, h := newEditFileName(t, mu, comp, pyFileContent, "indents.py")
	mu.Unlock()
	wg.Wait()
	_ = h

	sequenceCases := []handlertest.SequenceTestCase{
		{"ggjjjjjo", `┌#############───────────────┐
│# ###########               │
├────────────────────────────┤
│###### os                   │
│                            │
│#### collections ###### Orde│
│                            │
│                            │
│### greet(name):            │
│    ▐                       │
│    message = ######### + na│
│    ### i in #####(#):      │
│        #####(message, i)   │
│                     #######│
└────────────────────────────┘`},
	}
	handlertest.TestHandlerSequenceWriter(t, w, comp.Browser(), width, height, sequenceCases)
}

func TestPythonTreeSelectionIntegration(t *testing.T) {
	_, tree, cleanup := newPythonTreeWithContent(t, newInstalledPythonPkgManager(t), pyFileContent)
	defer cleanup()
	defer func() { require.NoError(t, tree.Close()) }()

	t.Run("expand climbs from identifier to enclosing nodes", func(t *testing.T) {
		from := term.Range{Start: term.Coordinates{Y: 6, X: 4}, End: term.Coordinates{Y: 6, X: 4}}
		var got []term.Range
		cur := from
		for {
			next, ok := tree.SelectionExpand(cur)
			if !ok || next == cur {
				break
			}
			got = append(got, next)
			cur = next
		}
		assert.Equal(t, []term.Range{
			{Start: term.Coordinates{X: 4, Y: 6}, End: term.Coordinates{X: 11, Y: 6}},
			{Start: term.Coordinates{X: 4, Y: 6}, End: term.Coordinates{X: 30, Y: 6}},
			{Start: term.Coordinates{X: 4, Y: 6}, End: term.Coordinates{X: 18, Y: 9}},
			{Start: term.Coordinates{X: 0, Y: 5}, End: term.Coordinates{X: 18, Y: 9}},
			{Start: term.Coordinates{X: 0, Y: 0}, End: term.Coordinates{X: 0, Y: 15}},
		}, got)
	})
}

const pyFileContent = `import os

from collections import OrderedDict


def greet(name):
    message = "hello, " + name
    for i in range(3):
        print(message, i)
    return message


class Greeter:
    def __init__(self, prefix):
        self.prefix = prefix
`
