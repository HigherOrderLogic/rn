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
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	thandler "unstable.build/go-tui/handler"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
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
	cmdAndArgs []string, maxWidth int, opts ...Option,
) (*Handler, error) {
	ret := new(Handler)
	return ret, ret.Init(publisher, notifications, e, t, tm,
		cmdAndArgs, maxWidth, opts...)
}

// Init initializes this Handler.
func (h *Handler) Init(
	publisher browser.EventPublisher, notifications browser.Notifications,
	e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
	cmdAndArgs []string, maxWidth int, opts ...Option,
) error {
	err := h.init(publisher, notifications, e, t, tm,
		cmdAndArgs, maxWidth, opts...)
	if err != nil {
		return err
	}
	go h.bar.initElapsedTicker()
	return err
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

// Close satisfies browser.Floating.
func (p *Handler) Close() error {
	p.cancelCtx()
	p.bar.Close()
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

func (h *Handler) init(
	publisher browser.EventPublisher, notifications browser.Notifications,
	e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
	cmdAndArgs []string, maxWidth int, opts ...Option,
) error {
	config := defaultConfig()
	for _, o := range opts {
		o(&config)
	}
	if len(cmdAndArgs) == 0 {
		return errors.New("empty command and args")
	}
	ch, interrupter := h.initState(publisher, cmdAndArgs, maxWidth, config)
	return h.initEmulator(publisher, notifications, e, t, tm, interrupter, ch)
}

func (h *Handler) initState(
	publisher browser.EventPublisher,
	cmdAndArgs []string, maxWidth int, config handlerConfig,
) (chan error, term.Interrupter) {
	if h.cancelCtx != nil {
		panic("tried to initialize already initialized plugin.Handler")
	}

	title := config.title
	if config.title == "" {
		title = strings.Join(cmdAndArgs, " ")
	}

	interrupter := browser.EventPublisherInterrupter(publisher)
	h.interactiveWidth = int(float64(maxWidth) * 0.8)
	h.interactiveHeight = h.interactiveWidth * 9 / 16
	h.nonInteractiveMinWidth = int(math.Max(float64(maxWidth)*0.2, float64(len(title)+4)*2))
	h.nonInteractiveMinHeight = h.nonInteractiveMinWidth * 9 / 16

	ch := make(chan error)

	topBar := newPluginHandlerBar(title, interrupter, config.bar)
	h.bar = topBar

	h.frame = config.frame
	h.frameCharSet = config.frameCharSet

	config.cfg.CommandAndArgs = cmdAndArgs
	config.cfg.WidthHint = h.interactiveWidth
	config.cfg.HeightHint = h.interactiveHeight
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
		return fmt.Errorf("new vte: %w", err)
	}
	e.emulator = vteh
	e.liveHandler = e.newUnion(e.emulator)

	go debug.CapturePanicReport(func() {
		term.InterruptAt(ctx, interrupter, 1)
	})
	go debug.CapturePanicReport(func() {
		var err error

		defer func() {
			e.bar.setDone(err)
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
	// re-initialize with Init, as opposed how it was initialized (InitPerformance)
	// so cursor can subscribe and use the underlying buffer
	data := term.CellsToString(orig.RawCells())
	orig.Init()
	_, _ = orig.ReadFrom(strings.NewReader(data))

	uri := e.emulator.Component().URI()

	clipboard := nullReplaceClipboard{root: e.cfg.Clipboard}
	var main text.Handler
	if e.cfg.Modal {
		main = vi.New(orig, uri,
			vi.WithResAttr(e.cfg.SelectionAttributes),
			vi.WithAttr(e.cfg.Attributes),
			vi.WithWrap(false),
			vi.WithTabspaces(1),
			vi.WithClipboard(clipboard),
		)
	} else {
		main = modeless.NewHandler(orig, uri,
			modeless.WithResAttr(e.cfg.SelectionAttributes),
			modeless.WithAttr(e.cfg.Attributes),
			modeless.WithWrap(false),
			modeless.WithTabspaces(1),
			modeless.WithCommandBar(true),
			modeless.WithClipboard(clipboard),
		)
	}

	e.doneHandler = e.newUnion(main)
	e.doneHandler.Resize(e.width, e.height)
	main.SetCursorAtScroll(e.emulator.Component().CursorAtScroll())
}

func (e *Handler) newUnion(unionMain tui.Handler) tui.Handler {
	union := thandler.NewFrameUnion(unionMain)
	union.Frame = false
	if e.bar.AlignBottom {
		union.UnionBottom(handler.NopFromComponent(e.bar), barHeight)
	} else {
		union.UnionTop(handler.NopFromComponent(e.bar), barHeight)
	}
	return union
}
