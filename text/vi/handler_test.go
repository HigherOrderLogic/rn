package vi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	testutil "unstable.build/go-tui/util/test"
)

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

func setupVi(
	t *testing.T, text string, tabspaces int, opts ...Option,
) *viHandlerImpl {
	buf := cell.NewBuffer()
	buf.InitWithTabspaces(tabspaces)
	_, err := buf.ReadFrom(strings.NewReader(text))
	require.NoError(t, err)

	vi := new(viHandlerImpl)
	vi.init(buf, opts...)

	return vi
}

func setupViIntegration(
	t *testing.T, text string, tabspaces int, opts ...Option,
) tui.Handler {
	buf := cell.NewBuffer()
	buf.InitWithTabspaces(tabspaces)
	_, err := buf.ReadFrom(strings.NewReader(text))
	require.NoError(t, err)

	vi := New(buf, uri, opts...)

	return vi
}

func TestCellAtCursor(t *testing.T) {
	cases := []struct {
		input string
		cell  rune
	}{
		{"k", '\x00'},
		{"j", '/'},
		{"l", '*'},
		{"$", '*'},
	}

	width, height := 20, 10

	writer := term.NewStringWriter(width, height)
	vi := setupVi(t, snippet, 2)
	vi.Resize(width, height)

	for _, tcase := range cases {
		for _, r := range tcase.input {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: r})
			require.True(t, handled)

			vi.Draw(writer)

			err := writer.Flush()
			require.NoError(t, err)
		}
		c, ok := vi.cursor.Cell()
		if tcase.cell == '\x00' {
			assert.False(t, ok)
		} else {
			assert.True(t, ok)
			assert.Equal(t, tcase.cell, c.Ch)
		}
	}
}

func TestViIntegrationSequence(t *testing.T) {
	cases := []testutil.HandlerSequenceTestCase{
		{"",
			`▐                   
/*                  
 * Check if the curr
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		{"jjjj",
			`                    
/*                  
 * Check if the curr
 * diff buffers.    
▐*/                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		{"/NULL>jjjjjjjjkkkkkkkk",
			`  if (wp == ▐ULL)   
  {                 
    i = diff_buf_idx
    if (i != DB_COUN
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
    }               
    searching 'NULL'`},
		{"Ahello",
			`f (wp == NULL)hello▐
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              INSERT`},
		{"<hhhhC<",
			`f (wp == NULL▐      
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"p",
			`f (wp == NULL)▐ello 
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"F(",
			`f ▐wp == NULL)hello 
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"f)",
			`f (wp == NULL▐hello 
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"F=",
			`f (wp =▐ NULL)hello 
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{",",
			`f (wp ▐= NULL)hello 
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{";",
			`f (wp =▐ NULL)hello 
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"D",
			`f (wp ▐             
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"sbrillo",
			`f (wp brillo▐       
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              INSERT`},
		{"<hhhhhhR == NULL)", // FIXME
			`f (wp == NULL)▐     
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
             REPLACE`},
		{"<r]h",
			`f (wp == NUL▐]      
                    
 i = diff_buf_idx(wi
 if (i != DB_COUNT) 
 {                  
 curtab->tp_diffbuf[
 curtab->tp_diff_inv
 diff_redraw(TRUE); 
 }                  
              NORMAL`},
		{"Vyp",
			`  if (wp == NULL]   
 ▐if (wp == NULL]   
  {                 
    i = diff_buf_idx
    if (i != DB_COUN
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
              NORMAL`},
		{"/i =>",
			`  if (wp == NULL]   
  if (wp == NULL]   
  {                 
    ▐ = diff_buf_idx
    if (i != DB_COUN
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
     searching 'i ='`},
		{"dd",
			`  if (wp == NULL]   
  if (wp == NULL]   
  {                 
▐   if (i != DB_COUN
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
    }               
              NORMAL`},
		{"h",
			`  if (wp == NULL]   
  if (wp == NULL]   
  {                 
▐   if (i != DB_COUN
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
    }               
              NORMAL`},
		{"h",
			`  if (wp == NULL]   
  if (wp == NULL]   
  {                 
▐   if (i != DB_COUN
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
    }               
              NORMAL`},
		{"df=",
			`  if (wp == NULL]   
  if (wp == NULL]   
  {                 
▐DB_COUNT)          
    {               
    curtab->tp_diffb
    curtab->tp_diff_
    diff_redraw(TRUE
    }               
              NORMAL`},
		{"/i>kkFDcndi<ldw",
			`  if (wp == NULL]   
  if (wp == NULL]   
  {                 
di▐urtab->tp_diff_in
    diff_redraw(TRUE
    }               
  }                 
  }                 
  else              
              NORMAL`},
		{"gg",
			`▐                   
/*                  
 * Check if the curr
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		{"jjyyp",
			`                    
/*                  
 * Check if the curr
▐* Check if the curr
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
              NORMAL`},
		{"lllcc *",
			`                    
/*                  
 * Check if the curr
 *▐                 
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
              INSERT`},
	}

	vi := setupViIntegration(t, snippet, 2)
	testutil.TestHandlerSequence(t, vi, 20, 10, cases)
}

func TestVidfd(t *testing.T) {
	cases := []testutil.HandlerSequenceTestCase{
		{"jjdfd",
			`                    
/*                  
▐be added to or remo
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		{"jjcfc",
			`                    
/*                  
▐ if the current buf
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              INSERT`},
	}

	newVi := func(t *testing.T) tui.Handler {
		return setupVi(t, snippet, 2)
	}
	testutil.TestHandlerIsolated(t, newVi, 20, 10, cases)
}

func TestViDeleteAWord(t *testing.T) {
	cases := []testutil.HandlerSequenceTestCase{
		{"jjjjjwdw",
			`                    
/*                  
 * Check if the curr
 * diff buffers.    
 */                 
 ▐                  
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		{"jjjjjwcw",
			`                    
/*                  
 * Check if the curr
 * diff buffers.    
 */                 
  ▐                 
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              INSERT`},
		{"jjjjjwce",
			`                    
/*                  
 * Check if the curr
 * diff buffers.    
 */                 
  ▐                 
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              INSERT`},
		{"jjjjjwecb",
			`                    
/*                  
 * Check if the curr
 * diff buffers.    
 */                 
  ▐                 
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              INSERT`},
		{"jjjwwcw",
			`                    
/*                  
 * Check if the curr
 * ▐uffers.         
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              INSERT`},
		{"jjjwwce",
			`                    
/*                  
 * Check if the curr
 * ▐buffers.        
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              INSERT`},
	}

	newVi := func(t *testing.T) tui.Handler {
		return setupVi(t, snippet, 2)
	}
	testutil.TestHandlerIsolated(t, newVi, 20, 10, cases)
}

func TestViCursorIsolated(t *testing.T) {
	cases := []testutil.HandlerSequenceTestCase{
		{"jjddp",
			`                    
/*                  
 * diff buffers.    
▐* Check if the curr
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		// insert block one rune (for now until repeater captures all insert)
		{"`jjjIh<",
			`h                   
h/*                 
h * Check if the cur
h▐* diff buffers.   
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		{"/C>/>",
			`                    
/*                  
 * ▐heck if the curr
 * diff buffers.    
 */                 
  void              
diff_buf_adjust(win_
{                   
  win_T  *wp;       
              NORMAL`},
		// TODO check yank paste after last line
		// the only thing from integration tests is that there's no
		// unix View that trims last EOL, this must in turn translatre in
		// some internal difference which renders this test failure
		/*{"Gyyp",
					`    curtab->tp_diff_
		    diff_redraw(TRUE
		    }
		  }
		  }
		  else
		  diff_buf_add(win->
		}
		▐
		              NORMAL`}, */
	}

	newVi := func(t *testing.T) tui.Handler {
		return setupViIntegration(t, snippet, 2)
	}
	testutil.TestHandlerIsolated(t, newVi, 20, 10, cases)
}

func TestIntegrationScrollEvent(t *testing.T) {
	tsuite := []struct {
		desc      string
		cursorPos term.Coordinates
		ev        term.Event
	}{
		{"MoveToMatchingRune", term.Coordinates{Y: 7}, term.Event{Type: term.EventKey, Ch: '%'}},
		{"MoveEndLine", term.Coordinates{Y: 2}, term.Event{Type: term.EventKey, Ch: '$'}},
		{"MoveRightStartWord", term.Coordinates{X: 4, Y: 9}, term.Event{Type: term.EventKey, Ch: 'w'}},
		{"MoveLeftStartWord", term.Coordinates{X: 21, Y: 9}, term.Event{Type: term.EventKey, Ch: 'B'}},
	}

	for _, tcase := range tsuite {
		tcase := tcase
		t.Run(tcase.desc, func(t *testing.T) {
			vi := setupVi(t, snippet, 2)
			vi.cursor.Insert('a')
			vi.setCursorAtScroll(tcase.cursorPos)
			vi.Resize(4, 4)

			var called int
			vi.less.Scroll().Subscribe(component.FuncScrollSubscriber(func(from, to term.Coordinates) {
				called++
			}))

			_, ok := vi.Handle(tcase.ev)
			assert.True(t, ok)
			assert.Equal(t, 1, called)
		})
	}
}

func TestIntegrationNewFile(t *testing.T) {
	vi := setupVi(t, "", 2)
	vi.Resize(4, 4)
	for _, ch := range "ihello\nworld" {
		vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
	}
	assert.Equal(t, "hello\nworld", vi.less.Buffer().String())
}
