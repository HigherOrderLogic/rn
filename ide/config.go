package ide

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/tcell/v3"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v3"
	"unstable.build/go-tui/api/config"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"
	extutil "unstable.build/go-tui/extension/util"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
	"unstable.build/go-tui/workspace"
)

const (
	inputEsc               = "esc"
	inputAlt               = "alt"
	inputMouse             = "mouse"
	inputCurrent           = "current"
	legacyDefaultWallpaper = `
         __       
        /\ \      
       /  \ \     
      / /\ \_\    
     / / /\/_/    
    / /_/_        
   / /___/\       
  / /\__ \ \      
 / / /__\ \ \     
/ / /____\ \ \    
\/__________\/    `

	editorModeModal    = "modal"
	editorModeModeless = "modeless"
	keyCommandAliases  = "aliases"
)

//go:embed sixrc
var defaultConfig string

var (
	defaultWindowManagerConfig = handler.DefaultWindowManagerConfig()
)

type extensionConfig struct {
	id     string
	parent *ideConfig
	cfg    config.Config
}

type ideConfig struct {
	defaultWallpaper string

	cfg    map[string]interface{}
	errors map[string]error
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

func initConfig(c *ideConfig, cfg map[string]interface{}, defaultWallpaper string) {
	c.cfg = cfg
	c.defaultWallpaper = defaultWallpaper
	c.errors = make(map[string]error)
}

func initDefaultConfig(c *ideConfig, defaultWallpaper string) {
	cfg := make(map[string]interface{})
	initConfig(c, cfg, defaultWallpaper)
}

func (c ideConfig) command() (config.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(config.MapConfig(c.cfg), "command")
}

func (c ideConfig) commandKeyMappings() map[handler.Sequence][]string {
	ret := make(map[handler.Sequence][]string)
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
		cmdAndArgsSliceIfc, ok := v.([]interface{})
		if ok {
			var cmdAndArgs []string
			for _, ifc := range cmdAndArgsSliceIfc {
				cmdOrArg, ok := ifc.(string)
				if !ok {
					c.errors["key_bindings."+k] = errors.New(
						"expected space-separated multi-word " +
							"string or []string but found unknown type")
					cmdAndArgs = nil
					break
				}
				cmdAndArgs = append(cmdAndArgs, cmdOrArg)
			}
			if len(cmdAndArgs) != 0 {
				ret[seq] = cmdAndArgs
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
			ret[seq] = parts
		}
	}

	return ret
}

func (c ideConfig) commandKey() (ret term.KeyComb) {
	ret = defaultCommandKey
	cfg, ok := c.command()
	if !ok {
		return
	}
	cfgKey, err := cfg.GetString("key")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["command.key"] = err
		}
		return
	}
	key, err := term.ParseKey(cfgKey)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["command.key"] = err
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

func (c ideConfig) commandAliases() (ret map[string][]string) {
	ret = make(map[string][]string)
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
		switch v.(type) {
		case string:
			ret[k] = make([]string, 1)
			ret[k][0] = v.(string)
		case []interface{}:
			ret[k] = make([]string, 0)
			for _, v := range v.([]interface{}) {
				switch v.(type) {
				case string:
					ret[k] = append(ret[k], v.(string))
				default:
					err = multierr.Append(err,
						fmt.Errorf("invalid value type for command.%s.%s", keyCommandAliases, k))
				}
			}
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

func (c ideConfig) workspacePath() (ret bool) {
	if c.cfg == nil {
		return
	}
	cfg, ok := c.browser()
	if !ok {
		return
	}
	cfgBarUri, err := cfg.GetBool("workspace_bar_uri")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.workspace_bar_uri"] = err
		}
		return
	}
	ret = cfgBarUri
	return
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
	ret := browser.DefaultConfig().Notifications
	ret.Attributes = c.getConfigAttr("notifications", "attr", ret.Attributes)
	ret.BackgroundAttributes = c.getConfigAttr("notifications", "background_attr", ret.BackgroundAttributes)
	ret.FrameCharSet = c.notificationsCharset(ret.FrameCharSet)
	ret.ProgressBar = c.notificationsBool("progress_bar", ret.ProgressBar)
	ret.AutoClose = c.notificationsDuration("auto_close", ret.AutoClose)
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

func (c ideConfig) windowFrameCharset() (cs component.FrameCharSet) {
	return c.windowCharset("frame_charset", defaultWindowManagerConfig.FrameCharSet)
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

func (c ideConfig) frameUnionCharset() (cs component.FrameUnionCharSet) {
	cs = component.DefaultFrameUnionCharSet()
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
		Dim:               c.dim(),
		FocusFrameAttr:    c.windowFocusFrameAttr(),
		FocusFrameCharSet: c.windowFocusFrameCharset(),
		WindowManagerConfig: component.WindowManagerConfig{
			Frame:        c.frame(),
			FrameAttr:    c.windowFrameAttr(),
			FrameCharSet: c.windowFrameCharset(),
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

func (c ideConfig) modalBarAttr() (attr term.Attributes) {
	attr = term.Attributes{}
	cfg, ok := c.modal()
	if !ok {
		return
	}
	attr, err := config.GetAttributes(cfg, "bar_attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modal.bar_attr"] = err
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

func (c ideConfig) modalDebug() (ret bool) {
	return c.modalBool("debug")
}

func (c ideConfig) modalWrap() (ret bool) {
	return c.modalBool("wrap")
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

func (c ideConfig) modalBool(name string) (ret bool) {
	cfg, ok := c.modal()
	if !ok {
		return
	}
	ret, err := cfg.GetBool(name)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modal."+name] = err
		}
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

func (c ideConfig) modelessBarAttr() (attr term.Attributes) {
	attr = term.Attributes{}
	cfg, ok := c.modeless()
	if !ok {
		return
	}
	attr, err := config.GetAttributes(cfg, "bar_attr")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modeless.bar_attr"] = err
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

func (c ideConfig) modelessWrap() (ret bool) {
	return c.modelessBool("wrap")
}

func (c ideConfig) modelessBool(name string) (ret bool) {
	cfg, ok := c.modeless()
	if !ok {
		return
	}
	ret, err := cfg.GetBool(name)
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["editor.modeless."+name] = err
		}
	}
	return ret
}

func (c ideConfig) browserTabspaces() (tabs int) {
	tabs = text.DefaultConfig().Tabspaces
	cfg, ok := c.browser()
	if !ok {
		return
	}
	cfgTabs, err := cfg.GetInt("tabspaces")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["browser.tabspaces"] = err
		}
		return
	}
	tabs = cfgTabs
	return
}

func (c ideConfig) wallpaper() (text string) {
	text = c.defaultWallpaper
	if c.cfg == nil {
		return
	}
	cfg := c.workspace()
	cfgText, err := cfg.GetString("wallpaper")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["workspace.wallpaper"] = err
		}
		return
	}
	text = cfgText
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
		browser.DefaultConfig().WallpaperAttr)
}

func (c ideConfig) workspaceWallpaperBackgroundAttr() term.Attributes {
	return c.getConfigAttr("workspace", "wallpaper_background_attr",
		browser.DefaultConfig().WallpaperBackgroundAttr)
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

func (c ideConfig) terminalShell() (ret string) {
	if c.cfg == nil {
		return
	}
	cfg, ok := c.getConfig(config.MapConfig(c.cfg), "terminal")
	if !ok {
		return
	}
	ret, err := cfg.GetString("shell")
	if err != nil {
		if err != config.ErrNotFound {
			c.errors["terminal.shell"] = err
		}
	}
	return ret
}

func (c ideConfig) terminalConfig() vte.Config {
	ret := vte.DefaultConfig()
	ret.Attributes = c.terminalDefaultAttr()
	ret.SelectionAttributes = c.terminalSelectionAttr()
	ret.NeedsAttentionAttributes = c.terminalNeedsAttentionAttr()
	ret.Modal = c.editorMode() == editorModeModal
	ret.Shell = c.terminalShell()
	ret.Clipboard = c.clipboard()
	if log.IsLevelEnabled(log.TraceLevel) {
		ret.ScheduleBell = func() {
			log.Trace("schedule bell")
			term.PublishBell()
		}
		ret.RingBell = func() {
			log.Trace("ring bell")
			term.RingBell()
		}
	} else {
		ret.ScheduleBell = term.PublishBell
		ret.RingBell = term.RingBell
	}
	return ret
}

func decodeConfig(r io.Reader) (cfg map[string]interface{}, err error) {
	d := yaml.NewDecoder(r)

	cfg = make(map[string]interface{})
	err = d.Decode(&cfg)
	return
}

func reloadConfig(configFilePath string, defaultWallpaper string) (ret ideConfig, err error) {
	err = loadConfig(&ret, configFilePath, defaultWallpaper)
	return
}

func loadWorkspaceConfig(filename string, cwd workspace.Workspace, uri workspaceapi.URI, c *ideConfig) (
	isConfigErr bool, err error,
) {
	f, werr := cwd.Open(filename, os.O_RDONLY, 0)
	if werr != nil {
		if werr.IsNotExist {
			return false, nil
		}
		if werr.IsPermission {
			return false, nil
		}
		return false, fmt.Errorf("failed to open local '%s': %v", filename, werr.ToError())
	}
	defer f.Close()

	cfg, err := decodeConfig(f)
	if err != nil {
		return true, err
	}

	overrideConfig(c.cfg, cfg)
	return false, nil
}

// NOTE: it's imperative that this function populates c with sane defaults even in the event
// of an error.
func loadConfig(c *ideConfig, configpath, defaultWallpaper string) (err error) {
	initDefaultConfig(c, defaultWallpaper)

	cfg, err := decodeConfig(strings.NewReader(defaultConfig))
	if err != nil {
		panic(err)
	}

	initConfig(c, cfg, defaultWallpaper)

	if err := loadFileConfig(c, configpath); err != nil {
		return err
	}

	err = text.ValidateCommandAliases(c.commandAliases())
	if err != nil {
		// void aliases but keep the rest of config intact.
		// this ensures that text.NewComponent doesn't hard error,
		// preventing user from editing using this same IDE.
		_, ok := c.cfg["command"]
		if !ok {
			panic("empty command config but detected invalid aliases")
		}
		cfg["command"].(map[string]interface{})[keyCommandAliases] = map[string]interface{}{}
		err = fmt.Errorf("'command.%s' is invalid: %w", keyCommandAliases, err)
	}
	return err
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
