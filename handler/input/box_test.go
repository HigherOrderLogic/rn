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

package input

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text/modeless"
)

func TestBox(t *testing.T) {
	ed := modeless.Editor()
	t.Run("min and max height passed are coherent or else it panics", func(t *testing.T) {
		// ok
		NewBox(cell.NewBuffer(), ed, BoxConfig{})

		// ok
		NewBox(cell.NewBuffer(), ed, BoxConfig{MinHeight: 1})

		// ok
		NewBox(cell.NewBuffer(), ed, BoxConfig{MaxHeight: 1})

		// ok
		NewBox(cell.NewBuffer(), ed, BoxConfig{MaxHeight: 1, MinHeight: 1})

		// ok
		NewBox(cell.NewBuffer(), ed, BoxConfig{MaxHeight: 2, MinHeight: 1})

		assert.Panics(t, func() {
			NewBox(cell.NewBuffer(), ed, BoxConfig{MaxHeight: 1, MinHeight: 3})
		})
	})
	t.Run("no placeholder", func(t *testing.T) {
		buf := cell.NewBuffer()
		b := NewBox(buf, ed, BoxConfig{})
		b.Resize(20, 4)

		w := term.NewStringWriter(20, 9)

		tests := []comptest.TestCase{
			{
				nil, `
┌──────────────────┐
│                  │
│                  │
└──────────────────┘
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(2, 2) }, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(20, 9) }, `
┌──────────────────┐
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() { b.Resize(20, 1) }, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(20, 9) }, `
┌──────────────────┐
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					writeBuffer(b, "hello world")
					assert.Equal(t, 3, b.Height(20))
					b.Resize(20, 1)
				}, `
hello world         
                    
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() {
					b.Resize(20, 9)
					writeBuffer(b, ". Let's test its responsiveness")
					require.Equal(t, 5, b.Height(20))
					b.Resize(20, 6)
				}, `
┌──────────────────┐
│hello world. Let's│
│ test its responsi│
│veness            │
│                  │
└──────────────────┘
                    
                    
                    `,
			}, {
				func() {
					writeBuffer(b, ". Let's test its responsiveness")
					require.Equal(t, 7, b.Height(20))
					b.Resize(20, 8)
				}, `
┌──────────────────┐
│ test its responsi│
│veness. Let's test│
│ its responsivenes│
│s                 │
│                  │
│                  │
└──────────────────┘
                    `,
			}, {
				func() {
					writeBuffer(b, ". Let's test its responsiveness")
					assert.Equal(t, 8, b.Height(20))
					b.Resize(20, 9)
				}, `
┌──────────────────┐
│ test its responsi│
│veness. Let's test│
│ its responsivenes│
│s. Let's test its │
│responsiveness    │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					writeBuffer(b, ". Let's test its scrolling")
					assert.Equal(t, 10, b.Height(20))
				}, `
┌──────────────────┐
│ test its responsi│
│veness. Let's test│
│ its responsivenes│
│s. Let's test its │
│responsiveness. Le│
│t's test its scrol│
│ling              │
└──────────────────┘`,
			}, {
				func() {
					handled := true
					for handled {
						_, handled = b.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowUp})
					}
				}, `
┌──────────────────┐
│hello world. Let's│
│ test its responsi│
│veness. Let's test│
│ its responsivenes│
│s. Let's test its │
│responsiveness. Le│
│t's test its scrol│
└──────────────────┘`,
			},
		}
		comptest.TestComponent(t, b, w, tests)
	})
	t.Run("with placeholder", func(t *testing.T) {
		buf := cell.NewBuffer()
		b := NewBox(buf, ed, BoxConfig{Placeholder: "HERE..."})
		b.Resize(20, 4)

		w := term.NewStringWriter(20, 9)

		tests := []comptest.TestCase{
			{
				nil, `
┌──────────────────┐
│HERE...           │
│                  │
└──────────────────┘
                    
                    
                    
                    
                    `,
			}, {
				func() {
					assert.Equal(t, 3, b.Height(20))
					b.Resize(2, 2)
					assert.Equal(t, 3, b.Height(20))
				}, `
HE                  
RE                  
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(10, 2) }, `
HERE...             
                    
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(20, 9) }, `
┌──────────────────┐
│HERE...           │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() { b.Resize(20, 9) }, `
┌──────────────────┐
│HERE...           │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					writeBuffer(b, "hello world")
					assert.Equal(t, 3, b.Height(20))
				}, `
┌──────────────────┐
│hello world       │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					// last row full feature
					writeBuffer(b, "xxxxxxx")
					assert.Equal(t, 4, b.Height(20))
				}, `
┌──────────────────┐
│hello worldxxxxxxx│
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					writeBuffer(b, "x")
					assert.Equal(t, 4, b.Height(20))
				}, `
┌──────────────────┐
│hello worldxxxxxxx│
│x                 │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			},
		}
		comptest.TestComponent(t, b, w, tests)
	})

	t.Run("with long placeholder", func(t *testing.T) {
		buf := cell.NewBuffer()
		b := NewBox(buf, ed, BoxConfig{Placeholder: "please write to your great, lovely, assistant"})
		b.Resize(20, 4)

		w := term.NewStringWriter(20, 9)

		tests := []comptest.TestCase{
			{
				nil, `
┌──────────────────┐
│please write to yo│
│ur great, lovely, │
└──────────────────┘
                    
                    
                    
                    
                    `,
			}, {
				func() {
					assert.Equal(t, 5, b.Height(20))
					b.Resize(2, 2)
					assert.Equal(t, 5, b.Height(20))
				}, `
pl                  
ea                  
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(10, 2) }, `
please wri          
te to your          
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() { b.Resize(20, 9) }, `
┌──────────────────┐
│please write to yo│
│ur great, lovely, │
│assistant         │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					writeBuffer(b, "hello world")
					assert.Equal(t, 3, b.Height(20))
				}, `
┌──────────────────┐
│hello world       │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			},
		}
		comptest.TestComponent(t, b, w, tests)
	})
	t.Run("with min, max height", func(t *testing.T) {
		buf := cell.NewBuffer()
		b := NewBox(buf, ed, BoxConfig{MinHeight: 4, MaxHeight: 5, Placeholder: "HERE..."})
		b.Resize(20, 4)

		w := term.NewStringWriter(20, 9)

		tests := []comptest.TestCase{
			{
				nil, `
┌──────────────────┐
│HERE...           │
│                  │
└──────────────────┘
                    
                    
                    
                    
                    `,
			}, {
				func() {
					assert.Equal(t, 4, b.Height(20))
					b.Resize(2, 2)
					assert.Equal(t, 4, b.Height(20))
				}, `
HE                  
RE                  
                    
                    
                    
                    
                    
                    
                    `,
			}, {
				func() {
					b.Resize(20, 9)
					writeBuffer(b, "hello world")
					assert.Equal(t, 4, b.Height(20))
				}, `
┌──────────────────┐
│hello world       │
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					writeBuffer(b, "hello worldhelloworldhelloworld")
					assert.Equal(t, 5, b.Height(20))
				}, `
┌──────────────────┐
│hello worldhello w│
│orldhelloworldhell│
│oworld            │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			}, {
				func() {
					// last row full feature is also capped by maxHeight
					writeBuffer(b, "xxxxxxxxxxxx")
					assert.Equal(t, 5, b.Height(20))
				}, `
┌──────────────────┐
│hello worldhello w│
│orldhelloworldhell│
│oworldxxxxxxxxxxxx│
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`,
			},
		}
		comptest.TestComponent(t, b, w, tests)
	})
}

func TestInsertIntegration(t *testing.T) {
	ed := modeless.Editor()
	buf := cell.NewBuffer()
	b := NewBox(buf, ed, BoxConfig{MinHeight: 4, MaxHeight: 5, Placeholder: "HERE..."})
	b.Resize(3, 3)

	for _, r := range "hello world" {
		// simulate real usage
		b.Handle(term.Event{Type: term.EventKey, Ch: r})
		b.Height(3)
		b.Resize(3, 3)
		b.Draw(term.NoopWriter{})
	}
	assert.Equal(t, "hello world", buf.String())
}

// we could write to cell.Buffer directly
// but this is a more realistic way
func writeBuffer(b tui.Handler, str string) {
	for _, r := range str {
		b.Handle(term.Event{Type: term.EventKey, Ch: r})
		b.Draw(term.NewStringWriter(20, 9))
		// scroll needs to be drawn for wrap features to be correct
		// width and height here are not important
	}
}
