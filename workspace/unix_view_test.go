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

package workspace

import (
	"context"
	"fmt"
	"io"

	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
)

const testFilesLines = 3

var fileWithNoEOL string
var fileWithEOL string

func init() {
	f, err := os.CreateTemp("", "test_raw_cells_1_")
	if err != nil {
		return
	}

	f.WriteString("LINE")
	for i := 1; i < testFilesLines; i++ {
		_, err := f.WriteString("\nLINE")
		if err != nil {
			return
		}
	}

	fileWithNoEOL = f.Name()
	f.Close()

	f, err = os.CreateTemp("", "test_raw_cells_2_")
	if err != nil {
		return
	}
	defer f.Close()

	for i := 0; i < testFilesLines; i++ {
		_, err := f.WriteString("LINE\n")
		if err != nil {
			return
		}
	}

	fileWithEOL = f.Name()
}

func TestUnixFile(t *testing.T) {
	require.NotZero(t, fileWithNoEOL)
	require.NotZero(t, fileWithEOL)

	f, err := os.Open(fileWithNoEOL)
	require.NoError(t, err)
	defer f.Close()

	f2, err := os.Open(fileWithEOL)
	require.NoError(t, err)
	defer f2.Close()

	tsuite := []struct {
		input    io.Reader
		expected string
		rows     int
	}{
		{strings.NewReader(""), "", 1},
		{strings.NewReader("fjelkwfjlkew"), "fjelkwfjlkew", 1},
		{strings.NewReader("fjelkwfjlkew\nfewjklfe"), "fjelkwfjlkew\nfewjklfe", 2},
		{f, "LINE\nLINE\nLINE", testFilesLines},
		{f2, "LINE\nLINE\nLINE", testFilesLines},
	}

	for i, tcase := range tsuite {
		reader := tcase.input
		{
			c := cell.NewBuffer()
			_, err := c.ReadFrom(tcase.input)
			require.NoError(t, err)

			reader := NewUnixFileView(c)

			assert.Equal(t, tcase.rows, reader.Rows(), fmt.Sprintf("ReadFrom(%d)", i))
			assert.Equal(t, tcase.expected, reader.String(), i)
		}

		s := reader.(io.Seeker)
		s.Seek(0, 0)

		{
			c := cell.NewBuffer()
			bytes, err := io.ReadAll(tcase.input)
			require.NoError(t, err)
			c.Edit(context.Background(), term.Coordinates{},
				term.Coordinates{}, string(bytes))

			reader := NewUnixFileView(c)

			assert.Equal(t, tcase.rows, reader.Rows(), fmt.Sprintf("insert(%d)", i))
			assert.Equal(t, tcase.expected, reader.String(), i)
		}
	}
}

func TestBufferViewIntegration(t *testing.T) {
	const snippet = "If you accept Hawking radiation and accept " +
		"that black holes radiate away all the information stored inside, eventually, " +
		"if you reverse the arrow of time, you'll find that the start of the universe " +
		"is actually information being injected into black holes, which eventually " +
		"start to spit out particles, stars, galaxies and even life."

	suite := []struct {
		desc    string
		content string
	}{
		{"reads reader content into buffer after reset", snippet},
		{"reads reader content into buffer after reset, with last EOL", snippet + "\n"},
	}

	for _, test := range suite {
		t.Run(test.desc, func(t *testing.T) {
			b := cell.NewBuffer()
			view := NewUnixFileView(b.View())
			b.WithView(view)

			_, err := b.ReadFrom(strings.NewReader(test.content))
			require.NoError(t, err)
			if !view.EndsWithEOL() {
				b.WriteString("\n")
			}
			require.Equal(t, snippet, b.String())

			for i := 0; i < 2; i++ {
				b.Reset()
				_, err = b.ReadFrom(strings.NewReader(test.content))
				require.NoError(t, err)
				if !view.EndsWithEOL() {
					b.WriteString("\n")
				}
				assert.Equal(t, snippet, b.String(), i)
			}
		})
	}
}
