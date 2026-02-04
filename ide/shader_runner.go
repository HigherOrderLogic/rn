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

package ide

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/component/shader/glslshader"
	"unstable.build/go-tui/component/shader/timeshader"
)

const (
	defaultShaderFPS      = 30
	defaultShaderDuration = 1 * time.Second
)

type shutdownShaderConfig struct {
	shader   func(term.Attributes) shader.Shader
	fps      int
	duration time.Duration
}

func nopShutdownShaderConfig() shutdownShaderConfig {
	return shutdownShaderConfig{
		shader:   func(_ term.Attributes) shader.Shader { return shader.Nop() },
		fps:      defaultShaderFPS,
		duration: 100 * time.Millisecond,
	}
}

// used as the root tui.Handler to dynamically run shaders
type shaderRunner struct {
	tui.Handler
	interrupter       term.Interrupter
	shader            *shader.Component
	defAttr           term.Attributes
	fc                component.FrameCharSet
	width, height     int
	shutdownShaderCfg shutdownShaderConfig
}

func (r *shaderRunner) HandleCommand(ctx context.Context, cmd textapi.Command) (
	err error,
) {
	if len(cmd.Args) == 0 {
		err = errors.New("expected at least one argument with the shader name")
		return
	}

	d := defaultShaderDuration
	if len(cmd.Args) > 1 {
		d, err = time.ParseDuration(cmd.Args[1])
		if err != nil {
			err = fmt.Errorf("parse duration: %v", err)
			return
		}
	}

	fps := defaultShaderFPS
	if len(cmd.Args) > 2 {
		fps, err = strconv.Atoi(cmd.Args[2])
		if err != nil {
			err = fmt.Errorf("parse fps: %v", err)
			return
		}
	}

	const (
		fadeInPerc  = 0.1
		fadeOutPerc = 0.1
	)
	var s shader.Shader
	// parse shader and duration
	// TODO: Review and leave only useful shaders. This at the moment is a review showroom!
	switch cmd.Args[0] {
	case "blaze":
		s = wrapShaderCrossFadeInOut(
			glslshader.Blaze(glslshader.DefaultBlazeParams(), float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "blazeBlue":
		params := glslshader.DefaultBlazeParams()
		params.SwapRedBlue = true
		s = wrapShaderCrossFadeInOut(
			glslshader.Blaze(params, float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "bomb":
		s = shader.Bomb(shader.DefaultBombParams(), r.defAttr)
	case "embers":
		s = wrapShaderCrossFadeInOut(
			glslshader.Embers(glslshader.DefaultEmbersParams(), r.defAttr, float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "fade":
		s = shader.Fade(r.defAttr)
	case "incendium":
		s = wrapShaderCrossFadeInOut(
			glslshader.Incendium(glslshader.DefaultIncendiumParams(), r.defAttr, float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "incendiumReversed":
		s = wrapShaderCrossFadeInOut(
			timeshader.Reverse(glslshader.Incendium(glslshader.DefaultIncendiumParams(), r.defAttr, float64(fps))),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "incendiumPingPong":
		s = wrapShaderCrossFadeInOut(
			timeshader.PingPong(glslshader.Incendium(
				glslshader.DefaultIncendiumParams(), r.defAttr, float64(fps))),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "flames":
		s = wrapShaderCrossFadeInOut(
			glslshader.Flames(glslshader.DefaultFlamesParams(), r.defAttr, float64(fps)),
			0.0, fadeOutPerc,
			r.defAttr,
		)
	case "flamesA":
		s = wrapShaderCrossFadeInOut(
			glslshader.Flames(glslshader.FlamesPresetAShape(), r.defAttr, float64(fps)),
			0.0, fadeOutPerc,
			r.defAttr,
		)
	case "flamesV":
		s = wrapShaderCrossFadeInOut(
			glslshader.Flames(glslshader.FlamesPresetVShape(), r.defAttr, float64(fps)),
			0.0, fadeOutPerc,
			r.defAttr,
		)
	case "inferno":
		s = wrapShaderCrossFadeInOut(
			glslshader.Inferno(glslshader.DefaultInfernoParams(), float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "infernoBlue":
		params := glslshader.DefaultInfernoParams()
		params.SwapRedBlue = true
		s = wrapShaderCrossFadeInOut(
			glslshader.Inferno(params, float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "burningOnlyRedFlamesLightsOn":
		params := glslshader.BurningPresetGentleOnlyFlames(false, r.fc)
		s = glslshader.Burning(params, r.defAttr, d, float64(fps))
	case "burningOnlyRedFlamesLightsOff":
		params := glslshader.BurningPresetGentleOnlyFlames(true, r.fc)
		s = glslshader.Burning(params, r.defAttr, d, float64(fps))
	case "burningOnlyBlueFlamesLightsOn":
		params := glslshader.BurningPresetGentleOnlyFlames(false, r.fc)
		params.Colors.SwapRedBlue = true
		s = glslshader.Burning(params, r.defAttr, d, float64(fps))
	case "burningOnlyBlueFlamesLightsOff":
		params := glslshader.BurningPresetGentleOnlyFlames(true, r.fc)
		params.Colors.SwapRedBlue = true
		s = glslshader.Burning(params, r.defAttr, d, float64(fps))
	case "noise":
		s = wrapShaderCrossFadeInOut(
			glslshader.Noise(glslshader.DefaultNoiseParams(), float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "nop":
		s = shader.Nop()
	case "risingChars":
		s = wrapShaderCrossFadeInOut(
			glslshader.RisingChars(glslshader.DefaultRisingCharsParams(), r.defAttr),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	case "trippy":
		s = wrapShaderCrossFadeInOut(
			glslshader.Trippy(glslshader.DefaultTrippyParams(), float64(fps)),
			fadeInPerc, fadeOutPerc,
			r.defAttr,
		)
	default:
		err = errors.New("expected one of the available shaders")
		return
	}

	r.runShader(s, fps, d)
	return
}

func (r *shaderRunner) Complete(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	if len(cmd.Args) <= 1 {
		return iterator.FromSlice([]string{
			// TODO: Review and leave only useful shaders. This at the moment is a review showroom!
			"blaze",
			"blazeBlue",
			"bomb",
			"embers",
			"fade",
			"incendium",
			"incendiumReversed",
			"incendiumPingPong",
			"flames",
			"flamesA",
			"flamesV",
			"inferno",
			"infernoBlue",
			"burningOnlyRedFlamesLightsOn",
			"burningOnlyRedFlamesLightsOff",
			"burningOnlyBlueFlamesLightsOn",
			"burningOnlyBlueFlamesLightsOff",
			"noise",
			"nop",
			"risingChars",
			"trippy",
		}), "", nil
	}
	return iterator.Empty[string](), "", nil
}

func (r *shaderRunner) init(
	root tui.Handler,
	interrupter term.Interrupter,
	defAttr term.Attributes,
	shutdownShaderCfg shutdownShaderConfig,
	fc component.FrameCharSet,
) {
	r.Handler = root
	r.interrupter = interrupter
	r.defAttr = defAttr
	r.shutdownShaderCfg = shutdownShaderCfg
	r.fc = fc

	// Initialize zero shader so we can treat field always as non-nil.
	//
	// Some duration > 0 is passed so the r.shader.done stays true,
	// that's something that happens after rendering any shader and
	// allows us to only use one variable (c.done) to determine if a
	// shader is running or not.
	r.shader = shader.New(r.Handler, shader.Nop(), r.interrupter, defaultShaderFPS, 100*time.Millisecond)
}

func (r *shaderRunner) runShader(s shader.Shader, fps int, duration time.Duration) {
	_ = r.shader.Close()
	r.shader = shader.New(r.Handler, s, r.interrupter, fps, duration)
	r.shader.Resize(r.width, r.height)
}

func (r *shaderRunner) cancel() {
	// Some duration > 0 is passed so the r.shader.done stays true,
	// that's something that happens after rendering any shader and
	// allows us to only use one variable (c.done) to determine if a
	// shader is running or not.
	r.runShader(shader.Nop(), defaultShaderFPS, 100*time.Millisecond)
}

func (r *shaderRunner) runShutdownShader() {
	// If no shutdown shader is set this will run a shader.Nop(). If a shader was
	// running when runShutdownShader is called it will appear as canceling it.
	r.runShader(
		r.shutdownShaderCfg.shader(r.defAttr),
		r.shutdownShaderCfg.fps,
		r.shutdownShaderCfg.duration)
}

func (r *shaderRunner) Draw(w term.Writer) {
	r.shader.Draw(w)
}

func (r *shaderRunner) Resize(width, height int) {
	r.width = width
	r.height = height
	r.shader.Resize(width, height)
}

func (r *shaderRunner) Close() error {
	return r.shader.Close()
}

// Adds fade in and out to a given shader.
//
// fadeInPerc specifies the percentage of the whole animation you want fading
// in (e.g. 0.1 means 10% of the beginning frames of shader will be
// transition), and similarly with fadeOutPerc (10% would mean at 90% of the
// animation it starts fading out).
func wrapShaderCrossFadeInOut(
	sh shader.Shader, fadeInPerc, fadeOutPerc float64,
	defaultAttr term.Attributes,
) shader.Shader {
	return shader.TransitionCrossFade(
		shader.TransitionCrossFadeParams{
			ChangeAtPerc: fadeInPerc,
			OverlapPerc:  fadeInPerc,
		},
		defaultAttr,
		shader.Nop(),
		shader.TransitionCrossFade(
			shader.TransitionCrossFadeParams{
				ChangeAtPerc: 1.0 - fadeOutPerc,
				OverlapPerc:  fadeOutPerc,
			},
			defaultAttr,
			sh,
			shader.Nop(),
		),
	)
}
