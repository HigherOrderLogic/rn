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

package plugin

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestBarDraw(t *testing.T) {
	suite := []struct {
		desc     string
		width    int
		layout   string
		action   func(b *pluginHandlerBar)
		expected string
	}{
		{"empty layout renders nothing", 20, "", nil,
			"                    ",
		},
		{"status icon, not done", 20, "{{ .StatusIcon }}", nil,
			"⠃                   ",
		},
		{"status icon with padding on the left NOT DONE, next align left", 20,
			" {{ .StatusIcon | bg \"gray\" | fg \"white\" }}   {{ .Elapsed }}", nil,
			" ⠃   0s             ",
		},
		{"status icon with padding on the left NOT DONE, next align right", 20,
			" {{ .StatusIcon | bg \"gray\" | fg \"white\" }}   {{ .AlignRight }}{{ .Elapsed }}", nil,
			" ⠃                0s",
		},
		{"status icon with padding on the left NOT DONE, next align center", 20,
			" {{ .StatusIcon | bg \"gray\" | fg \"white\" }}   {{ .AlignCenter }}{{ .Elapsed }}", nil,
			" ⠃        0s        ",
		},
		{"status icon with padding on the left DONE, next align center", 20,
			" {{ .StatusIcon | bg \"gray\" | fg \"white\" }}   {{ .AlignCenter }}{{ .Elapsed }}",
			func(h *pluginHandlerBar) {
				h.setDone(nil)
			},
			" ▀        0s        ",
		},
		{"status icon right aligned with no padding NOT DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight }}{{ .StatusIcon | bg \"gray\" | fg \"white\" }}", nil,
			"         0s        ⠃",
		},
		{"status icon right aligned with no padding DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight}}{{ .StatusIcon | bg \"gray\" | fg \"white\" }}",
			func(h *pluginHandlerBar) {
				h.setDone(nil)
			},
			"         0s        ▀",
		},
		{"status icon right aligned with padding on the right NOT DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight }}{{ .StatusIcon | bg \"gray\" | fg \"white\" }} ", nil,
			"         0s       ⠃ ",
		},
		{"status icon right aligned with padding on the right DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight}}{{ .StatusIcon | bg \"gray\" | fg \"white\" }} ",
			func(h *pluginHandlerBar) {
				h.setDone(nil)
			},
			"         0s       ▀ ",
		},
		{"status icon right aligned with padding on the left NOT DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight }} {{ .StatusIcon | bg \"gray\" | fg \"white\" }}", nil,
			"         0s        ⠃",
		},
		{"status icon right aligned with padding on the left DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight}} {{ .StatusIcon | bg \"gray\" | fg \"white\" }}",
			func(h *pluginHandlerBar) {
				h.setDone(nil)
			},
			"         0s        ▀",
		},
		{"status icon right aligned with padding on the left and right NOT DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight }} {{ .StatusIcon | bg \"gray\" | fg \"white\" }} ", nil,
			"         0s       ⠃ ",
		},
		{"status icon right aligned with padding on the left and right DONE with error, next align center", 20,
			"{{ .AlignCenter }}{{ .ExitStatus }}{{ .AlignRight}} {{ .StatusIcon | bg \"gray\" | fg \"white\" }} ",
			func(h *pluginHandlerBar) {
				h.setDone(errors.New("swift error"))
			},
			"      non-zero    ▀ ",
		},
		{"status icon right aligned with padding on the left and right NOT DONE, next align center", 20,
			"{{ .AlignCenter }}{{ .Elapsed }}{{ .AlignRight}} {{ .StatusIcon | bg \"gray\" | fg \"white\" }} ",
			func(h *pluginHandlerBar) {
			},
			"         0s       ⠃ ",
		},
		{"status icon right aligned with padding on the left and right DONE, double elapsed", 20,
			"{{ .AlignRight}}{{ .Elapsed }}   {{ .StatusIcon | bg \"gray\" | fg \"white\" }}   {{ .Elapsed }}  ",
			func(h *pluginHandlerBar) {
			},
			"            ⠃   0s  ",
		},
		{"status icon center aligned with padding on the left and right DONE", 20,
			"{{ .Elapsed }}{{ .AlignCenter }} {{ .StatusIcon | bg \"gray\" | fg \"white\" }} {{ .AlignRight }}{{ .Elapsed }}  ",
			func(h *pluginHandlerBar) {
			},
			"         ⠃      0s  ",
		},
	}

	for _, test := range suite {
		t.Run(test.desc, func(t *testing.T) {
			var err error
			config := DefaultBarConfig()
			config.BackgroundColor = tcell.ColorRed // exercise bg setting for panics
			config.StatusAnimationFrames, _ = component.ProgressAnimationFrames()
			config.Layout, err = ParseBarLayout(test.layout)
			require.NoError(t, err)
			h := newPluginHandlerBar("", term.NopInterrupter(), config)
			h.runningPrecision = time.Minute
			h.donePrecision = time.Minute
			h.rebuildElapsed()
			h.Resize(test.width, 1)
			if test.action != nil {
				test.action(h)
			}
			w := term.NewStringWriter(test.width, 1)
			h.Draw(w)
			require.NoError(t, w.Flush())
			assert.Equal(t, test.expected, w.String())
			assert.NoError(t, h.Close())
		})
	}
}
