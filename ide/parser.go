// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package ide

import (
	"errors"

	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"unstable.build/go-tui/ide/syntax"
)

// uses lazyly initialized syntaxapi.Parser to circumvent
// cosmetic circular dependency between pkgmanager and syntax.NewParser
type lazyParser struct {
	root *workspaceManagerHandler
	p    syntaxapi.Parser
}

var _ syntaxapi.Parser = (*lazyParser)(nil)

func (w *lazyParser) parser() syntaxapi.Parser {
	if w.p == nil {
		w.p = syntax.NewParser(w.root.homeWorkspace, w.root.pkgmanager, w.root.homeURI)
	}
	return w.p
}

func (w *lazyParser) Search(query string, captureNames []string, languages ...string) (
	iterator.Iterator[syntaxapi.Result], error,
) {
	return nil, errors.New("parser not supported on home workspace")
}

// SearchNode is implemented with Search by using an internally provided
// query that is able to capture a known set of tree nodes across programming
// languages. Multiple node types can be combined using bitwise OR.
func (w *lazyParser) SearchNode(nodeTypes syntaxapi.NodeCaptureName) (
	iterator.Iterator[syntaxapi.Result], error,
) {
	return nil, errors.New("parser not supported on home workspace")
}

// Query searches for matches in a specific file using the given tree-sitter
// literal query and a list of capture names that should be returned.
func (w *lazyParser) Query(file workspaceapi.URI, query string, captureNames []string) (
	iterator.Iterator[syntaxapi.Result], error,
) {
	return nil, errors.New("parser not supported on home workspace")
}

// QueryNode is implemented with Query by using an internally provided
// query that is able to capture a known set of tree nodes across
// programming languages. Multiple node types can be combined using bitwise OR.
func (w *lazyParser) QueryNode(
	file workspaceapi.URI, nodeTypes syntaxapi.NodeCaptureName,
) (iterator.Iterator[syntaxapi.Result], error) {
	return nil, errors.New("parser not supported on home workspace")
}

// Highlight returns syntax highlighting locations for the given content,
// interpreted as belonging to the file identified by uri.
func (w *lazyParser) Highlight(uri workspaceapi.URI, content string) (
	iterator.Iterator[textapi.Location], error,
) {
	return w.parser().Highlight(uri, content)
}
