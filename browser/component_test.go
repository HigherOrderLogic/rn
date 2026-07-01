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

package browser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	thandler "unstable.build/go-tui/handler"
)

func TestNewTabFromContent(t *testing.T) {
	t.Run("returns false if content is already a tab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		uri, err := workspaceapi.ParseURI("file:///" + "d")
		require.NoError(t, err)
		h := newTestHandler()
		tab := b.NewTab(uri, 'o', "d", h, h)
		b.Focus().SetContent(tab)

		expectedTab, ok := b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", expectedTab.URI().String())

		actualTab, ok := b.NewTabFromContent('o', "tab", b.Focus())
		assert.False(t, ok)
		assert.Equal(t, tab, actualTab)
	})

	t.Run("converts scrollable into a tab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		h := newTestScrollableHandler()
		b.Focus().SetContent(h)

		tab, ok := b.NewTabFromContent('o', "tab", b.Focus())
		assert.True(t, ok)

		h.maxSeekOffset = 10
		assert.Equal(t, 10, tab.MaxSeekOffset())
		assert.True(t, tab.SeekDown())
		assert.True(t, tab.SeekUp())
		assert.NotZero(t, tab.URI())
		win, ok := tab.Window()
		assert.True(t, ok)
		assert.Equal(t, b.Focus(), win)
	})

	t.Run("converts non-scrollable into a tab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		h := newTestHandler()
		b.Focus().SetContent(h)

		tab, ok := b.NewTabFromContent('o', "tab", b.Focus())
		assert.True(t, ok)
		assert.False(t, tab.SeekDown())
		assert.NotZero(t, tab.URI())
		win, ok := tab.Window()
		assert.True(t, ok)
		assert.Equal(t, b.Focus(), win)

		require.NoError(t, tab.Close())
		assert.True(t, h.closed)
	})

	t.Run("converts floating window content into a tab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		h := newTestHandler()
		floating := b.Floating(h, browserapi.FloatingConfig{
			Alignment: component.AlignmentHorizontallyCentered,
		})

		tab, ok := b.NewTabFromContent('o', "tab", floating)
		assert.True(t, ok)
		assert.False(t, tab.SeekDown())
		assert.NotZero(t, tab.URI())
		win, ok := tab.Window()
		assert.True(t, ok)
		assert.Equal(t, floating, win)

		require.NoError(t, floating.Close())
		assert.False(t, h.closed)

		win, ok = tab.Window()
		assert.False(t, ok)

		b.Focus().SetContent(tab)
		win, ok = tab.Window()
		assert.True(t, ok)
		assert.Equal(t, b.Focus(), win)
	})

	t.Run("converts content with URI into a tab, maintains URI", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		uri1, err := workspaceapi.ParseURI("file:///a")
		require.NoError(t, err)

		h := newTestHandlerURI(uri1)
		b.Focus().SetContent(h)

		tab, ok := b.NewTabFromContent('o', "tab", b.Focus())
		assert.True(t, ok)
		assert.Equal(t, uri1, tab.URI())
	})

	t.Run("multiple windows, multiple tabs", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		win0 := b.Focus()

		tabs := []string{"A", "b", "C", "d"}
		for i, name := range tabs {
			uri, err := workspaceapi.ParseURI("file:///" + name)
			require.NoError(t, err)
			h := newTestHandler()
			tab := b.NewTab(uri, 'o', name, h, h)
			if i%2 == 0 {
				b.Split(browserapi.OrientationRight, b.Focus(), tab)
			}
		}

		// window 0 has nothing
		// window 1 has A
		// window 2 has C
		h := newTestHandler()
		require.True(t, b.FocusLeft())
		require.True(t, b.FocusLeft())
		b.Focus().SetContent(h)
		tab, ok := b.NewTabFromContent('o', "tab", b.Focus())
		assert.True(t, ok)

		actualWin, ok := tab.Window()
		assert.True(t, ok)
		assert.Equal(t, win0, actualWin)

		actualTabs := b.Tabs()
		require.Len(t, actualTabs, 5)

		assert.True(t, b.SetContentToTab(win0, 1))

		actualWin, ok = tab.Window()
		assert.False(t, ok)
		assert.Nil(t, actualWin)
	})

	t.Run("subscribes content to tab focus changes, if applicable", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		uri1, err := workspaceapi.ParseURI("file:///a")
		require.NoError(t, err)

		h := newTestHandlerURI(uri1)
		b.Focus().SetContent(h)

		assert.Equal(t, 0, h.onFocus)
		assert.Equal(t, 0, h.onFree)

		tab1, ok := b.NewTabFromContent('o', "tab", b.Focus())
		assert.True(t, ok)
		assert.Equal(t, uri1, tab1.URI())

		assert.Equal(t, 1, h.onFocus)
		assert.Equal(t, 0, h.onFree)

		uri, err := workspaceapi.ParseURI("file:///b")
		require.NoError(t, err)
		tab2 := b.NewTab(uri, 'o', "b", newTestHandler(), nil)

		b.Focus().SetContent(tab2)
		assert.Equal(t, 1, h.onFocus)
		assert.Equal(t, 1, h.onFree)

		b.Focus().SetContent(tab1)
		assert.Equal(t, 2, h.onFocus)
		assert.Equal(t, 1, h.onFree)
	})
}

func TestWindowDraw(t *testing.T) {
	noFrameNoDim := DefaultConfig()
	noFrameNoDim.Dim = false
	noFrameNoDim.Frame = false

	noFrameDim := DefaultConfig()
	noFrameDim.Dim = true
	noFrameDim.Frame = false
	suite := []Config{
		noFrameNoDim,
		noFrameDim,
	}

	for _, cfg := range suite {
		t.Run(fmt.Sprintf("%#v", cfg), func(t *testing.T) {
			// create a decent mix of components and UI elements
			b := NewComponent(cfg)
			b.Split(browserapi.OrientationRight, b.Focus(), newTestHandler())
			b.Split(browserapi.OrientationBottom, b.Focus(), newTestHandler())
			b.Split(browserapi.OrientationTop, b.Focus(), newTestHandler())
			b.Split(browserapi.OrientationLeft, b.Focus(), newTestHandler())
			uri1, err := workspaceapi.ParseURI("file:///a")
			require.NoError(t, err)
			h := newTestHandler()
			b.NewTab(uri1, 'a', "a", h, h)
			cfg := browserapi.BarConfig{Size: 1, Orientation: browserapi.OrientationTop}
			b.Bar(cfg, newTestHandler())
			cfg.Orientation = browserapi.OrientationBottom
			b.Bar(cfg, newTestHandler())
			cfg.Orientation = browserapi.OrientationLeft
			b.Bar(cfg, newTestHandler())
			cfg.Orientation = browserapi.OrientationRight
			b.Bar(cfg, newTestHandler())
			b.Floating(newTestHandler(), browserapi.FloatingConfig{
				Alignment: component.AlignmentHorizontallyCentered,
			})

			width, height := 12, 8
			writer1 := term.NewStringWriter(width, height)
			writer2 := term.NewStringWriter(width, height)

			b.Resize(width, height)
			b.Draw(writer1)

			// draw first union (everything), and then windows on top
			// and it matches Draw, then DrawWindow is correct.
			b.tabs.ResetFocus()
			for id, t := range b.buffers {
				b.tabs.SetIconAttr(id, term.Attributes{})
				if !t.free {
					b.tabs.SetFocus(id)
				}
			}
			if b.focusWindow != (thandler.Window{}) {
				win, ok := b.findWindow(b.focusWindow.ID())
				if ok {
					tab, ok := browserTabAtWindow(win)
					if ok {
						b.tabs.SetIconAttr(b.mustFindTabID(tab), term.Attributes{})
					}
				}
			}
			b.union.Draw(writer2)
			b.wm.Iterate(func(w thandler.Window) {
				win, _ := b.findWindow(w.ID())
				b.DrawWindow(win, writer2)
			})
			b.overwriteFocusWindowUnion(writer2)

			writer1.Flush()
			writer2.Flush()
			assert.Equal(t, writer1.String(), writer2.String())
		})
	}
}

func TestWindowFocusTabIconCueFollowsFocus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	windowFocusIconAttr := term.Attributes{Bg: term.ColorGreen, Attrs: term.AttrBold}
	cfg.FocusTabIconAttr = windowFocusIconAttr

	b := NewComponent(cfg)

	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 'A', "a", newTestHandler(), nil)
	b.Focus().SetContent(tabA)

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 'B', "b", newTestHandler(), nil)
	_, ok := b.Split(browserapi.OrientationRight, b.Focus(), tabB)
	require.True(t, ok)

	width, height := 20, 5
	b.Resize(width, height)
	writer := term.NewStringWriter(width, height)
	b.Draw(writer)
	cells := writer.Cells()

	expectedIconAttr := windowFocusIconAttr
	expectedIconAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	expectedFocusTabAttr := cfg.FocusTabAttr
	expectedFocusTabAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	expectedDefaultIconAttr := term.Attributes{Attrs: term.AttrNegativeVerticalRenderOffset}
	assert.Equal(t, 'A', cells[0].Ch)
	assert.Equal(t, expectedDefaultIconAttr, cells[0].Attributes)
	assert.Equal(t, 'a', cells[2].Ch)
	assert.Equal(t, expectedFocusTabAttr, cells[2].Attributes)
	assert.Equal(t, 'B', cells[5].Ch)
	assert.Equal(t, expectedIconAttr, cells[5].Attributes)
	assert.Equal(t, 'b', cells[7].Ch)
	assert.Equal(t, expectedFocusTabAttr, cells[7].Attributes)

	require.True(t, b.FocusLeft())
	writer = term.NewStringWriter(width, height)
	b.Draw(writer)
	cells = writer.Cells()

	assert.Equal(t, 'A', cells[0].Ch)
	assert.Equal(t, expectedIconAttr, cells[0].Attributes)
	assert.Equal(t, 'a', cells[2].Ch)
	assert.Equal(t, expectedFocusTabAttr, cells[2].Attributes)
	assert.Equal(t, 'B', cells[5].Ch)
	assert.Equal(t, expectedDefaultIconAttr, cells[5].Attributes)
	assert.Equal(t, 'b', cells[7].Ch)
	assert.Equal(t, expectedFocusTabAttr, cells[7].Attributes)
}

// TestNewTabHonorsTabOverrideIcon verifies that when
// Config.TabOverrideIcon is set, every tab icon rendered in the tab
// bar uses that single rune regardless of the icon argument passed to
// NewTab.
func TestNewTabHonorsTabOverrideIcon(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	cfg.TabOverrideIcon = '●'

	b := NewComponent(cfg)

	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 'A', "a", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tabA))

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 'B', "b", newTestHandler(), nil)
	_, ok := b.Split(browserapi.OrientationRight, b.Focus(), tabB)
	require.True(t, ok)

	width, height := 20, 5
	b.Resize(width, height)
	writer := term.NewStringWriter(width, height)
	b.Draw(writer)
	cells := writer.Cells()

	assert.Equal(t, '●', cells[0].Ch, "first tab icon must be overridden")
	assert.Equal(t, 'a', cells[2].Ch)
	assert.Equal(t, '●', cells[5].Ch, "second tab icon must be overridden")
	assert.Equal(t, 'b', cells[7].Ch)
}

// TestWindowFocusTabHighlightCueFollowsFocus verifies that when multiple
// tabs are bound to different tiles, only the focused window's tab
// renders the focus-frame highlight; switching window focus moves the
// highlight to the newly focused tab.
func TestWindowFocusTabHighlightCueFollowsFocus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	cfg.FocusTabHighlightChar = '━'
	cfg.TabBarHeight = 2

	b := NewComponent(cfg)

	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 'A', "a", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tabA))

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 'B', "b", newTestHandler(), nil)
	_, ok := b.Split(browserapi.OrientationRight, b.Focus(), tabB)
	require.True(t, ok)

	width, height := 20, 5
	b.Resize(width, height)
	writer := term.NewStringWriter(width, height)
	b.Draw(writer)
	cells := writer.Cells()

	// After Split, the new right tile is focused, so tab B carries
	// the highlight (cells 5..7) and tab A does not (cells 0..2).
	for x := 0; x < 3; x++ {
		assert.NotEqual(t, '━', cells[x].Ch,
			"tab A must not be highlighted at x=%d", x)
	}
	for x := 5; x < 8; x++ {
		assert.Equal(t, '━', cells[x].Ch,
			"tab B must be highlighted at x=%d", x)
	}

	require.True(t, b.FocusLeft())
	writer = term.NewStringWriter(width, height)
	b.Draw(writer)
	cells = writer.Cells()

	// After focusing left, the highlight moves to tab A.
	for x := 0; x < 3; x++ {
		assert.Equal(t, '━', cells[x].Ch,
			"tab A must be highlighted after focus left at x=%d", x)
	}
	for x := 5; x < 8; x++ {
		assert.NotEqual(t, '━', cells[x].Ch,
			"tab B must not be highlighted after focus left at x=%d", x)
	}
}

// TestTabClickFocusesOwningWindow verifies that clicking a tab bound to
// a non-focused window switches focus to that window (instead of
// returning ErrTabNotFree silently).
func TestTabClickFocusesOwningWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	cfg.FocusTabHighlightChar = '━'
	cfg.TabBarHeight = 2

	b := NewComponent(cfg)

	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 'A', "a", newTestHandler(), nil)
	leftWin := b.Focus()
	require.NoError(t, leftWin.SetContent(tabA))

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 'B', "b", newTestHandler(), nil)
	rightWin, ok := b.Split(browserapi.OrientationRight, leftWin, tabB)
	require.True(t, ok)
	require.Equal(t, rightWin, b.Focus())

	width, height := 20, 5
	b.Resize(width, height)
	writer := term.NewStringWriter(width, height)
	b.Draw(writer)

	// Click tab A (id 0) — it is bound to the left, non-focused window.
	require.True(t, b.tabs.OnClick(0))
	assert.Equal(t, leftWin, b.Focus(),
		"clicking a tab bound to another window must focus that window")

	writer = term.NewStringWriter(width, height)
	b.Draw(writer)
	cells := writer.Cells()
	for x := 0; x < 3; x++ {
		assert.Equal(t, '━', cells[x].Ch,
			"tab A must be highlighted after click at x=%d", x)
	}
	for x := 5; x < 8; x++ {
		assert.NotEqual(t, '━', cells[x].Ch,
			"tab B must not be highlighted after click at x=%d", x)
	}
}

// TestSplitAlreadyBoundTabFocusesOwningWindow reproduces RUNE-236's
// sibling crash: `runectl wm split <focus> <file>` with the file already
// open resolves to the existing, already-bound *Tab. Binding that one
// tab to a second window corrupts the tab/window bookkeeping and later
// panics in Draw when the tab is removed. Split must instead focus the
// window already showing the tab, like the tab-click path.
func TestSplitAlreadyBoundTabFocusesOwningWindow(t *testing.T) {
	b := NewComponent(DefaultConfig())

	uri, err := workspaceapi.ParseURI("file:///CHANGELOG.md")
	require.NoError(t, err)
	h := newTestHandler()
	tab := b.NewTab(uri, 'o', "CHANGELOG.md", h, h)
	leftWin := b.Focus()
	require.NoError(t, leftWin.SetContent(tab))

	got, ok := b.Split(browserapi.OrientationRight, leftWin, tab)
	require.True(t, ok)
	assert.Equal(t, leftWin, got,
		"splitting with an already-open tab must return its existing window")
	assert.Equal(t, leftWin, b.Focus(),
		"splitting with an already-open tab must focus its existing window")

	assert.Len(t, b.buffers, 1, "the tab must not be duplicated in buffers")
	refs := 0
	for _, w := range b.windows {
		if bt, ok := browserTabAtWindow(w); ok && bt == tab {
			refs++
		}
	}
	assert.Equal(t, 1, refs, "exactly one window may reference the tab")
}

// TestTabClickOnFocusedTabIsNoOp verifies that clicking the tab of the
// currently-focused window does not change focus or surface an error.
func TestTabClickOnFocusedTabIsNoOp(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false

	b := NewComponent(cfg)

	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 'A', "a", newTestHandler(), nil)
	leftWin := b.Focus()
	require.NoError(t, leftWin.SetContent(tabA))

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 'B', "b", newTestHandler(), nil)
	rightWin, ok := b.Split(browserapi.OrientationRight, leftWin, tabB)
	require.True(t, ok)
	require.Equal(t, rightWin, b.Focus())

	b.Resize(20, 5)

	// Click tab B (id 1) — already bound to the focused window.
	require.True(t, b.tabs.OnClick(1))
	assert.Equal(t, rightWin, b.Focus(),
		"clicking the tab of the focused window must not change focus")
}

// TestTabClickFreeTabLoadsIntoFocusedWindow guards the unchanged
// free-tab path: clicking a free tab swaps it into the focused window.
func TestTabClickFreeTabLoadsIntoFocusedWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false

	b := NewComponent(cfg)

	// Open two tabs in the same window so the first ends up free.
	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 'A', "a", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tabA))

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 'B', "b", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tabB))

	// tabA is now free; tabB occupies the focused window.
	_, bound := tabA.Window()
	require.False(t, bound, "tabA must be free before click")

	b.Resize(20, 5)

	require.True(t, b.tabs.OnClick(0))

	focused, ok := b.FocusTab()
	require.True(t, ok)
	assert.Equal(t, tabA, focused,
		"clicking a free tab must load it into the focused window")
}

// TestLayoutAliasSwitchingThenTabClick reproduces the user's crash: open
// a file (a tab in the focused window), repeatedly switch window layouts
// via aliases that run `windowcloseall` followed by one or more
// `windownew right`, then click the tab. `windownew right` focuses the
// new empty window, so `windowcloseall` closes the tab's original window.
// CloseOtherWindows closed the raw handler window without going through
// the browser's closeWindow path, so the tab's window binding was never
// released: it stayed "stuck" pointing at a removed window. Clicking it
// focused that detached tile and the next cursor pass panicked in
// TileTree.TilePosition.
func TestLayoutAliasSwitchingThenTabClick(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	cfg.TabBarHeight = 2

	b := NewComponent(cfg)
	width, height := 40, 10
	b.Resize(width, height)

	// Open a file: a tab in the initially focused window.
	uri, err := workspaceapi.ParseURI("file:///main.go")
	require.NoError(t, err)
	tab := b.NewTab(uri, 'o', "main.go", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tab))

	// windownew right: split focus to a new empty window and focus it.
	newRight := func() {
		_, ok := b.Split(browserapi.OrientationRight, b.Focus(), nil)
		require.True(t, ok)
	}
	// windowcloseall: keep the focused window, close the rest.
	closeAll := func() {
		if err := b.CloseOtherWindows(b.Focus()); err != nil &&
			err.Error() != "no windows to close" {
			t.Fatalf("CloseOtherWindows: %v", err)
		}
	}
	draw := func() {
		require.NotPanics(t, func() {
			_, _, _ = b.Cursor()
			b.Draw(term.NewStringWriter(width, height))
		})
	}

	auxScreen := func() { closeAll(); newRight(); newRight(); draw() }
	laptop := func() { closeAll(); newRight(); draw() }

	for range 4 {
		auxScreen()
		laptop()
	}

	// The first windowcloseall closed the tab's original window, so the
	// tab must be released, not left bound to a removed window.
	_, bound := tab.Window()
	assert.False(t, bound,
		"tab whose window was closed by windowcloseall must be free")

	// The tab is still in the bar; click it the way the user did. This
	// must not crash the next cursor/draw pass.
	id, ok := b.findTabID(tab)
	require.True(t, ok, "tab must still be present in the bar")
	b.tabs.OnClick(id)
	draw()
}

// TestNonFocusTabAttrRespectedWithFrameFg is a regression test for
// non_focus_tab_attr being overridden by the window manager's frame_attr
// foreground. The tabs Scroll background used to share frame_attr (gray
// in the production rune.star), and any tab name whose configured fg
// was ColorDefault picked up that gray fg instead of the configured
// non_focus_tab_attr default. Verify that a free (non-focused) tab
// renders with NonFocusTabAttr (Fg=ColorDefault) even when frame_attr
// has a non-default Fg.
func TestNonFocusTabAttrRespectedWithFrameFg(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	// simulate user-configured frame_attr with a non-default foreground
	// (the production rune.star sets this to gray).
	cfg.WindowManagerConfig.FrameAttr = term.Attributes{Fg: term.ColorGray}
	cfg.FocusTabAttr = term.Attributes{Fg: term.ColorBlue}
	cfg.NonFocusTabAttr = term.Attributes{} // i.e. fg=default,bg=default

	b := NewComponent(cfg)

	// open two tabs in the same window so the first ends up free.
	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	tabA := b.NewTab(uriA, 0, "a", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tabA))

	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)
	tabB := b.NewTab(uriB, 0, "b", newTestHandler(), nil)
	require.NoError(t, b.Focus().SetContent(tabB))

	width, height := 20, 5
	b.Resize(width, height)
	writer := term.NewStringWriter(width, height)
	b.Draw(writer)
	cells := writer.Cells()

	expectedFocus := cfg.FocusTabAttr
	expectedFocus.Attrs |= term.AttrNegativeVerticalRenderOffset
	expectedNonFocus := cfg.NonFocusTabAttr
	expectedNonFocus.Attrs |= term.AttrNegativeVerticalRenderOffset

	// "a" is free (not bound to any window) and must render with
	// NonFocusTabAttr; "b" is bound to the focused window and renders
	// with FocusTabAttr.
	assert.Equal(t, 'a', cells[0].Ch)
	assert.Equal(t, expectedNonFocus, cells[0].Attributes,
		"non-focused tab name must render with NonFocusTabAttr, not frame fg")
	assert.Equal(t, 'b', cells[3].Ch)
	assert.Equal(t, expectedFocus, cells[3].Attributes,
		"focused tab name must render with FocusTabAttr")
}

// TestFocusedLastTabVisibleWithTabBarOffset is a regression test for a bug
// where the focused tab (the rightmost one) was being clipped off-screen
// because the underlying component.Tabs was being resized to the full
// component width while a TabBarOffset visually shifted the bar to the
// right, leaving the trailing portion of the bar outside the visible
// viewport. The fix is to subtract TabBarOffset from the width passed to
// the Tabs component so the resize algorithm operates on the actual
// viewport width and the focused tab is rendered in the visible area.
func TestFocusedLastTabVisibleWithTabBarOffset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	cfg.FrameUnion = false
	cfg.Dim = false
	cfg.TabBarOffset = 8

	b := NewComponent(cfg)

	names := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}
	tabs := make([]*Tab, len(names))
	for i, n := range names {
		uri, err := workspaceapi.ParseURI("file:///" + n)
		require.NoError(t, err)
		tabs[i] = b.NewTab(uri, 0, n, newTestHandler(), nil)
	}
	// Focus the last tab.
	require.NoError(t, b.Focus().SetContent(tabs[len(tabs)-1]))

	// Width is intentionally smaller than the sum of every tab at full
	// width, so the resize algorithm has to shrink non-focused tabs to
	// keep the focused (last) tab fully visible inside the available
	// inner width: width 40 minus offset 8 = 32 cells of bar.
	const width = 40
	const height = 5
	b.Resize(width, height)
	writer := term.NewStringWriter(width, height)
	b.Draw(writer)
	require.NoError(t, writer.Flush())
	rows := strings.Split(writer.String(), "\n")
	require.GreaterOrEqual(t, len(rows), 1)

	// The focused tab label must be fully visible somewhere on the top
	// bar — not clipped at the right edge.
	bar := rows[0]
	require.Contains(t, bar, "zeta",
		"focused (last) tab label must be visible inside the viewport: %q", bar)

	// Sanity: at least one preceding tab must have been truncated for
	// "zeta" to fit (otherwise the algorithm wasn't exercised).
	truncated := 0
	for _, n := range names[:len(names)-1] {
		if !strings.Contains(bar, n) {
			truncated++
		}
	}
	assert.Greater(t, truncated, 0,
		"expected at least one non-focused tab to be shrunk: %q", bar)
}

func TestComponentRestoreTileLayout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Frame = false
	b := NewComponent(cfg)
	root := b.Focus()
	right, ok := b.Split(browserapi.OrientationRight, root, newTestHandler())
	require.True(t, ok)
	bottom, ok := b.Split(browserapi.OrientationBottom, right, newTestHandler())
	require.True(t, ok)

	uri, err := workspaceapi.ParseURI("file:///restored")
	require.NoError(t, err)
	tabHandler := newTestHandlerURI(uri)
	tab := b.NewTab(uri, 'r', "restored", tabHandler, tabHandler)
	tab.Subscribe(tabHandler)
	layout := b.TileLayout()
	restored := b.RestoreTileLayout(layout, func(windowID uint64) browserapi.Handler {
		switch windowID {
		case root.WindowID():
			return newTestHandler()
		case right.WindowID():
			return tab
		case bottom.WindowID():
			return nil
		default:
			return nil
		}
	})

	require.Len(t, restored, 3)
	for _, oldID := range []uint64{root.WindowID(), right.WindowID(), bottom.WindowID()} {
		require.Contains(t, restored, oldID)
		require.NotEqual(t, oldID, restored[oldID].WindowID())
	}
	content, err := restored[right.WindowID()].Content()
	require.NoError(t, err)
	require.Equal(t, tab, content)
	tabWin, ok := tab.Window()
	require.True(t, ok)
	require.Equal(t, restored[right.WindowID()], tabWin)
	require.Equal(t, 1, tabHandler.onFocus)

	content, err = restored[bottom.WindowID()].Content()
	require.NoError(t, err)
	require.NotNil(t, content)
}

func TestWindowClosedOnClose(t *testing.T) {
	b := NewComponent(DefaultConfig())
	h := newTestHandler()
	var win Window
	win = b.Floating(FuncFloatingHandler(h, func() error {
		if !win.Closed() {
			_ = win.Close()
		}
		return h.Close()
	}), browserapi.FloatingConfig{})
	assert.NoError(t, win.Close())
}

func TestBrowserScrollable(t *testing.T) {
	t.Run("create floating window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		scrollable := newTestScrollableHandler()

		win := b.Floating(scrollable, browserapi.FloatingConfig{})
		content, err := win.Content()
		require.NoError(t, err)

		_, ok := content.(component.Scrollable)
		assert.True(t, ok)
		assert.NoError(t, win.Close())
	})

	t.Run("create split window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		scrollable := newTestScrollableHandler()

		win, ok := b.Split(browserapi.OrientationLeft, b.Focus(), scrollable)
		require.True(t, ok)

		content, err := win.Content()
		require.NoError(t, err)

		_, ok = content.(*nopScrollableHandler)
		assert.True(t, ok)
		assert.NoError(t, win.Close())
	})

	t.Run("update content of window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		win, ok := b.Split(browserapi.OrientationLeft, b.Focus(), newTestHandler())
		require.True(t, ok)

		scrollable := newTestScrollableHandler()
		err := win.SetContent(scrollable)
		require.NoError(t, err)

		content, err := win.Content()
		require.NoError(t, err)

		_, ok = content.(component.Scrollable)
		assert.True(t, ok)
		assert.NoError(t, win.Close())
	})

	t.Run("new tab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		uri1, err := workspaceapi.ParseURI("file:///a")
		require.NoError(t, err)
		h := newTestScrollableHandler()
		tab := b.NewTab(uri1, 'x', "a", h, h)

		content := tab.Handler()
		_, ok := content.(component.Scrollable)
		assert.True(t, ok)
	})
}

func TestBrowserFloating(t *testing.T) {
	t.Run("create floating window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		h := newTestHandler()

		win := b.Floating(h, browserapi.FloatingConfig{})
		content, err := win.Content()
		require.NoError(t, err)

		_, ok := content.(*nopHandler)
		assert.True(t, ok)
		assert.NoError(t, win.Close())
	})

	t.Run("update content of floating window with non floating uses static dimensions",
		func(t *testing.T) {
			b := NewComponent(DefaultConfig())
			var h Floating
			h = newTestHandler()
			win := b.Floating(h, browserapi.FloatingConfig{})

			notFloating := browserapi.NopHandler(handler.NewTestHandler())
			err := win.SetContent(notFloating)
			require.NoError(t, err)
		})

	t.Run("floating and scrollable maintains interfaces",
		func(t *testing.T) {
			b := NewComponent(DefaultConfig())
			var h Floating
			h = newTestScrollableHandler()
			win := b.Floating(h, browserapi.FloatingConfig{})

			content, err := win.Content()
			require.NoError(t, err)
			_, fok := content.(Floating)
			assert.True(t, fok)
			_, sok := content.(Scrollable)
			assert.True(t, sok)

			// also internally
			internal := win.(*browserWindow).win.Content()

			_, fok = internal.(Floating)
			assert.True(t, fok)
			_, sok = internal.(Scrollable)
			assert.True(t, sok)
		})
}

// TestComponentCloseFloatingReentrantWindowClose reproduces a shutdown crash
// where a floating handler's Close callback closes its own captured window
// (as the cheatsheet does). Component.Close must not create a duplicate
// browserWindow that defeats the double-close guard and panics with
// "window not found".
func TestComponentCloseFloatingReentrantWindowClose(t *testing.T) {
	b := NewComponent(DefaultConfig())

	var win Window
	floating := FuncFloatingHandler(newTestHandler(), func() error {
		return win.Close()
	})
	win = b.Floating(floating, browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	})

	assert.NotPanics(t, func() {
		_ = b.Close()
	})
}

func TestComponentCloseOtherWindows(t *testing.T) {
	t.Run("fails if there's only one window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		require.Error(t, b.CloseOtherWindows(b.Focus()))
		assert.Equal(t, 1, b.Tiles())
		assert.Equal(t, 0, b.FloatingWindows())
	})
	t.Run("fails if there's one tiled window and one floating window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		_ = b.Floating(newTestHandler(), browserapi.FloatingConfig{
			Alignment: component.AlignmentHorizontallyCentered,
		})
		require.Error(t, b.CloseOtherWindows(b.Focus()))
		assert.Equal(t, 1, b.Tiles())
		assert.Equal(t, 1, b.FloatingWindows())
	})
	t.Run("fails if called on floating window", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		win := b.Floating(newTestHandler(), browserapi.FloatingConfig{
			Alignment: component.AlignmentHorizontallyCentered,
		})
		require.Error(t, b.CloseOtherWindows(win))
		assert.Equal(t, 1, b.Tiles())
		assert.Equal(t, 1, b.FloatingWindows())
	})
	t.Run("closes all floating and non-floating windows except focus", func(t *testing.T) {
		b := NewComponent(DefaultConfig())
		orig := b.Focus()
		_ = b.Floating(newTestHandler(), browserapi.FloatingConfig{
			Alignment: component.AlignmentHorizontallyCentered,
		})
		win, ok := b.Split(browserapi.OrientationDefault, orig, newTestHandler())
		require.True(t, ok)
		require.NoError(t, b.CloseOtherWindows(win))
		assert.Equal(t, 1, b.Tiles())
		assert.Equal(t, 0, b.FloatingWindows())

		require.Error(t, b.CloseOtherWindows(win))
		assert.Equal(t, 1, b.Tiles())
		assert.Equal(t, 0, b.FloatingWindows())
	})
	t.Run("does not panic closing floating whose Close callback closes its own window",
		func(t *testing.T) {
			b := NewComponent(DefaultConfig())
			focus := b.Focus()

			var floatWin Window
			floating := FuncFloatingHandler(newTestHandler(), func() error {
				return floatWin.Close()
			})
			floatWin = b.Floating(floating, browserapi.FloatingConfig{
				Alignment: component.AlignmentCentered,
			})

			assert.NotPanics(t, func() {
				_ = b.CloseOtherWindows(focus)
			})
			assert.Equal(t, 0, b.FloatingWindows())
		})
}

func TestRemoveInactiveTabs(t *testing.T) {
	b := NewComponent(DefaultConfig())

	tabDefs := []struct {
		name   string
		active bool
	}{
		{"a", false}, {"b", true}, {"c", true}, {"d", false}}

	for _, tabDef := range tabDefs {
		uri, err := workspaceapi.ParseURI("file:///" + tabDef.name)
		require.NoError(t, err)
		h := newTestHandler()
		tab := b.NewTab(uri, 'o', tabDef.name, h, h)
		if tabDef.active {
			b.Split(browserapi.OrientationRight, b.Focus(), tab)
		}
	}

	assertTabNames(t, b, []string{"a", "b", "c", "d"})
	b.RemoveInactiveTabs()
	assertTabNames(t, b, []string{"b", "c"})
}

func TestRemoveTab(t *testing.T) {
	b := NewComponent(DefaultConfig())

	tabDefs := []struct {
		name  string
		split bool
	}{
		{"a", false}, {"b", true}, {"c", true}, {"d", false}}

	tabs := make([]*Tab, 0)
	for _, tabDef := range tabDefs {
		uri, err := workspaceapi.ParseURI("file:///" + tabDef.name)
		require.NoError(t, err)
		h := newTestHandler()
		tab := b.NewTab(uri, 'o', tabDef.name, h, h)
		if tabDef.split {
			b.Split(browserapi.OrientationRight, b.Focus(), tab)
		}
		tabs = append(tabs, tab)
	}

	assertTabNames(t, b, []string{"a", "b", "c", "d"})
	assert.True(t, b.RemoveTab(tabs[0]))
	assert.True(t, b.RemoveTab(tabs[1]))
	assert.True(t, b.RemoveTab(tabs[3]))
	assertTabNames(t, b, []string{"c"})
}

// TestRemoveTabStale asserts that removing a tab whose handle is already
// gone from the component returns false without panicking. A non-modal
// prompt can capture a *Tab, the user closes that tab, and the prompt's
// discard later calls RemoveTab on the now-stale handle.
func TestRemoveTabStale(t *testing.T) {
	b := NewComponent(DefaultConfig())

	uri, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	h := newTestHandler()
	tab := b.NewTab(uri, 'o', "a", h, h)

	assert.True(t, b.RemoveTab(tab))
	assert.NotPanics(t, func() {
		assert.False(t, b.RemoveTab(tab))
	})
}

// TestRemoveTabDoesNotRetainPointersInTail asserts that after RemoveTab the
// dropped *Tab pointers are not retained past len(c.buffers) in the slice's
// backing array. A naive append(s[:i], s[i+1:]...) leaves the previous tail
// duplicate behind, which transitively pins the file's *cell.Buffer and
// produces multi-GB workspace-close leaks on large files.
func TestRemoveTabDoesNotRetainPointersInTail(t *testing.T) {
	b := NewComponent(DefaultConfig())

	names := []string{"a", "b", "c", "d"}
	tabs := make([]*Tab, 0, len(names))
	for _, name := range names {
		uri, err := workspaceapi.ParseURI("file:///" + name)
		require.NoError(t, err)
		h := newTestHandler()
		tabs = append(tabs, b.NewTab(uri, 'o', name, h, h))
	}

	// Remove every tab in order. After each removal the slot at index
	// len(c.buffers) inside the backing array must be nil; otherwise the
	// removed tab (and its transitive cell.Buffer) stays GC-reachable.
	for range names {
		require.True(t, b.RemoveTab(tabs[0]))
		tabs = tabs[1:]
		tail := b.buffers[len(b.buffers) : len(b.buffers)+1]
		assert.Nilf(t, tail[0],
			"buffers[%d] not cleared after RemoveTab; tail still pins *Tab",
			len(b.buffers))
	}
}

func TestTabAttrs(t *testing.T) {
	b := NewComponent(DefaultConfig())

	tabDefs := []struct {
		name string
	}{
		{"a"}, {"b"},
	}

	expectedAttr := term.Attributes{Fg: term.ColorYellow, Bg: term.ColorGreen}
	uris := make([]workspaceapi.URI, 0)
	for i, tabDef := range tabDefs {
		uri, err := workspaceapi.ParseURI("file:///" + tabDef.name)
		require.NoError(t, err)
		h := newTestHandler()
		_ = b.NewTab(uri, 'o', tabDef.name, h, h)
		if i%2 == 0 {
			b.SetTabNameAndAttrs(uri, "NAME", expectedAttr)
		}
		uris = append(uris, uri)
	}

	actualAttr, ok := b.TabAttrs(uris[0])
	require.True(t, ok)
	assert.Equal(t, expectedAttr, actualAttr)

	actualAttr, ok = b.TabAttrs(uris[1])
	require.True(t, ok)
	assert.Equal(t, term.Attributes{}, actualAttr)
}

// TestSetTabIcon verifies SetTabIcon overrides the tab icon for a
// known URI and returns false for an unknown URI, and that
// ResetTabIcon restores the icon the tab was created with.
func TestSetTabIcon(t *testing.T) {
	b := NewComponent(DefaultConfig())

	uriA, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	uriB, err := workspaceapi.ParseURI("file:///b")
	require.NoError(t, err)

	hA := newTestHandler()
	hB := newTestHandler()
	_ = b.NewTab(uriA, 'A', "a", hA, hA)
	_ = b.NewTab(uriB, 'B', "b", hB, hB)

	// override the icon for tab A
	require.True(t, b.SetTabIcon(uriA, '★'))
	assert.Equal(t, '★', b.tabs.TabIcon(0))
	// tab B is unaffected
	assert.Equal(t, 'B', b.tabs.TabIcon(1))

	// unknown URI returns false and is a no-op
	missing, err := workspaceapi.ParseURI("file:///missing")
	require.NoError(t, err)
	assert.False(t, b.SetTabIcon(missing, '?'))
	assert.Equal(t, '★', b.tabs.TabIcon(0))
	assert.Equal(t, 'B', b.tabs.TabIcon(1))

	// ResetTabIcon restores the original icon
	require.True(t, b.ResetTabIcon(uriA))
	assert.Equal(t, 'A', b.tabs.TabIcon(0))
	assert.False(t, b.ResetTabIcon(missing))
}

// TestSetTabIconHonorsTabOverrideIcon verifies that when
// Config.TabOverrideIcon is set, ResetTabIcon restores to the
// overridden glyph (not the icon argument originally passed to
// NewTab).
func TestSetTabIconHonorsTabOverrideIcon(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TabOverrideIcon = '●'
	b := NewComponent(cfg)

	uri, err := workspaceapi.ParseURI("file:///a")
	require.NoError(t, err)
	h := newTestHandler()
	_ = b.NewTab(uri, 'A', "a", h, h)
	assert.Equal(t, '●', b.tabs.TabIcon(0))

	require.True(t, b.SetTabIcon(uri, '★'))
	assert.Equal(t, '★', b.tabs.TabIcon(0))

	require.True(t, b.ResetTabIcon(uri))
	assert.Equal(t, '●', b.tabs.TabIcon(0))
}

func TestMoveTabs(t *testing.T) {
	b := NewComponent(DefaultConfig())

	tabs := []string{"A", "b", "C", "d"}
	for i, name := range tabs {
		uri, err := workspaceapi.ParseURI("file:///" + name)
		require.NoError(t, err)
		h := newTestHandler()
		tab := b.NewTab(uri, 'o', name, h, h)
		if i%2 == 0 {
			b.Split(browserapi.OrientationRight, b.Focus(), tab)
		}
	}

	tab, ok := b.FocusTab()
	require.True(t, ok)
	assert.Equal(t, "file:///C", tab.URI().String())

	assert.NoError(t, b.MoveTabLeft(b.Focus()))
	assertTabNames(t, b, []string{"A", "C", "b", "d"})
	assert.NoError(t, b.MoveTabLeft(b.Focus()))
	assertTabNames(t, b, []string{"C", "A", "b", "d"})
	assert.Error(t, b.MoveTabLeft(b.Focus()))
	assertTabNames(t, b, []string{"C", "A", "b", "d"})
	assert.NoError(t, b.MoveTabRight(b.Focus()))
	assertTabNames(t, b, []string{"A", "C", "b", "d"})
	assert.NoError(t, b.MoveTabRight(b.Focus()))
	assertTabNames(t, b, []string{"A", "b", "C", "d"})
	assert.NoError(t, b.MoveTabRight(b.Focus()))
	assertTabNames(t, b, []string{"A", "b", "d", "C"})
	assert.Error(t, b.MoveTabRight(b.Focus()))
	assertTabNames(t, b, []string{"A", "b", "d", "C"})
	assert.NoError(t, b.MoveTabTo(b.Focus(), 1))
	assertTabNames(t, b, []string{"A", "C", "b", "d"})
	assert.NoError(t, b.MoveTabTo(b.Focus(), 0))
	assertTabNames(t, b, []string{"C", "A", "b", "d"})
	assert.NoError(t, b.MoveTabTo(b.Focus(), 10))
	assertTabNames(t, b, []string{"A", "b", "d", "C"})
}

func TestFocusLastFocusOnCloseTab(t *testing.T) {
	t.Run("RemoveWindowContent", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		tabs := []string{"A", "b", "C", "d"}
		for i, name := range tabs {
			uri, err := workspaceapi.ParseURI("file:///" + name)
			require.NoError(t, err)
			h := newTestHandler()
			tab := b.NewTab(uri, 'o', name, h, h)
			if i == len(tabs)-1 {
				b.Focus().SetContent(tab)
			}
		}

		tab, ok := b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", tab.URI().String())

		assert.True(t, b.PreviousTab(b.Focus()))
		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///C", tab.URI().String())

		assert.True(t, b.PreviousTab(b.Focus()))
		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///b", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///C", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///A", tab.URI().String())

		require.False(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.False(t, ok)
	})

	t.Run("RemoveTab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		tabs := []string{"A", "b", "C", "d"}
		for i, name := range tabs {
			uri, err := workspaceapi.ParseURI("file:///" + name)
			require.NoError(t, err)
			h := newTestHandler()
			tab := b.NewTab(uri, 'o', name, h, h)
			if i == len(tabs)-1 {
				b.Focus().SetContent(tab)
			}
		}

		tab, ok := b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", tab.URI().String())

		assert.True(t, b.PreviousTab(b.Focus()))
		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///C", tab.URI().String())

		assert.True(t, b.PreviousTab(b.Focus()))

		assert.True(t, b.RemoveTab(tab))

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///A", tab.URI().String())

		assert.True(t, b.RemoveTab(tab))
	})

	t.Run("SetContentToTab", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		tabs := []string{"A", "b", "C", "d"}
		for i, name := range tabs {
			uri, err := workspaceapi.ParseURI("file:///" + name)
			require.NoError(t, err)
			h := newTestHandler()
			tab := b.NewTab(uri, 'o', name, h, h)
			if i == len(tabs)-1 {
				b.Focus().SetContent(tab)
			}
		}

		assert.False(t, b.SetContentToTab(b.Focus(), 3)) // already at d
		tab, ok := b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", tab.URI().String())

		assert.True(t, b.SetContentToTab(b.Focus(), 2))
		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///C", tab.URI().String())

		assert.True(t, b.SetContentToTab(b.Focus(), 1))
		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///b", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///C", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///A", tab.URI().String())

		require.False(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.False(t, ok)
	})

	t.Run("multiple windows", func(t *testing.T) {
		b := NewComponent(DefaultConfig())

		tabs := []string{"A", "b", "C", "d"}
		for i, name := range tabs {
			uri, err := workspaceapi.ParseURI("file:///" + name)
			require.NoError(t, err)
			h := newTestHandler()
			tab := b.NewTab(uri, 'o', name, h, h)
			if i%2 == 0 {
				b.Split(browserapi.OrientationRight, b.Focus(), tab)
			}
		}

		// window 0 has nothing
		// window 1 has A
		// window 2 has C

		assert.True(t, b.SetContentToTab(b.Focus(), 3))
		tab, ok := b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///d", tab.URI().String())

		// move to window 0, set C as content, so d.prev is invalid
		require.True(t, b.FocusLeft())
		require.True(t, b.FocusLeft())

		assert.True(t, b.SetContentToTab(b.Focus(), 2))
		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///C", tab.URI().String())

		require.True(t, b.RemoveWindowContent(b.Focus()))

		tab, ok = b.FocusTab()
		require.True(t, ok)
		assert.Equal(t, "file:///b", tab.URI().String())
	})
}

var _ component.Scrollable = (*nopScrollableHandler)(nil)

type nopScrollableHandler struct {
	maxSeekOffset int
	nopHandler
}

func (b *nopScrollableHandler) SeekUp() bool {
	return true
}

func (b *nopScrollableHandler) SeekDown() bool {
	return true
}

func (b *nopScrollableHandler) SeekOffset() int {
	return 0
}

func (b *nopScrollableHandler) MaxSeekOffset() int {
	return b.maxSeekOffset
}

func newTestScrollableHandler() *nopScrollableHandler {
	return &nopScrollableHandler{}
}

type nopHandler struct {
	closed bool
	handler.TestHandler
}

func (n *nopHandler) Dimensions() (int, int) {
	return 12, 8
}

func (n *nopHandler) Close() error {
	n.closed = true
	return nil
}

func newTestHandler() *nopHandler {
	return &nopHandler{}
}

type nopHandlerURI struct {
	onFocus int
	onFree  int
	uri     workspaceapi.URI
	nopHandler
}

func (n *nopHandlerURI) URI() workspaceapi.URI {
	return n.uri
}

func (n *nopHandlerURI) OnFocus(*Tab) {
	n.onFocus++
}

func (n *nopHandlerURI) OnFree(*Tab) {
	n.onFree++
}

func newTestHandlerURI(uri workspaceapi.URI) *nopHandlerURI {
	return &nopHandlerURI{uri: uri}
}

func assertTabNames(t *testing.T, b *Component, expected []string) {
	var actual []string
	for _, tab := range b.Tabs() {
		tabName, _, _ := b.TabName(tab.URI())
		actual = append(actual, tabName)
	}
	assert.Equal(t, expected, actual)
}
