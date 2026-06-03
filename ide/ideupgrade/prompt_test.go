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

package ideupgrade

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
)

// mockWindowManager is a minimal browserapi.WindowManager that drives
// the floating prompt with a configurable input sequence and records
// the selected option index.
type mockWindowManager struct {
	feed func(browserapi.Floating)
}

func (m *mockWindowManager) Focus() (browserapi.Window, error) { return nil, nil }
func (m *mockWindowManager) Split(_ browserapi.Orientation, _ browserapi.Window, _ browserapi.Handler) (browserapi.Window, error) {
	return nil, nil
}
func (m *mockWindowManager) Floating(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
	if m.feed != nil {
		m.feed(h)
	}
	return nil, nil
}
func (m *mockWindowManager) Bar(_ browserapi.BarConfig, _ tui.Handler) error { return nil }
func (m *mockWindowManager) Tab(_ workspaceapi.URI, _ rune, _ string, _ browserapi.Handler) (browserapi.Handler, error) {
	return nil, nil
}
func (m *mockWindowManager) SetWindowContent(_ browserapi.Window, _ browserapi.Handler) error {
	return nil
}
func (m *mockWindowManager) CloseWindow(_ browserapi.Window) error { return nil }

func newPromptTestManager(t *testing.T, feed func(browserapi.Floating)) *Manager {
	t.Helper()
	storage := storagestub.NewInMemoryService()
	mgr, err := newWithPlatformOps(Config{
		CurrentVersion: "v1",
		Arch:           "darwin-arm64",
		ManifestURL:    "https://example.invalid",
		Storage:        storage,
		WindowManager:  &mockWindowManager{feed: feed},
		ScheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
	}, &fakePlatformOps{})
	require.NoError(t, err)
	return mgr
}

// resizeAndType drives a floating prompt by resizing it then sending
// a sequence of key events.
func resizeAndType(h browserapi.Floating, events ...term.Event) {
	h.Resize(80, 20)
	for _, ev := range events {
		h.Handle(ev)
	}
}

func TestPromptSkipPersistsSkippedVersion(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	mgr := newPromptTestManager(t, func(h browserapi.Floating) {
		defer wg.Done()
		// 's' is bound to "Skip This Version".
		resizeAndType(h, term.Event{Type: term.EventKey, Ch: 's'})
	})

	mgr.showPrompt(context.Background(), Manifest{
		Version: "v9.9.9", URL: "u", SHA256: "s",
	})
	wg.Wait()

	st := mgr.loadState(context.Background())
	require.Contains(t, st.SkippedVersions, "v9.9.9")
}

func TestPromptRemindLaterSetsRemindAfter(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	mgr := newPromptTestManager(t, func(h browserapi.Floating) {
		defer wg.Done()
		// 'l' is bound to "Remind Me Later".
		resizeAndType(h, term.Event{Type: term.EventKey, Ch: 'l'})
	})

	mgr.showPrompt(context.Background(), Manifest{
		Version: "v9.9.9", URL: "u", SHA256: "s",
	})
	wg.Wait()

	st := mgr.loadState(context.Background())
	require.False(t, st.RemindAfter.IsZero(), "remind_after should be set")
	require.True(t, st.RemindAfter.After(mgr.cfg.Now()), "remind_after should be in the future")
	require.NotContains(t, st.SkippedVersions, "v9.9.9")
}

func TestPromptShowFallsBackToNotificationWithoutWM(t *testing.T) {
	storage := storagestub.NewInMemoryService()
	notifs := &recordingNotifications{}
	mgr, err := newWithPlatformOps(Config{
		CurrentVersion: "v1",
		ManifestURL:    "https://example.invalid",
		Storage:        storage,
		Notifications:  notifs,
		ScheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
	}, &fakePlatformOps{})
	require.NoError(t, err)

	mgr.showPrompt(context.Background(), Manifest{Version: "v9", URL: "u", SHA256: "s"})
	notifyN, _ := notifs.snapshot()
	require.Equal(t, 1, notifyN, "fallback should post one notification")
	st := mgr.loadState(context.Background())
	require.Empty(t, st.SkippedVersions)
	require.True(t, st.RemindAfter.IsZero())
}
