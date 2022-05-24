package termutil

import (
	"strings"

	"github.com/ernestrc/go-tui/term"
)

type Line struct {
	wrapped bool // whether line was wrapped onto from the previous one
	cells   []term.Cell
}

func newLine() Line {
	return Line{
		wrapped: false,
		cells:   []term.Cell{},
	}
}

func (line *Line) Len() uint16 {
	return uint16(len(line.cells))
}

func (line *Line) String() string {
	runes := []rune{}
	for _, cell := range line.cells {
		runes = append(runes, cell.Ch)
	}
	return strings.TrimRight(string(runes), "\x00")
}

func (line *Line) append(cells ...term.Cell) {
	line.cells = append(line.cells, cells...)
}

func (line *Line) shrink(width uint16) {
	if line.Len() <= width {
		return
	}
	remove := line.Len() - width
	var cells []term.Cell
	for _, cell := range line.cells {
		if cell.Ch == 0 && remove > 0 {
			remove--
		} else {
			cells = append(cells, cell)
		}
	}
	line.cells = cells
}

func (line *Line) wrap(width uint16) []Line {

	var output []Line
	var current Line

	current.wrapped = line.wrapped

	for _, cell := range line.cells {
		if len(current.cells) == int(width) {
			output = append(output, current)
			current = newLine()
			current.wrapped = true
		}
		current.cells = append(current.cells, cell)
	}

	return append(output, current)
}
