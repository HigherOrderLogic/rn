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

package vctrlcmd

import (
	"context"
	"errors"

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/clipboard"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/text"
)

// SubscribeGitCommands returns a slice of git list related commands
// that can be registered for the returned CommandHandler.
func SubscribeGitCommands(
	file workspaceapi.URI, registry text.FileCommandRegistry, ed text.Handler,
	svc vctrl.Service, clip clipboard.Register, noti browserapi.Notifications,
) (text.Handler, error) {
	ret := gitCommandHandler{
		file:     file,
		registry: registry,
		Handler:  ed,
		webLink:  newCopyRemoteURL(svc, clip, noti),
	}
	var retErr error
	for _, cmd := range gitCommands {
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
	commandGitLink = "gitlink"
)

var gitCommands = []textapi.CommandManual{
	{
		Name:     commandGitLink,
		Summary:  "Copies to clipboard the web permalink of the line at the cursor.",
		Synopsis: "[remote]",
	},
}

type gitCommandHandler struct {
	text.Handler
	file     workspaceapi.URI
	registry text.FileCommandRegistry
	webLink  text.CommandHandler
}

func (u gitCommandHandler) HandleCommand(
	ctx context.Context, cmd textapi.Command,
) (err error) {
	switch cmd.Name {
	case commandGitLink:
		err = u.webLink.HandleCommand(ctx, cmd)
	default:
		err = errors.New("extraneous command")
	}
	return
}

func (u gitCommandHandler) Complete(ctx context.Context, cmd textapi.Command) (
	ret iterator.Iterator[string], arg0 string, err error,
) {
	switch cmd.Name {
	case commandGitLink:
		ret, arg0, err = u.webLink.Complete(ctx, cmd)
	default:
		err = errors.New("extraneous command")
	}
	return
}

func (g gitCommandHandler) Close() (ret error) {
	ret = g.Handler.Close()
	for _, cmd := range gitCommands {
		err := g.registry.UnsubscribeCommandForFile(g.file, cmd.Name)
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}
