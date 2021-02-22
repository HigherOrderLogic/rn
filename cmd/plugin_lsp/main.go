package main

import (
	"os"

	"github.com/ernestrc/go-tui/plugin"
	plugutil "github.com/ernestrc/go-tui/plugin/util"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetLevel(log.TraceLevel)
	plugin.SetLoggingLevel(log.TraceLevel)

	/* go func() {
		log.Println(http.ListenAndServe("localhost:6063", nil))
	}()*/

	plugutil.ServeEditorEventHandler(lspHandlerCommands, newLspHandler,
		lspHandlerPermissions...)
}
