package vte

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

func TestLastPromptLine(t *testing.T) {
	suite := []struct {
		description   string
		content       string
		width         int
		expectedStart term.Coordinates
		expectedEnd   term.Coordinates
	}{
		{
			description:   "empty just returns zero",
			content:       "",
			width:         10,
			expectedStart: term.Coordinates{},
			expectedEnd:   term.Coordinates{},
		},
		{
			description:   "exactly one line shell",
			content:       "$",
			width:         10,
			expectedStart: term.Coordinates{},
			expectedEnd:   term.Coordinates{X: 1},
		},
		{
			description: "one line shell with history",
			content: `
blablab  
$ echo`,
			width:         10,
			expectedStart: term.Coordinates{Y: 2},
			expectedEnd:   term.Coordinates{Y: 2, X: 6},
		},
		{
			description: "empty last line should return everything since last shell",
			content: `
blablabla
$ echo    
       `,
			width:         10,
			expectedStart: term.Coordinates{Y: 2},
			expectedEnd:   term.Coordinates{Y: 3, X: 7},
		},
		{
			description: "multiline simple",
			content: `
blablabla
$ echo bla
aaaaaaaaaa`,
			width:         10,
			expectedStart: term.Coordinates{Y: 2},
			expectedEnd:   term.Coordinates{Y: 3, X: 10},
		},
		{
			description: "multiline with space (previous less than width)",
			content: `
blablabla
$ echo
$ aaaaaaa `,
			width:         10,
			expectedStart: term.Coordinates{Y: 3},
			expectedEnd:   term.Coordinates{Y: 3, X: 10},
		},
		{
			description: "multiline with space (previous filled with blank chars)",
			content: "\n" +
				"blablabla\n" +
				"$ echo\x00\x00\x00\x00\n" +
				"$ aaaaaaa \n",
			width:         10,
			expectedStart: term.Coordinates{Y: 3},
			expectedEnd:   term.Coordinates{Y: 3, X: 10},
		},
		{
			description: "AMBIGUOUS multiline",
			content: `
blablabla
$ echo bla
$ aaaaaaaa`,
			width:         10,
			expectedStart: term.Coordinates{Y: 2},
			expectedEnd:   term.Coordinates{Y: 3, X: 10},
		},
		{
			description: "single line with blank lines under",
			content: "blablabla \n" +
				"$ echo\n" +
				"$ aaaaaaa\x00\x00\n" +
				"\x00\x00\x00",
			width:         10,
			expectedStart: term.Coordinates{Y: 2},
			expectedEnd:   term.Coordinates{Y: 2, X: 9},
		},
		{
			description: "multiline with blank lines under",
			content: "blablabla\n" +
				"$ echo aaa\n" +
				"aaaaaaaaa\x00\x00\n" +
				"\x00\x00\x00",
			width:         10,
			expectedStart: term.Coordinates{Y: 1},
			expectedEnd:   term.Coordinates{Y: 2, X: 9},
		},
	}

	for _, test := range suite {
		t.Run(test.description, func(t *testing.T) {
			buf := cell.NewBuffer()
			// do not use ReadFrom or InsertString as null characters
			// will be elided.
			var next term.Coordinates
			for _, ch := range test.content {
				next = buf.Insert(next, ch)
			}

			// sut
			actualStart, actualEnd := lastPromptLine(buf, test.width)
			assert.Equal(t, test.expectedStart, actualStart, "start")
			assert.Equal(t, test.expectedEnd, actualEnd, "end")
		})
	}
}
