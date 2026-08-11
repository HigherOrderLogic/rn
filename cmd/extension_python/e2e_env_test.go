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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/cmd/extension_python/pyshim"
)

// TestE2E_Scenarios_EnvAndLSP runs the extension against each testdata
// scenario, then runs the scenario's main.py through the synced
// environment. Every scenario declares the same dependency expressed
// differently, so all must produce identical stdout — proving the
// extension works around each environment shape. It also asserts the
// fake LSP received exactly one Initialize with langID=python.
func TestE2E_Scenarios_EnvAndLSP(t *testing.T) {
	findUV(t)

	for _, name := range allScenarios {
		t.Run(name, func(t *testing.T) {
			env := runExtensionOnScenario(t, name)

			expected, err := os.ReadFile(filepath.Join("testdata", name, "expected.txt"))
			require.NoError(t, err)

			got := runPython(t, env.dir)
			assert.Equal(t, normalize(string(expected)), normalize(got),
				"scenario %s produced unexpected program output", name)

			assertShimResolvesVenv(t, env)

			params, count := env.lsp.captured()
			assert.Equal(t, 1, count, "LSP must be initialized exactly once")

			var initOpts map[string]any
			require.NoError(t, json.Unmarshal(params.InitializeOptions, &initOpts))
			assert.Equal(t, "python", initOpts["langID"])
			assert.Equal(t, "file://"+env.dir, params.RootURI)

			require.Len(t, env.manuals, 1, "the python REPL command must be registered")
			assert.Equal(t, pyCommandName, env.manuals[0].Name)
		})
	}
}

// normalize trims trailing whitespace so committed expected.txt files do
// not need an exact trailing-newline match.
func normalize(s string) string {
	return trimTrailing(s)
}

func trimTrailing(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// assertShimResolvesVenv runs the python3 shim the bring-up wrote to the
// data dir from inside the scenario workspace and asserts it executes
// the project's .venv interpreter — the terminal contract: a bare
// `python3` in a project dir resolves that project's environment.
func assertShimResolvesVenv(t *testing.T, env scenarioEnv) {
	t.Helper()
	shim := filepath.Join(pyshim.Dir(env.dataDir), "python3")
	cmd := exec.Command(shim, "-c", "import sys; print(sys.prefix)")
	cmd.Dir = env.dir
	// A controlled environment so an activated venv or dev PATH cannot
	// leak into the resolution.
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "shim must execute the scenario venv python: %s", out)

	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(filepath.Join(env.dir, ".venv"))
	require.NoError(t, err)
	assert.Equal(t, want, got,
		"shim must resolve the project venv, not another interpreter")
}

// TestE2E_NestedProjectDiscovery covers a workspace with no root Python
// project but a nested one at deploy/cloudflare/oxprobe-worker. The
// extension must not initialize a language server on startup; only after
// a nested .py is opened should it initialize exactly once, rooted at the
// nested project. Opening a .py with no enclosing marker must not
// initialize.
func TestE2E_NestedProjectDiscovery(t *testing.T) {
	findUV(t)

	env := runExtensionOnScenario(t, "nested")

	_, count := env.lsp.captured()
	assert.Equal(t, 0, count, "no project at the workspace root must not initialize on startup")
	require.Len(t, env.manuals, 1, "the python REPL command must be registered once")
	assert.Equal(t, pyCommandName, env.manuals[0].Name)

	workerRoot := filepath.Join(env.dir, "deploy", "cloudflare", "oxprobe-worker")
	env.editor.open(t, filepath.Join(workerRoot, "worker.py"))
	env.lsp.waitForInit(t, 90*time.Second)

	params, count := env.lsp.captured()
	assert.Equal(t, 1, count, "opening the nested .py must initialize exactly once")
	assert.Equal(t, "file://"+workerRoot, params.RootURI)

	var initOpts map[string]any
	require.NoError(t, json.Unmarshal(params.InitializeOptions, &initOpts))
	assert.Equal(t, "python", initOpts["langID"])

	// A second open inside the same root must not re-initialize, and a
	// .py with no enclosing marker must not initialize a server.
	env.editor.open(t, filepath.Join(workerRoot, "worker.py"))
	env.editor.open(t, filepath.Join(env.dir, "scripts", "stray.py"))

	_, count = env.lsp.captured()
	assert.Equal(t, 1, count, "dedupe and stray opens must not add initializations")
}
