package main

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-multierror"
	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/text/vi"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

const (
	cmdSwitchToWorkspace = "switchToWorkspace"
	cmdCloseWorkspace    = "closeWorkspace"
)

var (
	workspaceCommands = map[string]func(*workspaceHandler, ...string) (bool, error){
		"addWorkspace":       (*workspaceHandler).commandAddWorkspace,
		cmdCloseWorkspace:    (*workspaceHandler).commandCloseWorkspace,
		cmdSwitchToWorkspace: (*workspaceHandler).commandSwitchToWorkspace,
		"quit":               (*workspaceHandler).commandQuit,
		"forceQuit!":         (*workspaceHandler).commandQuit,
	}
	defaultCommandEvent = term.Event{Type: term.EventKey, Ch: ':'}

	workspaceCommandList []string
	exCommandList        []string
)

func init() {
	for cmd := range workspaceCommands {
		workspaceCommandList = append(workspaceCommandList, cmd)
	}
	for cmd := range exCommands {
		exCommandList = append(exCommandList, cmd)
	}
	exCommandList = append(exCommandList, cmdSwitchToWorkspace)
	exCommandList = append(exCommandList, cmdCloseWorkspace)
}

// rename to workspaceHandler
type handlerManager struct {
	tui.Handler
	*workspace.Manager
	Plugins *plugin.Manager
}

// TODO rename to workspaceManagerHandler
type workspaceHandler struct {
	mu        sync.Mutex
	clipboard *plugin.ClipboardManager
	cfg       ideConfig
	logger    *log.Logger
	storage   browser.Storage

	union          handler.FrameUnion
	bar            handler.Tabs
	focusProxy     handler.Proxy
	width, height  int
	workspaces     []*handlerManager
	workspaceCount int
	focus          int
	empty          tui.Handler
}

func newWorkspaceHandler(
	logger *log.Logger,
	clipboard *plugin.ClipboardManager, initial workspace.URI,
	cfg ideConfig, recfilename string, filenames []string,
) (*workspaceHandler, error) {
	ret := new(workspaceHandler)
	err := ret.init(logger,
		clipboard, initial, cfg, recfilename, filenames)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (h *workspaceHandler) newEditor(cfg ideConfig) text.Editor {
	viOpts := append([]vi.Option{},
		vi.WithResAttr(cfg.viResultAttr()),
		vi.WithDebug(cfg.viDebug()),
		vi.WithWrap(cfg.viWrap()),
		vi.WithClipboard(h.clipboard),
	)
	return vi.Editor(viOpts...)
}

func (h *workspaceHandler) init(
	logger *log.Logger,
	clipboard *plugin.ClipboardManager, uri workspace.URI, cfg ideConfig,
	recfilename string, filenames []string,
) error {
	h.workspaces = make([]*handlerManager, 10)
	h.logger = logger
	h.cfg = cfg
	h.clipboard = clipboard
	h.storage = document.NewInMemoryCache()

	globalOpts := h.textOpts(h.cfg)
	h.empty, _ = newEx(h.newEditor(cfg), nopWorkspace{}, workspaceCommandList,
		func(argv []string) (bool, bool, error) {
			fn, ok := workspaceCommands[argv[0]]
			if !ok {
				return false, false, nil
			}
			quit, err := fn(h, argv[1:]...)
			return quit, true, err
		}, globalOpts...)

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

	err := h.addWorkspace(uri, h.cfg, recfilename, filenames)
	if err != nil {
		return err
	}
	h.focusProxy.Target = h.focusHandler()
	h.union.UnionBottom(&h.bar, h.barSize())
	return nil
}

func (h *workspaceHandler) focusHandler() tui.Handler {
	// TODO consider refactoring such that Handle() does not need
	// to do this extra if statement
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler
	}

	return h.empty
}

func (h *workspaceHandler) drawBar() bool {
	return h.workspaceCount > 1 || h.focusHandler() == h.empty
}

func (h *workspaceHandler) barSize() int {
	frame := h.cfg.frame()
	ret := 1
	if frame {
		ret += 2
	}
	return ret
}

func (h *workspaceHandler) Resize(width, height int) {
	h.width, h.height = width, height
	h.bar.RemoveAll()

	drawBar := h.drawBar()
	if drawBar {
		height -= h.barSize()
	}
	barFocusIdx := -1
	for i, w := range h.workspaces {
		if w != nil {
			w.Resize(width, height)
			idx := h.bar.Add(strconv.Itoa(i + 1))
			if i == h.focus {
				barFocusIdx = idx
			}
		}
	}
	// focus is empty workspace, add tab
	if barFocusIdx == -1 {
		idx := h.bar.Add(strconv.Itoa(h.focus + 1))
		barFocusIdx = idx
	}
	h.bar.SetFocus(barFocusIdx)
	h.empty.Resize(width, height)
	// bar needs to be drawn last so frame union characters
	// are drawn last
	if drawBar {
		h.union.Resize(h.width, h.height)
	}
}

func (h *workspaceHandler) Draw(w term.Writer) {
	// TODO
	// if workspaces is > 1 then draw bar with
	// workspaces and name of workspace on bottom right
	// otherwise do not draw bar
	target := h.focusHandler()
	h.focusProxy.Target = target
	if h.drawBar() {
		h.union.Draw(w)
	} else {
		target.Draw(w)
	}
}

func (h *workspaceHandler) switchToWorkspace(i int) {
	h.focus = i
	h.focusProxy.Target = h.focusHandler()
	// resize so disappearing bar feature can be implemented
	h.Resize(h.width, h.height)
}

func (h *workspaceHandler) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = h.focusHandler().Handle(ev)
	if exit || handled {
		return exit, handled
	}
	handled = true
	switch ev.Ch {
	case '1':
		h.switchToWorkspace(0)
	case '2':
		h.switchToWorkspace(1)
	case '3':
		h.switchToWorkspace(2)
	case '4':
		h.switchToWorkspace(3)
	case '5':
		h.switchToWorkspace(4)
	case '6':
		h.switchToWorkspace(5)
	case '7':
		h.switchToWorkspace(6)
	case '8':
		h.switchToWorkspace(7)
	case '9':
		h.switchToWorkspace(8)
	}
	return
}

func (h *workspaceHandler) Cursor() (pos term.Coordinates, show bool) {
	return h.focusHandler().Cursor()
}

func (h *workspaceHandler) Man() tui.Manual {
	return h.focusHandler().Man()
}

func (h *workspaceHandler) Close() error {
	var ret error
	for _, hm := range h.workspaces {
		if hm == nil {
			continue
		}
		if err := hm.Handler.(*ex).Close(); err != nil {
			multierror.Append(ret, err)
		}
		if err := hm.Manager.Close(); err != nil {
			multierror.Append(ret, err)
		}
		if err := hm.Plugins.Close(); err != nil {
			multierror.Append(ret, err)
		}
	}
	return nil
}

func (h *workspaceHandler) initPlugins(l *log.Logger, manager *plugin.Manager, cfg ideConfig) {
	for id, p := range cfg.plugins() {
		path, ok := p.path()
		if !ok {
			continue
		}
		config, ok := p.config()
		if !ok {
			config = plugin.MapConfig(make(map[string]interface{}))
		}
		err := manager.Run(id, path, config)
		if err != nil {
			l.Errorf("failed to run plugin: could not run plugin with id '%s': %v", id, err)
		}
	}
}

func (h *workspaceHandler) textOpts(cfg ideConfig) []text.Option {
	ret := []text.Option{
		text.WithTabspaces(cfg.browserTabspaces()),
		// TODO override for workspace with workspace wallpaper option
		text.WithStartText(cfg.browserStartText()),
		text.WithWindowManagerConfig(cfg.windowManagerConfig()),
		text.WithFrameUnionCharSet(cfg.frameUnionCharset()),
		text.WithCommandEvent(defaultCommandEvent),
		text.WithCommandMaxHistory(cfg.commandMaxHistory()),
		text.WithMessageBarAttr(cfg.messageBarAttr()),
		text.WithFocusTabAttr(cfg.focusTabAttr()),
		text.WithNonFocusTabAttr(cfg.nonFocusTabAttr()),
		text.WithStartTextAttr(cfg.startTextAttr()),
		text.WithStartTextBackgroundAttr(cfg.startTextBackgroundAttr()),
		text.WithDirtyTabAttr(cfg.dirtyTabAttr()),
		text.WithCommandOverlayConfig(cfg.commandOverlayConfig()),
		text.WithPromptConfig(cfg.promptConfig()),
		text.WithStorage(h.storage),
		// TODO validate not perf hit
		text.WithLogger(h.logger),
	}

	for seq, cmd := range cfg.commandKeyMappings() {
		if seq.Last != (term.Event{}) {
			ret = append(ret, text.WithCommandSequenceBinding(seq, cmd))
		} else {
			ret = append(ret, text.WithCommandKeyBinding(seq.First, cmd))
		}
	}

	return ret
}

func (h *workspaceHandler) workspaceOpts(cfg ideConfig) []workspace.Option {
	var workspaceOpts []workspace.Option

	for _, key := range cfg.workspaceSSHPrivateKeys() {
		workspaceOpts = append(workspaceOpts, workspace.WithSSHPrivateKey(key))
	}
	if cmd := cfg.workspaceSSHCommand(); cmd != "" {
		workspaceOpts = append(workspaceOpts, workspace.WithSSHCommand(cmd))
	}
	workspaceOpts = append(workspaceOpts,
		workspace.WithSSHTimeout(cfg.workspaceSSHTimeout()))

	return workspaceOpts
}

func (h *workspaceHandler) addWorkspace(
	uri workspace.URI, cfg ideConfig, recfilename string, filenames []string,
) error {
	// workspace manager local configs and logger config for Manager are ignored
	workspaceOpts := h.workspaceOpts(cfg)
	workspaceManager, err := workspace.NewManager(h.logger, uri, workspaceOpts...)
	if err != nil {
		return fmt.Errorf("Failed to create new workspace manager: %s", err)
	}

	configErr := loadLocalConfig(workspaceManager, uri, &cfg)

	textOpts := h.textOpts(cfg)
	if recfilename != "" {
		recFile, err := workspaceManager.URI(recfilename)
		if err != nil {
			return err
		}
		textOpts = append(textOpts, text.WithRecoveryFile(recFile))
	}

	for _, filename := range filenames {
		file, err := workspaceManager.URI(filename)
		if err != nil {
			return err
		}
		textOpts = append(textOpts, text.WithFile(file))
	}

	ex, err := newEx(h.newEditor(cfg), workspaceManager, exCommandList,
		func(argv []string) (bool, bool, error) {
			var err error
			handled := true
			switch argv[0] {
			case cmdSwitchToWorkspace:
				_, err = h.commandSwitchToWorkspace(argv[1:]...)
			case cmdCloseWorkspace:
				_, err = h.commandCloseWorkspace()
			default:
				handled = false
			}
			return false, handled, err
		}, textOpts...)
	if err != nil {
		return err
	}

	res := plugin.BrowserResources(ex.Browser())
	res = plugin.MergeResourceMap(res, plugin.EditorResources(ex.Editor()))
	res = plugin.MergeResourceMap(res, plugin.WorkspaceResources(workspaceManager))
	res[plugin.PermissionClipboard] = h.clipboard

	pluginOpts := []plugin.Option{
		plugin.WithLogger(h.logger),
		plugin.WithLocker(&h.mu),
		plugin.WithWorkspace(uri),
	}
	pluginManager, err := plugin.NewManager(plugin.GrantAll(res), pluginOpts...)
	if err != nil {
		return fmt.Errorf("error initializing plugin manager: %v", err)
	}

	go h.initPlugins(h.logger, pluginManager, cfg)

	h.workspaces[h.focus] = &handlerManager{
		Handler: ex,
		Manager: workspaceManager,
		Plugins: pluginManager,
	}
	h.workspaceCount++
	h.switchToWorkspace(h.focus)

	logNonFatalErrs(h.logger, configErr, cfg.errors)

	return nil
}

func logNonFatalErrs(
	l *log.Logger,
	configErr error,
	configErrs map[string]error,
) {
	all := configErr
	for key, err := range configErrs {
		err = fmt.Errorf("Failed to load %q: %v", key, err)
		all = multierr.Append(all, err)
	}
	if l != nil && all != nil {
		l.Warn(all)
	}
}

func (h *workspaceHandler) commandAddWorkspace(args ...string) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("invalid arguments. " +
			"Expecting 1 argument with workspace URI")
	}
	if h.focusHandler() != h.empty {
		return false, errors.New("workspace tab is not empty. " +
			"Switch to an empty workspace tab to add a workspace")
	}
	uri, err := workspace.ParseURI(args[0])
	if err != nil {
		return false, fmt.Errorf("ParseURI: %s", err)
	}
	return false, h.addWorkspace(uri, h.cfg, "", nil)
}

func (h *workspaceHandler) commandCloseWorkspace(args ...string) (
	quit bool, ret error,
) {
	if h.focusHandler() == h.empty {
		return false, errors.New("workspace tab is empty")
	}

	hm := h.workspaces[h.focus]
	if err := hm.Manager.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := hm.Handler.(*ex).Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	if err := hm.Plugins.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	h.workspaces[h.focus] = nil
	h.workspaceCount--

	for i := h.focus; i >= 0; i-- {
		if h.workspaces[i] != nil {
			h.switchToWorkspace(i)
			break
		}
	}

	return false, ret
}

func (h *workspaceHandler) commandQuit(args ...string) (bool, error) {
	return true, nil
}

func (h *workspaceHandler) commandSwitchToWorkspace(args ...string) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("invalid arguments. " +
			"Expecting 1 argument with workspace number")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return false, fmt.Errorf("invalid workspace number: %s", err)
	}
	n++ // UI does not use 0-based indexing
	h.switchToWorkspace(n)
	return false, nil
}
