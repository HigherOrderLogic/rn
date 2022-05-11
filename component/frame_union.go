package component

import (
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
)

// FrameUnionCharSet configures the characters used to draw the frame union
// between the top and bottom components.
type FrameUnionCharSet struct {
	Left   rune
	Right  rune
	Top    rune
	Bottom rune
}

// DefaultFrameUnionCharSet returns the default FrameUnionCharSet used.
func DefaultFrameUnionCharSet() (ret FrameUnionCharSet) {
	ret.Left = '├'
	ret.Right = '┤'
	ret.Top = '┬'
	ret.Bottom = '┴'
	return
}

// FrameUnion is a tui.Component which Draws a main Component
// at the max available width and height, but otherwise depending on
// how many other statically sized components are stacked either left, right
// top or bottom.
//
// It respects the stacked components height if stacked on top or bottom
// and it respects the stacked components width if stacked left or right.
// The API uses Virtual instead of tui.Component to determine what's the desired
// height or width.
type FrameUnion struct {
	main          Virtual
	top, bottom   []*frameVirtual
	left, right   []*frameVirtual
	height, width int

	// Frame determines whether underlying components are an instance of Frame
	// and so FrameUnion should stitch them together with FrameUnionCharSet.
	// Default is true.
	Frame bool

	// Attributes to be set to FrameUnionCharSet.
	term.Attributes

	FrameUnionCharSet
}

type frameVirtual struct {
	size int
	Virtual
}

// NewFrameUnion allocates storage for a new FrameUnion and initializes it.
func NewFrameUnion(main tui.Component) *FrameUnion {
	ret := new(FrameUnion)
	ret.Init(main)
	return ret
}

// Init initializes this frame union with main as the main compontent.
func (u *FrameUnion) Init(main tui.Component) {
	u.FrameUnionCharSet = DefaultFrameUnionCharSet()
	u.main.C = main
	u.Frame = true
}

// UnionTop stacks top on top of the main component. This
// method panics if top is nil.
func (u *FrameUnion) UnionTop(top tui.Component, height int) {
	if top == nil {
		panic("invalid componen.Virtual")
	}
	u.top = append(u.top, &frameVirtual{Virtual: Virtual{C: top}, size: height})
	u.Resize(u.width, u.height)
}

// UnionBottom stacks bottom under of the main component. This
// method panics if bottom is nil.
func (u *FrameUnion) UnionBottom(bottom tui.Component, height int) {
	if bottom == nil {
		panic("invalid componen.Virtual")
	}
	head := []*frameVirtual{{Virtual: Virtual{C: bottom}, size: height}}
	u.bottom = append(head, u.bottom...)
	u.Resize(u.width, u.height)
}

// UnionLeft stacks left to the left of the main component. This
// method panics if left is nil.
func (u *FrameUnion) UnionLeft(left tui.Component, width int) {
	if left == nil {
		panic("invalid componen.Virtual")
	}
	u.left = append(u.left, &frameVirtual{Virtual: Virtual{C: left}, size: width})
	u.Resize(u.width, u.height)
}

// UnionRight stacks right to the right of the main component. This
// method panics if right is nil.
func (u *FrameUnion) UnionRight(right tui.Component, width int) {
	if right == nil {
		panic("invalid componen.Virtual")
	}
	head := []*frameVirtual{{Virtual: Virtual{C: right}, size: width}}
	u.right = append(head, u.right...)
	u.Resize(u.width, u.height)
}

func (u *FrameUnion) componentAt(
	components []*frameVirtual, pos term.Coordinates,
) (Virtual, bool) {
	for _, t := range components {
		tpos := t.Position()
		twidth := t.Width()
		theight := t.Height()
		if pos.X >= tpos.X && pos.Y >= tpos.Y && pos.X < tpos.X+twidth && pos.Y < tpos.Y+theight {
			return t.Virtual, true
		}
	}
	return Virtual{}, false
}

// ComponentAt returns the component at pos or false if there's no component at pos.
func (u *FrameUnion) ComponentAt(pos term.Coordinates) (Virtual, bool) {
	frameVirtualMain := frameVirtual{Virtual: u.main}
	main := [1]*frameVirtual{&frameVirtualMain}
	c, ok := u.componentAt(main[:], pos)
	if ok {
		return c, true
	}
	c, ok = u.componentAt(u.top, pos)
	if ok {
		return c, true
	}
	c, ok = u.componentAt(u.bottom, pos)
	if ok {
		return c, true
	}
	c, ok = u.componentAt(u.left, pos)
	if ok {
		return c, true
	}
	return u.componentAt(u.right, pos)
}

// MainPosition returns the main component's offset from the top-left corner.
func (u *FrameUnion) MainPosition() (offset term.Coordinates) {
	return u.main.Position()
}

func (u *FrameUnion) resizeTopBottom(width, height int) (int, int) {
	var frameOverlap int
	if u.Frame {
		frameOverlap = 1
	}

	var topHeight int
	for _, top := range u.top {
		top.Move(term.Coordinates{Y: topHeight})

		height := top.size
		if height > 0 {
			topHeight += height - frameOverlap
			top.Resize(width, height)
		} else {
			top.Resize(0, 0)
		}
	}

	var bottomHeight int
	for _, bottom := range u.bottom {
		height := bottom.size
		if height > 0 {
			bottomHeight += height - frameOverlap
		}
	}

	// if there's too many union components for available height
	// do not draw them.
	mainHeight := u.height - topHeight - bottomHeight
	if mainHeight < 1+frameOverlap {
		for _, top := range u.top {
			top.Resize(0, 0)
		}
		for _, bottom := range u.bottom {
			bottom.Resize(0, 0)
		}
		return u.height, 0
	}

	bottomOffset := topHeight + mainHeight - frameOverlap
	for _, bottom := range u.bottom {
		bottom.Move(term.Coordinates{Y: bottomOffset})
		height := bottom.size
		if height > 0 {
			bottom.Resize(width, height)
			bottomOffset += height - frameOverlap
		} else {
			bottom.Resize(0, 0)
		}
	}

	return mainHeight, topHeight
}

func (u *FrameUnion) resizeLeftRight(width, height, topOffset int) (int, int) {
	var frameOverlap int
	if u.Frame {
		frameOverlap = 1
	}

	var leftWidth int
	for _, left := range u.left {
		left.Move(term.Coordinates{X: leftWidth, Y: topOffset})

		width := left.size
		if width > 0 {
			leftWidth += width - frameOverlap
			left.Resize(width, height)
		} else {
			left.Resize(0, 0)
		}
	}

	var rightWidth int
	for _, right := range u.right {
		width := right.size
		if width > 0 {
			rightWidth += width - frameOverlap
		}
	}

	mainWidth := u.width - leftWidth - rightWidth
	if mainWidth < 1+frameOverlap {
		for _, left := range u.left {
			left.Resize(0, 0)
		}
		for _, right := range u.right {
			right.Resize(0, 0)
		}
		return u.width, 0
	}

	rightOffset := leftWidth + mainWidth - frameOverlap
	for _, right := range u.right {
		right.Move(term.Coordinates{Y: topOffset, X: rightOffset})
		width := right.size
		if width > 0 {
			right.Resize(width, height)
			rightOffset += width - frameOverlap
		} else {
			right.Resize(0, 0)
		}
	}

	return mainWidth, leftWidth
}

// Resize satisfies tui.Component
func (u *FrameUnion) Resize(width, height int) {
	u.height, u.width = height, width

	mainHeight, topOffset := u.resizeTopBottom(width, height)
	mainWidth, leftOffset := u.resizeLeftRight(width, mainHeight, topOffset)

	u.main.Resize(mainWidth, mainHeight)
	u.main.Move(term.Coordinates{Y: topOffset, X: leftOffset})
}

func (u *FrameUnion) setVerticalUnionFrameCells(w term.Writer, v *frameVirtual, y int) {
	if v.Height() == 0 || v.Width() == 0 {
		return
	}
	w.SetCell(term.Coordinates{Y: y},
		term.Cell{Ch: u.Left, Bg: u.Attributes.Bg, Fg: u.Attributes.Fg})
	if u.width > 0 {
		w.SetCell(term.Coordinates{X: u.width - 1, Y: y},
			term.Cell{Ch: u.Right, Bg: u.Attributes.Bg, Fg: u.Attributes.Fg})
	}
}

func (u *FrameUnion) setHorizontalUnionFrameCells(w term.Writer, v *frameVirtual, height, x, y int) {
	if v.Height() == 0 || v.Width() == 0 {
		return
	}
	w.SetCell(term.Coordinates{X: x, Y: y},
		term.Cell{Ch: u.Top, Bg: u.Attributes.Bg, Fg: u.Attributes.Fg})
	if height > 0 {
		w.SetCell(term.Coordinates{X: x, Y: y + height - 1},
			term.Cell{Ch: u.Bottom, Bg: u.Attributes.Bg, Fg: u.Attributes.Fg})
	}
}

// Draw satisfies tui.Component
func (u *FrameUnion) Draw(w term.Writer) {
	for _, top := range u.top {
		top.Draw(w)
	}
	for _, bottom := range u.bottom {
		bottom.Draw(w)
	}
	for _, left := range u.left {
		left.Draw(w)
	}
	for _, right := range u.right {
		right.Draw(w)
	}

	u.main.Draw(w)

	if u.main.Height() < 2 || u.main.Width() < 2 || !u.Frame {
		return
	}

	for _, left := range u.left {
		pos := left.Position()
		u.setHorizontalUnionFrameCells(w, left, u.main.Height(), pos.X+left.Width()-1, pos.Y)
	}

	for _, right := range u.right {
		pos := right.Position()
		u.setHorizontalUnionFrameCells(w, right, u.main.Height(), pos.X, pos.Y)
	}
	for _, top := range u.top {
		u.setVerticalUnionFrameCells(w, top, top.Position().Y+top.Height()-1)
	}

	for _, bottom := range u.bottom {
		u.setVerticalUnionFrameCells(w, bottom, bottom.Position().Y)
	}
}
