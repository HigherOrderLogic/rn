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

//go:build darwin

package ideupgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
)

// TestDarwinE2E_HappyPath builds a real DMG with hdiutil from a
// synthetic Rune.app and runs Manager.CheckNow against a temp install
// root using a real darwinPlatformOps (with Gatekeeper assess
// disabled — that step requires a notarized bundle).
//
// Set RUNE_E2E_NOTARIZED=1 on a CI runner that has a notarized DMG to
// also exercise the spctl assert.
func TestDarwinE2E_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping darwin e2e in short mode")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not available")
	}

	// 1. Build a synthetic Rune.app directory tree.
	src := t.TempDir()
	app := filepath.Join(src, "Rune.app")
	binDir := filepath.Join(app, "Contents", "MacOS")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	binary := filepath.Join(binDir, "rune")
	const marker = "rune-e2e-marker"
	script := "#!/bin/sh\necho " + marker + "\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o755))

	// 2. Build a DMG from the app folder.
	dmgDir := t.TempDir()
	dmgPath := filepath.Join(dmgDir, "Rune.dmg")
	cmd := exec.Command("hdiutil", "create",
		"-quiet", "-fs", "HFS+", "-format", "UDRO",
		"-srcfolder", src, "-volname", "Rune", dmgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("hdiutil create unavailable in this environment: %v: %s",
			err, strings.TrimSpace(string(out)))
	}
	dmg, err := os.ReadFile(dmgPath)
	require.NoError(t, err)
	sum := sha256.Sum256(dmg)

	// 3. Stand up an HTTP server that serves the DMG and a manifest.
	manifestArch := runtime.GOOS + "-" + runtime.GOARCH
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/Rune.dmg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(dmg)
	})
	mux.HandleFunc("/"+manifestArch+"/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Manifest{
			Version:  "v9.9.9",
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Filename: "Rune.dmg",
			URL:      srv.URL + "/Rune.dmg",
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(dmg)),
		})
	})

	// 4. Construct a Manager with PlatformOps that wraps the real
	//    darwinPlatformOps but skips Gatekeeper unless explicitly
	//    enabled.
	installRoot := t.TempDir()
	cliRoot := t.TempDir()
	cacheRoot := t.TempDir()

	// On macOS the t.TempDir() lives under /var/folders/, which
	// EvalSymlinks resolves to /private/var/folders/. The upgrade
	// will land at the resolved location, so resolve the install
	// root up-front to keep assertions stable.
	if resolved, err := filepath.EvalSymlinks(installRoot); err == nil {
		installRoot = resolved
	}

	// Seed a synthetic install at <installRoot>/Rune.app that looks
	// real enough for detectRunningInstall to accept the launching
	// binary as a valid in-place upgrade target.
	existingBinDir := filepath.Join(installRoot, "Rune.app", "Contents", "MacOS")
	require.NoError(t, os.MkdirAll(existingBinDir, 0o755))
	existingBinary := filepath.Join(existingBinDir, "rune")
	require.NoError(t, os.WriteFile(existingBinary, []byte(script), 0o755))
	infoPlist := filepath.Join(installRoot, "Rune.app", "Contents", "Info.plist")
	require.NoError(t, os.WriteFile(infoPlist, []byte("<plist/>"), 0o644))

	var ops platformOps = darwinPlatformOps{httpClient: srv.Client()}
	if os.Getenv("RUNE_E2E_NOTARIZED") != "1" {
		ops = noGatekeeperOps{platformOps: ops}
	}

	mgr, err := newWithPlatformOps(Config{
		CurrentVersion:   "v9.9.0",
		Arch:             manifestArch,
		ManifestURL:      srv.URL,
		Storage:          storagestub.NewInMemoryService(),
		HTTPClient:       srv.Client(),
		Executable:       func() (string, error) { return existingBinary, nil },
		InstallRoot:      installRoot,
		AppName:          "Rune.app",
		CLISymlinkPath:   filepath.Join(cliRoot, "rune"),
		CLIBinaryRelPath: filepath.Join("Contents", "MacOS", "rune"),
		CacheDir:         cacheRoot,
		BackupRetention:  1,
		Notifications:    &recordingNotifications{},
		ScheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
	}, ops)
	require.NoError(t, err)

	// Drive the upgrade synchronously through the same path Manager
	// would use after the prompt selects "Upgrade Now".
	manifest, _, err := mgr.fetchManifest(context.Background())
	require.NoError(t, err)
	require.Equal(t, "v9.9.9", manifest.Version)

	require.NoError(t, mgr.runUpgrade(context.Background(), manifest))

	// Verify the new binary is installed.
	got, err := os.ReadFile(filepath.Join(installRoot, "Rune.app", "Contents", "MacOS", "rune"))
	require.NoError(t, err)
	require.Contains(t, string(got), marker)

	// CLI symlink resolves to the new binary.
	target, err := os.Readlink(filepath.Join(cliRoot, "rune"))
	require.NoError(t, err)
	require.Equal(t,
		filepath.Join(installRoot, "Rune.app", "Contents", "MacOS", "rune"),
		target)
}

// noGatekeeperOps wraps a platformOps and forces AssessGatekeeper to
// succeed regardless of the underlying implementation, and likewise
// short-circuits VerifyCodesign. This lets the e2e test run on dev
// machines where the synthetic DMG is unsigned.
type noGatekeeperOps struct {
	platformOps
}

func (n noGatekeeperOps) AssessGatekeeper(_ context.Context, _ string) error {
	return nil
}

func (n noGatekeeperOps) VerifyCodesign(_ context.Context, _ string) error {
	return nil
}
