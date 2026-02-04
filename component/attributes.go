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
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
)

type attributesAt struct {
	term.Coordinates
	term.Attributes
}

var _ component.WithAttributes = (*AttrSetter)(nil)
var _ component.Floating = (*AttrSetter)(nil)
var _ component.Responsive = (*AttrSetter)(nil)

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
	return s.comp.(component.Floating).Dimensions()
}

// Height satisfies Responsive if underlying tui.Component
// satisfies Responsive, or panics if it doesn't.
func (s *AttrSetter) Height(width int) int {
	return s.comp.(component.Responsive).Height(width)
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
	compWithAttr, is := s.comp.(component.WithAttributes)
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
