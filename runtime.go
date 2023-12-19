package tui

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/tcell/v2"
	"unstable.build/go-tui/term"
)

const exitSignalDuration = 1 * time.Second

func redraw(
	root Handler, lock sync.Locker, termw term.Writer,
	prevCursor term.CursorStyle,
) (term.CursorStyle, error) {
	// TODO Attr should be removed and Clear should no take any parameters
	if err := termw.Clear(term.Attr()); err != nil {
		return 0, err
	}

	lock.Lock()
	root.Draw(termw)
	cursor, style, show := root.Cursor()
	lock.Unlock()

	if show {
		termw.SetCursor(cursor)
		if style != prevCursor {
			term.SetCursorStyle(style)
		}
	} else {
		termw.SetCursor(term.Coordinates{X: -1, Y: -1})
	}

	if err := termw.Flush(); err != nil {
		return 0, err
	}

	return style, nil
}

func drain(evs <-chan tcell.Event) {
	for {
		select {
		case <-evs:
		default:
			return
		}
	}
}

// batch interrupt events such that we deliver exactly one more
// after every call to publish interrupt.
// This serves as a pressure valve when something is abusing
// the interrupt mechanism.
var interruptPending atomic.Bool

// PublishEvent publishes the given event to the event loop.
func PublishEvent(ev term.Event) bool {
	if ev.Type == term.EventInterrupt &&
		!interruptPending.CompareAndSwap(false, true) {
		return true
	}
	return term.PublishEvent(ev)
}

func handleInterruptSignal(
	lastSignalAt *time.Time, exit *bool,
) {
	now := time.Now()
	*exit = now.Sub(*lastSignalAt) < exitSignalDuration
	*lastSignalAt = now
}

func run(root Handler, lock sync.Locker, termw term.Writer) (err error) {
	width, height := term.Size()

	lock.Lock()
	root.Resize(width, height)
	lock.Unlock()

	evs := term.Poll()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)

	var handled, exit bool
	var lastSignalAt time.Time
	var prevCursor term.CursorStyle
	for !exit && err == nil {
		// reset interrupts so we don't stay forever in pending mode
		interruptPending.Store(false)
		if prevCursor, err = redraw(root, lock, termw, prevCursor); err != nil {
			return
		}

		for {
			select {
			case <-sigs:
				drain(evs)
				handleInterruptSignal(&lastSignalAt, &exit)
			case tev := <-evs:
				ev := term.FromTcellEvent(tev)
				switch ev.Type {
				case term.EventInterrupt:
				case term.EventError:
					err = ev.Err
				case term.EventResize:
					width, height := ev.Width, ev.Height
					lock.Lock()
					root.Resize(width, height)
					lock.Unlock()
				default:
					lock.Lock()
					exit, handled = root.Handle(ev)
					lock.Unlock()
					if ev.Key == term.KeyCtrlC && !handled {
						drain(evs) // avoid deadlocks with tcell's screen
						handleInterruptSignal(&lastSignalAt, &exit)
					}
				}
			}
			if exit || len(evs) == 0 {
				break
			}
		}
	}

	return err
}
