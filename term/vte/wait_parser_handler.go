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

package vte

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term/vte/vteparser"
)

var _ vteparser.Handler = (*waitParserHandler)(nil)

type waitParserHandler struct {
	ch      chan func()
	trigger func()
	vteparser.Handler
}

func newWaitParserHandler(ctx context.Context, h vteparser.Handler) *waitParserHandler {
	ret := &waitParserHandler{
		// NOTE: The channel size MUST BE smaller than the default
		// event-loop channel size so we stop processing callbacks
		// before event loop callbacks get backed up and bell
		// cannot be triggered anymore.
		ch:      make(chan func(), 50),
		Handler: h,
	}
	go debug.CapturePanicReport(func() {
		ret.monitorStarvation(ctx)
	})
	return ret
}
func (w *waitParserHandler) useTrigger(trigger func()) {
	w.trigger = trigger
}

func (w *waitParserHandler) monitorStarvation(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	var prevLength int
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currLength := len(w.ch)
			// if no events were drained for a second,
			// it's possible bell cannot be triggered: drain events to relieve
			// event loop
			if prevLength > 0 && currLength == prevLength && w.trigger != nil {
				// drain
				w.log(log.DebugLevel, "scheduler is not making progress: draining %d events", len(w.ch))
				for {
					select {
					case <-w.ch:
						continue
					default:
					}
					break
				}
			}
			prevLength = currLength
		}
	}
}

func (w *waitParserHandler) pendingCallbacks() int {
	return len(w.ch)
}

func (w *waitParserHandler) scheduleBellCallback(callback func()) (ok bool) {
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
		// don't block
	}

	return
}

func (w *waitParserHandler) Bell() {
	isLast := len(w.ch) == 1
	select {
	case cb := <-w.ch:
		cb()
		if lenCh := len(w.ch); !isLast && lenCh > 0 {
			w.trigger()
		}
	default:
		w.Handler.Bell()
	}
}

func (v *waitParserHandler) log(level log.Level, line string, params ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "vte.waitParserHandler").
		Logf(level, line, params...)
}
