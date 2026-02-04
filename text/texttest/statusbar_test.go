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

package texttest

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"unstable.build/go-tui/ide/syntax"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
)

func TestStatusBarStatus(t *testing.T) {
	testURI, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)
	mockGit := &differ{}
	cb := func(fn func()) bool {
		fn()
		return true
	}
	cfg := text.StatusBarConfig{
		ScheduleNextTick: cb,
		Workspace:        testURI,
		Publisher:        &TestEditor{},
		Layout: []text.StatusBarComponent{
			{
				Type:       text.StatusBarStatus,
				Attributes: term.Attributes{Fg: tcell.ColorYellow, Bg: tcell.ColorRed},
				Template:   "█%s█▓▒░",
			},
		},
		GitService: mockGit,
	}
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	bar := text.WithStatusBar(h, buf, scroll, false, false, cfg)

	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)

	tests := []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
██▓▒░               `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells := w.Cells()
	require.Len(t, cells, 10*20)
	require.Equal(t, '█', cells[180].Ch)
	assert.Equal(t, tcell.ColorRed, cells[180].Fg)
	assert.Equal(t, tcell.ColorRed, cells[181].Fg)
	for i := 182; i < 185; i++ {
		assert.Equal(t, tcell.ColorRed, cells[i].Fg)
	}

	bar.SetStatus("INTERESTING",
		term.Attributes{Bg: tcell.ColorOlive, Fg: tcell.ColorMaroon})
	tests = []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
█INTERESTING█▓▒░    `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells = w.Cells()
	require.Len(t, cells, 10*20)
	assert.Equal(t, tcell.ColorOlive, cells[180].Fg)
	for i := 181; i < 181+len("INTERESTING"); i++ {
		assert.Equal(t, tcell.ColorMaroon, cells[i].Fg)
		assert.Equal(t, tcell.ColorOlive, cells[i].Bg)
	}
	for i := 193; i < 196; i++ {
		assert.Equal(t, tcell.ColorOlive, cells[i].Fg)
	}
}

func TestStatusBarFilepath(t *testing.T) {
	testURI, err := workspaceapi.ParseURI("memory:///workspace")
	require.NoError(t, err)
	mockGit := &differ{}
	cb := func(fn func()) bool {
		fn()
		return true
	}
	ed := TestEditor{}
	cfg := text.StatusBarConfig{
		ScheduleNextTick: cb,
		Workspace:        testURI,
		Publisher:        &ed,
		GitService:       mockGit,
		Layout: []text.StatusBarComponent{
			{
				Type:       text.StatusBarFilePath,
				Attributes: term.Attributes{Fg: tcell.ColorYellow, Bg: tcell.ColorRed},
				Template:   "  %s",
			},
			{
				Type:     text.StatusBarStatus,
				Template: " %s",
			},
		},
	}
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	h.URI, err = workspaceapi.ParseURI("memory:///workspace/relpath.go")
	require.NoError(t, err)

	bar := text.WithStatusBar(h, buf, scroll, false, false, cfg)
	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)

	bar.SetStatus("X", term.Attributes{})
	tests := []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
  relpath.go    X   `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells := w.Cells()
	require.Len(t, cells, 10*20)
	for i := 180; i < 180+len("relpath.go"); i++ {
		assert.Equal(t, tcell.ColorYellow, cells[i].Fg)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg)
	}

	buf.InsertString(term.Coordinates{}, "package")
	tests = []comptest.TestCase{
		{Expected: `
packagepackage main 
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
  relpath.go   X   `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells = w.Cells()
	require.Len(t, cells, 10*20)
	for i := 180; i < 180+len("relpath.go"); i++ {
		assert.Equal(t, tcell.ColorOlive, cells[i].Fg)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg)
	}

	ok, _ := buf.Undo()
	require.True(t, ok)
	tests = []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
  relpath.go    X   `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells = w.Cells()
	require.Len(t, cells, 10*20)
	for i := 180; i < 180+len("relpath.go"); i++ {
		assert.Equal(t, tcell.ColorYellow, cells[i].Fg)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg)
	}

	ok, _ = buf.Redo()
	require.True(t, ok)
	tests = []comptest.TestCase{
		{Expected: `
packagepackage main 
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
  relpath.go   X   `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells = w.Cells()
	require.Len(t, cells, 10*20)
	for i := 180; i < 180+len("relpath.go"); i++ {
		assert.Equal(t, tcell.ColorOlive, cells[i].Fg)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg)
	}

	require.Len(t, ed.subs[textapi.EventTypeFlush], 1)
	assert.False(t, ed.subs[textapi.EventTypeFlush][0].
		Handle(context.Background(),
			textapi.Event{URI: h.URI, Type: textapi.EventTypeFlush}))
	tests = []comptest.TestCase{
		{Expected: `
packagepackage main 
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
  relpath.go    X   `,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	cells = w.Cells()
	require.Len(t, cells, 10*20)
	for i := 180; i < 180+len("relpath.go"); i++ {
		assert.Equal(t, tcell.ColorYellow, cells[i].Fg)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg)
	}

}

func TestStatusBarGitService(t *testing.T) {
	testURI, err := workspaceapi.ParseURI("memory:///workspace")
	require.NoError(t, err)
	mockGit := &differ{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	cb := func(fn func()) bool {
		defer wg.Done()
		mu.Lock()
		defer mu.Unlock()
		fn()
		return true
	}
	ed := TestEditor{}
	cfg := text.StatusBarConfig{
		ScheduleNextTick: cb,
		Workspace:        testURI,
		Publisher:        &ed,
		GitService:       mockGit,
		Layout: []text.StatusBarComponent{
			{Template: "   %s  ", Type: text.StatusBarGitShortRef},
			{Template: "  %d", Type: text.StatusBarGitDiffAdded},
			{Template: "   %d ", Type: text.StatusBarGitDiffDeleted},
		},
	}
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	h.URI, err = workspaceapi.ParseURI("memory:///workspace/relpath.go")
	require.NoError(t, err)

	wg.Add(1)
	mu.Lock()
	bar := text.WithStatusBar(h, buf, scroll, false, false, cfg)
	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)
	mu.Unlock()

	tests := []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
   main     2    `,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(1)
	mu.Lock()
	buf.InsertString(term.Coordinates{}, "package")
	assert.False(t, ed.subs[textapi.EventTypeFlush][0].
		Handle(context.Background(),
			textapi.Event{URI: h.URI, Type: textapi.EventTypeFlush}))
	mu.Unlock()
	tests = []comptest.TestCase{
		{Expected: `
packagepackage main 
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
   main     2    `,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()
}

func TestStatusBarCursor(t *testing.T) {
	testURI, err := workspaceapi.ParseURI("memory:///workspace")
	require.NoError(t, err)
	mockGit := &differ{}
	cb := func(fn func()) bool {
		fn()
		return true
	}
	ed := TestEditor{}
	cfg := text.StatusBarConfig{
		ScheduleNextTick: cb,
		Workspace:        testURI,
		Publisher:        &ed,
		GitService:       mockGit,
		Layout: []text.StatusBarComponent{
			{Type: text.StatusBarVoid},
			{Template: "%d:", Type: text.StatusBarCoordinatesCursorX},
			{Template: "%d  ", Type: text.StatusBarCoordinatesCursorY},
			{Template: "%d lines  ", Type: text.StatusBarTotalLines},
			{
				Template:   "%s  ",
				Type:       text.StatusBarLanguage,
				Attributes: term.Attributes{Attrs: tcell.AttrBold},
			},
		},
	}
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	h.URI, err = workspaceapi.ParseURI("memory:///workspace/relpath.go")
	require.NoError(t, err)

	bar := text.WithStatusBar(h, buf, scroll, false, false, cfg)
	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)

	tests := []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
1:1  22 lines  unkno`,
		},
	}
	comptest.TestComponent(t, bar, w, tests)

	h.cursor = 10
	_, handled := bar.Handle(term.Event{Ch: 'a', Type: term.EventKey})
	assert.True(t, handled)
	tests = []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
1:11  22 lines  unkn`,
		},
	}
	comptest.TestComponent(t, bar, w, tests)
}

func TestStatusBarSyntax(t *testing.T) {
	testURI, err := workspaceapi.ParseURI("memory:///workspace")
	require.NoError(t, err)
	mockGit := &differ{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	cb := func(fn func()) bool {
		defer wg.Done()
		mu.Lock()
		defer mu.Unlock()
		fn()
		return true
	}
	ed := TestEditor{}
	cfg := text.StatusBarConfig{
		ScheduleNextTick: cb,
		Workspace:        testURI,
		Publisher:        &ed,
		GitService:       mockGit,
		ErrorColor:       tcell.ColorMaroon,
		Layout: []text.StatusBarComponent{
			{Type: text.StatusBarVoid},
			{
				Template:   "%s  ",
				Type:       text.StatusBarLanguage,
				Attributes: term.Attributes{Fg: tcell.ColorYellow, Bg: tcell.ColorRed},
			},
		},
	}
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	h.URI, err = workspaceapi.ParseURI("memory:///workspace/relpath.go")
	require.NoError(t, err)

	state := make(chan syntax.State)
	mockSyntax := &testSyntax{ch: state}
	view := buf.WithView(mockSyntax)
	mockSyntax.view = view

	go func() {
		state <- syntax.State{}
	}()
	wg.Add(1)
	mu.Lock()
	bar := text.WithStatusBar(h, buf, scroll, false, false, cfg)
	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)
	mu.Unlock()

	tests := []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
           unknown  `,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(1)
	mu.Lock()
	state <- syntax.State{LangID: "GO"}
	mu.Unlock()
	tests = []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
                GO  `,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	cells := w.Cells()
	require.Len(t, cells, 10*20)
	for i := 196; i < 196+len("GO"); i++ {
		assert.Equal(t, tcell.ColorYellow, cells[i].Fg)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg)
	}

	wg.Add(1)
	mu.Lock()
	state <- syntax.State{ParserError: "OOPS", LangID: "GO"}
	mu.Unlock()
	tests = []comptest.TestCase{
		{Expected: `
package main        
                    
import (            
    "fmt"           
                    
    "github.com/unst
)                   
                    
func main() {       
                GO  `,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	cells = w.Cells()
	require.Len(t, cells, 10*20)
	for i := 196; i < 196+len("GO"); i++ {
		assert.Equal(t, tcell.ColorMaroon, cells[i].Fg, i)
		assert.Equal(t, tcell.ColorRed, cells[i].Bg, i)
	}

	require.NoError(t, bar.Close())
}

type testSyntax struct {
	ch   chan syntax.State
	view cell.View
}

func (s testSyntax) State() iterator.Iterator[syntax.State] {
	return iterator.FromFunc(func(ctx context.Context) (syntax.State, bool, error) {
		state, ok := <-s.ch
		return state, ok, nil
	}, func() error {
		close(s.ch)
		return nil
	})
}

func (s testSyntax) Rows() int {
	return s.view.Rows()
}
func (s testSyntax) Columns(row int) int {
	return s.view.Columns(row)
}
func (s testSyntax) Cell(pos term.Coordinates) (term.Cell, bool) {
	return s.view.Cell(pos)
}
func (s testSyntax) RawCells() [][]term.Cell {
	return s.view.RawCells()
}

func (s testSyntax) String() string {
	return s.view.String()
}

func BenchmarkStatusBarMoveCursorSmall(b *testing.B) {
	benchmarkStatusBar(b, 10, 10, true)
}

func BenchmarkStatusBarMoveCursorMedium(b *testing.B) {
	benchmarkStatusBar(b, 100, 100, true)
}

func BenchmarkStatusBarMoveCursorLarge(b *testing.B) {
	benchmarkStatusBar(b, 1000, 1000, true)
}

func BenchmarkStatusBarRenderAllSmall(b *testing.B) {
	benchmarkStatusBar(b, 10, 10, false)
}

func BenchmarkStatusBarRenderAllMedium(b *testing.B) {
	benchmarkStatusBar(b, 100, 100, false)
}

func BenchmarkStatusBarRenderAllLarge(b *testing.B) {
	benchmarkStatusBar(b, 1000, 1000, false)
}

func benchmarkStatusBar(b *testing.B, width, height int, moveCursor bool) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	var wg sync.WaitGroup

	uri, err := workspaceapi.ParseURI("memory:///tmp/my_workspace")
	require.NoError(b, err)

	cfg := text.StatusBarConfig{
		Workspace: uri,
		ScheduleNextTick: func(fn func()) bool {
			fn()
			wg.Done()
			return true
		},
		Publisher:  &TestEditor{},
		GitService: &differ{},
	}

	// do not perform async ops on every draw:
	// if we move cursor before every draw, this would require wg.Add, wg.Wait
	// on every iteration, hiding some of the ops we want to analyze
	moveCursorModulo := 1
	var layout string
	if moveCursor {
		moveCursorModulo = 2
		layout = `█{{ .Status | bg "red" | fg "black" | bold }}█▓▒░  {{ .Filepath }}   {{ .ShiftRight }} {{ .CursorColumn }}:{{ .CursorLine }}  {{ .TotalLines }}  {{ .Language | bold }}  `
	} else {
		wg.Add(1)
		layout = `█{{ .Status | bg "red" | fg "black" | bold }}█▓▒░  {{ .Filepath }}   {{ .GitShortRef | italic }}   {{ .GitDiffAdd | fg "green" }}   {{ .GitDiffDel | fg "red" }} {{ .ShiftRight }} {{ .CursorColumn }}:{{ .CursorLine }}  {{ .TotalLines }}  {{ .Language | bold }}  `
	}
	cfg.Layout, err = text.ParseStatusBarLayout(layout)
	require.NoError(b, err)

	bar := text.WithStatusBar(h, buf, scroll, false, false, cfg)
	bar.SetStatus(" NORMAL", term.Attributes{})
	bar.Resize(width, height)
	bar.Draw(term.NoopWriter{})
	wg.Wait()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.cursor = i % moveCursorModulo
		bar.Handle(term.Event{})
		bar.Draw(term.NoopWriter{})
	}
}
