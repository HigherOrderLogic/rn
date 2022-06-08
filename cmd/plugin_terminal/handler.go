package main

import (
	"fmt"
	"math"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	termutil "github.com/ernestrc/go-tui/cmd/plugin_terminal/util"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

type emulator struct {
	wm browser.WindowManager
	wp workspace.Workspace
	p  browser.EventPublisher
	m  browser.Messenger

	clipboard         string
	mouse             *text.Mouse
	mouseDriver       *mouseDriver
	windowManipulator *windowManipulator
	terminal          *termutil.Terminal
	theme             *termutil.Theme
	defAttr           term.Attributes
	selectAttr        term.Attributes

	closed        bool
	width, height int
	resizeErr     error
	updateCh      chan struct{}
	sema          chan struct{}
}

func newEmulator(
	wm browser.WindowManager, wp workspace.Workspace,
	p browser.EventPublisher, m browser.Messenger,
	c plugin.Clipboard,
	shell string, initialCmd string, title string,
	defAttr, selectionAttr term.Attributes,
) (*emulator, error) {
	ret := new(emulator)
	err := ret.init(wm, wp, p, m, c, shell, initialCmd,
		title, defAttr, selectionAttr)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (e *emulator) init(
	wm browser.WindowManager, wp workspace.Workspace,
	p browser.EventPublisher, m browser.Messenger,
	c plugin.Clipboard,
	shell string, initialCmd string, title string,
	defAttr, selectionAttr term.Attributes,
) error {
	e.wm = wm
	e.wp = wp
	e.p = p
	e.defAttr = defAttr
	e.selectAttr = selectionAttr
	e.m = m

	if c != nil {
		err := c.SetRegister(text.DefaultRegisterID, e)
		if err != nil {
			return err
		}
	}

	e.theme = &termutil.Theme{Default: defAttr}
	e.windowManipulator = newWindowManipulator(e.wm, e.m)
	opts := []termutil.Option{
		termutil.WithTheme(e.theme),
		termutil.WithWindowManipulator(e.windowManipulator),
	}
	if shell != "" {
		opts = append(opts, termutil.WithShell(shell))
	}
	if initialCmd != "" {
		opts = append(opts, termutil.WithInitialCommand(initialCmd))
	}
	e.terminal = termutil.New(opts...)
	cmd, err := e.terminal.CreatePty()
	if err != nil {
		return err
	}
	e.windowManipulator.SetTitle(e.terminal.Pty().Name() + title)
	e.mouseDriver = &mouseDriver{t: e.terminal, clipboard: c}
	e.mouse = text.NewMouse(e.mouseDriver)

	e.updateCh = make(chan struct{})
	e.sema = make(chan struct{})
	// set initial width/height to avoid panics
	// we control drawing outside of bounds in Draw
	go func() {
		err := e.terminal.Run(e.updateCh)
		if err != nil {
			log.Errorf("terminal.Run: %s", err)
		}
	}()

	go func() {
		var logErr error
		if err := cmd.Wait(); err != nil {
			err = fmt.Errorf("terminal.Cmd.Wait: %s", err)
			logErr = multierr.Append(logErr, err)
		}
		if err := e.Close(); err != nil {
			logErr = multierr.Append(logErr, err)
		}
		if err := e.p.PublishEventNone(); err != nil {
			err = fmt.Errorf("terminal.Cmd.Wait: %s", err)
			logErr = multierr.Append(logErr, err)
		}
		if logErr != nil {
			log.Errorf("(%p): %s", e, logErr)
		} else {
			log.Infof("(%p) terminal.Cmd.Wait: OK", e)
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
			err = e.p.PublishInterrupt()
			if err != nil {
				log.Errorf("PublishInterrupt: %s", err)
			}
		}
	}()

	return nil
}

func (e *emulator) Paste() (string, error) {
	log.Tracef("(%p).terminal.emulator.Paste: %s", e, e.clipboard)
	return e.clipboard, nil
}

func (e *emulator) Copy(data string) error {
	log.Tracef("(%p).terminal.emulator.Copy: %s", e, data)
	e.clipboard = data
	return nil
}

func (e *emulator) Resize(width, height int) {
	e.terminal.Lock()
	defer e.terminal.Unlock()
	// avoid SetSize error
	if e.closed {
		return
	}

	e.windowManipulator.ResizeInChars(height, width)
	e.width, e.height = width, height
	err := e.terminal.SetSize(uint16(height), uint16(width))
	if err != nil {
		log.Errorf("terminal.SetSize: %s", err)
	}
}

func (e *emulator) drawStr(str string, w term.Writer) {
	c := component.String(str)
	c.Resize(e.width, e.height)
	c.Draw(w)
}

func (e *emulator) drawRow(
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

func (e *emulator) drawContent(w term.Writer) {
	termbuf := e.terminal.GetActiveBuffer()
	viewY := int(math.Min(float64(e.height), float64(termbuf.ViewHeight()))) - 1
	for ; viewY >= 0; viewY-- {
		e.drawRow(w, termbuf, viewY, e.defAttr)
	}
}

func (e *emulator) drawSelection(w term.Writer) {
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

func (e *emulator) Draw(w term.Writer) {
	if e.resizeErr != nil {
		errStr := fmt.Sprintf("Error setting win size: %s", e.resizeErr)
		log.Errorf("(%p).emulator.Draw: resize err: %s", e, e.resizeErr)
		e.drawStr(errStr, w)
		return
	}

	e.terminal.Lock()
	defer e.terminal.Unlock()

	e.drawContent(w)
	e.drawSelection(w)
}

func (e *emulator) handleInput(ev term.Event) (exit, handled bool, raw []byte) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	exit = e.closed
	if exit {
		log.Debugf("(%p).emulator.Handle: closed", e)
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

// TODO figure out a way to not close tab on last ctrl-w on a terminal
//       - maybe should change all bindings thing to use meta rather than ctrl?
//       - could simply use ctrl-q
// TODO fix passing env variables to six plugin terminal
// TODO handle double ctrl-c by sending SIGINT to underlying terminal.
//	- expose as command and add default binding
// TODO reading from stdout should not block the next resize
// TODO add fullscreen capabilities to window manager
// TODO add moving of windows for window manager
// NOTE: ito reading large file from stdout blocking the whole editor
// what's probably happenning is a thundering herd effect
// calls to publish interrupt are flooding client and server goroutine pools
// so a resize is unlikely to go through responsively
// TODO implement adding SysProcAttr to workspace Start command
// then hook into creaty.pty to use workspace command
func (e *emulator) Handle(ev term.Event) (exit, handled bool) {
	exit, handled, raw := e.handleInput(ev)
	if exit || handled || len(raw) == 0 {
		return
	}

	e.sema <- struct{}{}
	defer func() { <-e.sema }()

	err := e.terminal.WriteToPty(raw)
	if err != nil {
		log.Errorf("(%p).emulator.Handle: %s", e, err)
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

func (e *emulator) Cursor() (term.Coordinates, bool) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	termbuf := e.terminal.GetActiveBuffer()

	if !termbuf.IsCursorVisible() {
		return term.Coordinates{}, false
	}

	return term.Coordinates{
		X: int(math.Min(float64(termbuf.CursorColumn()), float64(e.width-1))),
		Y: int(math.Min(float64(termbuf.CursorLine()), float64(e.height-1))),
	}, true
}

func (e *emulator) Man() tui.Manual {
	panic("TODO")
}

func (e *emulator) URI() (workspace.URI, error) {
	var name string
	func() {
		e.terminal.Lock()
		defer e.terminal.Unlock()
		name = e.terminal.GetTitle()
	}()

	return e.wp.URI(name)
}

func (e *emulator) Title() string {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	return e.terminal.GetTitle()
}

func (e *emulator) Close() error {
	log.Tracef("(%p).emulator.Close", e)

	e.terminal.Lock()
	defer e.terminal.Unlock()

	if e.closed {
		return nil
	}

	// undo circular dependency
	e.mouseDriver.clipboard = nil
	e.mouseDriver = nil

	err := e.terminal.Pty().Close()
	log.Debugf("(%p).emulator.Close: %s", e, err)
	e.closed = true
	return err
}
