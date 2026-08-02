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
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/llmapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cmd/rune-agent/agent"
	"unstable.build/go-tui/workspace/walkdir"
)

const maxFindResults = 500

type findFilesTool struct {
	fs      workspaceapi.FileSystem
	cwd     workspaceapi.URI
	tracker *FileTracker
	filter  walkdir.Filter
}

type findFilesArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func newFindFiles(wfs workspaceapi.FileSystem, cwd workspaceapi.URI, tracker *FileTracker, filter walkdir.Filter) agent.Tool {
	return &findFilesTool{fs: wfs, cwd: cwd, tracker: tracker, filter: filter}
}

func (t *findFilesTool) NeedsDeterministicOrder() bool { return false }

func (t *findFilesTool) Definition() llmapi.Tool {
	return llmapi.Tool{
		Type: llmapi.ToolTypeFunction,
		Function: llmapi.FunctionDefinition{
			Name: "find_files",
			Description: `Find files by name or path using a regex pattern. Recursively walks
the directory tree starting from path (defaults to workspace root),
skipping .git, node_modules, and vendor directories as well as
gitignored and editor swap files.

Returns matching file paths relative to the workspace root, one per
line, capped at 500 results. The pattern is matched against the full
relative path — use "\.go$" to find all Go files, "Makefile" for
exact names, "test" for any path containing test, or
"cmd/.*main" to match within specific directories. The pattern uses
Go regex (RE2) syntax.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Go regex (RE2) pattern matched against the full relative path.",
					},
					"path": map[string]any{
						"type":        []string{"string", "null"},
						"description": "Optional directory to search in (relative to workspace root or absolute). Defaults to workspace root.",
					},
				},
				"required":             []string{"pattern", "path"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *findFilesTool) Summary(arguments string) string {
	var args findFilesArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	s := `"` + args.Pattern + `"`
	if args.Path != "" {
		s += " in " + summaryPath(t.cwd, args.Path)
	}
	return s
}

func (t *findFilesTool) Execute(ctx context.Context, arguments string) agent.ToolResult {
	var args findFilesArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return agent.ToolResult{Content: fmt.Sprintf("error: invalid arguments: %v", err), IsError: true}
	}

	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return agent.ToolResult{Content: fmt.Sprintf("error: invalid regex: %v", err), IsError: true}
	}

	root := t.cwd.Path()
	if args.Path != "" {
		root = resolvePath(t.cwd, args.Path)
	}
	walkCtx := walkdir.WithContextFilter(boundedWalkdirContext(ctx), t.filter)
	wsRoot := t.cwd.Path()

	paths, err := listToolFiles(walkCtx, t.fs, t.cwd, root, t.filter)
	if err != nil {
		return agent.ToolResult{Content: fmt.Sprintf("error: listing files: %v", err), IsError: true}
	}

	filtered := iterator.Filter(paths, func(path string) bool {
		for _, part := range strings.Split(filepath.Dir(path), string(filepath.Separator)) {
			if skipDirs[part] {
				return false
			}
		}
		return re.MatchString(path)
	})
	defer func() { _ = filtered.Close() }()

	var results []string
	truncated := false
	for {
		path, ok := filtered.Next(ctx)
		if !ok {
			break
		}
		results = append(results, path)
		if len(results) >= maxFindResults {
			truncated = true
			break
		}
	}

	var iterErr error
	if !truncated {
		iterErr = filtered.Err()
	}
	warning, fatal := walkIterErr(ctx, iterErr, len(results))
	if fatal {
		return agent.ToolResult{Content: warning, IsError: true}
	}

	if len(results) == 0 {
		return agent.ToolResult{Content: "no files found"}
	}

	// Track discovered paths so file-reading tools can consume them.
	discoveredPaths := make([]string, len(results))
	for i, relPath := range results {
		discoveredPaths[i] = filepath.Join(wsRoot, relPath)
	}
	t.tracker.TrackDiscovery(ctx, discoveredPaths)

	output := strings.Join(results, "\n")
	if truncated {
		output += fmt.Sprintf("\n\n(results truncated at %d files)", maxFindResults)
	}
	if warning != "" {
		output += "\n\n" + warning
	}
	return agent.ToolResult{Content: output}
}
