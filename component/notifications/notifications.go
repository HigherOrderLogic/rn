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
	"time"

	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

var _ component.Responsive = (*notificationComp)(nil)

type notificationComp struct {
	component.Responsive
	cfg          Config
	cancel       func()
	width        int
	height       int
	duration     time.Duration
	end          time.Time
	pausedAt     time.Time
	progressCell term.Cell
}

func newString(cfg Config, msg string) component.Responsive {
	strConfig := component.StringResponsiveConfig{
		NoSplitWords: true,
		StringConfig: component.StringConfig{
			Alignment:            component.SpanAlignmentCentered,
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
) *notificationComp {

	var progressCell term.Cell
	progressCell.Ch = cfg.FrameCharSet.HorizontalBottom

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

	start := time.Now()
	end := start.Add(duration)
	return &notificationComp{
		Responsive:   newString(cfg, msg),
		cfg:          cfg,
		cancel:       cancel,
		duration:     duration,
		end:          end,
		progressCell: progressCell,
	}
}

func (n *notificationComp) Resize(width, height int) {
	n.width = width
	n.height = height
	n.Responsive.Resize(width, height)
}

func (n *notificationComp) Draw(w term.Writer) {
	n.Responsive.Draw(w)

	if !n.cfg.ProgressBar || n.width < 4 {
		return
	}

	remaining := time.Until(n.end)
	if !n.pausedAt.IsZero() {
		remaining = n.end.Sub(n.pausedAt)
	}

	remainingRatio := float64(remaining) / float64(n.duration)

	progressWidth := int(float64(n.width) * remainingRatio)
	progressOffset := n.width - progressWidth
	for i := 1; i < progressWidth; i++ {
		pos := term.Coordinates{X: progressOffset + i - 1, Y: n.height - 1}
		w.SetCell(pos, n.progressCell)
	}
}
