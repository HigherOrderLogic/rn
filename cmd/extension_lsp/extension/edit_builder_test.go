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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/golang-internal-tools/lsp/protocol"
	"github.com/unstablebuild/golang-internal-tools/span"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
)

const (
	fixture1 = `package config

import (
	"io"
	"os"
	"go.uber.org/config"
	"strings"
)
`
	expected1 = `package config

import (
	"go.uber.org/config"
	"strings"
)
`
	expected2 = `package config

import (
	"go.uber.org/config"
	"io"
	"os"
	"strings"
)
`
)

var (
	edit1 = protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 2},
			End:   protocol.Position{Line: 5, Character: 2},
		},
	}
	edit2 = protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: 5, Character: 20},
			End:   protocol.Position{Line: 5, Character: 20},
		},
		NewText: "\"\n\t\"io\"\n\t\"os",
	}
	edit3 = protocol.TextEdit{Range: protocol.Range{
		Start: protocol.Position{Line: 2, Character: 0},
		End:   protocol.Position{Line: 2, Character: 6}},
	}
	edit4 = protocol.TextEdit{Range: protocol.Range{
		Start: protocol.Position{Line: 2, Character: 6},
		End:   protocol.Position{Line: 2, Character: 6}},
		NewText: "imp",
	}
	edit5 = protocol.TextEdit{Range: protocol.Range{
		Start: protocol.Position{Line: 2, Character: 6},
		End:   protocol.Position{Line: 2, Character: 6}},
		NewText: "ort",
	}
	edits1 = []protocol.TextEdit{edit1}
	edits2 = []protocol.TextEdit{edit1, edit2}
	edits3 = []protocol.TextEdit{edit3, edit4, edit5}
)

func makeFile() *file {
	name := "gopls_espavila.go"
	uri := span.URIFromPath(name)
	docID := protocol.TextDocumentIdentifier{
		URI: protocol.URIFromSpanURI(uri),
	}
	u, err := workspaceapi.ParseURI(string(uri))
	if err != nil {
		panic(err)
	}
	return &file{uri: u, docID: docID}
}

func TestApplyEdits(t *testing.T) {
	tsuite := []struct {
		input  string
		ed     []protocol.TextEdit
		output string
	}{
		{"a", nil, "a"},
		{fixture1, edits1, expected1},
		{fixture1, edits2, expected2},
		{fixture1, edits3, fixture1},
	}

	for _, tcase := range tsuite {
		var out cell.Buffer
		out.Init()
		out.WriteString(tcase.input)

		var b editBuilder
		b.init(4, makeFile(), wrapEditor{out.Editor()}, term.StringToCells(tcase.input))
		edits := make([]protocol.TextEdit, len(tcase.ed))
		copy(edits, tcase.ed)
		b.applyEdits(edits)
		assert.Equal(t, tcase.output, b.buf.String())
		assert.Equal(t, tcase.output, out.String())
	}
}

func TestSortEdits(t *testing.T) {
	tsuite := []struct {
		in  []protocol.TextEdit
		out []protocol.TextEdit
	}{
		{edits1, edits1},
		{edits2, []protocol.TextEdit{edit2, edit1}},
		{edits3, []protocol.TextEdit{edit5, edit4, edit3}},
	}
	for _, tcase := range tsuite {
		edits := make([]protocol.TextEdit, len(tcase.in))
		copy(edits, tcase.in)
		sortEdits(edits)
		assert.Equal(t, tcase.out, edits)
	}
}
