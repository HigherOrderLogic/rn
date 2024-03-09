package cell

import (
	"fmt"

	"unstable.build/go-tui/term"
)

// View is the interface that wraps methods to query a 2D matrix of term.Cell.
type View interface {
	Rows() int
	Columns(row int) int
	Cell(term.Coordinates) (term.Cell, bool)
	RawCells() [][]term.Cell
	fmt.Stringer
}

// Editor is the interface that wraps methods to mutate a 2D matrix of term.Cell.
type Editor interface {
	// Edit replaces any content from [start:end) with str and returns the
	// right-exclusive coordinates of the effective insert range. Note that
	// returned from, to values will be equal to each other if this operation
	// only removes content. This effectively allows clients to reverse a call
	// to Edit by calling it again with the last return values.
	//
	// This method should panic if delete range between start, end is out of bounds.
	Edit(start, end term.Coordinates, new string) (from, to term.Coordinates, old string)
}

// NewView returns a new Reader which reads from cells and uses tabspaces.
func NewView(cells [][]term.Cell, tabspaces int) View {
	r := &rawCells{
		cells:     cells,
		tabspaces: tabspaces,
		columnCap: defColumnCap,
		rowCap:    defRowCap,
	}
	return r
}
