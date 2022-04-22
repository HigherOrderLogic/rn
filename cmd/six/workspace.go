package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/ernestrc/go-multierror"
	multierr "github.com/ernestrc/go-multierror"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/plugin"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

var (
	workspaceCommands = map[string]func(*workspaceHandler, ...string) error{
		"addWorkspace":      (*workspaceHandler).commandAddWorkspace,
		"closeWorkspace":    (*workspaceHandler).commandCloseWorkspace,
		"switchToWorkspace": (*workspaceHandler).commandSwitchToWorkspace,
	}
)

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
	ed        text.Editor
	messenger browser.Messenger

	width, height int
	workspaces    []*handlerManager
	focus         int
	empty         tui.Handler
}

// newHandler allocates storage for a new workspace tui.Handler and initializes it
// with the given initial workspace.Manager and an instance created
// with tue given tui.Handler factory function.
func newHandler(
	ed text.Editor, messenger browser.Messenger, logger *log.Logger,
	clipboard *plugin.ClipboardManager, initial workspace.URI,
	cfg ideConfig, recfilename string, filenames []string,
) (*workspaceHandler, error) {
	ret := new(workspaceHandler)
	err := ret.init(ed, messenger, logger,
		clipboard, initial, cfg, recfilename, filenames)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (h *workspaceHandler) init(
	ed text.Editor, messenger browser.Messenger, logger *log.Logger,
	clipboard *plugin.ClipboardManager, uri workspace.URI, cfg ideConfig,
	recfilename string, filenames []string,
) error {
	h.workspaces = make([]*handlerManager, 10)
	h.logger = logger
	h.cfg = cfg
	h.clipboard = clipboard
	h.messenger = messenger
	h.ed = ed
	// TODO
	// TODO
	// TODO
	// TODO
	// TODO
	/*h.empty = newCommandHandler(func(workspace string) error {
		// TODO
	})*/
	h.empty = handler.Nop(component.String("wasup"))

	for id, fn := range workspaceCommands {
		ed.SubscribeCommand(id, text.FuncCommandHandler(
			func(ctx context.Context, cmd text.Command) bool {
				err := fn(h, cmd.Args...)
				if err == nil {
					return false
				}
				msg := fmt.Sprintf("%s error: %s", id, err)
				logger.Error(err)
				err = messenger.SetMessage(msg)
				if err != nil {
					logger.Error(fmt.Sprintf("SetMessage error: %s", err))
				}
				return false
			}))
	}

	return h.addWorkspace(uri, h.cfg, recfilename, filenames)
}

func (h *workspaceHandler) Resize(width, height int) {
	h.width, h.height = width, height
	for _, w := range h.workspaces {
		if w != nil {
			w.Resize(width, height)
		}
	}
	h.empty.Resize(width, height)
}

func (h *workspaceHandler) focusHandler() tui.Handler {
	// TODO consider refactoring such that Handle() does not need
	// to do this extra if statement
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler
	}

	return h.empty
}

func (h *workspaceHandler) Draw(w term.Writer) {
	// TODO
	// if workspaces is > 1 then draw bar with
	// workspaces and name of workspace on bottom right
	// otherwise do not draw bar
	h.focusHandler().Draw(w)
}

func (h *workspaceHandler) switchToWorkspace(i int) {
	h.focus = i
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

func (h *workspaceHandler) addWorkspace(
	uri workspace.URI, cfg ideConfig, recfilename string, filenames []string,
) error {

	var (
		textOpts      []text.Option
		workspaceOpts []workspace.Option
	)

	for _, key := range cfg.workspaceSSHPrivateKeys() {
		workspaceOpts = append(workspaceOpts, workspace.WithSSHPrivateKey(key))
	}
	if cmd := cfg.workspaceSSHCommand(); cmd != "" {
		workspaceOpts = append(workspaceOpts, workspace.WithSSHCommand(cmd))
	}
	workspaceOpts = append(workspaceOpts,
		workspace.WithSSHTimeout(cfg.workspaceSSHTimeout()))

	// workspace manager local configs and logger config for Manager are ignored
	workspaceManager, err := workspace.NewManager(h.logger, uri, workspaceOpts...)
	if err != nil {
		return fmt.Errorf("Failed to create new workspace manager: %s", err)
	}

	configErr := loadLocalConfig(workspaceManager, uri, &cfg)

	textOpts = append(textOpts,
		text.WithTabspaces(cfg.browserTabspaces()),
		text.WithStartText(cfg.browserStartText()),
		text.WithWindowManagerConfig(cfg.windowManagerConfig()),
		text.WithFrameUnionCharSet(cfg.frameUnionCharset()),
		text.WithCommandEvent(term.Event{Type: term.EventKey, Ch: ':'}),
		text.WithCommandMaxHistory(cfg.commandMaxHistory()),
		text.WithMessageBarAttr(cfg.messageBarAttr()),
		text.WithFocusTabAttr(cfg.focusTabAttr()),
		text.WithNonFocusTabAttr(cfg.nonFocusTabAttr()),
		text.WithStartTextAttr(cfg.startTextAttr()),
		text.WithStartTextBackgroundAttr(cfg.startTextBackgroundAttr()),
		text.WithDirtyTabAttr(cfg.dirtyTabAttr()),
		text.WithCommandOverlayConfig(cfg.commandOverlayConfig()),
		text.WithPromptConfig(cfg.promptConfig()),
		// TODO validate not perf hit
		text.WithLogger(h.logger),
	)

	for seq, cmd := range cfg.commandKeyMappings() {
		if seq.Last != (term.Event{}) {
			textOpts = append(textOpts, text.WithCommandSequenceBinding(seq, cmd))
		} else {
			textOpts = append(textOpts, text.WithCommandKeyBinding(seq.First, cmd))
		}
	}

	ex, err := newEx(h.ed, workspaceManager, textOpts...)
	if err != nil {
		return err
	}
	ex.Resize(h.width, h.height)

	res := plugin.BrowserResources(ex.Browser())
	res = plugin.MergeResourceMap(res, plugin.EditorResources(ex.Editor()))
	res = plugin.MergeResourceMap(res, plugin.WorkspaceResources(workspaceManager))
	res[plugin.PermissionClipboard] = h.clipboard

	pluginOpts := []plugin.Option{
		plugin.WithLogger(h.logger),
		plugin.WithLocker(&h.mu),
	}
	pluginManager, err := plugin.NewManager(plugin.GrantAll(res), pluginOpts...)
	if err != nil {
		return fmt.Errorf("error initializing plugin manager: %v", err)
	}

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

	go h.initPlugins(h.logger, pluginManager, cfg)

	h.workspaces[h.focus] = &handlerManager{Handler: ex, Manager: workspaceManager, Plugins: pluginManager}

	reportNonFatalErrs(h.logger, h.messenger, configErr, cfg.errors)

	return nil
}

func (h *workspaceHandler) commandAddWorkspace(args ...string) error {
	if len(args) == 0 {
		return errors.New("invalid arguments. " +
			"Expecting 1 argument with workspace URI")
	}
	if h.focusHandler() != h.empty {
		return errors.New("workspace tab is not empty. " +
			"Switch to an empty workspace tab to add a workspace")
	}
	uri, err := workspace.ParseURI(args[0])
	if err != nil {
		return fmt.Errorf("ParseURI: %s", err)
	}
	return h.addWorkspace(uri, h.cfg, "", nil)
}

func (h *workspaceHandler) commandCloseWorkspace(args ...string) (ret error) {
	if h.focusHandler() == h.empty {
		return errors.New("workspace tab is empty")
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
	return ret
}

func (h *workspaceHandler) commandSwitchToWorkspace(args ...string) error {
	if len(args) == 0 {
		return errors.New("invalid arguments. " +
			"Expecting 1 argument with workspace number")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid workspace number: %s", err)
	}
	h.switchToWorkspace(n)
	return nil
}
