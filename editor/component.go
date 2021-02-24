package editor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

type flusherCloser interface {
	Flush() error
	io.Closer
}

var (
	// ErrInvalidSave is returned when trying to save a buffer that it's not a file
	// in the file system.
	ErrInvalidSave = errors.New("Cannot save this buffer")

	// ErrInvalidSplit is returned when attempting to split over a floating window.
	ErrInvalidSplit = errors.New("Cannot split this window")
)

type openFileFunc func(filePath string,
	buf *cell.Buffer, swapDir string) (flusherCloser, error)

type recoverFileFunc func(filePath,
	swapFilePath string, buf *cell.Buffer) (flusherCloser, error)

// Component is an implementation of browser.Browser for file editing.
// It also satisfies tui.Component, and editor.Editor.
type Component struct {
	openFileFn      openFileFunc
	recoverFileFn   recoverFileFunc
	interruptDraw   func()
	comp            browser.Component
	ed              Editor
	config          Config
	keymap          map[term.Event]term.Event
	termSubscribers map[term.Event]browser.EventHandler
	edSubscribers   map[EventType][]EventHandler
	cmdSubscribers  map[string]CommandHandler
}

// used to intercept calls to Close and Flush to dispatch
// corresponding events to subscribers.
type editorFlusherCloser struct {
	parent *Component
	fc     flusherCloser
	name   string
	h      Handler
}

func (e *editorFlusherCloser) Flush() error {
	content, err := e.parent.getContent(e.h)
	if err != nil {
		return err
	}

	ev := Event{
		Type:         EventTypeFlush,
		ResourceName: e.name,
		Resource:     e.h,
		Content:      content,
	}
	e.parent.dispatchEvent(ev)
	return e.fc.Flush()
}

func (e *editorFlusherCloser) Close() error {
	ev := Event{
		Type:         EventTypeClose,
		ResourceName: e.name,
		Resource:     e.h,
	}
	e.parent.dispatchEvent(ev)
	return e.fc.Close()
}

type compEventHandler struct {
	c *Component
	h browser.EventHandler
}

func (h compEventHandler) Handle(ev term.Event) (exit bool) {
	exit = h.h.Handle(ev)
	if exit {
		h.c.unsubscribe(ev)
	}
	return
}

// NewComponent allocates storage for a new Component and initializes it.
func NewComponent(ed Editor, config Config) (c *Component, err error) {
	c = new(Component)
	err = c.Init(ed, config)
	if err != nil {
		return
	}
	return
}

func (c *Component) initConstructors() {
	if c.openFileFn == nil {
		c.openFileFn = func(filePath string,
			buf *cell.Buffer, swapDir string) (flusherCloser, error) {
			return NewFileBuffer(filePath, buf, swapDir)
		}
	}

	if c.recoverFileFn == nil {
		c.recoverFileFn = func(filePath,
			swapFilePath string, buf *cell.Buffer) (flusherCloser, error) {
			return RecoverFileBuffer(filePath, swapFilePath, buf)
		}
	}
	if c.interruptDraw == nil {
		c.interruptDraw = term.Interrupt
	}
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
		// buf = buf.WithLogger(e.config.Logger)
	}
	return buf
}

func (c *Component) newFileBuffer(
	filename, recSwapFile string, buf *cell.Buffer,
) (ret *editorFlusherCloser, err error) {
	var fc flusherCloser
	if recSwapFile != "" {
		fc, err = c.recoverFileFn(filename, recSwapFile, buf)
	} else {
		fc, err = c.openFileFn(filename, buf, c.config.SwapDir)
	}

	if err != nil {
		return nil, err
	}

	return &editorFlusherCloser{
		parent: c,
		fc:     fc,
		name:   filename,
	}, nil
}

// Init initializes this Component with the given editor and Options.
// It returns an error if an initial filepath was given through WithFilePath option
// and the file failed to be opened.
func (c *Component) Init(ed Editor, config Config) (err error) {
	c.initConstructors()
	c.config = config

	c.comp.Init(c.config.Config)

	c.ed = ed
	c.termSubscribers = make(map[term.Event]browser.EventHandler)
	c.edSubscribers = make(map[EventType][]EventHandler)
	c.cmdSubscribers = make(map[string]CommandHandler)

	if c.config.RecoveryFilepath != "" {
		if len(c.config.Filepaths) != 1 {
			return errors.New("only one file expected if recovery file is passed")
		}
		_, err = c.OpenFileTab(c.config.Filepaths[0], c.config.RecoveryFilepath)
		return
	}

	for _, filename := range c.config.Filepaths {
		_, err = c.OpenFileTab(filename, "")
		if err != nil {
			return
		}
	}

	return
}

func (c *Component) setFocusToTab(id string) (browser.Handler, error) {
	t, ok := c.comp.Tab(id)
	if !ok {
		// NOTE: swap file was not cleaned up
		// probably because prev process terminated
		// abruptly.  Here we could interactively ask user
		// what to do.
		return nil, ErrFileAlreadyOpen
	}
	err := c.comp.Focus().SetContent(t)
	if err != nil {
		if err != browser.ErrTabNotFree {
			return nil, err
		}
	}
	return t, nil
}

// OpenFileTab opens the file at filename path, with an optional recovery file,
// as a new browser tab. It's up to the caller to use the returned
// browser.Handler and switch any of the active windows to use it.
//
// If recoveryFilename is not empty, then the file will be recovered from the
// contents of recoveryFilename.
func (c *Component) OpenFileTab(
	filename, recoveryFilename string,
) (browser.Handler, error) {
	// needed as tab ID
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("could not evaluate file path '%s': %v", filename, err)
	}

	buf := c.newCellBuffer()
	fc, err := c.newFileBuffer(filename, recoveryFilename, buf)
	if err != nil {
		if err == ErrFileAlreadyOpen {
			return c.setFocusToTab(filename)
		}
		return nil, err
	}

	editor, err := c.ed.Edit(filename, buf)
	if err != nil {
		return nil, err
	}
	fc.h = editor

	tabName := filepath.Base(filename)
	return c.comp.NewTab(filename, tabName, editor, fc), nil
}

// Open opens the given file in a new browser tab.
func (c *Component) Open(file string) (browser.Handler, error) {
	return c.OpenFileTab(file, "")
}

// KeyMapping returns a key mapping for ev and true
// or the original ev and false if there's
// no mapping. It also returns any command that was mapped
// to the return mapping (or the original event, if there's no event mapping).
// Mappings are created via MergeKeyMap.
func (c *Component) KeyMapping(ev term.Event) (term.Event, string, bool) {
	mev, ok := c.keymap[ev]
	if !ok {
		mev = ev
	}
	cmd, _ := c.config.CommandKeyBindings[mev]
	c.tryLog(log.TraceLevel, "KeyMapping(%#v): %#v, %s", ev, mev, cmd)
	return mev, cmd, ok
}

// MergeKeyMap takes the given keymap and merges it with the Browser's keymap
// to override the current event key mappings.
func (c *Component) MergeKeyMap(keymap map[term.Event]term.Event) error {
	if c.keymap == nil {
		c.keymap = make(map[term.Event]term.Event)
	}
	for k, v := range keymap {
		if k.Type != term.EventKey || v.Type != term.EventKey {
			return errors.New("invalid mapping of non-key event")
		}
		c.keymap[k] = v
	}
	return nil
}

// Subscribe subscribers h EventHandler to term.Event ev.
func (c *Component) Subscribe(ev term.Event, h browser.EventHandler) error {
	if ev.Type != term.EventKey {
		return errors.New("invalid subscription of non-key event")
	}
	if _, ok := c.termSubscribers[ev]; ok {
		return fmt.Errorf("there's already a subscriber subscribed to: %#v", ev)
	}
	c.termSubscribers[ev] = compEventHandler{c: c, h: h}
	return nil
}

// Publish dispatches ev to subscribers, previously installed via Subscribe.
func (c *Component) Publish(ev term.Event) (handled bool) {
	if subscriber, ok := c.termSubscribers[ev]; ok {
		subscriber.Handle(ev)
		handled = true
	}
	return
}

// DispatchCommand dispatches a EventTypeCommand with cmd to subscribers
// subscribed via SubscribeEditor.
func (c *Component) DispatchCommand(
	cmd string, resource Handler, resourceName string,
) (handled bool) {
	commander, handled := c.cmdSubscribers[cmd]
	if !handled {
		return false
	}

	cursor, _ := c.ed.Cursor(resource)
	strc := Command{
		Name:         cmd,
		Resource:     resource,
		ResourceName: resourceName,
		Cursor:       cursor,
	}
	exit := commander.HandleCommand(strc)
	if exit {
		delete(c.cmdSubscribers, cmd)
	}
	return true
}

// dispatchEvent either flush or close events
func (c *Component) dispatchEvent(ev Event) (handled bool) {
	subs, ok := c.edSubscribers[ev.Type]
	if !ok {
		return
	}

	remain := make([]EventHandler, 0, len(subs))
	for _, h := range subs {
		exit := h.Handle(ev)
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

// SplitVerticalRight opens a new window tile to the right of the
// current window in focus and initializes it with h.
func (c *Component) SplitVerticalRight(h browser.Handler) (browser.Window, error) {
	w, ok := c.comp.SplitVerticalRight(h)
	if !ok {
		return nil, ErrInvalidSplit
	}
	return w, nil
}

// SplitVerticalLeft opens a new window tile to the left of the
// current window in focus and initializes it with h.
func (c *Component) SplitVerticalLeft(h browser.Handler) (browser.Window, error) {
	w, ok := c.comp.SplitVerticalLeft(h)
	if !ok {
		return nil, ErrInvalidSplit
	}
	return w, nil
}

// SplitHorizontalBelow opens a new window tile below the current window in focus
// and initializes it with h.
func (c *Component) SplitHorizontalBelow(h browser.Handler) (browser.Window, error) {
	w, ok := c.comp.SplitHorizontalBelow(h)
	if !ok {
		return nil, ErrInvalidSplit
	}
	return w, nil
}

// SplitHorizontalAbove opens a new window tile above the current window in focus
// and initializes it with h.
func (c *Component) SplitHorizontalAbove(h browser.Handler) (browser.Window, error) {
	w, ok := c.comp.SplitHorizontalAbove(h)
	if !ok {
		return nil, ErrInvalidSplit
	}
	return w, nil
}

func (c *Component) unsubscribe(ev term.Event) {
	delete(c.termSubscribers, ev)
}

// PublishInterrupt interrupts the main event loop to redraw the terminal.
func (c *Component) PublishInterrupt() error {
	c.interruptDraw()
	return nil
}

// Focus returns the current window in focus. It satisfies browser.Browser.
func (c *Component) Focus() (browser.Window, error) {
	return c.comp.Focus(), nil
}

// Edit edits the resource with name and buffer with the underlying Editor
// in a new browser buffer.
func (c *Component) Edit(name string, buf *cell.Buffer) (Handler, error) {
	editor, err := c.ed.Edit(name, buf)
	if err != nil {
		return nil, err
	}
	_ = c.comp.NewTab(name, name, editor, nil)

	return editor, nil
}

// SetLocationList satisfies editor.Editor.
func (c *Component) SetLocationList(h Handler, ID string, loc LocationList) error {
	return c.ed.SetLocationList(h, ID, loc)
}

// MoveToNextLocation satisfies editor.Editor.
func (c *Component) MoveToNextLocation(h Handler, ID string) error {
	return c.ed.MoveToNextLocation(h, ID)
}

// MoveToPrevLocation satisfies editor.Editor.
func (c *Component) MoveToPrevLocation(h Handler, ID string) error {
	return c.ed.MoveToPrevLocation(h, ID)
}

// Reader satisfies editor.Editor.
func (c *Component) Reader(h Handler) Reader {
	return c.ed.Reader(h)
}

// Writer satisfies editor.Editor.
func (c *Component) Writer(h Handler) Writer {
	return c.ed.Writer(h)
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

	fc := t.Closer().(flusherCloser)
	err = fc.Flush()
	if err != nil {
		return fmt.Errorf("editor.Component.Flush: %v", err)
	}
	return nil
}

func (c *Component) getContent(h Handler) (string, error) {
	cells, err := c.ed.Reader(h).RawCells()
	if err != nil {
		return "", fmt.Errorf("Error dispatching event content: RawCells: %v", err)
	}
	return cell.CellsToString(cells), nil
}

// SubscribeEditor subscribes h to editor events of type ev.
// If ev is of type EventTypeOpen, an event will be dispatched for
// every Tab currently open.
func (c *Component) SubscribeEditor(ev EventType, h EventHandler) error {
	// delegate open/insert/delete event dispatching to underlying editor.
	switch ev {
	case EventTypeOpen:
		for _, tab := range c.comp.Tabs() {
			resHandler := tab.Handler()
			str, err := c.getContent(resHandler)
			if err != nil {
				return err
			}
			exit := h.Handle(Event{
				Type:         EventTypeOpen,
				ResourceName: tab.ID(),
				Resource:     resHandler,
				Content:      str,
			})
			if exit {
				return nil
			}
		}
		fallthrough
	case EventTypeDelete, EventTypeInsert:
		return c.ed.SubscribeEditor(ev, h)
	}

	if _, ok := c.edSubscribers[ev]; !ok {
		c.edSubscribers[ev] = make([]EventHandler, 0, 1)
	}

	c.edSubscribers[ev] = append(c.edSubscribers[ev], h)
	return nil
}

// Register installs cm as a command handler of cmd or returns
// an error if there's already a CommandHandler installed for this cmd.
func (c *Component) Register(cmd string, cm CommandHandler) error {
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

// SetCursor satisfies editor.Editor
func (c *Component) SetCursor(h Handler, pos term.Coordinates) error {
	return c.ed.SetCursor(h, pos)
}

// Cursor satisfies editor.Editor
func (c *Component) Cursor(h Handler) (term.Coordinates, error) {
	return c.ed.Cursor(h)
}

// FloatingWindow opens a new floating window at the given coordinates,
// with the given height and width.
func (c *Component) FloatingWindow(
	h browser.Handler, at term.Coordinates, width, height int,
) (browser.Window, error) {
	return c.comp.FloatingWindow(h, at, width, height), nil
}

// Close closes all resources associated with this Component.
func (c *Component) Close() error {
	// avoid dispatching close events on flusherCloser callbacks
	c.termSubscribers = make(map[term.Event]browser.EventHandler)
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
