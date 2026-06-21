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

package debugshell

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"unstable.build/go-tui/ide/idedebug"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/workspace"
)

// langGrammarPkg serves the tree-sitter grammar in grammar for a
// single langID, satisfying syntax.PkgManager for completion tests.
type langGrammarPkg struct {
	langID  string
	grammar string
}

func (p *langGrammarPkg) LibDir(
	_ context.Context, langID string,
) (iterator.Iterator[string], error) {
	var files []string
	if p.grammar != "" && langID == p.langID {
		if entries, err := os.ReadDir(p.grammar); err == nil {
			for _, e := range entries {
				files = append(files, filepath.Join(p.grammar, e.Name()))
			}
		}
	}
	return iterator.FromSlice(files), nil
}

// newCompletionHandler builds a Handler backed by a real tree-sitter
// parser for langID over a temp workspace populated with files. The
// adapters slice controls which languages completion will query.
func newCompletionHandler(
	t *testing.T, langID, grammar string,
	files map[string]string, adapters []string,
) *Handler {
	t.Helper()
	tmp := t.TempDir()
	for name, src := range files {
		path := filepath.Join(tmp, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}

	wuri, err := workspaceapi.ParseURI("file://" + tmp)
	require.NoError(t, err)
	fs, err := workspace.NewFileScheme(
		context.Background(), config.NopConfig(), wuri,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fs.Close() })

	pkg := &langGrammarPkg{langID: langID, grammar: grammar}
	parser := syntax.NewParser(fs, pkg, wuri)

	adapterMap := make(map[string]idedebug.AdapterConfig, len(adapters))
	for _, lang := range adapters {
		adapterMap[lang] = idedebug.AdapterConfig{}
	}
	h := New(newFakeDebugger(), nil, nil, parser, fs, Config{
		WorkspaceURI:     wuri,
		Debugger:         idedebug.Config{Adapters: adapterMap},
		ScheduleNextTick: syncScheduleNextTick,
	})
	return h
}

// drain collects all values from it, closing it when done.
func drain(t *testing.T, it iterator.Iterator[string]) []string {
	t.Helper()
	out, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	return out
}

func TestHandler_CompleteLaunchProgram_Go(t *testing.T) {
	t.Parallel()
	grammar := unitGrammarDir(t)

	files := map[string]string{
		"cmd/app/main.go": "package main\n\nfunc main() {}\n",
		"internal/lib.go": "package internal\n\nfunc Helper() {}\n",
	}

	t.Run("suggests main file, not non-main", func(t *testing.T) {
		h := newCompletionHandler(t, "go", grammar, files, []string{"go"})
		got := drain(t, h.completeLaunchProgram(""))
		assert.Equal(t, []string{"cmd/app/main.go"}, got)
	})

	t.Run("prefix narrows candidates", func(t *testing.T) {
		h := newCompletionHandler(t, "go", grammar, files, []string{"go"})
		assert.Equal(t, []string{"cmd/app/main.go"},
			drain(t, h.completeLaunchProgram("cmd/")))
		assert.Empty(t, drain(t, h.completeLaunchProgram("internal/")))
	})

	t.Run("only configured adapters are queried", func(t *testing.T) {
		h := newCompletionHandler(t, "go", grammar, files, []string{"python"})
		assert.Empty(t, drain(t, h.completeLaunchProgram("")))
	})

	t.Run("no adapters means no completion", func(t *testing.T) {
		h := newCompletionHandler(t, "go", grammar, files, nil)
		assert.Empty(t, drain(t, h.completeLaunchProgram("")))
	})

	t.Run("no matches yields empty, no panic", func(t *testing.T) {
		h := newCompletionHandler(t, "go", grammar,
			map[string]string{"internal/lib.go": "package internal\n"},
			[]string{"go"})
		assert.Empty(t, drain(t, h.completeLaunchProgram("")))
	})

	t.Run("excludes entrypoints under noise dirs", func(t *testing.T) {
		mainSrc := "package main\n\nfunc main() {}\n"
		h := newCompletionHandler(t, "go", grammar, map[string]string{
			"cmd/app/main.go":                   mainSrc,
			".venv/dep/main.go":                 mainSrc,
			"node_modules/dep/main.go":          mainSrc,
			"target/debug/build_script/main.go": mainSrc,
		}, []string{"go"})
		assert.Equal(t, []string{"cmd/app/main.go"},
			drain(t, h.completeLaunchProgram("")))
	})
}

func TestHandler_CompleteArgs_Launch(t *testing.T) {
	t.Parallel()
	grammar := unitGrammarDir(t)
	files := map[string]string{
		"cmd/app/main.go": "package main\n\nfunc main() {}\n",
	}
	h := newCompletionHandler(t, "go", grammar, files, []string{"go"})

	t.Run("launch with no token offers programs", func(t *testing.T) {
		assert.Equal(t, []string{"cmd/app/main.go"},
			drain(t, h.completeArgs([]string{subLaunch, ""})))
	})

	t.Run("launch -e FOO=bar still offers programs", func(t *testing.T) {
		assert.Equal(t, []string{"cmd/app/main.go"},
			drain(t, h.completeArgs([]string{subLaunch, "-e", "FOO=bar", ""})))
	})

	t.Run("once a program token is present completion is empty", func(t *testing.T) {
		assert.Empty(t,
			drain(t, h.completeArgs([]string{subLaunch, "cmd/app/main.go", ""})))
	})
}

func TestLaunchProgramPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		args       []string
		wantPrefix string
		wantOK     bool
	}{
		{"empty", nil, "", true},
		{"typing program", []string{"cmd/"}, "cmd/", true},
		{"after -e value", []string{"-e", "FOO=bar"}, "", true},
		{"after -e value, typing", []string{"-e", "FOO=bar", "cm"}, "cm", true},
		{"dangling -e", []string{"-e"}, "", true},
		{"after --", []string{"--"}, "", true},
		{"after --, typing", []string{"--", "cm"}, "cm", true},
		{"program already present", []string{"cmd/app/main.go", ""}, "", false},
		{"program plus args", []string{"prog", "arg1", "arg2"}, "", false},
		{"unknown flag treated as flag", []string{"-x", "cm"}, "cm", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix, ok := launchProgramPrefix(tc.args)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantPrefix, prefix)
		})
	}
}

func TestEntrypointMatchPredicates(t *testing.T) {
	t.Parallel()
	mk := func(text string) syntaxapi.Result {
		return syntaxapi.Result{Text: text}
	}
	t.Run("rust matches main only", func(t *testing.T) {
		m := entrypointQueries["rust"].match
		assert.True(t, m(mk("main")))
		assert.False(t, m(mk("helper")))
	})
	t.Run("python matches __name__ only", func(t *testing.T) {
		m := entrypointQueries["python"].match
		assert.True(t, m(mk("__name__")))
		assert.False(t, m(mk("__main__")))
		assert.False(t, m(mk("x")))
	})
}

func TestHandler_CompleteLaunchProgram_Python(t *testing.T) {
	t.Parallel()
	grammar := pyGrammarDir(t)

	files := map[string]string{
		"app/main.py": "if __name__ == \"__main__\":\n    pass\n",
		"app/util.py": "def helper():\n    pass\n",
	}
	h := newCompletionHandler(t, "python", grammar, files, []string{"python"})
	assert.Equal(t, []string{"app/main.py"},
		drain(t, h.completeLaunchProgram("")))
}

func TestHandler_CompleteLaunchProgram_PythonExcludesVenv(t *testing.T) {
	t.Parallel()
	grammar := pyGrammarDir(t)

	guard := "if __name__ == \"__main__\":\n    pass\n"
	files := map[string]string{
		"src/main.py":       guard,
		".venv/app/main.py": guard,
	}
	h := newCompletionHandler(t, "python", grammar, files, []string{"python"})
	assert.Equal(t, []string{"src/main.py"},
		drain(t, h.completeLaunchProgram("")))
}

// blockingSearchParser returns a Search iterator whose Next blocks
// until the goroutine context is cancelled, modelling a slow
// tree-sitter scan. It lets tests assert that completeLaunchProgram
// performs the scan off the calling goroutine.
type blockingSearchParser struct {
	passThroughParser
	started chan struct{}
	once    sync.Once
}

func (p *blockingSearchParser) Search(string, []string, ...string) (
	iterator.Iterator[syntaxapi.Result], error,
) {
	return iterator.FromFunc(
		func(ctx context.Context) (syntaxapi.Result, bool, error) {
			p.once.Do(func() { close(p.started) })
			<-ctx.Done()
			return syntaxapi.Result{}, false, ctx.Err()
		},
		func() error { return nil },
	), nil
}

func TestCompleteLaunchProgram_DoesNotBlockCaller(t *testing.T) {
	t.Parallel()
	parser := &blockingSearchParser{started: make(chan struct{})}
	h := New(newFakeDebugger(), nil, nil, parser, passThroughFS{}, Config{
		Debugger: idedebug.Config{
			Adapters: map[string]idedebug.AdapterConfig{"go": {}},
		},
		ScheduleNextTick: syncScheduleNextTick,
	})

	done := make(chan iterator.Iterator[string], 1)
	go func() { done <- h.completeLaunchProgram("") }()

	var it iterator.Iterator[string]
	select {
	case it = <-done:
	case <-time.After(time.Second):
		t.Fatal("completeLaunchProgram blocked the calling goroutine")
	}

	// The background scan must have started even though the call
	// returned, and Close must cancel it so the goroutine exits.
	select {
	case <-parser.started:
	case <-time.After(time.Second):
		t.Fatal("background scan did not start")
	}
	require.NoError(t, it.Close())
}
