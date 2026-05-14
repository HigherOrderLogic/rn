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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

var ctx = context.Background()

func TestCommandsIntegration(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "")
	require.NoError(t, err)
	path := filepath.Join(dir, "fai.go")
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.WriteString(fileContent)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	uri, err := workspaceapi.CurrentUserHostURI(f.Name())
	require.NoError(t, err)

	var mu sync.Mutex
	var wg sync.WaitGroup
	scheduleNextTick := func(fn func()) bool {
		defer wg.Done()
		mu.Lock()
		defer mu.Unlock()
		fn()
		return true
	}

	mu.Lock()
	const width, height = 30, 10
	c, pkg := newTestComponentWithFile(t, dir, uri, scheduleNextTick)
	c.Resize(width, height)
	mu.Unlock()

	wg.Add(1)
	it, _, err := c.CompleteCommand(context.Background(), textapi.Command{
		Name:   "jumptoast",
		Args:   []string{},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.NoError(t, err)
	items, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.Equal(t, []string{"locals.scm"}, items)

	it, _, err = c.CompleteCommand(context.Background(), textapi.Command{
		Name:   "jumptoast",
		Args:   []string{"locals.scm", ""},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.NoError(t, err)
	items, err = iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"local.definition.function",
		"local.definition.var",
		"local.scope",
		"local.definition.namespace",
		"local.reference",
	}, items)

	it, _, err = c.CompleteCommand(context.Background(), textapi.Command{
		Name:   "jumptoast",
		Args:   []string{"locals.scm", "local.definition.namespace", ""},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.NoError(t, err)
	items, err = iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"package main",
	}, items)

	_, err = c.DispatchCommand(ctx, textapi.Command{
		Name:   "jumptoast",
		Args:   []string{"NONEXISTENT", "local.definition.namespace", "package main"},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.True(t, strings.Contains(err.Error(), "NONEXISTENT"))

	_, err = c.DispatchCommand(ctx, textapi.Command{
		Name:   "jumptoast",
		Args:   []string{"locals.scm", "NONEXISTENT", "package main"},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.EqualError(t, err, "1 error occurred: capture name 'NONEXISTENT' does not exist in query")

	_, err = c.DispatchCommand(ctx, textapi.Command{
		Name:   "jumptoast",
		Args:   []string{"locals.scm", "local.definition.namespace", "NONEXISTENT"},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.EqualError(t, err, "could not find matching node")

	// test that we actually jump
	tests := []comptest.TestCase{
		{Action: func() {
			handled, err := c.DispatchCommand(ctx, textapi.Command{
				Name:   "jumptoast",
				Args:   []string{"locals.scm", "local.definition.var", "const fileContent = \"package main\\n\" +"},
				Window: c.Browser().Focus(),
				URI:    uri,
			})
			require.NoError(t, err)
			assert.True(t, handled)
		}, Expected: `
┌━━━━━━━━────────────────────┐
│o fai.go                    │
├────────────────────────────┤
│    for i := 0; i < 10; i++ │
│        fmt.Println("%d", i)│
│    }                       │
│}                           │
│                            │
│const fileContent = "package│
└────────────────────────────┘`,
		},
	}

	w := term.NewStringWriter(width, height)
	comptest.TestComponent(t, component.Sync(&mu, c), w, tests)

	// test re-open re-register
	mu.Lock()
	wg.Add(1)
	c.Browser().RemoveWindowContent(c.Browser().Focus())
	pkg.ret = iterator.FromSlice([]string{ // re-hydrate files iterator
		"go/tree-sitter.so",
		"go/highlights.scm",
		"go/indents.scm",
		"go/folds.scm",
		"go/locals.scm",
	})
	_, err = c.OpenFileTab(uri, false)
	require.NoError(t, err)
	require.True(t, c.Browser().PreviousTab(c.Browser().Focus()))
	mu.Unlock()
	wg.Wait()

	it, _, err = c.CompleteCommand(context.Background(), textapi.Command{
		Name:   "jumptoast",
		Args:   []string{"locals.scm", "local.definition.namespace", ""},
		Window: c.Browser().Focus(),
		URI:    uri,
	})
	require.NoError(t, err)
	items, err = iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"package main",
	}, items)

	tests = []comptest.TestCase{
		{Action: func() {
		}, Expected: `
┌━━━━━━━━────────────────────┐
│o fai.go                    │
├────────────────────────────┤
│package main                │
│                            │
│import (                    │
│    "fmt"                   │
│                            │
│    "github.com/unstablebuil│
└────────────────────────────┘`,
		},
		{Action: func() {
			handled, err := c.DispatchCommand(ctx, textapi.Command{
				Name:   "jumptoast",
				Args:   []string{"locals.scm", "local.definition.var", "const fileContent = \"package main\\n\" +"},
				Window: c.Browser().Focus(),
				URI:    uri,
			})
			require.NoError(t, err)
			require.True(t, handled)
		}, Expected: `
┌━━━━━━━━────────────────────┐
│o fai.go                    │
├────────────────────────────┤
│    for i := 0; i < 10; i++ │
│        fmt.Println("%d", i)│
│    }                       │
│}                           │
│                            │
│const fileContent = "package│
└────────────────────────────┘`,
		},
	}

	comptest.TestComponent(t, component.Sync(&mu, c), w, tests)
	require.NoError(t, c.Close())
}

func newTestComponentWithFile(
	t *testing.T, dir string, filename workspaceapi.URI,
	scheduleNextTick func(func()) bool,
) (*text.Component, *mockPkgManager) {
	cfg := text.DefaultConfig()
	cfg.ScheduleNextTick = func(fn func()) bool { fn(); return true }
	pkg := newInstalledPkgManager(t)
	cfg.PkgManager = pkg
	cfg.NoMaxSize = false
	cfg.Syntax.ScheduleNextTick = scheduleNextTick
	uri, err := workspaceapi.CurrentUserHostURI(dir)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	loader := workspace.NewSchemeWorkspace(uri, scheme)
	c, err := text.NewComponent(vi.Editor(), loader, cfg)
	require.NoError(t, err)
	_, err = c.OpenFileTab(filename, false)
	require.NoError(t, err)
	c.Browser().PreviousTab(c.Browser().Focus())
	return c, pkg
}
