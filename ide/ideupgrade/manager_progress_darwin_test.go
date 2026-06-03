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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
)

// TestManagerRunUpgrade_PostsDownloadProgress drives Manager.runUpgrade
// against a fakePlatformOps (which invokes the download progress
// callback) and asserts the anchored notification receives live
// progress samples that culminate at total.
func TestManagerRunUpgrade_PostsDownloadProgress(t *testing.T) {
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	fake := &fakePlatformOps{}
	const newBinary = "#!/bin/sh\necho new\n"
	fake.dmgPayload = map[string]string{"Contents/MacOS/rune": newBinary}
	fake.archiveContent = []byte("dmg-bytes")
	sum := sha256.Sum256(fake.archiveContent)

	writeExistingApp(t, root, "#!/bin/sh\necho old\n")
	existingBinary := filepath.Join(root, "Rune.app", "Contents", "MacOS", "rune")
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "Rune.app", "Contents", "Info.plist"),
		[]byte("<plist/>"), 0o644))

	notifs := &recordingNotifications{}
	mgr, err := newWithPlatformOps(Config{
		CurrentVersion:   "v9.9.0",
		Arch:             "darwin-arm64",
		ManifestURL:      "https://example.invalid",
		Storage:          storagestub.NewInMemoryService(),
		Notifications:    notifs,
		Executable:       func() (string, error) { return existingBinary, nil },
		InstallRoot:      root,
		AppName:          "Rune.app",
		CLISymlinkPath:   filepath.Join(t.TempDir(), "rune"),
		CLIBinaryRelPath: filepath.Join("Contents", "MacOS", "rune"),
		CacheDir:         t.TempDir(),
		BackupRetention:  1,
		ScheduleNextTick: func(fn func()) bool { fn(); return true },
		Now:              func() time.Time { return time.Unix(1_700_000_000, 0) },
	}, fake)
	require.NoError(t, err)

	manifest := Manifest{
		Version:  "v9.9.9",
		Filename: "Rune-v9.9.9.dmg",
		URL:      "https://example.invalid/Rune-v9.9.9.dmg",
		SHA256:   hex.EncodeToString(sum[:]),
		Size:     int64(len(fake.archiveContent)),
	}

	require.NoError(t, mgr.runUpgrade(context.Background(), manifest))

	notifyN, samples := notifs.snapshot()
	require.Equal(t, 2, notifyN,
		"expected one anchor Notify plus the terminal success Notify")
	require.NotEmpty(t, samples, "expected progress samples")

	var sawLive, sawComplete bool
	for _, s := range samples {
		if s.total > 0 && s.progress < s.total {
			sawLive = true
		}
		if s.total > 0 && s.progress == s.total {
			sawComplete = true
		}
	}
	require.True(t, sawLive, "expected at least one in-flight progress sample")
	require.True(t, sawComplete, "expected a completion sample reaching total")
}
