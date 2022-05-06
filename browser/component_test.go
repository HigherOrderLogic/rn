package browser

import (
	"testing"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func splitVerticalLeft(c *Component, h Handler) (Window, bool) {
	return c.Split(OrientationLeft, h)
}
func splitVerticalRight(c *Component, h Handler) (Window, bool) {
	return c.Split(OrientationRight, h)
}
func splitHorizontalAbove(c *Component, h Handler) (Window, bool) {
	return c.Split(OrientationTop, h)
}
func splitHorizontalBelow(c *Component, h Handler) (Window, bool) {
	return c.Split(OrientationBottom, h)
}

var splitSuite = []struct {
	method string
	split  func(c *Component, h Handler) (Window, bool)
}{
	{"SplitVerticalLeft:", splitVerticalLeft},
	{"SplitVerticalRight:", splitVerticalRight},
	{"SplitHorizontalAbove:", splitHorizontalAbove},
	{"SplitHorizontalBelow:", splitHorizontalBelow},
}

func TestComponentCloseWindow(t *testing.T) {

	t.Run("closing the last window returns error", func(t *testing.T) {
		c := NewComponent(Config{})
		assert.Error(t, c.Focus().Close())

		// makes sure that window list is not corrupted
		c.RemoveAllTabs()
	})

	for _, _tcase := range splitSuite {
		tcase := _tcase
		t.Run(tcase.method+"closes window correctly", func(t *testing.T) {
			c := NewComponent(Config{})
			uri, err := workspace.ParseURI("file:///OAK")
			require.NoError(t, err)

			var h Handler
			h = NewTestHandler()
			h = c.NewTab(uri, "OAK", h, nil)

			win, ok := tcase.split(c, h)
			require.True(t, ok)
			require.Equal(t, 2, c.wm.Size())

			assert.NoError(t, win.Close())
			require.Equal(t, 1, c.wm.Size())

			assert.Len(t, c.freeTabs(), 1)
		})

		t.Run(tcase.method+"Close is idempotent", func(t *testing.T) {
			c := NewComponent(Config{})
			win, ok := tcase.split(c, NewTestHandler())
			require.True(t, ok)
			require.NoError(t, win.Close())
			assert.NoError(t, win.Close())
		})

		t.Run(tcase.method+"close window on handler exit", func(t *testing.T) {
			c := NewComponent(Config{})
			var h Handler
			h = NewTestHandler()
			h.(*TestHandler).Exit = true
			h.(*TestHandler).Handled = true
			win, ok := tcase.split(c, h)
			require.True(t, ok)

			assert.Equal(t, 0, c.tabs.Size())
			assert.Equal(t, 2, c.wm.Size())

			exit, handled := c.Handle(term.Event{})
			require.True(t, handled)
			require.False(t, exit)

			assert.Equal(t, 0, c.tabs.Size())
			assert.Equal(t, 1, c.wm.Size())
			require.NoError(t, win.Close())
			assert.Equal(t, 1, c.wm.Size())
		})

		t.Run(tcase.method+"Close calls onWindowClosed callback", func(t *testing.T) {
			var i int
			c := NewComponent(Config{})
			win, ok := tcase.split(c, NewTestHandler())
			require.True(t, ok)
			win.onWindowClosed(func() {
				i++
			})

			require.NoError(t, win.Close())
			assert.Equal(t, 1, i)
		})
	}
}

func TestComponentRemoveAllTabs(t *testing.T) {
	w1 := NewComponent(Config{})
	w2 := NewComponent(Config{})
	w2.Split(OrientationTop, NewTestHandler())

	tsuite := []struct {
		description string
		c           *Component
		before      int
		after       int
	}{
		{"removes all tabs with one window", w1, 3, 0},
		{"removes all tabs with multiple windows", w2, 3, 0},
	}

	for _, tcase := range tsuite {
		c := tcase.c
		before := tcase.before
		after := tcase.after
		t.Run(tcase.description, func(t *testing.T) {
			uri1, err := workspace.ParseURI("file:///a")
			require.NoError(t, err)
			uri2, err := workspace.ParseURI("file:///b")
			require.NoError(t, err)
			uri3, err := workspace.ParseURI("file:///c")
			require.NoError(t, err)

			handlers := [3]testCloser{}
			c.NewTab(uri1, "a", &handlers[0], &handlers[0])
			c.NewTab(uri2, "b", &handlers[1], &handlers[1])
			c.NewTab(uri3, "c", &handlers[2], &handlers[2])
			c.EditWindowTabNextFree(c.Focus())
			c.ShiftFocus()
			c.EditWindowTabNextFree(c.Focus())

			assert.Equal(t, before, c.tabs.Size())

			for _, h := range handlers {
				assert.Equal(t, 0, h.closed)
			}

			c.RemoveAllTabs()

			assert.Equal(t, after, c.tabs.Size())
			for _, h := range handlers {
				// closer + handler
				assert.Equal(t, 2, h.closed)
			}
		})
	}
}

type testCloser struct {
	handler.TestHandler
	closed int
}

func (t *testCloser) Close() error {
	t.closed++
	return nil
}

func assertFreeTab(t *testing.T, h tui.Handler, free bool) {
	assert.Equal(t, free, h.(*Tab).free)
}

func assertWindowContent(t *testing.T, win Window, expected Handler) {
	content, err := win.Content()
	require.NoError(t, err)
	assert.Equal(t, expected, content)
}

func TestComponentSetContent(t *testing.T) {
	for _, _tcase := range splitSuite {
		tcase := _tcase
		t.Run(tcase.method, func(t *testing.T) {
			c := NewComponent(Config{})
			win0 := c.Focus()
			uri, err := workspace.ParseURI("file:///Merry_Christmas")
			require.NoError(t, err)

			christmasTab := c.NewTab(uri, "Merry Christmas", NewTestHandler(), nil)

			assertFreeTab(t, christmasTab, true)
			require.NoError(t, win0.SetContent(christmasTab))
			assertFreeTab(t, christmasTab, false)
			assertWindowContent(t, win0, christmasTab)
			assert.False(t, c.EditWindowTabNext(win0))

			h2 := NopHandler(&handler.TestHandler{})
			win1, ok := tcase.split(c, h2)
			require.True(t, ok)
			assertWindowContent(t, win1, h2)
			require.Error(t, win1.SetContent(christmasTab))
			assertWindowContent(t, win1, h2)
			assertFreeTab(t, christmasTab, false)

			h3 := NopHandler(&handler.TestHandler{})
			require.NoError(t, win1.SetContent(h3))
			assertWindowContent(t, win1, h3)
			assertFreeTab(t, christmasTab, false)

			h4 := NopHandler(&handler.TestHandler{})
			require.NoError(t, win0.SetContent(h4))
			assertWindowContent(t, win0, h4)
			assertFreeTab(t, christmasTab, true)
		})
	}
}

type testTabSubscriber struct {
	t Tab
}

func (s *testTabSubscriber) OnFocus(t *Tab) {
	s.t = *t
}

func (s *testTabSubscriber) OnFree(t *Tab) {
	s.t = *t
}

func TestComponentEditWindowTab(t *testing.T) {
	for _, _tcase := range splitSuite {
		tcase := _tcase
		t.Run(tcase.method, func(t *testing.T) {
			c := NewComponent(Config{})
			win0 := c.Focus()
			uri1, err := workspace.ParseURI("file:///AMZN")
			require.NoError(t, err)
			uri2, err := workspace.ParseURI("file:///TSLA")
			require.NoError(t, err)
			uri3, err := workspace.ParseURI("file:///GOOG")
			require.NoError(t, err)

			amzn := c.NewTab(uri1, "AMZN", NewTestHandler(), nil)
			c.EditWindowTabNextFree(win0)
			tsla := c.NewTab(uri2, "TSLA", NewTestHandler(), nil)
			goog := c.NewTab(uri3, "GOOG", NewTestHandler(), nil)
			win, ok := tcase.split(c, goog)
			require.True(t, ok)

			googSubs := testTabSubscriber{}
			goog.Subscribe(&googSubs)

			assertFreeTab(t, amzn, false)
			assertFreeTab(t, tsla, true)
			assertFreeTab(t, goog, false)
			assertFreeTab(t, &googSubs.t, false)

			for i := 0; i < 3; i++ {
				assert.True(t, c.EditWindowTabNext(win))
			}

			assertFreeTab(t, amzn, false)
			assertFreeTab(t, tsla, false)
			assertFreeTab(t, goog, true)
			assertFreeTab(t, &googSubs.t, true)

			for i := 0; i < 3; i++ {
				assert.True(t, c.EditWindowTabPrev(win0))
			}

			assertFreeTab(t, amzn, true)
			assertFreeTab(t, tsla, false)
			assertFreeTab(t, goog, false)
			assertFreeTab(t, &googSubs.t, false)

			assert.True(t, c.EditWindowTabNextFree(win0))

			assertFreeTab(t, amzn, false)
			assertFreeTab(t, tsla, false)
			assertFreeTab(t, goog, true)
			assertFreeTab(t, &googSubs.t, true)

			assert.True(t, c.EditWindowTabLastFree(win0))

			assertFreeTab(t, amzn, true)
			assertFreeTab(t, tsla, false)
			assertFreeTab(t, goog, false)
			assertFreeTab(t, &googSubs.t, false)

			assert.Error(t, win0.SetContent(tsla))
			assert.NoError(t, win0.SetContent(amzn))

			assertFreeTab(t, amzn, false)
			assertFreeTab(t, tsla, false)
			assertFreeTab(t, goog, true)
			assertFreeTab(t, &googSubs.t, true)

			assertWindowContent(t, win0, amzn)
			assertWindowContent(t, win, tsla)
		})
	}
}

func TestComponentSetContentUnmount(t *testing.T) {
	c := NewComponent(Config{})
	win0 := c.Focus()
	h1 := NewTestHandler()
	h2 := NewTestHandler()
	var closed int
	h1.CloseCallback = func() error {
		closed++
		return nil
	}
	assert.NoError(t, win0.SetContent(h1))
	assertWindowContent(t, win0, h1)
	assert.NoError(t, win0.SetContent(h2))
	assertWindowContent(t, win0, h2)
	assert.Equal(t, 1, closed)

	assert.NoError(t, win0.SetContent(h1))
	assertWindowContent(t, win0, h1)
	assert.Equal(t, 1, closed)
	content1, err := win0.Content()
	require.NoError(t, err)

	assert.NoError(t, win0.SetContent(content1))
	assertWindowContent(t, win0, h1)
	// closed and mounted again
	assert.Equal(t, 2, closed)

	win1, ok := c.Split(OrientationBottom, h2)
	require.True(t, ok)
	assert.Equal(t, 2, closed)

	assert.NoError(t, win0.Close())
	assert.Equal(t, 3, closed)

	assert.Equal(t, c.Focus(), win1)
}

func TestComponentHandlerClose(t *testing.T) {
	cfg := DefaultConfig()

	for _, _tcase := range splitSuite {
		split := _tcase.split
		t.Run(_tcase.method, func(t *testing.T) {
			tsuite := []struct {
				name string
				fn   func(*testing.T, *Component, Window, *testCloser)
			}{
				{
					"EditWindowTabNextFree",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						assert.True(t, c.EditWindowTabNextFree(win))
					},
				}, {
					"EditWindowTabNext",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						assert.True(t, c.EditWindowTabNext(win))
					},
				}, {
					"EditWindowTabPrev",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						assert.True(t, c.EditWindowTabPrev(win))
					},
				}, {
					"Window.SetContent",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						assert.NoError(t, win.SetContent(NewTestHandler()))
					},
				}, {
					"Window.Close",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						assert.NoError(t, win.Close())
					},
				}, {
					"RemoveWindowContent",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						assert.True(t, c.RemoveWindowContent(win))
					},
				}, {
					"Handle(exit=true)",
					func(t *testing.T, c *Component, win Window, h *testCloser) {
						h.Exit = true
						h.Handled = true
						exit, handled := c.Handle(term.Event{})
						assert.False(t, exit)
						assert.True(t, handled)
					},
				},
			}

			for _, tcase := range tsuite {
				t.Run(tcase.name, func(t *testing.T) {
					mock := &testCloser{}
					c := NewComponent(cfg)
					uri1, err := workspace.ParseURI("file:///Robinhood")
					require.NoError(t, err)
					uri2, err := workspace.ParseURI("file:///Stash")
					require.NoError(t, err)

					c.NewTab(uri1, "Robinhood", NewTestHandler(), nil)
					c.NewTab(uri2, "Stash", NewTestHandler(), nil)
					win, ok := split(c, mock)
					require.True(t, ok)

					tcase.fn(t, c, win, mock)
					assert.Equal(t, 1, mock.closed)
				})
			}
		})
	}
}

func TestComponentMultipleWindow(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	cfg := DefaultConfig()
	cfg.Logger = log.New()
	cfg.Logger.SetLevel(log.TraceLevel)
	cfg.WindowManagerConfig.TopLeft = 'O'
	cfg.WindowManagerConfig.FocusFrameCharSet.TopLeft = 'X'
	c := NewComponent(cfg)
	c.Resize(20, 8)

	// w1 := c.Focus()
	var w2 Window
	var ok bool
	// var w3 Window
	h2 := NewTestHandler()
	h3 := NewTestHandler()
	h3.Ch = 'C'

	tests := []testutil.ComponentTestCase{
		{
			nil /* X on top left window is overriden by union */, `
O──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
		}, {func() {
			w2, ok = c.Split(OrientationBottom, h2)
			require.True(t, ok)
		}, `
O──────────────────┐
│                  │
├──────────────────┤
│                  │
└──────────────────┘
X──────────────────┐
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`,
		}, {func() {
			/*w3 =*/ c.Split(OrientationRight, h3)
		}, `
O──────────────────┐
│                  │
├──────────────────┤
│                  │
└──────────────────┘
O────────┐X────────┐
│AAAAAAAA││CCCCCCCC│
└────────┘└────────┘`,
		}, {func() {
			assert.True(t, c.FocusLeft())

			var closeCallbacked int
			var closed int
			h2.Exit = true
			w2.onWindowClosed(func() {
				closed++
			})
			h2.CloseCallback = func() error {
				assert.NoError(t, w2.Close())
				closeCallbacked++
				return nil
			}
			h2.Handled = true
			_, handled := c.Handle(term.Event{})
			assert.True(t, handled)
			assert.Equal(t, 1, closeCallbacked)
			assert.Equal(t, 1, closed)
		}, /* same as case 1, topleft on window X is overriden */ `
O──────────────────┐
│                  │
├──────────────────┤
│                  │
└──────────────────┘
O──────────────────┐
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		},
	}

	testutil.TestComponent(t, c, w, tests)
}

func TestComponentSetMessage(t *testing.T) {
	w := term.NewStringWriter(24, 8)
	cfg := DefaultConfig()
	c := NewComponent(cfg)
	c.Resize(20, 8)

	c.SetMessage("wasup: %s", "hola")
	c.SetMessage("wasup: %s", "holaaaaaaaaaaaaaaaaaaaaaaaaa")

	expected := "wasup: holaaaaaaaaaaaaaaaaaaaaaaaaa"
	assert.Equal(t, expected, c.logBuf.String())

	tests := []testutil.ComponentTestCase{
		{
			nil, `
┌──────────────────┐    
│                  │    
├──────────────────┤    
│                  │    
│                  │    
│wasup: holaaaaaaaa│    
│aaaaaaaaaaaaaaaaa │    
└──────────────────┘    `,
		},
	}

	testutil.TestComponent(t, c, w, tests)
}

func TestComponentPrompt(t *testing.T) {
	w := term.NewStringWriter(24, 8)

	cfg := DefaultConfig()
	cfg.PromptConfig.Width = 18
	cfg.PromptConfig.Height = 7
	c := NewComponent(cfg)
	c.Resize(20, 8)

	tests := []testutil.ComponentTestCase{
		{
			nil, `
┌──────────────────┐    
│                  │    
├──────────────────┤    
│                  │    
│                  │    
│                  │    
│                  │    
└──────────────────┘    `,
		}, {func() {
			c.Prompt("Virgen Maria?", []string{"Boh", "Meh"}, nil, func(int, string) {})
		}, `
┌┌────────────────┐┐    
││ Virgen Maria?  ││    
├│                │┤    
││┌─────┐ ┌─────┐ ││    
│││ Boh │ │ Meh │ ││    
││└─────┘ └─────┘ ││    
│└────────────────┘│    
└──────────────────┘    `,
		}, {func() {
			c.Handle(term.Event{Key: term.KeyEnter})
		}, `
┌──────────────────┐    
│                  │    
├──────────────────┤    
│                  │    
│                  │    
│                  │    
│                  │    
└──────────────────┘    `,
		}, {func() {
			c.Prompt("Tokischa?", []string{"Yay", "Nay"}, nil, func(int, string) {})
			c.Prompt("Rosalia?", []string{"Yay", "Nay"}, nil, func(int, string) {})
		}, `
┌┌────────────────┐┐    
││    Rosalia?    ││    
├│                │┤    
││┌─────┐ ┌─────┐ ││    
│││ Yay │ │ Nay │ ││    
││└─────┘ └─────┘ ││    
│└────────────────┘│    
└──────────────────┘    `,
		}, {func() {
			c.Resize(10, 6)
		}, `
┌────────┐              
│Rosalia?│              
│        │              
│Yay Nay │              
│        │              
└────────┘              
                        
                        `,
		}, {func() {
			c.Resize(24, 8)
		}, `
┌──┌────────────────┐──┐
│  │    Rosalia?    │  │
├──│                │──┤
│  │┌─────┐ ┌─────┐ │  │
│  ││ Yay │ │ Nay │ │  │
│  │└─────┘ └─────┘ │  │
│  └────────────────┘  │
└──────────────────────┘`,
		}, {func() {
			c.Resize(20, 8)
		}, `
┌┌────────────────┐┐    
││    Rosalia?    ││    
├│                │┤    
││┌─────┐ ┌─────┐ ││    
│││ Yay │ │ Nay │ ││    
││└─────┘ └─────┘ ││    
│└────────────────┘│    
└──────────────────┘    `,
		}, {func() {
			c.Handle(term.Event{Key: term.KeyEsc})
		}, `
┌┌────────────────┐┐    
││   Tokischa?    ││    
├│                │┤    
││┌─────┐ ┌─────┐ ││    
│││ Yay │ │ Nay │ ││    
││└─────┘ └─────┘ ││    
│└────────────────┘│    
└──────────────────┘    `,
		}, {func() {
			c.Handle(term.Event{Key: term.KeyEnter})
		}, `
┌──────────────────┐    
│                  │    
├──────────────────┤    
│                  │    
│                  │    
│                  │    
│                  │    
└──────────────────┘    `,
		},
	}

	testutil.TestComponent(t, c, w, tests)
}

func TestComponentSplitNil(t *testing.T) {
	t.Run("Split with nil sets a wallpaper", func(t *testing.T) {
		w := term.NewStringWriter(24, 8)
		cfg := DefaultConfig()
		cfg.Wallpaper = "BART"
		c := NewComponent(cfg)
		c.Resize(20, 8)
		c.Split(OrientationLeft, nil)
		tests := []testutil.ComponentTestCase{
			{
				nil, `
┌──────────────────┐    
│                  │    
├────────┐┌────────┤    
│        ││        │    
│  BART  ││  BART  │    
│        ││        │    
│        ││        │    
└────────┘└────────┘    `,
			},
		}
		testutil.TestComponent(t, c, w, tests)
	})
}
