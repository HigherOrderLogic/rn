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
	"bytes"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// CellsToBytesBuffer copies the bytes representation of the given cell matrix
// to the supplied buffer.
//
// Caller is responsible for resetting buffer prior to this call if necessary.
func CellsToBytesBuffer(buffer *bytes.Buffer, cells [][]term.Cell) {
	copyToBuffer(buffer, cells)
}

// CellsToBuffer efficienty returns a Buffer that uses c as the
// underlying matrix of cells.
//
// Note that this buffer will honor the tabspaces observed in c.
// If c does not have any tabspaces, then 1 tabspace is assumed.
//
// Furthermore, it won't treat the last EOL as mandatory so it can be used
// as an in-memory buffer.
func CellsToBuffer(c [][]term.Cell) *Buffer {
	cells := new(rawCells)

	cells.init()
	cells.cells = term.CopyCells(cells.cells, c)

	// rawCells hasthe property that there's always at least one row
	if cells.Rows() == 0 {
		cells.fillInRows(0)
	}

	ret := new(Buffer)
	ret.initWithCells(cells)
	return ret
}

// ConvertRunePosToCoordinates converts the given column and row position in number of
// bytes, into code-point coordinates.
func ConvertRunePosToCoordinates(cells [][]term.Cell, y, x int) (
	ret term.Coordinates, ok bool,
) {
	if len(cells) == 0 {
		return
	}

	if y > len(cells) {
		y = len(cells)
	}

	ret.Y = y
	if ret.Y == len(cells) {
		ok = true
		return
	}

	line := cells[ret.Y]
	cellView := [1][]term.Cell{line}
	var bret term.Coordinates
	bret, ok = ConvertByteOffsetToCoordinates(cellView[:], x)
	if !ok {
		return
	}
	ret.X = bret.X
	return
}

// ConvertCoordinatesToRunePos converts the given coordinates, which
// represent code point coordinates into row, column position in numbers of bytes.
func ConvertCoordinatesToRunePos(cells [][]term.Cell, c term.Coordinates) (
	y, x int, ok bool,
) {
	if c.Y < 0 || c.X < 0 {
		panic("negative coordinates")
	}
	if len(cells) == 0 {
		return
	}

	if c.Y > len(cells) {
		c.Y = len(cells)
	}
	y = c.Y
	if c.Y == len(cells) {
		ok = true
		return
	}

	line := cells[c.Y]
	if c.X > len(line) {
		c.X = len(line)
	}
	cellView := [1][]term.Cell{line}
	var bretX int
	bretX, ok = ConvertCoordinatesToByteOffset(cellView[:], term.Coordinates{X: c.X})
	if !ok {
		return
	}
	x = bretX
	return
}

// ConvertByteOffsetToCoordinates converts the given byte offsets to term.Coordinates.
// It returns false if it's out of bounds.
func ConvertByteOffsetToCoordinates(cells [][]term.Cell, offset int) (
	ret term.Coordinates, ok bool,
) {
	if offset < 0 {
		panic("negative offset")
	}
	if len(cells) == 0 {
		return term.Coordinates{}, false
	}
	pos := 0
	for y, row := range cells {
		if pos >= offset {
			return term.Coordinates{X: 0, Y: y}, true
		}
		for x, cell := range row {
			pos += int(cell.Bytes)
			if pos >= offset {
				return term.Coordinates{X: x + 1, Y: y}, true
			}
		}
		if pos == offset {
			return term.Coordinates{X: len(row), Y: y}, true
		}
		if y+1 < len(cells) {
			pos++
		}
	}
	return term.Coordinates{X: 0, Y: len(cells)}, true
}

// ConvertCoordinatesToByteOffset converts the given coordinates to a byte offset.
// It returns false if it's out of bounds.
func ConvertCoordinatesToByteOffset(cells [][]term.Cell, c term.Coordinates) (
	offset int, ok bool,
) {
	if c.Y < 0 || c.X < 0 {
		panic("negative coordinates")
	}
	if c == (term.Coordinates{}) {
		return 0, len(cells) >= 1
	}
	var row []term.Cell
	if c.Y >= len(cells) {
		c.Y = len(cells)
	} else {
		row = cells[c.Y]
	}
	if c.X > len(row) {
		c.X = len(row)
	}
	for y := 0; y < c.Y; y++ {
		for _, cell := range cells[y] {
			offset += int(cell.Bytes)
		}
		offset++ // \n
	}
	for x := 0; x < c.X; x++ {
		offset += int(row[x].Bytes)
	}
	return offset, true
}

func nextWrite(c View) term.Coordinates {
	y := c.Rows() - 1
	x := len(c.RawCells()[y])
	return term.Coordinates{X: x, Y: y}
}
