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

package vtetest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Case is a vte test case.
type Case struct {
	InputSequence string
	Expected      string
}

// TestSequence tests the given handler with the given test cases.
// This differs from the default testing tui.Handler harness' in that
// it waits up to drawTimeout for interruptChan to stop sending requests
// to advance to the test case's draw and assertions.
//
// This is mostly useful for asynchronous tui.Handler that are 'ready'
// for assertion when idle for drawTimeout, as defined by not sending interrupt
// requests.
func TestSequence(
	t *testing.T, handler tui.Handler, width, height int,
	drawTimeout time.Duration, interruptChan chan struct{},
	cases []Case,
) {
	writer := term.NewStringWriter(width, height)
	handler.Resize(width, height)

	// wait for first handler to initialize,
	// first interrupt should be a good indicator
	<-interruptChan

	for i, tcase := range cases {
		handleTestCase(t, i, writer, handler, tcase,
			width, height, drawTimeout, interruptChan)
	}
}

// TestCases tests the given handler with the given test cases,
// but assumes that it's an already initialized handler,
// so it doesn't Resize or wait for initial interrupt.
func TestCases(
	t *testing.T, handler tui.Handler, width, height int,
	drawTimeout time.Duration, interruptChan chan struct{},
	cases []Case,
) {
	writer := term.NewStringWriter(width, height)
	for i, tcase := range cases {
		handleTestCase(t, i, writer, handler, tcase,
			width, height, drawTimeout, interruptChan)
	}
}

func handleTestCase(
	t *testing.T, i int, w *term.StringWriter,
	h tui.Handler, tcase Case, width, height int,
	drawTimeout time.Duration,
	interruptChan chan struct{},
) {
	t.Helper()
	err := w.Clear(term.Attributes{})
	require.NoError(t, err)

	// vte needs Raw field set
	callHandle := func(ev term.Event) {
		switch ev.Mod {
		case 0:
			switch ev.Key {
			case term.KeyBackspace:
				ev.Raw = []byte("\x7f")
			case term.KeyEnter:
				ev.Raw = []byte("\x0A")
			case term.KeyEsc:
				ev.Raw = []byte("\x1b")
			case term.KeyTab:
				ev.Raw = []byte("\x09")
			case term.KeyArrowDown:
				ev.Raw = []byte("\x50")
			case term.KeyArrowUp:
				ev.Raw = []byte("\x48")
			case term.KeyF1:
				ev.Raw = []byte("\x1bOP")
			case term.KeyF2:
				ev.Raw = []byte("\x1bOQ")
			case term.KeyF3:
				ev.Raw = []byte("\x1bOR")
			case term.KeyF4:
				ev.Raw = []byte("\x1bOS")
			case term.KeyF5:
				ev.Raw = []byte("\x1b[15~")
			case term.KeyF6:
				ev.Raw = []byte("\x1b[17~")
			case term.KeyF7:
				ev.Raw = []byte("\x1b[18~")
			case term.KeyF8:
				ev.Raw = []byte("\x1b[19~")
			case term.KeyF9:
				ev.Raw = []byte("\x1b[20~")
			case term.KeyF10:
				ev.Raw = []byte("\x1b[21~")
			case term.KeyF11:
				ev.Raw = []byte("\x1b[22~")
			case term.KeyF12:
				ev.Raw = []byte("\x1b[23~")
			case term.KeyInsert:
				ev.Raw = []byte("\x1b[2~")
			case term.KeyDelete:
				ev.Raw = []byte("\x1b[3~")
			default:
				ev.Raw = []byte(string(ev.Ch))
			}
		case term.ModCtrl:
			switch ev.Ch {
			case '\\':
				ev.Raw = []byte("\x1C")
			case 'v':
				ev.Raw = []byte("\x16")
			case 'l':
				ev.Raw = []byte("\x0c")
			default:
				t.Logf("WARNING: could not find raw vte sequence for input event: %+v", ev)
			}
		}
		h.Handle(ev)
	}

	var escapeNext bool
	for _, r := range tcase.InputSequence {
		if escapeNext {
			escapeNext = false
			callHandle(term.Event{Ch: r, Type: term.EventKey})
			continue
		}
		switch r {
		case '^':
			callHandle(term.Event{Key: term.KeyBackspace, Type: term.EventKey})
		case '#':
			callHandle(term.Event{Mod: term.ModCtrl, Ch: 'c', Type: term.EventKey})
		case '$':
			callHandle(term.Event{Mod: term.ModCtrl, Ch: 'l', Type: term.EventKey})
		case '>':
			callHandle(term.Event{Key: term.KeyEnter, Type: term.EventKey})
		case '<':
			callHandle(term.Event{Key: term.KeyEsc, Type: term.EventKey})
		case '✌':
			callHandle(term.Event{Key: term.KeyTab, Type: term.EventKey})
		case '⬇':
			callHandle(term.Event{Key: term.KeyArrowDown, Type: term.EventKey})
		case '⬆':
			callHandle(term.Event{Key: term.KeyArrowUp, Type: term.EventKey})
		case '\\':
			escapeNext = true
		default:
			callHandle(term.Event{Ch: r, Type: term.EventKey})
		}

		timer := time.NewTimer(drawTimeout)
	loop:
		for {
			select {
			case <-timer.C:
				break loop
			case <-interruptChan:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(drawTimeout)
			}
		}
	}

	h.Draw(w)

	cursor, _, ok := h.Cursor()
	if ok {
		w.SetCursor(cursor)
	}

	err = w.Flush()
	require.NoError(t, err)

	out := w.String()
	assert.Equal(t, tcase.Expected, out, "test case %d", i)
}
