package plugin

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"unstable.build/go-tui"
	schemeapi "unstable.build/go-tui/api/scheme"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/emulator"
)

// Handler implements a browser.Floating that runs a command in a terminal
// emulator, as a plugin. All fields of emulator.Config will be overriden
// except for term.Attributes.
func Handler(
	publisher browser.EventPublisher, notifications browser.Notifications,
	e schemeapi.Executor, t schemeapi.Terminal,
	cfg emulator.Config, cmdAndArgs string, maxWidth int,
	frame bool, frameCharSet component.FrameCharSet,
	frameAttr term.Attributes,
) (browser.Floating, error) {
	if cmdAndArgs == "" {
		cmdAndArgs = os.Getenv("SHELL")
	}
	if cmdAndArgs == "" {
		cmdAndArgs = "sh"
	}

	interactiveWidth := int(float64(maxWidth) * 0.8)
	interactiveHeight := interactiveWidth * 9 / 16
	nonInteractiveMinWidth := int(math.Max(float64(maxWidth)*0.2, float64(len(cmdAndArgs)+4)*2))
	nonInteractiveMinHeight := nonInteractiveMinWidth * 9 / 16

	ch := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())

	// we want shell to be a one-shot execution, so initialCmd must be empty
	// and shell must execute the command. Under the hood file scheme
	// allows for shell with command and arguments so it's fine to pass as-is.
	initialCmd := ""
	cfg.Shell = cmdAndArgs
	cfg.WidthHint = interactiveWidth
	cfg.HeightHint = interactiveHeight
	cfg.Watcher = workspaceapi.ChanWatcher(ch)
	h, err := emulator.New(publisher, notifications, t, e, cfg, initialCmd)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("new emulator: %v", err)
	}

	leftStrCfg := component.StringConfig{
		Alignment:  component.SpanAlignmentLeft,
		Attributes: frameAttr,
	}
	topBar := new(pluginHandlerBar)
	topBar.startTime = time.Now()
	topBar.leftMsgRunning = component.NewStringWithConfig(" "+cmdAndArgs, leftStrCfg)
	topBar.leftMsgError = component.NewStringWithConfig(" 💥  "+cmdAndArgs, leftStrCfg)
	topBar.leftMsgSuccess = component.NewStringWithConfig(" 🤘🏼 "+cmdAndArgs, leftStrCfg)
	topBar.frameAttr = frameAttr
	frames, seq := component.ProgressAnimationFrames()
	topBar.animation = component.NewAnimation(publisher, frames, seq, 10)
	unionMain := h
	union := handler.NewFrameUnion(unionMain)
	union.Frame = false
	union.UnionTop(handler.Nop(topBar), 1)
	if frame {
		separator := handler.Nop(&component.TestComponent{
			Ch:         frameCharSet.HorizontalBottom,
			Attributes: frameAttr,
		})
		union.UnionTop(separator, 1)
	}

	ret := &pluginHandler{
		union:                   union,
		emulator:                h,
		nonInteractiveMinWidth:  nonInteractiveMinWidth,
		nonInteractiveMinHeight: nonInteractiveMinHeight,
		interactiveHeight:       interactiveHeight,
		interactiveWidth:        interactiveWidth,
		cancelCtx:               cancel,
		bar:                     topBar,
	}
	go term.InterruptAt(ctx, publisher, 1)
	go func() {
		defer cancel()
		select {
		case err := <-ch:
			ret.bar.mu.Lock()
			defer ret.bar.mu.Unlock()
			ret.bar.done = true
			ret.bar.doneErr = err
			ret.bar.doneTime = time.Now()
		case <-ctx.Done():
			return
		}
	}()
	return ret, nil
}

type pluginHandler struct {
	union    tui.Handler
	emulator *emulator.Handler

	// non-interactive mode state
	nonInteractiveMinWidth  int
	nonInteractiveMinHeight int

	// interactive mode state
	drawn             int
	interactiveHeight int
	interactiveWidth  int

	cancelCtx func()

	bar *pluginHandlerBar
}

type pluginHandlerBar struct {
	mu        sync.Mutex
	done      bool
	doneErr   error
	doneTime  time.Time
	startTime time.Time
	width     int
	frameAttr term.Attributes

	animation      *component.Animation
	leftMsgError   tui.Component
	leftMsgSuccess tui.Component
	leftMsgRunning tui.Component
}

func (p *pluginHandler) Dimensions() (int, int) {
	// if we always draw interactive (commands that do not complete very fast)
	// then stick to it, otherwise user wanders around the screen creating a bit
	// of confusion.
	if p.emulator.IsComplete() && p.drawn == 0 {
		height := int(math.Max(float64(p.emulator.Height()),
			float64(p.nonInteractiveMinHeight)))
		width := int(math.Max(float64(p.emulator.MaxWidth()),
			float64(p.nonInteractiveMinWidth)))
		return width, height
	}
	return p.interactiveWidth, p.interactiveHeight
}

func (e *pluginHandler) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type == term.EventKey {
		if ev.Key == term.KeyEsc || ev.Key == term.KeyCtrlC {
			exit = true
			return
		}
		e.bar.mu.Lock()
		isDone := e.bar.done
		e.bar.mu.Unlock()
		if isDone && (ev.Key == term.KeyArrowDown || ev.Ch == 'j') {
			handled = e.emulator.ScrollDown(1)
			return
		}
		if isDone && (ev.Key == term.KeyArrowUp || ev.Ch == 'k') {
			handled = e.emulator.ScrollUp(1)
			return
		}
		if isDone && (ev.Key == term.KeyHome || ev.Ch == 'g') {
			handled = e.emulator.ScrollTop()
			return
		}
		if isDone && (ev.Key == term.KeyEnd || ev.Ch == 'G') {
			handled = e.emulator.ScrollBottom()
			return
		}
	}
	_, handled = e.union.Handle(ev)
	return
}

func (e *pluginHandler) Draw(w term.Writer) {
	if !e.emulator.IsComplete() {
		e.drawn++
	}
	e.union.Draw(w)
}

func (e *pluginHandler) Resize(width, height int) {
	e.bar.width = width
	e.union.Resize(width, height)
}

func (e *pluginHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, term.CursorStyleDefault, false
}

func (e *pluginHandler) Man() tui.Manual {
	return e.union.Man()
}

func (p *pluginHandler) Close() error {
	p.cancelCtx()
	p.bar.animation.Close()
	return p.emulator.Close()
}

func (e *pluginHandlerBar) Draw(w term.Writer) {
	e.mu.Lock()
	done := e.done
	doneErr := e.doneErr
	doneTime := e.doneTime
	startTime := e.startTime
	e.mu.Unlock()

	leftWidgetWidth := int(float64(e.width) / 2)
	const barHeight = 1

	var left tui.Component
	var rightMsg string
	if done {
		if doneErr != nil {
			rightMsg = fmt.Sprintf("%s", doneTime.Sub(startTime).Truncate(time.Millisecond))
			left = e.leftMsgError
		} else {
			rightMsg = fmt.Sprintf("%s", doneTime.Sub(startTime).Truncate(time.Millisecond))
			left = e.leftMsgSuccess
		}
	} else {
		rightMsg = fmt.Sprintf("%s", time.Now().Sub(startTime).Truncate(time.Second))
		leftMsg := e.leftMsgRunning
		var union component.FrameUnion
		union.Init(leftMsg)
		union.UnionLeft(e.animation, 3)
		union.Resize(leftWidgetWidth, barHeight)
		left = &union
	}

	main := component.NewStringWithConfig(rightMsg, component.StringConfig{
		Alignment:  component.SpanAlignmentRight,
		Attributes: e.frameAttr,
	})
	var union component.FrameUnion
	union.Init(main)
	union.Frame = false

	union.UnionLeft(left, leftWidgetWidth)

	union.Resize(e.width, barHeight)
	union.Draw(w)
	return
}

func (e *pluginHandlerBar) Resize(width, height int) {
}
