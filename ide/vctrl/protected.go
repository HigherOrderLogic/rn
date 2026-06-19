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

package vctrl

import (
	"path/filepath"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// AnyMatcher returns a Matcher that matches an entry when any of the
// given matchers match it. Nil matchers are ignored.
func AnyMatcher(matchers ...Matcher) Matcher {
	return anyMatcher(matchers)
}

type anyMatcher []Matcher

func (a anyMatcher) Match(file workspaceapi.URI, isDir bool) bool {
	for _, m := range a {
		if m != nil && m.Match(file, isDir) {
			return true
		}
	}
	return false
}

func (a anyMatcher) MatchRelPath(relpath string, isDir bool) bool {
	for _, m := range a {
		if m != nil && m.MatchRelPath(relpath, isDir) {
			return true
		}
	}
	return false
}

// ProtectedDirMatcher returns a Matcher that excludes OS-protected
// directories whose contents must not be read without an explicit user
// grant. On macOS, reading another app's data container under the user's
// ~/Library triggers the system "access data from other apps" (TCC App
// Data) prompt, so traversals must skip it by default. The match is
// home-aware: only the current user's real ~/Library is excluded, not
// workspace folders that happen to be named "Library". Returns
// NopMatcher(false) when no protected roots apply.
func ProtectedDirMatcher(cwd FileReader) Matcher {
	cwduri, err := cwd.URI(".")
	if err != nil {
		return NopMatcher(false)
	}
	return protectedDirMatcherForBase(filepath.Clean(cwduri.Path()))
}

// protectedDirMatcherForBase lets callers that already resolved the
// workspace path avoid a second URI(".") round trip.
func protectedDirMatcherForBase(base string) Matcher {
	roots := protectedRoots()
	if len(roots) == 0 {
		return NopMatcher(false)
	}
	return protectedMatcher{base: base, roots: roots}
}

type protectedMatcher struct {
	base  string
	roots []string
}

func (m protectedMatcher) Match(uri workspaceapi.URI, _ bool) bool {
	return m.matchAbs(filepath.Clean(uri.Path()))
}

func (m protectedMatcher) MatchRelPath(relpath string, _ bool) bool {
	abs := relpath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(m.base, relpath)
	}
	return m.matchAbs(filepath.Clean(abs))
}

func (m protectedMatcher) matchAbs(abs string) bool {
	for _, root := range m.roots {
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
