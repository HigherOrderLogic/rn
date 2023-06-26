package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
	testutil "unstable.build/go-tui/util/test"
)

type responsiveTestList struct {
	elementHeight int
	*ResponsiveList
}

type testResponsive struct {
	tui.Component
	height *int
}

func (t *testResponsive) Height(width int) int {
	return *t.height
}

func (l *responsiveTestList) newTestResponsive(c tui.Component) Responsive {
	// emulate List behaviour, and test max width default
	v := &testResponsive{Component: c, height: &l.elementHeight}
	return v
}

func (l *responsiveTestList) PushBackList(other testList) {
	l.ResponsiveList.PushBackList(other.(*responsiveTestList).ResponsiveList)
}
func (l *responsiveTestList) PushFrontList(other testList) {
	l.ResponsiveList.PushFrontList(other.(*responsiveTestList).ResponsiveList)
}

func (l *responsiveTestList) PushBack(c tui.Component) ListNode {
	return l.ResponsiveList.PushBack(l.newTestResponsive(c))
}

func (l *responsiveTestList) PushFront(c tui.Component) ListNode {
	return l.ResponsiveList.PushFront(l.newTestResponsive(c))
}

func (l *responsiveTestList) Remove(e ListNode) tui.Component {
	return l.ResponsiveList.Remove(e).(*testResponsive).Component
}

func (l *responsiveTestList) Sort(less func(a, b tui.Component) bool) {
	l.ResponsiveList.Sort(func(a, b Responsive) bool {
		return less(a.(*testResponsive).Component, b.(*testResponsive).Component)
	})
}

func (l *responsiveTestList) SetElementHeight(i int) {
	// setting element height from here bypassing ResponsiveList
	// allows for all test Responsive components to be resized
	// and emulate SetElementHeight behaviour.
	l.elementHeight = i
	l.Resize(l.width, l.height)
}

func newResponsiveTestList(i int) testList {
	ret := &responsiveTestList{ResponsiveList: NewResponsiveList()}
	ret.elementHeight = i
	return ret
}

func TestResponsiveListDraw(t *testing.T) {
	testListDraw(t, []int{13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}, newResponsiveTestList)
}

func TestResponsiveListSort(t *testing.T) {
	testListSort(t, newResponsiveTestList)
}

func TestResponsiveListFrontBack(t *testing.T) {
	testFrontBack(t, newResponsiveTestList)
}

func TestResponsiveListListRemove(t *testing.T) {
	testListRemove(t, newResponsiveTestList)
}

func TestResponsiveListEmptyDraw(t *testing.T) {
	testEmptyListDraw(t, newResponsiveTestList)
}

func TestResponsiveListResponsiveness(t *testing.T) {
	l := NewResponsiveList()
	l.Resize(8, 4)

	w := term.NewStringWriter(8, 4)

	tests := []testutil.ComponentTestCase{
		{
			nil, `
        
        
        
        `,
		}, {
			func() {
				l.PushBack(&TestResponsive{
					WantHeight: 2,
					TestComponent: TestComponent{
						Ch: 'X',
					},
				},
				)
			}, `
XXXXXXXX
XXXXXXXX
        
        `,
		}, {
			func() {
				l.PushBack(&TestResponsive{
					WantHeight: 2,
					TestComponent: TestComponent{
						Ch: 'Y',
					},
				},
				)
			}, `
XXXXXXXX
XXXXXXXX
YYYYYYYY
YYYYYYYY`,
		}, {
			func() {
				l.PushBack(&TestResponsive{
					WantHeight: 3,
					TestComponent: TestComponent{
						Ch: 'Z',
					},
				},
				)
				l.SeekEnd()
			}, `
YYYYYYYY
ZZZZZZZZ
ZZZZZZZZ
ZZZZZZZZ`,
		}, {
			func() {
				el, ok := l.ElementAt(term.Coordinates{})
				require.True(t, ok)
				v := el.Value().(*TestResponsive)
				assert.Equal(t, 'Y', v.Ch)

				el, ok = l.ElementAt(term.Coordinates{Y: 1})
				require.True(t, ok)
				v = el.Value().(*TestResponsive)
				assert.Equal(t, 'Z', v.Ch)

				el, ok = l.ElementAt(term.Coordinates{Y: 3})
				require.True(t, ok)
				v = el.Value().(*TestResponsive)
				assert.Equal(t, 'Z', v.Ch)
			}, `
YYYYYYYY
ZZZZZZZZ
ZZZZZZZZ
ZZZZZZZZ`,
		},
	}

	testutil.TestComponent(t, l, w, tests)
}

func TestResponsiveListResizeLarger(t *testing.T) {
	l := NewResponsiveList()

	w := term.NewStringWriter(8, 4)

	tests := []testutil.ComponentTestCase{
		{
			func() {
				l.PushBack(&TestResponsive{
					WantHeight: 2,
					TestComponent: TestComponent{
						Ch: 'X',
					},
				},
				)
				l.PushBack(&TestResponsive{
					WantHeight: 2,
					TestComponent: TestComponent{
						Ch: 'Y',
					},
				},
				)
				l.PushBack(&TestResponsive{
					WantHeight: 3,
					TestComponent: TestComponent{
						Ch: 'Z',
					},
				},
				)
				assert.True(t, l.SeekEnd())
				l.Resize(8, 4)
				assert.True(t, l.SeekEnd())
			}, `
YYYYYYYY
ZZZZZZZZ
ZZZZZZZZ
ZZZZZZZZ`,
		}, {
			func() {
				el, ok := l.ElementAt(term.Coordinates{})
				require.True(t, ok)
				v := el.Value().(*TestResponsive)
				assert.Equal(t, 'Y', v.Ch)

				el, ok = l.ElementAt(term.Coordinates{Y: 1})
				require.True(t, ok)
				v = el.Value().(*TestResponsive)
				assert.Equal(t, 'Z', v.Ch)

				el, ok = l.ElementAt(term.Coordinates{Y: 3})
				require.True(t, ok)
				v = el.Value().(*TestResponsive)
				assert.Equal(t, 'Z', v.Ch)
			}, `
YYYYYYYY
ZZZZZZZZ
ZZZZZZZZ
ZZZZZZZZ`,
		},
	}

	testutil.TestComponent(t, l, w, tests)
}
