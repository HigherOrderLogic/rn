//go:build !js

package term

import (
	"sync/atomic"

	"github.com/ernestrc/tcell/v2"
	"github.com/ernestrc/tcell/v2/termbox"
)

var (
	defaultAttr = Attributes{Fg: ColorDefault, Bg: ColorDefault}

	// DefaultWriter returns the global terminal Writer.
	DefaultWriter = new(termboxWriter)

	publishEvent atomic.Value
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

// SetOutputMode sets the output mode. This library has four output options:
//
//  1. OutputNormal => [1..8]
//     This mode provides 8 different colors:
//     black, red, green, yellow, blue, magenta, cyan, white
//     Shortcut: ColorBlack, ColorRed, ...
//     Attributes: AttrBold, AttrUnderline, AttrReverse
//
//     Example usage:
//     SetCell(x, y, '@', ColorBlack | AttrBold, ColorRed);
//
//  2. Output256 => [1..256]
//     In this mode you can leverage the 256 terminal mode:
//     0x01 - 0x08: the 8 colors as in OutputNormal
//     0x09 - 0x10: Color* | AttrBold
//     0x11 - 0xe8: 216 different colors
//     0xe9 - 0x1ff: 24 different shades of grey
//
//     Example usage:
//     SetCell(x, y, '@', 184, 240);
//     SetCell(x, y, '@', 0xb8, 0xf0);
//
//  3. Output216 => [1..216]
//     This mode supports the 3rd range of the 256 mode only.
//     But you don't need to provide an offset.
//
//  4. OutputGrayscale => [1..26]
//     This mode supports the 4th range of the 256 mode
//     and black and white colors from 3th range of the 256 mode
//     But you don't need to provide an offset.
//
// In all modes, 0x00 represents the default color.
//
// If 'mode' is OutputCurrent, it returns the current output mode.
//
// Note that this may return a different OutputMode than the one requested,
// as the requested mode may not be available on the target platform.
func SetOutputMode(mode OutputMode) OutputMode {
	return OutputMode(termbox.SetOutputMode(termbox.OutputMode(mode)))
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
	return publishEvent.Load().(func(termbox.Event) bool)(tev)
}

// HasPendingEvent returns true if PollEvent would return an event
// without blocking.  If the screen is stopped and PollEvent would
// return nil, then the return value from this function is unspecified.
// The purpose of this function is to allow multiple events to be collected
// at once, to minimize screen redraws.
func HasPendingEvent() bool {
	return termbox.HasPendingEvent()
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
