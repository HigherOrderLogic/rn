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

package vctrl

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"go.uber.org/mock/gomock"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/schemetest"
	"unstable.build/go-tui/workspace/workspaceapitest"
)

func TestLoadGitignore(t *testing.T) {
	t.Run("uses .gitignore excludes", func(t *testing.T) {
		cwd := newScheme(t)
		touchGitignore(t, cwd, ".ox.awe")
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox.awe"), false))
		assert.False(t, matcher.Match(makeURI(t, cwd, ".ox.go"), false))
	})

	t.Run("uses recursive .gitignore excludes", func(t *testing.T) {
		cwd := newFileScheme(t) // memscheme doesn't support dirs
		touchGitignoreAt(t, cwd, ".ox.CALIU", gitIgnoreFile)
		touchGitignoreAt(t, cwd, ".ox.BOIRA", filepath.Join("dir", "moreDirs", gitIgnoreFile))
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox.CALIU"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "dir/moreDirs/.ox.BOIRA"), false))
		assert.False(t, matcher.Match(makeURI(t, cwd, ".ox.go"), false))
	})

	t.Run("nested .gitignore patterns are scoped to their directory", func(t *testing.T) {
		cwd := newFileScheme(t) // memscheme doesn't support dirs
		// vendor/.gitignore says "*" — must only match inside vendor/, not
		// across the whole workspace.
		touchGitignoreAt(t, cwd, "*", filepath.Join("vendor", gitIgnoreFile))
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		// Inside vendor/: pattern applies.
		assert.True(t, matcher.Match(makeURI(t, cwd, "vendor/foo.go"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "vendor/sub/bar.go"), false))

		// Outside vendor/: pattern must NOT apply. This is the regression
		// from a nested pattern leaking globally and matching every entry.
		assert.False(t, matcher.Match(makeURI(t, cwd, "main.go"), false))
		assert.False(t, matcher.Match(makeURI(t, cwd, "cmd"), true))
		assert.False(t, matcher.Match(makeURI(t, cwd, "cmd/main.go"), false))
		assert.False(t, matcher.Match(makeURI(t, cwd, "pkg/lib.go"), false))
	})

	t.Run("uses .gitignore entire directory excludes", func(t *testing.T) {
		cwd := newScheme(t)
		touchGitignore(t, cwd, ".ox/")
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox/awe"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox/awe/inspiring/ide"), false))
		assert.False(t, matcher.Match(makeURI(t, cwd, ".ox.go"), false))
	})

	t.Run("uses .gitignore excludes with comments", func(t *testing.T) {
		cwd := newScheme(t)
		touchGitignore(t, cwd, "#this is a comment\n.ox.awe\n#this is another comment")
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox.awe"), false))
	})

	t.Run("file does not exist, uses common ignores", func(t *testing.T) {
		cwd := newScheme(t)
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		assert.True(t, matcher.Match(makeURI(t, cwd, "file.swp"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".file.swp"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "file.rswp"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".file.rswp"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox.sock"), false))
		assert.False(t, matcher.Match(makeURI(t, cwd, ".ox.awe"), false))
	})

	t.Run("common ignores exclude dependency and build noise dirs", func(t *testing.T) {
		cwd := newScheme(t)
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		// Python tooling / virtualenv / caches.
		assert.True(t, matcher.Match(makeURI(t, cwd, ".venv/lib/x.py"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".tox/py3/x.py"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".mypy_cache/x"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".pytest_cache/x"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "__pycache__/z.pyc"), false))
		// Nested virtualenv/caches in a monorepo sub-package.
		assert.True(t, matcher.Match(makeURI(t, cwd, "pkg/.venv/lib/x.py"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "pkg/__pycache__/z.pyc"), false))

		// JS/TS dependencies (unanchored, matches at any depth).
		assert.True(t, matcher.Match(makeURI(t, cwd, "node_modules/y.js"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "pkg/node_modules/y.js"), false))

		// Rust build output is root-anchored.
		assert.True(t, matcher.Match(makeURI(t, cwd, "target/debug/app"), false))

		// Real source is not excluded.
		assert.False(t, matcher.Match(makeURI(t, cwd, "src/main.py"), false))
		// A nested user dir literally named "target" must NOT be excluded
		// (Rust pattern is root-anchored).
		assert.False(t, matcher.Match(makeURI(t, cwd, "src/target/x.rs"), false))
	})

	t.Run("uri returns error is bubbled up", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := schemetest.NewMockScheme(ctrl)
		f := workspaceapitest.NewMockFile(ctrl)

		f.EXPECT().Read(gomock.Any()).Return(0, io.EOF).AnyTimes()
		f.EXPECT().Close().Return(nil).AnyTimes()
		mock.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(f, nil).
			AnyTimes()
		mock.EXPECT().ReadDir(gomock.Any()).
			Return(nil, nil).
			AnyTimes()
		mock.EXPECT().URI(gomock.Any()).Return(workspaceapi.URI{}, errors.New("boom"))
		_, err := LoadGitignore(mock)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("open .gitignore returns permissions error is ignored", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := schemetest.NewMockScheme(ctrl)

		uri, err := workspaceapi.ParseURI("memory:///tmp")
		require.NoError(t, err)

		mock.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, os.ErrPermission).
			Times(2) // .git/info/exclude
		mock.EXPECT().ReadDir(gomock.Any()).
			Return(nil, nil).
			Times(1)
		mock.EXPECT().URI(gomock.Any()).Return(uri, nil)
		_, err = LoadGitignore(mock)
		require.NoError(t, err)
	})

	t.Run(".gitignore is malformed, uses common patterns", func(t *testing.T) {
		cwd := newScheme(t)
		touchGitignore(t, cwd, "\x00\x00\x00{}jfl\x00kewjlkfw\njfklwjjk##")
		matcher, err := LoadGitignore(cwd)
		require.NoError(t, err)

		assert.False(t, matcher.Match(makeURI(t, cwd, ".ox.awe"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, ".ox.sock"), false))
		assert.True(t, matcher.Match(makeURI(t, cwd, "filename.swp"), false))
	})
}

// TestHiddenBaseMatcher exercises the standalone Matcher returned
// by HiddenBaseMatcher. The matcher must accept any path whose
// basename starts with a dot and reject every other path,
// regardless of whether the path has intermediate components.
func TestHiddenBaseMatcher(t *testing.T) {
	m := HiddenBaseMatcher()

	cases := []struct {
		relpath string
		isDir   bool
		want    bool
	}{
		{".git", true, true},
		{".DS_Store", false, true},
		{"sub/.git", true, true},
		{"sub/.gitignore", false, true},
		{"src", true, false},
		{"src/main.go", false, false},
		{"a/b.c/d", false, false},
		{".", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.relpath, func(t *testing.T) {
			assert.Equal(t, tc.want,
				m.MatchRelPath(tc.relpath, tc.isDir),
				"MatchRelPath(%q, %v)", tc.relpath, tc.isDir)
		})
	}

	// Match goes through workspaceapi.URI: only the basename
	// portion of the path drives the decision.
	cwd := newScheme(t)
	assert.True(t, m.Match(makeURI(t, cwd, ".git"), true))
	assert.True(t, m.Match(makeURI(t, cwd, "deep/.git"), true))
	assert.False(t, m.Match(makeURI(t, cwd, "src"), true))
}

func newScheme(t *testing.T) schemeapi.Scheme {
	uri, err := workspaceapi.ParseURI("memory:///tmp")
	require.NoError(t, err)
	scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	return scheme
}

func newFileScheme(t *testing.T) schemeapi.Scheme {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	uri, err := workspaceapi.ParseURI(filepath.Join("file:///", tempDir))
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	return scheme
}

func touchGitignore(t *testing.T, cwd schemeapi.Scheme, content string) {
	touchGitignoreAt(t, cwd, content, gitIgnoreFile)
}

func touchGitignoreAt(t *testing.T, cwd schemeapi.Scheme, content, at string) {
	require.NoError(t, cwd.MkdirAll(filepath.Dir(at), 0777))
	file, werr := cwd.OpenFile(at, os.O_CREATE|os.O_RDWR, 0666)
	require.Nil(t, werr)

	_, err := file.Write([]byte(content))
	require.NoError(t, err)

	err = file.Close()
	require.NoError(t, err)
}

func makeURI(t *testing.T, cwd schemeapi.Scheme, file string) workspaceapi.URI {
	uri, err := cwd.URI(file)
	require.NoError(t, err)
	return uri
}
