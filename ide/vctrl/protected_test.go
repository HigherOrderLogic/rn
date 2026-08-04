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
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProtectedMatcherHomeAware asserts that the matcher rejects the
// configured roots and their descendants but not prefix-sharing siblings
// or same-named folders elsewhere in the tree.
func TestProtectedMatcherHomeAware(t *testing.T) {
	t.Parallel()
	home := filepath.Join("/Users", "tester")
	library := filepath.Join(home, "Library")
	m := protectedMatcher{base: home, roots: []string{library}}

	cases := []struct {
		relpath string
		want    bool
	}{
		{"Library", true},
		{"Library/Containers", true},
		{"Library/Application Support/SomeApp", true},
		{library, true},
		{filepath.Join(library, "Caches"), true},
		{"Documents", false},
		{"Desktop/screenshot.png", false},
		{"Downloads", false},
		{filepath.Join(home, "Documents"), false},
		{"src/Library", false},
		{"LibraryNotReally", false},
		{"Projects/Documents", false},
		{filepath.Join(home, "DownloadsX"), false},
		{filepath.Join(home, "Projects"), false},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, m.MatchRelPath(tc.relpath, true),
			"MatchRelPath(%q)", tc.relpath)
	}
}

// TestProtectedDirMatcherNoRoots asserts that the matcher never matches
// when no protected roots apply.
func TestProtectedDirMatcherNoRoots(t *testing.T) {
	t.Parallel()
	m := protectedMatcher{base: "/work", roots: nil}
	assert.False(t, m.MatchRelPath("Library", true))
	assert.False(t, m.MatchRelPath("anything", false))
}
