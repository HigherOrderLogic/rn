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
	"fmt"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
)

// LLMProvider identifies the AWS Bedrock provider in the model registry.
const LLMProvider = "bedrock"

// Bedrock addresses models either by raw foundation-model ID or by
// inference-profile ID. The static catalog uses US cross-region inference
// profiles ("us." prefix) because on-demand throughput for the current
// generation of models is only offered through them.
const (
	// ClaudeOpus45 is Anthropic's Claude Opus 4.5 on Bedrock.
	ClaudeOpus45 = "us.anthropic.claude-opus-4-5-20251101-v1:0"
	// ClaudeSonnet45 is Anthropic's Claude Sonnet 4.5 on Bedrock.
	ClaudeSonnet45 = "us.anthropic.claude-sonnet-4-5-20250929-v1:0"
	// ClaudeHaiku45 is Anthropic's Claude Haiku 4.5 on Bedrock.
	ClaudeHaiku45 = "us.anthropic.claude-haiku-4-5-20251001-v1:0"
	// ClaudeOpus41 is Anthropic's Claude Opus 4.1 on Bedrock.
	ClaudeOpus41 = "us.anthropic.claude-opus-4-1-20250805-v1:0"
	// ClaudeSonnet4 is Anthropic's Claude Sonnet 4 on Bedrock.
	ClaudeSonnet4 = "us.anthropic.claude-sonnet-4-20250514-v1:0"
	// NovaPremier is Amazon's Nova Premier model.
	NovaPremier = "us.amazon.nova-premier-v1:0"
	// NovaPro is Amazon's Nova Pro model.
	NovaPro = "us.amazon.nova-pro-v1:0"
	// NovaLite is Amazon's Nova Lite model.
	NovaLite = "us.amazon.nova-lite-v1:0"
	// NovaMicro is Amazon's Nova Micro model.
	NovaMicro = "us.amazon.nova-micro-v1:0"
	// Llama4Maverick is Meta's Llama 4 Maverick model.
	Llama4Maverick = "us.meta.llama4-maverick-17b-instruct-v1:0"
	// Llama4Scout is Meta's Llama 4 Scout model.
	Llama4Scout = "us.meta.llama4-scout-17b-instruct-v1:0"
	// DeepSeekR1 is DeepSeek's R1 reasoning model.
	DeepSeekR1 = "us.deepseek.r1-v1:0"
	// MistralLarge is Mistral's Large 2 model.
	MistralLarge = "mistral.mistral-large-2407-v1:0"
)

// AvailableModels returns the static fallback catalog: a map from Bedrock
// model or inference-profile identifier -> nominal maximum context window
// (tokens). Model access varies per AWS account and region, so this list is
// only a starting point — the live catalog (see Client.Models) reports what
// the caller's account can actually invoke.
func AvailableModels() map[string]int {
	return map[string]int{
		ClaudeOpus45:   200000,
		ClaudeSonnet45: 200000,
		ClaudeHaiku45:  200000,
		ClaudeOpus41:   200000,
		ClaudeSonnet4:  200000,
		NovaPremier:    1000000,
		NovaPro:        300000,
		NovaLite:       300000,
		NovaMicro:      128000,
		Llama4Maverick: 1000000,
		Llama4Scout:    1000000,
		DeepSeekR1:     128000,
		MistralLarge:   128000,
	}
}

// maxOutput maps each catalog entry to its documented maximum output-token
// ceiling. Entries absent from the map have an unknown ceiling;
// MaxOutputTokens returns 0 for them so callers apply no cap.
var maxOutput = map[string]int{
	ClaudeOpus45:   64000,
	ClaudeSonnet45: 64000,
	ClaudeHaiku45:  64000,
	ClaudeOpus41:   32000,
	ClaudeSonnet4:  64000,
	NovaPremier:    32000,
	NovaPro:        10000,
	NovaLite:       10000,
	NovaMicro:      10000,
	Llama4Maverick: 8192,
	Llama4Scout:    8192,
	DeepSeekR1:     32768,
	MistralLarge:   8192,
}

// MaxOutputTokens returns the model's documented maximum output-token
// ceiling, or 0 when the limit is unknown.
func MaxOutputTokens(model string) int { return maxOutput[model] }

// IsClaudeModel reports whether the Bedrock identifier addresses an
// Anthropic Claude model. Claude is the only family whose extended thinking
// is configured through additionalModelRequestFields.
func IsClaudeModel(model string) bool {
	return strings.Contains(model, "anthropic.claude")
}

// SupportsEffort reports whether reasoning effort can be mapped onto the
// model's request parameters. Only the Claude family exposes a thinking
// budget through the Converse API; other families either always reason
// (DeepSeek R1) or not at all.
func SupportsEffort(model string) bool {
	// Claude 3 predates extended thinking.
	return IsClaudeModel(model) && !strings.Contains(model, "claude-3")
}

// thinkingBudget maps a normalized effort level onto the Claude extended
// thinking token budget sent in additionalModelRequestFields.
var thinkingBudget = map[string]int{
	"low":    4096,
	"medium": 16384,
	"high":   32768,
}

// NormalizeEffort validates the requested effort level against the given
// model's capabilities. It returns the effort to use (empty string means
// omit the parameter entirely) and a human-readable warning when the
// requested effort is not supported by the model.
func NormalizeEffort(model, effort string) (normalized string, warning string) {
	if effort == "" {
		return "", ""
	}
	if !SupportsEffort(model) {
		return "", fmt.Sprintf(
			"Effort %q is not supported by %s; using model default instead.", effort, model)
	}
	if _, ok := thinkingBudget[effort]; ok {
		return effort, ""
	}
	return "", fmt.Sprintf(
		"Effort %q is not supported by %s; using model default instead.", effort, model)
}

// ModelEntries returns the static fallback catalog as router-ready model
// entries.
func ModelEntries() []llmapi.ModelEntry {
	avail := AvailableModels()
	out := make([]llmapi.ModelEntry, 0, len(avail))
	for name, ctxWindow := range avail {
		out = append(out, llmapi.ModelEntry{
			Name:          name,
			Provider:      LLMProvider,
			ContextWindow: ctxWindow,
		})
	}
	return out
}

// FlagshipModel returns the provider's top model identifier. It is
// deterministic, unlike iterating ModelEntries() whose order is map-random.
func FlagshipModel() string { return ClaudeOpus45 }

// VerificationModelForRegion returns the flagship inference profile that
// serves the given AWS region. Cross-region profiles are partitioned by
// geography ("us.", "eu.", "apac."), so a key scoped to a European region
// must be verified against the "eu." profile; the US profile would fail
// with a model-access error even when the key is valid.
func VerificationModelForRegion(region string) string {
	return RegionalProfile(FlagshipModel(), region)
}

// RegionalProfile rewrites a cross-region inference profile ID so it
// addresses the geography serving the given AWS region.
func RegionalProfile(model, region string) string {
	return regionPrefixFor(region) + stripRegionPrefix(model)
}

// regionPrefixFor returns the cross-region inference profile prefix that
// serves the given AWS region.
func regionPrefixFor(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov-"):
		return "us-gov."
	case strings.HasPrefix(region, "eu-"):
		return "eu."
	case strings.HasPrefix(region, "ap-"):
		return "apac."
	default:
		return "us."
	}
}
