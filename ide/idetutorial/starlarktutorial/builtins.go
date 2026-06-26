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
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/component/markdown"
)

// builtins returns the predeclared globals exposed to a tutorial
// script: the tutorial() registration, blocking-UI builtins, side-
// effect builtins, level constants, and small helpers like exit().
func builtins(t *Tutorial) starlark.StringDict {
	return starlark.StringDict{
		// Registration + helpers.
		"tutorial":          starlark.NewBuiltin("tutorial", builtinTutorial(t)),
		"command_key":       starlark.NewBuiltin("command_key", builtinCommandKey(t)),
		"editor_mode":       starlark.NewBuiltin("editor_mode", builtinEditorMode(t)),
		"key_for":           starlark.NewBuiltin("key_for", builtinKeyFor(t)),
		"exit":              starlark.NewBuiltin("exit", builtinExit()),
		"cancel_on_dismiss": starlark.NewBuiltin("cancel_on_dismiss", builtinCancelOnDismiss()),

		// Notification level constants.
		"error":   starlark.MakeInt(int(browserapi.LevelError)),
		"warn":    starlark.MakeInt(int(browserapi.LevelWarn)),
		"info":    starlark.MakeInt(int(browserapi.LevelInfo)),
		"success": starlark.MakeInt(int(browserapi.LevelSuccess)),

		// Blocking UI builtins.
		"floating_window": starlark.NewBuiltin("floating_window", builtinFloatingWindow(t)),
		"markdown":        starlark.NewBuiltin("markdown", builtinMarkdown(t)),
		"wait_key":        starlark.NewBuiltin("wait_key", builtinWaitKey(t)),
		"wait_command":    starlark.NewBuiltin("wait_command", builtinWaitCommand(t)),
		"wait_shell":      starlark.NewBuiltin("wait_shell", builtinWaitShell(t)),
		"wait_event":      starlark.NewBuiltin("wait_event", builtinWaitEvent(t)),
		"confirm":         starlark.NewBuiltin("confirm", builtinConfirm(t)),
		"choice":          starlark.NewBuiltin("choice", builtinChoice(t)),

		// Side-effect builtins.
		"notify":           starlark.NewBuiltin("notify", builtinNotify(t)),
		"open_file":        starlark.NewBuiltin("open_file", builtinOpenFile(t)),
		"highlight_window": starlark.NewBuiltin("highlight_window", builtinHighlightWindow(t)),
	}
}

func builtinTutorial(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		if t.entry != nil {
			return nil, errors.New("tutorial(): already called in this file")
		}
		var (
			id      starlark.String
			title   starlark.String
			version starlark.String
			entry   starlark.Value
		)
		if err := starlark.UnpackArgs("tutorial", args, kwargs,
			"entry", &entry,
			"id?", &id,
			"title?", &title,
			"version?", &version,
		); err != nil {
			return nil, err
		}
		fn, ok := entry.(*starlark.Function)
		if !ok {
			return nil, fmt.Errorf("tutorial(): entry must be a function, "+
				"got %s", entry.Type())
		}
		if fn.NumParams() != 0 {
			return nil, fmt.Errorf("tutorial(): entry %q must take zero "+
				"arguments (has %d)", fn.Name(), fn.NumParams())
		}
		t.id = string(id)
		t.title = string(title)
		t.version = string(version)
		t.entry = fn
		return starlark.None, nil
	}
}

// builtinFloatingWindow runs synchronously on the Starlark goroutine.
// It builds a request, posts it via publishRequest, and returns when
// the TUI loop delivers a response. Returns None to the script.
func builtinFloatingWindow(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			text        starlark.String
			title       starlark.String
			alignment   starlark.String
			offset      starlark.Value
			allowKeys   *starlark.List
			dismissKeys *starlark.List
		)
		if err := starlark.UnpackArgs("floating_window", args, kwargs,
			"text", &text,
			"title?", &title,
			"alignment?", &alignment,
			"offset?", &offset,
			"allow_keys?", &allowKeys,
			"dismiss_keys?", &dismissKeys); err != nil {
			return nil, err
		}
		align, err := parseFloatingAlignment(string(alignment))
		if err != nil {
			return nil, fmt.Errorf("floating_window: %w", err)
		}
		off, err := parseFloatingOffset(offset)
		if err != nil {
			return nil, fmt.Errorf("floating_window: %w", err)
		}
		keys, err := parseKeyList(allowKeys, "allow_keys")
		if err != nil {
			return nil, fmt.Errorf("floating_window: %w", err)
		}
		dkeys, err := parseKeyList(dismissKeys, "dismiss_keys")
		if err != nil {
			return nil, fmt.Errorf("floating_window: %w", err)
		}
		mdCfg := markdown.DefaultConfig()
		mdCfg.HeaderPrefix = false
		md, err := markdown.NewWithConfig(string(text), mdCfg)
		if err != nil {
			return nil, fmt.Errorf("floating_window: parse: %w", err)
		}
		body := component.NewSpan(md, component.SpanConfig{
			PadHorizontal:    floatingWindowBodyPad,
			ContentAlignment: component.AlignmentHorizontallyCentered,
		})
		req := &request{
			kind:        reqFloatingWindow,
			text:        string(text),
			title:       string(title),
			align:       align,
			offset:      off,
			md:          md,
			body:        body,
			allowKeys:   keys,
			dismissKeys: dkeys,
		}
		if _, err := t.publishRequest(req); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}
}

func builtinMarkdown(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var text starlark.String
		if err := starlark.UnpackArgs("markdown", args, kwargs,
			"text", &text); err != nil {
			return nil, err
		}
		req := &request{kind: reqMarkdown, text: string(text)}
		if _, err := t.publishRequest(req); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}
}

func builtinWaitKey(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var key starlark.String
		if err := starlark.UnpackArgs("wait_key", args, kwargs,
			"key", &key); err != nil {
			return nil, err
		}
		req := &request{kind: reqWaitKey, waitKey: string(key)}
		if _, err := t.publishRequest(req); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}
}

func builtinWaitCommand(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			command starlark.String
			onError starlark.String
			title   starlark.String
		)
		if err := starlark.UnpackArgs("wait_command", args, kwargs,
			"command", &command,
			"on_error?", &onError,
			"title?", &title); err != nil {
			return nil, err
		}
		req := &request{
			kind:    reqWaitCommand,
			command: string(command),
			onError: string(onError),
			title:   string(title),
		}
		res, err := t.publishRequest(req)
		if err != nil {
			return nil, err
		}
		return newCommandResult(res.cmdName, res.cmdArgs), nil
	}
}

func builtinWaitShell(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			argList *starlark.List
			onError starlark.String
			title   starlark.String
		)
		if err := starlark.UnpackArgs("wait_shell", args, kwargs,
			"args", &argList,
			"on_error?", &onError,
			"title?", &title); err != nil {
			return nil, err
		}
		want, err := starlarkStringList(argList, "args")
		if err != nil {
			return nil, fmt.Errorf("wait_shell: %w", err)
		}
		if len(want) == 0 {
			return nil, errors.New("wait_shell: args must be non-empty")
		}
		req := &request{
			kind:      reqWaitShell,
			shellArgs: want,
			onError:   string(onError),
			title:     string(title),
		}
		res, err := t.publishRequest(req)
		if err != nil {
			return nil, err
		}
		return newCommandResult(res.cmdName, res.cmdArgs), nil
	}
}

func builtinConfirm(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var message starlark.String
		if err := starlark.UnpackArgs("confirm", args, kwargs,
			"message", &message); err != nil {
			return nil, err
		}
		req := &request{
			kind:    reqConfirm,
			message: string(message),
			options: []string{"Yes", "No"},
		}
		res, err := t.publishRequest(req)
		if err != nil {
			return nil, err
		}
		return starlark.Bool(res.confirmed), nil
	}
}

// builtinWaitEvent arms a step that resolves only when the host
// observes an editor event whose type name matches event. It never
// resolves from keystrokes, so every key reaches the IDE root while
// the hint stays up. The event name is not validated against a fixed
// set: an unknown name simply never matches.
func builtinWaitEvent(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			event   starlark.String
			text    starlark.String
			title   starlark.String
			onError starlark.String
		)
		if err := starlark.UnpackArgs("wait_event", args, kwargs,
			"event", &event,
			"text?", &text,
			"title?", &title,
			"on_error?", &onError); err != nil {
			return nil, err
		}
		req := &request{
			kind:    reqWaitEvent,
			event:   string(event),
			text:    string(text),
			title:   string(title),
			onError: string(onError),
		}
		if _, err := t.publishRequest(req); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}
}

func builtinChoice(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			message starlark.String
			options *starlark.List
		)
		if err := starlark.UnpackArgs("choice", args, kwargs,
			"message", &message,
			"options", &options); err != nil {
			return nil, err
		}
		opts, err := starlarkStringList(options, "options")
		if err != nil {
			return nil, fmt.Errorf("choice: %w", err)
		}
		if len(opts) == 0 {
			return nil, errors.New("choice: options must be non-empty")
		}
		req := &request{
			kind:    reqChoice,
			message: string(message),
			options: opts,
		}
		res, err := t.publishRequest(req)
		if err != nil {
			return nil, err
		}
		if !res.selected {
			return newChoiceResult(-1, "", false), nil
		}
		return newChoiceResult(res.selectedIdx, res.selectedValue, true), nil
	}
}

func builtinNotify(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			message starlark.String
			level   starlark.Int
		)
		if err := starlark.UnpackArgs("notify", args, kwargs,
			"message", &message,
			"level?", &level); err != nil {
			return nil, err
		}
		lvl, _ := level.Int64()
		t.runOnTUI(func() {
			if t.notifications != nil {
				_, _ = t.notifications.Notify(
					browserapi.NotificationLevel(lvl), "%s", string(message))
			}
		})
		return starlark.None, nil
	}
}

func builtinOpenFile(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var uri starlark.String
		if err := starlark.UnpackArgs("open_file", args, kwargs,
			"uri", &uri); err != nil {
			return nil, err
		}
		var openErr error
		t.runOnTUI(func() {
			if t.editor == nil {
				openErr = errors.New("no focused editor")
				return
			}
			parsed, err := workspaceapi.ParseURI(string(uri))
			if err != nil {
				parsed, err = workspaceapi.CurrentUserHostURI(string(uri))
				if err != nil {
					openErr = fmt.Errorf("parse uri %q: %w", string(uri), err)
					return
				}
			}
			if _, err := t.editor.Edit(context.Background(), parsed, nil,
				false, false); err != nil {
				openErr = err
			}
		})
		if openErr != nil {
			t.runOnTUI(func() {
				if t.notifications != nil {
					_, _ = t.notifications.Notify(browserapi.LevelError,
						"%s: %v", t.Title(), openErr)
				}
			})
		}
		return starlark.None, nil
	}
}

func builtinHighlightWindow(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var anchor starlark.String
		if err := starlark.UnpackArgs("highlight_window", args, kwargs,
			"anchor", &anchor); err != nil {
			return nil, err
		}
		t.runOnTUI(func() {
			if t.notifications == nil {
				return
			}
			_, _ = t.notifications.Notify(browserapi.LevelInfo,
				"tutorial: highlight_window(%q) — geometry deferred",
				string(anchor))
		})
		return starlark.None, nil
	}
}

// builtinExit raises errExitRequested so the entry function can
// terminate from any depth without explicit returns at every level.
func builtinExit() func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(*starlark.Thread, *starlark.Builtin,
		starlark.Tuple, []starlark.Tuple,
	) (starlark.Value, error) {
		return nil, errExitRequested
	}
}

// builtinCancelOnDismiss exits the tutorial when result.selected is
// False; otherwise it returns the result unchanged.
func builtinCancelOnDismiss() func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var result starlark.Value
		if err := starlark.UnpackArgs("cancel_on_dismiss", args, kwargs,
			"result", &result); err != nil {
			return nil, err
		}
		ha, ok := result.(starlark.HasAttrs)
		if !ok {
			return nil, fmt.Errorf("cancel_on_dismiss: argument must be a "+
				"choice_result or confirm result, got %s", result.Type())
		}
		sel, err := ha.Attr("selected")
		if err != nil || sel == nil {
			return nil, fmt.Errorf("cancel_on_dismiss: argument has no " +
				"`selected` attribute")
		}
		if b, ok := sel.(starlark.Bool); ok && !bool(b) {
			return nil, errExitRequested
		}
		return result, nil
	}
}

// runOnTUI runs fn synchronously: in production via the host's
// scheduleNextTick (so the work lands on the TUI loop and runOnTUI
// blocks until it completes), or inline when scheduleNextTick is nil.
func (t *Tutorial) runOnTUI(fn func()) {
	t.mu.Lock()
	sched := t.scheduleNextTick
	signal := t.firstSignal
	t.firstSignal = nil
	t.mu.Unlock()
	if signal != nil {
		signal()
	}
	if sched == nil {
		fn()
		return
	}
	done := make(chan struct{})
	if !sched(func() {
		fn()
		close(done)
	}) {
		fn()
		return
	}
	<-done
}

// parseFloatingAlignment maps a Starlark alignment keyword to a
// component.Alignment bitmask. An empty string yields zero, which
// floatingWindowAnchor treats as AlignmentCentered.
func parseFloatingAlignment(s string) (component.Alignment, error) {
	switch s {
	case "":
		return 0, nil
	case "center", "centered":
		return component.AlignmentCentered, nil
	case "top":
		return component.AlignmentTop | component.AlignmentHorizontallyCentered, nil
	case "bottom":
		return component.AlignmentBottom | component.AlignmentHorizontallyCentered, nil
	case "left":
		return component.AlignmentLeft | component.AlignmentVerticallyCentered, nil
	case "right":
		return component.AlignmentRight | component.AlignmentVerticallyCentered, nil
	case "top-left", "topleft":
		return component.AlignmentTop | component.AlignmentLeft, nil
	case "top-right", "topright":
		return component.AlignmentTop | component.AlignmentRight, nil
	case "bottom-left", "bottomleft":
		return component.AlignmentBottom | component.AlignmentLeft, nil
	case "bottom-right", "bottomright":
		return component.AlignmentBottom | component.AlignmentRight, nil
	}
	return 0, fmt.Errorf("alignment %q: want one of "+
		`"center", "top", "bottom", "left", "right", `+
		`"top-left", "top-right", "bottom-left", "bottom-right"`, s)
}

// parseFloatingOffset accepts None or a 2-element tuple/list of ints
// and returns a term.Coordinates. An absent value yields the zero
// coordinate.
func parseFloatingOffset(v starlark.Value) (term.Coordinates, error) {
	if v == nil || v == starlark.None {
		return term.Coordinates{}, nil
	}
	iter, ok := v.(starlark.Indexable)
	if !ok {
		return term.Coordinates{}, fmt.Errorf(
			"offset: want a (x, y) tuple, got %s", v.Type())
	}
	if iter.Len() != 2 {
		return term.Coordinates{}, fmt.Errorf(
			"offset: want a 2-element (x, y) tuple, got %d elements",
			iter.Len())
	}
	x, err := starlark.AsInt32(iter.Index(0))
	if err != nil {
		return term.Coordinates{}, fmt.Errorf("offset.x: %w", err)
	}
	y, err := starlark.AsInt32(iter.Index(1))
	if err != nil {
		return term.Coordinates{}, fmt.Errorf("offset.y: %w", err)
	}
	return term.Coordinates{X: x, Y: y}, nil
}

func starlarkStringList(list *starlark.List, key string) ([]string, error) {
	if list == nil {
		return nil, fmt.Errorf("missing required param %q", key)
	}
	out := make([]string, 0, list.Len())
	iter := list.Iterate()
	defer iter.Done()
	var item starlark.Value
	for iter.Next(&item) {
		s, ok := item.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("param %q[]: want string, got %s",
				key, item.Type())
		}
		out = append(out, string(s))
	}
	return out, nil
}

// parseKeyList converts a Starlark list of key combo strings (e.g.
// "<meta-1>", "<c-s-tab>", "q") into KeyCombs. A nil list returns
// nil so callers can treat absence as "no entries" without an extra
// check. The kwarg name is used in error messages so authors can
// spot which kwarg owns a bad string.
func parseKeyList(list *starlark.List, kwarg string) ([]term.KeyComb, error) {
	if list == nil {
		return nil, nil
	}
	out := make([]term.KeyComb, 0, list.Len())
	iter := list.Iterate()
	defer iter.Done()
	var item starlark.Value
	for iter.Next(&item) {
		s, ok := item.(starlark.String)
		if !ok {
			return nil, fmt.Errorf(
				"%s[]: want string, got %s", kwarg, item.Type())
		}
		k, err := term.ParseKey(string(s))
		if err != nil {
			return nil, fmt.Errorf("%s[%q]: %w", kwarg, string(s), err)
		}
		out = append(out, k)
	}
	return out, nil
}

func builtinCommandKey(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		_ starlark.Tuple, _ []starlark.Tuple,
	) (starlark.Value, error) {
		return starlark.String(t.commandKeyDisplay), nil
	}
}

// builtinEditorMode implements editor_mode(): it returns the user's
// resolved editor mode, "modal" or "modeless" (exo is resolved to its
// fallback by the host before the tutorial runs).
func builtinEditorMode(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin,
		_ starlark.Tuple, _ []starlark.Tuple,
	) (starlark.Value, error) {
		return starlark.String(t.editorMode), nil
	}
}

// builtinKeyFor implements key_for(command, *args): it returns the
// user's configured key spec bound to command (with optional args), or
// "" when unbound or when no lookup func is wired.
func builtinKeyFor(t *Tutorial) func(*starlark.Thread, *starlark.Builtin,
	starlark.Tuple, []starlark.Tuple,
) (starlark.Value, error) {
	return func(_ *starlark.Thread, b *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		if len(kwargs) != 0 {
			return nil, fmt.Errorf("%s: unexpected keyword arguments",
				b.Name())
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("%s: missing command argument", b.Name())
		}
		cmd, ok := starlark.AsString(args[0])
		if !ok {
			return nil, fmt.Errorf("%s: command must be a string, got %s",
				b.Name(), args[0].Type())
		}
		var cmdArgs []string
		for _, a := range args[1:] {
			s, ok := starlark.AsString(a)
			if !ok {
				return nil, fmt.Errorf("%s: args must be strings, got %s",
					b.Name(), a.Type())
			}
			cmdArgs = append(cmdArgs, s)
		}
		if t.keyForCommand == nil {
			return starlark.String(""), nil
		}
		return starlark.String(t.keyForCommand(cmd, cmdArgs)), nil
	}
}
