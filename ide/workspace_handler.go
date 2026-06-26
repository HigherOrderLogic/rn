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
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component"
	handlerapi "github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	handlermarkdown "unstable.build/go-tui/handler/markdown"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/idecursor"
	"unstable.build/go-tui/ide/idedebug"
	"unstable.build/go-tui/ide/idehistory"
	"unstable.build/go-tui/ide/idelsp"
	"unstable.build/go-tui/ide/idelsp/lspcmd"
	"unstable.build/go-tui/ide/idemacro"
	"unstable.build/go-tui/ide/idenotice"
	"unstable.build/go-tui/ide/idepkg"
	"unstable.build/go-tui/ide/ideshell/debugshell"
	"unstable.build/go-tui/ide/ideshell/workspaceshell"
	"unstable.build/go-tui/ide/llmshell"
	"unstable.build/go-tui/ide/pkgshell"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/ide/vctrl/gogit"
	"unstable.build/go-tui/llm/llmrouter"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/exoeditor"
	"unstable.build/go-tui/text/exofallback"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/text/textrpc"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

const (
	cmdSwitchToWorkspace = "workspacefocus"
	cmdMoveWorkspace     = "workspacemove"
	cmdCloseWorkspace    = "workspaceclose"
	cmdReloadWorkspace   = "workspacereload"
	cmdAddWorkspace      = "workspaceopen"
	cmdRenameWorkspace   = "workspacerename"
	cmdWorkspaceReady    = "workspaceready"
	cmdExtensionReady    = "extensionready"
	cmdMacroRecord       = "record"
	workspaceSlots       = 9
)

var (
	defaultModalCommandKey    = term.KeyComb{Ch: ':'}
	defaultModelessCommandKey = term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrl}
)

var _ text.EventPublisher = (*workspaceManagerHandler)(nil)

type workspaceManagerHandler struct {
	mu                      sync.Locker
	pkgmanager              *pkgManager
	exitPromptOpen          bool
	scheduleNextTick        func(func()) bool
	confirmedForceExit      bool
	notifications           *notisManager
	storage                 storageapi.Service
	ideStorage              storageapi.Service
	workspace               workspace.WorkspaceManager
	clip                    clipboard.Register
	macro                   *idemacro.Recorder
	macroPlayer             *idemacro.Player
	events                  *eventRouter
	tabsClickCallback       func(int) bool
	extensionRunner         ExtensionsRunner
	sixDir                  string
	configPath              string
	llmRouter               *llmrouter.Router
	frameCharSet            component.FrameCharSet
	tabBarOffset            int
	tabBarHeight            int
	builtinExtensions       map[string]Extension
	workspaceConfigFilename string
	tabAttentionNameSuffix  string
	workspaceBarKind        workspaceBarKind
	userHome                string
	state                   *idehistory.Store
	workspacesBarHeight     int
	workspacesIcon          rune
	externalCommands        map[string]externalCommand
	externalREPLCommands    map[string]externalREPLCommand
	externalEvents          []externalEvents
	initialVTECapacity      int
	dispatchOnPreview       map[string]previewFunc
	debugCommands           bool
	frame                   bool
	streamingOpen           bool
	reloadConfig            func() (ideConfig, error)

	packageConfigMergeHook func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error)

	// tutorialsInstalled is a required dependency wired by the IDE at
	// construction; afterPackageConfigMerge calls it unconditionally and a nil
	// value is a construction bug that must panic, not be guarded.
	tutorialsInstalled func(names []string) (bool, error)

	commandObserver *commandObserverRegistry

	union               handler.FrameUnion
	bar                 handler.Tabs
	barIdxToSlot        []int
	focusProxy          handler.Proxy
	width, height       int
	workspaces          [workspaceSlots]*workspaceHandler
	workspaceCount      int
	focus               int
	homeURI             workspaceapi.URI
	homeWorkspace       workspace.Workspace
	empty               *ex
	homeRunner          extension.Runner
	homeLSPManager      *idelsp.Manager
	homeDAPManager      *idedebug.Manager
	openPrevFiles       []idehistory.File
	openPrevFilesEx     *ex
	openPrevWindows     map[uint64]browser.Window
	shaderRunner        *shaderRunner
	pending             map[string]*pendingWorkspace
	lastReservedPending *pendingWorkspace
	pendingWG           sync.WaitGroup

	// Fields, not constants, so tests can shorten the extensionready
	// readiness and command-registration timeouts.
	extReadyWait   time.Duration
	extCommandWait time.Duration
	extHandleWait  time.Duration
}

type openFileTarget struct {
	workspaceIdx int
	workspace    *workspaceHandler
	tab          *browser.Tab
}

type pendingWorkspace struct {
	uri       workspaceapi.URI
	slot      int
	cancelCtx func()
	canceled  atomic.Bool
	onReady   [][]string
}

type visibleWorkspaceManager struct {
	parent  *workspaceManagerHandler
	manager workspace.WorkspaceManager
}

func (m visibleWorkspaceManager) AddWorkspace(
	ctx context.Context, uri workspaceapi.URI,
) (workspace.Workspace, error) {
	return m.manager.AddWorkspace(ctx, uri)
}

func (m visibleWorkspaceManager) Workspace(
	file workspaceapi.URI,
) (workspace.Workspace, bool, error) {
	target, ok := m.parent.workspaceForFile(file)
	if !ok || target == nil || target.workspace == nil || target.workspace.ex == nil {
		return nil, false, nil
	}
	return target.workspace.ex.workspace, true, nil
}

func (m visibleWorkspaceManager) RegisterScheme(
	scheme string, fn schemeapi.SchemeFunc,
) error {
	return m.manager.RegisterScheme(scheme, fn)
}

func (m visibleWorkspaceManager) UnregisterScheme(scheme string) error {
	return m.manager.UnregisterScheme(scheme)
}

func (m visibleWorkspaceManager) IncrementReference(uri workspaceapi.URI) {
	m.manager.IncrementReference(uri)
}

func (m visibleWorkspaceManager) DecrementReference(uri workspaceapi.URI) error {
	return m.manager.DecrementReference(uri)
}

func (h *workspaceManagerHandler) newEditor(
	reloader exoeditor.Reloader,
	cwd workspaceapi.URI, ws workspace.Workspace, tm browser.TabManager,
	cfg ideConfig, svc vctrl.Service,
) (text.Editor, error) {
	switch cfg.editorMode() {
	case editorModeModal:
		return h.newBuiltinModalEditor(cwd, cfg, svc), nil
	case editorModeModeless:
		return h.newBuiltinModelessEditor(cwd, cfg, svc), nil
	case editorModeExo:
		return h.newExoFallbackEditor(reloader, cwd, ws, tm, cfg, svc), nil
	default:
		panic("invalid editor mode")
	}
}

func (h *workspaceManagerHandler) newBuiltinModalEditor(
	cwd workspaceapi.URI, cfg ideConfig, svc vctrl.Service,
) text.Editor {
	auxBarConfig := cfg.auxiliaryBarConfig(h, svc)
	iconsBarConfig := cfg.iconsBarConfig(h)
	statusBarConfig := cfg.statusBarConfig(cwd, h, svc)
	viOpts := append([]vi.Option{},
		vi.WithResAttr(cfg.modalResultAttr()),
		vi.WithTabspaces(cfg.editorTabspaces()),
		vi.WithIndents(cfg.editorIndents()),
		vi.WithRuler(cfg.editorRuler()),
		vi.WithAutoPair(cfg.editorAutoPair()),
		vi.WithComments(cfg.editorComments()),
		vi.WithScheduleNextTick(cfg.scheduleNextTick),
		vi.WithAttr(cfg.modalAttr()),
		vi.WithAuxiliaryBar(cfg.auxiliaryBarEnabled(), auxBarConfig),
		vi.WithIconsBar(cfg.iconsBarEnabled(), iconsBarConfig),
		vi.WithGitIcons(cfg.gitIconsEnabled()),
		vi.WithStatusBarConfig(cfg.statusBarEnabled(), statusBarConfig),
		vi.WithHideInitialFolds(cfg.initialFolds()),
		vi.WithClipboard(h.clip),
		vi.WithMacroRecorder(h.macro),
		vi.WithMacroPlayer(h.macroPlayer),
		vi.WithWorkspaceCommandRegistry(cwd, h),
		vi.WithAutoCenter(true),
		vi.WithWindowManager(currentWorkspaceWindowManager{root: h}),
		vi.WithNotifications(h.notifications.current()),
	)
	return vi.Editor(viOpts...)
}

func (h *workspaceManagerHandler) newBuiltinModelessEditor(
	cwd workspaceapi.URI, cfg ideConfig, svc vctrl.Service,
) text.Editor {
	auxBarConfig := cfg.auxiliaryBarConfig(h, svc)
	iconsBarConfig := cfg.iconsBarConfig(h)
	statusBarConfig := cfg.statusBarConfig(cwd, h, svc)
	return modeless.Editor(
		modeless.WithCommandBar(true),
		modeless.WithResAttr(cfg.modelessResultAttr()),
		modeless.WithTabspaces(cfg.editorTabspaces()),
		modeless.WithIndents(cfg.editorIndents()),
		modeless.WithRuler(cfg.editorRuler()),
		modeless.WithAutoPair(cfg.editorAutoPair()),
		modeless.WithComments(cfg.editorComments()),
		modeless.WithScheduleNextTick(cfg.scheduleNextTick),
		modeless.WithAttr(cfg.modelessAttr()),
		modeless.WithAuxiliaryBar(cfg.auxiliaryBarEnabled(), auxBarConfig),
		modeless.WithIconsBar(cfg.iconsBarEnabled(), iconsBarConfig),
		modeless.WithGitIcons(cfg.gitIconsEnabled()),
		modeless.WithHideInitialFolds(cfg.initialFolds()),
		modeless.WithClipboard(h.clip),
		modeless.WithMacroRecorder(h.macro),
		modeless.WithMacroPlayer(h.macroPlayer),
		modeless.WithStatusBarConfig(cfg.statusBarEnabled(), statusBarConfig),
		modeless.WithWorkspaceCommandRegistry(cwd, h),
		modeless.WithAutoCenter(true),
		// See newBuiltinModalEditor for why we route notifications.
		modeless.WithNotifications(h.notifications.current()),
	)
}

func (h *workspaceManagerHandler) newExoFallbackEditor(
	reloader exoeditor.Reloader,
	cwd workspaceapi.URI, ws workspace.Workspace,
	tm browser.TabManager, cfg ideConfig, svc vctrl.Service,
) text.Editor {
	var fallback text.Editor
	switch cfg.exoFallback() {
	case editorFallbackModeless:
		fallback = h.newBuiltinModelessEditor(cwd, cfg, svc)
	default:
		fallback = h.newBuiltinModalEditor(cwd, cfg, svc)
	}
	return exofallback.New(
		cfg.exoCommand(),
		cfg.exoGoto(),
		cfg.exoQuit(),
		cfg.scheduleNextTick,
		ws,
		cwd,
		h.notifications.current(),
		exoeditor.PublisherFunc(h.events.newPublisher(cwd)),
		ws, // terminal
		ws, // executor
		tm,
		cfg.terminalConfig(),
		reloader,
		fallback,
		h.envSource,
		cfg.exoOverrideHighlights(),
		h,
		svc,
		h.clip,
	)
}

// newPromptEditor builds the editor that backs both the command
// prompt's modal edit mode and the companion shell's input line. Both
// edit a single logical line and wrap it through their own responsive
// renderer, so the editor itself stays wrap=false: cursor motions
// (0, $, l, …) traverse the whole command rather than a wrapped
// visual row.
func (h *workspaceManagerHandler) newPromptEditor(
	cfg ideConfig,
) command.Editor {
	switch cfg.pkgEditorMode() {
	case editorModeModeless:
		return modelessPromptEditor{
			tabspaces:        cfg.editorTabspaces(),
			indents:          cfg.editorIndents(),
			scheduleNextTick: cfg.scheduleNextTick,
			clipboard:        h.clip,
			autoPair:         cfg.editorAutoPair(),
		}
	case editorModeModal:
		return viPromptEditor{
			tabspaces:        cfg.editorTabspaces(),
			indents:          cfg.editorIndents(),
			scheduleNextTick: cfg.scheduleNextTick,
			clipboard:        h.clip,
			autoPair:         cfg.editorAutoPair(),
		}
	default:
		panic("invalid editor mode")
	}
}

type modelessPromptEditor struct {
	tabspaces        int
	indents          text.IndentConfig
	scheduleNextTick func(func()) bool
	clipboard        clipboard.Register
	autoPair         bool
}

func (m modelessPromptEditor) Edit(buf *cell.Buffer) command.EditHandler {
	uri := workspaceapi.RandomURI("memory")
	return modeless.NewHandler(buf, uri, text.IndentRuneTab, m.tabspaces,
		modeless.WithCommandBar(false),
		modeless.WithTabspaces(m.tabspaces),
		modeless.WithIndents(m.indents),
		modeless.WithScheduleNextTick(m.scheduleNextTick),
		modeless.WithClipboard(m.clipboard),
		modeless.WithAutoPair(m.autoPair),
		modeless.WithWrap(false),
	)
}

type viPromptEditor struct {
	tabspaces        int
	indents          text.IndentConfig
	scheduleNextTick func(func()) bool
	clipboard        clipboard.Register
	autoPair         bool
}

func (v viPromptEditor) Edit(buf *cell.Buffer) command.EditHandler {
	uri := workspaceapi.RandomURI("memory")
	return vi.NewWithIndent(buf, uri, text.IndentRuneTab, v.tabspaces,
		vi.WithTabspaces(v.tabspaces),
		vi.WithIndents(v.indents),
		vi.WithScheduleNextTick(v.scheduleNextTick),
		vi.WithClipboard(v.clipboard),
		vi.WithAutoPair(v.autoPair),
		vi.WithWrap(false),
	)
}

func (h *workspaceManagerHandler) init(
	cwd *workspaceapi.URI, homeDirUri workspaceapi.URI,
	manager workspace.WorkspaceManager,
	notiConfig notifications.Config,
	cfg ideConfig, storage storageapi.Service, sixDir string,
	publishEvent func(term.Event) bool,
	extensionRunner ExtensionsRunner, locker sync.Locker,
	builtinExtensions map[string]Extension,
	reloadConfig func() (ideConfig, error), workspaceConfigFilename string,
	tabBarOffset, tabBarHeight int, workspacesIcon rune,
	workspacesBarHeight, workspacesBarOffset int, workspacesBarFrame bool,
	tabsClickCallback func(int) bool,
	releaseManager release.Manager,
	shaderRunner *shaderRunner,
	initialVTECapacity int,
	dispatchOnPreview map[string]previewFunc,
	debugCommands bool,
	streamingOpen bool,
	commandObserver *commandObserverRegistry,
) (err error) {
	ctx := context.Background()

	if h.tutorialsInstalled == nil {
		panic("workspaceManagerHandler.init: tutorialsInstalled is required")
	}
	h.storage = storage
	h.ideStorage = storageapi.WithPartition(h.storage, "ide")
	router, err := llmrouter.New(cfg.llmConfig(), sixDir, h.storage)
	if err != nil {
		return fmt.Errorf("init llm router: %w", err)
	}
	h.llmRouter = router
	cfg.storage = h.ideStorage
	h.events = newEventRouter(publishEvent)
	h.frameCharSet = cfg.windowFrameCharset()
	notiConfig.Interrupter = h.events.globalInterrupter()
	h.notifications = newWorkspaceNotifications(h.ideStorage, notiConfig, h)
	h.shaderRunner = shaderRunner
	h.mu = locker
	h.externalCommands = make(map[string]externalCommand)
	h.externalREPLCommands = make(map[string]externalREPLCommand)
	h.frame = cfg.frame()
	h.scheduleNextTick = cfg.scheduleNextTick
	h.configPath = cfg.configPath
	h.reloadConfig = reloadConfig
	h.workspaceConfigFilename = workspaceConfigFilename
	h.tabsClickCallback = tabsClickCallback
	h.workspace = manager
	h.clip = cfg.clipboard()
	h.macro = idemacro.New(h.clip, h.notifications.current(), cfg.commandKey())
	h.macroPlayer = idemacro.NewPlayer(h.clip, h.macro, h.events.globalPublisher())
	h.sixDir = sixDir
	h.extensionRunner = extensionRunner
	h.builtinExtensions = builtinExtensions
	h.initialVTECapacity = initialVTECapacity
	h.dispatchOnPreview = dispatchOnPreview
	if h.dispatchOnPreview == nil {
		h.dispatchOnPreview = make(map[string]previewFunc)
	}

	h.workspacesIcon = workspacesIcon
	h.tabBarOffset = tabBarOffset
	h.tabBarHeight = tabBarHeight
	h.debugCommands = debugCommands
	h.streamingOpen = streamingOpen
	h.commandObserver = commandObserver
	h.pending = make(map[string]*pendingWorkspace)
	h.extReadyWait = extensionReadyWait
	h.extCommandWait = extensionCommandWait
	h.extHandleWait = extensionHandleWait

	homeWorkspace, err := h.workspace.AddWorkspace(ctx, homeDirUri)
	if err != nil {
		return fmt.Errorf("add home workspace: %v", err)
	}

	h.homeURI = homeDirUri
	h.homeWorkspace = homeWorkspace
	h.setReleaseManager(releaseManager)
	h.events.setFocus(h.homeURI)

	// don't install a fs watcher for the home workspace,
	// to prevent unecessary resource consumption
	homeParser := syntax.NewParser(h.homeWorkspace, h.pkgmanager, h.homeURI)
	globalOpts := h.textOpts(cfg, homeParser, h.homeURI)
	tm := new(workspaceTabManager)
	tm.parent = h
	h.empty, err = newEx(
		func(reloader exoeditor.Reloader) (text.Editor, error) {
			return h.newEditor(reloader, homeDirUri, h.homeWorkspace,
				tm, cfg, vctrl.NopService())
		},
		homeWorkspace, h.ideStorage, h.notifications, h.homeURI,
		cfg.terminalConfig(), cfg.pluginBarConfig(),
		h.events.newPublisher(h.homeURI), 0 /* vte capacity */, h.clip, h.macro,
		h.dispatchOnPreview, tm, homeParser,
		h.newPromptEditor(cfg), h.commandObserver, h.debugCommands,
		cfg.commandPromptCfg(),
		cfg.shellCfg(),
		globalOpts...)
	if err != nil {
		return fmt.Errorf("new ex: %w", err)
	}
	tm.tm = h.empty.Browser()
	if err = h.subscribeAllCommands(h.empty); err != nil {
		return err
	}
	if err = h.subscribeAllEvents(cfg, h.empty); err != nil {
		return err
	}

	wsExec := workspaceshell.NewExecutor(
		workspaceExecutorAdapter{e: h.homeWorkspace})
	trackedCwd := &trackedWorkspace{Workspace: h.homeWorkspace, exec: wsExec}
	extExec, err := newExtensionsExecutor()
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Error building extensions executor: %v", err)
		log.Errorf("build home workspace extensions executor: %v", err)
		return nil
	}
	runner, lspManager, dapManager, _, err := h.buildExtensions(
		cfg, homeDirUri, trackedCwd, h.empty, extExec)
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Error building channel for extensions and plugins: %v", err)
		log.Errorf("build home workspace extensions: %v", err)
	} else {
		exec, isExecutor := runner.(schemeapi.Executor)
		if isExecutor {
			h.empty.setExecutor(exec, wsExec, extExec.shell)
		}
		h.homeRunner = runner
		h.homeLSPManager = lspManager
		h.homeDAPManager = dapManager
		go debug.CapturePanicReport(func() { h.initExtensions(runner, cfg) })
	}

	h.bar.Init()
	h.bar.OnClick = h.onBarTabClick
	frameAttr := cfg.windowFrameAttr()
	frameAttr.Attrs |= term.AttrVerticalRenderOffset
	backgroundAttr := term.Attributes{Bg: frameAttr.Bg}
	focusTabAttr := cfg.focusTabAttr()
	focusTabAttr.Attrs |= term.AttrVerticalRenderOffset
	nonFocusTabAttr := cfg.nonFocusTabAttr()
	nonFocusTabAttr.Attrs |= term.AttrVerticalRenderOffset
	focusTabIconAttr := cfg.focusTabIconAttr()
	focusTabIconAttr.Attrs |= term.AttrVerticalRenderOffset
	nonFocusTabIconAttr := cfg.nonFocusTabIconAttr()
	nonFocusTabIconAttr.Attrs |= term.AttrVerticalRenderOffset
	highlightAttr := cfg.highlightTabAttr()
	h.bar.SetAttr(focusTabAttr, nonFocusTabAttr,
		focusTabIconAttr, nonFocusTabIconAttr,
		highlightAttr, frameAttr, backgroundAttr)
	h.bar.SetFrameCharSet(cfg.windowFrameCharset())
	h.bar.SetBorder(cfg.frame())
	h.bar.SetBottomHighlight(true)
	h.bar.SetFocusFrameChar(cfg.workspaceHighlightTabChar())

	h.union.Init(&h.focusProxy)
	h.union.Attributes = cfg.windowFrameAttr()
	h.union.Frame = cfg.frame()

	charset := cfg.frameUnionCharset()
	h.union.Right = charset.Right
	h.union.Left = charset.Left
	h.union.Top = charset.Top
	h.union.Bottom = charset.Bottom
	h.workspaceBarKind = cfg.workspaceBarKind()
	h.state = idehistory.New(h.ideStorage)

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

func (h *workspaceManagerHandler) focusRunner() extension.Runner {
	if handler := h.workspaces[h.focus]; handler != nil {
		runner, _ := handler.Extensions.Load().(extension.Runner)
		return runner
	}
	return h.homeRunner
}

func (h *workspaceManagerHandler) envSource(name string) (string, bool) {
	uri := h.focusURI()
	if uri == (workspaceapi.URI{}) {
		return "", false
	}
	switch name {
	case "WORKSPACE":
		return workspaceBasename(uri), true
	case "WORKSPACE_URI":
		return uri.String(), true
	case "WORKSPACE_PATH":
		return uri.Path(), true
	}
	return "", false
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

func (h *workspaceManagerHandler) workspaceForFile(file workspaceapi.URI) (*openFileTarget, bool) {
	for i, wh := range h.workspaces {
		if wh == nil {
			continue
		}
		is, err := workspace.IsWorkspaceURI(wh.workspace, file)
		if err != nil || !is {
			continue
		}
		return &openFileTarget{workspaceIdx: i, workspace: wh}, true
	}
	return nil, false
}

func (h *workspaceManagerHandler) openFileTarget(file workspaceapi.URI) (*openFileTarget, bool) {
	for i, wh := range h.workspaces {
		if wh == nil {
			continue
		}
		for _, tab := range wh.ex.comp.Tabs() {
			if tab.URI().Equal(file) {
				return &openFileTarget{workspaceIdx: i, workspace: wh, tab: tab}, true
			}
		}
	}
	return h.workspaceForFile(file)
}

func (h *workspaceManagerHandler) focusOpenFileTarget(
	target *openFileTarget, file workspaceapi.URI, readOnly bool,
) (*browser.Tab, error) {
	if target == nil || target.workspace == nil {
		return nil, errors.New("workspace target not found")
	}
	h.switchToWorkspace(target.workspaceIdx)
	if target.tab != nil {
		if win, ok := target.tab.Window(); ok {
			_, err := target.workspace.ex.comp.SetFocus(win)
			return target.tab, err
		}
		win := target.workspace.ex.invokeWindow()
		if win.Closed() {
			win, _ = target.workspace.ex.comp.Focus()
		}
		if err := win.SetContent(target.tab); err != nil && err != browserapi.ErrTabNotFree {
			return nil, err
		}
		return target.tab, nil
	}
	return target.workspace.ex.editFileURILocal(file, target.workspace.ex.invokeWindow(), readOnly)
}

func (h *workspaceManagerHandler) RouteOpen(
	file workspaceapi.URI, readOnly bool,
) (browserapi.Handler, bool, error) {
	target, ok := h.openFileTarget(file)
	if !ok || target == nil || target.workspace == nil {
		return nil, false, nil
	}
	if target.tab == nil {
		focus := h.focusHandler()
		if focus == target.workspace {
			return nil, false, nil
		}
	}
	tab, err := h.focusOpenFileTarget(target, file, readOnly)
	return tab, true, err
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
	h.barIdxToSlot = h.barIdxToSlot[:0]

	drawBar := h.drawBar()
	if drawBar {
		height = max(0, height-h.barSize())
	}
	var barFocusIdx int
	for i, w := range h.workspaces {
		if w != nil {
			name := h.makeWorkspaceTabName(i, w)
			idx := h.bar.Add(rune(int(h.workspacesIcon)+i), name)
			h.barIdxToSlot = append(h.barIdxToSlot, i)
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
			h.barIdxToSlot = append(h.barIdxToSlot, i)
			barFocusIdx = idx
		}
	}
	h.bar.SetFocus(barFocusIdx)
	h.bar.SetHighlight(barFocusIdx)
	h.empty.Resize(width, height)
	// bar needs to be drawn last so frame union characters
	// are drawn last
	if drawBar {
		h.union.Resize(h.width, h.height)
	}
	if h.openPrevFiles != nil && h.width != 0 && h.height != 0 {
		err := h.openPrevSessionFiles(h.openPrevFilesEx, h.openPrevFiles, h.openPrevWindows)
		if err != nil {
			// do not notify during a call to Resize
			log.Errorf("restore prev session: %v", err)
		}
		h.openPrevFiles = nil
		h.openPrevFilesEx = nil
		h.openPrevWindows = nil
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
	h.events.setFocus(h.focusURI())
	h.focusProxy.Target = h.focusHandler()
	// resize for bottom workspace bar to disappear
	h.Resize(h.width, h.height)
	return true
}

func (h *workspaceManagerHandler) onBarTabClick(barIdx int) bool {
	if barIdx < 0 || barIdx >= len(h.barIdxToSlot) {
		return false
	}
	return h.switchToWorkspace(h.barIdxToSlot[barIdx])
}

func (h *workspaceManagerHandler) Handle(ev term.Event) (exit, handled bool) {
	h.macro.BeginEvent(ev)
	defer h.macro.EndEvent()
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
		hasDirtyFilesOpen := h.state.DirtyFilesOpen()
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
		path, pconfig := extensionRunArgs(p)
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			if err := startUserExtension(manager, id, path, pconfig); err != nil {
				log.Errorf("failed to run extension with id %q: %v", id, err)
			}
		})
	}

	wg.Wait()
}

// extensionRunArgs resolves the executable path and config for a user
// extension. It must run on the calling goroutine because extensionConfig.config
// records parse errors into the shared ideConfig.errors map (see
// extensionConfig.config), which is not safe to touch from the per-extension
// goroutines that startUserExtension feeds.
func extensionRunArgs(p extensionConfig) (string, config.Config) {
	path, _ := p.path()
	pconfig, ok := p.config()
	if !ok {
		pconfig = config.MapConfig(make(map[string]any))
	}
	return path, pconfig
}

// startUserExtension runs one user-configured extension on the given runner
// with pre-resolved args (see extensionRunArgs). It returns nil when the
// extension is already running so callers can treat a re-run as a no-op.
func startUserExtension(
	manager extension.Runner, id, path string, pconfig config.Config,
) error {
	if err := manager.Run(id, path, pconfig); err != nil &&
		!errors.Is(err, extensionv2.ErrExtensionAlreadyRunning) {
		return err
	}
	return nil
}

// startInstalledExtensions starts the given newly-added extensions on every
// live workspace runner (home plus open workspaces). It is invoked after a
// package install merges entries under the `extensions:` config key so the
// tools become active without a restart. It returns whether at least one of
// the ids matched a configured extension (and was thus started or already
// running).
//
// This always runs off the host event loop: the package manager drives config
// merges from background install goroutines (installGate.install spawns one;
// the auto-install path is reached through LibDir consumers in idelsp/idedebug/
// syntax workers). It therefore acquires h.mu — the shared IDE locker the event
// loop holds — to read event-loop-owned runner state, then releases it before
// spawning processes so the lock is never held across a fork. reloadConfig and
// extensionRunArgs touch only freshly loaded, non-shared config state, so they
// stay outside the lock.
func (h *workspaceManagerHandler) startInstalledExtensions(ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	cfg, err := h.reloadConfig()
	if err != nil {
		log.Errorf("failed to reload config to start installed extensions: %v", err)
		return false
	}
	userExtensions := cfg.extensions()

	type startArgs struct {
		id      string
		path    string
		pconfig config.Config
	}
	var toStart []startArgs
	for _, id := range ids {
		if p, ok := userExtensions[id]; ok {
			path, pconfig := extensionRunArgs(p)
			toStart = append(toStart, startArgs{id: id, path: path, pconfig: pconfig})
		}
	}
	if len(toStart) == 0 {
		return false
	}

	h.mu.Lock()
	runners := make([]extension.Runner, 0, h.workspaceCount+1)
	if h.homeRunner != nil {
		runners = append(runners, h.homeRunner)
	}
	for _, hm := range h.workspaces {
		if hm == nil {
			continue
		}
		if runner, ok := hm.Extensions.Load().(extension.Runner); ok && runner != nil {
			runners = append(runners, runner)
		}
	}
	h.mu.Unlock()

	var wg sync.WaitGroup
	for _, runner := range runners {
		for _, a := range toStart {
			runner, a := runner, a
			wg.Add(1)
			go debug.CapturePanicReport(func() {
				defer wg.Done()
				if err := startUserExtension(runner, a.id, a.path, a.pconfig); err != nil {
					log.Errorf("failed to start installed extension with id %q: %v",
						a.id, err)
				}
			})
		}
	}
	wg.Wait()
	return true
}

// afterPackageConfigMerge is the post-merge hook wired into the package
// manager. It starts any extension added under the `extensions:` config key so
// the package's tools work without a restart, and composes the externally
// supplied gui.env hook so both live-apply paths run. The gui.env hook runs
// first because it applies the package's environment via os.Setenv, and the
// extension processes spawned by startInstalledExtensions inherit os.Environ()
// at fork time; starting them first would deny them those variables. Extension
// start failures are logged (as in initExtensions), so the returned error is
// the stored hook's, and LiveApplied is OR'd across both paths. Any tutorial
// added under the `tutorials:` config key is live-registered (and the user
// prompted) through tutorialsInstalled so a freshly-installed tutorial is
// runnable without a restart; its error is joined onto the returned error so
// idepkg can notify in one place.
func (h *workspaceManagerHandler) afterPackageConfigMerge(
	event idepkg.ConfigMergeEvent,
) (idepkg.ConfigMergeResult, error) {
	var result idepkg.ConfigMergeResult
	var err error
	if h.packageConfigMergeHook != nil {
		result, err = h.packageConfigMergeHook(event)
	}

	startedExtension := h.startInstalledExtensions(event.AddedExtensionIDs())
	result.LiveApplied = result.LiveApplied || startedExtension

	if names := event.AddedTutorialNames(); len(names) > 0 {
		registered, tutErr := h.tutorialsInstalled(names)
		result.LiveApplied = result.LiveApplied || registered
		err = errors.Join(err, tutErr)
	}
	return result, err
}

func (h *workspaceManagerHandler) textOpts(
	cfg ideConfig, parser syntaxapi.Parser, uri workspaceapi.URI,
) []text.Option {
	markdownConfig := markdown.DefaultConfig()
	markdownConfig.Parser = parser
	markdownConfig.ScheduleNextTick = cfg.scheduleNextTick
	ret := []text.Option{
		text.WithTabspaces(cfg.editorTabspaces()),
		text.WithComments(cfg.editorComments()),
		text.WithWindowManagerConfig(cfg.windowManagerConfig()),
		text.WithFrameUnionCharSet(cfg.frameUnionCharset()),
		text.WithFrameUnion(cfg.frameUnion()),
		text.WithTabsClickCallback(h.tabsClickCallback),
		text.WithCommandKey(cfg.commandKey()),
		text.WithCommandMaxHistory(cfg.commandMaxHistory()),
		text.WithShellMaxHistory(cfg.shellMaxHistory()),
		text.WithCommandHistoryKey(cfg.commandHistoryKey()),
		text.WithFocusTabAttr(cfg.focusTabAttr(), cfg.focusTabIconAttr()),
		text.WithNonFocusTabAttr(cfg.nonFocusTabAttr(), cfg.nonFocusTabIconAttr()),
		text.WithFocusTabHighlightAttr(cfg.highlightTabAttr()),
		text.WithFocusTabHighlightChar(cfg.highlightTabChar()),
		text.WithWallpaper(cfg.wallpaper()),
		text.WithDirtyTabAttr(cfg.dirtyTabAttr()),
		text.WithIconSet(cfg.icons()),
		text.WithTabOverrideIcon(cfg.tabOverrideIcon()),
		text.WithCommandOverlayConfig(cfg.commandOverlayConfig()),
		text.WithCommandAliases(cfg.commandAliases()),
		text.WithPromptConfig(cfg.promptConfig()),
		text.WithEventPublisher(h.events.newPublisher(uri)),
		text.WithTabBarOffset(h.tabBarOffset),
		text.WithTabBarHeight(h.tabBarHeight),
		text.WithTabNameSeparator(cfg.tabNameSeparator()),
		text.WithPackageManager(h.pkgmanager),
		text.WithSyntaxConfig(cfg.syntaxConfig()),
		text.WithMaxSyntaxParseSize(cfg.editorMaxSizeForSyntax()),
		text.WithMarkdownConfig(markdownConfig),
		text.WithClipboard(h.clip),
		text.WithOpenRouter(h),
		text.WithFileExplorerIndentAttr(cfg.fileExplorerIndentAttr()),
		text.WithFileExplorerIconAttr(cfg.fileExplorerIconAttr()),
		text.WithEnvSource(h.envSource),
		text.WithStreamingOpen(h.streamingOpen),
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
	uri workspaceapi.URI, shouldRestore, promptRecommended bool, slot int,
) error {
	if i, ok := h.findInstalledSlot(uri); ok {
		h.switchToWorkspace(i)
		return nil
	}
	if h.isPending(uri) {
		_, _ = h.notifications.current().Notify(browserapi.LevelWarn,
			"workspace %q is already loading", uri.String())
		return nil
	}
	cwd, ctx, cancel, err := h.createWorkspaceScheme(uri)
	if err != nil {
		return err
	}
	pending, err := h.reservePendingSlot(uri, slot, cancel)
	if err != nil {
		cancel()
		return err
	}
	h.shaderRunner.startLoading()
	h.pendingWG.Add(1)
	go debug.CapturePanicReport(func() {
		built, buildErr := h.buildWorkspaceAsync(uri, cwd, pending)
		if pending.canceled.Load() {
			h.abortPendingBuild(pending, uri, built, buildErr, cancel)
			return
		}
		scheduled := h.scheduleNextTick(func() {
			defer h.pendingWG.Done()
			h.installPendingWorkspace(pending, uri, ctx, cancel,
				cwd, built, buildErr, shouldRestore, promptRecommended)
		})
		if !scheduled {
			h.abortPendingBuild(pending, uri, built, buildErr, cancel)
		}
	})
	return nil
}

func (h *workspaceManagerHandler) abortPendingBuild(
	pending *pendingWorkspace, uri workspaceapi.URI,
	built *builtWorkspace, buildErr error, cancel context.CancelFunc,
) {
	h.mu.Lock()
	delete(h.pending, uri.String())
	if h.lastReservedPending == pending {
		h.lastReservedPending = nil
	}
	h.mu.Unlock()
	if buildErr == nil {
		h.discardBuiltWorkspace(built)
	}
	cancel()
	h.pendingWG.Done()
}

func (h *workspaceManagerHandler) findInstalledSlot(uri workspaceapi.URI) (int, bool) {
	for i, w := range h.workspaces {
		if w == nil {
			continue
		}
		if w.uri.Equal(uri) {
			return i, true
		}
	}
	return -1, false
}

func (h *workspaceManagerHandler) isPending(uri workspaceapi.URI) bool {
	_, ok := h.pending[uri.String()]
	return ok
}

func (h *workspaceManagerHandler) createWorkspaceScheme(uri workspaceapi.URI) (
	workspace.Workspace, context.Context, context.CancelFunc, error,
) {
	cwd, err := h.workspace.AddWorkspace(context.Background(), uri)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create new workspace for %q: %w", uri, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return cwd, ctx, cancel, nil
}

func (h *workspaceManagerHandler) reservePendingSlot(
	uri workspaceapi.URI, slot int, cancel context.CancelFunc,
) (*pendingWorkspace, error) {
	if slot == -1 {
		var ok bool
		slot, ok = h.nextAvailableWorkspace()
		if !ok {
			return nil, fmt.Errorf("no available workspaces")
		}
	}
	if slot < 0 || slot >= len(h.workspaces) {
		return nil, fmt.Errorf("invalid workspace slot %d", slot)
	}
	if h.workspaces[slot] != nil {
		return nil, fmt.Errorf("workspace slot %d is occupied", slot)
	}
	pending := &pendingWorkspace{
		uri:       uri,
		slot:      slot,
		cancelCtx: cancel,
	}
	h.pending[uri.String()] = pending
	h.lastReservedPending = pending
	return pending, nil
}

type builtWorkspace struct {
	cfg                 ideConfig
	configErr           error
	wh                  *workspaceHandler
	ex                  *ex
	runner              extension.Runner
	cursorHistoryCloser io.Closer
	notice              *idenotice.Crier
	lspManager          *idelsp.Manager
	dapManager          *idedebug.Manager
	promptStorage       storageapi.Service
}

func (h *workspaceManagerHandler) buildWorkspaceAsync(
	uri workspaceapi.URI, cwd workspace.Workspace,
	pending *pendingWorkspace,
) (*builtWorkspace, error) {
	cfg, configErr := h.reloadConfig()
	cfg.storage = h.ideStorage
	cfg.scheduleNextTick = h.scheduleNextTick
	if _, wConfigErr := loadWorkspaceConfig(
		h.workspaceConfigFilename, cwd, uri, &cfg,
	); wConfigErr != nil {
		configErr = multierror.Append(configErr,
			fmt.Errorf("workspace config: %w", wConfigErr))
	}

	parser := syntax.NewParser(cwd, h.pkgmanager, uri)
	textOpts := h.textOpts(cfg, parser, uri)
	vctrlService, err := gogit.NewService(uri, cwd)
	if err != nil {
		h.empty.log(log.ErrorLevel, "new git service for workspace %q: %v",
			uri.Path(), err)
		vctrlService = vctrl.NopService()
	} else {
		vctrlService = vctrl.SyncService(vctrlService, new(sync.Mutex))
	}

	visibleManager := visibleWorkspaceManager{parent: h, manager: h.workspace}
	multicwd := workspace.Multi(context.Background(), visibleManager, cwd, uri)
	tm := new(workspaceTabManager)
	tm.parent = h
	ex, err := newEx(
		func(reloader exoeditor.Reloader) (text.Editor, error) {
			return h.newEditor(reloader, uri, multicwd, tm, cfg, vctrlService)
		},
		multicwd, h.ideStorage, h.notifications, uri,
		cfg.terminalConfig(), cfg.pluginBarConfig(), h.events.newPublisher(uri),
		h.initialVTECapacity, h.clip, h.macro, h.dispatchOnPreview,
		tm, parser,
		h.newPromptEditor(cfg), h.commandObserver, h.debugCommands,
		cfg.commandPromptCfg(),
		cfg.shellCfg(),
		textOpts...)
	if err != nil {
		return nil, fmt.Errorf("new ex: %w", err)
	}
	apibrowser := newBrowserAdapter(ex.Browser())
	cursorHistoryCloser, err := idecursor.WithHistory(
		ex.Editor(), h.ideStorage, apibrowser, apibrowser, ex.workspace,
		syntax.NewParser(ex.workspace, h.pkgmanager, uri), visibleManager, uri,
		h.scheduleNextTick,
	)
	if err != nil {
		return nil, fmt.Errorf("install cursor history: %w", err)
	}
	tm.tm = ex.Browser()

	wh := &workspaceHandler{
		vctrlService:        vctrlService,
		cursorHistoryCloser: cursorHistoryCloser,
		cancelCtx:           pending.cancelCtx,
		uri:                 uri,
		ex:                  ex,
		cwd:                 cwd,
	}
	tm.workspace = wh

	wsExec := workspaceshell.NewExecutor(
		workspaceExecutorAdapter{e: cwd})
	trackedCwd := &trackedWorkspace{Workspace: cwd, exec: wsExec}
	extExec, err := newExtensionsExecutor()
	if err != nil {
		_ = cursorHistoryCloser.Close()
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Error building extensions executor: %v", err)
		log.Errorf("build extensions executor for workspace %s: %v",
			uri.String(), err)
		return nil, fmt.Errorf("new extensions executor: %w", err)
	}
	runner, lspManager, dapManager, promptStorage, err := h.buildExtensions(
		cfg, uri, trackedCwd, ex, extExec)
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Error building channel for extensions and plugins: %v", err)
		log.Errorf("build extensions for workspace %s: %v",
			uri.String(), err)
	} else {
		exec, isExecutor := runner.(schemeapi.Executor)
		if isExecutor {
			ex.setExecutor(exec, wsExec, extExec.shell)
		}
		wh.Extensions.Store(runner)
		wh.lspManager = lspManager
		wh.dapManager = dapManager
		wh.promptStorage = promptStorage
	}

	built := &builtWorkspace{
		cfg:                 cfg,
		configErr:           configErr,
		wh:                  wh,
		ex:                  ex,
		runner:              runner,
		cursorHistoryCloser: cursorHistoryCloser,
		lspManager:          lspManager,
		dapManager:          dapManager,
		promptStorage:       promptStorage,
	}
	if noticeCfg, ok := newNoticeConfig(cfg, h.ideStorage, uri); ok {
		built.notice = idenotice.New(
			cwd, apibrowser, parser, h.scheduleNextTick,
			noticeLinkCopier(h.clip, apibrowser), noticeCfg)
	}
	return built, nil
}

func (h *workspaceManagerHandler) installPendingWorkspace(
	pending *pendingWorkspace,
	uri workspaceapi.URI,
	ctx context.Context,
	cancel context.CancelFunc,
	cwd workspace.Workspace,
	built *builtWorkspace,
	buildErr error,
	shouldRestore, promptRecommended bool,
) {
	delete(h.pending, uri.String())
	if h.lastReservedPending == pending {
		h.lastReservedPending = nil
	}

	defer h.shaderRunner.stopLoading()

	if pending.canceled.Load() {
		if built != nil {
			h.discardBuiltWorkspace(built)
		}
		cancel()
		return
	}

	if buildErr != nil {
		log.Errorf("load workspace %s: %v", uri.String(), buildErr)
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"Failed to load workspace %s: %v", uri.String(), buildErr)
		cancel()
		return
	}

	if err := h.subscribeAllCommands(built.ex); err != nil {
		_ = built.cursorHistoryCloser.Close()
		cancel()
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"subscribe workspace commands: %v", err)
		return
	}
	if err := h.subscribeAllEvents(built.cfg, built.ex); err != nil {
		_ = built.cursorHistoryCloser.Close()
		cancel()
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"subscribe workspace events: %v", err)
		return
	}

	ex := built.ex
	wh := built.wh
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

	if built.runner != nil {
		// load async to speed up workspace initialization
		go debug.CapturePanicReport(func() {
			h.initExtensions(built.runner, built.cfg)
		})
	}

	// Capture the current root before switching slots so the open shader can
	// burn away the previous screen. Capturing inside the shader would be too
	// late: by its first Draw, the newly opened workspace is already focused.
	h.shaderRunner.captureOpenShaderCells()

	h.workspaces[pending.slot] = wh
	h.workspaceCount++
	h.switchToWorkspace(pending.slot)

	if built.notice != nil {
		notice := built.notice
		h.scheduleNextTick(func() {
			if err := notice.Show(ctx); err != nil {
				ex.log(log.WarnLevel, "show workspace notice: %v", err)
			}
		})
	}

	// Drain any commands enqueued via `workspaceready` while this
	// pending build was in flight. They run against the freshly
	// focused ex, in the same event-loop turn, so semantics match
	// "the new workspace just opened and then ran these commands".
	for _, cmd := range pending.onReady {
		if len(cmd) == 0 {
			continue
		}
		if err := ex.dispatchCommand(cmd[0], cmd[1:]...); err != nil {
			_, _ = h.notifications.current().Notify(browserapi.LevelError,
				"workspaceready %s: %v", cmd[0], err)
		}
	}
	pending.onReady = nil

	h.logNonFatalErrs(wh.Browser(), built.configErr, built.cfg.errors)

	state, err := h.state.LoadWorkspaceState(ctx, uri)
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"load workspace state for %s: %v", uri.String(), err)
		return
	}
	fexplorerURI, _ := workspaceapi.ParseURI(fileExplorerURI)
	wh.historyCloser = h.state.SubscribeEvents(
		ctx, uri, &ex.comp, exSnapshotter{ex: ex}, fexplorerURI)
	if !shouldRestore {
		if err := h.state.ClearWorkspaceState(ctx, uri); err != nil {
			_, _ = h.notifications.current().Notify(browserapi.LevelError,
				"clear workspace state for %s: %v", uri.String(), err)
		}
		return
	}
	if state.IsEmpty() {
		return
	}
	if promptRecommended && !built.cfg.autoRestore() {
		h.openRestorePrompt(ex, uri, state)
		return
	}
	if err := h.restorePreviousSession(ex, state); err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"restore previous session: %v", err)
	}
}

// discardBuiltWorkspace tears down a Phase B build whose install was
// canceled (via closeWorkspace before Phase C ran).
func (h *workspaceManagerHandler) discardBuiltWorkspace(built *builtWorkspace) {
	if built == nil {
		return
	}
	if built.cursorHistoryCloser != nil {
		_ = built.cursorHistoryCloser.Close()
	}
	if built.runner != nil {
		if c, ok := built.runner.(io.Closer); ok {
			_ = c.Close()
		}
	}
	if built.lspManager != nil {
		_ = built.lspManager.Close()
	}
	if built.dapManager != nil {
		_ = built.dapManager.Close()
	}
	if built.promptStorage != nil {
		_ = built.promptStorage.Close()
	}
	if built.ex != nil {
		_ = built.ex.Close()
	}
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
	cwd workspace.Workspace, ex *ex, extExecutor *extensionsExecutor,
) (
	extension.Runner, *idelsp.Manager, *idedebug.Manager,
	storageapi.Service, error,
) {
	notifications := h.notifications.new(uri, ex.container)
	ed := ex.Editor()
	promptOpener := &ex.comp
	promptStorage := storageapi.WithPartition(h.storage, "extension-permissions")
	var retErr error
	defer func() {
		if retErr != nil {
			_ = promptStorage.Close()
		}
	}()
	cmdAuthorizer, err := ideauthorizer.NewAuthorizer(
		ed, promptOpener, promptStorage, cfg.scheduleNextTick, notifications,
		cfg.authorizerAutoAuthorize())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new command authorizer: %w", err)
	}
	res := extension.BrowserResources(ex.Browser(), h.events.newPublisher(uri))
	res = extension.MergeResourceMap(res,
		extension.EditorResources(ex.Browser(), ed, h.events.newPublisher(uri)))
	res = extension.MergeResourceMap(res,
		extension.WorkspaceResources(cwd, cmdAuthorizer))
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
		Icons:            cfg.lspIcons(),
	}
	callbacks := idelsp.NewCallbackHandler(notifications, apibrowser, ex.Browser(),
		apieditor, cwd, uri.String(), lspCallbackCfg)
	lspConfig := idelsp.Config{
		NoInitializeServer: true,
		Callback:           callbacks,
		MaxRetries:         5,
		WorkDoneProgress:   true,
		ScheduleNextTick:   cfg.scheduleNextTick,
	}
	lsp := idelsp.New(uri, cwd,
		cwd, h.pkgmanager, notifications,
		ex.Browser(), lspConfig)
	dapCfg := idedebug.Config{
		MaxRetries: 5,
		Adapters:   cfg.debuggerConfigs(),
	}
	dap := idedebug.New(uri, cwd, h.pkgmanager, dapCfg)
	defer func() {
		if retErr == nil {
			return
		}
		if err := lsp.Close(); err != nil {
			log.Warnf("close lsp manager after build error: %v", err)
		}
		if err := dap.Close(); err != nil {
			log.Warnf("close dap manager after build error: %v", err)
		}
	}()
	err = ex.comp.SubscribeEvents(idelsp.EditorEvents(), lsp)
	if err != nil {
		log.Errorf("subscribe LSP manager: %v", err)
	}
	// Register the top-level "debugger" REPL command and its
	// command-prompt handler. debugshell.Handler owns the active
	// debug session lifecycle (initialize, launch, attach,
	// terminate) and forwards DAP events back to the REPL.
	dbgHandler := debugshell.New(dap, &ex.comp, apieditor, parser, ex.workspace, debugshell.Config{
		WorkspaceURI: uri,
		Icons: debugshell.Icons{
			Breakpoint: "",
			Stopped:    "",
		},
		Debugger:         dapCfg,
		ScheduleNextTick: cfg.scheduleNextTick,
	}).WithNotify(func(level browserapi.NotificationLevel, msg string, args ...any) {
		// DAP message-reader goroutines invoke this off the event
		// loop; hop through scheduleNextTick so notis.inFocus reads
		// workspaceManagerHandler.focus on the goroutine that mutates
		// it.
		cfg.scheduleNextTick(func() {
			_, _ = notifications.Notify(level, msg, args...)
		})
	})
	dbgMan := debugshell.Manual()
	if err := ex.comp.RegisterREPLCommand(dbgMan, dbgHandler); err != nil {
		log.Errorf("register debugger repl command: %v", err)
	}
	if err := ex.comp.SubscribeCommand(dbgMan,
		debugshell.NewPromptHandler(dbgHandler).
			WithOpenShell(ex.shellnewtab)); err != nil {
		log.Errorf("subscribe debugger command prompt: %v", err)
	}
	cmdcfg := lspCommandsConfig(uri, cfg, notifications,
		h.events.newInterrupter(uri), parser, callbacks)
	apiHandler, err := lspcmd.AllHandler(
		lsp, apieditor, apibrowser, apibrowser, apibrowser,
		ex.workspace, parser, cmdcfg)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new lsp command handler: %v", err)
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
	res = extension.MergeResourceMap(res, extension.LLMResources(h.llmRouter))

	// Register the top-level `models` REPL command. The llmshell
	// reads the local llama.cpp registry directly off the router.
	llmHandler := llmshell.New(llmshell.Config{
		Service:          h.llmRouter,
		LocalRegistry:    h.llmRouter.LocalRegistry(),
		Storage:          h.storage,
		Router:           h.llmRouter,
		WindowManager:    apibrowser,
		Notifications:    notifications,
		ScheduleNextTick: cfg.scheduleNextTick,
		PromptOpener:     &ex.comp,
	})
	if err := ex.comp.RegisterREPLCommand(llmshell.Manual(), llmHandler); err != nil {
		log.Errorf("register llm repl command: %v", err)
	}

	// Register the top-level `pkg` REPL command for package management.
	pkgHandler := pkgshell.New(pkgshell.Config{
		Manager:       h.pkgmanager.pkg,
		UpdateChecker: h.pkgmanager.uc,
	})
	if err := ex.comp.RegisterREPLCommand(pkgshell.Manual(), pkgHandler); err != nil {
		log.Errorf("register pkg repl command: %v", err)
	}

	dataDir := h.sixDir
	if err := os.MkdirAll(dataDir, 0777); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("mkdir %s: %v", dataDir, err)
	}
	browser := ex.Browser()
	// grant all permissions for now, until we actually have installable third
	// party extensions.
	grantor := extension.GrantAll()
	runner, err := h.extensionRunner.WorkspaceExtensionsRunner(uri, res, cmdAuthorizer,
		dataDir, browser, cwd, extExecutor, grantor,
		ed, promptOpener, promptStorage, cfg.scheduleNextTick)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new workspace extensions runner: %v", err)
	}
	return runner, lsp, dap, promptStorage, nil
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
	ex *ex, files []idehistory.File, windows map[uint64]browser.Window,
) (err error) {
	invokeWindow := ex.invokeWindow()
	for _, f := range files {
		if f.URI.String() == fileExplorerURI {
			continue
		}
		uri := f.URI
		win := invokeWindow
		if f.WindowID != 0 {
			if mappedWin, ok := windows[f.WindowID]; ok {
				win = mappedWin
			}
		}
		t, ferr := ex.editFileURI(uri, win, false)
		if ferr != nil {
			err = multierror.Append(err, ferr)
			continue
		}
		ed, ok := t.Handler().(text.Handler)
		if !ok {
			continue
		}
		ed.SetCursorAtScroll(f.Cursor)
	}
	return err
}

func (h *workspaceManagerHandler) restorePreviousSession(
	ex *ex,
	state idehistory.State,
) error {
	ret := new(multierror.Error)
	layout := state.Layout
	layout.Floating = nil
	restoreTerminals := len(state.Terminals) > 0
	windows := h.restoreWorkspaceWindows(ex, state.Files, restoreTerminals,
		layout, state.HasLayout)
	if restoreTerminals {
		ret = multierror.Append(ret,
			restoreOpenTerminalSessions(ex, state.Terminals, windows))
	}
	if len(state.Tasks) > 0 {
		ret = multierror.Append(ret,
			restoreOpenTaskSessions(ex, state.Tasks))
	}
	if len(state.Files) == 0 {
		return ret.ErrorOrNil()
	}
	if h.width == 0 || h.height == 0 {
		// if restoreSession is called on an size 0,0 handler
		// then cursor is not properly set.
		h.openPrevFiles = state.Files
		h.openPrevWindows = windows
		h.openPrevFilesEx = ex
		return ret.ErrorOrNil()
	}
	ret = multierror.Append(ret, h.openPrevSessionFiles(ex, state.Files, windows))
	return ret.ErrorOrNil()
}

func (h *workspaceManagerHandler) restoreWorkspaceWindows(
	ex *ex,
	files []idehistory.File,
	restoreTerminals bool,
	layout tcomponent.TileLayout,
	hasLayout bool,
) map[uint64]browser.Window {
	if !hasLayout {
		return nil
	}
	if len(files) == 0 && !restoreTerminals {
		return nil
	}
	return ex.comp.Browser().RestoreTileLayout(layout, func(windowID uint64) browserapi.Handler {
		return nil
	})
}
func (h *workspaceManagerHandler) nextAvailableWorkspace() (idx int, ok bool) {
	for i := h.focus; i >= 0 && i < len(h.workspaces); i++ {
		if h.slotIsFree(i) {
			return i, true
		}
	}
	for i := 0; i < h.focus && i < len(h.workspaces); i++ {
		if h.slotIsFree(i) {
			return i, true
		}
	}
	return 0, false
}

// slotIsFree reports whether slot i has neither an installed
// workspaceHandler nor an in-flight pending build reserving it.
func (h *workspaceManagerHandler) slotIsFree(i int) bool {
	if h.workspaces[i] != nil {
		return false
	}
	for _, p := range h.pending {
		if p.slot == i {
			return false
		}
	}
	return true
}

// pendingForFocus returns the pending build (if any) reserving the
// currently focused slot.
func (h *workspaceManagerHandler) pendingForFocus() (*pendingWorkspace, bool) {
	for _, p := range h.pending {
		if p.slot == h.focus {
			return p, true
		}
	}
	return nil, false
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
		return errors.New("expected at least one argument with the workspace path")
	}
	path := args[0]

	if uri, err := workspaceapi.ParseURI(path); err == nil {
		return h.addOrCreateWorkspace(uri)
	}

	uri, err := h.homeWorkspace.URI(path)
	if err != nil {
		return err
	}
	return h.addOrCreateWorkspace(uri)
}

func (h *workspaceManagerHandler) commandRenameWorkspace(args ...string) error {
	if h.focusHandler() == h.empty {
		return fmt.Errorf("there's no workspace to rename. " +
			"First you must open one via `workspaceopen`")
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

func (h *workspaceManagerHandler) commandWorkspaceReady(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected at least one argument with the command to run")
	}
	if h.lastReservedPending != nil {
		cmd := append([]string(nil), args...)
		h.lastReservedPending.onReady = append(h.lastReservedPending.onReady, cmd)
		return nil
	}
	ex := h.exHandler(h.focusHandler())
	return ex.dispatchCommand(args[0], args[1:]...)
}

func (h *workspaceManagerHandler) commandExtensionReady(args ...string) error {
	if len(args) < 2 {
		return fmt.Errorf("expected extension id and command")
	}
	if h.lastReservedPending != nil {
		return h.commandWorkspaceReady(append([]string{cmdExtensionReady}, args...)...)
	}
	runner := h.focusRunner()
	if runner == nil {
		return fmt.Errorf("no extension runner on the focused workspace")
	}
	id := args[0]
	job := extReadyJob{cmd: args[1], args: append([]string(nil), args[2:]...)}
	ex := h.exHandler(h.focusHandler())
	ch, ok := ex.extReady[id]
	if !ok {
		ch = make(chan extReadyJob, extReadyQueueLimit)
		ex.extReady[id] = ch
		ch <- job
		h.startExtReadyWorker(ex.extReadyCtx, ex, id, runner, ch)
		return nil
	}
	if len(ch) == extReadyQueueLimit {
		return fmt.Errorf(
			"too many extensionready commands queued for %q (max %d)",
			id, extReadyQueueLimit)
	}
	ch <- job
	return nil
}

// extReadyQueueLimit bounds the per-id extensionready follow-up queue.
const extReadyQueueLimit = 20

// extReadyJob is a queued extensionready follow-up command; the id and
// runner are fixed per worker, so only the command and args vary.
type extReadyJob struct {
	cmd  string
	args []string
}

// startExtReadyWorker drains ch for one extension id, dispatching queued
// commands in submission order once the extension is ready. ctx cancel
// (ex.Close) stops the worker mid-backlog.
func (h *workspaceManagerHandler) startExtReadyWorker(
	ctx context.Context, ex *ex, id string,
	runner extension.Runner, ch chan extReadyJob,
) {
	readyErr := make(chan error, 1)

	go debug.CapturePanicReport(func() {
		readyCtx, cancel := context.WithTimeout(ctx, h.extReadyWait)
		defer cancel()
		err := runner.WaitReady(readyCtx, id)
		// A cancelled parent ctx is ex.Close, handled by the worker; only
		// a genuine deadline becomes a user-facing error.
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			err = fmt.Errorf("extension %q not ready within %s", id, h.extReadyWait)
		}
		readyErr <- err
	})

	go debug.CapturePanicReport(func() {
		var err error
		select {
		case err = <-readyErr:
		case <-ctx.Done():
			return
		}
		for {
			var job extReadyJob
			select {
			case job = <-ch:
			case <-ctx.Done():
				return
			}
			jobErr := err
			if jobErr == nil {
				jobErr = h.waitCommandRegistered(ctx, ex, job.cmd)
			}
			if ctx.Err() != nil {
				return
			}
			if dispErr := h.runExtReadyJob(ctx, ex, id, job, jobErr); dispErr != nil {
				_, _ = h.notifications.current().Notify(
					browserapi.LevelError,
					"extensionready %s: %v", id, dispErr)
			}
		}
	})
}

// runExtReadyJob dispatches a single follow-up command and blocks until
// it completes, so the next job cannot overtake it. The dispatch carries
// a textrpc.Waiter: an out-of-process extension command claims it and
// reports completion on the channel after HandleCommand returns, so the
// wait (bounded by extensionHandleWait) preserves ordering across the RPC
// boundary. An in-process command leaves the waiter unclaimed and is
// already done when dispatch returns. A non-nil jobErr from the
// readiness/registration wait short-circuits dispatch.
func (h *workspaceManagerHandler) runExtReadyJob(
	ctx context.Context, ex *ex, _ string, job extReadyJob, jobErr error,
) error {
	if jobErr != nil {
		return jobErr
	}

	type dispatched struct {
		err     error
		claimed bool
	}
	waiterCh := make(chan error, 1)
	doneCh := make(chan dispatched, 1)
	scheduled := h.scheduleNextTick(func() {
		w := &textrpc.Waiter{Ch: waiterCh}
		err := ex.dispatchCommandCtx(
			textrpc.ContextWithWaiter(ctx, w), job.cmd, job.args...)
		doneCh <- dispatched{err: err, claimed: w.Claimed}
	})
	if !scheduled {
		return fmt.Errorf("could not schedule")
	}

	var d dispatched
	select {
	case d = <-doneCh:
	case <-ctx.Done():
		return nil
	}
	// An unclaimed waiter (in-process command) or a dispatch error (the
	// extension command never reached the wire) means no completion will
	// arrive on the channel; the dispatch result is final.
	if !d.claimed || d.err != nil {
		return d.err
	}

	waitCtx, cancel := context.WithTimeout(ctx, h.extHandleWait)
	defer cancel()
	select {
	case err := <-waiterCh:
		return err
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

// extensionCommandWait bounds how long extensionready blocks for the
// follow-up command to be registered after the extension's protocol
// handshake completes. Registration is published asynchronously over
// the extension's editor RPC, so the command may not exist the instant
// WaitReady returns.
const extensionCommandWait = 10 * time.Second

// extensionReadyWait bounds the wait for an extension to become ready,
// so a never-ready extension surfaces an error instead of parking its
// follow-up commands until the workspace is torn down.
const extensionReadyWait = 30 * time.Second

// extensionHandleWait bounds how long an extensionready follow-up waits
// for an out-of-process extension command to finish handling, so a wedged
// extension surfaces an error and the queue keeps draining instead of
// blocking the next follow-up forever.
const extensionHandleWait = 30 * time.Second

// waitCommandRegistered blocks until cmd is registered on ex, the
// timeout elapses, or scheduling fails. The registration check runs on
// the event loop because the command registry is owned by the editor
// component.
func (h *workspaceManagerHandler) waitCommandRegistered(
	ctx context.Context, ex *ex, cmd string,
) error {
	deadline := time.NewTimer(h.extCommandWait)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		registered := make(chan bool, 1)
		scheduled := h.scheduleNextTick(func() {
			registered <- commandRegistered(ex, cmd)
		})
		if !scheduled {
			return fmt.Errorf("command %q wait: event loop is closed", cmd)
		}
		select {
		case ok := <-registered:
			if ok {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("command %q was not registered within %s",
				cmd, h.extCommandWait)
		}
	}
}

// commandRegistered reports whether cmd is registered on ex's editor.
// It must run on the event loop.
func commandRegistered(ex *ex, cmd string) bool {
	for _, man := range ex.comp.Commands() {
		if man.Name == cmd {
			return true
		}
	}
	return false
}

func (h *workspaceManagerHandler) closeWorkspace() (
	workspaceapi.URI, []workspaceapi.URI, error,
) {
	if pending, ok := h.pendingForFocus(); ok {
		uri := pending.uri
		pending.canceled.Store(true)
		pending.cancelCtx()
		delete(h.pending, uri.String())
		if h.lastReservedPending == pending {
			h.lastReservedPending = nil
		}
		if err := h.state.ClearWorkspaceState(
			context.Background(), uri); err != nil {
			log.Warnf("clear workspace state %s: %v", uri.String(), err)
		}
		return uri, nil, nil
	}
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

	h.persistWorkspaceStateOnClose(hm)
	if err := hm.closeAndRemove(); err != nil {
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
	h.events.setFocus(h.focusURI())
	h.Resize(h.width, h.height)
	return err
}

func (h *workspaceManagerHandler) Close() (ret error) {
	for k, p := range h.pending {
		p.canceled.Store(true)
		p.cancelCtx()
		delete(h.pending, k)
	}
	h.mu.Unlock()
	h.pendingWG.Wait()
	h.mu.Lock()
	for _, hm := range h.workspaces {
		if hm == nil {
			continue
		}
		h.persistWorkspaceStateOnClose(hm)
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
	if h.homeLSPManager != nil {
		if err := h.homeLSPManager.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if h.homeDAPManager != nil {
		if err := h.homeDAPManager.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if h.pkgmanager != nil {
		if err := h.pkgmanager.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if h.llmRouter != nil {
		if err := h.llmRouter.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if merr, ok := ret.(*multierror.Error); ok {
		return merr.ErrorOrNil()
	}
	return ret
}

type workspaceHandler struct {
	*ex
	tabname             string
	attentionAttr       term.Attributes
	vctrlService        vctrl.Service
	cursorHistoryCloser io.Closer
	cancelCtx           func()
	uri                 workspaceapi.URI
	Extensions          atomic.Value
	cwd                 workspace.Workspace
	closeOnce           sync.Once
	closeErr            error
	historyCloser       io.Closer
	lspManager          *idelsp.Manager
	dapManager          *idedebug.Manager
	promptStorage       storageapi.Service
}

func (hm *workspaceHandler) Close() error {
	hm.closeOnce.Do(func() {
		var ret error
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
		if hm.cursorHistoryCloser != nil {
			if err := hm.cursorHistoryCloser.Close(); err != nil {
				ret = multierror.Append(ret, err)
			}
		}
		if hm.lspManager != nil {
			if err := hm.lspManager.Close(); err != nil {
				ret = multierror.Append(ret, err)
			}
		}
		if hm.dapManager != nil {
			if err := hm.dapManager.Close(); err != nil {
				ret = multierror.Append(ret, err)
			}
		}
		if hm.promptStorage != nil {
			if err := hm.promptStorage.Close(); err != nil {
				ret = multierror.Append(ret, err)
			}
		}
		// cancel at the end, so fs event processing is not
		// vacated before everything else is still potentially
		// sending events (i.e. mem scheme)
		hm.cancelCtx()
		hm.closeErr = ret
	})
	return hm.closeErr
}

func (hm *workspaceHandler) closeAndRemove() (ret error) {
	if err := hm.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if hm.cwd != nil {
		if err := hm.cwd.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
		hm.cwd = nil
	}
	return ret
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
	err = h.subscribeAllExternalREPLCommands(ex)
	if err != nil {
		return fmt.Errorf("subscribe external repl commands: %w", err)
	}
	err = ex.comp.SubscribeCommand(textapi.CommandManual{
		Name: cmdMacroRecord,
		Summary: "Toggle recording all user key events into the given clipboard register. " +
			"Run `record a` to start capturing keys into register `a`, then run `record a` " +
			"again to stop recording and save the key sequence. Recorded macros share the " +
			"same register namespace as editor copy/paste, so different register IDs can hold " +
			"different macros (`record a`, `record b`, `record +`, etc.). Replay a recorded " +
			"macro by pairing this command with echo's register instruction: " +
			"`echo {register}a` reads register `a`, parses the recorded keys, and sends them " +
			"back through the IDE event loop. Echo sequences can also combine literal keys, " +
			"instructions, and registers, for example `echo i{register}a<esc>{register}b`.",
		Synopsis: "[<register>]",
	}, h.macro)
	if err != nil {
		return fmt.Errorf("subscribe macro commands: %w", err)
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
				Synopsis: "[<scheme>:][//[<userinfo>@]<host>][/]<workspacepath>",
			},
		},
		cmdRenameWorkspace: {
			handler: (*workspaceManagerHandler).commandRenameWorkspace,
			man: textapi.CommandManual{
				Summary: "Renames the workspace tab. The tab is displayed at the " +
					"bottom of the screen when multiple workspaces are open.",
				Synopsis: "<name>",
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
		cmdWorkspaceReady: {
			handler: (*workspaceManagerHandler).commandWorkspaceReady,
			man: textapi.CommandManual{
				Summary: "Runs another workspace command once the most recently " +
					"issued `workspaceopen` has finished loading. If no workspace is " +
					"currently being loaded, the command is dispatched immediately " +
					"against the focused workspace.",
				Synopsis: "<command> [<args>...]",
			},
		},
		cmdExtensionReady: {
			handler: (*workspaceManagerHandler).commandExtensionReady,
			man: textapi.CommandManual{
				Summary: "Runs another command once the extension with the given " +
					"id has finished initializing on the workspace. If a " +
					"workspaceopen is currently pending, the wait starts after " +
					"that workspace finishes installing.",
				Synopsis: "<extension-id> <command> [<args>...]",
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

func (h *workspaceManagerHandler) subscribeAllExternalREPLCommands(ex *ex) (ret error) {
	var cmds []externalREPLCommand
	for _, cmd := range h.externalREPLCommands {
		cmds = append(cmds, cmd)
	}
	return h.subscribeExternalREPLCommands(ex, cmds...)
}

func (h *workspaceManagerHandler) subscribeExternalREPLCommands(
	ex *ex, commands ...externalREPLCommand,
) (ret error) {
	for _, cmd := range commands {
		err := ex.comp.RegisterREPLCommand(cmd.cmd, cmd.handler)
		if err != nil {
			ret = multierror.Append(ret,
				fmt.Errorf("register repl command '%s': %v", cmd.cmd.Name, err))
		}
	}
	return ret
}

type externalREPLCommand struct {
	cmd     textapi.CommandManual
	handler textapi.REPLHandler
}

func (h *workspaceManagerHandler) registerREPLCommand(
	cmd textapi.CommandManual, handler textapi.REPLHandler,
) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.externalREPLCommands[cmd.Name]; ok {
		return fmt.Errorf("repl command '%s' already registered", cmd.Name)
	}

	extCmd := externalREPLCommand{cmd: cmd, handler: handler}

	ret := h.subscribeExternalREPLCommands(h.empty, extCmd)
	for _, w := range h.workspaces {
		if w == nil {
			continue
		}
		if err := h.subscribeExternalREPLCommands(w.ex, extCmd); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if ret != nil {
		return ret
	}

	h.externalREPLCommands[cmd.Name] = extCmd
	return nil
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

func (h *workspaceManagerHandler) subscribeAllEvents(
	cfg ideConfig, ex *ex,
) error {
	if err := h.subscribeExternalEvents(ex, h.externalEvents...); err != nil {
		return err
	}
	if cfg.editorAutoSave() && !ex.ed.IsExternal() {
		saver := autoSaverFactory(&ex.comp, ex.notifications,
			cfg.scheduleNextTick, defaultAutoSaveDelay)
		if err := ex.comp.SubscribeEvents(autoSaveEvents, saver); err != nil {
			return fmt.Errorf("subscribe autoSaver: %w", err)
		}
	}
	return nil
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

func (h *workspaceManagerHandler) persistWorkspaceStateOnClose(hm *workspaceHandler) {
	if err := h.state.StoreWorkspaceStateForClose(
		context.Background(), hm.uri, exSnapshotter{ex: hm.ex}); err != nil {
		log.Warnf("persist workspace state %s: %v", hm.uri.String(), err)
	}
	if hm.historyCloser != nil {
		_ = hm.historyCloser.Close()
		hm.historyCloser = nil
	}
}

func (h *workspaceManagerHandler) Interrupt(ctx context.Context) error {
	return h.events.globalInterrupter().Interrupt(ctx)
}

func (h *workspaceManagerHandler) waitInflight() {
	h.mu.Lock()
	exes := make([]*ex, 0, len(h.workspaces))
	for _, w := range h.workspaces {
		if w == nil || w.ex == nil {
			continue
		}
		exes = append(exes, w.ex)
	}
	if h.empty != nil {
		exes = append(exes, h.empty)
	}
	h.mu.Unlock()
	for _, e := range exes {
		e.waitInflight()
	}
}

func (h *workspaceManagerHandler) setReleaseManager(releaseManager release.Manager) {
	notifications := h.notifications.current()
	if h.pkgmanager == nil {
		h.pkgmanager = new(pkgManager)
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.pkgmanager.pkg.Reconcile(ctx); err != nil {
				_, _ = notifications.Notify(browserapi.LevelError,
					"package manager: clean up: %v", err)
			}
			err := h.pkgmanager.pkg.ProcessInstalledSettings(ctx)
			if err != nil {
				_, _ = notifications.Notify(browserapi.LevelError,
					"package manager: process installed settings: %v", err)
			} else {
				log.Debugf("processed all installed settings")
			}
		}()
	} else {
		h.pkgmanager.Close()
	}
	wm := currentWorkspaceWindowManager{root: h}
	parser := &lazyParser{root: h}
	editorMode := ""
	autoInstall := true
	if cfg, err := h.reloadConfig(); err == nil {
		editorMode = cfg.pkgEditorMode()
		autoInstall = cfg.updatesAutoInstall()
	}
	h.pkgmanager.init(notifications, releaseManager, wm,
		h.ideStorage, h.homeWorkspace, h.sixDir, h.configPath, h.frameCharSet,
		h, h, h.scheduleNextTick, parser,
		editorMode, autoInstall, h.afterPackageConfigMerge)
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
	diagnosticsSource lspcmd.DiagnosticsSource,
) lspcmd.Config {
	cmdcfg := lspcmd.DefaultConfig()
	cmdcfg.RootURI = uri
	cmdcfg.Parser = parser
	cmdcfg.ScheduleNextTick = cfg.scheduleNextTick
	cmdcfg.DiagnosticsSource = diagnosticsSource
	cmdcfg.Highlight.Delay = 50 * time.Millisecond
	cmdcfg.Highlight.WriteAttr = term.Attributes{Attrs: term.AttrBold | term.AttrUnderline}
	cmdcfg.Highlight.ReadAttr = term.Attributes{Attrs: term.AttrUnderline}
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
