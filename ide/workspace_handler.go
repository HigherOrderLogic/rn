// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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
	"maps"
	"net/url"
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
	"github.com/unstablebuild/blue/ide/idedebug"
	"github.com/unstablebuild/blue/ide/idelsp"
	"github.com/unstablebuild/blue/ide/idelsp/lspcmd"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/blue/tui/component/markdown"
	handlermarkdown "github.com/unstablebuild/blue/tui/handler/markdown"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	handlerapi "github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/ide/vctrl/gogit"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

const (
	cmdSwitchToWorkspace = "workspacefocus"
	cmdMoveWorkspace     = "workspacemove"
	cmdCloseWorkspace    = "workspaceclose"
	cmdReloadWorkspace   = "workspacereload"
	cmdAddWorkspace      = "workspacenew"
	cmdRenameWorkspace   = "workspacerename"
	workspaceSlots       = 9
)

var (
	defaultModalCommandKey    = term.KeyComb{Ch: ':'}
	defaultModelessCommandKey = term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrl}
)

var _ text.EventPublisher = (*workspaceManagerHandler)(nil)

type workspaceManagerHandler struct {
	mu                 sync.Locker
	pkgmanager         *pkgManager
	exitPromptOpen     bool
	scheduleNextTick   func(func()) bool
	confirmedForceExit bool
	notifications      *notisManager
	storage            document.Service
	workspace          workspace.WorkspaceManager
	publishEvent       func(term.Event) bool
	tabsClickCallback  func(int) bool
	extensionRunner    ExtensionsRunner
	sixDir             string
	configPath         string
	frameCharSet       component.FrameCharSet
	tabBarOffset       int
	tabBarHeight       int
	builtinExtensions  map[string]Extension
	// this is the name of of the file to be expected in workspace folders
	workspaceConfigFilename string
	tabAttentionNameSuffix  string
	workspaceBarKind        workspaceBarKind
	userHome                string
	history                 *history
	workspacesBarHeight     int
	workspacesIcon          rune
	externalCommands        map[string]externalCommand
	externalEvents          []externalEvents
	initialVTECapacity      int
	dispatchOnPreview       map[string]PreviewFunc
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
	homeURI          workspaceapi.URI
	homeWorkspace    workspace.Workspace
	empty            *ex
	homeRunner       extension.Runner
	openPrevFiles    []file
	openPrevFilesEx  *ex
	openPrevFilesWin browser.Window
	shaderRunner     *shaderRunner
}

func (h *workspaceManagerHandler) newEditor(
	cwd workspaceapi.URI, cfg ideConfig, svc vctrl.Service,
) (
	text.Editor, error,
) {
	switch cfg.editorMode() {
	case editorModeModal:
		return h.newBuiltinModalEditor(cwd, cfg, svc), nil
	case editorModeModeless:
		return h.newBuiltinModelessEditor(cwd, cfg, svc), nil
	default:
		panic("invalid editor mode")
	}
}

func (h *workspaceManagerHandler) newBuiltinModalEditor(
	cwd workspaceapi.URI, cfg ideConfig, svc vctrl.Service,
) text.Editor {
	auxBarConfig := cfg.auxiliaryBarConfig(h, svc)
	gitBarConfig := cfg.gitBarConfig(h)
	statusBarConfig := cfg.statusBarConfig(cwd, h, svc)
	viOpts := append([]vi.Option{},
		vi.WithResAttr(cfg.modalResultAttr()),
		vi.WithTabspaces(cfg.editorTabspaces()),
		vi.WithScheduleNextTick(cfg.scheduleNextTick),
		vi.WithAttr(cfg.modalAttr()),
		vi.WithAuxiliaryBar(cfg.auxiliaryBarEnabled(), auxBarConfig),
		vi.WithStatusBarConfig(cfg.statusBarEnabled(), statusBarConfig),
		vi.WithGitBar(cfg.gitBarEnabled(), gitBarConfig),
		vi.WithHideInitialFolds(cfg.initialFolds()),
		vi.WithClipboard(cfg.clipboard()),
		vi.WithWorkspaceCommandRegistry(cwd, h),
		vi.WithAutoCenter(true),
	)
	return vi.Editor(viOpts...)
}

func (h *workspaceManagerHandler) newBuiltinModelessEditor(
	cwd workspaceapi.URI, cfg ideConfig, svc vctrl.Service,
) text.Editor {
	auxBarConfig := cfg.auxiliaryBarConfig(h, svc)
	gitBarConfig := cfg.gitBarConfig(h)
	statusBarConfig := cfg.statusBarConfig(cwd, h, svc)
	return modeless.Editor(
		modeless.WithCommandBar(true),
		modeless.WithResAttr(cfg.modelessResultAttr()),
		modeless.WithTabspaces(cfg.editorTabspaces()),
		modeless.WithScheduleNextTick(cfg.scheduleNextTick),
		modeless.WithAttr(cfg.modelessAttr()),
		modeless.WithAuxiliaryBar(cfg.auxiliaryBarEnabled(), auxBarConfig),
		modeless.WithGitBar(cfg.gitBarEnabled(), gitBarConfig),
		modeless.WithHideInitialFolds(cfg.initialFolds()),
		modeless.WithClipboard(cfg.clipboard()),
		modeless.WithStatusBarConfig(cfg.statusBarEnabled(), statusBarConfig),
		modeless.WithWorkspaceCommandRegistry(cwd, h),
		modeless.WithAutoCenter(true),
	)
}

func (h *workspaceManagerHandler) init(
	cwd *workspaceapi.URI, homeDirUri workspaceapi.URI,
	manager workspace.WorkspaceManager,
	notiConfig notifications.Config,
	cfg ideConfig, sixDir string, publishEvent func(term.Event) bool,
	extensionRunner ExtensionsRunner, locker sync.Locker,
	builtinExtensions map[string]Extension,
	reloadConfig func() (ideConfig, error), workspaceConfigFilename string,
	tabBarOffset, tabBarHeight int, workspacesIcon rune,
	workspacesBarHeight, workspacesBarOffset int, workspacesBarFrame bool,
	tabsClickCallback func(int) bool,
	releaseManager release.Manager,
	shaderRunner *shaderRunner,
	initialVTECapacity int,
	dispatchOnPreview map[string]PreviewFunc,
) (err error) {
	ctx := context.Background()

	h.storage = localstorage.New(ctx, sixDir, doctoml.Marshaler())
	notiStorage := document.WithPartition(h.storage, "noti")
	interrupter := term.FuncInterrupter(func(ctx context.Context) error {
		payload, _ := term.PayloadFromContext(ctx)
		if !h.publishEvent(term.Event{Type: term.EventInterrupt, Raw: payload, Context: ctx}) {
			return errEventStreamNotReady
		}
		return nil
	})
	h.frameCharSet = cfg.windowFrameCharset()
	notiConfig.Interrupter = interrupter
	h.notifications = newWorkspaceNotifications(notiStorage, notiConfig, h)
	h.shaderRunner = shaderRunner
	h.mu = locker
	h.externalCommands = make(map[string]externalCommand)
	h.frame = cfg.frame()
	h.scheduleNextTick = cfg.scheduleNextTick
	h.configPath = cfg.configPath
	h.reloadConfig = reloadConfig
	h.workspaceConfigFilename = workspaceConfigFilename
	h.tabsClickCallback = tabsClickCallback
	h.publishEvent = publishEvent
	h.workspace = manager
	h.sixDir = sixDir
	h.extensionRunner = extensionRunner
	h.builtinExtensions = builtinExtensions
	h.initialVTECapacity = initialVTECapacity
	h.dispatchOnPreview = dispatchOnPreview
	if h.dispatchOnPreview == nil {
		h.dispatchOnPreview = make(map[string]PreviewFunc)
	}

	h.workspacesIcon = workspacesIcon
	h.tabBarOffset = tabBarOffset
	h.tabBarHeight = tabBarHeight

	homeWorkspace, err := h.workspace.AddWorkspace(ctx, homeDirUri)
	if err != nil {
		return fmt.Errorf("add home workspace: %v", err)
	}

	h.homeURI = homeDirUri
	h.homeWorkspace = homeWorkspace
	h.setReleaseManager(releaseManager)

	// don't install a fs watcher for the home workspace,
	// to prevent unecessary resource consumption
	globalOpts := h.textOpts(h.homeURI, cfg, h.homeWorkspace)
	// do not pass a real version control for home workspace
	ed, err := h.newEditor(homeDirUri, cfg, vctrl.NopService())
	if err != nil {
		return fmt.Errorf("new editor: %v", err)
	}

	tm := new(workspaceTabManager)
	tm.parent = h
	h.empty, err = newEx(ed, homeWorkspace, h.storage, h.notifications, h.homeURI,
		cfg.terminalConfig(), cfg.pluginBarConfig(),
		h.publishEvent, 0 /* vte capacity */, cfg.clipboard(),
		h.dispatchOnPreview, tm, globalOpts...)
	if err != nil {
		return fmt.Errorf("new ex: %w", err)
	}
	tm.tm = h.empty.Browser()
	if err = h.subscribeAllCommands(h.empty); err != nil {
		return err
	}
	if err = h.subscribeAllEvents(h.empty); err != nil {
		return err
	}

	runner, err := h.buildExtensions(cfg, homeDirUri, h.homeWorkspace, h.empty)
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Error building channel for extensions and plugins: %v", err)
		log.Errorf("build home workspace extensions: %v", err)
	} else {
		exec, isExecutor := runner.(schemeapi.Executor)
		if isExecutor {
			h.empty.setExecutor(exec)
		}
		h.homeRunner = runner
		// speed up initialization by initializing extensions asynchronously
		go debug.CapturePanicReport(func() { h.initExtensions(runner, cfg) })
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

	err = h.addWorkspace(*cwd, true, !cfg.autoRestore(), -1)
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

func (h *workspaceManagerHandler) focusURI() workspaceapi.URI {
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler.uri
	}
	return h.homeURI
}

func (h *workspaceManagerHandler) setWorkspaceRequiresAttention(
	uri workspaceapi.URI, attr term.Attributes,
) {
	for _, w := range h.workspaces {
		if w == nil || w.uri.String() != uri.String() {
			continue
		}
		w.attentionAttr = attr
		h.Resize(h.width, h.height)
		break
	}
}

func (h *workspaceManagerHandler) focusBrowser() browser.Browser {
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler.Browser()
	}
	return h.empty.Browser()
}

func (h *workspaceManagerHandler) focusEx() *ex {
	if handler := h.workspaces[h.focus]; handler != nil {
		return handler.ex
	}
	return h.empty
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
	if w != nil && w.tabname != "" {
		return w.tabname
	}
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
		height = max(0, height-h.barSize())
	}
	var barFocusIdx int
	for i, w := range h.workspaces {
		if w != nil {
			name := h.makeWorkspaceTabName(i, w)
			idx := h.bar.Add(rune(int(h.workspacesIcon)+i), name)
			if w.attentionAttr != (term.Attributes{}) {
				h.bar.SetTabAttr(idx, w.attentionAttr)
				h.bar.SetTabName(idx, name+h.tabAttentionNameSuffix)
			}
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
	// clear attention attributes and propagate focus status
	if i != h.focus {
		if w := h.workspaces[h.focus]; w != nil {
			w.ex.container.PauseAll()
			w.ex.onFocusChange(false)
		} else {
			h.empty.container.PauseAll()
		}
		if w := h.workspaces[i]; w != nil {
			w.attentionAttr = term.Attributes{}
			w.ex.onFocusChange(true)
			w.ex.container.ResumeAll()
		} else {
			h.empty.container.ResumeAll()
		}
	}
	h.focus = i
	h.focusProxy.Target = h.focusHandler()
	// resize for bottom workspace bar to disappear
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
	if exHandler.forceExit || h.confirmedForceExit || (exHandler.exit && h.exitPromptOpen) {
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

func (h *workspaceManagerHandler) initExtensions(manager extension.Runner, cfg ideConfig) {
	var wg sync.WaitGroup
	userExtensions := cfg.extensions()

	wg.Add(len(userExtensions) + len(h.builtinExtensions))

	for id, p := range h.builtinExtensions {
		pconfig := p.Config
		if pconfig == nil {
			pconfig = config.MapConfig(make(map[string]any))
		}
		cmdAndArgs := p.CmdAndArgs
		id := id
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			err := manager.Run(id, cmdAndArgs, pconfig)
			if err != nil {
				log.Errorf("failed to run built-in extension with id %q: %v", id, err)
			}
		})
	}

	for id, p := range userExtensions {
		path, _ := p.path()
		pconfig, ok := p.config()
		if !ok {
			pconfig = config.MapConfig(make(map[string]any))
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

func (h *workspaceManagerHandler) textOpts(
	uri workspaceapi.URI, cfg ideConfig, workspace workspace.Workspace,
) []text.Option {
	markdownConfig := markdown.DefaultConfig()
	markdownConfig.Parser = syntax.NewParser(workspace, h.pkgmanager, uri)
	markdownConfig.ScheduleNextTick = cfg.scheduleNextTick
	ret := []text.Option{
		text.WithTabspaces(cfg.editorTabspaces()),
		text.WithWindowManagerConfig(cfg.windowManagerConfig()),
		text.WithFrameUnionCharSet(cfg.frameUnionCharset()),
		text.WithFrameUnion(cfg.frameUnion()),
		text.WithTabsClickCallback(h.tabsClickCallback),
		text.WithCommandKey(cfg.commandKey()),
		text.WithCommandMaxHistory(cfg.commandMaxHistory()),
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
		text.WithPackageManager(h.pkgmanager),
		text.WithSyntaxConfig(cfg.syntaxConfig()),
		text.WithMarkdownConfig(markdownConfig),
		text.WithClipboard(cfg.clipboard()),
	}

	for seq, cmd := range cfg.commandKeyMappings() {
		if seq.Last != (term.KeyComb{}) {
			ret = append(ret, text.WithCommandSequenceBinding(seq, cmd))
		} else {
			ret = append(ret, text.WithCommandKeyBinding(seq.First, cmd))
		}
	}

	if cfg.editorMode() == editorModeModal {
		for seq, cmd := range vi.KeyBindings() {
			if seq.Last != (term.KeyComb{}) {
				ret = append(ret, text.WithCommandSequenceBinding(seq, cmd))
			} else {
				ret = append(ret, text.WithCommandKeyBinding(seq.First, cmd))
			}
		}
	}

	return ret
}

// we have no conrol over what extensions are defining in configuration;
// it could be secret keys or anything worth stealing for a malicious extension
// that gets granted extensionapi.PermissionConfig.
func cleanedExtensionConfig(cfg map[string]any) map[string]any {
	m := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if k != "extensions" {
			m[k] = v
		}
	}
	return m
}

func (h *workspaceManagerHandler) addWorkspace(
	uri workspaceapi.URI, shouldRestore, promptRecommended bool, i int,
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

	textOpts := h.textOpts(uri, cfg, cwd)
	vctrlService := vctrl.NopService()
	if cfg.auxiliaryBarGit() || cfg.gitBarEnabled() {
		vctrlService, err = gogit.NewService(uri, cwd)
		if err != nil {
			h.empty.log(log.ErrorLevel, "new git service for workspace %q: %v",
				uri.Path(), err)
			vctrlService = vctrl.NopService()
		} else {
			vctrlService = vctrl.SyncService(vctrlService, new(sync.Mutex))
		}
	}

	ed, err := h.newEditor(uri, cfg, vctrlService)
	if err != nil {
		cancel()
		return fmt.Errorf("new editor: %w", err)
	}

	// workspace capable of opening URIs other than the workspaceapi.URI
	multicwd := workspace.Multi(ctx, h.workspace, cwd, uri)
	tm := new(workspaceTabManager)
	tm.parent = h
	ex, err := newEx(ed, multicwd, h.storage, h.notifications, uri,
		cfg.terminalConfig(), cfg.pluginBarConfig(), h.publishEvent, h.initialVTECapacity,
		cfg.clipboard(), h.dispatchOnPreview, tm, textOpts...)
	if err != nil {
		cancel()
		return fmt.Errorf("new ex: %w", err)
	}
	tm.tm = ex.Browser()
	if err := h.subscribeAllCommands(ex); err != nil {
		cancel()
		return err
	}
	if err = h.subscribeAllEvents(ex); err != nil {
		cancel()
		return err
	}

	wh := &workspaceHandler{
		vctrlService: vctrlService,
		cancelCtx:    cancel,
		uri:          uri,
		ex:           ex,
	}
	tm.workspace = wh

	go debug.CapturePanicReport(func() {
		start := time.Now()
		// to preserve the order of events we don't want to spawn
		// multiple workers so make the buffer sufficiently large
		// so we don't block the fs subsystem, even in large
		// monorepos with large git operations
		ch := make(chan schemeapi.EventInfo, 8192)
		watchPath := filepath.Join(uri.Path(), "...")
		watchID, err := cwd.Watch(watchPath, ch,
			schemeapi.Create, schemeapi.Write,
			schemeapi.Remove, schemeapi.Rename)
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

	runner, err := h.buildExtensions(cfg, uri, cwd, ex)
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Error building channel for extensions and plugins: %v", err)
		log.Errorf("build extensions for workspace %s: %v", uri.String(), err)
	} else {
		exec, isExecutor := runner.(schemeapi.Executor)
		if isExecutor {
			ex.setExecutor(exec)
		}
		wh.Extensions.Store(runner)
		// load async to speed up workspace initialization
		go debug.CapturePanicReport(func() {
			h.initExtensions(runner, cfg)
		})
	}

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

func lspConfig(cfg ideConfig) config.Config {
	ret := make(map[string]any)
	lspAny, ok := cfg.cfg["lsp"]
	if !ok {
		return config.MapConfig(ret)
	}
	lsp, ok := lspAny.(map[string]any)
	if !ok {
		return config.MapConfig(ret)
	}
	lspCfg := make(map[string]any)
	maps.Copy(lspCfg, lsp)
	ret["lsp"] = lspCfg
	return config.MapConfig(ret)
}

func (h *workspaceManagerHandler) buildExtensions(
	cfg ideConfig, uri workspaceapi.URI,
	cwd workspace.Workspace, ex *ex,
) (extension.Runner, error) {
	notifications := h.notifications.new(uri, ex.container)
	res := extension.BrowserResources(ex.Browser(), h.publishEvent)
	ed := ex.Editor()
	res = extension.MergeResourceMap(res,
		extension.EditorResources(ex.Browser(), ed, h.publishEvent))
	res = extension.MergeResourceMap(res,
		extension.WorkspaceResources(cwd))
	res = extension.MergeResourceMap(res,
		extension.StorageResources(h.sixDir))
	res = extension.MergeResourceMap(res,
		extension.ConfigResources(config.MapConfig(cleanedExtensionConfig(cfg.cfg))))
	parser := syntax.NewParser(ex.workspace, h.pkgmanager, uri)
	res = extension.MergeResourceMap(res,
		extension.SyntaxResources(parser))
	apibrowser := newBrowserAdapter(ex.Browser())
	apieditor := newEditorAdapter(ed)

	lspCallbackCfg := idelsp.CallbackHandlerConfig{
		Config:           lspConfig(cfg),
		ScheduleNextTick: cfg.scheduleNextTick,
	}
	callbacks := idelsp.NewCallbackHandler(notifications, apibrowser, ex.Browser(),
		apieditor, cwd, uri.String(), lspCallbackCfg)
	lspConfig := idelsp.Config{
		NoInitializeServer: true,
		Callback:           callbacks, MaxRetries: 5,
	}
	lsp := idelsp.New(uri, ex.workspace,
		ex.workspace, h.pkgmanager, notifications,
		ex.Browser(), lspConfig)
	dap := idedebug.New(uri, ex.workspace, h.pkgmanager, idedebug.Config{MaxRetries: 5})
	err := ex.comp.SubscribeEvents(idelsp.EditorEvents(), lsp)
	if err != nil {
		log.Errorf("subscribe LSP manager: %v", err)
	}
	cmdcfg := lspCommandsConfig(uri, cfg, notifications, h, parser)
	apiHandler, err := lspcmd.AllHandler(
		lsp, apieditor, apibrowser, apibrowser, apibrowser, ex.workspace, cmdcfg)
	if err != nil {
		return nil, fmt.Errorf("new lsp command handler: %v", err)
	}
	handler := text.FuncCommandHandler(apiHandler.HandleCommand,
		func(ctx context.Context, cmd textapi.Command) (iterator.Iterator[string], string, error) {
			ret, err := apiHandler.Complete(ctx, cmd.Name, cmd.Args)
			return ret, "", err
		})
	err = ex.comp.SubscribeCommand(lspcmd.Manual(), handler)
	if err != nil {
		log.Errorf("subscribe LSP manager: %v", err)
	}
	res = extension.MergeResourceMap(res, extension.SemanticResources(lsp))
	res = extension.MergeResourceMap(res, extension.DebugResources(dap))

	dataDir := h.sixDir
	if err := os.MkdirAll(dataDir, 0777); err != nil {
		return nil, fmt.Errorf("mkdir %s: %v", dataDir, err)
	}
	runner, err := h.extensionRunner.WorkspaceExtensionsRunner(uri, res,
		dataDir, ex.Browser(), cwd)
	if err != nil {
		return nil, fmt.Errorf("new workspace extensions runner: %v", err)
	}
	return runner, nil
}

func (h *workspaceManagerHandler) addOrCreateWorkspace(
	uri workspaceapi.URI,
) error {
	err := h.addWorkspace(uri, true, true, -1)
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
		ed.SetCursorAtScroll(f.Cursor)
	}
	return err
}

func (h *workspaceManagerHandler) nextAvailableWorkspace() (idx int, ok bool) {
	for i := h.focus; i >= 0 && i < len(h.workspaces); i++ {
		if h.workspaces[i] == nil {
			ok = true
			idx = i
			return
		}
	}
	for i := 0; i < h.focus && i < len(h.workspaces); i++ {
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
		_, _ = browser.Notify(browserapi.LevelError, "Config decode error: %v", all)
	}
}

func (h *workspaceManagerHandler) commandReloadWorkspace(args ...string) error {
	i := h.focus
	workspaceURI, _, err := h.closeWorkspace()
	if err != nil {
		return err
	}
	return h.addWorkspace(workspaceURI, true, false, i)
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
		log.Debugf("using path %q as a path of file:// scheme: "+
			"could not parse as uri: %v", path, parseErr)
		return h.addOrCreateWorkspace(uri)
	}

	uri, pathErr := workspaceapi.CurrentUserHostURI(path)
	if pathErr != nil {
		err := multierror.Append(pathErr, parseErr)
		return err
	}
	return h.addOrCreateWorkspace(uri)
}

func (h *workspaceManagerHandler) commandRenameWorkspace(args ...string) error {
	if h.focusHandler() == h.empty {
		return fmt.Errorf("there's no workspace to rename. " +
			"First you must open one via `workspacenew`")
	}
	if len(args) == 0 {
		return fmt.Errorf("expected one argument with the new name")
	}
	name := args[0]
	handler := h.workspaces[h.focus]
	handler.tabname = name
	h.Resize(h.width, h.height)
	return nil
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

func (h *workspaceManagerHandler) moveWorkspace(args ...string) error {
	if len(args) == 0 {
		return errors.New("command expects at least one argument")
	}

	var err error
	var next int
	curr := h.focus
	switch args[0] {
	case "right":
		if curr+1 == workspaceSlots {
			return errors.New("workspace is already at the last slot")
		}
		next = curr + 1
	case "left":
		if curr == 0 {
			return errors.New("workspace is already at the first slot")
		}
		next = curr - 1
	default:
		next, err = strconv.Atoi(args[0])
		if err != nil {
			return errInvalidTab
		}
		// next is 1-indexed
		if next == 0 {
			return errors.New("the first worskpace slot is 1")
		}
		if next > len(h.workspaces) {
			return fmt.Errorf("the last worskpace slot is %d", len(h.workspaces))
		}
		next--
	}
	temp := h.workspaces[curr]
	h.workspaces[curr] = h.workspaces[next]
	h.workspaces[next] = temp
	h.focus = next
	h.Resize(h.width, h.height)
	return err
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
	if h.homeRunner != nil {
		if err := h.homeRunner.(io.Closer).Close(); err != nil {
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
	tabname       string
	attentionAttr term.Attributes
	vctrlService  vctrl.Service
	cancelCtx     func()
	uri           workspaceapi.URI
	Extensions    atomic.Value
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
	if closer, ok := hm.vctrlService.(io.Closer); ok {
		if err := closer.Close(); err != nil {
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
		v := new(handlerapi.Virtual[*handler.Tabs])
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
				Summary: "Opens the workspace at the given URI in the current " +
					"workspace slot if it's empty, or in the next available slot if it's not. " +
					"If no scheme is present in the URI, file:// is assumed.",
				Synopsis: "[scheme:][//[userinfo@]host][/]workspacepath",
			},
		},
		cmdRenameWorkspace: {
			handler: (*workspaceManagerHandler).commandRenameWorkspace,
			man: textapi.CommandManual{
				Summary: "Renames the workspace tab. The tab is displayed at the " +
					"bottom of the screen when multiple workspaces are open.",
				Synopsis: "name",
			},
		},
		cmdCloseWorkspace: {
			handler: (*workspaceManagerHandler).commandCloseWorkspace,
			man: textapi.CommandManual{
				Summary: "Closes the current active workspace and switches focus " +
					"to the previous workspace.",
			},
		},
		cmdReloadWorkspace: {
			handler: (*workspaceManagerHandler).commandReloadWorkspace,
			man: textapi.CommandManual{
				Summary: "Reloads the current active workspace, along with all extensions.",
			},
		},
		cmdSwitchToWorkspace: {
			handler: (*workspaceManagerHandler).commandSwitchToWorkspace,
			man: textapi.CommandManual{
				Summary:  "Switches the current active workspace to the workspace at the given position.",
				Synopsis: "(1|2|3|4|5|6|7|8|9)",
			},
		},
		cmdMoveWorkspace: {
			man: textapi.CommandManual{
				Summary: "Moves the workspace tab in focus in the given direction " +
					"within the tabs list, or to an absolute position if a number is passed.",
				Synopsis: "(right|left|1|2|3|4|5|6|7|8|9)",
			},
			handler: (*workspaceManagerHandler).moveWorkspace,
		},
	}
	return h.subscribeInternalCommands(ex, workspaceActiveCommands)
}

func (h *workspaceManagerHandler) subscribeInternalCommands(
	ex *ex,
	commands map[string]commandAllWorkspace,
) (ret error) {
	for cmd, man := range commands {
		man.man.Name = cmd
		err := ex.comp.SubscribeCommand(man.man, text.FuncCommandHandler(
			func(ctx context.Context, cmd textapi.Command) error {
				return man.handler(h, cmd.Args...)
			}, func(ctx context.Context, cmd textapi.Command) (
				iterator.Iterator[string], string, error,
			) {
				return h.completeCommand(ctx, cmd)
			}))
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("subscribe command '%s': %w", cmd, err))
		}
	}
	return ret
}

func (h *workspaceManagerHandler) completeCommand(
	ctx context.Context, cmd textapi.Command,
) (iterator.Iterator[string], string, error) {
	switch cmd.Name {
	case cmdAddWorkspace:
		return command.DirsCompleter(h.empty.workspace).Complete(ctx, cmd.Args)
	case cmdMoveWorkspace:
		if len(cmd.Args) <= 1 {
			options := []string{"left", "right", "1", "2",
				"3", "4", "5", "6", "7", "8", "9"}
			return iterator.FromSlice(options), "", nil
		}
		return iterator.FromSlice[string](nil), "", nil
	case cmdSwitchToWorkspace:
		var tabNames []string
		if len(cmd.Args) <= 1 {
			for i, h := range h.workspaces {
				if h == nil {
					tabNames = append(tabNames, strconv.Itoa(i+1))
					continue
				}
				pretty := strconv.Itoa(i + 1)
				name := h.uri.String()
				pretty += " " + name
				tabNames = append(tabNames, pretty)
			}
		}
		return iterator.FromSlice(tabNames), "", nil
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

func (h *workspaceManagerHandler) SubscribeCommandForWorkspace(
	uri workspaceapi.URI, cmd textapi.CommandManual, handler text.CommandHandler,
) error {
	extCmd := externalCommand{cmd: cmd, handler: handler}
	if uri.String() == h.homeURI.String() {
		return h.subscribeExternalCommands(h.empty, extCmd)
	}
	for _, w := range h.workspaces {
		if w == nil || w.uri.String() != uri.String() {
			continue
		}
		return h.subscribeExternalCommands(w.ex, extCmd)
	}
	return errors.New("workspace with given uri not found")
}

func (h *workspaceManagerHandler) UnsubscribeCommandForWorkspace(
	uri workspaceapi.URI, name string,
) error {
	if uri.String() == h.homeURI.String() {
		return h.empty.comp.UnsubscribeCommand(name)
	}
	for _, w := range h.workspaces {
		if w == nil || w.uri.String() != uri.String() {
			continue
		}
		return w.ex.comp.UnsubscribeCommand(name)
	}
	return errors.New("command is not registered")
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

type externalEvents struct {
	events  []textapi.EventType
	handler text.EventHandler
}

func (h *workspaceManagerHandler) subscribeExternalEvents(
	ex *ex, evs ...externalEvents,
) (ret error) {
	for _, cmd := range evs {
		err := ex.comp.SubscribeEvents(cmd.events, cmd.handler)
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("subscribe events: %w", err))
		}
	}
	return ret
}

func (h *workspaceManagerHandler) subscribeAllEvents(ex *ex) error {
	return h.subscribeExternalEvents(ex, h.externalEvents...)
}

func (h *workspaceManagerHandler) SubscribeEvents(
	events []textapi.EventType, handler text.EventHandler,
) error {
	extEvt := externalEvents{events: events, handler: handler}

	// subscribe in current workspaces
	ret := h.subscribeExternalEvents(h.empty, extEvt)
	for _, w := range h.workspaces {
		if w == nil {
			continue
		}
		if err := h.subscribeExternalEvents(w.ex, extEvt); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if ret != nil {
		return ret
	}

	// store for future workspaces
	h.externalEvents = append(h.externalEvents, extEvt)
	return nil
}

func (h *workspaceManagerHandler) UnsubscribeEvents(
	handler text.EventHandler,
) (ok bool, ret error) {
	ok, err := h.empty.comp.UnsubscribeEvents(handler)
	if err != nil {
		ret = multierror.Append(ret, err)
	}
	for _, w := range h.workspaces {
		if w == nil {
			continue
		}
		if exOk, err := w.ex.comp.UnsubscribeEvents(handler); err != nil {
			ret = multierror.Append(ret, err)
		} else if !exOk {
			ok = false
		}
	}

	// remove for future workspaces
	for i, extEvt := range h.externalEvents {
		if extEvt.handler == handler {
			h.externalEvents[i] = h.externalEvents[len(h.externalEvents)-1]
			h.externalEvents = h.externalEvents[:len(h.externalEvents)-1]
			return
		}
	}
	ok = false
	return
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

func (h *workspaceManagerHandler) Interrupt(ctx context.Context) error {
	payload, _ := term.PayloadFromContext(ctx)
	ev := term.Event{Type: term.EventInterrupt, Raw: payload, Context: ctx}
	if !h.publishEvent(ev) {
		return errEventStreamNotReady
	}
	return nil
}

func (h *workspaceManagerHandler) setReleaseManager(releaseManager release.Manager) {
	pkgStorage := document.WithPartition(h.storage, "idepkg")
	if h.pkgmanager == nil {
		h.pkgmanager = new(pkgManager)
	} else {
		h.pkgmanager.Close()
	}
	wm := currentWorkspaceWindowManager{root: h}
	parser := &lazyParser{root: h}
	h.pkgmanager.init(h.notifications.current(), releaseManager, wm,
		pkgStorage, h.homeWorkspace, h.sixDir, h.configPath, h.frameCharSet,
		h, h, h.scheduleNextTick, parser)
	h.dispatchOnPreview[cmdPkgInstall] = h.pkgmanager.previewPkgInstall
}

func (h *workspaceManagerHandler) openURI(file workspaceapi.URI, focus bool) error {
	ex := h.exHandler(h.focusHandler())
	var err error
	if focus {
		_, err = ex.editFileURI(file, ex.invokeWindow(), false)
	} else {
		_, err = ex.comp.Open(file)
	}
	if err != nil {
		return err
	}
	return err
}

func (h *workspaceManagerHandler) openFile(file string, focus bool) error {
	ex := h.exHandler(h.focusHandler())
	uri, err := ex.workspace.URI(file)
	if err != nil {
		return err
	}
	return h.openURI(uri, focus)
}

type workspaceTabManager struct {
	workspace *workspaceHandler
	parent    *workspaceManagerHandler
	tm        browser.TabManager
}

func (f *workspaceTabManager) Tab(uri workspaceapi.URI, icon rune, name string, h browserapi.Handler) (
	browserapi.Handler, error,
) {
	if f.tm == nil {
		return nil, errors.New("tab manager is not initialized")
	}
	if uri == (workspaceapi.URI{}) {
		panic("nil uri")
	}
	return f.tm.Tab(uri, icon, name, h)
}

func (f *workspaceTabManager) SetTabName(
	uri workspaceapi.URI, name string, attr term.Attributes,
) error {
	if f.tm == nil {
		return errors.New("tab manager is not initialized")
	}
	f.parent.scheduleNextTick(func() {
		_ = f.tm.SetTabName(uri, name, attr)
		if attr == (term.Attributes{}) {
			log.Debugf("SetTabName called on workspace tab manager %p: "+
				"empty attributes", f)
			return
		}
		// home workspace: can't set workspace tab attributes
		if f.workspace == nil {
			log.Debugf("SetTabName called on workspace tab manager %p: "+
				"home workspace", f)
			return
		}
		// set workspace tab attr as well if workspace not in focus
		if f.parent.focusHandler() != f.workspace {
			log.Debugf("SetTabName called on workspace tab manager %p: "+
				"setting attention attrs", f)
			f.workspace.attentionAttr = attr
			f.parent.Resize(f.parent.width, f.parent.height)
			return
		}
		log.Debugf("SetTabName called on workspace tab manager %p: "+
			"workspace in focus", f)
	})
	return nil
}

func lspCommandsConfig(
	uri workspaceapi.URI, cfg ideConfig, notifications browserapi.Notifications,
	interrupter term.Interrupter, parser syntaxapi.Parser,
) lspcmd.Config {
	cmdcfg := lspcmd.DefaultConfig()
	cmdcfg.RootURI = uri
	cmdcfg.Parser = parser
	cmdcfg.ScheduleNextTick = cfg.scheduleNextTick
	cmdcfg.Highlight.Delay = 50 * time.Millisecond
	cmdcfg.Highlight.WriteAttr = term.Attributes{Attrs: tcell.AttrBold | tcell.AttrUnderline}
	cmdcfg.Highlight.ReadAttr = term.Attributes{Attrs: tcell.AttrUnderline}
	cmdcfg.SignatureHelp.TriggerCharacters = []string{"(", ","}
	cmdcfg.SignatureHelp.AutoTrigger = true
	cmdcfg.Interrupter = interrupter
	cmdcfg.Hover.MarkdownConfig = markdown.DefaultConfig()
	cmdcfg.Hover.MarkdownConfig.Parser = parser
	cmdcfg.Hover.MarkdownConfig.ScheduleNextTick = cfg.scheduleNextTick
	cmdcfg.Hover.MarkdownHandlerOptions = []handlermarkdown.Option{
		handlermarkdown.WithOnLinkClick(func(link *url.URL) bool {
			if link.Scheme != "http" && link.Scheme != "https" {
				return false
			}
			linkstr := link.String()
			meta := clipboard.Data{Text: linkstr}
			err := cfg.clipboard().Copy(clipboard.DefaultRegisterID, meta)
			if err != nil {
				_, _ = notifications.Notify(browserapi.LevelError,
					"copy URL to clipboard: %v", err)
			} else {
				_, _ = notifications.Notify(browserapi.LevelSuccess,
					"copied URL %s to clipboard", linkstr)
			}
			return true
		}),
	}
	return cmdcfg
}
