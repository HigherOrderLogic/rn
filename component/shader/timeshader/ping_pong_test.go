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

func TestPingPongPlot(t *testing.T) {
	ts := pingPong(&nopShader{})
	res := genPlot(ts, 1.0, 0.0)
	expect := `
····································xxx···································
···································x···x··································
·································xx·····x·································
·········································x································
································x·········x·······························
······························xx···········x······························
············································x·····························
····························xx···············x····························
··············································x···························
··························xx···················x··························
·························x······················x·························
·················································x························
·······················xx·························x·······················
······················x····························xx·····················
·····················x····················································
····················x································xx···················
···················x···································x··················
··················x·······················································
·················x······································xx················
················x·········································x···············
···············x···········································x··············
·············xx·············································x·············
·····························································x············
···········xx·················································x···········
··········x····················································x··········
································································x·········
········xx·······················································x········
··································································x·······
······xx···························································x······
·····x······························································xx····
····x·····································································
···x··································································x···
··x····································································xx·
·x········································································
x++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++x
`
	assert.Equal(t, expect, res)
}

func TestPingPong(t *testing.T) {
	sh := new(nopShader)

	tsuite := []struct {
		name     string
		bounces  int
		total    int
		frameIn  int
		frameOut int
		panics   bool
	}{
		{
			name:     "first frame stays first frame",
			bounces:  1,
			total:    101,
			frameIn:  0,
			frameOut: 0,
		},
		{
			name:     "last frame becomes first frame",
			bounces:  1,
			total:    101,
			frameIn:  100,
			frameOut: 0,
		},
		{
			name:     "middle frame becomes last frame",
			bounces:  1,
			total:    101,
			frameIn:  50,
			frameOut: 100,
		},
		{
			name:     "frame in quarter duration becomes middle frame",
			bounces:  1,
			total:    101,
			frameIn:  25,
			frameOut: 50,
		},
		{
			name:     "during first half frame increases, twice as fast",
			bounces:  1,
			total:    101,
			frameIn:  10,
			frameOut: 20,
		},
		{
			name:     "frame at 3 quarters duration becomes middle frame",
			bounces:  1,
			total:    101,
			frameIn:  75,
			frameOut: 50,
		},
		{
			name:     "during second half frame decreases, twice as fast",
			bounces:  1,
			total:    101,
			frameIn:  80,
			frameOut: 40,
		},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.name, func(t *testing.T) {
			defer sh.reset()
			ts := PingPong(sh)

			shade := func() {
				ts.Shade(tcase.frameIn, tcase.total, [][]term.Cell{})
			}

			if tcase.panics {
				assert.Panics(t, shade)
				return
			} else {
				assert.NotPanics(t, shade)
			}

			require.True(t, sh.shadeCalled)
			assert.Equal(t, tcase.frameOut, sh.frame)
		})
	}
}
