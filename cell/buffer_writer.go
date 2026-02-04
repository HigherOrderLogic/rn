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
	"context"

	"github.com/unstablebuild/rune-go-sdk/term"
)

var _ term.Writer = (*BufferWriter)(nil)

// BufferWriter satisfies term.Writer with a Buffer.
type BufferWriter struct {
	width, height int
	Cursor        term.Coordinates
	cells         [][]term.Cell
	ctx           context.Context
}

// NewBufferWriter allocates storage for a new BufferWriter and initializes it.
func NewBufferWriter(ctx context.Context, width, height int) *BufferWriter {
	ret := new(BufferWriter)
	ret.Init(ctx, width, height)
	return ret
}

// Init initializes a BufferWriter's internal structures.
func (w *BufferWriter) Init(ctx context.Context, width, height int) {
	w.width, w.height = width, height
	w.ctx = ctx
	w.cells = make([][]term.Cell, w.height)
	for i := 0; i < w.height; i++ {
		w.cells[i] = make([]term.Cell, w.width)
	}
}

// SetCell satisfies term.Writer
func (w *BufferWriter) SetCell(pos term.Coordinates, c term.Cell) {
	if pos.X >= w.width || pos.Y >= w.height || pos.X < 0 || pos.Y < 0 {
		return
	}

	w.cells[pos.Y][pos.X] = c
}

// UnionAttributes satisfies term.Writer
func (w *BufferWriter) UnionAttributes(pos term.Coordinates, attr term.Attributes) {
	if pos.X >= w.width || pos.Y >= w.height || pos.X < 0 || pos.Y < 0 {
		return
	}
	w.cells[pos.Y][pos.X].Attributes = term.AttributesUnion(
		w.cells[pos.Y][pos.X].Attributes, attr)
}

// Flush satisfies term.Writer
func (w *BufferWriter) Flush() error {
	return nil
}

// Clear satisfies term.Writer
func (w *BufferWriter) Clear(attr term.Attributes) error {
	for y, row := range w.cells {
		for x := range row {
			w.cells[y][x] = term.Cell{Attributes: attr}
		}
	}
	return nil
}

// SetCursor satisfies term.Writer
func (w *BufferWriter) SetCursor(pos term.Coordinates) {
	w.Cursor = pos
}

// ToBuffer copies the underlying cells to b.
func (w *BufferWriter) ToBuffer(b *Buffer) {
	cells := new(rawCells)
	// trim starting at the first null column
	// of each row
	for y, row := range w.cells {
		for x, cell := range row {
			if cell.Ch == 0 {
				w.cells[y] = w.cells[y][:x]
				break
			}
		}
	}
	cells.cells = w.cells
	cells.fillInChar = ' '
	cells.columnCap = defColumnCap
	cells.rowCap = defRowCap
	b.initWithCells(cells)
}

// RawCells returns the raw cells written so far to this BufferWritter.
func (w *BufferWriter) RawCells() [][]term.Cell {
	return w.cells
}

// Context returns the context passed to BufferWriterr's constructors,
// or the last context set via SetContext.
func (w *BufferWriter) Context() context.Context {
	return w.ctx
}

// SetContext sets the context to be returned in the next call to Context.
func (w *BufferWriter) SetContext(ctx context.Context) {
	w.ctx = ctx
}
