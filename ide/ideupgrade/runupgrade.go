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

package ideupgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// upgradeOpts is the per-call data needed by runUpgrade. Decoupling
// from Manager keeps runUpgrade testable in isolation against a
// fakePlatformOps with no IDE/storage dependencies.
type upgradeOpts struct {
	manifest         Manifest
	currentVersion   string
	cacheDir         string
	installRoot      string
	appName          string
	cliSymlinkPath   string
	cliBinaryRelPath string
	backupRetention  int
	ops              platformOps
	notify           func(format string, args ...any)
	// progress, when non-nil, receives byte-transfer samples from the
	// artifact download. It may be nil for tests and non-IDE callers.
	progress func(downloaded, total int64)
}

// runUpgrade is the OS-agnostic orchestration of a manifest-driven
// upgrade. It downloads the artifact, verifies it, replaces the
// installed app bundle (with backup + rollback on failure), refreshes
// the CLI symlink, and trims old backups.
//
// runUpgrade is split out from Manager so it can be exercised end to
// end against a fakePlatformOps without spinning up an IDE.
func runUpgrade(ctx context.Context, opts upgradeOpts) error {
	if opts.ops == nil {
		return errors.New("ideupgrade: platformOps is nil")
	}
	if opts.manifest.URL == "" || opts.manifest.SHA256 == "" {
		return errors.New("ideupgrade: manifest missing url or sha256")
	}

	notify := opts.notify
	if notify == nil {
		notify = func(string, ...any) {}
	}

	if err := os.MkdirAll(opts.cacheDir, 0o755); err != nil {
		return fmt.Errorf("mkdir cache: %w", err)
	}

	if err := preflightFreeSpace(opts); err != nil {
		return err
	}

	// Pick a destination filename based on the manifest filename, falling
	// back to the URL basename. Either way we keep it inside cacheDir.
	artifactName := opts.manifest.Filename
	if artifactName == "" {
		artifactName = path.Base(opts.manifest.URL)
	}
	if artifactName == "" || artifactName == "/" {
		return errors.New("ideupgrade: cannot derive artifact filename")
	}
	artifactPath := filepath.Join(opts.cacheDir, artifactName)

	notify("Downloading %s...", opts.manifest.Version)
	if err := opts.ops.Download(ctx, opts.manifest.URL, artifactPath, opts.progress); err != nil {
		return fmt.Errorf("download artifact: %w", err)
	}
	defer func() { _ = opts.ops.RemoveAll(artifactPath) }()

	if err := opts.ops.VerifySHA256(artifactPath, opts.manifest.SHA256); err != nil {
		return fmt.Errorf("verify sha256: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return runUpgradeDarwin(ctx, opts, artifactPath)
	case "linux":
		return runUpgradeLinux(ctx, opts, artifactPath)
	default:
		return fmt.Errorf("ideupgrade: unsupported os %q", runtime.GOOS)
	}
}

// preflightHeadroomExtra is the constant slack we add to the
// space-required estimate. It absorbs filesystem overhead (HFS+/APFS
// block padding, ditto's temporary files, hdiutil's mount-point
// staging) so the check is conservative without being noisy.
const preflightHeadroomExtra = 200 * 1024 * 1024

// preflightFreeSpace fails fast when there is not enough free space
// on either the cache filesystem (DMG/tarball download lands here)
// or the install-root filesystem (new bundle + backup live here
// simultaneously until pruneBackups runs). Skipped when the manifest
// does not advertise Size — without it we can't bound the requirement.
func preflightFreeSpace(opts upgradeOpts) error {
	if opts.manifest.Size <= 0 {
		return nil
	}
	required := uint64(opts.manifest.Size)*2 + preflightHeadroomExtra
	for _, p := range []string{opts.cacheDir, opts.installRoot} {
		free, err := opts.ops.FreeSpace(p)
		if err != nil {
			return fmt.Errorf("check free space at %s: %w", p, err)
		}
		if free < required {
			return fmt.Errorf(
				"insufficient free space at %s: need %d MiB, have %d MiB",
				p, required/(1024*1024), free/(1024*1024))
		}
	}
	return nil
}

// runUpgradeDarwin is the macOS-specific orchestration: mount DMG,
// assess Gatekeeper, snapshot the existing app, ditto the new bundle
// in, re-verify the installed bundle, refresh symlink, retire old
// backups, detach the DMG.
func runUpgradeDarwin(ctx context.Context, opts upgradeOpts, dmgPath string) error {
	mountpoint, detach, err := opts.ops.MountDMG(ctx, dmgPath)
	if err != nil {
		return fmt.Errorf("mount dmg: %w", err)
	}
	defer func() {
		if detach != nil {
			_ = detach()
		}
	}()

	srcApp, err := findAppBundle(mountpoint)
	if err != nil {
		return fmt.Errorf("find .app in dmg: %w", err)
	}

	if err := opts.ops.AssessGatekeeper(ctx, srcApp); err != nil {
		return fmt.Errorf("gatekeeper assess: %w", err)
	}

	dstApp := filepath.Join(opts.installRoot, opts.appName)
	backup := backupPath(dstApp, opts.currentVersion)

	hasExisting := pathExists(dstApp)
	symlinkOwned := shouldRefreshCLISymlink(opts.cliSymlinkPath, dstApp)
	if hasExisting {
		if err := opts.ops.RenameAtomic(dstApp, backup); err != nil {
			return fmt.Errorf("snapshot existing app: %w", err)
		}
	}

	if err := opts.ops.Ditto(ctx, srcApp, dstApp); err != nil {
		if rbErr := rollbackDarwin(opts, dstApp, backup, hasExisting); rbErr != nil {
			return fmt.Errorf("ditto failed: %w; rollback failed: %v", err, rbErr)
		}
		return fmt.Errorf("ditto: %w", err)
	}

	// Post-install verification: catches mutations to the installed
	// bundle that would prevent macOS from launching it (e.g. an
	// async AV scanner rewriting xattrs after ditto, or a stale
	// .zcompdump-style file inside the bundle). spctl re-checks the
	// Gatekeeper policy at the on-disk path; codesign --verify
	// checks the seal integrity of every signed component.
	if err := opts.ops.AssessGatekeeper(ctx, dstApp); err != nil {
		if rbErr := rollbackDarwin(opts, dstApp, backup, hasExisting); rbErr != nil {
			return fmt.Errorf("post-install gatekeeper assess failed: %w; rollback failed: %v", err, rbErr)
		}
		return fmt.Errorf("post-install gatekeeper assess: %w", err)
	}
	if err := opts.ops.VerifyCodesign(ctx, dstApp); err != nil {
		if rbErr := rollbackDarwin(opts, dstApp, backup, hasExisting); rbErr != nil {
			return fmt.Errorf("post-install codesign verify failed: %w; rollback failed: %v", err, rbErr)
		}
		return fmt.Errorf("post-install codesign verify: %w", err)
	}

	if err := refreshCLISymlink(opts, dstApp, symlinkOwned); err != nil {
		return err
	}

	pruneBackups(opts, dstApp)
	return nil
}

// rollbackDarwin restores the previously snapshotted bundle. It is
// shared by every failure path after the rename-into-backup step so
// the error wrapping at the call site stays focused on the *cause*
// rather than the recovery mechanics.
func rollbackDarwin(opts upgradeOpts, dstApp, backup string, hasExisting bool) error {
	_ = opts.ops.RemoveAll(dstApp)
	if !hasExisting {
		return nil
	}
	return opts.ops.RenameAtomic(backup, dstApp)
}

// runUpgradeLinux is the linux-specific orchestration: extract
// tarball, atomically swap install dirs, refresh symlink, retire old
// backups.
func runUpgradeLinux(ctx context.Context, opts upgradeOpts, archivePath string) error {
	dstApp := filepath.Join(opts.installRoot, opts.appName)
	stagingDir := dstApp + ".new"
	backup := backupPath(dstApp, opts.currentVersion)

	_ = opts.ops.RemoveAll(stagingDir)
	if err := opts.ops.ExtractTarGz(ctx, archivePath, stagingDir); err != nil {
		return fmt.Errorf("extract tarball: %w", err)
	}

	hasExisting := pathExists(dstApp)
	if hasExisting {
		if err := opts.ops.RenameAtomic(dstApp, backup); err != nil {
			_ = opts.ops.RemoveAll(stagingDir)
			return fmt.Errorf("snapshot existing app: %w", err)
		}
	}

	symlinkOwned := shouldRefreshCLISymlink(opts.cliSymlinkPath, dstApp)
	if err := opts.ops.RenameAtomic(stagingDir, dstApp); err != nil {
		// Restore previous app bundle.
		_ = opts.ops.RemoveAll(stagingDir)
		if hasExisting {
			if rbErr := opts.ops.RenameAtomic(backup, dstApp); rbErr != nil {
				return fmt.Errorf("install rename failed: %w; rollback failed: %v", err, rbErr)
			}
		}
		return fmt.Errorf("rename staging into place: %w", err)
	}

	if err := refreshCLISymlink(opts, dstApp, symlinkOwned); err != nil {
		return err
	}

	pruneBackups(opts, dstApp)
	return nil
}

// backupPath returns the path used to snapshot the previously
// installed app bundle before an upgrade. Format:
//
//	<installRoot>/<appName>.bak-<currentVersion>
//
// When currentVersion is empty (development build) we still use a
// stable suffix so the rollback path is deterministic.
func backupPath(installPath, currentVersion string) string {
	suffix := currentVersion
	if suffix == "" {
		suffix = "previous"
	}
	return installPath + ".bak-" + suffix
}

// pruneBackups deletes all but the most recent BackupRetention
// backup snapshots produced by previous upgrades. The most recent
// backup (just created) is always retained.
func pruneBackups(opts upgradeOpts, installPath string) {
	if opts.backupRetention <= 0 {
		opts.backupRetention = 1
	}
	dir := filepath.Dir(installPath)
	prefix := filepath.Base(installPath) + ".bak-"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type backup struct {
		path    string
		modTime int64
	}
	var backups []backup
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backup{
			path:    filepath.Join(dir, name),
			modTime: info.ModTime().UnixNano(),
		})
	}
	if len(backups) <= opts.backupRetention {
		return
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime > backups[j].modTime
	})
	for _, b := range backups[opts.backupRetention:] {
		_ = opts.ops.RemoveAll(b.path)
	}
}

// findAppBundle returns the absolute path of the first .app directory
// found at the top level of root.
func findAppBundle(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", root, err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(root, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .app bundle in %s", root)
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// shouldRefreshCLISymlink decides whether refreshCLISymlink should
// rewrite opts.cliSymlinkPath. The rule is:
//
//   - if the symlink does not exist → refresh (create).
//   - if the symlink resolves into the install path (oldApp) →
//     refresh (the user installed it via the standard layout, we
//     own it).
//   - otherwise → leave it alone (user has their own arrangement).
//
// We resolve the symlink before the in-place replace so we can
// observe the *old* target; after the swap, the symlink would
// resolve into the new bundle and the heuristic would no longer
// distinguish "ours" from "user's".
func shouldRefreshCLISymlink(symlinkPath, oldApp string) bool {
	if symlinkPath == "" {
		return false
	}
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		// Either it doesn't exist (create) or it isn't a symlink
		// (treat as user-owned and leave it).
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		return false
	}
	resolved, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		// Dangling symlink; previous target likely lived inside the
		// old install but is now gone. Use the raw link target.
		resolved = target
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(symlinkPath), resolved)
		}
	}
	resolved = filepath.Clean(resolved)
	oldApp = filepath.Clean(oldApp)
	rel, err := filepath.Rel(oldApp, resolved)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != "."
}

// refreshCLISymlink updates opts.cliSymlinkPath to point at the new
// binary inside dstApp when owned is true. owned is precomputed by
// shouldRefreshCLISymlink *before* the in-place replace so the
// "points into old install" check still works.
//
// A symlink-only failure is non-fatal: the bundle is already in
// place and we don't want to roll the whole upgrade back over a
// symlink that we couldn't write. We surface the error through the
// upgradeOpts.notify channel and return nil.
func refreshCLISymlink(opts upgradeOpts, dstApp string, owned bool) error {
	if !owned {
		if opts.notify != nil && opts.cliSymlinkPath != "" {
			opts.notify(
				"Left existing CLI symlink at %s untouched", opts.cliSymlinkPath)
		}
		return nil
	}
	target := filepath.Join(dstApp, opts.cliBinaryRelPath)
	if err := opts.ops.Symlink(target, opts.cliSymlinkPath); err != nil {
		if opts.notify != nil {
			opts.notify(
				"Failed to refresh CLI symlink at %s: %v",
				opts.cliSymlinkPath, err)
		}
		return nil
	}
	return nil
}
