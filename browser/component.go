package browser

import (
	"errors"
	"fmt"
	"io"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler"
	"github.com/ernestrc/go-tui/term"
)

var (
	// ErrTabNotFree is returned when a tab is being used in call to
	// SetContent but it's already owned by another Window.
	ErrTabNotFree = errors.New("Tab already rendered in Window")

	logBufDrawTimes = 2
)

// Component renders a browser-like tui.Compontent and exposes an API
// to open new windows, add new tabs, and switch between tabs.
//
// All tui.Handlers installed other than via NewTab are considered ephemeral,
// and will be destroyed either when windows close or when they return exit=true
// to a call to Handle. Conversely, tui.Handlers installed via NewTab
// will remain as a tab and can be managed independently from windows.
type Component struct {
	logBuf     cell.Buffer
	logBufDraw int
	logVirt    handler.Virtual
	tabs       handler.Tabs
	wm         handler.WindowManager
	union      handler.FrameUnion

	config       Config
	startHandler Handler
	buffers      []*Tab
	windows      map[uint64]*browserWindow
}

// component.WindowManager sinchronously removes tui.Handlers
// upon returning exit=true on calls to Handle. This
// structure is used to call OnUnmount when this occurs.
type browserContent struct {
	Handler
	unmounted bool
	c         *Component
}

func (c *browserContent) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = c.Handler.Handle(ev)
	if exit {
		c.c.onUnmount(c, "component exit via Handle()(exit=true)")
	}
	return
}

func (c *browserContent) OnUnmount() error {
	if c.unmounted {
		return nil
	}
	c.unmounted = true
	return c.Handler.OnUnmount()
}

// needed mutable to inverse a split
type browserWindow struct {
	parent  *Component
	win     handler.Window
	onClose func()
}

func (w *browserWindow) id() uint64 {
	return w.win.ID()
}

func (w *browserWindow) onWindowClosed(fn func()) {
	w.onClose = fn
}

func (w *browserWindow) Content() (Handler, error) {
	h := w.win.Content().(Handler)
	t, ok := h.(*Tab)
	if !ok {
		return h.(*browserContent).Handler, nil
	}
	return t, nil
}

func (w *browserWindow) SetContent(h Handler) error {
	return w.parent.tryUpdateWindowContent(w, h)
}

// browserWindow is passed by value, so we store whether
// it has been closed or not in Handler.
func (w *browserWindow) Close() error {
	if w.parent == nil {
		return nil
	}

	parent := w.parent
	onClose := w.onClose
	w.onClose = nil
	w.parent = nil

	err := parent.closeWindow(w)
	if err != nil {
		w.onClose = onClose
		w.parent = parent
		w.parent.setError(err)
	} else if onClose != nil {
		onClose()
	}

	return err
}

func (c *Component) newWindow(win handler.Window) *browserWindow {
	browserWin := &browserWindow{
		parent: c,
		win:    win,
	}
	c.windows[browserWin.id()] = browserWin
	return browserWin
}

// closeWindow closes win or returns an error if win is the last Window.
func (c *Component) closeWindow(win *browserWindow) error {
	_, ok := c.findWindow(win.id())
	if !ok {
		return nil
	}

	content := win.win.Content().(Handler)
	err := win.win.Close()
	if err != nil {
		return err
	}

	delete(c.windows, win.id())

	reason := fmt.Sprintf("Close called on window: %p", win)
	c.onUnmount(content, reason)
	return nil
}

func (c *Component) findWindow(winID uint64) (*browserWindow, bool) {
	w, ok := c.windows[winID]
	return w, ok
}

// NewComponent allocates storage for a new Component and initializes it.
func NewComponent(config Config) *Component {
	ret := new(Component)
	ret.Init(config)
	return ret
}

// Init initializes this Component with config.
func (c *Component) Init(config Config) {
	c.config = config
	c.windows = make(map[uint64]*browserWindow)

	c.logBuf.Init()
	c.logVirt = NewMessageSpan(&c.logBuf, config.MessageBarAttr)

	c.tabs.Init()
	c.tabs.OnClick = func(id int) {
		t := c.buffers[id]
		err := c.tryUpdateWindowContent(c.Focus().(*browserWindow), t)
		if err != nil {
			c.setError(err)
		}
	}

	handlerWmConfig := handler.WindowManagerConfig{
		FocusFrameAttr:      config.WindowManagerConfig.FrameAttr,
		FocusFrameCharSet:   config.WindowManagerConfig.FrameCharSet,
		WindowManagerConfig: config.WindowManagerConfig,
	}
	startText := component.StringBackgroundAttr(c.config.StartText,
		c.config.StartTextAttr, 0, c.config.StartTextBackgroundAttr)
	c.startHandler = &browserContent{Handler: FuncHandler(handler.Nop(startText), func() {}), c: c}
	c.wm.Init(c.startHandler, handlerWmConfig)
	_ = c.newWindow(c.wm.Focus()) // init handler with initial window
	c.union.Init(&c.wm)
	c.buffers = make([]*Tab, 0)

	// make sure that frame union attrs are same as window manager attrs
	c.union.Attributes = config.WindowManagerConfig.FrameAttr
	c.union.Right = config.FrameUnionCharSet.Right
	c.union.Left = config.FrameUnionCharSet.Left
	c.union.Top = config.FrameUnionCharSet.Top
	c.union.Bottom = config.FrameUnionCharSet.Bottom
	c.union.Frame = c.config.Frame

	c.tabs.SetAttr(config.FocusTabAttr, config.NonFocusTabAttr,
		config.WindowManagerConfig.FrameAttr, config.WindowManagerConfig.FrameAttr)
	c.tabs.SetFrameCharSet(config.WindowManagerConfig.FrameCharSet)
	c.tabs.SetBorder(config.WindowManagerConfig.Frame)

	// use UnionTop instead of Bar because tabs already have their own frame
	c.union.UnionTop(&c.tabs, c.barSize())

	return
}

// NewTab adds a new tab to the list of tabs on this Component.
func (c *Component) NewTab(id, name string, h tui.Handler, f io.Closer) *Tab {
	t := newTab(c, id, h, f)
	c.buffers = append(c.buffers, t)
	c.tabs.Add(name)
	c.setFocusIfStartHandler(t)
	return t
}

// Tab returns the tab with name and true if there's a tab with such name
// or nil and false otherwise.
func (c *Component) Tab(id string) (*Tab, bool) {
	for _, t := range c.buffers {
		if t.id == id {
			return t, true
		}
	}
	return nil, false
}

// SetTabAttr sets the attributes of the tab with ID id. It returns false
// if there's no tab with id.
func (c *Component) SetTabAttr(id string, attr term.Attributes) bool {
	for i, t := range c.buffers {
		if t.id == id {
			c.tabs.SetTabAttr(i, attr)
			return true
		}
	}
	return false
}

// SetTabName sets the tab name of the tab with ID id. It returns false
// if there's no tab with id.
func (c *Component) SetTabName(id string, name string) bool {
	for i, t := range c.buffers {
		if t.id == id {
			c.tabs.SetTabName(i, name)
			return true
		}
	}
	return false
}

// Tabs returns the tabs open in this browser.Component.
func (c *Component) Tabs() (ret []*Tab) {
	ret = make([]*Tab, len(c.buffers))
	for i, b := range c.buffers {
		ret[i] = b
	}
	return
}

func (c *Component) setFocusIfStartHandler(t *Tab) {
	win := c.focus()

	h := win.win.Content()
	if h == c.startHandler {
		win.SetContent(t)
	}
}

func (c *Component) closeTab(t *Tab) error {
	err := t.doClose()
	if err != nil && c.config.Logger != nil {
		c.config.Logger.Warningf("tab Close error: %v", err)
	}
	return err
}

func (c *Component) doRemoveTab(t *Tab) {
	id := c.findTabID(t)
	if !t.free {
		panic("trying to remove tab that is still attached to a window")
	}
	defer c.closeTab(t)

	c.buffers = append(c.buffers[:id], c.buffers[id+1:]...)
	ok := c.tabs.Remove(id)
	if !ok {
		panic(fmt.Sprintf("corrupted tabs: could not find tab with id %v", id))
	}
}

func (c *Component) findTabID(t *Tab) int {
	for i, f := range c.buffers {
		if f == t {
			return i
		}
	}
	panic("could not find tab")
}

func (c *Component) browserTabID(win *browserWindow) (
	*Tab, int,
) {
	t, ok := browserTabAtWindow(win)
	if !ok {
		return nil, 0
	}
	return t, c.findTabID(t)
}

func (c *Component) updateWindowTab(win *browserWindow, tabID int) bool {
	if tabID >= len(c.buffers) {
		panic(fmt.Sprintf("invalid tab at index: %d", tabID))
	}
	t := c.buffers[tabID]
	if t.free {
		c.updateWindowContent(win, t)
		return true
	}
	return false
}

// UpdateWindowTabNextFree updates win with the next available tab.
func (c *Component) UpdateWindowTabNextFree(win Window) bool {
	freeBufs := c.freeTabs()
	if len(freeBufs) != 0 {
		c.updateWindowContent(win.(*browserWindow), c.buffers[freeBufs[0]])
		return true
	}

	return false
}

// UpdateWindowTabLastFree updates win with the last available tab.
func (c *Component) UpdateWindowTabLastFree(win Window) bool {
	freeBufs := c.freeTabs()
	if len(freeBufs) != 0 {
		c.updateWindowContent(win.(*browserWindow), c.buffers[freeBufs[len(freeBufs)-1]])
		return true
	}

	return false
}

// UpdateWindowTabPrev updates win with the tab before the current tab.
func (c *Component) UpdateWindowTabPrev(win Window) bool {
	bWin := win.(*browserWindow)
	t, id := c.browserTabID(bWin)
	if t == nil {
		return c.UpdateWindowTabNextFree(win)
	}
	for i := 0; i < len(c.buffers); i++ {
		if id == 0 {
			id = len(c.buffers) - 1
		} else {
			id--
		}
		if c.updateWindowTab(bWin, id) {
			return true
		}
	}
	return false
}

// UpdateWindowTabNext updates win with the tab after the current tab.
func (c *Component) UpdateWindowTabNext(win Window) bool {
	bWin := win.(*browserWindow)
	t, id := c.browserTabID(bWin)
	if t == nil {
		return c.UpdateWindowTabNextFree(win)
	}
	for i := 0; i < len(c.buffers); i++ {
		id++
		if id == len(c.buffers) {
			id = 0
		}
		if c.updateWindowTab(bWin, id) {
			return true
		}
	}
	return false
}

func (c *Component) tryLog(msg string, args ...interface{}) {
	if c.config.Logger == nil {
		return
	}
	c.config.Logger.Debugf(msg, args...)
}

func (c *Component) onUnmount(h Handler, reason string) {
	err := h.OnUnmount()
	if err != nil && c.config.Logger != nil {
		c.config.Logger.Warningf("OnUnmount error: %v", err)
	}
	c.tryLog("Component.OnUnmount(%p): reason: %s", h, reason)
}

func (c *Component) tryUpdateWindowContent(
	win *browserWindow, content Handler,
) error {
	if b, ok := content.(*Tab); ok {
		if !b.free {
			return ErrTabNotFree
		}
	}
	c.updateWindowContent(win, content)
	return nil
}

func (c *Component) updateWindowContent(
	win *browserWindow, content Handler,
) Handler {
	newBuf, ok := content.(*Tab)
	if ok {
		id := c.findTabID(newBuf)
		c.tabs.SetFocus(id)
		newBuf.setWindow()
	} else {
		content = &browserContent{
			Handler: content,
			c:       c,
		}
	}
	oldComponent := win.win.SetContent(content).(Handler)
	reason := fmt.Sprintf("window content was updated: %p", win)
	c.onUnmount(oldComponent, reason)
	return oldComponent
}

func browserTabAtWindow(win *browserWindow) (*Tab, bool) {
	t, ok := win.win.Content().(*Tab)
	return t, ok
}

// RemoveAllTabs removes all tabs but the last one.
func (c *Component) RemoveAllTabs() {
	for _, t := range c.buffers {
		c.closeTab(t)
	}

	c.wm.Iterate(func(w handler.Window) {
		win, ok := c.findWindow(w.ID())
		if !ok {
			panic("corrupted browser: could not find WindowManager window")
		}
		c.updateWindowContent(win, c.startHandler)
	})

	c.tabs.RemoveAll()
	c.buffers = c.buffers[:0]
}

func (c *Component) freeTabs() []int {
	freeBufs := make([]int, 0)
	for i, b := range c.buffers {
		if b.free {
			freeBufs = append(freeBufs, i)
		}
	}
	return freeBufs
}

// returns the next free tab or an empty Handler
func (c *Component) getFreeTab() (Handler, bool) {
	freeBufs := c.freeTabs()
	if len(freeBufs) == 0 {
		return c.startHandler, false
	}

	id := freeBufs[0]
	return c.buffers[id], true
}

// RemoveWindowContent removes the content at win. It returns false
// if content was replaced with start handler because the content at win
// was the last content in this Component.
func (c *Component) RemoveWindowContent(win Window) bool {
	t, isNotStartHandler := c.getFreeTab()
	oldComponent := c.updateWindowContent(win.(*browserWindow), t)
	oldTab, ok := oldComponent.(*Tab)
	if ok {
		c.doRemoveTab(oldTab)
	}
	return isNotStartHandler
}

// TODO RemoveTab removes t from this Component. It returns false
// if this tab was active in one of the windows and the replacement
// is the start handler, because all tabs have been removed.
// func (c *Component) RemoveTab(t *Tab) bool {
// }

func (c *Component) splitRegular(
	split func(*handler.WindowManager, tui.Handler) (handler.Window, bool),
	newHandler Handler,
) (*browserWindow, bool) {
	win := c.split(split, newHandler)
	if win == nil {
		return nil, false
	}
	c.wm.SetFocus(win.win)
	return win, true
}

func (c *Component) splitInverted(
	split func(*handler.WindowManager, tui.Handler) (handler.Window, bool),
	newHandler Handler,
) (*browserWindow, bool) {
	focusBrowserWin := c.focus()
	focusHandlerWin := focusBrowserWin.win
	focusHandler := focusBrowserWin.win.Content()

	// perform a regular split
	newBrowserWin := c.split(split, newHandler)
	if newBrowserWin == nil {
		return nil, false
	}
	newHandlerWin := newBrowserWin.win
	newBrowserHandler := newHandlerWin.Content()

	// switch underlying handler.Window
	// so the new *browserWindow refers to the
	// original focus handler.Window
	focusBrowserWin.win = newHandlerWin
	newBrowserWin.win = focusHandlerWin

	// switch content
	newBrowserWin.win.SetContent(newBrowserHandler)
	focusBrowserWin.win.SetContent(focusHandler)

	// ammend id mapping
	c.windows[newBrowserWin.id()] = newBrowserWin
	c.windows[focusBrowserWin.id()] = focusBrowserWin

	// return new instance of browser window
	// pointing to old instance of focus window
	return newBrowserWin, true
}

func (c *Component) newWindowContent(h Handler) Handler {
	if t, ok := h.(*Tab); ok {
		t.setWindow()
	} else {
		h = &browserContent{
			Handler: h,
			c:       c,
		}
	}
	return h
}

func (c *Component) split(
	split func(*handler.WindowManager, tui.Handler) (handler.Window, bool),
	h Handler,
) *browserWindow {
	h = c.newWindowContent(h)
	win, ok := split(&c.wm, h)
	if !ok {
		return nil
	}
	return c.newWindow(win)
}

// Split splits the current window in two and installs h to the orientation
// of the original content.
//
// Note that if h is not a handler created with NewTab
// the handler is cleaned as soon as the window's content is swapped.
func (c *Component) Split(o Orientation, h Handler) (Window, bool) {
	switch o {
	case OrientationRight:
		return c.splitRegular((*handler.WindowManager).SplitVertical, h)
	case OrientationLeft:
		return c.splitInverted((*handler.WindowManager).SplitVertical, h)
	case OrientationTop:
		return c.splitInverted((*handler.WindowManager).SplitHorizontal, h)
	case OrientationBottom:
		return c.splitRegular((*handler.WindowManager).SplitHorizontal, h)
	default:
		panic("not a valid orientation")
	}
}

// Floating opens a new floating window at the given coordinates,
// with the given height and width.
func (c *Component) Floating(
	h Handler, at term.Coordinates, width, height int,
) Window {
	h = c.newWindowContent(h)
	win := c.newWindow(c.wm.FloatingWindow(h, at, width, height))
	c.wm.SetFocus(win.win)
	return win
}

func (c *Component) barSize() int {
	if c.config.Frame {
		return 3
	}
	return 1
}

// Bar adds a bar to the orientation of the main window.
// TODO continue
func (c *Component) Bar(o Orientation, h tui.Handler) {
	if c.config.Frame {
		f := handler.NewFrame(h)
		f.FrameCharSet = c.config.FrameCharSet
		f.Attributes = c.config.FrameAttr
		//if cfg.FrameCharSet != nil {
		//	f.FrameCharSet = *cfg.FrameCharSet
		//}
		//if cfg.FrameCharSetAttr != nil {
		//	f.Attributes= *cfg.FrameCharSetAttr
		//}
		h = f
	}
	size := c.barSize()
	switch o {
	case OrientationTop:
		c.union.UnionTop(h, size)
	case OrientationBottom:
		c.union.UnionBottom(h, size)
	case OrientationLeft:
		c.union.UnionLeft(h, size)
	case OrientationRight:
		c.union.UnionRight(h, size)
	}
}

func (c *Component) setError(err error) {
	c.SetMessage("Error: %s", err)
}

// SetMessage formats the given msg and args and displays it on next Draw.
func (c *Component) SetMessage(msg string, args ...interface{}) {
	msg = fmt.Sprintf(msg, args...)
	if c.config.Logger != nil {
		c.config.Logger.Infof("Message: %s", msg)
	}
	c.logBuf.Reset()
	c.logBuf.WriteString(msg)
	c.logBufDraw = logBufDrawTimes
}

// Resize satisfies tui.Component
func (c *Component) Resize(width, height int) {
	ResizeMessageSpan(&c.logVirt, width, height)
	c.union.Resize(width, height)
}

// Draw satisfies tui.Component
func (c *Component) Draw(w term.Writer) {
	c.tabs.ResetFocus()
	for id, t := range c.buffers {
		if !t.free {
			c.tabs.SetFocus(id)
		}
	}

	c.union.Draw(w)

	// only draw logBufDraw times
	if c.logBufDraw > 0 {
		c.logVirt.Draw(w)
		c.logBufDraw--
	} else {
		c.logBuf.Reset()
	}
}

// ShiftFocus calls the underlying WindowManager.ShiftFocus.
func (c *Component) ShiftFocus() bool {
	return c.wm.ShiftFocus()
}

// Focus calls the underlying WindowManager.Focus.
func (c *Component) Focus() Window {
	return c.focus()
}

// FocusTab returns the Tab corresponding to the current window in focus,
// or false if the current window in focus is not drawing a Tab.
func (c *Component) FocusTab() (*Tab, bool) {
	w := c.focus()
	h, _ := w.Content()
	t, ok := h.(*Tab)
	return t, ok
}

func (c *Component) focus() *browserWindow {
	win, ok := c.findWindow(c.wm.Focus().ID())
	if !ok {
		panic("corrupted browser: cannot find focus window")
	}
	return win
}

// Shiftable calls the underlying WindowManager.Shiftable.
func (c *Component) Shiftable() (Window, bool) {
	win, ok := c.wm.Shiftable()
	if !ok {
		return nil, false
	}

	w, ok := c.findWindow(win.ID())
	if !ok {
		panic("corrupted browser: cannot find focus window")
	}
	return w, true
}

// FocusDown calls the underlying WindowManager.FocusDown.
func (c *Component) FocusDown() bool {
	return c.wm.FocusDown()
}

// FocusLeft calls the underlying WindowManager.FocusLeft.
func (c *Component) FocusLeft() bool {
	return c.wm.FocusLeft()
}

// FocusRight calls the underlying WindowManager.FocusRight.
func (c *Component) FocusRight() bool {
	return c.wm.FocusRight()
}

// FocusUp calls the underlying WindowManager.FocusUp.
func (c *Component) FocusUp() bool {
	return c.wm.FocusUp()
}

// Close closes the resources associated with this browser.
func (c *Component) Close() (ret error) {
	for _, f := range c.buffers {
		err := c.closeTab(f)
		if err != nil {
			ret = err
		}
	}
	c.buffers = c.buffers[:0]
	return ret
}

// Handle proxies events to either the underlying Tabs or WindowManager.
func (c *Component) Handle(ev term.Event) (exit, handled bool) {
	return c.union.Handle(ev)
}

// Cursor calls the underlying FrameUnion's Cursor.
func (c *Component) Cursor() (pos term.Coordinates, show bool) {
	return c.union.Cursor()
}
