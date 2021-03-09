package main

import (
	plugutil "github.com/ernestrc/go-tui/plugin/util"
)

func main() {
	/* go func() {
		log.Println(http.ListenAndServe("localhost:6063", nil))
	}()*/

	plugutil.ServeEditorEventHandler(gitHandlerCommands, newGitHandler,
		gitHandlerEvents, gitHandlerPermissions...)
}
