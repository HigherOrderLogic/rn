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

package syntax

import (
	"strconv"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

/* NOTE: this is a port of nvim-treesitter's indents implementation,
   Licensed under Apache 2.0. Ported to Go, on October 2025.
*/

type captures struct {
	begin  map[uintptr]map[string]*string
	end    map[uintptr]map[string]*string
	branch map[uintptr]map[string]*string
	zero   map[uintptr]map[string]*string
	ignore map[uintptr]map[string]*string
	dedent map[uintptr]map[string]*string
	align  map[uintptr]map[string]*string
	auto   map[uintptr]map[string]*string
}

func newCaptures() captures {
	return captures{
		begin:  map[uintptr]map[string]*string{},
		end:    map[uintptr]map[string]*string{},
		branch: map[uintptr]map[string]*string{},
		zero:   map[uintptr]map[string]*string{},
		ignore: map[uintptr]map[string]*string{},
		dedent: map[uintptr]map[string]*string{},
		align:  map[uintptr]map[string]*string{},
		auto:   map[uintptr]map[string]*string{},
	}
}

func (t *tree) queryIndentCaptures() (ret captures) {
	ret = newCaptures()
	root := t.tree.RootNode()

	cur := tree_sitter.NewQueryCursor()
	defer cur.Close()

	captureNames := t.indents.CaptureNames()
	matches := cur.Matches(t.indents, root, []byte(t.buf.String()))
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		props := t.indents.PropertySettings(m.PatternIndex)
		propsMap := make(map[string]*string)
		for _, prop := range props {
			propsMap[prop.Key] = prop.Value
		}
		for _, cap := range m.Captures {
			name := captureNames[cap.Index]
			n := cap.Node
			switch name {
			case "indent.begin":
				ret.begin[n.Id()] = propsMap
			case "indent.dedent":
				ret.dedent[n.Id()] = propsMap
			case "indent.align":
				ret.align[n.Id()] = propsMap
			case "indent.auto":
				ret.auto[n.Id()] = propsMap
			case "indent.end":
				ret.end[n.Id()] = propsMap
			case "indent.branch":
				ret.branch[n.Id()] = propsMap
			case "indent.zero":
				ret.zero[n.Id()] = propsMap
			case "indent.ignore":
				ret.ignore[n.Id()] = propsMap
			default:
				// be forwards compatible with new indents.scm files
			}
		}
	}
	return
}

func (t *tree) getIndentation(line uint) (ret int) {
	captures := t.queryIndentCaptures()
	isEmptyLine := t.buf.Columns(int(line)) == 0
	var node *tree_sitter.Node
	if isEmptyLine {
		prevLine := t.getPreviousNonBlankLine(line)
		node = t.getLastNodeAtLine(prevLine)
		// TODO if node is a comment...
		if _, ok := captures.end[node.Id()]; ok {
			node = t.getFirstNodeAtLine(line)
		}
	} else {
		node = t.getFirstNodeAtLine(line)
	}

	if _, ok := captures.zero[node.Id()]; ok {
		return 0
	}

	indent := 0
	isProcessedByRow := make(map[uint]bool)
	for node != nil {
		beginMetadata, isBegin := captures.begin[node.Id()]
		_, isDedent := captures.dedent[node.Id()]
		alignMetadata, isAlign := captures.align[node.Id()]
		_, isAuto := captures.auto[node.Id()]
		_, isIgnore := captures.ignore[node.Id()]
		_, isBranch := captures.branch[node.Id()]
		rng := node.Range()
		startRow := rng.StartPoint.Row
		endRow := rng.EndPoint.Row

		// do autoindent if not marked with @indent
		if !isBegin && !isAlign && isAuto &&
			startRow < line &&
			line <= endRow {
			return -1
		}

		// do not indent inside ignore block
		if !isBegin && isIgnore &&
			startRow < line &&
			line <= endRow {
			return 0
		}

		var isProcessed bool
		if !isProcessedByRow[startRow] &&
			((isBranch && startRow == line) || (isDedent && startRow == line)) {
			indent--
			isProcessed = true
		}

		shouldProcess := !isProcessedByRow[startRow]
		isInErr := false

		if shouldProcess {
			parent := node.Parent()
			if parent != nil {
				isInErr = parent.HasError()
			}
		}
		_, beginHasImmediate := beginMetadata["indent.immediate"]
		_, beginHasStartAtSameLine := beginMetadata["indent.start_at_same_line"]
		if shouldProcess && isBegin &&
			(startRow != endRow || isInErr || beginHasImmediate) &&
			(startRow != line || beginHasStartAtSameLine) {
			indent++
			isProcessed = true
		}

		if isInErr && !isAlign {
			// only when the node is in error, promote the
			// first child's aligned indent to the error node
			cursor := t.tree.Walk()
			defer cursor.Close()
			for _, child := range node.Children(cursor) {
				childAlignMetadata, childIsAlign := captures.align[child.Id()]
				if childIsAlign {
					alignMetadata, isAlign = childAlignMetadata, childIsAlign
				}
			}
		}

		if shouldProcess && isAlign && (endRow != startRow || isInErr) && startRow != line {
			metadata := alignMetadata
			var odelimNode *tree_sitter.Node
			var oIsLastLine bool
			var cdelimNode *tree_sitter.Node
			var cIsLastLine bool
			var indentIsAbsolute bool

			openDelimiter, isOpenDelimiter := metadata["indent.open_delimiter"]
			closeDelimiter, isCloseDelimiter := metadata["indent.close_delimiter"]

			if isOpenDelimiter && openDelimiter != nil {
				odelimNode, oIsLastLine = t.findDelimiter(node, *openDelimiter)
			} else {
				odelimNode = node
			}
			if isCloseDelimiter && closeDelimiter != nil {
				cdelimNode, cIsLastLine = t.findDelimiter(node, *closeDelimiter)
			} else {
				cdelimNode = node
			}

			if odelimNode == nil {
				isProcessedByRow[startRow] = isProcessedByRow[startRow] || isProcessed
				node = node.Parent()
				continue
			}

			osRow := odelimNode.Range().StartPoint.Row
			osCol := odelimNode.Range().StartPoint.Column
			var csRow *uint
			if cdelimNode != nil {
				csRow = new(uint)
				*csRow = cdelimNode.Range().StartPoint.Row
			}

			if oIsLastLine {
				if shouldProcess {
					indent++
					if cIsLastLine {
						if csRow != nil && *csRow < line {
							indent = max(indent-1, 0)
						}
					}
				}
			} else {
				if cIsLastLine && csRow != nil && *csRow != osRow && *csRow < line {
					indent = max(indent-1, 0)
				} else {
					var inc int
					incStr, ok := metadata["indent.increment"]
					if !ok || incStr == nil {
						inc = 1
					} else {
						var err error
						inc, err = strconv.Atoi(*incStr)
						if err != nil {
							inc = 1
						}
					}
					if inc < 0 {
						inc = 1
					}
					indent = int(osCol + uint(inc))
					indentIsAbsolute = true
				}
			}

			avoidLastMatchingNext := false
			if csRow != nil && *csRow != osRow && *csRow < line {
				_, avoidLastMatchingNext = metadata["indent.avoid_last_matching_next"]
			}

			if avoidLastMatchingNext {
				if indent <= t.getCurrentIndent(osRow+1)+1 {
					indent++
				}
			}
			isProcessed = true
			if indentIsAbsolute {
				return indent
			}
		}

		isProcessedByRow[startRow] = isProcessedByRow[startRow] || isProcessed
		node = node.Parent()
	}

	return indent
}

func (t *tree) findDelimiter(node *tree_sitter.Node, del string) (
	ret *tree_sitter.Node, isLastLine bool,
) {
	cursor := t.tree.Walk()
	defer cursor.Close()
	for _, child := range node.Children(cursor) {
		if child.Kind() != del {
			continue
		}
		// NOTE: this is not very accurate (i.e. blank lines below last delim)
		end := child.EndPosition()
		rows := t.buf.Rows()
		return &child, end.Row == uint(rows)
	}

	return nil, false
}

func (t *tree) getPreviousNonBlankLine(line uint) (ret uint) {
	if line == 0 {
		return 0
	}

	for ret = line - 1; ret >= 1; ret-- {
		if t.buf.Columns(int(ret)) != 0 {
			return ret
		}
	}
	return
}

func (t *tree) getFirstNodeAtLine(line uint) *tree_sitter.Node {
	root := t.tree.RootNode()
	col := t.getCurrentIndent(line)
	start := tree_sitter.Point{Row: line, Column: uint(col)}
	end := tree_sitter.Point{Row: line, Column: uint(col + 1)}
	return root.DescendantForPointRange(start, end)
}

func (t *tree) getLastNodeAtLine(line uint) *tree_sitter.Node {
	indentCols := t.getCurrentIndent(line)
	col := max(indentCols + t.buf.Columns(int(line)) -1, 0)
	root := t.tree.RootNode()
	start := tree_sitter.Point{Row: line, Column: uint(col)}
	end := tree_sitter.Point{Row: line, Column: uint(col + 1)}
	return root.DescendantForPointRange(start, end)
}

func (t *tree) getCurrentIndent(line uint) (ret int) {
	cells := t.buf.RawCells()
	if line >= uint(len(cells)) {
		return 0
	}
	for _, cell := range cells[line] {
		if cell.Ch == '\t' {
			ret++
			continue
		}
		if cell.Ch != 0 {
			break
		}
	}
	return
}
