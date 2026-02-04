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

package input

import (
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
	tcomponent "unstable.build/go-tui/component"
	"unstable.build/go-tui/text"
)

var _ component.Responsive = (*Box)(nil)
var _ tui.Handler = (*Box)(nil)

// BoxConfig holds configuration for initializing an Box.
type BoxConfig struct {
	// MaxHeight defines the maximum height returned by Height.
	// If not set, then there is no max height.
	MaxHeight int
	// MinHeight defines the minimum height returned by Height.
	// If not set, then there is no minimum height.
	MinHeight         int
	Placeholder       string
	PlaceholderConfig component.StringConfig
	DefaultFrameAttr  term.Attributes
	ContentConfig     term.Attributes
}

// Box is an input box that essentially collects user input.
type Box struct {
	buf         *cell.Buffer
	cfg         BoxConfig
	frame       *handler.Frame
	placeholder *handler.Frame
	// used to assist in calculate height
	placeholderStr component.Responsive
	editor         text.Editor
	handler        text.Handler
	// used just for Height calculation support
	scroll tcomponent.Scroll
}

// NewBox allocates storage for a new Box and
// initializes it.
func NewBox(buf *cell.Buffer, ed text.Editor, cfg BoxConfig) *Box {
	ret := new(Box)
	ret.Init(buf, ed, cfg)
	return ret
}

// Init initializes this Box with the given config and cell.Buffer,
// which is used to share the contents of the input box with clients.
func (i *Box) Init(buf *cell.Buffer, ed text.Editor, cfg BoxConfig) {
	if cfg.MaxHeight < cfg.MinHeight && cfg.MaxHeight != 0 {
		panic("max height cannot be smaller than min height")
	}

	uri := workspaceapi.RandomURI("inputbox")
	edh, err := ed.Edit(uri, buf, false, false)
	if err != nil {
		// this should not really happen, as editor implementations
		// passed to an input box should all be internal, and so
		// no errors can occur.
		panic(fmt.Errorf("editor edit: %v", err))
	}

	// wrap must be set to true for input box
	// to work as expected by users.
	edh.SetWrap(true)
	// space is usually limited, so hide the text editor's
	// command bar
	edh.ShowCommandBar(false)

	edh.SetDefaultAttributes(cfg.ContentConfig)

	frame := handler.NewFrame(edh)
	frame.Attributes = cfg.DefaultFrameAttr

	// do not expose StringResponsiveConfig in BoxConfig because editor handlers are
	// not capable of emulating NoSplitWords property.
	placeholderStr := component.NewResponsiveString(cfg.Placeholder,
		component.StringResponsiveConfig{StringConfig: cfg.PlaceholderConfig})
	placeholder := handler.NewFrame(
		handler.NopFromComponent(placeholderStr))
	placeholder.Attributes = cfg.DefaultFrameAttr

	i.buf = buf
	i.cfg = cfg
	i.frame = frame
	i.placeholderStr = placeholderStr
	i.placeholder = placeholder
	i.handler = edh
	i.editor = ed
	i.scroll.Init(i.buf)
	i.scroll.Wrap = true
}

// SetFrameAttr overrides the default frame attributes passed via BoxConfig.
func (i *Box) SetFrameAttr(attr term.Attributes) {
	i.frame.Attributes = attr
	i.placeholder.Attributes = attr
}

// Buffer returns this input.Box's underlying content buffer.
func (r *Box) Buffer() *cell.Buffer {
	return r.buf
}

// Reset resets this input box to its initial state.
func (r *Box) Reset() {
	_ = r.handler.SetCursorAtScroll(term.Coordinates{})
	r.buf.Reset()
}

// Height satisfies component.Responsive.
func (i *Box) Height(width int) (ret int) {
	if width < 2 {
		return 0
	}
	width -= 2 // frame
	if i.buf.Size() == 0 {
		ret = i.placeholderStr.Height(width)
	} else {
		// use scroll, rather than frame as Frame's content is the editor
		// which doesn't satisfy component.Responsive.
		ret = i.scroll.Height(width)
		rows := i.buf.Rows()
		// if last visible row is "full", always return +1
		// to allow for cursor to fall in an empty row but within bounds.
		if rows > 0 {
			lastRowCols := i.buf.View().Columns(rows - 1)
			if lastRowCols != 0 && lastRowCols%width == 0 {
				ret++
			}
		}
	}
	ret += 2 // always leave one more for the cursor upon newline
	if ret > i.cfg.MaxHeight && i.cfg.MaxHeight != 0 {
		ret = i.cfg.MaxHeight
	}
	if ret < i.cfg.MinHeight && i.cfg.MinHeight != 0 {
		ret = i.cfg.MinHeight
	}
	return ret
}

// Resize satisfies tui.Component.
func (i *Box) Resize(width, height int) {
	i.frame.Resize(width, height)
	i.placeholder.Resize(width, height)
}

// Draw satisfies tui.Component.
func (i *Box) Draw(w term.Writer) {
	if i.buf.Size() == 0 {
		i.placeholder.Draw(w)
	} else {
		i.frame.Draw(w)
	}
}

// Handle satisfies tui.Handler.
func (i *Box) Handle(ev term.Event) (exit, handled bool) {
	return i.frame.Handle(ev)
}

// Cursor satisfies tui.Handler.
func (i *Box) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return i.frame.Cursor()
}

// Selection satisfies tui.Handler.
func (i *Box) Selection() (string, bool) {
	return i.frame.Selection()
}
