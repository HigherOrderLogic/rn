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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/mouse"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
)

var _ tui.Handler = (*Handler)(nil)

// defaultHandleTimeout bounds how long Handle will wait for the pty
// to produce an update after writing a keypress. It is a redraw
// debounce, NOT a gate on whether the event is considered handled.
const defaultHandleTimeout = 50 * time.Millisecond

// isNormalPtyExit reports whether the pty read loop ended because the
// child process exited rather than because something went wrong. Linux
// fails the master read with EIO once the last slave descriptor closes,
// where macOS reports EOF. For a remote workspace the error crosses the
// RPC boundary untyped, so the message is matched as well.
func isNormalPtyExit(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) ||
		errors.Is(err, syscall.EIO) {
		return true
	}
	return strings.Contains(err.Error(), syscall.EIO.Error())
}

// Handler is a terminal emulator that satisfies tui.Handler.
type Handler struct {
	comp          *Component
	publisher     browser.EventPublisher
	notifications browser.Notifications
	handleTimer   *time.Timer
	handleTimeout time.Duration
	vi            viHandler
	ctx           context.Context
	cancelCtx     func()

	modalEnabled bool
	viMode       bool
	mouse        *mouse.Mouse
	mouseDriver  *mouseDriver

	bracketedPaste    bool
	bracketedPasteBuf bytes.Buffer

	closed        atomic.Bool // whether Close has been called
	exit          atomic.Bool // whether running shell/program has exited
	width, height int
	updateCh      chan struct{}
	sema          chan struct{}
}

// NewHandler allocates storage for a new Handler and initializes it.
func NewHandler(
	publisher browser.EventPublisher, n browser.Notifications,
	terminal schemeapi.Terminal, executor schemeapi.Executor,
	tm browser.TabManager, config Config,
) (*Handler, error) {
	ret := new(Handler)
	err := ret.Init(publisher, n, terminal, executor, tm, config)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Init initializes this handler.
func (e *Handler) Init(
	publisher browser.EventPublisher, n browser.Notifications,
	termapi schemeapi.Terminal, executor schemeapi.Executor,
	tm browser.TabManager, config Config,
) error {
	// wrap clipboard to provide stitch newlines on paste
	// depending on the copy mode.
	config.Clipboard = stitchingClipboard{root: config.Clipboard}
	e.publisher = publisher
	e.notifications = n
	e.handleTimeout = defaultHandleTimeout
	e.handleTimer = time.NewTimer(e.handleTimeout)
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

	e.mouseDriver = &mouseDriver{t: e.comp, clipboard: config.Clipboard}
	e.mouse = mouse.New(e.mouseDriver)
	e.ctx, e.cancelCtx = context.WithCancel(context.Background())

	e.updateCh = make(chan struct{}, 1)
	e.sema = make(chan struct{})
	go debug.CapturePanicReport(func() {
		logErr := e.comp.Run(e.updateCh)
		e.exit.Store(true)
		if err := e.publisher.PublishEvent(term.Event{Type: term.EventNone}); err != nil {
			e.log(log.ErrorLevel, "pty exit publish: %s", err)
		}
		if e.closed.Load() {
			e.log(log.DebugLevel, "terminal run: ok")
			return
		}
		if logErr != nil && !isNormalPtyExit(logErr) {
			e.log(log.ErrorLevel, "terminal run: %v", logErr)
			_, _ = e.notifications.Notify(browserapi.LevelError,
				"terminal run: %v", logErr)
		}
	})

	go debug.CapturePanicReport(func() {
		for {
			// Park the publisher while Handle is running so a
			// keystroke that triggers a multi-flush repaint produces
			// at most one EventInterrupt downstream. The handshake
			// mirrors Handle's send/receive on the same channel.
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

// SystemCanDispatchBell tests whether the underlying vte is able
// to dispatch a bell by calling the given callback with an error,
// if there was one. There's a fixed timeout of 3 seconds.
func (e *Handler) SystemCanDispatchBell(callback func(error)) {
	e.comp.systemCanDispatchBell(callback)
}

// Component returns the underlying vte.Component.
func (e *Handler) Component() *Component {
	return e.comp
}

// Snapshot returns a durable snapshot of the terminal's rendered
// buffers. It captures output/history, not the live pty process.
func (e *Handler) Snapshot() (Snapshot, error) {
	return e.comp.Snapshot()
}

// SnapshotInto behaves like Snapshot but copies the active buffer's
// cells into dst, reusing dst's capacity. See Component.SnapshotInto for
// the dst ownership contract.
func (e *Handler) SnapshotInto(dst [][]term.Cell) (Snapshot, error) {
	return e.comp.SnapshotInto(dst)
}

// Version returns the underlying Component's grid revision. See
// Component.Version.
func (e *Handler) Version() uint64 {
	return e.comp.Version()
}

// DrawSnapshot paints the active terminal grid to w and returns a
// Snapshot of the same grid taken under one lock. See
// Component.DrawSnapshot. Unlike Draw it does not render the modal (vi)
// overlay; it is intended for non-modal embeddings that overlay on the
// terminal grid itself.
func (e *Handler) DrawSnapshot(w term.Writer, dst [][]term.Cell) (Snapshot, error) {
	return e.comp.DrawSnapshot(w, dst)
}

// RestoreFromSnapshot restores a saved terminal snapshot into this
// live terminal emulator.
//
// Component.RestoreFromSnapshot mutates the primary buffer cells in
// place — the *cell.Buffer identity (and therefore every editor/scroll
// reference that viHandler captured at init) is preserved. The only
// state that becomes stale on the vi side is the cursor and the scroll
// offset, so we just refresh those rather than re-initialising vi.
func (e *Handler) RestoreFromSnapshot(snapshot Snapshot) error {
	cursor, err := e.comp.RestoreFromSnapshot(snapshot)
	if err != nil {
		return err
	}
	if e.modalEnabled {
		e.vi.setCursorAtScroll(cursor)
	}
	return nil
}

// ClearPrimaryBuffer resets the primary buffer.
func (e *Handler) ClearPrimaryBuffer() (ok bool) {
	if e.comp.IsAltBuffer() {
		return
	}
	e.comp.Unselect()
	ok = e.comp.ClearPrimaryBuffer()
	if !ok {
		return
	}
	if e.viMode {
		// re-entering vi mode will reset cursor/offset for vi handler
		e.exitViMode()
	}
	return
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
			_, _ = e.notifications.Notify(browserapi.LevelError,
				"terminal set size: %v", err)
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
		cursor := e.comp.CursorAtScroll()
		e.comp.systemCanDispatchBell(func(err error) {
			if err == nil {
				e.enterViMode(cursor)
				return
			}
			msg := "You pressed <esc>, which would enable modal (vi) mode, " +
				"but it cannot be enabled because the shell's " +
				"audible bell is currently unavailable. " +
				"Ensure that the shell's audible bell is configured and " +
				"working correctly. You can test it in your terminal with `printf '\\a'`."
			e.log(log.WarnLevel, "%s: %v", msg, err)
			if _, err := e.notifications.NotifyOnce(browserapi.LevelWarn, "%s", msg); err != nil {
				e.log(log.ErrorLevel, "notify: %v", err)
			}
		})
		return
	}

	switch ev.Mod {
	case 0, term.ModCtrl, term.ModCtrlShift, term.ModShift:
	default:
		// no other modifiers are handled by vte
		return
	}

	var raw []byte
	handled, raw = e.handleInput(ev)
	if exit || handled || len(raw) == 0 {
		return
	}

	select {
	case e.sema <- struct{}{}:
	case <-e.ctx.Done():
		exit = true
		return
	}
	defer func() { <-e.sema }()

	err := e.comp.WriteToPty(raw)
	if err != nil {
		e.log(log.ErrorLevel, "write to pty: %s", err)
		e.notify(browserapi.LevelError, "write to pty: %v", err)
		return
	}
	// Mark the event handled as soon as it is written to the pty.
	// Whether the pty has finished echoing the bytes back yet is
	// orthogonal to whether we consumed the event. Gating `handled`
	// on the round-trip used to break callers that chain into a key
	// sequencer: over a high-latency transport (e.g. an SSH workspace
	// where pty bytes round-trip via workspacerpc), the echo arrives
	// after handleTimeout and we returned handled=false. The IDE
	// sequencer would then treat the keypress as an unhandled event
	// for sequence matching and re-issue it on timeout, producing
	// duplicated input (e.g. typing "g" surfaced as "gg" because
	// "g" is the prefix of "gg"/"gf" bindings).
	handled = true

	// do not scroll to bottom in all cases or it could
	// interfere with interactive program that uses primary buffer
	if ev.Type == term.EventKey && ev.Ch == 'c' && ev.Mod == term.ModCtrl {
		e.comp.ScrollBottom()
		e.log(log.TraceLevel, "written cltr-c to pty: %q", raw)
	}
	// Debounce: give the embedded program up to handleTimeout to flush
	// its post-keystroke repaint before returning. One update is
	// consumed directly off updateCh so it does not redundantly wake
	// the publisher we just parked.
	e.handleTimer.Reset(e.handleTimeout)
	select {
	case <-e.handleTimer.C:
		e.handleTimer.Stop()
	case <-e.ctx.Done():
		exit = true
	case <-e.updateCh:
		if !e.handleTimer.Stop() {
			<-e.handleTimer.C
		}
	}
	return
}

// OnFocusChange allows clients to report whether this vte.Handler is on focus or not.
func (e *Handler) OnFocusChange(inFocus bool) {
	err := e.comp.OnFocusChange(inFocus)
	if err != nil {
		e.notify(browserapi.LevelError, "failed to report focus changed: %v", err)
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

// SeekUp satisfies component.Scrollable.
func (e *Handler) SeekUp() bool {
	if e.viMode {
		return e.vi.SeekUp()
	}
	return e.comp.ScrollUp(1)
}

// SeekDown satisfies component.Scrollable.
func (e *Handler) SeekDown() bool {
	if e.viMode {
		return e.vi.SeekDown()
	}
	return e.comp.ScrollDown(1)
}

// SeekOffset satisfies component.Scrollable.
func (e *Handler) SeekOffset() int {
	if e.viMode {
		return e.vi.SeekOffset()
	}
	return e.comp.ScrollOffset()
}

// MaxSeekOffset satisfies component.Scrollable.
func (e *Handler) MaxSeekOffset() int {
	if e.viMode {
		return e.vi.MaxSeekOffset()
	}
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
	if (ev.Type == term.EventKey || ev.Type == term.EventRaw) && e.bracketedPaste {
		e.bracketedPasteBuf.Write(ev.Raw)
		handled = true
		return
	}
	if isStart := ev.Type == term.EventPasteStart; isStart || ev.Type == term.EventPasteEnd {
		e.bracketedPaste = isStart
		programBracketedMode := e.comp.ModeBracketedPaste()
		e.log(log.DebugLevel, "handled bracketed paste start=%t,"+
			" programBracketedMode : %v", isStart, programBracketedMode)
		if isStart {
			if programBracketedMode {
				raw = append(raw, ev.Raw...)
			} else {
				// handle bracketed paste when we receive EventPasteEnd
				handled = true
			}
			return
		}

		defer e.bracketedPasteBuf.Reset()
		if programBracketedMode {
			raw = e.bracketedPasteBuf.Bytes()
			// remove `\x1b` (escape sequence) and `\x03` (ctrl-c) to ensure it's
			// impossible for the pasted text to control the shell's behavior in any way
			raw = bytes.ReplaceAll(raw, []byte("\x1b"), nil)
			raw = bytes.ReplaceAll(raw, []byte("\x03"), nil)
			// start of paste sequence was written upon term.EventPasteStart
			// so append term.EventPasteEnd or end of paste sequence.
			raw = append(raw, ev.Raw...)
		} else {
			raw = e.bracketedPasteBuf.Bytes()
			// replace line breaks with a single carriage, to reproduce
			// the enter key as much as possible.
			raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\r"))
			raw = bytes.ReplaceAll(raw, []byte("\n"), []byte("\r"))
		}
		if !e.comp.IsAltBuffer() {
			e.comp.ScrollBottom()
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

func (e *Handler) notify(level browserapi.NotificationLevel, msg string, args ...any) {
	if _, err := e.notifications.Notify(level, msg, args...); err != nil {
		e.log(log.ErrorLevel, "notify: %v", err)
	}
}

func (e *Handler) enterViMode(cursor term.Coordinates) {
	e.viMode = true
	e.comp.Unselect()
	e.vi.enterViMode(cursor)
}

func (e *Handler) exitViMode() {
	e.viMode = false
}
