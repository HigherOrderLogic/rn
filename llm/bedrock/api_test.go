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
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
)

func documentJSON(t *testing.T, doc document.Interface) map[string]any {
	t.Helper()
	require.NotNil(t, doc)
	raw, err := doc.MarshalSmithyDocument()
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func blockText(t *testing.T, block types.ContentBlock) string {
	t.Helper()
	text, ok := block.(*types.ContentBlockMemberText)
	require.True(t, ok, "expected a text block, got %T", block)
	return text.Value
}

func TestConverseInput_SplitsSystemAndMergesRoles(t *testing.T) {
	in := converseInput(ClaudeSonnet45, llmapi.Request{
		Messages: []llmapi.Message{
			{Role: llmapi.RoleSystem, Content: "be brief"},
			{Role: llmapi.RoleUser, Content: "hello"},
			{Role: llmapi.RoleAssistant, Content: "hi"},
			{Role: llmapi.RoleTool, ToolCallID: "call-1", Content: "result"},
			{Role: llmapi.RoleUser, Content: "thanks"},
		},
	}, Config{})

	require.Equal(t, ClaudeSonnet45, *in.ModelId)
	require.Len(t, in.System, 1)
	assert.Equal(t, "be brief", in.System[0].(*types.SystemContentBlockMemberText).Value)

	require.Len(t, in.Messages, 3)
	assert.Equal(t, types.ConversationRoleUser, in.Messages[0].Role)
	assert.Equal(t, types.ConversationRoleAssistant, in.Messages[1].Role)

	// The tool result and the following user turn merge into one message.
	assert.Equal(t, types.ConversationRoleUser, in.Messages[2].Role)
	require.Len(t, in.Messages[2].Content, 2)
	result, ok := in.Messages[2].Content[0].(*types.ContentBlockMemberToolResult)
	require.True(t, ok)
	assert.Equal(t, "call-1", *result.Value.ToolUseId)
	assert.Equal(t, "thanks", blockText(t, in.Messages[2].Content[1]))
}

func TestConverseInput_EmptyUserTurnKeepsSentinel(t *testing.T) {
	in := converseInput(NovaPro, llmapi.Request{
		Messages: []llmapi.Message{{Role: llmapi.RoleUser, Content: "   "}},
	}, Config{})

	require.Len(t, in.Messages, 1)
	require.Len(t, in.Messages[0].Content, 1)
	assert.Equal(t, emptyUserContentSentinel, blockText(t, in.Messages[0].Content[0]))
}

func TestConverseInput_ToolsBecomeToolConfig(t *testing.T) {
	in := converseInput(ClaudeSonnet45, llmapi.Request{
		Messages: []llmapi.Message{{Role: llmapi.RoleUser, Content: "go"}},
		Tools: []llmapi.Tool{{
			Type: llmapi.ToolTypeFunction,
			Function: llmapi.FunctionDefinition{
				Name:        "read_file",
				Description: "read a file",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"path": map[string]any{"type": "string"}},
					"required":   []any{"path"},
				},
			},
		}},
		ToolChoice: llmapi.ToolChoiceRequired,
	}, Config{})

	require.NotNil(t, in.ToolConfig)
	require.Len(t, in.ToolConfig.Tools, 1)
	spec, ok := in.ToolConfig.Tools[0].(*types.ToolMemberToolSpec)
	require.True(t, ok)
	assert.Equal(t, "read_file", *spec.Value.Name)
	assert.Equal(t, "read a file", *spec.Value.Description)

	schema, ok := spec.Value.InputSchema.(*types.ToolInputSchemaMemberJson)
	require.True(t, ok)
	assert.Equal(t, "object", documentJSON(t, schema.Value)["type"])

	_, isAny := in.ToolConfig.ToolChoice.(*types.ToolChoiceMemberAny)
	assert.True(t, isAny)
}

func TestConverseInput_ImageDataURIBecomesImageBlock(t *testing.T) {
	payload := []byte{0x89, 0x50, 0x4e, 0x47}
	uri := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(payload)

	in := converseInput(ClaudeSonnet45, llmapi.Request{
		Messages: []llmapi.Message{{
			Role: llmapi.RoleUser,
			MultiContent: []llmapi.ContentPart{
				{Type: llmapi.ContentPartTypeText, Text: "what is this"},
				{Type: llmapi.ContentPartTypeImageURL, ImageURL: uri},
			},
		}},
	}, Config{})

	require.Len(t, in.Messages[0].Content, 2)
	img, ok := in.Messages[0].Content[1].(*types.ContentBlockMemberImage)
	require.True(t, ok)
	assert.Equal(t, types.ImageFormatJpeg, img.Value.Format)
	source, ok := img.Value.Source.(*types.ImageSourceMemberBytes)
	require.True(t, ok)
	assert.Equal(t, payload, source.Value)
}

func TestConverseInput_RemoteImageURLIsDropped(t *testing.T) {
	in := converseInput(ClaudeSonnet45, llmapi.Request{
		Messages: []llmapi.Message{{
			Role: llmapi.RoleUser,
			MultiContent: []llmapi.ContentPart{
				{Type: llmapi.ContentPartTypeImageURL, ImageURL: "https://example.com/a.png"},
			},
		}},
	}, Config{})

	require.Len(t, in.Messages[0].Content, 1)
	assert.Equal(t, emptyUserContentSentinel, blockText(t, in.Messages[0].Content[0]))
}

func TestConverseInput_ReasoningReplayRequiresSignature(t *testing.T) {
	in := converseInput(ClaudeSonnet45, llmapi.Request{
		Messages: []llmapi.Message{
			{Role: llmapi.RoleUser, Content: "hello"},
			{
				Role:    llmapi.RoleAssistant,
				Content: "hi",
				ReasoningBlocks: []llmapi.ReasoningBlock{
					{Kind: reasoningKindThinking, Text: "unsigned"},
					{Kind: reasoningKindThinking, Text: "signed", Signature: "sig"},
					{Kind: reasoningKindRedacted, Data: base64.StdEncoding.EncodeToString([]byte("x"))},
				},
				ToolCalls: []llmapi.ToolCall{{
					ID:       "call-1",
					Type:     llmapi.ToolTypeFunction,
					Function: llmapi.FunctionCall{Name: "read_file", Arguments: `{"path":"a.go"}`},
				}},
			},
		},
	}, Config{})

	blocks := in.Messages[1].Content
	require.Len(t, blocks, 4)

	reasoning, ok := blocks[0].(*types.ContentBlockMemberReasoningContent)
	require.True(t, ok)
	text, ok := reasoning.Value.(*types.ReasoningContentBlockMemberReasoningText)
	require.True(t, ok)
	assert.Equal(t, "signed", *text.Value.Text)
	assert.Equal(t, "sig", *text.Value.Signature)

	redacted, ok := blocks[1].(*types.ContentBlockMemberReasoningContent)
	require.True(t, ok)
	raw, ok := redacted.Value.(*types.ReasoningContentBlockMemberRedactedContent)
	require.True(t, ok)
	assert.Equal(t, []byte("x"), raw.Value)

	assert.Equal(t, "hi", blockText(t, blocks[2]))

	use, ok := blocks[3].(*types.ContentBlockMemberToolUse)
	require.True(t, ok)
	assert.Equal(t, "call-1", *use.Value.ToolUseId)
	assert.Equal(t, "a.go", documentJSON(t, use.Value.Input)["path"])
}

func TestConverseInput_CachePointsOnlyWhenEnabled(t *testing.T) {
	request := llmapi.Request{
		Messages: []llmapi.Message{
			{Role: llmapi.RoleSystem, Content: "be brief"},
			{Role: llmapi.RoleUser, Content: "one"},
			{Role: llmapi.RoleAssistant, Content: "two"},
			{Role: llmapi.RoleUser, Content: "three"},
		},
		Tools: []llmapi.Tool{{
			Type:     llmapi.ToolTypeFunction,
			Function: llmapi.FunctionDefinition{Name: "noop"},
		}},
	}

	off := converseInput(ClaudeSonnet45, request, Config{})
	assert.Len(t, off.System, 1)
	assert.Len(t, off.ToolConfig.Tools, 1)
	assert.Len(t, off.Messages[1].Content, 1)

	on := converseInput(ClaudeSonnet45, request, Config{CacheControl: "default"})
	require.Len(t, on.System, 2)
	assert.IsType(t, &types.SystemContentBlockMemberCachePoint{}, on.System[1])
	require.Len(t, on.ToolConfig.Tools, 2)
	assert.IsType(t, &types.ToolMemberCachePoint{}, on.ToolConfig.Tools[1])
	// The prefix breakpoint lands on the second-to-last turn.
	require.Len(t, on.Messages[1].Content, 2)
	assert.IsType(t, &types.ContentBlockMemberCachePoint{}, on.Messages[1].Content[1])
	assert.Len(t, on.Messages[2].Content, 1)
}

func TestConverseInput_ThinkingBudgetOnlyForClaude(t *testing.T) {
	request := llmapi.Request{
		Messages:        []llmapi.Message{{Role: llmapi.RoleUser, Content: "hi"}},
		ReasoningEffort: llmapi.ReasoningEffort("medium"),
	}

	claude := converseInput(ClaudeSonnet45, request, Config{})
	fields := documentJSON(t, claude.AdditionalModelRequestFields)
	thinking, ok := fields["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", thinking["type"])
	assert.EqualValues(t, thinkingBudget["medium"], thinking["budget_tokens"])

	nova := converseInput(NovaPro, request, Config{})
	assert.Nil(t, nova.AdditionalModelRequestFields)
}

func TestInferenceConfig_MaxTokensPrecedence(t *testing.T) {
	request := llmapi.Request{Messages: []llmapi.Message{{Role: llmapi.RoleUser, Content: "hi"}}}

	fromModel := converseInput(ClaudeSonnet45, request, Config{})
	assert.EqualValues(t, MaxOutputTokens(ClaudeSonnet45), *fromModel.InferenceConfig.MaxTokens)

	fromConfig := converseInput(ClaudeSonnet45, request, Config{MaxTokens: 111})
	assert.EqualValues(t, 111, *fromConfig.InferenceConfig.MaxTokens)

	request.MaxOutputTokens = 222
	fromRequest := converseInput(ClaudeSonnet45, request, Config{MaxTokens: 111})
	assert.EqualValues(t, 222, *fromRequest.InferenceConfig.MaxTokens)
}

func TestConverseInput_ResponseFormatBecomesStructuredOutput(t *testing.T) {
	request := llmapi.Request{Messages: []llmapi.Message{{Role: llmapi.RoleUser, Content: "hi"}}}

	assert.Nil(t, converseInput(ClaudeSonnet45, request, Config{}).OutputConfig)

	request.ResponseFormat = &llmapi.ResponseFormat{
		Type: llmapi.ResponseFormatTypeJSONSchema,
		JSONSchema: &llmapi.ResponseFormatJSONSchema{
			Name:   "verdict",
			Schema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
		},
	}
	in := converseInput(ClaudeSonnet45, request, Config{})
	require.NotNil(t, in.OutputConfig)
	require.NotNil(t, in.OutputConfig.TextFormat)
	assert.Equal(t, types.OutputFormatTypeJsonSchema, in.OutputConfig.TextFormat.Type)
	schema, ok := in.OutputConfig.TextFormat.Structure.(*types.OutputFormatStructureMemberJsonSchema)
	require.True(t, ok)
	assert.Equal(t, "verdict", *schema.Value.Name)
	assert.JSONEq(t,
		`{"type":"object","properties":{"ok":{"type":"boolean"}}}`, *schema.Value.Schema)
}
