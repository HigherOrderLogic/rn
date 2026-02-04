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

package glslshader

import (
	"unstable.build/go-tui/component/shader"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// ProgressVizABCD shows the process percentage as the first 10 letters of the
// alphabet, A being the start and I the end of the animation duration. The
// background is painted green where x is highest and blue where y is highest.
func ProgressVizABCD() shader.Shader {
	return &progressVizABCD{}
}

type progressVizABCD struct{}

func (s *progressVizABCD) Shade(frame, total int, cells [][]term.Cell) {
	h := len(cells)
	w := len(cells[0])

	progress := float(frame) / float(total) // 0.0..1.0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if cells[y] == nil || x >= len(cells[y]) {
				continue
			}

			uv := vec2(255.0*float(x)/float(w), 255.0*float(y)/float(h))
			cells[y][x].Bg = vecToCol(vec3(0.0, uv.x, uv.y))
			cells[y][x].Fg = vecToCol(vec3(0.0, 1.0-uv.x, 1.0-uv.y))
			cells[y][x].Ch = rune('A' + int(10.0*progress))
		}
	}
}
