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

package text

import (
	"context"
	"errors"

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// SubscribeFoldCommands returns a slice of fold list related commands
// that can be registered for the returned CommandHandler.
func SubscribeFoldCommands(
	file workspaceapi.URI, registry FileCommandRegistry,
	cursor *Cursor, Handler Handler,
) (Handler, error) {
	ret := foldCommandHandler{
		file:     file,
		cursor:   cursor,
		registry: registry,
		Handler:  Handler,
	}
	var retErr error
	for _, cmd := range foldCommands {
		if err := registry.SubscribeCommandForFile(file, cmd, ret); err != nil {
			retErr = multierror.Append(retErr, err)
		}
	}
	if retErr != nil {
		return nil, retErr
	}
	return ret, nil
}

const (
	commandExpandFold       = "foldexpand"
	commandCollapseFold     = "foldcollapse"
	commandToggleFold       = "foldtoggle"
	commandExpandAllFolds   = "foldexpandall"
	commandCollapseAllFolds = "foldcollapseall"
	commandToggleAllFolds   = "foldtoggleall"
)

var foldCommands = []textapi.CommandManual{
	{
		Name:     commandExpandFold,
		Summary:  "Expand the collapsed fold surrounding the current cursor position.",
		Synopsis: "",
	},
	{
		Name:     commandCollapseFold,
		Summary:  "Collapses the expanded fold surrounding the current cursor position.",
		Synopsis: "",
	},
	{
		Name:     commandToggleFold,
		Summary:  "Collapses or expands the fold surrounding the current cursor position.",
		Synopsis: "",
	},
	{
		Name:     commandExpandAllFolds,
		Summary:  "Expand all folds in the file.",
		Synopsis: "",
	},
	{
		Name:     commandCollapseAllFolds,
		Summary:  "Collapses all folds in the file.",
		Synopsis: "",
	},
	{
		Name:     commandToggleAllFolds,
		Summary:  "Collapses or expands all folds in the file.",
		Synopsis: "",
	},
}

type foldCommandHandler struct {
	Handler
	cursor   *Cursor
	file     workspaceapi.URI
	registry FileCommandRegistry
}

func (u foldCommandHandler) Close() (ret error) {
	ret = u.Handler.Close()
	for _, cmd := range foldCommands {
		err := u.registry.UnsubscribeCommandForFile(u.file, cmd.Name)
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (u foldCommandHandler) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) (err error) {
	var ok, all bool
	switch cmd.Name {
	case commandCollapseFold:
		ok = u.cursor.CollapseFold(ctx)
	case commandExpandFold:
		ok = u.cursor.ExpandFold(ctx)
	case commandToggleFold:
		ok = u.cursor.ToggleFold(ctx)
	case commandExpandAllFolds:
		ok = u.cursor.ExpandAllFolds(ctx)
		all = true
	case commandCollapseAllFolds:
		ok = u.cursor.CollapseAllFolds(ctx)
		all = true
	case commandToggleAllFolds:
		ok = u.cursor.ToggleAllFolds(ctx)
		all = true
	default:
		err = errors.New("extraneous command")
	}
	if !ok {
		if all {
			return errors.New("no folds in this file")
		} else {
			return errors.New("no folds at the current position")
		}
	}
	return
}

func (u foldCommandHandler) Complete(ctx context.Context, cmd textapi.Command) (
	ret iterator.Iterator[string], _ string, err error,
) {
	ret = iterator.Empty[string]()
	return
}
