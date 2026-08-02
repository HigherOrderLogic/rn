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

package oxapitest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
	blueauth "github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/blue/document"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/blue/release/cdnrelease"
	"github.com/unstablebuild/blue/release/docrelease"
	"github.com/unstablebuild/blue/release/gcsrelease"
	"github.com/unstablebuild/ox-api/api/oxapi"
	"github.com/unstablebuild/ox-api/auth"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/docmarshal/docbson"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"golang.org/x/oauth2"
	"unstable.build/go-tui/handler/handlertest"
	goide "unstable.build/go-tui/ide"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
	"unstable.build/go-tui/ide/pkgtrust"
	"unstable.build/go-tui/localstorage"
	"unstable.build/go-tui/text"
)

func TestReleaseInstallE2E(t *testing.T) {
	t.Parallel()

	const pkgID = "six"
	const latestVersion = release.Version("2")
	bucketName := os.Getenv("OXAPI_TEST_GCS_BUCKET")
	if bucketName == "" {
		t.Skip("set OXAPI_TEST_GCS_BUCKET and service-account ADC to run real GCS integration")
	}

	priv := loadKey(t, "../ox-api/testdata/jwk-priv.json")
	pub1 := loadKey(t, "../ox-api/testdata/jwk-pub1.json.pub")
	pub2 := loadKey(t, "../ox-api/testdata/jwk-pub2.json.pub")
	pub3 := loadKey(t, "../ox-api/testdata/jwk-pub3.json.pub")
	keys := blueauth.StaticAsymmetricKeys(priv, pub1, pub2, pub3)

	rpcToken, err := blueauth.SignToken(priv,
		"goodCitizen", "citizen@unstable.build",
		auth.RPCUser{
			ID:      "goodCitizen",
			Email:   "citizen@unstable.build",
			Role:    auth.RoleUser,
			Account: "test-account",
		},
		time.Hour,
	)
	require.NoError(t, err)

	pkgs := idepkgtest.MakePackages(release.Package{Name: pkgID, Latest: latestVersion})
	bundles := idepkgtest.MakeBundles([]release.Bundle{
		{Package: pkgID, Version: "1", CreatedAt: time.Now().Add(-time.Hour)},
		{Package: pkgID, Version: latestVersion, CreatedAt: time.Now()},
	})
	source := idepkgtest.NewReleaseManager(pkgs, bundles)

	gcsClient, err := storage.NewClient(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = gcsClient.Close()
	})
	inner := docrelease.NewManager(document.NewInMemoryService())
	prefix := fmt.Sprintf("oxapi-e2e/%d", time.Now().UnixNano())
	releaseManager := gcsrelease.NewManager(inner, gcsrelease.NewBucket(gcsClient.Bucket(bucketName)),
		gcsrelease.WithPrefix(prefix),
	)
	seedPackageFromMockReleaseManager(t, source, releaseManager, pkgID, "1")
	seedPackageFromMockReleaseManager(t, source, releaseManager, pkgID, latestVersion)
	t.Cleanup(func() {
		_ = releaseManager.Delete(context.Background(), pkgID, "1")
		_ = releaseManager.Delete(context.Background(), pkgID, latestVersion)
	})

	listener, err := newTCPListener()
	require.NoError(t, err)
	baseURL := "http://" + listener.Addr().String()

	apiURL, err := url.Parse(baseURL)
	require.NoError(t, err)
	signupURL, err := url.Parse(baseURL + "/signup")
	require.NoError(t, err)

	secretStore := blueauth.MapSecretStore(map[string][]byte{
		oxapi.Auth0SecretID: []byte("1234"),
	}, "client_id", "xxxx")

	authCfg := auth.DefaultM2MConfig()
	authCfg.APIURL = baseURL + "/api"
	authCfg.Endpoint.TokenURL = baseURL + auth.ServeTokenPath
	authCfg.Endpoint.AuthURL = baseURL + "/o/oauth2/auth"
	authCfg.JWKSURL = baseURL + "/o/oauth2/jwk"

	apiHandler, err := oxapi.NewHTTPApi(
		document.NewInMemoryService(),
		keys,
		keys,
		secretStore,
		time.Hour,
		signupURL,
		apiURL,
		authCfg,
		[]oxapi.ArchRelease{{
			Arch:    fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH),
			Manager: releaseManager,
			Signer:  releaseManager,
		}},
		oxapi.RPCAuthorizer("issues", []string{"releases"}),
		stubReportStore{}, oxapi.ReportConfig{},
		nil, // stripeClient
		nil, // stripeCfg
		nil, // eventsStore
		nil, // billingMailer
		"",  // billingMailFrom
		nil, // contactHandler
		nil, // newsletterHandler
		nil, // newsletterStore
		oxapi.HealthConfig{
			ProbeSecret:   "test-probe-secret",
			ReleaseBucket: gcsClient.Bucket(bucketName),
		},
	)
	require.NoError(t, err)

	root := http.NewServeMux()
	root.Handle("/", apiHandler)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- http.Serve(listener, root)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case err := <-serverErr:
			if err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) {
				t.Errorf("http serve: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("timed out waiting for test server shutdown")
		}
	})

	httpClient := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: rpcToken, TokenType: "Bearer"},
	))
	arch := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	clientReleaseManager := cdnrelease.NewManager(httpClient, baseURL+"/api/releases/"+arch)

	dataDir := t.TempDir()
	configFile, err := os.CreateTemp(dataDir, "iderc-*.yaml")
	require.NoError(t, err)
	_, err = configFile.WriteString(`
clipboard: memory
editor:
  mode: modal
command:
  key: ":"
  show_manual: false
workspace:
  wallpaper: workspaceWallpaper
  auto_restore: false
browser:
  workspace_bar: number
  window_manager:
    no_max_size: false
notifications:
  progress_bar: false
`)
	require.NoError(t, err)
	require.NoError(t, configFile.Close())

	ideInstance, err := goide.New(
		dataDir,
		configFile.Name(),
		dataDir,
		pkgtrust.NewStore(dataDir, nil),
		localstorage.New(context.Background(), dataDir, docbson.Marshaler()),
		goide.WithReleaseManager(clientReleaseManager),
		goide.WithScheduleNextTick(func(fn func()) bool {
			fn()
			return true
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ideInstance.Close()
	})

	require.NoError(t, ideInstance.SubscribeCommand(
		textapi.CommandManual{Name: "pkgwait"},
		text.FuncCommandHandler(func(_ context.Context, cmd textapi.Command) error {
			require.Len(t, cmd.Args, 1)
			deadline := time.Now().Add(10 * time.Second)
			pkgLibDir := filepath.Join(dataDir, "lib", cmd.Args[0])
			for time.Now().Before(deadline) {
				if _, err := os.Stat(pkgLibDir); err == nil {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
			return fmt.Errorf("timed out waiting for package %s install", cmd.Args[0])
		}, nil),
	))

	h := ideInstance.Ready()
	handlertest.RunHandlerSequence(t, h, 40, 15, []handlertest.SequenceTestCase{{
		InputSequence: ":pkginstall<space>six<enter>:pkgwait<space>six<enter>",
		Expected: `┌──────────────────────────────────────┐
│                                      │
├──────────────────────────────────────┤
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
│          workspaceWallpaper          │
│                                      │
│                                      │
│                                      │
│                                      │
│                                      │
└──────────────────────────────────────┘`,
	}})

	linkTarget, err := os.Readlink(filepath.Join(dataDir, "lib", pkgID))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dataDir, "pkg", pkgID, string(latestVersion)), linkTarget)

	for _, rel := range []string{
		"testpkg/README.md",
		"testpkg/VERSION",
		"testpkg/lib/time/zoneinfo.zip",
	} {
		_, err := os.Stat(filepath.Join(dataDir, "lib", pkgID, rel))
		require.NoError(t, err, rel)
	}

	binEntries, err := os.ReadDir(filepath.Join(dataDir, "bin"))
	require.NoError(t, err)
	require.Empty(t, binEntries)
}

func seedPackageFromMockReleaseManager(
	t *testing.T,
	src *idepkgtest.ReleaseManager,
	dst *gcsrelease.Manager,
	pkgID string,
	version release.Version,
) {
	t.Helper()

	ctx := context.Background()
	pkg, err := src.GetPackage(ctx, pkgID)
	require.NoError(t, err)
	if _, err := dst.GetPackage(ctx, pkgID); err != nil {
		require.NoError(t, dst.Create(ctx, pkg))
	}

	var buf bytes.Buffer
	bundle, err := src.Get(ctx, pkgID, version, release.NopProgressWriter(&buf))
	require.NoError(t, err)
	require.NoError(t, dst.Upload(ctx, bundle, release.NopProgressReader(bytes.NewReader(buf.Bytes()))))
}

func loadKey(t *testing.T, filename string) blueauth.Key {
	t.Helper()
	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	if strings.HasSuffix(filename, ".pub") {
		key, err := blueauth.LoadPublicKey(data)
		require.NoError(t, err)
		return key
	}
	key, err := blueauth.LoadPrivateKey(data)
	require.NoError(t, err)
	return key
}

func newTCPListener() (net.Listener, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	return listener, nil
}

type stubReportStore struct{}

func (stubReportStore) Store(_ context.Context, _ string, _ []byte, _ string, _ map[string]string) error {
	return nil
}
