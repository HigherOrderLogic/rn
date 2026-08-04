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
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
)

// fakeReader is a ConverseStreamOutputReader that replays a scripted list of
// events and then reports err (nil for a clean end of stream).
type fakeReader struct {
	events chan types.ConverseStreamOutput
	err    error
	closed bool
}

func (r *fakeReader) Events() <-chan types.ConverseStreamOutput { return r.events }

func (r *fakeReader) Close() error {
	if !r.closed {
		r.closed = true
	}
	return nil
}

func (r *fakeReader) Err() error { return r.err }

func scriptedStream(
	err error, events ...types.ConverseStreamOutput,
) *bedrockruntime.ConverseStreamEventStream {
	ch := make(chan types.ConverseStreamOutput, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	reader := &fakeReader{events: ch, err: err}
	return bedrockruntime.NewConverseStreamEventStream(
		func(es *bedrockruntime.ConverseStreamEventStream) { es.Reader = reader })
}

func drain(t *testing.T, it *streamIterator) []llmapi.Event {
	t.Helper()
	var out []llmapi.Event
	for {
		ev, ok := it.Next(t.Context())
		if !ok {
			return out
		}
		out = append(out, ev)
		require.LessOrEqual(t, len(out), 100, "iterator did not terminate")
	}
}

func textDelta(index int32, text string) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberContentBlockDelta{
		Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta:             &types.ContentBlockDeltaMemberText{Value: text},
		},
	}
}

func toolUseStart(index int32, id, name string) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberContentBlockStart{
		Value: types.ContentBlockStartEvent{
			ContentBlockIndex: aws.Int32(index),
			Start: &types.ContentBlockStartMemberToolUse{
				Value: types.ToolUseBlockStart{
					ToolUseId: aws.String(id),
					Name:      aws.String(name),
				},
			},
		},
	}
}

func toolUseDelta(index int32, partial string) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberContentBlockDelta{
		Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta: &types.ContentBlockDeltaMemberToolUse{
				Value: types.ToolUseBlockDelta{Input: aws.String(partial)},
			},
		},
	}
}

func reasoningDelta(index int32, delta types.ReasoningContentBlockDelta) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberContentBlockDelta{
		Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta:             &types.ContentBlockDeltaMemberReasoningContent{Value: delta},
		},
	}
}

func blockStop(index int32) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberContentBlockStop{
		Value: types.ContentBlockStopEvent{ContentBlockIndex: aws.Int32(index)},
	}
}

func messageStop(reason types.StopReason) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberMessageStop{
		Value: types.MessageStopEvent{StopReason: reason},
	}
}

func metadata(usage types.TokenUsage) types.ConverseStreamOutput {
	return &types.ConverseStreamOutputMemberMetadata{
		Value: types.ConverseStreamMetadataEvent{Usage: &usage},
	}
}

func doneEvent(t *testing.T, events []llmapi.Event) *llmapi.DoneData {
	t.Helper()
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, llmapi.EventStreamDone, last.Type)
	require.NotNil(t, last.DoneData)
	return last.DoneData
}

func TestStreamIterator_TextAndUsage(t *testing.T) {
	it := &streamIterator{stream: scriptedStream(nil,
		textDelta(0, "hello "),
		textDelta(0, "world"),
		blockStop(0),
		messageStop(types.StopReasonEndTurn),
		metadata(types.TokenUsage{
			InputTokens:           aws.Int32(120),
			OutputTokens:          aws.Int32(7),
			CacheReadInputTokens:  aws.Int32(80),
			CacheWriteInputTokens: aws.Int32(30),
		}),
	)}

	events := drain(t, it)
	require.Len(t, events, 3)
	assert.Equal(t, llmapi.EventTextDelta, events[0].Type)
	assert.Equal(t, "hello ", events[0].Text)
	assert.Equal(t, "world", events[1].Text)

	done := doneEvent(t, events)
	assert.Equal(t, "hello world", done.Message.Content)
	assert.Equal(t, llmapi.FinishReasonStop, done.FinishReason)
	assert.Equal(t, 120, done.Usage.TokensSent)
	assert.Equal(t, 7, done.Usage.TokensReceived)
	assert.Equal(t, 80, done.Usage.TokensCached)
	assert.Equal(t, 30, done.Usage.TokensCacheCreated)
	assert.NoError(t, it.Err())
}

func TestStreamIterator_ToolCallAssembly(t *testing.T) {
	it := &streamIterator{stream: scriptedStream(nil,
		toolUseStart(0, "call-1", "read_file"),
		toolUseDelta(0, `{"path":`),
		toolUseDelta(0, `"main.go"}`),
		blockStop(0),
		messageStop(types.StopReasonToolUse),
	)}

	events := drain(t, it)
	require.Len(t, events, 2)
	require.Equal(t, llmapi.EventToolCallDone, events[0].Type)
	require.NotNil(t, events[0].ToolCall)
	assert.Equal(t, "call-1", events[0].ToolCall.ID)
	assert.Equal(t, "read_file", events[0].ToolCall.Function.Name)
	assert.JSONEq(t, `{"path":"main.go"}`, events[0].ToolCall.Function.Arguments)

	done := doneEvent(t, events)
	assert.Equal(t, llmapi.FinishReasonToolCall, done.FinishReason)
	require.Len(t, done.Message.ToolCalls, 1)
	assert.JSONEq(t, `{"path":"main.go"}`, done.Message.ToolCalls[0].Function.Arguments)
}

func TestStreamIterator_ParallelToolCallsKeepTheirArguments(t *testing.T) {
	it := &streamIterator{stream: scriptedStream(nil,
		toolUseStart(0, "call-a", "read_file"),
		toolUseStart(1, "call-b", "write_file"),
		toolUseDelta(1, `{"b":2}`),
		toolUseDelta(0, `{"a":1}`),
		blockStop(1),
		blockStop(0),
		messageStop(types.StopReasonToolUse),
	)}

	done := doneEvent(t, drain(t, it))
	require.Len(t, done.Message.ToolCalls, 2)
	assert.Equal(t, "call-b", done.Message.ToolCalls[0].ID)
	assert.JSONEq(t, `{"b":2}`, done.Message.ToolCalls[0].Function.Arguments)
	assert.Equal(t, "call-a", done.Message.ToolCalls[1].ID)
	assert.JSONEq(t, `{"a":1}`, done.Message.ToolCalls[1].Function.Arguments)
}

func TestStreamIterator_ReasoningPreservesSignature(t *testing.T) {
	it := &streamIterator{stream: scriptedStream(nil,
		reasoningDelta(0, &types.ReasoningContentBlockDeltaMemberText{Value: "let me "}),
		reasoningDelta(0, &types.ReasoningContentBlockDeltaMemberText{Value: "think"}),
		reasoningDelta(0, &types.ReasoningContentBlockDeltaMemberSignature{Value: "sig-"}),
		reasoningDelta(0, &types.ReasoningContentBlockDeltaMemberSignature{Value: "abc=="}),
		blockStop(0),
		textDelta(1, "the answer"),
		blockStop(1),
		messageStop(types.StopReasonEndTurn),
	)}

	events := drain(t, it)
	assert.Equal(t, llmapi.EventReasoningDelta, events[0].Type)
	assert.Equal(t, "let me ", events[0].Reasoning)

	done := doneEvent(t, events)
	assert.Equal(t, "let me think", done.Message.ReasoningContent)
	assert.Equal(t, "the answer", done.Message.Content)
	require.Len(t, done.Message.ReasoningBlocks, 1)
	assert.Equal(t, reasoningKindThinking, done.Message.ReasoningBlocks[0].Kind)
	assert.Equal(t, "let me think", done.Message.ReasoningBlocks[0].Text)
	assert.Equal(t, "sig-abc==", done.Message.ReasoningBlocks[0].Signature)
}

func TestStreamIterator_RedactedReasoningIsBase64Encoded(t *testing.T) {
	it := &streamIterator{stream: scriptedStream(nil,
		reasoningDelta(0, &types.ReasoningContentBlockDeltaMemberRedactedContent{
			Value: []byte("secret"),
		}),
		blockStop(0),
		messageStop(types.StopReasonEndTurn),
	)}

	done := doneEvent(t, drain(t, it))
	require.Len(t, done.Message.ReasoningBlocks, 1)
	assert.Equal(t, reasoningKindRedacted, done.Message.ReasoningBlocks[0].Kind)
	assert.Equal(t, "c2VjcmV0", done.Message.ReasoningBlocks[0].Data)
}

func TestStreamIterator_StreamErrorIsSurfaced(t *testing.T) {
	wantErr := errors.New("boom")
	it := &streamIterator{stream: scriptedStream(wantErr, textDelta(0, "partial"))}

	events := drain(t, it)
	require.Len(t, events, 2)
	assert.Equal(t, llmapi.EventStreamError, events[1].Type)
	assert.Equal(t, wantErr, events[1].Error)
	assert.Equal(t, wantErr, it.Err())
	assert.NoError(t, it.Close())
}

func TestStreamIterator_MidStreamRetryResetsAccumulator(t *testing.T) {
	prev := retryWait
	retryWait = func(http.Header, int) time.Duration { return 0 }
	defer func() { retryWait = prev }()

	retryStream := scriptedStream(nil,
		textDelta(0, "second try"),
		blockStop(0),
		messageStop(types.StopReasonEndTurn),
	)
	it := &streamIterator{
		stream:           scriptedStream(&types.ThrottlingException{}, textDelta(0, "first")),
		midStreamRetries: 1,
		newStream: func(context.Context) (*bedrockruntime.ConverseStreamEventStream, error) {
			return retryStream, nil
		},
	}

	events := drain(t, it)
	require.Len(t, events, 5)
	assert.Equal(t, llmapi.EventTextDelta, events[0].Type)
	assert.Equal(t, llmapi.EventStreamReset, events[1].Type)
	assert.Equal(t, llmapi.EventRateLimitWarning, events[2].Type)
	assert.Equal(t, llmapi.EventTextDelta, events[3].Type)

	done := doneEvent(t, events)
	assert.Equal(t, "second try", done.Message.Content)
	assert.NoError(t, it.Err())
}

func TestStreamIterator_ContextCancellationEndsStream(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// A stream that never delivers an event nor closes: only cancellation
	// can end it.
	stalled := bedrockruntime.NewConverseStreamEventStream(
		func(es *bedrockruntime.ConverseStreamEventStream) {
			es.Reader = &fakeReader{events: make(chan types.ConverseStreamOutput)}
		})
	it := &streamIterator{stream: stalled}
	ev, ok := it.Next(ctx)
	require.True(t, ok)
	assert.Equal(t, llmapi.EventStreamError, ev.Type)
	assert.ErrorIs(t, it.Err(), context.Canceled)
}

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		name string
		in   types.StopReason
		want llmapi.FinishReason
		warn bool
	}{
		{"end_turn", types.StopReasonEndTurn, llmapi.FinishReasonStop, false},
		{"stop_sequence", types.StopReasonStopSequence, llmapi.FinishReasonStop, false},
		{"max_tokens", types.StopReasonMaxTokens, llmapi.FinishReasonLength, false},
		{"context window exceeded", types.StopReasonModelContextWindowExceeded, llmapi.FinishReasonLength, false},
		{"tool_use", types.StopReasonToolUse, llmapi.FinishReasonToolCall, false},
		{"guardrail", types.StopReasonGuardrailIntervened, llmapi.FinishReasonContentFilter, false},
		{"content_filtered", types.StopReasonContentFiltered, llmapi.FinishReasonContentFilter, false},
		{"unknown", types.StopReason("frobnicate"), llmapi.FinishReasonNull, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
			defer slog.SetDefault(prev)

			assert.Equal(t, tt.want, mapStopReason(tt.in))

			warned := strings.Contains(buf.String(), "unmapped stop_reason")
			assert.Equal(t, tt.warn, warned, "warn log expectation mismatch: %q", buf.String())
		})
	}
}
