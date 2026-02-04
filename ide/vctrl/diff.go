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
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/unstablebuild/tcell/v3"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Diff computes the (line oriented) modifications needed to turn the src
// string into the dst string. If the given context has a deadline, then this
// is used as a timeout for DiffWithTimeout, otherwise a default sane value
// is used.
func Diff(ctx context.Context, src, dst string) (diffs []diffmatchpatch.Diff) {
	deadline, ok := ctx.Deadline()
	if ok {
		return DiffWithTimeout(src, dst, time.Until(deadline))
	}
	return DiffWithTimeout(src, dst, 10*time.Second)
}

// DiffWithTimeout computes the (line oriented) modifications needed to turn the src
// string into the dst string. The `timeout` argument specifies the maximum
// amount of time it is allowed to spend in this function.
func DiffWithTimeout(src, dst string, timeout time.Duration) (diffs []diffmatchpatch.Diff) {
	// taken from diffmatchpatch.New, but without alloc
	dmp := diffmatchpatch.DiffMatchPatch{
		DiffTimeout:          time.Second,
		DiffEditCost:         4,
		MatchThreshold:       0.5,
		MatchDistance:        1000,
		PatchDeleteThreshold: 0.5,
		PatchMargin:          4,
		MatchMaxBits:         32,
	}
	dmp.DiffTimeout = timeout
	wSrc, wDst, warray := dmp.DiffLinesToRunes(src, dst)
	diffs = dmp.DiffMainRunes(wSrc, wDst, false)
	diffs = dmp.DiffCharsToLines(diffs, warray)
	return diffs
}

// ApplyChanges applies the given diff changes to the given editor.
func ApplyChanges(ctx context.Context, ed cell.Editor, changes []diffmatchpatch.Diff) {
	var cursor int
	for _, change := range changes {
		switch change.Type {
		case diffmatchpatch.DiffDelete:
			from := term.Coordinates{Y: cursor}
			to := from
			for _, ch := range change.Text {
				switch ch {
				case '\n':
					to.Y++
					to.X = 0
				default:
					to.X++
				}
			}
			ed.Edit(ctx, from, to, "")
		case diffmatchpatch.DiffInsert:
			at := term.Coordinates{Y: cursor}
			_, until, _ := ed.Edit(ctx, at, at, change.Text)
			cursor = until.Y
		case diffmatchpatch.DiffEqual:
			for _, ch := range change.Text {
				switch ch {
				case '\n':
					cursor++
				default:
				}
			}
		}
	}
}

// ConvertChangesToFileDiff converts the given slice of diff changes into
// a FileDiff.
func ConvertChangesToFileDiff(
	file workspaceapi.URI, changes []diffmatchpatch.Diff,
) (ret FileDiff) {
	ret.OrigName = file.Path()
	ret.NewName = ret.OrigName
	var cursor int
	for _, change := range changes {
		diffs := [1]diffmatchpatch.Diff{change}
		body := DiffString(diffs[:])

		lines := strings.Split(change.Text, "\n")
		linesWithoutLastEmpty := len(lines)
		if len(lines) > 0 {
			if lines[len(lines)-1] == "" {
				linesWithoutLastEmpty--
			}
		}
		switch change.Type {
		case diffmatchpatch.DiffDelete:
			from := cursor + 1
			ret.Hunks = append(ret.Hunks, Hunk{
				OrigStartLine: int32(from),
				OrigLines:     int32(linesWithoutLastEmpty),
				NewStartLine:  int32(from),
				NewLines:      0,
				Body:          body,
			})
		case diffmatchpatch.DiffInsert:
			from := cursor + 1
			ret.Hunks = append(ret.Hunks, Hunk{
				OrigStartLine: int32(from),
				OrigLines:     0,
				NewStartLine:  int32(from),
				NewLines:      int32(linesWithoutLastEmpty),
				Body:          body,
			})
			cursor += linesWithoutLastEmpty
		case diffmatchpatch.DiffEqual:
			cursor += linesWithoutLastEmpty
		}
	}
	return
}

// DiffString converts the given changes into a diff.
func DiffString(diffs []diffmatchpatch.Diff) string {
	scroll := DiffComponent(diffs).(*component.Scroll)
	return scroll.Buffer().String()
}

// DiffComponent converts the given changes into a string
// component with background and foreground attributes set.
func DiffComponent(diffs []diffmatchpatch.Diff) tui.Component {
	buf := cell.NewBuffer()
	cursor := term.Coordinates{}
	for _, diff := range diffs {
		text := diff.Text
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if i < len(lines)-1 {
					_, cursor = buf.InsertStringWithAttr(cursor, "+", addAttr)
					_, cursor = buf.InsertStringWithAttr(cursor, line, addAttr)
					_, cursor = buf.InsertString(cursor, "\n")
				} else if line != "" {
					_, cursor = buf.InsertStringWithAttr(cursor, "+", addAttr)
					_, cursor = buf.InsertStringWithAttr(cursor, line, addAttr)
				}
			}

		case diffmatchpatch.DiffDelete:
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if i < len(lines)-1 {
					_, cursor = buf.InsertStringWithAttr(cursor, "-", delAttr)
					_, cursor = buf.InsertStringWithAttr(cursor, line, delAttr)
					_, cursor = buf.InsertString(cursor, "\n")
				} else if line != "" {
					_, cursor = buf.InsertStringWithAttr(cursor, "-", delAttr)
					_, cursor = buf.InsertStringWithAttr(cursor, line, delAttr)
				}
			}
		case diffmatchpatch.DiffEqual:
			_, cursor = buf.InsertString(cursor, text)
		}
	}

	return component.NewScroll(buf)
}

var (
	addAttr = term.Attributes{Bg: tcell.ColorGreen}
	delAttr = term.Attributes{Bg: tcell.ColorRed}
)
