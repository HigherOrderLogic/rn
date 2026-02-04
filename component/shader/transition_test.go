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

package shader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// testTransitionFrames runs the given transition shader's Shade() method on
// the globalFrames input domain and checks the received frames by each of the
// composing shaders of the transition to match the expected ones.
//
// An example on how to call it might be easier to grasp what each argument is:
//
//	mustPanic:         false
//	total:             20,
//	transition:        TransitionCut(defaultParams, TransitionCut(defaultParams, shA, shB), shC),
//	transitionShaders: []*mockShader{shA, shB, shC},
//	globalFrames:      []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
//	shaderTotals:      []int{5, 5, 10},
//	shaderFrames: [][]int{
//		{0, 1, 2, 3, 4, x, x, x, x, x, x, x, x, x, x, x, x, x, x, x}, // shA
//		{x, x, x, x, x, 0, 1, 2, 3, 4, x, x, x, x, x, x, x, x, x, x}, // shB
//		{x, x, x, x, x, x, x, x, x, x, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, // shC
//	},
func testTransitionFrames(
	t *testing.T,
	mustPanic bool,
	total int,
	transition Shader,
	transitionShaders []*mockShader,
	globalFrames []int,
	shaderFrames [][]int,
	shaderTotals []int,
) {
	require.Equal(t, len(transitionShaders), len(shaderFrames),
		"illegal test: to watch %d shaders %d elements must be passed to shaderFrames, not %d",
		len(transitionShaders), len(transitionShaders), len(shaderFrames),
	)
	require.Equal(t, len(transitionShaders), len(shaderTotals),
		"illegal test: to watch %d shaders %d elements must be passed to shaderTotals, not %d",
		len(transitionShaders), len(transitionShaders), len(shaderTotals),
	)
	for i := 0; i < len(transitionShaders); i++ {
		require.Equal(t,
			len(globalFrames), len(shaderFrames[i]),
			"illegal test: globalFrames and shaderFrames[%d] must equal in length", i,
		)
	}

	cells := makeCellMatrix(7, 4)

	shade := func() {
		const x = -770077007700
		const noFrame = x // visually helpful for marking unrendered frames in table tests

		for frIdx, globalFr := range globalFrames {
			expectShaderCalled := make([]bool, len(transitionShaders))
			for i := 0; i < len(transitionShaders); i++ {
				expectShaderCalled[i] = shaderFrames[i][frIdx] != noFrame
			}

			transition.Shade(globalFr, total, cells)

			for i := 0; i < len(transitionShaders); i++ {
				sh := transitionShaders[i]

				if expectShaderCalled[i] {
					localFr := shaderFrames[i][frIdx]
					assert.True(t, sh.shadeCalled,
						"(global frame %d) [shader %d] "+
							"expected to be called and hasn't been called", globalFr, i,
					)
					assert.InDelta(t, localFr, sh.frame, 0.01,
						"(global frame %d) [shader %d] wrong 'frame' passed to Shade() ", globalFr, i,
					)
					assert.InDelta(t, shaderTotals[i], sh.total, 0.01,
						"(global frame %d) [shader %d] wrong 'total' passed to Shade()", globalFr, i,
					)
				} else {
					assert.False(t, sh.shadeCalled,
						"(global frame %d) [shader %d] "+
							"not expected be called and has been called with %d", globalFr, i, sh.frame,
					)
				}

				sh.reset()
			}
		}
	}

	if mustPanic {
		assert.Panics(t, shade)
	} else {
		assert.NotPanics(t, shade)
	}
}

type mockShader struct {
	shadeCalled bool
	frame       int
	total       int
}

func (s *mockShader) Shade(frame, total int, in [][]term.Cell) {
	s.shadeCalled = true
	s.frame = frame
	s.total = total
}

func (s *mockShader) reset() {
	s.shadeCalled = false
	s.frame = 0
	s.total = 0
}

// newConstantShader gives you a shader that renders always the same char, fg
// and bg regardless of frame and total.
func newConstantShader(ch rune, fg, bg tcell.Color) *constantShader {
	return &constantShader{ch, fg, bg}
}

type constantShader struct {
	ch     rune
	fg, bg tcell.Color
}

func (s *constantShader) Shade(frame, total int, in [][]term.Cell) {
	for y, row := range in {
		for x := range row {
			in[y][x].Ch = s.ch
			in[y][x].Fg = s.fg
			in[y][x].Bg = s.bg
		}
	}
}

// makeCellMatrix creates an two-dimensional array of zero-value cells
// representing a grid of w rows and h cols.
//
// NOTE: copy-pasted from shadertest.MakeCellMatrix
func makeCellMatrix(w int, h int) [][]term.Cell {
	cells := make([][]term.Cell, h)
	for y := 0; y < h; y++ {
		cells[y] = make([]term.Cell, w)
	}
	return cells
}

// assertMatrixAllCells checks all cells honor the assertion.
func assertMatrixAllCells(in [][]term.Cell, assertion func(term.Cell) bool) bool {
	is := true
	for _, row := range in {
		for _, cell := range row {
			is = is && assertion(cell)
			if !is {
				return false
			}
		}
	}
	return is
}

// assertMatrixSomeCells checks if one or more cells honor the assertion whilst others don't.
func assertMatrixSomeCells(in [][]term.Cell, assertion func(term.Cell) bool) bool {
	pass, fail := false, false
	for _, row := range in {
		for _, cell := range row {
			if assertion(cell) {
				pass = true
			} else {
				fail = true
			}
			if pass && fail {
				return true
			}
		}
	}
	return false
}
