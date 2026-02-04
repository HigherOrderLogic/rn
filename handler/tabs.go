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

package handler

import (
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var _ tui.Handler = (*Tabs)(nil)

// Tabs add mouse handling to component.Tabs.
type Tabs struct {
	component.Tabs
	mousePressedLeft bool
	OnClick          func(int) bool
}

// NewTabs returns a Tabs component which handles mouse events.
func NewTabs() *Tabs {
	t := new(Tabs)
	t.Init()
	return t
}

// Init initializes this Tabs with the given underlying handler
// and frame attributes.
func (t *Tabs) Init() {
	t.Tabs.Init()
}

// Handle delegates the event to the underlying handler.
func (t *Tabs) Handle(ev term.Event) (quit, handled bool) {
	defer func() {
		// if button is released then MouseRelease is dispatched
		// so this is reset
		t.mousePressedLeft = ev.Key == term.MouseLeft
	}()

	if ev.Type != term.EventMouse || ev.Key != term.MouseLeft || ev.Mod != 0 {
		return
	}

	// do not dispatch drags as multiple click events:
	// MouseRelease must be dispatched between MouseLeft for
	// events to be considered multiple mouse clicks.
	pressedLeft := ev.Key == term.MouseLeft && !t.mousePressedLeft
	if !pressedLeft {
		return
	}
	mousePos := term.Coordinates{X: ev.MouseX, Y: ev.MouseY}
	idx, ok := t.Tabs.TabAt(mousePos)
	if !ok {
		if t.OnClick != nil {
			handled = t.OnClick(-1)
		}
		return
	}

	handled = true

	t.SetFocus(idx)
	if t.OnClick != nil {
		_ = t.OnClick(idx)
	}
	return
}

// Cursor satisfies tui.Handler but always returns false.
func (t *Tabs) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, term.CursorStyleDefault, false
}

// Selection satisfies tui.Handler but always returns false.
func (t *Tabs) Selection() (string, bool) {
	return "", false
}
