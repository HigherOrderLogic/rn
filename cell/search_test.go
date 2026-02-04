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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func testSearch(t *testing.T, constructor func(*Buffer) Searcher) {
	t.Run("searches for occurrences of a word", func(t *testing.T) {
		r := NewBuffer()
		_, _ = r.ReadFrom(strings.NewReader("\thello"))
		s := constructor(r)
		require.Equal(t, 1, s.Search("hello"))

		res, ok := s.NextResult()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{X: 1}, res)
	})

	t.Run("searches for occurrences of a >1 width rune", func(t *testing.T) {
		r := NewBuffer()
		_, _ = r.ReadFrom(strings.NewReader("\nni hao!\n你好"))
		s := constructor(r)
		require.Equal(t, 1, s.Search("好"))

		res, ok := s.NextResult()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{X: 1, Y: 2}, res)
	})

	t.Run("searches for occurrences with multiple words", func(t *testing.T) {
		r := NewBuffer()
		_, _ = r.ReadFrom(strings.NewReader("a b\nc d e f g h i j k\n"))
		s := constructor(r)
		require.Equal(t, 1, s.Search(" g h i"))

		res, ok := s.NextResult()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{X: 7, Y: 1}, res)
	})

	t.Run("searches for occurrences with tabspaces", func(t *testing.T) {
		r := NewBuffer()
		_, _ = r.ReadFrom(strings.NewReader("a b\nc\td"))
		s := constructor(r)
		require.Equal(t, 1, s.Search("\td"))

		res, ok := s.NextResult()
		require.True(t, ok)
		assert.Equal(t, term.Coordinates{X: 1, Y: 1}, res)
	})

	t.Run("returns 0 if there are no matches", func(t *testing.T) {
		r := NewBuffer()
		_, _ = r.ReadFrom(strings.NewReader("\thello"))
		s := constructor(r)
		require.Equal(t, 0, s.Search("bollocks"))

		_, ok := s.NextResult()
		require.False(t, ok)
	})
}

func TestSimpleSearcher(t *testing.T) {
	testSearch(t, func(buf *Buffer) Searcher {
		return NewSimpleSearcher(buf)
	})
}
