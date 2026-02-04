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

package vte

import (
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/term/vte/vtescreen"
)

// lastPromptLine this attempts to find the last "shell" prompt line,
// or falls back to returning the last block of content.
//
//	┌─────────┐
//	│$ OOOOO  │
//	│$ OOOOOOO│
//	│OOOOOOOO │
//	│$ XXXXXX │
//	└─────────┘
func lastPromptLine(view cell.View, width int, excludeTailSpaces bool) (from term.Coordinates, to term.Coordinates) {
	cells := view.RawCells()
	if len(cells) == 0 {
		return
	}
	to.Y = len(cells) - 1
	to.X = len(cells[to.Y])

	if excludeTailSpaces {
	outerSpace:
		for y := len(cells) - 1; y >= 0; y-- {
			for x := len(cells[y]) - 1; x >= 0; x-- {
				ch := cells[y][x].Ch
				if ch != vtescreen.DefaultChar && ch != ' ' {
					to = term.Coordinates{Y: y, X: x + 1}
					break outerSpace
				}
			}
		}
	} else {
	outer:
		for y := len(cells) - 1; y >= 0; y-- {
			for x := len(cells[y]) - 1; x >= 0; x-- {
				if cells[y][x].Ch != vtescreen.DefaultChar {
					to = term.Coordinates{Y: y, X: x + 1}
					break outer
				}
			}
		}
	}

	var last term.Coordinates
	for y := to.Y; y >= 0; y-- {
		x := len(cells[y]) - 1
		if y == to.Y {
			x = to.X - 2
			// be robust against resizes. If there's a gap between
			// the end of the line and width, then consider that a
			// separate prompt line
		} else if x < width-1 {
			from = last
			return
		}
		for ; x >= 0; x-- {
			cell := cells[y][x]
			if cell.Ch == vtescreen.DefaultChar {
				from = last
				return
			}
			last = term.Coordinates{Y: y, X: x}
		}
	}

	from = last
	return
}
