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
	"runtime"
	"sync"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
)

type cellRunner interface {
	// runCell adapts the shader interface to be closer to pixel-shader.
	//
	// For instance Shadertoy's fragCoord would be fragCoordX and fragCoordY,
	// iResolution would be resolutionX and resolutionY and iTime would be
	// time (which is in seconds too).
	//
	// The original cell indices are passed along (cellCoords).
	runCell(
		frame, total int, fps float, time float,
		cellCoords term.Coordinates,
		fragCoordX, fragCoordY int,
		resolutionX, resolutionY int,
		inChar rune, inFg, inBg tcell.Color,
	) (char rune, fg, bg tcell.Color)
}

// glslHelper is a helper structure which parallelizes computation
type glslHelper struct {
	workers     int
	workersChan chan shadeRequest
}

func newHelper() *glslHelper {
	// must return a pointer, so we can use a runtime.Finalizer below
	// to cleanup worker goroutines upon garbage collection.
	return &glslHelper{workers: runtime.NumCPU()}
}

type shadeRequest struct {
	wg            *sync.WaitGroup
	y, rows, cols int
	row           []term.Cell
	time          float
	frame, total  int
	fps           float
	in            [][]term.Cell
	shader        cellRunner
}

// shadeGLSL adapts the input space to look like common pixel shader's input
// interface.
//
// To make it look like a pixel shader the Y axis is flipped and terminal cell
// aspect is taken into account, since pixels are squared and cells aren't.
//
// Intended to be run by the Shade() method of any pixel shader.
func (g *glslHelper) shadeGLSL(
	frame, total int, fps float, in [][]term.Cell, shader cellRunner,
) {
	if frame >= total {
		return
	}

	if g.workersChan == nil {
		g.initWorkers()
	}

	rows := len(in)
	cols := len(in[0])

	time := float(frame) / fps

	var wg sync.WaitGroup
	wg.Add(len(in))
	for y, row := range in {
		g.workersChan <- shadeRequest{
			wg: &wg,
			y:  y, rows: rows, cols: cols,
			row:   row,
			time:  time,
			frame: frame, total: total,
			fps:    fps,
			in:     in,
			shader: shader,
		}
	}
	wg.Wait()
}

func (g *glslHelper) initWorkers() {
	ch := make(chan shadeRequest)
	for i := 0; i < g.workers; i++ {
		go debug.CapturePanicReport(func() {
			for {
				req, ok := <-ch
				if !ok {
					return
				}
				shadeRow(req.wg, req.y, req.rows, req.cols,
					req.row, req.time, req.frame, req.total,
					req.fps, req.in, req.shader)
			}
		})
	}

	// cleanup workers when helper/shader is no longer in use
	runtime.SetFinalizer(g, func(g *glslHelper) {
		close(g.workersChan)
	})

	g.workersChan = ch
}

func shadeRow(
	wg *sync.WaitGroup, y, rows, cols int, row []term.Cell, time float,
	frame, total int, fps float, in [][]term.Cell, shader cellRunner,
) {
	defer wg.Done()

	_, fragCoordY, resolutionX, resolutionY := cellCoordToFragCoords(
		0, y, cols, rows,
	)

	for x := range row {
		char, fg, bg := shader.runCell(
			frame, total, fps, time,
			term.Coordinates{X: x, Y: y},
			x, fragCoordY,
			resolutionX, resolutionY,
			in[y][x].Ch, in[y][x].Fg, in[y][x].Bg,
		)
		in[y][x].Ch = char
		in[y][x].Fg = fg
		in[y][x].Bg = bg
	}
}

func cellCoordToFragCoords(x, y, cols, rows int) (
	fragCoordX, fragCoordY int, resolutionX, resolutionY int,
) {
	yFlipARCorrect := int(math.Round(float(rows-y-1) *
		asciiart.HeightToWidthCellAspectRatio))
	rowsFlipARCorrect := int(math.Round(float(rows) *
		asciiart.HeightToWidthCellAspectRatio))

	fragCoordX = x
	fragCoordY = yFlipARCorrect
	resolutionX = cols
	resolutionY = rowsFlipARCorrect
	return
}

func fragCoordsToCellCoords(
	fragCoordX, fragCoordY, resolutionX, resolutionY int,
) (x, y int, rows, cols int) {
	x = fragCoordX
	y = int(
		math.Round(
			float(resolutionY-fragCoordY-1) /
				asciiart.HeightToWidthCellAspectRatio,
		),
	)
	cols = int(float(resolutionY) / asciiart.HeightToWidthCellAspectRatio)
	rows = resolutionX
	return

}
