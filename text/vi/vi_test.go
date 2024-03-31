package vi

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	textapi "unstable.build/go-tui/api/text"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
	texttest "unstable.build/go-tui/text/test"
)

var uri workspaceapi.URI

func init() {
	var err error
	uri, err = workspaceapi.ParseURI("file:///vi_test")
	if err != nil {
		panic(err)
	}
}

type mockHandler struct {
	texttest.MockHandler
	h viHandlerImpl // used for mode parsing only

	received []term.Event
}

func newMockHandler(buf *cell.Buffer) (ret *mockHandler) {
	ret = new(mockHandler)
	ret.h.init(buf)
	return ret
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
func (h *mockHandler) moveToNextLocation(ID string) {
}
func (h *mockHandler) moveToPrevLocation(ID string) {
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
	testViHandleSize(t, 100, 100)
}

func TestViHandle10(t *testing.T) {
	testViHandleSize(t, 10, 10)
}

func TestViHandle5(t *testing.T) {
	testViHandleSize(t, 5, 5)
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
				ev = term.Event{Type: term.EventKey, Key: term.KeyCtrlR}
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
	testUndoSize(t, 100, 100)
}
func TestUndo10(t *testing.T) {
	testUndoSize(t, 10, 10)
}
func TestUndo5(t *testing.T) {
	testUndoSize(t, 5, 5)
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
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyCtrlR})
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

		quit, handled = vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyCtrlR})
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
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyCtrlR})
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
			quit, handled := vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyCtrlR})
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
					ev = term.Event{Type: term.EventKey, Key: term.KeyCtrlR}
				}
				vi.Handle(ev)
			}

			assert.Equal(t, tcase.wantCopy, mock.data.Text)
		})
	}
}
