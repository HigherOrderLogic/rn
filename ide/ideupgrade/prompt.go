// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package ideupgrade

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/debug"
)

const (
	upgradeNowOpt  = "   Upgrade Now   "
	remindLaterOpt = "   Remind Me Later   "
	skipVersionOpt = "   Skip This Version   "
)

// showPrompt renders the floating upgrade prompt for manifest. The
// user's choice is persisted (remind / skip) and, when "Upgrade Now"
// is chosen, runUpgrade is invoked on a background goroutine.
func (m *Manager) showPrompt(ctx context.Context, manifest Manifest) {
	if m.cfg.WindowManager == nil {
		_, _ = m.cfg.Notifications.Notify(browserapi.LevelInfo,
			"Rune %s is available", manifest.Version)
		return
	}

	message := fmt.Sprintf("A new version of Rune is available: **%s** (current: %s).\n\nUpgrade now?",
		manifest.Version, m.cfg.CurrentVersion)
	if manifest.Changelog != "" {
		message = manifest.Changelog + "\n\n---\n\n" + message
	}

	prompt := handler.NewPrompt(handler.PromptConfig{
		HighlightAttr: term.Attributes{
			Attrs: term.AttrBold,
			Bg:    term.ColorBlue,
		},
		OptionAttr: term.Attributes{
			Attrs: term.AttrBold,
			Bg:    term.ColorGray,
		},
		OptionBindings: []term.KeyComb{{Ch: 'y'}, {Ch: 'l'}, {Ch: 's'}},
		PromptConfig: component.PromptConfig{
			Message:    message,
			Options:    []string{upgradeNowOpt, remindLaterOpt, skipVersionOpt},
			NewMessage: markdownOrFallback(m.cfg.Parser, m.cfg.ScheduleNextTick),
		},
		PromptHandler: handler.FuncPromptHandler(func(idx int, _ string) {
			switch idx {
			case 0: // Upgrade Now
				go debug.CapturePanicReport(func() {
					if err := m.runUpgrade(ctx, manifest); err != nil {
						log.WithError(err).Warn("ideupgrade: run upgrade")
					}
				})
			case 1: // Remind Me Later
				m.remindLater(ctx)
			case 2: // Skip This Version
				m.skipVersion(ctx, manifest.Version)
			}
		}, func() error { return nil }),
	})

	m.cfg.ScheduleNextTick(func() {
		if _, err := m.cfg.WindowManager.Floating(prompt, browserapi.FloatingConfig{
			Alignment: component.AlignmentCentered,
		}); err != nil {
			log.WithError(err).Warn("ideupgrade: show upgrade prompt")
		}
	})
}

// markdownOrFallback returns a NewMessage callback that renders the
// prompt body as markdown when possible, falling back to a plain
// responsive string when the parser is unavailable. It mirrors the
// helper in idepkg/update_checker.go but is duplicated here to avoid
// pulling idepkg into ideupgrade.
func markdownOrFallback(
	parser syntaxapi.Parser, scheduleNextTick func(func()) bool,
) func(string) component.Floating {
	return func(str string) component.Floating {
		mcfg := markdown.DefaultConfig()
		mcfg.HeaderPrefix = false
		mcfg.ScheduleNextTick = scheduleNextTick
		mcfg.Parser = parser
		mkd, err := markdown.NewWithConfig(str, mcfg)
		if err == nil {
			return component.NewSpan(mkd, component.SpanConfig{
				PadHorizontal:    4,
				PadVertical:      2,
				ContentAlignment: component.AlignmentCentered,
			})
		}
		cfg := component.StringResponsiveConfig{
			NoSplitWords: true,
			StringConfig: component.StringConfig{
				PaddingVertical:   4,
				PaddingHorizontal: 4,
				Alignment:         component.AlignmentCentered,
			},
		}
		messageResponsive := component.NewResponsiveString(str, cfg)

		return component.NewAspectRatioFloatingResponsive(
			messageResponsive, component.DefaultAspectRatio)
	}
}
