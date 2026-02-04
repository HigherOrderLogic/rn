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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestWriterContext(t *testing.T) {
	type myKey string

	writer := NewBufferWriter(
		context.WithValue(context.Background(), myKey("a"), "a"), 10, 10)

	value := writer.Context().Value(myKey("a"))
	require.NotNil(t, value)
	str, ok := value.(string)
	require.True(t, ok)
	assert.Equal(t, "a", str)

	writer.SetContext(
		context.WithValue(context.Background(), myKey("b"), "b"),
	)

	value = writer.Context().Value(myKey("a"))
	require.Nil(t, value)

	value = writer.Context().Value(myKey("b"))
	require.NotNil(t, value)
	str, ok = value.(string)
	require.True(t, ok)
	assert.Equal(t, "b", str)
}

func TestWriteFlush(t *testing.T) {
	width, height := 5, 6
	writer := NewBufferWriter(context.Background(), width, height)

	c := 'E'
	for i := width - 1; i >= 0; i-- {
		for j := height - 1; j >= 0; j-- {
			if i > j-1 {
				writer.SetCell(term.Coordinates{X: j, Y: i}, term.Cell{Ch: c})
			}
		}
		c--
	}

	// should be fine to wtry to write out of bounds
	writer.SetCell(term.Coordinates{X: width + 1, Y: height + 1}, term.Cell{Ch: '='})

	require.NoError(t, writer.Flush())

	expected := "A\x00\x00\x00\x00\nBB\x00\x00\x00\n" +
		"CCC\x00\x00\nDDDD\x00\nEEEEE\n\x00\x00\x00\x00\x00"
	assert.Equal(t, expected, term.CellsToString(writer.RawCells()))
}

func benchBufferWriter(b *testing.B, n int) {
	width, height := n, n
	writer := NewBufferWriter(context.Background(), width, height)
	for i := 0; i < b.N; i++ {
		writer.Clear(term.Attributes{})
		c := 'E'
		for i := width - 1; i >= 0; i-- {
			for j := height - 1; j >= 0; j-- {
				if i > j-1 {
					writer.SetCell(term.Coordinates{X: j, Y: i}, term.Cell{Ch: c})
				}
			}
			c--
		}
	}
}

func BenchmarkBufferWriter10(b *testing.B) {
	benchBufferWriter(b, 10)
}
func BenchmarkBufferWriter100(b *testing.B) {
	benchBufferWriter(b, 100)
}
func BenchmarkBufferWriter1000(b *testing.B) {
	benchBufferWriter(b, 1000)
}
