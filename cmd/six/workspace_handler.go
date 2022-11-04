package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/ernestrc/blue/document"
	"github.com/ernestrc/blue/encoding/bson"
	multierr "github.com/ernestrc/go-multierror"
	"unstable.build/go-tui"
	"unstable.build/go-tui/config"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/plugin"
	"unstable.build/go-tui/storage"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

const (
	cmdSwitchToWorkspace = "switchToWorkspace"
	cmdCloseWorkspace    = "closeWorkspace"
)

var (
	workspaceCommands = map[string]func(*workspaceManagerHandler, ...string) error{
		"addWorkspace":       (*workspaceManagerHandler).commandAddWorkspace,
		cmdCloseWorkspace:    (*workspaceManagerHandler).commandCloseWorkspace,
		cmdSwitchToWorkspace: (*workspaceManagerHandler).commandSwitchToWorkspace,
		"quit":               (*workspaceManagerHandler).commandQuit,
		"forceQuit!":         (*workspaceManagerHandler).commandQuit,
	}
	defaultCommandKey         = term.KeyComb{Ch: ':'}
	defaultWorkspaceSequences = map[handler.Sequence][]string{
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '1'}}: {"switchToWorkspace", "1"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '2'}}: {"switchToWorkspace", "2"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '3'}}: {"switchToWorkspace", "3"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '4'}}: {"switchToWorkspace", "4"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '5'}}: {"switchToWorkspace", "5"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '6'}}: {"switchToWorkspace", "6"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '7'}}: {"switchToWorkspace", "7"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '8'}}: {"switchToWorkspace", "8"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '9'}}: {"switchToWorkspace", "9"},
		{First: term.KeyComb{Key: term.KeyCtrlX},
			Last: term.KeyComb{Ch: '0'}}: {"switchToWorkspace", "10"},
	}
)

type workspaceManagerHandler struct {
	mu           sync.Mutex
	exit         bool
	clipboard    clipboardManagerIfc
	cfg          ideConfig
	storage      document.Service
	workspace    workspace.WorkspaceManager
	publishEvent func(term.Event) bool
	sixDir       string

	union          handler.FrameUnion
	bar            handler.Tabs
	focusProxy     handler.Proxy
	width, height  int
	workspaces     []*workspaceHandler
	workspaceCount int
	focus          int
	empty          *ex
}

type clipboardManagerIfc interface {
	plugin.ResourceServer
	text.Clipboard
}

func newWorkspaceManagerHandler(
	clipboard clipboardManagerIfc, initial workspace.URI,
	manager workspace.WorkspaceManager,
	cfg ideConfig, recfilename string, filenames []string,
	sixDir string,
	publishEvent func(term.Event) bool,
) (*workspaceManagerHandler, error) {
	ret := new(workspaceManagerHandler)

	err := ret.init(clipboard, initial, manager,
		cfg, recfilename, filenames, sixDir, publishEvent)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (h *workspaceManagerHandler) newEditor(cfg ideConfig) text.Editor {
	viOpts := append([]vi.Option{},
		vi.WithResAttr(cfg.viResultAttr()),
		vi.WithDebug(cfg.viDebug()),
		vi.WithWrap(cfg.viWrap()),
		vi.WithClipboard(h.clipboard),
	)
	return vi.Editor(viOpts...)
}

func (h *workspaceManagerHandler) init(
	clipboard clipboardManagerIfc, uri workspace.URI,
	manager workspace.WorkspaceManager, cfg ideConfig,
	recfilename string, filenames []string,
	sixDir string,
	publishEvent func(term.Event) bool,
) error {
	h.workspaces = make([]*workspaceHandler, 10)
	h.cfg = cfg
	h.clipboard = clipboard
	h.publishEvent = publishEvent
	h.workspace = manager
	h.sixDir = sixDir
	storage, err := storage.New(sixDir, bson.Marshaler())
	if err != nil {
		storage = document.NewInMemoryService()
		debug.StandardLogger().Warnf("Could not setup fs-backed storage: %v. Using ephemeral.", err)
	}
	h.storage = storage

	globalOpts := h.textOpts(h.cfg)
	h.empty, _ = newEx(h.newEditor(cfg), nopLoader{}, h.storage, h.publishEvent, globalOpts...)
	err = h.subscribeAllWorkspaceCommands(h.empty)
	if err != nil {
		return err
	}

	h.bar.Init()
	h.bar.OnClick = h.switchToWorkspace
	h.bar.SetAttr(cfg.focusTabAttr(), cfg.nonFocusTabAttr(),
		cfg.windowFrameAttr(), cfg.windowFrameAttr())
	h.bar.SetFrameCharSet(cfg.windowFrameCharset())
	h.bar.SetBorder(cfg.frame())

	h.union.Init(&h.focusProxy)
	h.union.Attributes = cfg.windowFrameAttr()
	h.union.Frame = cfg.frame()

	charset := cfg.frameUnionCharset()
	h.union.Right = charset.Right
	h.union.Left = charset.Left
	h.union.Top = charset.Top
	h.union.Bottom = charset.Bottom

	err = h.addWorkspace(uri, recfilename, filenames)
	if err != nil {
		return err
	}
	h.focusProxy.Target = h.focusHandler()
	h.union.UnionBottom(&h.bar, h.barSize())
	return nil
}

func (h *workspaceManagerHandler) subscribeAllWorkspaceCommands(ex *ex) (ret error) {
	for cmd, _fn := range workspaceCommands {
		fn := _fn
		err := ex.comp.SubscribeCommand(cmd,
			text.FuncCommandHandler(func(ctx context.Context, cmd text.Command) (bool, error) {
				return false, fn(h, cmd.Args...)
			}))
		if err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	return ret
}

func (h *workspaceManagerHandler) subscribeActiveWorkspaceCommands(ex *ex) (ret error) {
	err := ex.comp.SubscribeCommand(cmdSwitchToWorkspace,
		text.FuncCommandHandler(func(ctx context.Context, cmd text.Command) (bool, error) {
			return false, h.commandSwitchToWorkspace(cmd.Args...)
		}))
	if err != nil {
		ret = multierr.Append(ret, err)
	}
	err = ex.comp.SubscribeCommand(cmdCloseWorkspace,
		text.FuncCommandHandler(func(ctx context.Context, cmd text.Command) (bool, error) {
			return false, h.commandCloseWorkspace()
		}))
	if err != nil {
		ret = multierr.Append(ret, err)
	}
	return
}

func (h *workspaceManagerHandler) focusHandler() tui.Handler {
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler
	}

	return h.empty
}

func (h *workspaceManagerHandler) drawBar() bool {
	return h.workspaceCount > 1 || h.focusHandler() == h.empty
}

func (h *workspaceManagerHandler) barSize() int {
	frame := h.cfg.frame()
	ret := 1
	if frame {
		ret += 2
	}
	return ret
}

func (h *workspaceManagerHandler) Resize(width, height int) {
	h.width, h.height = width, height
	h.bar.RemoveAll()

	drawBar := h.drawBar()
	if drawBar {
		height -= h.barSize()
	}
	var barFocusIdx int
	for i, w := range h.workspaces {
		if w != nil {
			w.Resize(width, height)
			idx := h.bar.Add(strconv.Itoa(i + 1))
			if i == h.focus {
				barFocusIdx = idx
			}
		} else if i == h.focus {
			idx := h.bar.Add(strconv.Itoa(i + 1))
			barFocusIdx = idx
		}
	}
	h.bar.SetFocus(barFocusIdx)
	h.empty.Resize(width, height)
	// bar needs to be drawn last so frame union characters
	// are drawn last
	if drawBar {
		h.union.Resize(h.width, h.height)
	}
}

func (h *workspaceManagerHandler) Draw(w term.Writer) {
	target := h.focusHandler()
	h.focusProxy.Target = target
	if h.drawBar() {
		h.union.Draw(w)
	} else {
		target.Draw(w)
	}
}

func (h *workspaceManagerHandler) switchToWorkspace(i int) {
	h.focus = i
	h.focusProxy.Target = h.focusHandler()
	// resize so disappearing bar feature can be implemented
	h.Resize(h.width, h.height)
}

func (h *workspaceManagerHandler) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = h.focusHandler().Handle(ev)
	return exit || h.exit, handled
}

func (h *workspaceManagerHandler) Cursor() (pos term.Coordinates, show bool) {
	return h.focusHandler().Cursor()
}

func (h *workspaceManagerHandler) Man() tui.Manual {
	return h.focusHandler().Man()
}

func (h *workspaceManagerHandler) initPlugins(manager *plugin.Manager, cfg ideConfig) {
	for id, p := range cfg.plugins() {
		path, ok := p.path()
		if !ok {
			continue
		}
		pconfig, ok := p.config()
		if !ok {
			pconfig = config.MapConfig(make(map[string]interface{}))
		}
		err := manager.Run(id, path, pconfig)
		if err != nil {
			debug.StandardLogger().Errorf("failed to run plugin with id %q: %v", id, err)
		}
	}
}

func (h *workspaceManagerHandler) textOpts(cfg ideConfig) []text.Option {
	ret := []text.Option{
		text.WithTabspaces(cfg.browserTabspaces()),
		text.WithWindowManagerConfig(cfg.windowManagerConfig()),
		text.WithFrameUnionCharSet(cfg.frameUnionCharset()),
		text.WithCommandKey(cfg.commandKey()),
		text.WithCommandMaxHistory(cfg.commandMaxHistory()),
		text.WithMessageBarAttr(cfg.messageBarAttr()),
		text.WithFocusTabAttr(cfg.focusTabAttr()),
		text.WithNonFocusTabAttr(cfg.nonFocusTabAttr()),
		text.WithWallpaperAttr(cfg.workspaceWallpaperAttr()),
		text.WithWallpaperBackgroundAttr(cfg.workspaceWallpaperBackgroundAttr()),
		text.WithWallpaper(cfg.wallpaper()),
		text.WithDirtyTabAttr(cfg.dirtyTabAttr()),
		text.WithCommandOverlayConfig(cfg.commandOverlayConfig()),
		text.WithCommandAliases(cfg.commandAliases()),
		text.WithPromptConfig(cfg.promptConfig()),
		text.WithInterrupt(func() {
			h.publishEvent(term.Event{Type: term.EventInterrupt})
		}),
		text.WithSendNone(func() {
			forcePublishEvent(h.publishEvent)(term.Event{Type: term.EventNone})
		}),
	}

	for seq, cmd := range defaultWorkspaceSequences {
		ret = append(ret, text.WithCommandSequenceBinding(seq, cmd))
	}

	for seq, cmd := range cfg.commandKeyMappings() {
		if seq.Last != (term.KeyComb{}) {
			ret = append(ret, text.WithCommandSequenceBinding(seq, cmd))
		} else {
			ret = append(ret, text.WithCommandKeyBinding(seq.First, cmd))
		}
	}

	return ret
}

func cloneConfig(cfg ideConfig) ideConfig {
	ret := make(map[string]interface{})
	config.Clone(config.MapConfig(cfg.cfg)).Iterate(func(k string, v interface{}) {
		ret[k] = v
	})
	return ideConfig{cfg: ret, errors: make(map[string]error)}
}

func (h *workspaceManagerHandler) addWorkspace(
	uri workspace.URI, recfilename string, filenames []string,
) error {
	cwd, err := h.workspace.AddWorkspace(uri)
	if err != nil {
		return fmt.Errorf("Failed to create new workspace for %q: %s", uri, err)
	}

	cfg := cloneConfig(h.cfg)

	isConfigErr, configErr := loadWorkspaceConfig(cwd, uri, &cfg)
	if configErr != nil && !isConfigErr {
		return configErr
	}

	textOpts := h.textOpts(cfg)
	if recfilename != "" {
		recFile, err := cwd.URI(recfilename)
		if err != nil {
			return err
		}
		textOpts = append(textOpts, text.WithRecoveryFile(recFile))
	}

	for _, filename := range filenames {
		file, err := cwd.URI(filename)
		if err != nil {
			return err
		}
		textOpts = append(textOpts, text.WithFile(file))
	}

	// workspace capable of opening URIs other than the workspace URI
	multicwd := workspace.Multi(h.workspace, cwd, uri)
	ex, err := newEx(h.newEditor(cfg), multicwd, h.storage,
		h.publishEvent, textOpts...)
	if err != nil {
		return err
	}
	err = ex.subscribeCommands()
	if err != nil {
		return err
	}
	err = h.subscribeActiveWorkspaceCommands(ex)
	if err != nil {
		return err
	}

	res := plugin.BrowserResources(ex.Browser())
	res = plugin.MergeResourceMap(res, plugin.EditorResources(ex.Editor()))
	res = plugin.MergeResourceMap(res, plugin.WorkspaceResources(cwd))
	// NOTE: plugins that register new schemes will fail for subsequent workspaces
	res = plugin.MergeResourceMap(res, plugin.SchemeManagerResources(h.workspace))
	res = plugin.MergeResourceMap(res, plugin.StorageResources(h.sixDir))
	res = plugin.MergeResourceMap(res, plugin.ConfigResources(
		config.MapConfig(cfg.cfg)))
	res[plugin.PermissionClipboard] = h.clipboard

	pluginOpts := []plugin.Option{
		plugin.WithLocker(&h.mu),
		plugin.WithWorkspace(uri),
	}
	pluginManager, err := plugin.NewManager(plugin.GrantAll(res), pluginOpts...)
	if err != nil {
		return fmt.Errorf("error initializing plugin manager: %v", err)
	}

	go h.initPlugins(pluginManager, cfg)

	h.workspaces[h.focus] = &workspaceHandler{
		Handler:         ex,
		workspaceCloser: cwd,
		Plugins:         pluginManager,
		pluginResources: res,
	}
	h.workspaceCount++
	h.switchToWorkspace(h.focus)

	logNonFatalErrs(configErr, cfg.errors)

	return nil
}

func logNonFatalErrs(
	configErr error,
	configErrs map[string]error,
) {
	all := configErr
	for key, err := range configErrs {
		err = fmt.Errorf("Failed to load %q: %v", key, err)
		all = multierr.Append(all, err)
	}
	if all != nil {
		debug.StandardLogger().Warn(all)
	}
}

func (h *workspaceManagerHandler) commandAddWorkspace(args ...string) error {
	if len(args) == 0 {
		return errors.New("invalid arguments. " +
			"Expecting 1 argument with workspace URI")
	}
	if h.focusHandler() != h.empty {
		return errors.New("workspace tab is not empty. " +
			"Switch to an empty workspace tab to add a workspace")
	}
	path := args[0]

	// try to use literal URI
	uri, parseErr := workspace.ParseURI(path)
	if parseErr == nil {
		return h.addWorkspace(uri, "", nil)
	}

	uri, pathErr := workspace.CurrentUserHostURI(path)
	if pathErr != nil {
		err := multierr.Append(pathErr, parseErr)
		return err
	}
	return h.addWorkspace(uri, "", nil)
}

func (h *workspaceManagerHandler) commandCloseWorkspace(args ...string) (
	ret error,
) {
	if h.focusHandler() == h.empty {
		return errors.New("workspace tab is empty")
	}

	hm := h.workspaces[h.focus]
	if err := hm.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	h.workspaces[h.focus] = nil
	h.workspaceCount--

	for i := h.focus; i >= 0; i-- {
		if h.workspaces[i] != nil {
			h.switchToWorkspace(i)
			return ret
		}
	}

	// for resize of current workspace with empty
	h.switchToWorkspace(h.focus)

	return ret
}

func (h *workspaceManagerHandler) commandQuit(args ...string) error {
	h.exit = true
	return nil
}

func (h *workspaceManagerHandler) commandSwitchToWorkspace(args ...string) error {
	if len(args) == 0 {
		return errors.New("invalid arguments. " +
			"Expecting 1 argument with workspace number")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid workspace: %s", err)
	}
	n-- // UI does not use 0-based indexing
	if n < 0 || n >= len(h.workspaces) {
		return fmt.Errorf("invalid workspace: there's only %d workspaces",
			len(h.workspaces))
	}
	h.switchToWorkspace(n)
	return nil
}

func (h *workspaceManagerHandler) Close() (ret error) {
	for _, hm := range h.workspaces {
		if hm == nil {
			continue
		}
		if err := hm.Handler.(*ex).Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
		if err := hm.workspaceCloser.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
		if err := hm.Plugins.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	if err := h.empty.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := h.storage.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := h.clipboard.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	return
}

type workspaceHandler struct {
	tui.Handler
	workspaceCloser io.Closer
	Plugins         *plugin.Manager
	pluginResources map[plugin.Permission]plugin.ResourceServer
}

func (hm *workspaceHandler) Close() (ret error) {
	if err := hm.workspaceCloser.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := hm.Handler.(*ex).Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := hm.Plugins.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	for _, res := range hm.pluginResources {
		if err := res.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	return
}
