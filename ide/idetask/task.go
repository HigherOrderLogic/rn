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

package idetask

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/term"
)

const (
	colorSuccess    = tcell.ColorGreen
	colorRunning    = tcell.ColorGray
	colorError      = tcell.ColorRed
	minimizePadding = 0
)

var _ browser.ScrollableFloating = (*Task)(nil)

// Task represents a task that runs a command in response to changes to files.
// Manager.Run must be called before a task can be rendered.
//
// A Task must not be copied after first use.
type Task struct {
	// Name of the task.
	Name string
	// Filter is used as an inclusive pattern match.
	Filter string
	// Cmd is the command to run.
	Cmd string
	// Args are the args to pass to the command.
	Args []string

	// MinimizeAlignment determines the alignment to use
	// when a task's window is minimized.
	MinimizeAlignment component.Alignment

	// Runs represents the number of times this task has been run.
	runs int

	// stale is true when a file has changed and the task should
	// be scheduled at soon as it's done with the current run.
	stale bool

	watchID     int
	ctx         context.Context
	cancelCtx   func()
	closeHook   func()
	doneWaitCh  chan struct{}
	mu          *sync.Mutex
	win         browser.Window
	cmdAndArgs  string
	pluginOpts  []plugin.Option
	donech      chan error
	interrupter term.Interrupter
	newPlugin   pluginBuilder

	bar             tui.Component
	barColor        tcell.Color
	defaultBarColor tcell.Color
	width           int
	maxWidth        int
	minWidth        int
	minHeight       int
	height          int
	running         bool
	lastExit        error
	lastStart       time.Time
	lastDuration    time.Duration
	handler         browser.ScrollableFloating
	// don't use mu to check if closed, so StopTask, followed by
	// browser close handler doesn't deadlock
	closed *atomic.Bool
}

// TaskInfo is an immutable snapshot of the status of a task.
type TaskInfo struct {
	Name        string
	Filter      string
	Cmd         string
	Args        []string
	Running     bool
	LastSuccess bool
	// Runs represents the number of times this task has been run.
	Runs int
	// Scheduled is true when a file has changed and the task should
	// be scheduled at soon as it's done with the current run.
	Scheduled    bool
	LastDuration time.Duration
}

// Info returns an immutable snapshot of this Task.
func (t *Task) Info() TaskInfo {
	t.mu.Lock()
	defer t.mu.Unlock()

	return TaskInfo{
		Name:         t.Name,
		Filter:       t.Filter,
		Cmd:          t.Cmd,
		Args:         t.Args,
		Running:      t.running,
		Runs:         t.runs,
		Scheduled:    t.stale,
		LastSuccess:  t.lastExit == nil,
		LastDuration: t.lastDuration,
	}
}

// Handle satisfies tui.Handler.
func (t *Task) Handle(ev term.Event) (exit, handled bool) {
	if ev.Ch == 'c' && ev.Mod == term.ModCtrl {
		t.minimize()
		return
	}
	_, handled = t.handler.Handle(ev)
	return
}

// Cursor satisfies tui.Handler.
func (t *Task) Cursor() (c term.Coordinates, s term.CursorStyle, show bool) {
	return t.handler.Cursor()
}

// Selection satisfies tui.Handler.
func (t *Task) Selection() (string, bool) {
	return t.handler.Selection()
}

// Draw satisfies tui.Component.
func (t *Task) Draw(w term.Writer) {
	if _, is := t.win.IsMinimized(); is {
		t.bar.Draw(w)
		return
	}
	t.handler.Draw(w)
}

// Dimensions satisfies browserapi.Floating.
func (t *Task) Dimensions() (width int, height int) {
	width, height = t.handler.Dimensions()
	width = int(math.Max(
		float64(width),
		float64(t.minWidth)),
	)
	height = int(math.Max(
		float64(height),
		float64(t.minHeight)),
	)
	return
}

// Resize satisfies tui.Component.
func (t *Task) Resize(width, height int) {
	if t.closed.Load() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.width = width
	t.height = height
	t.bar.Resize(width, height)
	if t.handler != nil {
		t.handler.Resize(width, height)
	}
}

// MaxSeekOffset satisfies component.Scrollable.
func (e *Task) MaxSeekOffset() int {
	return e.handler.MaxSeekOffset()
}

// SeekDown satisfies component.Scrollable.
func (e *Task) SeekDown() bool {
	return e.handler.SeekDown()
}

// SeekOffset satisfies component.Scrollable.
func (e *Task) SeekOffset() int {
	return e.handler.SeekOffset()
}

// SeekUp satisfies component.Scrollable.
func (e *Task) SeekUp() bool {
	return e.handler.SeekUp()
}

// Man satisfies tui.Handler.
func (t *Task) Man() tui.Manual {
	panic("TODO")
}

// Close satisfies browserapi.Handler.
func (t *Task) Close() error {
	t.closeHook()
	return t.doClose()
}

func calcMinSize(width, height int) (int, int) {
	const (
		factor    = 4
		minWidth  = 8
		minHeight = 5
	)
	return int(math.Max(float64(width/factor), minWidth)),
		int(math.Max(float64(height/factor), minHeight))
}

func (t *Task) doClose() (ret error) {
	if !t.closed.CompareAndSwap(false, true) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.win.Closed() {
		ret = t.win.Close()
	}
	t.cancelCtx()
	if t.handler != nil {
		if err := t.handler.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (t *Task) init(
	watchID int, ctx context.Context, b browser.Browser,
	scheme schemeapi.Scheme, newPlugin pluginBuilder,
	maxWidth, maxHeight int, closeHook func(), pluginOpts ...plugin.Option,
) (context.Context, func(), error) {
	t.closeHook = closeHook
	t.watchID = watchID
	t.maxWidth = maxWidth
	t.minWidth, t.minHeight = calcMinSize(maxWidth, maxHeight)
	t.mu = new(sync.Mutex)
	t.interrupter = browser.EventPublisherInterrupter(b)
	t.newPlugin = newPlugin
	t.pluginOpts = pluginOpts
	t.donech = make(chan error)
	t.pluginOpts = append(t.pluginOpts, plugin.WithProcessWatcher(
		workspaceapi.ChanProcessWatcher(t.donech),
	))
	t.closed = new(atomic.Bool)
	t.doneWaitCh = make(chan struct{})
	t.cmdAndArgs = strings.Join(append([]string{t.Cmd}, t.Args...), " ")
	t.bar = component.WithAttrSetter(component.NewString(""))
	// always have a valid handler so methods do not need to check for nil
	t.handler = browser.NopScrollableFloatingHandler(
		handler.NopScrollableFloating(component.NopScrollableFloating()))
	ctx, cancel := context.WithCancel(ctx)
	t.cancelCtx = cancel
	t.ctx = ctx
	t.win = noopWindow{} // placeholder to avoid panics or checking for nil

	prev, err := b.Focus()
	if err != nil {
		cancel()
		return context.Background(), nil, err
	}
	alignment := t.MinimizeAlignment
	switch t.MinimizeAlignment {
	case component.SpanAlignmentBottom:
		alignment |= component.SpanAlignmentHorizontallyCentered
	case component.SpanAlignmentLeft:
		alignment |= component.SpanAlignmentVerticallyCentered
	case component.SpanAlignmentTop:
		alignment |= component.SpanAlignmentHorizontallyCentered
	case component.SpanAlignmentRight:
		alignment |= component.SpanAlignmentVerticallyCentered
	}
	win, err := b.Floating(t, component.FloatingConfig{
		Alignment: alignment,
	})
	if err != nil {
		cancel()
		return context.Background(), nil, fmt.Errorf("new floating window: %w", err)
	}
	t.win = win

	// process exit errors
	go debug.CapturePanicReport(func() {
		defer close(t.doneWaitCh)
		for {
			select {
			case err := <-t.donech:
				t.mu.Lock()
				stale := t.stale // do not skip state changes
				t.mu.Unlock()
				if err != nil {
					t.setError(err)
				} else {
					t.setSuccess()
				}
				if stale {
					t.tryRunning(b, scheme)
				}
			case <-t.ctx.Done():
				t.setError(t.ctx.Err())
				return
			}
		}
	})

	t.tryRunning(b, scheme)
	t.minimize()
	_, err = b.SetFocus(prev)
	if err != nil {
		cancel()
		return context.Background(), nil, fmt.Errorf("set focus: %w", err)
	}
	return ctx, cancel, nil
}

func (t *Task) tryRunning(b browser.Browser, scheme schemeapi.Scheme) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		t.setStale()
		return false
	}

	handler, err := t.newPlugin(b, b, scheme, scheme, b,
		t.cmdAndArgs, t.maxWidth, t.pluginOpts...)
	if err != nil {
		t.doSetError(err)
		return false
	}

	t.setRunning(handler)
	return true
}

func (t *Task) setStale() {
	t.stale = true
}

func (t *Task) setRunning(h browser.ScrollableFloating) {
	_ = t.handler.Close()
	isFirst := t.runs == 0
	t.runs++
	t.lastStart = time.Now()
	t.running = true
	t.stale = false
	t.handler = h
	t.handler.Resize(t.width, t.height)
	t.barColor = colorRunning
	prev := t.setBarColor(t.barColor)
	if isFirst {
		t.defaultBarColor = prev
	}
	t.interrupt()
}

func (t *Task) setError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.doSetError(err)
}

func (t *Task) doSetError(err error) {
	t.lastDuration = time.Since(t.lastStart)
	t.lastExit = err
	t.running = false
	t.barColor = colorError
	t.setBarColor(t.barColor)

	if _, ok := t.handler.(*plugin.Handler); !ok && err != nil {
		t.handler = browser.NopScrollableFloatingHandler(
			handler.NopScrollableFloating(
				component.NewResponsiveString(
					err.Error(),
					component.StringResponsiveConfig{
						NoSplitWords: true,
						StringConfig: component.StringConfig{
							Alignment: component.SpanAlignmentCentered,
						},
					},
				),
			))
	}

	t.interrupt()
}

func (t *Task) setSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastDuration = time.Since(t.lastStart)
	t.lastExit = nil
	t.running = false
	t.barColor = colorSuccess
	t.setBarColor(t.barColor)
	t.interrupt()
}

func (t *Task) interrupt() {
	err := t.interrupter.Interrupt(context.Background())
	if err != nil {
		log.Errorf("task: interrupt: %v", err)
	}
}

func (t *Task) setBarColor(color tcell.Color) tcell.Color {
	if _, is := t.win.IsMinimized(); is {
		prev, _ := t.win.SetFrameAttr(term.Attributes{Bg: color})
		return prev.Bg
	}
	return 0
}

func (t *Task) setMaxWidthHeight(width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.maxWidth = width
	t.minWidth, t.minHeight = calcMinSize(width, height)
	t.minHeight = height / 8
}

func (t *Task) onFocus() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed.Load() {
		return
	}
	t.unminimize()
}

func (t *Task) onUnfocus() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed.Load() || t.win.Closed() {
		return
	}
	t.minimize()
}

func (t *Task) unminimize() {
	t.win.Unminimize()
	t.setBarColor(t.defaultBarColor)
}

func (t *Task) minimize() {
	switch t.MinimizeAlignment {
	case component.SpanAlignmentTop:
		t.win.MinimizeUp(minimizePadding)
	case component.SpanAlignmentBottom:
		t.win.MinimizeDown(minimizePadding)
	case component.SpanAlignmentLeft:
		t.win.MinimizeLeft(minimizePadding)
	default:
		t.win.MinimizeRight(minimizePadding)
	}
	t.setBarColor(t.barColor)
}

type pluginBuilder func(
	publisher browser.EventPublisher, notifications browser.Notifications,
	e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
	cmdAndArgs string, maxWidth int, opts ...plugin.Option,
) (browser.ScrollableFloating, error)
