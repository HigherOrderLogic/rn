package dialogue

import (
	"sync"

	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
)

// EOM must be used by clients of Handler on the returned tx
// to signal the end of a message and the start of the next one.
const EOM = "\n\n\n"

// Handler wraps a dialogue.Component and provides a simple-to-use
// tui.Handler which sends input messages via rx and can
// receive messages into the dialogue history via tx.
//
// Closing the tx channel effecively closes the returned handler.
//
// EOM must be used by clients of Handler on the returned tx
// to signal the end of a message and the start of the next one.
//
// locker is used to synchronize access to c.
func Handler(
	locker sync.Locker, c *Component,
	interrupter term.Interrupter, clip clipboard.Register,
) (h tui.Handler, tx chan<- string, rx <-chan string) {
	ch1 := make(chan string)
	ch2 := make(chan string)

	mouse := text.NewMouse(newMouseDelegate(&c.messages, clip))
	sh := &dialogueHandler{
		interrupter: interrupter,
		mouse:       mouse,
		comp:        c,
		rx:          ch2,
		tx:          ch1,
		mu:          locker,
	}
	go sh.consumeIncoming()

	return sh, ch2, ch1
}

type dialogueHandler struct {
	comp        *Component
	interrupter term.Interrupter
	mouse       *text.Mouse
	tx          chan string // user messages
	rx          chan string // assistant messages
	mu          sync.Locker
}

func (h *dialogueHandler) publishInterrupt() {
	err := h.interrupter.Interrupt()
	if err != nil {
		log.WithFields(log.Fields{
			logging.KeyClass: "dialogue.handler",
			logging.KeyError: err,
		}).Error("publish interrupt")
	}
}

func (s *dialogueHandler) Draw(w term.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.comp.Draw(w)
}

func (s *dialogueHandler) Resize(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.comp.Resize(width, height)
}

func (s *dialogueHandler) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type == term.EventMouse {
		pos := s.comp.InputPosition()
		if ev.MouseY >= pos.Y {
			ev.MouseY -= pos.Y
			ev.MouseX -= pos.X
			return s.comp.Input().Handle(ev)
		}
		pos = s.comp.MessagesPosition()
		ev.MouseY -= pos.Y
		ev.MouseX -= pos.X
		return s.mouse.Handle(ev)
	}

	if ev.Type != term.EventKey {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch ev.Key {
	case term.KeyEsc:
		exit = true
	case term.KeyEnter:
		item, ok := s.comp.InputSubmit()
		if ok {
			handled = true
			s.mu.Unlock()
			s.tx <- item
			s.mu.Lock()
		}
	}

	if !handled {
		_, handled = s.comp.Input().Handle(ev)
		if handled {
			exit = false
		}
		if exit {
			handled = true
		}
	}

	if handled {
		return
	}

	// the following might or might not be handled by input
	// so we handle if and only if input has not handled them.
	switch ev.Key {
	case term.KeyArrowDown, term.KeyCtrlJ:
		handled = s.comp.SeekDown()
		return
	case term.KeyCtrlC:
		handled = true
		s.comp.Input().Reset()
	case term.KeyArrowUp, term.KeyCtrlK:
		handled = s.comp.SeekUp()
		return
	}
	return
}

func (s *dialogueHandler) Cursor() (
	cursor term.Coordinates, style term.CursorStyle, ok bool,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cursor, style, ok = s.comp.Input().Cursor()
	if !ok {
		return
	}
	cursor = cell.CoordinatesSum(cursor, s.comp.InputPosition())
	return
}

func (s *dialogueHandler) Man() tui.Manual {
	return tui.Manual{}
}

func (s *dialogueHandler) consumeIncoming() {
	for msg := range s.rx {
		s.mu.Lock()
		if msg == EOM {
			s.comp.AddReceiveMessageBreak()
		} else {
			s.comp.AddReceiveMessageChunk(msg)
		}
		s.mu.Unlock()
		s.publishInterrupt()
	}
}
