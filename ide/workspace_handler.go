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
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/document/docmarshal/doctoml"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

const (
	cmdSwitchToWorkspace = "workspacefocus"
	cmdCloseWorkspace    = "workspaceclose"
	cmdReloadWorkspace   = "workspacereload"
	cmdAddWorkspace      = "workspacenew"
	workspaceSlots       = 10
)

var (
	defaultModalCommandKey    = term.KeyComb{Ch: ':'}
	defaultModelessCommandKey = term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrl}
)

type workspaceManagerHandler struct {
	mu                 sync.Locker
	pkgmanager         *pkgManager
	exitPromptOpen     bool
	confirmedForceExit bool
	notifications      *workspaceNotifications
	storage            document.Service
	workspace          workspace.WorkspaceManager
	publishEvent       func(term.Event) bool
	tabsClickCallback  func(int) bool
	extensionRunner    ExtensionsRunner
	sixDir             string
	tabBarOffset       int
	tabBarHeight       int
	builtinExtensions  map[string]Extension
	// this is the name of of the file to be expected in workspace folders
	workspaceConfigFilename string
	workspaceBarKind        workspaceBarKind
	userHome                string
	history                 *history
	workspacesBarHeight     int
	workspacesIcon          rune
	externalCommands        map[string]externalCommand
	// NOTE: if user changes frame config, then mouse calculations
	// for resize might be off.
	frame        bool
	reloadConfig func() (ideConfig, error)

	union            handler.FrameUnion
	bar              handler.Tabs
	focusProxy       handler.Proxy
	width, height    int
	workspaces       [workspaceSlots]*workspaceHandler
	workspaceCount   int
	focus            int
	homeWorkspace    workspace.Workspace
	empty            *ex
	homeRunner       atomic.Value
	openPrevFiles    []file
	openPrevFilesEx  *ex
	openPrevFilesWin browser.Window
	shaderRunner     *shaderRunner
}

func (h *workspaceManagerHandler) newEditor(cfg ideConfig) (text.Editor, error) {
	switch cfg.editorMode() {
	case editorModeModal:
		return h.newBuiltinModalEditor(cfg), nil
	case editorModeModeless:
		return h.newBuiltinModelessEditor(cfg), nil
	default:
		panic("invalid editor mode")
	}
}

func (h *workspaceManagerHandler) newBuiltinModalEditor(cfg ideConfig) text.Editor {
	viOpts := append([]vi.Option{},
		vi.WithBarAttr(cfg.modalBarAttr()),
		vi.WithResAttr(cfg.modalResultAttr()),
		vi.WithAttr(cfg.modalAttr()),
		vi.WithDebug(cfg.modalDebug()),
		vi.WithClipboard(cfg.clipboard()),
	)
	return vi.Editor(viOpts...)
}

func (h *workspaceManagerHandler) newBuiltinModelessEditor(cfg ideConfig) text.Editor {
	return text.NewSimpleEditor(
		cfg.clipboard(), cfg.modelessWrap(), true,
		cfg.modelessAttr(),
		cfg.modelessResultAttr(), cfg.modelessBarAttr())
}

func (h *workspaceManagerHandler) init(
	cwd *workspaceapi.URI, homeDirUri workspaceapi.URI,
	manager workspace.WorkspaceManager,
	notifications *notifications.Container,
	cfg ideConfig, recfilename string, filenames []string,
	sixDir string, publishEvent func(term.Event) bool,
	extensionRunner ExtensionsRunner, locker sync.Locker,
	builtinExtensions map[string]Extension,
	reloadConfig func() (ideConfig, error), workspaceConfigFilename string,
	tabBarOffset, tabBarHeight int, workspacesIcon rune,
	workspacesBarHeight, workspacesBarOffset int, workspacesBarFrame bool,
	tabsClickCallback func(int) bool,
	releaseManager release.Manager,
	shaderRunner *shaderRunner,
) (err error) {
	ctx := context.Background()

	notiStorage := document.WithPartition(h.storage, "noti")
	h.notifications = newWorkspaceNotifications(notiStorage, notifications)
	h.shaderRunner = shaderRunner
	h.mu = locker
	h.externalCommands = make(map[string]externalCommand)
	h.frame = cfg.frame()
	h.reloadConfig = reloadConfig
	h.workspaceConfigFilename = workspaceConfigFilename
	h.tabsClickCallback = tabsClickCallback
	h.publishEvent = publishEvent
	h.workspace = manager
	h.sixDir = sixDir
	h.extensionRunner = extensionRunner
	h.builtinExtensions = builtinExtensions
	h.storage = localstorage.New(ctx, sixDir, doctoml.Marshaler())

	h.workspacesIcon = workspacesIcon
	h.tabBarOffset = tabBarOffset
	h.tabBarHeight = tabBarHeight
	// don't install a fs watcher for the home workspace, to prevent unecessary
	// resource consumption and so we also don't disable the internal flush dispatching
	globalOpts := h.textOpts(cfg)
	ed, err := h.newEditor(cfg)
	if err != nil {
		return fmt.Errorf("new editor: %v", err)
	}

	pkgStorage := document.WithPartition(h.storage, "idepkg")
	h.pkgmanager = newPackageManager(h.notifications, releaseManager, pkgStorage, sixDir)

	homeWorkspace, err := h.workspace.AddWorkspace(ctx, homeDirUri)
	if err != nil {
		return fmt.Errorf("add home workspace: %v", err)
	}

	h.homeWorkspace = homeWorkspace
	h.empty, err = newEx(ed, homeWorkspace, h.storage, notifications,
		cfg.terminalConfig(), h.publishEvent, cfg.clipboard(),
		globalOpts...)
	if err != nil {
		return fmt.Errorf("new ex: %w", err)
	}
	if err = h.subscribeAllCommands(h.empty); err != nil {
		return err
	}

	// speed up initialization
	go debug.CapturePanicReport(func() {
		runner, err := h.buildExtensions(cfg, homeDirUri, h.homeWorkspace, h.empty)
		if err != nil {
			log.Errorf("build home workspace extensions: %v", err)
			return
		}
		h.initExtensions(runner, cfg)
		h.homeRunner.Store(runner)
	})

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
	h.workspaceBarKind = cfg.workspaceBarKind()
	historyStorge := document.WithPartition(h.storage, "history")
	h.history = newHistory(historyStorge)

	// best effort
	user, err := user.Current()
	if err == nil {
		h.userHome = user.HomeDir
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if cwd == nil {
		h.focusProxy.Target = h.focusHandler()
		h.initTabs(cfg, workspacesBarHeight,
			workspacesBarOffset, workspacesBarFrame)
		return nil
	}

	// AddWorkspace is idempotent, so it should be fine to here and later when
	// actually creating the workspace handler.
	tempcwd, err := h.workspace.AddWorkspace(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("add new workspace for %q: %s", *cwd, err)
	}
	var uris []workspaceapi.URI
	for _, filename := range filenames {
		uri, err := tempcwd.URI(filename)
		if err != nil {
			return fmt.Errorf("make file %q uri: %w", filename, err)
		}
		uris = append(uris, uri)
	}

	shouldRestore := len(uris) == 0
	err = h.addWorkspace(*cwd, recfilename, uris, shouldRestore, !cfg.autoRestore(), -1)
	if err != nil {
		return fmt.Errorf("add default workspace: %w", err)
	}

	h.focusProxy.Target = h.focusHandler()
	h.initTabs(cfg, workspacesBarHeight,
		workspacesBarOffset, workspacesBarFrame)
	return nil
}

func (h *workspaceManagerHandler) focusHandler() tui.Handler {
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler
	}

	return h.empty
}

func (h *workspaceManagerHandler) focusBrowser() browser.Browser {
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler.Browser()
	}
	return h.empty.Browser()
}

func (h *workspaceManagerHandler) drawBar() bool {
	return (h.workspaceCount > 1 || h.focusHandler() == h.empty) &&
		h.workspaceBarKind != workspaceBarKindDisabled
}

func (h *workspaceManagerHandler) barSize() int {
	if h.workspacesBarHeight != 0 {
		return h.workspacesBarHeight
	}
	ret := 1
	if h.frame {
		ret += 2
	}
	return ret
}

func (h *workspaceManagerHandler) makeWorkspaceTabName(
	i int, w *workspaceHandler,
) string {
	switch h.workspaceBarKind {
	case workspaceBarKindDisabled:
		return ""
	case workspaceBarKindNumbers:
		return strconv.Itoa(i + 1)
	case workspaceBarKindPaths:
	}
	if w == nil {
		return strconv.Itoa(i + 1)
	}
	if w.uri.Scheme() == workspace.FileScheme && h.userHome != "" {
		path := w.uri.Path()
		path = strings.ReplaceAll(path, h.userHome, "~")
		return path
	}
	return w.uri.String()
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
			idx := h.bar.Add(rune(int(h.workspacesIcon)+i), h.makeWorkspaceTabName(i, w))
			if i == h.focus {
				barFocusIdx = idx
				if !drawBar {
					w.Resize(width, height)
				}
			}
		} else if i == h.focus {
			idx := h.bar.Add(0, h.makeWorkspaceTabName(i, w))
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
	if h.openPrevFiles != nil && h.width != 0 && h.height != 0 {
		err := h.openPrevSessionFiles(h.openPrevFilesEx, h.openPrevFiles, h.openPrevFilesWin)
		if err != nil {
			// do not notify during a call to Resize
			log.Errorf("restore prev session: %v", err)
		}
		h.openPrevFiles = nil
		h.openPrevFilesEx = nil
		h.openPrevFilesWin = nil
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

func (h *workspaceManagerHandler) switchToWorkspace(i int) bool {
	if i < 0 || i >= workspaceSlots {
		return false
	}
	h.focus = i
	h.focusProxy.Target = h.focusHandler()
	// resize so disappearing bar feature can be implemented
	h.Resize(h.width, h.height)
	return true
}

func (h *workspaceManagerHandler) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type == term.EventMouse && h.drawBar() && ev.MouseY >= h.height-h.barSize() {
		_, handled = h.union.Handle(ev)
		return
	}
	focus := h.focusHandler()
	exit, handled = focus.Handle(ev)
	if !exit {
		return h.confirmedForceExit, handled || h.confirmedForceExit
	}

	exHandler := h.exHandler(focus)
	if exHandler.forceExit || h.confirmedForceExit {
		return true, true
	}

	if !h.exitPromptOpen {
		hasDirtyFilesOpen := h.history.dirtyFilesOpen()
		h.exitPromptOpen = true
		h.openConfirmExitPrompt(exHandler, hasDirtyFilesOpen)
		h.shaderRunner.runShutdownShader()
	}

	exHandler.forceExit = false
	exHandler.exit = false

	return false, true

}

func (h *workspaceManagerHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return h.focusHandler().Cursor()
}

func (h *workspaceManagerHandler) Selection() (string, bool) {
	return h.focusHandler().Selection()
}

func (h *workspaceManagerHandler) Man() tui.Manual {
	return h.focusHandler().Man()
}

func (h *workspaceManagerHandler) initExtensions(manager extension.Runner, cfg ideConfig) {
	var wg sync.WaitGroup
	userExtensions := cfg.extensions()

	wg.Add(len(userExtensions) + len(h.builtinExtensions))

	for id, p := range h.builtinExtensions {
		pconfig := p.Config
		if pconfig == nil {
			pconfig = config.MapConfig(make(map[string]interface{}))
		}
		path := p.Path
		id := id
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			err := manager.Run(id, path, pconfig)
			if err != nil {
				log.Errorf("failed to run built-in extension with id %q: %v", id, err)
			}
		})
	}

	for id, p := range userExtensions {
		path, _ := p.path()
		pconfig, ok := p.config()
		if !ok {
			pconfig = config.MapConfig(make(map[string]interface{}))
		}
		id := id
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			err := manager.Run(id, path, pconfig)
			if err != nil {
				log.Errorf("failed to run extension with id %q: %v", id, err)
			}
		})
	}

	wg.Wait()
}

func (h *workspaceManagerHandler) textOpts(cfg ideConfig) []text.Option {
	ret := []text.Option{
		text.WithTabspaces(cfg.editorTabspaces()),
		text.WithWindowManagerConfig(cfg.windowManagerConfig()),
		text.WithFrameUnionCharSet(cfg.frameUnionCharset()),
		text.WithFrameUnion(cfg.frameUnion()),
		text.WithTabsClickCallback(h.tabsClickCallback),
		text.WithCommandKey(cfg.commandKey()),
		text.WithCommandMaxHistory(cfg.commandMaxHistory()),
		text.WithNotifications(h.notifications),
		text.WithFocusTabAttr(cfg.focusTabAttr()),
		text.WithNonFocusTabAttr(cfg.nonFocusTabAttr()),
		text.WithWallpaper(cfg.wallpaper()),
		text.WithDirtyTabAttr(cfg.dirtyTabAttr()),
		text.WithIconSet(cfg.icons()),
		text.WithCommandOverlayConfig(cfg.commandOverlayConfig()),
		text.WithCommandAliases(cfg.commandAliases()),
		text.WithPromptConfig(cfg.promptConfig()),
		text.WithEventPublisher(h.publishEvent),
		text.WithTabBarOffset(h.tabBarOffset),
		text.WithTabBarHeight(h.tabBarHeight),
		text.WithTabNameSeparator(cfg.tabNameSeparator()),
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

// we have no conrol over what extensions are defining in configuration;
// it could be secret keys or anything worth stealing for a malicious extension
// that gets granted extensionapi.PermissionConfig.
func cleanedExtensionConfig(cfg map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		if k != "extensions" {
			m[k] = v
		}
	}
	return m
}

func (h *workspaceManagerHandler) addWorkspace(
	uri workspaceapi.URI, recfilename string, filenames []workspaceapi.URI,
	shouldRestore, promptRecommended bool, i int,
) error {
	for i, w := range h.workspaces {
		if w == nil {
			continue
		}
		if w.uri.Equal(uri) {
			h.switchToWorkspace(i)
			return nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cwd, err := h.workspace.AddWorkspace(ctx, uri)
	if err != nil {
		cancel()
		return fmt.Errorf("create new workspace for %q: %w", uri, err)
	}

	cfg, configErr := h.reloadConfig()
	_, wConfigErr := loadWorkspaceConfig(h.workspaceConfigFilename, cwd, uri, &cfg)
	if wConfigErr != nil {
		configErr = multierror.Append(configErr, fmt.Errorf("workspace config: %w", wConfigErr))
	}

	textOpts := h.textOpts(cfg)
	if recfilename != "" {
		recFile, err := cwd.URI(recfilename)
		if err != nil {
			cancel()
			return fmt.Errorf("cwd make uri for recovery file: %w", err)
		}
		textOpts = append(textOpts, text.WithRecoveryFile(recFile))
	}

	for _, uri := range filenames {
		textOpts = append(textOpts, text.WithFile(uri))
	}

	ed, err := h.newEditor(cfg)
	if err != nil {
		cancel()
		return fmt.Errorf("new editor: %w", err)
	}

	// workspace capable of opening URIs other than the workspaceapi.URI
	multicwd := workspace.Multi(ctx, h.workspace, cwd, uri)
	ex, err := newEx(ed, multicwd, h.storage, h.notifications.notifier,
		cfg.terminalConfig(), h.publishEvent, cfg.clipboard(), textOpts...)
	if err != nil {
		cancel()
		return fmt.Errorf("new ex: %w", err)
	}
	if err := h.subscribeAllCommands(ex); err != nil {
		cancel()
		return err
	}

	go debug.CapturePanicReport(func() {
		start := time.Now()
		// to preserve the order of events we don't want to spawn
		// multiple workers so make the buffer sufficiently large
		// so we don't block the fs subsystem, even in large
		// monorepos with large git operations
		ch := make(chan workspaceapi.EventInfo, 8192)
		watchPath := filepath.Join(uri.Path(), "...")
		watchID, err := cwd.Watch(watchPath, ch,
			workspaceapi.Create, workspaceapi.Write,
			workspaceapi.Remove, workspaceapi.Rename)
		if err != nil {
			ex.log(log.WarnLevel, "oob file monitoring: create FS event watcher: %v", err)
			return
		}
		defer cwd.StopWatch(watchID) //nolint:errcheck

		ex.log(log.InfoLevel, "created FS event watcher in %s", time.Since(start))

		ignores, err := vctrl.LoadGitignore(cwd)
		if err != nil {
			ex.log(log.ErrorLevel, "load excludes for filesystem event matching: %v", err)
			ignores = vctrl.NopMatcher(false)
		}

		dispatchFilesystemEvents(ctx, ex, h.mu, ch, ignores)
	})

	wh := &workspaceHandler{
		cancelCtx: cancel,
		uri:       uri,
		ex:        ex,
	}

	// load async to speed up workspace initialization
	go debug.CapturePanicReport(func() {
		runner, err := h.buildExtensions(cfg, uri, cwd, ex)
		if err != nil {
			log.Errorf("build extensions for workspace %s: %v", uri.String(), err)
			return
		}
		wh.Extensions.Store(runner)
		h.initExtensions(runner, cfg)
	})

	if i == -1 {
		var ok bool
		i, ok = h.nextAvailableWorkspace()
		if !ok {
			cancel()
			return fmt.Errorf("no available workspaces")
		}
	}

	h.workspaces[i] = wh
	h.workspaceCount++
	h.switchToWorkspace(i)

	h.logNonFatalErrs(wh.Browser(), configErr, cfg.errors)

	prevSessionFiles := h.history.recordAddWorkspace(uri, ex.Editor(), shouldRestore)
	if !shouldRestore || len(prevSessionFiles) == 0 {
		return nil
	}

	if promptRecommended && !cfg.autoRestore() {
		h.openRestorePrompt(ex, uri, prevSessionFiles)
		return nil
	}

	if h.width == 0 || h.height == 0 {
		// if restoreSession is called on an size 0,0 handler
		// then cursor is not properly set.
		h.openPrevFiles = prevSessionFiles
		h.openPrevFilesWin = ex.invokeWindow()
		h.openPrevFilesEx = ex
		return nil
	}

	return h.openPrevSessionFiles(ex, prevSessionFiles, ex.invokeWindow())
}

func (h *workspaceManagerHandler) buildExtensions(
	cfg ideConfig, uri workspaceapi.URI,
	cwd workspace.Workspace, ex *ex,
) (extension.Runner, error) {
	res := extension.BrowserResources(ex.Browser(), h.publishEvent)
	res = extension.MergeResourceMap(res,
		extension.EditorResources(ex.Browser(), ex.Editor(), h.publishEvent))
	res = extension.MergeResourceMap(res,
		extension.WorkspaceResources(cwd))
	res = extension.MergeResourceMap(res,
		extension.StorageResources(h.sixDir))
	res = extension.MergeResourceMap(res,
		extension.ConfigResources(config.MapConfig(cleanedExtensionConfig(cfg.cfg))))

	dataDir := h.sixDir
	if err := os.MkdirAll(dataDir, 0777); err != nil {
		return nil, fmt.Errorf("mkdir .extension: %v", err)
	}
	runner, err := h.extensionRunner.WorkspaceExtensionsRunner(uri, res, dataDir, ex.Browser())
	if err != nil {
		return nil, fmt.Errorf("error initializing extension manager: %v", err)
	}
	return runner, nil
}

func (h *workspaceManagerHandler) addOrCreateWorkspace(
	uri workspaceapi.URI,
) error {
	err := h.addWorkspace(uri, "", nil, true, true, -1)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		h.openCreateWorkspacePrompt(h.exHandler(h.focusHandler()), uri)
		return nil
	}
	return err
}

func (h *workspaceManagerHandler) openPrevSessionFiles(
	ex *ex, files []file, invokeWindow browser.Window,
) (err error) {
	for _, f := range files {
		uri, uerr := workspaceapi.ParseURI(f.URIString)
		// do not hard error, otherwise changes to storage representation
		// could prevent user from opening editor at all
		if uerr != nil {
			log.Warnf("parse uri from previous session file: %v", uerr)
			continue
		}
		t, ferr := ex.editFileURI(uri, invokeWindow, false)
		if ferr != nil {
			err = multierror.Append(err, ferr)
			continue
		}
		ed := t.Handler().(text.Handler)
		if serr := ex.Editor().SetCursor(ed, f.Cursor); serr != nil {
			log.Warnf("set cursor %v: %v", f.Cursor, serr)
			continue
		}
	}
	return err
}

func (h *workspaceManagerHandler) nextAvailableWorkspace() (idx int, ok bool) {
	for i := h.focus; i >= 0; i++ {
		if h.workspaces[i] == nil {
			ok = true
			idx = i
			return
		}
	}
	for i := 0; i < h.focus; i++ {
		if h.workspaces[i] == nil {
			ok = true
			idx = i
			return
		}
	}
	return
}

func (h *workspaceManagerHandler) logNonFatalErrs(
	browser browser.Browser,
	configErr error,
	configErrs map[string]error,
) {
	all := configErr
	for key, err := range configErrs {
		err = fmt.Errorf("load %q: %v", key, err)
		all = multierror.Append(all, err)
	}
	if all != nil {
		log.Warn(all)
		_, _ = browser.Notify(notifications.LevelError, "Config decode error: %v", all)
	}
}

func (h *workspaceManagerHandler) commandReloadWorkspace(args ...string) error {
	i := h.focus
	workspaceURI, _, err := h.closeWorkspace()
	if err != nil {
		return err
	}
	return h.addWorkspace(workspaceURI, "", nil, true, false, i)
}

func (h *workspaceManagerHandler) commandAddWorkspace(args ...string) error {
	if len(args) == 0 {
		tempDir := os.TempDir()
		uri, err := h.homeWorkspace.URI(tempDir)
		if err != nil {
			return fmt.Errorf("make uri %s: %v", tempDir, err)
		}
		args = append(args, uri.String())
	}
	path := args[0]

	uri, parseErr := workspaceapi.ParseURI(path)
	if parseErr == nil {
		return h.addOrCreateWorkspace(uri)
	}

	uri, pathErr := workspaceapi.CurrentUserHostURI(path)
	if pathErr != nil {
		err := multierror.Append(pathErr, parseErr)
		return err
	}
	return h.addOrCreateWorkspace(uri)
}

func (h *workspaceManagerHandler) closeWorkspace() (workspaceapi.URI, []workspaceapi.URI, error) {
	if h.focusHandler() == h.empty {
		return workspaceapi.URI{}, nil, errors.New("workspace tab is empty")
	}

	focus := h.focus
	hm := h.workspaces[focus]
	uri := hm.uri
	tabs := hm.ex.comp.Tabs()
	var files []workspaceapi.URI
	for _, tab := range tabs {
		// only reload with tabs that were created by workspace
		_, ok := tab.Closer().(workspace.FlusherCloser)
		uri := tab.URI()
		if ok && uri != (workspaceapi.URI{}) && uri.Scheme() != "" {
			files = append(files, uri)
		}
	}

	h.history.recordCloseWorkspace(uri)
	err := hm.Close()
	if err != nil {
		log.Error(err)
	} else {
		log.Debugf("Closed all workspace resources successfully")
	}

	h.workspaces[focus] = nil
	h.workspaceCount--

	for i := h.focus; i >= 0; i-- {
		if h.workspaces[i] != nil {
			h.switchToWorkspace(i)
			return uri, files, nil
		}
	}

	// for resize of current workspace with empty
	h.switchToWorkspace(h.focus)

	return uri, files, nil
}

func (h *workspaceManagerHandler) commandCloseWorkspace(args ...string) error {
	_, _, err := h.closeWorkspace()
	return err
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
		if err := hm.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if err := h.empty.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if err := h.notifications.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if runner := h.homeRunner.Load(); runner != nil {
		if err := runner.(io.Closer).Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if err := h.storage.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	return
}

type workspaceHandler struct {
	*ex
	cancelCtx  func()
	uri        workspaceapi.URI
	Extensions atomic.Value
}

func (hm *workspaceHandler) Close() (ret error) {
	if err := hm.ex.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if runner := hm.Extensions.Load(); runner != nil {
		if err := runner.(io.Closer).Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	// cancel at the end, so fs event processing is not
	// vacated before everything else is still potentially
	// sending events (i.e. mem scheme)
	hm.cancelCtx()
	return
}

func (h *workspaceManagerHandler) initTabs(
	cfg ideConfig, workspacesBarHeight, workspacesBarOffset int,
	workspacesBarFrame bool,
) {
	h.workspacesBarHeight = workspacesBarHeight
	h.bar.SetBorder(workspacesBarFrame)
	h.bar.SetNameSeparator(cfg.tabNameSeparator())
	var bar tui.Handler = &h.bar
	if workspacesBarOffset != 0 {
		v := new(handler.Virtual)
		v.C = &h.bar
		v.Move(term.Coordinates{X: workspacesBarOffset})
		bar = v
	}
	h.union.UnionBottomFrame(bar, h.barSize(), workspacesBarFrame)
}

func (h *workspaceManagerHandler) subscribeAllCommands(ex *ex) error {
	err := ex.subscribeCommands()
	if err != nil {
		return fmt.Errorf("subscribe ex commands: %w", err)
	}
	err = h.subscribeActiveWorkspaceCommands(ex)
	if err != nil {
		return fmt.Errorf("subscribe workspace commands: %w", err)
	}
	err = h.subscribeAllExternalCommands(ex)
	if err != nil {
		return fmt.Errorf("subscribe external commands: %w", err)
	}
	err = h.pkgmanager.subscribeCommands(ex)
	if err != nil {
		return fmt.Errorf("subscribe idepkg commands: %w", err)
	}
	return nil
}

type commandAllWorkspace struct {
	man     textapi.CommandManual
	handler func(*workspaceManagerHandler, ...string) error
}

func (h *workspaceManagerHandler) subscribeActiveWorkspaceCommands(ex *ex) (ret error) {
	workspaceActiveCommands := map[string]commandAllWorkspace{
		cmdAddWorkspace: {
			handler: (*workspaceManagerHandler).commandAddWorkspace,
			man: textapi.CommandManual{
				Summary: "Opens a new workspace as defined by the given URI, in the current " +
					"workspace slot if its empty, or in the next available slot if it's not. " +
					"If no scheme is present in the URI, file:// is assumed.",
				Synopsis: "[scheme:][//[userinfo@]host][/]workspacepath",
			},
		},
		cmdCloseWorkspace: {
			handler: (*workspaceManagerHandler).commandCloseWorkspace,
			man: textapi.CommandManual{
				Summary: "Closes the current active workspace and switches " +
					"the focus to the previous workspace.",
			},
		},
		cmdReloadWorkspace: {
			handler: (*workspaceManagerHandler).commandReloadWorkspace,
			man: textapi.CommandManual{
				Summary: "Reloads the current active workspace, along with all the extensions.",
			},
		},
		cmdSwitchToWorkspace: {
			handler: (*workspaceManagerHandler).commandSwitchToWorkspace,
			man: textapi.CommandManual{
				Summary:  "Switches the current active workspace to the workspace at the given position.",
				Synopsis: "(1|2|3|4|5|6|7|8|9)",
			},
		},
	}
	return h.subscribeInternalCommands(ex, workspaceActiveCommands)
}

func (h *workspaceManagerHandler) subscribeInternalCommands(
	ex *ex,
	commands map[string]commandAllWorkspace,
) (ret error) {
	for cmd, man := range commands {
		man := man
		cmd := cmd
		man.man.Name = cmd
		err := ex.comp.SubscribeCommand(man.man, text.FuncCommandHandler(
			func(ctx context.Context, cmd textapi.Command) error {
				return man.handler(h, cmd.Args...)
			}, func(ctx context.Context, name string, args []string) (
				iterator.Iterator[string], string, error,
			) {
				return h.completeCommand(ctx, name, args)
			}))
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("subscribe command '%s': %w", cmd, err))
		}
	}
	return ret
}

func (h *workspaceManagerHandler) completeCommand(
	ctx context.Context, cmd string, args []string,
) (iterator.Iterator[string], string, error) {
	switch cmd {
	case cmdSwitchToWorkspace:
		if len(args) <= 1 {
			nums := [10]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
			return iterator.FromSlice(nums[:]), "", nil
		}
		return iterator.FromSlice[string](nil), "", nil
	default:
		return iterator.FromSlice[string](nil), "", nil
	}
}

func (h *workspaceManagerHandler) subscribeAllExternalCommands(ex *ex) (ret error) {
	var cmds []externalCommand
	for _, cmd := range h.externalCommands {
		cmds = append(cmds, cmd)
	}
	return h.subscribeExternalCommands(ex, cmds...)
}

func (h *workspaceManagerHandler) subscribeExternalCommands(
	ex *ex, commands ...externalCommand,
) (ret error) {
	for _, cmd := range commands {
		err := ex.comp.SubscribeCommand(cmd.cmd, cmd.handler)
		if err != nil {
			ret = multierror.Append(ret,
				fmt.Errorf("subscribe command '%s': %v", cmd.cmd.Name, err))
		}
	}
	return ret
}

type externalCommand struct {
	cmd     textapi.CommandManual
	handler text.CommandHandler
}

func (h *workspaceManagerHandler) subscribeCommand(
	cmd textapi.CommandManual, handler text.CommandHandler,
) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.externalCommands[cmd.Name]; ok {
		return fmt.Errorf("command '%s' already registered", cmd.Name)
	}

	extCmd := externalCommand{cmd: cmd, handler: handler}

	// subscribe in current workspaces
	ret := h.subscribeExternalCommands(h.empty, extCmd)
	for _, w := range h.workspaces {
		if w == nil {
			continue
		}
		if err := h.subscribeExternalCommands(w.ex, extCmd); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if ret != nil {
		return ret
	}

	// store for future workspaces
	h.externalCommands[cmd.Name] = extCmd
	return nil
}

func (h *workspaceManagerHandler) exHandler(focus tui.Handler) *ex {
	if ex, ok := focus.(*ex); ok {
		return ex
	}
	if wh, ok := focus.(*workspaceHandler); ok {
		return wh.ex
	}

	panic("unknown focus handler")

}
