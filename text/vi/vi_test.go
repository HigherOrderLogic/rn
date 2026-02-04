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

package vi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/clipboard"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"unstable.build/go-tui/handler/handlertest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

var uri workspaceapi.URI

func init() {
	var err error
	uri, err = workspaceapi.ParseURI("file:///vi_test")
	if err != nil {
		panic(err)
	}
}

func TestCursorExternalEdit(t *testing.T) {
	t.Run("if external insert above, moves cursor to keep cursor in current logical line", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2}, vi.CursorAtScroll())
		at := term.Coordinates{}
		vi.CellEditor().Edit(context.Background(), at, at, "a\nb")
		assert.Equal(t, term.Coordinates{Y: 3}, vi.CursorAtScroll())
	})

	t.Run("if external insert below, it does nothing", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2}, vi.CursorAtScroll())
		at := term.Coordinates{Y: 3}
		vi.CellEditor().Edit(context.Background(), at, at, "a\nb")
		assert.Equal(t, term.Coordinates{Y: 2}, vi.CursorAtScroll())
	})

	t.Run("if external delete below, it does nothing", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2}, vi.CursorAtScroll())
		from := term.Coordinates{Y: 3}
		to := term.Coordinates{Y: 4}
		vi.CellEditor().Edit(context.Background(), from, to, "")
		assert.Equal(t, term.Coordinates{Y: 2}, vi.CursorAtScroll())
	})

	t.Run("if external delete above, it keeps cursor at logical line", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2}, vi.CursorAtScroll())
		from := term.Coordinates{Y: 0}
		to := term.Coordinates{Y: 1}
		vi.CellEditor().Edit(context.Background(), from, to, "")
		assert.Equal(t, term.Coordinates{Y: 1}, vi.CursorAtScroll())
	})

	t.Run("if external delete to current line, it keeps cursor at logical line", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'l'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2, X: 1}, vi.CursorAtScroll())
		from := term.Coordinates{Y: 0}
		to := term.Coordinates{Y: 2, X: 5}
		vi.CellEditor().Edit(context.Background(), from, to, "")
		assert.Equal(t, term.Coordinates{Y: 0, X: 0}, vi.CursorAtScroll())
	})

	t.Run("if external insert to current line, it keeps cursor at logical line", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'l'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2, X: 1}, vi.CursorAtScroll())
		at := term.Coordinates{Y: 2, X: 0}
		vi.CellEditor().Edit(context.Background(), at, at, "a\nbbb")
		assert.Equal(t, term.Coordinates{Y: 3, X: 3}, vi.CursorAtScroll())
	})

	t.Run("if external replace to current line, it keeps cursor at logical line", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'l'})
		require.True(t, handled)

		assert.Equal(t, term.Coordinates{Y: 2, X: 1}, vi.CursorAtScroll())
		start := term.Coordinates{Y: 1, X: 0}
		end := term.Coordinates{Y: 2, X: 1}
		vi.CellEditor().Edit(context.Background(), start, end, "a\nbbb")
		assert.Equal(t, term.Coordinates{Y: 2, X: 3}, vi.CursorAtScroll())
	})

	t.Run("if external replace only cols to current line, it keeps cursor at logical position", func(t *testing.T) {
		width, height := 20, 10

		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		vi.Resize(width, height)

		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'j'})
		require.True(t, handled)
		for range 3 {
			_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'l'})
			require.True(t, handled)
		}

		assert.Equal(t, term.Coordinates{Y: 2, X: 3}, vi.CursorAtScroll())
		start := term.Coordinates{Y: 2, X: 0}
		end := term.Coordinates{Y: 2, X: 4}
		vi.CellEditor().Edit(context.Background(), start, end, "bbb")
		assert.Equal(t, term.Coordinates{Y: 2, X: 3}, vi.CursorAtScroll())
	})
}

func TestFoldsIntegration(t *testing.T) {
	t.Run("initial folds", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.WriteString(snippet)
		fs := &testFoldsService{}
		fs.view = buf.WithView(fs)
		var wg sync.WaitGroup
		var mu sync.Mutex
		cb := func(fn func()) bool {
			// run async to guarantee we can lock below:
			// sometimes this cb can run in the main goroutine
			go func() {
				defer wg.Done()
				mu.Lock()
				defer mu.Unlock()
				fn()
			}()
			return true
		}
		wg.Add(2)
		mu.Lock()
		vi := New(buf, uri,
			WithHideInitialFolds(true),
			WithScheduleNextTick(cb),
			WithAutoCenter(true),
		)
		cfg := text.StatusBarConfig{
			Publisher: &texttest.TestEditor{},
			ScheduleNextTick: func(cb func()) bool {
				mu.Lock()
				defer mu.Unlock()
				cb()
				return true
			},
		}
		bar := text.WithStatusBar(vi, buf, vi.less.Scroll(),
			false, false, cfg)
		bar.Resize(20, 10)
		mu.Unlock()

		tests := []comptest.TestCase{
			{Expected: `
                    
/* [4 lines] */     
    void            
diff_buf_adjust(win_
{                   
    win_T    *wp;   
    int             
                    
    if (!win->w_p_di
                    `,
			},
		}

		wg.Wait()
		w := term.NewStringWriter(20, 10)

		mu.Lock()
		defer mu.Unlock()

		comptest.TestComponent(t, bar, w, tests)
	})

	t.Run("fold operations with auxiliary bar", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.WriteString(snippet)
		fs := &testFoldsService{}
		fs.view = buf.WithView(fs)
		var wg sync.WaitGroup
		var mu sync.Mutex
		cb := func(fn func()) bool {
			go func() {
				defer wg.Done()
				mu.Lock()
				fn()
				mu.Unlock()
			}()
			return true
		}
		mu.Lock()
		cfg := text.StatusBarConfig{
			Publisher: &texttest.TestEditor{},
			ScheduleNextTick: func(cb func()) bool {
				cb()
				return true
			},
		}
		ed := Editor(
			WithScheduleNextTick(cb),
			WithAuxiliaryBar(true, text.AuxBarConfig{FoldsEnabled: true, ScheduleNextTick: cb}),
			WithStatusBarConfig(true, cfg),
		)
		wg.Add(1)
		vi, err := ed.Edit(uri, buf, false, false)
		require.NoError(t, err)
		vi.Resize(50, 10)
		mu.Unlock()
		wg.Wait() // wait for bar

		t.Run("zA", func(t *testing.T) {
			tests := []comptest.TestCase{
				{Expected: `
                                                  
 /* [4 lines] */                                 
      void                                        
  diff_buf_adjust(win_T *win)                     
 { [25 lines] }                                  
                                                  
                                                  
                                                  
                                                  
                                            NORMAL`,
				},
			}

			wg.Add(6)
			for _, ch := range "GzAkklhj" {
				mu.Lock()
				_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				mu.Unlock()
				require.True(t, handled)
			}
			wg.Wait()
			w := term.NewStringWriter(50, 10)
			mu.Lock()
			comptest.TestComponent(t, vi, w, tests)
			mu.Unlock()
		})

		t.Run("zo", func(t *testing.T) {
			tests := []comptest.TestCase{
				{Expected: `
                                                  
 /*                                              
  * Check if the current buffer should be added t
   * diff buffers.                                
   */                                             
      void                                        
  diff_buf_adjust(win_T *win)                     
 { [25 lines] }                                  
                                                  
                                            NORMAL`,
				},
			}

			wg.Add(2)
			for _, ch := range "ggjzo" {
				mu.Lock()
				_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				mu.Unlock()
				require.True(t, handled)
			}
			wg.Wait()
			w := term.NewStringWriter(50, 10)
			mu.Lock()
			comptest.TestComponent(t, vi, w, tests)
			mu.Unlock()
		})

		t.Run("moving up and down after hidding/making visible", func(t *testing.T) {
			t.SkipNow()
			tests := []comptest.TestCase{
				{Expected: `
                                                  
 /*                                              
  * Check if the current buffer should be added t
   * diff buffers.                                
   */                                             
      void                                        
  diff_buf_adjust(win_T *win)                     
 {                                               
      win_T    *wp;                               
                                                  `,
				},
			}

			wg.Add(10)
			for _, ch := range "jjjjzAkzo" { // goes and stays at "void" line, move up again and unfold
				mu.Lock()
				_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				mu.Unlock()
				require.True(t, handled)
			}
			wg.Wait()
			w := term.NewStringWriter(50, 10)
			mu.Lock()
			comptest.TestComponent(t, vi, w, tests)
			mu.Unlock()
		})
	})
	t.Run("initial folds with initial cursor position", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.WriteString(snippet)
		fs := &testFoldsService{}
		fs.view = buf.WithView(fs)
		var wg sync.WaitGroup
		var mu sync.Mutex
		cb := func(fn func()) bool {
			// run async to guarantee we can lock below:
			// sometimes this cb can run in the main goroutine
			go func() {
				defer wg.Done()
				mu.Lock()
				defer mu.Unlock()
				fn()
			}()
			return true
		}
		wg.Add(2)
		mu.Lock()
		cfg := text.StatusBarConfig{
			Publisher: &texttest.TestEditor{},
			ScheduleNextTick: func(cb func()) bool {
				cb()
				return true
			},
		}
		vi := New(buf, uri,
			WithHideInitialFolds(true),
			WithAutoCenter(true),
			WithScheduleNextTick(cb),
		)
		bar := text.WithStatusBar(vi, buf, vi.less.Scroll(),
			false, false, cfg)
		bar.Resize(20, 10)
		require.True(t, vi.SetCursorAtScroll(term.Coordinates{Y: 7}))
		mu.Unlock()

		tests := []comptest.TestCase{
			{Expected: `
                    
/* [4 lines] */     
    void            
diff_buf_adjust(win_
{                   
    win_T    *wp;   
    int             
                    
    if (!win->w_p_di
                    `,
			},
		}

		wg.Wait()
		w := term.NewStringWriter(20, 10)

		mu.Lock()
		defer mu.Unlock()

		comptest.TestComponent(t, bar, w, tests)
		pos := vi.CursorAtScroll()
		assert.Equal(t, pos, term.Coordinates{Y: 7})
	})
}

func TestSetCursorAtScrollCenter(t *testing.T) {
	buf := cell.NewBuffer()
	buf.WriteString(snippet)
	vi := New(buf, uri,
		WithAutoCenter(true),
	)
	vi.Resize(20, 10)
	vi.SetCursorAtScroll(term.Coordinates{Y: 10, X: 5})

	cases := []handlertest.SequenceTestCase{
		{"",
			`    void            
diff_buf_adjust(win_
{                   
    win_T    *wp;   
    int             
▐                   
    if (!win->w_p_di
    {               
    /* When there is
     * it from the d`},
		{"j", // free mark works after SetCursorAtScroll
			`    void            
diff_buf_adjust(win_
{                   
    win_T    *wp;   
    int             
                    
   ▐if (!win->w_p_di
    {               
    /* When there is
     * it from the d`},
	}

	handlertest.TestHandlerSequence(t, vi, 20, 10, cases)
}

type mockHandler struct {
	texttest.MockHandler
	h viHandlerImpl // used for mode parsing only

	received []term.Event
}

func newMockHandler(buf *cell.Buffer) (ret *mockHandler) {
	ret = new(mockHandler)
	config := defaultviHandlerImplConfig()
	ret.h.init(buf, config)
	return ret
}

func (h *mockHandler) setStatusBar(bar statusBar) {
}

func (h *mockHandler) Resize(width, height int) {
	h.h.Resize(width, height)
}

func (h *mockHandler) setNormalMode() bool {
	return false
}

func (h *mockHandler) unselect() bool {
	return false
}

func (h *mockHandler) search(string) {
}

func (h *mockHandler) Handle(ev term.Event) (bool, bool) {
	h.received = append(h.received, ev)
	switch ev.Ch {
	case '#': // map for convenience
		ev = term.Event{Type: term.EventKey, Key: term.KeyEsc}
	case '&':
		ev = term.Event{Type: term.EventKey, Key: term.KeyEnter}
	}
	h.h.Handle(ev)
	return false, true
}

func (h *mockHandler) mode() viMode {
	return h.h.mode()
}
func (h *mockHandler) moveToNextLocation(ID string) bool {
	return false
}
func (h *mockHandler) moveToPrevLocation(ID string) bool {
	return false
}
func (h *mockHandler) setLocationList(pri textapi.LocationPriority, ID string, l text.LocationList) {
}
func (h *mockHandler) moveToBounds() {
}
func (h *mockHandler) setCursorAtScroll(pos term.Coordinates) bool {
	return false
}
func (h *mockHandler) cursorAtScroll() term.Coordinates {
	return term.Coordinates{}
}

func TestViHandle100(t *testing.T) {
	testViHandleSize(t, 100, 99)
}

func TestViHandle10(t *testing.T) {
	testViHandleSize(t, 10, 9)
}

func TestViHandle5(t *testing.T) {
	testViHandleSize(t, 5, 4)
}

func testViHandleSize(t *testing.T, width, height int) {
	tsuite := []struct {
		desc string
		in   string
		want string
	}{
		{
			desc: "delegates events to underlying handler in normal mode",
			in:   "j",
			want: "j",
		},
		{
			desc: "does not propagate normal events upon call to repeat",
			in:   "j.",
			want: "j",
		},
		{
			desc: "repeats update events upon call to repeat",
			in:   "jia#...",
			want: "jia#ia#ia#ia#",
		},
		{
			desc: "repeats insert events upon call to repeat",
			in:   "ia#.io#.",
			want: "ia#ia#io#io#",
		},
		{
			desc: "repeats delete events upon call to repeat",
			in:   "dd.",
			want: "dddd",
		},
		{
			desc: "repeats select + delete events upon call to repeat",
			in:   "jjjvllld..",
			want: "jjjvllldvllldvllld",
		},
		{
			desc: "repeats replace event upon call to repeat",
			in:   "jrl..",
			want: "jrlrlrl",
		},
		{
			desc: "repeats normal update events upon call to repeat",
			in:   ">...",
			want: ">>>>",
		},
		{
			desc: "does no repeat undo",
			in:   ">u...",
			want: ">>>>",
		},
		{
			desc: "does not repeat search events",
			in:   ">/hello#.",
			want: ">/hello#>",
		},
		{
			desc: "repeats select + insert events upon call to repeat",
			in:   "jlvllchello#h.",
			want: "jlvllchello#hvllchello#",
		},
		{
			desc: "repeats delete a word to insert events",
			in:   "jjwcwhello#b.",
			want: "jjwcwhello#bcwhello#",
		},
		{
			desc: "does not capture combination if there was no update",
			in:   "jjj>i#.",
			want: "jjj>i#>",
		},
		{
			desc: "handles search mode correctly",
			in:   "/put&>i#/put&.",
			want: "/put&>i#/put&>",
		},
		{
			desc: "propagates '.' in search mode",
			in:   "/.&>i#/.&.",
			want: "/.&>i#/.&>",
		},
		{
			desc: "does not repeat select events that did not wind up updating",
			in:   ">jjvlllll#..ihell#.",
			want: ">jjvlllll#>>ihell#ihell#",
		},
		{
			desc: "has infinite loop repeat protection",
			in:   "jjjjjjjjjjjjjjdf.u+udf.uu++.+",
			want: "jjjjjjjjjjjjjjdf.df.f.",
		},
	}

	testHandle := func(t *testing.T, in, want string) {
		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(snippet))
		vi := New(buf, uri)
		mock := newMockHandler(buf)
		vi.handler = mock
		vi.Resize(width, height)
		for _, ch := range in {
			var ev term.Event
			if ch == '+' {
				ev = term.Event{Type: term.EventKey, Ch: 'r', Mod: term.ModCtrl}
			} else {
				ev = term.Event{Type: term.EventKey, Ch: ch}
			}
			vi.Handle(ev)
		}
		var received strings.Builder
		for _, ev := range mock.received {
			received.WriteRune(ev.Ch)
		}
		assert.Equal(t, want, received.String())
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			testHandle(t, tcase.in, tcase.want)
		})
	}
}

func TestUndo100(t *testing.T) {
	testUndoSize(t, 100, 99)
}
func TestUndo10(t *testing.T) {
	testUndoSize(t, 10, 9)
}
func TestUndo5(t *testing.T) {
	testUndoSize(t, 5, 4)
}

func testUndoSize(t *testing.T, width, height int) {
	const undoFortune = `Love in your heart wasn't put there to stay.
Love isn't love 'til you give it away.
		-- Oscar Hammerstein 中国`
	suite := []struct {
		name string
		cmd  string
	}{
		{"Insert", "jji\t"},
		{"InsertRowAt", "ji\n"},
		{"DeleteCell", "jjllllx"},
		{"ConflateRow", "ggJ"},
		{"TruncateRowFrom", "jlD"},
		{"TruncateFrom", "lllllldG"},
		{"DeleteRow", "dd"},
		{"Edit which effectively replaces", "jjlvllllchello"},
		{"Repeat", "jji\t#..."},
		{"ReplaceAll", "kkcGhello\nworld"},
		{"InsertRowBelow", "Gohello"},
	}

	for _, _tcase := range suite {
		tcase := _tcase
		t.Run(fmt.Sprintf("undo %s", tcase.name), func(t *testing.T) {
			buf := cell.NewBuffer()
			buf.ReadFrom(strings.NewReader(undoFortune))

			vi := New(buf, uri)
			vi.Resize(width, height)

			for i := 0; i < 5; i++ {
				for _, ch := range tcase.cmd {
					ev := term.Event{Type: term.EventKey, Ch: ch}
					vi.Handle(ev)
				}
				assert.NotEqual(t, undoFortune, buf.String())
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
				quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
				assert.False(t, quit)
				assert.True(t, handled)
			}

			assert.Equal(t, undoFortune, buf.String())
		})
	}

	t.Run("undo/redo repeats", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(undoFortune))
		vi := New(buf, uri)
		vi.Resize(width, height)

		for _, ch := range "iasdfgh#.." {
			if ch == '#' {
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
			} else {
				vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			}
		}
		assert.NotEqual(t, undoFortune, buf.String())
		for i := 0; i < 3; i++ {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
			assert.False(t, quit, i)
			assert.True(t, handled, i)
		}
		assert.Equal(t, undoFortune, buf.String())
		for i := 0; i < 3; i++ {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'r'})
			assert.False(t, quit, i)
			assert.True(t, handled, i)
		}
		assert.NotEqual(t, undoFortune, buf.String())
		for i := 0; i < 3; i++ {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
			assert.False(t, quit, i)
			assert.True(t, handled, i)
		}
		assert.Equal(t, undoFortune, buf.String())
	})

	t.Run("undo/redo oob edits", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(undoFortune))
		vi := New(buf, uri)
		vi.Resize(width, height)

		buf.Edit(context.Background(), term.Coordinates{}, term.Coordinates{}, "abc")
		assert.Equal(t, "abc"+undoFortune, buf.String())

		quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
		assert.False(t, quit)
		assert.True(t, handled)
		assert.Equal(t, undoFortune, buf.String())

		quit, handled = vi.Handle(term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'r'})
		assert.False(t, quit)
		assert.True(t, handled)
		assert.Equal(t, "abc"+undoFortune, buf.String())

		quit, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
		assert.False(t, quit)
		assert.True(t, handled)
		assert.Equal(t, undoFortune, buf.String())
	})

	t.Run("undo/redo oob edits with regular edits", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(undoFortune))
		vi := New(buf, uri)
		vi.Resize(width, height)

		buf.Edit(context.Background(), term.Coordinates{}, term.Coordinates{}, "abc")
		assert.Equal(t, "abc"+undoFortune, buf.String())
		for _, ch := range "iasdfgh#" {
			if ch == '#' {
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
			} else {
				vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			}
		}
		assert.Equal(t, "asdfghabc"+undoFortune, buf.String())

		for i := 0; i < 2; i++ {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
			assert.False(t, quit)
			assert.True(t, handled)
		}
		assert.Equal(t, undoFortune, buf.String())

		for i := 0; i < 2; i++ {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'r'})
			assert.False(t, quit)
			assert.True(t, handled)
		}
		assert.Equal(t, "asdfghabc"+undoFortune, buf.String())

		for i := 0; i < 2; i++ {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
			assert.False(t, quit)
			assert.True(t, handled)
		}
		assert.Equal(t, undoFortune, buf.String())
	})

	t.Run("undo/redo a series of updates", func(t *testing.T) {
		buf := cell.NewBuffer()
		buf.ReadFrom(strings.NewReader(undoFortune))
		prev := buf.String()

		vi := New(buf, uri)
		vi.Resize(width, height)

		for _, tcase := range suite {
			for _, ch := range tcase.cmd {
				ev := term.Event{Type: term.EventKey, Ch: ch}
				vi.Handle(ev)
			}
			vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
		}

		middle := buf.String()

		for i := range suite {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
			assert.False(t, quit, i)
			assert.True(t, handled, i)
		}

		after := buf.String()
		assert.Equal(t, prev, after)

		for i := range suite {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'r'})
			assert.False(t, quit, i)
			assert.True(t, handled, i)
		}

		afterRedo := buf.String()
		assert.Equal(t, middle, afterRedo)

		for i := range suite {
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
			assert.False(t, quit, i)
			assert.True(t, handled, i)
		}

		after = buf.String()
		assert.Equal(t, prev, after)
	})
}

func TestMoveAfterClick(t *testing.T) {
	width, height := 20, 10

	buf := cell.NewBuffer()
	buf.ReadFrom(strings.NewReader(snippet))
	vi := New(buf, uri)
	vi.Resize(width, height)

	for _, r := range "jjjjjjj" {
		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: r})
		require.True(t, handled)
	}
	pos, _, ok := vi.Cursor()
	require.True(t, ok)
	assert.Equal(t, term.Coordinates{Y: 7}, pos)

	_, handled := vi.Handle(term.Event{
		Type:   term.EventMouse,
		Key:    term.MouseLeft,
		MouseY: 2,
		MouseX: 0,
	})
	pos, _, ok = vi.Cursor()
	require.True(t, ok)
	require.Equal(t, term.Coordinates{Y: 2, X: 0}, pos)
	require.True(t, handled)

	for _, r := range "jk" {
		_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: r})
		require.True(t, handled)
	}
	pos, _, ok = vi.Cursor()
	require.True(t, ok)
	assert.Equal(t, term.Coordinates{Y: 2, X: 0}, pos)
}

func TestMatchBraceAfterClick(t *testing.T) {
	width, height := 20, 10

	buf := cell.NewBuffer()
	buf.ReadFrom(strings.NewReader(snippet))
	vi := New(buf, uri)
	vi.Resize(width, height)

	_, handled := vi.Handle(term.Event{
		Type:   term.EventMouse,
		Key:    term.MouseLeft,
		MouseY: 7,
		MouseX: 0,
	})
	pos := vi.CursorAtScroll()
	require.Equal(t, term.Coordinates{Y: 7, X: 0}, pos)
	require.True(t, handled)

	_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: '%'})
	require.True(t, handled)
	pos = vi.CursorAtScroll()
	assert.Equal(t, term.Coordinates{Y: 31, X: 0}, pos)

	_, handled = vi.Handle(term.Event{Type: term.EventKey, Ch: '%'})
	require.True(t, handled)
	pos = vi.CursorAtScroll()
	assert.Equal(t, term.Coordinates{Y: 7, X: 0}, pos)
}

func TestCopyDelete(t *testing.T) {
	tsuite := []struct {
		desc     string
		content  string
		in       string
		wantCopy string
	}{
		{"copies delete data", "a", "x", "a"},
		{"does not copy insert data", "a", "ihello", ""},
		{"does not copy undo of an insert", "", "ihello#u", ""},
		{"does not copy undo of an insert (preserve old data)", "a", "xihello#u", "a"},
		{"copies remove portion of a delete select", "a", "clb", "a"},
		{"copies redo of a delete (or leaves previous delete)", "a", "xuR", "a"},
		{"does not copy backspace within an insert", "x", "ddihella<o#", "x"},
		{"does copy text deleted through C", "x", "Ca#", "x"},
		{"does copy text deleted through c", "x", "cla#", "x"},
		{"does copy text deleted through cc", "x\ny", "ccz#", "x"},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			buf := cell.NewBuffer()
			buf.ReadFrom(strings.NewReader(tcase.content))

			mock := new(mockClip)
			vi := New(buf, uri, WithClipboard(mock))
			vi.Resize(10, 10)

			for _, ch := range tcase.in {
				ev := term.Event{Type: term.EventKey, Ch: ch}
				if ch == '#' {
					ev = term.Event{Type: term.EventKey, Key: term.KeyEsc}
				} else if ch == '<' {
					ev = term.Event{Type: term.EventKey, Key: term.KeyBackspace}
				} else if ch == 'R' {
					ev = term.Event{Type: term.EventKey, Mod: term.ModCtrl, Ch: 'r'}
				}
				vi.Handle(ev)
			}

			assert.Equal(t, tcase.wantCopy, mock.data.Text)
		})
	}
}

var _ = (foldsService)(testFoldsService{})

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
		{Start: term.Coordinates{Y: 1, X: 0}, End: term.Coordinates{Y: 4}},
		{Start: term.Coordinates{Y: 2, X: 3}, End: term.Coordinates{Y: 3, X: 15}},
		{Start: term.Coordinates{Y: 7, X: 0}, End: term.Coordinates{Y: 31, X: 0}},
		{Start: term.Coordinates{Y: 12, X: 3}, End: term.Coordinates{Y: 28, X: 3}},
		{Start: term.Coordinates{Y: 19, X: 6}, End: term.Coordinates{Y: 27, X: 6}},
		{Start: term.Coordinates{Y: 22, X: 6}, End: term.Coordinates{Y: 26, X: 9}},
		{Start: term.Coordinates{Y: 29, X: 3}, End: term.Coordinates{Y: 30, X: 3}},
	}), true
}

func (f testFoldsService) InitialFolds() (iterator.Iterator[term.Range], bool) {
	return iterator.FromSlice([]term.Range{
		{Start: term.Coordinates{Y: 1, X: 0}, End: term.Coordinates{Y: 4}},
	}), true
}

type mockClip struct {
	data clipboard.Data
}

func (m *mockClip) Paste(registerID string) (clipboard.Data, error) {
	return m.data, nil
}

func (m *mockClip) Copy(registerID string, data clipboard.Data) error {
	m.data = data
	return nil
}
