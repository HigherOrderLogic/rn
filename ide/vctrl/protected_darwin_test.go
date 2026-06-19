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

//go:build darwin

package vctrl

import (
	"context"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

// TestLoadGitignoreExcludesProtectedHomeDirs asserts that LoadGitignore
// excludes the macOS TCC-protected home directories and their descendants
// by default while leaving unprotected dirs visible, so no IDE surface
// trips a system permission prompt.
func TestLoadGitignoreExcludesProtectedHomeDirs(t *testing.T) {
	usr, err := user.Current()
	require.NoError(t, err)
	if usr.HomeDir == "" {
		t.Skip("no home directory for current user")
	}

	uri, err := workspaceapi.ParseURI("file://" + usr.HomeDir)
	require.NoError(t, err)
	cwd, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)

	matcher, err := LoadGitignore(cwd)
	require.NoError(t, err)

	for _, dir := range []string{"Library", "Documents", "Desktop", "Downloads"} {
		uri, err := cwd.URI(dir)
		require.NoError(t, err)
		assert.Truef(t, matcher.Match(uri, true),
			"~/%s must be excluded by default", dir)
	}

	subURI, err := cwd.URI(filepath.Join("Library", "Containers", "com.example.app"))
	require.NoError(t, err)
	assert.True(t, matcher.Match(subURI, true),
		"protected dir descendants must be excluded by default")

	okURI, err := cwd.URI("Projects")
	require.NoError(t, err)
	assert.False(t, matcher.Match(okURI, true),
		"non-protected home dirs must remain visible")
}
