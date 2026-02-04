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
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/text"
)

var _ text.Handler = (*Vi)(nil)

// Vi implements a basic vi-like text editor which satisfies tui.Handler
type Vi struct {
	resource  workspaceapi.URI
	handler   viHandler
	buf       *cell.Buffer
	cursor    *text.Cursor
	mouse     *text.Mouse
	less      *handler.Less
	clipboard clipboard.Register
	config    viConfig

	scheduleNextTick func(func()) bool

	repeating   int
	currEdited  bool
	evEdited    bool
	oob         bool // out-of-band edits (i.e. via CellEditor)
	oobEdited   bool
	currEdits   []term.Event
	repeatEdits []term.Event

	resetting bool
}

// New allocates storage for a new Vi handler, initializes it and returns it.
func New(buf *cell.Buffer, resource workspaceapi.URI, opts ...Option) *Vi {
	vi := new(Vi)
	vi.Init(buf, resource, opts...)
	return vi
}

// Init initialies this vi handle with the given cell.Buffer.
func (vi *Vi) Init(buf *cell.Buffer, resource workspaceapi.URI, opts ...Option) {
	vi.config = defaultviHandlerImplConfig()
	for _, o := range opts {
		o(&vi.config)
	}
	viHandler := new(viHandlerImpl)
	viHandler.init(buf, vi.config)

	vi.init(viHandler, buf, resource)

	text.WithCopyDelete(viHandler.config.defaultRegister,
		vi, vi.cursor, buf)
	vi.buf.Subscribe((*cellSubscriber)(vi))
}

// InitWithScroll initialies this vi handle with the given component.Scroll
// and its cell.Buffer. This does not initialize this Vi implementation with copy
// deletes to clipboard or undo/redo because we don't know if the given Scroll was initialized with
// Subscribe functionality or not. Init should be used in favor of this method for standard
// usage of Vi.
func (vi *Vi) InitWithScroll(scroll *component.Scroll, resource workspaceapi.URI, opts ...Option) {
	viHandler := new(viHandlerImpl)
	viHandler.initWithScroll(scroll, opts...)

	vi.init(viHandler, scroll.Buffer(), resource)
}

func (vi *Vi) init(
	viHandler *viHandlerImpl, buf *cell.Buffer,
	resource workspaceapi.URI,
) {
	vi.resource = resource
	vi.handler = viHandler
	vi.buf = buf
	vi.less = &viHandler.less
	vi.cursor = &viHandler.cursor
	vi.mouse = text.NewMouse(newMouseDelegate(viHandler))
	vi.clipboard = viHandler.config.clipboard
	vi.scheduleNextTick = viHandler.config.scheduleNextTick

	vi.repeatEdits = make([]term.Event, 0)
	vi.currEdits = make([]term.Event, 0)
	vi.oob = true

	vi.snapshotContent()
	if viHandler.config.enableInitialFolds {
		vi.hideInitialFolds()
	}
}

// Selection returns the text currently selected by Vi's visual mode,
// or false if there's no text selected.
func (vi *Vi) Selection() (string, bool) {
	return vi.handler.Selection()
}

// Cursor satisfies tui.Handler
func (vi *Vi) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return vi.handler.Cursor()
}

// Draw satisfies tui.Component
func (vi *Vi) Draw(w term.Writer) {
	vi.handler.Draw(w)
}

func isEditMode(mode viMode) bool {
	switch mode {
	case normalMode, zMode, gMode, yankMode, searchMode,
		visualMode, visualLineMode, visualBlockMode:
		return false
	case insertMode, deleteMode, replaceMode, replaceOneMode:
		return true
	default:
		panic(fmt.Sprintf("unknown vi mode: %v", mode))
	}
}

func isSelectMode(mode viMode) bool {
	return mode == visualMode || mode == visualLineMode || mode == visualBlockMode
}

func (vi *Vi) resetEdits() {
	vi.currEdits = vi.currEdits[:0]
	vi.currEdited = false
}

func (vi *Vi) appendLastEdit(ev term.Event) {
	vi.currEdits = append(vi.currEdits, ev)
}

func (vi *Vi) copyRepeat() {
	if !vi.currEdited {
		return
	}
	vi.repeatEdits = vi.repeatEdits[:0]
	vi.repeatEdits = append(vi.repeatEdits, vi.currEdits...)
}

// Paste satisfies text.Clipboard. See Copy.
func (vi *Vi) Paste(registerID string) (clipboard.Data, error) {
	return vi.clipboard.Paste(registerID)
}

// Copy satisfies text.Clipboard to make sure undo/redo deletes
// are not being copied to the clipboard or backspace deletes within insert.
func (vi *Vi) Copy(registerID string, data clipboard.Data) error {
	if vi.resetting || vi.handler.mode() == insertMode {
		return nil
	}
	return vi.clipboard.Copy(registerID, data)
}

// Handle satisfies tui.Handler
func (vi *Vi) Handle(ev term.Event) (quit, handled bool) {
	mode := vi.handler.mode()

	if ev.Type == term.EventMouse && mode != insertMode {
		return vi.mouse.Handle(ev)
	}

	oobEdited := vi.oobEdited
	if oobEdited {
		vi.snapshotContent()
		vi.resetEdits()
		vi.oobEdited = false
	}

	if vi.repeating == 0 {
		switch mode {
		case normalMode:
			switch ev.Type {
			case term.EventKey:
				switch ev.Ch {
				case '.':
					handled = vi.repeat()
					return
				case 'u':
					handled = vi.undo()
					return
				case 'r':
					if ev.Mod == term.ModCtrl {
						handled = vi.redo()
						return
					}
				}
			}
		}
	}

	vi.evEdited = false
	vi.oob = false
	prevMode := vi.handler.mode()
	quit, handled = vi.handler.Handle(ev)
	nextMode := vi.handler.mode()
	vi.oob = true

	if oobEdited {
		return
	}

	if prevMode == nextMode {
		if !isEditMode(prevMode) && vi.evEdited {
			vi.appendLastEdit(ev)
			vi.copyRepeat()
			vi.snapshotContent()
			vi.resetEdits()
		} else if isSelectMode(prevMode) || isEditMode(prevMode) {
			vi.appendLastEdit(ev)
		}
		return
	}

	if !isEditMode(prevMode) && isEditMode(nextMode) {
		vi.appendLastEdit(ev)
	} else if isEditMode(prevMode) && !isEditMode(nextMode) {
		vi.appendLastEdit(ev)
		vi.copyRepeat()
		vi.snapshotContent()
		vi.resetEdits()
	} else if isEditMode(prevMode) && isEditMode(nextMode) {
		vi.appendLastEdit(ev)
	} else {
		vi.appendLastEdit(ev)
		if vi.currEdited {
			vi.copyRepeat()
			vi.snapshotContent()
			vi.resetEdits()
		} else if !isSelectMode(nextMode) {
			// reset always if going back to normal
			vi.resetEdits()
		}
	}
	return quit, handled
}

// Resize satisfies tui.Component
func (vi *Vi) Resize(width, height int) {
	vi.handler.Resize(width, height)
}

// MoveToNextLocation moves the cursor to the next location
// in the location list identified by ID.
func (vi *Vi) MoveToNextLocation(ID string) bool {
	return vi.handler.moveToNextLocation(ID)
}

// MoveToPrevLocation moves the cursor to the previous location
// in the location list identified by ID.
func (vi *Vi) MoveToPrevLocation(ID string) bool {
	return vi.handler.moveToPrevLocation(ID)
}

// SetLocationList sets a location list of this handler. See Cursor.SetLocationList
func (vi *Vi) SetLocationList(
	pri textapi.LocationPriority, ID string, l text.LocationList,
) {
	vi.handler.setLocationList(pri, ID, l)
}

// SetCursorAtScroll sets the cursor of this Vi handler at content pos.
func (vi *Vi) SetCursorAtScroll(pos term.Coordinates) bool {
	ok := vi.handler.setCursorAtScroll(pos)
	if !ok || !vi.config.autoCenter {
		return ok
	}
	if !vi.cursor.Center() {
		return true
	}
	// after calling center, the free cursor (used for repositioning
	// after out of bounds repositioning when moving up/down),
	// needs to be reset
	if vh, ok := vi.handler.(*viHandlerImpl); ok {
		vh.anchor = vi.handler.cursorAtScroll()
	}
	return true
}

// CursorAtScroll sets the cursor of this Vi handler at content pos.
func (vi *Vi) CursorAtScroll() term.Coordinates {
	return vi.handler.cursorAtScroll()
}

// CellView returns the underlying cell.View.
func (vi *Vi) CellView() cell.View {
	return vi.buf.View()
}

// CellEditor returns the underlying cell.Editor.
func (vi *Vi) CellEditor() cell.Editor {
	return text.ExternalEditor(vi.cursor, vi.buf.Editor())
}

// Resource satisfies editor.Handler.
func (vi *Vi) Resource() workspaceapi.URI {
	return vi.resource
}

// SetWrap satisfies editor.Handler.
func (vi *Vi) SetWrap(wrap bool) {
	vi.less.Scroll().Wrap = wrap
}

// ShowCommandBar satisfies editor.Handler.
func (vi *Vi) ShowCommandBar(show bool) {
	/* handled by StatusBar */
}

// SetMessage sets a message on the status bar.
func (vi *Vi) SetMessage(msg string) {
	/* handled by StatusBar */
}

// IsEditMode returns whether the current mode is one of
// the edit modes: insert, delete or replace.
func (vi *Vi) IsEditMode() bool {
	return isEditMode(vi.handler.mode())
}

// IsSearchMode returns whether the current mode is the search mode.
func (vi *Vi) IsSearchMode() bool {
	return vi.handler.mode() == searchMode
}

// Search runs a text search on the underlying scroll content.
func (vi *Vi) Search(target string) {
	vi.handler.search(target)
}

// SetNormalMode switches the mode to normal.
// It returns false if the current mode was already normal mode.
func (vi *Vi) SetNormalMode() bool {
	return vi.handler.setNormalMode()
}

// Unselect unselects any text that's been previously selected.
func (vi *Vi) Unselect() bool {
	return vi.handler.unselect()
}

// SetDefaultAttributes sets the underlying's Scroll's default Attributes.
func (vi *Vi) SetDefaultAttributes(attrs term.Attributes) {
	vi.less.Scroll().Attributes = attrs
}

// Close satisfies editor.Handler.
func (vi *Vi) Close() error {
	return nil
}

// SeekUp satisfies component.Scrollable.
func (vi *Vi) SeekUp() bool {
	return vi.less.Scroll().SeekUp()
}

// SeekDown satisfies component.Scrollable.
func (vi *Vi) SeekDown() bool {
	return vi.less.Scroll().SeekDown()
}

// SeekOffset satisfies component.Scrollable.
func (vi *Vi) SeekOffset() int {
	return vi.less.Scroll().SeekOffset()
}

// MaxSeekOffset satisfies component.Scrollable.
func (vi *Vi) MaxSeekOffset() int {
	return vi.less.Scroll().MaxSeekOffset()
}

// LocationLists satisfies text.Handler.
func (h *Vi) LocationLists() []text.LocationSet {
	return h.cursor.LocationLists()
}

func (vi *Vi) repeat() (handled bool) {
	vi.repeating++
	for _, ev := range vi.repeatEdits {
		handled = true
		vi.Handle(ev)
	}
	if handled {
		vi.handler.moveToBounds()
	}
	vi.repeating--
	return
}

func (vi *Vi) redo() bool {
	if vi.resetToSnapshot(vi.buf.Redo) {
		vi.handler.moveToBounds()
		return true
	}
	return false
}

func (vi *Vi) undo() bool {
	if vi.resetToSnapshot(vi.buf.Undo) {
		vi.handler.moveToBounds()
		return true
	}
	return false
}

func (vi *Vi) resetToSnapshot(op func() (bool, term.Coordinates)) bool {
	vi.resetting = true
	ok, at := op()
	if ok {
		vi.handler.setCursorAtScroll(at)
	}
	vi.resetting = false
	return ok
}

func (vi *Vi) setStatusBar(bar statusBar) {
	vi.handler.setStatusBar(bar)
}

func (vi *Vi) snapshotContent() {
	vi.buf.GroupUndo()
}

type cellSubscriber = Vi

// OnWillEdit satisfies cell.Subscriber.
func (vi *cellSubscriber) OnWillEdit(
	ctx context.Context, from, to term.Coordinates, str string,
) {
	if (!vi.oob && vi.oobEdited) || (!vi.currEdited && !vi.resetting) {
		vi.buf.MarkStartUndo()
	}
}

// OnDidEdit satisfies cell.Subscriber.
func (vi *cellSubscriber) OnDidEdit(
	ctx context.Context, start, end term.Coordinates, old string,
) {
	if !vi.resetting {
		vi.evEdited = true
		vi.currEdited = true
	}
	vi.oobEdited = vi.oob
}

var _ foldsService = (*syntax.Tree)(nil)

type foldsService interface {
	FoldsFrom(pos term.Coordinates) (iterator.Iterator[term.Range], bool)
	Folds() (iterator.Iterator[term.Range], bool)
	InitialFolds() (iterator.Iterator[term.Range], bool)
}

func (vi *Vi) hideInitialFolds() {
	svc, ok := vi.less.Buffer().View().(foldsService)
	if !ok {
		vi.log(log.DebugLevel, "folds service not available for resource: %s", vi.resource)
		return
	}
	folds, ok := svc.InitialFolds()
	if !ok {
		vi.log(log.DebugLevel, "initial folds returned false")
		return
	}

	scroll := vi.less.Scroll()

	go debug.CapturePanicReport(func() {
		folds, isEmpty := iterator.IsEmpty(context.Background(), folds)
		if isEmpty {
			folds.Close()
			return
		}
		vi.scheduleNextTick(func() {
			defer folds.Close()
			cursor := vi.handler.cursorAtScroll()
			for {
				fold, ok := folds.Next(context.Background())
				if !ok {
					break
				}
				scroll.MarkHidden(fold.Start.Y, fold.End.Y)
			}
			if err := folds.Err(); err != nil {
				vi.log(log.ErrorLevel, "error hiding initial folds: %v", err)
			}
			vi.handler.setCursorAtScroll(cursor)
		})
	})
}

func (e *Vi) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "vi.Vi").Logf(level, msg, args...)
}
