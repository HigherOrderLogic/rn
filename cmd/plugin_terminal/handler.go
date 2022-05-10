package main

import (
	"fmt"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	termutil "github.com/ernestrc/go-tui/cmd/plugin_terminal/util"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

type Handler struct {
	wm browser.WindowManager
	wp workspace.Workspace
	p  browser.EventPublisher
	m  browser.Messenger

	windowManipulator *windowManipulator
	terminal          *termutil.Terminal
	theme             *colorTheme
	defattr           term.Attributes

	closed        bool
	width, height int
	resizeErr     error
	updateCh      chan struct{}
	sema          chan struct{}
}

func NewHandler(
	wm browser.WindowManager, wp workspace.Workspace,
	p browser.EventPublisher, m browser.Messenger, defattr term.Attributes,
	shell string, initialCmd string, title string,
) (*Handler, error) {
	ret := new(Handler)
	err := ret.Init(wm, wp, p, m, defattr, shell, initialCmd, title)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (e *Handler) Init(
	wm browser.WindowManager, wp workspace.Workspace,
	p browser.EventPublisher, m browser.Messenger, defattr term.Attributes,
	shell string, initialCmd string, title string,
) error {
	e.wm = wm
	e.wp = wp
	e.p = p
	e.defattr = defattr
	e.m = m

	e.theme = newColorTheme()
	e.windowManipulator = newWindowManipulator(e.wm, e.m)
	opts := []termutil.Option{
		termutil.WithTheme(e.theme.theme),
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
		err := cmd.Wait()
		if err != nil {
			log.Errorf("terminal.Cmd.Wait: %s", err)
		}
		e.Close()
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
			} else {
				log.Tracef("PublishInterrupt: OK")
			}
		}
	}()

	return nil
}

func (e *Handler) Resize(width, height int) {
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

func (e *Handler) drawStr(str string, w term.Writer) {
	c := component.String(str)
	c.Resize(e.width, e.height)
	c.Draw(w)
}

func (e *Handler) drawRow(
	w term.Writer, termbuf *termutil.Buffer,
	viewY int, defattr term.Attributes,
) {
	for viewX := uint16(0); int(viewX) < e.width && viewX < termbuf.ViewWidth(); viewX++ {
		tcell := termbuf.GetCell(viewX, uint16(viewY))

		// we don't need to draw empty cells
		if tcell == nil || tcell.Rune().Rune == 0 {
			continue
		}

		var ok bool
		attr := defattr
		if fgcolor := tcell.Fg(); fgcolor != nil {
			attr.Fg, ok = e.theme.to8Bit(fgcolor)
			if !ok {
				log.Errorf("could not map RGBA FG color to term color: %#v", fgcolor)
				attr.Fg = defattr.Fg
			}
		}
		if bgcolor := tcell.Bg(); bgcolor != nil {
			attr.Bg, ok = e.theme.to8Bit(bgcolor)
			if !ok {
				log.Errorf("could not map RGBA BG color to term color: %#v", bgcolor)
				attr.Bg = defattr.Bg
			}
		}

		// pick a font face for the cell
		if tcell.Bold() {
			attr.Fg |= term.AttrBold
		}

		// underline the cell content if required
		if tcell.Underline() {
			attr.Fg |= term.AttrUnderline
		}

		// draw the text for the cell
		cell := term.Cell{Ch: tcell.Rune().Rune, Fg: attr.Fg, Bg: attr.Bg}
		pos := term.Coordinates{X: int(viewX), Y: viewY}
		w.SetCell(pos, cell)
	}
}

func (e *Handler) Draw(w term.Writer) {
	log.Tracef("(%p).Handler.Draw", e)
	if e.resizeErr != nil {
		errStr := fmt.Sprintf("Error setting win size: %s", e.resizeErr)
		log.Errorf("(%p).Handler.Draw: resize err: %s", e, e.resizeErr)
		e.drawStr(errStr, w)
		return
	}

	e.terminal.Lock()
	defer e.terminal.Unlock()

	termbuf := e.terminal.GetActiveBuffer()
	for viewY := int(termbuf.ViewHeight() - 1); viewY < e.height && viewY >= 0; viewY-- {
		e.drawRow(w, termbuf, viewY, e.defattr)
	}
}

// FIXME figure out why sometimes the first colors are not parsed correctly when
// passing certain options.
// TODO implement adding SysProcAttr to workspace Start command
// then hook into creaty.pty to use workspace command
// TODO implement SendEventNone
// TODO fix mouse events, selection etc.
func (e *Handler) Handle(ev term.Event) (exit, handled bool) {
	log.Tracef("(%p).Handler.Handle: (raw=%q)", e, ev.Raw)
	if len(ev.Raw) == 0 {
		return
	}

	e.terminal.Lock()
	exit = e.closed
	e.terminal.Unlock()
	if exit {
		log.Tracef("(%p).Handler.Handle: already closed", e)
		return
	}

	e.sema <- struct{}{}
	defer func() { <-e.sema }()

	err := e.terminal.WriteToPty(ev.Raw)
	if err != nil {
		log.Errorf("(%p).Handler.Handle: %s", e, err)
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

func (e *Handler) Cursor() (term.Coordinates, bool) {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	termbuf := e.terminal.GetActiveBuffer()
	if !termbuf.IsCursorVisible() {
		return term.Coordinates{}, false
	}
	return term.Coordinates{
		X: int(termbuf.CursorColumn()),
		Y: int(termbuf.CursorLine()),
	}, true
}

func (e *Handler) Man() tui.Manual {
	panic("TODO")
}

func (e *Handler) URI() (workspace.URI, error) {
	var name string
	func() {
		e.terminal.Lock()
		defer e.terminal.Unlock()
		name = e.terminal.GetTitle()
	}()

	return e.wp.URI(name)
}

func (e *Handler) Title() string {
	e.terminal.Lock()
	defer e.terminal.Unlock()

	return e.terminal.GetTitle()
}

func (e *Handler) Close() error {
	log.Tracef("(%p).Handler.Close", e)

	e.terminal.Lock()
	defer e.terminal.Unlock()

	if e.closed {
		return nil
	}

	err := e.terminal.Pty().Close()
	log.Debugf("(%p).Handler.Close: %s", e, err)
	e.closed = true
	return err
}
