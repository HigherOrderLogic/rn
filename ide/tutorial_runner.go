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
	"fmt"
	"sort"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"

	"unstable.build/go-tui/ide/idetutorial"
)

type tutorialRunner struct {
	tui.Handler
	tutorials     map[string]idetutorial.Tutorial
	overlay       *idetutorial.Handler
	activeName    string
	interrupter   term.Interrupter
	width, height int
}

var _ commandObserver = (*tutorialRunner)(nil)

func (r *tutorialRunner) init(
	root tui.Handler,
	tutorials map[string]idetutorial.Tutorial,
	interrupter term.Interrupter,
) {
	r.Handler = root
	r.tutorials = tutorials
	r.interrupter = interrupter
}

func (r *tutorialRunner) setActive(name string, t idetutorial.Tutorial) {
	r.overlay = idetutorial.New(r.Handler, t, r.interrupter)
	r.activeName = name
	if r.width > 0 && r.height > 0 {
		r.overlay.Resize(r.width, r.height)
	}
	r.overlay.Reset()
}

func (r *tutorialRunner) clearActive() {
	if r.overlay != nil {
		_ = r.overlay.Close()
	}
	r.overlay = nil
	r.activeName = ""
}

func (r *tutorialRunner) Resize(width, height int) {
	r.width, r.height = width, height
	if r.overlay != nil {
		r.overlay.Resize(width, height)
		return
	}
	r.Handler.Resize(width, height)
}

func (r *tutorialRunner) Draw(w term.Writer) {
	if r.overlay != nil {
		r.overlay.Draw(w)
		return
	}
	r.Handler.Draw(w)
}

func (r *tutorialRunner) Handle(ev term.Event) (bool, bool) {
	if r.overlay == nil {
		return r.Handler.Handle(ev)
	}
	exit, handled := r.overlay.Handle(ev)
	if exit {
		r.clearActive()
	}
	return false, handled
}

func (r *tutorialRunner) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	if r.overlay != nil {
		return r.overlay.Cursor()
	}
	return r.Handler.Cursor()
}

func (r *tutorialRunner) Selection() (string, bool) {
	if r.overlay != nil {
		return r.overlay.Selection()
	}
	return r.Handler.Selection()
}

func (r *tutorialRunner) Close() error {
	r.clearActive()
	return nil
}

func (r *tutorialRunner) setDefaultAttributes(defAttr term.Attributes) {
	for _, tut := range r.tutorials {
		tut.SetDefaultAttributes(defAttr)
	}
	if r.overlay != nil {
		r.overlay.SetDefaultAttributes(defAttr)
	}
}

func (r *tutorialRunner) observeCommand(
	typed, resolved string, args []string, err error,
) {
	if r.overlay == nil {
		return
	}
	exit := r.overlay.ObserveCommand(typed, resolved, args, err)
	if exit {
		r.clearActive()
	}
}

func (r *tutorialRunner) observeEvent(eventType, uri string) {
	if r.overlay == nil {
		return
	}
	if r.overlay.ObserveEvent(eventType, uri) {
		r.clearActive()
	}
}

func (r *tutorialRunner) HandleCommand(_ context.Context, cmd textapi.Command) error {
	if len(cmd.Args) == 0 {
		return errors.New(
			"usage: tutorial <start|stop> [<name>]")
	}
	switch cmd.Args[0] {
	case "start":
		if len(cmd.Args) != 2 {
			return errors.New("usage: tutorial start <name>")
		}
		name := cmd.Args[1]
		tut, ok := r.tutorials[name]
		if !ok {
			return fmt.Errorf("unknown tutorial %q", name)
		}
		r.setActive(name, tut)
		return nil
	case "stop":
		if len(cmd.Args) > 1 {
			return fmt.Errorf("tutorial stop: takes no arguments, got %d",
				len(cmd.Args)-1)
		}
		if r.overlay == nil {
			return errors.New("no tutorial is running")
		}
		r.clearActive()
		return nil
	}
	return fmt.Errorf("unknown subcommand %q: "+
		"want one of start, stop", cmd.Args[0])
}

func (r *tutorialRunner) Complete(_ context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	// First argument: list subcommands.
	if len(cmd.Args) <= 1 {
		return iterator.FromSlice([]string{
			"start", "stop",
		}), "", nil
	}
	// Second argument after `start`: tutorial names.
	if len(cmd.Args) == 2 {
		switch cmd.Args[0] {
		case "start":
			names := make([]string, 0, len(r.tutorials))
			for n := range r.tutorials {
				names = append(names, n)
			}
			sort.Strings(names)
			return iterator.FromSlice(names), "", nil
		}
	}
	return iterator.Empty[string](), "", nil
}
