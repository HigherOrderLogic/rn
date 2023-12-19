package browser

import (
	"fmt"
	"io"

	"github.com/ernestrc/blue/logging"
	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/notifications"

	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/term"
)

var _ browserapi.Handler = (*Component)(nil)

// Component renders a browser-like tui.Compontent and exposes an API
// to open new windows, add new tabs, and switch between tabs.
//
// All tui.Handlers installed other than via NewTab are considered ephemeral,
// and will be destroyed either when windows close or when they return exit=true
// to a call to Handle. Conversely, tui.Handlers installed via NewTab
// will remain as a tab and can be managed independently from windows.
type Component struct {
	tabs      handler.Tabs
	wm        handler.WindowManager
	union     handler.FrameUnion
	width     int
	height    int
	nextSplit browserapi.Orientation
	container notifications.Container

	focusWindow handler.Window
	config      Config
	buffers     []*Tab
	windows     map[uint64]*browserWindow
}

// component.WindowManager sinchronously removes tui.Handlers
// upon returning exit=true on calls to Handle. This
// structure is used to call Close when this occurs.
type browserContent struct {
	browserapi.Handler
	closed bool
	c      *Component
}

func (c *browserContent) Dimensions() (int, int) {
	return c.Handler.(Floating).Dimensions()
}

func (c *browserContent) Handle(ev term.Event) (exit, handled bool) {
	prev := c.c.focusWindow.Content()
	win := c.c.focusWindow
	exit, handled = c.Handler.Handle(ev)
	if exit {
		c.c.closeHandler(c)
		content := win.Content()
		// if previous content is not the same as the new content, then it must
		// mean that the underlying Handler swapped the content before exiting.
		// This window is going away (see handler.WindowManager) so make sure that
		// a non-ephemeral handler (Tab), is set free.
		if prev != content {
			if t, ok := content.(*Tab); ok {
				t.setFree()
			}
		}
	}
	return
}

func (c *browserContent) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.Handler.Close()
}

func (c *Component) newWindow(win handler.Window) *browserWindow {
	browserWin := &browserWindow{
		parent: c,
		win:    win,
	}
	c.windows[browserWin.ID()] = browserWin
	c.log(log.TraceLevel, "new window: %d", win.ID())
	return browserWin
}

// closeWindow closes win or returns an error if win is the last Window.
func (c *Component) closeWindow(win *browserWindow) error {
	_, ok := c.findWindow(win.ID())
	if !ok {
		panic("window not found")
	}

	content := win.win.Content().(browserapi.Handler)
	err := win.win.Close()
	c.log(log.TraceLevel, "closing window: %d, %v", win.ID(), err)
	if err != nil {
		return err
	}

	delete(c.windows, win.ID())

	// ensure that if handler calls Closed to ensure that
	// window is closed, the answer will be correct.
	win.parent = nil
	c.releaseHandler(content)
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

func (c *Component) wallpaper() browserapi.Handler {
	strcfg := component.StringConfig{
		Attributes:           c.config.WallpaperAttr,
		BackgroundAttributes: c.config.WallpaperBackgroundAttr,
		Alignment:            component.SpanAlignmentCentered,
	}
	wallpaper := component.NewStringWithConfig(c.config.Wallpaper, strcfg)
	return &browserContent{
		// make wallpaper satisfy Floating to avoid browserContent panic
		// if wallpaper is being set as a default on a floating window
		Handler: NopFloatingHandler(handler.NopFloatingHandler(wallpaper)),
		c:       c,
	}
}

// satisfies handler.WindowSubscriber to
// override union attrs of focus window
type wmSubscriber Component

func (s *wmSubscriber) OnFocus(prev, focus handler.Window) {
	c := (*Component)(s)
	c.focusWindow = focus
}

// Init initializes this Component with config.
func (c *Component) Init(config Config) {
	c.config = config
	c.windows = make(map[uint64]*browserWindow)

	c.nextSplit = browserapi.OrientationRight

	c.tabs.Init()
	c.tabs.OnClick = func(id int) {
		t := c.buffers[id]
		err := c.tryUpdateWindowContent(c.Focus().(*browserWindow), t)
		if err != nil {
			c.setError(err)
		}
	}

	c.wm.Init(c.wallpaper(), config.WindowManagerConfig)
	c.focusWindow = c.wm.Focus()
	_ = c.newWindow(c.focusWindow) // init handler with initial window
	c.union.Init(&c.wm)
	c.wm.Subscribe((*wmSubscriber)(c))
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

	c.container.Init(&c.union, config.Notifications)

	return
}

// NewTab adds a new tab to the list of tabs on this Component.
func (c *Component) NewTab(resource workspaceapi.URI, name string, h browserapi.Handler, f io.Closer) *Tab {
	t := newTab(c, resource, h, f)
	c.buffers = append(c.buffers, t)
	c.tabs.Add(name)
	return t
}

// Tab returns the tab with name and true if there's a tab with such name
// or nil and false otherwise.
func (c *Component) Tab(uri workspaceapi.URI) (*Tab, bool) {
	for _, t := range c.buffers {
		if t.uri.String() == uri.String() {
			return t, true
		}
	}
	return nil, false
}

// SetTabAttr sets the attributes of the tab with ID id. It returns false
// if there's no tab with id.
func (c *Component) SetTabAttr(uri workspaceapi.URI, attr term.Attributes) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.SetTabAttr(i, attr)
			return true
		}
	}
	return false
}

// SetTabName sets the tab name of the tab with ID id. It returns false
// if there's no tab with id.
func (c *Component) SetTabName(uri workspaceapi.URI, name string) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
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

func (c *Component) closeTab(t *Tab) error {
	err := t.Close()
	if err != nil {
		c.log(log.WarnLevel, "tab Close error: %v", err)
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

func (c *Component) updateWithNextFreeTab(win Window) bool {
	freeBufs := c.freeTabs()
	if len(freeBufs) != 0 {
		c.updateWindowContent(win.(*browserWindow), c.buffers[freeBufs[0]])
		return true
	}

	return false
}

// PreviousTab updates win with the tab before the current tab.
func (c *Component) PreviousTab(win Window) bool {
	bWin := win.(*browserWindow)
	// already closed
	if bWin.parent == nil {
		return false
	}
	t, id := c.browserTabID(bWin)
	if t == nil {
		return c.updateWithNextFreeTab(win)
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

// NextTab updates win with the tab after the current tab.
func (c *Component) NextTab(win Window) bool {
	bWin := win.(*browserWindow)
	// already closed
	if bWin.parent == nil {
		return false
	}
	t, id := c.browserTabID(bWin)
	if t == nil {
		return c.updateWithNextFreeTab(win)
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

func (c *Component) log(level log.Level, msg string, args ...interface{}) {
	log.
		WithField(logging.KeyClass, "browser.Component").Logf(level, msg, args...)
}

func (c *Component) closeHandler(h browserapi.Handler) {
	err := h.Close()
	if err != nil {
		c.log(log.WarnLevel, "Close error: %v", err)
	}
	c.log(log.DebugLevel, "Component.closeHandler(%p)", h)
}

func (c *Component) tryUpdateWindowContent(
	win *browserWindow, content browserapi.Handler,
) error {
	if b, ok := content.(*Tab); ok {
		if !b.free {
			return browserapi.ErrTabNotFree
		}
	}
	c.updateWindowContent(win, content)
	return nil
}

func (c *Component) updateWindowContent(
	win *browserWindow, content browserapi.Handler,
) browserapi.Handler {
	tab, ok := content.(*Tab)
	if ok {
		id := c.findTabID(tab)
		c.tabs.SetFocus(id)
		tab.setWindow(win)
	} else if _, ok := content.(*browserContent); !ok {
		content = &browserContent{
			Handler: content,
			c:       c,
		}
	}
	oldComponent := win.win.SetContent(content).(browserapi.Handler)
	// first call OnFree
	c.releaseHandler(oldComponent)
	// then call OnFocus if applicable
	if ok {
		tab.callOnFocus()
	}
	return oldComponent
}

// only tabs are able to be re-installed after content is updated.
func (c *Component) releaseHandler(h browserapi.Handler) {
	if t, ok := h.(*Tab); ok {
		t.setFree()
	} else {
		c.closeHandler(h)
	}
}

func browserTabAtWindow(win *browserWindow) (*Tab, bool) {
	t, ok := win.win.Content().(*Tab)
	return t, ok
}

// RemoveAllTabs removes all tabs but the last one.
func (c *Component) RemoveAllTabs() {
	c.wm.Iterate(func(w handler.Window) {
		win, ok := c.findWindow(w.ID())
		if !ok {
			panic("corrupted browser: could not find WindowManager window")
		}
		for c.RemoveWindowContent(win) {
		}
	})
}

// Window returns the window with the given ID or false if there's
// no window with the given ID.
func (c *Component) Window(id uint64) (Window, bool) {
	ret, ok := c.findWindow(id)
	c.log(log.TraceLevel, "find window: %d, %v, %v", id, ret, ok)
	return ret, ok
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
func (c *Component) getFreeTab() (browserapi.Handler, bool) {
	freeBufs := c.freeTabs()
	if len(freeBufs) == 0 {
		return c.wallpaper(), false
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

func (c *Component) splitRegular(
	split func(*handler.WindowManager, handler.Window, tui.Handler) (handler.Window, bool),
	splitWindow Window,
	newHandler browserapi.Handler,
) (*browserWindow, bool) {
	win := c.split(split, splitWindow, newHandler)
	if win == nil {
		return nil, false
	}
	c.wm.SetFocus(win.win)
	return win, true
}

func (c *Component) splitInverted(
	split func(*handler.WindowManager, handler.Window, tui.Handler) (handler.Window, bool),
	splitWindow Window,
	newHandler browserapi.Handler,
) (*browserWindow, bool) {
	focusBrowserWin := c.focus()
	focusHandlerWin := focusBrowserWin.win
	focusHandler := focusBrowserWin.win.Content()

	// perform a regular split
	newBrowserWin := c.split(split, splitWindow, newHandler)
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
	c.windows[newBrowserWin.ID()] = newBrowserWin
	c.windows[focusBrowserWin.ID()] = focusBrowserWin

	// return new instance of browser window
	// pointing to old instance of focus window
	return newBrowserWin, true
}

func (c *Component) newWindowContent(h browserapi.Handler) (browserapi.Handler, bool) {
	if h == nil {
		h = c.wallpaper()
	}
	_, ok := h.(*Tab)
	if !ok {
		h = &browserContent{
			Handler: h,
			c:       c,
		}
	}
	return h, ok
}

func (c *Component) split(
	split func(*handler.WindowManager, handler.Window, tui.Handler) (handler.Window, bool),
	splitWin Window, h browserapi.Handler,
) *browserWindow {
	h, isTab := c.newWindowContent(h)
	win, ok := split(&c.wm, splitWin.(*browserWindow).win, h)
	if !ok {
		return nil
	}
	ret := c.newWindow(win)
	if isTab {
		h.(*Tab).setWindow(ret)
		h.(*Tab).callOnFocus()
	}
	return ret
}

// SetDefaultSplit sets the default split to be used when Split
// is invoked with OrientationDefault.
func (c *Component) SetDefaultSplit(o browserapi.Orientation) browserapi.Orientation {
	if o == browserapi.OrientationDefault {
		panic("cannot set OrientationDefault as default orientation")
	}
	ret := c.nextSplit
	c.nextSplit = o
	return ret
}

// Split splits the current window in two and installs h to the orientation
// of the original content.
//
// Note that if h is not a handler created with NewTab
// the handler is cleaned as soon as the window's content is swapped.
func (c *Component) Split(o browserapi.Orientation, win Window, h browserapi.Handler) (Window, bool) {
	if win.(*browserWindow).parent == nil {
		panic("trying to split over a closed window")
	}
	if o == browserapi.OrientationDefault {
		o = c.nextSplit
	}
	switch o {
	case browserapi.OrientationRight:
		return c.splitRegular((*handler.WindowManager).SplitVertical, win, h)
	case browserapi.OrientationLeft:
		return c.splitInverted((*handler.WindowManager).SplitVertical, win, h)
	case browserapi.OrientationTop:
		return c.splitInverted((*handler.WindowManager).SplitHorizontal, win, h)
	case browserapi.OrientationBottom:
		return c.splitRegular((*handler.WindowManager).SplitHorizontal, win, h)
	default:
		panic("not a valid orientation")
	}
}

// Floating opens a new floating window at the given coordinates,
// with the given height and width.
func (c *Component) Floating(
	h Floating, cfg component.FloatingConfig,
) Window {
	if h == nil {
		panic("nil Floating handler")
	}
	h = &browserContent{
		Handler: h,
		c:       c,
	}
	win := c.newWindow(c.wm.FloatingWindow(h, cfg))
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
func (c *Component) Bar(o browserapi.Orientation, h tui.Handler) {
	if c.config.Frame {
		f := handler.NewFrame(h)
		f.FrameCharSet = c.config.FrameCharSet
		f.Attributes = c.config.FrameAttr
		h = f
	}
	size := c.barSize()
	if o == browserapi.OrientationDefault {
		o = c.nextSplit
	}
	switch o {
	case browserapi.OrientationTop:
		c.union.UnionTop(h, size)
	case browserapi.OrientationBottom:
		c.union.UnionBottom(h, size)
	case browserapi.OrientationLeft:
		c.union.UnionLeft(h, size)
	case browserapi.OrientationRight:
		c.union.UnionRight(h, size)
	}
}

func (c *Component) setError(err error) {
	c.Notify(notifications.LevelError, "Error: %s", err)
}

// Notify formats the given msg and args and displays it on next Draw.
func (c *Component) Notify(level notifications.Level, msg string, args ...interface{}) {
	c.container.Notify(level, fmt.Sprintf(msg, args...))
}

// Resize satisfies tui.Component
func (c *Component) Resize(width, height int) {
	c.width, c.height = width, height
	c.container.Resize(width, height)
}

func (c *Component) overwriteFocusWindowUnion(w term.Writer) {
	if !c.config.Frame || c.focusWindow == (handler.Window{}) || c.wm.SizeTiles() == 1 {
		return
	}
	topleft := c.focusWindow.Position()
	mainPos := c.union.MainPosition()
	mainWidth := c.union.MainWidth()
	mainHeight := c.union.MainHeight()
	attr := c.config.WindowManagerConfig.FocusFrameAttr
	cs := c.config.WindowManagerConfig.FocusFrameCharSet

	if topleft == (term.Coordinates{}) {
		cell := term.Cell{Ch: cs.TopLeft, Bg: attr.Bg, Fg: attr.Fg}
		w.SetCell(mainPos, cell)
	}

	topright := term.Coordinates{X: topleft.X + c.focusWindow.Width(), Y: topleft.Y}
	if topright == (term.Coordinates{X: mainWidth, Y: 0}) {
		cell := term.Cell{Ch: cs.TopRight, Bg: attr.Bg, Fg: attr.Fg}
		pos := term.Coordinates{X: mainPos.X + mainWidth - 1, Y: mainPos.Y}
		w.SetCell(pos, cell)
	}

	bottomleft := term.Coordinates{X: topleft.X, Y: topleft.Y + c.focusWindow.Height()}
	if bottomleft == (term.Coordinates{Y: mainHeight, X: 0}) {
		cell := term.Cell{Ch: cs.BottomLeft, Bg: attr.Bg, Fg: attr.Fg}
		pos := term.Coordinates{X: mainPos.X, Y: mainPos.Y + mainHeight - 1}
		w.SetCell(pos, cell)
	}

	bottomright := term.Coordinates{X: topleft.X + c.focusWindow.Width(), Y: topleft.Y + c.focusWindow.Height()}
	if bottomright == (term.Coordinates{Y: mainHeight, X: mainWidth}) {
		cell := term.Cell{Ch: cs.BottomRight, Bg: attr.Bg, Fg: attr.Fg}
		pos := term.Coordinates{X: mainPos.X + mainWidth - 1, Y: mainPos.Y + mainHeight - 1}
		w.SetCell(pos, cell)
	}
}

// Draw satisfies tui.Component
func (c *Component) Draw(w term.Writer) {
	c.tabs.ResetFocus()
	for id, t := range c.buffers {
		if !t.free {
			c.tabs.SetFocus(id)
		}
	}

	c.container.Draw(w)

	// set correct attributes for focus window union charset
	c.overwriteFocusWindowUnion(w)
}

// SetDim sets whether next call to draw should use
// non-focus window diming feature.
func (c *Component) SetDim(to bool) bool {
	return c.wm.SetDim(to)
}

// WindowManagerPosition returns the offset from the top left corner
// where the underlying window manager starts.
func (c *Component) WindowManagerPosition() term.Coordinates {
	return c.union.FrameUnion.MainPosition()
}

// WindowManagerSize returns the size of the window manager.
func (c *Component) WindowManagerSize() (width, height int) {
	return c.union.FrameUnion.MainWidth(), c.union.FrameUnion.MainHeight()
}

// DrawWindow draws target with the given term.Writer.
func (c *Component) DrawWindow(target Window, w term.Writer) {
	// emulate union draw
	pos := c.WindowManagerPosition()
	width, height := c.WindowManagerSize()
	w = component.VirtualWriter(w, pos, height, width)
	c.wm.DrawWindow(target.(*browserWindow).win, w)
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
// It returns the Window that would become in focus if ShiftFocus
// is called.
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

// SetFocus sets the underlying WindowManager's focus to win.
func (c *Component) SetFocus(win Window) Window {
	bwin := win.(*browserWindow)
	if bwin.parent == nil {
		panic("trying to set focus to a closed window")
	}
	prev := c.wm.SetFocus(bwin.win)
	ret, ok := c.findWindow(prev.ID())
	if !ok {
		panic("could not find previously set focus")
	}
	return ret
}

// Handle proxies events to either the underlying Tabs or WindowManager.
func (c *Component) Handle(ev term.Event) (exit, handled bool) {
	// container Handles only mouse events so it's not a full tui.Handler.
	// try to handle first and if it doesn't fallback handling to union.
	_, handled = c.container.Handle(ev)
	if handled {
		return
	}
	return c.union.Handle(ev)
}

// Cursor calls the underlying FrameUnion's Cursor.
func (c *Component) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return c.union.Cursor()
}

// Prompt creates a new prompt to be drawn as an overlay on the next call to Draw
// and it also takes over event control until user either exits prompt or selects
// an option. The passed options and bindings must be equal in length, or bindings
// must be zero in length, meaning no key bindings are provided to user.
// Callers must ensure that there's coherence between options and bindings, otherwise
// this method panics.
func (c *Component) Prompt(
	message string, options []string,
	bindings []term.KeyComb,
	cb func(int, string),
) Window {
	if len(options) == 0 || (len(bindings) != 0 && len(options) != len(bindings)) {
		panic("Prompt given invalid options and/or bindings")
	}
	promptConfig := handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message:              message,
			Options:              options,
			Frame:                c.config.FrameCharSet,
			BackgroundAttributes: c.config.PromptConfig.BackgroundAttr,
			MinWidth:             c.config.PromptConfig.MinWidth,
		},
		OptionBindings: bindings,
		OptionCallback: cb,
		OptionAttr:     c.config.PromptConfig.TextAttr,
		HighlightAttr:  c.config.PromptConfig.HighlightAttr,
	}
	if c.config.Frame {
		promptConfig.Frame = component.FrameCharSetDefault()
	}

	floatingConfig := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
	}

	return c.Floating(NopFloatingHandler(handler.NewPrompt(promptConfig)), floatingConfig)
}

// Subscribe subscribes sub to window focus events.
func (c *Component) Subscribe(sub handler.WindowSubscriber) {
	c.wm.Subscribe(sub)
}

// Tiles returns the number of tiled windows in this Component.
func (c *Component) Tiles() int {
	return c.wm.SizeTiles()
}

// FloatingWindows returns the number of floating windows in this Component.
func (c *Component) FloatingWindows() int {
	return c.wm.SizeFloating()
}

// Close closes the resources associated with this browser.
func (c *Component) Close() (ret error) {
	for _, f := range c.buffers {
		err := c.closeTab(f)
		if err != nil {
			ret = err
		}
	}
	if err := c.container.Close(); err != nil {
		ret = multierror.Append(ret, err)
	}
	c.buffers = c.buffers[:0]
	c.wm.UnsubscribeAll()

	c.wm.Iterate(func(w handler.Window) {
		// call Close on all browser handlers
		_ = c.newWindow(w).Close()
	})
	return ret
}

// Man satisfies tui.Handler
func (c *Component) Man() tui.Manual {
	panic("TODO")
}

// CloseNotifications closes all open notifications.
func (c *Component) CloseNotifications() {
	c.container.CloseAll()
}

// PauseNotifications pauses auto-close on all open notifications.
func (c *Component) PauseNotifications() {
	c.container.PauseAll()
}

// ResumeNotifications resumes auto-close on all open notifications.
func (c *Component) ResumeNotifications() {
	c.container.ResumeAll()
}
