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
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/debug"
)

// projectKind classifies the Python project layout of the workspace
// root, which determines how the uv environment is bootstrapped.
type projectKind int

const (
	kindNone projectKind = iota
	kindProject
	kindRequirements
	kindVenvOnly
	kindScript
)

// requirementsFiles lists the requirements manifests probed in priority
// order; the first that exists drives `uv pip install`.
var requirementsFiles = []string{
	"requirements.txt",
	"requirements.lock",
	"requirements.in",
}

func detectProject(_ context.Context, fs workspaceapi.FileSystem) projectKind {
	if info, err := fs.Stat("pyproject.toml"); err == nil && info != nil && !info.IsDir() {
		return kindProject
	}
	for _, name := range requirementsFiles {
		if info, err := fs.Stat(name); err == nil && info != nil && !info.IsDir() {
			return kindRequirements
		}
	}
	if info, err := fs.Stat(".venv"); err == nil && info != nil && info.IsDir() {
		return kindVenvOnly
	}
	if entries, err := fs.ReadDir("."); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
				return kindScript
			}
		}
	}
	return kindNone
}

func firstRequirementsFile(fs workspaceapi.FileSystem) string {
	for _, name := range requirementsFiles {
		if info, err := fs.Stat(name); err == nil && info != nil && !info.IsDir() {
			return name
		}
	}
	return "requirements.txt"
}

// ensureEnvironment bootstraps the uv-managed environment for the
// detected project kind, reporting step-based progress through a single
// notification. It is best-effort: the caller continues to LSP bring-up
// even on error, since the language server is useful without a synced
// env.
func ensureEnvironment(
	ctx context.Context,
	uvBin string,
	exec workspaceapi.Executor,
	notify browserapi.Notifications,
	kind projectKind,
	fs workspaceapi.FileSystem,
) error {
	notifID, _ := notify.Notify(browserapi.LevelInfo, "Preparing Python environment")

	total, step := int64(4), int64(3)
	installed, err := ensureInterpreter(ctx, uvBin, exec, notify, notifID)
	if err != nil {
		return err
	}
	if !installed {
		total--
		step--
	}

	err = runSyncStep(ctx, uvBin, exec, notify, notifID, kind, fs, step, total)
	if err != nil {
		return err
	}
	return notify.UpdateNotificationProgress(notifID, "Python environment ready", total, total)
}

// ensureInterpreter resolves a Python interpreter, installing one when
// `uv python find` fails so a fresh machine bootstraps on first run. It
// reports the find/install step and returns whether an install ran.
//
// The install passes `--default` so uv links the bare `python` and
// `python3` executables (not just the versioned `python3.X`) into
// UV_PYTHON_BIN_DIR. That dir is first on the Rune PATH (config.yaml
// gui.env), so a terminal `python`/`python3` resolves to the confined
// uv-managed interpreter instead of falling through to a system Python.
func ensureInterpreter(
	ctx context.Context,
	uvBin string,
	exec workspaceapi.Executor,
	notify browserapi.Notifications,
	notifID string,
) (bool, error) {
	_ = notify.UpdateNotificationProgress(notifID, "Finding Python interpreter", 1, 4)
	if err := runUV(ctx, uvBin, exec, "python", "find"); err == nil {
		return false, nil
	}
	_ = notify.UpdateNotificationProgress(notifID, "Installing Python interpreter", 2, 4)
	if err := runUV(ctx, uvBin, exec, "python", "install", "--default"); err != nil {
		return true, err
	}
	return true, nil
}

// runSyncStep installs or verifies the project dependencies for the
// detected kind, reporting a kind-specific message at step/total.
func runSyncStep(
	ctx context.Context,
	uvBin string,
	exec workspaceapi.Executor,
	notify browserapi.Notifications,
	notifID string,
	kind projectKind,
	fs workspaceapi.FileSystem,
	step, total int64,
) error {
	switch kind {
	case kindProject:
		_ = notify.UpdateNotificationProgress(notifID, "Syncing project dependencies", step, total)
		return runUV(ctx, uvBin, exec, "sync")
	case kindRequirements:
		_ = notify.UpdateNotificationProgress(notifID, "Installing requirements", step, total)
		if err := runUV(ctx, uvBin, exec, "venv"); err != nil {
			return err
		}
		return runUV(ctx, uvBin, exec, "pip", "install", "-r", firstRequirementsFile(fs))
	case kindVenvOnly:
		_ = notify.UpdateNotificationProgress(notifID, "Verifying virtual environment", step, total)
		return runUV(ctx, uvBin, exec, "python", "find")
	default:
		return nil
	}
}

// runUV runs `uv <args>` through the workspace executor, draining stderr
// so the process never blocks on a full pipe.
func runUV(
	ctx context.Context,
	uvBin string,
	exec workspaceapi.Executor,
	args ...string,
) error {
	bin := uvBin
	if bin == "" {
		bin = "uv"
	}

	stderrR, stderrW := io.Pipe()
	ch := make(chan error, 1)
	cmd := workspaceapi.Cmd{
		Path:    bin,
		Args:    args,
		Stderr:  stderrW,
		Watcher: workspaceapi.ChanProcessWatcher(ch),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go debug.CapturePanicReport(func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, stderrR)
	})

	if _, err := exec.Start(ctx, cmd); err != nil {
		_ = stderrW.Close()
		wg.Wait()
		return fmt.Errorf("start uv %s: %w", strings.Join(args, " "), err)
	}

	var runErr error
	select {
	case runErr = <-ch:
	case <-ctx.Done():
		runErr = ctx.Err()
	}
	_ = stderrW.Close()
	wg.Wait()

	if runErr != nil {
		return fmt.Errorf("uv %s: %w", strings.Join(args, " "), runErr)
	}
	return nil
}
