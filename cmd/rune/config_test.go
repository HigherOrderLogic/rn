// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/term"
	"go.uber.org/mock/gomock"

	"unstable.build/go-tui/browser/browsertest"
)

func TestGetGUIKeyMapping(t *testing.T) {
	t.Run("parses valid mappings", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := browsertest.NewMockBrowser(ctrl)

		cfg := config.MapConfig(map[string]any{
			"key_mapping": map[string]any{
				"<capslock>": "<esc>",
				"<numlock>":  "a",
			},
		})

		got := getGUIKeyMapping(b, cfg)
		want := map[term.KeyComb]term.KeyComb{
			{Key: term.KeyCapsLock}: {Key: term.KeyEsc},
			{Key: term.KeyNumLock}:  {Ch: 'a'},
		}
		assert.Equal(t, want, got)
	})

	t.Run("returns nil when absent", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := browsertest.NewMockBrowser(ctrl)

		got := getGUIKeyMapping(b, config.MapConfig(map[string]any{}))
		assert.Nil(t, got)
	})

	t.Run("skips invalid entries and notifies", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := browsertest.NewMockBrowser(ctrl)
		// One report for the bad source, one for the bad target, one for
		// the non-string value.
		b.EXPECT().Notify(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("", nil).Times(3)

		cfg := config.MapConfig(map[string]any{
			"key_mapping": map[string]any{
				"<capslock>":   "<esc>",
				"<not-a-key>":  "<esc>",
				"<numlock>":    "<also-bad>",
				"<scrolllock>": 42,
			},
		})

		got := getGUIKeyMapping(b, cfg)
		want := map[term.KeyComb]term.KeyComb{
			{Key: term.KeyCapsLock}: {Key: term.KeyEsc},
		}
		assert.Equal(t, want, got)
	})
}

func TestGetGUIFontSize(t *testing.T) {
	t.Run("returns 0 when absent to select DPI-aware default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := browsertest.NewMockBrowser(ctrl)

		got := getGUIFontSize(b, config.MapConfig(map[string]any{}))
		assert.Equal(t, float64(0), got)
	})

	t.Run("returns configured size when set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := browsertest.NewMockBrowser(ctrl)

		cfg := config.MapConfig(map[string]any{"font_size": 15.0})
		got := getGUIFontSize(b, cfg)
		assert.Equal(t, float64(15), got)
	})
}
