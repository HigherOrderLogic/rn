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
	"path"
	"sync"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/docmarshal/doctoml"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/localstorage"
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
	publishEventFn   EventPublisher
	storage          storageapi.Service
}

// EventPublisher is a function that publishes the given event back
// into the event loop.
type EventPublisher func(term.Event) bool

// New allocates storage for a new IDE and initializes it with config
// at cfgfilename and filename. Note that if filename is empty, a default inmutable
// buffer will be loaded.
func New(
	cwd, cfgfilename, dataDir string, opts ...Option,
) (i *IDE, err error) {
	i = new(IDE)
	err = i.init(cwd, cfgfilename, dataDir, opts...)
	return
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
	return &i.root
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

	if err := i.workspaceHandler.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	if err := i.workspaceManager.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	if err := i.root.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}

	return
}

func (i *IDE) init(
	cwd, cfgfilename, dataDir string, opts ...Option,
) error {
	op := defaultOptions()
	for _, o := range opts {
		o(&op)
	}
	i.options = op

	defaultCfg := defaultConfigSource{
		src:   op.defaultConfig,
		modal: op.defaultConfigModeModal,
		tui:   op.defaultConfigTUI,
	}
	configErr := loadConfig(&i.ideConfig, cfgfilename,
		op.defaultWallpaper, defaultCfg, op.bell,
		op.scheduleFn, op.zdotDir)

	// Storage is built here (not in workspaceManagerHandler.init) so
	// ideConfig — which is consulted to build alias completer chains
	// before any workspace is created — can resolve `{history}`
	// placeholders against the persisted command history doc. The
	// command Prompt writes history under the "ide" partition, so
	// alias chains must read from the same partition.
	i.storage = localstorage.New(context.Background(), dataDir, doctoml.Marshaler())
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
		i.ideConfig.workspace(), workspace.NewSchemeWorkspace)
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

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home dir: %v", err)
	}

	homeDirURI, err := workspaceapi.CurrentUserHostURI(homeDir)
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
		op.releaseManager, &i.root, i.ideConfig.initialTerminalCapacity(), op.dispatchOnPreview)
	if err != nil {
		return fmt.Errorf("new workspace manager: %w", err)
	}
	i.workspaceManager = workspaceManager
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
	i.root.init(i.workspaceHandler, i, i.ideConfig.defaultAttr(), shutdownShaderCfg,
		loadingShaderCfg, openShaderCfg, i.ideConfig.windowFrameCharset())
	return i.workspaceHandler.subscribeCommand(runShaderCmdManual, &i.root)
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

// fileSchemeFunc returns a schemeapi.SchemeFunc that wraps
// workspace.NewFileScheme to inject the IDE's --rune-zdotdir flag
// into the per-scheme config under "zdotdir". The fileScheme reads
// it back to set ZDOTDIR when starting an empty-Path login shell
// (see workspace.fileScheme.StartCommand). zdotdir applies only to
// the local file scheme — for SSH workspaces the *remote* rune
// process resolves its own zdotdir from its own config, so the IDE
// host's flag never leaks across the wire.
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

// configWithZdotDir returns a config.Config that overlays "zdotdir"
// onto base. If the user's workspace.file config already sets
// "zdotdir" we let it win — explicit configuration beats the
// command-line default.
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
