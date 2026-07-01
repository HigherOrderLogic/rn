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
	"net/url"
	"slices"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/component/markdown"
	thandler "unstable.build/go-tui/handler"
)

var _ browserapi.Handler = (*Component)(nil)

// offsetTabs is the tabs bar wrapped in a virtual writer offset so the
// bar visually starts at TabBarOffset. The underlying component.Tabs is
// resized to (width - offset) so the resize algorithm operates on the
// actually visible viewport — otherwise the focused tab would render
// past the right edge of the viewport.
type offsetTabs struct {
	handler.Virtual[*thandler.Tabs]
	offset int
}

func newOffsetTabs(tabs *thandler.Tabs, offset int) *offsetTabs {
	v := &offsetTabs{offset: offset}
	v.C = tabs
	v.Move(term.Coordinates{X: offset})
	return v
}

// Resize : tui.Component
func (v *offsetTabs) Resize(width, height int) {
	inner := width - v.offset
	if inner < 0 {
		inner = 0
	}
	v.Virtual.Resize(width, height)
	v.C.Resize(inner, height)
}

// Component renders a browser-like tui.Compontent and exposes an API
// to open new windows, add new tabs, and switch between tabs.
//
// All tui.Handlers installed other than via NewTab are considered ephemeral,
// and will be destroyed either when windows close or when they return exit=true
// to a call to Handle. Conversely, tui.Handlers installed via NewTab
// will remain as a tab and can be managed independently from windows.
type Component struct {
	tabs      thandler.Tabs
	wm        thandler.WindowManager
	union     thandler.FrameUnion
	width     int
	height    int
	nextSplit browserapi.Orientation

	dirtyTabs   bool
	focusWindow thandler.Window
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
		// If the tab is already bound to a window, switch focus to that
		// window rather than trying to move the tab into the focused
		// window. Clicking the tab of an already-focused window is a no-op.
		if win, ok := t.Window(); ok {
			bwin := win.(*browserWindow)
			if bwin != c.Focus().(*browserWindow) {
				c.SetFocus(bwin)
			}
			return true
		}
		bwin := c.Focus().(*browserWindow)
		err := c.tryUpdateWindowContent(bwin, t, bwin.win.Content().(browserapi.Handler))
		if err != nil {
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

	frameAttr := config.WindowManagerConfig.FrameAttr
	highlightAttr := config.FocusTabHighlightAttr
	frameAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	bgAttr := term.Attributes{Bg: frameAttr.Bg}
	focusTabAttr := config.FocusTabAttr
	focusTabAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	nonFocusTabAttr := config.NonFocusTabAttr
	nonFocusTabAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	c.tabs.SetAttr(focusTabAttr, nonFocusTabAttr,
		c.focusTabIconAttr(), c.nonFocusTabIconAttr(),
		highlightAttr, frameAttr, bgAttr)
	c.tabs.SetFrameCharSet(config.WindowManagerConfig.FrameCharSet)
	c.tabs.SetFocusFrameChar(config.FocusTabHighlightChar)

	// if tab bar offset is set, the remove frame from tabs
	// and install via union and no frame unioning.
	if config.TabBarOffset > 0 {
		vtabs := newOffsetTabs(&c.tabs, config.TabBarOffset)
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
	if c.config.TabOverrideIcon != 0 {
		icon = c.config.TabOverrideIcon
	}
	t := newTab(c, resource, h, f)
	c.buffers = append(c.buffers, t)
	c.tabs.Add(icon, name)
	return t
}

// NewTabFromContent converts the given window's content into a tab, if it's not a tab
// already. This method always returns a valid tab; whether it created one
// or returned false because the window's content is already a tab.
func (c *Component) NewTabFromContent(
	icon rune, name string, win Window,
) (*Tab, bool) {
	bwin := win.(*browserWindow)
	content := bwin.win.Content().(browserapi.Handler)
	tab, ok := content.(*Tab)
	if ok {
		return tab, false
	}
	content = c.unwrapContent(content)
	var uri workspaceapi.URI
	// best effort
	if urier, ok := content.(interface{ URI() workspaceapi.URI }); ok {
		uri = urier.URI()
	} else {
		var err error
		path := url.PathEscape(fmt.Sprintf("%p", content))
		uri, err = workspaceapi.ParseURI(fmt.Sprintf("internal:///%s", path))
		if err != nil {
			panic("parse internal uri")
		}
	}
	subscriber, ok := content.(TabSubscriber)
	tab = c.NewTab(uri, icon, name, content, nil)
	bwin.win.SetContent(tab)
	tab.setWindow(nil, bwin)
	if ok {
		tab.Subscribe(subscriber)
	}
	c.tabs.SetFocus(c.mustFindTabID(tab))
	c.dirtyTabs = true
	return tab, true
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

// SetTabAttrs overrides the name and attributes of the tab with the given uri.
// It returns false if there's no tab with id.
func (c *Component) SetTabAttrs(
	uri workspaceapi.URI, attr term.Attributes,
) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.SetTabAttr(i, attr)
			return true
		}
	}
	return false
}

// SetTabIcon overrides the icon of the tab with the given uri.
// It returns false if there's no tab with the given uri. The tab icon
// can be restored to its default via ResetTabIcon.
func (c *Component) SetTabIcon(uri workspaceapi.URI, icon rune) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.SetTabIcon(i, icon)
			return true
		}
	}
	return false
}

// ResetTabIcon resets the icon of the tab with the given uri to the
// last icon set via SetTabDefaultIcon or, if none, the icon the tab
// was created with (after any Config.TabOverrideIcon override).
// It returns false if there's no tab with the given uri.
func (c *Component) ResetTabIcon(uri workspaceapi.URI) bool {
	for i, t := range c.buffers {
		if t.uri.String() == uri.String() {
			c.tabs.ResetTabIcon(i)
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
	idx := c.mustFindTabID(t)
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
	idx := c.mustFindTabID(t)
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
	curridx := c.mustFindTabID(t)
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
	c.wm.Iterate(func(w thandler.Window) {
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
		removed = c.removeTab(tab) || removed
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

// IterateWindows applies fn to each open browser window.
func (c *Component) IterateWindows(fn func(Window)) {
	c.wm.Iterate(func(w thandler.Window) {
		fn(&browserWindow{parent: c, win: w})
	})
}

// TileLayout returns the current tiled browser window tree layout.
func (c *Component) TileLayout() tcomponent.TileLayout {
	return c.wm.TileLayout()
}

// RestoreTileLayout replaces the current tiled layout and returns a map from
// old layout window IDs to newly allocated browser windows.
func (c *Component) RestoreTileLayout(
	layout tcomponent.TileLayout,
	content func(windowID uint64) browserapi.Handler,
) map[uint64]Window {
	c.windows = make(map[uint64]*browserWindow)
	c.prompts = make(map[string]Window)
	windows := c.wm.RestoreTileLayout(layout, func(windowID uint64) tui.Handler {
		h := content(windowID)
		wrapped, _ := c.newWindowContent(h)
		return wrapped
	})
	ret := make(map[uint64]Window, len(windows))
	for id, win := range windows {
		bwin := c.newWindow(win)
		ret[id] = bwin
		if tab, ok := bwin.win.Content().(*Tab); ok {
			tab.setWindow(nil, bwin)
			tab.callOnFocus()
		}
	}
	// Make sure every live wm window is tracked in c.windows. This
	// covers leaf nodes that wm.RestoreTileLayout omitted from its
	// returned map (e.g. layouts with WindowID == 0), so that
	// c.focus() can always resolve wm.Focus() to a *browserWindow.
	c.wm.Iterate(func(w thandler.Window) {
		if _, ok := c.findWindow(w.ID()); ok {
			return
		}
		c.newWindow(w)
	})
	c.dirtyTabs = true
	return ret
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

// RemoveTab removes the given tab and reports whether it removed a tab.
// It returns false if the given handler is not a tab or if the tab has
// already been removed.
func (c *Component) RemoveTab(h browserapi.Handler) bool {
	t, ok := h.(*Tab)
	if !ok {
		return false
	}
	if !t.free {
		_ = c.RemoveWindowContent(t.win)
		return true
	}
	return c.removeTab(t)
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
	// A tab is bound to at most one window
	if existing, ok := boundTabWindow(h); ok {
		return c.SetFocus(existing), true
	}
	if o == browserapi.OrientationDefault {
		o = c.nextSplit
	}
	switch o {
	case browserapi.OrientationRight:
		return c.splitRegular((*thandler.WindowManager).SplitVertical, win, h)
	case browserapi.OrientationLeft:
		return c.splitInverted((*thandler.WindowManager).SplitVertical, win, h)
	case browserapi.OrientationTop:
		return c.splitInverted((*thandler.WindowManager).SplitHorizontal, win, h)
	case browserapi.OrientationBottom:
		return c.splitRegular((*thandler.WindowManager).SplitHorizontal, win, h)
	default:
		panic("not a valid orientation")
	}
}

// SplitRoot creates a new top-level split in the root tiled layout.
func (c *Component) SplitRoot(alignment component.Alignment, h browserapi.Handler) (Window, bool) {
	if existing, ok := boundTabWindow(h); ok {
		return c.SetFocus(existing), true
	}
	h, isTab := c.newWindowContent(h)
	win, ok := c.wm.SplitRoot(alignment, h)
	if !ok {
		return nil, false
	}
	ret := c.newWindow(win)
	if isTab {
		c.dirtyTabs = true
		h.(*Tab).setWindow(nil, ret)
		h.(*Tab).callOnFocus()
	}
	return ret, true
}

// Floating opens a new floating window at the given coordinates,
// with the given height and width.
func (c *Component) Floating(
	h Floating, cfg browserapi.FloatingConfig,
) Window {
	if h == nil {
		panic("nil Floating handler")
	}
	h = c.newBrowserContent(h).(Floating)
	win := c.newWindow(c.wm.FloatingWindow(h, tcomponent.FloatingConfig{
		Alignment: cfg.Alignment,
		Offset:    cfg.Offset,
	}))
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

func (c *Component) notify(level browserapi.NotificationLevel, msg string, args ...any) {
	_, _ = c.config.Notifications.Notify(level, msg, args...)
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
			c.tabs.SetIconAttr(id, c.nonFocusTabIconAttr())
			if !t.free {
				c.tabs.SetFocus(id)
			}
		}
		// SetFocus above also enables the highlight as a side effect;
		// reset it so the highlight is only drawn for the tab bound to
		// the focused window (mirroring the icon-attr logic below).
		c.tabs.ResetHighlight()

		if c.focusWindow != (thandler.Window{}) {
			win, ok := c.findWindow(c.focusWindow.ID())
			if ok {
				t, ok := browserTabAtWindow(win)
				if ok {
					// reset tab override attributes
					id := c.mustFindTabID(t)
					c.tabs.SetIconAttr(id, c.focusTabIconAttr())
					c.tabs.SetFocus(id)
				}
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

// SetWindowWidth sets the given window's width exactly.
func (c *Component) SetWindowWidth(win Window, width int) bool {
	bwin := win.(*browserWindow)
	if bwin.parent == nil {
		return false
	}
	return c.wm.SetWidth(bwin.win, width)
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
	clearHandler := &clearOnClosePromptHandler{
		root:    promptHandler,
		c:       c,
		message: message,
	}
	promptConfig := handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message:              message,
			Options:              options,
			BackgroundAttributes: c.config.PromptConfig.BackgroundAttr,
			MinWidth:             c.config.PromptConfig.MinWidth,
			NewMessage: func(str string) component.Floating {
				mcfg := markdown.DefaultConfig()
				mcfg.HeaderPrefix = false
				mkd, err := markdown.NewWithConfig(str, mcfg)
				if err == nil {
					return component.NewAspectRatioFloatingResponsive(
						component.NewSpan(mkd, component.SpanConfig{
							PadHorizontal:    4,
							PadVertical:      2,
							ContentAlignment: component.AlignmentCentered,
						}), component.DefaultAspectRatio)
				}
				cfg := component.StringResponsiveConfig{
					NoSplitWords: true,
					StringConfig: component.StringConfig{
						PaddingVertical:   4,
						PaddingHorizontal: 4,
						Alignment:         component.AlignmentCentered,
					},
				}
				messageResponsive := component.NewResponsiveString(str, cfg)

				return component.NewAspectRatioFloatingResponsive(
					messageResponsive, component.DefaultAspectRatio)
			},
		},
		PromptHandler:  clearHandler,
		OptionBindings: bindings,
		OptionAttr:     c.config.PromptConfig.TextAttr,
		HighlightAttr:  c.config.PromptConfig.HighlightAttr,
	}

	prompt := handler.NewPrompt(promptConfig)
	floatingConfig := browserapi.FloatingConfig{
		Alignment: component.AlignmentCentered,
	}

	win := c.Floating(prompt, floatingConfig)
	clearHandler.win = win
	c.prompts[message] = win
	return win
}

// Subscribe subscribes sub to window focus events.
func (c *Component) Subscribe(sub thandler.WindowSubscriber) {
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
	c.wm.Iterate(func(w thandler.Window) {
		if w.ID() == win.WindowID() {
			return
		}
		// Close through the browser window-close path so the tab binding
		// is released and c.windows is updated. Closing the raw handler
		// window would leave the tab marked bound to a removed window; a
		// later click on that stuck tab focuses a detached tile and
		// panics in TilePosition. Reuse the tracked browserWindow so the
		// double-close guard in browserWindow.Close stays effective: a fresh
		// browserWindow would overwrite c.windows and let a handler whose
		// Close callback closes its own captured window re-enter closeWindow
		// for an already-deleted window, panicking.
		bw, found := c.findWindow(w.ID())
		if !found {
			bw = c.newWindow(w)
		}
		if err := bw.Close(); err != nil {
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
	// Drop *Tab references so closed tabs can be GC'd immediately rather
	// than waiting for subsequent appends to overwrite the slots.
	clear(c.buffers)
	c.buffers = c.buffers[:0]
	c.wm.UnsubscribeAll()

	c.wm.Iterate(func(w thandler.Window) {
		// Reuse the tracked browserWindow so the double-close guard in
		// browserWindow.Close (which nils parent) stays effective. Creating a
		// fresh browserWindow here would overwrite c.windows and let a handler
		// whose Close callback closes its own captured window re-enter
		// closeWindow for an already-deleted window, panicking.
		bw, ok := c.findWindow(w.ID())
		if !ok {
			bw = c.newWindow(w)
		}
		_ = bw.Close()
	})
	return ret
}

func (c *Component) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "browser.Component").Logf(level, msg, args...)
}

func (c *Component) setError(err error) {
	c.notify(browserapi.LevelError, "%s", err)
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

func (c *Component) removeTab(t *Tab) bool {
	if !t.free {
		panic("trying to remove tab that is still attached to a window")
	}
	id, found := c.findTabID(t)
	if !found {
		return false
	}
	c.closeTab(t)
	c.doRemoveTab(id)
	ok := c.tabs.Remove(id)
	if !ok {
		panic(fmt.Sprintf("corrupted tabs: could not find tab with id %v", id))
	}
	return true
}

func (c *Component) doRemoveTab(idx int) {
	// Use slices.Delete so the now-unused tail slot is cleared. A bare
	// append(s[:idx], s[idx+1:]...) leaves the dropped *Tab pointer (which
	// transitively roots the file's cell.Buffer) in the backing array.
	c.buffers = slices.Delete(c.buffers, idx, idx+1)
}

func (c *Component) doInsertTab(idx int, t *Tab) {
	c.buffers = append(c.buffers, nil)
	copy(c.buffers[idx+1:], c.buffers[idx:])
	c.buffers[idx] = t
}

func (c *Component) findTabID(t *Tab) (int, bool) {
	for i, f := range c.buffers {
		if f == t {
			return i, true
		}
	}
	return -1, false
}

// mustFindTabID resolves a tab known to be present in the component. Use
// findTabID directly when the tab may have already been removed.
func (c *Component) mustFindTabID(t *Tab) int {
	id, ok := c.findTabID(t)
	if !ok {
		panic("could not find tab")
	}
	return id
}

func (c *Component) browserTabID(win *browserWindow) (
	*Tab, int,
) {
	t, ok := browserTabAtWindow(win)
	if !ok {
		return nil, 0
	}
	return t, c.mustFindTabID(t)
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
		id := c.mustFindTabID(tab)
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

// boundTabWindow reports the window currently showing h when h is a tab
// already bound to a window, so callers can refuse to bind one tab to a
// second window.
func boundTabWindow(h browserapi.Handler) (Window, bool) {
	t, ok := h.(*Tab)
	if !ok || t.free {
		return nil, false
	}
	return t.Window()
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
	split func(*thandler.WindowManager, thandler.Window, tui.Handler) (thandler.Window, bool),
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
	split func(*thandler.WindowManager, thandler.Window, tui.Handler) (thandler.Window, bool),
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
	split func(*thandler.WindowManager, thandler.Window, tui.Handler) (thandler.Window, bool),
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

func (c *Component) newWindow(win thandler.Window) *browserWindow {
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
	if !c.config.Frame || c.focusWindow == (thandler.Window{}) || c.wm.SizeTiles() == 1 {
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
		Handler: NopFloatingHandler(handler.NopFloatingFromComponent(floating)),
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

func (c *Component) focusTabIconAttr() term.Attributes {
	focusIconAttr := c.config.FocusTabIconAttr
	focusIconAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	return focusIconAttr
}

func (c *Component) nonFocusTabIconAttr() term.Attributes {
	nonFocusIconAttr := c.config.NonFocusTabIconAttr
	nonFocusIconAttr.Attrs |= term.AttrNegativeVerticalRenderOffset
	return nonFocusIconAttr
}

func (c *Component) unwrapContent(content browserapi.Handler) browserapi.Handler {
	bsc, ok := content.(*browserScrollableContent)
	if ok {
		return bsc.Handler
	}
	bc, ok := content.(*browserContent)
	if ok {
		return bc.Handler
	}
	bfc, ok := content.(*browserFloatingContent)
	if ok {
		return bfc.Handler
	}
	bfsc, ok := content.(*browserFloatingScrollableContent)
	if ok {
		return bfsc.Handler
	}
	panic("extraneous content")
}

// clearOnClosePromptHandler drops the prompt-dedup entry before
// running the root callbacks: a callback may reopen the same prompt,
// and a stale entry would dedupe that reopen against the window
// being closed. The entry is only dropped while it still points at
// this handler's own window, so a dying window's OnClose cannot
// evict an entry that an earlier OnSelect reopen just registered.
type clearOnClosePromptHandler struct {
	c       *Component
	message string
	root    handler.PromptHandler
	win     Window
}

func (c *clearOnClosePromptHandler) clear() {
	if cur, ok := c.c.prompts[c.message]; ok && cur == c.win {
		delete(c.c.prompts, c.message)
	}
}

func (c *clearOnClosePromptHandler) OnSelect(idx int, option string) {
	c.clear()
	c.root.OnSelect(idx, option)
}

func (c *clearOnClosePromptHandler) OnClose() error {
	c.clear()
	return c.root.OnClose()
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

func (s *wmSubscriber) OnFocus(prev, focus thandler.Window) {
	c := (*Component)(s)
	c.focusWindow = focus
	c.dirtyTabs = true
}
