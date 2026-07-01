// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2026 Unstable Build, All Rights Reserved.
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

package main

import (
	"time"

	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/component/shader"
)

const (
	// loadingShaderFPS is the cadence at which the shader played
	// while a workspace is loading is animated.
	loadingShaderFPS = 30

	// loadingShaderFadeDuration is how long the gray-fade animation takes
	// to reach full desaturation. The loading shader keeps running past
	// this point, holding at full desaturation, until the workspace finishes
	// loading and the open shader takes over.
	loadingShaderFadeDuration = 1 * time.Second

	// loadingShaderDuration is the lifetime of the underlying
	// [shader.Component] hosting the loading shader. It is intentionally
	// longer than loadingShaderFadeDuration so the Component does not
	// auto-expire mid-load: the loading shader holds at full desaturation
	// past the fade until [ide.WithLoadingShader]'s consumer swaps in the
	// open shader. 10s is an upper bound on plausible addWorkspace
	// latency; the shader is cancelled cleanly when the load completes.
	loadingShaderDuration = 10 * time.Second

	shutdownShaderDuration = 30 * time.Second

	// openShaderFPS is the cadence at which the shader played when a
	// workspace finishes loading is animated.
	openShaderFPS = 60

	// openShaderDuration is the lifetime of the open shader's
	// [shader.Component]. The first portion runs the burn sweep at
	// full intensity; the last [openShaderFadeOutDuration] fades the
	// burn output back to the live workspace cells via a cross-fade
	// transition with [shader.Nop] so the new content emerges
	// smoothly instead of snapping into place.
	openShaderDuration = 800 * time.Millisecond

	// openShaderFadeOutDuration is how long the trailing cross-fade
	// from the burn output to the live cells lasts. Must be strictly
	// less than [openShaderDuration]; the burn sweep itself plays
	// during the leading openShaderDuration-openShaderFadeOutDuration.
	openShaderFadeOutDuration = 100 * time.Millisecond
)

// loadingShader returns the shader played while a workspace is
// loading. It slowly fades the current screen to gray; the runner
// then swaps in the open shader which starts from the same tint to
// keep the two effects visually continuous. If loading completes
// before the fade does, the runner ends it early and reads
// [shader.GrayFadeShader.Progress] so the open shader matches the
// partial fade state.
func loadingShader(defaultAttr term.Attributes) shader.Shader {
	params := shader.DefaultGrayFadeParams()
	params.FadeFrames = int(loadingShaderFadeDuration / time.Second * loadingShaderFPS)
	return shader.GrayFade(params, defaultAttr)
}

// openShader returns the shader played when a workspace finishes
// loading. A quick "burn" sweep ignites the canvas and resolves
// back to the original content, acknowledging the transition
// without slowing the user down.
func openShader(defaultAttr term.Attributes) shader.Shader {
	params := shader.DefaultBurnParams()
	params.BurnGradient = []term.Color{
		term.ColorWhite,
		term.ColorSilver,
		term.ColorYellow,
		term.ColorRed,
		term.ColorMaroon,
	}
	params.BurnSymbols = []rune{
		'░', '▒', '▓', '█', '█', '▓', '▒', '░',
	}
	params.BurnDuration = 0.05
	params.SmokeChance = 0.01
	params.SmokeSymbols = []rune{
		'.', '▀', '▄', 'o',
	}
	params.SmokeRise = 0.99
	params.SmokeMaxRise = 20

	burn := shader.Burn(params, defaultAttr)

	// Cross-fade the trailing portion of the open shader into the
	// live cells. The change point sits exactly between the burn
	// finish and the shader's end so the overlap is the full
	// openShaderFadeOutDuration window centred on that boundary —
	// half blending burn out and half blending the workspace in. A
	// long fade keeps the workspace from "popping" into view at the
	// end of the burn.
	fadeOutPerc := float64(openShaderFadeOutDuration) / float64(openShaderDuration)
	wrapped := shader.TransitionCrossFade(
		shader.TransitionCrossFadeParams{
			ChangeAtPerc: 1.0 - fadeOutPerc/2.0,
			OverlapPerc:  fadeOutPerc,
		},
		defaultAttr,
		burn,
		shader.Nop(),
	)
	// Expose the burn's snapshot/desaturation hooks through the
	// cross-fade wrapper so the loading→open chain continues to
	// work: shaderRunner.stopLoading() type-asserts the returned
	// shader for these interfaces.
	return &openShaderShader{Shader: wrapped, burn: burn}
}

// openShaderShader pairs a cross-fade wrapper with the underlying
// burn so the runner's pre-shader hooks (initial cells, initial
// desaturation) still reach the burn after wrapping. The cross-fade
// itself is opaque to those hooks because it has no concept of an
// initial snapshot.
type openShaderShader struct {
	shader.Shader
	burn shader.Shader
}

// SetInitialCells forwards to the wrapped burn so the open shader
// keeps the pre-switch screen as its base layer.
func (s *openShaderShader) SetInitialCells(cells [][]term.Cell) {
	if sh, ok := s.burn.(interface {
		SetInitialCells([][]term.Cell)
	}); ok {
		sh.SetInitialCells(cells)
	}
}

// SetInitialDesaturation forwards to the wrapped burn so the open
// shader starts from the loading shader's terminal desaturation.
func (s *openShaderShader) SetInitialDesaturation(amount float64) {
	if sh, ok := s.burn.(interface {
		SetInitialDesaturation(amount float64)
	}); ok {
		sh.SetInitialDesaturation(amount)
	}
}

func shutdownShader(defaultAttr term.Attributes) shader.Shader {
	params := shader.DefaultGrayFadeParams()
	params.FadeFrames = shutdownShaderFPS
	return shader.GrayFade(params, defaultAttr)
}
