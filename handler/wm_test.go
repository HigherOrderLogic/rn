package handler

import (
	"testing"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func altEvent(ch rune) term.Event {
	return term.Event{
		Type: term.EventKey,
		Mod:  term.ModAlt,
		Ch:   ch,
	}
}

func prepareTest(width, height int, frame bool, root tui.Handler) (
	*term.StringWriter, *WindowManager,
) {
	writer := term.NewStringWriter(width, height)
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	handler := NewWindowManager(root, cfg)
	handler.Resize(width, height)

	return writer, handler
}

func TestWindowManagerSetFocusFrame(t *testing.T) {
	testWindowManagerSetFocus(t, true)
}
func TestWindowManagerSetFocusNoFrame(t *testing.T) {
	testWindowManagerSetFocus(t, false)
}

func testWindowManagerSetFocus(t *testing.T, frame bool) {
	width, height := 8, 4
	_, wm := prepareTest(width, height, frame, NewTestHandler())
	right, ok := wm.SplitHorizontal(NewTestHandler())
	require.True(t, ok)

	wm.SetFocus(right)

	if focus := wm.Focus(); focus != right {
		t.Errorf("focus should be %+v, instead of %+v", right, focus)
	}
	_, ok = wm.Focus().Content().(*TestHandler)
	assert.True(t, ok)
}

// TestHandler signals that it's handling event by incrementing it's fill rune
func TestWindowManagerHandle(t *testing.T) {
	t.Run("passes correct mouse position", func(t *testing.T) {
		handler := NewTestHandler()

		var actualEv term.Event
		handler.HandleOverride = func(ev term.Event) (bool, bool) {
			actualEv = ev
			return false, false
		}
		width, height := 8, 4
		cfg := DefaultWindowManagerConfig()
		cfg.Frame = true
		wm := NewWindowManager(handler, cfg)
		wm.Resize(width, height)

		wm.Handle(term.Event{Type: term.EventMouse})
		require.Equal(t, term.EventMouse, actualEv.Type)
		assert.Equal(t, 0, actualEv.MouseX)
		assert.Equal(t, 0, actualEv.MouseY)

		actualEv = term.Event{}
		wm.Handle(term.Event{Type: term.EventMouse, MouseX: 1, MouseY: 1})
		require.Equal(t, term.EventMouse, actualEv.Type)
		assert.Equal(t, 0, actualEv.MouseX)
		assert.Equal(t, 0, actualEv.MouseY)

		actualEv = term.Event{}
		wm.Handle(term.Event{Type: term.EventMouse, MouseX: 2, MouseY: 2})
		require.Equal(t, term.EventMouse, actualEv.Type)
		assert.Equal(t, 1, actualEv.MouseX)
		assert.Equal(t, 1, actualEv.MouseY)
	})

	t.Run("delegates to correct children", func(t *testing.T) {
		topLeftHandler := NewTestHandler()
		width, height := 8, 4
		writer, handler := prepareTest(width, height, false, topLeftHandler)

		bottomLeftHandler := NewTestHandler()
		bottomleft, ok := handler.SplitHorizontal(bottomLeftHandler)
		require.True(t, ok)

		topRightHandler := NewTestHandler()
		_, ok = handler.SplitVertical(topRightHandler)
		require.True(t, ok)

		topLeft := handler.SetFocus(bottomleft)
		bottomRightHandler := NewTestHandler()
		_, ok = handler.SplitVertical(bottomRightHandler)
		require.True(t, ok)

		handler.SetFocus(topLeft)

		cases := []testutil.HandlerTestCase{
			{
				term.Event{}, `
BBBBAAAA
BBBBAAAA
AAAAAAAA
AAAAAAAA`,
			},
			{
				altEvent('l'), `
BBBBAAAA
BBBBAAAA
AAAAAAAA
AAAAAAAA`,
			},
			{
				term.Event{}, `
BBBBBBBB
BBBBBBBB
AAAAAAAA
AAAAAAAA`,
			},
			{
				altEvent('l'), `
BBBBBBBB
BBBBBBBB
AAAAAAAA
AAAAAAAA`,
			},
			{
				term.Event{}, `
BBBBCCCC
BBBBCCCC
AAAAAAAA
AAAAAAAA`,
			},
			{
				altEvent('h'), `
BBBBCCCC
BBBBCCCC
AAAAAAAA
AAAAAAAA`,
			},
			{
				term.Event{}, `
CCCCCCCC
CCCCCCCC
AAAAAAAA
AAAAAAAA`,
			},
			{
				altEvent('j'), `
CCCCCCCC
CCCCCCCC
AAAAAAAA
AAAAAAAA`,
			},
			{
				term.Event{}, `
CCCCCCCC
CCCCCCCC
BBBBAAAA
BBBBAAAA`,
			},
			{
				altEvent('l'), `
CCCCCCCC
CCCCCCCC
BBBBAAAA
BBBBAAAA`,
			},
			{
				term.Event{}, `
CCCCCCCC
CCCCCCCC
BBBBBBBB
BBBBBBBB`,
			},
			{
				term.Event{}, `
CCCCCCCC
CCCCCCCC
BBBBCCCC
BBBBCCCC`,
			},
			{
				altEvent('k'), `
CCCCCCCC
CCCCCCCC
BBBBCCCC
BBBBCCCC`,
			},
			{
				term.Event{}, `
CCCCDDDD
CCCCDDDD
BBBBCCCC
BBBBCCCC`,
			},
		}

		testutil.TestHandler(t, handler, cases, writer)

		topRightHandler.Exit = true

		cases = []testutil.HandlerTestCase{
			{
				// testhandler will return after this event active = false
				term.Event{}, `
CCCCCCCC
CCCCCCCC
BBBBCCCC
BBBBCCCC`,
			},
			{
				term.Event{}, `
DDDDDDDD
DDDDDDDD
BBBBCCCC
BBBBCCCC`,
			},
			{
				altEvent('j'), `
DDDDDDDD
DDDDDDDD
BBBBCCCC
BBBBCCCC`,
			},
			{
				altEvent('h'), `
DDDDDDDD
DDDDDDDD
BBBBCCCC
BBBBCCCC`,
			},
			{
				term.Event{}, `
DDDDDDDD
DDDDDDDD
CCCCCCCC
CCCCCCCC`,
			},
			{
				term.Event{}, `
DDDDDDDD
DDDDDDDD
DDDDCCCC
DDDDCCCC`,
			},
		}

		testutil.TestHandler(t, handler, cases, writer)

		bottomLeftHandler.Exit = true

		cases = []testutil.HandlerTestCase{
			{
				// testhandler will return after this event active = false
				term.Event{}, `
DDDDDDDD
DDDDDDDD
CCCCCCCC
CCCCCCCC`,
			},
			{
				altEvent('k'), `
DDDDDDDD
DDDDDDDD
CCCCCCCC
CCCCCCCC`,
			},
			{
				term.Event{}, `
EEEEEEEE
EEEEEEEE
CCCCCCCC
CCCCCCCC`,
			},
		}

		testutil.TestHandler(t, handler, cases, writer)

		topLeftHandler.Exit = true

		cases = []testutil.HandlerTestCase{
			{
				// testhandler will return after this event active = false
				term.Event{}, `
CCCCCCCC
CCCCCCCC
CCCCCCCC
CCCCCCCC`,
			},
			{
				term.Event{}, `
DDDDDDDD
DDDDDDDD
DDDDDDDD
DDDDDDDD`,
			},
			{
				altEvent('k'), `
DDDDDDDD
DDDDDDDD
DDDDDDDD
DDDDDDDD`,
			},
			{
				altEvent('l'), `
DDDDDDDD
DDDDDDDD
DDDDDDDD
DDDDDDDD`,
			},
			{
				term.Event{}, `
EEEEEEEE
EEEEEEEE
EEEEEEEE
EEEEEEEE`,
			},
		}

		testutil.TestHandler(t, handler, cases, writer)
	})
}

func TestWindowManagerHandleFrame(t *testing.T) {
	leftHandler := NewTestHandler()
	width, height := 12, 4
	writer, handler := prepareTest(width, height, true, leftHandler)

	rightHandler := NewTestHandler()
	_, ok := handler.SplitVertical(rightHandler)
	require.True(t, ok)

	cases := []testutil.HandlerTestCase{
		{
			term.Event{}, `
┌────┐┌────┐
│BBBB││AAAA│
│BBBB││AAAA│
└────┘└────┘`,
		},
		{
			altEvent('l'), `
┌────┐┌────┐
│BBBB││AAAA│
│BBBB││AAAA│
└────┘└────┘`,
		},
	}

	testutil.TestHandler(t, handler, cases, writer)
}

func TestWindowFocusInitSplitVertical(t *testing.T) {
	leftHandler := NewTestHandler()
	width, height := 12, 4
	_, m := prepareTest(width, height, true, leftHandler)

	_, ok := m.Focus().TileLeft()
	assert.False(t, ok)
	_, ok = m.Shiftable()
	assert.False(t, ok)
	assert.False(t, m.ShiftFocus())

	rightHandler := NewTestHandler()
	_, ok = m.SplitVertical(rightHandler)
	require.True(t, ok)

	assert.Equal(t, leftHandler, m.Focus().Content())

	nextFocus, ok := m.Shiftable()
	assert.True(t, ok)
	assert.Equal(t, rightHandler, nextFocus.Content())
	assert.True(t, m.ShiftFocus())
	assert.Equal(t, rightHandler, m.Focus().Content())

	nextFocus, ok = m.Shiftable()
	assert.True(t, ok)
	assert.Equal(t, leftHandler, nextFocus.Content())
	assert.True(t, m.ShiftFocus())
	assert.Equal(t, leftHandler, m.Focus().Content())

	assert.Equal(t, 1, m.Focus().Size())
	assert.Equal(t, 6, m.Focus().Width())
}

func TestWindowManagerSetFocusContent(t *testing.T) {
	leftHandler := NewTestHandler()
	width, height := 8, 4
	writer, wm := prepareTest(width, height, true, leftHandler)

	rightHandler := NewTestHandler()
	_, ok := wm.SplitVertical(rightHandler)
	require.True(t, ok)

	prev := wm.Focus().SetContent(rightHandler)
	assert.Equal(t, prev, leftHandler)

	cases := []testutil.HandlerTestCase{
		{
			term.Event{}, `
┌──┐┌──┐
│BB││BB│
│BB││BB│
└──┘└──┘`,
		},
		{
			altEvent('l'), `
┌──┐┌──┐
│BB││BB│
│BB││BB│
└──┘└──┘`,
		},
		{
			term.Event{}, `
┌──┐┌──┐
│CC││CC│
│CC││CC│
└──┘└──┘`,
		},
	}

	testutil.TestHandler(t, wm, cases, writer)

	fb := component.FrameCharSet{}
	fb.TopLeft = '╔'
	fb.BottomRight = '╝'
	fb.BottomLeft = '╚'
	fb.TopRight = '╗'

	fb.VerticalLeft = '║'
	fb.VerticalRight = '║'
	fb.HorizontalTop = '═'
	fb.HorizontalBottom = '═'

	wm.SetFrameCharSet(component.FrameCharSetDefault(), fb)

	cases = []testutil.HandlerTestCase{
		{
			term.Event{}, `
┌──┐╔══╗
│DD│║DD║
│DD│║DD║
└──┘╚══╝`,
		},
	}

	testutil.TestHandler(t, wm, cases, writer)
}

func TestWindowManagerInit(t *testing.T) {
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = false
	wm := NewWindowManager(NewTestHandler(), cfg)
	require.NotNil(t, wm.Focus())
}

func TestWindowManagerSetAttr(t *testing.T) {
	wm := NewWindowManager(NewTestHandler(), DefaultWindowManagerConfig())
	cyan := term.ColorCyan
	red := term.ColorRed

	wm.SplitHorizontal(NewTestHandler())
	wm.SetAttr(term.Attributes{Bg: cyan, Fg: red}, term.Attributes{Bg: red, Fg: cyan})

	focus := wm.Focus()
	b, ok := focus.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, red, b.Bg)
	assert.Equal(t, cyan, b.Fg)

	wm.ShiftFocus()
	b, ok = focus.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, cyan, b.Bg)
	assert.Equal(t, red, b.Fg)
}

func testWindowManagerClose(t *testing.T, frame bool) {
	h1 := NewTestHandler()
	h2 := NewTestHandler()
	h2.Ch = 'D' // different char to enable assert.Equal
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(h1, cfg)
	node2, ok := wm.SplitHorizontal(h2)
	require.True(t, ok)

	assert.NotEqual(t, node2, wm.Focus())
	require.NoError(t, wm.Focus().Close())
	assert.Equal(t, node2, wm.Focus())
}

func TestWindowManagerClose(t *testing.T) {
	t.Run("Close with frame", func(t *testing.T) {
		testWindowManagerClose(t, true)
	})

	t.Run("Close without frame", func(t *testing.T) {
		testWindowManagerClose(t, false)
	})
}

func testWindowManagerContent(t *testing.T, frame bool) {
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(NewTestHandler(), cfg)
	node2, ok := wm.SplitHorizontal(NewTestHandler())
	require.True(t, ok)

	c := node2.Content()
	_, ok = c.(*TestHandler)
	require.True(t, ok)

	prev := node2.SetContent(NewTestHandler())
	_, ok = prev.(*TestHandler)
	require.True(t, ok)

	c = node2.Content()
	_, ok = c.(*TestHandler)
	require.True(t, ok)
}

func TestWindowManagerContent(t *testing.T) {
	t.Run("Content with frame", func(t *testing.T) {
		testWindowManagerContent(t, true)
	})
	t.Run("Content without frame", func(t *testing.T) {
		testWindowManagerContent(t, false)
	})
}

func testWindowManagerCursor(
	t *testing.T, frame bool,
	input, expected term.Coordinates,
) {
	handler := NewTestHandler()
	handler.CursorPos = input
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(handler, cfg)
	wm.Resize(10, 10)

	pos, ok := wm.Cursor()
	require.True(t, ok)
	assert.Equal(t, expected, pos)
}

func TestWindowManagerCursor(t *testing.T) {
	input := term.Coordinates{X: 1, Y: 2}
	t.Run("Cursor with frame", func(t *testing.T) {
		testWindowManagerCursor(t, true, input,
			term.Coordinates{X: 2, Y: 3})
	})

	t.Run("Cursor without frame", func(t *testing.T) {
		testWindowManagerCursor(t, false, input, input)
	})
}

func TestHandlerWindowZeroValue(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		var win Window
		assert.NotPanics(t, func() {
			win.Close()
		})
	})
	t.Run("Content", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			_ = win.Content()
		})
	})
	t.Run("SetContent", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			win.SetContent(NewTestHandler())
		})
	})
	t.Run("Size", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			win.Size()
		})
	})
	t.Run("TileDirection", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			win.TileDown()
		})
	})
}

func TestComponentWindowSplit(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	h1 := TestHandler{TestComponent: component.TestComponent{Ch: 'A'}}
	wm := NewWindowManager(&h1, DefaultWindowManagerConfig())
	w1 := wm.Focus()
	wm.Resize(20, 8)

	var w2 Window
	var w3 Window
	var ok bool
	h2 := TestHandler{TestComponent: component.TestComponent{Ch: 'B'}}
	h3 := TestHandler{TestComponent: component.TestComponent{Ch: 'C'}}

	tests := []testutil.ComponentTestCase{
		{
			nil, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`,
		}, {func() {
			w2, ok = wm.SplitHorizontal(&h2)
			require.True(t, ok)
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
┌──────────────────┐
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`,
		}, {func() {
			assert.True(t, wm.FocusDown())
			w3, ok = wm.SplitVertical(&h3)
			require.True(t, ok)
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
┌────────┐┌────────┐
│BBBBBBBB││CCCCCCCC│
│BBBBBBBB││CCCCCCCC│
└────────┘└────────┘`,
		}, {func() {
			h2.HandleOverride = func(ev term.Event) (bool, bool) {
				assert.NoError(t, w2.Close())
				return true, true
			}
			exit, handled := wm.Handle(term.Event{})
			assert.False(t, exit)
			assert.True(t, handled)
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			wm.FocusUp()
			h1.HandleOverride = func(ev term.Event) (bool, bool) {
				assert.NoError(t, w1.Close())
				return true, true
			}
			exit, handled := wm.Handle(term.Event{})
			assert.False(t, exit)
			assert.True(t, handled)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			h3.HandleOverride = func(ev term.Event) (bool, bool) {
				assert.Error(t, w3.Close())
				return true, true
			}
			exit, handled := wm.Handle(term.Event{})
			assert.True(t, exit)
			assert.True(t, handled)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			cfg := DefaultWindowManagerConfig()
			cfg.FocusFrameCharSet.TopLeft = 'X'
			wm.SetFrameCharSet(cfg.FrameCharSet, cfg.FocusFrameCharSet)
		}, `
X──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		},
	}

	testutil.TestComponent(t, wm, w, tests)
}
