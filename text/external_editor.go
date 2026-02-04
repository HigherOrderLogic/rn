// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

// ExternalEditor wraps a CellEditor to update cursor **line** upon external edits.
func ExternalEditor(c *Cursor, ed cell.Editor) cell.Editor {
	return extEditor{c, ed}
}

type extEditor struct {
	c  *Cursor
	ed cell.Editor
}

func (e extEditor) Edit(ctx context.Context, start, end term.Coordinates, new string) (
	from, to term.Coordinates, old string,
) {
	from, to, old = e.ed.Edit(ctx, start, end, new)

	cursor := e.c.CursorAtScroll()
	orig := cursor

	// delete
	if start.Y <= cursor.Y {
		cursor.Y -= end.Y - start.Y
	} else if start.Y == cursor.Y && start.X <= cursor.X {
		cursor.Y -= end.Y - start.Y
		cursor.X -= end.X - max(start.X, cursor.X)
	}

	// insert
	if from.Y < cursor.Y {
		cursor.Y += to.Y - from.Y
	} else if from.Y == cursor.Y && from.X <= cursor.X {
		cursor.Y += to.Y - from.Y
		cursor.X += to.X - max(from.X, cursor.X)
	}

	if cursor != orig {
		cursor.X = max(0, cursor.X)
		cursor.Y = max(0, cursor.Y)
		e.c.SetCursorAtScroll(cursor)
	}
	return
}
