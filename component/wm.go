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

package component

import (
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
)

// WindowManagerConfig represents the configuration for a WindowManager
// to be initialized.
type WindowManagerConfig struct {
	Frame         bool
	FrameAttr     term.Attributes
	ScrollBarAttr term.Attributes
	ScrollBarChar rune
	component.FrameCharSet
	NoMaxSize bool
}

// WindowManager wraps a TileTree to provide an easier API.
type WindowManager struct {
	tree          TileTree
	width, height int
	float         []*floatingNode
	config        WindowManagerConfig

	// this is cached and calculated to figure out
	// how to offset windows when there are minimized floating windows.
	minimizedDirty  bool
	minimizedOffset term.Coordinates
	minimizedHeight int
	minimizedWidth  int
	minimizedPos    map[uint64]windowPos
}

// Draw satisfies tui.Component
func (wm *WindowManager) Draw(w term.Writer) {
	wm.Iterate(func(win Window) {
		wm.DrawWindow(win, w)
	})
}

// FloatingWindows return a slice of all the open floating windows.
func (wm *WindowManager) FloatingWindows() (ret []Window) {
	for _, w := range wm.float {
		ret = append(ret, wm.nodeToWindow(w))
	}
	return
}

// TileTree returns a TileTree representing all the open tiles.
func (wm *WindowManager) TileTree() *TileTree {
	return &wm.tree
}

// DrawWindow can be used to arbitrarily draw floating windows returned
// by FloatingWindows. If win is not a floating window, this method will panic.
func (wm *WindowManager) DrawWindow(win Window, w term.Writer) {
	if wm.minimizedDirty {
		wm.Resize(wm.width, wm.height)
	}

	nonMinimizedW := component.VirtualWriter{
		Writer: w,
		Offset: wm.minimizedOffset,
		Height: wm.height,
		Width:  wm.width,
	}

	f, ok := win.node.(*floatingNode)
	if !ok {
		wm.tree.DrawTile(win.node.(*TileNode), &nonMinimizedW)
		return
	}

	if f.minimized == 0 {
		f.Draw(&nonMinimizedW)
		return
	}

	winPos, ok := wm.minimizedPos[f.ID()]
	if !ok {
		panic("corrupt WindowManager: no pre-calculated minimized position for floating node")
	}

	if wm.minimizedOffset.Y < 0 || wm.minimizedOffset.X < 0 ||
		wm.minimizedWidth <= 0 || wm.minimizedHeight <= 0 {
		return
	}

	pos := winPos.from()
	length := winPos.length()
	switch f.minimized {
	case component.AlignmentTop:
		wm.drawMinimizedTop(w, f, pos, length)
	case component.AlignmentBottom:
		wm.drawMinimizedBottom(w, f, pos, length)
	case component.AlignmentLeft:
		wm.drawMinimizedLeft(w, f, pos, length)
	case component.AlignmentRight:
		wm.drawMinimizedRight(w, f, pos, length)
	}
}

// NewWindowManager allocates storage for a new WindowManager and initializes it.
func NewWindowManager(
	content tui.Component, config WindowManagerConfig,
) (*WindowManager, Window) {
	ret := new(WindowManager)
	win := ret.Init(content, config)
	return ret, win
}

// Init initializes this WindowManager with content and config.
func (wm *WindowManager) Init(
	content tui.Component, config WindowManagerConfig,
) (n Window) {
	wm.float = make([]*floatingNode, 0)
	wm.minimizedDirty = true
	wm.config = config
	wm.minimizedPos = make(map[uint64]windowPos)

	if wm.config.Frame {
		content = wm.withFrame(content)
	}
	node := wm.tree.Init(content)
	return wm.nodeToWindow(node)
}

// Iterate applies op to the content of all widnows of this WindowManager.
func (wm *WindowManager) Iterate(op func(Window)) {
	wm.tree.Iterate(func(node *TileNode) {
		op(wm.nodeToWindow(node))
	})
	for _, fw := range wm.float {
		op(wm.nodeToWindow(fw))
	}
}

// Resize satisfies tui.Component.
func (wm *WindowManager) Resize(width, height int) {
	wm.width, wm.height = width, height
	wm.calculateMinimizedOffsets()
	wm.tree.Resize(wm.minimizedWidth, wm.minimizedHeight)
	for _, fw := range wm.float {
		if fw.minimized == 0 || fw.minimizedPadding == 0 {
			fw.SetMaxSize(wm.minimizedWidth, wm.minimizedHeight)
		}
	}
	wm.minimizedDirty = false
}

// SizeTiles returns the number of tiled windows of this WindowManager.
func (wm *WindowManager) SizeTiles() int {
	return wm.tree.Size()
}

// SizeFloating returns the number of floating windows of this
// WindowManager.
func (wm *WindowManager) SizeFloating() int {
	return len(wm.float)
}

// SplitHorizontal creates a new Window by splitting the height of win in two and
// initializes it with content. It returns true if it succeeds or false if
// this window is a floating window created via FloatingWindow and so it's not a tile.
func (wm *WindowManager) SplitHorizontal(win Window, content tui.Component) (Window, bool) {
	t, ok := win.node.(*TileNode)
	if !ok {
		return Window{}, false
	}
	if wm.config.Frame {
		content = wm.withFrame(content)
	}
	node := wm.tree.SplitHorizontal(t, content)
	return wm.nodeToWindow(node), true
}

// SplitVertical creates a new Window by splitting the width of win in two and
// initializes it with content.
func (wm *WindowManager) SplitVertical(win Window, content tui.Component) (Window, bool) {
	t, ok := win.node.(*TileNode)
	if !ok {
		return Window{}, false
	}
	if wm.config.Frame {
		content = wm.withFrame(content)
	}
	node := wm.tree.SplitVertical(t, content)
	return wm.nodeToWindow(node), true
}

// SetHeight fixes the height of the given window and returns true if possible,
// or returns false if not.
//
// Calling this method with height=0 effectively reverts back to the height
// being automatically distributed between windows.
func (wm *WindowManager) SetHeight(win Window, height int) bool {
	fn, ok := win.node.(*floatingNode)
	if ok {
		return fn.setHeight(height)
	}
	t := win.node.(*TileNode)
	// 0 resets, but less than 3 would make the window almost disappear
	if wm.config.Frame && height < 3 && height > 0 {
		return false
	}
	return t.SetFixedHeight(height)
}

// SetWidth fixes the width of the given window and returns true if possible,
// or returns false if not.
//
// Calling this method with width=0 effectively reverts back to the width
// being automatically distributed between windows.
func (wm *WindowManager) SetWidth(win Window, width int) bool {
	fn, ok := win.node.(*floatingNode)
	if ok {
		return fn.setWidth(width)
	}
	t := win.node.(*TileNode)
	// 0 resets, but less than 3 would make the window almost disappear
	if wm.config.Frame && width < 3 && width > 0 {
		return false
	}
	return t.SetFixedWidth(width)
}

// WindowAt returns the window at pos.
func (wm *WindowManager) WindowAt(pos term.Coordinates) (Window, bool) {
	if pos.X < 0 || pos.Y < 0 || pos.X >= wm.width || pos.Y >= wm.height {
		return Window{}, false
	}

	if wm.minimizedDirty {
		wm.Resize(wm.width, wm.height)
	}

	// first return any floating window that might be rendered over everything else
	floating := -1
	for i, fw := range wm.float {
		if fw.minimized != 0 {
			continue
		}
		fwpos := wm.nodeToWindow(fw).Position()
		fwidth, fheight := fw.Width(), fw.Height()
		if pos.X >= fwpos.X && pos.Y >= fwpos.Y &&
			pos.X < fwpos.X+fwidth && pos.Y < fwpos.Y+fheight {
			floating = i
		}
	}
	if floating >= 0 {
		return wm.nodeToWindow(wm.float[floating]), true
	}

	for _, winPos := range wm.minimizedPos {
		from, to := winPos.position()
		if pos.X >= from.X && pos.X < to.X && pos.Y >= from.Y && pos.Y < to.Y {
			return winPos.win, true
		}
	}

	pos.Y -= wm.minimizedOffset.Y
	pos.X -= wm.minimizedOffset.X
	return wm.nodeToWindow(wm.tree.TileAt(pos)), true
}

// SetFrameCharSet sets the defaultframe border cells used
// to draw borders around tiles.  Note that this has no effect if WindowManager was
// initialized with border == false.
func (wm *WindowManager) SetFrameCharSet(b component.FrameCharSet) {
	if !wm.config.Frame {
		return
	}

	wm.config.FrameCharSet = b
	wm.Iterate(func(w Window) {
		w.node.Content().(*component.Frame).FrameCharSet = b
	})
}

// SetFrameAttr sets the default and focus window border attributes. Note that
// this has no effect if WindowManager was initialized with border == false.
func (wm *WindowManager) SetFrameAttr(attr term.Attributes) {
	if !wm.config.Frame {
		return
	}

	wm.config.FrameAttr = attr
	wm.Iterate(func(w Window) {
		w.node.Content().(*component.Frame).Attributes = attr
	})
}

// FloatingConfig abstracts configuration for
// creating floating windows.
type FloatingConfig struct {
	// Sets the alignment of the window.
	component.Alignment
	// Offset is to be applied to the position of the window
	// after alignment has been determined.
	Offset term.Coordinates
}

// FloatingWindow creates a floating window.
func (wm *WindowManager) FloatingWindow(
	content component.Floating, cfg FloatingConfig,
) Window {
	if wm.config.Frame {
		content = wm.withFrame(content)
	}
	f := newFloatingNode(wm, content, cfg, wm.minimizedWidth, wm.minimizedHeight)
	wm.float = append(wm.float, f)
	wm.minimizedDirty = true
	return wm.nodeToWindow(f)
}

// DefaultWindowManagerConfig returns a sane WindowManagerConfig.
func DefaultWindowManagerConfig() WindowManagerConfig {
	charset := component.FrameCharSetDefault()
	return WindowManagerConfig{
		Frame:         true,
		FrameAttr:     term.Attributes{},
		FrameCharSet:  charset,
		NoMaxSize:     true,
		ScrollBarAttr: term.Attributes{Attrs: tcell.AttrBold},
	}
}

func (wm *WindowManager) withFrame(handler tui.Component) *component.Frame {
	f := component.NewFrame(handler)
	f.FrameCharSet = wm.config.FrameCharSet
	f.Attributes = wm.config.FrameAttr
	f.ScrollBarAttributes = wm.config.ScrollBarAttr
	f.ScrollBarChar = wm.config.ScrollBarChar
	return f
}

func (wm *WindowManager) closeFloatingWindow(w *floatingNode) {
	for i, f := range wm.float {
		if f == w {
			wm.float = append(wm.float[:i], wm.float[i+1:]...)
			break
		}
	}
}

func (wm *WindowManager) nodeToWindow(node windowNode) Window {
	return Window{node: node, wm: wm}
}

func (wm *WindowManager) calculateMinimizedOffsets() {
	clear(wm.minimizedPos)

	var offsetLeft, offsetTop, offsetBottom, offsetRight int
	for _, win := range wm.FloatingWindows() {
		fn := win.node.(*floatingNode)
		switch fn.minimized {
		case component.AlignmentTop:
			wm.minimizedPos[win.ID()] = windowPos{pos: offsetTop, win: win}
			offsetTop++
			offsetTop += fn.minimizedPadding
		case component.AlignmentBottom:
			wm.minimizedPos[win.ID()] = windowPos{pos: offsetBottom, win: win}
			offsetBottom++
			offsetBottom += fn.minimizedPadding
		case component.AlignmentLeft:
			wm.minimizedPos[win.ID()] = windowPos{pos: offsetLeft, win: win}
			offsetLeft++
			offsetLeft += fn.minimizedPadding
		case component.AlignmentRight:
			wm.minimizedPos[win.ID()] = windowPos{pos: offsetRight, win: win}
			offsetRight++
			offsetRight += fn.minimizedPadding
		default:
		}
	}
	wm.minimizedOffset = term.Coordinates{Y: offsetTop, X: offsetLeft}
	wm.minimizedHeight = wm.height - offsetBottom - offsetTop
	wm.minimizedWidth = wm.width - offsetRight - offsetLeft

	// now that we know where everything is positioned,
	// resize floating nodes with padding, aka that will
	// be partially rendered in calls to Draw
	for _, fn := range wm.float {
		if fn.minimizedPadding == 0 || fn.minimized == 0 {
			continue
		}
		switch fn.minimized {
		case component.AlignmentTop, component.AlignmentBottom:
			width := wm.width
			if wm.config.Frame {
				width -= 2
			}
			width= max(0, width)
			fn.Content().Resize(width, fn.minimizedPadding)
		case component.AlignmentLeft, component.AlignmentRight:
			height := wm.minimizedHeight
			if wm.config.Frame {
				height -= 2
			}
			height = max(0, height)
			fn.Content().Resize(fn.minimizedPadding, height)
		}
	}
}

func (wm *WindowManager) drawMinimizedContent(
	w term.Writer, f *floatingNode, at term.Coordinates,
) {
	vw := component.VirtualWriter{
		Writer: w,
		Offset: at,
		Height: wm.height,
		Width:  wm.width,
	}
	f.Content().Draw(&vw)
}

func (wm *WindowManager) drawMinimizedTop(
	w term.Writer, f *floatingNode, pos term.Coordinates, length int,
) {
	frameAttr := wm.config.FrameAttr
	if attr, ok := f.content.C.(component.WithAttributes); ok {
		frameAttr = attr.SetAttr(term.Attributes{})
		attr.SetAttr(frameAttr)
	}
	w.SetCell(pos, term.Cell{
		Width:      1,
		Ch:         wm.config.TopLeft,
		Attributes: frameAttr,
	})
	for x := 1; x < length-1; x++ {
		w.SetCell(term.Coordinates{X: x, Y: pos.Y}, term.Cell{
			Width:      1,
			Ch:         wm.config.HorizontalTop,
			Attributes: frameAttr,
		})
	}
	w.SetCell(term.Coordinates{X: length - 1, Y: pos.Y}, term.Cell{
		Width:      1,
		Ch:         wm.config.TopRight,
		Attributes: frameAttr,
	})
	if f.minimizedPadding == 0 {
		return
	}
	for y := pos.Y + 1; y < pos.Y+1+f.minimizedPadding; y++ {
		w.SetCell(term.Coordinates{X: length - 1, Y: y}, term.Cell{
			Width:      1,
			Ch:         wm.config.VerticalRight,
			Attributes: frameAttr,
		})
		w.SetCell(term.Coordinates{X: pos.X, Y: y}, term.Cell{
			Width:      1,
			Ch:         wm.config.VerticalLeft,
			Attributes: frameAttr,
		})
	}
	at := pos
	if wm.config.Frame {
		at.X++
		at.Y++
	}
	wm.drawMinimizedContent(w, f, at)
}

func (wm *WindowManager) drawMinimizedBottom(
	w term.Writer, f *floatingNode, pos term.Coordinates, length int,
) {
	frameAttr := wm.config.FrameAttr
	if attr, ok := f.content.C.(component.WithAttributes); ok {
		frameAttr = attr.SetAttr(term.Attributes{})
		attr.SetAttr(frameAttr)
	}
	bottomFramePos := pos.Y + f.minimizedPadding
	w.SetCell(term.Coordinates{X: pos.X, Y: bottomFramePos}, term.Cell{
		Width:      1,
		Ch:         wm.config.BottomLeft,
		Attributes: frameAttr,
	})
	for x := 1; x < length-1; x++ {
		w.SetCell(term.Coordinates{X: x, Y: bottomFramePos}, term.Cell{
			Width:      1,
			Ch:         wm.config.HorizontalBottom,
			Attributes: frameAttr,
		})
	}
	w.SetCell(term.Coordinates{X: length - 1, Y: bottomFramePos}, term.Cell{
		Width:      1,
		Ch:         wm.config.BottomRight,
		Attributes: frameAttr,
	})
	if f.minimizedPadding == 0 {
		return
	}
	for y := pos.Y; y < pos.Y+f.minimizedPadding; y++ {
		w.SetCell(term.Coordinates{X: length - 1, Y: y}, term.Cell{
			Width:      1,
			Ch:         wm.config.VerticalRight,
			Attributes: frameAttr,
		})
		w.SetCell(term.Coordinates{X: pos.X, Y: y}, term.Cell{
			Width:      1,
			Ch:         wm.config.VerticalLeft,
			Attributes: frameAttr,
		})
	}
	at := pos
	if wm.config.Frame {
		at.X++
		// no Y++ because top frame of bottom minimized window
		// is omitted.
	}
	wm.drawMinimizedContent(w, f, at)
}

func (wm *WindowManager) drawMinimizedLeft(
	w term.Writer, f *floatingNode, pos term.Coordinates, length int,
) {
	frameAttr := wm.config.FrameAttr
	if attr, ok := f.content.C.(component.WithAttributes); ok {
		frameAttr = attr.SetAttr(term.Attributes{})
		attr.SetAttr(frameAttr)
	}
	yOffset := wm.minimizedOffset.Y
	w.SetCell(pos, term.Cell{
		Width:      1,
		Ch:         wm.config.TopLeft,
		Attributes: frameAttr,
	})
	for y := 1 + yOffset; y < yOffset+length-1; y++ {
		w.SetCell(term.Coordinates{X: pos.X, Y: y}, term.Cell{
			Width:      1,
			Ch:         wm.config.VerticalLeft,
			Attributes: frameAttr,
		})
	}
	w.SetCell(term.Coordinates{X: pos.X, Y: yOffset + length - 1}, term.Cell{
		Width:      1,
		Ch:         wm.config.BottomLeft,
		Attributes: frameAttr,
	})
	if f.minimizedPadding == 0 {
		return
	}
	for x := pos.X + 1; x < pos.X+1+f.minimizedPadding; x++ {
		w.SetCell(term.Coordinates{X: x, Y: pos.Y}, term.Cell{
			Width:      1,
			Ch:         wm.config.HorizontalTop,
			Attributes: frameAttr,
		})
		w.SetCell(term.Coordinates{X: x, Y: yOffset + length - 1}, term.Cell{
			Width:      1,
			Ch:         wm.config.HorizontalBottom,
			Attributes: frameAttr,
		})
	}
	at := pos
	if wm.config.Frame {
		at.X++
		at.Y++
	}
	wm.drawMinimizedContent(w, f, at)
}

func (wm *WindowManager) drawMinimizedRight(
	w term.Writer, f *floatingNode, pos term.Coordinates, length int,
) {
	frameAttr := wm.config.FrameAttr
	if attr, ok := f.content.C.(component.WithAttributes); ok {
		frameAttr = attr.SetAttr(term.Attributes{})
		attr.SetAttr(frameAttr)
	}
	yOffset := wm.minimizedOffset.Y
	rightFramePos := pos.X + f.minimizedPadding
	w.SetCell(term.Coordinates{X: rightFramePos, Y: pos.Y}, term.Cell{
		Width:      1,
		Ch:         wm.config.TopRight,
		Attributes: frameAttr,
	})
	for y := 1 + yOffset; y < yOffset+length-1; y++ {
		w.SetCell(term.Coordinates{X: rightFramePos, Y: y}, term.Cell{
			Width:      1,
			Ch:         wm.config.VerticalRight,
			Attributes: frameAttr,
		})
	}
	w.SetCell(term.Coordinates{X: rightFramePos, Y: yOffset + length - 1},
		term.Cell{
			Width:      1,
			Ch:         wm.config.BottomRight,
			Attributes: frameAttr,
		})
	if f.minimizedPadding == 0 {
		return
	}
	for x := pos.X; x < rightFramePos; x++ {
		w.SetCell(term.Coordinates{X: x, Y: pos.Y}, term.Cell{
			Width:      1,
			Ch:         wm.config.HorizontalTop,
			Attributes: frameAttr,
		})
		w.SetCell(term.Coordinates{X: x, Y: yOffset + length - 1}, term.Cell{
			Width:      1,
			Ch:         wm.config.HorizontalBottom,
			Attributes: frameAttr,
		})
	}
	at := pos
	if wm.config.Frame {
		at.Y++
		// no X++ because left frame on right-side minimized window
		// is omitted.
	}
	wm.drawMinimizedContent(w, f, at)
}

func (wm *WindowManager) topMostTile() Window {
	return wm.nodeToWindow(wm.tree.root.leftMostChild())
}

func (wm *WindowManager) bottomMostTile() Window {
	return wm.nodeToWindow(wm.tree.root.rightMostChild())
}

func (wm *WindowManager) rightMostTile() Window {
	return wm.nodeToWindow(wm.tree.root.rightMostChild())
}

func (wm *WindowManager) leftMostTile() Window {
	return wm.nodeToWindow(wm.tree.root.leftMostChild())
}

// post-draw helper to find where minimized windows are positioned
type windowPos struct {
	win Window
	pos int
}

func (w windowPos) position() (term.Coordinates, term.Coordinates) {
	from := w.from()
	length := w.length()
	var to term.Coordinates
	fn := w.win.node.(*floatingNode)
	switch fn.minimized {
	case component.AlignmentTop, component.AlignmentBottom:
		to = from
		to.Y++
		to.Y += fn.minimizedPadding
		to.X += length
	case component.AlignmentLeft, component.AlignmentRight:
		to = from
		to.X++
		to.X += fn.minimizedPadding
		to.Y += length
	default:
		panic("invalid minimize alignment")
	}
	return from, to
}

func (w windowPos) from() term.Coordinates {
	fn := w.win.node.(*floatingNode)
	switch fn.minimized {
	case component.AlignmentTop:
		return term.Coordinates{X: 0, Y: w.pos}
	case component.AlignmentBottom:
		return term.Coordinates{X: 0, Y: w.win.wm.height - w.pos - 1 - fn.minimizedPadding}
	case component.AlignmentLeft:
		return term.Coordinates{X: w.pos, Y: w.win.wm.minimizedOffset.Y}
	case component.AlignmentRight:
		return term.Coordinates{
			X: w.win.wm.width - w.pos - 1 - fn.minimizedPadding,
			Y: w.win.wm.minimizedOffset.Y,
		}
	default:
		panic("invalid minimize alignment")
	}
}

func (w windowPos) length() int {
	switch w.win.node.(*floatingNode).minimized {
	case component.AlignmentTop:
		return w.win.wm.width
	case component.AlignmentBottom:
		return w.win.wm.width
	case component.AlignmentLeft:
		return w.win.wm.minimizedHeight
	case component.AlignmentRight:
		return w.win.wm.minimizedHeight
	default:
		panic("invalid minimize alignment")
	}
}
