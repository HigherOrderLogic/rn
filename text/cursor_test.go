package text

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ernestrc/tcell/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/api/config"
	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text/clipboard"
	"unstable.build/go-tui/workspace"
)

const locID = "errors"
const sampleSnippet = `
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
	e = NewCursor(scroll)
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
				e.buffer().InsertRowAt(0)
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
				e.cursor = term.Coordinates{X: 1, Y: 1}
				assert.False(t, e.MoveEndLine())
			},
			term.Coordinates{X: 1, Y: 1},
		},
		{
			"MoveEndLine should move to end of line if past the end of line",
			10, 10,
			func(t *testing.T, e *Cursor) {
				if e.scroll.Wrap {
					// in wrap mode, that would be a different scroll position
					t.SkipNow()
				}
				e.scroll.SeekEndFile()
				e.MoveToScroll(term.Coordinates{Y: 22, X: 10})
				assert.True(t, e.MoveEndLine())
			},
			term.Coordinates{X: 9, Y: 22},
		},
		{
			"MoveEndLine should move cursor to end of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 1
				assert.True(t, e.MoveEndLine())
			},
			term.Coordinates{X: 1, Y: 1},
		},
		{
			"MoveEndLine should seek to end of line if end is out of window",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2

				assert.True(t, e.MoveEndLine())
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
			"MoveDown should move the cursor position past the last line until end of window",
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
			term.Coordinates{X: 0, Y: 999},
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
			"MoveDown should NOT seek up if reached last line",
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
			"MoveUp should do nothing if already on first line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.False(t, e.MoveUp())
			},
			term.Coordinates{X: 0, Y: 0},
		},
		{
			"MoveUp should fix cursor position if negative",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = -1
				assert.True(t, e.MoveUp())
			},
			term.Coordinates{X: 0, Y: 0},
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
			"MoveLeft fixes cursor pos if negative",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = -1
				assert.True(t, e.MoveLeft())
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
			},
			term.Coordinates{X: 0, Y: 0},
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
			"MoveRight should move cursor right even if past current line's end of line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				assert.True(t, e.MoveRight())
			},
			term.Coordinates{X: 1, Y: 0},
		},
		{
			"MoveRight should move cursor right even if at the end of the line",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.X = 1
				assert.True(t, e.MoveRight())
			},
			term.Coordinates{X: 2, Y: 0},
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
			// line is 77 characters long so cursor should be a t x=76
			term.Coordinates{X: 76, Y: 2},
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
			term.Coordinates{X: 4, Y: 19},
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
				n := 3
				if e.scroll.Wrap {
					n += 7 // wraps
				}
				for i := 0; i < n; i++ {
					e.MoveDown()
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

			require.True(t, cursor.MoveLastLine())
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
		wrap bool
	}{
		{true}, {false},
	}
	for _, test := range suite {
		t.Run(fmt.Sprintf("wrap:%v", test.wrap), func(t *testing.T) {
			e := setupCursor(t, 10, 10, test.wrap)
			n := len(e.scroll.Buffer().String())

			require.True(t, e.MoveLastLine())
			require.True(t, e.MoveEndLine())
			require.True(t, e.MoveRight())

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
		initialBuf  string
		initialPos  func(*Cursor)
		selected    bool
		finalPos    func(*Cursor)
		deleted     bool
		finalBuf    string
		skipForMode []SelectMode
	}{
		{
			initialBuf: "",
			selected:   true, // technically select is in bounds, which is what bool return means
			finalPos:   func(*Cursor) {},
			deleted:    false,
			finalBuf:   "",
		},
		{
			initialBuf: "a",
			selected:   true,
			finalPos:   func(*Cursor) {},
			deleted:    true,
			finalBuf:   "",
		},
		{
			initialBuf:  "\n",
			selected:    true,
			finalPos:    func(*Cursor) {},
			deleted:     true,
			finalBuf:    "",
			skipForMode: []SelectMode{StandardSelection, BlockSelection},
		},
		{
			initialBuf:  "a\nb",
			selected:    true,
			finalPos:    func(c *Cursor) { c.MoveRight() },
			deleted:     true,
			finalBuf:    "\nb",
			skipForMode: []SelectMode{LineSelection, BlockSelection},
		},
		{
			initialBuf:  "a\nb",
			selected:    true,
			finalPos:    func(c *Cursor) { c.MoveDown() },
			finalBuf:    "",
			deleted:     true,
			skipForMode: []SelectMode{LineSelection, BlockSelection},
		},
		{
			initialBuf: "a\nb",
			initialPos: func(c *Cursor) {
				for i := 0; i < 3; i++ {
					c.MoveRight()
				}
			},
			finalPos:    func(c *Cursor) { c.MoveDown() },
			selected:    true,
			deleted:     true,
			finalBuf:    "a",
			skipForMode: []SelectMode{LineSelection, BlockSelection},
		},
		{
			initialBuf: "a\nb\nc\nd",
			selected:   true,
			finalPos: func(c *Cursor) {
				for i := 0; i < 100; i++ {
					c.MoveDown()
				}
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
			selected:    false,
			deleted:     false,
			finalBuf:    "a\nb",
			skipForMode: []SelectMode{LineSelection},
		},
		{
			initialBuf: "a\nb\nc\nd",
			selected:   true,
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
			selected:   true,
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
	}

	for i, tcase := range tsuite {
		t.Run(fmt.Sprintf("select %v test case %d", typeSelect, i), func(t *testing.T) {
			var skip bool
			for _, mode := range tcase.skipForMode {
				if mode == typeSelect {
					skip = true
					break
				}
			}
			if skip {
				return
			}

			c := setupCursorContent(t, width, height, tcase.initialBuf, false)
			if tcase.initialPos != nil {
				tcase.initialPos(c)
			}
			switch typeSelect {
			case NoSelection:
				panic("hmm...")
			case BlockSelection:
				require.Equal(t, tcase.selected, c.SelectBlock())
			case LineSelection:
				require.Equal(t, tcase.selected, c.SelectLine())
			case StandardSelection:
				require.Equal(t, tcase.selected, c.Select())
			}
			if !tcase.selected {
				return
			}
			tcase.finalPos(c)
			require.Equal(t, tcase.deleted, c.DeleteSelection())
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

	assert.True(t, e.MoveRight())
	assert.True(t, e.MoveRight())
	assert.True(t, e.MoveRight())

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
	assert.Equal(t, term.Coordinates{X: 0, Y: 31}, pos)
}

func TestCursorSkipNulls(t *testing.T) {
	e := setupCursor(t, 100, 100, false)

	assert.True(t, e.MoveRight())

	e.MoveToNextNonNull()

	pos := e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 0}, pos)

	e.MoveLastLine()
	e.MoveDown()

	e.MoveToNextNonNull()

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 0, Y: 32}, pos)

	e.MoveFirstLine()
	for i := 0; i < 16; i++ {
		e.MoveDown()
	}
	e.MoveRight()

	e.MoveToNextNonNull()

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 3, Y: 16}, pos)

	e.MoveRight()
	e.MoveToNextNonNull()

	pos = e.Coordinates()
	assert.Equal(t, term.Coordinates{X: 7, Y: 16}, pos)

	t.Run("does not infinite loop if cursor has negative coords", func(t *testing.T) {
		e := setupCursor(t, 100, 100, false)
		e.cursor = term.Coordinates{X: -1}
		e.MoveToNextNonNull()
	})
	t.Run("does not infinite loop if at end of line and on multi codepoint utf8", func(t *testing.T) {
		e := setupCursorContent(t, 100, 100, "abcd💥", true)
		for i := 0; i < 10; i++ {
			e.cursor = term.Coordinates{X: i}
			e.MoveToNextNonNull()
		}
	})
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
}

func TestCursorSortedLocations(t *testing.T) {
	t.Run("sorts by level", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)

		infoList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    abcAttr,
				Message: "info",
			},
			{
				From:    term.Coordinates{Y: 11},
				To:      term.Coordinates{Y: 11, X: 11},
				Attr:    abcAttr,
				Message: "info2",
			},
		})
		errList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 2},
				To:      term.Coordinates{Y: 2, X: 2},
				Attr:    abcAttr,
				Message: "err",
			},
		})
		criticalList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 3},
				To:      term.Coordinates{Y: 3, X: 3},
				Attr:    abcAttr,
				Message: "critical",
			},
		})
		warnList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 4},
				To:      term.Coordinates{Y: 4, X: 4},
				Attr:    abcAttr,
				Message: "warn",
			},
		})
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityError, "errList", errList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityCritical, "criticalList", criticalList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityWarning, "warnList", warnList))

		locations := c.SortedLocations()
		require.Len(t, locations, 5)

		assert.Equal(t, term.Coordinates{Y: 1}, locations[0].From)
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, locations[0].To)
		assert.Equal(t, "info", locations[0].Message)

		assert.Equal(t, "info2", locations[1].Message)
		assert.Equal(t, "warn", locations[2].Message)
		assert.Equal(t, "err", locations[3].Message)
		assert.Equal(t, "critical", locations[4].Message)
	})

	t.Run("sorts by id if level is the same", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)
		infoList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    abcAttr,
				Message: "info",
			},
			{
				From:    term.Coordinates{Y: 11},
				To:      term.Coordinates{Y: 11, X: 11},
				Attr:    abcAttr,
				Message: "info1",
			},
		})
		infoList2 := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 2},
				To:      term.Coordinates{Y: 2, X: 2},
				Attr:    abcAttr,
				Message: "info2",
			},
		})
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList2", infoList2))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))

		locations := c.SortedLocations()
		require.Len(t, locations, 3)

		assert.Equal(t, term.Coordinates{Y: 1}, locations[0].From)
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, locations[0].To)
		assert.Equal(t, "info", locations[0].Message)
		assert.Equal(t, "info1", locations[1].Message)
		assert.Equal(t, "info2", locations[2].Message)
	})

	t.Run("idempotency", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)
		infoList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    abcAttr,
				Message: "info",
			},
			{
				From:    term.Coordinates{Y: 11},
				To:      term.Coordinates{Y: 11, X: 11},
				Attr:    abcAttr,
				Message: "info1",
			},
		})
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))

		require.Len(t, c.SortedLocations(), 2)
		require.Len(t, c.SortedLocations(), 2)
		require.Len(t, c.SortedLocations(), 2)
		require.Len(t, c.SortedLocations(), 2)
	})
}

func TestCursorDrawLocationListsIntegration(t *testing.T) {
	t.Run("sets location list attrs", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		expected := [][]term.Cell{
			{{}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
			{{}},
		}

		abcList := LocationSlice(abcLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("clears location lists", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)
		abcList := LocationSlice(abcLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		assert.NotNil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, nil))

		expected := [][]term.Cell{
			{{}},
			{{}},
			{{}},
			{{}},
			{{}},
		}

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("lower priority lists do not override higher priority list attrs", func(t *testing.T) {
		infoLocations := []textapi.Location{
			{
				From: term.Coordinates{Y: 1},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorGreen},
			},
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 2, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorGreen},
			},
			{
				From: term.Coordinates{Y: 3},
				To:   term.Coordinates{Y: 3, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorGreen},
			},
		}
		criticalLocations := []textapi.Location{
			{
				From: term.Coordinates{Y: 1},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack},
			},
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 2, X: 1},
				Attr: term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack},
			},
			{
				From: term.Coordinates{Y: 3},
				To:   term.Coordinates{Y: 3, X: 1},
				Attr: term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack},
			},
		}
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		expected := [][]term.Cell{
			{{}},
			{{Attributes: term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}}},
			{{Attributes: term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}}},
			{{Attributes: term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}}},
			{{}},
		}

		criticalList := LocationSlice(criticalLocations)
		infoList := LocationSlice(infoLocations)
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityCritical, "list1", criticalList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "list2", infoList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("trims to fit location To line if From is in bounds", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		expected := [][]term.Cell{
			{{}},
			{{}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
		}
		locations := []textapi.Location{
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 6, X: 1},
				Attr: abcAttr,
			},
		}

		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("does not panic if width == 0 in wrap mode", func(t *testing.T) {
		c := setupCursorContent(t, 0, 5, "\na\nb\nc\n", false)
		c.scroll.Wrap = true

		expected := [][]term.Cell{
			{{}},
			{{}},
			{{}},
			{{}},
			{{}},
		}
		locations := []textapi.Location{
			{
				From: term.Coordinates{Y: 0},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: abcAttr,
			},
		}

		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})
}

func TestCursorSetLocationListMessages(t *testing.T) {
	content := "\naaa\nbbb\nccc\n"
	messageLocations := []textapi.Location{
		{
			From:    term.Coordinates{Y: 1},
			To:      term.Coordinates{Y: 1, X: 2},
			Attr:    abcAttr,
			Message: "1",
		},
		{
			From:    term.Coordinates{Y: 2},
			To:      term.Coordinates{Y: 2, X: 2},
			Attr:    abcAttr,
			Message: "2",
		},
		{
			From:    term.Coordinates{Y: 3},
			To:      term.Coordinates{Y: 3, X: 2},
			Attr:    abcAttr,
			Message: "3",
		},
	}

	assertMessages := func(t *testing.T, c *Cursor) {
		for i := 0; i < 3; i++ {
			locsByID, ok := c.LocationsAtCursor()
			require.True(t, ok, c.messages)
			require.Len(t, locsByID, 1)
			assert.Equal(t, locsByID[locID].Message, strconv.Itoa(i+1))
			c.MoveDown()
		}
	}

	t.Run("returns nil/false if cursor is not in from, to or in between", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)
		abcList := LocationSlice(messageLocations)
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		_, ok := c.LocationsAtCursor()
		assert.False(t, ok)
	})

	t.Run("return messages if cursor is at From", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)
		abcList := LocationSlice(messageLocations)
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		require.True(t, c.MoveDown())

		assertMessages(t, c)
	})

	t.Run("return messages if cursor between From/To", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)

		abcList := LocationSlice(messageLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		require.True(t, c.MoveDown())
		require.True(t, c.MoveRight())

		assertMessages(t, c)
	})

	t.Run("return messages if cursor is at To", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)

		abcList := LocationSlice(messageLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		require.True(t, c.MoveDown())
		c.MoveRight()
		c.MoveRight()

		assertMessages(t, c)
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
		t.Fatalf("Failed to get working directory: %s", err)
	}
	uri, err := workspaceapi.CurrentUserHostURI(wd)
	if err != nil {
		t.Fatalf("Failed to parse working directory as URI %s: %s", wd, err)
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
			cursor := NewCursor(scroll)

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
		{term.Coordinates{}, term.Coordinates{},
			"z", NoSelection, false, "za\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{},
			"z\n", NoSelection, false, "z\na\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{Y: 1},
			"z\n", NoSelection, true, "a\nz\nb\nc\nd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3},
			"z\n", NoSelection, false, "a\nb\nc\nz\nd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 4},
			"z\n", NoSelection, true, "a\nb\nc\nd\nz\n"},
		{term.Coordinates{}, term.Coordinates{},
			"z", StandardSelection, false, "za\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{X: 1},
			"z", StandardSelection, true, "az\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{},
			"z\nx", StandardSelection, false, "z\nxa\nb\nc\nd"},
		{term.Coordinates{}, term.Coordinates{X: 1},
			"z\nx", StandardSelection, true, "az\nx\nb\nc\nd"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 3},
			"z", StandardSelection, false, "a\nb\nc\nzd"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 3, X: 1},
			"z", StandardSelection, true, "a\nb\nc\ndz"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 3},
			"z\nx", StandardSelection, false, "a\nb\nc\nz\nxd"},
		{term.Coordinates{Y: 3}, term.Coordinates{Y: 3, X: 1},
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
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 4},
			"z", LineSelection, true, "a\nb\nc\nd\nz"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 3},
			"z\nx", LineSelection, false, "a\nb\nc\nz\nxd"},
		{term.Coordinates{X: 1, Y: 3}, term.Coordinates{Y: 4},
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

		c.Replace('X')
		assert.Equal(t, "X\nb\nc", c.buffer().String())

		c.MoveDown()
		c.MoveLeft()
		c.Replace('Y')
		assert.Equal(t, "X\nY\nc", c.buffer().String())

		c.MoveDown()
		c.MoveLeft()
		c.Replace('Z')
		assert.Equal(t, "X\nY\nZ", c.buffer().String())

		c.Replace('A')
		assert.Equal(t, "X\nY\nZA", c.buffer().String())

		c.Replace('B')
		assert.Equal(t, "X\nY\nZAB", c.buffer().String())
	})

	t.Run("wrap", func(t *testing.T) {
		const initialContent = "aaaaaaaaaaa\nb\nc"
		c := setupCursorContent(t, 5, 5, initialContent, false)

		c.MoveEndLine()
		c.MoveLeft()
		c.Replace('X')
		c.Replace('Y')
		assert.Equal(t, "aaaaaaaaaXY\nb\nc", c.buffer().String())
	})
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
	cursor := NewCursor(s)
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
	cursor := NewCursor(s)
	cursor.MoveToMark(CursorMark{term.Coordinates{Y: 7}, term.Coordinates{}})

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
	cursor := NewCursor(s)
	cursor.MoveToMark(CursorMark{term.Coordinates{Y: 7}, term.Coordinates{}})

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
