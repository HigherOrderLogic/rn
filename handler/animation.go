package handler

import (
	"context"

	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
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
	if ev.Type != term.EventKey {
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

func (p *player) Man() tui.Manual {
	return tui.Manual{}
}
