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

package template

import (
	"errors"
	"fmt"
	"strconv"
	"text/template/parse"

	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// AllowedFuncs returns a list of allowed functions to pass to parse.Parse
// such that ParseAction can parse.ActionNodes into a set of named components
// and their attributes.
func AllowedFuncs() map[string]any {
	return map[string]any{
		"fg":            func(string) string { return "" },
		"bg":            func(string) string { return "" },
		"bold":          func() string { return "" },
		"underline":     func() string { return "" },
		"reverse":       func() string { return "" },
		"blink":         func() string { return "" },
		"dim":           func() string { return "" },
		"italic":        func() string { return "" },
		"strikethrough": func() string { return "" },
	}
}

// ParseAction can be used alongside the standard library's template/parse
// to parse nodes into a set of string components with attributes after calling parse.Parse.
func ParseAction(n *parse.ActionNode) (string, term.Attributes, error) {
	if n.Pipe == nil || len(n.Pipe.Cmds) == 0 {
		return "", term.Attributes{}, fmt.Errorf("empty pipeline at position %d", n.Pos)
	}

	// first command must be the field reference: {{ .Status }}
	first := n.Pipe.Cmds[0]
	if len(first.Args) != 1 {
		return "", term.Attributes{}, fmt.Errorf("invalid field reference")
	}

	field, ok := first.Args[0].(*parse.FieldNode)
	if !ok || len(field.Ident) != 1 {
		return "", term.Attributes{}, fmt.Errorf("unsupported field expression")
	}

	fieldName := field.Ident[0]

	// remaining commands are attribute filters: {{ .Status | attr "bold" "red" }}
	var attrs term.Attributes
	for _, cmd := range n.Pipe.Cmds[1:] {
		if len(cmd.Args) == 0 {
			return "", term.Attributes{}, fmt.Errorf("empty function call in pipeline")
		}

		ident, ok := cmd.Args[0].(*parse.IdentifierNode)
		if !ok {
			return "", term.Attributes{}, fmt.Errorf("expected identifier in pipeline")
		}

		switch ident.Ident {
		case "bg":
			for _, arg := range cmd.Args[1:] {
				s, ok := arg.(*parse.StringNode)
				if !ok {
					return "", term.Attributes{}, fmt.Errorf("fg arguments must be a string")
				}
				var err error
				attrs.Bg, err = getColor(s.Text)
				if err != nil {
					return "", term.Attributes{}, err
				}
			}
			if len(cmd.Args[1:]) == 0 {
				return "", term.Attributes{}, errors.New("bg requires a color argument")
			}
		case "fg":
			for _, arg := range cmd.Args[1:] {
				s, ok := arg.(*parse.StringNode)
				if !ok {
					return "", term.Attributes{}, fmt.Errorf("bg arguments must be a string")
				}
				var err error
				attrs.Fg, err = getColor(s.Text)
				if err != nil {
					return "", term.Attributes{}, err
				}
			}
			if len(cmd.Args[1:]) == 0 {
				return "", term.Attributes{}, errors.New("fg requires a color argument")
			}
		case "bold":
			attrs.Attrs |= tcell.AttrBold
		case "underline":
			attrs.Attrs |= tcell.AttrUnderline
		case "reverse":
			attrs.Attrs |= tcell.AttrReverse
		case "blink":
			attrs.Attrs |= tcell.AttrBlink
		case "dim":
			attrs.Attrs |= tcell.AttrDim
		case "italic":
			attrs.Attrs |= tcell.AttrItalic
		case "strikethrough":
			attrs.Attrs |= tcell.AttrStrikeThrough
		default:
			return "", term.Attributes{}, fmt.Errorf("unsupported pipeline command %q", ident.Ident)
		}
	}
	return fieldName, attrs, nil
}

func getColor(name string) (tcell.Color, error) {
	if name == "default" {
		return tcell.ColorDefault, nil
	}
	if c, ok := tcell.ColorNames[name]; ok {
		return c, nil
	}
	if len(name) == 7 && name[0] == '#' {
		v, e := strconv.ParseInt(name[1:], 16, 32)
		if e != nil {
			return 0, errInvalidColor
		}
		return tcell.NewHexColor(int32(v)), nil
	}
	return 0, errInvalidColor
}

var errInvalidColor = errors.New("error is not a hex color starting with #, " +
	"nor a known named W3C color in lowercase")
