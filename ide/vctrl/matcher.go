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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// Matcher abstracts the ability to match files against glob patterns.
type Matcher interface {
	Match(file workspaceapi.URI, isDir bool) bool
}

// MatcherFromPatterns returns a matcher that matches files with the given patterns.
func MatcherFromPatterns(
	cwd schemeapi.Scheme, patterns ...gitignore.Pattern,
) (Matcher, error) {
	cwduri, err := cwd.URI(".")
	if err != nil {
		return nil, fmt.Errorf("get workspace uri: %w", err)
	}
	return uriMatcher{
		cwduri:  cwduri,
		matcher: gitignore.NewMatcher(patterns),
	}, nil
}

// NopMatcher returns a Matcher that either always or never matches.
func NopMatcher(match bool) Matcher {
	return nopMatcher{match: match}
}

type uriMatcher struct {
	cwduri  workspaceapi.URI
	matcher gitignore.Matcher
}

func (u uriMatcher) Match(uri workspaceapi.URI, isDir bool) (match bool) {
	relpath := workspaceapi.RelPath(u.cwduri, uri)
	pathcomps := strings.Split(relpath, string(filepath.Separator))
	match = u.matcher.Match(pathcomps, isDir)
	return
}

type nopMatcher struct {
	match bool
}

func (n nopMatcher) Match(uri workspaceapi.URI, isDir bool) bool {
	return n.match
}
