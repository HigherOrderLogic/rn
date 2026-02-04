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
	"math"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// Selection represents a selection of cells, defined by
// a from and to coordinates.
type Selection struct {
	From term.Coordinates
	To   term.Coordinates
}

// selector extends a view to perform cell selection operations.
type selector struct {
	view View
}

type iterateFunc func(int, term.Coordinates, term.Coordinates, []term.Cell)

func (s *selector) selectCells(from term.Coordinates, to term.Coordinates) (
	res [][]term.Cell, sels []Selection,
) {
	res = make([][]term.Cell, 0)
	s.iterateCells(from, to, func(i int, from, to term.Coordinates, cells []term.Cell) {
		res = append(res, cells)
		sels = append(sels, Selection{From: from, To: to})
	})
	return
}

func (s *selector) iterateCells(from term.Coordinates, to term.Coordinates, op iterateFunc) {
	from, to = term.CoordinatesSort(from, to)
	cells := s.view.RawCells()

	var i int
	for from.Y < to.Y && from.Y < len(cells) {
		x := int(math.Min(float64(from.X), float64(len(cells[from.Y]))))
		op(i,
			term.Coordinates{Y: from.Y, X: x},
			term.Coordinates{Y: from.Y, X: len(cells[from.Y])},
			cells[from.Y][x:])
		i++
		from.X = 0
		from.Y++
	}

	if from.Y >= len(cells) {
		return
	}

	fromX := int(math.Min(float64(from.X), float64(len(cells[from.Y]))))
	toX := int(math.Min(float64(to.X), float64(len(cells[from.Y]))))
	op(i,
		term.Coordinates{Y: from.Y, X: fromX},
		term.Coordinates{Y: from.Y, X: toX},
		cells[from.Y][fromX:toX],
	)
}

func (s *selector) selectLine(from term.Coordinates, to term.Coordinates) (
	res [][]term.Cell, sels []Selection,
) {
	res = make([][]term.Cell, 0)
	s.iterateLine(from, to, func(i int, from, to term.Coordinates, cells []term.Cell) {
		res = append(res, cells)
		sels = append(sels, Selection{From: from, To: to})
	})
	// NOTE: adding a line at the end should be moved to paste line
	res = append(res, make([]term.Cell, 0))
	return
}

func (s *selector) iterateLine(from term.Coordinates, to term.Coordinates, op iterateFunc) {
	from, to = term.CoordinatesSort(from, to)
	cells := s.view.RawCells()

	i := 0
	for from.Y <= to.Y && from.Y < len(cells) {
		line := cells[from.Y][:]
		op(i,
			term.Coordinates{Y: from.Y, X: 0},
			term.Coordinates{Y: from.Y, X: len(line)},
			line,
		)
		i++
		from.Y++
	}
}

func (s *selector) iterateBlocks(
	from term.Coordinates, to term.Coordinates, op iterateFunc,
) {
	from, to = term.CoordinatesBlockSort(from, to)
	cells := s.view.RawCells()
	i := 0
	lencells := len(cells)
	for from.Y <= to.Y && from.Y < lencells {
		maxx := float64(len(cells[from.Y]))
		xfrom := int(math.Min(maxx, float64(from.X)))
		xto := int(math.Min(maxx, float64(to.X)))

		op(i,
			term.Coordinates{Y: from.Y, X: xfrom},
			term.Coordinates{Y: from.Y, X: xto},
			cells[from.Y][xfrom:xto])

		from.Y++
		i++
	}
}

func (s *selector) selectBlock(from term.Coordinates, to term.Coordinates) (
	res [][]term.Cell, sels []Selection,
) {
	res = make([][]term.Cell, 0)
	s.iterateBlocks(from, to, func(i int, from, to term.Coordinates, cells []term.Cell) {
		res = append(res, cells)
		sels = append(sels, Selection{From: from, To: to})
	})
	return
}
