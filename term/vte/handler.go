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

package vte

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
)

var _ tui.Handler = (*Handler)(nil)

const handleTimeout = 50 * time.Millisecond

// Handler is a terminal emulator that satisfies tui.Handler.
type Handler struct {
	comp          *Component
	publisher     browser.EventPublisher
	notifications browser.Notifications
	handleTimer   *time.Timer
	vi            viHandler
	ctx           context.Context
	cancelCtx     func()

	checkSystemBell bool
	modalEnabled    bool
	viMode          bool
	mouse           *text.Mouse
	mouseDriver     *mouseDriver

	bracketedPaste bool
	closed         atomic.Bool // whether Close has been called
	exit           atomic.Bool // whether running shell/program has exited
	width, height  int
	updateCh       chan struct{}
	sema           chan struct{}
}

// NewHandler allocates storage for a new Handler and initializes it.
func NewHandler(
	publisher browser.EventPublisher, n browser.Notifications,
	terminal schemeapi.Terminal, executor schemeapi.Executor,
	tm browser.TabManager, config Config, initialCmd string,
) (*Handler, error) {
	ret := new(Handler)
	err := ret.Init(publisher, n, terminal, executor, tm, config, initialCmd)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Init initializes this handler.
func (e *Handler) Init(
	publisher browser.EventPublisher, n browser.Notifications,
	termapi schemeapi.Terminal, executor schemeapi.Executor,
	tm browser.TabManager, config Config, initialCmd string,
) error {
	e.publisher = publisher
	e.notifications = n
	e.handleTimer = time.NewTimer(handleTimeout)
	// by default we want to check if system bell works
	// when modal mode is enabled
	e.checkSystemBell = true
	// leave in idle state so we can call Reset directly in handle
	if !e.handleTimer.Stop() {
		<-e.handleTimer.C
	}

	comp, err := NewComponent(termapi, executor, tm, config)
	if err != nil {
		return err
	}
	e.comp = comp

	if config.Modal {
		e.modalEnabled = true
		e.vi.init(e.comp, config)
	}

	// set size hint before running firsrt program so output is correctly captured
	if config.WidthHint != 0 || config.HeightHint != 0 {
		err := e.comp.Resize(config.WidthHint, config.HeightHint)
		if err != nil {
			return fmt.Errorf("set initial pty size: %v", err)
		}
	}
	if initialCmd != "" {
		if err := e.comp.WriteToPty([]byte(initialCmd)); err != nil {
			return fmt.Errorf("write to pty: %v", err)
		}
	}
	e.mouseDriver = &mouseDriver{t: e.comp, clipboard: config.Clipboard}
	e.mouse = text.NewMouse(e.mouseDriver)
	e.ctx, e.cancelCtx = context.WithCancel(context.Background())

	e.updateCh = make(chan struct{}, 1)
	e.sema = make(chan struct{})
	go debug.CapturePanicReport(func() {
		logErr := e.comp.Run(e.updateCh)
		e.exit.Store(true)
		_ = e.publisher.PublishEvent(term.Event{Type: term.EventNone})
		if e.closed.Load() {
			e.log(log.DebugLevel, "terminal run: ok")
			return
		}
		if logErr != nil && !errors.Is(logErr, io.EOF) && !errors.Is(logErr, context.Canceled) {
			e.log(log.ErrorLevel, "terminal run: %v", logErr)
			_ = e.notifications.Notify(notifications.LevelError, "terminal run: %v", logErr)
		}
	})

	go debug.CapturePanicReport(func() {
		for {
			select {
			case <-e.sema:
				e.sema <- struct{}{}
			case <-e.ctx.Done():
				return
			case <-e.updateCh:
			}
			err = e.publisher.PublishEvent(term.Event{Type: term.EventInterrupt})
			if err != nil {
				e.log(log.ErrorLevel, "interrupt: %s", err)
			}
		}
	})

	return nil
}

// Component returns the underlying vte.Component.
func (e *Handler) Component() *Component {
	return e.comp
}

// SetDefaultAttributes sets the background and foreground attributes
// of the underlying buffer.
func (e *Handler) SetDefaultAttributes(attr term.Attributes) {
	if e.viMode {
		e.vi.setDefaultAttributes(attr)
	}
	e.comp.SetDefaultAttributes(attr)
}

// Resize satisfies tui.Component.
func (e *Handler) Resize(width, height int) {
	// avoid divisions by 0 in terminal impl
	if width == 0 || height == 0 {
		return
	}

	e.width, e.height = width, height
	// primary buffer resets the offset to max offset after every resize
	// so we need to, reset the cursor position
	var modalCursorPos term.Coordinates
	if e.modalEnabled {
		if e.viMode {
			modalCursorPos = e.vi.cursorAtScroll()
		}
		e.vi.Resize(width, height)
	}

	err := e.comp.Resize(width, height)
	if err != nil {
		e.log(log.ErrorLevel, "terminal set size: %s", err)
		// do not notify if already closed
		if !e.exit.Load() {
			_ = e.notifications.Notify(notifications.LevelError, "terminal set size: %v", err)
		}
		return
	}

	if e.viMode {
		e.vi.setCursorAtScroll(modalCursorPos)
	}
}

// Draw satisfies tui.Component.
func (e *Handler) Draw(w term.Writer) {
	if e.viMode {
		e.vi.Draw(w)
		return
	}
	e.comp.Draw(w)
}

// Handle satisfies tui.Handler.
func (e *Handler) Handle(ev term.Event) (exit, handled bool) {
	exit = e.exit.Load()
	if exit {
		e.log(log.DebugLevel, "Handle: exit")
		return
	}
	if e.viMode {
		exit, handled := e.vi.Handle(ev)
		if exit {
			e.exitViMode()
		}
		return false, handled
	}

	if e.modalEnabled && !e.comp.IsAltBuffer() && ev.Key == term.KeyEsc && ev.Mod == 0 {
		handled = true
		e.enterViMode()

		if !e.checkSystemBell {
			return
		}
		e.checkSystemBell = false
		e.vi.systemCanDispatchBell(func(err error) {
			if err == nil {
				return
			}
			e.log(log.WarnLevel, "bell dispatch check failed: %v", err)
			e.notify(notifications.LevelWarn,
				"VTE modal (vi) mode enabled but system "+
					"has no audible bell configured. "+
					"You need to enable your system's "+
					"audible bell for this feature to work correctly.")
		})
		return
	}
	var raw []byte
	handled, raw = e.handleInput(ev)
	if exit || handled || len(raw) == 0 {
		return
	}

	if !e.bracketedPaste {
		select {
		case e.sema <- struct{}{}:
		case <-e.ctx.Done():
			exit = true
			return
		}
		defer func() { <-e.sema }()
	}

	err := e.comp.WriteToPty(raw)
	if err != nil {
		e.log(log.ErrorLevel, "write to pty: %s", err)
		e.notify(notifications.LevelError, "write to pty: %v", err)
		return
	}

	if !e.bracketedPaste {
		// do not scroll to bottom in all cases or it could
		// interfere with interactive program that uses primary buffer
		if ev.Type == term.EventKey && ev.Ch == 'c' && ev.Mod == term.ModCtrl {
			e.comp.ScrollBottom()
		}
		e.handleTimer.Reset(handleTimeout)
		select {
		case <-e.handleTimer.C:
			e.handleTimer.Stop()
		case <-e.ctx.Done():
			exit = true
		case <-e.updateCh:
			handled = true
			if !e.handleTimer.Stop() {
				<-e.handleTimer.C
			}
		}
	} else {
		handled = true
	}
	return
}

// OnFocusChange allows clients to report whether this vte.Handler is on focus or not.
func (e *Handler) OnFocusChange(inFocus bool) {
	err := e.comp.OnFocusChange(inFocus)
	if err != nil {
		e.notify(notifications.LevelError, "failed to report focus changed: %v", err)
	}
}

// Cursor satisfies tui.Handler.
func (e *Handler) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	if e.exit.Load() {
		return
	}
	if e.viMode {
		return e.vi.Cursor()
	}
	if !e.comp.CursorVisible() {
		return
	}

	show = true
	pos = e.comp.CursorAtScreen()
	style = e.comp.CursorStyle()
	if !e.comp.IsAltBuffer() && style == term.CursorStyleDefault {
		style = term.CursorStyleSteadyBar
	}

	return
}

// Selection satisfies tui.Handler.
func (e *Handler) Selection() (data string, ok bool) {
	if e.exit.Load() {
		return
	}
	if e.viMode {
		return e.vi.Selection()
	}
	return e.comp.Selection()
}

// Man satisfies tui.Handler.
func (e *Handler) Man() tui.Manual {
	if e.viMode {
		return e.vi.Man()
	}
	panic("TODO")
}

// SeekUp satisfies component.Scrollable.
func (e *Handler) SeekUp() bool {
	return e.comp.ScrollUp(1)
}

// SeekDown satisfies component.Scrollable.
func (e *Handler) SeekDown() bool {
	return e.comp.ScrollDown(1)
}

// SeekOffset satisfies component.Scrollable.
func (e *Handler) SeekOffset() int {
	return e.comp.ScrollOffset()
}

// MaxSeekOffset satisfies component.Scrollable.
func (e *Handler) MaxSeekOffset() int {
	return e.comp.MaxScrollOffset()
}

// Close closes this terminal emulator and all the resources
// associated with it.
func (e *Handler) Close() error {
	e.log(log.TraceLevel, "close called")

	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	// undo circular dependency
	e.mouseDriver.clipboard = nil
	e.mouseDriver = nil

	e.cancelCtx()
	e.handleTimer.Stop()

	var ret error
	if err := e.comp.Close(); err != nil {
		ret = multierr.Append(ret, err)
		e.log(log.ErrorLevel, "terminal close: %s", err)
	}
	// we can't remove /dev/pts files so leave it up to the system
	return ret
}

func (e *Handler) handleInput(ev term.Event) (handled bool, raw []byte) {
	if isStart := ev.Type == term.EventPasteStart; isStart || ev.Type == term.EventPasteEnd {
		e.bracketedPaste = isStart
		programBracketedMode := e.comp.ModeBracketedPaste()
		e.log(log.DebugLevel, "handled bracketed paste start=%t,"+
			" programBracketedMode : %v", isStart, programBracketedMode)
		// if bracketed paste mode is not set, then we are done
		// otherwise, write bracket start via ev.Raw
		if programBracketedMode {
			raw = ev.Raw
		} else {
			// nothing else to do
			handled = true
		}
		return
	}

	if ev.Type == term.EventMouse {
		e.mouseDriver.hookRawBytes = nil
		_, handled = e.mouse.Handle(ev)
		raw = e.mouseDriver.hookRawBytes
		// raw bytes should be sent directly only
		// if we didn't handle mouse event
		if len(raw) != 0 {
			handled = false
		}
		e.log(log.TraceLevel, "input: mouse: handled=%t, raw=%q", handled, raw)
		return
	}

	if ev.Ch == 'l' && ev.Mod == term.ModCtrl {
		e.mouseDriver.ClearSelection()
	} else if ev.Mod == 0 && ev.Key == term.KeyEsc {
		e.comp.Unselect()
	}

	// we cannot simply send raw bytes coming from termbox.
	// The running program sends escape sequences to e.terminal via stdout which
	// configure the program's I/O mode and so this might not might not necessarily
	// match termbox's configuration.
	raw, ok := mapKeyToEscapeSequence(e.comp, ev)
	if !ok {
		raw = ev.Raw
	}
	return
}

func (e *Handler) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "vte.Handler",
	}).Logf(level, msg, args...)
}

func (e *Handler) notify(level notifications.Level, msg string, args ...any) {
	if err := e.notifications.Notify(level, msg, args...); err != nil {
		e.log(log.ErrorLevel, "notify: %v", err)
	}
}

func (e *Handler) enterViMode() {
	e.viMode = true
	e.comp.Unselect()
	e.vi.enterViMode(e.comp.CursorAtScroll())
}

func (e *Handler) exitViMode() {
	e.viMode = false
}
