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

// shader remixed from CaliCoastReplay's 301's Fire Shader - Remix 2
// (https://www.shadertoy.com/view/MtcGD7)

import (
	"math"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/shader"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Blaze shader remixed from CaliCoastReplay's 301's Fire Shader - Remix 2
// (https://www.shadertoy.com/view/MtcGD7)
func Blaze(params BlazeParams, fps float) shader.Shader {
	return &blaze{
		helper:      newHelper(),
		BlazeParams: params,
		fps:         fps,
		c1:          vec3(0.5, 0.0, 0.1),
		c2:          vec3(0.9, 0.1, 0.0),
		c3:          vec3(0.2, 0.1, 0.7),
		c4:          vec3(1.0, 0.9, 0.1),
		c5:          vec3(0.1, 0.1, 0.1),
		c6:          vec3(0.9, 0.9, 0.9),
		k1:          vec4(0.0, -1.0/3.0, 2.0/3.0, -1.0),
		k2:          vec4(1.0, 2.0/3.0, 1.0/3.0, 3.0),
		randDotV:    vec2(12.9898, 12.1414),
		randG:       83758.5453,
		noiseD:      vec2(0.0, 1.0),
	}
}

// BlazeParams defines the parameters used by the Bomb shader.
type BlazeParams struct {
	Speed       vec2D
	SwapRedBlue bool
}

// DefaultBlazeParams return a set of sane BlazeParams.
func DefaultBlazeParams() BlazeParams {
	return BlazeParams{
		Speed:       vec2(1.2, 0.1),
		SwapRedBlue: false,
	}
}

type blaze struct {
	helper *glslHelper
	BlazeParams
	fps      float
	c1       vec3D
	c2       vec3D
	c3       vec3D
	c4       vec3D
	c5       vec3D
	c6       vec3D
	k1       vec4D
	k2       vec4D
	randDotV vec2D
	randG    float
	noiseD   vec2D
}

func (s *blaze) runCell(
	frame, total int, fps float, time float,
	cellCoords term.Coordinates,
	fragCoordX, fragCoordY int,
	resolutionX, resolutionY int,
	inChar rune, inFg, inBg tcell.Color,
) (char rune, fg, bg tcell.Color) {
	fragCoord := vec2(float(fragCoordX), float(fragCoordY))
	iTime := time
	iResolution := vec2(float(resolutionX), float(resolutionY))

	shift := 1.327 + sin(iTime*2.0)/2.4

	// change the constant term for all kinds of cool distance versions,
	// make plus/minus to switch between
	// ground fire and fire rain!
	dist := 3.5 - sin(iTime*0.4)/1.89

	p := fragCoord.multSc(dist / iResolution.x)
	p.x -= iTime / 1.1

	q := s.fbm(p.add(vec2FromScalar(1.0*sin(iTime)/10.0 - iTime*0.01)))
	qb := s.fbm(p.add(vec2FromScalar(0.1*cos(iTime)/5.0 + iTime*0.002)))
	q2 := s.fbm(p.add(vec2FromScalar(-iTime*0.44-5.0*cos(iTime)/7.0))) - 6.0
	q3 := s.fbm(p.add(vec2FromScalar(-iTime*0.9-10.0*cos(iTime)/30.0))) - 4.0
	q4 := s.fbm(p.add(vec2FromScalar(-iTime*2.0-20.0*sin(iTime)/20.0))) + 2.0

	q = (q + qb - 0.4*q2 - 2.0*q3 + 0.6*q4) / 3.8

	r := vec2(
		s.fbm(p.addSc(q/2.0+iTime*s.Speed.x-p.x-p.y)),
		s.fbm(p.addSc(q-iTime*s.Speed.y)),
	)
	c := mix3D(s.c1, s.c2, s.fbm(p.add(r))).
		add(mix3D(s.c3, s.c4, r.x)).
		sub(mix3D(s.c5, s.c6, r.y))
	color := c.multSc(cos(shift * fragCoord.y / iResolution.y))
	color = color.addSc(0.05)
	color.x *= 0.8

	hsv := s.rgb2hsv(color)
	hsv.y *= hsv.z * 1.1
	hsv.z *= hsv.y * 1.13
	hsv.y = (2.2 - hsv.z*.9) * 1.20
	color = s.hsv2rgb(hsv)
	color = clamp3D(color, vec3FromScalar(0.0), vec3FromScalar(1.0))

	if s.SwapRedBlue {
		color = vec3(color.z, color.y, color.x) // blue flames insteead of red
	}

	color = color.multSc(255.0)

	fg = inFg
	bg = tcell.NewRGBColor(int32(color.x), int32(color.y), int32(color.z))
	char = inChar

	return
}

func (s *blaze) rgb2hsv(c vec3D) vec3D {
	p := mix4D(vec4(c.z, c.y, s.k1.w, s.k1.z), vec4(c.y, c.z, s.k1.x, s.k1.y), step(c.z, c.y))
	q := mix4D(vec4(p.x, p.y, p.w, c.x), vec4(c.x, p.y, p.z, p.x), step(p.x, c.x))
	d := q.x - math.Min(q.w, q.y)
	return vec3(abs(q.z+(q.w-q.y)/(6.0*d+mathE)), d/(q.x+mathE), q.x)
}

func (s *blaze) hsv2rgb(c vec3D) vec3D {
	p := abs3D(fract3D(c.xxx().add(s.k2.xyz())).multSc(6.0).sub(s.k2.www()))
	ret := mix3D(
		s.k2.xxx(),
		clamp3D(p.sub(s.k2.xxx()), vec3FromScalar(0.0), vec3FromScalar(1.0)),
		c.y,
	).multSc(c.z)
	return ret
}

func (s *blaze) rand(n vec2D) float {
	return fract(sin(cos(dot2D(n, s.randDotV))) * s.randG)
}

func (s *blaze) noise(n vec2D) float {
	b := n.floor()
	f := smoothstep2D(vec2FromScalar(0.0), vec2FromScalar(1.0), fract2D(n))
	return mix(
		mix(s.rand(b), s.rand(b.add(s.noiseD.yx())), f.x),
		mix(s.rand(b.add(s.noiseD.yx())), s.rand(b.add(s.noiseD.yy())), f.x),
		f.y,
	)
}

func (s *blaze) fbm(n vec2D) float {
	total := 0.0
	amplitude := 1.0
	for i := 0; i < 5; i++ {
		total += s.noise(n) * amplitude
		n = n.add(n.multSc(1.7))
		amplitude *= 0.47
	}
	return total
}

func (s *blaze) Shade(frame, total int, in [][]term.Cell) {
	s.helper.shadeGLSL(frame, total, s.fps, in, s)
}
