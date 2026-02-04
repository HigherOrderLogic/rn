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

package vte

import (
	"context"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/term/vte/vteparser"
	"unstable.build/go-tui/workspace/workspacetest"
)

func TestIntegrationComponent(t *testing.T) {
	t.Parallel()

	suite := []struct {
		desc      string
		altBuffer bool
		sut       func(*testing.T, *Component, *mockTabManager)
	}{
		{
			desc:      "primary scroll down 0 rows does nothing",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    ")
				p.ScrollUp(0)
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")
				assert.False(t, comp.ScrollUp(0))
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")
			},
		},
		{
			desc:      "primary scroll ScrollOffset/MaxOffset rows <= height",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    ")
				assert.Equal(t, 0, comp.ScrollOffset())
				assert.Equal(t, 0, comp.MaxScrollOffset())
			},
		},
		{
			desc:      "primary scroll ScrollOffset/MaxOffset rows > height",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \nf    \ng    ")
				assert.Equal(t, 2, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
			},
		},
		{
			desc:      "primary scroll up 0 rows does nothing",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    ")
				p.ScrollDown(0)
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")
				assert.False(t, comp.ScrollDown(0))
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")
			},
		},
		{
			desc:      "primary input mixed with user scrolls",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				for range 6 {
					p.Input('a')
					p.CarriageReturn()
					p.Linefeed()
				}
				require.True(t, comp.ScrollUp(1))
				p.Input('b')
				require.True(t, comp.ScrollUp(1))
				p.CarriageReturn()
				p.Linefeed()
				comp.ScrollUp(1)
				comp.ScrollDown(3)
				assertDraw(t, comp, "a    \na    \na    \nb    \n     ")
			},
		},
		{
			desc:      "primary scroll ScrollOffset/MaxOffset after scroll",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \nf    \ng    ")
				require.False(t, comp.ScrollDown(1))
				assert.Equal(t, 2, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
				require.True(t, comp.ScrollUp(1))
				assert.Equal(t, 1, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
				require.True(t, comp.ScrollUp(1))
				assert.Equal(t, 0, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
				require.False(t, comp.ScrollUp(1))
				assert.Equal(t, 0, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
				require.True(t, comp.ScrollDown(1))
				assert.Equal(t, 1, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
				require.True(t, comp.ScrollDown(1))
				assert.Equal(t, 2, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
				require.False(t, comp.ScrollDown(1))
				assert.Equal(t, 2, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
			},
		},
		{
			desc:      "primary scroll ScrollOffset/MaxOffset after ScrollBottom",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \nf    \ng    ")
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")
				require.True(t, comp.ScrollUp(100))
				require.True(t, comp.ScrollBottom())
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")
				assert.Equal(t, 2, comp.ScrollOffset())
				assert.Equal(t, 2, comp.MaxScrollOffset())
			},
		},
		{
			desc:      "selection after scroll",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \nf    \ng    ")
				require.True(t, comp.ScrollUp(1))
				assertDraw(t, comp, "b    \nc    \nd    \ne    \nf    ")

				comp.SelectWordAt(term.Coordinates{})
				data, ok := comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "b", data)

				comp.Unselect()

				comp.Select(term.Coordinates{})
				comp.SelectEnd(term.Coordinates{})
				data, ok = comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "b", data)

				comp.Unselect()

				comp.SelectLine(term.Coordinates{})
				data, ok = comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "b    \n", data)
			},
		},
		{
			desc:      "primary scroll up/down with cap",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				p := comp.parserHandler
				comp.Resize(5, 5)
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \nf    \ng    ")
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")

				assert.False(t, comp.ScrollDown(1))
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")

				assert.True(t, comp.ScrollUp(1))
				assertDraw(t, comp, "b    \nc    \nd    \ne    \nf    ")

				assert.True(t, comp.ScrollUp(1))
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")

				assert.False(t, comp.ScrollUp(1))
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")

				assert.True(t, comp.ScrollDown(1))
				assertDraw(t, comp, "b    \nc    \nd    \ne    \nf    ")

				assert.True(t, comp.ScrollDown(1))
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")

				assert.False(t, comp.ScrollDown(1))
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")
			},
		},
		{
			desc:      "primary clear mode saved",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				comp.Resize(5, 5)
				p := comp.parserHandler
				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \n$ .  \nout  \n$    ")
				assertDraw(t, comp, "d    \ne    \n$ .  \nout  \n$    ")

				p.ClearScreen(vteparser.ClearModeSaved)
				assertDraw(t, comp, "d    \ne    \n$ .  \nout  \n$    ")

				assert.False(t, comp.ScrollUp(1))
				assertDraw(t, comp, "d    \ne    \n$ .  \nout  \n$    ")

				assert.False(t, comp.ScrollDown(1))
				assertDraw(t, comp, "d    \ne    \n$ .  \nout  \n$    ")
			},
		},
		{
			desc:      "shell cltr-l with scrollback history",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				comp.Resize(5, 5)
				p := comp.parserHandler

				resetBuffer(t, p, "a    \nb    \nc    \nd    \ne    \n$ .  \nout  \n$    ")
				assertDraw(t, comp, "d    \ne    \n$ .  \nout  \n$    ")
				assert.Equal(t, 3, comp.ScrollOffset())
				assert.Equal(t, 3, comp.MaxScrollOffset())

				p.Goto(0, 0)
				p.ClearScreen(vteparser.ClearModeAll)
				p.ClearScreen(vteparser.ClearModeBelow)
				p.Input('$')
				p.ClearLine(vteparser.LineClearModeRight)
				assertDraw(t, comp, "$    \n     \n     \n     \n     ")
				assert.Equal(t, 8, comp.ScrollOffset())
				assert.Equal(t, 8, comp.MaxScrollOffset())

				// simulate user scrolling
				assert.True(t, comp.ScrollUp(1))
				assertDraw(t, comp, "$    \n$    \n     \n     \n     ")
				assert.Equal(t, 7, comp.ScrollOffset())
				assert.Equal(t, 8, comp.MaxScrollOffset())
				comp.Select(term.Coordinates{})
				comp.SelectEnd(term.Coordinates{})
				data, ok := comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "$", data)

				assert.True(t, comp.ScrollUp(1))
				assertDraw(t, comp, "out  \n$    \n$    \n     \n     ")
				assert.Equal(t, 6, comp.ScrollOffset())
				assert.Equal(t, 8, comp.MaxScrollOffset())

				assert.True(t, comp.ScrollUp(2))
				assertDraw(t, comp, "e    \n$ .  \nout  \n$    \n$    ")
				assert.Equal(t, 4, comp.ScrollOffset())
				assert.Equal(t, 8, comp.MaxScrollOffset())

				assert.True(t, comp.ScrollUp(100))
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")
				assert.Equal(t, 0, comp.ScrollOffset())
				assert.Equal(t, 8, comp.MaxScrollOffset())

				assert.True(t, comp.ScrollDown(100))
				assertDraw(t, comp, "e    \n$ .  \nout  \n$    \n$    ")
				assert.Equal(t, 4, comp.ScrollOffset())
				assert.Equal(t, 8, comp.MaxScrollOffset())

				comp.SelectWordAt(term.Coordinates{})
				data, ok = comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "e", data)

				comp.Unselect()

				comp.Select(term.Coordinates{})
				comp.SelectEnd(term.Coordinates{})
				data, ok = comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "e", data)

				comp.Unselect()

				comp.SelectLine(term.Coordinates{})
				data, ok = comp.Selection()
				assert.True(t, ok)
				assert.Equal(t, "e    \n", data)

				require.True(t, comp.ScrollBottom())
				assertDraw(t, comp, "$    \n     \n     \n     \n     ")
			},
		},
		{
			desc:      "Input + Linefeed + CarriageReturn hit max scrollback history",
			altBuffer: false,
			sut: func(t *testing.T, comp *Component, tm *mockTabManager) {
				comp.Resize(5, 5)
				p := comp.parserHandler
				p.maxScrollLength = 6
				p.ResetState()
				p.Input('a')
				p.Linefeed()
				p.CarriageReturn()
				p.Input('b')
				p.Linefeed()
				p.CarriageReturn()
				p.Input('c')
				p.Linefeed()
				p.CarriageReturn()
				p.Input('d')
				p.Linefeed()
				p.CarriageReturn()
				p.Input('e')
				assertDraw(t, comp, "a    \nb    \nc    \nd    \ne    ")

				p.Linefeed()
				p.CarriageReturn()
				p.Input('f')
				assertDraw(t, comp, "b    \nc    \nd    \ne    \nf    ")

				p.CarriageReturn()
				p.Linefeed()
				p.Input('g')
				assertDraw(t, comp, "c    \nd    \ne    \nf    \ng    ")
				assert.Equal(t, p.maxScrollLength, p.sync.buf.Rows())
			},
		},
	}

	for _, test := range suite {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			tm := mockTabManager{}

			cfg := DefaultConfig()
			comp, err := NewComponent(&testExecutor{}, &testExecutor{}, &tm, cfg)
			require.NoError(t, err)

			ph := comp.parserHandler
			ph.sync.primBuf.SetDefaultChar(' ')
			ph.sync.altBuf.SetDefaultChar(' ')

			if test.altBuffer {
				ph.SetPrivateMode(vteparser.PrivateModeSwapScreenAndSetRestoreCursor)
			} else {
				ph.UnsetPrivateMode(vteparser.PrivateModeSwapScreenAndSetRestoreCursor)
			}

			test.sut(t, comp, &tm)
		})
	}
}

type testExecutor struct {
}

func (e *testExecutor) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
	return 0, nil
}

func (e *testExecutor) Signal(pid workspaceapi.Pid, signal syscall.Signal) error {
	return nil
}

func (e *testExecutor) Close() error {
	return nil
}

func (e *testExecutor) NewPty(context.Context) (workspaceapi.Pty, error) {
	mockPtyFile := workspacetest.File{}
	return workspaceapi.Pty{Master: &mockPtyFile, Slave: &mockPtyFile}, nil
}

func (e *testExecutor) SetPtySize(p workspaceapi.Pty, width, height int) error {
	return nil
}

func assertDraw(t *testing.T, comp *Component, expected string) {
	t.Helper()
	writer := term.NewStringWriter(comp.width, comp.height)
	comp.Draw(writer)
	writer.Flush()
	assert.Equal(t, expected, writer.String())
}
