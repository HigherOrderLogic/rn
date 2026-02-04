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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestFade(t *testing.T) {
	suite := []struct {
		frame        int
		total        int
		defaultAttrs term.Attributes
		in           [][]term.Cell
		wantOut      [][]term.Cell
	}{
		{},
		{frame: 0, total: 9, in: [][]term.Cell{}, wantOut: [][]term.Cell{}},
		{
			frame: 0,
			total: 9,
			in: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: 0, Bg: tcell.ColorBlack}},
			}},
			wantOut: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorBlack}},
			}},
		},
		{
			frame: 10,
			total: 9,
			in: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: 0, Bg: tcell.ColorBlack}},
			}},
			wantOut: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: 0, Bg: tcell.ColorBlack}},
			}},
		},
		{
			frame: 4,
			total: 9,
			in: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.NewRGBColor(10, 10, 10), Bg: tcell.ColorBlack}},
			}},
			wantOut: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.NewRGBColor(4, 4, 4), Bg: tcell.ColorBlack}},
			}},
		},
		{
			frame: 4,
			total: 9,
			in: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.NewRGBColor(10, 10, 10), Bg: 0}},
			}},
			wantOut: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.NewRGBColor(3, 3, 3), Bg: 0}},
			}},
		},
		{
			frame:        4,
			total:        9,
			defaultAttrs: term.Attributes{Fg: tcell.NewRGBColor(10, 0, 0), Bg: tcell.NewRGBColor(0, 0, 10)},
			in: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.ColorDefault, Bg: tcell.ColorDefault}},
			}},
			wantOut: [][]term.Cell{{
				{Attributes: term.Attributes{Fg: tcell.NewRGBColor(4, 0, 5), Bg: tcell.ColorDefault}},
			}},
		},
	}

	for _, test := range suite {
		Fade(test.defaultAttrs).Shade(test.frame, test.total, test.in)
		assert.Equal(t, test.wantOut, test.in)
	}
}
