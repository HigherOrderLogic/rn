package component

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

// WithAttributes represents a tui.Component that can be set attributes.
type WithAttributes interface {
	tui.Component
	SetAttr(term.Attributes) term.Attributes
}

type attributesAt struct {
	term.Coordinates
	term.Attributes
}

var _ WithAttributes = (*AttrSetter)(nil)
var _ Floating = (*AttrSetter)(nil)
var _ Responsive = (*AttrSetter)(nil)

// AttrSetter wraps another tui.Component to satisfy WithAttributes
// and add the ability to set the Attributes of arbitrary cells.
type AttrSetter struct {
	def    term.Attributes
	attr   []attributesAt
	comp   tui.Component
	width  int
	height int
}

// WithAttrSetter wraps a tui.Component to satisfy WithAttributes.
func WithAttrSetter(comp tui.Component) *AttrSetter {
	attr := make([]attributesAt, 0, 20)
	return &AttrSetter{term.Attributes{}, attr, comp, 0, 0}
}

// Reset resets all the previous calls to SetAttrAt and SetAttr.
func (s *AttrSetter) Reset() {
	s.attr = s.attr[:0]
	s.def = term.Attributes{}
}

// SetAttr sets the default attributes drawn by this component.
// This attributes will be set in each of the final term.Cells drawn.
func (s *AttrSetter) SetAttr(attr term.Attributes) (ret term.Attributes) {
	ret = s.def
	s.def = attr
	return
}

// Dimensions satisfies Floating if underlying tui.Component
// satisfies Floating, or panics if it doesn't.
func (s *AttrSetter) Dimensions() (width, height int) {
	return s.comp.(Floating).Dimensions()
}

// Height satisfies Responsive if underlying tui.Component
// satisfies Responsive, or panics if it doesn't.
func (s *AttrSetter) Height(width int) int {
	return s.comp.(Responsive).Height(width)
}

// SetAttrAt sets the attributes at the given coordinates.
// If coordinates are out of bounds, this method will NOT panic, but
// the next call to Draw will.
func (s *AttrSetter) SetAttrAt(at term.Coordinates, attr term.Attributes) {
	s.attr = append(s.attr, attributesAt{at, attr})
}

// Content returns the underlying tui.Component of this AttrSetter.
func (s *AttrSetter) Content() tui.Component {
	return s.comp
}

// Resize satisfies tui.Component
func (s *AttrSetter) Resize(width, height int) {
	s.width, s.height = width, height
}

// Draw draws the underlying component along with
// the attributes.
func (s *AttrSetter) Draw(w term.Writer) {
	compWithAttr, is := s.comp.(WithAttributes)
	if is {
		compWithAttr.SetAttr(s.def)
	}

	var bw cell.BufferWriter
	bw.Init(w.Context(), s.width, s.height)
	s.comp.Resize(s.width, s.height)
	s.comp.Draw(&bw)
	cells := bw.RawCells()

	if !is {
		for y, row := range cells {
			for x := range row {
				cells[y][x].Attributes = term.AttributesUnion(cells[y][x].Attributes, s.def)
			}
		}
	}

	for _, a := range s.attr {
		if a.Y >= s.height || a.Y < 0 ||
			a.X >= s.width || a.X < 0 {
			continue
		}
		cells[a.Y][a.X].Attributes = term.AttributesUnion(
			cells[a.Y][a.X].Attributes, a.Attributes)
	}

	var buf cell.Buffer
	bw.ToBuffer(&buf)
	var sc Scroll
	sc.InitPerformance(&buf)
	sc.Resize(s.width, s.height)
	sc.Draw(w)
}
