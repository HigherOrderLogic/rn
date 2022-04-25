package main

import (
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/workspace"
)

type nopWorkspace struct {
}

func (n nopWorkspace) Open(
	file workspace.URI, buf *cell.Buffer, swapDir workspace.URI, readOnly bool,
) (workspace.FlusherCloser, error) {
	panic("called Open on nop workspace")
}
func (n nopWorkspace) Recover(
	file, swapFilePath workspace.URI, buf *cell.Buffer,
) (workspace.FlusherCloser, error) {
	panic("called Recover on nop workspace")
}
func (n nopWorkspace) URI(string) (workspace.URI, error) {
	panic("called URI on nop workspace")
}
