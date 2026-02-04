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

package vctrl

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
)

func TestDiffUtils(t *testing.T) {
	suite := []struct {
		src, dst    string
		expChanges  []diffmatchpatch.Diff
		expFileDiff FileDiff
	}{
		{
			src:         "",
			dst:         "",
			expChanges:  []diffmatchpatch.Diff{},
			expFileDiff: FileDiff{},
		},
		{
			src: "a",
			dst: "a",
			expChanges: []diffmatchpatch.Diff{
				{
					Type: 0,
					Text: "a",
				},
			},
			expFileDiff: FileDiff{
				Hunks: nil,
			},
		},
		{
			src: "",
			dst: "abc\ncba",
			expChanges: []diffmatchpatch.Diff{
				{
					Type: 1,
					Text: "abc\ncba",
				},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						NewStartLine:  1,
						OrigStartLine: 1,
						OrigLines:     0,
						NewLines:      2,
						Body:          "+abc\n+cba",
					},
				},
			},
		},
		{
			src: "",
			dst: "abc\n\ncba",
			expChanges: []diffmatchpatch.Diff{
				{
					Type: 1,
					Text: "abc\n\ncba",
				},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						NewStartLine:  1,
						OrigStartLine: 1,
						OrigLines:     0,
						NewLines:      3,
						Body:          "+abc\n+\n+cba",
					},
				},
			},
		},
		{
			src: "abc\ncba",
			dst: "",
			expChanges: []diffmatchpatch.Diff{
				{
					Type: -1,
					Text: "abc\ncba",
				},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						NewStartLine:  1,
						OrigStartLine: 1,
						OrigLines:     2,
						NewLines:      0,
						Body:          "-abc\n-cba",
					},
				},
			},
		},
		{
			src: "abc\n\ncba",
			dst: "",
			expChanges: []diffmatchpatch.Diff{
				{
					Type: -1,
					Text: "abc\n\ncba",
				},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						NewStartLine:  1,
						OrigStartLine: 1,
						OrigLines:     3,
						NewLines:      0,
						Body:          "-abc\n-\n-cba",
					},
				},
			},
		},
		{
			src: "abc\nbcd\ncde",
			dst: "000\nabc\n111\nBCD\n",
			expChanges: []diffmatchpatch.Diff{
				{Type: 1, Text: "000\n"},
				{Type: 0, Text: "abc\n"},
				{Type: -1, Text: "bcd\ncde"},
				{Type: 1, Text: "111\nBCD\n"},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OrigStartLine: 1,
						OrigLines:     0,
						NewStartLine:  1,
						NewLines:      1,
						Body:          "+000\n",
					},
					{
						OrigStartLine: 3,
						OrigLines:     2,
						NewStartLine:  3,
						NewLines:      0,
						Body:          "-bcd\n-cde",
					},
					{
						OrigStartLine: 3,
						OrigLines:     0,
						NewStartLine:  3,
						NewLines:      2,
						Body:          "+111\n+BCD\n",
					},
				},
			},
		},
		{
			src: "A\nB\nC\nD\nE\nF\nG\nH\nI\nJ\nK\nL\nM\nN\nÑ\nO\nP\nQ\nR\nS\nT\nU\nV\nW\nX\nY\nZ",
			dst: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\nY\nZ",
			expChanges: []diffmatchpatch.Diff{
				{Type: -1, Text: "A\n"},
				{Type: 0, Text: "B\nC\nD\nE\nF\nG\n"},
				{Type: -1, Text: "H\n"},
				{Type: 0, Text: "I\nJ\nK\nL\nM\nN\n"},
				{Type: -1, Text: "Ñ\n"},
				{Type: 0, Text: "O\nP\nQ\nR\nS\nT\n"},
				{Type: -1, Text: "U\n"},
				{Type: 0, Text: "V\nW\nX\nY\nZ"},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OrigStartLine: 1,
						OrigLines:     1,
						NewStartLine:  1,
						NewLines:      0,
						Body:          "-A\n",
					},
					{
						OrigStartLine: 7,
						OrigLines:     1,
						NewStartLine:  7,
						NewLines:      0,
						Body:          "-H\n",
					},
					{
						OrigStartLine: 13,
						OrigLines:     1,
						NewStartLine:  13,
						NewLines:      0,
						Body:          "-Ñ\n",
					},
					{
						OrigStartLine: 19,
						OrigLines:     1,
						NewStartLine:  19,
						NewLines:      0,
						Body:          "-U\n",
					},
				},
			},
		},
		{
			src: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\nY\nZ",
			dst: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\nY\n",
			expChanges: []diffmatchpatch.Diff{
				{Type: 0, Text: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\nY\n"},
				{Type: -1, Text: "Z"},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OrigStartLine: 23,
						OrigLines:     1,
						NewStartLine:  23,
						NewLines:      0,
						Body:          "-Z",
					},
				},
			},
		},
		{
			src: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\nY\nZ",
			dst: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\nY",
			expChanges: []diffmatchpatch.Diff{
				{Type: 0, Text: "B\nC\nD\nE\nF\nG\nI\nJ\nK\nL\nM\nN\nO\nP\nQ\nR\nS\nT\nV\nW\nX\n"},
				{Type: -1, Text: "Y\nZ"},
				{Type: 1, Text: "Y"},
			},
			expFileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OrigStartLine: 22,
						OrigLines:     2,
						NewStartLine:  22,
						NewLines:      0,
						Body:          "-Y\n-Z",
					},
					{
						OrigStartLine: 22,
						OrigLines:     0,
						NewStartLine:  22,
						NewLines:      1,
						Body:          "+Y",
					},
				},
			},
		},
	}

	for i, test := range suite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			uri, err := workspaceapi.ParseURI("file:///tmp")
			require.NoError(t, err)

			a := cell.NewBuffer()
			a.ReadFrom(strings.NewReader(test.src))
			b := cell.NewBuffer()
			b.ReadFrom(strings.NewReader(test.dst))

			ctx := context.Background()
			changes := Diff(ctx, a.String(), b.String())
			assert.Equal(t, test.expChanges, changes)

			t.Run("ApplyChanges", func(t *testing.T) {
				ApplyChanges(ctx, a, changes)
				assert.Equal(t, b.String(), a.String())
			})

			t.Run("ConvertChangesToFileDiff", func(t *testing.T) {
				diff := ConvertChangesToFileDiff(uri, changes)
				assert.Equal(t, "/tmp", diff.OrigName)
				assert.Equal(t, "/tmp", diff.NewName)
				diff.OrigName = ""
				diff.NewName = ""
				assert.Equal(t, test.expFileDiff, diff)
			})
		})
	}
}
