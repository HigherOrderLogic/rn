package vi

import (
	"context"
	"fmt"

	"unstable.build/go-tui"
	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
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
	resource  workspaceapi.URI
	handler   viHandler
	buf       *cell.Buffer
	cursor    *text.Cursor
	mouse     *text.Mouse
	less      *handler.Less
	clipboard clipboard.Register

	repeating    int
	currEdited   bool
	evEdited     bool
	oob          bool // out-of-band edits (i.e. via CellEditor)
	oobEdited    bool
	currSnapshot snapshot
	currEdits    []term.Event
	repeatEdits  []term.Event

	resetting    bool
	undoTimeline []snapshot
	redoTimeline []snapshot
}

// New allocates storage for a new Vi handler, initializes it and returns it.
func New(buf *cell.Buffer, resource workspaceapi.URI, opts ...Option) *Vi {
	vi := new(Vi)
	vi.Init(buf, resource, opts...)
	return vi
}

// Init initialies this vi handle with the given cell.Buffer.
func (vi *Vi) Init(buf *cell.Buffer, resource workspaceapi.URI, opts ...Option) {
	viHandler := new(viHandlerImpl)
	viHandler.init(buf, opts...)

	vi.init(viHandler, buf, resource, opts...)

	text.WithCopyDelete(viHandler.config.defaultRegister,
		vi, vi.cursor, buf)
	vi.buf.Subscribe(vi)
}

// InitWithScroll initialies this vi handle with the given component.Scroll
// and its cell.Buffer. This does not initialize this Vi implementation with copy
// deletes to clipboard or undo/redo because we don't know if the given Scroll was initialized with
// Subscribe functionality or not. Init should be used in favor of this method for standard
// usage of Vi.
func (vi *Vi) InitWithScroll(scroll *component.Scroll, resource workspaceapi.URI, opts ...Option) {
	viHandler := new(viHandlerImpl)
	viHandler.initWithScroll(scroll, opts...)

	vi.init(viHandler, scroll.Buffer(), resource, opts...)
}

func (vi *Vi) init(viHandler *viHandlerImpl, buf *cell.Buffer, resource workspaceapi.URI, opts ...Option) {
	vi.resource = resource
	vi.handler = viHandler
	vi.buf = buf
	vi.less = &viHandler.less
	vi.cursor = &viHandler.cursor
	vi.mouse = text.NewMouse(newMouseDelegate(viHandler))
	vi.clipboard = viHandler.config.clipboard

	vi.repeatEdits = make([]term.Event, 0)
	vi.currEdits = make([]term.Event, 0)
	vi.undoTimeline = make([]snapshot, 0)
	vi.redoTimeline = make([]snapshot, 0)
	vi.oob = true

	vi.snapshotContent()
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

func (vi *Vi) OnWillEdit(
	ctx context.Context, from, to term.Coordinates, str string,
) {
	if (!vi.oob && vi.oobEdited) || (!vi.currEdited && !vi.resetting) {
		pubVi := (*Vi)(vi)
		pubVi.currSnapshot.cursor = from
		pubVi.pushNewSnapshot()
		pubVi.resetRedoTimeline()
	}
}

func (vi *Vi) OnDidEdit(
	ctx context.Context, start, end term.Coordinates, old string,
) {
	if !vi.resetting {
		vi.evEdited = true
		vi.currEdited = true
	}
	vi.oobEdited = vi.oob
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
				}
				switch ev.Key {
				case term.KeyCtrlR:
					handled = vi.redo()
					return
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

// Man satisfies tui.Handler
func (vi *Vi) Man() tui.Manual {
	return vi.handler.Man()
}

// Resize satisfies tui.Component
func (vi *Vi) Resize(width, height int) {
	vi.handler.Resize(width, height)
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
func (vi *Vi) SetLocationList(
	pri textapi.LocationPriority, ID string, l text.LocationList,
) {
	vi.handler.setLocationList(pri, ID, l)
}

// SetCursorAtScroll sets the cursor of this Vi handler at content pos.
func (vi *Vi) SetCursorAtScroll(pos term.Coordinates) bool {
	return vi.handler.setCursorAtScroll(pos)
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
	return vi.buf.Editor()
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
	vi.less.ShowCommandBar(show)
}

// IsEditMode returns whether the current mode is one of
// the edit modes: insert, delete or replace.
func (vi *Vi) IsEditMode() bool {
	return isEditMode(vi.handler.mode())
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
func (e *Vi) SetDefaultAttributes(attrs term.Attributes) error {
	e.less.Scroll().Attributes = attrs
	return nil
}

// Close satisfies editor.Handler.
func (vi *Vi) Close() error {
	return nil
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
	vi.buf.Edit(context.Background(), from, to, s.content)
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
