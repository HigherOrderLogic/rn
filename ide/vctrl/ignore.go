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
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
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
	gitignore.ParsePattern("*.sock", nil),
}

// LoadGitignore load all workspaces' .gitignore files recursively
// and returns a Matcher that matches against all loaded patterns,
// plus adds some common excludes like .swp files or .git/** directory.
func LoadGitignore(cwd schemeapi.Scheme) (Matcher, error) {
	excludes, err := loadGitignoreRecursively(cwd, nil)
	if err != nil {
		return nil, err
	}
	return MatcherFromPatterns(cwd, append(excludes, commonExcludes...)...)
}

func loadGitignoreRecursively(cwd schemeapi.Scheme, path []string) (
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
			if gitignore.NewMatcher(ps).Match(append(path, fi.Name()), true) {
				continue
			}

			var subps []gitignore.Pattern
			subps, err = loadGitignoreRecursively(cwd, append(path, fi.Name()))
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

func readIgnoreFile(cwd schemeapi.Scheme, paths []string, file string) (
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
			patterns = append(patterns, gitignore.ParsePattern(s, nil))
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("scan %s: %w", gitIgnoreFile, err)
	}
	return
}
