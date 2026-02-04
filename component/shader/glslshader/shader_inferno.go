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

// Inferno shader remixed from codevinsky's Fire (https://www.shadertoy.com/view/XsXXRN)
func Inferno(params InfernoParams, fps float) shader.Shader {
	return &inferno{
		helper:        newHelper(),
		InfernoParams: params,
		fps:           fps,
		c1:            vec3(0.5, 0.0, 0.1),
		c2:            vec3(0.9, 0.1, 0.0),
		c3:            vec3(0.2, 0.1, 0.7),
		c4:            vec3(1.0, 0.9, 0.1),
		c5:            vec3(0.1, 0.1, 0.1),
		c6:            vec3(0.9, 0.9, 0.9),
		K1:            vec4(0.0, -1.0/3.0, 2.0/3.0, -1.0),
		K2:            vec4(1.0, 2.0/3.0, 1.0/3.0, 3.0),
	}
}

// InfernoParams defines the parameters used by the Inferno shader.
type InfernoParams struct {
	Speed       vec2D
	SwapRedBlue bool
}

// DefaultInfernoParams return a set of sane InfernoParams.
func DefaultInfernoParams() InfernoParams {
	return InfernoParams{
		Speed:       vec2(1.2, 0.1),
		SwapRedBlue: false,
	}
}

type inferno struct {
	helper *glslHelper
	InfernoParams
	fps float
	c1  vec3D
	c2  vec3D
	c3  vec3D
	c4  vec3D
	c5  vec3D
	c6  vec3D
	K1  vec4D
	K2  vec4D
}

func (s *inferno) runCell(
	frame, total int, fps float, time float,
	cellCoords term.Coordinates,
	fragCoordX, fragCoordY int,
	resolutionX, resolutionY int,
	inChar rune, inFg, inBg tcell.Color,
) (char rune, fg, bg tcell.Color) {
	fragCoord := vec2(float(fragCoordX), float(fragCoordY))

	iTime := time
	iResolution := vec2(float(resolutionX), float(resolutionY))

	shift := 1.6
	p := fragCoord.multSc(8.0).div(iResolution.xx())
	q := s.fbm(p.subSc(iTime * 0.1))
	r := vec2(s.fbm(p.addSc(q+iTime*s.Speed.x-p.x-p.y)), s.fbm(p.addSc(q-iTime*s.Speed.y)))
	c := mix3D(s.c1, s.c2, s.fbm(p.add(r))).add(mix3D(s.c3, s.c4, r.x)).sub(mix3D(s.c5, s.c6, r.y))
	col := c.mult(vec3FromScalar(cos(shift * fragCoord.y / iResolution.y)))
	col = clamp3D(col, vec3FromScalar(0.0), vec3FromScalar(1.0))
	col = col.multSc(255.0)

	if s.SwapRedBlue {
		col = vec3(col.z, col.y, col.x) // blue flames insteead of red
	}

	fg = inFg
	bg = tcell.NewRGBColor(int32(col.x), int32(col.y), int32(col.z))
	char = inChar
	return
}

func (s *inferno) rand(n vec2D) float {
	return fract(cos(dot2D(n, vec2(12.9898, 4.1414))) * 43758.5453)
}

func (s *inferno) noise(n vec2D) float {
	d := vec2(0.0, 1.0)
	b := n.floor()
	f := smoothstep2D(vec2FromScalar(0.0), vec2FromScalar(1.0), fract2D(n))
	return mix(
		mix(s.rand(b), s.rand(b.add(d.yx())), f.x),
		mix(s.rand(b.add(d.yx())), s.rand(b.add(d.yy())), f.x),
		f.y,
	)
}

func (s *inferno) fbm(n vec2D) float {
	total := 0.0
	amplitude := 1.0
	for i := 0; i < 4; i++ {
		total += s.noise(n) * amplitude
		n = n.add(n)
		amplitude *= 0.5
	}
	return total
}

func (s *inferno) Shade(frame, total int, in [][]term.Cell) {
	s.helper.shadeGLSL(frame, total, s.fps, in, s)
}
