package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	testutil "unstable.build/go-tui/util/test"
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
	right, ok := wm.SplitHorizontal(wm.Focus(), NewTestHandler())
	require.True(t, ok)

	assert.False(t, right.Focus())
	wm.SetFocus(right)
	assert.True(t, right.Focus())

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
}

func TestWindowManagerHandleFrame(t *testing.T) {
	leftHandler := NewTestHandler()
	width, height := 12, 4
	writer, handler := prepareTest(width, height, true, leftHandler)

	rightHandler := NewTestHandler()
	_, ok := handler.SplitVertical(handler.Focus(), rightHandler)
	require.True(t, ok)

	cases := []testutil.HandlerTestCase{
		{
			term.Event{}, `
┌────┐┌────┐
│AAAA││BBBB│
│AAAA││BBBB│
└────┘└────┘`,
		},
	}

	require.True(t, handler.FocusRight())

	testutil.TestHandler(t, handler, cases, writer)

	cases = []testutil.HandlerTestCase{
		{
			term.Event{}, `
┌────┐┌────┐
│AAAA││CCCC│
│AAAA││CCCC│
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
	_, ok = m.SplitVertical(m.Focus(), rightHandler)
	require.True(t, ok)

	assert.Equal(t, leftHandler, m.Focus().Content())
}

func TestWindowShiftFocusFocusPrev(t *testing.T) {
	leftHandler := NewTestHandler()
	width, height := 12, 4
	_, m := prepareTest(width, height, true, leftHandler)
	orig := m.Focus()

	rightHandler := NewTestHandler()
	_, ok := m.SplitVertical(m.Focus(), rightHandler)
	require.True(t, ok)

	_, ok = m.SplitVertical(orig, NewTestHandler())
	require.True(t, ok)

	assert.True(t, m.ShiftFocus())
	assert.Equal(t, rightHandler, m.Focus().Content())
}

func TestWindowManagerSetFocusContent(t *testing.T) {
	leftHandler := NewTestHandler()
	width, height := 8, 4
	writer, wm := prepareTest(width, height, true, leftHandler)

	rightHandler := NewTestHandler()
	_, ok := wm.SplitVertical(wm.Focus(), rightHandler)
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
	}

	require.True(t, wm.FocusRight())

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
│CC│║CC║
│CC│║CC║
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

	wm.SplitHorizontal(wm.Focus(), NewTestHandler())
	wm.SetAttr(term.Attributes{Bg: cyan, Fg: red}, term.Attributes{Bg: red, Fg: cyan})

	w1 := wm.Focus()
	b, ok := w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, red, b.Bg)
	assert.Equal(t, cyan, b.Fg)

	w1.SetContent(NewTestHandler())
	b, ok = w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, red, b.Bg)
	assert.Equal(t, cyan, b.Fg)

	wm.ShiftFocus()
	b, ok = w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, cyan, b.Bg)
	assert.Equal(t, red, b.Fg)

	w1.SetContent(NewTestHandler())
	b, ok = w1.FrameAttr()
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
	node2, ok := wm.SplitHorizontal(wm.Focus(), h2)
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
	node2, ok := wm.SplitHorizontal(wm.Focus(), NewTestHandler())
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

	pos, _, ok := wm.Cursor()
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

type testWindowSubscriber struct {
	lastPrev  Window
	lastFocus Window
}

func (w *testWindowSubscriber) reset() {
	w.lastPrev = Window{}
	w.lastFocus = Window{}
}

func (w *testWindowSubscriber) OnFocus(prev, focus Window) {
	w.lastPrev = prev
	w.lastFocus = focus
}

func TestWindowManagerSubscribe(t *testing.T) {
	h1 := NewTestHandler()
	h2 := NewTestHandler()
	wm := NewWindowManager(h1, DefaultWindowManagerConfig())
	mock := new(testWindowSubscriber)

	w1 := wm.Focus()
	wm.Subscribe(mock)
	assert.Zero(t, mock.lastPrev)
	assert.Equal(t, w1.ID(), mock.lastFocus.ID())

	w2, ok := wm.SplitVertical(wm.Focus(), h2)
	require.True(t, ok)
	wm.SetFocus(w2)
	assert.Equal(t, w1.ID(), mock.lastPrev.ID())
	assert.Equal(t, w2.ID(), mock.lastFocus.ID())

	wm.SetFocus(w1)
	assert.Equal(t, w2.ID(), mock.lastPrev.ID())
	assert.Equal(t, w1.ID(), mock.lastFocus.ID())

	mock.reset()
	wm.SetFocus(w1)
	assert.Zero(t, mock.lastPrev)
	assert.Zero(t, mock.lastFocus)

	mock.reset()
	wm.UnsubscribeAll()
	wm.SetFocus(w1)
	assert.Zero(t, mock.lastPrev)
	assert.Zero(t, mock.lastFocus)
}

func TestWindowManagerSplit(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	h1 := TestHandler{TestComponent: component.TestComponent{Ch: 'A'}}
	wm := NewWindowManager(&h1, DefaultWindowManagerConfig())
	w1 := wm.Focus()
	wm.Resize(20, 8)

	cfg := DefaultWindowManagerConfig()
	cfg.FocusFrameCharSet.TopLeft = 'A'
	cfg.FocusFrameCharSet.TopRight = 'B'
	cfg.FocusFrameCharSet.BottomLeft = 'C'
	cfg.FocusFrameCharSet.BottomRight = 'D'
	wm.SetFrameCharSet(cfg.FrameCharSet, cfg.FocusFrameCharSet)

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
			w2, ok = wm.SplitHorizontal(wm.Focus(), &h2)
			require.True(t, ok)
		}, `
A──────────────────B
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
C──────────────────D
┌──────────────────┐
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`,
		}, {func() {
			assert.True(t, wm.FocusDown())
			w3, ok = wm.SplitVertical(wm.Focus(), &h3)
			require.True(t, ok)
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
A────────B┌────────┐
│BBBBBBBB││CCCCCCCC│
│BBBBBBBB││CCCCCCCC│
C────────D└────────┘`,
		}, {func() {
			h2.HandleOverride = func(ev term.Event) (bool, bool) {
				assert.NoError(t, w2.Close())
				return true, true
			}
			exit, handled := wm.Handle(term.Event{})
			assert.False(t, exit)
			assert.True(t, handled)
			assert.True(t, w2.Closed())
		}, `
A──────────────────B
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
C──────────────────D
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
			assert.True(t, w1.Closed())
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
			hf := &TestHandler{TestComponent: component.TestComponent{Ch: 'F'}}
			wfloat := wm.FloatingWindow(StaticFloating(hf, 2, 2), component.FloatingConfig{})
			wm.SetFocus(wfloat)
			hf.HandleOverride = func(ev term.Event) (bool, bool) {
				// test that tiled window doesn't attempt to close last node
				// when there's a floating window
				require.Equal(t, 1, wm.SizeTiles())
				require.Equal(t, 1, wm.SizeFloating())
				assert.Error(t, w3.Close())
				assert.NoError(t, wfloat.Close())
				return true, true
			}
			exit, handled := wm.Handle(term.Event{})
			assert.False(t, exit)
			assert.True(t, handled)
			assert.False(t, w3.Closed())
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
			hf := &TestHandler{TestComponent: component.TestComponent{Ch: 'F'}}
			wfloat := wm.FloatingWindow(StaticFloating(hf, 2, 2), component.FloatingConfig{})
			wm.SetFocus(wfloat)
			hf.HandleOverride = func(ev term.Event) (bool, bool) {
				// test close itself and focus left
				assert.NoError(t, wm.Focus().Close())
				prevf := wm.SetFocus(w3)
				assert.Equal(t, w3, prevf)
				assert.False(t, wm.FocusLeft())
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
			// test that Dimensions are updated for a floating win
			buf := cell.NewBuffer()
			buf.WriteString("1234")
			hf := Nop(component.Buffer(buf, component.StringResponsiveConfig{}))
			wfloat := wm.FloatingWindow(FloatingBuffer(hf, buf), component.FloatingConfig{})
			wm.SetFocus(wfloat)
			buf.WriteString("1234")
		}, `
A────────B─────────┐
│12341234│CCCCCCCCC│
C────────DCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		},
	}

	testutil.TestComponent(t, wm, w, tests)
}
