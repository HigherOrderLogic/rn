package vte

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/screen"
	"unstable.build/go-tui/text"
)

func TestViEditUnit(t *testing.T) {
	t.Run("screen context bypasses Edit", func(t *testing.T) {
		comp := newTestParentComponent("a\nb", term.Coordinates{})
		var vi viHandler
		vi.doInit(comp, DefaultConfig())

		ctx := screen.NewContext(context.Background())
		vi.Edit(ctx, term.Coordinates{}, term.Coordinates{Y: 1, X: 1}, "b\nb")
		assert.Equal(t, "b\nb", comp.scroll.Buffer().String())
	})

	suite := []struct {
		description    string
		initialContent string
		// it is assumed that when Edit is invoked the component cursor
		// is at the start of the prompt.
		promptStart term.Coordinates
		start       term.Coordinates
		end         term.Coordinates
		str         string

		expectedRingBell bool
		expectedContent  string
		expectedFrom     term.Coordinates
		expectedTo       term.Coordinates
		expectedOld      string
	}{
		{
			description:      "(invalid) zero Edit",
			initialContent:   "",
			promptStart:      term.Coordinates{},
			start:            term.Coordinates{},
			end:              term.Coordinates{},
			str:              "",
			expectedRingBell: true,
			expectedContent:  "",
		},
		{
			description: "insert within last prompt line, exactly after prompt",
			initialContent: `
~/src/blue master
$ 
`,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "echo",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo
`,
		},
		{
			description: "insert within last prompt line, before prompt is shifted",
			initialContent: `
~/src/blue master
$ 
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "$ echo",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo
`,
		},
		{
			description: "delete until end of prompt line starting at prompt",
			initialContent: `
~/src/blue master
$ echo
`,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "delete until end of prompt line starting before prompt is trimmed to prompt start",
			initialContent: `
~/src/blue master
$ echo
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "delete prompt line until next line, (vi's dd), last line",
			initialContent: `
~/src/blue master
$ echo`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ `,
		},
		{
			description: "delete prompt line until next line, (vi's dd), not last line",
			initialContent: `
~/src/blue master
$ echo
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 6},
			expectedOld:      "echo",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "delete prompt multiline, no last line",
			initialContent: `
~/src/blue master
$ echo blaaaaaaaaa
aaaaaaaaa`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3, X: 16},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 3, X: 9},
			// must guarantee reversibility: since shell wraps lines
			// automatically, we must remove newlines.
			// expectedOld:      "echo blaaaaaaaaa\naaaaaaaaa",
			expectedOld: "echo blaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ `,
		},
		{
			description: "delete prompt multiline, with last line",
			initialContent: `
~/src/blue master
$ echo blaaaaaaaaa
aaaaaaaaa
`,
			start:            term.Coordinates{Y: 2},
			end:              term.Coordinates{Y: 3, X: 16},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 3, X: 9},
			expectedOld:      "echo blaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
`,
		},
		{
			description: "insert with new line, with last line",
			initialContent: `
~/src/blue master
$ echo 
`,
			start:            term.Coordinates{Y: 2, X: 7},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "aaaaaaaaaaaaaaaaaaaa",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 7},
			// must guarantee reversibility so must be multiline
			expectedTo:  term.Coordinates{Y: 3, X: 9},
			expectedOld: "",
			expectedContent: `
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaa
`,
		},
		{
			description: "insert with new line, no last line",
			initialContent: `
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 7},
			end:              term.Coordinates{Y: 2, X: 7},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "aaaaaaaaaaaaaaaaaaaa",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 7},
			expectedTo:       term.Coordinates{Y: 3, X: 9},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaa`,
		},
		{
			description: "insert above last prompt rings bell",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "a",
			expectedRingBell: true,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 2},
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
		},
		{
			description: "delete above last prompt rings bell",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 3},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: true,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 2},
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
		},
		{
			description: "delete up to prompt start rings a bell",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 4, X: 2},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: true,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 4, X: 2},
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo `,
		},
		{
			description: "insert at shell-wrapped line",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
a`,
			start:            term.Coordinates{Y: 5, X: 1},
			end:              term.Coordinates{Y: 5, X: 1},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "xyz",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 5, X: 1},
			expectedTo:       term.Coordinates{Y: 5, X: 4},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
axyz`,
		},
		{
			description: "insert at shell-wrapped line, with more than line wrapped line",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:            term.Coordinates{Y: 6, X: 1},
			end:              term.Coordinates{Y: 6, X: 1},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "xyz",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 6, X: 1},
			expectedTo:       term.Coordinates{Y: 6, X: 4},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
axyz`,
		},
		{
			description: "delete entire content, deletes only prompt lines, last line + 1",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:            term.Coordinates{},
			end:              term.Coordinates{Y: 7},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ `,
		},
		{
			description: "delete entire content, deletes only prompt lines, last column + 1",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:            term.Coordinates{},
			end:              term.Coordinates{Y: 6, X: 1},
			promptStart:      term.Coordinates{Y: 4, X: 2},
			str:              "",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ `,
		},
		{
			description: "entire content replace, replaces only prompt lines, exact length",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:       term.Coordinates{},
			end:         term.Coordinates{Y: 6, X: 1},
			promptStart: term.Coordinates{Y: 4, X: 2},
			str: `
~/SRC/BLUE MASTER
% ECHO XXXXXXXXXX
~/src/blue master
$ ECHO AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`,
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ ECHO AAAAAAAAAAA
AAAAAAAAAAAAAAAAAA
A`,
		},
		{
			description: "entire content replace, replaces only prompt lines + newlines until height",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaaaaaaaaaaaa
a`,
			start:       term.Coordinates{},
			end:         term.Coordinates{Y: 6, X: 1},
			promptStart: term.Coordinates{Y: 4, X: 2},
			str: `
~/SRC/BLUE MASTER
% ECHO XXXXXXXXXX
~/src/blue master
$ ECHO AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA












`,
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 6, X: 1},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ ECHO AAAAAAAAAAA
AAAAAAAAAAAAAAAAAA
A`,
		},
		{
			description: "paste at prompt, not last line",
			initialContent: `
~/src/blue master
$ 
`,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "echo bla",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 10},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo bla
`,
		},
		{
			description: "paste at prompt, last line",
			initialContent: `
~/src/blue master
$ `,
			start:            term.Coordinates{Y: 2, X: 2},
			end:              term.Coordinates{Y: 2, X: 2},
			promptStart:      term.Coordinates{Y: 2, X: 2},
			str:              "echo bla",
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 2, X: 2},
			expectedTo:       term.Coordinates{Y: 2, X: 10},
			expectedOld:      "",
			expectedContent: `
~/src/blue master
$ echo bla`,
		},
		{
			description: "undo on wrapped line",
			initialContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaXXaaaa





`,
			start:       term.Coordinates{},
			end:         term.Coordinates{Y: 6},
			promptStart: term.Coordinates{Y: 4, X: 2},
			str: `
~/SRC/BLUE MASTER
% ECHO XXXXXXXXXX
~/src/blue master
$ echo aaaaaaaaaaaaaaaaaaooaaaa`,
			expectedRingBell: false,
			expectedFrom:     term.Coordinates{Y: 4, X: 2},
			expectedTo:       term.Coordinates{Y: 5, X: 13},
			expectedOld:      "echo aaaaaaaaaaaaaaaaaaXXaaaa",
			expectedContent: `
~/src/blue master
$ 
~/src/blue master
$ echo aaaaaaaaaaa
aaaaaaaooaaaa





`,
		},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			logrus.SetLevel(logrus.TraceLevel)
			comp := newTestParentComponent(test.initialContent, test.promptStart)
			var vi viHandler
			cfg := DefaultConfig()
			var actualBellsRung int
			cfg.RingBell = func() {
				actualBellsRung++
			}
			vi.doInit(comp, cfg)
			vi.remote = newTestRemote(comp.scroll, test.promptStart)
			vi.Resize(18, 18)

			ctx := context.Background()
			actualFrom, actualTo, actualOld := vi.Edit(ctx, test.start, test.end, test.str)

			assert.Equal(t, test.expectedContent, comp.scroll.Buffer().String())
			assert.Equal(t, test.expectedFrom, actualFrom, "from")
			assert.Equal(t, test.expectedTo, actualTo, "to")
			assert.Equal(t, test.expectedOld, actualOld)

			var expectedBellsRung int
			if test.expectedRingBell {
				expectedBellsRung = 1
			}
			assert.Equal(t, expectedBellsRung, actualBellsRung, "bells rung")
		})
	}
}

type testParentComponent struct {
	scroll *component.Scroll
	uri    workspaceapi.URI
	cursor term.Coordinates
}

func newTestParentComponent(content string, cursorAtScroll term.Coordinates) *testParentComponent {
	buf := new(cell.Buffer)
	buf.InitPerformance(cell.DefaultTabspaces, 1, 1)

	scroll := new(component.Scroll)
	scroll.InitPerformance(buf)
	// do not use ReadFrom or InsertString as null characters
	// will be elided.
	var next term.Coordinates
	for _, ch := range content {
		next = buf.Insert(next, ch)
	}
	testURI, _ := workspaceapi.ParseURI("memory:///")
	return &testParentComponent{
		scroll: scroll,
		uri:    testURI,
		cursor: cursorAtScroll,
	}
}

func (c *testParentComponent) PrimaryScroll() *component.Scroll {
	return c.scroll
}

func (c *testParentComponent) URI() workspaceapi.URI {
	return c.uri
}

func (c *testParentComponent) Locker() sync.Locker {
	return nopLocker{}
}

func (c *testParentComponent) cursorAtScroll() term.Coordinates {
	return c.cursor
}

func (c *testParentComponent) scheduleBellCallback(timeout time.Duration, callback func()) bool {
	callback()
	return true
}

type nopLocker struct {
}

func (nopLocker) Lock() {
}

func (nopLocker) Unlock() {
}

type testRemote struct {
	cursor             *text.Cursor
	keyArrowUpCalled   int
	keyArrowDownCalled int
	formFeedCalled     int
	lineFeedCalled     int
	ops                []func()
	ctx                context.Context
}

func newTestRemote(scroll *component.Scroll, cursorPosition term.Coordinates) *testRemote {
	ret := new(testRemote)
	ret.cursor = new(text.Cursor)
	ret.cursor.InitPerformance(scroll)
	// needed to ensure that remote edits bypass Edit checks
	ret.ctx = screen.NewContext(context.Background())
	ret.cursor.MoveToScroll(cursorPosition)
	return ret
}

func (r *testRemote) moveStartOfLine() {
	r.cursor.MoveStartLine()
}

func (r *testRemote) keyArrowUp() {
	r.keyArrowUpCalled++
}

func (r *testRemote) keyArrowDown() {
	r.keyArrowDownCalled++
}

func (r *testRemote) deleteChar() {
	r.ops = append(r.ops, func() {
		r.cursor.DeleteContext(r.ctx)
	})
}

func (r *testRemote) insertChar(ch rune) {
	r.ops = append(r.ops, func() {
		r.cursor.InsertContext(r.ctx, ch)
	})
}

func (r *testRemote) linefeed() {
	r.lineFeedCalled++
}

func (r *testRemote) formFeed() {
	r.formFeedCalled++
}

func (r *testRemote) moveLeft() {
	r.ops = append(r.ops, func() {
		r.cursor.MoveLeft()
	})
}

func (r *testRemote) moveRight() {
	r.ops = append(r.ops, func() {
		r.cursor.MoveRight()
	})
}

func (r *testRemote) conflate() {
	r.ops = append(r.ops, func() {
		r.cursor.ConflateContext(r.ctx)
	})
}

func (r *testRemote) wrapLine() {
	r.ops = append(r.ops, func() {
		r.cursor.InsertContext(r.ctx, '\n')
	})
}

func (r *testRemote) cursorCRLF() {
	r.ops = append(r.ops, func() {
		r.cursor.MoveDown()
		r.cursor.MoveStartLine()
	})
}

func (r *testRemote) flush() error {
	for _, op := range r.ops {
		op()
	}
	r.ops = r.ops[:0]
	return nil
}
