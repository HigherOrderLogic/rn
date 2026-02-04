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
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/workspace"
)

func TestCursorHiddenLines(t *testing.T) {
	t.Run("move up and down across hidden lines", func(t *testing.T) {
		c := setupCursor(t, 10, 10, false)
		require.True(t, c.scroll.MarkHidden(1, 3))

		assert.True(t, c.MoveDown())
		assert.True(t, c.MoveDown())
		assert.True(t, c.MoveDown())
		assert.True(t, c.MoveDown())
		assert.True(t, c.MoveUp())
		assert.True(t, c.MoveUp())
		assert.True(t, c.MoveUp())
		assert.True(t, c.MoveUp())
	})
	t.Run("insert below hidden lines block, inserts below last hidden line", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "a\nb\nc\nd\ne", false)
		require.True(t, c.scroll.MarkHidden(1, 3))

		assert.True(t, c.MoveDown())
		c.InsertLineBelow()
		assert.Equal(t, "a\nb\nc\nd\n\ne", c.buffer().String())
	})
}

func TestCursorStartEndWord(t *testing.T) {
	suite := []struct {
		setCursorAtScroll       term.Coordinates
		expectStartWord         bool
		expectEndWord           bool
		rightInclusiveSemantics bool
	}{
		{term.Coordinates{}, false, false, true},
		{term.Coordinates{Y: 1}, false, false, true},
		{term.Coordinates{Y: 1, X: 1}, false, false, true},
		{term.Coordinates{Y: 2, X: 3}, true, false, true},
		{term.Coordinates{Y: 2, X: 4}, false, true, true},
		{term.Coordinates{Y: 2, X: 75}, true, false, true},
		{term.Coordinates{Y: 2, X: 76}, false, true, true},
		{term.Coordinates{Y: 5, X: 1}, true, false, true},
		{term.Coordinates{Y: 5, X: 0}, false, false, true},
		{term.Coordinates{Y: 18, X: 11}, true, false, true},
		{term.Coordinates{Y: 18, X: 14}, false, true, true},
		{term.Coordinates{Y: 18, X: 18}, false, false, true},
		{term.Coordinates{Y: 9, X: 6}, true, true, true},
		{term.Coordinates{}, false, false, false},
		{term.Coordinates{Y: 1, X: 1}, false, false, false},
		{term.Coordinates{Y: 1, X: 2}, false, false, false},
		{term.Coordinates{Y: 2, X: 3}, true, false, false},
		{term.Coordinates{Y: 2, X: 4}, false, false, false},
		{term.Coordinates{Y: 2, X: 5}, false, true, false},
		{term.Coordinates{Y: 2, X: 75}, true, false, false},
		{term.Coordinates{Y: 2, X: 76}, false, false, false},
		{term.Coordinates{Y: 2, X: 77}, false, true, false},
		{term.Coordinates{Y: 5, X: 1}, true, false, false},
		{term.Coordinates{Y: 5, X: 0}, false, false, false},
		{term.Coordinates{Y: 18, X: 11}, true, false, false},
		{term.Coordinates{Y: 18, X: 17}, false, false, false},
		{term.Coordinates{Y: 18, X: 15}, false, true, false},
		{term.Coordinates{Y: 9, X: 6}, true, false, false},
		{term.Coordinates{Y: 9, X: 7}, false, true, false},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursor(t, 10, 10, false)
			c.RightInclusiveSemantics = test.rightInclusiveSemantics

			c.MoveToScroll(test.setCursorAtScroll)
			actualStartWord := c.IsStartWord()
			actualEndWord := c.IsEndWord()

			word := c.Word()
			assert.Equal(t, test.expectStartWord, actualStartWord, "IsStartWord is incorrect: %q", word)
			assert.Equal(t, test.expectEndWord, actualEndWord, "IsEndWord is incorrect: %q", word)
		})
	}
}

func TestCursorUpperLowercase(t *testing.T) {
	suite := []struct {
		from, to     term.Coordinates
		inputBuffer  string
		outputBuffer string
		op           func(t *testing.T, c *Cursor)
	}{
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{},
			inputBuffer:  "a",
			outputBuffer: "A",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
			},
		},
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{},
			inputBuffer:  "\ta",
			outputBuffer: "\ta",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
			},
		},
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{X: 4},
			inputBuffer:  "\ta",
			outputBuffer: "\tA",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
			},
		},
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{X: 4},
			inputBuffer:  "\tA",
			outputBuffer: "\ta",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.LowercaseSelection())
			},
		},
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{X: 8},
			inputBuffer:  "\t\ta",
			outputBuffer: "\t\tA",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
			},
		},
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{Y: 2, X: 1},
			inputBuffer:  "\t\n\ta\nb",
			outputBuffer: "\t\n\tA\nB",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
			},
		},
		{
			from:         term.Coordinates{},
			to:           term.Coordinates{X: 1},
			inputBuffer:  "\x00a",
			outputBuffer: "\x00A",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
			},
		},
		{
			from:         term.Coordinates{Y: 1, X: 2},
			to:           term.Coordinates{Y: 3, X: 1},
			inputBuffer:  "func\n\ta word something else\nb\nhello",
			outputBuffer: "func\n\ta WORD SOMETHING ELSE\nB\nHEllo",
			op: func(t *testing.T, c *Cursor) {
				assert.True(t, c.UppercaseSelection())
				assert.True(t, c.Undo())
				assert.True(t, c.Redo())
			},
		},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursorContent(t, 10, 10, test.inputBuffer, false)
			c.RightInclusiveSemantics = true

			c.MoveToScroll(test.from)
			c.Select()
			c.MoveToScroll(test.to)

			// sut
			test.op(t, c)

			assert.Equal(t, test.outputBuffer, term.CellsToString(c.view().RawCells()))
		})
	}
}

func TestIndent(t *testing.T) {
	suite := []struct {
		inputBuffer          string
		expectIndentationAt  int
		expectIndent         bool
		cursorAtScroll       term.Coordinates
		outputBuffer         string
		expectCursorAtScroll term.Coordinates
	}{
		{
			inputBuffer:         "",
			expectIndentationAt: 0,
			expectIndent:        false,
			cursorAtScroll:      term.Coordinates{},
			outputBuffer:        "",
		},
		{
			inputBuffer:          "a",
			expectIndentationAt:  1,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{},
			outputBuffer:         "\ta",
			expectCursorAtScroll: term.Coordinates{X: 1},
		},
		{
			inputBuffer:         "\ta",
			expectIndentationAt: 1,
			expectIndent:        false,
			cursorAtScroll:      term.Coordinates{},
			outputBuffer:        "\ta",
		},
		{
			inputBuffer:          "\ta",
			expectIndentationAt:  2,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 1},
			outputBuffer:         "\t\ta",
			expectCursorAtScroll: term.Coordinates{X: 2},
		},
		{
			inputBuffer:          "\ta",
			expectIndentationAt:  0,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 1},
			outputBuffer:         "a",
			expectCursorAtScroll: term.Coordinates{},
		},
		{
			inputBuffer:          "\t\ta",
			expectIndentationAt:  0,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 2},
			outputBuffer:         "a",
			expectCursorAtScroll: term.Coordinates{},
		},
		{
			inputBuffer:         "a\ta",
			expectIndentationAt: 0,
			expectIndent:        false,
			cursorAtScroll:      term.Coordinates{X: 2},
			outputBuffer:        "a\ta",
		},
		{
			inputBuffer:          "a\ta",
			expectIndentationAt:  1,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 2},
			outputBuffer:         "\ta\ta",
			expectCursorAtScroll: term.Coordinates{X: 3},
		},
		{
			inputBuffer:          "\t\ta\ta",
			expectIndentationAt:  1,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 2},
			outputBuffer:         "\ta\ta",
			expectCursorAtScroll: term.Coordinates{X: 1},
		},
		{
			inputBuffer:          "\taX",
			expectIndentationAt:  2,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 2},
			outputBuffer:         "\t\taX",
			expectCursorAtScroll: term.Coordinates{X: 3},
		},
		{
			inputBuffer:          "\t\taX",
			expectIndentationAt:  1,
			expectIndent:         true,
			cursorAtScroll:       term.Coordinates{X: 3},
			outputBuffer:         "\taX",
			expectCursorAtScroll: term.Coordinates{X: 2},
		},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursorContent(t, 10, 10, test.inputBuffer, false)
			mock := &mockIndentService{returnIndentationAt: test.expectIndentationAt}
			mock.View = c.buffer().WithView(mock)
			c.MoveToScroll(test.cursorAtScroll)
			assert.Equal(t, test.expectIndent, c.TryIndent())
			assert.Equal(t, test.outputBuffer, term.CellsToString(c.view().RawCells()))
			if test.expectIndent {
				assert.Equal(t, test.expectCursorAtScroll, c.CursorAtScroll())
			}
		})
	}
}

func TestCursorCenter(t *testing.T) {
	suite := []struct {
		width, height             int
		setCursorAtScroll         term.Coordinates
		expectedHandled           bool
		expectedWindowCoordinates term.Coordinates
	}{
		{10, 10, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 2}, false, term.Coordinates{Y: 2}},
		{10, 10, term.Coordinates{Y: 5}, false, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 6}, true, term.Coordinates{Y: 5}},
		{10, 10, term.Coordinates{Y: 7}, true, term.Coordinates{Y: 5}},
		{10, 10, term.Coordinates{Y: 8}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 9}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 10}, true, term.Coordinates{Y: 5}},
		{10, 10, term.Coordinates{Y: 20}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 22}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 27}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 28}, true, term.Coordinates{X: 3, Y: 6}},
		{10, 10, term.Coordinates{Y: 30}, true, term.Coordinates{X: 3, Y: 8}},
		{10, 10, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 9}},
		{100, 100, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{100, 100, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 31}},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursor(t, test.width, test.height, false)
			sub := &testScrollSubscriber{}
			c.scroll.Subscribe(sub)

			c.MoveToScroll(test.setCursorAtScroll)
			handled := c.Center()
			require.Equal(t, test.expectedHandled, handled)

			cursor := c.CursorAtScroll()
			assert.Equal(t, test.setCursorAtScroll, cursor)
			assert.Equal(t, test.expectedWindowCoordinates, c.Coordinates())
			if test.expectedHandled {
				assert.NotZero(t, sub.seek)
			}
		})
	}
}

func TestCursorRepositionTop(t *testing.T) {
	suite := []struct {
		width, height             int
		setCursorAtScroll         term.Coordinates
		expectedHandled           bool
		expectedWindowCoordinates term.Coordinates
	}{
		{10, 10, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 2}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 5}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 6}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 7}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 8}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 9}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 10}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 20}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 22}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 24}, true, term.Coordinates{X: 3, Y: 2}},
		{10, 10, term.Coordinates{Y: 27}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 28}, true, term.Coordinates{X: 3, Y: 6}},
		{10, 10, term.Coordinates{Y: 30}, true, term.Coordinates{X: 3, Y: 8}},
		{10, 10, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 9}},
		{100, 100, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{100, 100, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 31}},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("offset test case %d", i), func(t *testing.T) {
			c := setupCursor(t, test.width, test.height, false)
			sub := &testScrollSubscriber{}
			c.scroll.Subscribe(sub)

			c.MoveToScroll(test.setCursorAtScroll)
			handled := c.RepositionTop()
			require.Equal(t, test.expectedHandled, handled)

			cursor := c.CursorAtScroll()
			assert.Equal(t, test.setCursorAtScroll, cursor)
			assert.Equal(t, test.expectedWindowCoordinates, c.Coordinates())
			if test.expectedHandled {
				assert.NotZero(t, sub.seek)
			}
		})
	}
}

func TestCursorRepositionTopInverted(t *testing.T) {
	suite := []struct {
		width, height             int
		setCursorAtScroll         term.Coordinates
		expectedHandled           bool
		expectedWindowCoordinates term.Coordinates
	}{
		{10, 10, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 2}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 5}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 6}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 7}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 8}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 9}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 10}, true, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 20}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 22}, true, term.Coordinates{X: 3, Y: 0}},
		{10, 10, term.Coordinates{Y: 24}, true, term.Coordinates{X: 3, Y: 2}},
		{10, 10, term.Coordinates{Y: 27}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 28}, true, term.Coordinates{X: 3, Y: 6}},
		{10, 10, term.Coordinates{Y: 30}, true, term.Coordinates{X: 3, Y: 8}},
		{10, 10, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 9}},
		{100, 100, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 68}},
		{100, 100, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 99}},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursor(t, test.width, test.height, false)
			c.scroll.InvertOffset = true
			sub := &testScrollSubscriber{}
			c.scroll.Subscribe(sub)

			// otherwise it's already at the bottom after MoveToScroll
			// since the cursor starts at the top after initializing it
			c.MoveFirstLine()
			c.MoveToScroll(test.setCursorAtScroll)
			handled := c.RepositionTop()
			require.Equal(t, test.expectedHandled, handled)

			cursor := c.CursorAtScroll()
			assert.Equal(t, test.setCursorAtScroll, cursor)
			assert.Equal(t, test.expectedWindowCoordinates, c.Coordinates())
			if test.expectedHandled {
				assert.NotZero(t, sub.seek)
			}
		})
	}
}

func TestCursorRepositionBottom(t *testing.T) {
	suite := []struct {
		width, height             int
		setCursorAtScroll         term.Coordinates
		expectedHandled           bool
		expectedWindowCoordinates term.Coordinates
	}{
		{10, 10, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 2}, true, term.Coordinates{Y: 2}},
		{10, 10, term.Coordinates{Y: 5}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 6}, true, term.Coordinates{Y: 6}},
		{10, 10, term.Coordinates{Y: 9}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 10}, true, term.Coordinates{Y: 9}},
		{10, 10, term.Coordinates{Y: 11}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 20}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 22}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 27}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 28}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 30}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 9}},
		{100, 100, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{100, 100, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 31}},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursor(t, test.width, test.height, false)
			sub := &testScrollSubscriber{}
			c.scroll.Subscribe(sub)

			// otherwise it's already at the bottom after MoveToScroll
			// since the cursor starts at the top after initializing it
			c.MoveLastLine()
			c.MoveToScroll(test.setCursorAtScroll)
			handled := c.RepositionBottom()
			require.Equal(t, test.expectedHandled, handled)

			cursor := c.CursorAtScroll()
			assert.Equal(t, test.setCursorAtScroll, cursor)
			assert.Equal(t, test.expectedWindowCoordinates, c.Coordinates())
			if test.expectedHandled {
				assert.NotZero(t, sub.seek)
			}
		})
	}
}

func TestCursorRepositionBottomInverted(t *testing.T) {
	suite := []struct {
		width, height             int
		setCursorAtScroll         term.Coordinates
		expectedHandled           bool
		expectedWindowCoordinates term.Coordinates
	}{
		{10, 10, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 0}},
		{10, 10, term.Coordinates{Y: 2}, true, term.Coordinates{Y: 2}},
		{10, 10, term.Coordinates{Y: 5}, true, term.Coordinates{X: 3, Y: 5}},
		{10, 10, term.Coordinates{Y: 6}, true, term.Coordinates{Y: 6}},
		{10, 10, term.Coordinates{Y: 9}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 10}, true, term.Coordinates{Y: 9}},
		{10, 10, term.Coordinates{Y: 11}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 20}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 22}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 27}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 28}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 30}, true, term.Coordinates{X: 3, Y: 9}},
		{10, 10, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 9}},
		{100, 100, term.Coordinates{Y: 0}, false, term.Coordinates{Y: 68}},
		{100, 100, term.Coordinates{Y: 31}, false, term.Coordinates{Y: 99}},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			c := setupCursor(t, test.width, test.height, false)
			c.scroll.InvertOffset = true
			sub := &testScrollSubscriber{}
			c.scroll.Subscribe(sub)

			c.MoveToScroll(test.setCursorAtScroll)
			handled := c.RepositionBottom()
			require.Equal(t, test.expectedHandled, handled)

			cursor := c.CursorAtScroll()
			assert.Equal(t, test.setCursorAtScroll, cursor)
			assert.Equal(t, test.expectedWindowCoordinates, c.Coordinates())
			if test.expectedHandled {
				assert.NotZero(t, sub.seek)
			}
		})
	}
}

func TestCursorFolds(t *testing.T) {
	ctx := context.Background()
	tsuite := []struct {
		desc            string
		op              func(t *testing.T, c *Cursor, wg *sync.WaitGroup)
		expectOnHide    int
		expectOnVisible int
		cursor          term.Coordinates
		width, height   int
	}{
		{
			"SelectFold selects nothing if cursor is not at fold",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.SelectFold(ctx))
				assert.Zero(t, c.Selection())
			}, 0, 0, term.Coordinates{}, 100, 100,
		},
		{
			"SelectFold selects fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.SelectFold(ctx))
				wg.Wait()
				expectedSelection := `/*
 * Ch@ek if the current buffer should be added to or removed from the list of
 * diff buffers.
 `
				assert.Equal(t, expectedSelection, c.Selection())
				assert.True(t, c.DeleteSelection())
			}, 0, 0, term.Coordinates{Y: 1}, 100, 100,
		},
		{
			"CollapseFold collapses nothing if cursor is not at fold",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.CollapseFold(ctx))
			}, 0, 0, term.Coordinates{}, 100, 100,
		},
		{
			"CollapseFold collapses fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.CollapseFold(ctx))
			}, 1, 0, term.Coordinates{Y: 1}, 100, 100,
		},
		{
			"CollapseFold collapses inner fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.MoveDown())
				assert.True(t, c.CollapseFold(ctx))
			}, 1, 0, term.Coordinates{Y: 2}, 100, 100,
		},
		{
			"ToggleFold toggles fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.ToggleFold(ctx))
			}, 1, 0, term.Coordinates{Y: 1}, 100, 100,
		},
		{
			"ToggleFold toggles inner fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.MoveDown())
				assert.True(t, c.ToggleFold(ctx))
			}, 1, 0, term.Coordinates{Y: 2}, 100, 100,
		},
		{
			"ExpandFold does nothing if there's no folded fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.ExpandFold(ctx))
			}, 0, 0, term.Coordinates{Y: 1}, 100, 100,
		},
		{
			"ExpandFold expands fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.CollapseFold(ctx))
				wg.Wait()
				assert.True(t, c.ExpandFold(ctx))
			}, 1, 1, term.Coordinates{Y: 1}, 100, 100,
		},
		{
			"ExpandFold expands inner fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.MoveDown())
				assert.True(t, c.CollapseFold(ctx))
				wg.Wait()
				assert.True(t, c.ExpandFold(ctx))
			}, 1, 1, term.Coordinates{Y: 2}, 100, 100,
		},
		{
			"ToggleFold expands folded fold at cursor",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				assert.True(t, c.MoveDown())
				assert.True(t, c.ToggleFold(ctx))
				wg.Wait()
				assert.True(t, c.ToggleFold(ctx))
			}, 1, 1, term.Coordinates{Y: 1}, 100, 100,
		},
		{
			"ToggleAllFolds collapses all outer folds",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.ToggleAllFolds(ctx))
			}, 2, 0, term.Coordinates{Y: 4}, 100, 100,
		},
		{
			"ToggleAllFolds expands all folded folds",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.ToggleAllFolds(ctx))
				wg.Wait()
				wg.Add(1)
				assert.True(t, c.ToggleAllFolds(ctx))
			}, 2, 2, term.Coordinates{Y: 7}, 100, 100,
		},
		{
			"CollapseAllFolds collapses all outer folds",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.CollapseAllFolds(ctx))
			}, 2, 0, term.Coordinates{Y: 4}, 100, 100,
		},
		{
			"ExpandAllFolds expands nothing if nothing is folded",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.ExpandAllFolds(ctx))
			}, 0, 0, term.Coordinates{Y: 7}, 100, 100,
		},
		{
			"ToggleAllFolds collapses all outer folds, maintains window position",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.ToggleAllFolds(ctx))
			}, 2, 0, term.Coordinates{Y: 4}, 100, 5,
		},
		{
			"ToggleAllFolds expands all folded folds, maintains window position",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.ToggleAllFolds(ctx))
				wg.Wait()
				wg.Add(1)
				assert.True(t, c.ToggleAllFolds(ctx))
			}, 2, 2, term.Coordinates{Y: 4}, 100, 5,
		},
		{
			"CollapseAllFolds collapses all outer folds, maintains window position",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.CollapseAllFolds(ctx))
			}, 2, 0, term.Coordinates{Y: 4}, 100, 5,
		},
		{
			"ExpandAllFolds expands nothing if nothing is folde, maintains window positiond",
			func(t *testing.T, c *Cursor, wg *sync.WaitGroup) {
				wg.Add(1)
				_, ok := c.MoveToScroll(term.Coordinates{Y: 7})
				require.True(t, ok)
				assert.True(t, c.ExpandAllFolds(ctx))
			}, 0, 0, term.Coordinates{Y: 4}, 100, 5,
		},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase

		t.Run(tcase.desc, func(t *testing.T) {
			var wg sync.WaitGroup
			e := setupCursorForFolds(t, tcase.width, tcase.height, &wg)
			sub := &testScrollSubscriber{}
			e.scroll.Subscribe(sub)

			tcase.op(t, e, &wg)

			wg.Wait()
			cursor := e.Coordinates()
			assert.Equal(t, tcase.cursor, cursor)
			assert.Equal(t, tcase.expectOnHide, sub.hide)
			assert.Equal(t, tcase.expectOnVisible, sub.visible)
		})
	}

	t.Run("clears results if search text is empty", func(t *testing.T) {
		e := setupCursor(t, 100, 100, false)

		require.Equal(t, 2, e.Search("NULL"))
		e.MoveToNextMatch()

		cursor := e.Coordinates()
		assert.Equal(t, term.Coordinates{X: 14, Y: 18}, cursor)

		require.Equal(t, 0, e.Search(""))
		e.MoveToNextMatch()

		cursor = e.Coordinates()
		assert.Equal(t, term.Coordinates{X: 14, Y: 18}, cursor)
	})
}

func TestCursorSearch(t *testing.T) {
	tsuite := []struct {
		desc          string
		width, height int
		results       int
		searchstring  string
		assertions    func(*testing.T, *Cursor)
		cursor        term.Coordinates
	}{
		{
			"does nothing if search text is not found",
			1000, 1000,
			0,
			"nothing",
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveToNextMatch())
			}, term.Coordinates{},
		},
		{
			"moves to the first result if search text is found",
			1000, 1000,
			2,
			"NULL",
			nil, term.Coordinates{X: 14, Y: 18},
		},
		{
			"tolerates inserts to buffer by updating locations",
			1000, 1000,
			2,
			"NULL",
			func(t *testing.T, e *Cursor) {
				var at term.Coordinates
				e.buffer().Edit(context.Background(), at, at, "\n")
				assert.True(t, e.MoveToNextMatch())
			},
			term.Coordinates{X: 14, Y: 19},
		},
		{
			"MoveToNextMatch does nothing if only one result is found",
			1000, 1000,
			1,
			"else",
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 4, Y: 29},
		},
		{
			"Seeks to first result if not in window",
			100, 10,
			1,
			"When",
			nil,
			term.Coordinates{X: 7, Y: 9},
		},
		{
			"Seeks to last result upon MoveToPrevMatch",
			1000, 1000,
			2,
			"NULL",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToPrevMatch())
			}, term.Coordinates{X: 32, Y: 23},
		},
		{
			"Seeks if MoveToNextMatch result is not in window",
			100, 10,
			2,
			"NULL",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 32, Y: 9},
		},
		{
			"returns partial word results",
			100, 10,
			2,
			"NU",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 32, Y: 9},
		},
		{
			"returns partial and complete word results",
			100, 10,
			38,
			"i",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 71, Y: 2},
		},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase

		t.Run(tcase.desc, func(t *testing.T) {
			e := setupCursor(t, tcase.width, tcase.height, false)

			require.Equal(t, tcase.results, e.Search(tcase.searchstring))
			e.MoveToNextMatch() // backwards compat
			if tcase.assertions != nil {
				tcase.assertions(t, e)
			}

			cursor := e.Coordinates()
			assert.Equal(t, tcase.cursor, cursor)

			if tcase.results == 0 {
				return
			}

			require.True(t, e.Select())
			for i := 1; i < len(tcase.searchstring); i++ {
				e.MoveRight()
			}
			assert.Equal(t, tcase.searchstring, e.Selection())
		})
	}

	t.Run("clears results if search text is empty", func(t *testing.T) {
		e := setupCursor(t, 100, 100, false)

		require.Equal(t, 2, e.Search("NULL"))
		e.MoveToNextMatch()

		cursor := e.Coordinates()
		assert.Equal(t, term.Coordinates{X: 14, Y: 18}, cursor)

		require.Equal(t, 0, e.Search(""))
		e.MoveToNextMatch()

		cursor = e.Coordinates()
		assert.Equal(t, term.Coordinates{X: 14, Y: 18}, cursor)
	})
}

func TestCursorSearchWord(t *testing.T) {
	tsuite := []struct {
		desc          string
		width, height int
		results       int
		searchstring  string
		assertions    func(*testing.T, *Cursor)
		cursor        term.Coordinates
	}{
		{
			"does nothing if search text is not found",
			1000, 1000,
			0,
			"nothing",
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveToNextMatch())
			}, term.Coordinates{},
		},
		{
			"moves to the first result if search text is found",
			1000, 1000,
			2,
			"NULL",
			nil, term.Coordinates{X: 14, Y: 18},
		},
		{
			"tolerates inserts to buffer by updating locations",
			1000, 1000,
			2,
			"NULL",
			func(t *testing.T, e *Cursor) {
				var at term.Coordinates
				e.buffer().Edit(context.Background(), at, at, "\n")
				assert.True(t, e.MoveToNextMatch())
			},
			term.Coordinates{X: 14, Y: 19},
		},
		{
			"MoveToNextMatch does nothing if only one result is found",
			1000, 1000,
			1,
			"else",
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 4, Y: 29},
		},
		{
			"Seeks to first result if not in window",
			100, 10,
			1,
			"When",
			nil,
			term.Coordinates{X: 7, Y: 9},
		},
		{
			"Seeks to last result upon MoveToPrevMatch",
			1000, 1000,
			2,
			"NULL",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToPrevMatch())
			}, term.Coordinates{X: 32, Y: 23},
		},
		{
			"Seeks if MoveToNextMatch result is not in window",
			100, 10,
			2,
			"NULL",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 32, Y: 9},
		},
		{
			"ignores non complete words",
			100, 10,
			0,
			"NU",
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveToNextMatch())
			}, term.Coordinates{},
		},
		{
			"returns only complete word results",
			100, 10,
			4,
			"i",
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveToNextMatch())
			}, term.Coordinates{X: 8, Y: 9},
		},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase

		t.Run(tcase.desc, func(t *testing.T) {
			e := setupCursor(t, tcase.width, tcase.height, false)

			require.Equal(t, tcase.results, e.SearchWord(tcase.searchstring))
			e.MoveToNextMatch() // backwards compat
			if tcase.assertions != nil {
				tcase.assertions(t, e)
			}

			cursor := e.Coordinates()
			assert.Equal(t, tcase.cursor, cursor)

			if tcase.results == 0 {
				return
			}

			require.True(t, e.Select())
			for i := 1; i < len(tcase.searchstring); i++ {
				e.MoveRight()
			}
			assert.Equal(t, tcase.searchstring, e.Selection())
		})
	}

	t.Run("clears results if search text is empty", func(t *testing.T) {
		e := setupCursor(t, 100, 100, false)

		require.Equal(t, 2, e.SearchWord("NULL"))
		e.MoveToNextMatch()

		cursor := e.Coordinates()
		assert.Equal(t, term.Coordinates{X: 14, Y: 18}, cursor)

		require.Equal(t, 0, e.SearchWord(""))
		e.MoveToNextMatch()

		cursor = e.Coordinates()
		assert.Equal(t, term.Coordinates{X: 14, Y: 18}, cursor)
	})
}

func TestCursorMove(t *testing.T) {
	tsuite := []struct {
		desc           string
		width, height  int
		sut            func(*testing.T, *Cursor)
		cursorAtScroll term.Coordinates
	}{
		{
			"MoveStartLine should do nothing if already at start of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveStartLine())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveStartLine should move to start of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 20
				assert.True(t, e.MoveStartLine())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveStartLineNonBlank moves to first none blank character of line, starts blank",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 20
				assert.True(t, e.MoveStartLineNonBlank())
			},
			term.Coordinates{X: 2, Y: 20},
		},
		{
			"MoveStartLineNonBlank moves to first none blank character of line, doesn't start blank",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 6
				assert.True(t, e.MoveStartLineNonBlank())
			},
			term.Coordinates{X: 0, Y: 6},
		},
		{
			"MoveStartLineNonBlank moves to first none blank character of line, cursor beyond end",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 27
				e.cursor.Y = 20
				assert.True(t, e.MoveStartLineNonBlank())
			},
			term.Coordinates{X: 2, Y: 20},
		},
		{
			"MoveStartLineNonBlank empty line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 10
				assert.False(t, e.MoveStartLineNonBlank())
			},
			term.Coordinates{X: 0, Y: 10},
		},
		{
			"MoveStartLine should seek to start of line if start is out of window",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.scroll.SeekEndLine()
				e.cursor.X = 20

				assert.True(t, e.MoveStartLine())

				assert.Equal(t, 0, e.scroll.Offset().X)
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveEndLine should do nothing if already at end of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor = term.Coordinates{X: 2, Y: 1}
				assert.False(t, e.MoveEndLine())
			},
			term.Coordinates{X: 2, Y: 1},
		},
		{
			"MoveEndLine should fix position if past the end of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor = term.Coordinates{X: 3, Y: 1}
				assert.True(t, e.MoveEndLine())
			},
			term.Coordinates{X: 2, Y: 1},
		},
		{
			"MoveEndLine should move cursor to end of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 1
				assert.True(t, e.MoveEndLine())
			},
			term.Coordinates{X: 2, Y: 1},
		},
		{
			"MoveEndLine should seek to end of line if end is out of window",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2

				assert.True(t, e.MoveEndLine())
				// backwards compat
				e.MoveToBounds(0)
				if !e.scroll.Wrap {
					assert.Equal(t, 76, e.scroll.Offset().X+e.cursor.X)
				} else {
					assert.Equal(t, 6, e.scroll.Offset().X+e.cursor.X)
				}
				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 'f', c.Ch, string(c.Ch))
			},
			term.Coordinates{X: 76, Y: 2},
		},
		{
			"MoveFirstLine should do nothing if already on first line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveFirstLine())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveFirstLine should move to first line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 10
				assert.True(t, e.MoveFirstLine())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveFirstLine should seek to first line if not in window",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.scroll.SeekEndFile()
				e.cursor.Y = 2
				assert.True(t, e.MoveFirstLine())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLastLine should do nothing if already on last line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 31
				assert.False(t, e.MoveLastLine())
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveLastLine should move to first line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveLastLine())
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveLastLine should seek to last line if not in window",
			10, 10,
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveLastLine())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				if !e.scroll.Wrap {
					assert.Equal(t, 31, e.scroll.Offset().Y+e.cursor.Y)
				}
				assert.Equal(t, '}', c.Ch)
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveDown should not move the cursor position past the last line until end of window",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					// semantics past last content are different between wrap and non-wrap
					t.Skip()
				}
				e.cursor.Y = 31
				for i := 0; i < e.scroll.SizeHeight(); i++ {
					e.MoveDown()
				}
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveDown should seek down if reached last line in window but not at last line",
			100, 10, // avoid wraps in wrap mode
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 9
				assert.True(t, e.MoveDown())
				assert.Equal(t, 1, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 10},
		},
		{
			"MoveDown should NOT seek down if reached last line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 9
				e.scroll.SeekEndFile()
				offsetY := e.scroll.Offset().Y
				assert.False(t, e.MoveDown())
				assert.Equal(t, offsetY, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveDownLines with count 0 does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 3
				assert.False(t, e.MoveDownLines(0))
			},
			term.Coordinates{X: 0, Y: 3},
		},
		{
			"MoveDownLines with negative count does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 3
				assert.False(t, e.MoveDownLines(-2))
			},
			term.Coordinates{X: 0, Y: 3},
		},
		{
			"MoveDownLines with count 3 should move cursor left 3 times",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 3
				assert.True(t, e.MoveDownLines(3))
			},
			term.Coordinates{X: 0, Y: 6},
		},
		{
			"MoveDownLines should not move the cursor position past the last line until end of window",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 0
				assert.True(t, e.MoveDownLines(200)) // this places it at content last row + 1
				assert.False(t, e.MoveDownLines(200))
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveDownLines should seek down if reached last line in window but not at last line",
			1000, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 9
				assert.True(t, e.MoveDownLines(3))
				assert.Equal(t, 3, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 12},
		},
		{
			"MoveDownLines should scroll to window end and stick cursor at last content line if jumping beyond last content line",
			1000, 20,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 5
				assert.Equal(t, 0, e.scroll.Offset().Y)
				assert.True(t, e.MoveDownLines(777))
				assert.Equal(t, 12, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveUpLines with count 0 does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 3
				assert.False(t, e.MoveUpLines(0))
			},
			term.Coordinates{X: 0, Y: 3},
		},
		{
			"MoveUpLines with negative count does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 3
				assert.False(t, e.MoveUpLines(-2))
			},
			term.Coordinates{X: 0, Y: 3},
		},
		{
			"MoveUp should do nothing if already on first line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveUp())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveUp should NOT fix cursor position if negative",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					// diff treatment of cursorAtScroll
					t.SkipNow()
				}
				e.cursor.Y = -1
				assert.False(t, e.MoveUp())
			},
			term.Coordinates{X: 0, Y: -1},
		},
		{
			"MoveUp should move the cursor up one line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 10
				assert.True(t, e.MoveUp())
			},
			term.Coordinates{X: 0, Y: 9},
		},
		{
			"MoveUp should seek up if not at first line and cursor is at first line of window",
			100, 10, // avoid creating wraps in wrap mode
			func(t *testing.T, e *Cursor) {
				e.scroll.SeekEndFile()
				offsetY := e.scroll.Offset().Y
				assert.True(t, e.MoveUp())
				assert.Equal(t, offsetY-1, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 21},
		},
		{
			"MoveUp should NOT seek up if already at first line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				offsetY := e.scroll.Offset().Y
				assert.False(t, e.MoveUp())
				assert.Equal(t, offsetY, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveUpLines should NOT fix cursor position if negative",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					t.SkipNow()
				}
				e.cursor.Y = -1
				assert.False(t, e.MoveUpLines(4))
			},
			term.Coordinates{X: 0, Y: -1},
		},
		{
			"MoveUpLines should move the cursor N lines",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 10
				assert.True(t, e.MoveUpLines(5))
			},
			term.Coordinates{X: 0, Y: 5},
		},
		{
			"MoveUpLines should not go beyond the top line 0 of content",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 10
				assert.True(t, e.MoveUpLines(66))
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveUpLines should seek up if not at first content line and cursor is at first line of window",
			100, 10, // avoid creating wraps in wrap mode
			func(t *testing.T, e *Cursor) {
				e.scroll.SeekEndFile()
				offsetY := e.scroll.Offset().Y
				assert.True(t, e.MoveUpLines(4))
				assert.Equal(t, offsetY-4, e.scroll.Offset().Y)
			},
			term.Coordinates{X: 0, Y: 18},
		},
		{
			"MoveUpLines should NOT seek up if already at first line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveUpLines(8))
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeft should do nothing if already at start of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveLeft())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeft should move cursor left",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 1
				assert.True(t, e.MoveLeft())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeft does nothing if pos is negative",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = -1
				assert.False(t, e.MoveLeft())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeft should seek left if at start of window but not at start of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					t.Skip()
					return
				}
				e.scroll.SeekEndLine()
				assert.NotZero(t, e.scroll.Offset().X)

				for i := 0; i < 100; i++ {
					e.MoveLeft()
				}
				assert.Zero(t, e.scroll.Offset().X)
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeftColumns with count 0 does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 7
				assert.False(t, e.MoveLeftColumns(0))
			},
			term.Coordinates{X: 7, Y: 0},
		},
		{
			"MoveLeftColumns with negative count does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 7
				assert.False(t, e.MoveLeftColumns(-2))
			},
			term.Coordinates{X: 7, Y: 0},
		},
		{
			"MoveLeftColumns should do nothing if already at start of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveLeftColumns(4))
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeftColumns should move cursor left",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 7
				assert.True(t, e.MoveLeftColumns(4))
			},
			term.Coordinates{X: 3, Y: 2},
		},
		{
			"MoveLeftColumns fixes cursor pos if negative",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = -1
				assert.False(t, e.MoveLeftColumns(4))
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveLeftColumns should seek left if at start of window but not at start of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					t.Skip()
					return
				}
				assert.Zero(t, e.scroll.Offset().X)
				e.scroll.SeekEndLine()
				assert.NotZero(t, e.scroll.Offset().X)
				e.MoveLeftColumns(100)
				assert.Zero(t, e.scroll.Offset().X)
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveRightColumns with count 0 does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 4
				assert.False(t, e.MoveRightColumns(0))
			},
			term.Coordinates{X: 4, Y: 0},
		},
		{
			"MoveRightColumns with negative count does nothing",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 4
				assert.False(t, e.MoveRightColumns(-2))
			},
			term.Coordinates{X: 4, Y: 0},
		},
		{
			"MoveRight should move cursor right",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 5
				assert.True(t, e.MoveRight())
			},
			term.Coordinates{X: 6, Y: 2},
		},
		{
			"MoveRight should not move cursor right if past current line's end of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 4
				e.cursor.Y = 1
				assert.False(t, e.MoveRight())
			},
			term.Coordinates{X: 4, Y: 1},
		},
		{
			"MoveRight should not move cursor right if at the end of the line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.RightInclusiveSemantics = false
				e.cursor.X = 3
				e.cursor.Y = 1
				assert.False(t, e.MoveRight())
			},
			term.Coordinates{X: 3, Y: 1},
		},
		{
			"MoveRight should seek right if at end of window but not at end of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					t.Skip()
					return
				}
				e.cursor.Y = 2
				e.cursor.X = 5
				e.scroll.SeekStartLine()

				for i := 0; i < 100; i++ {
					e.MoveRight()
				}
			},
			term.Coordinates{X: 77, Y: 2},
		},
		{
			"MoveRightColumns should move cursor right",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 5
				assert.True(t, e.MoveRightColumns(3))
			},
			term.Coordinates{X: 8, Y: 2},
		},
		{
			"MoveRightColumns should not move cursor right if past current line's end of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 1
				assert.False(t, e.MoveRightColumns(8))
			},
			term.Coordinates{X: 1, Y: 0},
		},
		{
			"MoveRightColumns should not move cursor right if at the end of the line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 4
				e.cursor.Y = 1
				assert.False(t, e.MoveRightColumns(10))
			},
			term.Coordinates{X: 4, Y: 1},
		},
		{
			"MoveRight should seek right if at end of window but not at end of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					t.Skip()
					return
				}
				e.cursor.Y = 2
				e.cursor.X = 5
				e.scroll.SeekStartLine()
				e.MoveRightColumns(100)
			},
			term.Coordinates{X: 77, Y: 2},
		},
		{
			"MoveRightStartWord should move to the start of the next word",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveToScroll(term.Coordinates{X: 9, Y: 2})

				require.True(t, e.MoveRightStartWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 't', c.Ch, string(c.Ch))
			},
			term.Coordinates{X: 12, Y: 2},
		},
		{
			"MoveRightStartWord should stop stop on special symbols such as the at-symbol @",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveToScroll(term.Coordinates{X: 3, Y: 2})

				require.True(t, e.MoveRightStartWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '@', c.Ch, string(c.Ch))
			},
			term.Coordinates{X: 5, Y: 2},
		},
		{
			"MoveLeftStartWord should move to the start of the next word",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 9
				e.MoveRightStartWord()

				assert.True(t, e.MoveLeftStartWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 'i', c.Ch)
			},
			term.Coordinates{X: 9, Y: 2},
		},
		{
			"MoveLeftStartWord should stop on special symbols such as the at-symbol @",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 7
				e.MoveRightStartWord()

				assert.True(t, e.MoveLeftStartWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 'e', c.Ch)
			},
			term.Coordinates{X: 6, Y: 2},
		},
		{
			"MoveRightEndWord should move to the end of the current word",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 9

				e.MoveRightStartWord()
				e.MoveLeftStartWord()

				assert.True(t, e.MoveRightEndWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 'f', c.Ch)
			},
			term.Coordinates{X: 10, Y: 2},
		},
		{
			"MoveRightEndWord should wrap around until end of file (no wrap)",
			10, 10,
			func(t *testing.T, e *Cursor) {
				for e.MoveRightEndWord() {
				}
			},
			term.Coordinates{X: 10, Y: 31},
		},
		{
			"MoveLeftEndWord should wrap around until start of file",
			10, 10,
			func(t *testing.T, e *Cursor) {
				require.True(t, e.MoveLastLine())
				for e.MoveLeftEndWord() {
				}
			},
			term.Coordinates{Y: 1},
		},
		{
			"MoveLeftEndWord should move to end of previous word (right inclusive)",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 9
				e.cursor.Y = 2
				assert.True(t, e.MoveLeftEndWord())
			},
			term.Coordinates{X: 7, Y: 2},
		},
		{
			"MoveLeftEndWord should move to end of previous word (right exclusive)",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.RightInclusiveSemantics = false
				e.cursor.X = 9
				e.cursor.Y = 2
				assert.True(t, e.MoveLeftEndWord())
			},
			term.Coordinates{X: 8, Y: 2},
		},
		{
			"MoveLeftStartWord should wrap around until start of file",
			10, 10,
			func(t *testing.T, e *Cursor) {
				require.True(t, e.MoveLastLine())
				for e.MoveLeftStartWord() {
				}
			},
			term.Coordinates{},
		},
		{
			"MoveRightEndWord should move to the end of the current word (right exclusive)",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.RightInclusiveSemantics = false
				e.cursor.Y = 2
				e.cursor.X = 9

				e.MoveRightStartWord()
				e.MoveLeftStartWord()

				assert.True(t, e.MoveRightEndWord())
			},
			term.Coordinates{X: 11, Y: 2},
		},
		{
			"MoveRightEndWord should stop on special symbols such as the at-symbol @",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 4

				assert.True(t, e.MoveRightEndWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '@', c.Ch)
			},
			term.Coordinates{X: 5, Y: 2},
		},
		{
			"MoveRightEndWord should stop on special symbols such as the at-symbol @",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 4
				e.RightInclusiveSemantics = false

				assert.True(t, e.MoveRightEndWord())
			},
			term.Coordinates{X: 6, Y: 2},
		},
		{
			"MoveToMatchingRune should do nothing if rune is not {,[,(,},],)",
			10, 10,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveToMatchingRune())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveToMatchingRune should not seek unless necessary",
			77, 77,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 27
				e.cursor.X = 4
				assert.True(t, e.MoveToMatchingRune())
			},
			term.Coordinates{X: 1, Y: 19},
		},
		{
			"MoveToMatchingRune should move to the 'matching rune' forward",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveToScroll(term.Coordinates{Y: 7})

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				require.Equal(t, '{', c.Ch)

				require.True(t, e.MoveToMatchingRune())

				c, _ = e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '}', c.Ch)
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveToMatchingRune should move to the 'matching rune' backwards",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveToScroll(term.Coordinates{Y: 31})

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				require.Equal(t, '}', c.Ch)

				require.True(t, e.MoveToMatchingRune())

				c, _ = e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '{', c.Ch)
			},
			term.Coordinates{X: 0, Y: 7},
		},
		{
			"MoveToMatchingRune should return false if current matching rune is not found",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 31
				e.cursor.X = 5
				e.scroll.SeekEndFile()
				assert.False(t, e.MoveToMatchingRune())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '{', c.Ch)
			},
			term.Coordinates{X: 5, Y: 31},
		},
		{
			"MoveToNextChar should do nothing if there is no matches in the line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveDown()
				e.MoveRight()
				assert.False(t, e.MoveToNextChar('a'))
			},
			term.Coordinates{X: 1, Y: 1},
		},
		{
			"MoveToNextChar should do nothing if are only matches before cursor",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveDown()
				e.MoveRight()
				assert.False(t, e.MoveToNextChar('/'))
			},
			term.Coordinates{X: 1, Y: 1},
		},
		{
			"MoveToNextChar should move the cursor to a matching character",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveDown()
				e.MoveDown()
				assert.True(t, e.MoveToNextChar('o'))

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 'o', c.Ch)
			},
			term.Coordinates{X: 33, Y: 2},
		},
		{
			"MoveToNextChar should move the cursor to a matching character at the end of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				for range 3 {
					assert.True(t, e.MoveDown())
				}
				assert.True(t, e.MoveToNextChar('.'))

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '.', c.Ch)
			},
			term.Coordinates{X: 15, Y: 3},
		},
		{
			"MoveToPrevChar should do nothing if there is no matches in the line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveDown()
				e.MoveRight()
				assert.False(t, e.MoveToPrevChar('a'))
			},
			term.Coordinates{X: 1, Y: 1},
		},
		{
			"MoveToPrevChar should do nothing if are only matches after cursor",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveDown()
				assert.False(t, e.MoveToPrevChar('/'))
			},
			term.Coordinates{X: 0, Y: 1},
		},
		{
			"MoveToPrevChar should move the cursor to a matching character",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveDown()
				e.MoveDown()
				e.MoveEndLine()
				assert.True(t, e.MoveToPrevChar('C'))

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 'C', c.Ch)
			},
			term.Coordinates{X: 3, Y: 2},
		},
		{
			"MoveToScroll cursor beyond vertical content does not change coordinates",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveLastLine()
				cur := e.Coordinates()
				e.MoveToScroll(term.Coordinates{X: cur.X + 1, Y: cur.Y + 1})
			},
			term.Coordinates{X: 1, Y: 10},
		},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase

		t.Run(tcase.desc, func(t *testing.T) {
			e := setupCursor(t, tcase.width, tcase.height, false)

			tcase.sut(t, e)

			cursor := e.CursorAtScroll()
			assert.Equal(t, tcase.cursorAtScroll, cursor)
		})

		t.Run(tcase.desc+" (wrap mode on)", func(t *testing.T) {
			e := setupCursor(t, tcase.width, tcase.height, true)

			tcase.sut(t, e)

			cursor := e.CursorAtScroll()
			assert.Equal(t, tcase.cursorAtScroll, cursor)
		})
	}
}

func TestCursorMultiMovePublish(t *testing.T) {
	suite := []struct {
		initialScrollPos term.Coordinates
		name             string
		moveFn           func(*Cursor) bool
	}{
		{
			initialScrollPos: term.Coordinates{X: 0, Y: 0},
			name:             "MoveDownLines multiplied move publishes a single scroll event",
			moveFn:           func(c *Cursor) bool { return c.MoveDownLines(44) },
		},
		{
			initialScrollPos: term.Coordinates{X: 0, Y: 100},
			name:             "MoveUpLines multiplied move publishes a single scroll event",
			moveFn:           func(c *Cursor) bool { return c.MoveUpLines(20) },
		},
		{
			initialScrollPos: term.Coordinates{X: 100, Y: 2},
			name:             "MoveLeftColumns multiplied move publishes a single scroll event",
			moveFn:           func(c *Cursor) bool { return c.MoveLeftColumns(33) },
		},
		{
			initialScrollPos: term.Coordinates{X: 0, Y: 3},
			name:             "MoveRightColumns multiplied move publishes a single scroll event",
			moveFn:           func(c *Cursor) bool { return c.MoveRightColumns(333) },
		},
	}

	for _, tcase := range suite {
		t.Run(tcase.name, func(t *testing.T) {
			e := setupCursor(t, 10, 6, false)
			e.cursor = tcase.initialScrollPos
			subscriberCalls := 0
			sub := component.FuncScrollSubscriber(func(from, to term.Coordinates) {
				subscriberCalls++
			})
			e.SubscribeScroll(sub)
			tcase.moveFn(e)
			assert.Equal(t, 1, subscriberCalls)
		})
	}
}

func TestCursorInsertLine(t *testing.T) {
	suite := []struct {
		wrap bool
	}{
		{false}, {true},
	}
	for _, test := range suite {
		t.Run(fmt.Sprintf("wrap: %v", test.wrap), func(t *testing.T) {
			e := setupCursor(t, 10, 10, test.wrap)

			assert.Equal(t, 32, e.scroll.Buffer().Rows())
			assert.Equal(t, term.Coordinates{}, e.Coordinates())
			assert.Equal(t, term.Coordinates{}, e.cursorAtScroll())

			e.InsertLineAbove()
			assert.Equal(t, 0, len(e.scroll.Buffer().RawCells()[0]))
			assert.Equal(t, 33, e.scroll.Buffer().Rows())
			assert.Equal(t, term.Coordinates{}, e.Coordinates())
			assert.Equal(t, term.Coordinates{}, e.cursorAtScroll())

			e.scroll.SeekEndLine()
			e.cursor.Y = 2
			e.cursor.X = 9

			assert.Equal(t, term.Coordinates{Y: 2, X: 9}, e.Coordinates())
			e.InsertLineAbove()
			assert.Equal(t, 0, len(e.scroll.Buffer().RawCells()[2]))
			assert.Equal(t, 34, e.scroll.Buffer().Rows())
			assert.Equal(t, term.Coordinates{Y: 2, X: 0}, e.Coordinates())

			e.MoveEndLine()
			e.InsertLineBelow()
			assert.Equal(t, 35, e.scroll.Buffer().Rows())
			assert.Equal(t, term.Coordinates{Y: 3, X: 0}, e.cursorAtScroll())

			e.MoveLastLine()
			e.MoveStartLine()
			assert.Equal(t, term.Coordinates{Y: 9, X: 0}, e.Coordinates())
			assert.Equal(t, term.Coordinates{Y: 34}, e.cursorAtScroll())

			e.InsertLineBelow()
			assert.Equal(t, 36, e.scroll.Buffer().Rows())
			assert.Equal(t, term.Coordinates{Y: 9, X: 0}, e.Coordinates())
			assert.Equal(t, term.Coordinates{Y: 35}, e.cursorAtScroll())
		})
	}
}

func TestCursorInsertLineBelow(t *testing.T) {
	suite := []struct {
		wrap bool
	}{
		{false}, {true},
	}
	for _, test := range suite {
		t.Run(fmt.Sprintf("wrap: %v", test.wrap), func(t *testing.T) {
			content := `package main
func main() {
}`
			cursor := setupCursorContent(t, 10, 10, content, test.wrap)
			buf := cursor.scroll.Buffer()
			assert.Equal(t, buf.String(), content)

			require.True(t, cursor.MoveLineDown())
			cursor.InsertLineBelow()
			cursor.InsertLineBelow()
			cursor.Insert('\t')
			cursor.Insert('f')
			cursor.Insert('m')
			cursor.Insert('t')
			cursor.Insert('.')

			assert.Equal(t, `package main
func main() {

	fmt.
}`, buf.String())

			require.True(t, cursor.MoveLastLine())

			cursor.InsertLineBelow()
			cursor.InsertLineBelow()
			cursor.Insert('i')
			cursor.InsertLineBelow()
			cursor.Insert('\t')
			cursor.InsertString("XXXXXXXXXXXXXXXXXXXXXXXX")
			cursor.InsertLineBelow()
			cursor.Insert('}')

			assert.Equal(t, `package main
func main() {

	fmt.
}

i
	XXXXXXXXXXXXXXXXXXXXXXXX
}`, buf.String())

			require.False(t, cursor.MoveLastLine())
			require.True(t, cursor.MoveLineUp())
			cursor.MoveStartLine()
			require.True(t, cursor.MoveEndLine())
			cursor.InsertLineBelow()
			cursor.InsertString("hello")

			assert.Equal(t, `package main
func main() {

	fmt.
}

i
	XXXXXXXXXXXXXXXXXXXXXXXX
hello
}`, buf.String())
		})
	}

}

func TestCursorInsertDeleteFirstEmptyLineEdgeCase(t *testing.T) {
	suite := []struct {
		wrap bool
	}{
		{true}, {false},
	}
	for _, test := range suite {
		t.Run(fmt.Sprintf("Delete from end, wrap:%v", test.wrap), func(t *testing.T) {
			e := setupCursorContent(t, 4, 4, "\n22222", test.wrap)
			str := e.scroll.Buffer().String()

			e.Insert('p')
			e.Insert('a')
			e.Insert('c')
			e.Insert('k')
			e.Insert('X')
			e.Insert('X')
			e.Insert('X')
			require.Equal(t, "packXXX\n22222", e.scroll.Buffer().String())

			for i := 0; i < 7; i++ {
				require.True(t, e.MoveLeft(), i)
				e.Delete()
			}

			assert.Equal(t, str, e.scroll.Buffer().String())
		})
		t.Run(fmt.Sprintf("Delete from start, wrap:%v", test.wrap), func(t *testing.T) {
			e := setupCursorContent(t, 4, 4, "\n22222", test.wrap)
			str := e.scroll.Buffer().String()

			e.Insert('p')
			e.Insert('a')
			e.Insert('c')
			e.Insert('k')
			e.Insert('X')
			e.Insert('X')
			e.Insert('X')
			require.Equal(t, "packXXX\n22222", e.scroll.Buffer().String())

			e.MoveFirstLine()
			e.MoveStartLine()
			for i := 0; i < 7; i++ {
				e.Delete()
			}

			assert.Equal(t, str, e.scroll.Buffer().String())
		})
	}
}

func TestCursorInsertLongStream(t *testing.T) {
	insertStr := func(c *Cursor, r rune) {
		c.InsertString(string(r))
	}
	suite := []struct {
		description string
		wrap        bool
		method      func(c *Cursor, r rune)
	}{
		{"Insert in wrap mode", true, (*Cursor).Insert},
		{"Insert in non-wrap mode", false, (*Cursor).Insert},
		{"InsertString in wrap mode", true, insertStr},
		{"InsertString in non-wrap mode", false, insertStr},
	}
	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			width, height := 4, 4
			e := setupCursorContent(t, width, height, "", test.wrap)
			for i := 0; i < width*height; i++ {
				test.method(e, rune(int('a')+i))
			}
			assert.Equal(t, "abcdefghijklmnop", e.scroll.Buffer().String())
		})
	}
}

func TestCursorBackspace(t *testing.T) {
	suite := []struct {
		wrap           bool
		rightInclusive bool
	}{
		{true, false}, {false, false},
		{true, true}, {false, true},
	}
	for _, test := range suite {
		t.Run(fmt.Sprintf("wrap:%v, right-inclusive:%t", test.wrap, test.rightInclusive), func(t *testing.T) {
			e := setupCursor(t, 10, 10, test.wrap)
			e.RightInclusiveSemantics = test.rightInclusive
			n := len(e.scroll.Buffer().String())

			require.True(t, e.MoveLastLine())
			require.True(t, e.MoveEndLine())

			for i := 0; i <= n; i++ {
				e.Backspace()
			}

			assert.Equal(t, "", e.scroll.Buffer().String())
		})
	}
}

func TestCursorConflate(t *testing.T) {
	conflateAllRows := func(t *testing.T, c *Cursor) {
		n := strings.Count(c.scroll.Buffer().String(), "\n")
		rows := c.scroll.Buffer().Rows()
		require.Equal(t, n+1, rows)

		for i := 0; i < n; i++ {
			require.True(t, c.Conflate(), i) //, "cursor: %+v, %s", c.Coordinates(), c.scroll.Buffer().String())
		}
		require.Equal(t, 1, c.scroll.Buffer().Rows())
		assert.Equal(t, 0, strings.Count(c.scroll.Buffer().String(), "\n"))
	}

	suite := []struct {
		description   string
		width, height int
		wrap          bool
		sut           func(*testing.T, *Cursor)
	}{
		{"conflate all rows into one (wrap)", 10, 10, true,
			conflateAllRows,
		},
		{"conflate all rows into one (wrap, no wraps)", 100, 100, true,
			conflateAllRows,
		},
		{"conflate all rows into one (no wrap)", 10, 10, false,
			conflateAllRows,
		},
	}
	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			e := setupCursor(t, test.width, test.height, test.wrap)
			test.sut(t, e)
		})
	}
}

func TestBackspaceViaConflate(t *testing.T) {
	const (
		str = `
/*
 * Check if the current buffer should be added to or removed from the list of
 * diff buffers.
 */
	void
diff_buf_adjust(win_T *win)
{
	win_T	*wp;
	int		i;

	if (!win->w_p_diff)
	{
	/* When there is no window showing a diff for this buffer, remove
	 * it from the diffs. */
	FOR_ALL_WINDOWS(wp)
		if (wp->w_buffer == win->w_buffer && wp->w_p_diff)
		break;
	if (wp == NULL)
	{
		i = diff_buf_idx(win->w_buffer);
		if (i != DB_COUNT)
		{X
`
		expected = `
/*
 * Check if the current buffer should be added to or removed from the list of
 * diff buffers.
 */
	void
diff_buf_adjust(win_T *win)
{
	win_T	*wp;
	int		i;

	if (!win->w_p_diff)
	{
	/* When there is no window showing a diff for this buffer, remove
	 * it from the diffs. */
	FOR_ALL_WINDOWS(wp)
		if (wp->w_buffer == win->w_buffer && wp->w_p_diff)
		break;
	if (wp == NULL)
	{
		i = diff_buf_idx(win->w_buffer);
		if (i != DB_COUNT)
		{`
	)
	e := setupCursorContent(t, 10, 10, str, true)
	e.MoveLastLine()
	e.MoveEndLine()

	// sut
	e.Backspace()
	e.Backspace()

	assert.Equal(t, expected, e.scroll.Buffer().String())
}

func testCursorSelect(t *testing.T, width, height int) {
	makeSelect := func(t *testing.T) *Cursor {
		e := setupCursor(t, width, height, false)
		str := e.scroll.Buffer().String()
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
		assert.False(t, e.Unselect())
		require.True(t, e.SelectLine())

		for e.MoveDown() {
		}
		for e.MoveRight() {
		}
		// test that we can switch between after move
		require.True(t, e.Select())
		assert.Equal(t, str, e.Selection())
		return e
	}

	makeSelectLine := func(t *testing.T) *Cursor {
		e := setupCursor(t, width, height, false)
		str := e.scroll.Buffer().String()
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
		require.True(t, e.Select())
		require.True(t, e.MoveLastLine())
		// test that we can switch between after move
		require.True(t, e.SelectLine())
		assert.Equal(t, fmt.Sprintf("%s\n", str), e.Selection())
		return e
	}

	makeSelectBlock := func(t *testing.T) *Cursor {
		e := setupCursor(t, width, height, false)
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
		require.True(t, e.SelectLine())
		require.True(t, e.MoveLastLine())
		require.True(t, e.MoveEndLine())
		require.True(t, e.SelectBlock())
		return e
	}

	t.Run("Select then Unselect should reverse all attributes", func(t *testing.T) {
		e := makeSelect(t)
		assert.True(t, e.Unselect())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
	})

	t.Run("Select then insert should add attributes to inserted runes", func(t *testing.T) {
		inserts := []func(*Cursor){
			func(e *Cursor) {
				e.Insert('a')
				e.Insert('b')
				e.Insert('c')
			},
			func(e *Cursor) {
				e.InsertString("abc")
			},
		}
		for _, insert := range inserts {
			e := makeSelect(t)
			e.Unselect()
			e.MoveFirstLine()
			e.MoveStartLine()
			e.Select()
			e.MoveLastLine()
			e.MoveEndLine()
			insert(e)
			assertBufferAttributes(t, e.buffer(), term.Attributes{Attrs: tcell.AttrReverse})
			assert.True(t, e.Unselect())
			assertBufferAttributes(t, e.buffer(), term.Attributes{})
		}
	})

	t.Run("SelectLine then Unselect should reverse all attributes", func(t *testing.T) {
		e := makeSelectLine(t)
		assert.True(t, e.Unselect())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
	})

	t.Run("SelectBlock then Unselect should reverse all attributes", func(t *testing.T) {
		e := makeSelectBlock(t)
		assert.True(t, e.Unselect())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
	})

	t.Run("Select/DeleteSelection selects from start to end", func(t *testing.T) {
		e := makeSelect(t)
		require.True(t, e.DeleteSelection())
		assert.Equal(t, "", e.Selection())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
	})

	t.Run("reverse coords Select/DeleteSelection selects from start to end", func(t *testing.T) {
		e := setupCursor(t, width, height, false)
		str := e.scroll.Buffer().String()
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		require.True(t, e.MoveLastLine())
		require.True(t, e.MoveEndLine())

		require.True(t, e.Select())

		require.True(t, e.MoveFirstLine())

		assert.Equal(t, str, e.Selection())
		require.True(t, e.DeleteSelection())
		assert.Equal(t, "", e.scroll.Buffer().String())
	})

	t.Run("SelectLine/DeleteSelection selects from start line to end line", func(t *testing.T) {
		e := makeSelectLine(t)
		require.True(t, e.DeleteSelection())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
	})

	t.Run("SelectBlock/DeleteSelection selects from start to end in block", func(t *testing.T) {
		e := makeSelectBlock(t)
		require.True(t, e.DeleteSelection())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
	})

	t.Run("Select/CopySelection copies from start to end", func(t *testing.T) {
		e := makeSelect(t)
		c := clipboard.NewInMemory()
		ok, err := e.CopySelection(clipboard.DefaultRegisterID, c)
		require.True(t, ok)
		require.NoError(t, err)
		assert.Equal(t, "", e.Selection())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		data, err := c.Paste(clipboard.DefaultRegisterID)
		require.NoError(t, err)
		assert.Equal(t, e.scroll.Buffer().String(), data.Text)
	})

	t.Run("Select/CopySelection doesn't panic if width is 0", func(t *testing.T) {
		e := makeSelect(t)
		e.scroll.Wrap = true
		e.scroll.Resize(0, 0)
		c := clipboard.NewInMemory()
		assert.NotPanics(t, func() {
			e.CopySelection(clipboard.DefaultRegisterID, c)
		})
	})

	t.Run("SelectLine/CopySelection copies from start line to end line", func(t *testing.T) {
		e := makeSelectLine(t)
		c := clipboard.NewInMemory()
		ok, err := e.CopySelection(clipboard.DefaultRegisterID, c)
		require.True(t, ok)
		require.NoError(t, err)
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		data, err := c.Paste(clipboard.DefaultRegisterID)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("%s\n", e.scroll.Buffer().String()), data.Text)
	})

	t.Run("SelectBlock/CopySelection copies from start to end in block", func(t *testing.T) {
		e := makeSelectBlock(t)
		c := clipboard.NewInMemory()
		ok, err := e.CopySelection(clipboard.DefaultRegisterID, c)
		require.True(t, ok)
		require.NoError(t, err)
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		data, err := c.Paste(clipboard.DefaultRegisterID)
		require.NoError(t, err)
		assert.NotZero(t, data.Text)
	})
}

func assertBufferAttributes(t *testing.T, b *cell.Buffer, attr term.Attributes) {
	for y, row := range b.RawCells() {
		for x, c := range row {
			assert.Equal(t, attr.Bg, c.Bg, "at y=%d;x=%d", y, x)
			assert.Equal(t, attr.Fg, c.Fg, "at y=%d;x=%d", y, x)
		}
	}
}

func TestCursorSelect10(t *testing.T) {
	testCursorSelect(t, 10, 100)
}

func TestCursorSelect20(t *testing.T) {
	testCursorSelect(t, 20, 100)
}

func TestCursorSelect50(t *testing.T) {
	testCursorSelect(t, 50, 100)
}

func TestCursorSelect100(t *testing.T) {
	testCursorSelect(t, 100, 100)
}

func testCursorUndoRedo(t *testing.T, moveBefore, moveAfter func(c *Cursor) bool, width, height int) {
	const input = "Aleda"
	e := setupCursor(t, width, height, false)
	str := e.scroll.Buffer().String()

	moveBefore(e)
	cBefore := e.Coordinates()

	e.InsertString(input)
	str2 := e.scroll.Buffer().String()

	require.True(t, e.Undo())
	require.False(t, e.Undo())

	c := e.Coordinates()
	assert.Equal(t, cBefore, c)
	assert.Equal(t, str, e.scroll.Buffer().String())

	moveAfter(e)

	require.True(t, e.Redo())
	assert.Equal(t, str2, e.scroll.Buffer().String())
	require.False(t, e.Redo())

	require.True(t, e.Undo())
	require.False(t, e.Undo())

	c = e.Coordinates()
	assert.Equal(t, cBefore, c)
	assert.Equal(t, str, e.scroll.Buffer().String())
}

func TestCursorUndoRedo10(t *testing.T) {
	testCursorUndoRedo(t, (*Cursor).MoveFirstLine, (*Cursor).MoveLastLine, 10, 10)
}
func TestCursorUndoRedo20(t *testing.T) {
	testCursorUndoRedo(t, (*Cursor).MoveFirstLine, (*Cursor).MoveLastLine, 20, 20)
}
func TestCursorUndoRedo100(t *testing.T) {
	testCursorUndoRedo(t, (*Cursor).MoveFirstLine, (*Cursor).MoveLastLine, 100, 100)
}

func TestCursorUndoRedo10Backwards(t *testing.T) {
	testCursorUndoRedo(t, (*Cursor).MoveLastLine, (*Cursor).MoveFirstLine, 10, 10)
}
func TestCursorUndoRedo20Backwards(t *testing.T) {
	testCursorUndoRedo(t, (*Cursor).MoveLastLine, (*Cursor).MoveFirstLine, 20, 20)
}
func TestCursorUndoRedo100Backwards(t *testing.T) {
	testCursorUndoRedo(t, (*Cursor).MoveLastLine, (*Cursor).MoveFirstLine, 100, 100)
}

func testCursorDeleteSelection(t *testing.T, width, height int, typeSelect SelectMode) {
	tsuite := []struct {
		initialBuf              string
		initialPos              func(*Cursor)
		finalPos                func(*Cursor)
		deleted                 bool
		finalBuf                string
		skipForMode             []SelectMode
		rightInclusiveSemantics bool
	}{
		{
			initialBuf:  "",
			finalPos:    func(*Cursor) {},
			deleted:     false,
			finalBuf:    "",
			skipForMode: []SelectMode{LineSelection},
		},
		{
			initialBuf:  "a",
			finalPos:    func(*Cursor) {},
			deleted:     false,
			finalBuf:    "a",
			skipForMode: []SelectMode{LineSelection},
		},
		{
			initialBuf: "a",
			finalPos:   func(c *Cursor) { c.MoveRight() },
			deleted:    true,
			finalBuf:   "",
		},
		{
			initialBuf:  "\n",
			finalPos:    func(*Cursor) {},
			deleted:     false,
			finalBuf:    "\n",
			skipForMode: []SelectMode{BlockSelection, LineSelection},
		},
		{
			initialBuf:  "\n",
			finalPos:    func(c *Cursor) { c.MoveDown() },
			deleted:     true,
			finalBuf:    "",
			skipForMode: []SelectMode{BlockSelection},
		},
		{
			initialBuf:  "a\nb",
			finalPos:    func(c *Cursor) { c.MoveRight() },
			deleted:     true,
			finalBuf:    "\nb",
			skipForMode: []SelectMode{LineSelection, BlockSelection},
		},
		{
			initialBuf:  "a\nb",
			finalPos:    func(c *Cursor) { c.MoveDown() },
			finalBuf:    "b",
			deleted:     true,
			skipForMode: []SelectMode{LineSelection, BlockSelection},
		},
		{
			initialBuf: "a\nb",
			initialPos: func(c *Cursor) {
				for c.MoveRight() {
				}
			},
			finalPos: func(c *Cursor) {
				c.MoveDown()
				c.MoveRight()
			},
			deleted:     true,
			finalBuf:    "a",
			skipForMode: []SelectMode{LineSelection, BlockSelection},
		},
		{
			initialBuf: "a\nb\nc\nd",
			finalPos: func(c *Cursor) {
				for c.MoveDown() {
				}
				c.MoveRight()
			},
			deleted:     true,
			finalBuf:    "",
			skipForMode: []SelectMode{BlockSelection},
		},
		{
			initialBuf: "a\nb",
			initialPos: func(c *Cursor) {
				for i := 0; i < 100; i++ {
					c.MoveDown()
				}
			},
			finalPos:    func(*Cursor) {},
			deleted:     false,
			finalBuf:    "a\nb",
			skipForMode: []SelectMode{LineSelection},
		},
		{
			initialBuf: "a\nb\nc\nd",
			finalPos: func(c *Cursor) {
				c.MoveLastLine()
				c.MoveEndLine()
			},
			deleted:     true,
			finalBuf:    "",
			skipForMode: []SelectMode{BlockSelection},
		},
		{
			initialBuf: "type Writer {\n\ta int\n\tb int\n}\n",
			initialPos: func(c *Cursor) {
				c.MoveLastLine()
			},
			finalPos: func(c *Cursor) {
				c.MoveFirstLine()
			},
			deleted:     true,
			finalBuf:    "",
			skipForMode: []SelectMode{StandardSelection, BlockSelection},
		},
		{
			initialBuf:              "",
			finalPos:                func(*Cursor) {},
			deleted:                 false,
			finalBuf:                "",
			rightInclusiveSemantics: true,
		},
		{
			initialBuf:              "a",
			finalPos:                func(*Cursor) {},
			deleted:                 true,
			finalBuf:                "",
			rightInclusiveSemantics: true,
		},
		{
			initialBuf:              "\n",
			finalPos:                func(*Cursor) {},
			deleted:                 true,
			finalBuf:                "",
			skipForMode:             []SelectMode{StandardSelection, BlockSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf:              "a\nb",
			finalPos:                func(c *Cursor) { c.MoveRight() },
			deleted:                 true,
			finalBuf:                "\nb",
			skipForMode:             []SelectMode{LineSelection, BlockSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf:              "a\nb",
			finalPos:                func(c *Cursor) { c.MoveDown() },
			finalBuf:                "",
			deleted:                 true,
			skipForMode:             []SelectMode{LineSelection, BlockSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf: "a\nb",
			initialPos: func(c *Cursor) {
				for range 3 {
					c.MoveRight()
				}
			},
			finalPos:                func(c *Cursor) { c.MoveDown() },
			deleted:                 true,
			finalBuf:                "a",
			skipForMode:             []SelectMode{LineSelection, BlockSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf: "a\nb\nc\nd",
			finalPos: func(c *Cursor) {
				for c.MoveDown() {
				}
			},
			deleted:                 true,
			finalBuf:                "",
			skipForMode:             []SelectMode{BlockSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf: "a\nb",
			initialPos: func(c *Cursor) {
				for c.MoveDown() {
				}
				c.MoveRight()
			},
			finalPos:                func(*Cursor) {},
			deleted:                 false,
			finalBuf:                "a\nb",
			skipForMode:             []SelectMode{LineSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf: "a\nb\nc\nd",
			finalPos: func(c *Cursor) {
				c.MoveLastLine()
				c.MoveEndLine()
			},
			deleted:                 true,
			finalBuf:                "",
			skipForMode:             []SelectMode{BlockSelection},
			rightInclusiveSemantics: true,
		},
		{
			initialBuf: "type Writer {\n\ta int\n\tb int\n}\n",
			initialPos: func(c *Cursor) {
				c.MoveLastLine()
			},
			finalPos: func(c *Cursor) {
				c.MoveFirstLine()
			},
			deleted:                 true,
			finalBuf:                "",
			skipForMode:             []SelectMode{StandardSelection, BlockSelection},
			rightInclusiveSemantics: true,
		},
	}

	for i, tcase := range tsuite {
		desc := fmt.Sprintf("select %v test case %d, right inclusive semantics: %t",
			typeSelect, i, tcase.rightInclusiveSemantics)
		t.Run(desc, func(t *testing.T) {
			if slices.Contains(tcase.skipForMode, typeSelect) {
				return
			}

			c := setupCursorContent(t, width, height, tcase.initialBuf, false)
			c.RightInclusiveSemantics = tcase.rightInclusiveSemantics
			if tcase.initialPos != nil {
				tcase.initialPos(c)
			}
			switch typeSelect {
			case NoSelection:
				panic("hmm...")
			case BlockSelection:
				c.SelectBlock()
			case LineSelection:
				c.SelectLine()
			case StandardSelection:
				c.Select()
			}
			tcase.finalPos(c)
			require.Equal(t, tcase.deleted, c.DeleteSelection(), "deleted ok")
			if !tcase.deleted {
				return
			}
			assert.Equal(t, tcase.finalBuf, c.scroll.Buffer().String())
		})
	}
}

func TestCursorDeleteSelection10(t *testing.T) {
	testCursorDeleteSelection(t, 10, 10, StandardSelection)
}
func TestCursorDeleteSelection20(t *testing.T) {
	testCursorDeleteSelection(t, 20, 20, StandardSelection)
}
func TestCursorDeleteSelection1000(t *testing.T) {
	testCursorDeleteSelection(t, 1000, 1000, StandardSelection)
}
func TestCursorDeleteSelectionLine10(t *testing.T) {
	testCursorDeleteSelection(t, 10, 10, LineSelection)
}
func TestCursorDeleteSelectionLine20(t *testing.T) {
	testCursorDeleteSelection(t, 20, 20, LineSelection)
}
func TestCursorDeleteSelectionLine1000(t *testing.T) {
	testCursorDeleteSelection(t, 1000, 1000, LineSelection)
}
func TestCursorDeleteSelectionBlock10(t *testing.T) {
	testCursorDeleteSelection(t, 10, 10, BlockSelection)
}
func TestCursorDeleteSelectionBlock20(t *testing.T) {
	testCursorDeleteSelection(t, 20, 20, BlockSelection)
}
func TestCursorDeleteSelectionBlock1000(t *testing.T) {
	testCursorDeleteSelection(t, 1000, 1000, BlockSelection)
}

func TestCursorMoveToBounds(t *testing.T) {
	tsuite := []struct {
		desc          string
		content       string
		cursorWin     term.Coordinates
		offset        term.Coordinates
		width, height int
		padding       int

		wantScroll term.Coordinates
	}{
		{"empty buf does nothing without padding",
			"", term.Coordinates{}, term.Coordinates{}, 10, 10, 0,
			term.Coordinates{}},
		{"empty buf does nothing with padding",
			"", term.Coordinates{}, term.Coordinates{}, 10, 10, 2,
			term.Coordinates{}},
		{"negative cursor window X",
			"a\nb", term.Coordinates{Y: 3, X: -3}, term.Coordinates{}, 5, 5, 1,
			term.Coordinates{Y: 1}},
		{"negative cursor window Y",
			"a\nb", term.Coordinates{X: 1, Y: -3}, term.Coordinates{}, 5, 5, 0,
			term.Coordinates{}},
		{"no offset, no padding out of X bounds first line",
			"a\nb", term.Coordinates{X: 2}, term.Coordinates{}, 5, 5, 0,
			term.Coordinates{X: 0}},
		{"no offset, with padding out of X bounds first line",
			"a\nb", term.Coordinates{X: 2}, term.Coordinates{}, 5, 5, 1,
			term.Coordinates{X: 1}},
		{"no offset, no padding out of X bounds last line",
			"a\nb", term.Coordinates{X: 2, Y: 1}, term.Coordinates{}, 5, 5, 0,
			term.Coordinates{X: 0, Y: 1}},
		{"no offset, with padding out of X bounds last line",
			"a\nb", term.Coordinates{X: 2, Y: 1}, term.Coordinates{}, 5, 5, 1,
			term.Coordinates{X: 1, Y: 1}},
		{"no offset, with padding NOT out of X bounds",
			"a\nb", term.Coordinates{X: 2, Y: 1}, term.Coordinates{}, 5, 5, 2,
			term.Coordinates{X: 2, Y: 1}},
		{"offset, no padding out of X bounds first line",
			"a\nbbbbb", term.Coordinates{X: 2}, term.Coordinates{X: 1}, 2, 2, 0,
			term.Coordinates{X: 0}},
		{"offset, with padding out of X bounds first line",
			"a\nbbbbbb", term.Coordinates{X: 2}, term.Coordinates{X: 1}, 2, 2, 1,
			term.Coordinates{X: 1}},
		{"offset, no padding out of X bounds last line",
			"aaaaaaa\nb", term.Coordinates{X: 1, Y: 1}, term.Coordinates{X: 1}, 2, 2, 0,
			term.Coordinates{X: 0, Y: 1}},
		{"offset, with padding out of X bounds last line",
			"aaaaaaa\nb", term.Coordinates{X: 1, Y: 1}, term.Coordinates{X: 1}, 2, 2, 1,
			term.Coordinates{X: 1, Y: 1}},
		{"offset, with padding NOT out of X bounds",
			"aaaaaaaaa\nb", term.Coordinates{X: 0, Y: 1}, term.Coordinates{X: 2}, 2, 2, 2,
			term.Coordinates{X: 2, Y: 1}},
		{"no offset, last EOL",
			"a\n", term.Coordinates{X: 3, Y: 2}, term.Coordinates{}, 5, 5, 0,
			term.Coordinates{X: 0, Y: 1}},
		{"offset, last EOL",
			"aaaaaaaaaaaa\n", term.Coordinates{}, term.Coordinates{X: 3, Y: 1}, 2, 1, 0,
			term.Coordinates{X: 0, Y: 1}},
	}
	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			e := setupCursorContent(t, tcase.width, tcase.height, tcase.content, false)
			e.cursor = tcase.cursorWin
			if tcase.offset != (term.Coordinates{}) {
				require.True(t, e.scroll.SetOffset(tcase.offset))
			}

			e.MoveToBounds(tcase.padding)
			assert.Equal(t, tcase.wantScroll, e.cursorAtScroll())
		})
	}
}

func TestCursorMoveToBoundsOld(t *testing.T) {
	e := setupCursor(t, 100, 100, false)

	pos := e.Coordinates()
	assert.Equal(t, term.Coordinates{}, pos)

	e.cursor.X += 3

	e.MoveToBounds(2)

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 1}, pos)

	pos = e.Coordinates()
	assert.True(t, e.MoveDown())

	e.MoveEndLine()
	e.MoveRight()
	e.MoveRight()

	e.MoveToBounds(1)

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 2, Y: 1}, pos)

	e.MoveToBounds(0)

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 1, Y: 1}, pos)

	e.MoveLastLine()
	e.MoveDown()

	e.MoveToBounds(0)

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 1, Y: 31}, pos)
}

func TestCursorCell(t *testing.T) {
	t.Run("does not panic if cursor has negative coords", func(t *testing.T) {
		e := setupCursor(t, 100, 100, false)
		e.cursor = term.Coordinates{X: -1}
		_, ok := e.Cell()
		assert.False(t, ok)
	})
}

func TestCursorShiftLine(t *testing.T) {
	c := setupCursorContent(t, 10, 1, " blabla\nbleble", false)
	assert.True(t, c.ShiftLineLeft())
	assert.False(t, c.ShiftLineLeft())
	assert.Equal(t, term.Coordinates{}, c.cursor)

	c.ShiftLineRight()
	assert.Equal(t, term.Coordinates{X: 4}, c.cursor)
	c.ShiftLineRight()
	assert.Equal(t, term.Coordinates{X: 8}, c.cursor)
	assert.True(t, c.ShiftLineLeft())
	assert.Equal(t, term.Coordinates{X: 4}, c.cursor)
	assert.True(t, c.ShiftLineLeft())
	assert.Equal(t, term.Coordinates{}, c.cursor)
	assert.True(t, c.MoveDown())
	c.ShiftLineRight()
	assert.Equal(t, term.Coordinates{Y: 0, X: 4}, c.cursor)
	assert.True(t, c.ShiftLineLeft())
	assert.Equal(t, term.Coordinates{Y: 0, X: 0}, c.cursor)
}

func TestCursorShiftSelection(t *testing.T) {
	c := setupCursorContent(t, 10, 10, " blabla\nbleble", false)
	require.True(t, c.Select())
	require.True(t, c.MoveDown())

	c.ShiftSelectionRight()
	assert.Equal(t, "\t blabla\n\tbleble", c.scroll.Buffer().String())

	require.True(t, c.SelectBlock())
	require.True(t, c.MoveUp())
	assert.True(t, c.ShiftSelectionLeft())
	assert.Equal(t, " blabla\nbleble", c.scroll.Buffer().String())
}

var (
	abcAttr      = term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}
	abcLocations = []textapi.Location{
		{
			From:    term.Coordinates{Y: 1},
			To:      term.Coordinates{Y: 1, X: 1},
			Attr:    abcAttr,
			Message: "blabla",
		},
		{
			From: term.Coordinates{Y: 2},
			To:   term.Coordinates{Y: 2, X: 1},
			Attr: abcAttr,
		},
		{
			From: term.Coordinates{Y: 3},
			To:   term.Coordinates{Y: 3, X: 1},
			Attr: abcAttr,
		},
	}
)

func TestCursorMoveLocationList(t *testing.T) {
	t.Run("MoveToPrevLocation should return false and do nothing if location list is nil", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "", false)
		assert.False(t, c.MoveToPrevLocation(locID))
	})

	t.Run("MoveToNextLocation should return false and do nothing if location list is nil", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "", false)
		assert.False(t, c.MoveToNextLocation(locID))
	})

	t.Run("MoveToPrevLocation should return false and do nothing if already at start of location list", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "", false)
		locations := []textapi.Location{{}}
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(locations)))
		assert.False(t, c.MoveToPrevLocation(locID))
	})

	t.Run("MoveToNextLocation should return false and do nothing if already at end of location list", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "", false)
		locations := []textapi.Location{{}}
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(locations)))
		assert.False(t, c.MoveToNextLocation(locID))
	})

	t.Run("MoveToPrevLocation should return true and move cursor to earlier location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, " X", false)
		require.True(t, c.MoveRight())

		locations := []textapi.Location{{}}
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(locations)))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{}, pos)
	})

	t.Run("MoveToNextLocation should wrap around to first location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc \n", false)
		require.True(t, c.MoveLastLine())

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(abcLocations)))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 1}, pos)
	})

	t.Run("MoveToPrevLocation should wrap around to last location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(abcLocations)))
		assert.True(t, c.MoveToPrevLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 3}, pos)
	})

	t.Run("MoveToNextLocation should wrap around to first location (special case)", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, " X", false)
		require.True(t, c.MoveRight())

		locations := []textapi.Location{{}, {From: term.Coordinates{X: 1}}}
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(locations)))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{}, pos)
	})

	t.Run("MoveToPrevLocation should wrap around to last location (special case)", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, " X", false)

		locations := []textapi.Location{{}, {From: term.Coordinates{X: 1}}}
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(locations)))
		assert.True(t, c.MoveToPrevLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{X: 1}, pos)
	})

	t.Run("MoveToNextLocation should go to next location after cursor", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc \n", false)
		require.True(t, c.MoveDown())
		require.True(t, c.MoveDown())

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(abcLocations)))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 3}, pos)
	})

	t.Run("MoveToPrevLocation should go to prev location before location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)
		require.True(t, c.MoveDown())
		require.True(t, c.MoveDown())

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(abcLocations)))
		assert.True(t, c.MoveToPrevLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 1}, pos)
	})

	t.Run("MoveToNextLocation should go to next location if current location is hidden", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc \n", false)
		require.True(t, c.Select())
		require.True(t, c.MoveDown())
		require.True(t, c.MoveDown())
		require.True(t, c.HideSelection())
		require.False(t, c.MoveFirstLine())

		assert.Equal(t, term.Coordinates{Y: 0}, c.Coordinates())

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, LocationSlice(abcLocations)))
		assert.True(t, c.MoveToNextLocation(locID))

		assert.Equal(t, term.Coordinates{Y: 1}, c.Coordinates())
	})
}

func TestCursorMoveToScroll(t *testing.T) {
	for _, wrap := range []bool{true, false} {
		t.Run(fmt.Sprintf("wrap=%v", wrap), func(t *testing.T) {
			t.Run("moves cursor to position within curr width,height", func(t *testing.T) {
				e := setupCursor(t, 10, 10, wrap)
				e.MoveToScroll(term.Coordinates{X: 1, Y: 3})
				pos := e.CursorAtScroll()
				assert.Equal(t, term.Coordinates{Y: 3, X: 1}, pos)
			})
			t.Run("moves cursor to position past curr height", func(t *testing.T) {
				e := setupCursor(t, 5, 5, wrap)
				e.MoveToScroll(term.Coordinates{X: 0, Y: 6})
				pos := e.CursorAtScroll()
				assert.Equal(t, term.Coordinates{Y: 6, X: 0}, pos)
			})
			t.Run("moves cursor to position past curr width", func(t *testing.T) {
				e := setupCursor(t, 5, 5, wrap)
				e.MoveToScroll(term.Coordinates{X: 7, Y: 2})
				pos := e.CursorAtScroll()
				assert.Equal(t, term.Coordinates{Y: 2, X: 7}, pos)
			})

		})
	}
}

func TestCursorWrap(t *testing.T) {
	t.Run("takes wraps into consideration", func(t *testing.T) {
		e := setupCursor(t, 10, 10, true)
		_, ok := e.MoveToScroll(term.Coordinates{X: 76, Y: 2})
		require.True(t, ok)
		pos := e.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 9, X: 6}, pos)
	})
	t.Run("handles cursor.X past last row's column", func(t *testing.T) {
		e := setupCursor(t, 10, 10, true)
		_, ok := e.MoveToScroll(term.Coordinates{X: 77, Y: 2})
		require.True(t, ok)
		pos := e.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 9, X: 7}, pos)
	})
	t.Run("MoveRight moves cursor past the end of line of a wrapped line", func(t *testing.T) {
		e := setupCursor(t, 10, 10, true)
		e.MoveToScroll(term.Coordinates{X: 15, Y: 3})

		require.True(t, e.MoveRight())
		cursor := e.CursorAtScroll()
		assert.Equal(t, term.Coordinates{Y: 3, X: 16}, cursor)
	})
}

func TestCursorSelectWordInsertWord(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrap:%v", wrap), func(t *testing.T) {
			e := setupCursorContent(t, 5, 5, sampleSnippet+"\n", wrap)
			require.True(t, e.MoveDown())
			require.True(t, e.MoveDown())
			require.True(t, e.MoveRightStartWord())
			require.True(t, e.Select())
			require.True(t, e.MoveRightStartWord())
			require.True(t, e.DeleteSelection())
			l := len(e.buffer().String())
			e.Insert('h')
			e.Insert('e')
			e.Insert('l')
			e.Insert('l')
			e.Insert('o')
			assert.Equal(t, l+5, len(e.buffer().String()))
		})
	}
}

func TestCursorInsertLimitedWidth(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrap:%v", wrap), func(t *testing.T) {
			e := setupCursorContent(t, 3, 1, "", wrap)
			e.Insert('h')
			e.Insert('e')
			e.Insert('l')
			e.Insert('l')
			e.Insert('o')
			e.Insert(' ')
			e.Insert('w')
			e.Insert('o')
			e.Insert('r')
			e.Insert('l')
			e.Insert('d')
			assert.Equal(t, "hello world", e.buffer().String())
		})
	}
}

func TestCursorReplaceAllWithNewline(t *testing.T) {
	e := setupCursorContent(t, 5, 5, sampleSnippet+"\n", false)
	require.True(t, e.SelectLine())
	require.True(t, e.MoveLastLine())
	require.True(t, e.DeleteSelection())
	e.Insert('h')
	e.Insert('e')
	e.Insert('l')
	e.Insert('l')
	e.Insert('o')
	e.Insert('\n')
	e.Insert('w')
	e.Insert('o')
	e.Insert('r')
	e.Insert('l')
	e.Insert('d')
	assert.Equal(t, "hello\nworld", e.buffer().String())
}

func cwdURI(t *testing.T) workspaceapi.URI {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %s", err)
	}
	uri, err := workspaceapi.CurrentUserHostURI(wd)
	if err != nil {
		t.Fatalf("parse working directory as URI %s: %s", wd, err)
	}
	return uri
}

func TestFileCursorIntegration(t *testing.T) {
	tsuite := []struct {
		description string
		lastEOL     bool
		test        func(t *testing.T, c *Cursor)
	}{
		{"does not move beyond line before last EOL", true, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			assert.Equal(t, term.Coordinates{Y: 31}, cursor.CursorAtScroll())
		}},
		{"does not move beyond last line", false, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			assert.Equal(t, term.Coordinates{Y: 31}, cursor.CursorAtScroll())
		}},
		{"is able to insert at last line + 1", false, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			cursor.InsertLineBelow()
			assert.Equal(t, term.Coordinates{Y: 32}, cursor.CursorAtScroll())
			assert.Equal(t, sampleSnippet+"\n", cursor.buffer().String())
		}},
		{"is able to insert at last EOL", true, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			cursor.InsertLineBelow()
			assert.Equal(t, term.Coordinates{Y: 32}, cursor.CursorAtScroll())
			assert.Equal(t, sampleSnippet+"\n", cursor.buffer().String())
		}},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.description, func(t *testing.T) {
			b := cell.NewBuffer()
			file, err := os.CreateTemp("", "frctl_file_test")
			require.NoError(t, err)

			_, err = file.Write([]byte(sampleSnippet))
			require.NoError(t, err)

			if tcase.lastEOL {
				_, err = file.Write([]byte{'\n'})
				require.NoError(t, err)
			}

			defer file.Close()
			defer os.Remove(file.Name())

			scroll := component.NewScroll(b)
			scroll.Resize(10, 10)
			cursor := NewCursor(scroll, nil)

			uri, err := workspaceapi.CurrentUserHostURI(file.Name())
			require.NoError(t, err)

			swapDir, err := workspaceapi.CurrentUserHostURI("/tmp")
			require.NoError(t, err)

			// installs unix reader
			m := workspace.NewManager(config.NopConfig())
			require.NoError(t, err)
			err = m.RegisterScheme(workspace.FileScheme, workspace.NewFileScheme)
			require.NoError(t, err)
			workspace, err := m.AddWorkspace(context.Background(), cwdURI(t))
			require.NoError(t, err)
			_, err = workspace.Load(uri, b, swapDir, false)
			require.NoError(t, err)

			tcase.test(t, cursor)
		})
	}
}

func TestCursorPaste(t *testing.T) {
	/*
	   z
	   x
	*/
	/*
		a
		b
		c
		d

	*/
	const initialContent = "a\nb\nc\nd"
	tsuite := []struct {
		initialPosition     term.Coordinates
		expectedEndPosition term.Coordinates
		txt                 string
		mode                SelectMode
		after               bool
		expectedBuffer      string
	}{
		{term.Coordinates{}, term.Coordinates{X: 1},
			"z", NoSelection, false, "za\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{},
			"z\n", NoSelection, false, "z\na\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{Y: 1},
			"z\n", NoSelection, true, "a\nz\nb\nc\nd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3},
			"z\n", NoSelection, false, "a\nb\nc\nz\nd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{X: 1, Y: 3},
			"z\n", NoSelection, true, "a\nb\nc\nd\nz\n"},
		{term.Coordinates{}, term.Coordinates{X: 1},
			"z", StandardSelection, false, "za\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{X: 2},
			"z", StandardSelection, true, "az\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{X: 1, Y: 1},
			"z\nx", StandardSelection, false, "z\nxa\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{X: 1, Y: 1},
			"z\nx", StandardSelection, true, "az\nx\nb\nc\nd"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 3, X: 1},
			"z", StandardSelection, false, "a\nb\nc\nzd"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 3, X: 2},
			"z", StandardSelection, true, "a\nb\nc\ndz"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 4, X: 1},
			"z\nx", StandardSelection, false, "a\nb\nc\nz\nxd"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 4, X: 1},
			"z\nx", StandardSelection, true, "a\nb\nc\ndz\nx"},
		{term.Coordinates{X: 1}, term.Coordinates{},
			"z", LineSelection, false, "za\nb\nc\nd"},
		{term.Coordinates{X: 1}, term.Coordinates{Y: 1},
			"z", LineSelection, true, "a\nzb\nc\nd"},
		{term.Coordinates{X: 1}, term.Coordinates{},
			"z\nx", LineSelection, false, "z\nxa\nb\nc\nd"},
		{term.Coordinates{X: 1}, term.Coordinates{Y: 1},
			"z\nx", LineSelection, true, "a\nz\nxb\nc\nd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3},
			"z", LineSelection, false, "a\nb\nc\nzd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3, X: 1},
			"z", LineSelection, true, "a\nb\nc\nd\nz"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3},
			"z\nx", LineSelection, false, "a\nb\nc\nz\nxd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3, X: 1},
			"z\nx", LineSelection, true, "a\nb\nc\nd\nz\nx"},
		// block selection is like standard but with InsertBlock so we test that instead
	}

	for _, wrap := range []bool{false, true} {
		for i, tcase := range tsuite {
			t.Run(fmt.Sprintf("wrap: %v, %d", wrap, i), func(t *testing.T) {
				c := setupCursorContent(t, 5, 5, initialContent, wrap)
				c.MoveToScroll(tcase.initialPosition)
				c.Paste(tcase.txt, tcase.mode, tcase.after)
				assert.Equal(t, tcase.expectedBuffer, c.buffer().String())
				assert.Equal(t, tcase.expectedEndPosition, c.Coordinates())
			})
		}
	}
}

func TestPasteWithSelection(t *testing.T) {
	name := "no selection"
	for _, wrap := range []bool{false, true} {
		if wrap {
			name += " (wrap)"
		}
		t.Run(name, func(t *testing.T) {
			c := setupCursorContent(t, 4, 10, "abcdefgh\nijklmnopqrstuvxyz", true)
			c.MoveRightColumns(4)
			c.Paste("123", NoSelection, false)
			assert.Equal(t, "abcd123efgh\nijklmnopqrstuvxyz", c.buffer().String())
		})
	}

	name = "standard selection"
	for _, wrap := range []bool{false, true} {
		if wrap {
			name += " (wrap)"
		}
		t.Run(name, func(t *testing.T) {
			c := setupCursorContent(t, 4, 10, "abcdefgh\nijklmnopqrstuvxyz", wrap)
			c.MoveRightColumns(3)
			c.Select()
			c.MoveRightColumns(3)
			require.Equal(t, "defg", c.Selection())
			require.Equal(t, c.selection.mode, StandardSelection)
			c.Paste("123", StandardSelection, false)
			assert.Equal(t, "abc123h\nijklmnopqrstuvxyz", c.buffer().String())
		})
	}

	name = "line selection"
	for _, wrap := range []bool{false, true} {
		if wrap {
			name += " (wrap)"
		}
		t.Run(name, func(t *testing.T) {
			c := setupCursorContent(t, 4, 10, "abcdefgh\nijklmnopqrstuvxyz", false)
			c.MoveRightColumns(5)
			c.SelectLine()

			// NOTE: If PR #127 goes through this text will break because it will
			// select physical line instead of logical. So we would get "efgh".
			require.Equal(t, "abcdefgh\n", c.Selection())
			require.Equal(t, c.selection.mode, LineSelection)
			c.Paste("123", LineSelection, false)
			assert.Equal(t, "123\nijklmnopqrstuvxyz", c.buffer().String())
		})
	}

	name = "block selection"
	for _, wrap := range []bool{false, true} {
		if wrap {
			name += " (wrap)"
		}
		t.Run(name, func(t *testing.T) {
			c := setupCursorContent(t, 4, 10, "012\nabcd\n34567\n", false)
			c.MoveRight()
			c.SelectBlock()
			c.MoveDown()
			c.MoveDown()
			c.MoveRight()
			require.Equal(t, "12\nbc\n45", c.Selection())
			require.Equal(t, c.selection.mode, BlockSelection)
			c.Paste("XXX\nYYY\nZZZ", BlockSelection, false)
			assert.Equal(t, "0XXX\naYYYd\n3ZZZ67\n", c.buffer().String())
		})
	}
}

func TestCursorInsertBlock(t *testing.T) {
	const initialContent = "a\nb\nc"
	tsuite := []struct {
		initialPosition term.Coordinates
		txt             string
		expected        string
	}{
		{term.Coordinates{}, "a\nb\nc", "aa\nbb\ncc"},
		{term.Coordinates{Y: 1}, "a\nb\nc", "a\nab\nbc\nc"},
		{term.Coordinates{Y: 1, X: 1}, "a\nb\nc", "a\nba\ncb\n c"},
		{term.Coordinates{Y: 2}, "a\nb\nc", "a\nb\nac\nb\nc"},
		{term.Coordinates{Y: 2, X: 1}, "a\nb\nc", "a\nb\nca\n b\n c"},
	}

	for i, tcase := range tsuite {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			c := setupCursorContent(t, 5, 5, initialContent, false)
			c.MoveToScroll(tcase.initialPosition)
			c.InsertBlock(tcase.txt)
			assert.Equal(t, tcase.expected, c.buffer().String())
		})
	}
}

func TestCursorReplace(t *testing.T) {
	t.Run("non wrap", func(t *testing.T) {
		const initialContent = "a\nb\nc"
		c := setupCursorContent(t, 5, 5, initialContent, false)
		assert.Equal(t, term.Coordinates{X: 0, Y: 0}, c.cursor)

		c.Replace('X')
		assert.Equal(t, "X\nb\nc", c.buffer().String())
		assert.Equal(t, term.Coordinates{X: 0, Y: 0}, c.cursor)

		c.MoveDown()

		assert.Equal(t, term.Coordinates{X: 0, Y: 1}, c.cursor)
		c.Replace('Y')
		assert.Equal(t, "X\nY\nc", c.buffer().String())
		assert.Equal(t, term.Coordinates{X: 0, Y: 1}, c.cursor)

		c.MoveDown()
		assert.Equal(t, term.Coordinates{X: 0, Y: 2}, c.cursor)

		c.Replace('Z')
		assert.Equal(t, "X\nY\nZ", c.buffer().String())
		assert.Equal(t, term.Coordinates{X: 0, Y: 2}, c.cursor)

		c.Replace('A')
		assert.Equal(t, "X\nY\nA", c.buffer().String())
		assert.Equal(t, term.Coordinates{X: 0, Y: 2}, c.cursor)

		c.Replace('B')
		assert.Equal(t, "X\nY\nB", c.buffer().String())
		assert.Equal(t, term.Coordinates{X: 0, Y: 2}, c.cursor)
	})

	t.Run("wrap", func(t *testing.T) {
		const initialContent = "aaaaaaaaaaa\nb\nc"
		c := setupCursorContent(t, 5, 5, initialContent, true)

		for range 9 {
			c.MoveRight()
		}
		assert.Equal(t, term.Coordinates{X: 4, Y: 1}, c.cursor)

		c.Replace('X')
		assert.Equal(t, term.Coordinates{X: 4, Y: 1}, c.cursor)
		assert.Equal(t, "aaaaaaaaaXa\nb\nc", c.buffer().String())

		c.Replace('Y')
		assert.Equal(t, term.Coordinates{X: 4, Y: 1}, c.cursor)
		assert.Equal(t, "aaaaaaaaaYa\nb\nc", c.buffer().String())
	})
}

func TestCursorMoveRightWrap(t *testing.T) {
	t.Run("no seek horizontal", func(t *testing.T) {
		const initialContent = "1\n2"
		c := setupCursorContent(t, 3, 3, initialContent, false)

		cell, ok := c.Cell()
		require.True(t, ok)
		assert.Equal(t, '1', cell.Ch)

		assert.True(t, c.MoveRightWrap())
		assert.Equal(t, term.Coordinates{X: 1}, c.CursorAtScroll())
		assert.True(t, c.MoveRightWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 0}, c.CursorAtScroll())
		assert.True(t, c.MoveRightWrap())
		assert.False(t, c.MoveRightWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, c.CursorAtScroll())
	})

	t.Run("with seek horizontal", func(t *testing.T) {
		const initialContent = "11\n22"
		c := setupCursorContent(t, 1, 1, initialContent, false)

		cell, ok := c.Cell()
		require.True(t, ok)
		assert.Equal(t, '1', cell.Ch)

		assert.True(t, c.MoveRightWrap())
		assert.Equal(t, term.Coordinates{X: 1}, c.CursorAtScroll())
		assert.True(t, c.MoveRightWrap())
		assert.True(t, c.MoveRightWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 0}, c.CursorAtScroll())
		assert.True(t, c.MoveRightWrap())
		assert.True(t, c.MoveRightWrap())
		assert.False(t, c.MoveRightWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 2}, c.CursorAtScroll())
	})
}

func TestCursorMoveLeftWrap(t *testing.T) {
	t.Run("no seek horizontal", func(t *testing.T) {
		const initialContent = "1\n2"
		c := setupCursorContent(t, 3, 3, initialContent, false)

		c.MoveToScroll(term.Coordinates{Y: 2, X: 2})

		assert.Equal(t, term.Coordinates{Y: 2, X: 2}, c.CursorAtScroll())
		assert.True(t, c.MoveLeftWrap())
		assert.True(t, c.MoveLeftWrap())
		assert.True(t, c.MoveLeftWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, c.CursorAtScroll())
		assert.True(t, c.MoveLeftWrap())
		assert.True(t, c.MoveLeftWrap())
		assert.True(t, c.MoveLeftWrap())
		assert.False(t, c.MoveLeftWrap())
		assert.Equal(t, term.Coordinates{}, c.CursorAtScroll())
	})

	t.Run("with seek horizontal", func(t *testing.T) {
		const initialContent = "11\n22"
		c := setupCursorContent(t, 1, 1, initialContent, false)

		c.MoveToScroll(term.Coordinates{Y: 2, X: 0})

		assert.True(t, c.MoveLeftWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 2}, c.CursorAtScroll())
		assert.True(t, c.MoveLeftWrap())
		assert.True(t, c.MoveLeftWrap())
		assert.Equal(t, term.Coordinates{Y: 1, X: 0}, c.CursorAtScroll())
		assert.True(t, c.MoveLeftWrap())
		assert.True(t, c.MoveLeftWrap())
		assert.Equal(t, term.Coordinates{X: 1}, c.CursorAtScroll())
		assert.True(t, c.MoveLeftWrap())
		assert.False(t, c.MoveLeftWrap())
		assert.Equal(t, term.Coordinates{}, c.CursorAtScroll())
	})
}

type testScrollSubscriber struct {
	hide    int
	visible int
	seek    int
}

func (s *testScrollSubscriber) OnWillSeek(from term.Coordinates) {
	/* no op */
}

func (s *testScrollSubscriber) OnWillHide(start, end int) {
}

func (s *testScrollSubscriber) OnWillVisible(start int) {
}

func (s *testScrollSubscriber) OnDidHide(start, end int) {
	s.hide++
}

func (s *testScrollSubscriber) OnDidVisible(start int) {
	s.visible++
}

func (s *testScrollSubscriber) OnDidSeek(from, to term.Coordinates) {
	s.seek++
}

var _ = (foldsService)(testFoldsService{})

type testFoldsService struct {
	folds iterator.Iterator[term.Range]
	view  cell.View
}

func (f testFoldsService) Rows() int {
	return f.view.Rows()
}

func (f testFoldsService) Columns(row int) int {
	return f.view.Columns(row)
}

func (f testFoldsService) Cell(at term.Coordinates) (term.Cell, bool) {
	return f.view.Cell(at)
}

func (f testFoldsService) RawCells() [][]term.Cell {
	return f.view.RawCells()
}

func (f testFoldsService) String() string {
	return f.view.String()
}

func (f testFoldsService) FoldsFrom(pos term.Coordinates) (
	iterator.Iterator[term.Range], bool,
) {
	folds, ok := f.Folds()
	if !ok {
		return nil, false
	}
	return iterator.Filter(folds, func(rng term.Range) bool {
		return (rng.End.Y > pos.Y || (rng.End.Y == pos.Y && rng.End.X > pos.X))
	}), true
}

func (f testFoldsService) Folds() (iterator.Iterator[term.Range], bool) {
	if f.folds != nil {
		return f.folds, true
	}
	return iterator.FromSlice([]term.Range{
		{Start: term.Coordinates{Y: 1, X: 0}, End: term.Coordinates{Y: 4}},
		{Start: term.Coordinates{Y: 2, X: 3}, End: term.Coordinates{Y: 3, X: 15}},
		{Start: term.Coordinates{Y: 7, X: 0}, End: term.Coordinates{Y: 31, X: 0}},
		{Start: term.Coordinates{Y: 12, X: 3}, End: term.Coordinates{Y: 28, X: 3}},
		{Start: term.Coordinates{Y: 19, X: 6}, End: term.Coordinates{Y: 27, X: 6}},
		{Start: term.Coordinates{Y: 22, X: 6}, End: term.Coordinates{Y: 26, X: 9}},
		{Start: term.Coordinates{Y: 29, X: 3}, End: term.Coordinates{Y: 30, X: 3}},
	}), true
}

func newBenchmarkScroll(width, height int, fortunes int) (scroll *component.Scroll) {
	scroll = component.NewScroll(cell.NewBuffer())
	for i := 0; i < fortunes; i++ {
		_, _ = scroll.Buffer().ReadFrom(strings.NewReader(sampleSnippet))
	}
	scroll.Resize(width, height)
	return
}

func benchmarkCursorMoveLeft(b *testing.B, width, height int, wrap bool) {
	s := newBenchmarkScroll(width, height, 10000)
	s.Wrap = wrap
	cursor := NewCursor(s, nil)
	cursor.MoveLastLine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ok := cursor.MoveLeftWrap()
		if !ok {
			cursor.MoveLastLine()
		}
	}
}

func benchmarkCursorMoveMatchingRune(b *testing.B, width, height int, wrap bool) {
	s := newBenchmarkScroll(width, height, 1)
	s.Wrap = wrap
	cursor := NewCursor(s, nil)
	cursor.MoveToMark(CursorMark{scroll: term.Coordinates{Y: 7}})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor.MoveToMatchingRune()
	}
}

func BenchmarkCursorMoveMatchingRuneLargeWindowNoWrap(b *testing.B) {
	benchmarkCursorMoveMatchingRune(b, 1000, 1000, false)
}

func BenchmarkCursorMoveMatchingRuneLargeWindowWrap(b *testing.B) {
	benchmarkCursorMoveMatchingRune(b, 1000, 1000, true)
}

func BenchmarkCursorMoveMatchingRuneSmallWindowWrap(b *testing.B) {
	benchmarkCursorMoveMatchingRune(b, 10, 10, true)
}

func BenchmarkCursorMoveMatchingRuneSmallWindowNoWrap(b *testing.B) {
	benchmarkCursorMoveMatchingRune(b, 10, 10, false)
}

func BenchmarkCursorMoveLeftNoWrap(b *testing.B) {
	benchmarkCursorMoveLeft(b, 1000, 1000, false)
}

func BenchmarkCursorMoveLeftWrap(b *testing.B) {
	benchmarkCursorMoveLeft(b, 10, 10, true)
}

func benchmarkCursorMoveToRune(b *testing.B, width, height int, wrap bool) {
	s := newBenchmarkScroll(width, height, 100)
	s.Wrap = wrap
	cursor := NewCursor(s, nil)
	cursor.MoveToMark(CursorMark{scroll: term.Coordinates{Y: 7}})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor.MoveRightStartWordGroup()
		cursor.MoveLeftStartWordGroup()
		cursor.MoveRightEndWordGroup()
		cursor.MoveLeftEndWordGroup()
		cursor.MoveRightStartWord()
		cursor.MoveLeftStartWord()
		cursor.MoveRightEndWord()
		cursor.MoveLeftEndWord()
	}
}

func BenchmarkCursorMoveToRuneLargeWrap(b *testing.B) {
	benchmarkCursorMoveToRune(b, 10000, 10000, true)
}
func BenchmarkCursorMoveToRuneLargeNoWrap(b *testing.B) {
	benchmarkCursorMoveToRune(b, 10000, 10000, false)
}
func BenchmarkCursorMoveToRuneSmallWrap(b *testing.B) {
	benchmarkCursorMoveToRune(b, 10, 10, true)
}
func BenchmarkCursorMoveToRuneSmallNoWrap(b *testing.B) {
	benchmarkCursorMoveToRune(b, 10, 10, false)
}

const locID = "errors"
const sampleSnippet = `
/*
 * Ch@ek if the current buffer should be added to or removed from the list of
 * diff buffers.
 */
	void
diff_buf_adjust(win_T *win)
{
	win_T	*wp;
	int		i;

	if (!win->w_p_diff)
	{
	/* When there is no window showing a diff for this buffer, remove
	 * it from the diffs. */
	FOR_ALL_WINDOWS(wp)
		if (wp->w_buffer == win->w_buffer && wp->w_p_diff)
		break;
	if (wp == NULL)
	{
		i = diff_buf_idx(win->w_buffer);
		if (i != DB_COUNT)
		{X
		curtab->tp_diffbuf[i] = NULL;
		curtab->tp_diff_invalid = TRUE;
		diff_redraw(TRUE);
		}
	}
	}
	else
	diff_buf_add(win->w_buffer);
} /* { */ `

func setupCursorContent(t *testing.T, width, height int, cont string, wrap bool) (e *Cursor) {
	scroll := component.NewScroll(cell.NewBuffer())
	e = NewCursor(scroll, nil)
	e.RightInclusiveSemantics = true
	scroll.Wrap = wrap
	scroll.Buffer().ReadFrom(strings.NewReader(cont))
	scroll.Resize(width, height)
	require.Equal(t, e.scroll.Buffer(), scroll.Buffer())
	if wrap {
		// needed for wraps to be accounted for
		e.scroll.RecalculateWraps()
	}
	return
}

func setupCursor(t *testing.T, width, height int, wrap bool) *Cursor {
	return setupCursorContent(t, width, height, sampleSnippet, wrap)
}

func setupCursorForFolds(t *testing.T, width, height int, wg *sync.WaitGroup) (e *Cursor) {
	buf := cell.NewBuffer()
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	e = NewCursor(scroll, func(fn func()) bool {
		defer wg.Done()
		fn()
		return true
	})
	e.RightInclusiveSemantics = true
	scroll.Buffer().ReadFrom(strings.NewReader(sampleSnippet))
	scroll.Resize(width, height)
	require.Equal(t, e.scroll.Buffer(), scroll.Buffer())
	return
}

type mockIndentService struct {
	cell.View
	returnIndentationAt int
}

func (m *mockIndentService) IndentationAt(line int) (int, bool) {
	return m.returnIndentationAt, true
}
