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

package component

import (
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

var (
	defaultFocusAttr    = term.Attributes{Fg: tcell.ColorRed}
	defaultNonFocusAttr = term.Attributes{Fg: tcell.ColorDefault}
	defaultScrollAttr   = term.Attributes{Fg: tcell.ColorWhite}
	defaultFrameAttr    = term.Attributes{Fg: tcell.ColorRed}
	defaultSeparator    = "  "
)

type tab struct {
	name    string
	defName string
	icon    rune
	focus   bool
	defAttr term.Attributes
	attr    term.Attributes
}

// Tabs is a simple component that draws a list of component
// names which can in in Focus (highlighted) or not.
type Tabs struct {
	fileListBuf   *cell.Buffer
	fileListFrame tui.Component
	tabs          []*tab
	width, height int
	offsetIdx     int

	border         bool
	focusAttr      term.Attributes
	nonFocusAttr   term.Attributes
	backgroundAttr term.Attributes
	frameAttr      term.Attributes
	frameBorders   FrameCharSet
	separator      string
	dirty          bool
}

func newListFrame(
	scrollAttr, frameAttr term.Attributes, buf *cell.Buffer, border bool,
	frameBorders FrameCharSet,
) (content tui.Component) {
	scroll := new(Scroll)
	scroll.InitPerformance(buf)
	scroll.Attributes = scrollAttr
	background := term.Cell{Attributes: scroll.Attributes}
	spanCfg := SpanConfig{
		ContentAlignment: SpanAlignmentCentered,
		PadVertical:      -1,
	}
	span := WithBackground(NewSpan(scroll, spanCfg), background)
	if !border {
		return span
	}

	f := NewFrame(span)
	f.FrameCharSet = frameBorders
	f.Attributes = frameAttr
	return f
}

// NewTabs allocates storage for a new instance of Tabs and initializes it.
func NewTabs() *Tabs {
	t := new(Tabs)
	t.Init()
	return t
}

// Init initializes this Tab and effectively resets all content.
func (t *Tabs) Init() {
	t.border = true
	t.focusAttr = defaultFocusAttr
	t.nonFocusAttr = defaultNonFocusAttr
	t.frameBorders = FrameCharSetDefault()
	t.fileListBuf = new(cell.Buffer)
	t.fileListBuf.InitPerformance(1, 10, ' ')
	t.fileListFrame = newListFrame(
		defaultScrollAttr, defaultFrameAttr, t.fileListBuf,
		t.border, t.frameBorders)
	t.separator = defaultSeparator
	t.dirty = true
}

// SetAttr sets the attributes of the text in focus, text not in focus, the tabs
// frame and the tabs background.
func (t *Tabs) SetAttr(focusTab, tab, frame, background term.Attributes) {
	t.focusAttr = focusTab
	t.nonFocusAttr = tab
	t.backgroundAttr = background
	t.frameAttr = frame
	t.fileListFrame = newListFrame(t.backgroundAttr,
		t.frameAttr, t.fileListBuf, t.border, t.frameBorders)
	t.fileListFrame.Resize(t.width, t.height)
	t.dirty = true
}

// SetBorder defines whether this Tabs draws a border around or not.
// The default is true.
func (t *Tabs) SetBorder(border bool) {
	t.border = border
	t.fileListFrame = newListFrame(
		t.backgroundAttr, t.frameAttr, t.fileListBuf, t.border, t.frameBorders)
	t.fileListFrame.Resize(t.width, t.height)
	t.dirty = true
}

// SetNameSeparator defines the separator used to separate the different tab names.
// By default two spaces are used.
func (t *Tabs) SetNameSeparator(separator string) {
	t.separator = separator
	t.fileListFrame = newListFrame(
		t.backgroundAttr, t.frameAttr, t.fileListBuf, t.border, t.frameBorders)
	t.fileListFrame.Resize(t.width, t.height)
	t.dirty = true
}

// SetFrameCharSet defines the characters used to draw a frame border.
// Note that this has no effect if border is set to false on this Tabs.
func (t *Tabs) SetFrameCharSet(fb FrameCharSet) {
	t.frameBorders = fb
	t.fileListFrame = newListFrame(
		t.backgroundAttr, t.frameAttr, t.fileListBuf, t.border, t.frameBorders)
	t.fileListFrame.Resize(t.width, t.height)
	t.dirty = true
}

// Resize : tui.Component
func (t *Tabs) Resize(width, height int) {
	t.width, t.height = width, height
	t.dirty = true
	t.fileListFrame.Resize(width, height)
}

// Draw : tui.Component
func (t *Tabs) Draw(w term.Writer) {
	if t.dirty {
		t.prepareFileList()
		t.dirty = false
	}

	t.fileListFrame.Draw(w)
}

// ResetFocus resets the focus of all the tabs to false.
func (t *Tabs) ResetFocus() {
	for _, tab := range t.tabs {
		tab.focus = false
	}
	t.dirty = true
}

// SetFocus sets the focus to the tab at idx. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) SetFocus(idx int) {
	t.tabs[idx].focus = true
	t.dirty = true
}

// SetTabAttr sets the attributes of the tab at idx. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) SetTabAttr(idx int, attr term.Attributes) {
	t.tabs[idx].attr = attr
	t.dirty = true
}

// TabAttr returns the attributes of the tab at idx. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) TabAttr(idx int) term.Attributes {
	return t.tabs[idx].attr
}

// TabDefaultAttr returns the default attributes of the tab at idx.
// If the tab at idx does not exist, this method will panic.
func (t *Tabs) TabDefaultAttr(idx int) term.Attributes {
	return t.tabs[idx].defAttr
}

// TabIcon returns the icon of the tab at idx.
// If the tab at idx does not exist, this method will panic.
func (t *Tabs) TabIcon(idx int) rune {
	return t.tabs[idx].icon
}

// SetTabDefaultAttr sets the default attributes of the tab at idx. Calls to ResetTabAttr
// will reset the tab attributes to the given attributes.
// If the tab at idx does not exist, this method will panic.
func (t *Tabs) SetTabDefaultAttr(idx int, attr term.Attributes) {
	t.tabs[idx].defAttr = attr
	t.dirty = true
}

// ResetTabAttr resets the attributes of the tab at idx. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) ResetTabAttr(idx int) {
	t.tabs[idx].attr = t.tabs[idx].defAttr
	t.dirty = true
}

// SetTabName sets the name of the tab at idx. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) SetTabName(idx int, name string) {
	t.tabs[idx].name = name
	t.dirty = true
}

// SetTabIcon sets the icon of the tab at idx. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) SetTabIcon(idx int, icon rune) {
	t.tabs[idx].icon = icon
	t.dirty = true
}

// SetTabDefaultName sets the default name of the tab at idx. Calls to ResetTabName
// will reset the tab name to the given name. If the tab at idx does not exist,
// this method will panic.
func (t *Tabs) SetTabDefaultName(idx int, name string) {
	t.tabs[idx].defName = name
	t.dirty = true
}

// ResetTabName resets the name of the tab at idx to either the initial name
// given to this tab or the last name set via SetDefaultTabName.
// If the tab at idx does not exist, this method will panic.
func (t *Tabs) ResetTabName(idx int) {
	t.tabs[idx].name = t.tabs[idx].defName
	t.dirty = true
}

// TabName returns the name of the tab at idx.
// If the tab at idx does not exist, this method will panic.
func (t *Tabs) TabName(idx int) string {
	return t.tabs[idx].name
}

// DefaultTabName returns the default name of the tab at idx.
// If the tab at idx does not exist, this method will panic.
func (t *Tabs) DefaultTabName(idx int) string {
	return t.tabs[idx].defName
}

// Add adds a tab with the given name and icon.
func (t *Tabs) Add(icon rune, name string) int {
	t.dirty = true
	tt := &tab{
		defAttr: term.Attributes{},
		attr:    term.Attributes{},
		defName: name,
		icon:    icon,
		name:    name,
	}
	if t.tabs == nil {
		t.tabs = make([]*tab, 1)
		tt.focus = true
		t.tabs[0] = tt
		return 0
	}
	idx := len(t.tabs)
	t.tabs = append(t.tabs, tt)
	return idx
}

// Remove removes the tab at idx.
func (t *Tabs) Remove(idx int) bool {
	t.doRemoveTab(idx)
	t.dirty = true
	return true
}

// MoveRight moves the tab at idx to the right.
func (t *Tabs) MoveRight(idx int) bool {
	if idx >= len(t.tabs)-1 {
		return false
	}
	tt := t.doRemoveTab(idx)
	idx++
	t.doInsertTab(idx, tt)
	t.dirty = true
	return true
}

// MoveLeft moves the tab at idx to the left.
func (t *Tabs) MoveLeft(idx int) bool {
	if idx <= 0 {
		return false
	}
	tt := t.doRemoveTab(idx)
	idx--
	t.doInsertTab(idx, tt)
	t.dirty = true
	return true
}

// MoveTo moves the tab at curridx to the given idx.
func (t *Tabs) MoveTo(curridx, idx int) bool {
	if curridx < 0 || idx < 0 || curridx >= len(t.tabs) || idx >= len(t.tabs) {
		return false
	}
	tt := t.doRemoveTab(curridx)
	t.doInsertTab(idx, tt)
	t.dirty = true
	return true
}

// RemoveAll removes all tabs.
func (t *Tabs) RemoveAll() bool {
	ret := t.Size() != 0
	t.tabs = t.tabs[:0]
	t.dirty = true
	return ret
}

// TabAt returns the idx of the tab at pos, or panics if pos is
// out of bounds.
func (t *Tabs) TabAt(pos term.Coordinates) (int, bool) {
	x := 0
	idx := -1

	for i, z := range t.tabs[t.offsetIdx:] {
		if z.icon != 0 {
			x += 2
		}
		x += len(t.separator)
		x += len(z.name)
		if x >= pos.X {
			idx = i
			break
		}
	}

	return t.offsetIdx + idx, idx != -1
}

// Tab returns the name of the tab at idx.
func (t *Tabs) Tab(idx int) (string, bool) {
	if idx >= len(t.tabs) {
		return "", false
	}
	return t.tabs[idx].name, true
}

// Size returns the number of tabs.
func (t *Tabs) Size() int {
	return len(t.tabs)
}

func (t *Tabs) prepareFileList() {
	t.fileListBuf.Reset()

	if t.width == 0 || t.height == 0 {
		return
	}

	var focusLen int
	var focusPos, next term.Coordinates
	for i, tab := range t.tabs {
		attr := tab.attr
		if tab.focus {
			focusPos = next
			focusLen = len(tab.name)
			attr = term.AttributesUnion(t.focusAttr, attr)
		} else {
			attr = term.AttributesUnion(t.nonFocusAttr, attr)
		}

		if tab.icon != 0 {
			next = t.fileListBuf.Insert(next, tab.icon)
			next = t.fileListBuf.Insert(next, ' ')
		}
		_, next = t.fileListBuf.InsertStringWithAttr(
			next, tab.name, attr)

		if i < len(t.tabs)-1 {
			_, next = t.fileListBuf.InsertString(next, t.separator)
		}
	}

	t.offsetIdx = 0
	lenSeparator := len(t.separator)
	var effectiveWidth int
	if frame, ok := t.fileListFrame.(*Frame); ok {
		effectiveWidth, _ = frame.ContentSize()
	} else {
		effectiveWidth = t.width
	}
	for i := 0; i < len(t.tabs) && focusLen+focusPos.X > effectiveWidth; i++ {
		z := t.tabs[i]
		lenTab := len(z.name)
		if z.icon != 0 {
			lenTab += 2
		}
		if i < len(t.tabs)-1 {
			lenTab += lenSeparator
		}
		_, str := t.fileListBuf.Delete(term.Coordinates{}, term.Coordinates{X: lenTab})
		focusPos.X -= len(str)
		next.X -= len(str)
		t.offsetIdx++
	}

	if t.offsetIdx > 0 {
		separator := ".." + t.separator
		t.fileListBuf.InsertStringWithAttr(term.Coordinates{},
			separator, t.nonFocusAttr)
	}

	if t.fileListBuf.Columns(0) > effectiveWidth && effectiveWidth > 2 {
		from := term.Coordinates{X: effectiveWidth - 2}
		t.fileListBuf.TruncateRowFrom(from)
		t.fileListBuf.InsertStringWithAttr(from, "..", t.nonFocusAttr)
	}
}

func (t *Tabs) doRemoveTab(idx int) *tab {
	ret := t.tabs[idx]
	t.tabs = append(t.tabs[:idx], t.tabs[idx+1:]...)
	return ret
}

func (t *Tabs) doInsertTab(idx int, tt *tab) {
	t.tabs = append(t.tabs, nil)
	copy(t.tabs[idx+1:], t.tabs[idx:])
	t.tabs[idx] = tt
}
