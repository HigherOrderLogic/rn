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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/handler/handlertest"
)

const content = `AAAXXBBBBB
CCCCCDDDDD
EEEEEFFFFF
GGGGGHHHHH
IIIIIJJJJJ
KKKKXXLLLL
9999999999
8888888888
3333333333
11111111XX`

func TestLessDrawSuperimposedBar(t *testing.T) {
	b := NewLess(LessConfig{Wrap: true, SuperimposeMessage: true})
	b.Resize(20, 4)

	w := term.NewStringWriter(20, 9)

	tests := []comptest.TestCase{
		{
			nil, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() { b.Resize(20, 1) }, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() { b.Resize(20, 9); b.SetMessage("P1Nav") }, `
                    
                    
                    
                    
                    
                    
                    
                    
               P1Nav`,
		}, {
			func() {
				b.Buffer().WriteString("hello world")
				b.Resize(20, 1)
			}, `
               P1Nav
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				b.Resize(20, 9)
				b.Buffer().WriteString(". Let's test its responsiveness")
				b.Resize(20, 6)
			}, `
hello world. Let's t
est its responsivene
ss                  
                    
                    
               P1Nav
                    
                    
                    `,
		}, {
			func() {
				b.scroll.Wrap = false
			}, `
hello world. Let's t
                    
                    
                    
                    
               P1Nav
                    
                    
                    `,
		}, {
			func() {
				b.SetMessage("")
			}, `
hello world. Let's t
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				b.scroll.InvertOffset = true
			}, `
                    
                    
                    
                    
                    
hello world. Let's t
                    
                    
                    `,
		}, {
			func() {
				b.SetMessage("remei")
			}, `
                    
                    
                    
                    
                    
hello world. Leremei
                    
                    
                    `,
		},
	}
	comptest.TestComponent(t, b, w, tests)
}

func TestLessDrawNoBarWrap(t *testing.T) {
	b := NewLess(LessConfig{Wrap: true, NoBar: true})
	b.Resize(20, 4)

	w := term.NewStringWriter(20, 9)

	tests := []comptest.TestCase{
		{
			nil, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() { b.Resize(20, 1) }, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() { b.Resize(20, 9) }, `
                    
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				b.Buffer().WriteString("hello world")
				b.Resize(20, 1)
			}, `
hello world         
                    
                    
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				b.Resize(20, 9)
				b.Buffer().WriteString(". Let's test its responsiveness")
				b.Resize(20, 6)
			}, `
hello world. Let's t
est its responsivene
ss                  
                    
                    
                    
                    
                    
                    `,
		}, {
			func() {
				b.Buffer().WriteString(". Let's test its responsiveness")
				b.Resize(20, 8)
			}, `
hello world. Let's t
est its responsivene
ss. Let's test its r
esponsiveness       
                    
                    
                    
                    
                    `,
		}, {
			func() {
				b.Buffer().WriteString(". Let's test its responsiveness")
				b.Buffer().WriteString(". Let's test its scrolling. " +
					"Let's make it overflow below and wrap," +
					"which might just take a little bit of text.")
				b.Resize(20, 9)
			}, `
hello world. Let's t
est its responsivene
ss. Let's test its r
esponsiveness. Let's
 test its responsive
ness. Let's test its
 scrolling. Let's ma
ke it overflow below
 and wrap,which migh`,
		}, {
			func() {
				_, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
				require.True(t, handled)
				_, handled = b.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
				require.True(t, handled)
			}, `
ss. Let's test its r
esponsiveness. Let's
 test its responsive
ness. Let's test its
 scrolling. Let's ma
ke it overflow below
 and wrap,which migh
t just take a little
 bit of text.       `,
		}, {
			func() {
				_, handled := b.Handle(term.Event{Type: term.EventKey, Ch: 'k'})
				require.True(t, handled)
				_, handled = b.Handle(term.Event{Type: term.EventKey, Ch: 'k'})
				require.True(t, handled)
			}, `
hello world. Let's t
est its responsivene
ss. Let's test its r
esponsiveness. Let's
 test its responsive
ness. Let's test its
 scrolling. Let's ma
ke it overflow below
 and wrap,which migh`,
		},
	}
	comptest.TestComponent(t, b, w, tests)
}

func TestLessHandle(t *testing.T) {
	cases := getLessHandleTestFlow([26]term.Event{
		{},
		{Ch: 'k', Type: term.EventKey},
		{Ch: 'j', Type: term.EventKey},
		{Ch: 'l', Type: term.EventKey},
		{Ch: 'h', Type: term.EventKey},
		{Ch: '$', Type: term.EventKey},
		{Ch: '0', Type: term.EventKey},
		{Ch: 'G', Type: term.EventKey},
		{Ch: 'g', Type: term.EventKey},
		{Ch: '/', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Key: term.KeyBackspace, Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Key: term.KeyEnter, Type: term.EventKey},
		{Ch: 'g', Type: term.EventKey},
		{Ch: 'N', Type: term.EventKey},
		{Ch: 'n', Type: term.EventKey},
		{Ch: 'G', Type: term.EventKey},
		{Ch: '?', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Ch: 'X', Type: term.EventKey},
		{Key: term.KeyEnter, Type: term.EventKey},
		{Ch: 'n', Type: term.EventKey},
		{Ch: 'n', Type: term.EventKey},
		{Ch: 'N', Type: term.EventKey},
	})
	testLessHandle(t, cases)
}

func getLessHandleTestFlow(events [26]term.Event) []handlertest.SingleTestCase {
	return []handlertest.SingleTestCase{
		{
			events[0], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
        `,
		},
		{
			events[1], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
        `,
		},
		{
			events[2], `
CCCCCDDD
EEEEEFFF
GGGGGHHH
        `,
		},
		{
			events[3], `
CCCCDDDD
EEEEFFFF
GGGGHHHH
        `,
		},
		{
			events[4], `
CCCCCDDD
EEEEEFFF
GGGGGHHH
        `,
		},
		{
			events[5], `
CCCDDDDD
EEEFFFFF
GGGHHHHH
        `,
		},
		{
			events[6], `
CCCCCDDD
EEEEEFFF
GGGGGHHH
        `,
		},
		{
			events[7], `
88888888
33333333
11111111
        `,
		},
		{
			events[8], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
        `,
		},
		{
			events[9], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
/       `,
		},
		{
			events[10], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
/X      `,
		},
		{
			events[11], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
/       `,
		},
		{
			events[12], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
/X      `,
		},
		{
			events[13], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
/XX     `,
		},
		{
			events[14], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
        `,
		},
		{
			events[15], `
AAAXXBBB
CCCCCDDD
EEEEEFFF
        `,
		},
		{
			events[16], `
111111XX
        
        
        `,
		},
		{
			events[17], `
AXXBBBBB
CCCDDDDD
EEEFFFFF
        `,
		},
		{
			events[18], `
88888888
33333333
111111XX
        `,
		},
		{
			events[19], `
88888888
33333333
111111XX
?       `,
		},
		{
			events[20], `
88888888
33333333
111111XX
?X      `,
		},
		{
			events[21], `
88888888
33333333
111111XX
?XX     `,
		},
		{
			events[22], `
KKXXLLLL
99999999
88888888
        `,
		},
		{
			events[23], `
AXXBBBBB
CCCDDDDD
EEEFFFFF
        `,
		},
		{
			events[24], `
111111XX
        
        
        `,
		},
		{
			events[25], `
AXXBBBBB
CCCDDDDD
EEEFFFFF
        `,
		},
	}
}

func testLessHandle(t *testing.T, cases []handlertest.SingleTestCase) {
	var less [2]Less
	var less1 *Less
	var writer1, writer2, writer3 *term.StringWriter
	_, writer1 = setup(t, &less[0], 8, 4)
	_, writer2 = setup(t, &less[1], 8, 4)
	less1, writer3 = setup(t, nil, 8, 4)

	// test cases with allocated less
	handlertest.TestHandler(t, &less[0], cases, writer1)
	handlertest.TestHandler(t, &less[1], cases, writer2)

	// test cases with stack less
	handlertest.TestHandler(t, less1, cases, writer3)
}

func setup(t *testing.T, less *Less, width, height int) (*Less, *term.StringWriter) {
	cfg := DefaultLessConfig()
	cfg.BarAttr = term.Attributes{Bg: tcell.ColorBlack}
	if less == nil {
		less = NewLess(cfg)
	} else {
		less.Init(cfg)
	}

	_, err := less.Buffer().ReadFrom(strings.NewReader(content))
	require.NoError(t, err)

	less.Resize(width, height)

	return less, term.NewStringWriter(width, height)
}
