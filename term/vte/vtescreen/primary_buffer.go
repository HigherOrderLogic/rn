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
	"unstable.build/go-tui/cell"
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

// Restore replaces this primary buffer's rendered cells and cursor with a
// saved snapshot while preserving primary-buffer configuration.
func (b *PrimaryBuffer) Restore(cells [][]term.Cell, cursor term.Coordinates, width, height int) {
	b.AltBuffer.restore(cells, cursor, max(b.minWidth, width), height)
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
	rows := b.Cells.Rows()
	if rows <= height {
		return
	}
	// We may trim any rows beyond the cursor row down to at most `height`.
	floor := max(pos.Y+1, height)
	if rows <= floor {
		return
	}
	if _, ok := b.Cells.TrimRowsFromEnd(rows - floor); ok {
		return
	}
	for y := b.Cells.Rows() - 1; y > 0 && b.Cells.Rows() > height; y-- {
		if pos.Y >= y {
			break
		}
		from := term.Coordinates{Y: y}
		to := term.Coordinates{Y: y}
		b.Cells.DeleteLineContext(b.AltBuffer.ctx, from, to)
	}
}

func (b *PrimaryBuffer) growColumns(width, _ int) (wraps int) {
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
		b.Cells.ExtendRowToWidth(y, width)
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
	// Unwrap previous wraps in a single O(rows) pass.
	merged, ok := b.Cells.MergeMarkedRows(at+1,
		func(c term.Cell) bool { return c.Bytes == wrapMarker },
		func(c *term.Cell) { c.Bytes = 0 })
	if ok {
		at -= merged
		n -= merged
	}
	b.log(log.TraceLevel, "un-wrapped %d lines", -n)
	if at < 0 {
		return
	}
	// wraps represents the wrapped lines and so
	// it's always a positive number, whereas n represent the total
	// wraps variation, so it can be negative if we unwrapped more
	// lines that we wrapped.
	//
	// After the unwrap pass, rows [0..at] are each a single logical line.
	// For each one we either:
	//   - pad short rows to `width`,
	//   - trim trailing blanks and possibly split into multiple rows of
	//     length `width`.
	//
	// Trim and pad operations mutate only a single row. Splits change the
	// outer row count — we collect them and apply in a single batched
	// structural edit (`SplitRowsBatchPadded`) to avoid repeated O(rows)
	// slice shifts.
	var splits []cell.RowSplit
	var created int
	cells := b.Cells.RawCells()
	for y := 0; y <= at && y < len(cells); y++ {
		row := cells[y]
		var x int
		for x = len(row) - 1; x > 0; x-- {
			cell := row[x]
			if cell.Ch != b.defaultChar && cell.Ch != ' ' {
				break
			}
		}
		cols := len(row)
		switch {
		case x < width && width <= cols:
			from := term.Coordinates{Y: y, X: width}
			to := term.Coordinates{Y: y, X: cols}
			b.Cells.DeleteContext(b.AltBuffer.ctx, from, to)
		case x >= width && width <= cols:
			// If trimming trailing blanks would not change the split
			// count, skip the trim entirely and let the trailing blanks
			// become part of the tail row. This avoids a DeleteContext
			// call plus pad-slab fill work for those same blanks.
			contentLen := x + 1
			trimmedTimes := contentLen / width
			if contentLen%width == 0 {
				trimmedTimes--
			}
			untrimmedTimes := cols / width
			if cols%width == 0 {
				untrimmedTimes--
			}
			if trimmedTimes != untrimmedTimes {
				from := term.Coordinates{Y: y, X: contentLen}
				to := term.Coordinates{Y: y, X: cols}
				b.Cells.DeleteContext(b.AltBuffer.ctx, from, to)
				cells = b.Cells.RawCells()
				cols = len(cells[y])
			}
			times := cols / width
			remainder := cols % width
			if remainder == 0 {
				times--
			}
			if times > 0 {
				if splits == nil {
					// Cap hint: at most one split per remaining row.
					remain := (at + 1) - y
					splits = make([]cell.RowSplit, 0, remain)
				}
				splits = append(splits, cell.RowSplit{
					Y: y, Width: width, Times: times,
				})
				created += times
			}
			b.log(log.TraceLevel, "wrapped a new line line (%d), times: %d", y, times)
		case x < width && width > cols:
			b.Cells.ExtendRowToWidth(y, width)
		default:
			panic("pack it up boys")
		}
		// Structural edits above (Delete, Extend) may have moved the
		// backing slice; refresh our local view.
		cells = b.Cells.RawCells()
	}
	if len(splits) > 0 {
		// Pad split tails to `width` via a single slab allocation so the
		// subsequent "grow history lines" loop is a no-op for tail rows.
		b.Cells.SplitRowsBatchPadded(splits, width, b.AltBuffer.defaultChar)
		cells = b.Cells.RawCells()
		shift := 0
		for _, s := range splits {
			headY := s.Y + shift
			// Mark head + Times-1 intermediate pieces. The final (tail)
			// piece at headY + Times is the tail of the original line and
			// is NOT marked, so the next resize knows where the logical
			// line actually ends.
			for i := range s.Times {
				row := cells[headY+i]
				row[len(row)-1].Bytes = wrapMarker
			}
			shift += s.Times
		}
		n += created
		at += created
	}
	wraps := created

	// grow history lines that fall short or were wrapped
	for y := range at {
		if b.Cells.Columns(y) < width {
			b.Cells.ExtendRowToWidth(y, width)
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
	b.Cells.ResetPerformanceCapacity(b.height, b.width)
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

func (t *PrimaryBuffer) log(level log.Level, line string, params ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "vtescreen.PrimaryBuffer").
		WithField("instance", fmt.Sprintf("%p", t)).
		Logf(level, line, params...)
}
