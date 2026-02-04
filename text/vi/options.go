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

package vi

import (
	"github.com/sirupsen/logrus"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

// viConfig holds configuration for Vi.
type viConfig struct {
	attr               term.Attributes
	resAttr            term.Attributes
	clipboard          clipboard.Register
	tabspaces          int
	scheduleNextTick   func(func()) bool
	defaultRegister    string
	registry           text.WorkspaceCommandRegistry
	wrap               bool
	cursorCorrections  bool
	autoCenter         bool
	enableInitialFolds bool
	enableAuxBar       bool
	auxBarConfig       text.AuxBarConfig
	statusBarConfig    text.StatusBarConfig
	statusBarEnabled   bool
	gitBarConfig       text.GitBarConfig
	workspace          workspaceapi.URI
	enableGitBar       bool
	notifications      browserapi.Notifications
}

// defaultviHandlerImplConfig is a sane configuration defaults for viHandlerImpl.
func defaultviHandlerImplConfig() viConfig {
	return viConfig{
		tabspaces: component.DefaultTabspaces,
		resAttr: term.Attributes{
			Attrs: tcell.AttrReverse,
		},
		clipboard: clipboard.NewInMemory(),
		scheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
		defaultRegister:   clipboard.DefaultRegisterID,
		cursorCorrections: true,
	}
}

// Option represents a Vi handler configuration option.
type Option func(*viConfig)

// WithTabspaces sets the tabspaces value.
func WithTabspaces(tabspaces int) Option {
	return func(cfg *viConfig) {
		cfg.tabspaces = tabspaces
	}
}

// WithResAttr sets the search result cell attributes to be rendered.
func WithResAttr(attr term.Attributes) Option {
	return func(cfg *viConfig) {
		cfg.resAttr = attr
	}
}

// WithNotifications defines the notifications mechanism to use by Editor.
func WithNotifications(noti browserapi.Notifications) Option {
	return func(cfg *viConfig) {
		cfg.notifications = noti
	}
}

// WithWorkspaceCommandRegistry sets the command registry to register workspace-level
// commands.
func WithWorkspaceCommandRegistry(
	cwd workspaceapi.URI, registry text.WorkspaceCommandRegistry,
) Option {
	return func(cfg *viConfig) {
		cfg.registry = registry
		cfg.workspace = cwd
	}
}

// WithScheduleNextTick defines the function to schedule and serializes asynchronous work.
func WithScheduleNextTick(fn func(func()) bool) Option {
	return func(cfg *viConfig) {
		cfg.scheduleNextTick = fn
	}
}

// WithAuxiliaryBar determines whether to draw an auxiliary bar on the left or not.
func WithAuxiliaryBar(enabled bool, config text.AuxBarConfig) Option {
	return func(cfg *viConfig) {
		cfg.enableAuxBar = enabled
		cfg.auxBarConfig = config
	}
}

// WithStatusBarConfig configures the status bar.
func WithStatusBarConfig(enabled bool, config text.StatusBarConfig) Option {
	return func(cfg *viConfig) {
		cfg.statusBarConfig = config
		cfg.statusBarEnabled = enabled
	}
}

// WithGitBar determines whether to render the changes between the
// open file's worktree and HEAD.
func WithGitBar(enabled bool, config text.GitBarConfig) Option {
	return func(cfg *viConfig) {
		cfg.enableGitBar = enabled
		cfg.gitBarConfig = config
	}
}

// WithAttr sets the default cell attributes to be rendered.
func WithAttr(attr term.Attributes) Option {
	return func(cfg *viConfig) {
		cfg.attr = attr
	}
}

// WithClipboard sets the editor.Clipboard implementation to use.
func WithClipboard(clip clipboard.Register) Option {
	return func(cfg *viConfig) {
		cfg.clipboard = clip
	}
}

// WithWrap enables or disables word wrapping mode.
func WithWrap(wrap bool) Option {
	return func(cfg *viConfig) {
		cfg.wrap = wrap
	}
}

// WithCursorCorrections enables or disables cursor out of bounds corrections.
// By default it's enabled, unless this option is passed; when disabled, clients
// must manage it themselves.
//
// This behaviour is force disabled if WithDebug Option is used.
func WithCursorCorrections(enabled bool) Option {
	return func(cfg *viConfig) {
		cfg.cursorCorrections = enabled
	}
}

// WithAutoCenter determines whether vi should automatically
// center the cursor after SetCursorAtScroll.
func WithAutoCenter(enabled bool) Option {
	return func(cfg *viConfig) {
		cfg.autoCenter = enabled
	}
}

// WithHideInitialFolds determines whether to hide the initial folds
// determined by the language query.
func WithHideInitialFolds(enabled bool) Option {
	return func(cfg *viConfig) {
		cfg.enableInitialFolds = enabled
	}
}

type nopBar struct {
}

func (n nopBar) SetStatus(status string, _ term.Attributes) {
	logrus.Debugf("vi handler status: %s", status)
}

func (n nopBar) ShowBar(bool) {
}
