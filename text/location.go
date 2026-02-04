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

package text

import (
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
)

// LocationList is the interface that groups Prev and Next
// location methods to fetch the previous and next item respectively.
//
// Both Prev/Next return false if reached either end or beginning of list.
//
// Current returns the current result. If there are no results in the list
// then Current returns false.
type LocationList interface {
	Current() (textapi.Location, bool)
	Prev() (textapi.Location, bool)
	Next() (textapi.Location, bool)
}

// LocationSlice returns a LocationList based on in
func LocationSlice(in []textapi.Location) LocationList {
	return &sliceLocations{in: in}
}

type sliceLocations struct {
	curr int
	in   []textapi.Location
}

func (s *sliceLocations) Current() (textapi.Location, bool) {
	if s.curr >= len(s.in) || s.curr < 0 {
		return textapi.Location{}, false
	}

	return s.in[s.curr], true
}

func (s *sliceLocations) Prev() (textapi.Location, bool) {
	if s.curr-1 < 0 {
		return textapi.Location{}, false
	}
	s.curr--
	return s.in[s.curr], true
}

func (s *sliceLocations) Next() (textapi.Location, bool) {
	if s.curr+1 >= len(s.in) {
		return textapi.Location{}, false
	}
	s.curr++
	return s.in[s.curr], true
}
