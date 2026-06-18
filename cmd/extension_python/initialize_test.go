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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPyCommand(t *testing.T) {
	cases := []struct {
		name     string
		bin      string
		fallback string
		subcmd   string
		want     string
	}{
		{"unresolved ty", "", "ty", "server", "ty server"},
		{"resolved ty", "/opt/ty", "ty", "server", "/opt/ty server"},
		{"unresolved ruff", "", "ruff", "server", "ruff server"},
		{"resolved ruff", "/usr/local/bin/ruff", "ruff", "server", "/usr/local/bin/ruff server"},
		{"no subcmd", "/opt/uv", "uv", "", "/opt/uv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, pyCommand(tc.bin, tc.fallback, tc.subcmd))
		})
	}
}

func TestPyInitializeParams(t *testing.T) {
	t.Run("with alternates", func(t *testing.T) {
		params, err := pyInitializeParams("file:///tmp/repo", "ty server", map[string]string{
			"textDocument/formatting":      "ruff server",
			"textDocument/rangeFormatting": "ruff server",
		})
		require.NoError(t, err)
		assert.Equal(t, "file:///tmp/repo", params.RootURI)

		var initOpts map[string]any
		require.NoError(t, json.Unmarshal(params.InitializeOptions, &initOpts))
		assert.Equal(t, "python", initOpts["langID"])
		assert.Equal(t, "ty server", initOpts["command"])
		assert.Equal(t, map[string]any{
			"textDocument/formatting":      "ruff server",
			"textDocument/rangeFormatting": "ruff server",
		}, initOpts["alternate_commands"])
	})

	t.Run("single server omits alternate_commands", func(t *testing.T) {
		params, err := pyInitializeParams("file:///tmp/repo", "pyright-langserver --stdio", nil)
		require.NoError(t, err)

		var initOpts map[string]any
		require.NoError(t, json.Unmarshal(params.InitializeOptions, &initOpts))
		assert.Equal(t, "pyright-langserver --stdio", initOpts["command"])
		_, ok := initOpts["alternate_commands"]
		assert.False(t, ok, "alternate_commands must be omitted in single-server mode")
	})
}
