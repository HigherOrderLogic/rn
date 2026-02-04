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

package extensionv2

import (
	"context"
	"crypto/tls"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/blue/auth/grpcauth"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/workspace"
)

// NewRunner returns an ide.ExtensionsRunner with a simple protocol that
// initially exchanges metadata and secrets over stdin/stdout and secures
// resources via TLS and per rpc authentication/authorization.
func NewRunner(
	ctx context.Context, locker sync.Locker,
	grantor extension.Grantor, dataDir string, opts ...Option,
) (ide.ExtensionsRunner, error) {
	dataDirURI, err := workspaceapi.CurrentUserHostURI(dataDir)
	if err != nil {
		return nil, fmt.Errorf("get data dir uri: %w", err)
	}
	executor, err := workspace.NewFileScheme(ctx, config.NopConfig(), dataDirURI)
	if err != nil {
		return nil, fmt.Errorf("new file scheme: %w", err)
	}
	authorizer := newAuthorizer()
	ret := &runner{
		authorizer: authorizer,
		locker:     locker,
		grantor:    grantor,
		dataDir:    dataDir,
		executor:   executor,
		opts:       opts,
	}
	ret.keys, err = auth.GenerateKeys()
	if err != nil {
		return nil, fmt.Errorf("generate keys: %w", err)
	}
	for _, o := range opts {
		o(&ret.cfg)
	}
	return ret, nil
}

const certExpiresIn = 10 * 24 * 365 * time.Hour

type runner struct {
	opts       []Option
	cfg        runnerConfig
	locker     sync.Locker
	keys       auth.Keys
	authorizer auth.Authorizer[Extension]
	grantor    extension.Grantor
	dataDir    string
	executor   schemeapi.Executor
}

func (r *runner) WorkspaceExtensionsRunner(
	uri workspaceapi.URI, res map[extensionapi.Permission]extension.ResourceRegistrar,
	dataDir string, notifications browser.Notifications,
) (extension.Runner, error) {
	var ret wrapCloser
	ret.URI = uri

	listener, err := r.newUnixListener(uri)
	if err != nil {
		return nil, fmt.Errorf("create unix listener: %w", err)
	}
	socket := listener.Addr().String()

	streamInterceptors := []grpc.StreamServerInterceptor{
		rpc.StreamReportRecoveryInterceptor(),
	}
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		rpc.UnaryReportRecoveryInterceptor(),
	}
	if log.IsLevelEnabled(log.DebugLevel) {
		fields := []logging.Field{
			{Key: logging.KeyClass, Value: "grpc.Server"},
			{Key: "workspace", Value: uri.String()},
		}
		streamInterceptors = append(streamInterceptors, rpc.StreamLoggingInterceptor(fields))
		unaryInterceptors = append(unaryInterceptors, rpc.UnaryLoggingInterceptor(fields))
	}
	opts := []grpc.ServerOption{
		grpc.ChainStreamInterceptor(streamInterceptors...),
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
	}
	var cert, key []byte
	if r.cfg.insecureTransport && !r.cfg.insecureAuth {
		opts = append(opts, grpcauth.GRPCServerWithInsecureOauth2(r.keys, r.authorizer)...)
	} else if !r.cfg.insecureTransport {
		cert, key, err = auth.GenerateSelfSignedCert(
			[]string{socket}, pkix.Name{CommonName: "ox"}, certExpiresIn)
		if err != nil {
			if cerr := listener.Close(); cerr != nil {
				err = multierror.Append(err, cerr)
			}
			return nil, fmt.Errorf("new extension runner: %v", err)
		}
		tlsCert, err := tls.X509KeyPair(cert, key)
		if err != nil {
			if cerr := listener.Close(); cerr != nil {
				err = multierror.Append(err, cerr)
			}
			return nil, fmt.Errorf("load tls credentials from cert and key: %w", err)
		}
		cfg := tls.Config{
			Certificates:       []tls.Certificate{tlsCert},
			InsecureSkipVerify: true,
		}
		creds := credentials.NewTLS(&cfg)
		if r.cfg.insecureAuth {
			opts = append(opts, grpc.Creds(creds))
		} else {
			opts = append(opts, grpcauth.GRPCServerWithOauth2(r.keys, r.authorizer, creds)...)
		}
	}
	ret.srv = grpc.NewServer(opts...)
	for _, registrar := range res {
		closer, rerr := registrar.Register(ret.srv, r.locker)
		if rerr != nil {
			err = multierror.Append(err, rerr)
			continue
		}
		ret.closers = append(ret.closers, closer)
	}
	if err != nil {
		if cerr := ret.Close(); cerr != nil {
			err = multierror.Append(err, cerr)
			return nil, err
		}
	}

	go debug.CapturePanicReport(func() {
		_ = ret.srv.Serve(listener)
	})

	ret.Runner = newWorkspaceRunner(r.executor, r.grantor, uri,
		socket, r.dataDir, cert, r.keys, r.opts...)
	if err != nil {
		err = fmt.Errorf("new workspace runner: %w", err)
		if cerr := ret.Close(); cerr != nil {
			err = multierror.Append(err, cerr)
			return nil, err
		}
		return nil, err
	}
	return ret, nil
}

func (r *runner) newUnixListener(uri workspaceapi.URI) (ret net.Listener, err error) {
	ctx := context.Background()
	socket := path.Join(uri.Path(), fmt.Sprintf(".%s.sock", r.cfg.pkg))
	err = retry.Retry(ctx, retrySocketStrategy, func(context.Context) (bool, error) {
		var cfg net.ListenConfig
		ret, err = cfg.Listen(ctx, "unix", socket)
		if err == nil {
			return false, nil
		}

		if errors.Is(err, syscall.EACCES) {
			return false, err
		}

		if errors.Is(err, syscall.ENOENT) { // a component of the path does not exist
			mkdirErr := os.MkdirAll(filepath.Dir(socket), 0766)
			if mkdirErr != nil {
				err = fmt.Errorf("listen: %w", err)
				return false, multierror.Append(err, mkdirErr)
			}
			return true, err
		}

		_ = os.Remove(socket)
		return true, err
	})
	return
}

type wrapCloser struct {
	workspaceapi.URI
	extension.Runner
	closers []io.Closer
	srv     *grpc.Server
}

func (w wrapCloser) Close() (ret error) {
	log.Tracef("closing workspace %q extensions", w.URI.String())
	for _, closer := range w.closers {
		if err := closer.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if w.Runner != nil {
		if err := w.Runner.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if w.srv != nil {
		w.srv.Stop() // stop closes listener
	}
	return
}

var retrySocketStrategy = retry.CombinedStrategy(
	retry.LimitStrategy(4),
	retry.ExponentialStrategy(time.Millisecond, time.Second),
)
