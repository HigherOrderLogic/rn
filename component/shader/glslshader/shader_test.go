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

package glslshader

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/shader/shadertest"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestShadeGLSL(t *testing.T) {
	sh := newFakeShader()

	t.Run("receives correct progress and duration", func(t *testing.T) {
		defer sh.cr.reset()
		cells := shadertest.MakeCellMatrix(4, 3)
		sh.Shade(20, 100, cells)
		assert.Equal(t, 20, sh.cr.frame)
		assert.Equal(t, 100, sh.cr.total)
		assert.Equal(t, 20.0/30.0, sh.cr.time)
	})

	t.Run("receives horizontal resolution as the number of cell columns", func(t *testing.T) {
		defer sh.cr.reset()
		cells := shadertest.MakeCellMatrix(4, 3)
		sh.Shade(20, 100, cells)
		assert.Equal(t, 4, sh.cr.resolutionX)
	})

	t.Run("runs once per cell", func(t *testing.T) {
		defer sh.cr.reset()
		cells := shadertest.MakeCellMatrix(4, 3)
		sh.Shade(20, 100, cells)
		assert.Len(t, sh.cr.coordsExecuted, 4*3)
		for _, row := range cells {
			for _, cell := range row {
				assert.Equal(t, uint8(1), cell.Width)
			}
		}
	})

	t.Run("adjusts vertical resolution considering the cell aspect ratio", func(t *testing.T) {
		defer sh.cr.reset()
		cells := shadertest.MakeCellMatrix(4, 3)
		sh.Shade(20, 100, cells)
		assert.NotEqual(t, 3, sh.cr.resolutionY)
		assert.Equal(t, int(math.Round(3*asciiart.HeightToWidthCellAspectRatio)), sh.cr.resolutionY)
	})

	t.Run("receives rows in reverse order (vertical axis is flipped)", func(t *testing.T) {
		defer sh.cr.reset()
		cells := shadertest.MakeCellMatrix(4, 3)
		sh.Shade(20, 100, cells)

		// as you can see the vertical range is not 0-3 but 0-5, this is
		// because aspect ratio correction (term cells are not square! they are
		// taller, so this is adjusted for the GLSL-like runner)
		expect := []coord{
			{0, 5}, {1, 5}, {2, 5}, {3, 5},
			{0, 2}, {1, 2}, {2, 2}, {3, 2},
			{0, 0}, {1, 0}, {2, 0}, {3, 0},
		}
		assert.Equal(t, expect, sh.cr.coordsExecuted)
	})

	t.Run("receives vertical coordinates adjusted to cell aspect ratio", func(t *testing.T) {
		defer sh.cr.reset()

		cells := shadertest.MakeCellMatrix(15, 25)
		sh.Shade(20, 100, cells)

		topLeftCoord := sh.cr.coordsExecuted[0]
		assert.Equal(t, topLeftCoord.y, int(math.Round((25-1)*asciiart.HeightToWidthCellAspectRatio)))
	})
}

func TestCoordinateSpaceTransform(t *testing.T) {
	t.Run("pixel shader frag coords from cell coords", func(t *testing.T) {
		x := 2
		y := 10
		cols := 100
		rows := 30
		fragX, fragY, resX, resY := cellCoordToFragCoords(x, y, cols, rows)
		assert.Equal(t, 2, fragX)
		assert.Equal(t, 44, fragY)
		assert.Equal(t, 100, resX)
		assert.Equal(t, 69, resY)
	})

	t.Run("cell coords from pixel shader frag coords", func(t *testing.T) {
		fragX := 2
		fragY := 44
		resX := 100
		resY := 69
		x, y, cols, rows := fragCoordsToCellCoords(fragX, fragY, resX, resY)
		assert.Equal(t, 2, x)
		assert.Equal(t, 10, y)
		assert.Equal(t, 100, cols)
		assert.Equal(t, 30, rows)
	})

	t.Run(
		"check we can generally convert from cell coords to fragment coords and viceversa",
		func(t *testing.T) {
			inX := 2
			inY := 10
			inRows := 30
			inCols := 100

			fragX, fragY, resX, resY := cellCoordToFragCoords(inX, inY, inCols, inRows)
			outX, outY, outCols, outRows := fragCoordsToCellCoords(fragX, fragY, resX, resY)

			assert.Equal(t, inX, outX)
			assert.Equal(t, inY, outY)
			assert.Equal(t, inCols, outCols)
			assert.Equal(t, inRows, outRows)
		})

	// Reflect the current behaviour where due to truncated divisions of floats
	// the transformed values can't be transformed back into their originals.
	// Because of this limitation we introduced the cellCoords as an argument
	// to cellRunner.runCell...
	t.Run(
		"converting from cell to frag coord and back might produce inaccurate results",
		func(t *testing.T) {
			rows := 30
			cols := 100
			inaccurate := false
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					fragX, fragY, resX, resY := cellCoordToFragCoords(x, y, cols, rows)
					outX, outY, outCols, outRows := fragCoordsToCellCoords(fragX, fragY, resX, resY)
					if x != outX || y != outY || cols != outCols || rows != outRows {
						inaccurate = true
					}
				}
			}
			assert.True(t, inaccurate)
		})
}

func newFakeShader() *fakeShader {
	cr := &fakeCellRunner{}
	cr.coordsExecuted = []coord{}
	helper := newHelper()
	// last row processed might not be the last row, so asserts might not work
	helper.workers = 1
	return &fakeShader{helper: helper, cr: cr}
}

type fakeShader struct {
	cr     *fakeCellRunner
	helper *glslHelper
}

func (s *fakeShader) Shade(frame, total int, in [][]term.Cell) {
	s.helper.shadeGLSL(frame, total, 30, in, s.cr)
}

type coord struct {
	x int
	y int
}

type fakeCellRunner struct {
	frame                    int
	total                    int
	fps                      float
	time                     float
	resolutionX, resolutionY int
	coordsExecuted           []coord
}

func (s *fakeCellRunner) runCell(
	frame, total int, fps float, time float,
	cellCoords term.Coordinates,
	fragCoordX, fragCoordY int,
	resolutionX, resolutionY int,
	inChar rune, inFg, inBg tcell.Color,
) (char rune, fg, bg tcell.Color) {
	s.frame = frame
	s.total = total
	s.fps = fps
	s.time = time
	s.resolutionX = resolutionX
	s.resolutionY = resolutionY
	s.coordsExecuted = append(s.coordsExecuted, coord{fragCoordX, fragCoordY})
	return inChar, inFg, inBg
}

func (s *fakeCellRunner) reset() {
	s.frame = 0
	s.total = 0
	s.fps = 0
	s.resolutionX = 0
	s.resolutionY = 0
	s.coordsExecuted = nil
}
