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

package cell

import (
	"container/list"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// Searcher is an interface that wraps methods to search text in a View.
//
// Search performs a text search on a View. It populates a search list
// so subsequent calls to PrevResult and NextResult can scroll through the results.
// The return value represents number of text occurrences found.
//
// PrevResult returns the coordinates of the previous result in the Search list
// and moves the search result pointer. Returns false if there aren't any search
// matches. Implementors should wrap around and continue to the
// last match once result list is exhausted.
//
// NextResult returns the coordinates of the next result in the Search list
// and moves the search result pointer. Returns false if there aren't any search
// matches. Implementors should wrap around and continue to the
// first match once result list is exhausted.
//
// Result returns the search pointer's coordinates or false if there is no
// search result.
//
// Reset resets the internal state of a Searcher.
type Searcher interface {
	Search(text string) int
	PrevResult() (pos term.Coordinates, ok bool)
	NextResult() (pos term.Coordinates, ok bool)
	Result() (pos term.Coordinates, ok bool)
	Reset()
}

// SubscriberSearcher is an interface that groups the Searcher and Subscriber interface.
// Searchers installed on a mutable buffer MUST satisfy Subscriber,
// otherwise search results become stale as buffer is updated.
type SubscriberSearcher interface {
	Subscriber
	Searcher
}

type simpleSearcher struct {
	reader  View
	reslist list.List
	result  *list.Element
}

// NewSimpleSearcher returns a Searcher which uses a simple O(n) algorithm to
// preemptively find all occurrences of the given string upon calling Search.
func NewSimpleSearcher(reader View) Searcher {
	s := new(simpleSearcher)
	s.reader = reader
	s.Reset()
	return s
}

func (s *simpleSearcher) Search(text string) int {
	s.reslist.Init()
	s.result = nil

	searchRunes := []rune(text)
	slen := len(searchRunes)
	if slen == 0 {
		return 0
	}

	var pos term.Coordinates
	var o int
	for y, r := range s.reader.RawCells() {
		for x, c := range r {
			if c.Ch != searchRunes[o] {
				o = 0
			} else if o == 0 {
				pos = term.Coordinates{X: x, Y: y}
				o++
			} else {
				o++
			}
			if o == slen {
				s.reslist.PushBack(pos)
				o = 0
			}
		}
		// search is not performed across rows
		o = 0
	}

	return s.reslist.Len()
}

// PrevResult returns the coordinates of the previous result in the Search list.
func (s *simpleSearcher) PrevResult() (pos term.Coordinates, ok bool) {
	if s.result == nil {
		s.result = s.reslist.Back()
	} else if s.result = s.result.Prev(); s.result == nil {
		s.result = s.reslist.Back()
	}

	if s.result == nil {
		return
	}

	pos = s.result.Value.(term.Coordinates)
	ok = true
	return
}

// NextResult returns the coordinates of the next result in the Search list.
func (s *simpleSearcher) NextResult() (pos term.Coordinates, ok bool) {
	if s.result == nil {
		s.result = s.reslist.Front()
	} else if s.result = s.result.Next(); s.result == nil {
		s.result = s.reslist.Front()
	}

	if s.result == nil {
		return
	}

	pos = s.result.Value.(term.Coordinates)
	ok = true
	return
}

// Result returns the current search result's coordinates.
func (s *simpleSearcher) Result() (pos term.Coordinates, ok bool) {
	if s.result == nil {
		ok = false
		return
	}
	pos = s.result.Value.(term.Coordinates)
	ok = true
	return
}

func (s *simpleSearcher) Reset() {
	s.result = nil
	s.reslist.Init()
}
