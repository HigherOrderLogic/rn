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

package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestTabsOnClick(t *testing.T) {
	t.Run("dispatches click event out of any tabs as -1 index", func(t *testing.T) {
		h := NewTabs()
		var called int
		h.OnClick = func(tabIdx int) bool {
			called++
			assert.Equal(t, -1, tabIdx)
			return true
		}

		_, handled := h.Handle(term.Event{
			Type:   term.EventMouse,
			Key:    term.MouseLeft,
			MouseX: 100000,
		})
		assert.True(t, handled)

		assert.Equal(t, 1, called)
	})

	t.Run("dispatches click event on tab with tab index", func(t *testing.T) {
		h := NewTabs()
		var called int
		h.OnClick = func(tabIdx int) bool {
			called++
			assert.Equal(t, 1, tabIdx)
			return true
		}

		assert.Equal(t, 0, h.Add(0, "Burning"))
		assert.Equal(t, 1, h.Add(0, "Man"))
		assert.Equal(t, 2, h.Add('x', "2024"))

		_, handled := h.Handle(term.Event{
			Type:   term.EventMouse,
			Key:    term.MouseLeft,
			MouseX: 10,
		})
		assert.True(t, handled)

		assert.Equal(t, 1, called)
	})

	t.Run("mouse drag dispatches OnClick only once", func(t *testing.T) {
		h := NewTabs()
		var called int
		h.OnClick = func(tabIdx int) bool {
			called++
			return true
		}

		_, handled := h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.True(t, handled)

		handled, _ = h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.False(t, handled)

		handled, _ = h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.False(t, handled)

		handled, _ = h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.False(t, handled)

		assert.Equal(t, 1, called)
	})

	t.Run("MouseLeft, MouseRelease, MouseLeft dispatches OnClick twice", func(t *testing.T) {
		h := NewTabs()
		var called int
		h.OnClick = func(tabIdx int) bool {
			called++
			return true
		}

		_, handled := h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.True(t, handled)

		_, handled = h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.False(t, handled)

		_, handled = h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseRelease})
		assert.False(t, handled)

		_, handled = h.Handle(term.Event{Type: term.EventMouse, Key: term.MouseLeft})
		assert.True(t, handled)

		assert.Equal(t, 2, called)
	})
}
