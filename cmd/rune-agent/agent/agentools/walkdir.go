// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2024-2026 Unstable Build, All Rights Reserved.
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

package agentools

import (
	"context"
	"fmt"
	"runtime"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace/walkdir"
)

const maxAgentWalkdirWorkers = 4

func boundedWalkdirContext(ctx context.Context) context.Context {
	workers := max(min(runtime.NumCPU(), maxAgentWalkdirWorkers), 1)
	return walkdir.ContextWithWorkerCount(ctx, workers)
}

// listToolFiles lists the files a search-style tool should traverse
// under root, relative to the workspace root cwd. walkdir.ListFiles
// replaces a file path with its nearest parent directory, which path
// completion relies on but which would make a tool search a whole tree
// the caller never named.
func listToolFiles(
	ctx context.Context, fs workspaceapi.FileSystem, cwd workspaceapi.URI,
	root string, filter walkdir.Filter,
) (iterator.Iterator[string], error) {
	info, err := fs.Stat(root)
	if err != nil || !info.Mode().IsRegular() {
		return walkdir.ListFiles(ctx, fs, root)
	}
	rootURI, err := fs.URI(root)
	if err != nil {
		return nil, fmt.Errorf("URI: %v", err)
	}
	rel := workspaceapi.RelPath(cwd, rootURI)
	if filter != nil && filter.MatchRelPath(rel, false) {
		return iterator.Empty[string](), nil
	}
	return iterator.FromSlice([]string{rel}), nil
}

// walkIterErr classifies an error surfaced by a walkdir iterator.
// Cancellation is always fatal: results collected before the abort must
// not render as a successful response. Per-file and per-directory
// failures fail the call only when nothing was found, otherwise they
// are reported next to the partial output.
func walkIterErr(ctx context.Context, err error, results int) (msg string, fatal bool) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Sprintf("error: canceled before completion: %v", ctxErr), true
	}
	if err == nil {
		return "", false
	}
	if results == 0 {
		return fmt.Sprintf("error: %v", err), true
	}
	return fmt.Sprintf("(warning: some paths could not be read: %v)", err), false
}
