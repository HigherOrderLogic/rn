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

package timeshader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestLoopPlot(t *testing.T) {
	ts := loop(&nopShader{}, 2)
	res := genPlot(ts, 1.0, 0.0)
	expect := `
····································x····································x
···································x····································x·
·································xx···································xx··
··········································································
································x····································x····
······························xx···································xx·····
··········································································
····························xx···································xx·······
··········································································
··························xx···································xx·········
·························x····································x···········
··········································································
·······················xx···································xx············
······················x····································x··············
·····················x····································x···············
····················x····································x················
···················x····································x·················
··················x····································x··················
·················x····································x···················
················x····································x····················
···············x····································x·····················
·············xx···································xx······················
··········································································
···········xx···································xx························
··········x····································x··························
··········································································
········xx···································xx···························
··········································································
······xx···································xx·····························
·····x····································x·······························
····x····································x································
···x····································x·································
··x····································x··································
·x····································x···································
x++++++++++++++++++++++++++++++++++++x++++++++++++++++++++++++++++++++++++
`
	assert.Equal(t, expect, res)

	ts = loop(&nopShader{}, 3)
	res = genPlot(ts, 1.0, 0.0)
	expect = `
························x·······················x·························
·······················x·······················x························x·
······················x················································x··
··········································································
··············································x·······················x···
·····················x·······················x····························
····················x················································x····
···················x························x·······················x·····
···········································x·······················x······
··················x·······················x·······························
·················x················································x·······
················x························x·······················x········
········································x·································
···············x·······················x························x·········
··············x················································x··········
·············x························x·······················x···········
·····································x····································
············x·······················x························x············
···········x················································x·············
··········x························x·······················x··············
··································x·······································
·········x·······················x························x···············
········x················································x················
································x·······················x·················
··········································································
·······x·······················x··········································
······x·······················x························x··················
·····x················································x···················
·····························x·······················x····················
····x·······················x·············································
···x················································x·····················
··x························x·······················x······················
··························x·······················x·······················
·x·······················x················································
x++++++++++++++++++++++++++++++++++++++++++++++++x+++++++++++++++++++++++x
`
	assert.Equal(t, expect, res)
}

func TestLoop(t *testing.T) {
	sh := new(nopShader)

	tsuite := []struct {
		name      string
		cycles    int
		total     int
		framesIn  []int
		framesOut []int
		panics    bool
	}{
		{
			name:     "0 cycles panics",
			cycles:   0,
			total:    101,
			framesIn: []int{0},
			panics:   true,
		},
		{
			name:      "1 cycle unaffects the frame",
			cycles:    1,
			total:     101,
			framesIn:  []int{0, 13, 50, 63, 100},
			framesOut: []int{0, 13, 50, 63, 100},
		},
		{
			name:      "2 cycle repeats twice in even input",
			cycles:    2,
			total:     100,
			framesIn:  []int{0, 13, 50, 63, 98, 99},
			framesOut: []int{0, 26, 00, 26, 96, 98},
		},
		{
			name:      "2 cycle last frames on even input loops",
			cycles:    2,
			total:     100,
			framesIn:  []int{99},
			framesOut: []int{98},
		},
		{
			name:      "2 cycle repeats twice in uneven input",
			cycles:    2,
			total:     101,
			framesIn:  []int{0, 13, 50, 63, 99, 100},
			framesOut: []int{0, 26, 00, 26, 98, 100},
		},
		{
			name:      "2 cycle last frames on uneven input loops",
			cycles:    2,
			total:     101,
			framesIn:  []int{100},
			framesOut: []int{100},
		},
		{
			name:      "3 cycle repeats thrice",
			cycles:    3,
			total:     101,
			framesIn:  []int{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			framesOut: []int{0, 30, 60, 90, 21, 51, 81, 12, 42, 72, 100},
		},
		{
			name:      "3 cycle loop finishes with last input frame",
			cycles:    3,
			total:     101,
			framesIn:  []int{100},
			framesOut: []int{100},
		},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.name, func(t *testing.T) {
			defer sh.reset()
			ts := Loop(sh, tcase.cycles)

			shade := func() {
				for i, fr := range tcase.framesIn {
					ts.Shade(fr, tcase.total, [][]term.Cell{})
					assert.Equal(t, tcase.framesOut[i], sh.frame)
				}
			}

			if tcase.panics {
				assert.Panics(t, shade)
				return
			} else {
				assert.NotPanics(t, shade)
			}

			require.True(t, sh.shadeCalled)
		})
	}
}
