package text

import (
	"context"
	"errors"
	"os/user"
	"strings"
	"sync"
	"testing"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
	"github.com/ernestrc/go-tui/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	keya = term.KeyComb{Ch: 'a'}
	keyb = term.KeyComb{Ch: 'b'}
)

type testFlusherCloser struct {
	closeFn func() error
	flushFn func() error
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

type testWorkspace struct {
	content       string
	flusherCloser *testFlusherCloser
	expectError   error
}

func (t *testWorkspace) Open(
	file workspace.URI, buf *cell.Buffer, swapDir workspace.URI, readOnly bool,
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
	return &testFlusherCloser{}, nil
}

func (t *testWorkspace) Recover(
	file, swapFilePath workspace.URI, buf *cell.Buffer,
) (workspace.FlusherCloser, error) {
	return t.Open(file, buf, workspace.URI{}, false)
}

func newTestComponentErr(ed Editor) (*Component, error) {
	cfg := DefaultConfig()
	c, err := NewComponent(ed, &testWorkspace{}, cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func newTestComponent(t *testing.T, ed Editor) *Component {
	c, err := newTestComponentErr(ed)
	require.NoError(t, err)
	return c
}

func TestComponentInterfaces(t *testing.T) {
	// this test is just a compile-time test
	c, err := NewComponent(&testEditor{}, &testWorkspace{}, DefaultConfig())
	require.NoError(t, err)

	var ed Editor
	ed = c

	var b browser.Browser
	b = c

	var comp tui.Component
	comp = c

	// use so compiler does not complain
	ed.Edit(workspace.URI{}, cell.NewBuffer())
	_, _ = b.Focus()
	comp.Resize(0, 0)
}

func TestComponentKeyMapper(t *testing.T) {
	t.Run("KeyMapping on a non-mapped event returns false", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})

		ev, _, ok := c.KeyMapping(keya)
		assert.False(t, ok)
		assert.Equal(t, keya, ev)
	})

	t.Run("KeyMapping returns mapped event", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})

		m := map[term.KeyComb]term.KeyComb{
			keya: keyb,
		}
		err := c.MergeKeyMap(m)
		require.NoError(t, err)

		ev, _, ok := c.KeyMapping(keya)
		assert.True(t, ok)
		assert.Equal(t, keyb, ev)
	})

	t.Run("KeyMapping returns mapped command", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})
		c.config.CommandKeyBindings[keyb] = "myCmd"

		m := map[term.KeyComb]term.KeyComb{
			keya: keyb,
		}
		err := c.MergeKeyMap(m)
		require.NoError(t, err)

		ev, cmd, ok := c.KeyMapping(keya)
		assert.True(t, ok)
		assert.Equal(t, keyb, ev)
		assert.Equal(t, "myCmd", cmd)
	})
}

func newTestComponentWithFile(
	t *testing.T, filename string,
) (*Component, browser.Handler, workspace.URI) {
	uri, err := workspace.ParseURI(filename)
	require.NoError(t, err)
	c := newTestComponent(t, &testEditor{})
	h, err := c.Open(uri)
	require.NoError(t, err)
	return c, h, uri
}

func TestComponentOpen(t *testing.T) {
	t.Run("opens a new tab", func(t *testing.T) {
		myName := "file:///tmp/Its_1am_and_Im_very_tired.go"
		c, h, uri := newTestComponentWithFile(t, myName)

		h2, ok := c.Browser().Tab(uri)
		assert.True(t, ok)
		assert.Equal(t, h, h2)
	})

	t.Run("it's idempotent", func(t *testing.T) {
		myName := "file:///var/music/La_Rosalia.mp3"

		c, h, uri := newTestComponentWithFile(t, myName)
		_, ok := c.Browser().Tab(uri)
		assert.True(t, ok)

		c.workspace.(*testWorkspace).expectError = workspace.ErrFileAlreadyOpen

		h2, err := c.Open(uri)
		require.NoError(t, err)
		assert.Equal(t, h, h2)
	})

	t.Run("it's idempotent 2", func(t *testing.T) {
		myName := "file:///tmp/Its_1am_and_Im_very_tired.go"
		c, _, uri := newTestComponentWithFile(t, myName)

		t2, ok := c.Browser().Tab(uri)
		require.True(t, ok)

		b3, err := c.Open(uri)
		require.NoError(t, err)
		assert.Equal(t, t2, b3)
	})

	t.Run("bubbles up open file error", func(t *testing.T) {
		c, _, _ := newTestComponentWithFile(t, "file:///tmp/lmao")
		myErr := errors.New("oopsie daisy")
		c.workspace.(*testWorkspace).expectError = myErr

		uri, err := workspace.ParseURI("file:///Holmes.xd")
		require.NoError(t, err)
		_, err = c.Open(uri)
		require.Error(t, err)
	})

	t.Run("if file is already open it returns its handler", func(t *testing.T) {
		c, h1, uri := newTestComponentWithFile(t, "file:///tmp/wasup")
		c.workspace.(*testWorkspace).expectError = errors.New("should not be called")

		h2, err := c.Open(uri)
		require.NoError(t, err)
		assert.Equal(t, h1, h2)
	})
}

func TestComponentEditorSubscriber(t *testing.T) {
	content := "Mr. Patoto"
	tsuite := []struct {
		name       string
		evType     EventType
		trigger    func(*testing.T, *Component, workspace.URI)
		preTrigger func(*testing.T, *Component, workspace.URI)
	}{
		{
			"Edit->EventTypeOpen",
			EventTypeOpen,
			func(t *testing.T, c *Component, resource workspace.URI) {
				buf := cell.NewBuffer()
				buf.WriteString(content)
				_, err := c.Edit(resource, buf)
				assert.NoError(t, err)
			},
			nil,
		},
		{
			"OpenFileTab->EventTypeOpen",
			EventTypeOpen,
			func(t *testing.T, c *Component, resource workspace.URI) {
				_, err := c.OpenFileTab(resource, false)
				assert.NoError(t, err)
			},
			nil,
		},
		{
			"Open->EventTypeOpen",
			EventTypeOpen,
			func(t *testing.T, c *Component, resource workspace.URI) {
				_, err := c.Open(resource)
				assert.NoError(t, err)
			},
			nil,
		},
		{
			"Flush->EventTypeFlush",
			EventTypeFlush,
			func(t *testing.T, c *Component, resource workspace.URI) {
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
			EventTypeClose,
			func(t *testing.T, c *Component, resource workspace.URI) {
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
			EventTypeEdit,
			func(t *testing.T, c *Component, resource workspace.URI) {
				buf := cell.NewBuffer()
				_, err := c.Edit(resource, buf)
				assert.NoError(t, err)

				buf.WriteString("wasup")
			},
			nil,
		},
		{
			"buf.DeleteRow->EventTypeEdit",
			EventTypeEdit,
			func(t *testing.T, c *Component, resource workspace.URI) {
				buf := cell.NewBuffer()
				buf.WriteString("wasup")
				_, err := c.Edit(resource, buf)
				assert.NoError(t, err)

				buf.DeleteRow(0)
			},
			nil,
		},
		{
			"Window.SetContent->EventTypeUnfocus",
			EventTypeUnfocus,
			func(t *testing.T, c *Component, resource workspace.URI) {
				uri2, err := workspace.ParseURI("file:///tmp/bleh")
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)

				h, err := c.Open(uri2)
				require.NoError(t, err)
				require.NoError(t, win.SetContent(h))
			},
			func(t *testing.T, c *Component, resource workspace.URI) {
				h, err := c.Open(resource)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)
				require.NoError(t, win.SetContent(h))
			},
		},
		{
			"Window.SetContent->EventTypeFocus",
			EventTypeFocus,
			func(t *testing.T, c *Component, resource workspace.URI) {
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
			EventTypeFocus,
			func(t *testing.T, c *Component, resource workspace.URI) {
				h, err := c.OpenFileTab(resource, false)
				require.NoError(t, err)

				_, err = c.Split(browser.OrientationBottom, h)
				require.NoError(t, err)
			},
			func(t *testing.T, c *Component, resource workspace.URI) {
				uri2, err := workspace.ParseURI("file:///tmp/blah")
				require.NoError(t, err)
				_, err = c.Open(uri2)
				require.NoError(t, err)
			},
		},
		{
			"SetContent->EventTypeFocus",
			EventTypeFocus,
			func(t *testing.T, c *Component, resource workspace.URI) {
				// SubscribeEditor should trigger it
			},
			func(t *testing.T, c *Component, resource workspace.URI) {
				h, err := c.Open(resource)
				require.NoError(t, err)

				win, err := c.Focus()
				require.NoError(t, err)

				require.NoError(t, win.SetContent(h))
			},
		},
		{
			"SubscribeOpen->EventTypeOpen",
			EventTypeOpen,
			func(t *testing.T, c *Component, resource workspace.URI) {
				// SubscribeEditor should trigger it
			},
			func(t *testing.T, c *Component, resource workspace.URI) {
				_, err := c.Open(resource)
				require.NoError(t, err)
			},
		},
		{
			"Handle>EventTypeCursor",
			EventTypeCursor,
			func(t *testing.T, c *Component, resource workspace.URI) {
				buf := cell.NewBuffer()
				buf.WriteString(content)
				h, err := c.Edit(resource, buf)
				assert.NoError(t, err)
				h.Handle(term.Event{Ch: 'l'})
			},
			nil,
		},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase
		t.Run(tcase.name+" SubscribeEditor subscribes an event handler", func(t *testing.T) {
			c := newTestComponent(t, &testEditor{})

			filename := "~/Joe_Biden.txt"
			uri, err := workspace.CurrentUserHostURI(filename)
			require.NoError(t, err)
			if tcase.preTrigger != nil {
				tcase.preTrigger(t, c, uri)
			}

			var fired int
			usr, _ := user.Current()
			dir := usr.HomeDir
			evs := []EventType{tcase.evType}
			h := FuncEventHandler(func(ctx context.Context, ev Event) bool {
				// if preTrigger, then only assert relevant file event
				if tcase.preTrigger == nil {
					assert.True(t, strings.Contains(ev.URI.String(), "Joe_Biden.txt"))
					fired++
				} else if strings.Contains(ev.URI.String(), "Joe_Biden.txt") {
					fired++
				}
				// Edit skip Edit as it takes the resource name as is.
				if tcase.evType == EventTypeOpen && tcase.name != "Edit->EventTypeOpen" {
					assert.True(t, strings.Contains(ev.URI.String(), dir))
				}
				return false
			})
			c.SubscribeEditorEvents(evs, h)

			tcase.trigger(t, c, uri)
			assert.Equal(t, 1, fired)
		})

		t.Run(tcase.name+" unsubscribes if handler returns exit=true", func(t *testing.T) {
			c := newTestComponent(t, &testEditor{})

			filename := "file:///Jill_Biden.txt"
			uri, err := workspace.ParseURI(filename)
			require.NoError(t, err)
			if tcase.preTrigger != nil {
				tcase.preTrigger(t, c, uri)
			}

			var fired int
			evs := []EventType{tcase.evType}
			h := FuncEventHandler(func(ctx context.Context, ev Event) bool {
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
			c.SubscribeEditorEvents(evs, h)

			tcase.trigger(t, c, uri)
			assert.Equal(t, 1, fired)
		})
	}

	t.Run("no events are dispatched after Close is called", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})

		filename := "file:///Jill_Biden.txt"
		uri, err := workspace.ParseURI(filename)
		require.NoError(t, err)
		ev := Event{
			Type: EventTypeClose,
			URI:  uri,
		}
		fc := testFlusherCloser{closeFn: func() error {
			c.dispatchEvent(ev)
			return nil
		}}

		c.workspace.(*testWorkspace).flusherCloser = &fc

		var fired int
		evs := []EventType{EventTypeClose}
		h := FuncEventHandler(func(ctx context.Context, ev Event) bool {
			fired++
			return false
		})
		c.SubscribeEditorEvents(evs, h)

		_, err = c.Open(uri)
		require.NoError(t, err)

		assert.NoError(t, c.Close())
		assert.Equal(t, 0, fired)
	})

	t.Run("one open event is dispatched per open tab upon subscribe to open", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})
		uri1, err := workspace.ParseURI("file:///Jill_Biden.txt")
		require.NoError(t, err)
		uri2, err := workspace.ParseURI("file:///Joe_Biden.txt")
		require.NoError(t, err)

		content := "how bout that"
		c.workspace.(*testWorkspace).content = content

		_, err = c.Open(uri1)
		require.NoError(t, err)
		_, err = c.Open(uri2)
		require.NoError(t, err)

		var fired int
		ev := []EventType{EventTypeOpen}
		h := FuncEventHandler(func(ctx context.Context, ev Event) bool {
			fired++
			assert.Equal(t, content, ev.Content)
			return false
		})
		c.SubscribeEditorEvents(ev, h)

		assert.Equal(t, 2, fired)
	})

	t.Run("EventTypeUnfocus is dispatched before EventTypeFocus on content update", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})
		uri1, err := workspace.ParseURI("file:///Jill_Biden.txt")
		require.NoError(t, err)
		uri2, err := workspace.ParseURI("file:///Joe_Biden.txt")
		require.NoError(t, err)

		a, err := c.Open(uri1)
		require.NoError(t, err)

		b, err := c.Open(uri2)
		require.NoError(t, err)

		win, err := c.Focus()
		require.NoError(t, err)

		require.NoError(t, win.SetContent(a))

		var i int
		evs := []EventType{EventTypeFocus, EventTypeUnfocus}
		h := FuncEventHandler(func(ctx context.Context, ev Event) bool {
			if i == 1 {
				assert.Equal(t, EventTypeUnfocus, ev.Type)
			} else {
				assert.Equal(t, EventTypeFocus, ev.Type)
			}
			i++
			return false
		})
		c.SubscribeEditorEvents(evs, h)

		// upon SubscribeEditorEvents, we dispatch first Focus
		assert.Equal(t, 1, i)
		require.NoError(t, win.SetContent(b))
		assert.Equal(t, 3, i)
	})
}

func TestDispatchCommand(t *testing.T) {
	t.Run("DispatchCommand returns false if there's no registered handler", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})
		uri, err := workspace.ParseURI("file:///MacMecMic")
		require.NoError(t, err)

		cmd := Command{
			Resource: NewTestHandler(),
			URI:      uri,
			Name:     "SELL",
		}
		assert.False(t, c.DispatchCommand(cmd))
	})
}

func TestComponentEditor(t *testing.T) {
	t.Run("returns tab with name as Handler", func(t *testing.T) {
		myName := "file:///tmp/Ennio_Morricone.go"
		c, h1, uri := newTestComponentWithFile(t, myName)

		h2, err := c.Editor(uri)
		assert.NoError(t, err)
		assert.Equal(t, h1.(*browser.Tab).Handler(), h2)
	})

	t.Run("returns error if no handler is found with name", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})

		h, err := c.Editor(workspace.URI{})
		assert.Error(t, err)
		assert.Nil(t, h)
	})
}

func TestComponentCommands(t *testing.T) {
	t.Run("returns empty slice if no commands have been registered", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})
		assert.Len(t, c.Commands(), 0)
	})

	t.Run("returns registered commands", func(t *testing.T) {
		c := newTestComponent(t, &testEditor{})
		c.SubscribeCommand("myCmd", FuncCommandHandler(func(context.Context, Command) bool {
			return true
		}))
		cmds := c.Commands()
		require.Len(t, cmds, 1)
		assert.Equal(t, "myCmd", cmds[0])
	})
}

func testRegister(t *testing.T,
	constructor func(ed Editor, mu *sync.Mutex, resource workspace.URI) (*Component, Editor, error)) {
	t.Run("Registered handler is unsubscribed upon returning exit=true", func(t *testing.T) {
		var mu sync.Mutex
		resource1, err := workspace.ParseURI("file:///HERS")
		require.NoError(t, err)
		myArgs := []string{"a", "bbbbbbbbbbbbbbbbbbbbb"}
		myCmd := "BUY"
		c, sut, err := constructor(&testEditor{}, &mu, resource1)
		require.NoError(t, err)

		mu.Lock()
		h1, err := c.Edit(resource1, cell.NewBuffer())
		mu.Unlock()
		require.NoError(t, err)

		var called int
		var wg sync.WaitGroup
		sut.SubscribeCommand(myCmd, FuncCommandHandler(func(ctx context.Context, cmd Command) bool {
			defer wg.Done()
			assert.Equal(t, myCmd, cmd.Name)
			assert.Equal(t, myArgs, cmd.Args)
			called++
			return true
		}))

		wg.Add(1)
		mu.Lock()
		cmd := Command{Resource: h1, URI: resource1, Name: myCmd, Args: myArgs}
		assert.True(t, c.DispatchCommand(cmd))
		mu.Unlock()

		wg.Wait()

		for i := 0; i < 20; i++ {
			mu.Lock()
			c.DispatchCommand(cmd)
			mu.Unlock()
		}

		assert.Equal(t, 1, called)
	})
}

func TestComponentRegister(t *testing.T) {
	testRegister(t, func(ed Editor, mu *sync.Mutex, res workspace.URI) (*Component, Editor, error) {
		c, err := newTestComponentErr(ed)
		return c, c, err
	})
}
func TestTabIntegration(t *testing.T) {
	testTabIntegration(t, func(ed Editor, mu *sync.Mutex) (*Component, browser.WindowManager, error) {
		c, err := newTestComponentErr(ed)
		return c, c, err
	})
}

func testTabIntegration(t *testing.T,
	constructor func(ed Editor, mu *sync.Mutex) (*Component, browser.WindowManager, error)) {
	t.Run("switches to a tab upon call to SetContent", func(t *testing.T) {
		cases := []testutil.HandlerSequenceTestCase{
			{"",
				`┌──────────────────┐
│$$  ##            │
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
			c, wm, err := constructor(&testEditor{}, &mu)
			require.NoError(t, err)

			resource1, err := workspace.ParseURI("file:///a")
			require.NoError(t, err)
			resource2, err := workspace.ParseURI("file:///b")
			require.NoError(t, err)
			b1 := browser.NewTestHandler()
			b1.Ch = '$'
			_, err = wm.Tab(resource1, "$$", b1)
			require.NoError(t, err)

			b2 := browser.NewTestHandler()
			b2.Ch = '#'
			t2, err := wm.Tab(resource2, "##", b2)
			require.NoError(t, err)

			win, err := wm.Focus()
			require.NoError(t, err)

			require.NoError(t, win.SetContent(t2))
			return handler.Sync(&mu, handler.Nop(&c.comp))
		}
		testutil.TestHandlerIsolated(t, fn, 20, 10, cases)
	})
}
