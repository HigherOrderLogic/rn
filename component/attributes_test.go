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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
)

func TestDrawAttributes(t *testing.T) {
	t.Run("draws cells correctly and respecting bounds", func(t *testing.T) {
		w := term.NewStringWriter(9, 5)
		u := &component.TestComponent{Ch: 'X'}
		s := WithAttrSetter(u)

		s.Resize(8, 4)

		tests := []comptest.TestCase{
			{
				nil, `
XXXXXXXX 
XXXXXXXX 
XXXXXXXX 
XXXXXXXX 
         `,
			},
		}

		comptest.TestComponent(t, s, w, tests)
	})
}

func TestAttrSetter(t *testing.T) {
	t.Run("attr oob is ignored", func(t *testing.T) {
		w := term.NewStringWriter(2, 2)
		s := WithAttrSetter(&component.TestComponent{Ch: 'x'})
		s.SetAttrAt(term.Coordinates{X: 2, Y: 2}, term.Attributes{Fg: tcell.ColorRed})

		s.Resize(2, 2)

		// test that it doesn't panic
		s.Draw(w)
	})

	t.Run("happy path", func(t *testing.T) {
		w := cell.NewBufferWriter(context.Background(), 4, 4)
		s := WithAttrSetter(&component.TestComponent{Ch: 'a'})
		s.SetAttr(term.Attributes{Fg: tcell.ColorBlue, Bg: tcell.ColorNavy})
		s.SetAttrAt(term.Coordinates{X: 3, Y: 3},
			term.Attributes{
				Fg:    tcell.ColorRed,
				Bg:    tcell.ColorGreen,
				Attrs: tcell.AttrBold | tcell.AttrUnderline,
			})

		s.Resize(4, 4)
		s.Draw(w)

		for y, row := range w.RawCells() {
			for x, cell := range row {
				if y == 3 && x == 3 {
					assert.Equal(t, tcell.ColorGreen, cell.Bg)
					assert.Equal(t, tcell.ColorRed, cell.Fg)
					assert.True(t, cell.Attrs&tcell.AttrUnderline != 0)
					assert.True(t, cell.Attrs&tcell.AttrBold != 0)
				} else {
					assert.Equal(t, tcell.ColorNavy, cell.Bg)
					assert.Equal(t, tcell.ColorBlue, cell.Fg)
				}
			}
		}
	})
}
