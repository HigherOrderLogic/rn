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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/api/config"
)

func TestApplyPyConfig(t *testing.T) {
	defCommand := "ty server"
	defAlternates := map[string]string{
		"textDocument/formatting":      "ruff server",
		"textDocument/rangeFormatting": "ruff server",
	}

	t.Run("nil config keeps defaults", func(t *testing.T) {
		cmd, alt := applyPyConfig(nil, newFakeNotifications(), defCommand, defAlternates)
		assert.Equal(t, defCommand, cmd)
		assert.Equal(t, defAlternates, alt)
	})

	t.Run("empty config keeps defaults", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{})
		cmd, alt := applyPyConfig(cfg, newFakeNotifications(), defCommand, defAlternates)
		assert.Equal(t, defCommand, cmd)
		assert.Equal(t, defAlternates, alt)
	})

	t.Run("command override drops default alternates", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{
			"command": "pyright-langserver --stdio",
		})
		cmd, alt := applyPyConfig(cfg, newFakeNotifications(), defCommand, defAlternates)
		assert.Equal(t, "pyright-langserver --stdio", cmd)
		assert.Nil(t, alt)
	})

	t.Run("command override keeps supplied alternates", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{
			"command": "pyright-langserver --stdio",
			"alternate_commands": map[string]any{
				"textDocument/formatting": "ruff server",
			},
		})
		cmd, alt := applyPyConfig(cfg, newFakeNotifications(), defCommand, defAlternates)
		assert.Equal(t, "pyright-langserver --stdio", cmd)
		assert.Equal(t, map[string]string{"textDocument/formatting": "ruff server"}, alt)
	})

	t.Run("alternates override without command keeps default command", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{
			"alternate_commands": map[string]any{
				"textDocument/formatting": "black server",
			},
		})
		cmd, alt := applyPyConfig(cfg, newFakeNotifications(), defCommand, defAlternates)
		assert.Equal(t, defCommand, cmd)
		assert.Equal(t, map[string]string{"textDocument/formatting": "black server"}, alt)
	})

	t.Run("invalid alternate value warns and keeps defaults", func(t *testing.T) {
		cfg := config.JSONFromMap(map[string]any{
			"alternate_commands": map[string]any{
				"textDocument/formatting": 42,
			},
		})
		notify := newFakeNotifications()
		cmd, alt := applyPyConfig(cfg, notify, defCommand, defAlternates)
		assert.Equal(t, defCommand, cmd)
		assert.Equal(t, defAlternates, alt)
		assert.NotEmpty(t, notify.notifs)
	})
}
