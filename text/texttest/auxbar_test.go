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
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/text"
)

func TestAuxBarDrawLinesRelative(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	cb := func(fn func()) bool {
		fn()
		return true
	}

	cfg := text.AuxBarConfig{LinesEnabled: true, ScheduleNextTick: cb}
	bar := text.WithAuxBar(h, buf, scroll, cfg)
	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)

	tests := []comptest.TestCase{
		{Expected: `
1  package main     
1                   
2  import (         
3      "fmt"        
4                   
5      "github.com/u
6  )                
7                   
8  func main() {    
9      fmt.Println("`,
		},
		{
			Action: func() {
				require.True(t, scroll.SeekDown())
			},
			Expected: `
2                   
1  import (         
2      "fmt"        
3                   
4      "github.com/u
5  )                
6                   
7  func main() {    
8      fmt.Println("
9      for i := 0; i`,
		},
	}
	comptest.TestComponent(t, bar, w, tests)
}

func TestAuxBarDrawLinesAbsolute(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	cb := func(fn func()) bool {
		fn()
		return true
	}

	cfg := text.AuxBarConfig{LinesEnabled: true, AbsoluteLines: true, ScheduleNextTick: cb}
	bar := text.WithAuxBar(h, buf, scroll, cfg)
	bar.Resize(20, 10)
	w := term.NewStringWriter(20, 10)

	tests := []comptest.TestCase{
		{Expected: `
1  package main     
2                   
3  import (         
4      "fmt"        
5                   
6      "github.com/u
7  )                
8                   
9  func main() {    
10     fmt.Println("`,
		},
		{
			Action: func() {
				require.True(t, scroll.SeekDown())
			},
			Expected: `
2                   
3  import (         
4      "fmt"        
5                   
6      "github.com/u
7  )                
8                   
9  func main() {    
10     fmt.Println("
11     for i := 0; i`,
		},
	}
	comptest.TestComponent(t, bar, w, tests)
}

func TestAuxBarDrawFolds(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cb := func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		wg.Done()
		return true
	}

	wg.Add(1)
	mu.Lock()
	cfg := text.AuxBarConfig{
		GitEnabled:       true, // test that if no lines => disabled
		FoldsEnabled:     true,
		ScheduleNextTick: cb,
	}
	bar := text.WithAuxBar(h, buf, scroll, cfg)
	bar.Resize(20, 10)
	mu.Unlock()
	w := term.NewStringWriter(20, 10)

	tests := []comptest.TestCase{
		{Expected: `
  package main      
                    
 import (          
      "fmt"         
                    
      "github.com/un
  )                 
                    
 func main() {     
      fmt.Println("%`,
		},
	}
	wg.Wait()

	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()
	require.True(t, scroll.SeekDown())

	tests = []comptest.TestCase{
		{Expected: `
                    
 import (          
      "fmt"         
                    
      "github.com/un
  )                 
                    
 func main() {     
      fmt.Println("%
      for i := 0; i `,
		},
	}
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(1)
	_, handled := bar.Handle(
		term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 0, MouseY: 1})
	require.True(t, handled)

	tests = []comptest.TestCase{
		{Expected: `
                    
 import ( [5 lines]
                    
 func main() {     
      fmt.Println("%
      for i := 0; i 
          fmt.Printl
      }             
  }                 
                    `,
		},
	}

	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(1)
	_, handled = bar.Handle(
		term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 0, MouseY: 3})
	require.True(t, handled)

	tests = []comptest.TestCase{
		{Expected: `
                    
 import ( [5 lines]
                    
 func main() { [6 l
                    
  const fileContent 
      "import (\n"+ 
      "\"fmt\"\n"+  
      "\n"+         
      "\"github.com/`,
		},
	}

	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(1)
	_, handled = bar.Handle(
		term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 0, MouseY: 1})
	require.True(t, handled)

	tests = []comptest.TestCase{
		{Expected: `
                    
 import (          
      "fmt"         
                    
      "github.com/un
  )                 
                    
 func main() { [6 l
                    
  const fileContent `,
		},
	}

	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	// nothing happens if we click outside of line
	_, handled = bar.Handle(
		term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 2, MouseY: 1})
	require.True(t, handled)

	tests = []comptest.TestCase{
		{Expected: `
                    
 import (          
      "fmt"         
                    
      "github.com/un
  )                 
                    
 func main() { [6 l
                    
  const fileContent `,
		},
	}

	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()
}

func TestGitBarWithAuxBarRelativeIntegration(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cb := func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		wg.Done()
		return true
	}

	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)

	registry := text.NewFileCommandRegistry(uri, newWorkspaceRegistry())

	ed := &TestEditor{}
	mockSvc := &differ{}
	wg.Add(3) // 2 bars + auxbar resize
	acfg := text.AuxBarConfig{
		LinesEnabled:        true,
		FoldsEnabled:        true,
		AbsoluteLines:       false,
		HighlightCursor:     true,
		GitEnabled:          true,
		ScheduleNextTick:    cb,
		CommandRegistry:     registry,
		Publisher:           ed,
		HighlightCursorAttr: term.Attributes{Bg: tcell.ColorGray}, // just not fg
		LineNumberAttr:      term.Attributes{Bg: tcell.ColorGray}, // just not fg
		Service:             mockSvc,
	}
	gcfg := text.GitBarConfig{
		ScheduleNextTick: cb,
		Publisher:        ed,
		CommandRegistry:  registry,
	}
	mu.Lock()
	bar := text.WithAuxBar(h, buf, scroll, acfg)
	bar = text.WithGitBar(mockSvc, bar, buf, scroll, gcfg)
	bar.Resize(20, 10)
	mu.Unlock()
	w := term.NewStringWriter(20, 10)
	w.ForegroundCh = '#'

	tests := []comptest.TestCase{
		{Expected: `
  1    package main 
  1                 
  2   import (     
  3        "fmt"    
# #                 
# #        "github.c
  6    )            
  7                 
  8   func main() {
# #        fmt.Print`,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	wg.Add(1)
	require.True(t, scroll.SeekDown())
	mu.Unlock()

	tests = []comptest.TestCase{
		{Expected: `
  2                 
  1   import (     
  2        "fmt"    
# #                 
# #        "github.c
  5    )            
  6                 
  7   func main() {
# #        fmt.Print
  9        for i := `,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(2)
	mu.Lock()
	assert.True(t, scroll.SetOffset(term.Coordinates{Y: 1, X: 4}))
	_, handled := bar.Handle(
		term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 5, MouseY: 1})
	require.True(t, handled)
	mu.Unlock()

	tests = []comptest.TestCase{
		{Expected: `
  2                 
# #   rt (#########
  2                 
  3    main() {    
# #    fmt.Println("
  5    for i := 0; i
  6        fmt.Print
  7    }            
  8                 
  9                 `,
		},
	}

	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	require.NoError(t, bar.Close())

	// unsubscribes
	require.Equal(t, 2, len(ed.subs))
	assert.Equal(t, 0, len(ed.subs[textapi.EventTypeFlush]))
	assert.Equal(t, 0, len(ed.subs[textapi.EventTypeFocus]))
}

func TestGitBarWithAuxBarAbsoluteIntegration(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(copy)
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cb := func(fn func()) bool {
		mu.Lock()
		defer mu.Unlock()
		fn()
		wg.Done()
		return true
	}

	uri, err := workspaceapi.ParseURI("memory:///")
	require.NoError(t, err)

	registry := text.NewFileCommandRegistry(uri, newWorkspaceRegistry())

	ed := &TestEditor{}
	mockSvc := &differ{}
	wg.Add(2)
	acfg := text.AuxBarConfig{
		LinesEnabled:        true,
		FoldsEnabled:        true,
		AbsoluteLines:       true,
		HighlightCursor:     true,
		GitEnabled:          true,
		ScheduleNextTick:    cb,
		CommandRegistry:     registry,
		Publisher:           ed,
		HighlightCursorAttr: term.Attributes{Bg: tcell.ColorGray}, // just not fg
		LineNumberAttr:      term.Attributes{Bg: tcell.ColorGray}, // just not fg
		Service:             mockSvc,
	}
	gcfg := text.GitBarConfig{
		ScheduleNextTick: cb,
		Publisher:        ed,
		CommandRegistry:  registry,
	}
	mu.Lock()
	bar := text.WithAuxBar(h, buf, scroll, acfg)
	bar = text.WithGitBar(mockSvc, bar, buf, scroll, gcfg)
	bar.Resize(20, 10)
	mu.Unlock()
	w := term.NewStringWriter(20, 10)
	w.ForegroundCh = '#'

	tests := []comptest.TestCase{
		{Expected: `
  1    package main 
  2                 
  3   import (     
  4        "fmt"    
# #                 
# #        "github.c
  7    )            
  8                 
  9   func main() {
# ##       fmt.Print`,
		},
	}
	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	require.True(t, scroll.SeekDown())
	mu.Unlock()

	tests = []comptest.TestCase{
		{Expected: `
  2                 
  3   import (     
  4        "fmt"    
# #                 
# #        "github.c
  7    )            
  8                 
  9   func main() {
# ##       fmt.Print
  11       for i := `,
		},
	}
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	wg.Add(2)
	assert.True(t, scroll.SetOffset(term.Coordinates{Y: 1, X: 4}))
	_, handled := bar.Handle(
		term.Event{Type: term.EventMouse, Key: term.MouseLeft, MouseX: 5, MouseY: 1})
	require.True(t, handled)

	tests = []comptest.TestCase{
		{Expected: `
  2                 
# #   rt (#########
  8                 
  9    main() {    
# ##   fmt.Println("
  11   for i := 0; i
  12       fmt.Print
  13   }            
  14                
  15                `,
		},
	}

	wg.Wait()
	mu.Lock()
	comptest.TestComponent(t, bar, w, tests)
	mu.Unlock()

	require.NoError(t, bar.Close())

	// unsubscribes
	require.Equal(t, 2, len(ed.subs))
	assert.Equal(t, 0, len(ed.subs[textapi.EventTypeFlush]))
	assert.Equal(t, 0, len(ed.subs[textapi.EventTypeFocus]))
}

func BenchmarkAuxBarAbsoluteSmall(b *testing.B) {
	benchmarkAuxBar(b, 10, 10, true, false)
}

func BenchmarkAuxBarAbsoluteMedium(b *testing.B) {
	benchmarkAuxBar(b, 100, 100, true, false)
}

func BenchmarkAuxBarAbsoluteLarge(b *testing.B) {
	benchmarkAuxBar(b, 1000, 1000, true, false)
}

func BenchmarkAuxBarRelativeSmall(b *testing.B) {
	benchmarkAuxBar(b, 10, 10, false, false)
}

func BenchmarkAuxBarRelativeMedium(b *testing.B) {
	benchmarkAuxBar(b, 100, 100, false, false)
}

func BenchmarkAuxBarRelativeLarge(b *testing.B) {
	benchmarkAuxBar(b, 1000, 1000, false, false)
}

func BenchmarkAuxBarAbsoluteMoveCursorSmall(b *testing.B) {
	benchmarkAuxBar(b, 10, 10, true, true)
}

func BenchmarkAuxBarAbsoluteMoveCursorMedium(b *testing.B) {
	benchmarkAuxBar(b, 100, 100, true, true)
}

func BenchmarkAuxBarAbsoluteMoveCursorLarge(b *testing.B) {
	benchmarkAuxBar(b, 1000, 1000, true, true)
}

func BenchmarkAuxBarRelativeMoveCursorSmall(b *testing.B) {
	benchmarkAuxBar(b, 10, 10, false, true)
}

func BenchmarkAuxBarRelativeMoveCursorMedium(b *testing.B) {
	benchmarkAuxBar(b, 100, 100, false, true)
}

func BenchmarkAuxBarRelativeMoveCursorLarge(b *testing.B) {
	benchmarkAuxBar(b, 1000, 1000, false, true)
}

func benchmarkAuxBar(b *testing.B, width, height int, absolute, moveCursor bool) {
	buf := cell.NewBuffer()
	for buf.Rows() < height {
		buf.WriteString(copy)
	}
	fs := &testFoldsService{}
	fs.view = buf.WithView(fs)
	scroll := component.NewScroll(buf)
	h := newTestHandler(scroll)
	var wg sync.WaitGroup
	cb := func(fn func()) bool {
		fn()
		wg.Done()
		return true
	}

	foldsEnabled := true
	moveCursorModulo := 1
	if moveCursor {
		moveCursorModulo = 2
		foldsEnabled = false
	}

	if foldsEnabled {
		wg.Add(1)
		if !absolute { // resize
			wg.Add(1)
		}
	}

	cfg := text.AuxBarConfig{
		FoldsEnabled:     foldsEnabled,
		LinesEnabled:     true,
		AbsoluteLines:    absolute,
		HighlightCursor:  true,
		ScheduleNextTick: cb,
	}
	bar := text.WithAuxBar(h, buf, scroll, cfg)
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

var _ = (foldsService)(testFoldsService{})

type foldsService interface {
	Folds() (iterator.Iterator[term.Range], bool)
}

type testFoldsService struct {
	view cell.View
}

func (f testFoldsService) Rows() int {
	return f.view.Rows()
}

func (f testFoldsService) Columns(row int) int {
	return f.view.Columns(row)
}

func (f testFoldsService) Cell(at term.Coordinates) (term.Cell, bool) {
	return f.view.Cell(at)
}

func (f testFoldsService) RawCells() [][]term.Cell {
	return f.view.RawCells()
}

func (f testFoldsService) String() string {
	return f.view.String()
}

func (f testFoldsService) FoldsFrom(pos term.Coordinates) (
	iterator.Iterator[term.Range], bool,
) {
	folds, ok := f.Folds()
	if !ok {
		return nil, false
	}
	return iterator.Filter(folds, func(rng term.Range) bool {
		return rng.End.Y > pos.Y || (rng.Start.Y == pos.Y && rng.End.X > pos.X)
	}), true
}

func (f testFoldsService) Folds() (iterator.Iterator[term.Range], bool) {
	return iterator.FromSlice([]term.Range{
		{Start: term.Coordinates{Y: 2, X: 0}, End: term.Coordinates{Y: 6}},
		{Start: term.Coordinates{Y: 8}, End: term.Coordinates{Y: 13}},
		{Start: term.Coordinates{Y: 8, X: 12}, End: term.Coordinates{Y: 13}},
	}), true
}

type testHandler struct {
	*component.Scroll
	cursor int
	URI    workspaceapi.URI
}

func newTestHandler(scroll *component.Scroll) (t *testHandler) {
	t = new(testHandler)
	t.Scroll = scroll
	return t
}

func (t *testHandler) Resource() workspaceapi.URI {
	return t.URI
}

func (t *testHandler) SetWrap(wrap bool) {
}

func (t *testHandler) ShowCommandBar(show bool) {
}

func (t *testHandler) SetCursorAtScroll(term.Coordinates) bool {
	return false
}

func (t *testHandler) Close() error {
	return nil
}

func (t *testHandler) SeekUp() bool {
	return false
}

func (t *testHandler) SeekDown() bool {
	return false
}

func (t *testHandler) SeekOffset() int {
	return 0
}

func (t *testHandler) MaxSeekOffset() int {
	return 0
}

func (h *testHandler) SetLocationList(
	pri textapi.LocationPriority, ID string, loc text.LocationList,
) {
}

func (h *testHandler) LocationLists() []text.LocationSet {
	return nil
}

func (h *testHandler) MoveToNextLocation(ID string) bool {
	return false
}

func (h *testHandler) MoveToPrevLocation(ID string) bool {
	return false
}

func (h *testHandler) CellView() cell.View {
	return nil
}

func (h *testHandler) CellEditor() cell.Editor {
	return nil
}

func (e *testHandler) SetDefaultAttributes(attr term.Attributes) {
}

func (h *testHandler) CursorAtScroll() term.Coordinates {
	return term.Coordinates{Y: h.cursor}
}

func (t *testHandler) Handle(ev term.Event) (bool, bool) {
	return false, true
}

func (t *testHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{Y: t.cursor}, 0, false
}

func (t *testHandler) Selection() (string, bool) {
	return "", false
}

const copy = `package main

import (
	"fmt"

	"github.com/unstablebuild/blue/cli"
)

func main() {
	fmt.Println("%+v", cli.NewCLI)
	for i := 0; i < 10; i++ {
		fmt.Println("%d", i)
	}
}

const fileContent = "package main\n" +
	"import (\n"+
	"\"fmt\"\n"+
	"\n"+
	"\"github.com/unstablebuild/blue/cli\"\n"
	")"
`

type differ struct {
}

func (d differ) ListRemotes(_ context.Context, file workspaceapi.URI) ([]string, error) {
	return nil, nil
}

func (d differ) ShortRef(ctx context.Context, file workspaceapi.URI) (string, error) {
	return "main", nil
}

func (d differ) Diff(ctx context.Context, file workspaceapi.URI) (vctrl.FileDiff, error) {
	return vctrl.FileDiff{Hunks: []vctrl.Hunk{{NewLines: 2, NewStartLine: 5}, {OrigStartLine: 10, OrigLines: 2}}}, nil
}

func (d differ) CurrentCommit(ctx context.Context, file workspaceapi.URI) (string, error) {
	panic("unimplemented")
}

func (d differ) RemoteURL(ctx context.Context, file workspaceapi.URI, remoteName string) (string, error) {
	panic("unimplemented")
}

func (d differ) RelPath(ctx context.Context, file string) (string, error) {
	panic("Unimplemented")
}
