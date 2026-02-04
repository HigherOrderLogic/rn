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

func TestReversePlot(t *testing.T) {
	ts := reverse(&nopShader{})
	res := genPlot(ts, 1.0, 0.0)
	expect := `
xxx·······································································
···xx·····································································
·····xxx··································································
········x·································································
·········xx·······························································
···········xx·····························································
·············xxx··························································
················xx························································
··················xx······················································
····················xx····················································
······················xx··················································
························xx················································
··························xx··············································
····························xx············································
······························xxx·········································
·································xx·······································
···································xx·····································
·····································xx···································
·······································xx·································
·········································xx·······························
···········································xx·····························
·············································xx···························
···············································xxx························
··················································xx······················
····················································x·····················
·····················································xxx··················
························································xx················
··························································xx··············
····························································xx············
······························································xxx·········
·································································xx·······
···································································x······
····································································xx····
······································································xxx·
+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++x
`
	assert.Equal(t, expect, res)
}

func TestReverse(t *testing.T) {
	sh := new(nopShader)
	ts := Reverse(sh)

	tsuite := []struct {
		name     string
		total    int
		frameIn  int
		frameOut int
	}{
		{
			name:     "first frame becomes last frame",
			total:    100,
			frameIn:  0,
			frameOut: 99,
		},
		{
			name:     "last frame becomes first",
			total:    100,
			frameIn:  99,
			frameOut: 0,
		},
		{
			name:     "midpoint is unchanged",
			total:    100,
			frameIn:  49,
			frameOut: 50,
		},
		{
			name:     "negative number does not panic",
			total:    100,
			frameIn:  -20,
			frameOut: 119, // strange results can stimulate creativity
		},
	}

	for _, tcase := range tsuite {
		defer sh.reset()
		ts.Shade(tcase.frameIn, tcase.total, [][]term.Cell{})
		require.True(t, sh.shadeCalled)
		assert.Equal(t, tcase.frameOut, sh.frame)
	}
}
