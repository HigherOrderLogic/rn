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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace/workspacetest"
)

// blockingTerminal is a schemeapi.Terminal whose SetPtySize parks
// until the test releases it, standing in for a wedged transport.
type blockingTerminal struct {
	entered chan [2]int
	release chan struct{}

	mu    sync.Mutex
	sizes [][2]int

	setSizeErr error

	newPtyErr error
	newPtyCnt int
}

func newBlockingTerminal() *blockingTerminal {
	return &blockingTerminal{
		entered: make(chan [2]int, 16),
		release: make(chan struct{}),
	}
}

func (b *blockingTerminal) NewPty(context.Context) (workspaceapi.Pty, error) {
	b.mu.Lock()
	b.newPtyCnt++
	b.mu.Unlock()
	if b.newPtyErr != nil {
		return workspaceapi.Pty{}, b.newPtyErr
	}
	return workspaceapi.Pty{
		Master: workspacetest.NewFile(),
		Slave:  workspacetest.NewFile(),
	}, nil
}

func (b *blockingTerminal) SetPtySize(
	_ workspaceapi.Pty, width, height int,
) error {
	b.entered <- [2]int{width, height}
	<-b.release
	b.mu.Lock()
	b.sizes = append(b.sizes, [2]int{width, height})
	b.mu.Unlock()
	return b.setSizeErr
}

func (b *blockingTerminal) delivered() [][2]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([][2]int(nil), b.sizes...)
}

// resizeNotifications captures the messages the resize worker
// raises so the tests can assert transport errors reach the user.
type resizeNotifications struct {
	nopNotifications
	mu   sync.Mutex
	msgs []string
}

func (r *resizeNotifications) Notify(
	_ browserapi.NotificationLevel, msg string, args ...any,
) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, fmt.Sprintf(msg, args...))
	return "", nil
}

func (r *resizeNotifications) notified() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.msgs...)
}

func TestAsyncTerminal(t *testing.T) {
	t.Run("SetPtySize returns while the wrapped call is blocked", func(t *testing.T) {
		wrapped := newBlockingTerminal()
		at := newAsyncTerminal(wrapped, nopNotifications{})
		defer at.Close()

		done := make(chan error, 1)
		go func() { done <- at.SetPtySize(workspaceapi.Pty{}, 80, 24) }()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("SetPtySize must not block on the transport RPC")
		}

		// The worker is now parked inside the wrapped RPC.
		select {
		case got := <-wrapped.entered:
			assert.Equal(t, [2]int{80, 24}, got)
		case <-time.After(5 * time.Second):
			t.Fatal("resize worker never dispatched the queued resize")
		}

		done2 := make(chan error, 1)
		go func() { done2 <- at.SetPtySize(workspaceapi.Pty{}, 100, 30) }()
		select {
		case err := <-done2:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("a queued resize must not block behind the in-flight RPC")
		}

		close(wrapped.release)
		require.Eventually(t, func() bool {
			d := wrapped.delivered()
			return len(d) == 2 && d[1] == [2]int{100, 30}
		}, 5*time.Second, 5*time.Millisecond,
			"the latest queued size must reach the transport once it unwinds")
	})

	t.Run("SetPtySize stays non-blocking past the queue capacity", func(t *testing.T) {
		wrapped := newBlockingTerminal()
		at := newAsyncTerminal(wrapped, nopNotifications{})
		defer at.Close()

		for i := range asyncTerminalResizeQueue * 3 {
			done := make(chan error, 1)
			go func() {
				done <- at.SetPtySize(workspaceapi.Pty{}, 80+i, 24)
			}()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatalf("SetPtySize blocked on resize %d", i)
			}
		}
	})

	t.Run("NewPty delegates synchronously", func(t *testing.T) {
		wrapped := newBlockingTerminal()
		at := newAsyncTerminal(wrapped, nopNotifications{})
		defer at.Close()

		pty, err := at.NewPty(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, pty.Master)
		assert.Equal(t, 1, wrapped.newPtyCnt)

		wrapped.newPtyErr = errors.New("boom")
		_, err = at.NewPty(context.Background())
		assert.EqualError(t, err, "boom")
	})

	t.Run("transport failures are notified", func(t *testing.T) {
		wrapped := newBlockingTerminal()
		wrapped.setSizeErr = errors.New("transport wedged")
		close(wrapped.release)
		notis := new(resizeNotifications)
		at := newAsyncTerminal(wrapped, notis)
		defer at.Close()

		require.NoError(t, at.SetPtySize(workspaceapi.Pty{}, 80, 24))
		require.Eventually(t, func() bool {
			return len(notis.notified()) == 1
		}, 5*time.Second, 5*time.Millisecond)
		assert.Equal(t, "resize terminal to 80x24: transport wedged",
			notis.notified()[0])
	})

	t.Run("a resize against a dead pty is not notified", func(t *testing.T) {
		wrapped := newBlockingTerminal()
		wrapped.setSizeErr = errors.New(
			"rpc error: code = Unknown desc = invalid master pty fd")
		close(wrapped.release)
		notis := new(resizeNotifications)
		at := newAsyncTerminal(wrapped, notis)
		defer at.Close()

		require.NoError(t, at.SetPtySize(workspaceapi.Pty{}, 80, 24))
		require.Eventually(t, func() bool {
			return len(wrapped.delivered()) == 1
		}, 5*time.Second, 5*time.Millisecond)
		assert.Empty(t, notis.notified())
	})

	t.Run("Close does not wait for the in-flight RPC", func(t *testing.T) {
		wrapped := newBlockingTerminal()
		at := newAsyncTerminal(wrapped, nopNotifications{})

		require.NoError(t, at.SetPtySize(workspaceapi.Pty{}, 80, 24))
		select {
		case <-wrapped.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("resize worker never dispatched the queued resize")
		}
		require.NoError(t, at.SetPtySize(workspaceapi.Pty{}, 100, 30))

		closed := make(chan struct{})
		go func() {
			at.Close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Fatal("Close must not wait for the in-flight transport RPC")
		}

		close(wrapped.release)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, [][2]int{{80, 24}}, wrapped.delivered(),
			"a resize queued before Close must not be dispatched after it")

		at.Close() // idempotent
	})
}
