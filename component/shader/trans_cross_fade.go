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
	"math"
	"math/rand"

	"unstable.build/go-tui/component/shader/shaderutils"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// TransitionCrossFade plays the first shader and before ending fades it
// out while fading in the second shader simultaneously for some frames.
//
// Colors are linearly interpolated and characters are randomly changed
// based on time-independent position-based random noise.
//
//	    Transition Diagram
//	|     S·····C·····E     | S: transStartFrame
//	|     [·c·r·o·s·s·] █4█ | C: changePoint
//	|     [·····|·····] ▀▀▀ | E: transEndFrame
//	|     [·····|·█3█·]     |
//	|     [·····|·▀▀▀·]     |
//	|     [·█2█·|·····]     |
//	|     [·▀▀▀·|·····]     |
//	| █1█ [·····|·····]     |
//	| ▀▀▀ [·····|·····]     |
//	|--shader1--|--shader2--|time->
//	|     [·c·r·o·s·s·]
func TransitionCrossFade(
	params TransitionCrossFadeParams,
	defaultAttr term.Attributes,
	shader1, shader2 Shader,
) Shader {
	if params.ChangeAtPerc < 0.0 || params.ChangeAtPerc > 1.0 {
		panic("ChangeAtPerc must be within the closed interval [0,1]")
	}

	if params.OverlapPerc < 0.0 || params.OverlapPerc > 1.0 {
		panic("OverlapPerc must be within the closed interval [0,1]")
	}

	return &transCrossFade{
		TransitionCrossFadeParams: params,
		shader1:                   shader1,
		shader2:                   shader2,
		defaultAttr:               defaultAttr,
	}
}

// DefaultTransitionCrossFadeParams return a set of sane TransitionCrossFadeParams.
func DefaultTransitionCrossFadeParams() TransitionCrossFadeParams {
	return TransitionCrossFadeParams{
		ChangeAtPerc: 0.5,
		OverlapPerc:  0.25,
	}
}

// TransitionCrossFadeParams defines the parameters used by the TransitionCrossFade shader.
type TransitionCrossFadeParams struct {
	// Point within closed interval [0.0,1.0] at which the shader change happens.
	ChangeAtPerc float64
	// Point within closed interval [0.0,1.0] at which the shaders overlap at
	// cross fading area.
	OverlapPerc float64
}

type transCrossFade struct {
	TransitionCrossFadeParams
	shader1     Shader
	shader2     Shader
	buf         [][]term.Cell
	activations [][]bool
	defaultAttr term.Attributes
}

func (t *transCrossFade) Shade(frame, total int, in [][]term.Cell) {
	changePointFrame := t.ChangeAtPerc * float64(total)
	transDurationInFrames := float64(total) * t.OverlapPerc

	transStartFrame := changePointFrame - (transDurationInFrames / 2.0)
	transEndFrame := transStartFrame + transDurationInFrames

	transHalfDur := transDurationInFrames / 2.0

	totalShader1 := math.Round(changePointFrame + transHalfDur)
	frameShader1 := math.Round(float64(frame))
	shader1FrameOverflow := frameShader1 >= totalShader1

	// TransitionCrossFade docstring diagram case 1:
	// no need for transition, it's 100% shader1
	if float64(frame) < transStartFrame {
		t.shader1.Shade(int(frameShader1), int(totalShader1), in)
		return
	}

	totalShader2 := math.Round(float64(total) - changePointFrame + transHalfDur)
	frameShader2 := math.Round((float64(frame) - (changePointFrame - transHalfDur)))

	// TransitionCrossFade docstring diagram case 4:
	// no need for transition, it's 100% shader2
	if float64(frame) > transEndFrame || shader1FrameOverflow {
		t.shader2.Shade(int(frameShader2), int(totalShader2), in)
		return
	}

	// TransitionCrossFade docstring diagram case 2 or case 3: we are in
	// transition zone from here on, depending if we are before or after
	// changePointFrame we will write the first or second shader to buf

	t.copyCells(in)

	midPointFramePassed := float64(frame) >= changePointFrame
	if midPointFramePassed {
		// TransitionCrossFade docstring diagram case 3:
		t.shader2.Shade(int(frameShader2), int(totalShader2), in)
		t.shader1.Shade(int(frameShader1), int(totalShader1), t.buf)
	} else {
		// TransitionCrossFade docstring diagram case 2:
		t.shader1.Shade(int(frameShader1), int(totalShader1), in)
		t.shader2.Shade(int(frameShader2), int(totalShader2), t.buf)
	}

	t.interpolateBuffer(
		frame, midPointFramePassed,
		transStartFrame, transEndFrame, transDurationInFrames,
		in,
	)
}

func (t *transCrossFade) interpolateBuffer(
	frame int,
	midPointPassed bool,
	transStartFrame float64,
	transEndFrame float64,
	transDurationInFrames float64,
	final [][]term.Cell,
) {
	var frameWithinTransition float64
	if !midPointPassed {
		frameWithinTransition = float64(frame) - transStartFrame
	} else {
		frameWithinTransition = (transEndFrame - float64(frame))
	}

	// -- Color Transition

	prog0to05 := frameWithinTransition / transDurationInFrames // range: 0.0..0.5

	for y, row := range final {
		for x := range row {
			final[y][x].Fg = shaderutils.InterpolateColor(
				prog0to05, final[y][x].Fg, t.buf[y][x].Fg, t.defaultAttr.Fg,
			)
			final[y][x].Bg = shaderutils.InterpolateColor(
				prog0to05, final[y][x].Bg, t.buf[y][x].Bg, t.defaultAttr.Bg,
			)
			t.activations[y][x] = false
		}
	}

	if len(final) == 0 || len(final[0]) == 0 {
		return
	}

	// -- Character Transition

	// During transition at max reveal that amount of characters, which is
	// adaptive to window resolution and amount of transitioning frame. If it
	// doesn't generalize well, then rework the arithmetics!
	maxCharRevealPerFrame := int(7.0 * float64(len(final)*len(final[0])) / transDurationInFrames)

	// NOTE: performance sacrifice here since we can't garantee we will receive
	// the frames linearly we run the simulation up to the frame we want so
	// this shader can be sampled at any point in time without having to run
	// the previous frames
	rng := rand.New(rand.NewSource(0))
	for f := 0; f < int(math.Round((float64(frame) - transStartFrame))); f++ {
		for i := 0; i < maxCharRevealPerFrame; i++ {
			rowIdx := rng.Intn(len(t.activations))
			row := t.activations[rowIdx]
			colIdx := rng.Intn(len(row))
			t.activations[rowIdx][colIdx] = true
		}
	}

	for y, row := range final {
		for x := range row {
			if !midPointPassed {
				if t.activations[y][x] {
					final[y][x].Ch = t.buf[y][x].Ch
				}
			} else {
				if !t.activations[y][x] {
					final[y][x].Ch = t.buf[y][x].Ch
				}
			}
		}
	}

}

func (t *transCrossFade) copyCells(in [][]term.Cell) {
	t.ensureBufCap(in)
	for i, row := range in {
		copy(t.buf[i], row)
	}
}

func (t *transCrossFade) ensureBufCap(in [][]term.Cell) {
	if len(t.buf) < len(in) {
		t.buf = append(t.buf, make([][]term.Cell, len(in)-len(t.buf))...)
		t.activations = append(t.activations, make([][]bool, len(in)-len(t.activations))...)
	}
	for i, row := range in {
		if len(t.buf[i]) < len(row) {
			t.buf[i] = append(t.buf[i], make([]term.Cell, len(row)-len(t.buf[i]))...)
		}
		if len(t.activations[i]) < len(row) {
			t.activations[i] = append(t.activations[i], make([]bool, len(row)-len(t.activations[i]))...)
		}
	}
}
