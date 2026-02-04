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

package ide

import (
	"context"
	"fmt"

	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/text/vi"
	"unstable.build/go-tui/workspace"
)

func newIntegrationTestCase(t *testing.T, content string) (
	*cell.Buffer, workspace.FlusherCloser, workspaceapi.URI, func(),
) {
	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	workspaceURI, err := workspaceapi.CurrentUserHostURI(tempDir)
	require.NoError(t, err)

	manager := workspace.NewManager(config.NopConfig())
	require.NoError(t, manager.RegisterScheme(workspace.FileScheme, workspace.NewFileScheme))
	w, err := manager.AddWorkspace(context.Background(), workspaceURI)
	require.NoError(t, err)

	file, err := os.CreateTemp(tempDir, "workspace_int_test")
	require.NoError(t, err)

	_, err = file.Write([]byte(content))
	require.NoError(t, err)

	uri, err := workspaceapi.CurrentUserHostURI(file.Name())
	require.NoError(t, err)

	swapURI, err := workspaceapi.CurrentUserHostURI(tempDir)
	require.NoError(t, err)

	buffer := cell.NewBuffer()
	fc, err := w.Load(uri, buffer, swapURI, false)
	require.NoError(t, err)

	return buffer, fc, uri, func() {
		manager.Close()
		file.Close()
		os.Remove(file.Name())
		fc.Close()
	}
}

func newViIntegrationTestCase(
	t *testing.T, content string, width, height int,
) (*cell.Buffer, tui.Handler, func()) {
	buf, _, uri, clean := newIntegrationTestCase(t, content)
	defer clean()

	vi := vi.New(buf, uri)
	vi.Resize(width, height)
	return buf, vi, clean
}

func TestLastEOLUndoFileIntegration(t *testing.T) {
	buf, _, _, cleanup := newIntegrationTestCase(t, "a\n")
	defer cleanup()

	initialString := buf.String()
	initialCells := buf.RawCells()
	assert.Equal(t, "a", initialString)
	assert.Equal(t,
		[][]term.Cell{{{Ch: 'a', Combining: []rune{}, Bytes: 1, Width: 1}}}, initialCells)

	at := term.Coordinates{Y: 1}
	buf.Edit(context.Background(), at, at, "\n")
	newString := buf.String()
	newCells := buf.RawCells()
	assert.Equal(t, "a\n", newString)
	assert.Equal(t,
		[][]term.Cell{{{Ch: 'a', Combining: []rune{}, Bytes: 1, Width: 1}}, {}}, newCells)

	ok, _ := buf.Undo()
	assert.True(t, ok)
	newString2 := buf.String()
	newCells2 := buf.RawCells()
	assert.Equal(t, initialString, newString2)
	assert.Equal(t, initialCells, newCells2)

	buf.Edit(context.Background(), at, at, "\n")
	newString = buf.String()
	newCells = buf.RawCells()
	assert.Equal(t, "a\n", newString)
	assert.Equal(t,
		[][]term.Cell{{{Ch: 'a', Combining: []rune{}, Bytes: 1, Width: 1}}, {}}, newCells)
}

func TestViIntegration(t *testing.T) {
	t.Run("last EOL", func(t *testing.T) {
		for _, content := range []string{"hello", "hello\n"} {
			t.Run(fmt.Sprintf("insert word below last line: %q", content), func(t *testing.T) {
				buf, vi, cleanup := newViIntegrationTestCase(t, content, 4, 4)
				defer cleanup()

				for _, ch := range "Goworld" {
					vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				}
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
				assert.Equal(t, "hello\nworld", buf.String())

				// undo
				_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
				require.True(t, handled)
				require.Equal(t, "hello", buf.String())

				for _, ch := range "Goworld" {
					vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				}
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
				assert.Equal(t, "hello\nworld", buf.String())
			})

			t.Run(fmt.Sprintf("insert a newline last line: %q", content), func(t *testing.T) {
				buf, vi, clean := newViIntegrationTestCase(t, content, 4, 4)
				defer clean()

				for _, ch := range "Go\n" {
					vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				}
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
				assert.Equal(t, "hello\n\n", buf.String())

				// undo
				_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: 'u'})
				require.True(t, handled)
				require.Equal(t, "hello", buf.String())

				for _, ch := range "Go\n" {
					vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
				}
				vi.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
				assert.Equal(t, "hello\n\n", buf.String())
			})
		}
	})

	t.Run("line select paste on last EOL", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\nb\nc\nd\n", 4, 4)
		defer clean()

		for _, ch := range "Gkyyp" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nb\nc\nc\nd", buf.String())
	})

	t.Run("line select copy last line after", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\nb\nc\nd\n", 4, 4)
		defer clean()

		for _, ch := range "Gyyggp" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nd\nb\nc\nd", buf.String())
	})

	t.Run("paste line after last line", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\nb\nc\nd\n", 2, 2)
		defer clean()

		for _, ch := range "VjyGp" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nb\nc\nd\na\nb", buf.String())
	})

	t.Run("delete last empty line", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\nb\nc\nd\n\n", 4, 4)
		defer clean()

		for _, ch := range "Gdd" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nb\nc\nd", buf.String())
	})

	t.Run("delete last empty line and second to last", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\nb\nc\nd\n\n", 4, 4)
		defer clean()

		for _, ch := range "Gdddd" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nb\nc", buf.String())
	})

	t.Run("delete any empty line", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\n\nc", 4, 4)
		defer clean()

		for _, ch := range "jdd" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nc", buf.String())
	})

	t.Run("delete only newline in visual mode", func(t *testing.T) {
		buf, vi, clean := newViIntegrationTestCase(t, "a\n\nc", 4, 4)
		defer clean()

		for _, ch := range "jvkld" {
			_, handled := vi.Handle(term.Event{Type: term.EventKey, Ch: ch})
			require.True(t, handled)
		}

		require.Equal(t, "a\nc", buf.String())
	})
}
