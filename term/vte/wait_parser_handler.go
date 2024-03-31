package vte

import (
	"time"

	"unstable.build/go-tui/term/vte/parser"
)

var _ parser.Handler = (*waitParserHandler)(nil)

type waitParserHandler struct {
	ch      chan func()
	trigger func()
	parser.Handler
}

func newWaitParserHandler(h parser.Handler) *waitParserHandler {
	ret := &waitParserHandler{
		// NOTE: The channel size MUST BE smaller than the default
		// event-loop channel size so we stop processing callbacks
		// before event loop callbacks get backed up and bell
		// cannot be triggered anymore.
		ch:      make(chan func(), 10),
		Handler: h,
	}
	return ret
}
func (w *waitParserHandler) useTrigger(trigger func()) {
	w.trigger = trigger
}

func (w *waitParserHandler) scheduleBellCallback(
	timeout time.Duration, callback func(),
) (ok bool) {
	if w.trigger == nil {
		panic("must install first a trigger via useTrigger")
	}

	first := len(w.ch) == 0
	select {
	case w.ch <- callback:
		if first && len(w.ch) == 1 {
			w.trigger()
		}
		ok = true
	default:
		timer := time.NewTimer(timeout)
		// channel might be full, schedule a bell trigger to
		// attempt to unblock. Wait to see if unblock succeeded
		// before giving up.
		w.trigger()
		select {
		case w.ch <- callback:
			// do not trigger here, if the channel was full
			// then Bell should take care of continuing dispatching
			ok = true
		case <-timer.C:
		}
	}

	return
}

func (w *waitParserHandler) Bell() {
	isLast := len(w.ch) == 1
	select {
	case cb := <-w.ch:
		cb()
		// a timeout could indicate that the trigger didn't
		// really work temporarily, so re-triggering is a way to clean up
		// and either force an audible bell, or continue processing tasks
		if lenCh := len(w.ch); !isLast && lenCh > 0 {
			w.trigger()
		}
	default:
		w.Handler.Bell()
	}
}
