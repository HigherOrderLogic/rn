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

package symbolresolve_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"

	"unstable.build/go-tui/ide/idelsp/symbolresolve"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/walkdir"
)

func TestDetectSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  []*symbolresolve.Spec
	}{
		{
			name:  "go only",
			files: []string{"main.go", "pkg/util.go"},
			want:  []*symbolresolve.Spec{symbolresolve.Go},
		},
		{
			name:  "python only",
			files: []string{"app.py", "lib/helpers.py"},
			want:  []*symbolresolve.Spec{symbolresolve.Python},
		},
		{
			name:  "rust only",
			files: []string{"main.rs", "src/lib.rs"},
			want:  []*symbolresolve.Spec{symbolresolve.Rust},
		},
		{
			name:  "mixed go and python",
			files: []string{"main.go", "app.py"},
			want:  []*symbolresolve.Spec{symbolresolve.Go, symbolresolve.Python},
		},
		{
			name:  "no recognized languages",
			files: []string{"README.md", "data.json"},
			want:  nil,
		},
		{
			name:  "empty workspace",
			files: nil,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fs := newDetectFS(t, tt.files)
			got, err := iterator.ToSlice(
				context.Background(),
				symbolresolve.DetectSpecs(context.Background(), fs),
			)
			require.NoError(t, err)
			wantIDs := make([]string, len(tt.want))
			for i, spec := range tt.want {
				wantIDs[i] = spec.LangID
			}
			gotIDs := make([]string, len(got))
			for i := range got {
				gotIDs[i] = got[i].LangID
			}
			if len(wantIDs) == 0 {
				assert.Empty(t, gotIDs)
				return
			}
			assert.ElementsMatch(t, wantIDs, gotIDs)
		})
	}
}

// TestDetectSpecsHonorsContextFilter verifies DetectSpecs respects a
// walkdir.Filter installed on the context: source files that live only
// inside excluded dependency/build directories (.venv, target) must not
// trigger language detection, while real source under src/ still does.
func TestDetectSpecsHonorsContextFilter(t *testing.T) {
	t.Parallel()

	fs := newDetectFS(t, []string{
		"src/main.go",
		".venv/lib/app.py",
		"target/debug/build.rs",
	})

	matcher, err := vctrl.LoadGitignore(fs)
	require.NoError(t, err)
	ctx := walkdir.WithContextFilter(context.Background(), matcher)

	got, err := iterator.ToSlice(ctx, symbolresolve.DetectSpecs(ctx, fs))
	require.NoError(t, err)

	gotIDs := make([]string, len(got))
	for i := range got {
		gotIDs[i] = got[i].LangID
	}
	assert.ElementsMatch(t, []string{symbolresolve.Go.LangID}, gotIDs)
}

func newDetectFS(t *testing.T, files []string) workspaceapi.FileSystem {
	t.Helper()
	root := t.TempDir()
	for _, rel := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("// content\n"), 0o644))
	}
	uri, err := workspaceapi.ParseURI("file://" + root)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	t.Cleanup(func() { _ = scheme.Close() })
	return scheme
}
