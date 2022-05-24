package text

import (
	"testing"

	"github.com/ernestrc/go-tui/term"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func evMouseKey(x, y int, key term.Key) term.Event {
	return term.Event{Type: term.EventMouse, MouseX: x, MouseY: y, Key: key}
}

func expectScrollUp(t *testing.T, n int) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().ScrollUp(gomock.Any()).
			DoAndReturn(func(m int) bool {
				assert.Equal(t, n, m)
				return true
			})
	}
}

func expectScrollDown(t *testing.T, n int) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().ScrollDown(gomock.Any()).
			DoAndReturn(func(m int) bool {
				assert.Equal(t, n, m)
				return true
			})
	}
}

func expectSetSelectionStart(t *testing.T, x, y int) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().SetSelectionStart(gomock.Any()).
			DoAndReturn(func(pos term.Coordinates) {
				assert.Equal(t, term.Coordinates{X: x, Y: y}, pos)
			})
	}
}

func expectClearSelection(t *testing.T) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().ClearSelection()
	}
}

func expectSetSelectionEnd(t *testing.T, x, y int) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().SetSelectionEnd(gomock.Any()).
			DoAndReturn(func(pos term.Coordinates) {
				assert.Equal(t, term.Coordinates{X: x, Y: y}, pos)
			})
	}
}

func expectSelectWord(t *testing.T, x, y int) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().SelectWordAt(gomock.Any()).
			DoAndReturn(func(pos term.Coordinates) {
				assert.Equal(t, term.Coordinates{X: x, Y: y}, pos)
			})
	}
}

func expectSelectLine(t *testing.T, y int) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().SelectLine(gomock.Any()).
			DoAndReturn(func(_y int) {
				assert.Equal(t, y, _y)
			})
	}
}

func expectIgnoreMouseAction() expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().OnAction(gomock.Any(), gomock.Any()).Return(false)
	}
}

func expectMouseAction(t *testing.T, x, y int, action MouseAction, handled, moved bool) expect {
	return func(m *MockMouseDelegate) {
		m.EXPECT().OnAction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(pos term.Coordinates, _action MouseAction) bool {
				assert.Equal(t, term.Coordinates{X: x, Y: y}, pos)
				assert.Equal(t, action, _action)
				return handled
			})
	}
}

type expect func(*MockMouseDelegate)

func multiExpect(e ...expect) expect {
	return func(m *MockMouseDelegate) {
		for _, expect := range e {
			expect(m)
		}
	}
}

func TestMouseHandle(t *testing.T) {
	tsuite := []struct {
		desc   string
		expect []expect
		evs    []term.Event
	}{
		{"ignores non-mouse events",
			nil, []term.Event{{Type: term.EventKey}}},
		{"delegates mouse release",
			[]expect{
				expectMouseAction(t, 4, 8, MouseLeftClick, true, false),
				expectMouseAction(t, 4, 8, MouseRelease, true, false),
			},
			[]term.Event{
				evMouseKey(4, 8, term.MouseLeft),
				evMouseKey(4, 8, term.MouseRelease),
			},
		},
		{"delegates mouse left click",
			[]expect{
				expectMouseAction(t, 4, 8, MouseLeftClick, true, false),
			},
			[]term.Event{evMouseKey(4, 8, term.MouseLeft)}},
		{"delegates mouse right click",
			[]expect{
				expectMouseAction(t, 4, 8, MouseRightClick, true, false),
			},
			[]term.Event{evMouseKey(4, 8, term.MouseRight)}},
		{"delegates mouse middle click",
			[]expect{
				expectMouseAction(t, 4, 8, MouseMiddleClick, true, false),
			},
			[]term.Event{evMouseKey(4, 8, term.MouseMiddle)}},
		{"scrolls up",
			[]expect{
				multiExpect(
					expectIgnoreMouseAction(),
					expectScrollUp(t, 5),
				),
			},
			[]term.Event{evMouseKey(4, 8, term.MouseWheelUp)}},
		{"scrolls down",
			[]expect{
				multiExpect(
					expectIgnoreMouseAction(),
					expectScrollDown(t, 5),
				),
			},
			[]term.Event{evMouseKey(4, 8, term.MouseWheelDown)}},
		{"click drag sets selection",
			[]expect{
				multiExpect(
					expectMouseAction(t, 1, 2, MouseLeftClick, false, true),
					expectClearSelection(t),
					expectSetSelectionStart(t, 1, 2),
				),
				multiExpect(
					expectScrollDown(t, 3),
					expectSetSelectionEnd(t, 4, 8),
				),
				multiExpect(
					expectScrollDown(t, 3),
					expectSetSelectionEnd(t, 4, 9),
				),
				multiExpect(
					expectSetSelectionEnd(t, 2, 4),
				),
				multiExpect(
					expectIgnoreMouseAction(),
				),
			},
			[]term.Event{
				evMouseKey(1, 2, term.MouseLeft),
				evMouseKey(4, 8, term.MouseLeft),
				evMouseKey(4, 9, term.MouseLeft),
				evMouseKey(2, 4, term.MouseLeft),
				evMouseKey(2, 5, term.MouseRelease),
			},
		},
		{"double click selects word",
			[]expect{
				multiExpect(
					expectIgnoreMouseAction(),
					expectClearSelection(t),
					expectSetSelectionStart(t, 1, 2),
				),
				expectIgnoreMouseAction(),
				multiExpect(
					expectIgnoreMouseAction(),
					expectSelectWord(t, 1, 2),
				),
				expectIgnoreMouseAction(),
			},
			[]term.Event{
				evMouseKey(1, 2, term.MouseLeft),
				evMouseKey(1, 2, term.MouseRelease),
				evMouseKey(1, 2, term.MouseLeft),
				evMouseKey(1, 2, term.MouseRelease),
			},
		},
		{"triple click selects line",
			[]expect{
				multiExpect(
					expectIgnoreMouseAction(),
					expectClearSelection(t),
					expectSetSelectionStart(t, 1, 2),
				),
				expectIgnoreMouseAction(),
				multiExpect(
					expectIgnoreMouseAction(),
					expectSelectWord(t, 1, 2),
				),
				expectIgnoreMouseAction(),
				multiExpect(
					expectIgnoreMouseAction(),
					expectSelectLine(t, 2),
				),
				expectIgnoreMouseAction(),
			},
			[]term.Event{
				evMouseKey(1, 2, term.MouseLeft),
				evMouseKey(1, 2, term.MouseRelease),
				evMouseKey(1, 2, term.MouseLeft),
				evMouseKey(1, 2, term.MouseRelease),
				evMouseKey(1, 2, term.MouseLeft),
				evMouseKey(1, 2, term.MouseRelease),
			},
		},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := NewMockMouseDelegate(ctrl)
			m := NewMouse(mock)
			mock.EXPECT().Width().Return(10).AnyTimes()
			mock.EXPECT().Height().Return(10).AnyTimes()

			for i, ev := range tcase.evs {
				// sut
				if i < len(tcase.expect) && tcase.expect[i] != nil {
					tcase.expect[i](mock)
				}
				_, _ = m.Handle(ev)
			}
		})
	}
}
