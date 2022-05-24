package text

import (
	"fmt"
	"io/ioutil"
	os "os"
	"strconv"
	"strings"
	"testing"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var discardLogger = log.New()

func init() {
	discardLogger.Out = ioutil.Discard
	discardLogger.Level = log.PanicLevel
}

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
		{
		curtab->tp_diffbuf[i] = NULL;
		curtab->tp_diff_invalid = TRUE;
		diff_redraw(TRUE);
		}
	}
	}
	else
	diff_buf_add(win->w_buffer);
} /* { */ `

func setupCursorContent(t *testing.T, width, height int, cont string) (e *Cursor) {
	scroll := component.NewScroll(cell.NewBuffer())
	e = NewCursor(scroll)
	scroll.Buffer().ReadFrom(strings.NewReader(cont))
	scroll.Resize(width, height)
	require.Equal(t, e.scroll.Buffer(), scroll.Buffer())
	require.Equal(t, e.subscriber.c, e)

	return
}

func setupCursor(t *testing.T, width, height int) *Cursor {
	return setupCursorContent(t, width, height, sampleSnippet)
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
			e := setupCursor(t, tcase.width, tcase.height)

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
		e := setupCursor(t, 100, 100)

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
				e.scroll.SeekEndFile()
				e.cursor.X = 9
				e.cursor.Y = 0
				e.scroll.SeekEndLine()
				assert.True(t, e.MoveEndLine())
			},
			term.Coordinates{X: 8, Y: 22},
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

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 76, e.scroll.Offset().X+e.cursor.X)
				assert.Equal(t, 'f', c.Ch)
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
				assert.Equal(t, 31, e.scroll.Offset().Y+e.cursor.Y)
				assert.Equal(t, '}', c.Ch)
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveDown should move the cursor position past the last line until end of window",
			1000, 1000,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 31
				for i := 0; i < e.scroll.Height(); i++ {
					e.MoveDown()
				}
			},
			term.Coordinates{X: 0, Y: 999},
		},
		{
			"MoveDown should seek down if reached last line in window but not at last line",
			10, 10,
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
			10, 10,
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
			2, 2,
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
			term.Coordinates{X: 77, Y: 2},
		},
		{
			"MoveRightStartWord should move to the start of the next word",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.cursor.Y = 2
				e.cursor.X = 9

				assert.True(t, e.MoveRightStartWord())

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, 't', c.Ch)
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
				e.MoveToMark(CursorMark{term.Coordinates{Y: 7}})

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '{', c.Ch)

				// scroll does not provide correct max y offsets
				// if Draw hasn't been called yet so seek might fail
				// if there are lots of wraps on an un-drawn scroll.
				// At some point we might want to fix this by checking
				// if wrapsLen == -1 or wraps == nil
				e.scroll.Draw(term.NoopWriter{})

				assert.True(t, e.MoveToMatchingRune())

				c, _ = e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '}', c.Ch)
			},
			term.Coordinates{X: 0, Y: 31},
		},
		{
			"MoveToMatchingRune should move to the 'matching rune' backwards",
			10, 10,
			func(t *testing.T, e *Cursor) {
				e.MoveToMark(CursorMark{term.Coordinates{Y: 31}})

				c, _ := e.scroll.Buffer().Cell(e.cursorAtScroll())
				assert.Equal(t, '}', c.Ch)

				assert.True(t, e.MoveToMatchingRune())

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
				e.MoveDown()
				e.MoveDown()
				e.MoveDown()
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
			e := setupCursor(t, tcase.width, tcase.height)

			tcase.sut(t, e)

			cursor := e.CursorAtScroll()
			assert.Equal(t, tcase.cursorAtScroll, cursor)
		})

		t.Run(tcase.desc+" (wrap mode on)", func(t *testing.T) {
			e := setupCursor(t, tcase.width, tcase.height)
			e.scroll.Wrap = true

			tcase.sut(t, e)

			cursor := e.CursorAtScroll()
			assert.Equal(t, tcase.cursorAtScroll, cursor)
		})
	}
}

func TestCursorInsertRow(t *testing.T) {
	e := setupCursor(t, 10, 10)

	assert.Equal(t, 32, e.scroll.Buffer().Rows())
	assert.Equal(t, term.Coordinates{}, e.Coordinates())
	assert.Equal(t, term.Coordinates{}, e.cursorAtScroll())

	e.InsertRowAbove()
	assert.Equal(t, 0, len(e.scroll.Buffer().RawCells()[0]))
	assert.Equal(t, 33, e.scroll.Buffer().Rows())
	assert.Equal(t, term.Coordinates{}, e.Coordinates())
	assert.Equal(t, term.Coordinates{}, e.cursorAtScroll())

	e.scroll.SeekEndLine()
	e.cursor.Y = 2
	e.cursor.X = 9

	e.InsertRowAbove()
	assert.Equal(t, 0, len(e.scroll.Buffer().RawCells()[2]))
	assert.Equal(t, 34, e.scroll.Buffer().Rows())
	assert.Equal(t, term.Coordinates{Y: 2, X: 0}, e.Coordinates())

	e.MoveEndLine()
	e.InsertRowBelow()
	assert.Equal(t, 35, e.scroll.Buffer().Rows())
	assert.Equal(t, term.Coordinates{Y: 3, X: 0}, e.Coordinates())
	assert.Equal(t, term.Coordinates{Y: 3, X: 0}, e.cursorAtScroll())

	e.MoveLastLine()
	e.MoveStartLine()
	assert.Equal(t, term.Coordinates{Y: 9, X: 0}, e.Coordinates())
	assert.Equal(t, term.Coordinates{Y: 34}, e.cursorAtScroll())

	e.InsertRowBelow()
	assert.Equal(t, 36, e.scroll.Buffer().Rows())
	assert.Equal(t, term.Coordinates{Y: 9, X: 0}, e.Coordinates())
	assert.Equal(t, term.Coordinates{Y: 35}, e.cursorAtScroll())
}

func TestCursorInsertRowBelow(t *testing.T) {
	scroll := component.NewScroll(cell.NewBuffer())
	scroll.Resize(10, 10)
	cursor := NewCursor(scroll)
	in := scroll.Buffer()

	content := `package main
func main() {
}`
	in.WriteString(content)
	assert.Equal(t, in.String(), content)

	require.True(t, cursor.MoveDown())
	cursor.InsertRowBelow()
	cursor.InsertRowBelow()
	cursor.Insert('\t')
	cursor.Insert('f')
	cursor.Insert('m')
	cursor.Insert('t')
	cursor.Insert('.')

	assert.Equal(t, `package main
func main() {

	fmt.
}`, in.String())

	require.True(t, cursor.MoveLastLine())

	cursor.InsertRowBelow()
	cursor.InsertRowBelow()
	cursor.Insert('i')
	cursor.InsertRowBelow()
	cursor.Insert('\t')
	cursor.Insert('X')
	cursor.InsertRowBelow()
	cursor.Insert('}')

	assert.Equal(t, `package main
func main() {

	fmt.
}

i
	X
}`, in.String())

}

func TestCursorInsertDelete(t *testing.T) {
	e := setupCursor(t, 10, 10)
	str := e.scroll.Buffer().String()

	e.Insert('p')
	e.Insert('a')
	e.Insert('c')
	e.Insert('k')

	for i := 0; i < 4; i++ {
		e.MoveLeft()
		e.Delete()
	}

	assert.Equal(t, str, e.scroll.Buffer().String())
}

func TestCursorBackspace(t *testing.T) {
	e := setupCursor(t, 10, 10)
	n := len(e.scroll.Buffer().String())

	e.MoveLastLine()
	e.MoveEndLine()
	e.MoveRight()

	for i := 0; i < n; i++ {
		e.Backspace()
	}

	assert.Equal(t, "", e.scroll.Buffer().String())
}

func TestCursorConflate(t *testing.T) {
	e := setupCursor(t, 10, 10)
	n := strings.Count(e.scroll.Buffer().String(), "\n")
	rows := e.scroll.Buffer().Rows()
	require.Equal(t, n+1, rows)

	for i := 0; i < n; i++ {
		e.Conflate()
	}

	assert.Equal(t, 1, e.scroll.Buffer().Rows())
	assert.Zero(t, strings.Count(e.scroll.Buffer().String(), "\n"))
}

func testCursorSelect(t *testing.T, width, height int) {
	makeSelect := func(t *testing.T) *Cursor {
		e := setupCursor(t, width, height)
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
		e := setupCursor(t, width, height)
		str := e.scroll.Buffer().String()
		assertBufferAttributes(t, e.buffer(), term.Attributes{})
		require.True(t, e.Select())
		require.True(t, e.MoveLastLine())
		// test that we can switch between after move
		require.True(t, e.SelectLine())
		assert.Equal(t, str, e.Selection())
		return e
	}

	makeSelectBlock := func(t *testing.T) *Cursor {
		e := setupCursor(t, width, height)
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
			assertBufferAttributes(t, e.buffer(), term.Attributes{Fg: term.AttrReverse, Bg: term.AttrReverse})
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
		e := setupCursor(t, width, height)
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
		clipboard := NewInMemoryClipboard()
		ok, err := e.CopySelection(DefaultRegisterID, clipboard)
		require.True(t, ok)
		require.NoError(t, err)
		assert.Equal(t, "", e.Selection())
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		data, err := clipboard.Paste(DefaultRegisterID)
		require.NoError(t, err)
		assert.Equal(t, e.scroll.Buffer().String(), data.Text)
	})

	t.Run("SelectLine/CopySelection copies from start line to end line", func(t *testing.T) {
		e := makeSelectLine(t)
		clipboard := NewInMemoryClipboard()
		ok, err := e.CopySelection(DefaultRegisterID, clipboard)
		require.True(t, ok)
		require.NoError(t, err)
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		data, err := clipboard.Paste(DefaultRegisterID)
		require.NoError(t, err)
		assert.Equal(t, e.scroll.Buffer().String(), data.Text)
	})

	t.Run("SelectBlock/CopySelection copies from start to end in block", func(t *testing.T) {
		e := makeSelectBlock(t)
		clipboard := NewInMemoryClipboard()
		ok, err := e.CopySelection(DefaultRegisterID, clipboard)
		require.True(t, ok)
		require.NoError(t, err)
		assertBufferAttributes(t, e.buffer(), term.Attributes{})

		data, err := clipboard.Paste(DefaultRegisterID)
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
	e := setupCursor(t, width, height)
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

			c := setupCursorContent(t, width, height, tcase.initialBuf)
			if tcase.initialPos != nil {
				tcase.initialPos(c)
			}
			switch typeSelect {
			case noSelection:
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
			e := setupCursorContent(t, tcase.width, tcase.height, tcase.content)
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
	e := setupCursor(t, 100, 100)

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
	e := setupCursor(t, 100, 100)

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
		e := setupCursor(t, 100, 100)
		e.cursor = term.Coordinates{X: -1}
		e.MoveToNextNonNull()
	})
}

func TestCursorCell(t *testing.T) {
	t.Run("does not panic if cursor has negative coords", func(t *testing.T) {
		e := setupCursor(t, 100, 100)
		e.cursor = term.Coordinates{X: -1}
		_, ok := e.Cell()
		assert.False(t, ok)
	})
}

func TestCursorShiftLine(t *testing.T) {
	c := setupCursorContent(t, 10, 1, " blabla\nbleble")
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
	c := setupCursorContent(t, 10, 10, " blabla\nbleble")
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
	abcAttr      = term.Attributes{Fg: term.AttrUnderline, Bg: term.ColorBlack}
	abcLocations = []Location{
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
		c := setupCursorContent(t, 10, 10, "")
		assert.False(t, c.MoveToPrevLocation(locID))
	})

	t.Run("MoveToNextLocation should return false and do nothing if location list is nil", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "")
		assert.False(t, c.MoveToNextLocation(locID))
	})

	t.Run("MoveToPrevLocation should return false and do nothing if already at start of location list", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "")
		locations := []Location{Location{}}
		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: locations}))
		assert.False(t, c.MoveToPrevLocation(locID))
	})

	t.Run("MoveToNextLocation should return false and do nothing if already at end of location list", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "")
		locations := []Location{Location{}}
		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: locations}))
		assert.False(t, c.MoveToNextLocation(locID))
	})

	t.Run("MoveToPrevLocation should return true and move cursor to earlier location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, " X")
		require.True(t, c.MoveRight())

		locations := []Location{Location{}}
		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: locations}))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{}, pos)
	})

	t.Run("MoveToNextLocation should wrap around to first location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc \n")
		require.True(t, c.MoveLastLine())

		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: abcLocations}))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 1}, pos)
	})

	t.Run("MoveToPrevLocation should wrap around to last location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n")

		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: abcLocations}))
		assert.True(t, c.MoveToPrevLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 3}, pos)
	})

	t.Run("MoveToNextLocation should wrap around to first location (special case)", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, " X")
		require.True(t, c.MoveRight())

		locations := []Location{Location{}, Location{From: term.Coordinates{X: 1}}}
		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: locations}))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{}, pos)
	})

	t.Run("MoveToPrevLocation should wrap around to last location (special case)", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, " X")

		locations := []Location{Location{}, Location{From: term.Coordinates{X: 1}}}
		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: locations}))
		assert.True(t, c.MoveToPrevLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{X: 1}, pos)
	})

	t.Run("MoveToNextLocation should go to next location after cursor", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc \n")
		require.True(t, c.MoveDown())
		require.True(t, c.MoveDown())

		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: abcLocations}))
		assert.True(t, c.MoveToNextLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 3}, pos)
	})

	t.Run("MoveToPrevLocation should go to prev location before location", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n")
		require.True(t, c.MoveDown())
		require.True(t, c.MoveDown())

		assert.Nil(t, c.SetLocationList(locID, &testLocationList{locations: abcLocations}))
		assert.True(t, c.MoveToPrevLocation(locID))

		pos := c.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 1}, pos)
	})
}

func TestCursorSetLocationListAttr(t *testing.T) {
	c := setupCursorContent(t, 10, 10, "\na\nb\nc\n")
	buf := c.scroll.Buffer()

	expected := [][]term.Cell{
		{},
		{{Ch: 'a', Bg: abcAttr.Bg, Fg: abcAttr.Fg}},
		{{Ch: 'b', Bg: abcAttr.Bg, Fg: abcAttr.Fg}},
		{{Ch: 'c', Bg: abcAttr.Bg, Fg: abcAttr.Fg}},
		{},
	}

	abcList := &testLocationList{locations: abcLocations}

	assert.Nil(t, c.SetLocationList(locID, abcList))
	assert.Equal(t, expected, buf.RawCells())

	buf.RawCells()[2][0].Bg = term.AttrUnderline
	buf.RawCells()[2][0].Fg = term.AttrBold

	newLocations := []Location{
		{
			From: term.Coordinates{Y: 2},
			To:   term.Coordinates{Y: 2, X: 1},
			Attr: term.Attributes{Bg: term.ColorRed},
		},
	}

	expected = [][]term.Cell{
		{},
		{{Ch: 'a'}},
		{{Ch: 'b',
			Fg: term.AttrBold, Bg: term.AttrUnderline | term.ColorRed}},
		{{Ch: 'c'}},
		{},
	}
	oldLocList := c.SetLocationList(locID, &testLocationList{locations: newLocations})
	assert.Equal(t, abcList, oldLocList)
	assert.Equal(t, expected, buf.RawCells())

	require.True(t, buf.DeleteRow(0))

	// make sure it doesn't remove the wrong one
	buf.RawCells()[0][0].Bg = term.ColorRed
	newLocations = []Location{
		{
			From: term.Coordinates{Y: 2},
			To:   term.Coordinates{Y: 2, X: 1},
			Attr: term.Attributes{Bg: term.ColorRed},
		},
	}
	expected = [][]term.Cell{
		{{Ch: 'a', Bg: term.ColorRed}},
		{{Ch: 'b', Fg: term.AttrBold, Bg: term.AttrUnderline}},
		{{Ch: 'c', Bg: term.ColorRed}},
		{},
	}
	newL := c.SetLocationList(locID, &testLocationList{locations: newLocations})
	assert.Equal(t, expected, buf.RawCells())
	assert.Nil(t, newL)

	// clear location list
	c.SetLocationList(locID, nil)
	expected = [][]term.Cell{
		{{Ch: 'a', Bg: term.ColorRed}},
		{{Ch: 'b', Fg: term.AttrBold, Bg: term.AttrUnderline}},
		{{Ch: 'c'}},
		{},
	}
	assert.Equal(t, expected, buf.RawCells())
}

func TestCursorSetLocationListMessages(t *testing.T) {
	content := "\naaa\nbbb\nccc\n"
	messageLocations := []Location{
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
			locsByID, ok := c.Locations()
			require.True(t, ok, c.messages)
			require.Len(t, locsByID, 1)
			assert.Equal(t, locsByID[locID].Message, strconv.Itoa(i+1))
			c.MoveDown()
		}
	}

	t.Run("returns nil/false if cursor is not in from, to or in between", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content)
		abcList := &testLocationList{locations: messageLocations}
		assert.Nil(t, c.SetLocationList(locID, abcList))

		_, ok := c.Locations()
		assert.False(t, ok)
	})

	t.Run("return messages if cursor is at From", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content)
		abcList := &testLocationList{locations: messageLocations}
		assert.Nil(t, c.SetLocationList(locID, abcList))
		require.True(t, c.MoveDown())

		assertMessages(t, c)
	})

	t.Run("return messages if cursor between From/To", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content)

		abcList := &testLocationList{locations: messageLocations}

		assert.Nil(t, c.SetLocationList(locID, abcList))
		require.True(t, c.MoveDown())
		require.True(t, c.MoveRight())

		assertMessages(t, c)
	})

	t.Run("return messages if cursor is at To", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content)

		abcList := &testLocationList{locations: messageLocations}

		assert.Nil(t, c.SetLocationList(locID, abcList))
		require.True(t, c.MoveDown())
		c.MoveRight()
		c.MoveRight()

		assertMessages(t, c)
	})
}

func TestCursorMoveToScroll(t *testing.T) {
	t.Run("moves cursor to position within curr width,height", func(t *testing.T) {
		e := setupCursor(t, 10, 10)
		e.MoveToScroll(term.Coordinates{X: 1, Y: 3})
		pos := e.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 3, X: 1}, pos)
		pos = e.CursorAtScroll()
		assert.Equal(t, term.Coordinates{Y: 3, X: 1}, pos)
	})
	t.Run("moves cursor to position past curr height", func(t *testing.T) {
		e := setupCursor(t, 5, 5)
		e.MoveToScroll(term.Coordinates{X: 0, Y: 6})
		pos := e.CursorAtScroll()
		assert.Equal(t, term.Coordinates{Y: 6, X: 0}, pos)
	})
	t.Run("moves cursor to position past curr width", func(t *testing.T) {
		e := setupCursor(t, 5, 5)
		e.MoveToScroll(term.Coordinates{X: 7, Y: 2})
		pos := e.CursorAtScroll()
		assert.Equal(t, term.Coordinates{Y: 2, X: 7}, pos)
	})
}

func TestCursorWrap(t *testing.T) {
	t.Run("takes wraps into consideration", func(t *testing.T) {
		e := setupCursor(t, 10, 10)
		e.scroll.Wrap = true
		e.MoveToScroll(term.Coordinates{X: 76, Y: 2})
		pos := e.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 9, X: 6}, pos)
	})
	t.Run("handles cursor.X past last row's column", func(t *testing.T) {
		e := setupCursor(t, 10, 10)
		e.scroll.Wrap = true
		e.MoveToScroll(term.Coordinates{X: 77, Y: 2})
		pos := e.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 9, X: 7}, pos)
	})
	t.Run("handles cursor.X == width", func(t *testing.T) {
		e := setupCursor(t, 10, 10)
		e.scroll.Wrap = true
		e.cursor.X = e.scroll.Width()
		pos := e.Coordinates()
		assert.Equal(t, term.Coordinates{Y: 1}, pos)
	})
	t.Run("MoveRight moves cursor past the end of line of a wrapped line", func(t *testing.T) {
		e := setupCursor(t, 10, 10)
		e.scroll.Wrap = true
		e.MoveToScroll(term.Coordinates{X: 15, Y: 3})

		require.True(t, e.MoveRight())
		cursor := e.CursorAtScroll()
		assert.Equal(t, term.Coordinates{Y: 3, X: 16}, cursor)
	})
}

func TestCursorSelectWordInsertWord(t *testing.T) {
	e := setupCursorContent(t, 5, 5, sampleSnippet+"\n")
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
}

func TestCursorReplaceAllWithNewline(t *testing.T) {
	e := setupCursorContent(t, 5, 5, sampleSnippet+"\n")
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

func cwdURI(t *testing.T) workspace.URI {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %s", err)
	}
	uri, err := workspace.CurrentUserHostURI(wd)
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
			assert.False(t, cursor.MoveDown())
		}},
		{"does not move beyond last line", false, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			assert.Equal(t, term.Coordinates{Y: 31}, cursor.CursorAtScroll())
			assert.False(t, cursor.MoveDown())
		}},
		{"is able to insert at last line + 1", false, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			assert.False(t, cursor.MoveDown())
			cursor.InsertRowBelow()
			assert.Equal(t, term.Coordinates{Y: 32}, cursor.CursorAtScroll())
			assert.False(t, cursor.MoveDown())
			assert.Equal(t, sampleSnippet+"\n", cursor.buffer().String())
		}},
		{"is able to insert at last EOL", true, func(t *testing.T, cursor *Cursor) {
			assert.True(t, cursor.MoveLastLine())
			assert.False(t, cursor.MoveDown())
			cursor.InsertRowBelow()
			assert.Equal(t, term.Coordinates{Y: 32}, cursor.CursorAtScroll())
			assert.False(t, cursor.MoveDown())
			assert.Equal(t, sampleSnippet+"\n", cursor.buffer().String())
		}},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.description, func(t *testing.T) {
			b := cell.NewBuffer()
			file, err := ioutil.TempFile("", "frctl_file_test")
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

			uri, err := workspace.CurrentUserHostURI(file.Name())
			require.NoError(t, err)

			swapDir, err := workspace.CurrentUserHostURI("/tmp")
			require.NoError(t, err)

			// installs unix reader
			m, err := workspace.NewManager(discardLogger, cwdURI(t))
			require.NoError(t, err)
			_, err = m.Open(uri, b, swapDir, false)
			require.NoError(t, err)

			tcase.test(t, cursor)
		})
	}
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
	cursor.MoveToMark(CursorMark{term.Coordinates{Y: 7}})

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
	cursor.MoveToMark(CursorMark{term.Coordinates{Y: 7}})

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
