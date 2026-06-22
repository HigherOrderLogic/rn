// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2018-2026 Unstable Build, All Rights Reserved.
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

package idepkg

import (
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/component"
)

// Option configures a Manager.
type Option func(*Manager)

// WithFrameCharSet sets the component.FrameCharSet to use for prompts.
func WithFrameCharSet(fcs component.FrameCharSet) Option {
	return func(m *Manager) {
		m.frameCharSet = fcs
	}
}

// WithSyntaxParser sets the syntax parser for showing config changes.
func WithSyntaxParser(parser syntaxapi.Parser) Option {
	return func(m *Manager) {
		m.parser = parser
	}
}

// WithEditorMode sets the resolved editor mode ("modal", "modeless", "exo")
// exposed to package config.star scripts as the RUNE_EDITOR_MODE predeclared
// global. Empty values are not forwarded.
func WithEditorMode(mode string) Option {
	return func(m *Manager) {
		m.editorMode = mode
	}
}

// WithConfigBase sets a provider for the editor's default config tree. It is
// predeclared as `config` when reading the user's .star config so
// overlay-style mutations (config[...] = ...) and Rune's managed block
// resolve without binding `config`. The provider is called per read so it
// reflects the current configuration.
func WithConfigBase(base func() map[string]any) Option {
	return func(m *Manager) {
		m.configBase = base
	}
}

// WithAfterConfigMerge installs a hook invoked after a package config merge is
// written to the user config file. The hook is generic and unaware of GUI or
// environment concerns; callers inspect the applied diff and report back
// whether any live reload happened via ConfigMergeResult. The hook runs for
// both the auto-apply path and the prompt's Allow path, and never for Deny.
func WithAfterConfigMerge(hook func(ConfigMergeEvent) (ConfigMergeResult, error)) Option {
	return func(m *Manager) {
		m.afterConfigMerge = hook
	}
}
