package main

import (
	"io"

	"github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
)

var (
	emptyWorkspace = handler.Nop(component.StringWithConfig("such empty!", component.StringConfig{
		Alignment: component.SpanAlignmentCentered,
	}))
	workspaceCommands = map[string]func(*workspaceHandler, ...string) (bool, error){
		"addWorkspace":      (*workspaceHandler).commandAddWorkspace,
		"switchToWorkspace": (*workspaceHandler).commandSwitchToWorkspace,
	}
)

type handlerManager struct {
	tui.Handler
	*workspace.Manager
}

type workspaceHandler struct {
	workspaces []*handlerManager
	focus      tui.Handler
}

// newHandler allocates storage for a new workspace tui.Handler and initializes it
// with the given initial workspace.Manager and an instance created
// with tue given tui.Handler factory function.
func newHandler(
	ed text.Editor, initial *workspace.Manager, opts ...text.Option,
) (tui.Handler, error) {
	ret := new(workspaceHandler)
	err := ret.init(ed, initial)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (h *workspaceHandler) init(
	ed text.Editor, initial *workspace.Manager, opts ...text.Option,
) error {
	h.workspaces = make([]*handlerManager, 10)

	// FIXME options might re-open same file over and over?
	// setup initial workspace
	ex, err := newEx(ed, initial, opts...)
	if err != nil {
		return err
	}
	hm := &handlerManager{Handler: ex, Manager: initial}
	h.workspaces[0] = hm
	h.focus = hm

	for id, cmd := range workspaceCommands {
	}
	ed.SubscribeCommand()
	return nil
}

func (h *workspaceHandler) Resize(width, height int) {
	for _, w := range h.workspaces {
		if w != nil {
			w.Resize(width, height)
		}
	}
}

func (h *workspaceHandler) Draw(w term.Writer) {
	h.focus.Draw(w)
}

func (h *workspaceHandler) switchToWorkspace(i int) {
	workspace := h.workspaces[i]
	if workspace == nil {
		h.focus = emptyWorkspace
	} else {
		h.focus = workspace
	}
}

func (h *workspaceHandler) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = h.focus.Handle(ev)
	if exit || handled {
		return exit, handled
	}
	handled = true
	switch ev.Ch {
	case '1':
		h.switchToWorkspace(0)
	case '2':
		h.switchToWorkspace(1)
	case '3':
		h.switchToWorkspace(2)
	case '4':
		h.switchToWorkspace(3)
	case '5':
		h.switchToWorkspace(4)
	case '6':
		h.switchToWorkspace(5)
	case '7':
		h.switchToWorkspace(6)
	case '8':
		h.switchToWorkspace(7)
	case '9':
		h.switchToWorkspace(8)
	}
	return
}

func (h *workspaceHandler) Cursor() (pos term.Coordinates, show bool) {
	return h.focus.Cursor()
}

func (h *workspaceHandler) Man() tui.Manual {
	return h.focus.Man()
}

func (h *workspaceHandler) Close() error {
	var ret error
	for _, hm := range h.workspaces {
		if hm == nil {
			continue
		}
		if closer, ok := hm.Handler.(io.Closer); ok {
			err := closer.Close()
			if err != nil {
				multierror.Append(ret, err)
			}
		}
		err := hm.Manager.Close()
		if err != nil {
			multierror.Append(ret, err)
		}
	}
	return nil
}

func (e *workspaceHandler) commandAddWorkspace(args ...string) (bool, error) {

}

func (e *workspaceHandler) commandSwitchToWorkspace(args ...string) (bool, error) {

}
