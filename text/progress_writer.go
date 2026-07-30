// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package text

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// NewNotifyProgressWriter returns a repl.ProgressWriter that pins
// a notification on the first sample and updates its progress on
// every subsequent sample. The notification message is rendered
// as "<message>: <progress>/<total> <units>", or "<message>: X%"
// when units is empty. interrupter is used to trigger a redraw
// after each update so the progress UI refreshes promptly.
//
// The Notify and UpdateNotificationProgress calls hop onto the host
// event loop via scheduleNextTick so notis.inFocus reads
// workspaceManagerHandler.focus on the goroutine that mutates it;
// without that hop, progress samples fired from the download
// goroutine race workspace switch / close.
func NewNotifyProgressWriter(
	n browserapi.Notifications,
	interrupter term.Interrupter,
	message string,
	scheduleNextTick func(func()) bool,
) repl.ProgressWriter {
	return &notifyProgressWriter{
		n: n, interrupter: interrupter, message: message,
		scheduleNextTick: scheduleNextTick,
	}
}

type notifyProgressWriter struct {
	n                browserapi.Notifications
	interrupter      term.Interrupter
	message          string
	scheduleNextTick func(func()) bool

	mu       sync.Mutex
	notifID  string
	lastEmit time.Time
	lastUnit string
}

// notifyProgressInterval throttles intermediate progress samples
// to avoid pegging the host event loop when the source emits many
// updates in tight succession. Boundary samples (progress==total)
// and phase changes (new units string) always emit.
const notifyProgressInterval = 50 * time.Millisecond

func (w *notifyProgressWriter) Progress(progress, total int64, units string) {
	if total <= 0 || progress > total {
		return
	}

	var sample string
	if units == "" {
		perc := int(float64(progress) / float64(total) * 100)
		sample = fmt.Sprintf("%d%%", perc)
	} else {
		sample = fmt.Sprintf("%d/%d %s", progress, total, units)
	}

	w.mu.Lock()
	now := time.Now()
	final := progress == total
	phaseChange := units != w.lastUnit
	if !final && !phaseChange && now.Sub(w.lastEmit) < notifyProgressInterval {
		w.mu.Unlock()
		return
	}
	w.lastEmit = now
	w.lastUnit = units
	w.mu.Unlock()

	w.scheduleNextTick(func() {
		w.mu.Lock()
		if w.notifID == "" {
			id, err := w.n.Notify(browserapi.LevelInfo, "%s", w.message)
			if err != nil {
				w.mu.Unlock()
				log.WithError(err).Warn(
					"text: notify progress start")
				return
			}
			w.notifID = id
		}
		id := w.notifID
		w.mu.Unlock()

		if err := w.n.UpdateNotificationProgress(
			id, fmt.Sprintf("%s: %s", w.message, sample),
			progress, total,
		); err != nil {
			// the notification routinely expires from display
			// while progress updates are still arriving
			log.WithError(err).Debug("text: update notification progress")
		}
	})
	if w.interrupter != nil {
		if err := w.interrupter.Interrupt(context.Background()); err != nil {
			log.WithError(err).Warn("text: interrupt")
		}
	}
}
