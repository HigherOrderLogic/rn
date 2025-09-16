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

package plugin

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/vi"
)

// Handler implements a browser.Floating that runs a command in a terminal
// emulator, as a plugin. All fields of the given vte.Config will be overriden
// except for term.Attributes.
type Handler struct {
	liveHandler   tui.Handler
	doneHandler   tui.Handler
	emulator      *vte.Handler
	frame         bool
	frameCharSet  component.FrameCharSet
	cfg           vte.Config
	width, height int

	// non-interactive mode state
	nonInteractiveMinWidth  int
	nonInteractiveMinHeight int

	// interactive mode state
	interactiveHeight int
	interactiveWidth  int
	exitKey           int
	lastExitKey       time.Time

	cancelCtx func()

	bar *pluginHandlerBar
}

var _ component.Scrollable = (*Handler)(nil)

const exitKeyRepeatTimeout = 400 * time.Millisecond

// New allocates storage for a new plugin.Handler and initializes it.
func New(
	publisher browser.EventPublisher, notifications browser.Notifications,
	e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
	cmdAndArgs string, maxWidth int, opts ...Option,
) (*Handler, error) {
	ret := new(Handler)
	return ret, ret.Init(publisher, notifications, e, t, tm,
		cmdAndArgs, maxWidth, opts...)
}

// Init initializes this Handler.
func (h *Handler) Init(
	publisher browser.EventPublisher, notifications browser.Notifications,
	e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
	cmdAndArgs string, maxWidth int, opts ...Option,
) error {
	config := defaultConfig()
	for _, o := range opts {
		o(&config)
	}
	ch, interrupter := h.initState(publisher, cmdAndArgs, maxWidth, config)
	return h.initEmulator(publisher, notifications, e, t, tm, interrupter, ch)
}

// Dimensions satisfies browser.Floating.
func (p *Handler) Dimensions() (int, int) {
	if p.emulator.Component().IsComplete() {
		height := int(math.Max(float64(p.emulator.Component().Height()),
			float64(p.nonInteractiveMinHeight)))
		width := int(math.Max(float64(p.emulator.Component().MaxWidth()),
			float64(p.nonInteractiveMinWidth)))
		return width, height
	}
	return p.interactiveWidth, p.interactiveHeight
}

// Handle satisfies browser.Floating.
func (e *Handler) Handle(ev term.Event) (exit, handled bool) {
	if exit = e.shouldExit(ev); exit {
		return
	}
	handler := e.handler()
	exit, handled = handler.Handle(ev)
	// User might want to inspect the output of a program
	// that ran in the primary buffer. It's assumed that
	// if the program used the alternate buffer, then it's interactive,
	// and so when it exits, the output of the primary will be empty,
	// and so exit should be bubbled up, and handler removed from the UI.
	if exit && !e.emulator.Component().UsedAlternateBuffer() &&
		handler == e.liveHandler {
		exit = false
	}
	return
}

// Draw satisfies browser.Floating.
func (e *Handler) Draw(w term.Writer) {
	e.handler().Draw(w)
}

// Resize satisfies browser.Floating.
func (e *Handler) Resize(width, height int) {
	e.bar.width = width
	e.width = width
	e.height = height
	// always resize live, so next call to Dimensions "adjusts" height of doneHandler
	// by mirroing what the liveHandler would do.
	e.liveHandler.Resize(width, height)
	if e.doneHandler != nil {
		e.doneHandler.Resize(width, height)
	}
}

// Cursor satisfies browser.Floating.
func (e *Handler) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	pos, style, show = e.handler().Cursor()
	if !show {
		return
	}
	pos.Y = int(math.Max(0, math.Min(float64(pos.Y), float64(e.height-1))))
	pos.X = int(math.Max(0, math.Min(float64(pos.X), float64(e.width-1))))
	return
}

// Selection satisfies browser.Floating.
func (e *Handler) Selection() (string, bool) {
	return e.handler().Selection()
}

// Man satisfies browser.Floating.
func (e *Handler) Man() tui.Manual {
	return e.handler().Man()
}

// Close satisfies browser.Floating.
func (p *Handler) Close() error {
	p.cancelCtx()
	p.bar.animation.Close()
	return p.emulator.Close()
}

// OnFocusChange dispatches a change in focus to the underlying
// vte.Handler.
func (p *Handler) OnFocusChange(inFocus bool) {
	p.emulator.OnFocusChange(inFocus)
}

// SeekUp satisfies component.Scrollable.
func (e *Handler) SeekUp() bool {
	return e.emulator.SeekUp()
}

// SeekDown satisfies component.Scrollable.
func (e *Handler) SeekDown() bool {
	return e.emulator.SeekDown()
}

// SeekOffset satisfies component.Scrollable.
func (e *Handler) SeekOffset() int {
	return e.emulator.SeekOffset()
}

// MaxSeekOffset satisfies component.Scrollable.
func (e *Handler) MaxSeekOffset() int {
	return e.emulator.MaxSeekOffset()
}

func (h *Handler) initState(
	publisher browser.EventPublisher,
	cmdAndArgs string, maxWidth int, config handlerConfig,
) (chan error, term.Interrupter) {
	if h.cancelCtx != nil {
		panic("tried to initialize already initialized plugin.Handler")
	}
	if cmdAndArgs == "" {
		cmdAndArgs = os.Getenv("SHELL")
	}
	if cmdAndArgs == "" {
		cmdAndArgs = "sh"
	}

	interrupter := browser.EventPublisherInterrupter(publisher)
	interactiveWidth := int(float64(maxWidth) * 0.8)
	interactiveHeight := interactiveWidth * 9 / 16
	nonInteractiveMinWidth := int(math.Max(float64(maxWidth)*0.2, float64(len(cmdAndArgs)+4)*2))
	nonInteractiveMinHeight := nonInteractiveMinWidth * 9 / 16

	ch := make(chan error)

	templateCfg := component.StringConfig{
		Alignment:            component.SpanAlignmentLeft,
		Attributes:           config.barAttr,
		BackgroundAttributes: config.barAttr,
	}

	errStrCfg := templateCfg
	errStrCfg.Attributes.Fg = tcell.ColorRed
	errStrCfg.Attributes.Attrs = tcell.AttrBold

	successStrCfg := templateCfg
	successStrCfg.Attributes.Fg = tcell.ColorGreen
	successStrCfg.Attributes.Attrs = tcell.AttrBold

	centerStrCfg := templateCfg
	centerStrCfg.Alignment = component.SpanAlignmentHorizontallyCentered

	topBar := new(pluginHandlerBar)
	topBar.startTime = time.Now()
	topBar.leftMsgRunning = component.NewStringWithConfig(" ", templateCfg)
	topBar.leftMsgError = component.NewStringWithConfig(" ◎ ", errStrCfg)
	topBar.leftMsgSuccess = component.NewStringWithConfig(" ◎ ", successStrCfg)
	topBar.centerMsg = component.NewStringWithConfig(cmdAndArgs, centerStrCfg)
	topBar.frameAttr = config.frameAttr
	topBar.attr = config.barAttr
	frames, seq := component.SpinningAnimationFrames()
	topBar.animation = component.NewAnimation(interrupter, frames, seq, 10)
	h.nonInteractiveMinWidth = nonInteractiveMinWidth
	h.nonInteractiveMinHeight = nonInteractiveMinHeight
	h.interactiveHeight = interactiveHeight
	h.interactiveWidth = interactiveWidth
	h.bar = topBar
	h.frame = config.frame
	h.frameCharSet = config.frameCharSet

	config.cfg.Shell = cmdAndArgs
	config.cfg.WidthHint = interactiveWidth
	config.cfg.HeightHint = interactiveHeight
	if config.cfg.Watcher != nil {
		config.cfg.Watcher = workspaceapi.MultiProcessWatcher(
			config.cfg.Watcher, workspaceapi.ChanProcessWatcher(ch))
	} else {
		config.cfg.Watcher = workspaceapi.ChanProcessWatcher(ch)
	}
	h.cfg = config.cfg

	return ch, interrupter
}

func (e *Handler) initEmulator(
	publisher browser.EventPublisher, notifications browser.Notifications,
	executor schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
	interrupter term.Interrupter, ch chan error,
) error {
	// we want shell to be a one-shot execution, so initialCmd must be empty
	// and shell must execute the command. Under the hood file scheme
	// allows for shell with command and arguments so it's fine to pass as-is.
	initialCmd := ""

	ctx, cancel := context.WithCancel(context.Background())
	e.cancelCtx = cancel

	vteh, err := vte.NewHandler(publisher, notifications, t, executor,
		tm, e.cfg, initialCmd)
	if err != nil {
		cancel()
		return fmt.Errorf("new vte: %v", err)
	}
	e.emulator = vteh
	e.liveHandler = e.newUnion(e.emulator)

	go debug.CapturePanicReport(func() {
		term.InterruptAt(ctx, interrupter, 1)
	})
	go debug.CapturePanicReport(func() {
		var err error

		defer func() {
			e.bar.mu.Lock()
			defer e.bar.mu.Unlock()

			e.bar.done = true
			e.bar.doneErr = err
			e.bar.doneTime = time.Now()
			cancel()
		}()

		select {
		case err = <-ch:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return nil
}

func (e *Handler) shouldExit(ev term.Event) (exit bool) {
	if ev.Type != term.EventKey {
		return
	}
	if ev.Ch == 'c' && ev.Mod == term.ModCtrl {
		exit = true
		return
	}
	if ev.Key == term.KeyEsc {
		e.exitKey++
		if e.exitKey >= 2 && time.Since(e.lastExitKey) < exitKeyRepeatTimeout {
			exit = true
			return
		}
		e.lastExitKey = time.Now()
	} else {
		e.exitKey = 0
	}
	return
}

func (e *Handler) handler() tui.Handler {
	e.bar.mu.Lock()
	isDone := e.bar.done
	e.bar.mu.Unlock()
	if !isDone {
		return e.liveHandler
	}

	if e.doneHandler == nil {
		e.initializeDoneHandler()
	}

	return e.doneHandler
}

func (e *Handler) initializeDoneHandler() {
	if e.emulator.Component().UsedAlternateBuffer() {
		// ensure that doneHandler is not nil; liveHandler will return
		// exit on the next call to Handle
		e.doneHandler = e.liveHandler
		return
	}
	orig := e.emulator.Component().PrimaryScroll().Buffer()
	// clone buffer; vte buffer is initialized with InitPerformance
	// which doesn't provide the facilities needed by less
	buf := cell.CellsToBuffer(orig.RawCells(), orig.Tabspaces())

	uri := e.emulator.Component().URI()

	var main text.Handler
	if e.cfg.Modal {
		main = vi.New(buf, uri,
			// vi.WithBarAttr(e.cfg.modalBarAttr()),
			vi.WithResAttr(e.cfg.SelectionAttributes),
			vi.WithAttr(e.cfg.Attributes),
			vi.WithBarAttr(e.bar.frameAttr),
			vi.WithDebug(false),
			vi.WithWrap(false),
			vi.WithClipboard(e.cfg.Clipboard),
		)
	} else {
		main = text.NewSimpleHandler(e.cfg.Clipboard, buf, uri, false, true,
			e.cfg.Attributes, e.cfg.SelectionAttributes, e.bar.frameAttr)
	}

	e.doneHandler = e.newUnion(main)
	e.doneHandler.Resize(e.width, e.height)
	main.SetCursorAtScroll(e.emulator.Component().CursorAtScroll())
}

func (e *Handler) newUnion(unionMain tui.Handler) tui.Handler {
	union := handler.NewFrameUnion(unionMain)
	union.Frame = false
	union.UnionTop(handler.Nop(e.bar), 1)
	if e.frame {
		separator := handler.Nop(&component.TestComponent{
			Ch:         e.frameCharSet.HorizontalBottom,
			Attributes: e.bar.frameAttr,
		})
		union.UnionTop(separator, 1)
	}
	return union
}

type pluginHandlerBar struct {
	mu        sync.Mutex
	done      bool
	doneErr   error
	doneTime  time.Time
	startTime time.Time
	width     int
	frameAttr term.Attributes
	attr      term.Attributes

	animation      *component.Animation
	centerMsg      tui.Component
	leftMsgError   tui.Component
	leftMsgSuccess tui.Component
	leftMsgRunning tui.Component
}

func (e *pluginHandlerBar) Draw(w term.Writer) {
	e.mu.Lock()
	done := e.done
	doneErr := e.doneErr
	doneTime := e.doneTime
	startTime := e.startTime
	e.mu.Unlock()

	leftWidgetWidth := 3
	const barHeight = 1

	var left tui.Component
	var rightMsg string
	if done {
		if doneErr != nil {
			rightMsg = doneTime.Sub(startTime).Truncate(time.Millisecond).String()
			left = e.leftMsgError
		} else {
			rightMsg = doneTime.Sub(startTime).Truncate(time.Millisecond).String()
			left = e.leftMsgSuccess
		}
	} else {
		leftWidgetWidth++
		rightMsg = time.Since(startTime).Truncate(time.Second).String()
		leftMsg := e.leftMsgRunning
		var union component.FrameUnion
		union.Init(leftMsg)
		animationBackground := term.Cell{Ch: ' ', Attributes: e.attr}
		union.UnionLeft(component.WithBackground(e.animation, animationBackground), 3)
		union.Resize(leftWidgetWidth, barHeight)
		left = &union
	}

	right := component.NewStringWithConfig(rightMsg, component.StringConfig{
		Alignment:            component.SpanAlignmentRight,
		Attributes:           e.attr,
		BackgroundAttributes: e.attr,
	})
	var union component.FrameUnion
	union.Init(e.centerMsg)
	union.Frame = false

	union.UnionLeft(left, leftWidgetWidth)
	union.UnionRight(right, len(rightMsg))

	union.Resize(e.width, barHeight)
	union.Draw(w)
}

func (e *pluginHandlerBar) Resize(width, height int) {
}
