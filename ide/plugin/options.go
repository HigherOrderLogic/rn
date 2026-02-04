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

package plugin

import (
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/term/vte"
)

// Option represents a Handler configuration option.
type Option func(*handlerConfig)

// WithVTEConfig instructs Handler to use
// the given vte.Config as the underlying vte.Handler config.
func WithVTEConfig(cfg vte.Config) Option {
	return func(hcfg *handlerConfig) {
		watcher := hcfg.cfg.Watcher
		hcfg.cfg = cfg
		if watcher != nil {
			hcfg.cfg.Watcher = workspaceapi.MultiProcessWatcher(
				watcher, hcfg.cfg.Watcher,
			)
		}
	}
}

// WithProcessWatcher instructs Handler to add the given ProcessWatcher
// to the vte.Config used. If WithVTEConfig is passed, then this ProcessWatcher
// will be added to the Watcher defined there.
func WithProcessWatcher(w workspaceapi.ProcessWatcher) Option {
	return func(hcfg *handlerConfig) {
		if hcfg.cfg.Watcher != nil {
			hcfg.cfg.Watcher = workspaceapi.MultiProcessWatcher(
				hcfg.cfg.Watcher, w,
			)
		} else {
			hcfg.cfg.Watcher = w
		}
	}
}

// WithFrame returns an option that configures
// whether Handler draws a divider between the top
// bar and the vte.
func WithFrame(frame bool) Option {
	return func(cfg *handlerConfig) {
		cfg.frame = frame
	}
}

// WithTitle returns an option that sets the title of the
// plugin handler, situated in the top bar. Passing an empty
// title, or not passing this option defaults to using
// the command and arguments as title.
func WithTitle(title string) Option {
	return func(cfg *handlerConfig) {
		cfg.title = title
	}
}

// WithFrameCharSet returns an option that configures
// Handler to use the given character set to draw
// a divider between the top bar and the vte. It's a no-op
// if frame is set to false.
func WithFrameCharSet(charSet component.FrameCharSet) Option {
	return func(cfg *handlerConfig) {
		cfg.frameCharSet = charSet
	}
}

// WithFrameAttr returns an option that configures
// the divider attributes.
func WithFrameAttr(attr term.Attributes) Option {
	return func(cfg *handlerConfig) {
		cfg.frameAttr = attr
	}
}

// WithBarConfig returns an option that configures
// the top bar.
func WithBarConfig(barConfig BarConfig) Option {
	return func(cfg *handlerConfig) {
		cfg.bar = barConfig
	}
}

type handlerConfig struct {
	bar          BarConfig
	cfg          vte.Config
	frame        bool
	frameCharSet component.FrameCharSet
	frameAttr    term.Attributes
	title        string
}

func defaultConfig() handlerConfig {
	return handlerConfig{
		bar:          DefaultBarConfig(),
		cfg:          vte.DefaultConfig(),
		frame:        true,
		frameCharSet: component.FrameCharSetDefault(),
		frameAttr:    term.Attributes{},
	}
}
