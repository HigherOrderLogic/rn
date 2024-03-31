package vte

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	schemeapi "unstable.build/go-tui/api/scheme"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/parser"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
)

// Component implements a vte terminal emulator tui.Component.
type Component struct {
	mu        sync.Mutex
	terminal  schemeapi.Terminal
	executor  schemeapi.Executor
	clipboard clipboard.Register
	pty       workspaceapi.Pty
	shell     string
	watcher   workspaceapi.Watcher
	ctx       context.Context
	cancelCtx func()
	uri       workspaceapi.URI

	width, height     int
	parserHandler     parserHandler
	waitParserHandler *waitParserHandler
	parser            parser.Parser
	complete          bool
	selectionAttr     term.Attributes
	defAttr           term.Attributes
}

// NOTE: this is an integrator implementation, it shouldn't really do much other
// than creating a pty and initializing the vte parser and the parser handler.

// NewComponent allocates storage for a new Component and initializes it.
func NewComponent(
	t schemeapi.Terminal, e schemeapi.Executor,
	tm browser.TabManager, cfg Config,
) (*Component, error) {
	ret := new(Component)
	err := ret.Init(t, e, tm, cfg)
	return ret, err
}

// Init initializes it with the given dependencies and options.
func (t *Component) Init(
	term schemeapi.Terminal, e schemeapi.Executor,
	tm browser.TabManager, cfg Config,
) error {
	t.clipboard = cfg.Clipboard
	t.shell = cfg.Shell
	t.watcher = cfg.Watcher
	t.defAttr = cfg.Attributes
	t.selectionAttr = cfg.SelectionAttributes
	t.terminal = term
	t.executor = e

	t.ctx, t.cancelCtx = context.WithCancel(context.Background())
	err := t.createPty(cfg)
	if err != nil {
		return err
	}

	if cfg.ScheduleBell == nil || cfg.RingBell == nil {
		panic("nil bell function(s)")
	}

	t.parserHandler.init(&t.mu, t.pty, tm, t.clipboard,
		cfg.ScheduleBell, t.uri, cfg.NeedsAttentionAttributes)
	// start with pty slave file name as title
	t.parserHandler.SetTitle(t.uri.Name())
	var h parser.Handler = &t.parserHandler
	if log.IsLevelEnabled(log.TraceLevel) {
		h = parser.HandlerWithLogging("vte.parserHandler", h)
	}
	t.waitParserHandler = newWaitParserHandler(h)
	h = t.waitParserHandler
	t.parser.Init(h, new(parser.StdTimeout))
	t.SetDefaultAttributes(t.defAttr)
	return err
}

// Run must be called in a separate goroutine to start processing incoming
// data from the pty master.
func (t *Component) Run(updateChan chan struct{}) error {
	t.mu.Lock()
	complete := t.complete
	t.mu.Unlock()
	if complete {
		panic("called Run twice on vte.Component")
	}

	defer func() {
		t.mu.Lock()
		t.complete = true
		t.mu.Unlock()
	}()

	// interrupt at most at a reasonable fps. This improves
	// performance when program is dumping Kbs of content
	// into the terminal scroll.
	// buffer to 1, so we publish one last interrupt after
	// maxInterruptPerSecond since last interrupt
	ch := make(chan struct{}, 1)
	go func() {
		maxInterruptPerSecond := time.Duration(int(time.Second) / 30)
		timer := time.NewTimer(maxInterruptPerSecond)
		defer timer.Stop()
		defer close(updateChan)
		for {
			select {
			case <-ch:
			case <-t.ctx.Done():
				return
			}
			select {
			case updateChan <- struct{}{}:
			case <-t.ctx.Done():
				return
			}
			timer.Reset(maxInterruptPerSecond)
			select {
			case <-timer.C:
			case <-t.ctx.Done():
				return
			}
		}
	}()

	buf := make([]byte, os.Getpagesize())
	for {
		n, err := t.pty.Master.Read(buf[:])
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			t.parser.Advance(buf[i])
		}
		select {
		// Close was called, just return error
		case <-t.ctx.Done():
			return t.ctx.Err()
		// no interrupts in the last maxInterruptPeriod
		case ch <- struct{}{}:
		// an interrupt was requested in the last maxInterruptPeriod
		// don't request any further interrupts for now
		default:
		}
	}
}

// Title returns the Title of this Component.
func (t *Component) Title() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.parserHandler.title
}

// URI returns the raw URI of this terminal emulator.
func (t *Component) URI() workspaceapi.URI {
	return t.uri
}

// WriteToPty writes the given data to the underlying pty master.
func (t *Component) WriteToPty(data []byte) error {
	// t.log(log.TraceLevel, "WriteToPty: %s", string(data))
	_, err := t.pty.Master.Write(data)
	return err
}

// Resize resizes this component and returns an error if
// the call to resize the underlying pty failed.
func (t *Component) Resize(width, height int) error {
	if t.pty.Master == nil {
		return fmt.Errorf("terminal is not running")
	}

	if t.width == width && t.height == height {
		// some programs will not re-print if width and height
		// are the same, but resizing buffers does clear all the content
		// so we would be left with an empty screen buffer.
		return nil
	}

	err := t.terminal.SetPtySize(t.pty, width, height)
	if err != nil {
		return err
	}

	t.width = width
	t.height = height

	t.parserHandler.Resize(width, height)

	return nil
}

// ModeBracketedPaste returns whether bracketed paste mode is set.
func (t *Component) ModeBracketedPate() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.modeBracketedPaste
}

// MouseModeReportMouseClicks returns whether PrivateMode 1000 (MouseModeVT200) is set.
func (t *Component) MouseModeReportMouseClicks() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.modeReportMouseClicks
}

// MouseModeReportCellMouseMotion returns whether PrivateMode 1002 (MouseModeButtonEvent) is set.
func (t *Component) MouseModeReportCellMouseMotion() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.modeReportCellMouseMotion
}

// MouseModeReportAllMouseMotion returns whether PrivateMode 1003 (MouseModeAnyEvent) is set.
func (t *Component) MouseModeReportAllMouseMotion() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.modeReportAllMouseMotion
}

// MouseModeUtf8Mouse returns whether PrivateMode 1005 (MouseExtUTF) is set.
func (t *Component) MouseModeUtf8Mouse() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.modeUtf8Mouse
}

// MouseModeSgrMouse returns whether PrivateMode 1006 (MouseExtSGR) is set.
func (t *Component) MouseModeSgrMouse() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.modeSgrMouse
}

// CursorVisible returns whether the cursor should be rendered or not.
func (t *Component) CursorVisible() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return !t.parserHandler.cursorHidden &&
		t.parserHandler.modeShowCursor &&
		t.parserHandler.sync.buf.CursorAtScreen().Y < t.height
}

// CursorAtScreen returns the current coordinates of the cursor,
// relative to the screen.
func (t *Component) CursorAtScreen() term.Coordinates {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.parserHandler.sync.buf.CursorAtScreen()
}

// CursorAtScroll returns the current coordinates of the cursor,
// relative to the underlying scroll.
func (t *Component) CursorAtScroll() term.Coordinates {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.cursorAtScroll()
}

// CursorStyle returns the term.CursorStyle that should be rendered
// with this Component, if IsCursorVisible returns true.
func (t *Component) CursorStyle() term.CursorStyle {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.parserHandler.modeBlinkingCursor {
		return t.parserHandler.cursorStyle
	}

	switch t.parserHandler.cursorStyle {
	case term.CursorStyleSteadyUnderline:
		return term.CursorStyleBlinkingUnderline
	case term.CursorStyleSteadyBar:
		return term.CursorStyleBlinkingBar
	default:
		return term.CursorStyleBlinkingBlock
	}
}

// Height returns the height of the underlying terminal buffer in lines.
func (t *Component) Height() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.parserHandler.sync.buf.Rows()
}

// MaxWidth returns the maximum width of the underlying terminal buffer in columns.
func (t *Component) MaxWidth() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		return t.parserHandler.sync.altBuf.MaxColumns()
	}
	return t.parserHandler.sync.primBuf.MaxColumns()
}

// ScrollDown scrolls down content of this terminal emulator.
func (t *Component) ScrollDown(count int) (ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.parserHandler.useAlt {
		return
	}
	// vte scroll up/down has inverse semantics
	return t.parserHandler.scrollUp(count, true)
}

// ScrollUp scrolls up the content of this terminal emulator.
func (t *Component) ScrollUp(count int) (ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.parserHandler.useAlt {
		return
	}
	// vte scroll up/down has inverse semantics
	return t.parserHandler.scrollDown(count, true)
}

// ScrollTop scrolls up the content of this terminal emulator to the top.
func (t *Component) ScrollTop() (ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.parserHandler.useAlt {
		return
	}

	// vte scroll up/down has inverse semantics
	buffer := t.parserHandler.sync.primBuf
	offset := buffer.Offset()
	return t.parserHandler.scrollDown(offset.Y, true)
}

// ScrollBottom scrolls down the content of this terminal emulator to the bottom.
func (t *Component) ScrollBottom() (ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.parserHandler.useAlt {
		return
	}

	// vte scroll up/down has inverse semantics
	buffer := t.parserHandler.sync.primBuf
	offset := buffer.Offset()
	max := buffer.MaxOffset()
	return t.parserHandler.scrollUp(max-offset.Y, true)
}

// SetDefaultAttributes updates the default attributes of this terminal emulator.
func (t *Component) SetDefaultAttributes(attrs term.Attributes) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.parserHandler.sync.primBuf.SetDefaultAttributes(attrs)
	t.parserHandler.sync.altBuf.SetDefaultAttributes(attrs)
}

// IsApplicationCursorKeysMode returns whether cursor keys mode is enabled.
// https://vt100.net/docs/vt510-rm/chapter2.html#S2.8.11
func (t *Component) IsApplicationCursorKeysMode() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.parserHandler.modeCursorKeys
}

// IsAltBuffer returns true if underlying buffer utilizes is the alternate buffer.
func (t *Component) IsAltBuffer() bool {
	return t.parserHandler.useAlt
}

// IsNewLineMode returns whether new line mode is enabled.
// https://vt100.net/docs/vt510-rm/chapter2.html#S2.5.13
func (t *Component) IsNewLineMode() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.parserHandler.modeLineFeedNewLine
}

// Draw satisfies tui.Component.
func (t *Component) Draw(w term.Writer) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		t.parserHandler.sync.altBuf.Draw(w)
	} else {
		t.parserHandler.sync.primBuf.Draw(w)
	}

	t.drawSelection(w)
}

// IsComplete returnes whether this terminal has stopped processing
// data from the pty file.
func (t *Component) IsComplete() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.complete
}

// Unselect clears this Component's selection.
func (t *Component) Unselect() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		t.parserHandler.sync.altBuf.Unselect()
	} else {
		t.parserHandler.sync.primBuf.Unselect()
	}
}

// Select anchors the current cursor position as the start and end of a text selection.
func (t *Component) Select(pos term.Coordinates) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		t.parserHandler.sync.altBuf.Select(pos)
	} else {
		t.parserHandler.sync.primBuf.Select(pos)
	}
}

// Select anchors the current cursor position as the end of a text selection.
func (t *Component) SelectEnd(pos term.Coordinates) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		t.parserHandler.sync.altBuf.SelectEnd(pos)
	} else {
		t.parserHandler.sync.primBuf.SelectEnd(pos)
	}
}

// Select select the word under the current cursor position.
func (t *Component) SelectWordAt(pos term.Coordinates) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		t.parserHandler.sync.altBuf.SelectWordAt(pos)
	} else {
		t.parserHandler.sync.primBuf.SelectWordAt(pos)
	}
}

// SelectLine anchors the current cursor position as the start and end line of
// the text selection.
func (t *Component) SelectLine(pos term.Coordinates) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.parserHandler.useAlt {
		t.parserHandler.sync.altBuf.SelectLine(pos)
	} else {
		t.parserHandler.sync.primBuf.SelectLine(pos)
	}
}

// Selection returns the current selection or false if no
// text is currently selected.
func (t *Component) Selection() (data string, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var cells [][]term.Cell
	cells, ok = t.selection()
	if !ok {
		return
	}
	return cell.CellsToString(cells), ok
}

// OnFocusChange allows clients to report whether this vte.Component is on focus or not.
func (t *Component) OnFocusChange(inFocus bool) error {
	t.mu.Lock()
	cmd, ok := t.parserHandler.onFocusChange(inFocus)
	t.mu.Unlock()
	if !ok {
		return nil
	}
	err := t.WriteToPty([]byte(fmt.Sprintf("\x1b[%s", cmd)))
	if err != nil {
		return fmt.Errorf("write to pty: %w", err)
	}
	return nil
}

// PrimaryScroll returns the primary buffer's underlying component.Scroll.
func (t *Component) PrimaryScroll() *component.Scroll {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parserHandler.sync.primBuf.Scroll()
}

// Locker returns the underlying sync.Locker used by this Component
// to synchronize access to the internal state.
func (t *Component) Locker() sync.Locker {
	return &t.mu
}

// Close assumes lock has been acquired by caller
func (t *Component) Close() (ret error) {
	defer t.cancelCtx()

	if err := t.pty.Slave.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := t.pty.Master.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	return ret
}

func (t *Component) log(level log.Level, line string, params ...interface{}) {
	log.WithField(logging.KeyClass, "vte.Component").
		Logf(level, line, params...)
}

func (t *Component) createPty(cfg Config) error {
	pty, err := t.terminal.NewPty(t.ctx)
	if err != nil {
		return fmt.Errorf("new pty: %v", err)
	}
	shell := t.shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "sh"
	}

	t.uri, err = workspaceapi.CurrentUserHostURI(pty.Slave.Name())
	if err != nil {
		return fmt.Errorf("pty URI: %v", err)
	}

	cmdAndArgs := strings.Split(shell, " ")
	cmd := workspaceapi.Cmd{
		Path: cmdAndArgs[0],
		Args: cmdAndArgs[1:],
		SysProcAttr: &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
		},
		Watcher: t.watcher,
	}

	cmd.Stdout = pty.Slave
	cmd.Stderr = pty.Slave
	cmd.Stdin = pty.Slave

	_, retErr := t.executor.StartCommand(t.ctx, cmd)
	if retErr != nil {
		retErr = fmt.Errorf("start command: %v", retErr)
		if err := pty.Master.Close(); err != nil {
			err = fmt.Errorf("close pty: %v", err)
			retErr = multierr.Append(retErr, err)
		}
	}
	if retErr != nil {
		return retErr
	}
	t.pty = pty
	return nil
}

func (t *Component) selection() (cells [][]term.Cell, ok bool) {
	if t.parserHandler.useAlt {
		cells, ok = t.parserHandler.sync.altBuf.Selection()
	} else {
		cells, ok = t.parserHandler.sync.primBuf.Selection()
	}
	return
}

func (t *Component) drawSelection(w term.Writer) {
	var from, to, offset term.Coordinates
	var mode text.SelectMode
	var ok bool
	if t.parserHandler.useAlt {
		mode, from, to, ok = t.parserHandler.sync.altBuf.SelectionCoordinatesAtScroll()
	} else {
		mode, from, to, ok = t.parserHandler.sync.primBuf.SelectionCoordinatesAtScroll()
		offset = t.parserHandler.sync.primBuf.Offset()
	}
	if !ok {
		return
	}
	for y := from.Y; y <= to.Y; y++ {
		xStart, xEnd := 0, t.parserHandler.sync.buf.Columns(y)
		if y == from.Y && mode == text.StandardSelection {
			xStart = from.X
		}
		if y == to.Y && mode == text.StandardSelection {
			xEnd = to.X
		}
		for x := xStart; x < xEnd; x++ {
			pos := term.Coordinates{X: x, Y: y}
			c := t.parserHandler.sync.buf.CellAt(pos)
			if c == nil {
				continue
			}
			posAtScreen := term.CoordinatesDiff(pos, offset)
			if posAtScreen.Y < 0 || posAtScreen.Y >= t.height ||
				posAtScreen.X < 0 || posAtScreen.X >= t.width {
				continue
			}
			w.SetCell(posAtScreen, term.Cell{
				Ch:         c.Ch,
				Attributes: t.selectionAttr,
				Width:      c.Width,
				Combining:  c.Combining,
			})
		}
	}
}
func (t *Component) cursorAtScroll() term.Coordinates {
	return t.parserHandler.sync.buf.CursorAtScroll()
}

func (t *Component) scheduleBellCallback(
	timeout time.Duration, callback func(),
) (ok bool) {
	return t.waitParserHandler.scheduleBellCallback(timeout, callback)
}
