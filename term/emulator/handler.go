package emulator

import (
	"fmt"
	"math"
	"time"

	"github.com/ernestrc/blue/logging"
	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	schemeapi "unstable.build/go-tui/api/scheme"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/term"
	termutil "unstable.build/go-tui/term/emulator/util"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
	sysclip "unstable.build/go-tui/text/clipboard/system"
)

var _ tui.Handler = (*Handler)(nil)

// Handler is a tui.Handler that implements a terminal emulator.
type Handler struct {
	publisher     browser.EventPublisher
	notifications browser.Notifications

	mouse             *text.Mouse
	mouseDriver       *mouseDriver
	windowManipulator *windowManipulator
	terminal          *termutil.Terminal
	theme             *termutil.Theme
	defAttr           term.Attributes
	selectAttr        term.Attributes

	closed        bool
	width, height int
	updateCh      chan struct{}
	sema          chan struct{}
}

// New allocates storage for a new Handler and initializes it.
func New(
	publisher browser.EventPublisher, n browser.Notifications,
	terminal schemeapi.Terminal, executor schemeapi.Executor,
	config Config, initialCmd string,
) (*Handler, error) {
	ret := new(Handler)
	err := ret.Init(publisher, n, terminal, executor, config, initialCmd)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Init initializes this handler.
func (e *Handler) Init(
	publisher browser.EventPublisher, n browser.Notifications,
	terminal schemeapi.Terminal, executor schemeapi.Executor,
	config Config, initialCmd string,
) error {
	e.publisher = publisher
	e.notifications = n
	e.defAttr = config.Attributes
	e.selectAttr = config.SelectionAttributes

	e.theme = &termutil.Theme{Default: config.Attributes}
	e.windowManipulator = newWindowManipulator(n)
	opts := []termutil.Option{
		termutil.WithTheme(e.theme),
		termutil.WithWindowManipulator(e.windowManipulator),
	}
	if config.Shell != "" {
		opts = append(opts, termutil.WithShell(config.Shell))
	}
	if config.Watcher != nil {
		opts = append(opts, termutil.WithWatcher(config.Watcher))
	}
	e.terminal = termutil.New(terminal, executor, opts...)
	_, err := e.terminal.CreatePty()
	if err != nil {
		return err
	}
	// set size hint before running firsrt program so output is correctly captured
	if config.WidthHint != 0 || config.HeightHint != 0 {
		err := e.terminal.SetSize(uint16(config.HeightHint), uint16(config.WidthHint))
		if err != nil {
			return fmt.Errorf("set initial pty size: %v", err)
		}
	}
	if initialCmd != "" {
		if err := e.terminal.WriteToPty([]byte(initialCmd)); err != nil {
			return fmt.Errorf("write to pty: %v", err)
		}
	}
	e.windowManipulator.SetTitle(e.terminal.Pty().Slave.Name())
	clip, err := sysclip.NewRegister()
	if err != nil {
		e.log(log.WarnLevel, "system clipboard unsupported: %v", err)
		clip = clipboard.NewInMemory()
	}
	e.mouseDriver = &mouseDriver{t: e.terminal, clipboard: clip}
	e.mouse = text.NewMouse(e.mouseDriver)

	e.updateCh = make(chan struct{})
	e.sema = make(chan struct{})
	go func() {
		logErr := e.terminal.Run(e.updateCh)
		if err := e.publisher.PublishEventNone(); err != nil {
			err = fmt.Errorf("publish event: %s", err)
			logErr = multierr.Append(logErr, err)
		}
		if logErr != nil {
			e.log(log.ErrorLevel, "terminal run: %v", logErr)
			e.notifications.Notify(notifications.LevelError, "terminal run: %v", logErr)
		} else {
			e.log(log.DebugLevel, "terminal run: ok")
		}
	}()

	go func() {
		for {
			select {
			case <-e.sema:
				e.sema <- struct{}{}
			case _, ok := <-e.updateCh:
				if !ok {
					return
				}
			}
			err = e.publisher.Interrupt()
			if err != nil {
				e.log(log.ErrorLevel, "interrupt: %s", err)
			}
		}
	}()

	return nil
}

func (e *Handler) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "emulator.Handler",
	}).Logf(level, msg, args...)
}

// Resize satisfies tui.Component.
func (e *Handler) Resize(width, height int) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	// avoid divisions by 0 in terminal impl
	if width == 0 || height == 0 {
		return
	}
	e.width, e.height = width, height

	e.windowManipulator.ResizeInChars(height, width)
	err := e.terminal.SetSize(uint16(height), uint16(width))
	if err != nil {
		e.log(log.ErrorLevel, "terminal set size: %s", err)
		// do not notify if already closed
		if !e.closed {
			e.notifications.Notify(notifications.LevelError, "terminal set size: %v", err)
		}
	}
}

func (e *Handler) Draw(w term.Writer) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	e.drawContent(w)
	e.drawSelection(w)
}

func (e *Handler) Handle(ev term.Event) (exit, handled bool) {
	exit, handled, raw := e.handleInput(ev)
	if exit || handled || len(raw) == 0 {
		return
	}

	select {
	case e.sema <- struct{}{}:
	case _, ok := <-e.updateCh:
		if !ok {
			exit = true
		}
		e.log(log.TraceLevel, "handle: closed update chan")
		return
	}
	defer func() { <-e.sema }()

	err := e.terminal.WriteToPty(raw)
	if err != nil {
		e.log(log.ErrorLevel, "write to pty: %s", err)
		e.notifications.Notify(notifications.LevelError, "write to pty: %v", err)
		return
	}

	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
	case _, ok := <-e.updateCh:
		if !ok {
			exit = true
		}
		handled = true
	}
	return
}

// Height returns the height of the underlying terminal buffer in lines.
func (e *Handler) Height() int {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	return e.terminal.Height()
}

// MaxWidth returns the maximum width of the underlying terminal buffer in columns.
func (e *Handler) MaxWidth() int {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	return e.terminal.MaxWidth()
}

// Cursor satisfies tui.Handler.
func (e *Handler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	termbuf := e.terminal.GetActiveBuffer()

	if !termbuf.IsCursorVisible() {
		return term.Coordinates{}, 0, false
	}

	var style term.CursorStyle
	switch termbuf.GetCursorShape() {
	case termutil.CursorShapeBlinkingBlock:
		style = term.CursorStyleBlinkingBlock
	case termutil.CursorShapeDefault:
		style = term.CursorStyleDefault
	case termutil.CursorShapeSteadyBlock:
		style = term.CursorStyleSteadyBlock
	case termutil.CursorShapeBlinkingUnderline:
		style = term.CursorStyleBlinkingUnderline
	case termutil.CursorShapeSteadyUnderline:
		style = term.CursorStyleSteadyUnderline
	case termutil.CursorShapeBlinkingBar:
		style = term.CursorStyleBlinkingBar
	case termutil.CursorShapeSteadyBar:
		style = term.CursorStyleSteadyBar
	}

	return term.Coordinates{
		X: int(math.Min(float64(termbuf.CursorColumn()), float64(e.width-1))),
		Y: int(math.Min(float64(termbuf.CursorLine()), float64(e.height-1))),
	}, style, true
}

// Man satisfies tui.Handler.
func (e *Handler) Man() tui.Manual {
	panic("TODO")
}

// URI returns the uri of the emulated terminal.
func (e *Handler) URI() (workspaceapi.URI, error) {
	var name string
	func() {
		e.terminal.Lock()
		defer e.terminal.Unlock()
		name = e.terminal.GetTitle()
	}()

	return workspaceapi.CurrentUserHostURI(name)
}

// Title returns the title of the emulated terminal.
func (e *Handler) Title() string {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	return e.terminal.GetTitle()
}

// IsComplete returns wether the underlying command has completed.
func (e *Handler) IsComplete() bool {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	return e.terminal.IsComplete()
}

// ScrollUp scrolls the buffer down by number of lines.
func (e *Handler) ScrollDown(lines int) bool {
	if lines < 0 {
		return e.ScrollUp(-lines)
	}
	e.terminal.Lock()
	defer e.terminal.Unlock()

	buffer := e.terminal.GetActiveBuffer()
	offset := buffer.GetScrollOffset()
	buffer.ScrollDown(uint(lines))
	return offset != buffer.GetScrollOffset()
}

// ScrollUp scrolls the buffer up by number of lines.
func (e *Handler) ScrollUp(lines int) bool {
	if lines < 0 {
		return e.ScrollDown(-lines)
	}
	e.terminal.Lock()
	defer e.terminal.Unlock()

	buffer := e.terminal.GetActiveBuffer()
	offset := buffer.GetScrollOffset()
	buffer.ScrollUp(uint(lines))
	return offset != buffer.GetScrollOffset()
}

// ScrollTop scrolls to the top of the buffer.
func (e *Handler) ScrollTop() bool {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	buffer := e.terminal.GetActiveBuffer()
	var offset uint
	for {
		offset = buffer.GetScrollOffset()
		buffer.ScrollUp(10)
		if offset == buffer.GetScrollOffset() {
			break
		}
	}
	return offset != buffer.GetScrollOffset()
}

// ScrollBottom scrolls to the bottom of the buffer.
func (e *Handler) ScrollBottom() bool {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	buffer := e.terminal.GetActiveBuffer()
	offset := buffer.GetScrollOffset()
	buffer.ScrollToEnd()
	return offset != buffer.GetScrollOffset()
}

// Close closes this terminal emulator and all the resources
// associated with it.
func (e *Handler) Close() error {
	e.log(log.TraceLevel, "close called")

	e.terminal.Lock()
	defer e.terminal.Unlock()

	if e.closed {
		return nil
	}

	// undo circular dependency
	e.mouseDriver.clipboard = nil
	e.mouseDriver = nil
	e.closed = true

	var ret error
	if err := e.terminal.Close(); err != nil {
		ret = multierr.Append(ret, err)
		e.log(log.ErrorLevel, "terminal close: %s", err)
	}
	// we can't remove /dev/pts files so leave it up to the system
	return ret
}

func (e *Handler) drawRow(
	w term.Writer, termbuf *termutil.Buffer,
	viewY int, defattr term.Attributes,
) {
	maxX := uint16(math.Min(float64(e.width), float64(termbuf.ViewWidth())))
	for viewX := uint16(0); viewX < maxX; viewX++ {
		cell := termbuf.GetCell(viewX, uint16(viewY))
		pos := term.Coordinates{X: int(viewX), Y: viewY}
		if cell == nil || cell.Ch == 0 {
			w.SetCell(pos, term.Cell{Bg: defattr.Bg, Fg: defattr.Fg})
		} else {
			tcell := *cell
			if tcell.Fg == term.ColorDefault {
				tcell.Fg = defattr.Fg
			}
			if tcell.Bg == term.ColorDefault {
				tcell.Bg = defattr.Bg
			}
			w.SetCell(pos, tcell)
		}
	}
}

func (e *Handler) drawContent(w term.Writer) {
	termbuf := e.terminal.GetActiveBuffer()
	viewY := int(math.Min(float64(e.height), float64(termbuf.ViewHeight()))) - 1
	for ; viewY >= 0; viewY-- {
		e.drawRow(w, termbuf, viewY, e.defAttr)
	}
}

func (e *Handler) drawSelection(w term.Writer) {
	termbuf := e.terminal.GetActiveBuffer()
	_, selection := termbuf.GetSelection()
	if selection == nil {
		return
	}

	bg, fg := e.selectAttr.Bg, e.selectAttr.Fg

	for y := selection.Start.Line; y <= selection.End.Line; y++ {
		xStart, xEnd := 0, int(termbuf.ViewWidth())
		if y == selection.Start.Line {
			xStart = int(selection.Start.Col)
		}
		if y == selection.End.Line {
			xEnd = int(selection.End.Col)
		}
		for x := xStart; x <= xEnd; x++ {
			cell := termbuf.GetCell(uint16(x), uint16(y))
			if cell == nil {
				continue
			}
			ch := cell.Ch
			pos := term.Coordinates{X: x, Y: int(y)}
			w.SetCell(pos, term.Cell{Ch: ch, Fg: fg, Bg: bg})
		}
	}
}

func (e *Handler) handleInput(ev term.Event) (exit, handled bool, raw []byte) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	exit = e.closed
	if exit {
		e.log(log.TraceLevel, "input: exit")
		return
	}

	if ev.Type == term.EventMouse {
		e.mouseDriver.hookRawBytes = nil
		exit, handled = e.mouse.Handle(ev)
		raw = e.mouseDriver.hookRawBytes
		// raw bytes should be sent directly only
		// if we didn't handle mouse event
		if len(raw) != 0 {
			handled = false
		}
		e.log(log.TraceLevel, "input: mouse: exit=%t, handled=%t, raw=%q",
			exit, handled, raw)
		return
	}

	switch ev.Key {
	case term.KeyCtrlL:
		e.mouseDriver.ClearSelection()
	}

	// we cannot simply send raw bytes coming from termbox.
	// The running program sends escape sequences to e.terminal via stdout which
	// configure the program's I/O mode and so this might not might not necessarily
	// match termbox's configuration.
	raw, ok := mapKeyToEscapeSequence(e.terminal.GetActiveBuffer(), ev)
	if !ok {
		raw = ev.Raw
	}
	return
}
