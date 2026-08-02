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

const maxSearchResults = 200

type searchTool struct {
	fs      workspaceapi.FileSystem
	cwd     workspaceapi.URI
	tracker *FileTracker
	filter  walkdir.Filter
}

type searchArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include"`
}

func newSearch(wfs workspaceapi.FileSystem, cwd workspaceapi.URI, tracker *FileTracker, filter walkdir.Filter) agent.Tool {
	return &searchTool{fs: wfs, cwd: cwd, tracker: tracker, filter: filter}
}

func (t *searchTool) NeedsDeterministicOrder() bool { return false }

func (t *searchTool) Definition() llmapi.Tool {
	return llmapi.Tool{
		Type: llmapi.ToolTypeFunction,
		Function: llmapi.FunctionDefinition{
			Name: "search_content",
			Description: `Search for a regex pattern in file contents across the workspace.
Recursively walks the directory tree starting from path (defaults to
workspace root), skipping .git, node_modules, and vendor directories as
well as gitignored and editor swap files. Binary files are automatically
excluded.

Returns matching lines formatted as "filepath:line:content", one per
line, capped at 200 matches. The filepath is relative to the workspace
root.

Best for searching string literals, error messages, comments, TODOs,
configuration values, or non-code text. When searching for a function
or type by name, prefer search_symbols or outline_file which are
semantic and more precise. When finding where a symbol is defined, use
find_definition. Use include to restrict to specific file
types (e.g. "*.go"). The pattern uses Go regex (RE2) syntax.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Regex pattern to search for.",
					},
					"path": map[string]any{
						"type":        []string{"string", "null"},
						"description": "Optional directory to search in (relative to workspace root or absolute). Defaults to workspace root.",
					},
					"include": map[string]any{
						"type":        []string{"string", "null"},
						"description": "Optional glob pattern to filter files (e.g. '*.go', '*.ts').",
					},
				},
				"required":             []string{"pattern", "path", "include"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *searchTool) Summary(arguments string) string {
	var args searchArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ""
	}
	s := `"` + args.Pattern + `"`
	if args.Path != "" {
		s += " in " + summaryPath(t.cwd, args.Path)
	}
	return s
}

// skipDirs contains directory names that should be excluded from search.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

func (t *searchTool) Execute(ctx context.Context, arguments string) agent.ToolResult {
	var args searchArgs
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
		if args.Include != "" {
			matched, _ := filepath.Match(args.Include, filepath.Base(path))
			if !matched {
				return false
			}
		}
		return !pathIsBinary(t.fs, filepath.Join(wsRoot, path))
	})

	lines, err := walkdir.ReadLines(walkCtx, t.fs, filtered)
	if err != nil {
		return agent.ToolResult{Content: fmt.Sprintf("error: reading files: %v", err), IsError: true}
	}
	defer func() { _ = lines.Close() }()

	var results []string
	truncated := false
	for {
		line, ok := lines.Next(ctx)
		if !ok {
			break
		}
		content := contentAfterLineNum(line)
		if re.MatchString(content) {
			results = append(results, line)
			if len(results) >= maxSearchResults {
				truncated = true
				break
			}
		}
	}

	var iterErr error
	if !truncated {
		iterErr = lines.Err()
	}
	warning, fatal := walkIterErr(ctx, iterErr, len(results))
	if fatal {
		return agent.ToolResult{Content: warning, IsError: true}
	}

	if len(results) == 0 {
		return agent.ToolResult{Content: "no matches found"}
	}

	// Collect unique absolute paths for discovery tracking.
	seen := make(map[string]struct{})
	var discoveredPaths []string
	for _, line := range results {
		if relPath, _, ok := strings.Cut(line, ":"); ok {
			absPath := filepath.Join(wsRoot, relPath)
			if _, dup := seen[absPath]; !dup {
				seen[absPath] = struct{}{}
				discoveredPaths = append(discoveredPaths, absPath)
			}
		}
	}
	t.tracker.TrackDiscovery(ctx, discoveredPaths)

	output := strings.Join(results, "\n")
	if truncated {
		output += fmt.Sprintf("\n\n(results truncated at %d matches)", maxSearchResults)
	}
	if warning != "" {
		output += "\n\n" + warning
	}
	return agent.ToolResult{Content: output}
}

// contentAfterLineNum extracts the content portion from a "path:linenum:content" line.
func contentAfterLineNum(line string) string {
	// Skip past first colon (end of path).
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return line
	}
	// Skip past second colon (end of line number).
	j := strings.IndexByte(line[i+1:], ':')
	if j < 0 {
		return line
	}
	return line[i+1+j+1:]
}
