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

package syntax_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"go.uber.org/goleak"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

func TestDelegateIntegration(t *testing.T) {
	var wg sync.WaitGroup
	ready := func(context.Context) error {
		wg.Done()
		return nil
	}
	const width, height = 30, 15
	pkgs := newInstalledPkgManager(t)
	mu, comp, cleanup := installDelegate(t, pkgs, width, height, ready)
	w := newWriter(width, height)

	wg.Add(1)
	mu.Lock()
	ed, _ := newEditFile(t, comp, fileContent)
	mu.Unlock()
	wg.Wait()

	cases := []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o ####                      │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    fmt.Println(#####, cli.N│
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start := term.Coordinates{Y: 8}
	end := term.Coordinates{Y: 8, X: 1}
	_, _, _, err := ed.Edit(context.Background(), start, end, "")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│unc main() {                │
│    fmt.Println(#####, cli.N│
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 9}
	end = term.Coordinates{Y: 9, X: 1}
	_, _, _, err = ed.Edit(context.Background(), start, end, "")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│unc main() {                │
│fmt.Println(#####, cli.NewCL│
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 9}
	end = term.Coordinates{Y: 9}
	_, _, _, err = ed.Edit(context.Background(), start, end, "\t")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│unc main() {                │
│    fmt.Println(#####, cli.N│
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 8}
	end = term.Coordinates{Y: 10}
	_, _, _, err = ed.Edit(context.Background(), start, end, "func main() {\n\tfmt.Sprintf(\"%s\", \"\")\n")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    fmt.Sprintf(####, ##)   │
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 9, X: 17}
	end = start
	_, _, _, err = ed.Edit(context.Background(), start, end, "🔥")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    fmt.Sprintf(######, ##) │
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 9, X: 17}
	end = term.Coordinates{Y: 9, X: 18}
	_, _, _, err = ed.Edit(context.Background(), start, end, "")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    fmt.Sprintf(####, ##)   │
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 9, X: 0}
	end = term.Coordinates{Y: 10, X: 0}
	_, _, _, err = ed.Edit(context.Background(), start, end, "")
	require.NoError(t, err)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    ### i := #; i < ##; i++ │
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandler(t, comp.Browser(), cases, w)

	start = term.Coordinates{Y: 7, X: 0}
	end = term.Coordinates{Y: 8, X: 0}
	_, _, _, err = ed.Edit(context.Background(), start, end, "")
	require.NoError(t, err)

	sequenceCases := []handlertest.SequenceTestCase{
		{
			"", `┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│#### main() {               │
│    ### i := #; i < ##; i++ │
│        fmt.Println(####, i)│
│                      NORMAL│
└────────────────────────────┘`,
		},
		{
			"uu", `┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│▐   fmt.Sprintf(####, ##)   │
│                      NORMAL│
└────────────────────────────┘`,
		},
		{
			"G", `┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│    }                       │
│}                           │
│                            │
│##### fileContent = ########│
│    ############+           │
│    ###########+            │
│    ####+                   │
│    ########################│
│    ###                     │
│▐                           │
│                      NORMAL│
└────────────────────────────┘`,
		},
		{
			"VkkkkkkduVkkkkkkkkkkkkkkkkkkdugg", `┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    fmt.Sprintf(####, ##)   │
│                      NORMAL│
└────────────────────────────┘`,
		},
		{
			"ggjjjjjjjjjwi/* <$i*/<", `┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    #### fmt.Sprintf(####, #│
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandlerSequenceWriter(t, w, comp.Browser(), width, height, sequenceCases)

	// after flush it should be the same
	win, err := comp.Focus()
	require.NoError(t, err)
	require.NoError(t, comp.Flush(win))
	sequenceCases = []handlertest.SequenceTestCase{
		{
			"", `┌────────────────────────────┐
│o ####                      │
├────────────────────────────┤
│####### main                │
│                            │
│###### (                    │
│    #####                   │
│                            │
│    ########################│
│)                           │
│                            │
│#### main() {               │
│    #### fmt.Sprintf(####, #│
│                      NORMAL│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandlerSequenceWriter(t, w, comp.Browser(), width, height, sequenceCases)

	sequenceCases = []handlertest.SequenceTestCase{
		{
			"Gofunc helloWorld(){\n\tfmt.Println(\"hello world\")\n}", `┌────────────────────────────┐
│o #####                     │
├────────────────────────────┤
│##### fileContent = ########│
│    ############+           │
│    ###########+            │
│    ####+                   │
│    ########################│
│    ###                     │
│                            │
│#### helloWorld(){          │
│    fmt.Println(############│
│}▐                          │
│                      INSERT│
└────────────────────────────┘`,
		},
	}
	handlertest.TestHandlerSequenceWriter(t, w, comp.Browser(), width, height, sequenceCases)

	cleanup()
	goleak.VerifyNone(t)
}

func newInstalledPkgManager(t *testing.T) syntax.PkgManager {
	wd, err := os.Getwd()
	require.NoError(t, err)
	return mockPkgManager{
		ret: iterator.FromSlice([]string{
			filepath.Join(wd, "testdata/tree-sitter.so"),
			filepath.Join(wd, "testdata/highlights.scm"),
		}),
	}
}

type mockPkgManager struct {
	ret iterator.Iterator[string]
}

func (m mockPkgManager) LibDir(ctx context.Context, pkg string) (iterator.Iterator[string], error) {
	return m.ret, nil
}

var i int

func newEditFile(t *testing.T, comp *text.Component, content string) (
	text.CellEditor, text.CellView,
) {
	i++
	uri, err := workspaceapi.ParseURI("memory:///" + strconv.Itoa(i) + ".go")
	require.NoError(t, err)

	tab, err := comp.OpenFileTab(uri, false)
	require.NoError(t, err)

	h, err := comp.Editor(uri)
	require.NoError(t, err)

	start := term.Coordinates{}
	ed := comp.CellEditor(h)
	view := comp.CellView(h)
	_, _, _, err = ed.Edit(context.Background(), start, start, content)
	require.NoError(t, err)

	require.NoError(t, comp.FlushTab(tab))

	err = comp.Browser().Focus().SetContent(tab)
	require.NoError(t, err)

	return ed, view
}

func newWriter(width, height int) *term.StringWriter {
	writer := term.NewStringWriter(width, height)
	writer.ForegroundCh = '#'
	return writer
}

func installDelegate(
	t *testing.T,
	pkgs syntax.PkgManager, width, height int,
	interrupt func(context.Context) error,
) (*sync.Mutex, *text.Component, func()) {
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)

	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)

	ed := vi.Editor()
	w := workspace.NewSchemeWorkspace(uri, scheme)
	comp, err := text.NewComponent(ed, w, text.DefaultConfig())
	require.NoError(t, err)

	comp.Browser().Resize(width, height)

	i := term.FuncInterrupter(interrupt)

	mu := new(sync.Mutex)
	cfg := syntax.DefaultConfig()
	cfg.ScheduleNextTick = func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		return true
	}
	d, evs := syntax.NewDelegate(pkgs, nopNotifications{t: t}, comp, i, cfg)
	require.NoError(t, comp.SubscribeEvents(evs, d))

	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})

	return mu, comp, func() {
		_ = comp.Close()
		_ = d.Close()
		_ = scheme.Close()
	}
}

type nopNotifications struct {
	t *testing.T
}

func (n nopNotifications) Notify(
	level notifications.Level, msg string, args ...any,
) (string, error) {
	switch level {
	case notifications.LevelError, notifications.LevelWarn:
		n.t.Logf(msg, args...)
	}
	return "", nil
}

func (n nopNotifications) NotifyOnce(
	level notifications.Level, msg string, args ...any,
) (string, error) {
	return n.Notify(level, msg, args...)
}

func (n nopNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return nil
}

const fileContent = `package main

import (
	"fmt"

	"github.com/unstablebuild/blue/cli"
)

func main() {
	fmt.Println("%+v", cli.NewCLI)
	for i := 0; i < 10; i++ {
		fmt.Println("%d", i)
	}
}

const fileContent = "package main\n" +
	"import (\n"+
	"\"fmt\"\n"+
	"\n"+
	"\"github.com/unstablebuild/blue/cli\"\n"
	")"
`
