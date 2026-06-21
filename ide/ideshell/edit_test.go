// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package ideshell

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/modeless"
)

// stubEditor is a minimal command.Editor for tests. It records every
// event delivered to its EditHandler and lets the test invoke an
// optional handle function to mutate the buffer (e.g. append a rune)
// the way a real editor would.
type stubEditor struct {
	seen *[]term.Event
	// initialCursor is recorded by SetCursorAtScroll so tests can
	// assert the host seeded the cursor correctly.
	initialCursor *term.Coordinates
}

func (s stubEditor) Edit(buf *cell.Buffer) command.EditHandler {
	return &stubEditHandler{
		buf:           buf,
		seen:          s.seen,
		initialCursor: s.initialCursor,
	}
}

type stubEditHandler struct {
	buf           *cell.Buffer
	seen          *[]term.Event
	initialCursor *term.Coordinates
	width         int
}

func (s *stubEditHandler) Resize(width, _ int) { s.width = width }
func (s *stubEditHandler) Draw(w term.Writer) {
	x, y := 0, 0
	for _, r := range s.buf.String() {
		if s.width > 0 && x >= s.width {
			x = 0
			y++
		}
		w.SetCell(term.Coordinates{X: x, Y: y}, term.Cell{Ch: r})
		x++
	}
}
func (s *stubEditHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{X: s.buf.Columns(0)},
		term.CursorStyleSteadyBar, true
}
func (s *stubEditHandler) CursorAtScroll() term.Coordinates {
	return term.Coordinates{X: s.buf.Columns(0)}
}
func (s *stubEditHandler) SetCursorAtScroll(pos term.Coordinates) bool {
	if s.initialCursor != nil {
		*s.initialCursor = pos
	}
	return true
}
func (s *stubEditHandler) Selection() (string, bool) { return "", false }
func (s *stubEditHandler) Handle(ev term.Event) (bool, bool) {
	if s.seen != nil {
		*s.seen = append(*s.seen, ev)
	}
	if ev.Type != term.EventKey {
		return false, false
	}
	// A real vi/modeless editor consumes <tab> (it inserts
	// indentation). The shell wrapper must therefore intercept <tab>
	// before delegating, the same way a VTE owns <tab> and never
	// forwards it to the running program.
	if ev.Key == term.KeyTab && ev.Mod == 0 {
		s.buf.WriteString("\t")
		return false, true
	}
	// vi's insert mode consumes <c-r> (insert register). The shell
	// wrapper must therefore intercept <c-r> for reverse-history
	// search before delegating, the same way a VTE owns <c-r>.
	if ev.Mod == term.ModCtrl && ev.Ch == 'r' {
		return false, true
	}
	// A real editor consumes arrow keys (and vi-style <c-j>/<c-k>) as
	// cursor motion. The shell wrapper must intercept these for
	// history cycling before the editor sees them.
	if ev.Mod == 0 &&
		(ev.Key == term.KeyArrowUp || ev.Key == term.KeyArrowDown) {
		return false, true
	}
	if ev.Mod == term.ModCtrl && (ev.Ch == 'j' || ev.Ch == 'k') {
		return false, true
	}
	switch ev.Key {
	case term.KeyBackspace:
		cols := s.buf.Columns(0)
		if cols > 0 {
			s.buf.DeleteCell(term.Coordinates{X: cols - 1})
		}
		return false, true
	case term.KeySpace:
		s.buf.WriteString(" ")
		return false, true
	}
	if ev.Ch != 0 {
		s.buf.WriteString(string(ev.Ch))
		return false, true
	}
	return false, false
}

var _ command.EditHandler = (*stubEditHandler)(nil)

func newEditTestHandler(t *testing.T, editor command.Editor) *Handler {
	t.Helper()
	h, _ := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		editor,
		Config{
			MaxHistory: 100,
		},
	)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func feedRunes(h *Handler, seq string) {
	for _, c := range seq {
		h.Handle(term.Event{Type: term.EventKey, Ch: c})
	}
}

// renderFrame draws the handler to a string buffer of its current size
// and returns the rendered text, one row per line.
func renderFrame(t *testing.T, h *Handler) string {
	t.Helper()
	w := term.NewStringWriter(h.width, h.height)
	h.Draw(w)
	require.NoError(t, w.Flush())
	return w.String()
}

func TestSignatureHelpHintOnOpenParen(t *testing.T) {
	helper := &sigHelperCmd{label: "Println(a ...any) (n int, err error)", ok: true}
	h, _ := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{},
		Config{MaxHistory: 100, DisableShellInterpreter: helper},
	)
	t.Cleanup(func() { _ = h.Close() })
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "fmt.Println(")
	require.True(t, h.sigActive, "hint should activate after typing (")
	require.Equal(t, "fmt.Println(", helper.gotLine)
	require.Equal(t, len("fmt.Println("), helper.gotCol)
	require.Contains(t, renderFrame(t, h), "Println(a ...any)")

	// Typing ) dismisses the hint immediately.
	h.Handle(term.Event{Type: term.EventKey, Ch: ')'})
	require.False(t, h.sigActive)
	require.NotContains(t, renderFrame(t, h), "Println(a ...any)")
}

func TestSignatureHelpHintNeverStealsKeys(t *testing.T) {
	helper := &sigHelperCmd{label: "f(x int)", ok: true}
	h, _ := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{},
		Config{MaxHistory: 100, DisableShellInterpreter: helper},
	)
	t.Cleanup(func() { _ = h.Close() })
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "f(")
	// The "(" was consumed by the editor as a normal rune, not stolen
	// by the hint machinery, so it appears in the buffer.
	require.Equal(t, "f(", h.editBuf.String())
	require.False(t, h.searching, "signature help must not enter search/overlay mode")
}

// newSigHelpHandler returns a handler whose fallback answers signature
// help, sized so the hint band renders.
func newSigHelpHandler(t *testing.T) *Handler {
	t.Helper()
	helper := &sigHelperCmd{label: "Println(a ...any) (n int, err error)", ok: true}
	h, _ := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{},
		Config{MaxHistory: 100, DisableShellInterpreter: helper},
	)
	t.Cleanup(func() { _ = h.Close() })
	h.Resize(testWidthH, testHeight)
	return h
}

func TestSignatureHelpHintClearsOnSubmit(t *testing.T) {
	h := newSigHelpHandler(t)
	feedRunes(h, "fmt.Println(")
	require.True(t, h.sigActive)

	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	require.False(t, h.sigActive, "submitting must clear the hint")
	require.NotContains(t, renderFrame(t, h), "Println(a ...any)")
}

func TestSignatureHelpHintClearsOnCtrlC(t *testing.T) {
	h := newSigHelpHandler(t)
	feedRunes(h, "fmt.Println(")
	require.True(t, h.sigActive)

	h.Handle(term.Event{Type: term.EventKey, Ch: 'c', Mod: term.ModCtrl})
	require.False(t, h.sigActive, "<c-c> must clear the hint")
	require.NotContains(t, renderFrame(t, h), "Println(a ...any)")
}

func TestSignatureHelpHintClearsWhenLineEmptied(t *testing.T) {
	h := newSigHelpHandler(t)
	feedRunes(h, "f(")
	require.True(t, h.sigActive)

	for range len("f(") {
		h.Handle(term.Event{Type: term.EventKey, Key: term.KeyBackspace})
	}
	require.Empty(t, h.editBuf.String())
	require.False(t, h.sigActive, "emptying the line must clear the hint")
	require.NotContains(t, renderFrame(t, h), "Println(a ...any)")
}

func TestEditorReceivesTypedRunes(t *testing.T) {
	var seen []term.Event
	h := newEditTestHandler(t, stubEditor{seen: &seen})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "hello")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeySpace})
	feedRunes(h, "world")

	// Every key reached the editor and mutated its buffer.
	assert.Equal(t, "hello world", h.editBuf.String())
	assert.Len(t, seen, 11)
}

func countInsertKeys(events []term.Event) int {
	n := 0
	for _, ev := range events {
		if ev.Type == term.EventKey && ev.Ch == 'i' && ev.Mod == 0 {
			n++
		}
	}
	return n
}

func TestModalStartInsertForwardsInsertKey(t *testing.T) {
	tests := []struct {
		name        string
		modal       bool
		startInsert bool
		wantInserts int
	}{
		{name: "modal start insert", modal: true, startInsert: true, wantInserts: 1},
		{name: "modal no start insert", modal: true, startInsert: false, wantInserts: 0},
		{name: "modeless start insert", modal: false, startInsert: true, wantInserts: 0},
		{name: "modeless no start insert", modal: false, startInsert: false, wantInserts: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []term.Event
			h, _ := New(
				func(func()) bool { return false },
				term.NopInterrupter(),
				stubEditor{seen: &seen},
				Config{
					MaxHistory:       100,
					Modal:            tt.modal,
					ModalStartInsert: tt.startInsert,
				},
			)
			t.Cleanup(func() { _ = h.Close() })
			assert.Equal(t, tt.wantInserts, countInsertKeys(seen))
		})
	}
}

func TestEditorSubmitDispatchesAndClears(t *testing.T) {
	var dispatched []string
	h, registry := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{},
		Config{MaxHistory: 100},
	)
	t.Cleanup(func() { _ = h.Close() })
	registry.Register("e", "echo", echoCmd{out: &dispatched})

	h.Resize(testWidthH, testHeight)
	feedRunes(h, "e")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	h.inner.Wait()

	require.Equal(t, []string{"e"}, dispatched)
	// Submit clears the editor buffer so the next prompt is empty.
	assert.Equal(t, "", h.editBuf.String())
}

// TestModalEnterSubmitsAndClears is a regression for the "must leave
// modal mode to submit" bug: a bare <enter> must submit even while a
// modal editor is in insert mode.
func TestModalEnterSubmitsAndClears(t *testing.T) {
	var dispatched []string
	h, registry := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{},
		Config{MaxHistory: 100, Modal: true},
	)
	t.Cleanup(func() { _ = h.Close() })
	registry.Register("e", "echo", echoCmd{out: &dispatched})

	h.Resize(testWidthH, testHeight)
	feedRunes(h, "e")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	h.inner.Wait()

	require.Equal(t, []string{"e"}, dispatched)
	assert.Equal(t, "", h.editBuf.String(),
		"modal insert-mode enter must submit and clear")
}

// TestShiftEnterInsertsNewlineNotSubmit verifies the shell owns
// <shift-enter>: it forwards a plain <enter> to the editor (newline
// insertion) instead of submitting the line.
func TestShiftEnterInsertsNewlineNotSubmit(t *testing.T) {
	var dispatched []string
	var seen []term.Event
	h, registry := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{seen: &seen},
		Config{MaxHistory: 100},
	)
	t.Cleanup(func() { _ = h.Close() })
	registry.Register("e", "echo", echoCmd{out: &dispatched})

	h.Resize(testWidthH, testHeight)
	feedRunes(h, "e")
	exit, handled := h.Handle(term.Event{
		Type: term.EventKey, Key: term.KeyEnter, Mod: term.ModShift,
	})
	h.inner.Wait()

	assert.False(t, exit)
	assert.True(t, handled, "shell owns shift-enter")
	assert.Empty(t, dispatched, "shift-enter must not submit the line")
	assert.Equal(t, "e", h.editBuf.String(),
		"shift-enter must not clear the input buffer")
	last := seen[len(seen)-1]
	assert.Equal(t, term.KeyEnter, last.Key,
		"shift-enter must forward a plain enter to the editor")
	assert.Equal(t, term.Modifier(0), last.Mod,
		"the shift modifier must be stripped before forwarding")
}

// realModelessEditor adapts a real modeless text editor to
// command.Editor, matching what production wires into the shell. The
// stub editors elsewhere in this file do not model newline insertion,
// so a real editor is required to exercise <shift-enter>.
type realModelessEditor struct{}

func (realModelessEditor) Edit(buf *cell.Buffer) command.EditHandler {
	return modeless.NewHandler(buf, workspaceapi.RandomURI("memory"),
		text.IndentRuneTab, 4, modeless.WithCommandBar(false))
}

// TestShiftEnterInsertsLiteralNewline reproduces the bug where
// <shift-enter> inserted a placeholder glyph instead of a real newline:
// driving "a", <shift-enter>, "b" through the shell must leave the
// editor buffer holding exactly "a\nb".
func TestShiftEnterInsertsLiteralNewline(t *testing.T) {
	h, _ := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		realModelessEditor{},
		Config{MaxHistory: 100},
	)
	t.Cleanup(func() { _ = h.Close() })
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "a")
	h.Handle(term.Event{
		Type: term.EventKey, Key: term.KeyEnter, Mod: term.ModShift,
	})
	feedRunes(h, "b")

	assert.Equal(t, "a\nb", h.editBuf.String(),
		"shift-enter must insert a literal newline, not a placeholder rune")
}

func TestNewPanicsWithoutEditor(t *testing.T) {
	assert.Panics(t, func() {
		_, _ = New(
			func(func()) bool { return false },
			term.NopInterrupter(),
			nil,
			Config{},
		)
	})
}

// arrowUp/arrowDown/ctrlJ/ctrlK are the history-cycling keys the shell
// wrapper must own (the editor would otherwise consume them as cursor
// motion).
var (
	arrowUp   = term.Event{Type: term.EventKey, Key: term.KeyArrowUp}
	arrowDown = term.Event{Type: term.EventKey, Key: term.KeyArrowDown}
	ctrlJ     = term.Event{Type: term.EventKey, Ch: 'j', Mod: term.ModCtrl}
	ctrlK     = term.Event{Type: term.EventKey, Ch: 'k', Mod: term.ModCtrl}
)

// TestArrowUpCyclesHistoryIntoEditor verifies that <up> recalls
// successively older history entries into the editor buffer even though
// the editor consumes arrow keys as cursor motion.
func TestArrowUpCyclesHistoryIntoEditor(t *testing.T) {
	h := newTestHandler(t, []string{"oldest", "middle", "newest"})
	h.Resize(testWidthH, testHeight)

	h.Handle(arrowUp)
	assert.Equal(t, "newest", h.editBuf.String())
	h.Handle(arrowUp)
	assert.Equal(t, "middle", h.editBuf.String())
	h.Handle(arrowUp)
	assert.Equal(t, "oldest", h.editBuf.String())
}

// TestArrowDownCyclesHistoryIntoEditor verifies that <down> walks back
// toward the most recent entry after <up> has moved into history.
func TestArrowDownCyclesHistoryIntoEditor(t *testing.T) {
	h := newTestHandler(t, []string{"oldest", "middle", "newest"})
	h.Resize(testWidthH, testHeight)

	h.Handle(arrowUp)
	h.Handle(arrowUp)
	require.Equal(t, "middle", h.editBuf.String())

	h.Handle(arrowDown)
	assert.Equal(t, "newest", h.editBuf.String())
}

// TestCtrlKCtrlJCycleHistory verifies the vi-style <c-k>/<c-j> aliases
// move up/down through history identically to the arrow keys.
func TestCtrlKCtrlJCycleHistory(t *testing.T) {
	h := newTestHandler(t, []string{"oldest", "middle", "newest"})
	h.Resize(testWidthH, testHeight)

	h.Handle(ctrlK)
	assert.Equal(t, "newest", h.editBuf.String())
	h.Handle(ctrlK)
	assert.Equal(t, "middle", h.editBuf.String())
	h.Handle(ctrlJ)
	assert.Equal(t, "newest", h.editBuf.String())
}

// TestEditingDuringHistoryCycleKeepsPosition verifies that editing a
// recalled line and pressing <up> again walks to the entry above it —
// matching how a real shell treats an edited history line as an
// in-place modification rather than resetting to the newest entry.
func TestEditingDuringHistoryCycleKeepsPosition(t *testing.T) {
	h := newTestHandler(t, []string{"oldest", "middle", "newest"})
	h.Resize(testWidthH, testHeight)

	h.Handle(arrowUp)
	h.Handle(arrowUp)
	require.Equal(t, "middle", h.editBuf.String())

	// Edit the recalled line.
	feedRunes(h, "X")
	require.Equal(t, "middleX", h.editBuf.String())

	// <up> walks to the entry above the edited line.
	h.Handle(arrowUp)
	assert.Equal(t, "oldest", h.editBuf.String())
}

// TestHistoryDownRestoresLiveEdit verifies that after typing a fresh
// line and pressing <up>, pressing <down> back past the newest entry
// restores the in-progress line the user was typing.
func TestHistoryDownRestoresLiveEdit(t *testing.T) {
	h := newTestHandler(t, []string{"newest"})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "draft")
	h.Handle(arrowUp)
	require.Equal(t, "newest", h.editBuf.String())

	h.Handle(arrowDown)
	assert.Equal(t, "draft", h.editBuf.String())
}

// multilineStubEditor is a command.Editor whose handler tracks a cursor
// row and moves it within the buffer on <up>/<down>, mirroring how a
// real editor navigates a multi-row buffer. It lets tests assert that
// the shell forwards cursor motion to the editor until a vertical edge
// is reached, only then cycling history.
type multilineStubEditor struct{ h *multilineStubHandler }

func (s *multilineStubEditor) Edit(buf *cell.Buffer) command.EditHandler {
	s.h = &multilineStubHandler{buf: buf}
	return s.h
}

type multilineStubHandler struct {
	buf   *cell.Buffer
	cy    int
	width int
}

func (s *multilineStubHandler) Resize(width, _ int) { s.width = width }
func (s *multilineStubHandler) Draw(term.Writer)    {}
func (s *multilineStubHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{Y: s.cy}, term.CursorStyleSteadyBar, true
}
func (s *multilineStubHandler) CursorAtScroll() term.Coordinates {
	return term.Coordinates{Y: s.cy}
}
func (s *multilineStubHandler) SetCursorAtScroll(pos term.Coordinates) bool {
	s.cy = pos.Y
	return true
}
func (s *multilineStubHandler) Selection() (string, bool) { return "", false }
func (s *multilineStubHandler) Handle(ev term.Event) (bool, bool) {
	if ev.Type != term.EventKey {
		return false, false
	}
	switch {
	case ev.Mod == 0 && ev.Key == term.KeyArrowUp:
		if s.cy > 0 {
			s.cy--
		}
		return false, true
	case ev.Mod == 0 && ev.Key == term.KeyArrowDown:
		if s.cy < s.buf.Rows()-1 {
			s.cy++
		}
		return false, true
	}
	return false, false
}

var _ command.EditHandler = (*multilineStubHandler)(nil)

// cursorStubEditor builds an edit handler whose buffer-relative cursor
// position is fixed by the test, so cursor-geometry assertions do not
// depend on a real editor's wrap/scroll behavior.
type cursorStubEditor struct{ cursor term.Coordinates }

func (s cursorStubEditor) Edit(buf *cell.Buffer) command.EditHandler {
	return &cursorStubHandler{buf: buf, cursor: s.cursor}
}

type cursorStubHandler struct {
	buf    *cell.Buffer
	cursor term.Coordinates
}

func (s *cursorStubHandler) Resize(int, int)  {}
func (s *cursorStubHandler) Draw(term.Writer) {}
func (s *cursorStubHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return s.cursor, term.CursorStyleSteadyBar, true
}
func (s *cursorStubHandler) CursorAtScroll() term.Coordinates        { return s.cursor }
func (s *cursorStubHandler) SetCursorAtScroll(term.Coordinates) bool { return true }
func (s *cursorStubHandler) Selection() (string, bool)               { return "", false }
func (s *cursorStubHandler) Handle(term.Event) (bool, bool)          { return false, false }

var _ command.EditHandler = (*cursorStubHandler)(nil)

// TestCursorVisualHonorsBufferLineForMultilineInput reproduces the bug
// where a multi-line input line (produced by <shift-enter>) placed the
// caret on the wrong row: the visual cursor ignored the editor's buffer
// line and folded every column onto the first row through the prompt's
// hanging indent. The prompt is "> " (2 cols) and the width is 30, so
// only buffer line 0 carries the prompt offset and continuation lines
// start at column 0.
func TestCursorVisualHonorsBufferLineForMultilineInput(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor term.Coordinates // buffer line/column of the caret
		wantX  int
		wantY  int // visual row within the editor band
	}{
		{
			name:   "single line keeps prompt offset",
			text:   "hello",
			cursor: term.Coordinates{X: 5, Y: 0},
			wantX:  7, // len("> ") + 5
			wantY:  0,
		},
		{
			name:   "second line drops prompt offset",
			text:   "hello\nworld",
			cursor: term.Coordinates{X: 5, Y: 1},
			wantX:  5,
			wantY:  1,
		},
		{
			name:   "third line accumulates preceding rows",
			text:   "a\nbb\nccc",
			cursor: term.Coordinates{X: 3, Y: 2},
			wantX:  3,
			wantY:  2,
		},
		{
			name:   "long first line wraps before continuation",
			text:   "0123456789012345678901234567\nx", // 28 cols + prompt = 30
			cursor: term.Coordinates{X: 1, Y: 1},
			wantX:  1,
			wantY:  2, // first line occupies rows 0-1, continuation on row 2
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandlerFull(t, nil, 100, nil)
			h.editHandler = cursorStubEditor{cursor: tt.cursor}.Edit(h.editBuf)
			h.editBuf.WriteString(tt.text)
			h.Resize(testWidthH, testHeight)

			pos, _, ok := h.Cursor()
			require.True(t, ok)
			innerH := h.editInnerH()
			assert.Equal(t,
				term.Coordinates{X: tt.wantX, Y: innerH + tt.wantY}, pos)
		})
	}
}

// TestCursorVisualClampsLineBeyondBuffer guards against a panic when the
// editor reports a cursor line past the buffer's last row: editBuf.Columns
// would index out of range, so the row accumulation must clamp to Rows().
func TestCursorVisualClampsLineBeyondBuffer(t *testing.T) {
	h := newTestHandlerFull(t, nil, 100, nil)
	// Cursor reports line 5 while the buffer only has one row ("hi").
	h.editHandler = cursorStubEditor{
		cursor: term.Coordinates{X: 0, Y: 5},
	}.Edit(h.editBuf)
	h.editBuf.WriteString("hi")
	h.Resize(testWidthH, testHeight)

	assert.NotPanics(t, func() {
		_, _, _ = h.Cursor()
	})
}

// TestArrowKeysMoveWithinMultilineItemBeforeCyclingHistory reproduces
// the bug where recalling a multi-line history entry trapped the cursor:
// the shell swallowed <up>/<down> for history cycling instead of letting
// the editor move the cursor between the entry's lines. Cursor motion
// must stay inside the buffer until a vertical edge is reached.
func TestArrowKeysMoveWithinMultilineItemBeforeCyclingHistory(t *testing.T) {
	editor := &multilineStubEditor{}
	h := newTestHandlerFull(t, []string{"old", "a\nb\nc"}, 100, nil)
	// Swap in the multiline-aware editor and rebind to the live buffer.
	h.editHandler = editor.Edit(h.editBuf)
	h.Resize(testWidthH, testHeight)

	// Recall the newest (multi-line) entry; cursor lands on its last row.
	h.Handle(arrowUp)
	require.Equal(t, "a\nb\nc", h.editBuf.String())
	require.Equal(t, 2, editor.h.cy)

	// <up> moves the cursor up within the recalled item, not into history.
	h.Handle(arrowUp)
	assert.Equal(t, 1, editor.h.cy)
	assert.Equal(t, "a\nb\nc", h.editBuf.String())

	h.Handle(arrowUp)
	assert.Equal(t, 0, editor.h.cy)
	assert.Equal(t, "a\nb\nc", h.editBuf.String())

	// At the top row, a further <up> cycles to the older entry.
	h.Handle(arrowUp)
	assert.Equal(t, "old", h.editBuf.String())
}

// TestArrowDownMovesWithinMultilineItemBeforeCyclingHistory is the
// downward counterpart: once the cursor has moved up inside a recalled
// multi-line entry, <down> walks back down its rows and only cycles to
// a newer history entry when the cursor reaches the bottom row.
func TestArrowDownMovesWithinMultilineItemBeforeCyclingHistory(t *testing.T) {
	editor := &multilineStubEditor{}
	h := newTestHandlerFull(t, []string{"a\nb\nc", "newest"}, 100, nil)
	h.editHandler = editor.Edit(h.editBuf)
	h.Resize(testWidthH, testHeight)

	// Recall "newest", then the older multi-line entry; cursor on row 2.
	h.Handle(arrowUp)
	require.Equal(t, "newest", h.editBuf.String())
	h.Handle(arrowUp)
	require.Equal(t, "a\nb\nc", h.editBuf.String())
	require.Equal(t, 2, editor.h.cy)

	// Climb to the top row of the recalled entry.
	h.Handle(arrowUp)
	h.Handle(arrowUp)
	require.Equal(t, 0, editor.h.cy)
	require.Equal(t, "a\nb\nc", h.editBuf.String())

	// <down> walks back down the entry's rows, not into newer history.
	h.Handle(arrowDown)
	assert.Equal(t, 1, editor.h.cy)
	assert.Equal(t, "a\nb\nc", h.editBuf.String())
	h.Handle(arrowDown)
	assert.Equal(t, 2, editor.h.cy)
	assert.Equal(t, "a\nb\nc", h.editBuf.String())

	// At the bottom row, a further <down> cycles to the newer entry.
	h.Handle(arrowDown)
	assert.Equal(t, "newest", h.editBuf.String())
}

func TestCtrlROpensReverseHistorySearch(t *testing.T) {
	h := newEditTestHandler(t, stubEditor{})
	h.Resize(testWidthH, testHeight)

	// Ordinary runes are consumed by the editor.
	feedRunes(h, "ab")
	assert.Equal(t, "ab", h.editBuf.String())
	assert.False(t, h.searching)

	// <c-r> is intercepted by the host (never delegated to the editor)
	// and opens the reverse-history search overlay.
	h.Handle(term.Event{Type: term.EventKey, Ch: 'r', Mod: term.ModCtrl})
	require.True(t, h.searching)
	assert.Equal(t, modeHistory, h.mode)
	h.cancelSearch()
}

// TestCtrlRNeverReachesEditor guards the VTE-like contract: the shell
// wrapper owns <c-r> and must intercept it for reverse-history search
// before the editor — which in vi insert mode would otherwise consume
// it to insert a register — ever sees it.
func TestCtrlRNeverReachesEditor(t *testing.T) {
	var seen []term.Event
	h := newEditTestHandler(t, stubEditor{seen: &seen})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "ab")
	h.Handle(term.Event{Type: term.EventKey, Ch: 'r', Mod: term.ModCtrl})

	for _, ev := range seen {
		require.False(t, ev.Mod == term.ModCtrl && ev.Ch == 'r',
			"editor must never receive <c-r>")
	}
	assert.True(t, h.searching)
	assert.Equal(t, modeHistory, h.mode)
	h.cancelSearch()
}

func TestTabTriggersCompletion(t *testing.T) {
	h, registry := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{},
		Config{MaxHistory: 100},
	)
	t.Cleanup(func() { _ = h.Close() })
	registry.Register("foo", "foo", echoCmd{out: new([]string)})
	registry.Register("foobar", "foobar", echoCmd{out: new([]string)})

	h.Resize(testWidthH, testHeight)
	feedRunes(h, "foo")
	require.Equal(t, "foo", h.editBuf.String())

	// <tab> is intercepted by the host (never delegated to the editor)
	// and forwarded to the completion machinery, which opens the
	// overlay for the two matching commands.
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})
	require.True(t, h.searching)
	assert.Equal(t, modeCompletion, h.mode)
	h.cancelSearch()
}

// TestTabNeverReachesEditor guards the VTE-like contract: the shell
// wrapper owns <tab> and must intercept it for completion before the
// editor — which would otherwise consume it to insert indentation —
// ever sees it.
func TestTabNeverReachesEditor(t *testing.T) {
	var seen []term.Event
	h, registry := New(
		func(func()) bool { return false },
		term.NopInterrupter(),
		stubEditor{seen: &seen},
		Config{MaxHistory: 100},
	)
	t.Cleanup(func() { _ = h.Close() })
	registry.Register("foo", "foo", echoCmd{out: new([]string)})
	registry.Register("foobar", "foobar", echoCmd{out: new([]string)})

	h.Resize(testWidthH, testHeight)
	feedRunes(h, "foo")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})

	for _, ev := range seen {
		require.False(t, ev.Key == term.KeyTab && ev.Mod == 0,
			"editor must never receive <tab>")
	}
	// The editor buffer keeps the typed prefix; no tab was inserted.
	assert.Equal(t, "foo", h.editBuf.String())
	assert.True(t, h.searching)
	h.cancelSearch()
}

func TestCtrlCCancelsRunningCommandBeforeEditor(t *testing.T) {
	var mu sync.Mutex
	var ticks []func()
	sched := func(fn func()) bool {
		mu.Lock()
		ticks = append(ticks, fn)
		mu.Unlock()
		return true
	}
	drain := func() {
		for {
			mu.Lock()
			if len(ticks) == 0 {
				mu.Unlock()
				return
			}
			fn := ticks[0]
			ticks = ticks[1:]
			mu.Unlock()
			fn()
		}
	}

	cmd := &blockingCmd{
		line:    "running-marker",
		release: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	var seen []term.Event
	h, registry := New(
		sched,
		term.NopInterrupter(),
		stubEditor{seen: &seen},
		Config{MaxHistory: 100},
	)
	t.Cleanup(func() {
		select {
		case <-cmd.release:
		default:
			close(cmd.release)
		}
		_ = h.Close()
	})
	registry.Register("block", "blocks", cmd)

	feedRunes(h, "block")
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	require.Eventually(t, func() bool {
		drain()
		return h.shim.running()
	}, time.Second, 5*time.Millisecond, "command should be in-flight")

	_, handled := h.Handle(term.Event{Type: term.EventKey, Ch: 'c', Mod: term.ModCtrl})
	require.True(t, handled, "<c-c> must be handled")

	require.Eventually(t, func() bool {
		drain()
		select {
		case <-cmd.closed:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond, "<c-c> should cancel the command")
	assert.False(t, h.shim.running(), "command should no longer be in-flight")
	assert.Equal(t, "", h.editBuf.String(), "<c-c> should not be typed into the editor")
	for _, ev := range seen {
		require.False(t, ev.Mod == term.ModCtrl && ev.Ch == 'c',
			"editor must never receive <c-c>")
	}
}

// TestAcceptHistoryMatchReplacesEditorBuffer verifies that choosing a
// reverse-history result writes the chosen command into the editor
// buffer (the editor owns the input line), not the inner inputbox.
func TestAcceptHistoryMatchReplacesEditorBuffer(t *testing.T) {
	h := newTestHandler(t, []string{"deploy --prod"})
	h.Resize(testWidthH, testHeight)

	// Open reverse-history search; the single entry is focused.
	h.Handle(term.Event{Type: term.EventKey, Ch: 'r', Mod: term.ModCtrl})
	require.True(t, h.searching)
	require.Equal(t, modeHistory, h.mode)

	// Accept the focused match.
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})

	assert.False(t, h.searching)
	assert.Equal(t, "deploy --prod", h.editBuf.String())
}

// TestAcceptCompletionCandidateReplacesEditorBuffer verifies that
// accepting a tab-completion candidate rewrites the editor buffer with
// the completed command rather than mutating the inner inputbox.
func TestAcceptCompletionCandidateReplacesEditorBuffer(t *testing.T) {
	h := newTestHandlerFull(t, nil, 100, func(r *CommandRegistry) {
		r.Register("foobar", "foobar", stubCmd{})
		r.Register("foobaz", "foobaz", stubCmd{})
	})
	h.Resize(testWidthH, testHeight)

	feedRunes(h, "foob")
	require.Equal(t, "foob", h.editBuf.String())

	// <tab> opens the completion overlay with the two matches; the
	// first is focused.
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyTab})
	// Candidates stream into the overlay off the event loop; wait for
	// them to settle before asserting and accepting.
	h.waitCompletion()
	drainTicks(h)
	require.True(t, h.searching)
	require.Equal(t, modeCompletion, h.mode)

	// Accept the focused candidate.
	h.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})

	assert.False(t, h.searching)
	// The chosen candidate fully replaces the editor buffer.
	assert.Contains(t, []string{"foobar", "foobaz"}, h.editBuf.String())
}

// echoCmd is a CommandHandler that records its dispatched name into
// a caller-supplied slice. It is used to verify that edit-mode
// submit forwards the original <enter> through to the inner repl.
type echoCmd struct{ out *[]string }

func (e echoCmd) HandleCommand(
	_ context.Context, cmd repl.Command, _ repl.ProgressWriter,
) (iterator.Iterator[component.Responsive], error) {
	*e.out = append(*e.out, cmd.Name)
	return iterator.Empty[component.Responsive](), nil
}

func (e echoCmd) Complete(
	context.Context, string, []string,
) (iterator.Iterator[string], error) {
	return iterator.Empty[string](), nil
}

func (e echoCmd) Help(
	context.Context, []string,
) (iterator.Iterator[component.Responsive], error) {
	return iterator.Empty[component.Responsive](), nil
}
