package vte

import (
	"github.com/ernestrc/tcell/v3"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text/clipboard"
)

// DefaultConfig returns a sane default Config.
func DefaultConfig() Config {
	return Config{
		Clipboard:                clipboard.NewInMemory(),
		ClipboardRegister:        clipboard.DefaultRegisterID,
		ScheduleBell:             func() {},
		RingBell:                 func() {},
		SelectionAttributes:      term.Attributes{Attrs: tcell.AttrReverse},
		NeedsAttentionAttributes: term.Attributes{Attrs: tcell.AttrBlink},
	}
}

// Config configures Handler.
type Config struct {
	// Shell is the default shell to use. Otherwise whatever is set
	// on the $SHELL environment variable is used.
	Shell             string
	Clipboard         clipboard.Register
	ClipboardRegister string
	// ScheduleBell schedules a bell to be run in the next event-loop tick.
	ScheduleBell func()
	// RingBell writes to the raw pty directly, bypassing the event loop.
	// This should only be used when called from the an event loop goroutine.
	RingBell func()
	Watcher  workspaceapi.Watcher

	Attributes               term.Attributes
	SelectionAttributes      term.Attributes
	NeedsAttentionAttributes term.Attributes

	// Modal enables entering modal mode via Esc key.
	// Changing mode to 'INSERT' mode switches back to
	// the shell being in control of the input.
	Modal bool

	// WidthHint and HeightHint hint allows emulator.Handler to better configure the
	// initial buffer size.
	WidthHint  int
	HeightHint int
}
