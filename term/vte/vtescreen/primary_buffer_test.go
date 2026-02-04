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
	"fmt"
	"testing"

	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

const testHistory = 1000

func TestPrimaryReset(t *testing.T) {
	t.Run("exactly screen height compared to amount of rows", func(t *testing.T) {
		b := NewPrimaryBuffer(0, testHistory)
		b.Resize(5, 5)
		resetPrimaryBuffer(b, "0\n1\n2\n3\n4")

		assert.Equal(t, 5, b.Rows())
		assert.Equal(t, 5, b.Columns(0))
		assert.Equal(t, 5, b.Columns(1))
		assert.Equal(t, 5, b.Columns(2))
		assert.Equal(t, 5, b.Columns(3))
		assert.Equal(t, 5, b.Columns(4))
		assert.Equal(t, 0, b.Columns(5))

		b.Reset()
		assert.Equal(t, 5, b.Rows())
		assert.Equal(t, 5, b.Columns(0))
		assert.Equal(t, 5, b.Columns(1))
		assert.Equal(t, 5, b.Columns(2))
		assert.Equal(t, 5, b.Columns(3))
		assert.Equal(t, 5, b.Columns(4))
		assert.Equal(t, 0, b.Columns(5))
	})

	t.Run("smaller screen height compared to amount of rows", func(t *testing.T) {
		b := NewPrimaryBuffer(0, testHistory)
		b.Resize(5, 2)
		assert.Equal(t, 2, b.Rows())
		resetPrimaryBuffer(b, "0\n1\n2\n3\n4")

		assert.Equal(t, 5, b.Rows())
		assert.Equal(t, 5, b.Columns(0))
		assert.Equal(t, 5, b.Columns(1))
		assert.Equal(t, 5, b.Columns(2))
		assert.Equal(t, 5, b.Columns(3))
		assert.Equal(t, 5, b.Columns(4))
		assert.Equal(t, 0, b.Columns(5))

		b.Reset()
		assert.Equal(t, 2, b.Rows())
		assert.Equal(t, 5, b.Columns(0))
		assert.Equal(t, 5, b.Columns(1))
		assert.Equal(t, 0, b.Columns(2))
	})
}

func TestPrimaryCoordinates(t *testing.T) {
	b := NewPrimaryBuffer(0, testHistory)
	assert.Equal(t, term.Coordinates{}, b.CursorAtScreen())
	assert.Equal(t, term.Coordinates{}, b.CursorAtScroll())

	b.Resize(5, 5)
	assert.Equal(t, term.Coordinates{}, b.CursorAtScroll())
	assert.Equal(t, term.Coordinates{}, b.CursorAtScreen())

	b.SetCursorAtScreen(term.Coordinates{Y: 4, X: 4}, false)
	assert.Equal(t, term.Coordinates{Y: 4, X: 4}, b.CursorAtScroll())
	assert.Equal(t, term.Coordinates{Y: 4, X: 4}, b.CursorAtScreen())

	b.SetCursorAtScreen(term.Coordinates{Y: 5, X: 5}, false)
	assert.Equal(t, term.Coordinates{Y: 4, X: 4}, b.CursorAtScroll())
	assert.Equal(t, term.Coordinates{Y: 4, X: 4}, b.CursorAtScreen())
}

func TestPrimarySelection(t *testing.T) {
	t.Run("select with scroll", func(t *testing.T) {
		b := makePrimaryBufferForTesting(1, 5)
		resetPrimaryBuffer(b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")

		b.Select(term.Coordinates{Y: 1})
		b.SelectEnd(term.Coordinates{Y: 2})
		actual, ok := b.Selection()
		require.True(t, ok)
		expected := [][]term.Cell{{{Ch: '6', Width: 1}}, {{Ch: '7', Width: 1}}}
		assert.Equal(t, expected, actual)
	})
	t.Run("word selection", func(t *testing.T) {
		b := makePrimaryBufferForTesting(2, 10)
		resetPrimaryBuffer(b, "00\n11\n22\n33\n44\n55\n66\n77\n88\n99")

		b.SelectWordAt(term.Coordinates{Y: 99})
		_, ok := b.Selection()
		require.False(t, ok)

		b.SelectWordAt(term.Coordinates{Y: 2})
		actual, ok := b.Selection()
		require.True(t, ok)
		expected := [][]term.Cell{{{Ch: '2', Width: 1}, {Ch: '2', Width: 1}}}
		assert.Equal(t, expected, actual)
	})

	t.Run("coordinates with scroll", func(t *testing.T) {
		b := makePrimaryBufferForTesting(1, 5)
		resetPrimaryBuffer(b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")

		b.Select(term.Coordinates{Y: 1})
		b.SelectEnd(term.Coordinates{Y: 2})

		mode, from, to, ok := b.SelectionCoordinatesAtScreen()
		require.True(t, ok)
		assert.Equal(t, mode, text.StandardSelection)
		assert.Equal(t, term.Coordinates{Y: 1}, from)
		assert.Equal(t, term.Coordinates{Y: 2, X: 1}, to)

		_, from, to, ok = b.SelectionCoordinatesAtScroll()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{Y: 6}, from)
		assert.Equal(t, term.Coordinates{Y: 7, X: 1}, to)
	})
}

func TestPrimaryClear(t *testing.T) {
	suite := []struct {
		width, height   int
		content         string
		expectedContent string
		expectedView    string
	}{
		{2, 2, "", "  \n  ", "  \n  "},
		{2, 2, "$ \n  ", "$ \n  \n  ", "  \n  "},
		{2, 2, "a \n$ ", "a \n$ \n  \n  ", "  \n  "},
		{2, 2, "b \na \n$ ", "b \na \n$ \n  \n  ", "  \n  "},
		{2, 2, "c \nb \na \n$ ", "c \nb \na \n$ \n  \n  ", "  \n  "},
		{2, 2, "c \nb \na \n$ \n  \n  \n  ", "c \nb \na \n$ \n  \n  \n  ", "  \n  "},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			b := makePrimaryBufferForTesting(test.width, test.height)
			resetPrimaryBuffer(b, test.content)

			initialCursor := b.CursorAtScreen()
			cleared := b.Clear()
			writer := term.NewStringWriter(test.width, test.height)
			b.Draw(writer)
			writer.Flush()
			assert.Equal(t, test.expectedView, writer.String(), "view is incorrect")
			if cleared {
				assert.Equal(t, term.Coordinates{}, b.CursorAtScreen(), "cursor is incorrect")
			} else {
				assert.Equal(t, initialCursor, b.CursorAtScreen(), "cursor is incorrect")
			}
			assert.Equal(t, test.expectedContent, b.Cells.String(), "content is incorrect")
		})
	}
}

func TestPrimaryClearHistory(t *testing.T) {
	suite := []struct {
		width, height   int
		content         string
		expectedContent string
		expectedView    string
	}{
		{2, 2, "", "  \n  ", "  \n  "},
		{2, 2, "$ \n  ", "$ \n  ", "$ \n  "},
		{2, 2, "a \n$ ", "a \n$ ", "a \n$ "},
		{2, 2, "b \na \n$ ", "a \n$ ", "a \n$ "},
		{2, 2, "c \nb \na \n$ ", "a \n$ ", "a \n$ "},
		{2, 2, "c \nb \na \n$ \n  \n  \n  ", "  \n  ", "  \n  "},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			b := makePrimaryBufferForTesting(test.width, test.height)
			resetPrimaryBuffer(b, test.content)

			initialCursor := b.CursorAtScreen()
			b.ClearHistory()
			writer := term.NewStringWriter(test.width, test.height)
			b.Draw(writer)
			writer.Flush()
			assert.Equal(t, test.expectedView, writer.String(), "view is incorrect")
			assert.Equal(t, initialCursor, b.CursorAtScreen(), "cursor is incorrect")
			assert.Equal(t, test.expectedContent, b.Cells.String(), "content is incorrect")
		})
	}
}

func TestPrimaryResize(t *testing.T) {
	t.Run("maintains number of rows if resize is equal height", func(t *testing.T) {

		t.Run("buffer with more lines than height", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 10)
			assert.Equal(t, '9', cells[9][0].Ch)

			b.Resize(2, 5)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 10)
			assert.Equal(t, '9', cells[9][0].Ch)
		})

		t.Run("buffer with less lines than height", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n \n ")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '2', cells[2][0].Ch)

			b.Resize(2, 5)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '2', cells[2][0].Ch)
		})

		t.Run("buffer with exactly height lines", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n3\n4")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '4', cells[4][0].Ch)

			b.Resize(2, 5)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '4', cells[4][0].Ch)
		})
	})

	t.Run("potentially trims number of rows if height is decreased", func(t *testing.T) {

		t.Run("buffer with more lines than height", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 10)
			assert.Equal(t, '9', cells[9][0].Ch)

			b.Resize(2, 4)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 10)
			assert.Equal(t, '9', cells[9][0].Ch)
		})

		t.Run("buffer with less lines than height", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '2', cells[2][0].Ch)

			b.Resize(2, 4)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 4, b.cursor.position)
			assert.Equal(t, '2', cells[2][0].Ch)
		})

		t.Run("buffer with exactly height lines", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n3\n4")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '4', cells[4][0].Ch)

			b.Resize(2, 4)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '4', cells[4][0].Ch)
		})
	})

	t.Run("extends rows if height is increased", func(t *testing.T) {

		t.Run("buffer with more lines than height", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 10)
			assert.Equal(t, '9', cells[9][0].Ch)

			b.Resize(2, 11)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 11)
			assert.Equal(t, '9', cells[9][0].Ch)
			assert.Equal(t, ' ', cells[10][0].Ch)
		})

		t.Run("buffer with less lines than height", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n \n ")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '2', cells[2][0].Ch)

			b.Resize(2, 11)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 11)
			assert.Equal(t, '2', cells[2][0].Ch)
			assert.Equal(t, ' ', cells[10][0].Ch)
		})

		t.Run("buffer with exactly height lines", func(t *testing.T) {
			b := makePrimaryBufferForTesting(1, 5)
			resetPrimaryBuffer(b, "0\n1\n2\n3\n4")
			cells := b.Cells.RawCells()
			require.Len(t, cells, 5)
			assert.Equal(t, '4', cells[4][0].Ch)

			b.Resize(2, 11)
			cells = b.Cells.RawCells()
			require.Len(t, cells, 11)
			assert.Equal(t, '4', cells[4][0].Ch)
			assert.Equal(t, ' ', cells[10][0].Ch)
		})
	})

	t.Run("wraps rows", func(t *testing.T) {
		suite := []struct {
			initialWidth     int
			initialHeight    int
			finalWidth       int
			finalHeight      int
			input            string
			expectedOutput   string
			expectedCursorAt term.Coordinates
		}{
			{0, 0, 5, 5, "", "     \n     \n     \n     \n     ", term.Coordinates{}},
			{2, 2, 5, 5, "a\nb", "a    \nb    \n     \n     \n     ", term.Coordinates{Y: 1, X: 1}},
			{10, 5, 5, 5, "aaaaaa\nbbb", "aaaaa\na    \nbbb  \n     \n     ", term.Coordinates{Y: 2, X: 3}},
			{10, 5, 9, 5, "aaaaaa\nbbb", "aaaaaa   \nbbb      \n         \n         \n         ", term.Coordinates{Y: 1, X: 3}},
			{10, 10, 5, 5, "aaaaaa\nbbb", "aaaaa\na    \nbbb  \n     \n     ", term.Coordinates{Y: 2, X: 3}},
			{10, 5, 2, 5, "aaaaaa\nbbb\n$", "aa\naa\nbb\nb \n$ ", term.Coordinates{Y: 5, X: 1}},
			{10, 5, 2, 2, "aaaaaa\nbbb\n$", "b \n$ ", term.Coordinates{Y: 5, X: 1}},
			{10, 5, 2, 8, "aaaaaa\nbbb\n$", "aa\naa\naa\nbb\nb \n$ \n  \n  ", term.Coordinates{Y: 5, X: 1}},
			{5, 5, 10, 2, "aaaa\nbbb\n$", "bbb       \n$         ", term.Coordinates{Y: 2, X: 1}},
			{5, 5, 0, 0, "aaaa\nbbb\n$", "", term.Coordinates{Y: 2, X: 1}},
		}

		for i, test := range suite {
			t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
				b := NewPrimaryBuffer(0, testHistory)
				b.Resize(test.initialWidth, test.initialHeight)
				writeToPrimaryBuffer(b, test.input)

				writer := term.NewStringWriter(test.initialWidth, test.initialHeight)
				b.Draw(writer)
				writer.Flush()
				initialContent := writer.String()
				initialCursor := b.CursorAtScroll()

				b.Resize(test.finalWidth, test.finalHeight)
				writer = term.NewStringWriter(test.finalWidth, test.finalHeight)
				b.Draw(writer)
				writer.Flush()
				assert.Equal(t, test.expectedOutput, writer.String(), "%q", b.Cells.String())
				assert.Equal(t, test.expectedCursorAt, b.CursorAtScroll())

				b.Resize(test.initialWidth, test.initialHeight)
				writer = term.NewStringWriter(test.initialWidth, test.initialHeight)
				b.Draw(writer)
				writer.Flush()
				assert.Equal(t, initialContent, writer.String(), "shrink back to original size: %q", b.Cells.String())
				assert.Equal(t, initialCursor, b.CursorAtScroll())
			})
		}
	})
}

func makePrimaryBufferForTesting(width, height int) *PrimaryBuffer {
	ret := NewPrimaryBuffer(0, testHistory)
	ret.SetDefaultChar(' ')
	ret.Resize(width, height)
	return ret
}

func writeToPrimaryBuffer(b *PrimaryBuffer, str string) {
	for _, ch := range str {
		pos := b.CursorAtScreen()
		if ch == '\n' {
			pos.X = 0
			if pos.Y+1 >= b.BottomScrollableRegion() {
				b.InsertLines(1, term.Coordinates{})
				start := 0
				end := b.Rows()
				count := max(0, min(1, end-start))
				if count > 0 {
					b.ScrollUp(start, end, count)
				}
			} else {
				pos.Y++
			}
			b.SetCursorAtScreen(pos, false)
		} else {
			b.Write(ch, uniseg.StringWidth(string(ch)), 0)
			pos.X++
			b.SetCursorAtScreen(pos, false)
		}
	}
}

func resetPrimaryBuffer(b *PrimaryBuffer, to string) {
	b.ResetLines(0, b.Height())
	b.SetCursorAtScreen(term.Coordinates{}, false)
	writeToPrimaryBuffer(b, to)
}
