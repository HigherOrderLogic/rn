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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelEntries_CoverCatalog(t *testing.T) {
	entries := ModelEntries()
	require.Len(t, entries, len(AvailableModels()))
	for _, e := range entries {
		assert.Equal(t, LLMProvider, e.Provider)
		assert.Positive(t, e.ContextWindow, "model %s", e.Name)
	}
}

func TestFlagshipModel_IsInCatalog(t *testing.T) {
	_, ok := AvailableModels()[FlagshipModel()]
	assert.True(t, ok)
}

func TestNormalizeEffort(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		effort   string
		want     string
		wantWarn bool
	}{
		{"unset", ClaudeSonnet45, "", "", false},
		{"claude high", ClaudeSonnet45, "high", "high", false},
		{"claude unsupported level", ClaudeSonnet45, "ultra", "", true},
		{"nova rejects effort", NovaPro, "high", "", true},
		{"claude 3 predates thinking", "us.anthropic.claude-3-haiku-20240307-v1:0", "low", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warn := NormalizeEffort(tt.model, tt.effort)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantWarn, warn != "", "warning: %q", warn)
		})
	}
}

func TestIsClaudeModel(t *testing.T) {
	assert.True(t, IsClaudeModel(ClaudeOpus45))
	assert.False(t, IsClaudeModel(NovaPro))
	assert.False(t, IsClaudeModel(DeepSeekR1))
}

func TestVerificationModelForRegion(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"", FlagshipModel()},
		{"us-east-1", FlagshipModel()},
		{"eu-west-1", "eu.anthropic.claude-opus-4-5-20251101-v1:0"},
		{"ap-southeast-2", "apac.anthropic.claude-opus-4-5-20251101-v1:0"},
		{"us-gov-west-1", "us-gov.anthropic.claude-opus-4-5-20251101-v1:0"},
	}
	for _, tt := range tests {
		t.Run("region "+tt.region, func(t *testing.T) {
			assert.Equal(t, tt.want, VerificationModelForRegion(tt.region))
		})
	}
}
