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

package idetask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/ide/plugin"
)

func TestManager(t *testing.T) {
	t.Run("StopTask closes windows with no leaks", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		const N = 25
		for i := range N {
			require.NoError(t, m.RunTask(Task{
				Name: fmt.Sprintf("N%02d", i),
				Cmd:  "run",
				Args: []string{fmt.Sprint(i)},
			}))
		}
		for i := range N {
			require.NoError(t, m.StopTask(fmt.Sprintf("N%02d", i)))
		}
		for i, w := range wm.Created() {
			assert.True(t, w.closed, fmt.Sprintf("window %d", i))
		}
	})

	t.Run("RunTask starts and creates window", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{
			Name:   "build",
			Cmd:    "echo",
			Args:   []string{"hello"},
			Filter: "",
		}
		err := m.RunTask(task)
		require.NoError(t, err, "RunTask should succeed")

		cmds := exec.StartedCmds()
		require.Len(t, cmds, 1)
		assert.Equal(t, "echo", cmds[0].Path)
		assert.Equal(t, []string{"hello"}, cmds[0].Args)

		ws := wm.Created()
		require.Len(t, ws, 1)
	})

	t.Run("OnFocus task unminimizes it", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{Name: "build", Cmd: "echo"}
		err := m.RunTask(task)
		require.NoError(t, err, "RunTask should succeed")

		ws := wm.Created()
		require.Len(t, ws, 1)

		m.onFocus(&fakeWindow{}, ws[0])
		_, minimized := ws[0].IsMinimized()
		assert.False(t, minimized)
	})

	t.Run("OnFocus changes to other window, minimizes it", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{Name: "build", Cmd: "echo"}
		err := m.RunTask(task)
		require.NoError(t, err, "RunTask should succeed")

		ws := wm.Created()
		require.Len(t, ws, 1)

		m.onFocus(&fakeWindow{}, ws[0])
		_, minimized := ws[0].IsMinimized()
		assert.False(t, minimized)

		m.onFocus(ws[0], &fakeWindow{})
		_, minimized = ws[0].IsMinimized()
		assert.True(t, minimized)
	})

	t.Run("SetMaxWidth changes to max width passed to new task's plugin handler", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		var actualMaxWidth int
		var mu sync.Mutex
		m.newPlugin = func(
			publisher browser.EventPublisher, notifications browser.Notifications,
			e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
			cmdAndArgs []string, maxWidth int, opts ...plugin.Option,
		) (browser.ScrollableFloating, error) {
			actualMaxWidth = maxWidth
			mu.Unlock()
			return browser.NopScrollableFloatingHandler(
				handler.NopScrollableFloating(),
			), nil
		}

		m.SetMaxWidthHeight(999, 111)
		task := Task{Name: "build", Cmd: "echo"}

		mu.Lock()
		err := m.RunTask(task)
		require.NoError(t, err, "RunTask should succeed")

		mu.Lock()
		assert.Equal(t, 999, actualMaxWidth)
	})

	t.Run("executor failure is surfaced as a failed task", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		exec.startErr = errors.New("executor exploded")
		m := newTestManager(wm, exec)

		err := m.RunTask(Task{Name: "X", Cmd: "noop"})
		require.NoError(t, err)
		assert.Len(t, wm.Created(), 1)

		tasks := m.ListTasks()
		require.Len(t, tasks, 1)
		info := tasks[0]
		assert.False(t, info.LastSuccess)
	})

	t.Run("window creation is surfaced as an error, no tasks are leaked", func(t *testing.T) {
		wm := newFakeBrowser()
		wm.createErr = errors.New("wm failed")
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		err := m.RunTask(Task{Name: "x", Cmd: "noop"})
		require.Error(t, err)

		assert.Len(t, exec.PIDs(), 0)
		tasks := m.ListTasks()
		require.Len(t, tasks, 0)
	})

	t.Run("StopTask kills process and closes window", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		require.NoError(t, m.RunTask(Task{Name: "svc", Cmd: "run"}))
		require.NoError(t, m.StopTask("svc"))
		ws := wm.Created()
		require.Len(t, ws, 1)
		assert.True(t, ws[0].closed, "window must be closed on StopTask")

		// handler.Close is what actually kills the process
		require.Len(t, wm.createdHandlers, 1)
		assert.True(t, ws[0].closed, "handler must be closed on StopTask")
	})

	t.Run("StopTask on unkown task is an error", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		err := m.StopTask("nope")
		require.Error(t, err)
	})

	t.Run("StopTask a second time is an error", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		require.NoError(t, m.RunTask(Task{Name: "job", Cmd: "run"}))
		require.NoError(t, m.StopTask("job"))

		err := m.StopTask("job")
		require.Error(t, err)
	})

	t.Run("RunTask allows concurrent tasks", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		err := m.RunTask(Task{Name: "A", Cmd: "run", Args: []string{"A"}})
		require.NoErrorf(t, err, "start A")
		err = m.RunTask(Task{Name: "B", Cmd: "run", Args: []string{"B"}})
		require.NoErrorf(t, err, "start B")

		require.Len(t, exec.StartedCmds(), 2)
		require.Len(t, wm.Created(), 2)

		require.NoError(t, m.StopTask("A"))
		time.Sleep(5 * time.Millisecond)

		require.Len(t, exec.StartedCmds(), 2)
		ws := wm.Created()
		require.Len(t, ws, 2)
		closed := 0
		for _, w := range ws {
			if w.closed {
				closed++
			}
		}
		assert.Equal(t, 1, closed, "only one window should be closed")
	})

	t.Run("RunTask with duplicate name is an error", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		require.NoError(t, m.RunTask(Task{Name: "dup", Cmd: "run"}))
		require.Error(t, m.RunTask(Task{Name: "dup", Cmd: "run"}))
	})

	t.Run("RunTask sets passed alignment", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)
		expectedAlignment := component.AlignmentBottom
		wm.createHook = func(_ string, cfg browserapi.FloatingConfig) {
			assert.Equal(t, expectedAlignment, cfg.Alignment&expectedAlignment)
		}

		err := m.RunTask(Task{
			Name:              "aligned",
			Cmd:               "run",
			MinimizeAlignment: expectedAlignment,
		})
		require.NoError(t, err)
	})

	t.Run("many tasks start/stop leave no leaks", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		for i := range 10000 {
			name := fmt.Sprintf("task-%d", i)
			require.NoError(t, m.RunTask(Task{Name: name, Cmd: "run"}))
			require.NoError(t, m.StopTask(name))
		}

		for _, w := range wm.Created() {
			assert.True(t, w.closed)
		}
	})

	t.Run("process exit with error sets the task to errored", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		require.NoError(t, m.RunTask(Task{Name: "RainingBlood", Cmd: "Slayer"}))

		assertTaskRunsWithin(t, m, 1*time.Second, "RainingBlood", errors.New("boom"),
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.False(t, info.LastSuccess)
				assert.NotZero(t, info.LastDuration)
				return true
			})
	})

	t.Run("process exit with no error sets the task to success", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		require.NoError(t, m.RunTask(Task{Name: "Yo Marco el Minuto", Cmd: "La Mala"}))

		assertTaskRunsWithin(t, m, 1*time.Second, "Yo Marco el Minuto", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.True(t, info.LastSuccess)
				assert.NotZero(t, info.LastDuration)
				return true
			})
	})

	t.Run("if files change on workspace, task is re-run when idle", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		require.NoError(t, m.RunTask(Task{Name: "task", Cmd: "runtask"}))
		assertTaskRunsWithin(t, m, 1*time.Second, "task", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.True(t, info.LastSuccess)
				assert.NotZero(t, info.LastDuration)
				return true
			})

		sendEvent(t, m, exec, "task", "temp")
		// wait until it's running
		assertTaskWithin(t, m, 1*time.Second, "task",
			func(info TaskInfo) bool {
				return info.Running
			})
		// wait until it's done running
		assertTaskRunsWithin(t, m, 1*time.Second, "task", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.True(t, info.LastSuccess)
				assert.Equal(t, 2, info.Runs)
				assert.NotZero(t, info.LastDuration)
				return true
			})

	})

	t.Run("if files change on workspace and it matches filter, task is re-run", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{Name: "task", Cmd: "runtask", Filter: "*.go,*.md"}
		require.NoError(t, m.RunTask(task))
		assertTaskRunsWithin(t, m, 1*time.Second, "task", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.True(t, info.LastSuccess)
				assert.NotZero(t, info.LastDuration)
				return true
			})

		sendEvent(t, m, exec, "task", "dir/someotherfile.md")
		assertTaskWithin(t, m, 1*time.Second, "task",
			func(info TaskInfo) bool {
				return info.Runs == 2
			})
	})

	t.Run("ReplaceTask runs task with new command and arguments", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{Name: "task", Cmd: "runtask", Filter: "*.go,*.md"}
		require.NoError(t, m.RunTask(task))
		assertTaskRunsWithin(t, m, 1*time.Second, "task", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.True(t, info.LastSuccess)
				assert.NotZero(t, info.LastDuration)
				return true
			})

		require.NoError(t, m.ReplaceTask(task.Name, "rumtask", "arg1"))
		assertTaskWithin(t, m, 1*time.Second, "task",
			func(info TaskInfo) bool {
				assert.Equal(t, []string{"rumtask", "arg1"}, info.CmdAndArgs)
				return info.Runs == 2
			})
	})

	t.Run("if ignored files change on workspace, task is not run", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{Name: "task", Cmd: "runtask", Filter: "*.go,dir/*.md"}
		require.NoError(t, m.RunTask(task))
		assertTaskRunsWithin(t, m, 1*time.Second, "task", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				assert.True(t, info.LastSuccess)
				assert.NotZero(t, info.LastDuration)
				return true
			})

		sendEvent(t, m, exec, "task", "someotherfile.md")
		time.Sleep(1 * time.Second)
		assertTaskWithin(t, m, 1*time.Second, "task",
			func(info TaskInfo) bool {
				assert.Equal(t, 1, info.Runs)
				return true
			})
	})

	t.Run("if commonly ignored files change on workspace, task is not run", func(t *testing.T) {
		wm := newFakeBrowser()
		exec := newFakeScheme()
		m := newTestManager(wm, exec)

		task := Task{Name: "task", Cmd: "runtask"}
		require.NoError(t, m.RunTask(task))
		assertTaskRunsWithin(t, m, 1*time.Second, "task", nil,
			func(info TaskInfo) bool {
				if info.Running {
					return false
				}
				return true
			})

		sendEvent(t, m, exec, "task", ".file.swp")
		time.Sleep(1 * time.Second)
		assertTaskWithin(t, m, 1*time.Second, "task",
			func(info TaskInfo) bool {
				assert.Equal(t, 1, info.Runs)
				return true
			})
	})
}

func sendEvent(t *testing.T, m *Manager, exec *fakeScheme, taskname, filename string) {
	taskIfc, ok := m.tasks.Load(taskname)
	require.True(t, ok)
	task := taskIfc.(*Task)
	ch := exec.tasks[task.watchID]
	require.NotNil(t, ch)

	uri, err := workspaceapi.ParseURI(filepath.Join("memory:///", filename))
	require.NoError(t, err)
	ch <- testEventInfo{uri: uri}
}

func newTestManager(b *fakeBrowser, scheme schemeapi.Scheme) *Manager {
	m := NewManager(b, scheme, func(fn func()) bool {
		fn()
		return true
	})
	m.newPlugin = func(
		publisher browser.EventPublisher, notifications browser.Notifications,
		e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
		cmdAndArgs []string, maxWidth int, opts ...plugin.Option,
	) (browser.ScrollableFloating, error) {
		_, err := e.StartCommand(context.Background(), workspaceapi.Cmd{
			Path: cmdAndArgs[0],
			Args: cmdAndArgs[1:],
			SysProcAttr: &syscall.SysProcAttr{
				Setsid:  true,
				Setctty: true,
			},
		})
		if err != nil {
			return nil, err
		}
		var closed bool
		handler := browser.FuncScrollableFloatingHandler(
			handler.NopScrollableFloating(),
			func() error {
				if closed {
					return errors.New("already closed")
				}
				closed = true
				return nil
			})
		b.createdHandlers = append(b.createdHandlers, handler)
		return handler, nil
	}
	return m
}

func assertTaskRunsWithin(
	t *testing.T, m *Manager, within time.Duration,
	name string, doneErr error, fn func(info TaskInfo) bool,
) {
	t.Helper()
	taskIfc, ok := m.tasks.Load(name)
	require.True(t, ok)
	task := taskIfc.(*Task)
	task.donech <- doneErr

	assertTaskWithin(t, m, within, name, fn)
}

func assertTaskWithin(
	t *testing.T, m *Manager, within time.Duration,
	name string, fn func(info TaskInfo) bool,
) {
	timeout := time.After(within)
	for {
		taskIfc, ok := m.tasks.Load(name)
		require.True(t, ok)
		task := taskIfc.(*Task)

		select {
		case <-timeout:
			t.Logf("didn't set the task to done within the expected time")
			t.Fail()
			return
		default:
			info := task.Info()
			if !fn(info) {
				continue
			}
			return
		}
	}
}

// fakeScheme simulates process start/exit and captures the Cmd passed in.
// It writes deterministic stdout/stderr and terminates when ctx is canceled.
type fakeScheme struct {
	schemeapi.Scheme
	mu        sync.Mutex
	started   []workspaceapi.Cmd
	pidSeq    atomic.Int64
	procs     map[workspaceapi.Pid]*procState
	startErr  error                      // if set, StartCommand will return this error
	startHook func(cmd workspaceapi.Cmd) // optional test hook
	tasks     map[int]chan<- schemeapi.EventInfo
	next      int
}

type procState struct {
	ctx       context.Context
	cancelled atomic.Bool
	doneCh    chan struct{}

	stdout io.Writer
	stderr io.Writer
}

func newFakeScheme() *fakeScheme {
	return &fakeScheme{
		procs: make(map[workspaceapi.Pid]*procState),
		tasks: make(map[int]chan<- schemeapi.EventInfo),
	}
}

func (f *fakeScheme) StopWatch(id int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tasks, id)
	return nil
}

func (f *fakeScheme) Watch(
	path string, c chan<- schemeapi.EventInfo, events ...schemeapi.Event,
) (int, error) {
	f.mu.Lock()
	f.next++
	f.tasks[f.next] = c
	f.mu.Unlock()
	return f.next, nil
}

func (f *fakeScheme) ReadDir(string) ([]os.DirEntry, error) {
	return nil, nil
}

func (f *fakeScheme) URI(path string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI(filepath.Join("memory://", path))
}

func (f *fakeScheme) OpenFile(path string, flag int, perm os.FileMode) (
	workspaceapi.File, error,
) {
	return nil, os.ErrNotExist
}

func (f *fakeScheme) StartCommand(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	if f.startHook != nil {
		f.startHook(cmd)
	}
	if f.startErr != nil {
		return 0, f.startErr
	}

	p := workspaceapi.Pid(f.pidSeq.Add(1))
	ps := &procState{
		ctx:    ctx,
		doneCh: make(chan struct{}),
		stdout: cmd.Stdout,
		stderr: cmd.Stderr,
	}
	f.mu.Lock()
	f.started = append(f.started, cmd)
	f.procs[p] = ps
	f.mu.Unlock()

	go func() {
		defer close(ps.doneCh)
		<-ctx.Done()
		ps.cancelled.Store(true)
	}()

	return p, nil
}

func (f *fakeScheme) PIDs() []workspaceapi.Pid {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]workspaceapi.Pid, 0, len(f.procs))
	for pid := range f.procs {
		out = append(out, pid)
	}
	return out
}

func (f *fakeScheme) StartedCmds() []workspaceapi.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]workspaceapi.Cmd, len(f.started))
	copy(cp, f.started)
	return cp
}

func (f *fakeScheme) Wait(pid workspaceapi.Pid, timeout time.Duration) bool {
	f.mu.Lock()
	ps := f.procs[pid]
	f.mu.Unlock()
	if ps == nil {
		return true
	}
	select {
	case <-ps.doneCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

type fakeWindow struct {
	mu        sync.Mutex
	name      string
	minimized bool
	closed    bool
	alignment component.Alignment

	stdoutBuf bytes.Buffer
	stderrBuf bytes.Buffer
	statusBuf bytes.Buffer

	minAlign component.Alignment
	cfg      browserapi.FloatingConfig
}

func (w *fakeWindow) Content() tui.Handler {
	return nil
}

func (w *fakeWindow) Unminimize() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.minimized = false
	return true
}
func (w *fakeWindow) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *fakeWindow) SetContent(browserapi.Handler) error {
	panic("unimplemeted")
}

func (w *fakeWindow) Focus() (bool, error) {
	panic("unimplemeted")
}

func (w *fakeWindow) WindowID() uint64 {
	return uint64(uintptr(unsafe.Pointer(w)))
}

func (w *fakeWindow) ID() uint64 {
	return uint64(uintptr(unsafe.Pointer(w)))
}

func (w *fakeWindow) IsFloating() bool {
	return true
}

func (w *fakeWindow) IsMinimized() (component.Alignment, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.alignment, w.minimized
}

func (w *fakeWindow) MinimizeUp(padding int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alignment = component.AlignmentTop
	w.minimized = true
	return true
}
func (w *fakeWindow) MinimizeDown(padding int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alignment = component.AlignmentBottom
	w.minimized = true
	return true
}

func (w *fakeWindow) MinimizeLeft(padding int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alignment = component.AlignmentLeft
	w.minimized = true
	return true
}
func (w *fakeWindow) MinimizeRight(padding int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alignment = component.AlignmentRight
	w.minimized = true
	return true
}
func (w *fakeWindow) SetFrameAttr(attr term.Attributes) (term.Attributes, bool) {
	return term.Attributes{}, true
}

func (w *fakeWindow) Closed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

type fakeBrowser struct {
	browser.Browser
	mu              sync.Mutex
	baseWindow      *fakeWindow
	focus           *fakeWindow
	created         []*fakeWindow
	createErr       error
	createHook      func(taskName string, cfg browserapi.FloatingConfig)
	createdHandlers []browser.ScrollableFloating
}

func newFakeBrowser() *fakeBrowser {
	base := &fakeWindow{}
	return &fakeBrowser{
		baseWindow: base,
		focus:      base,
	}
}

type fakeBrowserWindow struct {
	*fakeWindow
}

func (w fakeBrowserWindow) Content() (browserapi.Handler, error) {
	return w.fakeWindow.Content().(browserapi.Handler), nil
}

func (m *fakeBrowser) Focus() (browser.Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fakeBrowserWindow{m.focus}, nil
}

func (m *fakeBrowser) SetFocus(win browser.Window) (browser.Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev := fakeBrowserWindow{m.focus}
	m.focus = win.(fakeBrowserWindow).fakeWindow
	return prev, nil
}

func (m *fakeBrowser) PublishEvent(ev term.Event) error {
	return nil
}

func (m *fakeBrowser) RemoveTab(h browserapi.Handler) error {
	return nil
}

func (m *fakeBrowser) Floating(h browser.Floating, cfg browserapi.FloatingConfig) (
	browser.Window, error,
) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	w := &fakeWindow{cfg: cfg}
	m.mu.Lock()
	m.created = append(m.created, w)
	m.mu.Unlock()
	if m.createHook != nil {
		m.createHook("", cfg)
	}
	return fakeBrowserWindow{w}, nil
}

func (m *fakeBrowser) Created() []*fakeWindow {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*fakeWindow, len(m.created))
	copy(cp, m.created)
	return cp
}

type testEventInfo struct {
	uri workspaceapi.URI
}

func (t testEventInfo) Event() schemeapi.Event {
	return 0
}

func (t testEventInfo) URI() workspaceapi.URI {
	return t.uri
}

func (t testEventInfo) IsDir() (bool, error) {
	return false, nil
}
