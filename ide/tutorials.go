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
	"errors"
	"fmt"
	"os"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler"
	"github.com/unstablebuild/rune-go-sdk/term"

	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/handler/command"
	"unstable.build/go-tui/ide/idetutorial"
	"unstable.build/go-tui/ide/idetutorial/starlarktutorial"
)

func buildTutorials(i *IDE) map[string]idetutorial.Tutorial {
	files := i.ideConfig.tutorialFiles()
	embedded := i.options.starlarkTutorials
	i.tutorialsConfig = newTutorialsConfig(i)
	if len(files) == 0 && len(embedded) == 0 {
		return nil
	}
	tutorials := make(map[string]idetutorial.Tutorial,
		len(files)+len(embedded))
	for name, src := range embedded {
		t, err := i.tutorialsConfig.build(name, src)
		if err != nil {
			i.ideConfig.errors["tutorials."+name] = err
			continue
		}
		tutorials[name] = t
	}
	for name, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			i.ideConfig.errors["tutorials."+name] = fmt.Errorf(
				"read %q: %w", path, err)
			continue
		}
		t, err := i.tutorialsConfig.build(name, string(src))
		if err != nil {
			i.ideConfig.errors["tutorials."+name] = err
			continue
		}
		tutorials[name] = t
	}
	return tutorials
}

// onTutorialsInstalled is invoked by the package manager's post-merge hook
// (off the event loop) when a freshly-installed package added tutorials under
// the top-level `tutorials:` config key. It builds each not-yet-registered
// tutorial, then schedules their registration into the live runner onto the
// event loop. Every built tutorial is registered, but only the first one
// prompts "run it now?": prompting per tutorial would stack overlapping
// floating windows. The first return reports whether at least one tutorial was
// built so the merge result records a live-apply; the error aggregates every
// read/build failure so idepkg can notify in one place.
func (i *IDE) onTutorialsInstalled(names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	cfg, err := i.workspaceHandler.reloadConfig()
	if err != nil {
		return false, fmt.Errorf("reload config for installed tutorials: %w", err)
	}
	files := cfg.tutorialFiles()

	type built struct {
		name string
		t    idetutorial.Tutorial
	}
	var (
		ready []built
		errs  error
	)
	for _, name := range names {
		if i.tutorial.has(name) {
			continue
		}
		path, ok := files[name]
		if !ok {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("read tutorial %q: %w", name, err))
			continue
		}
		t, err := i.tutorialsConfig.build(name, string(src))
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("build tutorial %q: %w", name, err))
			continue
		}
		ready = append(ready, built{name: name, t: t})
	}
	if len(ready) == 0 {
		return false, errs
	}

	i.options.scheduleFn(func() {
		prompted := false
		for _, b := range ready {
			if !i.tutorial.register(b.name, b.t) {
				i.notifyTutorialNotRegistered(b.name)
				continue
			}
			if !prompted {
				i.promptRunTutorial(b.name)
				prompted = true
			}
		}
	})
	return true, errs
}

// notifyTutorialNotRegistered tells the user a freshly-installed tutorial could
// not be made live because one is already registered under the same name, so a
// restart is needed to pick up the new definition. It must run on the event
// loop.
func (i *IDE) notifyTutorialNotRegistered(name string) {
	_, _ = i.workspaceHandler.notifications.current().Notify(
		browserapi.LevelWarn,
		"a tutorial named %q is already loaded; restart to use the new one.",
		name,
	)
}

// promptRunTutorial asks the user whether to run a freshly-installed tutorial.
// It must run on the event loop. On Yes it dispatches `tutorial start <name>`,
// mirroring maybeStartTutorial; on No or close it does nothing.
func (i *IDE) promptRunTutorial(name string) {
	ex := i.workspaceHandler.focusEx()
	if ex == nil {
		return
	}
	message := fmt.Sprintf("Do you want to run the **%s** tutorial now?", name)
	var promptWindow browser.Window
	promptWindow = ex.comp.Prompt(
		message,
		[]string{yesOpt, noOpt},
		yesNoKeyCombs,
		handler.FuncPromptHandler(func(_ int, option string) {
			if promptWindow != nil {
				_ = promptWindow.Close()
			}
			if option != yesOpt {
				return
			}
			i.workspaceHandler.focusEx().Dispatch("tutorial", "start", name)
		}, func() error { return nil }),
	)
}

// tutorialsConfig captures the host services a tutorial is built against so a
// single tutorial can be constructed without recomputing them. It is built
// once during IDE init and reused when packages install new tutorials.
type tutorialsConfig struct {
	partition        storageapi.Service
	br               currentBrowser
	ed               currentEditor
	parser           currentParser
	notifications    browserapi.Notifications
	defaultAttr      term.Attributes
	frameCharSet     component.FrameCharSet
	promptConfig     browser.PromptConfig
	scheduleNextTick func(func()) bool
	commandKey       term.KeyComb
	editorMode       string
	keyForCommand    func(cmd string, args []string) string
	manualLookup     starlarktutorial.CommandManualLookup
}

func newTutorialsConfig(i *IDE) tutorialsConfig {
	partition := i.storage
	if p, err := i.storage.Partition("idetutorial"); err == nil {
		partition = p
	}
	rawKeyFor := i.ideConfig.commandKeyBindingLookup()
	return tutorialsConfig{
		partition:        partition,
		br:               currentBrowser{root: i.workspaceHandler},
		ed:               currentEditor{root: i.workspaceHandler},
		parser:           currentParser{root: i.workspaceHandler},
		notifications:    i.workspaceHandler.notifications.current(),
		defaultAttr:      i.ideConfig.defaultAttr(),
		frameCharSet:     i.ideConfig.windowFrameCharset(),
		promptConfig:     i.ideConfig.promptConfig(),
		scheduleNextTick: i.options.scheduleFn,
		commandKey:       i.ideConfig.commandKey(),
		editorMode:       i.ideConfig.pkgEditorMode(),
		keyForCommand: func(cmd string, args []string) string {
			return starlarktutorial.PrettyKeySpec(rawKeyFor(cmd, args))
		},
		manualLookup: buildTutorialCommandManualLookup(i.workspaceHandler),
	}
}

// build constructs a single tutorial from its starlark source.
func (c tutorialsConfig) build(
	name, src string,
) (idetutorial.Tutorial, error) {
	return starlarktutorial.New(
		name, src,
		c.br, c.ed, c.notifications, c.parser,
		c.defaultAttr, c.frameCharSet, c.promptConfig,
		c.scheduleNextTick, c.partition, c.commandKey,
		c.editorMode, c.keyForCommand, c.manualLookup,
	)
}

// buildTutorialCommandManualLookup returns a closure that resolves
// a command name to its registered command.Manual. It consults the
// focused editor's subscribed commands first and then the workspace
// handler's alias expander so authors writing `wait_command("e")`
// (an alias for `edit`) see the alias entry in the hint window.
func buildTutorialCommandManualLookup(
	root *workspaceManagerHandler,
) starlarktutorial.CommandManualLookup {
	if root == nil {
		return nil
	}
	return func(name string) (command.Manual, bool) {
		ex := root.focusEx()
		if ex == nil {
			return command.Manual{}, false
		}
		for _, man := range ex.comp.Commands() {
			if man.Name == name {
				return man, true
			}
		}
		if ex.aliasExpander != nil {
			for _, man := range ex.aliasExpander.Aliases() {
				if man.Name == name {
					return man, true
				}
			}
		}
		return command.Manual{}, false
	}
}
