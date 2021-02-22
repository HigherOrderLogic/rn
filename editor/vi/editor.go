package vi

import (
	"errors"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/editor"
	"github.com/ernestrc/go-tui/term"
)

type viEditor struct {
	opts []Option
	subs map[editor.EventType][]editor.EventHandler
}

// Editor returns a Vi editor.Editor.
func Editor(opts ...Option) editor.Editor {
	subs := make(map[editor.EventType][]editor.EventHandler)
	return &viEditor{opts: opts, subs: subs}
}

func (e *viEditor) Handle(ev editor.Event) bool {
	e.dispatchEvent(ev)
	return false
}

func (e *viEditor) dispatchEvent(ev editor.Event) {
	subs, ok := e.subs[ev.Type]
	if !ok {
		return
	}

	remain := make([]editor.EventHandler, 0, len(subs))
	for _, sub := range subs {
		exit := sub.Handle(ev)
		if !exit {
			remain = append(remain, sub)
		}
	}
	e.subs[ev.Type] = remain
}

func (e *viEditor) Edit(name string, buf *cell.Buffer) (editor.Handler, error) {
	h := New(buf, e.opts...)

	e.dispatchEvent(editor.Event{
		Type:         editor.EventTypeOpen,
		ResourceName: name,
		Resource:     h,
		Content:      buf.String(),
	})

	sub := editor.CellSubscriber(name, h, e)
	buf.Subscribe(sub)

	return h, nil
}

func (e *viEditor) Register(cmd string, h editor.CommandHandler) error {
	return errors.New("not supported")
}

// SubscribeEditor subsribes sub to ev. Note that this Editor is only capable
// of dispatching EventTypeOpen, EventTypeInsert and EventTypeDelete EventType events.
func (e *viEditor) SubscribeEditor(ev editor.EventType, sub editor.EventHandler) error {
	if _, ok := e.subs[ev]; !ok {
		e.subs[ev] = []editor.EventHandler{sub}
		return nil
	}
	e.subs[ev] = append(e.subs[ev], sub)
	return nil
}

func (e viEditor) SetLocationList(h editor.Handler, ID string, loc editor.LocationList) error {
	h.(*Vi).SetLocationList(ID, loc)
	return nil
}

func (e *viEditor) MoveToNextLocation(h editor.Handler, ID string) error {
	h.(*Vi).MoveToNextLocation(ID)
	return nil
}

func (e *viEditor) MoveToPrevLocation(h editor.Handler, ID string) error {
	h.(*Vi).MoveToPrevLocation(ID)
	return nil
}

func (e *viEditor) Reader(h editor.Handler) editor.Reader {
	return editor.CellReader(h.(*Vi).less.Buffer().Reader())
}

func (e *viEditor) Writer(h editor.Handler) editor.Writer {
	return editor.CellWriter(h.(*Vi).less.Buffer().Writer())
}

func (e *viEditor) SetCursor(h editor.Handler, pos term.Coordinates) error {
	ok := h.(*Vi).SetCursorAtScroll(pos)
	if !ok {
		return errors.New("SetCursor: invalid cursor position")
	}
	return nil
}

func (e *viEditor) Cursor(h editor.Handler) (term.Coordinates, error) {
	pos := h.(*Vi).CursorAtScroll()
	return pos, nil
}
