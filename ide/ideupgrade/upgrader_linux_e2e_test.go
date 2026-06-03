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

//go:build linux

package ideupgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
)

// makeTarGz returns the bytes of a gzipped tar containing the given
// path -> content map (mode 0o755 for everything).
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for path, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     path,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestLinuxE2E_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping linux e2e in short mode")
	}
	const marker = "rune-linux-e2e"
	tarData := makeTarGz(t, map[string]string{
		"bin/rune":          "#!/bin/sh\necho " + marker + "\n",
		"share/rune/README": "hello",
	})
	sum := sha256.Sum256(tarData)

	manifestArch := runtime.GOOS + "-" + runtime.GOARCH
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/rune.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(tarData)
	})
	mux.HandleFunc("/"+manifestArch+"/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Manifest{
			Version:  "v0.42.1",
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Filename: "rune.tar.gz",
			URL:      srv.URL + "/rune.tar.gz",
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(tarData)),
		})
	})

	installRoot := t.TempDir()
	cliRoot := t.TempDir()
	cacheRoot := t.TempDir()

	// Seed the existing install so detectRunningInstall accepts the
	// launching binary path.
	existingBinDir := filepath.Join(installRoot, "rune.app", "bin")
	require.NoError(t, os.MkdirAll(existingBinDir, 0o755))
	existingBinary := filepath.Join(existingBinDir, "rune")
	require.NoError(t, os.WriteFile(existingBinary,
		[]byte("#!/bin/sh\necho old\n"), 0o755))

	mgr, err := newWithPlatformOps(Config{
		CurrentVersion:   "v0.42.0",
		Arch:             manifestArch,
		ManifestURL:      srv.URL,
		Storage:          storagestub.NewInMemoryService(),
		HTTPClient:       srv.Client(),
		Executable:       func() (string, error) { return existingBinary, nil },
		InstallRoot:      installRoot,
		AppName:          "rune.app",
		CLISymlinkPath:   filepath.Join(cliRoot, "rune"),
		CLIBinaryRelPath: filepath.Join("bin", "rune"),
		CacheDir:         cacheRoot,
		BackupRetention:  1,
		Notifications:    &recordingNotifications{},
		ScheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
	}, linuxPlatformOps{httpClient: srv.Client()})
	require.NoError(t, err)

	manifest, _, err := mgr.fetchManifest(context.Background())
	require.NoError(t, err)
	require.Equal(t, "v0.42.1", manifest.Version)

	require.NoError(t, mgr.runUpgrade(context.Background(), manifest))

	got, err := os.ReadFile(filepath.Join(installRoot, "rune.app", "bin", "rune"))
	require.NoError(t, err)
	require.Contains(t, string(got), marker)

	target, err := os.Readlink(filepath.Join(cliRoot, "rune"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(installRoot, "rune.app", "bin", "rune"), target)
}

func TestLinuxE2E_ChecksumMismatchRollsBack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping linux e2e in short mode")
	}
	tarData := makeTarGz(t, map[string]string{
		"bin/rune": "#!/bin/sh\necho new\n",
	})

	manifestArch := runtime.GOOS + "-" + runtime.GOARCH
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/rune.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarData)
	})
	mux.HandleFunc("/"+manifestArch+"/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Manifest{
			Version:  "v2",
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Filename: "rune.tar.gz",
			URL:      srv.URL + "/rune.tar.gz",
			SHA256:   strings.Repeat("0", 64),
			Size:     int64(len(tarData)),
		})
	})

	installRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(installRoot, "rune.app", "bin"), 0o755))
	existing := filepath.Join(installRoot, "rune.app", "bin", "rune")
	require.NoError(t, os.WriteFile(existing, []byte("#!/bin/sh\necho old\n"), 0o755))

	mgr, err := newWithPlatformOps(Config{
		CurrentVersion:   "v1",
		Arch:             manifestArch,
		ManifestURL:      srv.URL,
		Storage:          storagestub.NewInMemoryService(),
		HTTPClient:       srv.Client(),
		Executable:       func() (string, error) { return existing, nil },
		InstallRoot:      installRoot,
		AppName:          "rune.app",
		CLISymlinkPath:   filepath.Join(t.TempDir(), "rune"),
		CLIBinaryRelPath: filepath.Join("bin", "rune"),
		CacheDir:         t.TempDir(),
		BackupRetention:  1,
		Notifications:    &recordingNotifications{},
		ScheduleNextTick: func(fn func()) bool {
			fn()
			return true
		},
	}, linuxPlatformOps{httpClient: srv.Client()})
	require.NoError(t, err)

	manifest, _, err := mgr.fetchManifest(context.Background())
	require.NoError(t, err)
	require.Error(t, mgr.runUpgrade(context.Background(), manifest))

	// Existing install untouched.
	got, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Contains(t, string(got), "old")
}
