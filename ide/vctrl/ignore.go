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
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/ernestrc/logd-go/logging"
	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

const (
	gitCommentChar  = "#"
	gitIgnoreFile   = ".gitignore"
	gitDir          = ".git"
	infoExcludeFile = gitDir + "/info/exclude"
)

var commonExcludes = []gitignore.Pattern{
	// VCS metadata and OSX cruft. Patterns use a trailing slash (not
	// "/**") so they are unanchored: they hide the directory entry
	// itself, and at any depth, including nested repos/checkouts beneath
	// the workspace root (e.g. the explorer opened in $HOME drilling into
	// src/blue/.git). A "name/**" pattern is root-anchored and matches
	// only the contents of a top-level dir, leaving the entry and every
	// nested copy visible.
	gitignore.ParsePattern(".git/", nil),
	gitignore.ParsePattern(".hg/", nil),
	gitignore.ParsePattern(".svn/", nil),
	gitignore.ParsePattern(".bzr/", nil),
	gitignore.ParsePattern(".DS_Store/", nil),

	// Python tooling / virtualenv / caches
	gitignore.ParsePattern(".venv/", nil),
	gitignore.ParsePattern(".tox/", nil),
	gitignore.ParsePattern(".mypy_cache/", nil),
	gitignore.ParsePattern(".pytest_cache/", nil),
	gitignore.ParsePattern("__pycache__/", nil),
	gitignore.ParsePattern("python_modules/", nil),

	// JS/TS dependencies
	gitignore.ParsePattern("node_modules/", nil),

	// Rust / Cargo build output. Root-anchored because "target" is a
	// generic name: only the workspace-root build dir is excluded, not a
	// user source dir named "target" deeper in the tree.
	gitignore.ParsePattern("/target/", nil),

	// temporary files
	gitignore.ParsePattern("*.swp", nil),
	gitignore.ParsePattern("*"+workspace.SwapFileExtensionName, nil),
	gitignore.ParsePattern("*.sock", nil),
}

// FileReader is the subset of file-system operations needed to load
// .gitignore-derived matchers. Both schemeapi.Scheme and
// workspaceapi.FileSystem satisfy this interface.
type FileReader interface {
	URI(path string) (workspaceapi.URI, error)
	OpenFile(path string, flag int, perm os.FileMode) (workspaceapi.File, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

// LoadGitignore returns a Matcher honoring git's ignore semantics: an
// entry is matched against the .gitignore (and .git/info/exclude) files
// found along its own ancestor directory chain, plus a set of common
// excludes (.git/, node_modules/, *.swp, ...).
//
// Loading is lazy. Construction reads only the workspace root's
// .gitignore/.git/info/exclude so it returns instantly even for huge
// roots such as the user's home directory; each ancestor directory's
// patterns are read and cached the first time a path under it is
// matched. This avoids the unbounded upfront tree walk that previously
// froze the UI when the file explorer opened in $HOME.
func LoadGitignore(cwd FileReader) (Matcher, error) {
	cwduri, err := cwd.URI(".")
	if err != nil {
		return nil, fmt.Errorf("get workspace uri: %w", err)
	}
	// Reading a protected directory to look for .gitignore files is what
	// trips the macOS "access data from other apps" prompt, so the lazy
	// loader must consult this before opening any ancestor's files, and
	// the composed matcher excludes protected dirs outright.
	protected := protectedDirMatcherForBase(filepath.Clean(cwduri.Path()))
	lazy := &lazyGitignoreMatcher{
		cwd:       cwd,
		cwduri:    cwduri,
		protected: protected,
		common:    commonExcludes,
	}
	lazy.load([]string{})
	return AnyMatcher(lazy, protected), nil
}

// lazyGitignoreMatcher matches paths against the .gitignore patterns of
// their ancestor directories, reading and caching each directory's
// patterns on first use. It is safe for concurrent Match/MatchRelPath
// calls (e.g. from walkdir's worker pool).
//
// The cache is a sync.Map: each directory's patterns are written once
// and read on every match of an entry beneath it (a grow-only cache),
// and concurrent walks touch disjoint subtrees — the two access
// patterns sync.Map is optimized for.
type lazyGitignoreMatcher struct {
	cwd       FileReader
	cwduri    workspaceapi.URI
	protected Matcher
	common    []gitignore.Pattern

	// cache maps a directory key (filepath.Join of its path components,
	// "." for the root) to its []gitignore.Pattern. A stored nil value
	// memoizes directories with no ignore files (or protected ones) so
	// they are never reopened.
	cache sync.Map
}

func (m *lazyGitignoreMatcher) Match(uri workspaceapi.URI, isDir bool) bool {
	return m.MatchRelPath(workspaceapi.RelPath(m.cwduri, uri), isDir)
}

func (m *lazyGitignoreMatcher) MatchRelPath(relpath string, isDir bool) bool {
	comps := splitPath(relpath)
	patterns := make([]gitignore.Pattern, 0, len(m.common))
	patterns = append(patterns, m.common...)
	for _, dir := range ancestorDirs(comps) {
		patterns = append(patterns, m.load(dir)...)
	}
	return gitignore.NewMatcher(patterns).Match(comps, isDir)
}

// load returns the patterns declared by the directory identified by the
// given path components (empty for the workspace root), reading them once
// and caching the result (including empty results, to memoize misses).
// Directories excluded by the protected matcher are never opened.
func (m *lazyGitignoreMatcher) load(dir []string) []gitignore.Pattern {
	key := filepath.Join(dir...)
	if key == "" {
		key = "."
	}

	if cached, ok := m.cache.Load(key); ok {
		return cached.([]gitignore.Pattern)
	}

	loaded := []gitignore.Pattern{}
	if len(dir) == 0 || m.protected == nil ||
		!m.protected.MatchRelPath(key, true) {
		excludePs, err := readIgnoreFile(m.cwd, dir, infoExcludeFile)
		if err != nil {
			m.logReadError(key, infoExcludeFile, err)
		}
		gitignorePs, err := readIgnoreFile(m.cwd, dir, gitIgnoreFile)
		if err != nil {
			m.logReadError(key, gitIgnoreFile, err)
		}
		loaded = append(loaded, excludePs...)
		loaded = append(loaded, gitignorePs...)
	}

	// A racing goroutine may have loaded the same directory first; reuse
	// its entry so the cache holds a single slice per directory.
	actual, _ := m.cache.LoadOrStore(key, loaded)
	return actual.([]gitignore.Pattern)
}

// ancestorDirs returns the directory path-component slices whose
// .gitignore files govern an entry at comps, ordered from the workspace
// root (nil) down to the entry's immediate parent. The entry itself is
// excluded: a directory's own .gitignore does not decide whether that
// directory is ignored.
func ancestorDirs(comps []string) [][]string {
	dirs := make([][]string, 0, len(comps))
	dirs = append(dirs, []string{})
	for i := 1; i < len(comps); i++ {
		dirs = append(dirs, comps[:i])
	}
	return dirs
}

func splitPath(relpath string) []string {
	return strings.Split(relpath, string(filepath.Separator))
}

func (m *lazyGitignoreMatcher) logReadError(dir, file string, err error) {
	log.WithField(logging.KeyClass, "vctrl.gitignore").Warnf(
		"read %s in %s: %v", file, filepath.Join(m.cwduri.Path(), dir), err)
}

func readIgnoreFile(cwd FileReader, paths []string, file string) (
	patterns []gitignore.Pattern, err error,
) {
	patterns = []gitignore.Pattern{}
	path := filepath.Join(paths...)
	f, err := cwd.OpenFile(filepath.Join(path, file), os.O_RDONLY, 0)
	// ENOTDIR means .git is a gitlink file (submodule/worktree pointer),
	// the normal layout for nested checkouts; treat it like a missing
	// ignore file.
	if err != nil && !errors.Is(err, os.ErrNotExist) &&
		!errors.Is(err, syscall.ENOTDIR) {
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	if err != nil {
		// A missing ignore file is the common case, not an error.
		return patterns, nil
	}

	defer f.Close() // nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := scanner.Text()
		if !strings.HasPrefix(s, gitCommentChar) && len(strings.TrimSpace(s)) > 0 {
			patterns = append(patterns, gitignore.ParsePattern(s, paths))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", file, err)
	}
	return patterns, nil
}
