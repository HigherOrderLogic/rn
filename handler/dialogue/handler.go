// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package dialogue

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
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
	interrupter term.Interrupter,
) (h tui.Handler, tx chan<- string, rx <-chan string) {
	ch1 := make(chan string)
	ch2 := make(chan string)
	mouseDelegate := newMouseDelegate(&c.messages)
	mouse := text.NewMouse(mouseDelegate)
	sh := &dialogueHandler{
		interrupter:   interrupter,
		mouse:         mouse,
		mouseDelegate: mouseDelegate,
		comp:          c,
		rx:            ch2,
		tx:            ch1,
		mu:            locker,
	}
	go debug.CapturePanicReport(sh.consumeIncoming)

	return sh, ch2, ch1
}

type dialogueHandler struct {
	*mouseDelegate
	comp        *Component
	interrupter term.Interrupter
	mouse       *text.Mouse
	tx          chan string // user messages
	rx          chan string // assistant messages
	mu          sync.Locker
}

func (h *dialogueHandler) publishInterrupt(ctx context.Context) {
	err := h.interrupter.Interrupt(ctx)
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

	switch ev.Mod {
	case 0:
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
	switch ev.Mod {
	case 0:
		switch ev.Key {
		case term.KeyArrowDown:
			handled = s.comp.SeekDown()
			return
		case term.KeyArrowUp:
			handled = s.comp.SeekUp()
			return
		}
	case term.ModCtrl:
		switch ev.Ch {
		case 'c':
			handled = true
			s.comp.Input().Reset()
		case 'k':
			handled = s.comp.SeekUp()
		case 'j':
			handled = s.comp.SeekDown()
		}
	}

	return
}

func (s *dialogueHandler) Selection() (string, bool) {
	return s.mouseDelegate.Selection()
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
	cursor = term.CoordinatesSum(cursor, s.comp.InputPosition())
	return
}

func (s *dialogueHandler) Man() tui.Manual {
	return tui.Manual{}
}

func (s *dialogueHandler) consumeIncoming() {
	ctx := context.Background()
	for msg := range s.rx {
		s.mu.Lock()
		if msg == EOM {
			s.comp.AddReceiveMessageBreak()
		} else {
			s.comp.AddReceiveMessageChunk(msg)
		}
		s.mu.Unlock()
		s.publishInterrupt(ctx)
	}
}
