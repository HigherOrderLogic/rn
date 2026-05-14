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
	"fmt"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/workspace"
)

// flusherTarget is the subset of text.Component that the flusher
// depends on. It is an interface so tests can stub the underlying
// disk/RPC work without spinning up a real workspace.
type flusherTarget interface {
	FlushTab(ctx context.Context, h browserapi.Handler) (<-chan error, error)
	ForceFlushTab(ctx context.Context, h browserapi.Handler) (<-chan error, error)
	OverwriteTab(ctx context.Context, h browserapi.Handler) (<-chan error, error)
	ReloadTab(ctx context.Context, h browserapi.Handler) (<-chan error, error)
}

// flusher coordinates async save / reload operations for an ex.
//
// It owns per-URI in-flight cancellation state and the awaiter
// goroutines that surface results as browser notifications scheduled
// through sched. Each public method that starts work returns the
// synchronous start-error from the underlying FlushTab / ReloadTab /
// OverwriteTab call, plus the busy sentinel workspace.ErrFlushInProgress
// when another async op is already in flight for the same URI.
//
// The flusher is the single owner of the inflight map and the awaiter
// WaitGroup: ex must not touch either directly so the invariants
// around scheduling, notification ordering, and cancellation stay
// local to this file.
type flusher struct {
	comp          flusherTarget
	notifications browserapi.Notifications
	sched         func(func()) bool

	mu sync.Mutex
	// files maps each URI with an in-flight async op to its cancel
	// function. Entries are removed by the completion callback that
	// runs on the sched goroutine.
	files map[workspaceapi.URI]context.CancelFunc

	// wg tracks awaiter goroutines so tests / shutdown can drain
	// outstanding work.
	wg sync.WaitGroup
}

// flusherOp tags the kind of async op for notification formatting in
// onDone.
type flusherOp int

const (
	opSave flusherOp = iota
	opOverwrite
	opReload
)

func newFlusher(
	comp flusherTarget,
	notifications browserapi.Notifications,
	sched func(func()) bool,
) *flusher {
	if sched == nil {
		panic("ide.flusher: sched must not be nil")
	}
	return &flusher{
		comp:          comp,
		notifications: notifications,
		sched:         sched,
		files:         make(map[workspaceapi.URI]context.CancelFunc),
	}
}

// flush starts a non-force save for the tab at uri/h.
func (f *flusher) flush(
	uri workspaceapi.URI, h browserapi.Handler,
) error {
	return f.flushAndThen(uri, h, false, nil)
}

// forceFlush starts a force save (overwrites changes from other
// processes) for the tab at uri/h.
func (f *flusher) forceFlush(
	uri workspaceapi.URI, h browserapi.Handler,
) error {
	return f.flushAndThen(uri, h, true, nil)
}

// flushAndThen starts an async save and invokes onSuccess on the
// sched goroutine after a successful completion. :wq uses this hook
// to schedule editor exit only when the save succeeded.
func (f *flusher) flushAndThen(
	uri workspaceapi.URI, h browserapi.Handler,
	force bool, onSuccess func(),
) error {
	start := func(ctx context.Context) (<-chan error, error) {
		return f.comp.FlushTab(ctx, h)
	}
	if force {
		start = func(ctx context.Context) (<-chan error, error) {
			return f.comp.ForceFlushTab(ctx, h)
		}
	}
	return f.startAsync(uri, opSave, start, onSuccess)
}

// overwrite starts an async overwrite for the tab at uri/h. The
// resulting error (if any) is surfaced as an error notification.
func (f *flusher) overwrite(
	uri workspaceapi.URI, h browserapi.Handler,
) error {
	return f.startAsync(uri, opOverwrite,
		func(ctx context.Context) (<-chan error, error) {
			return f.comp.OverwriteTab(ctx, h)
		}, nil)
}

// reloadAsync starts an async reload for the tab at uri/h. Used by
// prompt-driven reloads where the user shouldn't have to wait for
// the underlying scheme to respond before the prompt is dismissed.
func (f *flusher) reloadAsync(
	uri workspaceapi.URI, h browserapi.Handler,
) error {
	return f.startAsync(uri, opReload,
		func(ctx context.Context) (<-chan error, error) {
			return f.comp.ReloadTab(ctx, h)
		}, nil)
}

// reload runs a synchronous reload for uri/h, cancelling any prior
// in-flight op for the same URI first. Intended for :reloadfile,
// which is user-initiated and must show the new content immediately.
func (f *flusher) reload(
	uri workspaceapi.URI, h browserapi.Handler,
) error {
	f.mu.Lock()
	prev, busy := f.files[uri]
	f.mu.Unlock()
	if busy {
		// Cancel the prior in-flight op and wait for its awaiter
		// to settle before starting a fresh reload. :reloadfile is
		// forceful by user intent — it should win over any
		// speculative background reload the filesystem watcher
		// started.
		prev()
		f.wg.Wait()
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := f.comp.ReloadTab(ctx, h)
	if err != nil {
		cancel()
		return err
	}
	f.mu.Lock()
	f.files[uri] = cancel
	f.mu.Unlock()
	rerr := <-ch
	f.mu.Lock()
	delete(f.files, uri)
	f.mu.Unlock()
	if rerr != nil && !errors.Is(rerr, context.Canceled) {
		return fmt.Errorf("reload: %w", rerr)
	}
	return nil
}

// cancel cancels the in-flight async op for uri (if any). Returns
// workspace.ErrNoFlushInProgress when nothing is pending. The
// underlying scheme call cannot be aborted; its result is discarded
// when it eventually returns.
func (f *flusher) cancel(uri workspaceapi.URI) error {
	f.mu.Lock()
	c, ok := f.files[uri]
	f.mu.Unlock()
	if !ok {
		return workspace.ErrNoFlushInProgress
	}
	c()
	return nil
}

// inFlightCount reports how many URIs have an outstanding async op.
// Used by :q / :wq to refuse exit while saves are pending.
func (f *flusher) inFlightCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.files)
}

// wait blocks until every awaiter goroutine has delivered its
// completion callback through sched. Intended for tests and graceful
// shutdown.
func (f *flusher) wait() {
	f.wg.Wait()
}

func (f *flusher) startAsync(
	uri workspaceapi.URI,
	kind flusherOp,
	start func(ctx context.Context) (<-chan error, error),
	onSuccess func(),
) error {
	// We hold mu only for the busy check; the underlying start call
	// runs without the lock so a slow scheme can't stall other
	// flushers. Re-acquire mu briefly to register the cancel.
	f.mu.Lock()
	if _, busy := f.files[uri]; busy {
		f.mu.Unlock()
		return workspace.ErrFlushInProgress
	}
	f.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := start(ctx)
	if err != nil {
		cancel()
		return err
	}
	f.mu.Lock()
	f.files[uri] = cancel
	f.mu.Unlock()
	f.wg.Add(1)
	go debug.CapturePanicReport(func() {
		defer f.wg.Done()
		ferr := <-ch
		// sched must dispatch onto the UI goroutine so map
		// mutations and notifications happen on a single thread.
		f.sched(func() { f.onDone(uri, kind, ferr, onSuccess) })
	})
	return nil
}

func (f *flusher) onDone(
	uri workspaceapi.URI, kind flusherOp, err error, onSuccess func(),
) {
	f.mu.Lock()
	delete(f.files, uri)
	f.mu.Unlock()
	if err == nil {
		if onSuccess != nil {
			onSuccess()
		}
		return
	}
	name := uri.Name()
	switch kind {
	case opSave:
		switch {
		case errors.Is(err, context.Canceled):
			_, _ = f.notifications.Notify(browserapi.LevelInfo,
				"save cancelled for '%s'", name)
		case errors.Is(err, workspaceapi.ErrStaleData),
			errors.Is(err, workspaceapi.ErrFileIsNotWritable):
			_, _ = f.notifications.Notify(browserapi.LevelWarn,
				"save '%s': %v", name, err)
		default:
			_, _ = f.notifications.Notify(browserapi.LevelError,
				"save '%s': %v", name, err)
		}
	case opOverwrite:
		if errors.Is(err, context.Canceled) {
			return
		}
		_, _ = f.notifications.Notify(browserapi.LevelError,
			"failed to overwrite tab: %v", err)
	case opReload:
		if errors.Is(err, context.Canceled) {
			return
		}
		_, _ = f.notifications.Notify(browserapi.LevelError,
			"failed to reload tab: %v", err)
	}
}
