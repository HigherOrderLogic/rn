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

package workspace

import (
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
)

// NewUnixFileView returns newly initialized UnixFileView.
func NewUnixFileView(v cell.View) UnixFileView {
	return UnixFileView{view: v}
}

// UnixFileView is a cell.View that hides the last EOL if present,
// to account for unix last EOL termination.
type UnixFileView struct {
	view cell.View
}

// EndsWithEOL returns true if the underlying cell.View ends
// with a new line.
func (b UnixFileView) EndsWithEOL() bool {
	cells := b.view.RawCells()
	return len(cells) > 1 && len(cells[len(cells)-1]) == 0
}

// Rows satisfies cell.View.
func (b UnixFileView) Rows() (rows int) {
	rows = b.view.Rows()
	if !b.EndsWithEOL() {
		return
	}
	rows--
	return

}

// Columns satisfies cell.View.
func (b UnixFileView) Columns(row int) int {
	return b.view.Columns(row)
}

// Cell satisfies cell.View.
func (b UnixFileView) Cell(pos term.Coordinates) (term.Cell, bool) {
	return b.view.Cell(pos)
}

// RawCells satisfies cell.View.
func (b UnixFileView) RawCells() (cells [][]term.Cell) {
	cells = b.view.RawCells()
	if !b.EndsWithEOL() {
		return
	}
	cells = cells[:len(cells)-1]
	return
}

// String satisfies cell.View.
func (b UnixFileView) String() string {
	if !b.EndsWithEOL() {
		return b.view.String()
	}
	return term.CellsToString(b.RawCells())
}
