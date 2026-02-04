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

package cell

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var undoFortune = `Love in your heart wasn't put there to stay.
Love isn't love 'til you give it away.
		-- Oscar Hammerstein 中国`

func initUndoTestBuffer(t *testing.T) (u *undoer, b *Buffer) {
	b = newBufferWithContent(t, undoFortune)
	u = b.undoer
	return
}

func TestDebugUndo(t *testing.T) {
	undoer, buf := initUndoTestBuffer(t)
	prev := buf.String()

	buf.DeleteCell(term.Coordinates{X: 4, Y: 2})
	ok, _ := undoer.undo()
	assert.True(t, ok)
	assert.Equal(t, prev, buf.String())
}

func TestUndoRawEdit(t *testing.T) {
	undoer, buf := initUndoTestBuffer(t)
	prev := buf.String()

	buf.Edit(context.Background(), term.Coordinates{Y: 2, X: 0},
		term.Coordinates{Y: 2, X: 2}, "")
	noTabs := "Love in your heart wasn't put there to stay.\nLove isn't " +
		"love 'til you give it away.\n-- Oscar Hammerstein 中国"
	assert.Equal(t, noTabs, buf.String())
	ok, _ := undoer.undo()

	assert.True(t, ok)
	after := buf.String()
	assert.Equal(t, prev, after)
}

func TestUndo(t *testing.T) {
	suite := []struct {
		name string
		cmd  func(b *Buffer)
	}{
		{"Insert", func(b *Buffer) {
			b.Insert(term.Coordinates{X: 0, Y: 2}, '\t')
		}},
		{"InsertRowAt", func(b *Buffer) {
			insertRowAt(b, 1)
		}},
		{"DeleteCell", func(b *Buffer) {
			b.DeleteCell(term.Coordinates{X: 4, Y: 2})
		}},
		{"ConflateRow", func(b *Buffer) {
			b.ConflateRow(1)
		}},
		{"WrapRow", func(b *Buffer) {
			assert.True(t, b.WrapRow(1, 5))
		}},
		{"TruncateRowFrom", func(b *Buffer) {
			b.TruncateRowFrom(term.Coordinates{X: 0, Y: 1})
		}},
		{"TruncateFrom", func(b *Buffer) {
			b.TruncateFrom(term.Coordinates{X: 6, Y: 0})
		}},
		{"DeleteRow", func(b *Buffer) {
			b.DeleteRow(0)
		}},
		{"Edit which effectively replaces", func(b *Buffer) {
			b.Edit(context.Background(), term.Coordinates{Y: 2, X: 1},
				term.Coordinates{Y: 2, X: 5}, "a\tb\t")
		}},
		{"multiline Edit which effectively replaces", func(b *Buffer) {
			b.Edit(context.Background(), term.Coordinates{Y: 0, X: 1},
				term.Coordinates{Y: 2}, "a\tb\n\t")
		}},
	}

	for _, _tcase := range suite {
		tcase := _tcase
		t.Run(fmt.Sprintf("undo %s", tcase.name), func(t *testing.T) {
			undoer, buf := initUndoTestBuffer(t)
			prev := buf.String()

			for i := 0; i < 5; i++ {
				tcase.cmd(buf)
				ok, _ := undoer.undo()
				// TODO assert.Equal(t, term.Coordinates{}, at)
				assert.True(t, ok)
			}

			after := buf.String()

			assert.Equal(t, prev, after)
		})
	}

	t.Run("undo/redo a series of updates", func(t *testing.T) {
		undoer, buf := initUndoTestBuffer(t)
		prev := buf.String()

		for _, tcase := range suite {
			tcase.cmd(buf)
		}

		middle := buf.String()

		for range suite {
			undoer.undo()
		}

		after := buf.String()
		assert.Equal(t, prev, after)

		for range suite {
			undoer.redo()
		}

		afterRedo := buf.String()
		assert.Equal(t, middle, afterRedo)

		for range suite {
			undoer.undo()
		}

		after = buf.String()
		assert.Equal(t, prev, after)
	})

	t.Run("undo/redo a series of updates all via grouped undo", func(t *testing.T) {
		undoer, buf := initUndoTestBuffer(t)
		prev := buf.String()

		undoer.startMergeUndo()
		for _, tcase := range suite {
			tcase.cmd(buf)
		}
		undoer.endMergeUndo()

		middle := buf.String()

		ok, at := undoer.undo()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{Y: 2}, at)

		after := buf.String()
		assert.Equal(t, prev, after)

		ok, at = undoer.redo()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{X: 1, Y: 1}, at)

		afterRedo := buf.String()
		assert.Equal(t, middle, afterRedo)

		ok, at = undoer.undo()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{Y: 2}, at)

		after = buf.String()
		assert.Equal(t, prev, after)
	})

	t.Run("undo/redo a series of updates partially via grouped undo (after)", func(t *testing.T) {
		undoer, buf := initUndoTestBuffer(t)
		prev := buf.String()

		for i, tcase := range suite {
			if i == 4 {
				undoer.startMergeUndo()
			}
			tcase.cmd(buf)
		}
		undoer.endMergeUndo()

		middle := buf.String()

		undoer.undo()
		for range 6 {
			undoer.undo()
		}

		after := buf.String()
		assert.Equal(t, prev, after)

		for range 6 {
			undoer.redo()
		}
		undoer.redo()

		afterRedo := buf.String()
		assert.Equal(t, middle, afterRedo)

		undoer.undo()
		for range 6 {
			undoer.undo()
		}

		after = buf.String()
		assert.Equal(t, prev, after)
	})

	t.Run("undo/redo a series of updates partially via grouped undo (before)", func(t *testing.T) {
		undoer, buf := initUndoTestBuffer(t)
		prev := buf.String()

		undoer.startMergeUndo()
		for i, tcase := range suite {
			if i == 4 {
				undoer.endMergeUndo()
			}
			tcase.cmd(buf)
		}

		middle := buf.String()

		for range 6 {
			undoer.undo()
		}
		undoer.undo()

		after := buf.String()
		assert.Equal(t, prev, after)

		undoer.redo()
		for range 6 {
			undoer.redo()
		}

		afterRedo := buf.String()
		assert.Equal(t, middle, afterRedo)

		for range 6 {
			undoer.undo()
		}
		undoer.undo()

		after = buf.String()
		assert.Equal(t, prev, after)
	})
}

func TestUndoEOL(t *testing.T) {
	t.Run("undo EOL", func(t *testing.T) {
		const (
			filecontent1 = `package me.drton.jmavsim;
public class Rotor {
     sta  mtyp;


  myClass;
`
			insertStr = "\tmyClassVar\n"
		)

		insertAt := term.Coordinates{Y: 5, X: 9}
		abuf := NewBuffer()
		abuf.ReadFrom(strings.NewReader(filecontent1))
		astr0 := abuf.String()
		arcells0 := abuf.RawCells()

		afrom, ato, _ := abuf.editor.Edit(context.Background(), insertAt,
			insertAt, insertStr)
		astr1 := abuf.String()
		arcells1 := abuf.RawCells()

		abuf.editor.Edit(context.Background(), afrom, ato, "")
		astr2 := abuf.String()
		arcells2 := abuf.RawCells()
		assert.Equal(t, astr0, astr2)
		assert.Equal(t, arcells0, arcells2)

		ok, _ := abuf.Undo()
		assert.True(t, ok)
		bstr1 := abuf.String()
		brcells1 := abuf.RawCells()
		assert.Equal(t, astr1, bstr1)
		assert.Equal(t, arcells1, brcells1)

		ok, _ = abuf.Undo()
		assert.True(t, ok)
		bstr0 := abuf.String()
		brcells0 := abuf.RawCells()
		assert.Equal(t, astr0, bstr0)
		assert.Equal(t, arcells0, brcells0)

		ok, _ = abuf.Redo()
		assert.True(t, ok)
		ok, _ = abuf.Redo()
		assert.True(t, ok)
		bstr2 := abuf.String()
		brcells2 := abuf.RawCells()
		assert.Equal(t, astr2, bstr2)
		assert.Equal(t, arcells2, brcells2)
	})

	t.Run("last EOL", func(t *testing.T) {
		buf := NewBuffer()
		buf.ReadFrom(strings.NewReader("a\n"))
		initialString := buf.String()
		initialCells := buf.RawCells()

		insertRowAt(buf, 1)
		newString := buf.String()
		newCells := buf.RawCells()
		assert.Equal(t, "a\n\n", newString)
		assert.Equal(t,
			[][]term.Cell{{{Ch: 'a', Combining: []rune{}, Bytes: 1, Width: 1}}, {}, {}},
			newCells)

		ok, _ := buf.Undo()
		assert.True(t, ok)
		newString2 := buf.String()
		newCells2 := buf.RawCells()
		assert.Equal(t, initialString, newString2)
		assert.Equal(t, initialCells, newCells2)

	})
}
