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

package sh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/debug"
)

// New returns a repl.CommandHandler that interprets
// shell syntax (pipes, semicolons, variables, etc.)
// using mvdan/sh and delegates actual command execution
// to the underlying handler. The interpreter's working
// directory is seeded from cwd; passing the zero URI leaves
// it at the process working directory.
func New(underlying repl.CommandHandler, cwd workspaceapi.URI) repl.CommandHandler {
	return &commandHandler{underlying: underlying, cwd: cwd}
}

const pipeWidth = 200

type commandHandler struct {
	underlying repl.CommandHandler
	cwd        workspaceapi.URI
}

// HandleCommand parses the command line as shell syntax
// and executes it via mvdan/sh, delegating non-builtin
// commands to the underlying handler. Output is streamed
// via a channel-backed iterator.
func (h *commandHandler) HandleCommand(
	ctx context.Context, cmd repl.Command, pw repl.ProgressWriter,
) (iterator.Iterator[component.Responsive], error) {
	line := reconstructLine(cmd)
	if line == "" {
		return iterator.FromSlice[component.Responsive](nil), nil
	}

	file, err := syntax.NewParser().Parse(
		strings.NewReader(line), "",
	)
	if err != nil {
		return nil, err
	}

	ch := make(chan component.Responsive, 64)
	outW := &lineWriter{ch: ch, ctx: ctx}
	errW := &lineWriter{ch: ch, ctx: ctx}

	opts := []interp.RunnerOption{
		interp.StdIO(nil, outW, errW),
		interp.ExecHandlers(func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
			return h.execMiddleware(pw, next)
		}),
		interp.Interactive(true),
	}
	if dir := h.cwd.Path(); dir != "" {
		opts = append(opts, interp.Dir(dir))
	}
	runner, err := interp.New(opts...)
	if err != nil {
		return nil, err
	}

	var runErr error
	go debug.CapturePanicReport(func() {

		defer close(ch)
		runErr = runner.Run(ctx, file)
		outW.flush()
		errW.flush()

	})

	return iterator.FromFunc(func(ctx context.Context) (component.Responsive, bool, error) {
		select {
		case item, ok := <-ch:
			if !ok {
				if exit, ok := runErr.(interp.ExitStatus); ok && int(exit) != 0 {
					return nil, false, &repl.ExitError{Code: int(exit)}
				}
				if runErr != nil && !errors.Is(runErr, context.Canceled) {
					return nil, false, runErr
				}
				return nil, false, nil
			}
			return item, true, nil
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}, func() error { return nil }), nil
}

// Complete delegates to the underlying handler but as a fallback
// completes dirs or files.
func (h *commandHandler) Complete(
	ctx context.Context, cmd string, args []string,
) (iterator.Iterator[string], error) {
	iter, err := h.underlying.Complete(ctx, cmd, args)
	if err != nil {
		return nil, err
	}
	if args == nil {
		return withFallback(iter, func() []string {
			return pathCompletions(cmd)
		}), nil
	}
	return withFallback(iter, func() []string {
		if _, err := exec.LookPath(cmd); err != nil {
			return nil
		}
		var prefix string
		if len(args) > 0 {
			prefix = args[len(args)-1]
		}
		return fileCompletions(prefix)
	}), nil
}

func withFallback(
	primary iterator.Iterator[string], fallback func() []string,
) iterator.Iterator[string] {
	it := &fallbackIter{primary: primary, fallback: fallback}
	return iterator.FromFunc(it.next, primary.Close)
}

type fallbackIter struct {
	primary  iterator.Iterator[string]
	fallback func() []string

	decided  bool
	usingFB  bool
	fbValues []string
}

func (f *fallbackIter) next(ctx context.Context) (string, bool, error) {
	if !f.decided {
		f.decided = true
		if v, ok := f.primary.Next(ctx); ok {
			return v, true, nil
		}
		if err := f.primary.Err(); err != nil {
			return "", false, err
		}
		f.usingFB = true
		f.fbValues = f.fallback()
	}
	if !f.usingFB {
		v, ok := f.primary.Next(ctx)
		if !ok {
			return "", false, f.primary.Err()
		}
		return v, true, nil
	}
	if len(f.fbValues) == 0 {
		return "", false, nil
	}
	v := f.fbValues[0]
	f.fbValues = f.fbValues[1:]
	return v, true, nil
}

func (h *commandHandler) execMiddleware(
	pw repl.ProgressWriter,
	next interp.ExecHandlerFunc,
) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := interp.HandlerCtx(ctx)
		cmd := repl.Command{
			Name: args[0],
			Args: args[1:],
		}
		iter, err := h.underlying.HandleCommand(
			ctx, cmd, pw,
		)
		if errors.Is(err, repl.ErrNotFound) {
			return next(ctx, args)
		}
		if err != nil {
			_, _ = fmt.Fprintln(hc.Stderr, err.Error())
			return interp.ExitStatus(1)
		}
		defer func() { _ = iter.Close() }()

		// When stdout is the REPL's own lineWriter (i.e. the
		// command is not piped into another command or redirected
		// to a file), forward Responsives as-is so the terminal
		// layer can Resize them to its actual width. Flattening
		// to pipeWidth-wide plain text would break components
		// like markdown tables that depend on responsive layout.
		lw, direct := hc.Stdout.(*lineWriter)
		first := true
		for {
			item, ok := iter.Next(ctx)
			if !ok {
				break
			}
			if direct {
				lw.sendResponsive(item)
				continue
			}
			if !first {
				_, _ = fmt.Fprintln(hc.Stdout)
			}
			first = false
			_, _ = fmt.Fprint(hc.Stdout, responsiveToText(item, pipeWidth))
		}
		if err := iter.Err(); err != nil {
			_, _ = fmt.Fprintln(hc.Stderr, err.Error())
			return interp.ExitStatus(1)
		}
		return nil
	}
}

func reconstructLine(cmd repl.Command) string {
	if cmd.Name == "" {
		return ""
	}
	if len(cmd.Args) == 0 {
		return cmd.Name
	}
	return cmd.Name + " " + strings.Join(cmd.Args, " ")
}

func responsiveToText(r component.Responsive, width int) string {
	height := r.Height(width)
	if height <= 0 {
		return ""
	}
	w := term.NewStringWriter(width, height)
	r.Resize(width, height)
	r.Draw(w)
	_ = w.Flush()
	return trimTrailingWhitespace(w.String())
}

func trimTrailingWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	// Remove trailing empty lines.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// pathCompletions scans $PATH directories for executable
// names that start with prefix, deduplicates and returns
// them sorted.
func pathCompletions(prefix string) []string {
	if prefix == "" {
		return nil
	}
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}
	seen := make(map[string]bool)
	for _, dir := range filepath.SplitList(pathEnv) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if seen[name] {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0111 == 0 {
				continue
			}
			seen[name] = true
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// fileCompletions lists directory entries matching the
// given prefix, appending "/" for directories. Hidden
// files are only included when the prefix starts with ".".
func fileCompletions(prefix string) []string {
	dir, base := filepath.Split(prefix)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		// Skip hidden files unless the prefix starts with ".".
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		path := name
		if dir != "." {
			path = filepath.Join(dir, name)
		}
		if e.IsDir() {
			path += "/"
		}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
