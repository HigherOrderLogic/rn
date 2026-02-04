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

package walkdir

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

type readLinesTestFile struct {
	fullPath string
	content  string
}

func TestReadLines(t *testing.T) {
	tsuite := []struct {
		desc    string
		inFiles []readLinesTestFile
		wantErr string
		wantOut []string
	}{
		{"no files returns no data", nil, "", nil},
		{"empty files returns no data", []readLinesTestFile{{"a", ""}, {"b", ""}}, "", nil},
		{"one file returns data", []readLinesTestFile{{"a", "0\n1"}}, "", []string{"a:1:0", "a:2:1"}},
		{"multiple files returns data", []readLinesTestFile{{"a", "4\n5"}, {"b", "0\n1"}}, "",
			[]string{"a:1:4", "a:2:5", "b:1:0", "b:2:1"}},
		{"more files than workers", []readLinesTestFile{
			{"a", "4\n5"}, {"b", "0\n1"}, {"c", ""}, {"d", ""}, {"e", ""}, {"f", ""}, {"g", ""},
			{"z", "4\n5"}, {"y", "0\n1"}, {"x", ""}, {"w", ""}, {"s", ""}, {"r", ""}, {"n", ""},
			{"x1", ""}, {"x2", ""}, {"x3", ""}, {"x4", ""}, {"x5", ""},
			{"xx1", ""}, {"xx2", ""}, {"xx3", ""}, {"xx4", ""}, {"xx5", ""},
			{"xxx1", ""}, {"xxx2", ""}, {"xxx3", ""}, {"xxx4", ""}, {"xxx5", ""},
			{"xxxx1", ""}, {"xxxx2", ""}, {"xxxx3", ""}, {"xxxx4", ""}, {"xxxx5", ""},
			{"xxxxx1", ""}, {"xxxxx1x2", ""}, {"xxxxx1x3", ""}, {"xxxxx1x4", ""}, {"xxxxx1x5", ""},
		}, "", []string{
			"a:1:4", "a:2:5", "b:1:0", "b:2:1",
			"y:1:0", "y:2:1", "z:1:4", "z:2:5",
		}},
		{"invalid utf-8 character", []readLinesTestFile{{"a", "a\xc5z"}}, "",
			[]string{"a:1:a\xc5z"}},
		{"skips binary files", []readLinesTestFile{{"a", "\x00\x01..."}}, "", nil},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			uri, err := workspaceapi.ParseURI("memory:///")
			require.NoError(t, err)
			scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
			require.NoError(t, err)
			workspace := workspace.NewSchemeWorkspace(uri, scheme)

			for _, file := range tcase.inFiles {
				f, werr := scheme.OpenFile(file.fullPath, os.O_CREATE, 0)
				require.Nil(t, werr)
				_, err = f.Write([]byte(file.content))
				require.NoError(t, err)
				require.NoError(t, f.Sync())
				require.NoError(t, f.Close())
			}
			itIn, err := ListFiles(context.Background(), scheme, "")
			require.NoError(t, err)

			// sut
			itOut, err := ReadLines(context.Background(), workspace, itIn)
			if tcase.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tcase.wantErr)
				return
			}
			require.NoError(t, err)
			var lines []string
			for {
				line, ok := itOut.Next(context.Background())
				if !ok {
					require.NoError(t, itOut.Err())
					break
				}
				lines = append(lines, line)
			}
			// order doesn't matter and the iterator is unordered
			sort.Strings(lines)
			assert.Equal(t, tcase.wantOut, lines)
			assert.NoError(t, itOut.Close())
		})
	}
}
