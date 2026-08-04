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
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLLMConfig_BedrockBlock pins the `models.bedrock.*` keys the loader
// understands, so a rename in the config schema is caught here rather than
// silently dropping the provider's settings.
func TestLLMConfig_BedrockBlock(t *testing.T) {
	c := ideConfig{cfg: map[string]any{
		"models": map[string]any{
			"bedrock": map[string]any{
				"profile":          "work",
				"base_url":         "https://bedrock-gw.internal",
				"reasoning_effort": "medium",
				"cache_control":    "default",
			},
		},
	}, errors: map[string]error{}}

	got := c.llmConfig()
	assert.Equal(t, "work", got.Bedrock.Profile)
	assert.Equal(t, "https://bedrock-gw.internal", got.Bedrock.BaseURL)
	assert.Equal(t, "medium", got.Bedrock.ReasoningEffort)
	assert.Equal(t, "default", got.Bedrock.CacheControl)

	client := got.BedrockClientConfig()
	assert.Equal(t, "work", client.Profile)
	assert.Equal(t, "https://bedrock-gw.internal", client.BaseURL)
	assert.Equal(t, "medium", client.ReasoningEffort)
	assert.Equal(t, "default", client.CacheControl)
}
