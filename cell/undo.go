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

package cell

import (
	"context"

	"unstable.build/go-tui/term"
)

// undoer adds undo and redo methods to a otherwise, irreversible cell.writer.
// It satifies the cell.writer interface and it should be used as a replacement.
type undoer struct {
	version         int
	w               Editor
	undoTimeline    []op
	redoTimeline    []op
	mergeGroupStart int
}

type op struct {
	from term.Coordinates
	to   term.Coordinates
	do   func()
	undo func()
}

// Newundoer returns new instance of undoer to undo/redo operations of w.
func newUndoer(w Editor) *undoer {
	u := new(undoer)
	u.init(w)
	return u
}

// Init initializes this undoer to undo/redo operations of w.
func (u *undoer) init(w Editor) {
	u.w = w
	u.undoTimeline = make([]op, 0)
	u.redoTimeline = make([]op, 0)
}

func popLastOp(timeline []op) ([]op, op, bool) {
	lastCmd := len(timeline) - 1
	if lastCmd < 0 {
		return nil, op{}, false
	}
	op := timeline[lastCmd]
	return timeline[:lastCmd], op, true
}

func (u *undoer) startMergeUndo() bool {
	u.mergeGroupStart = len(u.undoTimeline)
	return true
}

func (u *undoer) endMergeUndo() bool {
	if u.mergeGroupStart >= len(u.undoTimeline) {
		return false
	}
	grouped := op{
		from: u.undoTimeline[u.mergeGroupStart].from,
		do:   func() {},
		undo: func() {},
	}
	for i := u.mergeGroupStart; i < len(u.undoTimeline); i++ {
		op := u.undoTimeline[i]
		do := grouped.do
		undo := grouped.undo
		grouped.do = func() {
			do()
			op.do()
		}
		grouped.undo = func() {
			op.undo()
			undo()
		}
		grouped.to = op.to
	}

	u.undoTimeline = u.undoTimeline[:u.mergeGroupStart]
	u.undoTimeline = append(u.undoTimeline, grouped)

	return true
}

func (u *undoer) redo() (bool, term.Coordinates) {
	redoTimeline, op, ok := popLastOp(u.redoTimeline)
	if !ok {
		return false, term.Coordinates{}
	}
	u.redoTimeline = redoTimeline
	op.do()
	u.pushUndo(op)
	return ok, op.to
}

func (u *undoer) undo() (bool, term.Coordinates) {
	if u.version == 0 {
		return false, term.Coordinates{}
	}
	undoTimeline, op, ok := popLastOp(u.undoTimeline)
	if !ok {
		return false, term.Coordinates{}
	}
	u.undoTimeline = undoTimeline
	op.undo()
	u.pushRedo(op)
	return ok, op.from
}

func (u *undoer) pushUndo(cmd op) {
	u.undoTimeline = append(u.undoTimeline, cmd)
}
func (u *undoer) pushRedo(cmd op) {
	u.redoTimeline = append(u.redoTimeline, cmd)
}

func (u *undoer) resetRedoTimeline() {
	u.redoTimeline = u.redoTimeline[:0]
}

// update captures underlying writer update so it can be undone. See cell.writer.Edit
func (u *undoer) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	op := op{
		do: func() {
			u.version++
			from, to, old = u.w.Edit(ctx, start, end, str)
		},
		undo: func() {
			u.version--
			u.w.Edit(ctx, from, to, old)
		},
	}

	op.do()
	op.from = start
	op.to = to
	u.pushUndo(op)
	u.resetRedoTimeline()
	return
}
