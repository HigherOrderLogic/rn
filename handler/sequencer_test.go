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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestSequencer(t *testing.T) {
	t.Run("returns sequence match", func(t *testing.T) {
		interests := []Sequence{{
			First: term.KeyComb{Ch: 'd'},
			Last:  term.KeyComb{Ch: 'd'},
		}}
		s := NewSequencer(interests, 1*time.Hour)

		seq, match := s.Sequence(term.KeyComb{Ch: 'd'})
		assert.Equal(t, SequencePartialMatch, match)
		assert.Zero(t, seq)

		seq, match = s.Sequence(term.KeyComb{Ch: 'd'})
		require.Equal(t, SequenceMatch, match)
		assert.Equal(t, interests[0], seq)
	})
	t.Run("does not return match if it took too long for second event", func(t *testing.T) {
		interests := []Sequence{{
			First: term.KeyComb{Ch: 'd'},
			Last:  term.KeyComb{Ch: 'd'},
		}}
		s := NewSequencer(interests, 1*time.Nanosecond)

		seq, match := s.Sequence(term.KeyComb{Ch: 'd'})
		assert.Equal(t, SequencePartialMatch, match)
		assert.Zero(t, seq)

		time.Sleep(time.Millisecond)

		seq, match = s.Sequence(term.KeyComb{Ch: 'd'})
		require.Equal(t, SequencePartialMatch, match)
		assert.Zero(t, seq)
	})
	t.Run("returns sequence match even if last event failed to match with initial event", func(t *testing.T) {
		interests := []Sequence{{
			First: term.KeyComb{Ch: 'd'},
			Last:  term.KeyComb{Ch: 'd'},
		}}
		s := NewSequencer(interests, 25*time.Millisecond)

		seq, match := s.Sequence(term.KeyComb{Ch: 'd'})
		assert.Equal(t, SequencePartialMatch, match)
		assert.Zero(t, seq)

		time.Sleep(40 * time.Millisecond)

		seq, match = s.Sequence(term.KeyComb{Ch: 'd'})
		assert.Equal(t, SequencePartialMatch, match)
		assert.Zero(t, seq)

		seq, match = s.Sequence(term.KeyComb{Ch: 'd'})
		assert.Equal(t, SequenceMatch, match)
		assert.Equal(t, interests[0], seq)
	})
	t.Run("returns SequenceNoMatch if there's is no match", func(t *testing.T) {
		interests := []Sequence{{
			First: term.KeyComb{Ch: 'x'},
			Last:  term.KeyComb{Ch: 'd'},
		}}
		s := NewSequencer(interests, 1*time.Hour)

		seq, match := s.Sequence(term.KeyComb{Ch: 'd'})
		assert.Equal(t, SequenceNoMatch, match)
		assert.Zero(t, seq)

		seq, match = s.Sequence(term.KeyComb{Ch: 'x'})
		require.Equal(t, SequencePartialMatch, match)
		assert.Zero(t, seq)

		seq, match = s.Sequence(term.KeyComb{Ch: 'd'})
		require.Equal(t, SequenceMatch, match)
		assert.Equal(t, interests[0], seq)
	})
}

func TestParseSequence(t *testing.T) {
	tsuite := []struct {
		in      string
		wantSeq Sequence
		wantErr bool
	}{
		{"", Sequence{}, true},
		{"f", Sequence{}, true},
		{"<c-x>", Sequence{}, true},
		{"<><>", Sequence{}, true},
		{"<c-s> ", Sequence{}, true},
		{" <c-p>", Sequence{}, true},
		{"<c-pf", Sequence{}, true},
		{"f<c-p", Sequence{}, true},
		{"c-p>f", Sequence{}, true},
		{">c-p<f", Sequence{}, true},
		{"f>c-p<", Sequence{}, true},
		{"><><", Sequence{}, true},
		{">>", Sequence{}, true},
		{"<<", Sequence{}, true},
		{"                                               ", Sequence{}, true},
		{"ff", Sequence{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Ch: 'f'},
		}, false},
		{"\\>\\>", Sequence{
			First: term.KeyComb{Ch: '>'},
			Last:  term.KeyComb{Ch: '>'},
		}, false},
		{"\\<\\<", Sequence{
			First: term.KeyComb{Ch: '<'},
			Last:  term.KeyComb{Ch: '<'},
		}, false},
		{"<c-x>\\\\", Sequence{
			First: term.KeyComb{Ch: 'x', Mod: term.ModCtrl},
			Last:  term.KeyComb{Ch: '\\'},
		}, false},
		{"f<c-p>", Sequence{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Ch: 'p', Mod: term.ModCtrl},
		}, false},
		{"<c-p>f", Sequence{
			First: term.KeyComb{Ch: 'p', Mod: term.ModCtrl},
			Last:  term.KeyComb{Ch: 'f'},
		}, false},
		{"<c-x><c-p>", Sequence{
			First: term.KeyComb{Ch: 'x', Mod: term.ModCtrl},
			Last:  term.KeyComb{Ch: 'p', Mod: term.ModCtrl},
		}, false},
		{"<c-m-]><c-p>", Sequence{
			First: term.KeyComb{
				Ch:  ']',
				Mod: term.ModCtrlMeta,
			},
			Last: term.KeyComb{Ch: 'p', Mod: term.ModCtrl},
		}, false},
	}

	for _, tcase := range tsuite {
		t.Run(fmt.Sprintf("test case: '%s'", tcase.in), func(t *testing.T) {
			actualSeq, actualErr := ParseSequence(tcase.in)
			if tcase.wantErr {
				require.Error(t, actualErr)
			} else {
				require.NoError(t, actualErr)
			}
			assert.Equal(t, tcase.wantSeq, actualSeq)
			if tcase.wantErr {
				return
			}

			// test that it's able to parse Sequence.String again
			// into the original sequence
			actualSeq, actualErr = ParseSequence(actualSeq.String())
			if tcase.wantErr {
				require.Error(t, actualErr, actualSeq.String())
			} else {
				require.NoError(t, actualErr)
			}
			assert.Equal(t, tcase.wantSeq, actualSeq)
		})
	}
}
