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
	"context"

	log "github.com/sirupsen/logrus"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"github.com/unstablebuild/blue/iterator"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

const captureNameFoldsInitial = "initial_fold"

func (t *Tree) getFolds(initial bool) []term.Range {
	ret := make([]term.Range, 0)
	root := t.tree.RootNode()

	cur := tree_sitter.NewQueryCursor()
	defer cur.Close()

	captureNames := t.folds.CaptureNames()
	matches := cur.Matches(t.folds, root, []byte(t.buf.String()))
	for {
		m, ok := matches.Next()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			n := cap.Node
			if initial && captureNames[cap.Index] != captureNameFoldsInitial {
				continue
			}
			rng, ok := t.treeSitterRangeToTerm(n.Range())
			if !ok {
				t.log(log.DebugLevel, "could not convert tree sitter range %+v to term",
					n.Range())
				continue
			}
			ret = append(ret, rng)
		}
	}
	return ret
}

func (t *Tree) getFoldsFrom(from term.Coordinates) []term.Range {
	ret := make([]term.Range, 0)

	from.X = 0 // ignore x offsets to make things easier
	byteOffset, sok := cell.ConvertCoordinatesToByteOffset(t.cells, from)
	if !sok {
		return nil
	}
	cur := tree_sitter.NewQueryCursor()
	matches := cur.Matches(t.folds, t.tree.RootNode(), t.content)
	cur.SetByteRange(uint(byteOffset), uint(len(t.content)))
	for {
		m, ok := matches.Next()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			n := cap.Node
			rng, ok := t.treeSitterRangeToTerm(n.Range())
			if !ok {
				t.log(log.DebugLevel, "could not convert tree sitter range %+v to term",
					n.Range())
				continue
			}
			ret = append(ret, rng)
		}
	}
	return ret
}

func (t *Tree) treeSitterRangeToTerm(rng tree_sitter.Range) (term.Range, bool) {
	start, sok := cell.ConvertRunePosToCoordinates(t.buf.RawCells(),
		int(rng.StartPoint.Row), int(rng.StartPoint.Column))
	end, eok := cell.ConvertRunePosToCoordinates(t.buf.RawCells(),
		int(rng.EndPoint.Row), int(rng.EndPoint.Column))
	return term.Range{
		Start: start,
		End:   end,
	}, sok && eok
}

type foldsIterator struct {
	initial bool
	ready   chan struct{}
	tree    *Tree
	from    term.Coordinates
	slice   iterator.Iterator[term.Range]
}

func newFoldsIterator(initial bool, t *Tree, ch chan struct{}) *foldsIterator {
	return &foldsIterator{
		tree:    t,
		ready:   ch,
		initial: initial,
	}
}

func newFoldsFromIterator(from term.Coordinates, t *Tree, ch chan struct{}) *foldsIterator {
	return &foldsIterator{
		tree:  t,
		ready: ch,
		from:  from,
	}
}

func (f *foldsIterator) Next(ctx context.Context) (term.Range, bool) {
	<-f.ready
	if f.slice == nil {
		if f.tree.tree == nil || f.tree.folds == nil {
			return term.Range{}, false
		}
		if f.from == (term.Coordinates{}) {
			f.slice = iterator.FromSlice(f.tree.getFolds(f.initial))
		} else {
			f.slice = iterator.FromSlice(f.tree.getFoldsFrom(f.from))
		}
	}
	return f.slice.Next(ctx)
}

func (f *foldsIterator) Err() error {
	<-f.ready
	if f.slice == nil {
		if f.tree.tree == nil || f.tree.folds == nil {
			return nil
		}
		if f.from == (term.Coordinates{}) {
			f.slice = iterator.FromSlice(f.tree.getFolds(f.initial))
		} else {
			f.slice = iterator.FromSlice(f.tree.getFoldsFrom(f.from))
		}
	}
	return f.slice.Err()
}

func (f *foldsIterator) Close() error {
	return nil
}
