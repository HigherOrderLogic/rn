package vte

import (
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/screen"
)

// lastPromptLine this attempts to find the last "shell" prompt line,
// or falls back to returning the last block of content.
//
//	┌─────────┐
//	│$ OOOOO  │
//	│$ OOOOOOO│
//	│OOOOOOOO │
//	│$ XXXXXX │
//	└─────────┘
func lastPromptLine(view cell.View, width int) (from term.Coordinates, to term.Coordinates) {
	cells := view.RawCells()
	if len(cells) == 0 {
		return
	}
	to.Y = len(cells) - 1
	to.X = len(cells[to.Y])

outer:
	for y := len(cells) - 1; y >= 0; y-- {
		for x := len(cells[y]) - 1; x >= 0; x-- {
			if cells[y][x].Ch != screen.DefaultChar {
				to = term.Coordinates{Y: y, X: x + 1}
				break outer
			}
		}
	}

	var last term.Coordinates
	for y := to.Y; y >= 0; y-- {
		x := len(cells[y]) - 1
		if y == to.Y {
			x = to.X - 2
			// be robust against resizes. If there's a gap between
			// the end of the line and width, then consider that a
			// separate prompt line
		} else if x < width-1 {
			from = last
			return
		}
		for ; x >= 0; x-- {
			cell := cells[y][x]
			if cell.Ch == screen.DefaultChar {
				from = last
				return
			}
			last = term.Coordinates{Y: y, X: x}
		}
	}

	from = last
	return
}
