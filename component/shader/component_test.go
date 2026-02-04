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
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"go.uber.org/goleak"
)

func TestShader(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("draws underlying component after shader is done", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		interrupter := term.FuncInterrupter(func(_ context.Context) error {
			defer wg.Done()
			return nil
		})
		s := New(&component.TestComponent{Ch: 'A'}, &testShader{}, interrupter, 1, 1*time.Second)
		s.Resize(20, 10)

		w := term.NewStringWriter(21, 11)
		tests := []comptest.TestCase{
			{Expected: `
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
                     `},
			{
				Action: func() {
					wg.Wait()
					assert.Equal(t, true, s.done.Load())
				},
				Expected: `
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
AAAAAAAAAAAAAAAAAAAA 
                     `},
		}

		comptest.TestComponent(t, s, w, tests)
		require.NoError(t, s.Close())
	})

	t.Run("resizes underlying component", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		interrupter := term.FuncInterrupter(func(_ context.Context) error {
			defer wg.Done()
			return nil
		})
		s := New(&component.TestComponent{Ch: 'A'}, &testShader{}, interrupter, 1, 1*time.Second)
		s.Resize(20, 10)

		w := term.NewStringWriter(21, 11)
		tests := []comptest.TestCase{
			{Expected: `
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
@@@@@@@@@@@@@@@@@@@@ 
                     `},
		}
		comptest.TestComponent(t, s, w, tests)

		s.Resize(21, 11)
		tests = []comptest.TestCase{
			{
				Action: func() {
					wg.Wait()
					assert.Equal(t, true, s.done.Load())
				},
				Expected: `
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAA`},
		}

		comptest.TestComponent(t, s, w, tests)
		require.NoError(t, s.Close())
	})

	t.Run("passes frame and total to shader", func(t *testing.T) {
		w := term.NewStringWriter(21, 11)
		mock := &testShader{}
		fps := 30

		var wg sync.WaitGroup
		var s *Component
		var i int
		var expectedFrames []int
		var expectedTotals []int
		const total = 30
		ready := make(chan struct{})
		interrupter := term.FuncInterrupter(func(_ context.Context) error {
			<-ready
			defer wg.Done()
			i++
			mock.called = false
			s.Draw(w)
			if i != total {
				expectedFrames = append(expectedFrames, i)
				expectedTotals = append(expectedTotals, total)
				assert.Equal(t, expectedFrames, mock.frames)
				assert.Equal(t, expectedTotals, mock.totals)
			}
			return nil
		})
		wg.Add(total)
		s = New(&component.TestComponent{Ch: 'A'}, mock, interrupter, fps, 1*time.Second)
		close(ready)
		wg.Wait()
		assert.NoError(t, s.Close())
	})
}

type testShader struct {
	called bool
	frames []int
	totals []int
}

func (t *testShader) Shade(frame, total int, in [][]term.Cell) {
	t.called = true
	t.frames = append(t.frames, frame)
	t.totals = append(t.totals, total)

	for y, row := range in {
		for x, cell := range row {
			in[y][x].Ch = cell.Ch - 1
		}
	}
}
