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

package cell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	"unstable.build/go-tui/cell/graphemecluster"
	"unstable.build/go-tui/term"
)

const (
	// DefaultTabspaces is the default number of tabspaces used by this package.
	DefaultTabspaces int = 4
	defColumnCap     int = 64
	defRowCap        int = 64
)

var (
	nullCluster = [1]rune{'\x00'}
	tabCluster  = [1]rune{'\t'}
)

// rawCells is a matrix of term.Cell.
type rawCells struct {
	columnCap  int
	rowCap     int
	cells      [][]term.Cell
	tabspaces  int
	fillInChar rune
	zwj        bool
	zwjPos     term.Coordinates
}

// init initializes this rawCells with the given tabspaces config and resets its contents.
func (c *rawCells) init(tabspaces int) {
	c.tabspaces = tabspaces
	c.fillInChar = ' '
	c.reset()
}

func (c *rawCells) initWithCap(tabspaces, rowCap, columnCap int, fillInChar rune) {
	c.tabspaces = tabspaces
	c.fillInChar = fillInChar
	c.resetWithCap(rowCap, columnCap)
}

func (c *rawCells) reset() {
	c.resetWithCap(defRowCap, defColumnCap)
}

func (c *rawCells) resetWithCap(rowCap, columnCap int) {
	c.columnCap = int(math.Max(float64(columnCap), float64(defColumnCap)))
	c.rowCap = int(math.Max(float64(rowCap), float64(defRowCap)))
	c.cells = make([][]term.Cell, 1, c.rowCap)
	c.cells[0] = makeNewRow(0, c.columnCap)
	c.zwj = false
	c.zwjPos = term.Coordinates{}
}

func assertValidCoords(pos term.Coordinates) {
	if pos.X < 0 || pos.Y < 0 {
		panic(fmt.Sprintf("invalid coordinates: %+v", pos))
	}
}

func makeNewRow(length, capacity int) (row []term.Cell) {
	capacity = int(math.Max(float64(length), float64(capacity)))
	row = make([]term.Cell, length, capacity)
	return
}

func (c *rawCells) insertNewRow(pos term.Coordinates) {
	assertValidCoords(pos)
	sourceRow := c.cells[pos.Y]
	targetY := pos.Y + 1

	// make enough space for one more row
	c.cells = append(c.cells, nil)
	copy(c.cells[targetY:], c.cells[pos.Y:])

	// if not last position, copy the rest of cells to the next row
	if pos.X < len(sourceRow) {
		c.cells[pos.Y] = c.cells[pos.Y][:pos.X]
		length := len(sourceRow[pos.X:])
		c.cells[targetY] = makeNewRow(length, c.columnCap)
		copy(c.cells[targetY], sourceRow[pos.X:])
	} else {
		c.cells[targetY] = makeNewRow(0, c.columnCap)
	}
}

func (c *rawCells) doInsertAt(pos term.Coordinates, r []rune, width int) {
	// make sure we have enough capacity
	c.cells[pos.Y] = append(c.cells[pos.Y], term.Cell{})
	copy(c.cells[pos.Y][pos.X+1:], c.cells[pos.Y][pos.X:])
	var combining []rune
	if len(r) > 1 {
		combining = r[1:]
	}
	cell := term.Cell{Ch: r[0], Combining: combining, Width: width}
	c.cells[pos.Y][pos.X] = cell
}

func (c *rawCells) insertAt(pos term.Coordinates, r []rune, width int) (
	next term.Coordinates,
) {
	switch r[0] {
	// zero-width joiner, at position 0, indicates that previous cell is not complete
	// This mechanism is needed because input event processes one rune at a time
	// This assumes that a zwj and the runes of the grapheme cluster it belongs to
	// are inserted sequentially
	case '\u200d':
		next = term.Coordinates{X: pos.X + 1, Y: pos.Y}
		// remove any previous null cells added to ensure that ith cell matches
		// ith user visual cell, for wide characters
		for pos.Y < len(c.cells) && pos.X > 0 &&
			pos.X-1 < len(c.cells[pos.Y]) &&
			c.cells[pos.Y][pos.X-1].Ch == '\x00' {
			pos.X--
			next.X--
			var nop strings.Builder
			c.deleteRowRange(&nop, pos.Y, pos.X, next.X)
		}
		c.zwj = true
		c.doInsertAt(pos, r, width)
		c.zwjPos = next
		return
	case '\n':
		c.insertNewRow(pos)
		next = term.Coordinates{X: 0, Y: pos.Y + 1}
	case '\t':
		if c.tabspaces > 0 {
			c.insertTabSpaces(pos)
			next = term.Coordinates{X: pos.X + c.tabspaces, Y: pos.Y}
			break
		}
		fallthrough
	default:
		pos := pos // need unmodified pos below
		c.doInsertAt(pos, r, width)
		for i := 1; i < width; i++ {
			pos.X++
			c.doInsertAt(pos, nullCluster[:], 0)
		}
		next = term.Coordinates{X: pos.X + 1, Y: pos.Y}
	}

	if c.zwj {
		if c.zwjPos == pos {
			str := c.String()
			c.reset()
			_, _ = c.ReadFrom(strings.NewReader(str))
			next = term.Coordinates{X: pos.X, Y: pos.Y}
		}
		c.zwj = false
		c.zwjPos = term.Coordinates{}
	}

	return
}

func (c *rawCells) insertTabSpaces(pos term.Coordinates) {
	n := term.Coordinates{X: pos.X, Y: pos.Y}
	for i := 1; i < c.tabspaces; i++ {
		n = c.insertAt(n, nullCluster[:], 0)
	}
	c.doInsertAt(n, tabCluster[:], 0)
}

func (c *rawCells) fillInRows(y int) (n int) {
	for y >= len(c.cells) {
		n++
		row := makeNewRow(0, c.columnCap)
		c.cells = append(c.cells, row)
	}
	return
}

func (c *rawCells) fillInColumns(pos term.Coordinates) (n int) {
	for pos.X > len(c.cells[pos.Y]) {
		c.cells[pos.Y] = append(c.cells[pos.Y], term.Cell{Ch: c.fillInChar})
		n++
	}
	return
}

func (c *rawCells) fillInCoords(pos term.Coordinates) (
	from, to term.Coordinates, rowsFilled int,
) {
	assertValidCoords(pos)
	from = pos
	if rowsFilled = c.fillInRows(pos.Y); rowsFilled != 0 {
		from.Y -= rowsFilled
		// if rawCells was un-initialized or for some
		// reason base row was removed i.e. a truncate op
		if from.Y < 0 {
			from.Y = 0
		}
		from.X = c.Columns(from.Y)
		c.fillInColumns(pos)
	} else {
		from.X -= c.fillInColumns(pos)
	}
	to = pos

	return
}

func (c *rawCells) insert(at term.Coordinates, str string) (
	from, to term.Coordinates,
) {
	var rowsFilled int
	from, to, rowsFilled = c.fillInCoords(at)
	// fillInCoords fills in with newlines up to Y
	if rowsFilled != 0 {
		var i int
		for ; i < len(str) && str[i] == '\n'; i++ {
		}
		str = str[i:]
	}
	// avoid breaking padding blocks in half
	if from == at {
		from, _ = c.skipPadding(at, at)
		to = from
	}
	next := to
	state := -1
	var cluster string
	var width int
	for len(str) > 0 {
		cluster, str, width, state = graphemecluster.StepString(str, state)
		to = next
		next = c.insertAt(next, []rune(cluster), width)
		if padding := next.X - to.X - 1; padding > 0 {
			to.X += padding
		}
	}
	to = next
	return
}

func copyToBuilder(builder *strings.Builder, cells [][]term.Cell) {
	for i, r := range cells {
		if i != 0 {
			builder.WriteByte('\n')
		}
		copyRowToBuilder(builder, r)
	}
}

func copyRowToBuilder(builder *strings.Builder, cells []term.Cell) {
	for _, c := range cells {
		if c.Ch != '\x00' {
			builder.WriteRune(c.Ch)
			for _, comb := range c.Combining {
				builder.WriteRune(comb)
			}
		}
	}
}

func (c *rawCells) conflate(row int) {
	// copy cells from next row into current row
	rlen := len(c.cells[row+1])
	if rlen != 0 {
		origLen := len(c.cells[row])
		c.cells[row] = append(c.cells[row], make([]term.Cell, rlen)...)
		copy(c.cells[row][origLen:], c.cells[row+1][:])
	}

	// copy all rows into row we just moved up and trim last row
	copy(c.cells[row+1:], c.cells[row+2:])
	c.cells = c.cells[:len(c.cells)-1]
}

func (c *rawCells) deleteRowRange(
	builder *strings.Builder, row, fromX, toX int,
) {
	copyRowToBuilder(builder, c.cells[row][fromX:toX])
	diff := toX - fromX
	copy(c.cells[row][fromX:], c.cells[row][toX:])
	c.cells[row] = c.cells[row][:len(c.cells[row])-diff]
}

func (c *rawCells) skipPadding(start, end term.Coordinates) (
	from term.Coordinates, to term.Coordinates,
) {
	from, to = start, end
	// to is right exclusive
	if to.X > 0 {
		// to.Y == len(c.cells) should never occur here since the only
		// correct way for clients to pass that is if to.X == 0,
		// which we are checking above
		endLastIdx := len(c.cells[to.Y])
		to.X--
		if to.X < endLastIdx {
			if width := c.cells[to.Y][to.X].Width; width > 1 {
				to.X += width - 1
			} else {
				// do not skip if this is a width right-padding
				// only if it's a tabspace left-padding.
				tokens := c.tabspaces - 1
				revertTo := to
				for tokens > 0 && to.X < endLastIdx && c.cells[to.Y][to.X].Ch == 0 {
					to.X++
					tokens--
				}
				if to.X < endLastIdx && c.cells[to.Y][to.X].Ch != '\t' {
					to = revertTo
				}
			}
		}
		to.X++
		// handle unexpected nulls at the end of line
		if to.X > endLastIdx {
			to.X = end.X
		}
	}

	startLastIdx := len(c.cells[from.Y]) - 1
	if from.X > 0 && from.X <= startLastIdx &&
		c.cells[from.Y][from.X].Ch == '\t' {
		from.X--
	}

	var done bool
	for from.X > 0 && from.X <= startLastIdx && c.cells[from.Y][from.X].Ch == 0 {
		from.X--
		done = true
	}

	// we need the last pad's position, but on the left there's no \t delimiter,
	// so we need to rollback one cell
	if done && c.cells[from.Y][from.X].Ch != 0 {
		// except if width > 1, in which case, the pad is a width pad
		// in that case, skip to right
		if width := c.cells[from.Y][from.X].Width; width > 1 {
			from.X += width
		} else {
			from.X++
		}
	}
	return from, to
}

func (c *rawCells) delete(from, to term.Coordinates) (
	start, end term.Coordinates, str string,
) {
	assertValidCoords(from)
	assertValidCoords(to)
	start, end = term.CoordinatesSort(from, to)
	start, end = c.skipPadding(start, end)

	builder := strings.Builder{}

	if start.Y == end.Y {
		c.deleteRowRange(&builder, start.Y, start.X, end.X)
		str = builder.String()
		return
	}

	// trim til end of first row
	if start.X < len(c.cells[start.Y]) {
		c.deleteRowRange(&builder, start.Y, start.X, len(c.cells[start.Y]))
	}
	if start.Y+1 < len(c.cells) {
		builder.WriteByte('\n')
	}

	// copy rows in between and move last row to second row, if applicable
	lastRow := end.Y
	if diff := end.Y - start.Y; diff > 1 {
		copyToBuilder(&builder, c.cells[start.Y+1:end.Y])
		copy(c.cells[start.Y+1:], c.cells[end.Y:])
		c.cells = c.cells[:len(c.cells)-diff+1]

		lastRow = start.Y + 1
		if lastRow < len(c.cells) {
			builder.WriteByte('\n')
		}
	}

	// then remove cells from last row; start.Y is now last row to delete
	if end.X > 0 {
		c.deleteRowRange(&builder, lastRow, 0, end.X)
	}

	// conflate last row in range
	if lastRow < len(c.cells) {
		c.conflate(start.Y)
	}

	str = builder.String()

	return
}

func (c *rawCells) Edit(_ context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	from = start
	to = start
	if start != end {
		from, _, old = c.delete(start, end)
		to = from
	}

	if str != "" {
		from, to = c.insert(from, str)
	}

	return
}

func (c *rawCells) Columns(y int) (j int) {
	j = len(c.cells[y])
	return
}

func (c *rawCells) Rows() int {
	return len(c.cells)
}

func (c *rawCells) String() string {
	return CellsToString(c.RawCells())
}

func (c *rawCells) RawCells() [][]term.Cell {
	return c.cells
}

func (c *rawCells) Cell(pos term.Coordinates) (
	cell term.Cell, ok bool,
) {
	assertValidCoords(pos)
	if pos.Y >= c.Rows() || pos.X >= len(c.cells[pos.Y]) {
		return
	}
	cell = c.cells[pos.Y][pos.X]
	ok = true
	return
}

func (c *rawCells) ReadFrom(r io.Reader) (int64, error) {
	rowY := nextWrite(c).Y
	reader := bufio.NewReader(r)
	n := int64(0)
	for {
		str, err := reader.ReadString('\n')
		state := -1
		var cluster string
		var width int
		n += int64(len([]byte(str)))
		for len(str) > 0 {
			// NOTE: this is significantly slower than, just ignoring grapheme clusters
			// but it should be ok as it's done once per file, and because calculating the width
			// is front loaded, it should amortize over long interactions on a particular file.
			cluster, str, width, state = graphemecluster.StepString(str, state)
			r := []rune(cluster)
			switch r[0] {
			case '\n':
				c.cells = append(c.cells, makeNewRow(0, c.columnCap))
				rowY++
			case '\t':
				for i := 1; r[0] == '\t' && i < c.tabspaces; i++ {
					c.cells[rowY] = append(c.cells[rowY], term.Cell{})
				}
				fallthrough
			default:
				var combining []rune
				if len(r) > 1 {
					combining = r[1:]
				}
				cell := term.Cell{Ch: r[0], Width: width, Combining: combining}
				c.cells[rowY] = append(c.cells[rowY], cell)
				for i := 1; i < width; i++ {
					c.cells[rowY] = append(c.cells[rowY], term.Cell{})
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return n, err
		}
	}
}
