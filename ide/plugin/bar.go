// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package plugin

import (
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/template"
	tcomponent "unstable.build/go-tui/component"
)

// BarComponentType is one of the many supported
// components by the status bar.
type BarComponentType uint8

// BarComponent represents a component to be
// rendered by the status bar.
type BarComponent struct {
	Type       BarComponentType
	Template   string
	Attributes term.Attributes
}

const (
	// BarCommand is the full command and args of the running program.
	BarCommand BarComponentType = iota
	// BarStatusIcon is the status of the program as an icon.
	BarStatusIcon
	// BarExitStatus is the exit status after it is done running.
	BarExitStatus
	// BarElapsed is the time the program has been running.
	BarElapsed
	// BarAlignRight indicates that the following components need to be aligned right.
	BarAlignRight
	// BarAlignCenter indicates that the following components need to be centered.
	BarAlignCenter
)

// BarConfig holds configuration for the status bar created
// by WithBar.
type BarConfig struct {
	Layout                []BarComponent
	BackgroundColor       tcell.Color
	StatusAnimationFrames []string
	StatusErrorIcon       string
	StatusErrorColor      tcell.Color
	StatusSuccessIcon     string
	StatusSuccessColor    tcell.Color
	AlignBottom           bool
}

// DefaultBarConfig returns the default plugin top bar configuration.
func DefaultBarConfig() BarConfig {
	defaultAnimationFrames, _ := tcomponent.SpinningSquareAnimationFrames()
	// this is backwards compatible with implementation before dynamic layout
	// so we don't need to refactor tests.
	return BarConfig{
		Layout: []BarComponent{
			{
				Template: " %s ",
				Type:     BarStatusIcon,
			},
			{Type: BarAlignCenter},
			{
				Type:     BarCommand,
				Template: "%s",
			},
			{Type: BarAlignRight},
			{
				Type:     BarElapsed,
				Template: "%s",
			},
		},
		StatusErrorIcon:       "▀",
		StatusErrorColor:      tcell.ColorRed,
		StatusSuccessIcon:     "▀",
		StatusSuccessColor:    tcell.ColorGreen,
		StatusAnimationFrames: defaultAnimationFrames,
	}
}

type pluginHandlerBar struct {
	BarConfig

	ticker    *time.Ticker
	mu        sync.Mutex
	done      bool
	doneErr   error
	doneTime  time.Time
	startTime time.Time
	width     int
	height    int

	runningPrecision time.Duration
	donePrecision    time.Duration

	barLeft   component.Virtual[component.Floating]
	barCenter component.Virtual[component.Floating]
	barRight  component.Virtual[component.Floating]

	animation         *tcomponent.Animation
	status            *component.FloatingReference
	statusTemplate    BarComponent
	statusAlignment   component.Alignment
	elapsed           *component.FloatingReference
	elapsedTemplate   BarComponent
	elapsedAlignment  component.Alignment
	command           *component.FloatingReference
	commandTemplate   BarComponent
	commandAlignment  component.Alignment
	exitCode          *component.FloatingReference
	exitCodeTemplate  BarComponent
	exitCodeAlignment component.Alignment
}

const (
	barHeight    = 1
	animationFPS = 10
)

func newPluginHandlerBar(
	commandAndArgs string, interrupter term.Interrupter,
	config BarConfig,
) *pluginHandlerBar {
	ret := new(pluginHandlerBar)
	ret.BarConfig = config
	ret.runningPrecision = time.Second
	ret.donePrecision = time.Millisecond

	ret.initLayout(interrupter)

	ret.mu.Lock()
	defer ret.mu.Unlock()

	ret.buildCommandAndArgs(commandAndArgs)
	ret.rebuildStatus()
	ret.startTime = time.Now()
	ret.rebuildElapsed()
	ret.resize()
	cadence := time.Duration(int(time.Second) / animationFPS)
	ret.ticker = time.NewTicker(cadence)
	return ret
}

func (e *pluginHandlerBar) initElapsedTicker() {
	for {
		_, ok := <-e.ticker.C
		e.mu.Lock()
		e.rebuildElapsed()
		e.resize()
		e.mu.Unlock()
		if !ok {
			return
		}
	}
}

func (e *pluginHandlerBar) initLayout(interrupter term.Interrupter) {
	var barCenter []component.Floating
	var barLeft []component.Floating
	var barRight []component.Floating
	alignment := component.AlignmentLeft
	for _, comp := range e.Layout {
		var toappend component.Floating
		switch comp.Type {
		case BarAlignRight:
			alignment = component.AlignmentRight
			continue
		case BarAlignCenter:
			alignment = component.AlignmentHorizontallyCentered
			continue
		case BarStatusIcon:
			var seq []int
			for i := range e.StatusAnimationFrames {
				seq = append(seq, i)
			}
			e.statusTemplate = comp
			e.animation = tcomponent.NewAnimation(interrupter, e.StatusAnimationFrames,
				seq, animationFPS)
			e.animation.SetAttr(e.statusTemplate.Attributes)
			e.statusAlignment = alignment
			e.status = component.NewFloatingReference(nil)
			toappend = e.status
		case BarCommand:
			e.commandTemplate = comp
			e.commandAlignment = alignment
			e.command = component.NewFloatingReference(nil)
			toappend = e.command
		case BarExitStatus:
			e.exitCodeTemplate = comp
			e.exitCodeAlignment = alignment
			e.exitCode = component.NewFloatingReference(nil)
			toappend = e.exitCode
		case BarElapsed:
			e.elapsedTemplate = comp
			e.elapsedAlignment = alignment
			e.elapsed = component.NewFloatingReference(nil)
			toappend = e.elapsed
		}
		switch alignment {
		case component.AlignmentLeft:
			barLeft = append(barLeft, toappend)
		case component.AlignmentRight:
			barRight = append(barRight, toappend)
		case component.AlignmentHorizontallyCentered:
			barCenter = append(barCenter, toappend)
		}
	}

	e.barLeft.C = component.Inline(barLeft, component.AlignmentLeft)
	e.barCenter.C = component.Inline(barCenter, component.AlignmentHorizontallyCentered)
	e.barRight.C = component.Inline(barRight, component.AlignmentRight)
}

func (e *pluginHandlerBar) buildCommandAndArgs(commandAndArgs string) {
	if e.command == nil {
		return
	}

	components := template.Build(e.commandTemplate.Template,
		commandAndArgs, term.Attributes{}, e.commandTemplate.Attributes, e.BackgroundColor)
	e.command.Init(component.Inline(components, e.commandAlignment))
}

func (e *pluginHandlerBar) rebuildStatus() {
	if e.status == nil {
		return
	}

	done := e.done
	doneErr := e.doneErr
	if done {
		var icon string
		var color tcell.Color
		if doneErr != nil {
			icon = e.StatusErrorIcon
			color = e.StatusErrorColor
		} else {
			icon = e.StatusSuccessIcon
			color = e.StatusSuccessColor
		}
		components := template.Build(e.statusTemplate.Template,
			icon, term.Attributes{Fg: color, Bg: e.statusTemplate.Attributes.Bg},
			e.statusTemplate.Attributes, e.BackgroundColor)
		e.status.Init(component.Inline(components, e.statusAlignment))
	} else {
		// use something that would have been parsed by template,
		// so we know it won't have conflicts
		const knownId = "{{}}"
		components := template.Build(e.statusTemplate.Template,
			knownId, term.Attributes{}, e.statusTemplate.Attributes, e.BackgroundColor)
		for i, comp := range components {
			str, ok := comp.(component.String)
			if !ok {
				continue
			}
			// insert animation where knownId would go
			compstr := str.String()
			strcfg := str.Config()
			if !strings.Contains(compstr, knownId) {
				continue
			}

			animation := component.StaticFloating(e.animation, 1, 1)
			before, after, _ := strings.Cut(compstr, knownId)
			if before == "" && after == "" {
				components[i] = animation
				break
			}
			if before == "" {
				components = append(components, nil)
				copy(components[i+1:], components[i:])
				components[i] = animation
				components[i+1] = component.NewStringWithConfig(after, strcfg)
				break
			}
			if after == "" {
				if i == len(components)-1 {
					components[i] = component.NewStringWithConfig(before, strcfg)
					components = append(components, animation)
					break
				}
				components = append(components, nil)
				copy(components[i+2:], components[i+1:])
				components[i] = component.NewStringWithConfig(before, strcfg)
				components[i+1] = animation
				break
			}
			if i == len(components)-1 {
				components[i] = component.NewStringWithConfig(before, strcfg)
				components = append(components, animation,
					component.NewStringWithConfig(after, strcfg))
				break
			}
			// both have components
			components = append(components, nil, nil)
			copy(components[i+3:], components[i+1:])
			components[i] = component.NewStringWithConfig(before, strcfg)
			components[i+1] = animation
			components[i+2] = component.NewStringWithConfig(after, strcfg)
			break
		}
		e.status.Init(component.Inline(components, e.statusAlignment))
	}
}

func (e *pluginHandlerBar) rebuildExitCode(err error) {
	if e.exitCode == nil {
		return
	}
	var status string
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		status = exitError.ProcessState.String()
	} else if err != nil {
		status = "non-zero"
	} else {
		status = "ok"
	}

	components := template.Build(e.exitCodeTemplate.Template,
		status, term.Attributes{}, e.exitCodeTemplate.Attributes, e.BackgroundColor)
	e.exitCode.Init(component.Inline(components, e.exitCodeAlignment))
}

func (e *pluginHandlerBar) rebuildElapsed() {
	if e.elapsed == nil {
		return
	}

	done := e.done
	doneTime := e.doneTime
	startTime := e.startTime

	var str string
	if done {
		str = doneTime.Sub(startTime).Truncate(e.donePrecision).String()
	} else {
		str = time.Since(startTime).Truncate(e.runningPrecision).String()
	}

	components := template.Build(e.elapsedTemplate.Template,
		str, term.Attributes{}, e.elapsedTemplate.Attributes, e.BackgroundColor)
	e.elapsed.Init(component.Inline(components, e.elapsedAlignment))
}

func (b *pluginHandlerBar) resize() {
	barHeight := 1
	if b.height < 1 {
		barHeight = 0
	}
	barLeftWidth, _ := b.barLeft.C.Dimensions()
	barRightWidth, _ := b.barRight.C.Dimensions()
	barCenterWidth, _ := b.barCenter.C.Dimensions()

	// first remove center
	if barLeftWidth+barRightWidth+barCenterWidth > b.width {
		barCenterWidth = 0
	}
	// then remove right
	if barLeftWidth+barRightWidth+barCenterWidth > b.width {
		barRightWidth = 0
	}

	b.barLeft.Resize(barLeftWidth, barHeight)

	// for the center bar to be centered, we have to do some magic
	centerOffset := term.Coordinates{X: (b.width - barCenterWidth) / 2}
	b.barCenter.Move(centerOffset)
	b.barCenter.Resize(barCenterWidth, barHeight)

	rightOffset := term.Coordinates{X: b.width - barRightWidth}
	b.barRight.Move(rightOffset)
	b.barRight.Resize(barRightWidth, barHeight)
}

func (e *pluginHandlerBar) Draw(w term.Writer) {
	if e.BackgroundColor != tcell.ColorDefault && e.barCenter.Height() != 0 {
		attrs := term.Attributes{Bg: e.BackgroundColor}
		for x := range e.width {
			w.UnionAttributes(term.Coordinates{Y: e.height - 1, X: x}, attrs)
		}
	}

	e.mu.Lock()
	e.barLeft.Draw(w)
	e.barCenter.Draw(w)
	e.barRight.Draw(w)
	e.mu.Unlock()

	attrs := term.Attributes{Attrs: term.AttrNegativeVerticalRenderOffset}
	if e.AlignBottom {
		attrs.Attrs = term.AttrVerticalRenderOffset
	}

	for y := range barHeight {
		for x := range e.width {
			w.UnionAttributes(term.Coordinates{Y: y, X: x},
				attrs)
		}
	}
}

func (e *pluginHandlerBar) Resize(width, height int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.width = width
	e.height = height
	e.resize()
}

func (e *pluginHandlerBar) Close() error {
	e.ticker.Stop()
	if e.animation != nil {
		return e.animation.Close()
	}
	return nil
}

func (e *pluginHandlerBar) setDone(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.done {
		return
	}

	e.ticker.Stop()

	e.done = true
	e.doneErr = err
	e.doneTime = time.Now()

	e.rebuildStatus()
	e.rebuildExitCode(err)
	e.rebuildElapsed()
	e.resize()
}
