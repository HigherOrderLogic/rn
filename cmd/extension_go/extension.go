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
	"os"
	"path/filepath"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
)

// NewExtension returns the Go extension and its metadata.
func NewExtension() (extensionapi.WorkspaceExtension, extensionapi.Metadata) {
	ext := &goExtension{}
	meta := extensionapi.Metadata{
		DeveloperID:    "Unstable Build",
		DeveloperEmail: "it@unstable.build",
		DeveloperKey:   "064D4ABCFA6D9338",
		ExtensionID:    "go",
		ExtensionName:  "Go Language Extension",
		Permissions: extensionapi.NewPermissions(
			extensionapi.PermissionLSP,
			extensionapi.PermissionEditor,
			extensionapi.PermissionCommands,
			extensionapi.PermissionConfig,
			extensionapi.PermissionBrowserWindowManager,
			extensionapi.PermissionNotifications,
			extensionapi.PermissionInterrupt,
			extensionapi.PermissionExecute,
			extensionapi.PermissionFileSystem,
			extensionapi.PermissionBrowserResourceOpener,
			extensionapi.PermissionSyntaxTree,
		),
	}
	return ext, meta
}

type goExtension struct{}

func (e *goExtension) ExtendWorkspace(
	ctx context.Context, w *extensionapi.Workspace, cfg config.Config,
) error {
	lsp := w.LSP(ctx)
	editor := w.Editor(ctx)
	wm := w.WindowManager(ctx)
	notify := w.Notifications(ctx)

	cwd, err := w.FileSystem(ctx).URI(".")
	if err != nil {
		return fmt.Errorf("resolve cwd uri: %w", err)
	}
	// gopls runs on the same host as the workspace files (locally for a
	// local workspace, or on the remote host for a remote one), so rewrite
	// the URI to the file:// scheme expected by the language server.
	rootURI := fmt.Sprintf("file://%s", cwd.Path())

	dbg := readGoplsDebugOptions(cfg)
	goplsBin := resolveGoplsForWorkspace(ctx, w, cfg, notify, cwd.Scheme())
	params, err := goplsInitializeParams(rootURI, dbg, goplsBin)
	if err != nil {
		return fmt.Errorf("build init params: %w", err)
	}
	_, err = lsp.Initialize(ctx, params)
	if err != nil {
		return fmt.Errorf("initialize gopls: %w", err)
	}
	slog.Info("gopls initialized",
		"rpc_trace", dbg.RPCTrace,
		"logfile", dbg.LogFile,
		"debug_addr", dbg.DebugAddr,
		"trace", string(dbg.Trace),
	)

	parser := w.Parser(ctx)
	executor := w.Executor(ctx)
	manual, handler, err := newGoHandler(lsp, editor, wm, notify, parser, executor)
	if err != nil {
		return fmt.Errorf("create handler: %w", err)
	}
	if err := w.RegisterCommand(manual, handler); err != nil {
		return fmt.Errorf("register command: %w", err)
	}
	return nil
}

func readGoplsDebugOptions(cfg config.Config) goplsDebugOptions {
	var opts goplsDebugOptions
	if cfg == nil {
		return opts
	}
	dbg, err := cfg.GetConfig("debug")
	if err != nil || dbg == nil {
		return opts
	}
	if v, err := dbg.GetBool("rpc_trace"); err == nil {
		opts.RPCTrace = v
	}
	if v, err := dbg.GetString("logfile"); err == nil {
		resolved, rerr := resolveLogFile(v)
		if rerr != nil {
			slog.Warn("gopls debug logfile disabled",
				"logfile", v, "error", rerr)
		} else {
			opts.LogFile = resolved
		}
	}
	if v, err := dbg.GetString("addr"); err == nil {
		opts.DebugAddr = v
	}
	if v, err := dbg.GetString("trace"); err == nil {
		switch semanticapi.TraceValue(v) {
		case semanticapi.TraceValueOff,
			semanticapi.TraceValueMessages,
			semanticapi.TraceValueVerbose:
			opts.Trace = semanticapi.TraceValue(v)
		}
	}
	return opts
}

func resolveLogFile(path string) (string, error) {
	if path == "" || path == "auto" {
		return path, nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create logfile dir: %w", err)
	}
	return path, nil
}

func readGoplsLspPath(cfg config.Config, notify browserapi.Notifications) (string, bool) {
	v, err := cfg.GetString("lsp_path")
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return "", false
		}
		if notify != nil {
			_, _ = notify.Notify(browserapi.LevelWarn,
				"extensions.go.config.lsp_path must be a string: %v", err)
		}
		return "", false
	}
	return v, v != ""
}

func resolveGoplsForWorkspace(
	ctx context.Context,
	w *extensionapi.Workspace,
	cfg config.Config,
	notify browserapi.Notifications,
	scheme string,
) string {
	fs := w.FileSystem(ctx)
	if !hasGoProjectFiles(ctx, fs) {
		return ""
	}
	if lspPath, ok := readGoplsLspPath(cfg, notify); ok {
		return lspPath
	}
	bin, err := resolveGoplsBinary(
		ctx, fs, w.Executor(ctx), w.DataDir(ctx))
	if err == nil {
		return bin
	}
	msg := "We could not locate the gopls executable, please set the " +
		"extensions.go.config.lsp_path property in your config and " +
		"reload the workspace"
	if scheme == "file" {
		msg = "We could not locate the gopls executable, please " +
			"reinstall the go extension"
	}
	if notify != nil {
		_, _ = notify.Notify(browserapi.LevelWarn, msg)
	}
	return ""
}
