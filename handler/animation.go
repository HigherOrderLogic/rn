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

package handler

import (
	"context"

	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// AnimationPlayer returns a tui.Handler that wraps a component.Animation
// into a tui.Handler that plays the animation and stops with the space key.
// It also handles Escape, in which case the animation is stopped and Handle
// returns exit=true.
func AnimationPlayer(a *component.Animation) tui.Handler {
	return &player{a: a}
}

var _ tui.Handler = (*player)(nil)

type player struct {
	a           *component.Animation
	width       int
	height      int
	pause       bool
	pausedFrame tui.Component
}

func (p *player) Resize(width, height int) {
	p.width = width
	p.height = height
	p.a.Resize(width, height)
}

func (p *player) Draw(w term.Writer) {
	if !p.pause {
		p.a.Draw(w)
		return
	}

	if p.pausedFrame != nil {
		p.pausedFrame.Draw(w)
		return
	}

	p.cachePausedFrame(w.Context())
	p.pausedFrame.Draw(w)
}

func (p *player) cachePausedFrame(ctx context.Context) {
	var bw cell.BufferWriter
	var buf cell.Buffer
	bw.Init(ctx, p.width, p.height)
	p.a.Draw(&bw)
	bw.ToBuffer(&buf)

	scroll := new(component.Scroll)
	scroll.InitPerformance(&buf)
	p.pausedFrame = scroll
	p.pausedFrame.Resize(p.width, p.height)
}

func (p *player) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventKey || ev.Mod != 0 {
		return
	}

	handled = true
	switch ev.Key {
	case term.KeySpace:
		p.pause = !p.pause
		if !p.pause {
			p.pausedFrame = nil
		}
	case term.KeyEsc:
		p.cachePausedFrame(context.Background())
		p.pause = true
		exit = true
		_ = p.a.Close()
	}
	return
}

func (p *player) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return
}

func (p *player) Selection() (string, bool) {
	return "", false
}
