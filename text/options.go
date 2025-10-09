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

package text

import (
	"fmt"
	"time"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/term"
)

// CommandOverlayConfig holds configuration for the
// command's interface.
type CommandOverlayConfig struct {
	MatchedTextAttr  term.Attributes
	FocusElementAttr term.Attributes
	ElementAttr      term.Attributes
	ManualAttr       term.Attributes
	ShowManualAfter  time.Duration
}

// Config holds configuration for an editor.Component.
type Config struct {
	Tabspaces               int
	Filepaths               []workspaceapi.URI
	RecoveryFilepath        workspaceapi.URI
	CommandEvent            term.KeyComb
	CommandMaxHistory       int
	CommandKeyBindings      map[term.KeyComb][][]string
	CommandSequenceBindings map[handler.Sequence][][]string
	CommandAliases          map[string]CommandAlias
	SequencerTimeout        time.Duration
	DirtyTabAttr            term.Attributes
	Icons                   IconSet
	Syntax                  syntax.Config
	PkgManager              syntax.PkgManager

	EventPublisher func(term.Event) bool

	CommandOverlay CommandOverlayConfig
	browser.Config
}

// IconSet is used to render icons next to file names in tabs.
type IconSet struct {
	Extensions map[string]rune
	Default    rune
	Terminal   rune
}

// CommandAlias is a command to command alias, along with completion configuration.
type CommandAlias struct {
	Name      string
	Commands  []string
	Completer func(*Component) command.Completer
}

// DefaultCommandOverlayConfig returns the default Config's CommandOverlayConfig.
func DefaultCommandOverlayConfig() (cfg CommandOverlayConfig) {
	// NOTE: cannot use handler/command config: dependency cycle
	cfg.MatchedTextAttr = term.Attributes{Fg: tcell.ColorRed}
	cfg.FocusElementAttr = term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrBold | tcell.AttrUnderline}
	cfg.ElementAttr = term.Attributes{}
	cfg.ManualAttr = term.Attributes{}
	cfg.ShowManualAfter = 1 * time.Second
	return
}

// DefaultConfig returns the default Config.
func DefaultConfig() Config {
	cfg := Config{
		Tabspaces:               4,
		Filepaths:               nil,
		RecoveryFilepath:        workspaceapi.URI{},
		CommandEvent:            term.KeyComb{Ch: ':'},
		CommandMaxHistory:       2000,
		Config:                  browser.DefaultConfig(),
		DirtyTabAttr:            term.Attributes{Attrs: tcell.AttrBold},
		CommandKeyBindings:      make(map[term.KeyComb][][]string),
		CommandSequenceBindings: make(map[handler.Sequence][][]string),
		CommandAliases:          make(map[string]CommandAlias),
		SequencerTimeout:        400 * time.Millisecond,
		CommandOverlay:          DefaultCommandOverlayConfig(),
		EventPublisher:          func(term.Event) bool { return false },
		Icons: IconSet{
			Extensions: map[string]rune{},
			Default:    'o',
			Terminal:   '$',
		},
	}
	return cfg
}

// Option represents a configuration option for Component.
type Option func(*Config)

// WithTabspaces sets the number of spaces used to render a tab.
func WithTabspaces(tabspaces int) Option {
	return func(cfg *Config) {
		cfg.Tabspaces = tabspaces
	}
}

// WithRecoveryFile indicates that an Editor is to be initialized
// from recovery file swapFilePath. This option overrides WithSwapDir because
// the swap directory of swapFilePath is used instead.
func WithRecoveryFile(swapFilePath workspaceapi.URI) Option {
	return func(cfg *Config) {
		cfg.RecoveryFilepath = swapFilePath
	}
}

// WithFile returns an Option that sets the filepath of the file to open with
// a Editor handler.
func WithFile(file workspaceapi.URI) Option {
	return func(cfg *Config) {
		cfg.Filepaths = append(cfg.Filepaths, file)
	}
}

// WithSyntaxConfig returns an Option that sets syntax configuration.
func WithSyntaxConfig(syntax syntax.Config) Option {
	return func(cfg *Config) {
		cfg.Syntax = syntax
	}
}

// WithPackageManager returns an Option that sets the package manager
// for the syntax tree parser.
func WithPackageManager(pkg syntax.PkgManager) Option {
	return func(cfg *Config) {
		cfg.PkgManager = pkg
	}
}

// WithCommandKey returns an Option that defines what key triggers the editor's
// command mode.
func WithCommandKey(event term.KeyComb) Option {
	return func(cfg *Config) {
		cfg.CommandEvent = event
	}
}

// WithTabBarOffset defines the x offset for
// rendering the tab bar.
func WithTabBarOffset(offset int) Option {
	return func(cfg *Config) {
		cfg.TabBarOffset = offset
	}
}

// WithTabBarHeight defines the height of the tab bar.
func WithTabBarHeight(height int) Option {
	return func(cfg *Config) {
		cfg.TabBarHeight = height
	}
}

// WithTabNameSeparator defines an alternate separator
// of tab names. By default it's two spaces.
func WithTabNameSeparator(sep string) Option {
	return func(cfg *Config) {
		cfg.TabNameSeparator = sep
	}
}

// WithWallpaper sets the starting buffer default text wallpaper.
func WithWallpaper(wallpaper browser.Wallpaper) Option {
	return func(cfg *Config) {
		cfg.Wallpaper = wallpaper
	}
}

// WithFrameUnion defines whether the frames should be unioned or not.
// By default is true if the configuration given to WithWindowManagerConfig
// sets Frame to true.
func WithFrameUnion(frameUnion bool) Option {
	return func(cfg *Config) {
		cfg.Config.FrameUnion = frameUnion
	}
}

// WithTabsClickCallback sets a callback to be called every time the top tabs bar
// is clicked.
func WithTabsClickCallback(fn func(int) bool) Option {
	return func(cfg *Config) {
		cfg.Config.OnTabsClick = fn
	}
}

// WithFrameUnionCharSet configures the characters used to draw the frame union
// between the browser tabs and the window manager.
func WithFrameUnionCharSet(cs component.FrameUnionCharSet) Option {
	return func(cfg *Config) {
		cfg.FrameUnionCharSet = cs
	}
}

// WithWindowManagerConfig returns an Option that defines
// the underlying's WindowManager initialization configuration.
// See handler.WindowManagerConfig for more info. If this option is not passed
// DefaultWindowManagerConfig is utilized.
func WithWindowManagerConfig(config handler.WindowManagerConfig) Option {
	return func(cfg *Config) {
		cfg.WindowManagerConfig = config
	}
}

// WithNotifications returns an Option that configures
// a Component's notifications.
func WithNotifications(n browser.Notifications) Option {
	return func(cfg *Config) {
		cfg.Notifications = n
	}
}

// WithFocusTabAttr returns an Option that configures the attributes of a
// Components's tab in focus.
func WithFocusTabAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.FocusTabAttr = attr
	}
}

// WithNonFocusTabAttr returns an Option that configures the attributes of a
// Components's tabs that are not in focus.
func WithNonFocusTabAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.NonFocusTabAttr = attr
	}
}

// WithCommandKeyBinding maps key to issue cmd.
func WithCommandKeyBinding(key term.KeyComb, cmdAndArgs [][]string) Option {
	return func(cfg *Config) {
		sum := term.KeyComb{Mod: key.Mod, Ch: key.Ch, Key: key.Key}
		cfg.CommandKeyBindings[sum] = cmdAndArgs
	}
}

// WithCommandMaxHistory sets the max command history to store for searching back through it.
func WithCommandMaxHistory(max int) Option {
	return func(cfg *Config) {
		cfg.CommandMaxHistory = max
	}
}

// WithCommandSequenceBinding configures an editor to trigger
// cmd when key sequence is pressed.
func WithCommandSequenceBinding(sequence handler.Sequence, cmdsAndArgs [][]string) Option {
	return func(cfg *Config) {
		seq := handler.Sequence{
			First: term.KeyComb{
				Mod: sequence.First.Mod,
				Ch:  sequence.First.Ch,
				Key: sequence.First.Key,
			},
			Last: term.KeyComb{
				Mod: sequence.Last.Mod,
				Ch:  sequence.Last.Ch,
				Key: sequence.Last.Key,
			},
		}
		cfg.CommandSequenceBindings[seq] = cmdsAndArgs
	}
}

// WithSequencerTimeout configures the time span during which two key events
// can be considered as a sequence.
func WithSequencerTimeout(t time.Duration) Option {
	return func(cfg *Config) {
		cfg.SequencerTimeout = t
	}
}

// WithDirtyTabAttr defines the attributes to use to indicate that
// the buffer in a tab has been modified but not flushed the changes to disk yet.
func WithDirtyTabAttr(attr term.Attributes) Option {
	return func(cfg *Config) {
		cfg.DirtyTabAttr = attr
	}
}

// WithCommandOverlayConfig defines the command overlay interface properties.
func WithCommandOverlayConfig(c CommandOverlayConfig) Option {
	return func(cfg *Config) {
		cfg.CommandOverlay = c
	}
}

// WithCommandAliases defines command aliases.
func WithCommandAliases(aliases map[string]CommandAlias) Option {
	return func(cfg *Config) {
		cfg.CommandAliases = aliases
	}
}

// WithPromptConfig sets the Components's prompt properties.
func WithPromptConfig(c browser.PromptConfig) Option {
	return func(cfg *Config) {
		cfg.Config.PromptConfig = c
	}
}

// WithIconSet sets the Components's file icons.
func WithIconSet(icons IconSet) Option {
	return func(cfg *Config) {
		cfg.Icons = icons
	}
}

// WithEventPublisher sets the Component's event publisher
func WithEventPublisher(f func(term.Event) bool) Option {
	return func(cfg *Config) {
		cfg.EventPublisher = f
	}
}

// ValidateCommandAliases validates that the given command aliases configuration
// doesn't contain any self-referencing aliases.
func ValidateCommandAliases(aliases map[string]CommandAlias) error {
	for alias := range aliases {
		if isErr := exploreAlias(aliases, alias, make(map[string]struct{})); isErr {
			return fmt.Errorf("alias cycle detected: '%s'", alias)
		}
	}
	return nil
}

func exploreAlias(
	aliases map[string]CommandAlias, exploringAlias string,
	origins map[string]struct{},
) bool {
	origins[exploringAlias] = struct{}{}
	for _, target := range aliases[exploringAlias].Commands {
		if _, seenInPath := origins[target]; seenInPath {
			return true
		}
		if _, isAlias := aliases[target]; !isAlias {
			continue
		}
		copyOrigins := make(map[string]struct{}, len(origins)+1)
		for k, v := range origins {
			copyOrigins[k] = v
		}
		if isErr := exploreAlias(aliases, target, copyOrigins); isErr {
			return true
		}
	}
	return false
}
