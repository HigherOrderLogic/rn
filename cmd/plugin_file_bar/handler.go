package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/plugin"
	plugutil "github.com/ernestrc/go-tui/plugin/util"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

var (
	fileBarHandlerCommands = []string{}
	fileBarHandlerEvents   = []text.EventType{
		text.EventTypeOpen,
		text.EventTypeEdit,
		text.EventTypeFlush,
		text.EventTypeCursor,
		text.EventTypeFocus,
		text.EventTypeUnfocus,
	}
	fileBarHandlerPermissions = []plugin.Permission{
		plugin.PermissionBrowserWindowManager,
		plugin.PermissionBrowserEventPublisher,
		plugin.PermissionEditor,
	}

	defaultScrollAttr     = term.Attributes{Fg: term.ColorDefault}
	defaultBackgroundAttr = term.Attributes{Fg: term.ColorDefault}
	defaultDirtyAttr      = term.Attributes{Fg: term.ColorYellow}
)

type fileInfo struct {
	cells  [][]term.Cell
	offset term.Coordinates
	dirty  bool
}

type fileBarEditorHandler struct {
	wm   browser.WindowManager
	p    browser.EventPublisher
	cwd  string
	exit uint32
	ch   chan text.Event

	filenameAttributes      term.Attributes
	filenameDirtyAttributes term.Attributes
	backgroundAttributes    term.Attributes
	showDirty               bool

	bar struct {
		sync.Mutex
		comp     component.Reference
		filename component.Scroll
		coords   component.Scroll
	}
	files map[string]*fileInfo
}

func newFileBarEditorHandler(
	ed text.Editor, grants []plugin.Grant,
	broker proto.MuxBroker, pconfig plugin.Config,

) (plugutil.CommandEventHandler, error) {
	ret := new(fileBarEditorHandler)
	ret.files = make(map[string]*fileInfo)
	ret.ch = make(chan text.Event)

	var err error
	ret.cwd, err = os.Getwd()
	if err != nil {
		return nil, err
	}

	ret.filenameAttributes, err = plugin.GetAttributes(pconfig, "filename_attr")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Warningf("failed to get 'filename_attr' from config: %v", err)
		}
		ret.filenameAttributes = defaultScrollAttr
	}

	ret.backgroundAttributes, err = plugin.GetAttributes(pconfig, "background_attr")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Warningf("failed to get 'background_attr' from config: %v", err)
		}
		ret.backgroundAttributes = defaultBackgroundAttr
	}

	ret.showDirty = true
	ret.showDirty, err = pconfig.GetBool("show_dirty")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Warningf("failed to get 'show_dirty' from config: %v", err)
		}
		ret.showDirty = true
	}

	ret.filenameDirtyAttributes, err = plugin.GetAttributes(pconfig, "filename_dirty_attr")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Warningf("failed to get 'filename_dirty_attr' from config: %v", err)
		}
		ret.filenameDirtyAttributes = defaultDirtyAttr
	}

	ret.bar.coords.Attributes, err = plugin.GetAttributes(pconfig, "coordinates_attr")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Warningf("failed to get 'coordinates_attr' from config: %v", err)
		}
		ret.bar.coords.Attributes = defaultScrollAttr
	}

	for _, grant := range grants {
		switch grant.Permission {
		case plugin.PermissionBrowserEventPublisher:
			ret.p, err = plugin.EventPublisher(grant.Token, broker)
			if err != nil {
				return nil, err
			}
		case plugin.PermissionBrowserWindowManager:
			ret.wm, err = plugin.WindowManager(grant.Token, broker)
			if err != nil {
				return nil, err
			}
			comp := component.Sync(&ret.bar, &ret.bar.comp)
			err = ret.wm.Bar(browser.OrientationBottom, handler.Nop(comp))
			if err != nil {
				return nil, err
			}
		}
	}

	go ret.handleEvents()

	return ret, nil
}

func (h *fileBarEditorHandler) HandleCommand(ctx context.Context, cmd text.Command) (
	exit bool,
) {
	return
}

func relativizeFilepath(prefixPath, path string) string {
	if filepath.HasPrefix(path, prefixPath) {
		return path[len(prefixPath)+1:]
	}
	return path
}

func (h *fileBarEditorHandler) refreshBarContent(name string) {
	h.bar.Lock()
	defer h.bar.Unlock()

	h.bar.filename.Init(cell.NewBuffer())
	h.bar.coords.Init(cell.NewBuffer())
	file, ok := h.files[name]
	if !ok || file == nil {
		log.Errorf("could not find file info for file %q", name)
		return
	}

	name = relativizeFilepath(h.cwd, name)
	rows := len(file.cells)
	var cols int
	if file.offset.Y < len(file.cells) {
		cols = len(file.cells[file.offset.Y])
	}
	coords := fmt.Sprintf("%d/%d %d/%d", file.offset.X+1, cols, file.offset.Y+1, rows)

	h.bar.filename.Buffer().WriteString(name)
	h.bar.coords.Buffer().WriteString(coords)

	if h.showDirty && file.dirty {
		h.bar.filename.Attributes = h.filenameDirtyAttributes
		h.bar.filename.Buffer().WriteString("[+]")
	} else {
		h.bar.filename.Attributes = h.filenameAttributes
	}

	fileSpanCfg := component.SpanConfig{
		ContentAlignment: component.SpanAlignmentLeft,
		PadHorizontal:    -h.bar.filename.Buffer().Columns(0),
	}
	coordsSpanCfg := component.SpanConfig{
		ContentAlignment: component.SpanAlignmentRight,
		PadHorizontal:    -len(coords),
	}
	fileSpan := component.NewSpan(&h.bar.filename, fileSpanCfg)
	coordsSpan := component.NewSpan(&h.bar.coords, coordsSpanCfg)
	h.bar.comp.Init(component.WithBackground(
		component.Grid([][]tui.Component{{fileSpan, coordsSpan}}),
		term.Cell{Bg: h.backgroundAttributes.Bg, Fg: h.backgroundAttributes.Fg},
	))
}

func (h *fileBarEditorHandler) getFileInfo(name string) *fileInfo {
	f, ok := h.files[name]
	if ok {
		return f
	}
	ret := new(fileInfo)
	h.files[name] = ret
	return ret
}

func (h *fileBarEditorHandler) setScrollMaxContent(resourceName string, ev text.Event) {
	cells := cell.StringToCells(ev.Content)
	h.getFileInfo(resourceName).cells = cells
	log.Debugf("setScrollMaxContent(%s): %d", resourceName, len(cells))
}

func (h *fileBarEditorHandler) setCursorOffset(filename string, pos term.Coordinates) {
	h.getFileInfo(filename).offset = pos
	log.Tracef("setScrollOffset(%s): %#v OK", filename, pos)
}

func (h *fileBarEditorHandler) setFileDirty(filename string, dirty bool) {
	h.getFileInfo(filename).dirty = dirty
	log.Tracef("setFileDirty(%s): %#v OK", filename, dirty)
}

func (h *fileBarEditorHandler) handleEvents() {
	for ev := range h.ch {
		if ev.URI == (workspace.URI{}) {
			continue
		}

		var start time.Time
		if log.IsLevelEnabled(log.TraceLevel) {
			start = time.Now()
			log.Tracef("Handle(%#v)", ev.Type)
		}

		resourceName := ev.URI.Path()

		var err error
		switch ev.Type {
		case text.EventTypeEdit:
			h.setFileDirty(resourceName, true)
			h.refreshBarContent(resourceName)
			err = h.p.PublishInterrupt()
		case text.EventTypeOpen:
			h.setCursorOffset(resourceName, term.Coordinates{})
			h.setScrollMaxContent(resourceName, ev)
			h.refreshBarContent(resourceName)
			err = h.p.PublishInterrupt()
		case text.EventTypeFlush:
			h.setFileDirty(resourceName, false)
			h.setScrollMaxContent(resourceName, ev)
			fallthrough
		case text.EventTypeFocus:
			h.refreshBarContent(resourceName)
			err = h.p.PublishInterrupt()
		case text.EventTypeUnfocus:
			h.refreshBarContent("")
			err = h.p.PublishInterrupt()
		case text.EventTypeCursor:
			h.setCursorOffset(resourceName, ev.From)
			h.refreshBarContent(resourceName)
			err = h.p.PublishInterrupt()
		}
		if err != nil {
			log.Errorf("Handle(%#v): %v", ev.Type, err)
		}
		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("Handle(%#v) in %s", ev.Type, time.Since(start))
		}
	}
}

func (h *fileBarEditorHandler) Handle(
	ctx context.Context, ev text.Event,
) (exit bool) {
	uexit := atomic.LoadUint32(&h.exit)
	exit = uexit != 0
	if exit {
		return
	}

	h.ch <- ev
	return
}

func (h *fileBarEditorHandler) Close() error {
	atomic.StoreUint32(&h.exit, 1)
	close(h.ch)
	return nil
}
