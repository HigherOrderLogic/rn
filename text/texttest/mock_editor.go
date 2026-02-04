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

//revive:disable:exported
package texttest

import (
	"context"
	"errors"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

// TestEditor is an editor that can be used in tests.
type TestEditor struct {
	uri  workspaceapi.URI
	buf  *cell.Buffer
	subs map[textapi.EventType][]text.EventHandler
	cb   func()
}

// NopEditor returns a editor suitable for testing.
// It doesn't return real tui.Handler upon Edit but mimics
// editor.Simple's subscription logic.
func NopEditor() *TestEditor {
	return &TestEditor{}
}

// NopEditorWithCallback returns a text.Editor that calls cb when
// SubscribeEvents is called.
func NopEditorWithCallback(cb func()) *TestEditor {
	return &TestEditor{cb: cb}
}

func (e *TestEditor) Handle(ctx context.Context, ev textapi.Event) bool {
	e.dispatchEvent(ctx, ev)
	return false
}

func (e *TestEditor) dispatchEvent(ctx context.Context, ev textapi.Event) {
	if len(e.subs) == 0 {
		return
	}
	subs, ok := e.subs[ev.Type]
	if !ok {
		return
	}

	remain := make([]text.EventHandler, 0, len(subs))
	for _, sub := range subs {
		exit := sub.Handle(ctx, ev)
		if !exit {
			remain = append(remain, sub)
		}
	}
	e.subs[ev.Type] = remain
}

type TestEditorHandler struct {
	browsertest.TestHandler
	LocationList  text.LocationList
	parent        *TestEditor
	uri           workspaceapi.URI
	Width, Height int
}

func (e *TestEditorHandler) Resource() workspaceapi.URI {
	return e.uri
}

func (e *TestEditorHandler) SetWrap(wrap bool) {
}

func (t *TestEditorHandler) ShowCommandBar(show bool) {
}

// SeekUp satisfies component.Scrollable.
func (t *TestEditorHandler) SeekUp() bool {
	return false
}

// SeekDown satisfies component.Scrollable.
func (t *TestEditorHandler) SeekDown() bool {
	return false
}

// SeekOffset satisfies component.Scrollable.
func (t *TestEditorHandler) SeekOffset() int {
	return 0
}

// MaxSeekOffset satisfies component.Scrollable.
func (t *TestEditorHandler) MaxSeekOffset() int {
	return 0
}

func (e *TestEditor) Edit(
	resource workspaceapi.URI, buf *cell.Buffer, readOnly, recovered bool,
) (text.Handler, error) {
	e.uri = resource
	e.buf = buf

	h := &TestEditorHandler{
		uri:         resource,
		parent:      e,
		TestHandler: *browsertest.NewTestHandler(),
	}
	e.dispatchEvent(context.Background(), textapi.Event{
		Type:     textapi.EventTypeOpen,
		URI:      resource,
		Resource: h,
		Content:  buf.String(),
	})

	subs := text.CellSubscriber(resource, h, e)
	buf.Subscribe(subs)
	return h, nil
}

func (t *TestEditorHandler) SetLocationList(
	pri textapi.LocationPriority, id string, loc text.LocationList,
) {
	t.LocationList = loc
}

func (e *TestEditorHandler) Handle(ev term.Event) (bool, bool) {
	e.parent.dispatchEvent(context.Background(), textapi.Event{
		Type:     textapi.EventTypeCursor,
		URI:      e.uri,
		Resource: e,
	})
	if ev.Ch == 'v' {
		e.parent.dispatchEvent(context.Background(), textapi.Event{
			Type:     textapi.EventTypeSelection,
			URI:      e.uri,
			Resource: e,
		})
	}
	return e.TestHandler.Handle(ev)
}

func (e *TestEditorHandler) Resize(width, height int) {
	e.Width, e.Height = width, height
	e.TestHandler.Resize(width, height)
}

func (e *TestEditorHandler) MoveToNextLocation(ID string) bool {
	return false
}

func (e *TestEditorHandler) MoveToPrevLocation(ID string) bool {
	return false
}

func (t *TestEditorHandler) SetCursorAtScroll(pos term.Coordinates) bool {
	t.CursorPos = pos
	return true
}

func (t *TestEditorHandler) CursorAtScroll() term.Coordinates {
	return t.CursorPos
}

func (t *TestEditorHandler) CellEditor() cell.Editor {
	return t.parent.buf.Editor()
}

func (t *TestEditorHandler) CellView() cell.View {
	return t.parent.buf.View()
}

func (e *TestEditor) SubscribeCommand(cmd textapi.CommandManual, h text.CommandHandler) error {
	return nil
}

func (e *TestEditor) UnsubscribeCommand(cmd string) error {
	return nil
}

func (h *TestEditorHandler) LocationLists() []text.LocationSet {
	return nil
}

func (t *TestEditorHandler) SetDefaultAttributes(attr term.Attributes) {
	t.Attributes.Fg = attr.Fg
	t.Attributes.Bg = attr.Bg
}

func (e *TestEditor) UnsubscribeEvents(
	sub text.EventHandler,
) (ret bool, err error) {
	final := make(map[textapi.EventType][]text.EventHandler)
	for ev, subs := range e.subs {
		final[ev] = make([]text.EventHandler, 0, len(subs))
		for _, s := range subs {
			if s != sub {
				final[ev] = append(final[ev], s)
			} else {
				ret = true
			}
		}
	}
	e.subs = final
	return
}

func (e *TestEditor) SubscribeEvents(
	evs []textapi.EventType, sub text.EventHandler,
) error {
	if e.subs == nil {
		e.subs = make(map[textapi.EventType][]text.EventHandler)
	}
	for _, ev := range evs {
		if _, ok := e.subs[ev]; !ok {
			e.subs[ev] = []text.EventHandler{sub}
		} else {
			e.subs[ev] = append(e.subs[ev], sub)
		}
	}
	if e.cb != nil {
		e.cb()
	}
	return nil
}

func (e *TestEditor) Subscribers() map[textapi.EventType][]text.EventHandler {
	return e.subs
}

func (e *TestEditor) Editor(file workspaceapi.URI) (text.Handler, error) {
	return nil, errors.New("nope")
}
