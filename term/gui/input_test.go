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
	"testing"
	"time"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/gui/font"
)

func TestInputFireOnce(t *testing.T) {
	suite := []struct {
		description   string
		pressedKeys   []ebiten.Key
		pressedChars  []rune
		expectedEvent term.Event
	}{
		{
			description:   "dispatches a single non-char key",
			pressedKeys:   []ebiten.Key{ebiten.KeyEnter},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Key: term.KeyEnter, Raw: []byte{0x0d, 0x0a}},
		},
		{
			description:   "dispatches a single key char, via key",
			pressedKeys:   []ebiten.Key{ebiten.KeyA},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Ch: 'a', Raw: []byte("a")},
		},
		{
			description:   "dispatches a single key char, via char",
			pressedKeys:   []ebiten.Key{},
			pressedChars:  []rune{'a'},
			expectedEvent: term.Event{Type: term.EventKey, Ch: 'a', Raw: []byte("a")},
		},
		{
			description:   "dispatches a space key",
			pressedKeys:   []ebiten.Key{ebiten.KeySpace},
			pressedChars:  []rune{' '},
			expectedEvent: term.Event{Type: term.EventKey, Key: term.KeySpace, Raw: []byte(" ")},
		},
		{
			description:   "dispatches a shift+space like a space key",
			pressedKeys:   []ebiten.Key{ebiten.KeySpace, ebiten.KeyShift},
			pressedChars:  []rune{' '},
			expectedEvent: term.Event{Type: term.EventKey, Key: term.KeySpace, Raw: []byte(" ")},
		},
		{
			// this tests the (rest of) modifiers
			description:   "dispatches a meta+space as meta+space key",
			pressedKeys:   []ebiten.Key{ebiten.KeySpace, ebiten.KeyShift},
			pressedChars:  []rune{' '},
			expectedEvent: term.Event{Type: term.EventKey, Key: term.KeySpace, Raw: []byte(" ")},
		},
		{
			description:   "dispatches a single key ctrl + char",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyControl},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'a', Raw: []byte{0x01}},
		},
		{
			description:   "dispatches a single key shift + ctrl + char",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyShift, ebiten.KeyControl},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'A', Raw: []byte{0x01}},
		},
		{
			description:   "dispatches a single key meta + char",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyMeta},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModMeta, Ch: 'a'},
		},
		{
			description:   "dispatches a single key alt + char, via char",
			pressedKeys:   []ebiten.Key{ebiten.KeyAlt},
			pressedChars:  []rune{'a'},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAlt, Ch: 'a', Raw: []byte{0x1b, 'a'}},
		},
		{
			description:   "dispatches a single key alt + char, via key",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyAlt},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAlt, Ch: 'a', Raw: []byte{0x1b, 'a'}},
		},
		{
			description:   "dispatches a single key alt + char, undoes macos special chars",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyAlt, ebiten.KeyAltLeft},
			pressedChars:  []rune{'å'},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAlt, Ch: 'a', Raw: []byte{0x1b, 'a'}},
		},
		{
			description:   "omits shift modifier for shift + char, via char",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyShift},
			pressedChars:  []rune{'A'},
			expectedEvent: term.Event{Type: term.EventKey, Ch: 'A', Raw: []byte("A")},
		},
		{
			description:   "omits shift modifier for shift + char, via key",
			pressedKeys:   []ebiten.Key{ebiten.KeyA, ebiten.KeyShift},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Ch: 'A', Raw: []byte("A")},
		},
		{
			description:  "dispatches a single key ctrl + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrl,
				Key: term.KeyF1, Raw: []byte{0x1b, 0x5b, 0x31, 0x3b, 0x35, 0x50}},
		},
		{
			description:  "dispatches a single key meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyMeta},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModMeta,
				Key: term.KeyF1, Raw: []byte{0x1b, 0x5b, 0x31, 0x3b, 0x39, 0x50}},
		},
		{
			description:  "dispatches a single key alt + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyAlt},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAlt,
				Key: term.KeyF1, Raw: []byte{0x1b, 0x5b, 0x31, 0x3b, 0x33, 0x50}},
		},
		{
			description:  "unhandled key dispatches no event",
			pressedKeys:  []ebiten.Key{ebiten.KeyF24},
			pressedChars: []rune{},
		},
		{
			description:  "dispatches a single key ctrl + alt + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl, ebiten.KeyAlt},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlAlt,
				Key: term.KeyF1, Raw: []byte("\x1b[1;7P")},
		},
		{
			description:  "dispatches a single key ctrl + meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl, ebiten.KeyMeta},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlMeta,
				Key: term.KeyF1, Raw: []byte("\x1b[1;13P")},
		},
		{
			description:  "dispatches a single key ctrl + shift + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl, ebiten.KeyShift},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlShift,
				Key: term.KeyF1, Raw: []byte("\x1b[1;6P")},
		},
		{
			description:  "dispatches a single key ctrl + alt + shift + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl, ebiten.KeyAlt, ebiten.KeyShift},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlShiftAlt,
				Key: term.KeyF1, Raw: []byte("\x1b[1;8P")},
		},
		{
			description:  "dispatches a single key ctrl + shift + meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl, ebiten.KeyMeta, ebiten.KeyShift},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlShiftMeta,
				Key: term.KeyF1, Raw: []byte("\x1b[1;14P")},
		},
		{
			description:  "dispatches a single key ctrl + alt + meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl, ebiten.KeyMeta, ebiten.KeyAlt},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlAltMeta,
				Key: term.KeyF1, Raw: []byte("\x1b[1;15P")},
		},
		{
			description:  "dispatches a single key shift + meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyShift, ebiten.KeyMeta},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModShiftMeta,
				Key: term.KeyF1, Raw: []byte("\x1b[1;10P")},
		},
		{
			description:  "dispatches a single key alt + meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyAlt, ebiten.KeyMeta},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAltMeta,
				Key: term.KeyF1, Raw: []byte("\x1b[1;11P")},
		},
		{
			description:  "dispatches a single key alt + shift + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyAlt, ebiten.KeyShift},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAltShift,
				Key: term.KeyF1, Raw: []byte("\x1b[1;4P")},
		},
		{
			description:  "dispatches a single key alt + shift + meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyAlt, ebiten.KeyShift, ebiten.KeyMeta},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAltShiftMeta,
				Key: term.KeyF1, Raw: []byte("\x1b[1;12P")},
		},
		{
			description:   "does not dispatch event with raw for meta + enter",
			pressedKeys:   []ebiten.Key{ebiten.KeyEnter, ebiten.KeyMeta},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModMeta, Key: term.KeyEnter},
		},
		{
			description:  "dispatches modifier ctrl + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyControl},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrl,
				Key: term.KeyF1, Raw: []byte{0x1b, 0x5b, 0x31, 0x3b, 0x35, 0x50}},
		},
		{
			description:  "dispatches modifier meta + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyMeta},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModMeta,
				Key: term.KeyF1, Raw: []byte{0x1b, 0x5b, 0x31, 0x3b, 0x39, 0x50}},
		},
		{
			description:  "dispatches modifier alt + key",
			pressedKeys:  []ebiten.Key{ebiten.KeyF1, ebiten.KeyAlt},
			pressedChars: []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAlt,
				Key: term.KeyF1, Raw: []byte{0x1b, 0x5b, 0x31, 0x3b, 0x33, 0x50}},
		},
		{
			description:  "unhandled key dispatches no event",
			pressedKeys:  []ebiten.Key{ebiten.KeyF24},
			pressedChars: []rune{},
		},
		{
			description:   "dispatches modifier ctrl + alt + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyControl, ebiten.KeyAlt},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlAlt},
		},
		{
			description:   "dispatches modifier ctrl + meta + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyControl, ebiten.KeyMeta},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlMeta},
		},
		{
			description:   "dispatches modifier ctrl + shift + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyControl, ebiten.KeyShift},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrl},
		},
		{
			description:   "dispatches modifier ctrl + alt + shift + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyControl, ebiten.KeyAlt, ebiten.KeyShift},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlAlt},
		},
		{
			description:   "dispatches modifier ctrl + shift + meta + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyControl, ebiten.KeyMeta, ebiten.KeyShift},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlMeta},
		},
		{
			description:   "dispatches modifier ctrl + alt + meta + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyControl, ebiten.KeyMeta, ebiten.KeyAlt},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModCtrlAltMeta},
		},
		{
			description:   "dispatches modifier shift + meta + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyShift, ebiten.KeyMeta},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModMeta},
		},
		{
			description:   "dispatches modifier alt + meta + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyAlt, ebiten.KeyMeta},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAltMeta},
		},
		{
			description:   "dispatches modifier alt + shift + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyAlt, ebiten.KeyShift},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAlt},
		},
		{
			description:   "dispatches modifier alt + shift + meta + key",
			pressedKeys:   []ebiten.Key{ebiten.KeyAlt, ebiten.KeyShift, ebiten.KeyMeta},
			pressedChars:  []rune{},
			expectedEvent: term.Event{Type: term.EventKey, Mod: term.ModAltMeta},
		},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			mock, input := newTestInput(t)
			for _, key := range test.pressedKeys {
				mock.pressedKeys[key] = struct{}{}
			}
			mock.pressedChars = test.pressedChars

			ev, ok := input.processEvents()
			if test.expectedEvent.Type == 0 {
				require.False(t, ok)
			} else {
				require.True(t, ok)
			}
			assert.Equal(t, test.expectedEvent, ev)
		})
	}
}

func TestInputFireDelay(t *testing.T) {
	suite := []struct {
		description    string
		pressedKeys    [][]ebiten.Key
		pressedChars   [][]rune
		expectedEvents []term.Event
	}{
		{
			description:    "multiple unhandled key dispatches no events",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyF24}, {ebiten.KeyF24}},
			pressedChars:   [][]rune{{}, {}},
			expectedEvents: []term.Event{{}, {}},
		},
		{
			description:    "dispatches once a repeated non-char key, only once",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyEnter}, {ebiten.KeyEnter}},
			pressedChars:   [][]rune{{}, {}},
			expectedEvents: []term.Event{{Key: term.KeyEnter}, {}},
		},
		{
			description: "dispatches once a repeated non-char key, " +
				"different key dismisses dispatches new event",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyEnter}, {ebiten.KeyEnter}, {ebiten.KeySpace}},
			pressedChars:   [][]rune{{}, {}, {}},
			expectedEvents: []term.Event{{Key: term.KeyEnter}, {}, {Key: term.KeySpace}},
		},
		{
			description:    "dispatches once a repeated key char, via key, only once",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyA}, {ebiten.KeyA}},
			pressedChars:   [][]rune{{}, {}},
			expectedEvents: []term.Event{{Ch: 'a'}, {}},
		},
		{
			description: "dispatches once a repeated key char, via key, " +
				"different char key dispatches new event",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyA}, {ebiten.KeyA}, {ebiten.KeyB}},
			pressedChars:   [][]rune{{}, {}, {}},
			expectedEvents: []term.Event{{Ch: 'a'}, {}, {Ch: 'b'}},
		},
		{
			description: "dispatches once a repeated key char, via char, " +
				"different char dispatches new event",
			pressedKeys:    [][]ebiten.Key{{}, {}, {}, {}},
			pressedChars:   [][]rune{{'a'}, {'b'}, {'a'}, {'b'}},
			expectedEvents: []term.Event{{Ch: 'a'}, {Ch: 'b'}, {Ch: 'a'}, {Ch: 'b'}},
		},
		{
			description: "dispatches once a repeated key ctrl + char",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyA, ebiten.KeyControl},
				{ebiten.KeyA, ebiten.KeyControl},
			},
			pressedChars:   [][]rune{{}, {}},
			expectedEvents: []term.Event{{Mod: term.ModCtrl, Ch: 'a'}, {}},
		},
		{
			description: "alternating modifiers",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyControl},
				{ebiten.KeyMeta},
				{ebiten.KeyControl},
				{ebiten.KeyMeta},
			},
			pressedChars: [][]rune{{}, {}, {}, {}},
			expectedEvents: []term.Event{
				{Mod: term.ModCtrl}, {Mod: term.ModMeta},
				{Mod: term.ModCtrl}, {Mod: term.ModMeta},
			},
		},
		{
			description: "dispatches once a repeated key ctrl + char, a " +
				"different ctrl+char dispatches a new event",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyA, ebiten.KeyControl},
				{ebiten.KeyA, ebiten.KeyControl},
				{ebiten.KeyB, ebiten.KeyControl},
			},
			pressedChars:   [][]rune{{}, {}, {}},
			expectedEvents: []term.Event{{Mod: term.ModCtrl, Ch: 'a'}, {}, {Mod: term.ModCtrl, Ch: 'b'}},
		},
		{
			description: "dispatches once a repeated key shift + ctrl + char",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyA, ebiten.KeyShift, ebiten.KeyControl},
				{ebiten.KeyA, ebiten.KeyShift, ebiten.KeyControl},
			},
			pressedChars:   [][]rune{{}, {}},
			expectedEvents: []term.Event{{Mod: term.ModCtrl, Ch: 'A'}, {}},
		},
		{
			description: "dispatches once a repeated key shift + ctrl + char," +
				" a new char key dispatches a new event",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyA, ebiten.KeyShift, ebiten.KeyControl},
				{ebiten.KeyA, ebiten.KeyShift, ebiten.KeyControl},
				{ebiten.KeyB, ebiten.KeyShift, ebiten.KeyControl},
			},
			pressedChars:   [][]rune{{}, {}, {}},
			expectedEvents: []term.Event{{Mod: term.ModCtrl, Ch: 'A'}, {}, {Mod: term.ModCtrl, Ch: 'B'}},
		},
		{
			description: "dispatches once a repeated key alt + char, first via" +
				" lower case char, then via key",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyAlt}, {ebiten.KeyAlt, ebiten.KeyA}},
			pressedChars:   [][]rune{{'a'}, {}},
			expectedEvents: []term.Event{{Mod: term.ModAlt, Ch: 'a'}, {}},
		},
		{
			description: "dispatches once a repeated key alt + char, first via key," +
				" then via lower case char",
			pressedKeys:    [][]ebiten.Key{{ebiten.KeyAlt, ebiten.KeyA}, {ebiten.KeyAlt}},
			pressedChars:   [][]rune{{}, {'a'}},
			expectedEvents: []term.Event{{Mod: term.ModAlt, Ch: 'a'}, {}},
		},
		{
			description: "dispatches once a repeated key alt + char, first via upper case char," +
				" then via shift + key",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyAlt},
				{ebiten.KeyAlt, ebiten.KeyA, ebiten.KeyShift},
			},
			pressedChars:   [][]rune{{'A'}, {}},
			expectedEvents: []term.Event{{Mod: term.ModAlt, Ch: 'A'}, {}},
		},
		{
			description: "dispatches once a repeated key alt + char, first via shift + key, " +
				"then via upper case char",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyAlt, ebiten.KeyA, ebiten.KeyShift},
				{ebiten.KeyAlt},
			},
			pressedChars:   [][]rune{{}, {'A'}},
			expectedEvents: []term.Event{{Mod: term.ModAlt, Ch: 'A'}, {}},
		},
		{
			description: "dispatches only new keys when joining keys together",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyA},
				{ebiten.KeyA},
				{ebiten.KeyA},
				{ebiten.KeyA, ebiten.KeyB},
				{ebiten.KeyA, ebiten.KeyB},
				{ebiten.KeyB},
				{ebiten.KeyB},
				{ebiten.KeyC, ebiten.KeyB},
				{ebiten.KeyC},
				{ebiten.KeyC},
			},
			pressedChars:   [][]rune{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
			expectedEvents: []term.Event{{Ch: 'a'}, {}, {}, {Ch: 'b'}, {}, {}, {}, {Ch: 'c'}, {}, {}},
		},
		{
			description: "dispatches only new keys when joining keys with modifiers together",
			pressedKeys: [][]ebiten.Key{
				{ebiten.KeyShift},
				{ebiten.KeyShift},
				{ebiten.KeyA, ebiten.KeyShift},
				{ebiten.KeyA, ebiten.KeyB, ebiten.KeyShift},
				{ebiten.KeyShift, ebiten.KeyB},
				{ebiten.KeyB, ebiten.KeyShift, ebiten.KeyControl},
				{ebiten.KeyB, ebiten.KeyShift, ebiten.KeyControl},
				{ebiten.KeyC, ebiten.KeyShift, ebiten.KeyControl},
				{ebiten.KeyControl, ebiten.KeyShift},
				{ebiten.KeyControl},
			},
			pressedChars: [][]rune{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
			expectedEvents: []term.Event{
				{}, {}, {Ch: 'A'},
				{Ch: 'B'}, {}, {Mod: term.ModCtrl}, {}, {Mod: term.ModCtrl, Ch: 'C'},
				{}, {},
			},
		},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			mock, input := newTestInput(t)
			if len(test.pressedKeys) != len(test.pressedChars) || len(test.pressedChars) != len(test.expectedEvents) {
				t.Fatalf("incorrectly setup test case: pressedChars, pressedKeys " +
					"and expectedEvents must be of the same length")
			}
			input.keyPressDelay = 4 * time.Second
			input.keyPressRepeat = 4 * time.Second
			for i, keys := range test.pressedKeys {
				mock.pressedKeys = make(map[ebiten.Key]struct{})
				pressedChars := test.pressedChars[i]
				expectedEvent := test.expectedEvents[i]
				for _, key := range keys {
					mock.pressedKeys[key] = struct{}{}
				}
				mock.pressedChars = pressedChars

				actualEvent, ok := input.processEvents()
				if expectedEvent.Key == 0 && expectedEvent.Ch == 0 && expectedEvent.Mod == 0 {
					assert.False(t, ok, i)
				} else {
					assert.True(t, ok, i)
				}
				// Note this test omits asserting Raw and Type.
				// These fields should be tested in TestInputFireOnce
				actualEvent.Raw = nil
				actualEvent.Type = 0
				assert.Equal(t, expectedEvent, actualEvent, i)
			}
		})
	}
}

func TestInputFireRepeatKey(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyA, ebiten.KeyMeta} {
		mock, input := newTestInput(t)
		input.keyPressDelay = 1 * time.Second
		input.keyPressRepeat = 500 * time.Millisecond

		mock.pressedKeys[key] = struct{}{}
		actualEvent, ok := input.processEvents()
		require.True(t, ok)
		assert.NotZero(t, actualEvent)

		_, ok = input.processEvents()
		require.False(t, ok)
		_, ok = input.processEvents()
		require.False(t, ok)
		_, ok = input.processEvents()
		require.False(t, ok)

		time.Sleep(time.Duration(input.keyPressDelay) + 1)

		actualEvent, ok = input.processEvents()
		require.True(t, ok)
		assert.NotZero(t, actualEvent)

		_, ok = input.processEvents()
		require.False(t, ok)

		time.Sleep(time.Duration(input.keyPressRepeat) + 1)

		actualEvent, ok = input.processEvents()
		require.True(t, ok)
		assert.NotZero(t, actualEvent)
	}
}

func newTestInput(t *testing.T) (*mockInputManager, *input) {
	mock := &mockInputManager{pressedKeys: map[ebiten.Key]struct{}{}}
	f, err := font.NewManager()
	require.NoError(t, err)
	f.SetFontByFamilyName("builtin")
	f.SetDeviceScale(1)
	ret := newInput(f)
	ret.input = mock
	return mock, ret
}

type mockInputManager struct {
	pressedKeys  map[ebiten.Key]struct{}
	pressedChars []rune
	now          *time.Time
}

func (m *mockInputManager) IsKeyPressed(key ebiten.Key) bool {
	_, ok := m.pressedKeys[key]
	return ok
}

func (m *mockInputManager) AppendInputChars([]rune) []rune {
	return m.pressedChars
}

func (m *mockInputManager) Now() time.Time {
	if m.now == nil {
		return time.Now()
	}
	return *m.now
}
