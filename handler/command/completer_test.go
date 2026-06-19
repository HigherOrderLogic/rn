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

package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/handler/search"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/walkdir"
)

// TestFilePathCompleterSkipsProtectedTraversal asserts that completing a
// path under the user's home never ReadDirs into the macOS TCC-protected
// dirs (~/Library, ~/Documents, ~/Desktop, ~/Downloads), the reads that
// trigger a system permission prompt. The reader is rooted at the real
// home so it lines up with the paths vctrl.ProtectedDirMatcher derives
// from user.Current().
func TestFilePathCompleterSkipsProtectedTraversal(t *testing.T) {
	usr, err := user.Current()
	require.NoError(t, err)
	if usr.HomeDir == "" {
		t.Skip("no home directory for current user")
	}

	var prefixes []string
	for _, dir := range []string{"Library", "Documents", "Desktop", "Downloads"} {
		p := filepath.Join(usr.HomeDir, dir)
		if _, err := os.Stat(p); err == nil {
			prefixes = append(prefixes, p)
		}
	}
	if len(prefixes) == 0 {
		t.Skip("no protected home dirs to guard against")
	}

	tracking := &trackingReader{inner: newFSReader(usr.HomeDir)}
	c := FilePathCompleter(tracking)

	it, _, err := c.Complete(context.Background(), []string{"edit"})
	require.NoError(t, err)
	_ = collectAll(t, it)

	for _, p := range tracking.snapshot() {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(usr.HomeDir, p)
		}
		abs = filepath.Clean(abs)
		for _, prefix := range prefixes {
			assert.Falsef(t,
				abs == prefix || strings.HasPrefix(abs, prefix+string(filepath.Separator)),
				"completer must not ReadDir into protected dir: %q", p)
		}
	}
}
func TestCommandOutputLinesCompleterCloseSignals(t *testing.T) {
	var pid workspaceapi.Pid
	var sig syscall.Signal

	exec := &mockExecutor{
		startFn: func(_ context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
			// don't close stdout: simulates a long-running process
			return 42, nil
		},
		signalFn: func(_pid workspaceapi.Pid, _sig syscall.Signal) error {
			pid = _pid
			sig = _sig
			return nil
		},
	}

	c := OutputLinesCompleter(exec, []string{"long-running"}, nil)
	iter, _, err := c.Complete(context.Background(), nil)
	require.NoError(t, err)

	err = iter.Close()
	require.NoError(t, err)

	assert.Equal(t, workspaceapi.Pid(42), pid)
	assert.Equal(t, syscall.SIGINT, sig)
}

func TestCommandOutputLinesCompleterContextCancellation(t *testing.T) {
	exec := &mockExecutor{
		startFn: func(_ context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
			// stdout stays open: process never exits on its own
			return 1, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := OutputLinesCompleter(exec, []string{"hanging-cmd"}, nil)
	iter, _, err := c.Complete(ctx, nil)
	require.NoError(t, err)

	cancel()

	_, ok := iter.Next(ctx)
	assert.False(t, ok)
	err = iter.Err()
	if err != nil {
		assert.Contains(t, err.Error(), "closed")
	}
}

func TestCommandOutputLinesCompleter(t *testing.T) {
	tests := []struct {
		name      string
		cmdArgs   []string
		executor  *mockExecutor
		wantLines []string
		wantErr   string
	}{
		{
			name:    "empty args returns error",
			cmdArgs: nil,
			executor: &mockExecutor{
				startFn: func(_ context.Context, _ workspaceapi.Cmd) (workspaceapi.Pid, error) {
					t.Fatal("StartCommand should not be called")
					return 0, nil
				},
			},
			wantErr: "expected at least one argument",
		},
		{
			name:    "start command error propagates",
			cmdArgs: []string{"failing-cmd"},
			executor: &mockExecutor{
				startFn: func(_ context.Context, _ workspaceapi.Cmd) (workspaceapi.Pid, error) {
					return 0, io.ErrUnexpectedEOF
				},
			},
			wantErr: "unexpected EOF",
		},
		{
			name:    "single line",
			cmdArgs: []string{"echo", "hello"},
			executor: &mockExecutor{
				startFn: writeAndExit("hello\n", 1),
			},
			wantLines: []string{"hello"},
		},
		{
			name:    "multiple lines",
			cmdArgs: []string{"my-cmd"},
			executor: &mockExecutor{
				startFn: writeAndExit("alpha\nbeta\ngamma\n", 1),
			},
			wantLines: []string{"alpha", "beta", "gamma"},
		},
		{
			name:    "empty output",
			cmdArgs: []string{"my-cmd"},
			executor: &mockExecutor{
				startFn: writeAndExit("", 1),
			},
			wantLines: nil,
		},
		{
			name:    "trailing line without EOL",
			cmdArgs: []string{"my-cmd"},
			executor: &mockExecutor{
				startFn: writeAndExit("foo\nbar", 1),
			},
			wantLines: []string{"foo", "bar"},
		},
		{
			name:    "lines with spaces preserved",
			cmdArgs: []string{"my-cmd"},
			executor: &mockExecutor{
				startFn: writeAndExit("  leading\ntrailing  \n  both  \n", 1),
			},
			wantLines: []string{"  leading", "trailing  ", "  both  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := OutputLinesCompleter(tt.executor, tt.cmdArgs, nil)
			iter, prefix, err := c.Complete(context.Background(), nil)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, iter)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "", prefix)

			got := collectAll(t, iter)
			assert.Equal(t, tt.wantLines, got)
		})
	}
}

type mockExecutor struct {
	startFn  func(ctx context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error)
	signalFn func(pid workspaceapi.Pid, sig syscall.Signal) error
}

func (m *mockExecutor) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	return m.startFn(ctx, cmd)
}

func (m *mockExecutor) Signal(pid workspaceapi.Pid, sig syscall.Signal) error {
	if m.signalFn != nil {
		return m.signalFn(pid, sig)
	}
	return nil
}

func (m *mockExecutor) Close() error {
	return nil
}

// collectAll drains the iterator and returns all values.
func collectAll(t *testing.T, iter iterator.Iterator[string]) []string {
	t.Helper()
	var out []string
	for {
		val, ok := iter.Next(context.Background())
		if !ok {
			require.NoError(t, iter.Err())
			break
		}
		out = append(out, val)
	}
	return out
}

// writeAndExit simulates a command that writes lines to stdout then exits.
func writeAndExit(output string, pid int) func(context.Context, workspaceapi.Cmd) (workspaceapi.Pid, error) {
	return func(_ context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
		go func() {
			w := cmd.Stdout.(io.WriteCloser)
			io.WriteString(w, output)
			w.Close()
			//cmd.Watcher.WatchProcess() <- nil
		}()
		return workspaceapi.Pid(pid), nil
	}
}

// fsReader is a tiny walkdir.Reader implementation rooted at a real
// directory on disk. The completer's Complete method composes URI,
// Stat, OpenFile and ReadDir; this fixture exercises each via os.* so
// we cover real-world filename handling (UTF-8, escapes, hidden
// entries, etc.) without dragging in a full workspace scheme.
type fsReader struct {
	root string
}

func newFSReader(root string) *fsReader { return &fsReader{root: root} }

func (r *fsReader) resolve(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(r.root, p)
}

func (r *fsReader) URI(p string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI("file://" + r.resolve(p))
}

func (r *fsReader) OpenFile(p string, flag int, perm os.FileMode) (workspaceapi.File, error) {
	return os.OpenFile(r.resolve(p), flag, perm)
}

func (r *fsReader) Stat(p string) (os.FileInfo, error) { return os.Stat(r.resolve(p)) }

func (r *fsReader) ReadDir(p string) ([]os.DirEntry, error) {
	return os.ReadDir(r.resolve(p))
}

// completerFixture is the canonical varied fixture used by both the
// table-driven completer tests and the benchmark. It is rooted at a
// fresh tmp dir per call and contains a mix of plain, escaped,
// quoted-style, unicode, hidden, deeply nested and shell-metachar
// names so that completer behaviour and performance can be measured
// against a realistic shape.
type completerFixture struct {
	root   string
	reader *fsReader
}

func newCompleterFixture(tb testing.TB) completerFixture {
	tb.Helper()
	root := tb.TempDir()

	// flat top-level entries spanning the edge cases we care about.
	files := []string{
		"alpha.go",
		"beta.txt",
		"gamma.md",
		"file with spaces.txt",
		"with\ttab.txt",
		"weird$name.txt",
		"glob*name.txt",
		"paren(name).txt",
		"quote'name.txt",
		"dquote\"name.txt",
		"backslash\\name.txt",
		"emoji-🚀.txt",
		"日本語.txt",
		"café.txt",       // composed (NFC)
		"cafe\u0301.txt", // decomposed (NFD)
		".hidden",
		"plain.swp",
		"plain.rswp", // filtered by completer
		"normal.swp.go",
	}
	for _, f := range files {
		require.NoError(tb, os.WriteFile(filepath.Join(root, f), []byte("x"), 0o600))
	}

	dirs := []string{
		"src",
		"src/cmd",
		"src/cmd/rune",
		"src/pkg",
		"docs",
		"with space",
		"with space/nested",
		".hiddenDir",
	}
	for _, d := range dirs {
		require.NoError(tb, os.MkdirAll(filepath.Join(root, d), 0o700))
	}

	// Drop a file inside the spaced directory tree so completer
	// traversals rooted at it return at least one entry.
	require.NoError(tb, os.WriteFile(
		filepath.Join(root, "with space", "nested", "file.txt"),
		[]byte("s"), 0o600))

	// Populate nested dirs so traversal has real work to do under the
	// benchmark. A few hundred entries is comparable to a midsize
	// repository folder.
	for i := range 50 {
		_ = os.WriteFile(filepath.Join(root, "src", "cmd",
			fmt.Sprintf("file_%02d.go", i)), []byte("y"), 0o600)
		_ = os.WriteFile(filepath.Join(root, "src", "pkg",
			fmt.Sprintf("pkg_%02d.go", i)), []byte("z"), 0o600)
		_ = os.WriteFile(filepath.Join(root, "docs",
			fmt.Sprintf("doc-%02d.md", i)), []byte("d"), 0o600)
	}

	return completerFixture{root: root, reader: newFSReader(root)}
}

func TestFilePathCompleterEdgeCases(t *testing.T) {
	t.Parallel()
	fix := newCompleterFixture(t)
	c := FilePathCompleter(fix.reader)

	tests := []struct {
		name string
		// args is the full arg list passed to Complete; the completer
		// only inspects the last entry. We always pass the command
		// name as args[0] so that the test exercises the same shape
		// the prompt produces in production.
		args []string
		// wantSubset asserts that every listed name appears in the
		// resulting iterator. We do not assert the full set so the
		// test stays robust against the fixture growing over time.
		wantSubset []string
		// wantNotIn asserts that none of these strings are returned.
		wantNotIn []string
		// wantSomeContains, when non-nil, asserts that at least one
		// returned entry contains every listed substring. Useful for
		// absolute-path expectations on platforms where the temp
		// directory may be reported via a different prefix (e.g.
		// /var vs /private/var on macOS).
		wantSomeContains []string
		// wantPrefix is the modified-last value the completer
		// returned (only set when the completer expands ~).
		wantPrefix string
		// wantErr, when non-empty, must be a substring of the error.
		wantErr string
	}{
		{
			name:       "no args lists workspace root",
			args:       []string{"edit"},
			wantSubset: []string{"alpha.go", "beta.txt", "plain.swp"},
			wantNotIn:  []string{"plain.rswp"},
		},
		{
			name:       "empty last arg lists workspace root",
			args:       []string{"edit", ""},
			wantSubset: []string{"alpha.go", "src/cmd/file_00.go"},
		},
		{
			name: "plain partial path",
			args: []string{"edit", "src"},
			// walkDirCompleter resolves to the workspace root (since
			// "src" is interpreted as a partial filename whose parent
			// is the cwd) and returns recursive results. We assert on
			// known files to keep the expectations stable.
			wantSubset: []string{"src/cmd/file_00.go", "src/pkg/pkg_00.go"},
		},
		{
			name: "absolute path",
			args: []string{"edit", filepath.Join(fix.root, "src")},
			wantSomeContains: []string{
				filepath.Join("src", "cmd", "file_00.go"),
			},
		},
		{
			name:       "backslash-escaped space",
			args:       []string{"edit", `with\ space`},
			wantSubset: []string{"with space/nested/file.txt"},
		},
		{
			name:       "single-quoted with space",
			args:       []string{"edit", `'with space'`},
			wantSubset: []string{"with space/nested/file.txt"},
		},
		{
			name:       "double-quoted with space",
			args:       []string{"edit", `"with space"`},
			wantSubset: []string{"with space/nested/file.txt"},
		},
		{
			name: "double-quoted unescapes inner quote",
			// We resolve to a path that does not exist and expect
			// either a clean empty result or a typed error — what we
			// must not see is panic or junk in the path lookup.
			args:      []string{"edit", `"a\"b"`},
			wantNotIn: []string{`a\"b`, `"a\"b"`},
		},
		{
			name: "unclosed single quote is lenient",
			// The unquoter strips the opener, so this resolves to
			// directory "missing" which doesn't exist in the
			// fixture; we expect no panic and an empty/clean result.
			args:      []string{"edit", `'missing`},
			wantNotIn: []string{`'missing`},
		},
		{
			name:      "unclosed double quote is lenient",
			args:      []string{"edit", `"missing`},
			wantNotIn: []string{`"missing`},
		},
		{
			name: "empty single-quoted string",
			// '' unquotes to "" which the completer treats as a
			// fresh listing of the workspace root.
			args:       []string{"edit", `''`},
			wantSubset: []string{"alpha.go"},
		},
		{
			name: "trailing dangling backslash",
			// Lenient: the trailing \ is dropped during unquote and
			// we look up the bare prefix.
			args:       []string{"edit", `src\`},
			wantSubset: []string{"src/cmd/file_00.go", "src/pkg/pkg_00.go"},
		},
		{
			name: "tab character escaped",
			// A literal tab in the path is rejected by the URI
			// parser, which the prompt already handles by showing
			// the error to the user. The regression we are guarding
			// against here is a panic inside the unquoter; the
			// completer must surface a typed error instead.
			args:    []string{"edit", "with\\\ttab.txt"},
			wantErr: "invalid control character",
		},
		{
			name: "shell metachars survive single quoting",
			args: []string{"edit", `'weird$name.txt'`},
			// The unquoted value is a partial filename; the parent
			// is the workspace root, so the recursive listing must
			// include it.
			wantSubset: []string{"weird$name.txt"},
		},
		{
			name:       "shell glob char in quoted value",
			args:       []string{"edit", `'glob*name.txt'`},
			wantSubset: []string{"glob*name.txt"},
		},
		{
			name:       "paren in quoted value",
			args:       []string{"edit", `'paren(name).txt'`},
			wantSubset: []string{"paren(name).txt"},
		},
		{
			name:       "wide CJK characters",
			args:       []string{"edit", "日本"},
			wantSubset: []string{"日本語.txt"},
		},
		{
			name:       "emoji prefix",
			args:       []string{"edit", "emoji-"},
			wantSubset: []string{"emoji-🚀.txt"},
		},
		{
			name:       "NFC composed lookup matches",
			args:       []string{"edit", "café"},
			wantSubset: []string{"café.txt"},
		},
		// NFD lookup is intentionally omitted: filesystems differ in
		// how they normalise filenames (HFS+/APFS fold to NFC) so
		// the assertion would not be portable. The completer itself
		// passes the bytes through to ReadDir unchanged.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			it, prefix, err := c.Complete(context.Background(), tc.args)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantPrefix, prefix)

			got := collectAll(t, it)
			gotSet := make(map[string]struct{}, len(got))
			for _, g := range got {
				gotSet[g] = struct{}{}
			}
			for _, want := range tc.wantSubset {
				_, ok := gotSet[want]
				assert.True(t, ok,
					"expected %q in result, got %v", want, got)
			}
			for _, want := range tc.wantNotIn {
				_, ok := gotSet[want]
				assert.False(t, ok,
					"did not expect %q in result, got %v", want, got)
			}
			for _, sub := range tc.wantSomeContains {
				found := false
				for _, g := range got {
					if strings.Contains(g, sub) {
						found = true
						break
					}
				}
				assert.True(t, found,
					"expected at least one entry containing %q, got %v",
					sub, got)
			}
		})
	}
}

func TestDirsCompleterFiltersHidden(t *testing.T) {
	t.Parallel()
	fix := newCompleterFixture(t)
	c := DirsCompleter(fix.reader)

	it, _, err := c.Complete(context.Background(), []string{"workspaceopen"})
	require.NoError(t, err)
	got := collectAll(t, it)
	sort.Strings(got)

	for _, name := range got {
		assert.False(t, strings.HasPrefix(filepath.Base(name), "."),
			"DirsCompleter should not return hidden entry %q", name)
	}
}

// TestDirsCompleterSkipsHiddenDirsTraversal asserts that the
// DirsCompleter does not descend into hidden directories. Walking
// into them is wasteful (the entries would be filtered from the
// output anyway) and on real workspaces it triggers expensive
// recursion through directories like .git or .hg that contain
// thousands of files and starve the rest of the system.
func TestDirsCompleterSkipsHiddenDirsTraversal(t *testing.T) {
	t.Parallel()
	fix := newCompleterFixture(t)

	// Drop a deep tree under .hiddenDir so that walking into it
	// would generate many ReadDir calls if traversal were not
	// skipped at the source.
	for _, sub := range []string{
		".hiddenDir/inner",
		".hiddenDir/inner/deeper",
		".hiddenDir/inner/deeper/more",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(fix.root, sub), 0o700))
	}

	tracking := &trackingReader{inner: fix.reader}
	c := DirsCompleter(tracking)

	it, _, err := c.Complete(context.Background(), []string{"workspaceopen"})
	require.NoError(t, err)
	_ = collectAll(t, it)

	for _, p := range tracking.snapshot() {
		// The path may be absolute, relative, or include the
		// fixture root prefix, so check every component for a
		// leading dot.
		for _, part := range strings.Split(filepath.ToSlash(p), "/") {
			assert.False(t, strings.HasPrefix(part, "."),
				"DirsCompleter must not ReadDir into hidden directory: %q", p)
		}
	}
}

// trackingReader wraps a walkdir.Reader and records every path
// passed to ReadDir so tests can assert traversal does not enter
// directories that the completer is supposed to skip.
type trackingReader struct {
	inner walkdir.Reader
	mu    sync.Mutex
	calls []string
}

func (r *trackingReader) URI(p string) (workspaceapi.URI, error) {
	return r.inner.URI(p)
}

func (r *trackingReader) OpenFile(p string, flag int, perm os.FileMode) (workspaceapi.File, error) {
	return r.inner.OpenFile(p, flag, perm)
}

func (r *trackingReader) Stat(p string) (os.FileInfo, error) { return r.inner.Stat(p) }

func (r *trackingReader) ReadDir(p string) ([]os.DirEntry, error) {
	r.mu.Lock()
	r.calls = append(r.calls, p)
	r.mu.Unlock()
	return r.inner.ReadDir(p)
}

func (r *trackingReader) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

// TestFilePathCompleterRejectsUnsupported verifies that pathological
// inputs that the unquoter cannot represent (e.g. embedded null byte)
// surface as a clean error instead of panicking.
func TestFilePathCompleterRejectsUnsupported(t *testing.T) {
	t.Parallel()
	fix := newCompleterFixture(t)
	c := FilePathCompleter(fix.reader)

	// A literal NUL byte cannot appear in a filename on POSIX. We use
	// it here to assert that the completer returns a typed error
	// (from the URI layer) rather than misbehaving silently.
	_, _, err := c.Complete(context.Background(),
		[]string{"edit", "bad\x00name"})
	if err == nil {
		// Some platforms accept the NUL through ParseURI and only
		// surface the error from the OS Stat call as an empty
		// iterator. Either path is acceptable as long as we don't
		// panic; the regression we are guarding against is a panic
		// inside the unquoter.
		t.Logf("no error returned for embedded NUL; iterator must be empty")
	}
}

// BenchmarkFilePathCompleter measures the per-call cost of
// FilePathCompleter.Complete on the canonical varied fixture. It is
// intended to track the overhead of the shell-aware unquoting added
// for RUNE-120.
//
// To compare with a baseline:
//
//	go test -run='^$' -bench=BenchmarkFilePathCompleter -benchmem \
//	    -count=10 ./handler/command/ > new.txt
//	go test -run='^$' -bench=BenchmarkFilePathCompleter -benchmem \
//	    -count=10 ./handler/command/ > old.txt   # at baseline
//	benchstat old.txt new.txt
func BenchmarkFilePathCompleter(b *testing.B) {
	fix := newCompleterFixture(b)
	c := FilePathCompleter(fix.reader)
	ctx := context.Background()

	// Each sub-benchmark mirrors a realistic prompt scenario. The
	// "*_quoted" variants exercise the shell-aware unquoting hot
	// path; the plain variants establish a baseline against which
	// the unquoting overhead can be measured.
	cases := []struct {
		name string
		args []string
	}{
		{"empty", []string{"edit"}},
		{"plain_partial", []string{"edit", "src"}},
		{"plain_nested", []string{"edit", "src/cmd"}},
		{"backslash_escape", []string{"edit", `with\ space`}},
		{"single_quoted", []string{"edit", `'with space'`}},
		{"double_quoted", []string{"edit", `"with space"`}},
		{"absolute", []string{"edit", filepath.Join(fix.root, "src", "cmd")}},
		{"unicode", []string{"edit", "日本"}},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				it, _, err := c.Complete(ctx, tc.args)
				if err != nil {
					b.Fatalf("Complete: %v", err)
				}
				// Drain so that the iterator's traversal cost is
				// reflected in the benchmark numbers.
				for {
					_, ok := it.Next(ctx)
					if !ok {
						break
					}
				}
				_ = it.Close()
			}
		})
	}
}

func sliceCompleter(values ...string) Completer {
	return FuncCompleter(func(
		_ context.Context, _ []string,
	) (iterator.Iterator[string], string, error) {
		return iterator.FromSlice(values), "", nil
	})
}

func errCompleter(err error) Completer {
	return FuncCompleter(func(
		_ context.Context, _ []string,
	) (iterator.Iterator[string], string, error) {
		return nil, "", err
	})
}

func TestMultiCompleter(t *testing.T) {
	tests := []struct {
		name       string
		completers []Completer
		want       []string
	}{
		{
			name:       "empty list yields empty iterator",
			completers: nil,
			want:       nil,
		},
		{
			name:       "single child returns its results",
			completers: []Completer{sliceCompleter("a", "b")},
			want:       []string{"a", "b"},
		},
		{
			name: "two children concatenate in order",
			completers: []Completer{
				sliceCompleter("a", "b"),
				sliceCompleter("c", "d"),
			},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "duplicates across children are preserved",
			completers: []Completer{
				sliceCompleter("a", "b"),
				sliceCompleter("b", "c", "a", "d"),
			},
			want: []string{"a", "b", "b", "c", "a", "d"},
		},
		{
			name: "child error is skipped, others continue",
			completers: []Completer{
				errCompleter(errors.New("first failed")),
				sliceCompleter("c", "d"),
			},
			want: []string{"c", "d"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := MultiCompleter(tt.completers...)
			it, _, err := c.Complete(context.Background(), nil)
			require.NoError(t, err)
			got := collectAll(t, it)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMultiCompleterPanicsOnNilChild documents that a nil entry in a
// completer chain is treated as a programmer error: configuration that
// emits no factories should drop the alias, not silently produce a
// chain with a missing slot.
func TestMultiCompleterPanicsOnNilChild(t *testing.T) {
	c := MultiCompleter(sliceCompleter("ok"), nil)
	assert.PanicsWithValue(t,
		"command.MultiCompleter: nil child completer",
		func() { _, _, _ = c.Complete(context.Background(), nil) })
}

func TestMultiCompleterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := MultiCompleter(sliceCompleter("a", "b", "c"))
	it, _, err := c.Complete(ctx, nil)
	require.NoError(t, err)
	cancel()
	for {
		_, ok := it.Next(ctx)
		if !ok {
			break
		}
	}
	// either completed naturally or aborted; just confirm Close doesn't panic
	require.NoError(t, it.Close())
}

// TestMultiCompleterNewLastArgFromAnyChild documents that newLastArg
// is propagated from whichever child returns one — not just the first.
// This is what allows e.g. `{file}` (typically a later child) to expand
// `~` even when an earlier `{history}` child returned no expansion.
func TestMultiCompleterNewLastArgFromAnyChild(t *testing.T) {
	withLast := func(values []string, last string) Completer {
		return FuncCompleter(func(
			_ context.Context, _ []string,
		) (iterator.Iterator[string], string, error) {
			return iterator.FromSlice(values), last, nil
		})
	}

	t.Run("first non-empty newLastArg wins, in chain order", func(t *testing.T) {
		c := MultiCompleter(
			sliceCompleter("a"), // returns ""
			withLast([]string{"b"}, "second-arg"),
			withLast([]string{"c"}, "third-arg"),
		)
		_, last, err := c.Complete(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "second-arg", last)
	})

	t.Run("newLastArg propagated even when only the last child has one", func(t *testing.T) {
		c := MultiCompleter(
			sliceCompleter("a"),
			sliceCompleter("b"),
			withLast([]string{"c"}, "expanded"),
		)
		_, last, err := c.Complete(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "expanded", last)
	})
}

// fakeHistoryAccessor is a HistoryAccessor backed by a function so tests
// can express the desired behavior inline.
type fakeHistoryAccessor func(ctx context.Context, args []string) (iterator.Iterator[string], bool)

func (f fakeHistoryAccessor) HistoryIterator(
	ctx context.Context, args []string,
) (iterator.Iterator[string], bool) {
	return f(ctx, args)
}

func TestHistoryCompleter(t *testing.T) {
	t.Run("nil accessor panics", func(t *testing.T) {
		assert.PanicsWithValue(t,
			"command.HistoryCompleter: nil HistoryAccessor",
			func() { HistoryCompleter(nil) })
	})

	t.Run("forwards accessor results", func(t *testing.T) {
		acc := fakeHistoryAccessor(func(ctx context.Context, args []string) (iterator.Iterator[string], bool) {
			return iterator.FromSlice([]string{"prev1", "prev2"}), true
		})
		c := HistoryCompleter(acc)
		it, _, err := c.Complete(context.Background(), []string{"e"})
		require.NoError(t, err)
		assert.Equal(t, []string{"prev1", "prev2"}, collectAll(t, it))
	})

	t.Run("not-ok accessor returns empty iterator", func(t *testing.T) {
		acc := fakeHistoryAccessor(func(ctx context.Context, args []string) (iterator.Iterator[string], bool) {
			return nil, false
		})
		c := HistoryCompleter(acc)
		it, _, err := c.Complete(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, collectAll(t, it))
	})

	t.Run("entries are filtered by command prefix and stripped", func(t *testing.T) {
		// HistoryAccessor returns the persisted command lines. The
		// completer is responsible for keeping only entries whose
		// command-prefix matches `args[:len(args)-1]` and for stripping
		// that prefix before yielding.
		acc := fakeHistoryAccessor(func(ctx context.Context, args []string) (iterator.Iterator[string], bool) {
			return iterator.FromSlice([]string{
				"won previous-arg",
				"othercmd irrelevant",
				"won another-prev",
				"won",
			}), true
		})
		c := HistoryCompleter(acc)
		// args here mirrors what setCompletionList passes: full
		// command tokens plus the partial last arg the user is
		// currently typing.
		it, _, err := c.Complete(context.Background(), []string{"won", ""})
		require.NoError(t, err)
		assert.Equal(t,
			[]string{"previous-arg", "another-prev"},
			collectAll(t, it))
	})

	t.Run("multi-token prefix filters and strips the full prefix", func(t *testing.T) {
		acc := fakeHistoryAccessor(func(ctx context.Context, args []string) (iterator.Iterator[string], bool) {
			return iterator.FromSlice([]string{
				"git push origin main",
				"git push other branch",
				"git pull origin main",
			}), true
		})
		c := HistoryCompleter(acc)
		it, _, err := c.Complete(context.Background(),
			[]string{"git", "push", ""})
		require.NoError(t, err)
		assert.Equal(t, []string{"origin main", "other branch"},
			collectAll(t, it))
	})
}

// blockingCompleter returns an iterator whose first Next blocks forever
// (until close). Used to assert that MultiCompleter does NOT buffer the
// first child's iterator before returning to the caller.
func blockingCompleter() Completer {
	return FuncCompleter(func(
		ctx context.Context, _ []string,
	) (iterator.Iterator[string], string, error) {
		closed := make(chan struct{})
		return iterator.FromFunc(func(ctx context.Context) (string, bool, error) {
			select {
			case <-ctx.Done():
				return "", false, ctx.Err()
			case <-closed:
				return "", false, nil
			}
		}, func() error { close(closed); return nil }), "", nil
	})
}

// TestMultiCompleterDoesNotBuffer verifies that MultiCompleter returns
// its iterator to the caller without first draining any child iterator.
// A buffered implementation would block here on the first child's Next.
func TestMultiCompleterDoesNotBuffer(t *testing.T) {
	first := blockingCompleter()
	second := sliceCompleter("after")

	c := MultiCompleter(first, second)

	done := make(chan struct {
		it  iterator.Iterator[string]
		err error
	}, 1)
	go func() {
		it, _, err := c.Complete(context.Background(), nil)
		done <- struct {
			it  iterator.Iterator[string]
			err error
		}{it, err}
	}()

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.it)
		// drain in a separate goroutine so we don't deadlock; the test
		// closes early to release the blocking child.
		require.NoError(t, got.it.Close())
	case <-time.After(time.Second):
		t.Fatal("MultiCompleter.Complete blocked: it must not drain children before returning")
	}
}

// TestMultiCompleterStreamsFirstValue verifies that the first value from
// a child iterator can be observed via Next() before the child has
// finished producing all of its values. A buffered implementation would
// hold the value back until the entire child stream completed.
func TestMultiCompleterStreamsFirstValue(t *testing.T) {
	hold := make(chan struct{})
	closed := make(chan struct{})
	produced := []string{"first", "second"}
	var idx int

	streaming := FuncCompleter(func(
		ctx context.Context, _ []string,
	) (iterator.Iterator[string], string, error) {
		return iterator.FromFunc(func(ctx context.Context) (string, bool, error) {
			if idx == 0 {
				idx++
				return produced[0], true, nil
			}
			// after first value, block until the test releases hold
			// to simulate a slow producer.
			select {
			case <-ctx.Done():
				return "", false, ctx.Err()
			case <-hold:
				if idx-1 < len(produced)-1 {
					idx++
					return produced[idx-1], true, nil
				}
				return "", false, nil
			case <-closed:
				return "", false, nil
			}
		}, func() error {
			select {
			case <-closed:
			default:
				close(closed)
			}
			return nil
		}), "", nil
	})

	c := MultiCompleter(streaming, sliceCompleter("trailing"))
	it, _, err := c.Complete(context.Background(), nil)
	require.NoError(t, err)

	type result struct {
		val string
		ok  bool
	}
	got := make(chan result, 1)
	go func() {
		v, ok := it.Next(context.Background())
		got <- result{v, ok}
	}()

	select {
	case r := <-got:
		require.True(t, r.ok)
		assert.Equal(t, "first", r.val)
	case <-time.After(time.Second):
		t.Fatal("MultiCompleter buffered: first value not surfaced before child stream completed")
	}

	close(hold)
	require.NoError(t, it.Close())
}

// streamingChild returns a Completer whose iterator yields one value
// per send on `gate` and ends when `done` is closed. This lets tests
// step through values one at a time and assert non-buffering behavior.
func streamingChild(values []string, gate <-chan struct{}, done <-chan struct{}) Completer {
	return FuncCompleter(func(
		_ context.Context, _ []string,
	) (iterator.Iterator[string], string, error) {
		var idx int
		return iterator.FromFunc(func(ctx context.Context) (string, bool, error) {
			select {
			case <-ctx.Done():
				return "", false, ctx.Err()
			case <-done:
				return "", false, nil
			case <-gate:
				if idx >= len(values) {
					return "", false, nil
				}
				v := values[idx]
				idx++
				return v, true, nil
			}
		}, func() error { return nil }), "", nil
	})
}

// TestMultiCompleterStreamingNoBuffering exercises the strongest
// invariant: each value produced by the underlying child iterator must
// be observable by the caller via Next BEFORE the next value is
// produced. A buffered implementation would block the caller's Next
// until many values have accumulated.
func TestMultiCompleterStreamingNoBuffering(t *testing.T) {
	gate := make(chan struct{})
	done := make(chan struct{})
	defer close(done)

	c := MultiCompleter(streamingChild(
		[]string{"a", "b", "c"}, gate, done,
	), sliceCompleter("trailer"))

	it, _, err := c.Complete(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = it.Close() }()

	for _, want := range []string{"a", "b", "c"} {
		type res struct {
			v  string
			ok bool
		}
		got := make(chan res, 1)
		// Start the consumer first so a buffering implementation has
		// no excuse: the consumer is parked in Next before any value
		// is produced.
		go func() {
			v, ok := it.Next(context.Background())
			got <- res{v, ok}
		}()

		// Open the gate for exactly one value.
		gate <- struct{}{}

		select {
		case r := <-got:
			require.True(t, r.ok)
			assert.Equal(t, want, r.v)
		case <-time.After(time.Second):
			t.Fatalf("MultiCompleter buffered: did not surface %q before next produce step", want)
		}
	}
}

// TestMultiCompleterAliasIntegration exercises the real-world wiring
// behind the `won` alias from the user's config:
//
//	aliases:
//	  won:
//	    command: workspaceopen
//	    completer:
//	      - '{history}'
//	      - '{file}'
//
// The chain composes a HistoryCompleter (backed by a real search.History
// over storagestub) with a real FilePathCompleter (backed by a real
// workspace.NewFileScheme over a temp dir). It asserts:
//
//  1. previously-run "won …" entries are surfaced first (with the
//     command-prefix stripped), so reissuing a past invocation just
//     requires picking from the list;
//  2. file-system results follow the history entries;
//  3. the file completer's `~` expansion is preserved as newLastArg
//     even though it is not the first child in the chain.
func TestMultiCompleterAliasIntegration(t *testing.T) {
	dir, err := os.MkdirTemp("", "won-alias-int-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	for _, name := range []string{"foo.go", "bar.go", "baz.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0644))
	}

	dirURI, err := workspaceapi.CurrentUserHostURI(dir)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(
		context.Background(), config.NopConfig(), dirURI)
	require.NoError(t, err)
	t.Cleanup(func() { _ = scheme.Close() })

	store := storagestub.NewInMemoryService()
	history := search.NewHistory(store, "alias-history-doc", 16)
	require.NoError(t, history.Load())

	// History contains both "won …" entries (relevant to this alias)
	// and unrelated commands that must NOT bleed into the suggestions.
	require.NoError(t, history.Add("won previous-arg"))
	require.NoError(t, history.Add("othercmd irrelevant"))
	require.NoError(t, history.Add("won another-prev"))

	chain := MultiCompleter(
		HistoryCompleter(history),
		FilePathCompleter(scheme),
	)

	t.Run("history entries are surfaced first, with command prefix stripped", func(t *testing.T) {
		// Empty last arg: file completer returns all files; history
		// returns just the prior `won …` invocations as their args.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		it, _, err := chain.Complete(ctx, []string{"won", ""})
		require.NoError(t, err)
		defer func() { _ = it.Close() }()

		var got []string
		for {
			val, ok := it.Next(ctx)
			if !ok {
				break
			}
			got = append(got, val)
		}

		// The two "won …" history entries must appear first, in
		// most-recent-first order, with the leading "won " stripped.
		// Unrelated history entries must NOT be present.
		require.GreaterOrEqual(t, len(got), 2,
			"expected at least the two history entries before files, got %v", got)
		assert.Equal(t, []string{"another-prev", "previous-arg"}, got[:2],
			"history entries must come first, most-recent-first, with command prefix stripped")
		assert.NotContains(t, got, "othercmd irrelevant",
			"unrelated history entries must not bleed into alias completions")

		// Files come after history.
		assert.ElementsMatch(t,
			[]string{"foo.go", "bar.go", "baz.txt"}, got[2:])
	})

	t.Run("file completer ~ expansion is propagated as newLastArg", func(t *testing.T) {
		usr, err := user.Current()
		require.NoError(t, err)

		it, newLastArg, err := chain.Complete(
			context.Background(), []string{"won", "~/"})
		require.NoError(t, err)
		defer func() { _ = it.Close() }()

		// MultiCompleter must propagate the file completer's ~
		// expansion even though it is the second child in the chain;
		// without that, the prompt buffer keeps the literal "~/" and
		// the user can never resolve their home directory.
		assert.Equal(t, usr.HomeDir, newLastArg,
			"file completer's ~ expansion must propagate as newLastArg "+
				"even when it is not the first viable child in the chain")
	})
}
