package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ernestrc/go-tui/cell"
	log "github.com/sirupsen/logrus"
)

var (
	_                 Workspace = (*Manager)(nil)
	errProcNotFound             = errors.New("process not found")
	errProcNotRunning           = errors.New("process not running")
)

// Manager manages resources on a workspace. It satisfies ResourceOpener.
type Manager struct {
	managerCfg
	logger    *log.Logger
	workspace URI
	cmds      map[Pid]*exec.Cmd
	nextPid   int32

	mu              sync.Mutex
	sshConn         sshClient
	sshErr          error
	workspaceClient *Client
	isInitProxy     bool

	initRemote func() (err error)
	userLookup func(string) (*user.User, error)
}

type managerCfg struct {
	sshPrivateKeys []string
	sshTimeout     time.Duration
	sshCommand     string
}

func isFileURI(file URI) bool {
	return strings.HasPrefix(file.uri, "file://")
}

func isSSHURI(file URI) bool {
	return strings.HasPrefix(file.uri, "ssh://")
}

// NewManager allocates storage fore a new Manage and initializes it with workspace.
func NewManager(
	l *log.Logger, workspace URI, opts ...Option,
) (*Manager, error) {
	if l == nil {
		panic("invalid logger")
	}
	ret := new(Manager)
	err := ret.Init(l, workspace, opts...)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Init initializes m or returns an error if there was a problem with
// the given workspace URI.
func (m *Manager) Init(l *log.Logger, workspace URI, opts ...Option) error {
	m.initRemote = func() (err error) {
		return m.initWorkspaceClient(m.managerCfg, m.workspace)
	}
	m.userLookup = user.Lookup
	return m.init(l, workspace, opts...)
}

func (m *Manager) init(l *log.Logger, workspace URI, opts ...Option) error {
	m.logger = l
	m.workspace = workspace
	m.cmds = make(map[Pid]*exec.Cmd)
	for _, o := range opts {
		o(&m.managerCfg)
	}
	if isFileURI(workspace) {
		return m.initLocal()
	}
	if isSSHURI(workspace) {
		return m.initRemote()
	}
	return fmt.Errorf("unknown scheme: %s", workspace)
}

func (m *Manager) getUser() (*user.User, error) {
	if m.workspace.parsed.User == nil {
		return user.Current()
	}
	username := m.workspace.parsed.User.Username()
	return m.userLookup(username)
}

func (m *Manager) extractAbsPath(filename string) (string, error) {
	return extractAbsPath(filename, m.getUser, func() (string, error) {
		return m.workspace.Path(), nil
	})
}

func extractAbsPath(
	filename string,
	getUser func() (*user.User, error),
	cwdFn func() (string, error),
) (string, error) {
	if filename == "~" {
		usr, err := getUser()
		if err != nil {
			return "", fmt.Errorf("could not get current user: %s", err)
		}
		filename = usr.HomeDir
	} else if strings.HasPrefix(filename, "~/") {
		usr, err := getUser()
		if err != nil {
			return "", fmt.Errorf("could not get current user: %s", err)
		}
		filename = filepath.Join(usr.HomeDir, filename[2:])
	}
	if filepath.IsAbs(filename) {
		return filename, nil
	}
	cwd, err := cwdFn()
	if err != nil {
		return "", fmt.Errorf("could not get cwd: %s", err)
	}
	abs := filepath.Join(cwd, filename)
	return abs, nil
}

func (m *Manager) initLocal() error {
	workspacewd := m.workspace.Path()
	fs, err := os.Stat(workspacewd)
	if err != nil {
		return err
	}
	if !fs.IsDir() {
		return errors.New("workspace is not a directory")
	}
	return nil
}

// Recover recovers the file with the swap file.
func (m *Manager) Recover(file, swapFile URI, buf *cell.Buffer) (
	FlusherCloser, error,
) {
	if isFileURI(file) {
		return recoverLocalFile(file, swapFile, buf)
	}
	if isSSHURI(file) {
		return m.recoverRemoteFile(file, swapFile, buf)
	}
	return nil, fmt.Errorf("unknown scheme: %s", file.uri)
}

// Open opens the file at the given URI and initializes buf with the contents of it.
// It uses swapDir as the file recovery and swap directory.
func (m *Manager) Open(
	file URI, buf *cell.Buffer, swapDir URI, readOnly bool,
) (
	fc FlusherCloser, err error,
) {
	if isFileURI(file) {
		fc, err = openLocalFile(file, buf, swapDir, readOnly)
	} else if isSSHURI(file) {
		fc, err = m.openRemoteFile(file, buf, swapDir, readOnly)
	} else {
		return nil, fmt.Errorf("unknown scheme: %s", file.uri)
	}

	if err == nil {
		return fc, nil
	}

	osErr, ok := err.(*osError)
	if !ok {
		return nil, err
	}
	if osErr.isPermission {
		return nil, os.ErrPermission
	}
	if osErr.isNotExist {
		return nil, os.ErrNotExist
	}
	return nil, err
}

func (m *Manager) commandLocal(name string, arg ...string) (Pid, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.Command(name, arg...)
	cmd.Dir = m.workspace.Path()
	m.nextPid++
	m.cmds[Pid(m.nextPid)] = cmd
	return Pid(m.nextPid), nil
}

func (m *Manager) getCmdForPid(pid Pid) (*exec.Cmd, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.cmds[pid]
	return f, ok
}

func (m *Manager) startLocal(pid Pid) error {
	f, ok := m.getCmdForPid(pid)
	if !ok {
		return errProcNotFound
	}
	err := f.Start()
	if err != nil {
		return fmt.Errorf("Cmd.Start: %w", err)
	}
	return nil
}

func (m *Manager) signalLocal(pid Pid, signal syscall.Signal) error {
	f, ok := m.getCmdForPid(pid)
	if !ok {
		return errProcNotFound
	}
	if f.Process == nil {
		return errProcNotRunning
	}
	err := syscall.Kill(int(f.Process.Pid), signal)
	if err != nil {
		return fmt.Errorf("syscall.Kill: %w", err)
	}
	return nil
}

func (m *Manager) stderrPipeLocal(pid Pid) (io.ReadCloser, error) {
	f, ok := m.getCmdForPid(pid)
	if !ok {
		return nil, errProcNotFound
	}
	pipe, err := f.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("Cmd.StderrPipe: %w", err)
	}
	return pipe, err
}

func (m *Manager) stdinPipeLocal(pid Pid) (io.WriteCloser, error) {
	f, ok := m.getCmdForPid(pid)
	if !ok {
		return nil, errProcNotFound
	}
	pipe, err := f.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("Cmd.StdinPipe: %w", err)
	}
	return pipe, err
}

func (m *Manager) stdoutPipeLocal(pid Pid) (io.ReadCloser, error) {
	f, ok := m.getCmdForPid(pid)
	if !ok {
		return nil, errProcNotFound
	}
	pipe, err := f.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("Cmd.StdoutPipe: %w", err)
	}
	return pipe, err
}

func (m *Manager) waitLocal(pid Pid) error {
	f, ok := m.getCmdForPid(pid)
	if !ok {
		return errProcNotFound
	}
	err := f.Wait()
	if err != nil {
		return fmt.Errorf("Cmd.Wait: %w", err)
	}
	return err
}

func (m *Manager) Command(name string, arg ...string) (pid Pid, err error) {
	m.logger.Tracef("workspace.Manager.Command(%s, %#v)", name, arg)
	if m.isInitProxy || isFileURI(m.workspace) {
		pid, err = m.commandLocal(name, arg...)
	} else if isSSHURI(m.workspace) {
		pid, err = m.workspaceClient.Command(name, arg...)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.Command(%s, %#v): %d, %v",
		name, arg, pid, err)
	return
}

func (m *Manager) Start(pid Pid) (err error) {
	m.logger.Tracef("workspace.Manager.Start(%v)", pid)
	if m.isInitProxy || isFileURI(m.workspace) {
		err = m.startLocal(pid)
	} else if isSSHURI(m.workspace) {
		err = m.workspaceClient.Start(pid)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.Start(%v): %v", pid, err)
	return
}

func (m *Manager) Signal(pid Pid, sig syscall.Signal) (err error) {
	m.logger.Tracef("workspace.Manager.Signal(%v, %d)", pid, sig)
	if m.isInitProxy || isFileURI(m.workspace) {
		err = m.signalLocal(pid, sig)
	} else if isSSHURI(m.workspace) {
		err = m.workspaceClient.Signal(pid, sig)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.Signal(%v, %#v): %v",
		pid, sig, err)
	return
}

func (m *Manager) StderrPipe(pid Pid) (ret io.ReadCloser, err error) {
	m.logger.Tracef("workspace.Manager.StderrPipe(%v)", pid)
	if m.isInitProxy || isFileURI(m.workspace) {
		ret, err = m.stderrPipeLocal(pid)
	} else if isSSHURI(m.workspace) {
		ret, err = m.workspaceClient.StderrPipe(pid)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.StderrPipe(%v): %v", pid, err)
	return
}

func (m *Manager) StdinPipe(pid Pid) (ret io.WriteCloser, err error) {
	m.logger.Tracef("workspace.Manager.StdinPipe(%v)", pid)
	if m.isInitProxy || isFileURI(m.workspace) {
		ret, err = m.stdinPipeLocal(pid)
	} else if isSSHURI(m.workspace) {
		ret, err = m.workspaceClient.StdinPipe(pid)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.StdinPipe(%v): %v", pid, err)
	return
}

func (m *Manager) StdoutPipe(pid Pid) (ret io.ReadCloser, err error) {
	m.logger.Tracef("workspace.Manager.StdoutPipe(%v)", pid)
	if m.isInitProxy || isFileURI(m.workspace) {
		ret, err = m.stdoutPipeLocal(pid)
	} else if isSSHURI(m.workspace) {
		ret, err = m.workspaceClient.StdoutPipe(pid)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.StdoutPipe(%v): %v", pid, err)
	return
}

func (m *Manager) Wait(pid Pid) (err error) {
	m.logger.Tracef("workspace.Manager.Wait(%v)", pid)
	if m.isInitProxy || isFileURI(m.workspace) {
		err = m.waitLocal(pid)
	} else if isSSHURI(m.workspace) {
		err = m.workspaceClient.Wait(pid)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.Wait(%v): %v", pid, err)
	return
}

func (m *Manager) URI(path string) (uri URI, err error) {
	m.logger.Tracef("workspace.Manager.URI(%s)", path)
	if isFileURI(m.workspace) {
		uri, err = m.localURI(path)
	} else if isSSHURI(m.workspace) {
		uri, err = m.remoteURI(path)
	} else {
		panic("manager has an invalid workspace URI")
	}
	m.logger.Debugf("workspace.Manager.URI(%s): %s, %v",
		path, uri, err)
	return
}

// Close closes all resources associated with this Manager.
func (m *Manager) Close() error {
	m.logger.Trace("workspace.Manager.Close()")
	var ret error
	if m.sshConn != nil {
		ret = m.sshConn.Close()
	}

	// do not block while sending signals
	m.mu.Lock()
	var copyCmds []*exec.Cmd
	for _, cmd := range m.cmds {
		copyCmds = append(copyCmds, cmd)
	}
	m.mu.Unlock()

	for _, cmd := range copyCmds {
		if cmd.Process != nil {
			err := syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
			if err != nil {
				ret = err
			}
		}
	}
	m.cmds = nil
	m.logger.Debugf("workspace.Manager.Close(): %v", ret)
	return ret
}
