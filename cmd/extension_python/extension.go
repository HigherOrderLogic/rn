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
	"errors"
	"fmt"
	"log/slog"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// NewExtension returns the Python extension and its metadata.
func NewExtension() (extensionapi.WorkspaceExtension, extensionapi.Metadata) {
	ext := &pyExtension{}
	meta := extensionapi.Metadata{
		DeveloperID:    "Unstable Build",
		DeveloperEmail: "it@unstable.build",
		DeveloperKey:   "064D4ABCFA6D9338",
		ExtensionID:    "python",
		ExtensionName:  "Python Language Extension",
		Permissions: extensionapi.NewPermissions(
			extensionapi.PermissionLSP,
			extensionapi.PermissionEditor,
			extensionapi.PermissionCommands,
			extensionapi.PermissionConfig,
			extensionapi.PermissionNotifications,
			extensionapi.PermissionExecute,
			extensionapi.PermissionFileSystem,
			extensionapi.PermissionSyntaxTree,
		),
	}
	return ext, meta
}

type pyExtension struct{}

func (e *pyExtension) ExtendWorkspace(
	ctx context.Context, w *extensionapi.Workspace, cfg config.Config,
) error {
	return e.extendWorkspaceWith(ctx,
		w.FileSystem(ctx),
		w.Executor(ctx),
		w.Notifications(ctx),
		w.LSP(ctx),
		w.DataDir(ctx),
		cfg,
		w.RegisterREPLCommand,
	)
}

// extendWorkspaceWith performs the workspace bring-up against an explicit
// set of dependencies. ExtendWorkspace supplies them from a real
// *extensionapi.Workspace; the e2e harness supplies real FileSystem and
// Executor with fake Notifications and LSP so the full path runs without
// a live host. The dependencies are passed positionally so the compiler
// flags a missing one at every call site.
func (e *pyExtension) extendWorkspaceWith(
	ctx context.Context,
	fs workspaceapi.FileSystem,
	exec workspaceapi.Executor,
	notify browserapi.Notifications,
	lsp semanticapi.LSP,
	dataDir string,
	cfg config.Config,
	registerREPL func(textapi.CommandManual, textapi.REPLHandler) error,
) error {
	kind := detectProject(ctx, fs)
	if kind == kindNone {
		return nil
	}

	cwd, err := fs.URI(".")
	if err != nil {
		return fmt.Errorf("resolve cwd uri: %w", err)
	}
	// ty/ruff run on the same host as the workspace files (locally for a
	// local workspace, or on the remote host for a remote one), so rewrite
	// the URI to the file:// scheme expected by the language server.
	rootURI := fmt.Sprintf("file://%s", cwd.Path())

	uvBin := resolvePyTool(ctx, fs, exec, dataDir, "uv")
	if err := ensureEnvironment(ctx, uvBin, exec, notify, kind, fs); err != nil {
		_, _ = notify.Notify(browserapi.LevelWarn,
			"Python environment setup failed, continuing without a synced env: %v", err)
		slog.Warn("python env setup failed", "error", err)
	}

	tyBin := resolvePyTool(ctx, fs, exec, dataDir, "ty")
	ruffBin := resolvePyTool(ctx, fs, exec, dataDir, "ruff")
	command := pyCommand(tyBin, "ty", "server")
	alternates := map[string]string{
		"textDocument/formatting":      pyCommand(ruffBin, "ruff", "server"),
		"textDocument/rangeFormatting": pyCommand(ruffBin, "ruff", "server"),
	}
	command, alternates = applyPyConfig(cfg, notify, command, alternates)

	params, err := pyInitializeParams(rootURI, command, alternates)
	if err != nil {
		return fmt.Errorf("build init params: %w", err)
	}
	if _, err := lsp.Initialize(ctx, params); err != nil {
		return fmt.Errorf("initialize python lsp: %w", err)
	}
	slog.Info("python lsp initialized", "command", command)

	manual, handler := newPyHandler(exec, notify, cwd.Path())
	if err := registerREPL(manual, handler); err != nil {
		return fmt.Errorf("register python command: %w", err)
	}
	return nil
}

// applyPyConfig overrides the ty/ruff defaults with the optional
// top-level `command` and `alternate_commands` config keys, each applied
// independently. Overriding `command` without supplying
// `alternate_commands` drops the default ruff alternates, since they
// assume the ty+ruff split; supply `alternate_commands` to keep a
// multi-server setup. Invalid values warn and leave the default in place.
func applyPyConfig(
	cfg config.Config, notify browserapi.Notifications,
	command string, alternates map[string]string,
) (string, map[string]string) {
	if cfg == nil {
		return command, alternates
	}

	override, err := cfg.GetString("command")
	switch {
	case err == nil && override != "":
		command = override
		alternates = nil
	case err != nil && !errors.Is(err, config.ErrNotFound):
		_, _ = notify.Notify(browserapi.LevelWarn,
			"extensions.python.config.command must be a string: %v", err)
	}

	raw, err := cfg.GetMap("alternate_commands")
	if err != nil {
		if !errors.Is(err, config.ErrNotFound) {
			_, _ = notify.Notify(browserapi.LevelWarn,
				"extensions.python.config.alternate_commands must be a map: %v", err)
		}
		return command, alternates
	}
	parsed := make(map[string]string, len(raw))
	for k, v := range raw {
		s, ok := v.(string)
		if !ok {
			_, _ = notify.Notify(browserapi.LevelWarn,
				"extensions.python.config.alternate_commands.%s must be a string", k)
			continue
		}
		parsed[k] = s
	}
	if len(parsed) > 0 {
		alternates = parsed
	}
	return command, alternates
}
