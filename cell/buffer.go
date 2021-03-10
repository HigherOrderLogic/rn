package cell

import (
	"fmt"
	"io"

	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

// A Buffer offers a high level API to manipulate a matrix of term.Cell.
// Note that this Buffer assumes to be a UNIX file buffer, so content written to it
// is assumed to end in EOL.
//
// A text file, under UNIX-like systems, consists of a series of lines, each of which
// ends with a newline character (\n). A file that is not empty and does not
// end with a newline is therefore not a text file.
//
// The write-end of this UNIX behaviour is implemented by editor.FileBuffer.
type Buffer struct {
	cells      *rawCells
	undoer     *undoer
	selector   selector
	unixReader *unixFileReader
	rootPub    *syncPublisher
	usagePub   *syncPublisher

	// effective Reader and Writer
	reader Reader
	writer Writer
}

// NewBuffer allocates storage for a new Buffer and initializes it.
func NewBuffer() (b *Buffer) {
	b = new(Buffer)
	b.Init()
	return b
}

func (b *Buffer) initWithCells(c *rawCells, logger *log.Logger, unixFile bool) {
	b.cells = c
	b.writer = b.cells
	b.reader = b.cells

	if unixFile {
		b.unixReader = newUnixFileReader(b.reader)
		b.reader = b.unixReader
	}

	if logger != nil {
		cellLogger := newLogger(b.reader, b.cells, logger)
		b.reader = cellLogger
		b.writer = cellLogger
	}

	// setup the root publisher as the deepest Writer
	b.rootPub = newPublisher(b.writer)
	b.undoer = newUndoer(b.rootPub)
	b.writer = b.undoer
	// setup the usage publisher at the shalowest Writer
	b.usagePub = newPublisher(b.writer)
	b.writer = b.usagePub

	b.selector.reader = b.reader
}

// InitWithTabspaces initializes this Buffer with
func (b *Buffer) InitWithTabspaces(tabspaces int) {
	cells := new(rawCells)
	cells.init(tabspaces)
	b.initWithCells(cells, nil, true)
}

// Init initializes this Buffer with the default configuration.
func (b *Buffer) Init() {
	b.InitWithTabspaces(defTabSpaces)
}

// WithLogger adds a cell logger which intercepts and logs all
// the calls to the underlying Writer/Reader.
func (b *Buffer) WithLogger(logger *log.Logger) {
	b.initWithCells(b.cells, logger, b.unixReader != nil)
}

// InsertRowAt inserts a new row at given position. If pos is out of bounds,
// this method does not panic; instead, it will fill in the necessary
// rows such that the new row is the last row in the buffer.
func (b *Buffer) InsertRowAt(y int) {
	b.writer.Insert(term.Coordinates{Y: y}, "\n")
}

// Insert inserts a rune in the given position and shift the cells to the right
func (b *Buffer) Insert(pos term.Coordinates, r rune) (next term.Coordinates) {
	_, next = b.writer.Insert(pos, string(r))

	if r == '\n' {
		next.Y++
		next.X = 0
		return
	}

	next.X++

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

	return
}

// InsertStringWithAttr inserts str with the given attr as the background
// and foreground cell term.Attributes.
func (b *Buffer) InsertStringWithAttr(
	at term.Coordinates, str string, attr term.Attributes,
) (from, until term.Coordinates) {
	from, until = b.InsertString(at, str)
	cells := b.Select(from, until)
	for y, row := range cells {
		for x := range row {
			cells[y][x].Bg = attr.Bg
			cells[y][x].Fg = attr.Fg
		}
	}
	return
}

// DeleteRow truncates the row at term.Coordinates.Y
func (b *Buffer) DeleteRow(y int) (ok bool) {
	if ok = y < b.reader.Rows(); !ok {
		return
	}
	from := term.Coordinates{Y: y, X: 0}
	to := term.Coordinates{Y: y, X: b.reader.Columns(y)}
	b.writer.Delete(from, to)
	return
}

func (b *Buffer) inBounds(pos term.Coordinates) (ok bool) {
	if pos.Y < 0 || pos.Y >= b.reader.Rows() ||
		pos.X < 0 || pos.X > b.reader.Columns(pos.Y) {
		return
	}
	ok = true
	return
}

// TruncateRowFrom truncates the row at term.Coordinates.Y starting from term.Coordinates.X
func (b *Buffer) TruncateRowFrom(pos term.Coordinates) (ok bool) {
	if ok = b.inBounds(pos); !ok {
		return
	}
	cols := b.reader.Columns(pos.Y)
	if cols == 0 {
		ok = false
		return
	}
	b.writer.Delete(pos, term.Coordinates{Y: pos.Y, X: cols - 1})
	return
}

// TruncateFrom truncates from the given position to the end of the buffer.
func (b *Buffer) TruncateFrom(pos term.Coordinates) (ok bool) {
	if ok = b.inBounds(pos); !ok {
		return
	}
	y := b.reader.Rows() - 1
	to := term.Coordinates{X: b.reader.Columns(y), Y: y}
	b.writer.Delete(pos, to)
	return
}

// ConflateRow will conflate row at index i with the next row
func (b *Buffer) ConflateRow(y int) (ok bool) {
	pos := term.Coordinates{Y: y}
	if ok = b.inBounds(pos); !ok {
		return
	}
	pos.X = b.reader.Columns(y)
	b.writer.Delete(pos, pos)
	return
}

// DeleteCell truncates the cell at the given position.
// It returns the position at which the current cell (width padding) started,
// if the width was > 1.
func (b *Buffer) DeleteCell(pos term.Coordinates) (term.Coordinates, rune, bool) {
	if ok := b.inBounds(pos); !ok {
		return term.Coordinates{}, 0, false
	}

	start, _, str := b.writer.Delete(pos, pos)
	return start, []rune(str)[0], true
}

// Rows returns the number of rows in the Buffer.
func (b *Buffer) Rows() int {
	return b.reader.Rows()
}

// Columns returns the number of cells of row at index y.
func (b *Buffer) Columns(y int) int {
	return b.reader.Columns(y)
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
	return b.reader.Cell(pos)
}

// RawCells gives clients access to the underlying cell matrix.
func (b *Buffer) RawCells() [][]term.Cell {
	return b.reader.RawCells()
}

// InsertString inserts string in the given position and shifts the remaining cells.
// insert never fails: if at is out-of-bounds, this method fills in the rows
// and/or columns of cells with blank spaces.
// It returns the start of the insert 'from', including the filled-in blank spaces
// and where the next logical Insert should go 'until'.
func (b *Buffer) InsertString(at term.Coordinates, str string) (from, until term.Coordinates) {
	from, until = b.writer.Insert(at, str)

	if str == "" {
		return
	}
	runes := []rune(str)
	if runes[len(runes)-1] == '\n' {
		until.Y++
		until.X = 0
		return
	}
	until.X++
	return
}

// Delete removes cells in left-inclusive, right-inclusive range
// and returns the corresponding string representation of the cells removed,
// along with the true start and end of the range, in case some cells groups
// (cell with padding) were deleted.
// Note that if to.X == b.Columns(to.Y), the newline at the end of the row is deleted,
// and so row is conflated with next row.
func (b *Buffer) Delete(from, to term.Coordinates) (start, end term.Coordinates, str string) {
	if !b.inBounds(from) || !b.inBounds(to) {
		panic(fmt.Sprintf("out of bounds: from=%+v, to=%+v", from, to))
	}
	return b.writer.Delete(from, to)
}

// DeleteLine deletes the lines starting at from, between from, to and including end.
// See Delete for more information about the return values.
func (b *Buffer) DeleteLine(from, to term.Coordinates) (
	start, end term.Coordinates, str string,
) {
	from, to = SortFromTo(from, to)
	from.X, to.X = 0, b.Columns(to.Y)
	if to.X < 0 {
		to.X = 0
	}
	return b.Delete(from, to)
}

// DeleteBlock deletes the blocks of cells between from, to. See SelectBlock for more
// information about how DeleteBlock selects the cells to delete.
// See Delete for more information about the return values.
func (b *Buffer) DeleteBlock(from, to term.Coordinates) (
	start, end term.Coordinates,
) {
	b.selector.iterateBlocks(from, to,
		func(i int, from, to term.Coordinates, cells []term.Cell) {
			columns := b.Columns(from.Y)
			if columns == 0 {
				if i == 0 {
					start = term.Coordinates{Y: from.Y}
				}
				end = term.Coordinates{Y: from.Y}
				return
			}

			if to.X > columns-1 {
				to.X = columns - 1
			}
			if from.X > columns-1 {
				return
			}
			blockStart, blockEnd, _ := b.Delete(from, to)

			if i == 0 {
				start = blockStart
			}
			end = blockEnd
		})

	return
}

// Reset resets the contents of this Buffer.
func (b *Buffer) Reset() {
	b.undoer.reset()
	b.cells.reset()
}

// ReadFrom reads data from r until EOF and appends it to the buffer, growing
// the buffer as needed. The return value n is the number of bytes read. Any
// error except io.EOF encountered during the read is also returned.
func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	return b.cells.ReadFrom(r)
}

// io.Writer
func (b *Buffer) Write(p []byte) (int, error) {
	nextWrite := b.cells.nextWrite()
	b.writer.Insert(nextWrite, string(p))
	return len(p), nil
}

// WriteString writes the given string at the end of the buffer
func (b *Buffer) WriteString(p string) {
	nextWrite := b.cells.nextWrite()
	b.writer.Insert(nextWrite, p)
}

// WriteStringWithAttr inserts str with the given attr as the background
// and foreground cell term.Attributes.
func (b *Buffer) WriteStringWithAttr(str string, attr term.Attributes) {
	at := b.cells.nextWrite()
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
func (b *Buffer) Select(from term.Coordinates, to term.Coordinates) [][]term.Cell {
	return b.selector.selectCells(from, to)
}

// SelectLine returns the lines inside the given coordinates or nil if
// coordinates are out of bounds.
func (b *Buffer) SelectLine(from term.Coordinates, to term.Coordinates) [][]term.Cell {
	return b.selector.selectLine(from, to)
}

// SelectBlock returns the block of cells inside the given coordinates or nil if
// coordinates are out of bounds.
func (b *Buffer) SelectBlock(from term.Coordinates, to term.Coordinates) [][]term.Cell {
	return b.selector.selectBlock(from, to)
}

func (b *Buffer) String() string {
	return b.reader.String()
}

// EndsWithEOL returns true if the underlying text ends with an EOL character.
func (b *Buffer) EndsWithEOL() bool {
	return b.unixReader.endswithEOL()
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
	for ; chars < b.Tabspaces(); chars++ {
		c, ok := b.reader.Cell(term.Coordinates{Y: row, X: 0})
		if !ok {
			return
		}

		switch c.Ch {
		case '\t', '\x00':
			chars = b.Tabspaces() - 1
			fallthrough
		case ' ':
			b.DeleteCell(term.Coordinates{Y: row})
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

// Height returns the required height if this Buffer was to be drawn on a term.Writer.
func (b *Buffer) Height() int {
	return b.Rows()
}

// Width returns the required width if this Buffer was to be drawn on a term.Writer.
func (b *Buffer) Width() int {
	var ret int
	for _, row := range b.RawCells() {
		if len(row) > ret {
			ret = len(row)
		}
	}
	return ret
}

// Reader returns this Buffer as a cell.Reader.
func (b *Buffer) Reader() Reader {
	return b.reader
}

// Writer returns this Buffer as a cell.Writer.
func (b *Buffer) Writer() Writer {
	return b.writer
}

// Size returns the total size in cells of this buffer.
func (b *Buffer) Size() (ret int) {
	for _, row := range b.RawCells() {
		ret += len(row)
	}
	return
}
