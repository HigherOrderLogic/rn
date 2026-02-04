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
	"os"
	"path"
	"sync"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
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

// Storage returns persistent storage acrosss IDE instances, given
// the same data dir passed in ide.New, or ide.NewRecovery.
func (i *IDE) Storage() document.Service {
	return i.workspaceHandler.storage
}

// Size returns the current width and height in cells.
func (i *IDE) Size() (width, height int) {
	return i.root.width, i.root.height
}

// Open opens the given file, in the currently active workspace.
func (i *IDE) Open(file workspaceapi.URI) error {
	return i.workspaceHandler.openURI(file, true /* focus */)
}

// SetReleaseManager sets the release.Manager of the IDE.
// This should be called before Run or Handler are called for the first time.
func (i *IDE) SetReleaseManager(m release.Manager) {
	i.workspaceHandler.setReleaseManager(m)
}

// Notifications returns an cross-workspace, goroutine-safe implementation
// of browserapi.Notifications.
func (i *IDE) Notifications() browserapi.Notifications {
	return i.workspaceHandler.notifications
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

	configErr := loadConfig(&i.ideConfig, cfgfilename,
		op.defaultWallpaper, op.defaultConfig, op.bell,
		op.scheduleFn, op.zdotDir)

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

		level := i.ideConfig.logLevel()

		log.SetOutput(f)
		log.SetLevel(level)
		log.SetFormatter(logging.LogrusLogdFormatter{})
	} else {
		log.SetOutput(io.Discard)
		log.SetLevel(log.PanicLevel)
	}

	log.Tracef("logging configured and ready")

	i.publishEventFn = op.publishEvent
	i.locker = op.locker

	// register default schemes
	workspaceManager := workspace.NewManagerWithWorkspaceFunc(
		i.ideConfig.workspace(), workspace.NewSchemeWorkspace)
	err := workspaceManager.RegisterScheme(workspacessh.Scheme, workspacessh.New)
	if err != nil {
		return fmt.Errorf("register ssh scheme: %w", err)
	}
	err = workspaceManager.RegisterScheme(workspace.FileScheme, workspace.NewFileScheme)
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
	notificationsCfg := i.ideConfig.notificationsConfig()
	interrupter := term.FuncInterrupter(func(ctx context.Context) error {
		payload, _ := term.PayloadFromContext(ctx)
		if !i.publishEvent(term.Event{Type: term.EventInterrupt, Raw: payload, Context: ctx}) {
			return errEventStreamNotReady
		}
		return nil
	})
	notificationsCfg.Interrupter = interrupter
	notifications := notifications.New(i.workspaceHandler, notificationsCfg)
	err = i.workspaceHandler.init(cwdURI, homeDirURI, workspaceManager,
		notifications, i.ideConfig, dataDir, i.publishEvent,
		op.extensionRunner, i.locker, op.extensions, func() (ideConfig, error) {
			return reloadConfig(cfgfilename,
				op.defaultWallpaper, op.defaultConfig, op.bell, op.scheduleFn,
				op.zdotDir)
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
	handler := handler.WithComponent(i.workspaceHandler, notifications)
	i.root.init(handler, i, i.ideConfig.defaultAttr(), shutdownShaderCfg,
		i.ideConfig.windowFrameCharset())
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
