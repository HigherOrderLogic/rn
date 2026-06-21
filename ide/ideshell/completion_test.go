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

package ideshell

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// iterCmd is a CommandHandler whose argument completion is backed by a
// caller-supplied iterator, letting tests drive streaming and blocking
// behavior precisely.
type iterCmd struct {
	iter func() iterator.Iterator[string]
}

func (iterCmd) HandleCommand(
	context.Context, repl.Command, repl.ProgressWriter,
) (iterator.Iterator[component.Responsive], error) {
	return iterator.Empty[component.Responsive](), nil
}

func (c iterCmd) Complete(
	context.Context, string, []string,
) (iterator.Iterator[string], error) {
	return c.iter(), nil
}

func (iterCmd) Help(
	context.Context, []string,
) (iterator.Iterator[component.Responsive], error) {
	return iterator.Empty[component.Responsive](), nil
}

// blockingStringIter yields its first candidate immediately, then
// blocks on subsequent Next calls until the context is cancelled. This
// models a completer that starts slow background I/O after surfacing an
// initial result: emitting one element first lets layers that probe
// emptiness (e.g. term/sh's IsEmpty file fallback) proceed, while the
// feeder still blocks mid-stream. It records that the blocking Next
// started and that Close ran so tests can assert no leak.
type blockingStringIter struct {
	first   string
	sent    bool
	started chan struct{}
	once    sync.Once
	closed  chan struct{}
	closeOK sync.Once
}

func (b *blockingStringIter) Next(ctx context.Context) (string, bool) {
	if !b.sent {
		b.sent = true
		return b.first, true
	}
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return "", false
}

func (b *blockingStringIter) Err() error { return nil }

func (b *blockingStringIter) Close() error {
	b.closeOK.Do(func() { close(b.closed) })
	return nil
}

// TestCompleteDoesNotDrainIterator asserts that the shim returns from
// Complete without draining the candidate iterator, even when that
// iterator blocks indefinitely. A synchronous drain on the event loop
// would freeze the prompt.
func TestCompleteDoesNotDrainIterator(t *testing.T) {
	blk := &blockingStringIter{
		first: "alpha", started: make(chan struct{}), closed: make(chan struct{}),
	}
	shim := &completionShim{
		underlying: iterCmd{iter: func() iterator.Iterator[string] { return blk }},
	}

	done := make(chan iterator.Iterator[string], 1)
	go func() {
		it, err := shim.Complete(context.Background(), "g", []string{""})
		require.NoError(t, err)
		done <- it
	}()

	var ret iterator.Iterator[string]
	select {
	case ret = <-done:
	case <-time.After(time.Second):
		t.Fatal("Complete blocked draining the iterator")
	}

	// The SDK inputbox must see an empty iterator so it does not enter
	// inline completion; the real candidates are stashed for the overlay.
	_, ok := ret.Next(context.Background())
	assert.False(t, ok, "returned iterator must be empty")

	captured, has := shim.consume()
	require.True(t, has)
	assert.Same(t, iterator.Iterator[string](blk), captured.iter)
}

// TestOpenCompletionStreamsIncrementally feeds several candidates
// through the overlay and asserts they all land in the list.
func TestOpenCompletionStreamsIncrementally(t *testing.T) {
	h := newTestHandlerFull(t, nil, 100, func(r *CommandRegistry) {
		r.Register("g", "", iterCmd{iter: func() iterator.Iterator[string] {
			return iterator.FromSlice([]string{"alpha", "beta", "gamma"})
		}})
	})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "g ")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})
	h.waitCompletion()
	drainTicks(h)

	assert.True(t, h.searching)
	assert.Equal(t, modeCompletion, h.mode)
	assert.Equal(t, 3, h.list.MatchCount())
}

// TestOpenCompletionCancelMidStreamClosesIterator cancels the overlay
// while the feeder is blocked on a slow iterator and asserts the
// goroutine exits and the iterator is closed (no leak).
func TestOpenCompletionCancelMidStreamClosesIterator(t *testing.T) {
	blk := &blockingStringIter{
		first: "alpha", started: make(chan struct{}), closed: make(chan struct{}),
	}
	h := newTestHandlerFull(t, nil, 100, func(r *CommandRegistry) {
		r.Register("g", "", iterCmd{iter: func() iterator.Iterator[string] { return blk }})
	})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "g ")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})

	select {
	case <-blk.started:
	case <-time.After(time.Second):
		t.Fatal("feeder did not start pulling from the iterator")
	}

	h.cancelSearch()

	assert.False(t, h.searching)
	assert.Nil(t, h.compDone, "feeder must be torn down")
	select {
	case <-blk.closed:
	case <-time.After(time.Second):
		t.Fatal("iterator was not closed after cancel")
	}
}

// TestOpenCompletionSingleMatchAutoAccepts asserts a stream that yields
// exactly one candidate fills the editor line inline and closes the
// overlay, preserving the pre-streaming single-completion UX.
func TestOpenCompletionSingleMatchAutoAccepts(t *testing.T) {
	h := newTestHandlerFull(t, nil, 100, func(r *CommandRegistry) {
		r.Register("g", "", iterCmd{iter: func() iterator.Iterator[string] {
			return iterator.FromSlice([]string{"alpha"})
		}})
	})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "g al")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})
	h.waitCompletion()
	drainTicks(h)

	assert.False(t, h.searching, "overlay must close on single match")
	assert.Equal(t, "g alpha", h.editBuf.String())
}

// TestOpenCompletionZeroMatchClosesOverlay asserts an empty stream
// closes the overlay and leaves the editor line unchanged.
func TestOpenCompletionZeroMatchClosesOverlay(t *testing.T) {
	h := newTestHandlerFull(t, nil, 100, func(r *CommandRegistry) {
		r.Register("g", "", iterCmd{iter: func() iterator.Iterator[string] {
			return iterator.Empty[string]()
		}})
	})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "g zz")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})
	h.waitCompletion()
	drainTicks(h)

	assert.False(t, h.searching, "overlay must close with no candidates")
	assert.Equal(t, "g zz", h.editBuf.String())
}
