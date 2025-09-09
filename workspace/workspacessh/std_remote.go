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
	"os"
	"os/user"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/unstablebuild/blue/bluectx"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/workspace"
)

const watcherWaitTimeout = 2 * time.Minute

var sigMap = map[syscall.Signal]ssh.Signal{
	syscall.SIGABRT: "ABRT",
	syscall.SIGALRM: "ALRM",
	syscall.SIGFPE:  "FPE",
	syscall.SIGHUP:  "HUP",
	syscall.SIGILL:  "ILL",
	syscall.SIGINT:  "INT",
	syscall.SIGKILL: "KILL",
	syscall.SIGPIPE: "PIPE",
	syscall.SIGQUIT: "QUIT",
	syscall.SIGSEGV: "SEGV",
	syscall.SIGTERM: "TERM",
	syscall.SIGUSR1: "USR1",
	syscall.SIGUSR2: "USR2",
}

// used to adapt ssh.Client to sshClient
type stdRemote struct {
	parentCtx context.Context
	client    *ssh.Client
	quitCh    chan struct{}
}

// used to adapt ssh.Session to Executor
type goSshSession struct {
	parentCtx context.Context
	ses       *ssh.Session
	quitCh    chan struct{}
	pid       int
}

func newStdRemote(ctx context.Context, cfg sshConfig, uri workspaceapi.URI) (
	remote, error,
) {
	username, err := usernameOrCurrent(uri)
	if err != nil {
		return nil, err
	}
	auths, err := authMethodsFromURI(cfg, uri)
	if err != nil {
		return nil, err
	}

	var hostkeyCallback ssh.HostKeyCallback
	if cfg.insecure {
		hostkeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		hostkeyCallback, err = defaultHostkeyCallback()
		if err != nil {
			return nil, err
		}
	}
	conf := &ssh.ClientConfig{
		User:            username,
		HostKeyCallback: hostkeyCallback,
		Auth:            auths,
		Timeout:         cfg.timeout,
	}

	hostport := hostPortFromURI(uri)
	conn, err := ssh.Dial("tcp", hostport, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to ssh dial: %s", err)
	}
	ret := &stdRemote{parentCtx: ctx, client: conn, quitCh: make(chan struct{})}
	return ret, nil
}

func (s *goSshSession) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	if s.pid != 0 {
		panic("Command called more than once on an ssh session")
	}
	if cmd.Path == "" {
		return 0, errors.New("no command")
	}

	s.pid++

	s.ses.Stdout = cmd.Stdout
	s.ses.Stderr = cmd.Stderr
	s.ses.Stdin = cmd.Stdin

	err := s.ses.Start(
		fmt.Sprintf("%s %s", cmd.Path, strings.Join(cmd.Args, " ")))
	if err != nil {
		return 0, err
	}

	// ensure that at least one of the ctxs passed to First
	// is canceled after the command is done.
	ctx, cancel := context.WithCancel(ctx)
	ctx = bluectx.First(s.parentCtx, ctx)

	// wait and dispatch error to watcher
	go debug.CapturePanicReport(func() {
		defer cancel()

		err := s.ses.Wait()
		if cmd.Watcher != nil && cmd.Watcher.WatchProcess() != nil {
			// avoid buggy watchers to cause this goroutine to block forever,
			// so the timeout should be in the order of minutes.
			ctx, cancelTimeout := context.WithTimeout(
				context.Background(), watcherWaitTimeout)
			defer cancelTimeout()
			select {
			case <-ctx.Done():
			case cmd.Watcher.WatchProcess() <- err:
			}
		}
	})

	// kill command if context is done
	go debug.CapturePanicReport(func() {
		select {
		case <-ctx.Done():
			s.ses.Close()
		case <-s.quitCh:
		}
	})

	return workspaceapi.Pid(s.pid), nil
}

func (s *goSshSession) Signal(_ workspaceapi.Pid, sig syscall.Signal) error {
	signal, ok := sigMap[sig]
	if !ok {
		return errors.New("unknown signal")
	}
	return s.ses.Signal(signal)
}

func (r *goSshSession) Close() error {
	return r.ses.Close()
}

func (r *stdRemote) NewSession() (schemeapi.Executor, error) {
	ses, err := r.client.NewSession()
	if err != nil {
		return nil, err
	}
	ret := &goSshSession{parentCtx: r.parentCtx, ses: ses, quitCh: r.quitCh}
	return ret, nil
}

func (r *stdRemote) Close() error {
	close(r.quitCh)
	return r.client.Close()
}

func authMethodsFromURI(config sshConfig, uri workspaceapi.URI) ([]ssh.AuthMethod, error) {
	var auths []ssh.AuthMethod
	for _, key := range config.privateKeys {
		signer, err := privateKeySigner(key)
		if err != nil {
			// TODO should notify via notifications and continue
			return nil, err
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if pass, ok := uri.Password(); ok {
		auths = append(auths, ssh.Password(pass))
	}
	return auths, nil
}

func defaultHostkeyCallback() (ssh.HostKeyCallback, error) {
	home, err := currentHomePath()
	if err != nil {
		return nil, err
	}

	knownHostsPath := path.Join(home, ".ssh/known_hosts")
	hostkeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %s", knownHostsPath, err)
	}
	return hostkeyCallback, nil
}

func getCurrentUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get default user: %s", err)
	}
	return u.Username, nil
}

func usernameOrCurrent(uri workspaceapi.URI) (string, error) {
	if uri.User() != "" {
		return uri.User(), nil
	}
	return getCurrentUser()
}

func hostPortFromURI(uri workspaceapi.URI) string {
	hostname, port := uri.Hostname(), uri.Port()
	if port == "" {
		port = "22"
	}
	return fmt.Sprintf("%s:%s", hostname, port)
}

func currentHomePath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to lookup current username: %s", err)
	}
	return u.HomeDir, nil
}

// TODO improve collection of passhprase via secret prompt
func readPassphrase(key string) (string, error) {
	fmt.Fprintf(os.Stdout, "Key %s requires a passphrase: ", key)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	password := string(bytePassword)
	return password, nil
}

func privateKeySigner(privateKeyPath string) (ssh.Signer, error) {
	privateKey, err := workspace.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("could not read private key file %s: %s",
			privateKeyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err == nil {
		return signer, nil
	}
	if _, ok := err.(*ssh.PassphraseMissingError); !ok {
		return nil, fmt.Errorf("could not parse private key at %s: %s",
			privateKeyPath, err)
	}

	// handle passhprase errors by reading password from stdin
	passphrase, err := readPassphrase(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("could not read passphrase for private key at %s: %s",
			privateKeyPath, err)
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(privateKey, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("could not parse private key with passphrase at %s: %s",
			privateKeyPath, err)
	}
	return signer, nil
}
