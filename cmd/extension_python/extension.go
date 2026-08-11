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
	"unstable.build/go-tui/cmd/extension_python/pyshim"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension/langext"
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
		w.Editor(ctx),
		w,
		cfg,
		w.DataDir(ctx),
		w.RegisterREPLCommand,
	)
}

// extendWorkspaceWith wires Python project discovery to per-root language
// server bring-up. It registers the REPL command once for the workspace,
// subscribes for opened .py files so a server is initialized rooted at
// each file's nearest project, and eagerly initializes the workspace-root
// project when one is present. The dependencies are passed positionally
// so the compiler flags a missing one at every call site.
func (e *pyExtension) extendWorkspaceWith(
	ctx context.Context,
	fs workspaceapi.FileSystem,
	exec workspaceapi.Executor,
	notify browserapi.Notifications,
	lsp semanticapi.LSP,
	editor textapi.Editor,
	inst installer,
	cfg config.Config,
	dataDir string,
	registerREPL func(textapi.CommandManual, textapi.REPLHandler) error,
) error {
	// The REPL command's cwd is always the workspace root, independent of
	// any nested project, so register it once up front regardless of
	// whether a project root is ever discovered.
	cwd, err := fs.URI(".")
	if err != nil {
		return fmt.Errorf("resolve cwd uri: %w", err)
	}
	manual, handler := newPyHandler(exec, notify, cwd.Path())
	if err := registerREPL(manual, handler); err != nil {
		return fmt.Errorf("register python command: %w", err)
	}

	init := langext.NewInitializer(ctx, fs, editor, langext.ProjectConfig{
		LanguageID:  "python",
		Markers:     pyMarkers,
		FileMatch:   isPythonFile,
		WatchEvents: pyWatchEvents(cfg, notify),
		InitRoot: func(ctx context.Context, root langext.Root) error {
			return initializeProjectRoot(ctx, fs, exec, notify, lsp, inst, cfg, dataDir, root)
		},
	})
	if err := init.Start(); err != nil {
		return fmt.Errorf("subscribe python open events: %w", err)
	}

	// Preserve the eager workspace-root behavior: if the workspace root
	// is itself a Python project, bring it up immediately rather than
	// waiting for the first open. Discovery still drives nested projects.
	if detectProjectAt(ctx, fs, ".") != kindNone {
		root := langext.Root{Dir: cwd.Path(), URI: fmt.Sprintf("file://%s", cwd.Path())}
		if err := init.InitializeAt(ctx, root); err != nil {
			return err
		}
	}
	return nil
}

// initializeProjectRoot performs the language-specific bring-up for a
// discovered project root: it bootstraps the uv environment rooted there
// (best-effort), installs the venv-aware python shims, prewarms the
// debugpy adapter env, resolves ty/ruff, applies config overrides, and
// initializes the language server with the nested root URI.
func initializeProjectRoot(
	ctx context.Context,
	fs workspaceapi.FileSystem,
	exec workspaceapi.Executor,
	notify browserapi.Notifications,
	lsp semanticapi.LSP,
	inst installer,
	cfg config.Config,
	dataDir string,
	root langext.Root,
) error {
	kind := detectProjectAt(ctx, fs, root.Dir)

	uvBin := resolvePyTool(ctx, fs, exec, inst, "uv")
	if err := ensureEnvironment(ctx, uvBin, exec, notify, kind, fs, root.Dir, dataDir); err != nil {
		_, _ = notify.Notify(browserapi.LevelWarn,
			"Python environment setup failed, continuing without a synced env: %v", err)
		slog.Warn("python env setup failed", "root", root.Dir, "error", err)
	}

	if dataDir != "" {
		if err := pyshim.Write(fs, dataDir); err != nil {
			slog.Warn("python shim install failed", "dataDir", dataDir, "error", err)
		}
	}

	if pin := debugpyPin(cfg, notify); pin != "" {
		uvxBin := resolvePyTool(ctx, fs, exec, inst, "uvx")
		go debug.CapturePanicReport(func() {
			prewarmDebugpy(ctx, uvxBin, exec, root.Dir, pin)
		})
	}

	tyBin := resolvePyTool(ctx, fs, exec, inst, "ty")
	ruffBin := resolvePyTool(ctx, fs, exec, inst, "ruff")
	logLevel := pyLogLevel(cfg, notify)
	command := pyCommand(tyBin, "ty", "server")
	alternates := map[string]string{
		"textDocument/formatting":      pyRuffCommand(ruffBin, logLevel),
		"textDocument/rangeFormatting": pyRuffCommand(ruffBin, logLevel),
	}
	command, alternates = applyPyConfig(cfg, notify, command, alternates)

	diagnosticMode := pyDiagnosticMode(cfg, notify)
	params, err := pyInitializeParams(
		root.URI, command, alternates, diagnosticMode, logLevel)
	if err != nil {
		return fmt.Errorf("build init params: %w", err)
	}
	if _, err := lsp.Initialize(ctx, params); err != nil {
		return fmt.Errorf("initialize python lsp: %w", err)
	}
	slog.Info("python lsp initialized", "root", root.Dir, "command", command)
	return nil
}

func pyLogLevel(cfg config.Config, notify browserapi.Notifications) string {
	if cfg == nil {
		return ""
	}
	debug, err := cfg.GetConfig("debug")
	if err != nil || debug == nil {
		return ""
	}
	level, err := debug.GetString("log_level")
	if err != nil {
		return ""
	}
	switch level {
	case "trace", "debug", "info", "warn", "error":
		return level
	default:
		if notify != nil {
			_, _ = notify.Notify(browserapi.LevelWarn,
				"extensions.python.config.debug.log_level must be one of "+
					"\"trace\", \"debug\", \"info\", \"warn\", \"error\"; got %q", level)
		}
		return ""
	}
}

// debugpyPin reads the optional `debugpy` config key: the version pin
// used to prewarm the debug adapter's uvx environment right after the
// project env syncs, so the first debug launch works offline. Unset
// skips the prewarm; the pin must match the one in the package's
// debugger.python.command so the prewarmed env is the one the adapter
// resolves.
func debugpyPin(cfg config.Config, notify browserapi.Notifications) string {
	if cfg == nil {
		return ""
	}
	pin, err := cfg.GetString("debugpy")
	switch {
	case errors.Is(err, config.ErrNotFound):
		return ""
	case err != nil:
		_, _ = notify.Notify(browserapi.LevelWarn,
			"extensions.python.config.debugpy must be a string: %v", err)
		return ""
	}
	return pin
}

// prewarmDebugpy resolves the pinned debugpy into uvx's cached env by
// running a no-op python through it. Best-effort: a failure only means
// the first debug launch pays the resolution cost (or fails offline).
func prewarmDebugpy(
	ctx context.Context,
	uvxBin string,
	exec workspaceapi.Executor,
	dir, pin string,
) {
	if uvxBin == "" {
		uvxBin = "uvx"
	}
	if err := runUV(ctx, uvxBin, exec, dir, "--from", "debugpy=="+pin, "python", "-c", ""); err != nil {
		slog.Warn("debugpy prewarm failed", "pin", pin, "error", err)
	}
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

// pyWatchEvents returns the editor events that drive project discovery,
// defaulting to opens plus out-of-band changes and creates. Setting the
// optional `watch_events` config key to false restores open-only
// behavior, an escape hatch for pathological monorepos.
func pyWatchEvents(cfg config.Config, notify browserapi.Notifications) []textapi.EventType {
	openOnly := []textapi.EventType{textapi.EventTypeOpen}
	onChange := []textapi.EventType{
		textapi.EventTypeOpen, textapi.EventTypeChange, textapi.EventTypeCreate,
	}
	enabled, err := cfg.GetBool("watch_events")
	switch {
	case errors.Is(err, config.ErrNotFound):
		return onChange
	case err != nil:
		_, _ = notify.Notify(browserapi.LevelWarn,
			"extensions.python.config.watch_events must be a bool: %v", err)
		return onChange
	case !enabled:
		return openOnly
	default:
		return onChange
	}
}

// pyDiagnosticMode reads the optional `diagnostic_mode` config key that
// controls ty's diagnostic scope. It is opt-in: when unset, ty keeps
// its default "openFilesOnly" scope. Only ty's documented values are
// accepted ("off", "openFilesOnly", "workspace"); an unknown value
// warns and is ignored. "workspace" makes ty type-check the whole
// project and answer workspace/diagnostic pulls for unopened files, at
// the cost of a full-project scan per pull, so it is left to the user
// to enable per project.
func pyDiagnosticMode(cfg config.Config, notify browserapi.Notifications) string {
	if cfg == nil {
		return ""
	}
	mode, err := cfg.GetString("diagnostic_mode")
	switch {
	case errors.Is(err, config.ErrNotFound):
		return ""
	case err != nil:
		_, _ = notify.Notify(browserapi.LevelWarn,
			"extensions.python.config.diagnostic_mode must be a string: %v", err)
		return ""
	}
	switch mode {
	case "", "off", "openFilesOnly", "workspace":
		return mode
	default:
		_, _ = notify.Notify(browserapi.LevelWarn,
			"extensions.python.config.diagnostic_mode must be one of "+
				"\"off\", \"openFilesOnly\", \"workspace\"; got %q", mode)
		return ""
	}
}
