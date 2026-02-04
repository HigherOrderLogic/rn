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

package vtescreen

import (
	"testing"

	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/term/vte/vteparser"

	"github.com/stretchr/testify/assert"
)

func TestNewAltBuffer(t *testing.T) {
	b := NewAltBuffer()
	assert.Equal(t, 1, b.Width())
	assert.Equal(t, 1, b.Height())
	assert.Equal(t, 0, b.TopScrollableRegion())
	assert.Equal(t, 1, b.BottomScrollableRegion())
	assert.Equal(t, 0, b.CursorAtScreen().X)
	assert.Equal(t, 0, b.CursorAtScreen().Y)
	require.NotNil(t, b.Cells.RawCells())
	assert.NotNil(t, b.Cursor().Charsets)

	assert.Equal(t, [][]term.Cell{
		{{Ch: DefaultChar, Width: 1}},
	}, b.Cells.RawCells())

	// rendering corresponds to stored cells
	// this is important so writes to buffer are correct

	b.Resize(4, 4)
	b.Insert('\t', 1, 0)
	b.SetCursorAtScreen(term.Coordinates{X: 1}, false)
	b.Insert('a', 1, 0)
	w := term.NewStringWriter(4, 4)
	b.Draw(w)
	w.Flush()

	assert.Equal(t, " a  \n    \n    \n    ", w.String())
}

func TestResize(t *testing.T) {
	t.Run("resize up", func(t *testing.T) {
		b := NewAltBuffer()
		b.defaultChar = 'X'
		b.Resize(2, 2)
		assert.Equal(t, 2, b.Width())
		assert.Equal(t, 2, b.Height())
		assert.Equal(t, 0, b.TopScrollableRegion())
		assert.Equal(t, 2, b.BottomScrollableRegion())
		require.NotNil(t, b.Cells.RawCells())

		assert.Equal(t, [][]term.Cell{
			{{Ch: 'X', Width: 1}, {Ch: 'X', Width: 1}},
			{{Ch: 'X', Width: 1}, {Ch: 'X', Width: 1}},
		}, b.Cells.RawCells())
	})
	t.Run("resize down", func(t *testing.T) {
		b := NewAltBuffer()
		b.Resize(3, 3)
		b.defaultChar = 'X'
		b.Resize(2, 2)
		assert.Equal(t, 2, b.Width())
		assert.Equal(t, 2, b.Height())
		assert.Equal(t, 0, b.TopScrollableRegion())
		assert.Equal(t, 2, b.BottomScrollableRegion())
		require.NotNil(t, b.Cells.RawCells())

		assert.Equal(t, [][]term.Cell{
			{{Ch: 'X', Width: 1}, {Ch: 'X', Width: 1}},
			{{Ch: 'X', Width: 1}, {Ch: 'X', Width: 1}},
		}, b.Cells.RawCells())
	})
}

func TestResetLines(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb")
		assertEqualBuf(t, b, "a    \nb    \n     \n     \n     ")

		b.ResetLines(0, 5)
		assertEqualBuf(t, b, "     \n     \n     \n     \n     ")
	})
	t.Run("subset of lines at start", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb")
		assertEqualBuf(t, b, "a    \nb    \n     \n     \n     ")

		b.ResetLines(0, 1)
		assertEqualBuf(t, b, "     \nb    \n     \n     \n     ")
	})
	t.Run("subset of lines in the middle", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb")
		assertEqualBuf(t, b, "a    \nb    \n     \n     \n     ")

		b.ResetLines(1, 2)
		assertEqualBuf(t, b, "a    \n     \n     \n     \n     ")
	})
	t.Run("subset of lines at the end", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		b.ResetLines(4, 5)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \n     ")
	})
	t.Run("past last line does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		b.ResetLines(5, 6)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")
	})
	t.Run("start > end does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		b.ResetLines(3, 2)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")
	})
	t.Run("start == end does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		b.ResetLines(0, 0)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")
	})
	t.Run("negative start does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		b.ResetLines(-1, 2)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")
	})
	t.Run("negative end does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		b.ResetLines(1, -2)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")
	})
}

func TestResetCells(t *testing.T) {
	t.Run("reset after content does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb")
		assertEqualBuf(t, b, "a    \nb    \n     \n     \n     ")

		b.ResetCells(1, 5)
		assertEqualBuf(t, b, "a    \nb    \n     \n     \n     ")
	})

	t.Run("reset one", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb")
		assertEqualBuf(t, b, "a    \nb    \n     \n     \n     ")

		b.ResetCells(0, 1)
		assertEqualBuf(t, b, "a    \n     \n     \n     \n     ")
	})

	t.Run("reset with cursor after height does not panic", func(t *testing.T) {
		b := makeAltBufferForTesting(5, 5)
		writeToAltBuffer(b, "a\nb\nc\nd\ne\n")
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    ")

		// this shouldn't happen if AltBuffer is used correctly
		// but be resilient against misuse
		b.cursor.position.Y = 5
		b.ResetCells(0, 1)
		assertEqualBuf(t, b, "a    \nb    \nc    \nd    \ne    \n     ")
	})
}

func TestScrollUp(t *testing.T) {
	t.Run("count zero does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 10, 0)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
	})

	t.Run("all once", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 10, 1)
		assertEqualBuf(t, b, "1\n2\n3\n4\n5\n6\n7\n8\n9\n ")
	})
	t.Run("all multi ", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 10, 2)
		assertEqualBuf(t, b, "2\n3\n4\n5\n6\n7\n8\n9\n \n ")
	})
	t.Run("all exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 10, 10)
		assertEqualBuf(t, b, " \n \n \n \n \n \n \n \n \n ")
	})
	t.Run("all more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 10, 11)
		assertEqualBuf(t, b, " \n \n \n \n \n \n \n \n \n ")
	})

	t.Run("top once ", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 3, 1)
		assertEqualBuf(t, b, "1\n2\n \n3\n4\n5\n6\n7\n8\n9")
	})
	t.Run("top multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 3, 2)
		assertEqualBuf(t, b, "2\n \n \n3\n4\n5\n6\n7\n8\n9")
	})
	t.Run("top exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 3, 3)
		assertEqualBuf(t, b, " \n \n \n3\n4\n5\n6\n7\n8\n9")
	})
	t.Run("top more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(0, 3, 4)
		assertEqualBuf(t, b, " \n \n \n3\n4\n5\n6\n7\n8\n9")
	})

	t.Run("mid once ", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(1, 4, 1)
		assertEqualBuf(t, b, "0\n2\n3\n \n4\n5\n6\n7\n8\n9")
	})
	t.Run("mid multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(1, 4, 2)
		assertEqualBuf(t, b, "0\n3\n \n \n4\n5\n6\n7\n8\n9")
	})
	t.Run("mid exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(1, 4, 3)
		assertEqualBuf(t, b, "0\n \n \n \n4\n5\n6\n7\n8\n9")
	})
	t.Run("mid more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(1, 4, 4)
		assertEqualBuf(t, b, "0\n \n \n \n4\n5\n6\n7\n8\n9")
	})

	t.Run("bottom once ", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(7, 10, 1)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n8\n9\n ")
	})
	t.Run("bottom multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(7, 10, 2)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n9\n \n ")
	})
	t.Run("bottom exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(7, 10, 3)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n \n \n ")
	})
	t.Run("bottom more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollUp(7, 10, 4)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n \n \n ")
	})

	t.Run("once all but top line", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "a\nb\n \n \n \n \n \n \n \n ")
		b.ScrollUp(1, 10, 1)
		assertEqualBuf(t, b, "a\n \n \n \n \n \n \n \n \n ")
	})
}

func TestScrollDown(t *testing.T) {
	t.Run("count zero does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 10, 0)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
	})

	t.Run("all once", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 10, 1)
		assertEqualBuf(t, b, " \n0\n1\n2\n3\n4\n5\n6\n7\n8")
	})
	t.Run("all multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 10, 2)
		assertEqualBuf(t, b, " \n \n0\n1\n2\n3\n4\n5\n6\n7")
	})
	t.Run("all exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 10, 10)
		assertEqualBuf(t, b, " \n \n \n \n \n \n \n \n \n ")
	})
	t.Run("all more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 10, 11)
		assertEqualBuf(t, b, " \n \n \n \n \n \n \n \n \n ")
	})

	t.Run("top once", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 3, 1)
		assertEqualBuf(t, b, " \n0\n1\n3\n4\n5\n6\n7\n8\n9")
	})
	t.Run("top multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 3, 2)
		assertEqualBuf(t, b, " \n \n0\n3\n4\n5\n6\n7\n8\n9")
	})
	t.Run("top exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 3, 3)
		assertEqualBuf(t, b, " \n \n \n3\n4\n5\n6\n7\n8\n9")
	})
	t.Run("top more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(0, 3, 4)
		assertEqualBuf(t, b, " \n \n \n3\n4\n5\n6\n7\n8\n9")
	})

	t.Run("mid once", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(1, 4, 1)
		assertEqualBuf(t, b, "0\n \n1\n2\n4\n5\n6\n7\n8\n9")
	})
	t.Run("mid multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(1, 4, 2)
		assertEqualBuf(t, b, "0\n \n \n1\n4\n5\n6\n7\n8\n9")
	})
	t.Run("mid exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(1, 4, 3)
		assertEqualBuf(t, b, "0\n \n \n \n4\n5\n6\n7\n8\n9")
	})
	t.Run("mid more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(1, 4, 4)
		assertEqualBuf(t, b, "0\n \n \n \n4\n5\n6\n7\n8\n9")
	})

	t.Run("bottom once ", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(7, 10, 1)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n \n7\n8")
	})
	t.Run("bottom multi", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(7, 10, 2)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n \n \n7")
	})
	t.Run("bottom exactly length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(7, 10, 3)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n \n \n ")
	})
	t.Run("bottom more than length", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
		b.ScrollDown(7, 10, 4)
		assertEqualBuf(t, b, "0\n1\n2\n3\n4\n5\n6\n \n \n ")
	})
	t.Run("once all but top line", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "a\nb\n \n \n \n \n \n \n \n ")
		b.ScrollDown(1, 10, 1)
		assertEqualBuf(t, b, "a\n \nb\n \n \n \n \n \n \n ")
	})
}

func TestDelete(t *testing.T) {
	t.Run("deletes one character", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "a\nb\n \n \n \n \n \n \n \n ")
		b.SetCursorAtScreen(term.Coordinates{}, false)
		b.Delete(1)
		assertEqualBuf(t, b, " \nb\n \n \n \n \n \n \n \n ")
	})

	t.Run("deletes partial start of row", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)
		b.Delete(1)
		assertEqualBuf(t, b, "a \nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("deletes partial end of row", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{X: 1}, false)
		b.Delete(1)
		assertEqualBuf(t, b, "a \nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("deletes multiple characters", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)
		b.Delete(2)
		assertEqualBuf(t, b, "  \nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("past last column does nothing past last column", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)
		b.Delete(3)
		assertEqualBuf(t, b, "  \nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("count + pos.X oob", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{X: 1}, false)
		b.Delete(2)
		assertEqualBuf(t, b, "a \nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("zero count does nothing", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)
		b.Delete(0)
		assertEqualBuf(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("deletes first few characters in long line", func(t *testing.T) {
		b := makeAltBufferForTesting(10, 2)
		resetAltBuffer(t, b, "0123456789\nabcdefghij")
		b.SetCursorAtScreen(term.Coordinates{X: 2}, false)
		b.Delete(3)
		assertEqualBuf(t, b, "0156789   \nabcdefghij")
	})
}

func TestWriteInsert(t *testing.T) {
	t.Run("maps character to ' ' if SetHiddenCursor(true) was called", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)
		b.SetHiddenCursor(true)
		b.Write('X', 1, 0)
		assertEqualBuf(t, b, " a\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetHiddenCursor(false)
		b.Write('X', 1, 0)
		assertEqualBuf(t, b, "Xa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("copies the attributes last set via SetCursorAttributes", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)

		attrs := term.Attributes{
			Fg:    tcell.ColorYellow,
			Bg:    tcell.ColorBlue,
			Attrs: tcell.AttrItalic,
		}
		b.SetCursorAttributes(attrs)
		b.Write('X', 1, 0)
		assertEqualBuf(t, b, "Xa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")

		cell := b.Cells.RawCells()[0][0]
		assert.Equal(t, attrs, cell.Attributes)
	})

	t.Run("maps the character using the given charset index", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)

		b.ConfigureCharset(vteparser.CharsetIndexG1, vteparser.StandardCharsetSpecialCharacterAndLineDrawing)
		b.Write('`', 1, vteparser.CharsetIndexG1)
		assertEqualBuf(t, b, "◆a\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("does nothing if passed charset has not been configured", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{}, false)

		b.Write('`', 1, vteparser.CharsetIndexG1)
		assertEqualBuf(t, b, "`a\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("writes character at max column if if out of bounds (x)", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{X: 4}, false)

		b.Write('X', 1, 0)
		assertEqualBuf(t, b, "aX\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("inserts character at max column if writing out of bounds (x)", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{X: 4}, false)

		b.Write('X', 1, 0)
		assertEqualBuf(t, b, "aX\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("inserts character at max line if writing out of bounds (y)", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{Y: 12, X: 6}, false)

		b.Write('X', 1, 0)
		assertEqualBuf(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n X")
	})

	t.Run("inserts character in bounds", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
		b.SetCursorAtScreen(term.Coordinates{X: 1}, false)

		b.Insert('X', 1, 0)
		assertEqualBuf(t, b, "aX\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")
	})

	t.Run("insert at pos guarantees that CellAt does not return nil", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")

		b.SetCursorAtScreen(term.Coordinates{Y: 10}, false)
		// invalidate cursor position
		b.Resize(2, 9)

		require.NotPanics(t, func() {
			b.Insert('X', 1, 0)
		})
		assertEqualBuf(t, b, "  \n  \n  \n  \n  \n  \n  \n  \n  \nX")
	})

	t.Run("write at pos guarantees that CellAt does not return nil", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "aa\nbb\n  \n  \n  \n  \n  \n  \n  \n  ")

		b.SetCursorAtScreen(term.Coordinates{Y: 10}, false)
		// invalidate cursor position
		b.Resize(2, 9)

		require.NotPanics(t, func() {
			b.Write('X', 1, 0)
		})
		assertEqualBuf(t, b, "  \n  \n  \n  \n  \n  \n  \n  \n  \nX")
	})
}

func TestAltSelection(t *testing.T) {
	t.Run("no selection returns false", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")

		_, ok := b.Selection()
		require.False(t, ok)
	})
	t.Run("select start end are zero select one cell", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")

		b.Select(term.Coordinates{})
		b.SelectEnd(term.Coordinates{})
		cells, ok := b.Selection()
		require.True(t, ok)
		assert.Equal(t, [][]term.Cell{{{Ch: '0', Width: 1}}}, cells)
	})
	t.Run("multiline select standard single character lines", func(t *testing.T) {
		b := makeAltBufferForTesting(1, 10)
		resetAltBuffer(t, b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")

		b.Select(term.Coordinates{})
		b.SelectEnd(term.Coordinates{Y: 1})
		cells, ok := b.Selection()
		require.True(t, ok)
		assert.Equal(t, [][]term.Cell{{{Ch: '0', Width: 1}}, {{Ch: '1', Width: 1}}}, cells)
	})
	t.Run("multiline select standard multiple character lines", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "00\n11\n22\n33\n44\n55\n66\n77\n88\n99")

		b.Select(term.Coordinates{})
		b.SelectEnd(term.Coordinates{Y: 1})
		cells, ok := b.Selection()
		require.True(t, ok)
		assert.Equal(t, [][]term.Cell{{{Ch: '0', Width: 1}, {Ch: '0', Width: 1}}, {{Ch: '1', Width: 1}}}, cells)
	})
	t.Run("multiline select line", func(t *testing.T) {
		b := makeAltBufferForTesting(2, 10)
		resetAltBuffer(t, b, "00\n11\n22\n33\n44\n55\n66\n77\n88\n99")

		b.SelectLine(term.Coordinates{})
		b.SelectEnd(term.Coordinates{Y: 1})
		cells, ok := b.Selection()
		require.True(t, ok)
		assert.Equal(t, [][]term.Cell{{{Ch: '0', Width: 1}, {Ch: '0', Width: 1}},
			{{Ch: '1', Width: 1}, {Ch: '1', Width: 1}}, {}}, cells)
	})
}

func makeAltBufferForTesting(width, height int) *AltBuffer {
	ret := NewAltBuffer()
	ret.defaultChar = ' '
	// re-init cells with default char set to space
	ret.Cells.InitPerformance(120, 80, ret.defaultChar)
	ret.Resize(width, height)
	return ret
}

func writeToAltBuffer(b *AltBuffer, str string) {
	for _, ch := range str {
		pos := b.CursorAtScreen()
		if ch == '\n' {
			pos.Y++
			pos.X = 0
			b.SetCursorAtScreen(pos, false)
		} else {
			b.Write(ch, uniseg.StringWidth(string(ch)), 0)
			pos.X++
			b.SetCursorAtScreen(pos, false)
		}
	}
}

func assertEqualBuf(t *testing.T, p *AltBuffer, expected string) {
	assert.Equal(t, expected, term.CellsToString(p.Cells.RawCells()))
}

func resetAltBuffer(t *testing.T, b *AltBuffer, to string) {
	b.ResetLines(0, b.Height())
	b.SetCursorAtScreen(term.Coordinates{}, false)
	writeToAltBuffer(b, to)
	require.Equal(t, to, term.CellsToString(b.Cells.RawCells()))
}
