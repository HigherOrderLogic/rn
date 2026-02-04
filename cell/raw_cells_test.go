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
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var (
	rawCellsFortune = `Love in your heart wasn't put there to stay.
Love isn't love 'til you give it away.
		-- Oscar Hammerstein 中国`
	emptyString     = ""
	widthString     = "💥"
	graphemeCluster = "👨‍👧‍👦"
)

func TestRawCellsPanicsNegativeCoordinates(t *testing.T) {

	var c rawCells
	c.init()
	negativeCoords := []term.Coordinates{
		{X: -1, Y: 0},
		{X: 0, Y: -1},
	}

	for _, pos := range negativeCoords {
		t.Run("cell()", func(t *testing.T) {
			assert.Panics(t, func() {
				c.Cell(pos)
			})
		})

		t.Run("insert()", func(t *testing.T) {
			assert.Panics(t, func() {
				c.Edit(context.Background(), pos, pos, "r")
			})
		})
		t.Run("delete(from)", func(t *testing.T) {
			assert.Panics(t, func() {
				c.Edit(context.Background(), pos, term.Coordinates{X: 0, Y: 2}, "")
			})
		})
		t.Run("delete(until)", func(t *testing.T) {
			assert.Panics(t, func() {
				c.Edit(context.Background(), term.Coordinates{X: 0, Y: 2}, pos, "")
			})
		})
	}
}

type readFromTestCase struct {
	reads       []string
	errors      []error
	expectedN   int64
	expectedErr error
}

func (r *readFromTestCase) Read(p []byte) (n int, err error) {
	if len(r.reads) == 0 {
		err = io.EOF
		return
	}

	defer func() {
		r.reads = r.reads[1:]
		r.errors = r.errors[1:]
	}()

	read := r.reads[0]
	err = r.errors[0]
	if err != nil {
		return
	}
	n = copy(p, read)
	return
}

func TestRawCellsReset(t *testing.T) {
	t.Run("zero capacity reset does not panic", func(t *testing.T) {
		var c rawCells
		assert.NotPanics(t, func() {
			c.resetWithCap(1, 0)
		})
		assert.NotPanics(t, func() {
			c.resetWithCap(0, 1)
		})
	})

	t.Run("length as capacity is used if default capacity is smaller", func(t *testing.T) {
		var c rawCells
		c.resetWithCap(1, 1)
		c.Edit(context.Background(), term.Coordinates{}, term.Coordinates{},
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		// forces copying half of first row with length
		// into another row, with length and capacity
		c.Edit(context.Background(), term.Coordinates{X: 2}, term.Coordinates{X: 2}, "\n")
	})
}

func TestRawCellsReadFrom(t *testing.T) {
	myError := errors.New("oopsie daisy")

	tsuite := []readFromTestCase{
		{[]string{"a"}, []error{nil}, 1, nil},
		{[]string{""}, []error{nil}, 0, nil},
		{[]string{"a", "b"}, []error{nil, nil}, 2, nil},
		{[]string{"ab", "c"}, []error{nil, nil}, 3, nil},
		{[]string{"a", ""}, []error{nil, io.EOF}, 1, nil},
		{[]string{""}, []error{myError}, 0, myError},
	}

	for i, tcase := range tsuite {
		var c rawCells
		c.init()
		reads := tcase.reads
		n, err := c.ReadFrom(&tcase)
		assert.Equal(t, tcase.expectedErr, err, "tcase %d", i)
		assert.Equal(t, tcase.expectedN, n, "tcase %d", i)
		if tcase.expectedErr == nil {
			assert.Equal(t, strings.Join(reads, ""), c.String())
		}
	}
}

func TestRawCellsStringReadFrom(t *testing.T) {
	tsuite := []struct {
		in   string
		want [][]term.Cell
	}{
		{"", [][]term.Cell{{}}},
		{"\n", [][]term.Cell{{}, {}}},
		{"\t\n", [][]term.Cell{{{Combining: []rune{}, Bytes: 1, Ch: '\t'}}, {}}},
		{"\t", [][]term.Cell{{{Combining: []rune{}, Bytes: 1, Ch: '\t'}}}},
		{"a", [][]term.Cell{{{Ch: 'a', Combining: []rune{}, Bytes: 1, Width: 1}}}},
		{"\nb", [][]term.Cell{{}, {{Ch: 'b', Combining: []rune{}, Bytes: 1, Width: 1}}}},
		{"c\n", [][]term.Cell{{{Ch: 'c', Combining: []rune{}, Bytes: 1, Width: 1}}, {}}},
		{"\n\n\n", [][]term.Cell{{}, {}, {}, {}}},
		{"\n\n\na", [][]term.Cell{{}, {}, {}, {{Ch: 'a', Combining: []rune{}, Bytes: 1, Width: 1}}}},
		{"💥", [][]term.Cell{{{Ch: '💥', Width: 2, Combining: []rune{}, Bytes: 4}}}},
		{"👨‍👧‍👦", [][]term.Cell{{{Ch: '👨', Combining: []rune{
			rune(8205),
			rune(128103),
			rune(8205),
			rune(128102),
		}, Width: 2, Bytes: 18}}}},
	}

	for i, _tcase := range tsuite {
		tcase := _tcase
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			var c rawCells
			c.init()
			n, err := c.ReadFrom(strings.NewReader(tcase.in))
			assert.NoError(t, err)
			assert.Equal(t, int64(len([]byte(tcase.in))), n)
			assert.Equal(t, tcase.in, c.String())
			assert.Equal(t, tcase.want, c.RawCells())
		})
	}
}

func TestRawCellsInsert(t *testing.T) {
	const baseRawCells = `
syntax = "proto2";
package rpc;`

	const expectedRawCellsCase3 = `
syntax = "proto2";
package rpc;


  // what's up`

	const expectedRawCellsCase2 = `
syntax -= "proto2";
package rpc;`

	const expectedRawCellsCase4 = `Love in your heart wasn't put there to stay.

Love isn't love 'til you give it away.
		-- Oscar Hammerstein 中国`

	const expectedRawCellsCase5 = `Love in your heart wasn't put there to stay.
Love isn't love 'til you give it away.
			-- Oscar Hammerstein 中国`

	tsuite := []struct {
		overrideBaseRawCells     *string
		inputStr                 string
		inputAt                  term.Coordinates
		expectedRawCells         string
		expectedFrom, expectedTo term.Coordinates
	}{
		{
			inputStr:         ">>>\n",
			inputAt:          term.Coordinates{},
			expectedRawCells: ">>>\n" + baseRawCells,
			expectedFrom:     term.Coordinates{},
			expectedTo:       term.Coordinates{Y: 1},
		},
		{
			inputStr:         "-",
			inputAt:          term.Coordinates{X: 7, Y: 1},
			expectedRawCells: expectedRawCellsCase2,
			expectedFrom:     term.Coordinates{X: 7, Y: 1},
			expectedTo:       term.Coordinates{X: 8, Y: 1},
		},
		{
			inputStr:         "// what's up",
			inputAt:          term.Coordinates{X: 2, Y: 5},
			expectedRawCells: expectedRawCellsCase3,
			expectedFrom:     term.Coordinates{X: 12, Y: 2},
			expectedTo:       term.Coordinates{X: 14, Y: 5},
		},
		{
			overrideBaseRawCells: &rawCellsFortune,
			expectedRawCells:     expectedRawCellsCase4,
			inputAt:              term.Coordinates{Y: 1},
			inputStr:             "\n",
			expectedFrom:         term.Coordinates{Y: 1},
			expectedTo:           term.Coordinates{Y: 2},
		},
		{
			overrideBaseRawCells: &rawCellsFortune,
			expectedRawCells:     expectedRawCellsCase5,
			inputAt:              term.Coordinates{Y: 2},
			inputStr:             "\t",
			expectedFrom:         term.Coordinates{Y: 2},
			expectedTo:           term.Coordinates{X: 1, Y: 2},
		},
		{
			overrideBaseRawCells: &emptyString,
			expectedRawCells:     "\t\n",
			inputAt:              term.Coordinates{},
			inputStr:             "\t\n",
			expectedFrom:         term.Coordinates{},
			expectedTo:           term.Coordinates{Y: 1},
		},
		{
			overrideBaseRawCells: &emptyString,
			expectedRawCells:     "\n",
			inputAt:              term.Coordinates{},
			inputStr:             "\n",
			expectedFrom:         term.Coordinates{},
			expectedTo:           term.Coordinates{Y: 1},
		},
		{
			overrideBaseRawCells: &emptyString,
			inputAt:              term.Coordinates{Y: 1},
			inputStr:             "\n",
			expectedFrom:         term.Coordinates{Y: 0},
			expectedTo:           term.Coordinates{Y: 1},
			expectedRawCells:     "\n",
		},
		{
			overrideBaseRawCells: &emptyString,
			inputAt:              term.Coordinates{Y: 2},
			inputStr:             "\n\n",
			expectedFrom:         term.Coordinates{Y: 0},
			expectedTo:           term.Coordinates{Y: 2},
			expectedRawCells:     "\n\n",
		},
		{
			overrideBaseRawCells: &emptyString,
			inputAt:              term.Coordinates{X: 0},
			inputStr:             "💥",
			expectedFrom:         term.Coordinates{X: 0},
			expectedTo:           term.Coordinates{X: 1},
			expectedRawCells:     "💥",
		},
		{
			overrideBaseRawCells: &widthString,
			inputAt:              term.Coordinates{X: 1},
			inputStr:             "123",
			expectedFrom:         term.Coordinates{X: 1},
			expectedTo:           term.Coordinates{X: 4},
			expectedRawCells:     "💥123",
		},
		{
			overrideBaseRawCells: &widthString,
			inputAt:              term.Coordinates{X: 0},
			inputStr:             "🤘",
			expectedFrom:         term.Coordinates{X: 0},
			expectedTo:           term.Coordinates{X: 1},
			expectedRawCells:     "🤘💥",
		},
		{
			overrideBaseRawCells: &emptyString,
			inputAt:              term.Coordinates{X: 0},
			inputStr:             "👨‍👧‍👦",
			expectedFrom:         term.Coordinates{X: 0},
			expectedTo:           term.Coordinates{X: 1},
			expectedRawCells:     "👨‍👧‍👦",
		},
		{
			overrideBaseRawCells: &widthString,
			inputAt:              term.Coordinates{X: 0},
			inputStr:             "👨‍👧‍👦",
			expectedFrom:         term.Coordinates{X: 0},
			expectedTo:           term.Coordinates{X: 1},
			expectedRawCells:     "👨‍👧‍👦💥",
		},
		{
			overrideBaseRawCells: &graphemeCluster,
			inputAt:              term.Coordinates{X: 0},
			inputStr:             "💥",
			expectedFrom:         term.Coordinates{X: 0},
			expectedTo:           term.Coordinates{X: 1},
			expectedRawCells:     "💥👨‍👧‍👦",
		},
	}

	for i, tcase := range tsuite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			var c rawCells
			c.init()
			var input string
			if tcase.overrideBaseRawCells != nil {
				input = *tcase.overrideBaseRawCells
			} else {
				input = baseRawCells
			}
			if input != "" {
				_, err := c.ReadFrom(strings.NewReader(input))
				require.NoError(t, err)
			}

			actualFrom, actualTo, actualOld := c.Edit(context.Background(), tcase.inputAt, tcase.inputAt, tcase.inputStr)
			assert.Equal(t, tcase.expectedFrom, actualFrom, "test case %d", i)
			assert.Equal(t, tcase.expectedTo, actualTo, "test case %d", i)
			assert.Equal(t, tcase.expectedRawCells, c.String())
			assert.Zero(t, actualOld)

			from, to, old := c.Edit(context.Background(), actualFrom, actualTo, actualOld)
			assert.Equal(t, input, c.String(), "string ret: %+v %+v", actualFrom, actualTo)

			from, to, old = c.Edit(context.Background(), from, to, old)
			assert.Equal(t, tcase.expectedRawCells, c.String())

			_, _, _ = c.Edit(context.Background(), from, to, old)
			assert.Equal(t, input, c.String())
		})
	}
}

func TestRawCellsInsertGraphemeCluster(t *testing.T) {
	var c rawCells
	c.init()
	_, next := c.insert(term.Coordinates{}, "👨")
	_, next = c.insert(next, "\u200d")
	_, next = c.insert(next, "👧")
	_, next = c.insert(next, "\u200d")
	_, next = c.insert(next, "👦")

	assert.Equal(t, [][]term.Cell{
		{{Ch: '👨', Combining: []rune{
			rune(8205),
			rune(128103),
			rune(8205),
			rune(128102),
		}, Bytes: 18, Width: 2}},
	}, c.RawCells())
}

func TestRawCellsDelete(t *testing.T) {
	const baseRawCells = `
syntax = "proto2";
package rpc;


  // what's up`

	const expectedRawCellsCase1 = `
syntax = "proto2";
package rpc;


  `

	const expectedRawCellsCase2 = `
syntax  "proto2";
package rpc;


  // what's up`
	const expectedRawCellsCase3 = `
syntax = "proto2";
package rpc;

/ what's up`
	const expectedRawCellsCase4 = `;
package rpc;


  // what's up`

	const inputRawCellsCase5 = `Love in your heart wasn't put there to stay.

Love isn't love 'til you give it away.
		-- Oscar Hammerstein 中国`

	tsuite := []struct {
		expectedStr              string
		expectedRawCells         string
		expectedRawCellsRawCells [][]term.Cell // optional;
		inputFrom, inputTo       term.Coordinates
		overrideBaseRawCells     string //optional; otherwise baseRawCells is used
		//optional; otherwise inputFrom and inputTo is assumed to be returned
		expectedStart, expectedEnd *term.Coordinates
	}{
		{
			overrideBaseRawCells: "a",
			expectedStr:          "a",
			expectedRawCells:     "",
			inputFrom:            term.Coordinates{X: 0},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			overrideBaseRawCells: "a",
			expectedStr:          "a",
			expectedRawCells:     "",
			inputFrom:            term.Coordinates{X: 0},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			overrideBaseRawCells: "a\nb",
			expectedStr:          "a",
			expectedRawCells:     "\nb",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			overrideBaseRawCells: "a\nb",
			expectedStr:          "a\n",
			expectedRawCells:     "b",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 1},
		},
		{
			expectedStr:      "// what's up",
			expectedRawCells: expectedRawCellsCase1,
			inputFrom:        term.Coordinates{X: 2, Y: 5},
			inputTo:          term.Coordinates{X: 2 + len("// what's up"), Y: 5},
		},
		{
			expectedStr:      "=",
			expectedRawCells: expectedRawCellsCase2,
			inputFrom:        term.Coordinates{X: 7, Y: 1},
			inputTo:          term.Coordinates{X: 8, Y: 1},
		},
		{
			overrideBaseRawCells: "a\nbc",
			expectedStr:          "a\nb",
			expectedRawCells:     "c",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1, Y: 1},
		},
		{
			overrideBaseRawCells: "aa\nbb",
			expectedStr:          "a\nb",
			expectedRawCells:     "ab",
			inputFrom:            term.Coordinates{X: 1},
			inputTo:              term.Coordinates{X: 1, Y: 1},
		},
		{ // 8
			overrideBaseRawCells: "a\nbc",
			expectedStr:          "a\nbc",
			expectedRawCells:     "",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 2},
		},
		{
			overrideBaseRawCells: "a\nb\nc",
			expectedStr:          "a\nb\n",
			expectedRawCells:     "c",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 2},
		},
		{
			overrideBaseRawCells: "a",
			expectedStr:          "a",
			expectedRawCells:     "",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			// inverted from/until
			expectedStr:      baseRawCells,
			expectedRawCells: "",
			inputFrom:        term.Coordinates{Y: 6},
			inputTo:          term.Coordinates{},
			expectedStart:    &term.Coordinates{},
			expectedEnd:      &term.Coordinates{},
		},
		{
			expectedStr:      baseRawCells,
			expectedRawCells: "",
			inputFrom:        term.Coordinates{},
			inputTo:          term.Coordinates{X: 14, Y: 5},
		},
		{
			expectedStr:      "\nsyntax = \"proto2\"",
			expectedRawCells: expectedRawCellsCase4,
			inputFrom:        term.Coordinates{},
			inputTo:          term.Coordinates{X: 17, Y: 1},
		},
		{
			expectedStr:      "\n  /",
			expectedRawCells: expectedRawCellsCase3,
			inputFrom:        term.Coordinates{Y: 4},
			inputTo:          term.Coordinates{X: 3, Y: 5},
		},
		{
			overrideBaseRawCells: "a\tb",
			expectedStr:          "a",
			expectedRawCells:     "\tb",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			overrideBaseRawCells: "a\tb",
			expectedStr:          "a\t",
			expectedRawCells:     "b",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 2},
		},
		{
			overrideBaseRawCells: "a\tb",
			expectedStr:          "a\t",
			expectedRawCells:     "b",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 2},
		},
		{
			overrideBaseRawCells: "aa\tb",
			expectedStr:          "\t",
			expectedRawCells:     "aab",
			inputFrom:            term.Coordinates{X: 2},
			inputTo:              term.Coordinates{X: 3},
			expectedStart:        &term.Coordinates{X: 2},
			expectedEnd:          &term.Coordinates{X: 2},
		},
		{
			overrideBaseRawCells: "a\n\tb",
			expectedStr:          "a\n\t",
			expectedRawCells:     "b",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 1, X: 1},
		},
		{
			overrideBaseRawCells: "a\n\tb\n\tc",
			expectedStr:          "\tb\n\t",
			expectedRawCells:     "a\nc",
			inputFrom:            term.Coordinates{Y: 1, X: 0},
			inputTo:              term.Coordinates{Y: 2, X: 1},
			expectedStart:        &term.Coordinates{Y: 1, X: 0},
			expectedEnd:          &term.Coordinates{Y: 1, X: 0},
		},
		{
			overrideBaseRawCells: "\t\t\ta",
			expectedStr:          "\t",
			expectedRawCells:     "\t\ta",
			inputFrom:            term.Coordinates{X: 1},
			inputTo:              term.Coordinates{X: 2},
			expectedStart:        &term.Coordinates{X: 1},
			expectedEnd:          &term.Coordinates{X: 1},
		},
		{
			overrideBaseRawCells: "a\nb\n\nc\n\n\nd",
			expectedStr:          "a\nb\n\nc\n",
			expectedRawCells:     "\n\nd",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 4},
		},
		{
			overrideBaseRawCells: "a\nbb\nccc",
			expectedStr:          "\nbb",
			expectedRawCells:     "a\nccc",
			inputFrom:            term.Coordinates{X: 2, Y: 1},
			inputTo:              term.Coordinates{X: 1},
			expectedStart:        &term.Coordinates{X: 1},
			expectedEnd:          &term.Coordinates{X: 1},
		},
		{
			overrideBaseRawCells: "\t\n",
			expectedStr:          "\t\n",
			expectedRawCells:     "",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 1},
		},
		{
			overrideBaseRawCells: "a\n",
			expectedStr:          "\n",
			expectedRawCells:     "a",
			inputFrom:            term.Coordinates{X: 1},
			inputTo:              term.Coordinates{Y: 1},
		},
		{
			overrideBaseRawCells: "a\nb\nc",
			expectedStr:          "b\n",
			expectedRawCells:     "a\nc",
			inputFrom:            term.Coordinates{Y: 1},
			inputTo:              term.Coordinates{Y: 2},
		},
		{
			overrideBaseRawCells: "a\n\n",
			expectedStr:          "\n",
			expectedRawCells:     "a\n",
			inputFrom:            term.Coordinates{Y: 1},
			inputTo:              term.Coordinates{Y: 3},
		},
		{
			overrideBaseRawCells: "a\n\n",
			expectedStr:          "\n",
			expectedRawCells:     "a\n",
			inputFrom:            term.Coordinates{Y: 1},
			inputTo:              term.Coordinates{Y: 2},
		},
		{
			overrideBaseRawCells:     "a\n",
			expectedStr:              "a\n",
			expectedRawCells:         "",
			expectedRawCellsRawCells: [][]term.Cell{{}},
			inputFrom:                term.Coordinates{},
			inputTo:                  term.Coordinates{Y: 1},
		},
		{
			// emulate truncateFrom with a -1 rows view
			overrideBaseRawCells: "a\n\n",
			expectedStr:          "a\n",
			expectedRawCells:     "\n",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{Y: 1, X: 0},
		},
		{
			// mult-width cells, delete end falls on padding
			overrideBaseRawCells: "💥",
			expectedStr:          "💥",
			expectedRawCells:     "",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			// mult-width
			overrideBaseRawCells: "💥 hello world",
			expectedStr:          "💥",
			expectedRawCells:     " hello world",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			// mult-width cells, delete falls on padding, >1 string
			overrideBaseRawCells: "💥 hello world",
			expectedStr:          "💥",
			expectedRawCells:     " hello world",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
		{
			// mult-width cells 3
			overrideBaseRawCells: "💥 hello world",
			expectedStr:          "💥 hello ",
			expectedRawCells:     "world",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 8},
		},
		{
			// two >1 width runes
			overrideBaseRawCells: "💥🚀",
			expectedStr:          "💥",
			expectedRawCells:     "🚀",
			inputFrom:            term.Coordinates{},
			inputTo:              term.Coordinates{X: 1},
		},
	}

	for i, tcase := range tsuite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			var c rawCells
			c.init()
			base := baseRawCells
			if tcase.overrideBaseRawCells != "" {
				base = tcase.overrideBaseRawCells
			}

			_, err := c.ReadFrom(strings.NewReader(base))
			require.NoError(t, err)

			actualStart, actualEnd, actualStr := c.Edit(context.Background(), tcase.inputFrom, tcase.inputTo, "")
			assert.Equal(t, tcase.expectedStr, actualStr,
				"expected return string")
			assert.Equal(t, tcase.expectedRawCells, c.String(),
				"expected resulting cells")

			if tcase.expectedStart == nil {
				tcase.expectedStart = &tcase.inputFrom
			}
			if tcase.expectedEnd == nil {
				tcase.expectedEnd = &tcase.inputFrom
			}
			assert.Equal(t, *tcase.expectedStart, actualStart)
			assert.Equal(t, *tcase.expectedEnd, actualEnd)

			from, to, old := c.Edit(context.Background(), actualStart, actualEnd, actualStr)
			assert.Equal(t, base, c.String())

			from, to, old = c.Edit(context.Background(), from, to, old)
			assert.Equal(t, tcase.expectedRawCells, c.String())
			if tcase.expectedRawCellsRawCells != nil {
				assert.Equal(t, tcase.expectedRawCellsRawCells, c.RawCells())
			}

			_, _, old = c.Edit(context.Background(), from, to, old)
			assert.Equal(t, base, c.String())
		})
	}
}

func TestRawCellsCell(t *testing.T) {
	var c rawCells
	c.init()
	c.ReadFrom(strings.NewReader(benchmarkFortune))

	cell, ok := c.Cell(term.Coordinates{})
	assert.False(t, ok)

	cell, ok = c.Cell(term.Coordinates{X: 1})
	assert.False(t, ok)

	cell, ok = c.Cell(term.Coordinates{Y: 1, X: 4})
	assert.True(t, ok)
	assert.Equal(t, term.Cell{Ch: 'L', Bytes: 1, Combining: []rune{}, Width: 1}, cell)

	cell, ok = c.Cell(term.Coordinates{Y: 3, X: 25})
	assert.True(t, ok)
	assert.Equal(t, term.Cell{Ch: '中', Combining: []rune{}, Bytes: 3, Width: 2}, cell)

	cell, ok = c.Cell(term.Coordinates{Y: 666})
	assert.False(t, ok)
}

func TestRawCellsEditSymmetryBug(t *testing.T) {
	b := newBufferWithContent(t, longStr)
	from := term.Coordinates{X: 1, Y: 2}
	to := term.Coordinates{X: from.X + 1, Y: from.Y}
	from, to, ok := fromToInBounds(b.cells, from, to)
	require.True(t, ok)
	start, end, str := b.editor.Edit(context.Background(), from, to, "")
	require.NotZero(t, str)
	assert.Equal(t, term.Coordinates{X: 1, Y: 2}, start)
	assert.Equal(t, term.Coordinates{X: 1, Y: 2}, end)

	expectedStr := `Love in your heart wasn't put there to stay.
Love isn't love 'til you give it away.
	-- Oscar Hammerstein 中国`
	assert.Equal(t, expectedStr, b.String())

	from, to, old := b.editor.Edit(context.Background(), start, start, str)
	assert.Zero(t, old)
	assert.Equal(t, term.Coordinates{X: 1, Y: 2}, start)
	assert.Equal(t, term.Coordinates{X: 1, Y: 2}, end)
	assert.Equal(t, longStr, b.String())
}

func TestRawCellsFillBufferNoLine(t *testing.T) {
	const N = 4096 * 256
	var builder strings.Builder
	for i := 0; i < N; i++ {
		builder.WriteByte(byte(i))
	}
	str := builder.String()

	var cells rawCells
	cells.init()

	n, err := cells.ReadFrom(strings.NewReader(str))
	require.NoError(t, err)
	assert.Equal(t, int64(N), n)
}

func newBenchmarkRawCells(fortunes int) (*rawCells, string) {
	cells := new(rawCells)
	cells.init()
	payload := ""
	for i := 0; i < fortunes; i++ {
		payload = payload + benchmarkFortune
	}
	return cells, payload
}

func benchmarkBufferReadFrom(b *testing.B, fortunes int) {
	cells, payload := newBenchmarkRawCells(fortunes)
	reader := strings.NewReader(payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cells.reset()
		reader.Reset(payload)
		_, _ = cells.ReadFrom(reader)
	}
}

var benchmarkFortune = `
				Love in your heart wasn't put there to stay.
				Love isn't love 'til you give it away.
				-- Oscar Hammerstein 中国
`

// NOTE: names starting with 'Buffer' are kept so we can
// compare to when ReadFrom was implemented in Buffer.
func BenchmarkBufferReadFrom10(b *testing.B) {
	benchmarkBufferReadFrom(b, 10)
}
func BenchmarkBufferReadFrom100(b *testing.B) {
	benchmarkBufferReadFrom(b, 100)
}
func BenchmarkBufferReadFrom1000(b *testing.B) {
	benchmarkBufferReadFrom(b, 1000)
}
func BenchmarkBufferReadFrom10000(b *testing.B) {
	benchmarkBufferReadFrom(b, 10000)
}

// func BenchmarkBufferReadFrom100MB(b *testing.B) {
// 	benchmarkBufferReadFrom(b, 1000000)
// }
