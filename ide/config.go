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
	_ "embed"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/tcell/v3"
	yaml "gopkg.in/yaml.v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
)

const (
	inputEsc           = "esc"
	inputAlt           = "alt"
	inputMouse         = "mouse"
	inputCurrent       = "current"
	editorModeModal    = "modal"
	editorModeModeless = "modeless"
	keyCommandAliases  = "aliases"
	keyCommandKey      = "key"
)

var (
	defaultWindowManagerConfig = handler.DefaultWindowManagerConfig()
)

type extensionConfig struct {
	id     string
	parent *ideConfig
	cfg    config.Config
}

type ideConfig struct {
	defaultWallpaper browser.Wallpaper

	cfg              map[string]interface{}
	errors           map[string]error
	ringBell         func()
	scheduleNextTick func(func()) bool
	zdotDir          string
}

func overrideConfig(ideConfig, cfg map[string]interface{}) {
	for key, new := range cfg {
		prev, ok := ideConfig[key]
		if !ok {
			ideConfig[key] = new
			continue
		}
		prevMap, ok := prev.(map[string]interface{})
		if !ok {
			ideConfig[key] = new
			continue
		}
		newMap, ok := new.(map[string]interface{})
		if !ok {
			ideConfig[key] = new
			continue
		}
		overrideConfig(prevMap, newMap)
	}
}

func initConfig(
	c *ideConfig, cfg map[string]interface{}, defaultWallpaper browser.Wallpaper,
	ringBell func(), scheduleNextTick func(func()) bool,
	zdotDir string,
) {
	c.cfg = cfg
	c.ringBell = ringBell
	c.scheduleNextTick = scheduleNextTick
	c.zdotDir = zdotDir
	c.defaultWallpaper = defaultWallpaper
	c.errors = make(map[string]error)
}

func initDefaultConfig(
	c *ideConfig, defaultWallpaper browser.Wallpaper,
	ringBell func(), scheduleNextTick func(func()) bool,
	zdotDir string,
) {
	cfg := make(map[string]interface{})
	initConfig(c, cfg, defaultWallpaper, ringBell,
		scheduleNextTick, zdotDir)
}

func (c ideConfig) command() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(config.MapConfig(c.cfg), "command")
}

func (c ideConfig) commandKeyMappings() map[handler.Sequence][][]string {
	ret := make(map[handler.Sequence][][]string)
	cfg, ok := c.command()
	if !ok {
		return ret
	}
	m, err := cfg.GetMap("key_bindings")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["key_bindings"] = err
		}
		return ret
	}

	for k, v := range m {
		seq, err := handler.ParseSequence(k)
		if err != nil {
			seq.First, err = term.ParseKey(k)
			if err != nil {
				c.errors["key_bindings."+k] = err
				continue
			}
		}
		cmdsAndArgsSliceIfc, ok := v.([]interface{})
		if ok {
			var cmdsAndArgs [][]string
			for _, ifc := range cmdsAndArgsSliceIfc {
				cmdAndArgs, ok := ifc.(string)
				if !ok {
					c.errors["key_bindings."+k] = errors.New(
						"expected space-separated multi-word " +
							"string or []string but found unknown type")
					cmdsAndArgs = nil
					break
				}
				cmdsAndArgs = append(cmdsAndArgs,
					strings.Split(strings.Trim(cmdAndArgs, " "), " "))
			}
			if len(cmdsAndArgs) != 0 {
				ret[seq] = cmdsAndArgs
			}
		} else {
			cmd, ok := v.(string)
			if !ok {
				c.errors["key_bindings."+k] = errors.New(
					"expected space-separated multi-word string or " +
						"[]string but found unknown type")
				continue
			}
			parts := strings.Split(strings.Trim(cmd, " "), " ")
			ret[seq] = [][]string{parts}
		}
	}

	return ret
}

func (c ideConfig) commandKey() (ret term.KeyComb) {
	if c.editorMode() == editorModeModeless {
		ret = defaultModelessCommandKey
	} else {
		ret = defaultModalCommandKey
	}
	cfg, ok := c.command()
	if !ok {
		return
	}
	cfgKey, err := cfg.GetString(keyCommandKey)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("command.%s", keyCommandKey)] = err
		}
		return
	}
	key, err := term.ParseKey(cfgKey)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("command.%s", keyCommandKey)] = err
		}
		return
	}
	ret = key
	return
}

func (c ideConfig) commandMaxHistory() (ret int) {
	ret = text.DefaultConfig().CommandMaxHistory
	b, ok := c.command()
	if !ok {
		return
	}
	height, err := b.GetInt("max_history")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["command.max_history"] = err
		}
		return
	}
	ret = height
	return
}

func (c ideConfig) prompt() (config.Config, bool) {
	b, ok := c.browser()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "prompt")
}

func (c ideConfig) getCfgAttr(
	key string, def term.Attributes,
	cfgKey string,
	cfgFn func() (config.Config, bool),
) (attr term.Attributes) {
	attr = def
	cfg, ok := cfgFn()
	if !ok {
		return
	}
	cfgAttr, err := config.GetAttributes(cfg, key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[cfgKey+"."+key] = err
		}
		return
	}
	attr = cfgAttr
	return
}

func (c ideConfig) getCommandAttr(
	key string, def term.Attributes) term.Attributes {
	return c.getCfgAttr(key, def, "command", c.command)
}

func (c ideConfig) promptTextAttr() term.Attributes {
	return c.getCfgAttr(
		"text_attr", browser.DefaultConfig().TextAttr,
		"prompt", c.prompt,
	)
}

func (c ideConfig) promptHighlightAttr() term.Attributes {
	return c.getCfgAttr(
		"highlight_attr", browser.DefaultConfig().HighlightAttr,
		"prompt", c.prompt,
	)
}

func (c ideConfig) promptBackgroundAttr() term.Attributes {
	return c.getCfgAttr(
		"background_attr", browser.DefaultConfig().BackgroundAttr,
		"prompt", c.prompt,
	)
}

func (c ideConfig) commandOverlayMatchedTextAttr() (ret term.Attributes) {
	def := text.DefaultCommandOverlayConfig().MatchedTextAttr
	return c.getCommandAttr("matched_text_attr", def)
}

func (c ideConfig) commandOverlayFocusElementAttr() (ret term.Attributes) {
	def := text.DefaultCommandOverlayConfig().FocusElementAttr
	return c.getCommandAttr("focus_element_attr", def)
}

func (c ideConfig) commandOverlayElementAttr() (ret term.Attributes) {
	def := text.DefaultCommandOverlayConfig().ElementAttr
	return c.getCommandAttr("element_attr", def)
}

func (c ideConfig) commandOverlayManualAttr() (ret term.Attributes) {
	def := text.DefaultCommandOverlayConfig().ManualAttr
	return c.getCommandAttr("manual_attr", def)
}

func (c ideConfig) commandOverlayShowManualAfter() (ret time.Duration) {
	ret = text.DefaultCommandOverlayConfig().ShowManualAfter
	cfg, ok := c.command()
	if !ok {
		return
	}
	key := "show_manual_after"
	dur, err := config.GetDuration(cfg, key, ret)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("command.%s", key)] = err
		}
		return
	}
	ret = dur
	return
}

func (c ideConfig) commandAliases() (ret map[string]text.CommandAlias) {
	ret = make(map[string]text.CommandAlias)
	cfg, ok := c.command()
	if !ok {
		return
	}

	cfgsAliases, err := cfg.GetMap(keyCommandAliases)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("command.%s", keyCommandAliases)] = err
		}
		return
	}

	for k, v := range cfgsAliases {
		alias := text.CommandAlias{Name: k}
		switch tp := v.(type) {
		case map[string]any:
			for kk, vv := range tp {
				switch kk {
				case "commands", "command":
					switch ttp := vv.(type) {
					case string:
						alias.Commands = make([]string, 1)
						alias.Commands[0] = ttp
					case []any:
						alias.Commands = make([]string, 0)
						for _, v := range ttp {
							switch vtp := v.(type) {
							case string:
								alias.Commands = append(alias.Commands, vtp)
							default:
								err = multierr.Append(err,
									fmt.Errorf("invalid value type for command.%s.%s", keyCommandAliases, k))
							}
						}
					}
				case "completer":
					switch ttp := vv.(type) {
					case string:
						switch ttp {
						case "filepath":
							alias.Completer = filepathCompleter
						case "history":
							alias.Completer = nil // default is history
						default:
							err = multierr.Append(err,
								fmt.Errorf("invalid value for command.%s.%s.completer: "+
									"expected 'history', 'files' or list of completion options",
									keyCommandAliases, k))
						}
					case []any:
						options := make([]string, 0)
						for _, v := range ttp {
							switch vtp := v.(type) {
							case string:
								options = append(options, vtp)
							default:
								err = multierr.Append(err,
									fmt.Errorf("invalid value type for option in "+
										"command.%s.%s.completer: expected string or list of "+
										"strings", keyCommandAliases, k))
							}
						}
						alias.Completer = func(c *text.Component) command.Completer {
							return command.FuncCompleter(func(context.Context, []string) (
								iterator.Iterator[string], string, error,
							) {
								return iterator.FromSlice(options), "", nil
							})
						}
					}
				}
			}
		case string:
			alias.Commands = make([]string, 1)
			alias.Commands[0] = tp
		case []any:
			alias.Commands = make([]string, 0)
			for _, v := range tp {
				switch vtp := v.(type) {
				case string:
					alias.Commands = append(alias.Commands, vtp)
				default:
					err = multierr.Append(err,
						fmt.Errorf("invalid value type for command.%s.%s", keyCommandAliases, k))
				}
			}
		}
		if len(alias.Commands) != 0 {
			ret[k] = alias
		}
	}
	if err != nil {
		c.errors[fmt.Sprintf("command.%s", keyCommandAliases)] = err
	}
	return
}

func (c ideConfig) commandOverlayConfig() text.CommandOverlayConfig {
	cfg := text.CommandOverlayConfig{
		MatchedTextAttr:  c.commandOverlayMatchedTextAttr(),
		FocusElementAttr: c.commandOverlayFocusElementAttr(),
		ElementAttr:      c.commandOverlayElementAttr(),
		ManualAttr:       c.commandOverlayManualAttr(),
		ShowManualAfter:  c.commandOverlayShowManualAfter(),
	}
	return cfg
}

func (c ideConfig) promptConfig() browser.PromptConfig {
	ret := browser.DefaultConfig().PromptConfig
	ret.TextAttr = c.promptTextAttr()
	ret.HighlightAttr = c.promptHighlightAttr()
	ret.BackgroundAttr = c.promptBackgroundAttr()
	return ret
}

func (c ideConfig) getConfig(cfg config.Config, key string) (config.Config, bool) {
	cfg, err := cfg.GetConfig(key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[key] = err
		}
		return nil, false
	}
	return cfg, true
}

func (c ideConfig) windowManager() (config.Config, bool) {
	b, ok := c.browser()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "window_manager")
}

func (c ideConfig) windowFrameAttr() (attr term.Attributes) {
	return c.windowAttr("frame_attr", defaultWindowManagerConfig.FrameAttr)
}

type workspaceBarKind uint8

const (
	workspaceBarKindDisabled workspaceBarKind = iota
	workspaceBarKindPaths
	workspaceBarKindNumbers
)

func (c ideConfig) workspaceBarKind() (ret workspaceBarKind) {
	ret = workspaceBarKindNumbers
	if c.cfg == nil {
		return
	}
	cfg, ok := c.browser()
	if !ok {
		return
	}

	if barKindBool, err := cfg.GetBool("workspace_bar"); err == nil {
		if barKindBool {
			return workspaceBarKindPaths
		} else {
			return workspaceBarKindDisabled
		}
	}

	barKindStr, err := cfg.GetString("workspace_bar")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.workspace_bar"] = err
		}
		return
	}
	switch barKindStr {
	case "false":
		return workspaceBarKindDisabled
	case "number", "numbers":
		return workspaceBarKindNumbers
	case "path", "paths":
		return workspaceBarKindPaths
	default:
		c.errors["browser.workspace_bar"] = errors.New("expected either " +
			"false, 'number' or 'path'")
		return
	}
}

func (c ideConfig) windowFocusFrameAttr() (attr term.Attributes) {
	return c.windowAttr("focus_frame_attr",
		defaultWindowManagerConfig.FrameAttr)
}

func (c ideConfig) windowAttr(key string, def term.Attributes) (
	attr term.Attributes,
) {
	attr = def
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	cfgAttr, err := config.GetAttributes(cfg, key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("window_manager.%s", key)] = err
		}
		return
	}
	attr = cfgAttr
	return
}

func (c ideConfig) getConfigAttr(
	cfgKey, key string, def term.Attributes,
) (attr term.Attributes) {
	attr = def
	if c.cfg == nil {
		return
	}
	cfg, ok := c.getConfig(config.MapConfig(c.cfg), cfgKey)
	if !ok {
		return
	}
	cfgAttr, err := config.GetAttributes(cfg, key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("%s.%s", cfgKey, key)] = err
		}
		return
	}
	attr = cfgAttr
	return
}

func (c ideConfig) defaultAttr() (attr term.Attributes) {
	const key = "default_attr"
	cfgAttr, err := config.GetAttributes(config.MapConfig(c.cfg), key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[key] = err
		}
		return
	}
	attr = cfgAttr
	return
}

func (c ideConfig) getBrowserAttr(
	key string, def term.Attributes,
) (attr term.Attributes) {
	return c.getConfigAttr("browser", key, def)
}

func (c ideConfig) notifications() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(config.MapConfig(c.cfg), "notifications")
}

func (c ideConfig) notificationsCharset(def component.FrameCharSet) (
	cs component.FrameCharSet,
) {
	cs = def
	cfg, ok := c.notifications()
	if !ok {
		return
	}
	cfgCs, err := config.GetFrameCharset(cfg, "frame_charset", cs)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("notifications.%s", "frame_charset")] = err
		}
		return
	}
	cs = cfgCs
	return
}

func (c ideConfig) notificationsPadding(def int) (ret int) {
	ret = def
	cfg, ok := c.notifications()
	if !ok {
		return
	}
	i, err := cfg.GetInt("padding")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("notifications.%s", "padding")] = err
		}
		return
	}
	ret = i
	return
}

func (c ideConfig) notificationsProgressRunes(def notifications.ProgressRunes) (
	cs notifications.ProgressRunes,
) {
	cs = def
	cfg, ok := c.notifications()
	if !ok {
		return
	}
	cfgCs, err := getProgressRunes(cfg, cs)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("notifications.%s", "progress_format")] = err
		}
		return
	}
	cs = cfgCs

	return
}

func getProgressRunes(c config.Config, def notifications.ProgressRunes) (
	notifications.ProgressRunes, error,
) {
	cfg, err := c.GetConfig("progress_format")
	if err != nil {
		return def, err
	}

	ret := def
	r, err := cfg.GetRune("current")
	if err == nil {
		ret.Current = r
	}
	r, err = cfg.GetRune("remain")
	if err == nil {
		ret.Remain = r
	}
	r, err = cfg.GetRune("current_tip")
	if err == nil {
		ret.CurrentTip = r
	}
	r, err = cfg.GetRune("end")
	if err == nil {
		ret.End = r
	}
	r, err = cfg.GetRune("start")
	if err == nil {
		ret.Start = r
	}
	return ret, nil
}

func (c ideConfig) notificationsBool(key string, def bool) (ret bool) {
	ret = def
	cfg, ok := c.notifications()
	if !ok {
		return
	}
	cfgFrame, err := cfg.GetBool(key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("notifications.%s", key)] = err
		}
		return
	}
	ret = cfgFrame
	return
}

func (c ideConfig) notificationsDuration(key string, def time.Duration) (ret time.Duration) {
	ret = def
	cfg, ok := c.notifications()
	if !ok {
		return
	}
	cfgDur, err := config.GetDuration(cfg, key, def)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("notifications.%s", key)] = err
		}
		return
	}
	ret = cfgDur
	return
}

func (c ideConfig) notificationsConfig() notifications.Config {
	ret := defaultNotificationsConfig()
	ret.Attributes = c.getConfigAttr("notifications", "attr", ret.Attributes)
	ret.BackgroundAttributes = c.getConfigAttr("notifications", "background_attr", ret.BackgroundAttributes)
	ret.FrameCharSet = c.notificationsCharset(ret.FrameCharSet)
	ret.ProgressBar = c.notificationsBool("progress_bar", ret.ProgressBar)
	ret.AutoClose = c.notificationsDuration("auto_close", ret.AutoClose)
	ret.ProgressRunes = c.notificationsProgressRunes(ret.ProgressRunes)
	ret.Padding = c.notificationsPadding(ret.Padding)
	return ret
}

func (c ideConfig) focusTabAttr() term.Attributes {
	return c.getBrowserAttr("focus_tab_attr",
		browser.DefaultConfig().FocusTabAttr)
}

func (c ideConfig) nonFocusTabAttr() term.Attributes {
	return c.getBrowserAttr("non_focus_tab_attr",
		browser.DefaultConfig().NonFocusTabAttr)
}

func (c ideConfig) dirtyTabAttr() term.Attributes {
	return c.getBrowserAttr("dirty_tab_attr",
		text.DefaultConfig().DirtyTabAttr)
}

func (c ideConfig) icons() (ret text.IconSet) {
	ret = text.DefaultConfig().Icons
	e, ok := c.editor()
	if !ok {
		return
	}

	m, err := e.GetMap("icons")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.icons"] = err
		}
		return
	}

	ret.Default = c.getSpecialIcon(m, "default")
	ret.Terminal = c.getSpecialIcon(m, "terminal")

	for k, v := range m {
		vstr, ok := v.(string)
		if !ok {
			c.errors[fmt.Sprintf("editor.icons.%s", k)] =
				errors.New("expected a map of strings")
		} else if vstr != "" {
			ret.Extensions[k] = []rune(vstr)[0]
		}
	}
	return ret
}

func (c ideConfig) getSpecialIcon(m map[string]any, key string) (ret rune) {
	iconIfc, ok := m[key]
	if !ok {
		return
	}
	iconStr, ok := iconIfc.(string)
	if !ok {
		c.errors[fmt.Sprintf("editor.icons.%s", key)] =
			errors.New("expected a string")
	} else if iconStr != "" {
		ret = []rune(iconStr)[0]
	}
	delete(m, key)
	return
}

func (c ideConfig) windowFrameCharset() (cs component.FrameCharSet) {
	return c.windowCharset("frame_charset", defaultWindowManagerConfig.FrameCharSet)
}

func (c ideConfig) windowScrollBarAttr() (attr term.Attributes) {
	return c.windowAttr("scroll_bar_attr", defaultWindowManagerConfig.ScrollBarAttr)
}

func (c ideConfig) windowScrollBarChar() (ch rune) {
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	r, err := cfg.GetRune("scroll_bar_char")
	if err == nil {
		ch = r
	}
	return
}

func (c ideConfig) windowScrollBarHoverChar() (ch rune) {
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	r, err := cfg.GetRune("scroll_bar_hover_char")
	if err == nil {
		ch = r
	}
	return
}

func (c ideConfig) windowNoMaxSize() (ret bool) {
	ret = true
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	val, err := cfg.GetBool("no_max_size")
	if err == nil {
		ret = val
	}
	return
}

func (c ideConfig) windowFocusFrameCharset() (cs component.FrameCharSet) {
	return c.windowCharset("focus_frame_charset",
		defaultWindowManagerConfig.FocusFrameCharSet)
}

func (c ideConfig) windowCharset(key string, def component.FrameCharSet) (
	cs component.FrameCharSet,
) {
	cs = def
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	cfgCs, err := config.GetFrameCharset(cfg, key, cs)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("window_manager.%s", key)] = err
		}
		return
	}
	cs = cfgCs
	return
}

func (c ideConfig) windowManagerBool(key string, def bool) (frame bool) {
	frame = def
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	cfgFrame, err := cfg.GetBool(key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("window_manager.%s", key)] = err
		}
		return
	}
	frame = cfgFrame
	return
}

func (c ideConfig) frame() (frame bool) {
	return c.windowManagerBool("frame", defaultWindowManagerConfig.Frame)
}

func (c ideConfig) dim() (dim bool) {
	return c.windowManagerBool("dim", defaultWindowManagerConfig.Dim)
}

func (c ideConfig) frameUnion() (ret bool) {
	ret = c.frame()
	cfg, ok := c.browser()
	if !ok {
		return
	}

	unionFrames, err := cfg.GetBool("union_frames")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.union_frames"] = err
		}
	} else {
		ret = unionFrames
	}
	return
}

func (c ideConfig) frameUnionCharset() (cs component.FrameUnionCharSet) {
	cs = component.DefaultFrameUnionCharSet()
	cs.FrameCharSet = c.windowFrameCharset()
	cfg, ok := c.browser()
	if !ok {
		return
	}

	cfg, err := cfg.GetConfig("frameunion_charset")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.frameunion_charset"] = err
		}
		return
	}

	left, err := cfg.GetRune("left")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.frameunion_charset.left"] = err
		}
	} else {
		cs.Left = left
	}

	right, err := cfg.GetRune("right")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.frameunion_charset.right"] = err
		}
	} else {
		cs.Right = right
	}

	top, err := cfg.GetRune("top")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.frameunion_charset.top"] = err
		}
	} else {
		cs.Top = top
	}

	bottom, err := cfg.GetRune("bottom")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.frameunion_charset.bottom"] = err
		}
	} else {
		cs.Bottom = bottom
	}

	return
}

func (c ideConfig) windowManagerConfig() handler.WindowManagerConfig {
	return handler.WindowManagerConfig{
		Dim:                c.dim(),
		FocusFrameAttr:     c.windowFocusFrameAttr(),
		FocusFrameCharSet:  c.windowFocusFrameCharset(),
		ScrollBarHoverChar: c.windowScrollBarHoverChar(),
		WindowManagerConfig: component.WindowManagerConfig{
			NoMaxSize:     c.windowNoMaxSize(),
			Frame:         c.frame(),
			FrameAttr:     c.windowFrameAttr(),
			FrameCharSet:  c.windowFrameCharset(),
			ScrollBarAttr: c.windowScrollBarAttr(),
			ScrollBarChar: c.windowScrollBarChar(),
		},
	}
}

func (c ideConfig) browser() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(config.MapConfig(c.cfg), "browser")
}

func (c ideConfig) editor() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(config.MapConfig(c.cfg), "editor")
}

func (c ideConfig) modal() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	b, ok := c.editor()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "modal")
}

func (c ideConfig) modeless() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	b, ok := c.editor()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "modeless")
}

func (c ideConfig) virtual() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	b, ok := c.editor()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "virtual")
}

func (c ideConfig) editorMode() (ret string) {
	ret = "modal"
	if c.cfg == nil {
		return
	}
	e, ok := c.editor()
	if !ok {
		return
	}
	mode, err := e.GetString("mode")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.mode"] = err
		}
		return
	}
	switch mode {
	case editorModeModal, editorModeModeless:
		ret = mode
	}
	return
}

func (c ideConfig) virtualEditorAttr() (ret term.Attributes) {
	cfg, ok := c.virtual()
	if !ok {
		return
	}
	cfgAttr, err := config.GetAttributes(cfg, "attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.virtual.attr"] = err
		}
		return
	}
	ret = cfgAttr
	return
}

func (c ideConfig) virtualEditorSelectionAttr() (ret term.Attributes) {
	ret = term.Attributes{Attrs: tcell.AttrReverse}
	cfg, ok := c.virtual()
	if !ok {
		return
	}
	cfgAttr, err := config.GetAttributes(cfg, "selection_attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.virtual.selection_attr"] = err
		}
		return
	}
	ret = cfgAttr
	return
}

func (c ideConfig) virtualEditorShell() (ret string) {
	ret = os.Getenv("SHELL")
	if ret == "" {
		ret = "sh"
	}
	if c.cfg == nil {
		return
	}
	cfg, ok := c.virtual()
	if !ok {
		return
	}
	virtual, err := cfg.GetString("shell")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.virtual.shell"] = err
		}
		return
	}
	ret = virtual
	return
}

func (c ideConfig) virtualEditorEditor() (ret string) {
	if c.cfg == nil {
		return
	}
	cfg, ok := c.virtual()
	if !ok {
		return
	}
	virtual, err := cfg.GetString("editor")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.virtual.editor"] = err
		}
		return
	}
	ret = virtual
	return
}

func (c ideConfig) modalResultAttr() (attr term.Attributes) {
	attr = term.Attributes{Bg: tcell.ColorYellow, Fg: tcell.ColorBlack}
	cfg, ok := c.modal()
	if !ok {
		return
	}
	attr, err := config.GetAttributes(cfg, "search_attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modal.search_attr"] = err
		}
	}
	return attr
}

func (c ideConfig) modalAttr() (attr term.Attributes) {
	cfg, ok := c.modal()
	if !ok {
		return
	}
	attr, err := config.GetAttributes(cfg, "attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modal.attr"] = err
		}
	}
	return attr
}

func (c ideConfig) initialFolds() bool {
	cfg, ok := c.editor()
	if !ok {
		return false
	}
	enabled, err := cfg.GetBool("initial_folds")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.initial_folds"] = err
		}
	}
	return enabled
}

func (c ideConfig) auxiliaryBar() (config.Config, bool) {
	cfg, ok := c.editor()
	if !ok {
		return nil, false
	}
	cfg, err := cfg.GetConfig("aux_bar")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar"] = err
		}
		return nil, false
	}
	return cfg, true
}

func (c ideConfig) statusBar() (config.Config, bool) {
	cfg, ok := c.editor()
	if !ok {
		return nil, false
	}
	cfg, err := cfg.GetConfig("status_bar")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.status_bar"] = err
		}
		return nil, false
	}
	return cfg, true
}

func (c ideConfig) auxiliaryBarEnabled() bool {
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return false
	}
	enabled, err := cfg.GetBool("enabled")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar.enabled"] = err
		}
	}
	return enabled
}

func (c ideConfig) statusBarEnabled() bool {
	cfg, ok := c.statusBar()
	if !ok {
		return true
	}
	enabled, err := cfg.GetBool("enabled")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.status_bar.enabled"] = err
		}
		return true
	}
	return enabled
}

func (c ideConfig) auxiliaryBarConfig(
	pub text.EventPublisher, svc vctrl.Service,
) text.AuxBarConfig {
	auxBarLinesEnabled, auxBarLinesAbsolute := c.auxiliaryBarLines()
	return text.AuxBarConfig{
		GitEnabled:          c.auxiliaryBarGit(),
		LinesEnabled:        auxBarLinesEnabled,
		FoldsEnabled:        c.auxiliaryBarFolds(),
		AbsoluteLines:       auxBarLinesAbsolute,
		HighlightCursor:     c.auxiliaryBarHighlightCursor(),
		Service:             svc,
		Publisher:           pub,
		ScheduleNextTick:    c.scheduleNextTick,
		DelAttr:             c.auxiliaryBarAttr("git_del_inline_attr"),
		AddAttr:             c.auxiliaryBarAttr("git_add_inline_attr"),
		DelOverlayAttr:      c.auxiliaryBarAttr("git_del_locations_attr"),
		AddOverlayAttr:      c.auxiliaryBarAttr("git_add_locations_attr"),
		HighlightCursorAttr: c.auxiliaryBarAttr("highlight_cursor_attr"),
		LineNumberAttr:      c.auxiliaryBarAttr("line_number_attr"),
	}
}

func (c ideConfig) gitBarConfig(pub text.EventPublisher) text.GitBarConfig {
	return text.GitBarConfig{
		ScheduleNextTick: c.scheduleNextTick,
		Publisher:        pub,
		DelAttr:          c.auxiliaryBarAttr("git_del_inline_attr"),
		AddAttr:          c.auxiliaryBarAttr("git_add_inline_attr"),
		DelOverlayAttr:   c.auxiliaryBarAttr("git_del_locations_attr"),
		AddOverlayAttr:   c.auxiliaryBarAttr("git_add_locations_attr"),
	}
}

func (c ideConfig) statusBarConfig(
	cwd workspaceapi.URI, pub text.EventPublisher, svc vctrl.Service,
) text.StatusBarConfig {
	return text.StatusBarConfig{
		Workspace:        cwd,
		ScheduleNextTick: c.scheduleNextTick,
		Publisher:        pub,
		BackgroundColor:  c.statusBarAttr("background_attr", term.Attributes{}).Bg,
		ErrorColor:       c.statusBarAttr("foreground_error_attr", term.Attributes{}).Fg,
		GitService:       svc,
		Layout:           c.statusBarLayout(),
	}
}

func (c ideConfig) auxiliaryBarFolds() bool {
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return false
	}
	enabled, err := cfg.GetBool("folds")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar.folds"] = err
		}
	}
	return enabled
}

func (c ideConfig) auxiliaryBarAttr(key string) term.Attributes {
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return term.Attributes{}
	}
	attrs, err := config.GetAttributes(cfg, key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("editor.aux_bar.%s", key)] = err
		}
	}
	return attrs
}

func (c ideConfig) statusBarAttr(key string, def term.Attributes) (ret term.Attributes) {
	ret = def
	cfg, ok := c.statusBar()
	if !ok {
		return
	}
	attrs, err := config.GetAttributes(cfg, key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("editor.status_bar.%s", key)] = err
		}
	} else {
		ret = attrs
	}
	return
}

func (c ideConfig) statusBarLayout() (ret []text.StatusBarComponent) {
	ret = nil // nil delegates default config to StatusBar
	cfg, ok := c.statusBar()
	if !ok {
		return
	}
	val, err := cfg.GetString("layout")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.status_bar.layout"] = err
		}
		return
	}
	ret, err = text.ParseStatusBarLayout(val)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.status_bar.layout"] = err
		}
		return
	}
	return
}

func (c ideConfig) auxiliaryBarGit() bool {
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return false
	}
	enabled, err := cfg.GetString("git")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar.git"] = err
		}
	}
	return enabled == "all" || enabled == "inline"
}

func (c ideConfig) gitBarEnabled() bool {
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return false
	}
	enabled, err := cfg.GetString("git")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar.git"] = err
		}
	}
	return enabled == "all" || enabled == "bar"
}

func (c ideConfig) auxiliaryBarHighlightCursor() (ret bool) {
	ret = true
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return
	}
	enabled, err := cfg.GetBool("highlight_cursor")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar.highlight_cursor"] = err
		}
		return
	}
	return enabled
}

func (c ideConfig) auxiliaryBarLines() (bool, bool) {
	cfg, ok := c.auxiliaryBar()
	if !ok {
		return true, true
	}
	enabled, err := cfg.GetBool("lines")
	if err == nil {
		// default is absolute
		return enabled, true
	}
	lines, err := cfg.GetString("lines")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.aux_bar.lines"] = err
		}
		// defaults is absolute and enabled
		return true, true
	}
	switch lines {
	case "disabled":
		return false, false
	case "absolute":
		return true, true
	case "relative":
		return true, false
	default:
		return true, true
	}
}

func (c ideConfig) clipboard() clipboard.Register {
	cfg := config.MapConfig(c.cfg)
	ret, err := extutil.Clipboard(cfg)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["clipboard"] = err
		}
		ret = clipboard.NewInMemory()
	}
	return ret
}

func (c ideConfig) modelessResultAttr() (attr term.Attributes) {
	attr = term.Attributes{Bg: tcell.ColorYellow, Fg: tcell.ColorBlack}
	cfg, ok := c.modeless()
	if !ok {
		return
	}
	attr, err := config.GetAttributes(cfg, "search_attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modeless.search_attr"] = err
		}
	}
	return attr
}

func (c ideConfig) modelessAttr() (attr term.Attributes) {
	cfg, ok := c.modeless()
	if !ok {
		return
	}
	attr, err := config.GetAttributes(cfg, "attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modeless.attr"] = err
		}
	}
	return attr
}

func (c ideConfig) syntaxConfig() (ret syntax.Config) {
	ret = syntax.DefaultConfig()
	ret.ScheduleNextTick = c.scheduleNextTick
	cfg, ok := c.editor()
	if !ok {
		return
	}
	reparse, err := cfg.GetBool("reparse_syntax_errors")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.reparse_syntax_errors"] = err
		}
	} else {
		ret.ReparseOnErrors = reparse
	}
	strictErrors, err := cfg.GetBool("strict_errors")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.reparse_strict_errors"] = err
		}
	} else {
		ret.StrictErrors = strictErrors
	}
	autoindent, err := cfg.GetBool("autoindent")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.autoindent"] = err
		}
	} else {
		ret.Autoindent = autoindent
	}
	cfgHighlights, err := cfg.GetMap("highlights")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.highlights"] = err
		}
		return
	}
	cfg = config.MapConfig(cfgHighlights)
	for key := range cfgHighlights {
		cfgAttr, err := config.GetAttributes(cfg, key)
		if err != nil {
			c.errors["editor.highlights."+key] = err
		} else {
			ret.CaptureNamesAttributes[key] = cfgAttr
		}
	}
	return
}

func (c ideConfig) editorTabspaces() (tabs int) {
	tabs = text.DefaultConfig().Tabspaces
	cfg, ok := c.editor()
	if !ok {
		return
	}
	cfgTabs, err := cfg.GetInt("tabspaces")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.tabspaces"] = err
		}
		return
	}
	tabs = cfgTabs
	return
}

func (c ideConfig) wallpaper() (ret browser.Wallpaper) {
	backgroundAttr := c.workspaceWallpaperBackgroundAttr()

	ret = c.defaultWallpaper
	// allow user to override the default wallpaper's background
	ret.BackgroundAttr = backgroundAttr
	if c.cfg == nil {
		return
	}

	cfg := c.workspace()
	cfgImage, err := cfg.GetString("wallpaper_image")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["workspace.wallpaper_image"] = err
		}
		return c.wallpaperASCII(cfg, backgroundAttr)
	}

	densityChars, err := cfg.GetString("wallpaper_density_characters")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["workspace.wallpaper_density_characters"] = err
		}
		densityChars = " ▓▓▓▓"
	}

	img, err := openImage(cfgImage)
	if err != nil {
		c.errors["workspace.wallpaper_image"] = err
		return c.wallpaperASCII(cfg, backgroundAttr)
	}

	ret = makeWallpaper(img, densityChars)
	ret.BackgroundAttr = backgroundAttr
	return ret
}

func openImage(imagePath string) (image.Image, error) {
	// resolve image path
	imageURI, err := workspaceapi.CurrentUserHostURI(imagePath)
	if err != nil {
		return nil, fmt.Errorf("expand image %q: %w", imagePath, err)
	}
	f, err := os.Open(imageURI.Path())
	if err != nil {
		return nil, fmt.Errorf("open image %q: %w", imageURI.Path(), err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("png decode: %v", err)
	}
	return img, nil
}

func makeWallpaper(img image.Image, densityCharacters string) browser.Wallpaper {
	return browser.Wallpaper{
		NewComponent: func() tui.Component {
			cfg := asciiart.DefaultConfig()
			cfg.Color = true
			cfg.MaintainAspectRatio = true
			cfg.DensityCharacters = densityCharacters
			comp := asciiart.NewComponent(img, cfg)
			return component.NewSpan(comp, component.SpanConfig{
				PadHorizontalPerc: 0.4,
				PadVerticalPerc:   0.2,
				ContentAlignment:  component.SpanAlignmentCentered,
			})
		},
	}
}

func (c ideConfig) wallpaperASCII(
	cfg config.Config, backgroundAttr term.Attributes,
) (ret browser.Wallpaper) {
	ret = c.defaultWallpaper
	ret.BackgroundAttr = backgroundAttr
	if c.cfg == nil {
		return
	}
	cfgText, err := cfg.GetString("wallpaper")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["workspace.wallpaper"] = err
		}
		return
	}
	strcfg := component.StringConfig{
		Attributes:           c.workspaceWallpaperAttr(),
		BackgroundAttributes: c.workspaceWallpaperBackgroundAttr(),
		Alignment:            component.SpanAlignmentCentered,
	}
	ret.NewComponent = func() tui.Component {
		return component.NewStringWithConfig(cfgText, strcfg)
	}
	return
}

func (c ideConfig) autoRestore() (ret bool) {
	ret = true
	if c.cfg == nil {
		return
	}
	cfg := c.workspace()
	cfgBool, err := cfg.GetBool("auto_restore")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["workspace.auto_restore"] = err
		}
		return
	}
	ret = cfgBool
	return
}

func (c ideConfig) logOutputPath() string {
	if c.cfg == nil {
		return ""

	}
	path, err := config.MapConfig(c.cfg).GetString("log_path")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["log_path"] = err
		}
		return ""
	}
	return path
}

func (c ideConfig) logLevel() log.Level {
	if c.cfg == nil {
		return log.ErrorLevel

	}
	levelStr, err := config.MapConfig(c.cfg).GetString("log_level")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["log_level"] = err
		}
		return log.ErrorLevel
	}

	level, err := log.ParseLevel(levelStr)
	if err != nil {
		c.errors["log_level"] = err
		return log.ErrorLevel
	}
	return level
}

func (c ideConfig) inputMode() term.InputMode {
	inputModeIfc, ok := c.cfg["input_mode"]
	if !ok {
		return term.InputCurrent
	}

	inputModeSlice, ok := inputModeIfc.([]interface{})
	if !ok {
		inputModeStr, ok := inputModeIfc.(string)
		if !ok {
			c.errors["input_mode"] = errors.New("invalid type")
			return term.InputCurrent
		}

		inputModeSlice = []interface{}{inputModeStr}
	}

	var ret term.InputMode
	for _, inputMode := range inputModeSlice {
		switch inputMode {
		case inputEsc:
			ret |= term.InputEsc
		case inputAlt:
			ret |= term.InputAlt
		case inputMouse:
			ret |= term.InputMouse
		case inputCurrent:
			return term.InputCurrent
		default:
			c.errors["input_mode"] = fmt.Errorf("unknown input mode: %s", inputMode)
		}
	}

	return ret
}

func (c ideConfig) extensions() map[string]extensionConfig {
	pConfigIfc, ok := c.cfg["extensions"]
	if !ok {
		return nil
	}
	pConfigMap, ok := pConfigIfc.(map[string]interface{})
	if !ok {
		c.errors["extensions"] = errors.New("invalid type")
		return nil
	}

	ret := make(map[string]extensionConfig)
	for id, pConfig := range pConfigMap {
		pcfg, ok := pConfig.(map[string]interface{})
		if !ok {
			c.errors["extensions."+id] = errors.New("invalid type")
			continue
		}
		ret[id] = extensionConfig{
			id:     id,
			parent: &c,
			cfg:    config.MapConfig(pcfg),
		}
	}

	return ret
}

func (c extensionConfig) path() (string, bool) {
	path, err := c.cfg.GetString("path")
	if err != nil {
		return "", false
	}
	return path, true
}

func (c extensionConfig) config() (config.Config, bool) {
	cfg, err := c.cfg.GetConfig("config")
	if err != nil {
		if err != config.ErrNotFound {
			errorID := fmt.Sprintf("extension.%s.config", c.id)
			c.parent.errors[errorID] = err
		}
		return nil, false
	}
	return cfg, true
}

func (c ideConfig) workspace() config.Config {
	if c.cfg == nil {
		return config.NopConfig()
	}
	cfg, ok := c.getConfig(config.MapConfig(c.cfg), "workspace")
	if ok {
		return cfg
	}
	return config.NopConfig()
}

func (c ideConfig) workspaceWallpaperAttr() term.Attributes {
	return c.getConfigAttr("workspace", "wallpaper_attr",
		term.Attributes{})
}

func (c ideConfig) workspaceWallpaperBackgroundAttr() term.Attributes {
	return c.getConfigAttr("workspace", "wallpaper_background_attr",
		term.Attributes{})
}

func (c ideConfig) terminalDefaultAttr() term.Attributes {
	return c.getConfigAttr("terminal", "attr", term.Attributes{})
}

func (c ideConfig) terminalSelectionAttr() term.Attributes {
	return c.getConfigAttr("terminal", "selection_attr",
		term.Attributes{Attrs: tcell.AttrReverse})
}

func (c ideConfig) terminalNeedsAttentionAttr() term.Attributes {
	return c.getConfigAttr("terminal", "needs_attention_attr",
		term.Attributes{Attrs: tcell.AttrBlink})
}

func (c ideConfig) terminalDynamicTabName() bool {
	return c.terminalBool("dynamic_tab_name")
}

func (c ideConfig) terminal() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	cfg, ok := c.getConfig(config.MapConfig(c.cfg), "terminal")
	return cfg, ok
}

func (c ideConfig) terminalShell() (ret []string) {
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	str, err := cfg.GetString("shell")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.shell"] = err
		}
	}
	return strings.Split(str, " ")
}

func (c ideConfig) terminalMaxLines() (ret int) {
	ret = vte.DefaultConfig().MaxLines
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	ret, err := cfg.GetInt("max_lines")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.max_lines"] = err
		}
	}
	return ret
}

func (c ideConfig) terminalBellTrigger() (ret []byte) {
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	retStr, err := cfg.GetString("bell_trigger")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.bell_trigger"] = err
		}
	} else {
		ret = []byte(retStr)
	}
	return
}

func (c ideConfig) initialTerminalCapacity() (ret int) {
	ret = 1
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	res, err := cfg.GetInt("initial_reservoir")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.initial_reservoir"] = err
		}
		return
	}
	return res
}
func (c ideConfig) terminalBool(key string) (ret bool) {
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	ret, err := cfg.GetBool(key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("terminal.%s", key)] = err
		}
	}
	return ret
}

func (c ideConfig) terminalModal() (ret bool) {
	return c.terminalBool("modal")
}

func (c ideConfig) terminalDebug() (ret bool) {
	return c.terminalBool("debug")
}

func (c ideConfig) tabNameSeparator() (ret string) {
	ret = "  "
	cfg, ok := c.browser()
	if !ok {
		return
	}
	sep, err := cfg.GetString("tab_name_separator")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.tab_name_separator"] = err
		}
		return
	}
	ret = sep
	return
}

// this is an internal optimization, no need to expose it
const defaultMinWidth = 30

func (c ideConfig) terminalConfig() vte.Config {
	ret := vte.DefaultConfig()
	ret.Attributes = c.terminalDefaultAttr()
	ret.SelectionAttributes = c.terminalSelectionAttr()
	ret.NeedsAttentionAttributes = c.terminalNeedsAttentionAttr()
	ret.DynamicTabName = c.terminalDynamicTabName()
	ret.MaxLines = c.terminalMaxLines()
	ret.Bell = c.terminalBellTrigger()
	ret.Modal = c.terminalModal()
	ret.Debug = c.terminalDebug()
	ret.CommandAndArgs = c.terminalShell()
	ret.Clipboard = c.clipboard()
	ret.ScheduleNextTick = c.scheduleNextTick
	ret.RingBell = c.ringBell
	ret.MinWidth = defaultMinWidth
	ret.ZdotDir = c.zdotDir
	return ret
}

func (c ideConfig) pluginBarBackgroundColor(def tcell.Color) (bg tcell.Color) {
	return c.pluginAttr(term.Attributes{Bg: def}, "bar_background_attr").Bg
}

func (c ideConfig) pluginStatusSuccessColor(def tcell.Color) (fg tcell.Color) {
	return c.pluginAttr(term.Attributes{Fg: def}, "status_success_attr").Fg
}

func (c ideConfig) pluginStatusErrorColor(def tcell.Color) (fg tcell.Color) {
	return c.pluginAttr(term.Attributes{Fg: def}, "status_error_attr").Fg
}

func (c ideConfig) pluginAttr(def term.Attributes, key string) (bg term.Attributes) {
	bg = def
	if c.cfg == nil {
		return
	}
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	cfg, ok = c.getConfig(cfg, "plugin")
	if !ok {
		return
	}
	cfgAttr, err := config.GetAttributes(cfg, key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("terminal.plugin.%s", key)] = err
		}
		return
	}
	bg = cfgAttr
	return
}

func (c ideConfig) pluginString(def, key string) (ret string) {
	ret = def
	if c.cfg == nil {
		return
	}
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	cfg, ok = c.getConfig(cfg, "plugin")
	if !ok {
		return
	}
	str, err := cfg.GetString(key)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors[fmt.Sprintf("terminal.plugin.%s", key)] = err
		}
		return
	}
	ret = str
	return
}

func (c ideConfig) pluginAnimationFrames(def []string) (ret []string) {
	ret = def
	str := c.pluginString("", "animation")
	if str == "" {
		return
	}
	ret = strings.Split(str, "")
	return
}

func (c ideConfig) pluginStatusErrorIcon(def string) (ret string) {
	ret = def
	str := c.pluginString(def, "status_error_icon")
	if str == "" {
		return
	}
	ret = str
	return
}

func (c ideConfig) pluginStatusSuccessIcon(def string) (ret string) {
	ret = def
	str := c.pluginString(def, "status_success_icon")
	if str == "" {
		return
	}
	ret = str
	return
}

func (c ideConfig) pluginBarLayout(def []plugin.BarComponent) (ret []plugin.BarComponent) {
	ret = def
	layoutStr := c.pluginString("", "bar_layout")
	if layoutStr == "" {
		return
	}
	layout, err := plugin.ParseBarLayout(layoutStr)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.plugin.bar_layout"] = err
		}
		return
	}
	ret = layout
	return
}

func (c ideConfig) pluginAlignBottom(def bool) (ret bool) {
	ret = def
	if c.cfg == nil {
		return
	}
	cfg, ok := c.terminal()
	if !ok {
		return
	}
	cfg, ok = c.getConfig(cfg, "plugin")
	if !ok {
		return
	}
	do, err := cfg.GetBool("bar_align_bottom")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.plugin.bar_align_bottom"] = err
		}
		return
	}
	ret = do
	return
}

func (c ideConfig) pluginBarConfig() plugin.BarConfig {
	ret := plugin.DefaultBarConfig()
	ret.BackgroundColor = c.pluginBarBackgroundColor(ret.BackgroundColor)
	ret.StatusAnimationFrames = c.pluginAnimationFrames(ret.StatusAnimationFrames)
	ret.StatusErrorIcon = c.pluginStatusErrorIcon(ret.StatusErrorIcon)
	ret.StatusErrorColor = c.pluginStatusErrorColor(ret.StatusErrorColor)
	ret.StatusSuccessIcon = c.pluginStatusSuccessIcon(ret.StatusSuccessIcon)
	ret.StatusSuccessColor = c.pluginStatusSuccessColor(ret.StatusSuccessColor)
	ret.AlignBottom = c.pluginAlignBottom(ret.AlignBottom)
	ret.Layout = c.pluginBarLayout(ret.Layout)
	return ret
}

func decodeConfig(r io.Reader) (cfg map[string]interface{}, err error) {
	d := yaml.NewDecoder(r)

	cfg = make(map[string]interface{})
	err = d.Decode(&cfg)
	return
}

func reloadConfig(
	configFilePath string, defaultWallpaper browser.Wallpaper,
	defaultConfig string, ringBell func(), scheduleNextTick func(func()) bool,
	zdotDir string,
) (ret ideConfig, err error) {
	err = loadConfig(&ret, configFilePath,
		defaultWallpaper, defaultConfig, ringBell,
		scheduleNextTick, zdotDir)
	return
}

func loadWorkspaceConfig(
	filename string, cwd workspace.Workspace, uri workspaceapi.URI, c *ideConfig,
) (isConfigErr bool, err error) {
	f, err := cwd.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if errors.Is(err, os.ErrPermission) {
			return false, nil
		}
		return false, fmt.Errorf("failed to open local '%s': %v", filename, err)
	}
	defer f.Close()

	cfg, err := decodeConfig(f)
	if err != nil {
		return true, err
	}

	err = validateConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("validate config: %w", err)
	}

	overrideConfig(c.cfg, cfg)
	return false, nil
}

// NOTE: it's imperative that this function populates c with sane defaults even in the event
// of an error.
func loadConfig(
	c *ideConfig, configpath string,
	defaultWallpaper browser.Wallpaper,
	defaultConfig string,
	ringBell func(), scheduleNextTick func(func()) bool,
	zdotDir string,
) (err error) {
	initDefaultConfig(c, defaultWallpaper, ringBell, scheduleNextTick,
		zdotDir)

	cfg, err := decodeConfig(strings.NewReader(defaultConfig))
	if err != nil {
		panic(err)
	}

	initConfig(c, cfg, defaultWallpaper, ringBell,
		scheduleNextTick, zdotDir)

	if err := loadFileConfig(c, configpath); err != nil {
		return err
	}

	err = validateConfig(c.cfg)
	if err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	return nil
}

func loadFileConfig(c *ideConfig, configpath string) (err error) {
	f, err := workspace.OpenFile(configpath, os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	cfg, err := decodeConfig(f)
	if err != nil {
		return err
	}

	overrideConfig(c.cfg, cfg)

	return nil
}

func filepathCompleter(c *text.Component) command.Completer {
	return command.FilePathCompleter(c.Workspace())
}

func defaultNotificationsConfig() notifications.Config {
	return notifications.Config{
		AutoClose:            5 * time.Second,
		ProgressBar:          true,
		Width:                50,
		Attributes:           term.Attributes{},
		BackgroundAttributes: term.Attributes{},
		FrameCharSet:         component.FrameCharSetDefault(),
		Interrupter:          term.NopInterrupter(),
	}
}
