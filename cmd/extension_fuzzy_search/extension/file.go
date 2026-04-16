// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.

package extension

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	tconfig "unstable.build/go-tui/api/config"
	"unstable.build/go-tui/cmd/extension_fuzzy_search/finder"
	"unstable.build/go-tui/workspace/walkdir"
)

const fileHistoryDocumentID = "extension-fuzzy-file-history"

var (
	fileDefaultHistoryKey = term.KeyComb{Ch: 'p', Mod: term.ModCtrl}
	cmdSearchFile         = textapi.CommandManual{
		Name: "searchfile",
		Summary: "Opens a new window to perform a fuzzy search for files in the workspace. " +
			"Results are sorted by match score in descending order. " +
			"Arrow keys and <ctrl-k>/<ctrl-j> scroll up and down and <enter> opens up the selected file in a new tab. " +
			"By default, a built-in implementation is used to scan for files in the workspace, but " +
			"for very large workspaces, a program like ripgrep or the silver searcher " +
			"can be used by adding the corresponding 'command' key in the extension's " +
			"configuration or passing an argument (i.e. searchFile rg -l \"\"). To scroll back to previous searches, " +
			"<ctrl-p> can be used by default or a 'history_key' can be set in the extension's " +
			"configuration. Ctrl-c can be used to cancel a scan in progress.",
		Synopsis: "[command]",
	}
)

func workspaceListFiles(cwd workspaceapi.FileSystem, ctx context.Context) (
	iterator.Iterator[string], error,
) {
	return walkdir.ListFiles(ctx, cwd, ".")
}

func fileResource(workspace workspaceapi.FileSystem, file string) (
	workspaceapi.URI, term.Coordinates, bool,
) {
	uri, err := workspace.URI(file)
	return uri, term.Coordinates{}, err == nil
}

func newFileHandler(
	ctx context.Context, cmd textapi.Command,
	clients finder.Clients, invokeWindow browserapi.Window, c config.Config,
) (finder.RedispatchHandler, error) {
	cmdStr, err := c.GetString("command")
	if err != nil {
		if err != config.ErrNotFound {
			log.Printf("failed to load 'command' config: %v", err)
		}
	}
	historyKey, err := tconfig.GetKey(c, "history_key")
	if err != nil {
		if err != config.ErrNotFound {
			log.Printf("failed to load 'history_key' config: %v", err)
		}
		historyKey = fileDefaultHistoryKey
	}
	if len(cmd.Args) != 0 {
		cmdStr = strings.Join(cmd.Args, " ")
	}
	return finder.NewV2(context.Background(), clients, invokeWindow, c,
		historyKey, fileHistoryDocumentID, cmdStr,
		workspaceListFiles, fileResource)
}
