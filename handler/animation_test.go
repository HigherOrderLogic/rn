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

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func newTestInterrupter() (term.Interrupter, chan struct{}) {
	ch := make(chan struct{})
	interrupter := term.FuncInterrupter(func(context.Context) error {
		ch <- struct{}{}
		return nil
	})
	return interrupter, ch
}

func TestAnimationPlayer(t *testing.T) {
	fps := 30
	interrupter, ch := newTestInterrupter()

	frames := []string{
		"0000    \n0000    \n    0000\n    0000",
		"1111    \n1111    \n    1111\n    1111",
	}
	sequences := []int{0, 0, 1}

	c := component.NewAnimation(interrupter, frames, sequences, fps)

	// sut
	h := AnimationPlayer(c)
	h.Resize(8, 4)

	for i := 0; i < 3; i++ {
		w := term.NewStringWriter(8, 4)
		<-ch
		h.Draw(w)
		var expected string
		switch i {
		case 0:
			expected = "0000    \n0000    \n    0000\n    0000"
		case 1:
			expected = "0000    \n0000    \n    0000\n    0000"
		case 2:
			expected = "1111    \n1111    \n    1111\n    1111"
		default:
			panic("hmmm")
		}

		require.NoError(t, w.Flush())
		assert.Equal(t, expected, w.String(), i)
	}

	// pause
	exit, handled := h.Handle(term.Event{Key: term.KeySpace, Type: term.EventKey})
	assert.True(t, handled)
	assert.False(t, exit)

	for i := 0; i < 3; i++ {
		w := term.NewStringWriter(8, 4)
		<-ch
		h.Draw(w)
		expected := "0000    \n0000    \n    0000\n    0000"
		require.NoError(t, w.Flush())
		assert.Equal(t, expected, w.String(), i)
	}

	// unpause
	exit, handled = h.Handle(term.Event{Key: term.KeySpace, Type: term.EventKey})
	assert.True(t, handled)
	assert.False(t, exit)

	for i := 0; i < 3; i++ {
		w := term.NewStringWriter(8, 4)
		<-ch
		h.Draw(w)
		var expected string
		switch i {
		case 2:
			expected = "0000    \n0000    \n    0000\n    0000"
		case 0:
			expected = "0000    \n0000    \n    0000\n    0000"
		case 1:
			expected = "1111    \n1111    \n    1111\n    1111"
		default:
			panic("hmmm")
		}

		require.NoError(t, w.Flush())
		assert.Equal(t, expected, w.String(), i)
	}

	// close
	exit, handled = h.Handle(term.Event{Key: term.KeyEsc, Type: term.EventKey})
	assert.True(t, handled)
	assert.True(t, exit)
	goleak.VerifyNone(t)

	// after close animation should be paused and cause no panics
	for i := 0; i < 3; i++ {
		w := term.NewStringWriter(8, 4)
		h.Draw(w)
		expected := "0000    \n0000    \n    0000\n    0000"
		require.NoError(t, w.Flush())
		assert.Equal(t, expected, w.String(), i)
	}
}
