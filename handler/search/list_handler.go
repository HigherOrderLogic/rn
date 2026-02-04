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

package search

import (
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Handler wraps a List to satisfy tui.Handler.
// It handles enter key by calling fn with the element in focus,
// if there's an element in focus at all.
// It handles esc key by exiting and handles arrow keys up/down
// by scrolling up and down the list.
func Handler(l *List, ed tui.Handler, fn func(string)) tui.Handler {
	ret := simpleHandler{List: l, fn: fn, ed: ed}
	return ret
}

type simpleHandler struct {
	*List
	ed tui.Handler
	fn func(string)
}

func (s simpleHandler) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventKey {
		return
	}

	switch ev.Mod {
	case 0:
		switch ev.Key {
		case term.KeyEnter, term.KeyTab:
			s.Cancel()
			s.Wait()
			item, ok := s.Focus()
			if ok {
				handled = true
				exit = true
				s.fn(string(item.data))
			}
		case term.KeyEsc:
			handled = true
			exit = true
		case term.KeyArrowDown:
			handled = s.FocusDown()
		case term.KeyArrowUp:
			handled = s.FocusUp()
		default:
			_, handled = s.ed.Handle(ev)
		}
	case term.ModCtrl:
		switch ev.Ch {
		case 'c':
			s.Cancel()
			handled = true
		case 'j':
			handled = s.FocusDown()
		case 'k':
			handled = s.FocusUp()
		}
	}

	return
}

func (s simpleHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	c, style, ok := s.ed.Cursor()
	if s.cfg.bottomSearchBar {
		c.Y += (s.List.height - s.List.inputHeight())
	}
	return c, style, ok
}

func (s simpleHandler) Selection() (string, bool) {
	match, ok := s.Focus()
	if !ok {
		return "", false
	}
	return string(match.Data()), true
}

func (s simpleHandler) Resize(width, height int) {
	s.List.Resize(width, height)

	inputHeight := s.InputHeight()
	s.ed.Resize(width, inputHeight)
}
