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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
)

// TestE2E_Scenarios_REPL brings up each scenario, then drives the
// `python` REPL handler against the synced environment and asserts the
// exposed commands work consistently regardless of how the environment
// was expressed. The handler is the same one ExtendWorkspace registers.
func TestE2E_Scenarios_REPL(t *testing.T) {
	findUV(t)

	for _, name := range allScenarios {
		t.Run(name, func(t *testing.T) {
			env := runExtensionOnScenario(t, name)
			handler := &pyHandler{
				exec:   newDirExecutor(env.dir),
				notify: newFakeNotifications(),
				cwd:    env.dir,
			}

			t.Run("pip list shows installed dependencies", func(t *testing.T) {
				out := strings.ToLower(runREPL(t, handler, "pip", "list"))
				assert.Contains(t, out, "numpy",
					"the synced env should expose the numpy dependency")
				assert.Contains(t, out, "requests",
					"the synced env should expose the requests dependency")
			})

			t.Run("run executes against synced env", func(t *testing.T) {
				out := runREPL(t, handler, "run", "python", "-c",
					"import numpy; print(int(numpy.arange(5).sum()))")
				assert.Contains(t, out, "10")
			})

			t.Run("pip show resolves a compiled dependency", func(t *testing.T) {
				out := strings.ToLower(runREPL(t, handler, "pip", "show", "numpy"))
				assert.Contains(t, out, "name: numpy")
			})
		})
	}
}

// runREPL dispatches a `python` subcommand through the real handler,
// verifying the full public path produces exactly one responsive
// result, then returns the raw uv output the handler captured for
// content assertions. Driving HandleCommand exercises the routing and
// argument expansion; runUVCapture reads back the same output without
// having to render the markdown component to a terminal.
func runREPL(t *testing.T, h *pyHandler, args ...string) string {
	t.Helper()
	it, err := h.HandleCommand(
		context.Background(),
		repl.Command{Name: pyCommandName, Args: args},
		repl.NopProgressWriter(),
	)
	require.NoError(t, err)
	results, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	require.Len(t, results, 1)

	prefix := uvRoutes[args[0]]
	expanded := append(append([]string{}, prefix...), args[1:]...)
	out, err := h.runUVCapture(context.Background(), expanded...)
	require.NoError(t, err)
	return out
}
