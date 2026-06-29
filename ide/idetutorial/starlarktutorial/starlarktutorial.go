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

// Package starlarktutorial parses a starlark tutorial DSL program and
// runs it as an in-process [idetutorial.Tutorial]. The DSL exposes a
// single `tutorial(entry=fn)` registration plus a set of blocking
// builtins (floating_window, wait_command, choice, ...) that authors
// invoke from a regular Starlark function. The entry function runs on
// a dedicated goroutine; each blocking builtin posts a request to the
// TUI loop, waits for a response, and returns a real Starlark value
// so authors can branch, loop, and compose helpers naturally.
// NOTE: this package needs heavy refactoring, it's currently a pile
// of AI slop.
package starlarktutorial

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/syntaxapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/idetutorial"
	"unstable.build/go-tui/text"
)

// errStopped is returned from a blocking builtin when the Tutorial's
// run context is cancelled. The runLoop recognises it as a clean exit
// path and does not surface a notification.
var errStopped = errors.New("starlarktutorial: stopped")

// errExitRequested is raised by the exit() builtin to terminate the
// entry function from any depth. The runLoop unwraps it from a
// starlark.EvalError and treats it as a clean exit.
var errExitRequested = errors.New("starlarktutorial: exit requested")

// CommandManualLookup resolves a registered command name to its
// manual, so the wait_command hint window can render the command's
// synopsis and description. ok=false when no command (or alias)
// matches name. Implementations may be called from the TUI loop in
// Tutorial.Draw; they must be safe to call concurrently and return
// quickly.
type CommandManualLookup func(name string) (command.Manual, bool)

// Tutorial is the runtime state of a single starlark-defined tutorial.
// It satisfies [idetutorial.Tutorial].
type Tutorial struct {
	name    string
	title   string
	id      string
	version string

	browser          browser.Browser
	editor           text.Editor
	notifications    browserapi.Notifications
	parser           syntaxapi.Parser
	defaultAttr      term.Attributes
	frameCharSet     component.FrameCharSet
	scheduleNextTick func(func()) bool
	storage          storageapi.Service
	// commandKeyDisplay is the prettified, display-ready command-prompt
	// key spec (e.g. ":" rather than "<shift-;>"). It is the single
	// rendered form every tutorial surface uses; nothing re-renders the
	// raw term.KeyComb, so a new render site cannot reintroduce the ugly
	// spec.
	commandKeyDisplay string
	// editorMode is the user's resolved editor mode ("modal" or
	// "modeless"), exposed to the DSL via editor_mode(). exo is
	// resolved to its fallback by the host before New.
	editorMode string
	// keyForCommand resolves a command (and optional args) to the
	// user's configured key spec, or "" when unbound. nil disables
	// key_for() lookups (they return ""). Used by key_for().
	keyForCommand func(cmd string, args []string) string
	// promptConfig carries the IDE's confirm/choice prompt styling
	// (option, highlight, background attributes and minimum width) so
	// tutorial prompts match the IDE's browser-driven prompts.
	promptConfig browser.PromptConfig
	// commandManualLookup resolves a command name to its registered
	// manual, used by the wait_command hint window so the user sees
	// the command's synopsis and description while the request is
	// armed. nil disables manual rendering: the hint falls back to a
	// plain prefix line.
	commandManualLookup CommandManualLookup

	// parsed entry function. Set once at New time.
	entry *starlark.Function

	// Runtime state. mu guards everything below.
	mu       sync.Mutex
	width    int
	height   int
	active   *request
	runCtx   context.Context
	cancel   context.CancelFunc
	thread   *starlark.Thread
	runDone  chan struct{}
	finished bool

	// stepCount is the count of "visible content" requests
	// published so far (reqFloatingWindow / reqMarkdown). Each
	// publish snapshots this value into request.stepNum. Reset()
	// zeroes it so re-running the tutorial restarts at Step 1.
	stepCount int

	// firstSignal is set by Reset to a one-shot signal that the
	// runtime has made user-visible progress (posted its first
	// blocking request, scheduled its first side effect, or exited).
	// Reset waits on it so subsequent Draw/Handle/Close observe a
	// stable state — and so a test that drives input immediately
	// after Reset never races with the run goroutine.
	firstSignal func()
}

var _ idetutorial.Tutorial = (*Tutorial)(nil)

// New parses src as a starlark tutorial DSL program and returns a
// runnable Tutorial. The program must call `tutorial(entry=fn)`
// exactly once at top level; fn becomes the entry function the
// Starlark thread executes on Reset. name is the registry name used
// for error reporting and as the default id. The remaining arguments
// are the host services the DSL builtins resolve at runtime.
func New(
	name, src string,
	br browser.Browser,
	ed text.Editor,
	notifications browserapi.Notifications,
	parser syntaxapi.Parser,
	defaultAttr term.Attributes,
	frameCharSet component.FrameCharSet,
	promptConfig browser.PromptConfig,
	scheduleNextTick func(func()) bool,
	storage storageapi.Service,
	commandKey term.KeyComb,
	editorMode string,
	keyForCommand func(cmd string, args []string) string,
	commandManualLookup CommandManualLookup,
) (*Tutorial, error) {
	if src == "" {
		return nil, errors.New("starlarktutorial: empty source")
	}
	t := &Tutorial{
		name:                name,
		browser:             br,
		editor:              ed,
		notifications:       notifications,
		parser:              parser,
		defaultAttr:         defaultAttr,
		frameCharSet:        frameCharSet,
		promptConfig:        promptConfig,
		scheduleNextTick:    scheduleNextTick,
		storage:             storage,
		commandKeyDisplay:   PrettyKeySpec(commandKey.String()),
		editorMode:          editorMode,
		keyForCommand:       keyForCommand,
		commandManualLookup: commandManualLookup,
	}

	if err := t.parse(src); err != nil {
		return nil, fmt.Errorf("starlarktutorial %q: %w", name, err)
	}
	if t.entry == nil {
		return nil, fmt.Errorf("starlarktutorial %q: tutorial() never called",
			name)
	}
	return t, nil
}

// parse runs the registration phase: a Starlark module that defines
// `tutorial(entry=fn, ...)` and stops. Blocking-UI builtins are
// exposed but raise an error if invoked at parse time so authors get
// a clear message if they call them outside the entry function.
func (t *Tutorial) parse(src string) error {
	thread := &starlark.Thread{
		Name:  t.name + ".star",
		Print: func(*starlark.Thread, string) {},
		Load: func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
			return nil, fmt.Errorf("load() is not allowed: cannot load %q",
				module)
		},
	}
	opts := &syntax.FileOptions{
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}
	predeclared := builtins(t)
	_, err := starlark.ExecFileOptions(opts, thread, t.name+".star",
		[]byte(src), predeclared)
	if err != nil {
		var evalErr *starlark.EvalError
		if errors.As(err, &evalErr) {
			return fmt.Errorf("starlark: %s", evalErr.Backtrace())
		}
		return fmt.Errorf("starlark: %w", err)
	}
	return nil
}

// Name returns the registry name passed to New.
func (t *Tutorial) Name() string { return t.name }

// Title returns the title declared by the DSL, falling back to Name
// when no title was provided.
func (t *Tutorial) Title() string {
	if t.title != "" {
		return t.title
	}
	return t.name
}

// ID returns the stable tutorial ID, falling back to Name when no id
// was provided.
func (t *Tutorial) ID() string {
	if t.id != "" {
		return t.id
	}
	return t.name
}

// Version returns the tutorial version string, or "" when not
// provided.
func (t *Tutorial) Version() string { return t.version }

// Reset stops any in-progress run and starts a fresh Starlark thread
// that calls the entry function on its own goroutine. Reset returns
// after launching the goroutine; Draw/Handle observe an empty active
// slot until the entry posts its first blocking request, at which
// point the slot is filled atomically.
func (t *Tutorial) Reset() {
	t.Stop()
	t.mu.Lock()
	t.finished = false
	t.active = nil
	t.stepCount = 0
	t.runCtx, t.cancel = context.WithCancel(context.Background())
	t.thread = &starlark.Thread{
		Name:  t.name + ".run",
		Print: func(*starlark.Thread, string) {},
		Load: func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
			return nil, fmt.Errorf("load() is not allowed: cannot load %q",
				module)
		},
	}
	t.runDone = make(chan struct{})
	ready := make(chan struct{})
	var once sync.Once
	t.firstSignal = func() { once.Do(func() { close(ready) }) }
	t.mu.Unlock()
	t.runLoop()
	<-ready
}

// Stop tears down any in-progress run and unblocks the Starlark
// thread. Stop is safe to call multiple times and on a tutorial that
// never ran. Stop waits for the run goroutine to exit before
// returning.
func (t *Tutorial) Stop() {
	t.mu.Lock()
	cancel := t.cancel
	thread := t.thread
	done := t.runDone
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if thread != nil {
		thread.Cancel("stopped")
	}
	if done != nil {
		<-done
	}
	t.mu.Lock()
	t.cancel = nil
	t.thread = nil
	t.runDone = nil
	t.active = nil
	t.mu.Unlock()
}

// runLoop starts the goroutine that calls the entry function and
// returns immediately. The goroutine drives request publication and
// completion; Stop() waits for it to exit.
func (t *Tutorial) runLoop() {
	t.mu.Lock()
	if t.entry == nil || t.thread == nil {
		t.finished = true
		if t.runDone != nil {
			close(t.runDone)
			t.runDone = nil
		}
		t.mu.Unlock()
		return
	}
	thread := t.thread
	entry := t.entry
	done := t.runDone
	t.mu.Unlock()

	go func() {
		defer close(done)
		_, err := starlark.Call(thread, entry, nil, nil)
		t.handleRunResult(err)
	}()
}

// handleRunResult finalises a run: marks the tutorial finished and
// surfaces any non-cancellation error via the notifications service.
func (t *Tutorial) handleRunResult(err error) {
	t.mu.Lock()
	t.finished = true
	t.active = nil
	t.runCtx = nil
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.thread = nil
	signal := t.firstSignal
	t.firstSignal = nil
	t.mu.Unlock()
	if signal != nil {
		signal()
	}

	if err == nil {
		return
	}
	// Cancellation is a clean exit; do not notify the user.
	if errors.Is(err, errStopped) {
		return
	}
	if isStarlarkExit(err) {
		return
	}
	var evalErr *starlark.EvalError
	if errors.As(err, &evalErr) {
		t.runOnTUI(func() {
			if t.notifications != nil {
				_, _ = t.notifications.Notify(browserapi.LevelError,
					"%s: %s", t.Title(), evalErr.Msg)
			}
		})
		return
	}
	t.runOnTUI(func() {
		if t.notifications != nil {
			_, _ = t.notifications.Notify(browserapi.LevelError,
				"%s: %v", t.Title(), err)
		}
	})
}

// isStarlarkExit reports whether err originated from the exit()
// builtin. The runLoop treats such errors as a clean exit.
func isStarlarkExit(err error) bool {
	if errors.Is(err, errExitRequested) {
		return true
	}
	var evalErr *starlark.EvalError
	if errors.As(err, &evalErr) && evalErr.Unwrap() != nil {
		return errors.Is(evalErr.Unwrap(), errExitRequested)
	}
	return false
}

// Shader returns the current step's declared shader spec, if any.
// ok=false when the tutorial is done or the current request has no
// shader.
func (t *Tutorial) Shader() (idetutorial.Shader, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || t.active == nil || !t.active.hasShader {
		return idetutorial.Shader{}, false
	}
	return t.active.shaderSpec, true
}

// SetDefaultAttributes updates the tutorial's view of the default
// terminal attributes and restages the active step's shader spec so
// the next reconcile rebuilds the underlying shader with the new
// attributes.
func (t *Tutorial) SetDefaultAttributes(defAttr term.Attributes) {
	t.mu.Lock()
	t.defaultAttr = defAttr
	if t.active != nil && t.active.kind == reqFloatingWindow {
		stageFloatingWindowShader(t.active, t.width, t.height,
			t.frameCharSet, t.defaultAttr)
	}
	t.mu.Unlock()
}

// Resize records the most recent dimensions and restages the active
// step's shader spec so a terminal resize is reflected on the next
// reconcile.
func (t *Tutorial) Resize(width, height int) {
	t.mu.Lock()
	t.width, t.height = width, height
	if t.active != nil && t.active.kind == reqFloatingWindow {
		stageFloatingWindowShader(t.active, width, height,
			t.frameCharSet, t.defaultAttr)
	}
	t.mu.Unlock()
}

// Draw paints the current step's overlay on top of whatever was
// previously drawn into w. Out-of-range writes are silently dropped.
// Draw performs no state mutation.
func (t *Tutorial) Draw(w term.Writer) {
	t.mu.Lock()
	active := t.active
	width, height := t.width, t.height
	fcs := t.frameCharSet
	attr := t.defaultAttr
	finished := t.finished
	cmdKey := t.commandKeyDisplay
	lookup := t.commandManualLookup
	keyForCmd := t.keyForCommand
	t.mu.Unlock()
	if finished || active == nil {
		return
	}
	switch active.kind {
	case reqFloatingWindow:
		drawFloatingWindow(w, width, height, active, fcs, attr)
	case reqMarkdown:
		drawBanner(w, width, height, []string{active.text})
	case reqWaitKey:
		drawHintBox(w, width, height, active.title, active.stepNum,
			"Press "+active.waitKey+" to continue.",
			fcs, attr)
	case reqWaitCommand:
		body := buildWaitCommandHint(active, cmdKey, lookup, keyForCmd)
		drawHintBox(w, width, height, active.title, active.stepNum, body, fcs, attr)
	case reqWaitShell:
		body := buildWaitShellHint(active, cmdKey)
		drawHintBox(w, width, height, active.title, active.stepNum, body, fcs, attr)
	case reqWaitEvent:
		drawHintBox(w, width, height, active.title, active.stepNum,
			active.text, fcs, attr)
	case reqChoice, reqConfirm:
		drawPromptOverlay(w, width, height, active, fcs, attr)
	}
}

// Handle advances the state machine on input. exit=true once the
// tutorial finishes or is dismissed.
func (t *Tutorial) Handle(ev term.Event) (bool, bool) {
	if ev.Type != term.EventKey {
		return t.exitState(), false
	}
	t.mu.Lock()
	active := t.active
	finished := t.finished
	t.mu.Unlock()
	if finished {
		return true, false
	}
	if active == nil {
		return false, false
	}
	switch active.kind {
	case reqFloatingWindow:
		return t.handleFloatingWindow(active, ev)
	case reqMarkdown:
		return t.handleMarkdown(active, ev)
	case reqWaitKey:
		return t.handleWaitKey(active, ev)
	case reqWaitCommand:
		return t.handleWaitCommand(active, ev)
	case reqWaitShell:
		return t.handleWaitCommand(active, ev)
	case reqWaitEvent:
		return t.handleWaitEvent(active, ev)
	case reqChoice, reqConfirm:
		return t.handlePrompt(active, ev)
	}
	return false, false
}

func (t *Tutorial) handleFloatingWindow(r *request, ev term.Event) (bool, bool) {
	if ev.Mod == 0 && (ev.Key == term.KeyEnter || ev.Key == term.KeyEsc ||
		ev.Key == term.KeySpace || ev.Ch == ' ') {
		t.resolve(r, response{})
		return t.exitState(), true
	}
	if ev.Type == term.EventKey {
		kc := ev.KeyComb()
		// dismiss_keys: resolve the window AND let the event reach
		// the IDE root, so a read-then-act binding (e.g. "Press
		// `:` to open the command prompt") advances the tutorial
		// at the same time the user's keypress triggers the
		// described action.
		if slices.Contains(r.dismissKeys, kc) {
			t.resolve(r, response{})
			return t.exitState(), false
		}
		// allow_keys: let the author explicitly pass specific keys
		// through to the IDE root so the user can act on the very
		// bindings they are reading about (e.g. <meta-1>..<meta-9>)
		// without advancing the tutorial.
		if slices.Contains(r.allowKeys, kc) {
			return false, false
		}
	}
	// Swallow stray keys so the IDE root never sees a stray ':' that
	// would open a command prompt under the overlay; arm the hint
	// pulse so the user notices.
	t.mu.Lock()
	if r.shaderSpec.Shader != nil {
		r.hasShader = true
	}
	t.mu.Unlock()
	return false, true
}

func (t *Tutorial) handleMarkdown(r *request, ev term.Event) (bool, bool) {
	if ev.Key == term.KeyEnter || ev.Key == term.KeyEsc ||
		ev.Key == term.KeySpace || ev.Ch == ' ' {
		t.resolve(r, response{})
		return t.exitState(), true
	}
	return false, false
}

func (t *Tutorial) handleWaitKey(r *request, ev term.Event) (bool, bool) {
	want, err := term.ParseKeys(r.waitKey)
	if err != nil || len(want) != 1 {
		// Bad key spec: advance on any keystroke so the user is
		// never stuck on an unreachable step.
		t.resolve(r, response{})
		return t.exitState(), true
	}
	k := want[0]
	if ev.Key == k.Key && ev.Mod == k.Mod && ev.Ch == k.Ch {
		t.resolve(r, response{})
		return t.exitState(), true
	}
	return false, false
}

// handleWaitCommand never resolves from keystrokes: the host's
// command observer is the source of truth for command dispatches and
// carries the real args. handleWaitCommand only swallows events when
// the on_error hint has been swapped in so the user sees the hint
// instead of falling through to the root.
func (t *Tutorial) handleWaitCommand(_ *request, _ term.Event) (bool, bool) {
	return false, false
}

// handleWaitEvent never resolves from keystrokes and never swallows:
// the host's editor-event observer is the source of truth, so every
// key falls through to the IDE root while the hint stays up.
func (t *Tutorial) handleWaitEvent(_ *request, _ term.Event) (bool, bool) {
	return false, false
}

// handlePrompt forwards ev to the active prompt overlay and finalises
// the request on exit. Enter triggers OnSelect (which stamps
// r.pendingResp/pendingSelected); Esc only sets the prompt's exit
// flag. handlePrompt then resolves with the pending response or
// synthesises the per-kind dismissal response. resolve() installs
// the barrier before delivery so the run goroutine's next publish
// is observed.
func (t *Tutorial) handlePrompt(r *request, ev term.Event) (bool, bool) {
	if r.prompt == nil {
		return false, false
	}
	exit, handled := r.promptVirtual.Handle(ev)
	if !exit {
		return false, handled
	}
	res := r.pendingResp
	if !r.pendingSelected {
		if r.kind == reqConfirm {
			res = response{confirmed: false}
		} else {
			res = response{selectedIdx: -1, selected: false}
		}
	}
	t.resolve(r, res)
	return t.exitState(), handled
}

// Cursor returns no cursor; tutorial overlays do not own the cursor.
func (t *Tutorial) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	return term.Coordinates{}, term.CursorStyleDefault, false
}

// Selection returns no selection.
func (t *Tutorial) Selection() (string, bool) { return "", false }

// ObserveCommand advances the state machine when the current step is
// wait_command, the dispatched command matches typed or resolved, and
// err is nil. On dispatch error the request is kept armed and the
// hint is swapped to the on_error message. Returns exit=true when
// the tutorial finishes as a result of this observation.
func (t *Tutorial) ObserveCommand(
	typed, resolved string, args []string, err error,
) bool {
	t.mu.Lock()
	active := t.active
	finished := t.finished
	t.mu.Unlock()
	if finished {
		return true
	}
	if active == nil {
		return false
	}
	if active.kind == reqWaitShell {
		return t.observeShellCommand(active, typed, resolved, args, err)
	}
	if active.kind != reqWaitCommand {
		return false
	}
	if active.command != typed && active.command != resolved {
		return false
	}
	if err != nil {
		// Do not surface err as a tutorial notification: the IDE's
		// command prompt already shows the underlying error.
		if active.onError != "" {
			active.text = expandCmdTemplate(active.onError, t.commandKeyDisplay)
		}
		return false
	}
	t.resolve(active, response{cmdName: active.command, cmdArgs: args})
	return t.exitState()
}

// ObserveEvent advances the state machine when the current step is
// wait_event and the observed editor event's type name matches the
// armed event name. uri is accepted for parity with the event payload
// but is not matched (no scheme/URI filtering). Returns exit=true when
// the tutorial finishes as a result of this observation.
func (t *Tutorial) ObserveEvent(eventType, _ string) bool {
	t.mu.Lock()
	active := t.active
	finished := t.finished
	t.mu.Unlock()
	if finished {
		return true
	}
	if active == nil || active.kind != reqWaitEvent || active.event != eventType {
		return false
	}
	t.resolve(active, response{})
	return t.exitState()
}

// shellCommandName is the typed/resolved command name under which the
// IDE reports companion-console REPL submissions to the command
// observer. The console wrapper prepends the REPL command name to the
// observed args (e.g. ["pkg", "install", "rune-agent"]) so a
// wait_shell step can match on argument tokens alone.
const shellCommandName = "console"

// observeShellCommand advances a reqWaitShell step. It only reacts to
// companion-shell observations (typed/resolved == shellCommandName)
// and requires every expected token to be present in the observed
// args (containment, so completion and alias variants still match). A
// dispatch error keeps the step armed and swaps in the on_error hint.
func (t *Tutorial) observeShellCommand(
	active *request, typed, resolved string, args []string, err error,
) bool {
	if typed != shellCommandName && resolved != shellCommandName {
		return false
	}
	if err != nil {
		if active.onError != "" {
			active.text = expandCmdTemplate(active.onError, t.commandKeyDisplay)
		}
		return false
	}
	if !argsContainAll(args, active.shellArgs) {
		return false
	}
	t.resolve(active, response{cmdName: shellCommandName, cmdArgs: args})
	return t.exitState()
}

// argsContainAll reports whether every token in want appears in have.
func argsContainAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

// publishRequest blocks until the runLoop is allowed to install r as
// the active request and waits for the TUI loop to deliver a
// response or for the run context to be cancelled. Side-effect
// builtins (notify, open_file, highlight_window) do not call this;
// they run inline via runOnTUI instead.
func (t *Tutorial) publishRequest(r *request) (response, error) {
	t.mu.Lock()
	if t.runCtx == nil {
		t.mu.Unlock()
		return response{}, errStopped
	}
	ctx := t.runCtx
	width, height := t.width, t.height
	fcs := t.frameCharSet
	defAttr := t.defaultAttr
	r.respond = make(chan response, 1)
	if r.kind == reqFloatingWindow {
		stageFloatingWindowShader(r, width, height, fcs, defAttr)
	}
	if r.kind == reqFloatingWindow || r.kind == reqMarkdown {
		t.stepCount++
		r.stepNum = t.stepCount
	} else {
		r.stepNum = t.stepCount
	}
	if r.kind == reqConfirm || r.kind == reqChoice {
		buildPromptOverlay(r, t.promptConfig)
	}
	t.active = r
	signal := t.firstSignal
	t.firstSignal = nil
	t.mu.Unlock()
	if signal != nil {
		signal()
	}

	select {
	case res := <-r.respond:
		return res, nil
	case <-ctx.Done():
		return response{}, errStopped
	}
}

// buildPromptOverlay constructs the handler.Prompt and its
// positioning wrapper for a reqConfirm/reqChoice request. OnSelect
// stamps the request's pending response fields; handlePrompt then
// calls resolve with that response so the resolve barrier is in
// place before delivery. OnClose is a no-op here: handlePrompt
// observes prompt exit and resolves with the dismissal response
// directly when no OnSelect ran.
func buildPromptOverlay(r *request, promptCfg browser.PromptConfig) {
	ph := handler.FuncPromptHandler(
		func(idx int, option string) {
			value := option
			if idx >= 0 && idx < len(r.options) {
				value = r.options[idx]
			}
			r.pendingResp = response{
				selectedIdx:   idx,
				selectedValue: value,
				selected:      true,
			}
			if r.kind == reqConfirm {
				r.pendingResp.confirmed = idx == 0
			}
			r.pendingSelected = true
		},
		func() error {
			return nil
		},
	)
	cfg := handler.PromptConfig{
		PromptConfig: component.PromptConfig{
			Message:              r.message,
			Options:              padPromptOptions(r.options),
			BackgroundAttributes: promptCfg.BackgroundAttr,
			MinWidth:             promptCfg.MinWidth,
			NewMessage:           newPromptMarkdownMessage,
		},
		PromptHandler: ph,
		OptionAttr:    promptCfg.TextAttr,
		HighlightAttr: promptCfg.HighlightAttr,
	}
	r.prompt = handler.NewPrompt(cfg)
	r.promptVirtual = &handler.Virtual[*handler.Prompt]{}
	r.promptVirtual.C = r.prompt
}

// padPromptOptions surrounds each option label with a single space on
// either side so the rendered prompt buttons are naturally padded,
// matching the IDE's browser-driven prompts. The returned slice is for
// display only; the unpadded labels remain the authoritative selection
// values via request.options.
func padPromptOptions(options []string) []string {
	padded := make([]string, len(options))
	for i, o := range options {
		padded[i] = " " + o + " "
	}
	return padded
}

// newPromptMarkdownMessage renders a confirm/choice prompt message as
// markdown, mirroring browser.Component.Prompt so tutorial prompts
// match the IDE's prompts. It falls back to a centered plain string
// when the message is not valid markdown.
func newPromptMarkdownMessage(str string) component.Floating {
	mcfg := markdown.DefaultConfig()
	mcfg.HeaderPrefix = false
	if mkd, err := markdown.NewWithConfig(str, mcfg); err == nil {
		return component.NewAspectRatioFloatingResponsive(
			component.NewSpan(mkd, component.SpanConfig{
				PadHorizontal:    4,
				PadVertical:      2,
				ContentAlignment: component.AlignmentCentered,
			}), component.DefaultAspectRatio)
	}
	messageResponsive := component.NewResponsiveString(str,
		component.StringResponsiveConfig{
			NoSplitWords: true,
			StringConfig: component.StringConfig{
				PaddingVertical:   4,
				PaddingHorizontal: 4,
				Alignment:         component.AlignmentCentered,
			},
		})
	return component.NewAspectRatioFloatingResponsive(
		messageResponsive, component.DefaultAspectRatio)
}

// resolve delivers res to r and clears the active slot. The TUI loop
// calls this from Draw/Handle; multiple resolves on the same request
// are no-ops thanks to request.deliver's sync.Once. After delivery,
// resolve installs a one-shot barrier and blocks until the run
// goroutine reaches its next observable state (a freshly published
// request, an inline side effect, or run completion), so that
// Handle/ObserveCommand callers can assume the runtime is quiescent
// on return.
func (t *Tutorial) resolve(r *request, res response) {
	barrier := make(chan struct{})
	var once sync.Once
	signal := func() { once.Do(func() { close(barrier) }) }

	t.mu.Lock()
	if t.active == r {
		t.active = nil
	}
	if t.finished {
		// The run goroutine has already exited (e.g. raced an
		// exit/fail with the dispatch). Skip the barrier — there
		// will be no more state transitions.
		t.mu.Unlock()
		r.deliver(res)
		return
	}
	t.firstSignal = signal
	t.mu.Unlock()

	r.deliver(res)
	<-barrier
}

// exitState reports whether the runtime has finished. Used as the
// `exit` return for Handle/ObserveCommand after resolving a request.
func (t *Tutorial) exitState() bool {
	t.mu.Lock()
	finished := t.finished
	t.mu.Unlock()
	return finished
}

// WaitActive blocks until the run goroutine publishes a request whose
// kind matches want or the tutorial finishes, whichever happens
// first. It is intended for tests that drive the runtime
// synchronously through Handle/ObserveCommand and need a barrier
// between events. Returns true when the matching request became
// active, false on timeout or when the tutorial finished without
// publishing such a request. want is one of "floating_window",
// "markdown", "wait_key", "wait_command", "confirm", "choice".
func (t *Tutorial) WaitActive(want string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		t.mu.Lock()
		active := t.active
		finished := t.finished
		t.mu.Unlock()
		if finished {
			return false
		}
		if active != nil && active.kind.String() == want {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

// WaitFinished blocks until the run goroutine has exited or d
// elapses. Returns true once finished. Intended for tests.
func (t *Tutorial) WaitFinished(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		t.mu.Lock()
		done := t.runDone
		finished := t.finished
		t.mu.Unlock()
		if finished && done == nil {
			return true
		}
		if done != nil {
			select {
			case <-done:
				return true
			case <-time.After(time.Until(deadline)):
				return false
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}
