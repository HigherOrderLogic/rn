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

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/shader/shaderutils"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Bomb shows an expansive ring expanding from the center outwards displacing
// the characters under the ring outwards making it look like magnifier.
func Bomb(params BombParams, defaultAttrs term.Attributes) Shader {
	b := &bomb{
		BombParams:  params,
		ringScale:   1.0 / (1 - params.RingStart - (1 - params.RingEnd)),
		defaultAttr: defaultAttrs,
	}

	return b
}

// BombParams defines the parameters used by the Bomb shader.
type BombParams struct {
	RingCol         tcell.Color
	RingStart       float64
	RingEnd         float64
	CharBandwidth   float64
	ShiftCharacters bool
}

// DefaultBombParams return a set of sane BombParams.
func DefaultBombParams() BombParams {
	return BombParams{
		RingCol:         tcell.NewRGBColor(255, 0, 0),
		RingStart:       0.5,
		RingEnd:         0.8,
		CharBandwidth:   0.3,
		ShiftCharacters: true,
	}
}

type bomb struct {
	BombParams
	defaultAttr term.Attributes
	ringScale   float64
}

func (s *bomb) Shade(epoch, total int, cells [][]term.Cell) {
	if epoch >= total {
		return
	}

	cAR := 1 / asciiart.HeightToWidthCellAspectRatio
	cARSq := cAR * cAR

	cols := len(cells)
	rows := len(cells[0])

	wh := float64(rows) / 2.0
	hh := float64(cols) / 2.0

	t := (float64(epoch) / float64(total-1))

	// rL would be squared hypothenuse (a² = b² + c²)
	rl := float64(wh*wh) + float64(hh*hh)/cARSq
	rl *= 1.5

	for y, row := range cells {
		for x := range row {
			xx := float64(x)
			yy := float64(y)

			vx := (xx - wh)
			vy := (yy - hh)

			// vL would be squared hypothenuse (a² = b² + c²)
			vl := float64(vx*vx + vy*vy/cARSq)

			// no need to get the real hypothenuses if we only want to compare
			if vl <= (rl * t) {
				// shade the ring using a falloff
				gr := math.Max(0, math.Min(1, (vl/rl)+(2*((1-t)-0.5))))
				res := math.Max(0, math.Min(1, s.ringScale*(gr-(1-s.RingEnd))))
				bg := shaderutils.InterpolateColor(res, cells[y][x].Bg, s.RingCol, s.defaultAttr.Bg)
				cells[y][x].Bg = bg

				// show text in the inner circle from certain threshold
				if s.ShiftCharacters && vl >= (rl*(t-s.CharBandwidth)) {
					i := int(wh + vx)
					j := int(hh + vy)

					// step one unit towards the vector direction. We would add
					// the unit vector int((vx-w_h)/(vx-w_h)) but to avoid the
					// expensive division, since we are stepping only one unit
					// we are only interested in the sign of that vector
					vx_wh := int(vx - wh)
					if vx_wh != 0 {
						if vx_wh > 0 {
							i = i + 1
						} else {
							i = i - 1
						}
					}
					vy_hh := int(vy - hh)
					if vy_hh != 0 {
						if vy_hh > 0 {
							j = j - 1
						} else {
							j = j + 1
						}
					}

					j = int(math.Max(0, math.Min(float64(cols-1), float64(j))))
					if cells[j] == nil {
						continue
					}

					i = int(math.Max(0, math.Min(float64(rows-1), float64(i))))
					if i >= len(cells[j]) {
						continue
					}

					cells[y][x].Ch = cells[j][i].Ch
					cells[y][x].Width = 1
				}
			}
		}
	}
}
