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

package ide

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	log "github.com/sirupsen/logrus"
	sdkiterator "github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide/idepkg"
)

const (
	cmdPkgInstall    = "pkginstall"
	cmdPkgRemove     = "pkgremove"
	cmdPkgUse        = "pkguse"
	cmdPkgCurrent    = "pkgcurrent"
	cmdPkgUpgradeAll = "pkgupgradeall"

	installStorageKey = "autoInstallPrompt"
)

var (
	pkgCommands = []textapi.CommandManual{
		{
			Name: cmdPkgInstall,
			Summary: "Installs a package from the official distribution. " +
				"If version is omitted, the package is upgraded to the latest version. " +
				"If package contains executables, then this " +
				"will be made available to terminal sessions via PATH env variable." +
				cmdPkgUse + " is not necessary after running this command.",
			Synopsis: "<package> [version]",
		},
		{
			Name:     cmdPkgUpgradeAll,
			Summary:  "Upgrades all installed packages to the latest version.",
			Synopsis: "",
		},
		{
			Name: cmdPkgRemove,
			Summary: "Removes a package from local storage. " +
				"If version is specified, then only the specified version is removed, " +
				"otherwise all versions are removed. " +
				"If version is passed and it is in use, this command errors out.",
			Synopsis: "<package> [version]",
		},
		{
			Name: cmdPkgUse,
			Summary: fmt.Sprintf("Use the given package version, if available. "+
				"If not available, download first via %s.", cmdPkgInstall),
			Synopsis: "<package> <version>",
		},
		{
			Name:     cmdPkgCurrent,
			Summary:  "Print the given package version in use.",
			Synopsis: "<package>",
		},
	}
)

type pkgManager struct {
	pkg              *idepkg.Manager
	n                browserapi.Notifications
	wh               *workspaceManagerHandler
	storage          document.Service
	scheduleNextTick func(func()) bool
	interrupter      term.Interrupter
	pending          sync.Map // map[string]*sync.Mutex
}

type installStorageValue struct {
	Value bool // true => always, false => never
}

func (m *pkgManager) init(
	n browserapi.Notifications, rm release.Manager,
	storage document.Service, scheme schemeapi.Scheme, dataDir string,
	interrupter term.Interrupter, wh *workspaceManagerHandler,
	scheduleNextTick func(func()) bool,
) {
	m.pkg = idepkg.NewManager(n, rm, storage, scheme, dataDir, interrupter)
	m.scheduleNextTick = scheduleNextTick
	m.n = n
	m.interrupter = interrupter
	m.wh = wh
	m.storage = storage
	err := m.pkg.ProcessInstalledSettings(context.Background())
	if err != nil {
		log.Errorf("process installed settings: %v", err)
	} else {
		log.Debugf("processed all installed settings")
	}
}

// LibDir installs package via prompt if not installed yet
func (m *pkgManager) LibDir(ctx context.Context, pkgID string) (
	sdkiterator.Iterator[string], error,
) {
	it, err := m.pkg.LibDir(ctx, pkgID)
	if err == nil || !errors.Is(err, idepkg.ErrNotInstalled) {
		return it, err
	}

	version, err := m.getLatestVersion(ctx, pkgID)
	if err != nil {
		if errors.Is(err, document.ErrNotFound) {
			return nil, document.ErrNotFound
		}
		return nil, fmt.Errorf("get latest version: %w", err)
	}

	ready, ok := m.pending.Load(pkgID)
	if ok {
		return newPendingIterator(m.pkg, pkgID, ready.(*sync.Mutex)), nil
	}

	var val installStorageValue
	if err := m.storage.Get(ctx, installStorageKey, &val); err != nil {
		return m.openInstallPrompt(pkgID, version)
	}
	if !val.Value {
		// signals that package does not exist, which
		// should prevent further attempts or errors being logged.
		return nil, document.ErrNotFound
	}
	if err := m.pkg.InstallPackageVersion(ctx, pkgID, version); err != nil {
		return nil, fmt.Errorf("install latest version: %w", err)
	}
	return m.pkg.LibDir(ctx, pkgID)
}

func (m *pkgManager) HandleCommand(ctx context.Context, cmd textapi.Command) error {
	switch cmd.Name {
	case cmdPkgInstall:
		return m.handlePkgInstall(ctx, cmd)
	case cmdPkgRemove:
		return m.handlePkgRemove(ctx, cmd)
	case cmdPkgUse:
		return m.handlePkgUse(ctx, cmd)
	case cmdPkgUpgradeAll:
		return m.handlePkgUpgradeAll(ctx, cmd)
	case cmdPkgCurrent:
		return m.handlePkgCurrent(ctx, cmd)
	default:
		return fmt.Errorf("unknown command: %v", cmd.Name)
	}
}

func (m *pkgManager) Complete(
	ctx context.Context, cmd textapi.Command,
) (iterator.Iterator[string], string, error) {
	switch cmd.Name {
	case cmdPkgInstall:
		return m.completePkgInstall(ctx, cmd.Name, cmd.Args)
	case cmdPkgRemove:
		return m.completePkgInstalled(ctx, cmd.Name, cmd.Args, true)
	case cmdPkgUse:
		return m.completePkgInstalled(ctx, cmd.Name, cmd.Args, true)
	case cmdPkgCurrent:
		return m.completePkgInstalled(ctx, cmd.Name, cmd.Args, false)
	case cmdPkgUpgradeAll:
		return iterator.FromSlice[string](nil), "", nil
	default:
		return iterator.FromSlice[string](nil), "", nil
	}
}

func (m *pkgManager) subscribeCommands(e *ex) (ret error) {
	for _, man := range pkgCommands {
		err := e.comp.SubscribeCommand(man, m)
		if err != nil {
			ret = multierror.Append(ret, fmt.Errorf("subscribe command: %w", err))
		}
	}
	return
}

func (m *pkgManager) handlePkgInstall(ctx context.Context, cmd textapi.Command) error {
	if len(cmd.Args) == 0 {
		return errors.New("package name is missing")
	}
	pkgID := cmd.Args[0]
	version := release.Version(release.Latest)
	if len(cmd.Args) == 2 {
		version = release.Version(cmd.Args[1])
	}
	if version == release.Latest {
		var err error
		version, err = m.getLatestVersion(ctx, pkgID)
		if err != nil {
			return fmt.Errorf("install package: %w", err)
		}
	}
	return m.pkg.InstallPackageVersion(ctx, pkgID, version)
}

func (m *pkgManager) makeProgressAnimation() component.Responsive {
	frames, seq := component.ProgressAnimationFrames()
	animation := component.FuncResponsive(
		component.NewAnimation(m.interrupter, frames, seq, 10),
		func(width int) int { return 15 },
	)
	return animation
}

func (m *pkgManager) previewPkgInstall(cmd string, args ...string) (
	comp component.Responsive, cancel func(), ok bool,
) {
	if len(args) == 0 || cmd != cmdPkgInstall || args[0] == "" {
		return
	}
	pkgID := args[0]
	if len(args) == 1 {
		ok = true
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		comp = component.Async(
			m.interrupter, m.makeProgressAnimation(),
			func() (component.Responsive, error) {
				pkg, err := m.pkg.DescribePackage(ctx, pkgID)
				if err != nil {
					return nil, err
				}
				attrs := term.Attributes{}
				comp = makePackagePreviewComponent(pkg, attrs)
				return comp, nil
			})
	} else if len(args) > 1 && args[1] != "" {
		ok = true
		version := args[1]
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		comp = component.Async(
			m.interrupter, m.makeProgressAnimation(),
			func() (component.Responsive, error) {
				release, err := m.pkg.DescribeRelease(ctx, pkgID, version)
				if err != nil {
					return nil, err
				}
				attrs := term.Attributes{}
				comp = makeReleasePreviewComponent(release, attrs)
				return comp, nil
			})
	}
	return
}

func (m *pkgManager) handlePkgRemove(ctx context.Context, cmd textapi.Command) error {
	if len(cmd.Args) == 0 {
		return errors.New("package name is missing")
	}
	pkgID := cmd.Args[0]
	var version release.Version
	if len(cmd.Args) == 2 {
		version = release.Version(cmd.Args[1])
	}
	if version == "" {
		err := m.pkg.DeletePackage(ctx, pkgID)
		if err == nil {
			_, err = m.n.Notify(browserapi.LevelSuccess,
				"package %s has been removed", pkgID)
		} else if errors.Is(err, idepkg.ErrNotInstalled) {
			return fmt.Errorf("package %s is not installed", pkgID)
		}
		return err
	}
	err := m.pkg.DeletePackageVersion(ctx, pkgID, version, false)
	if err == nil {
		_, err = m.n.Notify(browserapi.LevelSuccess,
			"version %s of package %s has been removed", version, pkgID)
		return err
	} else if errors.Is(err, idepkg.ErrNotInstalled) {
		return fmt.Errorf("version %s of package %s is not installed", version, pkgID)
	}
	if errors.Is(err, idepkg.ErrVersionInUse) {
		return fmt.Errorf("version %s of package %s is "+
			"currently in use, run '%s' with some other version first before removing, "+
			"or pass no version argument to remove all package versions",
			version, pkgID, cmdPkgUse)
	}
	return err
}

func (m *pkgManager) handlePkgUse(ctx context.Context, cmd textapi.Command) error {
	if len(cmd.Args) < 2 {
		return errors.New("package name or version are missing")
	}
	pkgID := cmd.Args[0]
	version := release.Version(cmd.Args[1])
	err := m.pkg.UsePackageVersion(ctx, pkgID, version)
	if err != nil {
		if errors.Is(err, idepkg.ErrVersionInUse) {
			return fmt.Errorf("version %s is already in use", version)
		}
		return err
	}
	_, err = m.n.Notify(browserapi.LevelSuccess,
		"version %s of package %s is now in use", version, pkgID)
	return err
}

func (m *pkgManager) handlePkgUpgradeAll(ctx context.Context, cmd textapi.Command) error {
	pkgs, err := m.pkg.ListInstalledPackages(ctx)
	if err != nil {
		return err
	}
	defer pkgs.Close()

	var packages []string
	for {
		pkg, ok := pkgs.Next(ctx)
		if !ok {
			break
		}
		packages = append(packages, pkg)
	}
	if err := pkgs.Err(); err != nil {
		return err
	}

	if len(packages) == 0 {
		return errors.New("no packages are installed")
	}

	var wg sync.WaitGroup
	errors := make([]error, len(packages))
	wg.Add(len(packages))
	for i := 0; i < len(packages); i++ {
		i := i
		go debug.CapturePanicReport(func() {
			defer wg.Done()
			pkgID := packages[i]
			inUse, err := m.pkg.PackageVersionInUse(ctx, pkgID)
			if err != nil {
				errors[i] = fmt.Errorf("package version in use: %w", err)
				return
			}
			latest, err := m.getLatestVersion(ctx, pkgID)
			if err != nil {
				errors[i] = fmt.Errorf("cannot upgrade package %s: %w", pkgID, err)
				return
			}
			if latest == inUse {
				_, errors[i] = m.n.Notify(browserapi.LevelInfo,
					"package %s already upgraded to the latest version (%s)",
					pkgID, latest)
				return
			}

			if err := m.pkg.InstallPackageVersion(ctx, pkgID, latest); err != nil {
				errors[i] = fmt.Errorf("upgrade package version: %w", err)
			}
		})
	}

	wg.Wait()
	var ret error
	for _, err := range errors {
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return ret
}

func (m *pkgManager) handlePkgCurrent(ctx context.Context, cmd textapi.Command) error {
	if len(cmd.Args) < 1 {
		return errors.New("package name is missing")
	}
	pkgID := cmd.Args[0]
	version, err := m.pkg.PackageVersionInUse(ctx, pkgID)
	if err != nil {
		return err
	}

	_, err = m.n.Notify(browserapi.LevelInfo,
		"version %s of package %s is in use", version, pkgID)
	return err
}

func (m *pkgManager) completePkgInstall(
	ctx context.Context, cmd string, args []string,
) (iterator.Iterator[string], string, error) {
	if len(args) <= 1 {
		it, err := m.pkg.ListPackages(ctx, nil)
		if err != nil {
			return nil, "", fmt.Errorf("list packages: %w", err)
		}
		return iterator.Map(it, func(in release.Package) (out string) {
			return in.Name
		}), "", nil
	}
	if len(args) == 2 {
		it, err := m.pkg.ListPackageVersions(ctx, args[0], nil)
		if err != nil {
			return nil, "", fmt.Errorf("list packages: %w", err)
		}
		return iterator.Map(it, func(in release.Bundle) (out string) {
			return string(in.Version)
		}), "", nil
	}
	return iterator.FromSlice[string](nil), "", nil
}

func (m *pkgManager) completePkgInstalled(
	ctx context.Context, cmd string, args []string, showVersions bool,
) (iterator.Iterator[string], string, error) {
	if len(args) <= 1 {
		it, err := m.pkg.ListInstalledPackages(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("list packages: %w", err)
		}
		return it, "", nil
	}
	if len(args) == 2 && showVersions {
		it, err := m.pkg.ListInstalledPackageVersions(ctx, args[0])
		if err != nil {
			return nil, "", fmt.Errorf("list packages: %w", err)
		}
		return iterator.Map(it, func(in release.Version) (out string) {
			return string(in)
		}), "", nil
	}
	return iterator.FromSlice[string](nil), "", nil
}

func (m *pkgManager) getLatestVersion(
	ctx context.Context, pack string,
) (release.Version, error) {
	p, err := m.pkg.DescribePackage(ctx, pack)
	if err != nil {
		return "", err
	}
	if p.Latest == "" {
		iter, err := m.pkg.ListPackageVersions(ctx, pack, nil)
		if err != nil {
			return "", fmt.Errorf("list %q versions: %w", pack, err)
		}
		// this could be pretty slow, so hopefully either there's not many package
		// versions or Latest is always populated
		defer iter.Close()
		var latest time.Time
		var version release.Version
		for {
			bundle, ok := iter.Next(ctx)
			if !ok {
				break
			}
			if bundle.CreatedAt.After(latest) {
				latest = bundle.CreatedAt
				version = bundle.Version
			}
		}
		if err := iter.Err(); err != nil {
			return "", fmt.Errorf("iterate over versions of %q: %w", pack, err)
		}
		if version == "" {
			return "", fmt.Errorf("package %q has no releases", pack)
		}
		return version, nil
	}
	return p.Latest, nil
}

func (m *pkgManager) setAutoInstall() error {
	return m.storage.Set(context.Background(),
		installStorageKey, installStorageValue{Value: true})
}

func (m *pkgManager) openInstallPrompt(pkgID string, version release.Version) (
	iterator.Iterator[string], error,
) {
	const (
		yes       = "Yes"
		yesAlways = "Yes, Always"
		no        = "No"
		noNever   = "No, Never"
	)

	msg := fmt.Sprintf("Do you want to install package %q?", pkgID)

	ready := new(sync.Mutex)
	it := newPendingIterator(m.pkg, pkgID, ready)

	ctx := context.Background()
	ready.Lock()
	m.scheduleNextTick(func() {
		m.wh.focusEx().comp.Prompt(msg, []string{yes, yesAlways, no, noNever},
			[]term.KeyComb{{Ch: 'Y'}, {Ch: 'A'}, {Ch: 'N'}, {Ch: 'V'}},
			handler.FuncPromptHandler(
				func(i int, opt string) {
					defer ready.Unlock()
					var err error
					switch opt {
					case yesAlways:
						_ = m.storage.Set(ctx, installStorageKey, installStorageValue{Value: true})
						fallthrough
					case yes:
						err = m.pkg.InstallPackageVersion(ctx, pkgID, version)
						if err == nil {
							it.it, err = m.pkg.LibDir(ctx, pkgID)
						}
					case noNever:
						_ = m.storage.Set(ctx, installStorageKey, installStorageValue{Value: false})
						fallthrough
					case no:
						err = document.ErrNotFound
					}
					if err != nil {
						it.err = err
					}
				},
				func() error {
					if it.it == nil && it.err == nil {
						it.err = document.ErrNotFound
						ready.Unlock()
					}
					m.pending.Delete(pkgID)
					return nil
				}))
	})

	m.pending.Store(pkgID, ready)
	return it, nil
}

type pkgManagerIterator struct {
	pkgID string
	ready *sync.Mutex
	pkg   *idepkg.Manager

	err error
	it  iterator.Iterator[string]
}

func newPendingIterator(
	pkg *idepkg.Manager, pkgID string, ready *sync.Mutex,
) *pkgManagerIterator {
	return &pkgManagerIterator{
		pkgID: pkgID,
		ready: ready,
		pkg:   pkg,
	}
}

func (l *pkgManagerIterator) Next(ctx context.Context) (string, bool) {
	l.ready.Lock()
	defer l.ready.Unlock()
	if l.err != nil {
		return "", false
	}
	if l.it == nil {
		l.it, l.err = l.pkg.LibDir(context.Background(), l.pkgID)
	}
	if l.err != nil {
		return "", false
	}
	return l.it.Next(ctx)
}

func (l *pkgManagerIterator) Err() error {
	l.ready.Lock()
	defer l.ready.Unlock()
	if l.err != nil {
		return l.err
	}
	if l.it == nil {
		l.it, l.err = l.pkg.LibDir(context.Background(), l.pkgID)
	}
	if l.err != nil {
		return l.err
	}
	return l.it.Err()
}

func (l *pkgManagerIterator) Close() error {
	l.ready.Lock()
	defer l.ready.Unlock()
	if l.it == nil {
		return nil
	}
	return l.it.Close()
}
