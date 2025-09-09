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

package workspaceapi

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui/debug"
)

// ChanProcessWatcher returns a ProcessWatcher that simply returns
// ch when Watch is called.
func ChanProcessWatcher(ch chan error) ProcessWatcher {
	return waitCh(ch)
}

// MultiProcessWatcher returns a ProcessWatcher that ensures that
// all the given watchers get notified when the process is done.
func MultiProcessWatcher(watchers ...ProcessWatcher) ProcessWatcher {
	if len(watchers) == 0 {
		panic("watchers cannot be empty")
	}
	return newMultiWatcher(watchers)
}

type waitCh chan error

func (w waitCh) WatchProcess() chan error {
	return w
}

type multiWatcher struct {
	ch chan error
}

func newMultiWatcher(watchers []ProcessWatcher) ProcessWatcher {
	ret := &multiWatcher{ch: make(chan error)}
	go debug.CapturePanicReport(func() {
		const watcherWaitTimeout = 30 * time.Second
		err := <-ret.ch
		ctx, cancel := context.WithTimeout(context.Background(),
			watcherWaitTimeout)
		defer cancel()

		var wg sync.WaitGroup
		for _, watcher := range watchers {
			ch := watcher.WatchProcess()
			if ch == nil {
				continue
			}
			wg.Add(1)
			go debug.CapturePanicReport(func() {
				defer wg.Done()
				select {
				case ch <- err:
				case <-ctx.Done():
					log.Warnf("could not deliver error to watcher chan: " +
						"watcher not ready for too long")
				}
			})
		}
		wg.Wait()
	})
	return ret
}

func (w *multiWatcher) WatchProcess() chan error {
	return w.ch
}
