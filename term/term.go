//go:build !js

package term

import (
	"sync/atomic"

	"github.com/ernestrc/tcell/v3"
	"github.com/ernestrc/tcell/v3/termbox"
)

var (
	defaultAttr  = Attributes{Fg: tcell.ColorDefault, Bg: tcell.ColorDefault}
	publishEvent atomic.Value

	// DefaultWriter returns the global terminal Writer.
	DefaultWriter ContextWriter = newTermboxWriter()
)

func init() {
	publishEvent.Store(func(termbox.Event) bool { return false })
}

// SetInputMode sets termbox input mode. Termbox has two input modes:
//
// 1. Esc input mode. When ESC sequence is in the buffer and it doesn't match
// any known sequence. ESC means KeyEsc. This is the default input mode.
//
// 2. Alt input mode. When ESC sequence is in the buffer and it doesn't match
// any known sequence. ESC enables ModAlt modifier for the next keyboard event.
//
// Both input modes can be OR'ed with Mouse mode. Setting Mouse mode bit up will
// enable mouse button press/release and drag events.
//
// If 'mode' is InputCurrent, returns the current input mode. See also Input*
// constants.
func SetInputMode(mode InputMode) InputMode {
	return InputMode(termbox.SetInputMode(termbox.InputMode(mode)))
}

// SetAttr sets the global foreground and background attributes.
func SetAttr(newattr Attributes) {
	defaultAttr = newattr
}

// Attr returns the global foreground and background attributes.
func Attr() Attributes {
	return defaultAttr
}

// Init Initializes writer.
// This function should be called before any other functions.
// After successful initialization, the writer must be finalized using 'Close'
// function.
func Init() error {
	err := termbox.Init()
	if err != nil {
		return err
	}
	screen := termbox.Screen()
	screen.EnablePaste()
	screen.EnableFocus()
	publishEvent.Store(termbox.PublishEvent)
	return nil
}

// Size returns the size of the terminal window.
func Size() (width int, height int) {
	return termbox.Size()
}

func makeEvent(tev termbox.Event) (ev Event) {
	ev.Type = EventType(tev.Type)
	ev.Mod = Modifier(tev.Mod)
	ev.Key = Key(tev.Key)
	ev.Ch = tev.Ch
	ev.Width = tev.Width
	ev.Height = tev.Height
	ev.Err = tev.Err
	ev.MouseX = tev.MouseX
	ev.MouseY = tev.MouseY
	ev.Raw = tev.Raw
	if tev.Metadata != nil {
		ev.UserFunc = tev.Metadata.(func())
	}
	return
}

// PollEvent waits for an event and returns it.
// This is a blocking function call.
func PollEvent() (ev Event) {
	tev := termbox.PollEvent()
	return makeEvent(tev)
}

// Close writer; should be called after successful initialization
// when termbox's functionality isn't required anymore.
func Close() {
	termbox.Close()
}

// DisableInterruptForTesting disables interrupts. It should only be
// used for testing purposes.
func DisableInterruptForTesting() {
	publishEvent.Store(func(termbox.Event) bool {
		return false
	})
}

// PublishEvent sends a synthetic event to the event poller.
// If the event queue is full then this method does not
// publish the event and returns false.
func PublishEvent(ev Event) bool {
	tev := termbox.Event{}
	tev.Type = termbox.EventType(ev.Type)
	tev.Mod = termbox.Modifier(ev.Mod)
	tev.Key = termbox.Key(ev.Key)
	tev.Ch = ev.Ch
	tev.Width = ev.Width
	tev.Height = ev.Height
	tev.Err = ev.Err
	tev.MouseX = ev.MouseX
	tev.MouseY = ev.MouseY
	tev.Raw = ev.Raw
	tev.Metadata = ev.UserFunc
	return publishEvent.Load().(func(termbox.Event) bool)(tev)
}

// RingBell makes an audible noise. This must be synchronized
// against other accesses to the term.Writer's screen buffer.
func RingBell() {
	termbox.Screen().Bell()
}

// PublishBell It's a shorthand for `ScheduleNextTick(RingBell)`.
func PublishBell() {
	ScheduleNextTick(RingBell)
}

// ScheduleNextTick schedules running fn on the next event-loop iteration.
func ScheduleNextTick(fn func()) bool {
	return PublishEvent(Event{Type: EventInterrupt, UserFunc: fn})
}

// Poll gives access to the underlying tcell.Event channel.
func Poll() <-chan tcell.Event {
	return termbox.Screen().Poll()
}

// FromTcellEvent converts a tcell.Event into a term.Event.
func FromTcellEvent(tev tcell.Event) Event {
	return makeEvent(termbox.NewEvent(tev))
}

// CursorStyle represents a given cursor style, which can include the shape and
// whether the cursor blinks or is solid.  Support for changing this is not universal.
type CursorStyle int

const (
	CursorStyleDefault           = CursorStyle(tcell.CursorStyleDefault)
	CursorStyleBlinkingBlock     = CursorStyle(tcell.CursorStyleBlinkingBlock)
	CursorStyleSteadyBlock       = CursorStyle(tcell.CursorStyleSteadyBlock)
	CursorStyleBlinkingUnderline = CursorStyle(tcell.CursorStyleBlinkingUnderline)
	CursorStyleSteadyUnderline   = CursorStyle(tcell.CursorStyleSteadyUnderline)
	CursorStyleBlinkingBar       = CursorStyle(tcell.CursorStyleBlinkingBar)
	CursorStyleSteadyBar         = CursorStyle(tcell.CursorStyleSteadyBar)
)

// SetCursorStyle is used to set the cursor style. If the style
// is not supported (or cursor styles are not supported at all),
// then this will have no effect.
func SetCursorStyle(style CursorStyle) {
	termbox.Screen().SetCursorStyle(tcell.CursorStyle(style))
}
