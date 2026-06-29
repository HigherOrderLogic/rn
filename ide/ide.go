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
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path"
	"sync"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide/ideplan"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacessh"
)

// IDE encapsulates the ability to run an IDE within a TUI session.
type IDE struct {
	ideConfig
	options
	locker           sync.Locker
	workspaceManager *workspace.Manager
	workspaceHandler *workspaceManagerHandler
	root             shaderRunner
	tutorial         tutorialRunner
	tutorialsConfig  tutorialsConfig
	planLockdown     *planLockdownRunner
	planMonitor      *ideplan.Monitor
	publishEventFn   EventPublisher
	storage          storageapi.Service
}

// EventPublisher is a function that publishes the given event back
// into the event loop.
type EventPublisher func(term.Event) bool

// New allocates storage for a new IDE and initializes it with config
// at cfgfilename and filename. Note that if filename is empty, a default inmutable
// buffer will be loaded.
//
// storage is borrowed, not owned: the caller retains ownership and must
// close it. IDE.Close does not close storage, so a single storage handle
// may be shared across IDEs (as cmd/rune's bootstrap swap does) without one
// IDE's shutdown tearing it down underneath another.
func New(
	cwd, cfgfilename, dataDir string, storage storageapi.Service, opts ...Option,
) (i *IDE, err error) {
	i = new(IDE)
	err = i.init(cwd, cfgfilename, dataDir, storage, opts...)
	return
}

// Config loads and returns the IDE configuration without starting workspaces.
func Config(cfgfilename string, opts ...Option) (config.Config, error) {
	op := newOptions(opts...)
	cfg, err := loadIDEConfig(cfgfilename, op)
	return config.MapConfig(cfg.cfg), err
}

// Interrupt satisfies term.Interrupter
func (i *IDE) Interrupt(ctx context.Context) error {
	return i.workspaceHandler.Interrupt(ctx)
}

// SubscribeCommand subscribes the given handler in calls to the given cmd,
// or returns an error if there's already a CommandHandler
// installed for this command.
//
// The command will be automatically installed to all active
// and future workspaces.
func (c *IDE) SubscribeCommand(
	cmd textapi.CommandManual, handler text.CommandHandler,
) error {
	return c.workspaceHandler.subscribeCommand(cmd, handler)
}

// RegisterREPLCommand registers the given handler as a top-level REPL
// command in the IDE shell, or returns an error if a REPL command with
// the same name is already registered.
//
// The command will be automatically installed to all active
// and future workspaces.
func (c *IDE) RegisterREPLCommand(
	cmd textapi.CommandManual, handler textapi.REPLHandler,
) error {
	return c.workspaceHandler.registerREPLCommand(cmd, handler)
}

// SubscribeEvents subscribes the given handler to all the given events,
// or returns an error if there's an error while subscribing it.
//
// The command will be automatically installed to all active
// and future workspaces.
func (c *IDE) SubscribeEvents(
	events []textapi.EventType, handler text.EventHandler,
) error {
	c.workspaceHandler.mu.Lock()
	defer c.workspaceHandler.mu.Unlock()
	return c.workspaceHandler.SubscribeEvents(events, handler)
}

// UnsubscribeEvents unsubscribes the given handler to all the events.
func (c *IDE) UnsubscribeEvents(handler text.EventHandler) (bool, error) {
	c.workspaceHandler.mu.Lock()
	defer c.workspaceHandler.mu.Unlock()
	return c.workspaceHandler.UnsubscribeEvents(handler)
}

// DefaultAttributes return the default attributes to be used to fill the screen.
func (i *IDE) DefaultAttributes() term.Attributes {
	return i.root.defAttr
}

// InputMode return the configured tui input mode.
func (i *IDE) InputMode() term.InputMode {
	return i.ideConfig.inputMode()
}

// SetDefaultAttributes sets the default attributes to be used to fill the screen.
func (i *IDE) SetDefaultAttributes(defAttr term.Attributes) {
	i.root.defAttr = defAttr
	// Propagate to every registered tutorial so per-step shaders
	// (e.g. floating_window hint blinks) blend against the active
	// theme. Tutorials snapshot their
	// host services at build time, but the IDE's default
	// attribute pair is theme-time, not configuration-time, so it
	// has to flow through the live setter.
	i.tutorial.setDefaultAttributes(defAttr)
}

// Config returns the configuration loaded by this IDE.
func (i *IDE) Config() config.Config {
	return config.MapConfig(i.ideConfig.cfg)
}

// Ready returns the root Handler of this IDE,
// and marks this IDE as ready to run.
// Close must be called when this IDE is no longer in use.
func (i *IDE) Ready() tui.Handler {
	i.initRunning()
	i.maybeStartTutorial()
	i.maybeOpenHomePrompt()
	return i.planLockdown
}

// tutorialInitShaderBuffer is an extra delay added on top of the init
// shader duration before the tutorial is dispatched, so the prompt
// mounts only after the shader has fully settled.
const tutorialInitShaderBuffer = 2 * time.Second

func (i *IDE) maybeStartTutorial() {
	name := i.options.startingTutorial
	if name == "" {
		return
	}
	if _, ok := i.tutorial.tutorials[name]; !ok {
		return
	}
	dispatch := func() {
		i.options.scheduleFn(func() {
			i.workspaceHandler.focusEx().Dispatch("tutorial", "start", name)
		})
	}
	// Defer the tutorial until the init shader finishes so the shader
	// does not animate on top of the freshly-mounted tutorial prompt.
	if i.options.initShaderFn != nil && i.options.initShaderDuration > 0 {
		i.options.afterFunc(i.options.initShaderDuration+tutorialInitShaderBuffer, func() {
			debug.CapturePanicReport(dispatch)
		})
		return
	}
	dispatch()
}

// maybeOpenHomePrompt pre-opens the command prompt with the
// workspaceopen command when the user lands on the home workspace and no
// first-run tutorial is configured. The home workspace is not a real
// workspace, so this lets returning users immediately fuzzy-search and
// re-open a previously opened workspace from command history.
func (i *IDE) maybeOpenHomePrompt() {
	if i.options.disableHomePrompt {
		return
	}
	// The tutorial owns the prompt during first-run onboarding; avoid a
	// double-prompt by deferring to it when it will start.
	if name := i.options.startingTutorial; name != "" {
		if _, ok := i.tutorial.tutorials[name]; ok {
			return
		}
	}
	// A workspace requested at launch installs asynchronously, so the
	// home workspace may still be focused at Ready; skip in that case so
	// the prompt does not open over the workspace that is loading in.
	if i.workspaceHandler.startupWorkspace {
		return
	}
	if !i.workspaceHandler.focusEx().home {
		return
	}
	dispatch := func() {
		i.options.scheduleFn(func() {
			ex := i.workspaceHandler.focusEx()
			// The pre-open is a convenience, not a takeover: if the user
			// already opened the command prompt themselves before this
			// deferred dispatch ran, leave their prompt untouched.
			if ex.cmd != nil {
				return
			}
			ex.Dispatch("echo", "{prompt}workspaceopen<space>")
		})
	}
	// Defer behind the init shader the same way maybeStartTutorial does so
	// the shader does not animate on top of the freshly-mounted prompt.
	if i.options.initShaderFn != nil && i.options.initShaderDuration > 0 {
		i.options.afterFunc(i.options.initShaderDuration+tutorialInitShaderBuffer, func() {
			debug.CapturePanicReport(dispatch)
		})
		return
	}
	dispatch()
}

// Browser returns the current browser in focus.
func (i *IDE) Browser() browser.Browser {
	return i.workspaceHandler.focusBrowser()
}

// WindowManager returns a browserapi.WindowManager that routes calls
// to the workspace currently in focus. Use this when registering UI
// elements (e.g. floating prompts) that need to remain attached to
// the focused workspace as the user switches between them.
func (i *IDE) WindowManager() browserapi.WindowManager {
	return currentWorkspaceWindowManager{root: i.workspaceHandler}
}

// Storage returns persistent storage acrosss IDE instances, given
// the same data dir passed in ide.New, or ide.NewRecovery.
func (i *IDE) Storage() storageapi.Service {
	return i.storage
}

// Size returns the current width and height in cells.
func (i *IDE) Size() (width, height int) {
	return i.root.width, i.root.height
}

// Open opens the given file, in the currently active workspace.
func (i *IDE) Open(file workspaceapi.URI) error {
	return i.workspaceHandler.openURI(file, true /* focus */)
}

// WaitWorkspaces blocks until every async addWorkspace launched by
// the IDE has either installed its workspace or had its pending
// reservation cleaned up. Use this after constructing the IDE — and
// before driving keyboard input or calling Open — when you want to
// be sure the cwd workspace is fully wired in. Production code does
// not need it: the event loop pumps install callbacks naturally as
// part of its tick. Callers must NOT hold the IDE locker (set via
// WithLocker) when invoking this — installs need to acquire it.
func (i *IDE) WaitWorkspaces() {
	i.workspaceHandler.pendingWG.Wait()
}

// WaitInflight blocks until every active workspace's in-flight async
// save / reload awaiter goroutines have completed and their completion
// callbacks have been dispatched to the event-loop scheduler. Intended
// for tests that assert post-:write state in the same input turn (or
// during shutdown when callers need a strict drain).
//
// Callers must not hold the IDE locker when invoking this.
func (i *IDE) WaitInflight() {
	i.workspaceHandler.waitInflight()
}

// SetReleaseManager sets the release.Manager of the IDE.
// This should be called before Run or Handler are called for the first time.
func (i *IDE) SetReleaseManager(m release.Manager) {
	i.workspaceHandler.setReleaseManager(m)
}

// PlanSourceConfig collects the dependencies WithPlanSource needs to
// wire the lockdown overlay and monitor. CheckoutURL is the checkout
// URL the Upgrade button opens; OnReSignIn purges the cached token
// and kicks a fresh Login so the user can re-auth (possibly as a
// paid account or a different user).
type PlanSourceConfig struct {
	Source      ideplan.Source
	CheckoutURL string
	OnReSignIn  func()
}

// installPlanSource wires cfg into the existing planLockdown
// wrapper, styles the lockdown prompt using the workspace's
// configured frame and prompt colors, and starts the daily monitor.
func (i *IDE) initPlanSource(cfg PlanSourceConfig) {
	noti := i.workspaceHandler.notifications.current()
	pcfg := i.ideConfig.promptConfig()
	deps := planLockdownPromptDeps{
		checkoutURL:   cfg.CheckoutURL,
		onReSignIn:    cfg.OnReSignIn,
		notifications: noti,
		frameCharSet:  i.ideConfig.windowFrameCharset(),
		textAttr:      pcfg.TextAttr,
		highlightAttr: pcfg.HighlightAttr,
		backgroundBg:  pcfg.BackgroundAttr,
	}
	i.planLockdown.setPromptFactory(func() tui.Handler {
		return newPlanLockdownPrompt(deps)
	})
	i.planMonitor = ideplan.NewMonitor(ideplan.MonitorConfig{
		Source:        cfg.Source,
		Notifications: noti,
		Locker:        i.planLockdown,
	})
	i.planMonitor.Start(context.Background())
}

// TickPlan forces an immediate plan-gating re-evaluation off the
// daily monitor cadence, reading the currently cached token rather
// than rotating it via refresh_token. Used by the lockdown overlay's
// Re-signin flow to surface a freshly-paid subscription (acquired
// through a from-scratch browser login) without waiting for the next
// scheduled tick.
func (i *IDE) TickPlan(ctx context.Context) {
	i.planMonitor.Reevaluate(ctx)
}

// Notifications returns an cross-workspace, goroutine-safe implementation
// of browserapi.Notifications.
func (i *IDE) Notifications() browserapi.Notifications {
	return i.workspaceHandler.notifications.current()
}

// Prompt opens a yes/no floating prompt in the currently focused workspace.
// It returns the prompt window that can be closed by the caller.
func (i *IDE) Prompt(
	message string,
	options []string,
	bindings []term.KeyComb,
	h handler.PromptHandler,
) browser.Window {
	return i.workspaceHandler.focusEx().comp.Prompt(
		message, options, bindings, h,
	)
}

// Close satisfies io.Closer by closing this all ide's resources, including
// the terminal state.
func (i *IDE) Close() error {
	return i.closeResources()
}

func (i *IDE) closeResources() (ret error) {
	i.workspaceHandler.mu.Lock()
	defer i.workspaceHandler.mu.Unlock()

	if i.planMonitor != nil {
		i.planMonitor.Stop()
	}
	i.planLockdown.SetLocked(false)

	if err := i.workspaceHandler.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	if err := i.workspaceManager.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	if err := i.root.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	if err := i.tutorial.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	return
}

func (i *IDE) init(
	cwd, cfgfilename, dataDir string, storage storageapi.Service, opts ...Option,
) error {
	op := newOptions(opts...)
	i.options = op

	defaultCfg := newDefaultConfigSource(op)
	var configErr error
	i.ideConfig, configErr = loadIDEConfig(cfgfilename, op)

	i.storage = storage
	i.ideConfig.storage = storageapi.WithPartition(i.storage, "ide")

	var logger *slog.Logger
	if logPath := i.ideConfig.logOutputPath(); logPath != "" {
		f, err := workspace.OpenFile(logPath,
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			expanded, expandErr := workspaceapi.CurrentUserHostURI(logPath)
			if expandErr != nil {
				return multierr.Append(
					fmt.Errorf("open log file %q: %w", logPath, err),
					fmt.Errorf("make uri %q: %w", logPath, expandErr),
				)
			}
			logDir := path.Dir(expanded.Path())
			dirErr := os.MkdirAll(logDir, 0755)
			if dirErr != nil {
				return multierr.Append(
					fmt.Errorf("open log file %q: %w", logPath, err),
					fmt.Errorf("make dir %q: %w", logDir, dirErr),
				)
			}
			f, err = workspace.OpenFile(logPath,
				os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
			if err != nil {
				return fmt.Errorf("open log file %q: %w", logPath, err)
			}
		}

		level, slogLevel := i.ideConfig.logLevel()

		log.SetOutput(f)
		log.SetLevel(level)
		log.SetFormatter(logging.LogrusLogdFormatter{})
		logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{
			Level: slogLevel,
		}))

	} else {
		log.SetOutput(io.Discard)
		log.SetLevel(log.PanicLevel)
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	}
	slog.SetDefault(logger)

	log.Tracef("logging configured and ready")
	slog.Debug("structured logging configured and ready")

	i.publishEventFn = op.publishEvent
	i.locker = op.locker

	// register default schemes
	workspaceManager := workspace.NewManagerWithWorkspaceFunc(
		i.ideConfig.workspace(), op.scheduleFn, workspace.NewSchemeWorkspace)
	err := workspaceManager.RegisterScheme(
		workspacessh.Scheme,
		workspacessh.New(newWorkspaceWindowManagerUI(i)),
	)
	if err != nil {
		return fmt.Errorf("register ssh scheme: %w", err)
	}
	err = workspaceManager.RegisterScheme(workspace.FileScheme,
		fileSchemeFunc(op.zdotDir))
	if err != nil {
		return fmt.Errorf("register file scheme: %w", err)
	}
	err = workspaceManager.RegisterScheme(workspace.MemoryScheme, workspace.NewMemoryScheme)
	if err != nil {
		return fmt.Errorf("register memory scheme: %w", err)
	}

	// register user-provided schemes via ide.WithScheme
	for scheme, fn := range op.schemes {
		if err := workspaceManager.RegisterScheme(scheme, fn); err != nil {
			return fmt.Errorf("register %q scheme: %w", scheme, err)
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home dir: %v", err)
	}

	homePath, err := workspaceapi.ExpandPath(i.ideConfig.workspaceHome(),
		func() (*user.User, error) {
			return &user.User{HomeDir: homeDir}, nil
		}, os.Getwd)
	if err != nil {
		return fmt.Errorf("expand home workspace path: %v", err)
	}

	homeDirURI, err := workspaceapi.CurrentUserHostURI(homePath)
	if err != nil {
		return fmt.Errorf("make home dir uri: %v", err)
	}

	var cwdURI *workspaceapi.URI
	if cwd != "" {
		uri, parseErr := workspaceapi.ParseURI(cwd)
		if parseErr != nil {
			parseErr = fmt.Errorf("could not parse workspace uri: %w", parseErr)
			var pathErr error
			uri, pathErr = workspaceapi.CurrentUserHostURI(cwd)
			if pathErr != nil {
				return multierr.Append(
					parseErr, fmt.Errorf("make current host URI: %w", pathErr))
			}
		}
		cwdURI = &uri
	}

	i.workspaceHandler = new(workspaceManagerHandler)
	// The observer registry breaks the construction cycle between
	// the workspace handler (whose ex instances dispatch commands)
	// and the tutorial runner (which observes those dispatches).
	// The registry is created first and threaded into newEx
	// through workspaceManagerHandler.init; the tutorial runner
	// subscribes after it has been built below.
	commandObserver := newCommandObserverRegistry()
	i.workspaceHandler.packageConfigMergeHook = op.packageConfigMergeHook
	i.workspaceHandler.tutorialsInstalled = i.onTutorialsInstalled
	err = i.workspaceHandler.init(cwdURI, homeDirURI, workspaceManager,
		i.ideConfig.notificationsConfig(), i.ideConfig, i.storage, dataDir,
		i.publishEvent,
		op.extensionRunner, i.locker, op.extensions, func() (ideConfig, error) {
			cfg, err := reloadConfig(cfgfilename,
				op.defaultWallpaper, defaultCfg, op.bell, op.scheduleFn,
				op.zdotDir)
			cfg.storage = i.ideConfig.storage
			return cfg, err
		}, op.workspaceConfig, op.tabBarOffset,
		op.tabBarHeight, op.workspacesIcon, op.workspacesBarHeight,
		op.workspacesBarOffset, op.workspacesBarFrame, op.tabsClickCallback,
		op.releaseManager, &i.root, i.ideConfig.initialTerminalCapacity(),
		op.dispatchOnPreview, op.debugCommands, op.streamingOpen,
		commandObserver)
	if err != nil {
		return fmt.Errorf("new workspace manager: %w", err)
	}
	i.workspaceManager = workspaceManager
	// Build tutorials before logNonFatalErrs so any per-tutorial
	// parse errors recorded into ideConfig.errors are surfaced
	// alongside other config decode errors.
	tutorials := buildTutorials(i)
	i.workspaceHandler.logNonFatalErrs(i.workspaceHandler.focusBrowser(),
		configErr, i.ideConfig.errors)

	shutdownShaderCfg := nopShutdownShaderConfig()
	if op.shutdownShaderFn != nil {
		shutdownShaderCfg = shutdownShaderConfig{
			shader:   op.shutdownShaderFn,
			fps:      op.shutdownShaderFPS,
			duration: op.shutdownShaderDuration,
		}
	}
	loadingShaderCfg := loadingShaderConfig{
		shader:   op.loadingShaderFn,
		fps:      op.loadingShaderFPS,
		duration: op.loadingShaderDuration,
	}
	openShaderCfg := openShaderConfig{
		shader:   op.openShaderFn,
		fps:      op.openShaderFPS,
		duration: op.openShaderDuration,
	}
	// Suppress the loading/open workspace animations when the
	// rune.star config explicitly disables them. Defaults are
	// enabled. The open shader has no effect without a loading
	// shader, so disabling loading effectively disables both.
	if !i.ideConfig.animationsLoadingWorkspace() {
		loadingShaderCfg.shader = nil
	}
	if !i.ideConfig.animationsOpenWorkspace() {
		openShaderCfg.shader = nil
	}
	// Apply rune.star overrides for the open-workspace shader, if
	// any. The shader override uses the showroom defaults exposed by
	// :shaderrun; duration replaces the runtime-provided lifetime.
	if name, ok := i.ideConfig.animationsOpenWorkspaceShader(); ok {
		fps := openShaderCfg.fps
		fc := i.ideConfig.windowFrameCharset()
		dur := openShaderCfg.duration
		openShaderCfg.shader = func(defAttr term.Attributes) shader.Shader {
			s, _ := buildNamedShader(name, defAttr, fps, dur, fc)
			return s
		}
	}
	if dur, ok := i.ideConfig.animationsOpenWorkspaceDuration(); ok {
		openShaderCfg.duration = dur
	}
	i.tutorial.init(i.workspaceHandler, tutorials,
		i.workspaceHandler.events.globalInterrupter())
	commandObserver.subscribe(&i.tutorial)
	_ = i.workspaceHandler.SubscribeEvents(
		textapi.AllEvents(),
		text.FuncEventHandler(func(_ context.Context, ev textapi.Event) bool {
			i.tutorial.observeEvent(ev.Type.String(), ev.URI.String())
			return false
		}),
	)
	i.root.init(&i.tutorial, i, i.ideConfig.defaultAttr(), shutdownShaderCfg,
		loadingShaderCfg, openShaderCfg, i.ideConfig.windowFrameCharset())
	i.planLockdown = newPlanLockdownRunner(&i.root, nil)
	i.planLockdown.setDefaultAttr(i.DefaultAttributes)
	i.initPlanSource(i.options.planSource)
	err = i.workspaceHandler.subscribeCommand(runShaderCmdManual, &i.root)
	if err != nil {
		return err
	}
	return i.workspaceHandler.subscribeCommand(tutorialCmdManual, &i.tutorial)
}

func newOptions(opts ...Option) options {
	op := defaultOptions()
	for _, o := range opts {
		o(&op)
	}
	return op
}

func newDefaultConfigSource(op options) defaultConfigSource {
	return defaultConfigSource{
		src:   op.defaultConfig,
		modal: op.defaultConfigModeModal,
		tui:   op.defaultConfigTUI,
	}
}

func loadIDEConfig(cfgfilename string, op options) (ideConfig, error) {
	var cfg ideConfig
	err := loadConfig(&cfg, cfgfilename,
		op.defaultWallpaper, newDefaultConfigSource(op), op.bell,
		op.scheduleFn, op.zdotDir)
	return cfg, err
}

func (i *IDE) initRunning() {
	if i.options.initShaderFn != nil {
		// protect access to root, simulating a std loop iteration
		i.locker.Lock()
		defer i.locker.Unlock()
		i.root.runShader(i.options.initShaderFn(i.root.defAttr, i.ideConfig.windowFrameCharset()),
			i.options.initShaderFPS, i.options.initShaderDuration)
	}
}

func fileSchemeFunc(zdotDir string) schemeapi.SchemeFunc {
	return func(
		ctx context.Context, cfg config.Config, uri workspaceapi.URI,
	) (schemeapi.Scheme, error) {
		if zdotDir != "" {
			cfg = configWithZdotDir(cfg, zdotDir)
		}
		return workspace.NewFileScheme(ctx, cfg, uri)
	}
}

func configWithZdotDir(base config.Config, zdotDir string) config.Config {
	if base != nil {
		if existing, err := base.GetString("zdotdir"); err == nil && existing != "" {
			return base
		}
	}
	merged := map[string]any{"zdotdir": zdotDir}
	if base != nil {
		base.Iterate(func(k string, v any) {
			if _, ok := merged[k]; !ok {
				merged[k] = v
			}
		})
	}
	return config.MapConfig(merged)
}
