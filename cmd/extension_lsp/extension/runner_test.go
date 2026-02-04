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

package extension

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/golang-internal-tools/lsp/protocol"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
)

//go:embed test/*.in
var testin embed.FS

//go:embed test/*.want
var testout embed.FS

func TestLSPFormatting(t *testing.T) {
	baseDir := "test"
	entries, err := testin.ReadDir(baseDir)
	require.NoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		extension := filepath.Ext(name)
		name = name[0 : len(name)-len(extension)]

		input := entry.Name()
		output := fmt.Sprintf("%s.want", name)
		t.Run(fmt.Sprintf("%s", name), func(t *testing.T) {
			in, err := testin.Open(path.Join(baseDir, input))
			require.NoError(t, err)

			out, err := testout.Open(path.Join(baseDir, output))
			require.NoError(t, err)

			want, err := io.ReadAll(out)
			require.NoError(t, err)

			buffer := cell.NewBuffer()
			_, err = buffer.ReadFrom(in)
			require.NoError(t, err)
			editsRaw := term.CellsToString(buffer.RawCells()[buffer.Rows()-2:])
			var edits []protocol.TextEdit
			err = json.Unmarshal([]byte(editsRaw), &edits)
			require.NoError(t, err)

			var b editBuilder
			b.init(4, makeFile(), wrapEditor{buffer.Editor()},
				term.StringToCells(buffer.String()))
			b.applyEdits(edits)
			assert.Equal(t, string(want), b.buf.String())
		})
	}
}

type wrapEditor struct {
	ed cell.Editor
}

func (w wrapEditor) Edit(ctx context.Context, start, end term.Coordinates, new string) (
	from, to term.Coordinates, old string, err error,
) {
	from, to, old = w.ed.Edit(ctx, start, end, new)
	return
}
