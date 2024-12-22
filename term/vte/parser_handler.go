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

package vte

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/tcell/v3"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell/graphemecluster"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/vteparser"
	"unstable.build/go-tui/term/vte/vtescreen"
)

var _ vteparser.Handler = (*parserHandler)(nil)

const (
	pkgVersion             = 1
	selectionRegisterID    = "srid"
	defaultMaxScrollLength = 10_000
)

type parserHandler struct {
	pty       workspaceapi.Pty
	clipboard clipboard.Register
	tm        browser.TabManager
	bell      func()
	sync      struct {
		mu      sync.Locker
		buf     screenBuffer
		primBuf *vtescreen.PrimaryBuffer
		altBuf  *vtescreen.AltBuffer
	}
	tabs              tabstops
	maxScrollLength   int
	useTitleAsTabname bool

	needsAttentionAttr        term.Attributes
	needsAttention            bool
	uri                       workspaceapi.URI
	width, height             int
	inFocus                   bool
	cursorStyle               term.CursorStyle
	cursorHidden              bool
	title                     string
	titles                    []string
	modeCursorKeys            bool
	modeInsert                bool
	modeOrigin                bool
	modeBlinkingCursor        bool
	modeLineFeedNewLine       bool
	modeShowCursor            bool
	modeReportMouseClicks     bool
	modeReportCellMouseMotion bool
	modeReportAllMouseMotion  bool
	modeReportFocusInOut      bool
	modeWrap                  bool
	modeUtf8Mouse             bool
	modeSgrMouse              bool
	modeAlternateScroll       bool
	modeUrgencyHints          bool
	modeBracketedPaste        bool
	modeSyncUpdate            bool

	shouldWrap     bool
	useAlt         bool
	usedAlt        bool
	currentCharset vteparser.CharsetIndex
}

// use a common api for alternate and primary buffers
// used to simplify critical path calls and avoid extra branches
type screenBuffer interface {
	SetCursorAtScreen(c term.Coordinates, relative bool)
	SetCursorAtScroll(c term.Coordinates, relative bool)
	CursorAtScreen() term.Coordinates
	CursorAtScroll() term.Coordinates
	Insert(c rune, width int, charset vteparser.CharsetIndex)
	Write(c rune, width int, charset vteparser.CharsetIndex)
	Delete(count int)
	ResetCells(start, end int)
	ResetLines(start, end int)
	BottomScrollableRegion() int
	TopScrollableRegion() int
	Columns(line int) int
	Rows() int
	CellAt(pos term.Coordinates) *term.Cell
	CursorAttributes() term.Attributes
	SetCursorAttributes(attr term.Attributes)
	SetHiddenCursor(hidden bool)
}

func newParserHandler(
	mu sync.Locker, pty workspaceapi.Pty,
	tm browser.TabManager,
	clipboard clipboard.Register,
	bell func(),
	uri workspaceapi.URI,
	needsAttentionAttr term.Attributes,
	useTitleAsTabname bool,
) *parserHandler {
	ret := new(parserHandler)
	ret.init(mu, pty, tm, clipboard, bell, uri,
		needsAttentionAttr, useTitleAsTabname)
	return ret
}

func (t *parserHandler) init(
	mu sync.Locker, pty workspaceapi.Pty,
	tm browser.TabManager,
	clipboard clipboard.Register,
	bell func(),
	uri workspaceapi.URI,
	needsAttentionAttr term.Attributes,
	useTitleAsTabname bool,
) {
	t.sync.altBuf = vtescreen.NewAltBuffer()
	t.sync.primBuf = vtescreen.NewPrimaryBuffer()
	t.sync.buf = t.sync.primBuf
	t.sync.mu = mu
	t.pty = pty
	t.clipboard = clipboard
	t.tm = tm
	t.uri = uri
	t.title = uri.Name()
	t.maxScrollLength = defaultMaxScrollLength
	t.bell = bell
	t.needsAttentionAttr = needsAttentionAttr
	t.useTitleAsTabname = useTitleAsTabname

	// assume we are in focus when initialized
	t.inFocus = true
	t.modeAlternateScroll = true
	t.modeUrgencyHints = true
	t.modeShowCursor = true
	t.modeWrap = true
}

func (t *parserHandler) Resize(width, height int) {
	if width < 0 || height < 0 {
		return
	}

	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.sync.primBuf.Resize(width, height)
	t.sync.altBuf.Resize(width, height)
	t.width = width
	t.height = height
	t.tabs.resize(width)
}

// OSC to set window title.
func (t *parserHandler) SetTitle(title string) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if !t.useTitleAsTabname {
		return
	}

	t.updateTabName(title)
}

// Set the cursor style.
func (t *parserHandler) SetCursorStyle(style vteparser.CursorStyle) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.setCursorShape(style.Shape)
	if t.cursorHidden {
		return
	}

	if !style.Blinking {
		// already set by SetCursorShape
		return
	}

	switch style.Shape {
	case vteparser.CursorShapeBlock:
		t.cursorStyle = term.CursorStyleBlinkingBlock
	case vteparser.CursorShapeUnderline:
		t.cursorStyle = term.CursorStyleBlinkingUnderline
	case vteparser.CursorShapeBeam:
		t.cursorStyle = term.CursorStyleBlinkingBar
	case vteparser.CursorShapeHollowBlock:
		/* unsupported by tcell */
		t.cursorStyle = term.CursorStyleBlinkingBlock
	}
}

// Set the cursor shape.
func (t *parserHandler) SetCursorShape(shape vteparser.CursorShape) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.setCursorShape(shape)
}

// A character to be displayed.
func (t *parserHandler) Input(c rune) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if t.shouldWrap {
		t.wrapLine()
	}

	width := graphemecluster.StringWidth(string(c))
	if t.modeInsert {
		t.sync.buf.Insert(c, width, t.currentCharset)
	} else {
		t.sync.buf.Write(c, width, t.currentCharset)
	}

	pos := t.sync.buf.CursorAtScreen()
	pos.X++

	if pos.X < t.width {
		t.setCursorAtScreen(pos, t.modeOrigin)
	} else {
		// implementations that use DECAWM usually expect the next call to Input
		// to push the cursor down to the next line; i.e. there could be
		// carriageReturn or other commands that could be send before the next
		// call to Input if program wanted to manage the wrap around process manually.
		t.shouldWrap = true
	}
}

// Set cursor to position.
func (t *parserHandler) Goto(line int, col int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.setCursorAtScreen(term.Coordinates{Y: line, X: col}, t.modeOrigin)
}

// Set cursor to specific row.
func (t *parserHandler) GotoLine(line int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	position := term.Coordinates{
		Y: line,
		X: t.sync.buf.CursorAtScreen().X,
	}
	t.setCursorAtScreen(position, t.modeOrigin)
}

// Set cursor to specific column.
func (t *parserHandler) GotoCol(col int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	position := term.Coordinates{
		X: col,
		Y: t.sync.buf.CursorAtScreen().Y,
	}
	t.setCursorAtScreen(position, t.modeOrigin)
}

// Insert blank characters in current line starting from cursor.
func (t *parserHandler) InsertBlank(count int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScreen()
	width := t.width
	count = int(math.Min(float64(count), float64(width-pos.X)))

	for i := 0; i < count; i++ {
		t.sync.buf.Insert(' ', 1, t.currentCharset)
	}
}

// Move cursor up `rows`.
func (t *parserHandler) MoveUp(rows int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.moveUp(rows)
}

// Move cursor down `rows`.
func (t *parserHandler) MoveDown(rows int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.moveDown(rows)
}

// IdentifyTerminal identifies the terminal implementatino.
func (t *parserHandler) IdentifyTerminal(secondary bool) {
	var err error
	if secondary {
		_, err = t.pty.Master.Write([]byte(fmt.Sprintf("\x1b[>0;%d;1c", pkgVersion)))
	} else {
		_, err = t.pty.Master.Write([]byte("\x1b[?6c"))
	}
	if err != nil {
		t.log(log.ErrorLevel, "identify terminal: write to master: %v", err)
	}
}

// Report device status.
func (t *parserHandler) DeviceStatus(status int) {
	var err error
	switch status {
	case 5:
		_, err = t.pty.Master.Write([]byte("\x1b[0n"))
	case 6:
		t.sync.mu.Lock()
		pos := t.sync.buf.CursorAtScreen()
		t.sync.mu.Unlock()
		text := fmt.Sprintf("\x1b[%d;%dR", pos.Y+1, pos.X+1)
		_, err = t.pty.Master.Write([]byte(text))
	default:
		t.log(log.WarnLevel, "unknown device status query: %d", status)
	}
	if err != nil {
		t.log(log.ErrorLevel, "device status: write to master: %v", err)
	}
}

// Move cursor forward `cols`.
func (t *parserHandler) MoveForward(cols int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.moveRight(cols)
}

// Move cursor backward `cols`.
func (t *parserHandler) MoveBackward(cols int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.moveLeft(cols)
}

// Move cursor down `rows` and set to column 1.
func (t *parserHandler) MoveDownAndCR(rows int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.moveDown(rows)
	t.carriageReturn()
}

// Move cursor up `rows` and set to column 1.
func (t *parserHandler) MoveUpAndCR(rows int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.moveUp(rows)
	t.carriageReturn()
}

// Put a tab.
func (t *parserHandler) PutTab() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if t.shouldWrap {
		t.wrapLine()
		return
	}

	pos := t.sync.buf.CursorAtScroll()
	if pos.X+1 >= t.width {
		return
	}

	var cursor vtescreen.CursorState
	if t.useAlt {
		cursor = t.sync.altBuf.Cursor()
	} else {
		cursor = t.sync.primBuf.Cursor()
	}
	c := '\t'
	if charset, ok := cursor.Charsets[t.currentCharset]; ok {
		c = charset.Map(c)
	}

	// overwrite cell at current position, if it's an empty cell
	cell := t.sync.buf.CellAt(pos)
	if cell != nil && cell.Ch == vtescreen.DefaultChar {
		cell.Ch = c
	}

	// move cursor until next tab stop
	for {
		pos := t.sync.buf.CursorAtScroll()
		if pos.X+1 >= t.width {
			break
		}

		t.moveRight(1)

		if t.tabs.get(t.sync.buf.CursorAtScroll().X) {
			break
		}
	}
}

// Backspace `count` characters.
func (t *parserHandler) Backspace() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if t.sync.buf.CursorAtScreen().X <= 0 {
		return
	}
	t.moveLeft(1)
}

// Carriage return.
func (t *parserHandler) CarriageReturn() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.carriageReturn()
}

// Linefeed
func (t *parserHandler) Linefeed() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	buf := t.sync.buf
	pos := buf.CursorAtScreen()
	pos.Y++

	if pos.Y >= buf.BottomScrollableRegion() {
		t.scrollUp(1, false)
	} else if pos.Y < t.height {
		t.setCursorAtScreen(pos, t.modeOrigin)
	}
}

// Ring the bell.
func (t *parserHandler) Bell() {
	if !t.inFocus && t.modeUrgencyHints {
		t.setNeedsAttention()
	} else if t.inFocus {
		t.bell()
	}
}

// Substitute char under the cursor.
func (t *parserHandler) Substitute() {
	t.log(log.WarnLevel, "unimplemented substitute")
}

// Set current position as a tabstop.
func (t *parserHandler) SetHorizontalTabstop() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.tabs.set(t.sync.buf.CursorAtScroll().X, true)
}

// Clear tab stops.
func (t *parserHandler) ClearTabs(mode vteparser.TabulationClearMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case vteparser.TabulationClearModeCurrent:
		t.tabs.set(t.sync.buf.CursorAtScroll().X, false)
	case vteparser.TabulationClearModeAll:
		t.tabs.clearAll()
	default:
		t.log(log.WarnLevel, "unknown clear tabs mode: %v", mode)
	}
}

// Scroll up `rows` rows.
func (t *parserHandler) ScrollUp(rows int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.scrollUp(rows, false)
}

// Scroll down `rows` rows.
func (t *parserHandler) ScrollDown(rows int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.scrollDown(rows, false)
}

// Insert `count` blank lines.
func (t *parserHandler) InsertBlankLines(count int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	start := t.sync.buf.CursorAtScreen().Y
	if start >= t.sync.buf.TopScrollableRegion() && start < t.sync.buf.BottomScrollableRegion() {
		if t.useAlt {
			t.scrollDownAltRelative(start, count)
		} else {
			t.sync.primBuf.InsertLines(count)
		}
	}
}

// Delete `count` lines.
func (t *parserHandler) DeleteLines(count int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if count <= 0 {
		return
	}

	start := t.sync.buf.CursorAtScreen().Y
	if t.useAlt && start >= t.sync.buf.TopScrollableRegion() && start < t.sync.buf.BottomScrollableRegion() {
		count = int(math.Min(float64(count), float64(t.height-start)))
		t.scrollUpAltRelative(start, count)
	} else if !t.useAlt {
		t.sync.primBuf.DeleteLines(count)
	}
}

// Erase `count` chars in the current line following the cursor.
//
// Erase means resetting to the default state (default colors, no content,
// no mode flags).
func (t *parserHandler) EraseChars(count int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScroll()
	start := pos.X
	end := int(math.Min(float64(start+count), float64(t.endOfLine(pos.Y))))

	t.sync.buf.ResetCells(start, end)
}

// Delete `count` chars.
//
// Deleting a character is like the delete key on the keyboard - everything
// to the right of the deleted things is shifted left.
func (t *parserHandler) DeleteChars(count int) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.sync.buf.Delete(count)
}

// Move backward `count` tabs.
func (t *parserHandler) MoveBackwardTabs(count int) {
	t.log(log.WarnLevel, "unimplemented move backward %d tabs", count)
}

// Move forward `count` tabs.
func (t *parserHandler) MoveForwardTabs(count int) {
	t.log(log.WarnLevel, "unimplemented move forward %d tabs", count)
}

// Save the current cursor position.
func (t *parserHandler) SaveCursorPosition() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if t.useAlt {
		t.sync.altBuf.SaveCursor()
	} else {
		t.sync.primBuf.SaveCursor()
	}
}

// Restore cursor position.
func (t *parserHandler) RestoreCursorPosition() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if t.useAlt {
		t.sync.altBuf.RestoreCursor()
	} else {
		t.sync.primBuf.RestoreCursor()
	}
}

// Clear the current line.
func (t *parserHandler) ClearLine(mode vteparser.LineClearMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScroll()
	var start, end int
	switch mode {
	case vteparser.LineClearModeRight:
		if t.shouldWrap {
			return
		}
		start = pos.X
		end = t.endOfLine(pos.Y)
	case vteparser.LineClearModeLeft:
		end = pos.X + 1
	case vteparser.LineClearModeAll:
		end = t.endOfLine(pos.Y)
	default:
		t.log(log.WarnLevel, "unknown clear line mode: %v", mode)
		return
	}

	t.sync.buf.ResetCells(start, end)
}

// Clear the screen.
func (t *parserHandler) ClearScreen(mode vteparser.ClearMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScroll()
	columns := t.endOfLine(pos.Y)
	lines := t.maxRows()

	switch mode {
	case vteparser.ClearModeBelow:
		t.sync.buf.ResetCells(pos.X, columns)
		if pos.Y+1 < lines {
			t.sync.buf.ResetLines(pos.Y+1, lines)
		}

	case vteparser.ClearModeAbove:
		if pos.Y > 1 {
			t.sync.buf.ResetLines(0, pos.Y)
		}
		start := 0
		end := int(math.Min(float64(pos.X+1), float64(columns)))
		t.sync.buf.ResetCells(start, end)

	case vteparser.ClearModeAll:
		if t.useAlt {
			t.resetBufLines(t.sync.buf)
		} else {
			t.clearPrimaryView()
		}

	case vteparser.ClearModeSaved:
		if t.useAlt {
			return
		}
		pos := t.sync.primBuf.Offset()
		t.sync.buf.ResetLines(0, pos.Y)
	default:
		t.log(log.WarnLevel, "unknown clear screen mode: %v", mode)
	}
}

// Reset parserHandler state.
func (t *parserHandler) ResetState() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pty := t.pty
	clipboard := t.clipboard
	tm := t.tm
	uri := t.uri
	needsAttentionAttr := t.needsAttentionAttr
	bell := t.bell
	mu := t.sync.mu
	*t = parserHandler{}
	t.init(mu, pty, tm, clipboard, bell, uri,
		needsAttentionAttr, t.useTitleAsTabname)
}

// Reverse Index.
//
// Move the active position to the same horizontal position on the
// preceding line. If the active position is at the top margin, a scroll
// down is performed.
func (t *parserHandler) ReverseIndex() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScreen()
	if pos.Y <= t.sync.buf.TopScrollableRegion() {
		t.scrollDown(1, false)
	} else {
		t.moveUp(1)
	}
}

// Set a parserHandler attribute.
func (t *parserHandler) TerminalAttribute(pattr vteparser.Attr) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	attr := t.sync.buf.CursorAttributes()

	switch pattr.Type {
	case vteparser.ResetAttr:
		attr.Fg = 0
		attr.Bg = 0
		attr.Attrs = 0
	case vteparser.BoldAttr:
		attr.Attrs |= tcell.AttrBold
	case vteparser.DimAttr:
		attr.Attrs |= tcell.AttrDim
	case vteparser.ItalicAttr:
		attr.Attrs |= tcell.AttrItalic
	case vteparser.UnderlineAttr:
		attr.Attrs |= tcell.AttrUnderline
	case vteparser.BlinkSlowAttr, vteparser.BlinkFastAttr:
		attr.Attrs |= tcell.AttrBlink
	case vteparser.ReverseAttr:
		attr.Attrs |= tcell.AttrReverse
	case vteparser.HiddenAttr:
		t.sync.buf.SetHiddenCursor(true)
		return
	case vteparser.StrikeAttr:
		attr.Attrs |= tcell.AttrStrikeThrough
	case vteparser.CancelBoldAttr:
		attr.Attrs &^= tcell.AttrBold
	case vteparser.CancelBoldDimAttr:
		attr.Attrs &^= tcell.AttrBold
		attr.Attrs &^= tcell.AttrDim
	case vteparser.CancelItalicAttr:
		attr.Attrs &^= tcell.AttrItalic
	case vteparser.CancelUnderlineAttr:
		attr.Attrs &^= tcell.AttrUnderline
	case vteparser.CancelBlinkAttr:
		attr.Attrs &^= tcell.AttrBlink
	case vteparser.CancelReverseAttr:
		attr.Attrs &^= tcell.AttrReverse
	case vteparser.CancelHiddenAttr:
		t.sync.buf.SetHiddenCursor(false)
		return
	case vteparser.CancelStrikeAttr:
		attr.Attrs &^= tcell.AttrStrikeThrough
	case vteparser.ForegroundAttr:
		attr.Fg = pattr.Color
	case vteparser.BackgroundAttr:
		attr.Bg = pattr.Color
	case vteparser.DoubleUnderlineAttr, vteparser.UndercurlAttr,
		vteparser.DottedUnderlineAttr, vteparser.DashedUnderlineAttr,
		vteparser.UnderlineColorAttr:
		/* ignored */
		return
	}

	t.sync.buf.SetCursorAttributes(attr)
}

// Report private mode
func (t *parserHandler) ReportPrivateMode(mode vteparser.PrivateMode) {
	// no need to sync since writes only occur on pty parsing goroutine

	var modeVar *bool
	switch mode {
	case vteparser.PrivateModeCursorKeys:
		modeVar = &t.modeCursorKeys
	case vteparser.PrivateModeColumnMode:
		/* not supported for reporting */
	case vteparser.PrivateModeOrigin:
		modeVar = &t.modeOrigin
	case vteparser.PrivateModeScreen:
		/* DECSCNM not supported */
	case vteparser.PrivateModeLineWrap:
		modeVar = &t.modeWrap
	case vteparser.PrivateModeBlinkingCursor:
		modeVar = &t.modeBlinkingCursor
	case vteparser.PrivateModeShowCursor:
		modeVar = &t.modeShowCursor
	case vteparser.PrivateModeReportMouseClicks:
		modeVar = &t.modeReportMouseClicks
	case vteparser.PrivateModeReportCellMouseMotion:
		modeVar = &t.modeReportCellMouseMotion
	case vteparser.PrivateModeReportAllMouseMotion:
		modeVar = &t.modeReportAllMouseMotion
	case vteparser.PrivateModeReportFocusInOut:
		modeVar = &t.modeReportFocusInOut
	case vteparser.PrivateModeUtf8Mouse:
		modeVar = &t.modeUtf8Mouse
	case vteparser.PrivateModeSgrMouse:
		modeVar = &t.modeSgrMouse
	case vteparser.PrivateModeAlternateScroll:
		modeVar = &t.modeAlternateScroll
	case vteparser.PrivateModeUrgencyHints:
		modeVar = &t.modeUrgencyHints
	case vteparser.PrivateModeSwapScreenAndSetRestoreCursor:
		modeVar = &t.useAlt
	case vteparser.PrivateModeBracketedPaste:
		modeVar = &t.modeBracketedPaste
	case vteparser.PrivateModeSyncUpdate:
		modeVar = &t.modeSyncUpdate
	default:
		t.log(log.WarnLevel, "Set unkown private mode: %v", mode)
	}
	t.reportMode("\x1b[?%d;%d$y", int(mode), modeVar)
}

// Set private mode.
func (t *parserHandler) SetPrivateMode(mode vteparser.PrivateMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case vteparser.PrivateModeCursorKeys:
		t.modeCursorKeys = true
	case vteparser.PrivateModeColumnMode:
		t.deccolm()
	case vteparser.PrivateModeOrigin:
		t.modeOrigin = true
	case vteparser.PrivateModeScreen:
		/* DECSCNM not supported */
	case vteparser.PrivateModeLineWrap:
		t.modeWrap = true
	case vteparser.PrivateModeBlinkingCursor:
		t.modeBlinkingCursor = true
	case vteparser.PrivateModeShowCursor:
		t.modeShowCursor = true
	case vteparser.PrivateModeReportMouseClicks:
		t.modeReportMouseClicks = true
	case vteparser.PrivateModeReportCellMouseMotion:
		t.modeReportCellMouseMotion = true
	case vteparser.PrivateModeReportAllMouseMotion:
		t.modeReportAllMouseMotion = true
	case vteparser.PrivateModeReportFocusInOut:
		t.modeReportFocusInOut = true
	case vteparser.PrivateModeUtf8Mouse:
		t.modeUtf8Mouse = true
	case vteparser.PrivateModeSgrMouse:
		t.modeSgrMouse = true
	case vteparser.PrivateModeAlternateScroll:
		t.modeAlternateScroll = true
	case vteparser.PrivateModeUrgencyHints:
		t.modeUrgencyHints = true
	case vteparser.PrivateModeSwapScreenAndSetRestoreCursor:
		if !t.useAlt {
			t.swapAlt()
		}
	case vteparser.PrivateModeBracketedPaste:
		t.modeBracketedPaste = true
	case vteparser.PrivateModeSyncUpdate:
		t.modeSyncUpdate = true
	default:
		t.log(log.WarnLevel, "Set unkown private mode: %v", mode)
		return
	}
}

// Unset private mode.
func (t *parserHandler) UnsetPrivateMode(mode vteparser.PrivateMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case vteparser.PrivateModeCursorKeys:
		t.modeCursorKeys = false
	case vteparser.PrivateModeColumnMode:
		t.deccolm()
	case vteparser.PrivateModeOrigin:
		t.modeOrigin = false
	case vteparser.PrivateModeScreen:
		/* DECSCNM not supported */
	case vteparser.PrivateModeLineWrap:
		t.modeWrap = false
	case vteparser.PrivateModeBlinkingCursor:
		t.modeBlinkingCursor = false
	case vteparser.PrivateModeShowCursor:
		t.modeShowCursor = false
	case vteparser.PrivateModeReportMouseClicks:
		t.modeReportMouseClicks = false
	case vteparser.PrivateModeReportCellMouseMotion:
		t.modeReportCellMouseMotion = false
	case vteparser.PrivateModeReportAllMouseMotion:
		t.modeReportAllMouseMotion = false
	case vteparser.PrivateModeReportFocusInOut:
		t.modeReportFocusInOut = false
	case vteparser.PrivateModeUtf8Mouse:
		t.modeUtf8Mouse = false
	case vteparser.PrivateModeSgrMouse:
		t.modeSgrMouse = false
	case vteparser.PrivateModeAlternateScroll:
		t.modeAlternateScroll = false
	case vteparser.PrivateModeUrgencyHints:
		t.modeUrgencyHints = false
	case vteparser.PrivateModeSwapScreenAndSetRestoreCursor:
		if t.useAlt {
			t.swapAlt()
		}
	case vteparser.PrivateModeBracketedPaste:
		t.modeBracketedPaste = false
	case vteparser.PrivateModeSyncUpdate:
		t.modeSyncUpdate = false
	default:
		t.log(log.WarnLevel, "Unset unkown private mode: %v", mode)
		return
	}
}

// Report mode
func (t *parserHandler) ReportMode(mode vteparser.Mode) {
	// no need to sync since writes only occur on pty parsing goroutine

	var modeVar *bool
	switch mode {
	case vteparser.ModeInsert:
		modeVar = &t.modeInsert
	case vteparser.ModeLineFeedNewLine:
		modeVar = &t.modeLineFeedNewLine
	default:
		t.log(log.DebugLevel, "Report unkown public mode: %v", mode)
	}

	t.reportMode("\x1b[%d;%d$y", int(mode), modeVar)
}

// Set mode.
func (t *parserHandler) SetMode(mode vteparser.Mode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case vteparser.ModeInsert:
		t.modeInsert = true
	case vteparser.ModeLineFeedNewLine:
		t.modeLineFeedNewLine = true
	default:
		t.log(log.WarnLevel, "Set unkown public mode: %v", mode)
		return
	}
}

// Unset mode.
func (t *parserHandler) UnsetMode(mode vteparser.Mode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case vteparser.ModeInsert:
		t.modeInsert = false
	case vteparser.ModeLineFeedNewLine:
		t.modeLineFeedNewLine = false
	default:
		t.log(log.WarnLevel, "Unset unkown public mode: %v", mode)
		return
	}
}

// DECSTBM - Set the parserHandler scrolling region.
func (t *parserHandler) SetScrollingRegion(top, bottom int, end bool) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if t.useAlt {
		t.setScrollingRegion(top, bottom, end)
	} else {
		t.shouldWrap = false
	}
}

// DECKPAM - Set the keypad to applications mode (ESCape instead of digits).
func (t *parserHandler) SetKeypadApplicationMode() {
	t.log(log.WarnLevel, "unsupported call to SetKeypadApplicationMode")
}

// DECKPNM - Set the keypad to numeric mode (digits instead of ESCape seq).
func (t *parserHandler) UnsetKeypadApplicationMode() {
	t.log(log.WarnLevel, "unsupported call to UnsetKeypadApplicationMode")
}

// Set one of the graphic character sets, G0 to G3, as the active charset.
//
// 'Invoke' one of G0 to G3 in the GL area. Also referred to as shift in,
// shift out and locking shift depending on the set being activated.
func (t *parserHandler) SetActiveCharset(index vteparser.CharsetIndex) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.currentCharset = index
}

// Assign a graphic character set to G0, G1, G2 or G3.
//
// 'Designate' a graphic character set as one of G0 to G3 so that it can
// later be 'invoked' by `SetActiveCharset`.
func (t *parserHandler) ConfigureCharset(
	index vteparser.CharsetIndex, charset vteparser.StandardCharset,
) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	var buf *vtescreen.AltBuffer
	if t.useAlt {
		buf = t.sync.altBuf
	} else {
		buf = &t.sync.primBuf.AltBuffer
	}
	buf.ConfigureCharset(index, charset)
}

// Store data into the clipboard.
func (t *parserHandler) ClipboardStore(register int, data []byte) {
	d := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(data))
	decodedData, err := io.ReadAll(d)
	if err != nil {
		t.log(log.WarnLevel, "decode base64 data "+
			"for ClipboardStore (len=%d): %v", len(data), err)
		return
	}

	clipData := clipboard.Data{Text: string(decodedData)}
	switch register {
	case int('c'):
		err = t.clipboard.Copy(clipboard.DefaultRegisterID, clipData)
	case int('p') | int('s'):
		err = t.clipboard.Copy(selectionRegisterID, clipData)
	default:
		t.log(log.WarnLevel, "unknown register ID upon ClipboardStore: %c", rune(register))
	}
	if err != nil {
		t.log(log.ErrorLevel, "copy to clipboard: %v", err)
	}
}

// Load data from the clipboard.
func (t *parserHandler) ClipboardLoad(register int, terminator string) {
	var data clipboard.Data
	var err error
	switch register {
	case int('c'):
		data, err = t.clipboard.Paste(clipboard.DefaultRegisterID)
	case int('p') | int('s'):
		data, err = t.clipboard.Paste(selectionRegisterID)
	default:
		t.log(log.WarnLevel, "unknown register ID upon ClipboardLoad: %c", rune(register))
		return
	}
	if err != nil {
		t.log(log.ErrorLevel, "load data from clipboard "+
			"for ClipboardLoad: %v", err)
		t.sync.mu.Unlock()
		return
	}

	var buf bytes.Buffer
	w := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := w.Write([]byte(data.Text)); err != nil {
		t.log(log.WarnLevel, "encode data in base64 "+
			"for ClipboardLoad: %v", err)
		return
	}
	if err := w.Close(); err != nil {
		t.log(log.WarnLevel, "encode data in base64 "+
			"for ClipboardLoad: %v", err)
		return
	}

	encodedCmd := fmt.Sprintf("\x1b]52;%c;%s%s", rune(register), buf.String(), terminator)
	if _, err := t.pty.Master.Write([]byte(encodedCmd)); err != nil {
		t.log(log.WarnLevel, "write encoded clipboard data to pty "+
			"for ClipboardLoad: %v", err)
		return
	}
}

// Run the decaln routine.
func (t *parserHandler) Decaln() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	// keep the screenBuffer ifc small and Decaln
	// is not critical path
	var buf *vtescreen.AltBuffer
	if t.useAlt {
		buf = t.sync.altBuf
	} else {
		buf = &t.sync.primBuf.AltBuffer
	}
	buf.ResetLinesWith(0, buf.Cells.Rows(), 'E')
}

// Push a title onto the stack.
func (t *parserHandler) PushTitle() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.titles = append(t.titles, t.title)
}

// Pop the last title from the stack.
func (t *parserHandler) PopTitle() {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	if len(t.titles) > 0 {
		e := t.titles[len(t.titles)-1]
		t.titles = t.titles[:len(t.titles)-1]
		if !t.useTitleAsTabname {
			return
		}
		t.updateTabName(e)
	}
}

// Report text area size in pixels.
func (t *parserHandler) TextAreaSizePixels() {
	t.log(log.WarnLevel, "unsupported call to TextAreaSizePixels")
}

// Report text area size in characters.
func (t *parserHandler) TextAreaSizeChars() {
	t.sync.mu.Lock()
	data := fmt.Sprintf("\x1b[8;%d;%dt", t.height, t.width)
	t.sync.mu.Unlock()

	if _, err := t.pty.Master.Write([]byte(data)); err != nil {
		t.log(log.WarnLevel, "write text area size in chars: %v", err)
		return
	}
}

// Set hyperlink.
func (t *parserHandler) SetHyperlink(link *vteparser.Hyperlink) {
	t.log(log.WarnLevel, "unsupported call to SetHyperlink")
}

// ReportKeyboardMode reports current keyboard mode.
func (t *parserHandler) ReportKeyboardMode() {
	t.log(log.WarnLevel, "unsupported call to ReportKeyboardMode")
}

// PushKeyboardMode pushes the keyboard mode into the keyboard mode stack.
func (t *parserHandler) PushKeyboardMode(mode vteparser.KeyboardMode) {
	t.log(log.WarnLevel, "unsupported call to PushKeyboardMode: "+
		"kitty keyboard handling not supported yet")
}

// PopKeyboardModes pops the given amount of keyboard modes
// from the keyboard mode stack.
func (t *parserHandler) PopKeyboardModes(count int) {
	t.log(log.WarnLevel, "unsupported call to PopKeyboardModes: "+
		"kitty keyboard handling not supported yet")
}

// SetKeyboardMode sets the [`keyboard mode`] using the given [`behavior`].
func (t *parserHandler) SetKeyboardMode(
	mode vteparser.KeyboardMode, behavior vteparser.KeyboardModesApplyBehavior,
) {
	t.log(log.WarnLevel, "unsupported call to SetKeyboardMode: "+
		"kitty keyboard handling not supported yet")
}

// SetModifyOtherKeys sets XTerm's [`ModifyOtherKeys`] option.
func (t *parserHandler) SetModifyOtherKeys(mode vteparser.ModifyOtherKeysMode) {
	t.log(log.WarnLevel, "unsupported call to SetModifyOtherKeys")
}

// ReportModifyOtherKeys report XTerm's [`ModifyOtherKeys`] state.
func (t *parserHandler) ReportModifyOtherKeys() {
	t.log(log.WarnLevel, "unsupported call to ReportModifyOtherKeys")
}

func (t *parserHandler) log(level log.Level, line string, params ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "vte.parserHandler").
		Logf(level, line, params...)
}

func (t *parserHandler) reportMode(template string, mode int, modeVar *bool) {
	var rep int
	if modeVar == nil {
		rep = 0
	} else if *modeVar {
		rep = 1
	} else {
		rep = 2
	}

	_, err := t.pty.Master.Write([]byte(
		fmt.Sprintf(template, mode, rep),
	))
	if err != nil {
		t.log(log.WarnLevel, "report mode with template %q: %v", template, err)
	}
}

func (t *parserHandler) swapAlt() {
	if !t.useAlt {
		// Set alt screen cursor to the current primary screen cursor.
		t.sync.altBuf.SetCursor(t.sync.primBuf.CloneCursor())

		// Drop information about the primary screens saved cursor.
		t.sync.primBuf.SetSavedCursor(t.sync.primBuf.CloneCursor())

		// Reset alternate screen contents.
		t.resetBufLines(t.sync.altBuf)

		t.sync.buf = t.sync.altBuf
		t.useAlt = true
		t.usedAlt = true

	} else {
		t.sync.buf = t.sync.primBuf
		t.useAlt = false
	}
}

func (t *parserHandler) deccolm() {
	if t.useAlt {
		t.setScrollingRegion(1, 0, true)
	} else {
		t.sync.primBuf.SetOffset(term.Coordinates{})
		t.shouldWrap = false
	}
	t.resetBufLines(t.sync.buf)
}

func (t *parserHandler) resetBufLines(buf screenBuffer) {
	// use rows rather than height if height is not yet == rows
	// to be resilient against constant resizes
	buf.ResetLines(0, t.maxRows())
}

func (t *parserHandler) carriageReturn() {
	t.setCursorAtScreen(term.Coordinates{
		X: 0,
		Y: t.sync.buf.CursorAtScreen().Y,
	}, false)
}

func (t *parserHandler) scrollDown(rows int, userScroll bool) bool {
	if t.useAlt {
		return t.scrollDownAltRelative(t.sync.buf.TopScrollableRegion(), rows)
	}

	buf := t.sync.primBuf
	offset := buf.Offset()
	t.shouldWrap = false

	if userScroll {
		newOffset := int(math.Max(float64(offset.Y-rows), 0))
		if offset.Y != newOffset {
			buf.MoveToOffset(term.Coordinates{Y: newOffset})
			return true
		}
		return false
	}

	// primary buffer includes history so we cannot simply
	// use bottom and top of scrollable region.
	start := buf.Rows() - t.height
	end := buf.Rows()
	count := int(math.Min(float64(rows), float64(end-start)))
	if count != 0 {
		buf.AltBuffer.ScrollDown(start, end, count)
		return true
	}
	return false
}

func (t *parserHandler) scrollUp(rows int, userScroll bool) bool {
	if t.useAlt {
		return t.scrollUpAltRelative(t.sync.buf.TopScrollableRegion(), rows)
	}

	buf := t.sync.primBuf
	offset := buf.Offset()
	t.shouldWrap = false

	if userScroll {
		newOffset := int(math.Min(float64(offset.Y+rows), float64(buf.MaxOffset())))
		if offset.Y != newOffset {
			buf.MoveToOffset(term.Coordinates{Y: newOffset})
			return true
		}
		return false
	}

	cursorAtScroll := buf.CursorAtScroll()
	if cursorAtScroll.Y+rows >= t.maxScrollLength {
		buf.AltBuffer.ScrollUp(0, buf.Rows(), rows)
	} else if cursorAtScroll.Y >= buf.Rows() {
		offset.Y += rows
		buf.SetOffset(offset)
		// optimization for long streams of output so all columns are pre-allocated
		// by using the underlying buffer's configured column capacity, thus
		// reducing the number of allocations.
		buf.ResetCells(0, t.width)
	} else {
		offset.Y += rows
		buf.SetOffset(offset)
	}
	return true
}

func (t *parserHandler) scrollUpAltRelative(start int, count int) bool {
	if t.sync.buf != t.sync.altBuf {
		panic("called scroll up relative on non alternate buffer")
	}
	count = int(math.Min(
		float64(count),
		float64(t.sync.buf.BottomScrollableRegion()-t.sync.buf.TopScrollableRegion()),
	))
	end := t.sync.altBuf.BottomScrollableRegion()
	ok := count != 0
	if ok {
		t.sync.altBuf.ScrollUp(start, end, count)
	}
	return ok
}

func (t *parserHandler) scrollDownAltRelative(start int, count int) bool {
	if t.sync.buf != t.sync.altBuf {
		panic("called scroll down relative on non alternate buffer")
	}
	count = int(math.Min(
		float64(count),
		float64(t.sync.buf.BottomScrollableRegion()-t.sync.buf.TopScrollableRegion()),
	))
	count = int(math.Min(
		float64(count),
		float64(t.sync.buf.BottomScrollableRegion()-start),
	))

	end := t.sync.altBuf.BottomScrollableRegion()
	ok := count != 0
	if ok {
		t.sync.altBuf.ScrollDown(start, end, count)
	}
	return ok
}

// moveUp moves the cursor up delta lines.
func (t *parserHandler) moveUp(delta int) {
	pos := t.sync.buf.CursorAtScreen()
	pos.Y -= delta
	t.setCursorAtScreen(pos, t.modeOrigin)
}

// moveDown moves the cursor down delta lines.
func (t *parserHandler) moveDown(delta int) {
	pos := t.sync.buf.CursorAtScreen()
	pos.Y += delta
	t.setCursorAtScreen(pos, t.modeOrigin)
}

// moveRight moves the cursor right delta cells.
func (t *parserHandler) moveRight(delta int) {
	pos := t.sync.buf.CursorAtScreen()
	pos.X += delta
	t.setCursorAtScreen(pos, t.modeOrigin)
}

// moveLeft moves the cursor left delta cells.
func (t *parserHandler) moveLeft(delta int) {
	pos := t.sync.buf.CursorAtScreen()
	pos.X -= delta
	t.setCursorAtScreen(pos, t.modeOrigin)
}

func (t *parserHandler) setCursorShape(shape vteparser.CursorShape) {
	t.cursorHidden = shape == vteparser.CursorShapeHidden
	if t.cursorHidden {
		return
	}
	switch shape {
	case vteparser.CursorShapeBlock:
		t.cursorStyle = term.CursorStyleSteadyBlock
	case vteparser.CursorShapeUnderline:
		t.cursorStyle = term.CursorStyleSteadyUnderline
	case vteparser.CursorShapeBeam:
		t.cursorStyle = term.CursorStyleSteadyBar
	case vteparser.CursorShapeHollowBlock:
		/* unsupported by tcell */
		t.cursorStyle = term.CursorStyleSteadyBlock
	}
}

func (t *parserHandler) setScrollingRegion(top, bottom int, end bool) {
	if t.sync.buf != t.sync.altBuf {
		panic("called set scrolling region on non alternate buffer")
	}
	// top and bottom are not zero indexed. We leave bottom intact to
	// maintain right exclusive range semantics.
	top--
	t.sync.altBuf.SetScrollableRegion(top, bottom, end)
	t.setCursorAtScreen(term.Coordinates{}, true)
}

func (t *parserHandler) wrapLine() {
	if !t.modeWrap {
		return
	}

	buf := t.sync.buf
	pos := buf.CursorAtScreen()
	pos.X = 0
	if pos.Y+1 >= buf.BottomScrollableRegion() {
		t.scrollUp(1, false)
	} else {
		pos.Y++
	}

	t.setCursorAtScreen(pos, false)
	t.shouldWrap = false
}

func (t *parserHandler) setCursorAtScreen(pos term.Coordinates, relative bool) {
	t.sync.buf.SetCursorAtScreen(pos, relative)
	t.shouldWrap = false
}

func (t *parserHandler) clearPrimaryView() {
	if t.sync.buf != t.sync.primBuf {
		panic("called scroll up view on non primary buffer")
	}

	buf := t.sync.primBuf
	cells := buf.Cells.RawCells()
	for y := buf.Rows() - 1; y >= 0; y-- {
		for x := 0; x < buf.Columns(y); x++ {
			c := cells[y][x]
			if c.Ch != vtescreen.DefaultChar {
				newOffset := term.Coordinates{Y: y + 1}
				t.log(log.TraceLevel, "new offset after clear view %+v", newOffset)
				buf.SetOffset(newOffset)
				return
			}
		}
	}
}

func (t *parserHandler) endOfLine(y int) int {
	return int(math.Max(float64(t.sync.buf.Columns(y)), float64(t.width)))
}

func (t *parserHandler) maxRows() int {
	return int(math.Max(float64(t.height), float64(t.sync.buf.Rows())))
}

func (t *parserHandler) clearNeedsAttention() {
	t.needsAttention = false
	err := t.tm.SetTabName(t.uri, t.title, term.Attributes{})
	if err != nil {
		t.log(log.ErrorLevel, "set tab name: %v", err)
	}
}

func (t *parserHandler) setNeedsAttention() {
	t.needsAttention = true
	err := t.tm.SetTabName(t.uri, t.title, t.needsAttentionAttr)
	if err != nil {
		t.log(log.ErrorLevel, "set tab name: %v", err)
	}
}

func (t *parserHandler) onFocusChange(inFocus bool) (cmd string, ok bool) {
	if inFocus && t.needsAttention {
		t.clearNeedsAttention()
	}
	t.inFocus = inFocus

	t.log(log.TraceLevel, "OnFocusChange(inFocus=%t, reportFocusMode=%t)",
		inFocus, t.modeReportFocusInOut)
	if !t.modeReportFocusInOut {
		return
	}

	if inFocus {
		cmd = "I"
	} else {
		cmd = "O"
	}
	ok = true
	return
}

func (t *parserHandler) updateTabName(title string) {
	t.title = title
	if t.needsAttention {
		t.setNeedsAttention()
	} else {
		t.clearNeedsAttention()
	}
}

func (t *parserHandler) usedAlternate() bool {
	return t.usedAlt
}
