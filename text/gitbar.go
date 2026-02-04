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

package text

import (
	"context"
	"errors"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide/vctrl"
)

// GitBarConfig holds configuration for the auxiliary bar created
// by WithGitBar.
type GitBarConfig struct {
	ScheduleNextTick func(func()) bool
	DelAttr          term.Attributes
	AddAttr          term.Attributes
	DelOverlayAttr   term.Attributes
	AddOverlayAttr   term.Attributes
	Publisher        EventPublisher
	CommandRegistry  FileCommandRegistry
}

// WithGitBar wraps the given editor with an git bar. The given buffer,
// and scroll should correspond to the buffer and scroll used by the given editor.
func WithGitBar(
	svc vctrl.Service, handler Handler,
	buf *cell.Buffer, scroll *component.Scroll,
	cfg GitBarConfig,
) Handler {
	if cfg.CommandRegistry == nil || cfg.Publisher == nil || cfg.ScheduleNextTick == nil {
		panic("gitbar configuration is missing key dependencies")
	}
	ret := new(gitBar)
	ret.buf = buf
	ret.scroll = scroll
	ret.Handler = handler
	ret.vhandler.C = handler
	ret.scheduleNextTick = cfg.ScheduleNextTick
	ret.svc = svc

	if cfg.DelAttr == (term.Attributes{}) {
		cfg.DelAttr = term.Attributes{Fg: tcell.ColorWhite, Bg: tcell.ColorRed}
	}
	if cfg.AddAttr == (term.Attributes{}) {
		cfg.AddAttr = term.Attributes{Fg: tcell.ColorWhite, Bg: tcell.ColorGreen}
	}
	if cfg.DelOverlayAttr == (term.Attributes{}) {
		cfg.DelOverlayAttr = term.Attributes{Bg: tcell.ColorMaroon}
	}
	if cfg.AddOverlayAttr == (term.Attributes{}) {
		cfg.AddOverlayAttr = term.Attributes{Bg: tcell.ColorGreen}
	}
	ret.delAttr = cfg.DelAttr
	ret.addAttr = cfg.AddAttr

	b := new(cell.Buffer)
	b.InitPerformance(buf.Rows(), 1, ' ')

	ret.bar = new(component.Scroll)
	ret.bar.InitPerformance(b)
	ret.bar.SetOffset(term.Coordinates{Y: scroll.Offset().Y})
	ret.file = handler.Resource()
	ret.pub = cfg.Publisher
	ret.registry = cfg.CommandRegistry
	ret.config = cfg

	scroll.Subscribe(ret)
	buf.Subscribe(ret)
	evs := []textapi.EventType{textapi.EventTypeFlush, textapi.EventTypeFocus}
	ret.dirty = true
	ret.cancelBuild = func() {}
	_ = ret.pub.SubscribeEvents(evs, (*gitBarSubscriber)(ret))
	for _, cmd := range gitCommands {
		// could return error if auxbar is enabled
		_ = ret.registry.SubscribeCommandForFile(ret.file, cmd, ret)
	}

	if ret.dirty {
		ret.rebuildBar(context.Background())
	}
	return ret
}

const (
	// shared amongst bars
	gitLocationsID       = "gitchange"
	addIcon              = "+"
	delIcon              = "-"
	commandToggleOverlay = "gittoggleoverlay"
)

var (
	commandToggleOverlayManual = textapi.CommandManual{
		Name:    commandToggleOverlay,
		Summary: "Shows or hides the git diff hunks overlay.",
	}
	gitCommands = []textapi.CommandManual{
		commandToggleOverlayManual,
	}
)

type gitBar struct {
	Handler
	svc              vctrl.Service
	scheduleNextTick func(func()) bool
	pub              EventPublisher
	registry         FileCommandRegistry
	config           GitBarConfig

	file    workspaceapi.URI
	buf     *cell.Buffer
	scroll  *component.Scroll
	delAttr term.Attributes
	addAttr term.Attributes

	dirty       bool
	cancelBuild func()
	closed      bool
	vhandler    handler.Virtual[Handler]
	bar         *component.Scroll
	addLocAttr  term.Attributes
	delLocAttr  term.Attributes
}

func (b *gitBar) Selection() (string, bool) {
	return b.vhandler.Selection()
}

func (b *gitBar) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return b.vhandler.Cursor()
}

func (b *gitBar) Draw(w term.Writer) {
	b.vhandler.Draw(w)
	b.bar.Draw(w)
	if b.bar.Width() != 0 {
		bg := term.Attributes{Bg: b.scroll.Attributes.Bg}
		for y := range b.bar.SizeHeight() {
			w.UnionAttributes(term.Coordinates{Y: y, X: 0}, bg)
			w.UnionAttributes(term.Coordinates{Y: y, X: 1}, bg)
		}
	}
}

func (b *gitBar) Handle(ev term.Event) (quit, handled bool) {
	// TODO handle mouse events
	//fold := ev.Type == term.EventMouse && ev.Key == term.MouseLeft &&
	//	ev.MouseX >= 0 && ev.MouseX < 1
	return b.vhandler.Handle(ev)
}

func (b *gitBar) Resize(width, height int) {
	const minSpaceForMain = 4
	barWidth := 2
	if width < minSpaceForMain+barWidth {
		barWidth = 0
	}
	b.bar.Resize(barWidth, height)
	b.vhandler.Move(term.Coordinates{X: barWidth})
	b.vhandler.Resize(width-barWidth, height)
}

func (b *gitBar) HandleCommand(ctx context.Context, cmd textapi.Command) error {
	switch cmd.Name {
	case commandToggleOverlay:
		if b.addLocAttr == (term.Attributes{}) {
			b.addLocAttr = b.config.AddOverlayAttr
		} else {
			b.addLocAttr = term.Attributes{}
		}
		if b.delLocAttr == (term.Attributes{}) {
			b.delLocAttr = b.config.DelOverlayAttr
		} else {
			b.delLocAttr = term.Attributes{}
		}
		b.rebuildBar(ctx)
		return nil
	default:
		return nil
	}
}

func (b *gitBar) Complete(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	return iterator.Empty[string](), "", nil
}

func (b *gitBar) Close() (ret error) {
	if b.closed {
		return nil
	}
	b.closed = true
	ok, err := b.pub.UnsubscribeEvents((*gitBarSubscriber)(b))
	if err != nil {
		ret = multierror.Append(ret, err)
	} else if !ok {
		ret = multierror.Append(ret, errors.New("could not unsubscribe auxiliary bar"))
	}
	for _, cmd := range gitCommands {
		// could return error if auxbar is enabled
		_ = b.registry.UnsubscribeCommandForFile(b.file, cmd.Name)
	}
	if err := b.vhandler.C.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	return
}

func (b *gitBar) rebuildBar(ctx context.Context) {
	b.dirty = false
	b.cancelBuild()
	ctx, b.cancelBuild = context.WithCancel(ctx)
	uri := b.Handler.Resource()
	delLocAttr := b.delLocAttr
	addLocAttr := b.addLocAttr
	go debug.CapturePanicReport(func() {
		filediff, err := b.svc.Diff(ctx, uri)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				b.log(log.ErrorLevel, "compute diff: %v", err)
			}
			return
		}
		b.log(log.TraceLevel, "computed diff: %v", filediff)

		ll := filediff.LocationList(delLocAttr, addLocAttr)
		b.scheduleNextTick(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			b.bar.Buffer().ResetPerformance()
			for loc, ok := ll.Current(); ok; loc, ok = ll.Next() {
				from := term.Coordinates{Y: loc.From.Y}
				to := term.Coordinates{Y: loc.To.Y}
				if from == to {
					at, _ := b.scrollToBarCoordinates(from)
					at.X = 0
					b.bar.Buffer().DeleteCell(at)
					b.bar.Buffer().InsertStringWithAttr(at, delIcon, b.delAttr)
					continue
				}

				for y := from.Y; y < to.Y; y++ {
					at, _ := b.scrollToBarCoordinates(term.Coordinates{Y: y})
					at.X = 0
					icon := addIcon
					b.bar.Buffer().DeleteCell(at)
					b.bar.Buffer().InsertStringWithAttr(at, icon, b.addAttr)
				}
			}
			b.Handler.SetLocationList(textapi.LocationPriorityInfo, gitLocationsID, ll)
		})
	})
}

type gitBarSubscriber gitBar

func (b *gitBarSubscriber) Handle(ctx context.Context, ev textapi.Event) bool {
	if !ev.URI.Equal(b.file) || (ev.Type != textapi.EventTypeFlush &&
		ev.Type != textapi.EventTypeFocus) {
		return false
	}
	(*gitBar)(b).log(log.TraceLevel, "received event: %s", ev.Type.String())
	(*gitBar)(b).rebuildBar(ctx)
	return false
}

func (b *gitBar) OnDidSeek(_, to term.Coordinates) {
	b.bar.SetOffset(term.Coordinates{Y: to.Y})
}

func (b *gitBar) OnWillSeek(_ term.Coordinates) {
}

func (b *gitBar) OnWillHide(start, end int) {
}

func (b *gitBar) OnWillVisible(start int) {
}

func (b *gitBar) OnDidHide(start, end int) {
	b.rebuildBar(context.Background())
}

func (b *gitBar) OnDidVisible(start int) {
	b.rebuildBar(context.Background())
}

func (b *gitBar) OnWillEdit(
	ctx context.Context, from, to term.Coordinates, str string,
) {
}

func (b *gitBar) OnDidEdit(
	ctx context.Context, start, end term.Coordinates, old string,
) {
}

func (b *gitBar) scrollToBarCoordinates(pos term.Coordinates) (ret term.Coordinates, ok bool) {
	pos, ok = b.scroll.ScrollToWindowCoordinates(pos)
	ret = pos
	ret.Y += b.scroll.Offset().Y
	ret.X += b.scroll.Offset().X
	return
}

func (b *gitBar) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "text.gitBar").Logf(level, msg, args...)
}
