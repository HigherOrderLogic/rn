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

package idelsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/iterator"
)

// TestParseAlternateCommands asserts the parsing contract for the
// optional alternate_commands initialize option: absent yields nil,
// a well-formed map yields the method->command mapping, and a
// malformed value (wrong type) is an error.
func TestParseAlternateCommands(t *testing.T) {
	t.Parallel()

	t.Run("absent yields nil", func(t *testing.T) {
		t.Parallel()
		got, err := parseAlternateCommands(map[string]any{
			"langID":  "python",
			"command": "ty server",
		})
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("well-formed map parses", func(t *testing.T) {
		t.Parallel()
		got, err := parseAlternateCommands(map[string]any{
			"alternate_commands": map[string]any{
				"textDocument/formatting":      "ruff server",
				"textDocument/rangeFormatting": "ruff server",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"textDocument/formatting":      "ruff server",
			"textDocument/rangeFormatting": "ruff server",
		}, got)
	})

	t.Run("wrong container type errors", func(t *testing.T) {
		t.Parallel()
		_, err := parseAlternateCommands(map[string]any{
			"alternate_commands": "ruff server",
		})
		require.EqualError(t, err,
			"'alternate_commands' should be a map of LSP method to command")
	})

	t.Run("non-string value errors", func(t *testing.T) {
		t.Parallel()
		_, err := parseAlternateCommands(map[string]any{
			"alternate_commands": map[string]any{
				"textDocument/formatting": 42,
			},
		})
		require.EqualError(t, err,
			"'alternate_commands.textDocument/formatting' should be a command string")
	})
}

// TestChildName asserts that childName reduces a command string to
// the base name of its executable, which is used as the diagnostics
// source identity for a multi-server child.
func TestChildName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		command string
		want    string
	}{
		{"ty server", "ty"},
		{"ruff server", "ruff"},
		{"/usr/local/bin/ruff", "ruff"},
		{"pyright-langserver --stdio", "pyright-langserver"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, childName(tt.command))
		})
	}
}

// multiBinPkgManager returns a fixed set of binary paths regardless
// of the requested package id, mirroring how a single Python package
// ships several executables (ty, ruff) in one lib dir.
type multiBinPkgManager struct {
	paths []string
}

func (p *multiBinPkgManager) LibDir(
	_ context.Context, _ string,
) (iterator.Iterator[string], error) {
	return iterator.FromSlice(p.paths), nil
}

// TestFindBinaryDisambiguatesByCommand asserts that findBinary picks
// the binary whose base name matches lang.command even when several
// executables for the same language id share one lib dir. This is the
// invariant multi-server children rely on to resolve ty vs ruff from
// the same Python package directory.
func TestFindBinaryDisambiguatesByCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tyPath := filepath.Join(dir, "ty")
	ruffPath := filepath.Join(dir, "ruff")
	require.NoError(t, os.WriteFile(tyPath, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(ruffPath, []byte("#!/bin/sh\n"), 0o755))

	uri := makeURI(t, "file:///workspace")
	m := New(uri, nil, nil,
		&multiBinPkgManager{paths: []string{tyPath, ruffPath}},
		nil, nil, Config{NoInitializeServer: true})
	t.Cleanup(func() { _ = m.Close() })

	tyBin, err := m.findBinary(t.Context(), &langConfig{id: "python", command: "ty"})
	require.NoError(t, err)
	assert.Equal(t, tyPath, tyBin)

	ruffBin, err := m.findBinary(t.Context(), &langConfig{id: "python", command: "ruff"})
	require.NoError(t, err)
	assert.Equal(t, ruffPath, ruffBin)
}
