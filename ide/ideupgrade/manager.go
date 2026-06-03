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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"golang.org/x/mod/semver"
	"unstable.build/go-tui/debug"
)

// Default tunables for Manager. Exported so callers can derive
// alternate Configs without importing time directly when constructing
// configs from external sources.
const (
	DefaultInitialDelay = 30 * time.Second
	DefaultCheckPeriod  = 24 * time.Hour
	DefaultBackupKeep   = 1

	// DefaultHTTPDialTimeout bounds the TCP/TLS dial of the default
	// HTTPClient. A wedged DNS/SYN can otherwise hang an upgrade
	// check (and any caller that runs it synchronously) for the
	// system-default ~75s.
	DefaultHTTPDialTimeout = 5 * time.Second

	// DefaultHTTPResponseHeaderTimeout bounds the wait between
	// sending the request and receiving the response headers. We do
	// NOT bound full-body reads because artifact downloads on slow
	// links can legitimately exceed any flat timeout.
	DefaultHTTPResponseHeaderTimeout = 15 * time.Second

	storagePartition = "ideupgrade"
	storageKeyState  = "state"
)

// upgradeProgressInterval throttles intermediate download progress
// samples so a fast download does not peg the IDE event loop. The
// boundary sample (downloaded == total) always emits regardless.
const upgradeProgressInterval = 50 * time.Millisecond

// Config is the externally-provided configuration for a Manager.
// All fields except CurrentVersion, ManifestURL and Storage have safe
// defaults applied by New.
type Config struct {
	// CurrentVersion is the version of the running client. When empty,
	// every check is skipped (safety net for development builds where
	// the manifest version always looks "newer").
	CurrentVersion string

	// Arch is the "<os>-<arch>" identifier appended to the manifest URL.
	// Defaults to runtime.GOOS + "-" + runtime.GOARCH.
	Arch string

	// ManifestURL is the base URL of the public downloads CDN where
	// release manifests live (without a trailing arch). The Manager
	// appends "/<arch>/manifest.json" to derive the per-arch
	// endpoint.
	ManifestURL string

	// Storage is the persistent storage used to throttle checks and
	// remember user choices. The Manager partitions it under
	// "ideupgrade".
	Storage storageapi.Service

	// HTTPClient overrides the http.Client used for manifest fetches
	// and artifact downloads.
	HTTPClient *http.Client

	// Executable returns the running binary's path. Defaults to
	// os.Executable. Exposed so tests can simulate running from
	// different install layouts.
	Executable func() (string, error)

	// Notifications, when set, is used to surface upgrade progress and
	// errors in the IDE.
	Notifications browserapi.Notifications

	// WindowManager, when set, is used to render the upgrade prompt as
	// a floating window.
	WindowManager browserapi.WindowManager

	// Parser is used by the prompt's markdown renderer.
	Parser syntaxapi.Parser

	// ScheduleNextTick schedules fn to run on the IDE event loop.
	ScheduleNextTick func(func()) bool

	// Now overrides time.Now (for tests).
	Now func() time.Time

	// InitialDelay overrides DefaultInitialDelay (for tests).
	InitialDelay time.Duration

	// CheckPeriod overrides DefaultCheckPeriod (for tests).
	CheckPeriod time.Duration

	// InstallRoot is the directory containing the installed app
	// bundle. Defaults: /Applications on darwin, ~/.local on linux.
	InstallRoot string

	// AppName is the name of the installed app directory inside
	// InstallRoot. Defaults: Rune.app on darwin, rune.app on linux.
	AppName string

	// CLISymlinkPath is the path of the CLI symlink to refresh after
	// a successful upgrade. Defaults to ~/.local/bin/rune.
	CLISymlinkPath string

	// CLIBinaryRelPath is the binary location relative to the app
	// bundle. Defaults: Contents/MacOS/rune on darwin, bin/rune on
	// linux.
	CLIBinaryRelPath string

	// CacheDir is where downloaded artifacts land before
	// installation. Defaults to <user cache>/rune/upgrade.
	CacheDir string

	// BackupRetention is the number of `.bak-*` snapshots to keep
	// after a successful upgrade. Defaults to DefaultBackupKeep.
	BackupRetention int
}

// state is the persistent record of what the user has decided about
// the latest available version, plus the throttle timestamp.
type state struct {
	LastCheckedAt   time.Time `json:"last_checked_at"`
	RemindAfter     time.Time `json:"remind_after"`
	SkippedVersions []string  `json:"skipped_versions"`
}

// Manager runs the periodic check loop and orchestrates downloads,
// prompts and upgrades. It is safe for concurrent use; the public
// methods serialize via mu.
type Manager struct {
	cfg Config
	ops platformOps

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
}

// New returns a new Manager wired with the production platform
// operations for the current OS. Required Config fields:
// CurrentVersion, ManifestURL, Storage. Other fields have sensible
// defaults applied.
func New(cfg Config) (*Manager, error) {
	// HTTPClient is materialized here (rather than inside
	// newWithPlatformOps) so the production platformOps and the
	// manifest fetcher share the same client.
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = defaultHTTPClient()
	}
	// protects against nil notifications and auto-schedules
	// to avoid data races on notifications
	cfg.Notifications = newScheduledNotifications(
		cfg.Notifications, cfg.ScheduleNextTick)
	return newWithPlatformOps(cfg, newPlatformOps(cfg.HTTPClient))
}

// newWithPlatformOps constructs a Manager with the given platformOps.
// It is the seam used by tests inside this package to inject a fake
// implementation; production code goes through New.
func newWithPlatformOps(cfg Config, ops platformOps) (*Manager, error) {
	if ops == nil {
		return nil, errors.New("ideupgrade: platformOps is nil")
	}
	if cfg.Storage == nil {
		return nil, errors.New("ideupgrade: Storage is required")
	}
	if cfg.ManifestURL == "" {
		return nil, errors.New("ideupgrade: ManifestURL is required")
	}
	if err := requireSecureURL(cfg.ManifestURL); err != nil {
		return nil, fmt.Errorf("ideupgrade: ManifestURL: %w", err)
	}
	if cfg.Arch == "" {
		cfg.Arch = runtime.GOOS + "-" + runtime.GOARCH
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = defaultHTTPClient()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.InitialDelay == 0 {
		cfg.InitialDelay = DefaultInitialDelay
	}
	if cfg.CheckPeriod == 0 {
		cfg.CheckPeriod = DefaultCheckPeriod
	}
	if cfg.BackupRetention == 0 {
		cfg.BackupRetention = DefaultBackupKeep
	}
	if cfg.InstallRoot == "" {
		cfg.InstallRoot = defaultInstallRoot()
	}
	if cfg.AppName == "" {
		cfg.AppName = defaultAppName()
	}
	if cfg.CLISymlinkPath == "" {
		cfg.CLISymlinkPath = defaultCLISymlinkPath()
	}
	if cfg.CLIBinaryRelPath == "" {
		cfg.CLIBinaryRelPath = defaultCLIBinaryRelPath()
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = defaultCacheDir()
	}
	if cfg.ScheduleNextTick == nil {
		return nil, fmt.Errorf("ScheduleNextTick is nil")
	}

	storage, err := cfg.Storage.Partition(storagePartition)
	if err != nil {
		return nil, fmt.Errorf("partition storage: %w", err)
	}
	cfg.Storage = storage

	return &Manager{cfg: cfg, ops: ops}, nil
}

// Start launches the background check loop. Calling Start more than
// once is a no-op.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	ctx, m.cancel = context.WithCancel(ctx)
	m.running = true
	m.mu.Unlock()

	go debug.CapturePanicReport(func() {
		m.run(ctx)
	})
}

// Close cancels the background check loop. Subsequent calls are no-ops.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.running = false
	return nil
}

// CheckNow performs a manifest fetch + prompt outside the regular
// throttle. Used by the `:upgrade` command.
func (m *Manager) CheckNow(ctx context.Context) error {
	manifest, has, err := m.fetchManifest(ctx)
	if err != nil {
		return err
	}
	if !has {
		_, _ = m.cfg.Notifications.Notify(browserapi.LevelInfo,
			"Rune is up to date (%s)", m.cfg.CurrentVersion)
		return nil
	}
	m.persistChecked(ctx)
	m.showPrompt(ctx, manifest)
	return nil
}

// run is the goroutine body for Start. It first waits InitialDelay,
// then ticks every CheckPeriod.
func (m *Manager) run(ctx context.Context) {
	timer := time.NewTimer(m.cfg.InitialDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	m.tick(ctx)

	ticker := time.NewTicker(m.cfg.CheckPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick(ctx)
		}
	}
}

func (m *Manager) tick(ctx context.Context) {
	if m.isThrottled(ctx) {
		return
	}
	manifest, has, err := m.fetchManifest(ctx)
	if err != nil {
		log.WithError(err).Warn("ideupgrade: fetch manifest")
		return
	}
	m.persistChecked(ctx)
	if !has {
		return
	}
	m.showPrompt(ctx, manifest)
}

// fetchManifest downloads the manifest and returns it along with a
// bool indicating whether it represents an upgrade the user has not
// already skipped or postponed.
func (m *Manager) fetchManifest(ctx context.Context) (Manifest, bool, error) {
	if m.cfg.CurrentVersion == "" {
		return Manifest{}, false, nil
	}

	endpoint, err := manifestEndpoint(m.cfg.ManifestURL, m.cfg.Arch)
	if err != nil {
		return Manifest{}, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Manifest{}, false, fmt.Errorf("new request: %w", err)
	}
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return Manifest{}, false, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	// GCS public buckets serve missing objects as 403 (not 404)
	// because the anonymous caller doesn't have permission to learn
	// the object exists. Treat both as "no manifest published yet"
	// — the more common case during initial rollout — so the
	// `:upgrade` UI shows "Rune is up to date" instead of a noisy
	// HTTP error.
	if resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusForbidden {
		return Manifest{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, false,
			fmt.Errorf("manifest fetch %s: status %d", endpoint, resp.StatusCode)
	}
	var manifest Manifest
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Manifest{}, false, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, false, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Version == "" || manifest.URL == "" || manifest.SHA256 == "" {
		return Manifest{}, false, errors.New("manifest missing required fields")
	}
	if err := requireSecureURL(manifest.URL); err != nil {
		return Manifest{}, false, fmt.Errorf("manifest artifact url: %w", err)
	}

	// MinSupportedVersion is enforced before the same/older check so
	// a client that is too old to upgrade in place still learns it
	// must take action, even when the manifest version happens to
	// match the current one.
	if min := manifest.MinSupportedVersion; min != "" &&
		compareVersions(m.cfg.CurrentVersion, min) < 0 {
		return Manifest{}, false, fmt.Errorf(
			"installed version %s is below manifest min_supported_version %s; manual upgrade required",
			m.cfg.CurrentVersion, min)
	}

	// Refuse to "upgrade" to the same or an older version. The same
	// branch also covers a notarized-but-vulnerable older artifact
	// served by a compromised CDN.
	if compareVersions(manifest.Version, m.cfg.CurrentVersion) <= 0 {
		return manifest, false, nil
	}

	st := m.loadState(ctx)
	if slices.Contains(st.SkippedVersions, manifest.Version) {
		return manifest, false, nil
	}
	if !st.RemindAfter.IsZero() && m.cfg.Now().Before(st.RemindAfter) {
		return manifest, false, nil
	}

	return manifest, true, nil
}

// manifestEndpoint joins base with `<arch>/manifest.json` and validates
// the URL. Manifests live on the public downloads CDN (the same bucket
// dist.sh publishes artifacts to), one per-arch object at
// `<base>/<arch>/manifest.json` — there is no server-side proxy.
//
// Any path prefix on base is preserved so callers can host the CDN
// under a sub-path (e.g. https://example.test/cdn/...) for local
// testing without rewriting the URL.
func manifestEndpoint(base, arch string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse manifest base: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + arch + "/manifest.json"
	endpoint := u.String()
	if err := requireSecureURL(endpoint); err != nil {
		return "", err
	}
	return endpoint, nil
}

// requireSecureURL rejects rawURL when it is not https://, unless the
// scheme is exactly "https". There is no loopback exemption: an
// attacker who can bind a service on localhost (a malicious package
// running as the user, a compromised dev tool, a port-forward from a
// hostile network) must not be able to silently feed the upgrader a
// poisoned manifest forever. Tests stand up TLS servers via
// httptest.NewTLSServer and pass srv.Client() through Config.HTTPClient.
func requireSecureURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("url %q is not https", rawURL)
	}
	return nil
}

// compareVersions returns semver.Compare(a, b) but falls back to
// string equality when either side is not a valid semver. Falling
// back to "equal" on unparseable inputs is intentional: it keeps the
// downgrade check from firing on dev-build tags like
// "v0.42.1-2-gabcdef-dirty" that git describe produces.
func compareVersions(a, b string) int {
	if !semver.IsValid(a) || !semver.IsValid(b) {
		if a == b {
			return 0
		}
		// When we cannot compare, behave as if a is older so the
		// downgrade check (a <= b) returns "no upgrade" rather than
		// "yes upgrade" on garbage input. This is the safer default.
		return -1
	}
	return semver.Compare(a, b)
}

func (m *Manager) isThrottled(ctx context.Context) bool {
	st := m.loadState(ctx)
	if st.LastCheckedAt.IsZero() {
		return false
	}
	return m.cfg.Now().Sub(st.LastCheckedAt) < m.cfg.CheckPeriod
}

func (m *Manager) persistChecked(ctx context.Context) {
	st := m.loadState(ctx)
	st.LastCheckedAt = m.cfg.Now()
	m.saveState(ctx, st)
}

// remindLater pushes the next prompt out by one CheckPeriod.
func (m *Manager) remindLater(ctx context.Context) {
	st := m.loadState(ctx)
	st.RemindAfter = m.cfg.Now().Add(m.cfg.CheckPeriod)
	m.saveState(ctx, st)
}

// skipVersion records that the user does not want to be reminded
// about this specific version again.
func (m *Manager) skipVersion(ctx context.Context, version string) {
	st := m.loadState(ctx)
	if !slices.Contains(st.SkippedVersions, version) {
		st.SkippedVersions = append(st.SkippedVersions, version)
	}
	m.saveState(ctx, st)
}

func (m *Manager) loadState(ctx context.Context) state {
	var st state
	if err := m.cfg.Storage.Get(ctx, storageKeyState, &st); err != nil {
		if !errors.Is(err, storageapi.ErrNotFound) {
			log.WithError(err).Warn("ideupgrade: load state")
		}
		return state{}
	}
	return st
}

func (m *Manager) saveState(ctx context.Context, st state) {
	if err := m.cfg.Storage.Set(ctx, storageKeyState, st); err != nil {
		log.WithError(err).Warn("ideupgrade: save state")
	}
}

// runUpgrade kicks off the actual download/install dance and surfaces
// progress through Notifications. It runs synchronously on the
// caller's goroutine.
//
// The install root targeted by the upgrade is *not* taken from
// Config; it is derived from the path of the running binary via
// detectRunningInstall so we upgrade the install that actually
// produced this process. When the running binary is not part of a
// managed install (dev build, sandbox, etc.) we surface a
// notification and return ErrUpgradeNotSupported instead of
// touching anything.
func (m *Manager) runUpgrade(ctx context.Context, manifest Manifest) error {
	var (
		notifID  string
		lastEmit time.Time
	)

	notify := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if notifID == "" {
			newID, err := m.cfg.Notifications.Notify(browserapi.LevelInfo, "%s", msg)
			if err != nil {
				log.WithError(err).Warn("ideupgrade: notify info: upgrade start")
				return
			}
			notifID = newID
			return
		}
		_ = m.cfg.Notifications.UpdateNotificationProgress(notifID, msg, 0, 1)
	}

	progress := func(downloaded, total int64) {
		if total <= 0 || downloaded > total {
			return
		}
		final := downloaded == total
		if !final && time.Since(lastEmit) < upgradeProgressInterval {
			return
		}
		lastEmit = time.Now()
		if notifID == "" {
			return
		}
		_ = m.cfg.Notifications.UpdateNotificationProgress(notifID, "", downloaded, total)
	}

	detected, err := detectRunningInstall(m.cfg)
	if err != nil {
		var notSupported *ErrUpgradeNotSupported
		if errors.As(err, &notSupported) {
			_, _ = m.cfg.Notifications.Notify(browserapi.LevelError,
				"Rune cannot upgrade itself in place: %v",
				notSupported.Reason)
		}
		return err
	}

	err = runUpgrade(ctx, upgradeOpts{
		manifest:         manifest,
		currentVersion:   m.cfg.CurrentVersion,
		cacheDir:         m.cfg.CacheDir,
		installRoot:      detected.InstallRoot,
		appName:          detected.AppName,
		cliSymlinkPath:   m.cfg.CLISymlinkPath,
		cliBinaryRelPath: detected.CLIBinaryRelPath,
		backupRetention:  m.cfg.BackupRetention,
		ops:              m.ops,
		notify:           notify,
		progress:         progress,
	})
	if err != nil {
		if notifID != "" {
			_ = m.cfg.Notifications.UpdateNotificationProgress(notifID, "", 1, 1)
		}
		_, _ = m.cfg.Notifications.Notify(browserapi.LevelError,
			"Upgrade to %s failed: %v", manifest.Version, err)
		return err
	}
	if notifID != "" {
		_ = m.cfg.Notifications.UpdateNotificationProgress(notifID, "", 1, 1)
	}
	_, _ = m.cfg.Notifications.Notify(browserapi.LevelSuccess,
		"Upgrade to %s complete — restart Rune to apply",
		manifest.Version)
	return nil
}

// homeDir returns the user's home directory or "" on error. Used to
// derive the default install paths.
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

func defaultCacheDir() string {
	if cd, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cd, "rune", "upgrade")
	}
	return filepath.Join(os.TempDir(), "rune-upgrade")
}

// defaultHTTPClient returns the http.Client used when the caller of
// New doesn't pass one. The transport bounds dial and
// response-header reads but deliberately leaves the body read
// unbounded so that slow artifact downloads can complete.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   DefaultHTTPDialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   DefaultHTTPDialTimeout,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: DefaultHTTPResponseHeaderTimeout,
		},
	}
}

func defaultCLISymlinkPath() string {
	return filepath.Join(homeDir(), ".local", "bin", "rune")
}
