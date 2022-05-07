//go:build !js
// +build !js

package term

import (
	"sync"

	"github.com/nsf/termbox-go"
)

var (
	quit      chan struct{}
	events    chan Event
	interrupt int32
	mu        sync.Mutex

	defaultAttr = Attributes{Fg: ColorDefault, Bg: ColorDefault}

	// DefaultWriter returns the global terminal Writer.
	DefaultWriter = new(termboxWriter)
)

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
// 1. OutputNormal => [1..8]
//    This mode provides 8 different colors:
//        black, red, green, yellow, blue, magenta, cyan, white
//    Shortcut: ColorBlack, ColorRed, ...
//    Attributes: AttrBold, AttrUnderline, AttrReverse
//
//    Example usage:
//        SetCell(x, y, '@', ColorBlack | AttrBold, ColorRed);
//
// 2. Output256 => [1..256]
//    In this mode you can leverage the 256 terminal mode:
//    0x01 - 0x08: the 8 colors as in OutputNormal
//    0x09 - 0x10: Color* | AttrBold
//    0x11 - 0xe8: 216 different colors
//    0xe9 - 0x1ff: 24 different shades of grey
//
//    Example usage:
//        SetCell(x, y, '@', 184, 240);
//        SetCell(x, y, '@', 0xb8, 0xf0);
//
// 3. Output216 => [1..216]
//    This mode supports the 3rd range of the 256 mode only.
//    But you don't need to provide an offset.
//
// 4. OutputGrayscale => [1..26]
//    This mode supports the 4th range of the 256 mode
//    and black and white colors from 3th range of the 256 mode
//    But you don't need to provide an offset.
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
	mu.Lock()
	defer mu.Unlock()

	events = make(chan Event)
	quit = make(chan struct{})

	// start polling events
	go func() {
		buf := make([]byte, 128)
		for {
			view := buf
			var tev termbox.Event
			var res Event

			switch rev := termbox.PollRawEvent(view); rev.Type {
			case termbox.EventRaw:
				view = view[:rev.N]
				for {
					ev := termbox.ParseEvent(view)
					if ev.N == 0 {
						break
					}
					tev = ev
					res.Raw = make([]byte, tev.N)
					copy(res.Raw, view[:tev.N])
					view = view[tev.N:]
				}
			case termbox.EventError:
				tev = rev
			default:
				tev = rev
			}

			res.Type = EventType(tev.Type)
			res.Mod = Modifier(tev.Mod)
			res.Key = Key(tev.Key)
			res.Ch = tev.Ch
			res.Width = tev.Width
			res.Height = tev.Height
			res.Err = tev.Err
			res.MouseX = tev.MouseX
			res.MouseY = tev.MouseY

			select {
			case <-quit:
				return
			case events <- res:
			}
		}
	}()

	return termbox.Init()
}

// Size returns the size of the terminal window.
func Size() (width int, height int) {
	return termbox.Size()
}

// PollEvent waits for an event and returns it.
// This is a blocking function call.
func PollEvent() (ev Event) {
	mu.Lock()
	if interrupt != 0 {
		interrupt = 0
		ev = Event{Type: EventInterrupt}
		mu.Unlock()
		return
	}
	mu.Unlock()
	ev = <-events
	return
}

// Close writer; should be called after successful initialization
// when termbox's functionality isn't required anymore.
func Close() {
	close(quit)
	termbox.Close()
}

// Interrupt an in-progress call to the event poller and forces redraw.
// This is useful when the root handler's has been updated by another goroutine,
// other than the main event loop goroutine.
func Interrupt() {
	mu.Lock()
	select {
	case events <- Event{Type: EventInterrupt}:
	default:
		interrupt++
	}
	mu.Unlock()
}

// SendNoneEvent sends a term.EventNone to the event poller and
// forces event handling which in turn forces redraw.
func SendNoneEvent() {
	events <- Event{Type: EventNone}
}
