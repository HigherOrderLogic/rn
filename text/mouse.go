package text

import (
	"time"

	"github.com/ernestrc/go-tui/term"
)

// TODO should be defaults. add config or options
// time allowed between mouse clicks to chain them into e.g. double-click
const clickChainWindow = 500 * time.Millisecond

type MouseAction uint16

const (
	MouseLeftClick MouseAction = iota
	MouseRightClick
	MouseMiddleClick
	MouseWheelUp
	MouseWheelDown
	MouseRelease
	mouseNone
)

// MouseDelegate is an interface that wraps callbacks to enable
// scrolling and text selection.
//
// OnAction is provided to extend to other features. It takes precedence
// over builtin features so if it returns true, Mouse won't call
// any other callbacks.
type MouseDelegate interface {
	OnAction(pos term.Coordinates, action MouseAction) bool
	ScrollUp(n int) bool
	ScrollDown(n int) bool
	SetSelectionEnd(pos term.Coordinates)
	SetSelectionStart(pos term.Coordinates)
	ClearSelection()
	SelectWordAt(pos term.Coordinates)
	SelectLine(y int)
	Width() int
	Height() int
}

// Mouse handles mouse events to provide text selection and scrolling
// functionality to a text tui.Handler. It can be extended by
// providing an OnAction callback. See MouseDelegate for more details.
type Mouse struct {
	delegate           MouseDelegate
	mousePos           term.Coordinates
	mousePressedLeft   bool
	mousePressedMiddle bool
	mousePressedRight  bool
	lastClick          time.Time
	clickCount         int
}

// NewMouse allocates storage for a Mouse and initializes it.
func NewMouse(d MouseDelegate) *Mouse {
	ret := new(Mouse)
	ret.Init(d)
	return ret
}

// Init initializes this Mouse with d and resets all its internal state.
func (m *Mouse) Init(d MouseDelegate) {
	m.delegate = d
	m.mousePos = term.Coordinates{}
	m.mousePressedLeft = false
	m.mousePressedRight = false
	m.mousePressedMiddle = false
	m.lastClick = time.Time{}
	m.clickCount = 0
}

// Handle satisfies tui.Handler.
func (h *Mouse) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventMouse {
		return
	}

	pos := term.Coordinates{X: ev.MouseX, Y: ev.MouseY}
	pressedLeft := ev.Key == term.MouseLeft && !h.mousePressedLeft
	pressedMiddle := ev.Key == term.MouseMiddle && !h.mousePressedMiddle
	pressedRight := ev.Key == term.MouseRight && !h.mousePressedRight
	wheelUp := ev.Key == term.MouseWheelUp
	wheelDown := ev.Key == term.MouseWheelDown
	released := ev.Key == term.MouseRelease

	defer func() {
		h.mousePressedLeft = ev.Key == term.MouseLeft
		h.mousePressedMiddle = ev.Key == term.MouseMiddle
		h.mousePressedRight = ev.Key == term.MouseRight
	}()

	var action MouseAction
	switch true {
	case pressedLeft:
		action = MouseLeftClick
	case pressedMiddle:
		action = MouseMiddleClick
	case pressedRight:
		action = MouseRightClick
	case released:
		action = MouseRelease
	case wheelUp:
		action = MouseWheelUp
	case wheelDown:
		action = MouseWheelDown
	default:
		action = mouseNone
	}

	// we do not want to dispatch on drags with mouseNone
	// because it could confuse clients to think that we
	// are able to dispatch simply on mouse moves.
	if action != mouseNone {
		handled = h.delegate.OnAction(pos, action)
		if handled {
			return
		}
	}

	switch ev.Key {
	case term.MouseWheelUp:
		handled = h.delegate.ScrollUp(5)
	case term.MouseWheelDown:
		handled = h.delegate.ScrollDown(5)
	case term.MouseLeft:
		handled = h.handleLeftClickSelect(pos)
	}
	return
}

func (h *Mouse) handleLeftClickSelect(pos term.Coordinates) (handled bool) {
	if h.mousePressedLeft { /* drag */
		handled = true
		h.delegate.SetSelectionEnd(pos)
		return
	}

	if h.clickCount == 0 || time.Since(h.lastClick) < clickChainWindow {
		h.clickCount++
	} else {
		h.clickCount = 1
	}

	h.lastClick = time.Now()
	handled = true
	switch h.clickCount {
	case 1:
		h.delegate.ClearSelection()
		h.delegate.SetSelectionStart(pos)
	case 2:
		h.delegate.SelectWordAt(pos)
	default:
		h.delegate.SelectLine(pos.Y)
	}
	return
}
