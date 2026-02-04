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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestTabsDraw(t *testing.T) {
	l := NewTabs()
	l.Resize(20, 4)
	fb := component.FrameCharSetDefault()
	fb.BottomLeft, fb.BottomRight = '├', '┤'
	l.SetFrameCharSet(fb)

	w := term.NewStringWriter(20, 9)

	tests := []comptest.TestCase{
		{
			nil, `
┌──────────────────┐
│                  │
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.Add('X', "Atzari") }, `
┌──────────────────┐
│X Atzari          │
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.Resize(20, 9) }, `
┌──────────────────┐
│                  │
│                  │
│                  │
│X Atzari          │
│                  │
│                  │
│                  │
├──────────────────┤`,
		}, {
			func() {
				l.Add('$', "Saturn")
				l.Resize(20, 4)
			}, `
┌──────────────────┐
│X Atzari  $ Saturn│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() {
				assert.True(t, l.MoveLeft(1))
				assert.False(t, l.MoveLeft(0))
			}, `
┌──────────────────┐
│$ Saturn  X Atzari│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() {
				assert.True(t, l.MoveTo(1, 0))
			}, `
┌──────────────────┐
│X Atzari  $ Saturn│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() {
				assert.True(t, l.MoveTo(0, 1))
			}, `
┌──────────────────┐
│$ Saturn  X Atzari│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() {
				assert.True(t, l.MoveRight(0))
				assert.False(t, l.MoveRight(1))
			}, `
┌──────────────────┐
│X Atzari  $ Saturn│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.Add('X', "Other") }, `
┌──────────────────┐
│X Atzari  $ Satu..│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.Add('X', "Things") }, `
┌──────────────────┐
│X Atzari  $ Satu..│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.SetFocus(2) }, `
┌──────────────────┐
│..  $ Saturn  X ..│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.Add('#', "Morsins"); l.Add('#', "Morsillonins") }, `
┌──────────────────┐
│..  $ Saturn  X ..│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.SetFocus(5) }, `
┌──────────────────┐
│..  # Morsillonins│
│                  │
├──────────────────┤
                    
                    
                    
                    
                    `,
		}, {
			func() { l.SetBorder(false); l.Resize(20, 1) }, `
..  # Morsillonins  
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() { l.SetFocus(3) }, `
..  # Morsillonins  
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() { l.SetBorder(false); l.Resize(4, 0) }, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		},
	}

	comptest.TestComponent(t, l, w, tests)
}

func TestTabsDrawCustomSeparator(t *testing.T) {
	l := NewTabs()
	l.Resize(20, 4)
	l.SetNameSeparator(" | ")

	w := term.NewStringWriter(20, 9)

	tests := []comptest.TestCase{
		{
			nil, `
┌──────────────────┐
│                  │
│                  │
└──────────────────┘
                    
                    
                    
                    
                    `,
		}, {
			func() { l.Add('#', "Atzari") }, `
┌──────────────────┐
│# Atzari          │
│                  │
└──────────────────┘
                    
                    
                    
                    
                    `,
		}, {
			func() {
				l.Add('#', "Saturn")
			}, `
┌──────────────────┐
│# Atzari | # Sat..│
│                  │
└──────────────────┘
                    
                    
                    
                    
                    `,
		},
	}

	comptest.TestComponent(t, l, w, tests)
}

func setupOneTab(width, height int) *Tabs {
	l := NewTabs()
	l.Resize(width, height)

	const name = "blah"
	l.Add('#', name)

	return l
}

func TestTabsTabAt(t *testing.T) {
	t.Run("should return false if no tab in list", func(t *testing.T) {
		l := NewTabs()
		_, ok := l.TabAt(term.Coordinates{})
		assert.False(t, ok)
	})

	t.Run("should return first tab if coordinates is zero", func(t *testing.T) {
		l := setupOneTab(4, 4)

		idx, ok := l.TabAt(term.Coordinates{})
		require.True(t, ok)
		assert.Equal(t, "blah", l.tabs[idx].name)
	})

	t.Run("should return first tab if size is 0", func(t *testing.T) {
		l := setupOneTab(0, 0)

		l.TabAt(term.Coordinates{})
		idx, ok := l.TabAt(term.Coordinates{})
		require.True(t, ok)
		assert.Equal(t, "blah", l.tabs[idx].name)
	})

	t.Run("should take offset into consideration", func(t *testing.T) {
		l := setupOneTab(4, 4)

		l.Add(0, "1111")
		l.Add(0, "2222222222222222222")
		l.Add(0, "3")
		l.SetFocus(l.Add(0, "4"))

		// force calculating offsets
		w := term.NewStringWriter(4, 4)
		l.Draw(w)
		require.NoError(t, w.Flush())

		idx, ok := l.TabAt(term.Coordinates{})
		require.True(t, ok)
		assert.Equal(t, "4", l.tabs[idx].name)
	})
}
