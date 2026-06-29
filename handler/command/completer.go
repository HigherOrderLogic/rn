// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
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

package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"mvdan.cc/sh/v3/shell"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/walkdir"
)

// Completer abstracts the ability to complete command arguments.
type Completer interface {
	// Complete takes the given command and arguments and returns an iterator
	// over an expanded list of options for the last argument. It also returns
	// an expanded version of the last argument, if there is one, or an empty
	// string if the last argument could/should not be automatically expanded.
	Complete(ctx context.Context, args []string) (
		iterator.Iterator[string], string, error,
	)
}

// FuncCompleter returns a Completer that calls fn every time Complete is called.
func FuncCompleter(
	fn func(context.Context, []string) (iterator.Iterator[string], string, error),
) Completer {
	return fnCompleter{fn: fn}
}

// NopCompleter returns a Completer that does nothing.
func NopCompleter() Completer {
	return fnCompleter{}
}

type fnCompleter struct {
	fn func(context.Context, []string) (iterator.Iterator[string], string, error)
}

func (d fnCompleter) Complete(
	ctx context.Context, args []string,
) (iterator.Iterator[string], string, error) {
	if d.fn != nil {
		return d.fn(ctx, args)
	}
	return iterator.Empty[string](), "", nil
}

// FilePathCompleter returns a files path completer with the given directory reader.
func FilePathCompleter(reader walkdir.Reader) Completer {
	return walkDirCompleter(reader, false)
}

// DirsCompleter returns a files path completer with the given directory reader.
func DirsCompleter(reader walkdir.Reader) Completer {
	return walkDirCompleter(reader, true)
}

func walkDirCompleter(reader walkdir.Reader, dirOnly bool) Completer {
	traverse := func(
		ctx context.Context, w walkdir.Reader, root string,
	) (iterator.Iterator[string], error) {
		// Skipping ~/Library (macOS) avoids the system "access data from
		// other apps" prompt. In dirOnly mode dot directories are pruned
		// at the source too: the post-iter filter would drop them anyway,
		// but descending into e.g. .git first floods the system with
		// ReadDir/Open syscalls during completion.
		filter := vctrl.ProtectedDirMatcher(w)
		if dirOnly {
			filter = vctrl.AnyMatcher(filter, vctrl.HiddenBaseMatcher())
		}
		ctx = walkdir.WithContextFilter(ctx, filter)
		fn := walkdir.ListFiles
		if dirOnly {
			fn = walkdir.ListDirs
		}
		it, err := fn(ctx, w, root)
		if err != nil {
			return nil, err
		}
		it = iterator.Filter(it, func(val string) bool {
			return !strings.HasSuffix(val, workspace.SwapFileExtensionName)
		})
		if dirOnly {
			it = iterator.Filter(it, func(val string) bool {
				return !strings.HasPrefix(val, ".")
			})
		}
		return it, nil
	}
	return FuncCompleter(func(
		ctx context.Context, args []string,
	) (iterator.Iterator[string], string, error) {
		if len(args) == 0 || args[len(args)-1] == "" {
			it, err := traverse(ctx, reader, ".")
			if err != nil {
				return nil, "", err
			}
			return iterator.Map(it, ShellQuote), "", nil
		}

		var modifiedLast string
		last := UnquoteToken(args[len(args)-1])

		// take ~ as the home of the user using the editor.
		// rather than the home directory of the user at the workspace.
		// do not always expand without making sure that we are not
		// erasing trailing /, which prevents user from editing files
		// in folders.
		var err error
		if strings.Contains(last, "~") {
			last, err = workspaceapi.ExpandPath(last, user.Current,
				func() (string, error) {
					// do not really expand to cwd,
					// let parseURIOrWorkspaceURI take care of that
					return ".", nil
				})
			if err != nil {
				return nil, "", fmt.Errorf("expand path: %v", err)
			}
			modifiedLast = last
		}

		uri, err := parseURIOrWorkspaceURI(reader, last)
		if err != nil {
			return nil, "", err
		}

		cwd, err := reader.URI(".")
		if err != nil {
			return nil, "", err
		}

		// if filter is absolute path and it happens to be the current working
		// directory of the given reader, the iterator returned by walkdir.ListFiles
		// will return paths relative to it, but filter will be absolute, machting
		// no results.
		needsExpand := workspaceapi.HasPrefix(uri, cwd) && filepath.IsAbs(last)

		it, err := traverse(ctx, reader, uri.Path())
		if err != nil {
			return nil, "", err
		}
		if needsExpand {
			it = iterator.Map(it, func(val string) string {
				return workspaceapi.Join(cwd, val).Path()
			})
		}
		return iterator.Map(it, ShellQuote), modifiedLast, nil
	})
}

// OutputLinesCompleter returns a files path completer with the given
// directory reader. lookup resolves any $VAR references inside
// cmdAndArgs; pass nil to use os.Getenv alone. Callers that want to
// expand Rune-managed variables on top of os.Getenv should pass the
// function returned by text/cmdenv.Lookup(src).
func OutputLinesCompleter(
	w schemeapi.Executor, cmdAndArgs []string, lookup func(string) string,
) Completer {
	return FuncCompleter(func(
		ctx context.Context, args []string,
	) (iterator.Iterator[string], string, error) {
		if len(cmdAndArgs) == 0 {
			return nil, "", errors.New("expected at least one argument with the name " +
				"of the executable to run")
		}
		ch := make(chan error)
		cmdAndArgsStr := strings.Join(cmdAndArgs, " ")
		if lookup == nil {
			lookup = os.Getenv
		}
		cmdAndArgs, err := shell.Fields(cmdAndArgsStr, lookup)
		if err != nil {
			return nil, "", fmt.Errorf("expand shell arguments: %w", err)
		}
		cmd := workspaceapi.Cmd{
			Path:    cmdAndArgs[0],
			Watcher: workspaceapi.ChanProcessWatcher(ch),
		}
		if len(cmdAndArgs) > 1 {
			cmd.Args = cmdAndArgs[1:]
		}

		pr, pw := io.Pipe()
		cmd.Stdout = pw
		scanner := bufio.NewScanner(pr)
		pid, err := w.StartCommand(ctx, cmd)
		if err != nil {
			return nil, "", err
		}

		go debug.CapturePanicReport(func() {

			select {
			case <-ctx.Done():
			case <-ch:
			}
			_ = pr.Close()

		})
		return iterator.FromFunc(func(ctx context.Context) (string, bool, error) {
			if scanner.Scan() {
				return scanner.Text(), true, nil
			}
			if err := scanner.Err(); err != nil {
				return "", false, err
			}
			return "", false, nil // EOF
		}, func() (ret error) {
			if err := w.Signal(pid, syscall.SIGINT); err != nil {
				ret = errors.Join(ret, err)
			}
			if err := pr.Close(); err != nil {
				ret = errors.Join(ret, err)
			}
			return ret
		}), "", nil
	})
}

func parseURIOrWorkspaceURI(reader walkdir.Reader, path string) (workspaceapi.URI, error) {
	uri, err := workspaceapi.ParseURI(path)
	if err != nil {
		uri, err = reader.URI(path)
	}
	return uri, err
}

// HistoryAccessor exposes the persisted command history as a stream of
// raw command-and-args lines. Implementations return the iterator and
// true when any history is available, or a nil iterator and false when
// no history exists. The args parameter is the same one passed to a
// Completer's Complete; implementations are free to ignore it (the
// canonical implementation does — filtering happens in HistoryCompleter,
// see its docs).
type HistoryAccessor interface {
	HistoryIterator(ctx context.Context, args []string) (iterator.Iterator[string], bool)
}

// HistoryCompleter returns a Completer backed by the given HistoryAccessor.
// Used by alias chains to expose a per-alias argument history as a regular
// Completer that can be combined with other completers via MultiCompleter.
//
// HistoryCompleter narrows the raw history stream to entries that begin
// with the same command prefix that the user is currently typing — the
// `args` slice received by Complete is "<alias-name>", "<arg1>", …,
// "<partial-last-arg>". Only entries whose first len(args)-1 tokens
// match are kept, and the matching prefix is stripped from each entry
// before being yielded. This mirrors the implicit history fallback the
// command Prompt has always applied (see commandArgsHistoryIterator)
// so the `{history}` placeholder behaves consistently with it.
//
// Panics when acc is nil — the caller must supply a working accessor.
func HistoryCompleter(acc HistoryAccessor) Completer {
	if acc == nil {
		panic("command.HistoryCompleter: nil HistoryAccessor")
	}
	return FuncCompleter(func(
		ctx context.Context, args []string,
	) (iterator.Iterator[string], string, error) {
		it, ok := acc.HistoryIterator(ctx, args)
		if !ok || it == nil {
			return iterator.Empty[string](), "", nil
		}
		// "<cmd> <arg1> … <partialLastArg>" — keep entries that start
		// with the leading "<cmd> <arg1> … " (everything except the
		// trailing partial last arg) and strip that prefix on emit.
		prefixTokens := args
		if len(prefixTokens) > 0 {
			prefixTokens = prefixTokens[:len(prefixTokens)-1]
		}
		prefix := strings.Join(prefixTokens, " ")
		if prefix != "" {
			prefix += " "
		}
		filtered := iterator.Filter(it, func(entry string) bool {
			if prefix == "" {
				return entry != ""
			}
			return strings.HasPrefix(entry, prefix)
		})
		stripped := iterator.Map(filtered, func(entry string) string {
			return strings.TrimPrefix(entry, prefix)
		})
		return iterator.Filter(stripped, func(entry string) bool {
			return entry != ""
		}), "", nil
	})
}

// MultiCompleter returns a Completer that yields the concatenation of
// every child's results, in order. The returned iterator is a pure
// projection — no values are ever buffered on the calling goroutine,
// every Next call forwards directly to the underlying child iterator.
// Duplicate values across children are NOT filtered.
//
// Each child's Complete is called eagerly when the parent Complete is
// called. Complete is intended to be cheap — it just sets up the
// pipeline; the expensive work (walking a directory, reading a slow
// command's stdout, …) only happens as Next is pulled. Calling all
// Complete()s up front lets us collect each child's newLastArg
// synchronously: the first non-empty value wins, in chain order. This
// matters for placeholders like {file}, where an entry such as `~/foo`
// must be expanded to `/home/user/foo` no matter where in the chain
// the file completer lives.
//
// The resulting iterator streams via iterator.Aggregate, which only
// advances to the next child once the current one is exhausted. So a
// slow first child never starves the later ones, and a caller that
// stops iterating early (Close) tears every child down without ever
// pulling from them.
//
// nil entries in completers panic on iteration: an empty slot in a
// completer chain is a programmer error and should fail loudly.
// If a child returns an error from Complete, the error is logged and
// the child is skipped; iteration continues with the remaining
// children.
func MultiCompleter(completers ...Completer) Completer {
	return FuncCompleter(func(
		ctx context.Context, args []string,
	) (iterator.Iterator[string], string, error) {
		if len(completers) == 0 {
			return iterator.Empty[string](), "", nil
		}

		iters := make([]iterator.Iterator[string], 0, len(completers))
		var newLastArg string
		for _, c := range completers {
			if c == nil {
				panic("command.MultiCompleter: nil child completer")
			}
			it, last, err := c.Complete(ctx, args)
			if err != nil {
				log.WithField("class", "command.MultiCompleter").
					Warnf("child completer returned error: %v", err)
				continue
			}
			if it == nil {
				continue
			}
			// First non-empty newLastArg wins, but keep walking so
			// later children also have their Complete invoked and
			// their iterators contribute to the streamed output.
			if newLastArg == "" && last != "" {
				newLastArg = last
			}
			iters = append(iters, errorSwallowingIterator(it))
		}
		if len(iters) == 0 {
			return iterator.Empty[string](), newLastArg, nil
		}
		return iterator.Aggregate(iters...), newLastArg, nil
	})
}

// errorSwallowingIterator wraps it so that Aggregate (which stops on
// the first child error) keeps streaming subsequent children when one
// child reports an iteration error. The error is logged for diagnostics
// instead of bubbling up.
func errorSwallowingIterator(it iterator.Iterator[string]) iterator.Iterator[string] {
	return iterator.FromFunc(func(ctx context.Context) (string, bool, error) {
		v, ok := it.Next(ctx)
		if !ok {
			if err := it.Err(); err != nil && !errors.Is(err, context.Canceled) {
				log.WithField("class", "command.MultiCompleter").
					Warnf("child completer iterator error: %v", err)
			}
			return v, false, nil
		}
		return v, true, nil
	}, it.Close)
}
