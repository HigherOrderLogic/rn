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

package modeless

import (
	"context"
	"strings"

	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/text"
)

var _ component.Scrollable = (*editorHandler)(nil)

type editorHandler struct {
	cfg              modelessConfig
	buf              *cell.Buffer
	less             handler.Less
	statusBar        statusBar
	pasteBuf         strings.Builder
	pasteStarted     bool
	resource         workspaceapi.URI
	cursor           text.Cursor
	height           int
	mouse            *text.Mouse
	clipboard        clipboard.Register
	pendingSetCursor *term.Coordinates
	lastIterateWord  term.Coordinates
	setLocations     bool
	metaK            bool
}

// NewHandler returns a modeless, simple-to-use text.Handler.
func NewHandler(
	buf *cell.Buffer, resource workspaceapi.URI,
	opts ...Option,
) text.Handler {
	ret := new(editorHandler)
	ret.Init(buf, resource, opts...)
	return ret
}

func (h *editorHandler) Init(
	buf *cell.Buffer, resource workspaceapi.URI,
	opts ...Option,
) {
	h.cfg = defaultConfig()
	for _, o := range opts {
		o(&h.cfg)
	}
	h.buf = buf
	h.resource = resource
	h.less.InitWithBuffer(buf, handler.LessConfig{
		Wrap:               h.cfg.wrap,
		NoBar:              !h.cfg.commandBar,
		SuperimposeMessage: true,
		ResAttr:            h.cfg.resAttr,
		Attributes:         h.cfg.attr,
	})
	h.less.Scroll().SetTabspaces(h.cfg.tabspaces)
	h.cursor.Init(h.less.Scroll(), h.cfg.scheduleNextTick)
	h.mouse = text.NewMouse(text.CursorMouseDelegate(&h.cursor))
	h.clipboard = h.cfg.clipboard
	h.statusBar = nopBar{}
	h.lastIterateWord.X = -1
	if h.cfg.enableInitialFolds {
		h.hideInitialFolds()
	}
}

// Resize satisfies tui.Component
func (h *editorHandler) Resize(width, height int) {
	h.height = height
	h.less.Resize(width, height)
	if h.pendingSetCursor != nil {
		h.SetCursorAtScroll(*h.pendingSetCursor)
		h.pendingSetCursor = nil
	}
}

func (h *editorHandler) setActiveLocationListMessage(locs []textapi.Location) {
	// NOTE: if there are multiple location lists with a message
	// in current cursor position, then there's no guarantee of which one
	// is going to be rendered.
	for _, loc := range locs {
		if loc.Message != "" {
			h.less.SetMessage("%s", loc.Message)
			return
		}
	}
	h.less.SetMessage("")
}

func (h *editorHandler) drawLocationMessage() {
	locs, ok := h.cursor.LocationsAtCursor()
	if ok {
		h.setActiveLocationListMessage(locs)
		h.setLocations = true
	} else if h.setLocations {
		h.less.SetMessage("")
		h.setLocations = false
	}
}

// Draw satisfies tui.Component
func (h *editorHandler) Draw(w term.Writer) {
	h.drawLocationMessage()
	h.less.Draw(w)
	text.DrawLocations(h.cursor.SortedLocations(), h.less.Scroll(), w)
}

func (h *editorHandler) handleMetaK(ev term.Event) (handled bool) {
	h.log(log.TraceLevel, "handle metak, event: %#v", ev)
	if ev.Mod != term.ModMeta {
		return
	}
	switch ev.Key {
	case term.KeyBackspace:
		if h.cursor.Select() {
			h.cursor.MoveStartLine()
			handled = h.cursor.DeleteSelection()
		}
	}
	if handled {
		return
	}
	switch ev.Ch {
	case 'u':
		handled = h.cursor.UppercaseSelection()
	case 'l':
		handled = h.cursor.LowercaseSelection()
	case 'k':
		if h.cursor.Select() {
			h.cursor.MoveEndLine()
			handled = h.cursor.DeleteSelection()
		}
	case 'j':
		handled = h.cursor.ExpandAllFolds(context.Background())
	case '1':
		handled = h.cursor.CollapseAllFolds(context.Background())
	}
	return
}

func (h *editorHandler) Handle(ev term.Event) (exit, handled bool) {
	ctx := context.Background()

	// only a user event clears a pending set cursor
	h.pendingSetCursor = nil

	switch ev.Type {
	case term.EventMouse:
		return h.mouse.Handle(ev)
	case term.EventPasteStart:
		h.pasteBuf.Reset()
		h.pasteStarted = true
		handled = true
		return
	case term.EventPasteEnd:
		str := h.pasteBuf.String()
		if _, ok := h.cursor.SelectionMode(); ok {
			handled = h.cursor.DeleteSelection()
			h.cursor.Unselect()
		} else {
			h.cursor.InsertString(str)
			handled = true
		}
		h.pasteStarted = false
		return
	case term.EventKey:
	default:
		return
	}

	if h.pasteStarted {
		if ev.Ch != 0 {
			h.pasteBuf.WriteRune(ev.Ch)
			handled = true
		}
		return
	}

	if ev.Mod == term.ModShift {
		switch ev.Key {
		case term.KeyTab:
			if _, ok := h.cursor.SelectionMode(); ok {
				h.cursor.ShiftSelectionLeft()
				h.cursor.Unselect()
			} else {
				h.cursor.ShiftLineLeft()
			}
			handled = true
			return
		}
	}

	var shift bool
	if ev.Mod&term.ModShift != 0 {
		if _, ok := h.cursor.SelectionMode(); !ok {
			h.cursor.Select()
		}
		ev.Mod = ev.Mod &^ term.ModShift
		shift = true
	}

	// if only Mod is pressed, then user might be
	// preparing to fire next key/ch.
	if h.metaK && (ev.Key != 0 || ev.Ch != 0) {
		h.metaK = false
		handled = h.handleMetaK(ev)
		if handled {
			return
		}
	}

	cursorAt := h.cursor.CursorAtScroll()
	switch ev.Mod {
	case term.ModAltShift:
		switch ev.Key {
		case term.KeyArrowDown:
			handled = h.duplicateLine(false /* down */)
		case term.KeyArrowUp:
			handled = h.duplicateLine(true /* up */)
		}
	case term.ModAltMeta:
		switch ev.Key {
		case 0:
			switch ev.Ch {
			case '[':
				handled = h.cursor.CollapseFold(context.Background())
			case ']':
				handled = h.cursor.ExpandFold(context.Background())
			}
		}
	case term.ModCtrlMeta:
		switch ev.Key {
		case term.KeyArrowDown:
			handled = h.moveLine(false /* down */)
		case term.KeyArrowUp:
			handled = h.moveLine(true /* up */)
		}
	case term.ModAlt:
		switch ev.Key {
		case term.KeyArrowDown:
			handled = h.moveLine(false /* down */)
		case term.KeyArrowUp:
			handled = h.moveLine(true /* up */)
		case term.KeyArrowLeft:
			handled = h.cursor.MoveLeftStartWord()
		case term.KeyArrowRight:
			handled = h.cursor.MoveRightEndWord()
		case term.KeyBackspace:
			h.cursor.Select()
			h.cursor.MoveRightStartWord()
			h.cursor.DeleteSelection()
		case term.KeyDelete:
			h.cursor.Select()
			h.cursor.MoveLeftStartWord()
			h.cursor.DeleteSelection()
		case 0:
			switch ev.Ch {
			case '{':
				handled = h.cursor.CollapseFold(context.Background())
			case '}':
				handled = h.cursor.ExpandFold(context.Background())
			}
		}
	case term.ModMeta:
		switch ev.Key {
		case term.KeyArrowLeft:
			handled = h.cursor.MoveStartLineNonBlank()
		case term.KeyArrowRight:
			handled = h.cursor.MoveEndLine()
		case term.KeyArrowUp:
			handled = h.cursor.MoveFirstLine()
		case term.KeyArrowDown:
			handled = h.cursor.MoveLastLine()
		case term.KeyDelete:
			if ok := h.cursor.Select(); ok {
				h.cursor.MoveEndLine()
				handled = h.cursor.DeleteSelection()
			}
		case 0:
			switch ev.Ch {
			case 'k':
				h.log(log.TraceLevel, "waiting for metaK event")
				h.metaK = true
				handled = true
			case 'j':
				handled = h.cursor.Conflate()
			case ']':
				handled = h.cursor.TryIndent()
			case '[':
				handled = h.cursor.TryIndent()
			case 'l':
				if mode, ok := h.cursor.SelectionMode(); ok && mode == text.LineSelection {
					handled = h.cursor.MoveDown()
				} else {
					handled = h.cursor.SelectLine()
				}
				// avoid unselect due to not shift
				return
			case 'D':
				handled = h.duplicateLine(false /* down */)
			case 'd':
				handled = h.selectNextWordAtCursor()
				return
			case 'x':
				if _, ok := h.cursor.SelectionMode(); !ok {
					h.cursor.SelectLine()
				}
				_, err := h.cursor.CopySelectionNoUnselect(
					clipboard.DefaultRegisterID, h.clipboard)
				if err != nil {
					h.log(log.ErrorLevel, "cursor copy selection: %v", err)
				} else {
					handled = h.cursor.DeleteSelection()
				}
			case 'f':
				ev.Key = 0
				ev.Ch = '/'
				_, handled = h.less.Handle(ev)
			case 'a':
				if _, ok := h.cursor.SelectionMode(); ok {
					h.cursor.Unselect()
				}
				h.cursor.MoveFirstLine()
				if h.cursor.SelectLine() {
					h.cursor.MoveLastLine()
					h.cursor.MoveRight()
					handled = true
				}
				return
			// the following two are defined here in case we're not capturing them at the command level
			case 'c':
				_, err := h.cursor.CopySelection(clipboard.DefaultRegisterID, h.clipboard)
				if err != nil {
					h.log(log.ErrorLevel, "cursor copy selection: %v", err)
				} else {
					handled = true
				}
			case 'v':
				paste, err := h.clipboard.Paste(clipboard.DefaultRegisterID)
				if err != nil {
					h.log(log.ErrorLevel, "clipboard paste: %v", err)
				} else {
					str := paste.Text
					mode, _ := paste.Metadata.(text.SelectMode)
					h.cursor.Paste(str, mode, false)
					handled = true
				}
			case 'z':
				handled = h.cursor.Undo()
			case 'Z':
				handled = h.cursor.Redo()
			case 'K':
				if h.cursor.SelectLine() {
					handled = h.cursor.DeleteSelection()
				}
			}
		}
	// no modifier
	case 0:
		switch ev.Key {
		case term.KeyEsc:
			handled = h.cursor.Unselect()
			h.cursor.Search("")
		case term.KeyEnd:
			handled = h.cursor.MoveEndLine()
		case term.KeyHome:
			handled = h.cursor.MoveStartLine()
		case term.KeyPgup:
			handled = h.cursor.MoveUpLines(h.less.Scroll().SizeHeight())
			if handled {
				h.cursor.RepositionBottom()
			}
		case term.KeyPgdn:
			handled = h.cursor.MoveDownLines(h.less.Scroll().SizeHeight())
			if handled {
				h.cursor.RepositionTop()
			}
		case term.KeyArrowLeft:
			handled = h.cursor.MoveLeft()
		case term.KeyArrowRight:
			handled = h.cursor.MoveRight()
		case term.KeyArrowUp:
			handled = h.cursor.MoveUp()
		case term.KeyArrowDown:
			handled = h.cursor.MoveDown()
		case term.KeyEnter:
			if _, ok := h.cursor.SelectionMode(); ok {
				h.cursor.DeleteSelection()
				h.cursor.Unselect()
			}
			h.cursor.Insert('\n')
			handled = true
		case term.KeySpace:
			if _, ok := h.cursor.SelectionMode(); ok {
				h.cursor.DeleteSelection()
				h.cursor.Unselect()
			}
			h.cursor.Insert(' ')
			handled = true
		case term.KeyTab:
			if _, ok := h.cursor.SelectionMode(); ok {
				h.cursor.ShiftSelectionRight()
				h.cursor.Unselect()
			} else {
				h.cursor.Insert('\t')
			}
			handled = true
		case term.KeyBackspace:
			if _, ok := h.cursor.SelectionMode(); ok {
				handled = h.cursor.DeleteSelection()
				h.cursor.Unselect()
			} else {
				handled = h.cursor.Backspace()
			}
		case term.KeyDelete:
			if _, ok := h.cursor.SelectionMode(); ok {
				handled = h.cursor.DeleteSelection()
				h.cursor.Unselect()
			} else {
				handled = h.cursor.Delete()
			}
		default:
			if ev.Ch != 0 {
				if _, ok := h.cursor.SelectionMode(); ok {
					handled = h.cursor.DeleteSelection()
					h.cursor.Unselect()
				} else {
					h.cursor.Insert(ev.Ch)
					handled = true
				}
			}
		}
	case term.ModCtrl:
		switch ev.Ch {
		case 'y', 'c':
			_, err := h.cursor.CopySelection(clipboard.DefaultRegisterID, h.clipboard)
			if err != nil {
				h.log(log.ErrorLevel, "cursor copy selection: %v", err)
			}
			handled = true
		case 'l':
			handled = h.cursor.Center()
		case 'v':
			handled = h.less.Scroll().SeekDownPage()
		case 'm':
			handled = h.cursor.MoveToMatchingRune()
		case 'd':
			handled = h.cursor.Delete()
		case 'h':
			handled = h.cursor.Backspace()
		case 'a':
			handled = h.cursor.MoveStartLine()
		case 'p':
			handled = h.cursor.MoveUp()
		case 'n':
			handled = h.cursor.MoveDown()
		case 'e':
			handled = h.cursor.MoveEndLine()
		case 'z':
			handled = h.cursor.Undo()
		case 'Z':
			handled = h.cursor.ToggleFold(ctx)
		case 'A':
			handled = h.cursor.ToggleAllFolds(ctx)
		case 'f':
			handled = h.cursor.MoveRight()
		case 'b':
			handled = h.cursor.MoveLeft()
		case 'k':
			h.cursor.Select()
			h.cursor.MoveEndLine()
			handled = h.cursor.DeleteSelection()
		case '-':
			handled = h.cursor.MoveToNextMatch()
		case '_':
			handled = h.cursor.MoveToPrevMatch()
		case 't':
			if h.cursor.Select() {
				handled, _ = h.cursor.CopySelectionNoUnselect(
					clipboard.DefaultRegisterID, h.clipboard)
				if handled {
					h.cursor.DeleteSelection()
					paste, err := h.clipboard.Paste(clipboard.DefaultRegisterID)
					if err != nil {
						h.log(log.ErrorLevel, "clipboard paste: %v", err)
					} else {
						str := paste.Text
						h.cursor.Paste(str, text.StandardSelection, false)
					}
				}
			}
		case 'K':
			if h.cursor.SelectLine() {
				handled = h.cursor.DeleteSelection()
			}
		}
	case term.ModCtrlAlt:
		switch ev.Key {
		case term.KeyArrowUp:
			pos := h.cursor.CursorAtScroll()
			if handled = h.less.Scroll().SeekUp(); handled {
				win, ok := h.cursor.WindowCoordinates(pos)
				if ok && win.Y >= 0 {
					h.cursor.MoveToScroll(pos)
				}
			}
		case term.KeyArrowDown:
			pos := h.cursor.CursorAtScroll()
			if handled = h.less.Scroll().SeekDown(); handled {
				win, ok := h.cursor.WindowCoordinates(pos)
				if ok && win.Y >= 0 {
					h.cursor.MoveToScroll(pos)
				}
			}
		case 0:
			switch ev.Ch {
			case 'h':
				handled = h.cursor.HideSelection()
				h.cursor.Unselect()
			case 'v':
				handled = h.cursor.Unhide()
				h.cursor.Unselect()
			}
		}
	}

	// if moved and shift is not pressed
	if cursorAt != h.cursor.CursorAtScroll() && !shift {
		h.cursor.Unselect()
		h.cursor.Search("")
	}
	return
}

// Cursor satisfies tui.Handler
func (h *editorHandler) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	return h.cursor.Coordinates(), term.CursorStyleSteadyBar, true
}

// Selection satisfies tui.Handler.
func (h *editorHandler) Selection() (string, bool) {
	text := h.cursor.Selection()
	return text, text != ""
}

// Close satisfies editor.Handler.
func (h *editorHandler) Close() error {
	return nil
}

// Resource satisfies editor.Handler.
func (h *editorHandler) Resource() workspaceapi.URI {
	return h.resource
}

// SetWrap satisfies editor.Handler.
func (h *editorHandler) SetWrap(wrap bool) {
	h.less.Scroll().Wrap = wrap
}
func (t *editorHandler) ShowCommandBar(show bool) {
	t.less.ShowCommandBar(show)
}

func (h *editorHandler) SetCursorAtScroll(pos term.Coordinates) bool {
	// setCursor should be robust against resizes, etc.
	// only the first client interaction should clear this position
	if h.less.Scroll().Width() == 0 || h.less.Scroll().SizeHeight() == 0 {
		h.pendingSetCursor = new(term.Coordinates)
		*h.pendingSetCursor = pos
		return false
	}

	_, ok := h.cursor.MoveToScroll(pos)
	if !ok || !h.cfg.autoCenter {
		return ok
	}
	h.cursor.Center()
	return true
}

func (h *editorHandler) SeekUp() bool {
	return h.less.Scroll().SeekUp()
}

func (h *editorHandler) SeekDown() bool {
	return h.less.Scroll().SeekDown()
}

func (h *editorHandler) SeekOffset() int {
	return h.less.Scroll().SeekOffset()
}

func (h *editorHandler) MaxSeekOffset() int {
	return h.less.Scroll().MaxSeekOffset()
}

func (h editorHandler) SetLocationList(
	pri textapi.LocationPriority, ID string, loc text.LocationList,
) {
	h.cursor.SetLocationList(pri, ID, loc)
}

func (h *editorHandler) MoveToNextLocation(ID string) bool {
	return h.cursor.MoveToNextLocation(ID)
}

func (h *editorHandler) MoveToPrevLocation(ID string) bool {
	return h.cursor.MoveToPrevLocation(ID)
}

func (h *editorHandler) CellView() cell.View {
	return h.buf.View()
}

func (h *editorHandler) CellEditor() cell.Editor {
	return text.ExternalEditor(&h.cursor, h.buf.Editor())
}

func (e *editorHandler) SetDefaultAttributes(attr term.Attributes) {
	e.less.Scroll().Attributes = attr
}

func (h *editorHandler) CursorAtScroll() term.Coordinates {
	return h.cursor.CursorAtScroll()
}

func (h *editorHandler) LocationLists() []text.LocationSet {
	return h.cursor.LocationLists()
}

func (h *editorHandler) moveLine(up bool) (handled bool) {
	if _, ok := h.cursor.SelectionMode(); !ok {
		if !h.cursor.SelectLine() {
			return
		}
	}
	handled, _ = h.cursor.CopySelectionNoUnselect(
		clipboard.DefaultRegisterID, h.clipboard)
	if !handled {
		return
	}
	h.cursor.DeleteSelection()
	if up {
		h.cursor.MoveUp()
	} else {
		h.cursor.MoveDown()
	}
	h.cursor.MoveStartLine()
	paste, err := h.clipboard.Paste(clipboard.DefaultRegisterID)
	if err != nil {
		h.log(log.ErrorLevel, "clipboard paste: %v", err)
	} else {
		str := paste.Text
		curr := h.cursor.CursorAtScroll()
		h.cursor.Paste(str, text.StandardSelection, false)
		if up {
			curr.Y--
		}
		h.cursor.MoveToScroll(curr)
	}
	return
}

func (h *editorHandler) duplicateLine(up bool) (handled bool) {
	if _, ok := h.cursor.SelectionMode(); !ok {
		if !h.cursor.SelectLine() {
			return
		}
	}
	handled, _ = h.cursor.CopySelection(
		clipboard.DefaultRegisterID, h.clipboard)
	if !handled {
		return
	}
	if !up {
		h.cursor.MoveDown()
	}
	h.cursor.MoveStartLine()
	paste, err := h.clipboard.Paste(clipboard.DefaultRegisterID)
	if err != nil {
		h.log(log.ErrorLevel, "clipboard paste: %v", err)
	} else {
		str := paste.Text
		curr := h.cursor.CursorAtScroll()
		h.cursor.Paste(str, text.StandardSelection, false)
		h.cursor.MoveToScroll(curr)
	}
	return
}

func (h *editorHandler) setStatusBar(bar statusBar) {
	h.statusBar = bar
}

func (h *editorHandler) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "modeless.handler").Logf(level, msg, args...)
}

var _ foldsService = (*syntax.Tree)(nil)

type foldsService interface {
	FoldsFrom(pos term.Coordinates) (iterator.Iterator[term.Range], bool)
	Folds() (iterator.Iterator[term.Range], bool)
	InitialFolds() (iterator.Iterator[term.Range], bool)
}

func (h *editorHandler) hideInitialFolds() {
	svc, ok := h.less.Buffer().View().(foldsService)
	if !ok {
		h.log(log.DebugLevel, "folds service not available for resource: %s", h.resource)
		return
	}
	folds, ok := svc.InitialFolds()
	if !ok {
		h.log(log.DebugLevel, "initial folds returned false")
		return
	}

	scroll := h.less.Scroll()

	go debug.CapturePanicReport(func() {
		folds, isEmpty := iterator.IsEmpty(context.Background(), folds)
		if isEmpty {
			folds.Close()
			return
		}
		h.cfg.scheduleNextTick(func() {
			defer folds.Close()
			cursor := h.CursorAtScroll()
			for {
				fold, ok := folds.Next(context.Background())
				if !ok {
					break
				}
				scroll.MarkHidden(fold.Start.Y, fold.End.Y)
			}
			if err := folds.Err(); err != nil {
				h.log(log.ErrorLevel, "error hiding initial folds: %v", err)
			}
			h.SetCursorAtScroll(cursor)
		})
	})
}

func (h *editorHandler) selectNextWordAtCursor() bool {
	h.cursor.Unselect()
	if h.cursor.IsEndWord() && !h.cursor.IsStartWord() {
		h.cursor.MoveLeft()
		word := h.cursor.Word()
		h.cursor.SearchWord(word)
		h.cursor.MoveToNextMatch()
	} else {
		word := h.cursor.Word()
		h.cursor.SearchWord(word)
	}
	if !h.cursor.IsStartWord() {
		h.cursor.MoveLeftStartWordNoWrap()
	}
	h.cursor.Select()
	h.cursor.MoveRightEndWordNoWrap()
	return true
}
