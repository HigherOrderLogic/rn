package screen

import (
	"math"

	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

// PrimaryBuffer wraps a Buffer to provide scroll-back for a primary vte screen buffer.
type PrimaryBuffer struct {
	AltBuffer
}

// NewPrimaryBuffer allocates storage for a new PrimaryBuffer and initializes it.
func NewPrimaryBuffer() *PrimaryBuffer {
	ret := new(PrimaryBuffer)
	ret.Init()
	return ret
}

// Init initializes this PrimaryBuffer.
func (b *PrimaryBuffer) Init() {
	b.AltBuffer.Init()
}

// Resize resizes this Buffer and resets the vertical margins.
func (b *PrimaryBuffer) Resize(width, height int) {
	b.AltBuffer.width = width
	b.AltBuffer.height = height
	b.scroll.Resize(width, height)
	b.Cells.ResetCapacity(width)
	b.ResetOffset()
}

// Dimensions returns the dimensions of this buffer.
func (b *PrimaryBuffer) Dimensions() (width, height int) {
	width = b.width
	height = b.height
	return
}

// ScrollUp panics. Use MoveToOffset or SetOffset.
func (b *PrimaryBuffer) ScrollUp(rows int) {
	// this prevents calling AltBuffer's ScrollUp inadvertently
	// and forces thinking about MoveToOffset vs SetOffset.
	panic("ScrollUp not supported for primary buffer, use SetOffset or MoveToOffset instead")
}

// ScrollDown panics. Use MoveToOffset or SetOffset.
func (b *PrimaryBuffer) ScrollDown(rows int) {
	// this prevents calling AltBuffer's ScrollDown inadvertently
	// and forces thinking about MoveToOffset vs SetOffset.
	panic("ScrollDown not supported for primary buffer, use SetOffset or MoveToOffset instead")
}

// MoveToOffset moves to the new offset.
// It does not change the cursor content/scroll position, thus
// changes the screen cursor position.
func (b *PrimaryBuffer) MoveToOffset(offset term.Coordinates) {
	b.AltBuffer.scroll.SetOffset(offset)
}

// SetOffset sets the raw offset of the underlying scroll.
// It does not change the screen cursor position, thus
// changes the content/scroll position.
func (b *PrimaryBuffer) SetOffset(offset term.Coordinates) {
	orig := b.CursorAtScreen()
	b.AltBuffer.scroll.SetOffset(offset)
	b.SetCursorAtScreen(orig, false)
}

// Offset returns the scroll offset of this PrimaryBuffer.
func (b *PrimaryBuffer) Offset() term.Coordinates {
	return b.AltBuffer.scroll.Offset()
}

// TopScrollableRegion is always 0 for a PrimaryBuffer,
// scrollable regions are not supported.
func (b *PrimaryBuffer) TopScrollableRegion() int {
	return 0
}

// BottomScrollableRegion is always height for a PrimaryBuffer,
// scrollable regions are not supported.
func (b *PrimaryBuffer) BottomScrollableRegion() int {
	return b.height
}

// InsertLines inserts blank lines on the cursor's position.
func (b *PrimaryBuffer) InsertLines(count int) {
	for i := 0; i < count; i++ {
		b.Cells.InsertRowAt(b.cursor.position.Y)
	}
}

// InsertLines deletes lines on the cursor's position.
func (b *PrimaryBuffer) DeleteLines(count int) {
	from := b.cursor.position
	to := from
	to.Y += count
	b.Cells.DeleteLine(from, to)
}

// SetCursorAtScroll sets the cursor at the content/scroll position c.
// The relative argument is ignored for PrimaryBuffer.
func (b *PrimaryBuffer) SetCursorAtScroll(c term.Coordinates, relative bool) {
	b.cursor.position.X = int(math.Max(float64(c.X), 0))
	b.cursor.position.Y = int(math.Max(float64(c.Y), 0))
}

// SetCursorAtScroll sets the cursor at the screen position c.
// The relative argument is ignored for PrimaryBuffer.
func (b *PrimaryBuffer) SetCursorAtScreen(c term.Coordinates, relative bool) {
	b.SetCursorAtScroll(cell.CoordinatesSum(c, b.scroll.Offset()), false)
}

// CursorAtScroll returns the cursor position in relation to the underlying
// content scroll.
func (b *PrimaryBuffer) CursorAtScroll() term.Coordinates {
	return b.cursor.position
}

// CursorAtScreen returns the current cursor position in relation to the
// screen coordinates.
func (b *PrimaryBuffer) CursorAtScreen() term.Coordinates {
	return cell.CoordinatesDiff(b.cursor.position, b.scroll.Offset())
}

// ResetOffset resets the offset to MaxOffset.
func (b *PrimaryBuffer) ResetOffset() {
	b.MoveToOffset(term.Coordinates{Y: b.MaxOffset()})
}

// MaxOffset returns the max offset such that the last line is at its bottommost position.
// If there's enough content to fill up the height of the screen, then that is height - 1,
// otherwise it ensures that the view content start at position 0.
func (b *PrimaryBuffer) MaxOffset() int {
	return int(math.Max(float64(b.Rows()-b.height), 0))
}

// ResetLines erases all the lines from start to end.
// The start to end range is left inclusive, right exclusive.
// As oppposed to AltBuffer's ResetLines, the `end` argument is not capped.
func (b *PrimaryBuffer) ResetLines(start, end int) {
	b.AltBuffer.ResetLinesWith(start, end, b.defaultChar)
}
