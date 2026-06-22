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

package idepkg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"archive/tar"
	"bytes"
	"compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/document/docmarshal/docbson"
	"github.com/unstablebuild/blue/document/docmarshal/doctoml"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/blue/release/cdnrelease"
	"github.com/unstablebuild/ox-api/bluestore"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/handler/handlertest"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"gopkg.in/yaml.v3"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
	"unstable.build/go-tui/ide/starlarkconfig"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/workspace/walkdir"
)

func TestLibDir(t *testing.T) {
	t.Parallel()
	t.Run("returns installed files", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		it, err := m.LibDir(context.Background(), "go")
		require.NoError(t, err)
		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		// test that paths are absolute
		actual := make([]string, 0)
		for _, path := range installed {
			require.True(t, filepath.IsAbs(path))
			actual = append(actual, filepath.Base(path))
		}
		expected := []string{
			"highlights.scm",
			"tags.scm",
			"indents.scm",
			"tree-sitter.so",
			"go",
			"gofmt",
			"goimports",
			"gopls",
		}
		assert.ElementsMatch(t, expected, actual)
	})
	t.Run("if install started, returned iterator blocks until package is done installing", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		it, err := m.LibDir(context.Background(), "go")
		require.NoError(t, err)
		actual, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.NotEmpty(t, actual)
	})
	t.Run("multiple cals to LibDir while installing return complete iterators", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		it1, err := m.LibDir(context.Background(), "go")
		require.NoError(t, err)

		it2, err := m.LibDir(context.Background(), "go")
		require.NoError(t, err)

		it3, err := m.LibDir(context.Background(), "go")
		require.NoError(t, err)

		for _, it := range []iterator.Iterator[string]{it1, it2, it3} {
			files, err := iterator.ToSlice(context.Background(), it)
			require.NoError(t, err)
			var actual []string
			for _, file := range files {
				actual = append(actual, filepath.Base(file))
			}
			expected := []string{
				"highlights.scm", "tags.scm", "indents.scm",
				"tree-sitter.so", "go",
				"gofmt", "goimports", "gopls",
			}
			assert.ElementsMatch(t, expected, actual)
		}
	})
	t.Run("returns error if package is not installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.LibDir(context.Background(), "go")
		require.Equal(t, ErrNotInstalled, err)
	})
	t.Run("survives manager restart with reconcile", func(t *testing.T) {
		t.Parallel()
		type storageFactory struct {
			name string
			make func() storageapi.Service
		}
		factories := []storageFactory{
			{"bson", func() storageapi.Service {
				return storagestub.NewInMemoryServiceWithMarshaler(docbson.Marshaler())
			}},
			{"toml", func() storageapi.Service {
				return storagestub.NewInMemoryServiceWithMarshaler(doctoml.Marshaler())
			}},
			{"toml_partitioned", func() storageapi.Service {
				return storageapi.WithPartition(
					storagestub.NewInMemoryServiceWithMarshaler(doctoml.Marshaler()), "idepkg")
			}},
		}
		for _, sf := range factories {
			t.Run(sf.name, func(t *testing.T) {
				t.Parallel()
				pkgs := idepkgtest.MakePackages()
				versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})

				temp, err := os.MkdirTemp("", "")
				require.NoError(t, err)
				t.Cleanup(func() { _ = os.RemoveAll(temp) })

				configPath := filepath.Join(temp, "config.yaml")
				storage := sf.make()
				fileScheme := newLocalScheme(temp)
				wm := &mockWindowManager{
					floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
						h.Resize(70, 20)
						h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
						return &mockWindow{}, nil
					},
				}

				// First Manager: install a package.
				n1 := idepkgtest.NewNotifications(t)
				rm := idepkgtest.NewReleaseManager(pkgs, versions)
				m1 := NewManager(n1, rm, storage,
					fileScheme, temp, configPath, wm, syncTick, term.NopInterrupter())

				err = m1.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
				require.NoError(t, err)
				n1.RequireNoErrorNotification()

				// Sanity: LibDir works on the first manager.
				it, err := m1.LibDir(context.Background(), "go")
				require.NoError(t, err)
				files, err := iterator.ToSlice(context.Background(), it)
				require.NoError(t, err)
				require.NotEmpty(t, files)

				// Second Manager: same storage and dataDir, simulating a restart.
				n2 := idepkgtest.NewNotifications(t)
				m2 := NewManager(n2, rm, storage,
					fileScheme, temp, configPath, wm, syncTick, term.NopInterrupter())

				err = m2.Reconcile(context.Background())
				require.NoError(t, err)

				// LibDir must still work after restart + reconcile.
				it, err = m2.LibDir(context.Background(), "go")
				require.NoError(t, err)
				files, err = iterator.ToSlice(context.Background(), it)
				require.NoError(t, err)
				require.NotEmpty(t, files)
			})
		}
	})
}

func TestDescribePackage(t *testing.T) {
	t.Parallel()
	t.Run("no packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.DescribePackage(context.Background(), "myPkg")
		assert.EqualError(t, err, "not found")
	})
	t.Run("returns error if package id is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.DescribePackage(context.Background(), "")
		require.Error(t, err)
	})
	t.Run("returns package", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		pkg, err := m.DescribePackage(context.Background(), "go")
		require.NoError(t, err)
		assert.Equal(t, release.Package{Name: "go"}, pkg)
	})
	t.Run("bubbles up release manager error", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles()
		m, _, r, _ := newTestManager(t, pkgs, versions)

		r.ExpectReturnErr(errors.New("boom"))
		_, err := m.DescribePackage(context.Background(), "go")
		assert.EqualError(t, err, "boom")
	})
}

func TestDescribeRelease(t *testing.T) {
	t.Parallel()
	t.Run("no releases", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.DescribeRelease(context.Background(), "myPkg", "notFoundRelease")
		assert.EqualError(t, err, "not found")
	})
	t.Run("returns error if package id is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "", Version: "m"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.DescribeRelease(context.Background(), "", "m")
		require.Error(t, err)
	})
	t.Run("returns error if release id is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: ""}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.DescribeRelease(context.Background(), "go", "")
		require.Error(t, err)
	})
	t.Run("returns release bundle", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "m"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		pkg, err := m.DescribeRelease(context.Background(), "go", "m")
		require.NoError(t, err)
		assert.Equal(t, release.Bundle{Package: "go", Version: "m"}, pkg)
	})
	t.Run("bubbles up release manager error", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "m"}})
		m, _, r, _ := newTestManager(t, pkgs, versions)

		r.ExpectReturnErr(errors.New("boom"))
		_, err := m.DescribeRelease(context.Background(), "go", "m")
		assert.EqualError(t, err, "boom")
	})
}

func TestListPackages(t *testing.T) {
	t.Parallel()
	t.Run("no packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		it, err := m.ListPackages(context.Background(), nil)
		require.NoError(t, err)

		_, empty := iterator.IsEmpty(context.Background(), it)
		assert.True(t, empty)
	})
	t.Run("returns packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"}, release.Package{Name: "ox"})
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		it, err := m.ListPackages(context.Background(), nil)
		require.NoError(t, err)

		actual, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.ElementsMatch(t, []release.Package{{Name: "go"}, {Name: "ox"}}, actual)
	})
	t.Run("bubbles up release manager error", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles()
		m, _, r, _ := newTestManager(t, pkgs, versions)

		r.ExpectReturnErr(errors.New("boom"))
		_, err := m.ListPackages(context.Background(), nil)
		assert.EqualError(t, err, "boom")
	})
}

func TestListPackageVersions(t *testing.T) {
	t.Parallel()
	t.Run("no packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.ListPackageVersions(context.Background(), "go", nil)
		require.Error(t, err)
		assert.EqualError(t, err, "not found")
	})
	t.Run("returns error if package id is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.ListPackageVersions(context.Background(), "", nil)
		require.Error(t, err)
	})
	t.Run("returns packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		it, err := m.ListPackageVersions(context.Background(), "go", nil)
		require.NoError(t, err)

		actual, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.ElementsMatch(t, []release.Bundle{{Package: "go", Version: "1"}}, actual)
	})
	t.Run("bubbles up release manager error", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(release.Package{Name: "go"})
		versions := idepkgtest.MakeBundles()
		m, _, r, _ := newTestManager(t, pkgs, versions)

		r.ExpectReturnErr(errors.New("boom"))
		_, err := m.ListPackageVersions(context.Background(), "go", nil)
		assert.EqualError(t, err, "boom")
	})
}

// TestTranslateReleaseErrors verifies that the wrapper methods
// translate *cdnrelease.StatusError responses into friendly
// sentinel-wrapped errors that callers can match with errors.Is.
// Other (non-StatusError) errors must pass through unchanged.
func TestTranslateReleaseErrors(t *testing.T) {
	t.Parallel()

	notFound := &cdnrelease.StatusError{
		URL:    "https://example/api/releases/darwin-arm64/packages/go",
		Status: http.StatusNotFound,
	}
	serverErr := &cdnrelease.StatusError{
		URL:    "https://example/api/releases/darwin-arm64/packages",
		Status: http.StatusBadGateway,
	}
	downloadErr := &cdnrelease.StatusError{
		// Empty URL marks the signed-URL data download branch.
		Status: http.StatusForbidden,
	}

	t.Run("DescribePackage 404 -> ErrPackageNotFound", func(t *testing.T) {
		t.Parallel()
		m, _, r, _ := newTestManager(t,
			idepkgtest.MakePackages(release.Package{Name: "go"}),
			idepkgtest.MakeBundles())
		r.ExpectReturnErr(notFound)

		_, err := m.DescribePackage(context.Background(), "go")
		require.ErrorIs(t, err, ErrPackageNotFound)
		assert.Contains(t, err.Error(), `package "go" does not exist`)
	})

	t.Run("ListPackages 5xx -> ErrServerUnavailable", func(t *testing.T) {
		t.Parallel()
		m, _, r, _ := newTestManager(t,
			idepkgtest.MakePackages(), idepkgtest.MakeBundles())
		r.ExpectReturnErr(serverErr)

		_, err := m.ListPackages(context.Background(), nil)
		require.ErrorIs(t, err, ErrServerUnavailable)
		assert.Contains(t, err.Error(), "status 502")
	})

	t.Run("ListPackageVersions 404 -> ErrPackageNotFound", func(t *testing.T) {
		t.Parallel()
		m, _, r, _ := newTestManager(t,
			idepkgtest.MakePackages(release.Package{Name: "go"}),
			idepkgtest.MakeBundles())
		r.ExpectReturnErr(notFound)

		_, err := m.ListPackageVersions(context.Background(), "go", nil)
		require.ErrorIs(t, err, ErrPackageNotFound)
		assert.Contains(t, err.Error(), `package "go" does not exist`)
	})

	t.Run("DescribeRelease 404 -> ErrVersionNotFound", func(t *testing.T) {
		t.Parallel()
		m, _, r, _ := newTestManager(t,
			idepkgtest.MakePackages(release.Package{Name: "go"}),
			idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}}))
		r.ExpectReturnErr(notFound)

		_, err := m.DescribeRelease(context.Background(), "go", "1")
		require.ErrorIs(t, err, ErrVersionNotFound)
		assert.Contains(t, err.Error(), `version "1" of package "go" does not exist`)
	})

	t.Run("DescribeRelease signed-URL failure -> ErrServerUnavailable", func(t *testing.T) {
		t.Parallel()
		m, _, r, _ := newTestManager(t,
			idepkgtest.MakePackages(release.Package{Name: "go"}),
			idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}}))
		r.ExpectReturnErr(downloadErr)

		_, err := m.DescribeRelease(context.Background(), "go", "1")
		require.ErrorIs(t, err, ErrServerUnavailable)
		assert.Contains(t, err.Error(), `download of "go" version "1" failed`)
	})

	t.Run("non-StatusError passes through", func(t *testing.T) {
		t.Parallel()
		m, _, r, _ := newTestManager(t,
			idepkgtest.MakePackages(release.Package{Name: "go"}),
			idepkgtest.MakeBundles())
		r.ExpectReturnErr(errors.New("boom"))

		_, err := m.DescribePackage(context.Background(), "go")
		assert.EqualError(t, err, "boom")
	})
}

func TestInstallPackageVersion(t *testing.T) {
	t.Parallel()
	t.Run("returns error if pkg is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)
		err := m.InstallPackageVersion(context.Background(), "", "1", repl.NopProgressWriter())
		assert.Error(t, err)
	})

	t.Run("returns error if version is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)
		err := m.InstallPackageVersion(context.Background(), "go", "", repl.NopProgressWriter())
		assert.Error(t, err)
	})

	t.Run("installs a package with executables", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})

	t.Run("install a package and version already installed fails", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		err = m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		assert.Error(t, err)
	})

	t.Run("installs a package without executables", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "testpkg", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "testpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertDataDirExists(t, datadir, "testpkg")
		assertExecutables(t, datadir)
	})

	t.Run("install errors are retryable", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, r, datadir := newTestManager(t, pkgs, versions)

		r.ExpectReturnErr(errors.New("boom"))
		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.Error(t, err)

		r.ExpectReturnErr(nil)
		n.Reset()
		err = m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
	t.Run("notification progress is completed, even if writer doesn't", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, r, datadir := newTestManager(t, pkgs, versions)
		r.SetMissProgressComplete(true)

		pw := &recordingProgressWriter{}
		err := m.InstallPackageVersion(context.Background(), "go", "1", pw)
		require.NoError(t, err)
		n.RequireNoErrorNotification()
		assert.True(t, pw.completed(),
			"progress writer should be driven to completion even if the "+
				"release manager omits the final progress sample")

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
}

func TestListInstalledPackageVersions(t *testing.T) {
	t.Parallel()
	t.Run("returns installed packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		it, err := m.ListInstalledPackages(context.Background())
		require.NoError(t, err)

		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"go"}, installed)
	})
	t.Run("deduplicates when multiple versions are installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		it, err := m.ListInstalledPackages(context.Background())
		require.NoError(t, err)

		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.Equal(t, []string{"go"}, installed)
	})
	t.Run("returns nothing if there are no packages installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		it, err := m.ListInstalledPackages(context.Background())
		require.NoError(t, err)

		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Empty(t, installed)
	})
}

func TestListInstalledPackages(t *testing.T) {
	t.Parallel()
	t.Run("returns installed packages", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		it, err := m.ListInstalledPackageVersions(context.Background(), "go")
		require.NoError(t, err)

		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.ElementsMatch(t, []release.Version{"1"}, installed)
	})
	t.Run("returns nothing if there are no packages installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)

		it, err := m.ListInstalledPackageVersions(context.Background(), "go")
		require.NoError(t, err)

		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Empty(t, installed)
	})
}

func TestPackageVersionInUse(t *testing.T) {
	t.Parallel()
	t.Run("returns installed package version in use", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		actual, err := m.PackageVersionInUse(context.Background(), "go")
		require.NoError(t, err)

		assert.Equal(t, release.Version("1"), actual)
	})

	t.Run("returns latest installed package version", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		actual, err := m.PackageVersionInUse(context.Background(), "go")
		require.NoError(t, err)

		assert.Equal(t, release.Version("2"), actual)
	})

	t.Run("returns package version in use, after UsePackageVersion", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.UsePackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)

		actual, err := m.PackageVersionInUse(context.Background(), "go")
		require.NoError(t, err)

		assert.Equal(t, release.Version("1"), actual)
	})

	t.Run("returns error if package is not installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		_, err := m.PackageVersionInUse(context.Background(), "go")
		require.Error(t, err)
	})
}

func TestUsePackageVersion(t *testing.T) {
	t.Parallel()
	t.Run("returns error if package is not installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.UsePackageVersion(context.Background(), "go", "1")
		require.Error(t, err)
	})

	t.Run("returns error if version is not installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.UsePackageVersion(context.Background(), "go", "2")
		require.Error(t, err)
	})

	t.Run("relinks executables and lib", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		require.NoError(t, os.RemoveAll(filepath.Join(datadir, "bin", "go")))
		require.NoError(t, os.RemoveAll(filepath.Join(datadir, "lib", "go")))

		err = m.UsePackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
	t.Run("returns error if version already in use", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.UsePackageVersion(context.Background(), "go", "2")
		require.Equal(t, err, ErrVersionInUse)
	})
}

func TestDeletePackage(t *testing.T) {
	t.Parallel()
	t.Run("deletes all versions of an installed package", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.DeletePackage(context.Background(), "go")
		require.NoError(t, err)

		assertDataDirNotExists(t, datadir, "go")
		assertExecutables(t, datadir /* none */)
	})

	t.Run("returns ErrNotInstalled if package is not installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.DeletePackage(context.Background(), "testpkg")
		require.Equal(t, ErrNotInstalled, err)
	})
}

func TestDeletePackageVersion(t *testing.T) {
	t.Parallel()
	t.Run("deletes a version of an installed package", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.DeletePackageVersion(context.Background(), "go", "1", false)
		require.NoError(t, err)

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
	t.Run("returns ErrNotInstalled if package and version is not installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.DeletePackageVersion(context.Background(), "go", "3", false)
		require.Equal(t, ErrNotInstalled, err)

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
	t.Run("returns ErrVersionInUse if package version is in use and force is false", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.DeletePackageVersion(context.Background(), "go", "2", false)
		require.Equal(t, err, ErrVersionInUse)

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
	t.Run("removes lib+executables if package version is in use and force is true", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "go", Version: "1"},
			{Package: "go", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "2", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.DeletePackageVersion(context.Background(), "go", "2", true)
		require.NoError(t, err)

		assertExecutables(t, datadir /* none */)

		// use package version remaining version works after delete+force
		err = m.UsePackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})
}

func assertDataDirExists(t *testing.T, datadir string, pkgs ...string) {
	t.Helper()

	info, err := os.Stat(datadir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	info, err = os.Stat(filepath.Join(datadir, "bin"))
	require.NoError(t, err)
	require.True(t, info.IsDir())

	info, err = os.Stat(filepath.Join(datadir, "lib"))
	require.NoError(t, err)
	require.True(t, info.IsDir())

	info, err = os.Stat(filepath.Join(datadir, "pkg"))
	require.NoError(t, err)
	require.True(t, info.IsDir())

	files := listFiles(t, datadir)
	require.NoError(t, err)
	filesDebug := fmt.Sprintf("all files: %+v", files)

	for _, pkg := range pkgs {
		info, err = os.Stat(filepath.Join(datadir, "lib", pkg))
		require.NoError(t, err)
		assert.True(t, info.IsDir(), filesDebug)

		info, err = os.Stat(filepath.Join(datadir, "pkg", pkg))
		require.NoError(t, err)
		assert.True(t, info.IsDir(), filesDebug)
	}
}

func assertDataDirNotExists(t *testing.T, datadir string, pkgs ...string) {
	t.Helper()

	info, err := os.Stat(datadir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	for _, pkg := range pkgs {
		_, err = os.Stat(filepath.Join(datadir, "lib", pkg))
		require.Error(t, err)

		_, err = os.Stat(filepath.Join(datadir, "pkg", pkg))
		require.Error(t, err)
	}
}

var goTarExpectedExecutables = []string{
	"go", "gofmt", "goimports", "gopls", "tree-sitter.so",
}

func assertExecutables(t *testing.T, datadir string, expected ...string) {
	t.Helper()

	bindir := filepath.Join(datadir, "bin")
	info, err := os.Stat(bindir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	actual := listFiles(t, bindir)
	require.NoError(t, err)

	require.Len(t, actual, len(expected))
	assert.ElementsMatch(t, expected, actual)
}

func listFiles(t *testing.T, bindir string) []string {
	scheme := newLocalScheme(bindir)
	it, err := walkdir.ListFiles(context.Background(), scheme, ".")
	require.NoError(t, err)
	files, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	return files
}

var syncTick = func(fn func()) bool { fn(); return true }

type recordingProgressWriter struct {
	mu       sync.Mutex
	complete bool
}

func (w *recordingProgressWriter) Progress(progress, total int64, _ string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if total > 0 && progress >= total {
		w.complete = true
	}
}

func (w *recordingProgressWriter) completed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.complete
}

func newTestManager(
	t *testing.T,
	packages map[string]release.Package,
	versions map[string][]release.Bundle,
) (*Manager, *idepkgtest.Notifications, *idepkgtest.ReleaseManager, string) {
	temp, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(temp)
	})
	configPath := filepath.Join(temp, "config.yaml")
	n := idepkgtest.NewNotifications(t)
	m := idepkgtest.NewReleaseManager(packages, versions)
	fileScheme := newLocalScheme(temp)
	wm := &mockWindowManager{
		floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
			// Auto-accept: send Enter to select "Allow"
			h.Resize(70, 20)
			h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
			return &mockWindow{}, nil
		},
	}
	manager := NewManager(n, m, storagestub.NewInMemoryService(),
		fileScheme, temp, configPath, wm, syncTick, term.NopInterrupter())
	return manager, n, m, temp
}

type mockWindowManager struct {
	floatingFn func(browserapi.Floating, browserapi.FloatingConfig) (browserapi.Window, error)
}

func (m *mockWindowManager) Focus() (browserapi.Window, error) { return nil, nil }
func (m *mockWindowManager) Split(_ browserapi.Orientation, _ browserapi.Window, _ browserapi.Handler) (browserapi.Window, error) {
	return nil, nil
}
func (m *mockWindowManager) Floating(h browserapi.Floating, cfg browserapi.FloatingConfig) (browserapi.Window, error) {
	if m.floatingFn != nil {
		return m.floatingFn(h, cfg)
	}
	return &mockWindow{}, nil
}
func (m *mockWindowManager) Bar(_ browserapi.BarConfig, _ tui.Handler) error { return nil }
func (m *mockWindowManager) Tab(_ workspaceapi.URI, _ rune, _ string, _ browserapi.Handler) (browserapi.Handler, error) {
	return nil, nil
}
func (m *mockWindowManager) SetWindowContent(_ browserapi.Window, _ browserapi.Handler) error {
	return nil
}
func (m *mockWindowManager) CloseWindow(_ browserapi.Window) error { return nil }

type mockWindow struct{}

func (m *mockWindow) WindowID() uint64 { return 0 }

// localScheme implements schemeapi.Scheme using os package functions for testing.
type localScheme struct {
	root string
}

func newLocalScheme(root string) *localScheme {
	return &localScheme{root: root}
}

func (s *localScheme) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.root, path)
}

func (s *localScheme) URI(path string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI("file://" + s.resolve(path))
}

func (s *localScheme) Root() string { return s.root }

func (s *localScheme) NewFile(_ uintptr, _ string) workspaceapi.File {
	panic("not implemented")
}

func (s *localScheme) Chroot(path string) (schemeapi.Scheme, error) {
	return newLocalScheme(s.resolve(path)), nil
}

func (s *localScheme) Watch(_ string, _ chan<- schemeapi.EventInfo, _ ...schemeapi.Event) (int, error) {
	panic("not implemented")
}

func (s *localScheme) StopWatch(_ int) error {
	panic("not implemented")
}

// schemeapi.FileSystem

func (s *localScheme) Create(filename string) (workspaceapi.File, error) {
	return os.Create(s.resolve(filename))
}

func (s *localScheme) Open(filename string) (workspaceapi.File, error) {
	return os.Open(s.resolve(filename))
}

func (s *localScheme) OpenFile(filename string, flag int, perm fs.FileMode) (workspaceapi.File, error) {
	return os.OpenFile(s.resolve(filename), flag, perm)
}

func (s *localScheme) Stat(filename string) (fs.FileInfo, error) {
	return os.Stat(s.resolve(filename))
}

func (s *localScheme) Rename(oldpath, newpath string) error {
	return os.Rename(s.resolve(oldpath), s.resolve(newpath))
}

func (s *localScheme) Remove(filename string) error {
	return os.Remove(s.resolve(filename))
}

func (s *localScheme) Join(elem ...string) string {
	return filepath.Join(elem...)
}

func (s *localScheme) TempFile(dir, prefix string) (workspaceapi.File, error) {
	return os.CreateTemp(s.resolve(dir), prefix)
}

func (s *localScheme) Lstat(filename string) (fs.FileInfo, error) {
	return os.Lstat(s.resolve(filename))
}

func (s *localScheme) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, s.resolve(newname))
}

func (s *localScheme) Readlink(link string) (string, error) {
	return os.Readlink(s.resolve(link))
}

func (s *localScheme) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(s.resolve(path))
}

func (s *localScheme) MkdirAll(filename string, perm fs.FileMode) error {
	return os.MkdirAll(s.resolve(filename), perm)
}

// schemeapi.Executor

func (s *localScheme) StartCommand(_ context.Context, _ workspaceapi.Cmd) (workspaceapi.Pid, error) {
	panic("not implemented")
}

func (s *localScheme) Signal(_ workspaceapi.Pid, _ syscall.Signal) error {
	panic("not implemented")
}

func (s *localScheme) Close() error { return nil }

// schemeapi.Terminal

func (s *localScheme) NewPty(_ context.Context) (workspaceapi.Pty, error) {
	panic("not implemented")
}

func (s *localScheme) SetPtySize(_ workspaceapi.Pty, _, _ int) error {
	panic("not implemented")
}

func readUserConfig(t *testing.T, datadir string) *yaml.Node {
	t.Helper()
	configPath := filepath.Join(datadir, "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Equal(t, yaml.DocumentNode, doc.Kind)
	return &doc
}

func readUserConfigMap(t *testing.T, path string) map[string]any {
	t.Helper()
	return readUserConfigMapBase(t, path, nil)
}

func readUserConfigMapBase(t *testing.T, path string, base map[string]any) map[string]any {
	t.Helper()
	cfg, err := loadIdePkgConfigFile(path, base)
	require.NoError(t, err)
	return cfg
}

func deepCopyConfig(v map[string]any) map[string]any {
	out := make(map[string]any, len(v))
	for k, val := range v {
		if m, ok := val.(map[string]any); ok {
			out[k] = deepCopyConfig(m)
			continue
		}
		out[k] = val
	}
	return out
}

func assertYAMLKey(t *testing.T, mapping *yaml.Node, key, expected string) {
	t.Helper()
	idx := findMappingKey(mapping, key)
	require.GreaterOrEqual(t, idx, 0, "key %q not found", key)
	assert.Equal(t, expected, mapping.Content[idx+1].Value)
}

func assertNestedYAMLKey(t *testing.T, mapping *yaml.Node, outerKey, innerKey, expected string) {
	t.Helper()
	idx := findMappingKey(mapping, outerKey)
	require.GreaterOrEqual(t, idx, 0, "outer key %q not found", outerKey)
	inner := mapping.Content[idx+1]
	require.Equal(t, yaml.MappingNode, inner.Kind, "outer key %q is not a mapping", outerKey)
	assertYAMLKey(t, inner, innerKey, expected)
}

func TestInstallPackageVersionConfig(t *testing.T) {
	t.Parallel()
	t.Run("config.yaml is merged into user config after install", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "configpkg", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		// Create an existing user config so processConfig merges into it
		configPath := filepath.Join(datadir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))

		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		doc := readUserConfig(t, datadir)
		root := doc.Content[0]
		assertNestedYAMLKey(t, root, "env", "GOROOT",
			datadir+"/pkg/configpkg/1/go")
		assertNestedYAMLKey(t, root, "settings", "theme", "dark")
		assertNestedYAMLKey(t, root, "settings", "indent", "4")
	})
	t.Run("skipped when no user config exists", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "configpkg", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Config should not have been created
		_, err = os.Stat(filepath.Join(datadir, "config.yaml"))
		assert.True(t, os.IsNotExist(err))
	})
}

func TestUsePackageVersionConfig(t *testing.T) {
	t.Parallel()
	t.Run("switching version updates user config", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "1"},
			{Package: "configpkg", Version: "2"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		// Create an existing user config so processConfig merges into it
		configPath := filepath.Join(datadir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))

		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		err = m.InstallPackageVersion(context.Background(), "configpkg", "2", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Prevent UsePackageVersion's synchronous prompt notification
		// from calling Done on a completed WaitGroup.
		n.ClearWg()

		// Switch to version 1
		require.NoError(t, os.RemoveAll(filepath.Join(datadir, "lib", "configpkg")))
		err = m.UsePackageVersion(context.Background(), "configpkg", "1")
		require.NoError(t, err)

		doc := readUserConfig(t, datadir)
		root := doc.Content[0]
		assertNestedYAMLKey(t, root, "env", "GOROOT",
			datadir+"/pkg/configpkg/1/go")
		assertNestedYAMLKey(t, root, "settings", "theme", "dark")
		assertNestedYAMLKey(t, root, "settings", "indent", "4")

		// Backup should exist since the config was modified
		_, err = os.Stat(filepath.Join(datadir, "config.yaml.backup"))
		require.NoError(t, err)
	})
}

func TestProcessInstalledSettingsConfig(t *testing.T) {
	t.Parallel()
	t.Run("merges config.yaml from installed packages into user config", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "configpkg", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		// Create an existing user config so processConfig merges into it
		configPath := filepath.Join(datadir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))

		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Reset the user config to an empty mapping to force re-merge
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))
		_ = os.Remove(filepath.Join(datadir, "config.yaml.backup"))

		// Prevent ProcessInstalledSettings' synchronous prompt notification
		// from calling Done on a completed WaitGroup.
		n.ClearWg()

		err = m.ProcessInstalledSettings(context.Background())
		require.NoError(t, err)

		doc := readUserConfig(t, datadir)
		root := doc.Content[0]
		assertNestedYAMLKey(t, root, "env", "GOROOT",
			datadir+"/pkg/configpkg/1/go")
		assertNestedYAMLKey(t, root, "settings", "theme", "dark")
	})
	t.Run("skipped when no user config exists", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "configpkg", Version: "1"}})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.ProcessInstalledSettings(context.Background())
		require.NoError(t, err)

		// Config should not have been created
		_, err = os.Stat(filepath.Join(datadir, "config.yaml"))
		assert.True(t, os.IsNotExist(err))
	})
}

func TestInstallPackageVersionConfigCrossFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userConfigName string
		userConfig     string
		pkgName        string
		wantGOROOT     string
		wantTheme      string
		wantIndent     string
	}{
		{
			name:           "extension config yaml onto user config yaml",
			userConfigName: "config.yaml",
			userConfig:     "{}\n",
			pkgName:        "configpkg",
			wantTheme:      "dark",
			wantIndent:     "4",
		},
		{
			name:           "extension config star onto user config star",
			userConfigName: "config.star",
			userConfig:     "config = {}\n",
			pkgName:        "configpkgstar",
			wantTheme:      "dark",
			wantIndent:     "4",
		},
		{
			name:           "extension config star onto user config yaml",
			userConfigName: "config.yaml",
			userConfig:     "{}\n",
			pkgName:        "configpkgstar",
			wantTheme:      "dark",
			wantIndent:     "4",
		},
		{
			name:           "extension config yaml onto user config star",
			userConfigName: "config.star",
			userConfig:     "config = {}\n",
			pkgName:        "configpkg",
			wantTheme:      "dark",
			wantIndent:     "4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pkgs := idepkgtest.MakePackages()
			versions := idepkgtest.MakeBundles([]release.Bundle{{Package: tt.pkgName, Version: "1"}})
			m, n, _, datadir := newTestManager(t, pkgs, versions)

			configPath := filepath.Join(datadir, tt.userConfigName)
			m.configPath = configPath
			require.NoError(t, os.WriteFile(configPath, []byte(tt.userConfig), 0o644))

			err := m.InstallPackageVersion(context.Background(), tt.pkgName, "1", repl.NopProgressWriter())
			require.NoError(t, err)
			n.RequireNoErrorNotification()

			cfg := readUserConfigMap(t, configPath)

			env, ok := cfg["env"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, filepath.Join(datadir, "pkg", tt.pkgName, "1", "go"), env["GOROOT"])

			settings, ok := cfg["settings"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantTheme, fmt.Sprint(settings["theme"]))
			assert.Equal(t, tt.wantIndent, fmt.Sprint(settings["indent"]))
		})
	}
}

// TestInstallPackageVersionConfigEmptyUserConfig verifies that a package
// overlay can still merge into a user config file whose body is empty or
// only contains comments. Without this, installing any package against a
// freshly created (or fully commented-out) user config.star failed with
// "load user config: starlark: expected top-level \"config\" dict".
func TestInstallPackageVersionConfigEmptyUserConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userConfigName string
		userConfig     string
		pkgName        string
	}{
		{
			name:           "empty user config yaml",
			userConfigName: "config.yaml",
			userConfig:     "",
			pkgName:        "configpkg",
		},
		{
			name:           "comments-only user config yaml",
			userConfigName: "config.yaml",
			userConfig:     "# just a comment\n# another one\n",
			pkgName:        "configpkg",
		},
		{
			name:           "empty user config star",
			userConfigName: "config.star",
			userConfig:     "",
			pkgName:        "configpkgstar",
		},
		{
			name:           "whitespace-only user config star",
			userConfigName: "config.star",
			userConfig:     "\n   \n\t\n",
			pkgName:        "configpkgstar",
		},
		{
			name:           "comments-only user config star",
			userConfigName: "config.star",
			userConfig:     "# just a comment\n# another one\n",
			pkgName:        "configpkgstar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pkgs := idepkgtest.MakePackages()
			versions := idepkgtest.MakeBundles([]release.Bundle{{Package: tt.pkgName, Version: "1"}})
			m, n, _, datadir := newTestManager(t, pkgs, versions)

			configPath := filepath.Join(datadir, tt.userConfigName)
			m.configPath = configPath
			require.NoError(t, os.WriteFile(configPath, []byte(tt.userConfig), 0o644))

			err := m.InstallPackageVersion(context.Background(), tt.pkgName, "1", repl.NopProgressWriter())
			require.NoError(t, err)
			n.RequireNoErrorNotification()

			cfg := readUserConfigMap(t, configPath)

			env, ok := cfg["env"].(map[string]any)
			require.True(t, ok, "env not present in merged config: %#v", cfg)
			assert.Equal(t, filepath.Join(datadir, "pkg", tt.pkgName, "1", "go"), env["GOROOT"])

			settings, ok := cfg["settings"].(map[string]any)
			require.True(t, ok, "settings not present in merged config: %#v", cfg)
			assert.Equal(t, "dark", fmt.Sprint(settings["theme"]))
			assert.Equal(t, "4", fmt.Sprint(settings["indent"]))

			if strings.HasSuffix(strings.ToLower(configPath), ".star") {
				raw, err := os.ReadFile(configPath)
				require.NoError(t, err)
				assert.Contains(t, string(raw), starlarkconfig.ManagedBegin)
				assert.Contains(t, string(raw), starlarkconfig.ManagedEnd)
			}
		})
	}
}

func TestPromptConfigChangeRender(t *testing.T) {
	t.Parallel()
	prompt := handler.NewPrompt(handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message: "Extension testpkg (v1) wants to update your configuration " +
				"with the following settings:\n\nenv:\n  GOROOT: /data/go\n\n" +
				"Do you want to allow this?",
			Options: []string{"Allow", "Deny"},
		},
		PromptHandler: handler.FuncPromptHandler(
			func(_ int, _ string) {},
			func() error { return nil },
		),
	})
	prompt.Resize(40, 15)
	rendered := handlertest.DrawHandler(prompt, 40, 15)
	assert.Contains(t, rendered, "Extension testpkg")
	assert.Contains(t, rendered, "Allow")
	assert.Contains(t, rendered, "Deny")
}

func TestPromptConfigChangeAllow(t *testing.T) {
	t.Parallel()
	var selected = -1
	prompt := handler.NewPrompt(handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message: "Allow changes?",
			Options: []string{"Allow", "Deny"},
		},
		PromptHandler: handler.FuncPromptHandler(
			func(idx int, _ string) { selected = idx },
			func() error { return nil },
		),
	})
	prompt.Resize(40, 15)
	exit, handled := prompt.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	assert.True(t, exit)
	assert.True(t, handled)
	assert.Equal(t, 0, selected)
}

func TestPromptConfigChangeDeny(t *testing.T) {
	t.Parallel()
	var selected = -1
	prompt := handler.NewPrompt(handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message: "Allow changes?",
			Options: []string{"Allow", "Deny"},
		},
		PromptHandler: handler.FuncPromptHandler(
			func(idx int, _ string) { selected = idx },
			func() error { return nil },
		),
	})
	prompt.Resize(40, 15)
	// Move right to "Deny" then press Enter
	prompt.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
	exit, handled := prompt.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	assert.True(t, exit)
	assert.True(t, handled)
	assert.Equal(t, 1, selected)
}

func TestPromptConfigChangeEscape(t *testing.T) {
	t.Parallel()
	var selected = -1
	prompt := handler.NewPrompt(handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message: "Allow changes?",
			Options: []string{"Allow", "Deny"},
		},
		PromptHandler: handler.FuncPromptHandler(
			func(idx int, _ string) { selected = idx },
			func() error { return nil },
		),
	})
	prompt.Resize(40, 15)
	exit, handled := prompt.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
	assert.True(t, exit)
	assert.True(t, handled)
	// OnSelect should not have been called
	assert.Equal(t, -1, selected)
}

func TestInstallConfigPromptDeny(t *testing.T) {
	t.Parallel()
	t.Run("prompt denies config merge, existing config unchanged", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "1"},
			{Package: "configpkg", Version: "2"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		// Create an existing user config so the prompt path is taken
		configPath := filepath.Join(datadir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("existing: true\n"), 0644))

		// First install merges into existing config (auto-accepted by default mock)
		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Capture decoded config state after first install.
		origCfg := readUserConfigMap(t, configPath)
		origEnv := origCfg["env"].(map[string]any)
		origGOROOT := fmt.Sprint(origEnv["GOROOT"])

		// Override wm to simulate "Deny" (select index 1)
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				h.Resize(70, 20)
				// Move to "Deny" then press Enter
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		// Second install (v2 has different config values) — deny the prompt
		err = m.InstallPackageVersion(context.Background(), "configpkg", "2", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Denying the conflicting (version-dependent) GOROOT change must
		// leave the user's existing value intact. Brand-new keys (newkey)
		// are auto-applied regardless of the deny, since they do not
		// override any existing user value.
		afterCfg := readUserConfigMap(t, configPath)
		afterEnv := afterCfg["env"].(map[string]any)
		assert.Equal(t, origGOROOT, fmt.Sprint(afterEnv["GOROOT"]),
			"denied version-dependent value must not change")
		settings := afterCfg["settings"].(map[string]any)
		assert.Equal(t, "added", fmt.Sprint(settings["newkey"]),
			"brand-new key is auto-applied even when the conflict prompt is denied")
	})
}

// TestInstallConfigPreservesUserValues verifies that when a package's
// config overlaps with a key the user already has set, the install
// does not prompt and does not overwrite the user's value.
func TestInstallConfigPreservesUserValues(t *testing.T) {
	t.Parallel()

	t.Run("static overlap does not prompt; version-dependent key re-prompts", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "1"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		configPath := filepath.Join(datadir, "config.yaml")
		// User pre-sets every key the package would write. Static keys
		// (theme, indent) must be preserved (RUNE-187); GOROOT derives
		// from $RUNE_PKG_VERSION so it must be re-prompted and updated
		// to the resolved value (RUNE-225).
		require.NoError(t, os.WriteFile(configPath, []byte(
			"env:\n  GOROOT: /custom/go\n"+
				"settings:\n  theme: light\n  indent: 8\n",
		), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assert.Equal(t, 1, promptCount, "prompt fires for the version-dependent GOROOT change")

		cfg := readUserConfigMap(t, configPath)
		env, ok := cfg["env"].(map[string]any)
		require.True(t, ok)
		wantGOROOT := filepath.Join(datadir, "pkg", "configpkg", "1", "go")
		assert.Equal(t, wantGOROOT, fmt.Sprint(env["GOROOT"]),
			"version-dependent GOROOT updates to the resolved value")
		settings, ok := cfg["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "light", fmt.Sprint(settings["theme"]))
		assert.Equal(t, "8", fmt.Sprint(settings["indent"]))
	})

	t.Run("new key and version-dependent change prompt; static values preserved", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "1"},
			{Package: "configpkg", Version: "2"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		configPath := filepath.Join(datadir, "config.yaml")
		// User has all v1 keys with custom values. v2 adds settings.newkey
		// and resolves GOROOT against version 2. The new key and the
		// version-dependent GOROOT are offered together; static theme/indent
		// and the unrelated `other` key are preserved.
		require.NoError(t, os.WriteFile(configPath, []byte(
			"env:\n  GOROOT: /custom/go\n"+
				"settings:\n  theme: light\n  indent: 8\n"+
				"other: untouched\n",
		), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		// Install v2 directly: only the newkey is new, the rest overlap.
		err := m.InstallPackageVersion(context.Background(), "configpkg", "2", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assert.Equal(t, 1, promptCount,
			"prompt fires once for the new key and the version-dependent change")

		cfg := readUserConfigMap(t, configPath)
		env, ok := cfg["env"].(map[string]any)
		require.True(t, ok)
		wantGOROOT := filepath.Join(datadir, "pkg", "configpkg", "2", "go")
		assert.Equal(t, wantGOROOT, fmt.Sprint(env["GOROOT"]),
			"version-dependent GOROOT updates to the v2 resolved value")
		settings, ok := cfg["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "light", fmt.Sprint(settings["theme"]))
		assert.Equal(t, "8", fmt.Sprint(settings["indent"]))
		assert.Equal(t, "added", fmt.Sprint(settings["newkey"]))
		assert.Equal(t, "untouched", fmt.Sprint(cfg["other"]))
	})

	t.Run("conflict prompt deny leaves conflict alone but auto-applies new key", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "2"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		configPath := filepath.Join(datadir, "config.yaml")
		userYAML := "env:\n  GOROOT: /custom/go\n" +
			"settings:\n  theme: light\n  indent: 8\n"
		require.NoError(t, os.WriteFile(configPath, []byte(userYAML), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		err := m.InstallPackageVersion(context.Background(), "configpkg", "2", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assert.Equal(t, 1, promptCount,
			"prompt should fire for the version-dependent GOROOT conflict")

		cfg := readUserConfigMap(t, configPath)
		env := cfg["env"].(map[string]any)
		assert.Equal(t, "/custom/go", fmt.Sprint(env["GOROOT"]),
			"denied version-dependent value must be preserved")
		settings := cfg["settings"].(map[string]any)
		assert.Equal(t, "light", fmt.Sprint(settings["theme"]), "static user value preserved")
		assert.Equal(t, "8", fmt.Sprint(settings["indent"]), "static user value preserved")
		assert.Equal(t, "added", fmt.Sprint(settings["newkey"]),
			"brand-new key is auto-applied even when the conflict prompt is denied")
	})
}

// --- Test helpers for crash-safe download tests ---

func createStaleEntry(
	t *testing.T, s document.Service, pkgID string, version release.Version,
) {
	t.Helper()
	key := idepkgTestStorageKey(pkgID, version)
	err := s.Create(context.Background(), key, newPkgVersionValue(pkgID, version))
	require.NoError(t, err)
}

func createCompleteEntry(
	t *testing.T, s document.Service, pkgID string, version release.Version,
) {
	t.Helper()
	key := idepkgTestStorageKey(pkgID, version)
	val := newPkgVersionValue(pkgID, version)
	val.Complete = true
	err := s.Create(context.Background(), key, val)
	require.NoError(t, err)
}

func assertStorageEntryComplete(
	t *testing.T, s document.Service, pkgID string, version release.Version,
) {
	t.Helper()
	key := idepkgTestStorageKey(pkgID, version)
	var val pkgVersionValue
	err := s.Get(context.Background(), key, &val)
	require.NoError(t, err, "storage entry should exist")
	assert.True(t, val.Complete, "storage entry should be complete")
}

func assertStorageEntryNotExists(
	t *testing.T, s document.Service, pkgID string, version release.Version,
) {
	t.Helper()
	key := idepkgTestStorageKey(pkgID, version)
	var val pkgVersionValue
	err := s.Get(context.Background(), key, &val)
	assert.True(t, errors.Is(err, document.ErrNotFound),
		"storage entry should not exist, got: %v", err)
}

func idepkgTestStorageKey(pkgID string, version release.Version) string {
	return fmt.Sprintf("%s:%s", pkgID, version)
}

// newTestManagerWithStorage is like newTestManager but returns the storage service too.
func newTestManagerWithStorage(
	t *testing.T,
	packages map[string]release.Package,
	versions map[string][]release.Bundle,
) (*Manager, *idepkgtest.Notifications, *idepkgtest.ReleaseManager, string, document.Service) {
	temp, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(temp)
	})
	configPath := filepath.Join(temp, "config.yaml")
	n := idepkgtest.NewNotifications(t)
	m := idepkgtest.NewReleaseManager(packages, versions)
	fileScheme := newLocalScheme(temp)
	wm := &mockWindowManager{
		floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
			h.Resize(70, 20)
			h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
			return &mockWindow{}, nil
		},
	}
	storage := document.NewInMemoryService()
	manager := NewManager(n, m, bluestore.AdaptTo(storage),
		fileScheme, temp, configPath, wm, syncTick, term.NopInterrupter())
	return manager, n, m, temp, storage
}

// --- Reconcile tests ---

func TestReconcile(t *testing.T) {
	t.Parallel()

	t.Run("crash_after_storage_create_before_download", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryNotExists(t, storage, "go", "1")
		_, serr := os.Stat(makePackageVersionDirname(datadir, "go", "1"))
		assert.True(t, os.IsNotExist(serr))
	})

	t.Run("crash_during_extraction", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		// Create partial pkg dir
		pkgDir := makePackageVersionDirname(datadir, "go", "1")
		require.NoError(t, os.MkdirAll(pkgDir, 0777))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "partial.txt"), []byte("x"), 0644))

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryNotExists(t, storage, "go", "1")
		_, serr := os.Stat(pkgDir)
		assert.True(t, os.IsNotExist(serr))
	})

	t.Run("crash_after_extraction_before_storage_update", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		pkgDir := makePackageVersionDirname(datadir, "go", "1")
		require.NoError(t, os.MkdirAll(pkgDir, 0777))

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryNotExists(t, storage, "go", "1")
		_, serr := os.Stat(pkgDir)
		assert.True(t, os.IsNotExist(serr))
	})

	t.Run("crash_during_link_lib_copy_bin", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		pkgDir := makePackageVersionDirname(datadir, "go", "1")
		require.NoError(t, os.MkdirAll(pkgDir, 0777))
		libLink := makePackageLibDirname(datadir, "go")
		require.NoError(t, os.Symlink(pkgDir, libLink))

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryNotExists(t, storage, "go", "1")
		_, serr := os.Stat(pkgDir)
		assert.True(t, os.IsNotExist(serr))
		_, serr = os.Lstat(libLink)
		assert.True(t, os.IsNotExist(serr))
	})

	t.Run("staging_dir_leftover", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		stagingDir := makeStagingDirname(datadir, "go", "1")
		require.NoError(t, os.MkdirAll(stagingDir, 0777))

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		_, serr := os.Stat(stagingDir)
		assert.True(t, os.IsNotExist(serr))
		assertStorageEntryNotExists(t, storage, "go", "1")
	})

	t.Run("stale_tmp_symlink", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, _ := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		tmpLink := filepath.Join(datadir, "lib", "go.tmp")
		require.NoError(t, os.Symlink("/nonexistent", tmpLink))

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		_, serr := os.Lstat(tmpLink)
		assert.True(t, os.IsNotExist(serr))
	})

	t.Run("multiple_packages_one_stale", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, _, storage := newTestManagerWithStorage(t, pkgs, versions)

		// Install "go:1" fully
		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Create stale "testpkg:1"
		createStaleEntry(t, storage, "testpkg", "1")

		err = m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryComplete(t, storage, "go", "1")
		assertStorageEntryNotExists(t, storage, "testpkg", "1")
	})

	t.Run("no_incomplete_installs", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _, storage := newTestManagerWithStorage(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryComplete(t, storage, "go", "1")
	})

	t.Run("empty_storage", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _, _ := newTestManagerWithStorage(t, pkgs, versions)

		err := m.Reconcile(context.Background())
		require.NoError(t, err)
	})

	t.Run("all_entries_stale", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		createStaleEntry(t, storage, "testpkg", "1")

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryNotExists(t, storage, "go", "1")
		assertStorageEntryNotExists(t, storage, "testpkg", "1")
	})

	t.Run("complete_entry_missing_dir", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createCompleteEntry(t, storage, "go", "1")
		// Don't create the pkg dir — simulates dir deletion

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		assertStorageEntryNotExists(t, storage, "go", "1")
	})

	t.Run("then_install_succeeds", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertStorageEntryComplete(t, storage, "go", "1")
		assertDataDirExists(t, datadir, "go")
	})

	t.Run("then_delete_returns_not_installed", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		err = m.DeletePackageVersion(context.Background(), "go", "1", false)
		require.Equal(t, ErrNotInstalled, err)
	})

	t.Run("then_list_excludes_recovered", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "testpkg", "1")

		err := m.Reconcile(context.Background())
		require.NoError(t, err)

		it, err := m.ListInstalledPackages(context.Background())
		require.NoError(t, err)
		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)
		assert.Empty(t, installed)
	})
}

// --- Install stale entry handling tests ---

func TestInstallStaleEntry(t *testing.T) {
	t.Parallel()

	t.Run("detects_and_cleans_stale_entry", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)

		createStaleEntry(t, storage, "go", "1")

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertStorageEntryComplete(t, storage, "go", "1")
		assertDataDirExists(t, datadir, "go")
	})

	t.Run("stale_entry_with_partial_files", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		pkgDir := makePackageVersionDirname(datadir, "go", "1")
		require.NoError(t, os.MkdirAll(pkgDir, 0777))
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "partial.txt"), []byte("x"), 0644))

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertStorageEntryComplete(t, storage, "go", "1")
		assertDataDirExists(t, datadir, "go")
	})

	t.Run("completed_entry_rejects_reinstall", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _, _ := newTestManagerWithStorage(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		err = m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already been installed")
	})

	t.Run("stale_entry_with_staging_dir", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)
		require.NoError(t, makePkgDirs(datadir))

		createStaleEntry(t, storage, "go", "1")
		stagingDir := makeStagingDirname(datadir, "go", "1")
		require.NoError(t, os.MkdirAll(stagingDir, 0777))

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assertStorageEntryComplete(t, storage, "go", "1")
		_, serr := os.Stat(stagingDir)
		assert.True(t, os.IsNotExist(serr))
	})
}

// --- Listing methods filter incomplete tests ---

func TestListInstalledPackagesExcludesStale(t *testing.T) {
	t.Parallel()

	t.Run("excludes_stale", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _, storage := newTestManagerWithStorage(t, pkgs, versions)

		// Install "go:1" fully
		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		// Create stale "testpkg:1"
		createStaleEntry(t, storage, "testpkg", "1")

		it, err := m.ListInstalledPackages(context.Background())
		require.NoError(t, err)
		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.Equal(t, []string{"go"}, installed)
	})
}

func TestListInstalledPackageVersionsExcludesStale(t *testing.T) {
	t.Parallel()

	t.Run("excludes_stale", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles(
			[]release.Bundle{
				{Package: "go", Version: "1"},
				{Package: "go", Version: "2"},
			},
		)
		m, _, _, _, storage := newTestManagerWithStorage(t, pkgs, versions)

		// Install "go:1" fully
		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		// Create stale "go:2"
		createStaleEntry(t, storage, "go", "2")

		it, err := m.ListInstalledPackageVersions(context.Background(), "go")
		require.NoError(t, err)
		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.Equal(t, []release.Version{"1"}, installed)
	})
}

func TestUsePackageVersionRejectsIncomplete(t *testing.T) {
	t.Parallel()

	t.Run("rejects_incomplete", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles(
			[]release.Bundle{
				{Package: "go", Version: "1"},
				{Package: "go", Version: "2"},
			},
		)
		m, _, _, _, storage := newTestManagerWithStorage(t, pkgs, versions)

		// Install "go:1" fully
		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		// Create stale "go:2"
		createStaleEntry(t, storage, "go", "2")

		err = m.UsePackageVersion(context.Background(), "go", "2")
		require.Equal(t, ErrNotInstalled, err)
	})
}

func TestProcessInstalledSettingsSkipsIncomplete(t *testing.T) {
	t.Parallel()

	t.Run("skips_incomplete", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "configpkg", Version: "1"}})
		m, n, _, datadir, storage := newTestManagerWithStorage(t, pkgs, versions)

		configPath := filepath.Join(datadir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))

		// Install "configpkg:1" fully
		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Create stale "configpkg:2" — processInstalledSettings should skip it
		createStaleEntry(t, storage, "configpkg", "2")

		n.ClearWg()
		err = m.ProcessInstalledSettings(context.Background())
		require.NoError(t, err)
	})
}

// --- Atomic operation tests ---

func TestInstallAtomicOperations(t *testing.T) {
	t.Parallel()

	t.Run("no_staging_dir_after_success", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, datadir, _ := newTestManagerWithStorage(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		stagingDir := makeStagingDirname(datadir, "go", "1")
		_, serr := os.Stat(stagingDir)
		assert.True(t, os.IsNotExist(serr), "staging dir should not exist after success")
	})

	t.Run("storage_entry_is_complete_after_success", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _, storage := newTestManagerWithStorage(t, pkgs, versions)

		err := m.InstallPackageVersion(context.Background(), "go", "1", repl.NopProgressWriter())
		require.NoError(t, err)

		assertStorageEntryComplete(t, storage, "go", "1")
	})
}

// TestProcessConfigAutoApply covers the auto-apply path: purely-new overlay
// keys are written to disk without prompting, while keys that override an
// existing user value still prompt for confirmation.
func TestProcessConfigAutoApply(t *testing.T) {
	t.Parallel()

	hasInfoNotification := func(n *idepkgtest.Notifications) bool {
		for _, noti := range n.Active() {
			if noti.Level == browserapi.LevelInfo {
				return true
			}
		}
		return false
	}

	t.Run("all-new keys auto-apply without prompt and notify", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "vpkg", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)

		require.NoError(t, os.WriteFile(m.configPath, []byte("existing: true\n"), 0o644))

		m.wm = &mockWindowManager{
			floatingFn: func(_ browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				t.Fatal("no prompt should be scheduled for purely-new config")
				return nil, nil
			},
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"settings:\n  theme: dark\n  indent: 4\n"), 0o644))

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		n.RequireNoErrorNotification()
		assert.True(t, hasInfoNotification(n),
			"auto-apply must surface an info notification")

		cfg := readUserConfigMap(t, m.configPath)
		assert.Equal(t, true, cfg["existing"], "user key preserved")
		settings := cfg["settings"].(map[string]any)
		assert.Equal(t, "dark", fmt.Sprint(settings["theme"]))
		assert.Equal(t, "4", fmt.Sprint(settings["indent"]))
	})

	t.Run("existing-key override prompts and leaves disk unchanged until allow", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
			{Package: "vpkg", Version: "2"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		v1GOROOT := filepath.Join(datadir, "pkg", "vpkg", "1", "go")
		require.NoError(t, os.WriteFile(m.configPath, []byte(
			"env:\n  GOROOT: "+v1GOROOT+"\n"), 0o644))

		var prompted bool
		var promptYAML []byte
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				prompted = true
				// Deny so the disk stays unchanged.
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"env:\n  GOROOT: $RUNE_DATADIR/pkg/$RUNE_PKG_ID/$RUNE_PKG_VERSION/go\n"), 0o644))

		plan, err := planConfigChange(pkgConfig,
			[]byte("env:\n  GOROOT: $RUNE_DATADIR/pkg/$RUNE_PKG_ID/$RUNE_PKG_VERSION/go\n"),
			readUserConfigMap(t, m.configPath), "vpkg", release.Version("2"),
			m.dataDir, m.editorMode)
		require.NoError(t, err)
		require.True(t, plan.prompt)
		require.Nil(t, plan.autoApplyDoc, "no new keys to auto-apply")
		promptYAML = plan.missingYAML
		assert.Contains(t, string(promptYAML), "GOROOT",
			"prompt covers only the conflicting key")

		require.NoError(t, m.processConfig("vpkg", release.Version("2"), pkgConfig))
		n.RequireNoErrorNotification()
		assert.True(t, prompted, "conflicting key must prompt")

		cfg := readUserConfigMap(t, m.configPath)
		env := cfg["env"].(map[string]any)
		assert.Equal(t, v1GOROOT, fmt.Sprint(env["GOROOT"]),
			"disk unchanged after deny")
	})

	t.Run("mixed new and conflicting auto-applies new and prompts for conflict", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
			{Package: "vpkg", Version: "2"},
		})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		v1GOROOT := filepath.Join(datadir, "pkg", "vpkg", "1", "go")
		require.NoError(t, os.WriteFile(m.configPath, []byte(
			"env:\n  GOROOT: "+v1GOROOT+"\n"), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				// Deny: only the conflicting key is gated by the prompt.
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"env:\n  GOROOT: $RUNE_DATADIR/pkg/$RUNE_PKG_ID/$RUNE_PKG_VERSION/go\n"+
				"  NEW: added\n"), 0o644))

		require.NoError(t, m.processConfig("vpkg", release.Version("2"), pkgConfig))
		n.RequireNoErrorNotification()
		assert.Equal(t, 1, promptCount, "conflicting key prompts")
		assert.True(t, hasInfoNotification(n),
			"auto-apply of the new key must notify")

		cfg := readUserConfigMap(t, m.configPath)
		env := cfg["env"].(map[string]any)
		assert.Equal(t, "added", fmt.Sprint(env["NEW"]),
			"new key auto-applied to disk immediately")
		assert.Equal(t, v1GOROOT, fmt.Sprint(env["GOROOT"]),
			"conflicting key unchanged after deny")
	})

	t.Run("same-value overlay is a no-op without prompt or notification", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "vpkg", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)

		require.NoError(t, os.WriteFile(m.configPath, []byte(
			"settings:\n  theme: dark\n"), 0o644))

		m.wm = &mockWindowManager{
			floatingFn: func(_ browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				t.Fatal("no prompt for an already-equal overlay")
				return nil, nil
			},
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"settings:\n  theme: dark\n"), 0o644))

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		n.RequireNoErrorNotification()
		assert.False(t, hasInfoNotification(n),
			"no notification when nothing changed")
	})
}

func TestProcessConfigSkipsPromptWhenAlreadyMerged(t *testing.T) {
	t.Parallel()
	t.Run("no prompt shown when config is already merged", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "configpkg", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		// Create an existing user config so processConfig merges into it
		configPath := filepath.Join(datadir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))

		// First install: prompt is shown and accepted (default mock auto-accepts)
		err := m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		// Verify config was written
		_ = readUserConfig(t, datadir)

		// Override wm to panic if prompt is shown — it should NOT be called
		// since the config is already merged.
		m.wm = &mockWindowManager{
			floatingFn: func(_ browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				t.Fatal("prompt should not be shown when config is already merged")
				return nil, nil
			},
		}

		// Re-process settings: should detect config is already merged and skip
		err = m.ProcessInstalledSettings(context.Background())
		require.NoError(t, err)
	})
}

// TestProcessConfigRepromptsOnVersionDependentChange is a regression test
// for RUNE-225: when a package config scalar's raw template depends on
// $RUNE_PKG_VERSION, the resolved value changes across versions. Rune must
// re-prompt and persist the new value when it differs from what is stored,
// while leaving user-customized static scalars untouched (RUNE-187).
func TestProcessConfigRepromptsOnVersionDependentChange(t *testing.T) {
	t.Parallel()

	t.Run("version-dependent scalar re-prompts and updates on version bump", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
			{Package: "vpkg", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)

		require.NoError(t, os.WriteFile(m.configPath, []byte("{}\n"), 0o644))

		pkgDir := t.TempDir()
		pkgConfig := filepath.Join(pkgDir, "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"env:\n  GOROOT: $RUNE_DATADIR/pkg/$RUNE_PKG_ID/$RUNE_PKG_VERSION/go\n",
		), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		// v1: new key auto-applies without a prompt and the value is written.
		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		assert.Equal(t, 0, promptCount, "first install auto-applies the new key without prompting")

		v1Want := filepath.Join(datadir, "pkg", "vpkg", "1", "go")
		cfg := readUserConfigMap(t, m.configPath)
		env, ok := cfg["env"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, v1Want, fmt.Sprint(env["GOROOT"]))

		// v2: same template, different resolved value -> must re-prompt and update.
		require.NoError(t, m.processConfig("vpkg", release.Version("2"), pkgConfig))
		assert.Equal(t, 1, promptCount, "version bump re-prompts the version-dependent key")

		v2Want := filepath.Join(datadir, "pkg", "vpkg", "2", "go")
		cfg = readUserConfigMap(t, m.configPath)
		env, ok = cfg["env"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, v2Want, fmt.Sprint(env["GOROOT"]), "on-disk value updates to v2")
	})

	t.Run("version-dependent scalar does not re-prompt when value is unchanged", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		require.NoError(t, os.WriteFile(m.configPath, []byte("{}\n"), 0o644))

		pkgDir := t.TempDir()
		pkgConfig := filepath.Join(pkgDir, "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"env:\n  GOROOT: $RUNE_DATADIR/pkg/$RUNE_PKG_ID/$RUNE_PKG_VERSION/go\n",
		), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		assert.Equal(t, 0, promptCount, "first install auto-applies the new key without prompting")

		// Same version again: resolved value matches disk, no re-prompt.
		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		assert.Equal(t, 0, promptCount, "unchanged version-dependent value must not re-prompt")
	})

	t.Run("static user-customized scalar is preserved and not re-prompted", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
		})
		m, _, _, _ := newTestManager(t, pkgs, versions)

		// User customized a static scalar the package also ships.
		require.NoError(t, os.WriteFile(m.configPath, []byte(
			"settings:\n  theme: light\n",
		), 0o644))

		pkgDir := t.TempDir()
		pkgConfig := filepath.Join(pkgDir, "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"settings:\n  theme: dark\n",
		), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		assert.Equal(t, 0, promptCount, "static differing scalar must not re-prompt (RUNE-187)")

		cfg := readUserConfigMap(t, m.configPath)
		settings, ok := cfg["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "light", fmt.Sprint(settings["theme"]), "user value preserved")
	})
}

// TestInstallPackageVersionNonUTF8PAXXattr is a regression test for
// RUNE-174: previously, tar.Header values returned by untar were persisted
// verbatim into the TOML-backed package store, so a tarball carrying a PAX
// xattr with non-UTF-8 bytes (such as macOS's "com.apple.provenance"
// containing 0xad) made storage.Update fail with
// "invalid UTF-8 byte: 0xad", aborting the install.
func TestInstallPackageVersionNonUTF8PAXXattr(t *testing.T) {
	t.Parallel()
	pkgID := "paxpkg"
	pkgs := idepkgtest.MakePackages()
	versions := idepkgtest.MakeBundles([]release.Bundle{
		{Package: pkgID, Version: "1"},
	})

	tarball := makePAXXattrTarball(t)

	m, n, r, datadir, storage := newTestManagerWithLocalStorage(t, pkgs, versions)
	r.SetTarball(pkgID, tarball)

	err := m.InstallPackageVersion(context.Background(), pkgID, "1", repl.NopProgressWriter())
	require.NoError(t, err)
	n.RequireNoErrorNotification()

	// The binary must end up in the package's bin directory.
	binPath := filepath.Join(datadir, "bin", "tool")
	info, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "installed file must be executable")

	// Round-trip the persisted record through the TOML store. Before the
	// fix, the file written during install contained raw 0xad bytes inside
	// a TOML string and the next read failed with "invalid UTF-8 byte:
	// 0xad". After the fix, the stored value carries only Name/Mode and
	// reads succeed.
	var got pkgVersionValue
	key := m.makeDownloadKey(pkgID, "1")
	require.NoError(t, storage.Get(context.Background(), key, &got))
	assert.Equal(t, pkgID, got.Package)
	require.Len(t, got.Executables, 1)
	assert.Equal(t, "tool", got.Executables[0].Name)
}

// newTestManagerWithLocalStorage is like newTestManager but uses the
// production-style TOML-backed local storage so that any non-UTF-8 bytes in
// persisted values surface as marshaling errors.
func newTestManagerWithLocalStorage(
	t *testing.T,
	packages map[string]release.Package,
	versions map[string][]release.Bundle,
) (*Manager, *idepkgtest.Notifications, *idepkgtest.ReleaseManager, string, storageapi.Service) {
	temp, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(temp)
	})
	configPath := filepath.Join(temp, "config.yaml")
	n := idepkgtest.NewNotifications(t)
	r := idepkgtest.NewReleaseManager(packages, versions)
	fileScheme := newLocalScheme(temp)
	wm := &mockWindowManager{
		floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
			h.Resize(70, 20)
			h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
			return &mockWindow{}, nil
		},
	}
	storage := localstorage.New(context.Background(), temp, docbson.Marshaler())
	manager := NewManager(n, r, storage,
		fileScheme, temp, configPath, wm, syncTick, term.NopInterrupter())
	return manager, n, r, temp, storage
}

// makePAXXattrTarball builds a gzipped tar with a single executable whose
// PAX records include the macOS "com.apple.provenance" xattr containing a
// non-UTF-8 byte (0xad). This mirrors what Go binaries on macOS carry and
// is the payload that triggered the original install failure.
func makePAXXattrTarball(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	const content = "#!/bin/sh\necho hi\n"
	hdr := &tar.Header{
		Name:   "tool",
		Mode:   0o755,
		Size:   int64(len(content)),
		Format: tar.FormatPAX,
		PAXRecords: map[string]string{
			"SCHILY.xattr.com.apple.provenance": "\xad",
		},
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gzw.Close())
	return buf.Bytes()
}

// failingUpdateStorage wraps a storageapi.Service and returns failErr from
// Update calls whose ID matches failKey. All other operations delegate
// to the inner service unchanged.
type failingUpdateStorage struct {
	storageapi.Service
	failKey string
	failErr error
}

func (s *failingUpdateStorage) Update(
	ctx context.Context, id string,
	updates []storageapi.Update, precond ...storageapi.Precondition,
) error {
	if id == s.failKey {
		return s.failErr
	}
	return s.Service.Update(ctx, id, updates, precond...)
}

func TestInstallNoConfigPromptOnFailure(t *testing.T) {
	t.Parallel()
	t.Run("no config prompt scheduled when storage update fails", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "1"},
		})

		temp, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(temp) })

		configPath := filepath.Join(temp, "config.yaml")
		// Create user config so processConfig would otherwise schedule a prompt.
		require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0644))

		n := idepkgtest.NewNotifications(t)
		rel := idepkgtest.NewReleaseManager(pkgs, versions)
		fileScheme := newLocalScheme(temp)
		var promptCalled atomic.Bool
		wm := &mockWindowManager{
			floatingFn: func(_ browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				// Recorded and asserted on the test goroutine after
				// Wait — install runs on a different goroutine where
				// t.Fatal would not reliably abort the install path.
				promptCalled.Store(true)
				return &mockWindow{}, nil
			},
		}
		storage := &failingUpdateStorage{
			Service: storagestub.NewInMemoryService(),
			failKey: "configpkg:1",
			failErr: errors.New("simulated storage update failure"),
		}
		m := NewManager(n, rel, storage, fileScheme, temp, configPath, wm,
			syncTick, term.NopInterrupter())

		err = m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.Error(t, err)
		assert.False(t, promptCalled.Load(),
			"config prompt must not be scheduled when install fails")

		// The installed package dir should have been rolled back.
		pkgDir := makePackageVersionDirname(temp, "configpkg", "1")
		_, statErr := os.Stat(pkgDir)
		assert.True(t, os.IsNotExist(statErr),
			"package version dir should be removed after rollback")

		// User config should be untouched.
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Equal(t, "{}\n", string(data),
			"user config should not be modified when install fails")
	})
}

func TestInstallConfigPromptScheduledAfterSuccess(t *testing.T) {
	t.Parallel()
	t.Run("prompt only scheduled after install success notification", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "configpkg", Version: "1"},
		})

		temp, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(temp) })

		configPath := filepath.Join(temp, "config.yaml")
		// Seed a stale version-dependent value so processConfig classifies
		// it as a conflict and schedules the prompt; auto-apply only covers
		// brand-new keys, which would not exercise the Floating path.
		require.NoError(t, os.WriteFile(configPath, []byte(
			"env:\n  GOROOT: /stale/go\n"), 0644))

		n := idepkgtest.NewNotifications(t)
		rel := idepkgtest.NewReleaseManager(pkgs, versions)
		fileScheme := newLocalScheme(temp)

		// Record the order of success notification vs. prompt scheduling.
		// Floating is invoked from inside processConfig's scheduled fn,
		// which runs synchronously under syncTick. So if processConfig
		// runs after storage.Update (the fix), Floating must be observed
		// only after storage has marked the entry complete.
		var (
			promptCalled    bool
			storageComplete bool
		)
		wm := &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				assert.True(t, storageComplete,
					"prompt must not be scheduled before storage marks the install complete")
				promptCalled = true
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}
		storage := &orderingStorage{
			Service:    storagestub.NewInMemoryService(),
			completeAt: "configpkg:1",
			completed:  &storageComplete,
		}
		m := NewManager(n, rel, storage, fileScheme, temp, configPath, wm,
			syncTick, term.NopInterrupter())

		err = m.InstallPackageVersion(context.Background(), "configpkg", "1", repl.NopProgressWriter())
		require.NoError(t, err)
		n.RequireNoErrorNotification()

		assert.True(t, promptCalled, "config prompt should be scheduled on success path")
	})
}

// orderingStorage flips *completed to true after a successful Update for completeAt.
// Used to observe whether the config prompt is scheduled before the
// install is durably committed.
type orderingStorage struct {
	storageapi.Service
	completeAt string
	completed  *bool
}

func (s *orderingStorage) Update(
	ctx context.Context, id string,
	updates []storageapi.Update, precond ...storageapi.Precondition,
) error {
	err := s.Service.Update(ctx, id, updates, precond...)
	if err == nil && id == s.completeAt {
		*s.completed = true
	}
	return err
}

// TestEditorModeParamExposed verifies that RUNE_EDITOR_MODE reaches package
// config.star scripts via idePkgStarlarkParams. RUNE-137 needs this to drive
// the extension_fuzzy_search mode-aware key bindings.
func TestEditorModeParamExposed(t *testing.T) {
	t.Parallel()

	const src = `
config = {}
if RUNE_EDITOR_MODE == "modeless":
    config["picked_mode"] = "modeless"
else:
    config["picked_mode"] = "other"
`
	cases := []struct {
		mode    string
		wantMod string
	}{
		{"modal", "other"},
		{"modeless", "modeless"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			got, err := loadIdePkgConfigFromBytes(
				"config.star", []byte(src),
				nil, "pkg", release.Version("1"), "/data",
				tc.mode,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.wantMod, got["picked_mode"])
		})
	}
}

// TestPkgConfigFilePrefersYAML and TestPkgConfigFileFallsBackToStar lock in
// the shared discovery used by download/untar/UsePackageVersion/
// ProcessInstalledSettings: config.yaml wins when both formats are present,
// config.star is picked up when only it ships.
func TestPkgConfigFilePrefersYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("{}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.star"), []byte("config = {}\n"), 0o644))
	assert.Equal(t, filepath.Join(dir, "config.yaml"), pkgConfigFile(dir))
}

func TestPkgConfigFileFallsBackToStar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.star"), []byte("config = {}\n"), 0o644))
	assert.Equal(t, filepath.Join(dir, "config.star"), pkgConfigFile(dir))
}

// TestPkgConfigFileNestedWrapperDir locks in discovery for tarballs that
// nest everything under a single top-level wrapper directory (the Linux
// rune-agent bundle tars `rune-agent/...` rather than `.`, so config.yaml
// lands at <dir>/rune-agent/config.yaml). Without descending one level the
// extension's shipped config is silently never merged.
func TestPkgConfigFileNestedWrapperDir(t *testing.T) {
	t.Parallel()
	t.Run("yaml under single wrapper dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		wrapper := filepath.Join(dir, "rune-agent")
		require.NoError(t, os.MkdirAll(filepath.Join(wrapper, "bin"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(wrapper, "config.yaml"), []byte("{}\n"), 0o644))
		assert.Equal(t, filepath.Join(wrapper, "config.yaml"), pkgConfigFile(dir))
	})
	t.Run("star under single wrapper dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		wrapper := filepath.Join(dir, "pkg")
		require.NoError(t, os.MkdirAll(wrapper, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(wrapper, "config.star"), []byte("config = {}\n"), 0o644))
		assert.Equal(t, filepath.Join(wrapper, "config.star"), pkgConfigFile(dir))
	})
	t.Run("top level still wins over nested", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		wrapper := filepath.Join(dir, "rune-agent")
		require.NoError(t, os.MkdirAll(wrapper, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(wrapper, "config.yaml"), []byte("{}\n"), 0o644))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "config.yaml"), []byte("{}\n"), 0o644))
		assert.Equal(t, filepath.Join(dir, "config.yaml"), pkgConfigFile(dir))
	})
}

// TestLoadUserConfigStarEmptyOrCommentsOnly verifies that a user-side
// config.star that is empty or contains only comments loads as an empty
// configuration instead of failing with "expected top-level config dict".
// Without this, installing a package against a fresh/commented user config
// would surface a confusing "load user config" error to the user.
func TestLoadUserConfigStarEmptyOrCommentsOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"whitespace only", "\n   \n\t\n"},
		{"comments only", "# just a comment\n# another one\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadIdePkgConfigFromBytes(
				"config.star", []byte(tc.src),
				nil, "pkg", release.Version("1"), "/data", "modal",
			)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{}, got)
		})
	}
}

func fileInode(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	st, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "expected *syscall.Stat_t")
	return uint64(st.Ino)
}

// TestCopyExecutablesAtomicSwap verifies that upgrading an installed
// executable replaces the directory entry with a fresh inode rather
// than truncating the existing one in place. Rewriting the inode of a
// running, unsigned extension binary causes macOS Gatekeeper/AMFI to
// kill it.
func TestCopyExecutablesAtomicSwap(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "ext"),
		[]byte("new-binary"), 0o755))

	target := filepath.Join(dstDir, "ext")
	require.NoError(t, os.WriteFile(target, []byte("old-binary"), 0o755))
	oldInode := fileInode(t, target)

	files := []executableEntry{{Name: "ext", Mode: 0o755}}
	require.NoError(t, copyExecutables(files, srcDir, dstDir))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new-binary", string(got))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	assert.NotEqual(t, oldInode, fileInode(t, target),
		"executable should be swapped via rename, not truncated in place")

	entries, err := os.ReadDir(dstDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no leftover temp files")
	assert.Equal(t, "ext", entries[0].Name())
}

func TestCopyExecutablesMissingSource(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	files := []executableEntry{{Name: "missing", Mode: 0o755}}
	err := copyExecutables(files, srcDir, dstDir)
	require.Error(t, err)
	entries, derr := os.ReadDir(dstDir)
	require.NoError(t, derr)
	assert.Empty(t, entries, "no temp files left behind on error")
}

// mergeStep is one apply iteration in a TestConfigMergeIntegration case:
// it supplies the package overlay and version, and the expectations to
// check after the apply.
type mergeStep struct {
	pkgConfig       string
	version         string
	wantPrompt      bool
	wantConfig      map[string]any
	wantContains    []string
	wantNotContains []string
}

// TestConfigMergeIntegration is the black-box contract for extension config
// updates. It drives the real production path (processConfig ->
// planConfigChange -> auto-accepted prompt -> buildMergedConfig -> atomic
// write) for both .star and .yaml user configs, asserting only inputs and
// final on-disk + decoded outputs. It is the primary guard for RUNE-225:
// .star updates must preserve user comments, helpers, and custom logic.
func TestConfigMergeIntegration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		configName string
		base       map[string]any
		userConfig string
		steps      []mergeStep
	}{
		{
			name:       "star comments and helper preserved",
			configName: "config.star",
			userConfig: "# top comment\n" +
				"def merge(a, b):\n    return a\n\n" +
				"config = {\"editor\": {\"mode\": \"exo\"}}\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"editor": map[string]any{"mode": "exo"},
					"env":    map[string]any{"GOROOT": "/go"},
				},
				wantContains: []string{
					"# top comment", "def merge(a, b):",
					starlarkconfig.ManagedBegin, starlarkconfig.ManagedEnd,
				},
			}},
		},
		{
			name:       "star fresh empty user config",
			configName: "config.star",
			userConfig: "",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"env": map[string]any{"GOROOT": "/go"}},
				wantContains: []string{
					starlarkconfig.ManagedBegin, "config = {}",
				},
			}},
		},
		{
			name:       "star comments-only user config",
			configName: "config.star",
			userConfig: "# just a comment\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"env": map[string]any{"GOROOT": "/go"}},
				wantContains: []string{
					"# just a comment", starlarkconfig.ManagedBegin,
				},
			}},
		},
		{
			name:       "star deep merge keeps sibling",
			configName: "config.star",
			userConfig: "config = {\"settings\": {\"theme\": \"light\"}}\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"settings\": {\"indent\": 4}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"settings": map[string]any{"theme": "light", "indent": 4},
				},
			}},
		},
		{
			name:       "star static user scalar preserved",
			configName: "config.star",
			userConfig: "config = {\"settings\": {\"theme\": \"light\"}}\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"settings\": {\"theme\": \"dark\"}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"settings": map[string]any{"theme": "light"},
				},
				wantNotContains: []string{starlarkconfig.ManagedBegin},
			}},
		},
		{
			name:       "star accumulation across extensions",
			configName: "config.star",
			userConfig: "config = {}\n",
			steps: []mergeStep{
				{
					pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
					version:    "1",
					wantPrompt: false,
					wantConfig: map[string]any{"env": map[string]any{"GOROOT": "/go"}},
				},
				{
					pkgConfig:  "config = {\"settings\": {\"indent\": 4}}\n",
					version:    "1",
					wantPrompt: false,
					wantConfig: map[string]any{
						"env":      map[string]any{"GOROOT": "/go"},
						"settings": map[string]any{"indent": 4},
					},
				},
			},
		},
		{
			name:       "star already merged no prompt",
			configName: "config.star",
			userConfig: "config = {}\n",
			steps: []mergeStep{
				{
					pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
					version:    "1",
					wantPrompt: false,
				},
				{
					pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
					version:    "1",
					wantPrompt: false,
				},
			},
		},
		{
			name:       "yaml new key",
			configName: "config.yaml",
			userConfig: "a: 1\n",
			steps: []mergeStep{{
				pkgConfig:  "env:\n  GOROOT: /go\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"a": 1, "env": map[string]any{"GOROOT": "/go"}},
			}},
		},
		{
			name:       "yaml nested sibling merge keeps siblings",
			configName: "config.yaml",
			userConfig: "settings:\n  theme: light\n",
			steps: []mergeStep{{
				pkgConfig:  "settings:\n  indent: 4\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"settings": map[string]any{"theme": "light", "indent": 4},
				},
			}},
		},
		{
			name:       "yaml static scalar preserved",
			configName: "config.yaml",
			userConfig: "settings:\n  theme: light\n",
			steps: []mergeStep{{
				pkgConfig:  "settings:\n  theme: dark\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"settings": map[string]any{"theme": "light"}},
			}},
		},
		// ---- parity edge cases: .star and .yaml must agree ----
		{
			name:       "star list new key added",
			configName: "config.star",
			userConfig: "config = {}\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"plugins\": [\"a\", \"b\"]}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"plugins": []any{"a", "b"}},
			}},
		},
		{
			name:       "yaml list new key added",
			configName: "config.yaml",
			userConfig: "{}\n",
			steps: []mergeStep{{
				pkgConfig:  "plugins:\n  - a\n  - b\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"plugins": []any{"a", "b"}},
			}},
		},
		{
			name:       "star existing list key preserved no prompt",
			configName: "config.star",
			userConfig: "config = {\"plugins\": [\"x\"]}\n",
			steps: []mergeStep{{
				pkgConfig:       "config = {\"plugins\": [\"a\", \"b\"]}\n",
				version:         "1",
				wantPrompt:      false,
				wantConfig:      map[string]any{"plugins": []any{"x"}},
				wantNotContains: []string{starlarkconfig.ManagedBegin},
			}},
		},
		{
			name:       "yaml existing list key preserved no prompt",
			configName: "config.yaml",
			userConfig: "plugins:\n  - x\n",
			steps: []mergeStep{{
				pkgConfig:  "plugins:\n  - a\n  - b\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"plugins": []any{"x"}},
			}},
		},
		{
			name:       "star null value new key",
			configName: "config.star",
			userConfig: "config = {}\n",
			steps: []mergeStep{{
				pkgConfig:    "config = {\"feature\": None}\n",
				version:      "1",
				wantPrompt:   false,
				wantConfig:   map[string]any{"feature": nil},
				wantContains: []string{"\"feature\": None"},
			}},
		},
		{
			name:       "yaml null value new key",
			configName: "config.yaml",
			userConfig: "{}\n",
			steps: []mergeStep{{
				pkgConfig:  "feature:\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"feature": nil},
			}},
		},
		{
			name:       "star user scalar vs overlay map preserves user no prompt",
			configName: "config.star",
			userConfig: "config = {\"x\": 1}\n",
			steps: []mergeStep{{
				pkgConfig:       "config = {\"x\": {\"y\": 2}}\n",
				version:         "1",
				wantPrompt:      false,
				wantConfig:      map[string]any{"x": 1},
				wantNotContains: []string{starlarkconfig.ManagedBegin},
			}},
		},
		{
			name:       "yaml user scalar vs overlay map preserves user no prompt",
			configName: "config.yaml",
			userConfig: "x: 1\n",
			steps: []mergeStep{{
				pkgConfig:  "x:\n  y: 2\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"x": 1},
			}},
		},
		{
			name:       "star user map vs overlay scalar preserves user no prompt",
			configName: "config.star",
			userConfig: "config = {\"x\": {\"y\": 2}}\n",
			steps: []mergeStep{{
				pkgConfig:       "config = {\"x\": 1}\n",
				version:         "1",
				wantPrompt:      false,
				wantConfig:      map[string]any{"x": map[string]any{"y": 2}},
				wantNotContains: []string{starlarkconfig.ManagedBegin},
			}},
		},
		{
			name:       "yaml user map vs overlay scalar preserves user no prompt",
			configName: "config.yaml",
			userConfig: "x:\n  y: 2\n",
			steps: []mergeStep{{
				pkgConfig:  "x: 1\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"x": map[string]any{"y": 2}},
			}},
		},
		{
			name:       "star mixed scalar types added",
			configName: "config.star",
			userConfig: "config = {}\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"b\": True, \"i\": 7, \"f\": 1.5, \"s\": \"hi\"}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"b": true, "i": 7, "f": 1.5, "s": "hi"},
			}},
		},
		{
			name:       "yaml mixed scalar types added",
			configName: "config.yaml",
			userConfig: "{}\n",
			steps: []mergeStep{{
				pkgConfig:  "b: true\ni: 7\nf: 1.5\ns: hi\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"b": true, "i": 7, "f": 1.5, "s": "hi"},
			}},
		},
		{
			name:       "star deep nested add leaf keeps sibling",
			configName: "config.star",
			userConfig: "config = {\"a\": {\"b\": {\"c\": 1}}}\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"a\": {\"b\": {\"d\": 2}}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"a": map[string]any{"b": map[string]any{"c": 1, "d": 2}},
				},
			}},
		},
		{
			name:       "yaml deep nested add leaf keeps sibling",
			configName: "config.yaml",
			userConfig: "a:\n  b:\n    c: 1\n",
			steps: []mergeStep{{
				pkgConfig:  "a:\n  b:\n    d: 2\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"a": map[string]any{"b": map[string]any{"c": 1, "d": 2}},
				},
			}},
		},
		{
			name:       "star empty overlay is a no-op",
			configName: "config.star",
			userConfig: "config = {\"a\": 1}\n",
			steps: []mergeStep{{
				pkgConfig:       "config = {}\n",
				version:         "1",
				wantPrompt:      false,
				wantConfig:      map[string]any{"a": 1},
				wantNotContains: []string{starlarkconfig.ManagedBegin},
			}},
		},
		{
			name:       "yaml empty overlay is a no-op",
			configName: "config.yaml",
			userConfig: "a: 1\n",
			steps: []mergeStep{{
				pkgConfig:  "{}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"a": 1},
			}},
		},
		{
			name:       "star subscript-mutated config preserved",
			configName: "config.star",
			userConfig: "config = {\"editor\": {}}\nconfig[\"editor\"][\"mode\"] = \"exo\"\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"editor": map[string]any{"mode": "exo"},
					"env":    map[string]any{"GOROOT": "/go"},
				},
				wantContains: []string{"config[\"editor\"][\"mode\"] = \"exo\""},
			}},
		},
		{
			// Bootstrap-generated user configs never bind `config`; they
			// only mutate the predeclared overlay dict (config[...] = ...).
			// Rune must read and update these without "undefined: config".
			name:       "star overlay-style config without binding",
			configName: "config.star",
			base:       map[string]any{"editor": map[string]any{"mode": "modeless"}},
			userConfig: "config[\"editor\"][\"mode\"] = \"modal\"\n",
			steps: []mergeStep{{
				pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{
					"editor": map[string]any{"mode": "modal"},
					"env":    map[string]any{"GOROOT": "/go"},
				},
				wantContains: []string{"config[\"editor\"][\"mode\"] = \"modal\""},
			}},
		},
		{
			name:       "star vdep nested keeps static sibling across bump",
			configName: "config.star",
			userConfig: "config = {}\n",
			steps: []mergeStep{
				{
					pkgConfig:  "config = {\"env\": {\"GOROOT\": RUNE_DATADIR + \"/\" + RUNE_PKG_VERSION, \"STATIC\": \"x\"}}\n",
					version:    "1",
					wantPrompt: false,
				},
				{
					pkgConfig:  "config = {\"env\": {\"GOROOT\": RUNE_DATADIR + \"/\" + RUNE_PKG_VERSION, \"STATIC\": \"y\"}}\n",
					version:    "2",
					wantPrompt: true,
				},
			},
		},
		{
			name:       "yaml vdep nested keeps static sibling across bump",
			configName: "config.yaml",
			userConfig: "{}\n",
			steps: []mergeStep{
				{
					pkgConfig:  "env:\n  GOROOT: $RUNE_DATADIR/$RUNE_PKG_VERSION\n  STATIC: x\n",
					version:    "1",
					wantPrompt: false,
				},
				{
					pkgConfig:  "env:\n  GOROOT: $RUNE_DATADIR/$RUNE_PKG_VERSION\n  STATIC: y\n",
					version:    "2",
					wantPrompt: true,
				},
			},
		},
		{
			name:       "star whitespace-only user config seeds and merges",
			configName: "config.star",
			userConfig: "\n   \n\t\n",
			steps: []mergeStep{{
				pkgConfig:    "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
				version:      "1",
				wantPrompt:   false,
				wantConfig:   map[string]any{"env": map[string]any{"GOROOT": "/go"}},
				wantContains: []string{"config = {}", starlarkconfig.ManagedBegin},
			}},
		},
		{
			name:       "yaml empty user config merges",
			configName: "config.yaml",
			userConfig: "",
			steps: []mergeStep{{
				pkgConfig:  "env:\n  GOROOT: /go\n",
				version:    "1",
				wantPrompt: false,
				wantConfig: map[string]any{"env": map[string]any{"GOROOT": "/go"}},
			}},
		},
		{
			name:       "star stray marker in user comment survives accumulation",
			configName: "config.star",
			userConfig: "config = {\"a\": 1}\n" +
				starlarkconfig.ManagedBegin + "\n# trailing user note\n",
			steps: []mergeStep{
				{
					pkgConfig:    "config = {\"b\": 2}\n",
					version:      "1",
					wantPrompt:   false,
					wantConfig:   map[string]any{"a": 1, "b": 2},
					wantContains: []string{"# trailing user note"},
				},
				{
					pkgConfig:    "config = {\"c\": 3}\n",
					version:      "1",
					wantPrompt:   false,
					wantConfig:   map[string]any{"a": 1, "b": 2, "c": 3},
					wantContains: []string{"# trailing user note"},
				},
			},
		},
		{
			name:       "star multi-extension accumulation three steps",
			configName: "config.star",
			userConfig: "config = {\"user\": \"keep\"}\n",
			steps: []mergeStep{
				{
					pkgConfig:  "config = {\"env\": {\"GOROOT\": \"/go\"}}\n",
					version:    "1",
					wantPrompt: false,
					wantConfig: map[string]any{
						"user": "keep",
						"env":  map[string]any{"GOROOT": "/go"},
					},
				},
				{
					pkgConfig:  "config = {\"settings\": {\"indent\": 4}}\n",
					version:    "1",
					wantPrompt: false,
					wantConfig: map[string]any{
						"user":     "keep",
						"env":      map[string]any{"GOROOT": "/go"},
						"settings": map[string]any{"indent": 4},
					},
				},
				{
					pkgConfig:  "config = {\"env\": {\"GOPATH\": \"/gp\"}}\n",
					version:    "1",
					wantPrompt: false,
					wantConfig: map[string]any{
						"user":     "keep",
						"env":      map[string]any{"GOROOT": "/go", "GOPATH": "/gp"},
						"settings": map[string]any{"indent": 4},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pkgs := idepkgtest.MakePackages()
			versions := idepkgtest.MakeBundles([]release.Bundle{
				{Package: "mpkg", Version: "1"},
				{Package: "mpkg", Version: "2"},
			})
			m, n, _, datadir := newTestManager(t, pkgs, versions)
			m.configPath = filepath.Join(datadir, tc.configName)
			if tc.base != nil {
				base := tc.base
				m.configBase = func() map[string]any { return deepCopyConfig(base) }
			}
			require.NoError(t, os.WriteFile(m.configPath, []byte(tc.userConfig), 0o644))

			var promptCount int
			m.wm = &mockWindowManager{
				floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
					promptCount++
					h.Resize(70, 20)
					h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
					return &mockWindow{}, nil
				},
			}

			pkgDir := t.TempDir()
			for i, step := range tc.steps {
				prev := promptCount
				pkgConfig := filepath.Join(pkgDir, tc.configName)
				require.NoError(t, os.WriteFile(pkgConfig, []byte(step.pkgConfig), 0o644))

				require.NoError(t, m.processConfig("mpkg", release.Version(step.version), pkgConfig),
					"step %d", i)
				n.RequireNoErrorNotification()

				if step.wantPrompt {
					assert.Equal(t, prev+1, promptCount, "step %d expected a prompt", i)
				} else {
					assert.Equal(t, prev, promptCount, "step %d expected no prompt", i)
				}

				if step.wantConfig != nil {
					got := readUserConfigMapBase(t, m.configPath, m.configBaseTree())
					assert.Equal(t, normalizeYAML(t, step.wantConfig), got, "step %d decoded config", i)
				}

				raw, err := os.ReadFile(m.configPath)
				require.NoError(t, err)
				for _, want := range step.wantContains {
					assert.Contains(t, string(raw), want, "step %d must contain", i)
				}
				for _, notWant := range step.wantNotContains {
					assert.NotContains(t, string(raw), notWant, "step %d must not contain", i)
				}
				// Rune renders exactly one managed block. Counting the
				// end marker (which only Rune emits) is robust to a user
				// who pasted a bare begin-marker line into a comment.
				assert.LessOrEqual(t, strings.Count(string(raw), starlarkconfig.ManagedEnd), 1,
					"step %d managed block duplicated", i)
			}
		})
	}

	// Version-dependent re-prompt must behave identically for .star and
	// .yaml overlays: a value that resolves differently across package
	// versions re-prompts and updates, while a value unchanged across the
	// bump does not. This is the .star counterpart of
	// TestProcessConfigRepromptsOnVersionDependentChange.
	t.Run("star version-dependent re-prompts and updates on bump", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
			{Package: "vpkg", Version: "2"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)
		m.configPath = filepath.Join(datadir, "config.star")
		require.NoError(t, os.WriteFile(m.configPath, []byte("config = {}\n"), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.star")
		src := "config = {\"env\": {\"GOROOT\": RUNE_DATADIR + \"/pkg/\" + RUNE_PKG_ID + \"/\" + RUNE_PKG_VERSION + \"/go\"}}\n"
		require.NoError(t, os.WriteFile(pkgConfig, []byte(src), 0o644))

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		assert.Equal(t, 0, promptCount, "first install auto-applies the new key without prompting")
		cfg := readUserConfigMap(t, m.configPath)
		env := cfg["env"].(map[string]any)
		assert.Equal(t, filepath.Join(datadir, "pkg", "vpkg", "1", "go"), fmt.Sprint(env["GOROOT"]))

		require.NoError(t, m.processConfig("vpkg", release.Version("2"), pkgConfig))
		assert.Equal(t, 1, promptCount, "version bump re-prompts the version-dependent key")
		cfg = readUserConfigMap(t, m.configPath)
		env = cfg["env"].(map[string]any)
		assert.Equal(t, filepath.Join(datadir, "pkg", "vpkg", "2", "go"), fmt.Sprint(env["GOROOT"]))

		raw, err := os.ReadFile(m.configPath)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(raw), starlarkconfig.ManagedBegin),
			"version bump updates in place without duplicating the managed block")
	})

	t.Run("star version-dependent does not re-prompt when unchanged", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
		})
		m, _, _, datadir := newTestManager(t, pkgs, versions)
		m.configPath = filepath.Join(datadir, "config.star")
		require.NoError(t, os.WriteFile(m.configPath, []byte("config = {}\n"), 0o644))

		var promptCount int
		m.wm = &mockWindowManager{
			floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
				promptCount++
				h.Resize(70, 20)
				h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
				return &mockWindow{}, nil
			},
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.star")
		src := "config = {\"env\": {\"GOROOT\": RUNE_DATADIR + \"/pkg/\" + RUNE_PKG_ID + \"/\" + RUNE_PKG_VERSION + \"/go\"}}\n"
		require.NoError(t, os.WriteFile(pkgConfig, []byte(src), 0o644))

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		assert.Equal(t, 0, promptCount, "unchanged version-dependent value must not re-prompt")
	})
}

func TestAfterConfigMergeHook(t *testing.T) {
	t.Parallel()

	t.Run("auto-apply invokes hook after config written", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "vpkg", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)
		require.NoError(t, os.WriteFile(m.configPath, []byte("existing: true\n"), 0o644))

		var events []ConfigMergeEvent
		var configOnDisk string
		m.afterConfigMerge = func(e ConfigMergeEvent) (ConfigMergeResult, error) {
			events = append(events, e)
			data, _ := os.ReadFile(m.configPath)
			configOnDisk = string(data)
			return ConfigMergeResult{LiveApplied: true}, nil
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"gui:\n  env:\n    FOO: bar\n"), 0o644))

		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))
		n.RequireNoErrorNotification()

		require.Len(t, events, 1)
		assert.Equal(t, "vpkg", events[0].PkgID)
		assert.Equal(t, release.Version("1"), events[0].PkgVersion)
		assert.True(t, events[0].TouchesPath("gui", "env"))
		assert.Contains(t, configOnDisk, "FOO",
			"hook must run after the merged config is written to disk")
	})

	t.Run("live-applied result yields env-oriented notification", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "vpkg", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)
		require.NoError(t, os.WriteFile(m.configPath, []byte("existing: true\n"), 0o644))
		m.afterConfigMerge = func(ConfigMergeEvent) (ConfigMergeResult, error) {
			return ConfigMergeResult{LiveApplied: true}, nil
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"gui:\n  env:\n    FOO: bar\n"), 0o644))
		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))

		require.True(t, hasNotificationContaining(n, "reload the workspace"))
		assert.False(t, hasNotificationContaining(n, "Restart the program"))
	})

	t.Run("non-live result keeps restart notification", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "vpkg", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)
		require.NoError(t, os.WriteFile(m.configPath, []byte("existing: true\n"), 0o644))
		m.afterConfigMerge = func(ConfigMergeEvent) (ConfigMergeResult, error) {
			return ConfigMergeResult{}, nil
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"settings:\n  theme: dark\n"), 0o644))
		require.NoError(t, m.processConfig("vpkg", release.Version("1"), pkgConfig))

		require.True(t, hasNotificationContaining(n, "Restart the program"))
	})

	t.Run("hook error propagates through auto-apply", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "vpkg", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)
		require.NoError(t, os.WriteFile(m.configPath, []byte("existing: true\n"), 0o644))
		m.afterConfigMerge = func(ConfigMergeEvent) (ConfigMergeResult, error) {
			return ConfigMergeResult{}, errors.New("boom")
		}

		pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(pkgConfig, []byte(
			"gui:\n  env:\n    FOO: bar\n"), 0o644))
		err := m.processConfig("vpkg", release.Version("1"), pkgConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("prompt allow invokes hook, deny does not", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{
			{Package: "vpkg", Version: "1"},
			{Package: "vpkg", Version: "2"},
		})

		run := func(t *testing.T, deny bool) int {
			m, n, _, datadir := newTestManager(t, pkgs, versions)
			v1GOROOT := filepath.Join(datadir, "pkg", "vpkg", "1", "go")
			require.NoError(t, os.WriteFile(m.configPath, []byte(
				"env:\n  GOROOT: "+v1GOROOT+"\n"), 0o644))

			var calls int
			m.afterConfigMerge = func(ConfigMergeEvent) (ConfigMergeResult, error) {
				calls++
				return ConfigMergeResult{}, nil
			}
			m.wm = &mockWindowManager{
				floatingFn: func(h browserapi.Floating, _ browserapi.FloatingConfig) (browserapi.Window, error) {
					h.Resize(70, 20)
					if deny {
						h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
					}
					h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
					return &mockWindow{}, nil
				},
			}

			pkgConfig := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(pkgConfig, []byte(
				"env:\n  GOROOT: $RUNE_DATADIR/pkg/$RUNE_PKG_ID/$RUNE_PKG_VERSION/go\n"), 0o644))
			require.NoError(t, m.processConfig("vpkg", release.Version("2"), pkgConfig))
			n.RequireNoErrorNotification()
			return calls
		}

		t.Run("allow", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, 1, run(t, false))
		})
		t.Run("deny", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, 0, run(t, true))
		})
	})
}

func hasNotificationContaining(n *idepkgtest.Notifications, substr string) bool {
	for _, noti := range n.Active() {
		if strings.Contains(noti.Msg, substr) {
			return true
		}
	}
	return false
}
