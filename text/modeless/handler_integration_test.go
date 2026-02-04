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

package modeless

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

func TestFoldsIntegration(t *testing.T) {
	uri, err := workspaceapi.ParseURI("file:///vi_test")
	require.NoError(t, err)

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
		wg.Add(1)
		mu.Lock()
		h := NewHandler(buf, uri,
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
		bar := text.WithStatusBar(h, buf, h.(*editorHandler).less.Scroll(),
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
		h, err := ed.Edit(uri, buf, false, false)
		require.NoError(t, err)
		h.Resize(50, 10)
		mu.Unlock()
		wg.Wait() // wait for bar

		t.Run("zA", func(t *testing.T) {
			tests := []comptest.TestCase{
				{Expected: `
                                                  
 /* [4 lines] */                                 
      void                                        
  diff_buf_adjust(win_T *win)                     
 { [25 lines] }                                  
                                                  
                                                  
                                                  
                                                  
                                                  `,
				},
			}

			wg.Add(3)
			mu.Lock()
			_, handled := h.Handle(term.Event{Type: term.EventKey,
				Mod: term.ModMeta, Key: term.KeyArrowDown})
			require.True(t, handled)
			_, handled = h.Handle(term.Event{Type: term.EventKey,
				Mod: term.ModCtrl, Ch: 'A'})
			require.True(t, handled)
			_, handled = h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowUp})
			require.True(t, handled)
			mu.Unlock()
			wg.Wait()
			w := term.NewStringWriter(50, 10)
			mu.Lock()
			comptest.TestComponent(t, h, w, tests)
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
                                                  
                                                  `,
				},
			}

			wg.Add(1)
			mu.Lock()
			_, handled := h.Handle(term.Event{Type: term.EventKey,
				Mod: term.ModMeta, Key: term.KeyArrowUp})
			require.True(t, handled)
			_, handled = h.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowDown})
			require.True(t, handled)
			_, handled = h.Handle(term.Event{Type: term.EventKey,
				Mod: term.ModCtrl, Ch: 'Z'})
			mu.Unlock()
			wg.Wait()
			w := term.NewStringWriter(50, 10)
			mu.Lock()
			comptest.TestComponent(t, h, w, tests)
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
		wg.Add(1)
		mu.Lock()
		cfg := text.StatusBarConfig{
			Publisher: &texttest.TestEditor{},
			ScheduleNextTick: func(cb func()) bool {
				cb()
				return true
			},
		}
		h := NewHandler(buf, uri,
			WithHideInitialFolds(true),
			WithAutoCenter(true),
			WithScheduleNextTick(cb),
		)
		bar := text.WithStatusBar(h, buf, h.(*editorHandler).less.Scroll(),
			false, false, cfg)
		bar.Resize(20, 10)
		require.True(t, h.SetCursorAtScroll(term.Coordinates{Y: 7}))
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
		pos := h.CursorAtScroll()
		assert.Equal(t, pos, term.Coordinates{Y: 7})
	})
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

const snippet = `
/*
 * Check if the current buffer should be added to or removed from the list of
 * diff buffers.
 */
	void
diff_buf_adjust(win_T *win)
{
	win_T	*wp;
	int				i;

	if (!win->w_p_diff)
	{
	/* When there is no window showing a diff for this buffer, remove
	 * it from the diffs... */
	FOR_ALL_WINDOWS(wp)
		if (wp->w_buffer == win->w_buffer && wp->w_p_diff)
		break;
	if (wp == NULL)
	{
		i = diff_buf_idx(win->w_buffer);
		if (i != DB_COUNT)
		{
		curtab->tp_diffbuf[i] = NULL;
		curtab->tp_diff_invalid = TRUE;
		diff_redraw(TRUE);
		}
	}
	}
	else
	diff_buf_add(win->w_buffer);
}`
