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
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

const goplsResolutionTimeout = 5 * time.Second

var wellKnownGoplsPaths = []string{
	"~/go/bin/gopls",
	"/usr/local/go/bin/gopls",
	"/opt/homebrew/bin/gopls",
	"/usr/local/bin/gopls",
}

func hasGoProjectFiles(_ context.Context, fs workspaceapi.FileSystem) bool {
	for _, name := range []string{"go.mod", "go.sum", "go.work"} {
		if _, err := fs.Stat(name); err == nil {
			return true
		}
	}
	return false
}

func resolveGoplsBinary(
	ctx context.Context,
	fs workspaceapi.FileSystem,
	exec workspaceapi.Executor,
	dataDir string,
) (string, error) {
	candidate := path.Join(dataDir, "bin", "gopls")
	if info, err := fs.Stat(candidate); err == nil && info != nil && !info.IsDir() {
		return candidate, nil
	}

	if bin, ok := probeWellKnown(fs); ok {
		return bin, nil
	}

	ctx, cancel := context.WithTimeout(ctx, goplsResolutionTimeout)
	defer cancel()
	if bin, err := probeShellLookup(ctx, exec); err == nil {
		return bin, nil
	}

	return "", errors.New("gopls binary not found on workspace host")
}

func probeShellLookup(
	ctx context.Context, exec workspaceapi.Executor,
) (string, error) {
	var stdout, stderr bytes.Buffer
	ch := make(chan error, 1)
	cmd := workspaceapi.Cmd{
		Path:    "sh",
		Args:    []string{"-lc", "command -v gopls"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Watcher: workspaceapi.ChanProcessWatcher(ch),
	}
	if _, err := exec.Start(ctx, cmd); err != nil {
		return "", fmt.Errorf("start sh: %w", err)
	}
	select {
	case err := <-ch:
		if err != nil {
			return "", err
		}
	case <-ctx.Done():
		return "", ctx.Err()
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" || !strings.HasPrefix(out, "/") {
		return "", fmt.Errorf("shell probe produced no absolute path: %q", out)
	}
	// command -v may print multiple lines for aliases/functions; take
	// the first absolute path it printed.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/") {
			return line, nil
		}
	}
	return "", fmt.Errorf("shell probe produced no absolute path: %q", out)
}

func probeWellKnown(fs workspaceapi.FileSystem) (string, bool) {
	for _, candidate := range wellKnownGoplsPaths {
		uri, err := fs.URI(candidate)
		if err != nil {
			continue
		}
		full := uri.Path()
		info, err := fs.Stat(full)
		if err != nil || info == nil || info.IsDir() {
			continue
		}
		return full, true
	}
	return "", false
}
