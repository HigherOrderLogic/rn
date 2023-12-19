package handler

import (
	"fmt"

	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// WindowManagerConfig represents a WindowManager's
// configuration properties.
type WindowManagerConfig struct {
	component.WindowManagerConfig

	Dim               bool
	FocusFrameAttr    term.Attributes
	FocusFrameCharSet component.FrameCharSet
}

// WindowManager implements Handler as a tiled window manager.
type WindowManager struct {
	comp      component.WindowManager
	config    WindowManagerConfig
	prevFocus Window // best effort to set focus to prev win upon ShiftFocus
	focus     Window
	subs      []WindowSubscriber
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
	return
}

// SetFrameCharSet sets the frame border cells used to draw borders around tiles.
// Note that this has no effect if WindowManager was
// initialized with border == false.
func (wm *WindowManager) SetFrameCharSet(def, focus component.FrameCharSet) {
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

// Handle : Handler
func (wm *WindowManager) Handle(ev term.Event) (exit bool, handled bool) {
	if ev.Type == term.EventMouse {
		mousePos := term.Coordinates{X: ev.MouseX, Y: ev.MouseY}
		childAtMouse, ok := wm.comp.WindowAt(mousePos)
		if !ok {
			return
		}
		if wm.Focus().Window != childAtMouse {
			if ev.Key == term.MouseLeft {
				wm.SetFocus(wm.newNode(childAtMouse))
			}
			return
		}
		offset := childAtMouse.Position()
		if wm.config.Frame {
			offset.Y++
			offset.X++
		}
		ev.MouseX -= offset.X
		ev.MouseY -= offset.Y

		// mouse on frame
		if ev.MouseX < 0 {
			ev.MouseX = 0
		}
		if ev.MouseY < 0 {
			ev.MouseY = 0
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
	content Floating, cfg component.FloatingConfig,
) Window {
	ret := wm.newNode(wm.comp.FloatingWindow(content, cfg))
	wm.setFocusAttr(wm.focus)
	return ret
}

func (wm *WindowManager) switchFocus(tileFn func(Window) (Window, bool)) bool {
	tile, ok := tileFn(wm.focus)
	if !ok {
		return false
	}
	wm.SetFocus(wm.newNode(tile.Window))
	return true
}

// FocusLeft switches the focus to the tile on the left side of the tile in focus
// If the tile in focus is the left-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusLeft() bool {
	return wm.switchFocus((Window).TileLeft)
}

// FocusRight switches the focus to the tile on the right side of the tile in focus
// If the tile in focus is the right-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusRight() bool {
	return wm.switchFocus((Window).TileRight)
}

// FocusUp switches the focus to the tile above the tile in focus
// If the tile in focus is the up-most tile in this window manager, then this method does nothing.
func (wm *WindowManager) FocusUp() bool {
	return wm.switchFocus((Window).TileUp)
}

// FocusDown switches the focus to the tile beneath the tile in focus
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
	offset := wm.focus.Window.Position()
	content := wm.focus.Content()
	if wm.config.Frame {
		offset.Y++
		offset.X++
	}
	cursor, style, show := content.Cursor()
	return cell.CoordinatesSum(cursor, offset), style, show
}

// Man : Handler
func (wm *WindowManager) Man() tui.Manual {
	return tui.Manual{
		Summary: "WindowManager implements a tiled window manager.",
		Keys: tui.KeyMap{
			term.KeyComb{Mod: term.ModAlt, Ch: 'q'}: {
				ID:          "Exit",
				Description: "Exit handler.",
			},
			term.KeyComb{Mod: term.ModAlt, Ch: 'j'}: {
				ID:          "FocusDown",
				Description: "Switch focus to tile below tile in focus.",
			},
			term.KeyComb{Mod: term.ModAlt, Ch: 'k'}: {
				ID:          "FocusUp",
				Description: "Switch focus to tile above tile in focus.",
			},
			term.KeyComb{Mod: term.ModAlt, Ch: 'h'}: {
				ID:          "FocusLeft",
				Description: "Switch focus to tile on the left of tile in focus.",
			},
			term.KeyComb{Mod: term.ModAlt, Ch: 'l'}: {
				ID:          "FocusRight",
				Description: "Switch focus to tile on the right of tile in focus.",
			},
		},
	}
}

// Draw : tui.Component
func (wm *WindowManager) Draw(w term.Writer) {
	if wm.comp.SizeTiles()+wm.comp.SizeFloating() == 1 || !wm.config.Dim {
		wm.comp.Draw(w)
		return
	}

	focusWin := wm.focus.Window
	dimWriter := term.DimWriter(w)
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
		FocusFrameCharSet:   component.FrameCharSetDefault(),
		FocusFrameAttr: term.Attributes{
			Fg: term.ColorRed,
			Bg: term.ColorDefault,
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
