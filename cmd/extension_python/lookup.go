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
	"path"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// resolvePyTool locates a Python toolchain binary (ty, ruff, uv) at the
// per-workspace `<dataDir>/bin/<name>` install location, returning its
// path or "" when absent. Unlike the Go extension, it does not probe
// well-known dirs or the shell: Python tooling lacks the cross-version
// compatibility guarantees that make a loosely-resolved binary safe, so
// the caller falls back to the bare command name and lets the executor
// resolve it through $PATH.
func resolvePyTool(
	_ context.Context,
	fs workspaceapi.FileSystem,
	_ workspaceapi.Executor,
	dataDir, name string,
) string {
	candidate := path.Join(dataDir, "bin", name)
	if info, err := fs.Stat(candidate); err == nil && info != nil && !info.IsDir() {
		return candidate
	}
	return ""
}
