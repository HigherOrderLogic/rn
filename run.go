package tui

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/tcell/v3"
	"unstable.build/go-tui/term"
)

const exitSignalDuration = 1 * time.Second

func redraw(
	root Handler, lock sync.Locker, termw term.Writer,
	prevCursor term.CursorStyle,
) (term.CursorStyle, error) {
	if err := termw.Clear(term.Attr()); err != nil {
		return 0, err
	}

	lock.Lock()
	root.Draw(termw)
	cursor, style, show := root.Cursor()
	lock.Unlock()

	if show {
		termw.SetCursor(cursor)
	} else {
		termw.SetCursor(term.Coordinates{X: -1, Y: -1})
	}
	if style != prevCursor {
		term.SetCursorStyle(style)
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
	// only conflate interrupts that have no payload
	if ev.Type == term.EventInterrupt && ev.Raw == nil &&
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

func run(root Handler, lock sync.Locker, termw term.ContextWriter) (err error) {
	ctx := context.Background()
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
	var i int64
	termw.SetContext(ContextWithIteration(ctx, i))

loop:
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
					if ev.Raw != nil {
						if id, ok := parsePayload(ev.Raw); ok {
							termw.SetContext(ContextWithIteration(ctx, id))
						} else {
							termw.SetContext(term.ContextWithPayload(ctx, ev.Raw))
						}
					} else {
						// do not set a new iteration id, instead reset to nil
						// so clients can differentiate between an interrupt
						// and a regular iteration loop.
						termw.SetContext(ctx)
					}
					// ensure that i is not incremented
					// and context is not overwritten
					continue loop
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
				i++
				termw.SetContext(ContextWithIteration(ctx, i))
				break
			}
		}
	}

	return err
}
