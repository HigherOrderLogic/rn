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
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/ernestrc/logd-go/logging"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/auth"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/extensionapi"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/workspace"
)

var _ extension.Runner = (*workspaceRunner)(nil)

// workspaceRunner satisfies extension.Runner with a simple
// protocol that initially exchanges metadata and secrets
// over stdin/stdout and secures resources via TLS and
// per rpc authentication/authorization.
type workspaceRunner struct {
	cfg       runnerConfig
	workspace workspaceapi.URI
	dataDir   string
	grantor   extension.Grantor
	executor  schemeapi.Executor
	socket    string
	tlsCert   []byte
	keys      auth.Keys
	ctx       context.Context
	cancelCtx func()
	pids      sync.Map
}

func newWorkspaceRunner(
	executor schemeapi.Executor, grantor extension.Grantor,
	workspace workspaceapi.URI, socket, dataDir string,
	tlsCert []byte, keys auth.Keys, opts ...Option,
) *workspaceRunner {
	ret := new(workspaceRunner)
	ret.init(executor, grantor, workspace,
		socket, dataDir, tlsCert, keys, opts...)
	return ret
}

// Init initializes this workspaceRunner with the given grantor and options.
func (m *workspaceRunner) init(
	executor schemeapi.Executor, grantor extension.Grantor,
	workspace workspaceapi.URI, socket, dataDir string,
	tlsCert []byte, keys auth.Keys, opts ...Option,
) {
	m.ctx, m.cancelCtx = context.WithCancel(context.Background())
	for _, o := range opts {
		o(&m.cfg)
	}
	m.grantor = grantor
	m.keys = keys
	m.executor = executor
	m.socket = socket
	m.dataDir = dataDir
	m.workspace = workspace
	m.tlsCert = tlsCert
}

// Run runs the given extension with an executable at the given path,
// with the given config.
func (m *workspaceRunner) Run(name, path string, config config.Config) error {
	if name == "" || path == "" {
		return errors.New("extension name and path must not be empty")
	}
	extensionID := name

	ctx, cancel := context.WithCancel(m.ctx)
	cmd, err := m.makeCommand(ctx, extensionID, path, config)
	if err != nil {
		cancel()
		return fmt.Errorf("make command: %w", err)
	}

	pid, err := m.executor.StartCommand(ctx, cmd)
	if err != nil {
		cancel()
		return fmt.Errorf("start command: %w", err)
	}

	m.log(log.DebugLevel, "running extension with name %q at path %q, pid: %d",
		name, path, pid)

	m.pids.Store(extensionID, cancel)

	return nil
}

// Close stops all extensions and cleans up all resources associated
// with this workspaceRunner.
func (m *workspaceRunner) Close() error {
	m.cancelCtx() // kills all extensions
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
) (ret workspaceapi.Cmd, err error) {
	waitCh := make(chan error)
	go debug.CapturePanicReport(func() {
		select {
		case err := <-waitCh:
			if err != nil {
				m.log(log.ErrorLevel, "extension %s exit: %v", extensionID, err)
			}
		case <-m.ctx.Done():
		}
	})
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
		var err error
		ret.Dir, err = workspaceapi.ExpandPathWithURI(m.workspace.Path(), m.workspace)
		if err != nil {
			err = fmt.Errorf("could not expand workspace "+
				"path: %q: %w", m.workspace.Path(), err)
			return workspaceapi.Cmd{}, err
		}
	}

	ret.Env = append(ret.Env, makeLogLevelEnv(log.GetLevel()))
	ret.Stdin, ret.Stdout, ret.Stderr = m.makeProtocolExchange(extensionID, config)
	return
}

func (m *workspaceRunner) makeProtocolExchange(extensionID string, cfg config.Config) (
	io.Reader, io.Writer, io.Writer,
) {
	protocol := newProtocol(m.ctx, m.grantor, extensionID, m.socket,
		m.dataDir, m.tlsCert, m.cfg.insecureAuth, cfg, m.keys)
	collector := newCollector(extensionID, m.workspace)
	stdout := errIntercept{m: m, extensionID: extensionID, protocol: protocol}
	return protocol, stdout, collector
}

func (m *workspaceRunner) stopExtension(extensionID string, reason error) {
	m.log(log.WarnLevel, "stopping extension %q: reason: %v", extensionID, reason)
	cancelIfc, ok := m.pids.LoadAndDelete(extensionID)
	if !ok {
		return
	}
	cancelIfc.(context.CancelFunc)()
}

func makeLogLevelEnv(l log.Level) string {
	return fmt.Sprintf("%s=%s", extensionapi.EnvLogLevel, l)
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
	}
	return n, err
}
