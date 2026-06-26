// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2024-2026 Unstable Build, All Rights Reserved.
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

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"unstable.build/go-tui/cmd/rune-agent/agent/skills"
)

func TestSkillsPromptSection(t *testing.T) {
	t.Run("empty skills returns empty string", func(t *testing.T) {
		assert.Equal(t, "", skillsPromptSection(nil))
	})

	t.Run("prompt skills only", func(t *testing.T) {
		loaded := []skills.Skill{
			{Name: "debug", Description: "Debug issues", Dir: "/skills/debug"},
		}
		result := skillsPromptSection(loaded)
		assert.Contains(t, result, "- debug: Debug issues")
		assert.NotContains(t, result, "Agent skills")
	})

	t.Run("agent skills separated from prompt skills", func(t *testing.T) {
		loaded := []skills.Skill{
			{Name: "debug", Description: "Debug issues", Dir: "/skills/debug"},
			{Name: "explore", Description: "Explore code", Dir: "/skills/explore", Type: "agent"},
		}
		result := skillsPromptSection(loaded)
		assert.Contains(t, result, "- debug: Debug issues")
		assert.Contains(t, result, "Agent skills (spawn a sub-agent")
		assert.Contains(t, result, "- explore (agent): Explore code")
	})

	t.Run("agent skills only", func(t *testing.T) {
		loaded := []skills.Skill{
			{Name: "explore", Description: "Explore code", Dir: "/skills/explore", Type: "agent"},
		}
		result := skillsPromptSection(loaded)
		assert.Contains(t, result, "Agent skills")
		assert.Contains(t, result, "- explore (agent): Explore code")
	})
}

func TestProviderToolAddendum(t *testing.T) {
	t.Run("anthropic addendum", func(t *testing.T) {
		a := ProviderToolAddendum("anthropic")
		assert.Contains(t, a, "CRITICAL: TOOL SELECTION")
		assert.Contains(t, a, "find_definition")
		assert.Contains(t, a, "find_references")
		assert.Contains(t, a, "search_symbols")
		assert.Contains(t, a, "outline_file")
		assert.Contains(t, a, "search_content")
		assert.Contains(t, a, "bash")
		assert.NotContains(t, a, "grep_files")
	})

	t.Run("openai addendum", func(t *testing.T) {
		a := ProviderToolAddendum("openai")
		assert.Contains(t, a, "CRITICAL: TOOL SELECTION")
		assert.Contains(t, a, "find_definition")
		assert.Contains(t, a, "search_symbols")
		assert.Contains(t, a, "outline_file")
		assert.Contains(t, a, "exec_command")
		assert.Contains(t, a, "grep_files")
		assert.NotContains(t, a, "search_content")
	})

	t.Run("llamacpp addendum", func(t *testing.T) {
		a := ProviderToolAddendum("llamacpp")
		assert.Contains(t, a, "CRITICAL: TOOL SELECTION")
		assert.Contains(t, a, "find_definition")
		assert.Contains(t, a, "exec_command")
		assert.Contains(t, a, "grep_files")
	})

	t.Run("gemini addendum", func(t *testing.T) {
		a := ProviderToolAddendum("gemini")
		assert.Contains(t, a, "CRITICAL: TOOL SELECTION")
		assert.Contains(t, a, "find_definition")
		assert.Contains(t, a, "search_symbols")
		assert.Contains(t, a, "outline_file")
		assert.Contains(t, a, "run_command")
		assert.Contains(t, a, "grep_search")
		assert.NotContains(t, a, "search_content")
	})

	t.Run("unknown provider returns empty", func(t *testing.T) {
		assert.Empty(t, ProviderToolAddendum("ollama"))
		assert.Empty(t, ProviderToolAddendum("unknown"))
		assert.Empty(t, ProviderToolAddendum(""))
	})
}
