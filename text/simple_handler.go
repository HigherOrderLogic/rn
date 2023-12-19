package text

import (
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text/clipboard"
)

type simpleEditorHandler struct {
	buf              *cell.Buffer
	less             handler.Less
	resource         workspaceapi.URI
	cursor           Cursor
	height           int
	mouse            *Mouse
	clipboard        clipboard.Register
	pendingSetCursor *term.Coordinates
}

// NewSimpleHandler returns a modeless, simple-to-use text.Handler.
func NewSimpleHandler(
	clipboard clipboard.Register,
	buf *cell.Buffer, resource workspaceapi.URI,
	wrap, commandBar bool,
	attr, resAttr term.Attributes,
) Handler {
	ret := new(simpleEditorHandler)
	ret.Init(clipboard, buf, resource, wrap, commandBar, attr, resAttr)
	return ret
}

func (h *simpleEditorHandler) Init(
	clipboard clipboard.Register,
	buf *cell.Buffer, resource workspaceapi.URI,
	wrap, commandBar bool,
	attr, resAttr term.Attributes,
) {
	h.buf = buf
	h.resource = resource
	h.less.InitWithBuffer(buf, handler.LessConfig{
		Wrap:  wrap,
		NoBar: !commandBar,
	})
	scroll := h.less.Scroll()
	scroll.Attributes = attr
	scroll.ResultsAttr = resAttr
	h.cursor.Init(h.less.Scroll())
	h.mouse = NewMouse(CursorMouseDelegate(&h.cursor))
	h.clipboard = clipboard
}

// Resize satisfies tui.Component
func (h *simpleEditorHandler) Resize(width, height int) {
	h.height = height
	h.less.Resize(width, height)
	if h.pendingSetCursor != nil {
		h.setCursor(*h.pendingSetCursor)
	}
}

// Draw satisfies tui.Component
func (h *simpleEditorHandler) Draw(w term.Writer) {
	locs, _ := h.cursor.Locations()
	for _, loc := range locs {
		h.less.Notify(loc.Message)
		return
	}
	h.less.Draw(w)
}

func (h *simpleEditorHandler) Handle(ev term.Event) (exit, handled bool) {
	// only a user event clears a pending set cursor
	h.pendingSetCursor = nil

	if ev.Type == term.EventMouse {
		return h.mouse.Handle(ev)
	}

	switch ev.Key {
	case term.KeyArrowLeft:
		if ev.Mod == term.ModAlt {
			handled = h.cursor.MoveLeftStartWord()
		} else {
			handled = h.cursor.MoveLeft()
		}
	case term.KeyArrowRight:
		if ev.Mod == term.ModAlt {
			handled = h.cursor.MoveRightStartWord()
		} else {
			handled = h.cursor.MoveRight()
		}
	case term.KeyArrowUp:
		handled = h.cursor.MoveUp()
	case term.KeyArrowDown:
		handled = h.cursor.MoveDown()
	case term.KeyEnter:
		h.cursor.Insert('\n')
		handled = true
	case term.KeySpace:
		h.cursor.Insert(' ')
		handled = true
	case term.KeyTab:
		h.cursor.Insert('\t')
		handled = true
	case term.KeyCtrlC:
		h.cursor.CopySelection(clipboard.DefaultRegisterID, h.clipboard)
		handled = true
	case term.KeyCtrlV:
		paste, err := h.clipboard.Paste(clipboard.DefaultRegisterID)
		if err != nil {
			log.Errorf("clipboard paste: %v", err)
		} else {
			str := paste.Text
			h.cursor.Paste(str, StandardSelection, false)
		}
		handled = true
	case term.KeyBackspace, term.KeyBackspace2:
		if h.cursor.Selection() != "" {
			handled = h.cursor.DeleteSelection()
		} else {
			handled = h.cursor.Backspace()
		}
	case term.KeyCtrlA:
		handled = h.cursor.MoveStartLine()
	case term.KeyCtrlE:
		handled = h.cursor.MoveEndLine()
	case term.KeyCtrlZ:
		handled = h.cursor.Undo()
	case term.KeyCtrlR:
		handled = h.cursor.Redo()
	case term.KeyCtrlF:
		ev.Key = 0
		ev.Ch = '/'
		_, handled = h.less.Handle(ev)
	default:
		if ev.Ch != 0 {
			h.cursor.Insert(ev.Ch)
			handled = true
		}
	}
	return
}

// Cursor satisfies tui.Handler
func (h *simpleEditorHandler) Cursor() (
	pos term.Coordinates, style term.CursorStyle, show bool,
) {
	return h.cursor.Coordinates(), term.CursorStyleSteadyBar, true
}

// Man satisfies tui.Handler
func (h *simpleEditorHandler) Man() tui.Manual {
	return tui.Manual{}
}

// Close satisfies editor.Handler.
func (h *simpleEditorHandler) Close() error {
	return nil
}

// Resource satisfies editor.Handler.
func (h *simpleEditorHandler) Resource() workspaceapi.URI {
	return h.resource
}

// SetWrap satisfies editor.Handler.
func (h *simpleEditorHandler) SetWrap(wrap bool) {
	h.less.Scroll().Wrap = wrap
}
func (t *simpleEditorHandler) ShowCommandBar(show bool) {
	t.less.ShowCommandBar(show)
}

func (h *simpleEditorHandler) setCursor(pos term.Coordinates) bool {
	// setCursor should be robust against resizes, etc.
	// only the first client interaction should clear this position
	h.pendingSetCursor = new(term.Coordinates)
	*h.pendingSetCursor = pos

	_, ok := h.cursor.MoveToScroll(pos)
	return ok
}
