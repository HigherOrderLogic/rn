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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
)

func TestSetWidthHeight(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	h1 := component.TestComponent{Ch: 'A'}
	wm, win1 := NewWindowManager(&h1, testWindowManagerConfig())
	wm.Resize(20, 8)

	var win2, win3 Window
	tests := []comptest.TestCase{
		{
			Action: func() {
				var ok bool
				win2, ok = wm.SplitVertical(win1, &component.TestComponent{Ch: 'B'})
				require.True(t, ok)
				win3, ok = wm.SplitHorizontal(win1, &component.TestComponent{Ch: 'C'})
				require.True(t, ok)
			}, Expected: `
┌────────┐┌────────┐
│AAAAAAAA││BBBBBBBB│
│AAAAAAAA││BBBBBBBB│
└────────┘│BBBBBBBB│
┌────────┐│BBBBBBBB│
│CCCCCCCC││BBBBBBBB│
│CCCCCCCC││BBBBBBBB│
└────────┘└────────┘`,
		},
		{
			Action: func() {
				assert.True(t, wm.SetHeight(win1, win1.MaxHeight()))
				assert.True(t, wm.SetWidth(win1, win1.MaxWidth()))
				assert.True(t, wm.SetHeight(win2, win2.MaxHeight()))
				assert.True(t, wm.SetWidth(win2, win2.MaxWidth()))
				assert.True(t, wm.SetHeight(win3, win3.MaxHeight()))
				assert.True(t, wm.SetWidth(win3, win3.MaxWidth()))
			}, Expected: `
┌───────────────┐┌─┐
│AAAAAAAAAAAAAAA││B│
└───────────────┘│B│
┌───────────────┐│B│
│CCCCCCCCCCCCCCC││B│
│CCCCCCCCCCCCCCC││B│
│CCCCCCCCCCCCCCC││B│
└───────────────┘└─┘`,
		},
	}

	comptest.TestComponent(t, wm, w, tests)
}

func TestWindowZeroValue(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		var win Window
		assert.NotPanics(t, func() {
			win.Close()
		})
	})
	t.Run("Content", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			_ = win.Content()
		})
	})
	t.Run("SetContent", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			win.SetContent(component.NewString("ballz"))
		})
	})
	t.Run("Size", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			win.Size()
		})
	})
	t.Run("TileDirection", func(t *testing.T) {
		var win Window
		assert.PanicsWithValue(t, errCalledZeroValuedWin, func() {
			win.TileDown()
		})
	})
}

func TestWindowManagerSplit(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	h1 := component.TestComponent{Ch: 'A'}
	wm, w1 := NewWindowManager(&h1, testWindowManagerConfig())
	wm.Resize(20, 8)

	var w2, w3 Window
	var prevFloating tui.Component
	var ok bool
	h2 := component.TestComponent{Ch: 'B'}
	hnop := component.TestComponent{Ch: 0}
	h3 := component.TestComponent{Ch: 'C'}

	tests := []comptest.TestCase{
		{
			nil, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`,
		}, {func() {
			w2, ok = wm.SplitHorizontal(w1, &h2)
			assert.True(t, ok)
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
┌──────────────────┐
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`,
		}, {func() {
			w3, ok = wm.SplitVertical(w2, &h3)
			assert.True(t, ok)
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
┌────────┐┌────────┐
│BBBBBBBB││CCCCCCCC│
│BBBBBBBB││CCCCCCCC│
└────────┘└────────┘`,
		}, {func() {
			assert.False(t, w2.Closed())
			assert.NoError(t, w2.Close())
			assert.True(t, w2.Closed())
		}, `
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			assert.False(t, w1.Closed())
			assert.NoError(t, w1.Close())
			assert.True(t, w1.Closed())
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			assert.Error(t, w1.Close())
			assert.True(t, w1.Closed())
			floating := component.StaticFloating(&h2, 2, 2)
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentLeft | component.AlignmentTop,
					Offset:    term.Coordinates{X: 1, Y: 1},
				},
			)
		}, `
┌──────────────────┐
│┌──┐CCCCCCCCCCCCCC│
││BB│CCCCCCCCCCCCCC│
││BB│CCCCCCCCCCCCCC│
│└──┘CCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentRight | component.AlignmentTop,
					Offset:    term.Coordinates{X: 1, Y: 1},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCC┌──┐│
│CCCCCCCCCCCCCC│BB││
│CCCCCCCCCCCCCC│BB││
│CCCCCCCCCCCCCC└──┘│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			assert.False(t, w2.Closed())
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentRight | component.AlignmentBottom,
					Offset:    term.Coordinates{X: 1, Y: 1},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCC┌──┐│
│CCCCCCCCCCCCCC│BB││
│CCCCCCCCCCCCCC│BB││
│CCCCCCCCCCCCCC└──┘│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentLeft | component.AlignmentBottom,
					Offset:    term.Coordinates{X: 1, Y: 1},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│┌──┐CCCCCCCCCCCCCC│
││BB│CCCCCCCCCCCCCC│
││BB│CCCCCCCCCCCCCC│
│└──┘CCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentHorizontallyCentered,
					Offset:    term.Coordinates{X: 1, Y: 1}, // offset.X is ignored
				},
			)
		}, `
┌──────────────────┐
│CCCCCCC┌──┐CCCCCCC│
│CCCCCCC│BB│CCCCCCC│
│CCCCCCC│BB│CCCCCCC│
│CCCCCCC└──┘CCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentVerticallyCentered,
					Offset:    term.Coordinates{X: 1, Y: 1}, // offset.Y is ignored
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│┌──┐CCCCCCCCCCCCCC│
││BB│CCCCCCCCCCCCCC│
││BB│CCCCCCCCCCCCCC│
│└──┘CCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&hnop, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentCentered,
					Offset:    term.Coordinates{X: 1, Y: 1}, // offset is ignored
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCC┌──┐CCCCCCC│
│CCCCCCC│  │CCCCCCC│
│CCCCCCC│  │CCCCCCC│
│CCCCCCC└──┘CCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentBottom,
					Offset:    term.Coordinates{X: 400, Y: 500},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentTop,
					Offset:    term.Coordinates{X: 400, Y: 500},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentLeft,
					Offset:    term.Coordinates{X: 400, Y: 500},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{
					Alignment: component.AlignmentRight,
					Offset:    term.Coordinates{X: 400, Y: 500},
				},
			)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			floating := component.StaticFloating(&h2, 2, 2)
			w2.Close()
			assert.True(t, w2.Closed())
			w2 = wm.FloatingWindow(floating,
				FloatingConfig{},
			)
		}, `
┌──┐───────────────┐
│BB│CCCCCCCCCCCCCCC│
│BB│CCCCCCCCCCCCCCC│
└──┘CCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			// test setting non floating component
			prevFloating = w2.SetContent(&component.TestComponent{Ch: '5'})
		}, `
┌──┐───────────────┐
│55│CCCCCCCCCCCCCCC│
│55│CCCCCCCCCCCCCCC│
└──┘CCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			// test return of SetContent is always what we expect
			prevFloating = w2.SetContent(prevFloating)
			assert.Equal(t, '5', prevFloating.(*component.TestComponent).Ch)
		}, `
┌──┐───────────────┐
│BB│CCCCCCCCCCCCCCC│
│BB│CCCCCCCCCCCCCCC│
└──┘CCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			wx, ok := wm.WindowAt(term.Coordinates{})
			assert.True(t, ok)
			assert.Equal(t, w2, wx)

			wx, ok = wm.WindowAt(term.Coordinates{X: 6})
			assert.True(t, ok)
			assert.Equal(t, w3, wx)

			wx, ok = wm.WindowAt(term.Coordinates{Y: 7})
			assert.True(t, ok)
			assert.Equal(t, w3, wx)

			assert.NoError(t, w2.Close())
			wx, ok = wm.WindowAt(term.Coordinates{})
			assert.True(t, ok)
			assert.Equal(t, w3, wx)
		}, `
┌──────────────────┐
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
│CCCCCCCCCCCCCCCCCC│
└──────────────────┘`,
		}, {func() {
			wx, ok := wm.WindowAt(term.Coordinates{})
			require.True(t, ok)
			comp := component.TestComponent{Ch: '#'}
			comp.Resize(3, 3)
			wx.SetContentResize(&comp, true)
		}, `
┌──────────────────┐
│##################│
│##################│
│##################│
│##################│
│##################│
│##################│
└──────────────────┘`,
		}, {func() {
			wx, ok := wm.WindowAt(term.Coordinates{})
			require.True(t, ok)
			comp := component.TestComponent{Ch: '$'}
			comp.Resize(3, 3)
			wx.SetContentResize(&comp, false)
		}, `
┌──────────────────┐
│$$$               │
│$$$               │
│$$$               │
│                  │
│                  │
│                  │
└──────────────────┘`,
		},
	}

	comptest.TestComponent(t, wm, w, tests)
}

func TestWindowManagerMinimize(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	h1 := component.TestComponent{Ch: 'A'}
	wm, _ := NewWindowManager(&h1, testWindowManagerConfig())
	wm.Resize(20, 8)

	var fwin Window
	tests := []comptest.TestCase{
		{
			Action: func() {
				floating := component.StaticFloating(&component.TestComponent{Ch: 'u'}, 2, 2)
				fwin = wm.FloatingWindow(floating,
					FloatingConfig{
						Alignment: component.AlignmentCentered,
					},
				)
				assert.True(t, fwin.MinimizeUp(0))
			}, Expected: `
┌──────────────────┐
┌──────────────────┐
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`,
		}, {Action: func() {
			w := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'l'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeLeft(0))
			assert.False(t, w.MinimizeLeft(0))

			w = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'r'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeRight(0))
			assert.False(t, w.MinimizeRight(0))

			w = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'd'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeDown(0))
			assert.False(t, w.MinimizeDown(0))
			assert.False(t, w.MinimizeLeft(0))
			assert.False(t, w.MinimizeRight(0))
			assert.False(t, w.MinimizeUp(0))
		}, Expected: `

┌──────────────────┐
┌┌────────────────┐┐
││AAAAAAAAAAAAAAAA││
││AAAAAAAAAAAAAAAA││
││AAAAAAAAAAAAAAAA││
││AAAAAAAAAAAAAAAA││
└└────────────────┘┘
└──────────────────┘`,
		}, {Action: func() {
			assert.True(t, fwin.Unminimize())
			assert.False(t, fwin.Unminimize())
		}, Expected: `
┌┌────────────────┐┐
││AAAAAA┌──┐AAAAAA││
││AAAAAA│uu│AAAAAA││
││AAAAAA│uu│AAAAAA││
││AAAAAA└──┘AAAAAA││
││AAAAAAAAAAAAAAAA││
└└────────────────┘┘
└──────────────────┘`,
		}, {Action: func() {
			assert.True(t, fwin.MinimizeUp(0))
			w := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'U'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeUp(1))
			w = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'L'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeLeft(1))

			w = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'R'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeRight(1))

			w = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'D'}, 2, 2),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			assert.True(t, w.MinimizeDown(1))
		}, Expected: `
┌──────────────────┐
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─AAAAAAAAAAAAAA─┐┐
└└─AAAAAAAAAAAAAA─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		}, {Action: func() {
			require.NoError(t, fwin.Close())
		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌────────────┐─┐┐
││L│AAAAAAAAAAAA│R││
└└─└────────────┘─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		}, {Action: func() {
			win, ok := wm.WindowAt(term.Coordinates{X: 3, Y: 2})
			require.True(t, ok)
			assert.Equal(t, 'A', win.Content().(*component.TestComponent).Ch)

			wof, ok := win.TileLeft()
			require.True(t, ok)
			assertEqualTile(t, wof, 'L')

			wof, ok = win.TileRight()
			require.True(t, ok)
			assertEqualTile(t, wof, 'R')

			wof, ok = win.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'U')

			wof, ok = win.TileDown()
			require.True(t, ok)
			assertEqualTile(t, wof, 'D')

			win, ok = wm.WindowAt(term.Coordinates{X: 0, Y: 0})
			require.True(t, ok)
			assertEqualTile(t, win, 'U')

			wof, ok = win.TileLeft()
			assert.False(t, ok)

			wof, ok = win.TileRight()
			assert.False(t, ok)

			wof, ok = win.TileUp()
			assert.False(t, ok)

			wof, ok = win.TileDown()
			require.True(t, ok)
			assert.Equal(t, 'A', wof.Content().(*component.TestComponent).Ch)

			win, ok = wm.WindowAt(term.Coordinates{X: 0, Y: 2})
			assert.True(t, ok)
			assertEqualTile(t, win, 'l')

			wof, ok = win.TileLeft()
			assert.False(t, ok)

			wof, ok = win.TileRight()
			require.True(t, ok)
			assertEqualTile(t, wof, 'L')

			wof, ok = win.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'U')

			wof, ok = win.TileDown()
			require.True(t, ok)
			assertEqualTile(t, wof, 'D')

			win, ok = wm.WindowAt(term.Coordinates{X: 19, Y: 2})
			require.True(t, ok)
			assertEqualTile(t, win, 'r')

			wof, ok = win.TileLeft()
			require.True(t, ok)
			assertEqualTile(t, wof, 'R')

			wof, ok = win.TileRight()
			assert.False(t, ok)

			wof, ok = win.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'U')

			wof, ok = win.TileDown()
			require.True(t, ok)
			assertEqualTile(t, wof, 'D')

			win, ok = wm.WindowAt(term.Coordinates{X: 19, Y: 7})
			require.True(t, ok)
			assertEqualTile(t, win, 'd')

			wof, ok = win.TileLeft()
			assert.False(t, ok)

			wof, ok = win.TileRight()
			assert.False(t, ok)

			wof, ok = win.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'D')

			wof, ok = win.TileDown()
			assert.False(t, ok)

		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌────────────┐─┐┐
││L│AAAAAAAAAAAA│R││
└└─└────────────┘─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		}, {Action: func() {
			win, ok := wm.WindowAt(term.Coordinates{Y: 6})
			require.True(t, ok)
			assertEqualTile(t, win, 'D')

			require.True(t, win.Unminimize())
		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌────────────┐─┐┐
││L│AAAA┌──┐AAAA│R││
││L│AAAA│DD│AAAA│R││
││L│AAAA└──┘AAAA│R││
└└─└────────────┘─┘┘
└──────────────────┘`,
		}, {Action: func() {
			win, ok := wm.WindowAt(term.Coordinates{Y: 3, X: 9})
			require.True(t, ok)
			assertEqualTile(t, win, 'D')
			require.True(t, win.MinimizeDown(1))

			w := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'X'}, 7, 100),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			wof, ok := w.TileRight()
			require.True(t, ok)
			assert.Equal(t, 'A', wof.Content().(*component.TestComponent).Ch)
		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌────────────┐─┐┐
││L│AXXXXXXXXXAA│R││
└└─└────────────┘─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		}, {Action: func() {
			win, ok := wm.WindowAt(term.Coordinates{Y: 3, X: 6})
			require.True(t, ok)
			assertEqualTile(t, win, 'X')
			require.NoError(t, win.Close())

			w := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'X'}, 100, 100),
				FloatingConfig{
					Alignment: component.AlignmentCentered,
				},
			)
			wof, ok := w.TileRight()
			require.True(t, ok)
			assertEqualTile(t, wof, 'A')

			wof, ok = w.TileLeft()
			require.True(t, ok)
			assertEqualTile(t, wof, 'A')

			wof, ok = w.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'A')

			wof, ok = w.TileDown()
			require.True(t, ok)
			assertEqualTile(t, wof, 'A')
		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌────────────┐─┐┐
││L│XXXXXXXXXXXX│R││
└└─└────────────┘─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		}, {Action: func() {
			assert.NotPanics(t, func() {
				wm.Resize(2, 2)
				wm.WindowAt(term.Coordinates{X: 0, Y: 0})
				wm.WindowAt(term.Coordinates{X: 0, Y: 1})
				wm.WindowAt(term.Coordinates{X: 1, Y: 0})
				wm.WindowAt(term.Coordinates{X: 1, Y: 1})
			})
		}, Expected: `

                    
                    
                    
    X               
                    
                    
                    
                    `,
		}, {Action: func() {
			wm.Resize(20, 8)
			win, ok := wm.WindowAt(term.Coordinates{Y: 3, X: 6})
			require.True(t, ok)
			assertEqualTile(t, win, 'X')
			require.NoError(t, win.Close())

			win, ok = wm.WindowAt(term.Coordinates{Y: 3, X: 6})
			require.True(t, ok)
			assert.Equal(t, 'A', win.Content().(*component.TestComponent).Ch)

			assert.True(t, win.MinimizeUp(0))
			_, ok = wm.SplitVertical(win, &component.TestComponent{Ch: 'a'})
			require.True(t, ok)

			require.True(t, win.MinimizeLeft(0))

			win, ok = wm.WindowAt(term.Coordinates{Y: 2, X: 6})
			require.True(t, ok)
			assert.Equal(t, 'a', win.Content().(*component.TestComponent).Ch)

			wof, ok := win.TileRight()
			require.True(t, ok)
			assertEqualTile(t, wof, 'R')

			wof, ok = win.TileLeft()
			require.True(t, ok)
			assert.Equal(t, 'A', wof.Content().(*component.TestComponent).Ch)

			wof, ok = win.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'U')

			wof, ok = win.TileDown()
			require.True(t, ok)
			assertEqualTile(t, wof, 'D')
		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌─┐┌─────────┐─┐┐
││L│A││aaaaaaaaa│R││
└└─└─┘└─────────┘─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		}, {Action: func() {
			win, ok := wm.WindowAt(term.Coordinates{Y: 4, X: 16})
			require.True(t, ok)
			assert.Equal(t, 'a', win.Content().(*component.TestComponent).Ch)

			win, ok = wm.SplitHorizontal(win, &component.TestComponent{Ch: 'b'})
			require.True(t, ok)

			require.True(t, win.MinimizeDown(0))

			win, ok = wm.WindowAt(term.Coordinates{Y: 2, X: 6})
			require.True(t, ok)
			assert.Equal(t, 'A', win.Content().(*component.TestComponent).Ch)

			wof, ok := win.TileRight()
			require.True(t, ok)
			assert.Equal(t, 'b', wof.Content().(*component.TestComponent).Ch)

			wof, ok = win.TileLeft()
			require.True(t, ok)
			assertEqualTile(t, wof, 'L')

			wof, ok = win.TileUp()
			require.True(t, ok)
			assertEqualTile(t, wof, 'U')

			wof, ok = win.TileDown()
			require.True(t, ok)
			assertEqualTile(t, wof, 'D')
		}, Expected: `
┌──────────────────┐
│UUUUUUUUUUUUUUUUUU│
┌┌─┌─────────┐aaa─┐┐
││L│AAAAAAAAA│bbbR││
└└─└─────────┘bbb─┘┘
│DDDDDDDDDDDDDDDDDD│
└──────────────────┘
└──────────────────┘`,
		},
	}

	comptest.TestComponent(t, wm, w, tests)
}

func TestExposedRootTileAt(t *testing.T) {
	h1 := component.TestComponent{Ch: 'A'}
	wm, w1 := NewWindowManager(&h1, testWindowManagerConfig())
	wm.Resize(20, 8)

	_, found := w1.TileUp()
	require.False(t, found)
	_, found = w1.TileLeft()
	require.False(t, found)
	_, found = w1.TileRight()
	require.False(t, found)
	_, found = w1.TileDown()
	require.False(t, found)

	h2 := component.TestComponent{Ch: 'B'}
	w2, ok := wm.SplitVertical(w1, &h2)
	require.True(t, ok)

	_, found = w2.TileUp()
	assert.False(t, found)

	require.NoError(t, w1.Close())

	_, found = w2.TileUp()
	assert.False(t, found)
	_, found = w2.TileLeft()
	assert.False(t, found)
	_, found = w2.TileRight()
	assert.False(t, found)
	_, found = w2.TileDown()
	assert.False(t, found)
}

func TestWindowManagerTileFocusFloating(t *testing.T) {
	w := term.NewStringWriter(20, 8)

	wm, w1 := NewWindowManager(&component.TestComponent{Ch: 'A'}, testWindowManagerConfig())
	wm.Resize(20, 8)

	_, ok := w1.TileUp()
	require.False(t, ok)

	w2, ok := wm.SplitVertical(w1, &component.TestComponent{Ch: 'B'})
	require.True(t, ok)
	wm.SplitHorizontal(w1, &component.TestComponent{Ch: 'C'})
	wm.SplitHorizontal(w2, &component.TestComponent{Ch: 'D'})

	var f1 Window
	tests := []comptest.TestCase{
		{
			Action: func() {
				floating := component.StaticFloating(&component.TestComponent{Ch: 'a'}, 2, 4)
				f1 = wm.FloatingWindow(floating,
					FloatingConfig{
						Alignment: component.AlignmentHorizontallyCentered,
					},
				)
				_, ok := f1.TileUp()
				require.False(t, ok)

				actual, ok := f1.TileLeft()
				require.True(t, ok)
				assertEqualTile(t, actual, 'A')

				actual, ok = f1.TileRight()
				require.True(t, ok)
				assertEqualTile(t, actual, 'B')

				actual, ok = f1.TileDown()
				require.True(t, ok)
				assertEqualTile(t, actual, 'A')
			}, Expected: `
┌───────┌──┐───────┐
│AAAAAAA│aa│BBBBBBB│
│AAAAAAA│aa│BBBBBBB│
└───────│aa│───────┘
┌───────│aa│───────┐
│CCCCCCC└──┘DDDDDDD│
│CCCCCCCC││DDDDDDDD│
└────────┘└────────┘`,
		},
		{
			Action: func() {
				require.NoError(t, f1.Close())

				floating := component.StaticFloating(&component.TestComponent{Ch: 'a'}, 2, 4)
				f1 = wm.FloatingWindow(floating,
					FloatingConfig{
						Alignment: component.AlignmentHorizontallyCentered | component.AlignmentBottom,
					},
				)
				_, ok := f1.TileDown()
				require.False(t, ok)

				actual, ok := f1.TileLeft()
				require.True(t, ok)
				assertEqualTile(t, actual, 'C')

				actual, ok = f1.TileRight()
				require.True(t, ok)
				assertEqualTile(t, actual, 'D')

				actual, ok = f1.TileUp()
				require.True(t, ok)
				assertEqualTile(t, actual, 'D')
			}, Expected: `
┌────────┐┌────────┐
│AAAAAAAA││BBBBBBBB│
│AAAAAAA┌──┐BBBBBBB│
└───────│aa│───────┘
┌───────│aa│───────┐
│CCCCCCC│aa│DDDDDDD│
│CCCCCCC│aa│DDDDDDD│
└───────└──┘───────┘`,
		},
		{
			Action: func() {
				require.NoError(t, f1.Close())

				floating := component.StaticFloating(&component.TestComponent{Ch: 'a'}, 12, 2)
				f1 = wm.FloatingWindow(floating,
					FloatingConfig{
						Alignment: component.AlignmentVerticallyCentered,
					},
				)
				_, ok := f1.TileLeft()
				require.False(t, ok)

				actual, ok := f1.TileUp()
				require.True(t, ok)
				assertEqualTile(t, actual, 'A')

				actual, ok = f1.TileDown()
				require.True(t, ok)
				assertEqualTile(t, actual, 'C')

				actual, ok = f1.TileRight()
				require.True(t, ok)
				assertEqualTile(t, actual, 'A')
			}, Expected: `
┌────────┐┌────────┐
│AAAAAAAA││BBBBBBBB│
┌────────────┐BBBBB│
│aaaaaaaaaaaa│─────┘
│aaaaaaaaaaaa│─────┐
└────────────┘DDDDD│
│CCCCCCCC││DDDDDDDD│
└────────┘└────────┘`,
		},
		{
			Action: func() {
				require.NoError(t, f1.Close())

				floating := component.StaticFloating(&component.TestComponent{Ch: 'a'}, 12, 2)
				f1 = wm.FloatingWindow(floating,
					FloatingConfig{
						Alignment: component.AlignmentVerticallyCentered | component.AlignmentRight,
					},
				)
				_, ok := f1.TileRight()
				require.False(t, ok)

				actual, ok := f1.TileUp()
				require.True(t, ok)
				assertEqualTile(t, actual, 'B')

				actual, ok = f1.TileDown()
				require.True(t, ok)
				assertEqualTile(t, actual, 'D')

				actual, ok = f1.TileLeft()
				require.True(t, ok)
				assertEqualTile(t, actual, 'D')
			}, Expected: `
┌────────┐┌────────┐
│AAAAAAAA││BBBBBBBB│
│AAAAA┌────────────┐
└─────│aaaaaaaaaaaa│
┌─────│aaaaaaaaaaaa│
│CCCCC└────────────┘
│CCCCCCCC││DDDDDDDD│
└────────┘└────────┘`,
		},
	}

	comptest.TestComponent(t, wm, w, tests)
}

func TestComponentWindowAt(t *testing.T) {
	h1 := &component.TestComponent{Ch: '1'}
	wm, w1 := NewWindowManager(h1, testWindowManagerConfig())
	wm.Resize(20, 8)

	h2 := &component.TestComponent{Ch: '2'}
	w2, ok := wm.SplitVertical(w1, h2)
	require.True(t, ok)

	h3 := &component.TestComponent{Ch: '3'}
	w3, ok := wm.SplitHorizontal(w2, h3)
	require.True(t, ok)

	h4 := &component.TestComponent{Ch: '4'}
	w4, ok := wm.SplitVertical(w3, h4)
	require.True(t, ok)

	wf := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'f'}, 100, 100),
		FloatingConfig{
			Alignment: component.AlignmentCentered,
		},
	)
	require.True(t, wf.MinimizeDown(1))

	wF := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'F'}, 2, 2),
		FloatingConfig{
			Alignment: component.AlignmentCentered,
		},
	)

	/*
			  ┌────────┐┌────────┐
		      │1111111┌──┐2222222│
		      │1111111│FF│───────┘
		      │1111111│FF│──┐┌───┐
		      │1111111└──┘33││444│
		      └────────┘└───┘└───┘
		      │ffffffffffffffffff│
		      └──────────────────┘
	*/

	suite := []struct {
		at               term.Coordinates
		expectedOut      Window
		expectedNotFound bool
	}{
		{
			at:          term.Coordinates{X: 10, Y: 3},
			expectedOut: wF,
		},
		{
			at:          term.Coordinates{X: 14, Y: 5},
			expectedOut: w3,
		},
		{
			at:          term.Coordinates{X: 12, Y: 3},
			expectedOut: w3,
		},
		{
			at:          term.Coordinates{X: 19, Y: 2},
			expectedOut: w2,
		},
		{
			at:          term.Coordinates{X: 10, Y: 0},
			expectedOut: w2,
		},
		{
			at:          term.Coordinates{X: 0, Y: 6},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 0, Y: 7},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 19, Y: 7},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 19, Y: 6},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 0, Y: 5},
			expectedOut: w1,
		},
		{
			at:          term.Coordinates{X: 9, Y: 0},
			expectedOut: w1,
		},
		{
			at:          term.Coordinates{X: 15, Y: 3},
			expectedOut: w4,
		},
		{
			at:          term.Coordinates{X: 19, Y: 5},
			expectedOut: w4,
		},
		{
			at:          term.Coordinates{X: 8, Y: 1},
			expectedOut: wF,
		},
		{
			at:          term.Coordinates{X: 11, Y: 4},
			expectedOut: wF,
		},
	}
	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			actualOut, actualOk := wm.WindowAt(test.at)
			require.Equal(t, !test.expectedNotFound, actualOk)
			assert.Equal(t, test.expectedOut, actualOut)
		})
	}

	wx := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'x'}, 100, 100),
		FloatingConfig{
			Alignment: component.AlignmentCentered,
		},
	)
	require.True(t, wx.MinimizeUp(2))
	wy := wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'y'}, 100, 100),
		FloatingConfig{
			Alignment: component.AlignmentCentered,
		},
	)
	require.True(t, wy.MinimizeLeft(2))

	/*
		┌──────────────────┐
		│xxxxxxxxxxxxxxxxxx│
		│xxxxxxxxxxxxxxxxxx│
		┌──┌─────┌──┐2222222
		│yy│11111│FF│3344444
		└──└─────└──┘3344444
		│ffffffffffffffffff│
		└──────────────────┘
	*/

	suite = []struct {
		at               term.Coordinates
		expectedOut      Window
		expectedNotFound bool
	}{
		{
			at:          term.Coordinates{X: 10, Y: 4},
			expectedOut: wF,
		},
		{
			at:          term.Coordinates{X: 13, Y: 4},
			expectedOut: w3,
		},
		{
			at:          term.Coordinates{X: 14, Y: 5},
			expectedOut: w3,
		},
		{
			at:          term.Coordinates{X: 13, Y: 3},
			expectedOut: w2,
		},
		{
			at:          term.Coordinates{X: 19, Y: 3},
			expectedOut: w2,
		},
		{
			at:          term.Coordinates{X: 0, Y: 6},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 0, Y: 7},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 19, Y: 7},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 19, Y: 6},
			expectedOut: wf,
		},
		{
			at:          term.Coordinates{X: 3, Y: 3},
			expectedOut: w1,
		},
		{
			at:          term.Coordinates{X: 8, Y: 5},
			expectedOut: w1,
		},
		{
			at:          term.Coordinates{X: 15, Y: 4},
			expectedOut: w4,
		},
		{
			at:          term.Coordinates{X: 19, Y: 5},
			expectedOut: w4,
		},
		{
			at:          term.Coordinates{X: 9, Y: 4},
			expectedOut: wF,
		},
		{
			at:          term.Coordinates{X: 12, Y: 4},
			expectedOut: wF,
		},
	}
	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			actualOut, actualOk := wm.WindowAt(test.at)
			require.Equal(t, !test.expectedNotFound, actualOk)
			assert.Equal(t, test.expectedOut, actualOut)
		})
	}
}

func TestFixedSizeWindows(t *testing.T) {
	h1 := &component.TestComponent{Ch: '1'}
	wm, w1 := NewWindowManager(h1, testWindowManagerConfig())
	wm.Resize(20, 8)

	var w2, w3, w4, wf, wF Window
	tests := []comptest.TestCase{
		{
			Action: func() {

				assert.False(t, wm.SetHeight(w1, 3))
				// it's the only window so it should fail
				require.False(t, wm.SetWidth(w1, 3))
				assert.Equal(t, 20, w1.MaxWidth())
				assert.Equal(t, 8, w1.MaxHeight())
				assert.Equal(t, 3, w1.MinWidth())
				assert.Equal(t, 3, w1.MinHeight())

				h2 := &component.TestComponent{Ch: '2'}
				var ok bool
				w2, ok = wm.SplitVertical(w1, h2)
				require.True(t, ok)

				assert.Equal(t, 17, w1.MaxWidth())
				assert.Equal(t, 8, w1.MaxHeight())
				assert.Equal(t, 17, w2.MaxWidth())
				assert.Equal(t, 8, w2.MaxHeight())

				assert.False(t, wm.SetHeight(w2, 3))
				require.True(t, wm.SetWidth(w1, 3))
				// should reset the fixed width of w1
				require.True(t, wm.SetWidth(w2, 3))

				// cannot resize to less than content size 1
				// with frame, that is less than 3.
				require.False(t, wm.SetWidth(w2, 2))

			}, Expected: `
┌───────────────┐┌─┐
│111111111111111││2│
│111111111111111││2│
│111111111111111││2│
│111111111111111││2│
│111111111111111││2│
│111111111111111││2│
└───────────────┘└─┘`,
		},
		{
			Action: func() {
				h3 := &component.TestComponent{Ch: '3'}
				var ok bool
				w3, ok = wm.SplitHorizontal(w2, h3)
				require.True(t, ok)
				assert.Equal(t, 17, w1.MaxWidth())
				assert.Equal(t, 8, w1.MaxHeight())
				assert.Equal(t, 17, w2.MaxWidth())
				assert.Equal(t, 5, w2.MaxHeight())
				assert.Equal(t, 17, w3.MaxWidth())
				assert.Equal(t, 5, w3.MaxHeight())
			}, Expected: `
┌───────────────┐┌─┐
│111111111111111││2│
│111111111111111││2│
│111111111111111│└─┘
│111111111111111│┌─┐
│111111111111111││3│
│111111111111111││3│
└───────────────┘└─┘`,
		},
		{
			Action: func() {
				assert.True(t, wm.SetHeight(w3, 3))
				assert.True(t, wm.SetWidth(w3, 13))
			}, Expected: `
┌─────┐┌───────────┐
│11111││22222222222│
│11111││22222222222│
│11111││22222222222│
│11111│└───────────┘
│11111│┌───────────┐
│11111││33333333333│
└─────┘└───────────┘`,
		},
		{
			Action: func() {
				h4 := &component.TestComponent{Ch: '4'}
				var ok bool
				w4, ok = wm.SplitVertical(w3, h4)
				require.True(t, ok)
				assert.Equal(t, 17, w1.MaxWidth()) // let's keep things simple
				assert.Equal(t, 8, w1.MaxHeight())
				assert.Equal(t, 17, w2.MaxWidth())
				assert.Equal(t, 5, w2.MaxHeight())
				assert.Equal(t, 10, w3.MaxWidth()) // ditto
				assert.Equal(t, 5, w3.MaxHeight())
				assert.Equal(t, 10, w4.MaxWidth()) // ditto
				assert.Equal(t, 5, w3.MaxHeight())
			}, Expected: `
┌─────┐┌───────────┐
│11111││22222222222│
│11111││22222222222│
│11111││22222222222│
│11111│└───────────┘
│11111│┌────┐┌─────┐
│11111││3333││44444│
└─────┘└────┘└─────┘`,
		},
		{
			Action: func() {
				wf = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'f'}, 100, 100),
					FloatingConfig{
						Alignment: component.AlignmentCentered,
					},
				)
				require.True(t, wf.MinimizeDown(1))

				wF = wm.FloatingWindow(component.StaticFloating(&component.TestComponent{Ch: 'F'}, 2, 2),
					FloatingConfig{
						Alignment: component.AlignmentCentered,
					},
				)

				assert.Equal(t, 17, w1.MaxWidth())
				assert.Equal(t, 6, w1.MaxHeight())
				assert.Equal(t, 17, w2.MaxWidth())
				assert.Equal(t, 3, w2.MaxHeight())
				assert.Equal(t, 10, w3.MaxWidth())
				assert.Equal(t, 3, w3.MaxHeight())
				assert.Equal(t, 10, w4.MaxWidth())
				assert.Equal(t, 3, w3.MaxHeight())
			}, Expected: `
┌─────┐┌───────────┐
│11111││┌──┐2222222│
│11111│└│FF│───────┘
│11111│┌│FF│┐┌─────┐
│11111││└──┘││44444│
└─────┘└────┘└─────┘
│ffffffffffffffffff│
└──────────────────┘`,
		},
		{
			Action: func() {
				assert.False(t, wm.SetHeight(w4, w4.Height()+1))
				assert.True(t, wm.SetWidth(w4, w4.Width()+2))
			}, Expected: `
┌─────┐┌───────────┐
│11111││┌──┐2222222│
│11111│└│FF│───────┘
│11111│┌│FF│───────┐
│11111││└──┘4444444│
└─────┘└──┘└───────┘
│ffffffffffffffffff│
└──────────────────┘`,
		},
		{
			Action: func() {
				assert.False(t, wm.SetHeight(w2, w2.Height()+1))
				assert.True(t, wm.SetWidth(w2, w2.Width()+2))
				assert.False(t, wm.SetWidth(w2, w2.Width()+8))
			}, Expected: `
┌───┐┌─────────────┐
│111││22┌──┐2222222│
│111│└──│FF│───────┘
│111│┌──│FF│───────┐
│111││33└──┘4444444│
└───┘└────┘└───────┘
│ffffffffffffffffff│
└──────────────────┘`,
		},
		{
			Action: func() {
				assert.False(t, wm.SetHeight(wf, 5)) // is minimized
				assert.False(t, wm.SetWidth(wf, 5))  // is minimized
				assert.True(t, wm.SetHeight(wF, wF.Height()+1))
				assert.True(t, wm.SetWidth(wF, wF.Width()+1))

				assert.Equal(t, 4, wF.MinHeight())
				assert.Equal(t, 4, wF.MinWidth())
				assert.Equal(t, 6, wF.MaxHeight())
				assert.Equal(t, 20, wF.MaxWidth())
			}, Expected: `
┌───┐┌─────────────┐
│111││2┌───┐2222222│
│111│└─│FFF│───────┘
│111│┌─│FFF│───────┐
│111││3└───┘4444444│
└───┘└────┘└───────┘
│ffffffffffffffffff│
└──────────────────┘`,
		},
		{
			Action: func() {
				assert.True(t, wm.SetHeight(wF, 0)) // reset
				assert.True(t, wm.SetWidth(wF, 0))  // reset
			}, Expected: `
┌───┐┌─────────────┐
│111││22┌──┐2222222│
│111│└──│FF│───────┘
│111│┌──│FF│───────┐
│111││33└──┘4444444│
└───┘└────┘└───────┘
│ffffffffffffffffff│
└──────────────────┘`,
		},
		{
			Action: func() {
				require.NoError(t, w3.Close())
				require.NoError(t, w1.Close())
			}, Expected: `
┌──────────────────┐
│2222222┌──┐2222222│
└───────│FF│───────┘
┌───────│FF│───────┐
│4444444└──┘4444444│
└──────────────────┘
│ffffffffffffffffff│
└──────────────────┘`,
		},
		{
			Action: func() {
				require.NoError(t, wf.Close())
				require.NoError(t, wF.Close())
				require.NoError(t, w2.Close())
				require.Error(t, w4.Close())
			}, Expected: `
┌──────────────────┐
│444444444444444444│
│444444444444444444│
│444444444444444444│
│444444444444444444│
│444444444444444444│
│444444444444444444│
└──────────────────┘`,
		},
	}

	w := term.NewStringWriter(20, 8)
	comptest.TestComponent(t, wm, w, tests)
}

func TestSetFrameAttr(t *testing.T) {
	h1 := &component.TestComponent{Ch: '1'}
	cfg := testWindowManagerConfig()
	wm, w1 := NewWindowManager(h1, cfg)
	wm.Resize(20, 8)

	newAttr := term.Attributes{Fg: tcell.ColorGreen, Bg: tcell.ColorBlue}
	prev, ok := w1.SetFrameAttr(newAttr)
	assert.True(t, ok)
	assert.Equal(t, cfg.FrameAttr, prev)

	actualAttr, ok := w1.SetFrameAttr(prev)
	assert.True(t, ok)
	assert.Equal(t, newAttr, actualAttr)
}

func assertEqualTile(t *testing.T, win Window, expected rune) {
	t.Helper()
	if f, ok := win.Content().(interface { Content() tui.Component }); ok {
		assert.Equal(t, string(expected), string(f.Content().(*component.TestComponent).Ch))
	} else {
		assert.Equal(t, string(expected), string(win.Content().(*component.TestComponent).Ch))
	}

}

func testWindowManagerConfig() WindowManagerConfig {
	ret := DefaultWindowManagerConfig()
	ret.NoMaxSize = true
	return ret
}
