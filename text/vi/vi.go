package vi

import (
	"fmt"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

var _ tui.Handler = (*Vi)(nil)

type snapshot struct {
	content string
	cursor  term.Coordinates
}

func (s snapshot) String() string {
	return fmt.Sprintf("Snapshot{%q:%v}", s.content, s.cursor)
}

// Vi implements a basic vi-like text editor which satisfies tui.Handler
type Vi struct {
	resource  workspace.URI
	handler   viHandler
	buf       *cell.Buffer
	cursor    *text.Cursor
	mouse     *text.Mouse
	logger    *log.Logger
	messenger text.Messenger
	less      *handler.Less
	clipboard text.Clipboard

	repeating    bool
	currEdited   bool
	evEdited     bool
	currSnapshot snapshot
	currEdits    []term.Event
	repeatEdits  []term.Event

	resetting    bool
	undoTimeline []snapshot
	redoTimeline []snapshot
}

type viSubscriber Vi

// New allocates storage for a new Vi handler, initializes it and returns it.
func New(buf *cell.Buffer, resource workspace.URI, opts ...Option) *Vi {
	vi := new(Vi)
	vi.Init(buf, resource, opts...)
	return vi
}

// Init initialies this vi handle with a new Buffer.
func (vi *Vi) Init(buf *cell.Buffer, resource workspace.URI, opts ...Option) {
	vi.resource = resource

	viHandler := new(viHandlerImpl)
	viHandler.init(buf, opts...)
	vi.handler = viHandler
	vi.buf = buf
	vi.logger = viHandler.config.logger
	vi.messenger = viHandler.config.messenger
	vi.less = &viHandler.less
	vi.cursor = &viHandler.cursor
	vi.mouse = text.NewMouse(newMouseDelegate(viHandler))
	vi.clipboard = viHandler.config.clipboard

	vi.repeatEdits = make([]term.Event, 0)
	vi.currEdits = make([]term.Event, 0)
	vi.undoTimeline = make([]snapshot, 0)
	vi.redoTimeline = make([]snapshot, 0)

	vi.buf.Subscribe((*viSubscriber)(vi))
	vi.snapshotContent()

	text.WithCopyDelete(viHandler.config.defaultRegister,
		vi, vi.cursor, buf)
}

// Cursor satisfies tui.Handler
func (vi *Vi) Cursor() (term.Coordinates, bool) {
	return vi.handler.Cursor()
}

// Draw satisfies tui.Component
func (vi *Vi) Draw(w term.Writer) {
	vi.handler.Draw(w)
}

func isEditMode(mode viMode) bool {
	switch mode {
	case normalMode, gMode, yankMode, searchMode,
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

func (vi *Vi) pushNewSnapshot() {
	vi.pushUndo(vi.currSnapshot)
}

func (vi *viSubscriber) OnWillEdit(from, to term.Coordinates, str string) {
	if !vi.currEdited && !vi.resetting {
		pubVi := (*Vi)(vi)
		pubVi.currSnapshot.cursor = from
		pubVi.pushNewSnapshot()
		pubVi.resetRedoTimeline()
	}
}

func (vi *viSubscriber) OnDidEdit(start, end term.Coordinates, old string) {
	if !vi.resetting && !vi.repeating {
		vi.evEdited = true
		vi.currEdited = true
	}
}

// Paste satisfies text.Clipboard. See Copy.
func (vi *Vi) Paste(registerID string) (text.ClipboardData, error) {
	return vi.clipboard.Paste(registerID)
}

// Copy satisfies text.Clipboard to make sure undo/redo deletes
// are not being copied to the clipboard or backspace deletes within insert.
func (vi *Vi) Copy(registerID string, data text.ClipboardData) error {
	if vi.resetting || vi.handler.mode() == insertMode {
		return nil
	}
	return vi.clipboard.Copy(registerID, data)
}

// Handle satisfies tui.Handler
func (vi *Vi) Handle(ev term.Event) (quit, handled bool) {
	if ev.Type == term.EventMouse {
		return vi.mouse.Handle(ev)
	}

	switch vi.handler.mode() {
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
			}
			switch ev.Key {
			case term.KeyCtrlR:
				handled = vi.redo()
				return
			}
		}
	}

	vi.evEdited = false
	prevMode := vi.handler.mode()
	quit, handled = vi.handler.Handle(ev)
	nextMode := vi.handler.mode()

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
		if vi.currEdited || vi.repeating {
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

// Man satisfies tui.Handler
func (vi *Vi) Man() tui.Manual {
	return vi.handler.Man()
}

// Resize satisfies tui.Component
func (vi *Vi) Resize(width, height int) {
	vi.handler.Resize(width, height)
}

// SetMessage uses vi's configured Messenger to set msg with args.
func (vi *Vi) SetMessage(msg string, args ...interface{}) {
	if vi.logger != nil {
		vi.logger.Debugf(msg, args...)
	}

	if vi.messenger != nil {
		vi.messenger.SetMessage(msg, args...)
		return
	}
	vi.less.SetMessage(msg, args...)
}

// MoveToNextLocation moves the cursor to the next location
// in the location list identified by ID.
func (vi *Vi) MoveToNextLocation(ID string) {
	vi.handler.moveToNextLocation(ID)
}

// MoveToPrevLocation moves the cursor to the previous location
// in the location list identified by ID.
func (vi *Vi) MoveToPrevLocation(ID string) {
	vi.handler.moveToPrevLocation(ID)
}

// SetLocationList sets a location list of this handler. See Cursor.SetLocationList
func (vi *Vi) SetLocationList(ID string, l text.LocationList) {
	vi.handler.setLocationList(ID, l)
}

// SetCursorAtScroll sets the cursor of this Vi handler at content pos.
func (vi *Vi) SetCursorAtScroll(pos term.Coordinates) bool {
	return vi.handler.setCursorAtScroll(pos)
}

// CursorAtScroll sets the cursor of this Vi handler at content pos.
func (vi *Vi) CursorAtScroll() term.Coordinates {
	return vi.handler.cursorAtScroll()
}

// SubscribeScroll subscribe sub to scroll events.
func (vi *Vi) SubscribeScroll(sub component.ScrollSubscriber) {
	vi.handler.subscribeScroll(sub)
}

// CellView returns the underlying cell.View.
func (vi *Vi) CellView() cell.View {
	return vi.buf.View()
}

// CellEditor returns the underlying cell.Editor.
func (vi *Vi) CellEditor() cell.Editor {
	return vi.buf.Editor()
}

// Resource satisfies editor.Handler.
func (vi *Vi) Resource() workspace.URI {
	return vi.resource
}

// Close satisfies editor.Handler.
func (vi *Vi) Close() error {
	return nil
}

func (vi *Vi) repeat() (handled bool) {
	vi.repeating = true
	for _, ev := range vi.repeatEdits {
		handled = true
		vi.Handle(ev)
	}
	vi.repeating = false
	if handled {
		vi.handler.moveToBounds()
	}
	return
}

func popSnapshot(timeline []snapshot) ([]snapshot, snapshot, bool) {
	lastCmd := len(timeline) - 1
	if lastCmd < 0 {
		return timeline, snapshot{}, false
	}
	snap := timeline[lastCmd]
	return timeline[:lastCmd], snap, true
}

func (vi *Vi) redo() bool {
	redoTimeline, snapshot, ok := popSnapshot(vi.redoTimeline)
	if !ok {
		return false
	}
	vi.redoTimeline = redoTimeline

	current := vi.currSnapshot
	vi.resetToSnapshot(snapshot)
	vi.pushUndo(current)
	vi.handler.moveToBounds()
	return ok
}

func (vi *Vi) undo() bool {
	undoTimeline, snapshot, ok := popSnapshot(vi.undoTimeline)
	if !ok {
		return false
	}
	vi.undoTimeline = undoTimeline

	current := vi.currSnapshot
	vi.resetToSnapshot(snapshot)
	vi.pushRedo(current)
	vi.handler.moveToBounds()
	return ok
}

func (vi *Vi) resetToSnapshot(s snapshot) {
	from, to := term.Coordinates{}, term.Coordinates{Y: vi.buf.Rows()}
	vi.resetting = true
	vi.buf.Edit(from, to, s.content)
	vi.handler.setCursorAtScroll(s.cursor)
	vi.currSnapshot = s
	vi.resetting = false
}

func (vi *Vi) snapshotContent() {
	vi.currSnapshot.content = vi.buf.String()
}
func (vi *Vi) pushUndo(content snapshot) {
	// TODO pop last op if exceed mem limit
	vi.undoTimeline = append(vi.undoTimeline, content)
}

func (vi *Vi) pushRedo(content snapshot) {
	vi.redoTimeline = append(vi.redoTimeline, content)
}

func (vi *Vi) resetRedoTimeline() {
	vi.redoTimeline = vi.redoTimeline[:0]
}
