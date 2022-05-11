package text

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

var (
	// ErrInvalidSave is returned when trying to save a buffer that it's not a file
	// in the file system.
	ErrInvalidSave = errors.New("Cannot save this buffer")

	// ErrInvalidSplit is returned when attempting to split over a floating window.
	ErrInvalidSplit = errors.New("Cannot split this window")
)

// used for command and event handlers
const defaultTimeout = 1 * time.Second

// Component is an implementation of browser.Browser for file editing.
// It also satisfies tui.Component, and text.Editor.
type Component struct {
	comp           browser.Component
	workspace      workspace.ResourceOpener
	ed             Editor
	config         Config
	edSubscribers  map[EventType][]EventHandler
	cmdSubscribers map[string]CommandHandler
}

// used to intercept calls to Close and Flush to dispatch
// corresponding events to subscribers.
type editorFlusherCloser struct {
	parent    *Component
	fc        workspace.FlusherCloser
	h         Handler
	uri       workspace.URI
	buf       *cell.Buffer
	lastFlush string
}

func (c editorFlusherCloser) OnWillEdit(start, end term.Coordinates, str string) {
}

func (c editorFlusherCloser) OnDidEdit(from, to term.Coordinates, old string) {
	c.parent.setTabAttr(c.uri, c.buf, c.lastFlush)
}

func (e *editorFlusherCloser) Flush() error {
	content, err := e.parent.dispatchFlush(e.uri, e.h)
	if err != nil {
		return err
	}
	e.lastFlush = content
	return e.fc.Flush()
}

func (e *editorFlusherCloser) Close() error {
	ev := Event{
		Type:     EventTypeClose,
		URI:      e.uri,
		Resource: e.h,
	}
	e.parent.dispatchEvent(ev)
	return e.fc.Close()
}

// NewComponent allocates storage for a new Component and initializes it.
func NewComponent(ed Editor, w workspace.ResourceOpener, config Config) (
	c *Component, err error,
) {
	c = new(Component)
	err = c.Init(ed, w, config)
	if err != nil {
		return
	}
	return
}

func (c *Component) tryLog(level log.Level, msg string, args ...interface{}) {
	if c.config.Logger != nil {
		c.config.Logger.Logf(level, msg, args...)
	}
}

func (c *Component) newCellBuffer() *cell.Buffer {
	buf := cell.NewBuffer()
	buf.InitWithTabspaces(c.config.Tabspaces)
	if c.config.Logger != nil {
		// NOTE: only enable when trying to debug low level buffer bugs
		// as it degrades performance quite a bit.
		// buf.WithLogger(c.config.Logger)
	}
	return buf
}

func (c *Component) resetTabProperties(file workspace.URI) {
	c.comp.SetTabAttr(file, term.Attributes{})
	c.comp.SetTabName(file, file.Name())
}

// TODO this is a very inefficient way of checking if a file was changed.
// We should instead collect edits and check for undos by comparing arguments
// and return values.
func (c *Component) setTabAttr(file workspace.URI, buf *cell.Buffer, lastFlush string) {
	content := buf.String()
	if content == lastFlush {
		c.resetTabProperties(file)
		return
	}
	c.comp.SetTabAttr(file, c.config.DirtyTabAttr)
	tabname := fmt.Sprintf("%s*", file.Name())
	c.comp.SetTabName(file, tabname)
}

func (c *Component) getSwapDir(file workspace.URI) (workspace.URI, error) {
	return workspace.DefaultSwapDirectory(file)
}

func (c *Component) newFileBuffer(
	file, recSwapFile workspace.URI, buf *cell.Buffer, readOnly bool,
) (ret *editorFlusherCloser, err error) {
	var fc workspace.FlusherCloser
	if recSwapFile != (workspace.URI{}) {
		fc, err = c.workspace.Recover(file, recSwapFile, buf)
	} else {
		var swapDir workspace.URI
		swapDir, err = c.getSwapDir(file)
		if err == nil {
			fc, err = c.workspace.Open(file, buf, swapDir, readOnly)
		}
	}

	if err != nil {
		return nil, err
	}

	efc := &editorFlusherCloser{
		parent:    c,
		fc:        fc,
		uri:       file,
		buf:       buf,
		lastFlush: buf.String(),
	}

	// no need to unsubscribe upon Close since the assumption
	// is that a Component always outlives a cell.Buffer
	buf.Subscribe(efc)

	return efc, nil
}

// Init initializes this Component with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (c *Component) Init(ed Editor, w workspace.ResourceOpener, config Config) error {
	c.config = config

	c.comp.Init(c.config.Config)

	c.ed = ed
	c.workspace = w
	c.edSubscribers = make(map[EventType][]EventHandler)
	c.cmdSubscribers = make(map[string]CommandHandler)

	var first browser.Handler

	if c.config.RecoveryFilepath != (workspace.URI{}) {
		if len(c.config.Filepaths) != 1 {
			return errors.New("only one file expected if recovery file is passed")
		}
		h, err := c.RecoverFileTab(c.config.Filepaths[0], c.config.RecoveryFilepath, false)
		if err != nil {
			return err
		}
		first = h
	}

	for _, filename := range c.config.Filepaths {
		h, err := c.Open(filename)
		if err == workspace.ErrFileAlreadyOpen {
			// handled via user Prompt
			err = nil
		}
		if err != nil {
			return err
		}
		if first == nil {
			first = h
		}
	}

	if first != nil {
		return c.comp.Focus().SetContent(first)
	}

	return nil
}

func (c *Component) setFocusToTab(t *browser.Tab) (browser.Handler, error) {
	err := c.comp.Focus().SetContent(t)
	if err != nil {
		if err != browser.ErrTabNotFree {
			return nil, err
		}
	}
	return t, nil
}

type compTabSubscriber Component

func (s *compTabSubscriber) OnFocus(t *browser.Tab) {
	res, ok := t.Handler().(Handler)
	if !ok {
		return
	}
	(*Component)(s).dispatchEvent(Event{
		Type:     EventTypeFocus,
		URI:      t.URI(),
		Resource: res,
	})
}

func (s *compTabSubscriber) OnFree(t *browser.Tab) {
	res, ok := t.Handler().(Handler)
	if !ok {
		return
	}
	(*Component)(s).dispatchEvent(Event{
		Type:     EventTypeUnfocus,
		URI:      t.URI(),
		Resource: res,
	})
}

// OpenFileTab opens the file at filename path, as a new browser tab.
// It's up to the caller to use the returned browser.Handler and switch
// any of the active windows to use it.
//
// If recoveryFilename is not empty, then the file will be recovered from the
// contents of recoveryFilename.
func (c *Component) OpenFileTab(file workspace.URI, readOnly bool) (
	browser.Handler, error,
) {
	if file == (workspace.URI{}) {
		return nil, errors.New("empty URI")
	}
	return c.openFileTab(file, workspace.URI{}, readOnly)
}

// RecoverFileTab recovers the file at filename by using the file at recoverFilename
// and opens a tab it like OpenFileTab. See OpenFileTab for more details.
func (c *Component) RecoverFileTab(
	file workspace.URI, recoveryFilename workspace.URI, readOnly bool,
) (browser.Handler, error) {
	if file == (workspace.URI{}) || recoveryFilename == (workspace.URI{}) {
		return nil, errors.New("empty URI")
	}
	return c.openFileTab(file, recoveryFilename, readOnly)
}

func (c *Component) openFileTab(
	file workspace.URI, recoveryFilename workspace.URI, readOnly bool,
) (browser.Handler, error) {
	t, ok := c.comp.Tab(file)
	if ok {
		return c.setFocusToTab(t)
	}

	buf := c.newCellBuffer()
	fc, err := c.newFileBuffer(file, recoveryFilename, buf, readOnly)
	if err != nil {
		return nil, err
	}

	editor, err := c.ed.Edit(file, buf)
	if err != nil {
		return nil, err
	}
	fc.h = editor

	t = c.newTab(file, file.Name(), editor, fc)
	return t, nil
}

func (c *Component) openRecoveryPrompt(file workspace.URI) {
	const (
		recoverOpt  = "Recover"
		readOnlyOpt = "Open Read-Only"
		skipOpt     = "Skip"
	)

	msg := fmt.Sprintf(`File %s is already
open by another process or
an edit session for this file crashed.`, file)

	c.comp.Prompt(msg, []string{recoverOpt, readOnlyOpt, skipOpt},
		[]term.KeyComb{{Ch: 'R'}, {Ch: 'O'}, {Ch: 'S'}},
		func(i int, opt string) {

			var h browser.Handler
			var err error

			switch opt {
			case recoverOpt:
				var swapDir, swapFile workspace.URI
				swapDir, err = c.getSwapDir(file)
				if err == nil {
					swapFile, err = workspace.DefaultSwapFile(swapDir, file)
					if err == nil {
						h, err = c.RecoverFileTab(file, swapFile, false)
					}
				}
			case readOnlyOpt:
				h, err = c.OpenFileTab(file, true)
			case skipOpt:
			}
			if h != nil {
				err = c.comp.Focus().SetContent(h)
			}
			if err != nil {
				c.tryLog(log.ErrorLevel, "recovery prompt: %v", err)
				c.SetMessage("%v", err)
				return
			}
		})
}

// Open opens the given file in a new browser tab. If file is already
// open by another session or the last edit session crashed, it
// will create a prompt for the user to decide what to do.
func (c *Component) Open(file workspace.URI) (browser.Handler, error) {
	h, err := c.OpenFileTab(file, false)
	if err != nil && err == workspace.ErrFileAlreadyOpen {
		c.openRecoveryPrompt(file)
	}
	return h, err
}

// Editor satisfies Editor interface.
func (c *Component) Editor(file workspace.URI) (Handler, error) {
	for _, tab := range c.comp.Tabs() {
		if tab.URI().String() == file.String() {
			h, ok := tab.Handler().(Handler)
			if !ok {
				continue
			}
			return h, nil
		}
	}
	return nil, errors.New("handler not found")
}

// KeyMapping returns a command that was mapped to the given key
// combination and true or an empty string and false if there was
// no command mapped to the given key.
func (c *Component) KeyMapping(key term.KeyComb) (string, bool) {
	cmd, ok := c.config.CommandKeyBindings[key]
	c.tryLog(log.TraceLevel, "KeyMapping(%#v): %s", key, cmd)
	return cmd, ok
}

// DispatchCommand dispatches a EventTypeCommand with cmd to subscribers
// subscribed via SubscribeEditorEvents.
func (c *Component) DispatchCommand(cmd Command) (handled bool) {
	commander, handled := c.cmdSubscribers[cmd.Name]
	if !handled {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	exit := commander.HandleCommand(ctx, cmd)
	if exit {
		delete(c.cmdSubscribers, cmd.Name)
	}
	return true
}

func (c *Component) dispatchFlush(file workspace.URI, h Handler) (string, error) {
	content, err := c.getContent(h)
	if err != nil {
		return "", err
	}

	ev := Event{
		Type:     EventTypeFlush,
		URI:      file,
		Resource: h,
		Content:  content,
	}
	// clear dirty/flushed attributes
	c.resetTabProperties(file)
	c.dispatchEvent(ev)
	return content, nil
}

// dispatchEvent either flush or close events
func (c *Component) dispatchEvent(ev Event) (handled bool) {
	subs, ok := c.edSubscribers[ev.Type]
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	remain := make([]EventHandler, 0, len(subs))
	for _, h := range subs {
		exit := h.Handle(ctx, ev)
		if !exit {
			remain = append(remain, h)
		}
		handled = true
	}
	c.edSubscribers[ev.Type] = remain
	return
}

// SetMessage formats the given msg and args and displays it on next Draw.
func (c *Component) SetMessage(msg string, args ...interface{}) error {
	c.comp.SetMessage(msg, args...)
	return nil
}

// Split satisfies browser.WindowManager.
func (c *Component) Split(o browser.Orientation, h browser.Handler) (browser.Window, error) {
	w, ok := c.comp.Split(o, h)
	if !ok {
		return nil, ErrInvalidSplit
	}
	return w, nil
}

// Bar creates a new status bar with h's component and delegates handling of mouse events to h.
func (c *Component) Bar(o browser.Orientation, h tui.Handler) error {
	c.comp.Bar(o, h)
	return nil
}

// PublishInterrupt interrupts the main event loop to redraw the terminal.
func (c *Component) PublishInterrupt() error {
	c.config.SendInterrupt()
	return nil
}

// PublishEventNone sends an EventNone to the main event loop which
// forces Handle to be called on the tui.Handler in focus.
func (c *Component) PublishEventNone() error {
	c.config.SendEventNone()
	return nil
}

// Focus returns the current window in focus. It satisfies browser.Browser.
func (c *Component) Focus() (browser.Window, error) {
	return c.comp.Focus(), nil
}

// SetFocus sets the window in focus and returns the previous window in focus.
// It satisfies browser.Browser.
func (c *Component) SetFocus(win browser.Window) (browser.Window, error) {
	return c.comp.SetFocus(win), nil
}

// Edit edits the resource with name and buffer with the underlying Editor
// in a new browser buffer.
func (c *Component) Edit(file workspace.URI, buf *cell.Buffer) (Handler, error) {
	editor, err := c.ed.Edit(file, buf)
	if err != nil {
		return nil, err
	}

	c.newTab(file, file.Name(), editor, nil)
	return editor, nil
}

// SetLocationList satisfies text.Editor.
func (c *Component) SetLocationList(h Handler, ID string, loc LocationList) error {
	return c.ed.SetLocationList(h, ID, loc)
}

// MoveToNextLocation satisfies text.Editor.
func (c *Component) MoveToNextLocation(h Handler, ID string) error {
	return c.ed.MoveToNextLocation(h, ID)
}

// MoveToPrevLocation satisfies text.Editor.
func (c *Component) MoveToPrevLocation(h Handler, ID string) error {
	return c.ed.MoveToPrevLocation(h, ID)
}

// CellView satisfies text.Editor.
func (c *Component) CellView(h Handler) CellView {
	return c.ed.CellView(h)
}

// CellEditor satisfies text.Editor.
func (c *Component) CellEditor(h Handler) CellEditor {
	return c.ed.CellEditor(h)
}

// Flush flushes the contents of the buffer at win, if this buffer
// was created with a FlusherCloser. See browser.NewBuffer.
func (c *Component) Flush(win browser.Window) error {
	content, err := win.Content()
	if err != nil {
		return fmt.Errorf("editor.Component.Flush: win.Content: %v", err)
	}
	t, ok := content.(*browser.Tab)
	if !ok {
		return ErrInvalidSave
	}

	fc := t.Closer().(workspace.FlusherCloser)
	err = fc.Flush()
	if err != nil {
		return fmt.Errorf("editor.Component.Flush: %v", err)
	}
	return nil
}

func (c *Component) getContent(h Handler) (string, error) {
	cells, err := c.ed.CellView(h).RawCells()
	if err != nil {
		return "", fmt.Errorf("Error dispatching event content: RawCells: %v", err)
	}
	return cell.CellsToString(cells), nil
}

func (c *Component) dispatchOpenTabs(h EventHandler) (error, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tab := range c.comp.Tabs() {
		resHandler, ok := tab.Handler().(Handler)
		if !ok {
			// tab handler does not implement Handler
			continue
		}
		str, err := c.getContent(resHandler)
		if err != nil {
			return err, false
		}
		exit := h.Handle(ctx, Event{
			Type:     EventTypeOpen,
			URI:      tab.URI(),
			Resource: resHandler,
			Content:  str,
		})
		if exit {
			return nil, true
		}
	}
	return nil, false
}

func (c *Component) dispatchFocusTab(h EventHandler) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t, ok := c.comp.FocusTab()
	if ok {
		resHandler, ok := t.Handler().(Handler)
		if ok {
			ev := Event{
				Type:     EventTypeFocus,
				URI:      t.URI(),
				Resource: resHandler,
			}
			return h.Handle(ctx, ev)
		}
	}
	return false
}

// SubscribeEditorEvents subscribes h to editor events of type ev.
// If ev is of type EventTypeOpen, an event will be dispatched for
// every Tab currently open.
func (c *Component) SubscribeEditorEvents(evs []EventType, h EventHandler) error {
	// iterate to dispatch immediate events
	for _, ev := range evs {
		switch ev {
		case EventTypeOpen:
			err, exit := c.dispatchOpenTabs(h)
			if err != nil {
				return err
			}
			if exit {
				return nil
			}
		case EventTypeFocus:
			exit := c.dispatchFocusTab(h)
			if exit {
				return nil
			}
		}
	}

	// iterate again so if handler exited for any event, we have returned
	// and we do not subscribe it
	var delegated []EventType
	for _, ev := range evs {
		switch ev {
		// delegate certain event dispatching to underlying editor.
		case EventTypeOpen, EventTypeEdit, EventTypeScroll, EventTypeCursor:
			delegated = append(delegated, ev)
		default:
			if _, ok := c.edSubscribers[ev]; !ok {
				c.edSubscribers[ev] = make([]EventHandler, 0, 1)
			}
			c.edSubscribers[ev] = append(c.edSubscribers[ev], h)
		}
	}

	return c.ed.SubscribeEditorEvents(delegated, h)
}

// Commands returns a list of commands registered via SubscribeCommand.
func (c *Component) Commands() (ret []string) {
	ret = make([]string, len(c.cmdSubscribers))
	var i int
	for cmd := range c.cmdSubscribers {
		ret[i] = cmd
		i++
	}
	return ret
}

// SubscribeCommand installs cm as a command handler of cmd or returns
// an error if there's already a CommandHandler installed for this cmd.
func (c *Component) SubscribeCommand(cmd string, cm CommandHandler) error {
	if _, ok := c.cmdSubscribers[cmd]; ok {
		return errors.New("command already registered")
	}

	c.cmdSubscribers[cmd] = cm
	return nil
}

// Browser returns this Component's underlying browser.Component.
func (c *Component) Browser() *browser.Component {
	return &c.comp
}

// Resize satisfies tui.Component.
func (c *Component) Resize(width, height int) {
	c.comp.Resize(width, height)
}

// Draw satisfies tui.Component.
func (c *Component) Draw(w term.Writer) {
	c.comp.Draw(w)
}

// Create satisfies browser.Storage
func (c *Component) Create(
	ctx context.Context, ID string, doc interface{},
) error {
	return c.config.Storage.Create(ctx, ID, doc)
}

// Set satisfies browser.Storage
func (c *Component) Set(
	ctx context.Context, ID string, doc interface{},
) error {
	return c.config.Storage.Set(ctx, ID, doc)
}

// Update satisfies browser.Storage
func (c *Component) Update(
	ctx context.Context, ID string, updates []document.Update,
) error {
	return c.config.Storage.Update(ctx, ID, updates)
}

// Get satisfies browser.Storage
func (c *Component) Get(
	ctx context.Context, ID string, doc interface{},
) error {
	return c.config.Storage.Get(ctx, ID, doc)
}

// Delete satisfies browser.Storage
func (c *Component) Delete(
	ctx context.Context, ID string,
) error {
	return c.config.Storage.Delete(ctx, ID)
}

// List satisfies browser.Storage
func (c *Component) List(
	ctx context.Context, filters []document.Filter,
) (document.Iterator, error) {
	return c.config.Storage.List(ctx, filters)
}

// SetCursor satisfies text.Editor
func (c *Component) SetCursor(h Handler, pos term.Coordinates) error {
	return c.ed.SetCursor(h, pos)
}

// Cursor satisfies text.Editor
func (c *Component) Cursor(h Handler) (term.Coordinates, error) {
	return c.ed.Cursor(h)
}

// Floating satisfies browser.WindowManager.
func (c *Component) Floating(
	h browser.Handler, at term.Coordinates, width, height int,
) (browser.Window, error) {
	if at.Y < 0 || at.X < 0 {
		return nil, fmt.Errorf("invalid floating window coordinates: %v", at)
	}
	return c.comp.Floating(h, at, width, height), nil
}

func (c *Component) newTab(
	resource workspace.URI, name string, h browser.Handler, closer io.Closer,
) *browser.Tab {
	t := c.comp.NewTab(resource, name, h, closer)
	t.Subscribe((*compTabSubscriber)(c))
	return t
}

// Tab satisfies browser.WindowManager.
func (c *Component) Tab(resource workspace.URI, name string, h browser.Handler) (
	browser.Handler, error,
) {
	t, ok := c.comp.Tab(resource)
	if ok {
		t, err := c.setFocusToTab(t)
		return t, err
	}

	t = c.newTab(resource, name, h, nil)
	return t, nil
}

// Close closes all resources associated with this Component.
func (c *Component) Close() error {
	// avoid dispatching close events on flusherCloser callbacks
	c.edSubscribers = make(map[EventType][]EventHandler)

	err1 := c.comp.Close()
	err2 := c.config.Storage.Close()
	if err2 != nil {
		c.tryLog(log.ErrorLevel, "config.Storage.Close error: %v", err2)
	}
	if err1 != nil {
		c.tryLog(log.ErrorLevel, "browser.Component.Close error: %v", err1)
		return err1
	}
	return err2
}
