package editor

import (
	"fmt"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
)

// SelectMode represents a select mode.
type SelectMode uint8

const (
	noSelection SelectMode = iota
	// StandardSelection represents a select mode. See Select for more details.
	StandardSelection
	// LineSelection represents a select mode. See SelectLine for more details.
	LineSelection
	// BlockSelection represents a select mode. See SelectBlock for more details.
	BlockSelection

	searchLocationListID = "search"
)

// used to subscribe to buffer updates
type curSubscriber struct {
	c *Cursor
}

type message struct {
	listID   string
	location Location
}

// Cursor is a helper structure which manages a cursor over a Scroll.
type Cursor struct {
	scroll    *component.Scroll
	search    string
	cursor    term.Coordinates
	locs      map[string]LocationList
	messages  map[term.Coordinates][]message
	selection struct {
		mode       SelectMode
		scrollFrom term.Coordinates
		cells      [][]term.Cell
	}
	subscriber curSubscriber
}

// NewCursor allocates storage for a new cursor,
// initializes it with an empty Scroll, and returns it.
func NewCursor(scroll *component.Scroll) *Cursor {
	c := new(Cursor)
	c.Init(scroll)
	return c
}

// Init initializes this cursor with the given scroll and subscribes
// to changes to the scroll's buffer. If buffer is swapped
// via Scroll.SetBuffer, consider re-initializing this cursor with the
// updated scroll, unless it's a temporary swap.
// Note that this Cursor implementation does not support text wrap mode,
// so scroll.Wrap should be false. Also, once initialized
// this cursor MUST NOT be copied.
func (c *Cursor) Init(scroll *component.Scroll) {
	if scroll.Wrap == true {
		panic("Cursor does not support wrap mode yet")
	}
	c.cursor = term.Coordinates{}
	c.scroll = scroll
	c.selection.mode = noSelection
	c.subscriber.c = c
	c.locs = make(map[string]LocationList)
	c.messages = make(map[term.Coordinates][]message)

	c.scroll.Buffer().Subscribe(&c.subscriber)
}

func (c *curSubscriber) clearAllLocations() {
	for _, list := range c.c.locs {
		c.c.setLocListAttr(list, true)
	}
	// this is optimized by the compiler starting at go 1.11
	for k := range c.c.locs {
		delete(c.c.locs, k)
	}
	for k := range c.c.messages {
		delete(c.c.messages, k)
	}
}

func (c *curSubscriber) OnWillInsert(at term.Coordinates, str string) {
	c.clearAllLocations()
}

func (c *curSubscriber) OnDidInsert(from, to term.Coordinates) {
	c.c.setSearchLocationList(c.c.search)
}

func (c *curSubscriber) OnWillDelete(from, to term.Coordinates) {
	c.clearAllLocations()
}

func (c *curSubscriber) OnDidDelete(start, end term.Coordinates, str string) {
	c.c.setSearchLocationList(c.c.search)
}

// Cursor returns the current position of the cursor. It safisfies tui.Handler.Cursor.
func (c *Cursor) Cursor() (term.Coordinates, bool) {
	return c.cursor, true
}

func (c *Cursor) buffer() *cell.Buffer {
	return c.scroll.Buffer()
}

// MoveTo moves the cursor to thw window relative position pos.
func (c *Cursor) MoveTo(pos term.Coordinates) term.Coordinates {
	ret := c.cursor
	c.cursor = pos
	return ret
}

// CursorAtScroll returns the current position of the cursor relative
// to the scroll coorindates.
func (c *Cursor) CursorAtScroll() term.Coordinates {
	return c.cursorAtScroll()
}

// SelectionFrom returns the position of the current selection,
// if cursor is in select mode.
func (c *Cursor) SelectionFrom() (pos term.Coordinates, ok bool) {
	if c.selection.mode == noSelection {
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
	if pos.Y >= c.scroll.Buffer().Rows() {
		return
	}
	if pos.X > c.scroll.Buffer().Columns(pos.Y) {
		return
	}
	ok = true
	ret = c.cursorAtScroll()
	c.moveToScroll(pos)
	return
}

// the bounds of the current view, then underlying scroll is used
// to seek to pos.
func (c *Cursor) moveToScroll(pos term.Coordinates) {
	offset := c.scroll.Offset()
	height := c.scroll.Height()
	if pos.Y >= height+offset.Y {
		c.scroll.SeekTo(term.Coordinates{X: pos.X, Y: pos.Y})
	}
	c.cursor = c.scrollToWindowCoordinates(pos)
}

// note that pos is window coordinates, not scroll coordinates
func (c *Cursor) setCursor(pos term.Coordinates) {
	c.cursor = pos

	if c.selection.mode != noSelection {
		c.setSelection()
	}
}

func (c *Cursor) setSearchLocationList(text string) int {
	c.search = text
	if c.search == "" {
		return 0
	}

	n := c.scroll.Search(text)

	searchLoc := make([]Location, n)
	for i := 0; i < n; i++ {
		res, ok := c.scroll.NextResult()
		if !ok {
			panic("invalid scroll search results")
		}
		searchLoc[i] = Location{
			From: res,
			// To: is not necessary for MoveToNextLocation
		}
	}

	c.SetLocationList(searchLocationListID, LocationSlice(searchLoc))
	return n
}

// Search searches text string in the underlying cell buffer. It returns
// the number of occurrences found.
func (c *Cursor) Search(text string) int {
	n := c.setSearchLocationList(text)
	return n
}

func isBeforeCursor(cursor, pos term.Coordinates) bool {
	return pos.Y < cursor.Y || (pos.Y == cursor.Y && pos.X <= cursor.X)
}

func isPastCursor(cursor, pos term.Coordinates) bool {
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
	ok = c.scroll.SeekStartLine()
	pos := term.Coordinates{X: 0, Y: c.cursor.Y}
	if !ok {
		ok = pos != c.cursor
	}
	c.setCursor(pos)
	return
}

// MoveEndLine moves the cursor at the end of the current line, scrolling
// to the end of the line if required.
func (c *Cursor) MoveEndLine() (ok bool) {
	y := c.cursorAtScroll().Y
	if y >= c.buffer().Rows() {
		c.setCursor(term.Coordinates{X: 0, Y: c.cursor.Y})
		return
	}

	cols := c.buffer().Columns(y)
	width := c.scroll.Width()
	if cols == 0 || width == 0 {
		return
	}

	var didSeek bool
	for cols > c.scroll.Offset().X+width {
		didSeek = true
		c.scroll.SeekRight()
	}

	// note that padding is subject to limits imposed by Scroll's max offset.
	// If we want to add padding even for the longest line of the scroll,
	// we should add some padding to the max X offset of the scroll.
	const padding = 10
	if didSeek {
		for i := 0; i < padding; i++ {
			c.scroll.SeekRight()
		}
	}

	// handle cursor *past* end of line
	var pos term.Coordinates
	for {
		pos = term.Coordinates{X: cols - c.scroll.Offset().X - 1, Y: c.cursor.Y}
		if pos.X >= 0 {
			break
		}
		c.scroll.SeekLeft()
	}
	ok = c.cursor != pos
	c.setCursor(pos)

	return
}

// MoveFirstLine moves the cursor to the first line, scrolling the content
// if appplicable.
func (c *Cursor) MoveFirstLine() (ok bool) {
	ok = c.scroll.SeekStartFile()
	pos := term.Coordinates{}
	if !ok {
		ok = pos != c.cursor
	}
	c.setCursor(pos)
	return
}

// MoveLastLine moves the cursor to the last line, scrolling the content
// if required.
func (c *Cursor) MoveLastLine() (ok bool) {
	height := c.scroll.Height()
	rows := c.buffer().Rows()
	if height == 0 || rows == 0 {
		return
	}

	ok = c.scroll.SeekEndFile()
	pos := term.Coordinates{X: 0, Y: rows - c.scroll.Offset().Y - 1}
	if !ok {
		ok = pos != c.cursor
	}
	c.setCursor(pos)

	return
}

// MoveDown moves the cursor to the line under the current line, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveDown() (ok bool) {
	if c.cursor.Y+1 >= c.scroll.Height() {
		ok = c.scroll.SeekDown()
		c.setCursor(c.cursor)
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: c.cursor.X, Y: c.cursor.Y + 1})
	return
}

// MoveUp moves the cursor the the line above the current line, scrolling
// the content up if required.
func (c *Cursor) MoveUp() (ok bool) {
	if c.cursor.Y == 0 {
		ok = c.scroll.SeekUp()
		c.setCursor(c.cursor)
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: c.cursor.X, Y: c.cursor.Y - 1})
	return
}

// MoveLeft moves the cursor to the cell left of the current cell, scrolling
// the content left if required.
func (c *Cursor) MoveLeft() (ok bool) {
	if c.cursor.X == 0 {
		ok = c.scroll.SeekLeft()
		c.setCursor(c.cursor)
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: c.cursor.X - 1, Y: c.cursor.Y})
	return
}

// MoveRight moves the cursor to the cell right of the current cell, scrolling
// the content right if required.
func (c *Cursor) MoveRight() (ok bool) {
	if c.cursor.X+1 >= c.scroll.Width() {
		ok = c.scroll.SeekRight()
		c.setCursor(c.cursor)
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: c.cursor.X + 1, Y: c.cursor.Y})
	return
}

// MoveLeftWrap will move the cursor to the left or wrap to end of
// previous line if cursor is at X=0
func (c *Cursor) MoveLeftWrap() bool {
	// we cannot use 0 as a wrap coordinate because what we really want
	// is to return false when we cannot move which depends on the type of movc.
	// Returing false when reached 0, in the case of moving one cell at a time is correct
	// but not when we jump from start of word to the next
	currc, curro := c.cursor, c.scroll.Offset()
	c.MoveLeft()
	if c.cursor.X == currc.X && c.scroll.Offset().X == curro.X {
		c.MoveUp()
		if c.cursor.Y == currc.Y && c.scroll.Offset().Y == curro.Y {
			return false
		}
		c.MoveEndLine()
	}
	return true
}

// MoveRightWrap will move the cursor to the right or wrap to beginning
// of next line if cursor is at X=EOL
func (c *Cursor) MoveRightWrap() bool {
	currc, curro := c.cursor, c.scroll.Offset()
	c.MoveRight()

	if currc.X == c.cursor.X && c.scroll.Offset().X == curro.X {
		c.MoveDown()
		if currc.Y == c.cursor.Y && c.scroll.Offset().Y == curro.Y {
			return false
		}
		c.MoveStartLine()
	}
	return true
}

func (c *Cursor) cursorAtScroll() term.Coordinates {
	return c.windowToScrollCoordinates(c.cursor)
}

func (c *Cursor) cellAtCursor() (cell term.Cell, ok bool) {
	cursorAtScroll := c.cursorAtScroll()
	cells := c.buffer().RawCells()
	if cursorAtScroll.Y >= len(cells) ||
		cursorAtScroll.X >= len(cells[cursorAtScroll.Y]) {
		return
	}
	ok = true
	cell = cells[cursorAtScroll.Y][cursorAtScroll.X]
	return
}

func isOneOf(cell term.Cell, special []rune) bool {
	for _, r := range special {
		if r == cell.Ch {
			return true
		}
	}
	return false
}

func isNoneOf(cell term.Cell, skip []rune) (none bool) {
	none = true
	for _, r := range skip {
		none = none && cell.Ch != r
	}
	return
}

func (c *Cursor) moveAfterRune(skip, special []rune, move func() bool) (ok bool) {
	const (
		init = iota
		foundRune
	)

	initialPos := c.cursor
	initialOffset := c.scroll.Offset()
	lastSanePos := initialPos
	lastSaneOffset := initialOffset
	state := init

	cell, cOk := c.cellAtCursor()
	if !cOk {
		return
	}

	if isOneOf(cell, special) {
		state = foundRune
	}

	for move() {
		cell, cOk := c.cellAtCursor()
		if !cOk {
			continue
		}
		ok = true
		lastSanePos = c.cursor
		lastSaneOffset = c.scroll.Offset()

		// newline should stop the procedure
		if initialPos.Y != lastSanePos.Y ||
			initialOffset.Y != lastSaneOffset.Y {
			state = foundRune
		}
		// also finding a special character
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

	c.revertTo(lastSanePos, lastSaneOffset)
	return
}

func (c *Cursor) revertTo(pos, offset term.Coordinates) {
	c.setCursor(pos)
	c.scroll.SetOffset(offset)
}

func (c *Cursor) moveBeforeRune(skip, all []rune, move func() bool) (ok bool) {
	const (
		skipRune = iota
		findRune
		done
	)

	state := skipRune
	initial := c.cursor
	initialOffset := c.scroll.Offset()
	prev := initial
	prevOffset := initialOffset

	for move() {
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
				prev = c.cursor
				prevOffset = c.scroll.Offset()
			} else {
				state = findRune
			}
		case findRune:
			if isOneOf(cell, all) {
				state = done
			}
			if initial.Y != c.cursor.Y ||
				initialOffset.Y != c.scroll.Offset().Y {
				state = done
			}
		}
		if state == done {
			break
		}
		prev = c.cursor
		prevOffset = c.scroll.Offset()
	}

	if state == done {
		ok = true
		c.revertTo(prev, prevOffset)
	}
	return
}

var allSpecialCharacters = []rune{'.', ',', ':', ';', ' ', ')', '"',
	'\'', '(', '{', '}', '[', ']', '\t', '\x00', '\\', '/',
	'+', '`', '_'}

var skipCharacters = []rune{' ', '\t', '\x00', '_'}

// MoveRightStartWordGroup moves the cursor right to the start of the next word.
func (c *Cursor) MoveRightStartWordGroup() bool {
	return c.moveAfterRune(skipCharacters, skipCharacters, c.MoveRightWrap)
}

// MoveLeftStartWordGroup moves the cursor left to the start of the previous word.
func (c *Cursor) MoveLeftStartWordGroup() bool {
	return c.moveBeforeRune(skipCharacters, skipCharacters, c.MoveLeftWrap)
}

// MoveRightEndWordGroup moves the cursor right to the end of the next or current word.
func (c *Cursor) MoveRightEndWordGroup() bool {
	return c.moveBeforeRune(skipCharacters, skipCharacters, c.MoveRightWrap)
}

// MoveLeftEndWordGroup moves the cursor left to the end of the previous word.
func (c *Cursor) MoveLeftEndWordGroup() bool {
	return c.moveAfterRune(skipCharacters, skipCharacters, c.MoveLeftWrap)
}

// MoveRightStartWord moves the cursor right to the start of the next word.
func (c *Cursor) MoveRightStartWord() bool {
	return c.moveAfterRune(skipCharacters, allSpecialCharacters, c.MoveRightWrap)
}

// MoveLeftStartWord moves the cursor left to the start of the previous word.
func (c *Cursor) MoveLeftStartWord() bool {
	return c.moveBeforeRune(skipCharacters, allSpecialCharacters, c.MoveLeftWrap)
}

// MoveRightEndWord moves the cursor right to the end of the next or current word.
func (c *Cursor) MoveRightEndWord() bool {
	return c.moveBeforeRune(skipCharacters, allSpecialCharacters, c.MoveRightWrap)
}

// MoveLeftEndWord moves the cursor left to the end of the previous word.
func (c *Cursor) MoveLeftEndWord() bool {
	return c.moveAfterRune(skipCharacters, allSpecialCharacters, c.MoveLeftWrap)
}

func (c *Cursor) moveMatchRune(target, match rune, move func() bool) bool {
	currc, curro := c.cursor, c.scroll.Offset()
	pending := 1
	var prev, prevOffset term.Coordinates

	for pending != 0 && move() {
		prev = c.cursor
		prevOffset = c.scroll.Offset()
		cell, ok := c.cellAtCursor()
		if !ok {
			continue
		}
		switch cell.Ch {
		case target:
			pending++
		case match:
			pending--
		}
	}

	if pending == 0 {
		c.revertTo(prev, prevOffset)
		return true
	}

	c.revertTo(currc, curro)
	return false
}

// MoveToMatchingRune moves the cursor to the balanced matching rune of the rune at
// the current cursor's cell.
func (c *Cursor) MoveToMatchingRune() bool {
	cell, ok := c.cellAtCursor()
	if !ok {
		return ok
	}

	switch cell.Ch {
	case '[':
		ok = c.moveMatchRune('[', ']', c.MoveRightWrap)
	case '{':
		ok = c.moveMatchRune('{', '}', c.MoveRightWrap)
	case '(':
		ok = c.moveMatchRune('(', ')', c.MoveRightWrap)

	case ']':
		ok = c.moveMatchRune(']', '[', c.MoveLeftWrap)
	case '}':
		ok = c.moveMatchRune('}', '{', c.MoveLeftWrap)
	case ')':
		ok = c.moveMatchRune(')', '(', c.MoveLeftWrap)
	}

	return ok
}

// InsertRowAbove inserts a row above the current row and moves the cursor up.
func (c *Cursor) InsertRowAbove() {
	mode := c.selection.mode
	c.selection.mode = noSelection
	c.setSelection()

	cursorAtScroll := c.cursorAtScroll()
	if cursorAtScroll.Y == 0 {
		c.buffer().InsertRowAt(0)
		return
	}
	c.buffer().InsertRowAt(cursorAtScroll.Y - 1)
	c.selection.mode = mode
	c.setSelection()
	c.MoveUp()
}

// InsertRowBelow inserts a row below the current row and moves the cursor down.
func (c *Cursor) InsertRowBelow() {
	mode := c.selection.mode
	c.selection.mode = noSelection
	c.setSelection()

	cursorAtScroll := c.cursorAtScroll()
	c.buffer().InsertRowAt(cursorAtScroll.Y + 1)
	c.selection.mode = mode
	c.setSelection()
	c.MoveDown()
}

// Insert inserts rune at the current cursor's position.
func (c *Cursor) Insert(r rune) {
	mode := c.selection.mode
	c.selection.mode = noSelection
	c.setSelection()

	pos := c.buffer().Insert(c.cursorAtScroll(), r)
	c.selection.mode = mode
	c.setSelection()
	c.setCursor(c.scrollToWindowCoordinates(pos))
}

// InsertString inserts str at the current cursor's position.
func (c *Cursor) InsertString(str string) {
	mode := c.selection.mode
	c.selection.mode = noSelection
	c.setSelection()

	_, until := c.buffer().InsertString(c.cursorAtScroll(), str)
	c.selection.mode = mode
	c.setSelection()
	c.setCursor(c.scrollToWindowCoordinates(until))
}

// Delete deletes the cell at the current cursor position.
func (c *Cursor) Delete() (ok bool) {
	var pos term.Coordinates
	pos, _, ok = c.buffer().DeleteCell(c.cursorAtScroll())
	if ok {
		c.setCursor(c.scrollToWindowCoordinates(pos))
	}
	return
}

// Backspace is a special form of Delete, named after the keyboard key backspace.
func (c *Cursor) Backspace() (ok bool) {
	if c.cursor.X > 0 || c.scroll.Offset().X > 0 {
		c.MoveLeft()
		ok = c.Delete()
		return
	}

	if ok = c.MoveUp(); ok {
		c.Conflate()
	}
	return
}

// Conflate removes the new line character at the end of the current line.
func (c *Cursor) Conflate() (ok bool) {
	pos := c.cursorAtScroll()
	if pos.Y >= c.buffer().Rows() {
		ok = false
		return
	}

	ok = true

	c.MoveEndLine()
	length := c.buffer().Columns(pos.Y)
	if length == 0 {
		ok = c.buffer().DeleteRow(pos.Y)
		if !ok {
			panic(fmt.Sprintf("could not delete row at index: %d", pos.Y))
		}
		return
	}

	c.MoveRight()
	c.buffer().ConflateRow(pos.Y)
	return
}

func invertAttr(cells [][]term.Cell) {
	for i := 0; i < len(cells); i++ {
		for j := 0; j < len(cells[i]); j++ {
			c := cells[i][j]
			if c.Bg&term.AttrReverse == term.AttrReverse ||
				c.Fg&term.AttrReverse == term.AttrReverse {
				cells[i][j].Fg &^= term.AttrReverse
				cells[i][j].Bg &^= term.AttrReverse
			} else {
				cells[i][j].Fg |= term.AttrReverse
				cells[i][j].Bg |= term.AttrReverse
			}
		}
	}
}

func (c *Cursor) scrollToWindowCoordinates(pos term.Coordinates) term.Coordinates {
	pos = term.Coordinates{
		X: pos.X - c.scroll.Offset().X,
		Y: pos.Y - c.scroll.Offset().Y,
	}

	// scroll can return some coordinates that are be outside
	// of the bounds of the current window.
	// For instance, if DeleteCell deletes a tab, it could be that
	// pos.X at scroll yields a negative coordinate
	// (i.c. -3 if tabspaces is 4)
	for pos.X < 0 && c.scroll.SeekLeft() {
		pos.X++
	}
	for pos.Y < 0 && c.scroll.SeekUp() {
		pos.Y++
	}
	for pos.X >= c.scroll.Width() && c.scroll.SeekRight() {
		pos.X--
	}
	for pos.Y >= c.scroll.Height() && c.scroll.SeekDown() {
		pos.Y--
	}
	return pos
}

func (c *Cursor) windowToScrollCoordinates(pos term.Coordinates) term.Coordinates {
	return term.Coordinates{
		X: pos.X + c.scroll.Offset().X,
		Y: pos.Y + c.scroll.Offset().Y,
	}
}

func (c *Cursor) setSelection() {
	invertAttr(c.selection.cells)

	from := c.selection.scrollFrom
	to := c.cursorAtScroll()

	switch c.selection.mode {
	case StandardSelection:
		c.selection.cells = c.buffer().Select(from, to)
	case LineSelection:
		c.selection.cells = c.buffer().SelectLine(from, to)
	case BlockSelection:
		c.selection.cells = c.buffer().SelectBlock(from, to)
	case noSelection:
		c.selection.cells = nil
	}

	invertAttr(c.selection.cells)
}

// returns ok=false if there's no content to select in buffer
func (c *Cursor) cursorAtScrollBounds() (pos term.Coordinates, ok bool) {
	rows := c.buffer().Rows()
	if rows == 0 {
		pos = term.Coordinates{}
		return
	}

	pos = c.cursorAtScroll()
	ok = true

	if pos.Y >= rows {
		pos.Y = rows - 1
		pos.X = c.buffer().Columns(pos.Y)
		return
	}

	cols := c.buffer().Columns(pos.Y)
	if pos.X > cols {
		pos.X = cols
	}

	return
}

// SelectionMode returns the current SelectMode if any.
func (c *Cursor) SelectionMode() (mode SelectMode, ok bool) {
	mode = c.selection.mode
	ok = mode != noSelection
	return
}

// Select anchors the current cursor position as the start of a text selection.
// In order to unset anchor, use Unselect(). It returns true if cursor is in bounds or
// false if selection failed. If cursor has already been called one of the Select methods,
// then this method switches to the new mode and maintains original cursor position.
func (c *Cursor) Select() (ok bool) {
	mode := c.selection.mode
	c.selection.mode = StandardSelection
	if mode != noSelection {
		ok = true
		c.setSelection()
		return
	}
	c.selection.scrollFrom, ok = c.cursorAtScrollBounds()
	if !ok {
		return
	}
	c.setSelection()
	return
}

// SelectLine anchors the current cursor position as the start of a line selection.
// In order to unset anchor, use Unselect(). It returns true if cursor is in bounds or
// false if selection failed. If cursor has already been called one of the Select methods,
// then this method switches to the new mode and maintains original cursor position.
func (c *Cursor) SelectLine() (ok bool) {
	mode := c.selection.mode
	c.selection.mode = LineSelection
	if mode != noSelection {
		ok = true
		c.setSelection()
		return
	}
	c.selection.scrollFrom, ok = c.cursorAtScrollBounds()
	if !ok {
		return
	}
	c.setSelection()
	return
}

// SelectBlock anchors the current cursor position as the start of a block selection.
// In order to unset anchor, use Unselect(). It returns true if cursor is in bounds or
// false if selection failed. If cursor has already been called one of the Select methods,
// then this method switches to the new mode and maintains original cursor position.
func (c *Cursor) SelectBlock() (ok bool) {
	mode := c.selection.mode
	c.selection.mode = BlockSelection
	if mode != noSelection {
		ok = true
		c.setSelection()
		return
	}
	c.selection.scrollFrom, ok = c.cursorAtScrollBounds()
	if !ok {
		return
	}
	c.setSelection()
	return
}

// Unselect resets the current selection anchor.
func (c *Cursor) Unselect() bool {
	if c.selection.mode == noSelection {
		return false
	}
	c.selection.mode = noSelection
	invertAttr(c.selection.cells)
	c.selection.cells = nil
	return true
}

// Selection returns the current text under either text, line or block selection.
func (c *Cursor) Selection() string {
	return cell.CellsToString(c.selection.cells)
}

// Redo reverses the previously reversed update to the underlying buffer.
func (c *Cursor) Redo() bool {
	ok, at := c.buffer().Redo()
	if !ok {
		return false
	}
	c.setCursor(c.scrollToWindowCoordinates(at))
	return true
}

// Undo reverses the last update to the underlying buffer.
func (c *Cursor) Undo() bool {
	ok, at := c.buffer().Undo()
	if !ok {
		return false
	}
	c.setCursor(c.scrollToWindowCoordinates(at))
	return true
}

// Row returns the row number of the row where the cursor is positioned.
func (c *Cursor) Row() int {
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
	if c.selection.mode == noSelection {
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

	ok = true

	var start term.Coordinates
	switch mode {
	case StandardSelection:
		start, _, _ = c.buffer().Delete(from, to)
	case LineSelection:
		start, _, _ = c.buffer().DeleteLine(from, to)
	case BlockSelection:
		start, _ = c.buffer().DeleteBlock(from, to)
	}
	c.selection.mode = noSelection
	c.setCursor(c.scrollToWindowCoordinates(start))
	return
}

// CopySelection copies the current text under selection and returns true
// or does nothing and returns false.
func (c *Cursor) CopySelection(clip Clipboard) (ok bool) {
	if c.selection.mode == noSelection {
		return
	}

	selection := c.Selection()
	mode := c.selection.mode
	c.Unselect()
	c.setCursor(c.scrollToWindowCoordinates(c.selection.scrollFrom))

	clip.Set(Paste{Data: selection, Metadata: mode})

	ok = true
	return
}

// MoveToBounds moves the cursor up and to the left until it is in a row
// with content and it is 'padding' cells away from the last column in the row.
// If cursor is already in a row and/or in a column with content, then this method
// does nothing.
func (c *Cursor) MoveToBounds(padding int) {
	for c.Row() >= c.buffer().Rows() && c.MoveUp() {
	}

	for c.Column() >= c.buffer().Columns(c.Row())+padding && c.MoveLeft() {
	}
}

// MoveToNextNonNull will move the cursor to the right until it finds
// a cell with a non-null character. If the current cell is already a cell with
// a non-null character, then this method does nothing.
func (c *Cursor) MoveToNextNonNull() {
	for cell, ok := c.Cell(); ; cell, ok = c.Cell() {
		if !ok {
			if !c.MoveLeft() {
				break
			}
			continue
		}
		if cell.Ch == '\x00' {
			if !c.MoveRight() {
				break
			}
			continue
		}
		break
	}
}

func (c *Cursor) moveToChar(
	ch rune, findResult func(int, cell.Searcher) (term.Coordinates, bool),
) bool {
	cursor := c.cursorAtScroll()
	lastPos := c.buffer().Columns(cursor.Y) - 1
	if lastPos < 0 {
		lastPos = 0
	}
	start := term.Coordinates{Y: cursor.Y}
	end := term.Coordinates{Y: cursor.Y, X: lastPos}

	cells := c.buffer().Select(start, end)
	if len(cells) == 0 {
		return false
	}

	reader := cell.NewReader(cells, c.buffer().Tabspaces())

	searcher := cell.NewSimpleSearcher(reader)
	n := searcher.Search(string(ch))
	if n == 0 {
		return false
	}

	result, ok := findResult(n, searcher)
	if !ok { // results not aligned with direction
		return false
	}

	resultAtScroll := term.Coordinates{Y: cursor.Y, X: result.X}

	resultAtWindow := c.scrollToWindowCoordinates(resultAtScroll)
	c.setCursor(resultAtWindow)

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
	n := c.buffer().ShiftRowRight(cursor.Y)
	cursor.X += n
	c.setCursor(c.scrollToWindowCoordinates(cursor))
}

// ShiftLineLeft shifts the current cursor's line one tab to the left. It returns
// false if line's start of content is already at the start of the line.
func (c *Cursor) ShiftLineLeft() bool {
	cursor := c.cursorAtScroll()
	n := c.buffer().ShiftRowLeft(cursor.Y)
	if n == 0 {
		return false
	}
	cursor.X -= n
	if cursor.X < 0 {
		cursor.X = 0
	}
	c.setCursor(c.scrollToWindowCoordinates(cursor))
	return true
}

func (c *Cursor) getShiftSelection() (from, to term.Coordinates) {
	from, to = c.selection.scrollFrom, c.cursorAtScroll()
	switch c.selection.mode {
	case BlockSelection:
		from, to = cell.SortFromToBlock(from, to)
	default:
		from, to = cell.SortFromTo(from, to)
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
		n := c.buffer().ShiftRowLeft(y)
		if n != 0 {
			ok = true
		}
	}
	return
}

func scrollStartList(l LocationList) {
	for ok := true; ok; _, ok = l.Prev() {
	}
}

func (c *Cursor) setLocListAttr(l LocationList, reverse bool) {
	scrollStartList(l)

	loc, ok := l.Current()
	if !ok {
		return
	}

	for {
		selection := c.buffer().Select(loc.From, loc.To)
		if reverse {
			for y, row := range selection {
				for x := range row {
					selection[y][x].Fg &^= loc.Attr.Fg
					selection[y][x].Bg &^= loc.Attr.Bg
				}
			}
		} else {
			for y, row := range selection {
				for x := range row {
					selection[y][x].Fg |= loc.Attr.Fg
					selection[y][x].Bg |= loc.Attr.Bg
				}
			}
		}

		loc, ok = l.Next()
		if !ok {
			break
		}
	}
}

func (c *Cursor) setMessages(ID string, l LocationList) {
	scrollStartList(l)
	for n, ok := l.Current(); ok; n, ok = l.Next() {
		if n.Message == "" {
			continue
		}
		from, to := cell.SortFromTo(n.From, n.To)
		for {
			msgs, ok := c.messages[from]
			if !ok {
				msgs = make([]message, 0, 1)
				c.messages[from] = msgs
			}
			c.messages[from] = append(msgs, message{
				listID:   ID,
				location: n,
			})
			if from.Y == to.Y && from.X == to.X {
				break
			}
			if from.Y == to.Y {
				from.X++
				continue
			}

			from.Y++
			from.X = 0
		}
	}
}

func (c *Cursor) clearMessages(ID string) {
	for from, msgs := range c.messages {
		var stay []message
		for _, msg := range msgs {
			if msg.listID == ID {
				continue
			}
			stay = append(stay, msg)
		}
		c.messages[from] = stay
	}
}

// Locations returns the set of locations by location list ID set by SetLocationList,
// at the current cursor position, if there's any.
func (c *Cursor) Locations() (map[string]Location, bool) {
	msgs, ok := c.messages[c.cursorAtScroll()]
	if !ok {
		return nil, false
	}
	if len(msgs) == 0 {
		return nil, false
	}
	ret := make(map[string]Location, len(msgs))
	for _, msg := range msgs {
		ret[msg.listID] = msg.location
	}
	return ret, true
}

// SetLocationList sets a location list on this cursor. It substitutes and returns
// the previous location list with the same ID, if there was any.
// Any calls to Insert on the underlying Writer will reset all location lists.
func (c *Cursor) SetLocationList(ID string, l LocationList) LocationList {
	prev, ok := c.locs[ID]
	if ok {
		c.setLocListAttr(prev, true)
		c.clearMessages(ID)
	}

	if l == nil {
		delete(c.locs, ID)
	} else {
		c.locs[ID] = l
		c.setLocListAttr(l, false)
		c.setMessages(ID, l)
	}

	return prev
}

func (c *Cursor) moveEndOfLocationList(
	l LocationList, op func(LocationList) (Location, bool),
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
	l LocationList, op, reverse func(LocationList) (Location, bool),
	continueIf func(term.Coordinates, term.Coordinates) bool,
) bool {
	_, gotLocations := c.moveEndOfLocationList(l, reverse)
	if !gotLocations {
		return false
	}

	cursor := c.cursorAtScroll()
	for {
		pos, _ := l.Current()
		if !continueIf(cursor, pos.From) {
			c.moveToScroll(pos.From)
			break
		}

		_, ok := op(l)
		if ok {
			continue
		}

		pos.From, _ = c.moveEndOfLocationList(l, reverse)
		c.moveToScroll(pos.From)
		break
	}

	return cursor != c.cursorAtScroll()
}

// MoveToNextLocation moves the cursor to the next position returned by the location
// list set by SetLocationList. If there isn't a location list set, this method returns
// false.
func (c *Cursor) MoveToNextLocation(ID string) bool {
	l, ok := c.locs[ID]
	if !ok {
		return false
	}

	return c.movePastCursor(l, (LocationList).Next,
		(LocationList).Prev, isBeforeCursor)
}

// MoveToPrevLocation moves the cursor to the previous position returned by the
// location list set by SetLocationList. If there isn't a location list set,
// this method returns false.
func (c *Cursor) MoveToPrevLocation(ID string) bool {
	l, ok := c.locs[ID]
	if !ok {
		return false
	}

	return c.movePastCursor(l, (LocationList).Prev,
		(LocationList).Next, isPastCursor)
}

// Word returns the word under the cursor or empty if it's not a word.
func (c *Cursor) Word() string {
	return c.scroll.WordAt(c.cursorAtScroll())
}
