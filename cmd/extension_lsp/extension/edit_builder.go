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
	"errors"
	"fmt"
	"sort"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/golang-internal-tools/lsp/protocol"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
)

type editBuilder struct {
	f        *file
	w        textapi.CellEditor
	original *cell.Buffer
	buf      *cell.Buffer
	colmap   protocol.ColumnMapper
}

func (b *editBuilder) init(tabspaces int, f *file, w textapi.CellEditor, cells [][]term.Cell) {
	b.f = f
	b.w = w
	b.original = cell.CellsToBuffer(cells)
	b.buf = cell.CellsToBuffer(cells)
	spanURI := workspaceURIToSpan(b.f.uri)
	b.colmap = getColumnMapper(spanURI, b.original)
}

func (b *editBuilder) applyEdit(ed protocol.TextEdit) error {
	start, end, ok := convertRange(ed.Range, b.original.RawCells(), b.colmap)
	if !ok {
		return errors.New("could not convert rage")
	}
	ctx := context.Background()
	// update remote and local buffer
	_, _, _, err := b.w.Edit(ctx, start, end, ed.NewText)
	_, _, _ = b.buf.Edit(ctx, start, end, ed.NewText)
	return err
}

func (b *editBuilder) applyWorkspaceEdit(ed protocol.WorkspaceEdit) (ret error) {
	for _, ch := range ed.DocumentChanges {
		if ch.TextDocument.TextDocumentIdentifier != b.f.docID {
			continue
		}
		if err := b.applyEdits(ch.Edits); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	return ret
}

func sortEdits(eds []protocol.TextEdit) {
	sort.Slice(eds, func(i, j int) bool {
		if eds[i].Range.Start.Line > eds[j].Range.Start.Line {
			return true
		}
		if eds[i].Range.Start.Line < eds[j].Range.Start.Line {
			return false
		}
		if eds[i].Range.Start.Character > eds[j].Range.Start.Character {
			return true
		}
		if eds[i].Range.Start.Character < eds[j].Range.Start.Character {
			return false
		}
		return i > j
	})
}

func (b *editBuilder) applyEdits(eds []protocol.TextEdit) (ret error) {
	sortEdits(eds)
	for _, ed := range eds {
		err := b.applyEdit(ed)
		if err != nil {
			ret = multierr.Append(fmt.Errorf("applyEdit(%s): %v", b.f.uri, err))
		}
	}
	return ret
}
