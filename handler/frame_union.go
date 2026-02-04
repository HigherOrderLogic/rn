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
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
)

// FrameUnion wraps a component.FrameUnion to satisfy tui.Handler by
// dispatching mouse events to FrameUnion nodes and returning the
// correct main Cursor offset.
//
// See component.FrameUnion for more details.
type FrameUnion struct {
	main tui.Handler
	component.FrameUnion
}

// NewFrameUnion allocates storage for a new FrameUnion and initializes it.
func NewFrameUnion(main tui.Handler) *FrameUnion {
	ret := new(FrameUnion)
	ret.Init(main)
	return ret
}

// Init initializes this frame union with main.
func (u *FrameUnion) Init(main tui.Handler) {
	u.main = main
	u.FrameUnion.Init(main)
}

// UnionTop stacks top on top of the main handler. This
// method panics if top is nil.
func (u *FrameUnion) UnionTop(top tui.Component, height int) {
	u.FrameUnion.UnionTop(top, height)
}

// UnionTopFrame stacks top on top of the main handler, and if
// u.Frame is set to true and the given frame argument too, it will
// union the frames of the adjacent handlers with the configured
// union charset. This method panics if top is nil.
func (u *FrameUnion) UnionTopFrame(top tui.Component, height int, frame bool) {
	u.FrameUnion.UnionTopFrame(top, height, frame)
}

// UnionBottom stacks bottom under of the main handler. This
// method panics if bottom is nil.
func (u *FrameUnion) UnionBottom(bottom tui.Component, height int) {
	u.FrameUnion.UnionBottom(bottom, height)
}

// UnionBottomFrame stacks bottom under of the main handler, and if
// u.Frame is set to true and the given frame argument too, it will
// union the frames of the adjacent handlers with the configured
// union charset. This method panics if bottom is nil.
func (u *FrameUnion) UnionBottomFrame(bottom tui.Component, height int, frame bool) {
	u.FrameUnion.UnionBottomFrame(bottom, height, frame)
}

// UnionLeft stacks left to the left of the main handler. This
// method panics if left is nil.
func (u *FrameUnion) UnionLeft(left tui.Component, width int) {
	u.FrameUnion.UnionLeft(left, width)
}

// UnionLeftFrame stacks left to the left of the main handler, and if
// u.Frame is set to true and the given frame argument too, it will
// union the frames of the adjacent handlers with the configured
// union charset. This method panics if left is nil.
func (u *FrameUnion) UnionLeftFrame(left tui.Component, width int, frame bool) {
	u.FrameUnion.UnionLeftFrame(left, width, frame)
}

// UnionRight stacks right to the right of the main handler. This
// method panics if right is nil.
func (u *FrameUnion) UnionRight(right tui.Component, width int) {
	u.FrameUnion.UnionRight(right, width)
}

// UnionRightFrame stacks right to the right of the main handler, and if
// u.Frame is set to true and the given frame argument too, it will
// union the frames of the adjacent handlers with the configured
// union charset. This method panics if right is nil.
func (u *FrameUnion) UnionRightFrame(right tui.Component, width int, frame bool) {
	u.FrameUnion.UnionRightFrame(right, width, frame)
}

// Resize satisfies tui.Handler.
func (u *FrameUnion) Resize(width, height int) {
	u.FrameUnion.Resize(width, height)
}

// Draw satisfies tui.Handler.
func (u *FrameUnion) Draw(w term.Writer) {
	u.FrameUnion.Draw(w)
}

// Handle delegates ev to main handler, unless event is a mouse event,
// in which case it's delegated to the component at ev.MouseX and ev.MouseY.
func (u *FrameUnion) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventMouse {
		return u.main.Handle(ev)
	}

	c, ok := u.FrameUnion.ComponentAt(term.Coordinates{X: ev.MouseX, Y: ev.MouseY})
	if !ok {
		return
	}
	handler := c.C.(tui.Handler)
	ev.MouseX -= c.Position().X
	ev.MouseY -= c.Position().Y
	if ev.MouseX < 0 {
		ev.MouseX = 0
	}
	if ev.MouseY < 0 {
		ev.MouseY = 0
	}
	return handler.Handle(ev)
}

// Cursor returns the main component's cursor position.
func (u *FrameUnion) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	o := u.FrameUnion.MainPosition()
	pos, style, show = u.main.Cursor()
	pos.X += o.X
	pos.Y += o.Y
	return
}

// Selection returns the main component's selection.
func (u *FrameUnion) Selection() (string, bool) {
	return u.main.Selection()
}
