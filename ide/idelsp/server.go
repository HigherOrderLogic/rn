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

package idelsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/semanticapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/ide/idelsp/jsonrpc2"
	"unstable.build/go-tui/workspace/processctx"
)

// server is the routing-facing contract the Manager uses to talk
// to a language backend. A backend is either a single langServer
// or a multiLangServer that fans calls out to several langServers
// sharing one language id (e.g. ty + ruff for python). call and
// notify pick the appropriate child by LSP method; the remaining
// methods drive lifecycle and report state.
type server interface {
	io.Closer
	call(ctx context.Context, method string, params, result any) error
	notify(ctx context.Context, method string, params any) error
	initialize(ctx context.Context) (semanticapi.InitializeResult, error)
	stop(ctx context.Context) error
	start(ctx context.Context) error
	config() langConfig
	initResult() semanticapi.InitializeResult
	isAlive() bool
}

var _ server = (*langServer)(nil)

type langServer struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     func()
	stopCalled bool
	params     semanticapi.InitializeParams
	cfg        langConfig
	serverName string
	binPath    string
	pid        workspaceapi.Pid
	watcher    chan error
	rootURI    string
	executor   schemeapi.Executor
	handler    jsonrpc2.Handler
	stdin      net.Conn
	stdout     net.Conn
	conn       *jsonrpc2.Connection
	alive      bool
	log        *slog.Logger
	init       semanticapi.InitializeResult
}

// pipeCloser wraps read and write pipes to close them together.
type pipeCloser struct {
	r net.Conn
	w net.Conn
}

func (pc pipeCloser) Close() error {
	slog.Debug("closing pipes")
	_ = pc.w.Close()
	return pc.r.Close()
}

// deadlineWriter wraps a jsonrpc2.Writer to apply a per-write deadline
// derived from the call context onto the underlying net.Conn. This
// prevents Write from blocking indefinitely if the kernel write buffer
// fills (e.g. peer LSP stalled). Because jsonrpc2.Connection serializes
// writes through a 1-buffered channel, the deadline pokes here cannot
// overlap with another writer's deadline. Crucially, only the writer
// FD is touched — the reader FD is never given a per-RPC deadline,
// so a slow caller cannot kill the long-lived readIncoming goroutine.
type deadlineWriter struct {
	inner jsonrpc2.Writer
	conn  net.Conn
}

func (w *deadlineWriter) Write(ctx context.Context, msg jsonrpc2.Message) error {
	if deadline, ok := ctx.Deadline(); ok {
		if err := w.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
		defer func() { _ = w.conn.SetWriteDeadline(time.Time{}) }()
	}
	return w.inner.Write(ctx, msg)
}

func newLangServer(
	ctx context.Context,
	cfg langConfig,
	binPath string,
	executor schemeapi.Executor,
	rootURI string,
	handler jsonrpc2.Handler,
	params semanticapi.InitializeParams,
) *langServer {
	ctx, cancel := context.WithCancel(ctx)
	return &langServer{
		params:     params,
		ctx:        ctx,
		cancel:     cancel,
		cfg:        cfg,
		serverName: cfg.id,
		binPath:    binPath,
		executor:   executor,
		rootURI:    rootURI,
		handler:    handler,
		log: slog.With("struct", "idelsp.langServer",
			"language", cfg.id, "workspace", rootURI),
	}
}

func (s *langServer) start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("create socket pair: %v", err)
	}
	// One os.File per fd to avoid GC finalizer races that can
	// close reused file descriptors in other goroutines.
	lspFile := os.NewFile(uintptr(fds[1]), "file+net lsp")
	ideFile := os.NewFile(uintptr(fds[0]), "file+net ide")

	watchCh := make(chan error, 1)
	watcher := workspaceapi.ChanProcessWatcher(watchCh)

	cmd := workspaceapi.Cmd{
		Path:    s.binPath,
		Args:    s.cfg.args,
		Stdin:   lspFile,
		Stdout:  lspFile,
		Watcher: watcher,
	}

	// Do not use ctx for lifecycle cancellation: it is scoped to the initial
	// protocol exchange. Copy logical process metadata from ctx into the server
	// lifecycle context so LSP server processes remain associated with the
	// extension that requested them.
	lifecycleContext := processctx.DeriveCommandContext(s.ctx, ctx)

	s.log.Info("starting server", "path", cmd.Path, "args", cmd.Args)
	pid, err := s.executor.StartCommand(lifecycleContext, cmd)
	if err != nil {
		_ = lspFile.Close()
		_ = ideFile.Close()
		return fmt.Errorf(
			"start %s: %w", s.cfg.command, err,
		)
	}
	// StartCommand duped the fd; close our copy.
	_ = lspFile.Close()

	stdout, err := net.FileConn(ideFile)
	if err != nil {
		_ = ideFile.Close()
		return fmt.Errorf("new stdout file conn: %w", err)
	}
	stdin, err := net.FileConn(ideFile)
	if err != nil {
		_ = stdout.Close()
		_ = ideFile.Close()
		return fmt.Errorf("new stdin file conn: %w", err)
	}
	// FileConn duped the fd; close our copy.
	_ = ideFile.Close()
	s.pid = pid
	s.watcher = watchCh
	framer := jsonrpc2.HeaderFramer()
	closer := pipeCloser{r: stdout, w: stdin}
	s.conn = jsonrpc2.NewConnection(lifecycleContext, jsonrpc2.ConnectionConfig{
		Reader: framer.Reader(stdout),
		Writer: &deadlineWriter{inner: framer.Writer(stdin), conn: stdin},
		Closer: closer,
		Bind:   func(*jsonrpc2.Connection) jsonrpc2.Handler { return s.handler },
	})
	s.alive = true
	s.stdin = stdin
	s.stdout = stdout

	resp, err := s.initialize(ctx)
	if err != nil {
		_ = s.Close()
		return fmt.Errorf("initialize %s: %w", s.cfg.command, err)
	}
	s.init = resp
	return nil
}

func (s *langServer) Close() error {
	// Cancel s.ctx so exec.CommandContext kills the child gopls process;
	// without this, a Close before watchServer takes over (e.g. failed
	// initialize) leaves an orphan subprocess.
	if s.cancel != nil {
		s.cancel()
	}
	// NOTE: if we don't close this first, there's a risk that
	// conn.Close blocks before because it's calling conn.Wait.
	_ = s.stdin.Close()
	_ = s.stdout.Close()
	return s.conn.Close()
}

func (s *langServer) initialize(ctx context.Context) (
	response semanticapi.InitializeResult, err error,
) {

	s.log.Debug("rpc call", "method", "initialize")
	err = s.conn.Call(ctx, "initialize", s.params).Await(ctx, &response)
	if err != nil {
		return
	}

	err = s.conn.Notify(ctx, "initialized", struct{}{})
	return
}

func (s *langServer) stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopCalled = true
	if !s.alive {
		s.mu.Unlock()
		return nil
	}
	s.alive = false
	s.mu.Unlock()

	var raw json.RawMessage
	err := s.conn.Call(ctx, "shutdown", nil).Await(ctx, &raw)
	if err != nil {
		return fmt.Errorf("call shutdown: %w", err)
	}
	err = s.conn.Notify(ctx, "exit", nil)
	if err != nil {
		return fmt.Errorf("notify exit: %w", err)
	}
	return nil
}

func (s *langServer) call(
	ctx context.Context, method string,
	params, result any,
) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		s.log.Warn("rpc call", "method", method, "error", "no server")
		return ErrNoServer
	}
	conn := s.conn
	s.mu.Unlock()
	s.log.Debug("rpc call", "method", method, "step", "attempt")

	call := conn.Call(ctx, method, params)
	err := call.Await(ctx, result)
	if err != nil {
		s.log.Warn("rpc call", "method", method, "error", err)
	} else {
		s.log.Debug("rpc call", "method", method, "step", "success")
	}
	return err
}

func (s *langServer) notify(
	ctx context.Context, method string, params any,
) error {
	s.mu.Lock()
	if !s.alive {
		s.mu.Unlock()
		s.log.Warn("rpc notify", "method", method, "error", "no server")
		return ErrNoServer
	}
	conn := s.conn
	s.mu.Unlock()

	s.log.Debug("rpc notify", "method", method, "step", "attempt")
	err := conn.Notify(ctx, method, params)
	if err != nil {
		s.log.Warn("rpc notify", "method", method, "step", "error", "error", err)
	} else {
		s.log.Debug("rpc notify", "method", method, "step", "success")
	}
	return err
}

func (s *langServer) config() langConfig {
	return s.cfg
}

func (s *langServer) initResult() semanticapi.InitializeResult {
	return s.init
}

func (s *langServer) isAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alive
}
