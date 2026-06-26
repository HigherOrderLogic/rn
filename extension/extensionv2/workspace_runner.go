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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/logd-go/logging"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/ideauthorizer"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/processctx"
)

var _ extension.Runner = (*workspaceRunner)(nil)

// ErrExtensionAlreadyRunning is returned by Run when the extension with the
// given id is already running, so callers can treat a re-run as a no-op.
var ErrExtensionAlreadyRunning = errors.New("extension is already running")

// Runner satisfies extension.Runner with a simple
// protocol that initially exchanges metadata and secrets
// over stdin/stdout and secures resources via TLS and
// per rpc authentication/authorization.
type workspaceRunner struct {
	cfg       runnerConfig
	workspace workspaceapi.URI
	dataDir   string
	grantor   extension.Grantor
	// executor runs ad-hoc commands submitted via StartCommand. This
	// is the workspace's own executor (file://, ssh:// gRPC, etc.) so
	// callers like vte.Component can pass workspace-bound
	// stdin/stdout/stderr (pty fds, remote files, ...) and have them
	// routed correctly to the host where the workspace lives.
	executor schemeapi.Executor
	// extExecutor runs extension binaries that the IDE launches via
	// Run(). Extensions are user-owned local binaries that always
	// live on the IDE host, so we always run them through a local
	// fileScheme regardless of the workspace's scheme. Routing them
	// through a remote (ssh) executor breaks the "user owns
	// extensions" model and surfaces as "lost connection to remote"
	// when N extensions race to fork/exec over a single SSH channel.
	extExecutor schemeapi.Executor
	socket      string
	tlsCert     []byte
	keys        auth.Keys
	ctx         context.Context
	cancelCtx   func()
	mu          sync.Mutex
	// registered broadcasts whenever Run installs or replaces a
	// state. WaitReady uses it to block for an extension that has not
	// been registered yet, since Run executes asynchronously from
	// initExtensions.
	registered *sync.Cond
	states     map[string]*extensionRunState
}

type extensionRunState struct {
	id         string
	cmdAndArgs string
	config     config.Config
	cancel     context.CancelFunc
	pid        workspaceapi.Pid
	started    time.Time
	running    bool
	lastErr    error
	startCount int
	readiness  *extensionReadiness
	logPath    string
}

type extensionRunStateSnapshot struct {
	ID         string
	CmdAndArgs string
	Config     config.Config
	Pid        workspaceapi.Pid
	Started    time.Time
	Running    bool
	LastErr    error
	StartCount int
	LogPath    string
}

var _ schemeapi.Executor = (*workspaceRunner)(nil)
var _ extension.Runner = (*workspaceRunner)(nil)

func newWorkspaceRunner(
	executor, extExecutor schemeapi.Executor, grantor extension.Grantor,
	workspace workspaceapi.URI, socket, dataDir string,
	tlsCert []byte, keys auth.Keys, opts ...Option,
) *workspaceRunner {
	ret := new(workspaceRunner)
	ret.init(executor, extExecutor, grantor, workspace,
		socket, dataDir, tlsCert, keys, opts...)
	return ret
}

// Init initializes this Runner with the given grantor and options.
func (m *workspaceRunner) init(
	executor, extExecutor schemeapi.Executor, grantor extension.Grantor,
	workspace workspaceapi.URI, socket, dataDir string,
	tlsCert []byte, keys auth.Keys, opts ...Option,
) {
	m.ctx, m.cancelCtx = context.WithCancel(context.Background())
	m.states = make(map[string]*extensionRunState)
	m.registered = sync.NewCond(&m.mu)
	m.cfg.authCertEnv = "RUNE_CERT"
	m.cfg.authTokenEnv = "RUNE_TOKEN"
	m.cfg.socketEnv = "RUNE_SOCKET"
	m.cfg.dataDirEnv = "RUNE_DATADIR"
	for _, o := range opts {
		o(&m.cfg)
	}
	m.grantor = grantor
	m.keys = keys
	m.executor = executor
	if extExecutor == nil {
		panic("extensionv2: extExecutor must not be nil")
	}
	m.extExecutor = extExecutor
	m.socket = socket
	m.dataDir = dataDir
	m.workspace = workspace
	m.tlsCert = tlsCert
}

// StartCommand runs an ad-hoc program and authorizes it to access resources.
func (m *workspaceRunner) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	env, err := m.commandEnvs(ctx, cmd.Path, cmd.Args)
	if err != nil {
		return 0, err
	}
	cmd.Env = append(cmd.Env, env...)
	return m.executor.StartCommand(ctx, cmd)
}

// Signal sends a signal to the running process.
func (m *workspaceRunner) Signal(pid workspaceapi.Pid, sig syscall.Signal) error {
	return m.executor.Signal(pid, sig)
}

// Run runs the given extension with an executable at the given path,
// with the given config.
func (m *workspaceRunner) Run(id, cmdAndArgs string, config config.Config) error {
	if id == "" || cmdAndArgs == "" {
		return errors.New("extension id and cmd must not be empty")
	}

	m.mu.Lock()
	if state := m.states[id]; state != nil && state.running {
		m.mu.Unlock()
		return fmt.Errorf("extension %q: %w", id, ErrExtensionAlreadyRunning)
	}
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(m.ctx)
	ctx = processctx.ContextWithExtensionID(ctx, id)
	readiness := newExtensionReadiness()
	logPath := extensionLogPath(m.workspace, id)
	cmd, err := m.makeCommand(ctx, id, cmdAndArgs, config, readiness, logPath)
	if err != nil {
		cancel()
		return fmt.Errorf("make command: %w", err)
	}

	pid, err := m.extExecutor.StartCommand(ctx, cmd)
	if err != nil {
		cancel()
		return fmt.Errorf("start command: %w", err)
	}

	m.log(log.DebugLevel, "running extension with name %q at path %q, pid: %d",
		id, cmdAndArgs, pid)

	m.mu.Lock()
	state := m.states[id]
	if state == nil {
		state = &extensionRunState{id: id}
		m.states[id] = state
	}
	state.id = id
	state.cmdAndArgs = cmdAndArgs
	state.config = config
	state.cancel = cancel
	state.pid = pid
	state.started = time.Now()
	state.running = true
	state.lastErr = nil
	state.startCount++
	state.readiness = readiness
	state.logPath = logPath
	m.registered.Broadcast()
	m.mu.Unlock()

	return nil
}

// Close stops all extensions and cleans up all resources associated
// with this Runner.
func (m *workspaceRunner) Close() error {
	// Cancelling the runner context terminates each extension
	// process; the per-extension waitCh goroutine fires
	// setExtensionExit which closes the collector (and its log file)
	// after exec.Cmd.Wait returns. We only need to drop the on-disk
	// log files here; the OS would clean them on reboot but doing it
	// eagerly keeps /tmp tidy.
	m.cancelCtx()
	m.mu.Lock()
	m.registered.Broadcast()
	paths := make([]string, 0, len(m.states))
	for _, state := range m.states {
		paths = append(paths, state.logPath)
	}
	m.mu.Unlock()
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return nil
}

func (m *workspaceRunner) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "extensionv2.workspaceRunner",
		"workspace":      m.workspace.String(),
	}).Logf(level, msg, args...)
}

func (m *workspaceRunner) makeCommand(
	ctx context.Context, extensionID, path string, config config.Config,
	readiness *extensionReadiness, logPath string,
) (ret workspaceapi.Cmd, err error) {
	waitCh := make(chan error)
	// allow args to be passed to extensions
	argv := strings.Split(path, " ")
	ret = workspaceapi.Cmd{
		Path:    argv[0],
		Args:    argv[1:],
		Dir:     m.dataDir, // default
		Watcher: workspaceapi.ChanProcessWatcher(waitCh),
	}

	// if local workspace, then do set dir in a best effort for
	// extensions that do not use APIs and call os functions directly.
	if m.workspace.Scheme() == workspace.FileScheme {
		var werr error
		ret.Dir, werr = workspaceapi.ExpandPathWithURI(m.workspace.Path(), m.workspace)
		if werr != nil {
			err = fmt.Errorf("could not expand workspace "+
				"path: %q: %w", m.workspace.Path(), werr)
			return workspaceapi.Cmd{}, err
		}
	}

	ret.Env, err = m.commandEnvs(ctx, ret.Path, ret.Args)
	if err != nil {
		return workspaceapi.Cmd{}, err
	}
	stdin, stdout, stderr, collector, err := m.makeProtocolExchange(
		extensionID, config, readiness, logPath)
	if err != nil {
		return workspaceapi.Cmd{}, err
	}
	ret.Stdin, ret.Stdout, ret.Stderr = stdin, stdout, stderr
	// The watcher goroutine owns the collector's lifecycle: it must
	// close it only after exec.Cmd.Wait has returned (signalled by
	// waitCh), which is the one moment we know the stderr-copy
	// goroutine has stopped writing. Closing from any other
	// goroutine (stopExtension, runner Close, m.ctx.Done) would race
	// with that writer. If m.ctx is cancelled first we don't close
	// the collector at all; the log file lives in os.TempDir and is
	// unlinked from workspaceRunner.Close, and the FD is released on
	// process exit.
	go debug.CapturePanicReport(func() {
		select {
		case exitErr := <-waitCh:
			_ = collector.Close()
			m.setExtensionExit(extensionID, exitErr)
			if exitErr != nil {
				m.log(log.ErrorLevel, "extension %s exit: %v", extensionID, exitErr)
			}
		case <-m.ctx.Done():
		}
	})
	return
}

func (m *workspaceRunner) commandEnvs(ctx context.Context, path string, args []string) ([]string, error) {
	// TODO expire token manually when program finishes
	const tokenExpiresIn = 24 * 365 * time.Hour

	env := []string{makeLogLevelEnv(log.GetLevel())}
	env = append(env, fmt.Sprintf("%s=%s", m.cfg.socketEnv, m.socket))
	env = append(env, fmt.Sprintf("%s=%s", m.cfg.dataDirEnv, m.dataDir))
	cert := base64.StdEncoding.EncodeToString(m.tlsCert)
	env = append(env, fmt.Sprintf("%s=%s", m.cfg.authCertEnv, cert))
	if m.cfg.insecureAuth {
		return env, nil
	}

	signKey, err := m.keys.Sign(ctx)
	if err != nil {
		return nil, fmt.Errorf("get sign key: %w", err)
	}
	name := fmt.Sprintf("%s_%s", path, strings.Join(args, "_"))
	id := uuid.New().String()
	if extensionID, ok := processctx.ExtensionIDFromContext(ctx); ok {
		id = extensionID
	}
	permissions := extensionapi.AllPermissions()
	m.log(log.DebugLevel, "creating one shot authentication for "+
		"command %s, id: %s, permissions: %v", name, id, permissions)
	claimsExtra := ideauthorizer.Extension{
		Metadata: extensionapi.Metadata{
			DeveloperID:   "you",
			DeveloperKey:  "",
			ExtensionID:   id,
			ExtensionName: name,
			Permissions:   permissions,
		},
		Plugin: true,
		Path:   path,
		Args:   append([]string(nil), args...),
	}
	accessToken, err := auth.SignToken(signKey,
		claimsExtra.DeveloperID, claimsExtra.DeveloperEmail, claimsExtra,
		tokenExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	env = append(env, fmt.Sprintf("%s=%s", m.cfg.authTokenEnv, accessToken))
	return env, nil
}

func (m *workspaceRunner) makeProtocolExchange(
	extensionID string, cfg config.Config, readiness *extensionReadiness, logPath string,
) (
	io.Reader, io.Writer, io.Writer, *logCollector, error,
) {
	protocol := newProtocol(m.ctx, m.grantor, extensionID, m.socket,
		m.dataDir, m.tlsCert, m.cfg.insecureAuth, cfg, m.keys, readiness)
	logFile, err := os.OpenFile(logPath,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf(
			"open extension %q log file %q: %w", extensionID, logPath, err)
	}
	collector := newCollector(extensionID, m.workspace, logFile)
	stdout := errIntercept{
		m:           m,
		extensionID: extensionID,
		protocol:    protocol,
	}
	return protocol, stdout, collector, collector, nil
}

func (m *workspaceRunner) setExtensionExit(extensionID string, err error) {
	m.mu.Lock()
	state := m.states[extensionID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	state.running = false
	state.pid = 0
	state.cancel = nil
	state.lastErr = err
	readiness := state.readiness
	m.mu.Unlock()
	readiness.Set(err)
}

func (m *workspaceRunner) listExtensions() []extensionRunStateSnapshot {
	m.mu.Lock()
	ret := make([]extensionRunStateSnapshot, 0, len(m.states))
	for _, state := range m.states {
		ret = append(ret, extensionRunStateSnapshot{
			ID:         state.id,
			CmdAndArgs: state.cmdAndArgs,
			Config:     state.config,
			Pid:        state.pid,
			Started:    state.started,
			Running:    state.running,
			LastErr:    state.lastErr,
			StartCount: state.startCount,
			LogPath:    state.logPath,
		})
	}
	m.mu.Unlock()
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].ID < ret[j].ID
	})
	return ret
}

func (m *workspaceRunner) extension(id string) (extensionRunStateSnapshot, bool) {
	m.mu.Lock()
	state := m.states[id]
	m.mu.Unlock()
	if state == nil {
		return extensionRunStateSnapshot{}, false
	}
	return extensionRunStateSnapshot{
		ID:         state.id,
		CmdAndArgs: state.cmdAndArgs,
		Config:     state.config,
		Pid:        state.pid,
		Started:    state.started,
		Running:    state.running,
		LastErr:    state.lastErr,
		StartCount: state.startCount,
		LogPath:    state.logPath,
	}, true
}

func (m *workspaceRunner) startExtension(
	ctx context.Context, id, cmdAndArgs string, cfg config.Config,
) error {
	if err := m.Run(id, cmdAndArgs, cfg); err != nil {
		return err
	}
	// Run just stored a fresh readiness in the state. Capture it and wait
	// for the protocol handshake (or a stop/exit) to resolve it.
	m.mu.Lock()
	readiness := m.states[id].readiness
	m.mu.Unlock()
	return readiness.Wait(ctx)
}

// WaitReady blocks until the extension with this id has been registered
// (Run called) and completed its protocol handshake, failed/exited, the
// runner is closed, or ctx is cancelled. Registration happens
// asynchronously, so an id that is not yet known is not an error: the
// call waits for it to appear rather than failing immediately.
func (m *workspaceRunner) WaitReady(ctx context.Context, id string) error {
	stop := context.AfterFunc(ctx, func() {
		m.mu.Lock()
		m.registered.Broadcast()
		m.mu.Unlock()
	})
	defer stop()

	m.mu.Lock()
	for m.states[id] == nil && ctx.Err() == nil && m.ctx.Err() == nil {
		m.registered.Wait()
	}
	state := m.states[id]
	m.mu.Unlock()
	if state == nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("extension %q runner is closed: %w", id, m.ctx.Err())
	}
	return state.readiness.Wait(ctx)
}

func (m *workspaceRunner) stopExtensionByID(id string) error {
	m.mu.Lock()
	state := m.states[id]
	m.mu.Unlock()
	if state == nil {
		return fmt.Errorf("extension %q not found", id)
	}
	if !state.running {
		return fmt.Errorf("extension %q is not running", id)
	}
	m.stopExtension(id, nil)
	return nil
}

func (m *workspaceRunner) restartExtension(ctx context.Context, id string) error {
	m.mu.Lock()
	state := m.states[id]
	if state == nil {
		m.mu.Unlock()
		return fmt.Errorf("extension %q not found", id)
	}
	cmdAndArgs := state.cmdAndArgs
	cfg := state.config
	running := state.running
	m.mu.Unlock()
	if cmdAndArgs == "" {
		return fmt.Errorf("extension %q has no stored command", id)
	}
	if running {
		if err := m.stopExtensionByID(id); err != nil {
			return err
		}
	}
	return m.startExtension(ctx, id, cmdAndArgs, cfg)
}

func (m *workspaceRunner) stopExtension(extensionID string, reason error) {
	m.log(log.WarnLevel, "stopping extension %q: reason: %v", extensionID, reason)
	m.mu.Lock()
	state := m.states[extensionID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	cancel := state.cancel
	state.cancel = nil
	state.running = false
	state.pid = 0
	state.lastErr = reason
	readiness := state.readiness
	m.mu.Unlock()
	// Cancelling triggers the watcher goroutine spawned in
	// makeCommand, which closes the collector after the stderr-copy
	// goroutine completes. Doing it here would race with that writer.
	readiness.Set(reason)
	if cancel != nil {
		cancel()
	}
}

func makeLogLevelEnv(l log.Level) string {
	return fmt.Sprintf("%s=%s", extensionapi.EnvLogLevel, l)
}

// extensionLogPath returns the per-extension stderr log file path
// inside the OS temp directory. The filename is derived from a SHA-256
// hash of the workspace URI and the extension id so that:
//   - the same (workspace, id) pair maps to the same path, and
//   - different workspaces (or ids) get distinct files.
func extensionLogPath(ws workspaceapi.URI, extensionID string) string {
	sum := sha256.Sum256([]byte(ws.String() + "\x00" + extensionID))
	return filepath.Join(os.TempDir(),
		fmt.Sprintf("rune-extension-%s.log", hex.EncodeToString(sum[:])[:16]))
}

type errIntercept struct {
	m           *workspaceRunner
	protocol    io.Writer
	extensionID string
}

func (e errIntercept) Write(data []byte) (int, error) {
	n, err := e.protocol.Write(data)
	if err != nil {
		e.m.stopExtension(e.extensionID, err)
		return n, err
	}
	return n, err
}
