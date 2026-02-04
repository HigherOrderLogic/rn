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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi/workspacerpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"unstable.build/go-tui/debug"
)

const (
	// Scheme represents the URL scheme that this package implements
	Scheme = "ssh"

	debugPathError = "You can run %q to troubleshoot this. Also, double check your workspace.ssh.command " +
		"and workspace.ssh.shell configuration, if you have any."
)

// New returns a schemeapi.Scheme capable of managing
// files over an ssh connection.
func New(
	ctx context.Context, cfg config.Config, uri workspaceapi.URI,
) (schemeapi.Scheme, error) {
	return newScheme(ctx, cfg, uri)
}

type remote interface {
	NewSession() (schemeapi.Executor, error)
	Close() error
}

type scheme struct {
	cfg      sshConfig
	hostPort string
	user     string
	homedir  string
	basePath string

	getUser         func() (*user.User, error)
	remoteFn        func(context.Context, sshConfig, workspaceapi.URI) (remote, error)
	connectSchemeFn connectSchemeFn
	ctx             context.Context
	cancelCtx       func()

	schemeapi.Scheme
}

func newScheme(
	ctx context.Context, ccfg config.Config, uri workspaceapi.URI,
) (*scheme, error) {
	ret := new(scheme)
	ret.ctx, ret.cancelCtx = context.WithCancel(context.Background())

	cc, err := fromConfig(ccfg)
	if err != nil {
		return nil, err
	}

	ret.getUser = user.Current
	if cc.command == "" {
		ret.remoteFn = newStdRemote
	} else {
		ret.remoteFn = newProcRemote
	}

	ret.connectSchemeFn = ret.connectScheme
	err = ret.init(ctx, cc, uri, (schemeapi.Scheme).Stat)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func parseWorkspaceURI(u workspaceapi.URI, getUser func() (*user.User, error)) (
	username, homedir, hostPort, basePath string, err error,
) {
	username = u.User()

	// respect empty username for URI creation
	// but we need its implicit value for ~ expansion to work
	usernameForHomeDir := username
	if usernameForHomeDir == "" {
		var u *user.User
		u, err = getUser()
		if err != nil {
			return
		}
		usernameForHomeDir = u.Username
	}
	// I doubt we'll ever ssh into a non-linux host
	homedir = filepath.Join("/", "home", usernameForHomeDir)

	basePath, err = workspaceapi.ExpandPath(u.Path(), func() (*user.User, error) {
		return &user.User{Username: username, HomeDir: homedir}, nil
	}, func() (string, error) {
		// return host's base path, but this should never happen
		return "/", nil
	})
	if err != nil {
		return
	}

	hostPort = u.Host()
	if hostPort == "" {
		err = errors.New("ssh scheme with empty host is invalid")
		return
	}

	return
}

func (s *scheme) runAndWait(
	ctx context.Context,
	remote remote, cmdStr string, args ...string,
) (string, bool, error) {
	ses, err := remote.NewSession()
	if err != nil {
		return "", false, fmt.Errorf("new session: %v", err)
	}

	if s.cfg.shell != "" {
		args = append([]string{"-c", cmdStr}, args...)
		cmdStr = s.cfg.shell
	}
	var stderr, stdout bytes.Buffer
	ch := make(chan error)
	cmd := workspaceapi.Cmd{
		Path:    cmdStr,
		Args:    args,
		Stderr:  &stderr,
		Stdout:  &stdout,
		Watcher: workspaceapi.ChanProcessWatcher(ch),
	}
	_, err = ses.StartCommand(s.ctx, cmd)
	if err != nil {
		return "", false, fmt.Errorf("start command: %v", err)
	}

	err = <-ch
	if err != nil {
		if stdout.Len() != 0 {
			err = fmt.Errorf("stdout: %v: %s", err, stdout.String())
		}
		if stderr.Len() != 0 {
			err = fmt.Errorf("stderr: %v: %s", err, stderr.String())
		}
		if procRemote, ok := remote.(*procRemote); ok {
			cmd, args := procRemote.CommandString(cmdStr, args...)
			return fmt.Sprintf("%s %s", cmd, strings.Join(args, " ")), false, err
		}
		return "", false, err
	}

	err = ses.Close()
	// stdlib ssh session returns io.EOF if closing after command returned
	if err != nil && err != io.EOF {
		err = fmt.Errorf("close session: %v", err)
		return "", false, err
	}
	return "", true, nil
}

func (s *scheme) whichCommand(ctx context.Context, remote remote, cmd string) error {
	cmdAndArgs, avail, err := s.runAndWait(ctx, remote, "which", cmd)
	if err != nil {
		err = fmt.Errorf("could not check if %s executable is in PATH: %w", cmd, err)
	}
	if !avail {
		errStr := "%q executable was not found on remote. " +
			"Make sure it's installed and available via $PATH to a non-interactive shell. "
		if err != nil {
			errStr = fmt.Sprintf("%s %v. ", errStr, err)
		}
		if cmdAndArgs != "" {
			return fmt.Errorf(errStr+debugPathError, cmd, cmdAndArgs)
		}
		return fmt.Errorf(errStr, cmd)
	}
	return nil
}

func (s *scheme) workspaceExists(
	ctx context.Context, remote remote, uri workspaceapi.URI,
) error {
	cmdAndArgs, ok, err := s.runAndWait(ctx, remote, "ls", uri.Path())
	if err != nil {
		return fmt.Errorf("could not check if workspace path %q exists: %w", uri.Path(), err)
	}
	if !ok {
		return fmt.Errorf("path %q was not found on remote "+ //nolint:staticcheck
			debugPathError, uri.Path(), cmdAndArgs)
	}
	return nil
}

func (s *scheme) connectScheme(
	ctx context.Context, uri workspaceapi.URI, closeHook func(error),
) (schemeapi.Scheme, error) {
	const six = "six"

	sshPath := s.basePath
	if sshPath == "" {
		sshPath = "."
	}

	remote, err := s.remoteFn(ctx, s.cfg, uri)
	if err != nil {
		return nil, fmt.Errorf("could not initialize remote: %w", err)
	}

	// NOTE: the next checks are to avoid error messages getting lost when
	// trying to connect so we can provide better error messages

	err = s.whichCommand(ctx, remote, six)
	if err != nil {
		return nil, err
	}

	err = s.workspaceExists(ctx, remote, uri)
	if err != nil {
		return nil, err
	}

	ses, err := remote.NewSession()
	if err != nil {
		return nil, fmt.Errorf("NewSession: %v", err)
	}

	var extraArgs []string
	if log.IsLevelEnabled(log.TraceLevel) {
		extraArgs = []string{"-p", "-o", "six-workspace-server.log"}
	}

	cmdStr := six
	args := append([]string{"-x", sshPath}, extraArgs...)
	if s.cfg.shell != "" {
		args = append([]string{"-c", cmdStr}, args...)
		cmdStr = s.cfg.shell
	}
	ch := make(chan error)
	cmd := workspaceapi.Cmd{
		Path:    cmdStr,
		Args:    args,
		Watcher: workspaceapi.ChanProcessWatcher(ch),
	}

	stdoutRead, stderrRead, stdinWrite, closers, err := s.setPipes(&cmd)
	if err != nil {
		return nil, fmt.Errorf("could not create pipes: %s", err)
	}

	// context of command should mirror the lifcycle of this scheme
	// not the ctx passed to this constructor, which could have
	// a connection timeout (i.e. retry ctx)
	_, err = ses.StartCommand(s.ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("could not create command: %s", err)
	}

	conn, err := grpc.Dial("",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(_ context.Context, addr string) (net.Conn, error) {
			// FIXME std conn is not goroutine-safe. Close causes *hook=nil to race
			// with a second Close *hook.
			return newStdConn(
				log.StandardLogger(), stdoutRead, stdinWrite, false, /* stdio */
				func() {
					closeHook(errors.New("ssh connection closed unexpectedly"))
				})
		}))
	if err != nil {
		return nil, err
	}

	go debug.CapturePanicReport(func() {
		defer remote.Close()
		err := <-ch
		if err != nil {
			stderrStr, rerr := io.ReadAll(stderrRead)
			if rerr != nil {
				err = fmt.Errorf("could not read error from stderr but there was"+
					"an error executing remote six server over SSH: %s", err)
			} else {
				err = fmt.Errorf("error executing remote six server over SSH: %s: %s", err, stderrStr)
			}
		}
		closeHook(err)
		for _, closer := range closers {
			_ = closer.Close()
		}
	})

	return workspacerpc.NewClient(s.ctx, conn), nil
}

func (s *scheme) setPipes(
	cmd *workspaceapi.Cmd,
) (stdout, stderr, stdin *os.File, closers []io.Closer, err error) {
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("pipe: %v", err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("pipe: %v", err)
	}
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("pipe: %v", err)
	}
	cmd.Stdout = stdoutWrite
	cmd.Stderr = stderrWrite
	cmd.Stdin = stdinRead
	closers = []io.Closer{
		stdoutWrite, stderrWrite, stdinWrite,
		stdoutRead, stderrRead, stdinRead,
	}
	return stdoutRead, stderrRead, stdinWrite, closers, nil
}

func (s *scheme) init(
	ctx context.Context, cc sshConfig, uri workspaceapi.URI,
	statWorkspaceDir func(schemeapi.Scheme, string) (os.FileInfo, error),
) (err error) {
	if uri.Scheme() != Scheme {
		return errors.New("invalid non-ssh scheme")
	}

	s.cfg = cc
	s.user, s.homedir, s.hostPort, s.basePath, err = parseWorkspaceURI(uri, s.getUser)
	if err != nil {
		return fmt.Errorf("could not parse ssh workspaceapi.URI: %s", err)
	}

	// expand any relative path or home aliases
	uri, err = s.URI(uri.Path())
	if err != nil {
		return fmt.Errorf("URI from path %s: %v", uri.Path(), err)
	}

	s.Scheme = newRemoteScheme(ctx, s.connectSchemeFn, uri)
	fi, err := statWorkspaceDir(s.Scheme, uri.Path())
	if err != nil {
		return fmt.Errorf("Stat workspace: %v", err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("workspaceapi.URI does not refer to a directory: %s", uri.String())
	}

	return nil
}

// override to provide user with an error message that guides to a solution
func (s *scheme) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
	pid, err := s.Scheme.StartCommand(ctx, cmd)
	if err != nil && strings.Contains(err.Error(), "executable file not found in $PATH") {
		return 0, fmt.Errorf("%w. Make sure that $PATH is configured "+
			"even for non-interactive shells", err)
	}
	return pid, err
}

func (s *scheme) URI(path string) (workspaceapi.URI, error) {
	absPath, err := s.expandPath(path)
	if err != nil {
		return workspaceapi.URI{}, err
	}

	var uriStr string
	if s.user != "" {
		uriStr = fmt.Sprintf("ssh://%s@%s%s", s.user, s.hostPort, absPath)
	} else {
		uriStr = fmt.Sprintf("ssh://%s%s", s.hostPort, absPath)
	}

	return workspaceapi.ParseURI(uriStr)
}

func (s *scheme) Close() (ret error) {
	if err := s.Scheme.Close(); err != nil {
		ret = multierr.Append(ret, err)
	}
	s.cancelCtx()
	return ret
}

func (s *scheme) expandPath(path string) (string, error) {
	return workspaceapi.ExpandPath(path, func() (*user.User, error) {
		return &user.User{Username: s.user, HomeDir: s.homedir}, nil
	}, func() (string, error) {
		return s.basePath, nil
	})
}
