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

package vtescreen

import (
	"fmt"
	"strings"

	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/term/vte/vteparser"
)

// PrimaryBuffer wraps a Buffer to provide scroll-back for a primary vte screen buffer.
// This implementation doesn't provide methods to update the underlying offset
// (it's always 0, with InvertOffset set to true), so callers must access the underlying
// Cells and add scroll via an external component. This is important to keep
// implementation simple, and avoid user scrolling interfering with standard vte processing.
type PrimaryBuffer struct {
	AltBuffer
	wraps      int
	maxHistory int
	minWidth   int
}

// NewPrimaryBuffer allocates storage for a new PrimaryBuffer and initializes it.
func NewPrimaryBuffer(minWidth, maxHistory int) *PrimaryBuffer {
	ret := new(PrimaryBuffer)
	ret.Init(minWidth, maxHistory)
	return ret
}

// Init initializes this PrimaryBuffer.
func (b *PrimaryBuffer) Init(minWidth int, maxHistory int) {
	b.AltBuffer.Init()
	b.maxHistory = maxHistory
	b.minWidth = minWidth
}

const wrapMarker uint8 = 1 << 7

// MarkWrapAtCursor marks the current line/column of the cursor
// as a wrapped line, so it can later be un-wrapped upon Resize.
func (b *PrimaryBuffer) MarkWrapAtCursor() {
	pos := b.CursorAtScroll()
	c := b.CellAt(pos)
	if c == nil {
		return
	}
	// use unused field to mark that line is wrapped oob
	c.Bytes = wrapMarker
}

// Resize resizes this Buffer and resets the vertical margins.
func (b *PrimaryBuffer) Resize(width, height int) {
	cursor := b.CursorAtScroll()
	savedCursor := b.scroll.WindowToScrollCoordinates(b.savedCursor.position)

	origWidth := width
	width = max(b.minWidth, width)

	var wraps int
	if width != 0 && width > b.width {
		wraps = b.growColumns(width, height)
	} else if width != 0 && width < b.width {
		wraps = b.shrinkColumns(width)
	}
	if height != 0 && (height > b.height || wraps < b.wraps) {
		b.growLines(width, height)
	}
	if height != 0 && (height < b.height || wraps > b.wraps) {
		b.shrinkLines(height)
	}
	b.wraps = wraps
	b.width = width
	b.height = height
	b.scroll.Resize(origWidth, height)
	b.Cells.ResetCapacity(width)

	cursor.Y = max(0, cursor.Y+wraps)
	b.cursor.position, _ = b.scroll.ScrollToWindowCoordinates(cursor)
	savedCursor.Y = max(0, b.savedCursor.position.Y+wraps)
	b.savedCursor.position = savedCursor
}

func (b *PrimaryBuffer) growLines(width, height int) {
	if b.Cells.Rows() < height {
		pos := term.Coordinates{
			Y: max(0, height-1),
			X: max(0, width-1),
		}
		b.Cells.InsertContext(b.AltBuffer.ctx, pos, b.AltBuffer.defaultChar)
	}
}

func (b *PrimaryBuffer) shrinkLines(height int) {
	pos := b.CursorAtScroll()
	for y := b.Cells.Rows() - 1; y > 0 && b.Cells.Rows() > height; y-- {
		if pos.Y >= y {
			break
		}
		from := term.Coordinates{Y: y}
		to := term.Coordinates{Y: y}
		b.Cells.DeleteLineContext(b.AltBuffer.ctx, from, to)
	}
}

func (b *PrimaryBuffer) growColumns(width, height int) (wraps int) {
	if width == 0 {
		return
	}
	pos := b.CursorAtScroll()
	var y int
	for y = max(0, b.Cells.Rows()-1); y > 0; y-- {
		if y == pos.Y {
			wraps = b.wrapTopLines(y, width)
			break
		}
		if b.Cells.Columns(y) < width {
			at := term.Coordinates{Y: y, X: width - 1}
			b.Cells.InsertContext(b.AltBuffer.ctx, at, b.AltBuffer.defaultChar)
		}
	}

	return
}

func (b *PrimaryBuffer) shrinkColumns(width int) (wraps int) {
	pos := b.CursorAtScroll()
	var y int
	for y = max(0, b.Cells.Rows()-1); y > 0; y-- {
		if y == pos.Y {
			wraps = b.wrapTopLines(y, width)
			break
		}
		if cols := b.Cells.Columns(y); cols > width {
			from := term.Coordinates{Y: y, X: width}
			to := term.Coordinates{Y: y, X: cols}
			b.Cells.DeleteContext(b.AltBuffer.ctx, from, to)
		}
	}

	y += wraps
	for ; y > 0 && y < b.Cells.Rows(); y-- {
		if cols := b.Cells.Columns(y); cols > width {
			from := term.Coordinates{Y: y, X: width}
			to := term.Coordinates{Y: y, X: cols}
			b.Cells.DeleteContext(b.AltBuffer.ctx, from, to)
		}
	}

	return
}

func (b *PrimaryBuffer) wrapTopLines(at, width int) (n int) {
	if width == 0 {
		return
	}
	// unwrap previous wraps
	for y := 0; y <= at && y < b.Cells.Rows(); y++ {
		for {
			lastCol := b.Cells.Columns(y) - 1
			if lastCol < 0 {
				break
			}
			c := b.CellAt(term.Coordinates{Y: y, X: lastCol})
			if c == nil || c.Bytes != wrapMarker {
				break
			}
			c.Bytes = 0
			if _, ok := b.Cells.ConflateRowContext(b.AltBuffer.ctx, y); ok {
				at--
				n--
			}
		}
	}
	b.log(log.TraceLevel, "un-wrapped %d lines", -n)
	if at < 0 {
		return
	}
	// wraps represents the wrapped lines and so
	// it's always a positive number, whereas n represent the total
	// wraps variation, so it can be negative if we unwrapped more
	// lines that we wrapped.
	var wraps int
	for y := 0; y <= at && y < b.Cells.Rows(); y++ {
		var x int
		for x = b.Cells.Columns(y) - 1; x > 0; x-- {
			cell, _ := b.Cells.Cell(term.Coordinates{Y: y, X: x})
			if cell.Ch != b.defaultChar && cell.Ch != ' ' {
				break
			}
		}
		cols := b.Cells.Columns(y)
		switch {
		case x < width && width <= cols:
			from := term.Coordinates{Y: y, X: width}
			to := term.Coordinates{Y: y, X: cols}
			b.Cells.DeleteContext(b.AltBuffer.ctx, from, to)
		case x >= width && width <= cols:
			from := term.Coordinates{Y: y, X: x + 1}
			to := term.Coordinates{Y: y, X: cols}
			b.Cells.DeleteContext(b.AltBuffer.ctx, from, to)
			// use the new number of columns
			cols = b.Cells.Columns(y)
			times := cols / width
			remainder := cols % width
			if remainder == 0 {
				times--
			}
			for i := times; i > 0 && b.Cells.WrapRowContext(b.AltBuffer.ctx, y, width*i); i-- {
				c := b.CellAt(term.Coordinates{Y: y, X: b.Cells.Columns(y) - 1})
				c.Bytes = wrapMarker
				n++
				at++
				wraps++
			}
			b.log(log.TraceLevel, "wrapped a new line line (%d), times: %d", y, times)
		case x < width && width > cols:
			at := term.Coordinates{Y: y, X: width - 1}
			b.Cells.InsertContext(b.AltBuffer.ctx, at, b.AltBuffer.defaultChar)
		default:
			panic("pack it up boys")
		}
	}

	// grow history lines that fall short or were wrapped
	for y := range at {
		if b.Cells.Columns(y) < width {
			at := term.Coordinates{Y: y, X: width - 1}
			b.Cells.InsertContext(b.AltBuffer.ctx, at, b.AltBuffer.defaultChar)
		}
	}
	b.log(log.TraceLevel, "wrapped back %d lines", wraps)
	return
}

// Dimensions returns the dimensions of this buffer.
func (b *PrimaryBuffer) Dimensions() (width, height int) {
	width = b.width
	height = b.height
	return
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

// InsertLinesCursor inserts blank lines on the cursor's position.
func (b *PrimaryBuffer) InsertLinesCursor(count int) {
	pos := b.CursorAtScroll()
	pos.X = b.Cells.Columns(pos.Y)
	b.InsertLines(count, pos)
}

// InsertLines inserts blank lines at the given position's line.
func (b *PrimaryBuffer) InsertLines(count int, pos term.Coordinates) {
	var builder strings.Builder
	if pos.X == 0 {
		for range count {
			for range b.width {
				_, _ = builder.WriteRune(b.defaultChar)
			}
			_ = builder.WriteByte('\n')
		}
	} else {
		for range count {
			_ = builder.WriteByte('\n')
			for range b.width {
				_, _ = builder.WriteRune(b.defaultChar)
			}
		}
	}
	b.Cells.Edit(b.ctx, pos, pos, builder.String())
	if b.Cells.Rows() > b.maxHistory {
		to := b.Cells.Rows() - b.maxHistory - 1
		b.log(log.TraceLevel, "rows > max history: %d", to)
		b.DeleteLines(0, to)
	}
}

// DeleteLines deletes the lines from start to end, inclusively.
func (b *PrimaryBuffer) DeleteLines(start, end int) {
	if start > end {
		return
	}
	end = min(end, b.Cells.Rows()-1)

	from := term.Coordinates{Y: start}
	to := term.Coordinates{Y: end}
	b.Cells.DeleteLineContext(b.ctx, from, to)
}

// Reset clears the screen and removes history, effectively
// leaving the content as blank and the cursor position at the top.
func (b *PrimaryBuffer) Reset() {
	b.resetLinesTrim(0, b.height, true, b.defaultChar)
	b.SetCursorAtScreen(term.Coordinates{}, false)
}

// Clear clears the screen and moves the current view into history, effectively
// leaving the content as blank and the cursor position at the top.
func (b *PrimaryBuffer) Clear() bool {
	// find the last row that's not a blank row; cursor cannot
	// be used because some shell implementations move the cursor
	// before sending the clear sequence. Cases:
	//
	//  X  X
	//  ---- start view
	//  XXXX
	//    XX
	//  ---- end view
	//
	//  1122 insert/count
	cells := b.Cells.RawCells()
	for y := b.Rows() - 1; y >= 0; y-- {
		for x := 0; x < b.Columns(y); x++ {
			c := cells[y][x]
			if c.Ch == b.defaultChar {
				continue
			}
			endWindowCoordinates, _ := b.scroll.ScrollToWindowCoordinates(term.Coordinates{Y: y})
			count := endWindowCoordinates.Y + 1
			if count <= 0 {
				return false
			}
			b.InsertLines(count, term.Coordinates{Y: y, X: b.Columns(y)})
			b.cursor.position = term.Coordinates{}
			b.log(log.TraceLevel, "clear view, insert lines count %d, found non-blank at y:%d", count, y)
			return true
		}
	}
	return false
}

// ClearHistory clears the scrollback history, leaving the current view as-is.
func (b *PrimaryBuffer) ClearHistory() bool {
	count := b.Cells.Rows() - b.height
	if count <= 0 {
		return false
	}
	b.DeleteLines(0, count-1)
	return true
}

// CursorAtScroll returns the cursor position in relation to the underlying
// content scroll.
func (b *PrimaryBuffer) CursorAtScroll() term.Coordinates {
	pos := b.scroll.WindowToScrollCoordinates(b.cursor.position)
	pos.X = max(0, pos.X)
	pos.Y = max(0, pos.Y)
	return pos
}

// ResetLines erases all the lines from start to end.
// The start to end range is left inclusive, right exclusive.
// As oppposed to AltBuffer's ResetLines, the `end` argument is not capped.
func (b *PrimaryBuffer) ResetLines(start, end int) {
	b.AltBuffer.ResetLinesWith(start, end, b.defaultChar)
}

// Insert inserts a new character at the cursor position, shifting right
// all the cells to the right of the cursor. It does not extend the number columns
// in the buffer, as it should always be capped at exactly b.Width(), set by
// the previous call to Resize.
func (b *PrimaryBuffer) Insert(c rune, width int, charset vteparser.CharsetIndex) {
	pos := b.CursorAtScroll()
	b.AltBuffer.InsertAt(pos, c, width, charset)
}

// Write writes the given character with the given width to the cell
// at the current cursor position.
func (b *PrimaryBuffer) Write(c rune, width int, charset vteparser.CharsetIndex) {
	pos := b.CursorAtScroll()
	b.AltBuffer.WriteAt(pos, c, width, charset)
}

// Delete deletes the the given number of cells, shifting left
// all the cells to the right of the cursor.
func (b *PrimaryBuffer) Delete(count int) {
	pos := b.CursorAtScroll()
	b.AltBuffer.DeleteAt(pos, count)
}

// ResetCells erases all the cells from start to end, on the current
// cursor line. The start to end range is left inclusive, right exclusive.
func (b *PrimaryBuffer) ResetCells(start, end int) {
	pos := b.CursorAtScroll()
	b.resetCellsAt(pos.Y, start, end, b.defaultChar)
}

func (t *PrimaryBuffer) log(level log.Level, line string, params ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "vtescreen.PrimaryBuffer").
		WithField("instance", fmt.Sprintf("%p", t)).
		Logf(level, line, params...)
}
