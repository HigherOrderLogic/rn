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
	"sync/atomic"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
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
	running          int32
	publishEventFn   EventPublisher
}

// EventPublisher is a function that publishes the given event back
// into the event loop.
type EventPublisher func(term.Event) bool

// New allocates storage for a new IDE and initializes it with config
// at cfgfilename and filename. Note that if filename is empty, a default inmutable
// buffer will be loaded.
func New(
	cwd, cfgfilename, dataDir string,
	filenames []string,
	opts ...Option,
) (i *IDE, err error) {
	i = new(IDE)
	err = i.init(cwd, cfgfilename, "", dataDir, filenames, opts...)
	return
}

// NewRecovery allocates storage for a new IDE and initializes in recovery mode.
// The underlying editor will use recfilename to try to recover file at filename.
// Note that this function panics if either filename or recfilename are empty.
func NewRecovery(
	cwd, cfgfilename, filename, recfilename, dataDir string,
	opts ...Option,
) (i *IDE, err error) {
	if filename == "" || recfilename == "" {
		panic(fmt.Sprintf("invalid input: filename='%s', recfilename='%s'",
			filename, recfilename))
	}
	i = new(IDE)
	err = i.init(cwd, cfgfilename, recfilename, dataDir,
		[]string{filename}, opts...)
	return
}

// Interrupt satisfies term.Interrupter
func (i *IDE) Interrupt(ctx context.Context) error {
	payload, _ := term.PayloadFromContext(ctx)
	if !i.publishEvent(term.Event{Type: term.EventInterrupt, Raw: payload}) {
		return errEventStreamNotReady
	}
	return nil
}

// Run initialzes the underlying terminal environment and runs
// it with a workspace handler
func (i *IDE) Run() error {
	err := tui.Init()
	if err != nil {
		return fmt.Errorf("tui init: %w", err)
	}

	term.SetAttr(i.DefaultAttributes())
	term.SetInputMode(i.ideConfig.inputMode())

	i.initRunning()
	err = tui.RunWithLocker(&i.root, i.locker)
	if err != nil {
		return fmt.Errorf("tui run: %w", err)
	}

	return nil
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
	return c.workspaceHandler.subscribeEventHandler(events, handler)
}

// DefaultAttributes return the default attributes to be used to fill the screen.
func (i *IDE) DefaultAttributes() term.Attributes {
	return i.root.defAttr
}

// SetDefaultAttributes sets the default attributes to be used to fill the screen.
func (i *IDE) SetDefaultAttributes(defAttr term.Attributes) {
	i.root.defAttr = defAttr
}

// Config returns the configuration loaded by this IDE.
func (i *IDE) Config() config.Config {
	return config.MapConfig(i.ideConfig.cfg)
}

// Handler returns the root Handler of this IDE, and
// a cleanup function when this IDE is no longer in use.
// This can be used insteaf of Run and Close, which
// install this IDE on a TUI system.
func (i *IDE) Handler() (tui.Handler, func()) {
	i.initRunning()
	return &i.root, func() {
		running := atomic.CompareAndSwapInt32(&i.running, 1, 0)
		if !running {
			return
		}
		_ = i.closeResources()
	}
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
	i.workspaceHandler.mu.Lock()
	defer i.workspaceHandler.mu.Unlock()

	ex := i.workspaceHandler.exHandler(i.workspaceHandler.focusHandler())
	_, err := ex.editFileURI(file, ex.invokeWindow(), false)
	if err != nil {
		return err
	}
	return err
}

// Close satisfies io.Closer by closing this all ide's resources, including
// the terminal state.
func (i *IDE) Close() error {
	running := atomic.CompareAndSwapInt32(&i.running, 1, 0)
	if !running {
		return nil
	}
	err := i.closeResources()
	tui.Close()
	return err
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
	cwd, cfgfilename, recfilename string, dataDir string,
	filenames []string,
	opts ...Option,
) error {
	op := defaultOptions()
	for _, o := range opts {
		o(&op)
	}
	i.options = op

	configErr := loadConfig(&i.ideConfig, cfgfilename,
		op.defaultWallpaper, op.defaultConfig, op.bell, op.scheduleFn)

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
		if !i.publishEvent(term.Event{Type: term.EventInterrupt, Raw: payload}) {
			return errEventStreamNotReady
		}
		return nil
	})
	notificationsCfg.Interrupter = interrupter
	notifications := notifications.New(i.workspaceHandler, notificationsCfg)
	err = i.workspaceHandler.init(cwdURI, homeDirURI, workspaceManager,
		notifications, i.ideConfig, recfilename, filenames,
		dataDir, i.publishEvent, op.extensionRunner, i.locker, op.extensions,
		func() (ideConfig, error) {
			return reloadConfig(cfgfilename,
				op.defaultWallpaper, op.defaultConfig, op.bell, op.scheduleFn)
		}, op.workspaceConfig, op.tabBarOffset,
		op.tabBarHeight, op.workspacesIcon, op.workspacesBarHeight,
		op.workspacesBarOffset, op.workspacesBarFrame, op.tabsClickCallback,
		op.releaseManager, &i.root)
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
	i.root.init(handler, i, i.ideConfig.defaultAttr(), shutdownShaderCfg)
	return i.workspaceHandler.subscribeCommand(runShaderCmdManual, &i.root)
}

func (i *IDE) publishEvent(ev term.Event) bool {
	// avoid termbox' screen panicking because
	// some component wants to publish interrupt
	// before we are fully initialized
	running := atomic.LoadInt32(&i.running)
	if running != 1 {
		return false
	}

	return i.publishEventFn(ev)
}

func (i *IDE) initRunning() {
	atomic.StoreInt32(&i.running, 1)
	if i.options.initShaderFn != nil {
		// protect access to root, simulating a std loop iteration
		i.locker.Lock()
		defer i.locker.Unlock()
		i.root.runShader(i.options.initShaderFn(i.root.defAttr),
			i.options.initShaderFPS, i.options.initShaderDuration)
	}
}
