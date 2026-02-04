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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestMakeCellMatrix(t *testing.T) {
	tsuite := []struct {
		name   string
		rows   int
		cols   int
		panics bool
	}{
		{
			name: "creates a list of 'rows' lists with 'cols' cells each of them",
			rows: 4,
			cols: 7,
		},
		{
			name: "zero cells",
			rows: 0,
			cols: 0,
		},
		{
			name:   "negative dimensions",
			rows:   -4,
			cols:   -7,
			panics: true,
		},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.name, func(t *testing.T) {
			if tcase.panics {
				assert.Panics(t, func() {
					MakeCellMatrix(tcase.cols, tcase.rows)
				})
				return
			}

			cells := MakeCellMatrix(tcase.cols, tcase.rows)
			assert.Len(t, cells, tcase.rows)

			for y := 0; y < tcase.rows; y++ {
				assert.Len(t, cells[y], tcase.cols)
				for x := 0; x < tcase.cols; x++ {
					assert.Equal(t, term.Cell{}, cells[y][x])
				}
			}
		})
	}
}
