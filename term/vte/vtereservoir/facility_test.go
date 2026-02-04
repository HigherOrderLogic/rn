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

package vtereservoir

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/workspace/workspacetest"
)

func TestFacility(t *testing.T) {
	t.Parallel()
	t.Run("pre-allocates initial capacity", func(t *testing.T) {
		t.Parallel()
		var called atomic.Int64
		f := newTestFacility(5, func(f *Facility) (VTE, error) {
			called.Add(1)
			return newTestVte(f), nil
		})
		assert.Equal(t, 5, int(called.Load()))

		_, err := f.Get()
		require.NoError(t, err)

		assert.Equal(t, 5, int(called.Load()))
	})

	t.Run("pre-allocates if pool is empty", func(t *testing.T) {
		t.Parallel()
		var called atomic.Int64
		f := newTestFacility(1, func(f *Facility) (VTE, error) {
			called.Add(1)
			return newTestVte(f), nil
		})
		assert.Equal(t, 1, int(called.Load()))

		_, err := f.Get()
		require.NoError(t, err)

		assert.Equal(t, 2, int(called.Load()))
	})

	t.Run("errors are handled and pool capacity reduced accordingly", func(t *testing.T) {
		t.Parallel()
		var called atomic.Int64
		f := newTestFacility(4, func(f *Facility) (VTE, error) {
			if called.Add(1)%2 == 0 {
				return nil, errors.New("errors, lots of them")
			}
			return newTestVte(f), nil
		})
		f.Resize(10, 10)

		_, err := f.Get()
		require.NoError(t, err)

		_, err = f.Get()
		require.NoError(t, err)

		_, err = f.Get()
		require.Error(t, err)

		_, err = f.Get()
		require.NoError(t, err)

		assert.Equal(t, 7, int(called.Load()))

		f.Resize(10, 10)
	})

	t.Run("facility.Close closes all free vtes", func(t *testing.T) {
		t.Parallel()
		var mu sync.Mutex
		var created []*testVte
		f := newTestFacility(5, func(f *Facility) (VTE, error) {
			tvte := newTestVte(f)
			mu.Lock()
			defer mu.Unlock()
			created = append(created, tvte)
			return tvte, nil
		})

		// free and returned
		vte1, err := f.Get()
		require.NoError(t, err)

		vte2, err := f.Get()
		require.NoError(t, err)

		require.NoError(t, vte1.Close())
		require.NoError(t, f.Close())
		require.NoError(t, vte2.Close())

		for _, vte := range created {
			assert.True(t, vte.calledClose)
		}
	})

	t.Run("VTE.Close puts vte back into the pool", func(t *testing.T) {
		t.Parallel()
		var called atomic.Int32
		f := newTestFacility(2, func(f *Facility) (VTE, error) {
			tvte := newTestVte(f)
			called.Add(1)
			return tvte, nil
		})

		vte, err := f.Get()
		require.NoError(t, err)

		require.NoError(t, vte.Close())

		vte, err = f.Get()
		require.NoError(t, err)

		assert.Equal(t, 2, int(called.Load()))
	})

	t.Run("VTE.Close does not put vte back into the pool, if used alternate buffer", func(t *testing.T) {
		t.Parallel()
		var called atomic.Int32
		f := newTestFacility(2, func(f *Facility) (VTE, error) {
			tvte := newTestVte(f)
			tvte.usedAlt = true
			called.Add(1)
			return tvte, nil
		})

		vte, err := f.Get()
		require.NoError(t, err)

		require.NoError(t, vte.Close())

		vte, err = f.Get()
		require.NoError(t, err)

		assert.Equal(t, 3, int(called.Load()))
	})

	t.Run("VTE.Close clears the primary buffer when put back into pool", func(t *testing.T) {
		t.Parallel()
		var tvte *testVte
		var c atomic.Int32
		var mu sync.Mutex
		f := newTestFacility(1, func(f *Facility) (VTE, error) {
			if c.CompareAndSwap(0, 1) {
				mu.Lock()
				defer mu.Unlock()
				tvte = newTestVte(f)
				return tvte, nil
			}
			return newTestVte(f), nil
		})

		vte, err := f.Get()
		require.NoError(t, err)

		require.NoError(t, vte.Close())

		mu.Lock()
		defer mu.Unlock()
		
		require.NotNil(t, 2, tvte)
		assert.True(t, tvte.clearedPrimary)
	})

	t.Run("Resize after new doesn't block waiting for all vtes to be created", func(t *testing.T) {
		t.Parallel()
		b := nopBrowser{}
		var wg sync.WaitGroup
		const capacity = 5
		wg.Add(capacity)
		uri, err := workspaceapi.ParseURI("file:///tmp")
		require.NoError(t, err)
		scheme, err := workspacetest.NewNopScheme("file:///tmp")(
			context.Background(), config.NopConfig(), uri)
		scheme.(*workspacetest.NopScheme).NewPtyFunc = func(ctx context.Context) (workspaceapi.Pty, error) {
			defer wg.Done()
			return workspaceapi.Pty{
				Master: scheme.NewFile(0, ""),
				Slave:  scheme.NewFile(1, ""),
			}, nil
		}
		scheme.(*workspacetest.NopScheme).StartCommandFunc = func(context.Context, workspaceapi.Cmd) (workspaceapi.Pid, error) {
			time.Sleep(100 * time.Minute)
			return 0, nil
		}
		require.NoError(t, err)
		f := New(b, b, scheme, scheme, b, vte.DefaultConfig(), capacity)
		// this would block for 100 minutes if not implemented correctly
		f.Resize(10, 10)
		wg.Wait()
	})
}

type nopBrowser struct {
}

func (n nopBrowser) PublishEvent(term.Event) error {
	return nil
}
func (n nopBrowser) Notify(level browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	return "", nil
}
func (n nopBrowser) NotifyOnce(level browserapi.NotificationLevel, msg string, args ...any) (string, error) {
	return "", nil
}
func (n nopBrowser) UpdateNotificationProgress(id, message string, progress, total int64) error {
	return nil
}
func (nopBrowser) Tab(uri workspaceapi.URI, icon rune, name string, h browserapi.Handler) (
	browserapi.Handler, error,
) {
	return browsertest.NewTestHandler(), nil
}

func (n nopBrowser) SetTabName(workspaceapi.URI, string, term.Attributes) error {
	return nil
}

func newTestFacility(
	initCap int, newFn func(*Facility) (VTE, error),
) *Facility {
	f := new(Facility)
	f.new = func(bool) (VTE, error) { return newFn(f) }
	f.initCap(initCap)
	return f
}

var _ VTE = (*testVte)(nil)

type testVte struct {
	component.String
	initialCmd     string
	f              *Facility
	usedAlt        bool
	clearedPrimary bool

	calledClose bool
}

func newTestVte(f *Facility) *testVte {
	return newTestVteWithConfig(f, "")
}

func newTestVteWithConfig(f *Facility, initialCmd string) *testVte {
	ret := new(testVte)
	ret.initialCmd = initialCmd
	ret.String = component.NewString(initialCmd)
	ret.f = f
	return ret
}

func (t *testVte) Handle(ev term.Event) (bool, bool) {
	return false, false
}

func (t *testVte) SeekUp() bool {
	return false
}

func (t *testVte) SeekDown() bool {
	return false
}

func (t *testVte) SeekOffset() int {
	return 0
}

func (t *testVte) MaxSeekOffset() int {
	return 0
}

func (t *testVte) Cursor() (ret term.Coordinates, style term.CursorStyle, show bool) {
	show = true
	ret = term.Coordinates{X: len(t.initialCmd)}
	return
}

func (t *testVte) Selection() (string, bool) {
	return "", false
}

func (t *testVte) Dimensions() (int, int) {
	return 0, 0
}

func (t *testVte) Close() error {
	if t.calledClose {
		return errors.New("called close twice")
	}
	if t.f.pool == nil || t.UsedAlternateBuffer() {
		t.calledClose = true
		return nil
	}
	t.f.put(t)
	return nil
}

func (t *testVte) OnFocusChange(inFocus bool) {
}

func (t *testVte) SetDefaultAttributes(attr term.Attributes) {
}

func (t *testVte) IsComplete() bool {
	return false
}

func (t *testVte) URI() workspaceapi.URI {
	return workspaceapi.URI{}
}

func (t *testVte) Title() string {
	return t.initialCmd
}

func (v *testVte) UsedAlternateBuffer() bool {
	return v.usedAlt
}

func (v *testVte) ClearPrimaryBuffer() bool {
	v.clearedPrimary = true
	return true
}
