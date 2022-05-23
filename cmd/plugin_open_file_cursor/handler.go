package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/plugin"
	plugutil "github.com/ernestrc/go-tui/plugin/util"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

const (
	commandOpenFileCursor = "openFileUnderCursor"
)

var (
	gfHandlerCommands = []string{commandOpenFileCursor}
	gfHandlerEvents   = []text.EventType{
		text.EventTypeOpen,
		text.EventTypeClose,
		text.EventTypeEdit,
		text.EventTypeFlush,
		text.EventTypeCursor,
	}
	gfHandlerPermissions = []plugin.Permission{
		plugin.PermissionWorkspace,
		plugin.PermissionBrowserResourceOpener,
		plugin.PermissionBrowserWindowManager,
	}
)

type file struct {
	cell.Buffer
	component.Scroll
	text.Cursor
}

type gfEditorHandler struct {
	ed  text.Editor
	o   browser.ResourceOpener
	wm  browser.WindowManager
	cwd workspace.Workspace

	files map[string]*file
}

func newFile(content string) *file {
	f := new(file)
	f.Buffer.Init()
	f.Scroll.Init(&f.Buffer)
	f.Cursor.Init(&f.Scroll)
	f.Buffer.WriteString(content)
	return f
}

func newGFHandler(
	ed text.Editor, grants []plugin.Grant,
	broker proto.MuxBroker, pconfig plugin.Config,

) (plugutil.CommandEventHandler, error) {
	ret := new(gfEditorHandler)
	ret.ed = ed
	ret.files = make(map[string]*file)

	var err error
	for _, grant := range grants {
		switch grant.Permission {
		case plugin.PermissionWorkspace:
			ret.cwd, err = plugin.Workspace(grant.Token, broker)
		case plugin.PermissionBrowserWindowManager:
			ret.wm, err = plugin.WindowManager(grant.Token, broker)
		case plugin.PermissionBrowserResourceOpener:
			ret.o, err = plugin.ResourceOpener(grant.Token, broker)
		}
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (f *file) uriAtCursor() string {
	_, _, uri := f.Scroll.TokenAt(f.CursorAtScroll(), func(r rune) bool {
		return (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') || r == '_' ||
			(r >= '0' && r <= '9') || r == ':' || r == '@' ||
			r == '+' || r == '/' || r == '.' || r == '-'
	})
	return uri
}

func (h *gfEditorHandler) openFileUnderCursor(uri workspace.URI) error {
	f, ok := h.files[uri.String()]
	if !ok {
		return fmt.Errorf("could not find buffer for file %s", uri.String())
	}

	word := f.uriAtCursor()
	if word == "" {
		log.Debugf("Could not open file under cursor: word under cursor is empty")
		return nil
	}

	uri, err := workspace.ParseURI(word)
	if err != nil {
		uri, err = h.cwd.URI(word)
	}
	if err != nil {
		return fmt.Errorf("could not build a URI from word under cursor: %v", err)
	}

	opened, err := h.o.Open(uri)
	if err != nil {
		return fmt.Errorf("could not Open URI: %v", err)
	}

	focus, err := h.wm.Focus()
	if err != nil {
		err = fmt.Errorf("wm.Focus: %s", err)
		return err
	}
	err = focus.SetContent(opened)
	if err != nil && err != browser.ErrTabNotFree {
		return err
	}
	return nil
}

func (h *gfEditorHandler) HandleCommand(ctx context.Context, cmd text.Command) (
	exit bool,
) {
	if cmd.Resource == nil {
		return
	}

	switch cmd.Name {
	case commandOpenFileCursor:
		err := h.openFileUnderCursor(cmd.URI)
		if err != nil {
			log.Error(err)
			return
		}
	}

	return
}

func (h *gfEditorHandler) syncBuffers(ev text.Event) error {
	switch ev.Type {
	case text.EventTypeClose:
		delete(h.files, ev.URI.String())
	case text.EventTypeOpen:
		h.files[ev.URI.String()] = newFile(ev.Content)
	case text.EventTypeEdit:
		f, ok := h.files[ev.URI.String()]
		if !ok {
			return fmt.Errorf("could not find buffer for file %s", ev.URI.String())
		}
		f.Edit(ev.Start, ev.End, ev.Content)
	case text.EventTypeCursor:
		f, ok := h.files[ev.URI.String()]
		if !ok {
			return fmt.Errorf("could not find buffer for file %s", ev.URI.String())
		}
		log.Tracef("MoveToScroll(%#v): %s", ev.From, f.String())
		_, ok = f.MoveToScroll(ev.From)
		if !ok {
			return fmt.Errorf("MoveToScroll(%#v): %v", ev.From, ok)
		}
	}
	return nil
}

func (h *gfEditorHandler) Handle(
	ctx context.Context, ev text.Event,
) (exit bool) {
	var start time.Time
	if log.IsLevelEnabled(log.TraceLevel) {
		start = time.Now()
		log.Tracef("Handle(%#v)", ev.Type)
	}

	err := h.syncBuffers(ev)
	if err != nil {
		log.Errorf("Handle(%#v): %v", ev.Type, err)
	}
	if log.IsLevelEnabled(log.TraceLevel) {
		log.Tracef("Handle(%#v) in %s", ev.Type, time.Since(start))
	}
	return
}

func (h *gfEditorHandler) Close() error {
	return nil
}
