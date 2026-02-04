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
	"bufio"
	"context"
	"maps"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
)

// SelectMode represents a select mode.
type SelectMode uint8

const (
	// NoSelection represents the mode where there's no selection.
	NoSelection SelectMode = iota
	// StandardSelection represents a select mode. See Select for more details.
	StandardSelection
	// LineSelection represents a select mode. See SelectLine for more details.
	LineSelection
	// BlockSelection represents a select mode. See SelectBlock for more details.
	BlockSelection
)

func (s SelectMode) String() string {
	switch s {
	case NoSelection:
		return "nop"
	case StandardSelection:
		return "standard"
	case LineSelection:
		return "line"
	case BlockSelection:
		return "block"
	}
	panic("unknown selection mode")
}

const (
	searchLocationListID         = "search"
	selectionLocationListID      = "selection"
	internalLocationListPriority = textapi.LocationPriorityCritical
)

// used to subscribe to buffer updates
type curSubscriber struct {
	mark CursorMark
	c    *Cursor
}

type message struct {
	listID   string
	location textapi.Location
}

// Cursor is a helper structure which manages a cursor over a Scroll.
type Cursor struct {
	// RightInclusiveSemantics changes the cursor selection behaviour to
	// add the current cursor position to the selection.
	RightInclusiveSemantics bool

	ctx        context.Context
	scroll     *component.Scroll
	search     string
	cursor     term.Coordinates
	shouldSeek bool
	searchAttr term.Attributes

	scheduleNextTick func(func()) bool

	locationStore LocationStore

	selection struct {
		mode       SelectMode
		scrollFrom term.Coordinates
		cells      [][]term.Cell
	}
	subscriber curSubscriber
}

// NewCursor allocates storage for a new cursor,
// initializes it with an empty Scroll, and returns it.
func NewCursor(scroll *component.Scroll, scheduleNextTick func(func()) bool) *Cursor {
	c := new(Cursor)
	c.Init(scroll, scheduleNextTick)
	return c
}

// Center centers the cursor at the scroll such that the cursor
// occupies the line at the center of the view.
func (c *Cursor) Center() (handled bool) {
	seeked := c.scroll.RepositionLineCenter(c.cursorAtScroll().Y)
	c.cursor.Y -= seeked
	return seeked != 0
}

// RepositionTop repositions the cursor at the scroll such that the cursor
// occupies the line at the top of the view.
func (c *Cursor) RepositionTop() (handled bool) {
	seeked := c.scroll.RepositionLineTop(c.cursorAtScroll().Y)
	c.cursor.Y -= seeked
	return seeked != 0
}

// RepositionBottom repositions the cursor at the scroll such that the cursor
// occupies the line at the bottom of the view.
func (c *Cursor) RepositionBottom() (handled bool) {
	seeked := c.scroll.RepositionLineBottom(c.cursorAtScroll().Y)
	c.cursor.Y -= seeked
	return seeked != 0
}

// Init initializes this cursor with the given scroll and subscribes
// to changes to the scroll's buffer. If buffer is swapped
// via Scroll.SetBuffer, consider re-initializing this cursor with the
// updated scroll, unless it's a temporary swap.
// Also, once initialized this cursor MUST NOT be copied.
func (c *Cursor) Init(scroll *component.Scroll, scheduleNextTick func(func()) bool) {
	c.InitPerformance(scroll)
	c.scroll.Buffer().Subscribe(&c.subscriber)
	c.scroll.Subscribe(&c.subscriber)
	c.shouldSeek = true
	c.scheduleNextTick = scheduleNextTick
}

// InitPerformance initializes this Cursor with a Scroll
// that was initialized with InitPerformance. It also
// disables automatic scrolling of content for the client.
// It also disables all fold-related methods.
func (c *Cursor) InitPerformance(scroll *component.Scroll) {
	c.ctx = context.Background()
	c.cursor = term.Coordinates{}
	c.scroll = scroll
	c.selection.mode = NoSelection
	c.subscriber.c = c
	c.locationStore.Init()

	// zero-out scroll attributes so we can better control
	// what gets highlighted upon search/search word, etc.
	c.searchAttr = scroll.ResultsAttr
	scroll.ResultsAttr = term.Attributes{}
}

func (c *curSubscriber) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
}

func (c *curSubscriber) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
	c.c.setSearchLocationList(c.c.search, false)
}

func (c *curSubscriber) OnWillSeek(from term.Coordinates) {
}

func (c *curSubscriber) OnDidSeek(from, to term.Coordinates) {
}

func (c *curSubscriber) OnWillHide(start, end int) {
	c.mark = c.c.Mark()
}

func (c *curSubscriber) OnDidHide(start, end int) {
	c.c.MoveToMark(c.mark)
}

func (c *curSubscriber) OnWillVisible(start int) {
	c.mark = c.c.Mark()
}

func (c *curSubscriber) OnDidVisible(start int) {
	c.c.MoveToMark(c.mark)
}

// Coordinates returns the current position of the cursor.
func (c *Cursor) Coordinates() term.Coordinates {
	return c.cursor
}

// View returns the underlying cell.Buffer view.
func (c *Cursor) View() cell.View {
	return c.view()
}

func (c *Cursor) view() cell.View {
	return c.buffer().View()
}

func (c *Cursor) buffer() *cell.Buffer {
	return c.scroll.Buffer()
}

// CursorMark is used with Mark and MoveToMark to
// move the cursor to previously marked positions.
type CursorMark struct {
	// keep it private so cursor position semantics are hidden from clients
	scroll term.Coordinates
	window term.Coordinates
}

// Before returns true if other is before CursorMark.
func (c CursorMark) Before(other term.Coordinates) bool {
	res := term.CoordinatesDiff(c.window, other)
	return res.Y < 0 || res.Y == 0 && res.X < 0
}

// Mark returns the current cursor position as a CursorMark
// to later be used in calls to MoveToMark.
func (c *Cursor) Mark() CursorMark {
	return CursorMark{
		scroll: c.cursorAtScroll(),
		window: c.cursor,
	}
}

// MoveToMark moves the cursor to the position represented by mark.
// It attempts to keep cursor in the same window position as it was
// when the given mark was created.
func (c *Cursor) MoveToMark(mark CursorMark) (CursorMark, bool) {
	ret := CursorMark{window: c.cursor, scroll: c.cursorAtScroll()}
	pos := mark.scroll
	pos.Y = max(0, min(pos.Y, c.rows()-1))
	pos.X = max(0, min(pos.X, c.view().Columns(pos.Y)))
	win, _ := c.scroll.ScrollToWindowCoordinates(pos)
	// if inside hidden block, we still want to make the best out of it
	c.setCursor(win, false)
	// try to keep cursor at the same window position, if possible
	if c.cursor.Y > mark.window.Y {
		diff := c.cursor.Y - mark.window.Y
		for diff > 0 && c.cursor.Y > 0 && c.scroll.SeekDown() {
			c.cursor.Y--
			diff--
		}
	} else if c.cursor.Y < mark.window.Y {
		diff := mark.window.Y - c.cursor.Y
		for diff > 0 && c.cursor.Y < c.scroll.SizeHeight() && c.scroll.SeekUp() {
			c.cursor.Y++
			diff--
		}
	}
	return ret, c.cursorAtScroll() != ret.scroll
}

func (c *Cursor) rows() int {
	return c.view().Rows()
}

// CursorAtScroll returns the current position of the cursor relative
// to the scroll coorindates.
func (c *Cursor) CursorAtScroll() term.Coordinates {
	return c.cursorAtScroll()
}

// SetCursorAtScroll moves the cursor to the given scroll position.
func (c *Cursor) SetCursorAtScroll(pos term.Coordinates) term.Coordinates {
	prev := c.scroll.WindowToScrollCoordinates(c.cursor)
	win, _ := c.scroll.ScrollToWindowCoordinates(pos)
	c.setCursor(win, false)
	return prev
}

// SelectionFrom returns the position of the current selection,
// if cursor is in select mode.
func (c *Cursor) SelectionFrom() (pos term.Coordinates, ok bool) {
	if c.selection.mode == NoSelection {
		return
	}
	ok = true
	pos = c.selection.scrollFrom
	return
}

// MoveToScroll moves the cursor to pos in scroll.
func (c *Cursor) MoveToScroll(pos term.Coordinates) (
	ret term.Coordinates, ok bool,
) {
	enable := c.disablePublishing()
	defer enable()

	pos.Y = max(0, pos.Y)
	pos.X = max(0, pos.X)
	ret = c.cursorAtScroll()
	c.moveToScroll(pos)
	ok = ret != c.cursorAtScroll()
	return
}

// the bounds of the current view, then underlying scroll is used
// to seek to pos.
func (c *Cursor) moveToScroll(pos term.Coordinates) {
	var windowPos term.Coordinates
	if c.scroll.Width() == 0 && c.scroll.Wrap {
		// best effort, assume no wrap when width is 0
		windowPos = term.CoordinatesDiff(pos, c.scroll.Offset())
	} else {
		windowPos, _ = c.scroll.ScrollToWindowCoordinates(pos)
	}
	c.setCursor(windowPos, c.shouldSeek)
}

func (c *Cursor) seekToScrollCoordinates() {
	if c.scroll.Width() == 0 || c.scroll.SizeHeight() == 0 {
		return
	}
	pos := c.cursor
	// scroll can return some coordinates that are be outside
	// of the bounds of the current window.
	// For instance, if DeleteCell deletes a tab, it could be that
	// pos.X at scroll yields a negative coordinate
	// (i.c. -3 if tabspaces is 4)
	for !c.scroll.Wrap && pos.X < 0 && c.scroll.SeekLeft() {
		pos.X++
	}
	for pos.Y < 0 && c.scroll.SeekUp() {
		pos.Y++
	}
	for !c.scroll.Wrap && pos.X > 0 && pos.X >= c.scroll.Width() && c.scroll.SeekRight() {
		pos.X--
	}

	for pos.Y > 0 && pos.Y >= c.scroll.SizeHeight() && c.scroll.SeekDown() {
		pos.Y--
	}
	c.cursor = pos
}

// note that pos is window coordinates, not scroll coordinates
func (c *Cursor) setCursor(pos term.Coordinates, seek bool) {
	if c.cursor == pos {
		// even if pos is the same, content could have scrolled
		if c.selection.mode != NoSelection {
			c.setSelection()
		}
		return
	}

	c.cursor = pos

	// this is an optimization to disable expensive calculations
	// during composite moves that call setCursor multiple times
	if seek {
		c.seekToScrollCoordinates()
	}

	if c.selection.mode != NoSelection {
		c.setSelection()
	}
}

// Search searches text string in the underlying cell buffer. It returns
// the number of occurrences found.
func (c *Cursor) Search(text string) int {
	n := c.setSearchLocationList(text, false /* word */)
	return n
}

// SearchWord searches the given word in the underlying cell buffer. It returns
// the number of occurrences found.
func (c *Cursor) SearchWord(text string) int {
	n := c.setSearchLocationList(text, true /* word */)
	return n
}

func isBeforeCursorOrInsideHiddenBlock(c *Cursor, cursor, pos term.Coordinates) bool {
	return pos.Y < cursor.Y || (pos.Y == cursor.Y && pos.X <= cursor.X) ||
		isInsideHiddenBlock(c, cursor, pos)
}

func isInsideHiddenBlock(c *Cursor, cursor, pos term.Coordinates) bool {
	at, ok := c.scroll.ScrollToWindowCoordinates(pos)
	return !ok && at.Y == cursor.Y
}

func isPastCursor(c *Cursor, cursor, pos term.Coordinates) bool {
	return pos.Y > cursor.Y || (pos.Y == cursor.Y && pos.X >= cursor.X)
}

// MoveToNextMatch moves the cursor to the next search result if any.
func (c *Cursor) MoveToNextMatch() (ok bool) {
	return c.MoveToNextLocation(searchLocationListID)
}

// MoveToPrevMatch moves the cursor to the next search result if any.
func (c *Cursor) MoveToPrevMatch() (ok bool) {
	return c.MoveToPrevLocation(searchLocationListID)
}

// MoveStartLine moves the cursor at the start of the current line, scrolling
// to the start of the line if required.
func (c *Cursor) MoveStartLine() (ok bool) {
	_, ok = c.MoveToScroll(term.Coordinates{Y: c.cursorAtScroll().Y})
	return
}

// MoveStartLineNonBlank moves the cursor at the first character that's not
// blank (e.g. space, tab, etc.) in the current line.
func (c *Cursor) MoveStartLineNonBlank() bool {
	enable := c.disablePublishing()
	defer enable()

	initialScrollPos := c.cursorAtScroll()
	pos, _ := c.scroll.ScrollToWindowCoordinates(
		term.Coordinates{X: 0, Y: initialScrollPos.Y})
	c.setCursor(pos, false)
	cells := c.view().RawCells()

	x := 0
	isBlank := true
	for isBlank {
		if x >= len(cells) {
			return false
		}
		cell, ok := c.cellAtCursor()
		if !ok {
			return false
		}
		isBlank = isOneOf(cell, blankCharacters)
		if !isBlank {
			return true
		}
		at, _ := c.scroll.ScrollToWindowCoordinates(
			term.Coordinates{X: x, Y: initialScrollPos.Y})
		c.setCursor(at, false)
		x++
	}

	return true
}

// MoveEndLine moves the cursor at the end of the current line, scrolling
// to the end of the line if required.
func (c *Cursor) MoveEndLine() (ok bool) {
	curr := c.cursorAtScroll()
	if curr.Y >= c.rows() {
		return
	}
	x := c.view().Columns(curr.Y)
	_, ok = c.MoveToScroll(term.Coordinates{Y: curr.Y, X: x})
	return
}

// MoveFirstLine moves the cursor to the first line, scrolling the content
// if appplicable.
func (c *Cursor) MoveFirstLine() (ok bool) {
	_, ok = c.MoveToScroll(term.Coordinates{X: c.cursorAtScroll().X})
	return
}

// MoveLastLine moves the cursor to the last line, scrolling the content
// if required.
func (c *Cursor) MoveLastLine() (ok bool) {
	max := c.rows()
	if max == 0 {
		return
	}
	_, ok = c.MoveToScroll(term.Coordinates{X: c.cursorAtScroll().X, Y: max - 1})
	return ok
}

// MoveDown moves the cursor one line below the current line, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveDown() (ok bool) {
	atScroll := c.cursorAtScroll()
	if atScroll.Y >= c.rows()-1 {
		if c.scroll.Wrap && c.scroll.SeekDown() {
			// last line is longer than the entire height x width
			// allow user to scroll down by moving up and down the window axis
			return c.tryMoveDownWindowRow()
		}
		return
	}
	atScroll.Y++
	_, ok = c.MoveToScroll(atScroll)
	if !ok {
		return c.tryMoveDownWindowRow()
	}
	return ok
}

// MoveDownLines moves the cursor "n" lines below the current line, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveDownLines(n int) (ok bool) {
	return c.multiplyMove(n, c.MoveDown)
}

// MoveUp moves the cursor one line above the current line, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveUp() (ok bool) {
	atScroll := c.cursorAtScroll()
	if atScroll.Y <= 0 {
		// first line is longer than the entire height x width
		// allow user to scroll up by moving up and down the window axis
		if c.scroll.Wrap && c.scroll.SeekUp() {
			var win term.Coordinates
			win, ok = c.scroll.ScrollToWindowCoordinates(atScroll)
			if !ok {
				return ok
			}
			win.Y--
			_, ok = c.MoveToScroll(c.scroll.WindowToScrollCoordinates(win))
		}
		return
	}
	atScroll.Y--
	_, ok = c.MoveToScroll(atScroll)
	return
}

// MoveUpLines moves the cursor "n" lines above the current line, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveUpLines(n int) (ok bool) {
	return c.multiplyMove(n, c.MoveUp)
}

// MoveLeft moves the cursor one column before the current cell, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveLeft() (ok bool) {
	atScroll := c.cursorAtScroll()
	if atScroll.X <= 0 {
		return
	}
	atScroll.X--
	_, ok = c.MoveToScroll(atScroll)
	return
}

// MoveLeftColumns moves the cursor "n" columns before the current cell, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveLeftColumns(n int) (ok bool) {
	return c.multiplyMove(n, c.MoveLeft)
}

// MoveRight moves the cursor one column after the current cell, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveRight() (ok bool) {
	atScroll := c.cursorAtScroll()
	if atScroll.Y >= c.rows() || atScroll.X >= c.view().Columns(atScroll.Y) {
		return
	}
	atScroll.X++
	_, ok = c.MoveToScroll(atScroll)
	return
}

// MoveRightColumns moves the cursor "n" columns after the current cell, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveRightColumns(n int) (ok bool) {
	return c.multiplyMove(n, c.MoveRight)
}

// MoveLeftWrap will move the cursor to the left or wrap to end of
// previous line if cursor is at X=0
func (c *Cursor) MoveLeftWrap() bool {
	ok := c.MoveLeft()
	if ok {
		return true
	}
	ok = c.MoveUp()
	if !ok {
		return false
	}
	c.MoveEndLine()
	return true
}

// MoveRightWrap will move the cursor to the right or wrap to beginning
// of next line if cursor is at X=EOL
func (c *Cursor) MoveRightWrap() bool {
	ok := c.MoveRight()
	if ok {
		return true
	}
	ok = c.MoveDown()
	if !ok {
		return false
	}
	c.MoveStartLine()
	return true
}

func (c *Cursor) multiplyMove(n int, move func() bool) (ok bool) {
	enable := c.disablePublishing()
	defer enable()
	for i := 0; i < n; i++ {
		mOk := move()
		// If it was moving ok and now it stopped, we're done.
		if ok && !mOk {
			return
		}
		ok = mOk || ok
	}
	return
}

func (c *Cursor) cursorAtScroll() term.Coordinates {
	return c.scroll.WindowToScrollCoordinates(c.cursor)
}

func (c *Cursor) cellAtCursor() (cell term.Cell, ok bool) {
	cursorAtScroll := c.cursorAtScroll()
	cells := c.view().RawCells()
	if cursorAtScroll.Y >= len(cells) ||
		cursorAtScroll.X >= len(cells[cursorAtScroll.Y]) ||
		cursorAtScroll.Y < 0 || cursorAtScroll.X < 0 {
		return
	}
	ok = true
	cell = cells[cursorAtScroll.Y][cursorAtScroll.X]
	return
}

func isOneOf(cell term.Cell, special map[rune]struct{}) bool {
	_, ok := special[cell.Ch]
	return ok
}

func isNoneOf(cell term.Cell, skip map[rune]struct{}) (none bool) {
	_, ok := skip[cell.Ch]
	return !ok
}

func (c *Cursor) moveAfterRune(budget int, skip, special map[rune]struct{}, move func() bool) (ok bool) {
	enable := c.disablePublishing()
	defer enable()

	const (
		init = iota
		foundRune
	)

	initialScrollPos := c.cursorAtScroll()
	mark := c.Mark()
	state := init

	cell, cOk := c.cellAtCursor()
	if !cOk {
		return
	}

	if isOneOf(cell, special) {
		state = foundRune
	}

	for i := 0; i < budget && move(); i++ {
		cell, cOk := c.cellAtCursor()
		if !cOk {
			continue
		}
		ok = true
		mark = c.Mark()

		if initialScrollPos.Y != c.cursorAtScroll().Y {
			state = foundRune
		}

		if isOneOf(cell, special) {
			state = foundRune
		}

		switch state {
		case init:
			if isOneOf(cell, skip) {
				state = foundRune
			}
		case foundRune:
			if isNoneOf(cell, skip) {
				return
			}
		}
	}

	c.MoveToMark(mark)
	return
}

func (c *Cursor) moveBeforeRune(budget int, skip, all map[rune]struct{}, move func() bool) (ok bool) {
	enable := c.disablePublishing()
	defer enable()

	const (
		skipRune = iota
		findRune
		done
	)

	state := skipRune
	initialScrollPos := c.cursorAtScroll()
	mark := c.Mark()

	var moved bool
	for i := 0; i < budget && move(); i++ {
		moved = true
		cell, cOk := c.cellAtCursor()
		if !cOk {
			continue
		}

		switch state {
		case skipRune:
			if isOneOf(cell, skip) {
				continue
			}
			if isOneOf(cell, all) {
				state = done
				mark = c.Mark()
			} else {
				state = findRune
			}
		case findRune:
			if isOneOf(cell, all) {
				state = done
			}
			if initialScrollPos.Y != c.cursorAtScroll().Y {
				state = done
			}
		}
		if state == done {
			break
		}
		mark = c.Mark()
	}

	if !moved {
		return
	}
	if state == done {
		ok = true
		c.MoveToMark(mark)
	}
	return
}

var allSpecialCharacters = map[rune]struct{}{
	'.': {}, ',': {}, ':': {}, ';': {}, ' ': {}, ')': {}, '"': {},
	'\'': {}, '(': {}, '{': {}, '}': {}, '[': {}, ']': {}, '\t': {},
	'\x00': {}, '\\': {}, '/': {}, '+': {}, '`': {}, '_': {}, '@': {},
	'=': {}, '<': {}, '>': {}, '!': {}, '?': {}, '|': {}, '&': {},
	'*': {}, '%': {}, '#': {}, '^': {}, '-': {}}

var skipCharacters = map[rune]struct{}{' ': {}, '\t': {}, '\x00': {}, '_': {}}

var blankCharacters = map[rune]struct{}{'\x00': {}, ' ': {}, '\t': {}}

const budgetFindWord = 100

// MoveRightStartWordGroup moves the cursor right to the start of the next word.
func (c *Cursor) MoveRightStartWordGroup() bool {
	return c.moveAfterRune(budgetFindWord, skipCharacters, skipCharacters, c.MoveRightWrap)
}

// MoveLeftStartWordGroup moves the cursor left to the start of the previous word.
func (c *Cursor) MoveLeftStartWordGroup() bool {
	return c.moveBeforeRune(budgetFindWord, skipCharacters,
		skipCharacters, c.MoveLeftWrap)
}

// MoveRightEndWordGroup moves the cursor right to the end of the next or current word.
func (c *Cursor) MoveRightEndWordGroup() bool {
	ok := c.moveBeforeRune(budgetFindWord, skipCharacters,
		skipCharacters, c.MoveRightWrap)
	if !ok || c.RightInclusiveSemantics {
		return false
	}
	c.MoveRight()
	return true
}

// MoveLeftEndWordGroup moves the cursor left to the end of the previous word.
func (c *Cursor) MoveLeftEndWordGroup() bool {
	ok := c.moveAfterRune(budgetFindWord, skipCharacters,
		skipCharacters, c.MoveLeftWrap)
	if !ok || c.RightInclusiveSemantics {
		return false
	}
	c.MoveRight()
	return true
}

// MoveRightStartWord moves the cursor right to the start of the next word.
func (c *Cursor) MoveRightStartWord() bool {
	return c.moveAfterRune(budgetFindWord, skipCharacters,
		allSpecialCharacters, c.MoveRightWrap)
}

// MoveLeftStartWord moves the cursor left to the start of the previous word.
func (c *Cursor) MoveLeftStartWord() bool {
	return c.moveBeforeRune(budgetFindWord, skipCharacters,
		allSpecialCharacters, c.MoveLeftWrap)
}

// MoveLeftStartWordNoWrap moves the cursor left to the start of the previous word,
// but as opposed to MoveLeftStartWord, it doesn't continue on the previous line after exhausting
// results on the current line.
func (c *Cursor) MoveLeftStartWordNoWrap() bool {
	return c.moveBeforeRune(budgetFindWord, skipCharacters,
		allSpecialCharacters, c.MoveLeft)
}

// MoveRightEndWord moves the cursor right to the end of the next or current word.
func (c *Cursor) MoveRightEndWord() bool {
	ok := c.moveBeforeRune(budgetFindWord, skipCharacters,
		allSpecialCharacters, c.MoveRightWrap)
	if !ok || c.RightInclusiveSemantics {
		return ok
	}
	c.MoveRight()
	return true
}

// MoveRightEndWordNoWrap moves the cursor right to the end of the next or current word,
// but as opposed to MoveRightEndWord, it doesn't continue on the next line after exhausting
// results on the current line.
func (c *Cursor) MoveRightEndWordNoWrap() bool {
	ok := c.moveBeforeRune(budgetFindWord, skipCharacters,
		allSpecialCharacters, c.MoveRight)
	if !ok || c.RightInclusiveSemantics {
		return ok
	}
	c.MoveRight()
	return true
}

// MoveLeftEndWord moves the cursor left to the end of the previous word.
func (c *Cursor) MoveLeftEndWord() bool {
	ok := c.moveAfterRune(budgetFindWord, skipCharacters,
		allSpecialCharacters, c.MoveLeftWrap)
	if !ok || c.RightInclusiveSemantics {
		return ok
	}
	c.MoveRight()
	return true
}

// IsStartWord returns true if cursor is at the start of a word.
func (c *Cursor) IsStartWord() bool {
	pos := c.cursorAtScroll()
	start, _, word := c.scroll.WordAt(pos)
	return word != "" && start == pos
}

// IsEndWord returns true if cursor is at the start of a word.
func (c *Cursor) IsEndWord() bool {
	pos := c.cursorAtScroll()
	if pos.X > 0 && !c.RightInclusiveSemantics {
		pos.X--
	}
	_, end, word := c.scroll.WordAt(pos)
	if end.X > 0 && c.RightInclusiveSemantics {
		end.X--
	} else {
		pos.X++
	}
	return word != "" && end == pos
}

// MoveToMatchingRune moves the cursor to the balanced matching rune of the rune at
// the current cursor's cell.
func (c *Cursor) MoveToMatchingRune() bool {
	cell, ok := c.cellAtCursor()
	if !ok {
		return ok
	}

	ok = false
	switch cell.Ch {
	case '[':
		ok = c.moveMatchRuneForward('[', ']')
	case '{':
		ok = c.moveMatchRuneForward('{', '}')
	case '(':
		ok = c.moveMatchRuneForward('(', ')')

	case ']':
		ok = c.moveMatchRuneBackward(']', '[')
	case '}':
		ok = c.moveMatchRuneBackward('}', '{')
	case ')':
		ok = c.moveMatchRuneBackward(')', '(')
	}

	return ok
}

// InsertLineAbove inserts a row above the current row and moves the cursor up.
func (c *Cursor) InsertLineAbove() {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	cursorAtScroll := c.cursorAtScroll()
	pos := cursorAtScroll
	pos.X = 0
	c.buffer().Edit(c.ctx, pos, pos, "\n")
	pos, _ = c.tryIndent(c.ctx, pos)
	c.selection.mode = mode
	c.setSelection()
	c.setCursorAfterUpdate(pos)
}

// InsertLineBelow inserts a row below the current row and moves the cursor down.
func (c *Cursor) InsertLineBelow() {
	if _, ok := c.scroll.HiddenBlockAt(c.cursorAtScroll().Y); ok {
		c.MoveDown()
		c.MoveStartLine()
		c.Insert('\n')
		c.MoveUp()
		c.MoveEndLine()
		return
	}
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	buf := c.buffer()
	pos := c.cursorAtScroll()
	pos.X = buf.Columns(pos.Y)
	_, to, _ := buf.Edit(c.ctx, pos, pos, "\n")
	to, _ = c.tryIndent(c.ctx, to)

	c.selection.mode = mode
	c.setSelection()
	c.setCursorAfterUpdate(to)
}

// Insert is equivalent to InsertContext with context.Background.
func (c *Cursor) Insert(r rune) {
	c.InsertContext(c.ctx, r)
}

// InsertContext inserts rune at the current cursor's position.
func (c *Cursor) InsertContext(ctx context.Context, r rune) {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	insertAt := c.cursorAtScroll()
	pos := c.buffer().InsertContext(ctx, insertAt, r)
	switch r {
	case '\n':
		pos, _ = c.tryIndent(ctx, pos)
	case '}', ']', ')':
		var ok bool
		pos, ok = c.tryDedent(ctx, pos)
		if ok {
			pos.X++
		}
	}
	c.selection.mode = mode
	c.setSelection()
	c.setCursorAfterUpdate(pos)
}

// InsertWithAttr inserts the given rune with the given attributes,
// at the current cursor's position .
func (c *Cursor) InsertWithAttr(r rune, attr term.Attributes) {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	insertAt := c.cursorAtScroll()
	pos := c.buffer().InsertWithAttr(insertAt, r, attr)
	switch r {
	case '\n':
		pos, _ = c.tryIndent(c.ctx, pos)
	case '}', ']', ')':
		var ok bool
		pos, ok = c.tryDedent(c.ctx, pos)
		if ok {
			pos.X++
		}
	}
	c.selection.mode = mode
	c.setSelection()
	c.setCursorAfterUpdate(pos)
}

// InsertString inserts str at the current cursor's position.
func (c *Cursor) InsertString(str string) {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	_, until := c.buffer().InsertString(c.cursorAtScroll(), str)
	c.selection.mode = mode

	c.setSelection()
	c.setCursorAfterUpdate(until)
}

// InsertBlock inserts a string in a block-wise fashion meaning it
// will insert each of the lines at corresponding relative x and y positions
// shifting content to the right accordingly.
func (c *Cursor) InsertBlock(str string) {
	reader := bufio.NewReader(strings.NewReader(str))
	for {
		str, err := reader.ReadString('\n')
		if err == nil && len(str) > 0 {
			str = str[:len(str)-1]
		}
		cur := c.CursorAtScroll()
		c.InsertString(str)
		if err != nil {
			break
		}
		cur.Y++
		c.MoveToScroll(cur)
	}
}

// Paste pastes the given string on the underlying scroll at the current
// cursor position.
func (c *Cursor) Paste(str string, mode SelectMode, after bool) {
	// disable seeking while performing combined move
	prev := c.shouldSeek
	c.shouldSeek = false

	if mode == NoSelection {
		// this is how vim behaves when using a system clipboard
		if strings.HasSuffix(str, "\n") {
			mode = LineSelection
		} else {
			mode = StandardSelection
		}
	}

	if ok := c.DeleteSelection(); ok {
		// vim keeps the new line when pasting on a fully selected line
		if mode == LineSelection {
			c.InsertString("\n")
			c.MoveUp()
		}
	}

	switch mode {
	case StandardSelection:
		if after {
			c.MoveRight()
		}
		c.InsertString(str)
	case LineSelection:
		if after {
			movedDown := c.MoveLineDown()
			var cur term.Coordinates
			if !movedDown {
				// force set cursor past last line
				cur = c.CursorAtScroll()
				c.setCursor(term.Coordinates{X: 0, Y: c.cursor.Y + 1}, false)
			} else {
				c.MoveStartLine()
				cur = c.CursorAtScroll()
			}
			c.InsertString(str)
			c.MoveToScroll(cur)
		} else {
			cur := c.CursorAtScroll()
			c.MoveStartLine()
			c.InsertString(str)
			c.MoveToScroll(cur)
			c.MoveStartLine()
		}
	case BlockSelection:
		if after {
			c.MoveRight()
			cur := c.CursorAtScroll()
			c.InsertBlock(str)
			c.MoveToScroll(cur)
		} else {
			cur := c.CursorAtScroll()
			c.InsertBlock(str)
			c.MoveToScroll(cur)
		}
	}
	c.shouldSeek = prev
}

// Replace is equivalent to ReplaceContext with context.Background. It returns the
// position next to the replaced character for the caller to decide whether to move.
func (c *Cursor) Replace(r rune) (next term.Coordinates) {
	return c.ReplaceContext(c.ctx, r)
}

// ReplaceContext replaces the cell under the cursor with r. It returns the position
// next to the replaced character for the caller to decide whether to move.
func (c *Cursor) ReplaceContext(ctx context.Context, r rune) (next term.Coordinates) {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	from := c.cursorAtScroll()
	to := term.Coordinates{X: from.X + 1, Y: from.Y}
	_, next, _ = c.buffer().Edit(ctx, from, to, string(r))

	c.selection.mode = mode
	c.setSelection()
	return
}

// Delete is equivalent to DeleteContext with context.Background.
func (c *Cursor) Delete() (ok bool) {
	ok = c.DeleteContext(c.ctx)
	return
}

// DeleteContext deletes the cell at the current cursor position.
func (c *Cursor) DeleteContext(ctx context.Context) (ok bool) {
	var pos term.Coordinates
	pos, _, ok = c.buffer().DeleteCellContext(ctx, c.cursorAtScroll())
	if ok {
		c.setCursorAfterUpdate(pos)
	}
	return
}

// Backspace is a special form of Delete, named after the keyboard key backspace.
func (c *Cursor) Backspace() (ok bool) {
	if c.cursorAtScroll().X > 0 {
		if c.MoveLeft() {
			ok = c.Delete()
		}
		return
	}

	if ok = c.MoveLineUp(); ok {
		ok = c.Conflate()
	}
	return
}

// Conflate is equivalent to calling ConflateContext with context.Background.
func (c *Cursor) Conflate() (ok bool) {
	return c.ConflateContext(c.ctx)
}

// ConflateContext removes the new line character at the end of the current line.
func (c *Cursor) ConflateContext(ctx context.Context) (ok bool) {
	enable := c.disablePublishing()
	defer enable()

	pos := c.cursorAtScroll()
	if pos.Y >= c.rows() {
		ok = false
		return
	}

	if c.view().Columns(pos.Y) == 0 {
		ok = c.buffer().DeleteRowContext(ctx, pos.Y)
		return
	}
	x, ok := c.buffer().ConflateRowContext(ctx, pos.Y)
	if ok {
		c.setCursorAfterUpdate(term.Coordinates{Y: pos.Y, X: x})
	}
	return
}

func (c *Cursor) setSelection() (ok bool) {
	from := c.selection.scrollFrom
	to := c.cursorAtScroll()

	from, to = term.CoordinatesSort(from, to)
	if c.RightInclusiveSemantics {
		to.X++
	}

	var sels []cell.Selection
	switch c.selection.mode {
	case StandardSelection:
		c.selection.cells, sels, ok = c.buffer().Select(from, to)
	case LineSelection:
		c.selection.cells, sels, ok = c.buffer().SelectLine(from, to)
	case BlockSelection:
		c.selection.cells, sels, ok = c.buffer().SelectBlock(from, to)
	case NoSelection:
		c.selection.cells = nil
		ok = true
	}

	var locs []textapi.Location
	for _, sel := range sels {
		locs = append(locs, textapi.Location{
			From: sel.From,
			To:   sel.To,
			Attr: term.Attributes{Attrs: tcell.AttrReverse},
		})
	}

	c.SetLocationList(internalLocationListPriority, selectionLocationListID, LocationSlice(locs))

	return
}

// returns ok=false if there's no content to select in buffer
func (c *Cursor) cursorAtScrollBounds() (pos term.Coordinates, ok bool) {
	rows := c.rows()
	if rows == 0 {
		pos = term.Coordinates{}
		return
	}

	pos = c.cursorAtScroll()
	ok = true

	if pos.Y >= rows {
		pos.Y = rows
		pos.X = 0
		return
	}

	pos.X = min(pos.X, c.view().Columns(pos.Y))
	return
}

// SelectionMode returns the current SelectMode if any.
func (c *Cursor) SelectionMode() (mode SelectMode, ok bool) {
	mode = c.selection.mode
	ok = mode != NoSelection
	return
}

// Select anchors the current cursor position as the start of a text selection.
// In order to unset anchor, use Unselect(). It returns true if cursor is in bounds or
// false if selection failed. If cursor has already been called one of the Select methods,
// then this method switches to the new mode and maintains original cursor position.
func (c *Cursor) Select() (ok bool) {
	mode := c.selection.mode
	c.selection.mode = StandardSelection
	if mode != NoSelection {
		ok = c.setSelection()
		return
	}
	c.selection.scrollFrom, ok = c.cursorAtScrollBounds()
	if !ok {
		return
	}
	ok = c.setSelection()
	return
}

// SelectLine anchors the current cursor position as the start of a line selection.
// In order to unset anchor, use Unselect(). It returns true if cursor is in bounds or
// false if selection failed. If cursor has already been called one of the Select methods,
// then this method switches to the new mode and maintains original cursor position.
func (c *Cursor) SelectLine() (ok bool) {
	mode := c.selection.mode
	c.selection.mode = LineSelection
	if mode != NoSelection {
		ok = c.setSelection()
		return
	}
	c.selection.scrollFrom, ok = c.cursorAtScrollBounds()
	if !ok {
		return
	}
	ok = c.setSelection()
	return
}

// SelectBlock anchors the current cursor position as the start of a block selection.
// In order to unset anchor, use Unselect(). It returns true if cursor is in bounds or
// false if selection failed. If cursor has already been called one of the Select methods,
// then this method switches to the new mode and maintains original cursor position.
func (c *Cursor) SelectBlock() (ok bool) {
	mode := c.selection.mode
	c.selection.mode = BlockSelection
	if mode != NoSelection {
		ok = c.setSelection()
		return
	}
	c.selection.scrollFrom, ok = c.cursorAtScrollBounds()
	if !ok {
		return
	}
	ok = c.setSelection()
	return
}

// Unselect resets the current selection anchor.
func (c *Cursor) Unselect() bool {
	if c.selection.mode == NoSelection {
		return false
	}
	c.selection.mode = NoSelection
	c.selection.cells = nil
	c.SetLocationList(internalLocationListPriority, selectionLocationListID, nil)
	return true
}

// Selection returns the current text under either text, line or block selection.
func (c *Cursor) Selection() string {
	return term.CellsToString(c.selection.cells)
}

// Redo reverses the previously reversed update to the underlying buffer.
func (c *Cursor) Redo() bool {
	enable := c.disablePublishing()
	defer enable()

	ok, at := c.buffer().Redo()
	if !ok {
		return false
	}
	c.setCursorAfterUpdate(at)
	return true
}

// Undo reverses the last update to the underlying buffer.
func (c *Cursor) Undo() bool {
	enable := c.disablePublishing()
	defer enable()

	ok, at := c.buffer().Undo()
	if !ok {
		return false
	}
	c.setCursorAfterUpdate(at)
	return true
}

// Line returns the row number of the row where the cursor is positioned.
func (c *Cursor) Line() int {
	return c.cursorAtScroll().Y
}

// Column returns the column number of the column where the cursor is positioned.
func (c *Cursor) Column() int {
	return c.cursorAtScroll().X
}

// Cell returns the cell where the cursor is positioned or false if there's no cell
// at the current cursor position.
func (c *Cursor) Cell() (term.Cell, bool) {
	return c.cellAtCursor()
}

// DeleteSelection deletes the current text under selection and returns true
// or does nothing and returns false.
func (c *Cursor) DeleteSelection() (ok bool) {
	if c.selection.mode == NoSelection {
		return
	}

	mode := c.selection.mode
	from := c.selection.scrollFrom
	to, ok := c.cursorAtScrollBounds()
	c.Unselect()

	// allow for subscribers of buffer to intercept via OnWillDelete
	// the current selection mode via SelectionMode.
	c.selection.mode = mode

	// this means that content was modified after Select started
	// and now there's no content to select, so we are done.
	if !ok {
		return
	}

	// buffer delete uses right exclusive semantics
	from, to = term.CoordinatesSort(from, to)
	if c.RightInclusiveSemantics {
		to.X++
	}

	var start term.Coordinates
	var str string
	switch mode {
	case StandardSelection:
		start, str = c.buffer().Delete(from, to)
	case LineSelection:
		start, str = c.buffer().DeleteLine(from, to)
	case BlockSelection:
		start, str = c.buffer().DeleteBlock(from, to)
	}
	c.selection.mode = NoSelection
	c.setCursorAfterUpdate(start)
	ok = str != ""
	return
}

// UppercaseSelection updates the current text under selection to upper case
// or does nothing and returns false.
func (c *Cursor) UppercaseSelection() (ok bool) {
	return c.selectionOp(func(cells string) string {
		return strings.ToUpper(cells)
	})
}

// LowercaseSelection updates the current text under selection to upper case
// or does nothing and returns false.
func (c *Cursor) LowercaseSelection() (ok bool) {
	return c.selectionOp(func(cells string) string {
		return strings.ToLower(cells)
	})
}

// TryIndent attempts to indent the cursor if an indent service is available.
func (c *Cursor) TryIndent() bool {
	pos := c.cursorAtScroll()
	svc := c.getIndentService()
	target, ok := svc.IndentationAt(pos.Y)
	if !ok {
		return false
	}
	after, ok := c.doTryIndent(c.ctx, pos, target)
	if ok {
		c.setCursorAfterUpdate(after)
		return true
	}
	after, ok = c.doTryDedent(c.ctx, pos, target)
	if ok {
		c.setCursorAfterUpdate(after)
		return true
	}
	return false
}

// ToggleHide either unhides the hidden block at cursor,
// or hides the current selection.
func (c *Cursor) ToggleHide() (ok bool) {
	if c.Unhide() {
		return true
	}
	return c.HideSelection()
}

// HideSelection hides the current text under selection and returns true
// or does nothing and returns false.
func (c *Cursor) HideSelection() (ok bool) {
	if c.selection.mode == NoSelection {
		return
	}

	from := c.selection.scrollFrom
	to, ok := c.cursorAtScrollBounds()
	c.Unselect()
	// this means that content was modified after Select started
	// and now there's no content to select, so we are done.
	if !ok {
		return
	}

	from, to = term.CoordinatesSort(from, to)
	if c.RightInclusiveSemantics {
		to.X++
	}
	ok = c.scroll.MarkHidden(from.Y, to.Y)
	return
}

// Unhide un-hides the block of hidden lines starting at the cursor position.
func (c *Cursor) Unhide() (ok bool) {
	at, ok := c.cursorAtScrollBounds()
	if !ok {
		return
	}
	ok = c.scroll.MarkVisible(at.Y)
	return
}

// CopySelection copies the current text under selection and returns true
// or does nothing and returns false.
func (c *Cursor) CopySelection(registerID string, clip clipboard.Register) (ok bool, err error) {
	if c.selection.mode == NoSelection ||
		(c.scroll.Width() == 0 && c.scroll.Wrap) {
		return
	}

	enable := c.disablePublishing()
	defer enable()

	selection := c.Selection()
	mode := c.selection.mode
	c.Unselect()
	c.moveToScroll(c.selection.scrollFrom)

	ok = true
	err = clip.Copy(registerID, clipboard.Data{Text: selection, Metadata: mode})
	return
}

// CopySelectionNoUnselect copies the current text under selection and returns true
// or does nothing and returns false, but as opposed to CopySelection,
// it keeps selection selected.
func (c *Cursor) CopySelectionNoUnselect(
	registerID string, clip clipboard.Register,
) (ok bool, err error) {
	if c.selection.mode == NoSelection {
		return
	}

	enable := c.disablePublishing()
	defer enable()

	selection := c.Selection()
	mode := c.selection.mode

	ok = true
	err = clip.Copy(registerID, clipboard.Data{Text: selection, Metadata: mode})
	return
}

// MoveToBounds moves the cursor up and to the left until it is in a row
// with content and it is 'padding' cells away from the last column in the row.
// If cursor is already in a row and/or in a column with content, then this method
// does nothing.
func (c *Cursor) MoveToBounds(padding int) {
	enable := c.disablePublishing()
	defer enable()

	for c.Line() < 0 && c.MoveLineDown() {
	}

	for c.Line() >= c.rows() && c.MoveLineUp() {
	}

	for c.Column() < 0 && c.MoveRight() {
	}

	for c.Line() < c.rows() && c.Column() >= c.view().Columns(c.Line())+padding && c.MoveLeft() {
	}
}

func (c *Cursor) moveToChar(
	ch rune, findResult func(int, cell.Searcher) (term.Coordinates, bool),
) bool {
	enable := c.disablePublishing()
	defer enable()

	cursor := c.cursorAtScroll()
	if cursor.Y >= c.rows() {
		return false
	}
	lastPos := c.view().Columns(cursor.Y)
	start := term.Coordinates{Y: cursor.Y}
	end := term.Coordinates{Y: cursor.Y, X: lastPos}

	cells, _, ok := c.buffer().Select(start, end)
	if !ok || len(cells) == 0 {
		return false
	}

	view := cell.NewView(cells)

	searcher := cell.NewSimpleSearcher(view)
	n := searcher.Search(string(ch))
	if n == 0 {
		return false
	}

	result, ok := findResult(n, searcher)
	if !ok { // results not aligned with direction
		return false
	}

	c.moveToScroll(term.Coordinates{Y: cursor.Y, X: result.X})

	return true
}

// MoveToNextChar moves the cursor to the next occurence of ch in the current line,
// from the cursor's current position.
func (c *Cursor) MoveToNextChar(ch rune) bool {
	cursor := c.cursorAtScroll()
	return c.moveToChar(ch, func(n int, searcher cell.Searcher) (term.Coordinates, bool) {
		for i := 0; i < n; i++ {
			result, _ := searcher.NextResult()
			if result.X > cursor.X {
				return result, true
			}
		}
		return term.Coordinates{}, false
	})
}

// MoveToPrevChar moves the cursor to the previous occurence of ch in the current line,
// from the cursor's current position.
func (c *Cursor) MoveToPrevChar(ch rune) bool {
	cursor := c.cursorAtScroll()
	return c.moveToChar(ch, func(n int, searcher cell.Searcher) (term.Coordinates, bool) {
		for i := 0; i < n; i++ {
			result, _ := searcher.PrevResult()
			if result.X < cursor.X {
				return result, true
			}
		}
		return term.Coordinates{}, false
	})
}

// ShiftLineRight shifts the current cursor's line one tab to the right.
func (c *Cursor) ShiftLineRight() {
	cursor := c.cursorAtScroll()
	c.buffer().ShiftRowRight(cursor.Y)
	cursor.X++
	c.setCursorAfterUpdate(cursor)
}

// ShiftLineLeft shifts the current cursor's line one tab to the left. It returns
// false if line's start of content is already at the start of the line.
func (c *Cursor) ShiftLineLeft() bool {
	cursor := c.cursorAtScroll()
	ok := c.buffer().ShiftRowLeft(cursor.Y)
	if !ok {
		return false
	}
	cursor.X--
	if cursor.X < 0 {
		cursor.X = 0
	}
	c.setCursorAfterUpdate(cursor)
	return true
}

func (c *Cursor) getShiftSelection() (from, to term.Coordinates) {
	from, to = c.selection.scrollFrom, c.cursorAtScroll()
	switch c.selection.mode {
	case BlockSelection:
		from, to = term.CoordinatesBlockSort(from, to)
	default:
		from, to = term.CoordinatesSort(from, to)
	}
	return
}

// ShiftSelectionRight shifts the current selection one tab to the right.
func (c *Cursor) ShiftSelectionRight() {
	from, to := c.getShiftSelection()

	c.Unselect()

	for y := from.Y; y <= to.Y; y++ {
		c.buffer().ShiftRowRight(y)
	}
}

// ShiftSelectionLeft shifts the current selection one tab to the left. It returns
// false if selection could not be shifted.
func (c *Cursor) ShiftSelectionLeft() (ok bool) {
	from, to := c.getShiftSelection()

	c.Unselect()

	for y := from.Y; y <= to.Y; y++ {
		sok := c.buffer().ShiftRowLeft(y)
		if sok {
			ok = true
		}
	}
	return
}

// LocationsAtCursor returns the set of locations by location list ID set by SetLocationList,
// at the current cursor position, if there's any. The returned slice is only valid
// until this method is called again.
func (c *Cursor) LocationsAtCursor() ([]textapi.Location, bool) {
	return c.locationStore.LocationsAtCoordinates(c.cursorAtScroll())
}

// SortedLocations returns all the locations sorted by priority level. If two
// location lists have the same priority level, then the location list ID is used
// to disambiguate order.
func (c *Cursor) SortedLocations() []textapi.Location {
	return c.locationStore.SortedLocations()
}

// SetLocationList sets a location list on this cursor. It substitutes and returns
// the previous location list with the same ID, if there was any.
// Any calls to Insert on the underlying Writer will reset all location lists.
func (c *Cursor) SetLocationList(
	pri textapi.LocationPriority, ID string, l LocationList,
) LocationList {
	return c.locationStore.SetLocationList(pri, ID, l)
}

// LocationLists returns a map of location list IDs to their
// respective locations.
func (c *Cursor) LocationLists() []LocationSet {
	return c.locationStore.LocationLists()
}

func (c *Cursor) endOfLocationList(
	l LocationList, op func(LocationList) (textapi.Location, bool),
) (term.Coordinates, bool) {
	prev, ok := l.Current()
	if !ok {
		return term.Coordinates{}, false
	}
	for {
		pos, ok := op(l)
		if !ok {
			return prev.From, true
		}
		prev = pos
	}
}

func (c *Cursor) movePastCursor(
	l LocationList, op, reverse func(LocationList) (textapi.Location, bool),
	continueIf func(*Cursor, term.Coordinates, term.Coordinates) bool,
) bool {
	enable := c.disablePublishing()
	defer enable()

	_, gotLocations := c.endOfLocationList(l, reverse)
	if !gotLocations {
		return false
	}

	cursor := c.cursorAtScroll()
	for {
		pos, _ := l.Current()
		if !continueIf(c, cursor, pos.From) {
			c.moveToScroll(pos.From)
			break
		}

		_, ok := op(l)
		if ok {
			continue
		}

		pos.From, _ = c.endOfLocationList(l, reverse)
		c.moveToScroll(pos.From)
		break
	}

	return cursor != c.cursorAtScroll()
}

// MoveToNextLocation moves the cursor to the next position returned by the location
// list set by SetLocationList. If there isn't a location list set, this method returns
// false.
func (c *Cursor) MoveToNextLocation(ID string) bool {
	l, ok := c.locationStore.LocationList(ID)
	if !ok {
		return false
	}

	return c.movePastCursor(l, (LocationList).Next,
		(LocationList).Prev, isBeforeCursorOrInsideHiddenBlock)
}

// MoveToPrevLocation moves the cursor to the previous position returned by the
// location list set by SetLocationList. If there isn't a location list set,
// this method returns false.
func (c *Cursor) MoveToPrevLocation(ID string) bool {
	l, ok := c.locationStore.LocationList(ID)
	if !ok {
		return false
	}

	return c.movePastCursor(l, (LocationList).Prev,
		(LocationList).Next, isPastCursor)
}

// LocationList returns the location list identified by ID, set previously via SetLocationList,
// or nil and false, if no location list is currently set with the given ID.
func (c *Cursor) LocationList(ID string) (LocationList, bool) {
	return c.locationStore.LocationList(ID)
}

// Word returns the word under the cursor or an empty string if
// the token under cursor is not a word.
func (c *Cursor) Word() string {
	_, _, word := c.scroll.WordAt(c.cursorAtScroll())
	return word
}

// SubscribeScroll subscribes subs to scroll events. This should be
// prefered over subscribing directly to scroll because some
// cursor movements are composite movements that would trigger
// multiple OnSeek dispatches rather than a single one.
func (c *Cursor) SubscribeScroll(subs component.ScrollSubscriber) {
	c.scroll.Subscribe(subs)
}

// ScrollCoordinates translates window coordinates to the scroll coordinates system.
func (c *Cursor) ScrollCoordinates(pos term.Coordinates) term.Coordinates {
	return c.scroll.WindowToScrollCoordinates(pos)
}

// WindowCoordinates translates scroll coordinates to the window coordinates system.
func (c *Cursor) WindowCoordinates(pos term.Coordinates) (term.Coordinates, bool) {
	if c.scroll.Width() == 0 && c.scroll.Wrap {
		// best effort conversion, if scroll width is 0 assume no wrap
		return term.CoordinatesDiff(pos, c.scroll.Offset()), true
	}
	return c.scroll.ScrollToWindowCoordinates(pos)
}

// MoveLineDown is equivalent to MoveDown in non wrap mode. In wrap mode,
// rather than going down one row, the cursor goes down to the line above.
func (c *Cursor) MoveLineDown() bool {
	pos := c.cursorAtScroll()
	pos.Y++
	if pos.Y >= c.rows() {
		return false
	}
	_, ok := c.MoveToScroll(pos)
	return ok
}

// MoveLineUp is equivalent to MoveUp in non wrap mode. In wrap mode,
// rather than going up one row, the cursor goes up to the line below.
func (c *Cursor) MoveLineUp() bool {
	pos := c.cursorAtScroll()
	pos.Y--
	if pos.Y < 0 {
		return false
	}
	_, ok := c.MoveToScroll(pos)
	return ok
}

// FoldAt runs the given callback if a fold is found at or around the
// current cursor position or returns false if folds are not enabled.
func (c *Cursor) FoldAt(ctx context.Context, cb func(term.Range, bool)) bool {
	if c.scheduleNextTick == nil {
		return false
	}

	if block, ok := c.scroll.HiddenBlockAt(c.cursorAtScroll().Y); ok {
		cb(block, ok)
		return true
	}

	return c.opFoldsFrom(ctx, c.scroll.Offset(), func(folds []term.Range) {
		pos := c.cursorAtScroll()

		var ok bool
		var found term.Range
		for _, fold := range folds {
			if pos.Y < fold.Start.Y || pos.Y > fold.End.Y {
				continue
			}
			if !ok || (fold.Start.Y > found.Start.Y || fold.End.Y < found.End.Y) {
				found = fold
				ok = true
			}
		}
		cb(found, ok)
	})
}

// CollapseFold collapses the fold at the current cursor position,
// or returns false if folds are not enabled.
func (c *Cursor) CollapseFold(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.FoldAt(ctx, func(fold term.Range, ok bool) {
		if !ok {
			return
		}
		c.scroll.MarkHidden(fold.Start.Y, fold.End.Y)
	})
}

// ExpandFold expands the fold at the current cursor position,
// or returns false if folds are not enabled.
func (c *Cursor) ExpandFold(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.FoldAt(ctx, func(fold term.Range, ok bool) {
		if !ok {
			return
		}
		mark := c.Mark()
		if c.scroll.MarkVisible(fold.Start.Y) {
			c.MoveToMark(mark)
		}
	})
}

// SelectFold selects the fold at the current cursor position,
// or returns false if folds are not enabled.
func (c *Cursor) SelectFold(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.FoldAt(ctx, func(fold term.Range, ok bool) {
		if !ok {
			return
		}
		c.MoveToScroll(fold.End)
		c.selection.scrollFrom = fold.Start
		c.selection.mode = StandardSelection
		c.setSelection()
	})
}

// ToggleFold toggles the fold at the current cursor position,
// or returns false if folds are not enabled.
func (c *Cursor) ToggleFold(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.FoldAt(ctx, func(fold term.Range, ok bool) {
		if !ok {
			return
		}
		if hidden, ok := c.isFoldHidden(fold.Start, fold.End); hidden || !ok {
			c.scroll.MarkVisible(fold.Start.Y)
		} else if ok {
			c.scroll.MarkHidden(fold.Start.Y, fold.End.Y)
		}
	})
}

// CollapseAllFolds collapses all the folds available in the file,
// or returns false if folds are not enabled.
func (c *Cursor) CollapseAllFolds(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.opFolds(ctx, func(folds []term.Range) {
		for _, fold := range folds {
			c.scroll.MarkHidden(fold.Start.Y, fold.End.Y)
		}
	})
}

// ExpandAllFolds expands all the folds available in the file,
// or returns false if folds are not enabled.
func (c *Cursor) ExpandAllFolds(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.opFolds(ctx, func(folds []term.Range) {
		mark := c.Mark()
		var handled bool
		for _, fold := range folds {
			handled = c.scroll.MarkVisible(fold.Start.Y) || handled
		}
		if handled {
			c.MoveToMark(mark)
		}
	})
}

// ToggleAllFolds toggles all the folds available in the file,
// or returns false if folds are not enabled.
func (c *Cursor) ToggleAllFolds(ctx context.Context) bool {
	if c.scheduleNextTick == nil {
		return false
	}
	return c.opFolds(ctx, func(folds []term.Range) {
		// first determine if they're currently hidden or visible:
		// if we start toggling as we're iterating, nested folds
		// will be incorrectly categorized.
		var visible, hidden []term.Range
		for _, fold := range folds {
			if isHidden, ok := c.isFoldHidden(fold.Start, fold.End); isHidden || !ok {
				visible = append(visible, fold)
			} else if ok {
				hidden = append(hidden, fold)
			}
		}
		for _, fold := range visible {
			c.scroll.MarkVisible(fold.Start.Y)
		}
		for _, fold := range hidden {
			c.scroll.MarkHidden(fold.Start.Y, fold.End.Y)
		}
	})
}

// FindMatchingRuneForward tries to find the target's matching rune by scrolling
// through cells forward, until either a match is found or until the end of the file,
// in which case false is returned.
func (c *Cursor) FindMatchingRuneForward(target, match rune) (term.Coordinates, bool) {
	return c.findMatchRune(target, match, c.matchRuneForward)
}

// FindMatchingRuneBackward tries to find the target's matching rune by scrolling
// through cells backward, until either a match is found or until the end of the file,
// in which case false is returned.
func (c *Cursor) FindMatchingRuneBackward(target, match rune) (term.Coordinates, bool) {
	return c.findMatchRune(target, match, c.matchRuneBackward)
}

func (c *Cursor) disablePublishing() func() {
	if !c.scroll.PublishingEnabled() {
		return func() {}
	}
	enable := c.scroll.DisablePublishing()
	return func() {
		c.seekToScrollCoordinates()
		enable()
	}
}

func (c *Cursor) matchRuneForward(pos *term.Coordinates) bool {
	lastRow := c.rows() - 1
	pos.X++
	for pos.Y <= lastRow && pos.X >= c.view().Columns(pos.Y) {
		pos.Y++
		pos.X = 0
	}
	return pos.Y <= lastRow || (pos.Y == lastRow && pos.X < c.view().Columns(pos.Y))
}

func (c *Cursor) matchRuneBackward(pos *term.Coordinates) bool {
	pos.X--
	for pos.Y > 0 && pos.X < 0 {
		pos.Y--
		pos.X = c.view().Columns(pos.Y) - 1
	}
	return pos.Y >= 0 && pos.X >= 0
}

func (c *Cursor) moveMatchRuneForward(target, match rune) bool {
	pos, ok := c.findMatchRune(target, match, c.matchRuneForward)
	if !ok {
		return false
	}
	_, ok = c.MoveToScroll(pos)
	return ok
}

func (c *Cursor) findMatchRune(
	target, match rune,
	advance func(*term.Coordinates) bool,
) (term.Coordinates, bool) {
	cells := c.view().RawCells()
	pos := c.cursorAtScroll()
	pending := 1
	for pending != 0 && advance(&pos) {
		switch cells[pos.Y][pos.X].Ch {
		case target:
			pending++
		case match:
			pending--
		}
	}

	if pending == 0 {
		return pos, true
	}
	return term.Coordinates{}, false
}

func (c *Cursor) moveMatchRuneBackward(target, match rune) bool {
	pos, ok := c.findMatchRune(target, match, c.matchRuneBackward)
	if !ok {
		return false
	}
	_, ok = c.MoveToScroll(pos)
	return ok
}

func (c *Cursor) setCursorAfterUpdate(atScroll term.Coordinates) {
	if c.scroll.Width() == 0 && c.scroll.Wrap {
		// nothing should be updating if width of the scroll is 0!
		return
	}
	c.scroll.RecalculateWraps()
	res, _ := c.scroll.ScrollToWindowCoordinates(atScroll)
	c.setCursor(res, c.shouldSeek)
}

func (c *Cursor) getIndentation(pos term.Coordinates) (ret int, ok bool) {
	cells := c.buffer().RawCells()
	if pos.Y >= len(cells) {
		return
	}
	for x, cell := range cells[pos.Y] {
		switch cell.Ch {
		case '\t':
			ret++
		default:
			ok = x == pos.X
			return
		}
	}
	return
}

func (c *Cursor) tryIndent(ctx context.Context, to term.Coordinates) (term.Coordinates, bool) {
	svc := c.getIndentService()
	indentation, ok := svc.IndentationAt(to.Y)
	if !ok {
		return to, false
	}
	return c.doTryIndent(ctx, to, indentation)
}

func (c *Cursor) doTryIndent(ctx context.Context, to term.Coordinates, target int) (term.Coordinates, bool) {
	current, _ := c.getIndentation(to)
	diff := target - current
	if diff <= 0 {
		c.log(log.DebugLevel, "try indent: already equal or more than correct indentation: %d", target)
		return to, false
	}

	var builder strings.Builder
	for range diff {
		builder.WriteByte('\t')
	}
	buf := c.buffer()
	tabs := builder.String()
	// even if given position to indent is not at the start of the line
	// to "indent" we must resolve to start of line
	at := term.Coordinates{Y: to.Y}
	_, _, _ = buf.Edit(ctx, at, at, tabs)
	to.X += diff
	return to, true
}

func (c *Cursor) tryDedent(ctx context.Context, pos term.Coordinates) (
	term.Coordinates, bool,
) {
	svc := c.getIndentService()
	indentation, ok := svc.IndentationAt(pos.Y)
	if !ok {
		return pos, false
	}
	return c.doTryDedent(ctx, pos, indentation)
}

func (c *Cursor) doTryDedent(ctx context.Context, pos term.Coordinates, target int) (
	term.Coordinates, bool,
) {
	buf := c.buffer()
	current, _ := c.getIndentation(pos)
	diff := current - target
	if diff <= 0 {
		c.log(log.TraceLevel, "try dedent: already equal or less than correct indentation: %d", target)
		return pos, false
	}
	// start of edit should be at the end of starting tabs block
	from := term.Coordinates{X: (current - diff), Y: pos.Y}
	to := term.Coordinates{X: current, Y: pos.Y}
	if diff <= 0 || from.X < 0 || to.X < 0 {
		c.log(log.TraceLevel, "try dedent: could not dedent at (%v), current: %d, "+
			"should be: %d, from: %v, to: %v", pos, current, target, from, to)
		return pos, false
	}
	cells, _, ok := buf.Select(from, to)
	empty := isEmpty(cells[0])
	if ok && len(cells) != 0 && empty {
		buf.Edit(ctx, from, to, "")
		pos.X -= diff
		return pos, true
	}
	c.log(log.TraceLevel, "try dedent: could not dedent at (%v), cells: %#v, from: %#v, to: %#v ",
		pos, empty, from, to)
	return pos, false
}

func isEmpty(cells []term.Cell) bool {
	for _, cell := range cells {
		switch cell.Ch {
		case ' ', '\t':
			continue
		}
		return false
	}
	return true
}

func (c *Cursor) getIndentService() indentService {
	svc, ok := c.buffer().View().(indentService)
	if ok {
		return svc
	}
	return nopIndentService{}
}

// either op is invoked or this function returns false
func (c *Cursor) opFolds(ctx context.Context, op func([]term.Range)) bool {
	svc, ok := c.buffer().View().(foldsService)
	if !ok {
		return false
	}

	folds, ok := svc.Folds()
	if !ok {
		return false
	}
	c.opFoldsIter(ctx, op, folds)
	return true
}

func (c *Cursor) opFoldsFrom(
	ctx context.Context, from term.Coordinates, op func([]term.Range),
) bool {
	svc, ok := c.buffer().View().(foldsService)
	if !ok {
		return false
	}

	folds, ok := svc.FoldsFrom(from)
	if !ok {
		return false
	}
	c.opFoldsIter(ctx, op, folds)
	return true
}

func (c *Cursor) opFoldsIter(
	ctx context.Context, op func([]term.Range), folds iterator.Iterator[term.Range],
) {
	go debug.CapturePanicReport(func() {
		folds, isEmpty := iterator.IsEmpty(ctx, folds)
		if isEmpty {
			op(nil)
			return
		}
		c.scheduleNextTick(func() {
			defer folds.Close()
			// this needs to roughly follow the same algorithm used by aux_bar
			m := make(map[term.Coordinates]term.Coordinates)
			for {
				fold, ok := folds.Next(ctx)
				if !ok {
					break
				}
				if fold.Start.Y >= fold.End.Y {
					continue
				}
				fold.Start.X = 0 // avoid ambiguity
				if end, exists := m[fold.Start]; exists && end.Y > fold.End.Y {
					continue
				}
				m[fold.Start] = fold.End
			}
			if err := folds.Err(); err != nil {
				c.log(log.ErrorLevel, "error getting folds: %v", err)
				op(nil)
				return
			}
			seq := maps.All(m)
			slice := make([]term.Range, 0, len(m))
			for a, b := range seq {
				slice = append(slice, term.Range{Start: a, End: b})
			}
			sort.Slice(slice, func(i, j int) bool {
				return slice[i].Start.Y < slice[j].Start.Y
			})
			op(slice)
		})
	})
}

func (c *Cursor) isFoldHidden(start, end term.Coordinates) (bool, bool) {
	// convert folds which are scroll coordinates to window coordinates
	foldStart, startOk := c.scroll.ScrollToWindowCoordinates(start)
	foldEnd, endOk := c.scroll.ScrollToWindowCoordinates(end)
	if foldStart.Y != foldEnd.Y && (!startOk || !endOk) {
		// inside hidden block
		return false, false
	}
	return foldStart.Y == foldEnd.Y, true
}

func (c *Cursor) selectionOp(fn func(string) string) (ok bool) {
	if c.selection.mode == NoSelection {
		return
	}

	mode := c.selection.mode
	from := c.selection.scrollFrom
	to, ok := c.cursorAtScrollBounds()
	c.Unselect()
	c.selection.mode = mode
	if !ok {
		return
	}

	// buffer delete uses right exclusive semantics
	from, to = term.CoordinatesSort(from, to)
	if c.RightInclusiveSemantics {
		to.X++
	}

	var cells [][]term.Cell
	switch mode {
	case StandardSelection:
		cells, _, ok = c.buffer().Select(from, to)
		if ok {
			str := term.CellsToString(cells)
			c.log(log.TraceLevel, "selectionOp: replace with cells: %#v, from: %v, to: %v", cells, from, to)
			c.buffer().Edit(c.ctx, from, to, fn(str))
		}
	case LineSelection:
		cells, _, ok = c.buffer().SelectLine(from, to)
		if ok {
			c.buffer().Edit(c.ctx, from, to, fn(term.CellsToString(cells)))
		}
	case BlockSelection:
		ok = false
		return
		/* not supported at the moment
		cells, _, ok = c.buffer().SelectBlock(from, to)
		if ok {
			_, str := c.buffer().DeleteBlock(from, to)
			c.InsertBlock(fn(str))
		}
		*/
	}
	if !ok {
		return
	}
	c.selection.mode = NoSelection
	return
}

func (c *Cursor) setSearchLocationList(text string, word bool) int {
	c.search = text
	n := c.scroll.Search(text)

	searchLoc := make([]textapi.Location, 0, n)
	for range n {
		res, ok := c.scroll.NextResult()
		if !ok {
			panic("invalid scroll search results")
		}
		if word {
			_, _, wordAtPos := c.scroll.WordAt(res)
			// in word mode, if word doesn't match exactly
			// continue with the next result
			if text != wordAtPos {
				continue
			}
		}
		searchLoc = append(searchLoc, textapi.Location{
			From: res,
			To:   term.Coordinates{Y: res.Y, X: res.X + len(text)},
			Attr: c.searchAttr,
		})
	}

	c.SetLocationList(internalLocationListPriority, searchLocationListID, LocationSlice(searchLoc))
	return len(searchLoc)
}

func (c *Cursor) tryMoveDownWindowRow() bool {
	atScroll := c.cursorAtScroll()
	win, _ := c.scroll.ScrollToWindowCoordinates(atScroll)
	win.Y++
	atScroll = c.scroll.WindowToScrollCoordinates(win)
	if atScroll.Y >= c.rows() {
		return false
	}
	_, ok := c.MoveToScroll(atScroll)
	return ok
}

func (c *Cursor) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "text.Cursor").Logf(level, msg, args...)
}
