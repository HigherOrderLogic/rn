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
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Wrap wraps a tui.Handler with a fn that gets called
// instead of Handle called on h. The rest of tui.Handler
// methods are delegated directly to h, so if h.Handle needs to be
// called, it's the client's responsibility to do so.
//
// Additionally to the usual term.Events, term.EventResize events
// are also dispatched as events to fn, after Resize has been called on h.
func Wrap(h tui.Handler, fn func(term.Event) (bool, bool)) tui.Handler {
	return wrapHandler{h: h, fn: fn}
}

type wrapHandler struct {
	h  tui.Handler
	fn func(term.Event) (bool, bool)
}

func (n wrapHandler) Resize(width, height int) {
	n.h.Resize(width, height)
	n.fn(term.Event{Type: term.EventResize, Width: width, Height: height})
}

func (n wrapHandler) Draw(w term.Writer) {
	n.h.Draw(w)
}

func (n wrapHandler) Handle(ev term.Event) (exit, handled bool) {
	return n.fn(ev)
}

func (n wrapHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return n.h.Cursor()
}

func (n wrapHandler) Selection() (string, bool) {
	return n.h.Selection()
}
