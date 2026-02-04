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
	"unsafe"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
)

type floatingNode struct {
	desiredOffset               term.Coordinates // desired offset
	alignment                   component.Alignment        // desired alignment
	maxWidth, maxHeight         int              // window space size
	desiredWidth, desiredHeight int              // content desired Dimensions size
	userWidth, userHeight       int
	minimized                   component.Alignment
	minimizedPadding            int

	realWidth, realHeight int              // calculated upon Resize, considering trimming
	realOffset            term.Coordinates // calculated offset with alignment

	wm      *WindowManager
	content component.Virtual[component.Floating]
}

func newFloatingNode(
	wm *WindowManager, content component.Floating,
	cfg FloatingConfig,
	maxWidth, maxHeight int,
) *floatingNode {
	if cfg.Offset.Y < 0 || cfg.Offset.X < 0 {
		panic("invalid floating window coordinates")
	}
	ret := new(floatingNode)
	ret.desiredOffset = cfg.Offset
	ret.alignment = cfg.Alignment
	ret.maxWidth = maxWidth
	ret.maxHeight = maxHeight
	ret.wm = wm
	ret.SetContentResize(content, true)
	return ret
}

func (w *floatingNode) ID() uint64 {
	return uint64(uintptr(unsafe.Pointer(w)))
}

func (w *floatingNode) Width() int {
	if w.minimized == 0 {
		return w.realWidth
	}
	switch w.minimized {
	case component.AlignmentTop, component.AlignmentBottom:
		return w.wm.minimizedPos[w.ID()].length()
	case component.AlignmentLeft, component.AlignmentRight:
		return 1 + w.minimizedPadding
	default:
		panic("invalid minimize alignment")
	}
}

func (w *floatingNode) Height() int {
	if w.minimized == 0 {
		return w.realHeight
	}
	switch w.minimized {
	case component.AlignmentTop, component.AlignmentBottom:
		return 1 + w.minimizedPadding
	case component.AlignmentLeft, component.AlignmentRight:
		return w.wm.minimizedPos[w.ID()].length()
	default:
		panic("invalid minimize alignment")
	}
}

func (w *floatingNode) Content() tui.Component {
	if f, ok := w.content.C.(prevNodeFloating); ok {
		return f.Component
	}
	return w.content.C
}

func (w *floatingNode) Draw(wr term.Writer) {
	desiredWidth, desiredHeight := w.desiredWidth, w.desiredHeight
	w.updateDesiredDimensions()
	if desiredWidth != w.desiredWidth || desiredHeight != w.desiredHeight {
		w.resize()
	}
	// clear content
	for y := w.realOffset.Y; y < w.realOffset.Y+w.realHeight; y++ {
		for x := w.realOffset.X; x < w.realOffset.X+w.realWidth; x++ {
			wr.SetCell(term.Coordinates{Y: y, X: x}, term.Cell{})
		}
	}
	w.content.Draw(wr)
}

func (w *floatingNode) SetContentResize(c tui.Component, resize bool) (
	prev tui.Component,
) {
	var ok bool

	if w.wm.config.Frame {
		_, ok = c.(*component.Frame).Content().(component.Floating)
	} else {
		_, ok = c.(component.Floating)
	}

	if !ok {
		// Window.SetContent could pass a non Floating component
		// if that's the case, then set it to a static floating element which
		// uses the last Floating's desired dimensions
		c = prevNodeFloating{Component: c, width: w.desiredWidth, height: w.desiredHeight}
	}

	prev = w.content.C
	if f, ok := prev.(prevNodeFloating); ok {
		prev = f.Component // unwrap
	}
	w.content.C = c.(component.Floating)
	if resize {
		w.updateDesiredDimensions()
		w.resize()
	}
	return
}

func (w *floatingNode) Size() int {
	return 1
}

func (w *floatingNode) Close() {
	if w.wm == nil {
		return
	}

	wm := w.wm
	w.wm = nil
	wm.closeFloatingWindow(w)
}

func (w *floatingNode) Position() term.Coordinates {
	if w.minimized == 0 {
		return w.realOffset
	}
	return w.wm.minimizedPos[w.ID()].from()
}

func (w *floatingNode) compDimensions() (int, int) {
	return w.content.C.Dimensions()
}

func (w *floatingNode) setWidth(width int) bool {
	compWidth, _ := w.compDimensions()
	if w.minimized != 0 && width < compWidth && width != 0 { // width 0 resets
		return false
	}
	w.userWidth = width
	return true
}

func (w *floatingNode) setHeight(height int) bool {
	_, compHeight := w.compDimensions()
	if w.minimized != 0 && height < compHeight && height != 0 { // height 0 resets
		return false
	}
	w.userHeight = height
	return true
}

func (w *floatingNode) updateDesiredDimensions() {
	w.desiredWidth, w.desiredHeight = w.compDimensions()
	if w.wm.config.NoMaxSize {
		w.desiredHeight = min(w.maxHeight-2, max(w.userHeight, w.desiredHeight))
		w.desiredWidth = min(w.maxWidth-2, max(w.userWidth, w.desiredWidth))
	} else {
		w.desiredHeight = max(w.userHeight, w.desiredHeight)
		w.desiredWidth = max(w.userWidth, w.desiredWidth)
	}
}

func (w *floatingNode) resize() {
	verticalDiff := max(0, w.maxHeight-w.desiredHeight)
	horizontalDiff := max(0, w.maxWidth-w.desiredWidth)

	var offset term.Coordinates
	if w.alignment&component.AlignmentVerticallyCentered != 0 {
		offset.Y = verticalDiff / 2
	} else if w.alignment&component.AlignmentBottom != 0 {
		offset.Y = verticalDiff
		offset.Y -= w.desiredOffset.Y
	} else {
		offset.Y += w.desiredOffset.Y
	}

	if w.alignment&component.AlignmentHorizontallyCentered != 0 {
		offset.X = horizontalDiff / 2
	} else if w.alignment&component.AlignmentRight != 0 {
		offset.X = horizontalDiff
		offset.X -= w.desiredOffset.X
	} else {
		offset.X += w.desiredOffset.X
	}

	w.realOffset.X = max(0, offset.X)
	w.realOffset.Y = max(0, offset.Y)

	w.realWidth = w.desiredWidth
	w.realHeight = w.desiredHeight
	if w.realOffset.Y+w.realHeight >= w.maxHeight {
		w.realHeight = max(0, w.maxHeight-w.realOffset.Y)
	}
	if w.realOffset.X+w.realWidth >= w.maxWidth {
		w.realWidth = max(0, w.maxWidth-w.realOffset.X)
	}

	if w.realOffset.Y >= w.maxHeight || w.realOffset.X >= w.maxWidth {
		return
	}
	w.content.Resize(w.realWidth, w.realHeight)
	w.content.Move(w.realOffset)
}

func (w *floatingNode) Resize(width, height int) {
	panic("called resize on a floating node")
}

func (w *floatingNode) SetMaxSize(width, height int) {
	w.maxWidth = width
	w.maxHeight = height
	w.updateDesiredDimensions()
	w.resize()
}

func (t *floatingNode) Closed() bool {
	return t.wm == nil
}

// Floating used to indicate that it should be unwrapped in calls to Content
// or as a return of SetContentResize
type prevNodeFloating struct {
	tui.Component
	width, height int
}

func (p prevNodeFloating) Dimensions() (width, height int) {
	return p.width, p.height
}
