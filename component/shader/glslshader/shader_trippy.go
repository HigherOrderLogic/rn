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

// Trippy layers animated colors with some noise.
//
// Tutorial file from: http://x.unstable.build/docs/tutorials/ox/pixel_shader#porting-from-shadertoy
func Trippy(params TrippyParams, fps float) shader.Shader {
	return &trippy{
		helper:       newHelper(),
		TrippyParams: params,
		fps:          fps,
		noiseD:       vec2(0.0, 1.0),
		randVec:      vec2(12.9898, 4.1414),
	}
}

// DefaultTrippyParams return a set of sane TrippyParams.
func DefaultTrippyParams() TrippyParams {
	return TrippyParams{Speed: 1.0}
}

// TrippyParams defines the parameters used by the Trippy shader.
type TrippyParams struct {
	Speed float // float is an alias of float64
}

type trippy struct {
	TrippyParams
	helper  *glslHelper
	fps     float
	noiseD  vec2D
	randVec vec2D
}

func (s *trippy) Shade(frame, total int, in [][]term.Cell) {
	s.helper.shadeGLSL(frame, total, s.fps, in, s)
}

func (s *trippy) runCell(
	frame, total int, fps float, time float,
	cellCoords term.Coordinates,
	fragCoordX, fragCoordY int,
	resolutionX, resolutionY int,
	inChar rune, inFg, inBg tcell.Color,
) (char rune, fg, bg tcell.Color) {
	fragCoord := vec2(float(fragCoordX), float(fragCoordY))

	iTime := time // time in seconds
	iResolution := vec2(float64(resolutionX), float64(resolutionY))

	// glsl: vec2 uv = fragCoord/iResolution.xy; // normalized pixel coords (from 0 to 1)
	// normalized pixel coordinates (from 0 to 1)
	uv := fragCoord.div(iResolution)

	// glsl: uv.y /= iResolution.x / iResolution.y;
	// show always squares regardless of screen aspect ratio
	ar := float(resolutionX) / float(resolutionY)
	uv.y /= ar

	// glsl: vec3 col = 0.5 + 0.5*cos(speed*iTime+uv.xyx+vec3(0,2,4));
	col := vec3FromScalar(0.5).add(cos3D(
		vec3FromScalar(s.Speed * iTime).add(uv.xyx()).add(vec3(0.0, 2.0, 4.0)),
	).multSc(0.5))
	// glsl: col *= noise(5.0*(0.5+0.5*sin(iTime))*uv/0.3);
	col = col.multSc(
		s.noise(
			vec2FromScalar(
				5.0 * (0.5 + 0.5*sin(iTime)),
			).mult(uv.div(vec2FromScalar(0.3))),
		),
	)

	col = col.multSc(255.0)
	bg = tcell.NewRGBColor(int32(col.x), int32(col.y), int32(col.z))
	return
}

func (s *trippy) rand(n vec2D) float {
	return fract(cos(dot2D(n, s.randVec)) * 43758.5453)
}

func (s *trippy) noise(n vec2D) float {
	d := s.noiseD
	b := n.floor()
	f := smoothstep2D(vec2FromScalar(0.0), vec2FromScalar(1.0), fract2D(n))
	return mix(
		mix(s.rand(b), s.rand(b.add(d.yx())), f.x),
		mix(s.rand(b.add(d.yx())), s.rand(b.add(d.yy())), f.x),
		f.y,
	)
}
