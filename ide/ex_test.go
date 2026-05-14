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
	"io"
	"io/fs"
	"os/user"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	sdkiterator "github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"mvdan.cc/sh/v3/shell"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/cell"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	thandler "unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide/ideshell"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/term/vte/vtereservoir"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/text/registerset"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacetest"
)

// handlertest.TestHandlerSequence maps ':' characters to the following event
// this is to work around ex's assumptions on underlying handler.
var testCommandKey = term.KeyComb{Ch: '\\', Mod: term.ModCtrl}
var bgctx = context.Background()

type browserConstructor func(ed text.Editor, opts ...text.Option) (tui.Handler, browser.Browser, error)

type testFileBuffer struct {
	readOnly  bool
	flushErr  error
	reloadErr error
	closeErr  error
	closed    bool
	lastFlush time.Time
}

func (t *testFileBuffer) Flush(context.Context) (<-chan error, error) {
	if t.readOnly {
		return testFBDone(workspaceapi.ErrFileIsNotWritable), nil
	}
	t.lastFlush = time.Now()
	return testFBDone(t.flushErr), nil
}

func (t *testFileBuffer) Reload(context.Context) (<-chan error, error) {
	t.lastFlush = time.Now()
	return testFBDone(t.reloadErr), nil
}

func (t *testFileBuffer) ForceFlush(context.Context) (<-chan error, error) {
	if t.readOnly {
		t.readOnly = false
	}
	t.lastFlush = time.Now()
	return testFBDone(t.flushErr), nil
}

func testFBDone(err error) <-chan error {
	ch := make(chan error, 1)
	ch <- err
	close(ch)
	return ch
}

func (t *testFileBuffer) LastFlush() time.Time {
	return t.lastFlush
}

func (t *testFileBuffer) Close() error {
	t.closed = true
	return t.closeErr
}

type testLoader struct {
	buf *testFileBuffer
}

func (w *testLoader) Remove(string) error {
	return nil
}

func (w *testLoader) MkdirAll(string, fs.FileMode) error {
	return nil
}

func (w *testLoader) Close() error {
	return nil
}

func (w *testLoader) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
	return 0, nil
}

func (w *testLoader) Signal(workspaceapi.Pid, syscall.Signal) error {
	return nil
}

func (w *testLoader) Load(
	filePath workspaceapi.URI, buf *cell.Buffer,
	swapDir workspaceapi.URI, readOnly bool,
) (
	workspace.FlusherCloser, error,
) {
	if w.buf != nil {
		return w.buf, nil
	}
	return &testFileBuffer{readOnly: readOnly}, nil
}

func (w *testLoader) Recover(
	filePath, swapFilePath workspaceapi.URI,
	buf *cell.Buffer, force bool,
) (workspace.FlusherCloser, error) {
	return w.Load(filePath, buf, swapFilePath, false)
}

func (w *testLoader) ReadDir(name string) ([]os.DirEntry, error) {
	return nil, nil
}

func (w *testLoader) Stat(name string) (os.FileInfo, error) {
	return testFileInfo{name: name}, nil
}

func (w *testLoader) OpenFile(
	path string, flag int, perm os.FileMode,
) (workspaceapi.File, error) {
	panic("unimplemented")
}
func (w *testLoader) NewPty(context.Context) (workspaceapi.Pty, error) {
	return workspaceapi.Pty{
		Master: workspacetest.NewFile(),
		Slave:  workspacetest.NewFile(),
	}, nil
}

func (w *testLoader) SetPtySize(p workspaceapi.Pty, width, height int) error {
	return nil
}

func (w *testLoader) NewFile(fd uintptr, name string) workspaceapi.File {
	panic("unimplemented")
}

func (w *testLoader) Rename(old, new string) error {
	panic("unimplemented")
}

func (w *testLoader) Lstat(path string) (os.FileInfo, error) {
	panic("unimplemented")
}

func (w *testLoader) Readlink(path string) (string, error) {
	panic("unimplemented")
}

func (w *testLoader) Watch(
	path string, c chan<- schemeapi.EventInfo, events ...schemeapi.Event,
) (int, error) {
	return 0, nil
}

func (w *testLoader) StopWatch(int) error {
	return nil
}

func (t testLoader) Chroot(path string) (schemeapi.Scheme, error) {
	panic("unimplemented")
}

func (t testLoader) Root() string {
	panic("unimplemented")
}

func (t testLoader) Symlink(target, link string) error {
	panic("unimplemented")
}

func (t testLoader) TempFile(dir, prefix string) (workspaceapi.File, error) {
	panic("unimplemented")
}

func (t testLoader) Join(elem ...string) string {
	panic("unimplemented")
}

func (t testLoader) Create(filename string) (workspaceapi.File, error) {
	panic("unimplemented")
}

func (t testLoader) Open(filename string) (workspaceapi.File, error) {
	panic("unimplemented")
}

type testFileInfo struct {
	name string
}

func (t testFileInfo) Name() string {
	return t.name
}

func (t testFileInfo) IsDir() bool {
	return t.name == "/" || t.name == "" || t.name == "."
}

func (t testFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (t testFileInfo) Mode() os.FileMode {
	return 0
}

func (t testFileInfo) Size() int64 {
	return 0
}

func (t testFileInfo) Sys() any {
	return nil
}

func (w *testLoader) URI(path string) (workspaceapi.URI, error) {
	return workspaceapi.CurrentUserHostURI(path)
}

func TestBrowserHandlerDraw(t *testing.T) {
	testBrowserHandlerDraw(t, func(ed text.Editor, opts ...text.Option) (tui.Handler, browser.Browser, error) {
		b := newExForTesting(t, ed, opts...)
		return b, b.Browser(), nil
	})
}

func TestComponentOpenEditorIntegration(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	)
	uri, err := workspaceapi.ParseURI("file:///bugz")
	require.NoError(t, err)

	tab, err := b.editFileURI(uri, b.invokeWindow(), false)
	require.NoError(t, err)
	assert.NotPanics(t, func() {
		_ = tab.Handler().(text.Handler)
	})
	assert.NoError(t, b.Close())
}

func TestReadfileCommandReadsFileInOtherWorkspace(t *testing.T) {
	currentDir := t.TempDir()
	currentWorkspaceURI, err := workspaceapi.ParseURI("file://" + currentDir)
	require.NoError(t, err)

	foreignDir := t.TempDir()
	foreignPath := filepath.Join(foreignDir, "foreign.txt")
	require.NoError(t, os.WriteFile(foreignPath, []byte("foreign contents"), 0o644))
	foreignURI, err := workspaceapi.ParseURI("file://" + foreignPath)
	require.NoError(t, err)

	workspaceWithForeignRead := &readfileCrossWorkspaceLoader{
		testWorkspaceWithURI: testWorkspaceWithURI{
			testLoader: &testLoader{},
			uri:        currentWorkspaceURI,
		},
		foreignURI:      foreignURI,
		currentContents: "current\n",
	}

	b := newExForTestingWithWorkspace(t, workspaceWithForeignRead,
		texttest.NopEditor(), vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	defer b.Close()

	currentFileURI, err := workspaceapi.ParseURI(
		"file://" + filepath.Join(currentDir, "current.txt"),
	)
	require.NoError(t, err)
	_, err = b.editFileURI(currentFileURI, b.invokeWindow(), false)
	require.NoError(t, err)

	require.NoError(t, b.ex.dispatchCommand("readfile", foreignURI.String()))

	_, h, ok := b.ex.handlerInFocus()
	require.True(t, ok)
	assert.Equal(t, "current\nforeign contents\n", term.CellsToString(h.CellView().RawCells()))
}

func TestFileExplorerOpenFile(t *testing.T) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)
	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	touchTestFile(t, scheme, "alpha.go")

	b := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
		texttest.NopEditor(), vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	defer b.Close()

	require.Nil(t, b.fileExplorerWin)
	require.NoError(t, b.fexplorer(context.Background()))
	require.NotNil(t, b.fileExplorerWin)

	assert.NotPanics(t, func() {
		_, handled := b.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
		assert.True(t, handled)
	})

	for _, tab := range b.comp.Browser().Tabs() {
		if tab.URI().String() == "memory:///alpha.go" {
			return
		}
	}
	t.Fatalf("expected alpha.go to be opened in a tab")
}

// TestFileExplorerToggleTwice reproduces the bug where toggling the
// file explorer off and on again returns a "command already
// registered" error. Each :fexplorer invocation that opens the
// explorer calls the real editor's Edit on the same URI, which
// subscribes file-level commands (fold/location/git). The second
// call must NOT re-register those commands for the same URI.
func TestFileExplorerToggleTwice(t *testing.T) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)
	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)

	// Use a vi.Editor with a workspace command registry so that
	// fold/location/git file-scoped commands get registered on
	// Edit — this is what the real IDE does and what makes the
	// bug reproducible.
	ed := vi.Editor(vi.WithWorkspaceCommandRegistry(
		uri, texttest.NopWorkspaceRegistry(),
	))
	b := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
		ed, vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	defer b.Close()

	// 1st call: opens the explorer and focuses it.
	require.NoError(t, b.fexplorer(context.Background()))
	require.NotNil(t, b.fileExplorerWin)

	// 2nd call: explorer is focused, so closes it.
	require.NoError(t, b.fexplorer(context.Background()))
	require.Nil(t, b.fileExplorerWin)

	// 3rd call: must re-open without "command already registered".
	require.NoError(t, b.fexplorer(context.Background()))
	require.NotNil(t, b.fileExplorerWin)

	// 4th call: closes again.
	require.NoError(t, b.fexplorer(context.Background()))
	require.Nil(t, b.fileExplorerWin)

	// 5th call: re-opens for the second cycle.
	require.NoError(t, b.fexplorer(context.Background()))
	require.NotNil(t, b.fileExplorerWin)
}

// TestFileExplorerOpenFileThenToggle exercises the realistic
// production path: user opens fexplorer, opens a file from it (which
// registers fold/location/git for that file's URI on the shared vi
// editor's registry), closes the explorer, then reopens. Before the
// caching fix, reopen re-invoked e.ed.Edit("memory:///fexplorer",
// ...) which re-subscribed per-file commands and failed with
// "command already registered".
func TestFileExplorerOpenFileThenToggle(t *testing.T) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)
	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	touchTestFile(t, scheme, "alpha.go")

	ed := vi.Editor(vi.WithWorkspaceCommandRegistry(
		uri, texttest.NopWorkspaceRegistry(),
	))
	b := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
		ed, vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	defer b.Close()

	require.NoError(t, b.fexplorer(context.Background()))
	require.NotNil(t, b.fileExplorerWin)

	// Open alpha.go from the file explorer via Enter.
	_, handled := b.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	require.True(t, handled)
	found := false
	for _, tab := range b.comp.Browser().Tabs() {
		if tab.URI().String() == "memory:///alpha.go" {
			found = true
			break
		}
	}
	require.True(t, found, "alpha.go should be open in a tab")

	// Close the explorer, then re-open it. This MUST succeed.
	require.NoError(t, b.fexplorer(context.Background()))
	require.Nil(t, b.fileExplorerWin)

	require.NoError(t, b.fexplorer(context.Background()))
	require.NotNil(t, b.fileExplorerWin)
}

// TestFileExplorerToggleViaTabKey reproduces the user's exact
// reproduction: with <tab> bound to :fexplorer, pressing <tab>
// repeatedly to open and close the explorer must never fail with
// "command already registered". This exercises the full event
// routing path: the KeyTab event is dispatched to the focused
// window's handler, propagates back to ex.handleEvent, and only
// then falls through to the command-key-binding dispatcher that
// runs :fexplorer. This path differs from calling fexplorer
// directly because when the explorer is focused, <tab> is first
// delivered to the file explorer handler (and hence to the vi
// editor chain) before reaching the :fexplorer binding.
func TestFileExplorerToggleViaTabKey(t *testing.T) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)
	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	touchTestFile(t, scheme, "alpha.go")

	// Use a stateful workspace registry that tracks subscriptions and
	// reports duplicates — this is what the real IDE uses and what
	// would surface a double-Edit on memory:///fexplorer.
	wsReg := newTrackingWorkspaceRegistry()
	ed := vi.Editor(vi.WithWorkspaceCommandRegistry(
		uri, wsReg,
	))
	b := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
		ed, vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		text.WithCommandKeyBinding(
			term.KeyComb{Key: term.KeyTab}, [][]string{{"fexplorer"}}))
	defer b.Close()
	b.Resize(30, 15)

	tabEv := term.Event{Type: term.EventKey, Key: term.KeyTab}

	// 1st <tab>: open explorer.
	b.Handle(tabEv)
	require.NotNil(t, b.fileExplorerWin, "explorer should be open after 1st <tab>")
	require.Empty(t, wsReg.errors(), "no errors after 1st <tab>")

	// 2nd <tab>: close explorer (explorer is focused).
	b.Handle(tabEv)
	require.Nil(t, b.fileExplorerWin, "explorer should be closed after 2nd <tab>")
	require.Empty(t, wsReg.errors(), "no errors after 2nd <tab>")

	// 3rd <tab>: re-open. This is where "command already
	// registered" would surface if the underlying editor was
	// re-created on reopen.
	b.Handle(tabEv)
	require.NotNil(t, b.fileExplorerWin, "explorer should be open after 3rd <tab>")
	require.Empty(t, wsReg.errors(), "no errors after 3rd <tab>")

	// 4th <tab>: close again.
	b.Handle(tabEv)
	require.Nil(t, b.fileExplorerWin, "explorer should be closed after 4th <tab>")
	require.Empty(t, wsReg.errors(), "no errors after 4th <tab>")
}

// TestFileExplorerRestoredAsTabNotDuplicated exercises the bug
// surfaced by the user's real-world setup: a previous session
// persisted memory:///fexplorer as an open file. On startup, the
// workspace handler restores it as a tab via editFileURI, which
// ends up calling vi.Editor.Edit on memory:///fexplorer and
// subscribing per-file commands. When the user then presses <tab>
// to open the explorer, ex.initFileExplorer calls Edit a second
// time on the same URI, which fails with "command already
// registered".
//
// With the fix, the file explorer URI is filtered out of session
// history and restore, so the second Edit call never happens.
func TestFileExplorerRestoredAsTabNotDuplicated(t *testing.T) {
	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)
	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)

	wsReg := newTrackingWorkspaceRegistry()
	ed := vi.Editor(vi.WithWorkspaceCommandRegistry(
		uri, wsReg,
	))
	b := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
		ed, vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	defer b.Close()
	b.Resize(30, 15)

	// Simulate the session-restore path: a stale session cache lists
	// memory:///fexplorer as an open file. The workspace handler
	// would normally call editFileURI on it, which eventually calls
	// vi.Editor.Edit and subscribes fold/location/git commands for
	// the URI on the shared workspace registry.
	fexURI, err := workspaceapi.ParseURI("memory:///fexplorer")
	require.NoError(t, err)
	_, err = b.editFileURI(fexURI, b.invokeWindow(), false)
	require.NoError(t, err)
	require.Empty(t, wsReg.errors())

	// Now trigger :fexplorer. Before the fix this would call Edit
	// on the same URI a second time and one of the per-file command
	// subscriptions would return "command already registered".
	require.NoError(t, b.fexplorer(context.Background()))
	require.Empty(t, wsReg.errors(), "fexplorer must not double-register commands")
	require.NotNil(t, b.fileExplorerWin)
}

// trackingWorkspaceRegistry is a text.WorkspaceCommandRegistry that
// records every subscribe/unsubscribe and fails fast on duplicates —
// mirroring the behaviour of the real per-workspace registry that
// backs vi's file command subscriptions.
type trackingWorkspaceRegistry struct {
	mu   sync.Mutex
	cmds map[string]struct{}
	errs []string
}

func newTrackingWorkspaceRegistry() *trackingWorkspaceRegistry {
	return &trackingWorkspaceRegistry{cmds: make(map[string]struct{})}
}

func (r *trackingWorkspaceRegistry) SubscribeCommandForWorkspace(
	_ workspaceapi.URI, cmd textapi.CommandManual, _ text.CommandHandler,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.cmds[cmd.Name]; ok {
		err := fmt.Sprintf("command already registered: %s", cmd.Name)
		r.errs = append(r.errs, err)
		return errors.New(err)
	}
	r.cmds[cmd.Name] = struct{}{}
	return nil
}

func (r *trackingWorkspaceRegistry) UnsubscribeCommandForWorkspace(
	_ workspaceapi.URI, name string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cmds, name)
	return nil
}

func (r *trackingWorkspaceRegistry) errors() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.errs...)
}

func testBrowserHandlerDraw(t *testing.T, constructor browserConstructor) {
	cases := []handlertest.SequenceTestCase{
		{"a",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`},
		{":edit a.go>",
			`┌━━━━━━────────────┐
│o a.go            │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"a",
			`┌━━━━━━────────────┐
│o a.go            │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":edit /tmp/o.go>",
			`┌────────━━━━━━────┐
│o a.go  o o.go    │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"#", // simulates ctrl-h
			`┌━━━━━━────────────┐
│o a.go  o o.go    │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{"#", // simulates ctrl-h
			`┌────────━━━━━━────┐
│o a.go  o o.go    │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{"$", // simulates ctrl-l
			`┌━━━━━━────────────┐
│o a.go  o o.go    │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{"$", // simulates ctrl-l
			`┌────────━━━━━━────┐
│o a.go  o o.go    │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{":tcl>",
			`┌━━━━━━────────────┐
│o a.go            │
├──────────────────┤
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘`},
		{":wq!^^^^^",
			`┌━━━━━━────────────┐
│o a.go            │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
		{":<",
			`┌━━━━━━────────────┐
│o a.go            │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
		{":edit o.go>1111",
			`┌────────━━━━━━────┐
│o a.go  o o.go    │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
		{"$$##",
			`┌────────━━━━━━────┐
│o a.go  o o.go    │
├──────────────────┤
│GGGGGGGGGGGGGGGGGG│
│GGGGGGGGGGGGGGGGGG│
│GGGGGGGGGGGGGGGGGG│
│GGGGGGGGGGGGGGGGGG│
│GGGGGGGGGGGGGGGGGG│
│GGGGGGGGGGGGGGGGGG│
└──────────────────┘`},
	}
	bh, b, err := constructor(texttest.NopEditor(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandKeyBinding(term.KeyComb{Ch: '4'}, [][]string{{"windowclose"}}),
	)
	require.NoError(t, err)

	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	focus, err := b.Focus()
	require.NoError(t, err)

	win, err := b.Split(browserapi.OrientationLeft, focus, browsertest.NewTestHandler())
	require.NoError(t, err)

	focus = win

	h := browsertest.NewTestHandler()
	h.Ch = 'Z' // helps identify in tests

	_, err = b.Split(browserapi.OrientationBottom, focus, h)
	require.NoError(t, err)

	cases = []handlertest.SequenceTestCase{
		{"",
			`┌──────────────────┐
│o a.go  o o.go    │
├────────┐┌────────┤
│AAAAAAAA││GGGGGGGG│
│AAAAAAAA││GGGGGGGG│
└────────┘│GGGGGGGG│
┌────────┐│GGGGGGGG│
│ZZZZZZZZ││GGGGGGGG│
│ZZZZZZZZ││GGGGGGGG│
└────────┘└────────┘`},
		{":<111111111",
			`┌──────────────────┐
│o a.go  o o.go    │
├────────┐┌────────┤
│AAAAAAAA││GGGGGGGG│
│AAAAAAAA││GGGGGGGG│
└────────┘│GGGGGGGG│
┌────────┐│GGGGGGGG│
│cccccccc││GGGGGGGG│
│cccccccc││GGGGGGGG│
└────────┘└────────┘`},
	}

	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	var closed int
	hx := browsertest.NewTestHandler()
	hx.Ch = '$'
	hx.CloseCallback = func() error { closed++; return nil }
	require.NoError(t, focus.SetContent(hx))

	cases = []handlertest.SequenceTestCase{
		{"",
			`┌──────────────────┐
│o a.go  o o.go    │
├────────┐┌────────┤
│$$$$$$$$││GGGGGGGG│
│$$$$$$$$││GGGGGGGG│
└────────┘│GGGGGGGG│
┌────────┐│GGGGGGGG│
│cccccccc││GGGGGGGG│
│cccccccc││GGGGGGGG│
└────────┘└────────┘`},
	}

	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	require.NoError(t, win.Close())
	require.NoError(t, focus.Close())

	assert.Equal(t, 1, closed)

	cases = []handlertest.SequenceTestCase{
		// test CommandKeyBindings
		{"4$$$",
			`┌━━━━━━────────────┐
│o a.go  o o.go    │
├──────────────────┤
│HHHHHHHHHHHHHHHHHH│
│HHHHHHHHHHHHHHHHHH│
│HHHHHHHHHHHHHHHHHH│
│HHHHHHHHHHHHHHHHHH│
│HHHHHHHHHHHHHHHHHH│
│HHHHHHHHHHHHHHHHHH│
└──────────────────┘`},
		{":tcall>:edit o.go>bcde####",
			`┌━━━━━━────────────┐
│o o.go            │
├──────────────────┤
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
└──────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	cases = []handlertest.SequenceTestCase{
		{"", `┌━━┐
│o │
├II┤
IIII`},
	}
	handlertest.TestHandlerSequence(t, bh, 4, 4, cases)

	_, err = b.Notify(browserapi.LevelInfo, "wasup: %s", "Z")
	require.NoError(t, err)
	cases = []handlertest.SequenceTestCase{
		{"",
			`┌━━━━┌─────────────┐
│o o.│ wasup: Z    │
├────└─────────────┘
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
│IIIIIIIIIIIIIIIIII│
└──────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	uri, err := workspaceapi.ParseURI("file:///bugz")
	require.NoError(t, err)
	nh, err := b.Open(uri)
	require.NoError(t, err)
	focus, err = b.Focus()
	require.NoError(t, err)
	err = focus.SetContent(nh)
	require.NoError(t, err)

	cases = []handlertest.SequenceTestCase{
		{"b",
			`┌────┌─────────────┐
│o o.│ wasup: Z    │
├────└─────────────┘
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":3>",
			`┌────┌─────────────┐
│o o.│ wasup: Z    │
├────└─────────────┘
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│▐BBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":0>",
			`┌────┌─────────────┐
│o o.│ wasup: Z    │
├────└─────────────┘
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	// calls from a browser client will block until the first
	// "use" of the installed handler. Client and server
	// are tipically running in different goroutines so
	// this is not a problem elsewhere.
	testCh := make(chan struct{})
	defer close(testCh)
	go func() {
		for {
			select {
			case _, ok := <-testCh:
				if !ok {
					return
				}
			default:
				if sh, ok := bh.(*safeHandler); ok {
					sh.Draw(term.NewStringWriter(209, 100))
				}
			}
		}
	}()

	floating1, err := b.Floating(browsertest.NewTestFloating(4, 2),
		browserapi.FloatingConfig{Offset: term.Coordinates{X: 1, Y: 1}})
	require.NoError(t, err)

	focus, err = b.Focus()
	require.NoError(t, err)

	// should not be able to split over a floating window, which is currently in focus
	_, err = b.Split(browserapi.OrientationTop, focus, browsertest.NewTestHandler())
	require.Error(t, err)
	cases = []handlertest.SequenceTestCase{
		{"",
			`┌────┌─────────────┐
│o o.│ wasup: Z    │
├────└─────────────┘
│┌────┐BBBBBBBBBBBB│
││AAAA│BBBBBBBBBBBB│
││AAAA│BBBBBBBBBBBB│
│└────┘BBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":e!>", // test reload non file
			`┌────┌─────────────┐
│o o.│ cannot      │
├────│ reload      │
│┌───│ this        │
││AAA│ content     │
││AAA└─────────────┘
│└───┌─────────────┐
│BBBB│ wasup: Z    │
│BBBB└─────────────┘
└──────────────────┘`},
	}

	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	require.NoError(t, floating1.Close())

	cases = []handlertest.SequenceTestCase{
		{"",
			`┌────┌─────────────┐
│o o.│ cannot      │
├────│ reload      │
│BBBB│ this        │
│BBBB│ content     │
│BBBB└─────────────┘
│BBBB┌─────────────┐
│BBBB│ wasup: Z    │
│BBBB└─────────────┘
└──────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	o := browserapi.BarConfig{Size: 1, Orientation: browserapi.OrientationTop}
	for i := range 4 {
		b1 := browsertest.NewTestHandler()
		b1.Ch = rune(strconv.Itoa(i)[0])
		err = b.Bar(o, b1)
		require.NoError(t, err)
		o.Orientation++
	}

	// test case for issue #27
	cases = []handlertest.SequenceTestCase{
		{":edit ait^^^aix^^^^ airsoft.map>",
			`┌────────────────━━━━━━━━━━━━━─────┌─────────────┐
│o o.go  o bugz  o airsoft.map     │ cannot      │
├──────────────────────────────────│ reload      │
│0000000000000000000000000000000000│ this        │
├─┬────────────────────────────────│ content     │
│2│AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA└─────────────┘
│2│AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA┌─────────────┐
├─┴────────────────────────────────│ wasup: Z    │
│1111111111111111111111111111111111└─────────────┘
└────────────────────────────────────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, bh, 50, 10, cases)

	assert.NoError(t, bh.(io.Closer).Close())
	assert.NoError(t, b.Close())
	assert.Equal(t, 1, closed)
}

func TestShellCommandOpensTab(t *testing.T) {
	workspaceURI, err := workspaceapi.ParseURI("file:///tmp/my-workspace")
	require.NoError(t, err)
	w := testWorkspaceWithURI{testLoader: &testLoader{}, uri: workspaceURI}
	cfg := vte.DefaultConfig()
	scheduler := newQueuedScheduler()
	cfg.ScheduleNextTick = scheduler.ScheduleNextTick
	b := newExForTestingWithWorkspace(t, w, texttest.NopEditor(),
		cfg, nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
	)
	b.mu = &sync.Mutex{}
	b.scheduler = scheduler
	defer b.Close()

	require.NoError(t, b.comp.RegisterREPLCommand(
		textapi.CommandManual{Name: "status", Summary: "show status"},
		&testShellREPLHandler{},
	))

	cases := []handlertest.SequenceTestCase{
		{
			InputSequence: "<c-\\\\>shell<enter>help<enter>",
			Expected: "┌━━━━━━━───────────┐\n" +
				"│ shell           │\n" +
				"├──────────────────┤\n" +
				"│  available       │\n" +
				"│  commands        │\n" +
				"│• status — show   │\n" +
				"│  status          │\n" +
				"│                  │\n" +
				"│> ▐               │\n" +
				"└──────────────────┘",
		},
	}

	handlertest.RunHandlerSequence(t, b, 20, 10, cases)

	tabs := b.comp.Tabs()
	require.Len(t, tabs, 1)
	_, ok := tabs[0].Handler().(*ideshell.Handler)
	assert.True(t, ok)
	assert.Equal(t, text.DefaultConfig().Icons.Shell, b.config.Icons.Shell)
	assert.Equal(t, "shell:///tmp/my-workspace", tabs[0].URI().String())
}

// TestShellCommandPastesArgument verifies that arguments passed to the
// `:shell` ex command are submitted to the shell prompt as a single
// command line, both when the shell tab is created on first use and
// when the existing companion shell tab is reused.
func TestShellCommandPastesArgument(t *testing.T) {
	workspaceURI, err := workspaceapi.ParseURI("file:///tmp/shell-paste")
	require.NoError(t, err)
	w := testWorkspaceWithURI{testLoader: &testLoader{}, uri: workspaceURI}
	cfg := vte.DefaultConfig()
	scheduler := newQueuedScheduler()
	cfg.ScheduleNextTick = scheduler.ScheduleNextTick
	svc := storagestub.NewInMemoryService()
	b := newExForTestingWithStorage(t, w, svc, texttest.NopEditor(),
		cfg, nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
	)
	b.mu = &sync.Mutex{}
	b.scheduler = scheduler
	defer b.Close()

	// First call creates the companion shell tab and pastes the
	// command. The repl persists submitted commands to history.
	require.NoError(t, b.shellnewtab(context.Background(), "help"))
	var doc struct{ Items []string }
	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	require.NotEmpty(t, doc.Items)
	assert.Equal(t, "help", doc.Items[len(doc.Items)-1])

	// Second call reuses the existing tab and submits a different
	// command, which should also be persisted to history.
	require.NoError(t, b.shellnewtab(
		context.Background(), "help", "help",
	))
	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	assert.Equal(t, "help help", doc.Items[len(doc.Items)-1])

	// Calling without arguments must not submit anything new.
	prev := len(doc.Items)
	require.NoError(t, b.shellnewtab(context.Background()))
	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	assert.Equal(t, prev, len(doc.Items))
}

// TestDebuggerCommandOpensShellWithDebugger verifies that running
// `:debugger` with no arguments behaves like `:shell debugger`:
// the companion shell tab is opened (or focused) and the literal
// command "debugger" is submitted on its prompt. The integration
// is wired in workspace_handler.go via
// debugshell.PromptHandler.WithOpenShell(ex.shellnewtab), so
// driving shellnewtab directly with "debugger" exercises the same
// code path that the prompt handler will invoke.
func TestDebuggerCommandOpensShellWithDebugger(t *testing.T) {
	workspaceURI, err := workspaceapi.ParseURI("file:///tmp/debugger-noargs")
	require.NoError(t, err)
	w := testWorkspaceWithURI{testLoader: &testLoader{}, uri: workspaceURI}
	cfg := vte.DefaultConfig()
	scheduler := newQueuedScheduler()
	cfg.ScheduleNextTick = scheduler.ScheduleNextTick
	svc := storagestub.NewInMemoryService()
	b := newExForTestingWithStorage(t, w, svc, texttest.NopEditor(),
		cfg, nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
	)
	b.mu = &sync.Mutex{}
	b.scheduler = scheduler
	defer b.Close()

	// First call: companion shell tab is created and "debugger"
	// is submitted, which the repl persists to history.
	require.NoError(t, b.shellnewtab(context.Background(), "debugger"))
	var doc struct{ Items []string }
	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	require.NotEmpty(t, doc.Items)
	assert.Equal(t, "debugger", doc.Items[len(doc.Items)-1])

	// The shell tab must be the companion ideshell handler.
	tabs := b.comp.Tabs()
	require.Len(t, tabs, 1)
	_, ok := tabs[0].Handler().(*ideshell.Handler)
	assert.True(t, ok)

	// Second call reuses the existing companion shell tab and
	// re-submits "debugger" on its prompt.
	require.NoError(t, b.shellnewtab(context.Background(), "debugger"))
	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	assert.Equal(t, "debugger", doc.Items[len(doc.Items)-1])
}

// TestShellCommandPersistsHistory verifies that commands entered into
// the IDE shell are persisted to the shared storageapi.Service via the
// shellHistoryDocumentID, and that a second ex booted on the same
// storage observes the previously persisted entries.
func TestShellCommandPersistsHistory(t *testing.T) {
	workspaceURI, err := workspaceapi.ParseURI("file:///tmp/shell-hist")
	require.NoError(t, err)
	w := testWorkspaceWithURI{testLoader: &testLoader{}, uri: workspaceURI}

	cfg := vte.DefaultConfig()
	scheduler := newQueuedScheduler()
	cfg.ScheduleNextTick = scheduler.ScheduleNextTick

	svc := storagestub.NewInMemoryService()

	b := newExForTestingWithStorage(t, w, svc, texttest.NopEditor(),
		cfg, nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
	)
	b.mu = &sync.Mutex{}
	b.scheduler = scheduler

	// Drive `:shell help<enter>` through the handler. The exact
	// layout is verified by TestShellCommandOpensTab; here we only
	// care about the persistence side-effect on storageapi.Service.
	handlertest.RunHandlerSequence(t, b, 20, 10, []handlertest.SequenceTestCase{
		{
			InputSequence: "<c-\\\\>shell<enter>help<enter>",
			Expected: "┌━━━━━━━───────────┐\n" +
				"│\ue691 shell           │\n" +
				"├──────────────────┤\n" +
				"│> help            │\n" +
				"│• help — Show     │\n" +
				"│  available       │\n" +
				"│  commands        │\n" +
				"│                  │\n" +
				"│> ▐               │\n" +
				"└──────────────────┘",
		},
	})

	// The command typed through the shell tab should have been
	// persisted by repl.WithStorage under shellHistoryDocumentID.
	var doc struct {
		Items []string
	}
	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	require.NotEmpty(t, doc.Items)
	assert.Equal(t, "help", doc.Items[0])

	b.Close()

	// Boot a fresh ex backed by the same storage and verify the
	// repl-backed history document is still available.
	b2 := newExForTestingWithStorage(t, w, svc, texttest.NopEditor(),
		cfg, nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
	)
	b2.mu = &sync.Mutex{}
	b2.scheduler = scheduler
	defer b2.Close()

	require.NoError(t, b2.shellnewtab(context.Background()))
	tabs := b2.comp.Tabs()
	require.Len(t, tabs, 1)
	_, ok := tabs[0].Handler().(*ideshell.Handler)
	assert.True(t, ok)

	require.NoError(t, svc.Get(
		context.Background(), shellHistoryDocumentID, &doc,
	))
	assert.Contains(t, doc.Items, "help")
}

func TestShellCommandRespectsMaxHistory(t *testing.T) {
	workspaceURI, err := workspaceapi.ParseURI("file:///tmp/shell-hist-max")
	require.NoError(t, err)
	w := testWorkspaceWithURI{testLoader: &testLoader{}, uri: workspaceURI}

	cfg := vte.DefaultConfig()
	scheduler := newQueuedScheduler()
	cfg.ScheduleNextTick = scheduler.ScheduleNextTick

	svc := storagestub.NewInMemoryService()
	b := newExForTestingWithStorage(t, w, svc, texttest.NopEditor(),
		cfg, nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
		text.WithShellMaxHistory(2),
	)
	b.mu = &sync.Mutex{}
	b.scheduler = scheduler
	defer b.Close()

	openShell := func() {
		b.Handle(term.Event{Type: term.EventKey, Ch: '\\', Mod: term.ModCtrl})
		for _, r := range "shell" {
			b.Handle(term.Event{Type: term.EventKey, Ch: r})
		}
		b.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	}
	submitHelp := func() {
		for _, r := range "help" {
			b.Handle(term.Event{Type: term.EventKey, Ch: r})
		}
		b.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	}

	openShell()
	for range 3 {
		submitHelp()
	}

	var doc struct {
		Items []string
	}
	require.NoError(t, svc.Get(context.Background(), shellHistoryDocumentID, &doc))
	require.Len(t, doc.Items, 2)
	assert.Equal(t, []string{"help", "help"}, doc.Items)
	assert.Len(t, doc.Items, 2)
}

func assertHandled(
	t *testing.T, h *browsertest.TestHandler, startingRune rune, exit, handled bool,
) {
	// test handler increments the character that it displays next
	// upon handling a new event
	require.False(t, exit)
	require.True(t, handled)
	assert.NotEqual(t, startingRune, h.Ch)
}

func TestBrowserHandlerInterrupts(t *testing.T) {
	t.Run("Interrupt calls interrupt handle", func(t *testing.T) {
		var wg sync.WaitGroup
		opts := []text.Option{text.WithEventPublisher(
			func(ev term.Event) bool {
				assert.Equal(t, term.EventInterrupt, ev.Type)
				wg.Done()
				return true
			},
		),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		browser := newExForTesting(t, texttest.NopEditor(), opts...)
		defer browser.Close()

		wg.Add(1)
		browser.Browser().PublishEvent(term.Event{Type: term.EventInterrupt})

		wg.Wait()
	})
	t.Run("SendEventNone calls interrupt handle", func(t *testing.T) {
		var wg sync.WaitGroup
		opts := []text.Option{text.WithEventPublisher(func(ev term.Event) bool {
			assert.Equal(t, term.EventNone, ev.Type)
			wg.Done()
			return true
		}),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		browser := newExForTesting(t, texttest.NopEditor(), opts...)
		defer browser.Close()

		wg.Add(1)
		browser.Browser().PublishEvent(term.Event{Type: term.EventNone})

		wg.Wait()
	})
}

func TestMultipleFilesStartup(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{"",
			`┌────────━━━━━━━───┐
│o a.go  o wi.go   │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"aa",
			`┌────────━━━━━━━───┐
│o a.go  o wi.go   │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{"#", // simulates ctrl-h
			`┌━━━━━━────────────┐
│o a.go  o wi.go   │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"#:reloadfile>",
			`┌────────━━━━━━━───┐
│o a.go  o wi.go   │
├──────────────────┤
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘`},
	}

	file1, err := workspaceapi.ParseURI("file:///a.go")
	require.NoError(t, err)
	file2, err := workspaceapi.ParseURI("file:///wi.go")
	require.NoError(t, err)
	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}
	mockBuf := testFileBuffer{}
	workspace := testLoader{buf: &mockBuf}
	b := newExForTestingWithWorkspace(t, &workspace, texttest.NopEditor(),
		vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), opts...)
	defer b.Close()
	_, err = b.editFileURI(file1, b.invokeWindow(), false)
	require.NoError(t, err)
	_, err = b.editFileURI(file2, b.invokeWindow(), false)
	require.NoError(t, err)

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)

}

func TestWriteExclamationNoQuit(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":w!>",
			`┌────┌─────────────┐
│    │ cannot      │
├────│ flush this  │
│    │ content     │
│    └─────────────┘
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`},
	}
	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}
	mockBuf := testFileBuffer{}
	workspace := testLoader{buf: &mockBuf}
	b := newExForTestingWithWorkspace(t, &workspace, texttest.NopEditor(),
		vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
	assert.False(t, b.exit)
}

func TestPreviewCommands(t *testing.T) {
	t.Run("reverts a preview", func(t *testing.T) {
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		var called, reverted bool
		previews := map[string]PreviewFunc{
			"setTheme": func(string, ...string) (component.Responsive, func(), bool) {
				called = true
				return nil, func() {
					reverted = true
				}, true
			},
		}
		mockBuf := testFileBuffer{}
		workspace := testLoader{buf: &mockBuf}
		b := newExForTestingCommandsPreview(t, &workspace, texttest.NopEditor(),
			vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), previews, opts...)
		defer b.Close()

		_, cancel, ok := b.Preview("setTheme", "arg1")
		require.True(t, ok)

		assert.True(t, called)
		assert.False(t, reverted)

		cancel()
		assert.True(t, called)
		assert.True(t, reverted)
	})

	t.Run("ignores previews when command is not set", func(t *testing.T) {
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		var called, reverted bool
		previews := map[string]PreviewFunc{
			"setTheme": func(string, ...string) (component.Responsive, func(), bool) {
				called = true
				return nil, func() {
					reverted = true
				}, true
			},
		}
		mockBuf := testFileBuffer{}
		workspace := testLoader{buf: &mockBuf}
		b := newExForTestingCommandsPreview(t, &workspace, texttest.NopEditor(),
			vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), previews, opts...)
		defer b.Close()

		_, _, ok := b.Preview("guiSetTheme", "arg1")
		require.False(t, ok)

		assert.False(t, called)
		assert.False(t, reverted)
	})

	t.Run("ignores previews when previews are disabled", func(t *testing.T) {
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		var called, reverted bool
		mockBuf := testFileBuffer{}
		workspace := testLoader{buf: &mockBuf}
		b := newExForTestingCommandsPreview(t, &workspace, texttest.NopEditor(),
			vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(),
			nil /*previews*/, opts...)
		defer b.Close()

		_, _, ok := b.Preview("guiSetTheme", "arg1")
		require.False(t, ok)

		assert.False(t, called)
		assert.False(t, reverted)
	})
}

func TestBrowserCloseLastWindow(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":windowclose>",
			`┌────┌─────────────┐
│    │ cannot      │
├────│ close last  │
│    │ tiled       │
│    │ window      │
│    └─────────────┘
│                  │
│                  │
│                  │
└──────────────────┘`},
	}
	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}
	mockBuf := testFileBuffer{}
	workspace := testLoader{buf: &mockBuf}
	b := newExForTestingWithWorkspace(t, &workspace, texttest.NopEditor(),
		vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
	assert.False(t, b.exit)
}

func TestExCommandResponsive(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit",
			`                    
                    
                    
                    
edit▐               
edit                
                    
                    
                    
                    `},
		{":eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
			`                    
                    
                    
                    
eeeeeeeeeeeeeeeee   
eeeeeeeeeeeeeeeee   
eeeeeeeeeeeeeeee▐   
                    
                    
                    `},
		{":edit eeeeeeeeeeeeeeeeeeeeeeeee",
			`                    
                    
                    
                    
edit eeeeeeeeeeee   
eeeeeeeeeeeee▐      
                    
                    
                    
                    `},
	}

	var closeFns []func() error
	fn := func(t *testing.T) tui.Handler {
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithWindowManagerConfig(thandler.WindowManagerConfig{
				WindowManagerConfig: tcomponent.WindowManagerConfig{Frame: false}}),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		b := newExForTesting(t, texttest.NopEditor(), opts...)
		closeFns = append(closeFns, b.Close)
		return b
	}
	handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
	for _, close := range closeFns {
		close()
	}
}

func TestExKeySequence(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{"zgl",
			`┌━━━━━━━━──────────┐
│o 10k.go  o 2     │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"g",
			`┌──────────━━━─────┐
│o 10k.go  o 2     │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{"go",
			`┌──────────━━━─────┐
│o 10k.go  o 2     │
├──────────────────┤
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘`},
		// 2 seconds of wait should be plenty for sequencer to deem 'g' sequence
		// stale and re-issue event.
		{"g____________________",
			`┌──────────━━━─────┐
│o 10k.go  o 2     │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{"gg",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`},
	}

	var closeFns []func() error
	fn := func(t *testing.T) tui.Handler {
		var mu sync.Mutex
		file1, err := workspaceapi.ParseURI("file:///10k.go")
		require.NoError(t, err)
		file2, err := workspaceapi.ParseURI("file:///2")
		require.NoError(t, err)
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithCommandSequenceBinding(thandler.Sequence{
				First: term.KeyComb{Ch: 'g'},
				Last:  term.KeyComb{Ch: 'l'},
			}, [][]string{{"tabnext"}}),
			text.WithCommandSequenceBinding(thandler.Sequence{
				First: term.KeyComb{Ch: 'g'},
				Last:  term.KeyComb{Ch: 'g'},
			}, [][]string{{"tabcloseall"}}),
			text.WithSequencerTimeout(1 * time.Second),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}

		ex := new(ex)
		notifications := newWorkspaceNotifications(
			storagestub.NewInMemoryService(), notificationsConfig(),
			&workspaceManagerMock{workspace: ex})
		ex.syncCommandPrompt = true
		require.NoError(t, ex.init(texttest.NopEditor(), &testLoader{},
			storagestub.NewInMemoryService(), notifications, file2,
			vte.DefaultConfig(), plugin.DefaultBarConfig(), func(ev term.Event) bool {
				// do not confuse interrupt from list with sequence re-issue commands
				if ev.Type == term.EventInterrupt {
					return true
				}
				mu.Lock()
				defer mu.Unlock()
				ex.Handle(ev)
				return true
			}, 0, clipboard.NewInMemory(), nil, nil, nil, nil, opts...))
		ex.subscribeCommands()
		b := testEx{ex: ex}
		closeFns = append(closeFns, func() error {
			mu.Lock()
			defer mu.Unlock()
			return b.Close()
		})
		_, err = b.editFileURI(file1, ex.invokeWindow(), false)
		require.NoError(t, err)
		_, err = b.editFileURI(file2, ex.invokeWindow(), false)
		require.NoError(t, err)
		return handler.Sync(&mu, b)
	}
	handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
	for _, close := range closeFns {
		close()
	}
}

func TestExTabIntegration(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{"",
			`┌━━━━━━━━━─────────┐
│x Fieshta  x Pahty│
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{":tabcloseall>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`},
	}

	var closeFns []func() error
	fn := func(t *testing.T) tui.Handler {
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		b := newExForTesting(t, texttest.NopEditor(), opts...)
		uri1, err := workspaceapi.ParseURI("file:///Fieshta")
		require.NoError(t, err)
		uri2, err := workspaceapi.ParseURI("file:///Pahty")
		require.NoError(t, err)
		tab, err := b.comp.Tab(uri1, 'x', "Fieshta", browsertest.NewTestHandler())
		require.NoError(t, err)
		_, err = b.comp.Tab(uri2, 'x', "Pahty", browsertest.NewTestHandler())
		require.NoError(t, err)
		focus, err := b.comp.Focus()
		require.NoError(t, err)
		require.NoError(t, focus.SetContent(tab))
		closeFns = append(closeFns, b.Close)
		return b
	}
	handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
	for _, close := range closeFns {
		close()
	}
}

func TestExTabclosePromptsForDirtyTab(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	)
	defer b.Close()

	uri, err := workspaceapi.ParseURI("file:///dirty.go")
	require.NoError(t, err)
	_, err = b.editFileURI(uri, b.invokeWindow(), false)
	require.NoError(t, err)
	editBuffer(t, b.ex, uri, "ABC")

	require.NoError(t, b.tabclose(context.Background()))

	assert.Len(t, b.comp.Tabs(), 1)
	dirty, ok := b.comp.IsDirty(uri)
	require.True(t, ok)
	assert.True(t, dirty)
	assert.Equal(t, 1, b.comp.Browser().FloatingWindows())

	exit, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
	assert.False(t, exit)
	assert.True(t, handled)
	assert.Empty(t, b.comp.Tabs())
	assert.Equal(t, 0, b.comp.Browser().FloatingWindows())
}

func TestExTabcloseDirtyTabNoKeepsTabOpen(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	)
	defer b.Close()

	uri, err := workspaceapi.ParseURI("file:///dirty.go")
	require.NoError(t, err)
	_, err = b.editFileURI(uri, b.invokeWindow(), false)
	require.NoError(t, err)
	editBuffer(t, b.ex, uri, "ABC")

	require.NoError(t, b.tabclose(context.Background()))

	assert.Len(t, b.comp.Tabs(), 1)
	assert.Equal(t, 1, b.comp.Browser().FloatingWindows())

	exit, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'n'})
	assert.False(t, exit)
	assert.True(t, handled)
	assert.Len(t, b.comp.Tabs(), 1)
	assert.Equal(t, 0, b.comp.Browser().FloatingWindows())
}

func TestExTabcloseallPromptsForDirtyTabs(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	)
	defer b.Close()

	dirtyURI, err := workspaceapi.ParseURI("file:///dirty.go")
	require.NoError(t, err)
	_, err = b.editFileURI(dirtyURI, b.invokeWindow(), false)
	require.NoError(t, err)
	editBuffer(t, b.ex, dirtyURI, "ABC")
	cleanURI, err := workspaceapi.ParseURI("file:///clean.go")
	require.NoError(t, err)
	_, err = b.editFileURI(cleanURI, b.invokeWindow(), false)
	require.NoError(t, err)

	require.NoError(t, b.tabcloseall(context.Background()))

	assert.Len(t, b.comp.Tabs(), 2)
	assert.Equal(t, 1, b.comp.Browser().FloatingWindows())

	exit, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
	assert.False(t, exit)
	assert.True(t, handled)
	assert.Empty(t, b.comp.Tabs())
	assert.Equal(t, 0, b.comp.Browser().FloatingWindows())
}

func TestExTabcloseinactivePromptsForDirtyInactiveTabs(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(),
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	)
	defer b.Close()

	inactiveURI, err := workspaceapi.ParseURI("file:///inactive.go")
	require.NoError(t, err)
	inactiveTab, err := b.editFileURI(inactiveURI, b.invokeWindow(), false)
	require.NoError(t, err)
	editBuffer(t, b.ex, inactiveURI, "ABC")
	activeURI, err := workspaceapi.ParseURI("file:///active.go")
	require.NoError(t, err)
	activeTab, err := b.editFileURI(activeURI, b.invokeWindow(), false)
	require.NoError(t, err)
	_, active := activeTab.Window()
	require.True(t, active)
	_, inactive := inactiveTab.Window()
	require.False(t, inactive)

	require.NoError(t, b.tabcloseinactive(context.Background()))

	assert.Len(t, b.comp.Tabs(), 2)
	assert.Equal(t, 1, b.comp.Browser().FloatingWindows())

	exit, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
	assert.False(t, exit)
	assert.True(t, handled)
	tabs := b.comp.Tabs()
	require.Len(t, tabs, 1)
	assert.True(t, tabs[0].URI().Equal(activeURI))
	assert.Equal(t, 0, b.comp.Browser().FloatingWindows())
}

func TestExExit(t *testing.T) {
	commands := []string{
		"writequit",
		"writeforcequit!",
		"forcequit!",
		"quit",
	}

	for _, cmd := range commands {
		t.Run(fmt.Sprintf("ex exits %s command is issued", cmd), func(t *testing.T) {
			b := newExForTesting(t, texttest.NopEditor(),
				text.WithCommandKey(testCommandKey),
				text.WithCommandOverlayConfig(testCommandOverlayConfig()),
			)
			defer b.Close()

			// start command prompt
			ev := term.Event{
				Type: term.EventKey,
				Ch:   testCommandKey.Ch,
				Mod:  testCommandKey.Mod,
				Key:  testCommandKey.Key,
			}
			exit, handled := b.Handle(ev)
			assert.True(t, handled)
			require.False(t, exit)

			for _, ch := range cmd {
				exit, handled := b.Handle(term.Event{Ch: ch, Type: term.EventKey})
				assert.True(t, handled)
				require.False(t, exit)
			}

			exit, handled = b.Handle(
				term.Event{Key: term.KeyEnter, Type: term.EventKey})
			assert.True(t, handled)
			require.True(t, exit)

		})
	}

	t.Run("ex does not exit when inner handler returns exit=true", func(t *testing.T) {
		b := newExForTesting(t, texttest.NopEditor(),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		)
		defer b.Close()

		h := browsertest.NewTestHandler()
		h.Exit = true
		h.Handled = true

		uri, err := workspaceapi.ParseURI("file:///bols")
		require.NoError(t, err)

		tab, err := b.comp.Tab(uri, 'x', "bleh", h)
		require.NoError(t, err)

		w, err := b.comp.Focus()
		require.NoError(t, err)

		err = w.SetContent(tab)
		require.NoError(t, err)

		exit, handled := b.Handle(term.Event{Ch: 'a', Type: term.EventKey})
		assert.True(t, handled)
		assert.False(t, exit)
	})
}

// remove non-determinism of search.List async search
type testEx struct {
	*ex
	mu        sync.Locker
	scheduler *queuedScheduler
}

func (t testEx) Handle(ev term.Event) (bool, bool) {
	unlock := t.lock()
	quit, handle := t.ex.Handle(ev)
	unlock()
	if t.ex.companionShell != nil {
		t.ex.companionShell.Wait()
	}
	t.ex.Wait()
	// Wait for async flush completions to deliver their callbacks
	// to the scheduler queue, then drain the scheduler once under
	// t.mu (mirroring the host event loop). New callbacks
	// scheduled by tasks (e.g. tcell colour resets) are intentionally
	// not awaited here — they ride the next Handle/Draw turn the
	// same way the production event loop processes them.
	t.ex.waitInflight()
	t.flushScheduled()
	return quit, handle
}

func (t testEx) Draw(w term.Writer) {
	t.flushScheduled()
	unlock := t.lock()
	defer unlock()
	t.ex.Draw(w)
}

func (t testEx) Resize(width, height int) {
	unlock := t.lock()
	defer unlock()
	t.ex.Resize(width, height)
}

func (t testEx) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	unlock := t.lock()
	defer unlock()
	return t.ex.Cursor()
}

func (t testEx) Selection() (string, bool) {
	unlock := t.lock()
	defer unlock()
	return t.ex.Selection()
}

func (t testEx) lock() func() {
	if t.mu == nil {
		return func() {}
	}
	t.mu.Lock()
	return t.mu.Unlock
}

func (t testEx) flushScheduled() {
	if t.scheduler == nil {
		return
	}
	t.scheduler.Flush(t.mu)
}

type queuedScheduler struct {
	mu      sync.Mutex
	pending []func()
}

func newQueuedScheduler() *queuedScheduler {
	return &queuedScheduler{}
}

func (s *queuedScheduler) ScheduleNextTick(fn func()) bool {
	s.mu.Lock()
	s.pending = append(s.pending, fn)
	s.mu.Unlock()
	return true
}

func (s *queuedScheduler) Flush(lock sync.Locker) {
	for {
		s.mu.Lock()
		if len(s.pending) == 0 {
			s.mu.Unlock()
			return
		}
		fn := s.pending[0]
		s.pending = s.pending[1:]
		s.mu.Unlock()
		// Mirror the host event loop's UserFunc dispatch
		// (gui.Update / tui.Run): the lock is held while fn
		// runs so callbacks observe a consistent IDE state.
		if lock != nil {
			lock.Lock()
		}
		fn()
		if lock != nil {
			lock.Unlock()
		}
	}
}

// installDefaultTestScheduler installs a queued, lock-serializing
// scheduler into cfg if the caller didn't supply one (i.e. cfg still
// has the inline default from vte.DefaultConfig). It returns the
// scheduler and the lock so testEx can drain pending callbacks before
// Handle/Draw assertions, mirroring the production event loop where
// scheduled callbacks run under the host's UI lock.
//
// We can't compare function values directly; instead we detect the
// inline default by exercising it: it runs the callback synchronously
// and returns true. A custom scheduler that queues for later won't run
// the probe inline.
func installDefaultTestScheduler(cfg *vte.Config) (*queuedScheduler, sync.Locker) {
	if cfg.ScheduleNextTick == nil {
		scheduler := newQueuedScheduler()
		mu := &sync.Mutex{}
		cfg.ScheduleNextTick = scheduler.ScheduleNextTick
		return scheduler, mu
	}
	var ran atomic.Bool
	cfg.ScheduleNextTick(func() { ran.Store(true) })
	if !ran.Load() {
		// Custom scheduler that queues; trust the caller.
		return nil, nil
	}
	scheduler := newQueuedScheduler()
	cfg.ScheduleNextTick = scheduler.ScheduleNextTick
	// Callbacks queued by scheduler.ScheduleNextTick run only when
	// testEx.Handle/Draw drains the queue (via flushScheduled). The
	// drain is on the test goroutine so there is no concurrent
	// access to serialise — we therefore pass nil for the locker,
	// which avoids deadlocks when Handle is invoked re-entrantly
	// (e.g. echo → publishEvent → testEx.Handle).
	return scheduler, nil
}

type testWorkspaceWithURI struct {
	*testLoader
	uri workspaceapi.URI
}

func (w testWorkspaceWithURI) URI(path string) (workspaceapi.URI, error) {
	return workspace.NewWorkspaceURI(w.uri, path)
}

type readfileCrossWorkspaceLoader struct {
	testWorkspaceWithURI
	foreignURI      workspaceapi.URI
	currentContents string
}

func (w *readfileCrossWorkspaceLoader) Load(
	filePath workspaceapi.URI, buf *cell.Buffer,
	swapDir workspaceapi.URI, readOnly bool,
) (workspace.FlusherCloser, error) {
	if filePath.Equal(w.foreignURI) {
		return nil, workspace.ErrOpenInOtherWorkspace
	}
	if w.currentContents != "" {
		buf.WriteString(w.currentContents)
	}
	return &testFileBuffer{readOnly: readOnly}, nil
}

func (w *readfileCrossWorkspaceLoader) OpenFile(
	path string, flag int, perm os.FileMode,
) (workspaceapi.File, error) {
	expanded, err := workspaceapi.ExpandPathWithURI(path, w.uri)
	if err == nil {
		path = expanded
	}
	return os.OpenFile(path, flag, perm)
}

func (w *readfileCrossWorkspaceLoader) Open(path string) (workspaceapi.File, error) {
	return w.OpenFile(path, os.O_RDONLY, 0)
}

func defCommandKeyBindings() (opts []text.Option) {
	opts = append(opts, text.WithCommandKeyBinding(
		term.KeyComb{Mod: term.ModCtrl, Ch: 'w'}, [][]string{{"tabclose"}}))
	opts = append(opts, text.WithCommandKeyBinding(
		term.KeyComb{Mod: term.ModCtrl, Ch: 'l'}, [][]string{{"tabnext"}}))
	opts = append(opts, text.WithCommandKeyBinding(
		term.KeyComb{Mod: term.ModCtrl, Ch: 'h'}, [][]string{{"tabprevious"}}))
	return
}

func newExForTestingTerminal(
	t *testing.T, workspace workspace.Workspace,
	ed text.Editor,
	emulatorCfg vte.Config,
	publishEvent func(term.Event) bool,
	opts ...text.Option,
) testEx {
	ex := new(ex)
	ex.syncCommandPrompt = true
	svc := storagestub.NewInMemoryService()
	notifications := newWorkspaceNotifications(svc, notificationsConfig(),
		&workspaceManagerMock{workspace: ex})
	uri, err := workspace.URI(".")
	require.NoError(t, err)
	opts = append(opts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	opts = append(opts, defCommandKeyBindings()...)
	scheduler, mu := installDefaultTestScheduler(&emulatorCfg)
	require.NoError(t, ex.init(ed, workspace, svc,
		notifications, uri, emulatorCfg, plugin.DefaultBarConfig(), publishEvent,
		0, clipboard.NewInMemory(), nil, nil, nil, nil, opts...))
	ex.subscribeCommands()
	return testEx{ex: ex, mu: mu, scheduler: scheduler}
}

func newExForTestingWithWorkspace(
	t *testing.T, workspace workspace.Workspace,
	ed text.Editor,
	emulatorCfg vte.Config,
	publishEvent func(term.Event) bool,
	clip clipboard.Register,
	opts ...text.Option,
) testEx {
	ex := new(ex)
	ex.syncCommandPrompt = true
	// user opts override default test opts
	finalOpts := defCommandKeyBindings()
	finalOpts = append(finalOpts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	finalOpts = append(finalOpts, text.WithFloatingNoMaxSize(false))
	finalOpts = append(finalOpts, opts...)

	svc := storagestub.NewInMemoryService()
	notifications := newWorkspaceNotifications(svc, notificationsConfig(),
		&workspaceManagerMock{workspace: ex})

	uri, err := workspace.URI(".")
	require.NoError(t, err)

	scheduler, mu := installDefaultTestScheduler(&emulatorCfg)
	require.NoError(t, ex.init(ed, workspace, svc,
		notifications, uri, emulatorCfg, plugin.DefaultBarConfig(),
		publishEvent, 0, clip, nil, nil, nil, nil, finalOpts...))
	ex.subscribeCommands()
	ex.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
		return newTestVteWithConfig(args), nil
	}
	ex.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return newTestVteWithConfig(args), nil
	}
	ex.pluginWaitTimeout = 1 * time.Second
	return testEx{ex: ex, mu: mu, scheduler: scheduler}
}

func newExForTestingCommandsPreview(
	t *testing.T, workspace workspace.Workspace,
	ed text.Editor,
	emulatorCfg vte.Config,
	publishEvent func(term.Event) bool,
	clip clipboard.Register,
	previews map[string]PreviewFunc,
	opts ...text.Option,
) testEx {
	ex := new(ex)
	ex.syncCommandPrompt = true
	// user opts override default test opts
	finalOpts := defCommandKeyBindings()
	finalOpts = append(finalOpts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	finalOpts = append(finalOpts, text.WithFloatingNoMaxSize(false))
	finalOpts = append(finalOpts, opts...)

	svc := storagestub.NewInMemoryService()
	notifications := newWorkspaceNotifications(svc, notificationsConfig(),
		&workspaceManagerMock{workspace: ex})

	uri, err := workspace.URI(".")
	require.NoError(t, err)

	scheduler, mu := installDefaultTestScheduler(&emulatorCfg)
	require.NoError(t, ex.init(ed, workspace, svc,
		notifications, uri, emulatorCfg, plugin.DefaultBarConfig(),
		publishEvent, 0, clip, nil, previews, nil, nil, finalOpts...))
	ex.subscribeCommands()
	ex.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
		return newTestVteWithConfig(args), nil
	}
	ex.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return newTestVteWithConfig(args), nil
	}
	return testEx{ex: ex, mu: mu, scheduler: scheduler}
}

func newExForTesting(t *testing.T, ed text.Editor, opts ...text.Option) testEx {
	return newExForTestingWithWorkspace(t, &testLoader{}, ed, vte.DefaultConfig(),
		nopPublishEvent, clipboard.NewInMemory(), opts...)
}

func newExForTestingClipboard(
	t *testing.T, ed text.Editor, clip clipboard.Register, opts ...text.Option,
) testEx {
	return newExForTestingWithWorkspace(t, &testLoader{}, ed, vte.DefaultConfig(),
		nopPublishEvent, clip, opts...)
}

// newExForTestingWithStorage is a variant of newExForTestingWithWorkspace
// that accepts a caller-supplied storage service so a second session can
// be booted on the same backing storage (e.g. to exercise workspace
// layout restore flows).
func newExForTestingWithStorage(
	t *testing.T, workspace workspace.Workspace, svc storageapi.Service,
	ed text.Editor,
	emulatorCfg vte.Config,
	publishEvent func(term.Event) bool,
	clip clipboard.Register,
	opts ...text.Option,
) testEx {
	ex := new(ex)
	ex.syncCommandPrompt = true
	finalOpts := defCommandKeyBindings()
	finalOpts = append(finalOpts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	finalOpts = append(finalOpts, text.WithFloatingNoMaxSize(false))
	finalOpts = append(finalOpts, opts...)

	notifications := newWorkspaceNotifications(svc, notificationsConfig(),
		&workspaceManagerMock{workspace: ex})

	uri, err := workspace.URI(".")
	require.NoError(t, err)

	require.NoError(t, ex.init(ed, workspace, svc,
		notifications, uri, emulatorCfg, plugin.DefaultBarConfig(),
		publishEvent, 0, clip, nil, nil, nil, nil, finalOpts...))
	ex.subscribeCommands()
	ex.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
		return newTestVteWithConfig(args), nil
	}
	ex.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return newTestVteWithConfig(args), nil
	}
	ex.pluginWaitTimeout = 1 * time.Second
	return testEx{ex: ex}
}

func TestNewWindow(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":windownew>:windowdefaultsplit h>:windownew>",
			`┌────┌─────────────┐
│    │ changed     │
├────│ split       │
│    │ direction   │
│    │ to          │
│    │ horizontal  │
│    └─────────────┘
│        ││        │
│        ││        │
└────────┘└────────┘`},
		{":winclose>:winclose>aaaaaaa",
			`┌────┌─────────────┐
│    │ changed     │
├────│ split       │
│    │ direction   │
│    │ to          │
│    │ horizontal  │
│    └─────────────┘
│                  │
│                  │
└──────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestFloatingPromptClosePrefersFloatingFocus(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(), text.WithCommandKey(testCommandKey))
	defer b.Close()

	b.Resize(40, 12)
	browserComp := b.ex.comp.Browser()
	mainFocus, err := b.ex.Browser().Focus()
	require.NoError(t, err)
	require.False(t, mainFocus.IsFloating())

	var prompts []browser.Window
	for _, message := range []string{
		"first prompt",
		"second prompt",
		"third prompt",
		"fourth prompt",
		"fifth prompt",
	} {
		prompts = append(prompts, browserComp.Prompt(
			message,
			[]string{yesOpt, noOpt},
			yesNoKeyCombs,
			handler.NopPromptHandler(),
		))
	}

	require.Equal(t, len(prompts), browserComp.FloatingWindows())

	closed := make(map[uint64]bool, len(prompts))
	for i := len(prompts) - 1; i >= 0; i-- {
		focus, err := b.ex.Browser().Focus()
		require.NoError(t, err)
		require.True(t, focus.IsFloating())
		require.False(t, closed[focus.WindowID()])

		content, err := focus.Content()
		require.NoError(t, err)
		_, ok := content.(*handler.Prompt)
		require.True(t, ok)

		closedID := focus.WindowID()
		_, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
		require.True(t, handled)
		closed[closedID] = true
		assert.Equal(t, i, browserComp.FloatingWindows())

		if i == 0 {
			break
		}

		nextFocus, err := b.ex.Browser().Focus()
		require.NoError(t, err)
		assert.True(t, nextFocus.IsFloating())
		assert.False(t, closed[nextFocus.WindowID()])
		assert.NotEqual(t, closedID, nextFocus.WindowID())
	}

	for _, prompt := range prompts {
		assert.True(t, prompt.Closed())
	}

	focus, err := b.ex.Browser().Focus()
	require.NoError(t, err)
	assert.Equal(t, mainFocus.WindowID(), focus.WindowID())
	assert.False(t, focus.IsFloating())
}

type testShellREPLHandler struct{}

func (*testShellREPLHandler) HandleCommand(
	context.Context, repl.Command, repl.ProgressWriter,
) (sdkiterator.Iterator[component.Responsive], error) {
	return sdkiterator.Empty[component.Responsive](), nil
}

func (*testShellREPLHandler) Complete(
	context.Context, string, []string,
) (sdkiterator.Iterator[string], error) {
	return sdkiterator.Empty[string](), nil
}

func (*testShellREPLHandler) Help(
	context.Context, []string,
) (sdkiterator.Iterator[component.Responsive], error) {
	return sdkiterator.Empty[component.Responsive](), nil
}

func TestCommandHistory(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit a.go>:edit wi.go>1234",
			`┌────────━━━━━━━───┐
│o a.go  o wi.go   │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
		{"::::>",
			`┌────────━━━━━━━───┐
│o a.go  o wi.go   │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestCommandHistoryPrompt(t *testing.T) {
	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()
	b.Resize(30, 12)

	// Execute some commands to build history by driving events directly.
	// In the legacy handleTestCase encoding: ':' => command key (Ctrl+\),
	// ' ' => space, '>' => Enter.
	for _, seq := range []string{":echo a>", ":echo b>"} {
		for _, r := range seq {
			switch r {
			case ':':
				b.Handle(term.Event{Mod: term.ModCtrl, Ch: '\\', Type: term.EventKey})
			case ' ':
				b.Handle(term.Event{Key: term.KeySpace, Type: term.EventKey})
			case '>':
				b.Handle(term.Event{Key: term.KeyEnter, Type: term.EventKey})
			default:
				b.Handle(term.Event{Ch: r, Type: term.EventKey})
			}
		}
	}

	// Now open the history prompt programmatically.
	require.Nil(t, b.ex.cmd)
	err := b.ex.openCommandHistoryPrompt(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b.ex.cmd)

	// Close the history prompt.
	require.NotNil(t, b.ex.cmdWin)
	require.NoError(t, b.ex.cmdWin.Close())
	assert.Nil(t, b.ex.cmd)
}

func TestCommandPromptUsesSharedStoragePartition(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor(), text.WithCommandKey(testCommandKey))
	store := &closeCountingPartitionStore{Service: storagestub.NewInMemoryService()}
	b.ex.storage = store

	b.ex.openCommandPrompt()
	require.NotNil(t, b.ex.cmdWin)
	require.NoError(t, b.ex.cmdWin.Close())
	assert.Equal(t, int32(0), store.partitionCloseCount.Load())
	assert.Nil(t, b.ex.cmd)

	b.ex.openCommandPrompt()
	require.NotNil(t, b.ex.cmd)
	require.NoError(t, b.ex.Close())
	assert.Equal(t, int32(0), store.partitionCloseCount.Load())
}

type closeCountingPartitionStore struct {
	storageapi.Service
	partitionCloseCount atomic.Int32
}

func (s *closeCountingPartitionStore) Partition(name string) (storageapi.Service, error) {
	partitioned, err := s.Service.Partition(name)
	if err != nil {
		return nil, err
	}
	return &closeCountingPartition{Service: partitioned, parent: s}, nil
}

type closeCountingPartition struct {
	storageapi.Service
	parent *closeCountingPartitionStore
}

func (s *closeCountingPartition) Partition(name string) (storageapi.Service, error) {
	partitioned, err := s.Service.Partition(name)
	if err != nil {
		return nil, err
	}
	return &closeCountingPartition{Service: partitioned, parent: s.parent}, nil
}

func (s *closeCountingPartition) Close() error {
	s.parent.partitionCloseCount.Add(1)
	return s.Service.Close()
}

func TestCloseOtherWindows(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit hello.go>:windowsplit>:windowsplit>:windowfocus left>:windowfocus left>",
			`┌━━━━━━━━━━────────┐
│o hello.go        │
┌────┐┌─────┐┌─────┤
│AAAA││     ││     │
│AAAA││     ││     │
│AAAA││     ││     │
│AAAA││     ││     │
│AAAA││     ││     │
│AAAA││     ││     │
└────┘└─────┘└─────┘`},
		{":windowcloseall>",
			`┌━━━━━━━━━━────────┐
│o hello.go        │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestCommandAliases(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":todo>1234",
			`┌────────━━━━━━━───┐
│o a.go  o wi.go   │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
		{":bp>",
			`┌━━━━━━────────────┐
│o a.go  o wi.go   │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{":e x.go>",
			`┌────────────━━━━━━┐
│o a.  o wi  o x.go│
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		text.WithCommandKey(testCommandKey),
		text.WithCommandAliases(map[string]text.CommandAlias{
			"todo": text.CommandAlias{Commands: []string{"edit a.go", "edit wi.go"}},
			"e":    text.CommandAlias{Commands: []string{"edit"}},
			"bp":   text.CommandAlias{Commands: []string{"tabnext"}},
		}),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestCommandPluginWait(t *testing.T) {
	t.Run("alias is missing arg", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{"<c-\\\\>todo<enter>",
				`┌────┌─────────────┐
│    │ alias       │
├────│ expects an  │
│    │ argument    │
│    │ at          │
│    │ position 1  │
│    │ ($1)        │
│    └─────────────┘
│                  │
└──────────────────┘`},
		}

		opts := []text.Option{
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
			text.WithCommandKey(testCommandKey),
			text.WithCommandAliases(map[string]text.CommandAlias{
				"todo": {Commands: []string{
					"!! echo '$1'",
					"edit wi.go",
				}},
			}),
		}
		b := newExForTesting(t, texttest.NopEditor(), opts...)
		defer b.Close()

		handlertest.RunHandlerSequence(t, b, 20, 10, cases)
	})

	t.Run("command alias takes too long", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{"<c-\\\\>todo<enter>",
				`┌────┌─────────────┐
│    │ !! sleep    │
├────│ 10:         │
│    │ command     │
│    │ was taking  │
│    │ too long    │
│    │ and so it   │
│    │ was         │
│    │ canceled    │
└────└─────────────┘`},
		}

		opts := []text.Option{
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
			text.WithCommandKey(testCommandKey),
			text.WithCommandAliases(map[string]text.CommandAlias{
				"todo": {Commands: []string{
					"!! sleep 10",
					"edit wi.go",
				}},
			}),
		}
		b := newExForTesting(t, texttest.NopEditor(), opts...)
		defer b.Close()

		handlertest.RunHandlerSequence(t, b, 20, 10, cases)
	})
}

func TestIntegrationEphemeralTerminal(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":! sleep 20>",
			`┌──────────────────────────────────────┐
│                                      │
├──┌────────────────────────────────┐──┤
│  │ ▀          sleep 20          0s│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":tabclose>",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
└──────────────────────────────────────┘`,
		},
		{":! sleep 20>",
			`┌──────────────────────────────────────┐
│                                      │
├──┌────────────────────────────────┐──┤
│  │ ▀          sleep 20          0s│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":windowclose>",
			`┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
└──────────────────────────────────────┘`,
		},
		{":! sleep 20>",
			`┌──────────────────────────────────────┐
│                                      │
├──┌────────────────────────────────┐──┤
│  │ ▀          sleep 20          0s│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":windowcloseall>",
			`┌────────────────────────┌─────────────┐
│                        │ cannot      │
├──┌─────────────────────│ close all   │
│  │ ▐          sleep 20 │ tiled       │
│  │▐                    │ windows     │
│  │                     └─────────────┘
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":noticloseall>:! sh -c 'sleep 20 && echo %'>",
			`┌──────────────────────────────────────┐
│                                      │
├──┌────────────────────────────────┐──┤
│  │ ▀ sh -c "sleep 20 && echo %" 0s│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":windowclose>:windowclose>:edit a>:! sh -c 'sleep 20 && echo %'>",
			`┌──────────────────────────────────────┐
│o a                                   │
├──┌────────────────────────────────┐──┤
│  │ ▀                            0s│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		text.WithEventPublisher(nopPublishEvent),
		text.WithFloatingNoMaxSize(false),
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	ctx := context.Background()
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	defer fileScheme.Close()

	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	b := newExForTestingTerminal(t, workspace,
		modeless.Editor(),
		vte.DefaultConfig(), nopPublishEvent, opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 40, 10, cases)
}

func TestIntegrationCompanionTerminal(t *testing.T) {

	cases := []handlertest.SequenceTestCase{
		{":!>_______",
			`┌──────────────────┐
│                  │
├┌────────────────┐┤
││sh ▐            ││
││                ││
││                ││
││                ││
││                ││
││                ││
└└────────────────┘┘`,
		},
		{"<$:noticloseall>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		},
		{":!>", // no need to wait now, it should pick previous session
			`┌──────────────────┐
│                  │
├┌────────────────┐┤
││sh ▐            ││
││                ││
││                ││
││                ││
││                ││
││                ││
└└────────────────┘┘`,
		},
		{"#:noticloseall>",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		},
		{":!>`", // ` simulates ctrl-v
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		},
		{":!>:windowconverttab companion X>", // ` simulates ctrl-v
			`┌━━━━━━━━━━━───────┐
│X companion       │
├──────────────────┤
│sh ▐              │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		},
		{":!>:!>",
			`┌━━━━━━━━━━━───────┐
│X companion       │
├──────────────────┤
│sh ▐              │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandKeyBinding(term.KeyComb{Mod: term.ModCtrl, Ch: 'h'},
			[][]string{{"windowclose"}}),
		text.WithCommandKeyBinding(term.KeyComb{Mod: term.ModCtrl, Ch: 'l'},
			[][]string{{"tabnext"}}),
		text.WithCommandKeyBinding(term.KeyComb{Mod: term.ModCtrl, Ch: 'v'},
			[][]string{{"tabclose"}}),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		text.WithFloatingNoMaxSize(false),
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	ctx := context.Background()
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	defer fileScheme.Close()

	// do not depend on host shell, which can vary across hosts
	cfg := vte.DefaultConfig()
	cfg.CommandAndArgs = []string{"sh"}

	// do not depend on default shell prompt, as it can change
	// and it does change accross versions
	ps1 := os.Getenv("PS1")
	os.Setenv("PS1", "sh ")
	defer os.Setenv("PS1", ps1)

	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	b := newExForTestingTerminal(t, workspace,
		texttest.NopEditor(), cfg, nopPublishEvent, opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestTerminalWriteOpensSavePrompt(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()

	require.NoError(t, b.ex.terminalnew(context.Background(), "terminal output"))
	require.NoError(t, b.ex.flush(context.Background()))
	require.Nil(t, b.ex.cmd)

	var doc terminalSessionDocument
	require.NoError(t, b.ex.storage.Get(context.Background(),
		terminalSessionDocumentID("terminal-saved"), &doc))
	require.Equal(t, "terminal-saved", doc.Name)
	require.Contains(t, term.CellsToString(doc.Snapshot.ActiveCells()), "terminal output")
}

func TestTerminalNewForwardsShellArgument(t *testing.T) {
	type call struct {
		name string
		fn   func(b *testEx, args ...string) error
	}
	calls := []call{
		{
			name: "terminalnew",
			fn: func(b *testEx, args ...string) error {
				return b.ex.terminalnew(context.Background(), args...)
			},
		},
		{
			name: "terminalnewtab",
			fn: func(b *testEx, args ...string) error {
				return b.ex.terminalnewtab(context.Background(), args...)
			},
		},
		{
			name: "terminalneworsplit",
			fn: func(b *testEx, args ...string) error {
				return b.ex.terminalneworsplit(context.Background(), args...)
			},
		},
	}
	cases := []struct {
		name     string
		args     []string
		expected []string
	}{
		{name: "no args", args: nil, expected: nil},
		{name: "shell only", args: []string{"zsh"}, expected: []string{"zsh"}},
		{name: "shell with flags", args: []string{"zsh", "-i"}, expected: []string{"zsh", "-i"}},
	}
	for _, c := range calls {
		for _, tc := range cases {
			t.Run(c.name+"/"+tc.name, func(t *testing.T) {
				b := newExForTesting(t, texttest.NopEditor())
				defer b.Close()
				var captured []string
				var captureCalled bool
				b.ex.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
					captureCalled = true
					captured = append([]string(nil), args...)
					h := newTestVteWithConfig(args)
					uri, err := workspaceapi.ParseURI(
						fmt.Sprintf("terminaltest:///%s/%s", c.name, tc.name))
					if err != nil {
						return nil, err
					}
					h.uri = uri
					return h, nil
				}
				require.NoError(t, c.fn(&b, tc.args...))
				require.True(t, captureCalled)
				require.Equal(t, tc.expected, captured)
			})
		}
	}
}

func TestEmulatorHandlerUsesReservoir(t *testing.T) {
	// Verifies the fix for RUNE-129: when an ex is configured with a
	// non-zero initialVTECapacity, the warm reservoir is consulted on
	// terminalnew/terminalnewtab even when the call passes shell
	// arguments that match the configured shell.
	type call struct {
		name string
		fn   func(b *testEx, args ...string) error
	}
	calls := []call{
		{
			name: "terminalnew",
			fn: func(b *testEx, args ...string) error {
				return b.ex.terminalnew(context.Background(), args...)
			},
		},
		{
			name: "terminalnewtab",
			fn: func(b *testEx, args ...string) error {
				return b.ex.terminalnewtab(context.Background(), args...)
			},
		},
	}
	cases := []struct {
		name            string
		args            []string
		expectFromPool  bool
		configuredShell []string
	}{
		{name: "no args", args: nil, expectFromPool: true,
			configuredShell: []string{"sh"}},
		{name: "args match shell", args: []string{"sh"}, expectFromPool: true,
			configuredShell: []string{"sh"}},
		{name: "args match shell with flags", args: []string{"sh", "-i"},
			expectFromPool: true, configuredShell: []string{"sh", "-i"}},
		{name: "args differ from shell", args: []string{"echo", "hi"},
			expectFromPool: false, configuredShell: []string{"sh"}},
	}
	for _, c := range calls {
		for _, tc := range cases {
			t.Run(c.name+"/"+tc.name, func(t *testing.T) {
				b := newExForReservoirTesting(t, tc.configuredShell, 1)
				defer b.Close()

				before := b.reservoirGets.Load()
				newCallsBefore := b.newCalls.Load()
				require.NoError(t, c.fn(&b.testEx, tc.args...))
				gotFromPool := b.reservoirGets.Load() > before
				assert.Equal(t, tc.expectFromPool, gotFromPool,
					"reservoir Get count: before=%d after=%d",
					before, b.reservoirGets.Load())
				if tc.expectFromPool {
					// Reservoir served the request: no fresh from-scratch
					// call must have been observed by the closure.
					assert.Equal(t, newCallsBefore, b.newCalls.Load())
				}
			})
		}
	}
}

func TestSetExecutorPreservesReservoirCapacity(t *testing.T) {
	// Verifies the fix for RUNE-129: setExecutor must preserve the
	// configured initial capacity when re-creating the reservoir, and
	// must not race the in-flight initCap by reading Capacity().
	b := newExForReservoirTesting(t, []string{"sh"}, 3)
	defer b.Close()

	require.Equal(t, 3, b.ex.initialReservoirCapacity)

	// Before setExecutor, drain the reservoir's pending initCap so we
	// have a known starting state, then snapshot the configured
	// capacity.
	first := b.ex.reservoir
	require.NotNil(t, first)

	// Trigger setExecutor with the original executor; the new
	// reservoir must come up with the originally configured capacity,
	// regardless of what the (just-closed) old reservoir reports.
	b.ex.setExecutor(b.ex.executor, b.ex.wsExecutor, b.ex.extensionsExecutor)

	require.NotNil(t, b.ex.reservoir)
	assert.NotSame(t, first, b.ex.reservoir)
	assert.Equal(t, 3, b.ex.reservoir.InitialCapacity())
}

type reservoirTestEx struct {
	testEx
	reservoirGets *atomic.Int64
	newCalls      *atomic.Int64
}

func newExForReservoirTesting(
	t *testing.T, shell []string, initialCapacity int,
) reservoirTestEx {
	t.Helper()

	uri, err := workspaceapi.ParseURI("file:///tmp")
	require.NoError(t, err)
	scheme, err := workspacetest.NewNopScheme("file:///tmp")(
		context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	ws := workspace.NewSchemeWorkspace(uri, scheme)

	emCfg := vte.DefaultConfig()
	emCfg.CommandAndArgs = shell

	e := new(ex)
	e.syncCommandPrompt = true
	finalOpts := defCommandKeyBindings()
	finalOpts = append(finalOpts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	finalOpts = append(finalOpts, text.WithFloatingNoMaxSize(false))

	svc := storagestub.NewInMemoryService()
	notifications := newWorkspaceNotifications(svc, notificationsConfig(),
		&workspaceManagerMock{workspace: e})

	require.NoError(t, e.init(texttest.NopEditor(), ws, svc,
		notifications, uri, emCfg, plugin.DefaultBarConfig(),
		nopPublishEvent, initialCapacity, clipboard.NewInMemory(),
		nil, nil, nil, nil, finalOpts...))
	require.NoError(t, e.subscribeCommands())

	// Wrap the closure created by init so we can observe which branch
	// the production logic would have taken. The wrapper mirrors the
	// real routing in ex.init exactly.
	var reservoirGets atomic.Int64
	var newCalls atomic.Int64
	e.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
		if e.reservoir != nil && argsMatchEmulatorShell(args, e.emulatorConfig.CommandAndArgs) {
			reservoirGets.Add(1)
		} else {
			newCalls.Add(1)
		}
		// Substitute a stub VTE so we don't actually start a process.
		return newTestVteWithConfig(args), nil
	}
	e.pluginWaitTimeout = 1 * time.Second

	return reservoirTestEx{
		testEx:        testEx{ex: e},
		reservoirGets: &reservoirGets,
		newCalls:      &newCalls,
	}
}

func TestTerminalWriteUsesNextAvailableName(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()

	require.NoError(t, b.ex.terminalnew(context.Background(), "terminal output"))
	require.NoError(t, b.ex.flush(context.Background()))
	require.NoError(t, b.ex.flush(context.Background()))

	var doc terminalSessionDocument
	require.NoError(t, b.ex.storage.Get(context.Background(),
		terminalSessionDocumentID("terminal-saved-1"), &doc))
	require.Equal(t, "terminal-saved-1", doc.Name)
}

func TestTerminalSaveAndResume(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()

	require.NoError(t, b.ex.terminalnew(context.Background(), "terminal output"))
	require.NoError(t, b.ex.terminalsave(context.Background(), "demo"))
	require.NoError(t, b.ex.terminalresume(context.Background(), "demo"))

	content, err := b.ex.invokeWindow().Content()
	require.NoError(t, err)
	tab, ok := content.(*browser.Tab)
	require.True(t, ok)
	session, ok := tab.Handler().(*testVte)
	require.True(t, ok)
	require.True(t, session.restoredSnapshot)
	require.Contains(t, session.initialCmd, "terminal output")
	cursor, _, _ := session.Cursor()
	require.Equal(t, term.Coordinates{X: 4, Y: 1}, cursor)
	require.Equal(t, 2, session.SeekOffset())
}

func TestOpenTerminalSessionsPersistAndRestore(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()
	var terminalID int
	b.ex.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
		terminalID++
		h := newTestVteWithConfig(args)
		uri, err := workspaceapi.ParseURI(fmt.Sprintf("terminaltest:///%d", terminalID))
		if err != nil {
			return nil, err
		}
		h.uri = uri
		return h, nil
	}

	require.NoError(t, b.ex.terminalnewtab(context.Background(), "first terminal"))
	require.NoError(t, b.ex.terminalnewtab(context.Background(), "second terminal"))
	require.NoError(t, b.ex.saveOpenTerminalSessions(context.Background()))

	var first terminalSessionDocument
	require.NoError(t, b.ex.storage.Get(context.Background(),
		terminalSessionDocumentID(terminalSessionAutoName(b.ex.workspaceURI, 0)), &first))
	require.Equal(t, terminalSessionAutoName(b.ex.workspaceURI, 0), first.Name)
	require.Contains(t, term.CellsToString(first.Snapshot.ActiveCells()), "first terminal")

	var second terminalSessionDocument
	require.NoError(t, b.ex.storage.Get(context.Background(),
		terminalSessionDocumentID(terminalSessionAutoName(b.ex.workspaceURI, 1)), &second))
	require.Equal(t, terminalSessionAutoName(b.ex.workspaceURI, 1), second.Name)
	require.Contains(t, term.CellsToString(second.Snapshot.ActiveCells()), "second terminal")

	restored := newExForTesting(t, texttest.NopEditor())
	defer restored.Close()
	restored.ex.storage = b.ex.storage
	require.NoError(t, restored.ex.restoreOpenTerminalSessions(context.Background(), nil))

	tabs := restored.ex.comp.Tabs()
	require.Len(t, tabs, 2)

	var restoredCommands []string
	for _, tab := range tabs {
		session, ok := tab.Handler().(*testVte)
		require.True(t, ok)
		require.True(t, session.restoredSnapshot)
		restoredCommands = append(restoredCommands, session.initialCmd)
	}
	require.ElementsMatch(t, []string{"first terminal", "second terminal"}, restoredCommands)
}

func TestOpenTerminalSessionsClearedWhenNoTerminalTabsAreOpen(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()

	require.NoError(t, b.ex.storage.Set(context.Background(),
		terminalSessionDocumentID(terminalSessionAutoName(b.ex.workspaceURI, 0)), terminalSessionDocument{
			Kind: terminalSessionDocumentKind,
			Name: terminalSessionAutoName(b.ex.workspaceURI, 0),
		}))

	require.NoError(t, b.ex.saveOpenTerminalSessions(context.Background()))

	var doc terminalSessionDocument
	err := b.ex.storage.Get(context.Background(),
		terminalSessionDocumentID(terminalSessionAutoName(b.ex.workspaceURI, 0)), &doc)
	require.ErrorIs(t, err, storageapi.ErrNotFound)
}

func TestTerminalSessionCompletionSkipsOpenSessionSnapshots(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()

	require.NoError(t, b.ex.storage.Set(context.Background(), terminalSessionDocumentID("manual"),
		terminalSessionDocument{Kind: terminalSessionDocumentKind, Name: "manual"}))
	require.NoError(t, b.ex.storage.Set(context.Background(),
		terminalSessionDocumentID(terminalSessionAutoName(b.ex.workspaceURI, 0)),
		terminalSessionDocument{
			Kind: terminalSessionDocumentKind,
			Name: terminalSessionAutoName(b.ex.workspaceURI, 0),
		}))

	it, _, err := b.ex.completeTerminalSessions(context.Background(), textapi.Command{})
	require.NoError(t, err)
	defer it.Close()

	var names []string
	for {
		name, ok := it.Next(context.Background())
		if !ok {
			break
		}
		names = append(names, name)
	}
	require.NoError(t, it.Err())

	require.Equal(t, []string{"manual"}, names)
}

func TestTerminalSaveCommandRequiresTerminal(t *testing.T) {
	b := newExForTesting(t, texttest.NopEditor())
	defer b.Close()

	err := b.ex.terminalsave(context.Background(), "demo")
	require.EqualError(t, err, "not a terminal")
}

type closeCountingStorage struct {
	storageapi.Service
	partitionCalls int
	closeCalls     int
	partitions     map[string]*closeCountingPartitionStorage
}

func (s *closeCountingStorage) Partition(name string) (storageapi.Service, error) {
	s.partitionCalls++
	if s.partitions == nil {
		s.partitions = make(map[string]*closeCountingPartitionStorage)
	}
	svc, err := s.Service.Partition(name)
	if err != nil {
		return nil, err
	}
	s.partitions[name] = &closeCountingPartitionStorage{Service: svc}
	return s.partitions[name], nil
}

func (s *closeCountingStorage) Close() error {
	s.closeCalls++
	return s.Service.Close()
}

type closeCountingPartitionStorage struct {
	storageapi.Service
	closeCalls int
}

func (s *closeCountingPartitionStorage) Close() error {
	s.closeCalls++
	return s.Service.Close()
}

func TestExUsesSharedIDEStorage(t *testing.T) {
	storage := &closeCountingStorage{Service: storagestub.NewInMemoryService()}
	workspace := &testLoader{}
	ex := new(ex)
	ex.syncCommandPrompt = true
	notifications := newWorkspaceNotifications(storagestub.NewInMemoryService(),
		notificationsConfig(), &workspaceManagerMock{workspace: ex})
	uri, err := workspace.URI(".")
	require.NoError(t, err)
	require.NoError(t, ex.init(texttest.NopEditor(), workspace, storage,
		notifications, uri, vte.DefaultConfig(), plugin.DefaultBarConfig(),
		nopPublishEvent, 0, clipboard.NewInMemory(), nil, nil, nil, nil))

	require.Equal(t, 0, storage.partitionCalls)
	require.Same(t, storage, ex.storage)

	require.NoError(t, ex.Close())
	require.Equal(t, 0, storage.closeCalls)
	require.NoError(t, ex.Close())
	require.Equal(t, 0, storage.closeCalls)
}

func TestFullScreen(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":windowsplit>:edit aaa>:edit bbb>:windowtogglemaximize>",
			`┌───────━━━━━──────┐
│o aaa  o bbb      │
├─┐┌───────────────┐
│ ││AAAAAAAAAAAAAAA│
│ ││AAAAAAAAAAAAAAA│
│ ││AAAAAAAAAAAAAAA│
│ ││AAAAAAAAAAAAAAA│
│ ││AAAAAAAAAAAAAAA│
│ ││AAAAAAAAAAAAAAA│
└─┘└───────────────┘`,
		},
		{":windowmax>",
			`┌───────━━━━━──────┐
│o aaa  o bbb      │
├────────┐┌────────┐
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
└────────┘└────────┘`,
		},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	ctx := context.Background()
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	defer fileScheme.Close()

	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	b := newExForTestingWithWorkspace(t, workspace,
		texttest.NopEditor(), vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestMoveWindowContent(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":windowsplit>:edit aaa>:windowmove left>",
			`┌━━━━━─────────────┐
│o aaa             │
┌────────┐┌────────┤
│AAAAAAAA││        │
│AAAAAAAA││        │
│AAAAAAAA││        │
│AAAAAAAA││        │
│AAAAAAAA││        │
│AAAAAAAA││        │
└────────┘└────────┘`,
		},
		{":windowmove right>",
			`┌━━━━━─────────────┐
│o aaa             │
├────────┐┌────────┐
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        ││AAAAAAAA│
└────────┘└────────┘`,
		},
		{":windowsplit down>:windowfocus up>:windowmove down>",
			`┌━━━━━─────────────┐
│o aaa             │
├────────┐┌────────┤
│        ││        │
│        ││        │
│        │└────────┘
│        │┌────────┐
│        ││AAAAAAAA│
│        ││AAAAAAAA│
└────────┘└────────┘`,
		},
		{":windowmove up>",
			`┌━━━━━─────────────┐
│o aaa             │
├────────┐┌────────┐
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        │└────────┘
│        │┌────────┐
│        ││        │
│        ││        │
└────────┘└────────┘`,
		},
		{":windowmove left>",
			`┌━━━━━─────────────┐
│o aaa             │
┌────────┐┌────────┤
│AAAAAAAA││        │
│AAAAAAAA││        │
│AAAAAAAA│└────────┘
│AAAAAAAA│┌────────┐
│AAAAAAAA││        │
│AAAAAAAA││        │
└────────┘└────────┘`,
		},
		{":terminalnew>:! sh>:windowmove left>:windowmove right>",
			`┌────┌─────────────┐
│o aa│ cannot      │
├───┌│ move        │
│   ││ ▐indow in   │
│   ││ this        │
│   ││ direction   │
│   │└─────────────┘
│   │          │   │
│   │          │   │
└───└──────────┘───┘`,
		},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	ctx := context.Background()
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	defer fileScheme.Close()

	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	b := newExForTestingWithWorkspace(t, workspace,
		texttest.NopEditor(), vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestResizeWindows(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":windowsplit>:edit aaa>:windowresize increase width>",
			`┌━━━━━─────────────┐
│o aaa             │
├───────┐┌─────────┐
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
└───────┘└─────────┘`,
		},
		{":windowsplit down>:windowresize min height>",
			`┌──────────────────┐
│o aaa             │
├───────┐┌─────────┤
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
│       ││AAAAAAAAA│
│       │└─────────┘
│       │┌─────────┐
│       ││         │
└───────┘└─────────┘`,
		},
		{":windowresize max height>",
			`┌──────────────────┐
│o aaa             │
├───────┐┌─────────┤
│       ││AAAAAAAAA│
│       │└─────────┘
│       │┌─────────┐
│       ││         │
│       ││         │
│       ││         │
└───────┘└─────────┘`,
		},
		{":windowresize max width>",
			`┌──────────────────┐
│o aaa             │
├─┐┌───────────────┤
│ ││AAAAAAAAAAAAAAA│
│ │└───────────────┘
│ │┌───────────────┐
│ ││               │
│ ││               │
│ ││               │
└─┘└───────────────┘`,
		},
		{":windowresize min width>",
			`┌──────────────────┐
│o aaa             │
├───────────────┐┌─┤
│               ││A│
│               │└─┘
│               │┌─┐
│               ││ │
│               ││ │
│               ││ │
└───────────────┘└─┘`,
		},
		{":windowresize reset>",
			`┌──────────────────┐
│o aaa             │
├────────┐┌────────┤
│        ││AAAAAAAA│
│        ││AAAAAAAA│
│        │└────────┘
│        │┌────────┐
│        ││        │
│        ││        │
└────────┘└────────┘`,
		},
		{":windowresize decrease height>:windowresize decrease width>",
			`┌──────────────────┐
│o aaa             │
├─────────┐┌───────┤
│         ││AAAAAAA│
│         ││AAAAAAA│
│         ││AAAAAAA│
│         │└───────┘
│         │┌───────┐
│         ││       │
└─────────┘└───────┘`,
		},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	ctx := context.Background()
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	defer fileScheme.Close()

	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	b := newExForTestingWithWorkspace(t, workspace,
		texttest.NopEditor(), vte.DefaultConfig(), nopPublishEvent,
		clipboard.NewInMemory(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestExposedRootNodeIssue(t *testing.T) {
	opts := []text.Option{
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		text.WithCommandKey(testCommandKey),
		text.WithCommandAliases(map[string]text.CommandAlias{
			"boom": text.CommandAlias{
				Commands: []string{
					"windownew",
					"windowdefaultsplit h",
					"windownew",
					"windowdefaultsplit v",
					"windownew",
					"windowfocus left",
					"windowfocus left",
				},
			},
		}),
	}
	cases := []handlertest.SequenceTestCase{
		{":boom>",
			`┌────┌─────────────┐
│    │ changed     │
┌────│ split       │
│    │ direction   │
│    │ to vertical │
│    └─────────────┘
│    ┌─────────────┐
│    │ changed     │
│    │ split       │
└────│ direction   │`,
		},
	}

	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()
	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestEditCompletion(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit re",
			`                    
                    
                    
                    
edit re▐            
retalls             
                    
                    
                    
                    `},
		{":edit dawo",
			`                    
                    
                    
                    
edit dawo▐          
daworg              
                    
                    
                    
                    `},
		{":edit dawo✌re",
			`                    
                    
                    
                    
edit daworg re▐     
retalls             
                    
                    
                    
                    `},
		{":edit dawo⬇✌re",
			`                    
                    
                    
                    
edit daworg re▐     
retalls             
                    
                    
                    
                    `},
		{":edit dawo⬇✌re✌^^^^^^^^^^^",
			`                    
                    
                    
                    
edit dawo▐          
daworg              
                    
                    
                    
                    `},
		{":edit dawo⬇✌re✌^^^^^^^^^^^✌re",
			`                    
                    
                    
                    
edit daworg re▐     
retalls             
                    
                    
                    
                    `},
		{":edit dawo⬇✌re✌^^^^^^^^^^^^^^^^^",
			`                    
                    
                    
                    
edi▐                
edit                
readfile            
reloadfile!         
                    
                    `},
		{":edit dawo⬇✌re✌^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^",
			`                    
                    
                    
                    
                    
                    
                    
                    
                    
                    `},
	}

	fn := func(t *testing.T) tui.Handler {
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
			text.WithWindowManagerConfig(thandler.WindowManagerConfig{
				WindowManagerConfig: tcomponent.WindowManagerConfig{Frame: false}}),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		uri, err := workspaceapi.ParseURI("memory:///")
		require.NoError(t, err)
		scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
		require.NoError(t, err)
		touchTestFile(t, scheme, "daworg")
		touchTestFile(t, scheme, "retalls")
		b := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
			texttest.NopEditor(), vte.DefaultConfig(), nopPublishEvent,
			clipboard.NewInMemory(), opts...)
		t.Cleanup(func() { _ = b.Close() })
		return b
	}

	handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
}

func TestRenameTab(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit hello.go>:tabrename 8berSucks>",
			`┌━━━━━━━━━━━───────┐
│o 8berSucks       │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestEventNone(t *testing.T) {
	t.Run("delegates to underlying handler", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{"🎉edit hello.go>",
				`┌━━━━━━━━━━────────┐
│o hello.go        │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		}

		testCommandKey := term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrl}
		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
		}
		b := newExForTesting(t, texttest.NopEditor(), opts...)
		defer b.Close()

		handlertest.TestHandlerSequence(t, b, 20, 10, cases)

		b.Handle(term.Event{Type: term.EventNone})

		cases = []handlertest.SequenceTestCase{
			{"",
				`┌━━━━━━━━━━────────┐
│o hello.go        │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		}
		handlertest.TestHandlerSequence(t, b, 20, 10, cases)
	})
}

func TestMultipleCommandArgsKeyBindings(t *testing.T) {

	cases := []handlertest.SequenceTestCase{
		{"`",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
└──────────────────┘
┌──────────────────┐
│                  │
└──────────────────┘`,
		},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandKeyBinding(term.KeyComb{Mod: term.ModCtrl, Ch: 'v'},
			[][]string{
				{"windowsplit", "down"},
				{"windowresize", "min", "height"},
			}),
		text.WithCommandOverlayConfig(testCommandOverlayConfig()),
	}

	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 20, 10, cases)
}

func TestTerminalOnFocus(t *testing.T) {
	t.Run("new terminal tab", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("memory:///")
		require.NoError(t, err)
		scheme, _ := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
		testConfig := vte.DefaultConfig()
		ex := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
			texttest.NopEditor(), testConfig, nopPublishEvent, clipboard.NewInMemory())
		tvte := newTestVte()
		ex.newEmulatorHandler = func(args []string) (vtereservoir.VTE, error) {
			assert.Equal(t, "echo bla", strings.Join(args, " "))
			return tvte, nil
		}
		t.Cleanup(func() { _ = ex.Close() })

		ex.terminalnewtab(context.Background(), "echo", "bla")

		require.Len(t, tvte.onFocusChange, 2)
		assert.False(t, tvte.onFocusChange[0])
		assert.True(t, tvte.onFocusChange[1])

		// switch to some other tab, same window
		ex.editFiles(context.Background(), "a")
		require.Len(t, tvte.onFocusChange, 3)
		assert.False(t, tvte.onFocusChange[2])

		// switch back to terminal tab, same window
		ex.tabprevious(context.Background())
		require.Len(t, tvte.onFocusChange, 4)
		assert.True(t, tvte.onFocusChange[3])

		// new window, tab still in screen but not focused
		ex.windownew(context.Background())
		require.Len(t, tvte.onFocusChange, 5)
		assert.False(t, tvte.onFocusChange[4])

		// focus back to tab window
		ex.windowfocus(context.Background(), "left")
		require.Len(t, tvte.onFocusChange, 6)
		assert.True(t, tvte.onFocusChange[5])

		_, handled := ex.Handle(term.Event{Type: term.EventUnfocus})
		require.True(t, handled)
		require.Len(t, tvte.onFocusChange, 7)
		assert.False(t, tvte.onFocusChange[6])

		_, handled = ex.Handle(term.Event{Type: term.EventFocus})
		require.True(t, handled)
		require.Len(t, tvte.onFocusChange, 8)
		assert.True(t, tvte.onFocusChange[7])

		ex.tabclose(context.Background())
		require.Len(t, tvte.onFocusChange, 9)
		assert.False(t, tvte.onFocusChange[8])
	})

	t.Run("companion terminal", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("memory:///")
		require.NoError(t, err)
		scheme, _ := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
		testConfig := vte.DefaultConfig()
		ex := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
			texttest.NopEditor(), testConfig, nopPublishEvent, clipboard.NewInMemory())
		tvte := newTestVte()
		ex.newEmulatorHandler = func([]string) (vtereservoir.VTE, error) {
			return tvte, nil
		}
		t.Cleanup(func() { _ = ex.Close() })
		ex.Resize(100, 100)

		ex.executePlugin(context.Background())
		content, err := ex.invokeWindow().Content()
		require.NoError(t, err)
		_, ok := content.(vtereservoir.VTE)
		require.True(t, ok)

		require.Len(t, tvte.onFocusChange, 2)
		assert.False(t, tvte.onFocusChange[0])
		assert.True(t, tvte.onFocusChange[1])

		// switching from floating to other window should trigger on focus change
		ex.windowfocus(bgctx, "left")
		require.Len(t, tvte.onFocusChange, 3)
		assert.False(t, tvte.onFocusChange[2])

		// switching back to floating should trigger again
		ex.executePlugin(bgctx)
		require.Len(t, tvte.onFocusChange, 4)
		assert.True(t, tvte.onFocusChange[3])

		_, handled := ex.Handle(term.Event{Type: term.EventUnfocus})
		require.True(t, handled)
		require.Len(t, tvte.onFocusChange, 5)
		assert.False(t, tvte.onFocusChange[4])

		_, handled = ex.Handle(term.Event{Type: term.EventFocus})
		require.True(t, handled)
		require.Len(t, tvte.onFocusChange, 6)
		assert.True(t, tvte.onFocusChange[5])

		// indirectly toggle terminal companion
		ex.editFiles(bgctx, "a")
		require.Len(t, tvte.onFocusChange, 7)
		assert.False(t, tvte.onFocusChange[6])
	})

	t.Run("ephemeral terminal", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("memory:///")
		require.NoError(t, err)
		scheme, _ := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
		testConfig := vte.DefaultConfig()
		ex := newExForTestingWithWorkspace(t, workspace.NewSchemeWorkspace(uri, scheme),
			texttest.NopEditor(), testConfig, nopPublishEvent, clipboard.NewInMemory())
		tvte := newTestVte()
		ex.newPluginHandler = func(args ...string) (pluginHandler, error) {
			require.Len(t, args, 2)
			assert.Equal(t, "echo", args[0])
			assert.Equal(t, "bla", args[1])
			return tvte, nil
		}
		t.Cleanup(func() { _ = ex.Close() })
		ex.Resize(100, 100)
		ex.editFiles(bgctx, "a", "b") // have tabs available for later

		ex.executePlugin(bgctx, "echo", "bla")

		require.Len(t, tvte.onFocusChange, 2)
		assert.False(t, tvte.onFocusChange[0])
		assert.True(t, tvte.onFocusChange[1])

		// switching from floating to other window should trigger on focus change
		ex.windowfocus(bgctx, "left")
		require.Len(t, tvte.onFocusChange, 3)
		assert.False(t, tvte.onFocusChange[2])

		// switching back to floating should trigger again
		ex.Handle(term.Event{Type: term.EventMouse, MouseX: 50, MouseY: 50, Key: term.MouseLeft})
		require.Len(t, tvte.onFocusChange, 4)
		assert.True(t, tvte.onFocusChange[3])

		_, handled := ex.Handle(term.Event{Type: term.EventUnfocus})
		require.True(t, handled)
		require.Len(t, tvte.onFocusChange, 5)
		assert.False(t, tvte.onFocusChange[4])

		_, handled = ex.Handle(term.Event{Type: term.EventFocus})
		require.True(t, handled)
		require.Len(t, tvte.onFocusChange, 6)
		assert.True(t, tvte.onFocusChange[5])

		// ephemeral close should trigger another focus event
		ex.tabnext(context.Background())
		require.Len(t, tvte.onFocusChange, 7)
		assert.False(t, tvte.onFocusChange[6])
	})
}

func TestSwitchToTab(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit hello.go>B:edit world.go>",
			`┌────────────━━━━━━━━━━──────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},

		{":tabfocus ",
			`┌────────────━━━━━━━━━━──────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
┌────────────────────────────┐
│tabfocus ▐                  │
│1 hello.go                  │
│2 world.go                  │
└────────────────────────────┘
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},
		{"1>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{"2 world.go>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
│MMMMMMMMMMMMMMMMMMMMMMMMMMMM│
└────────────────────────────┘`},
		{"1 hell>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
│TTTTTTTTTTTTTTTTTTTTTTTTTTTT│
└────────────────────────────┘`},
		{"2 notexist.go>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
└────────────────────────────┘`},
		{":tabfocus 3>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go  o world.go      │
├────────────────────────────┤
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
└────────────────────────────┘`},
		{":tabfocus 0>",
			`┌━━━━━━━━━━────┌─────────────┐
│o hello.go  o │ the first   │
├──────────────│ tab is 1    │
│bbbbbbbbbbbbbb└─────────────┘
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
│bbbbbbbbbbbbbbbbbbbbbbbbbbbb│
└────────────────────────────┘`},
		// command prompt shouldn't complete with history
		{":tabcloseall>:tabfocus ",
			`┌──────────────┌─────────────┐
│              │ the first   │
├──────────────│ tab is 1    │
│              └─────────────┘
│                            │
│                            │
│                            │
┌────────────────────────────┐
│tabfocus ▐                  │
│                            │
│                            │
└────────────────────────────┘
│                            │
│                            │
└────────────────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	b := newExForTesting(t, texttest.NopEditor(), opts...)
	defer b.Close()

	handlertest.TestHandlerSequence(t, b, 30, 15, cases)
}

func TestRunStopTasks(t *testing.T) {
	t.Run("newtask is called with incorrect number of args returns error", func(t *testing.T) {
		b, mu, cleanup := newExForTestingTasks(t)
		defer cleanup()

		mu.Lock()
		defer mu.Unlock()

		require.Error(t, b.newTask(bgctx, "up", "--", "make"))
		require.Error(t, b.newTask(bgctx, "newTask", "left", "make"))
		require.Error(t, b.newTask(bgctx, "--", "make", "test", "things"))
		require.Error(t, b.newTask(bgctx, "up", ".go,.md", "--", "make", "test"))
	})

	t.Run("newtask is called with correct number of args returns no error", func(t *testing.T) {
		b, mu, cleanup := newExForTestingTasks(t)
		defer cleanup()

		mu.Lock()
		defer mu.Unlock()

		require.NoError(t, b.newTask(bgctx, "myTask", "left", "--", "make"))
		require.NoError(t, b.newTask(bgctx, "myTask2", "right", "--", "make", "test", "things"))
		require.NoError(t, b.newTask(bgctx, "myTask3", "left", ".go,.md", "--", "make", "test"))
		require.NoError(t, b.newTask(bgctx, "myTask4", "right", ".go,.md", "--", "make", "test"))
	})

	// left/right alignment combined with up/down is ugly; stick to left/right only
	t.Run("newtask is called with left or right alignment is error", func(t *testing.T) {
		b, mu, cleanup := newExForTestingTasks(t)
		defer cleanup()

		mu.Lock()
		defer mu.Unlock()

		require.Error(t, b.newTask(bgctx, "myTask", "up", "--", "make"))
		require.Error(t, b.newTask(bgctx, "myTask2", "down", "--", "make", "test", "things"))
		require.Error(t, b.newTask(bgctx, "myTask3", "up", ".go,.md", "--", "make", "test"))
		require.Error(t, b.newTask(bgctx, "myTask4", "down", ".go,.md", "--", "make", "test"))
	})

	t.Run("newtask called twice with same task name opens a prompt", func(t *testing.T) {
		b, mu, cleanup := newExForTestingTasks(t)
		defer cleanup()

		mu.Lock()
		defer mu.Unlock()

		require.NoError(t, b.newTask(bgctx, "myTask", "left", "--", "make"))
		require.NoError(t, b.newTask(bgctx, "myTask", "right", "--", "make", "test", "things"))
		_, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
		assert.True(t, handled)
	})

	t.Run("integration", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{":tasknew test right .go -- go test ./...>",
				`┌────────────────────────────┐
│                            │
├───────────────────────────┐┤
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
│                           ││
└───────────────────────────┘┘`},
			{":tasknew build left -- go build ./...>:tasknew assets left -- echo a>:tasknew validateAssets right .html,.js,.css,.ts -- echo b>",
				`┌────────────────────────────┐
│                            │
├┌┌────────────────────────┐┐┤
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
│││                        │││
└└└────────────────────────┘┘┘`},
			{":taskclose ",
				`┌────────────────────────────┐
│                            │
├┌┌────────────────────────┐┐┤
│││                        │││
│││                        │││
│││                        │││
│││                        │││
┌────────────────────────────┐
│taskclose ▐                 │
│assets                      │
│build                       │
│test                        │
│validateAssets              │
└────────────────────────────┘
└└└────────────────────────┘┘┘`},
			{" assets>:taskclose test>:taskclose ",
				`┌────────────────────────────┐
│                            │
├┌──────────────────────────┐┤
││                          ││
││                          ││
││                          ││
││                          ││
┌────────────────────────────┐
│taskclose ▐                 │
│build                       │
│validateAssets              │
└────────────────────────────┘
││                          ││
││                          ││
└└──────────────────────────┘┘`},
			{"<:windowfocus right>",
				`┌────────────────────────────┐
│                            │
├┌───────────────────────────┤
││                           │
││                           │
││                  ┌────────┐
││                  │new     │
││                  │vte:    │
││                  │start   │
││                  │command:│
││                  │ context│
││                  └────────┘
││                           │
││                           │
└└───────────────────────────┘`},
			{":windowclose>:tasknew validateAssets right -- echo a>", // recreate after close
				`┌────────────────────────────┐
│                            │
├┌──────────────────────────┐┤
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
││                          ││
└└──────────────────────────┘┘`},
			{":tasknew validateAssets right -- echo b>", // prompt to replace
				`┌────────────────────────────┐
│                            │
├┌──────────────────────────┐┤
│┌──────────────────────────┐│
││                          ││
││  A task with the name    ││
││  "validateAssets"        ││
││  already exists. Do you  ││
││  want to replace it?     ││
││                          ││
││                          ││
││     Yes          No      ││
│└──────────────────────────┘│
││                          ││
└└──────────────────────────┘┘`},
			{"y:windowfocus right>",
				`┌────────────────────────────┐
│                            │
├┌───────────────────────────┤
││                           │
││                           │
││                  ┌────────┐
││                  │new     │
││                  │vte:    │
││                  │start   │
││                  │command:│
││                  │ context│
││                  └────────┘
││                           │
││                           │
└└───────────────────────────┘`},
			{":windowconverttab asset x>",
				`┌━━━━━━━─────────────────────┐
│x asset                     │
├┌───────────────────────────┤
││                           │
││                           │
││                           │
││                           │
││ new vte: start command:   │
││ context canceled          │
││                           │
││                           │
││                           │
││                           │
││                           │
└└───────────────────────────┘`},
			{":edit abc>:write>",
				`┌─────────━━━━━──────────────┐
│x asset  o abc              │
├┌───────────────────────────┤
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
││AAAAAAAAAAAAAAAAAAAAAAAAAAA│
└└───────────────────────────┘`},
			{":tabprevious>",
				`┌━━━━━━━─────────────────────┐
│x asset  o abc              │
├┌───────────────────────────┤
││                           │
││                           │
││                           │
││                           │
││ new vte: start command:   │
││ context canceled          │
││                           │
││                           │
││                           │
││                           │
││                           │
└└───────────────────────────┘`},
			{":windowfocus left>:windowconverttab build X>",
				`┌────────────────━━━━━━━─────┐
│x asset  o abc  X build     │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│  new vte: start command:   │
│  context canceled          │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`},
			{":tabprevious>:tabprevious>:tabprevious>",
				`┌────────────────━━━━━━━─────┐
│x asset  o abc  X build     │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│  new vte: start command:   │
│  context canceled          │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`},
			{":windowsplit right>:tabnext>:tabnext>",
				`┌─────────━━━━━──────────────┐
│x asset  o abc  X build     │
├─────────────┐┌─────────────┐
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│  new vte:   ││AAAAAAAAAAAAA│
│  start      ││AAAAAAAAAAAAA│
│  command:   ││AAAAAAAAAAAAA│
│  context    ││AAAAAAAAAAAAA│
│  canceled   ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
			{":taskclose validateAssets>",
				`┌━━━━━───────────────────────┐
│o abc  X build              │
├─────────────┐┌─────────────┐
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│  new vte:   ││AAAAAAAAAAAAA│
│  start      ││AAAAAAAAAAAAA│
│  command:   ││AAAAAAAAAAAAA│
│  context    ││AAAAAAAAAAAAA│
│  canceled   ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
			{":taskclose build>",
				`┌━━━━━───────────────────────┐
│o abc                       │
├─────────────┐┌─────────────┐
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
			{":windowfocus left>:tasknewtab tests -- go test ./...>",
				`┌───────━━━━━━━──────────────┐
│o abc  8 tests              │
┌─────────────┐┌─────────────┤
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│  new vte:   ││AAAAAAAAAAAAA│
│  start      ││AAAAAAAAAAAAA│
│  command:   ││AAAAAAAAAAAAA│
│  context    ││AAAAAAAAAAAAA│
│  canceled   ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
			{":windowfocus right>:tasknewtab build -- go build ./...>",
				`┌────────────────━━━━━━━─────┐
│o abc  8 tests  8 build     │
├─────────────┐┌─────────────┐
│             ││             │
│             ││             │
│             ││             │
│  new vte:   ││  new vte:   │
│  start      ││  start      │
│  command:   ││  command:   │
│  context    ││  context    │
│  canceled   ││  canceled   │
│             ││             │
│             ││             │
│             ││             │
└─────────────┘└─────────────┘`},
			{":tasknewtab build -- go build ./...>",
				`┌────────────────────────────┐
│o abc  8 tests  8 build     │
├─────────────┐┌─────────────┤
┌────────────────────────────┐
│                            │
│  A task with the name      │
│  "build" already exists.   │
│  Do you want to replace    │
│  it?                       │
│                            │
│                            │
│      Yes          No       │
└────────────────────────────┘
│             ││             │
└─────────────┘└─────────────┘`},
			{"y",
				`┌────────────────━━━━━━━─────┐
│o abc  8 tests  8 build     │
├─────────────┐┌─────────────┐
│             ││             │
│             ││             │
│             ││             │
│  new vte:   ││  new vte:   │
│  start      ││  start      │
│  command:   ││  command:   │
│  context    ││  context    │
│  canceled   ││  canceled   │
│             ││             │
│             ││             │
│             ││             │
└─────────────┘└─────────────┘`},
			{":tabclose>",
				`┌━━━━━───────────────────────┐
│o abc  8 tests              │
├─────────────┐┌─────────────┐
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│  new vte:   ││AAAAAAAAAAAAA│
│  start      ││AAAAAAAAAAAAA│
│  command:   ││AAAAAAAAAAAAA│
│  context    ││AAAAAAAAAAAAA│
│  canceled   ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
			{":taskclose tests>",
				`┌━━━━━───────────────────────┐
│o abc                       │
├─────────────┐┌─────────────┐
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
│             ││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
		}

		defaultConvertTabIcon = '8'
		e, mu, cleanup := newExForTestingTasks(t)
		handlertest.TestHandlerSequence(t, handler.Sync(mu, e), 30, 15, cases)
		cleanup()
	})
}

func TestEcho(t *testing.T) {
	t.Run("events get dispatched", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{`:edit hello.go>:echo 01234>`,
				`┌━━━━━━━━━━──────────────────┐
│o hello.go                  │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},
		}

		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
		}
		var e testEx
		var i int
		publishEvent := func(ev term.Event) bool {
			if ev.Type == term.EventInterrupt {
				return true
			}
			assert.Equal(t, string(ev.Ch), strconv.Itoa(i))
			i++
			return true
		}

		e = newExForTestingWithWorkspace(t, &testLoader{},
			texttest.NopEditor(), vte.DefaultConfig(),
			publishEvent, clipboard.NewInMemory(), opts...)
		defer e.Close()

		handlertest.TestHandlerSequence(t, e, 30, 15, cases)
	})

	t.Run("recursive register expansion returns error before publishing events", func(t *testing.T) {
		clip := registerset.New(clipboard.NewInMemory())
		require.NoError(t, clip.Copy("a", clipboard.Data{Text: "{register}a"}))

		published := 0
		e := newExForTestingWithWorkspace(t, &testLoader{},
			texttest.NopEditor(), vte.DefaultConfig(),
			func(ev term.Event) bool {
				published++
				return true
			}, clip,
			text.WithCommandKey(testCommandKey),
		)
		defer e.Close()

		err := e.echo(context.Background(), "{register}a")
		require.EqualError(t, err, `expand register "a": recursive register expansion detected: a -> a`)
		require.Zero(t, published)
	})

	t.Run("mutually recursive register expansion returns error before publishing events", func(t *testing.T) {
		clip := registerset.New(clipboard.NewInMemory())
		require.NoError(t, clip.Copy("a", clipboard.Data{Text: "{register}b"}))
		require.NoError(t, clip.Copy("b", clipboard.Data{Text: "{register}a"}))

		published := 0
		e := newExForTestingWithWorkspace(t, &testLoader{},
			texttest.NopEditor(), vte.DefaultConfig(),
			func(ev term.Event) bool {
				published++
				return true
			}, clip,
			text.WithCommandKey(testCommandKey),
		)
		defer e.Close()

		err := e.echo(context.Background(), "{register}a")
		require.EqualError(t, err, `expand register "a": expand register "b": recursive register expansion detected: a -> b -> a`)
		require.Zero(t, published)
	})

	t.Run("{prompt} instruction", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{`:echo {prompt}edit\<space\>hello.go\<enter\>>`,
				`┌━━━━━━━━━━──────────────────┐
│o hello.go                  │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},
		}

		opts := []text.Option{
			text.WithCommandKey(testCommandKey),
		}
		var e testEx
		publishEvent := func(ev term.Event) bool {
			if ev.Type == term.EventInterrupt {
				return true
			}
			e.Handle(ev)
			return true
		}

		e = newExForTestingWithWorkspace(t, &testLoader{},
			texttest.NopEditor(), vte.DefaultConfig(),
			publishEvent, clipboard.NewInMemory(), opts...)
		defer e.Close()

		handlertest.TestHandlerSequence(t, e, 30, 15, cases)
	})
}

// TestExEchoMultipleArgs covers the per-argv-element parsing of the
// `echo` ex command: each argument is parsed independently with
// parseEchoKeys, and resulting key sequences are concatenated. Used
// to be a join-with-space + parse, which spuriously injected a literal
// <space> key between logically independent argv elements.
func TestExEchoMultipleArgs(t *testing.T) {
	var published []term.Event
	publishEvent := func(ev term.Event) bool {
		if ev.Type == term.EventInterrupt {
			return true
		}
		published = append(published, ev)
		return true
	}
	e := newExForTestingWithWorkspace(t, &testLoader{},
		texttest.NopEditor(), vte.DefaultConfig(),
		publishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(testCommandKey),
	)
	defer e.Close()

	require.NoError(t, e.echo(context.Background(), "<space>", "<enter>"))
	require.Len(t, published, 2)
	assert.Equal(t, term.KeySpace, published[0].Key)
	assert.Equal(t, term.KeyEnter, published[1].Key)
}

// TestSearchAstAliasesFromRuneStar is the RUNE-123 regression covering
// the actual `searchfunc` / `searchvar` / `searchtype` aliases shipped
// in cmd/rune/rune.star. Their bodies use both echo's `<…>` key syntax
// and the `|` separator inside the searchast query — all of which the
// previous shell-style Layer 1 tokenizer would split incorrectly,
// causing the recursive `echo` dispatch to fail with an
// `invalid syntax` error notification. With the layered tokenizer
// each alias body survives unchanged as a single argv element, and
// parseEchoKeys accepts it.
func TestSearchAstAliasesFromRuneStar(t *testing.T) {
	// Source of truth: cmd/rune/rune.star. Keep these in sync with
	// the strings declared there.
	const (
		searchfuncBody = `echo {prompt}searchast<space>locals.scm<space>local.definition.method|local.definition.function<enter>`
		searchvarBody  = `echo {prompt}searchast<space>locals.scm<space>local.definition.var<enter>`
		searchtypeBody = `echo {prompt}searchast<space>locals.scm<space>local.definition.type<enter>`
	)
	cases := []struct {
		alias string
		body  string
	}{
		{"searchfunc", searchfuncBody},
		{"searchvar", searchvarBody},
		{"searchtype", searchtypeBody},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			publishEvent := func(ev term.Event) bool { return true }
			e := newExForTestingWithWorkspace(t, &testLoader{},
				texttest.NopEditor(), vte.DefaultConfig(),
				publishEvent, clipboard.NewInMemory(),
				text.WithCommandKey(testCommandKey),
				text.WithCommandAliases(map[string]text.CommandAlias{
					tc.alias: {Commands: []string{tc.body}},
				}),
			)
			defer e.Close()

			require.NoError(t, e.dispatchCommand(tc.alias))
		})
	}
}

// captureLoader is a testLoader that records the workspaceapi.Cmd values
// passed to StartCommand so tests can assert what argv reaches the
// VTE-bound `!!` plugin executor (post reshellQuoteArgs + shell.Fields).
type captureLoader struct {
	testLoader
	startErr error
	cmds     []workspaceapi.Cmd
}

func (c *captureLoader) StartCommand(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	c.cmds = append(c.cmds, cmd)
	return 0, c.startErr
}

// TestWorktreeNewAliasFromRuneStarPreservesEnvExpansion is a regression
// for the RUNE_DATADIR worktree disaster: invoking the `worktreenew`
// alias from cmd/rune/rune.star wrapped substituted positional args
// (e.g. "$RUNE_DATADIR/worktrees/$1") in single quotes when re-handing
// them to the VTE-backed `!!` plugin. shell.Fields then treated the
// value as a literal, so `git worktree add` saw "$RUNE_DATADIR" rather
// than the expanded path, and a literal "$RUNE_DATADIR" directory was
// created inside the repository.
//
// The test asserts the round-trip invariant directly on
// reshellQuoteArgs: after re-quoting and re-tokenising via shell.Fields
// (the same call the VTE pipeline performs), every $VAR survives env
// expansion. Sibling subtests cover the alias body in isolation by
// configuring it on a freshly built ex and verifying the args reaching
// the (intercepted) downstream commands match what the user typed.
func TestWorktreeNewAliasFromRuneStarPreservesEnvExpansion(t *testing.T) {
	// Source of truth: cmd/rune/rune.star. Keep these in sync with the
	// strings declared there.
	const (
		runeStarPluginCmd       = `!! git worktree add "$RUNE_DATADIR/worktrees/$1" -b $1`
		runeStarWorkspaceCmd    = `workspacenew $RUNE_DATADIR/worktrees/$1`
		runeStarWorkspaceRename = `workspacerename $1`
		worktreeName            = "tabs-refresh-gpt"
	)
	dataDir := t.TempDir()
	t.Setenv("RUNE_DATADIR", dataDir)

	t.Run("reshellQuoteArgs preserves $VAR through shell.Fields", func(t *testing.T) {
		// These are exactly the args the recursive `!!` dispatch
		// receives after Layer 1 strips the surrounding double quotes
		// from the alias body and replacePositionalArgs has substituted
		// $1.
		args := []string{
			"git", "worktree", "add",
			"$RUNE_DATADIR/worktrees/" + worktreeName,
			"-b", worktreeName,
		}
		quoted := reshellQuoteArgs(args)
		fields, err := shell.Fields(strings.Join(quoted, " "), os.Getenv)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"git", "worktree", "add",
			filepath.Join(dataDir, "worktrees", worktreeName),
			"-b", worktreeName,
		}, fields,
			"$RUNE_DATADIR must survive reshellQuoteArgs round-trip; "+
				"single-quoting would suppress shell.Fields env expansion "+
				"and create a literal $RUNE_DATADIR directory")
	})

	t.Run("worktreenew dispatch reaches StartCommand with expanded path", func(t *testing.T) {
		// Capture StartCommand so we observe the argv reaching the
		// VTE's executor — i.e. after reshellQuoteArgs + shell.Fields,
		// the same place a real `!!` invocation would land. A non-nil
		// return makes executePluginWait fail fast.
		captured := &captureLoader{
			testLoader: testLoader{},
			startErr:   errors.New("captured-start-command"),
		}
		publishEvent := func(ev term.Event) bool { return true }
		e := newExForTestingWithWorkspace(t, captured,
			texttest.NopEditor(), vte.DefaultConfig(),
			publishEvent, clipboard.NewInMemory(),
			text.WithCommandKey(testCommandKey),
			text.WithCommandAliases(map[string]text.CommandAlias{
				"worktreenew": {Commands: []string{
					runeStarPluginCmd,
					runeStarWorkspaceCmd,
					runeStarWorkspaceRename,
				}},
			}),
		)
		defer e.Close()

		// dispatchCommand returns the StartCommand failure wrapped
		// inside the alias error chain; that's expected. The chain
		// aborts after the first line, so we only assert on the args
		// reaching the VTE executor for `!!`.
		_ = e.dispatchCommand("worktreenew", worktreeName)

		require.NotEmpty(t, captured.cmds,
			"!! must have reached the executor's StartCommand")
		got := captured.cmds[0]
		argv := append([]string{got.Path}, got.Args...)
		assert.Equal(t, []string{
			"git", "worktree", "add",
			filepath.Join(dataDir, "worktrees", worktreeName),
			"-b", worktreeName,
		}, argv,
			"$RUNE_DATADIR must be expanded by the VTE's shell.Fields; "+
				"single-quoting would leave a literal $RUNE_DATADIR in argv "+
				"and create a directory of that literal name")
	})
}

func TestMoveTabs(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit caliu.go>:edit boira.go>b",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":tabmove 1>",
			`┌━━━━━━━━━━──────────────────┐
│o boira.go  o caliu.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":tabmove 99>",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":tabmove left>",
			`┌━━━━━━━━━━──────────────────┐
│o boira.go  o caliu.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":tabmove left>",
			`┌━━━━━━━━━━────┌─────────────┐
│o boira.go  o │ tab is      │
├──────────────│ already at  │
│BBBBBBBBBBBBBB│ the start   │
│BBBBBBBBBBBBBB│ of the list │
│BBBBBBBBBBBBBB└─────────────┘
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":notificationCloseAll>:tabmove right>",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":tabmove right>",
			`┌────────────━━┌─────────────┐
│o caliu.go  o │ tab is      │
├──────────────│ already at  │
│BBBBBBBBBBBBBB│ the end of  │
│BBBBBBBBBBBBBB│ the list    │
│BBBBBBBBBBBBBB└─────────────┘
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":notificationCloseAll>:windowsplit right>:tabprevious>",
			`┌━━━━━━━━━━──────────────────┐
│o caliu.go  o boira.go      │
├─────────────┐┌─────────────┐
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
		{":tabmove right>",
			`┌────────────━━━━━━━━━━──────┐
│o boira.go  o caliu.go      │
├─────────────┐┌─────────────┐
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
		{":windowfocus left>:tabmove right>",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
┌─────────────┐┌─────────────┤
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
│BBBBBBBBBBBBB││AAAAAAAAAAAAA│
└─────────────┘└─────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	e := newExForTestingWithWorkspace(t, &testLoader{},
		texttest.NopEditor(), vte.DefaultConfig(),
		nopPublishEvent, clipboard.NewInMemory(), opts...)
	defer e.Close()
	handlertest.TestHandlerSequence(t, e, 30, 15, cases)
}

func TestViewForceWrite(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":view caliu.go>b",
			`┌━━━━━━━━━━──────────────────┐
│o caliu.go                  │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":write>",
			`┌━━━━━━━━━━────┌─────────────┐
│o caliu.go    │ save        │
├──────────────│ 'caliu.go': │
│BBBBBBBBBBBBBB│  file is    │
│BBBBBBBBBBBBBB│ not         │
│BBBBBBBBBBBBBB│ writable    │
│BBBBBBBBBBBBBB└─────────────┘
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":notificationCloseAll>:write!>",
			`┌━━━━━━━━━━──────────────────┐
│o caliu.go                  │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":write>",
			`┌━━━━━━━━━━──────────────────┐
│o caliu.go                  │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	e := newExForTestingWithWorkspace(t, &testLoader{},
		texttest.NopEditor(), vte.DefaultConfig(),
		nopPublishEvent, clipboard.NewInMemory(), opts...)
	defer e.Close()
	handlertest.TestHandlerSequence(t, e, 30, 15, cases)
}

func TestViewForceWriteAll(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":view caliu.go>:view boira.go>b",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":writeall>",
			`┌────────────━━┌─────────────┐
│o caliu.go  o │ save        │
├──────────────│ 'boira.go': │
│BBBBBBBBBBBBBB│  file is    │
│BBBBBBBBBBBBBB│ not         │
│BBBBBBBBBBBBBB│ writable    │
│BBBBBBBBBBBBBB└─────────────┘
│BBBBBBBBBBBBBB┌─────────────┐
│BBBBBBBBBBBBBB│ save        │
│BBBBBBBBBBBBBB│ 'caliu.go': │
│BBBBBBBBBBBBBB│  file is    │
│BBBBBBBBBBBBBB│ not         │
│BBBBBBBBBBBBBB│ writable    │
│BBBBBBBBBBBBBB└─────────────┘
└────────────────────────────┘`},
		{":notificationCloseAll>:writeall!>",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":writeall>",
			`┌────────────━━━━━━━━━━──────┐
│o caliu.go  o boira.go      │
├────────────────────────────┤
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
	}
	e := newExForTestingWithWorkspace(t, &testLoader{},
		texttest.NopEditor(), vte.DefaultConfig(),
		nopPublishEvent, clipboard.NewInMemory(), opts...)
	defer e.Close()
	handlertest.TestHandlerSequence(t, e, 30, 15, cases)
}

func TestIntegrationUndoAfterOpen(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit<space>enm.go<enter>u",
			`┌━━━━━━━━────────────────────┐
│o enm.go                    │
├────────────────────────────┤
│▐                           │
└────────────────────────────┘`},
	}

	tempDir, err := os.MkdirTemp("", "")
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	ctx := context.Background()
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	e := newExForTestingWithWorkspace(t, workspace, vi.Editor(),
		vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(),
		text.WithCommandKey(term.KeyComb{Ch: ':'}),
	)
	handlertest.RunHandlerSequence(t, e, 30, 5, cases)
	require.NoError(t, e.Close())
	require.NoError(t, workspace.Close())
	require.NoError(t, fileScheme.Close())
}

func TestCopyPath(t *testing.T) {
	tsuite := []struct {
		name   string
		cmd    string
		expect func() string
	}{
		{
			name:   "relative",
			cmd:    ":tabcopypath",
			expect: func() string { return "hello.go" },
		},
		{
			name: "absolute",
			cmd:  ":tabcopypath absolute",
			expect: func() string {
				absPath, _ := workspaceapi.ExpandPath(
					"hello.go", user.Current, os.Getwd)
				return absPath // e.g. /Users/ramon/Devel/go-tui/hello.go
			},
		},
	}
	for _, tcase := range tsuite {
		t.Run(tcase.name, func(t *testing.T) {
			cases := []handlertest.SequenceTestCase{
				{fmt.Sprintf(":edit hello.go>%s>", tcase.cmd),
					`┌━━━━┌─────────────┐
│o he│ file path   │
├────│ copied to   │
│AAAA│ clipboard   │
│AAAA└─────────────┘
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
			}

			clip := clipboard.NewInMemory()
			opts := []text.Option{text.WithCommandKey(testCommandKey)}
			e := newExForTestingClipboard(t, texttest.NopEditor(), clip, opts...)
			defer e.Close()
			handlertest.TestHandlerSequence(t, e, 20, 10, cases)

			data, err := clip.Paste(clipboard.DefaultRegisterID)
			require.NoError(t, err)
			assert.Equal(t, tcase.expect(), data.Text)
		})
	}

}

func TestCopyPathNestedWorkspaceRelative(t *testing.T) {
	clip := clipboard.NewInMemory()
	e, fileScheme, tempDir := newExForTestingFileWorkspace(t, clip)
	defer func() { require.NoError(t, e.Close()) }()
	defer func() { require.NoError(t, fileScheme.Close()) }()

	relPath := filepath.Join("nested", "hello.go")
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, relPath), []byte("hello\n"), 0o644))

	uri, err := e.workspace.URI(relPath)
	require.NoError(t, err)
	_, err = e.editFileURI(uri, e.invokeWindow(), false)
	require.NoError(t, err)
	e.Resize(20, 10)
	require.NoError(t, e.tabcopypath(context.Background()))

	data, err := clip.Paste(clipboard.DefaultRegisterID)
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(relPath), data.Text)
}

func TestCopyLocation(t *testing.T) {
	ts := []struct {
		name   string
		cmd    string
		expect func(tempDir, relPath string) string
	}{
		{
			name: "relative",
			cmd:  ":tabcopylocation",
			expect: func(tempDir, relPath string) string {
				return filepath.ToSlash(relPath) + ":2"
			},
		},
		{
			name: "absolute",
			cmd:  ":tabcopylocation absolute",
			expect: func(tempDir, relPath string) string {
				return filepath.Join(tempDir, relPath) + ":2"
			},
		},
	}

	for _, tc := range ts {
		t.Run(tc.name, func(t *testing.T) {
			clip := clipboard.NewInMemory()
			e, fileScheme, tempDir := newExForTestingFileWorkspace(t, clip)
			defer func() { require.NoError(t, e.Close()) }()
			defer func() { require.NoError(t, fileScheme.Close()) }()

			relPath := filepath.Join("nested", "hello.go")
			require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "nested"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(tempDir, relPath), []byte("one\ntwo\n"), 0o644))

			uri, err := e.workspace.URI(relPath)
			require.NoError(t, err)
			_, err = e.editFileURI(uri, e.invokeWindow(), false)
			require.NoError(t, err)
			e.Resize(20, 10)
			require.NoError(t, e.moveFocusCursor(1))
			args := strings.TrimPrefix(tc.cmd, ":tabcopylocation")
			require.NoError(t, e.tabcopylocation(context.Background(), strings.Fields(args)...))

			data, err := clip.Paste(clipboard.DefaultRegisterID)
			require.NoError(t, err)
			assert.Equal(t, tc.expect(tempDir, relPath), data.Text)
		})
	}
}

func newExForTestingFileWorkspace(
	t *testing.T, clip clipboard.Register,
) (testEx, schemeapi.Scheme, string) {
	t.Helper()
	tempDir := t.TempDir()
	ctx := context.Background()
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	e := newExForTestingWithWorkspace(t, workspace, vi.Editor(),
		vte.DefaultConfig(), nopPublishEvent, clip,
		text.WithCommandKey(testCommandKey),
	)
	return e, fileScheme, tempDir
}

func TestCopyToClipboard(t *testing.T) {
	clip := clipboard.NewInMemory()
	testCopyToClipboard(t, clip, func(ed text.Editor, opts ...text.Option) (
		tui.Handler, browser.Browser, error,
	) {
		b := newExForTestingClipboard(t, texttest.NopEditor(), clip, opts...)
		defer b.Close()
		return b, b.Browser(), nil
	})
}

func testCopyToClipboard(
	t *testing.T, clip clipboard.Register, constructor browserConstructor,
) {
	cases := []handlertest.SequenceTestCase{
		{":edit hello.go>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go                  │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},
		{":clipboardpaste>",
			`┌━━━━━━━━━━────┌─────────────┐
│o hello.go    │ nothing to  │
├──────────────│ paste       │
│AAAAAAAAAAAAAA└─────────────┘
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},
		{":noticloseall>:clipboardcopy>",
			`┌━━━━━━━━━━────┌─────────────┐
│o hello.go    │ copied to   │
├──────────────│ clipboard   │
│AAAAAAAAAAAAAA└─────────────┘
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`},
		{":noticloseall>:clipboardpaste>",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go                  │
├────────────────────────────┤
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
└────────────────────────────┘`},
		{":####",
			`┌━━━━━━━━━━──────────────────┐
│o hello.go                  │
├────────────────────────────┤
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
┌────────────────────────────┐
│AAAA▐                       │
│                            │
│                            │
└────────────────────────────┘
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
│DDDDDDDDDDDDDDDDDDDDDDDDDDDD│
└────────────────────────────┘`},
	}

	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithCommandKeyBinding(term.KeyComb{Mod: term.ModCtrl, Ch: 'h'},
			[][]string{{cmdClipboardPaste}}),
	}
	bh, _, err := constructor(texttest.NopEditor(), opts...)
	require.NoError(t, err)

	handlertest.TestHandlerSequence(t, bh, 30, 15, cases)

	data, err := clip.Paste(clipboard.DefaultRegisterID)
	require.NoError(t, err)
	assert.Equal(t, "A", data.Text)
}

func notificationsConfig() notifications.Config {
	ret := defaultNotificationsConfig()
	ret.Width = 15
	ret.ProgressBar = false // deterministic tests
	return ret
}

func touchTestFile(t *testing.T, scheme schemeapi.Scheme, name string) {
	f, werr := scheme.OpenFile(name, os.O_CREATE, 0666)
	require.Nil(t, werr)
	require.NoError(t, f.Sync())
}

func testCommandOverlayConfig() text.CommandOverlayConfig {
	return text.CommandOverlayConfig{
		ShowManualAfter:  1 * time.Hour,
		ShowProgressHint: false,
	}
}

type testVte struct {
	component.String

	initialCmd       string
	calledClose      bool
	defAttr          term.Attributes
	onFocusChange    []bool
	restoredSnapshot bool
	cursor           term.Coordinates
	seekOffset       int

	isComplete bool
	uri        workspaceapi.URI
	title      string
}

func newTestVte() *testVte {
	return newTestVteWithConfig(nil)
}

func newTestVteWithConfig(initialCmd []string) *testVte {
	ret := new(testVte)
	ret.initialCmd = strings.Join(initialCmd, " ")
	ret.String = component.NewString(ret.initialCmd)
	return ret
}

func (t *testVte) Handle(ev term.Event) (bool, bool) {
	return false, false
}

func (t *testVte) SeekUp() bool {
	return false
}

func (t *testVte) SeekDown() bool {
	return false
}

func (t *testVte) SeekOffset() int {
	return t.seekOffset
}

func (t *testVte) MaxSeekOffset() int {
	return 0
}

func (v *testVte) UsedAlternateBuffer() bool {
	return false
}

func (v *testVte) ClearPrimaryBuffer() bool {
	return true
}

func (v *testVte) Snapshot() (vte.Snapshot, error) {
	title := v.title
	if title == "" {
		title = "terminal"
	}
	return vte.Snapshot{
		Version: 1,
		Title:   title,
		Width:   10,
		Height:  10,
		ScrollOffset: term.Coordinates{
			Y: 2,
		},
		Primary: vte.ScreenSnapshot{
			Cells:  term.StringToCells(v.initialCmd),
			Cursor: term.Coordinates{X: 4, Y: 1},
		},
	}, nil
}

func (v *testVte) RestoreFromSnapshot(snapshot vte.Snapshot) error {
	v.restoredSnapshot = true
	v.initialCmd = term.CellsToString(snapshot.ActiveCells())
	v.String = component.NewString(v.initialCmd)
	v.cursor = snapshot.Primary.Cursor
	v.seekOffset = snapshot.ScrollOffset.Y
	return nil
}

func (t *testVte) Cursor() (ret term.Coordinates, style term.CursorStyle, show bool) {
	show = true
	if t.restoredSnapshot {
		ret = t.cursor
		return
	}
	ret = term.Coordinates{X: len(t.initialCmd)}
	return
}

func (t *testVte) Selection() (string, bool) {
	return "", false
}

func (t *testVte) Dimensions() (int, int) {
	return 10, 10
}

func (v *testVte) Close() error {
	if v.calledClose {
		return errors.New("called close twice")
	}
	v.calledClose = true
	return nil
}

func (v *testVte) OnFocusChange(inFocus bool) {
	v.onFocusChange = append(v.onFocusChange, inFocus)
}

func (v *testVte) SetDefaultAttributes(attr term.Attributes) {
	v.defAttr = attr
}

func (v *testVte) IsComplete() bool {
	return v.isComplete
}

func (v *testVte) URI() workspaceapi.URI {
	return v.uri
}

func (v *testVte) Title() string {
	return v.title
}

func newExForTestingTasks(t *testing.T) (testEx, *sync.Mutex, func()) {
	mu := new(sync.Mutex)
	opts := []text.Option{
		text.WithCommandKey(testCommandKey),
		text.WithFloatingNoMaxSize(false),
	}
	tempDir, err := os.MkdirTemp("", "")
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	require.NoError(t, err)
	uri, err := workspaceapi.ParseURI(filepath.Join("file://", tempDir))
	require.NoError(t, err)
	ctx := context.Background()
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	require.NoError(t, err)
	defer fileScheme.Close()
	workspace := workspace.NewSchemeWorkspace(uri, fileScheme)
	vteConfig := vte.DefaultConfig()
	vteConfig.ScheduleNextTick = func(fn func()) bool {
		go func() {
			mu.Lock()
			defer mu.Unlock()
			fn()
		}()
		return true
	}
	e := newExForTestingTerminal(t, workspace, texttest.NopEditor(),
		vteConfig, nopPublishEvent, opts...)
	return e, mu, func() {
		mu.Lock()
		defer mu.Unlock()
		require.NoError(t, e.Close())
	}
}

// the calling workspace is always in focus
type workspaceManagerMock struct {
	workspace    *ex
	attrs        map[workspaceapi.URI]term.Attributes
	wantFocusURI workspaceapi.URI
}

func (w *workspaceManagerMock) focusHandler() tui.Handler {
	return w.workspace
}

func (w *workspaceManagerMock) focusURI() workspaceapi.URI {
	if w.workspace == nil || w.workspace.workspace == nil {
		return w.wantFocusURI
	}
	uri, _ := w.workspace.workspace.URI(".")
	return uri
}

func (w *workspaceManagerMock) setWorkspaceRequiresAttention(
	uri workspaceapi.URI, attrs term.Attributes,
) {
	if w.attrs == nil {
		w.attrs = make(map[workspaceapi.URI]term.Attributes)
	}
	w.attrs[uri] = attrs
}
