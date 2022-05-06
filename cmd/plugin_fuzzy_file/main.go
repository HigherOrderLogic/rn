package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cmd/plugin_fuzzy_file/finder"
	"github.com/ernestrc/go-tui/plugin"
	plugutil "github.com/ernestrc/go-tui/plugin/util"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/workspace"
	log "github.com/sirupsen/logrus"
)

var (
	defaultCommand           = `set -o pipefail; command find -L . -mindepth 1 \( -path '*/\.*' -o -fstype 'sysfs' -o -fstype 'devfs' -o -fstype 'devtmpfs' -o -fstype 'proc' \) -prune -o -type f -print -o -type l -print 2> /dev/null | cut -b3-`
	defaultHistoryKey        = term.KeyComb{Key: term.KeyCtrlP}
	defaultHistoryDocumentID = "plugin-fuzzy-file-history"
)

func newHandler(grants []plugin.Grant, broker proto.MuxBroker,
	invokeWindow browser.Window, config plugin.Config) (tui.Handler, error) {
	cmdStr, err := config.GetString("command")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Printf("failed to load 'command' config: %v", err)
		}
		cmdStr = defaultCommand
	}
	historyKey, err := plugin.GetKey(config, "history_key")
	if err != nil {
		if err != plugin.ErrNotFound {
			log.Printf("failed to load 'command' config: %v", err)
		}
		historyKey = defaultHistoryKey
	}
	return finder.New(grants, broker, invokeWindow, config,
		historyKey, defaultHistoryDocumentID, cmdStr, func(workspace workspace.Workspace, file string) (
			workspace.URI, term.Coordinates,
		) {
			uri, _ := workspace.URI(file)
			return uri, term.Coordinates{}
		})
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6061", nil))
	}()

	plugutil.ServeCommandSplitHandler(plugutil.CommandSplitHandlerConfig{
		SplitOrientation: browser.OrientationBottom,
		Handler:          newHandler,
		Permissions:      finder.Permissions(),
		Command:          "searchFile",
	})
}
