package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v3"
)

const (
	outputNormal           = "normal"
	output256              = "color_256"
	outputGrayscale        = "grayscale"
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
	defaultWallpaper = `
███████╗██╗██╗ ██╗
██╔════╝██║██████║
███████╗██║╚═██╔═╝
╚════██║██║██████╗
███████║██║██╔═██║
╚══════╝╚═╝╚═╝ ╚═╝`
)

var (
	defaultWindowManagerConfig = handler.DefaultWindowManagerConfig()
	defSSHTimeout              = 10 * time.Second
)

type pluginConfig struct {
	id     string
	parent *ideConfig
	cfg    plugin.Config
}

type ideConfig struct {
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

func initConfig(c *ideConfig, cfg map[string]interface{}) {
	c.cfg = cfg
	c.errors = make(map[string]error)
}

func initDefaultConfig(c *ideConfig) {
	cfg := make(map[string]interface{})
	initConfig(c, cfg)
}

func (c ideConfig) command() (plugin.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(plugin.MapConfig(c.cfg), "command")
}

func (c ideConfig) commandKeyMappings() map[handler.Sequence]string {
	ret := make(map[handler.Sequence]string)
	cfg, ok := c.command()
	if !ok {
		return ret
	}
	m, err := cfg.GetMap("key_bindings")
	if err != nil {
		if err != plugin.ErrNotFound {
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
		strValue, ok := v.(string)
		if !ok {
			c.errors["key_bindings."+k] = errors.New("expected string found unknown type")
			continue
		}
		ret[seq] = strValue
	}

	return ret
}

func (c ideConfig) commandOverlayFrame() (ret bool) {
	ret = text.DefaultCommandOverlayConfig().Frame
	cfg, ok := c.command()
	if !ok {
		return
	}
	cfgFrame, err := cfg.GetBool("frame")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["command.frame"] = err
		}
		return
	}
	ret = cfgFrame
	return
}

func (c ideConfig) commandKey() (ret term.KeyComb) {
	ret = defaultCommandKey
	cfg, ok := c.command()
	if !ok {
		return
	}
	cfgKey, err := cfg.GetString("key")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["command.key"] = err
		}
		return
	}
	key, err := term.ParseKey(cfgKey)
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["command.key"] = err
		}
		return
	}
	ret = key
	return
}

func (c ideConfig) commandOverlayWidth() (ret int) {
	ret = text.DefaultCommandOverlayConfig().Width
	cfg, ok := c.command()
	if !ok {
		return
	}
	width, err := cfg.GetInt("width")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["command.width"] = err
		}
		return
	}
	ret = width
	return
}

func (c ideConfig) commandOverlayHeight() (ret int) {
	ret = text.DefaultCommandOverlayConfig().Height
	cfg, ok := c.command()
	if !ok {
		return
	}
	height, err := cfg.GetInt("height")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["command.height"] = err
		}
		return
	}
	ret = height
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
		if err != plugin.ErrNotFound {
			c.errors["command.max_history"] = err
		}
		return
	}
	ret = height
	return
}

func (c ideConfig) prompt() (plugin.Config, bool) {
	b, ok := c.browser()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "prompt")
}

func (c ideConfig) promptWidth() (ret int) {
	ret = browser.DefaultConfig().PromptConfig.Width
	cfg, ok := c.prompt()
	if !ok {
		return
	}
	width, err := cfg.GetInt("width")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["prompt.width"] = err
		}
		return
	}
	ret = width
	return
}

func (c ideConfig) promptHeight() (ret int) {
	ret = browser.DefaultConfig().PromptConfig.Height
	cfg, ok := c.prompt()
	if !ok {
		return
	}
	height, err := cfg.GetInt("height")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["prompt.height"] = err
		}
		return
	}
	ret = height
	return
}

func (c ideConfig) getCfgAttr(
	key string, def term.Attributes,
	cfgKey string,
	cfgFn func() (plugin.Config, bool),
) (attr term.Attributes) {
	attr = def
	cfg, ok := cfgFn()
	if !ok {
		return
	}
	cfgAttr, err := plugin.GetAttributes(cfg, key)
	if err != nil {
		if err != plugin.ErrNotFound {
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

func (c ideConfig) commandOverlayMatchedTextAttr() (ret term.Attributes) {
	return c.getCommandAttr("matched_text_attr",
		text.DefaultCommandOverlayConfig().MatchedTextAttr)
}

func (c ideConfig) commandOverlayCountAttr() (ret term.Attributes) {
	return c.getCommandAttr("count_attr", text.DefaultCommandOverlayConfig().CountAttr)
}

func (c ideConfig) commandOverlayFocusElementAttr() (ret term.Attributes) {
	return c.getCommandAttr("focus_element_attr", text.DefaultCommandOverlayConfig().FocusElementAttr)
}

func (c ideConfig) commandOverlayElementAttr() (ret term.Attributes) {
	return c.getCommandAttr("element_attr", text.DefaultCommandOverlayConfig().ElementAttr)
}

func (c ideConfig) commandOverlayConfig() text.CommandOverlayConfig {
	return text.CommandOverlayConfig{
		Frame:            c.commandOverlayFrame(),
		Width:            c.commandOverlayWidth(),
		Height:           c.commandOverlayHeight(),
		MatchedTextAttr:  c.commandOverlayMatchedTextAttr(),
		CountAttr:        c.commandOverlayCountAttr(),
		FocusElementAttr: c.commandOverlayFocusElementAttr(),
		ElementAttr:      c.commandOverlayElementAttr(),
	}
}

func (c ideConfig) promptConfig() browser.PromptConfig {
	return browser.PromptConfig{
		Width:         c.promptWidth(),
		Height:        c.promptHeight(),
		TextAttr:      c.promptTextAttr(),
		HighlightAttr: c.promptHighlightAttr(),
	}
}

func (c ideConfig) getConfig(cfg plugin.Config, key string) (plugin.Config, bool) {
	cfg, err := cfg.GetConfig(key)
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors[key] = err
		}
		return nil, false
	}
	return cfg, true
}

func (c ideConfig) windowManager() (plugin.Config, bool) {
	b, ok := c.browser()
	if !ok {
		return nil, false
	}
	return c.getConfig(b, "window_manager")
}

func (c ideConfig) windowFrameAttr() (attr term.Attributes) {
	attr = defaultWindowManagerConfig.FrameAttr
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	cfgAttr, err := plugin.GetAttributes(cfg, "frame_attr")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["window_manager.frame_attr"] = err
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
	cfg, ok := c.getConfig(plugin.MapConfig(c.cfg), cfgKey)
	if !ok {
		return
	}
	cfgAttr, err := plugin.GetAttributes(cfg, key)
	if err != nil {
		if err != plugin.ErrNotFound {
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

func (c ideConfig) messageBarAttr() term.Attributes {
	return c.getBrowserAttr("message_bar_attr",
		browser.DefaultConfig().MessageBarAttr)
}

func (c ideConfig) focusTabAttr() term.Attributes {
	return c.getBrowserAttr("focus_tab_attr",
		browser.DefaultConfig().FocusTabAttr)
}

func (c ideConfig) nonFocusTabAttr() term.Attributes {
	return c.getBrowserAttr("non_focus_tab_attr",
		browser.DefaultConfig().NonFocusTabAttr)
}

func (c ideConfig) browserWallpaperAttr() term.Attributes {
	return c.getBrowserAttr("wallpaper_attr",
		browser.DefaultConfig().WallpaperAttr)
}

func (c ideConfig) browserWallpaperBackgroundAttr() term.Attributes {
	return c.getBrowserAttr("wallpaper_background_attr",
		browser.DefaultConfig().WallpaperBackgroundAttr)
}

func (c ideConfig) dirtyTabAttr() term.Attributes {
	return c.getBrowserAttr("dirty_tab_attr",
		text.DefaultConfig().DirtyTabAttr)
}

func (c ideConfig) windowFrameCharset() (cs component.FrameCharSet) {
	cs = defaultWindowManagerConfig.FrameCharSet
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	cfgCs, err := plugin.GetFrameCharset(cfg, "frame_charset", cs)
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["window_manager.frame_charset"] = err
		}
		return
	}
	cs = cfgCs
	return
}

func (c ideConfig) frame() (frame bool) {
	frame = defaultWindowManagerConfig.Frame
	cfg, ok := c.windowManager()
	if !ok {
		return
	}
	cfgFrame, err := cfg.GetBool("frame")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["window_manager.frame"] = err
		}
		return
	}
	frame = cfgFrame
	return
}

func (c ideConfig) frameUnionCharset() (cs component.FrameUnionCharSet) {
	cs = component.DefaultFrameUnionCharSet()
	cfg, ok := c.browser()
	if !ok {
		return
	}

	cfg, err := cfg.GetConfig("frameunion_charset")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["browser.frameunion_charset"] = err
		}
		return
	}

	left, err := cfg.GetRune("left")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["browser.frameunion_charset.left"] = err
		}
	} else {
		cs.Left = left
	}

	right, err := cfg.GetRune("right")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["browser.frameunion_charset.right"] = err
		}
	} else {
		cs.Right = right
	}

	top, err := cfg.GetRune("top")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["browser.frameunion_charset.top"] = err
		}
	} else {
		cs.Top = top
	}

	bottom, err := cfg.GetRune("bottom")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["browser.frameunion_charset.bottom"] = err
		}
	} else {
		cs.Bottom = bottom
	}

	return
}

func (c ideConfig) windowManagerConfig() component.WindowManagerConfig {
	return component.WindowManagerConfig{
		Frame:        c.frame(),
		FrameAttr:    c.windowFrameAttr(),
		FrameCharSet: c.windowFrameCharset(),
	}
}

func (c ideConfig) browser() (plugin.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(plugin.MapConfig(c.cfg), "browser")
}

func (c ideConfig) vi() (plugin.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(plugin.MapConfig(c.cfg), "vi")
}

func (c ideConfig) viResultAttr() (attr term.Attributes) {
	attr = term.Attributes{Bg: term.ColorYellow, Fg: term.ColorBlack}
	cfg, ok := c.vi()
	if !ok {
		return
	}
	attr, err := plugin.GetAttributes(cfg, "search_attr")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["vi.search_attr"] = err
		}
	}
	return attr
}

func (c ideConfig) viDebug() (ret bool) {
	return c.viBool("debug")
}

func (c ideConfig) viWrap() (ret bool) {
	return c.viBool("wrap")
}

func (c ideConfig) viBool(name string) (ret bool) {
	cfg, ok := c.vi()
	if !ok {
		return
	}
	ret, err := cfg.GetBool(name)
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors["vi."+name] = err
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
		if err != plugin.ErrNotFound {
			c.errors["browser.tabspaces"] = err
		}
		return
	}
	tabs = cfgTabs
	return
}

func (c ideConfig) wallpaperFrom(cfgKey string) (text string) {
	text = defaultWallpaper
	if c.cfg == nil {
		return
	}
	cfg, ok := c.getConfig(plugin.MapConfig(c.cfg), cfgKey)
	if !ok {
		return
	}
	cfgText, err := cfg.GetString("wallpaper")
	if err != nil {
		if err != plugin.ErrNotFound {
			c.errors[fmt.Sprintf("%s.wallpaper", cfgKey)] = err
		}
		return
	}
	text = cfgText
	return
}

func (c ideConfig) browserWallpaper() (text string) {
	return c.wallpaperFrom("browser")
}

func (c ideConfig) logOutputPath() string {
	if c.cfg == nil {
		return ""

	}
	path, err := plugin.MapConfig(c.cfg).GetString("log_path")
	if err != nil {
		if err != plugin.ErrNotFound {
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
	levelStr, err := plugin.MapConfig(c.cfg).GetString("log_level")
	if err != nil {
		if err != plugin.ErrNotFound {
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

func (c ideConfig) outputMode() (out term.OutputMode) {
	out = term.Output256

	outputModeIfc, ok := c.cfg["output_mode"]
	if !ok {
		return
	}

	outputModeStr, ok := outputModeIfc.(string)
	if !ok {
		c.errors["output_mode"] = errors.New("invalid type")
		return
	}

	switch outputModeStr {
	case outputNormal:
		out = term.OutputNormal
	case output256:
		out = term.Output256
	case outputGrayscale:
		out = term.OutputGrayscale
	default:
		c.errors["output_mode"] = fmt.Errorf("unknown output mode: %s", outputModeStr)
	}

	return
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

func (c ideConfig) plugins() map[string]pluginConfig {
	pConfigIfc, ok := c.cfg["plugins"]
	if !ok {
		return nil
	}
	pConfigMap, ok := pConfigIfc.(map[string]interface{})
	if !ok {
		c.errors["plugins"] = errors.New("invalid type")
		return nil
	}

	ret := make(map[string]pluginConfig)
	for id, pConfig := range pConfigMap {
		pcfg, ok := pConfig.(map[string]interface{})
		if !ok {
			c.errors["plugins."+id] = errors.New("invalid type")
			continue
		}
		ret[id] = pluginConfig{
			id:     id,
			parent: &c,
			cfg:    plugin.MapConfig(pcfg),
		}
	}

	return ret
}

func (c pluginConfig) path() (string, bool) {
	path, err := c.cfg.GetString("path")
	if err != nil {
		// path is non-optional
		errorID := fmt.Sprintf("plugin.%s.path", c.id)
		c.parent.errors[errorID] = err
		return "", false
	}
	return path, true
}

func (c pluginConfig) config() (plugin.Config, bool) {
	cfg, err := c.cfg.GetConfig("config")
	if err != nil {
		if err != plugin.ErrNotFound {
			errorID := fmt.Sprintf("plugin.%s.config", c.id)
			c.parent.errors[errorID] = err
		}
		return nil, false
	}
	return cfg, true
}

func (c ideConfig) workspace() (plugin.Config, bool) {
	if c.cfg == nil {
		return nil, false
	}
	return c.getConfig(plugin.MapConfig(c.cfg), "workspace")
}

func (c ideConfig) workspaceWallpaper() (text string) {
	return c.wallpaperFrom("workspace")
}

func (c ideConfig) workspaceWallpaperAttr() term.Attributes {
	return c.getConfigAttr("workspace", "wallpaper_attr",
		browser.DefaultConfig().WallpaperAttr)
}

func (c ideConfig) workspaceWallpaperBackgroundAttr() term.Attributes {
	return c.getConfigAttr("workspace", "wallpaper_background_attr",
		browser.DefaultConfig().WallpaperBackgroundAttr)
}

func (c ideConfig) workspaceSSHTimeout() (ret time.Duration) {
	ret = defSSHTimeout

	cfg, ok := c.workspace()
	if !ok {
		return
	}

	sshTimeout, err := plugin.GetDuration(cfg, "ssh_timeout", defSSHTimeout)
	if err != nil {
		c.errors["workspace.ssh_timeout"] = err
		return
	}

	ret = sshTimeout
	return
}

func (c ideConfig) workspaceSSHCommand() string {
	cfg, ok := c.workspace()
	if !ok {
		return ""
	}

	cmd, err := cfg.GetString("ssh_command")
	if err != nil {
		c.errors["workspace.ssh_command"] = err
		return ""
	}

	return cmd
}

func (c ideConfig) workspaceSSHPrivateKeys() (ret []string) {
	cfg, ok := c.workspace()
	if !ok {
		return
	}

	keyIfcs, err := cfg.GetSlice("ssh_private_keys")
	if err != nil {
		c.errors["workspace.ssh_private_keys"] = err
		return
	}

	for i, keyIfc := range keyIfcs {
		key, ok := keyIfc.(string)
		if !ok {
			errorID := fmt.Sprintf("workspace.ssh_private_keys.%d", i)
			c.errors[errorID] = fmt.Errorf("string expected but found %v", key)
			continue
		}
		ret = append(ret, key)
	}

	return ret
}

func decodeConfig(r io.Reader) (cfg map[string]interface{}, err error) {
	d := yaml.NewDecoder(r)

	cfg = make(map[string]interface{})
	err = d.Decode(&cfg)
	return
}

func loadLocalConfig(m *workspace.Manager, cwd workspace.URI, c *ideConfig) error {
	localConfigPath := workspace.Join(cwd, ".sixrc")
	buf := cell.NewBuffer()
	closer, err := m.Open(localConfigPath, buf, workspace.URI{}, true)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open local config file: %s", err)
	}
	defer closer.Close()

	reader := strings.NewReader(buf.String())
	cfg, err := decodeConfig(reader)
	if err != nil {
		return err
	}

	overrideConfig(c.cfg, cfg)
	return nil
}

func loadConfig(c *ideConfig, configpath string) error {
	f, err := os.Open(configpath)
	if err != nil {
		initDefaultConfig(c)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cfg, err := decodeConfig(f)
	if err != nil {
		initDefaultConfig(c)
		return err
	}

	initConfig(c, cfg)

	return nil
}
