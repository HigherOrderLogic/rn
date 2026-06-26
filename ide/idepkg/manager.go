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
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ernestrc/go-multierror"
	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/blue/release/cdnrelease"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/term"
	"gopkg.in/yaml.v3"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide/starlarkconfig"
	"unstable.build/go-tui/workspace/walkdir"
)

// Sentinel errors returned by release-manager wrappers below so callers
// can branch on the kind of failure with errors.Is without parsing
// messages. The underlying cdnrelease error is preserved in the wrap
// chain for diagnostics.
var (
	// ErrPackageNotFound is returned when the server reports that a
	// requested package does not exist.
	ErrPackageNotFound = errors.New("package not found")
	// ErrVersionNotFound is returned when the server reports that a
	// requested version of a known package does not exist.
	ErrVersionNotFound = errors.New("package version not found")
	// ErrServerUnavailable is returned for transient (5xx) failures
	// from the package server or the signed-URL download backend.
	ErrServerUnavailable = errors.New("package server unavailable")
	// ErrForbidden is returned when the package server rejects the
	// caller with a 403 (no valid token or insufficient subscription).
	// The wrapped message is rendered directly to the user.
	ErrForbidden = errors.New("login first via `login` command and " +
		"ensure you have a valid subscription to download packages")
)

// translatePackageErr maps a *cdnrelease.StatusError on a
// package-scoped call into a user-facing wrapped sentinel. Errors
// without a recognisable status (network errors, non-StatusError
// wraps) are returned unchanged.
func translatePackageErr(err error, pkgID string) error {
	var se *cdnrelease.StatusError
	if !errors.As(err, &se) {
		return err
	}
	switch {
	case se.Status == http.StatusForbidden:
		return ErrForbidden
	case se.Status == http.StatusNotFound:
		return fmt.Errorf("package %q does not exist: %w", pkgID, ErrPackageNotFound)
	case se.Status >= 500:
		return fmt.Errorf("%w (status %d)", ErrServerUnavailable, se.Status)
	}
	return err
}

func translateVersionErr(err error, pkgID, version string) error {
	var se *cdnrelease.StatusError
	if !errors.As(err, &se) {
		return err
	}
	switch {
	case se.URL == "":
		// Signed-URL download from GCS failed
		return fmt.Errorf("download of %q version %q failed: %w (status %d)",
			pkgID, version, ErrServerUnavailable, se.Status)
	case se.Status == http.StatusForbidden:
		return ErrForbidden
	case se.Status == http.StatusNotFound:
		return fmt.Errorf("version %q of package %q does not exist: %w",
			version, pkgID, ErrVersionNotFound)
	case se.Status >= 500:
		return fmt.Errorf("%w (status %d)", ErrServerUnavailable, se.Status)
	}
	return err
}

func translateListErr(err error, pkgID string) error {
	var se *cdnrelease.StatusError
	if !errors.As(err, &se) {
		return err
	}
	switch {
	case se.Status == http.StatusForbidden:
		return ErrForbidden
	case se.Status == http.StatusNotFound && pkgID != "":
		return fmt.Errorf("package %q does not exist: %w", pkgID, ErrPackageNotFound)
	case se.Status >= 500:
		return fmt.Errorf("%w (status %d)", ErrServerUnavailable, se.Status)
	}
	return err
}

// NewManager allocates storage for a new Manager and initializes it.
// The dataDir argument will be used to store downloaded bundles
// and manage executables.
func NewManager(
	n browserapi.Notifications, m release.Manager,
	storage storageapi.Service, scheme schemeapi.Scheme, dataDir string,
	configPath string, wm browserapi.WindowManager,
	scheduleNextTick func(func()) bool,
	interrupter term.Interrupter, opts ...Option,
) *Manager {
	if dataDir == "" {
		panic("data directory must not be empty")
	}
	if configPath == "" {
		panic("config path must not be empty")
	}
	schemeURI, _ := scheme.URI(".")
	binDir := makeBinDirname(dataDir)
	ret := &Manager{
		dataDir:          dataDir,
		configPath:       configPath,
		scheme:           scheme,
		binDir:           binDir,
		schemeURI:        schemeURI,
		interrupter:      interrupter,
		frameCharSet:     component.FrameCharSetDefault(),
		wm:               wm,
		scheduleNextTick: scheduleNextTick,
		n:                n,
		m:                m,
		storage:          storage,
	}
	ret.iterators.m = make(map[string]*sync.Mutex)
	for _, opt := range opts {
		opt(ret)
	}
	return ret
}

// Manager implements ManagerInterface and creates the managed package dirs
// used to make downloaded executables available via PATH.
//
// Note that OS/system is managed by having a separate Manager that points
// to a different underlying release.Manager.
type Manager struct {
	n                browserapi.Notifications
	m                release.Manager
	interrupter      term.Interrupter
	wm               browserapi.WindowManager
	parser           syntaxapi.Parser
	frameCharSet     component.FrameCharSet
	scheduleNextTick func(func()) bool
	storage          storageapi.Service
	dataDir          string
	configPath       string
	scheme           schemeapi.Scheme
	schemeURI        workspaceapi.URI
	binDir           string

	editorMode string

	// configBase returns the editor's default config tree, used as the
	// predeclared `config` base when reading the user's .star config so
	// overlay-style mutations (config[...] = ...) resolve. May be nil.
	configBase func() map[string]any

	// afterConfigMerge, when set, is invoked after a package config merge is
	// written to the user config file. See WithAfterConfigMerge.
	afterConfigMerge func(ConfigMergeEvent) (ConfigMergeResult, error)

	iterators struct {
		sync.Mutex
		m map[string]*sync.Mutex
	}
}

// LibDir returns an iterator to the lib directory of the given package.
// The paths returned by the iterator are always absolute.
func (m *Manager) LibDir(ctx context.Context, pkgID string) (iterator.Iterator[string], error) {
	m.iterators.Lock()
	defer m.iterators.Unlock()

	libDir := makePackageLibDirname(m.dataDir, pkgID)
	ready, ok := m.iterators.m[pkgID]
	if ok {
		return newPendingIterator(ready, m.scheme, m.schemeURI, libDir), nil
	}

	_, err := os.Stat(libDir)
	if err != nil {
		return nil, ErrNotInstalled
	}

	return newReadyIterator(ctx, m.scheme, m.schemeURI, libDir), nil
}

// DescribePackage fetches a Package manifest.
func (m *Manager) DescribePackage(ctx context.Context, pkgID string) (release.Package, error) {
	pkgID = escapeString(pkgID)
	if pkgID == "" {
		return release.Package{}, errors.New("package id must not be empty")
	}
	pkg, err := m.m.GetPackage(ctx, pkgID)
	if err != nil {
		return release.Package{}, translatePackageErr(err, pkgID)
	}
	return pkg, nil
}

// DescribeRelease fetches a release bundle manifest.
func (m *Manager) DescribeRelease(ctx context.Context, pkgID string, version string) (
	release.Bundle, error,
) {
	pkgID = escapeString(pkgID)
	if pkgID == "" {
		return release.Bundle{}, errors.New("package id must not be empty")
	}
	if version == "" {
		return release.Bundle{}, errors.New("release version must not be empty")
	}
	b, err := m.m.Get(ctx, pkgID, release.Version(version),
		release.NopProgressWriter(io.Discard))
	if err != nil {
		return release.Bundle{}, translateVersionErr(err, pkgID, version)
	}
	return b, nil
}

// ListPackages lists all packages.
func (m *Manager) ListPackages(ctx context.Context, filters map[string]string) (
	iterator.Iterator[release.Package], error,
) {
	it, err := m.m.ListPackages(ctx, filters)
	if err != nil {
		return nil, translateListErr(err, "")
	}
	return it, nil
}

// ListPackageVersions lists all bundles of a package.
func (m *Manager) ListPackageVersions(ctx context.Context, pkgID string, filters map[string]string) (
	iterator.Iterator[release.Bundle], error,
) {
	pkgID = escapeString(pkgID)
	it, err := m.m.List(ctx, pkgID, filters)
	if err != nil {
		return nil, translateListErr(err, pkgID)
	}
	return it, nil
}

// InstallPackageVersion downloads and installs a package bundle by name
// and version, reporting progress via the ProgressWriter. It blocks
// until the install finishes and returns the first error encountered, so
// callers can report success or failure directly.
func (m *Manager) InstallPackageVersion(
	ctx context.Context, pkgID string, version release.Version,
	pw repl.ProgressWriter,
) error {
	if pkgID == "" || version == "" {
		return errors.New("package and version must not be empty")
	}
	if pw == nil {
		pw = repl.NopProgressWriter()
	}

	// ensure no one is being naughty
	pkgID = escapeString(pkgID)
	version = release.Version(escapeString(string(version)))

	m.iterators.Lock()
	_, ok := m.iterators.m[pkgID]
	if ok {
		m.log(log.InfoLevel, "there's already an ongoing install of package: %s", pkgID)
		m.iterators.Unlock()
		return nil
	}

	tarfile, err := os.CreateTemp("", "")
	if err != nil {
		m.iterators.Unlock()
		return fmt.Errorf("create temp: %w", err)
	}

	key := m.makeDownloadKey(pkgID, version)
	err = m.storage.Create(ctx, key, newPkgVersionValue(pkgID, version))
	if err != nil {
		if errors.Is(err, storageapi.ErrAlreadyExists) {
			var existing pkgVersionValue
			if getErr := m.storage.Get(ctx, key, &existing); getErr == nil && !existing.Complete {
				_ = m.storage.Delete(ctx, key)
				_ = os.RemoveAll(makePackageVersionDirname(m.dataDir, pkgID, version))
				_ = os.RemoveAll(makeStagingDirname(m.dataDir, pkgID, version))
				err = m.storage.Create(ctx, key, newPkgVersionValue(pkgID, version))
			}
			if err != nil {
				m.cleanupFile(tarfile)
				m.iterators.Unlock()
				return fmt.Errorf("version %s of package %s has "+
					"already been installed", version, pkgID)
			}
		} else {
			m.cleanupFile(tarfile)
			m.iterators.Unlock()
			return fmt.Errorf("store package version: %w", err)
		}
	}

	mu := new(sync.Mutex)
	m.iterators.m[pkgID] = mu
	mu.Lock() // block calls to iterator
	m.iterators.Unlock()

	return m.download(pkgID, version, tarfile, pw, key)
}

// DeletePackageVersion deletes a package version from local storage. This method is idempotent.
func (m *Manager) DeletePackageVersion(
	ctx context.Context, pkgID string, version release.Version, force bool,
) (ret error) {
	if pkgID == "" || version == "" {
		return errors.New("package and version must not be empty")
	}
	pkgID = escapeString(pkgID)
	version = release.Version(escapeString(string(version)))

	key := m.makeDownloadKey(pkgID, version)
	var val pkgVersionValue
	if err := m.storage.Get(ctx, key, &val); err != nil {
		if errors.Is(err, storageapi.ErrNotFound) {
			return ErrNotInstalled
		}
		return err
	}

	dirname, libdirname, isInUse, err := m.isPackageVersionInUse(pkgID, version)
	if err != nil {
		ret = multierror.Append(ret, err)
	}
	if isInUse && force {
		// remove lib + executables if package version was being used
		err = os.RemoveAll(libdirname)
		if err != nil {
			ret = multierror.Append(ret, err)
		}
		err = removeExecutables(val.Executables, m.binDir)
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	} else if isInUse {
		return ErrVersionInUse
	}

	if err := m.storage.Delete(ctx, key); err != nil {
		ret = multierror.Append(ret, err)
	}

	err = os.RemoveAll(dirname)
	if err != nil {
		ret = multierror.Append(ret, err)
	}
	if ret != nil {
		// best effort to try to keep delete retryable
		_ = m.storage.Set(ctx, key, val)
	}
	return
}

// DeletePackage deletes all bundles of the given package.
func (m *Manager) DeletePackage(
	ctx context.Context, pkgID string,
) error {
	if pkgID == "" {
		return errors.New("package and version must not be empty")
	}
	pkgID = escapeString(pkgID)
	it, err := m.ListInstalledPackageVersions(ctx, pkgID)
	if err != nil {
		return fmt.Errorf("list bundles: %w", err)
	}

	var versions []release.Version
	for {
		next, ok := it.Next(ctx)
		if !ok {
			break
		}

		versions = append(versions, next)
	}
	if err := it.Err(); err != nil {
		return fmt.Errorf("bundles iterator: %w", err)
	}
	if len(versions) == 0 {
		return ErrNotInstalled
	}

	errs := make([]error, len(versions))
	var wg sync.WaitGroup
	wg.Add(len(versions))
	for i := 0; i < len(versions); i++ {
		i := i
		go debug.CapturePanicReport(func() {
			version := versions[i]
			defer wg.Done()
			errs[i] = m.DeletePackageVersion(ctx, pkgID, version, true)
		})
	}
	wg.Wait()

	var ret error
	for _, err := range errs {
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	pkgDirname := filepath.Join(m.dataDir, "pkg", pkgID)
	if err := os.RemoveAll(pkgDirname); err != nil {
		ret = multierror.Append(ret, err)
	}
	return ret
}

// ListInstalledPackages lists the packages installed.
func (m *Manager) ListInstalledPackages(ctx context.Context) (
	iterator.Iterator[string], error,
) {
	dit, err := m.storage.List(ctx, nil)
	if err != nil {
		return nil, err
	}
	it := iterator.FromDocumentIterator[pkgVersionValue](dit)
	complete := iterator.Filter(it, func(p pkgVersionValue) bool {
		return p.Complete
	})
	mapped := iterator.Map(complete, func(p pkgVersionValue) string {
		return p.Package
	})
	seen := make(map[string]struct{})
	return iterator.Filter(mapped, func(pkg string) bool {
		if _, ok := seen[pkg]; ok {
			return false
		}
		seen[pkg] = struct{}{}
		return true
	}), nil
}

// ProcessInstalledSettings processes settings by packages.
func (m *Manager) ProcessInstalledSettings(ctx context.Context) (ret error) {
	dit, err := m.storage.List(ctx, nil)
	if err != nil {
		return err
	}
	it := iterator.FromDocumentIterator[pkgVersionValue](dit)
	slice, err := iterator.ToSlice(ctx, it)
	if err != nil {
		return err
	}
	for _, pkv := range slice {
		if !pkv.Complete {
			continue
		}
		_, _, isInUse, err := m.isPackageVersionInUse(pkv.Package, pkv.Version)
		if err != nil {
			err = fmt.Errorf("could not check if package %s version %s is in use: %v",
				pkv.Package, pkv.Version, err)
			ret = errors.Join(ret, err)
			continue
		}
		if !isInUse {
			continue
		}
		dir := makePackageVersionDirname(m.dataDir, pkv.Package, pkv.Version)
		configFile := pkgConfigFile(dir)
		err = m.processConfig(pkv.Package, pkv.Version, configFile)
		if err != nil {
			ret = errors.Join(ret, fmt.Errorf("process %s: %w", configFile, err))
		}
	}
	return ret
}

// ListInstalledPackageVersions lists the bundles installed for the given package.
func (m *Manager) ListInstalledPackageVersions(ctx context.Context, pkgID string) (
	iterator.Iterator[release.Version], error,
) {
	if pkgID == "" {
		return nil, errors.New("package must not be empty")
	}
	pkgID = escapeString(pkgID)
	dit, err := m.storage.List(ctx, []storageapi.Filter{{
		Field: storageapi.Field{
			FieldPath: []string{"Package"},
			Value:     pkgID,
		},
		Op: storageapi.OpEqual,
	}})
	if err != nil {
		return nil, err
	}
	it := iterator.FromDocumentIterator[pkgVersionValue](dit)
	complete := iterator.Filter(it, func(p pkgVersionValue) bool {
		return p.Complete
	})
	return iterator.Map(complete, func(p pkgVersionValue) release.Version {
		return p.Version
	}), nil
}

// UsePackageVersion updates the lib and bin directories to point to the given
// package version.
func (m *Manager) UsePackageVersion(
	ctx context.Context, pkgID string, version release.Version,
) error {
	if pkgID == "" {
		return errors.New("package must not be empty")
	}
	key := m.makeDownloadKey(pkgID, version)
	var val pkgVersionValue
	if err := m.storage.Get(ctx, key, &val); err != nil {
		if errors.Is(err, storageapi.ErrNotFound) {
			return ErrNotInstalled
		}
		return err
	}
	if !val.Complete {
		return ErrNotInstalled
	}
	_, _, isInUse, err := m.isPackageVersionInUse(pkgID, version)
	if err != nil {
		return err
	}
	if isInUse {
		return ErrVersionInUse
	}
	pkgID = escapeString(pkgID)
	pkgVersionDirname := makePackageVersionDirname(m.dataDir, pkgID, version)

	configFile := pkgConfigFile(pkgVersionDirname)
	err = m.processConfig(pkgID, version, configFile)
	if err != nil {
		return err
	}
	return m.linkLibCopyBin(pkgID, version, val.Executables, pkgVersionDirname)
}

// PackageVersionInUse returns the package version in use for the given package.
func (m *Manager) PackageVersionInUse(
	ctx context.Context, pkgID string,
) (release.Version, error) {
	if pkgID == "" {
		return "", errors.New("package must not be empty")
	}
	pkgID = escapeString(pkgID)
	it, err := m.m.List(ctx, pkgID, nil)
	if err != nil {
		return "", translateListErr(err, pkgID)
	}

	var versions []release.Version
	for {
		next, ok := it.Next(ctx)
		if !ok {
			break
		}

		versions = append(versions, next.Version)
	}
	if err := it.Err(); err != nil {
		return "", fmt.Errorf("bundles iterator: %w", err)
	}

	latest := release.Version(release.Latest)
	var inUse atomic.Value
	inUse.Store(latest)

	errs := make([]error, len(versions))
	var wg sync.WaitGroup
	wg.Add(len(versions))
	for i := 0; i < len(versions); i++ {
		version := versions[i]
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			var isInUse bool
			_, _, isInUse, errs[i] = m.isPackageVersionInUse(pkgID, version)
			if isInUse { // only one will be in use
				inUse.Store(version)
			}
		})
	}
	wg.Wait()

	var ret error
	for _, err := range errs {
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if ret != nil {
		return "", ret
	}

	versionInUse := inUse.Load().(release.Version)
	if versionInUse == latest {
		return "", errors.New("no versions of this package are currently in use")
	}
	return versionInUse, nil
}

func (m *Manager) isPackageVersionInUse(
	pkgID string, version release.Version,
) (dirname string, libdirname string, isInUse bool, err error) {
	dirname = makePackageVersionDirname(m.dataDir, pkgID, version)
	libdirname = makePackageLibDirname(m.dataDir, pkgID)
	infodirname, serr := os.Stat(dirname)
	infolibdirname, lerr := os.Stat(libdirname)
	if serr != nil || lerr != nil {
		return
	}
	isInUse = os.SameFile(infodirname, infolibdirname)
	return
}

func (m *Manager) cleanupFile(file *os.File) {
	if err := file.Close(); err != nil {
		m.log(log.WarnLevel, "close temp tar file: %v", err)
	}
	if err := os.Remove(file.Name()); err != nil {
		m.log(log.WarnLevel, "remove temp tar file: %v", err)
	}
}

func (m *Manager) makeDownloadKey(pkgID string, version release.Version) string {
	return fmt.Sprintf("%s:%s", pkgID, version)
}

func newPkgVersionValue(pkgID string, version release.Version) pkgVersionValue {
	return pkgVersionValue{Package: pkgID, Version: version}
}

func (m *Manager) download(
	pkgID string, version release.Version, tarfile *os.File,
	pw repl.ProgressWriter, key string,
) error {
	err := m.runDownload(pkgID, version, tarfile, pw, key)
	if err != nil {
		m.abortDownload(err, pkgID, version)
		return err
	}
	m.finishDownload(pkgID)
	return nil
}

// runDownload performs the fetch, extract, link and config steps for a
// single package version, returning the first error encountered. It owns
// the on-disk cleanup of partial state so download can keep the
// completion bookkeeping in one place.
func (m *Manager) runDownload(
	pkgID string, version release.Version, tarfile *os.File,
	pw repl.ProgressWriter, key string,
) error {
	ctx := context.Background()
	defer m.cleanupFile(tarfile)

	if err := makePkgDirs(m.dataDir); err != nil {
		return err
	}

	writer := &progressTarWriter{
		Writer: tarfile,
		pw:     pw,
	}
	m.log(log.TraceLevel, "fetching package %s version %s", pkgID, version)
	if _, err := m.m.Get(ctx, pkgID, version, writer); err != nil {
		return translateVersionErr(err, pkgID, string(version))
	}

	m.log(log.TraceLevel, "extracting package %s version %s", pkgID, version)
	// extract to staging dir then atomically rename to final dir
	stagingDir := makeStagingDirname(m.dataDir, pkgID, version)
	_ = os.RemoveAll(stagingDir)
	pkgVersionDirname := makePackageVersionDirname(m.dataDir, pkgID, version)
	_, executables, err := m.untar(tarfile, stagingDir, pw)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return err
	}

	_ = os.RemoveAll(pkgVersionDirname)
	if err := os.Rename(stagingDir, pkgVersionDirname); err != nil {
		_ = os.RemoveAll(stagingDir)
		return fmt.Errorf("rename staging dir: %w", err)
	}

	configFile := pkgConfigFile(pkgVersionDirname)

	if err := m.linkLibCopyBin(pkgID, version, executables, pkgVersionDirname); err != nil {
		_ = os.RemoveAll(pkgVersionDirname)
		return err
	}

	updates := []storageapi.Update{
		{FieldPath: []string{"Executables"}, Value: executables},
		{FieldPath: []string{"Complete"}, Value: true},
	}
	if err := m.storage.Update(ctx, key, updates); err != nil {
		_ = os.RemoveAll(pkgVersionDirname)
		_ = removeExecutables(executables, m.binDir)
		return fmt.Errorf("update storage field: %w", err)
	}

	if err := m.processConfig(pkgID, version, configFile); err != nil {
		return fmt.Errorf("process configuration for %s version %s: %w",
			pkgID, version, err)
	}

	pw.Progress(1, 1, "done")
	return nil
}

// finishDownload releases the per-package install gate after a
// successful install so blocked LibDir iterators can proceed.
func (m *Manager) finishDownload(pkgID string) {
	m.iterators.Lock()
	defer m.iterators.Unlock()

	ready, ok := m.iterators.m[pkgID]
	if !ok {
		panic("iterator for package not found")
	}
	delete(m.iterators.m, pkgID)
	ready.Unlock()
}

func (m *Manager) abortDownload(
	err error, pkgID string, version release.Version,
) {
	m.log(log.WarnLevel, "aborting installation of package %s version %s: %v",
		pkgID, version, err)
	key := m.makeDownloadKey(pkgID, version)
	if err := m.storage.Delete(context.Background(), key); err != nil {
		m.log(log.ErrorLevel, "delete pkg %s version %s "+
			"lock key (%s): %v", pkgID, version, key, err)
	}
	m.finishDownload(pkgID)
}

func (m *Manager) linkLibVersion(pkgID string, version release.Version) error {
	dirname := makePackageVersionDirname(m.dataDir, pkgID, version)
	libdirname := makePackageLibDirname(m.dataDir, pkgID)
	tmpLink := libdirname + ".tmp"
	_ = os.Remove(tmpLink)
	if err := os.Symlink(dirname, tmpLink); err != nil {
		return fmt.Errorf("symlink lib dir: %w", err)
	}
	if err := os.Rename(tmpLink, libdirname); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("rename lib symlink: %w", err)
	}
	return nil
}

func (m *Manager) linkLibCopyBin(
	pkgID string, version release.Version,
	executables []executableEntry, pkgVersionDirname string,
) error {
	m.log(log.TraceLevel, "linking package %s version %s library", pkgID, version)
	err := m.linkLibVersion(pkgID, version)
	if err != nil {
		return err
	}

	m.log(log.TraceLevel, "copying package %s version %s executables", pkgID, version)
	if err := copyExecutables(executables, pkgVersionDirname, m.binDir); err != nil {
		return err
	}
	return nil
}

func (m *Manager) promptConfigChange(
	pkgID string, pkgVersion release.Version, configYAML []byte,
	userDoc, pkgDoc *yaml.Node,
) error {
	message := fmt.Sprintf(
		"Extension %s (version %s) wants to **update** your configuration "+
			"with the following settings:\n\n```yaml\n%s\n```\n\nDo you want to allow this?",
		pkgID, pkgVersion, string(configYAML))

	prompt := handler.NewPrompt(handler.PromptConfig{
		HighlightAttr: term.Attributes{
			Attrs: term.AttrBold,
			Bg:    term.ColorRed,
		},
		OptionAttr: term.Attributes{
			Attrs: term.AttrBold,
			Bg:    term.ColorGray,
		},
		OptionBindings: []term.KeyComb{{Ch: 'a'}, {Ch: 'd'}},
		PromptConfig: component.PromptConfig{
			Message:    message,
			Options:    []string{"    Allow    ", "    Deny    "},
			NewMessage: markdownOrFallback(m.parser, m.scheduleNextTick),
		},
		PromptHandler: handler.FuncPromptHandler(func(idx int, _ string) {
			allowed := idx == 0
			if !allowed {
				return
			}
			result, err := m.applyConfigMerge(pkgID, pkgVersion, userDoc, pkgDoc)
			if err != nil {
				_, _ = m.n.Notify(browserapi.LevelError, "apply configuration: %s", err)
				return
			}
			m.notifyConfigApplied(browserapi.LevelSuccess, pkgID, result)
		}, func() error { return nil }),
	})

	ok := m.scheduleNextTick(func() {
		_, err := m.wm.Floating(prompt, browserapi.FloatingConfig{
			Alignment: component.AlignmentCentered,
		})
		if err != nil {
			_, _ = m.n.Notify(browserapi.LevelError, "show config prompt: %s", err)
		}
	})
	if !ok {
		m.log(log.ErrorLevel, "idepkg config prompt: could not schedule")
		return nil
	}

	return nil
}

// ConfigMergeEvent describes a package config merge that was just written to
// the user config file. The applied diff (Diff) carries only the keys that
// were merged. Consumers inspect it to decide whether any live reload is
// possible; the idepkg package itself is unaware of what the keys mean.
type ConfigMergeEvent struct {
	PkgID      string
	PkgVersion release.Version
	// Diff is the YAML document node for the applied diff. Its first content
	// child is the mapping of merged keys.
	Diff *yaml.Node
}

// TouchesPath reports whether the applied diff includes the given nested key
// path, e.g. TouchesPath("gui", "env").
func (e ConfigMergeEvent) TouchesPath(path ...string) bool {
	return configDiffTouchesPath(e.Diff, path...)
}

// AddedExtensionIDs returns the ids added under the top-level "extensions"
// key of the applied diff, or nil when the diff did not add any.
func (e ConfigMergeEvent) AddedExtensionIDs() []string {
	return addedExtensionIDs(e.Diff)
}

// ConfigMergeResult reports what a post-merge hook did. LiveApplied is true
// when the hook applied changes to the running process such that a full
// restart is not required for new work to observe them.
type ConfigMergeResult struct {
	LiveApplied bool
}

// applyConfigMerge deep-merges addDoc into userDoc and writes the result to
// the user config file atomically, after backing up the existing file. It is
// shared by the auto-apply path (purely-new keys) and the prompt's Allow path
// (version-dependent conflicts the user approved). On success it invokes the
// post-merge hook (if configured) and returns its result.
func (m *Manager) applyConfigMerge(
	pkgID string, pkgVersion release.Version, userDoc, addDoc *yaml.Node,
) (ConfigMergeResult, error) {
	starConfig := strings.HasSuffix(strings.ToLower(m.configPath), ".star")
	merged, err := buildMergedConfig(userDoc, addDoc, starConfig)
	if err != nil {
		return ConfigMergeResult{}, err
	}

	backup, err := backupUserConfig(m.configPath)
	if err != nil {
		return ConfigMergeResult{}, fmt.Errorf("backup user config: %w", err)
	}

	m.log(log.InfoLevel, "created config backup "+
		"before applying package updates: %s", backup)

	if starConfig {
		if err := starlarkconfig.WriteManagedConfigFileAtomic(m.configPath, merged.starDiff); err != nil {
			return ConfigMergeResult{}, fmt.Errorf("write starlark config: %w", err)
		}
		return m.runAfterConfigMerge(pkgID, pkgVersion, addDoc)
	}
	if err := writeYAMLAtomic(m.configPath, merged.yamlDoc, addDoc.Content[0]); err != nil {
		return ConfigMergeResult{}, fmt.Errorf("write config: %w", err)
	}
	return m.runAfterConfigMerge(pkgID, pkgVersion, addDoc)
}

func (m *Manager) runAfterConfigMerge(
	pkgID string, pkgVersion release.Version, addDoc *yaml.Node,
) (ConfigMergeResult, error) {
	if m.afterConfigMerge == nil {
		return ConfigMergeResult{}, nil
	}
	return m.afterConfigMerge(ConfigMergeEvent{
		PkgID:      pkgID,
		PkgVersion: pkgVersion,
		Diff:       addDoc,
	})
}

// notifyConfigApplied reports a successful config merge. When the post-merge
// hook live-applied changes, it tells the user that new local processes will
// pick them up and that already-running tools need a workspace reload;
// otherwise it keeps the restart-oriented wording.
func (m *Manager) notifyConfigApplied(
	level browserapi.NotificationLevel, pkgID string, result ConfigMergeResult,
) {
	if result.LiveApplied {
		_, _ = m.n.Notify(level, "applied %s configuration updates. "+
			"Environment updates are now active for new local processes; "+
			"reload the workspace to update already-running tools.", pkgID)
		return
	}
	_, _ = m.n.Notify(level, "applied %s configuration updates. "+
		"Restart the program to load the changes.", pkgID)
}

func (m *Manager) processConfig(
	pkgID string, pkgVersion release.Version, pkgConfigFile string,
) error {
	_, err := os.Stat(pkgConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat config: %v", err)
	}
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(pkgConfigFile)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if _, statErr := os.Stat(m.configPath); os.IsNotExist(statErr) {
		return nil
	}

	userCfg, err := loadIdePkgConfigFile(m.configPath, m.configBaseTree())
	if err != nil {
		return fmt.Errorf("load user config: %w", err)
	}

	plan, err := planConfigChange(
		pkgConfigFile, data, userCfg,
		pkgID, pkgVersion, m.dataDir, m.editorMode,
	)
	if err != nil {
		return err
	}

	if plan.autoApplyDoc != nil {
		result, err := m.applyConfigMerge(pkgID, pkgVersion, plan.userDoc, plan.autoApplyDoc)
		if err != nil {
			return fmt.Errorf("auto-apply config change: %w", err)
		}
		m.notifyConfigApplied(browserapi.LevelInfo, pkgID, result)
	}

	if plan.prompt {
		err = m.promptConfigChange(
			pkgID, pkgVersion, plan.missingYAML, plan.userDoc, plan.pkgDoc,
		)
		if err != nil {
			return fmt.Errorf("prompt config change: %w", err)
		}
	}

	return nil
}

func (m *Manager) untar(
	tarfile *os.File, dirname string, pw repl.ProgressWriter,
) (string, []executableEntry, error) {
	if err := os.MkdirAll(dirname, 0777); err != nil {
		err = fmt.Errorf("mkdir: %w", err)
		return "", nil, err
	}
	stat, err := tarfile.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("stat tarball file: %w", err)
	}
	totalBytes := stat.Size()
	_, err = tarfile.Seek(0, 0)
	if err != nil {
		err = fmt.Errorf("seek tarball file: %w", err)
		return "", nil, err
	}

	// Count compressed bytes read from disk so progress tracks
	// against tarfile size (the only total we know up front).
	counter := &countingReader{r: tarfile}
	gzr, err := gzip.NewReader(counter)
	if err != nil {
		err = fmt.Errorf("new gzip reader: %w", err)
		return "", nil, err
	}
	defer func() { _ = gzr.Close() }()

	executables, err := untar(dirname, gzr, func() {
		// Hold back the terminal extract sample so notification
		// writers that auto-dismiss on progress==total stay alive
		// until install actually completes.
		n := counter.n
		if n >= totalBytes {
			n = totalBytes - 1
		}
		scaledP, scaledT, unit := scaleBytes(n, totalBytes)
		pw.Progress(scaledP, scaledT, unit+" extracted")
	})
	if err != nil {
		err = fmt.Errorf("untar into %s: %w", dirname, err)
		return "", nil, err
	}

	configFile := pkgConfigFile(dirname)
	return configFile, executables, nil
}

func loadIdePkgConfigFile(path string, base map[string]any) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadIdePkgConfigFromBytes(path, data, base, "", "", "", "")
}

// configBaseTree returns the editor's default config tree to predeclare as
// `config` when reading the user's .star config, or nil when unavailable.
func (m *Manager) configBaseTree() map[string]any {
	if m.configBase == nil {
		return nil
	}
	return m.configBase()
}

// pkgConfigFile returns the path to the package's settings file inside dir.
// Packages may ship either config.yaml (legacy) or config.star (mode-aware);
// when both exist, config.yaml wins. The returned path may not exist on disk —
// callers must stat it themselves.
//
// Some tarballs nest everything under a single top-level wrapper directory
// rather than the bundle contents, so the config ends up at
// <dir>/<wrapper>/config.{yaml,star}. When no top-level config is present,
// descend into a lone subdirectory to find it.
func pkgConfigFile(dir string) string {
	if path, ok := configFileIn(dir); ok {
		return path
	}
	if sub, ok := loneSubdir(dir); ok {
		if path, ok := configFileIn(sub); ok {
			return path
		}
	}
	return filepath.Join(dir, "config.yaml")
}

// configFileIn reports the package config file directly inside dir, preferring
// config.yaml over config.star, and whether one exists.
func configFileIn(dir string) (string, bool) {
	yamlPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath, true
	}
	starPath := filepath.Join(dir, "config.star")
	if _, err := os.Stat(starPath); err == nil {
		return starPath, true
	}
	return "", false
}

// loneSubdir returns the path of dir's single subdirectory when dir contains
// exactly one entry and that entry is a directory.
func loneSubdir(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return "", false
	}
	return filepath.Join(dir, entries[0].Name()), true
}

func idePkgStarlarkParams(
	pkgID string, pkgVersion release.Version, dataDir string,
	editorMode string,
) map[string]any {
	params := map[string]any{}
	if dataDir != "" {
		params["RUNE_DATADIR"] = dataDir
	}
	if pkgID != "" {
		params["RUNE_PKG_ID"] = pkgID
	}
	if pkgVersion != "" {
		params["RUNE_PKG_VERSION"] = string(pkgVersion)
	}
	if editorMode != "" {
		params["RUNE_EDITOR_MODE"] = editorMode
	}
	return params
}

func loadIdePkgConfigFromBytes(
	filename string, data []byte, base map[string]any,
	pkgID string, pkgVersion release.Version, dataDir string,
	editorMode string,
) (map[string]any, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".star") {
		// User configs are authored as overlays that mutate a
		// predeclared `config` (config[...] = ...) and never bind it,
		// matching how the editor loads them. Decode in overlay mode
		// against the editor's default config tree so subscript
		// mutations on nested keys — and Rune's appended managed block's
		// reference to `config` — resolve without "undefined: config".
		// A nil base still predeclares `config` as an empty dict, so
		// empty or comments-only files yield an empty configuration.
		cfg, err := starlarkconfig.Decode(starlarkconfig.Source{
			Src:      data,
			Filename: filename,
			Params:   idePkgStarlarkParams(pkgID, pkgVersion, dataDir, editorMode),
			Base:     base,
		})
		if errors.Is(err, starlarkconfig.ErrMissingConfig) {
			return map[string]any{}, nil
		}
		return cfg, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return normalizeIdePkgConfig(cfg).(map[string]any), nil
}

func loadIdePkgConfigOverlay(
	filename string, data []byte, base map[string]any,
	pkgID string, pkgVersion release.Version, dataDir string,
	editorMode string,
) (map[string]any, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".star") {
		return starlarkconfig.Decode(starlarkconfig.Source{
			Src:      data,
			Filename: filename,
			Params:   idePkgStarlarkParams(pkgID, pkgVersion, dataDir, editorMode),
			Base:     base,
		})
	}
	return loadIdePkgConfigFromBytes(filename, data, base, pkgID, pkgVersion,
		dataDir, editorMode)
}

func normalizeIdePkgConfig(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeIdePkgConfig(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k.(string)] = normalizeIdePkgConfig(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeIdePkgConfig(val)
		}
		return out
	default:
		return t
	}
}

// idePkgConfigDiff classifies overlay keys against the user config into two
// disjoint subsets:
//
//   - newCfg: overlay key paths absent from the user config (safe additions,
//     including additive leaves under an existing parent map). These can be
//     auto-applied without prompting.
//   - conflictCfg: overlay leaves whose path already exists in the user config
//     with a different value, but only when the leaf is version-dependent — its
//     raw template references $RUNE_PKG_VERSION, so the value is
//     package-version-derived rather than a user customization, and a changed
//     resolved value is safe to re-offer (RUNE-225). The user must approve
//     these.
//
// For keys that are mappings on both sides it recurses, splitting sub-keys the
// same way. Either returned map is nil when its subset is empty.
//
// Static present scalars are always skipped so user customizations are
// preserved (RUNE-187). A key the user already has with the same value is
// neither new nor conflicting. versionDependent mirrors the overlay's nesting:
// scalar leaves are bool, nested maps are map[string]any.
func idePkgConfigDiff(
	user, overlay map[string]any, versionDependent map[string]any,
) (newCfg, conflictCfg map[string]any) {
	for key, overlayVal := range overlay {
		userVal, ok := user[key]
		if !ok {
			if newCfg == nil {
				newCfg = map[string]any{}
			}
			newCfg[key] = overlayVal
			continue
		}
		overlayMap, overlayIsMap := overlayVal.(map[string]any)
		userMap, userIsMap := userVal.(map[string]any)
		if !overlayIsMap || !userIsMap {
			if isVersionDependentScalar(versionDependent, key) &&
				fmt.Sprint(overlayVal) != fmt.Sprint(userVal) {
				if conflictCfg == nil {
					conflictCfg = map[string]any{}
				}
				conflictCfg[key] = overlayVal
			}
			continue
		}
		nestedVersionDependent, _ := versionDependent[key].(map[string]any)
		nestedNew, nestedConflict := idePkgConfigDiff(userMap, overlayMap, nestedVersionDependent)
		if nestedNew != nil {
			if newCfg == nil {
				newCfg = map[string]any{}
			}
			newCfg[key] = nestedNew
		}
		if nestedConflict != nil {
			if conflictCfg == nil {
				conflictCfg = map[string]any{}
			}
			conflictCfg[key] = nestedConflict
		}
	}
	return newCfg, conflictCfg
}

func isVersionDependentScalar(versionDependent map[string]any, key string) bool {
	dep, ok := versionDependent[key].(bool)
	return ok && dep
}

// versionDependentKeys walks the raw (pre-expansion) overlay and returns a
// structure mirroring its nesting that marks scalar leaves whose template
// references $RUNE_PKG_VERSION. Such values change across package versions,
// so a changed resolved value should re-prompt (RUNE-225). It must be
// computed before expandMapValues replaces the template with its value.
//
// This handles the YAML overlay path, where the raw $RUNE_PKG_VERSION
// template survives in the decoded map. The .star path resolves the
// template during decode, so its version-dependent leaves are detected
// separately by versionDependentByDecode.
func versionDependentKeys(overlay map[string]any) map[string]any {
	var out map[string]any
	for key, val := range overlay {
		switch t := val.(type) {
		case map[string]any:
			nested := versionDependentKeys(t)
			if nested == nil {
				continue
			}
			if out == nil {
				out = map[string]any{}
			}
			out[key] = nested
		case string:
			if !referencesPkgVersion(t) {
				continue
			}
			if out == nil {
				out = map[string]any{}
			}
			out[key] = true
		}
	}
	return out
}

// versionDependentSentinel is an implausible package version used to
// re-resolve a .star overlay so that leaves whose value depends on
// $RUNE_PKG_VERSION can be detected by comparison.
const versionDependentSentinel = "\x00rune-version-sentinel\x00"

// versionDependentOverlayKeys returns the version-dependent structure for
// the overlay, dispatching by format so .star and YAML re-prompt
// identically on version bumps (RUNE-225). overlay is the overlay decoded
// for the real version; for YAML it still carries raw $RUNE_PKG_VERSION
// templates, while for .star the template is already resolved and must be
// detected by decoding a second time with a sentinel version.
func versionDependentOverlayKeys(
	filename string, data []byte, overlay map[string]any,
	pkgID string, dataDir, editorMode string,
) (map[string]any, error) {
	if !strings.HasSuffix(strings.ToLower(filename), ".star") {
		return versionDependentKeys(overlay), nil
	}
	sentinel, err := loadIdePkgConfigOverlay(
		filename, data, map[string]any{},
		pkgID, versionDependentSentinel, dataDir, editorMode,
	)
	if err != nil {
		return nil, fmt.Errorf("decode package config (version probe): %w", err)
	}
	return versionDependentByDecode(overlay, sentinel), nil
}

// versionDependentByDecode returns a structure mirroring real's nesting
// that marks every scalar leaf whose value differs from sentinel, i.e. the
// leaves that changed solely because RUNE_PKG_VERSION changed.
func versionDependentByDecode(real, sentinel map[string]any) map[string]any {
	var out map[string]any
	for key, realVal := range real {
		sentinelVal, ok := sentinel[key]
		if !ok {
			continue
		}
		realMap, realIsMap := realVal.(map[string]any)
		sentinelMap, sentinelIsMap := sentinelVal.(map[string]any)
		if realIsMap && sentinelIsMap {
			nested := versionDependentByDecode(realMap, sentinelMap)
			if nested == nil {
				continue
			}
			if out == nil {
				out = map[string]any{}
			}
			out[key] = nested
			continue
		}
		if realIsMap || sentinelIsMap {
			continue
		}
		if fmt.Sprint(realVal) == fmt.Sprint(sentinelVal) {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[key] = true
	}
	return out
}

// referencesPkgVersion reports whether s expands $RUNE_PKG_VERSION (in
// either $VAR or ${VAR} form), using os.Expand so detection matches the
// expansion semantics applied by expandMapValues.
func referencesPkgVersion(s string) bool {
	var found bool
	os.Expand(s, func(name string) string {
		if name == "RUNE_PKG_VERSION" {
			found = true
		}
		return ""
	})
	return found
}

func mapToYAMLDocument(cfg map[string]any) (*yaml.Node, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func loadIdePkgConfigFromYAMLDoc(doc *yaml.Node) (map[string]any, error) {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return normalizeIdePkgConfig(cfg).(map[string]any), nil
}

func newReadyIterator(
	ctx context.Context, scheme schemeapi.Scheme,
	schemeURI workspaceapi.URI, libDir string,
) iterator.Iterator[string] {
	it, err := walkdir.ListFiles(ctx, scheme, libDir)
	if err != nil {
		return iterator.Error[string](fmt.Errorf("list files: %v", err))
	}
	// make paths absolute
	return iterator.Map(it, func(filename string) string {
		path, _ := workspaceapi.ExpandPathWithURI(filename, schemeURI)
		return path
	})
}

func (m *Manager) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "idepkg.Manager").Logf(level, msg, args...)
}

// progressTarWriter composes a tarfile io.Writer with a
// caller-supplied repl.ProgressWriter to satisfy
// release.ProgressWriter. Raw byte counts are scaled to the unit
// best matching total so callers see "12.4 / 120.0 MiB downloaded"
// instead of an unreadable byte count. The terminal download
// sample (progress==total) is held back so notification-backed
// writers don't auto-dismiss before the extract phase runs.
type progressTarWriter struct {
	io.Writer
	pw repl.ProgressWriter
}

func (w *progressTarWriter) Progress(progress, total int64, units string) {
	if total <= 0 || progress > total {
		return
	}
	if progress == total {
		return
	}
	scaledP, scaledT, unit := scaleBytes(progress, total)
	w.pw.Progress(scaledP, scaledT, unit+" downloaded")
}

// scaleBytes picks a human-readable byte unit based on total and
// returns progress/total scaled to that unit. The unit is picked
// from total so it stays stable across successive samples.
func scaleBytes(progress, total int64) (int64, int64, string) {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
		tib = gib * 1024
	)
	switch {
	case total >= tib:
		return progress / tib, total / tib, "TiB"
	case total >= gib:
		return progress / gib, total / gib, "GiB"
	case total >= mib:
		return progress / mib, total / mib, "MiB"
	case total >= kib:
		return progress / kib, total / kib, "KiB"
	default:
		return progress, total, "B"
	}
}

// countingReader wraps an io.Reader to track the total number of
// bytes consumed. It is used to report extraction progress against
// the on-disk tarfile size — the only total known up front, since
// tar headers don't expose entry counts.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func isExecutable(info fs.FileInfo) bool {
	// check owner/group/other exec bits, if any
	// match then the file is an executable.
	return info.Mode()&os.ModeType == 0 && info.Mode()&0111 != 0
}

func untar(dst string, r io.Reader, onProgress func()) ([]executableEntry, error) {
	tr := tar.NewReader(r)

	var executables []executableEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar next: %w", err)
		}
		if onProgress != nil {
			onProgress()
		}

		target := filepath.Join(dst, filepath.Clean(hdr.Name))
		if isExecutable(hdr.FileInfo()) && !isHidden(hdr.Name) {
			executables = append(executables, executableEntry{
				Name: hdr.Name,
				Mode: hdr.Mode,
			})
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				return nil, fmt.Errorf("make dir %s: %w", target, err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return nil, fmt.Errorf("make parent dirs for symlink %s: %w", target, err)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return nil, fmt.Errorf("symlink %s -> %s: %w", target, hdr.Linkname, err)
			}
		case tar.TypeLink:
			/* hard links are ignored */
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return nil, fmt.Errorf("make parent dirs: %w", err)
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC,
				hdr.FileInfo().Mode())
			if err != nil {
				return nil, fmt.Errorf("create file %s: %w", target, err)
			}
			_, err = io.Copy(f, tr)
			_ = f.Close()
			if err != nil {
				return nil, fmt.Errorf("write file %s: %w", target, err)
			}
		}
	}
	return executables, nil
}

func copyExecutables(files []executableEntry, dirname, targetdirname string) error {
	var ret error
	for _, executable := range files {
		name := filepath.Clean(executable.Name)
		orig := filepath.Join(dirname, name)
		origfile, err := os.OpenFile(orig, os.O_RDONLY, 0)
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("open executable: %w", err))
			continue
		}
		if err := swapExecutable(origfile, targetdirname, name, executable.Mode); err != nil {
			ret = multierror.Append(ret, err)
		}
		_ = origfile.Close()
	}
	return ret
}

// swapExecutable installs orig into targetdirname by writing a temp
// file and atomically renaming it over the destination. Overwriting in
// place (O_TRUNC) mutates the inode of an already-running binary, which
// macOS Gatekeeper/AMFI detects and kills for unsigned extensions;
// renaming installs a fresh inode and leaves the running process
// untouched.
func swapExecutable(orig *os.File, targetdirname, name string, mode int64) error {
	target := filepath.Join(targetdirname, filepath.Base(name))
	tmp, err := os.CreateTemp(targetdirname, filepath.Base(name)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp executable for %s: %w", target, err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, orig); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("copy executable %s: %w", target, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp executable for %s: %w", target, err)
	}
	if err := os.Chmod(tmpName, os.FileMode(mode)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod executable %s: %w", target, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename executable %s: %w", target, err)
	}
	if err := clearQuarantine(target); err != nil {
		return fmt.Errorf("clear quarantine on %s: %w", target, err)
	}
	return nil
}

func isHidden(file string) bool {
	return strings.HasPrefix(filepath.Base(file), ".")
}

func removeExecutables(files []executableEntry, targetdirname string) error {
	var ret error
	for _, executable := range files {
		name := filepath.Clean(executable.Name)
		target := filepath.Join(targetdirname, filepath.Base(name))
		err := os.Remove(target)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			ret = multierror.Append(ret, fmt.Errorf("remove executable %s: %w", target, err))
			continue
		}
	}
	return ret
}

func makeBinDirname(dataDir string) string {
	return filepath.Join(dataDir, "bin")
}

func makeLibDirname(dataDir string) string {
	return filepath.Join(dataDir, "lib")
}

func makePackageVersionDirname(
	dataDir, pkgID string, version release.Version,
) string {
	return filepath.Join(dataDir, "pkg", pkgID, string(version))
}

func makePkgDirs(dataDir string) error {
	binDir := makeBinDirname(dataDir)
	libDir := makeLibDirname(dataDir)
	targets := []string{binDir, libDir}
	for _, target := range targets {
		err := os.MkdirAll(target, 0777)
		if err != nil {
			return fmt.Errorf("mkdir dir %s: %w", target, err)
		}
	}
	return nil
}

func makePackageLibDirname(
	dataDir, pkgID string,
) string {
	return filepath.Join(dataDir, "lib", pkgID)
}

func makeStagingDirname(dataDir, pkgID string, version release.Version) string {
	return filepath.Join(dataDir, "pkg", pkgID, ".staging-"+string(version))
}

type pkgVersionValue struct {
	Package     string
	Version     release.Version
	Executables []executableEntry
	Complete    bool
}

// executableEntry is the UTF-8-safe representation of an executable file
// extracted from a package tarball. The raw [tar.Header] cannot be persisted
// directly because PAX records (for example macOS's
// "com.apple.provenance" xattr) may contain non-UTF-8 bytes
type executableEntry struct {
	Name string
	Mode int64
}

func escapeString(val string) string {
	val = url.PathEscape(val)
	val = strings.ReplaceAll(val, ":", "_")
	return val
}

// Reconcile cleans up incomplete installs left by a previous crash.
// It should be called once at startup, before any new installs.
func (m *Manager) Reconcile(ctx context.Context) error {
	// Phase 1: Clean leftover staging directories.
	pkgRoot := filepath.Join(m.dataDir, "pkg")
	if entries, err := os.ReadDir(pkgRoot); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pkgDir := filepath.Join(pkgRoot, entry.Name())
			subEntries, err := os.ReadDir(pkgDir)
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if strings.HasPrefix(sub.Name(), ".staging-") {
					_ = os.RemoveAll(filepath.Join(pkgDir, sub.Name()))
				}
			}
		}
	}

	// Phase 2: Clean stale storage entries (Complete == false).
	dit, err := m.storage.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("list storage entries: %w", err)
	}
	it := iterator.FromDocumentIterator[pkgVersionValue](dit)
	entries, err := iterator.ToSlice(ctx, it)
	if err != nil {
		return fmt.Errorf("read storage entries: %w", err)
	}
	for _, pkv := range entries {
		if pkv.Package == "" || pkv.Version == "" {
			continue
		}
		key := m.makeDownloadKey(pkv.Package, pkv.Version)
		dirname := makePackageVersionDirname(m.dataDir, pkv.Package, pkv.Version)
		if !pkv.Complete {
			_ = os.RemoveAll(dirname)
			// Remove lib symlink if it points to the stale version.
			libdirname := makePackageLibDirname(m.dataDir, pkv.Package)
			if target, lerr := os.Readlink(libdirname); lerr == nil {
				if target == dirname {
					_ = os.Remove(libdirname)
					_ = removeExecutables(pkv.Executables, m.binDir)
				}
			}
			_ = m.storage.Delete(ctx, key)
			continue
		}
		// Phase 4: Verify complete entries — if dir is missing, delete storage.
		if _, serr := os.Stat(dirname); os.IsNotExist(serr) {
			_ = m.storage.Delete(ctx, key)
		}
	}

	// Phase 3: Clean stale .tmp symlinks in lib/.
	libRoot := makeLibDirname(m.dataDir)
	if libEntries, err := os.ReadDir(libRoot); err == nil {
		for _, entry := range libEntries {
			if strings.HasSuffix(entry.Name(), ".tmp") {
				_ = os.Remove(filepath.Join(libRoot, entry.Name()))
			}
		}
	}

	return nil
}

type libDirIterator struct {
	ready     *sync.Mutex
	it        iterator.Iterator[string]
	scheme    schemeapi.Scheme
	schemeURI workspaceapi.URI
	libDir    string
}

func newPendingIterator(
	mu *sync.Mutex, scheme schemeapi.Scheme,
	schemeURI workspaceapi.URI, libDir string,
) *libDirIterator {
	return &libDirIterator{
		ready:     mu,
		scheme:    scheme,
		schemeURI: schemeURI,
		libDir:    libDir,
	}
}

func (l *libDirIterator) Next(ctx context.Context) (string, bool) {
	l.ready.Lock()
	defer l.ready.Unlock()
	if l.it == nil {
		l.it = newReadyIterator(context.Background(), l.scheme, l.schemeURI, l.libDir)
	}
	return l.it.Next(ctx)
}

func (l *libDirIterator) Err() error {
	l.ready.Lock()
	defer l.ready.Unlock()
	if l.it == nil {
		l.it = newReadyIterator(context.Background(), l.scheme, l.schemeURI, l.libDir)
	}
	return l.it.Err()
}

func (l *libDirIterator) Close() error {
	l.ready.Lock()
	defer l.ready.Unlock()
	if l.it == nil {
		return nil
	}
	return l.it.Close()
}
