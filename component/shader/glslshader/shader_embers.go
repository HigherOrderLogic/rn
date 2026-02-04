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
	"math/rand"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/shader"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Embers are shows burning in ring-like blobs that turn into ashes as sparks
// glitter on top. As this happens ascending rising particles displace the
// original characters that end up landing on the original position.
func Embers(params EmbersParams, defaultAttr term.Attributes, fps float) shader.Shader {
	// Change normalized parameters to suitable ranges.
	params.DurationPercCoolDown = clamp(params.DurationPercCoolDown, 0.0, 1.0)
	params.DurationPercDisappear = clamp(params.DurationPercDisappear, 0.0, 1.0)
	params.DurationPercFgEmberToAsh = clamp(params.DurationPercFgEmberToAsh, 0.0, 1.0)

	// Parameters that represent percentage of an animation between 0% (0.0)
	// and 100% (1.0) do not make sense to exceed 1.0 when added.
	if (params.DurationPercCoolDown + params.DurationPercDisappear) > 1.0 {
		panic("DurationCoolDown and DurationDisappear mustn't exceed 1.0 " +
			"(100% of the animation)",
		)
	}

	params.Scale *= 20.0       // from 0..1 to 0..20
	params.Fierce *= 2.0       // from 0..1 to 0..2
	params.GlowSpeed *= 20.0   // from 0..1 to 0..16
	params.SparksSpeed *= 60.0 // from 0..1 to 0..60

	return &embers{
		EmbersParams: params,
		defaultAttr:  defaultAttr,
		fps:          fps,
	}
}

// EmbersParams allows you to customize the Embers effect.
//
// Most parameters are in the normalized range 0-1. Exceeding the range is
// allowed in most of the cases (it's specified in the docstring).
type EmbersParams struct {
	// Draw these specific characters based on the heat.
	Chars []rune
	// DurationCoolDown (in percentage) of the embers to cool down and
	// (DurationDisappear) to vanish afterwards.
	// (range: 0..1 clamped (0% to 100%) and shouldn't exceed 1.0 when added)
	DurationPercCoolDown, DurationPercDisappear float
	// Controls how long does it take for the foreground characters to
	// transition between the spark colors to the ash pale colors.
	//
	// It's similar to DurationPercCoolDown and DurationPercDisappear but only
	// affects the foreground characters.
	//
	// (range: 0..1 clamped (0% to 100%) and shouldn't exceed 1.0 when added)
	DurationPercFgEmberToAsh float
	// The smaller the bigger the ember blobs.
	// (range: 0..1 unrestricted)
	Scale float
	// The higher the more fierce and larger the remaining ember rings.
	// (range: 0..1 unrestricted)
	Fierce float
	// How fast the heat rings pulsate.
	// (range: 0..1 unrestricted)
	GlowSpeed float
	// Sparks are warm-flickering star-like bright points that affect
	// foreground only.
	// (range: 0..1 unrestricted)
	SparksAmount float
	// How fast the sparks pulsate.
	// (range: 0..1 unrestricted)
	SparksSpeed float
	// Colors to paint the effect with.
	Colors EmbersColors
}

// DefaultEmbersParams gives you a set of params that exhibits the Embers
// effect properly. It's a good starting point to customizing on top of with
// parameter overrides.
func DefaultEmbersParams() EmbersParams {
	return EmbersParams{
		Chars:                    []rune{' ', '.', ':', '∴', 'o'},
		DurationPercCoolDown:     0.4,
		DurationPercDisappear:    0.4,
		DurationPercFgEmberToAsh: 0.3,
		Scale:                    0.25,
		Fierce:                   0.55,
		GlowSpeed:                0.5,
		SparksAmount:             0.45,
		SparksSpeed:              0.5,
		Colors:                   EmbersColorsRed(),
	}
}

// EmbersColorsRed provides natural fire-looking colors to grade the embers
// with.
//
// Aimed to be passed to EmbersParams.Colors.
func EmbersColorsRed() EmbersColors {
	return EmbersColors{
		Fg1:     tcell.NewRGBColor(255, 22, 0),
		Fg2:     tcell.NewRGBColor(255, 198, 28),
		Bg1:     tcell.NewRGBColor(255, 22, 0),
		Bg2:     tcell.NewRGBColor(255, 255, 0),
		Sparks1: tcell.NewRGBColor(241, 24, 0),
		Sparks2: tcell.NewRGBColor(243, 255, 24),
		Ashes:   tcell.NewRGBColor(47, 30, 32),
	}
}

// EmbersColors defines color-related parameters for EmbersParams.
type EmbersColors struct {
	// Colors to taint the foreground with: Fg1 on cooler ember regions
	// presence Fg2 otherwise.
	Fg1, Fg2 tcell.Color
	// Colors to taint the background with: Fg1 on cooler ember regions
	// presence Fg2 otherwise.
	Bg1, Bg2 tcell.Color
	// Colors to taint the foreground with: Fg1 on cooler ember parts Fg2
	// otherwise.
	Sparks1, Sparks2 tcell.Color
	// Base color for the ashes embers blend into.
	Ashes tcell.Color
}

type embers struct {
	EmbersParams
	defaultAttr term.Attributes
	fps         float
}

func (s *embers) Shade(frame, total int, cells [][]term.Cell) {
	if frame < 0 || frame >= total {
		return
	}

	for y := 0; y < len(cells); y++ {
		for x := 0; x < len(cells[0]); x++ {
			s.processCell(x, y, frame, total, cells)
		}
	}
}

func (s *embers) processCell(x, y int, frame, total int, cells [][]term.Cell) (
	embersAmount, hotRings float, // useful outputs for creative shader combination
) {
	if y >= len(cells) || x >= len(cells[y]) {
		return
	}

	// Prepare position vector to feed to embers generator.

	rows := len(cells)
	cols := len(cells[0])

	cellX := x
	cellY := y

	fragCoordX := x
	fragCoordY := int(math.Round(float(rows-y-1) *
		asciiart.HeightToWidthCellAspectRatio))
	resolutionX := cols
	resolutionY := int(math.Round(float(rows) *
		asciiart.HeightToWidthCellAspectRatio))

	aspectRatio := float(resolutionX) / float(resolutionY)

	p := vec2(float(fragCoordX), float(fragCoordY)).
		div(vec2(float(resolutionX), aspectRatio*float(resolutionY))).
		multSc(s.Scale)

	// Produce the embers.

	embersProgress := clamp(
		(float(frame))/(float(total)*s.DurationPercCoolDown),
		0.0, 1.0,
	)
	embersAmount, hotRings, embersChar, embersFg, embersBg := s.embers(
		p, cellX, cellY, embersProgress, frame, total, cells,
	)

	char := embersChar

	// Blend embers with ashes.

	ashesFg, ashesBg := s.ashes(p)

	const kFgEmberAshFadeAmplitude = 30.0
	fgEmbersAshesFade := clamp(
		(1.0)-(float(frame)-
			kFgEmberAshFadeAmplitude*float((fragCoordX+fragCoordY)%4))/
			(s.DurationPercFgEmberToAsh*float(total)),
		0.0, 1.0,
	)
	hotRingsFg := mix3D(
		colToVec(s.Colors.Sparks2, s.defaultAttr.Fg),
		colToVec(s.Colors.Sparks1, s.defaultAttr.Fg),
		noiseSimplex01(p.multSc(75.0)),
	)
	fg := vecToCol(mix3D(
		mix3D(ashesFg, embersFg, fgEmbersAshesFade), hotRingsFg, embersProgress*hotRings),
	)
	bg := vecToCol(mix3D(ashesBg, embersBg, embersAmount))
	_ = embersAmount

	cells[y][x].Fg = fg
	cells[y][x].Bg = bg
	cells[y][x].Ch = char

	// A cell with a character to be visible must have width set.
	if char != rune(0) {
		cells[cellY][cellX].Width = 1
	}

	return
}

func (s *embers) embers(
	p vec2D,
	cellX, cellY int,
	emberProgress float,
	frame, total int,
	cells [][]term.Cell,
) (emberAmount float, hotRings float, char rune, fg, bg vec3D) {
	if cellY >= len(cells) || cellX >= len(cells[cellY]) {
		return
	}

	cell := cells[cellY][cellX]

	progress := float(frame) / float(total)

	noise := noiseSimplex(p)
	strength := s.Fierce * smoothstep(
		s.DurationPercCoolDown+s.DurationPercDisappear,
		s.DurationPercCoolDown,
		progress,
	)
	loc := clamp(strength+noise-emberProgress, 0.0, 1.0)
	glow := 0.5 + 0.5*sin((s.GlowSpeed*(float(frame)/float(s.fps)))+loc*10.0)

	// General amount of ember.
	emberAmount = clamp(loc*glow, 0.0, 1.0)

	// Influence on fg and bg.
	bgAmount := emberAmount
	fgAmount := emberAmount

	// Chars glitch depending on ember location.
	char = s.Chars[int((loc)*(float(len(s.Chars))-1))]
	if char == ' ' {
		char = cell.Ch
	}

	// Cut some parts which we mark as sparks and draw glittery stars on them.
	sparksLoc := (1.0 - (0.5 + 0.5*noise)) * noiseSimplex(p.multSc(9.0))

	if sparksLoc >= (-1.0 + 2.0*float(1.0-s.SparksAmount)) {
		// Spark locations outside the ember blobs, scattered in ashes.
		char = cell.Ch
		if cell.Width == 0 { // show ash characters on empty cells
			char = s.Chars[rand.Intn(len(s.Chars)-1)]
			cells[cellY][cellX].Width = 1
		}
		const kBreakPatternX = 389
		const kBreakPatternY = 123
		sp := 0.5*sin(
			(s.SparksSpeed*(float(frame)/float(s.fps)))/(3.0+sparksLoc*4.0)+
				float(cellX*kBreakPatternX+cellY*kBreakPatternY)) + 0.5
		fg = mix3D(
			colToVec(s.Colors.Sparks2, s.defaultAttr.Fg),
			colToVec(s.Colors.Sparks1, s.defaultAttr.Fg),
			sp,
		)

	} else {
		// Non-spark characters living inside the ember blobs.
		fg = mix3D(
			colToVec(s.Colors.Fg1, s.defaultAttr.Fg),
			colToVec(s.Colors.Fg2, s.defaultAttr.Fg),
			fgAmount,
		)
		hotRings = fgAmount
		const kNonSparkHotRingThreshold = 0.33
		const kNonSparkHotRingMult = 3
		if hotRings > kNonSparkHotRingThreshold {
			hotRings = 1.0
		} else {
			hotRings *= kNonSparkHotRingMult
			if hotRings > 1.0 {
				hotRings = 1.0
			}
		}
	}

	bg = mix3D(
		colToVec(s.Colors.Bg2, s.defaultAttr.Bg),
		colToVec(s.Colors.Bg1, s.defaultAttr.Bg),
		0.5+0.5*sin(3.0*bgAmount),
	)

	return
}

func (s *embers) ashes(p vec2D) (fg vec3D, bg vec3D) {
	const (
		kScale1           = 0.9
		kScale2           = 1.7
		kScale3           = 8.0
		kAmplitude3       = 0.7
		kFgBase           = 80.0
		kFgNoiseAmplitude = 60.0
		kFgNoiseScale     = 12.0
	)

	// Stack some noises.
	af := clamp(noiseSimplex01(p.multSc(kScale1)), 0.0, 1.0)
	af += -0.5 + clamp(noiseSimplex01(p.multSc(kScale2)), 0.0, 1.0)
	af += kAmplitude3 * (-0.5 + clamp(noiseSimplex01(p.multSc(kScale3)), 0.0, 1.0))
	af = clamp(af, 0.0, 1.0)

	bg = mix3D(colToVec(s.Colors.Ashes, s.defaultAttr.Bg), vec3FromScalar(0.0), af)
	fg = bg.addSc(kFgBase + kFgNoiseAmplitude*noiseSimplex(p.multSc(kFgNoiseScale)))

	return fg, bg
}
