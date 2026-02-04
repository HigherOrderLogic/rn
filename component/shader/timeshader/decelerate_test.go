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

func TestDeceleratePlot(t *testing.T) {
	ts := decelerate(&nopShader{})
	res := genPlot(ts, 1.0, 0.0)
	expect := `
····························································xxxxxxxxxxxxxx
·······················································xxxxx··············
···················································xxxx···················
·················································xx·······················
··············································xxx·························
···········································xxx····························
········································xxx·······························
······································xx··································
····································xx····································
·································xxx······································
··········································································
······························xxx·········································
·····························x············································
···························xx·············································
·························xx···············································
························x·················································
······················xx··················································
·····················x····················································
···················xx·····················································
··················x·······················································
················xx························································
···············x··························································
·············xx···························································
··········································································
···········xx·····························································
··········x·······························································
·········x································································
········x·································································
·······x··································································
·····xx···································································
····x·····································································
···x······································································
··x·······································································
·x········································································
x+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
`
	assert.Equal(t, expect, res)
}

func TestDecelerate(t *testing.T) {
	sh := new(nopShader)
	ts := Decelerate(sh)

	tsuite := []struct {
		name     string
		total    int
		frameIn  int
		frameOut int
	}{
		{
			name:     "first frame is unchanged",
			total:    100,
			frameIn:  0,
			frameOut: 0,
		},
		{
			name:     "last frame is unchanged",
			total:    100,
			frameIn:  99,
			frameOut: 99,
		},
		{
			name:     "midpoint is at 30 per cent",
			total:    100,
			frameIn:  30,
			frameOut: 51,
		},
		{
			name:     "negative frames might produce negative output",
			total:    100,
			frameIn:  -30,
			frameOut: -69,
		},
	}

	for _, tcase := range tsuite {
		defer sh.reset()
		ts.Shade(tcase.frameIn, tcase.total, [][]term.Cell{})
		require.True(t, sh.shadeCalled)
		assert.Equal(t, tcase.frameOut, sh.frame)
	}
}
