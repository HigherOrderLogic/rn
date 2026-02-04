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

package textrpc

import (
	"context"

	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	"unstable.build/go-tui/cell"
)

// NewEditRequest converts a buf into an EditRequest.
func NewEditRequest(
	file workspaceapi.URI, buf *cell.Buffer, readOnly, recovered bool,
) textrpc.EditRequest {
	return textrpc.EditRequest{
		Buffer:       rawCellsToProtoCells(buf.RawCells()),
		ResourceName: NewURI(file),
		ReadOnly:     readOnly,
		Recovered:    recovered,
	}
}

// EditRequestToBuffer converts an EditRequest into a cell.Buffer
func EditRequestToBuffer(in *textrpc.EditRequest) *cell.Buffer {
	return rowsToBuffer(in.GetBuffer())
}

// NewURIFromProto maps rpc.URI into a workspaceapi.URI.
func NewURIFromProto(u *textrpc.URI) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI(u.GetUri())
}

// NewURI maps a workspaceapi.URI into a rpc.URI.
func NewURI(u workspaceapi.URI) *textrpc.URI {
	return &textrpc.URI{Uri: u.String()}
}

// RawCellsResponseToBuffer converts an EditRequest into a cell.Buffer
func RawCellsResponseToBuffer(in *textrpc.RawCellsResponse) *cell.Buffer {
	return rowsToBuffer(in.GetRows())
}

// NewRawCellsResponse converts a buf into an RawCellsResponse.
func NewRawCellsResponse(cells [][]term.Cell) *textrpc.RawCellsResponse {
	return &textrpc.RawCellsResponse{Rows: rawCellsToProtoCells(cells)}
}

func rowsToBuffer(in []*termrpc.CellRow) *cell.Buffer {
	var maxWidth int
	for _, row := range in {
		if len(row.Cells) > maxWidth {
			maxWidth = len(row.Cells)
		}
	}

	var w cell.BufferWriter
	w.Init(context.Background(), maxWidth, len(in))

	for y, rows := range in {
		for x, cell := range rows.Cells {
			w.SetCell(term.Coordinates{X: x, Y: y}, cell.ToModel())
		}
	}

	ret := new(cell.Buffer)
	w.ToBuffer(ret)
	return ret
}

func rawCellsToProtoCells(cells [][]term.Cell) []*termrpc.CellRow {
	var size int
	for _, row := range cells {
		size += len(row)
	}

	// these slabs reduce allocations from ~N (=num cells)
	// to 4 which reduces this function's ns/op from 60 to 80%
	rows := make([]*termrpc.CellRow, len(cells))
	protoCellRowSlabPtr := make([]termrpc.CellRow, len(cells))
	cellRowSlab := make([]*termrpc.Cell, size)
	cellRowSlabIdx := 0
	cellSlabPtr := make([]termrpc.Cell, size)
	cellSlabPtrIdx := 0

	for y, row := range cells {
		cells := cellRowSlab[cellRowSlabIdx : cellRowSlabIdx+len(row)]
		cellRowSlabIdx += len(row)
		for x, cell := range row {
			c := &cellSlabPtr[cellSlabPtrIdx]
			cellSlabPtrIdx++
			c.FromModel(cell)
			cells[x] = c
		}
		protoCellRowSlabPtr[y].Cells = cells
		rows[y] = &protoCellRowSlabPtr[y]
	}

	return rows
}
