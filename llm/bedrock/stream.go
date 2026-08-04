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

package bedrock

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
	"github.com/unstablebuild/rune-go-sdk/retry"
	"unstable.build/go-tui/llm/ratelimit"
)

type streamState int

const (
	streamStateStreaming streamState = iota
	streamStateDone
)

// streamIterator drains a ConverseStream event stream and emits typed
// llmapi.Event values. The stream is dialled lazily on the first Next call
// so CreateCompletion performs no network I/O.
type streamIterator struct {
	// newStream dials one ConverseStream attempt. It is used for the lazy
	// initial connect (with retry) and for mid-stream retries.
	newStream func(context.Context) (*bedrockruntime.ConverseStreamEventStream, error)
	stream    *bedrockruntime.ConverseStreamEventStream
	state     streamState

	textContent      strings.Builder
	reasoningContent strings.Builder
	toolCalls        []llmapi.ToolCall
	// pendingCalls accumulates tool-call arguments by content block index
	// until the matching contentBlockStop.
	pendingCalls map[int32]*llmapi.ToolCall

	// reasoningBlocks holds finalized reasoning in stream order so the
	// assistant turn can be replayed; Converse requires the signature to be
	// returned unmodified alongside the reasoning text.
	reasoningBlocks  []llmapi.ReasoningBlock
	pendingReasoning map[int32]*llmapi.ReasoningBlock

	usage      types.TokenUsage
	stopReason types.StopReason

	err error

	pendingWarnings []llmapi.Event
	warningIdx      int

	// Mid-stream retry support.
	midStreamRetries int
	retryEvents      []llmapi.Event
	retryEventIdx    int
}

func (s *streamIterator) Next(ctx context.Context) (llmapi.Event, bool) {
	for {
		// Buffered warnings and retry events drain before anything else so
		// they keep their position relative to the stream that produced them.
		if s.warningIdx < len(s.pendingWarnings) {
			ev := s.pendingWarnings[s.warningIdx]
			s.warningIdx++
			return ev, true
		}
		if s.retryEventIdx < len(s.retryEvents) {
			ev := s.retryEvents[s.retryEventIdx]
			s.retryEventIdx++
			return ev, true
		}

		switch s.state {
		case streamStateDone:
			return llmapi.Event{}, false

		case streamStateStreaming:
			if s.stream == nil {
				if err := s.connect(ctx); err != nil {
					s.state = streamStateDone
					s.err = err
					// Queue the error behind any warnings collected while
					// retrying the connect.
					s.retryEvents = append(s.retryEvents,
						llmapi.Event{Type: llmapi.EventStreamError, Error: err})
				}
				continue
			}
			event, ok := s.receive(ctx)
			if !ok {
				if ev, emit := s.handleStreamEnd(ctx); emit {
					return ev, true
				}
				continue
			}
			if ev, emit := s.handleStreamEvent(event); emit {
				return ev, true
			}
		}
	}
}

// connect establishes the initial stream. Bedrock returns throttling and
// capacity errors on the connect itself, before any event is delivered;
// those are retried here with backoff, surfacing each wait as a warning
// event so the caller sees a delay rather than a failed turn.
func (s *streamIterator) connect(ctx context.Context) error {
	var warnings []llmapi.Event
	// Bedrock surfaces throttling through typed errors rather than response
	// headers, so the header set stays nil and the strategy falls back to
	// exponential backoff.
	var headers http.Header
	strategy := retry.CombinedStrategy(
		retry.LimitStrategy(ratelimit.MaxStreamRetries+1),
		ratelimit.RetryAfterOrBackoffStrategy(&headers, &warnings),
	)
	var stream *bedrockruntime.ConverseStreamEventStream
	err := retry.Retry(ctx, strategy, func(ctx context.Context) (bool, error) {
		var dialErr error
		stream, dialErr = s.newStream(ctx)
		if dialErr == nil {
			return false, nil
		}
		return isRetryableError(dialErr), dialErr
	})
	s.pendingWarnings = append(s.pendingWarnings, warnings...)
	if err != nil {
		return err
	}
	s.stream = stream
	return nil
}

// receive reads the next event, honouring caller cancellation. The SDK
// closes the event channel on both clean completion and failure; the
// distinction is made by the stream's Err.
func (s *streamIterator) receive(ctx context.Context) (types.ConverseStreamOutput, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case event, ok := <-s.stream.Events():
		if !ok || event == nil {
			return nil, false
		}
		return event, true
	}
}

// handleStreamEnd decides what to do when the event channel closes: retry,
// surface the error, or finish the turn.
func (s *streamIterator) handleStreamEnd(ctx context.Context) (llmapi.Event, bool) {
	err := s.stream.Err()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		s.state = streamStateDone
		return s.buildDoneEvent(), true
	}

	if ev, ok := s.retryStream(ctx, err); ok {
		return ev, true
	}

	s.state = streamStateDone
	s.err = err
	slog.Warn("bedrock completion stream error",
		"error", err,
		"accumulated_text_len", s.textContent.Len(),
		"accumulated_reasoning_len", s.reasoningContent.Len(),
		"accumulated_tool_calls", len(s.toolCalls),
	)
	return llmapi.Event{Type: llmapi.EventStreamError, Error: err}, true
}

// retryStream re-establishes the stream after a transient failure and
// queues the reset + warning events the consumer needs to discard the
// partial turn.
func (s *streamIterator) retryStream(ctx context.Context, err error) (llmapi.Event, bool) {
	if s.newStream == nil || s.midStreamRetries <= 0 || !isRetryableError(err) {
		return llmapi.Event{}, false
	}
	s.midStreamRetries--
	_ = s.stream.Close()

	attempt := ratelimit.MaxMidStreamRetries - s.midStreamRetries
	wait := retryWait(nil, attempt-1)
	slog.Warn("bedrock mid-stream retryable error, retrying",
		"error", err, "attempt", attempt, "wait", wait)

	select {
	case <-ctx.Done():
		return llmapi.Event{}, false
	case <-time.After(wait):
	}

	stream, newErr := s.newStream(ctx)
	if newErr != nil {
		return llmapi.Event{}, false
	}
	s.stream = stream
	s.resetAccumulator()

	msg := ratelimit.RetryStreamMessage(err, wait, attempt, ratelimit.MaxMidStreamRetries)
	if ratelimit.IsTransientNetworkError(err) {
		msg = ratelimit.RetryNetworkMessage(err, wait, attempt, ratelimit.MaxMidStreamRetries)
	}
	s.retryEvents = []llmapi.Event{
		{Type: llmapi.EventStreamReset},
		{Type: llmapi.EventRateLimitWarning, RateLimit: &llmapi.RateLimitInfo{
			WaitDuration: wait,
			Message:      msg,
		}},
	}
	s.retryEventIdx = 1
	return s.retryEvents[0], true
}

// handleStreamEvent folds one Converse stream event into the accumulator and
// reports the llmapi.Event to emit, if any.
func (s *streamIterator) handleStreamEvent(event types.ConverseStreamOutput) (llmapi.Event, bool) {
	switch ev := event.(type) {
	case *types.ConverseStreamOutputMemberContentBlockStart:
		s.handleBlockStart(ev.Value)

	case *types.ConverseStreamOutputMemberContentBlockDelta:
		return s.handleBlockDelta(ev.Value)

	case *types.ConverseStreamOutputMemberContentBlockStop:
		return s.handleBlockStop(ev.Value)

	case *types.ConverseStreamOutputMemberMessageStop:
		// The turn is only complete once the trailing metadata event has
		// been folded in, so the done event waits for the stream to close.
		s.stopReason = ev.Value.StopReason

	case *types.ConverseStreamOutputMemberMetadata:
		if ev.Value.Usage != nil {
			s.usage = *ev.Value.Usage
		}
	}
	return llmapi.Event{}, false
}

func (s *streamIterator) handleBlockStart(ev types.ContentBlockStartEvent) {
	start, ok := ev.Start.(*types.ContentBlockStartMemberToolUse)
	if !ok {
		return
	}
	if s.pendingCalls == nil {
		s.pendingCalls = make(map[int32]*llmapi.ToolCall)
	}
	s.pendingCalls[deref(ev.ContentBlockIndex)] = &llmapi.ToolCall{
		ID:   derefString(start.Value.ToolUseId),
		Type: llmapi.ToolTypeFunction,
		Function: llmapi.FunctionCall{
			Name: derefString(start.Value.Name),
		},
	}
}

func (s *streamIterator) handleBlockDelta(ev types.ContentBlockDeltaEvent) (llmapi.Event, bool) {
	index := deref(ev.ContentBlockIndex)
	switch delta := ev.Delta.(type) {
	case *types.ContentBlockDeltaMemberText:
		s.textContent.WriteString(delta.Value)
		return llmapi.Event{Type: llmapi.EventTextDelta, Text: delta.Value}, true

	case *types.ContentBlockDeltaMemberToolUse:
		if tc, ok := s.pendingCalls[index]; ok {
			tc.Function.Arguments += derefString(delta.Value.Input)
		}

	case *types.ContentBlockDeltaMemberReasoningContent:
		return s.handleReasoningDelta(index, delta.Value)
	}
	return llmapi.Event{}, false
}

func (s *streamIterator) handleReasoningDelta(
	index int32, delta types.ReasoningContentBlockDelta,
) (llmapi.Event, bool) {
	switch d := delta.(type) {
	case *types.ReasoningContentBlockDeltaMemberText:
		s.reasoningContent.WriteString(d.Value)
		s.pendingReasoningBlock(index, reasoningKindThinking).Text += d.Value
		return llmapi.Event{Type: llmapi.EventReasoningDelta, Reasoning: d.Value}, true

	case *types.ReasoningContentBlockDeltaMemberSignature:
		s.pendingReasoningBlock(index, reasoningKindThinking).Signature += d.Value

	case *types.ReasoningContentBlockDeltaMemberRedactedContent:
		block := s.pendingReasoningBlock(index, reasoningKindRedacted)
		block.Kind = reasoningKindRedacted
		block.Data += base64.StdEncoding.EncodeToString(d.Value)
	}
	return llmapi.Event{}, false
}

func (s *streamIterator) pendingReasoningBlock(index int32, kind string) *llmapi.ReasoningBlock {
	if s.pendingReasoning == nil {
		s.pendingReasoning = make(map[int32]*llmapi.ReasoningBlock)
	}
	block, ok := s.pendingReasoning[index]
	if !ok {
		block = &llmapi.ReasoningBlock{Kind: kind}
		s.pendingReasoning[index] = block
	}
	return block
}

func (s *streamIterator) handleBlockStop(ev types.ContentBlockStopEvent) (llmapi.Event, bool) {
	index := deref(ev.ContentBlockIndex)
	if tc, ok := s.pendingCalls[index]; ok {
		delete(s.pendingCalls, index)
		s.toolCalls = append(s.toolCalls, *tc)
		return llmapi.Event{Type: llmapi.EventToolCallDone, ToolCall: tc}, true
	}
	if block, ok := s.pendingReasoning[index]; ok {
		delete(s.pendingReasoning, index)
		s.reasoningBlocks = append(s.reasoningBlocks, *block)
	}
	return llmapi.Event{}, false
}

func (s *streamIterator) buildDoneEvent() llmapi.Event {
	msg := llmapi.Message{
		Role:             llmapi.RoleAssistant,
		Content:          s.textContent.String(),
		ReasoningContent: s.reasoningContent.String(),
		ToolCalls:        s.toolCalls,
		ReasoningBlocks:  s.reasoningBlocks,
	}

	cacheRead := int(deref(s.usage.CacheReadInputTokens))
	cacheWrite := int(deref(s.usage.CacheWriteInputTokens))

	return llmapi.Event{
		Type: llmapi.EventStreamDone,
		DoneData: &llmapi.DoneData{
			Message:      msg,
			FinishReason: mapStopReason(s.stopReason),
			Usage: llmapi.Usage{
				// Bedrock reports inputTokens as the full input count,
				// cache buckets included.
				TokensSent:         int(deref(s.usage.InputTokens)),
				TokensReceived:     int(deref(s.usage.OutputTokens)),
				TokensCached:       cacheRead,
				TokensCacheCreated: cacheWrite,
			},
		},
	}
}

func (s *streamIterator) resetAccumulator() {
	s.textContent.Reset()
	s.reasoningContent.Reset()
	s.toolCalls = nil
	s.pendingCalls = nil
	s.reasoningBlocks = nil
	s.pendingReasoning = nil
	s.usage = types.TokenUsage{}
	s.stopReason = ""
}

func (s *streamIterator) Err() error { return s.err }

func (s *streamIterator) Close() error {
	if s.stream == nil {
		return nil
	}
	err := s.stream.Close()
	// Close reports the stream's terminal error, which Next has already
	// surfaced as an EventStreamError; re-reporting it here would make
	// every failed turn look like a teardown failure too.
	if err != nil && err == s.err {
		return nil
	}
	return err
}

// mapStopReason converts Converse stop reasons to llmapi.FinishReason.
func mapStopReason(reason types.StopReason) llmapi.FinishReason {
	switch reason {
	case types.StopReasonEndTurn, types.StopReasonStopSequence:
		return llmapi.FinishReasonStop
	case types.StopReasonMaxTokens, types.StopReasonModelContextWindowExceeded:
		return llmapi.FinishReasonLength
	case types.StopReasonToolUse:
		return llmapi.FinishReasonToolCall
	case types.StopReasonGuardrailIntervened, types.StopReasonContentFiltered:
		return llmapi.FinishReasonContentFilter
	default:
		slog.Warn("bedrock: unmapped stop_reason", "reason", reason)
		return llmapi.FinishReasonNull
	}
}

func deref(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// retryWait is indirected so tests can drive the retry path without
// sleeping for the real backoff.
var retryWait = ratelimit.RetryWait
