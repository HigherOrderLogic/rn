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
	"net"
	"strconv"

	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"go.uber.org/goleak"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

func assertIteratorEqual(
	t *testing.T, expected []string, it iterator.Iterator[string],
) {
	var actual []string
	for {
		next, ok := it.Next(context.Background())
		if !ok {
			break
		}
		actual = append(actual, next)
	}
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())

	sort.Strings(actual)
	sort.Strings(expected)
	assert.Equal(t, actual, expected)
}

func TestListFiles(t *testing.T) {

	t.Run("lists all files under workspace as relative", func(t *testing.T) {
		defer goleak.VerifyNone(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		for _, path := range []string{".", dir} {
			t.Run(path, func(t *testing.T) {
				uri, err := workspaceapi.CurrentUserHostURI(dir)
				require.NoError(t, err)

				_, err = os.OpenFile(filepath.Join(dir, "a"), os.O_CREATE, 0666)
				require.NoError(t, err)

				_, err = os.OpenFile(filepath.Join(dir, "b"), os.O_CREATE, 0666)
				require.NoError(t, err)

				scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
				require.NoError(t, err)

				it, err := ListFiles(context.Background(), scheme, path)
				assertIteratorEqual(t, []string{"a", "b"}, it)

				require.NoError(t, scheme.Close())
			})
		}
	})

	t.Run("lists only regular files", func(t *testing.T) {
		defer goleak.VerifyNone(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		for _, path := range []string{".", dir} {
			t.Run(path, func(t *testing.T) {
				uri, err := workspaceapi.CurrentUserHostURI(dir)
				require.NoError(t, err)

				_, err = os.OpenFile(filepath.Join(dir, "a"), os.O_CREATE, 0666)
				require.NoError(t, err)

				l, err := net.Listen("unix", filepath.Join(dir, "b"))
				require.NoError(t, err)
				defer l.Close()

				scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
				require.NoError(t, err)

				it, err := ListFiles(context.Background(), scheme, path)
				assertIteratorEqual(t, []string{"a"}, it)

				require.NoError(t, scheme.Close())
			})
		}
	})

	t.Run("Close before scanning all should abort", func(t *testing.T) {
		defer goleak.VerifyNone(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		uri, err := workspaceapi.CurrentUserHostURI(dir)
		require.NoError(t, err)

		const n = 1000
		for i := 0; i < n; i++ {
			subdir := filepath.Join(dir, strconv.Itoa(i))
			require.NoError(t, os.MkdirAll(subdir, 0777))
			_, err = os.OpenFile(filepath.Join(subdir, "a"), os.O_CREATE, 0666)
			require.NoError(t, err)
		}

		scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
		require.NoError(t, err)

		it, err := ListFiles(context.Background(), scheme, dir)
		require.NoError(t, it.Close())

		require.NoError(t, scheme.Close())
	})

	t.Run("lists all files under non-workspace dir as absolute", func(t *testing.T) {
		defer goleak.VerifyNone(t)

		workspaceDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(workspaceDir)
		})

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.CurrentUserHostURI(workspaceDir)
		require.NoError(t, err)

		f1, err := os.OpenFile(filepath.Join(dir, "a"), os.O_CREATE, 0666)
		require.NoError(t, err)

		f2, err := os.OpenFile(filepath.Join(dir, "b"), os.O_CREATE, 0666)
		require.NoError(t, err)

		scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
		require.NoError(t, err)

		it, err := ListFiles(context.Background(), scheme, dir)
		assertIteratorEqual(t, []string{f1.Name(), f2.Name()}, it)
		require.NoError(t, scheme.Close())
	})
}

func TestFileSchemeListFilesLarge(t *testing.T) {
	defer goleak.VerifyNone(t)

	const n = 100

	workspaceURI, closeFn := setupTestDirectory(t, n, 10, 10)
	defer closeFn()

	scheme, err := workspace.NewFileScheme(context.Background(),
		config.NopConfig(), workspaceURI)
	require.NoError(t, err)

	it, err := ListFiles(context.Background(), scheme, "")
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		path, ok := it.Next(context.Background())
		require.True(t, ok)
		require.NoError(t, it.Err())
		assert.NotZero(t, path)
		assert.False(t, filepath.IsAbs(path))
	}

	path, ok := it.Next(context.Background())
	require.False(t, ok)
	assert.NoError(t, it.Err())
	assert.Zero(t, path)
	require.NoError(t, scheme.Close())
}

func BenchmarkListFilesTinyDir(b *testing.B) {
	benchListFiles(b, 5, 5, 5)
}

func BenchmarkListFilesSmallDirShallow(b *testing.B) {
	benchListFiles(b, 50, 5, 1)
}

func BenchmarkListFilesSmallDirDeep(b *testing.B) {
	benchListFiles(b, 50, 1, 1)
}

func BenchmarkListFilesLargeDirDeep(b *testing.B) {
	benchListFiles(b, 500, 10, 1)
}

func BenchmarkListFilesLargeDirShallow(b *testing.B) {
	benchListFiles(b, 500, 100, 1)
}

func BenchmarkListFilesHugeDirDeep(b *testing.B) {
	benchListFiles(b, 5000, 100, 1)
}

func BenchmarkListFilesHugeDirShallow(b *testing.B) {
	benchListFiles(b, 5000, 1000, 1)
}

func BenchmarkListFilesUberDir(b *testing.B) {
	benchListFiles(b, 500000, 10000, 1)
}

func BenchmarkListFilesLotsEmptyDir(b *testing.B) {
	benchListFiles(b, 500, 100, 100)
}

func benchListFiles(b *testing.B, totalFiles, nestEvery, emptyDirsPerFile int) {
	workspaceURI, closeFn := setupTestDirectory(b, totalFiles, nestEvery, emptyDirsPerFile)

	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), workspaceURI)
	if err != nil {
		b.Fatalf("error: %s", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it, _ := ListFiles(context.Background(), scheme, "")

		// consume iterator
		ok := true
		for ok {
			_, ok = it.Next(context.Background())
		}
	}
	b.StopTimer()
	closeFn()
	_ = scheme.Close()
}

func setupTestDirectory(
	t testing.TB, totalFiles, nestEvery, emptyDirsPerFile int,
) (workspaceapi.URI, func()) {
	dir, err := os.MkdirTemp("", "list_files_test")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	workspaceURI, err := workspaceapi.ParseURI("file://" + dir)
	require.NoError(t, err)

	var closeFns []func()
	// write test files
	for i := 0; i < totalFiles; i++ {
		f, err := os.CreateTemp(dir, strconv.Itoa(i))
		require.NoError(t, err)

		_, err = f.WriteString(strconv.Itoa(i))
		require.NoError(t, err)

		err = f.Close()
		require.NoError(t, err)

		for i := 0; i < emptyDirsPerFile; i++ {
			// create more dirs than workers
			d, err := os.MkdirTemp(dir, "emptydir")
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = os.RemoveAll(d)
			})
		}
		if i%nestEvery == 0 {
			// nest next temp file created
			dir, err = os.MkdirTemp(dir, "nested")
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = os.RemoveAll(dir)
			})
		}
		closeFns = append(closeFns, func() { os.Remove(f.Name()) })
	}
	return workspaceURI, func() {
		for _, closeFn := range closeFns {
			closeFn()
		}
	}
}
