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
	"context"

	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Repeater is a helper structure to enable repeating the last
// text insertion or delete in a buffer. See Repeat for more details.
type Repeater struct {
	buf    *cell.Buffer
	cursor *Cursor

	i         bool
	insertStr string

	d          bool
	repeating  bool
	deleteFrom term.Coordinates
	deleteTo   term.Coordinates
}

// NewRepeater allocates storage for a Repeater and initializes it.
func NewRepeater(cursor *Cursor, buf *cell.Buffer) *Repeater {
	r := new(Repeater)
	r.Init(cursor, buf)
	return r
}

// Init initializes this Repeater with buf by subscribing it to it.
func (r *Repeater) Init(cursor *Cursor, buf *cell.Buffer) {
	r.buf = buf
	r.cursor = cursor
	buf.SubscribeUsage(r)
}

// Repeat repeats the last Delete or last text insertion.
//
// Last text insertion is the accumulated string written through buffer's Writer's
// Insert since construction of Repeater or since last call to Repeater's Clear.
func (r *Repeater) Repeat() (ok bool) {
	if r.i {
		// avoid OnWillEdit loop
		r.repeating = true
		r.cursor.InsertString(r.insertStr)
		r.repeating = false
		ok = true
		return
	}
	if r.d {
		cursor := r.cursor.CursorAtScroll()
		from, to := term.CoordinatesSort(r.deleteFrom, r.deleteTo)
		diff := term.CoordinatesDiff(to, from)

		from = cursor
		to = term.CoordinatesSum(from, diff)

		// avoid OnWillEdit loop
		r.repeating = true
		_, str := r.buf.Delete(from, to)
		r.repeating = false

		ok = str != ""
		return
	}
	return
}

// Clear resets the insert string to be repeated.
func (r *Repeater) Clear() {
	r.insertStr = ""
}

// OnWillEdit satisfies cell.Subscriber.
func (r *Repeater) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
	if r.repeating {
		return
	}
	r.d = r.insertStr == "" && start != end
	r.i = r.insertStr != "" || str != ""
	r.insertStr += str
	r.deleteFrom = start
	r.deleteTo = end
}

// OnDidEdit satisfies cell.Subscriber.
func (r *Repeater) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
	// text has been deleted most likely by backlash
	if r.insertStr != "" && old != "" {
		idx := len(r.insertStr) - len(old)
		if idx >= 0 && idx < len(r.insertStr) {
			r.insertStr = r.insertStr[:idx]
		}
	}
}
