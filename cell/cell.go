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
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// View is the interface that wraps methods to query a 2D matrix of term.Cell.
type View interface {
	Rows() int
	Columns(row int) int
	Cell(term.Coordinates) (term.Cell, bool)
	RawCells() [][]term.Cell
	fmt.Stringer
}

// Editor is the interface that wraps methods to mutate a 2D matrix of term.Cell.
type Editor interface {
	// Edit replaces any content from [start:end) with str and returns the
	// right-exclusive coordinates of the effective insert range. Note that
	// returned from, to values will be equal to each other if this operation
	// only removes content. This effectively allows clients to reverse a call
	// to Edit by calling it again with the last return values.
	//
	// This method should panic if delete range between start, end is out of bounds.
	Edit(ctx context.Context, start, end term.Coordinates, new string) (
		from, to term.Coordinates, old string,
	)
}

// NewView returns a new Reader which reads from cells and uses tabspaces.
func NewView(cells [][]term.Cell) View {
	r := &rawCells{
		cells:      cells,
		fillInChar: ' ',
		columnCap:  defColumnCap,
		rowCap:     defRowCap,
	}
	return r
}
