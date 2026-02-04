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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	"unstable.build/go-tui/cell"
)

func TestBufferEditRequest(t *testing.T) {
	tsuite := []struct {
		in  string
		out textrpc.EditRequest
	}{
		{
			in: "a",
			out: textrpc.EditRequest{
				ResourceName: &textrpc.URI{Uri: ""},
				Buffer: []*termrpc.CellRow{
					{Cells: []*termrpc.Cell{{Character: 'a'}}},
				},
			},
		},
		{
			in: "a\nbb\nccc",
			out: textrpc.EditRequest{
				ResourceName: &textrpc.URI{Uri: ""},
				Buffer: []*termrpc.CellRow{
					{Cells: []*termrpc.Cell{{Character: 'a'}}},
					{Cells: []*termrpc.Cell{{Character: 'b'}, {Character: 'b'}}},
					{Cells: []*termrpc.Cell{{Character: 'c'}, {Character: 'c'}, {Character: 'c'}}},
				},
			},
		},
	}

	for _, tcase := range tsuite {
		buf := cell.NewBuffer()
		buf.WriteString(tcase.in)
		out := NewEditRequest(workspaceapi.URI{}, buf, false, false)
		assert.Equal(t, tcase.out, out)

		outbuf := EditRequestToBuffer(&out)
		assert.Equal(t, tcase.in, outbuf.String())
	}
}

func benchmarkEditRequest(b *testing.B, width, height int) {
	var str string
	for i := 0; i < width; i++ {
		str += "fjkelwjflk\njflw\njfklewfkjlkew\n"
	}

	buf := cell.NewBuffer()
	buf.WriteString(str)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewEditRequest(workspaceapi.URI{}, buf, false, false)
	}
}

func BenchmarkEditRequestTiny(b *testing.B) {
	benchmarkEditRequest(b, 5, 5)
}
func BenchmarkEditRequestSmall(b *testing.B) {
	benchmarkEditRequest(b, 50, 50)
}
func BenchmarkEditRequestMedium(b *testing.B) {
	benchmarkEditRequest(b, 500, 500)
}
func BenchmarkEditRequestBig(b *testing.B) {
	benchmarkEditRequest(b, 5000, 5000)
}
