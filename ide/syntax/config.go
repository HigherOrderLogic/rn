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

package syntax

import (
	"context"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
)

// Config configures a tree parser.
type Config struct {
	// CaptureNamesAttributes maps the capture names
	// of a highlights.scm tree-sitter file, into term.Attributes.
	CaptureNamesAttributes map[string]term.Attributes

	// ScheduleNextTick schedules an arbitrary function to be run
	// in the next event-loop tick.
	ScheduleNextTick func(func()) bool

	// ReparseOnErrors forces tree to re-parse the entire file
	// if there are failures to keep tree sitter's tree and the file contents
	// in sync.
	ReparseOnErrors bool

	// StrictErrors enables showing when the parser reports an error
	// but incremental parsing overall didn't fail.
	StrictErrors bool

	// Autoindent enables or disables indentation features. In practice,
	// if disabled, IndentationAt always returns 0, false.
	Autoindent bool
}

// PkgManager abstracts a subset of idepkg.Manager for a tree parser.
type PkgManager interface {
	LibDir(ctx context.Context, pkgID string) (iterator.Iterator[string], error)
}

// LocationSetter abstracts the ability to visualize locations.
type LocationSetter interface {
	SetLocationList(textapi.LocationList)
}

// FuncLocationSetter returns a LocationSetter that uses the given fn to
// set locations.
func FuncLocationSetter(fn func(textapi.LocationList)) LocationSetter {
	return fnLocationList{fn: fn}
}

// DefaultConfig returns a sane configuration for a tree parser.
func DefaultConfig() Config {
	return Config{
		CaptureNamesAttributes: defaultCaptureNamesAttributes,
		ScheduleNextTick:       func(cb func()) bool { cb(); return true },
		ReparseOnErrors:        true,
		StrictErrors:           false,
		Autoindent:             true,
	}
}

var defaultCaptureNamesAttributes = map[string]term.Attributes{
	"function":         {},
	"function.builtin": {Fg: tcell.ColorYellow},
	"function.method":  {},
	"type":             {},
	"property":         {},
	"variable":         {},
	"operator":         {},
	"keyword":          {Fg: tcell.ColorYellow},
	"string":           {Fg: tcell.ColorFuchsia},
	"escape":           {},
	"number":           {Fg: tcell.ColorRed},
	"constant.builtin": {},
	"comment":          {Fg: tcell.ColorBlue},

	// Markup captures are emitted by common tree-sitter queries for markdown,
	// markdown_inline, djot, rst, latex, typst, vimdoc, gitcommit, pod, and
	// other prose/markup-like languages.
	"markup.heading":       {Fg: tcell.ColorYellow},
	"markup.raw":           {Fg: tcell.ColorFuchsia},
	"markup.raw.delimiter": {Fg: tcell.ColorYellow},
	"markup.link":          {Fg: tcell.ColorYellow},
	"markup.link.url":      {Fg: tcell.ColorFuchsia},
	"markup.link.label":    {Fg: tcell.ColorYellow},
	"markup.link.text":     {Fg: tcell.ColorYellow},
	"markup.list":          {Fg: tcell.ColorYellow},
	"markup.quote":         {Fg: tcell.ColorBlue},
	"markup.strong":        {Fg: tcell.ColorYellow},
	"markup.italic":        {Fg: tcell.ColorYellow},
	"markup.strikethrough": {Fg: tcell.ColorBlue},
	"markup.underline":     {Fg: tcell.ColorYellow},
	"markup.math":          {Fg: tcell.ColorFuchsia},

	// Legacy markdown captures used by older upstream queries.
	"text.title":     {Fg: tcell.ColorYellow},
	"text.literal":   {Fg: tcell.ColorFuchsia},
	"text.uri":       {Fg: tcell.ColorFuchsia},
	"text.reference": {Fg: tcell.ColorYellow},
	"text.emphasis":  {Fg: tcell.ColorYellow},
	"text.strong":    {Fg: tcell.ColorYellow},
}

func captureNameAttributes(
	captureNamesAttributes map[string]term.Attributes, name string,
) term.Attributes {
	for {
		if attr, ok := captureNamesAttributes[name]; ok {
			return attr
		}
		if attr, ok := defaultCaptureNamesAttributes[name]; ok {
			return attr
		}
		if !strings.HasPrefix(name, "markup.") && !strings.HasPrefix(name, "text.") {
			return term.Attributes{}
		}

		idx := strings.LastIndexByte(name, '.')
		if idx < 0 {
			return term.Attributes{}
		}
		name = name[:idx]
	}
}

type fnLocationList struct {
	fn func(textapi.LocationList)
}

func (f fnLocationList) SetLocationList(list textapi.LocationList) {
	f.fn(list)
}
