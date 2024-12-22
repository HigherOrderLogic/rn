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
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/config"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/term"
)

// Option is a configuration option for an IDE.
type Option func(*options)

// Extension represents a built-in extension executable.
type Extension struct {
	// ID should be a unique representation of the logical
	// extension.
	ID string

	// Path is the path to the executable.
	Path string

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

// WithPublishEvent sets the EventPublisher of the IDE.
// The default is tui.PublishEvent.
func WithPublishEvent(p EventPublisher) Option {
	return func(opts *options) {
		opts.publishEvent = p
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

// WithDefaultConfigYAML sets the default baseline config.
func WithDefaultConfigYAML(configYaml string) Option {
	return func(opts *options) {
		opts.defaultConfig = configYaml
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

// WithInitShader configures the IDE to initialize with the
// given Shader animation.
func WithInitShader(
	shaderFn func(defaultAttr term.Attributes) shader.Shader,
	fps int, duration time.Duration,
) Option {
	return func(opts *options) {
		opts.initShaderFn = shaderFn
		opts.initShaderFPS = fps
		opts.initShaderDuration = duration
	}
}

// WithShutdownShader configures the IDE to close with the
// given Shader animation upon "quit" or "writeQuit".
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

type options struct {
	publishEvent        EventPublisher
	extensionRunner     ExtensionsRunner
	tabBarOffset        int
	tabsClickCallback   func(int) bool
	tabBarHeight        int
	workspacesBarFrame  bool
	workspacesBarHeight int
	workspacesIcon      rune
	workspacesBarOffset int
	locker              sync.Locker
	extensions          map[string]Extension
	workspaceConfig     string
	defaultWallpaper    browser.Wallpaper
	defaultConfig       string
	bell                func()
	scheduleFn          func(func()) bool

	initShaderFn           func(term.Attributes) shader.Shader
	initShaderDuration     time.Duration
	initShaderFPS          int
	shutdownShaderFn       func(term.Attributes) shader.Shader
	shutdownShaderDuration time.Duration
	shutdownShaderFPS      int
}

func defaultOptions() options {
	return options{
		publishEvent:       tui.PublishEvent,
		extensionRunner:    nopExtensions{},
		locker:             nopLocker{},
		extensions:         make(map[string]Extension),
		workspaceConfig:    ".iderc",
		defaultWallpaper:   browser.NopWallpaper(),
		defaultConfig:      "{}",
		bell:               term.RingBell,
		scheduleFn:         term.ScheduleNextTick,
		workspacesBarFrame: true,
		workspacesIcon:     '1',
	}
}

type nopExtensions struct {
}

func (n nopExtensions) WorkspaceExtensionsRunner(locker sync.Locker,
	uri workspaceapi.URI,
	res map[extension.Permission]extension.ResourceRegistrar,
	dataDir string, notifications browser.Notifications) (extension.Runner, error) {
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

type nopLocker struct {
}

func (l nopLocker) Lock() {
}

func (l nopLocker) Unlock() {
}
