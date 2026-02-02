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

package vte

import (
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/term"
)

// DefaultConfig returns a sane default Config.
func DefaultConfig() Config {
	return Config{
		Clipboard:                clipboard.NewInMemory(),
		ClipboardRegister:        clipboard.DefaultRegisterID,
		ScheduleNextTick:         func(cb func()) bool { cb(); return true },
		RingBell:                 func() {},
		SelectionAttributes:      term.Attributes{Attrs: tcell.AttrReverse},
		NeedsAttentionAttributes: term.Attributes{Attrs: tcell.AttrBlink},
		DynamicTabName:           false,
		MaxLines:                 10_000,
		MinWidth:                 0,
	}
}

// Config configures Handler.
type Config struct {
	// CommandAndArgs is the program to run. Otherwise whatever is set
	// on the $SHELL environment variable is used.
	CommandAndArgs    []string
	Clipboard         clipboard.Register
	ClipboardRegister string

	// ScheduleNextTick schedules an arbitrary function to be run
	// in the next event-loop tick.
	ScheduleNextTick func(func()) bool

	// Use the VTE's title as the tab name.
	DynamicTabName bool

	// RingBell writes to the raw pty directly, bypassing the event loop.
	// This should only be used when called from the an event loop goroutine.
	RingBell func()
	Watcher  workspaceapi.ProcessWatcher

	Attributes               term.Attributes
	SelectionAttributes      term.Attributes
	NeedsAttentionAttributes term.Attributes
	MaxLines                 int
	// Bell overrides the default bell trigger. This is useful for non-standard
	// shells like the fish shell, which don't trigger the bell with the standard
	// escape sequence.
	Bell []byte

	// MinWidth helps optimize growing and shrinking rows upon resize.
	MinWidth int

	// Modal enables entering modal mode via Esc key.
	// Changing mode to 'INSERT' mode switches back to
	// the shell being in control of the input.
	Modal bool

	// WidthHint and HeightHint hint allows emulator.Handler to better configure the
	// initial buffer size.
	WidthHint  int
	HeightHint int

	Debug      bool
	BashrcFile string
	ZdotDir    string
}

func (c Config) scheduleBell() {
	c.ScheduleNextTick(c.RingBell)
}
