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
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	tconfig "unstable.build/go-tui/api/config"
	"unstable.build/go-tui/cmd/extension_fuzzy_file/finder"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/workspace/walkdir"
)

const (
	// defaults now to using native workspace.ListFiles if 'command' not defined in config
	// defaultCommand           = `grep -n -r "" .`
	defaultHistoryDocumentID = "extension-fuzzy-line-history"
)

var (
	defaultHistoryKey = term.KeyComb{Ch: '\\', Mod: term.ModCtrl}
	cmdSearchText     = textapi.CommandManual{
		Name: "searchtext",
		Summary: "Opens a new window to perform a fuzzy search for file contents in the workspace. " +
			"Results are sorted by match score in descending order. " +
			"Arrow keys and <ctrl-k>/<ctrl-j> scroll up and down and <enter> opens up the selected file in a new tab. " +
			"By default, a built-in implementation is used to scan for files in the workspace, but " +
			"for very large workspaces, a program like ripgrep or the silver searcher " +
			"can be used by adding the corresponding 'command' key in the extension's " +
			"configuration or by passing an argument (i.e. searchText rg --color never -n --no-heading --max-columns 500 \"\"" +
			"). To scroll back to previous searches, " +
			`<ctrl-\> can be used by default or a 'history_key' can be set in the extension's ` +
			"configuration. Ctrl-c can be used to cancel a scan in progress.",
		Synopsis: "[command]",
	}
)

// Grantee returns this extension's Grantee and the permissions required to run it.
func Grantee() (extension.Grantee, []extensionapi.Permission) {
	return extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationBottom,
		Handler:          newHandler,
		Permissions:      finder.Permissions(),
		Command:          cmdSearchText,
	})
}

func readfiles(cwd workspaceapi.FileSystem, ctx context.Context) (
	iterator.Iterator[string], error,
) {
	it, err := walkdir.ListFiles(ctx, cwd, ".")
	if err != nil {
		return nil, err
	}

	return walkdir.ReadLines(ctx, cwd, it)
}

func parseLine(workspace workspaceapi.FileSystem, data string) (
	workspaceapi.URI, term.Coordinates, bool,
) {
	// NOTE: if ag breaks this or there's an edge case that it's not covered
	// let it panic so we catch it early and fix it
	chunks := strings.Split(data, ":")
	if len(chunks) < 2 {
		return workspaceapi.URI{}, term.Coordinates{}, false
	}
	y, _ := strconv.Atoi(chunks[1])
	name := chunks[0]
	uri, err := workspace.URI(name)
	return uri, term.Coordinates{Y: y - 1}, err == nil
}

func newHandler(
	ctx context.Context, cmd textapi.Command,
	grants []extension.Grant, broker rpc.MuxBroker,
	invokeWindow browserapi.Window, c config.Config,
) (extutil.RedispatchHandler, error) {
	cmdStr, err := c.GetString("command")
	if err != nil {
		if err != config.ErrNotFound {
			log.Printf("failed to load 'command' config: %v", err)
		}
	}
	historyKey, err := tconfig.GetKey(c, "history_key")
	if err != nil {
		if err != config.ErrNotFound {
			log.Printf("failed to load 'command' config: %v", err)
		}
		historyKey = defaultHistoryKey
	}
	if len(cmd.Args) != 0 {
		cmdStr = strings.Join(cmd.Args, " ")
	}
	return finder.New(ctx, grants, broker, invokeWindow,
		c, historyKey, defaultHistoryDocumentID, cmdStr, readfiles, parseLine)
}
