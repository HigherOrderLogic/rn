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

package browser

import (
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/handler"
)

// DefaultConfig returns the default Config.
func DefaultConfig() Config {
	return Config{
		Wallpaper:           NopWallpaper(),
		FocusTabAttr:        term.Attributes{Fg: tcell.ColorWhite},
		NonFocusTabAttr:     term.Attributes{Fg: tcell.ColorRed},
		FrameUnionCharSet:   component.DefaultFrameUnionCharSet(),
		WindowManagerConfig: handler.DefaultWindowManagerConfig(),
		FrameUnion:          handler.DefaultWindowManagerConfig().Frame,
		PromptConfig: PromptConfig{
			TextAttr:       term.Attributes{},
			HighlightAttr:  term.Attributes{Bg: tcell.ColorRed, Fg: tcell.ColorWhite},
			BackgroundAttr: term.Attributes{},
			MinWidth:       60,
		},
		Notifications: logNotifications{},
	}
}

// PromptConfig holds configuration for the browser's Prompt component.
type PromptConfig struct {
	TextAttr       term.Attributes
	HighlightAttr  term.Attributes
	BackgroundAttr term.Attributes
	MinWidth       int
}

// Wallpaper is a tui.Component wallpaper factory.
// NOTE: we might want to move it to the component package
// and call it a Factory.
type Wallpaper struct {
	BackgroundAttr term.Attributes
	NewComponent   func() tui.Component
}

// NopWallpaper is a Wallpaper of component.Nop.
func NopWallpaper() Wallpaper {
	return Wallpaper{
		NewComponent: func() tui.Component {
			return component.Nop()
		},
	}
}

// Config holds configuration for an browser.Component.
type Config struct {
	Notifications
	Wallpaper Wallpaper

	FocusTabAttr     term.Attributes
	NonFocusTabAttr  term.Attributes
	TabBarOffset     int
	TabBarHeight     int
	TabNameSeparator string
	FrameUnion       bool
	OnTabsClick      func(int) bool

	PromptConfig

	component.FrameUnionCharSet
	handler.WindowManagerConfig
}

type logNotifications struct {
}

func (n logNotifications) Notify(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	var l log.Level
	switch level {
	case browserapi.LevelWarn:
		l = log.WarnLevel
	case browserapi.LevelError:
		l = log.ErrorLevel
	case browserapi.LevelInfo:
		l = log.InfoLevel
	case browserapi.LevelSuccess:
		l = log.InfoLevel
	}
	log.WithField(logging.KeyClass, "notifications").Logf(l, msg, args...)
	return "", nil
}

func (n logNotifications) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	return n.Notify(level, msg, args...)
}

func (n logNotifications) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return nil
}
