package text

import (
	"bufio"
	"context"
	"sort"
	"strings"

	"github.com/ernestrc/tcell/v3"
	textapi "unstable.build/go-tui/api/text"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text/clipboard"
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
	c *Cursor
}

type message struct {
	listID   string
	location textapi.Location
}

// Cursor is a helper structure which manages a cursor over a Scroll.
type Cursor struct {
	scroll *component.Scroll
	search string
	cursor term.Coordinates

	locs          map[string]LocationList          // used by cursor moves
	drawLocations map[string]*priorityLocationList // used to draw
	locsSliceTemp []*priorityLocationList
	locsSlice     []textapi.Location
	messages      map[term.Coordinates][]message

	selection struct {
		mode       SelectMode
		scrollFrom term.Coordinates
		cells      [][]term.Cell
	}
	subscriber curSubscriber
}

type priorityLocationList struct {
	locations []textapi.Location
	ID        string
	priority  textapi.LocationPriority
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
// Also, once initialized this cursor MUST NOT be copied.
func (c *Cursor) Init(scroll *component.Scroll) {
	c.InitPerformance(scroll)
	c.scroll.Buffer().Subscribe(&c.subscriber)
}

// InitPerformance initializes this Cursor with a Scroll
// that was initialized with InitPerformance.
func (c *Cursor) InitPerformance(scroll *component.Scroll) {
	c.cursor = term.Coordinates{}
	c.scroll = scroll
	c.selection.mode = NoSelection
	c.subscriber.c = c
	c.locs = make(map[string]LocationList)
	c.drawLocations = make(map[string]*priorityLocationList)
	c.messages = make(map[term.Coordinates][]message)
}

func (c *curSubscriber) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
}

func (c *curSubscriber) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
	c.c.setSearchLocationList(c.c.search)
}

// Coordinates returns the current position of the cursor.
func (c *Cursor) Coordinates() term.Coordinates {
	return c.cursor
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
	// this allows to keep cursor position semantics hidden from clients
	internal term.Coordinates
	offset   term.Coordinates
}

// Before returns true if other is before CursorMark.
func (c CursorMark) Before(other term.Coordinates) bool {
	res := term.CoordinatesDiff(c.internal, other)
	return res.Y < 0 || res.Y == 0 && res.X < 0
}

// Mark returns the current cursor position as a CursorMark
// to later be used in calls to MoveToMark.
func (c *Cursor) Mark() CursorMark {
	return CursorMark{internal: c.cursor, offset: c.scroll.Offset()}
}

// MoveToMark moves the cursor to the position represented by mark.
func (c *Cursor) MoveToMark(mark CursorMark) CursorMark {
	ret := c.cursor
	c.setCursor(mark.internal, false)
	c.scroll.SetOffset(mark.offset)
	return CursorMark{internal: ret}
}

func (c *Cursor) rows() int {
	return c.view().Rows()
}

// CursorAtScroll returns the current position of the cursor relative
// to the scroll coorindates.
func (c *Cursor) CursorAtScroll() term.Coordinates {
	return c.cursorAtScroll()
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

	if pos.Y < 0 {
		pos.Y = 0
	}
	if pos.X < 0 {
		pos.X = 0
	}
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
		windowPos = c.scroll.ScrollToWindowCoordinates(pos)
	}
	c.setCursor(windowPos, true)
}

func (c *Cursor) seekToScrollCoordinates() {
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

func (c *Cursor) setSearchLocationList(text string) int {
	c.search = text
	n := c.scroll.Search(text)

	searchLoc := make([]textapi.Location, n)
	for i := 0; i < n; i++ {
		res, ok := c.scroll.NextResult()
		if !ok {
			panic("invalid scroll search results")
		}
		searchLoc[i] = textapi.Location{
			From: res,
			To:   term.Coordinates{Y: res.Y, X: res.X + len(text) - 1},
			Attr: c.scroll.ResultsAttr,
		}
	}

	c.SetLocationList(internalLocationListPriority, searchLocationListID, LocationSlice(searchLoc))
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
	_, ok = c.MoveToScroll(term.Coordinates{Y: c.cursorAtScroll().Y})
	return
}

// MoveEndLine moves the cursor at the end of the current line, scrolling
// to the end of the line if required.
func (c *Cursor) MoveEndLine() (ok bool) {
	y := c.cursorAtScroll().Y
	x := c.view().Columns(y) - 1
	_, ok = c.MoveToScroll(term.Coordinates{Y: y, X: x})
	return
}

// MoveFirstLine moves the cursor to the first line, scrolling the content
// if appplicable.
func (c *Cursor) MoveFirstLine() (ok bool) {
	_, ok = c.MoveToScroll(term.Coordinates{})
	return
}

// MoveLastLine moves the cursor to the last line, scrolling the content
// if required.
func (c *Cursor) MoveLastLine() (ok bool) {
	height := c.scroll.SizeHeight()
	max := c.rows()
	if height == 0 || max == 0 {
		return
	}
	_, ok = c.MoveToScroll(term.Coordinates{Y: max - 1})
	return ok
}

// MoveDown moves the cursor to the line under the current line, scrolling
// the content if required. It returns false and does nothing when the end
// of the content is reached.
func (c *Cursor) MoveDown() (ok bool) {
	pos := c.cursor
	if pos.Y+1 >= c.scroll.SizeHeight() {
		ok = c.scroll.SeekDown()
		if ok {
			c.setCursor(c.cursor, false)
		}
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: c.cursor.X, Y: c.cursor.Y + 1}, false)
	return
}

// MoveUp moves the cursor the the line above the current line, scrolling
// the content up if required.
func (c *Cursor) MoveUp() (ok bool) {
	cursor := c.cursor
	if cursor.Y < 0 {
		cursor.Y = 0
		c.setCursor(cursor, false)
		ok = true
		return
	}
	if cursor.Y == 0 {
		ok = c.scroll.SeekUp()
		if ok {
			c.setCursor(cursor, false)
		}
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: cursor.X, Y: cursor.Y - 1}, false)
	return
}

// MoveLeft moves the cursor to the cell left of the current cell, scrolling
// the content left if required.
func (c *Cursor) MoveLeft() (ok bool) {
	cursor := c.cursor
	if cursor.X < 0 {
		cursor.X = 0
		ok = true
		c.setCursor(cursor, false)
		return
	}
	if c.scroll.Wrap {
		atScroll := c.cursorAtScroll()
		if atScroll.X == 0 {
			return
		}
		_, ok = c.MoveToScroll(term.Coordinates{X: atScroll.X - 1, Y: atScroll.Y})
		return
	}
	if cursor.X == 0 {
		ok = c.scroll.SeekLeft()
		if ok {
			c.setCursor(cursor, false)
		}
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: cursor.X - 1, Y: cursor.Y}, false)
	return
}

// MoveRight moves the cursor to the cell right of the current cell, scrolling
// the content right if required.
func (c *Cursor) MoveRight() (ok bool) {
	if c.scroll.Wrap {
		atScroll := c.cursorAtScroll()
		if atScroll.Y >= c.rows() ||
			(atScroll.X+1 > c.view().Columns(atScroll.Y) &&
				c.cursor.X+1 >= c.scroll.Width()) {
			return
		}
		_, ok = c.MoveToScroll(term.Coordinates{X: atScroll.X + 1, Y: atScroll.Y})
		return
	}
	if c.cursor.X+1 >= c.scroll.Width() {
		if !c.scroll.Wrap {
			ok = c.scroll.SeekRight()
			if ok {
				c.setCursor(c.cursor, false)
			}
		}
		return
	}
	ok = true
	c.setCursor(term.Coordinates{X: c.cursor.X + 1, Y: c.cursor.Y}, false)
	return
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
	if !c.scroll.Wrap {
		c.MoveEndLine()
		return true
	}
	pos := c.cursorAtScroll()
	i := pos.X / c.scroll.Width()
	after := term.Coordinates{X: (i+1)*c.scroll.Width() - 1, Y: pos.Y}
	_, ok = c.MoveToScroll(after)
	return ok
}

// MoveRightWrap will move the cursor to the right or wrap to beginning
// of next line if cursor is at X=EOL
func (c *Cursor) MoveRightWrap() bool {
	if c.scroll.Width() == 0 && c.scroll.Wrap {
		return false
	}
	ok := c.MoveRight()
	if ok {
		return true
	}
	ok = c.MoveDown()
	if !ok {
		return false
	}
	if !c.scroll.Wrap {
		c.MoveStartLine()
		return true
	}
	pos := c.cursorAtScroll()
	i := pos.X / c.scroll.Width()
	_, ok = c.MoveToScroll(term.Coordinates{X: (i * c.scroll.Width()), Y: pos.Y})
	return ok
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

func (c *Cursor) moveAfterRune(skip, special map[rune]struct{}, move func() bool) (ok bool) {
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

	for move() {
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

func (c *Cursor) moveBeforeRune(skip, all map[rune]struct{}, move func() bool) (ok bool) {
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

	if state == done {
		ok = true
		c.MoveToMark(mark)
	}
	return
}

var allSpecialCharacters = map[rune]struct{}{
	'.': {}, ',': {}, ':': {}, ';': {}, ' ': {}, ')': {}, '"': {},
	'\'': {}, '(': {}, '{': {}, '}': {}, '[': {}, ']': {}, '\t': {},
	'\x00': {}, '\\': {}, '/': {}, '+': {}, '`': {}, '_': {}}

var skipCharacters = map[rune]struct{}{' ': {}, '\t': {}, '\x00': {}, '_': {}}

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
	c.buffer().InsertRowAt(cursorAtScroll.Y)
	c.selection.mode = mode
	c.setSelection()
	c.scroll.RecalculateWraps()
	c.MoveStartLine()
}

// InsertLineBelow inserts a row below the current row and moves the cursor down.
func (c *Cursor) InsertLineBelow() {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	pos := c.cursorAtScroll()
	pos.Y++
	pos.X = 0

	c.buffer().InsertRowAt(pos.Y)
	c.selection.mode = mode
	c.setSelection()
	c.setCursorAfterUpdate(pos)
}

// Insert is equivalent to InsertContext with context.Background.
func (c *Cursor) Insert(r rune) {
	c.InsertContext(context.Background(), r)
}

// InsertContext inserts rune at the current cursor's position.
func (c *Cursor) InsertContext(ctx context.Context, r rune) {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	insertAt := c.cursorAtScroll()
	pos := c.buffer().InsertContext(ctx, insertAt, r)
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
		cur := c.Mark()
		c.InsertString(str)
		if err != nil {
			break
		}
		c.MoveToMark(cur)
		c.MoveLineDown()
	}
}

// Paste pastes the given string on the underlying scroll at the current
// cursor position.
func (c *Cursor) Paste(str string, mode SelectMode, after bool) {
	if mode == NoSelection {
		// this is how vim behaves when using a system clipboard
		if strings.HasSuffix(str, "\n") {
			mode = LineSelection
		} else {
			mode = StandardSelection
		}
	}

	switch mode {
	case StandardSelection:
		if after {
			c.MoveRight()
			cur := c.Mark()
			c.InsertString(str)
			c.MoveToMark(cur)
		} else {
			cur := c.Mark()
			c.InsertString(str)
			c.MoveToMark(cur)
		}
	case LineSelection:
		if after {
			movedDown := c.MoveLineDown()
			var cur CursorMark
			if !movedDown {
				// force set cursor past last line
				cur = c.Mark()
				c.setCursor(term.Coordinates{X: 0, Y: c.cursor.Y + 1}, false)
			} else {
				c.MoveStartLine()
				cur = c.Mark()
			}
			c.InsertString(str)
			c.MoveToMark(cur)
		} else {
			cur := c.Mark()
			c.MoveStartLine()
			c.InsertString(str)
			c.MoveToMark(cur)
			c.MoveStartLine()
		}
	case BlockSelection:
		if after {
			c.MoveRight()
			cur := c.Mark()
			c.InsertBlock(str)
			c.MoveToMark(cur)
		} else {
			cur := c.Mark()
			c.InsertBlock(str)
			c.MoveToMark(cur)
		}
	}
}

// Replace is equivalent to ReplaceContext with context.Background.
func (c *Cursor) Replace(r rune) {
	c.ReplaceContext(context.Background(), r)
}

// ReplaceContext replaces the cell under the cursor with r.
func (c *Cursor) ReplaceContext(ctx context.Context, r rune) {
	mode := c.selection.mode
	c.selection.mode = NoSelection
	c.setSelection()

	from := c.cursorAtScroll()
	to := term.Coordinates{X: from.X + 1, Y: from.Y}
	_, next, _ := c.buffer().Edit(ctx, from, to, string(r))

	c.selection.mode = mode
	c.setSelection()
	c.setCursorAfterUpdate(next)

	return
}

// Delete is equivalent to DeleteContext with context.Background.
func (c *Cursor) Delete() (ok bool) {
	ok = c.DeleteContext(context.Background())
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
	if c.cursor.X > 0 || c.scroll.Offset().X > 0 || c.cursorAtScroll().X > 0 {
		if c.MoveLeft() {
			ok = c.Delete()
		}
		return
	}

	if ok = c.MoveLineUp(); ok {
		c.Conflate()
	}
	return
}

// Conflate is equivalent to calling ConflateContext with context.Background.
func (c *Cursor) Conflate() (ok bool) {
	return c.ConflateContext(context.Background())
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

	ok = true

	length := c.view().Columns(pos.Y)
	if length == 0 {
		c.buffer().DeleteRowContext(ctx, pos.Y)
		return
	}
	c.buffer().ConflateRowContext(ctx, pos.Y)
	c.setCursorAfterUpdate(term.Coordinates{Y: pos.Y, X: length})
	return
}

func (c *Cursor) setSelection() (ok bool) {
	from := c.selection.scrollFrom
	to := c.cursorAtScroll()

	from, to = term.CoordinatesSort(from, to)
	to.X++

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
	max := c.rows()
	if max == 0 {
		pos = term.Coordinates{}
		return
	}

	pos = c.cursorAtScroll()
	ok = true

	if pos.Y >= max {
		pos.Y = max
		pos.X = 0
		return
	}

	cols := c.view().Columns(pos.Y)
	if pos.X > cols {
		pos.X = cols
	}

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
	return cell.CellsToString(c.selection.cells)
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
	to.X++

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
	clip.Copy(registerID, clipboard.Data{Text: selection, Metadata: mode})
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

	for c.Column() >= c.view().Columns(c.Line())+padding && c.MoveLeft() {
	}
}

// MoveToNextNonNull will move the cursor to the right until it finds
// a cell with a non-null character. If the current cell is already a cell with
// a non-null character, then this method does nothing.
func (c *Cursor) MoveToNextNonNull() {
	enable := c.disablePublishing()
	defer enable()

	for cell, ok := c.Cell(); ; cell, ok = c.Cell() {
		if !ok {
			if !c.MoveLeft() {
				break
			}
			// if line ends in null, stop here
			// otherwise this continues until the end of time.
			cell, ok := c.Cell()
			if !ok || cell.Ch != '\x00' {
				continue
			}
			break
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

// MoveToPrevNonNull will move the cursor to the left until it finds
// a cell with a non-null character. If the current cell is already a cell with
// a non-null character, then this method does nothing.
func (c *Cursor) MoveToPrevNonNull() {
	enable := c.disablePublishing()
	defer enable()

	for cell, ok := c.Cell(); ; cell, ok = c.Cell() {
		if !ok {
			if !c.MoveLeft() {
				break
			}
			continue
		}
		if cell.Ch == '\x00' {
			if !c.MoveLeft() {
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
	enable := c.disablePublishing()
	defer enable()

	cursor := c.cursorAtScroll()
	lastPos := c.view().Columns(cursor.Y)
	start := term.Coordinates{Y: cursor.Y}
	end := term.Coordinates{Y: cursor.Y, X: lastPos}

	cells, _, ok := c.buffer().Select(start, end)
	if !ok || len(cells) == 0 {
		return false
	}

	view := cell.NewView(cells, c.buffer().Tabspaces())

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
	n := c.buffer().ShiftRowRight(cursor.Y)
	cursor.X += n
	c.setCursorAfterUpdate(cursor)
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

func (c *Cursor) processList(ID string, l LocationList) {
	scrollStartList(l)
	for n, ok := l.Current(); ok; n, ok = l.Next() {
		c.drawLocations[ID].locations = append(c.drawLocations[ID].locations, n)
		if n.Message == "" {
			continue
		}
		from, to := term.CoordinatesSort(n.From, n.To)
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

// LocationsAtCursor returns the set of locations by location list ID set by SetLocationList,
// at the current cursor position, if there's any.
func (c *Cursor) LocationsAtCursor() (map[string]textapi.Location, bool) {
	msgs, ok := c.messages[c.cursorAtScroll()]
	if !ok {
		return nil, false
	}
	if len(msgs) == 0 {
		return nil, false
	}
	ret := make(map[string]textapi.Location, len(msgs))
	for _, msg := range msgs {
		ret[msg.listID] = msg.location
	}
	return ret, true
}

// SortedLocations returns all the locations sorted by priority level. If two
// location lists have the same priority level, then the location list ID is used
// to disambiguate order.
func (c *Cursor) SortedLocations() []textapi.Location {
	// re-use previous allocation
	c.locsSlice = c.locsSlice[:0]
	c.locsSliceTemp = c.locsSliceTemp[:0]
	for _, list := range c.drawLocations {
		c.locsSliceTemp = append(c.locsSliceTemp, list)
	}

	sort.Slice(c.locsSliceTemp, func(i, j int) bool {
		return c.locsSliceTemp[i].priority <
			c.locsSliceTemp[j].priority ||
			(c.locsSliceTemp[i].priority == c.locsSliceTemp[j].priority &&
				c.locsSliceTemp[i].ID < c.locsSliceTemp[j].ID)
	})

	for _, list := range c.locsSliceTemp {
		for _, loc := range list.locations {
			c.locsSlice = append(c.locsSlice, loc)
		}
	}
	return c.locsSlice
}

// SetLocationList sets a location list on this cursor. It substitutes and returns
// the previous location list with the same ID, if there was any.
// Any calls to Insert on the underlying Writer will reset all location lists.
func (c *Cursor) SetLocationList(
	pri textapi.LocationPriority, ID string, l LocationList,
) LocationList {
	prev, ok := c.locs[ID]
	if ok {
		c.clearMessages(ID)
		c.drawLocations[ID].locations = c.drawLocations[ID].locations[:0]
		c.drawLocations[ID].priority = pri
	} else {
		c.drawLocations[ID] = &priorityLocationList{ID: ID, priority: pri}
	}

	if l == nil {
		delete(c.locs, ID)
	} else {
		c.locs[ID] = l
		c.processList(ID, l)
	}

	return prev
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
	continueIf func(term.Coordinates, term.Coordinates) bool,
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
		if !continueIf(cursor, pos.From) {
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
func (c *Cursor) WindowCoordinates(pos term.Coordinates) term.Coordinates {
	if c.scroll.Width() == 0 && c.scroll.Wrap {
		// best effort conversion, if scroll width is 0 assume no wrap
		return term.CoordinatesDiff(pos, c.scroll.Offset())
	}
	return c.scroll.ScrollToWindowCoordinates(pos)
}

// MoveLineDown is equivalent to MoveDown in non wrap mode. In wrap mode,
// rather than going down one row, the cursor goes down to the line above.
func (c *Cursor) MoveLineDown() bool {
	pos := c.cursorAtScroll()
	pos.Y++
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

func (c *Cursor) moveMatchRuneForward(target, match rune) bool {
	lastRow := c.rows() - 1
	return c.moveMatchRune(target, match, func(pos *term.Coordinates) bool {
		pos.X++
		for pos.Y <= lastRow && pos.X >= c.view().Columns(pos.Y) {
			pos.Y++
			pos.X = 0
		}
		return pos.Y <= lastRow || (pos.Y == lastRow && pos.X < c.view().Columns(pos.Y))
	})
}

func (c *Cursor) moveMatchRune(
	target, match rune,
	advance func(*term.Coordinates) bool,
) bool {
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
		c.MoveToScroll(pos)
		return true
	}
	return false
}

func (c *Cursor) moveMatchRuneBackward(target, match rune) bool {
	return c.moveMatchRune(target, match, func(pos *term.Coordinates) bool {
		pos.X--
		for pos.Y > 0 && pos.X < 0 {
			pos.Y--
			pos.X = c.view().Columns(pos.Y) - 1
		}
		return pos.Y >= 0 && pos.X >= 0
	})
}

func (c *Cursor) setCursorAfterUpdate(atScroll term.Coordinates) {
	if c.scroll.Width() == 0 && c.scroll.Wrap {
		// nothing should be updating if width of the scroll is 0!
		return
	}
	c.scroll.RecalculateWraps()
	var done bool
	// the next position might be beyond the width of the current row
	// in which case ScrollToWidnowCoordiantes would wrap around it
	if atScroll.Y < c.scroll.Buffer().Rows() &&
		atScroll.X > 0 &&
		atScroll.X == c.scroll.Buffer().Columns(atScroll.Y) {
		atScroll.X--
		done = true
	}
	res := c.scroll.ScrollToWindowCoordinates(atScroll)
	if done {
		res.X++
	}
	c.setCursor(res, true)
}
