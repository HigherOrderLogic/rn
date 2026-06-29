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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// agentOnboardingSrc mirrors the install → provider → credentials
// shape of teach_agent() in cmd/rune/tutorials/basics.star so the DSL
// mechanics the real tutorial relies on (the wait_shell builtin's
// open-shell gate, its token-containment re-arm, and choice branching)
// are exercised here without linking the cmd/rune native libraries.
const agentOnboardingSrc = `
def run():
    floating_window(text="open the console")
    wait_command(command="console")
    floating_window(text="install the agent")
    wait_shell(args=["pkg", "install", "rune-agent"],
               on_error="run pkg install rune-agent")
    notify(level=success, message="Rune Agent installed.")

    pick = choice(message="provider?",
                  options=["OpenAI", "Anthropic", "Gemini", "Codex", "Claude", "Skip"])
    if not pick.selected or pick.value == "Skip":
        notify(level=info, message="skipped provider setup")
        return

    floating_window(text="connect " + pick.value)
    wait_shell(args=["models", "providers", "openai", "add"],
               on_error="run models providers openai add default")
    notify(level=success, message="OpenAI connected.")
    floating_window(text="open the agent")
    wait_command(command="agent", on_error="run agent")
    notify(level=success, message="Rune Agent is ready.")

tutorial(entry=run)
`

// TestAgentOnboardingInstallGateReArmsOnWrongCommand drives the agent
// onboarding flow: the install gate must ignore an unrelated shell
// submission, advance on the correct "pkg install rune-agent"
// submission, branch on the provider choice, and complete the
// credentials gate.
func TestAgentOnboardingInstallGateReArmsOnWrongCommand(t *testing.T) {
	t.Parallel()
	tut, notis := newTutorial(t, agentOnboardingSrc)
	resetAndWait(t, tut, time.Second)

	require.Equal(t, "floating_window", activeKindFor(tut))
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_command", time.Second)

	// Open the companion shell, then advance to the install window.
	tut.ObserveCommand("console", "console", nil, nil)
	waitNextActive(t, tut, "floating_window", time.Second)
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_shell", time.Second)

	// A shell submission that is not the install command keeps the
	// gate armed without resolving it.
	exit := tut.ObserveCommand("console", "console", []string{"ls"}, nil)
	assert.False(t, exit)
	assert.Equal(t, "wait_shell", activeKindFor(tut),
		"a non-install shell command must keep the install gate armed")

	// The correct install command advances to the provider choice.
	tut.ObserveCommand("console", "console",
		[]string{"pkg", "install", "rune-agent"}, nil)
	waitNextActive(t, tut, "choice", time.Second)
	assert.True(t, notis.containsSubstring("Rune Agent installed."))

	// Pick OpenAI (index 0, already highlighted).
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "floating_window", time.Second)
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_shell", time.Second)

	tut.ObserveCommand("console", "console",
		[]string{"models", "providers", "openai", "add", "default"}, nil)
	waitNextActive(t, tut, "floating_window", time.Second)
	assert.True(t, notis.containsSubstring("OpenAI connected."))

	// The final step waits for the user to open the agent.
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_command", time.Second)
	tut.ObserveCommand("agent", "agent", nil, nil)
	waitFinished(t, tut, time.Second)
	assert.True(t, notis.containsSubstring("Rune Agent is ready."))
}

// TestAgentOnboardingSkipProviderExits asserts that choosing Skip at
// the provider prompt short-circuits credential setup with an info
// notification.
func TestAgentOnboardingSkipProviderExits(t *testing.T) {
	t.Parallel()
	tut, notis := newTutorial(t, agentOnboardingSrc)
	resetAndWait(t, tut, time.Second)

	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_command", time.Second)
	tut.ObserveCommand("console", "console", nil, nil)
	waitNextActive(t, tut, "floating_window", time.Second)
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_shell", time.Second)
	tut.ObserveCommand("console", "console",
		[]string{"pkg", "install", "rune-agent"}, nil)
	waitNextActive(t, tut, "choice", time.Second)

	// Skip is the last option (index 5).
	for range 5 {
		_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyArrowRight})
	}
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitFinished(t, tut, time.Second)
	assert.True(t, notis.containsSubstring("skipped provider setup"),
		"Skip must short-circuit credential setup, got %v",
		notis.renderedCalls())
}

// TestAgentOnboardingDismissedProviderExits asserts that dismissing the
// provider prompt (Esc, selected=False) also short-circuits setup.
func TestAgentOnboardingDismissedProviderExits(t *testing.T) {
	t.Parallel()
	tut, notis := newTutorial(t, agentOnboardingSrc)
	resetAndWait(t, tut, time.Second)

	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_command", time.Second)
	tut.ObserveCommand("console", "console", nil, nil)
	waitNextActive(t, tut, "floating_window", time.Second)
	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	waitNextActive(t, tut, "wait_shell", time.Second)
	tut.ObserveCommand("console", "console",
		[]string{"pkg", "install", "rune-agent"}, nil)
	waitNextActive(t, tut, "choice", time.Second)

	_, _ = tut.Handle(term.Event{Type: term.EventKey, Key: term.KeyEsc})
	waitFinished(t, tut, time.Second)
	assert.True(t, notis.containsSubstring("skipped provider setup"),
		"dismissing the provider prompt must short-circuit setup, got %v",
		notis.renderedCalls())
}

// TestWaitShellRequiresArgs asserts that wait_shell is exclusively for
// commands run inside the console: calling it without args is a runtime
// error (opening the console is wait_command(command="console")).
func TestWaitShellRequiresArgs(t *testing.T) {
	t.Parallel()
	src := `
def run():
    wait_shell()
    notify(message="should not reach here")
tutorial(entry=run)
`
	tut, notis := newTutorial(t, src)
	resetAndWait(t, tut, time.Second)
	waitFinished(t, tut, time.Second)
	hasError := false
	for _, c := range notis.captured {
		if c.level == browserapi.LevelError {
			hasError = true
		}
	}
	assert.True(t, hasError,
		"wait_shell without args must surface as a runtime error, got %v",
		notis.renderedCalls())
	assert.False(t, notis.containsSubstring("should not reach here"))
}

// TestWaitShellIgnoresNonShellObservations asserts that a wait_shell
// step does not resolve on a regular (non-shell) command dispatch.
func TestWaitShellIgnoresNonShellObservations(t *testing.T) {
	t.Parallel()
	src := `
def run():
    wait_shell(args=["pkg", "install", "rune-agent"])
    notify(message="installed")
tutorial(entry=run)
`
	tut, notis := newTutorial(t, src)
	resetAndWait(t, tut, time.Second)

	exit := tut.ObserveCommand("edit", "edit", []string{"main.go"}, nil)
	assert.False(t, exit)
	assert.Equal(t, "wait_shell", activeKindFor(tut),
		"a non-shell command must not resolve a wait_shell step")
	assert.Equal(t, 0, notis.len())

	tut.ObserveCommand("console", "console",
		[]string{"pkg", "install", "rune-agent"}, nil)
	waitFinished(t, tut, time.Second)
	assert.True(t, notis.containsSubstring("installed"))
}

// TestWaitShellTokenContainmentReArms asserts that a shell submission
// missing an expected token keeps the step armed, and that a later
// submission containing every token (plus extras) resolves it. The
// resolved command_result carries the full observed args.
func TestWaitShellTokenContainmentReArms(t *testing.T) {
	t.Parallel()
	src := `
def run():
    r = wait_shell(args=["pkg", "install", "rune-agent"])
    notify(message="installed " + r.args[2])
tutorial(entry=run)
`
	tut, notis := newTutorial(t, src)
	resetAndWait(t, tut, time.Second)

	// Missing "rune-agent" keeps the step armed.
	exit := tut.ObserveCommand("console", "console", []string{"pkg", "install"}, nil)
	assert.False(t, exit)
	assert.Equal(t, "wait_shell", activeKindFor(tut))

	// Containment with extra tokens (e.g. a flag) still matches.
	tut.ObserveCommand("console", "console",
		[]string{"pkg", "install", "rune-agent", "--force"}, nil)
	waitFinished(t, tut, time.Second)
	assert.True(t, notis.containsSubstring("installed rune-agent"),
		"command_result.args must carry the full observed args, got %v",
		notis.renderedCalls())
}

// TestWaitShellErrorStaysArmedAndSwapsHint asserts that a dispatch
// error keeps the step armed and swaps the on_error hint into the
// rendered window.
func TestWaitShellErrorStaysArmedAndSwapsHint(t *testing.T) {
	t.Parallel()
	src := `
def run():
    wait_shell(args=["pkg", "install", "rune-agent"],
               on_error="install failed, try again")
    notify(message="installed")
tutorial(entry=run)
`
	tut, _ := newTutorial(t, src)
	tut.Resize(80, 60)
	resetAndWait(t, tut, time.Second)

	exit := tut.ObserveCommand("console", "console",
		[]string{"pkg", "install", "rune-agent"}, assert.AnError)
	assert.False(t, exit)
	assert.Equal(t, "wait_shell", activeKindFor(tut),
		"a dispatch error must keep the wait_shell step armed")

	g := newGridWriter(80, 60)
	tut.Draw(g)
	assert.True(t, gridContains(g, "install failed"),
		"on_error hint must be rendered after a dispatch error")
	tut.Stop()
}
