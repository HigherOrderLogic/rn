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
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/term/vte/vtereservoir"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/text/texttest"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacetest"
)

// handlertest.TestHandlerSequence maps ':' characters to the following event
// this is to work around ex's assumptions on underlying handler.
var testCommandKey = term.KeyComb{Ch: '\\', Mod: term.ModCtrl}

type browserConstructor func(ed text.Editor, opts ...text.Option) (tui.Handler, browser.Browser, error)

type testFileBuffer struct {
	readOnly  bool
	flushErr  error
	reloadErr error
	closeErr  error
	closed    bool
	lastFlush time.Time
}

func (t *testFileBuffer) Flush() error {
	if t.readOnly {
		return workspaceapi.ErrFileIsNotWritable
	}
	t.lastFlush = time.Now()
	return t.flushErr
}

func (t *testFileBuffer) Reload() error {
	t.lastFlush = time.Now()
	return t.reloadErr
}

func (t *testFileBuffer) ForceFlush() error {
	if t.readOnly {
		t.readOnly = false
	}
	t.lastFlush = time.Now()
	return t.flushErr
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
│o a.go  o o.go    │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"$", // simulates ctrl-l
			`┌──────────────────┐
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
			`┌──────────────────┐
│o a.go  o o.go    │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{":tcl>",
			`┌──────────────────┐
│o a.go            │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":wq!^^^^^",
			`┌──────────────────┐
│o a.go            │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{":<",
			`┌──────────────────┐
│o a.go            │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{":edit o.go>1111",
			`┌──────────────────┐
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
			`┌──────────────────┐
│o a.go  o o.go    │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
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
│AAAAAAAA││EEEEEEEE│
│AAAAAAAA││EEEEEEEE│
└────────┘│EEEEEEEE│
┌────────┐│EEEEEEEE│
│ZZZZZZZZ││EEEEEEEE│
│ZZZZZZZZ││EEEEEEEE│
└────────┘└────────┘`},
		{":<111111111",
			`┌──────────────────┐
│o a.go  o o.go    │
├────────┐┌────────┤
│AAAAAAAA││EEEEEEEE│
│AAAAAAAA││EEEEEEEE│
└────────┘│EEEEEEEE│
┌────────┐│EEEEEEEE│
│cccccccc││EEEEEEEE│
│cccccccc││EEEEEEEE│
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
│$$$$$$$$││EEEEEEEE│
│$$$$$$$$││EEEEEEEE│
└────────┘│EEEEEEEE│
┌────────┐│EEEEEEEE│
│cccccccc││EEEEEEEE│
│cccccccc││EEEEEEEE│
└────────┘└────────┘`},
	}

	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	require.NoError(t, win.Close())
	require.NoError(t, focus.Close())

	assert.Equal(t, 1, closed)

	cases = []handlertest.SequenceTestCase{
		// test CommandKeyBindings
		{"4$$$",
			`┌──────────────────┐
│o a.go  o o.go    │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		{":tcall>:edit o.go>bcde####",
			`┌──────────────────┐
│o o.go            │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
	}
	handlertest.TestHandlerSequence(t, bh, 20, 10, cases)

	cases = []handlertest.SequenceTestCase{
		{"", `┌──┐
│..│
├EE┤
EEEE`},
	}
	handlertest.TestHandlerSequence(t, bh, 4, 4, cases)

	_, err = b.Notify(notifications.LevelInfo, "wasup: %s", "Z")
	require.NoError(t, err)
	cases = []handlertest.SequenceTestCase{
		{"",
			`┌────┌─────────────┐
│o o.│ wasup: Z    │
├────└─────────────┘
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
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
		component.FloatingConfig{Offset: term.Coordinates{X: 1, Y: 1}})
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
	for i := 0; i < 4; i++ {
		b1 := browsertest.NewTestHandler()
		b1.Ch = rune(strconv.Itoa(i)[0])
		err = b.Bar(o, b1)
		require.NoError(t, err)
		o.Orientation++
	}

	// test case for issue #27
	cases = []handlertest.SequenceTestCase{
		{":edit ait^^^aix^^^^ airsoft.map>",
			`┌──────────────────────────────────┌─────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
│o a.go  o wi.go   │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
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
		previews := map[string]func() func(){
			"setTheme": func() func() {
				called = true
				return func() {
					reverted = true
				}
			},
		}
		mockBuf := testFileBuffer{}
		workspace := testLoader{buf: &mockBuf}
		b := newExForTestingCommandsPreview(t, &workspace, texttest.NopEditor(),
			vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), previews, opts...)
		defer b.Close()

		cancel, ok := b.Preview("setTheme", "arg1")
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
		previews := map[string]func() func(){
			"setTheme": func() func() {
				called = true
				return func() {
					reverted = true
				}
			},
		}
		mockBuf := testFileBuffer{}
		workspace := testLoader{buf: &mockBuf}
		b := newExForTestingCommandsPreview(t, &workspace, texttest.NopEditor(),
			vte.DefaultConfig(), nopPublishEvent, clipboard.NewInMemory(), previews, opts...)
		defer b.Close()

		_, ok := b.Preview("guiSetTheme", "arg1")
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

		_, ok := b.Preview("guiSetTheme", "arg1")
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
eeeeeeeeeeeeeeeee   
eeeeeeeeeeeeeeeee   
eeeeeeeeeeeeeeeee   
eeeeeeeeeeeeeeee▐   `},
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
			text.WithWindowManagerConfig(handler.WindowManagerConfig{
				WindowManagerConfig: component.WindowManagerConfig{Frame: false}}),
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
			`┌──────────────────┐
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
			`┌──────────────────┐
│o 10k.go  o 2     │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"go",
			`┌──────────────────┐
│o 10k.go  o 2     │
├──────────────────┤
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`},
		// 2 seconds of wait should be plenty for sequencer to deem 'g' sequence
		// stale and re-issue event.
		{"g____________________",
			`┌──────────────────┐
│o 10k.go  o 2     │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
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
			text.WithCommandSequenceBinding(handler.Sequence{
				First: term.KeyComb{Ch: 'g'},
				Last:  term.KeyComb{Ch: 'l'},
			}, [][]string{{"tabnext"}}),
			text.WithCommandSequenceBinding(handler.Sequence{
				First: term.KeyComb{Ch: 'g'},
				Last:  term.KeyComb{Ch: 'g'},
			}, [][]string{{"tabcloseall"}}),
			text.WithSequencerTimeout(1 * time.Second),
			text.WithCommandOverlayConfig(testCommandOverlayConfig()),
		}
		ex := new(ex)
		n := notifications.New(ex, notificationsConfig())
		ex.syncCommandPrompt = true
		require.NoError(t, ex.init(texttest.NopEditor(), &testLoader{},
			document.NewInMemoryService(), n,
			vte.DefaultConfig(), func(ev term.Event) bool {
				// do not confuse interrupt from list with sequence re-issue commands
				if ev.Type == term.EventInterrupt {
					return true
				}
				mu.Lock()
				defer mu.Unlock()
				ex.Handle(ev)
				return true
			}, 0, clipboard.NewInMemory(), nil, opts...))
		ex.subscribeCommands()
		b := testEx{Component: n, ex: ex}
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
			`┌──────────────────┐
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
	tui.Component
	*ex
}

func (t testEx) Draw(w term.Writer) {
	t.Component.Draw(w)
}

func (t testEx) Resize(width, height int) {
	t.Component.Resize(width, height)
}

func (t testEx) Handle(ev term.Event) (bool, bool) {
	quit, handle := t.ex.Handle(ev)
	t.ex.Wait()
	return quit, handle
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
	svc := document.NewInMemoryService()
	container := notifications.New(ex, notificationsConfig())
	notifications := newWorkspaceNotifications(svc, container)
	opts = append(opts, text.WithNotifications(notifications))
	opts = append(opts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	opts = append(opts, defCommandKeyBindings()...)
	require.NoError(t, ex.init(ed, workspace, svc,
		container, emulatorCfg, publishEvent,
		0, clipboard.NewInMemory(), nil, opts...))
	ex.subscribeCommands()
	return testEx{Component: container, ex: ex}
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

	svc := document.NewInMemoryService()
	container := notifications.New(ex, notificationsConfig())
	notifications := newWorkspaceNotifications(svc, container)
	finalOpts = append(finalOpts, text.WithNotifications(notifications))

	require.NoError(t, ex.init(ed, workspace, svc,
		container, emulatorCfg, publishEvent, 0, clip, nil, finalOpts...))
	ex.subscribeCommands()
	ex.newEmulatorHandler = func(initialCmd string) (vtereservoir.VTE, error) {
		return newTestVteWithConfig(initialCmd), nil
	}
	ex.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return newTestVteWithConfig(strings.Join(args, " ")), nil
	}
	return testEx{Component: container, ex: ex}
}

func newExForTestingCommandsPreview(
	t *testing.T, workspace workspace.Workspace,
	ed text.Editor,
	emulatorCfg vte.Config,
	publishEvent func(term.Event) bool,
	clip clipboard.Register,
	previews map[string]func() func(),
	opts ...text.Option,
) testEx {
	ex := new(ex)
	ex.syncCommandPrompt = true
	// user opts override default test opts
	finalOpts := defCommandKeyBindings()
	finalOpts = append(finalOpts, text.WithCommandOverlayConfig(testCommandOverlayConfig()))
	finalOpts = append(finalOpts, text.WithFloatingNoMaxSize(false))
	finalOpts = append(finalOpts, opts...)

	svc := document.NewInMemoryService()
	container := notifications.New(ex, notificationsConfig())
	notifications := newWorkspaceNotifications(svc, container)
	finalOpts = append(finalOpts, text.WithNotifications(notifications))

	require.NoError(t, ex.init(ed, workspace, svc,
		container, emulatorCfg, publishEvent, 0, clip, previews, finalOpts...))
	ex.subscribeCommands()
	ex.newEmulatorHandler = func(initialCmd string) (vtereservoir.VTE, error) {
		return newTestVteWithConfig(initialCmd), nil
	}
	ex.newPluginHandler = func(args ...string) (pluginHandler, error) {
		return newTestVteWithConfig(strings.Join(args, " ")), nil
	}
	return testEx{Component: container, ex: ex}
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

func TestCommandHistory(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit a.go>:edit wi.go>1234",
			`┌──────────────────┐
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
			`┌──────────────────┐
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

func TestCloseOtherWindows(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit hello.go>:windowsplit>:windowsplit>:windowfocus left>:windowfocus left>",
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
│..  o wi.go  o x..│
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

func TestIntegrationEphemeralTerminal(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":! sleep 20>",
			`┌──────────────────────────────────────┐
│                                      │
├──┌────────────────────────────────┐──┤
│  │ ▀           sleep 20         0s│  │
│  │────────────────────────────────│  │
│  │▐                               │  │
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
│  │ ▀           sleep 20         0s│  │
│  │────────────────────────────────│  │
│  │▐                               │  │
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
│  │ ▀           sleep 20         0s│  │
│  │────────────────────────────────│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":windowcloseall>",
			`┌────────────────────────┌─────────────┐
│                        │ cannot      │
├──┌─────────────────────│ close all   │
│  │ ▐           sleep 20│ tiled       │
│  │─────────────────────│ windows     │
│  │▐                    └─────────────┘
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":noticloseall>:! sh -c 'sleep 20 && echo % '>",
			`┌──────────────────────────────────────┐
│                                      │
├──┌────────────────────────────────┐──┤
│  │ ▀  sh -c 'sleep 20 && echo % 0s│  │
│  │────────────────────────────────│  │
│  │▐                               │  │
│  │                                │  │
│  │                                │  │
│  │                                │  │
└──└────────────────────────────────┘──┘`,
		},
		{":windowclose>:windowclose>:edit a>:! sh -c 'sleep 20 && echo % '>",
			`┌──────────────────────────────────────┐
│o a                                   │
├──┌────────────────────────────────┐──┤
│  │ ▀  sh -c 'sleep 20 && echo /v0s│  │
│  │────────────────────────────────│  │
│  │▐                               │  │
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
	cfg.Shell = "sh"

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

func TestFullScreen(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":windowsplit>:edit aaa>:edit bbb>:windowtogglemaximize>",
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			`┌──────────────────┐
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
			text.WithWindowManagerConfig(handler.WindowManagerConfig{
				WindowManagerConfig: component.WindowManagerConfig{Frame: false}}),
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
			`┌──────────────────┐
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
				`┌──────────────────┐
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
				`┌──────────────────┐
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
		ex.newEmulatorHandler = func(initialCmd string) (vtereservoir.VTE, error) {
			assert.Equal(t, "echo bla", initialCmd)
			return tvte, nil
		}
		t.Cleanup(func() { _ = ex.Close() })

		ex.terminalnewtab("echo bla")

		require.Len(t, tvte.onFocusChange, 2)
		assert.False(t, tvte.onFocusChange[0])
		assert.True(t, tvte.onFocusChange[1])

		// switch to some other tab, same window
		ex.editFiles("a")
		require.Len(t, tvte.onFocusChange, 3)
		assert.False(t, tvte.onFocusChange[2])

		// switch back to terminal tab, same window
		ex.tabprevious()
		require.Len(t, tvte.onFocusChange, 4)
		assert.True(t, tvte.onFocusChange[3])

		// new window, tab still in screen but not focused
		ex.windownew()
		require.Len(t, tvte.onFocusChange, 5)
		assert.False(t, tvte.onFocusChange[4])

		// focus back to tab window
		ex.windowfocus("left")
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

		ex.tabclose()
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
		ex.newEmulatorHandler = func(initialCmd string) (vtereservoir.VTE, error) {
			return tvte, nil
		}
		t.Cleanup(func() { _ = ex.Close() })
		ex.Resize(100, 100)

		ex.executePlugin()

		require.Len(t, tvte.onFocusChange, 2)
		assert.False(t, tvte.onFocusChange[0])
		assert.True(t, tvte.onFocusChange[1])

		// switching from floating to other window should trigger on focus change
		ex.windowfocus("left")
		require.Len(t, tvte.onFocusChange, 3)
		assert.False(t, tvte.onFocusChange[2])

		// switching back to floating should trigger again
		ex.executePlugin()
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
		ex.editFiles("a")
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
		ex.editFiles("a", "b") // have tabs available for later

		ex.executePlugin("echo", "bla")

		require.Len(t, tvte.onFocusChange, 2)
		assert.False(t, tvte.onFocusChange[0])
		assert.True(t, tvte.onFocusChange[1])

		// switching from floating to other window should trigger on focus change
		ex.windowfocus("left")
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
		ex.tabnext()
		require.Len(t, tvte.onFocusChange, 7)
		assert.False(t, tvte.onFocusChange[6])
	})
}

func TestSwitchToTab(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit hello.go>B:edit world.go>",
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌──────────────┌─────────────┐
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
		b, cleanup := newExForTestingTasks(t)
		defer cleanup()

		require.Error(t, b.newTask("up", "--", "make"))
		require.Error(t, b.newTask("newTask", "left", "make"))
		require.Error(t, b.newTask("--", "make", "test", "things"))
		require.Error(t, b.newTask("up", ".go,.md", "--", "make", "test"))
	})

	t.Run("newtask is called with correct number of args returns no error", func(t *testing.T) {
		b, cleanup := newExForTestingTasks(t)
		defer cleanup()

		require.NoError(t, b.newTask("myTask", "left", "--", "make"))
		require.NoError(t, b.newTask("myTask2", "right", "--", "make", "test", "things"))
		require.NoError(t, b.newTask("myTask3", "left", ".go,.md", "--", "make", "test"))
		require.NoError(t, b.newTask("myTask4", "right", ".go,.md", "--", "make", "test"))
	})

	// left/right alignment combined with up/down is ugly; stick to left/right only
	t.Run("newtask is called with left or right alignment is error", func(t *testing.T) {
		b, cleanup := newExForTestingTasks(t)
		defer cleanup()

		require.Error(t, b.newTask("myTask", "up", "--", "make"))
		require.Error(t, b.newTask("myTask2", "down", "--", "make", "test", "things"))
		require.Error(t, b.newTask("myTask3", "up", ".go,.md", "--", "make", "test"))
		require.Error(t, b.newTask("myTask4", "down", ".go,.md", "--", "make", "test"))
	})

	t.Run("newtask called twice with same task name opens a prompt", func(t *testing.T) {
		b, cleanup := newExForTestingTasks(t)
		defer cleanup()

		require.NoError(t, b.newTask("myTask", "left", "--", "make"))
		require.NoError(t, b.newTask("myTask", "right", "--", "make", "test", "things"))
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
││                  ┌────────┐
││                  │new     │
││                  │vte:    │
││                  │start   │
││                  │command:│
││                  │ context│
││                  │ cancele│
││                  │d       │
││                  └────────┘
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
││                          ││
││                          ││
││  A task with the name    ││
││  "validateAssets"        ││
││  already exists. Do      ││
││  you want to replace     ││
││  it?                     ││
││                          ││
││                          ││
││     Yes          No      ││
││                          ││
└└──────────────────────────┘┘`},
			{"y",
				`┌────────────────────────────┐
│                            │
├┌───────────────────────────┤
││                           │
││                  ┌────────┐
││                  │new     │
││                  │vte:    │
││                  │start   │
││                  │command:│
││                  │ context│
││                  │ cancele│
││                  │d       │
││                  └────────┘
││                           │
└└───────────────────────────┘`},
		}

		e, cleanup := newExForTestingTasks(t)
		defer cleanup()
		handlertest.TestHandlerSequence(t, e, 30, 15, cases)
	})
}

func TestEcho(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{`:edit hello.go>:echo 01234>`,
			`┌────────────────────────────┐
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
}

func TestMoveTabs(t *testing.T) {
	cases := []handlertest.SequenceTestCase{
		{":edit caliu.go>:edit boira.go>b",
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌──────────────┌─────────────┐
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
			`┌────────────────────────────┐
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
			`┌──────────────┌─────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌──────────────┌─────────────┐
│o caliu.go    │ flush:      │
├──────────────│ file is     │
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
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":notificationCloseAll>:write!>",
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌──────────────┌─────────────┐
│o caliu.go  o │ 2 errors    │
├──────────────│ occurred:   │
│BBBBBBBBBBBBBB│ flush:      │
│BBBBBBBBBBBBBB│ file is     │
│BBBBBBBBBBBBBB│ not         │
│BBBBBBBBBBBBBB│ writable;   │
│BBBBBBBBBBBBBB│ flush:      │
│BBBBBBBBBBBBBB│ file is     │
│BBBBBBBBBBBBBB│ not         │
│BBBBBBBBBBBBBB│ writable    │
│BBBBBBBBBBBBBB└─────────────┘
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBBBBBBBBBBBB│
└────────────────────────────┘`},
		{":notificationCloseAll>:writeall!>",
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
					`┌────┌─────────────┐
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
			`┌────────────────────────────┐
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
			`┌──────────────┌─────────────┐
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
			`┌──────────────┌─────────────┐
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
			`┌────────────────────────────┐
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
			`┌────────────────────────────┐
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
		ShowManualAfter: 1 * time.Hour,
	}
}

type testVte struct {
	component.String
	initialCmd string

	calledClose   bool
	defAttr       term.Attributes
	onFocusChange []bool

	isComplete bool
	uri        workspaceapi.URI
	title      string
}

func newTestVte() *testVte {
	return newTestVteWithConfig("")
}

func newTestVteWithConfig(initialCmd string) *testVte {
	ret := new(testVte)
	ret.initialCmd = initialCmd
	ret.String = component.NewString(initialCmd)
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
	return 0
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

func (t *testVte) Cursor() (ret term.Coordinates, style term.CursorStyle, show bool) {
	show = true
	ret = term.Coordinates{X: len(t.initialCmd)}
	return
}

func (t *testVte) Selection() (string, bool) {
	return "", false
}

func (t *testVte) Man() tui.Manual {
	return tui.Manual{}
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

func newExForTestingTasks(t *testing.T) (testEx, func()) {
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
	e := newExForTestingTerminal(t, workspace, texttest.NopEditor(),
		vte.DefaultConfig(), nopPublishEvent, opts...)
	return e, func() {
		require.NoError(t, e.Close())
	}
}
