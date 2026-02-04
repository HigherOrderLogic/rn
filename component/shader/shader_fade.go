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

package shader

import (
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component/shader/shaderutils"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Fade is a Shader that interpolates the foreground and background color
// slowly as epoc progresses, creating a fade in effect.
func Fade(defaultAttrs term.Attributes) Shader {
	return &fade{defaultAttr: defaultAttrs}
}

type fade struct {
	defaultAttr term.Attributes
}

func (s fade) Shade(epoch, total int, cells [][]term.Cell) {
	if epoch >= total {
		return
	}
	if epoch == 0 {
		for y, row := range cells {
			for x, cell := range row {
				if cell.Bg == tcell.ColorDefault {
					cell.Bg = s.defaultAttr.Bg
				}
				cells[y][x].Fg = cell.Bg
			}
		}
		return
	}
	opacity := float64(epoch) / float64(total)
	for y, row := range cells {
		for x, cell := range row {
			if cell.Bg == tcell.ColorDefault {
				cell.Bg = s.defaultAttr.Bg
			}
			if cell.Fg == tcell.ColorDefault {
				cell.Fg = s.defaultAttr.Fg
			}
			cells[y][x].Fg = shaderutils.InterpolateColor(
				opacity, cell.Bg, cell.Fg, s.defaultAttr.Fg,
			)
		}
	}
}
