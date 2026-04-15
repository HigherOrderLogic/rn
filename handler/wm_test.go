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

package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	compapi "github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler/handlertest"
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
	cfg.ScrollBarChar = '|'
	cfg.ScrollBarHoverChar = 'X'
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
	_, wm := prepareTest(width, height, frame, handler.NewTestHandler())
	right, ok := wm.SplitHorizontal(wm.Focus(), handler.NewTestHandler())
	require.True(t, ok)

	assert.False(t, right.Focus())
	wm.SetFocus(right)
	assert.True(t, right.Focus())

	if focus := wm.Focus(); focus != right {
		t.Errorf("focus should be %+v, instead of %+v", right, focus)
	}
	_, ok = wm.Focus().Content().(*handler.TestHandler)
	assert.True(t, ok)
}

// TestHandler signals that it's handling event by incrementing it's fill rune
func TestWindowManagerHandle(t *testing.T) {
	t.Run("passes correct mouse position", func(t *testing.T) {
		handler := handler.NewTestHandler()

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
	leftHandler := handler.NewTestHandler()
	width, height := 12, 4
	writer, h := prepareTest(width, height, true, leftHandler)

	rightHandler := handler.NewTestHandler()
	_, ok := h.SplitVertical(h.Focus(), rightHandler)
	require.True(t, ok)

	cases := []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────┐┌────┐
│AAAA││BBBB│
│AAAA││BBBB│
└────┘└────┘`,
		},
	}

	require.True(t, h.FocusRight())

	handlertest.TestHandler(t, h, cases, writer)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────┐┌────┐
│AAAA││CCCC│
│AAAA││CCCC│
└────┘└────┘`,
		},
	}

	handlertest.TestHandler(t, h, cases, writer)
}

func TestWindowFocusInitSplitVertical(t *testing.T) {
	leftHandler := handler.NewTestHandler()
	width, height := 12, 4
	_, m := prepareTest(width, height, true, leftHandler)

	_, ok := m.Focus().TileLeft()
	assert.False(t, ok)
	_, ok = m.Shiftable()
	assert.False(t, ok)
	assert.False(t, m.ShiftFocus())

	rightHandler := handler.NewTestHandler()
	_, ok = m.SplitVertical(m.Focus(), rightHandler)
	require.True(t, ok)

	assert.Equal(t, leftHandler, m.Focus().Content())
}

func TestSwapContent(t *testing.T) {
	t.Run("swaps content left", func(t *testing.T) {
		leftHandler := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '1'}}
		width, height := 12, 4
		_, m := prepareTest(width, height, true, leftHandler)

		w1 := m.Focus()

		w2, ok := m.SplitVertical(w1,
			&handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '2'}})
		require.True(t, ok)

		m.SetFocus(w2)

		assert.True(t, m.SwapContentLeft())

		assert.Equal(t, '1', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '2', w1.Content().(*handler.TestHandler).TestComponent.Ch)

		assert.True(t, m.SwapContentLeft())

		assert.Equal(t, '2', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '1', w1.Content().(*handler.TestHandler).TestComponent.Ch)
	})

	t.Run("swaps content right", func(t *testing.T) {
		leftHandler := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '1'}}
		width, height := 12, 4
		_, m := prepareTest(width, height, true, leftHandler)

		w1 := m.Focus()

		w2, ok := m.SplitVertical(w1,
			&handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '2'}})
		require.True(t, ok)

		assert.True(t, m.SwapContentRight())

		assert.Equal(t, '1', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '2', w1.Content().(*handler.TestHandler).TestComponent.Ch)

		assert.True(t, m.SwapContentRight())

		assert.Equal(t, '2', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '1', w1.Content().(*handler.TestHandler).TestComponent.Ch)
	})

	t.Run("swaps content down", func(t *testing.T) {
		leftHandler := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '1'}}
		width, height := 12, 4
		_, m := prepareTest(width, height, true, leftHandler)

		w1 := m.Focus()

		w2, ok := m.SplitHorizontal(w1,
			&handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '2'}})
		require.True(t, ok)

		assert.True(t, m.SwapContentDown())

		assert.Equal(t, '1', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '2', w1.Content().(*handler.TestHandler).TestComponent.Ch)

		assert.True(t, m.SwapContentDown())

		assert.Equal(t, '2', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '1', w1.Content().(*handler.TestHandler).TestComponent.Ch)
	})

	t.Run("swaps content up", func(t *testing.T) {
		leftHandler := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '1'}}
		width, height := 12, 4
		_, m := prepareTest(width, height, true, leftHandler)

		w1 := m.Focus()

		w2, ok := m.SplitHorizontal(w1,
			&handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '2'}})
		require.True(t, ok)

		m.SetFocus(w2)

		assert.True(t, m.SwapContentUp())

		assert.Equal(t, '1', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '2', w1.Content().(*handler.TestHandler).TestComponent.Ch)

		assert.True(t, m.SwapContentUp())

		assert.Equal(t, '2', w2.Content().(*handler.TestHandler).TestComponent.Ch)
		assert.Equal(t, '1', w1.Content().(*handler.TestHandler).TestComponent.Ch)
	})

	t.Run("does not swap floating window content", func(t *testing.T) {
		leftHandler := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '1'}}
		width, height := 12, 4
		_, m := prepareTest(width, height, true, leftHandler)

		w2 := m.FloatingWindow(handler.NewTestFloating(0, 0), component.FloatingConfig{})
		m.SetFocus(w2)

		assert.False(t, m.SwapContentUp())
		assert.False(t, m.SwapContentDown())
		assert.False(t, m.SwapContentLeft())
		assert.False(t, m.SwapContentRight())
	})

	t.Run("does not tile content into floating window", func(t *testing.T) {
		leftHandler := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: '1'}}
		width, height := 12, 4
		_, m := prepareTest(width, height, true, leftHandler)

		_ = m.FloatingWindow(handler.NewTestFloating(0, 0), component.FloatingConfig{})

		assert.False(t, m.SwapContentUp())
		assert.False(t, m.SwapContentDown())
		assert.False(t, m.SwapContentLeft())
		assert.False(t, m.SwapContentRight())
	})
}

func TestWindowShiftFocusFocusPrev(t *testing.T) {
	leftHandler := handler.NewTestHandler()
	width, height := 12, 4
	_, m := prepareTest(width, height, true, leftHandler)
	orig := m.Focus()

	rightHandler := handler.NewTestHandler()
	_, ok := m.SplitVertical(m.Focus(), rightHandler)
	require.True(t, ok)

	_, ok = m.SplitVertical(orig, handler.NewTestHandler())
	require.True(t, ok)

	assert.True(t, m.ShiftFocus())
	assert.Equal(t, rightHandler, m.Focus().Content())
}

func TestWindowManagerSetFocusContent(t *testing.T) {
	leftHandler := handler.NewTestHandler()
	width, height := 8, 4
	writer, wm := prepareTest(width, height, true, leftHandler)

	rightHandler := handler.NewTestHandler()
	_, ok := wm.SplitVertical(wm.Focus(), rightHandler)
	require.True(t, ok)

	prev := wm.Focus().SetContent(rightHandler)
	assert.Equal(t, prev, leftHandler)

	cases := []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌──┐┌──┐
│BB││BB│
│BB││BB│
└──┘└──┘`,
		},
	}

	require.True(t, wm.FocusRight())

	handlertest.TestHandler(t, wm, cases, writer)

	fb := compapi.FrameCharSet{}
	fb.TopLeft = '╔'
	fb.BottomRight = '╝'
	fb.BottomLeft = '╚'
	fb.TopRight = '╗'

	fb.VerticalLeft = '║'
	fb.VerticalRight = '║'
	fb.HorizontalTop = '═'
	fb.HorizontalBottom = '═'

	wm.SetFrameCharSet(compapi.FrameCharSetDefault(), fb)

	cases = []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌──┐╔══╗
│CC│║CC║
│CC│║CC║
└──┘╚══╝`,
		},
	}

	handlertest.TestHandler(t, wm, cases, writer)
}

func TestWindowManagerInit(t *testing.T) {
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = false
	wm := NewWindowManager(handler.NewTestHandler(), cfg)
	require.NotNil(t, wm.Focus())
}

func TestWindowManagerSetAttr(t *testing.T) {
	wm := NewWindowManager(handler.NewTestHandler(), DefaultWindowManagerConfig())
	cyan := tcell.ColorNavy
	red := tcell.ColorRed

	wm.SplitHorizontal(wm.Focus(), handler.NewTestHandler())
	wm.SetAttr(term.Attributes{Bg: cyan, Fg: red}, term.Attributes{Bg: red, Fg: cyan})

	w1 := wm.Focus()
	b, ok := w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, red, b.Bg)
	assert.Equal(t, cyan, b.Fg)

	w1.SetContent(handler.NewTestHandler())
	b, ok = w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, red, b.Bg)
	assert.Equal(t, cyan, b.Fg)

	wm.ShiftFocus()
	b, ok = w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, cyan, b.Bg)
	assert.Equal(t, red, b.Fg)

	w1.SetContent(handler.NewTestHandler())
	b, ok = w1.FrameAttr()
	require.True(t, ok)
	assert.Equal(t, cyan, b.Bg)
	assert.Equal(t, red, b.Fg)
}

func testWindowManagerClose(
	t *testing.T,
	frame bool,
	split func(*WindowManager, Window, tui.Handler) (Window, bool),
) {
	h1 := handler.NewTestHandler()
	h1.Ch = 'C'
	h2 := handler.NewTestHandler()
	h2.Ch = 'D'
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(h1, cfg)
	node2, ok := split(wm, wm.Focus(), h2)
	require.True(t, ok)

	assert.NotEqual(t, node2, wm.Focus())
	require.NoError(t, wm.Focus().Close())
	assert.Equal(t, node2, wm.Focus())
}

func testWindowManagerCloseLast(
	t *testing.T,
	frame bool,
	split func(*WindowManager, Window, tui.Handler) (Window, bool),
) {
	h1 := handler.NewTestHandler()
	h1.Ch = 'C'
	h2 := handler.NewTestHandler()
	h2.Ch = 'D'
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(h1, cfg)
	node2, ok := split(wm, wm.Focus(), h2)
	require.True(t, ok)

	node1 := wm.Focus()
	assert.NotEqual(t, node2, node1)
	require.NotNil(t, wm.SetFocus(node2))
	assert.Equal(t, node2, wm.Focus())
	require.NoError(t, node2.Close())
	assert.Equal(t, node1, wm.Focus())
}

func TestWindowManagerClose(t *testing.T) {
	suite := []struct {
		description string
		split       func(*WindowManager, Window, tui.Handler) (Window, bool)
		frame       bool
	}{
		{"split horizontal with frame", (*WindowManager).SplitHorizontal, true},
		{"split horizontal without frame", (*WindowManager).SplitHorizontal, false},
		{"split vertical with frame", (*WindowManager).SplitVertical, true},
		{"split vertical without frame", (*WindowManager).SplitVertical, false},
	}

	for _, test := range suite {
		t.Run("Close "+test.description, func(t *testing.T) {
			testWindowManagerClose(t, test.frame, test.split)
			testWindowManagerCloseLast(t, test.frame, test.split)
		})
	}
}

func TestWindowManagerCloseFloatingFocusesFrontmostRemainingFloating(t *testing.T) {
	cfg := DefaultWindowManagerConfig()
	wm := NewWindowManager(handler.NewTestHandler(), cfg)
	wm.Resize(20, 8)

	f1 := wm.FloatingWindow(
		handler.StaticFloating(handler.NewTestHandler(), 4, 4),
		component.FloatingConfig{Alignment: compapi.AlignmentCentered},
	)
	f2 := wm.FloatingWindow(
		handler.StaticFloating(handler.NewTestHandler(), 4, 4),
		component.FloatingConfig{Alignment: compapi.AlignmentCentered},
	)
	f3 := wm.FloatingWindow(
		handler.StaticFloating(handler.NewTestHandler(), 4, 4),
		component.FloatingConfig{Alignment: compapi.AlignmentCentered},
	)

	wm.SetFocus(f3)
	require.NoError(t, f3.Close())

	assert.Equal(t, f2.ID(), wm.Focus().ID())
	assert.NotEqual(t, f1.ID(), wm.Focus().ID())
}

func testWindowManagerContent(t *testing.T, frame bool) {
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(handler.NewTestHandler(), cfg)
	node2, ok := wm.SplitHorizontal(wm.Focus(), handler.NewTestHandler())
	require.True(t, ok)

	c := node2.Content()
	_, ok = c.(*handler.TestHandler)
	require.True(t, ok)

	prev := node2.SetContent(handler.NewTestHandler())
	_, ok = prev.(*handler.TestHandler)
	require.True(t, ok)

	c = node2.Content()
	_, ok = c.(*handler.TestHandler)
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

func testWindowManagerCursorShow(
	t *testing.T, frame bool,
	input, expected term.Coordinates,
) {
	handler := handler.NewTestHandler()
	handler.CursorPos = input
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(handler, cfg)
	wm.Resize(10, 10)

	pos, _, ok := wm.Cursor()
	require.True(t, ok)
	assert.Equal(t, expected, pos)
}

func testWindowManagerCursorHide(
	t *testing.T, frame bool,
	input term.Coordinates,
) {
	handler := handler.NewTestHandler()
	handler.CursorPos = input
	cfg := DefaultWindowManagerConfig()
	cfg.Frame = frame
	wm := NewWindowManager(handler, cfg)
	wm.Resize(10, 10)

	_, _, ok := wm.Cursor()
	require.False(t, ok)
}

func TestWindowManagerCursor(t *testing.T) {
	t.Run("Cursor with frame", func(t *testing.T) {
		input := term.Coordinates{X: 1, Y: 2}
		testWindowManagerCursorShow(t, true, input,
			term.Coordinates{X: 2, Y: 3})
	})

	t.Run("Cursor with frame at bounds - 1", func(t *testing.T) {
		testWindowManagerCursorShow(t, true, term.Coordinates{X: 7, Y: 7},
			term.Coordinates{X: 8, Y: 8})
	})

	t.Run("Cursor without frame", func(t *testing.T) {
		input := term.Coordinates{X: 1, Y: 2}
		testWindowManagerCursorShow(t, false, input, input)
	})

	t.Run("Cursor without frame at bounds - 1", func(t *testing.T) {
		testWindowManagerCursorShow(t, false, term.Coordinates{X: 9, Y: 9},
			term.Coordinates{X: 9, Y: 9})
	})

	t.Run("overrides show to off if out of bounds, with frame", func(t *testing.T) {
		testWindowManagerCursorHide(t, true, term.Coordinates{X: 8, Y: 8})
	})

	t.Run("overrides show to off if out of bounds, with frame", func(t *testing.T) {
		testWindowManagerCursorHide(t, false, term.Coordinates{X: 10, Y: 10})
	})

	t.Run("overrides show to off if negative out of bounds, with frame", func(t *testing.T) {
		testWindowManagerCursorHide(t, true, term.Coordinates{X: -1, Y: -1})
	})

	t.Run("overrides show to off if negative out of bounds, with frame", func(t *testing.T) {
		testWindowManagerCursorHide(t, false, term.Coordinates{X: -1, Y: -1})
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
			win.SetContent(handler.NewTestHandler())
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
	h1 := handler.NewTestHandler()
	h2 := handler.NewTestHandler()
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

func TestWindowManagerRestoreTileLayout(t *testing.T) {
	tests := []struct {
		name        string
		frame       bool
		layout      component.TileLayout
		content     map[uint64]rune
		wantMapped  []uint64
		assertExtra func(*testing.T, *WindowManager, map[uint64]Window, *testWindowSubscriber)
	}{
		{
			name:       "single leaf",
			layout:     component.TileLayout{WindowID: 1},
			content:    map[uint64]rune{1: 'A'},
			wantMapped: []uint64{1},
		},
		{
			name:       "single leaf framed",
			frame:      true,
			layout:     component.TileLayout{WindowID: 1},
			content:    map[uint64]rune{1: 'B'},
			wantMapped: []uint64{1},
		},
		{
			name: "deep mixed layout",
			layout: component.TileLayout{
				Split: component.SplitOrientationVertical,
				Children: []component.TileLayout{
					{WindowID: 1},
					{
						Split: component.SplitOrientationHorizontal,
						Children: []component.TileLayout{
							{WindowID: 2},
							{WindowID: 3},
							{
								Split:    component.SplitOrientationVertical,
								Children: []component.TileLayout{{WindowID: 4}, {WindowID: 5}},
							},
						},
					},
				},
			},
			content:    map[uint64]rune{1: 'A', 2: 'B', 3: 'C', 4: 'D', 5: 'E'},
			wantMapped: []uint64{1, 2, 3, 4, 5},
		},
		{
			name: "nil content fallback",
			layout: component.TileLayout{
				Split:    component.SplitOrientationHorizontal,
				Children: []component.TileLayout{{WindowID: 1}, {WindowID: 2}},
			},
			content:    map[uint64]rune{2: 'Z'},
			wantMapped: []uint64{1, 2},
			assertExtra: func(t *testing.T, wm *WindowManager, restored map[uint64]Window, sub *testWindowSubscriber) {
				assert.NotPanics(t, func() {
					writer := term.NewStringWriter(20, 8)
					wm.Draw(writer)
				})
				require.IsType(t, &handler.TestHandler{}, restored[2].Content())
			},
		},
		{
			name: "restore sets focus and notifies subscribers",
			layout: component.TileLayout{
				Split:    component.SplitOrientationVertical,
				Children: []component.TileLayout{{WindowID: 1}, {WindowID: 2}},
			},
			content:    map[uint64]rune{1: 'L', 2: 'R'},
			wantMapped: []uint64{1, 2},
			assertExtra: func(t *testing.T, wm *WindowManager, restored map[uint64]Window, sub *testWindowSubscriber) {
				require.Contains(t, restored, uint64(1))
				require.Equal(t, restored[uint64(1)].ID(), wm.Focus().ID())
				require.Equal(t, restored[uint64(1)].ID(), sub.lastFocus.ID())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultWindowManagerConfig()
			cfg.Frame = tt.frame
			wm := NewWindowManager(handler.NewTestHandler(), cfg)
			wm.Resize(20, 8)
			sub := new(testWindowSubscriber)
			wm.Subscribe(sub)
			sub.reset()

			restored := wm.RestoreTileLayout(tt.layout, func(windowID uint64) tui.Handler {
				ch, ok := tt.content[windowID]
				if !ok {
					return nil
				}
				return &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: ch}}
			})

			require.Len(t, restored, len(tt.wantMapped))
			for _, id := range tt.wantMapped {
				require.Contains(t, restored, id)
				require.NotEqual(t, id, restored[id].ID())
				if ch, ok := tt.content[id]; ok {
					assertHandlerWindowTile(t, restored[id], ch)
				}
			}
			assertHandlerWindowManagerLayoutShape(t, tt.layout, wm.TileLayout(), restored)
			if tt.assertExtra != nil {
				tt.assertExtra(t, wm, restored, sub)
			}
		})
	}
}

func assertHandlerWindowTile(t *testing.T, win Window, expected rune) {
	t.Helper()
	h, ok := win.Content().(*handler.TestHandler)
	require.True(t, ok)
	require.Equal(t, string(expected), string(h.Ch))
}

func assertHandlerWindowManagerLayoutShape(
	t *testing.T,
	want component.TileLayout,
	got component.TileLayout,
	restored map[uint64]Window,
) {
	t.Helper()
	require.Equal(t, want.Split, got.Split)
	require.Len(t, got.Children, len(want.Children))
	if len(want.Children) == 0 {
		if want.WindowID == 0 {
			require.Equal(t, uint64(0), got.WindowID)
			return
		}
		require.Equal(t, restored[want.WindowID].ID(), got.WindowID)
		return
	}
	for i := range want.Children {
		assertHandlerWindowManagerLayoutShape(t, want.Children[i], got.Children[i], restored)
	}
}

func TestWindowManagerSplit(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	h1 := handler.TestHandler{TestComponent: compapi.TestComponent{Ch: 'A'}}
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
	h2 := handler.TestHandler{TestComponent: compapi.TestComponent{Ch: 'B'}}
	h3 := handler.TestHandler{TestComponent: compapi.TestComponent{Ch: 'C'}}

	tests := []comptest.TestCase{
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
			hf := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: 'F'}}
			wfloat := wm.FloatingWindow(handler.StaticFloating(hf, 2, 2), component.FloatingConfig{})
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
			hf := &handler.TestHandler{TestComponent: compapi.TestComponent{Ch: 'F'}}
			wfloat := wm.FloatingWindow(handler.StaticFloating(hf, 2, 2), component.FloatingConfig{})
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
			hf := handler.NopFromComponent(component.Buffer(buf, compapi.StringResponsiveConfig{}))
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

	comptest.TestComponent(t, wm, w, tests)
}

type scrollableHandler struct {
	compapi.Scrollable
}

func (t scrollableHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, 0, false
}

func (t scrollableHandler) Selection() (string, bool) {
	return "", false
}

func (t scrollableHandler) Handle(ev term.Event) (bool, bool) {
	return false, false
}

func TestWindowManagerScrollBar(t *testing.T) {
	buf2 := cell.NewBuffer()
	buf2.WriteString("a\nb\nc\n")
	comp2 := component.NewScroll(buf2)
	rightHandler := scrollableHandler{Scrollable: comp2}

	buf1 := cell.NewBuffer()
	buf1.WriteString("A\nB\nC\n")
	comp1 := component.NewScroll(buf1)
	leftHandler := scrollableHandler{Scrollable: comp1}

	width, height := 12, 4
	writer, wm := prepareTest(width, height, true, leftHandler)

	_, ok := wm.SplitVertical(wm.Focus(), rightHandler)
	require.True(t, ok)

	cases := []handlertest.SingleTestCase{
		{
			term.Event{}, `
┌────┐┌────┐
│A   |│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 5, MouseY: 1}, `
┌────┐┌────┐
│A   X│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 6, MouseY: 1}, `
┌────┐┌────┐
│A   |│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 5, MouseY: 1}, `
┌────┐┌────┐
│A   X│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 11, MouseY: 1}, `
┌────┐┌────┐
│A   |│a   X
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 11, MouseY: 2}, `
┌────┐┌────┐
│A   |│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 11, MouseY: 1}, `
┌────┐┌────┐
│A   |│a   X
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 11, MouseY: 2}, `
┌────┐┌────┐
│A   |│b   X
│B   ││c   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 11, MouseY: 3}, `
┌────┐┌────┐
│A   |│c   │
│B   ││    X
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 1, MouseY: 0}, `
┌────┐┌────┐
│A   |│a   X
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 1, MouseY: 0}, `
┌────┐┌────┐
│A   |│a   X
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 1, MouseY: 0}, `
┌────┐┌────┐
│A   |│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 5, MouseY: 1}, `
┌────┐┌────┐
│A   X│a   |
│B   ││b   │
└────┘└────┘`,
		},
		{
			term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 25, MouseY: 25}, `
┌────┐┌────┐
│C   ││a   |
│    X│b   │
└────┘└────┘`,
		},
	}

	handlertest.TestHandler(t, wm, cases, writer)
}
