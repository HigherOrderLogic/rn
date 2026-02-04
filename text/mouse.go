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

package text

import (
	"time"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// TODO should be defaults. add config or options
// time allowed between mouse clicks to chain them into e.g. double-click
const clickChainWindow = 500 * time.Millisecond

// MouseAction represents a mouse action.
type MouseAction uint16

// List of mouse actions.
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
	OnAction(ev term.Event, pos term.Coordinates, action MouseAction) bool
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
	defer func() {
		h.mousePressedLeft = ev.Key == term.MouseLeft
		h.mousePressedMiddle = ev.Key == term.MouseMiddle
		h.mousePressedRight = ev.Key == term.MouseRight
	}()

	if ev.Type != term.EventMouse || ev.Mod != 0 {
		return
	}

	pos := term.Coordinates{X: ev.MouseX, Y: ev.MouseY}
	pressedLeft := ev.Key == term.MouseLeft && !h.mousePressedLeft
	pressedMiddle := ev.Key == term.MouseMiddle && !h.mousePressedMiddle
	pressedRight := ev.Key == term.MouseRight && !h.mousePressedRight
	wheelUp := ev.Key == term.MouseWheelUp
	wheelDown := ev.Key == term.MouseWheelDown
	released := ev.Key == term.MouseRelease

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
		handled = h.delegate.OnAction(ev, pos, action)
		if handled {
			return
		}
	}

	switch ev.Key {
	case term.MouseWheelUp:
		handled = h.delegate.ScrollUp(1)
	case term.MouseWheelDown:
		handled = h.delegate.ScrollDown(1)
	case term.MouseLeft:
		handled = h.handleLeftClickSelect(pos)
	}
	return
}

func (h *Mouse) handleLeftClickSelect(pos term.Coordinates) (handled bool) {
	if h.mousePressedLeft { /* drag */
		handled = true
		h.delegate.SetSelectionEnd(pos)
		if pos.Y < 4 {
			h.delegate.ScrollUp(1)
		} else if pos.Y > h.delegate.Height()-4 {
			h.delegate.ScrollDown(1)
		}
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
