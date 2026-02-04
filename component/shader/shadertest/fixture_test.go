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

package shadertest

import (
	"math"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// testShader1234 draws characters progressively from 0-9 based on progress
// frame/total; useful to assert against in your shadertest tests.
type testShader1234 struct{}

func (s *testShader1234) Shade(frame, total int, cells [][]term.Cell) {
	h := len(cells)
	w := len(cells[0])

	progress := int(math.Round(9.0 * float64(frame) / float64(total))) // 0..9

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if cells[y] == nil || x >= len(cells[y]) {
				continue
			}
			cells[y][x].Ch = rune('0' + progress)
		}
	}
}

// testShaderABCD draws characters progressively from A-J based on progress
// frame/total; useful to assert against in your shadertest tests.
type testShaderABCD struct{}

func (s *testShaderABCD) Shade(frame, total int, cells [][]term.Cell) {
	h := len(cells)
	w := len(cells[0])

	progress := int(math.Round(9.0 * float64(frame) / float64(total))) // A..I

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if cells[y] == nil || x >= len(cells[y]) {
				continue
			}
			cells[y][x].Ch = rune('A' + progress)
		}
	}
}
