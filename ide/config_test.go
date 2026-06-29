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
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/idelsp"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
)

var sampleConfig = `
extensions:
    fuzzy_search:
        path: "/path/extension_fuzzy_search"
        config:
            file:
                command: ag -g ""

log_path: "/tmp/debug.log"
log_level: "trace"
clipboard: memory
default_attr:
    bg: yellow
    fg: "#f2f2f2"

editor:
    ruler: 72
    auto_pair: true
    auto_save: true
    status_bar:
        enabled: true
        background_attr:
            bg: maroon
        layout: '█{{ .Status | bg "red" | fg "black" | bold }}█▓▒░  {{ .Filepath }}   {{ .GitShortRef }}   {{ .GitDiffAdd | fg "green" }}   {{ .GitDiffDel | fg "red" }} {{ .ShiftRight }}{{ .CursorColumn }}:{{ .CursorLine }}  {{ .TotalLines }} lines  {{ .Language | bold }}  '
    aux_bar:
        enabled: true
        icons: true
        folds: true
        lines: relative
        highlight_cursor: false
        git: all
        git_del_inline_attr:
            fg: maroon
            flags: bold
        git_add_inline_attr:
            fg: green
            flags: bold
        git_del_locations_attr:
            bg: maroon
            flags: dim
        git_add_locations_attr:
            bg: green
            flags: dim
        line_number_attr:
            fg: gray
            bg: default
        highlight_cursor_attr:
            fg: white
            bg: gray
            flags: bold
    mode: modal
    modal:
        attr:
            bg: yellow
            fg: "#f2f2f2"
        search_attr:
            bg: red
            fg: "#f0f0f0"
        debug: true
        wrap: true
    modeless:
        attr:
            bg: green
            fg: "#f9f9f9"
        search_attr:
            bg: red
            fg: "#f1f1f1"
        wrap: false
    virtual:
        shell: bash
        editor: vim
        attr:
            bg: red
            fg: "#f0f0f0"
        selection_attr:
            bg: green
            fg: "#f3f3f3"
    autoindent: false
    comments:
        go:
            line:
                - "//"
            block:
                - "/*"
                - "*/"
    highlights:
        function:
            fg: green
            bg: yellow

input_mode:
  - mouse
  - esc

lsp:
    icons:
        error: E
        warning: W
        information: I
        hint: H
        inline: '>'
        escape: '^'
        bounds: B
        nilcheck: '0'
        compiler: C

command:
  show_manual: false
  aliases:
    cherry: bomb
    todo:
      - e file:///tmp/todo.md
      - jenesaisquoi
    error: 1
    parcels:
      commands:
        - Somethinggreater
        - NowIcaresomemore
        - Comingback
      completer: files
    daynight:
      commands: ram
      completer:
        - a
        - B
    dtmf:
      commands: ram
      completer: '! hello'
    editmix:
      commands: e
      completer:
        - '{history}'
        - '{file}'
    static_with_hist:
      commands: m
      completer:
        - '{history}'
        - a
        - B
    bad:
      commands: x
      completer:
        - '{nope}'
  manual_attr:
    fg: black
    bg: yellow
    flags: bold
  key_bindings:
    f: searchfile
    l: searchtext
    <c-x>: closeDoors
    <c-x><c-p>: openAllDoors
    f<c-p>: openDoors small
    <c-x>9:
      - openDoors 1
      - large 2
    <c-x>p:
      - invalid A
      - smtg:
        - else
    <-x>f: invalidMapping

notifications:
    auto_close: 1s
    padding: 1
    progress_bar: false
    progress_format:
        start: "{"
        current: "-"
        current_tip: ">"
        remain: "_"
        end: "}"
    attr:
        bg: red
        fg: "#f0f0f0"
    background_attr:
        bg: red
        fg: "#f0f0f0"
    frame_charset:
        horizontalbottom: '━'
        horizontaltop: '━'
        verticalleft: '┃'
        verticalright: '┃'
        topleft: '┏'
        topright: '┓'
        bottomleft: '┗'
        bottomright: '┛'
browser:
    workspace_bar: false
    tab_name_separator: 'XX'
    tabspaces: 4
    icons:
        default: x
        terminal: '&'
        console: '8'
        .go: $
        .py: 1 # ignored
    prompt:
        width: 20
        height: 10
        text_attr:
            fg: teal
        highlight_attr:
            bg: red
            fg: "#f0f0f0"
    window_manager:
        frame: true
        dim: false
        frame_attr:
            fg: red
        frame_charset:
            horizontalbottom: '━'
            horizontaltop: '━'
            verticalleft: '┃'
            verticalright: '┃'
            topleft: '┏'
            topright: '┓'
            bottomleft: '┗'
            bottomright: '┛'
        scroll_bar_attr:
            fg: "#f0f0f0"
        scroll_bar_char: '|'
        scroll_bar_hover_char: 'X'
    frameunion_charset:
        left: '┣'
        right: '┫'
        top: '┫'
        bottom: '┫'
    message_bar_attr:
        fg: white
        bg: teal
    focus_tab_attr:
        fg: "#f0f0f0"
    non_focus_tab_attr:
        fg: white

workspace:
    auto_restore: false
    wallpaper: abc
    wallpaper_attr:
        fg: yellow
        bg: white
    wallpaper_background_attr:
        bg: white

terminal:
    plugin:
        bar_align_bottom: true
        bar_layout: ' {{ .StatusIcon | bg "gray" | fg "white" }} █▓▒░{{ .AlignCenter}}{{ .Command | fg "white" | bold }}{{ .AlignRight }}  ░▒▓█ {{ .Elapsed | fg "white" | bg "gray" }} '
        status_error_icon: "X"
        status_error_attr:
            fg: yellow
        status_success_icon: "$"
        status_success_attr:
            fg: blue
        animation: "ABC"
        bar_background_attr:
            bg: gray
    shell: sh
    max_lines: 999
    bell_trigger: "\x07"
    modal: true
    dynamic_tab_name: true
    initial_reservoir: 0
    attr:
        fg: white
        bg: yellow
    selection_attr:
        fg: green
        bg: teal
    needs_attention_attr:
        fg: red
        flags:
          - blink
`

func assertDefaultConfig(t *testing.T, cfg *ideConfig) {
	assert.Len(t, cfg.extensions(), 0)
	assert.Equal(t, 4, cfg.editorTabspaces())
	assert.NotNil(t, cfg.wallpaper())
	defWmConfig := handler.DefaultWindowManagerConfig()
	defWmConfig.FocusFrameAttr = defWmConfig.FrameAttr
	defWmConfig.FocusFrameCharSet = defWmConfig.FrameCharSet
	assert.Equal(t, defWmConfig, cfg.windowManagerConfig())
	assert.Equal(t, "", cfg.logOutputPath())
	level, slogLevel := cfg.logLevel()
	assert.Equal(t, logrus.ErrorLevel, level)
	assert.Equal(t, slog.LevelError, slogLevel)
	assert.Equal(t, term.InputCurrent, cfg.inputMode())
	assert.Equal(t, component.DefaultFrameUnionCharSet(), cfg.frameUnionCharset())
	assert.True(t, cfg.frameUnion())
	assert.Equal(t, text.DefaultConfig().Icons, cfg.icons())
	assert.Equal(t, idelsp.DefaultIconSet(), cfg.lspIcons())

	assert.Equal(t, workspaceBarKindNumbers, cfg.workspaceBarKind())

	auxBar := cfg.auxiliaryBarEnabled()
	assert.False(t, auxBar)

	folds := cfg.auxiliaryBarFolds()
	assert.False(t, folds)

	git := cfg.auxiliaryBarGit()
	assert.False(t, git)

	gitIcons := cfg.gitIconsEnabled()
	assert.False(t, gitIcons)

	iconsBar := cfg.iconsBarEnabled()
	assert.False(t, iconsBar)

	statusBar := cfg.statusBarEnabled()
	assert.True(t, statusBar)

	enabled, absolute := cfg.auxiliaryBarLines()
	assert.True(t, enabled)
	assert.True(t, absolute)

	cursor := cfg.auxiliaryBarHighlightCursor()
	assert.True(t, cursor)

	actualNotifications := cfg.notificationsConfig()
	expectedNotifications := defaultNotificationsConfig()
	setNotificationsColor(&expectedNotifications)
	assert.NotNil(t, actualNotifications.Interrupter)
	actualNotifications.Interrupter = nil
	expectedNotifications.Interrupter = nil
	assert.Equal(t, expectedNotifications, actualNotifications)

	assert.Equal(t, browser.DefaultConfig().FocusTabAttr, cfg.focusTabAttr())
	assert.Equal(t, browser.DefaultConfig().NonFocusTabAttr, cfg.nonFocusTabAttr())
	assert.Equal(t, term.Attributes{}, cfg.workspaceWallpaperAttr())
	assert.Equal(t, term.Attributes{}, cfg.workspaceWallpaperBackgroundAttr())
	selectAttr := term.Attributes{Attrs: term.AttrReverse}

	reservoir := cfg.initialTerminalCapacity()
	assert.Equal(t, 1, reservoir)

	barConfig := cfg.pluginBarConfig()
	expectedBarConfig := plugin.DefaultBarConfig()
	assert.Equal(t, expectedBarConfig, barConfig)

	vteConfig := cfg.terminalConfig()
	assert.NotNil(t, vteConfig.RingBell)
	assert.NotNil(t, vteConfig.ScheduleNextTick)
	vteConfig.ScheduleNextTick = nil
	vteConfig.RingBell = nil
	assert.Equal(t, vte.Config{
		Clipboard:                cfg.clipboard(),
		SelectionAttributes:      selectAttr,
		NeedsAttentionAttributes: term.Attributes{Attrs: term.AttrBlink},
		Modal:                    true,
		ClipboardRegister:        clipboard.DefaultRegisterID,
		MaxLines:                 10_000,
		MinWidth:                 defaultMinWidth,
	}, vteConfig)
	assert.Equal(t, command.DefaultConfig().ShowManual, cfg.commandOverlayShowManual())
	assert.Equal(t, command.DefaultConfig().ManualAttr, cfg.commandOverlayManualAttr())
	assert.Zero(t, cfg.defaultAttr())

	assert.Equal(t, term.Attributes{Fg: term.ColorBlack, Bg: term.ColorYellow},
		cfg.modelessResultAttr())
	assert.Equal(t, term.Attributes{}, cfg.modelessAttr())
	assert.Equal(t, term.Attributes{}, cfg.modalAttr())
	assert.True(t, cfg.autoRestore())
	assert.Equal(t, "  ", cfg.tabNameSeparator())
	assert.Equal(t, 90, cfg.editorRuler())
	assert.False(t, cfg.editorAutoPair())
	assert.False(t, cfg.editorAutoSave())

	expectedSyntaxConfig := syntax.DefaultConfig()
	syntaxConfig := cfg.syntaxConfig()
	assert.NotNil(t, syntaxConfig.ScheduleNextTick)
	syntaxConfig.ScheduleNextTick = nil
	expectedSyntaxConfig.ScheduleNextTick = nil
	assert.Equal(t, expectedSyntaxConfig, syntaxConfig)
	assert.Empty(t, cfg.editorComments())

	assert.Equal(t, "modal", cfg.editorMode())
	os.Setenv("SHELL", "fish")
}

func TestConfigDefault(t *testing.T) {
	ret := new(ideConfig)
	initDefaultConfig(ret, browser.NopWallpaper(),
		term.RingBell, term.ScheduleNextTick, "", "")
	assertDefaultConfig(t, ret)
}

func TestUpdatesAutoInstall(t *testing.T) {
	for _, tc := range []struct {
		name    string
		updates any
		want    bool
		wantErr bool
	}{
		{"absent updates defaults true", nil, true, false},
		{"absent key defaults true", map[string]any{}, true, false},
		{"explicit true", map[string]any{"auto_install": true}, true, false},
		{"explicit false", map[string]any{"auto_install": false}, false, false},
		{"invalid type defaults true", map[string]any{"auto_install": "yes"},
			true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]any{}
			if tc.updates != nil {
				m["updates"] = tc.updates
			}
			cfg := &ideConfig{cfg: m, errors: map[string]error{}}
			assert.Equal(t, tc.want, cfg.updatesAutoInstall())
			if tc.wantErr {
				assert.Error(t, cfg.errors["updates.auto_install"])
			} else {
				assert.Empty(t, cfg.errors)
			}
		})
	}
}

func TestAuthorizerAutoAuthorize(t *testing.T) {
	for _, tc := range []struct {
		name       string
		authorizer any
		want       bool
		wantErr    bool
	}{
		{"absent authorizer defaults true", nil, true, false},
		{"absent key defaults true", map[string]any{}, true, false},
		{"explicit true", map[string]any{"auto_authorize": true}, true, false},
		{"explicit false", map[string]any{"auto_authorize": false}, false, false},
		{"invalid type defaults true", map[string]any{"auto_authorize": "yes"},
			true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]any{}
			if tc.authorizer != nil {
				m["authorizer"] = tc.authorizer
			}
			cfg := &ideConfig{cfg: m, errors: map[string]error{}}
			assert.Equal(t, tc.want, cfg.authorizerAutoAuthorize())
			if tc.wantErr {
				assert.Error(t, cfg.errors["authorizer.auto_authorize"])
			} else {
				assert.Empty(t, cfg.errors)
			}
		})
	}
}

// TestTerminalModalDefaultFromEditorMode asserts that when terminal.modal
// is not set its default follows editor.mode: modal editors default to
// modal terminals, modeless to modeless, and exo follows its fallback.
func TestTerminalModalDefaultFromEditorMode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		editor map[string]any
		want   bool
	}{
		{"unset editor defaults modal", nil, true},
		{"modal", map[string]any{"mode": "modal"}, true},
		{"modeless", map[string]any{"mode": "modeless"}, false},
		{"exo fallback modal", map[string]any{
			"mode": "exo",
			"exo":  map[string]any{"command": "vim {file}", "fallback": "modal"},
		}, true},
		{"exo fallback modeless", map[string]any{
			"mode": "exo",
			"exo":  map[string]any{"command": "vim {file}", "fallback": "modeless"},
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]any{}
			if tc.editor != nil {
				m["editor"] = tc.editor
			}
			cfg := &ideConfig{cfg: m, errors: map[string]error{}}
			assert.Equal(t, tc.want, cfg.terminalModal())
		})
	}

	// An explicit terminal.modal always wins over the editor-mode default.
	cfg := &ideConfig{cfg: map[string]any{
		"editor":   map[string]any{"mode": "modeless"},
		"terminal": map[string]any{"modal": true},
	}, errors: map[string]error{}}
	assert.True(t, cfg.terminalModal())

	cfg = &ideConfig{cfg: map[string]any{
		"editor":   map[string]any{"mode": "modal"},
		"terminal": map[string]any{"modal": false},
	}, errors: map[string]error{}}
	assert.False(t, cfg.terminalModal())
}

// TestHighlightTabCharEmptyDisables asserts that an explicitly empty
// focus_tab_highlight_char value disables the highlight (returns 0)
// while an absent key falls back to the browser default.
func TestHighlightTabCharEmptyDisables(t *testing.T) {
	def := browser.DefaultConfig().FocusTabHighlightChar

	// key absent → default
	cfg := &ideConfig{cfg: map[string]any{
		"browser":   map[string]any{},
		"workspace": map[string]any{},
	}, errors: map[string]error{}}
	assert.Equal(t, def, cfg.highlightTabChar(),
		"absent browser.focus_tab_highlight_char must fall back to default")
	assert.Equal(t, def, cfg.workspaceHighlightTabChar(),
		"absent workspace.focus_tab_highlight_char must fall back to default")

	// key present and empty → disabled (rune 0)
	cfg = &ideConfig{cfg: map[string]any{
		"browser":   map[string]any{"focus_tab_highlight_char": ""},
		"workspace": map[string]any{"focus_tab_highlight_char": ""},
	}, errors: map[string]error{}}
	assert.Equal(t, rune(0), cfg.highlightTabChar(),
		"empty browser.focus_tab_highlight_char must disable the highlight")
	assert.Equal(t, rune(0), cfg.workspaceHighlightTabChar(),
		"empty workspace.focus_tab_highlight_char must disable the highlight")

	// key present and non-empty → first rune
	cfg = &ideConfig{cfg: map[string]any{
		"browser":   map[string]any{"focus_tab_highlight_char": "▔"},
		"workspace": map[string]any{"focus_tab_highlight_char": "▁"},
	}, errors: map[string]error{}}
	assert.Equal(t, '▔', cfg.highlightTabChar())
	assert.Equal(t, '▁', cfg.workspaceHighlightTabChar())
}

// TestTabOverrideIcon asserts browser.tab_override_icon parses
// correctly: absent → 0 (no override), empty → 0, non-empty → first
// rune.
func TestTabOverrideIcon(t *testing.T) {
	// key absent → 0
	cfg := &ideConfig{cfg: map[string]any{
		"browser": map[string]any{},
	}, errors: map[string]error{}}
	assert.Equal(t, rune(0), cfg.tabOverrideIcon(),
		"absent browser.tab_override_icon must return rune 0")

	// key empty → 0
	cfg = &ideConfig{cfg: map[string]any{
		"browser": map[string]any{"tab_override_icon": ""},
	}, errors: map[string]error{}}
	assert.Equal(t, rune(0), cfg.tabOverrideIcon(),
		"empty browser.tab_override_icon must return rune 0")

	// key present and non-empty → first rune
	cfg = &ideConfig{cfg: map[string]any{
		"browser": map[string]any{"tab_override_icon": "●"},
	}, errors: map[string]error{}}
	assert.Equal(t, '●', cfg.tabOverrideIcon())
}

// TestWorkspaceHome asserts workspace.home parses correctly: absent →
// "~", empty → "~", non-empty → the configured path.
func TestWorkspaceHome(t *testing.T) {
	// key absent → default "~"
	cfg := &ideConfig{cfg: map[string]any{
		"workspace": map[string]any{},
	}, errors: map[string]error{}}
	assert.Equal(t, "~", cfg.workspaceHome(),
		"absent workspace.home must default to ~")
	assert.Empty(t, cfg.errors)

	// key empty → default "~"
	cfg = &ideConfig{cfg: map[string]any{
		"workspace": map[string]any{"home": ""},
	}, errors: map[string]error{}}
	assert.Equal(t, "~", cfg.workspaceHome(),
		"empty workspace.home must default to ~")
	assert.Empty(t, cfg.errors)

	// key present and non-empty → configured path
	cfg = &ideConfig{cfg: map[string]any{
		"workspace": map[string]any{"home": "~/work"},
	}, errors: map[string]error{}}
	assert.Equal(t, "~/work", cfg.workspaceHome())
}

func TestConfigDecodeError(t *testing.T) {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = f.WriteString("||\\n\x00{'BABY':'$$'}")
	require.NoError(t, err)

	var ret ideConfig
	// for assertDefaultConfig
	ret.cfg = map[string]any{
		"workspace": map[string]any{
			"wallpaper": "notEmpty",
		},
	}
	ret.ringBell = term.RingBell
	ret.scheduleNextTick = term.ScheduleNextTick
	err = loadFileConfig(&ret, f.Name())
	assert.Error(t, err)
	assertDefaultConfig(t, &ret)
}

func TestConfigSetting(t *testing.T) {
	m, err := decodeConfig(strings.NewReader(sampleConfig))
	require.NoError(t, err)

	var cfg ideConfig
	initConfig(&cfg, m, browser.NopWallpaper(),
		term.RingBell, term.ScheduleNextTick, "", "")
	cfg.storage = storagestub.NewInMemoryService()

	assert.Equal(t, 4, cfg.editorTabspaces())
	assert.Equal(t, 72, cfg.editorRuler())
	assert.True(t, cfg.editorAutoPair())
	_, ok := cfg.wallpaper().NewComponent().(component.String)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/debug.log", cfg.logOutputPath())
	level, slogLevel := cfg.logLevel()
	assert.Equal(t, logrus.TraceLevel, level)
	assert.Equal(t, slog.LevelDebug, slogLevel)
	assert.True(t, term.InputMouse&cfg.inputMode() != 0)
	assert.True(t, term.InputEsc&cfg.inputMode() != 0)
	assert.Equal(t, "XX", cfg.tabNameSeparator())

	expectedConfig := handler.WindowManagerConfig{
		WindowManagerConfig: tcomponent.WindowManagerConfig{
			Frame:         true,
			FrameAttr:     term.Attributes{Fg: term.ColorRed},
			FrameCharSet:  component.FrameCharSetHighlight(),
			ScrollBarAttr: term.Attributes{Fg: term.GetColor("#f0f0f0")},
			ScrollBarChar: '|',
			NoMaxSize:     true,
		},
		Dim:                false,
		ScrollBarHoverChar: 'X',
		FocusFrameAttr:     handler.DefaultWindowManagerConfig().FrameAttr,
		FocusFrameCharSet:  handler.DefaultWindowManagerConfig().FrameCharSet,
	}
	assert.Equal(t, expectedConfig, cfg.windowManagerConfig())
	assert.True(t, cfg.frameUnion())

	expectedIcons := text.IconSet{
		Directory:     '',
		OpenDirectory: '',
		Default:       'x',
		Terminal:      '&',
		Shell:         '8',
		Extensions:    map[string]rune{".go": '$'},
	}
	actualIcons := cfg.icons()
	assert.Equal(t, expectedIcons, actualIcons)
	assert.Equal(t, text.CommentConfig{
		"go": {
			Line:  []string{"//"},
			Block: []text.CommentBlock{{Start: "/*", End: "*/"}},
		},
	}, cfg.editorComments())
	expectedLSPIcons := idelsp.IconSet{
		idelsp.IconDiagnosticError:       "E",
		idelsp.IconDiagnosticWarning:     "W",
		idelsp.IconDiagnosticInformation: "I",
		idelsp.IconDiagnosticHint:        "H",
		idelsp.IconCompilerInline:        ">",
		idelsp.IconCompilerEscape:        "^",
		idelsp.IconCompilerBounds:        "B",
		idelsp.IconCompilerNilcheck:      "0",
		idelsp.IconCompilerDefault:       "C",
	}
	assert.Equal(t, expectedLSPIcons, cfg.lspIcons())

	assert.Equal(t, workspaceBarKindDisabled, cfg.workspaceBarKind())

	expectedCommandAliases := map[string]text.CommandAlias{
		"todo": text.CommandAlias{Name: "todo",
			Commands: []string{"e file:///tmp/todo.md", "jenesaisquoi"}},
		"cherry": text.CommandAlias{Name: "cherry", Commands: []string{"bomb"}},
		"parcels": text.CommandAlias{
			Name: "parcels",
			Commands: []string{
				"Somethinggreater",
				"NowIcaresomemore",
				"Comingback",
			},
		},
		"daynight": text.CommandAlias{
			Name:     "daynight",
			Commands: []string{"ram"},
		},
		"dtmf": text.CommandAlias{
			Name:     "dtmf",
			Commands: []string{"ram"},
		},
		"editmix": text.CommandAlias{
			Name:     "editmix",
			Commands: []string{"e"},
		},
		"static_with_hist": text.CommandAlias{
			Name:     "static_with_hist",
			Commands: []string{"m"},
		},
		"bad": text.CommandAlias{
			Name:     "bad",
			Commands: []string{"x"},
		},
	}
	actualCommandAliases := cfg.commandAliases()
	parcelsAlias := actualCommandAliases["parcels"]
	require.Len(t, parcelsAlias.Completers, 1)
	assert.NotNil(t, parcelsAlias.Completers[0])
	parcelsAlias.Completers = nil
	actualCommandAliases["parcels"] = parcelsAlias

	daynight := actualCommandAliases["daynight"]
	require.Len(t, daynight.Completers, 1)
	require.NotNil(t, daynight.Completers[0])
	it, _, err := daynight.Completers[0](new(text.Component)).
		Complete(context.Background(), []string{})
	require.NoError(t, err)
	daynight.Completers = nil
	actualCommandAliases["daynight"] = daynight

	dtmf := actualCommandAliases["dtmf"]
	require.Len(t, dtmf.Completers, 1)
	assert.NotNil(t, dtmf.Completers[0])
	dtmf.Completers = nil
	actualCommandAliases["dtmf"] = dtmf

	editmix := actualCommandAliases["editmix"]
	require.Len(t, editmix.Completers, 2)
	assert.NotNil(t, editmix.Completers[0])
	assert.NotNil(t, editmix.Completers[1])
	editmix.Completers = nil
	actualCommandAliases["editmix"] = editmix

	staticWithHist := actualCommandAliases["static_with_hist"]
	require.Len(t, staticWithHist.Completers, 2)
	assert.NotNil(t, staticWithHist.Completers[0])
	// second factory exposes the static options "a" and "B"
	staticIt, _, sterr := staticWithHist.Completers[1](new(text.Component)).
		Complete(context.Background(), []string{})
	require.NoError(t, sterr)
	staticOpts, sterr := iterator.ToSlice(context.Background(), staticIt)
	require.NoError(t, sterr)
	assert.Equal(t, []string{"a", "B"}, staticOpts)
	staticWithHist.Completers = nil
	actualCommandAliases["static_with_hist"] = staticWithHist

	bad := actualCommandAliases["bad"]
	// {nope} is unknown so no factories are produced.
	assert.Empty(t, bad.Completers)
	bad.Completers = nil
	actualCommandAliases["bad"] = bad
	// the parser should record an error under
	// command.aliases.bad.completer (or command.aliases when aggregated).
	_, hasErr := cfg.errors["command.aliases"]
	assert.True(t, hasErr, "expected parser error for {nope} placeholder")

	actualOptions, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "B"}, actualOptions)

	assert.Equal(t, expectedCommandAliases, actualCommandAliases)
	assert.False(t, cfg.commandOverlayShowManual())
	assert.Equal(t, term.Attributes{Fg: term.ColorBlack, Bg: term.ColorYellow, Attrs: term.AttrBold},
		cfg.commandOverlayManualAttr())

	expectedFUCs := component.DefaultFrameUnionCharSet()
	expectedFUCs.HorizontalBottom = '━'
	expectedFUCs.HorizontalTop = '━'
	expectedFUCs.VerticalLeft = '┃'
	expectedFUCs.VerticalRight = '┃'
	expectedFUCs.TopLeft = '┏'
	expectedFUCs.TopRight = '┓'
	expectedFUCs.BottomLeft = '┗'
	expectedFUCs.BottomRight = '┛'
	expectedFUCs.Left = '┣'
	expectedFUCs.Right = '┫'
	expectedFUCs.Top = '┫'
	expectedFUCs.Bottom = '┫'
	assert.Equal(t, expectedFUCs, cfg.frameUnionCharset())

	statusBar := cfg.statusBarEnabled()
	assert.True(t, statusBar)

	statusBarCfg := cfg.statusBarConfig(workspaceapi.URI{}, nil, nil)
	assert.NotNil(t, statusBarCfg.ScheduleNextTick)
	statusBarCfg.ScheduleNextTick = nil
	assert.Equal(t, text.StatusBarConfig{
		BackgroundColor: term.ColorMaroon,
		Layout: []text.StatusBarComponent{
			{
				Template: "█%s█▓▒░",
				Type:     text.StatusBarStatus,
				Attributes: term.Attributes{
					Bg:    term.ColorRed,
					Fg:    term.ColorBlack,
					Attrs: term.AttrBold,
				},
			},
			{Template: "  %s", Type: text.StatusBarFilePath},
			{Template: "   %s", Type: text.StatusBarGitShortRef},
			{
				Template:   "   %d",
				Type:       text.StatusBarGitDiffAdded,
				Attributes: term.Attributes{Fg: term.ColorGreen},
			},
			{
				Template:   "   %d ",
				Type:       text.StatusBarGitDiffDeleted,
				Attributes: term.Attributes{Fg: term.ColorRed},
			},
			{Type: text.StatusBarVoid},
			{Template: "%d:", Type: text.StatusBarCoordinatesCursorX},
			{Template: "%d  ", Type: text.StatusBarCoordinatesCursorY},
			{Template: "%d lines  ", Type: text.StatusBarTotalLines},
			{
				Template:   "%s  ",
				Type:       text.StatusBarLanguage,
				Attributes: term.Attributes{Attrs: term.AttrBold},
			},
		},
		GitService: nil,
	}, statusBarCfg)

	auxBar := cfg.auxiliaryBarEnabled()
	assert.True(t, auxBar)

	folds := cfg.auxiliaryBarFolds()
	assert.True(t, folds)

	git := cfg.auxiliaryBarGit()
	assert.True(t, git)

	gitIcons := cfg.gitIconsEnabled()
	assert.True(t, gitIcons)

	iconsBar := cfg.iconsBarEnabled()
	assert.True(t, iconsBar)

	cursor := cfg.auxiliaryBarHighlightCursor()
	assert.False(t, cursor)

	enabled, absolute := cfg.auxiliaryBarLines()
	assert.True(t, enabled)
	assert.False(t, absolute)

	noti := cfg.notificationsConfig()
	assert.Equal(t, 1, noti.Padding)
	assert.Equal(t, notifications.ProgressRunes{
		Start:      '{',
		Current:    '-',
		CurrentTip: '>',
		Remain:     '_',
		End:        '}',
	}, noti.ProgressRunes)
	assert.False(t, noti.ProgressBar)
	assert.Equal(t, 1*time.Second, noti.AutoClose)
	assert.Equal(t, component.FrameCharSetHighlight(), noti.FrameCharSet)
	assert.Equal(t, term.Attributes{Fg: term.GetColor("#f0f0f0"), Bg: term.ColorRed},
		noti.Attributes)
	assert.Equal(t, term.Attributes{Fg: term.GetColor("#f0f0f0"), Bg: term.ColorRed},
		noti.BackgroundAttributes)

	assert.Equal(t, term.Attributes{Fg: term.GetColor("#f0f0f0")}, cfg.focusTabAttr())
	assert.Equal(t, term.Attributes{Fg: term.ColorWhite}, cfg.nonFocusTabAttr())
	assert.Equal(t, term.Attributes{Fg: term.ColorYellow, Bg: term.ColorWhite}, cfg.workspaceWallpaperAttr())
	assert.Equal(t, term.Attributes{Bg: term.ColorWhite}, cfg.workspaceWallpaperBackgroundAttr())

	reservoir := cfg.initialTerminalCapacity()
	assert.Equal(t, 0, reservoir)

	barConfig := cfg.pluginBarConfig()
	expectedBarConfig := plugin.BarConfig{
		StatusErrorIcon:       "X",
		StatusSuccessIcon:     "$",
		StatusErrorColor:      term.ColorYellow,
		StatusSuccessColor:    term.ColorBlue,
		StatusAnimationFrames: []string{"A", "B", "C"},
		BackgroundColor:       term.ColorGray,
		AlignBottom:           true,
		Layout: []plugin.BarComponent{
			{
				Type:     plugin.BarStatusIcon,
				Template: " %s █▓▒░",
				Attributes: term.Attributes{
					Fg: term.ColorWhite,
					Bg: term.ColorGray,
				},
			},
			{
				Type: plugin.BarAlignCenter,
			},
			{
				Type:     plugin.BarCommand,
				Template: "%s",
				Attributes: term.Attributes{
					Fg:    term.ColorWhite,
					Attrs: term.AttrBold,
				},
			},
			{
				Type:     plugin.BarAlignRight,
				Template: "  ",
			},
			{
				Type:     plugin.BarElapsed,
				Template: "░▒▓█ %s ",
				Attributes: term.Attributes{
					Fg: term.ColorWhite,
					Bg: term.ColorGray,
				},
			},
		},
	}
	assert.Equal(t, expectedBarConfig, barConfig)

	vteConfig := cfg.terminalConfig()
	assert.NotNil(t, vteConfig.ScheduleNextTick)
	vteConfig.ScheduleNextTick = nil
	assert.NotNil(t, vteConfig.RingBell)
	vteConfig.RingBell = nil
	expectedEmulatorConfig := vte.Config{
		CommandAndArgs:           []string{"sh"},
		Attributes:               term.Attributes{Fg: term.ColorWhite, Bg: term.ColorYellow},
		Clipboard:                cfg.clipboard(),
		ClipboardRegister:        clipboard.DefaultRegisterID,
		SelectionAttributes:      term.Attributes{Fg: term.ColorGreen, Bg: term.ColorTeal},
		NeedsAttentionAttributes: term.Attributes{Attrs: term.AttrBlink, Fg: term.ColorRed},
		Modal:                    true,
		DynamicTabName:           true,
		MaxLines:                 999,
		Bell:                     []byte{0x07},
		MinWidth:                 defaultMinWidth,
	}
	assert.Equal(t, expectedEmulatorConfig, vteConfig)

	expectedPrompt := browser.PromptConfig{
		TextAttr:      term.Attributes{Fg: term.ColorTeal},
		HighlightAttr: term.Attributes{Fg: term.GetColor("#f0f0f0"), Bg: term.ColorRed},
		MinWidth:      browser.DefaultConfig().MinWidth,
	}
	assert.Equal(t, expectedPrompt, cfg.promptConfig())

	assert.Equal(t, term.Attributes{Bg: term.ColorRed,
		Fg: term.GetColor("#f0f0f0")}, cfg.modalResultAttr())

	assert.Equal(t, term.Attributes{Bg: term.ColorRed,
		Fg: term.GetColor("#f1f1f1")}, cfg.modelessResultAttr())

	expectedSyntaxConfig := syntax.DefaultConfig()
	expectedSyntaxConfig.Autoindent = false
	expectedSyntaxConfig.CaptureNamesAttributes["function"] = term.Attributes{
		Fg: term.ColorGreen, Bg: term.ColorYellow}
	syntaxConfig := cfg.syntaxConfig()
	assert.NotNil(t, syntaxConfig.ScheduleNextTick)
	syntaxConfig.ScheduleNextTick = nil
	expectedSyntaxConfig.ScheduleNextTick = nil
	assert.Equal(t, expectedSyntaxConfig, syntaxConfig)

	assert.Equal(t, term.Attributes{Bg: term.ColorGreen,
		Fg: term.GetColor("#f9f9f9")}, cfg.modelessAttr())
	assert.Equal(t, term.Attributes{Bg: term.ColorYellow,
		Fg: term.GetColor("#f2f2f2")}, cfg.modalAttr())

	assert.Equal(t, "modal", cfg.editorMode())
	os.Setenv("SHELL", "")

	wantMappings := map[handler.Sequence][][]string{
		{First: term.KeyComb{Ch: 'f'}}:                    {{"searchfile"}},
		{First: term.KeyComb{Ch: 'l'}}:                    {{"searchtext"}},
		{First: term.KeyComb{Ch: 'x', Mod: term.ModCtrl}}: {{"closeDoors"}},
		{
			First: term.KeyComb{Ch: 'x', Mod: term.ModCtrl},
			Last:  term.KeyComb{Ch: 'p', Mod: term.ModCtrl},
		}: {{"openAllDoors"}},
		{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Ch: 'p', Mod: term.ModCtrl},
		}: {{"openDoors", "small"}},
		{
			First: term.KeyComb{Ch: 'x', Mod: term.ModCtrl},
			Last:  term.KeyComb{Ch: '9'},
		}: {{"openDoors", "1"}, {"large", "2"}},
	}
	assert.Equal(t, wantMappings, cfg.commandKeyMappings())
	assert.False(t, cfg.autoRestore())

	assert.Len(t, cfg.extensions(), 1)
	extensionCfgStruct := cfg.extensions()["fuzzy_search"]
	assert.Equal(t, "fuzzy_search", extensionCfgStruct.id)
	cfg.cfg["workspace"].(map[string]any)["wallpaper"] = ""
	extensionCfgStruct.parent.cfg["workspace"].(map[string]any)["wallpaper"] = ""
	cfg.defaultWallpaper.NewComponent = nil
	cfg.ringBell = nil
	cfg.scheduleNextTick = nil
	extensionCfgStruct.parent.defaultWallpaper.NewComponent = nil
	extensionCfgStruct.parent.ringBell = nil
	extensionCfgStruct.parent.scheduleNextTick = nil
	assert.Equal(t, &cfg, extensionCfgStruct.parent)

	extensionCfg, ok := extensionCfgStruct.config()
	require.True(t, ok)

	fileCfg, err := extensionCfg.GetConfig("file")
	require.NoError(t, err)
	cmd, err := fileCfg.GetString("command")
	require.NoError(t, err)
	assert.Equal(t, "ag -g \"\"", cmd)

	assert.NotZero(t, cfg.defaultAttr())
}

func TestLoadEmbededConfig(t *testing.T) {
	var cfg ideConfig
	err := loadConfig(&cfg, "nonExistent", browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
}

func TestTutorialFilesDecoded(t *testing.T) {
	cfg := ideConfig{
		cfg: map[string]any{
			"tutorials": map[string]any{
				"basics":   "/etc/x.star",
				"advanced": "/etc/y.star",
			},
		},
		errors: map[string]error{},
	}
	got := cfg.tutorialFiles()
	require.Equal(t, 2, len(got))
	assert.Equal(t, "/etc/x.star", got["basics"])
	assert.Equal(t, "/etc/y.star", got["advanced"])
	assert.Empty(t, cfg.errors)
}

func TestTutorialFilesMissingReturnsNil(t *testing.T) {
	cfg := ideConfig{
		cfg:    map[string]any{},
		errors: map[string]error{},
	}
	assert.Nil(t, cfg.tutorialFiles())
	assert.Empty(t, cfg.errors)
}

func TestTutorialFilesWrongRootTypeRecordsError(t *testing.T) {
	cfg := ideConfig{
		cfg: map[string]any{
			"tutorials": "not a map",
		},
		errors: map[string]error{},
	}
	assert.Nil(t, cfg.tutorialFiles())
	require.NotNil(t, cfg.errors["tutorials"])
	assert.Contains(t, cfg.errors["tutorials"].Error(), "invalid type")
}

func TestTutorialFilesEntryWrongTypeRecordsError(t *testing.T) {
	cfg := ideConfig{
		cfg: map[string]any{
			"tutorials": map[string]any{
				"good": "/etc/ok.star",
				"bad":  42,
			},
		},
		errors: map[string]error{},
	}
	got := cfg.tutorialFiles()
	assert.Equal(t, map[string]string{"good": "/etc/ok.star"}, got)
	require.NotNil(t, cfg.errors["tutorials.bad"])
	assert.Contains(t, cfg.errors["tutorials.bad"].Error(),
		"expected string path")
}

func TestShellMaxHistoryFromConfig(t *testing.T) {
	f, err := os.CreateTemp("", "*.star")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`config = {
    "console": {
        "max_history": 7,
    },
}`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Equal(t, 7, cfg.consoleMaxHistory())
}

func TestShellModalStartInsertFromConfig(t *testing.T) {
	f, err := os.CreateTemp("", "*.star")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`config = {
    "console": {
        "modal_start_insert": False,
    },
}`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.False(t, cfg.consoleModalStartInsert())
}

func TestShellModalStartInsertDefaultsTrue(t *testing.T) {
	f, err := os.CreateTemp("", "*.star")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`config = {
    "console": {
        "max_history": 7,
    },
}`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.True(t, cfg.consoleModalStartInsert())
}

func TestConsolePromptFromConfig(t *testing.T) {
	f, err := os.CreateTemp("", "*.star")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`config = {
    "console": {
        "prompt": "rune> ",
    },
}`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Equal(t, "rune> ", cfg.consolePrompt())
	assert.Equal(t, "rune> ", cfg.consoleCfg().prompt)
}

func TestConsolePromptDefaultsEmpty(t *testing.T) {
	f, err := os.CreateTemp("", "*.star")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`config = {
    "console": {
        "max_history": 7,
    },
}`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Empty(t, cfg.consolePrompt())
}

// TestShellEditorModalFromEditorMode asserts consoleCfg().modal mirrors the
// editor backing the console prompt: modal is modal, modeless is not, and
// exo follows its configured fallback.
func TestShellEditorModalFromEditorMode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		editor map[string]any
		want   bool
	}{
		{"unset editor defaults modal", nil, true},
		{"modal", map[string]any{"mode": "modal"}, true},
		{"modeless", map[string]any{"mode": "modeless"}, false},
		{"exo fallback modal", map[string]any{
			"mode": "exo",
			"exo":  map[string]any{"command": "vim {file}", "fallback": "modal"},
		}, true},
		{"exo fallback modeless", map[string]any{
			"mode": "exo",
			"exo":  map[string]any{"command": "vim {file}", "fallback": "modeless"},
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]any{}
			if tc.editor != nil {
				m["editor"] = tc.editor
			}
			cfg := &ideConfig{cfg: m, errors: map[string]error{}}
			assert.Equal(t, tc.want, cfg.consoleCfg().modal)
		})
	}
}

func TestInvalidAliases(t *testing.T) {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`
command:
  aliases:
    meh:
      - yay
    yay:
      - nay
    nay: meh
`)
	require.NoError(t, err)

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alias cycle detected")
	aliases, _ := cfg.parseAliasCommands()
	assert.Empty(t, aliases)
}

func TestTabspaces(t *testing.T) {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`
editor:
  tabspaces: 2
`)
	require.NoError(t, err)

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.editorTabspaces())
}

func TestEditorMaxSizeForSyntax(t *testing.T) {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`
editor:
  max_size_for_syntax: 2048
`)
	require.NoError(t, err)

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Equal(t, 2048, cfg.editorMaxSizeForSyntax())
}

func TestEditorIndentType(t *testing.T) {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	_, err = f.WriteString(`
editor:
  indents:
    yaml: spaces
    go: tab
`)
	require.NoError(t, err)

	var cfg ideConfig
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(),
		defaultConfigSource{src: "config = {}"},
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Equal(t, text.IndentConfig{
		"yaml": text.IndentRuneSpace,
		"go":   text.IndentRuneTab,
	}, cfg.editorIndents())
}

// TestCommandKeyBindingLookup asserts that commandKeyBindingLookup
// inverts the configured key bindings: each command line resolves to
// its key, multi-command sequences map every line to the same key, and
// an args-qualified miss falls back to the bare command.
func TestCommandKeyBindingLookup(t *testing.T) {
	t.Parallel()
	c := ideConfig{
		cfg: map[string]any{
			"command": map[string]any{
				"key_bindings": map[string]any{
					"<m-n>":      "windownew",
					"<s-m-n>":    "windownew right",
					"<c-x><c-h>": "windowfocus left",
					"<m-d>":      []any{"openDoors 1", "large 2"},
				},
			},
		},
		errors: map[string]error{},
	}
	lookup := c.commandKeyBindingLookup()

	assert.Equal(t, "<meta-n>", lookup("windownew", nil))
	assert.Equal(t, "<shift-meta-n>", lookup("windownew", []string{"right"}))
	assert.Equal(t, "<ctrl-x><ctrl-h>",
		lookup("windowfocus", []string{"left"}))
	// Multi-command sequence: every command line maps to the key.
	assert.Equal(t, "<meta-d>", lookup("openDoors", []string{"1"}))
	assert.Equal(t, "<meta-d>", lookup("large", []string{"2"}))
	// Args-qualified miss falls back to the bare command binding.
	assert.Equal(t, "<meta-n>", lookup("windownew", []string{"down"}))
	// Unbound command yields "".
	assert.Equal(t, "", lookup("tabclose", nil))
	assert.Empty(t, c.errors)
}
