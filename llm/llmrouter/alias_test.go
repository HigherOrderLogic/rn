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

package llmrouter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
	"unstable.build/go-tui/llm/anthropic"
	"unstable.build/go-tui/llm/gemini"
	"unstable.build/go-tui/llm/openai"
)

func TestRouter_GetModel_ResolvesAlias(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	require.NoError(t, r.SetAlias(ctx, "default", "openai/"+openai.GPT5Dot5))

	got, err := r.GetModel(ctx, llmapi.ModelEntry{Name: "default"})
	require.NoError(t, err)
	assert.Equal(t, ProviderOpenAI, got.Provider)
	assert.Equal(t, openai.GPT5Dot5, got.Name)
}

func TestRouter_SetAlias_RejectsUnknownTarget(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	err := r.SetAlias(ctx, "default", "openai/does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an available model")

	err = r.SetAlias(ctx, "default", "bare-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider/name form")
}

func TestRouter_RemoveAlias(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	require.NoError(t, r.SetAlias(ctx, "default", "openai/"+openai.GPT5Dot5))
	require.NoError(t, r.RemoveAlias(ctx, "default"))
	require.ErrorIs(t, r.RemoveAlias(ctx, "default"), ErrAliasNotFound)
}

// TestRouter_Alias_UnsetReservedFallsBackToFlagship verifies that an
// unset reserved alias resolves, via `default`, to the flagship of the
// most-recently authenticated hosted provider.
func TestRouter_Alias_UnsetReservedFallsBackToFlagship(t *testing.T) {
	ctx := context.Background()
	r := newTestRouter(t)
	// The key document's server-managed UpdatedAt advances on every add,
	// so the most-recently authenticated provider wins. A short sleep
	// guarantees the millisecond-resolution timestamps differ.
	require.NoError(t, r.store.add(ctx, ProviderAnthropic, "k", "key-a", ""))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, r.store.add(ctx, ProviderGemini, "k", "key-g", ""))

	for _, name := range []string{llmapi.DefaultModel, "query"} {
		got, err := r.GetModel(ctx, llmapi.ModelEntry{Name: name})
		require.NoError(t, err, name)
		assert.Equal(t, ProviderGemini, got.Provider, name)
		assert.Equal(t, gemini.FlagshipModel(), got.Name, name)
	}

	// Re-authenticate anthropic so it becomes the most recent; the
	// fallback flips.
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, r.store.add(ctx, ProviderAnthropic, "k2", "key-a2", ""))
	got, err := r.GetModel(ctx, llmapi.ModelEntry{Name: llmapi.DefaultModel})
	require.NoError(t, err)
	assert.Equal(t, ProviderAnthropic, got.Provider)
	assert.Equal(t, anthropic.FlagshipModel(), got.Name)
}

// TestRouter_Alias_NoAuthFallsBackToFirstModel verifies that with no
// authenticated provider, an unset reserved alias resolves to the first
// catalog entry rather than failing.
func TestRouter_Alias_NoAuthFallsBackToFirstModel(t *testing.T) {
	ctx := context.Background()
	r := newTestRouter(t)

	// With no authenticated provider the reserved alias falls back to a
	// catalog model. The concrete model is not asserted because Models()
	// draws from map-ordered provider catalogs; only that resolution
	// succeeds and yields a provider-qualified entry.
	got, err := r.GetModel(ctx, llmapi.ModelEntry{Name: llmapi.DefaultModel})
	require.NoError(t, err)
	assert.NotEmpty(t, got.Provider)
	assert.NotEmpty(t, got.Name)
}

// TestRouter_Alias_UnknownBareNameStillUnresolved verifies that a bare
// name that is neither a stored alias nor a reserved name is not treated
// as an alias.
func TestRouter_Alias_UnknownBareNameStillUnresolved(t *testing.T) {
	ctx := context.Background()
	r := newTestRouter(t)
	_, err := r.GetModel(ctx, llmapi.ModelEntry{Name: "gpt-5.5"})
	assert.ErrorIs(t, err, llmapi.ErrModelNotFound)
}

// TestRouter_Alias_UnsetReservedDefersToDefault verifies that an unset
// reserved alias resolves to whatever `default` resolves to, including a
// `default` set to an explicit model.
func TestRouter_Alias_UnsetReservedDefersToDefault(t *testing.T) {
	ctx := context.Background()
	r := newTestRouter(t)
	require.NoError(t, r.SetAlias(ctx, llmapi.DefaultModel, "openai/"+openai.GPT5Dot5))

	for _, name := range []string{"query", "compact", "dream"} {
		got, err := r.GetModel(ctx, llmapi.ModelEntry{Name: name})
		require.NoError(t, err, name)
		assert.Equal(t, ProviderOpenAI, got.Provider, name)
		assert.Equal(t, openai.GPT5Dot5, got.Name, name)
	}
}

// TestRouter_Alias_SetReservedOverridesDefault verifies that a reserved
// alias set to its own target does not defer to `default`.
func TestRouter_Alias_SetReservedOverridesDefault(t *testing.T) {
	ctx := context.Background()
	r := newTestRouter(t)
	require.NoError(t, r.SetAlias(ctx, llmapi.DefaultModel, "openai/"+openai.GPT5Dot5))
	require.NoError(t, r.SetAlias(ctx, "compact", "anthropic/"+anthropic.ClaudeOpus4Dot8))

	got, err := r.GetModel(ctx, llmapi.ModelEntry{Name: "compact"})
	require.NoError(t, err)
	assert.Equal(t, ProviderAnthropic, got.Provider)
	assert.Equal(t, anthropic.ClaudeOpus4Dot8, got.Name)
}

// TestRouter_Alias_ExplicitProviderBypassesAlias verifies a qualified
// model entry never engages alias resolution even when its name matches
// an alias.
func TestRouter_Alias_ExplicitProviderBypassesAlias(t *testing.T) {
	ctx := context.Background()
	r := newTestRouter(t)
	require.NoError(t, r.SetAlias(ctx, "default", "openai/"+openai.GPT5Dot5))

	// "default" is not a real provider, so a qualified lookup fails
	// rather than resolving the alias.
	_, err := r.GetModel(ctx, llmapi.ModelEntry{Provider: ProviderAnthropic, Name: "default"})
	assert.ErrorIs(t, err, llmapi.ErrModelNotFound)
}
