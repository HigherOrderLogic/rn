package cell

import (
	"context"
	"io"
	"math"
	"strings"

	"unstable.build/go-tui/term"
)

// A Buffer offers a high level API to manipulate a matrix of term.Cell.
type Buffer struct {
	cells    *rawCells
	undoer   *undoer
	selector selector
	rootPub  *syncPublisher
	usagePub *syncPublisher
	safew    safeEditor

	// effective View and Editor
	view   View
	editor Editor
}

type safeEditor struct {
	editor Editor
	view   View
}

func fromToInBounds(cells View, from, to term.Coordinates) (
	newFrom, newTo term.Coordinates, ok bool,
) {
	rows := cells.Rows()
	if rows == 0 || from.Y >= rows || (from.Y == rows-1 && from.X > cells.Columns(from.Y)) {
		return
	}

	if cols := cells.Columns(from.Y); from.X > cols {
		from.X = cols
	}

	if to.Y >= rows {
		to.Y = rows - 1
		to.X = cells.Columns(rows - 1)
	} else if cols := cells.Columns(to.Y); to.X > cols {
		to.X = cols
	}

	return from, to, true
}

func (s safeEditor) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	// only check in case of delete range
	if start != end {
		var ok bool
		start, end, ok = fromToInBounds(s.view, start, end)
		if !ok {
			from = start
			to = start
			return
		}
	}
	return s.editor.Edit(ctx, start, end, str)
}

// NewBuffer allocates storage for a new Buffer and initializes it.
func NewBuffer() (b *Buffer) {
	b = new(Buffer)
	b.Init()
	return b
}

// InitWithTabspaces initializes this Buffer with the given tabspaces.
func (b *Buffer) InitWithTabspaces(tabspaces int) {
	cells := new(rawCells)
	cells.init(tabspaces)
	b.initWithCells(cells)
}

// InitPerformance initializes this Buffer without Undo, Redo,
// SubscribeUsage, UnsubscribeUsage, Subscribe or Unsubscribe functionality.
// Calling any of these methods will cause the calling goroutine to panic.
func (b *Buffer) InitPerformance(tabspaces int, rowCapacity, columnCapacity int) {
	cells := new(rawCells)
	cells.initWithCap(tabspaces, rowCapacity, columnCapacity)
	b.initPerformanceWithCells(cells)
}

// ResetCapacity resets the capacity given to new rows.
func (b *Buffer) ResetCapacity(capacity int) {
	b.cells.columnCap = int(math.Max(float64(defColumnCap), float64(capacity)))
}

// Init initializes this Buffer with the default configuration.
func (b *Buffer) Init() {
	b.InitWithTabspaces(DefaultTabspaces)
}

// InsertRowAt is equivalent to calling InsertRowAtContext
// with context.Background.
func (b *Buffer) InsertRowAt(y int) {
	b.InsertRowAtContext(context.Background(), y)
}

// InsertRowAtContext inserts a new row at given position. If pos is out of bounds,
// this method does not panic; instead, it will fill in the necessary
// rows such that the new row is the last row in the buffer.
func (b *Buffer) InsertRowAtContext(ctx context.Context, y int) {
	at := term.Coordinates{Y: y}
	b.editor.Edit(ctx, at, at, "\n")
}

// Insert is equivalent to calling InsertContext with context.Background.
func (b *Buffer) Insert(pos term.Coordinates, r rune) (next term.Coordinates) {
	return b.InsertContext(context.Background(), pos, r)
}

// InsertContext inserts a rune in the given position and shift the cells to the right
func (b *Buffer) InsertContext(ctx context.Context, pos term.Coordinates, r rune) (next term.Coordinates) {
	_, next, _ = b.editor.Edit(ctx, pos, pos, string(r))
	return
}

// InsertWithAttr writes str and gives it attr term.Attributes.
func (b *Buffer) InsertWithAttr(
	pos term.Coordinates, r rune, attr term.Attributes,
) (next term.Coordinates) {
	next = b.Insert(pos, r)

	switch r {
	case '\n', '\t':
		return
	}
	cells := b.RawCells()
	cells[pos.Y][pos.X].Bg = attr.Bg
	cells[pos.Y][pos.X].Fg = attr.Fg
	cells[pos.Y][pos.X].Attrs = attr.Attrs

	return
}

// InsertStringWithAttr inserts str with the given attr as the background
// and foreground cell term.Attributes.
func (b *Buffer) InsertStringWithAttr(
	at term.Coordinates, str string, attr term.Attributes,
) (from, until term.Coordinates) {
	from, until = b.InsertString(at, str)
	cells, _, _ := b.Select(from, until)
	for y, row := range cells {
		for x := range row {
			cells[y][x].Bg = attr.Bg
			cells[y][x].Fg = attr.Fg
			cells[y][x].Attrs = attr.Attrs
		}
	}
	return
}

// DeleteRow is equivalent to calling DeleteRowContext
// with context.Background.
func (b *Buffer) DeleteRow(y int) (ok bool) {
	return b.DeleteRowContext(context.Background(), y)
}

// DeleteRowContext deletes the row at term.Coordinates.Y
func (b *Buffer) DeleteRowContext(ctx context.Context, y int) (ok bool) {
	if ok = y < b.view.Rows(); !ok {
		return
	}
	var from, to term.Coordinates
	if y == 0 {
		from = term.Coordinates{Y: y}
		to = term.Coordinates{Y: y + 1}
	} else {
		from = term.Coordinates{Y: y - 1, X: b.view.Columns(y - 1)}
		to = term.Coordinates{Y: y, X: b.view.Columns(y)}
	}
	_, _, old := b.editor.Edit(ctx, from, to, "")
	ok = old != ""
	return
}

// TruncateRowFrom is equivalent to calling TruncateRowFromContext
// with context.Background.
func (b *Buffer) TruncateRowFrom(from term.Coordinates) (ok bool) {
	return b.TruncateRowFromContext(context.Background(), from)
}

// TruncateRowFromContext truncates the row at term.Coordinates.Y starting
// from term.Coordinates.X
func (b *Buffer) TruncateRowFromContext(
	ctx context.Context, from term.Coordinates,
) (ok bool) {
	to := term.Coordinates{Y: from.Y}
	from, to, ok = fromToInBounds(b.view, from, to)
	if !ok {
		return
	}
	cols := b.view.Columns(to.Y)
	if cols == 0 {
		ok = false
		return
	}
	to.X = cols
	b.editor.Edit(ctx, from, to, "")
	return
}

// TruncateFrom is equivalent to calling TruncateFromContext with
// context.Background.
func (b *Buffer) TruncateFrom(from term.Coordinates) (ok bool) {
	return b.TruncateFromContext(context.Background(), from)
}

// TruncateFromContext truncates from the given position to the end of the buffer.
func (b *Buffer) TruncateFromContext(
	ctx context.Context, from term.Coordinates,
) (ok bool) {
	to := term.Coordinates{Y: b.view.Rows()}
	from, to, ok = fromToInBounds(b.view, from, to)
	if !ok {
		return
	}
	b.editor.Edit(ctx, from, to, "")
	return
}

// Replace is equivalent to calling ReplaceContext with
// context.Background.
func (b *Buffer) Replace(str string) {
	b.ReplaceContext(context.Background(), str)
}

// ReplaceContext replaces the content of the buffer with str.
func (b *Buffer) ReplaceContext(ctx context.Context, str string) {
	from := term.Coordinates{}
	to := term.Coordinates{Y: b.view.Rows()}
	b.editor.Edit(ctx, from, to, str)
}

// ConflateRow is equivalent to calling ConflateRowContext
// with context.Background.
func (b *Buffer) ConflateRow(y int) (ok bool) {
	return b.ConflateRowContext(context.Background(), y)
}

// ConflateRowContext will conflate row at index i with the next row
func (b *Buffer) ConflateRowContext(
	ctx context.Context, y int,
) (ok bool) {
	from := term.Coordinates{Y: y}
	to := term.Coordinates{Y: y + 1}
	from, to, ok = fromToInBounds(b.view, from, to)
	if !ok {
		return
	}
	from.X = b.view.Columns(from.Y)
	b.editor.Edit(ctx, from, to, "")
	return
}

// DeleteCell is equivalent to calling DeleteCellContext
// with context.Background.
func (b *Buffer) DeleteCell(pos term.Coordinates) (term.Coordinates, rune, bool) {
	return b.DeleteCellContext(context.Background(), pos)
}

// DeleteCellContext removes the cell at the given position.
// It returns the position at which the current cell (width padding) started,
// if the width was > 1.
func (b *Buffer) DeleteCellContext(
	ctx context.Context, pos term.Coordinates,
) (term.Coordinates, rune, bool) {
	from := pos
	to := term.Coordinates{X: from.X + 1, Y: from.Y}
	from, to, ok := fromToInBounds(b.view, from, to)
	if !ok {
		return term.Coordinates{}, 0, false
	}

	start, _, str := b.editor.Edit(ctx, from, to, "")
	if str == "" {
		return term.Coordinates{}, 0, false
	}

	return start, []rune(str)[0], true
}

// Rows returns the number of rows in the Buffer.
func (b *Buffer) Rows() int {
	return b.view.Rows()
}

// Columns returns the number of cells of row at index y.
func (b *Buffer) Columns(y int) int {
	return b.view.Columns(y)
}

// MaxColumns returns the max number of columns.
func (b *Buffer) MaxColumns() (max int) {
	for i := 0; i < b.Rows(); i++ {
		if col := b.Columns(i); col > max {
			max = col
		}
	}
	return
}

// Cell returns the cell and true or a zero-valued cell and false if there is no
// cell at position.
func (b *Buffer) Cell(pos term.Coordinates) (term.Cell, bool) {
	return b.view.Cell(pos)
}

// RawCells gives clients access to the underlying cell matrix.
func (b *Buffer) RawCells() [][]term.Cell {
	return b.view.RawCells()
}

// InsertString is equivalent to calling InsertStringContext
// with context.Background.
func (b *Buffer) InsertString(at term.Coordinates, str string) (
	from, until term.Coordinates,
) {
	return b.InsertStringContext(context.Background(), at, str)
}

// InsertStringContext inserts string in the given position and shifts the remaining cells.
// insert never fails: if at is out-of-bounds, this method fills in the rows
// and/or columns of cells with blank spaces.
// It returns the start of the insert 'from', including the filled-in blank spaces
// and where the next logical Insert should go 'until'.
func (b *Buffer) InsertStringContext(
	ctx context.Context, at term.Coordinates, str string,
) (from, until term.Coordinates) {
	from, until, _ = b.editor.Edit(ctx, at, at, str)
	return
}

// Delete is equivalent to calling DeleteContext with
// context.Background.
func (b *Buffer) Delete(from, to term.Coordinates) (start term.Coordinates, str string) {
	return b.DeleteContext(context.Background(), from, to)
}

// DeleteContext removes cells in left-inclusive right-exclusive range
// and returns the corresponding string representation of the cells removed,
// along with the true start of the range, which accounts for padding.
func (b *Buffer) DeleteContext(ctx context.Context, from, to term.Coordinates) (
	start term.Coordinates, str string,
) {
	start, _, str = b.safew.Edit(ctx, from, to, "")
	return
}

// Edit removes cells in left-inclusive right-exclusive range (start, end) and
// inserts s at the start of the range. It returns the corresponding string
// representation of the content removed and the true from, to range, which
// accounts for possible padding added or removed.
//
// As opposed to cell.Editor.Edit, this method does not panic if range
// is out of bounds. Instead, it trims the coordinates to be in-bounds or
// simply does nothing and returned str is empty.
func (b *Buffer) Edit(ctx context.Context, start, end term.Coordinates, s string) (
	from, to term.Coordinates, old string,
) {
	return b.safew.Edit(ctx, start, end, s)
}

// DeleteLine is equivalent to calling DeleteLineContext
// with context.Background.
func (b *Buffer) DeleteLine(from, to term.Coordinates) (
	start term.Coordinates, str string,
) {
	return b.DeleteLineContext(context.Background(), from, to)
}

// DeleteLineContext deletes the lines starting at from, between from, to and including end.
// See Delete for more information about the return values.
func (b *Buffer) DeleteLineContext(
	ctx context.Context, from, to term.Coordinates,
) (start term.Coordinates, str string) {
	from, to = term.CoordinatesSort(from, to)
	from, to, ok := fromToInBounds(b.view, from, to)
	if !ok {
		return
	}
	from.X, to.X = 0, 0
	to.Y++
	start, _, str = b.editor.Edit(ctx, from, to, "")
	return
}

// DeleteBlock is equivalent to calling DeleteBlockContext
// with context.Background.
func (b *Buffer) DeleteBlock(from, to term.Coordinates) (
	start term.Coordinates, str string,
) {
	return b.DeleteBlockContext(context.Background(), from, to)
}

// DeleteBlockContext deletes the blocks of cells between from, to. See SelectBlock for more
// information about how DeleteBlock selects the cells to delete.
// See Delete for more information about the return values.
func (b *Buffer) DeleteBlockContext(
	ctx context.Context, from, to term.Coordinates,
) (start term.Coordinates, str string) {
	var builder strings.Builder
	b.selector.iterateBlocks(from, to,
		func(i int, from, to term.Coordinates, cells []term.Cell) {
			if i != 0 {
				builder.WriteRune('\n')
			}
			columns := b.Columns(from.Y)
			if columns == 0 || from.X > columns {
				if i == 0 {
					start = term.Coordinates{Y: from.Y}
				}
				return
			}

			if to.X > columns {
				to.X = columns
			}
			blockStart, _, str := b.safew.Edit(ctx, from, to, "")

			if i == 0 {
				start = blockStart
			}
			builder.WriteString(str)
		})

	str = builder.String()
	return
}

// Reset resets the contents of this Buffer.
func (b *Buffer) Reset() {
	// make sure that reset is propagated to subscribers.
	b.TruncateFrom(term.Coordinates{})
	if b.undoer != nil {
		b.undoer.reset()
	}
}

func (b *Buffer) Version() int {
	return b.undoer.version
}

// ReadFrom reads data from r until EOF and appends it to the buffer, growing
// the buffer as needed. The return value n is the number of bytes read. Any
// error except io.EOF encountered during the read is also returned.
func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	return b.cells.ReadFrom(r)
}

// io.Editor
func (b *Buffer) Write(p []byte) (int, error) {
	nextWrite := nextWrite(b.view)
	b.editor.Edit(context.Background(), nextWrite, nextWrite, string(p))
	return len(p), nil
}

// WriteString is equivalent to WriteStringContext with context.Background.
func (b *Buffer) WriteString(p string) {
	b.WriteStringContext(context.Background(), p)
}

// WriteStringContext writes the given string at the end of the buffer
func (b *Buffer) WriteStringContext(ctx context.Context, p string) {
	nextWrite := nextWrite(b.view)
	b.editor.Edit(ctx, nextWrite, nextWrite, p)
}

// WriteStringWithAttr inserts str with the given attr as the background
// and foreground cell term.Attributes.
func (b *Buffer) WriteStringWithAttr(str string, attr term.Attributes) {
	at := nextWrite(b.view)
	b.InsertStringWithAttr(at, str, attr)
}

// Undo reverses the last update to the Buffer.
// Redo can be used to reverse Undo.
func (b *Buffer) Undo() (bool, term.Coordinates) {
	return b.undoer.undo()
}

// Redo reverses the previously reversed update to the Buffer.
func (b *Buffer) Redo() (bool, term.Coordinates) {
	return b.undoer.redo()
}

// Select returns the cells inside the given coordinates or nil if coordinates
// are out of bounds.
func (b *Buffer) Select(from term.Coordinates, to term.Coordinates) (
	[][]term.Cell, []Selection, bool,
) {
	from, to, ok := fromToInBounds(b.view, from, to)
	if !ok {
		return nil, nil, false
	}
	cell, sels := b.selector.selectCells(from, to)
	return cell, sels, true
}

// SelectLine returns the lines inside the given coordinates or nil if
// coordinates are out of bounds.
func (b *Buffer) SelectLine(from term.Coordinates, to term.Coordinates) (
	[][]term.Cell, []Selection, bool,
) {
	from, to, ok := fromToInBounds(b.view, from, to)
	if !ok {
		return nil, nil, false
	}
	cell, sels := b.selector.selectLine(from, to)
	return cell, sels, true
}

// SelectBlock returns the block of cells inside the given coordinates or nil if
// coordinates are out of bounds.
func (b *Buffer) SelectBlock(from term.Coordinates, to term.Coordinates) (
	[][]term.Cell, []Selection, bool,
) {
	from, to, ok := fromToInBounds(b.view, from, to)
	if !ok {
		return nil, nil, false
	}
	cell, sels := b.selector.selectBlock(from, to)
	return cell, sels, true
}

func (b *Buffer) String() string {
	return b.view.String()
}

// Tabspaces returns the number of tabspaces uses to initialized this Buffer.
func (b *Buffer) Tabspaces() int {
	return b.cells.tabspaces
}

// ShiftRowRight shifts row one tab to the right. It returns
// the number of cells that the line was shifted.
func (b *Buffer) ShiftRowRight(row int) int {
	b.Insert(term.Coordinates{Y: row}, '\t')
	return b.Tabspaces()
}

// ShiftRowLeft shifts row one tab to the left. It returns
// the number of cells that the line was shifted.
func (b *Buffer) ShiftRowLeft(row int) (chars int) {
	from, to := term.Coordinates{Y: row}, term.Coordinates{Y: row, X: 1}
	from, to, ok := fromToInBounds(b.view, from, to)
	if !ok {
		return
	}
	origLen := b.Columns(row)
	for chars < b.Tabspaces() {
		c, ok := b.view.Cell(from)
		if !ok {
			return
		}
		switch c.Ch {
		case '\t', '\x00', ' ':
			b.Delete(from, to)
			chars = origLen - b.Columns(row)
		default:
			return
		}
	}
	return
}

// Subscribe subscribes s to all updates to the underlying buffer.
func (b *Buffer) Subscribe(s Subscriber) {
	b.rootPub.Subscribe(s)
}

// Unsubscribe unsubscribes s from updates.
func (b *Buffer) Unsubscribe(s Subscriber) {
	b.rootPub.Unsubscribe(s)
}

// SubscribeUsage subscribes s to direct update calls to this buffer. Unlike Subscribe,
// this method does not capture indirect updates to Buffer, for instance via Undo.
func (b *Buffer) SubscribeUsage(s Subscriber) {
	b.usagePub.Subscribe(s)
}

// UnsubscribeUsage reverses SubscribeUsage.
func (b *Buffer) UnsubscribeUsage(s Subscriber) {
	b.usagePub.Unsubscribe(s)
}

// Height returns the required height if this Buffer was to be drawn on a term.Editor.
func (b *Buffer) Height() int {
	return b.Rows()
}

// Width returns the required width if this Buffer was to be drawn on a term.Editor.
func (b *Buffer) Width() int {
	var ret int
	for _, row := range b.RawCells() {
		if len(row) > ret {
			ret = len(row)
		}
	}
	return ret
}

// View returns this Buffer as a cell.View.
func (b *Buffer) View() View {
	return b.view
}

// Editor returns a cell.Editor that doesn't panic on out-of-bounds calls.
func (b *Buffer) Editor() Editor {
	return b.safew
}

// Size returns the total size in cells of this buffer.
func (b *Buffer) Size() (ret int) {
	for _, row := range b.RawCells() {
		ret += len(row)
	}
	return
}

// WithView installs a new view and returns this Buffer's previous view.
// This should only be utilized for advanced use cases.
func (b *Buffer) WithView(v View) (ret View) {
	ret = b.view
	b.setView(v)
	return
}

// WithEditor installs a new Editor and returns this Buffer's previous Editor.
// This should only be utilized for advanced use cases.
func (b *Buffer) WithEditor(e Editor) (ret Editor) {
	ret = b.editor
	b.setEditor(e)
	return
}

func (b *Buffer) setView(view View) {
	b.view = view
	b.safew.view = b.view
	b.selector.view = b.view
}

func (b *Buffer) setEditor(ed Editor) {
	b.editor = ed
	b.safew.editor = ed
}

func (b *Buffer) initWithCells(cells *rawCells) {
	b.cells = cells
	// setup the root publisher as the deepest Editor
	b.rootPub = newPublisher(b.cells)
	b.undoer = newUndoer(b.rootPub)
	// setup the usage publisher at the shallowest Editor
	b.usagePub = newPublisher(b.undoer)

	b.setEditor(b.usagePub)
	b.setView(b.cells)
}

func (b *Buffer) initPerformanceWithCells(cells *rawCells) {
	b.cells = cells

	b.setEditor(b.cells)
	b.setView(b.cells)
}
