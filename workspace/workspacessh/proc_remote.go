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

package workspacessh

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/bluectx"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

type procRemote struct {
	cmd  string
	args []string

	executor  schemeapi.Executor
	sessions  []*procSession
	ctx       context.Context
	cancelCtx func()
}

type procSession struct {
	sshCmd   string
	sshArgs  []string
	executor schemeapi.Executor
	ctx      context.Context
}

func newProcRemote(ctx context.Context, cfg sshConfig, uri workspaceapi.URI) (
	remote, error,
) {
	port := uri.Port()
	cmd := cfg.command
	if port != "" {
		if !strings.Contains(cfg.command, "%p") {
			return nil, fmt.Errorf("unable to set custom port: 'command' value is missing %%p")
		}
		cmd = strings.ReplaceAll(cfg.command, "%p", port)
	}

	host := uri.Hostname()
	if uri.User() != "" {
		host = fmt.Sprintf("%s@%s", uri.User(), host)
	}
	cmd = strings.ReplaceAll(cmd, "%h", host)

	args := strings.Split(cmd, " ")

	// make sure NewFileScheme will not return an error
	uri, err := workspaceapi.CurrentUserHostURI(".")
	if err != nil {
		return nil, fmt.Errorf("could not get current user host URI: %s", err)
	}

	// local because we are going to execute commands locally with command ssh sessions
	localExecutor, err := workspace.NewFileScheme(ctx, config.NopConfig(), uri)
	if err != nil {
		return nil, fmt.Errorf("could initialize local executor on URI %q: %s", uri.String(), err)
	}

	ret := &procRemote{executor: localExecutor, cmd: args[0], args: args[1:]}
	ret.ctx, ret.cancelCtx = context.WithCancel(context.Background())
	return ret, nil
}

func (m *procRemote) NewSession() (schemeapi.Executor, error) {
	ses := &procSession{
		sshCmd:   m.cmd,
		sshArgs:  m.args,
		executor: m.executor,
		ctx:      m.ctx,
	}

	m.sessions = append(m.sessions, ses)
	return ses, nil
}

func (m *procRemote) Close() (ret error) {
	m.cancelCtx()
	for _, ses := range m.sessions {
		if err := ses.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	m.sessions = nil

	if err := m.executor.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}

	return ret
}

func (m *procRemote) CommandString(name string, arg ...string) (string, []string) {
	ses := procSession{
		sshCmd:  m.cmd,
		sshArgs: m.args,
	}
	return ses.CommandString(name, arg...)
}

func (s *procSession) CommandString(name string, arg ...string) (string, []string) {
	arg = append(s.sshArgs, append([]string{name}, arg...)...)
	name = s.sshCmd
	return name, arg
}

func (s *procSession) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	if cmd.Path == "" {
		return 0, errors.New("no command")
	}

	cmd.Path, cmd.Args = s.CommandString(cmd.Path, cmd.Args...)
	var cancelFn func()
	ctx, cancelFn = context.WithCancel(ctx)
	cmd.Watcher = newWrapWatcher(cmd.Watcher, cancelFn)
	pid, err := s.executor.StartCommand(bluectx.First(s.ctx, ctx), cmd)
	if err != nil {
		return workspaceapi.Pid(0), fmt.Errorf("create ssh command: %s", err)
	}
	return pid, nil
}

func (s *procSession) Signal(pid workspaceapi.Pid, sig syscall.Signal) error {
	return s.executor.Signal(pid, sig)
}

func (s *procSession) Close() (ret error) {
	return ret
}
