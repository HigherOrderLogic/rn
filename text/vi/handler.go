package vi

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
)

type viMode uint8
type moveMode uint8

const (
	normalMode viMode = iota
	insertMode
	deleteMode
	gMode
	yankMode
	visualMode
	visualLineMode
	visualBlockMode
	replaceMode
	replaceOneMode
	searchMode
)

const (
	moveNone moveMode = iota
	moveToNext
	moveToPrev
)

var _ viHandler = (*viHandlerImpl)(nil)

type viHandler interface {
	tui.Handler

	mode() viMode
	moveToNextLocation(ID string)
	moveToPrevLocation(ID string)
	setLocationList(ID string, l text.LocationList)
	setCursorAtScroll(pos term.Coordinates) bool
	cursorAtScroll() term.Coordinates
	subscribeScroll(sub component.ScrollSubscriber)
	moveToBounds()
}

// viHandlerImpl implements a basic vi-like text editor which satisfies tui.Handler
// and tui.Component.
type viHandlerImpl struct {
	config       viConfig
	less         handler.Less // used for message bar and text search capabilities
	free         text.CursorMark
	cursor       text.Cursor
	repeater     text.Repeater // used for block repeat only
	currMode     viMode
	moveMode     moveMode
	searchMode   moveMode
	moveChar     rune
	deleteInsert bool
	blockRepeat  struct {
		From term.Coordinates
		To   term.Coordinates
	}
}

// DefaultviHandlerImplConfig is a sane configuration defaults for viHandlerImpl.
var defaultviHandlerImplConfig = viConfig{
	resAttr: term.Attributes{
		Fg: term.AttrReverse,
		Bg: term.ColorDefault,
	},
	clipboard:       text.NewInMemoryClipboard(),
	defaultRegister: text.DefaultRegisterID,
}

func (vi *viHandlerImpl) init(buf *cell.Buffer, opts ...Option) {
	vi.config = defaultviHandlerImplConfig
	for _, o := range opts {
		o(&vi.config)
	}

	vi.less.InitWithBuffer(buf, handler.LessConfig{
		Wrap:    vi.config.wrap,
		Debug:   vi.config.debug,
		ResAttr: vi.config.resAttr,
	})
	vi.cursor.Init(vi.less.Scroll())
	vi.repeater.Init(&vi.cursor, buf)

	vi.free = vi.cursor.Mark()

	vi.setNormalMode()
}

// Resize : tui.Component
func (vi *viHandlerImpl) Resize(width, height int) {
	vi.less.Resize(width, height)
}

func (vi *viHandlerImpl) setActiveLocationListMessage(locs map[string]text.Location) {
	// NOTE: if therea re multiple location lists with a message
	// in current cursor position, then there's no guarantee of which one
	// is going to be rendered.
	for _, loc := range locs {
		vi.less.SetMessage(loc.Message)
		return
	}
}

// Draw : tui.Component
func (vi *viHandlerImpl) Draw(w term.Writer) {
	locs, ok := vi.cursor.Locations()
	if ok {
		vi.setActiveLocationListMessage(locs)
	} else {
		vi.setMode(vi.currMode)
	}
	vi.less.Draw(w)
}

// Man : tui.Handler
func (vi *viHandlerImpl) Man() tui.Manual {
	panic("TODO")
}

// Cursor : tui.Handler
func (vi *viHandlerImpl) Cursor() (term.Coordinates, bool) {
	// use less Cursor if we are in search mode
	if vi.mode() == searchMode {
		return vi.less.Cursor()
	}
	return vi.cursor.Coordinates(), true
}

func (vi *viHandlerImpl) setMode(mode viMode) {
	var text string
	switch mode {
	case normalMode:
		text = "NORMAL"
	case insertMode:
		text = "INSERT"
	case deleteMode:
		text = "DELETE"
	case gMode:
		text = "NORMAL"
	case yankMode:
		text = "YANK"
	case visualMode:
		text = "VISUAL"
	case visualLineMode:
		text = "V-LINE"
	case visualBlockMode:
		text = "V-BLOCK"
	case replaceMode:
		text = "REPLACE"
	case searchMode:
		text = "SEARCH"
	case replaceOneMode:
		text = "NORMAL"
	default:
		panic(fmt.Sprintf("unknown mode: %v", mode))
	}
	vi.less.SetMessageAlt(":")
	vi.less.SetMessage(text)
	vi.currMode = mode
}

func (vi *viHandlerImpl) setNormalMode() bool {
	if vi.currMode == normalMode && vi.moveMode == moveNone {
		return false
	}
	vi.setMode(normalMode)
	vi.moveMode = moveNone
	return true
}

func (vi *viHandlerImpl) setInsertMode() {
	vi.blockRepeat.From = term.Coordinates{}
	vi.blockRepeat.To = term.Coordinates{}
	vi.setMode(insertMode)
}

func (vi *viHandlerImpl) setDeleteMode(thenInsert bool) {
	vi.setMode(deleteMode)
	vi.deleteInsert = thenInsert
}

func (vi *viHandlerImpl) setGMode() {
	vi.setMode(gMode)
}

func (vi *viHandlerImpl) setYankMode() {
	vi.setMode(yankMode)
}

func (vi *viHandlerImpl) setVisualMode() {
	if vi.cursor.Select() {
		vi.setMode(visualMode)
	}
}

func (vi *viHandlerImpl) setVisualLineMode() {
	if vi.cursor.SelectLine() {
		vi.setMode(visualLineMode)
	}
}

func (vi *viHandlerImpl) setVisualBlockMode() {
	if vi.cursor.SelectBlock() {
		vi.setMode(visualBlockMode)
	}
}

func (vi *viHandlerImpl) setMoveToCharacterMode(mode moveMode) {
	vi.moveMode = mode
}

func (vi *viHandlerImpl) setReplaceMode() {
	vi.setMode(replaceMode)
}

func (vi *viHandlerImpl) setReplaceOneMode() {
	vi.setMode(replaceOneMode)
}

// we delegate search buffer Component to Less but delegate cursor position
// and results seeking to Editor so this function makes sure that we only
// perform the search once, at the same time we delegate the right logic to
// Editor and Less.
func (vi *viHandlerImpl) handleSearch(ev term.Event) (bool, bool) {
	switch ev.Key {
	case term.KeyEnter:
		text := vi.less.SearchText()
		vi.less.SetNormalMode()
		vi.searchMode = moveToNext
		vi.less.SetMessage("searching '%s'", text)
		vi.search(text)
		return false, true
	default:
		return vi.less.Handle(ev)
	}
}

func (vi *viHandlerImpl) insertBlock(str string) {
	reader := bufio.NewReader(strings.NewReader(str))
	for {
		str, err := reader.ReadString('\n')
		if err == nil && len(str) > 0 {
			str = str[:len(str)-1]
		}
		cur := vi.cursor.Mark()
		vi.cursor.InsertString(str)
		if err != nil {
			break
		}
		vi.cursor.MoveToMark(cur)
		vi.cursor.MoveDown()
	}
}

func (vi *viHandlerImpl) logError(err error) {
	if vi.config.logger == nil {
		return
	}
	vi.config.logger.Error(err)
}

func (vi *viHandlerImpl) pasteClipboard(registerID string, after bool) bool {
	paste, err := vi.config.clipboard.Paste(registerID)
	if err != nil {
		vi.logError(fmt.Errorf("clipboard.Get: %s", err))
		return false
	}

	str := paste.Text
	mode, ok := paste.Metadata.(text.SelectMode)
	if !ok {
		mode = text.StandardSelection
	}

	cur := vi.cursor.Mark()

	switch mode {
	case text.StandardSelection:
		if after {
			vi.cursor.MoveRight()
			vi.cursor.InsertString(str)
		} else {
			vi.cursor.InsertString(str)
			vi.cursor.MoveToMark(cur)
		}
	case text.LineSelection:
		if after {
			vi.cursor.MoveStartLine()
			if !vi.cursor.MoveDown() {
				vi.cursor.InsertString(fmt.Sprintf("%s\n", str))
			} else {
				vi.cursor.InsertString(str)
			}
			vi.cursor.MoveToMark(cur)
			vi.cursor.MoveDown()
			vi.cursor.MoveStartLine()
		} else {
			vi.cursor.MoveStartLine()
			vi.cursor.InsertString(str)
			vi.cursor.MoveToMark(cur)
			vi.cursor.MoveStartLine()
		}
	case text.BlockSelection:
		if after {
			vi.cursor.MoveRight()
			vi.insertBlock(str)
			vi.cursor.MoveToMark(cur)
			vi.cursor.MoveRight()
		} else {
			vi.insertBlock(str)
			vi.cursor.MoveToMark(cur)
		}
	}
	return true
}

func (vi *viHandlerImpl) handleNormal(ev term.Event) (quit, handled bool) {
	quit, handled = vi.handleMoveToCharacter(vi.moveMode, ev)
	if handled {
		return
	}

	switch ev.Type {
	case term.EventKey:
		handled = true
		switch ev.Ch {
		case 'R':
			vi.setReplaceMode()
		case 'r':
			vi.setReplaceOneMode()
		case '>':
			vi.cursor.ShiftLineRight()
		case '<':
			vi.cursor.ShiftLineLeft()
		case ',':
			event := term.Event{Type: term.EventKey, Ch: vi.moveChar}
			vi.handleMoveToCharacter(moveToPrev, event)
		case ';':
			event := term.Event{Type: term.EventKey, Ch: vi.moveChar}
			vi.handleMoveToCharacter(moveToNext, event)
		case 'f':
			vi.setMoveToCharacterMode(moveToNext)
		case 'F':
			vi.setMoveToCharacterMode(moveToPrev)
		case 'g':
			vi.setGMode()
		case 'd':
			vi.setDeleteMode(false)
		case 'c':
			vi.setDeleteMode(true)
		case 'y':
			vi.setYankMode()
		case 'N':
			switch vi.searchMode {
			case moveToNext:
				vi.cursor.MoveToPrevMatch()
			case moveToPrev:
				vi.cursor.MoveToNextMatch()
			}
		case 'n':
			switch vi.searchMode {
			case moveToNext:
				vi.cursor.MoveToNextMatch()
			case moveToPrev:
				vi.cursor.MoveToPrevMatch()
			}
		case 'p':
			vi.pasteClipboard(vi.config.defaultRegister, true)
		case 'P':
			vi.pasteClipboard(vi.config.defaultRegister, false)
		case '0':
			vi.cursor.MoveStartLine()
		case '$':
			vi.cursor.MoveEndLine()
		case 'G':
			vi.cursor.MoveLastLine()
		case 'j':
			vi.cursor.MoveToMark(vi.free)
			vi.cursor.MoveDown()
		case 'k':
			vi.cursor.MoveToMark(vi.free)
			vi.cursor.MoveUp()
		case 'h':
			vi.cursor.MoveLeft()
		case 'l':
			vi.cursor.MoveRight()
		case 'O':
			vi.setInsertMode()
			vi.cursor.InsertRowAbove()
		case 'o':
			vi.setInsertMode()
			vi.cursor.InsertRowBelow()
		case 'i':
			vi.setInsertMode()
		case 'I':
			vi.cursor.MoveStartLine()
			vi.setInsertMode()
		case 'J':
			vi.cursor.Conflate()
		case 'a':
			vi.cursor.MoveRight()
			vi.setInsertMode()
		case 'A':
			vi.setInsertMode()
			vi.cursor.MoveEndLine()
			vi.cursor.MoveRight()
		case 'C':
			if vi.cursor.Select() {
				vi.cursor.MoveEndLine()
				vi.cursor.DeleteSelection()
			}
			vi.setInsertMode()
		case 'D':
			if vi.cursor.Select() {
				vi.cursor.MoveEndLine()
				vi.cursor.DeleteSelection()
			}
		case 'x':
			vi.cursor.Delete()
		case 's':
			vi.cursor.Delete()
			vi.setInsertMode()
		case 'v':
			vi.setVisualMode()
		case 'V':
			vi.setVisualLineMode()
		case 'w':
			vi.cursor.MoveRightStartWord()
		case 'W':
			vi.cursor.MoveRightStartWordGroup()
		case 'e':
			vi.cursor.MoveRightEndWord()
		case 'E':
			vi.cursor.MoveRightEndWordGroup()
		case 'b':
			vi.cursor.MoveLeftStartWord()
		case 'B':
			vi.cursor.MoveLeftStartWordGroup()
		case '?':
			vi.searchMode = moveToPrev
			ev.Ch = '/'
			vi.less.Handle(ev)
		case '/':
			vi.searchMode = moveToNext
			vi.less.Handle(ev)
		case '%':
			handled = vi.cursor.MoveToMatchingRune()
		case '#':
			vi.searchMode = moveToPrev
			vi.search(vi.cursor.Word())
		case '*':
			vi.searchMode = moveToNext
			vi.search(vi.cursor.Word())
		default:
			switch ev.Key {
			case term.KeyCtrlV:
				vi.setVisualBlockMode()
			case term.KeyEsc:
				handled = vi.setNormalMode()
				vi.cursor.Unselect()
			default:
				handled = false
			}
		}
	}

	return
}

func (vi *viHandlerImpl) search(text string) {
	vi.cursor.Search(text)
	switch vi.searchMode {
	case moveToNext:
		vi.cursor.MoveToNextMatch()
	case moveToPrev:
		vi.cursor.MoveToPrevMatch()
	default:
	}
}

func (vi *viHandlerImpl) handleInsert(ev term.Event) (quit, handled bool) {
	handled = true
	switch ev.Key {
	case term.KeyEnter:
		vi.cursor.Insert('\n')
	case term.KeySpace:
		vi.cursor.Insert(' ')
	case term.KeyTab:
		vi.cursor.Insert('\t')
	case term.KeyBackspace, term.KeyBackspace2:
		vi.cursor.Backspace()
	case term.KeyEsc:
		vi.cursor.MoveLeft()
		vi.repeatInsertStart()
		vi.setNormalMode()
	default:
		if ev.Ch != 0 {
			vi.cursor.Insert(ev.Ch)
		} else {
			handled = false
		}
	}
	return
}

func (vi *viHandlerImpl) copySelection() {
	vi.cursor.CopySelection(vi.config.defaultRegister, vi.config.clipboard)
}

func (vi *viHandlerImpl) repeatInsertStart() {
	from, to := cell.SortFromTo(vi.blockRepeat.From, vi.blockRepeat.To)
	n := to.Y - from.Y
	for i := 0; i < n; i++ {
		vi.blockRepeat.From.Y++
		vi.cursor.MoveToScroll(vi.blockRepeat.From)
		vi.repeater.Repeat()
	}
}

func (vi *viHandlerImpl) handleVisualBlockInsertStart() {
	vi.setInsertMode()
	vi.blockRepeat.From, _ = vi.cursor.SelectionFrom()
	vi.blockRepeat.To = vi.cursor.CursorAtScroll()
	vi.setCursorAtScroll(vi.blockRepeat.From)
}

func (vi *viHandlerImpl) handleVisual(ev term.Event) (quit, handled bool) {
	if ev.Key == term.KeyEsc {
		vi.setNormalMode()
		vi.cursor.Unselect()
		handled = true
		return
	}

	if ev.Type == term.EventKey {
		handled = true
		switch ev.Ch {
		case '>':
			vi.cursor.ShiftSelectionRight()
		case '<':
			vi.cursor.ShiftSelectionLeft()
		case 'y':
			vi.copySelection()
			vi.setNormalMode()
		case 'd', 'x':
			vi.cursor.DeleteSelection()
			vi.setNormalMode()
		case 's', 'c':
			vi.cursor.DeleteSelection()
			vi.setInsertMode()
		case 'I':
			switch vi.currMode {
			case visualBlockMode:
				vi.handleVisualBlockInsertStart()
			default:
				handled = false
			}
		default:
			handled = false
		}
	}

	if !handled {
		quit, handled = vi.handleNormal(ev)
	}

	switch vi.currMode {
	case visualMode, visualLineMode, visualBlockMode:
	default:
		vi.cursor.Unselect()
	}

	return
}

func (vi *viHandlerImpl) handleMoveToCharacter(mode moveMode, ev term.Event) (bool, bool) {
	switch ev.Type {
	case term.EventKey:
		switch mode {
		case moveToNext:
			vi.cursor.MoveToNextChar(ev.Ch)
		case moveToPrev:
			vi.cursor.MoveToPrevChar(ev.Ch)
		case moveNone:
			return false, false
		}
		vi.moveChar = ev.Ch
		vi.setNormalMode()
	default:
		vi.setNormalMode()
	}
	return false, true
}

func (vi *viHandlerImpl) handleReplace(ev term.Event) (quit, handled bool) {
	if ev.Type != term.EventKey {
		return
	}

	handled = true

	switch ev.Key {
	case term.KeyEnter:
		ev.Ch = '\n'
	case term.KeySpace:
		ev.Ch = ' '
	case term.KeyTab:
		ev.Ch = '\t'
	case term.KeyBackspace, term.KeyBackspace2:
		vi.cursor.MoveLeft()
	case term.KeyEsc:
		vi.cursor.MoveLeft()
		vi.setNormalMode()
	}

	if ev.Ch != 0 {
		// do not delete column == len(row); it contains a newline
		// and that would conflate the current row with the next
		if vi.cursor.Column() < vi.less.Buffer().Columns(vi.cursor.Row()) {
			vi.cursor.Delete()
		}
		vi.cursor.Insert(ev.Ch)
	}
	return
}

func (vi *viHandlerImpl) handleMetaNormal(ev term.Event) (quit, handled, done bool) {
	before := vi.cursor.Coordinates()
	vi.cursor.Select()

	prevMode := vi.moveMode
	quit, handled = vi.handleNormal(ev)
	isMoveSwitch := prevMode == moveNone && vi.moveMode != moveNone
	after := vi.cursor.Coordinates()

	if before == after {
		vi.cursor.Unselect()
		if !isMoveSwitch {
			handled = false
			vi.setNormalMode()
		}
		return
	}

	done = true

	// handle <op>wWeEbB idiosyncrasies
	after = vi.cursor.Coordinates()
	switch ev.Ch {
	case 'e', 'E':
	case 'w', 'W':
		vi.cursor.MoveLeft()
		if before.Y < after.Y {
			vi.cursor.MoveLeftEndWord()
		}
	case 'b', 'B':
		if before.Y > after.Y {
			vi.cursor.MoveLeftStartWord()
		}
	default:
		if before.Y != after.Y {
			vi.cursor.SelectLine()
		}
	}
	return
}

func (vi *viHandlerImpl) handleYank(ev term.Event) (quit, handled bool) {
	if ev.Ch == 'y' {
		if vi.cursor.SelectLine() {
			vi.copySelection()
			handled = true
		}
		vi.setNormalMode()
		return
	}

	var done bool
	quit, handled, done = vi.handleMetaNormal(ev)
	if !done {
		return
	}

	vi.copySelection()
	vi.setNormalMode()
	return
}

func (vi *viHandlerImpl) handleDelete(ev term.Event) (quit, handled bool) {
	if !vi.deleteInsert && vi.moveMode == moveNone && ev.Ch == 'd' {
		if vi.cursor.SelectLine() {
			vi.cursor.DeleteSelection()
		}
		vi.setNormalMode()
		handled = true
		return
	}

	if vi.deleteInsert && vi.moveMode == moveNone && ev.Ch == 'c' {
		vi.cursor.MoveStartLine()
		if vi.cursor.Select() {
			vi.cursor.MoveEndLine()
			vi.cursor.DeleteSelection()
		}
		vi.setInsertMode()
		handled = true
		return
	}

	var done bool
	quit, handled, done = vi.handleMetaNormal(ev)
	if !done {
		return
	}

	vi.cursor.DeleteSelection()
	if vi.deleteInsert {
		vi.setInsertMode()
	} else {
		vi.setNormalMode()
	}
	return
}

func (vi *viHandlerImpl) handleGo(ev term.Event) (quit, handled bool) {
	defer vi.setNormalMode()

	switch ev.Ch {
	case 'g':
		vi.cursor.MoveFirstLine()
		handled = true
	default:
	}
	return
}

// Handle : tui.Handler
func (vi *viHandlerImpl) Handle(ev term.Event) (quit, handled bool) {
	switch vi.mode() {
	case searchMode:
		quit, handled = vi.handleSearch(ev)
	case normalMode:
		quit, handled = vi.handleNormal(ev)
	case insertMode:
		quit, handled = vi.handleInsert(ev)
	case gMode:
		quit, handled = vi.handleGo(ev)
	case yankMode:
		quit, handled = vi.handleYank(ev)
	case deleteMode:
		quit, handled = vi.handleDelete(ev)
	case visualMode, visualLineMode, visualBlockMode:
		quit, handled = vi.handleVisual(ev)
	case replaceMode:
		quit, handled = vi.handleReplace(ev)
	case replaceOneMode:
		quit, _ = vi.handleReplace(ev)
		handled = true
		vi.setNormalMode()
	default:
		panic(fmt.Sprintf("unknown mode: %d", vi.currMode))
	}

	vi.moveToBounds()
	return
}

func (vi *viHandlerImpl) moveToBounds() {
	// copy the cursor to maintain original cursor for next vertcial move
	// except when moving the cursor beyond last line
	if vi.cursor.CursorAtScroll().Y < vi.less.Buffer().Rows() {
		vi.free = vi.cursor.Mark()
	}

	switch vi.mode() {
	case normalMode, yankMode, searchMode, gMode, deleteMode,
		visualMode, visualLineMode, visualBlockMode:
		if !vi.config.debug {
			vi.cursor.MoveToBounds(0)
			vi.cursor.MoveToNextNonNull()
		}
	case insertMode, replaceMode, replaceOneMode:
		if !vi.config.debug {
			vi.cursor.MoveToBounds(1)
		}
	default:
		panic(fmt.Sprintf("unknown mode: %d", vi.currMode))
	}
}

// MoveToNextLocation moves the cursor to the next location
// in the location list identified by ID.
func (vi *viHandlerImpl) moveToNextLocation(ID string) {
	vi.cursor.MoveToNextLocation(ID)
	vi.free = vi.cursor.Mark()
}

// MoveToPrevLocation moves the cursor to the previous location
// in the location list identified by ID.
func (vi *viHandlerImpl) moveToPrevLocation(ID string) {
	vi.cursor.MoveToPrevLocation(ID)
	vi.free = vi.cursor.Mark()
}

// SetLocationList sets a location list of this handler. See Cursor.SetLocationList
func (vi *viHandlerImpl) setLocationList(ID string, l text.LocationList) {
	_ = vi.cursor.SetLocationList(ID, l)
}

// SetCursorAtScroll sets the cursor of this viHandlerImpl handler at content pos.
func (vi *viHandlerImpl) setCursorAtScroll(pos term.Coordinates) bool {
	_, ok := vi.cursor.MoveToScroll(pos)
	vi.free = vi.cursor.Mark()
	return ok
}

// CursorAtScroll sets the cursor of this viHandlerImpl handler at content pos.
func (vi *viHandlerImpl) cursorAtScroll() term.Coordinates {
	return vi.cursor.CursorAtScroll()
}

func (vi *viHandlerImpl) subscribeScroll(sub component.ScrollSubscriber) {
	vi.cursor.SubscribeScroll(sub)
}

func (vi *viHandlerImpl) mode() viMode {
	if vi.currMode == normalMode && vi.less.Mode() != handler.LessNormalMode {
		return searchMode
	}
	return vi.currMode
}
