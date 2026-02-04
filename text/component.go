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

package text

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	shsyntax "mvdan.cc/sh/v3/syntax"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/syntax"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/walkdir"
)

var _ tui.Component = (*Component)(nil)
var _ browser.Browser = (*Component)(nil)
var _ Editor = (*Component)(nil)

// Workspace abstracts the workspace functionality needed for a Component.
type Workspace interface {
	workspace.Loader
	walkdir.Reader
}

// Component is an implementation of browser.Browser for file editing.
// It also satisfies tui.Component, and text.Editor.
type Component struct {
	ctx            context.Context
	cancelCtx      func()
	comp           browser.Component
	workspace      Workspace
	ed             Editor
	config         Config
	focus          handler.Window
	edSubscribers  map[textapi.EventType][]EventHandler
	cmdSubscribers map[string]commandAll
	editors        map[string]Handler
	fileRegistry   FileCommandRegistry
}

// NewComponent allocates storage for a new Component and initializes it.
func NewComponent(ed Editor, w Workspace, config Config) (
	c *Component, err error,
) {
	c = new(Component)
	err = c.Init(ed, w, config)
	if err != nil {
		return
	}
	return
}

func (c *Component) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "text.Component").Logf(level, msg, args...)
}

func (c *Component) resetTabProperties(file workspaceapi.URI) {
	c.comp.ResetTabNameAndAttrs(file)
}

func (c *Component) setDirtyFileAttr(file workspaceapi.URI, buf *cell.Buffer, lastFlush int) {
	v := buf.Version()
	c.log(log.TraceLevel, "edited, snapshot is %d, buffer version is %d", lastFlush, v)
	if v == lastFlush {
		c.resetTabProperties(file)
		return
	}
	if _, defName, ok := c.comp.TabName(file); ok {
		tabname := fmt.Sprintf("%s*", defName)
		c.comp.SetTabNameAndAttrs(file, tabname, c.config.DirtyTabAttr)
	}
}

func (c *Component) getSwapDir(file workspaceapi.URI) (workspaceapi.URI, error) {
	return workspace.DefaultSwapDirectory(file)
}

func (c *Component) newFileBuffer(
	file, recSwapFile workspaceapi.URI, buf *cell.Buffer,
	readOnly, forceRecover bool,
) (handler Handler, ret *editorFlusherCloser, err error) {
	recover := recSwapFile != (workspaceapi.URI{})
	var fc workspace.FlusherCloser
	if recover {
		fc, err = c.workspace.Recover(file, recSwapFile, buf, forceRecover)
	} else {
		var swapDir workspaceapi.URI
		swapDir, err = c.getSwapDir(file)
		if err == nil {
			fc, err = c.workspace.Load(file, buf, swapDir, readOnly)
		}
	}
	if err != nil {
		return nil, nil, err
	}

	interrupter := browser.EventPublisherInterrupter(c)
	locs := syntax.FuncLocationSetter(func(ll textapi.LocationList) {
		handler.SetLocationList(textapi.LocationPriorityInfo, "syntax", ll)
	})

	// install tree in Buffer first so editor can use
	// its capabilities while initializing

	tree := syntax.WithTree(c.ctx, c.config, interrupter,
		c.config.PkgManager, locs, file, buf, fc, c.workspace, c.config.Syntax)
	fc = tree

	handler, err = c.ed.Edit(file, buf, readOnly, recover)
	if err != nil {
		return nil, nil, err
	}

	commands, cmdHandler := syntax.Commands(handler, tree)
	for _, cmd := range commands {
		err := c.fileRegistry.SubscribeCommandForFile(file, cmd, cmdHandler)
		if err != nil {
			return nil, nil, err
		}
	}

	efc := &editorFlusherCloser{
		parent:    c,
		fc:        fc,
		uri:       file,
		buf:       buf,
		lastFlush: buf.Version(),
		h:         handler,
		commands:  commands,
	}

	// no need to unsubscribe upon Close since the assumption
	// is that a Component always outlives a cell.Buffer
	buf.Subscribe(efc)

	return handler, efc, nil
}

// Init initializes this Component with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (c *Component) Init(
	ed Editor, w Workspace, config Config,
) error {
	c.config = config
	c.ctx, c.cancelCtx = context.WithCancel(context.Background())

	c.comp.Init(c.config.Config)
	c.comp.Subscribe((*handlerWindowSubscriber)(c))
	c.fileRegistry = newFileCommandRegistryFromComponent(c)

	c.ed = ed
	c.workspace = w
	c.edSubscribers = make(map[textapi.EventType][]EventHandler)
	c.cmdSubscribers = make(map[string]commandAll)
	c.editors = make(map[string]Handler)

	// validate that config aliases are not recursive
	return ValidateCommandAliases(c.config.CommandAliases)
}

func (c *Component) tryDispatchEventFocus(win handler.Window) {
	content := win.Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return
	}
	res, ok := t.Handler().(Handler)
	if !ok {
		return
	}
	cursor := res.CursorAtScroll()
	(*Component)(c).DispatchEvent(textapi.Event{
		Type:     textapi.EventTypeFocus,
		URI:      t.URI(),
		Resource: res,
		Start:    c.getContentDimensions(win),
		From:     cursor,
	})
}

func (c *Component) getContentDimensions(win handler.Window) term.Coordinates {
	width := win.Width()
	height := win.Height()
	if c.config.WindowManagerConfig.Frame {
		width = max(0, width-2)
		height = max(0, height-2)
	}
	return term.Coordinates{X: width, Y: height}
}

func (c *Component) tryDispatchEvent(win handler.Window, evType textapi.EventType) {
	content := win.Content()
	t, ok := content.(*browser.Tab)
	if !ok {
		return
	}
	res, ok := t.Handler().(Handler)
	if !ok {
		return
	}
	(*Component)(c).DispatchEvent(textapi.Event{
		Type:     evType,
		URI:      t.URI(),
		Resource: res,
	})
}

type compTabSubscriber struct {
	parent *Component
	window browser.Window
}

func (s *compTabSubscriber) OnFocus(t *browser.Tab) {
	res, ok := t.Handler().(Handler)
	if !ok {
		return
	}
	w, ok := t.Window()
	if !ok {
		// this should never happen, given that we just received OnFocus
		return
	}
	s.window = w
	if ok, err := w.Focus(); err != nil || !ok {
		// tab has been assigned to window but window
		// is not main window
		return
	}
	dimensions := s.parent.getContentDimensions(s.parent.focus)
	cursor := res.CursorAtScroll()
	s.parent.DispatchEvent(textapi.Event{
		Type:     textapi.EventTypeFocus,
		URI:      t.URI(),
		Resource: res,
		Start:    dimensions,
		From:     cursor,
	})
}

func (s *compTabSubscriber) OnFree(t *browser.Tab) {
	res, ok := t.Handler().(Handler)
	if !ok {
		return
	}
	if s.window == nil {
		// should not happen, if OnFree is called
		// OnFocus should have been called before
		return
	}
	if ok, err := s.window.Focus(); err != nil || !ok {
		return
	}
	s.parent.DispatchEvent(textapi.Event{
		Type:     textapi.EventTypeUnfocus,
		URI:      t.URI(),
		Resource: res,
	})
}

// OpenFileTab opens the file at filename path, as a new browser tab.
// It's up to the caller to use the returned browserapi.Handler and switch
// any of the active windows to use it.
//
// If recoveryFilename is not empty, then the file will be recovered from the
// contents of recoveryFilename.
func (c *Component) OpenFileTab(file workspaceapi.URI, readOnly bool) (
	browserapi.Handler, error,
) {
	if file == (workspaceapi.URI{}) {
		return nil, errors.New("empty URI")
	}
	return c.openFileTab(file, workspaceapi.URI{}, readOnly, false)
}

// Reload reloads the content of the tab at the given window. If the content
// is not a tab, then this method returns an error.
func (c *Component) Reload(win browser.Window) error {
	content, err := win.Content()
	if err != nil {
		return fmt.Errorf("get window content: %w", err)
	}
	t, ok := content.(*browser.Tab)
	if !ok {
		return textapi.ErrInvalidReload
	}

	return c.ReloadTab(t)
}

// ReloadTab reloads the given handler, if it is a tab,
// and if it can be reloaded.
func (c *Component) ReloadTab(h browserapi.Handler) error {
	t, ok := h.(*browser.Tab)
	if !ok {
		return textapi.ErrInvalidReload
	}

	if t.Closer() == nil {
		return textapi.ErrInvalidReload
	}

	fc, ok := t.Closer().(workspace.FlusherCloser)
	if !ok {
		return textapi.ErrInvalidReload
	}
	err := fc.Reload()
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	return nil
}

// RemoveTab removes the given handler, if it is a tab,
// and if it can be removed.
func (c *Component) RemoveTab(h browserapi.Handler) error {
	ok := c.comp.RemoveTab(h)
	if !ok {
		return errors.New("content is not a tab")
	}
	return nil
}

// Overwrite overwrites the content of the tab at the given window. If the content
// is not a tab, then this method returns an error.
func (c *Component) Overwrite(win browser.Window) error {
	content, err := win.Content()
	if err != nil {
		return fmt.Errorf("get window content: %w", err)
	}
	t, ok := content.(*browser.Tab)
	if !ok {
		return textapi.ErrInvalidReload
	}

	return c.OverwriteTab(t)
}

// OverwriteTab overwrites the given handler from persistence, if it is a tab,
// and if it can be overwritten.
func (c *Component) OverwriteTab(h browserapi.Handler) error {
	t, ok := h.(*browser.Tab)
	if !ok {
		return textapi.ErrInvalidOverwrite
	}

	if t.Closer() == nil {
		return textapi.ErrInvalidOverwrite
	}

	fc, ok := t.Closer().(workspace.FlusherCloser)
	if !ok {
		return textapi.ErrInvalidOverwrite
	}

	return fc.ForceFlush()
}

// recoverOpenFileTab recovers the file at filename by using the file at recoverFilename
// and opens a tab it like OpenFileTab. See OpenFileTab for more details.
func (c *Component) recoverOpenFileTab(
	file workspaceapi.URI, recoveryFilename workspaceapi.URI, readOnly bool,
) (browserapi.Handler, error) {
	if file == (workspaceapi.URI{}) || recoveryFilename == (workspaceapi.URI{}) {
		return nil, errors.New("empty URI")
	}
	return c.openFileTab(file, recoveryFilename, readOnly, false)
}

func (c *Component) openFileTab(
	file workspaceapi.URI, recoveryFilename workspaceapi.URI,
	readOnly, forceRecover bool,
) (browserapi.Handler, error) {
	t, ok := c.comp.Tab(file)
	if ok {
		return t, nil
	}

	buf := cell.NewBuffer()
	handler, fc, err := c.newFileBuffer(file, recoveryFilename, buf, readOnly, forceRecover)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(file.Name())
	icon, ok := c.config.Icons.Extensions[ext]
	if !ok {
		icon = c.config.Icons.Default
	}
	t = c.newTab(file, icon, file.Name(), handler, fc)
	return t, nil
}

// Open opens the given file in a new browser tab. If file is already
// open by another session or the last edit session crashed, it
// will create a prompt for the user to decide what to do.
func (c *Component) Open(file workspaceapi.URI) (browserapi.Handler, error) {
	h, err := c.OpenFileTab(file, false)
	if err != nil && err == workspaceapi.ErrFileAlreadyOpen {
		c.openRecoveryPrompt(file)
	}
	return h, err
}

// OpenReadOnly is like Open but opens the file in read-only mode (cannot be saved).
func (c *Component) OpenReadOnly(file workspaceapi.URI) (browserapi.Handler, error) {
	h, err := c.OpenFileTab(file, true)
	if err != nil && err == workspaceapi.ErrFileAlreadyOpen {
		c.openRecoveryPrompt(file)
	}
	return h, err
}

// ReadFile injects the contents of the desired file and writes it under the cursor.
func (c *Component) ReadFile(file workspaceapi.URI, h Handler) error {
	if ed, ok := h.(wrapEditor); ok {
		h = ed.Handler
	}

	var swapDir workspaceapi.URI

	swapDir, err := c.getSwapDir(file)
	if err != nil {
		return err
	}

	// dump the file contents into a buffer we can read from
	buf := cell.NewBuffer()
	_, err = c.workspace.Load(file, buf, swapDir, true)
	if err != nil {
		return err
	}

	// get the current cursor position to insert to, which will be the line below
	coords := h.CursorAtScroll()
	coords.X = 0
	coords.Y += 1

	// inject the file contents surrounded by newlines to emulate vim's behaviour
	// we don't prefix with \n because we already moved the cursor at a point that
	// follows a \n (coords.X = 0; coords.Y += 1).
	cellEditor := h.CellEditor()
	_, _, _ = cellEditor.Edit(
		context.Background(), coords, coords, fmt.Sprintf("%s\n", buf),
	)
	return err
}

// Editor satisfies Editor interface.
func (c *Component) Editor(resource workspaceapi.URI) (Handler, error) {
	for _, tab := range c.comp.Tabs() {
		if tab.URI().String() == resource.String() {
			h, ok := tab.Handler().(Handler)
			if !ok {
				continue
			}
			return h, nil
		}
	}
	// ensure that non-tab handlers returned by Handler
	// can also be returned with Editor.
	for _, ed := range c.editors {
		if ed.Resource().String() == resource.String() {
			return ed, nil
		}
	}
	return nil, errors.New("handler not found")
}

// CommandKeyBinding returns a command that was mapped to the given key
// combination and true or an empty string and false if there was
// no command mapped to the given key.
func (c *Component) CommandKeyBinding(key term.KeyComb) ([][]string, bool) {
	cmd, ok := c.config.CommandKeyBindings[key]
	c.log(log.TraceLevel, "command key binding for %#v: %s", key, cmd)
	return cmd, ok
}

// CompleteCommand calls the command's completer with the given args and returns
// an interator with the possible argument completions.
func (c *Component) CompleteCommand(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	if a, ok := c.config.CommandAliases[cmd.Name]; ok {
		if a.Completer == nil {
			return iterator.FromSlice[string](nil), "", nil
		}
		return a.Completer(c).
			Complete(ctx, append([]string{cmd.Name}, cmd.Args...))
	}

	man, ok := c.cmdSubscribers[cmd.Name]
	if !ok {
		c.log(log.DebugLevel, "complete command %q: no subscribers", cmd)
		return iterator.FromSlice[string](nil), "", nil
	}

	return man.handler.Complete(ctx, cmd)
}

// handle escaped dollar signs by susbstituting for an extremely
// rare string that couldn't possible be included in a command
const impossibleMark = ""

func (c *Component) replacePositionalArgs(
	alias CommandAlias, dispatched textapi.Command,
) (CommandAlias, textapi.Command, error) {
	// clone so we don't mess with originals
	cmds := alias.Commands
	alias.Commands = make([]string, len(cmds))
	copy(alias.Commands, cmds)
	argsReplaced := make(map[int]struct{})
	for j, cmd := range alias.Commands {
		cmd = strings.ReplaceAll(cmd, "$$", impossibleMark)
		for i, arg := range []string{"$1", "$2", "$3", "$4", "$5", "$6", "$7", "$8", "$9"} {
			replace := i
			if replace >= len(dispatched.Args) {
				break
			}
			old := cmd
			cmd = strings.ReplaceAll(cmd, arg, dispatched.Args[replace])
			if old != cmd {
				argsReplaced[replace] = struct{}{}
			}
		}
		reworked := make([]string, 0, len(dispatched.Args))
		for i, arg := range dispatched.Args {
			if _, ok := argsReplaced[i]; !ok {
				reworked = append(reworked, arg)
			}
		}
		dispatched.Args = reworked
		cmd = strings.ReplaceAll(cmd, impossibleMark, "$")
		alias.Commands[j] = cmd
	}
	return alias, dispatched, nil
}

// DispatchCommand dispatches a EventTypeCommand with cmd to subscribers
// subscribed via SubscribeEvents.
func (c *Component) DispatchCommand(cmd textapi.Command) (handled bool, err error) {
	if cmd.Window == nil {
		panic("invalid command: missing Window from which command was invoked")
	}

	// replace % with current open file, before shell expansion
	content, _ := c.comp.Focus().Content()
	th, ok := content.(*browser.Tab)
	if ok {
		for i, arg := range cmd.Args {
			arg = strings.ReplaceAll(arg, "%%", impossibleMark)
			arg = strings.ReplaceAll(arg, "%", th.URI().Path())
			cmd.Args[i] = strings.ReplaceAll(arg, impossibleMark, "%")
		}
	}
	// best effort attempt to group arguments in single/double quotes, etc.
	args, err := regroupArgs(strings.Join(cmd.Args, " "))
	if err != nil {
		c.log(log.DebugLevel, "could not group command arguments: %v", err)
		args = cmd.Args
	}
	cmd.Args = args
	targets, ok := c.config.CommandAliases[cmd.Name]
	if ok {
		c.log(log.DebugLevel, "Dispatching alias %s: %#v", cmd.Name, targets)
		targets, cmd, err = c.replacePositionalArgs(targets, cmd)
		if err != nil {
			return
		}
		c.log(log.TraceLevel, "replaced positional args: %#v, cmd: %#v", targets, cmd)
		for _, target := range targets.Commands {
			argv := strings.Split(target, " ")
			targetCmd := textapi.Command{
				Name:     argv[0],
				Args:     append(argv[1:], cmd.Args...),
				URI:      cmd.URI,
				Resource: cmd.Resource,
				Window:   cmd.Window,
				Cursor:   cmd.Cursor,
			}
			// re-use re-expansion logic or call to another alias
			targetHandled, targetErr := c.DispatchCommand(targetCmd)
			if targetErr != nil {
				return targetHandled, fmt.Errorf("%s: %s", target, targetErr)
			}
			handled = handled || targetHandled
		}
		return handled, nil
	}
	man, ok := c.cmdSubscribers[cmd.Name]
	if !ok {
		c.log(log.DebugLevel, "Dispatching command %q: no subscribers", cmd.Name)
		return false, nil
	}
	c.log(log.DebugLevel, "Dispatching command %q with args %v", cmd.Name, cmd.Args)
	err = man.handler.HandleCommand(context.Background(), cmd)
	if err != nil {
		return true, err
	}
	return true, nil
}

func (c *Component) dispatchFlush(file workspaceapi.URI, h Handler) error {
	content := c.getContent(h)

	// clear dirty/flushed attributes
	c.resetTabProperties(file)

	ev := textapi.Event{
		Type:     textapi.EventTypeFlush,
		URI:      file,
		Resource: h,
		Content:  content,
	}
	c.DispatchEvent(ev)
	return nil
}

// DispatchEvent dispatches the given event to subscribers.
func (c *Component) DispatchEvent(ev textapi.Event) (handled bool) {
	subs, ok := c.edSubscribers[ev.Type]
	if !ok || len(subs) == 0 {
		return
	}

	c.edSubscribers[ev.Type] = make([]EventHandler, 0, len(subs))
	for _, h := range subs {
		exit := h.Handle(context.Background(), ev)
		if !exit {
			c.edSubscribers[ev.Type] = append(c.edSubscribers[ev.Type], h)
		}
	}

	handled = true
	return
}

// Notify satisfies browser.Browser.
func (c *Component) Notify(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	return c.config.Notify(level, fmt.Sprintf(msg, args...))
}

// NotifyOnce satisfies browser.Browser.
func (c *Component) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	return c.config.NotifyOnce(level, msg, args...)
}

// UpdateNotificationProgress satisfies browser.Browser.
func (c *Component) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return c.config.UpdateNotificationProgress(id, message, progress, total)
}

// Split satisfies browser.WindowManager.
func (c *Component) Split(
	o browserapi.Orientation, win browser.Window, h browserapi.Handler,
) (browser.Window, error) {
	w, ok := c.comp.Split(o, win, h)
	if !ok {
		return nil, textapi.ErrInvalidSplit
	}
	return w, nil
}

// Bar creates a new status bar with h's component and delegates handling of mouse events to h.
func (c *Component) Bar(cfg browserapi.BarConfig, h tui.Handler) error {
	if cfg.Size <= 0 {
		return fmt.Errorf("invalid bar size: %d", cfg.Size)
	}
	c.comp.Bar(cfg, h)
	return nil
}

// PublishEvent sends an event to the main event loop which
// forces Handle to be called on the tui.Handler in focus.
func (c *Component) PublishEvent(ev term.Event) error {
	ok := c.config.EventPublisher(ev)
	if !ok {
		return errors.New("event stream not ready")
	}
	return nil
}

// Focus returns the current window in focus. It satisfies browser.Browser.
func (c *Component) Focus() (browser.Window, error) {
	return c.comp.Focus(), nil
}

// FocusTab returns the Tab corresponding to the current window in focus,
// or false if the current window in focus is not drawing a Tab.
func (c *Component) FocusTab() (*browser.Tab, bool) {
	return c.comp.FocusTab()
}

// SetFocus sets the window in focus and returns the previous window in focus.
// It satisfies browser.Browser.
func (c *Component) SetFocus(win browser.Window) (browser.Window, error) {
	return c.comp.SetFocus(win), nil
}

// Edit edits the resource with name and buffer
// with the underlying Editor in a new browser buffer.
//
// NOTE: Returned Handler has no EventTypeFocus support.
//
// Deprecated: Use OpenFileTab instead.
func (c *Component) Edit(
	file workspaceapi.URI, buf *cell.Buffer, readOnly, recover bool,
) (Handler, error) {
	editor, err := c.ed.Edit(file, buf, readOnly, recover)
	if err != nil {
		return nil, err
	}
	editor = wrapEditor{parent: c, Handler: editor}
	c.editors[file.String()] = editor
	return editor, nil
}

// Flush flushes the contents of the tab at the given window,
// if there's one.
func (c *Component) Flush(win browser.Window) error {
	content, err := win.Content()
	if err != nil {
		return fmt.Errorf("get window content: %w", err)
	}
	return c.FlushTab(content)
}

// ForceFlush flushes the contents of the tab at the given window,
// if there's one, overriding read-only mode, and ignoring
// stale data errors and others.
func (c *Component) ForceFlush(win browser.Window) error {
	content, err := win.Content()
	if err != nil {
		return fmt.Errorf("get window content: %w", err)
	}
	return c.ForceFlushTab(content)
}

// FlushTab flushes the contents of the given handler, if it is a tab.
func (c *Component) FlushTab(h browserapi.Handler) error {
	t, ok := h.(*browser.Tab)
	if !ok || t.Closer() == nil {
		return textapi.ErrInvalidSave
	}

	fc, ok := t.Closer().(workspace.FlusherCloser)
	if !ok {
		return textapi.ErrInvalidSave
	}

	err := fc.Flush()
	if err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}

// ForceFlushTab force-flushes the contents of the given handler, if it is a tab,
// overriding read-only mode, and ignoring stale data errors and others.
func (c *Component) ForceFlushTab(h browserapi.Handler) error {
	t, ok := h.(*browser.Tab)
	if !ok || t.Closer() == nil {
		return textapi.ErrInvalidSave
	}

	fc, ok := t.Closer().(workspace.FlusherCloser)
	if !ok {
		return textapi.ErrInvalidSave
	}

	err := fc.ForceFlush()
	if err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}

// LastFlush returns the last time this handler was flushed, if it is a valid
// tab that can be flushed. If the tab has not yet been flushed, then
// the returned time will be a zero time.Time value.
func (c *Component) LastFlush(h browserapi.Handler) (time.Time, error) {
	t, ok := h.(*browser.Tab)
	if !ok || t.Closer() == nil {
		return time.Time{}, errors.New("content is not a tab")
	}

	fc, ok := t.Closer().(workspace.FlusherCloser)
	if !ok {
		return time.Time{}, errors.New("content is not a tab that can be flushed")
	}

	return fc.LastFlush(), nil
}

func (c *Component) getContent(h Handler) string {
	cells := h.CellView().RawCells()
	return term.CellsToString(cells)
}

func (c *Component) dispatchOpenUponSubscribe(h EventHandler) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tab := range c.comp.Tabs() {
		resHandler, ok := tab.Handler().(Handler)
		if !ok {
			// tab handler does not implement Handler
			continue
		}
		str := c.getContent(resHandler)
		exit := h.Handle(ctx, textapi.Event{
			Type:     textapi.EventTypeOpen,
			URI:      tab.URI(),
			Resource: resHandler,
			Content:  str,
		})
		if exit {
			return true
		}
	}
	return false
}

func (c *Component) dispatchFocusUponSubscribe(h EventHandler) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t, ok := c.comp.FocusTab()
	if ok {
		dimensions := c.getContentDimensions(c.focus)
		resHandler, ok := t.Handler().(Handler)
		if ok {
			ev := textapi.Event{
				Type:     textapi.EventTypeFocus,
				URI:      t.URI(),
				Resource: resHandler,
				Start:    dimensions,
				From:     resHandler.CursorAtScroll(),
			}
			return h.Handle(ctx, ev)
		}
	}
	return false
}

// SubscribeEvents subscribes h to editor events of type ev.
// If ev is of type EventTypeOpen, an event will be dispatched for
// every Tab currently open.
func (c *Component) SubscribeEvents(evs []textapi.EventType, h EventHandler) error {
	c.log(log.TraceLevel, "subscribe sub=%p: evs=%v", h, evs)
	// iterate to dispatch immediate events
	for _, ev := range evs {
		switch ev {
		case textapi.EventTypeOpen:
			exit := c.dispatchOpenUponSubscribe(h)
			if exit {
				c.log(log.DebugLevel, "not subscribe sub=%p: open: exit=true", h)
				return nil
			}
		case textapi.EventTypeFocus:
			exit := c.dispatchFocusUponSubscribe(h)
			if exit {
				c.log(log.DebugLevel, "not subscribe sub=%p: focus: exit=true", h)
				return nil
			}
		}
	}

	// iterate again so if handler exited for any event, we have returned
	// and we do not subscribe it
	var delegated []textapi.EventType
	for _, ev := range evs {
		switch ev {
		// delegate certain event dispatching to underlying editor.
		case textapi.EventTypeOpen, textapi.EventTypeEdit,
			textapi.EventTypeScroll, textapi.EventTypeCursor,
			textapi.EventTypeSelection, textapi.EventTypeHidden,
			textapi.EventTypeVisible:
			delegated = append(delegated, ev)
		default:
			if _, ok := c.edSubscribers[ev]; !ok {
				c.edSubscribers[ev] = make([]EventHandler, 0, 1)
			}
			c.edSubscribers[ev] = append(c.edSubscribers[ev], h)
		}
	}

	return c.ed.SubscribeEvents(delegated, h)
}

// UnsubscribeEvents unsubscribes sub from all events.
func (c *Component) UnsubscribeEvents(sub EventHandler) (ret bool, err error) {
	c.log(log.TraceLevel, "unsubscribe sub=%p", sub)
	final := make(map[textapi.EventType][]EventHandler)
	for ev, subs := range c.edSubscribers {
		final[ev] = make([]EventHandler, 0, len(subs))
		for _, s := range subs {
			if s != sub {
				final[ev] = append(final[ev], s)
			} else {
				ret = true
			}
		}
	}
	c.edSubscribers = final
	var ed bool
	ed, err = c.ed.UnsubscribeEvents(sub)
	if err != nil {
		return
	}
	return ret || ed, nil
}

// Commands returns a list of commands registered via SubscribeCommand
// or via Config.CommandAliases.
func (c *Component) Commands() (ret []command.Manual) {
	ret = make([]command.Manual, len(c.cmdSubscribers))
	var i int
	for _, cmd := range c.cmdSubscribers {
		ret[i] = cmd.man
		i++
	}
	for alias, aliasOf := range c.config.CommandAliases {
		ret = append(ret, command.Manual{
			Name:    alias,
			AliasOf: aliasOf.Commands,
		})
	}
	return ret
}

// SubscribeCommand installs cm as a command handler of cmd or returns
// an error if there's already a CommandHandler installed for this cmd.
func (c *Component) SubscribeCommand(cmd textapi.CommandManual, cm CommandHandler) error {
	if _, ok := c.cmdSubscribers[cmd.Name]; ok {
		return errors.New("command already registered")
	}

	c.cmdSubscribers[cmd.Name] = commandAll{
		man:     apiManualToManual(cmd),
		handler: cm,
	}
	return nil
}

// UnsubscribeCommand un-registers command.
func (c *Component) UnsubscribeCommand(cmd string) error {
	if _, ok := c.cmdSubscribers[cmd]; !ok {
		return errors.New("command not registered")
	}
	delete(c.cmdSubscribers, cmd)
	return nil
}

// Browser returns this Component's underlying browser.Component.
func (c *Component) Browser() *browser.Component {
	return &c.comp
}

// Resize satisfies tui.Component.
func (c *Component) Resize(width, height int) {
	c.comp.Resize(width, height)
	if c.focus != (handler.Window{}) {
		c.tryDispatchEventFocus(c.focus)
	}
}

// Draw satisfies tui.Component.
func (c *Component) Draw(w term.Writer) {
	c.comp.Draw(w)
}

// Handle delegates events to the underlying browser.Component.
func (c *Component) Handle(ev term.Event) (exit, handled bool) {
	return c.comp.Handle(ev)
}

// SetDim sets whether next call to draw should use
// non-focus window diming feature.
func (c *Component) SetDim(to bool) bool {
	return c.comp.SetDim(to)
}

// WindowManagerPosition returns the offset from the top left corner
// where the underlying window manager starts.
func (c *Component) WindowManagerPosition() term.Coordinates {
	return c.comp.WindowManagerPosition()
}

// WindowManagerSize returns the size of the window manager.
func (c *Component) WindowManagerSize() (width, height int) {
	return c.comp.WindowManagerSize()
}

// Floating satisfies browser.WindowManager.
func (c *Component) Floating(
	h browser.Floating, cfg browserapi.FloatingConfig,
) (browser.Window, error) {
	return c.comp.Floating(h, cfg), nil
}

// Tab satisfies browser.WindowManager.
func (c *Component) Tab(
	resource workspaceapi.URI, icon rune, name string, h browserapi.Handler,
) (browserapi.Handler, error) {
	if _, ok := h.(Handler); ok {
		panic("handler must not be a text.Handler. " +
			// Otherwise events might be inconsistently
			// delivered: i.e. EventTypeFocus/Unfocus events
			// will be delievered for text.Handler
			// that have not had EventTypeOpen delivered.
			"Use OpenFileTab if attempting to create a tab with the return value of Edit")
	}

	t, ok := c.comp.Tab(resource)
	if ok {
		return t, nil
	}

	t = c.newTab(resource, icon, name, h, nil)
	return t, nil
}

// Resource returns an open resource or false if resource with uri is not open.
func (c *Component) Resource(uri workspaceapi.URI) (browserapi.Handler, bool) {
	return c.comp.Tab(uri)
}

// IsDirty returns whether the given tab has unflushed changes.
func (c *Component) IsDirty(uri workspaceapi.URI) (dirty bool, ok bool) {
	attrs, ok := c.comp.TabAttrs(uri)
	if !ok {
		return
	}
	dirty = attrs == c.config.DirtyTabAttr
	ok = true
	return
}

// Window returns the window with the given id and true or nil and false if no
// window with the given id could be found in the underlying browser.
func (c *Component) Window(id uint64) (browser.Window, bool) {
	return c.comp.Window(id)
}

// Tabs returns the tabs open in this browser.Component.
func (c *Component) Tabs() []*browser.Tab {
	return c.comp.Tabs()
}

// SetTabName sets the title and attributes of the title of the given tab.
// If the given browserapi.Handler is not a tab, then this method returns an error.
func (c *Component) SetTabName(uri workspaceapi.URI, title string, attr term.Attributes) error {
	ok := c.comp.SetTabDefaultNameAndAttrs(uri, title, attr)
	if !ok {
		return errors.New("set title called on unknown tab")
	}
	_ = c.comp.SetTabNameAndAttrs(uri, title, attr)
	return nil
}

// Prompt creates a new prompt to be drawn as an overlay on the next call to Draw
// and it also takes over event control until user either exits prompt or selects
// an option.
func (c *Component) Prompt(
	message string, options []string,
	bindings []term.KeyComb,
	promptHandler handler.PromptHandler,
) browser.Window {
	return c.comp.Prompt(message, options, bindings, promptHandler)
}

// SubscribeWindow subscribes sub to changes in focus due to changing the window in focus.
func (c *Component) SubscribeWindow(sub handler.WindowSubscriber) {
	c.comp.Subscribe(sub)
}

// Workspace returns the workspace used by this Component.
func (c *Component) Workspace() Workspace {
	return c.workspace
}

// Close closes all resources associated with this Component.
func (c *Component) Close() error {
	// avoid dispatching close events on flusherCloser callbacks
	c.edSubscribers = make(map[textapi.EventType][]EventHandler)
	c.cancelCtx()

	err := c.comp.Close()
	if err != nil {
		c.log(log.ErrorLevel, "browser.Component.Close error: %v", err)
		return err
	}
	return nil
}

func (c *Component) newTab(
	resource workspaceapi.URI, icon rune, name string,
	h browserapi.Handler, closer workspace.FlusherCloser,
) *browser.Tab {
	t := c.comp.NewTab(resource, icon, name, h, closer)
	t.Subscribe(&compTabSubscriber{parent: c})
	return t
}

func regroupArgs(s string) ([]string, error) {
	p := shsyntax.NewParser()
	printer := shsyntax.NewPrinter()
	var words []string
	for w, err := range p.WordsSeq(strings.NewReader(s)) {
		if err != nil {
			return nil, err
		}
		var builder strings.Builder
		for _, node := range w.Parts {
			if err := printer.Print(&builder, node); err != nil {
				return nil, err
			}
		}
		words = append(words, builder.String())
	}
	return words, nil
}

var _ workspace.FlusherCloser = (*editorFlusherCloser)(nil)

// used to intercept calls to Close and Flush to dispatch
// corresponding events to subscribers.
type editorFlusherCloser struct {
	parent    *Component
	fc        workspace.FlusherCloser
	h         Handler
	uri       workspaceapi.URI
	buf       *cell.Buffer
	commands  []textapi.CommandManual
	lastFlush int
}

func (c editorFlusherCloser) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
}

func (c editorFlusherCloser) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
	c.parent.setDirtyFileAttr(c.uri, c.buf, c.lastFlush)
}

func (e *editorFlusherCloser) ForceFlush() error {
	err := e.fc.ForceFlush()
	if derr := e.dispatchFlush(); derr != nil {
		return multierror.Append(err, derr)
	}
	return err
}

func (e *editorFlusherCloser) LastFlush() time.Time {
	return e.fc.LastFlush()
}

func (e *editorFlusherCloser) Flush() error {
	err := e.fc.Flush()
	if derr := e.dispatchFlush(); derr != nil {
		return multierror.Append(err, derr)
	}
	return err
}

func (e *editorFlusherCloser) Reload() error {
	err := e.fc.Reload()
	if err != nil {
		return err
	}
	return e.dispatchFlush()
}

func (e *editorFlusherCloser) dispatchFlush() error {
	e.lastFlush = e.buf.Version()
	e.parent.log(log.TraceLevel, "flushed, new snapshot is at %d", e.lastFlush)
	return e.parent.dispatchFlush(e.uri, e.h)
}

func (e *editorFlusherCloser) Close() error {
	ev := textapi.Event{
		Type:     textapi.EventTypeClose,
		URI:      e.uri,
		Resource: e.h,
	}
	e.parent.DispatchEvent(ev)
	ret := e.fc.Close()
	for _, cmd := range e.commands {
		err := e.parent.fileRegistry.UnsubscribeCommandForFile(e.uri, cmd.Name)
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return ret
}

type commandAll struct {
	handler CommandHandler
	man     command.Manual
}

// wraps Editor returned in calls to Edit
// to auto-delete in calls to Close or Handle(exit=true)
type wrapEditor struct {
	parent *Component
	Handler
}

func (w wrapEditor) Close() error {
	delete(w.parent.editors, w.Resource().String())
	return w.Handler.Close()
}

func (w wrapEditor) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = w.Handler.Handle(ev)
	if exit {
		delete(w.parent.editors, w.Resource().String())
	}
	return
}

type handlerWindowSubscriber = Component

func (c *handlerWindowSubscriber) OnFocus(old, focus handler.Window) {
	if old != (handler.Window{}) {
		c.tryDispatchEvent(old, textapi.EventTypeUnfocus)
	}
	c.tryDispatchEventFocus(focus)
	(*Component)(c).focus = focus
}

func apiManualToManual(cmd textapi.CommandManual) command.Manual {
	var subcmds []command.Manual
	if len(cmd.Commands) != 0 {
		subcmds = make([]command.Manual, len(cmd.Commands))
		for i, cmd := range cmd.Commands {
			subcmds[i] = apiManualToManual(cmd)
		}
	}
	return command.Manual{
		Name:     cmd.Name,
		Summary:  cmd.Summary,
		Synopsis: cmd.Synopsis,
		Commands: subcmds,
	}
}
