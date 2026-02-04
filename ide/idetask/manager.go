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
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ernestrc/logd-go/logging"
	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/ide/plugin"
	"unstable.build/go-tui/ide/vctrl"
)

// Manager runs and manages tasks, which are processes that
// run in response to changes in the workspace. See Task for more details.
type Manager struct {
	b                Browser
	scheme           schemeapi.Scheme
	pluginOpts       []plugin.Option
	ctx              context.Context
	cancelCtx        func()
	tasks            sync.Map
	width            int
	height           int
	newPlugin        pluginBuilder
	scheduleNextTick func(func()) bool
}

// Browser extends a browser.Browser with RemoveTab.
type Browser interface {
	browser.Browser
	RemoveTab(h browserapi.Handler) error
}

// NewManager allocates storage for a new Manager and initializes it.
func NewManager(
	b Browser, scheme schemeapi.Scheme,
	scheduleNextTick func(func()) bool,
	opts ...plugin.Option,
) *Manager {
	m := new(Manager)
	m.Init(b, scheme, scheduleNextTick, opts...)
	return m
}

// Init initializes this Manager with the given browser, scheme and options.
func (m *Manager) Init(
	b Browser, scheme schemeapi.Scheme,
	scheduleNextTick func(func()) bool,
	opts ...plugin.Option,
) {
	m.b = b
	m.scheme = scheme
	m.scheduleNextTick = scheduleNextTick
	m.pluginOpts = opts
	m.ctx, m.cancelCtx = context.WithCancel(context.Background())
	m.newPlugin = func(
		publisher browser.EventPublisher, notifications browser.Notifications,
		e schemeapi.Executor, t schemeapi.Terminal, tm browser.TabManager,
		cmdAndArgs []string, maxWidth int, opts ...plugin.Option,
	) (browser.ScrollableFloating, error) {
		return plugin.New(publisher, notifications, e, t, tm, cmdAndArgs, maxWidth, opts...)
	}
}

// SetMaxWidthHeight is used to ensure that a task's initial VTE is resized
// to an approppiate initial width and height.
func (m *Manager) SetMaxWidthHeight(width, height int) {
	m.width = width
	m.height = height
	m.tasks.Range(func(k, v any) bool {
		v.(*Task).setMaxWidthHeight(width, height)
		return true
	})
}

// FocusTask switches the window manager's focus to the tasks window or tab.
func (m *Manager) FocusTask(name string) bool {
	taskIfc, loaded := m.tasks.Load(name)
	if !loaded {
		return false
	}
	t := taskIfc.(*Task)
	win := t.win
	if win.Closed() {
		var ok bool
		win, ok = t.tab.Window()
		if !ok || win.Closed() {
			return false
		}
	}
	_, err := t.b.SetFocus(win)
	return err == nil
}

// RunTask runs a task in the background and creates a minimized floating window
// that displays the status of the task. When a Task's window is un-minimized,
// the full stdout and stderr of the task can be visualized.
func (m *Manager) RunTask(t Task) error {
	validateTask(t)
	if _, loaded := m.tasks.LoadOrStore(t.Name, &t); loaded {
		return ErrTaskExists
	}
	ch := make(chan schemeapi.EventInfo)
	id, err := m.scheme.Watch("./...", ch, schemeapi.AllEvents()...)
	if err != nil {
		m.tasks.Delete(t.Name)
		return fmt.Errorf("workspace watch: %w", err)
	}
	ctx, cancel, err := t.init(id, m.ctx, m.b, m.scheme,
		m.newPlugin, m.width, m.height, func() {
			m.tasks.Delete(t.Name)
		}, m.scheduleNextTick, m.pluginOpts...)
	if err != nil {
		_ = m.scheme.StopWatch(id)
		m.tasks.Delete(t.Name)
		return fmt.Errorf("init task: %w", err)
	}

	go debug.CapturePanicReport(func() {
		defer m.scheme.StopWatch(id) //nolint:errcheck
		defer cancel()
		ignore, err := vctrl.LoadGitignore(m.scheme)
		if err != nil {
			m.log(log.ErrorLevel, "load gitignore: %v", err)
			ignore = vctrl.NopMatcher(false)
		}
		matcher := vctrl.NopMatcher(true)
		var filters []gitignore.Pattern
		if t.Filter != "" {
			filtersStr := strings.Split(t.Filter, ",")
			filters = make([]gitignore.Pattern, len(filtersStr))
			for i, filter := range filtersStr {
				filters[i] = gitignore.ParsePattern(filter, nil)
			}
			matcher, err = vctrl.MatcherFromPatterns(m.scheme, filters...)
			if err != nil {
				matcher = vctrl.NopMatcher(true)
				m.log(log.ErrorLevel, "new matcher: %v", err)
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				isDir, _ := ev.IsDir()
				if ignore.Match(ev.URI(), isDir) {
					m.log(log.TraceLevel, "ignoring %s due to gitignore", ev.URI().Path())
					continue
				}

				if !matcher.Match(ev.URI(), isDir) {
					m.log(log.TraceLevel, "ignoring %s due to not matching filters: %v", ev.URI().Path(), filters)
					continue
				}

				var icon string
				switch ev.Event() {
				case schemeapi.Create:
					icon = " "
				case schemeapi.Rename:
					fallthrough // files are flushed by means of renaming them
				case schemeapi.Write:
					icon = " "
				case schemeapi.Remove:
					icon = " "
				}

				t.tryRunning(m.b, m.scheme, icon+ev.URI().Name())
			}
		}
	})

	return nil
}

// ListTasks creates a floating window that displays
// all the running tasks and its status details.
func (m *Manager) ListTasks() (ret []TaskInfo) {
	m.tasks.Range(func(k, v any) bool {
		ret = append(ret, v.(*Task).Info())
		return true
	})
	return
}

// StopTask stops the task with the given name or returns
// an error if the task doesn't exist or there was an error stopping
// it.
func (m *Manager) StopTask(name string) (err error) {
	info, ok := m.tasks.LoadAndDelete(name)
	if !ok {
		return errors.New("task with this name does not exist")
	}
	t := info.(*Task)
	t.doClose()
	<-info.(*Task).doneWaitCh
	if t.tab != nil {
		err = t.b.RemoveTab(t.tab)
	}
	return err
}

// ReplaceTask replaces the command of the given task and attempts to
// run the task.
func (m *Manager) ReplaceTask(name string, cmd string, args ...string) error {
	info, ok := m.tasks.Load(name)
	if !ok {
		return errors.New("task with this name does not exist")
	}
	task := info.(*Task)
	task.mu.Lock()
	task.Cmd = cmd
	task.Args = args
	task.cmdAndArgs = append([]string{task.Cmd}, task.Args...)
	task.mu.Unlock()
	task.tryRunning(m.b, m.scheme, "  task")
	return nil
}

// OnFocus satisfies handler.WindowSubscriber.
func (m *Manager) OnFocus(prevFocus, newFocus handler.Window) {
	m.onFocus(prevFocus, newFocus)
}

// Close stops all tasks and closes this Manager's resources.
func (m *Manager) Close() error {
	m.cancelCtx()
	return nil
}

// allows calling it directly in tests with fake windows
type window interface {
	ID() uint64
	Content() tui.Handler
}

func (m *Manager) onFocus(prevFocus, newFocus window) {
	m.tasks.Range(func(k, v any) bool {
		task := v.(*Task)
		winID := task.win.WindowID()
		if winID == prevFocus.ID() {
			task.onUnfocus()
		} else if winID == newFocus.ID() {
			task.onFocus()
		}
		return true
	})
	t, ok := newFocus.Content().(*browser.Tab)
	if !ok {
		return
	}
	task, ok := t.Handler().(*Task)
	if !ok {
		return
	}
	if task.tab != nil {
		t.ResetAttrs()
	}
}

func (m *Manager) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "idetask.Manager",
	}).Logf(level, msg, args...)
}

func validateTask(t Task) {
	if t.Cmd == "" {
		panic("task command must not be empty")
	}
	if t.Name == "" {
		panic("task name must not be empty")
	}
}
