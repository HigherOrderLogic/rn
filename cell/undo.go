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

	"github.com/unstablebuild/rune-go-sdk/term"
)

// undoer adds undo and redo methods to a otherwise, irreversible cell.writer.
// It satifies the cell.writer interface and it should be used as a replacement.
type undoer struct {
	version         int
	w               Editor
	undoTimeline    []edits
	redoTimeline    []edits
	mergeGroupStart int
}

type edits struct {
	Version int
	Edits   []edit
}

// newUndoer returns new instance of undoer to undo/redo operations of w.
func newUndoer(w Editor) *undoer {
	u := new(undoer)
	u.init(w)
	return u
}

// Init initializes this undoer to undo/redo operations of w.
func (u *undoer) init(w Editor) {
	u.w = w
	u.undoTimeline = make([]edits, 0)
	u.redoTimeline = make([]edits, 0)
}

func popLastOp(timeline []edits) ([]edits, edits, bool) {
	lastCmd := len(timeline) - 1
	if lastCmd < 0 {
		return nil, edits{}, false
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
	tail := u.undoTimeline[len(u.undoTimeline)-1]
	grouped := edits{Version: tail.Version}
	for i := len(u.undoTimeline) - 1; i >= u.mergeGroupStart; i-- {
		op := u.undoTimeline[i]
		grouped.Edits = append(grouped.Edits, op.Edits...)
	}

	u.undoTimeline = u.undoTimeline[:u.mergeGroupStart]
	u.undoTimeline = append(u.undoTimeline, grouped)

	return true
}

func (u *undoer) redo() (bool, term.Coordinates) {
	redoTimeline, ed, ok := popLastOp(u.redoTimeline)
	if !ok {
		return false, term.Coordinates{}
	}
	if ed.Version != u.version || len(ed.Edits) == 0 {
		return false, term.Coordinates{}
	}
	u.redoTimeline = redoTimeline
	u.version += len(ed.Edits)

	var op edits
	op.Version = u.version
	op.Edits = make([]edit, len(ed.Edits))
	for i, ed := range ed.Edits {
		from, to, old := u.w.Edit(context.Background(), ed.From, ed.To, ed.String)
		ed.From = from
		ed.To = to
		ed.String = old
		op.Edits[len(op.Edits)-i-1] = ed
	}

	u.pushUndo(op)
	return ok, op.Edits[0].To
}

func (u *undoer) undo() (bool, term.Coordinates) {
	if u.version == 0 {
		return false, term.Coordinates{}
	}
	undoTimeline, ed, ok := popLastOp(u.undoTimeline)
	if !ok {
		return false, term.Coordinates{}
	}
	if ed.Version != u.version || len(ed.Edits) == 0 {
		return false, term.Coordinates{}
	}
	u.undoTimeline = undoTimeline
	u.version -= len(ed.Edits)

	var op edits
	op.Version = u.version
	op.Edits = make([]edit, len(ed.Edits))
	for i, ed := range ed.Edits {
		from, to, old := u.w.Edit(context.Background(), ed.From, ed.To, ed.String)
		ed.From = from
		ed.To = to
		ed.String = old
		op.Edits[len(op.Edits)-i-1] = ed
	}
	u.pushRedo(op)

	return ok, op.Edits[0].From
}

func (u *undoer) pushUndo(cmd edits) {
	u.undoTimeline = append(u.undoTimeline, cmd)
}
func (u *undoer) pushRedo(cmd edits) {
	u.redoTimeline = append(u.redoTimeline, cmd)
}

func (u *undoer) resetRedoTimeline() {
	u.redoTimeline = u.redoTimeline[:0]
}

// Edit is an edit operation on a Buffer.
type edit struct {
	From   term.Coordinates
	To     term.Coordinates
	String string
}

// update captures underlying writer update so it can be undone. See cell.writer.Edit
func (u *undoer) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	u.version++
	from, to, old = u.w.Edit(ctx, start, end, str)

	var op edit
	op.From = from
	op.To = to
	op.String = old

	u.pushUndo(edits{Version: u.version, Edits: []edit{op}})
	u.resetRedoTimeline()
	return
}
