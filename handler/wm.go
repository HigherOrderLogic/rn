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
	"fmt"

	compapi "github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component"
	tterm "unstable.build/go-tui/term"
)

// WindowManagerConfig represents a WindowManager's
// configuration properties.
type WindowManagerConfig struct {
	component.WindowManagerConfig

	Dim                bool
	FocusFrameAttr     term.Attributes
	FocusFrameCharSet  compapi.FrameCharSet
	ScrollBarHoverChar rune
}

// WindowManager implements Handler as a tiled window manager.
type WindowManager struct {
	comp   component.WindowManager
	config WindowManagerConfig
	// best effort to set focus to prev win upon ShiftFocus
	prevFocus                Window
	focus                    Window
	subs                     []WindowSubscriber
	prevMouseScrollBarDrag   bool
	prevMouseLeftChild       component.Window
	prevMouseScrollBarOffset int
}

// WindowSubscriber wraps the OnFocus callback used
// to subscribe to window focus. Upon calling Subscribe
// the first OnFocus is dispatched, but prevFocus will
// a zero Window, and so it should not be used.
type WindowSubscriber interface {
	OnFocus(prevFocus, newFocus Window)
}

// NewWindowManager allocates storage for a new WindowManager and initializes it with the
// given handler. If border is true, it will draw a border around every window.
func NewWindowManager(
	handler tui.Handler, cfg WindowManagerConfig,
) (wm *WindowManager) {
	wm = new(WindowManager)
	wm.Init(handler, cfg)
	return
}

func (wm *WindowManager) newNode(t component.Window) Window {
	return Window{wm: wm, Window: t}
}

// Init initializes this WindowManager with the given handler. If border is true, it will draw
// a border around every tile.
func (wm *WindowManager) Init(handler tui.Handler, cfg WindowManagerConfig) {
	win := wm.comp.Init(handler, cfg.WindowManagerConfig)
	wm.config = cfg
	wm.focus = wm.newNode(win)
	wm.SetFocus(wm.focus)
}

// SetFrameCharSet sets the frame border cells used to draw borders around tiles.
// Note that this has no effect if WindowManager was
// initialized with border == false.
func (wm *WindowManager) SetFrameCharSet(def, focus compapi.FrameCharSet) {
	if !wm.config.Frame {
		return
	}
	wm.config.FocusFrameCharSet = focus
	wm.config.FrameCharSet = def
	wm.comp.SetFrameCharSet(def)
	wm.setFocusAttr(wm.focus)
}

func (wm *WindowManager) setFocusAttr(win Window) {
	// only set focus attr+charset if there's more than one window
	if wm.SizeTiles()+wm.SizeFloating() > 1 {
		win.SetFrameAttr(wm.config.FocusFrameAttr)
		win.SetFrameCharSet(wm.config.FocusFrameCharSet)
	} else {
		win.SetFrameAttr(wm.config.FrameAttr)
		win.SetFrameCharSet(wm.config.FrameCharSet)
	}
}

// SetAttr sets the default and focus window border attributes. Note that
// this has no effect if WindowManager was initialized with border == false.
func (wm *WindowManager) SetAttr(def, focus term.Attributes) {
	if !wm.config.Frame {
		return
	}
	wm.config.FocusFrameAttr = focus
	wm.config.FrameAttr = def
	wm.comp.SetFrameAttr(def)
	wm.setFocusAttr(wm.focus)
}

// Handle satisfies tui.Handler.
func (wm *WindowManager) Handle(ev term.Event) (exit bool, handled bool) {
	if ev.Type == term.EventMouse {
		mousePos := term.Coordinates{X: ev.MouseX, Y: ev.MouseY}
		childAtMouse, ok := wm.comp.WindowAt(mousePos)
		if wm.prevMouseScrollBarDrag {
			childAtMouse = wm.prevMouseLeftChild
			ok = true
		}
		if !ok {
			return
		}
		offset := childAtMouse.Position()
		ev.MouseX -= offset.X
		ev.MouseY -= offset.Y

		// reset scroll bars
		wm.comp.Iterate(func(win component.Window) {
			f, ok := win.Frame()
			if ok {
				f.ScrollBarChar = wm.config.ScrollBarChar
			}
		})

		// set scroll bar hover char, if applicable
		if frame, ok := childAtMouse.Frame(); ok {
			position, height, ok := frame.ScrollBar()
			if ok {
				end := position.Y + height
				if ev.MouseX == position.X && ev.MouseY >= position.Y && ev.MouseY < end ||
					wm.prevMouseScrollBarDrag {
					return wm.handleScrollBarMouse(childAtMouse, position.Y, height, frame, ev)
				}
				// lost drag; reset if mouse is now outside of scroll bar
				wm.resetScrollBarMouse()
			} else {
				// lost scroll bar; content could have changed
				wm.resetScrollBarMouse()
			}
		}

		if wm.config.Frame {
			ev.MouseY--
			ev.MouseX--
		}

		// mouse on frame
		if ev.MouseX < 0 {
			ev.MouseX = 0
		}

		if ev.MouseY < 0 {
			ev.MouseY = 0
		}

		if wm.Focus().Window != childAtMouse {
			if ev.Key == term.MouseLeft {
				wm.SetFocus(wm.newNode(childAtMouse))
			}
			return
		}
	}

	var hexit bool
	focus := wm.focus
	size := wm.comp.SizeTiles()
	hexit, handled = focus.Content().Handle(ev)

	// if handler in focus wants to exit, close the window,
	// or signal exit to upstream handler if it was last window
	if hexit {
		if focus.IsFloating() {
			focus.Close()
			return
		}

		if exit = size == 1; exit {
			return
		}

		curr := wm.focus
		// make sure that if Handle above closed second to last window
		// we are not closing last window
		size := wm.comp.SizeTiles()
		if curr.ID() == focus.ID() && size != 1 {
			wm.ShiftFocus()
			curr.Close()
		}
	}

	return
}

// SplitVertical creates a new vertical split over the tile currently in focus.
func (wm *WindowManager) SplitVertical(win Window, h tui.Handler) (Window, bool) {
	w, ok := wm.comp.SplitVertical(win.Window, h)
	if !ok {
		return Window{}, false
	}
	ret := wm.newNode(w)
	wm.setFocusAttr(wm.focus)
	return ret, true
}

// SplitHorizontal creates a new horizontal split over the tile currently in focus.
func (wm *WindowManager) SplitHorizontal(win Window, h tui.Handler) (Window, bool) {
	w, ok := wm.comp.SplitHorizontal(win.Window, h)
	if !ok {
		return Window{}, false
	}
	ret := wm.newNode(w)
	wm.setFocusAttr(wm.focus)
	return ret, true
}

// FloatingWindow creates a floating window.
func (wm *WindowManager) FloatingWindow(
	content handler.Floating, cfg component.FloatingConfig,
) Window {
	ret := wm.newNode(wm.comp.FloatingWindow(content, cfg))
	wm.setFocusAttr(wm.focus)
	return ret
}

// SwapContentLeft swaps the content of the tile on the left side of the tile in focus.
// If the tile in focus is the left-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) SwapContentLeft() bool {
	return wm.swapContent((Window).TileLeft)
}

// SwapContentRight switches the focus to the tile on the right side of the tile in focus.
// If the tile in focus is the right-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) SwapContentRight() bool {
	return wm.swapContent((Window).TileRight)
}

// SwapContentUp switches the focus to the tile above the tile in focus.
// If the tile in focus is the up-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) SwapContentUp() bool {
	return wm.swapContent((Window).TileUp)
}

// SwapContentDown switches the focus to the tile beneath the tile in focus.
// If the tile in focus is the down-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) SwapContentDown() bool {
	return wm.swapContent((Window).TileDown)
}

// FocusLeft switches the focus to the tile on the left side of the tile in focus.
// If the tile in focus is the left-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusLeft() bool {
	return wm.switchFocus((Window).TileLeft)
}

// FocusRight switches the focus to the tile on the right side of the tile in focus.
// If the tile in focus is the right-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusRight() bool {
	return wm.switchFocus((Window).TileRight)
}

// FocusUp switches the focus to the tile above the tile in focus.
// If the tile in focus is the up-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusUp() bool {
	return wm.switchFocus((Window).TileUp)
}

// FocusDown switches the focus to the tile beneath the tile in focus.
// If the tile in focus is the down-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusDown() bool {
	return wm.switchFocus((Window).TileDown)
}

// Focus returns the tile currently in focus.
func (wm *WindowManager) Focus() Window {
	return wm.focus
}

// ShiftFocus attempts to shift to focus to another tile. It returns
// false if focus did not shift to another tile because there aren't any tiles left.
func (wm *WindowManager) ShiftFocus() (ok bool) {
	if wm.prevFocus != (Window{}) {
		ok = wm.Focus() != wm.prevFocus
		if ok {
			wm.SetFocus(wm.prevFocus)
			return
		}
	}

	ok = wm.FocusLeft()
	if ok {
		return
	}
	ok = wm.FocusUp()
	if ok {
		return
	}
	ok = wm.FocusRight()
	if ok {
		return
	}
	ok = wm.FocusDown()
	return
}

// Shiftable returns whether next call to ShiftFocus would return true.
func (wm *WindowManager) Shiftable() (w Window, ok bool) {
	focus := wm.Focus()
	w, ok = focus.TileLeft()
	if ok {
		return
	}
	w, ok = focus.TileUp()
	if ok {
		return
	}
	w, ok = focus.TileRight()
	if ok {
		return
	}
	w, ok = focus.TileDown()
	return
}

// Cursor returns the cursor coordinates of the tile in focus.
func (wm *WindowManager) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	window := wm.focus.Window
	offset := window.Position()
	content := wm.focus.Content()
	bounds := term.Coordinates{X: window.Width(), Y: window.Height()}
	if wm.config.Frame {
		offset.Y++
		offset.X++
		bounds.X -= 2
		bounds.Y -= 2
	}
	cursor, style, show := content.Cursor()
	if show {
		show = term.CoordinatesInBounds(cursor, bounds)
	}
	return term.CoordinatesSum(cursor, offset), style, show
}

// Selection satisfies tui.Handler.
func (wm *WindowManager) Selection() (string, bool) {
	content := wm.focus.Content()
	return content.Selection()
}

// Draw : tui.Component
func (wm *WindowManager) Draw(w term.Writer) {
	if wm.comp.SizeTiles()+wm.comp.SizeFloating() == 1 || !wm.config.Dim {
		wm.comp.Draw(w)
		return
	}

	focusWin := wm.focus.Window
	dimWriter := tterm.DimWriter(w)
	wm.comp.Iterate(func(win component.Window) {
		if win == focusWin {
			wm.comp.DrawWindow(win, w)
		} else {
			wm.comp.DrawWindow(win, dimWriter)
		}
	})
}

// DrawWindow draws target with the given term.Writer.
func (wm *WindowManager) DrawWindow(target Window, w term.Writer) {
	wm.comp.Iterate(func(win component.Window) {
		if win == target.Window {
			wm.comp.DrawWindow(win, w)
		}
	})
}

// SetDim sets whether next call to draw should use
// non-focus window diming feature.
func (wm *WindowManager) SetDim(to bool) (prev bool) {
	prev = wm.config.Dim
	wm.config.Dim = to
	return
}

// SetHeight fixes the height of the given window and returns true if possible,
// or returns false if not.
//
// Calling this method with height=0 effectively reverts back to the height
// being automatically distributed between windows.
func (wm *WindowManager) SetHeight(win Window, height int) bool {
	return wm.comp.SetHeight(win.Window, height)
}

// SetWidth fixes the width of the given window and returns true if possible,
// or returns false if not.
//
// Calling this method with width=0 effectively reverts back to the width
// being automatically distributed between windows.
func (wm *WindowManager) SetWidth(win Window, width int) bool {
	return wm.comp.SetWidth(win.Window, width)
}

// Resize : tui.Component
func (wm *WindowManager) Resize(width, height int) {
	wm.comp.Resize(width, height)
}

// SetFocus sets the passed tile in focus. It returns the previous tile in focus.
// The behaviour is undefined if the given tile is not part of this WindowManager.
func (wm *WindowManager) SetFocus(tile Window) (
	prev Window,
) {
	if tile.wm != wm {
		panic(fmt.Sprintf("Tile does not belong to"+
			"this window manager: %p vs %p", wm, tile.wm))
	}
	if wm.config.Frame {
		wm.focus.Window.SetFrameAttr(wm.config.FrameAttr)
		wm.focus.Window.SetFrameCharSet(wm.config.FrameCharSet)
		wm.setFocusAttr(tile)
	}
	prev = wm.focus
	wm.focus = tile
	if prev != tile {
		wm.dispatchOnFocus(prev, wm.focus)
		wm.prevFocus = prev
	}
	return
}

// DefaultWindowManagerConfig returns a sane WindowManagerConfig ready to use.
func DefaultWindowManagerConfig() WindowManagerConfig {
	return WindowManagerConfig{
		Dim:                 true,
		WindowManagerConfig: component.DefaultWindowManagerConfig(),
		FocusFrameCharSet:   compapi.FrameCharSetDefault(),
		FocusFrameAttr: term.Attributes{
			Fg: tcell.ColorRed,
			Bg: tcell.ColorDefault,
		},
	}
}

// SizeTiles returns the number of tiled windows in this WindowManager.
func (wm *WindowManager) SizeTiles() int {
	return wm.comp.SizeTiles()
}

// SizeFloating returns the number of floating windows in this WindowManager.
func (wm *WindowManager) SizeFloating() int {
	return wm.comp.SizeFloating()
}

// Iterate applies op to the content of all widnows of this WindowManager.
func (wm *WindowManager) Iterate(fn func(Window)) {
	wm.comp.Iterate(func(c component.Window) {
		fn(wm.newNode(c))
	})
}

func (wm *WindowManager) dispatchOnFocus(prev, focus Window) {
	for _, sub := range wm.subs {
		sub.OnFocus(prev, focus)
	}
}

// Subscribe subscribes sub to window focus events.
func (wm *WindowManager) Subscribe(sub WindowSubscriber) {
	wm.subs = append(wm.subs, sub)
	sub.OnFocus(Window{}, wm.focus)
}

// UnsubscribeAll unsubscribes all WindowSubscriber.
func (wm *WindowManager) UnsubscribeAll() {
	wm.subs = nil
}

func (wm *WindowManager) resetScrollBarMouse() {
	wm.prevMouseScrollBarDrag = false
}

func (wm *WindowManager) handleScrollBarMouse(
	win component.Window, barPos, barHeight int,
	frame *compapi.Frame, ev term.Event,
) (bool, bool) {
	frame.ScrollBarChar = wm.scrollBarHoverChar(frame)
	if ev.Key != term.MouseLeft {
		wm.resetScrollBarMouse()
		return false, false
	}

	prevDrag := wm.prevMouseScrollBarDrag
	if !prevDrag {
		wm.prevMouseScrollBarOffset = barPos - ev.MouseY
		wm.prevMouseScrollBarDrag = true
		wm.prevMouseLeftChild = win
		return false, false
	}

	scroll := frame.Content().(compapi.Scrollable)
	mouseOffset := wm.prevMouseScrollBarOffset
	if barPos-mouseOffset > ev.MouseY {
		for i := 0; i < barPos-mouseOffset-ev.MouseY && scroll.SeekUp(); i++ {
		}
		return false, true
	} else if barPos+mouseOffset < ev.MouseY {
		for i := 0; i < ev.MouseY-barPos+mouseOffset && scroll.SeekDown(); i++ {
		}
		return false, true
	}

	return false, false
}

func (wm *WindowManager) scrollBarHoverChar(f *compapi.Frame) (ch rune) {
	ch = wm.config.ScrollBarHoverChar
	if ch != 0 {
		return
	}
	ch = f.ScrollBarChar
	if ch != 0 {
		return
	}
	ch = f.FrameCharSet.VerticalRight
	return
}

func (wm *WindowManager) switchFocus(tileFn func(Window) (Window, bool)) bool {
	tile, ok := tileFn(wm.focus)
	if !ok {
		return false
	}
	wm.SetFocus(wm.newNode(tile.Window))
	return true
}

func (wm *WindowManager) swapContent(tileFn func(Window) (Window, bool)) bool {
	win, ok := tileFn(wm.focus)
	if !ok {
		return false
	}
	if win.IsFloating() || wm.focus.IsFloating() {
		return false
	}
	focusContent := wm.focus.Content()
	swapContent := win.SetContent(focusContent)
	wm.focus.SetContent(swapContent)
	return true
}
