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

package text

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

func TestLocationStoreCursorIntegrationSortedLocations(t *testing.T) {
	t.Run("sorts by level", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)

		infoList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    abcAttr,
				Message: "info",
			},
			{
				From:    term.Coordinates{Y: 11},
				To:      term.Coordinates{Y: 11, X: 11},
				Attr:    abcAttr,
				Message: "info2",
			},
		})
		errList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 2},
				To:      term.Coordinates{Y: 2, X: 2},
				Attr:    abcAttr,
				Message: "err",
			},
		})
		criticalList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 3},
				To:      term.Coordinates{Y: 3, X: 3},
				Attr:    abcAttr,
				Message: "critical",
			},
		})
		warnList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 4},
				To:      term.Coordinates{Y: 4, X: 4},
				Attr:    abcAttr,
				Message: "warn",
			},
		})
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityError, "errList", errList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityCritical, "criticalList", criticalList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityWarning, "warnList", warnList))

		locations := c.SortedLocations()
		require.Len(t, locations, 5)

		assert.Equal(t, term.Coordinates{Y: 1}, locations[0].From)
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, locations[0].To)
		assert.Equal(t, "info", locations[0].Message)

		assert.Equal(t, "info2", locations[1].Message)
		assert.Equal(t, "warn", locations[2].Message)
		assert.Equal(t, "err", locations[3].Message)
		assert.Equal(t, "critical", locations[4].Message)
	})

	t.Run("sorts by id if level is the same", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)
		infoList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    abcAttr,
				Message: "info",
			},
			{
				From:    term.Coordinates{Y: 11},
				To:      term.Coordinates{Y: 11, X: 11},
				Attr:    abcAttr,
				Message: "info1",
			},
		})
		infoList2 := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 2},
				To:      term.Coordinates{Y: 2, X: 2},
				Attr:    abcAttr,
				Message: "info2",
			},
		})
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList2", infoList2))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))

		locations := c.SortedLocations()
		require.Len(t, locations, 3)

		assert.Equal(t, term.Coordinates{Y: 1}, locations[0].From)
		assert.Equal(t, term.Coordinates{Y: 1, X: 1}, locations[0].To)
		assert.Equal(t, "info", locations[0].Message)
		assert.Equal(t, "info1", locations[1].Message)
		assert.Equal(t, "info2", locations[2].Message)
	})

	t.Run("idempotency", func(t *testing.T) {
		c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)
		infoList := LocationSlice([]textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    abcAttr,
				Message: "info",
			},
			{
				From:    term.Coordinates{Y: 11},
				To:      term.Coordinates{Y: 11, X: 11},
				Attr:    abcAttr,
				Message: "info1",
			},
		})
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))

		require.Len(t, c.SortedLocations(), 2)
		require.Len(t, c.SortedLocations(), 2)
		require.Len(t, c.SortedLocations(), 2)
		require.Len(t, c.SortedLocations(), 2)
	})
}

func TestLocationStoreCursorIntegrationLocationLists(t *testing.T) {
	c := setupCursorContent(t, 10, 10, "\na\nb\nc\n", false)
	infoList := LocationSlice([]textapi.Location{
		{
			From:    term.Coordinates{Y: 1},
			To:      term.Coordinates{Y: 1, X: 1},
			Attr:    abcAttr,
			Message: "info",
		},
		{
			From:    term.Coordinates{Y: 11},
			To:      term.Coordinates{Y: 11, X: 11},
			Attr:    abcAttr,
			Message: "info1",
		},
	})
	infoList2 := LocationSlice([]textapi.Location{
		{
			From:    term.Coordinates{Y: 2},
			To:      term.Coordinates{Y: 2, X: 2},
			Attr:    abcAttr,
			Message: "info2",
		},
	})
	assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList2", infoList2))
	assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "infoList", infoList))

	expected := []LocationSet{
		{
			ID:       "infoList2",
			Priority: 0,
			Locations: []textapi.Location{
				{
					From:    term.Coordinates{X: 0, Y: 2},
					To:      term.Coordinates{X: 2, Y: 2},
					Attr:    abcAttr,
					Message: "info2",
				},
			},
		},
		{
			ID:       "infoList",
			Priority: 0,
			Locations: []textapi.Location{
				{
					From:    term.Coordinates{X: 0, Y: 1},
					To:      term.Coordinates{X: 1, Y: 1},
					Attr:    abcAttr,
					Message: "info",
				},
				{
					From:    term.Coordinates{X: 0, Y: 11},
					To:      term.Coordinates{X: 11, Y: 11},
					Attr:    abcAttr,
					Message: "info1",
				},
			},
		},
	}

	actual := c.LocationLists()
	require.Len(t, actual, 2)
	assert.ElementsMatch(t, expected, actual)
}

func TestLocationStoreCursorIntegrationSetLocationListMessages(t *testing.T) {
	content := "\naaa\nbbb\nccc\n"
	messageLocations := []textapi.Location{
		{
			From:    term.Coordinates{Y: 1},
			To:      term.Coordinates{Y: 1, X: 2},
			Attr:    abcAttr,
			Message: "1",
		},
		{
			From:    term.Coordinates{Y: 2},
			To:      term.Coordinates{Y: 2, X: 2},
			Attr:    abcAttr,
			Message: "2",
		},
		{
			From:    term.Coordinates{Y: 3},
			To:      term.Coordinates{Y: 3, X: 2},
			Attr:    abcAttr,
			Message: "3",
		},
	}

	assertMessages := func(t *testing.T, c *Cursor) {
		for i := range 3 {
			locs, ok := c.LocationsAtCursor()
			require.True(t, ok)
			require.Len(t, locs, 1)
			assert.Equal(t, locs[0].Message, strconv.Itoa(i+1))
			c.MoveDown()
		}
	}

	t.Run("returns nil/false if cursor is not in from, to or in between", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)
		abcList := LocationSlice(messageLocations)
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		_, ok := c.LocationsAtCursor()
		assert.False(t, ok)
	})

	t.Run("return messages if cursor is at From", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)
		abcList := LocationSlice(messageLocations)
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		require.True(t, c.MoveDown())

		assertMessages(t, c)
	})

	t.Run("return messages if cursor between From/To", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)

		abcList := LocationSlice(messageLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		require.True(t, c.MoveDown())
		require.True(t, c.MoveRight())

		assertMessages(t, c)
	})

	t.Run("return messages if cursor is at To", func(t *testing.T) {
		c := setupCursorContent(t, 10, 2, content, false)

		abcList := LocationSlice(messageLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		require.True(t, c.MoveDown())
		c.MoveRight()
		c.MoveRight()

		assertMessages(t, c)
	})
}

func TestCursorDrawLocationListsIntegration(t *testing.T) {
	t.Run("sets location list attrs", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		expected := [][]term.Cell{
			{{}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
			{{}},
		}

		abcList := LocationSlice(abcLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("clears location lists", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)
		abcList := LocationSlice(abcLocations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))
		assert.NotNil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, nil))

		expected := [][]term.Cell{
			{{}},
			{{}},
			{{}},
			{{}},
			{{}},
		}

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("lower priority lists do not override higher priority list attrs", func(t *testing.T) {
		infoLocations := []textapi.Location{
			{
				From: term.Coordinates{Y: 1},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorGreen},
			},
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 2, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorGreen},
			},
			{
				From: term.Coordinates{Y: 3},
				To:   term.Coordinates{Y: 3, X: 1},
				Attr: term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorGreen},
			},
		}
		criticalLocations := []textapi.Location{
			{
				From: term.Coordinates{Y: 1},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack},
			},
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 2, X: 1},
				Attr: term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack},
			},
			{
				From: term.Coordinates{Y: 3},
				To:   term.Coordinates{Y: 3, X: 1},
				Attr: term.Attributes{Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack},
			},
		}
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		expected := [][]term.Cell{
			{{}},
			{{Attributes: term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}}},
			{{Attributes: term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}}},
			{{Attributes: term.Attributes{Fg: tcell.ColorRed, Attrs: tcell.AttrUnderline, Bg: tcell.ColorBlack}}},
			{{}},
		}

		criticalList := LocationSlice(criticalLocations)
		infoList := LocationSlice(infoLocations)
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityCritical, "list1", criticalList))
		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, "list2", infoList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("trims to fit location To line if From is in bounds", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		expected := [][]term.Cell{
			{{}},
			{{}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
			{{Attributes: abcAttr}},
		}
		locations := []textapi.Location{
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 6, X: 1},
				Attr: abcAttr,
			},
		}

		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("does not panic if width == 0 in wrap mode", func(t *testing.T) {
		c := setupCursorContent(t, 0, 5, "\na\nb\nc\n", false)
		c.scroll.Wrap = true

		expected := [][]term.Cell{
			{{}},
			{{}},
			{{}},
			{{}},
			{{}},
		}
		locations := []textapi.Location{
			{
				From: term.Coordinates{Y: 0},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: abcAttr,
			},
		}

		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 1, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("sets full height worth of locations when there are hidden lines", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		require.True(t, c.scroll.MarkHidden(1, 3))

		locations := []textapi.Location{
			{
				From:    term.Coordinates{Y: 1},
				To:      term.Coordinates{Y: 1, X: 1},
				Attr:    term.Attributes{Bg: tcell.ColorRed},
				Message: "blabla",
			},
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 2, X: 1},
				Attr: term.Attributes{Bg: tcell.ColorOrange},
			},
			{
				From: term.Coordinates{Y: 3},
				To:   term.Coordinates{Y: 3, X: 1},
				Attr: term.Attributes{Bg: tcell.ColorYellow},
			},
		}

		expected := [][]term.Cell{
			{{}, {}},
			{{Attributes: term.Attributes{Bg: tcell.ColorRed}},
				{Attributes: term.Attributes{Bg: 0}}},
			{{}, {}},
			{{}, {}},
			{{}, {}},
		}
		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		w := cell.NewBufferWriter(context.Background(), 2, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("shows cue for messages hidden inside hidden lines", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		require.True(t, c.scroll.MarkHidden(1, 3))

		locations := []textapi.Location{
			{
				From:    term.Coordinates{Y: 2},
				To:      term.Coordinates{Y: 2, X: 1},
				Attr:    term.Attributes{Bg: tcell.ColorGreen},
				Message: "B HAS A MESSAGE FOR YOU",
			},
		}

		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		expected := [][]term.Cell{
			{{}, {}},
			{{Attributes: term.Attributes{Bg: tcell.ColorGreen}},
				{Attributes: term.Attributes{Bg: 0}}},
			{{}, {}},
			{{}, {}},
			{{}, {}},
		}

		w := cell.NewBufferWriter(context.Background(), 2, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})

	t.Run("does not render attrs past last rendered line", func(t *testing.T) {
		c := setupCursorContent(t, 1, 5, "\na\nb\nc\n", false)

		require.True(t, c.scroll.MarkHidden(1, 3))

		locations := []textapi.Location{
			{
				From: term.Coordinates{Y: 1},
				To:   term.Coordinates{Y: 1, X: 1},
				Attr: term.Attributes{Bg: tcell.ColorRed},
			},
			{
				From: term.Coordinates{Y: 2},
				To:   term.Coordinates{Y: 2, X: 1},
				Attr: term.Attributes{Bg: tcell.ColorOrange},
			},
			{
				From: term.Coordinates{Y: 3},
				To:   term.Coordinates{Y: 3, X: 1},
				Attr: term.Attributes{Bg: tcell.ColorYellow},
			},
		}

		expected := [][]term.Cell{
			{{}, {}},
			{{}, {}},
			{{}, {}},
			{{}, {}},
			{{}, {}},
		}
		abcList := LocationSlice(locations)

		assert.Nil(t, c.SetLocationList(textapi.LocationPriorityInfo, locID, abcList))

		c.scroll.Resize(1, 1)
		w := cell.NewBufferWriter(context.Background(), 2, 5)
		DrawLocations(c.SortedLocations(), c.scroll, w)
		assert.Equal(t, expected, w.RawCells())
	})
}
