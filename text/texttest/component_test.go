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

package texttest

import (
	"context"
	"errors"
	"os"
	"os/user"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	gomock "go.uber.org/mock/gomock"
	"unstable.build/go-tui/api/extutil"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacetest"
)

var (
	keya = term.KeyComb{Ch: 'a'}
)

type testFlusherCloser struct {
	buf       *cell.Buffer
	closeFn   func() error
	flushFn   func() error
	reloadFn  func() error
	content   string
	lastFlush time.Time
}

func (t *testFlusherCloser) Close() error {
	if t.closeFn != nil {
		return t.closeFn()
	}
	return nil
}
func (t *testFlusherCloser) Flush() error {
	if t.flushFn != nil {
		return t.flushFn()
	}
	return nil
}

func (t *testFlusherCloser) ForceFlush() error {
	if t.flushFn != nil {
		return t.flushFn()
	}
	return nil
}

func (t *testFlusherCloser) LastFlush() time.Time {
	return t.lastFlush
}

func (t *testFlusherCloser) Reload() error {
	if t.reloadFn != nil {
		return t.reloadFn()
	}
	if t.content != "" {
		t.buf.Replace(t.content)
	}
	return nil
}

type testLoader struct {
	content       string
	flusherCloser *testFlusherCloser
	expectError   error
}

func (t *testLoader) Remove(string) error {
	return nil
}

func (t *testLoader) Load(
	file workspaceapi.URI, buf *cell.Buffer, swapDir workspaceapi.URI, readOnly bool,
) (workspace.FlusherCloser, error) {
	if t.expectError != nil {
		return nil, t.expectError
	}
	if t.flusherCloser != nil {
		return t.flusherCloser, nil
	}
	if t.content != "" {
		buf.WriteString(t.content)
	}
	return &testFlusherCloser{buf: buf, content: t.content}, nil
}

func (t *testLoader) Recover(
	file, swapFilePath workspaceapi.URI, buf *cell.Buffer, force bool,
) (workspace.FlusherCloser, error) {
	return t.Load(file, buf, workspaceapi.URI{}, false)
}

func (t *testLoader) URI(path string) (workspaceapi.URI, error) {
	panic("unused")
}

func (t *testLoader) OpenFile(path string, flag int, perm os.FileMode) (
	workspaceapi.File, error,
) {
	panic("unused")
}

func (t *testLoader) Stat(path string) (os.FileInfo, error) {
	panic("unused")
}

func (t *testLoader) ReadDir(name string) ([]os.DirEntry, error) {
	panic("unused")
}

func newTestComponentErr(ed text.Editor, cfg text.Config) (*text.Component, *testLoader, error) {
	loader := &testLoader{}
	c, err := text.NewComponent(ed, loader, cfg)
	if err != nil {
		return nil, nil, err
	}
	return c, loader, nil
}

func newTestComponent(t *testing.T, ed text.Editor) (*text.Component, *testLoader) {
	return newTestComponentConfig(t, ed, text.DefaultConfig())
}

func newTestComponentConfig(t *testing.T, ed text.Editor, cfg text.Config) (
	*text.Component, *testLoader,
) {
	cfg.NoMaxSize = false
	c, loader, err := newTestComponentErr(ed, cfg)
	require.NoError(t, err)
	return c, loader
}

func TestComponentInterfaces(t *testing.T) {
	// this test is just a compile-time test
	c, err := text.NewComponent(NopEditor(), &testLoader{}, text.DefaultConfig())
	require.NoError(t, err)

	var ed text.Editor
	ed = c

	var b browser.Browser
	b = c

	var comp tui.Component
	comp = c

	// use so compiler does not complain
	ed.Edit(workspaceapi.URI{}, cell.NewBuffer(), false, false)
	_, _ = b.Focus()
	comp.Resize(0, 0)
}

func TestComponentCommandKeyBinding(t *testing.T) {
	t.Run("on a non-mapped event returns false", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())
		_, ok := c.CommandKeyBinding(keya)
		assert.False(t, ok)
	})

	t.Run("returns mapped command", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandKeyBindings[keya] = [][]string{{"myCmd"}}
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		cmd, ok := c.CommandKeyBinding(keya)
		assert.True(t, ok)
		assert.Equal(t, [][]string{{"myCmd"}}, cmd)
	})

	t.Run("returns mapped command and args", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandKeyBindings[keya] = [][]string{{"myCmd", "1"}}
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		cmd, ok := c.CommandKeyBinding(keya)
		assert.True(t, ok)
		assert.Equal(t, [][]string{{"myCmd", "1"}}, cmd)
	})

	t.Run("returns multiple mapped commands and args", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandKeyBindings[keya] = [][]string{
			{"myCmd", "1"},
			{"GZA", "Duel Of The Iron Mic", "Masta Killa", "Dreddy Kruger"},
		}
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		cmd, ok := c.CommandKeyBinding(keya)
		assert.True(t, ok)
		assert.Equal(t, [][]string{
			{"myCmd", "1"},
			{"GZA", "Duel Of The Iron Mic", "Masta Killa", "Dreddy Kruger"},
		}, cmd)
	})
}

func newTestComponentWithFile(
	t *testing.T, filename string,
) (*text.Component, *testLoader, browserapi.Handler, workspaceapi.URI) {
	uri, err := workspaceapi.ParseURI(filename)
	require.NoError(t, err)
	c, loader := newTestComponent(t, NopEditor())
	h, err := c.Open(uri)
	require.NoError(t, err)
	return c, loader, h, uri
}

func TestComponentOpen(t *testing.T) {
	t.Run("opens a new tab", func(t *testing.T) {
		myName := "file:///tmp/Its_1am_and_Im_very_tired.go"
		c, _, h, uri := newTestComponentWithFile(t, myName)

		h2, ok := c.Browser().Tab(uri)
		assert.True(t, ok)
		assert.Equal(t, h, h2)
	})

	t.Run("it's idempotent", func(t *testing.T) {
		myName := "file:///var/music/La_Rosalia.mp3"

		c, loader, h, uri := newTestComponentWithFile(t, myName)
		_, ok := c.Browser().Tab(uri)
		assert.True(t, ok)

		loader.expectError = workspaceapi.ErrFileAlreadyOpen

		h2, err := c.Open(uri)
		require.NoError(t, err)
		assert.Equal(t, h, h2)
	})

	t.Run("it's idempotent 2", func(t *testing.T) {
		myName := "file:///tmp/Its_1am_and_Im_very_tired.go"
		c, _, _, uri := newTestComponentWithFile(t, myName)

		t2, ok := c.Browser().Tab(uri)
		require.True(t, ok)

		b3, err := c.Open(uri)
		require.NoError(t, err)
		assert.Equal(t, t2, b3)
	})

	t.Run("bubbles up open file error", func(t *testing.T) {
		c, loader, _, _ := newTestComponentWithFile(t, "file:///tmp/lmao")
		myErr := errors.New("oopsie daisy")
		loader.expectError = myErr

		uri, err := workspaceapi.ParseURI("file:///Holmes.xd")
		require.NoError(t, err)
		_, err = c.Open(uri)
		require.Error(t, err)
	})

	t.Run("if file is already open it returns its handler", func(t *testing.T) {
		c, loader, h1, uri := newTestComponentWithFile(t, "file:///tmp/wasup")
		loader.expectError = errors.New("should not be called")

		h2, err := c.Open(uri)
		require.NoError(t, err)
		assert.Equal(t, h1, h2)
	})

	t.Run("opens recovery prompt if err == workspaceapi.ErrFileAlreadyOpen", func(t *testing.T) {
		c, loader, _, _ := newTestComponentWithFile(t, "file:///tmp/wasup")
		c.Resize(30, 20)

		tests := []comptest.TestCase{
			{nil, `
┌────────────────────────────┐
│o wasup                     │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`,
			},
			{func() {
				uri, err := workspaceapi.ParseURI("file:///tmp/busy")
				require.NoError(t, err)

				loader.expectError = workspaceapi.ErrFileAlreadyOpen
				_, err = c.Open(uri)
				require.Equal(t, workspaceapi.ErrFileAlreadyOpen, err)
			}, `
┌────────────────────────────┐
│o wasup                     │
├────────────────────────────┤
│                            │
┌────────────────────────────┐
│                            │
│                            │
│   File file:///tmp/busy    │
│   is already open by       │
│   another process or an    │
│   edit session for this    │
│   file crashed.            │
│                            │
│                            │
│  Rec    Ope    for    Ski  │
│  ove    n      ce     p    │
└────────────────────────────┘
│                            │
│                            │
└────────────────────────────┘`,
			},
			{func() {
				loader.expectError = nil
				_, handled := c.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				assert.True(t, handled)
			}, `
┌────────────────────────────┐
│o wasup  o busy             │
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
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`,
			},
			{func() { // test double prompt, switches focuses correctly
				loader.expectError = workspaceapi.ErrFileAlreadyOpen

				uri1, err := workspaceapi.ParseURI("file:///tmp/m")
				require.NoError(t, err)

				_, err = c.Open(uri1)
				require.Equal(t, workspaceapi.ErrFileAlreadyOpen, err)

				uri2, err := workspaceapi.ParseURI("file:///tmp/more")
				require.NoError(t, err)

				_, err = c.Open(uri2)
				require.Equal(t, workspaceapi.ErrFileAlreadyOpen, err)
			}, `
┌────────────────────────────┐
│o wasup  o busy             │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
┌────────────────────────────┐
│                            │
│                            │
│   File file:///tmp/more    │
│   is already open by       │
│   another process or an    │
│   edit session for this    │
│   file crashed.            │
│                            │
│                            │
│  Rec    Ope    for    Ski  │
│  ove    n      ce     p    │
└────────────────────────────┘
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`,
			},
			{func() {
				loader.expectError = nil
				_, handled := c.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				assert.True(t, handled)
			}, `
┌────────────────────────────┐
│o wasup  o busy  o more     │
├────────────────────────────┤
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
┌────────────────────────────┐
│                            │
│                            │
│  File file:///tmp/m is     │
│  already open by another   │
│  process or an edit        │
│  session for this file     │
│  crashed.                  │
│                            │
│                            │
│  Rec    Ope    for    Ski  │
│  ove    n      ce     p    │
└────────────────────────────┘
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`,
			},
			{func() {
				loader.expectError = nil
				_, handled := c.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				assert.True(t, handled)
			}, `
┌────────────────────────────┐
│o wasup  o busy  o more  o m│
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
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAAAAAAAAAAAA│
└────────────────────────────┘`,
			},
		}

		w := term.NewStringWriter(30, 20)
		comptest.TestComponent(t, c, w, tests)

	})

	t.Run("sets a tab name", func(t *testing.T) {
		c, _, _, _ := newTestComponentWithFile(t, "file:///tmp/wasup")
		c.Resize(30, 20)

		tests := []comptest.TestCase{
			{nil, `
┌────────────────────────────┐
│o wasup                     │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`,
			},
			{func() {
				uri, err := workspaceapi.ParseURI("file:///tmp/wasup")
				require.NoError(t, err)

				require.NoError(t, c.SetTabName(uri, "whatevs", term.Attributes{}))
			}, `
┌────────────────────────────┐
│o whatevs                   │
├────────────────────────────┤
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
│                            │
└────────────────────────────┘`,
			},
		}

		w := term.NewStringWriter(30, 20)
		comptest.TestComponent(t, c, w, tests)

	})
}

func TestComponentEditorSubscriber(t *testing.T) {
	content := "Mr. Patoto"
	tsuite := []struct {
		name       string
		evType     textapi.EventType
		trigger    func(*testing.T, *text.Component, workspaceapi.URI)
		preTrigger func(*testing.T, *text.Component, workspaceapi.URI)
	}{
		{
			"Edit->EventTypeOpen",
			textapi.EventTypeOpen,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				buf := cell.NewBuffer()
				buf.WriteString(content)
				_, err := c.Edit(resource, buf, false, false)
				assert.NoError(t, err)
			},
			nil,
		},
		{
			"OpenFileTab->EventTypeOpen",
			textapi.EventTypeOpen,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				_, err := c.OpenFileTab(resource, false)
				assert.NoError(t, err)
			},
			nil,
		},
		{
			"Open->EventTypeOpen",
			textapi.EventTypeOpen,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				_, err := c.Open(resource)
				assert.NoError(t, err)
			},
			nil,
		},
		{
			"Flush->EventTypeFlush",
			textapi.EventTypeFlush,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				h, err := c.OpenFileTab(resource, false)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)

				require.NoError(t, win.SetContent(h))

				assert.NoError(t, c.Flush(win))
			},
			nil,
		},
		{
			"Browser.RemoveWindowContent->EventTypeClose",
			textapi.EventTypeClose,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				h, err := c.OpenFileTab(resource, false)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)

				require.NoError(t, win.SetContent(h))

				c.Browser().RemoveWindowContent(win)
			},
			nil,
		},
		{
			"buf.WriteString->EventTypeEdit",
			textapi.EventTypeEdit,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				buf := cell.NewBuffer()
				_, err := c.Edit(resource, buf, false, false)
				assert.NoError(t, err)

				buf.WriteString("wasup")
			},
			nil,
		},
		{
			"buf.DeleteRow->EventTypeEdit",
			textapi.EventTypeEdit,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				buf := cell.NewBuffer()
				buf.WriteString("wasup")
				_, err := c.Edit(resource, buf, false, false)
				assert.NoError(t, err)

				buf.DeleteRow(0)
			},
			nil,
		},
		{
			"Window.SetContent->EventTypeUnfocus",
			textapi.EventTypeUnfocus,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				uri2, err := workspaceapi.ParseURI("file:///tmp/bleh")
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)

				h, err := c.Open(uri2)
				require.NoError(t, err)
				require.NoError(t, win.SetContent(h))
			},
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				h, err := c.Open(resource)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)
				require.NoError(t, win.SetContent(h))
			},
		},
		{
			"Window.SetContent->EventTypeFocus",
			textapi.EventTypeFocus,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				h, err := c.Open(resource)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)
				require.NoError(t, win.SetContent(h))
			},
			nil,
		},
		{
			"Split->EventTypeFocus",
			textapi.EventTypeFocus,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				h, err := c.OpenFileTab(resource, false)
				require.NoError(t, err)

				focus, err := c.Focus()
				require.NoError(t, err)
				_, err = c.Split(browserapi.OrientationBottom, focus, h)
				require.NoError(t, err)
			},
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				uri2, err := workspaceapi.ParseURI("file:///tmp/blah")
				require.NoError(t, err)
				_, err = c.Open(uri2)
				require.NoError(t, err)
			},
		},
		{
			"SetContent->EventTypeFocus",
			textapi.EventTypeFocus,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				// SubscribeEditor should trigger it
			},
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				h, err := c.Open(resource)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)

				require.NoError(t, win.SetContent(h))
			},
		},
		{
			"SubscribeOpen->EventTypeOpen",
			textapi.EventTypeOpen,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				// SubscribeEditor should trigger it
			},
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				_, err := c.Open(resource)
				require.NoError(t, err)
			},
		},
		{
			"Handle>EventTypeCursor",
			textapi.EventTypeCursor,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				buf := cell.NewBuffer()
				buf.WriteString(content)
				h, err := c.Edit(resource, buf, false, false)
				assert.NoError(t, err)
				h.Handle(term.Event{Ch: 'l'})
			},
			nil,
		},
		{
			"Handle>EventTypeSelection",
			textapi.EventTypeSelection,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				buf := cell.NewBuffer()
				buf.WriteString(content)
				h, err := c.Edit(resource, buf, false, false)
				assert.NoError(t, err)
				h.Handle(term.Event{Ch: 'v'})
			},
			nil,
		},
		{
			"DispatchEvent>EventTypeRename",
			textapi.EventTypeRename,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				assert.True(t, c.DispatchEvent(textapi.Event{
					Type: textapi.EventTypeRename,
					URI:  resource,
				}))
			},
			nil,
		},
		{
			"DispatchEvent>EventTypeRemove",
			textapi.EventTypeRemove,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				assert.True(t, c.DispatchEvent(textapi.Event{
					Type: textapi.EventTypeRemove,
					URI:  resource,
				}))
			},
			nil,
		},
		{
			"DispatchEvent>EventTypeChange",
			textapi.EventTypeChange,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				assert.True(t, c.DispatchEvent(textapi.Event{
					Type: textapi.EventTypeChange,
					URI:  resource,
				}))
			},
			nil,
		},
		{
			"DispatchEvent>EventTypeCreate",
			textapi.EventTypeCreate,
			func(t *testing.T, c *text.Component, resource workspaceapi.URI) {
				assert.True(t, c.DispatchEvent(textapi.Event{
					Type: textapi.EventTypeCreate,
					URI:  resource,
				}))
			},
			nil,
		},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase
		t.Run(tcase.name+" SubscribeEditor subscribes an event handler", func(t *testing.T) {
			c, _ := newTestComponent(t, NopEditor())

			filename := "~/Joe_Biden.txt"
			uri, err := workspaceapi.CurrentUserHostURI(filename)
			require.NoError(t, err)
			if tcase.preTrigger != nil {
				tcase.preTrigger(t, c, uri)
			}

			var fired int
			usr, _ := user.Current()
			dir := usr.HomeDir
			evs := []textapi.EventType{tcase.evType}
			h := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				// if preTrigger, then only assert relevant file event
				if tcase.preTrigger == nil {
					assert.True(t, strings.Contains(ev.URI.String(), "Joe_Biden.txt"))
					fired++
				} else if strings.Contains(ev.URI.String(), "Joe_Biden.txt") {
					fired++
				}
				// Edit skip Edit as it takes the resource name as is.
				if tcase.evType == textapi.EventTypeOpen && tcase.name != "Edit->EventTypeOpen" {
					assert.True(t, strings.Contains(ev.URI.String(), dir))
				}
				return false
			})
			c.SubscribeEvents(evs, h)

			tcase.trigger(t, c, uri)
			assert.Equal(t, 1, fired)
		})

		t.Run(tcase.name+" unsubscribes if handler returns exit=true", func(t *testing.T) {
			c, _ := newTestComponent(t, NopEditor())

			filename := "file:///Jill_Biden.txt"
			uri, err := workspaceapi.ParseURI(filename)
			require.NoError(t, err)
			if tcase.preTrigger != nil {
				tcase.preTrigger(t, c, uri)
			}

			var fired int
			evs := []textapi.EventType{tcase.evType}
			h := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
				if tcase.preTrigger == nil {
					assert.True(t, strings.Contains(ev.URI.String(), "Jill_Biden.txt"))
					fired++
					return true
				}
				if strings.Contains(ev.URI.String(), "Jill_Biden.txt") {
					fired++
					return true
				}
				return false
			})
			c.SubscribeEvents(evs, h)

			tcase.trigger(t, c, uri)
			assert.Equal(t, 1, fired)
		})
	}

	t.Run("no events are dispatched after Close is called", func(t *testing.T) {
		c, loader := newTestComponent(t, NopEditor())

		filename := "file:///Jill_Biden.txt"
		uri, err := workspaceapi.ParseURI(filename)
		require.NoError(t, err)
		fc := testFlusherCloser{closeFn: func() error {
			return nil
		}}

		loader.flusherCloser = &fc

		var fired int
		evs := []textapi.EventType{textapi.EventTypeClose}
		h := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			fired++
			return false
		})
		c.SubscribeEvents(evs, h)

		_, err = c.Open(uri)
		require.NoError(t, err)

		assert.NoError(t, c.Close())
		assert.Equal(t, 0, fired)
	})

	t.Run("one open event is dispatched per open tab upon subscribe to open", func(t *testing.T) {
		c, loader := newTestComponent(t, NopEditor())
		uri1, err := workspaceapi.ParseURI("file:///Jill_Biden.txt")
		require.NoError(t, err)
		uri2, err := workspaceapi.ParseURI("file:///Joe_Biden.txt")
		require.NoError(t, err)

		content := "how bout that"
		loader.content = content

		_, err = c.Open(uri1)
		require.NoError(t, err)
		_, err = c.Open(uri2)
		require.NoError(t, err)

		var fired int
		ev := []textapi.EventType{textapi.EventTypeOpen}
		h := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			fired++
			assert.Equal(t, content, ev.Content)
			return false
		})
		c.SubscribeEvents(ev, h)

		assert.Equal(t, 2, fired)
	})

	t.Run("EventTypeUnfocus is dispatched before EventTypeFocus on content update", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())
		uri1, err := workspaceapi.ParseURI("file:///Jill_Biden.txt")
		require.NoError(t, err)
		uri2, err := workspaceapi.ParseURI("file:///Joe_Biden.txt")
		require.NoError(t, err)

		a, err := c.Open(uri1)
		require.NoError(t, err)

		b, err := c.Open(uri2)
		require.NoError(t, err)

		win, err := c.Focus()
		require.NoError(t, err)

		require.NoError(t, win.SetContent(a))

		var i int
		evs := []textapi.EventType{textapi.EventTypeFocus, textapi.EventTypeUnfocus}
		h := text.FuncEventHandler(func(ctx context.Context, ev textapi.Event) bool {
			if i == 1 {
				assert.Equal(t, textapi.EventTypeUnfocus, ev.Type)
			} else {
				assert.Equal(t, textapi.EventTypeFocus, ev.Type)
			}
			i++
			return false
		})
		c.SubscribeEvents(evs, h)

		// upon SubscribeEvents, we dispatch first Focus
		assert.Equal(t, 1, i)
		require.NoError(t, win.SetContent(b))
		assert.Equal(t, 3, i)
	})
}

func TestEventTypeFocusIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	c, _ := newTestComponent(t, &TestEditor{})
	c.Resize(100, 100)

	uri1, err := workspaceapi.ParseURI("file:///Bastardo.txt")
	require.NoError(t, err)

	uri2, err := workspaceapi.ParseURI("file:///Bigotudo.txt")
	require.NoError(t, err)

	h1, err := c.OpenFileTab(uri1, false)
	require.NoError(t, err)

	h2, err := c.OpenFileTab(uri2, false)
	require.NoError(t, err)

	w1, err := c.Focus()
	require.NoError(t, err)

	require.NoError(t, w1.SetContent(h2))

	mock := NewMockEventHandler(ctrl)
	evs := []textapi.EventType{textapi.EventTypeFocus, textapi.EventTypeUnfocus}

	const (
		frameWidth  = 2
		tabBarWidth = 2
	)

	expectedWidth := 100 - frameWidth
	expectedHeight := 100 - frameWidth - tabBarWidth

	t.Run("dispatch focus event upon subscribe", func(t *testing.T) {
		expectFocusEvent(t, mock, uri2, expectedWidth, expectedHeight)
		require.NoError(t, c.SubscribeEvents(evs, mock))
	})

	t.Run("dispatch focus/unfocus events on focus window content changes", func(t *testing.T) {
		expectFocusEvents(t, mock, uri1, uri2, expectedWidth, expectedHeight)
		require.NoError(t, w1.SetContent(h1))

		expectFocusEvents(t, mock, uri2, uri1, expectedWidth, expectedHeight)
		require.NoError(t, c.Browser().Focus().SetContent(h2))
	})
	var w2 browser.Window
	t.Run("dispatch focus/unfocus events upon creating a new window", func(t *testing.T) {
		expectFocusEvents(t, mock, uri1, uri2, 50-frameWidth, expectedHeight)
		w2, err = c.Split(browserapi.OrientationRight, w1, h1)
		require.NoError(t, err)
	})

	t.Run("dispatch focus/unfocus events on changing window in focus", func(t *testing.T) {
		expectFocusEvents(t, mock, uri2, uri1, 50-frameWidth, expectedHeight)
		_, err = c.SetFocus(w1)
		require.NoError(t, err)

		expectFocusEvents(t, mock, uri1, uri2, 50-frameWidth, expectedHeight)
		_, err = c.SetFocus(w2)
		require.NoError(t, err)
	})

	t.Run("do not dispatch focus/unfocus events on new tab", func(t *testing.T) {
		ok := c.Browser().NextTab(w2)
		require.False(t, ok)
	})

	t.Run("dispatch focus/unfocus events on window focus shift", func(t *testing.T) {
		expectFocusEvents(t, mock, uri2, uri1, 50-frameWidth, expectedHeight)
		ok := c.Browser().ShiftFocus()
		require.True(t, ok)

		expectFocusEvents(t, mock, uri1, uri2, 50-frameWidth, expectedHeight)
		ok = c.Browser().ShiftFocus()
		require.True(t, ok)
	})

	t.Run("dispatch focus/unfocus events on window in focus close", func(t *testing.T) {
		expectFocusEvents(t, mock, uri2, uri1, expectedWidth, expectedHeight)
		require.NoError(t, w2.Close())
	})

	t.Run("do not dispatch focus/unfocus events upon NextTab on window not in focus",
		func(t *testing.T) {
			ok := c.Browser().NextTab(w2)
			require.False(t, ok)
		})

	t.Run("dispatch focus/unfocus events upon NextTab on window in focus",
		func(t *testing.T) {
			expectFocusEvents(t, mock, uri1, uri2, expectedWidth, expectedHeight)
			ok := c.Browser().NextTab(w1)
			require.True(t, ok)
		})

	t.Run("dispatch focus event on resize", func(t *testing.T) {
		expectFocusEvent(t, mock, uri1, 8-frameWidth, 8-frameWidth-tabBarWidth)
		c.Resize(8, 8)
	})

	t.Run("dispatch focus event upon subscribe, non handler doesn't panic", func(t *testing.T) {
		win, err := c.Focus()
		require.NoError(t, err)
		mock.EXPECT().Handle(gomock.Any(), gomock.Any()).Return(false).Times(1)
		win.SetContent(browserapi.NopHandler(handler.Nop()))
		_, ok := c.Browser().NewTabFromContent('a', "bla", win)
		require.True(t, ok)
		require.NoError(t, c.SubscribeEvents(evs, mock))
	})
}

func TestDispatchCommand(t *testing.T) {
	uri, err := workspaceapi.ParseURI("file:///MacMecMic")
	require.NoError(t, err)

	t.Run("returns false if there's no registered handler", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())

		win, _ := c.Focus()
		cmd := textapi.Command{
			Resource: NewTestHandler(),
			URI:      uri,
			Name:     "SELL",
			Window:   win,
		}
		ok, err := c.DispatchCommand(cmd)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("uses aliases from config to dispatch", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandAliases = map[string]text.CommandAlias{
			"workstation_layout": text.CommandAlias{
				Commands: []string{"newWindow", "edit /tmp/todo.md"},
			},
		}
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		win, _ := c.Focus()

		var newWindowCalled, editCalled bool

		c.SubscribeCommand(testCommand("newWindow", "", ""),
			text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
				newWindowCalled = true
				assert.Equal(t, cmd.Name, "newWindow")
				assert.Equal(t, cmd.Args, []string{"newArgs"})
				return nil
			}, nil))

		c.SubscribeCommand(testCommand("edit", "", ""),
			text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
				editCalled = true
				assert.Equal(t, cmd.Name, "edit")
				assert.Equal(t, cmd.Args, []string{"/tmp/todo.md", "newArgs"})
				return nil
			}, nil))

		cmd := textapi.Command{
			Resource: NewTestHandler(),
			URI:      uri,
			Name:     "workstation_layout",
			Args:     []string{"newArgs"},
			Window:   win,
		}
		ok, err := c.DispatchCommand(cmd)
		assert.True(t, ok)
		require.NoError(t, err)
		assert.True(t, newWindowCalled)
		assert.True(t, editCalled)
	})

	t.Run("replaces aliases positional commands with dispatched cmds", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandAliases = map[string]text.CommandAlias{
			"yeti": {
				Commands: []string{"newWindow wasup '$2' $name $$1", "edit $1 hellagood"},
			},
		}
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		win, _ := c.Focus()

		var newWindowCalled, editCalled int

		// make sure that substitution doesn't replace original alias
		// so we can replace it dynamically every time
		const n = 100
		for range n {
			c.SubscribeCommand(testCommand("newWindow", "", ""),
				text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
					newWindowCalled++
					assert.Equal(t, "newWindow", cmd.Name)
					assert.Equal(t, []string{"wasup", "'arg2'", "$name", "$1"}, cmd.Args)
					return nil
				}, nil))

			c.SubscribeCommand(testCommand("edit", "", ""),
				text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
					editCalled++
					assert.Equal(t, "edit", cmd.Name)
					assert.Equal(t, []string{"arg1", "hellagood"}, cmd.Args)
					return nil
				}, nil))

			cmd := textapi.Command{
				Resource: NewTestHandler(),
				URI:      uri,
				Name:     "yeti",
				Args:     []string{"arg1", "arg2"},
				Window:   win,
			}
			ok, err := c.DispatchCommand(cmd)
			assert.True(t, ok)
			require.NoError(t, err)
		}

		assert.Equal(t, n, newWindowCalled)
		assert.Equal(t, n, editCalled)
	})

	t.Run("replaces commands % arg with current file", func(t *testing.T) {
		config := text.DefaultConfig()
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		win, _ := c.Focus()

		var editCalled int

		c.SubscribeCommand(testCommand("edit", "", ""),
			text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
				editCalled++
				assert.Equal(t, "edit", cmd.Name)
				assert.Equal(t, []string{"'/a'", "--all"}, cmd.Args)
				return nil
			}, nil))

		resource1, err := workspaceapi.ParseURI("file:///a")
		require.NoError(t, err)

		h, err := c.Open(resource1)
		require.NoError(t, err)

		c.Browser().Focus().SetContent(h)

		cmd := textapi.Command{
			Resource: NewTestHandler(),
			URI:      uri,
			Name:     "edit",
			Args:     []string{"'%'", "--all"},
			Window:   win,
		}
		ok, err := c.DispatchCommand(cmd)
		assert.True(t, ok)
		require.NoError(t, err)
	})

	t.Run("bubbles up HandleCommand errors", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())
		win, _ := c.Focus()
		c.SubscribeCommand(testCommand("bla", "", ""),
			text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
				return errors.New("boom")
			}, nil))

		cmd := textapi.Command{
			Resource: NewTestHandler(),
			URI:      uri,
			Name:     "bla",
			Window:   win,
		}
		ok, err := c.DispatchCommand(cmd)
		assert.True(t, ok)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("handles bad aliases", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandAliases = map[string]text.CommandAlias{
			"bad1": text.CommandAlias{Commands: []string{""}},
			"bad2": text.CommandAlias{Commands: []string{}},
			"bad3": text.CommandAlias{},
		}
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		win, _ := c.Focus()

		for _, cmd := range []string{"bad1", "bad2", "bad3"} {
			cmd := textapi.Command{
				Resource: NewTestHandler(),
				URI:      uri,
				Name:     cmd,
				Window:   win,
			}
			ok, err := c.DispatchCommand(cmd)
			assert.False(t, ok)
			require.NoError(t, err)
		}
	})

	t.Run("handles aliases missing from config", func(t *testing.T) {
		config := text.DefaultConfig()
		config.CommandAliases = nil
		c, _ := newTestComponentConfig(t, NopEditor(), config)
		win, _ := c.Focus()

		cmd := textapi.Command{
			Resource: NewTestHandler(),
			URI:      uri,
			Name:     "kaboom",
			Window:   win,
		}
		ok, err := c.DispatchCommand(cmd)
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("NewComponent returns error if aliases create an infinite loop of command calls", func(t *testing.T) {
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"bleh"}},
			"bleh": text.CommandAlias{Commands: []string{"blah"}},
		}
		_, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("NewComponent does not return error if aliases simply embeds another alias", func(t *testing.T) {
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"bleh"}},
			"bleh": text.CommandAlias{Commands: []string{"bloh"}},
		}
		_, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.NoError(t, err)
	})

	t.Run("NewComponent returns error if aliases create an infinite loop of nested command calls", func(t *testing.T) {
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"bleh"}},
			"bleh": text.CommandAlias{Commands: []string{"bloh"}},
			"bloh": text.CommandAlias{Commands: []string{"bluh", "blah"}},
		}
		_, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("NewComponent does not return error if aliases simply embeds another nested alias", func(t *testing.T) {
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"bleh"}},
			"bleh": text.CommandAlias{Commands: []string{"bloh"}},
			"bloh": text.CommandAlias{Commands: []string{"bluh", "otherThing"}},
		}
		_, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.NoError(t, err)
	})
}
func TestCompleteCommand(t *testing.T) {
	t.Run("uses alias completer, if defined", func(t *testing.T) {
		completer := func(c *text.Component) command.Completer {
			return command.FuncCompleter(func(ctx context.Context, args []string) (
				iterator.Iterator[string], string, error,
			) {
				return iterator.FromSlice[string]([]string{"a", "b", "c"}), "sus", nil
			})
		}
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"bleh"}, Completer: completer},
		}
		c, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.NoError(t, err)

		it, arg, err := c.CompleteCommand(context.Background(), textapi.Command{Name: "blah"})
		require.NoError(t, err)

		slice, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b", "c"}, slice)
		assert.Equal(t, "sus", arg)
	})

	t.Run("bubbles up alias completer error", func(t *testing.T) {
		completer := func(c *text.Component) command.Completer {
			return command.FuncCompleter(func(ctx context.Context, args []string) (
				iterator.Iterator[string], string, error,
			) {
				return nil, "", errors.New("kaboom")
			})
		}
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"bleh"}, Completer: completer},
		}
		c, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.NoError(t, err)

		_, _, err = c.CompleteCommand(context.Background(), textapi.Command{Name: "blah"})
		require.EqualError(t, err, "kaboom")
	})

	t.Run("returns empty iterator if there's no registered handler", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())

		it, _, err := c.CompleteCommand(context.Background(), textapi.Command{Name: "blabla"})
		require.NoError(t, err)
		assertIteratorLen(t, 0, it)
	})

	t.Run("returns empty iterator if attempting to complete alias with no completer defined", func(t *testing.T) {
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"workstation_layout": text.CommandAlias{
				Commands: []string{
					"newWindow",
					"edit /tmp/todo.md",
				},
			},
		}
		c, _ := newTestComponentConfig(t, NopEditor(), cfg)

		it, _, err := c.CompleteCommand(context.Background(), textapi.Command{Name: "workstation_layout"})
		require.NoError(t, err)
		assertIteratorLen(t, 0, it)
	})

	t.Run("calls command handler Complete", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())

		c.SubscribeCommand(testCommand("edit", "", ""),
			text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
				return nil
			}, func(ctx context.Context, cmd textapi.Command) (iterator.Iterator[string], string, error) {
				assert.Equal(t, []string{"letter", "number"}, cmd.Args)
				return iterator.FromSlice([]string{"one", "two"}), "2", nil
			}))

		it, newLastArg, err := c.CompleteCommand(context.Background(), textapi.Command{Name: "edit", Args: []string{"letter", "number"}})
		require.NoError(t, err)
		options := assertIteratorLen(t, 2, it)
		assert.Equal(t, []string{"one", "two"}, options)
		assert.Equal(t, "2", newLastArg)
	})
}

func assertIteratorLen(t *testing.T, n int, it iterator.Iterator[string]) []string {
	var ret []string
	var i int
	for ; ; i++ {
		next, ok := it.Next(context.Background())
		if !ok {
			break
		}
		ret = append(ret, next)
	}
	assert.Equal(t, n, i)
	return ret
}

func TestComponentEditor(t *testing.T) {
	t.Run("returns tab with name as Handler", func(t *testing.T) {
		myName := "file:///tmp/Ennio_Morricone.go"
		c, _, h1, uri := newTestComponentWithFile(t, myName)

		h2, err := c.Editor(uri)
		assert.NoError(t, err)
		assert.Equal(t, h1.(*browser.Tab).Handler(), h2)
	})

	t.Run("returns editor returned in call to Edit", func(t *testing.T) {
		myName, err := workspaceapi.ParseURI("file:///tmp/Ennio_Morricone.go")
		require.NoError(t, err)
		c, _ := newTestComponent(t, NopEditor())

		h1, err := c.Edit(myName, cell.NewBuffer(), false, false)
		assert.NoError(t, err)

		actualH1, err := c.Editor(myName)
		assert.NoError(t, err)
		assert.Equal(t, h1, actualH1)
	})

	t.Run("returns error if Handler returned in call to Edit is closed", func(t *testing.T) {
		myName, err := workspaceapi.ParseURI("file:///tmp/Ennio_Morricone.go")
		require.NoError(t, err)
		c, _ := newTestComponent(t, NopEditor())

		h1, err := c.Edit(myName, cell.NewBuffer(), false, false)
		assert.NoError(t, err)

		require.NoError(t, h1.Close())

		actualH1, err := c.Editor(myName)
		assert.Error(t, err)
		assert.Nil(t, actualH1)
	})

	t.Run("returns error if no handler is found with name", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())

		h, err := c.Editor(workspaceapi.URI{})
		assert.Error(t, err)
		assert.Nil(t, h)
	})
}

func TestComponentCommands(t *testing.T) {
	t.Run("returns empty slice if no commands have been registered", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())
		assert.Len(t, c.Commands(), 0)
	})

	t.Run("returns registered commands", func(t *testing.T) {
		c, _ := newTestComponent(t, NopEditor())
		c.SubscribeCommand(testCommand("myCmd", "mySummary", "mySynopsis"),
			text.FuncCommandHandler(func(context.Context, textapi.Command) error {
				return nil
			}, nil))
		cmds := c.Commands()
		require.Len(t, cmds, 1)
		expectedMan := command.Manual{Name: "myCmd", Summary: "mySummary", Synopsis: "mySynopsis"}
		assert.Equal(t, expectedMan, cmds[0])
	})
	t.Run("returns configured aliases", func(t *testing.T) {
		cfg := text.DefaultConfig()
		cfg.CommandAliases = map[string]text.CommandAlias{
			"blah": text.CommandAlias{Commands: []string{"myCmd"}},
		}
		c, err := text.NewComponent(NopEditor(), &testLoader{}, cfg)
		require.NoError(t, err)
		cmds := c.Commands()
		require.Len(t, cmds, 1)
		expectedMan := command.Manual{Name: "blah", AliasOf: []string{"myCmd"}}
		assert.Equal(t, expectedMan, cmds[0])
	})
}

func testRegister(t *testing.T,
	constructor func(ed text.Editor, mu *sync.Mutex, resource workspaceapi.URI) (*text.Component, text.Editor, error)) {
	t.Run("calls subscribed command handler", func(t *testing.T) {
		var mu sync.Mutex
		resource1, err := workspaceapi.ParseURI("file:///HERS")
		require.NoError(t, err)
		myArgs := []string{"a", "bbbbbbbbbbbbbbbbbbbbb"}
		myCmd := "BUY"
		c, sut, err := constructor(NopEditor(), &mu, resource1)
		require.NoError(t, err)

		mu.Lock()
		win, _ := c.Focus()
		h1, err := c.Edit(resource1, cell.NewBuffer(), false, false)
		mu.Unlock()
		require.NoError(t, err)

		var called int
		var wg sync.WaitGroup
		sut.SubscribeCommand(testCommand(myCmd, "", ""),
			text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
				defer wg.Done()
				assert.Equal(t, myCmd, cmd.Name)
				assert.Equal(t, myArgs, cmd.Args)
				called++
				return nil
			}, nil))

		wg.Add(1)
		mu.Lock()
		cmd := textapi.Command{Resource: h1, URI: resource1, Name: myCmd, Args: myArgs,
			Window: win}
		ok, err := c.DispatchCommand(cmd)
		assert.True(t, ok)
		require.NoError(t, err)
		mu.Unlock()

		wg.Wait()
		assert.Equal(t, 1, called)
	})
}

func TestComponentRegister(t *testing.T) {
	testRegister(t, func(ed text.Editor, mu *sync.Mutex, res workspaceapi.URI) (*text.Component, text.Editor, error) {
		c, _, err := newTestComponentErr(ed, text.DefaultConfig())
		return c, c, err
	})
}
func TestUnregisterCommand(t *testing.T) {
	resource1, err := workspaceapi.ParseURI("file:///HERS")
	require.NoError(t, err)
	myArgs := []string{"a", "bbbbbbbbbbbbbbbbbbbbb"}
	myCmd := "BUY"
	c, err := text.NewComponent(NopEditor(), &testLoader{}, text.DefaultConfig())
	require.NoError(t, err)

	win, _ := c.Focus()
	h1, err := c.Edit(resource1, cell.NewBuffer(), false, false)
	require.NoError(t, err)

	var called int
	c.SubscribeCommand(testCommand(myCmd, "", ""),
		text.FuncCommandHandler(func(ctx context.Context, cmd textapi.Command) error {
			called++
			return nil
		}, nil))

	cmd := textapi.Command{
		Resource: h1,
		URI:      resource1,
		Name:     myCmd,
		Args:     myArgs,
		Window:   win,
	}
	ok, err := c.DispatchCommand(cmd)
	assert.True(t, ok)
	require.NoError(t, err)

	assert.Equal(t, 1, called)

	err = c.UnsubscribeCommand(myCmd)
	require.NoError(t, err)

	ok, err = c.DispatchCommand(cmd)
	assert.False(t, ok)
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestTabIntegration(t *testing.T) {
	testTabIntegration(t, func(ed text.Editor, mu *sync.Mutex) (*text.Component, browser.WindowManager, error) {
		c, _, err := newTestComponentErr(ed, text.DefaultConfig())
		return c, c, err
	})
}

func testTabIntegration(t *testing.T,
	constructor func(ed text.Editor, mu *sync.Mutex) (*text.Component, browser.WindowManager, error)) {
	t.Run("switches to a tab upon call to SetContent", func(t *testing.T) {
		cases := []handlertest.SequenceTestCase{
			{"",
				`┌──────────────────┐
│x $$  x ##        │
├──────────────────┤
│##################│
│##################│
│##################│
│##################│
│##################│
│##################│
└──────────────────┘`},
		}

		fn := func(t *testing.T) tui.Handler {
			var mu sync.Mutex
			c, wm, err := constructor(NopEditor(), &mu)
			require.NoError(t, err)

			resource1, err := workspaceapi.ParseURI("file:///a")
			require.NoError(t, err)
			resource2, err := workspaceapi.ParseURI("file:///b")
			require.NoError(t, err)
			b1 := browsertest.NewTestHandler()
			b1.TestHandler.Ch = '$'
			_, err = wm.Tab(resource1, 'x', "$$", b1)
			require.NoError(t, err)

			b2 := browsertest.NewTestHandler()
			b2.TestHandler.Ch = '#'
			t2, err := wm.Tab(resource2, 'x', "##", b2)
			require.NoError(t, err)

			win, err := wm.Focus()
			require.NoError(t, err)

			require.NoError(t, win.SetContent(t2))
			return handler.Sync(&mu, handler.NopFromComponent(c))
		}
		handlertest.TestHandlerIsolated(t, fn, 20, 10, cases)
	})
}

func TestFlush(t *testing.T) {
	resource1, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)

	t.Run("calls underlying closer Flush", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockHandler(ctrl)
		mockEditor := NewMockEditor(ctrl)
		mockWorkspace := NewMockWorkspace(ctrl)
		mockFlusherCloser := workspacetest.NewMockFlusherCloser(ctrl)

		c, err := text.NewComponent(mockEditor, mockWorkspace, text.DefaultConfig())
		require.NoError(t, err)

		win, err := c.Focus()
		require.NoError(t, err)

		mockWorkspace.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(mockFlusherCloser, nil).Times(1)
		mock.EXPECT().Resize(gomock.Any(), gomock.Any()).Times(1)
		mock.EXPECT().CursorAtScroll().
			Return(term.Coordinates{}).Times(1)
		mock.EXPECT().CellView().
			Return(cell.NewBuffer().View()).Times(1)
		mockEditor.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Eq(true), gomock.Any()).Return(mock, nil)

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, win.SetContent(h))

		mockFlusherCloser.EXPECT().Flush().Times(1)
		require.NoError(t, c.Flush(win))
	})

	t.Run("returns ErrInvalidSave if called on tab with nil closer handle", func(t *testing.T) {
		mock := browsertest.NewTestHandler()
		c, _ := newTestComponent(t, nil)
		win, err := c.Focus()
		require.NoError(t, err)

		h, err := c.Tab(resource1, 'x', "Rupi Kaur", mock)
		require.NoError(t, win.SetContent(h))

		require.Equal(t, textapi.ErrInvalidSave, c.Flush(win))
	})

	t.Run("returns ErrInvalidSave if called on non-tab", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockHandler(ctrl)
		c, _ := newTestComponent(t, nil)
		win, err := c.Focus()
		require.NoError(t, err)

		mock.EXPECT().Resize(gomock.Any(), gomock.Any()).AnyTimes()
		_, err = c.Split(browserapi.OrientationTop, win, mock)
		require.NoError(t, err)

		require.Equal(t, textapi.ErrInvalidSave, c.Flush(win))
	})
}

func TestReload(t *testing.T) {
	resource1, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)

	t.Run("calls underlying closer Flush", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockHandler(ctrl)
		mockEditor := NewMockEditor(ctrl)
		mockWorkspace := NewMockWorkspace(ctrl)
		mockFlusherCloser := workspacetest.NewMockFlusherCloser(ctrl)

		c, err := text.NewComponent(mockEditor, mockWorkspace, text.DefaultConfig())
		require.NoError(t, err)

		win, err := c.Focus()
		require.NoError(t, err)

		mockWorkspace.EXPECT().Load(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(mockFlusherCloser, nil).Times(1)
		mock.EXPECT().Resize(gomock.Any(), gomock.Any()).Times(1)
		mock.EXPECT().CursorAtScroll().
			Return(term.Coordinates{}).Times(1)
		mockEditor.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mock, nil)

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, win.SetContent(h))

		mock.EXPECT().CellView().
			Return(cell.NewBuffer().View()).Times(1)

		mockFlusherCloser.EXPECT().Reload().Times(1)
		require.NoError(t, c.Reload(win))
	})

	t.Run("returns ErrInvalidSave if called on tab with nil closer handle", func(t *testing.T) {
		mock := browsertest.NewTestHandler()
		c, _ := newTestComponent(t, nil)
		win, err := c.Focus()
		require.NoError(t, err)

		h, err := c.Tab(resource1, 'x', "Rupi Kaur", mock)
		require.NoError(t, win.SetContent(h))

		require.Equal(t, textapi.ErrInvalidReload, c.Reload(win))
	})

	t.Run("returns ErrInvalidReload if called on non-tab", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockHandler(ctrl)
		c, _ := newTestComponent(t, nil)
		win, err := c.Focus()
		require.NoError(t, err)

		mock.EXPECT().Resize(gomock.Any(), gomock.Any()).AnyTimes()
		_, err = c.Split(browserapi.OrientationTop, win, mock)
		require.NoError(t, err)

		require.Equal(t, textapi.ErrInvalidReload, c.Reload(win))
	})

	t.Run("bubbles up file reload errors", func(t *testing.T) {
		c, testLoader := newTestComponent(t, NopEditor())
		win, err := c.Focus()
		require.NoError(t, err)

		testLoader.flusherCloser = &testFlusherCloser{
			reloadFn: func() error { return errors.New("boom") },
		}

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, err)
		require.NoError(t, win.SetContent(h))

		err = c.Reload(win)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("integration with event dispatching", func(t *testing.T) {
		ctx := context.Background()
		c, testLoader := newTestComponent(t, NopEditor())
		win, err := c.Focus()
		require.NoError(t, err)

		const content = "abc\ndef\n"
		testLoader.content = content

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, err)
		require.NoError(t, win.SetContent(h))

		cfg := text.DefaultConfig()
		tracker := extutil.NewResourceTracker(cfg.Tabspaces, false)
		err = c.SubscribeEvents(textapi.AllEvents(), tracker)
		require.NoError(t, err)

		ed, err := c.Editor(resource1)
		require.NoError(t, err)

		ced := ed.CellEditor()
		_, _, _ = ced.Edit(ctx, term.Coordinates{}, term.Coordinates{}, "ABC")

		assertContent := func(t *testing.T, expected string) {
			t.Helper()
			cview := ed.CellView()
			cells := cview.RawCells()
			require.NoError(t, err)
			assert.Equal(t, expected, term.CellsToString(cells))

			res, ok := tracker.Resource(resource1)
			require.True(t, ok)
			assert.Equal(t, expected, res.Buffer().String())
		}

		assertContent(t, "ABC"+content)

		require.NoError(t, c.Reload(win))

		assertContent(t, content)
	})

	t.Run("integration with IsDirty", func(t *testing.T) {
		ctx := context.Background()
		c, testLoader := newTestComponent(t, NopEditor())
		win, err := c.Focus()
		require.NoError(t, err)

		const content = "abc\ndef\n"
		testLoader.content = content

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, err)
		require.NoError(t, win.SetContent(h))

		ed, err := c.Editor(resource1)
		require.NoError(t, err)

		ced := ed.CellEditor()
		_, _, _ = ced.Edit(ctx, term.Coordinates{}, term.Coordinates{}, "ABC")

		isDirty, ok := c.IsDirty(resource1)
		require.True(t, ok)
		assert.True(t, isDirty)

		require.NoError(t, c.Reload(win))

		isDirty, ok = c.IsDirty(resource1)
		require.True(t, ok)
		assert.False(t, isDirty)
	})
}

func TestOverwrite(t *testing.T) {
	resource1, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)

	t.Run("integration with event dispatching", func(t *testing.T) {
		ctx := context.Background()
		c, testLoader := newTestComponent(t, NopEditor())
		win, err := c.Focus()
		require.NoError(t, err)

		const content = "abc\ndef\n"
		testLoader.content = content

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, err)
		require.NoError(t, win.SetContent(h))

		cfg := text.DefaultConfig()
		tracker := extutil.NewResourceTracker(cfg.Tabspaces, false)
		err = c.SubscribeEvents([]textapi.EventType{
			textapi.EventTypeFlush, textapi.EventTypeOpen,
		}, tracker)
		require.NoError(t, err)

		ed, err := c.Editor(resource1)
		require.NoError(t, err)

		ced := ed.CellEditor()
		_, _, _ = ced.Edit(ctx, term.Coordinates{}, term.Coordinates{}, "ABC")

		assertContent := func(t *testing.T, expected string) {
			t.Helper()
			cview := ed.CellView()
			cells := cview.RawCells()
			assert.Equal(t, expected, term.CellsToString(cells))

			res, ok := tracker.Resource(resource1)
			require.True(t, ok)
			assert.Equal(t, expected, res.Buffer().String())
		}

		require.NoError(t, c.Overwrite(win))
		assertContent(t, "ABC"+content)
	})

	t.Run("integration with IsDirty", func(t *testing.T) {
		ctx := context.Background()
		c, testLoader := newTestComponent(t, NopEditor())
		win, err := c.Focus()
		require.NoError(t, err)

		const content = "abc\ndef\n"
		testLoader.content = content

		h, err := c.OpenFileTab(resource1, true)
		require.NoError(t, err)
		require.NoError(t, win.SetContent(h))

		ed, err := c.Editor(resource1)
		require.NoError(t, err)

		ced := ed.CellEditor()
		_, _, _ = ced.Edit(ctx, term.Coordinates{}, term.Coordinates{}, "ABC")
		require.NoError(t, err)

		isDirty, ok := c.IsDirty(resource1)
		require.True(t, ok)
		assert.True(t, isDirty)

		require.NoError(t, c.Overwrite(win))

		isDirty, ok = c.IsDirty(resource1)
		require.True(t, ok)
		assert.False(t, isDirty)
	})
}

func testCommand(cmd string, summary string, synopsis string) textapi.CommandManual {
	return textapi.CommandManual{
		Name:     cmd,
		Synopsis: synopsis,
		Summary:  summary,
	}
}

func assertFocusWidthHeight(t *testing.T,
	expectedWidth, expectedHeight int,
	ev textapi.Event,
) {
	assert.Equal(t, term.Coordinates{
		X: expectedWidth, Y: expectedHeight,
	}, ev.Start)
	edh, ok := ev.Resource.(*TestEditorHandler)
	require.True(t, ok)
	assert.Equal(t, expectedWidth, edh.Width)
	assert.Equal(t, expectedHeight, edh.Height)
}

func expectFocusEvents(
	t *testing.T, mock *MockEventHandler, focus, unfocus workspaceapi.URI,
	expectedWidth, expectedHeight int,
) {
	mock.EXPECT().Handle(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ev textapi.Event) bool {
			if textapi.EventTypeFocus == ev.Type {
				assert.Equal(t, focus.String(), ev.URI.String())
				assertFocusWidthHeight(t, expectedWidth, expectedHeight, ev)
			} else if textapi.EventTypeUnfocus == ev.Type {
				assert.Equal(t, unfocus.String(), ev.URI.String())
			}
			return false
		}).Times(2)
}

func expectFocusEvent(
	t *testing.T, mock *MockEventHandler, uri workspaceapi.URI,
	expectedWidth, expectedHeight int,
) {
	mock.EXPECT().Handle(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ev textapi.Event) bool {
			assert.Equal(t, textapi.EventTypeFocus, ev.Type)
			assert.Equal(t, uri.String(), ev.URI.String())
			assertFocusWidthHeight(t, expectedWidth, expectedHeight, ev)
			return false
		})
}
