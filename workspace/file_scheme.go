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

package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ernestrc/go-multierror"
	"github.com/ernestrc/sensible/find"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/bluectx"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/notify"
	"github.com/unstablebuild/pty"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
)

const (
	// FileScheme represents the local file URL scheme.
	FileScheme         = "file"
	watcherWaitTimeout = 2 * time.Minute
)

var (
	errProcNotFound = errors.New("process not found")
)

// NewFileScheme returns a Scheme that manages resources
// on the local file system.
func NewFileScheme(
	ctx context.Context, cfg config.Config, workspace workspaceapi.URI,
) (schemeapi.Scheme, error) {
	ret := new(fileScheme)
	ret.getUser = user.Current
	ret.lookupUser = user.Lookup
	ret.osStat = os.Stat
	err := ret.init(cfg, workspace)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// OpenFile opens the given filename using the default current working
// directory's file scheme. This is preferrable over os.OpenFile, because
// it does expansion of paths (i.e. ~ is expanded to the current user's home directory).
func OpenFile(filename string, flag int, perm os.FileMode) (workspaceapi.File, error) {
	cwdURI, _ := workspaceapi.CurrentUserHostURI(".")
	fs, err := NewFileScheme(context.Background(), config.NopConfig(), cwdURI)
	if err != nil {
		return nil, fmt.Errorf("file scheme: %v", err)
	}
	defer fs.Close() // nolint:errcheck
	f, werr := fs.Open(filename, flag, perm)
	if werr != nil {
		return nil, werr.ToError()
	}
	return f, nil
}

// ReadFile reads the file named by filename and returns the contents.
// See os.ReadFile for more details.
func ReadFile(filename string) ([]byte, error) {
	f, err := OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return io.ReadAll(f)
}

type fileScheme struct {
	osStat     func(path string) (os.FileInfo, error)
	getUser    func() (*user.User, error)
	lookupUser func(string) (*user.User, error)
	workspace  workspaceapi.URI
	ctx        context.Context
	cancelCtx  func()
	cmds       sync.Map // map[workspaceapi.Pid]struct{}

	watchpoints    sync.Map
	nextWatchPoint atomic.Int64

	// This is important to prevent runtime finalizers
	// running on files that are garbage collected on host
	// but that clients hold references to.
	//
	// Technically we could leak files if clients
	// never close files, but once Server is garbage
	// collected, all files that are orhpaned will be closed.
	files sync.Map // map[uintptr]workspaceapi.File
}

func (p *fileScheme) init(cfg config.Config, workspace workspaceapi.URI) error {
	if workspace.Host() != "" || workspace.User() != "" || workspace.Scheme() != FileScheme {
		return errors.New("invalid file URI")
	}
	workspacewd := workspace.Path()
	fs, err := p.osStat(workspacewd)
	if err != nil {
		return err
	}
	if !fs.IsDir() {
		return fmt.Errorf("workspaceapi.URI does not refer to a directory: %s", workspace.String())
	}
	p.workspace = workspace
	p.nextWatchPoint.Add(1)
	p.ctx, p.cancelCtx = context.WithCancel(context.Background())
	return nil
}

func (p *fileScheme) Open(path string, flag int, perm os.FileMode) (workspaceapi.File, *workspaceapi.Error) {
	var err error
	path, err = workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return nil, workspaceapi.NopError(err)
	}
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, &workspaceapi.Error{
			Err:          err,
			IsPermission: os.IsPermission(err),
			IsExist:      os.IsExist(err),
			IsNotExist:   os.IsNotExist(err),
		}
	}

	ret := &fileSchemeFile{File: f, p: p, fd: f.Fd()}
	p.files.Store(f.Fd(), ret)

	return ret, nil
}

func (p *fileScheme) NewFile(fd uintptr, filename string) workspaceapi.File {
	f, ok := p.files.Load(fd)
	if !ok {
		return nil
	}
	return f.(workspaceapi.File)
}

func (p *fileScheme) Remove(path string) error {
	var err error
	path, err = workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (p *fileScheme) Rename(old, new string) error {
	var err error
	old, err = workspaceapi.ExpandPath(old, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return err
	}
	new, err = workspaceapi.ExpandPath(new, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return err
	}
	return os.Rename(old, new)
}

func (p *fileScheme) Stat(path string) (os.FileInfo, error) {
	var err error
	path, err = workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (p *fileScheme) ReadDir(name string) ([]os.DirEntry, error) {
	var err error
	name, err = workspaceapi.ExpandPath(name, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return nil, err
	}
	return os.ReadDir(name)
}

func (p *fileScheme) Lstat(path string) (os.FileInfo, error) {
	var err error
	path, err = workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return nil, err
	}
	return os.Lstat(path)
}

func (p *fileScheme) ReadLink(path string) (string, error) {
	var err error
	path, err = workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return "", err
	}
	return os.Readlink(path)
}

func (p *fileScheme) getUserOrLookup() (*user.User, error) {
	if p.workspace.User() == "" {
		return p.getUser()
	}
	username := p.workspace.User()
	return p.lookupUser(username)
}

func (p *fileScheme) URI(path string) (workspaceapi.URI, error) {
	absPath, err := workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return workspaceapi.URI{}, fmt.Errorf("expand path: %w", err)
	}
	return makeLocalURI(absPath)
}

func (p *fileScheme) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "fileScheme").
		WithField("URI", p.workspace.String()).
		Logf(level, msg, args...)
}

func (p *fileScheme) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	var err error
	path := cmd.Path
	if filepath.Base(cmd.Path) == cmd.Path {
		path, err = find.Executable(cmd.Path)
		if err != nil {
			p.log(log.WarnLevel,
				"find.Executable: could not find executable of '%s' in path. "+
					"Falling back to shell expanding it: %v", cmd.Path, err)
			path = cmd.Path
		}
	}
	// ensure that if file scheme is closed, all commands are cleaned up
	var cancelFn func()
	ctx, cancelFn = context.WithCancel(ctx)
	ctx = bluectx.First(p.ctx, ctx)
	stdcmd := exec.CommandContext(ctx, path, cmd.Args...)

	if cmd.Dir != "" {
		stdcmd.Dir = cmd.Dir
	} else {
		stdcmd.Dir = p.workspace.Path()
	}

	stdcmd.Env = cmd.Env
	stdcmd.SysProcAttr = cmd.SysProcAttr

	// unwrap os.File if Stdout is a fileSchemeFile
	// In principle, closing of files should be done by callers
	// so it's fine to lose delete from map on close, as the stdcmd
	// routine should not close them. This is necessary
	// to allow the standard library to run its ioctl checks
	// correctly, for example when running a process over a pty/tty.
	stdcmd.Stdout = tryUnwrapFileWriter(cmd.Stdout)
	stdcmd.Stderr = tryUnwrapFileWriter(cmd.Stderr)
	stdcmd.Stdin = tryUnwrapFileReader(cmd.Stdin)

	err = stdcmd.Start()
	if err != nil {
		cancelFn()
		return 0, err
	}

	pid := workspaceapi.Pid(stdcmd.Process.Pid)
	go debug.CapturePanicReport(func() {
		defer cancelFn()
		defer func() {
			p.cmds.Delete(pid)
		}()

		start := time.Now()
		p.log(log.TraceLevel, "exec.Command: Wait: pid=%d", stdcmd.Process.Pid)
		err := stdcmd.Wait()
		p.log(log.DebugLevel, "exec.Command: Wait returned: cmd=%v pid=%d, err=%v"+
			", duration=%s",
			stdcmd.Args, stdcmd.Process.Pid, err, time.Since(start).String())

		// set a timeout to how long we wait for a watcher
		// to drain the error. This is just to avoid
		// buggy watchers to cause this goroutine to block forever,
		// so the timeout should be in the order of minutes.
		// use a new context so the cancelation of the command doesn't
		// prevent watcher from being called.
		ctx, cancel := context.WithTimeout(p.ctx,
			watcherWaitTimeout)
		defer cancel()

		if cmd.Watcher != nil && cmd.Watcher.WatchProcess() != nil {
			select {
			case cmd.Watcher.WatchProcess() <- err:
			case <-ctx.Done():
				p.log(log.WarnLevel, "could not deliver error to watcher chan: "+
					"watcher not ready for too long")
			}
		}
	})

	p.log(log.DebugLevel, "exec.Command: (%#v, pid=%d)", cmd, stdcmd.Process.Pid)

	p.cmds.Store(pid, struct{}{})

	return pid, nil
}

func (p *fileScheme) Signal(pid workspaceapi.Pid, signal syscall.Signal) error {
	_, ok := p.cmds.Load(pid)
	if !ok {
		return errProcNotFound
	}

	err := syscall.Kill(int(pid), signal)
	if err != nil {
		return fmt.Errorf("syscall kill: %w", err)
	}
	return nil
}

func (p *fileScheme) NewPty(ctx context.Context) (workspaceapi.Pty, error) {
	// open master/slave files
	pty, tty, err := pty.Open()
	if err != nil {
		return workspaceapi.Pty{}, fmt.Errorf("open pty: %v", err)
	}

	master := &fileSchemeFile{File: pty, p: p, fd: pty.Fd()}
	p.files.Store(pty.Fd(), master)

	slave := &fileSchemeFile{File: tty, p: p, fd: tty.Fd()}
	p.files.Store(tty.Fd(), slave)

	return workspaceapi.Pty{
		Master: master,
		Slave:  slave,
	}, nil
}

func (p *fileScheme) SetPtySize(pp workspaceapi.Pty, width, height int) error {
	ptyFile, ok := pp.Master.(*fileSchemeFile)
	if !ok {
		return fmt.Errorf("extraneous pty: %+v", pp)
	}

	err := pty.Setsize(ptyFile.Fd(), &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("set pty size: %v", err)
	}
	return nil
}

func (p *fileScheme) MkdirAll(path string, perm os.FileMode) error {
	var err error
	path, err = workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

func (p *fileScheme) Watch(
	path string, c chan<- workspaceapi.EventInfo, events ...workspaceapi.Event,
) (int, error) {
	path, err := workspaceapi.ExpandPath(path, p.getUserOrLookup, func() (string, error) {
		return p.workspace.Path(), nil
	})
	if err != nil {
		return 0, err
	}
	// NOTE: We cannot simply re-use notify.Event values, because we need
	// this values to remain stable across platforms: i.e. a value is sent
	// across the wire from a linux to a macos system.
	var notifyEvents []notify.Event
	for _, ev := range events {
		var nev notify.Event
		switch ev {
		case workspaceapi.Create:
			nev = notify.Create
		case workspaceapi.Write:
			nev = notify.Write
		case workspaceapi.Rename:
			nev = notify.Rename
		case workspaceapi.Remove:
			nev = notify.Remove
		}
		notifyEvents = append(notifyEvents, nev)
	}
	ch := make(chan notify.EventInfo, 8192)
	go debug.CapturePanicReport(func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				uri, err := p.URI(ev.Path())
				if err != nil {
					p.log(log.WarnLevel, "watched file uri %q: %v", ev.Path(), err)
					continue
				}
				ei := newEventInfo(ev, uri)
				select {
				case c <- ei:
				case <-p.ctx.Done():
					return
				}
			}
		}
	})
	w, err := notify.Watch(path, ch, notifyEvents...)
	if err != nil {
		return 0, fmt.Errorf("notify: %v", err)
	}
	id := p.nextWatchPoint.Add(1)
	p.watchpoints.Store(int(id), w)
	return int(id), nil
}

func (p *fileScheme) StopWatch(ID int) error {
	w, ok := p.watchpoints.LoadAndDelete(ID)
	if !ok {
		return errors.New("watchpoint not found")
	}
	closer := w.(io.Closer)
	return closer.Close()
}

func (p *fileScheme) Close() (ret error) {
	p.cancelCtx()
	p.watchpoints.Range(func(id any, value any) bool {
		if err := p.StopWatch(id.(int)); err != nil {
			ret = multierror.Append(ret, err)
		}
		return true
	})
	return
}

// enables overriding Close to delete from map.
type fileSchemeFile struct {
	*os.File
	p *fileScheme
	// cache fd so pty.SetSize doesn't cause races on fd destroy (on reads)
	fd uintptr
}

func (f *fileSchemeFile) Fd() uintptr {
	return f.fd
}

func (f *fileSchemeFile) Close() error {
	f.p.files.Delete(f.Fd())
	return f.File.Close()
}

func makeLocalURI(path string) (workspaceapi.URI, error) {
	uriStr := "file://" + path
	return workspaceapi.ParseURI(uriStr)
}

func tryUnwrapFileWriter(f io.Writer) io.Writer {
	if f, ok := f.(*fileSchemeFile); ok {
		return f.File
	}
	return f
}

func tryUnwrapFileReader(f io.Reader) io.Reader {
	if f, ok := f.(*fileSchemeFile); ok {
		return f.File
	}
	return f
}

type eventInfo struct {
	uri workspaceapi.URI
	e   workspaceapi.Event
	d   bool
}

func (e eventInfo) Event() workspaceapi.Event {
	return e.e
}

func (e eventInfo) URI() workspaceapi.URI {
	return e.uri
}

func (e eventInfo) IsDir() (bool, error) {
	return e.d, nil
}

func newEventInfo(ei notify.EventInfo, uri workspaceapi.URI) eventInfo {
	var nev workspaceapi.Event
	switch ei.Event() {
	case notify.Create:
		nev = workspaceapi.Create
	case notify.Write:
		nev = workspaceapi.Write
	case notify.Rename:
		nev = workspaceapi.Rename
	case notify.Remove:
		nev = workspaceapi.Remove
	}
	// this can be an error only in windows
	isDir, _ := ei.IsDir()
	return eventInfo{
		d:   isDir,
		uri: uri,
		e:   nev,
	}
}
