// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package starlarktutorial

import (
	"sync"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/ide/idetutorial"
)

// requestKind selects which builtin enqueued a request and which
// per-kind dispatch path runs in the TUI loop.
type requestKind int

const (
	// Blocking UI: live in Tutorial.active and gate Draw/Handle.
	reqFloatingWindow requestKind = iota + 1
	reqMarkdown
	reqWaitKey
	reqWaitCommand
	reqWaitShell
	reqWaitEvent
	reqConfirm
	reqChoice
)

// String renders the requestKind for diagnostics and test assertions.
func (k requestKind) String() string {
	switch k {
	case reqFloatingWindow:
		return "floating_window"
	case reqMarkdown:
		return "markdown"
	case reqWaitKey:
		return "wait_key"
	case reqWaitCommand:
		return "wait_command"
	case reqWaitShell:
		return "wait_shell"
	case reqWaitEvent:
		return "wait_event"
	case reqConfirm:
		return "confirm"
	case reqChoice:
		return "choice"
	}
	return "unknown"
}

// request is the message a blocking Starlark builtin posts to the
// TUI loop. The TUI loop drains active, performs per-kind Draw and
// Handle work, and delivers exactly one response by reading from
// respond. once guards a single delivery so adapters that fire
// OnSelect followed by OnClose never panic on a closed channel and
// the second callback becomes a no-op.
type request struct {
	kind requestKind

	// floating_window / markdown.
	text, title string
	align       component.Alignment
	offset      term.Coordinates
	md          *markdown.Component
	body        *component.Span

	// stepNum is the 1-based "visible content" step number snapshot
	// at publish time. Only reqFloatingWindow and reqMarkdown bump
	// the counter; other request kinds inherit the prior value but
	// only floating_window renders it in the title bar.
	stepNum int

	// allowKeys is the set of key combinations that the floating
	// window does NOT swallow: matching events fall through to the
	// IDE root so the user can e.g. press <meta-1>..<meta-9> to
	// switch workspace slots while reading instructions that mention
	// those very bindings. Empty for every other kind.
	allowKeys []term.KeyComb

	// dismissKeys behaves like allowKeys but ALSO resolves the
	// floating window: matching events dismiss the request and
	// reach the IDE root. Authors use this for read-then-act keys
	// such as the command-prompt key when the next step expects
	// the prompt to be open.
	dismissKeys []term.KeyComb

	// wait_key.
	waitKey string

	// wait_command.
	command string
	onError string

	// wait_event: the awaited editor event-type name (e.g. "open").
	event string

	// wait_shell: the expected companion-shell argument tokens (e.g.
	// ["pkg", "install", "rune-agent"]). Always non-empty — wait_shell
	// is exclusively for commands run inside the shell.
	shellArgs []string

	// choice / confirm.
	message string
	options []string
	// prompt / promptVirtual back the overlay-rendered confirm/choice
	// UI. Built by publishRequest for reqConfirm/reqChoice and nil
	// for every other kind. Tutorial.Draw positions promptVirtual
	// inside the overlay frame; Tutorial.Handle forwards events to
	// prompt directly.
	prompt        *handler.Prompt
	promptVirtual *handler.Virtual[*handler.Prompt]
	// pendingResp / pendingSelected capture the prompt's OnSelect
	// outcome so Tutorial.Handle can resolve with the correct
	// response after the barrier is in place. pendingSelected stays
	// false on a pure dismissal (Esc) and the per-kind dismissal
	// response is synthesised at resolve time.
	pendingResp     response
	pendingSelected bool

	// shader spec staged for the floating_window hint pulse.
	shaderSpec idetutorial.Shader
	hasShader  bool

	respond chan response
	once    sync.Once
}

// deliver sends res to respond exactly once. Subsequent calls are
// no-ops so a prompt that fires both OnSelect and OnClose on a normal
// selection cannot deadlock the runLoop nor panic on a closed channel.
func (r *request) deliver(res response) {
	r.once.Do(func() {
		r.respond <- res
	})
}

// response carries the user-visible result of a blocking builtin
// back to the runLoop. Unused fields are zero values; each builtin
// reads only the fields it needs.
type response struct {
	// wait_command.
	cmdName string
	cmdArgs []string

	// choice.
	selectedIdx   int
	selectedValue string
	selected      bool

	// confirm.
	confirmed bool
}
