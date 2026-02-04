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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var (
	loc1 = textapi.Location{
		To:   term.Coordinates{X: 1, Y: 3},
		Attr: term.Attributes{Attrs: tcell.AttrBold},
	}
	loc2 = textapi.Location{
		From:    term.Coordinates{X: 1, Y: 3},
		Attr:    term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorGreen},
		Message: "wsb: hold BBBY",
	}
	loc3 = textapi.Location{}
)

func resetLocationList(l LocationList) {
	for {
		_, ok := l.Prev()
		if !ok {
			break
		}
	}
}

func assertLocation(t *testing.T, l LocationList, idx int, loca textapi.Location) {
	resetLocationList(l)
	var i int
	for loc, ok := l.Current(); ok; loc, ok = l.Next() {
		if idx == i {
			assert.Equal(t, loca, loc)
			return
		}
		i++
	}
}
func assertLocationListLen(t *testing.T, l LocationList, length int) {
	resetLocationList(l)
	var i int
	for _, ok := l.Current(); ok; _, ok = l.Next() {
		i++
	}
	assert.Equal(t, length, i)
}

func TestLocationSlice(t *testing.T) {
	l := LocationSlice([]textapi.Location{
		loc2,
	})

	assertLocation(t, l, 0, loc2)
	assertLocationListLen(t, l, 1)

	l = LocationSlice([]textapi.Location{
		loc1,
		loc2,
		loc3,
	})

	assertLocation(t, l, 0, loc1)
	assertLocation(t, l, 1, loc2)
	assertLocation(t, l, 2, loc3)
	assertLocationListLen(t, l, 3)

	l = LocationSlice([]textapi.Location{})
	assertLocationListLen(t, l, 0)

	l = LocationSlice(nil)
	assertLocationListLen(t, l, 0)
}

func BenchmarkSetLocationsAndDrawWrapBalancedSize(b *testing.B) {
	benchmarkSetLocationsAndDrawSize(b, true, 30, 15)
}

func BenchmarkSetLocationsAndDrawNoWrapBalancedSize(b *testing.B) {
	benchmarkSetLocationsAndDrawSize(b, false, 30, 15)
}

func BenchmarkSetLocationsAndDrawWrapAllVisibleSize(b *testing.B) {
	benchmarkSetLocationsAndDrawSize(b, true, 100, 100)
}

func BenchmarkSetLocationsAndDrawNoWrapAllVisibleSize(b *testing.B) {
	benchmarkSetLocationsAndDrawSize(b, false, 100, 100)
}

func BenchmarkSetLocationsAndDrawWrapMostlyNonVisibleSize(b *testing.B) {
	benchmarkSetLocationsAndDrawSize(b, true, 10, 5)
}

func BenchmarkSetLocationsAndDrawNoWrapMostlyNonVisibleSize(b *testing.B) {
	benchmarkSetLocationsAndDrawSize(b, false, 10, 5)
}

func benchmarkSetLocationsAndDrawSize(b *testing.B, wrap bool, width, height int) {
	const file = `
package auth

import "context"

// CombineKeys combines a set of Keys, tipically used in calls to Verify.
// Sign will return the key in k.
func CombineKeys(k Keys, extra ...Keys) Keys {
	keys := make([]Keys, 0, len(extra)+1)
	keys = append(keys, k)
	keys = append(keys, extra...)
	return multiKeys{keys: keys}
}

type multiKeys struct {
	keys []Keys
}

func (m multiKeys) Sign(ctx context.Context) (Key, error) {
	return m.keys[0].Sign(ctx)
}

func (m multiKeys) Verify(ctx context.Context) (ret []Key, err error) {
	for _, keys := range m.keys {
		other, err := keys.Verify(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, other...)
	}
	return
}
`
	scroll := component.NewScroll(cell.NewBuffer())
	scroll.Wrap = wrap
	scroll.Resize(width, height)
	_, err := scroll.Buffer().ReadFrom(strings.NewReader(file))
	require.NoError(b, err)

	locations := []textapi.Location{
		{From: term.Coordinates{X: 0, Y: 0}, To: term.Coordinates{X: 7, Y: 0}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 7, Y: 0}, To: term.Coordinates{X: 8, Y: 0}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 0}, To: term.Coordinates{X: 12, Y: 0}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 12, Y: 0}, To: term.Coordinates{X: 13, Y: 0}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 1}, To: term.Coordinates{X: 0, Y: 1}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 1}, To: term.Coordinates{X: 1, Y: 1}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 2}, To: term.Coordinates{X: 0, Y: 2}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 2}, To: term.Coordinates{X: 6, Y: 2}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 6, Y: 2}, To: term.Coordinates{X: 7, Y: 2}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 7, Y: 2}, To: term.Coordinates{X: 16, Y: 2}, Attr: term.Attributes{Fg: tcell.ColorFuchsia, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 16, Y: 2}, To: term.Coordinates{X: 17, Y: 2}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 3}, To: term.Coordinates{X: 0, Y: 3}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 3}, To: term.Coordinates{X: 1, Y: 3}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 4}, To: term.Coordinates{X: 0, Y: 4}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 4}, To: term.Coordinates{X: 74, Y: 4}, Attr: term.Attributes{Fg: tcell.ColorBlue, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 5}, To: term.Coordinates{X: 0, Y: 5}, Attr: term.Attributes{Fg: tcell.ColorBlue, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 5}, To: term.Coordinates{X: 34, Y: 5}, Attr: term.Attributes{Fg: tcell.ColorBlue, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 6}, To: term.Coordinates{X: 0, Y: 6}, Attr: term.Attributes{Fg: tcell.ColorBlue, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 6}, To: term.Coordinates{X: 4, Y: 6}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 6}, To: term.Coordinates{X: 5, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 5, Y: 6}, To: term.Coordinates{X: 16, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 16, Y: 6}, To: term.Coordinates{X: 17, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 6}, To: term.Coordinates{X: 18, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 6}, To: term.Coordinates{X: 19, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 6}, To: term.Coordinates{X: 23, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 6}, To: term.Coordinates{X: 24, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 6}, To: term.Coordinates{X: 25, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 6}, To: term.Coordinates{X: 30, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 30, Y: 6}, To: term.Coordinates{X: 31, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 31, Y: 6}, To: term.Coordinates{X: 34, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 34, Y: 6}, To: term.Coordinates{X: 38, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 38, Y: 6}, To: term.Coordinates{X: 39, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 39, Y: 6}, To: term.Coordinates{X: 40, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 40, Y: 6}, To: term.Coordinates{X: 44, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 44, Y: 6}, To: term.Coordinates{X: 45, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 45, Y: 6}, To: term.Coordinates{X: 46, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 46, Y: 6}, To: term.Coordinates{X: 47, Y: 6}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 7}, To: term.Coordinates{X: 3, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 7}, To: term.Coordinates{X: 4, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 7}, To: term.Coordinates{X: 8, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 7}, To: term.Coordinates{X: 9, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 9, Y: 7}, To: term.Coordinates{X: 11, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 7}, To: term.Coordinates{X: 12, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 12, Y: 7}, To: term.Coordinates{X: 16, Y: 7}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 16, Y: 7}, To: term.Coordinates{X: 17, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 7}, To: term.Coordinates{X: 18, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 7}, To: term.Coordinates{X: 19, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 7}, To: term.Coordinates{X: 23, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 7}, To: term.Coordinates{X: 24, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 7}, To: term.Coordinates{X: 25, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 7}, To: term.Coordinates{X: 26, Y: 7}, Attr: term.Attributes{Fg: tcell.ColorRed, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 7}, To: term.Coordinates{X: 27, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 27, Y: 7}, To: term.Coordinates{X: 28, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 28, Y: 7}, To: term.Coordinates{X: 31, Y: 7}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 31, Y: 7}, To: term.Coordinates{X: 32, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 32, Y: 7}, To: term.Coordinates{X: 37, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 37, Y: 7}, To: term.Coordinates{X: 38, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 38, Y: 7}, To: term.Coordinates{X: 39, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 39, Y: 7}, To: term.Coordinates{X: 40, Y: 7}, Attr: term.Attributes{Fg: tcell.ColorRed, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 40, Y: 7}, To: term.Coordinates{X: 41, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 41, Y: 7}, To: term.Coordinates{X: 42, Y: 7}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 8}, To: term.Coordinates{X: 3, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 8}, To: term.Coordinates{X: 4, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 8}, To: term.Coordinates{X: 8, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 8}, To: term.Coordinates{X: 9, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 9, Y: 8}, To: term.Coordinates{X: 10, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 8}, To: term.Coordinates{X: 11, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 8}, To: term.Coordinates{X: 17, Y: 8}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 8}, To: term.Coordinates{X: 18, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 8}, To: term.Coordinates{X: 22, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 22, Y: 8}, To: term.Coordinates{X: 23, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 8}, To: term.Coordinates{X: 24, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 8}, To: term.Coordinates{X: 25, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 8}, To: term.Coordinates{X: 26, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 8}, To: term.Coordinates{X: 27, Y: 8}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 9}, To: term.Coordinates{X: 3, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 9}, To: term.Coordinates{X: 4, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 9}, To: term.Coordinates{X: 8, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 9}, To: term.Coordinates{X: 9, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 9, Y: 9}, To: term.Coordinates{X: 10, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 9}, To: term.Coordinates{X: 11, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 9}, To: term.Coordinates{X: 17, Y: 9}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 9}, To: term.Coordinates{X: 18, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 9}, To: term.Coordinates{X: 22, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 22, Y: 9}, To: term.Coordinates{X: 23, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 9}, To: term.Coordinates{X: 24, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 9}, To: term.Coordinates{X: 29, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 29, Y: 9}, To: term.Coordinates{X: 32, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 32, Y: 9}, To: term.Coordinates{X: 33, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 33, Y: 9}, To: term.Coordinates{X: 34, Y: 9}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 10}, To: term.Coordinates{X: 3, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 10}, To: term.Coordinates{X: 4, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 10}, To: term.Coordinates{X: 10, Y: 10}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 10}, To: term.Coordinates{X: 11, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 10}, To: term.Coordinates{X: 20, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 20, Y: 10}, To: term.Coordinates{X: 21, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 21, Y: 10}, To: term.Coordinates{X: 25, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 10}, To: term.Coordinates{X: 26, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 10}, To: term.Coordinates{X: 27, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 27, Y: 10}, To: term.Coordinates{X: 31, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 31, Y: 10}, To: term.Coordinates{X: 32, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 32, Y: 10}, To: term.Coordinates{X: 33, Y: 10}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 11}, To: term.Coordinates{X: 0, Y: 11}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 11}, To: term.Coordinates{X: 1, Y: 11}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 1, Y: 11}, To: term.Coordinates{X: 2, Y: 11}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 12}, To: term.Coordinates{X: 0, Y: 12}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 12}, To: term.Coordinates{X: 1, Y: 12}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 13}, To: term.Coordinates{X: 0, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 13}, To: term.Coordinates{X: 4, Y: 13}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 13}, To: term.Coordinates{X: 5, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 5, Y: 13}, To: term.Coordinates{X: 14, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 14, Y: 13}, To: term.Coordinates{X: 15, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 15, Y: 13}, To: term.Coordinates{X: 21, Y: 13}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 21, Y: 13}, To: term.Coordinates{X: 22, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 22, Y: 13}, To: term.Coordinates{X: 23, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 13}, To: term.Coordinates{X: 24, Y: 13}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 14}, To: term.Coordinates{X: 3, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 14}, To: term.Coordinates{X: 4, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 14}, To: term.Coordinates{X: 8, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 14}, To: term.Coordinates{X: 9, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 9, Y: 14}, To: term.Coordinates{X: 10, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 14}, To: term.Coordinates{X: 11, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 14}, To: term.Coordinates{X: 15, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 15, Y: 14}, To: term.Coordinates{X: 16, Y: 14}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 15}, To: term.Coordinates{X: 0, Y: 15}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 15}, To: term.Coordinates{X: 1, Y: 15}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 1, Y: 15}, To: term.Coordinates{X: 2, Y: 15}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 16}, To: term.Coordinates{X: 0, Y: 16}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 16}, To: term.Coordinates{X: 1, Y: 16}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 17}, To: term.Coordinates{X: 0, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 17}, To: term.Coordinates{X: 4, Y: 17}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 17}, To: term.Coordinates{X: 5, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 5, Y: 17}, To: term.Coordinates{X: 6, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 6, Y: 17}, To: term.Coordinates{X: 7, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 7, Y: 17}, To: term.Coordinates{X: 8, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 17}, To: term.Coordinates{X: 17, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 17}, To: term.Coordinates{X: 18, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 17}, To: term.Coordinates{X: 19, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 17}, To: term.Coordinates{X: 23, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 17}, To: term.Coordinates{X: 24, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 17}, To: term.Coordinates{X: 27, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 27, Y: 17}, To: term.Coordinates{X: 28, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 28, Y: 17}, To: term.Coordinates{X: 35, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 35, Y: 17}, To: term.Coordinates{X: 36, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 36, Y: 17}, To: term.Coordinates{X: 43, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 43, Y: 17}, To: term.Coordinates{X: 44, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 44, Y: 17}, To: term.Coordinates{X: 45, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 45, Y: 17}, To: term.Coordinates{X: 46, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 46, Y: 17}, To: term.Coordinates{X: 49, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 49, Y: 17}, To: term.Coordinates{X: 50, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 50, Y: 17}, To: term.Coordinates{X: 51, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 51, Y: 17}, To: term.Coordinates{X: 56, Y: 17}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 56, Y: 17}, To: term.Coordinates{X: 57, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 57, Y: 17}, To: term.Coordinates{X: 58, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 58, Y: 17}, To: term.Coordinates{X: 59, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 59, Y: 17}, To: term.Coordinates{X: 60, Y: 17}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 18}, To: term.Coordinates{X: 3, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 18}, To: term.Coordinates{X: 4, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 18}, To: term.Coordinates{X: 10, Y: 18}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 18}, To: term.Coordinates{X: 11, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 18}, To: term.Coordinates{X: 12, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 12, Y: 18}, To: term.Coordinates{X: 13, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 13, Y: 18}, To: term.Coordinates{X: 17, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 18}, To: term.Coordinates{X: 18, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 18}, To: term.Coordinates{X: 19, Y: 18}, Attr: term.Attributes{Fg: tcell.ColorRed, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 18}, To: term.Coordinates{X: 20, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 20, Y: 18}, To: term.Coordinates{X: 21, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 21, Y: 18}, To: term.Coordinates{X: 25, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 18}, To: term.Coordinates{X: 26, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 18}, To: term.Coordinates{X: 29, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 29, Y: 18}, To: term.Coordinates{X: 30, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 30, Y: 18}, To: term.Coordinates{X: 31, Y: 18}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 19}, To: term.Coordinates{X: 0, Y: 19}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 19}, To: term.Coordinates{X: 1, Y: 19}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 1, Y: 19}, To: term.Coordinates{X: 2, Y: 19}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 20}, To: term.Coordinates{X: 0, Y: 20}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 20}, To: term.Coordinates{X: 1, Y: 20}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 21}, To: term.Coordinates{X: 0, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 21}, To: term.Coordinates{X: 4, Y: 21}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 21}, To: term.Coordinates{X: 5, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 5, Y: 21}, To: term.Coordinates{X: 6, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 6, Y: 21}, To: term.Coordinates{X: 7, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 7, Y: 21}, To: term.Coordinates{X: 8, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 21}, To: term.Coordinates{X: 17, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 21}, To: term.Coordinates{X: 18, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 21}, To: term.Coordinates{X: 19, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 21}, To: term.Coordinates{X: 25, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 21}, To: term.Coordinates{X: 26, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 21}, To: term.Coordinates{X: 29, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 29, Y: 21}, To: term.Coordinates{X: 30, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 30, Y: 21}, To: term.Coordinates{X: 37, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 37, Y: 21}, To: term.Coordinates{X: 38, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 38, Y: 21}, To: term.Coordinates{X: 45, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 45, Y: 21}, To: term.Coordinates{X: 46, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 46, Y: 21}, To: term.Coordinates{X: 47, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 47, Y: 21}, To: term.Coordinates{X: 48, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 48, Y: 21}, To: term.Coordinates{X: 51, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 51, Y: 21}, To: term.Coordinates{X: 52, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 52, Y: 21}, To: term.Coordinates{X: 53, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 53, Y: 21}, To: term.Coordinates{X: 54, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 54, Y: 21}, To: term.Coordinates{X: 57, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 57, Y: 21}, To: term.Coordinates{X: 58, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 58, Y: 21}, To: term.Coordinates{X: 59, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 59, Y: 21}, To: term.Coordinates{X: 62, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 62, Y: 21}, To: term.Coordinates{X: 63, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 63, Y: 21}, To: term.Coordinates{X: 68, Y: 21}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 68, Y: 21}, To: term.Coordinates{X: 69, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 69, Y: 21}, To: term.Coordinates{X: 70, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 70, Y: 21}, To: term.Coordinates{X: 71, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 71, Y: 21}, To: term.Coordinates{X: 72, Y: 21}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 22}, To: term.Coordinates{X: 3, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 22}, To: term.Coordinates{X: 4, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 22}, To: term.Coordinates{X: 7, Y: 22}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 7, Y: 22}, To: term.Coordinates{X: 8, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 22}, To: term.Coordinates{X: 9, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 9, Y: 22}, To: term.Coordinates{X: 10, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 22}, To: term.Coordinates{X: 11, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 22}, To: term.Coordinates{X: 15, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 15, Y: 22}, To: term.Coordinates{X: 16, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 16, Y: 22}, To: term.Coordinates{X: 18, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 22}, To: term.Coordinates{X: 19, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 22}, To: term.Coordinates{X: 24, Y: 22}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 22}, To: term.Coordinates{X: 25, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 22}, To: term.Coordinates{X: 26, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 22}, To: term.Coordinates{X: 27, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 27, Y: 22}, To: term.Coordinates{X: 31, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 31, Y: 22}, To: term.Coordinates{X: 32, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 32, Y: 22}, To: term.Coordinates{X: 33, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 33, Y: 22}, To: term.Coordinates{X: 34, Y: 22}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 23}, To: term.Coordinates{X: 3, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 23}, To: term.Coordinates{X: 8, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 23}, To: term.Coordinates{X: 13, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 13, Y: 23}, To: term.Coordinates{X: 14, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 14, Y: 23}, To: term.Coordinates{X: 15, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 15, Y: 23}, To: term.Coordinates{X: 18, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 23}, To: term.Coordinates{X: 19, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 23}, To: term.Coordinates{X: 21, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 21, Y: 23}, To: term.Coordinates{X: 22, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 22, Y: 23}, To: term.Coordinates{X: 26, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 23}, To: term.Coordinates{X: 27, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 27, Y: 23}, To: term.Coordinates{X: 33, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 33, Y: 23}, To: term.Coordinates{X: 34, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 34, Y: 23}, To: term.Coordinates{X: 37, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 37, Y: 23}, To: term.Coordinates{X: 38, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 38, Y: 23}, To: term.Coordinates{X: 39, Y: 23}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 24}, To: term.Coordinates{X: 3, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 24}, To: term.Coordinates{X: 8, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 24}, To: term.Coordinates{X: 10, Y: 24}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 24}, To: term.Coordinates{X: 11, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 24}, To: term.Coordinates{X: 14, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 14, Y: 24}, To: term.Coordinates{X: 15, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 15, Y: 24}, To: term.Coordinates{X: 17, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 17, Y: 24}, To: term.Coordinates{X: 18, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 24}, To: term.Coordinates{X: 21, Y: 24}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 21, Y: 24}, To: term.Coordinates{X: 22, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 22, Y: 24}, To: term.Coordinates{X: 23, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 24}, To: term.Coordinates{X: 24, Y: 24}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 25}, To: term.Coordinates{X: 3, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 25}, To: term.Coordinates{X: 12, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 12, Y: 25}, To: term.Coordinates{X: 18, Y: 25}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 18, Y: 25}, To: term.Coordinates{X: 19, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 19, Y: 25}, To: term.Coordinates{X: 22, Y: 25}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 22, Y: 25}, To: term.Coordinates{X: 23, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 23, Y: 25}, To: term.Coordinates{X: 24, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 25}, To: term.Coordinates{X: 27, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 27, Y: 25}, To: term.Coordinates{X: 28, Y: 25}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 26}, To: term.Coordinates{X: 3, Y: 26}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 26}, To: term.Coordinates{X: 8, Y: 26}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 26}, To: term.Coordinates{X: 9, Y: 26}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 9, Y: 26}, To: term.Coordinates{X: 10, Y: 26}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 27}, To: term.Coordinates{X: 3, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 27}, To: term.Coordinates{X: 8, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 8, Y: 27}, To: term.Coordinates{X: 11, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 11, Y: 27}, To: term.Coordinates{X: 12, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 12, Y: 27}, To: term.Coordinates{X: 13, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 13, Y: 27}, To: term.Coordinates{X: 14, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 14, Y: 27}, To: term.Coordinates{X: 20, Y: 27}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 20, Y: 27}, To: term.Coordinates{X: 21, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 21, Y: 27}, To: term.Coordinates{X: 24, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 24, Y: 27}, To: term.Coordinates{X: 25, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 25, Y: 27}, To: term.Coordinates{X: 26, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 26, Y: 27}, To: term.Coordinates{X: 31, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 31, Y: 27}, To: term.Coordinates{X: 34, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 34, Y: 27}, To: term.Coordinates{X: 35, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 35, Y: 27}, To: term.Coordinates{X: 36, Y: 27}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 28}, To: term.Coordinates{X: 3, Y: 28}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 28}, To: term.Coordinates{X: 4, Y: 28}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 28}, To: term.Coordinates{X: 5, Y: 28}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 5, Y: 28}, To: term.Coordinates{X: 6, Y: 28}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 29}, To: term.Coordinates{X: 3, Y: 29}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 3, Y: 29}, To: term.Coordinates{X: 4, Y: 29}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 4, Y: 29}, To: term.Coordinates{X: 10, Y: 29}, Attr: term.Attributes{Fg: tcell.ColorYellow, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 10, Y: 29}, To: term.Coordinates{X: 11, Y: 29}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 30}, To: term.Coordinates{X: 0, Y: 30}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
		{From: term.Coordinates{X: 0, Y: 30}, To: term.Coordinates{X: 1, Y: 30}, Attr: term.Attributes{Fg: 0, Bg: 0, Attrs: 0}},
	}

	b.ResetTimer()

	w := term.NoopWriter{}
	for i := 0; i < b.N; i++ {
		DrawLocations(locations, scroll, w)
	}
	_ = w.Flush()
}
