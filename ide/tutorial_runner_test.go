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

package ide

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/ide/idetutorial"
	"unstable.build/go-tui/ide/idetutorial/starlarktutorial"
)

type rootStub struct {
	handled int
}

func (r *rootStub) Resize(_, _ int)    {}
func (r *rootStub) Draw(_ term.Writer) {}
func (r *rootStub) Handle(_ term.Event) (bool, bool) {
	r.handled++
	return false, true
}

func (r *rootStub) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, term.CursorStyleDefault, false
}
func (r *rootStub) Selection() (string, bool) { return "", false }

type tutStub struct {
	exitOn      rune
	handleCount int
	resetCount  int
	stopCount   int
}

func (t *tutStub) Resize(_, _ int)    {}
func (t *tutStub) Draw(_ term.Writer) {}
func (t *tutStub) Handle(ev term.Event) (bool, bool) {
	t.handleCount++
	return ev.Ch == t.exitOn, true
}
func (t *tutStub) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, term.CursorStyleDefault, false
}
func (t *tutStub) Selection() (string, bool) { return "", false }
func (t *tutStub) Reset()                    { t.resetCount++ }
func (t *tutStub) Stop()                     { t.stopCount++ }
func (t *tutStub) ObserveCommand(_, _ string, _ []string, _ error) bool {
	return false
}
func (t *tutStub) ObserveEvent(_, _ string) bool {
	return false
}
func (t *tutStub) Shader() (idetutorial.Shader, bool) {
	return idetutorial.Shader{}, false
}
func (t *tutStub) SetDefaultAttributes(_ term.Attributes) {}

func newTestRunner(tutorials map[string]idetutorial.Tutorial) (
	*tutorialRunner, *rootStub,
) {
	root := &rootStub{}
	r := &tutorialRunner{}
	r.init(root, tutorials, term.NopInterrupter())
	r.Resize(20, 5)
	return r, root
}

func TestTutorialRunnerStartsAndStopsOnExit(t *testing.T) {
	t.Parallel()
	tut := &tutStub{exitOn: 'q'}
	r, root := newTestRunner(map[string]idetutorial.Tutorial{"basics": tut})

	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "basics"}}))
	assert.NotNil(t, r.overlay, "expected overlay after start")
	assert.Equal(t, "basics", r.activeName)
	assert.Equal(t, 1, tut.resetCount,
		"runner should Reset the tutorial on every dispatch")

	_, handled := r.Handle(term.Event{Type: term.EventKey, Ch: 'x'})
	assert.True(t, handled)
	assert.Equal(t, 1, tut.handleCount)
	assert.Equal(t, 0, root.handled,
		"root should not see events while tutorial is active")

	_, handled = r.Handle(term.Event{Type: term.EventKey, Ch: 'q'})
	assert.True(t, handled)
	assert.Nil(t, r.overlay)
	assert.Equal(t, "", r.activeName)

	_, _ = r.Handle(term.Event{Type: term.EventKey, Ch: 'y'})
	assert.Equal(t, 1, root.handled)

	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "basics"}}))
	// Each dispatch installs a fresh overlay and calls Reset once.
	// The runner used to also call Reset during clearActive to
	// release per-step resources; that's now Stop's job, so the
	// total Reset count is one per dispatch.
	assert.Equal(t, 2, tut.resetCount)
	assert.Equal(t, 1, tut.stopCount,
		"clearActive forwards Stop through Handler.Close")
}

func TestTutorialRunnerUnknownTutorial(t *testing.T) {
	t.Parallel()
	r, _ := newTestRunner(nil)
	err := r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "nope"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown tutorial "nope"`)
}

func TestTutorialRunnerStop(t *testing.T) {
	t.Parallel()
	r, _ := newTestRunner(map[string]idetutorial.Tutorial{
		"basics": &tutStub{exitOn: 'q'},
	})

	err := r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"stop"}})
	require.Error(t, err)

	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "basics"}}))
	require.NotNil(t, r.overlay)
	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"stop"}}))
	assert.Nil(t, r.overlay)
}

func TestTutorialRunnerComplete(t *testing.T) {
	t.Parallel()
	r, _ := newTestRunner(map[string]idetutorial.Tutorial{
		"basics":   &tutStub{},
		"advanced": &tutStub{},
	})

	ctx := context.Background()
	drain := func(it iterator.Iterator[string]) []string {
		out := []string{}
		for {
			v, ok := it.Next(ctx)
			if !ok {
				break
			}
			out = append(out, v)
		}
		return out
	}

	// First arg: subcommands.
	it, _, err := r.Complete(ctx, textapi.Command{Name: "tutorial"})
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"start", "stop"},
		drain(it))

	// `tutorial start <TAB>`: tutorial names.
	it, _, err = r.Complete(ctx,
		textapi.Command{Name: "tutorial", Args: []string{"start", ""}})
	require.NoError(t, err)
	assert.Equal(t, []string{"advanced", "basics"}, drain(it))

	// `tutorial stop <TAB>`: no completions.
	it, _, err = r.Complete(ctx,
		textapi.Command{Name: "tutorial", Args: []string{"stop", ""}})
	require.NoError(t, err)
	assert.Empty(t, drain(it))

	// Too many args: no completions.
	it, _, err = r.Complete(ctx,
		textapi.Command{Name: "tutorial", Args: []string{"start", "basics", "extra"}})
	require.NoError(t, err)
	assert.Empty(t, drain(it))
}

// TestTutorialRunnerBasicsFlowEndToEnd asserts that the runner drives
// a real starlark tutorial through the full basics flow and clears the
// overlay once the final step exits.
//
// Regression: wait_command(edit) previously failed to advance after
// wait_command(wopen) succeeded.
func TestTutorialRunnerBasicsFlowEndToEnd(t *testing.T) {
	t.Parallel()
	src := `
def run():
    floating_window(title="welcome", text="open something")
    ws = wait_command(command="wopen")
    notify(level=info, message="opened " + ws.args[0])
    floating_window(title="edit", text="now edit a file")
    ed = wait_command(command="edit")
    notify(level=info, message="edited " + ed.args[0])
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"basics", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, root := newTestRunner(
		map[string]idetutorial.Tutorial{"basics": tut})

	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "basics"}}))
	require.NotNil(t, r.overlay)

	enter := term.Event{Type: term.EventKey, Key: term.KeyEnter}
	feed := func(t *testing.T, evs ...term.Event) {
		t.Helper()
		for _, ev := range evs {
			r.Handle(ev)
		}
	}

	require.True(t, tut.WaitActive("floating_window", time.Second),
		"first floating_window did not become active")
	feed(t, enter)
	require.True(t, tut.WaitActive("wait_command", time.Second),
		"wait_command(wopen) did not become active")
	// The IDE's command observer (not the tutorial's Handle) is the
	// source of truth for command dispatches: it carries the real
	// args after alias expansion. Drive the observer the way ex.go
	// would after the user dispatched `:wopen ~/proj`.
	r.observeCommand("wopen", "wopen", []string{"~/proj"}, nil)
	require.True(t, tut.WaitActive("floating_window", time.Second),
		"second floating_window did not become active")
	feed(t, enter)
	require.True(t, tut.WaitActive("wait_command", time.Second),
		"wait_command(edit) did not become active")
	r.observeCommand("edit", "edit", []string{"somefile.go"}, nil)
	require.True(t, tut.WaitFinished(time.Second),
		"tutorial did not finish after edit dispatch")

	assert.Nil(t, r.overlay,
		"runner overlay should be cleared once basics flow completes")
	_ = root
}

// TestTutorialRunnerWaitCommandArgsComeFromObserver is a regression
// test for an empty-tuple panic users hit when `ws.args[0]` was
// evaluated after `wait_command(command="wopen")`. The tutorial's
// Handle used to greedily resolve a wait_command after seeing
// `<cmd-key>wopen<enter>` in its own buffer; that resolve carried no
// args because Handle only saw keystrokes. The fix is to rely on the
// host's command observer — which fires with the post-expansion args
// — as the only source of resolution.
func TestTutorialRunnerWaitCommandArgsComeFromObserver(t *testing.T) {
	t.Parallel()
	src := `
def run():
    ws = wait_command(command="wopen")
    notify(level=info, message="opened " + ws.args[0])
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"argpanic", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, _ := newTestRunner(
		map[string]idetutorial.Tutorial{"argpanic": tut})
	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "argpanic"}}))
	require.NotNil(t, r.overlay)

	require.True(t, tut.WaitActive("wait_command", time.Second),
		"wait_command(wopen) did not become active")

	// Drive the same keystrokes ex.go would forward to the tutorial
	// during a `:wopen ~/proj<enter>` dispatch. The tutorial must
	// NOT resolve from these — args would be empty and the entry
	// would panic on ws.args[0].
	colon := term.Event{Type: term.EventKey, Ch: ':'}
	r.Handle(colon)
	for _, ch := range "wopen ~/proj" {
		r.Handle(term.Event{Type: term.EventKey, Ch: ch})
	}
	r.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})

	// The host's observer fires next — with the real args after
	// alias expansion. This is the only place wait_command resolves.
	r.observeCommand("wopen", "wopen", []string{"~/proj"}, nil)

	require.True(t, tut.WaitFinished(time.Second),
		"tutorial did not finish after observer fired with args")
	assert.Nil(t, r.overlay)
}

// TestTutorialRunnerObserveCommandAdvancesWaitCommand asserts that an
// observed alias dispatch reaches the overlay's ObserveCommand and
// advances the active wait_command step.
func TestTutorialRunnerObserveCommandAdvancesWaitCommand(t *testing.T) {
	t.Parallel()
	src := `
def run():
    wait_command(command="edit")
    notify(level=info, message="advanced via observer")
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"observe", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, _ := newTestRunner(
		map[string]idetutorial.Tutorial{"observe": tut})
	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "observe"}}))
	require.NotNil(t, r.overlay)

	r.observeCommand("e", "edit", []string{"somefile.go"}, nil)

	// notify is non-blocking, so the entry function returns and the
	// runtime is finished. The next Handle observes finished=true and
	// clears the overlay.
	deadline := time.After(time.Second)
	for r.overlay != nil {
		_, _ = r.Handle(term.Event{Type: term.EventKey, Ch: 'x'})
		select {
		case <-deadline:
			t.Fatal("runner did not clear overlay within 1s")
		default:
		}
	}
	assert.Nil(t, r.overlay,
		"runner should clear overlay once tutorial exits")
}

// TestTutorialRunnerObserveCommandKeepsArmedOnError asserts that a
// failed dispatch forwarded through observeCommand keeps the overlay
// mounted and does not advance the wait_command step.
func TestTutorialRunnerObserveCommandKeepsArmedOnError(t *testing.T) {
	t.Parallel()
	src := `
def run():
    wait_command(command="wopen", on_error="wopen <directory>")
    notify(level=info, message="should not fire")
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"on_error", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, _ := newTestRunner(
		map[string]idetutorial.Tutorial{"on_error": tut})
	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "on_error"}}))
	require.NotNil(t, r.overlay)

	r.observeCommand("wopen", "wopen", nil,
		errors.New("missing directory argument"))
	assert.NotNil(t, r.overlay,
		"failed dispatch must keep the overlay mounted")

	r.observeCommand("wopen", "wopen", []string{"~/proj"}, nil)
	deadline := time.After(time.Second)
	for r.overlay != nil {
		_, _ = r.Handle(term.Event{Type: term.EventKey, Ch: 'x'})
		select {
		case <-deadline:
			t.Fatal("runner did not drain overlay within 1s")
		default:
		}
	}
	assert.Nil(t, r.overlay,
		"successful dispatch + notify drains the overlay")
}

// TestTutorialRunnerObserveEventAdvancesWaitEvent asserts that an
// observed editor event reaches the overlay's ObserveEvent and
// advances the active wait_event step, then the runner clears the
// overlay once the tutorial finishes. A non-matching event observed
// first must keep the overlay armed.
func TestTutorialRunnerObserveEventAdvancesWaitEvent(t *testing.T) {
	t.Parallel()
	src := `
def run():
    wait_event(event="open")
    notify(level=info, message="advanced via event observer")
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"observe-event", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, _ := newTestRunner(
		map[string]idetutorial.Tutorial{"observe-event": tut})
	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "observe-event"}}))
	require.NotNil(t, r.overlay)

	r.observeEvent("close", "file:///x.go")
	assert.NotNil(t, r.overlay,
		"a non-matching event must keep the overlay mounted")

	r.observeEvent("open", "file:///x.go")
	deadline := time.After(time.Second)
	for r.overlay != nil {
		_, _ = r.Handle(term.Event{Type: term.EventKey, Ch: 'x'})
		select {
		case <-deadline:
			t.Fatal("runner did not clear overlay within 1s")
		default:
		}
	}
	assert.Nil(t, r.overlay,
		"runner should clear overlay once the wait_event step resolves")
}

// TestTutorialRunnerObserveEventNoOverlay asserts that observeEvent is
// a no-op when no tutorial overlay is mounted.
func TestTutorialRunnerObserveEventNoOverlay(t *testing.T) {
	t.Parallel()
	r, _ := newTestRunner(map[string]idetutorial.Tutorial{})
	require.Nil(t, r.overlay)
	r.observeEvent("open", "file:///x.go")
	assert.Nil(t, r.overlay)
}

// TestCommandObserverRegistryLateSubscribe asserts that a subscriber
// registered after the registry was already handed to ex still
// receives subsequent dispatches.
func TestCommandObserverRegistryLateSubscribe(t *testing.T) {
	t.Parallel()
	reg := newCommandObserverRegistry()

	var exObs commandObserver = reg
	exObs.observeCommand("noop", "noop", nil, nil)

	stub := &recordingObserver{}
	reg.subscribe(stub)

	exObs.observeCommand("wopen", "wopen", []string{"~/proj"}, nil)
	require.Len(t, stub.calls, 1,
		"subscriber registered after construction must still receive dispatches")
	assert.Equal(t, "wopen", stub.calls[0])

	reg.subscribe(nil)
	assert.Len(t, reg.subscribers, 1)
}

type recordingObserver struct {
	calls []string
}

func (r *recordingObserver) observeCommand(
	typed, resolved string, args []string, err error,
) {
	r.calls = append(r.calls, typed)
}

// TestTutorialRunnerWrongKeyOnFloatingWindowSwallowedAndPulses asserts
// that a stray key on an active floating_window step is swallowed by
// the tutorial (never reaching the IDE root) and flips Shader() from
// false to true through the full runner -> handler -> tutorial path.
func TestTutorialRunnerWrongKeyOnFloatingWindowSwallowedAndPulses(t *testing.T) {
	t.Parallel()
	src := `
def run():
    floating_window(title="welcome", text="press enter or esc")
    notify(level=info, message="done")
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"wrongkey", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, root := newTestRunner(
		map[string]idetutorial.Tutorial{"wrongkey": tut})

	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "wrongkey"}}))
	require.NotNil(t, r.overlay)

	_, want := tut.Shader()
	require.False(t, want,
		"fresh floating_window must not declare a shader")

	_, handled := r.Handle(term.Event{Type: term.EventKey, Ch: ':'})
	assert.True(t, handled,
		"stray ':' on floating_window must be swallowed by the "+
			"tutorial — the IDE root must never see it")
	assert.Equal(t, 0, root.handled,
		"root must not have observed the stray ':'")

	_, want = tut.Shader()
	assert.True(t, want,
		"wrong key on floating_window must stage a hint pulse")
}

// TestTutorialRunnerPromptSurvivesRootRewire asserts that a
// confirm/choice overlay rendered by the tutorial does not depend on
// the runner's root handler. After the runner's root handler is
// swapped (simulating a workspace switch), the tutorial overlay
// continues to render the prompt's message and options, and Enter
// still resolves the prompt.
//
// Regression for RUNE-188: the old design installed the prompt as a
// browser.Window via idetutorial.Prompter; that handle was attached
// to whichever workspace was focused at the time and was lost across
// workspace switches. The new design renders the prompt directly on
// the tutorial layer so the runner's root is irrelevant.
func TestTutorialRunnerPromptSurvivesRootRewire(t *testing.T) {
	t.Parallel()
	src := `
def run():
    yes = confirm("continue?")
    if yes:
        notify(level=info, message="ok")
tutorial(entry=run)
`
	tut, err := starlarktutorial.New(
		"survives", src,
		nil, nil, nil, nil,
		term.Attributes{},
		component.FrameCharSet{},
		browser.PromptConfig{},
		nil, nil,
		term.KeyComb{Ch: ':'},
		"modeless", nil,
		nil,
	)
	require.NoError(t, err)

	r, _ := newTestRunner(
		map[string]idetutorial.Tutorial{"survives": tut})
	require.NoError(t, r.HandleCommand(context.Background(),
		textapi.Command{Name: "tutorial", Args: []string{"start", "survives"}}))
	require.NotNil(t, r.overlay)

	require.True(t, tut.WaitActive("confirm", time.Second),
		"confirm did not become active")

	// Simulate a workspace switch by swapping the runner's root.
	// The tutorial overlay captured the original root at setActive
	// time but renders its own prompt; neither the captured root
	// nor the runner's current root contribute to the prompt's
	// pixels.
	r.Handler = &rootStub{}
	r.Resize(80, 24)

	g := newGridWriter80x24()
	// Draw the overlay directly: that's what the runner ends up
	// calling. The overlay's tut.Draw paints the prompt regardless
	// of which root is underneath, which is the contract we are
	// asserting.
	r.overlay.Draw(g)
	assert.True(t, gridStringContains(g, "continue?"),
		"prompt message must be drawn on the overlay layer, not the root")
	assert.True(t, gridStringContains(g, "Yes"),
		"prompt option must be drawn on the overlay layer, not the root")

	_, _ = r.Handle(term.Event{Type: term.EventKey, Key: term.KeyEnter})
	require.True(t, tut.WaitFinished(time.Second),
		"tutorial must finish after Enter resolves the confirm")
}

// gridWriter80x24 is a minimal recording term.Writer used by
// TestTutorialRunnerPromptSurvivesRootRewire so we don't have to
// import the starlarktutorial test helpers.
type gridWriter80x24 struct {
	w, h  int
	cells [][]rune
}

func newGridWriter80x24() *gridWriter80x24 {
	const w, h = 80, 24
	cells := make([][]rune, h)
	for i := range cells {
		cells[i] = make([]rune, w)
	}
	return &gridWriter80x24{w: w, h: h, cells: cells}
}

func (g *gridWriter80x24) SetCell(pos term.Coordinates, c term.Cell) {
	if pos.X < 0 || pos.Y < 0 || pos.X >= g.w || pos.Y >= g.h {
		return
	}
	g.cells[pos.Y][pos.X] = c.Ch
}

func (g *gridWriter80x24) Context() context.Context                              { return context.Background() }
func (g *gridWriter80x24) UnionAttributes(_ term.Coordinates, _ term.Attributes) {}

func gridStringContains(g *gridWriter80x24, needle string) bool {
	for y := range g.h {
		row := string(g.cells[y])
		if strings.Contains(row, needle) {
			return true
		}
	}
	return false
}
