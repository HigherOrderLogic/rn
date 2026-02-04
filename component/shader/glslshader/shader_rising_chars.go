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

	"unstable.build/go-tui/component/shader"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// RisingChars produce ascending particles that displacing the original
// characters. The traveling character ends up landing on the original position
// producing a kind of alien abduction effect.
func RisingChars(
	params RisingCharsParams, defaultAttr term.Attributes,
) shader.Shader {
	params.DurationPercTravelling = clamp(params.DurationPercTravelling, 0.0, 1.0)
	params.DurationPercStillness = clamp(params.DurationPercStillness, 0.0, 1.0)

	// Parameters that represent percentage of an animation between 0% (0.0)
	// and 100% (1.0) do not make sense to exceed 1.0 when added.
	if (params.DurationPercTravelling + params.DurationPercStillness) > 1.0 {
		panic("DurationPercTravelling and DurationPercStillness mustn't exceed 1.0 " +
			"(100% of the animation)",
		)
	}

	params.TravelDistance *= 70.0   // from 0..1 to 0..70
	params.CharFgFlickerSpeed *= 20 // from 0..1 to 0..20

	return &risingChars{
		RisingCharsParams: params,
		defaultAttr:       defaultAttr,
	}
}

// RisingCharsParams allows you to customize the RisingChars effect.
//
// Most parameters are in the normalized range 0-1. Exceeding the range is
// allowed in most of the cases (it's specified in the docstring).
type RisingCharsParams struct {
	// DurationPercTravelling (in percentage) sets how long the characters are
	// in motion and (DurationPercStillness) is the time they stay after
	// landing to their destination.
	// (range: 0..1 clamped (0% to 100%) and shouldn't exceed 1.0 when added)
	DurationPercTravelling, DurationPercStillness float
	// How far the characters are displaced, if they are further then it will
	// visibly look faster.
	// (range: 0..1 unrestricted)
	TravelDistance float
	// The speed at which the travelling character foreground color is
	// animated.
	// (range: 0..1 unrestricted)
	CharFgFlickerSpeed float
}

// DefaultRisingCharsParams gives you a set of params that exhibits the
// RisingChars effect properly. It's a good starting point to customizing on
// top of with parameter overrides.
func DefaultRisingCharsParams() RisingCharsParams {
	return RisingCharsParams{
		DurationPercTravelling: 0.57,
		DurationPercStillness:  0.43,
		TravelDistance:         0.71,
		CharFgFlickerSpeed:     0.8,
	}
}

type risingChars struct {
	defaultAttr term.Attributes
	RisingCharsParams
	charTxs []charTx // character translations
}

func (s *risingChars) Shade(frame, total int, cells [][]term.Cell) {
	s.computeTranslations(frame, total, cells)
	for y := 0; y < len(cells); y++ {
		for x := 0; x < len(cells[0]); x++ {
			s.processCell(x, y, frame, total, cells)
		}
	}
}

func (s *risingChars) computeTranslations(frame, total int, cells [][]term.Cell) {
	w := len(cells[0])
	h := len(cells)

	// Amount of time while the character translation is happening.
	durationCharTrans := int(math.Round(s.DurationPercTravelling * float(total)))

	// Amount of time we rest after the translation happened.
	durationAfterTrans := int(math.Round(s.DurationPercStillness * float(total)))

	// This initialization step is more valuable to have it here than in
	// constructor because that way logic corresponding to char transformations
	// are close together in code.
	if len(s.charTxs) == 0 {
		for i := 0; i < (w * h / 7); i++ {
			ox := rand.Intn(w)
			oy := rand.Intn(h)

			// do not bother transforming empty cells
			if cells[oy][ox].Ch == ' ' || cells[oy][ox].Width == 0 {
				continue
			}

			s.charTxs = append(s.charTxs,
				charTx{
					dest:   &term.Coordinates{X: ox, Y: oy},
					destCh: cells[oy][ox].Ch,
					curr:   &term.Coordinates{X: ox, Y: oy},
					orig:   &term.Coordinates{X: ox, Y: oy + 5 + rand.Intn(h)},
					frameOffset: int(s.TravelDistance * noiseSimplex(
						vec2(float(ox), float(oy)).div(vec2(float(w), float(h))).multSc(10.0)),
					),
				},
			)
		}
	}

	framesLeft := total - frame

	for i, charTx := range s.charTxs {
		fr := framesLeft - durationAfterTrans - charTx.frameOffset
		t := 1.0 - (float(fr) / float(durationCharTrans))
		t = clamp(t, 0.0, 1.0)
		t = 1.0 - (1.0-t)*(1.0-t)

		tx := t*float64(charTx.dest.X) + (1.0-t)*float(charTx.orig.X)
		ty := t*float64(charTx.dest.Y) + (1.0-t)*float(charTx.orig.Y)

		s.charTxs[i].curr.X = int(math.Round(tx))
		s.charTxs[i].curr.Y = int(math.Round(ty))
		if charTx.dest.Y < len(cells) && charTx.dest.X < len(cells[charTx.dest.Y]) {
			s.charTxs[i].destCh = cells[charTx.dest.Y][charTx.dest.X].Ch
		}
	}
}

func (s *risingChars) processCell(
	x, y int, frame, total int, cells [][]term.Cell,
) {
	if y >= len(cells) || x >= len(cells[y]) {
		return
	}

	fg := cells[y][x].Fg
	bg := cells[y][x].Bg
	char := cells[y][x].Ch

	w := len(cells[0])
	h := len(cells)

	// this is run by all cells and they check if they are either the source or
	// the current value that is being interpolated
	const kNoiseScale = 3.0
	for _, charTx := range s.charTxs {
		cellIsTravelling := x == charTx.curr.X && y == charTx.curr.Y
		cellIsDestination := x == charTx.dest.X && y == charTx.dest.Y
		if cellIsTravelling {
			char = charTx.destCh
			n := noiseSimplex(vec2(
				kNoiseScale*float(charTx.dest.X)/float(w),
				kNoiseScale*float(charTx.dest.Y)/float(h),
			))
			fg = vecToCol(
				mix3D(
					colToVec(fg, s.defaultAttr.Fg),
					vec3FromScalar(255.0*(0.5+0.5*n)),
					0.5+0.5*sin(s.CharFgFlickerSpeed*math.Pi*(float(frame)/float(total))+
						float(charTx.dest.X+charTx.dest.Y))),
			)
		} else if cellIsDestination {
			char = ' '
		}
	}

	cells[y][x].Fg = fg
	cells[y][x].Bg = bg
	cells[y][x].Ch = char

	// A cell with a character to be visible must have width set.
	if char != rune(0) {
		cells[y][x].Width = 1
	}
}

// charTx represents a character moving around screen from one point to another
// (translation, viz. Tx)
type charTx struct {
	// The destination of this character to land on.
	dest *term.Coordinates
	// The actual character at the destination at the very moment, that way if
	// it changes it reflects into the moving char too.
	destCh rune
	// The point the traveling character is currently at.
	curr *term.Coordinates
	// The original position it departed from.
	orig *term.Coordinates
	// How many frames to offset this travel from orig to dest. This breaks up
	// the progress of the concurrent travelers so that all characters don't
	// walk the same percentage along the path at once.
	frameOffset int
}
