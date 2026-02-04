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

package text

import (
	"context"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// CommandHandler wraps the basic methods HandleCommand and Complete.
type CommandHandler interface {
	// HandleCommand is called when user issued a command previously registered via SubscribeCommand.
	HandleCommand(context.Context, textapi.Command) (err error)

	// Complete takes command args and returns a list of expanded options for them.
	// It also returns a expanded version of the last arg, or an empty string
	// if the last arg could/should not be automatically expanded.
	Complete(ctx context.Context, cmd textapi.Command) (
		iterator.Iterator[string], string, error,
	)
}

// WorkspaceCommandRegistry abstracts the ability to subscribe to commands
// for a particular workspace.
type WorkspaceCommandRegistry interface {
	SubscribeCommandForWorkspace(
		workspace workspaceapi.URI, cmd textapi.CommandManual, handler CommandHandler) error
	UnsubscribeCommandForWorkspace(workspace workspaceapi.URI, name string) error
}

// FileCommandRegistry abstracts the ability to subscribe to commands
// for a particular file.
type FileCommandRegistry interface {
	SubscribeCommandForFile(
		file workspaceapi.URI, cmd textapi.CommandManual, handler CommandHandler) error
	UnsubscribeCommandForFile(file workspaceapi.URI, name string) error
}

// FuncCommandHandler returns an CommandHandler that calls fn
// every time HandleCommand is invoked. If completeFn is not nil,
// then it is called when Complete is invoked.
func FuncCommandHandler(
	fn func(context.Context, textapi.Command) error,
	completeFn func(context.Context, textapi.Command) (
		iterator.Iterator[string], string, error,
	),
) CommandHandler {
	return fnCommandHandler{
		cb:         fn,
		completeFn: completeFn,
	}
}

type fnCommandHandler struct {
	cb         func(context.Context, textapi.Command) error
	completeFn func(context.Context, textapi.Command) (
		iterator.Iterator[string], string, error,
	)
}

func (f fnCommandHandler) HandleCommand(ctx context.Context, c textapi.Command) error {
	return f.cb(ctx, c)
}

func (f fnCommandHandler) Complete(ctx context.Context, cmd textapi.Command) (
	iterator.Iterator[string], string, error,
) {
	if f.completeFn != nil {
		return f.completeFn(ctx, cmd)
	}
	return iterator.FromSlice[string](nil), "", nil
}
