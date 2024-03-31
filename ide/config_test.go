package ide

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ernestrc/tcell/v3"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text/clipboard"
	sysclip "unstable.build/go-tui/text/clipboard/system"
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

editor:
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
  manual_attr:
    fg: black
    bg: yellow
    flags: bold
  key_bindings:
    f: searchFile
    l: searchText
    <c-x>: closeDoors
    <c-x><c-p>: openAllDoors
    f<c-p>: openDoors small
    <c-x>9:
      - openDoors
      - large
    <c-x>p:
      - invalid
      - smtg:
        - else
    <-x>f: invalidMapping

notifications:
    auto_close: 1s
    progress_bar: false
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
    shell: sh
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
	assert.Equal(t, 4, cfg.browserTabspaces())
	assert.NotZero(t, cfg.wallpaper())
	defWmConfig := handler.DefaultWindowManagerConfig()
	defWmConfig.FocusFrameAttr = defWmConfig.FrameAttr
	defWmConfig.FocusFrameCharSet = defWmConfig.FrameCharSet
	assert.Equal(t, defWmConfig, cfg.windowManagerConfig())
	assert.Equal(t, "", cfg.logOutputPath())
	assert.Equal(t, logrus.ErrorLevel, cfg.logLevel())
	assert.Equal(t, term.InputCurrent, cfg.inputMode())
	assert.Equal(t, component.DefaultFrameUnionCharSet(), cfg.frameUnionCharset())

	actualNotifications := cfg.notificationsConfig()
	expectedNotifications := browser.DefaultConfig().Notifications
	assert.NotNil(t, actualNotifications.Interrupter)
	actualNotifications.Interrupter = nil
	expectedNotifications.Interrupter = nil
	assert.Equal(t, expectedNotifications, actualNotifications)

	assert.Equal(t, browser.DefaultConfig().FocusTabAttr, cfg.focusTabAttr())
	assert.Equal(t, browser.DefaultConfig().NonFocusTabAttr, cfg.nonFocusTabAttr())
	assert.Equal(t, browser.DefaultConfig().WallpaperAttr, cfg.workspaceWallpaperAttr())
	assert.Equal(t, browser.DefaultConfig().WallpaperBackgroundAttr, cfg.workspaceWallpaperBackgroundAttr())
	selectAttr := term.Attributes{Attrs: tcell.AttrReverse}
	reg, err := sysclip.NewRegister()
	require.NoError(t, err)

	vteConfig := cfg.terminalConfig()
	assert.NotNil(t, vteConfig.RingBell)
	assert.NotNil(t, vteConfig.ScheduleBell)
	vteConfig.ScheduleBell = nil
	vteConfig.RingBell = nil
	assert.Equal(t, vte.Config{
		Clipboard:                reg,
		SelectionAttributes:      selectAttr,
		NeedsAttentionAttributes: term.Attributes{Attrs: tcell.AttrBlink},
		Modal:                    true,
		ClipboardRegister:        clipboard.DefaultRegisterID,
	}, vteConfig)
	assert.Equal(t, command.DefaultConfig().ShowManualAfter, cfg.commandOverlayShowManualAfter())
	assert.Equal(t, command.DefaultConfig().ManualAttr, cfg.commandOverlayManualAttr())

	assert.Equal(t, term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorYellow},
		cfg.modelessResultAttr())
	assert.Equal(t, term.Attributes{}, cfg.modelessAttr())
	assert.Equal(t, term.Attributes{}, cfg.modalAttr())
	assert.False(t, cfg.modelessWrap())
	assert.True(t, cfg.autoRestore())

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
	initDefaultConfig(ret, "myWallpaper")
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
	err = loadFileConfig(&ret, f.Name())
	assert.Error(t, err)
	assertDefaultConfig(t, &ret)
}

func TestConfigSetting(t *testing.T) {
	m, err := decodeConfig(strings.NewReader(sampleConfig))
	require.NoError(t, err)

	var cfg ideConfig
	initConfig(&cfg, m, "myWallpaper")

	assert.Equal(t, 4, cfg.browserTabspaces())
	assert.Equal(t, "abc", cfg.wallpaper())
	assert.Equal(t, "/tmp/debug.log", cfg.logOutputPath())
	assert.Equal(t, logrus.TraceLevel, cfg.logLevel())
	assert.True(t, term.InputMouse&cfg.inputMode() != 0)
	assert.True(t, term.InputEsc&cfg.inputMode() != 0)

	expectedConfig := handler.WindowManagerConfig{
		WindowManagerConfig: component.WindowManagerConfig{
			Frame:        true,
			FrameAttr:    term.Attributes{Fg: tcell.ColorRed},
			FrameCharSet: component.FrameCharSetHighlight(),
		},
		Dim:               false,
		FocusFrameAttr:    handler.DefaultWindowManagerConfig().FrameAttr,
		FocusFrameCharSet: handler.DefaultWindowManagerConfig().FrameCharSet,
	}
	assert.Equal(t, expectedConfig, cfg.windowManagerConfig())

	assert.Len(t, cfg.extensions(), 1)
	extensionCfgStruct := cfg.extensions()["fuzzy_file"]
	assert.Equal(t, "fuzzy_file", extensionCfgStruct.id)
	assert.Equal(t, &cfg, extensionCfgStruct.parent)

	extensionCfg, ok := extensionCfgStruct.config()
	require.True(t, ok)

	cmd, err := extensionCfg.GetString("command")
	require.NoError(t, err)
	assert.Equal(t, "ag -g \"\"", cmd)

	expectedCommandAliases := map[string][]string{
		"todo":   {"e file:///tmp/todo.md", "jenesaisquoi"},
		"cherry": {"bomb"},
	}
	assert.Equal(t, expectedCommandAliases, cfg.commandAliases())
	assert.Equal(t, 2*time.Second, cfg.commandOverlayShowManualAfter())
	assert.Equal(t, term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorYellow, Attrs: tcell.AttrBold},
		cfg.commandOverlayManualAttr())

	expectedFUCs := component.FrameUnionCharSet{Left: '┣', Right: '┫', Top: '┫', Bottom: '┫'}
	assert.Equal(t, expectedFUCs, cfg.frameUnionCharset())

	notifications := cfg.notificationsConfig()
	assert.False(t, notifications.ProgressBar)
	assert.Equal(t, 1*time.Second, notifications.AutoClose)
	assert.Equal(t, component.FrameCharSetHighlight(), notifications.FrameCharSet)
	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0"), Bg: tcell.ColorRed},
		notifications.Attributes)
	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0"), Bg: tcell.ColorRed},
		notifications.BackgroundAttributes)

	assert.Equal(t, term.Attributes{Fg: tcell.GetColor("#f0f0f0")}, cfg.focusTabAttr())
	assert.Equal(t, term.Attributes{Fg: tcell.ColorWhite}, cfg.nonFocusTabAttr())
	assert.Equal(t, term.Attributes{Fg: tcell.ColorYellow, Bg: tcell.ColorWhite}, cfg.workspaceWallpaperAttr())
	assert.Equal(t, term.Attributes{Bg: tcell.ColorWhite}, cfg.workspaceWallpaperBackgroundAttr())

	vteConfig := cfg.terminalConfig()
	assert.NotNil(t, vteConfig.ScheduleBell)
	vteConfig.ScheduleBell = nil
	assert.NotNil(t, vteConfig.RingBell)
	vteConfig.RingBell = nil
	expectedEmulatorConfig := vte.Config{
		Shell:                    "sh",
		Attributes:               term.Attributes{Fg: tcell.ColorWhite, Bg: tcell.ColorYellow},
		Clipboard:                clipboard.NewInMemory(),
		ClipboardRegister:        clipboard.DefaultRegisterID,
		SelectionAttributes:      term.Attributes{Fg: tcell.ColorGreen, Bg: tcell.ColorTeal},
		NeedsAttentionAttributes: term.Attributes{Attrs: tcell.AttrBlink, Fg: tcell.ColorRed},
		Modal:                    true,
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
	assert.True(t, cfg.modalDebug())
	assert.True(t, cfg.modalWrap())

	assert.Equal(t, term.Attributes{Bg: tcell.ColorRed,
		Fg: tcell.GetColor("#f1f1f1")}, cfg.modelessResultAttr())
	assert.False(t, cfg.modelessWrap())

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

	wantMappings := map[handler.Sequence][]string{
		{First: term.KeyComb{Ch: 'f'}}:            {"searchFile"},
		{First: term.KeyComb{Ch: 'l'}}:            {"searchText"},
		{First: term.KeyComb{Key: term.KeyCtrlX}}: {"closeDoors"},
		{
			First: term.KeyComb{Key: term.KeyCtrlX},
			Last:  term.KeyComb{Key: term.KeyCtrlP},
		}: {"openAllDoors"},
		{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Key: term.KeyCtrlP},
		}: {"openDoors", "small"},
		{
			First: term.KeyComb{Key: term.KeyCtrlX},
			Last:  term.KeyComb{Ch: '9'},
		}: {"openDoors", "large"},
	}
	assert.Equal(t, wantMappings, cfg.commandKeyMappings())
	assert.False(t, cfg.autoRestore())
}

func TestLoadEmbededConfig(t *testing.T) {
	var cfg ideConfig
	err := loadConfig(&cfg, "nonExistent", "notEmpty")
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
	err = loadConfig(&cfg, f.Name(), "notEmpty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Alias cycle detected")
	assert.Empty(t, cfg.commandAliases())
}
