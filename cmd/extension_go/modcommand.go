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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/debug"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"unstable.build/go-tui/ide/idelsp/lspcmd"
)

// modCommandHandler creates a handler for module management commands
// (tidy, vendor) that execute gopls workspace commands against the
// go.mod file. These commands expect {"URIs": [goModURI]} as arguments.
func modCommandHandler(
	lsp semanticapi.LSP,
	notify browserapi.Notifications,
	goplsCommand string,
) textapi.CommandHandler {
	return &modCmd{
		lsp: lsp, notify: notify, goplsCommand: goplsCommand,
		buildArgs: func(goModURI string) any {
			return map[string]any{"URIs": []string{goModURI}}
		},
	}
}

// vulncheckHandler creates a handler for gopls.vulncheck (synchronous).
// Unlike the deprecated gopls.run_govulncheck which returned only a
// progress token, gopls.vulncheck blocks until the scan completes and
// returns the actual vulnerability results. Progress is still reported
// via $/progress notifications during execution.
func vulncheckHandler(
	lsp semanticapi.LSP,
	notify browserapi.Notifications,
) textapi.CommandHandler {
	return &vulncheckCmd{lsp: lsp, notify: notify}
}

var _ textapi.CommandHandler = (*vulncheckCmd)(nil)

type vulncheckCmd struct {
	lsp    semanticapi.LSP
	notify browserapi.Notifications
}

func (h *vulncheckCmd) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) error {
	if cmd.Resource == nil {
		return fmt.Errorf("no file open; open a file first")
	}

	fileURI := lspcmd.URIToLSP(cmd.URI)
	goModURI := findGoModURI(fileURI)

	argsData, err := json.Marshal(map[string]any{"URI": goModURI, "Pattern": "./..."})
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}

	go debug.CapturePanicReport(func() {
		_, err := h.lsp.ExecuteCommand(ctx, semanticapi.ExecuteCommandParams{
			Command:   "gopls.vulncheck",
			Arguments: []json.RawMessage{argsData},
		})
		if err != nil {
			_, _ = h.notify.Notify(browserapi.LevelError, "vulncheck: %s", err)
			return
		}
	})
	return nil
}

func (h *vulncheckCmd) Complete(
	_ context.Context, _ string, _ []string,
) (iterator.Iterator[string], error) {
	return iterator.Empty[string](), nil
}

var _ textapi.CommandHandler = (*modCmd)(nil)

type modCmd struct {
	lsp          semanticapi.LSP
	notify       browserapi.Notifications
	goplsCommand string
	buildArgs    func(goModURI string) any
}

func (h *modCmd) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) error {
	if cmd.Resource == nil {
		return fmt.Errorf("no file open; open a file first")
	}

	fileURI := lspcmd.URIToLSP(cmd.URI)
	goModURI := findGoModURI(fileURI)

	argsData, err := json.Marshal(h.buildArgs(goModURI))
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}

	go debug.CapturePanicReport(func() {
		result, err := h.lsp.ExecuteCommand(ctx, semanticapi.ExecuteCommandParams{
			Command:   h.goplsCommand,
			Arguments: []json.RawMessage{argsData},
		})
		if err != nil {
			_, _ = h.notify.Notify(browserapi.LevelError, "execute %s: %s", h.goplsCommand, err)
			return
		}

		if result != "" {
			_, _ = h.notify.Notify(browserapi.LevelInfo, "%s: %s", h.goplsCommand, result)
		} else {
			_, _ = h.notify.Notify(browserapi.LevelInfo, "%s completed", h.goplsCommand)
		}
	})
	return nil
}

func (h *modCmd) Complete(
	_ context.Context, _ string, _ []string,
) (iterator.Iterator[string], error) {
	return iterator.Empty[string](), nil
}

// addImportHandler creates a handler for the "add-import" subcommand
// that adds an import path to the current file.
func addImportHandler(
	lsp semanticapi.LSP,
	notify browserapi.Notifications,
) textapi.CommandHandler {
	return &addImportCmd{lsp: lsp, notify: notify}
}

var _ textapi.CommandHandler = (*addImportCmd)(nil)

type addImportCmd struct {
	lsp    semanticapi.LSP
	notify browserapi.Notifications
}

func (h *addImportCmd) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) error {
	if cmd.Resource == nil {
		return fmt.Errorf("no file open; open a file first")
	}
	if len(cmd.Args) == 0 {
		_, _ = h.notify.Notify(browserapi.LevelError,
			"add-import requires a package path argument")
		return nil
	}
	importPath := cmd.Args[0]
	fileURI := lspcmd.URIToLSP(cmd.URI)

	args := map[string]any{
		"URI":        fileURI,
		"ImportPath": importPath,
	}
	argsData, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}

	_, err = h.lsp.ExecuteCommand(ctx, semanticapi.ExecuteCommandParams{
		Command:   "gopls.add_import",
		Arguments: []json.RawMessage{argsData},
	})
	if err != nil {
		return fmt.Errorf("add import %s: %w", importPath, err)
	}

	_, _ = h.notify.Notify(browserapi.LevelInfo, "Added import: %s", importPath)
	return nil
}

func (h *addImportCmd) Complete(
	ctx context.Context, _ string, args []string,
) (iterator.Iterator[string], error) {
	if len(args) > 0 {
		return iterator.Empty[string](), nil
	}
	result, err := h.lsp.ExecuteCommand(ctx, semanticapi.ExecuteCommandParams{
		Command: "gopls.list_known_packages",
	})
	if err != nil {
		return iterator.Empty[string](), nil
	}
	if result == "" {
		return iterator.Empty[string](), nil
	}
	var resp struct {
		Packages []string `json:"Packages"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return iterator.Empty[string](), nil
	}
	return iterator.FromSlice(resp.Packages), nil
}

// findGoModURI walks up from the given file URI to find the nearest
// directory containing a go.mod file. It strips the filename first,
// then walks up parent directories. If no go.mod is found, it falls
// back to the immediate parent directory of the file.
func findGoModURI(fileURI string) string {
	path := strings.TrimPrefix(fileURI, "file://")
	// Strip the filename to get the directory.
	idx := strings.LastIndex(path, "/")
	if idx > 0 {
		path = path[:idx]
	}
	dir := path
	for {
		candidate := dir + "/go.mod"
		if _, err := os.Stat(candidate); err == nil {
			return "file://" + candidate
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir || parent == "" {
			break
		}
		dir = parent
	}
	// Fallback: assume go.mod is in the starting directory.
	return "file://" + path + "/go.mod"
}
