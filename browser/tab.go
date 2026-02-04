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

package browser

import (
	"io"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var _ component.Scrollable = (*Tab)(nil)

// Tab is a structure that represents a tab in a Browser.Component.
// It satisfies browser.Handler interface so it can be used
// with browser.Browser API. See browser.Component.NewTab for more details.
//
// If the underlying browser.Handler of a Tab satisfies component.Scrollable,
// then Tab satisfies it too; otherwise all the component.Scrollable methods
// are no-op.
type Tab struct {
	parent      *Component
	uri         workspaceapi.URI
	closer      io.Closer
	handler     browserapi.Handler
	free        bool
	win         Window
	subscribers []TabSubscriber
	prev        *Tab
}

// Resize satisfies tui.Component
func (b *Tab) Resize(width, height int) {
	b.handler.Resize(width, height)
}

// Draw satisfies tui.Component
func (b *Tab) Draw(w term.Writer) {
	b.handler.Draw(w)
}

// Handle satisfies tui.Handler
func (b *Tab) Handle(ev term.Event) (exit, handled bool) {
	// ignore exit, a tab is managed manually by user
	_, handled = b.handler.Handle(ev)
	return
}

// Cursor satisfies tui.Handler.
func (b *Tab) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return b.handler.Cursor()
}

// Selection satisfies tui.Handler.
func (b *Tab) Selection() (string, bool) {
	return b.handler.Selection()
}

// Close satisfies browser.Handler.
func (b *Tab) Close() error {
	err1 := b.handler.Close()
	if b.closer != nil {
		err2 := b.closer.Close()
		b.closer = nil
		return err2
	}
	return err1
}

// URI returns the identifier of this tab.
func (b *Tab) URI() workspaceapi.URI {
	return b.uri
}

// SetAttrs sets this tab's name and tab name attributes.
func (b *Tab) SetAttrs(name string, attrs term.Attributes) {
	b.parent.SetTabAttrs(b.uri, attrs)
}

// ResetAttrs resets this tab's name and tab name attributes.
func (b *Tab) ResetAttrs() {
	b.parent.ResetTabNameAndAttrs(b.uri)
}

// Window returns this tab's Window and true or nil and false
// if this tab is not currently active on any window.
func (b *Tab) Window() (Window, bool) {
	return b.win, !b.free
}

// Handler returns the Handler responsible for drawing
// the contents of this tab.
func (b *Tab) Handler() browserapi.Handler {
	return b.handler
}

// Closer returns the closer passed to browser.Component.NewTab,
// which is used when tab is closed via
func (b *Tab) Closer() io.Closer {
	return b.closer
}

// TabSubscriber is a subscriber of tab focus or free operations.
type TabSubscriber interface {
	OnFocus(*Tab)
	OnFree(*Tab)
}

// Subscribe subscribes sub to OnFocus and OnFree operations.
func (b *Tab) Subscribe(sub TabSubscriber) {
	b.subscribers = append(b.subscribers, sub)
	if b.free {
		sub.OnFree(b)
	} else {
		sub.OnFocus(b)
	}
}

// SeekUp satisfies component.Scrollable.
func (b *Tab) SeekUp() bool {
	scrollable, ok := b.handler.(component.Scrollable)
	if !ok {
		return false
	}
	return scrollable.SeekUp()
}

// SeekDown satisfies component.Scrollable.
func (b *Tab) SeekDown() bool {
	scrollable, ok := b.handler.(component.Scrollable)
	if !ok {
		return false
	}
	return scrollable.SeekDown()
}

// SeekOffset satisfies component.Scrollable.
func (b *Tab) SeekOffset() int {
	scrollable, ok := b.handler.(component.Scrollable)
	if !ok {
		return 0
	}
	return scrollable.SeekOffset()
}

// MaxSeekOffset satisfies component.Scrollable.
func (b *Tab) MaxSeekOffset() int {
	scrollable, ok := b.handler.(component.Scrollable)
	if !ok {
		return 0
	}
	return scrollable.MaxSeekOffset()
}

// newTab allocates storage for a new tab and initializes it.
func newTab(
	c *Component, uri workspaceapi.URI, h browserapi.Handler, f io.Closer,
) *Tab {
	ret := new(Tab)
	ret.init(c, uri, h, f)
	return ret
}

// Init initializes this tab with id, h as the Handler, and f as the
// io.Closer handle.
func (b *Tab) init(
	c *Component, uri workspaceapi.URI, h browserapi.Handler, f io.Closer,
) {
	b.parent = c
	b.uri = uri
	b.closer = f
	b.handler = h
	b.free = true
}

func (b *Tab) callOnFocus() {
	for _, sub := range b.subscribers {
		sub.OnFocus(b)
	}
}

// if prev is nil, then it means that we don't want
// to record it.
func (b *Tab) setWindow(prev *Tab, win Window) {
	b.free = false
	b.win = win
	if prev != nil {
		b.prev = prev
	}
}

func (b *Tab) setFree() {
	b.free = true
	b.win = nil
	for _, sub := range b.subscribers {
		sub.OnFree(b)
	}
}
