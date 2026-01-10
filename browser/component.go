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

package browser

import (
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/workspaceapi"
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

	dirtyTabs   bool
	focusWindow handler.Window
	config      Config
	buffers     []*Tab
	windows     map[uint64]*browserWindow
	prompts     map[string]Window
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

	c.nextSplit = browserapi.OrientationRight
	c.prompts = make(map[string]Window)

	c.tabs.Init()
	c.tabs.OnClick = func(id int) (ret bool) {
		if config.OnTabsClick != nil {
			ret = config.OnTabsClick(id)
		}
		if id < 0 || id >= len(c.buffers) {
			return ret
		}
		t := c.buffers[id]
		bwin := c.Focus().(*browserWindow)
		err := c.tryUpdateWindowContent(bwin, t, bwin.win.Content().(browserapi.Handler))
		if err != nil && err != browserapi.ErrTabNotFree {
			c.setError(err)
		}
		return true
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
	c.union.Frame = c.config.Frame && c.config.FrameUnion

	c.tabs.SetAttr(config.FocusTabAttr, config.NonFocusTabAttr,
		config.WindowManagerConfig.FrameAttr, config.WindowManagerConfig.FrameAttr)
	c.tabs.SetFrameCharSet(config.WindowManagerConfig.FrameCharSet)

	// if tab bar offset is set, the remove frame from tabs
	// and install via union and no frame unioning.
	if config.TabBarOffset > 0 {
		vtabs := &handler.Virtual[*handler.Tabs]{Virtual: component.Virtual[*handler.Tabs]{C: &c.tabs}}
		vtabs.Move(term.Coordinates{X: config.TabBarOffset})
		c.tabs.SetBorder(false)
		c.union.UnionTopFrame(vtabs, c.tabsSize(), false)
	} else {
		c.tabs.SetBorder(config.Frame)
		c.union.UnionTop(&c.tabs, c.tabsSize())
	}
	if config.TabNameSeparator != "" {
		c.tabs.SetNameSeparator(config.TabNameSeparator)
	}
}

// NewTab adds a new tab to the list of tabs on this Component.
func (c *Component) NewTab(
	resource workspaceapi.URI, icon rune, name string,
	h browserapi.Handler, f io.Closer,
) *Tab {
	t := newTab(c, resource, h, f)
	c.buffers = append(c.buffers, t)
	c.tabs.Add(icon, name)
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

// TabName returns the name and default name of the tab with the given uri or false
// if there's no tab with the given uri.
func (c *Component) TabName(uri workspaceapi.URI) (string, string, bool) {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			return c.tabs.TabName(i), c.tabs.DefaultTabName(i), true
		}
	}
	return "", "", false
}

// TabAttrs returns the attributes of the tab with the given uri or false
// if there's no tab with the given uri.
func (c *Component) TabAttrs(uri workspaceapi.URI) (term.Attributes, bool) {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			return c.tabs.TabAttr(i), true
		}
	}
	return term.Attributes{}, false
}

// SetTabNameAndAttrs overrides the name and attributes of the tab with the given uri.
// It returns false if there's no tab with id.
func (c *Component) SetTabNameAndAttrs(
	uri workspaceapi.URI, name string, attr term.Attributes,
) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.SetTabName(i, name)
			c.tabs.SetTabAttr(i, attr)
			return true
		}
	}
	return false
}

// ResetTabNameAndAttrs resets the name and attributes of the tab with the given uri.
// Moving forward, the name and attributes and name set via
// SetTabDefaultNameAndAttrs will be used. It returns false if there's no tab with id.
func (c *Component) ResetTabNameAndAttrs(uri workspaceapi.URI) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.ResetTabName(i)
			c.tabs.ResetTabAttr(i)
			return true
		}
	}
	return false
}

// SetTabDefaultNameAndAttrs resets the attributes and name of the
// tab with the given uri. It returns false if there's no tab with id.
func (c *Component) SetTabDefaultNameAndAttrs(
	uri workspaceapi.URI, name string, attr term.Attributes,
) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.SetTabDefaultAttr(i, attr)
			c.tabs.SetTabDefaultName(i, name)
			return true
		}
	}
	return false
}

// Tabs returns the tabs open in this browser.Component.
func (c *Component) Tabs() (ret []*Tab) {
	ret = make([]*Tab, len(c.buffers))
	copy(ret, c.buffers)
	return
}

// MoveTabLeft moves the tab in the given window to the left
// of the tabs list.
func (c *Component) MoveTabLeft(win Window) error {
	bWin := win.(*browserWindow)
	t, ok := browserTabAtWindow(bWin)
	if !ok {
		return errors.New("window content is not a tab")
	}
	idx := c.findTabID(t)
	if idx == 0 {
		return errors.New("tab is already at the start of the list")
	}
	if c.tabs.MoveLeft(idx) {
		c.doRemoveTab(idx)
		c.doInsertTab(idx-1, t)
	}
	return nil
}

// MoveTabRight moves the tab in the given window to the right
// of the tabs list.
func (c *Component) MoveTabRight(win Window) error {
	bWin := win.(*browserWindow)
	t, ok := browserTabAtWindow(bWin)
	if !ok {
		return errors.New("window content is not a tab")
	}
	idx := c.findTabID(t)
	if idx == c.tabs.Size()-1 {
		return errors.New("tab is already at the end of the list")
	}
	if c.tabs.MoveRight(idx) {
		c.doRemoveTab(idx)
		c.doInsertTab(idx+1, t)
	}
	return nil
}

// MoveTabTo moves the tab in the given window to the given position.
func (c *Component) MoveTabTo(win Window, idx int) error {
	bWin := win.(*browserWindow)
	t, ok := browserTabAtWindow(bWin)
	if !ok {
		return errors.New("window content is not a tab")
	}
	if idx > c.tabs.Size()-1 {
		idx = c.tabs.Size() - 1
	}
	curridx := c.findTabID(t)
	if c.tabs.MoveTo(curridx, idx) {
		c.doRemoveTab(curridx)
		c.doInsertTab(idx, t)
		return nil
	}
	return errors.New("")
}

// PreviousTab updates win with the tab before the current tab.
func (c *Component) PreviousTab(win Window) bool {
	bWin := win.(*browserWindow)
	// already closed
	if bWin.parent == nil {
		return false
	}
	prev := bWin.win.Content().(browserapi.Handler)
	t, id := c.browserTabID(bWin)
	if t == nil {
		return c.updateWithNextFreeTab(win, prev)
	}
	for i := 0; i < len(c.buffers); i++ {
		if id == 0 {
			id = len(c.buffers) - 1
		} else {
			id--
		}
		if c.updateWindowTab(bWin, id, prev) {
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
	prev := bWin.win.Content().(browserapi.Handler)
	t, id := c.browserTabID(bWin)
	if t == nil {
		return c.updateWithNextFreeTab(win, prev)
	}
	for i := 0; i < len(c.buffers); i++ {
		id++
		if id == len(c.buffers) {
			id = 0
		}
		if c.updateWindowTab(bWin, id, prev) {
			return true
		}
	}
	return false
}

// SetContentToTab updates win with the tab at the given index.
func (c *Component) SetContentToTab(win Window, tabIdx int) bool {
	bWin := win.(*browserWindow)
	if tabIdx < 0 {
		panic("negative tab index")
	}
	if tabIdx >= len(c.buffers) ||
		bWin.parent == nil {
		return false
	}
	prev := bWin.win.Content().(browserapi.Handler)
	return c.updateWindowTab(bWin, tabIdx, prev)
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

// RemoveInactiveTabs removes all tabs that aren't used by any window.
// If no tabs were closed then false is returned.
func (c *Component) RemoveInactiveTabs() (removed bool) {
	for _, tab := range c.Tabs() {
		if !tab.free {
			// used by a window
			continue
		}
		c.removeTab(tab)
		removed = true
	}
	return
}

// Window returns the window with the given ID or false if there's
// no window with the given ID.
func (c *Component) Window(id uint64) (Window, bool) {
	ret, ok := c.findWindow(id)
	c.log(log.TraceLevel, "find window: %d, %v, %v", id, ret, ok)
	return ret, ok
}

// RemoveWindowContent removes the content at win. It returns false
// if content was replaced with start handler because the content at win
// was the last content in this Component.
func (c *Component) RemoveWindowContent(win Window) bool {
	bwin := win.(*browserWindow)
	oldComponent := bwin.win.Content().(browserapi.Handler)
	t, isNotStartHandler := c.getFreeTab(oldComponent)
	oldComponent = c.updateWindowContent(bwin, t, nil /* don't store prev here */)
	oldTab, ok := oldComponent.(*Tab)
	if ok {
		c.removeTab(oldTab)
	}
	return isNotStartHandler
}

// RemoveTab removes the given tab. If the given handler is not
// a tab, this method returns false. If the tab has already been removed, this
// method will panic.
func (c *Component) RemoveTab(h browserapi.Handler) bool {
	t, ok := h.(*Tab)
	if !ok {
		return false
	}
	if !t.free {
		_ = c.RemoveWindowContent(t.win)
		return true
	}
	c.removeTab(t)
	return true
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
func (c *Component) Split(
	o browserapi.Orientation, win Window, h browserapi.Handler,
) (Window, bool) {
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
	h = c.newBrowserContent(h).(Floating)
	win := c.newWindow(c.wm.FloatingWindow(h, cfg))
	c.wm.SetFocus(win.win)
	return win
}

// Bar adds a bar to the orientation of the main window.
func (c *Component) Bar(cfg browserapi.BarConfig, h tui.Handler) {
	if cfg.Size <= 0 {
		panic("invalid bar size")
	}
	frame := (cfg.Frame == browserapi.BarFrameDefault && c.config.Frame) ||
		cfg.Frame == browserapi.BarFrameAlways

	if frame {
		f := handler.NewFrame(h)
		f.FrameCharSet = c.config.FrameCharSet
		f.Attributes = c.config.FrameAttr
		h = f
		cfg.Size += 2
	}

	if cfg.Orientation == browserapi.OrientationDefault {
		cfg.Orientation = c.nextSplit
	}
	switch cfg.Orientation {
	case browserapi.OrientationTop:
		c.union.UnionTopFrame(h, cfg.Size, frame)
	case browserapi.OrientationBottom:
		c.union.UnionBottomFrame(h, cfg.Size, frame)
	case browserapi.OrientationLeft:
		c.union.UnionLeftFrame(h, cfg.Size, frame)
	case browserapi.OrientationRight:
		c.union.UnionRightFrame(h, cfg.Size, frame)
	}
}

func (c *Component) notify(level notifications.Level, msg string, args ...interface{}) {
	_, _ = c.config.Notifications.Notify(level, fmt.Sprintf(msg, args...))
}

// Resize satisfies tui.Component
func (c *Component) Resize(width, height int) {
	c.width, c.height = width, height
	c.union.Resize(width, height)
}

// Draw satisfies tui.Component
func (c *Component) Draw(w term.Writer) {
	if c.dirtyTabs {
		c.tabs.ResetFocus()
		for id, t := range c.buffers {
			if !t.free {
				c.tabs.SetFocus(id)
			}
		}
		c.dirtyTabs = false
	}

	c.union.Draw(w)

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
	vw := component.VirtualWriter{
		Writer: w,
		Offset: pos,
		Height: height,
		Width:  width,
	}
	c.wm.DrawWindow(target.(*browserWindow).win, &vw)
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

// SwapContentDown calls the underlying WindowManager.SwapContentDown.
func (c *Component) SwapContentDown() bool {
	return c.wm.SwapContentDown()
}

// SwapContentLeft calls the underlying WindowManager.SwapContentLeft.
func (c *Component) SwapContentLeft() bool {
	return c.wm.SwapContentLeft()
}

// SwapContentRight calls the underlying WindowManager.SwapContentRight.
func (c *Component) SwapContentRight() bool {
	return c.wm.SwapContentRight()
}

// SwapContentUp calls the underlying WindowManager.SwapContentUp.
func (c *Component) SwapContentUp() bool {
	return c.wm.SwapContentUp()
}

// ResetWindowSize resets width and height to be automatically calculated.
func (c *Component) ResetWindowSize() bool {
	w := c.wm.Focus()
	okh := c.wm.SetHeight(w, 0)
	okw := c.wm.SetWidth(w, 0)
	return okh || okw
}

// SetMaxWindowHeight maximizes the focus window's height.
func (c *Component) SetMaxWindowHeight() bool {
	w := c.wm.Focus()
	return c.wm.SetHeight(w, w.MaxHeight())
}

// SetMaxWindowWidth maximizes the focus window's width.
func (c *Component) SetMaxWindowWidth() bool {
	w := c.wm.Focus()
	return c.wm.SetWidth(w, w.MaxWidth())
}

// SetMinWindowHeight minimizes the focus window's height.
func (c *Component) SetMinWindowHeight() bool {
	w := c.wm.Focus()
	return c.wm.SetHeight(w, w.MinHeight())
}

// SetMinWindowWidth minimizes the focus window's width.
func (c *Component) SetMinWindowWidth() bool {
	w := c.wm.Focus()
	return c.wm.SetWidth(w, w.MinWidth())
}

// IncreaseWindowHeight increases the focus window's height.
func (c *Component) IncreaseWindowHeight() bool {
	w := c.wm.Focus()
	return c.wm.SetHeight(w, w.Height()+1)
}

// DecreaseWindowHeight decreases the focus window's height.
func (c *Component) DecreaseWindowHeight() bool {
	w := c.wm.Focus()
	return c.wm.SetHeight(w, w.Height()-1)
}

// IncreaseWindowWidth increases the focus window's width.
func (c *Component) IncreaseWindowWidth() bool {
	w := c.wm.Focus()
	return c.wm.SetWidth(w, w.Width()+1)
}

// DecreaseWindowWidth decreases the focus window's width.
func (c *Component) DecreaseWindowWidth() bool {
	w := c.wm.Focus()
	return c.wm.SetWidth(w, w.Width()-1)
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
	return c.union.Handle(ev)
}

// Cursor calls the underlying FrameUnion's Cursor.
func (c *Component) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return c.union.Cursor()
}

// Selection returns the underlying FrameUnion's selection.
func (c *Component) Selection() (string, bool) {
	return c.union.Selection()
}

// Prompt creates a new prompt to be drawn as an overlay on the next call to Draw
// and it also takes over event control until user either exits prompt or selects
// an option. The passed options and bindings must be equal in length, or bindings
// must be zero in length, meaning no key bindings are provided to user.
// Callers must ensure that there's coherence between options and bindings, otherwise
// this method panics.
//
// Prompt uses message to de-duplicate prompts. If a prompt is already open
// then this method returns the window used to display that prompt.
func (c *Component) Prompt(
	message string, options []string,
	bindings []term.KeyComb,
	promptHandler handler.PromptHandler,
) Window {
	if len(options) == 0 || (len(bindings) != 0 && len(options) != len(bindings)) {
		panic("Prompt given invalid options and/or bindings")
	}
	if win, ok := c.prompts[message]; ok {
		return win
	}
	promptHandler = clearOnClosePromptHandler{
		root:    promptHandler,
		c:       c,
		message: message,
	}
	promptConfig := handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message:              message,
			Options:              options,
			Frame:                c.config.FrameCharSet,
			BackgroundAttributes: c.config.PromptConfig.BackgroundAttr,
			MinWidth:             c.config.PromptConfig.MinWidth,
		},
		PromptHandler:  promptHandler,
		OptionBindings: bindings,
		OptionAttr:     c.config.PromptConfig.TextAttr,
		HighlightAttr:  c.config.PromptConfig.HighlightAttr,
	}
	if c.config.Frame {
		promptConfig.Frame = component.FrameCharSetDefault()
	}

	prompt := handler.NewPrompt(promptConfig)
	floatingConfig := component.FloatingConfig{
		Alignment: component.SpanAlignmentCentered,
	}

	win := c.Floating(prompt, floatingConfig)
	c.prompts[message] = win
	return win
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

// CloseOtherWindows closes all the windows except the given window.
func (c *Component) CloseOtherWindows(win Window) (retErr error) {
	if win.IsFloating() {
		return errors.New("cannot close all tiled windows")
	}
	var ok bool
	c.wm.Iterate(func(w handler.Window) {
		if w.ID() == win.WindowID() {
			return
		}
		if err := w.Close(); err != nil {
			retErr = multierror.Append(retErr, err)
			return
		}
		ok = true
	})
	if retErr != nil {
		return retErr
	}
	if !ok {
		retErr = errors.New("no windows to close")
	}
	return
}

// Close closes the resources associated with this browser.
func (c *Component) Close() (ret error) {
	for _, f := range c.buffers {
		err := f.Close()
		if err != nil {
			ret = err
		}
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

func (c *Component) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "browser.Component").Logf(level, msg, args...)
}

func (c *Component) setError(err error) {
	c.notify(notifications.LevelError, "%s", err)
}

func (c *Component) focus() *browserWindow {
	win, ok := c.findWindow(c.wm.Focus().ID())
	if !ok {
		panic("corrupted browser: cannot find focus window")
	}
	return win
}

func (c *Component) closeTab(t *Tab) {
	err := t.Close()
	if err != nil {
		c.log(log.WarnLevel, "tab Close error: %v", err)
	}
}

func (c *Component) removeTab(t *Tab) {
	if !t.free {
		panic("trying to remove tab that is still attached to a window")
	}
	c.closeTab(t)
	id := c.findTabID(t)
	c.doRemoveTab(id)
	ok := c.tabs.Remove(id)
	if !ok {
		panic(fmt.Sprintf("corrupted tabs: could not find tab with id %v", id))
	}
}

func (c *Component) doRemoveTab(idx int) {
	c.buffers = append(c.buffers[:idx], c.buffers[idx+1:]...)
}

func (c *Component) doInsertTab(idx int, t *Tab) {
	c.buffers = append(c.buffers, nil)
	copy(c.buffers[idx+1:], c.buffers[idx:])
	c.buffers[idx] = t
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

func (c *Component) updateWindowTab(
	win *browserWindow, tabID int, prev browserapi.Handler,
) bool {
	if tabID >= len(c.buffers) {
		panic(fmt.Sprintf("invalid tab at index: %d", tabID))
	}
	t := c.buffers[tabID]
	if t.free {
		c.updateWindowContent(win, t, prev)
		return true
	}
	return false
}

func (c *Component) updateWithNextFreeTab(win Window, prev browserapi.Handler) bool {
	freeBufs := c.freeTabs()
	if len(freeBufs) != 0 {
		c.updateWindowContent(win.(*browserWindow), c.buffers[freeBufs[0]], prev)
		return true
	}

	return false
}

func (c *Component) closeHandler(h browserapi.Handler) {
	err := h.Close()
	if err != nil {
		c.log(log.WarnLevel, "Close error: %v", err)
	}
	c.log(log.DebugLevel, "Component.closeHandler(%p)", h)
}

func (c *Component) tryUpdateWindowContent(
	win *browserWindow, content browserapi.Handler, prev browserapi.Handler,
) error {
	if b, ok := content.(*Tab); ok {
		if !b.free {
			return browserapi.ErrTabNotFree
		}
	}
	c.updateWindowContent(win, content, prev)
	return nil
}

func (c *Component) updateWindowContent(
	win *browserWindow, content browserapi.Handler, prev browserapi.Handler,
) browserapi.Handler {
	tab, ok := content.(*Tab)
	if ok {
		id := c.findTabID(tab)
		c.tabs.SetFocus(id)
		c.dirtyTabs = true
		prevt, _ := prev.(*Tab)
		tab.setWindow(prevt, win)
	} else {
		_, sok := content.(*browserScrollableContent)
		_, bok := content.(*browserContent)
		_, fok := content.(*browserFloatingContent)
		_, fsok := content.(*browserFloatingScrollableContent)
		if !sok && !bok && !fok && !fsok {
			content = c.newBrowserContent(content)
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
		c.dirtyTabs = true
		t.setFree()
	} else {
		c.closeHandler(h)
	}
}

func browserTabAtWindow(win *browserWindow) (*Tab, bool) {
	t, ok := win.win.Content().(*Tab)
	return t, ok
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
func (c *Component) getFreeTab(hint tui.Handler) (browserapi.Handler, bool) {
	t, ok := hint.(*Tab)
	if ok && t.prev != nil && t.prev.free && slices.Contains(c.buffers, t.prev) {
		return t.prev, true
	}
	freeBufs := c.freeTabs()
	if len(freeBufs) == 0 {
		return c.wallpaper(), false
	}

	id := freeBufs[0]
	return c.buffers[id], true
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
	c.windows[newBrowserWin.WindowID()] = newBrowserWin
	c.windows[focusBrowserWin.WindowID()] = focusBrowserWin

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
		h = c.newBrowserContent(h)
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
		c.dirtyTabs = true
		h.(*Tab).setWindow(nil, ret)
		h.(*Tab).callOnFocus()
	}
	return ret
}

func (c *Component) tabsSize() int {
	if c.config.TabBarHeight != 0 {
		return c.config.TabBarHeight
	}
	if c.config.Frame {
		return 3
	}
	return 1
}

func (c *Component) newWindow(win handler.Window) *browserWindow {
	browserWin := &browserWindow{
		parent: c,
		win:    win,
	}
	c.windows[browserWin.WindowID()] = browserWin
	c.log(log.TraceLevel, "new window: %d", win.ID())
	return browserWin
}

// closeWindow closes win or returns an error if win is the last Window.
func (c *Component) closeWindow(win *browserWindow) error {
	_, ok := c.findWindow(win.WindowID())
	if !ok {
		panic("window not found")
	}

	content := win.win.Content().(browserapi.Handler)
	err := win.win.Close()
	c.log(log.TraceLevel, "closing window: %d, %v", win.WindowID(), err)
	if err != nil {
		return err
	}

	delete(c.windows, win.WindowID())

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
		cell := term.Cell{Width: 1, Ch: cs.TopLeft, Attributes: attr}
		w.SetCell(mainPos, cell)
	}

	topright := term.Coordinates{X: topleft.X + c.focusWindow.Width(), Y: topleft.Y}
	if topright == (term.Coordinates{X: mainWidth, Y: 0}) {
		cell := term.Cell{Width: 1, Ch: cs.TopRight, Attributes: attr}
		pos := term.Coordinates{X: mainPos.X + mainWidth - 1, Y: mainPos.Y}
		w.SetCell(pos, cell)
	}

	bottomleft := term.Coordinates{X: topleft.X, Y: topleft.Y + c.focusWindow.Height()}
	if bottomleft == (term.Coordinates{Y: mainHeight, X: 0}) {
		cell := term.Cell{Width: 1, Ch: cs.BottomLeft, Attributes: attr}
		pos := term.Coordinates{X: mainPos.X, Y: mainPos.Y + mainHeight - 1}
		w.SetCell(pos, cell)
	}

	bottomright := term.Coordinates{
		X: topleft.X + c.focusWindow.Width(),
		Y: topleft.Y + c.focusWindow.Height(),
	}
	if bottomright == (term.Coordinates{Y: mainHeight, X: mainWidth}) {
		cell := term.Cell{Width: 1, Ch: cs.BottomRight, Attributes: attr}
		pos := term.Coordinates{
			X: mainPos.X + mainWidth - 1,
			Y: mainPos.Y + mainHeight - 1,
		}
		w.SetCell(pos, cell)
	}
}

func (c *Component) wallpaper() browserapi.Handler {
	wallpaper := c.config.Wallpaper
	if wallpaper.NewComponent == nil {
		wallpaper.NewComponent = component.Nop
	}
	instance := wallpaper.NewComponent()
	// if background attrs were passed try to re-construct string wallpaper
	// or set a background via component.Background.
	if wallpaper.BackgroundAttr != (term.Attributes{}) {
		if str, ok := instance.(component.String); ok {
			cfg := str.Config()
			cfg.BackgroundAttributes = wallpaper.BackgroundAttr
			cfg.Attributes.Bg = wallpaper.BackgroundAttr.Bg
			instance = component.NewStringWithConfig(str.String(), cfg)
		} else {
			// activate override behaviour
			nonZeroCh := ' '
			instance = component.NewBackground(instance,
				term.Cell{Ch: nonZeroCh, Attributes: wallpaper.BackgroundAttr})
		}
	}
	// make wallpaper satisfy Floating to avoid browserContent panic
	// if wallpaper is being set as a default on a floating window
	floating := component.StaticFloating(instance, 80, 40)
	return &browserContent{
		Handler: NopFloatingHandler(handler.NopFloatingHandler(floating)),
		c:       c,
	}
}

func (c *Component) newBrowserContent(content browserapi.Handler) browserapi.Handler {
	bc := browserContent{c: c, Handler: content}
	_, fok := content.(component.Floating)
	_, sok := content.(component.Scrollable)
	if sok && fok {
		return &browserFloatingScrollableContent{browserContent: bc}
	}
	if fok {
		return &browserFloatingContent{browserContent: bc}
	}
	if sok {
		return &browserScrollableContent{browserContent: bc}
	}
	return &bc
}

type clearOnClosePromptHandler struct {
	c       *Component
	message string
	root    handler.PromptHandler
}

func (c clearOnClosePromptHandler) OnSelect(idx int, option string) {
	c.root.OnSelect(idx, option)
	delete(c.c.prompts, c.message)
}

func (c clearOnClosePromptHandler) OnClose() error {
	err := c.root.OnClose()
	delete(c.c.prompts, c.message)
	return err
}

// component.WindowManager sinchronously removes tui.Handlers
// upon returning exit=true on calls to Handle. This
// structure is used to call Close when this occurs.
type browserContent struct {
	browserapi.Handler
	closed bool
	c      *Component
}

// allow for advanced use of content
func (c *browserContent) Content() browserapi.Handler {
	return c.Handler
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
				c.c.dirtyTabs = true
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

var _ Floating = (*browserFloatingContent)(nil)

type browserFloatingContent struct {
	browserContent
}

func (c *browserFloatingContent) Dimensions() (int, int) {
	return c.Handler.(Floating).Dimensions()
}

var _ Floating = (*browserFloatingScrollableContent)(nil)
var _ Scrollable = (*browserFloatingScrollableContent)(nil)

type browserFloatingScrollableContent struct {
	browserContent
}

func (c *browserFloatingScrollableContent) Dimensions() (int, int) {
	return c.Handler.(Floating).Dimensions()
}

func (c *browserFloatingScrollableContent) SeekUp() bool {
	return c.Handler.(component.Scrollable).SeekUp()
}

func (c *browserFloatingScrollableContent) SeekDown() bool {
	return c.Handler.(component.Scrollable).SeekDown()
}

func (c *browserFloatingScrollableContent) SeekOffset() int {
	return c.Handler.(component.Scrollable).SeekOffset()
}

func (c *browserFloatingScrollableContent) MaxSeekOffset() int {
	return c.Handler.(component.Scrollable).MaxSeekOffset()
}

var _ Scrollable = (*browserScrollableContent)(nil)

type browserScrollableContent struct {
	browserContent
}

func (c *browserScrollableContent) SeekUp() bool {
	return c.Handler.(component.Scrollable).SeekUp()
}

func (c *browserScrollableContent) SeekDown() bool {
	return c.Handler.(component.Scrollable).SeekDown()
}

func (c *browserScrollableContent) SeekOffset() int {
	return c.Handler.(component.Scrollable).SeekOffset()
}

func (c *browserScrollableContent) MaxSeekOffset() int {
	return c.Handler.(component.Scrollable).MaxSeekOffset()
}

// satisfies handler.WindowSubscriber to
// override union attrs of focus window
type wmSubscriber Component

func (s *wmSubscriber) OnFocus(prev, focus handler.Window) {
	c := (*Component)(s)
	c.focusWindow = focus
}
