package browser

//go:generate mockgen -destination=./browser_gomock.go -package browser -self_package browser -source browser.go

import (
	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/term"
)

// Handler adds io.Closer to a tui.Handler.
type Handler interface {
	tui.Handler
	OnUnmount() error
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

// WindowManager is the interface that groups tile
// window management methods.
type WindowManager interface {
	Focus() (Window, error)
	SplitVerticalRight(Handler) (Window, error)
	SplitVerticalLeft(Handler) (Window, error)
	SplitHorizontalAbove(Handler) (Window, error)
	SplitHorizontalBelow(Handler) (Window, error)
	FloatingWindow(h Handler, at term.Coordinates, width, height int) (Window, error)
}

// EventHandler wraps the basic tui.Handler method Handle.
type EventHandler interface {
	Handle(term.Event) (exit bool)
}

// EventSubscriber handler is the interface that wraps
// the method Subscribe which allows clients to subscribe to
// specific events.
type EventSubscriber interface {
	Subscribe(term.Event, EventHandler) error
}

// KeyMapper is the interface that wraps the method MergeKeyMap
// to merge new key mappings.
type KeyMapper interface {
	MergeKeyMap(map[term.Event]term.Event) error
}

// Messenger is the interface that wraps methods to display
// messages to the user.
type Messenger interface {
	SetMessage(msg string, args ...interface{}) error
}

// ResourceOpener is the interface that wraps the method Open.
type ResourceOpener interface {
	Open(resource string) (Handler, error)
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
	EventSubscriber
	EventPublisher
	KeyMapper
	ResourceOpener
	Messenger
	Storage
	// io.Closer by means of Storage
}

type unmountHandler struct {
	tui.Handler
	onUnmount func()
}

func (h *unmountHandler) OnUnmount() error {
	h.onUnmount()
	return nil
}

// CallbackHandler returns a Handler by wrapping a tui.Handler
// with an OnUnmount callback.
func CallbackHandler(h tui.Handler, onUnmount func()) Handler {
	return &unmountHandler{Handler: h, onUnmount: onUnmount}
}

// NopHandler returns a Handler by wrapping a tui.Handler
// with an nop OnUnmount callback.
func NopHandler(h tui.Handler) Handler {
	return &unmountHandler{Handler: h, onUnmount: func() {}}
}

type fnEventHandler func(term.Event) bool

// Handle satisfies EventHandler
func (h fnEventHandler) Handle(ev term.Event) bool {
	return h(ev)
}

// FuncEventHandler wraps fn to satisfy EventHandler.
func FuncEventHandler(fn func(term.Event) bool) EventHandler {
	return fnEventHandler(fn)
}

// NopEventHandler returns an EventHandler that does nothing.
func NopEventHandler(fn func(term.Event) bool) EventHandler {
	return FuncEventHandler(func(term.Event) bool { return false })
}
