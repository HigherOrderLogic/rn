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

	"unstable.build/go-tui/component/shader"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Incendium creates flames that turn screen into embers and ashes while sparks
// glitter rising while dragging some characters along the way.
//
// This shader is not a new creation but a combination of the following ones:
// - glslshader.Flames
// - glslshader.Embers
// - glslshader.RisingChars
func Incendium(
	params IncendiumParams, defaultAttr term.Attributes, fps float,
) shader.Shader {
	shFlames := Flames(params.Flames, defaultAttr, fps).(*flames)
	shEmbers := Embers(params.Embers, defaultAttr, fps).(*embers)
	shRisingChars := RisingChars(params.RisingChars, defaultAttr).(*risingChars)
	return &incendium{
		IncendiumParams:   params,
		fps:               fps,
		shaderFlames:      shFlames,
		shaderEmbers:      shEmbers,
		shaderRisingChars: shRisingChars,
	}
}

// IncendiumParams allows you to customize the Incendium effect.
//
// Most parameters are in the normalized range 0-1. Exceeding the range is
// allowed in most of the cases (it's specified in the docstring).
type IncendiumParams struct {
	ExtraFlames float // 0..1 brings out more of the flames visual component
	Flames      FlamesParams
	Embers      EmbersParams
	RisingChars RisingCharsParams
}

// DefaultIncendiumParams gives you a set of params that exhibits the Incendium
// effect properly. It's a good starting point to customizing on top of with
// parameter overrides.
func DefaultIncendiumParams() IncendiumParams {
	flamesParams := DefaultFlamesParams()
	embersParams := DefaultEmbersParams()
	risingChars := DefaultRisingCharsParams()

	return IncendiumParams{
		Flames:      flamesParams,
		Embers:      embersParams,
		RisingChars: risingChars,
	}
}

type incendium struct {
	IncendiumParams
	fps               float
	shaderFlames      *flames
	shaderEmbers      *embers
	shaderRisingChars *risingChars
}

func (s *incendium) Shade(frame, total int, cells [][]term.Cell) {
	if frame < 0 || frame >= total {
		return
	}

	// NOTE: Why not simply running the shade of each inner shader? Here's why:
	//
	// Instead of running shaders sequentially respecting each range:
	//
	//	s.shaderFlames.Shade(frame, total, cells)
	//	s.shaderEmbers.Shade(frame, total, cells)
	//	s.shaderRisingChars.Shade(frame, total, cells)
	//
	//	... or instead of using a Transition shader like this:
	//
	//	s.timeline := shader.TransitionCrossFade(s.shaderFlames, s.shaderEmbers, ...)
	//
	// ... the design decision for the Incendium shader is to be aware of what
	// shader to render on a per-cell basis. This allows starting the second
	// animation only if the first has finished. It produces nicer-looking
	// result (since it blends nicer) and it's more performant (since you don't
	// throw renders away by painting on top of existing paint).

	// Forget about Flames shader if all are done with the Flames animation.
	s.shaderFlames.analizePlayingDirection(frame)
	wouldRenderNoFlames := s.shaderFlames.wouldRenderNothing()
	if !wouldRenderNoFlames {
		s.shaderFlames.ensureSliceFieldsCap(frame, total, cells)
		s.shaderFlames.jitterActivations(frame, total, cells)
	}

	s.shaderRisingChars.computeTranslations(frame, total, cells)

	for y := 0; y < len(cells); y++ {
		for x := 0; x < len(cells[0]); x++ {
			// Don't bother rendering Flames shader if no cells would be changed.
			if !wouldRenderNoFlames {
				s.shaderFlames.processCell(x, y, frame, total+int(2*s.ExtraFlames*float(total)), cells)
			}

			// Do not render embers if unless the incendium passed over the cells.
			if s.shaderFlames.flamesCellStatuses[y][x] != flamesPassed {
				continue
			}

			offsets := s.shaderFlames.flamesOffsets[y][x]
			flamesDuration := len(s.Flames.Chars)
			offsetFrame := frame - int(math.Round(offsets)) - flamesDuration
			s.shaderEmbers.processCell(x, y, offsetFrame, total, cells)

			// Have some characters rising up from the screen as it were light
			// ashes following the flames heat.
			s.shaderRisingChars.processCell(x, y, offsetFrame, total, cells)
		}
	}
}
