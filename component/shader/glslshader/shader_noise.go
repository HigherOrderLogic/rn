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
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/shader"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Noise shades simplex noise patterns on screen.
//
// This is intended to be used as an exploration of that noise space.
func Noise(params NoiseParams, fps float) shader.Shader {
	return &noise{
		NoiseParams: params,
		fps:         fps,
		helper:      newHelper(),
	}
}

// DefaultNoiseParams return a set of sane NoiseParams.
func DefaultNoiseParams() NoiseParams {
	return NoiseParams{
		Animated:  true,
		Amplitude: 1.0,
		ScaleX:    5.0,
		ScaleY:    5.0,
		Speed:     1.0,
	}
}

// NoiseParams defines the parameters used by the Noise shader.
type NoiseParams struct {
	Animated  bool
	Amplitude float
	ScaleX    float
	ScaleY    float
	Speed     float
}

type noise struct {
	NoiseParams
	helper *glslHelper
	fps    float
}

func (s *noise) Shade(frame, total int, in [][]term.Cell) {
	s.helper.shadeGLSL(frame, total, s.fps, in, s)
}

func (s *noise) runCell(
	frame, total int, fps float, time float,
	cellCoords term.Coordinates,
	fragCoordX, fragCoordY int,
	resolutionX, resolutionY int,
	inChar rune, inFg, inBg tcell.Color,
) (char rune, fg, bg tcell.Color) {
	spedTime := s.Speed * time

	ar := float(resolutionX) / float(resolutionY)

	y := float(fragCoordY) / float(resolutionY)
	x := float(fragCoordX) / float(resolutionX)

	if s.Animated {
		y += 0.2 * spedTime
		x += 0.2 * spedTime
	}

	if s.Animated {
		x *= s.ScaleX + 3.0*cos(0.5*spedTime)
		y *= s.ScaleY + 3.0*sin(0.5*spedTime)
	} else {
		x *= s.ScaleX
		y *= s.ScaleY

	}
	y /= ar

	out := noiseSimplex(vec2(x, y))

	plane := 0.0
	if s.Animated {
		plane = sin(spedTime)
	}

	out = plane + s.Amplitude*(0.5*out)

	fgBase := int32(255.0 * (1.0 - out) * (1.0 - out))
	bgBase := int32(255 * out)

	char = inChar
	if out > 0.5 {
		char = 'x'
	}

	fg = tcell.NewRGBColor(fgBase, fgBase, fgBase)
	bg = tcell.NewRGBColor(bgBase, bgBase, bgBase)
	return
}
