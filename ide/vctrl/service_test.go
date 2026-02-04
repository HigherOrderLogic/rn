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

package vctrl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestDiffToLocationList(t *testing.T) {
	suite := []struct {
		description string
		diff        FileDiff
		expected    []textapi.Location
	}{
		{"empty FileDiff returns empty locations", FileDiff{}, []textapi.Location{}},
		{"converts an add operation", FileDiff{Hunks: []Hunk{
			{NewStartLine: 1, NewLines: 1},
		}}, []textapi.Location{
			{
				From: term.Coordinates{},
				To:   term.Coordinates{Y: 1},
				Attr: term.Attributes{Fg: tcell.ColorGreen},
			},
		}},
		{"converts a delete operation", FileDiff{Hunks: []Hunk{
			{OrigStartLine: 1, OrigLines: 1},
		}}, []textapi.Location{
			{
				From: term.Coordinates{},
				To:   term.Coordinates{Y: 0, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed},
			},
		}},
		{"converts a replace operation into an add", FileDiff{Hunks: []Hunk{
			{OrigStartLine: 1, OrigLines: 1, NewStartLine: 1, NewLines: 1},
		}}, []textapi.Location{
			{
				From: term.Coordinates{},
				To:   term.Coordinates{Y: 1},
				Attr: term.Attributes{Fg: tcell.ColorGreen},
			},
		}},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			actual := test.diff.LocationList(
				term.Attributes{Fg: tcell.ColorRed}, term.Attributes{Fg: tcell.ColorGreen})
			locs := make([]textapi.Location, 0)
			for loc, ok := actual.Current(); ok; loc, ok = actual.Next() {
				locs = append(locs, loc)
			}
			assert.Equal(t, test.expected, locs)
		})
	}
}
