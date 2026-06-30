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
	"context"
	_ "embed"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	sensiblebrowser "github.com/ernestrc/sensible/browser"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/markdown"
	hmarkdown "unstable.build/go-tui/handler/markdown"
	"unstable.build/go-tui/ide/idetutorial/starlarktutorial"
	"unstable.build/go-tui/text"
)

//go:embed cheatsheet.md.tmpl
var cheatsheetTmpl string

// cheatsheetWidth is the fixed width of the cheatsheet floating window.
// The content is known and authored to fit, so a static width keeps the
// table columns tight instead of stretching to the markdown component's
// natural ideal width.
const cheatsheetWidth = 100

// cheatsheetData is the template context for cheatsheet.md.tmpl.
type cheatsheetData struct {
	CommandKey string
	Modal      bool
	EditorMode string
	AutoSave   bool
}

// renderCheatsheet fills cheatsheet.md.tmpl from the resolved key
// bindings and editor mode, producing a keys-first Markdown cheatsheet.
// The `key` template func resolves a command line to the user's bound
// chord, falling back to the command-prompt sequence when unbound.
func renderCheatsheet(cfg text.Config, modal bool, editorMode string, autoSave bool) (string, error) {
	commandKey := starlarktutorial.PrettyKeySpec(cfg.CommandEvent.String())
	lookup := commandKeyLookup(cfg.CommandKeyBindings)

	key := func(cmd string, args ...string) string {
		line := strings.Join(append([]string{cmd}, args...), " ")
		if k, ok := lookup[line]; ok {
			return k
		}
		if len(args) > 0 {
			if k, ok := lookup[cmd]; ok {
				return k
			}
		}
		return commandKey + " " + line
	}

	// jumptoast bindings are prompt-prefill `echo {prompt}jumptoast ...`
	// macros, so the command-line lookup cannot match them by name. Match
	// the binding by its `local.definition.<kind>` capture instead and
	// fall back to the command-prompt sequence when no binding is found.
	jumpKey := func(kind string) string {
		capture := "local.definition." + kind
		for line, k := range lookup {
			if strings.Contains(line, "jumptoast") && strings.Contains(line, capture) {
				return k
			}
		}
		return commandKey + " jumptoast"
	}

	tmpl, err := template.New("cheatsheet").Funcs(template.FuncMap{
		"key":     key,
		"jumpkey": jumpKey,
	}).Parse(cheatsheetTmpl)
	if err != nil {
		return "", fmt.Errorf("parse cheatsheet template: %w", err)
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, cheatsheetData{
		CommandKey: commandKey,
		Modal:      modal,
		EditorMode: editorMode,
		AutoSave:   autoSave,
	}); err != nil {
		return "", fmt.Errorf("execute cheatsheet template: %w", err)
	}
	return b.String(), nil
}

// commandKeyLookup inverts resolved key bindings into a command-line to
// pretty-key map. Each command line in a binding maps to that binding's
// key; the first writer wins so earlier bindings stay stable.
func commandKeyLookup(bindings map[term.KeyComb][][]string) map[string]string {
	lookup := make(map[string]string)
	for kc, cmds := range bindings {
		key := starlarktutorial.PrettyKeySpec(kc.String())
		for _, cmdAndArgs := range cmds {
			line := strings.Join(cmdAndArgs, " ")
			if _, ok := lookup[line]; !ok {
				lookup[line] = key
			}
		}
	}
	return lookup
}

// cheatsheet renders a keys-first cheatsheet from the user's resolved key
// bindings and editor mode and opens it as a read-only markdown view in a
// centered floating window.
func (e *ex) cheatsheet(_ context.Context, _ ...string) error {
	md, err := renderCheatsheet(e.config, e.editorModeModal, e.editorMode, e.editorAutoSave)
	if err != nil {
		return err
	}
	mdCfg := e.config.Markdown
	mdCfg.HeaderPrefix = false
	mdComp, err := markdown.NewWithConfig(md, mdCfg)
	if err != nil {
		return fmt.Errorf("new markdown component: %w", err)
	}
	mdh := hmarkdown.New(mdComp, hmarkdown.WithOnLinkClick(openCheatsheetLink))
	span := handler.NewSpan(mdh, component.SpanConfig{
		PadHorizontal:    2,
		ContentAlignment: component.AlignmentCentered,
	})

	var win browser.Window
	floating := browser.FuncFloatingHandler(span, func() error {
		defer win.Close() //nolint:errcheck
		return mdh.Close()
	})
	staticWidth := browser.FuncFloating(floating, func() (int, int) {
		_, h := floating.Dimensions()
		return cheatsheetWidth, h
	})
	win, err = e.comp.Floating(staticWidth, browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	})
	return err
}

// browseURL opens a URL in the system browser. It is a package var so
// tests can stub it instead of launching a real browser.
var browseURL = sensiblebrowser.Browse

// openCheatsheetLink opens http(s) links from the cheatsheet in the
// system browser. Other schemes (such as in-document anchors) are left
// to the markdown handler's default handling.
func openCheatsheetLink(link *url.URL) bool {
	if link.Scheme != "http" && link.Scheme != "https" {
		return false
	}
	if err := browseURL(link); err != nil {
		log.WithError(err).Warnf("cheatsheet: open url %q", link.String())
	}
	return true
}
