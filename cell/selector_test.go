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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestSelect(t *testing.T) {
	str := `hello
	world

itsme`
	buf := newBufferWithContent(t, str)

	testCases := []selectCase{
		{
			from: term.Coordinates{},
			to:   term.Coordinates{X: 2, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}},
			},
		},
		{
			from: term.Coordinates{X: 2, Y: 0},
			to:   term.Coordinates{X: 2, Y: 1},
			expected: [][]term.Cell{
				{{Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}},
			},
		},
		{
			from: term.Coordinates{X: 3, Y: 0},
			to:   term.Coordinates{X: 1, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'e'}, {Ch: 'l'}},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 1},
			to:   term.Coordinates{X: 9, Y: 1},
			expected: [][]term.Cell{
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
			},
		},
		{
			from: term.Coordinates{X: 2, Y: 0},
			to:   term.Coordinates{X: 5, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
			},
		},
		{
			from: term.Coordinates{X: 2, Y: 0},
			to:   term.Coordinates{X: 8, Y: 1},
			expected: [][]term.Cell{
				{{Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
			},
		},
		{
			from: term.Coordinates{X: 8, Y: 1},
			to:   term.Coordinates{X: 2, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
			},
		},
		{
			from: term.Coordinates{X: 4, Y: 0},
			to:   term.Coordinates{X: 5, Y: 3},
			expected: [][]term.Cell{
				{{Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
				{},
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 2},
			to:   term.Coordinates{X: 5, Y: 3},
			expected: [][]term.Cell{
				{},
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 2},
			to:   term.Coordinates{X: 6, Y: 3},
			expected: [][]term.Cell{
				{},
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 2},
			to:   term.Coordinates{X: 5, Y: 4},
			expected: [][]term.Cell{
				{},
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
			},
		},
		{
			from: term.Coordinates{},
			to:   term.Coordinates{Y: 1},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 3},
			to:   term.Coordinates{X: 6, Y: 3},
			expected: [][]term.Cell{
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
			},
		},
	}

	for i, tcase := range testCases {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			selector := selector{view: buf.view}
			selection, coords := selector.selectCells(tcase.from, tcase.to)
			// we do not care about width; makes defining tests easier
			for y, row := range selection {
				for x := range row {
					selection[y][x].Width = 0
				}
			}
			require.Equal(t, term.CellsToString(tcase.expected), term.CellsToString(selection))
			assertReturnedCoordinatesSelectSame(t, selector, coords, tcase.expected, false)
		})
	}
}

func TestSelectLine(t *testing.T) {
	str := "hello\n\tworld\n\nitsme"
	buf := newBufferWithContent(t, str)

	testCases := []selectCase{
		{
			from: term.Coordinates{},
			to:   term.Coordinates{X: 1, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 2, Y: 0},
			to:   term.Coordinates{X: 4, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 2, Y: 0},
			to:   term.Coordinates{X: 7, Y: 1},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 2},
			to:   term.Coordinates{X: 2, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
				{},
				{},
			},
		},
		{
			to:   term.Coordinates{X: 2, Y: 0},
			from: term.Coordinates{X: 10, Y: 2},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
				{},
				{},
			},
		},
		{
			to:   term.Coordinates{X: 2, Y: 0},
			from: term.Coordinates{X: 0, Y: 10},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
				{},
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 1},
			to:   term.Coordinates{X: 0, Y: 1},
			expected: [][]term.Cell{
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}, {Ch: 'd'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 3},
			to:   term.Coordinates{X: 0, Y: 3},
			expected: [][]term.Cell{
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
				{},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 2},
			to:   term.Coordinates{X: 0, Y: 2},
			expected: [][]term.Cell{
				{},
				{},
			},
		},
	}

	for i, tcase := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			selector := selector{view: buf.view}
			selection, coords := selector.selectLine(tcase.from, tcase.to)
			// we do not care about width; makes defining tests easier
			for y, row := range selection {
				for x := range row {
					selection[y][x].Width = 0
				}
			}
			require.Equal(t, term.CellsToString(tcase.expected), term.CellsToString(selection))
			assertReturnedCoordinatesSelectSame(t, selector, coords, tcase.expected, true)
		})
	}
}

func TestSelectBlock(t *testing.T) {
	str := "hello\n\tworld\n\nitsme\n\n\nhi"
	buf := newBufferWithContent(t, str)

	testCases := []selectCase{
		{
			from: term.Coordinates{},
			to:   term.Coordinates{X: 2, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}},
			},
		},
		{
			from: term.Coordinates{X: 0, Y: 0},
			to:   term.Coordinates{X: 4, Y: 1},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}},
			},
		},
		{
			from: term.Coordinates{X: 4, Y: 1},
			to:   term.Coordinates{X: 0, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}},
			},
		},
		{
			from: term.Coordinates{X: 4, Y: 3},
			to:   term.Coordinates{X: 0, Y: 0},
			expected: [][]term.Cell{
				{{Ch: 'h'}, {Ch: 'e'}, {Ch: 'l'}, {Ch: 'l'}},
				{{Ch: '\t'}, {Ch: 'w'}, {Ch: 'o'}, {Ch: 'r'}},
				{},
				{{Ch: 'i'}, {Ch: 't'}, {Ch: 's'}, {Ch: 'm'}},
			},
		},
		{
			from: term.Coordinates{X: 5, Y: 0},
			to:   term.Coordinates{X: 2, Y: 6},
			expected: [][]term.Cell{
				{{Ch: 'l'}, {Ch: 'l'}, {Ch: 'o'}},
				{{Ch: 'o'}, {Ch: 'r'}, {Ch: 'l'}},
				{},
				{{Ch: 's'}, {Ch: 'm'}, {Ch: 'e'}},
				{},
				{},
				{},
			},
		},
	}

	for i, tcase := range testCases {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			selector := selector{view: buf.view}
			selection, coords := selector.selectBlock(tcase.from, tcase.to)
			// we do not care about width; makes defining tests easier
			for y, row := range selection {
				for x := range row {
					selection[y][x].Width = 0
				}
			}
			require.Equal(t, term.CellsToString(tcase.expected), term.CellsToString(selection), i)
			assertReturnedCoordinatesSelectSame(t, selector, coords, tcase.expected, false)
		})
	}
}

func assertReturnedCoordinatesSelectSame(
	t *testing.T, selector selector,
	coords []Selection, expected [][]term.Cell,
	isLine bool,
) {
	t.Helper()
	var selectedSelection [][]term.Cell
	for _, coords := range coords {
		cells, actualCoords := selector.selectCells(coords.From, coords.To)
		// it shouldn't break apart further
		require.Len(t, actualCoords, 1)
		assert.Equal(t, coords, actualCoords[0])
		selectedSelection = append(selectedSelection, cells...)
	}
	expect := term.CellsToString(expected)
	// line selection always appends a newline at the end
	// it should be the line paste that adds it, rather than the selection
	// but for now this is needed for this assertion
	if isLine {
		expect = string(expect[:len(expect)-1])
	}
	assert.Equal(t, expect, term.CellsToString(selectedSelection))
}
