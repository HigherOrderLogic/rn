package vte

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/ernestrc/blue/logging"
	"github.com/ernestrc/tcell/v3"
	"github.com/rivo/uniseg"
	log "github.com/sirupsen/logrus"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/parser"
	"unstable.build/go-tui/term/vte/screen"
	"unstable.build/go-tui/text/clipboard"
)

var _ parser.Handler = (*parserHandler)(nil)

const (
	pkgVersion             = 1
	selectionRegisterID    = "srid"
	defaultMaxScrollLength = 10_000
)

type parserHandler struct {
	pty       workspaceapi.Pty
	clipboard clipboard.Register
	sync      struct {
		mu      sync.Locker
		buf     screenBuffer
		primBuf *screen.PrimaryBuffer
		altBuf  *screen.AltBuffer
	}
	tabs            tabstops
	maxScrollLength int

	width, height             int
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
	currentCharset parser.CharsetIndex
}

// use a common api for alternate and primary buffers
// used to simplify critical path calls and avoid extra branches
type screenBuffer interface {
	SetCursorAtScreen(c term.Coordinates, relative bool)
	SetCursorAtScroll(c term.Coordinates, relative bool)
	CursorAtScreen() term.Coordinates
	CursorAtScroll() term.Coordinates
	Insert(c rune, width int, charset parser.CharsetIndex)
	Write(c rune, width int, charset parser.CharsetIndex)
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
	clipboard clipboard.Register,
) *parserHandler {
	ret := new(parserHandler)
	ret.init(mu, pty, clipboard)
	return ret
}

func (t *parserHandler) init(
	mu sync.Locker, pty workspaceapi.Pty,
	clipboard clipboard.Register,
) {
	t.sync.altBuf = screen.NewAltBuffer()
	t.sync.primBuf = screen.NewPrimaryBuffer()
	t.sync.buf = t.sync.primBuf
	t.sync.mu = mu
	t.pty = pty
	t.clipboard = clipboard
	t.maxScrollLength = defaultMaxScrollLength

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

	t.title = title
}

// Set the cursor style.
func (t *parserHandler) SetCursorStyle(style parser.CursorStyle) {
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
	case parser.CursorShapeBlock:
		t.cursorStyle = term.CursorStyleBlinkingBlock
	case parser.CursorShapeUnderline:
		t.cursorStyle = term.CursorStyleBlinkingUnderline
	case parser.CursorShapeBeam:
		t.cursorStyle = term.CursorStyleBlinkingBar
	case parser.CursorShapeHollowBlock:
		/* unsupported by tcell */
		t.cursorStyle = term.CursorStyleBlinkingBlock
	}
}

// Set the cursor shape.
func (t *parserHandler) SetCursorShape(shape parser.CursorShape) {
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

	width := uniseg.StringWidth(string(c))
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
	if secondary {
		t.pty.Master.Write([]byte(fmt.Sprintf("\x1b[>0;%d;1c", pkgVersion)))
	} else {
		t.pty.Master.Write([]byte("\x1b[?6c"))
	}
}

// Report device status.
func (t *parserHandler) DeviceStatus(status int) {
	switch status {
	case 5:
		t.pty.Master.Write([]byte("\x1b[0n"))
	case 6:
		t.sync.mu.Lock()
		pos := t.sync.buf.CursorAtScreen()
		t.sync.mu.Unlock()
		text := fmt.Sprintf("\x1b[%d;%dR", pos.Y+1, pos.X+1)
		t.pty.Master.Write([]byte(text))
	default:
		t.log(log.WarnLevel, "unknown device status query: %d", status)
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

	var cursor screen.CursorState
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
	if cell != nil && (cell.Ch == ' ' || cell.Ch == 0) {
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
	// TODO do something with the draw output, maybe an animation?
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
func (t *parserHandler) ClearTabs(mode parser.TabulationClearMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case parser.TabulationClearModeCurrent:
		t.tabs.set(t.sync.buf.CursorAtScroll().X, false)
	case parser.TabulationClearModeAll:
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
func (t *parserHandler) ClearLine(mode parser.LineClearMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScroll()
	var start, end int
	switch mode {
	case parser.LineClearModeRight:
		if t.shouldWrap {
			return
		}
		start = pos.X
		end = t.endOfLine(pos.Y)
	case parser.LineClearModeLeft:
		end = pos.X + 1
	case parser.LineClearModeAll:
		end = t.endOfLine(pos.Y)
	default:
		t.log(log.WarnLevel, "unknown clear line mode: %v", mode)
		return
	}

	t.sync.buf.ResetCells(start, end)
}

// Clear the screen.
func (t *parserHandler) ClearScreen(mode parser.ClearMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	pos := t.sync.buf.CursorAtScroll()
	columns := t.endOfLine(pos.Y)
	lines := t.maxRows()

	switch mode {
	case parser.ClearModeBelow:
		t.sync.buf.ResetCells(pos.X, columns)
		if pos.Y+1 < lines {
			t.sync.buf.ResetLines(pos.Y+1, lines)
		}

	case parser.ClearModeAbove:
		if pos.Y > 1 {
			t.sync.buf.ResetLines(0, pos.Y)
		}
		start := 0
		end := int(math.Min(float64(pos.X+1), float64(columns)))
		t.sync.buf.ResetCells(start, end)

	case parser.ClearModeAll:
		if t.useAlt {
			t.resetBufLines(t.sync.buf)
		} else {
			t.scrollUpPrimaryView()
		}

	case parser.ClearModeSaved:
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
	mu := t.sync.mu
	*t = parserHandler{}
	t.init(mu, pty, clipboard)
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
func (t *parserHandler) TerminalAttribute(pattr parser.Attr) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	attr := t.sync.buf.CursorAttributes()

	switch pattr.Type {
	case parser.ResetAttr:
		attr.Fg = 0
		attr.Bg = 0
		attr.Attrs = 0
	case parser.BoldAttr:
		attr.Attrs |= tcell.AttrBold
	case parser.DimAttr:
		attr.Attrs |= tcell.AttrDim
	case parser.ItalicAttr:
		attr.Attrs |= tcell.AttrItalic
	case parser.UnderlineAttr:
		attr.Attrs |= tcell.AttrUnderline
	case parser.BlinkSlowAttr, parser.BlinkFastAttr:
		attr.Attrs |= tcell.AttrBlink
	case parser.ReverseAttr:
		attr.Attrs |= tcell.AttrReverse
	case parser.HiddenAttr:
		t.sync.buf.SetHiddenCursor(true)
		return
	case parser.StrikeAttr:
		attr.Attrs |= tcell.AttrStrikeThrough
	case parser.CancelBoldAttr:
		attr.Attrs &^= tcell.AttrBold
	case parser.CancelBoldDimAttr:
		attr.Attrs &^= tcell.AttrBold
		attr.Attrs &^= tcell.AttrDim
	case parser.CancelItalicAttr:
		attr.Attrs &^= tcell.AttrItalic
	case parser.CancelUnderlineAttr:
		attr.Attrs &^= tcell.AttrUnderline
	case parser.CancelBlinkAttr:
		attr.Attrs &^= tcell.AttrBlink
	case parser.CancelReverseAttr:
		attr.Attrs &^= tcell.AttrReverse
	case parser.CancelHiddenAttr:
		t.sync.buf.SetHiddenCursor(false)
		return
	case parser.CancelStrikeAttr:
		attr.Attrs &^= tcell.AttrStrikeThrough
	case parser.ForegroundAttr:
		attr.Fg = pattr.Color
	case parser.BackgroundAttr:
		attr.Bg = pattr.Color
	case parser.DoubleUnderlineAttr, parser.UndercurlAttr,
		parser.DottedUnderlineAttr, parser.DashedUnderlineAttr,
		parser.UnderlineColorAttr:
		/* ignored */
		return
	}

	t.sync.buf.SetCursorAttributes(attr)
}

// Report private mode
func (t *parserHandler) ReportPrivateMode(mode parser.PrivateMode) {
	// no need to sync since writes only occur on pty parsing goroutine

	var modeVar *bool
	switch mode {
	case parser.PrivateModeCursorKeys:
		modeVar = &t.modeCursorKeys
	case parser.PrivateModeColumnMode:
		/* not supported for reporting */
	case parser.PrivateModeOrigin:
		modeVar = &t.modeOrigin
	case parser.PrivateModeScreen:
		/* DECSCNM not supported */
	case parser.PrivateModeLineWrap:
		modeVar = &t.modeWrap
	case parser.PrivateModeBlinkingCursor:
		modeVar = &t.modeBlinkingCursor
	case parser.PrivateModeShowCursor:
		modeVar = &t.modeShowCursor
	case parser.PrivateModeReportMouseClicks:
		modeVar = &t.modeReportMouseClicks
	case parser.PrivateModeReportCellMouseMotion:
		modeVar = &t.modeReportCellMouseMotion
	case parser.PrivateModeReportAllMouseMotion:
		modeVar = &t.modeReportAllMouseMotion
	case parser.PrivateModeReportFocusInOut:
		modeVar = &t.modeReportFocusInOut
	case parser.PrivateModeUtf8Mouse:
		modeVar = &t.modeUtf8Mouse
	case parser.PrivateModeSgrMouse:
		modeVar = &t.modeSgrMouse
	case parser.PrivateModeAlternateScroll:
		modeVar = &t.modeAlternateScroll
	case parser.PrivateModeUrgencyHints:
		modeVar = &t.modeUrgencyHints
	case parser.PrivateModeSwapScreenAndSetRestoreCursor:
		modeVar = &t.useAlt
	case parser.PrivateModeBracketedPaste:
		modeVar = &t.modeBracketedPaste
	case parser.PrivateModeSyncUpdate:
		modeVar = &t.modeSyncUpdate
	default:
		t.log(log.WarnLevel, "Set unkown private mode: %v", mode)
	}
	t.reportMode("\x1b[?%d;%d$y", int(mode), modeVar)
}

// Set private mode.
func (t *parserHandler) SetPrivateMode(mode parser.PrivateMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case parser.PrivateModeCursorKeys:
		t.modeCursorKeys = true
	case parser.PrivateModeColumnMode:
		t.deccolm()
	case parser.PrivateModeOrigin:
		t.modeOrigin = true
	case parser.PrivateModeScreen:
		/* DECSCNM not supported */
	case parser.PrivateModeLineWrap:
		t.modeWrap = true
	case parser.PrivateModeBlinkingCursor:
		t.modeBlinkingCursor = true
	case parser.PrivateModeShowCursor:
		t.modeShowCursor = true
	case parser.PrivateModeReportMouseClicks:
		t.modeReportMouseClicks = true
	case parser.PrivateModeReportCellMouseMotion:
		t.modeReportCellMouseMotion = true
	case parser.PrivateModeReportAllMouseMotion:
		t.modeReportAllMouseMotion = true
	case parser.PrivateModeReportFocusInOut:
		t.modeReportFocusInOut = true
	case parser.PrivateModeUtf8Mouse:
		t.modeUtf8Mouse = true
	case parser.PrivateModeSgrMouse:
		t.modeSgrMouse = true
	case parser.PrivateModeAlternateScroll:
		t.modeAlternateScroll = true
	case parser.PrivateModeUrgencyHints:
		t.modeUrgencyHints = true
	case parser.PrivateModeSwapScreenAndSetRestoreCursor:
		if !t.useAlt {
			t.swapAlt()
		}
	case parser.PrivateModeBracketedPaste:
		t.modeBracketedPaste = true
	case parser.PrivateModeSyncUpdate:
		t.modeSyncUpdate = true
	default:
		t.log(log.WarnLevel, "Set unkown private mode: %v", mode)
		return
	}
}

// Unset private mode.
func (t *parserHandler) UnsetPrivateMode(mode parser.PrivateMode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case parser.PrivateModeCursorKeys:
		t.modeCursorKeys = false
	case parser.PrivateModeColumnMode:
		t.deccolm()
	case parser.PrivateModeOrigin:
		t.modeOrigin = false
	case parser.PrivateModeScreen:
		/* DECSCNM not supported */
	case parser.PrivateModeLineWrap:
		t.modeWrap = false
	case parser.PrivateModeBlinkingCursor:
		t.modeBlinkingCursor = false
	case parser.PrivateModeShowCursor:
		t.modeShowCursor = false
	case parser.PrivateModeReportMouseClicks:
		t.modeReportMouseClicks = false
	case parser.PrivateModeReportCellMouseMotion:
		t.modeReportCellMouseMotion = false
	case parser.PrivateModeReportAllMouseMotion:
		t.modeReportAllMouseMotion = false
	case parser.PrivateModeReportFocusInOut:
		t.modeReportFocusInOut = false
	case parser.PrivateModeUtf8Mouse:
		t.modeUtf8Mouse = false
	case parser.PrivateModeSgrMouse:
		t.modeSgrMouse = false
	case parser.PrivateModeAlternateScroll:
		t.modeAlternateScroll = false
	case parser.PrivateModeUrgencyHints:
		t.modeUrgencyHints = false
	case parser.PrivateModeSwapScreenAndSetRestoreCursor:
		if t.useAlt {
			t.swapAlt()
		}
	case parser.PrivateModeBracketedPaste:
		t.modeBracketedPaste = false
	case parser.PrivateModeSyncUpdate:
		t.modeSyncUpdate = false
	default:
		t.log(log.WarnLevel, "Unset unkown private mode: %v", mode)
		return
	}
}

// Report mode
func (t *parserHandler) ReportMode(mode parser.Mode) {
	// no need to sync since writes only occur on pty parsing goroutine

	var modeVar *bool
	switch mode {
	case parser.ModeInsert:
		modeVar = &t.modeInsert
	case parser.ModeLineFeedNewLine:
		modeVar = &t.modeLineFeedNewLine
	default:
		t.log(log.DebugLevel, "Report unkown public mode: %v", mode)
	}

	t.reportMode("\x1b[%d;%d$y", int(mode), modeVar)
}

// Set mode.
func (t *parserHandler) SetMode(mode parser.Mode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case parser.ModeInsert:
		t.modeInsert = true
	case parser.ModeLineFeedNewLine:
		t.modeLineFeedNewLine = true
	default:
		t.log(log.WarnLevel, "Set unkown public mode: %v", mode)
		return
	}
}

// Unset mode.
func (t *parserHandler) UnsetMode(mode parser.Mode) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	switch mode {
	case parser.ModeInsert:
		t.modeInsert = false
	case parser.ModeLineFeedNewLine:
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
func (t *parserHandler) SetActiveCharset(index parser.CharsetIndex) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	t.currentCharset = index
}

// Assign a graphic character set to G0, G1, G2 or G3.
//
// 'Designate' a graphic character set as one of G0 to G3 so that it can
// later be 'invoked' by `SetActiveCharset`.
func (t *parserHandler) ConfigureCharset(
	index parser.CharsetIndex, charset parser.StandardCharset,
) {
	t.sync.mu.Lock()
	defer t.sync.mu.Unlock()

	var buf *screen.AltBuffer
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
		t.clipboard.Copy(clipboard.DefaultRegisterID, clipData)
	case int('p') | int('s'):
		t.clipboard.Copy(selectionRegisterID, clipData)
	default:
		t.log(log.WarnLevel, "unknown register ID upon ClipboardStore: %c", rune(register))
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
	var buf *screen.AltBuffer
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
		t.title = e
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
func (t *parserHandler) SetHyperlink(link *parser.Hyperlink) {
	t.log(log.WarnLevel, "unsupported call to SetHyperlink")
}

// ReportKeyboardMode reports current keyboard mode.
func (t *parserHandler) ReportKeyboardMode() {
	t.log(log.WarnLevel, "unsupported call to ReportKeyboardMode")
}

// PushKeyboardMode pushes the keyboard mode into the keyboard mode stack.
func (t *parserHandler) PushKeyboardMode(mode parser.KeyboardMode) {
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
	mode parser.KeyboardMode, behavior parser.KeyboardModesApplyBehavior,
) {
	t.log(log.WarnLevel, "unsupported call to SetKeyboardMode: "+
		"kitty keyboard handling not supported yet")
}

// SetModifyOtherKeys sets XTerm's [`ModifyOtherKeys`] option.
func (t *parserHandler) SetModifyOtherKeys(mode parser.ModifyOtherKeysMode) {
	t.log(log.WarnLevel, "unsupported call to SetModifyOtherKeys")
}

// ReportModifyOtherKeys report XTerm's [`ModifyOtherKeys`] state.
func (t *parserHandler) ReportModifyOtherKeys() {
	t.log(log.WarnLevel, "unsupported call to ReportModifyOtherKeys")
}

func (t *parserHandler) log(level log.Level, line string, params ...interface{}) {
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

	// satisfy alternate buffer semantics
	// if it's a programmatic scroll down
	defer func() {
		offset := buf.Offset().Y
		buf.ResetLines(offset, offset+rows)
	}()

	offset.Y -= rows
	if offset.Y < 0 {
		scrollDown := -offset.Y
		offset.Y = 0
		buf.SetOffset(offset)
		buf.AltBuffer.ScrollDown(0, buf.Rows(), scrollDown)
		return true
	}
	buf.SetOffset(offset)
	return true
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

func (t *parserHandler) setCursorShape(shape parser.CursorShape) {
	t.cursorHidden = shape == parser.CursorShapeHidden
	if t.cursorHidden {
		return
	}
	switch shape {
	case parser.CursorShapeBlock:
		t.cursorStyle = term.CursorStyleSteadyBlock
	case parser.CursorShapeUnderline:
		t.cursorStyle = term.CursorStyleSteadyUnderline
	case parser.CursorShapeBeam:
		t.cursorStyle = term.CursorStyleSteadyBar
	case parser.CursorShapeHollowBlock:
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

func (t *parserHandler) scrollUpPrimaryView() {
	if t.sync.buf != t.sync.primBuf {
		panic("called scroll up view on non primary buffer")
	}

	buf := t.sync.primBuf
	cells := buf.Cells.RawCells()
	for y := buf.Rows() - 1; y >= 0; y-- {
		for x := 0; x < buf.Columns(y); x++ {
			c := cells[y][x]
			if c.Ch != ' ' && c.Ch != 0 {
				newOffset := term.Coordinates{Y: y + 1}
				t.log(log.TraceLevel, "new offset after clear view %+v", newOffset)
				buf.SetOffset(newOffset)
				buf.SetCursorAtScreen(term.Coordinates{}, false)
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
