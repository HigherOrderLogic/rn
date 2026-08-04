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

package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateConfig_DefaultAccepted is the pin against which we
// check that rune.star's shipped defaults never make the router or
// the IDE config loader unhappy.
func TestValidateConfig_DefaultAccepted(t *testing.T) {
	require.NoError(t, ValidateConfig(DefaultConfig()))
}

// TestValidateConfig_RejectsInvalid covers the cases that would
// otherwise surface as opaque errors from llmrouter.New or the
// provider clients.
func TestValidateConfig_RejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*Config)
		msg  string
	}{
		{
			name: "reasoning summary",
			mut: func(c *Config) {
				c.ReasoningSummary = "loud"
			},
			msg: "reasoning_summary",
		},
		{
			name: "openai effort",
			mut: func(c *Config) {
				c.OpenAI.ReasoningEffort = "warp"
			},
			msg: "openai.reasoning_effort",
		},
		{
			name: "anthropic effort",
			mut: func(c *Config) {
				c.Anthropic.ReasoningEffort = "turbo"
			},
			msg: "anthropic.reasoning_effort",
		},
		{
			name: "bedrock effort",
			mut: func(c *Config) {
				c.Bedrock.ReasoningEffort = "turbo"
			},
			msg: "bedrock.reasoning_effort",
		},
		{
			name: "relative cache dir",
			mut: func(c *Config) {
				c.Local.ModelsCacheDir = "relative/path"
			},
			msg: "absolute path",
		},
		{
			name: "custom models without url",
			mut: func(c *Config) {
				c.Custom.AvailableModels = map[string]int{"foo": 8192}
			},
			msg: "models.custom.url",
		},
		{
			name: "negative top_k",
			mut: func(c *Config) {
				c.Local.Service.Sampling.TopK = -1
			},
			msg: "top_k",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mut(&cfg)
			err := ValidateConfig(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.msg)
		})
	}
}
