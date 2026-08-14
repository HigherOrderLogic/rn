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

// Package upgradeshell exposes the `upgrade` REPL command on the rune
// (IDE) side. It checks the release manifest, prompts the user to
// confirm, and installs the new release in place.
//
// Download and install progress is reported through the
// repl.ProgressWriter handed to it by the host shell, and results are
// returned as markdown, so it integrates with the REPL the same way
// the `pkg` shell does.
package upgradeshell

import (
	"context"
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/handler/repl"
	"github.com/unstablebuild/rune-go-sdk/iterator"
	"unstable.build/go-tui/component/markdown"
	"unstable.build/go-tui/ide/ideupgrade"
)

// CommandName is the top-level REPL command exposed by this shell.
const CommandName = "upgrade"

var commandManual = textapi.CommandManual{
	Name: CommandName,
	Summary: "Check for a new Rune release and install it in place. " +
		"When a release is available you are prompted to confirm, " +
		"postpone the reminder, or skip the version.",
}

// Manual returns the REPL command manual.
func Manual() textapi.CommandManual { return commandManual }

// Config configures a Handler.
type Config struct {
	// Manager performs the manifest check, the confirmation prompt
	// and the in-place upgrade.
	Manager *ideupgrade.Manager
}

// Handler implements the `upgrade` command.
type Handler struct {
	mgr *ideupgrade.Manager
}

var _ textapi.REPLHandler = (*Handler)(nil)

// New returns a Handler configured with cfg. It panics if Manager is
// nil — the rune-side wiring only registers the command once the
// manager has been constructed, so a missing one is a programming
// error.
func New(cfg Config) *Handler {
	if cfg.Manager == nil {
		panic("upgradeshell: Config.Manager must not be nil")
	}
	return &Handler{mgr: cfg.Manager}
}

// HandleCommand satisfies repl.CommandHandler. It runs off the IDE
// event loop, which is what lets it block on both the network fetch
// and the confirmation prompt.
func (h *Handler) HandleCommand(
	ctx context.Context, cmd repl.Command, pw repl.ProgressWriter,
) (iterator.Iterator[component.Responsive], error) {
	out, err := h.upgrade(ctx, cmd, pw)
	if err != nil {
		return nil, err
	}
	return markdownOutput(out), nil
}

// upgrade performs the command and returns its markdown output.
func (h *Handler) upgrade(
	ctx context.Context, cmd repl.Command, pw repl.ProgressWriter,
) (string, error) {
	if len(cmd.Args) > 0 && cmd.Args[0] == "help" {
		return usageMarkdown(), nil
	}

	pw.Progress(0, 1, "checking for updates")
	manifest, available, err := h.mgr.Check(ctx)
	if err != nil {
		return "", err
	}
	if !available {
		return fmt.Sprintf(
			"Rune is up to date (**%s**)", h.mgr.CurrentVersion()), nil
	}

	choice, err := h.mgr.PromptChoice(ctx, manifest)
	if err != nil {
		return "", err
	}
	switch choice {
	case ideupgrade.ChoiceUpgradeNow:
		if err := h.mgr.Upgrade(ctx, manifest, pw); err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"Upgraded to **%s** — restart Rune to apply", manifest.Version), nil
	case ideupgrade.ChoiceRemindLater:
		return fmt.Sprintf(
			"Rune **%s** is available. Reminder postponed.", manifest.Version), nil
	case ideupgrade.ChoiceSkipVersion:
		return fmt.Sprintf("Skipping Rune **%s**.", manifest.Version), nil
	default:
		return fmt.Sprintf("Rune **%s** is available.", manifest.Version), nil
	}
}

// Complete satisfies repl.CommandHandler. `upgrade` takes no
// arguments, so there is nothing to complete.
func (h *Handler) Complete(
	context.Context, string, []string,
) (iterator.Iterator[string], error) {
	return iterator.FromSlice[string](nil), nil
}

// Help satisfies textapi.REPLHandler.
func (h *Handler) Help(
	context.Context, []string,
) (iterator.Iterator[component.Responsive], error) {
	return markdownOutput(usageMarkdown()), nil
}

func usageMarkdown() string {
	return fmt.Sprintf("## `%s`\n\n%s\n",
		commandManual.Name, commandManual.Summary)
}

// markdownOutput wraps a markdown string into a single-shot iterator
// suitable for returning from HandleCommand. Falls back to a plain
// responsive string when the markdown parser rejects the content.
func markdownOutput(content string) iterator.Iterator[component.Responsive] {
	md, err := markdown.New(content)
	if err != nil {
		r := component.NewResponsiveString(content, component.StringResponsiveConfig{})
		return iterator.FromSlice([]component.Responsive{r})
	}
	return iterator.FromSlice([]component.Responsive{md})
}
