package main

import (
	"fmt"

	termutil "github.com/ernestrc/go-tui/cmd/plugin_terminal/util"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
)

type mouseDriver struct {
	t            *termutil.Terminal
	hookRawBytes []byte
	clipboard    plugin.ClipboardRegister
}

func (e *mouseDriver) OnAction(pos term.Coordinates, action text.MouseAction) bool {
	tx, ty := pos.X, pos.Y
	mode := e.t.GetMouseMode()

	switch mode {
	case termutil.MouseModeX10:
		var button rune
		switch action {
		case text.MouseLeftClick:
			button = 0
		case text.MouseMiddleClick:
			button = 1
		case text.MouseRightClick:
			button = 2
		default:
			return true
		}
		e.hookRawBytes = []byte(fmt.Sprintf("\x1b[M%c%c%c", (rune(button + 32)), (rune(tx + 32)), (rune(ty + 32))))
		return true

	case termutil.MouseModeVT200, termutil.MouseModeButtonEvent:
		var button rune
		extMode := e.t.GetMouseExtMode()
		switch action {
		case text.MouseLeftClick:
			button = 0
		case text.MouseMiddleClick:
			button = 1
		case text.MouseRightClick:
			button = 2
		case text.MouseRelease:
			if extMode != termutil.MouseExtSGR {
				button = 3
			}
		default:
			return true
		}

		if /* moved && */ mode == termutil.MouseModeButtonEvent {
			button |= 32
		}

		if extMode == termutil.MouseExtSGR {
			final := 'M'
			if action == text.MouseRelease {
				final = 'm'
			}
			e.hookRawBytes = []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", button, tx, ty, final))
		} else {
			e.hookRawBytes = []byte(fmt.Sprintf("\x1b[M%c%c%c", button+32, tx+32, ty+32))
		}
		return true

	case termutil.MouseModeNone:
		fallthrough
	default:
		if action == text.MouseMiddleClick {
			paste, _ := e.clipboard.Paste()
			e.hookRawBytes = []byte(paste)
			return true
		}
		return false
	}
}

func coordsToTermutil(pos term.Coordinates) termutil.Position {
	return termutil.Position{Line: uint64(pos.Y), Col: uint16(pos.X)}
}

func alphaMatcher(r rune) bool {
	if r >= 65 && r <= 90 {
		return true
	}
	if r >= 97 && r <= 122 {
		return true
	}
	return false
}

func numberMatcher(r rune) bool {
	if r >= 48 && r <= 57 {
		return true
	}
	return false
}

func wordMatcher(r rune) bool {
	if alphaMatcher(r) || numberMatcher(r) || r == '_' {
		return true
	}
	return false
}

func (e *mouseDriver) ScrollUp(n int) (ok bool) {
	if e.t.GetMouseMode() == termutil.MouseModeNone {
		e.t.GetActiveBuffer().ScrollUp(uint(n))
		ok = true
	}
	return
}

func (e *mouseDriver) ScrollDown(n int) (ok bool) {
	if e.t.GetMouseMode() == termutil.MouseModeNone {
		e.t.GetActiveBuffer().ScrollDown(uint(n))
		ok = true
	}
	return
}

func (e *mouseDriver) ClearSelection() {
	e.t.GetActiveBuffer().ClearSelection()
}

func (e *mouseDriver) SetSelectionStart(pos term.Coordinates) {
	e.t.GetActiveBuffer().SetSelectionStart(coordsToTermutil(pos))
}

func (e *mouseDriver) copySelectionToClipboard() {
	data, _ := e.t.GetActiveBuffer().GetSelection()
	e.clipboard.Copy(data)
}

func (e *mouseDriver) SetSelectionEnd(pos term.Coordinates) {
	e.t.GetActiveBuffer().SetSelectionEnd(coordsToTermutil(pos))
	e.copySelectionToClipboard()
}

func (e *mouseDriver) SelectWordAt(pos term.Coordinates) {
	e.t.GetActiveBuffer().SelectWordAt(coordsToTermutil(pos), wordMatcher)
	e.copySelectionToClipboard()
}

func (e *mouseDriver) SelectLine(y int) {
	pos := term.Coordinates{Y: y}
	e.t.GetActiveBuffer().SetSelectionStart(coordsToTermutil(pos))
	e.t.GetActiveBuffer().ExtendSelectionToEntireLines()
	e.copySelectionToClipboard()
}

func (e *mouseDriver) Width() int {
	return int(e.t.GetActiveBuffer().ViewWidth())
}

func (e *mouseDriver) Height() int {
	return int(e.t.GetActiveBuffer().ViewHeight())
}
