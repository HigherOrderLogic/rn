package browser

import (
	"unstable.build/go-tui"
	"unstable.build/go-tui/term"
)

const errMsg = "this Handler is a token handler that cannot be used directly"

// Token is a token handler used to indicate which of the remote handlers
// to set as content to a remote server. It satisfies tui.Handler so that clients
// can take the result of an browser.Open type of requests and pass it to Split* or SetContent
// type of responses.
type Token struct {
	ID string
}

// Handle panics if called. This tui.Handler implementation is symbolic.
func (h Token) Handle(term.Event) (exit, handled bool) {
	panic(errMsg)
}

// Cursor panics if called. This tui.Handler implementation is symbolic.
func (h Token) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	panic(errMsg)
}

// Man panics if called. This tui.Handler implementation is symbolic.
func (h Token) Man() tui.Manual {
	panic(errMsg)
}

// Resize panics if called. This tui.Handler implementation is symbolic.
func (h Token) Resize(width, height int) {
	panic(errMsg)
}

// Draw panics if called. This tui.Handler implementation is symbolic.
func (h Token) Draw(w term.Writer) {
	panic(errMsg)
}

// Close does nothing if called.
func (h Token) Close() error {
	return nil
}
