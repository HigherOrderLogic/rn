// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// fakeHandler counts calls to each tui.Handler method so tests can
// assert delegation behavior.
type fakeHandler struct {
	resizeCalls    int
	lastResizeW    int
	lastResizeH    int
	drawCalls      int
	cursorCalls    int
	selectionCalls int
	handleCalls    int
	lastEv         term.Event

	cursorCoord term.Coordinates
	cursorStyle term.CursorStyle
	cursorShow  bool

	selectionText string
	selectionOK   bool

	handleExit    bool
	handleHandled bool
}

func (f *fakeHandler) Resize(w, h int) {
	f.resizeCalls++
	f.lastResizeW = w
	f.lastResizeH = h
}

func (f *fakeHandler) Draw(term.Writer) { f.drawCalls++ }

func (f *fakeHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	f.cursorCalls++
	return f.cursorCoord, f.cursorStyle, f.cursorShow
}

func (f *fakeHandler) Selection() (string, bool) {
	f.selectionCalls++
	return f.selectionText, f.selectionOK
}

func (f *fakeHandler) Handle(ev term.Event) (exit, handled bool) {
	f.handleCalls++
	f.lastEv = ev
	return f.handleExit, f.handleHandled
}

func TestBootstrapHandlerDelegates(t *testing.T) {
	inner := &fakeHandler{
		cursorCoord:   term.Coordinates{X: 3, Y: 4},
		cursorShow:    true,
		selectionText: "selected",
		selectionOK:   true,
		handleHandled: true,
	}
	b := &bootstrapHandler{inner: inner}

	b.Resize(80, 24)
	require.Equal(t, 1, inner.resizeCalls)
	require.Equal(t, 80, inner.lastResizeW)
	require.Equal(t, 24, inner.lastResizeH)

	b.Draw(nil)
	require.Equal(t, 1, inner.drawCalls)

	c, _, show := b.Cursor()
	require.Equal(t, 1, inner.cursorCalls)
	require.Equal(t, term.Coordinates{X: 3, Y: 4}, c)
	require.True(t, show)

	sel, ok := b.Selection()
	require.Equal(t, 1, inner.selectionCalls)
	require.Equal(t, "selected", sel)
	require.True(t, ok)

	ev := term.Event{Type: term.EventKey}
	exit, handled := b.Handle(ev)
	require.Equal(t, 1, inner.handleCalls)
	require.Equal(t, ev, inner.lastEv)
	require.False(t, exit)
	require.True(t, handled)
}

// TestBootstrapHandlerSwapInner verifies that swapping the inner
// handler causes Handle to forward events to the new inner. We cannot
// build a real *ide.IDE in a unit test so we exercise the post-swap
// state directly.
func TestBootstrapHandlerSwapInner(t *testing.T) {
	a := &fakeHandler{}
	b := &fakeHandler{}
	bh := &bootstrapHandler{inner: a, chosenEditor: editorModal}
	bh.inner = b

	ev := term.Event{Type: term.EventKey}
	_, _ = bh.Handle(ev)
	require.Equal(t, 0, a.handleCalls, "old inner should not receive events after swap")
	require.Equal(t, 1, b.handleCalls, "new inner should receive events after swap")

	_, _ = bh.Handle(ev)
	require.Equal(t, 2, b.handleCalls)
}

// TestBootstrapHandlerResizesAfterSwap reproduces a panic where the
// configured IDE's first Draw rendered against a zero-width buffer
// because gui.Update only invokes Resize on layout change, not on every
// tick. The handler must remember the last Resize dimensions and apply
// them to the new inner immediately after the swap.
func TestBootstrapHandlerResizesAfterSwap(t *testing.T) {
	pre := &fakeHandler{}
	post := &fakeHandler{}

	bh := &bootstrapHandler{inner: pre}
	bh.Resize(120, 40)
	require.Equal(t, 1, pre.resizeCalls)
	require.Equal(t, 120, pre.lastResizeW)
	require.Equal(t, 40, pre.lastResizeH)

	// Simulate the post-swap assignment + explicit Resize that
	// performSwap does for the configured IDE.
	bh.inner = post
	if bh.lastResizeW > 0 && bh.lastResizeH > 0 {
		bh.inner.Resize(bh.lastResizeW, bh.lastResizeH)
	}
	require.Equal(t, 1, post.resizeCalls, "new inner must be Resized after swap")
	require.Equal(t, 120, post.lastResizeW)
	require.Equal(t, 40, post.lastResizeH)
}
