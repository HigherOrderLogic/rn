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

package extutil

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"
)

// ResourceTracker implements a remote text editor tracker,
// which handles events emitted by a textapi.Editor
// to replicate the exact state of all the open resources.
//
// It satisfies textapi.EventHandler so clients are
// responsible for subscribing it to a textapi.Editor.
//
// Clients must subscribe to EventTypeFocus if
// wrap mode is set, and cursor position needs
// to be tracked.
type ResourceTracker struct {
	resources map[string]*TrackedResource
	wrap      bool
	tabspaces int
	focus     *TrackedResource
}

var _ textapi.EventHandler = (*ResourceTracker)(nil)

// ResourceTrackerEventsComplete returns a slice of textapi.EventType
// required in calls to textapi.Editor.SubscribeEvents for a ResourceTracker
// to replicate all state, which includes, cursor and scroll offsets.
//
// Subscribing to cursor and scroll events might impose a performance
// penalty, so use only when indeed cursor and scroll coordinates
// are strictly necessary.
func ResourceTrackerEventsComplete() []textapi.EventType {
	return []textapi.EventType{
		textapi.EventTypeOpen,
		textapi.EventTypeFocus,
		textapi.EventTypeClose,
		textapi.EventTypeFlush,
		textapi.EventTypeEdit,
		textapi.EventTypeScroll,
		textapi.EventTypeCursor,
		textapi.EventTypeVisible,
		textapi.EventTypeHidden,
	}
}

// ResourceTrackerEventsContent returns a slice of textapi.EventType
// required in calls to textapi.Editor.SubscribeEvents for a ResourceTracker
// to replicate only the resource content, edit by edit. Cursor and
// scroll offsets will not be tracked.
func ResourceTrackerEventsContent() []textapi.EventType {
	return []textapi.EventType{
		textapi.EventTypeOpen,
		textapi.EventTypeFocus,
		textapi.EventTypeClose,
		textapi.EventTypeFlush,
		textapi.EventTypeEdit,
	}
}

// ResourceTrackerEventsFlushOnly returns a slice of textapi.EventType
// required in calls to textapi.Editor.SubscribeEvents for a ResourceTracker
// to replicate only the resource content, upon flushing. Edits before
// flushing to disk are ignored and also scroll and cursor offsets
// will not be tracked.
func ResourceTrackerEventsFlushOnly() []textapi.EventType {
	return []textapi.EventType{
		textapi.EventTypeOpen,
		textapi.EventTypeFocus,
		textapi.EventTypeClose,
		textapi.EventTypeFlush,
	}
}

// NewResourceTracker allocates storage for a new tracker and initializes it.
func NewResourceTracker(tabspaces int, wrap bool) *ResourceTracker {
	ret := new(ResourceTracker)
	ret.Init(tabspaces, wrap)
	return ret
}

// Init initializes this ResourceTracker.
func (t *ResourceTracker) Init(tabspaces int, wrap bool) {
	t.resources = make(map[string]*TrackedResource)
	t.tabspaces = tabspaces
	t.wrap = wrap
}

// Resource returns the tracked resource with the given URI, or false
// if a resource with the given uri is not currently open.
func (t *ResourceTracker) Resource(uri workspaceapi.URI) (*TrackedResource, bool) {
	ret, ok := t.resources[uri.String()]
	return ret, ok
}

// Focus returns the tracked resource currently in focus, or false
// if there is no tracked resource currently in focus. Clients
// must subscribe this ResourceTracker to EventTypeFocus events.
func (t *ResourceTracker) Focus() (*TrackedResource, bool) {
	ok := t.focus != nil
	return t.focus, ok
}

// Handle satisfies textapi.EventHandler.
func (h *ResourceTracker) Handle(_ context.Context, ev textapi.Event) (exit bool) {
	if ev.URI == (workspaceapi.URI{}) {
		h.log(log.TraceLevel, "ignoring event with resource with empty uri: %d", ev.Type)
		return
	}
	var ok bool
	switch ev.Type {
	case textapi.EventTypeOpen:
		h.handleResourceOpen(ev)
		ok = true
	case textapi.EventTypeFocus:
		ok = h.handleResourceFocus(ev)
	case textapi.EventTypeClose:
		h.handleResourceClose(ev)
		ok = true
	case textapi.EventTypeEdit:
		ok = h.handleResourceEdit(ev)
	case textapi.EventTypeFlush:
		ok = h.handleResourceFlush(ev)
	case textapi.EventTypeScroll:
		ok = h.handleResourceScroll(ev)
	case textapi.EventTypeCursor:
		ok = h.handleResourceCursor(ev)
	case textapi.EventTypeHidden:
		ok = h.handleResourceHidden(ev)
	case textapi.EventTypeVisible:
		ok = h.handleResourceVisible(ev)
	default:
		ok = true // ignore the rest of event types
	}

	if !ok {
		h.log(log.WarnLevel, "resource with uri '%s' not found "+
			"or could not process event type: %d", ev.URI.String(), ev.Type)
	}
	return
}

func (h *ResourceTracker) handleResourceOpen(ev textapi.Event) {
	buf := new(cell.Buffer)
	buf.Init()
	buf.WriteString(ev.Content)
	trackedResource := new(TrackedResource)
	trackedResource.uri = ev.URI
	trackedResource.Scroll.Init(buf)
	trackedResource.Scroll.Wrap = h.wrap
	trackedResource.Scroll.SetTabspaces(h.tabspaces)

	h.resources[ev.URI.String()] = trackedResource
}

func (h *ResourceTracker) handleResourceFocus(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}

	res.Scroll.Resize(ev.Start.X, ev.Start.Y)
	if res.Scroll.Wrap {
		res.Scroll.RecalculateWraps()
	}
	h.focus = res
	return true
}

func (h *ResourceTracker) handleResourceClose(ev textapi.Event) {
	delete(h.resources, ev.URI.String())
}

func (h *ResourceTracker) handleResourceEdit(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}
	res.Scroll.Buffer().Edit(context.Background(), ev.Start, ev.End, ev.Content)
	if res.Scroll.Wrap {
		res.Scroll.RecalculateWraps()
	}
	return true
}

func (h *ResourceTracker) handleResourceFlush(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}
	res.Scroll.Buffer().Reset()
	res.Scroll.Buffer().WriteString(ev.Content)
	if res.Scroll.Wrap {
		res.Scroll.RecalculateWraps()
	}
	return true
}

func (h *ResourceTracker) handleResourceScroll(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}
	if res.Scroll.Offset() == ev.Start {
		return true
	}
	return res.Scroll.SetOffset(ev.Start)
}

func (h *ResourceTracker) handleResourceHidden(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}
	return res.Scroll.MarkHidden(ev.Start.Y, ev.End.Y)
}

func (h *ResourceTracker) handleResourceVisible(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}
	return res.Scroll.MarkVisible(ev.Start.Y)
}

func (h *ResourceTracker) handleResourceCursor(ev textapi.Event) bool {
	res, ok := h.resources[ev.URI.String()]
	if !ok {
		return false
	}

	// only initialize cursor if we are monitoring cursor
	// keeps it obvious for the rest of impl that cursor might
	// not be useful.
	if res.cursor == nil {
		res.cursor = text.NewCursor(&res.Scroll, nil)
	}
	_, _ = res.cursor.MoveToScroll(ev.From)
	// do not use the return of MoveToScroll, as SetOffset might have
	// changed the resource's cursor position as well.
	return true
}

func (h *ResourceTracker) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "extutil.ResourceTracker",
	}).Logf(level, msg, args...)
}
