package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/ernestrc/go-tui/term"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{"ff", Sequence{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Ch: 'f'},
		}, false},
		{">>", Sequence{
			First: term.KeyComb{Ch: '>'},
			Last:  term.KeyComb{Ch: '>'},
		}, false},
		{"<<", Sequence{
			First: term.KeyComb{Ch: '<'},
			Last:  term.KeyComb{Ch: '<'},
		}, false},
		{"f<c-p>", Sequence{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Key: term.KeyCtrlP},
		}, false},
		{"<c-p>f", Sequence{
			First: term.KeyComb{Key: term.KeyCtrlP},
			Last:  term.KeyComb{Ch: 'f'},
		}, false},
		{"<c-x><c-p>", Sequence{
			First: term.KeyComb{Key: term.KeyCtrlX},
			Last:  term.KeyComb{Key: term.KeyCtrlP},
		}, false},
		{"<m-c-]><c-p>", Sequence{
			First: term.KeyComb{
				Key: term.KeyCtrlRsqBracket,
				Mod: term.ModAlt,
			},
			Last: term.KeyComb{Key: term.KeyCtrlP},
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
		})
	}
}
