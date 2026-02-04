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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestBarLayout(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantComps  []BarComponent
		wantErr    bool
		wantErrSub string // substring to look for in error
	}{
		{
			name:  "single component - language",
			input: `{{ .StatusIcon }}`,
			wantComps: []BarComponent{
				{Type: BarStatusIcon, Template: "%s"},
			},
		},
		{
			name:  "prefix before component",
			input: `⚑{{ .ExitStatus }}`,
			wantComps: []BarComponent{
				{Type: BarExitStatus, Template: "⚑%s"},
			},
		},
		{
			name:  "multiple components with separator",
			input: `{{ .Command }}+{{ .Elapsed }}`,
			wantComps: []BarComponent{
				{Type: BarCommand, Template: "%s+"},
				{Type: BarElapsed, Template: "%s"},
			},
		},
		{
			name:  "trailing padding appended to last component",
			input: `{{ .StatusIcon }}  `,
			wantComps: []BarComponent{
				{Type: BarStatusIcon, Template: "%s  "},
			},
		},
		{
			name:  "prefix/mid/trailing is parsed into templates",
			input: `pre {{ .StatusIcon }} mid {{ .ExitStatus}} post`,
			wantComps: []BarComponent{
				{Type: BarStatusIcon, Template: "pre %s mid "},
				// second receives " mid " during parsing, plus trailing " post" appended after loop
				{Type: BarExitStatus, Template: "%s post"},
			},
		},
		{
			name:       "unknown component name returns error",
			input:      `{{ .IAmNoComponent }}`,
			wantErr:    true,
			wantErrSub: "unknown status bar component",
		},
		{
			name:       "unsupported action (function call) returns error",
			input:      `{{ printf "%s" "x" }}`,
			wantErr:    true,
			wantErrSub: "template: status_bar.layout:1: function \"printf\" not defined",
		},
		{
			name:       "unsupported node type (if) returns error",
			input:      `{{ if true }}{{ .Language }}{{ end }}`,
			wantErr:    true,
			wantErrSub: "unsupported node type",
		},
		{
			name:  "fg color - quoted",
			input: `{{ .StatusIcon | fg "red" }}`,
			wantComps: []BarComponent{
				{
					Type:       BarStatusIcon,
					Template:   "%s",
					Attributes: term.Attributes{Fg: tcell.ColorRed},
				},
			},
		},
		{
			name:  "bg color - quoted",
			input: `{{ .StatusIcon | bg "blue" }}`,
			wantComps: []BarComponent{
				{
					Type:       BarStatusIcon,
					Template:   "%s",
					Attributes: term.Attributes{Bg: tcell.ColorBlue},
				},
			},
		},
		{
			name:  "fg and bg together",
			input: `{{ .StatusIcon | fg "black" | bg "red" }}`,
			wantComps: []BarComponent{
				{
					Type:       BarStatusIcon,
					Template:   "%s",
					Attributes: term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorRed},
				},
			},
		},
		{
			name:  "bold style",
			input: `{{ .Command | bold }}`,
			wantComps: []BarComponent{
				{
					Type:       BarCommand,
					Template:   "%s",
					Attributes: term.Attributes{Attrs: tcell.AttrBold},
				},
			},
		},
		{
			name:  "italic style",
			input: `{{ .ExitStatus | italic }}`,
			wantComps: []BarComponent{
				{
					Type:       BarExitStatus,
					Template:   "%s",
					Attributes: term.Attributes{Attrs: tcell.AttrItalic},
				},
			},
		},
		{
			name:  "multiple style attributes combined",
			input: `{{ .Elapsed | bold | underline }}`,
			wantComps: []BarComponent{
				{
					Type:       BarElapsed,
					Template:   "%s",
					Attributes: term.Attributes{Attrs: tcell.AttrBold | tcell.AttrUnderline},
				},
			},
		},
		{
			name:  "full styling - colors and attributes",
			input: `{{ .Elapsed | bg "red" | fg "black" | bold }}`,
			wantComps: []BarComponent{
				{
					Type:     BarElapsed,
					Template: "%s",
					Attributes: term.Attributes{
						Bg:    tcell.ColorRed,
						Fg:    tcell.ColorBlack,
						Attrs: tcell.AttrBold,
					},
				},
			},
		},
		{
			name:  "hex color - with hash",
			input: `{{ .Elapsed | fg "#ff5500" }}`,
			wantComps: []BarComponent{
				{
					Type:       BarElapsed,
					Template:   "%s",
					Attributes: term.Attributes{Fg: tcell.NewRGBColor(0xff, 0x55, 0x00)},
				},
			},
		},
		{
			name:       "hex color - without hash is an error",
			input:      `{{ .Elapsed | bg "00ff00" }}`,
			wantErr:    true,
			wantErrSub: "error is not a hex color starting with #, nor a known named W3C color in lowercase",
		},
		{
			name:  "multiple components with different attributes",
			input: `{{ .Elapsed | fg "green" }}  {{ .Command | fg "red" }}`,
			wantComps: []BarComponent{
				{
					Type:       BarElapsed,
					Template:   "%s",
					Attributes: term.Attributes{Fg: tcell.ColorGreen},
				},
				{
					Type:       BarCommand,
					Template:   "  %s",
					Attributes: term.Attributes{Fg: tcell.ColorRed},
				},
			},
		},
		{
			name:  "blank lines should be grouped with the succeeding compoent, before shift right",
			input: `█{{ .StatusIcon | bg "red" | bold }}█▓▒░  {{ .Elapsed }}`,
			wantComps: []BarComponent{
				{
					Type:     BarStatusIcon,
					Template: "█%s█▓▒░",
					Attributes: term.Attributes{
						Bg:    tcell.ColorRed,
						Attrs: tcell.AttrBold,
					},
				},
				{
					Type:       BarElapsed,
					Template:   "  %s",
					Attributes: term.Attributes{},
				},
			},
		},
		{
			name:  "components are grouped by double blank space",
			input: `█{{ .StatusIcon }}█▓▒░  {{ .Command | bg "red" }} lines  {{ .Elapsed }} {{ .ExitStatus }}`,
			wantComps: []BarComponent{
				{
					Type:       BarStatusIcon,
					Template:   "█%s█▓▒░",
					Attributes: term.Attributes{},
				},
				{
					Type:     BarCommand,
					Template: "  %s lines",
					Attributes: term.Attributes{
						Bg: tcell.ColorRed,
					},
				},
				{
					Type:       BarElapsed,
					Template:   "  %s ",
					Attributes: term.Attributes{},
				},
				{
					Type:       BarExitStatus,
					Template:   "%s",
					Attributes: term.Attributes{},
				},
			},
		},
		{
			name:  "all style attributes",
			input: `{{ .StatusIcon | bold | italic | underline | dim | reverse | strikethrough | blink }}`,
			wantComps: []BarComponent{
				{
					Type:     BarStatusIcon,
					Template: "%s",
					Attributes: term.Attributes{
						Attrs: tcell.AttrBold | tcell.AttrItalic | tcell.AttrUnderline |
							tcell.AttrDim | tcell.AttrReverse | tcell.AttrStrikeThrough | tcell.AttrBlink,
					},
				},
			},
		},
		{
			name:  "align right component with attributes ignored",
			input: `{{ .AlignRight }}`,
			wantComps: []BarComponent{
				{Type: BarAlignRight, Template: ""},
			},
		},
		{
			name:  "align center component with attributes ignored",
			input: `{{ .AlignCenter }}`,
			wantComps: []BarComponent{
				{Type: BarAlignCenter, Template: ""},
			},
		},
		{
			name:  "default color",
			input: `{{ .StatusIcon | fg "default" }}`,
			wantComps: []BarComponent{
				{
					Type:       BarStatusIcon,
					Template:   "%s",
					Attributes: term.Attributes{Fg: tcell.ColorDefault},
				},
			},
		},
		// Error cases
		{
			name:       "unknown color name",
			input:      `{{ .StatusIcon | fg "belindings" }}`,
			wantErr:    true,
			wantErrSub: "error is not a hex color starting with #, nor a known named W3C color in lowercase",
		},
		{
			name:       "fg missing color argument",
			input:      `{{ .StatusIcon | fg }}`,
			wantErr:    true,
			wantErrSub: "requires a color argument",
		},
		{
			name:       "bg missing color argument",
			input:      `{{ .StatusIcon | bg }}`,
			wantErr:    true,
			wantErrSub: "requires a color argument",
		},
		{
			name:       "unknown attribute command",
			input:      `{{ .StatusIcon | sparkle }}`,
			wantErr:    true,
			wantErrSub: "template: status_bar.layout:1: function \"sparkle\" not defined",
		},
		{
			name:       "invalid hex color - too short",
			input:      `{{ .StatusIcon | fg "#fff" }}`,
			wantErr:    true,
			wantErrSub: "error is not a hex color starting with #, nor a known named W3C color in lowercase",
		},
		{
			name:       "invalid hex color - bad characters",
			input:      `{{ .StatusIcon | fg "#gggggg" }}`,
			wantErr:    true,
			wantErrSub: "error is not a hex color starting with #, nor a known named W3C color in lowercase",
		},
		{
			name:  "blank lines should be grouped with the preceding component, after shiftright",
			input: `{{ .AlignRight }}{{ .StatusIcon }}:{{ .ExitStatus }}  {{ .Command }} lines   {{ .Elapsed | bg "navy" | bold }} `,
			wantComps: []BarComponent{
				{
					Type:     BarAlignRight,
					Template: "",
				},
				{
					Type:     BarStatusIcon,
					Template: "%s:",
				},
				{
					Type:       BarExitStatus,
					Template:   "%s  ",
					Attributes: term.Attributes{Fg: tcell.ColorDefault},
				},
				{
					Type:       BarCommand,
					Template:   "%s lines  ",
					Attributes: term.Attributes{Fg: tcell.ColorDefault},
				},
				{
					Type:       BarElapsed,
					Template:   " %s ",
					Attributes: term.Attributes{Bg: tcell.ColorNavy, Attrs: tcell.AttrBold},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBarLayout(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrSub != "" {
					assert.Contains(t, err.Error(), tt.wantErrSub)
				}
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantComps, got)
		})
	}
}
