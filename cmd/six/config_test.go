package main

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sampleConfig = `
plugins:
    fuzzy_file:
        path: "/path/plugin_fuzzy_file"
        config:
            command: ag -g ""

log_path: "/tmp/debug.log"
log_level: "trace"

vi:
    search_attr:
        bg: red
        fg: 219
    debug: true
    wrap: true

input_mode:
  - mouse
  - esc

output_mode: color_256

command:
  key_bindings:
    f: searchFile
    l: searchLine
    <c-x>: closeDoors
    <c-x><c-p>: openAllDoors
    f<c-p>: openSmallDoors
    <c-x>f: openLargeDoors
    <-x>f: invalidMapping

browser:
    tabspaces: 4
    prompt:
        width: 20
        height: 10
        text_attr:
            fg: cyan
        highlight_attr:
            bg: red
            fg: 219
    window_manager:
        frame: true
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
        bg: cyan
    focus_tab_attr:
        fg: 219
    non_focus_tab_attr:
        fg: white
    wallpaper: abc
    wallpaper_attr:
        fg: yellow
        bg: white
    wallpaper_background_attr:
        bg: white

`

func assertDefaultConfig(t *testing.T, cfg *ideConfig) {
	assert.Len(t, cfg.plugins(), 0)
	assert.Equal(t, 4, cfg.browserTabspaces())
	assert.NotZero(t, cfg.browserWallpaper())
	assert.Equal(t, component.DefaultWindowManagerConfig(), cfg.windowManagerConfig())
	assert.Equal(t, "", cfg.logOutputPath())
	assert.Equal(t, logrus.ErrorLevel, cfg.logLevel())
	assert.Equal(t, term.Output256, cfg.outputMode())
	assert.Equal(t, term.InputCurrent, cfg.inputMode())
	assert.Equal(t, component.DefaultFrameUnionCharSet(), cfg.frameUnionCharset())
	assert.Equal(t, browser.DefaultConfig().MessageBarAttr, cfg.messageBarAttr())
	assert.Equal(t, browser.DefaultConfig().FocusTabAttr, cfg.focusTabAttr())
	assert.Equal(t, browser.DefaultConfig().NonFocusTabAttr, cfg.nonFocusTabAttr())
	assert.Equal(t, browser.DefaultConfig().WallpaperAttr, cfg.browserWallpaperAttr())
	assert.Equal(t, browser.DefaultConfig().WallpaperBackgroundAttr, cfg.browserWallpaperBackgroundAttr())
}

func TestDefaultConfig(t *testing.T) {
	ret := new(ideConfig)
	initDefaultConfig(ret)
	assertDefaultConfig(t, ret)
}

func TestDecodeConfigError(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	_, err = f.WriteString("||\\n\x00{'BABY':'$$'}")
	require.NoError(t, err)

	var ret ideConfig
	err = loadConfig(&ret, f.Name())
	assert.Error(t, err)
	assertDefaultConfig(t, &ret)
}

func TestConfigSetting(t *testing.T) {
	m, err := decodeConfig(strings.NewReader(sampleConfig))
	require.NoError(t, err)

	var cfg ideConfig
	initConfig(&cfg, m)

	assert.Equal(t, 4, cfg.browserTabspaces())
	assert.Equal(t, "abc", cfg.browserWallpaper())
	assert.Equal(t, "/tmp/debug.log", cfg.logOutputPath())
	assert.Equal(t, logrus.TraceLevel, cfg.logLevel())
	assert.Equal(t, term.Output256, cfg.outputMode())
	assert.True(t, term.InputMouse&cfg.inputMode() != 0)
	assert.True(t, term.InputEsc&cfg.inputMode() != 0)

	expectedConfig := component.WindowManagerConfig{
		Frame:        true,
		FrameAttr:    term.Attributes{Fg: term.ColorRed},
		FrameCharSet: component.FrameCharSetHighlight(),
	}
	assert.Equal(t, expectedConfig, cfg.windowManagerConfig())

	assert.Len(t, cfg.plugins(), 1)
	pluginCfgStruct := cfg.plugins()["fuzzy_file"]
	assert.Equal(t, "fuzzy_file", pluginCfgStruct.id)
	assert.Equal(t, &cfg, pluginCfgStruct.parent)

	pluginCfg, ok := pluginCfgStruct.config()
	require.True(t, ok)

	cmd, err := pluginCfg.GetString("command")
	require.NoError(t, err)
	assert.Equal(t, "ag -g \"\"", cmd)

	expectedCs := component.FrameUnionCharSet{Left: '┣', Right: '┫', Top: '┫', Bottom: '┫'}
	assert.Equal(t, expectedCs, cfg.frameUnionCharset())

	assert.Equal(t, term.Attributes{Fg: term.ColorWhite, Bg: term.ColorCyan}, cfg.messageBarAttr())
	assert.Equal(t, term.Attributes{Fg: term.Attribute(219)}, cfg.focusTabAttr())
	assert.Equal(t, term.Attributes{Fg: term.ColorWhite}, cfg.nonFocusTabAttr())
	assert.Equal(t, term.Attributes{Fg: term.ColorYellow, Bg: term.ColorWhite}, cfg.browserWallpaperAttr())
	assert.Equal(t, term.Attributes{Bg: term.ColorWhite}, cfg.browserWallpaperBackgroundAttr())

	expectedPrompt := browser.PromptConfig{
		Width:         20,
		Height:        10,
		TextAttr:      term.Attributes{Fg: term.ColorCyan},
		HighlightAttr: term.Attributes{Fg: 219, Bg: term.ColorRed},
	}
	assert.Equal(t, expectedPrompt, cfg.promptConfig())

	assert.Equal(t, term.Attributes{Bg: term.ColorRed,
		Fg: term.Attribute(219)}, cfg.viResultAttr())
	assert.True(t, cfg.viDebug())
	assert.True(t, cfg.viWrap())
	wantMappings := map[handler.Sequence]string{
		{First: term.KeyComb{Ch: 'f'}}:            "searchFile",
		{First: term.KeyComb{Ch: 'l'}}:            "searchLine",
		{First: term.KeyComb{Key: term.KeyCtrlX}}: "closeDoors",
		{
			First: term.KeyComb{Key: term.KeyCtrlX},
			Last:  term.KeyComb{Key: term.KeyCtrlP},
		}: "openAllDoors",
		{
			First: term.KeyComb{Ch: 'f'},
			Last:  term.KeyComb{Key: term.KeyCtrlP},
		}: "openSmallDoors",
		{
			First: term.KeyComb{Key: term.KeyCtrlX},
			Last:  term.KeyComb{Ch: 'f'},
		}: "openLargeDoors",
	}
	assert.Equal(t, wantMappings, cfg.commandKeyMappings())
}
