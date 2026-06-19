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

	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
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
	gitignore.ParsePattern(".git/**", nil),
	gitignore.ParsePattern(".hg/**", nil),
	gitignore.ParsePattern(".svn/**", nil),
	gitignore.ParsePattern(".bzr/**", nil),

	// OSX
	gitignore.ParsePattern(".DS_Store/**", nil),

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

// LoadGitignore load all workspaces' .gitignore files recursively
// and returns a Matcher that matches against all loaded patterns,
// plus adds some common excludes like .swp files or .git/** directory.
func LoadGitignore(cwd FileReader) (Matcher, error) {
	cwduri, err := cwd.URI(".")
	if err != nil {
		return nil, fmt.Errorf("get workspace uri: %w", err)
	}
	// The walk itself must skip protected dirs, not only the returned
	// matcher: reading them to look for .gitignore files is what trips the
	// macOS "access data from other apps" prompt.
	protected := protectedDirMatcherForBase(filepath.Clean(cwduri.Path()))
	excludes, err := loadGitignoreRecursively(cwd, protected, nil)
	if err != nil {
		return nil, err
	}
	return matcherFromPatternsURI(cwduri, append(excludes, commonExcludes...)...), nil
}

func loadGitignoreRecursively(cwd FileReader, protected Matcher, path []string) (
	ps []gitignore.Pattern, err error,
) {
	ps, _ = readIgnoreFile(cwd, path, infoExcludeFile)

	subps, _ := readIgnoreFile(cwd, path, gitIgnoreFile)
	ps = append(ps, subps...)

	var fis []fs.DirEntry
	fis, err = cwd.ReadDir(filepath.Join(path...))
	if err != nil {
		return
	}

	for _, fi := range fis {
		if fi.IsDir() && fi.Name() != gitDir {
			child := append(path, fi.Name())
			if protected != nil &&
				protected.MatchRelPath(filepath.Join(child...), true) {
				continue
			}
			if gitignore.NewMatcher(ps).Match(child, true) {
				continue
			}

			var subps []gitignore.Pattern
			subps, err = loadGitignoreRecursively(cwd, protected, child)
			if err != nil {
				return
			}

			if len(subps) > 0 {
				ps = append(ps, subps...)
			}
		}
	}

	return
}

func readIgnoreFile(cwd FileReader, paths []string, file string) (
	patterns []gitignore.Pattern, err error,
) {
	path := filepath.Join(paths...)
	f, err := cwd.OpenFile(filepath.Join(path, file), os.O_RDONLY, 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	if err != nil {
		return
	}

	defer f.Close() // nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := scanner.Text()
		if !strings.HasPrefix(s, gitCommentChar) && len(strings.TrimSpace(s)) > 0 {
			patterns = append(patterns, gitignore.ParsePattern(s, paths))
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("scan %s: %w", gitIgnoreFile, err)
	}
	return
}
