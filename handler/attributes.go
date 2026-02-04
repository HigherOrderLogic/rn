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
	compapi "github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/component"
)

var _ compapi.WithAttributes = AttrSetter{}

// AttrSetter provides an API like component.AttrSetter for a tui.Handler.
type AttrSetter struct {
	comp *component.AttrSetter
	tui.Handler
}

// WithAttrSetter wraps h with a component.AttrSetter.
func WithAttrSetter(h tui.Handler) AttrSetter {
	return AttrSetter{
		comp:    component.WithAttrSetter(h),
		Handler: h,
	}
}

// Resize satisfies tui.Component
func (s AttrSetter) Resize(width, height int) {
	s.comp.Resize(width, height)
}

// Draw satisfies tui.Component
func (s AttrSetter) Draw(w term.Writer) {
	s.comp.Draw(w)
}

// Reset resets all the previous calls to SetAttrAt and SetAttr.
func (s AttrSetter) Reset() {
	s.comp.Reset()
}

// SetAttr sets the default attributes drawn by this component.
// This attributes will be set in each of the final term.Cells drawn.
func (s AttrSetter) SetAttr(attr term.Attributes) term.Attributes {
	return s.comp.SetAttr(attr)
}

// SetAttrAt sets the attributes at the given coordinates.
// If coordinates are out of bounds, this method will NOT panic, but
// the next call to Draw will.
func (s AttrSetter) SetAttrAt(at term.Coordinates, attr term.Attributes) {
	s.comp.SetAttrAt(at, attr)
}
