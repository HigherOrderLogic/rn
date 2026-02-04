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

package notifications

import (
	"math"
	"time"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var _ component.Responsive = (*notificationComp)(nil)

type notificationComp struct {
	component.Responsive
	cfg                    Config
	cancel                 func()
	width                  int
	height                 int
	duration               time.Duration
	manualProgress         int64
	manualProgressTotal    int64
	end                    time.Time
	pausedAt               time.Time
	progressCellStart      term.Cell
	progressCellEnd        term.Cell
	progressCellCurrent    term.Cell
	progressCellCurrentTip term.Cell
	progressCellRemain     term.Cell
}

func newString(cfg Config, msg string) component.Responsive {
	strConfig := component.StringResponsiveConfig{
		NoSplitWords: true,
		StringConfig: component.StringConfig{
			Alignment:            component.AlignmentCentered,
			BackgroundRune:       ' ',
			Attributes:           cfg.Attributes,
			BackgroundAttributes: cfg.BackgroundAttributes,
			PaddingHorizontal:    2,
			FrameCharSet:         cfg.FrameCharSet,
			MinWidth:             cfg.Width,
		},
	}
	return component.NewResponsiveString(msg, strConfig)
}

func newNotification(
	level Level, msg string, cfg Config, duration time.Duration,
	cancel func(),
) component.Responsive {

	// template for each progress rune
	var progressCell term.Cell
	switch level {
	case LevelInfo:
		progressCell.Attrs = cfg.ColorInfo.Attrs
		progressCell.Fg = cfg.ColorInfo.Fg
		progressCell.Bg = cfg.ColorInfo.Bg
	case LevelSuccess:
		progressCell.Attrs = cfg.ColorSuccess.Attrs
		progressCell.Fg = cfg.ColorSuccess.Fg
		progressCell.Bg = cfg.ColorSuccess.Bg
	case LevelWarn:
		progressCell.Attrs = cfg.ColorWarning.Attrs
		progressCell.Fg = cfg.ColorWarning.Fg
		progressCell.Bg = cfg.ColorWarning.Bg
	case LevelError:
		progressCell.Attrs = cfg.ColorError.Attrs
		progressCell.Fg = cfg.ColorError.Fg
		progressCell.Bg = cfg.ColorError.Bg
	default:
		panic("unknown level")
	}
	progressCell.Width = 1

	progressCellStart := progressCell
	progressCellStart.Ch = cfg.ProgressRunes.Start
	progressCellEnd := progressCell
	progressCellEnd.Ch = cfg.ProgressRunes.End
	progressCellRemain := progressCell
	progressCellRemain.Ch = cfg.ProgressRunes.Remain
	progressCellCurrent := progressCell
	progressCellCurrent.Ch = cfg.ProgressRunes.Current
	progressCellCurrentTip := progressCell
	progressCellCurrentTip.Ch = cfg.ProgressRunes.CurrentTip

	start := time.Now()
	end := start.Add(duration)
	return &notificationComp{
		Responsive:             newString(cfg, msg),
		cfg:                    cfg,
		cancel:                 cancel,
		duration:               duration,
		end:                    end,
		progressCellStart:      progressCellStart,
		progressCellEnd:        progressCellEnd,
		progressCellRemain:     progressCellRemain,
		progressCellCurrent:    progressCellCurrent,
		progressCellCurrentTip: progressCellCurrentTip,
	}
}

func (n *notificationComp) Resize(width, height int) {
	n.width = width
	n.height = height
	n.Responsive.Resize(width, height)
}

func (n *notificationComp) Draw(w term.Writer) {
	n.Responsive.Draw(w)

	if !n.cfg.ProgressBar {
		return
	}

	if n.width >= 3 && n.height >= 3 {
		attrs := term.Attributes{
			Attrs: n.progressCellStart.Attrs,
			Fg:    n.progressCellStart.Fg,
			Bg:    n.progressCellStart.Bg,
		}
		component.DrawFrame(w, n.cfg.FrameCharSet, attrs, n.width-1, n.height-1)
	}

	remaining := time.Until(n.end)
	if !n.pausedAt.IsZero() {
		remaining = n.end.Sub(n.pausedAt)
	}
	total := int64(n.duration)
	current := total - int64(remaining)
	size := n.width

	if n.manualProgressTotal != 0 {
		current = n.manualProgress
		total = n.manualProgressTotal
	}
	currCount := int(math.Ceil(
		float64(current) / float64(total) * float64(size),
	))
	if size < currCount {
		return
	}
	remaCount := size - currCount

	start := term.Coordinates{X: 0, Y: n.height - 1}
	w.SetCell(start, n.progressCellStart)

	for x := 1; x < currCount; x++ {
		pos := term.Coordinates{X: x, Y: n.height - 1}
		w.SetCell(pos, n.progressCellCurrent)
	}
	if currCount != 0 {
		tip := term.Coordinates{X: currCount, Y: n.height - 1}
		w.SetCell(tip, n.progressCellCurrentTip)
	}

	for x := currCount + 1; x < currCount+remaCount-1; x++ {
		pos := term.Coordinates{X: x, Y: n.height - 1}
		w.SetCell(pos, n.progressCellRemain)
	}

	end := term.Coordinates{X: size - 1, Y: n.height - 1}
	w.SetCell(end, n.progressCellEnd)
}
