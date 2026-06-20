// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/workspace"
)

// decodeStarlark is a thin test helper mirroring the legacy positional
// signature so tests read naturally.
func decodeStarlark(src string, params, base map[string]any) (map[string]any, error) {
	return decodeStarlarkConfig(starlarkConfigSource{
		src:    []byte(src),
		params: params,
		base:   base,
	})
}

// readRuneStar reads the shipped rune.star joined with themes.star, mirroring
// how the production config (cmd/rune/config.go) assembles the default
// Starlark source. rune.star references GUI_THEMES, which themes.star binds.
func readRuneStar(t *testing.T) []byte {
	t.Helper()
	themes, err := os.ReadFile("../cmd/rune/themes.star")
	require.NoError(t, err)
	runeStar, err := os.ReadFile("../cmd/rune/rune.star")
	require.NoError(t, err)
	return append(append(themes, '\n'), runeStar...)
}

func TestDecodeStarlarkConfigBasic(t *testing.T) {
	src := `
config = {
    "log_path": "/tmp/debug.log",
    "log_level": "info",
    "clipboard": "system",
    "editor": {
        "mode": "modal",
        "autoindent": True,
        "highlights": {
            "keyword": {"fg": "yellow"},
        },
    },
    "gui": {
        "themes": {
            "romero": {"red": "#990000"},
        },
    },
}
`
	cfg, err := decodeStarlark(src, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/debug.log", cfg["log_path"])
	assert.Equal(t, "info", cfg["log_level"])
	assert.Equal(t, "system", cfg["clipboard"])

	editor := cfg["editor"].(map[string]any)
	assert.Equal(t, "modal", editor["mode"])
	assert.Equal(t, true, editor["autoindent"])

	hl := editor["highlights"].(map[string]any)
	keyword := hl["keyword"].(map[string]any)
	assert.Equal(t, "yellow", keyword["fg"])

	gui := cfg["gui"].(map[string]any)
	themes := gui["themes"].(map[string]any)
	romero := themes["romero"].(map[string]any)
	assert.Equal(t, "#990000", romero["red"])
}

func TestDecodeStarlarkConfigTopLevelControl(t *testing.T) {
	src := `
is_gui = False

theme = "romero" if is_gui else "carmack"

aliases = {}
for kb in [("<m-q>", "quit"), ("<m-t>", "tabnew")]:
    aliases[kb[0]] = kb[1]

config = {
    "gui": {"default_theme": theme},
    "command": {"key_bindings": aliases},
}
`
	cfg, err := decodeStarlark(src, nil, nil)
	require.NoError(t, err)
	gui := cfg["gui"].(map[string]any)
	assert.Equal(t, "carmack", gui["default_theme"])

	cmd := cfg["command"].(map[string]any)
	kb := cmd["key_bindings"].(map[string]any)
	assert.Equal(t, "quit", kb["<m-q>"])
	assert.Equal(t, "tabnew", kb["<m-t>"])
}

func TestDecodeStarlarkConfigLists(t *testing.T) {
	src := `
config = {
    "command": {
        "aliases": {
            "worktreenew": [
                "!! git worktree add",
                "workspaceopen",
                "workspaceready workspacerename",
            ],
        },
    },
    "flags": ("dim", "bold"),
    "ints": [1, 2, 3],
}
`
	cfg, err := decodeStarlark(src, nil, nil)
	require.NoError(t, err)
	cmd := cfg["command"].(map[string]any)
	aliases := cmd["aliases"].(map[string]any)
	assert.Equal(t, []any{
		"!! git worktree add",
		"workspaceopen",
		"workspaceready workspacerename",
	}, aliases["worktreenew"])
	assert.Equal(t, []any{"dim", "bold"}, cfg["flags"])
	assert.Equal(t, []any{1, 2, 3}, cfg["ints"])
}

func TestDecodeStarlarkConfigTypes(t *testing.T) {
	src := `
config = {
    "yes": True,
    "no": False,
    "n": 42,
    "f": 3.5,
    "s": "hello",
    "null": None,
}
`
	cfg, err := decodeStarlark(src, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, true, cfg["yes"])
	assert.Equal(t, false, cfg["no"])
	assert.Equal(t, 42, cfg["n"])
	assert.Equal(t, 3.5, cfg["f"])
	assert.Equal(t, "hello", cfg["s"])
	assert.Nil(t, cfg["null"])
}

func TestDecodeStarlarkConfigMissingGlobal(t *testing.T) {
	_, err := decodeStarlark("x = 1\n", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config")
}

func TestDecodeStarlarkConfigNotADict(t *testing.T) {
	_, err := decodeStarlark(`config = 1`, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a dict")
}

func TestDecodeStarlarkConfigRejectsLoad(t *testing.T) {
	_, err := decodeStarlark(`load("other.star", "x")
config = {"x": x}
`, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load")
}

func TestDecodeStarlarkConfigNonStringKey(t *testing.T) {
	_, err := decodeStarlark(`config = {1: "a"}`, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keys must be strings")
}

func TestDecodeConfigFileUsesFilenameExtension(t *testing.T) {
	cfg, err := decodeConfigFile(
		strings.NewReader(`config = {"log_level": "debug"}`), "config.star")
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg["log_level"])

	cfg, err = decodeConfigFile(strings.NewReader("log_level: info\n"), "config.yaml")
	require.NoError(t, err)
	assert.Equal(t, "info", cfg["log_level"])

	_, err = decodeConfigFile(strings.NewReader(`config = {"log_level": "debug"}`), "config.yaml")
	require.Error(t, err)

	_, err = decodeConfigFile(strings.NewReader("log_level: info\n"), "config.star")
	require.Error(t, err)
}

//go:embed testdata/runerc_sample.star
var starlarkSampleConfig string

// TestStarlarkSampleEndToEnd loads a realistic rune.star-style config through decodeConfigFile
// and asserts the resulting config is shaped as expected.
func TestStarlarkSampleEndToEnd(t *testing.T) {
	cfg, err := decodeConfigFile(strings.NewReader(starlarkSampleConfig), "rune.star")
	require.NoError(t, err)

	assert.Equal(t, "info", cfg["log_level"])
	editor := cfg["editor"].(map[string]any)
	assert.Equal(t, "modal", editor["mode"])
	assert.Equal(t, true, editor["autoindent"])

	cmd := cfg["command"].(map[string]any)
	kb := cmd["key_bindings"].(map[string]any)
	assert.Equal(t, "quit", kb["<m-q>"])
	assert.Equal(t, "tabfocus 3", kb["<a-3>"]) // produced by the for-loop

	aliases := cmd["aliases"].(map[string]any)
	assert.Equal(t, []any{
		`!! ROOT=$(git rev-parse --git-common-dir) && ROOT=$(cd "$ROOT/.." && pwd) || exit 1`,
		`!! ROOT_HASH=$(printf %s "$ROOT" | (sha256sum 2>/dev/null || shasum -a 256) | cut -c1-4)`,
		`!! ROOT_NAME=${ROOT##*/}`,
		`!! WORKTREE=$RUNE_DATADIR/worktrees/$ROOT_NAME-$ROOT_HASH/$1`,
		`!! git worktree add "$WORKTREE" -b $1`,
		"workspaceopen $WORKTREE",
		"workspaceready workspacerename $1",
	}, aliases["worktreenew"])
}

// TestStarlarkConfigLoadsFromFile exercises loadFileConfig end-to-end
// with a .star-suffixed path.
func TestStarlarkConfigLoadsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rune.star")
	require.NoError(t, os.WriteFile(path, []byte(starlarkSampleConfig), 0o644))

	c := &ideConfig{cfg: map[string]any{}, errors: map[string]error{}}
	require.NoError(t, loadFileConfig(c, path))
	assert.Equal(t, "info", c.cfg["log_level"])
}

func TestYAMLConfigLoadsFromFileByExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("log_level: info\n"), 0o644))

	c := &ideConfig{cfg: map[string]any{}, errors: map[string]error{}}
	require.NoError(t, loadFileConfig(c, path))
	assert.Equal(t, "info", c.cfg["log_level"])
}

func TestDecodeStarlarkConfigWithParams(t *testing.T) {
	src := `
config = {
    "mode": mode,
    "tui":  tui,
}
`
	cfg, err := decodeStarlark(src,
		map[string]any{"tui": true, "mode": "modeless"}, nil)
	require.NoError(t, err)
	assert.Equal(t, true, cfg["tui"])
	assert.Equal(t, "modeless", cfg["mode"])

	// Same script with different params produces a different result.
	cfg, err = decodeStarlark(src,
		map[string]any{"tui": false, "mode": "modal"}, nil)
	require.NoError(t, err)
	assert.Equal(t, false, cfg["tui"])
	assert.Equal(t, "modal", cfg["mode"])
}

// TestRuneStarFixture decodes the shipped cmd/rune/rune.star and checks it
// branches correctly on the tui/mode params.
func TestRuneStarFixture(t *testing.T) {
	data := readRuneStar(t)

	cases := []struct {
		name   string
		params map[string]any
		checks func(*testing.T, map[string]any)
	}{
		{
			name:   "gui",
			params: map[string]any{"mode": "modal", "tui": false},
			checks: func(t *testing.T, cfg map[string]any) {
				editor := cfg["editor"].(map[string]any)
				// rune.star no longer sets editor.mode; the bootstrap
				// override layered on top is what picks the mode.
				_, hasMode := editor["mode"]
				assert.False(t, hasMode, "editor.mode should not be set in rune.star")
				assert.Equal(t, false, editor["auto_pair"])
				// GUI-specific window manager frame charset should use the
				// braille-ish corners.
				wm := cfg["browser"].(map[string]any)["window_manager"].(map[string]any)
				cs := wm["frame_charset"].(map[string]any)
				assert.Equal(t, "🭽", cs["topleft"])
				assert.NotContains(t, cfg, "default_attr")
			},
		},
		{
			name:   "tui",
			params: map[string]any{"mode": "modal", "tui": true},
			checks: func(t *testing.T, cfg map[string]any) {
				assert.Contains(t, cfg, "default_attr")
				wm := cfg["browser"].(map[string]any)["window_manager"].(map[string]any)
				cs := wm["frame_charset"].(map[string]any)
				assert.Equal(t, "┌", cs["topleft"])
				editor := cfg["editor"].(map[string]any)
				_, hasMode := editor["mode"]
				assert.False(t, hasMode, "editor.mode should not be set in rune.star")
				assert.Equal(t, false, editor["auto_pair"])
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := decodeStarlarkConfig(starlarkConfigSource{
				src:      data,
				filename: "rune.star",
				params:   c.params,
			})
			require.NoError(t, err)
			c.checks(t, cfg)
		})
	}
}

// TestRuneStarAsDefaultConfig wires the shipped rune.star Starlark config through
// the full loadConfig path to ensure initConfig + validateConfig are happy
// with the decoded tree.
func TestRuneStarAsDefaultConfig(t *testing.T) {
	data := readRuneStar(t)

	var cfg ideConfig
	require.NoError(t, loadConfig(&cfg, "nonExistent", browser.NopWallpaper(),
		defaultConfigSource{
			src:   string(data),
			modal: true,
			tui:   false,
		},
		term.RingBell, term.ScheduleNextTick, ""))
	assert.Equal(t, "modal", cfg.editorMode())
	assert.False(t, cfg.editorAutoPair())
	assert.False(t, cfg.editorAutoSave())
	assert.Equal(t, "info", cfg.cfg["log_level"])
	assert.Equal(t, 2000, cfg.shellMaxHistory())
}

// TestRuneStarModelsConfig verifies the shipped rune.star renders a
// `models` block with the documented sub-keys. This locks the schema so
// downstream loaders can rely on the keys being present.
func TestRuneStarModelsConfig(t *testing.T) {
	data := readRuneStar(t)

	cfg, err := decodeStarlarkConfig(starlarkConfigSource{
		src:      data,
		filename: "rune.star",
		params:   map[string]any{"mode": "modal", "tui": false},
	})
	require.NoError(t, err)

	models, ok := cfg["models"].(map[string]any)
	require.True(t, ok, "models block missing")
	// rune.star no longer ships a `models.default`; the loader keeps the
	// built-in default when the key is absent and the bootstrap override
	// is what sets a concrete model.
	_, hasDefault := models["default"]
	assert.False(t, hasDefault, "models.default should not be set in rune.star")
	assert.Equal(t, "auto", models["reasoning_summary"])
	assert.Equal(t, false, models["debug_http"])

	for _, prov := range []string{"openai", "anthropic", "gemini", "codex", "claude", "custom", "local"} {
		_, ok := models[prov].(map[string]any)
		assert.Truef(t, ok, "models.%s missing", prov)
	}

	openai := models["openai"].(map[string]any)
	assert.Contains(t, openai, "base_url")
	assert.Contains(t, openai, "reasoning_effort")

	local := models["local"].(map[string]any)
	assert.Contains(t, local, "models_cache_dir")
	assert.Contains(t, local, "n_gpu_layers")
	assert.Contains(t, local, "threads")
	assert.Contains(t, local, "flash_attention")
	assert.Contains(t, local, "batch_size")
	assert.Contains(t, local, "max_output_tokens")
	assert.Contains(t, local, "chat_template")
	assert.Contains(t, local, "n_cache_reuse")
	sampling, ok := local["sampling"].(map[string]any)
	require.True(t, ok, "models.local.sampling missing")
	for _, key := range []string{
		"seed", "temperature", "top_k", "top_p", "min_p",
		"repeat_penalty", "repeat_last_n",
		"freq_penalty", "presence_penalty",
		"typical_p", "top_n_sigma",
		"mirostat", "mirostat_tau", "mirostat_eta",
		"dynatemp_range", "dynatemp_exponent",
		"xtc_probability", "xtc_threshold",
		"dry_multiplier", "dry_base",
		"dry_allowed_length", "dry_penalty_last_n",
	} {
		assert.Containsf(t, sampling, key, "models.local.sampling.%s missing", key)
	}
}

func TestDecodeStarlarkOverlayRead(t *testing.T) {
	base := map[string]any{
		"editor":     map[string]any{"mode": "modal"},
		"log_level":  "info",
		"extensions": map[string]any{},
	}
	src := `
if config["editor"]["mode"] == "modal":
    config["log_level"] = "debug"
    config["extensions"]["my_extension"] = {"enabled": True}
else:
    config["log_level"] = "warn"
`
	cfg, err := decodeStarlark(src, nil, base)
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg["log_level"])
	ext := cfg["extensions"].(map[string]any)
	my := ext["my_extension"].(map[string]any)
	assert.Equal(t, true, my["enabled"])
}

func TestDecodeStarlarkOverlayRebind(t *testing.T) {
	base := map[string]any{"log_level": "info"}
	src := `config = {"log_level": "error"}`
	cfg, err := decodeStarlark(src, nil, base)
	require.NoError(t, err)
	assert.Equal(t, "error", cfg["log_level"])
}

func TestDecodeOverlayConfigFileStarlark(t *testing.T) {
	base := map[string]any{
		"editor":    map[string]any{"mode": "modal"},
		"log_level": "info",
	}
	src := `
if config["editor"]["mode"] == "modal":
    config["command"] = {"key": "<c-p>"}
`
	cfg, err := decodeOverlayConfigFile(strings.NewReader(src), "override.star", base)
	require.NoError(t, err)
	cmd := cfg["command"].(map[string]any)
	assert.Equal(t, "<c-p>", cmd["key"])
	// Existing keys from base are preserved.
	assert.Equal(t, "info", cfg["log_level"])
}

func TestDecodeOverlayConfigFileYAML(t *testing.T) {
	base := map[string]any{
		"editor":    map[string]any{"mode": "modal"},
		"log_level": "info",
	}
	cfg, err := decodeOverlayConfigFile(
		strings.NewReader("log_level: debug\neditor:\n  autoindent: true\n"),
		"override.yaml", base)
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg["log_level"])
	editor := cfg["editor"].(map[string]any)
	// deep-merge: existing mode preserved, new autoindent added.
	assert.Equal(t, "modal", editor["mode"])
	assert.Equal(t, true, editor["autoindent"])
}

func TestDecodeOverlayConfigFileStarlarkRebindMergesBase(t *testing.T) {
	base := map[string]any{
		"editor":    map[string]any{"mode": "vi"},
		"log_level": "info",
	}
	src := `config = {"command": {"key": "<c-p>"}}`
	cfg, err := decodeOverlayConfigFile(strings.NewReader(src),
		"override.star", base)
	require.NoError(t, err)
	cmd := cfg["command"].(map[string]any)
	assert.Equal(t, "<c-p>", cmd["key"])
	// Base keys preserved across a top-level rebind.
	assert.Equal(t, "info", cfg["log_level"])
	editor := cfg["editor"].(map[string]any)
	assert.Equal(t, "vi", editor["mode"])
}

func TestDecodeOverlayConfigFileStarlarkEmptyPreservesBase(t *testing.T) {
	base := map[string]any{"log_level": "info"}
	cfg, err := decodeOverlayConfigFile(
		strings.NewReader("# just a comment\n"),
		"override.star", base)
	require.NoError(t, err)
	assert.Equal(t, "info", cfg["log_level"])
}

func TestLoadConfigStarUserOverlayPreservesEmbeddedRuneStar(t *testing.T) {
	data := readRuneStar(t)

	dir := t.TempDir()
	userPath := filepath.Join(dir, "config.star")
	require.NoError(t, os.WriteFile(userPath,
		[]byte(`config = {"log_level": "debug"}`), 0o644))

	var cfg ideConfig
	require.NoError(t, loadConfig(&cfg, userPath, browser.NopWallpaper(),
		defaultConfigSource{
			src:   string(data),
			modal: true,
			tui:   false,
		},
		term.RingBell, term.ScheduleNextTick, ""))

	assert.Equal(t, "debug", cfg.cfg["log_level"])
	// Defaults from cmd/rune/rune.star survive the top-level rebind.
	assert.Equal(t, "modal", cfg.editorMode())
	assert.False(t, cfg.editorAutoPair())
	assert.Equal(t, 2000, cfg.shellMaxHistory())
}

func TestLoadWorkspaceConfigStar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.star")
	require.NoError(t, os.WriteFile(path, []byte(`config["log_level"] = "debug"`), 0o644))

	c := &ideConfig{cfg: map[string]any{"log_level": "info"}, errors: map[string]error{}}
	uri, err := workspaceapi.CurrentUserHostURI(dir)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	ws := workspace.NewSchemeWorkspace(uri, scheme, inlineSchedule)
	defer ws.Close()

	isConfigErr, err := loadWorkspaceConfig("config.star", ws, uri, c)
	require.NoError(t, err)
	assert.False(t, isConfigErr)
	assert.Equal(t, "debug", c.cfg["log_level"])
}

func TestLoadWorkspaceConfigStarOverridesExtensionConfigFromBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.star")
	require.NoError(t, os.WriteFile(path, []byte(`
config["extensions"]["git"]["config"]["from_star"] = "star"
config["extensions"]["git"]["config"]["nested"]["override"] = "star"
`), 0o644))

	c := &ideConfig{cfg: map[string]any{
		"extensions": map[string]any{
			"git": map[string]any{
				"path": "myPath",
				"config": map[string]any{
					"from_base": "base",
					"nested": map[string]any{
						"keep":     "yes",
						"override": "base",
					},
				},
			},
		},
	}, errors: map[string]error{}}
	uri, err := workspaceapi.CurrentUserHostURI(dir)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	ws := workspace.NewSchemeWorkspace(uri, scheme, inlineSchedule)
	defer ws.Close()

	isConfigErr, err := loadWorkspaceConfig("config.star", ws, uri, c)
	require.NoError(t, err)
	assert.False(t, isConfigErr)

	exts := c.extensions()
	gitExt, ok := exts["git"]
	require.True(t, ok)

	extCfg, ok := gitExt.config()
	require.True(t, ok)

	fromBase, err := extCfg.GetString("from_base")
	require.NoError(t, err)
	assert.Equal(t, "base", fromBase)

	fromStar, err := extCfg.GetString("from_star")
	require.NoError(t, err)
	assert.Equal(t, "star", fromStar)

	nested, err := extCfg.GetConfig("nested")
	require.NoError(t, err)

	keep, err := nested.GetString("keep")
	require.NoError(t, err)
	assert.Equal(t, "yes", keep)

	override, err := nested.GetString("override")
	require.NoError(t, err)
	assert.Equal(t, "star", override)
}

func TestLoadWorkspaceConfigYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("log_level: warn\n"), 0o644))

	c := &ideConfig{cfg: map[string]any{"log_level": "info"}, errors: map[string]error{}}
	uri, err := workspaceapi.CurrentUserHostURI(dir)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	ws := workspace.NewSchemeWorkspace(uri, scheme, inlineSchedule)
	defer ws.Close()

	isConfigErr, err := loadWorkspaceConfig("config.yaml", ws, uri, c)
	require.NoError(t, err)
	assert.False(t, isConfigErr)
	assert.Equal(t, "warn", c.cfg["log_level"])
}

func TestDecodeOverlayConfigFileUsesFilenameExtension(t *testing.T) {
	base := map[string]any{"log_level": "info"}

	cfg, err := decodeOverlayConfigFile(
		strings.NewReader(`config["log_level"] = "debug"`),
		"config.star", cloneTestMap(base))
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg["log_level"])

	cfg, err = decodeOverlayConfigFile(
		strings.NewReader("log_level: warn\n"),
		"config.yaml", cloneTestMap(base))
	require.NoError(t, err)
	assert.Equal(t, "warn", cfg["log_level"])

	_, err = decodeOverlayConfigFile(
		strings.NewReader(`config["log_level"] = "debug"`),
		"config.yaml", cloneTestMap(base))
	require.Error(t, err)

	_, err = decodeOverlayConfigFile(
		strings.NewReader("log_level: warn\n"),
		"config.star", cloneTestMap(base))
	require.Error(t, err)
}

// TestModelessPresetsUseArrowLayoutBindings pins the modeless window/tab
// layout shared by the modeless and exo-modeless presets, once overlaid
// on the embedded rune.star defaults:
//
//   - windowfocus on <meta>+arrows
//   - windowmove on <shift-meta>+arrows (the "shift means move" rule)
//   - windowresize on <ctrl-shift-meta>+arrows (kept off the move chord)
//   - tabmove on <alt-shift>+arrows
func TestModelessPresetsUseArrowLayoutBindings(t *testing.T) {
	runeStar := readRuneStar(t)

	wantBound := map[string]string{
		"<m-left>":  "windowfocus left",
		"<m-right>": "windowfocus right",
		"<m-down>":  "windowfocus down",
		"<m-up>":    "windowfocus up",

		"<s-m-left>":  "windowmove left",
		"<s-m-right>": "windowmove right",
		"<s-m-down>":  "windowmove down",
		"<s-m-up>":    "windowmove up",

		"<c-s-m-left>":  "windowresize decrease width",
		"<c-s-m-right>": "windowresize increase width",
		"<c-s-m-down>":  "windowresize decrease height",
		"<c-s-m-up>":    "windowresize increase height",

		"<a-s-left>":  "tabmove left",
		"<a-s-right>": "tabmove right",

		"<f12>":   "lsp definition",
		"<s-f12>": "lsp references",

		"<m-\\\\>": "searchtext",
	}

	for _, file := range []string{
		"override_modeless.star",
		"override_modeless.yaml",
		"override_exo_modeless.star",
		"override_exo_modeless.yaml",
	} {
		t.Run(file, func(t *testing.T) {
			base, err := decodeDefaultConfig(defaultConfigSource{
				src: string(runeStar), modal: true, tui: false,
			})
			require.NoError(t, err)

			overlay, err := os.ReadFile(filepath.Join("../cmd/rune", file))
			require.NoError(t, err)
			cfg, err := decodeOverlayConfigFile(
				bytes.NewReader(overlay), file, base)
			require.NoError(t, err)

			c := &ideConfig{cfg: cfg, errors: map[string]error{}}
			mappings := c.commandKeyMappings()

			for key, wantCmd := range wantBound {
				seq := mustParseBindingKey(t, key)
				got, ok := mappings[seq]
				require.Truef(t, ok, "%s must be bound", key)
				require.Equalf(t, [][]string{strings.Split(wantCmd, " ")},
					got, "%s must run %q", key, wantCmd)
			}
		})
	}
}

// mustParseBindingKey mirrors commandKeyMappings' own key parsing: it
// first tries a two-key handler.Sequence, then falls back to a single
// term key stored in Sequence.First. This keeps the test lookup keyed
// the same way the resolved binding map is.
func mustParseBindingKey(t *testing.T, key string) handler.Sequence {
	t.Helper()
	seq, err := handler.ParseSequence(key)
	if err != nil {
		seq.First, err = term.ParseKey(key)
		require.NoErrorf(t, err, "parse binding key %q", key)
	}
	return seq
}

func mustLegacyModelessConfigFromGit(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"editor": map[string]any{
			"mode": "modeless",
			"modeless": map[string]any{
				"attr":        map[string]any{"fg": "default", "bg": "default"},
				"bar_attr":    map[string]any{"fg": "default", "bg": "default"},
				"search_attr": map[string]any{"fg": "grey", "bg": "yellow"},
			},
			"status_bar": map[string]any{
				"layout": `██▓▒░  {{ .Filepath }}  {{ .GitShortRef }}   {{ .GitDiffAdd | fg "green" }}   {{ .GitDiffDel | fg "red" }} {{ .ShiftRight }} {{ .CursorColumn }}:{{ .CursorLine }}  {{ .TotalLines }} lines  {{ .Language | bold }}  ░▒▓██`,
			},
		},
		"command": map[string]any{
			"key": "<s-m-p>",
			"key_bindings": map[string]any{
				"<m-n>":                 "windownew",
				"<m-o>":                 "lsphover",
				"<m-s>":                 "write",
				"<a-m-s>":               "writeall",
				"<m-w>":                 "tabclose",
				"<m-q>":                 "quit",
				"<m-s-n>":               "windownew",
				"<m-s-w>":               "windowclose",
				"<m-p>":                 "searchfile",
				"<a-g>":                 "searchtext",
				"<m-r>":                 "echo {prompt}jumptoast<space>locals.scm<space>local.definition.type<space>",
				"<s-m-r>":               "searchtype",
				"<m-;>":                 "searchtext",
				"<c-m-p>":               "echo <s-m-p>workspacefocus{wait}<space>",
				"<m-f2>":                "locationtoggle bookmark",
				"<f2>":                  "jumptolocation next bookmark",
				"<s-f2>":                "jumptolocation prev bookmark",
				"<s-m-f2>":              "locationdeleteall bookmark",
				"<a-m-right>":           "tabnext",
				"<a-m-left>":            "tabprevious",
				"<m-f>":                 "searchtext",
				"<m-g>":                 "jumptolocation next search",
				"<s-m-g>":               "jumptolocation prev search",
				"<m-u>":                 "cursorhistory prev",
				"<s-m-u>":               "cursorhistory next",
				"<m-,>":                 "config",
				"<a-m-h>":               "tabmove left",
				"<s-m-]>":               "tabnext",
				"<s-m-[>":               "tabprevious",
				"<c-g>":                 "echo <esc>:",
				"<ctrl-meta-p>":         "echo <esc>:workspacefocus<space>",
				"<alt-meta-down>":       "lspgotodef",
				"<f12>":                 "lspgotodef",
				"<alt-shift-meta-down>": "lspref",
				"<m-j>":                 "",
				"<m-k>":                 "",
				"<m-l>":                 "",
				"<m-h>":                 "",
				"<m-=>":                 "guifontsize increase",
				"<m-->":                 "guifontsize decrease",
				"<m-t>":                 "tabnew",
				"<a-m-l>":               "tabmove right",
				"<m-c>":                 "clipboardcopy",
				"<m-v>":                 "clipboardpaste",
				"<m-y>":                 "echolastcmd",
				"<s-m-h>":               "windowfocus left",
				"<s-m-l>":               "windowfocus right",
				"<s-m-j>":               "windowfocus down",
				"<s-m-k>":               "windowfocus up",
				"<a-s-m-h>":             "windowmove left",
				"<a-s-m-l>":             "windowmove right",
				"<a-s-m-j>":             "windowmove down",
				"<a-s-m-k>":             "windowmove up",
				"<s-m-backspace>":       "windowresize reset",
				"<s-m-+>":               []any{"windowresize max width", "windowresize max height"},
				"<s-m-->":               []any{"windowresize min width", "windowresize min height"},
				"<s-m-up>":              "windowresize increase height",
				"<s-m-down>":            "windowresize decrease height",
				"<s-m-left>":            "windowresize decrease width",
				"<s-m-right>":           "windowresize increase width",
				"<s-m-w>":               "windowclose",
				"<c-m-h>":               "windowdefaultsplit h",
				"<c-m-v>":               "windowdefaultsplit v",
				"<s-m-f>":               "windowtogglemaximize",
				"<m-b>":                 "lspformat",
				"<m-m>":                 "lspformatimports",
				"<a-j>":                 "gitnextchange",
				"<a-k>":                 "gitprevchange",
				"<m-\\\\>":              "searchtext",
				"<m-enter>":             "terminalneworsplit",
				"<s-m-enter>":           "!",
				"gf":                    "editfileoncursor",
				"<m-1>":                 "workspacefocus 1",
				"<m-2>":                 "workspacefocus 2",
				"<m-3>":                 "workspacefocus 3",
				"<m-4>":                 "workspacefocus 4",
				"<m-5>":                 "workspacefocus 5",
				"<m-6>":                 "workspacefocus 6",
				"<m-7>":                 "workspacefocus 7",
				"<m-8>":                 "workspacefocus 8",
				"<m-9>":                 "workspacefocus 9",
			},
		},
		"terminal": map[string]any{"modal": false},
	}
}

func execCommandOutput(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() != 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

func cloneTestMap(m map[string]any) map[string]any {
	return normalizeTestConfig(m).(map[string]any)
}

func normalizeTestConfig(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeTestConfig(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k.(string)] = normalizeTestConfig(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeTestConfig(val)
		}
		return out
	default:
		return t
	}
}

func normalizeLegacyExpected(cfg map[string]any) map[string]any {
	out := cloneTestMap(cfg)
	delete(out, "frame_theme")
	delete(out, "frame_charset_theme")
	delete(out, "frameunion_charset_theme")
	delete(out, "frame_attr_theme")
	delete(out, "focus_frame_charset_theme")
	delete(out, "focus_frame_attr_theme")
	delete(out, "color_theme_1")
	delete(out, "color_theme_2")
	delete(out, "color_theme_3")
	delete(out, "color_theme_4")
	delete(out, "color_theme_5")
	delete(out, "color_theme_6")
	delete(out, "color_theme_7")
	delete(out, "color_theme_8")
	extsIfc, ok := out["extensions"]
	if !ok {
		return out
	}
	exts, ok := extsIfc.(map[string]any)
	if !ok {
		return out
	}
	legacyFile, hasFile := exts["fuzzy_file"].(map[string]any)
	legacyLine, hasLine := exts["fuzzy_line"].(map[string]any)
	_, hasSearch := exts["fuzzy_search"]
	if !hasSearch && !hasFile && !hasLine {
		return out
	}
	fuzzySearch := map[string]any{
		"path": "extension_fuzzy_search",
		"config": map[string]any{
			"syntax": map[string]any{
				"case_sensitive": true,
				"algo":           "fuzzy",
			},
		},
	}
	if cfgMap, ok := exts["fuzzy_search"].(map[string]any); ok {
		fuzzySearch = cloneTestMap(cfgMap)
		if _, ok := fuzzySearch["path"]; !ok {
			fuzzySearch["path"] = "extension_fuzzy_search"
		}
		configMap, ok := fuzzySearch["config"].(map[string]any)
		if !ok {
			configMap = map[string]any{}
			fuzzySearch["config"] = configMap
		}
		if _, ok := configMap["syntax"]; !ok {
			configMap["syntax"] = map[string]any{"case_sensitive": true, "algo": "fuzzy"}
		}
	}
	configMap := fuzzySearch["config"].(map[string]any)
	if hasFile {
		configMap["file"] = cloneTestMap(legacyFile["config"].(map[string]any))
		delete(exts, "fuzzy_file")
	}
	if hasLine {
		configMap["line"] = cloneTestMap(legacyLine["config"].(map[string]any))
		delete(exts, "fuzzy_line")
	}
	exts["fuzzy_search"] = fuzzySearch
	return out
}

func flattenConfigCSV(cfg map[string]any) []string {
	var rows []string
	var walk func(string, any)
	walk = func(prefix string, v any) {
		switch t := v.(type) {
		case map[string]any:
			if len(t) == 0 {
				rows = append(rows, prefix+",{}")
				return
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				next := k
				if prefix != "" {
					next = prefix + "." + k
				}
				walk(next, t[k])
			}
		case []any:
			if len(t) == 0 {
				rows = append(rows, prefix+",[]")
				return
			}
			for i, val := range t {
				walk(prefix+"["+strconv.Itoa(i)+"]", val)
			}
		default:
			rows = append(rows, prefix+","+stringifyTestScalar(t))
		}
	}
	walk("", cfg)
	sort.Strings(rows)
	return rows
}

func stringifyTestScalar(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(t)
	}
}
