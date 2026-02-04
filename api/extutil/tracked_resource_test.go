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

package extutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
	"unstable.build/go-tui/workspace"
)

func TestResourceTrackerIntegration(t *testing.T) {
	suite := []struct {
		description string
		evs         []textapi.EventType
	}{
		{"complete event set", ResourceTrackerEventsComplete()},
		{"content changes only", ResourceTrackerEventsContent()},
		{"flushed content changes only", ResourceTrackerEventsFlushOnly()},
		{"content changes with scroll", append(ResourceTrackerEventsContent(), textapi.EventTypeScroll)},
	}
	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			tabspaces := 4
			simpleEd := modeless.Editor()

			cfg := text.DefaultConfig()
			cfg.Tabspaces = tabspaces

			cwd := makeURI(t, "memory:///")

			scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), cwd)
			require.NoError(t, err)
			loader := workspace.NewSchemeWorkspace(cwd, scheme)

			ed, err := text.NewComponent(simpleEd, loader, cfg)
			require.NoError(t, err)

			tracker := NewResourceTracker(tabspaces, false)
			require.NoError(t, ed.SubscribeEvents(test.evs, tracker))

			ed.Resize(8, 7)

			res1 := makeURI(t, "memory:///1")

			// sut
			var bh browserapi.Handler
			var edh text.Handler
			t.Run("Edit on editor is tracked by tracker", func(t *testing.T) {
				bh, err = ed.OpenFileTab(res1, false)
				require.NoError(t, err)

				res, ok := tracker.Resource(res1)
				require.True(t, ok)

				assert.Equal(t, false, res.Scroll.Wrap)
				assert.Equal(t, tabspaces, res.Scroll.Tabspaces())

				edh, err = ed.Editor(res1)
				require.NoError(t, err)

				assert.Equal(t, "memory:///1", res.URI().String())
				assert.Equal(t, "", res.Scroll.Buffer().String())
				// not in focus yet, so propagated scroll width, height is 0
				assert.Equal(t, 0, res.Scroll.Width())
				assert.Equal(t, 0, res.Scroll.SizeHeight())
			})

			t.Run("switching focus to content propagates width, height", func(t *testing.T) {
				win, err := ed.Focus()
				require.NoError(t, err)
				require.NoError(t, win.SetContent(bh))

				res, ok := tracker.Resource(res1)
				require.True(t, ok)

				assert.Equal(t, 6, res.Scroll.Width())
				assert.Equal(t, 3, res.Scroll.SizeHeight())
			})

			t.Run("Focus returns last resource in focus", func(t *testing.T) {
				res, ok := tracker.Focus()
				require.True(t, ok)

				assert.Equal(t, res1, res.URI())
			})

			if setHasType(textapi.EventTypeEdit, test.evs) {
				t.Run("updates to buffer are replicated to resource", func(t *testing.T) {
					res, ok := tracker.Resource(res1)
					require.True(t, ok)

					edh.CellEditor().
						Edit(context.Background(), term.Coordinates{}, term.Coordinates{},
							"abcdefghi\n1234\nXXXX\nX\nX\nX\nX\nX\nX")

					assert.Equal(t, "abcdefghi\n1234\nXXXX\nX\nX\nX\nX\nX\nX",
						res.Scroll.Buffer().String())

					winpos, ok := res.WindowCoordinates(term.Coordinates{})
					require.True(t, ok)
					assert.NotPanics(t, func() {
						assert.Equal(t, term.Coordinates{}, res.ContentCoordinates(term.Coordinates{}))
						assert.Equal(t, term.Coordinates{}, winpos)
					})
				})
			}

			if setHasType(textapi.EventTypeScroll, test.evs) &&
				setHasType(textapi.EventTypeEdit, test.evs) {
				t.Run("scroll position is replicated to resource", func(t *testing.T) {
					_, handled := bh.Handle(term.Event{Type: term.EventKey, Mod: term.ModMeta, Key: term.KeyArrowUp})
					require.True(t, handled)
					for handled {
						_, handled = bh.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowDown})
					}

					res, ok := tracker.Resource(res1)
					require.True(t, ok)

					winPos, ok := res.WindowCoordinates(term.Coordinates{})
					require.True(t, ok)
					assert.Equal(t, term.Coordinates{Y: 6}, res.Offset())
					assert.Equal(t, term.Coordinates{Y: 6}, res.ContentCoordinates(term.Coordinates{}))
					assert.Equal(t, term.Coordinates{Y: -6}, winPos)
					assert.NotPanics(t, func() {
						_ = res.Cursor()
					})
				})
			}

			if setHasType(textapi.EventTypeCursor, test.evs) &&
				setHasType(textapi.EventTypeScroll, test.evs) &&
				setHasType(textapi.EventTypeEdit, test.evs) {
				t.Run("cursor position is replicated to resource", func(t *testing.T) {
					res, ok := tracker.Resource(res1)
					require.True(t, ok)

					cur := edh.CursorAtScroll()
					require.NoError(t, err)
					assert.Equal(t, term.Coordinates{Y: 8, X: 1}, cur)
					assert.Equal(t, term.Coordinates{Y: 8, X: 1}, res.Cursor())

					edh.SetCursorAtScroll(term.Coordinates{Y: 2})
					assert.Equal(t, term.Coordinates{Y: 2}, res.Cursor())
				})

			}

			if setHasType(textapi.EventTypeHidden, test.evs) {
				t.Run("marking lines hidden/visible is replicated", func(t *testing.T) {
					// reset
					handled := true
					for handled {
						_, handled = bh.Handle(term.Event{
							Type: term.EventKey, Key: term.KeyArrowUp})
					}
					_, handled = bh.Handle(term.Event{
						Type: term.EventKey, Mod: term.ModCtrl, Ch: 'a'})

					// mark line as hidden
					for range 3 {
						bh.Handle(term.Event{Type: term.EventKey, Mod: term.ModShift, Key: term.KeyArrowDown})
					}
					bh.Handle(term.Event{
						Type: term.EventKey,
						Ch:   'h',
						Mod:  term.ModCtrlAlt,
					})

					res, ok := tracker.Resource(res1)
					require.True(t, ok)

					winPos := res.ContentCoordinates(term.Coordinates{Y: 1})
					require.True(t, ok)
					assert.Equal(t, term.Coordinates{Y: 4}, winPos)

					// mark line as visible
					bh.Handle(term.Event{
						Type: term.EventKey,
						Ch:   'v',
						Mod:  term.ModCtrlAlt,
					})
					winPos = res.ContentCoordinates(term.Coordinates{Y: 1})
					require.True(t, ok)
					assert.Equal(t, term.Coordinates{Y: 1}, winPos)
				})
			}
		})
	}
}

func makeURI(t *testing.T, uriStr string) workspaceapi.URI {
	uri, err := workspaceapi.ParseURI(uriStr)
	require.NoError(t, err)
	return uri
}

func setHasType(t textapi.EventType, evs []textapi.EventType) bool {
	for _, ev := range evs {
		if ev == t {
			return true
		}
	}
	return false
}
