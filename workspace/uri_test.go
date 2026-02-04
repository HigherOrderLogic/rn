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

package workspace

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

func TestDefaultSwapDirectory(t *testing.T) {
	tsuite := []struct {
		file    string
		wantDir string
		wantErr bool
	}{
		{"other:///tmp/a.go", "other:///tmp", false},
		{"file:///a.go", "file:///", false},
		{"file:///tmp/a.go", "file:///tmp", false},
		{"file://tmp/a.go", "file://tmp/", false},
		{"ssh://unstable.build/tmp/a.go", "ssh://unstable.build/tmp", false},
	}

	for _, tcase := range tsuite {
		t.Run(fmt.Sprintf("DefaultLocalSwapDirectory of %s", tcase.file), func(t *testing.T) {
			uri, err := workspaceapi.ParseURI(tcase.file)
			require.NoError(t, err)
			out, err := DefaultSwapDirectory(uri)
			if tcase.wantErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tcase.wantDir, out.String())
			}
		})
	}
}

func TestDefaultSwapFile(t *testing.T) {
	tsuite := []struct {
		fileIn       string
		swapDirIn    string
		wantSwapFile string
		wantErr      bool
	}{
		{"other:///tmp/a.go", "other:///tmp", "other:///tmp/.a.go.swp", false},
		{"file:///a.go", "file:///tmp", "file:///tmp/.a.go.swp", false},
		{"file:///a.go", "file:///", "file:///.a.go.swp", false},
		{"file:///tmp/a.go", "file:///tmp", "file:///tmp/.a.go.swp", false},
		{"file:///tmp/a.go", "file:///", "file:///.a.go.swp", false},
		{"file://./tmp/a.go", "file://./", "file://./.a.go.swp", false},
		{"file://./a.go", "file://./tmp", "file://./tmp/.a.go.swp", false},
		{"ssh:///a.go", "ssh://my_host/tmp", "", true},
		{"ssh://my_host/a.go", "ssh:///tmp", "", true},
		{"ssh://my_host/a.go", "ssh://creepy_host/tmp", "", true},
		{"ssh://unstablebuild@my_host/a.go", "ssh://jj.furman@my_host/tmp", "", true},
		{"ssh://user@my_host/a.go", "ssh://user@my_host/tmp", "ssh://user@my_host/tmp/.a.go.swp", false},
		{"ssh://my_host/a.go", "ssh://my_host/tmp", "ssh://my_host/tmp/.a.go.swp", false},
		{"ssh://my_host/./a.go", "ssh://my_host/./tmp", "ssh://my_host/tmp/.a.go.swp", false},
	}

	for i, tcase := range tsuite {
		desc := fmt.Sprintf("DefaultLocalSwapFile %d of %s", i, tcase.fileIn)
		t.Run(desc, func(t *testing.T) {
			uri, err := workspaceapi.ParseURI(tcase.fileIn)
			require.NoError(t, err)
			swapUri, err := workspaceapi.ParseURI(tcase.swapDirIn)
			require.NoError(t, err)

			// sut
			out, err := DefaultSwapFile(swapUri, uri)

			if tcase.wantErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tcase.wantSwapFile, out.String())
			}
		})
	}
}

func TestIsWorkspaceURI(t *testing.T) {
	tsuite := []struct {
		workspaceURI string
		uri          string
		expectedOut  bool
	}{
		{"file:///", "file:///tmp", true},
		{"file:///tmp", "file:///tmp", true},
		{"file:///var", "file:///tmp/file", true}, // different folder but workspace can handle it
		{"file:///var", "file:///var/file", true},
		{"file:///var", "file:///var/dir/dir/dir/file", true},
		{"file:///var/", "file:///var/file", true},
		{"file:///", "ssh:///tmp", false},
	}

	for i, tcase := range tsuite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			inURI, err := workspaceapi.ParseURI(tcase.uri)
			require.NoError(t, err)

			inWorkspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			fileScheme, err := newTestFileScheme(inWorkspaceURI)
			require.NoError(t, err)
			inWorkspace := NewSchemeWorkspace(inWorkspaceURI, fileScheme)

			// sut
			actualOut, err := IsWorkspaceURI(inWorkspace, inURI)
			require.NoError(t, err)
			assert.Equal(t, tcase.expectedOut, actualOut)
		})
	}
}
