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
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/walkdir"
)

func TestLibDir(t *testing.T) {
	t.Parallel()
	t.Run("returns installed files", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

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

		err := m.InstallPackageVersion(context.Background(), "go", "1")
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

		err := m.InstallPackageVersion(context.Background(), "go", "1")
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

func TestInstallPackageVersion(t *testing.T) {
	t.Parallel()
	t.Run("returns error if pkg is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles()
		m, _, _, _ := newTestManager(t, pkgs, versions)
		err := m.InstallPackageVersion(context.Background(), "", "1")
		assert.Error(t, err)
	})

	t.Run("returns error if version is empty", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, _, _, _ := newTestManager(t, pkgs, versions)
		err := m.InstallPackageVersion(context.Background(), "go", "")
		assert.Error(t, err)
	})

	t.Run("installs a package with executables", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()
		n.RequireNoErrorNotification()

		assertDataDirExists(t, datadir, "go")
		assertExecutables(t, datadir, goTarExpectedExecutables...)
	})

	t.Run("install a package and version already installed fails", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "go", Version: "1"}})
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()
		n.RequireNoErrorNotification()

		err = m.InstallPackageVersion(context.Background(), "go", "1")
		assert.Error(t, err)
	})

	t.Run("installs a package without executables", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages()
		versions := idepkgtest.MakeBundles([]release.Bundle{{Package: "testpkg", Version: "1"}})
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "testpkg", "1")
		require.NoError(t, err)
		n.Wg.Wait()
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
		n.ExpectErrorNotification = true
		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()
		n.RequireErrorNotification()

		r.ExpectReturnErr(nil)
		n.Reset()
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()
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

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()
		n.RequireNoErrorNotification()
		assert.Len(t, n.Active(), 1)

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		it, err := m.ListInstalledPackages(context.Background())
		require.NoError(t, err)

		installed, err := iterator.ToSlice(context.Background(), it)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"go"}, installed)
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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, _ := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		m, n, _, datadir := newTestManager(t, pkgs, versions)

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err := m.InstallPackageVersion(context.Background(), "go", "1")
		require.NoError(t, err)
		n.Wg.Wait()

		n.Wg = new(sync.WaitGroup)
		n.Wg.Add(1)
		err = m.InstallPackageVersion(context.Background(), "go", "2")
		require.NoError(t, err)
		n.Wg.Wait()

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
		info, err = os.Stat(filepath.Join(datadir, "lib", pkg))
		require.Error(t, err)

		info, err = os.Stat(filepath.Join(datadir, "pkg", pkg))
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
	uri, err := workspaceapi.ParseURI("file:///" + bindir)
	require.NoError(t, err)
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	require.NoError(t, err)
	it, err := walkdir.ListFiles(context.Background(), scheme, ".")
	require.NoError(t, err)
	files, err := iterator.ToSlice(context.Background(), it)
	require.NoError(t, err)
	return files
}

func newTestManager(
	t *testing.T,
	packages map[string]release.Package,
	versions map[string][]release.Bundle,
) (*Manager, *idepkgtest.Notifications, *idepkgtest.ReleaseManager, string) {
	ctx := context.Background()
	temp, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(temp)
	})
	n := idepkgtest.NewNotifications(t)
	m := idepkgtest.NewReleaseManager(packages, versions)
	tempURI, err := workspaceapi.ParseURI("file://" + temp)
	require.NoError(t, err)
	fileScheme, err := workspace.NewFileScheme(ctx, config.NopConfig(), tempURI)
	require.NoError(t, err)
	manager := NewManager(n, m, document.NewInMemoryService(),
		fileScheme, temp, term.NopInterrupter())
	return manager, n, m, temp
}
