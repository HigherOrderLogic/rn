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

package shadertest

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/component/shader/timeshader"
)

// TestShader runs an exhaustive suite of tests against a shader.Shader.
func TestShader(t *testing.T, sh shader.Shader) {
	testFrameRanges(t, sh)
	testFedIntoSingleTimeShader(t, sh)
	testFedIntoMultipleTimeShaders(t, sh)
	testCellMatrix(t, sh)
}

// BenchmarkShader runs a benchmark against shader.Shader.
func BenchmarkShader(
	b *testing.B, sh shader.Shader, width, height int, cells [][]term.Cell,
) {
	copy := MakeCellMatrix(width, height)

	b.ResetTimer()
	totalFrames := 30 * 5 // 5 seconds at 30fps
	for i := 0; i < b.N; i++ {
		for i := range totalFrames {
			term.CopyCells(copy, cells)
			sh.Shade(i, totalFrames, copy)
		}
	}
}

func testFrameRanges(t *testing.T, sh shader.Shader) {
	tsuite := []struct {
		name       string
		frameStart int
		frameEnd   int
		total      int
	}{
		{
			name:       "shading continuously forward doesn't panic",
			frameStart: 0,
			frameEnd:   19,
			total:      20,
		},
		{
			name:       "running mid-frame continuously doesn't panic",
			frameStart: 9,
			frameEnd:   12,
			total:      20,
		},
		{
			name:       "shading continuously backwards doesn't panic",
			frameStart: 19,
			frameEnd:   0,
			total:      20,
		},
		{
			name:       "shading frames out of total range doesn't panic",
			frameStart: 30,
			frameEnd:   50,
			total:      20,
		},
		{
			name:       "negative frame range doesn't panic",
			frameStart: -10,
			frameEnd:   10,
			total:      20,
		},
		{
			name:       "total is zero doesn't panic",
			frameStart: 0,
			frameEnd:   20,
			total:      0,
		},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				cells := MakeCellMatrix(7, 4)
				if tcase.frameStart == tcase.frameEnd {
					sh.Shade(tcase.frameStart, tcase.total, cells)
				} else if tcase.frameStart < tcase.frameEnd {
					for f := tcase.frameStart; f < tcase.frameEnd; f++ {
						sh.Shade(f, tcase.total, cells)
					}
				} else {
					// tcase.frameStart > tcase.frameEnd
					for f := tcase.frameStart; f > tcase.frameEnd; f-- {
						sh.Shade(f, tcase.total, cells)
					}
				}
			})
		})
	}

}

func testFedIntoSingleTimeShader(t *testing.T, sh shader.Shader) {
	for timeShaderName, timeShaderFn := range timeShaderFns {
		t.Run("when fed into "+timeShaderName+" time shader doesn't panic", func(t *testing.T) {
			tsh := timeShaderFn.fn(sh)
			cells := MakeCellMatrix(7, 4)
			for frame := 0; frame < 10; frame++ {
				assert.NotPanics(t, func() {
					tsh.Shade(frame, 10, cells)
				})
			}
		})
	}
}
func testFedIntoMultipleTimeShaders(t *testing.T, sh shader.Shader) {
	t.Run("doesn't panic when stacking all time shaders", func(t *testing.T) {
		tsh := sh
		cells := MakeCellMatrix(7, 4)
		for _, timeShaderFn := range timeShaderFns {
			tsh = timeShaderFn.fn(tsh)
		}
		for frame := 0; frame < 10; frame++ {
			assert.NotPanics(t, func() {
				tsh.Shade(frame, 10, cells)
			})
		}
	})
}

func testCellMatrix(t *testing.T, sh shader.Shader) {
	wht := tcell.ColorWhite
	blk := tcell.ColorBlack

	// regular cell (char, fg and bg are set)
	cell := func(ch rune) term.Cell {
		return term.Cell{
			Ch:         ch,
			Attributes: term.Attributes{Fg: wht, Bg: blk, Attrs: tcell.AttrNone},
			Combining:  []rune{},
			Width:      1,
		}
	}

	// zero-value cell
	cellZr0 := func() term.Cell {
		return term.Cell{}
	}

	// foreground-only cell (char and fg are set)
	cellFgO := func() term.Cell {
		c := term.Cell{}
		c.Ch = 'F'
		c.Fg = wht
		c.Width = 1
		return c
	}

	// background-only cell (bg is set)
	cellBgO := func() term.Cell {
		c := term.Cell{}
		c.Bg = blk
		return c
	}

	// italic cell (char, fg, bg and italic attr are set)
	itaCell := func() term.Cell {
		return term.Cell{
			Ch:         'C',
			Attributes: term.Attributes{Fg: wht, Bg: blk, Attrs: tcell.AttrItalic},
			Combining:  []rune{},
			Width:      1,
		}
	}

	// character-less cell, emulates empty spaces on the window
	cellGap := func() term.Cell {
		return term.Cell{
			Ch:         0,
			Attributes: term.Attributes{Fg: wht, Bg: blk, Attrs: tcell.AttrNone},
			Combining:  []rune{},
			Width:      0,
		}
	}

	tsuite := []struct {
		name  string
		cells [][]term.Cell
	}{
		{
			name: "standard square size (5x5)",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h')},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j')},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o')},
			},
		},
		{
			name: "proportional resize down: smaller square size (3x3)",
			cells: [][]term.Cell{
				{cell('a'), cell('b')},
				{cell('c'), cell('d')},
			},
		},
		{
			name: "proportional resize up: larger square size (7x7)",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "resize: narrower: reduce columns (7x5)",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h')},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j')},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap()},
			},
		},
		{
			name: "resize: shorter: reduce rows (7x2)",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap()},
			},
		},
		{
			name: "resize: taller: increase columns (7x3)",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h')},
			},
		},
		{
			name: "resize: wider: incrase rows (10x3)",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap(), cell('a'), cell('a'), cell('a')},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap(), cell('a'), cellGap(), cell('a')},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap(), cell('a'), cell('a'), cell('a')},
			},
		},
		{
			name: "resize: rotate (3x10)",
			cells: [][]term.Cell{
				{cell('a'), cell('c'), cellGap()},
				{cell('b'), cell('d'), cellGap()},
				{cellGap(), cellGap(), cellGap()},
				{cellGap(), cell('e'), cell('f')},
				{cellGap(), cellGap(), cell('h')},
				{cell('1'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap()},
				{cell('a'), cell('a'), cell('a')},
				{cell('a'), cellGap(), cell('a')},
				{cell('a'), cell('a'), cell('a')},
			},
		},
		{
			name: "cells with different styles",
			cells: [][]term.Cell{
				{cell('a'), itaCell(), cellGap(), cellGap(), cellGap(), itaCell(), cellGap()},
				{cell('c'), cell('d'), cellBgO(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellFgO(), cell('i'), cellGap(), cell('j'), cellGap(), cellFgO()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), itaCell(), cellGap(), cellBgO(), itaCell(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellBgO(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "some cells with zero values",
			cells: [][]term.Cell{
				{cellZr0(), cell('b'), cellZr0(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellZr0(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellZr0(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellZr0(), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
			},
		},
		{
			name: "first row is nil",
			cells: [][]term.Cell{
				nil,
				{cellZr0(), cell('b'), cellZr0(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellZr0(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellZr0(), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
			},
		},
		{
			name: "all rows but last is nil",
			cells: [][]term.Cell{
				nil,
				nil,
				nil,
				nil,
				nil,
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
			},
		},
		{
			name: "last row is nil",
			cells: [][]term.Cell{
				{cellZr0(), cell('b'), cellZr0(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellZr0(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellZr0(), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
				nil,
			},
		},
		{
			name: "some non-contiguous rows are nil",
			cells: [][]term.Cell{
				{cellZr0(), cell('b'), cellZr0(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellZr0(), cell('e'), cellGap(), cellGap(), cellGap()},
				nil,
				{cellGap(), cellGap(), cellZr0(), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				nil,
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
			},
		},
		{
			name: "some contiguous rows are nil",
			cells: [][]term.Cell{
				{cellZr0(), cell('b'), cellZr0(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellZr0(), cell('e'), cellGap(), cellGap(), cellGap()},
				nil,
				nil,
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellZr0()},
			},
		},
		{
			name: "first row has less column cells",
			cells: [][]term.Cell{
				{cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "last row has less column cells",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap()},
			},
		},
		{
			name: "some rows have less column cells",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "each row has more cells than it's previous",
			cells: [][]term.Cell{
				{cell('a')},
				{cell('c'), cell('d')},
				{cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "each row has more cells than it's previous",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h')},
				{cellGap(), cellGap(), cell('i'), cellGap()},
				{cell('k'), cell('l'), cell('m')},
				{cellGap(), cellGap()},
				{cellGap()},
			},
		},
		{
			name: "first row is empty",
			cells: [][]term.Cell{
				{},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "last row is empty",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cell('f'), cell('h'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('8')},
				{},
			},
		},
		{
			name: "some rows are empty",
			cells: [][]term.Cell{
				{cell('a'), cell('b'), cellGap(), cellGap(), cellGap(), cell('1'), cellGap()},
				{cell('c'), cell('d'), cellGap(), cell('e'), cellGap(), cellGap(), cellGap()},
				{},
				{cellGap(), cellGap(), cell('i'), cellGap(), cell('j'), cellGap(), cellGap()},
				{cell('k'), cell('l'), cell('m'), cell('n'), cell('o'), cellGap(), cellGap()},
				{},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "all rows but last are empty",
			cells: [][]term.Cell{
				{},
				{},
				{},
				{},
				{},
				{},
				{cellGap(), cellGap(), cellGap(), cellGap(), cellGap(), cell('4'), cellGap()},
			},
		},
		{
			name: "only gap cells",
			cells: [][]term.Cell{
				{cellGap(), cellGap()},
				{cellGap(), cellGap()},
			},
		},
		{
			name: "only zero-value cells",
			cells: [][]term.Cell{
				{cellZr0(), cellZr0()},
				{cellZr0(), cellZr0()},
			},
		},
		{
			name: "only cells with char and foreground",
			cells: [][]term.Cell{
				{cellFgO(), cellFgO()},
				{cellFgO(), cellFgO()},
			},
		},
		{
			name: "only cells with background and no char",
			cells: [][]term.Cell{
				{cellFgO(), cellFgO()},
				{cellFgO(), cellFgO()},
			},
		},
		{
			name: "only nil rows",
			cells: [][]term.Cell{
				nil,
				nil,
			},
		},
		{
			name: "1-cell matrix: gap cell",
			cells: [][]term.Cell{
				{cellGap()},
			},
		},
		{
			name: "1-cell matrix: zero-value cell",
			cells: [][]term.Cell{
				{cellZr0()},
			},
		},
		{
			name: "1-cell matrix: char and fg cell",
			cells: [][]term.Cell{
				{cellFgO()},
			},
		},
		{
			name: "1-cell matrix: bg no char",
			cells: [][]term.Cell{
				{cellFgO()},
			},
		},
		{
			name: "1-cell matrix: nil",
			cells: [][]term.Cell{
				nil,
			},
		},
	}

	t.Run("cell matrix sizes, resizes, malformed shape and empty cells", func(t *testing.T) {
		for _, tcase := range tsuite {
			total := 20
			for frame := 0; frame < 20; frame++ {
				assert.NotPanics(t, func() {
					sh.Shade(frame, total, tcase.cells)
				}, tcase.name+" frame "+strconv.Itoa(frame))
			}
		}
	})
}

var timeShaderFns = map[string]timeShaderFn{
	"Abs":        {func(sh shader.Shader) shader.Shader { return timeshader.Abs(sh) }},
	"Accelerate": {func(sh shader.Shader) shader.Shader { return timeshader.Accelerate(sh) }},
	"Decelerate": {func(sh shader.Shader) shader.Shader { return timeshader.Decelerate(sh) }},
	"Loop":       {func(sh shader.Shader) shader.Shader { return timeshader.Loop(sh, 1) }},
	"PingPong":   {func(sh shader.Shader) shader.Shader { return timeshader.PingPong(sh) }},
	"Reverse":    {func(sh shader.Shader) shader.Shader { return timeshader.Reverse(sh) }},
	"Sine":       {func(sh shader.Shader) shader.Shader { return timeshader.Sine(sh, 1) }},
}

type timeShaderFn struct {
	fn func(shader.Shader) shader.Shader
}
