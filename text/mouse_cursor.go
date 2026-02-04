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
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// CursorMouseDelegate satisfies MouseDelegate with a Cursor on a component.Scroll.
func CursorMouseDelegate(c *Cursor) MouseDelegate {
	return mouseDelegate{cursor: c}
}

// satisfies text.MouseDelegate
type mouseDelegate struct {
	cursor *Cursor
}

func (d mouseDelegate) OnAction(ev term.Event, pos term.Coordinates, action MouseAction) bool {
	return false
}

func (d mouseDelegate) ScrollUp(n int) (ok bool) {
	for i := 0; i < n; i++ {
		ok = d.scroll().SeekUp()
		if !ok {
			return
		}
	}
	return
}

func (d mouseDelegate) ScrollDown(n int) (ok bool) {
	for i := 0; i < n; i++ {
		ok = d.scroll().SeekDown()
		if !ok {
			return
		}
	}
	return
}

func (d mouseDelegate) SetSelectionStart(pos term.Coordinates) {
	if _, ok := d.cursor.SelectionMode(); ok {
		d.cursor.Unselect()
	}
	pos = d.cursor.ScrollCoordinates(pos)
	d.cursor.MoveToScroll(pos)
	d.cursor.Select()
}

func (d mouseDelegate) SetSelectionEnd(pos term.Coordinates) {
	if _, ok := d.cursor.SelectionMode(); !ok {
		return
	}
	pos = d.cursor.ScrollCoordinates(pos)
	d.cursor.MoveToScroll(pos)
}

func (d mouseDelegate) ClearSelection() {
	d.cursor.Unselect()
}

func (d mouseDelegate) SelectWordAt(pos term.Coordinates) {
	pos = d.cursor.ScrollCoordinates(pos)
	start, end, word := d.scroll().WordAt(pos)
	if word == "" {
		return
	}
	start, _ = d.cursor.WindowCoordinates(start)
	end, _ = d.cursor.WindowCoordinates(end)
	d.SetSelectionStart(start)
	d.SetSelectionEnd(end)
}

func (d mouseDelegate) SelectLine(y int) {
	if _, ok := d.cursor.SelectionMode(); ok {
		d.cursor.Unselect()
	}
	pos := d.cursor.ScrollCoordinates(term.Coordinates{Y: y})
	d.cursor.MoveToScroll(pos)
	d.cursor.SelectLine()
}

func (d mouseDelegate) Width() int {
	return d.scroll().Width()
}

func (d mouseDelegate) Height() int {
	return d.scroll().SizeHeight()
}

func (d mouseDelegate) scroll() *component.Scroll {
	return d.cursor.scroll
}
