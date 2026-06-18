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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
)

func TestPyHandlerRouting(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCall string
	}{
		{"python install", []string{"install", "3.12"}, "uv python install 3.12"},
		{"python list", []string{"list"}, "uv python list"},
		{"python find", []string{"find"}, "uv python find"},
		{"python pin", []string{"pin", "3.12"}, "uv python pin 3.12"},
		{"python uninstall", []string{"uninstall", "3.12"}, "uv python uninstall 3.12"},
		{"init", []string{"init"}, "uv init"},
		{"add", []string{"add", "requests"}, "uv add requests"},
		{"remove", []string{"remove", "requests"}, "uv remove requests"},
		{"sync", []string{"sync"}, "uv sync"},
		{"lock", []string{"lock"}, "uv lock"},
		{"tree", []string{"tree"}, "uv tree"},
		{"build", []string{"build"}, "uv build"},
		{"run", []string{"run", "pytest"}, "uv run pytest"},
		{"tool", []string{"tool", "run", "black"}, "uv tool run black"},
		{"pip", []string{"pip", "install", "flask"}, "uv pip install flask"},
		{"venv", []string{"venv"}, "uv venv"},
		{"cache", []string{"cache", "clean"}, "uv cache clean"},
		{"self", []string{"self", "version"}, "uv self version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex := newFakeExecutor()
			ex.respond(tc.wantCall, scriptedCmd{stdout: "ok\n"})
			_, handler := newPyHandler(ex, newFakeNotifications(), "/repo")
			it, err := handler.HandleCommand(
				context.Background(),
				repl.Command{Name: "python", Args: tc.args},
				repl.NopProgressWriter(),
			)
			require.NoError(t, err)
			out, err := iterator.ToSlice(context.Background(), it)
			require.NoError(t, err)
			assert.Len(t, out, 1)
			assert.Equal(t, []string{tc.wantCall}, ex.callsSnapshot())
		})
	}
}

func TestPyHandlerUnknownSubcommand(t *testing.T) {
	ex := newFakeExecutor()
	_, handler := newPyHandler(ex, newFakeNotifications(), "/repo")
	_, err := handler.HandleCommand(
		context.Background(),
		repl.Command{Name: "python", Args: []string{"bogus"}},
		repl.NopProgressWriter(),
	)
	require.Error(t, err)
	assert.Empty(t, ex.callsSnapshot())
}

func TestPyHandlerEmptyShowsUsage(t *testing.T) {
	ex := newFakeExecutor()
	_, handler := newPyHandler(ex, newFakeNotifications(), "/repo")
	it, err := handler.HandleCommand(
		context.Background(),
		repl.Command{Name: "python"},
		repl.NopProgressWriter(),
	)
	require.NoError(t, err)
	out, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Empty(t, ex.callsSnapshot())
}

func TestPyHandlerComplete(t *testing.T) {
	_, handler := newPyHandler(newFakeExecutor(), newFakeNotifications(), "/repo")

	t.Run("depth 0 lists subcommands", func(t *testing.T) {
		it, err := handler.Complete(context.Background(), "", nil)
		require.NoError(t, err)
		got, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Equal(t, pySubcommandNames, got)
	})

	t.Run("depth 0 filters by prefix", func(t *testing.T) {
		it, err := handler.Complete(context.Background(), "", []string{"p"})
		require.NoError(t, err)
		got, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Equal(t, []string{"pin", "pip"}, got)
	})

	t.Run("depth 1 lists nested uv subcommands", func(t *testing.T) {
		it, err := handler.Complete(context.Background(), "", []string{"pip", ""})
		require.NoError(t, err)
		got, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Equal(t,
			[]string{"check", "compile", "freeze", "install", "list", "show", "sync", "tree", "uninstall"},
			got)
	})

	t.Run("depth 1 filters nested by prefix", func(t *testing.T) {
		it, err := handler.Complete(context.Background(), "", []string{"tool", "u"})
		require.NoError(t, err)
		got, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Equal(t, []string{"uninstall", "upgrade", "update-shell"}, got)
	})

	t.Run("depth 1 for non-group subcommand is empty", func(t *testing.T) {
		it, err := handler.Complete(context.Background(), "", []string{"add", "req"})
		require.NoError(t, err)
		got, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("deeper args defer to uv (empty)", func(t *testing.T) {
		it, err := handler.Complete(context.Background(), "", []string{"pip", "install", "fl"})
		require.NoError(t, err)
		got, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPyHandlerHelp(t *testing.T) {
	_, handler := newPyHandler(newFakeExecutor(), newFakeNotifications(), "/repo")

	t.Run("root", func(t *testing.T) {
		it, err := handler.Help(context.Background(), nil)
		require.NoError(t, err)
		out, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Len(t, out, 1)
	})

	t.Run("known subcommand", func(t *testing.T) {
		it, err := handler.Help(context.Background(), []string{"sync"})
		require.NoError(t, err)
		out, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Len(t, out, 1)
	})
}
