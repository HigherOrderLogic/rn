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
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

func TestCreateTemp(t *testing.T) {
	suite := []struct {
		description string
		dir         string
		pattern     string
		expectErr   string
		assert      func(*testing.T, workspaceapi.File)
	}{
		{"returns error if path is passed in pattern", "", "a/b", "pattern cannot contain separator", nil},
		{"uses dir if passed", "/stomp", "", "", func(t *testing.T, file workspaceapi.File) {
			require.Equal(t, "/stomp", filepath.Dir(file.Name()), file.Name())
		}},
		{"uses pattern if passed", "/stomp", "sup_*_bla", "", func(t *testing.T, file workspaceapi.File) {
			require.True(t, strings.HasPrefix(file.Name(), "/stomp/sup_"))
			require.True(t, strings.HasSuffix(file.Name(), "_bla"))
		}},
		{"uses TMPDIR env var if set", "/stomp", "sup_*_bla", "", func(t *testing.T, file workspaceapi.File) {
			require.True(t, strings.HasPrefix(file.Name(), "/stomp/sup_"))
			require.True(t, strings.HasSuffix(file.Name(), "_bla"))
		}},
	}

	for _, test := range suite {
		ctx := context.Background()
		cfg := config.NopConfig()
		uri, err := workspaceapi.ParseURI("memory:///a")
		require.NoError(t, err)
		t.Run(test.description, func(t *testing.T) {
			memfs, err := NewMemoryScheme(ctx, cfg, uri)
			require.NoError(t, err)
			for range 1000 {
				file, err := CreateTemp(memfs, test.dir, test.pattern)
				if test.expectErr != "" {
					require.Error(t, err)
					require.True(t, strings.Contains(err.Error(), test.expectErr), err.Error())
				} else {
					require.NoError(t, err)
					actual, err := memfs.Open(file.Name())
					require.NoError(t, err)
					require.Equal(t, file.Name(), actual.Name())
					require.NoError(t, actual.Close())
					if test.assert != nil {
						test.assert(t, file)
					}
				}
				if err == nil {
					require.NoError(t, file.Close())
				}
			}
		})
	}
}
