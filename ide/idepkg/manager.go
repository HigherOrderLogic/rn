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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ernestrc/go-multierror"
	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/workspace/walkdir"
)

// NewManager allocates storage for a new Manager and initializes it.
// The dataDir argument will be used to store downloaded bundles
// and manage executables.
func NewManager(
	n browserapi.Notifications, m release.Manager,
	storage document.Service, scheme schemeapi.Scheme, dataDir string,
	interrupter term.Interrupter,
) *Manager {
	if dataDir == "" {
		panic("data directory must not be empty")
	}
	schemeURI, _ := scheme.URI(".")
	binDir := makeBinDirname(dataDir)
	ret := &Manager{
		dataDir:     dataDir,
		scheme:      scheme,
		binDir:      binDir,
		schemeURI:   schemeURI,
		interrupter: interrupter,
		n:           n,
		m:           m,
		storage:     storage,
	}
	ret.iterators.m = make(map[string]*sync.Mutex)
	return ret
}

// Manager implements ManagerInterface and adds SetPathEnv, which can be used
// to make available downloaded executables via PATH setting.
//
// Note that OS/system is managed by having a separate Manager that points
// to a different underlying release.Manager.
type Manager struct {
	n           browserapi.Notifications
	m           release.Manager
	interrupter term.Interrupter
	storage     document.Service
	dataDir     string
	scheme      schemeapi.Scheme
	schemeURI   workspaceapi.URI
	binDir      string

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
	return m.m.GetPackage(ctx, pkgID)
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
	return m.m.Get(ctx, pkgID, release.Version(version),
		release.NopProgressWriter(io.Discard))
}

// ListPackages lists all packages.
func (m *Manager) ListPackages(ctx context.Context, filters map[string]string) (
	iterator.Iterator[release.Package], error,
) {
	return m.m.ListPackages(ctx, filters)
}

// ListPackageVersions lists all bundles of a package.
func (m *Manager) ListPackageVersions(ctx context.Context, pkgID string, filters map[string]string) (
	iterator.Iterator[release.Bundle], error,
) {
	pkgID = escapeString(pkgID)
	return m.m.List(ctx, pkgID, filters)
}

// InstallPackageVersion downloads a Package bundle by package name and version, reports
// progress via ProgressWriter and returns a release.Bundle and a dirname
// that contains the extracted bundle.
func (m *Manager) InstallPackageVersion(
	ctx context.Context, pkgID string, version release.Version,
) error {
	if pkgID == "" || version == "" {
		return errors.New("package and version must not be empty")
	}

	// ensure no one is being naughty
	pkgID = escapeString(pkgID)
	version = release.Version(escapeString(string(version)))

	m.iterators.Lock()
	defer m.iterators.Unlock()

	_, ok := m.iterators.m[pkgID]
	if ok {
		m.log(log.InfoLevel, "there's already an ongoing install of package: %s", pkgID)
		return nil
	}

	tarfile, err := os.CreateTemp("", "")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	key := m.makeDownloadKey(pkgID, version)
	err = m.storage.Create(ctx, key, pkgVersionValue{Package: pkgID, Version: version})
	if err != nil {
		if errors.Is(err, document.ErrAlreadyExists) {
			return fmt.Errorf("version %s of package %s has "+
				"already been installed", version, pkgID)
		}
		m.cleanupFile(tarfile)
		return fmt.Errorf("store package version: %w", err)
	}

	notificationID, err := m.n.Notify(browserapi.LevelInfo,
		"downloading version %s of package %s", version, pkgID)
	if err != nil {
		m.cleanupFile(tarfile)
		_ = m.storage.Delete(ctx, key)
		return fmt.Errorf("notify: %w", err)
	}

	mu := new(sync.Mutex)
	m.iterators.m[pkgID] = mu

	mu.Lock() // block calls to iterator
	go debug.CapturePanicReport(func() {
		m.download(pkgID, version, tarfile, notificationID, key)
	})

	return err
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
		if errors.Is(err, document.ErrNotFound) {
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
	return iterator.Map(it, func(p pkgVersionValue) string {
		return p.Package
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
		dir := makePackageVersionDirname(m.dataDir, pkv.Package, pkv.Version)
		settings := filepath.Join(dir, "settings.json")
		err := m.processSettings(pkv.Package, pkv.Version, settings)
		if err != nil {
			ret = errors.Join(ret, fmt.Errorf("process %s: %w", settings, err))
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
	dit, err := m.storage.List(ctx, []document.Filter{
		{
			Field: document.Field{
				FieldPath: []string{"Package"},
				Value:     pkgID,
			},
			Op: document.OpEqual,
		},
	})
	if err != nil {
		return nil, err
	}
	it := iterator.FromDocumentIterator[pkgVersionValue](dit)
	return iterator.Map(it, func(p pkgVersionValue) release.Version {
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
		if errors.Is(err, document.ErrNotFound) {
			return ErrNotInstalled
		}
		return err
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
		return "", fmt.Errorf("list bundles: %w", err)
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

func (m *Manager) download(
	pkgID string, version release.Version, tarfile *os.File,
	notificationID string, key string,
) {
	ctx := context.Background()
	defer m.cleanupFile(tarfile)

	if err := makePkgDirs(m.dataDir); err != nil {
		m.abortDownload(err, pkgID, version, notificationID)
		return
	}

	writer := &notificationsProgressWriter{
		Writer:         tarfile,
		m:              m,
		notificationID: notificationID,
		pkgID:          pkgID,
		version:        version,
	}
	m.log(log.TraceLevel, "fetching package %s version %s", pkgID, version)
	_, err := m.m.Get(ctx, pkgID, version, writer)
	if err != nil {
		err = fmt.Errorf("download: %w", err)
		m.abortDownload(err, pkgID, version, notificationID)
		return
	}

	m.log(log.TraceLevel, "extracting package %s version %s", pkgID, version)
	// copy to pkg/<pkgID>/<version> for managing versions
	pkgVersionDirname := makePackageVersionDirname(m.dataDir, pkgID, version)
	settings, executables, err := m.untar(tarfile, pkgVersionDirname)
	if err != nil {
		m.abortDownload(err, pkgID, version, notificationID)
		return
	}

	err = m.processSettings(pkgID, version, settings)
	if err != nil {
		_ = os.RemoveAll(pkgVersionDirname)
		m.abortDownload(err, pkgID, version, notificationID)
		return
	}

	err = m.linkLibCopyBin(pkgID, version, executables, pkgVersionDirname)
	if err != nil {
		_ = os.RemoveAll(pkgVersionDirname)
		m.abortDownload(err, pkgID, version, notificationID)
		return
	}

	updates := []document.Update{{FieldPath: []string{"Executables"}, Value: executables}}
	if err := m.storage.Update(ctx, key, updates); err != nil {
		err = fmt.Errorf("update storage field: %w", err)
		_ = os.RemoveAll(pkgVersionDirname)
		_ = removeExecutables(executables, m.binDir)
		m.abortDownload(err, pkgID, version, notificationID)
		return
	}

	if !writer.completeProgress {
		_ = m.n.UpdateNotificationProgress(notificationID,
			"downloaded version of package", 1, 1)
	}
	_, err = m.n.Notify(browserapi.LevelSuccess,
		"downloaded version %s of package %s", version, pkgID)
	if err != nil {
		m.log(log.WarnLevel, "notify: %v", err)
	}

	m.iterators.Lock()
	defer m.iterators.Unlock()

	ready, ok := m.iterators.m[pkgID]
	if !ok {
		panic("iterator for package not found")
	}
	delete(m.iterators.m, pkgID)

	ready.Unlock()

	if err := m.interrupter.Interrupt(ctx); err != nil {
		m.log(log.WarnLevel, "interrupt: %v", err)
	}
}

func (m *Manager) abortDownload(
	err error, pkgID string, version release.Version,
	notificationID string,
) {
	m.log(log.WarnLevel, "aborting installation of package %s version %s: %v",
		pkgID, version, err)
	key := m.makeDownloadKey(pkgID, version)
	if err := m.storage.Delete(context.Background(), key); err != nil {
		m.log(log.ErrorLevel, "delete pkg %s version %s "+
			"lock key (%s): %v", pkgID, version, key, err)
	}
	m.notifyError(err, pkgID, version, notificationID)

	m.iterators.Lock()
	defer m.iterators.Unlock()

	ready, ok := m.iterators.m[pkgID]
	if !ok {
		panic("iterator for package not found")
	}
	delete(m.iterators.m, pkgID)

	ready.Unlock()
}

func (m *Manager) notifyError(
	err error, pkgID string, version release.Version,
	notificationID string,
) {
	newMsg := fmt.Sprintf("downloading version %s of "+
		"package %s failed: %v", version, pkgID, err)
	if _, err := m.n.Notify(browserapi.LevelError, newMsg); err != nil {
		m.log(log.WarnLevel, "notify: %v", err)
	}
	if err := m.n.UpdateNotificationProgress(notificationID, newMsg, 1, 1); err != nil {
		m.log(log.WarnLevel, "update notification progress: %v", err)
	}
	if err := m.interrupter.Interrupt(context.Background()); err != nil {
		m.log(log.WarnLevel, "interrupt: %v", err)
	}
}

func (m *Manager) linkLibVersion(pkgID string, version release.Version) error {
	dirname := makePackageVersionDirname(m.dataDir, pkgID, version)
	libdirname := makePackageLibDirname(m.dataDir, pkgID)
	_ = os.RemoveAll(libdirname) // if it fails, error will be handled next
	err := os.Symlink(dirname, libdirname)
	if err != nil {
		err = fmt.Errorf("symlink lib dir: %w", err)
	}
	return err
}

func (m *Manager) linkLibCopyBin(
	pkgID string, version release.Version,
	executables []*tar.Header, pkgVersionDirname string,
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

type settings struct {
	Env map[string]any `json:"env"`
}

func (m *Manager) processSettings(
	pkgID string, pkgVersion release.Version, settingsFile string,
) error {
	_, err := os.Stat(settingsFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat settings.json: %v", err)
	}
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	var set settings
	err = json.Unmarshal(data, &set)
	if err != nil {
		return fmt.Errorf("unmarshal settings: %w", err)
	}

	var ret error
	for k, v := range set.Env {
		// refuse to set anything that's not on this whitelist
		switch k {
		case "GOROOT":
		default:
			continue
		}
		// expand env variables
		str, ok := v.(string)
		if !ok {
			continue
		}
		str = os.Expand(str, func(key string) string {
			switch key {
			case "RUNE_DATADIR":
				return m.dataDir
			case "RUNE_PKG_NAME":
				return pkgID
			case "RUNE_PKG_VERSION":
				return string(pkgVersion)
			}
			return ""
		})
		if err := os.Setenv(k, str); err != nil {
			ret = errors.Join(ret, err)
		}
	}

	return ret
}

func (m *Manager) untar(tarfile *os.File, dirname string) (string, []*tar.Header, error) {
	if err := os.MkdirAll(dirname, 0777); err != nil {
		err = fmt.Errorf("mkdir: %w", err)
		return "", nil, err
	}
	_, err := tarfile.Seek(0, 0)
	if err != nil {
		err = fmt.Errorf("seek tarball file: %w", err)
		return "", nil, err
	}

	gzr, err := gzip.NewReader(tarfile)
	if err != nil {
		err = fmt.Errorf("new gzip reader: %w", err)
		return "", nil, err
	}
	defer gzr.Close()

	executables, err := untar(dirname, gzr)
	if err != nil {
		err = fmt.Errorf("untar into %s: %w", dirname, err)
		return "", nil, err
	}

	settings := filepath.Join(dirname, "settings.json")
	return settings, executables, nil
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

type notificationsProgressWriter struct {
	io.Writer
	pkgID            string
	version          release.Version
	notificationID   string
	completeProgress bool
	m                *Manager
}

func (w *notificationsProgressWriter) Progress(progress, total int64, units string) {
	if total == 0 || progress > total {
		return
	}
	var message string
	if units == "" {
		perc := int(float64(progress) / float64(total) * 100)
		message = fmt.Sprintf("downloading version %s of package %s: %d%%",
			w.version, w.pkgID, perc)
	} else {
		message = fmt.Sprintf("downloading version %s of package %s: %d/%d %s",
			w.version, w.pkgID, progress, total, units)
	}
	err := w.m.n.UpdateNotificationProgress(w.notificationID, message, progress, total)
	if err != nil {
		w.m.log(log.WarnLevel, "could not update notification progress: %v", err)
	}
	w.completeProgress = total == progress
	if err := w.m.interrupter.Interrupt(context.Background()); err != nil {
		w.m.log(log.WarnLevel, "interrupt: %v", err)
	}
}

func isExecutable(info fs.FileInfo) bool {
	// check owner/group/other exec bits, if any
	// match then the file is an executable.
	return info.Mode()&os.ModeType == 0 && info.Mode()&0111 != 0
}

func untar(dst string, r io.Reader) ([]*tar.Header, error) {
	tr := tar.NewReader(r)

	var executables []*tar.Header
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar next: %w", err)
		}

		target := filepath.Join(dst, filepath.Clean(hdr.Name))
		if isExecutable(hdr.FileInfo()) && !isHidden(hdr.Name) {
			executables = append(executables, hdr)
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

func copyExecutables(files []*tar.Header, dirname, targetdirname string) error {
	var ret error
	for _, executable := range files {
		name := filepath.Clean(executable.Name)
		orig := filepath.Join(dirname, name)
		origfile, err := os.OpenFile(orig, os.O_RDONLY, 0)
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("open executable: %w", err))
			continue
		}
		defer origfile.Close()
		target := filepath.Join(targetdirname, filepath.Base(name))
		targetfile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_RDWR,
			executable.FileInfo().Mode())
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("create executable %s: %w", target, err))
			continue
		}
		defer targetfile.Close()
		if _, err := io.Copy(targetfile, origfile); err != nil {
			ret = multierror.Append(ret, fmt.Errorf("copy executable %s: %w", target, err))
			continue
		}
	}
	return ret
}

func isHidden(file string) bool {
	return strings.HasPrefix(filepath.Base(file), ".")
}

func removeExecutables(files []*tar.Header, targetdirname string) error {
	var ret error
	for _, executable := range files {
		name := filepath.Clean(executable.Name)
		target := filepath.Join(targetdirname, filepath.Base(name))
		err := os.Remove(target)
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("create executable %s: %w", target, err))
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

type pkgVersionValue struct {
	Package     string
	Version     release.Version
	Executables []*tar.Header
}

func escapeString(val string) string {
	val = url.PathEscape(val)
	val = strings.ReplaceAll(val, ":", "_")
	return val
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
