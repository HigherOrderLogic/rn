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

package extension

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	browserextension "unstable.build/go-tui/api/browser/extension"
	"unstable.build/go-tui/api/config"
	configextension "unstable.build/go-tui/api/config/extension"
	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	workspaceextension "unstable.build/go-tui/api/workspace/extension"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/term"
)

// Grantee returns this extension's Grantee.
func Grantee() (extension.Grantee, []extension.Permission) {
	return extutil.NewEditorEventHandler(FileBarHandlerCommands,
		newFileBarEditorHandler, FileBarHandlerEvents,
		FileBarHandlerPermissions...)
}

var (
	// FileBarHandlerCommands returns the commands that this extension is
	// interested in registering.
	FileBarHandlerCommands = []textapi.CommandManual{}

	// FileBarHandlerEvents returns the events that this extension is
	// interested in subscribing to.
	FileBarHandlerEvents = []textapi.EventType{
		textapi.EventTypeOpen,
		textapi.EventTypeEdit,
		textapi.EventTypeFlush,
		textapi.EventTypeCursor,
		textapi.EventTypeFocus,
		textapi.EventTypeUnfocus,
	}

	// FileBarHandlerPermissions are the required permissions for this
	// extension to run.
	FileBarHandlerPermissions = []extension.Permission{
		extension.Permission(extension.PermissionBrowserWindowManager),
		extension.Permission(extension.PermissionBrowserEventPublisher),
		extension.Permission(extension.PermissionEditor),
		extension.PermissionConfig,
		extension.Permission(extension.PermissionFileSystem),
	}

	defaultScrollAttr     = term.Attributes{Fg: tcell.ColorDefault}
	defaultBackgroundAttr = term.Attributes{Fg: tcell.ColorDefault}
	defaultDirtyAttr      = term.Attributes{Fg: tcell.ColorYellow}
)

const defaultDirtyIcon = "[+]"

type fileInfo struct {
	dirty bool
}

type fileBarEditorHandler struct {
	wm   browserapi.WindowManager
	p    browserapi.EventPublisher
	cwd  workspaceapi.URI
	exit uint32
	ch   chan textapi.Event

	filenameAttributes      term.Attributes
	filenameDirtyAttributes term.Attributes
	backgroundAttributes    term.Attributes
	showDirty               bool
	tracker                 extutil.ResourceTracker
	dirtyIcon               string

	bar struct {
		sync.Mutex
		comp     component.Reference
		filename component.Scroll
		coords   component.Scroll
	}
}

func newFileBarEditorHandler(
	ctx context.Context, ed textapi.Editor, grants []extension.Grant,
	broker rpc.MuxBroker, pconfig config.Config,
) (extutil.CommandEventHandler, error) {
	ret := new(fileBarEditorHandler)
	ret.ch = make(chan textapi.Event)

	var err error
	ret.filenameAttributes, err = config.GetAttributes(pconfig, "filename_attr")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'filename_attr' from config: %v", err)
		}
		ret.filenameAttributes = defaultScrollAttr
	}

	ret.dirtyIcon, err = pconfig.GetString("dirty_icon")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'dirty_icon' from config: %v", err)
		}
		ret.dirtyIcon = defaultDirtyIcon
	}

	ret.backgroundAttributes, err = config.GetAttributes(pconfig, "background_attr")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'background_attr' from config: %v", err)
		}
		ret.backgroundAttributes = defaultBackgroundAttr
	}

	ret.showDirty, err = pconfig.GetBool("show_dirty")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'show_dirty' from config: %v", err)
		}
		ret.showDirty = true
	}

	ret.filenameDirtyAttributes, err = config.GetAttributes(pconfig, "filename_dirty_attr")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'filename_dirty_attr' from config: %v", err)
		}
		ret.filenameDirtyAttributes = defaultDirtyAttr
	}

	ret.bar.coords.Attributes, err = config.GetAttributes(pconfig, "coordinates_attr")
	if err != nil {
		if err != config.ErrNotFound {
			log.Warningf("failed to get 'coordinates_attr' from config: %v", err)
		}
		ret.bar.coords.Attributes = defaultScrollAttr
	}

	ret.initBar(component.Nop())

	for _, grant := range grants {
		switch grant.Permission {
		case extension.Permission(extension.PermissionBrowserEventPublisher):
			ret.p, err = browserextension.EventPublisher(ctx, grant, broker)
			if err != nil {
				return nil, err
			}
		case extension.Permission(extension.PermissionBrowserWindowManager):
			ret.wm, err = browserextension.WindowManager(ctx, grant, broker)
			if err != nil {
				return nil, err
			}
			comp := component.Sync(&ret.bar, &ret.bar.comp)
			cfg := browserapi.BarConfig{
				Frame:       browserapi.BarFrameDefault,
				Size:        1,
				Orientation: browserapi.OrientationBottom,
			}
			err = ret.wm.Bar(cfg, handler.Nop(comp))
			if err != nil {
				return nil, err
			}
		case extension.Permission(extension.PermissionFileSystem):
			w, err := workspaceextension.FileSystem(ctx, grant, broker)
			if err != nil {
				return nil, err
			}
			ret.cwd, err = w.URI(".")
			if err != nil {
				return nil, err
			}
		case extension.PermissionConfig:
			config, err := configextension.FetchConfig(ctx, grant, broker)
			if err != nil {
				return nil, err
			}
			tabspaces, err := extutil.Tabspaces(config)
			if err != nil {
				return nil, fmt.Errorf("could not get tabspaces from config: %v", err)
			}
			wrap, err := extutil.Wrap(config)
			if err != nil {
				return nil, fmt.Errorf("could not get wrap mode from config: %v", err)
			}
			ret.tracker.Init(tabspaces, wrap)
		}
	}

	ret.bar.filename.Init(cell.NewBuffer())
	ret.bar.coords.Init(cell.NewBuffer())

	go ret.handleEvents()

	return ret, nil
}

func (t *fileBarEditorHandler) Complete(ctx context.Context, name string, args []string) (
	iterator.Iterator[string], error,
) {
	return iterator.FromSlice[string](nil), nil
}

func (h *fileBarEditorHandler) HandleCommand(ctx context.Context, cmd textapi.Command) (
	err error,
) {
	return
}

func (h *fileBarEditorHandler) Handle(
	ctx context.Context, ev textapi.Event,
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
	closing := atomic.CompareAndSwapUint32(&h.exit, 0, 1)
	if !closing {
		return nil // already closed
	}
	close(h.ch)
	return nil
}

func (h *fileBarEditorHandler) prettyFileName(resource workspaceapi.URI) string {
	return workspaceapi.RelPath(h.cwd, resource)
}

func (h *fileBarEditorHandler) resetBarContent() {
	h.bar.Lock()
	defer h.bar.Unlock()

	h.bar.filename.Buffer().Reset()
	h.bar.filename.Init(h.bar.filename.Buffer())
	h.bar.coords.Buffer().Reset()
	h.bar.coords.Init(h.bar.coords.Buffer())
}

func (h *fileBarEditorHandler) refreshBarContent(ev textapi.Event) {
	h.resetBarContent()

	h.bar.Lock()
	defer h.bar.Unlock()

	res, ok := h.getResource(ev)
	if !ok {
		return
	}

	name := h.prettyFileName(res.URI())
	totalRows := res.Buffer().Rows()
	totalCols := 0
	cursor := res.Cursor()
	if cursor.Y < totalRows {
		totalCols = res.Buffer().Columns(cursor.Y)
	}
	coords := fmt.Sprintf("%d/%d %d/%d", cursor.X+1, totalCols, cursor.Y+1, totalRows)

	h.bar.filename.Buffer().WriteString(name)
	h.bar.coords.Buffer().WriteString(coords)

	if h.showDirty && res.Metadata.(*fileInfo).dirty {
		h.bar.filename.Attributes = h.filenameDirtyAttributes
		h.bar.filename.Buffer().WriteString(h.dirtyIcon)
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
	h.initBar(component.Grid([][]tui.Component{{fileSpan, coordsSpan}}))
}

func (h *fileBarEditorHandler) initBar(c tui.Component) {
	h.bar.comp.Init(component.WithBackground(
		c, term.Cell{Attributes: h.backgroundAttributes},
	))
}

func (h *fileBarEditorHandler) setFileDirty(ev textapi.Event, dirty bool) {
	res, ok := h.getResource(ev)
	if !ok {
		return
	}
	res.Metadata.(*fileInfo).dirty = dirty
	log.Tracef("set file dirty (%s): %#v", ev.URI.Path(), dirty)
}

func (h *fileBarEditorHandler) handleEvents() {
	ctx := context.Background()
	for ev := range h.ch {
		if ev.URI == (workspaceapi.URI{}) {
			continue
		}

		var start time.Time
		if log.IsLevelEnabled(log.TraceLevel) {
			start = time.Now()
			h.log(log.TraceLevel, "handle %v", ev.Type)
		}

		h.tracker.Handle(ctx, ev)

		switch ev.Type {
		case textapi.EventTypeEdit:
			h.setFileDirty(ev, true)
			h.refreshBarContent(ev)
			h.interrupt(ctx)
		case textapi.EventTypeOpen:
			res, _ := h.getResource(ev)
			res.Metadata = new(fileInfo)
			h.refreshBarContent(ev)
			h.interrupt(ctx)
		case textapi.EventTypeFlush:
			h.setFileDirty(ev, false)
			fallthrough
		case textapi.EventTypeFocus:
			h.refreshBarContent(ev)
			h.interrupt(ctx)
		case textapi.EventTypeUnfocus:
			h.resetBarContent()
			h.interrupt(ctx)
		case textapi.EventTypeCursor:
			h.refreshBarContent(ev)
			h.interrupt(ctx)
		}
		if log.IsLevelEnabled(log.TraceLevel) {
			h.log(log.TraceLevel, "handle %v in %s", ev.Type, time.Since(start))
		}
	}
}

func (h *fileBarEditorHandler) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "extension.fileBarEditorHandler",
	}).Logf(level, msg, args...)
}

func (h *fileBarEditorHandler) interrupt(ctx context.Context) {
	if err := h.p.Interrupt(ctx); err != nil {
		h.log(log.ErrorLevel, "interrupt: %v", err)
	}
}

func (h *fileBarEditorHandler) getResource(ev textapi.Event) (
	*extutil.TrackedResource, bool,
) {
	res, ok := h.tracker.Resource(ev.URI)
	if !ok {
		h.log(log.ErrorLevel, "resource with uri %q not found in tracker",
			ev.URI.String())
	}
	return res, ok
}
