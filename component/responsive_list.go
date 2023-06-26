package component

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

// ResponsiveList differs from List in that it enables clients to
// set the min width and height of the children components by using a push
// API with Responsive components, rather than tui.Components.
type ResponsiveList struct {
	// we override only the relevant methods:
	//   - methods that use elementHeight
	//   - methods that manipulate children components
	List

	drawn int // used to keep track of how many elements are drawn

	loffset, ypos int // used to mark a draw offset when List's offset is last
}

// NewList allocates storage for a new ResponsiveList and initializes it.
func NewResponsiveList() (l *ResponsiveList) {
	l = new(ResponsiveList)
	l.Init()
	return
}

// Init initializes this list. It can be used to reset its internal state.
func (l *ResponsiveList) Init() {
	l.Reset()
	l.List.Init(1)
}

// Reset resets the contents of this List.
func (l *ResponsiveList) Reset() {
	l.List.Reset()
	l.drawn = 0
	l.loffset = 0
}

// PushBackList inserts a copy of an other list at the back of list l. The
// lists l and other must NOT be the same or nil.
func (l *ResponsiveList) PushBackList(other *ResponsiveList) {
	if l == other {
		panic("other list cannot be self: components can't be deep cloned")
	}
	other.Iterate(func(c Responsive) {
		l.PushBack(c)
	})
}

// PushFrontList inserts a copy of an other list at the front of list l. The
// lists l and other must NOT be the same or nil.
func (l *ResponsiveList) PushFrontList(other *ResponsiveList) {
	if l == other {
		panic("other list cannot be self: components can't be deep cloned")
	}
	for node, ok := other.Back(); ok; node, ok = node.Prev() {
		l.PushFront(node.Value().(Responsive))
	}
}

// InsertAfter inserts a new element c immediately after mark and
// returns the linked node. If mark is not an element of l, the list
// is not modified.
func (l *ResponsiveList) InsertAfter(c Responsive, mark ListNode) ListNode {
	defer l.setDrawOffset()
	defer l.simulateDraw()
	return l.List.InsertAfter(c, mark)
}

// InsertBefore inserts a new element c immediately before mark
// and returns the linked node. If mark is not an element of l,
// the list is not modified.
func (l *ResponsiveList) InsertBefore(c Responsive, mark ListNode) ListNode {
	defer l.setDrawOffset()
	defer l.simulateDraw()
	return l.List.InsertBefore(c, mark)
}

// PushBack inserts a new element c at the back of list l and
// returns the linked node.
func (l *ResponsiveList) PushBack(c Responsive) ListNode {
	defer l.setDrawOffset()
	defer l.simulateDraw()
	return l.List.PushBack(c)
}

// PushFront inserts a new element c at the front of list l and
// returns the linked node.
func (l *ResponsiveList) PushFront(c Responsive) ListNode {
	defer l.setDrawOffset()
	defer l.simulateDraw()
	return l.List.PushFront(c)
}

// Remove removes e from l if e is a node of list l. It returns the element
// value e.Value.
func (l *ResponsiveList) Remove(e ListNode) Responsive {
	defer l.setDrawOffset()
	defer l.simulateDraw()
	return l.List.Remove(e).(Responsive)
}

// CanSeekDown returns whether SeekDown would seek one row down.
func (l *ResponsiveList) CanSeekDown() bool {
	// let it get to == len, so we are always able to draw
	// a last element that would not fit with a last index == len-1
	return l.offset.value+l.drawn < l.Len()
}

// SeekDown shifts the contents of this list one row down.
func (l *ResponsiveList) SeekDown() bool {
	ok := l.CanSeekDown()
	if ok {
		// NOTE: we break encapsulation but it's necessary to enable
		// drawing last element until the end of it, in case the elements
		// drawn in the screen are > 1 in element height.
		l.offset.value++
		l.simulateDraw()
		l.setDrawOffset()
	}
	return ok
}

// SeekEnd shifts the contents of this list such that the last element
// is drawn at the top of the list.
func (l *ResponsiveList) SeekEnd() (ok bool) {
	// fix case when a previous seek down/end went past
	// max offset with new width/height.
	prev := l.offset.value
	for l.SeekUp() {
	}
	for l.SeekDown() {
	}
	return prev != l.offset.value
}

// if offset is last, we should make sure last component is fully drawn
// this should be called every time we manipulate list or resize
func (l *ResponsiveList) setDrawOffset() {
	l.loffset = 0
	// only set loffset if we are at last offset
	if l.offset.value+l.drawn != l.Len() {
		return
	}
	// only set loffset if drawn elements overflowed height
	if l.ypos <= l.height {
		return
	}
	l.loffset = l.ypos - l.height
}

func (l *ResponsiveList) simulateDraw() {
	l.drawn = 0
	l.ypos = 0
	var i int
	for el, ok := l.Front(); ok; el, ok = el.Next() {
		comp := el.el.Value.(*Virtual)
		if i < l.offset.value {
			i++
			continue
		}

		if l.ypos >= l.height {
			break
		}

		l.ypos += comp.C.(Responsive).Height(l.width)
		l.drawn++
		i++
	}
}

// Resize resizes this list to fit within width and height.
func (l *ResponsiveList) Resize(width, height int) {
	l.width, l.height = width, height
	l.drawn = 0
	l.ypos = 0

	var i int
	for el, ok := l.Front(); ok; el, ok = el.Next() {
		comp := el.el.Value.(*Virtual)
		if i < l.offset.value {
			// clean break signal for Draw
			comp.Move(term.Coordinates{X: 0, Y: 0})
			i++
			continue
		}

		if l.ypos >= height {
			// signals Draw to break loop
			comp.Move(term.Coordinates{X: 0, Y: -1})
			break
		}

		compHeight := comp.C.(Responsive).Height(width)
		comp.Resize(width, compHeight)
		comp.Move(term.Coordinates{X: 0, Y: l.ypos})
		l.ypos += compHeight
		l.drawn++
		i++
	}
	l.setDrawOffset()
}

// Draw draws this list's elements with the current seek offset.
func (l *ResponsiveList) Draw(w term.Writer) {
	// re-build list offsets to cleanup nodes
	l.Resize(l.width, l.height)

	// elements can be partially rendered
	w = term.BoundsCheckWriter(l.width, l.height, w)

	var i int
	for el, ok := l.Front(); ok; el, ok = el.Next() {
		if i < l.offset.value {
			i++
			continue
		}
		if el.el.Value.(*Virtual).Position().Y == -1 {
			break
		}
		pos := el.el.Value.(*Virtual).Position()
		pos.Y -= l.loffset

		el.el.Value.(*Virtual).Move(pos)
		el.el.Value.(*Virtual).Draw(w)
		i++
	}

	return
}

// ElementAt returns the element at pos Coordinates or panics if
// coordinates are out of the bounds of this ResponsiveList.
func (l *ResponsiveList) ElementAt(pos term.Coordinates) (ListNode, bool) {
	if pos.X < 0 || pos.Y < 0 {
		panic("negative coordinates")
	}

	el, ok := l.Front()
	if !ok {
		return ListNode{}, ok
	}

	for i := 0; i < l.offset.value; i++ {
		// we could have gone past if there was a resize
		// from 0,0 to anything larger
		el, ok = el.Next()
		if !ok {
			return ListNode{}, false
		}
	}

	ok = true
	ypos := -l.loffset
	for ok {
		ypos += el.el.Value.(*Virtual).Height()
		if ypos > pos.Y {
			return el, true
		}
		el, ok = el.Next()
	}

	return ListNode{}, false
}

// Sort sorts the elements of this list with the provided less function.
func (l *ResponsiveList) Sort(less func(a, b Responsive) bool) {
	l.List.Sort(func(a, b tui.Component) bool {
		return less(a.(Responsive), b.(Responsive))
	})
}

// Iterate iterates over all elements in l.
func (l *ResponsiveList) Iterate(fn func(Responsive)) {
	for node, ok := l.Front(); ok; node, ok = node.Next() {
		fn(node.Value().(Responsive))
	}
}

// SetElementHeight panics because this list's elements are Responsive
// components and so they are responsible for setting their own height.
func (l *ResponsiveList) SetElementHeight(i int) {
	panic("Invalid method for ResponsiveList")
}
