package browser

import (
	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
)

// Handler adds Close to a tui.Handler.
type Handler interface {
	tui.Handler
	Close() error
}

// Window is the interface that represents
// a closeable window in a WindowManager.
type Window interface {
	SetContent(Handler) error
	Content() (Handler, error)

	// Close closes the window.
	Close() error

	// used to cache windows and so avoid leaks
	// when same client is requesting via Focus()
	// the same window over and over.
	id() uint64
	onWindowClosed(fn func())
}

// Orientation represents a window orientation.
type Orientation uint8

const (
	OrientationDefault Orientation = iota
	OrientationTop
	OrientationBottom
	OrientationLeft
	OrientationRight
)

// WindowManager is the interface that groups tile
// window management methods.
type WindowManager interface {
	// Focus returns the current Window in focus.
	Focus() (Window, error)

	// SetFocus sets win to be the Window in focus and returns the
	// previous window in focus.
	SetFocus(win Window) (Window, error)

	// Split splits the current window in focus in two, and installs
	// Handler in the new window.
	Split(Orientation, Handler) (Window, error)

	// Floating creates a new floating window at coordinates,
	// with static width and height.
	Floating(h Handler, at term.Coordinates, width, height int) (Window, error)

	// Bar creates a status bar with Orientation and Handler.
	// Bars differ from Split and Floating windows in that they can't
	// be in focus and can only receive mouse events.
	Bar(Orientation, tui.Handler) error

	// Tab creates a new tab with h and returns a handle that can be
	// used with the rest of methods that take a browser.Handler.
	// URI is used to uniquely identify a tab and name is used as a label
	// to display it in the tab bar.
	Tab(uri workspace.URI, name string, h Handler) (Handler, error)
}

// Messenger is the interface that wraps methods to display
// messages to the user.
type Messenger interface {
	SetMessage(msg string, args ...interface{}) error
}

// ResourceOpener is the interface that wraps the method Open.
type ResourceOpener interface {
	Open(resource workspace.URI) (Handler, error)
}

// EventPublisher is the interface that wraps the method PublishInterrupt.
type EventPublisher interface {
	// PublishInterrupt will publish an interrupt event, which will force
	// redrawing all components in the terminal.
	PublishInterrupt() error
}

// Storage is the interface that wraps persistence CRUD methods.
// See document.Service for more details.
type Storage interface {
	document.Service
}

// Browser is an interface that groups methods to manipulate
// the user interface of a browser.
type Browser interface {
	WindowManager
	EventPublisher
	ResourceOpener
	Messenger
	Storage
	// io.Closer by means of Storage
}

type closeHandler struct {
	tui.Handler
	doClose func()
}

func (h *closeHandler) Close() error {
	h.doClose()
	return nil
}

// FuncHandler returns a Handler by wrapping a tui.Handler
// with an Close callback.
func FuncHandler(h tui.Handler, doClose func()) Handler {
	return &closeHandler{Handler: h, doClose: doClose}
}

// NopHandler returns a Handler by wrapping a tui.Handler
// with an nop Close callback.
func NopHandler(h tui.Handler) Handler {
	return &closeHandler{Handler: h, doClose: func() {}}
}

type fnEventHandler func(term.Event) bool

// Handle satisfies EventHandler
func (h fnEventHandler) Handle(ev term.Event) bool {
	return h(ev)
}
