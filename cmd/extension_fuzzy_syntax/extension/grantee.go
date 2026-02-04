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
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cmd/extension_fuzzy_file/finder"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/rpc"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/workspace/walkdir"
)

var (
	cmdSearchSyntax = textapi.CommandManual{
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
)

// Grantee returns this extension's Grantee and the permissions required to run it.
func Grantee() (extension.Grantee, []extensionapi.Permission) {
	perms := finder.Permissions()
	perms = append(perms, extensionapi.PermissionConfig)

	searchSyntax, mergedPerms := extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationBottom,
		Handler:          newHandler,
		Permissions:      perms,
		Command:          cmdSearchSyntax,
	})

	return searchSyntax, mergedPerms
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

func parseLine(exec workspaceapi.FileSystem, data string) (
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

func newHandler(
	ctx context.Context, cmd textapi.Command,
	grants []extension.Grant, broker rpc.MuxBroker,
	invokeWindow browserapi.Window, c config.Config,
) (extutil.RedispatchHandler, error) {
	if len(cmd.Args) < 2 {
		return nil, errors.New("expected at least three arguments")
	}
	queryFile, captureNameString := cmd.Args[0], cmd.Args[1]
	captureNames := strings.Split(captureNameString, "|")
	noHistoryKey := term.KeyComb{}

	dataDir, err := c.GetString("datadir")
	if err != nil {
		return nil, fmt.Errorf("could not get data directory: %v", err)
	}
	return finder.New(ctx, grants, broker, invokeWindow,
		c, noHistoryKey, "unused", "",
		readSymbolsFunction(dataDir, queryFile, captureNames), parseLine)
}
