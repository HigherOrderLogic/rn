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

package ide

import (
	"context"
	"errors"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

// defaultAutoSaveDelay is the idle time after the last edit before the
// autoSaver flushes the dirty buffer. Defined as a var so integration
// tests can shorten it to keep runs fast.
var defaultAutoSaveDelay = 2 * time.Second

// autoSaverFactory builds the autoSaver attached to a workspace. It is a
// var so integration tests can wrap the constructed saver to observe its
// notifications without changing production wiring.
var autoSaverFactory = newAutoSaver

// autoSaveEvents are the editor events that drive the autoSaver.
var autoSaveEvents = []textapi.EventType{
	textapi.EventTypeEdit,
	textapi.EventTypeFlush,
	textapi.EventTypeClose,
}

// autoSaverFlusher is the subset of *text.Component the autoSaver depends
// on. Defined as an interface so tests can stub out the disk I/O.
type autoSaverFlusher interface {
	Resource(uri workspaceapi.URI) (browserapi.Handler, bool)
	FlushTab(ctx context.Context, h browserapi.Handler) (<-chan error, error)
}

// autoSaver debounces editor edits per URI and triggers a buffer flush
// after the configured delay of inactivity. Edit events are dispatched
// synchronously by the editor's event loop, so all map mutations happen
// on the same goroutine and need no synchronization.
type autoSaver struct {
	comp          autoSaverFlusher
	notifications browserapi.Notifications
	sched         func(func()) bool
	delay         time.Duration

	timers map[string]*time.Timer
}

func newAutoSaver(
	comp autoSaverFlusher,
	notifications browserapi.Notifications,
	sched func(func()) bool,
	delay time.Duration,
) *autoSaver {
	if sched == nil {
		panic("ide.autoSaver: sched must not be nil")
	}
	return &autoSaver{
		comp:          comp,
		notifications: notifications,
		sched:         sched,
		delay:         delay,
		timers:        make(map[string]*time.Timer),
	}
}

// Handle implements textapi.EventHandler.
func (a *autoSaver) Handle(_ context.Context, ev textapi.Event) bool {
	switch ev.Type {
	case textapi.EventTypeEdit:
		a.scheduleFlush(ev.URI)
	case textapi.EventTypeFlush, textapi.EventTypeClose:
		a.cancel(ev.URI)
	}
	return false
}

func (a *autoSaver) scheduleFlush(uri workspaceapi.URI) {
	key := uri.String()
	if t, ok := a.timers[key]; ok {
		t.Stop()
	}
	a.timers[key] = time.AfterFunc(a.delay, func() {
		a.sched(func() { a.flushURI(uri) })
	})
}

func (a *autoSaver) cancel(uri workspaceapi.URI) {
	key := uri.String()
	if t, ok := a.timers[key]; ok {
		t.Stop()
		delete(a.timers, key)
	}
}

// flushURI runs on the UI goroutine and attempts to flush the tab for uri.
// Non-file tabs (terminals, file-explorer, ...) are skipped silently.
// All other failures — including read-only files and stale-on-disk —
// surface via the workspace notifications channel; auto-save never forces
// a write.
func (a *autoSaver) flushURI(uri workspaceapi.URI) {
	delete(a.timers, uri.String())

	h, ok := a.comp.Resource(uri)
	if !ok {
		return
	}
	ch, err := a.comp.FlushTab(context.Background(), h)
	if err != nil {
		switch {
		case errors.Is(err, textapi.ErrInvalidSave):
			// not a file tab (e.g. terminal/file-explorer); silently skip.
			return
		case errors.Is(err, workspace.ErrFlushInProgress):
			// previous save still in flight; next edit's debounce
			// will retry. Silently skip.
			return
		case errors.Is(err, workspaceapi.ErrFileIsNotWritable):
			_, _ = a.notifications.Notify(browserapi.LevelWarn,
				"auto-save skipped: %s is not writable", uri.Path())
			return
		case errors.Is(err, workspaceapi.ErrStaleData):
			_, _ = a.notifications.Notify(browserapi.LevelWarn,
				"auto-save skipped: %s changed on disk", uri.Path())
			return
		default:
			_, _ = a.notifications.Notify(browserapi.LevelError,
				"auto-save failed for %s: %v", uri.Path(), err)
			return
		}
	}
	// Async completion: dispatch the result back through sched so
	// the notification runs on the UI goroutine.
	go func() {
		ferr := <-ch
		if ferr == nil {
			return
		}
		a.sched(func() {
			switch {
			case errors.Is(ferr, workspaceapi.ErrFileIsNotWritable):
				_, _ = a.notifications.Notify(browserapi.LevelWarn,
					"auto-save skipped: %s is not writable", uri.Path())
			case errors.Is(ferr, workspaceapi.ErrStaleData):
				_, _ = a.notifications.Notify(browserapi.LevelWarn,
					"auto-save skipped: %s changed on disk", uri.Path())
			case errors.Is(ferr, context.Canceled):
				// caller cancelled; do nothing.
			default:
				_, _ = a.notifications.Notify(browserapi.LevelError,
					"auto-save failed for %s: %v", uri.Path(), ferr)
			}
		})
	}()
}
