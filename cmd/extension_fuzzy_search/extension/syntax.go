// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.

package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cmd/extension_fuzzy_search/finder"
	"unstable.build/go-tui/workspace/walkdir"
)

var cmdSearchSyntax = textapi.CommandManual{
	Name: "searchast",
	Summary: "Fuzzy search AST nodes by running the given query against all the files in " +
		"the workspace. The first argument is the name or path of the query file to run. " +
		"The second argument is the match capture name(s), and the last argument " +
		"is the name of the node in the file (i.e. function name, variable name, etc.). " +
		"The second argument can be ORed by adding a `|` character between match names." +
		"For example to match against functions and methods you can pass: " +
		"locals.scm local.definition.method|local.definition.function. " +
		"The query file should be a relative or absolute path and if not found, " +
		"it will be searched in the file's language package installation " +
		"folder.",
	Synopsis: "query capture1[...|captureN]",
}

func readSymbolsFunction(dataDir string, queryFile string, captureNames []string) func(
	workspaceapi.FileSystem, context.Context) (iterator.Iterator[string], error,
) {
	return func(cwd workspaceapi.FileSystem, ctx context.Context) (
		iterator.Iterator[string], error,
	) {
		var q string
		switch queryFile {
		case "folds.scm", "indents.scm",
			"highlights.scm", "locals.scm":
			// empty query instructs readSymbols to find .scm file in lang lib
		default:
			data, err := os.ReadFile(queryFile)
			if err != nil {
				return nil, fmt.Errorf("read query file: %w", err)
			}
			q = string(data)
		}
		it, err := walkdir.ListFiles(ctx, cwd, ".")
		if err != nil {
			return nil, err
		}
		uri, err := cwd.URI(".")
		if err != nil {
			return nil, err
		}
		sit, err := readSymbols(ctx, cwd, dataDir, uri, it, queryFile, q)
		if err != nil {
			return nil, err
		}
		if len(captureNames) != 0 {
			sit = iterator.Filter(sit, func(m match) bool {
				return slices.Contains(captureNames, m.CaptureName)
			})
		}
		return iterator.Map(sit, func(m match) string { return m.LineString }), nil
	}
}

func syntaxResource(exec workspaceapi.FileSystem, data string) (
	uri workspaceapi.URI, coords term.Coordinates, ok bool,
) {
	chunks := strings.Split(data, ":")
	if len(chunks) < 2 {
		return workspaceapi.URI{}, term.Coordinates{}, false
	}
	y, _ := strconv.Atoi(chunks[1])
	name := chunks[0]
	uri, err := exec.URI(name)
	return uri, term.Coordinates{Y: y - 1}, err == nil
}

func newSyntaxHandler(
	ctx context.Context, cmd textapi.Command,
	clients finder.Clients, invokeWindow browserapi.Window, c config.Config, dataDir string,
) (finder.RedispatchHandler, error) {
	if len(cmd.Args) < 2 {
		return nil, errors.New("expected at least three arguments")
	}
	queryFile, captureNameString := cmd.Args[0], cmd.Args[1]
	captureNames := strings.Split(captureNameString, "|")
	noHistoryKey := term.KeyComb{}

	return finder.NewV2(ctx, clients, invokeWindow,
		c, noHistoryKey, "unused", "",
		readSymbolsFunction(dataDir, queryFile, captureNames), syntaxResource)
}
