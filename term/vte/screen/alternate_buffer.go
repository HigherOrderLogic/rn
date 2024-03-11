package screen

import (
	"math"

	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/parser"
	"unstable.build/go-tui/text"
)

// AltBuffer implements a vte terminal screen buffer by wrapping a cell.Buffer
// and implementing vte screen buffer semantics. This buffer does not offer
// support for scroll-back or 'history', which keeps the implementation
// immensely simpler. Implementations that need scroll-back should
// use PrimaryAltBuffer instead.
type AltBuffer struct {
	Cells                  cell.Buffer
	scroll                 component.Scroll
	width                  int
	height                 int
	topScrollableRegion    int // start of scrollable region
	bottomScrollableRegion int // end of scrollable region
	selection              struct {
		mode text.SelectMode
		from term.Coordinates
		to   term.Coordinates
	}

	defaultChar rune
	tempScroll  [1][]term.Cell
	savedCursor CursorState
	cursor      CursorState
}

// CursorState holds the state of the cursor.
type CursorState struct {
	position term.Coordinates
	attr     term.Attributes
	hidden   bool // hidden flag on cursor attrs, not cursor itself
	Charsets map[parser.CharsetIndex]parser.StandardCharset
}

// NewAltBuffer allocates storage for a new AltBuffer and initializes it.
func NewAltBuffer() *AltBuffer {
	ret := new(AltBuffer)
	ret.Init()
	return ret
}

// Init initializes this buffer.
func (b *AltBuffer) Init() {
	b.defaultChar = ' '
	b.width = 1
	b.height = 1
	b.topScrollableRegion = 0
	b.bottomScrollableRegion = b.height
	b.cursor = CursorState{
		Charsets: make(map[parser.CharsetIndex]parser.StandardCharset),
	}
	b.Cells.InitPerformance(cell.DefaultTabspaces, 120, 80)
	b.resetLinesTrim(0, b.height, true, b.defaultChar)
	b.scroll.InitPerformance(&b.Cells)
}

// Resize resizes this AltBuffer and resets the vertical margins.
func (b *AltBuffer) Resize(width, height int) {
	b.width = width
	b.height = height
	b.resetLinesTrim(0, height, true, b.defaultChar)
	b.SetScrollableRegion(0, 0, true)
	b.scroll.Resize(width, height)
}

// Insert inserts a new character at the cursor position, shifting right
// all the cells to the right of the cursor. It does not extend the number columns
// in the buffer, as it should always be capped at exactly b.Width(), set by
// the previous call to Resize.
func (b *AltBuffer) Insert(c rune, width int, charset parser.CharsetIndex) {
	b.Cells.Insert(b.cursor.position, b.defaultChar)
	b.Write(c, width, charset)
	columns := b.Cells.Columns(b.cursor.position.Y)
	if columns > b.width {
		from := term.Coordinates{Y: b.cursor.position.Y, X: b.width}
		to := term.Coordinates{Y: b.cursor.position.Y, X: columns}
		b.Cells.Delete(from, to)
	}
}

// Write writes the given character with the given width to the cell
// at the current cursor position.
func (b *AltBuffer) Write(c rune, width int, charset parser.CharsetIndex) {
	if charset, ok := b.cursor.Charsets[charset]; ok {
		c = charset.Map(c)
	}
	if b.cursor.hidden {
		c = b.defaultChar
	}
	cell := b.CellAt(b.cursor.position)
	if cell == nil {
		b.Insert(c, width, charset)
		return
	}
	cell.Ch = c
	cell.Attributes = b.cursor.attr
	cell.Width = width
}

// ResetCells erases all the cells from start to end, on the current
// cursor line. The start to end range is left inclusive, right exclusive.
func (b *AltBuffer) ResetCells(start, end int) {
	b.resetCellsAt(b.cursor.position.Y, start, end, b.defaultChar)
}

// ResetLines erases all the lines from start to end.
// The start to end range is left inclusive, right exclusive.
// The `end` argument is capped to height.
func (b *AltBuffer) ResetLines(start, end int) {
	end = int(math.Min(float64(b.height), float64(end)))
	b.ResetLinesWith(start, end, b.defaultChar)
}

// ResetLinesWith erases all the lines from start to end,
// using with and the default attributes as the new content.
// The start to end range is left inclusive, right exclusive.
// The `end` argument is not capped, so care must be taken
// when using this method.
func (b *AltBuffer) ResetLinesWith(start, end int, with rune) {
	b.resetLinesTrim(start, end, false, with)
}

// Delete deletes the the given number of cells, shifting left
// all the cells to the right of the cursor.
func (b *AltBuffer) Delete(count int) {
	if count <= 0 {
		return
	}
	pos := b.cursor.position
	columns := b.Cells.Columns(pos.Y)
	count = int(math.Min(
		float64(count),
		float64(columns-pos.X),
	))

	cells := b.Cells.RawCells()
	copy(cells[pos.Y][pos.X:], cells[pos.Y][pos.X+count:])
	cells[pos.Y] = cells[pos.Y][:columns-count]

	// reset cells that were deleted
	b.ResetCells(columns-count, columns)
}

// SetCursorAtScreen updates the cursor position in the screen.
func (b *AltBuffer) SetCursorAtScreen(c term.Coordinates, relative bool) {
	var yOffset, yMax int
	if relative {
		yOffset = b.topScrollableRegion
		yMax = b.bottomScrollableRegion - 1
	} else {
		yMax = b.height - 1
	}
	b.cursor.position.X = int(math.Max(float64(c.X), 0))
	b.cursor.position.Y = int(math.Max(math.Min(float64(c.Y+yOffset), float64(yMax)), 0))
}

// SetCursorAtScroll for AltBuffer is equivalent to SetCursorAtScreen,
// since the viewed screen matches exactly the content in the scroll.
func (b *AltBuffer) SetCursorAtScroll(c term.Coordinates, relative bool) {
	b.SetCursorAtScreen(c, relative)
}

// ScrollUp scrolls up the scrollable region set by SetScrollableRegion by count of lines
func (b *AltBuffer) ScrollUp(start, end, count int) {
	if end-start <= count {
		b.ResetLinesWith(start, end, b.defaultChar)
		return
	}
	var temp [][]term.Cell
	if count == 1 {
		// optimization for long output streams on primary buffer that
		// cause Linefeed to scroll up exactly 1 when max scrollable history
		// is reached.
		temp = b.tempScroll[:]
	} else {
		temp = make([][]term.Cell, count)
	}

	cells := b.Cells.RawCells()
	copy(temp, cells[start:start+count])
	copy(cells[start:end-count], cells[start+count:end])
	copy(cells[end-count:end], temp)

	b.ResetLinesWith(end-count, end, b.defaultChar)
}

// ScrollDown scrolls down the scrollable region set by SetScrollableRegion by count of lines
func (b *AltBuffer) ScrollDown(start, end, count int) {
	if end-start <= count {
		b.ResetLinesWith(start, end, b.defaultChar)
		return
	}

	var temp [][]term.Cell
	if count == 1 {
		temp = b.tempScroll[:]
	} else {
		temp = make([][]term.Cell, count)
	}

	cells := b.Cells.RawCells()
	copy(temp, cells[end-count:end])
	copy(cells[start+count:end], cells[start:end-count])
	copy(cells[start:start+count], temp)

	b.ResetLinesWith(start, start+count, b.defaultChar)
}

// SetScrollableRegion sets the start and end of the scrollable area.
func (b *AltBuffer) SetScrollableRegion(top int, bottom int, end bool) {
	b.topScrollableRegion = int(math.Min(float64(top), float64(b.height)))
	if end {
		b.bottomScrollableRegion = b.height
	} else {
		b.bottomScrollableRegion = int(math.Min(float64(bottom), float64(b.height)))
	}
}

// BottomScrollableRegion returns the bottom margin, set by SetVerticalScrollableRegions.
func (b *AltBuffer) BottomScrollableRegion() int {
	return b.bottomScrollableRegion
}

// TopScrollableRegion returns the bottom margin, set by SetVerticalScrollableRegions.
func (b *AltBuffer) TopScrollableRegion() int {
	return b.topScrollableRegion
}

// Height returns the height set by Resize.
func (b *AltBuffer) Height() int {
	return b.height
}

// Width returns the height set by Resize.
func (b *AltBuffer) Width() int {
	return b.width
}

// Columns returns the columns of the given line.
func (b *AltBuffer) Columns(line int) int {
	if line < 0 || line >= b.Cells.Rows() {
		return 0
	}
	return b.Cells.Columns(line)
}

// CellAt returns the cell at the given position or nil
// if there's no cell at the given position.
func (b *AltBuffer) CellAt(pos term.Coordinates) *term.Cell {
	// Do not use Height, or intended number of screen lines here:
	// there might be a significant latency betwen resizing and upserting cells.
	// This effectively prevents Insert(pos)=ok then CellAt(pos)=nil
	cells := b.Cells.RawCells()
	if pos.Y >= len(cells) {
		return nil
	}
	if pos.X >= len(cells[pos.Y]) {
		return nil
	}
	return &cells[pos.Y][pos.X]
}

// SaveCursor saves the current cursor state to be restored
// later by RestoreCursor.
func (b *AltBuffer) SaveCursor() {
	b.SetSavedCursor(b.CloneCursor())
}

// CloneCursor clones the current cursor state and returns it.
func (b *AltBuffer) CloneCursor() (ret CursorState) {
	ret.attr = b.cursor.attr
	ret.position = b.cursor.position
	ret.Charsets = make(map[parser.CharsetIndex]parser.StandardCharset)
	for k, v := range b.cursor.Charsets {
		ret.Charsets[k] = v
	}
	return ret
}

// SetCursor sets the current cursor state to c.
func (b *AltBuffer) SetCursor(c CursorState) {
	b.cursor = c
}

// SetSavedCursor sets the saved cursor, to be restored
// later by RestoreCursor.
func (b *AltBuffer) SetSavedCursor(c CursorState) {
	b.savedCursor = c
}

// RestoreCursor sets the cursor to the previously stored
// cursor via SaveCursor or SetSavedCursor.
func (b *AltBuffer) RestoreCursor() {
	b.SetCursor(b.savedCursor)
}

// Cursor returns the current CursorState.
func (b *AltBuffer) Cursor() CursorState {
	return b.cursor
}

// CursorAtScreen returns the current cursor position in relation
// to the screen coordinates. This is equivalent to CursorAtScroll.
func (b *AltBuffer) CursorAtScreen() term.Coordinates {
	return b.cursor.position
}

// CursorAtScroll returns the current cursor position in relation
// to the scroll contents. This is equivalent to CursorAtScreen.
func (b *AltBuffer) CursorAtScroll() term.Coordinates {
	return b.cursor.position
}

// SetHiddenCursor marks as hidden the current cursor attributes.
func (b *AltBuffer) SetHiddenCursor(hidden bool) {
	b.cursor.hidden = hidden
}

// CursorAttributes returns the current cursor attributes.
func (b *AltBuffer) CursorAttributes() term.Attributes {
	return b.cursor.attr
}

// SetCursorAttributes sets the default cursor attributes.
func (b *AltBuffer) SetCursorAttributes(attr term.Attributes) {
	b.cursor.attr = attr
}

// ConfigureCharset configures the given charset index to use charset.
func (b *AltBuffer) ConfigureCharset(
	index parser.CharsetIndex, charset parser.StandardCharset,
) {
	b.cursor.Charsets[index] = charset
}

// MaxColumns returns the max columns of the underlying cell.Buffer.
func (b *AltBuffer) MaxColumns() int {
	return b.Cells.MaxColumns()
}

// Rows returns the number of rows in this AltBuffer.
func (b *AltBuffer) Rows() int {
	return b.Cells.Rows()
}

// Draw renders this buffer onto w.
func (b *AltBuffer) Draw(w term.Writer) {
	b.scroll.Draw(w)
}

// SetDefaultAttributes sets the default attributes to be used
// in the next call to Draw.
func (b *AltBuffer) SetDefaultAttributes(attr term.Attributes) {
	b.scroll.Attributes = attr
}

// Select select the word at the given screen position.
func (b *AltBuffer) SelectWordAt(pos term.Coordinates) {
	pos = cell.CoordinatesSum(pos, b.scroll.Offset())
	b.selection.from, b.selection.to, _ = b.scroll.WordAt(pos)
	b.selection.mode = text.StandardSelection
}

// Unselect clears the current selection if there's any.
func (b *AltBuffer) Unselect() {
	b.selection.mode = text.NoSelection
}

// Select anchors the given screen position as the start
// and end of a text selection.
func (b *AltBuffer) Select(pos term.Coordinates) {
	pos = cell.CoordinatesSum(pos, b.scroll.Offset())
	b.selection.from = pos
	pos.X++
	b.selection.to = pos
	b.selection.mode = text.StandardSelection
}

// Select anchors the current screen position as the end of a text selection.
func (b *AltBuffer) SelectEnd(pos term.Coordinates) {
	if b.selection.mode == text.NoSelection {
		return
	}
	pos = cell.CoordinatesSum(pos, b.scroll.Offset())
	// buffer selection has right exclusive semantics
	pos.X++
	b.selection.to = pos
}

// SelectLine anchors the current screen position as the start and end line of
// the text selection.
func (b *AltBuffer) SelectLine(pos term.Coordinates) {
	pos = cell.CoordinatesSum(pos, b.scroll.Offset())
	b.selection.from = pos
	pos.X++
	b.selection.to = pos
	b.selection.mode = text.LineSelection
}

// Selection returns the current selection or false if no
// text is currently selected.
func (b *AltBuffer) Selection() (cells [][]term.Cell, ok bool) {
	mode, from, to, _ := b.SelectionCoordinatesAtScroll()
	switch mode {
	case text.StandardSelection:
		cells, ok = b.Cells.Select(from, to)
	case text.LineSelection:
		cells, ok = b.Cells.SelectLine(from, to)
	default:
	}

	return
}

// SelectionCoordinatesAtScroll returns the content/scroll coordinates of the selected text.
// The returned coordinates are left inclusive, right exclusive.
func (b *AltBuffer) SelectionCoordinatesAtScroll() (
	mode text.SelectMode, from, to term.Coordinates, ok bool,
) {
	from, to = cell.SortFromTo(b.selection.from, b.selection.to)
	mode = b.selection.mode
	ok = b.selection.mode != text.NoSelection
	return
}

// SelectionCoordinatesAtScreen returns the screen coordinates of the selected text.
// The returned coordinates are left inclusive, right exclusive.
func (b *AltBuffer) SelectionCoordinatesAtScreen() (
	mode text.SelectMode, from, to term.Coordinates, ok bool,
) {
	mode, from, to, ok = b.SelectionCoordinatesAtScroll()
	from = cell.CoordinatesDiff(from, b.scroll.Offset())
	to = cell.CoordinatesDiff(to, b.scroll.Offset())
	return
}

// resetCellsAt erases all the cells from start to end, at the given line,
// The start to end range is left inclusive, right exclusive.
func (b *AltBuffer) resetCellsAt(y int, start, end int, with rune) {
	// ensure there are enough columns
	if y >= b.Cells.Rows() || end > b.Cells.Columns(y) {
		endInsert := int(math.Max(float64(end), float64(b.width)))
		b.Cells.Insert(term.Coordinates{Y: y, X: endInsert - 1}, with)
	}

	cells := b.Cells.RawCells()
	for x := start; x < end; x++ {
		cells[y][x] = term.Cell{
			Width: 1,
			Ch:    with,
			Attributes: term.Attributes{
				Bg: b.cursor.attr.Bg,
			},
		}
	}
}

func (b *AltBuffer) resetLinesTrim(start, end int, trim bool, with rune) {
	if start < 0 || end <= 0 || start >= end {
		return
	}
	// ensure there are enough rows
	if end > b.Cells.Rows() {
		b.Cells.Insert(term.Coordinates{Y: end - 1}, with)
	} else if end < b.Cells.Rows() && trim {
		b.Cells.TruncateFrom(term.Coordinates{Y: end - 1})
	}

	for y := start; y < end; y++ {
		columns := b.Cells.Columns(y)
		if columns > b.width {
			from := term.Coordinates{Y: y, X: b.width}
			to := term.Coordinates{Y: y, X: columns}
			b.Cells.Delete(from, to)
		}
		b.resetCellsAt(y, 0, b.width, with)
	}
}
