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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unstablebuild/rune-go-sdk/term"
	tterm "unstable.build/go-tui/term"
)

// SequenceMatchResult represents the result of sequencing.
type SequenceMatchResult uint8

// List of sequence match results.
const (
	SequenceNoMatch SequenceMatchResult = iota
	SequencePartialMatch
	SequenceMatch
)

// Sequence represents a term.EventKey sequence. For matching purposes,
// only Event.Mod, Event.Key and Event.Ch are considered,
// the rest of fields are ignored.
type Sequence struct {
	First term.KeyComb
	Last  term.KeyComb
}

// Sequencer is a key event sequencer which detects sequences of (2) term.EventKey
// events that are triggered within a specified time.
type Sequencer struct {
	interests map[term.KeyComb]map[term.KeyComb]struct{}
	timeout   time.Duration

	first    term.KeyComb
	firstCtx context.Context
	ctxClean func()
}

// NewSequencer allocates storage for a new Sequencer and initalizes it.
func NewSequencer(interests []Sequence, timeout time.Duration) *Sequencer {
	ret := new(Sequencer)
	ret.Init(interests, timeout)
	return ret
}

// Init initializes this sequencer with the given interests and a timeout.
func (s *Sequencer) Init(interests []Sequence, timeout time.Duration) {
	s.timeout = timeout
	s.interests = make(map[term.KeyComb]map[term.KeyComb]struct{})
	for _, i := range interests {
		first, last := i.First, i.Last
		if _, ok := s.interests[first]; !ok {
			s.interests[first] = make(map[term.KeyComb]struct{})
		}
		s.interests[first][last] = struct{}{}
	}
	// initialize now with canceled context so we don't need to check if ctxClean
	// or firstCtx has been initialized
	s.firstCtx, s.ctxClean = context.WithCancel(context.Background())
	s.ctxClean()
}

// Sequence processes ev and returns whether there's a sequence match or not,
// and if so, which sequence was processed.
func (s *Sequencer) Sequence(key term.KeyComb) (
	seq Sequence, match SequenceMatchResult,
) {
	first := s.first
	last := key

	firstCtx := s.firstCtx
	ctxClean := s.ctxClean
	defer ctxClean()

	s.firstCtx, s.ctxClean = context.WithTimeout(context.Background(), s.timeout)
	s.first = last

	if _, ok := s.interests[last]; ok {
		match = SequencePartialMatch
	} else {
		match = SequenceNoMatch
	}

	select {
	case <-firstCtx.Done():
		return
	default:
	}

	m, ok := s.interests[first]
	if !ok {
		return
	}

	_, ok = m[last]
	if !ok {
		return
	}

	// first is consumed if match is found
	s.Reset()
	seq = Sequence{First: first, Last: last}
	match = SequenceMatch
	return
}

// Reset resets this Sequencer such that the next event passed
// to Handle is considered as a first event.
func (s *Sequencer) Reset() {
	s.ctxClean()
}

// ParseSequence parses str into a Sequence or returns
// error if it fails to parse it. This function is not case sensitive.
// It accepts two key string representation, as they would be individually
// parsed by term.ParseKey: i.e. <c-x><c-p>, f<c-p>, <c-x>t, gf
func ParseSequence(str string) (Sequence, error) {
	keys, err := tterm.ParseKeys(str)
	if err != nil {
		return Sequence{}, fmt.Errorf("invalid sequence: %v", err)
	}
	if len(keys) != 2 {
		return Sequence{}, errors.New("invalid sequence: expected exactly 2 keys")
	}
	return Sequence{First: keys[0], Last: keys[1]}, nil
}

func (s Sequence) String() string {
	var ret strings.Builder
	ret.WriteString(tterm.KeyCombString(s.First))
	ret.WriteString(tterm.KeyCombString(s.Last))
	return ret.String()
}
