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

package ide

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

const pkgTemplateString = `{{ .Name }}

NOTES
{{ .Notes }}
	
VERSION
{{ .Latest }}

{{ if .Metadata }}{{ range $key, $value := .Metadata }}{{ upper $key }}
{{ $value }}

{{ end }}{{ else }}

{{ end }}
CREATED AT
{{ .CreatedAt.Local.Format "Jan 2, 2006 3:04 PM" }}`

const releaseTemplateString = `{{ .Package }} @ {{ .Version }}

CHANGE LOG
{{ index .Metadata "git-log" }}	

AUTHOR
{{ index .Metadata "git-author-email" }}

CREATED AT
{{ .CreatedAt.Local.Format "Jan 2, 2006 3:04 PM" }}`

var pkgTemplate *template.Template
var releaseTemplate *template.Template

func init() {
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
	}
	pkgTemplate = template.Must(template.New("ide.pkg-prev").
		Funcs(funcMap).
		Parse(pkgTemplateString))
	releaseTemplate = template.Must(template.New("ide.release-prev").
		Funcs(funcMap).
		Parse(releaseTemplateString))
}

func makePackagePreviewComponent(
	man release.Package,
	attr term.Attributes,
) component.Responsive {
	var builder strings.Builder
	var str string
	err := writePackageTemplate(&builder, man)
	if err != nil {
		str = fmt.Sprintf("build preview: %v", err)
	} else {
		str = builder.String()
	}

	ret := component.NewResponsiveString(str, component.StringResponsiveConfig{
		NoSplitWords: false,
		StringConfig: component.StringConfig{
			Alignment:            component.AlignmentCentered,
			Attributes:           attr,
			BackgroundAttributes: attr,
			PaddingVertical:      2,
			PaddingHorizontal:    2,
			MinWidth:             100,
		},
	})
	return ret
}

func writePackageTemplate(w io.Writer, m release.Package) error {
	if err := pkgTemplate.Execute(w, m); err != nil {
		return fmt.Errorf("template execute: %v", err)
	}
	return nil
}

func makeReleasePreviewComponent(
	man release.Bundle,
	attr term.Attributes,
) component.Responsive {
	var builder strings.Builder
	var str string
	err := writeReleaseTemplate(&builder, man)
	if err != nil {
		str = fmt.Sprintf("build preview: %v", err)
	} else {
		str = builder.String()
	}

	ret := component.NewResponsiveString(str, component.StringResponsiveConfig{
		NoSplitWords: false,
		StringConfig: component.StringConfig{
			Alignment:            component.AlignmentCentered,
			Attributes:           attr,
			BackgroundAttributes: attr,
			PaddingVertical:      2,
			PaddingHorizontal:    2,
			MinWidth:             100,
		},
	})
	return ret
}

func writeReleaseTemplate(w io.Writer, m release.Bundle) error {
	if err := releaseTemplate.Execute(w, m); err != nil {
		return fmt.Errorf("template execute: %v", err)
	}
	return nil
}
