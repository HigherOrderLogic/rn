package browser

import (
	"io"

	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/term"
)

// Tab is a structure that represents a tab in a Browser.Component.
// It satisfies browser.Handler interface so it can be used
// with browser.Browser API. See browser.Component.NewTab for more details.
type Tab struct {
	parent      *Component
	uri         workspaceapi.URI
	closer      io.Closer
	handler     browserapi.Handler
	free        bool
	win         Window
	subscribers []TabSubscriber
}

// newTab allocates storage for a new tab and initializes it.
func newTab(c *Component, uri workspaceapi.URI, h browserapi.Handler, f io.Closer) *Tab {
	ret := new(Tab)
	ret.init(c, uri, h, f)
	return ret
}

// Init initializes this tab with id, h as the Handler, and f as the
// io.Closer handle.
func (b *Tab) init(c *Component, uri workspaceapi.URI, h browserapi.Handler, f io.Closer) {
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

func (b *Tab) setWindow(win Window) {
	b.free = false
	b.win = win
}

func (b *Tab) setFree() {
	b.free = true
	b.win = nil
	for _, sub := range b.subscribers {
		sub.OnFree(b)
	}
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

// Cursor satisfies tui.Handler
func (b *Tab) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return b.handler.Cursor()
}

// Man satisfies tui.Handler
func (b *Tab) Man() tui.Manual {
	return b.handler.Man()
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
