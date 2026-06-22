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
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/blue/release/docrelease"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	yaml "gopkg.in/yaml.v3"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/ide/idepkg"
	"unstable.build/go-tui/ide/ideplan"
	"unstable.build/go-tui/text"
)

// Option is a configuration option for an IDE.
type Option func(*options)

// Extension represents a built-in extension executable.
type Extension struct {
	// ID should be a unique representation of the logical
	// extension.
	ID string

	// CmdAndArgs is the path to the executable.
	CmdAndArgs string

	// Config is the configuration for the extension.
	Config config.Config
}

// WithLocker returns an option that sets locker
// as the event loop locker to synchronize access
// to resources against extension goroutines.
//
// The default is nop locker, so no synchronization.
func WithLocker(locker sync.Locker) Option {
	return func(opts *options) {
		opts.locker = locker
	}
}

// WithDispatchOnPreview determines a list of commands
// that should be dispatched as user is scrolling down the list of completions.
func WithDispatchOnPreview(cmd string, fn previewFunc) Option {
	return func(opts *options) {
		if opts.dispatchOnPreview == nil {
			opts.dispatchOnPreview = make(map[string]previewFunc)
		}
		opts.dispatchOnPreview[cmd] = fn
	}
}

// WithPublishEvent sets the EventPublisher of the IDE.
// The default is tui.PublishEvent.
func WithPublishEvent(p EventPublisher) Option {
	return func(opts *options) {
		opts.publishEvent = p
	}
}

// WithReleaseManager sets the release.Manager of the IDE.
func WithReleaseManager(m release.Manager) Option {
	return func(opts *options) {
		opts.releaseManager = m
	}
}

// WithPackageConfigMergeHook installs a hook invoked after a package config
// merge is written to the user config file. The hook inspects the applied
// diff and may live-apply changes to the running process, reporting back via
// idepkg.ConfigMergeResult. It is wired into the package manager.
func WithPackageConfigMergeHook(
	hook func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error),
) Option {
	return func(opts *options) {
		opts.packageConfigMergeHook = hook
	}
}

// WithPlanSource installs an ideplan.Source on the IDE: the lockdown
// overlay reacts to its decisions and the daily monitor uses it as
// its evaluation source.
func WithPlanSource(cfg PlanSourceConfig) Option {
	return func(opts *options) {
		opts.planSource = cfg
	}
}

// WithExtensionsRunner sets the Extensions facility of this IDE.
// The default is no extension runner.
func WithExtensionsRunner(p ExtensionsRunner) Option {
	return func(opts *options) {
		opts.extensionRunner = p
	}
}

// WithExtension adds Extension to the IDE's built-in extensions.
func WithExtension(p Extension) Option {
	return func(opts *options) {
		if _, ok := opts.extensions[p.ID]; ok {
			panic("built-in extension with same ID already registered")
		}
		opts.extensions[p.ID] = p
	}
}

// WithScheme registers a custom workspace scheme on the IDE's
// workspace.Manager. The given fn is invoked to construct the scheme
// when a workspace with a URI of the given scheme is opened.
//
// Registering a scheme name that is already in use causes IDE
// initialization to fail with an error.
func WithScheme(scheme string, fn schemeapi.SchemeFunc) Option {
	return func(opts *options) {
		if opts.schemes == nil {
			opts.schemes = make(map[string]schemeapi.SchemeFunc)
		}
		opts.schemes[scheme] = fn
	}
}

// WithConfigFilename defines the base filename of the IDE configuration
// to be expected in a workspace's directory.
func WithConfigFilename(filename string) Option {
	return func(opts *options) {
		opts.workspaceConfig = filename
	}
}

// WithDefaultWallpaper sets the default wallpaper if the
// user doesn't provide one via rc configuration.
func WithDefaultWallpaper(wallpaper browser.Wallpaper) Option {
	return func(opts *options) {
		opts.defaultWallpaper = wallpaper
	}
}

// WithDefaultConfigYAML sets the default baseline config from one or more
// YAML documents. Subsequent documents override the earlier ones via the
// usual deep-merge semantics.
func WithDefaultConfigYAML(base string, overrides ...string) Option {
	return func(opts *options) {
		if len(overrides) == 0 {
			opts.defaultConfig = base
			return
		}
		// NOTE: once we add the ability to choose between modal or modeless config,
		// this should be a compile-time operation
		ret := make(map[string]any)
		for _, cfg := range append([]string{base}, overrides...) {
			d := yaml.NewDecoder(strings.NewReader(cfg))
			mcfg := make(map[string]any)
			err := d.Decode(mcfg)
			if err != nil {
				panic(fmt.Sprintf("error decoding default config: %v", err))
			}
			overrideConfig(ret, mcfg)
		}
		var buf strings.Builder
		d := yaml.NewEncoder(&buf)
		err := d.Encode(ret)
		if err != nil {
			panic("error decoding default config")
		}
		opts.defaultConfig = buf.String()
	}
}

// WithDefaultConfigStarlark sets the default baseline config as a Starlark
// source. The script must bind a top-level `config` dict. The loader exposes
// two predeclared globals to the script:
//   - mode: "modal" when modal is true, otherwise "modeless"
//   - tui: the given tui boolean
func WithDefaultConfigStarlark(src string, modal bool, tui bool) Option {
	return func(opts *options) {
		opts.defaultConfig = src
		opts.defaultConfigModeModal = modal
		opts.defaultConfigTUI = tui
	}
}

// WithBell sets the default mechanism to ring the system bell.
func WithBell(bell func()) Option {
	return func(opts *options) {
		opts.bell = bell
	}
}

// WithScheduleNextTick sets the default mechanism to schedule a user functio to run before
// the next event loop tick.
func WithScheduleNextTick(scheduleFn func(func()) bool) Option {
	return func(opts *options) {
		opts.scheduleFn = scheduleFn
	}
}

// WithZdotDir sets the starting zsh directory configuration file via env ZDOTDIR
// when zsh is used as the default shell, or is passed via config (terminal.shell)
// as "zsh", rather than with the proper flags (-i, --login, etc.).
func WithZdotDir(dir string) Option {
	return func(opts *options) {
		opts.zdotDir = dir
	}
}

// WithInitShader configures the IDE to initialize with the
// given Shader animation.
func WithInitShader(
	shaderFn func(term.Attributes, component.FrameCharSet) shader.Shader,
	fps int, duration time.Duration,
) Option {
	return func(opts *options) {
		opts.initShaderFn = shaderFn
		opts.initShaderFPS = fps
		opts.initShaderDuration = duration
	}
}

// WithShutdownShader configures the IDE to close with the
// given Shader animation upon "quit" or "writequit".
func WithShutdownShader(
	shaderFn func(term.Attributes) shader.Shader,
	fps int, duration time.Duration,
) Option {
	return func(opts *options) {
		opts.shutdownShaderFn = shaderFn
		opts.shutdownShaderFPS = fps
		opts.shutdownShaderDuration = duration
	}
}

// WithLoadingShader configures the IDE to play the given Shader
// animation while a workspace is being loaded by addWorkspace.
//
// duration is the lifetime of the underlying [shader.Component]; it is
// decoupled from the visual length of the effect itself (e.g.
// [shader.GrayFadeParams.FadeFrames]) so callers can pick a "fade then
// hold" budget that outlives the animation. If the load completes
// before duration elapses, the loading shader is replaced by the
// shader configured via [WithOpenShader] (if any). If the load takes
// longer, the shader self-finishes and the UI is drawn normally while
// loading continues in the background. Passing duration <= 0 uses an
// internal default.
func WithLoadingShader(
	shaderFn func(term.Attributes) shader.Shader,
	fps int,
	duration time.Duration,
) Option {
	return func(opts *options) {
		opts.loadingShaderFn = shaderFn
		opts.loadingShaderFPS = fps
		opts.loadingShaderDuration = duration
	}
}

// WithOpenShader configures the IDE to play the given Shader
// animation when a workspace finishes loading. Has no effect if no
// loading shader is configured via [WithLoadingShader].
//
// duration is the lifetime of the underlying [shader.Component]; the
// open shader runs for this long after the loading shader finishes.
// Passing duration <= 0 uses an internal default.
func WithOpenShader(
	shaderFn func(term.Attributes) shader.Shader,
	fps int,
	duration time.Duration,
) Option {
	return func(opts *options) {
		opts.openShaderFn = shaderFn
		opts.openShaderFPS = fps
		opts.openShaderDuration = duration
	}
}

// WithTabBarOffset configures the IDE to render
// with a tab bar x offset in cells to accomodate
// perhaps another UI element.
func WithTabBarOffset(offset int) Option {
	return func(opts *options) {
		opts.tabBarOffset = offset
	}
}

// WithTabBarHeight defines the height of the tab bar.
func WithTabBarHeight(height int) Option {
	return func(opts *options) {
		opts.tabBarHeight = height
	}
}

// WithWorkspacesBarFrame defines whether the IDE renders
// the bottom workspaces bar with frame or not.
func WithWorkspacesBarFrame(frame bool) Option {
	return func(opts *options) {
		opts.workspacesBarFrame = frame
	}
}

// WithWorkspacesBarHeight defines the height of the
// workspaces bar.
func WithWorkspacesBarHeight(height int) Option {
	return func(opts *options) {
		opts.workspacesBarHeight = height
	}
}

// WithWorkspacesBarOffset defines the x offset in cells
// of the rendered workspaces bar.
func WithWorkspacesBarOffset(xoffset int) Option {
	return func(opts *options) {
		opts.workspacesBarOffset = xoffset
	}
}

// WithWorkspacesIcon defines the icon to use as the first workspace.
// Subsequent workspaces use subsequent icons.
func WithWorkspacesIcon(icon rune) Option {
	return func(opts *options) {
		opts.workspacesIcon = icon
	}
}

// WithTabsClickCallback sets a callback to be called every time the top tabs bar
// is clicked.
func WithTabsClickCallback(fn func(int) bool) Option {
	return func(opts *options) {
		opts.tabsClickCallback = fn
	}
}

// WithDebugCommands toggles registration of debug-only ex commands.
// When enabled, the IDE registers commands that intentionally crash
// the process (`panic`, `crash`) for use during development. The
// default is false; production builds must leave this off.
func WithDebugCommands(enabled bool) Option {
	return func(opts *options) {
		opts.debugCommands = enabled
	}
}

// WithStreamingOpen enables opening files asynchronously.
func WithStreamingOpen(enabled bool) Option {
	return func(opts *options) {
		opts.streamingOpen = enabled
	}
}

// WithStarlarkTutorial registers an embedded Starlark tutorial under
// the given name. The src is parsed at IDE startup and the tutorial
// becomes invokable via `:tutorial start <name>`. If the same name is also
// present in the user's config, the config entry wins.
func WithStarlarkTutorial(name, src string) Option {
	return func(opts *options) {
		if opts.starlarkTutorials == nil {
			opts.starlarkTutorials = make(map[string]string)
		}
		opts.starlarkTutorials[name] = src
	}
}

// WithStartingTutorial schedules the named tutorial to run
// automatically once the IDE becomes ready. The name must match a
// tutorial registered via WithStarlarkTutorial or the user config;
// if it is empty or unknown, no tutorial is started. Intended for
// first-run onboarding after bootstrap.
func WithStartingTutorial(name string) Option {
	return func(opts *options) {
		opts.startingTutorial = name
	}
}

type options struct {
	publishEvent        EventPublisher
	extensionRunner     ExtensionsRunner
	releaseManager      release.Manager
	planSource          PlanSourceConfig
	tabBarOffset        int
	tabsClickCallback   func(int) bool
	tabBarHeight        int
	workspacesBarFrame  bool
	workspacesBarHeight int
	workspacesIcon      rune
	workspacesBarOffset int
	locker              sync.Locker
	dispatchOnPreview   map[string]previewFunc
	extensions          map[string]Extension
	schemes             map[string]schemeapi.SchemeFunc
	workspaceConfig     string
	defaultWallpaper    browser.Wallpaper
	defaultConfig       string
	bell                func()
	scheduleFn          func(func()) bool
	afterFunc           func(time.Duration, func()) *time.Timer
	debugCommands       bool
	streamingOpen       bool

	defaultConfigModeModal bool
	defaultConfigTUI       bool

	initShaderFn           func(term.Attributes, component.FrameCharSet) shader.Shader
	initShaderDuration     time.Duration
	initShaderFPS          int
	shutdownShaderFn       func(term.Attributes) shader.Shader
	shutdownShaderDuration time.Duration
	shutdownShaderFPS      int
	loadingShaderFn        func(term.Attributes) shader.Shader
	loadingShaderFPS       int
	loadingShaderDuration  time.Duration
	openShaderFn           func(term.Attributes) shader.Shader
	openShaderFPS          int
	openShaderDuration     time.Duration
	zdotDir                string
	starlarkTutorials      map[string]string
	startingTutorial       string

	packageConfigMergeHook func(idepkg.ConfigMergeEvent) (idepkg.ConfigMergeResult, error)
}

func defaultOptions() options {
	return options{
		publishEvent:       tui.PublishEvent,
		extensionRunner:    nopExtensions{},
		locker:             new(sync.Mutex),
		extensions:         make(map[string]Extension),
		workspaceConfig:    ".iderc",
		defaultWallpaper:   browser.NopWallpaper(),
		defaultConfig:      "config = {}",
		bell:               term.RingBell,
		scheduleFn:         term.ScheduleNextTick,
		afterFunc:          time.AfterFunc,
		workspacesBarFrame: true,
		workspacesIcon:     '1',
		releaseManager:     docrelease.NewManager(document.NewInMemoryService()),
		planSource:         PlanSourceConfig{Source: ideplan.NopSource{}},
	}
}

type nopExtensions struct {
}

func (n nopExtensions) WorkspaceExtensionsRunner(
	uri workspaceapi.URI,
	res map[extensionapi.Permission]extension.ResourceRegistrar,
	authorizer *ideauthorizer.Authorizer,
	dataDir string, notifications browser.Notifications,
	exec, extExec schemeapi.Executor,
	grantor extension.Grantor,
	editor text.Editor,
	promptOpener ideauthorizer.PromptOpener, storage storageapi.Service,
	scheduleNextTick func(func()) bool) (extension.Runner, error) {
	return nopExtensionsRunner{}, nil
}

type nopExtensionsRunner struct {
}

func (n nopExtensionsRunner) Run(extensionID, path string, config config.Config) error {
	log.Warnf("attempting to run extension %q but no "+
		"extensions facility has been configured", extensionID)
	return nil
}

func (n nopExtensionsRunner) Close() error {
	return nil
}

func (n nopExtensionsRunner) WaitReady(ctx context.Context, id string) error {
	return nil
}
