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
	context "context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/unstablebuild/rune-go-sdk/api/config"
	term "github.com/unstablebuild/rune-go-sdk/term"
	cell "unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
)

func TestReadFile(t *testing.T) {
	tcases := []struct {
		name             string
		contentsToReadIn string
		contentsToReadTo string
		cursorPosition   term.Coordinates
		expectedResult   string
	}{
		{
			name: "insert single-line non-newline-terminated file (ooo) " +
				"at the start of the file and start of the line " +
				"of the newline-terminated file (xxx)",
			contentsToReadIn: "ooo",
			contentsToReadTo: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n",
			cursorPosition: term.Coordinates{X: 0, Y: 0},
			expectedResult: "" +
				"xxx\n" +
				"ooo\n" +
				"xxx\n" +
				"xxx\n",
		},
		{
			name: "insert single-line non-newline-terminated file (ooo) " +
				"at the start of the file and start of the line " +
				"of the newline-terminated file (xxx)",
			contentsToReadIn: "ooo",
			contentsToReadTo: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n",
			cursorPosition: term.Coordinates{X: 1, Y: 1},
			expectedResult: "" +
				"xxx\n" +
				"xxx\n" +
				"ooo\n" +
				"xxx\n",
		},
		{
			name: "insert multi-line non-newline-terminated file (ooo) " +
				"at the end of the file and end of the line " +
				"of the newline-terminated file (xxx)",
			contentsToReadIn: "" +
				"ooo\n" +
				"ooo\n" +
				"ooo",
			contentsToReadTo: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n",
			cursorPosition: term.Coordinates{X: 3, Y: 2},
			expectedResult: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n" +
				"ooo\n" +
				"ooo\n" +
				"ooo\n",
		},
		{
			name: "insert multi-line newline-terminated file (ooo) " +
				"at the middle of the file and middle of the line " +
				"of the newline-terminated file (xxx)",
			contentsToReadIn: "" +
				"ooo\n" +
				"ooo\n" +
				"ooo\n",
			contentsToReadTo: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n",
			cursorPosition: term.Coordinates{X: 1, Y: 1},
			expectedResult: "" +
				"xxx\n" +
				"xxx\n" +
				"ooo\n" +
				"ooo\n" +
				"ooo\n" +
				"xxx\n",
		},
		{
			name: "insert multi-line newline-terminated file (ooo) " +
				"at the end of the file and end of the line " +
				"of the newline-terminated file (xxx)",
			contentsToReadIn: "" +
				"ooo\n" +
				"ooo\n" +
				"ooo\n",
			contentsToReadTo: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n",
			cursorPosition: term.Coordinates{X: 3, Y: 2},
			expectedResult: "" +
				"xxx\n" +
				"xxx\n" +
				"xxx\n" +
				"ooo\n" +
				"ooo\n" +
				"ooo\n",
		},
	}

	for i, tcase := range tcases {
		t.Run(fmt.Sprintf(tcase.name, i), func(t *testing.T) {
			dir, err := os.MkdirTemp("", "TestReadFile")
			require.NoError(t, err)

			t.Cleanup(func() {
				_ = os.RemoveAll(dir)
			})

			workspaceURI, err := workspaceapi.ParseURI("file://" + dir)
			require.NoError(t, err)

			scheme, err := workspace.NewFileScheme(
				context.Background(), config.NopConfig(), workspaceURI,
			)
			require.NoError(t, err)

			workspace := workspace.NewSchemeWorkspace(workspaceURI, scheme)

			c, err := text.NewComponent(
				NopEditor(), workspace, text.DefaultConfig(),
			)
			require.NoError(t, err)

			fileToReadPath := filepath.Join(dir, fmt.Sprintf("file_to_read_%d.txt", i))
			fileToRead, err := os.Create(fileToReadPath)
			require.NoError(t, err)

			_, err = fileToRead.Write([]byte(tcase.contentsToReadIn))
			require.NoError(t, err)

			fileToReadURI, err := workspaceapi.ParseURI("file://" + fileToReadPath)
			require.NoError(t, err)

			currentURI, err := workspaceapi.ParseURI(
				"file://" + filepath.Join(dir+fmt.Sprintf("current_file_%d.txt", i)),
			)
			require.NoError(t, err)

			buffer := cell.NewBuffer()
			buffer.Write([]byte(tcase.contentsToReadTo))

			h, err := c.Edit(currentURI, buffer, false, false)
			require.NoError(t, err)

			h.SetCursorAtScroll(tcase.cursorPosition)
			require.NoError(t, err)

			err = c.ReadFile(fileToReadURI, h)
			require.NoError(t, err)

			cells := h.CellView().RawCells()
			require.NoError(t, err)
			assert.Equal(t, tcase.expectedResult, term.CellsToString(cells))
		})
	}
}
