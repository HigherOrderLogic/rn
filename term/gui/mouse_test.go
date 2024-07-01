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
package gui

import (
	"image"
	"testing"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/gui/font"
)

func TestMouseEvents(t *testing.T) {
	suite := []struct {
		description    string
		pressedButtons [][]ebiten.MouseButton
		cursorPosition []image.Point // in pixels
		wheel          []float64
		expectedEvents []term.Event
	}{
		{
			description: "does not dispatch if no mouse buttons are pressed or " +
				"wheel engaged, and position is the same",
			pressedButtons: [][]ebiten.MouseButton{{}, {}},
			cursorPosition: []image.Point{{}, {}},
			wheel:          []float64{0, 0},
			expectedEvents: []term.Event{{}, {}},
		},
		{
			description:    "does not dispatch if moved but outside of window on the x axis",
			pressedButtons: [][]ebiten.MouseButton{{}, {}},
			cursorPosition: []image.Point{{}, {X: defaultWidth}},
			wheel:          []float64{0, 0},
			expectedEvents: []term.Event{{}, {}},
		},
		{
			description:    "does not dispatch if moved but outside of window on the x axis (negative)",
			pressedButtons: [][]ebiten.MouseButton{{}, {}},
			cursorPosition: []image.Point{{}, {X: -defaultWidth}},
			wheel:          []float64{0, 0},
			expectedEvents: []term.Event{{}, {}},
		},
		{
			description:    "does not dispatch if moved but outside of window on the y axis",
			pressedButtons: [][]ebiten.MouseButton{{}, {}},
			cursorPosition: []image.Point{{}, {Y: defaultHeight}},
			wheel:          []float64{0, 0},
			expectedEvents: []term.Event{{}, {}},
		},
		{
			description:    "does not dispatch if moved but outside of window on the y axis (negative)",
			pressedButtons: [][]ebiten.MouseButton{{}, {}},
			cursorPosition: []image.Point{{}, {Y: -defaultHeight}},
			wheel:          []float64{0, 0},
			expectedEvents: []term.Event{{}, {}},
		},
		{
			description:    "dispatches mouse left click and subsequent release",
			pressedButtons: [][]ebiten.MouseButton{{ebiten.MouseButtonLeft}, {ebiten.MouseButtonLeft}, {}},
			cursorPosition: []image.Point{{}, {}, {}},
			wheel:          []float64{0, 0, 0},
			expectedEvents: []term.Event{
				{Type: term.EventMouse, Key: term.MouseLeft},
				{},
				{Type: term.EventMouse, Key: term.MouseRelease},
			},
		},
		{
			description:    "dispatches mouse right click and subsequent release",
			pressedButtons: [][]ebiten.MouseButton{{ebiten.MouseButtonRight}, {ebiten.MouseButtonRight}, {}},
			cursorPosition: []image.Point{{}, {}, {}},
			wheel:          []float64{0, 0, 0},
			expectedEvents: []term.Event{
				{Type: term.EventMouse, Key: term.MouseRight},
				{},
				{Type: term.EventMouse, Key: term.MouseRelease},
			},
		},
		{
			description:    "dispatches mouse middle click and subsequent release",
			pressedButtons: [][]ebiten.MouseButton{{ebiten.MouseButtonMiddle}, {ebiten.MouseButtonMiddle}, {}},
			cursorPosition: []image.Point{{}, {}, {}},
			wheel:          []float64{0, 0, 0},
			expectedEvents: []term.Event{
				{Type: term.EventMouse, Key: term.MouseMiddle},
				{},
				{Type: term.EventMouse, Key: term.MouseRelease},
			},
		},
		{
			description:    "dispatches mouse wheel up and down",
			pressedButtons: [][]ebiten.MouseButton{{}, {}, {}},
			cursorPosition: []image.Point{{}, {}, {}},
			wheel:          []float64{1, 0, -1},
			expectedEvents: []term.Event{
				{Type: term.EventMouse, Key: term.MouseWheelUp},
				{},
				{Type: term.EventMouse, Key: term.MouseWheelDown},
			},
		},
		{
			description:    "dispatches mouse cursor position changes",
			pressedButtons: [][]ebiten.MouseButton{{}, {}, {}},
			cursorPosition: []image.Point{{X: 100, Y: 100}, {}, {X: 100, Y: 100}},
			wheel:          []float64{0, 0, 0},
			expectedEvents: []term.Event{
				{Type: term.EventMouse, MouseX: 11, MouseY: 5},
				{Type: term.EventMouse},
				{Type: term.EventMouse, MouseX: 11, MouseY: 5},
			},
		},
		{
			description: "dispatches mouse left click, drag and release",
			pressedButtons: [][]ebiten.MouseButton{
				{ebiten.MouseButtonLeft},
				{ebiten.MouseButtonLeft},
				{ebiten.MouseButtonLeft},
				{},
			},
			cursorPosition: []image.Point{{}, {X: 100, Y: 100}, {X: 120, Y: 120}, {X: 120, Y: 120}},
			wheel:          []float64{0, 0, 0, 0},
			expectedEvents: []term.Event{
				{Type: term.EventMouse, Key: term.MouseLeft},
				{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 11, MouseY: 5},
				{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 13, MouseY: 7},
				{Type: term.EventMouse, Key: term.MouseRelease, MouseX: 13, MouseY: 7},
			},
		},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			mock, mouse := newTestMouse(t)
			if len(test.pressedButtons) != len(test.cursorPosition) || len(test.cursorPosition) != len(test.wheel) ||
				len(test.wheel) != len(test.expectedEvents) {
				t.Fatalf("incorrectly setup test case: pressedButtons, cursorPosition, wheel " +
					"and expectedEvents must be of the same length")
			}
			for i, buttons := range test.pressedButtons {
				mock.pressedButtons = make(map[ebiten.MouseButton]struct{})
				mock.wheel = test.wheel[i]
				mock.cursorPosition = test.cursorPosition[i]
				expectedEvent := test.expectedEvents[i]
				for _, button := range buttons {
					mock.pressedButtons[button] = struct{}{}
				}
				actualEvent, ok := mouse.processMouse()
				if expectedEvent.Type == 0 {
					assert.False(t, ok, i)
				} else {
					assert.True(t, ok, i)
				}
				assert.Equal(t, expectedEvent, actualEvent, i)
			}
		})
	}
}

func newTestMouse(t *testing.T) (*mockMouseManager, *mouse) {
	mock := &mockMouseManager{pressedButtons: map[ebiten.MouseButton]struct{}{}}
	f, err := font.NewManager()
	require.NoError(t, err)
	f.SetFontByFamilyName("builtin")
	f.SetDeviceScale(1)
	ret := newMouse(f)
	ret.mouse = mock
	ret.resize(f.CellsWidth(defaultWidth), f.CellsHeight(defaultHeight))
	return mock, ret
}

type mockMouseManager struct {
	pressedButtons map[ebiten.MouseButton]struct{}
	wheel          float64
	cursorPosition image.Point
}

func (m *mockMouseManager) IsMouseButtonPressed(key ebiten.MouseButton) bool {
	_, ok := m.pressedButtons[key]
	return ok
}

func (m *mockMouseManager) Wheel() (float64, float64) {
	return 0, m.wheel
}

func (m *mockMouseManager) CursorPosition() (int, int) {
	return m.cursorPosition.X, m.cursorPosition.Y
}
