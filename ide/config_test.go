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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/clipboard"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
)

var sampleConfig = `
extensions:
    fuzzy_file:
        path: "/path/extension_fuzzy_file"
        config:
            command: ag -g ""

log_path: "/tmp/debug.log"
log_level: "trace"
clipboard: memory
default_attr:
    bg: yellow
    fg: "#f2f2f2"

editor:
    status_bar:
        enabled: true
        background_attr:
            bg: maroon
        layout: '█{{ .Status | bg "red" | fg "black" | bold }}█▓▒░  {{ .Filepath }}   {{ .GitShortRef }}   {{ .GitDiffAdd | fg "green" }}   {{ .GitDiffDel | fg "red" }} {{ .ShiftRight }}{{ .CursorColumn }}:{{ .CursorLine }}  {{ .TotalLines }} lines  {{ .Language | bold }}  '
    aux_bar:
        enabled: true
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
    highlights:
        function:
            fg: green
            bg: yellow
    icons:
        default: x
        terminal: '&'
        .go: $
        .py: 1 # ignored

input_mode:
  - mouse
  - esc

command:
  show_manual_after: 2s
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
      completer: filepath
    daynight:
      commands: ram
      completer:
        - a
        - B
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
	assert.Equal(t, logrus.ErrorLevel, cfg.logLevel())
	assert.Equal(t, term.InputCurrent, cfg.inputMode())
	assert.Equal(t, component.DefaultFrameUnionCharSet(), cfg.frameUnionCharset())
	assert.True(t, cfg.frameUnion())
	assert.Equal(t, text.DefaultConfig().Icons, cfg.icons())

	assert.Equal(t, workspaceBarKindNumbers, cfg.workspaceBarKind())

	auxBar := cfg.auxiliaryBarEnabled()
	assert.False(t, auxBar)

	folds := cfg.auxiliaryBarFolds()
	assert.False(t, folds)

	git := cfg.auxiliaryBarGit()
	assert.False(t, git)

	gitBar := cfg.gitBarEnabled()
	assert.False(t, gitBar)

	statusBar := cfg.statusBarEnabled()
	assert.True(t, statusBar)

	enabled, absolute := cfg.auxiliaryBarLines()
	assert.True(t, enabled)
	assert.True(t, absolute)

	cursor := cfg.auxiliaryBarHighlightCursor()
	assert.True(t, cursor)

	actualNotifications := cfg.notificationsConfig()
	expectedNotifications := defaultNotificationsConfig()
	assert.NotNil(t, actualNotifications.Interrupter)
	actualNotifications.Interrupter = nil
	expectedNotifications.Interrupter = nil
	assert.Equal(t, expectedNotifications, actualNotifications)

	assert.Equal(t, browser.DefaultConfig().FocusTabAttr, cfg.focusTabAttr())
	assert.Equal(t, browser.DefaultConfig().NonFocusTabAttr, cfg.nonFocusTabAttr())
	assert.Equal(t, term.Attributes{}, cfg.workspaceWallpaperAttr())
	assert.Equal(t, term.Attributes{}, cfg.workspaceWallpaperBackgroundAttr())
	selectAttr := term.Attributes{Attrs: tcell.AttrReverse}

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
		Clipboard:                clipboard.NewInMemory(),
		SelectionAttributes:      selectAttr,
		NeedsAttentionAttributes: term.Attributes{Attrs: tcell.AttrBlink},
		Modal:                    false,
		ClipboardRegister:        clipboard.DefaultRegisterID,
		MaxLines:                 10_000,
		MinWidth:                 defaultMinWidth,
	}, vteConfig)
	assert.Equal(t, command.DefaultConfig().ShowManualAfter, cfg.commandOverlayShowManualAfter())
	assert.Equal(t, command.DefaultConfig().ManualAttr, cfg.commandOverlayManualAttr())
	assert.Zero(t, cfg.defaultAttr())

	assert.Equal(t, term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorYellow},
		cfg.modelessResultAttr())
	assert.Equal(t, term.Attributes{}, cfg.modelessAttr())
	assert.Equal(t, term.Attributes{}, cfg.modalAttr())
	assert.True(t, cfg.autoRestore())
	assert.Equal(t, "  ", cfg.tabNameSeparator())

	expectedSyntaxConfig := syntax.DefaultConfig()
	syntaxConfig := cfg.syntaxConfig()
	assert.NotNil(t, syntaxConfig.ScheduleNextTick)
	syntaxConfig.ScheduleNextTick = nil
	expectedSyntaxConfig.ScheduleNextTick = nil
	assert.Equal(t, expectedSyntaxConfig, syntaxConfig)

	assert.Equal(t, "modal", cfg.editorMode())
	os.Setenv("SHELL", "fish")
	assert.Equal(t, "", cfg.virtualEditorEditor())
	assert.Equal(t, "fish", cfg.virtualEditorShell())
	assert.Equal(t, term.Attributes{}, cfg.virtualEditorAttr())
	assert.Equal(t, term.Attributes{Attrs: tcell.AttrReverse},
		cfg.virtualEditorSelectionAttr())
}

func TestConfigDefault(t *testing.T) {
	ret := new(ideConfig)
	initDefaultConfig(ret, browser.NopWallpaper(),
		term.RingBell, term.ScheduleNextTick, "")
	assertDefaultConfig(t, ret)
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
		term.RingBell, term.ScheduleNextTick, "")

	assert.Equal(t, 4, cfg.editorTabspaces())
	_, ok := cfg.wallpaper().NewComponent().(component.String)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/debug.log", cfg.logOutputPath())
	assert.Equal(t, logrus.TraceLevel, cfg.logLevel())
	assert.True(t, term.InputMouse&cfg.inputMode() != 0)
	assert.True(t, term.InputEsc&cfg.inputMode() != 0)
	assert.Equal(t, "XX", cfg.tabNameSeparator())

	expectedConfig := handler.WindowManagerConfig{
		WindowManagerConfig: tcomponent.WindowManagerConfig{
			Frame:         true,
			FrameAttr:     term.Attributes{Fg: tcell.ColorRed},
			FrameCharSet:  component.FrameCharSetHighlight(),
			ScrollBarAttr: term.Attributes{Fg: tcell.GetColor("#f0f0f0")},
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
		Default:    'x',
		Terminal:   '&',
		Extensions: map[string]rune{".go": '$'},
	}
	actualIcons := cfg.icons()
	assert.Equal(t, expectedIcons, actualIcons)

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
			Completer: nil,
		},
		"daynight": text.CommandAlias{
			Name:      "daynight",
			Commands:  []string{"ram"},
			Completer: nil,
		},
	}
	actualCommandAliases := cfg.commandAliases()
	parcelsAlias := actualCommandAliases["parcels"]
	assert.NotNil(t, parcelsAlias.Completer)
	parcelsAlias.Completer = nil
	actualCommandAliases["parcels"] = parcelsAlias

	daynight := actualCommandAliases["daynight"]
	require.NotNil(t, daynight.Completer)
	it, _, err := daynight.Completer(new(text.Component)).
		Complete(context.Background(), []string{})
	require.NoError(t, err)
	daynight.Completer = nil
	actualCommandAliases["daynight"] = daynight

	actualOptions, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "B"}, actualOptions)

	assert.Equal(t, expectedCommandAliases, actualCommandAliases)
	assert.Equal(t, 2*time.Second, cfg.commandOverlayShowManualAfter())
	assert.Equal(t, term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorYellow, Attrs: tcell.AttrBold},
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
		BackgroundColor: tcell.ColorMaroon,
		Layout: []text.StatusBarComponent{
			{
				Template: "█%s█▓▒░",
				Type:     text.StatusBarStatus,
				Attributes: term.Attributes{
					Bg:    tcell.ColorRed,
					Fg:    tcell.ColorBlack,
					Attrs: tcell.AttrBold,
				},
			},
			{Template: "  %s", Type: text.StatusBarFilePath},
			{Template: "   %s", Type: text.StatusBarGitShortRef},
			{
				Template:   "   %d",
				Type:       text.StatusBarGitDiffAdded,
				Attributes: term.Attributes{Fg: tcell.ColorGreen},
			},
			{
				Template:   "   %d ",
				Type:       text.StatusBarGitDiffDeleted,
				Attributes: term.Attributes{Fg: tcell.ColorRed},
			},
			{Type: text.StatusBarVoid},
			{Template: "%d:", Type: text.StatusBarCoordinatesCursorX},
			{Template: "%d  ", Type: text.StatusBarCoordinatesCursorY},
			{Template: "%d lines  ", Type: text.StatusBarTotalLines},
			{
				Template:   "%s  ",
				Type:       text.StatusBarLanguage,
				Attributes: term.Attributes{Attrs: tcell.AttrBold},
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

	gitBar := cfg.gitBarEnabled()
	assert.True(t, gitBar)

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
	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0"), Bg: tcell.ColorRed},
		noti.Attributes)
	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0"), Bg: tcell.ColorRed},
		noti.BackgroundAttributes)

	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0")}, cfg.focusTabAttr())
	assert.Equal(t, term.Attributes{Fg: tcell.ColorWhite}, cfg.nonFocusTabAttr())
	assert.Equal(t, term.Attributes{Fg: tcell.ColorYellow, Bg: tcell.ColorWhite}, cfg.workspaceWallpaperAttr())
	assert.Equal(t, term.Attributes{Bg: tcell.ColorWhite}, cfg.workspaceWallpaperBackgroundAttr())

	reservoir := cfg.initialTerminalCapacity()
	assert.Equal(t, 0, reservoir)

	barConfig := cfg.pluginBarConfig()
	expectedBarConfig := plugin.BarConfig{
		StatusErrorIcon:       "X",
		StatusSuccessIcon:     "$",
		StatusErrorColor:      tcell.ColorYellow,
		StatusSuccessColor:    tcell.ColorBlue,
		StatusAnimationFrames: []string{"A", "B", "C"},
		BackgroundColor:       tcell.ColorGray,
		AlignBottom:           true,
		Layout: []plugin.BarComponent{
			{
				Type:     plugin.BarStatusIcon,
				Template: " %s █▓▒░",
				Attributes: term.Attributes{
					Fg: tcell.ColorWhite,
					Bg: tcell.ColorGray,
				},
			},
			{
				Type: plugin.BarAlignCenter,
			},
			{
				Type:     plugin.BarCommand,
				Template: "%s",
				Attributes: term.Attributes{
					Fg:    tcell.ColorWhite,
					Attrs: tcell.AttrBold,
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
					Fg: tcell.ColorWhite,
					Bg: tcell.ColorGray,
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
		Attributes:               term.Attributes{Fg: tcell.ColorWhite, Bg: tcell.ColorYellow},
		Clipboard:                clipboard.NewInMemory(),
		ClipboardRegister:        clipboard.DefaultRegisterID,
		SelectionAttributes:      term.Attributes{Fg: tcell.ColorGreen, Bg: tcell.ColorTeal},
		NeedsAttentionAttributes: term.Attributes{Attrs: tcell.AttrBlink, Fg: tcell.ColorRed},
		Modal:                    true,
		DynamicTabName:           true,
		MaxLines:                 999,
		Bell:                     []byte{0x07},
		MinWidth:                 defaultMinWidth,
	}
	assert.Equal(t, expectedEmulatorConfig, vteConfig)

	expectedPrompt := browser.PromptConfig{
		TextAttr:      term.Attributes{Fg: tcell.ColorTeal},
		HighlightAttr: term.Attributes{Fg: tcell.GetColor("#f0f0f0"), Bg: tcell.ColorRed},
		MinWidth:      browser.DefaultConfig().MinWidth,
	}
	assert.Equal(t, expectedPrompt, cfg.promptConfig())

	assert.Equal(t, term.Attributes{Bg: tcell.ColorRed,
		Fg: tcell.GetColor("#f0f0f0")}, cfg.modalResultAttr())

	assert.Equal(t, term.Attributes{Bg: tcell.ColorRed,
		Fg: tcell.GetColor("#f1f1f1")}, cfg.modelessResultAttr())

	expectedSyntaxConfig := syntax.DefaultConfig()
	expectedSyntaxConfig.Autoindent = false
	expectedSyntaxConfig.CaptureNamesAttributes["function"] = term.Attributes{
		Fg: tcell.ColorGreen, Bg: tcell.ColorYellow}
	syntaxConfig := cfg.syntaxConfig()
	assert.NotNil(t, syntaxConfig.ScheduleNextTick)
	syntaxConfig.ScheduleNextTick = nil
	expectedSyntaxConfig.ScheduleNextTick = nil
	assert.Equal(t, expectedSyntaxConfig, syntaxConfig)

	assert.Equal(t, term.Attributes{Bg: tcell.ColorGreen,
		Fg: tcell.GetColor("#f9f9f9")}, cfg.modelessAttr())
	assert.Equal(t, term.Attributes{Bg: tcell.ColorYellow,
		Fg: tcell.GetColor("#f2f2f2")}, cfg.modalAttr())

	assert.Equal(t, "modal", cfg.editorMode())
	os.Setenv("SHELL", "")
	assert.Equal(t, "vim", cfg.virtualEditorEditor())
	assert.Equal(t, "bash", cfg.virtualEditorShell())
	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0"), Bg: tcell.ColorRed},
		cfg.virtualEditorAttr())
	virtualEditorSelectionAttr := cfg.virtualEditorSelectionAttr()
	assert.Equal(t, tcell.GetColor("#f3f3f3"), virtualEditorSelectionAttr.Fg)
	assert.Equal(t, tcell.ColorGreen, virtualEditorSelectionAttr.Bg)

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
	extensionCfgStruct := cfg.extensions()["fuzzy_file"]
	assert.Equal(t, "fuzzy_file", extensionCfgStruct.id)
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

	cmd, err := extensionCfg.GetString("command")
	require.NoError(t, err)
	assert.Equal(t, "ag -g \"\"", cmd)

	assert.NotZero(t, cfg.defaultAttr())
}

func TestLoadEmbededConfig(t *testing.T) {
	var cfg ideConfig
	err := loadConfig(&cfg, "nonExistent", browser.NopWallpaper(), "{}",
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
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
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(), "{}",
		term.RingBell, term.ScheduleNextTick, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alias cycle detected")
	assert.Empty(t, cfg.commandAliases())
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
	err = loadConfig(&cfg, f.Name(), browser.NopWallpaper(), "{}",
		term.RingBell, term.ScheduleNextTick, "")
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.editorTabspaces())
}
