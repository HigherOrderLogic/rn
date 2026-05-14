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
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	handlermarkdown "unstable.build/go-tui/handler/markdown"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/idecursor"
	"unstable.build/go-tui/ide/idedebug"
	"unstable.build/go-tui/ide/idelsp"
	"unstable.build/go-tui/ide/idelsp/lspcmd"
	"unstable.build/go-tui/ide/idemacro"
	"unstable.build/go-tui/ide/ideshell/debugshell"
	"unstable.build/go-tui/ide/ideshell/workspaceshell"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/ide/vctrl/gogit"
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
	cmdMacroRecord       = "record"
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
	storage            storageapi.Service
	ideStorage         storageapi.Service
	workspace          workspace.WorkspaceManager
	clip               clipboard.Register
	macro              *idemacro.Recorder
	macroPlayer        *idemacro.Player
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

	union handler.FrameUnion
	bar   handler.Tabs
	// barIdxToSlot maps a bar tab index (as returned by handler.Tabs.TabAt)
	// to the workspace slot index in h.workspaces. When middle slots are
	// empty (nil), the bar skips them, so the bar tab index does not
	// equal the slot index.
	barIdxToSlot    []int
	focusProxy      handler.Proxy
	width, height   int
	workspaces      [workspaceSlots]*workspaceHandler
	workspaceCount  int
	focus           int
	homeURI         workspaceapi.URI
	homeWorkspace   workspace.Workspace
	empty           *ex
	homeRunner      extension.Runner
	openPrevFiles   []file
	openPrevFilesEx *ex
	openPrevWindows map[uint64]browser.Window
	shaderRunner    *shaderRunner
	// pending tracks workspaces whose Phase B (async build) is in
	// flight. Entries are added in Phase A (under h.mu) and removed
	// in Phase C (also under h.mu). The pending slot index reserves
	// position in h.workspaces so concurrent addWorkspace calls do
	// not collide.
	pending map[string]*pendingWorkspace
	// pendingWG counts in-flight Phase B builds (and the scheduled
	// Phase C closure they enqueue). Tests use it via
	// waitForWorkspaces to block until all installs land. Production
	// code never inspects it.
	pendingWG sync.WaitGroup
}

type openFileTarget struct {
	workspaceIdx int
	workspace    *workspaceHandler
	tab          *browser.Tab
}

// pendingWorkspace describes an addWorkspace request whose Phase B build
// is running in a goroutine. It reserves a slot in h.workspaces so other
// addWorkspace calls (and slot-finding helpers) can see it while the
// async build is in progress, and exposes a cancel hook so that the slot
// can be torn down (e.g. via :workspaceclose) before install finishes.
type pendingWorkspace struct {
	uri       workspaceapi.URI
	slot      int
	cancelCtx func()
	// canceled is set by Phase C / closeWorkspace when the install
	// must be aborted; the goroutine and the install closure both
	// check it before installing the handler.
	canceled atomic.Bool
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
		// Route per-file command handlers (e.g. :gitlink → notify
		// "copied <url>") through the focused workspace's
		// notifications container so the message renders on screen
		// instead of being dropped into nopNotifications{}.
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

// newCommandPromptEditor returns the command.Editor adapter used by
// the command Prompt's modal edit mode. It bypasses text.Editor.Edit
// (which layers status / icons / location / aux bars on top of the
// bare buffer view, displacing the cursor coordinates the prompt
// reads back) and instantiates either a vi.Vi or a modeless editor
// handler directly, mirroring the editor mode the user has selected
// for the rest of the IDE.
func (h *workspaceManagerHandler) newCommandPromptEditor(
	cfg ideConfig,
) command.Editor {
	switch cfg.editorMode() {
	case editorModeModeless:
		return modelessCommandPromptEditor{
			tabspaces:        cfg.editorTabspaces(),
			indents:          cfg.editorIndents(),
			scheduleNextTick: cfg.scheduleNextTick,
			clipboard:        h.clip,
			autoPair:         cfg.editorAutoPair(),
		}
	case editorModeModal:
		return viCommandPromptEditor{
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

// modelessCommandPromptEditor and viCommandPromptEditor are bare
// command.Editor adapters: each Edit invocation spins up a fresh
// editor handler bound to the supplied buffer, with no bar chrome.
type modelessCommandPromptEditor struct {
	tabspaces        int
	indents          text.IndentConfig
	scheduleNextTick func(func()) bool
	clipboard        clipboard.Register
	autoPair         bool
}

func (m modelessCommandPromptEditor) Edit(buf *cell.Buffer) command.EditHandler {
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

type viCommandPromptEditor struct {
	tabspaces        int
	indents          text.IndentConfig
	scheduleNextTick func(func()) bool
	clipboard        clipboard.Register
	autoPair         bool
}

func (v viCommandPromptEditor) Edit(buf *cell.Buffer) command.EditHandler {
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
	dispatchOnPreview map[string]PreviewFunc,
) (err error) {
	ctx := context.Background()

	h.storage = storage
	h.ideStorage = storageapi.WithPartition(h.storage, "ide")
	// Ensure ideConfig — used to assemble alias completer chains during
	// textOpts — can resolve `{history}` against the same partitioned
	// storage the command Prompt writes to. The Prompt persists command
	// history under the "ide" partition; the {history} completer must
	// read from the same partition or it will see an empty document.
	cfg.storage = h.ideStorage
	interrupter := term.FuncInterrupter(func(ctx context.Context) error {
		payload, _ := term.PayloadFromContext(ctx)
		if !h.publishEvent(term.Event{Type: term.EventInterrupt, Raw: payload, Context: ctx}) {
			return errEventStreamNotReady
		}
		return nil
	})
	h.frameCharSet = cfg.windowFrameCharset()
	notiConfig.Interrupter = interrupter
	h.notifications = newWorkspaceNotifications(h.ideStorage, notiConfig, h)
	h.shaderRunner = shaderRunner
	h.mu = locker
	h.externalCommands = make(map[string]externalCommand)
	h.frame = cfg.frame()
	// The host event loop (term/gui/gui.go's (*GUI).Update,
	// rune-go-sdk's tui.Run) is contracted to hold the shared IDE
	// locker around every UserFunc it dispatches. Callbacks
	// scheduled via cfg.scheduleNextTick therefore already run
	// serialized with the rest of the IDE state machine — no
	// extra wrapping required, and any extra h.mu.Lock() here
	// would self-deadlock the event loop because *sync.Mutex is
	// not reentrant.
	h.scheduleNextTick = cfg.scheduleNextTick
	h.configPath = cfg.configPath
	h.reloadConfig = reloadConfig
	h.workspaceConfigFilename = workspaceConfigFilename
	h.tabsClickCallback = tabsClickCallback
	h.publishEvent = publishEvent
	h.workspace = manager
	h.clip = cfg.clipboard()
	h.macro = idemacro.New(h.clip, h.notifications.current(), cfg.commandKey())
	h.macroPlayer = idemacro.NewPlayer(h.clip, h.macro, h.publishEvent)
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
	h.pending = make(map[string]*pendingWorkspace)

	homeWorkspace, err := h.workspace.AddWorkspace(ctx, homeDirUri)
	if err != nil {
		return fmt.Errorf("add home workspace: %v", err)
	}

	h.homeURI = homeDirUri
	h.homeWorkspace = homeWorkspace
	h.setReleaseManager(releaseManager)

	// don't install a fs watcher for the home workspace,
	// to prevent unecessary resource consumption
	homeParser := syntax.NewParser(h.homeWorkspace, h.pkgmanager, h.homeURI)
	globalOpts := h.textOpts(cfg, homeParser)
	// do not pass a real version control for home workspace
	ed, err := h.newEditor(homeDirUri, cfg, vctrl.NopService())
	if err != nil {
		return fmt.Errorf("new editor: %v", err)
	}

	tm := new(workspaceTabManager)
	tm.parent = h
	h.empty, err = newEx(ed, homeWorkspace, h.ideStorage, h.notifications, h.homeURI,
		cfg.terminalConfig(), cfg.pluginBarConfig(),
		h.publishEvent, 0 /* vte capacity */, h.clip, h.macro,
		h.dispatchOnPreview, tm, homeParser, globalOpts...)
	if err != nil {
		return fmt.Errorf("new ex: %w", err)
	}
	h.empty.commandEditor = h.newCommandPromptEditor(cfg)
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
	runner, err := h.buildExtensions(cfg, homeDirUri, trackedCwd, h.empty, extExec)
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
		// speed up initialization by initializing extensions asynchronously
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
	h.history = newHistory(h.ideStorage)

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
	h.focusProxy.Target = h.focusHandler()
	// resize for bottom workspace bar to disappear
	h.Resize(h.width, h.height)
	return true
}

// onBarTabClick translates a bar tab index (sequential among visible
// tabs) to the workspace slot index before switching. The bar may skip
// empty middle slots, so a direct slot lookup would otherwise focus the
// wrong workspace.
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
	cfg ideConfig, parser syntaxapi.Parser,
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
		text.WithClipboard(h.clip),
		text.WithOpenRouter(h),
		text.WithFileExplorerIndentAttr(cfg.fileExplorerIndentAttr()),
		text.WithFileExplorerIconAttr(cfg.fileExplorerIconAttr()),
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

// addWorkspace begins loading a workspace for the given URI. It is split
// into three phases:
//
//  1. Phase A (synchronous, on the event loop, holding h.mu when called
//     by init): validate the request, deduplicate by URI, reserve a
//     pending slot, and create the underlying workspace.Workspace via
//     the manager. None of these steps may block on remote IO.
//
//  2. Phase B (goroutine): perform the blocking work — load the
//     workspace overlay config (cwd.OpenFile), construct the git
//     vctrl.Service (cwd.Stat), build the editor / ex / cursor history
//     / extensions runner. This is where SSH workspaces typically wait
//     for the first connection attempt and any associated UI prompts.
//
//  3. Phase C (scheduled back onto the event loop via scheduleNextTick):
//     install the constructed workspaceHandler into h.workspaces,
//     subscribe commands/events, switch focus, spawn the FS watcher
//     goroutine and run session restore.
//
// addWorkspace returns synchronously after Phase A so the event loop is
// not blocked while Phase B is in progress; if Phase A fails the caller
// receives an error immediately. Phase B/C errors are surfaced via the
// notifications system since the caller has already returned.
//
// The slot index argument follows the original semantics: -1 picks the
// next available slot; otherwise the given slot is reserved (used by
// commandReloadWorkspace which needs to land back in the same slot).
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
		// scheduleNextTick may legitimately return false if the
		// host event loop is not running (e.g. tests that exercise
		// the IDE without tui.Run, or the loop has already exited).
		// In that case the install closure never runs, so we still
		// need to release pendingWG and tear down the reservation —
		// otherwise drainPendingWorkspaces would hang forever and a
		// reserved-but-empty slot would leak in h.pending.
		scheduled := h.scheduleNextTick(func() {
			defer h.pendingWG.Done()
			h.installPendingWorkspace(pending, uri, ctx, cancel,
				cwd, built, buildErr, shouldRestore, promptRecommended)
		})
		if !scheduled {
			h.mu.Lock()
			delete(h.pending, uri.String())
			h.mu.Unlock()
			cancel()
			h.pendingWG.Done()
		}
	})
	return nil
}

// findInstalledSlot returns the index of an already-installed workspace
// whose URI matches uri.
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

// isPending reports whether a Phase B build is currently running for
// the given URI.
func (h *workspaceManagerHandler) isPending(uri workspaceapi.URI) bool {
	_, ok := h.pending[uri.String()]
	return ok
}

// createWorkspaceScheme is Phase A's only call into workspace.Manager.
// It constructs the underlying workspace.Workspace plus a per-handler
// lifetime context that the Phase B goroutine and the FS watcher
// share — distinct from the *scheme's* parent context.
//
// The scheme is always built with context.Background() as its parent.
// Schemes manage their own internal lifecycle via Close() (e.g.
// remoteScheme.Close cancels its own ctx, closes the ssh session and
// stops the maintainConnection goroutine), and workspace.Manager
// caches schemes by URI: closeWorkspaceKeepScheme on :workspacereload
// deliberately reuses the cached entry, so cancelling the per-handler
// ctx must NOT propagate into the scheme. Otherwise the next dial /
// stat / pty call short-circuits with context.Canceled before it
// reaches the wire (the bug TestIntegrationIDEWorkspaceReloadOverSSH
// reproduces).
//
// workspace.Manager.AddWorkspace itself does not block on remote IO —
// for SSH it returns a remoteScheme whose first connection attempt
// happens in a background goroutine — so this is safe to call from
// the event loop.
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

// reservePendingSlot picks (or validates) the slot the workspace will
// occupy and records a pendingWorkspace entry so concurrent
// addWorkspace calls and nextAvailableWorkspace see the slot as taken.
// It must be called with whatever locking discipline the caller already
// uses (init holds h.mu; commandAddWorkspace runs single-threaded on
// the event loop).
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
	return pending, nil
}

// builtWorkspace bundles the heavyweight artifacts produced by Phase B
// so installPendingWorkspace can install them atomically on the event
// loop.
type builtWorkspace struct {
	cfg                 ideConfig
	configErr           error
	wh                  *workspaceHandler
	ex                  *ex
	runner              extension.Runner
	cursorHistoryCloser io.Closer
}

// buildWorkspaceAsync runs the blocking IO and heavy construction in a
// goroutine. It MUST NOT touch h.workspaces, h.pending or any other
// event-loop-owned state directly: results are handed back via the
// returned builtWorkspace and consumed under the event-loop lock by
// installPendingWorkspace.
func (h *workspaceManagerHandler) buildWorkspaceAsync(
	uri workspaceapi.URI, cwd workspace.Workspace,
	pending *pendingWorkspace,
) (*builtWorkspace, error) {
	cfg, configErr := h.reloadConfig()
	// Ensure ideConfig used for textOpts can resolve `{history}` placeholders
	// in command alias completer chains. Use the same partitioned storage
	// the command Prompt uses so the doc IDs line up.
	cfg.storage = h.ideStorage
	// reloadConfig returns a fresh ideConfig that didn't go through
	// ide.New / ide.WithScheduleNextTick wiring, so propagate the
	// host scheduler captured in init. Downstream consumers
	// (debugshell, lsp, etc.) require it to be non-nil.
	cfg.scheduleNextTick = h.scheduleNextTick
	if _, wConfigErr := loadWorkspaceConfig(
		h.workspaceConfigFilename, cwd, uri, &cfg,
	); wConfigErr != nil {
		configErr = multierror.Append(configErr,
			fmt.Errorf("workspace config: %w", wConfigErr))
	}

	parser := syntax.NewParser(cwd, h.pkgmanager, uri)
	textOpts := h.textOpts(cfg, parser)
	// Always attempt to construct a real vctrl.Service so file-level
	// git commands (e.g. :gitlink) work even when the user has not
	// enabled aux-bar git decorations. gogit.NewService falls back to
	// a lazy lookup when no .git is found at the workspace root, so
	// it is safe to call unconditionally on non-git workspaces.
	vctrlService, err := gogit.NewService(uri, cwd)
	if err != nil {
		h.empty.log(log.ErrorLevel, "new git service for workspace %q: %v",
			uri.Path(), err)
		vctrlService = vctrl.NopService()
	} else {
		vctrlService = vctrl.SyncService(vctrlService, new(sync.Mutex))
	}

	ed, err := h.newEditor(uri, cfg, vctrlService)
	if err != nil {
		return nil, fmt.Errorf("new editor: %w", err)
	}

	// workspace capable of opening URIs other than the workspaceapi.URI
	// while only routing to other currently visible IDE workspaces.
	visibleManager := visibleWorkspaceManager{parent: h, manager: h.workspace}
	// Multi may construct *new* schemes for URIs outside the
	// workspace (loadExtraneous / recoverExtraneous below). Those
	// schemes are cached by workspace.Manager and outlive any
	// individual workspaceHandler — ditto the rationale in
	// createWorkspaceScheme: the scheme parent must be Background
	// so :workspacereload tearing down ctx never leaves a cached
	// extraneous scheme with a canceled context.
	multicwd := workspace.Multi(context.Background(), visibleManager, cwd, uri)
	tm := new(workspaceTabManager)
	tm.parent = h
	ex, err := newEx(ed, multicwd, h.ideStorage, h.notifications, uri,
		cfg.terminalConfig(), cfg.pluginBarConfig(), h.publishEvent,
		h.initialVTECapacity, h.clip, h.macro, h.dispatchOnPreview,
		tm, parser, textOpts...)
	if err != nil {
		return nil, fmt.Errorf("new ex: %w", err)
	}
	ex.commandEditor = h.newCommandPromptEditor(cfg)
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
	runner, err := h.buildExtensions(cfg, uri, trackedCwd, ex, extExec)
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
	}

	built := &builtWorkspace{
		cfg:                 cfg,
		configErr:           configErr,
		wh:                  wh,
		ex:                  ex,
		runner:              runner,
		cursorHistoryCloser: cursorHistoryCloser,
	}
	return built, nil
}

// installPendingWorkspace runs on the event loop after Phase B
// completes. It either tears down the partial state on error, or
// installs the constructed workspaceHandler into h.workspaces, runs
// command/event subscription, switches focus, spawns the FS watcher
// goroutine and runs session restore.
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
	// Phase C is invoked from the host event loop's UserFunc
	// dispatch (gui.Update / tui.Run), which already holds h.mu —
	// so we run with the IDE lock held without re-locking here.
	delete(h.pending, uri.String())

	// Stop the loading shader at the very end of this Phase C, *in the
	// same event-loop turn*. We previously deferred stopLoading through
	// scheduleNextTick so the new workspace would Draw once before the
	// open shader took over, but that drew the freshly-installed
	// workspace in full color for a frame between the gray-fade loading
	// shader and the burn open shader, breaking the chained transition.
	// captureOpenShaderCells already snapshots the pre-switch screen
	// before switchToWorkspace, so the burn has everything it needs to
	// start immediately and there is no reason to give the new workspace
	// a frame to render first. If no loading/open shader is configured,
	// this is a no-op.
	defer h.shaderRunner.stopLoading()

	// closeWorkspace can flag the pending entry as canceled while
	// Phase B is in progress; in that case the cancelCtx has already
	// been called. Drop the partially built artifacts so we don't
	// install a workspace the user closed.
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

	// subscribeAllCommands / subscribeAllEvents touch event-loop-owned
	// state (per-ex command registries, the publisher), so they live in
	// Phase C even though they are CPU-only.
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

	// FS watcher goroutine — we keep this naked goroutine tied to ctx
	// rather than to pending so that closeWorkspace's cancel() shuts
	// it down naturally.
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

	h.logNonFatalErrs(wh.Browser(), built.configErr, built.cfg.errors)

	prevSessionFiles, hasTermSessions, hasTaskSessions, layout, hasLayout, err :=
		h.resolveSessionRestoreState(ex, uri, shouldRestore)
	if err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"resolve session state for %s: %v", uri.String(), err)
		return
	}
	if !shouldRestore || (len(prevSessionFiles) == 0 && !hasTermSessions && !hasTaskSessions) {
		return
	}
	if promptRecommended && !built.cfg.autoRestore() {
		h.openRestorePrompt(ex, uri, prevSessionFiles, hasTermSessions,
			hasTaskSessions, layout, hasLayout)
		return
	}
	if err := h.restorePreviousSession(ex, prevSessionFiles, hasTermSessions,
		hasTaskSessions, layout, hasLayout); err != nil {
		_, _ = h.notifications.current().Notify(browserapi.LevelError,
			"restore previous session: %v", err)
	}
}

// resolveSessionRestoreState consults stored session metadata for uri
// and either returns previously open files / windows / layouts (when
// shouldRestore is true) or clears them (when shouldRestore is false,
// e.g. autoRestore disabled).
func (h *workspaceManagerHandler) resolveSessionRestoreState(
	ex *ex, uri workspaceapi.URI, shouldRestore bool,
) (
	prevSessionFiles []file,
	hasTermSessions bool,
	hasTaskSessions bool,
	layout tcomponent.TileLayout,
	hasLayout bool,
	err error,
) {
	prevSessionFiles = h.history.recordAddWorkspace(uri, ex.Editor(), shouldRestore)
	if shouldRestore {
		hasTermSessions, err = ex.hasOpenTerminalSessions(context.Background())
		if err != nil {
			err = fmt.Errorf("load open terminal sessions: %w", err)
			return
		}
		hasTaskSessions, err = ex.hasOpenTaskSessions(context.Background())
		if err != nil {
			err = fmt.Errorf("load open task sessions: %w", err)
			return
		}
		layout, hasLayout, err = ex.loadWorkspaceLayout(context.Background())
		if err != nil {
			err = fmt.Errorf("load workspace layout: %w", err)
			return
		}
		return
	}
	if cerr := ex.clearOpenTerminalSessions(context.Background()); cerr != nil {
		err = fmt.Errorf("clear open terminal sessions: %w", cerr)
		return
	}
	if cerr := ex.clearOpenTaskSessions(context.Background()); cerr != nil {
		err = fmt.Errorf("clear open task sessions: %w", cerr)
		return
	}
	if cerr := ex.clearWorkspaceLayout(context.Background()); cerr != nil {
		err = fmt.Errorf("clear workspace layout: %w", cerr)
		return
	}
	return
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
) (extension.Runner, error) {
	notifications := h.notifications.new(uri, ex.container)
	ed := ex.Editor()
	promptOpener := &ex.comp
	promptStorage := storageapi.WithPartition(h.storage, "extension-permissions")
	cmdAuthorizer, err := ideauthorizer.NewAuthorizer(
		ed, promptOpener, promptStorage, cfg.scheduleNextTick, notifications)
	if err != nil {
		return nil, fmt.Errorf("new command authorizer: %w", err)
	}
	res := extension.BrowserResources(ex.Browser(), h.publishEvent)
	res = extension.MergeResourceMap(res,
		extension.EditorResources(ex.Browser(), ed, h.publishEvent))
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
	}
	lsp := idelsp.New(uri, cwd,
		cwd, h.pkgmanager, notifications,
		ex.Browser(), lspConfig)
	// Debug adapter manager. Session creation is owned by the
	// consumer (typically agentshell); the Manager itself holds
	// no "current" session and routes every call by sessionID.
	dapCfg := idedebug.Config{
		MaxRetries: 5,
		Adapters:   cfg.debuggerConfigs(),
	}
	dap := idedebug.New(uri, cwd, h.pkgmanager, dapCfg)
	err = ex.comp.SubscribeEvents(idelsp.EditorEvents(), lsp)
	if err != nil {
		log.Errorf("subscribe LSP manager: %v", err)
	}
	// Register the top-level "debugger" REPL command and its
	// command-prompt handler. debugshell.Handler owns the active
	// debug session lifecycle (initialize, launch, attach,
	// terminate) and forwards DAP events back to the REPL.
	dbgHandler := debugshell.New(dap, &ex.comp, apieditor, debugshell.Config{
		WorkspaceURI: uri,
		Icons: debugshell.Icons{
			Breakpoint: "",
			Stopped:    "",
		},
		Debugger:         dapCfg,
		ScheduleNextTick: cfg.scheduleNextTick,
	}).WithNotify(func(level browserapi.NotificationLevel, msg string, args ...any) {
		_, _ = notifications.Notify(level, msg, args...)
	}).WithParser(parser)
	dbgMan := debugshell.Manual()
	if err := ex.comp.RegisterREPLCommand(dbgMan, dbgHandler); err != nil {
		log.Errorf("register debugger repl command: %v", err)
	}
	if err := ex.comp.SubscribeCommand(dbgMan,
		debugshell.NewPromptHandler(dbgHandler).
			WithOpenShell(ex.shellnewtab)); err != nil {
		log.Errorf("subscribe debugger command prompt: %v", err)
	}
	cmdcfg := lspCommandsConfig(uri, cfg, notifications, h, parser, callbacks)
	apiHandler, err := lspcmd.AllHandler(
		lsp, apieditor, apibrowser, apibrowser, apibrowser,
		ex.workspace, parser, cmdcfg)
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
	browser := ex.Browser()
	grantor := newExtensionPromptGrantor(promptOpener, promptStorage, cfg.scheduleNextTick)
	runner, err := h.extensionRunner.WorkspaceExtensionsRunner(uri, res, cmdAuthorizer,
		dataDir, browser, cwd, extExecutor, grantor,
		ed, promptOpener, promptStorage, cfg.scheduleNextTick)
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
	ex *ex, files []file, windows map[uint64]browser.Window,
) (err error) {
	invokeWindow := ex.invokeWindow()
	for _, f := range files {
		// Skip the file explorer singleton: it is re-opened via
		// :fexplorer and must not be restored as a regular tab,
		// otherwise ex.initFileExplorer would call Edit a second
		// time on the same URI and fail with
		// "command already registered".
		if f.URIString == fileExplorerURI {
			continue
		}
		uri, uerr := workspaceapi.ParseURI(f.URIString)
		// do not hard error, otherwise changes to storage representation
		// could prevent user from opening editor at all
		if uerr != nil {
			log.Warnf("parse uri from previous session file: %v", uerr)
			continue
		}
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
		ed := t.Handler().(text.Handler)
		ed.SetCursorAtScroll(f.Cursor)
	}
	return err
}

func (h *workspaceManagerHandler) restorePreviousSession(
	ex *ex,
	files []file,
	restoreTerminals bool,
	restoreTasks bool,
	layout tcomponent.TileLayout,
	hasLayout bool,
) error {
	ret := new(multierror.Error)
	layout.Floating = nil
	windows := h.restoreWorkspaceWindows(ex, files, restoreTerminals,
		layout, hasLayout)
	if restoreTerminals {
		ret = multierror.Append(ret,
			ex.restoreOpenTerminalSessions(context.Background(), windows))
	}
	if restoreTasks {
		ret = multierror.Append(ret,
			ex.restoreOpenTaskSessions(context.Background()))
	}
	if len(files) == 0 {
		return ret.ErrorOrNil()
	}
	if h.width == 0 || h.height == 0 {
		// if restoreSession is called on an size 0,0 handler
		// then cursor is not properly set.
		h.openPrevFiles = files
		h.openPrevWindows = windows
		h.openPrevFilesEx = ex
		return ret.ErrorOrNil()
	}
	ret = multierror.Append(ret, h.openPrevSessionFiles(ex, files, windows))
	return ret.ErrorOrNil()
}

func (h *workspaceManagerHandler) restoreWorkspaceWindows(
	ex *ex,
	files []file,
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
	workspaceURI, _, err := h.closeWorkspaceKeepScheme()
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
	return h.doCloseWorkspace(true)
}

// closeWorkspaceKeepScheme is like closeWorkspace but does NOT remove the
// backing workspace from workspace.Manager. This is used by
// commandReloadWorkspace, where the workspace is immediately re-added under
// the same URI; keeping the Manager entry means AddWorkspace returns the
// same managerWorkspace and the existing scheme (e.g. an in-memory scheme
// holding live buffer content) survives the reload.
func (h *workspaceManagerHandler) closeWorkspaceKeepScheme() (
	workspaceapi.URI, []workspaceapi.URI, error,
) {
	return h.doCloseWorkspace(false)
}

func (h *workspaceManagerHandler) doCloseWorkspace(removeFromManager bool) (
	workspaceapi.URI, []workspaceapi.URI, error,
) {
	// If the focused slot is currently a pending build (Phase B in
	// progress), tear it down: cancel the context so any blocking IO
	// returns and mark the pending entry so installPendingWorkspace
	// drops the partial build instead of installing it.
	if pending, ok := h.pendingForFocus(); ok {
		uri := pending.uri
		pending.canceled.Store(true)
		pending.cancelCtx()
		delete(h.pending, uri.String())
		h.history.recordCloseWorkspace(uri)
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

	h.history.recordWorkspaceFileWindows(uri, hm.ex.fileWindowIDs())
	h.history.recordCloseWorkspace(uri)
	var err error
	if removeFromManager {
		err = hm.closeAndRemove()
	} else {
		err = hm.Close()
	}
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
	// Cancel any in-flight pending builds so their goroutines can
	// exit and their finalize closures (when they eventually run) see
	// canceled and discard the partially built artifacts instead of
	// installing them after Close.
	for k, p := range h.pending {
		p.canceled.Store(true)
		p.cancelCtx()
		delete(h.pending, k)
	}
	for _, hm := range h.workspaces {
		if hm == nil {
			continue
		}
		h.history.recordWorkspaceFileWindows(hm.uri, hm.ex.fileWindowIDs())
		h.history.recordCloseWorkspace(hm.uri)
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
	if h.pkgmanager != nil {
		if err := h.pkgmanager.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if err := h.ideStorage.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	if err := h.storage.Close(); err != nil {
		ret = multierror.Append(ret, err)
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
	// cwd is the managerWorkspace returned by workspace.Manager.AddWorkspace.
	// It is closed in closeAndRemove (the single-workspace close path) so
	// its underlying scheme can be dropped from workspace.Manager and
	// garbage-collected. It is NOT closed in Close(), which is also invoked
	// on full-IDE shutdown where the workspace.Manager is owned by the
	// caller.
	cwd       workspace.Workspace
	closeOnce sync.Once
	closeErr  error
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
		// cancel at the end, so fs event processing is not
		// vacated before everything else is still potentially
		// sending events (i.e. mem scheme)
		hm.cancelCtx()
		hm.closeErr = ret
	})
	return hm.closeErr
}

// closeAndRemove closes this handler and also removes the backing workspace
// from the owning workspace.Manager so the scheme it owns can be
// garbage-collected. This is the right teardown when a single workspace is
// closed while the rest of the IDE stays alive (e.g. via :close). It must
// NOT be used on full-IDE shutdown, because the Manager is owned by the
// caller and survives the handler.
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
		Synopsis: "[register]",
	}, h.macro)
	if err != nil {
		return fmt.Errorf("subscribe macro commands: %w", err)
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

func (h *workspaceManagerHandler) subscribeAllEvents(
	cfg ideConfig, ex *ex,
) error {
	if err := h.subscribeExternalEvents(ex, h.externalEvents...); err != nil {
		return err
	}
	if cfg.editorAutoSave() {
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

func (h *workspaceManagerHandler) Interrupt(ctx context.Context) error {
	payload, _ := term.PayloadFromContext(ctx)
	ev := term.Event{Type: term.EventInterrupt, Raw: payload, Context: ctx}
	if !h.publishEvent(ev) {
		return errEventStreamNotReady
	}
	return nil
}

// waitInflight blocks until every workspace's in-flight async save /
// reload completion goroutines have delivered their callbacks via
// sched. Intended for tests and graceful shutdown.
func (h *workspaceManagerHandler) waitInflight() {
	h.mu.Lock()
	exes := make([]*ex, 0, len(h.workspaces))
	for _, w := range h.workspaces {
		if w == nil || w.ex == nil {
			continue
		}
		exes = append(exes, w.ex)
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
		// do this once only when initializing pgmanager for the first time
		// these don't require release.Manager so it's ok to do it
		// with the initial release.Manager.
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
	h.pkgmanager.init(notifications, releaseManager, wm,
		h.ideStorage, h.homeWorkspace, h.sixDir, h.configPath, h.frameCharSet,
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
	diagnosticsSource lspcmd.DiagnosticsSource,
) lspcmd.Config {
	cmdcfg := lspcmd.DefaultConfig()
	cmdcfg.RootURI = uri
	cmdcfg.Parser = parser
	cmdcfg.ScheduleNextTick = cfg.scheduleNextTick
	cmdcfg.DiagnosticsSource = diagnosticsSource
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
