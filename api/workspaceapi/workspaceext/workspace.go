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

package workspaceext

import (
	"context"
	"os"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi/workspacerpc"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
)

func dial(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	*workspacerpc.Client, error,
) {
	conn, err := broker.DialChannel(ctx, grant.Token,
		os.Args[0], "workspace", string(grant.Permission))
	if err != nil {
		return nil, err
	}
	c := workspacerpc.NewClient(grant.Context, conn)
	return c, nil
}

// FileSystem acquires the workspace's file-system with the given token.
func FileSystem(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	workspaceapi.FileSystem, error,
) {
	return dial(ctx, grant, broker)
}

// Executor acquires the workspace's processes executor with the given token.
func Executor(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	workspaceapi.Executor, error,
) {
	return dial(ctx, grant, broker)
}

// Terminal acquires the workspace's pseudo-terminal with the given token.
func Terminal(ctx context.Context, grant extension.Grant, broker rpc.MuxBroker) (
	workspaceapi.Terminal, error,
) {
	return dial(ctx, grant, broker)
}
