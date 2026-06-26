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

package ide

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/release/docrelease"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/docmarshal/docbson"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/workspace"
)

// TestScheduleNextTickDoesNotReacquireHostLock encodes the contract
// every host event loop in this codebase already follows: when it
// dispatches a UserFunc scheduled via cfg.scheduleNextTick, it does so
// while already holding the IDE locker (see term/gui/gui.go's
// (*GUI).Update — g.mu is locked at gui.go:241 and ev.UserFunc() runs
// at gui.go:248 still under the lock).
//
// workspaceManagerHandler.init must therefore not wrap the host
// scheduler with another h.mu.Lock(); doing so re-acquires the same
// non-reentrant *sync.Mutex on the same goroutine and self-deadlocks
// the event loop. This test reproduces exactly that scenario without
// pulling a full IDE.
func TestScheduleNextTickDoesNotReacquireHostLock(t *testing.T) {
	t.Parallel()

	// Mirror the production wiring: a single non-reentrant
	// sync.Mutex is shared between the host event loop (gui /
	// tui) and the IDE handler.
	mu := new(sync.Mutex)

	// rawScheduleNextTick is the contract a real host (gui /
	// tui) presents to the IDE: the host buffers fn until its
	// next Update tick, then invokes it while holding mu.
	var pending func()
	cfg := defaultCfg()
	cfg.scheduleNextTick = func(fn func()) bool {
		pending = fn
		return true
	}

	homeURI, err := workspaceapi.ParseURI("memory:///home")
	require.NoError(t, err)

	manager := workspace.NewManager(cfg.workspace(), inlineSchedule)
	require.NoError(t, manager.RegisterScheme(workspace.MemoryScheme,
		workspace.NewMemoryScheme))

	dir := t.TempDir()
	storage := localstorage.New(context.Background(), dir, docbson.Marshaler())
	releaseManager := docrelease.NewManager(document.NewInMemoryService())

	shRunner := new(shaderRunner)
	shRunner.init(handler.Nop(), term.NopInterrupter(), term.Attributes{},
		nopShutdownShaderConfig(), loadingShaderConfig{}, openShaderConfig{},
		component.FrameCharSetDefault())

	h := new(workspaceManagerHandler)
	h.tutorialsInstalled = func([]string) (bool, error) { return false, nil }
	err = h.init(nil, homeURI, manager,
		notificationsConfig(), cfg, storage, dir,
		func(term.Event) bool { return true },
		FuncExtensionsRunner(testRunnerFn), mu, nil,
		func() (ideConfig, error) { return cfg, nil },
		".sixrc", 0, 0, '1', 0, 0, true, nil, releaseManager,
		shRunner, 0, nil, false, false, newCommandObserverRegistry())
	require.NoError(t, err)

	// Schedule a callback through the IDE's installed scheduler
	// (which is what every IDE caller — idelsp, lspcmd, autosaver,
	// addWorkspace's Phase B closure, etc. — invokes). The
	// callback records that it ran so we can distinguish
	// "deadlocked" from "ran but did the wrong thing".
	var ran bool
	require.True(t, h.scheduleNextTick(func() {
		ran = true
	}))
	require.NotNil(t, pending,
		"scheduleNextTick must forward to the host scheduler")

	// Simulate the host's next tick. (*GUI).Update locks first,
	// then dispatches every queued UserFunc inline. If init wrapped
	// scheduleNextTick with another mu.Lock(), pending() would
	// deadlock here and the watchdog below would fire.
	done := make(chan struct{})
	go func() {
		mu.Lock()
		defer mu.Unlock()
		pending()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, ran, "scheduled callback never ran")
	case <-time.After(2 * time.Second):
		t.Fatal("scheduleNextTick callback deadlocked: " +
			"init wrapped the host scheduler with h.mu.Lock()")
	}
}
