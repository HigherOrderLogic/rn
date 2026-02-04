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
	"errors"
	"fmt"
	"strings"

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Handler is a subset of text.Handler.
type Handler interface {
	SetCursorAtScroll(term.Coordinates) bool
}

// Commands returns the file-level commands supported by the given tree.
func Commands(handler Handler, t *Tree) ([]textapi.CommandManual, CommandHandler) {
	return commands, CommandHandler{handler: handler, t: t}
}

const (
	cmdJumpToSyntax = "jumptoast"
)

var commands = []textapi.CommandManual{
	{
		Name: cmdJumpToSyntax,
		Summary: "Run the given query against the file currently in focus and jump to a " +
			"matching AST node. The first argument is the name or path of the query file to run. " +
			"The second argument is the match capture name(s), and the last argument " +
			"is the name of the node in the file (i.e. function name, variable name, etc.). " +
			"The second argument can be ORed by adding a `|` character between match names." +
			"For example to match against functions and methods you can pass: " +
			"locals.scm local.definition.method|local.definition.function MyFuncName. " +
			"The query file should be a relative or absolute path and if not found, " +
			"it will be searched in the file's language package installation " +
			"folder.",
		Synopsis: "query capture1[...|captureN] name",
	},
}

// CommandHandler satisfies text.CommandHandler.
type CommandHandler struct {
	handler Handler
	t       *Tree
}

// HandleCommand satisfies text.CommandHandler.
func (c CommandHandler) HandleCommand(ctx context.Context, cmd textapi.Command) (err error) {
	switch cmd.Name {
	case cmdJumpToSyntax:
		return c.handleJumpToSyntax(ctx, cmd)
	default:
		return errors.New("unknown command")
	}
}

// Complete satisfies text.CommandHandler.
func (c CommandHandler) Complete(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	switch cmd.Name {
	case cmdJumpToSyntax:
		return c.completeJumpToSyntax(ctx, cmd)
	default:
		return nil, "", errors.New("unkown command")
	}
}

func (c CommandHandler) completeJumpToSyntax(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	// complete with pre-loaded files
	if len(cmd.Args) <= 1 {
		return iterator.FromSlice([]string{"locals.scm"}), "", nil
	}

	// complete query capture names
	set := make(map[string]struct{})
	if len(cmd.Args) == 2 {
		it, err := c.t.Query(cmd.Args[0])
		if err != nil {
			return nil, "", err
		}
		matches, err := iterator.ToSlice(ctx, it)
		if err != nil {
			return nil, "", err
		}
		for _, match := range matches {
			set[match.CaptureName] = struct{}{}
		}
		sliceSet := make([]string, 0, len(set))
		for captureName := range set {
			sliceSet = append(sliceSet, captureName)
		}
		return iterator.FromSlice(sliceSet), "", nil
	}

	if len(cmd.Args) == 3 {
		queryFile, captureNameString := cmd.Args[0], cmd.Args[1]
		captureNames := strings.Split(captureNameString, "|")

		matches, err := c.t.Query(queryFile, captureNames...)
		if err != nil {
			return nil, "", err
		}
		return iterator.Map(matches, func(s Match) string {
			return s.LineString
		}), "", nil
	}
	return iterator.Empty[string](), "", nil
}

func (c CommandHandler) handleJumpToSyntax(
	ctx context.Context, cmd textapi.Command,
) (err error) {
	if len(cmd.Args) < 3 {
		return errors.New("expected at least three arguments")
	}
	queryFile, captureNameString, lineStringChunks := cmd.Args[0], cmd.Args[1], cmd.Args[2:]
	captureNames := strings.Split(captureNameString, "|")
	lineString := strings.Join(lineStringChunks, " ")

	matches, err := c.t.Query(queryFile, captureNames...)
	if err != nil {
		return err
	}
	defer matches.Close()
	for {
		match, ok := matches.Next(ctx)
		if !ok {
			err = fmt.Errorf("could not find matching node")
			break
		}
		if match.LineString == lineString {
			c.handler.SetCursorAtScroll(term.Coordinates{Y: match.Line})
			break
		}
	}
	if merr := matches.Err(); merr != nil {
		err = multierror.Append(err, fmt.Errorf("symbols iterator: %w", merr))
	}
	return err
}
