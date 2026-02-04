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

package command

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Manual represents a command's manual and documentation.
type Manual struct {
	Name string

	// Summary is a short 80-100 character description.
	Summary string

	// Synopsis is a single line synopsis of how
	// this CLI is to be used. It should ONLY include
	// the semantic information about how arguments are parsed.
	//
	// Example: [<options>] [<revision-range>] [[--] <path>...]
	Synopsis string

	// Commands is a list of accepted commands or nil
	// if no commands are expected.
	Commands []Manual

	// AliasOf defines this command as an alias of the
	// given command or sequence of commands.
	AliasOf []string
}

const manualTemplate = `USAGE
{{ .Name }} {{ .Synopsis }}

DESCRIPTION{{ with .AliasOf }}{{ $length := len . }} {{if eq $length 1 }}
Alias of {{ index . 0 }}{{else}}
Alias of the following sequence of commands:

{{ range . }}- {{ . }}
{{ end }}{{ end }}{{end}}
{{ .Summary }}{{ with .Commands }}

SUB-COMMANDS{{ range . }}
    - {{ .Name }}{{ end }}
{{ end }}`

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.New("test").Parse(manualTemplate))
}

func makeManualComponent(
	man Manual,
	frameCharSet component.FrameCharSet,
	attr term.Attributes,
) component.Responsive {
	var builder strings.Builder
	var str string
	err := writeTemplate(&builder, man)
	if err != nil {
		str = fmt.Sprintf("ERROR: build manual: %v", err)
	} else {
		str = builder.String()
	}

	minWidth := minWidthManualComponent
	if frameCharSet != (component.FrameCharSet{}) {
		minWidth -= 2
	}

	ret := component.NewResponsiveString(str, component.StringResponsiveConfig{
		NoSplitWords: true,
		StringConfig: component.StringConfig{
			Alignment:            component.AlignmentCentered,
			Attributes:           attr,
			BackgroundAttributes: attr,
			PaddingVertical:      2,
			PaddingHorizontal:    2,
			MinWidth:             minWidth,
		},
	})
	return ret
}

func writeTemplate(w io.Writer, m Manual) error {
	if err := tmpl.Execute(w, m); err != nil {
		return fmt.Errorf("template execute: %v", err)
	}
	return nil
}

func getSubcommandManual(cmd Manual, input []string) (
	man Manual, ok bool,
) {
	if len(input) == 0 {
		return
	}
	name := input[0]
	for _, subCmd := range cmd.Commands {
		if subCmd.Name == name {
			textSubCmd := subCmd
			subSubCmd, subSubCmdOk := getSubcommandManual(textSubCmd, input[1:])
			if subSubCmdOk {
				return subSubCmd, true
			}
			return textSubCmd, true
		}
	}
	return
}

func manualToName(man Manual) string {
	return man.Name
}
