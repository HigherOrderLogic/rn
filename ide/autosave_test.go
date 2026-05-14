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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/workspace"
)

// TestAutoSaver exercises the debounced flush behaviour of autoSaver.
// The table is keyed by the inputs feed into Handle and the configured
// flusher/notification behaviour, asserting on the URIs that ended up
// being flushed and any notifications emitted.
func TestAutoSaver(t *testing.T) {
	uriA := mustURI(t, "memory:///tmp/a")
	uriB := mustURI(t, "memory:///tmp/b")

	cases := []autoSaverCase{
		{
			name:        "flushes after idle delay",
			edits:       []autoSaverInput{{evt: textapi.EventTypeEdit, uri: uriA}},
			drainOnce:   true,
			wantCalls:   []workspaceapi.URI{uriA},
			wantNotifLn: 0,
		},
		{
			name: "debounces repeated edits to a single flush",
			edits: []autoSaverInput{
				{evt: textapi.EventTypeEdit, uri: uriA, sleep: 5 * time.Millisecond},
				{evt: textapi.EventTypeEdit, uri: uriA, sleep: 5 * time.Millisecond},
				{evt: textapi.EventTypeEdit, uri: uriA, sleep: 5 * time.Millisecond},
				{evt: textapi.EventTypeEdit, uri: uriA, sleep: 5 * time.Millisecond},
				{evt: textapi.EventTypeEdit, uri: uriA, sleep: 5 * time.Millisecond},
			},
			delay:       50 * time.Millisecond,
			drainOnce:   true,
			settle:      80 * time.Millisecond,
			wantCalls:   []workspaceapi.URI{uriA},
			wantNotifLn: 0,
		},
		{
			name: "manual flush event cancels pending auto-save",
			edits: []autoSaverInput{
				{evt: textapi.EventTypeEdit, uri: uriA},
				{evt: textapi.EventTypeFlush, uri: uriA},
			},
			delay:       50 * time.Millisecond,
			settle:      80 * time.Millisecond,
			wantCalls:   nil,
			wantNotifLn: 0,
		},
		{
			name: "close event cancels pending auto-save",
			edits: []autoSaverInput{
				{evt: textapi.EventTypeEdit, uri: uriA},
				{evt: textapi.EventTypeClose, uri: uriA},
			},
			delay:       50 * time.Millisecond,
			settle:      80 * time.Millisecond,
			wantCalls:   nil,
			wantNotifLn: 0,
		},
		{
			name: "per-uri isolation: cancelling A leaves B's timer",
			edits: []autoSaverInput{
				{evt: textapi.EventTypeEdit, uri: uriA},
				{evt: textapi.EventTypeEdit, uri: uriB},
				{evt: textapi.EventTypeFlush, uri: uriA},
			},
			delay:       30 * time.Millisecond,
			drainOnce:   true,
			settle:      60 * time.Millisecond,
			wantCalls:   []workspaceapi.URI{uriB},
			wantNotifLn: 0,
		},
		{
			name:        "skips closed tabs silently",
			edits:       []autoSaverInput{{evt: textapi.EventTypeEdit, uri: uriA}},
			missing:     []string{uriA.String()},
			drainOnce:   true,
			wantCalls:   nil,
			wantNotifLn: 0,
		},
		{
			name:           "stale data on disk surfaces a warning",
			edits:          []autoSaverInput{{evt: textapi.EventTypeEdit, uri: uriA}},
			preflightErr:   workspaceapi.ErrStaleData,
			drainOnce:      true,
			wantCalls:      []workspaceapi.URI{uriA},
			wantNotifLn:    1,
			wantNotifLevel: browserapi.LevelWarn,
		},
		{
			name:           "read-only file surfaces a warning",
			edits:          []autoSaverInput{{evt: textapi.EventTypeEdit, uri: uriA}},
			preflightErr:   workspaceapi.ErrFileIsNotWritable,
			drainOnce:      true,
			wantCalls:      []workspaceapi.URI{uriA},
			wantNotifLn:    1,
			wantNotifLevel: browserapi.LevelWarn,
		},
		{
			name:         "invalid-save tab kinds (e.g. terminal) are silently skipped",
			edits:        []autoSaverInput{{evt: textapi.EventTypeEdit, uri: uriA}},
			preflightErr: textapi.ErrInvalidSave,
			drainOnce:    true,
			wantCalls:    []workspaceapi.URI{uriA},
			wantNotifLn:  0,
		},
		{
			name:         "in-flight save is silently skipped (debounce will retry)",
			edits:        []autoSaverInput{{evt: textapi.EventTypeEdit, uri: uriA}},
			preflightErr: workspace.ErrFlushInProgress,
			drainOnce:    true,
			wantCalls:    []workspaceapi.URI{uriA},
			wantNotifLn:  0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runAutoSaverCase(t, tc)
		})
	}
}

// FIXTURES — kept below the test functions per project convention.

// autoSaverInput is a single event sent into autoSaver.Handle, with an
// optional inter-event sleep used by the debounce case.
type autoSaverInput struct {
	evt   textapi.EventType
	uri   workspaceapi.URI
	sleep time.Duration
}

// autoSaverCase is one row in the TestAutoSaver table.
type autoSaverCase struct {
	name string

	// inputs
	edits        []autoSaverInput
	missing      []string
	preflightErr error
	delay        time.Duration // defaults to 10ms
	drainOnce    bool          // pull a single scheduled callback
	settle       time.Duration // additional time to let stray timers fire

	// expectations
	wantCalls      []workspaceapi.URI
	wantNotifLn    int
	wantNotifLevel browserapi.NotificationLevel
}

func runAutoSaverCase(t *testing.T, tc autoSaverCase) {
	t.Helper()
	missing := map[string]bool{}
	for _, m := range tc.missing {
		missing[m] = true
	}
	flusher := &recordingFlusher{err: tc.preflightErr, missing: missing}
	notif := &fakeNotifications{}
	sched := newQueueSched()
	delay := tc.delay
	if delay == 0 {
		delay = 10 * time.Millisecond
	}
	saver := newAutoSaver(flusher, notif, sched.sched, delay)

	for _, in := range tc.edits {
		saver.Handle(context.Background(),
			textapi.Event{Type: in.evt, URI: in.uri})
		if in.sleep > 0 {
			time.Sleep(in.sleep)
		}
	}

	if tc.drainOnce {
		sched.drain(t)
	}
	if tc.settle > 0 {
		time.Sleep(tc.settle)
		sched.drainAll()
	}

	if tc.wantCalls == nil {
		assert.Empty(t, flusher.calls)
	} else {
		assert.Equal(t, tc.wantCalls, flusher.calls)
	}
	if tc.wantNotifLn == 0 {
		assert.Empty(t, notif.notes)
	} else {
		require.Len(t, notif.notes, tc.wantNotifLn)
		assert.Equal(t, tc.wantNotifLevel, notif.notes[0].level)
	}
}

// recordingFlusher implements autoSaverFlusher and records every URI it is
// asked to flush. All calls happen on the test goroutine because tests
// drain the scheduler queue inline (matching the editor's single-threaded
// event dispatch).
type recordingFlusher struct {
	calls   []workspaceapi.URI
	err     error
	missing map[string]bool
}

func (r *recordingFlusher) Resource(uri workspaceapi.URI) (browserapi.Handler, bool) {
	if r.missing[uri.String()] {
		return nil, false
	}
	return recordingHandler{uri: uri}, true
}

func (r *recordingFlusher) FlushTab(
	_ context.Context, h browserapi.Handler,
) (<-chan error, error) {
	rh := h.(recordingHandler)
	r.calls = append(r.calls, rh.uri)
	if r.err != nil {
		// pre-flight error (e.g. ErrInvalidSave / ErrFlushInProgress).
		// Return nil channel like the production interface.
		return nil, r.err
	}
	// async completion: no error.
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch, nil
}

// recordingHandler is a minimal browserapi.Handler used to round-trip a
// URI from Resource into FlushTab.
type recordingHandler struct{ uri workspaceapi.URI }

func (recordingHandler) Resize(_, _ int)                          {}
func (recordingHandler) Draw(_ term.Writer)                       {}
func (recordingHandler) Handle(_ term.Event) (exit, handled bool) { return false, false }
func (recordingHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, 0, false
}
func (recordingHandler) Selection() (string, bool) { return "", false }
func (recordingHandler) Close() error              { return nil }

var _ tui.Handler = recordingHandler{}

// fakeNotifications captures calls into browserapi.Notifications.
type fakeNotifications struct {
	notes []notifRecord
}

type notifRecord struct {
	level browserapi.NotificationLevel
	msg   string
}

func (f *fakeNotifications) Notify(level browserapi.NotificationLevel,
	msg string, _ ...any) (string, error) {
	f.notes = append(f.notes, notifRecord{level: level, msg: msg})
	return "", nil
}

func (f *fakeNotifications) NotifyOnce(level browserapi.NotificationLevel,
	msg string, args ...any) (string, error) {
	return f.Notify(level, msg, args...)
}

func (f *fakeNotifications) UpdateNotificationProgress(_, _ string, _, _ int64) error {
	return nil
}

// queueSched mimics scheduleNextTick by enqueuing scheduled callbacks onto
// a buffered channel. Tests drain the queue inline so all flushURI calls
// run on the test goroutine, matching the editor's single-threaded event
// dispatch.
type queueSched struct{ q chan func() }

func newQueueSched() *queueSched { return &queueSched{q: make(chan func(), 16)} }

func (s *queueSched) sched(fn func()) bool {
	s.q <- fn
	return true
}

// drain pulls the next scheduled callback and runs it. It fails the test
// if no callback shows up within the timeout.
func (s *queueSched) drain(t *testing.T) {
	t.Helper()
	select {
	case fn := <-s.q:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduled flush")
	}
}

// drainAll runs everything currently queued.
func (s *queueSched) drainAll() {
	for {
		select {
		case fn := <-s.q:
			fn()
		default:
			return
		}
	}
}

func mustURI(t *testing.T, raw string) workspaceapi.URI {
	t.Helper()
	u, err := workspaceapi.ParseURI(raw)
	require.NoError(t, err)
	return u
}
