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

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/release"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/component/notifications"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/ide/idepkg"
)

const (
	cmdPkgInstall    = "pkginstall"
	cmdPkgRemove     = "pkgremove"
	cmdPkgUse        = "pkguse"
	cmdPkgCurrent    = "pkgcurrent"
	cmdPkgUpgradeAll = "pkgupgradeall"
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
	pkg *idepkg.Manager
	n   browserapi.Notifications
}

func newPackageManager(
	n browserapi.Notifications, m release.Manager,
	storage document.Service, dataDir string,
) *pkgManager {
	ret := new(pkgManager)
	ret.pkg = idepkg.NewManager(n, m, storage, dataDir)
	ret.n = n
	return ret
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
	ctx context.Context, cmd string, args []string,
) (iterator.Iterator[string], string, error) {
	switch cmd {
	case cmdPkgInstall:
		return m.completePkgInstall(ctx, cmd, args)
	case cmdPkgRemove:
		return m.completePkgInstalled(ctx, cmd, args, true)
	case cmdPkgUse:
		return m.completePkgInstalled(ctx, cmd, args, true)
	case cmdPkgCurrent:
		return m.completePkgInstalled(ctx, cmd, args, false)
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
			_, err = m.n.Notify(notifications.LevelSuccess,
				"package %s has been removed", pkgID)
		} else if errors.Is(err, idepkg.ErrNotInstalled) {
			return fmt.Errorf("package %s is not installed", pkgID)
		}
		return err
	}
	err := m.pkg.DeletePackageVersion(ctx, pkgID, version, false)
	if err == nil {
		_, err = m.n.Notify(notifications.LevelSuccess,
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
	_, err = m.n.Notify(notifications.LevelSuccess,
		"version %s of package %s is now in use", version, pkgID)
	return err
}

func (m *pkgManager) handlePkgUpgradeAll(ctx context.Context, cmd textapi.Command) error {
	pkgs, err := m.pkg.ListInstalledPackages(ctx)
	if err != nil {
		return err
	}

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
				_, errors[i] = m.n.Notify(notifications.LevelInfo,
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

	_, err = m.n.Notify(notifications.LevelInfo,
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
		return "", errors.New("latest version is not known")

	}
	return p.Latest, nil
}
