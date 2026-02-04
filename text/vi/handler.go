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

package vi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

type viMode uint8
type moveMode uint8

const (
	normalMode viMode = iota
	insertMode
	deleteMode
	gMode
	zMode
	yankMode
	visualMode
	visualLineMode
	visualBlockMode
	replaceMode
	replaceOneMode
	searchMode // this is never set in currMode, but returned by mode()
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
	moveToNextLocation(ID string) bool
	moveToPrevLocation(ID string) bool
	setLocationList(pri textapi.LocationPriority, ID string, l text.LocationList)
	setCursorAtScroll(pos term.Coordinates) bool
	setNormalMode() bool
	cursorAtScroll() term.Coordinates
	search(string)
	moveToBounds()
	unselect() bool
	setStatusBar(bar statusBar)
}

// viHandlerImpl implements a basic vi-like text editor which satisfies tui.Handler
// and tui.Component.
type viHandlerImpl struct {
	config       viConfig
	less         handler.Less // used for search capabilities
	statusBar    statusBar
	anchor       term.Coordinates
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
	pendingSetCursor *term.Coordinates
	setLocations     bool
	countDigits      string
	count            int
}

type statusBar interface {
	SetStatus(string, term.Attributes)
}

func (vi *viHandlerImpl) init(buf *cell.Buffer, cfg viConfig) {
	vi.config = cfg
	vi.statusBar = nopBar{}
	vi.less.InitWithBuffer(buf, handler.LessConfig{
		Wrap:               vi.config.wrap,
		ResAttr:            vi.config.resAttr,
		SuperimposeMessage: true,
		Attributes:         vi.config.attr,
	})
	vi.less.Scroll().SetTabspaces(vi.config.tabspaces)
	scroll := vi.less.Scroll()
	scroll.Attributes = vi.config.attr
	scroll.ResultsAttr = vi.config.resAttr
	scroll.Subscribe(vi)
	vi.cursor.Init(vi.less.Scroll(), vi.config.scheduleNextTick)
	vi.cursor.RightInclusiveSemantics = true
	vi.repeater.Init(&vi.cursor, buf)

	vi.anchor = vi.cursorAtScroll()

	vi.setMode(normalMode)
	vi.resetCount()
}

func (vi *viHandlerImpl) initWithScroll(scroll *component.Scroll, opts ...Option) {
	vi.config = defaultviHandlerImplConfig()
	for _, o := range opts {
		o(&vi.config)
	}

	vi.statusBar = nopBar{}
	vi.less.InitWithScroll(scroll, handler.LessConfig{
		Wrap:               vi.config.wrap,
		ResAttr:            vi.config.resAttr,
		SuperimposeMessage: true,
		Attributes:         vi.config.attr,
	})
	vi.less.Scroll().SetTabspaces(vi.config.tabspaces)
	// do not initialize repeater, as we don't know if scroll
	// was initialized with subscription functionality.
	// vi.repeater.Init(&vi.cursor, scroll.Buffer())
	vi.cursor.InitPerformance(vi.less.Scroll())
	vi.cursor.RightInclusiveSemantics = true
	vi.anchor = vi.cursorAtScroll()
	vi.setMode(normalMode)
	vi.resetCount()
}

func (vi *viHandlerImpl) setStatusBar(msg statusBar) {
	vi.statusBar = msg
	vi.setMode(vi.mode())
}

// Resize satisfies tui.Component
func (vi *viHandlerImpl) Resize(width, height int) {
	vi.less.Resize(width, height)
	if vi.pendingSetCursor != nil {
		pos := *vi.pendingSetCursor
		vi.setCursorAtScroll(pos)
		vi.pendingSetCursor = nil
	}
}

func (vi *viHandlerImpl) setActiveLocationListMessage(locs []textapi.Location) {
	// NOTE: if therea re multiple location lists with a message
	// in current cursor position, then there's no guarantee of which one
	// is going to be rendered.
	for _, loc := range locs {
		if loc.Message != "" {
			vi.less.SetMessage("%s", loc.Message)
			return
		}
	}
	vi.less.SetMessage("")
}

func (vi *viHandlerImpl) drawLocationMessage() {
	locs, ok := vi.cursor.LocationsAtCursor()
	if ok {
		vi.setActiveLocationListMessage(locs)
		vi.setLocations = true
	} else if vi.setLocations {
		vi.less.SetMessage("")
		vi.setLocations = false
	}
}

// Draw satisfies tui.Component
func (vi *viHandlerImpl) Draw(w term.Writer) {
	vi.drawLocationMessage()
	vi.less.Draw(w)
	locations := vi.cursor.SortedLocations()
	text.DrawLocations(locations, vi.less.Scroll(), w)
}

// Cursor satisfies tui.Handler
func (vi *viHandlerImpl) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	var style term.CursorStyle
	switch vi.mode() {
	case searchMode:
		return vi.less.Cursor()
	case insertMode:
		style = term.CursorStyleSteadyBar
	case zMode, gMode, yankMode, deleteMode, replaceMode, replaceOneMode:
		style = term.CursorStyleSteadyUnderline
	case normalMode, visualMode, visualLineMode, visualBlockMode:
		style = term.CursorStyleDefault
	default:
		panic(fmt.Sprintf("unknown mode: %d", vi.currMode))
	}
	return vi.cursor.Coordinates(), style, true
}

func (vi *viHandlerImpl) setMode(mode viMode) {
	var text string
	var attrs term.Attributes
	switch mode {
	case normalMode, gMode, replaceOneMode, zMode:
		text = " NORMAL"
	case insertMode:
		text = " INSERT"
		attrs = term.Attributes{Bg: tcell.ColorGreen, Fg: tcell.ColorBlack}
	case deleteMode:
		text = " DELETE"
		attrs = term.Attributes{Bg: tcell.ColorRed, Fg: tcell.ColorBlack}
	case yankMode:
		text = "  YANK "
		attrs = term.Attributes{Bg: tcell.ColorFuchsia, Fg: tcell.ColorBlack}
	case visualMode:
		text = " VISUAL"
		attrs = term.Attributes{Bg: tcell.ColorBlue, Fg: tcell.ColorBlack}
	case visualLineMode:
		text = " V-LINE"
		attrs = term.Attributes{Bg: tcell.ColorTeal, Fg: tcell.ColorBlack}
	case visualBlockMode:
		text = "V-BLOCK"
		attrs = term.Attributes{Bg: tcell.ColorAqua, Fg: tcell.ColorBlack}
	case replaceMode:
		text = "REPLACE"
		attrs = term.Attributes{Bg: tcell.ColorYellow, Fg: tcell.ColorBlack}
	case searchMode:
		text = " SEARCH"
		attrs = term.Attributes{Bg: tcell.ColorSilver, Fg: tcell.ColorBlack}
	default:
		panic(fmt.Sprintf("unknown mode: %v", mode))
	}
	vi.statusBar.SetStatus(text, attrs)
	vi.currMode = mode
}

func (vi *viHandlerImpl) setNormalMode() bool {
	if vi.less.Mode() == handler.LessSearchMode {
		vi.less.SetNormalMode()
	}

	vi.resetCount()
	vi.setMode(normalMode)
	vi.moveMode = moveNone
	return true
}

func (vi *viHandlerImpl) setInsertMode() {
	vi.repeater.Clear()
	vi.blockRepeat.From = term.Coordinates{}
	vi.blockRepeat.To = term.Coordinates{}
	vi.setMode(insertMode)
	vi.less.SetMessage("")
	vi.resetCount()
}

func (vi *viHandlerImpl) setDeleteMode(thenInsert bool) {
	vi.setMode(deleteMode)
	vi.deleteInsert = thenInsert
	vi.less.SetMessage("")
}

func (vi *viHandlerImpl) setGMode() {
	vi.setMode(gMode)
}

const foldHighlightLocationListID = "_foldHighlightID"

func (vi *viHandlerImpl) setZMode() {
	vi.setMode(zMode)
	vi.cursor.FoldAt(context.Background(), func(fold term.Range, ok bool) {
		foldHighlightAttr := term.Attributes{Bg: tcell.ColorGray}
		if !ok {
			return
		}
		vi.cursor.SetLocationList(
			textapi.LocationPriorityInfo, foldHighlightLocationListID,
			text.LocationSlice([]textapi.Location{
				{From: fold.Start, To: fold.End, Attr: foldHighlightAttr},
			}))
	})
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

func (vi *viHandlerImpl) handleSearch(ev term.Event) (bool, bool) {
	switch ev.Mod {
	case 0:
		switch ev.Key {
		case term.KeyEnter:
			text := vi.less.SearchText()
			vi.less.SetNormalMode()
			if text == "" {
				vi.less.SetMessage("")
			} else {
				vi.less.SetMessage("searching '%s'", text)
			}
			vi.search(text)
			return false, true
		}
	}
	return vi.less.Handle(ev)
}

func (vi *viHandlerImpl) logError(err error) {
	log.WithField(logging.KeyClass, "vi.handler").Error(err)
}

func (vi *viHandlerImpl) pasteClipboard(registerID string, after bool) bool {
	paste, err := vi.config.clipboard.Paste(registerID)
	if err != nil {
		vi.logError(fmt.Errorf("clipboard.Get: %s", err))
		return false
	}
	str := paste.Text

	// if not ok, zero value of mode is accepted
	// and interpreted by cursor.
	mode, _ := paste.Metadata.(text.SelectMode)

	// Pasting on visual selection will first delete it before inserting the pasted
	// text. cursor.DeleteSelection sets the cursor mode to NoSelection so the state
	// that called replacing from (text.LineSelection or others) is lost.
	//
	// This does not happen if no deletion happens between the yanking and the pasting,
	// in other words: if the user is not replacing but simply yank-pasting.
	if vi.currMode == visualLineMode {
		mode = text.LineSelection
	}

	vi.cursor.Paste(str, mode, after)
	return true
}

const matchingLocID = "_matchingMark"

func (vi *viHandlerImpl) markMatchingBrace() {
	vi.cursor.SetLocationList(textapi.LocationPriorityInfo, matchingLocID, nil)
	c, ok := vi.cursor.Cell()
	if !ok {
		return
	}
	var matching rune
	var end bool
	switch c.Ch {
	case '{':
		matching = '}'
	case '(':
		matching = ')'
	case '[':
		matching = ']'
	case '<':
		matching = '>'
	case '}':
		matching = '{'
		end = true
	case ')':
		matching = '('
		end = true
	case ']':
		matching = '['
		end = true
	case '>':
		matching = '<'
		end = true
	default:
		return
	}

	vi.markMatchingBraceViaCursor(c.Ch, matching, end)
}

func (vi *viHandlerImpl) markMatchingBraceViaCursor(target, match rune, end bool) {
	var pos term.Coordinates
	var ok bool
	if end {
		pos, ok = vi.cursor.FindMatchingRuneBackward(target, match)
	} else {
		pos, ok = vi.cursor.FindMatchingRuneForward(target, match)
	}
	if !ok {
		return
	}
	selectEnd := pos
	selectEnd.X++
	vi.cursor.SetLocationList(textapi.LocationPriorityInfo,
		matchingLocID, textapi.LocationSlice([]textapi.Location{
			{From: pos, To: selectEnd, Attr: term.Attributes{
				Attrs: tcell.AttrReverse,
			}},
		}))
}

func (vi *viHandlerImpl) handleNormal(ev term.Event) (quit, handled bool) {
	doResetCount := true
	defer func() {
		if doResetCount {
			vi.resetCount()
		}
	}()

	quit, handled = vi.handleMoveToCharacter(vi.moveMode, ev)
	if handled {
		return
	}

	switch ev.Mod {
	case term.ModCtrl:
		switch ev.Ch {
		case 'e':
			pos := vi.cursor.CursorAtScroll()
			if handled = vi.less.Scroll().SeekDown(); handled {
				win, _ := vi.cursor.WindowCoordinates(pos)
				if win.Y >= 0 {
					vi.cursor.SetCursorAtScroll(pos)
				}
			}
		case 'y':
			pos := vi.cursor.CursorAtScroll()
			if handled = vi.less.Scroll().SeekUp(); handled {
				win, _ := vi.cursor.WindowCoordinates(pos)
				if win.Y < vi.less.Scroll().SizeHeight() {
					vi.cursor.SetCursorAtScroll(pos)
				}
			}
		case 'f':
			handled = vi.cursor.MoveDownLines(vi.less.Scroll().SizeHeight())
			if handled {
				vi.cursor.RepositionTop()
			}
		case 'b':
			handled = vi.cursor.MoveUpLines(vi.less.Scroll().SizeHeight())
			if handled {
				vi.cursor.RepositionBottom()
			}
		case 'd':
			handled = vi.cursor.MoveDownLines(vi.less.Scroll().SizeHeight() / 2)
			if handled {
				vi.cursor.RepositionTop()
			}
		case 'u':
			handled = vi.cursor.MoveUpLines(vi.less.Scroll().SizeHeight() / 2)
			if handled {
				vi.cursor.RepositionBottom()
			}
		case 'v':
			vi.setVisualBlockMode()
			handled = true
		}
	case 0:
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
			doResetCount = false
		case 'z':
			vi.setZMode()
			doResetCount = false
		case 'd':
			vi.setDeleteMode(false)
			doResetCount = false
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
			// In Vim when pasting on a visual selection it doesn't make any difference
			// if you press "p" or "P" it will always paste before the cursor.
			pasteAfter := true
			switch vi.currMode {
			case visualMode, visualLineMode, visualBlockMode:
				pasteAfter = false
			}
			vi.pasteClipboard(vi.config.defaultRegister, pasteAfter)
		case 'P':
			vi.pasteClipboard(vi.config.defaultRegister, false)
		case '^':
			vi.cursor.MoveStartLineNonBlank()
		case '$':
			vi.cursor.MoveEndLine()
		case 'G':
			vi.cursor.MoveLastLine()
		case 'j':
			vi.cursor.MoveToScroll(vi.anchor)
			if vi.count == 1 {
				vi.cursor.MoveDown()
			} else {
				vi.cursor.MoveDownLines(vi.count)
			}
		case 'k':
			vi.cursor.MoveToScroll(vi.anchor)
			if vi.count == 1 {
				vi.cursor.MoveUp()
			} else {
				vi.cursor.MoveUpLines(vi.count)
			}
		case 'h':
			if vi.count == 1 {
				vi.cursor.MoveLeft()
			} else {
				vi.cursor.MoveLeftColumns(vi.count)
			}
		case 'l':
			if vi.count == 1 {
				vi.cursor.MoveRight()
			} else {
				vi.cursor.MoveRightColumns(vi.count)
			}
		case 'O':
			vi.setInsertMode()
			vi.cursor.InsertLineAbove()
		case 'o':
			vi.setInsertMode()
			vi.cursor.InsertLineBelow()
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
			if vi.count > 1 {
				vi.cursor.MoveRightColumns(vi.count - 1)
			}
		case 'V':
			vi.setVisualLineMode()
			if vi.count > 1 {
				vi.cursor.MoveDownLines(vi.count - 1)
			}
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
			vi.less.Handle(ev)
		case '/':
			vi.searchMode = moveToNext
			vi.less.Handle(ev)
		case '%':
			cell, _ := vi.cursor.Cell()
			switch cell.Ch {
			case '[', '{', '(':
				handled = vi.cursor.MoveToNextLocation(matchingLocID)
			case ']', '}', ')':
				handled = vi.cursor.MoveToPrevLocation(matchingLocID)
			}
		case '#':
			vi.searchMode = moveToPrev
			vi.searchWord(vi.cursor.Word())
		case '*':
			vi.searchMode = moveToNext
			vi.searchWord(vi.cursor.Word())
		default:
			switch ev.Key {
			case term.KeyEsc:
				handled = vi.setNormalMode()
				vi.cursor.Unselect()
			case term.KeyArrowUp:
				vi.cursor.MoveToScroll(vi.anchor)
				vi.cursor.MoveUp()
			case term.KeyArrowRight:
				vi.cursor.MoveToScroll(vi.anchor)
				vi.cursor.MoveRight()
			case term.KeyArrowDown:
				vi.cursor.MoveToScroll(vi.anchor)
				vi.cursor.MoveDown()
			case term.KeyArrowLeft:
				vi.cursor.MoveToScroll(vi.anchor)
				vi.cursor.MoveLeft()
			default:
				if ev.Ch == '0' && vi.countDigits == "" {
					vi.cursor.MoveStartLine()
				} else if unicode.IsDigit(ev.Ch) {
					vi.countDigits += string(ev.Ch)
					parsedCount, err := strconv.Atoi(vi.countDigits)
					if err == nil {
						vi.count = parsedCount
						doResetCount = false
					} else {
						if errors.Is(err, strconv.ErrRange) {
							vi.count = math.MaxInt
							doResetCount = false
						} else {
							vi.logError(fmt.Errorf("count digits parse: %s %v", err, parsedCount))
						}
					}
					return
				}
				handled = false
			}
		}
	}

	return
}

func (vi *viHandlerImpl) resetCount() {
	vi.count = 1
	vi.countDigits = ""
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

func (vi *viHandlerImpl) searchWord(text string) {
	vi.cursor.SearchWord(text)
	switch vi.searchMode {
	case moveToNext:
		vi.cursor.MoveToNextMatch()
	case moveToPrev:
		vi.cursor.MoveToPrevMatch()
	default:
	}
}

func (vi *viHandlerImpl) exitInsert() {
	vi.cursor.MoveLeft()
	vi.repeatInsertStart()
	vi.setNormalMode()
}

func (vi *viHandlerImpl) handleInsert(ev term.Event) (quit, handled bool) {
	switch ev.Mod {
	case 0:
		switch ev.Key {
		case term.KeyEnter:
			vi.cursor.Insert('\n')
			handled = true
		case term.KeySpace:
			vi.cursor.Insert(' ')
			handled = true
		case term.KeyTab:
			vi.cursor.Insert('\t')
			handled = true
		case term.KeyBackspace:
			vi.cursor.Backspace()
			handled = true
		case term.KeyEsc:
			vi.exitInsert()
			handled = true
		case term.KeyArrowUp:
			vi.cursor.MoveToScroll(vi.anchor)
			vi.cursor.MoveUp()
			handled = true
		case term.KeyArrowRight:
			vi.cursor.MoveToScroll(vi.anchor)
			vi.cursor.MoveRight()
			handled = true
		case term.KeyArrowDown:
			vi.cursor.MoveToScroll(vi.anchor)
			vi.cursor.MoveDown()
			handled = true
		case term.KeyArrowLeft:
			vi.cursor.MoveToScroll(vi.anchor)
			vi.cursor.MoveLeft()
			handled = true
		default:
			if ev.Ch != 0 {
				vi.cursor.Insert(ev.Ch)
				handled = true
			}
		}
	case term.ModCtrl:
		switch ev.Ch {
		case 'c':
			vi.exitInsert()
			handled = true
		}
	}
	return
}

func (vi *viHandlerImpl) copySelection() {
	_, err := vi.cursor.CopySelection(vi.config.defaultRegister, vi.config.clipboard)
	if err != nil {
		vi.logError(err)
	}
}

func (vi *viHandlerImpl) repeatInsertStart() {
	from, to := term.CoordinatesSort(vi.blockRepeat.From, vi.blockRepeat.To)
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
	if ev.Type != term.EventKey {
		return
	}
	if ev.Key == term.KeyEsc || (ev.Ch == 'c' && ev.Mod == term.ModCtrl) {
		vi.setNormalMode()
		vi.cursor.Unselect()
		handled = true
		return
	}

	quit, handled = vi.handleMoveToCharacter(vi.moveMode, ev)
	if handled {
		return
	}

	handled = true
	switch ev.Mod {
	case 0:
		switch ev.Ch {
		case 'z':
			vi.cursor.HideSelection()
			vi.setNormalMode()
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
		case 'u':
			vi.cursor.LowercaseSelection()
		case 'U':
			vi.cursor.UppercaseSelection()
		case 'I':
			switch vi.mode() {
			case visualBlockMode:
				vi.handleVisualBlockInsertStart()
			default:
				handled = false
			}
		default:
			handled = false
		}
	default:
		handled = false
	}

	if !handled {
		quit, handled = vi.handleNormal(ev)
	}

	switch vi.mode() {
	case visualMode, visualLineMode, visualBlockMode:
		if ev.Mod == 0 && ev.Ch == 'p' {
			vi.setNormalMode()
		}
	default:
		vi.cursor.Unselect()
	}

	return
}

func (vi *viHandlerImpl) handleMoveToCharacter(mode moveMode, ev term.Event) (exit, handled bool) {
	switch ev.Mod {
	case 0:
		switch ev.Type {
		case term.EventKey:
			switch mode {
			case moveToNext:
				vi.cursor.MoveToNextChar(ev.Ch)
			case moveToPrev:
				vi.cursor.MoveToPrevChar(ev.Ch)
			case moveNone:
				return
			}
			vi.moveChar = ev.Ch
			vi.setNormalMode()
			handled = true
		default:
			vi.setMode(vi.mode())
			handled = true
		}
	}
	return
}

func (vi *viHandlerImpl) handleReplace(ev term.Event) (quit, handled bool) {
	if ev.Type != term.EventKey || ev.Mod != 0 {
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
	case term.KeyBackspace:
		vi.cursor.MoveLeft()
	case term.KeyEsc:
		vi.cursor.MoveLeft()
		vi.setNormalMode()
	}

	if ev.Ch != 0 {
		// do not delete column == len(row); it contains a newline
		// and that would conflate the current row with the next
		if vi.cursor.Column() < vi.less.Buffer().Columns(vi.cursor.Line()) {
			next := vi.cursor.Replace(ev.Ch)
			if vi.mode() == replaceMode { // replaceOneMode therefore stays in same char
				vi.cursor.MoveToScroll(next)
			}
		} else {
			vi.cursor.Insert(ev.Ch)
		}
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
	switch ev.Mod {
	case 0:
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
	}
	return
}

func (vi *viHandlerImpl) handleYank(ev term.Event) (quit, handled bool) {
	if ev.Ch == 'y' && ev.Mod == 0 {
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
	if !vi.deleteInsert && vi.moveMode == moveNone && ev.Ch == 'd' && ev.Mod == 0 {
		if !vi.cursor.SelectLine() {
			return
		}
		if vi.count > 1 {
			for i := 0; i < vi.count && vi.cursor.MoveLineDown(); i++ {
			}
		}
		vi.cursor.DeleteSelection()
		vi.setNormalMode()
		handled = true
		return
	}

	if vi.deleteInsert && vi.moveMode == moveNone && ev.Ch == 'c' && ev.Mod == 0 {
		vi.cursor.MoveStartLine()
		if vi.cursor.Select() {
			vi.cursor.MoveEndLine()
			vi.cursor.DeleteSelection()
			vi.cursor.TryIndent()
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

	switch ev.Mod {
	case 0:
		switch ev.Ch {
		// lowercase case 'u':
		// uppercase case 'U':
		case 'g':
			vi.cursor.MoveToScroll(vi.anchor)
			if vi.count == 1 {
				vi.cursor.MoveFirstLine()
			} else {
				target := max(0, min(vi.count-1, vi.less.Buffer().Rows()-1))
				vi.setCursorAtScroll(term.Coordinates{Y: target})
			}
			vi.resetCount()
			handled = true
		default:
		}
	}
	return
}

// Selection satisfies tui.Handler
func (vi *viHandlerImpl) Selection() (string, bool) {
	text := vi.cursor.Selection()
	return text, text != ""
}

// Handle satisfies tui.Handler
func (vi *viHandlerImpl) Handle(ev term.Event) (quit, handled bool) {
	// only a user event clears a pending set cursor
	vi.pendingSetCursor = nil

	mode := vi.mode()
	defer vi.doneHandle(mode)

	switch mode {
	case searchMode:
		quit, handled = vi.handleSearch(ev)
	case normalMode:
		quit, handled = vi.handleNormal(ev)
	case insertMode:
		quit, handled = vi.handleInsert(ev)
	case gMode:
		quit, handled = vi.handleGo(ev)
	case zMode:
		quit, handled = vi.handleZ(ev)
	case yankMode:
		quit, handled = vi.handleYank(ev)
	case deleteMode:
		quit, handled = vi.handleDelete(ev)
	case visualMode, visualLineMode, visualBlockMode:
		quit, handled = vi.handleVisual(ev)
	case replaceMode:
		quit, handled = vi.handleReplace(ev)
	case replaceOneMode:
		quit, handled = vi.handleReplace(ev)
		vi.setNormalMode()
	default:
		panic(fmt.Sprintf("unknown mode: %d", vi.currMode))
	}

	return
}

func (vi *viHandlerImpl) doneHandle(mode viMode) {
	vi.doMoveToBounds()
	if mode == normalMode || mode == visualMode /* clicks */ {
		vi.markMatchingBrace()
	}
}

func (vi *viHandlerImpl) moveToBounds() {
	vi.doMoveToBounds()
	vi.markMatchingBrace()
}

func (vi *viHandlerImpl) doMoveToBounds() {
	if vi.cursor.CursorAtScroll().Y < vi.less.Buffer().Rows() {
		vi.anchor = vi.cursorAtScroll()
	}

	switch vi.mode() {
	case normalMode, yankMode, searchMode, zMode, gMode, deleteMode:
		if vi.config.cursorCorrections {
			prevCoords := vi.cursor.Coordinates()
			vi.cursor.MoveToBounds(0)
			// Only vertical marking. Not horizontal because otherwise when scrolling
			// down with `j` or `k` from middle columns the cursor will start snapping
			// to shorter column indices as it comes across shorter text lines.
			if vi.cursor.Coordinates().Y != prevCoords.Y {
				vi.anchor = vi.cursorAtScroll()
			}
		}
	case insertMode, replaceMode, replaceOneMode,
		visualMode, visualLineMode, visualBlockMode:
		if vi.config.cursorCorrections {
			vi.cursor.MoveToBounds(1)
		}
	default:
		panic(fmt.Sprintf("unknown mode: %d", vi.currMode))
	}
}

func (vi *viHandlerImpl) OnWillSeek(from term.Coordinates) {}

func (vi *viHandlerImpl) OnDidSeek(from, to term.Coordinates) {}

func (vi *viHandlerImpl) OnWillHide(start, end int) {
}

func (vi *viHandlerImpl) OnWillVisible(start int) {
}

func (vi *viHandlerImpl) OnDidHide(start, end int) {
	// next tick because this callback is called before cursor calls MoveToScroll
	vi.config.scheduleNextTick(func() {
		vi.anchor = vi.cursorAtScroll()
	})
}

func (vi *viHandlerImpl) OnDidVisible(start int) {
	vi.config.scheduleNextTick(func() {
		vi.anchor = vi.cursorAtScroll()
	})
}

// moveToNextLocation moves the cursor to the next location
// in the location list identified by ID.
func (vi *viHandlerImpl) moveToNextLocation(ID string) bool {
	ok := vi.cursor.MoveToNextLocation(ID)
	vi.anchor = vi.cursorAtScroll()
	vi.markMatchingBrace()
	return ok
}

// moveToPrevLocation moves the cursor to the previous location
// in the location list identified by ID.
func (vi *viHandlerImpl) moveToPrevLocation(ID string) bool {
	ok := vi.cursor.MoveToPrevLocation(ID)
	vi.anchor = vi.cursorAtScroll()
	vi.markMatchingBrace()
	return ok
}

// setLocationList sets a location list of this handler. See Cursor.SetLocationList
func (vi *viHandlerImpl) setLocationList(
	pri textapi.LocationPriority, ID string, l text.LocationList,
) {
	_ = vi.cursor.SetLocationList(pri, ID, l)
}

// setCursorAtScroll sets the cursor of this viHandlerImpl handler at content pos.
func (vi *viHandlerImpl) setCursorAtScroll(pos term.Coordinates) bool {
	pos.Y = max(0, min(pos.Y, vi.less.Buffer().Rows()-1))
	pos.X = max(0, min(pos.X, vi.less.Buffer().Columns(pos.Y)))
	// setCursorAtScroll should be robust against resizes, etc.
	// only the first client interaction should clear this position
	if vi.less.Scroll().Width() == 0 || vi.less.Scroll().SizeHeight() == 0 {
		vi.pendingSetCursor = new(term.Coordinates)
		*vi.pendingSetCursor = pos
		return false
	}

	_, ok := vi.cursor.MoveToScroll(pos)
	vi.anchor = vi.cursorAtScroll()
	vi.markMatchingBrace()
	return ok
}

// CursorAtScroll sets the cursor of this viHandlerImpl handler at content pos.
func (vi *viHandlerImpl) cursorAtScroll() term.Coordinates {
	return vi.cursor.CursorAtScroll()
}

func (vi *viHandlerImpl) mode() viMode {
	if vi.currMode == normalMode && vi.less.Mode() != handler.LessNormalMode {
		return searchMode
	}
	return vi.currMode
}

func (vi *viHandlerImpl) unselect() bool {
	return vi.cursor.Unselect()
}

func (vi *viHandlerImpl) handleZ(ev term.Event) (quit, handled bool) {
	defer vi.cursor.SetLocationList(
		textapi.LocationPriorityInfo, foldHighlightLocationListID, nil)

	ctx := context.Background()
	var visual bool
	switch ev.Mod {
	case 0:
		switch ev.Ch {
		case '.':
			handled = vi.cursor.Center()
			if handled {
				vi.cursor.MoveStartLineNonBlank()
			}
		case 'z':
			handled = vi.cursor.Center()
		case 'v', 'V':
			handled = vi.cursor.SelectFold(ctx)
			visual = true
			vi.setVisualMode()
		case 'h':
			pos := vi.cursor.CursorAtScroll()
			if handled = vi.less.Scroll().SeekLeft(); handled {
				win, _ := vi.cursor.WindowCoordinates(pos)
				if win.X >= 0 {
					vi.cursor.SetCursorAtScroll(pos)
				}
			}
		case 'l':
			pos := vi.cursor.CursorAtScroll()
			if handled = vi.less.Scroll().SeekRight(); handled {
				win, _ := vi.cursor.WindowCoordinates(pos)
				if win.X < vi.less.Scroll().Width() {
					vi.cursor.SetCursorAtScroll(pos)
				}
			}
		case 'H':
			pos := vi.cursor.CursorAtScroll()
			for range vi.less.Scroll().Width() / 2 {
				if !vi.less.Scroll().SeekLeft() {
					break
				}
			}
			win, _ := vi.cursor.WindowCoordinates(pos)
			if win.X >= 0 {
				vi.cursor.SetCursorAtScroll(pos)
			}
		case 'L':
			pos := vi.cursor.CursorAtScroll()
			width := vi.less.Scroll().Width()
			for range vi.less.Scroll().Width() / 2 {
				if !vi.less.Scroll().SeekRight() {
					break
				}
			}
			win, _ := vi.cursor.WindowCoordinates(pos)
			if win.X < width {
				vi.cursor.SetCursorAtScroll(pos)
			}
		case 't':
			handled = vi.cursor.RepositionTop()
		case 'b':
			handled = vi.cursor.RepositionBottom()
		case 'c':
			handled = vi.cursor.CollapseFold(ctx)
		case 'o':
			handled = vi.cursor.ExpandFold(ctx)
		case 'a':
			handled = vi.cursor.ToggleFold(ctx)
		case 'M', 'C': // neovim uses zM, but zC follows lower/upper case convention
			handled = vi.cursor.CollapseAllFolds(ctx)
		case 'R', 'O': // neovim has a recursive vs non-recursive option
			handled = vi.cursor.ExpandAllFolds(ctx)
		case 'A':
			handled = vi.cursor.ToggleAllFolds(ctx)
		default:
		}
	}
	if !visual {
		vi.setNormalMode()
	}
	return
}
