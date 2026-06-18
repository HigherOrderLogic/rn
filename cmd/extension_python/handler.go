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
	"fmt"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"unstable.build/go-tui/component/markdown"
)

// pyCommandName is the top-level REPL command exposed by this extension.
const pyCommandName = "python"

// uvRoute maps a `python` subcommand to the uv argv prefix it expands
// to. The user-supplied trailing args are appended verbatim. A nil
// prefix means the subcommand name is itself the uv subcommand.
var uvRoutes = map[string][]string{
	// Python interpreter management.
	"install":   {"python", "install"},
	"list":      {"python", "list"},
	"find":      {"python", "find"},
	"pin":       {"python", "pin"},
	"uninstall": {"python", "uninstall"},
	// Projects.
	"init":   {"init"},
	"add":    {"add"},
	"remove": {"remove"},
	"sync":   {"sync"},
	"lock":   {"lock"},
	"tree":   {"tree"},
	"build":  {"build"},
	// Run.
	"run": {"run"},
	// Tools.
	"tool": {"tool"},
	// pip interface.
	"pip":  {"pip"},
	"venv": {"venv"},
	// Utility.
	"cache": {"cache"},
	"self":  {"self"},
}

// pySubcommandNames is the deterministic completion order at depth 0.
var pySubcommandNames = []string{
	"add", "build", "cache", "find", "init", "install", "list", "lock",
	"pin", "pip", "remove", "run", "self", "sync", "tool", "tree",
	"uninstall", "venv",
}

var pyManual = textapi.CommandManual{
	Name:     pyCommandName,
	Summary:  "Manage the Python toolchain through uv.",
	Synopsis: "<command> [<args>]",
	Commands: []textapi.CommandManual{
		{Name: "install", Summary: "Install a Python interpreter version.", Synopsis: "[<version>]"},
		{Name: "list", Summary: "List available and installed Python versions."},
		{Name: "find", Summary: "Find an installed Python interpreter."},
		{Name: "pin", Summary: "Pin the project to a Python version.", Synopsis: "<version>"},
		{Name: "uninstall", Summary: "Uninstall a Python interpreter version.", Synopsis: "<version>"},
		{Name: "init", Summary: "Initialize a new uv project.", Synopsis: "[<path>]"},
		{Name: "add", Summary: "Add dependencies to the project.", Synopsis: "<package>..."},
		{Name: "remove", Summary: "Remove dependencies from the project.", Synopsis: "<package>..."},
		{Name: "sync", Summary: "Sync the environment with the project lockfile."},
		{Name: "lock", Summary: "Update the project lockfile."},
		{Name: "tree", Summary: "Display the project dependency tree."},
		{Name: "build", Summary: "Build the project into distributable archives."},
		{Name: "run", Summary: "Run a command in the project environment.", Synopsis: "<command> [<args>]"},
		{Name: "tool", Summary: "Run and manage tools provided by Python packages.", Synopsis: "<args>"},
		{Name: "pip", Summary: "Manage packages with a pip-compatible interface.", Synopsis: "<args>"},
		{Name: "venv", Summary: "Create a virtual environment.", Synopsis: "[<path>]"},
		{Name: "cache", Summary: "Manage the uv cache.", Synopsis: "<args>"},
		{Name: "self", Summary: "Manage the uv executable.", Synopsis: "<args>"},
	},
}

var uvNestedSubcommands = map[string][]string{
	"tool":  {"dir", "install", "list", "run", "uninstall", "upgrade", "update-shell"},
	"pip":   {"check", "compile", "freeze", "install", "list", "show", "sync", "tree", "uninstall"},
	"cache": {"clean", "dir", "prune", "size"},
	"self":  {"update", "version"},
}

type pyHandler struct {
	exec   workspaceapi.Executor
	notify browserapi.Notifications
	cwd    string
}

var _ textapi.REPLHandler = (*pyHandler)(nil)

// newPyHandler builds the `python` REPL command handler and its manual.
func newPyHandler(
	exec workspaceapi.Executor, notify browserapi.Notifications, cwd string,
) (textapi.CommandManual, textapi.REPLHandler) {
	return pyManual, &pyHandler{exec: exec, notify: notify, cwd: cwd}
}

// HandleCommand routes the first arg to the matching uv subcommand.
func (h *pyHandler) HandleCommand(
	ctx context.Context, cmd repl.Command, _ repl.ProgressWriter,
) (iterator.Iterator[component.Responsive], error) {
	if len(cmd.Args) == 0 {
		return markdownOutput(usageMarkdown(pyManual)), nil
	}
	sub := cmd.Args[0]
	if sub == "help" {
		return h.Help(ctx, cmd.Args[1:])
	}
	prefix, ok := uvRoutes[sub]
	if !ok {
		return nil, fmt.Errorf("unknown command: %s", sub)
	}
	args := append(append([]string{}, prefix...), cmd.Args[1:]...)
	out, err := h.runUVCapture(ctx, args...)
	if err != nil {
		return nil, err
	}
	return markdownOutput(fmt.Sprintf("```\n%s\n```", out)), nil
}

// Complete offers the `python` subcommands at depth 0 and the nested uv
// subcommands of a command group (tool, pip, cache, self) at depth 1.
// Deeper positions defer to uv at runtime and return no completions.
func (h *pyHandler) Complete(
	_ context.Context, _ string, args []string,
) (iterator.Iterator[string], error) {
	if len(args) <= 1 {
		filter := ""
		if len(args) == 1 {
			filter = args[0]
		}
		return iterator.FromSlice(filterNames(pySubcommandNames, filter)), nil
	}
	if len(args) == 2 {
		if nested, ok := uvNestedSubcommands[args[0]]; ok {
			return iterator.FromSlice(filterNames(nested, args[1])), nil
		}
	}
	return iterator.FromSlice[string](nil), nil
}

// Help renders the manual for the command tree, descending into
// subcommands named in args.
func (h *pyHandler) Help(
	_ context.Context, args []string,
) (iterator.Iterator[component.Responsive], error) {
	man := pyManual
	for _, a := range args {
		sub, ok := findSubcommand(man, a)
		if !ok {
			break
		}
		man = sub
	}
	return markdownOutput(usageMarkdown(man)), nil
}

func (h *pyHandler) runUVCapture(
	ctx context.Context, args ...string,
) (string, error) {
	var stdout, stderr bytes.Buffer
	ch := make(chan error, 1)
	cmd := workspaceapi.Cmd{
		Path:    "uv",
		Args:    args,
		Dir:     h.cwd,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Watcher: workspaceapi.ChanProcessWatcher(ch),
	}
	if _, err := h.exec.Start(ctx, cmd); err != nil {
		return "", fmt.Errorf("start uv %s: %w", strings.Join(args, " "), err)
	}
	var runErr error
	select {
	case runErr = <-ch:
	case <-ctx.Done():
		runErr = ctx.Err()
	}

	out := strings.TrimRight(stdout.String(), "\n")
	errOut := strings.TrimRight(stderr.String(), "\n")
	if runErr != nil {
		combined := strings.TrimSpace(out + "\n" + errOut)
		return "", fmt.Errorf("uv %s: %w\n%s", strings.Join(args, " "), runErr, combined)
	}
	if out == "" {
		out = errOut
	}
	return out, nil
}

// markdownOutput falls back to a plain responsive string when the
// markdown parser rejects the content.
func markdownOutput(content string) iterator.Iterator[component.Responsive] {
	md, err := markdown.New(content)
	if err != nil {
		r := component.NewResponsiveString(content, component.StringResponsiveConfig{})
		return iterator.FromSlice([]component.Responsive{r})
	}
	return iterator.FromSlice([]component.Responsive{md})
}

func filterNames(names []string, prefix string) []string {
	if prefix == "" {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

func findSubcommand(man textapi.CommandManual, name string) (textapi.CommandManual, bool) {
	for _, c := range man.Commands {
		if c.Name == name {
			return c, true
		}
	}
	return textapi.CommandManual{}, false
}

func usageMarkdown(man textapi.CommandManual) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## `%s`\n\n", man.Name)
	if man.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", man.Summary)
	}
	if man.Synopsis != "" {
		fmt.Fprintf(&b, "**Usage:** `%s %s`\n\n", man.Name, man.Synopsis)
	}
	if len(man.Commands) > 0 {
		b.WriteString("### Subcommands\n\n")
		for _, c := range man.Commands {
			invocation := c.Name
			if c.Synopsis != "" {
				invocation = fmt.Sprintf("%s %s", c.Name, c.Synopsis)
			}
			fmt.Fprintf(&b, "- `%s`\n  %s\n", invocation, c.Summary)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
