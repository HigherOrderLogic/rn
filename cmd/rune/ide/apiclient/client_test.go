// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package apiclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/ox-api/auth"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi/storagestub"
	"golang.org/x/oauth2"
)

func TestNewDoesNotStartTelemetryWhenDisabled(t *testing.T) {
	requests := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// New always fetches the oauth2 config in the background for
		// the package trust keyring; only telemetry traffic matters.
		if r.URL.Path == auth.ServeConfigPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	select {
	case <-requests:
		t.Fatal("telemetry request sent with disabled telemetry")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNewStartsTelemetryWhenEnabled(t *testing.T) {
	requests := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL
	config.EnableTelemetry = true
	config.InstallBackupDir = t.TempDir()

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	for {
		select {
		case path := <-requests:
			if path == telemetryPath {
				return
			}
		case <-timer.C:
			t.Fatal("expected telemetry request when telemetry is enabled")
		}
	}
}

func TestNewPanicsOnZeroPeriodWithTelemetryEnabled(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for zero TelemetryPeriod with EnableTelemetry=true")
		}
	}()
	config := DefaultConfig()
	config.EnableTelemetry = true
	config.TelemetryPeriod = 0
	config.HTTPEndpointAddress = "http://localhost"
	config.InstallBackupDir = t.TempDir()
	_ = New(storagestub.NewInMemoryService(), config, t.TempDir())
}

func TestNewPanicsOnEmptyInstallBackupDirWithTelemetryEnabled(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty InstallBackupDir with EnableTelemetry=true")
		}
	}()
	config := DefaultConfig()
	config.EnableTelemetry = true
	config.HTTPEndpointAddress = "http://localhost"
	_ = New(storagestub.NewInMemoryService(), config, t.TempDir())
}

func TestInstallTampered(t *testing.T) {
	newClient := func(t *testing.T, enableTelemetry bool, backupDir string) *Client {
		t.Helper()
		config := DefaultConfig()
		config.HTTPEndpointAddress = "http://localhost"
		config.EnableTelemetry = enableTelemetry
		config.InstallBackupDir = backupDir
		client := New(storagestub.NewInMemoryService(), config, t.TempDir())
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		return client
	}

	t.Run("fresh install is not tampered", func(t *testing.T) {
		dir, _ := testInstallIDBackup(t)
		assert.False(t, newClient(t, true, dir).InstallTampered())
	})

	t.Run("wiped store with surviving backup is tampered", func(t *testing.T) {
		dir, tempPath := testInstallIDBackup(t)
		require.NoError(t, os.WriteFile(tempPath, []byte("backup-id"), 0o600))
		assert.True(t, newClient(t, true, dir).InstallTampered())
	})

	t.Run("disabled telemetry never reports tampering", func(t *testing.T) {
		dir, tempPath := testInstallIDBackup(t)
		require.NoError(t, os.WriteFile(tempPath, []byte("backup-id"), 0o600))
		assert.False(t, newClient(t, false, dir).InstallTampered())
	})
}

// fakeOAuthServer responds with a minimal /config endpoint and a 400
// invalid_grant on /token so the test cannot accidentally complete a flow.
func fakeOAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv
}

func TestClient_TokenSource_NoImplicitBrowserFlow(t *testing.T) {
	srv := fakeOAuthServer(t)
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL
	var browserCalls atomic.Int32
	config.OpenBrowser = func(*url.URL) error {
		browserCalls.Add(1)
		return nil
	}

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer client.Close()

	_, err := client.OAuthTokenSource().Token()
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrNotAuthenticated),
		"expected ErrNotAuthenticated, got %v", err)
	assert.Equal(t, int32(0), browserCalls.Load(),
		"browser must not be opened when :login was not invoked")
}

func TestClient_Login_AttemptsBrowserFlow(t *testing.T) {
	srv := fakeOAuthServer(t)
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL
	browserCalls := make(chan struct{}, 1)
	config.OpenBrowser = func(*url.URL) error {
		select {
		case browserCalls <- struct{}{}:
		default:
		}
		return errors.New("test: browser not actually opened")
	}

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer client.Close()

	ctx, cancel := context.WithCancel(t.Context())
	session := client.Login(ctx)

	select {
	case <-browserCalls:
	case <-time.After(5 * time.Second):
		t.Fatal("expected browser to be opened by :login flow")
	}

	// The URL is still published so the wait prompt can show it.
	select {
	case u, ok := <-session.URL:
		require.True(t, ok, "URL channel must emit before close")
		require.NotNil(t, u, "URL channel must emit a non-nil URL")
	case <-time.After(5 * time.Second):
		t.Fatal("expected Login to publish the OAuth URL")
	}

	// A failed browser open must NOT abort the flow: Done stays open so
	// the user can copy the URL and finish sign-in in any browser.
	select {
	case err := <-session.Done:
		t.Fatalf("Login must stay alive after browser-open failure, got Done=%v", err)
	case <-time.After(200 * time.Millisecond):
	}

	// Cancelling the context tears the flow down and resolves Done.
	cancel()
	select {
	case <-session.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected Login to resolve after context cancellation")
	}
}

// TestClient_Login_SecondAttemptAfterCancel reproduces the bug where
// cancelling an in-flight Login leaves the OAuth goroutine blocked
// inside blueauth.NewClientWithPorts holding the CachedTokenSource
// lock, so a subsequent Login never reaches openBrowser and never
// resolves.
func TestClient_Login_SecondAttemptAfterCancel(t *testing.T) {
	srv := fakeOAuthServer(t)
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL
	browserCalls := make(chan struct{}, 4)
	var browserBehavior atomic.Value
	browserBehavior.Store(func() error { return nil })
	config.OpenBrowser = func(*url.URL) error {
		browserCalls <- struct{}{}
		return browserBehavior.Load().(func() error)()
	}

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer client.Close()

	ctx1, cancel1 := context.WithCancel(t.Context())
	session1 := client.Login(ctx1)

	select {
	case <-browserCalls:
	case <-time.After(5 * time.Second):
		t.Fatal("expected first Login to open browser")
	}

	cancel1()

	select {
	case <-session1.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected first Login to publish completion after cancel")
	}

	browserBehavior.Store(func() error {
		return errors.New("test: browser not actually opened")
	})

	ctx2, cancel2 := context.WithCancel(t.Context())
	session2 := client.Login(ctx2)

	select {
	case <-browserCalls:
	case <-time.After(5 * time.Second):
		t.Fatal("expected second Login to open browser after cancel of first")
	}

	// A failed browser open keeps the flow alive; cancelling resolves it.
	cancel2()
	select {
	case <-session2.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected second Login to publish completion after cancel")
	}
}

// TestClient_Login_PublishesOAuthURL verifies that the LoginSession
// surfaces the OAuth authorize URL — the same URL handed to the
// browser — so the bootstrap UI can inline it in the wait prompt.
// The URL channel is the only signal that drives the pre-swap login
// flow's "click here" link; if a refactor breaks the ctx plumbing
// that carries it from Login down into tokenSourceRefresh, the
// channel silently never resolves and the user is stuck staring at
// an empty prompt while the browser opens in the background.
func TestClient_Login_PublishesOAuthURL(t *testing.T) {
	srv := fakeOAuthServer(t)
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL
	browserURLCh := make(chan *url.URL, 1)
	config.OpenBrowser = func(u *url.URL) error {
		select {
		case browserURLCh <- u:
		default:
		}
		return errors.New("test: browser not actually opened")
	}

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer client.Close()

	ctx, cancel := context.WithCancel(t.Context())
	session := client.Login(ctx)

	var published *url.URL
	select {
	case u, ok := <-session.URL:
		require.True(t, ok, "URL channel must emit before close")
		require.NotNil(t, u, "URL channel must emit a non-nil URL")
		assert.NotEmpty(t, u.String())
		assert.Contains(t, u.Query(), "state",
			"published URL must be an OAuth authorize URL (has state)")
		assert.Contains(t, u.Query(), "redirect_uri",
			"published URL must be an OAuth authorize URL (has redirect_uri)")
		published = u
	case <-time.After(5 * time.Second):
		t.Fatal("expected Login to publish OAuth URL")
	}

	select {
	case browserURL := <-browserURLCh:
		assert.Equal(t, published.String(), browserURL.String(),
			"URL handed to openBrowser must match the URL published on session.URL")
	case <-time.After(5 * time.Second):
		t.Fatal("expected openBrowser to be invoked with the same URL")
	}

	// A failed browser open must not abort the flow; cancelling resolves it.
	cancel()
	select {
	case <-session.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected Login Done to resolve after context cancellation")
	}

	_, ok := <-session.URL
	assert.False(t, ok, "URL channel must be closed after Login completes")
}

// TestClient_Login_CompletesWhileBrowserOpenerBlocks reproduces the
// Linux login deadlock: the underlying OAuth client only starts
// reading the callback result after the visit-URL callback returns, so
// an opener that blocks for the lifetime of the browser process wedges
// the loopback redirect handler. The browser tab then spins forever on
// the redirect and login never completes.
func TestClient_Login_CompletesWhileBrowserOpenerBlocks(t *testing.T) {
	srv := completingOAuthServer(t)
	defer srv.Close()

	config := DefaultConfig()
	config.HTTPEndpointAddress = srv.URL
	openerRelease := make(chan struct{})
	t.Cleanup(func() { close(openerRelease) })
	config.OpenBrowser = func(*url.URL) error {
		<-openerRelease
		return nil
	}

	client := New(storagestub.NewInMemoryService(), config, t.TempDir())
	defer client.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	session := client.Login(ctx)

	var authorizeURL *url.URL
	select {
	case u, ok := <-session.URL:
		require.True(t, ok, "URL channel must emit before close")
		require.NotNil(t, u)
		authorizeURL = u
	case <-time.After(5 * time.Second):
		t.Fatal("expected Login to publish OAuth URL")
	}

	redirectURI := authorizeURL.Query().Get("redirect_uri")
	require.NotEmpty(t, redirectURI)
	state := authorizeURL.Query().Get("state")
	require.NotEmpty(t, state)

	callbackURL, err := url.Parse(redirectURI)
	require.NoError(t, err)
	q := callbackURL.Query()
	q.Set("code", "fake-auth-code")
	q.Set("state", state)
	callbackURL.RawQuery = q.Encode()

	// The browser tab hitting the loopback redirect must get a response
	// back even though the opener is still running.
	browserTab := &http.Client{Timeout: 5 * time.Second}
	resp, err := browserTab.Get(callbackURL.String())
	require.NoError(t, err, "loopback redirect must respond while the opener blocks")
	_, _ = io.Copy(io.Discard, resp.Body)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case err := <-session.Done:
		assert.NoError(t, err, "Login must complete after the redirect")
	case <-time.After(5 * time.Second):
		t.Fatal("expected Login to complete after the OAuth redirect")
	}
}

// completingOAuthServer serves an oauth2 config pointing back at itself
// and a token endpoint that completes the PKCE code exchange.
func completingOAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	var base atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc(auth.ServeConfigPath, func(w http.ResponseWriter, _ *http.Request) {
		u, _ := base.Load().(string)
		cfg := auth.Config{
			APIURL:    u + "/api/v2/",
			JWKSURL:   u + "/.well-known/jwks.json",
			SignupURL: u + "/signup",
			Config: oauth2.Config{
				// An empty ClientSecret selects the PKCE flow.
				ClientID: "test-client-id",
				Scopes:   []string{"offline_access", "openid"},
				Endpoint: oauth2.Endpoint{
					AuthStyle: oauth2.AuthStyleInParams,
					AuthURL:   u + "/authorize",
					TokenURL:  u + auth.ServeTokenPath,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(cfg))
	})
	mux.HandleFunc(auth.ServeTokenPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token",` +
			`"token_type":"Bearer","expires_in":3600,` +
			`"refresh_token":"fake-refresh-token"}`))
	})
	srv := httptest.NewServer(mux)
	base.Store(srv.URL)
	return srv
}

func TestParseAccountClaims(t *testing.T) {
	planEnds := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	token := makeAccountJWT(t, auth.RPCUser{
		ID:       "auth0|abc",
		Email:    "user@example.com",
		Role:     auth.RolePaid,
		Account:  "acc-1",
		PlanEnds: planEnds,
	})

	user, err := parseAccountClaims(token)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", user.Email)
	assert.Equal(t, auth.RolePaid, user.Role)
	assert.Equal(t, "acc-1", user.Account)
	assert.True(t, planEnds.Equal(user.PlanEnds))
}

func TestParseAccountClaimsRejectsMalformedToken(t *testing.T) {
	_, err := parseAccountClaims("not-a-jwt")
	require.Error(t, err)
}

func makeAccountJWT(t *testing.T, user auth.RPCUser) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(struct {
		Extra auth.RPCUser `json:"extra"`
	}{Extra: user})
	require.NoError(t, err)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
