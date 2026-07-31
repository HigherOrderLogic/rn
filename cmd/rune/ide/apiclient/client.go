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
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ernestrc/go-multierror"
	"github.com/ernestrc/sensible/browser"
	log "github.com/sirupsen/logrus"
	blueauth "github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/logging/trace"
	"github.com/unstablebuild/ox-api/api"
	"github.com/unstablebuild/ox-api/auth"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"unstable.build/go-tui/debug"
)

//go:embed callback_page.html
var callbackPageHTML string

// Client implements a client to an instance of ox-api.
// This client should be subscribed to events as a text.EventHandler,
// for the telemetry implementation to collect all stats.
type Client struct {
	config               Config
	dataDir              string
	httpEndpointURL      *url.URL
	tokenSource          *auth.CachedTokenSource
	storage              storageapi.Service
	ctx                  context.Context
	ctxCancel            func()
	telemetry            *telemetry
	telemetryTokenSource *auth.CachedTokenSource
}

// New returns allocates storage for a new Client and initializes it.
func New(
	storage storageapi.Service,
	config Config, dataDir string,
) *Client {
	httpEndpointURL, err := url.Parse(config.HTTPEndpointAddress)
	if err != nil {
		panic(fmt.Sprintf("parse http endpoint url: %s", err))
	}
	if config.OpenBrowser == nil {
		config.OpenBrowser = openBrowser
	}
	ret := &Client{
		config:          config,
		dataDir:         dataDir,
		httpEndpointURL: httpEndpointURL,
	}
	authStorage := storageapi.WithPartition(storage, "auth")
	ret.storage = authStorage
	ret.tokenSource = auth.NewCachedTokenSource(ret, authStorage)
	ret.ctx, ret.ctxCancel = context.WithCancel(context.Background())

	if config.EnableTelemetry {
		// for telemetry we only want to use the cached token, if there's any
		// or refresh token
		refreshOnlySourcer := auth.FuncTokenSourcer(
			func(ctx context.Context, t *oauth2.Token) (oauth2.TokenSource, error) {
				return ret.tokenSourceRefresh(ctx, t, true)
			})
		ret.telemetryTokenSource = auth.NewCachedTokenSource(refreshOnlySourcer, authStorage)
		telemetryStorage := storageapi.WithPartition(storage, "telemetry")
		ret.telemetry = newTelemetry(ret.telemetryTokenSource,
			ret.httpEndpointURL, ret.config.TelemetryPeriod, debug.Tag,
			ret.config.EditorMode, telemetryStorage, ret.config.InstallBackupDir)
		ret.telemetry.start()
	}

	return ret
}

// openBrowser launches the user's preferred browser and returns as soon
// as the process is started. browser.Browse would instead block until
// the browser exits, and its shared session state rejects a second
// login while the first browser is still open.
func openBrowser(u *url.URL) error {
	b, err := browser.FindBrowser()
	if err != nil {
		return err
	}
	if err := b.Start(u); err != nil {
		return err
	}
	go debug.CapturePanicReport(func() {
		if err := b.Wait(); err != nil {
			log.Warnf("oauth2: browser process: %v", err)
		}
	})
	return nil
}

// Handle satisfies text.EventHandler.
func (t *Client) Handle(ctx context.Context, ev textapi.Event) bool {
	if t.telemetry == nil {
		return false
	}
	return t.telemetry.Handle(ctx, ev)
}

// TelemetryEnabled returns whether telemetry is active for this client.
func (a *Client) TelemetryEnabled() bool {
	return a.telemetry != nil
}

// InstallTampered reports whether install-ID resolution found a wiped
// data directory with a surviving backup identifier — the signal of
// an attempt to reset local state. Detection runs only when telemetry
// is enabled, and it self-heals: the first client to observe the
// signal consumes it, so callers must persist the flag if they need
// it beyond this process.
func (a *Client) InstallTampered() bool {
	return a.telemetry != nil && a.telemetry.tampered
}

// TokenSource satisfies auth.TokenSourcer.
func (a *Client) TokenSource(ctx context.Context, token *oauth2.Token) (
	oauth2.TokenSource, error,
) {
	return a.tokenSourceRefresh(ctx, token, false)
}

// OAuthTokenSource returns the underlying oauth2.TokenSource used for
// authenticated requests. This can be used to build an *http.Client via
// oauth2.NewClient for HTTP-based APIs such as cdnrelease.
func (a *Client) OAuthTokenSource() oauth2.TokenSource {
	return a.tokenSource
}

// CachedTokenSource returns the underlying *auth.CachedTokenSource so
// callers can read cached JWT claims or purge the token without going
// through the gRPC transport.
func (a *Client) CachedTokenSource() *auth.CachedTokenSource {
	return a.tokenSource
}

// Logout purges the user's underlying authentication credentials,
// so next requests sent to the server via the connections created via NewConn
// will be un-authenticated.
func (a *Client) Logout(ctx context.Context) error {
	if err := a.tokenSource.Purge(); err != nil {
		log.Warnf("cache reset: %v", err)
	}
	// NOTE: we do not want to purge telemetry token source, as it doesn't
	// have effect on the user functionality, but we want to maintain authed logs
	// _ = a.telemetryTokenSource.Purge()
	return nil
}

// LoginSession exposes the asynchronous state of an in-flight Login
// call. URL emits the OAuth authorization URL as soon as the
// underlying flow computes it (after binding the local callback
// port), then closes. Done receives the result of the underlying
// CachedTokenSource.Token() call (nil on success), then closes.
type LoginSession struct {
	URL  <-chan *url.URL
	Done <-chan error
}

// Login authenticates the user using a browser oauth2 flow. Cancelling
// ctx propagates into CachedTokenSource.TokenCtx so callers may abort
// a stuck OAuth round-trip; the session's Done channel still resolves
// (with the cancellation error) and URL is closed without emitting if
// the cancellation beats the local server binding.
func (a *Client) Login(ctx context.Context) LoginSession {
	urlCh := make(chan *url.URL, 1)
	done := make(chan error, 1)
	ctx = withLoginURLCh(ctx, urlCh)
	go debug.CapturePanicReport(func() {
		defer close(done)
		defer close(urlCh)
		_, err := a.tokenSource.TokenCtx(ctx)
		if err != nil {
			log.Warnf("login: %v", err)
			select {
			case done <- err:
			case <-ctx.Done():
			}
			return
		}
		select {
		case done <- nil:
		case <-ctx.Done():
		}
	})
	return LoginSession{URL: urlCh, Done: done}
}

// AccountStatus returns the authenticated user's account details parsed
// from the cached access token. It returns ok=false when no token is
// cached (the user is not signed in) and an error only when a cached
// token cannot be decoded.
func (a *Client) AccountStatus(ctx context.Context) (user auth.RPCUser, ok bool, err error) {
	tok := a.tokenSource.Cached(ctx)
	if tok == nil || tok.AccessToken == "" {
		return auth.RPCUser{}, false, nil
	}
	user, err = parseAccountClaims(tok.AccessToken)
	if err != nil {
		return auth.RPCUser{}, false, err
	}
	return user, true, nil
}

// parseAccountClaims decodes the JWT payload without verifying the
// signature: ox-api verifies tokens server-side before issuing them.
// The account fields live in the token's "extra" claim, which ox-api's
// granter serializes from auth.RPCUser.
func parseAccountClaims(token string) (auth.RPCUser, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return auth.RPCUser{}, fmt.Errorf("jwt: expected at least 2 segments, got %d", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return auth.RPCUser{}, fmt.Errorf("jwt: decode payload: %w", err)
	}
	var payload struct {
		Extra auth.RPCUser `json:"extra"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return auth.RPCUser{}, fmt.Errorf("jwt: unmarshal payload: %w", err)
	}
	return payload.Extra, nil
}

// Dial creates a new grpc.ClientConn that uses the underlying
// user authentication state to send authenticated or unauthenticated requests.
// The returned grpc.ClientConn's lifecycle is responsibility of the caller, i.e.
// grpc.ClientConn.Close is not called when Client.Close is called.
func (a *Client) Dial() (*grpc.ClientConn, error) {
	rpcCreds := oauth.TokenSource{TokenSource: a.tokenSource}
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(api.MaxRecvMsgSize),
			grpc.MaxCallRecvMsgSize(api.MaxSendMsgSize),
		),
	}

	if a.config.InsecureTransport {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			err = fmt.Errorf("x509 system cert pool: %v", err)
			return nil, err
		}
		transportCreds := credentials.NewClientTLSFromCert(certPool, "")
		opts = append(opts, grpc.WithTransportCredentials(transportCreds))
		opts = append(opts, grpc.WithPerRPCCredentials(rpcCreds))
	}

	conn, err := grpc.NewClient(a.config.GRPCEndpointAddress, opts...)
	if err != nil {
		err = fmt.Errorf("dial rune GRPC address: %v", err)
		return nil, err
	}
	return conn, nil
}

// Close closes all resources associated with this Client,
// except connections created via NewConn.
func (a *Client) Close() (ret error) {
	if a.telemetry != nil {
		if err := a.telemetry.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if a.storage != nil {
		if err := a.storage.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	a.ctxCancel()
	return
}

var (
	tryPorts = []int{
		11524, 12524, 12624,
		21524, 22524, 22624,
		31524, 32524, 32624,
		41524, 42524, 42624,
		51524, 52524, 52624,
		61524, 62524, 62624,
	}
)

// defaultProdNativeConfig returns the production oauth2 configuration used as a
// fallback when the server is unreachable and FetchConfig fails. It starts from
// auth.DefaultNativeConfig (to keep scopes and the ox-api token-proxy endpoint)
// and overrides only the production-specific URLs and client id. auth's
// package-level endpoints default to the development tenant unless overridden
// via -ldflags at prod build time (see cmd/ox-api/Makefile), so the rune binary
// must hardcode the production values here.
func defaultProdNativeConfig(api *url.URL) auth.Config {
	conf := auth.DefaultNativeConfig(api)
	conf.APIURL = "https://rune-prod.us.auth0.com/api/v2/"
	conf.JWKSURL = "https://auth.rune.build/.well-known/jwks.json"
	conf.SignupURL = "https://rune.build/signup"
	conf.MgmtTokenURL = "https://rune-prod.us.auth0.com/oauth/token"
	conf.ClientID = "XHBpJIm3q6PYazpxZMAhcwxAuR5Ks9B7"
	conf.Endpoint.AuthURL = "https://auth.rune.build/authorize"
	return conf
}

func (a *Client) tokenSourceRefresh(ctx context.Context, token *oauth2.Token, refreshOnly bool) (
	oauth2.TokenSource, error,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// propagate traceID to grantee logs
	traceID, ctx := trace.FromContextOrNew(ctx)
	log := log.WithFields(log.Fields{logging.KeyTraceID: traceID})

	conf, err := auth.FetchConfig(a.httpEndpointURL)
	if err != nil {
		log.Warnf("could not fetch oauth2 configuration, fallback to builtin: %v", err)
		conf = defaultProdNativeConfig(a.httpEndpointURL)
	}
	log.Infof("acquiring new oauth2 token source: "+
		"http=%v grpc=%v config_url=%v "+
		"auth_url=%v token_url=%v jwks_url=%v api_url=%v signup_url=%v "+
		"client_id=%v scopes=%v "+
		"token=%v valid=%v",
		a.config.HTTPEndpointAddress, a.config.GRPCEndpointAddress,
		a.httpEndpointURL.JoinPath(auth.ServeConfigPath),
		conf.Endpoint.AuthURL, conf.Endpoint.TokenURL, conf.JWKSURL,
		conf.APIURL, conf.SignupURL,
		conf.ClientID, conf.Scopes,
		token != nil, token.Valid())

	if token != nil {
		// use component lifecycle ctx rather than this rpc's ctx
		// to ensure that the refresh process is not canceled incorrectly
		source := conf.TokenSource(a.ctx, token)
		return source, nil
	}

	if refreshOnly {
		// signal that this is an expected state so it is not logged as an error
		return nil, auth.ErrUnavailable
	}

	urlCh, isLogin := loginURLChFrom(ctx)
	if !isLogin {
		// Do not implicitly open a browser for background or incidental
		// callers. The user must explicitly invoke the login command,
		// which seeds the per-call URL channel into ctx.
		return nil, auth.ErrNotAuthenticated
	}

	log.Debugf("oauth2: waiting for oauth2 flow to complete")

	_, source, err := blueauth.NewClientWithPorts(ctx, conf.Config, func(rawURL string) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		urlCh <- u
		// The oauth2 client only starts reading the callback result
		// once this callback returns, so an opener that blocks for the
		// lifetime of the browser process would wedge the loopback
		// redirect handler and the browser tab would spin forever.
		go debug.CapturePanicReport(func() {
			if err := a.config.OpenBrowser(u); err != nil {
				// Keep the OAuth flow alive instead of aborting: the URL was
				// already published, so the user can copy it from the wait
				// prompt and complete sign-in in any browser. The local
				// callback server stays listening for the redirect.
				log.Warnf("oauth2: open browser failed, falling back to manual URL: %v", err)
			}
		})

		return nil
	}, tryPorts, blueauth.WithSuccessHTML(callbackPageHTML))
	if err != nil {
		return nil, fmt.Errorf("new oauth2 client: %w", err)
	}
	log.Debugf("oauth2: successfully generated token source")
	return source, nil
}
