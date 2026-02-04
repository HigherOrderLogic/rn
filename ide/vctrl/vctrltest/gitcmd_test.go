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

package vctrltest

import (
	"context"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/ide/vctrl"
	"unstable.build/go-tui/workspace"
)

func TestCmdDiff(t *testing.T) {
	testGitDiff(t, setupGitCmdService)
}

func TestCmdGitCurrentCommit(t *testing.T) {
	testGitCurrentCommit(t, setupGitCmdService)
}

func TestCmdGitShortRef(t *testing.T) {
	testGitShortRef(t, setupGitCmdService)
}

func TestCmdGitRemoteURL(t *testing.T) {
	testGitRemoteURL(t, setupGitCmdService)
}

func TestCmdRelPath(t *testing.T) {
	testRelPath(t, setupGitCmdService)
}

func setupGitCmdService(t *testing.T, cwd workspaceapi.URI) vctrl.Service {
	scheme, err := workspace.NewFileScheme(
		context.Background(), config.NopConfig(), cwd,
	)
	require.NoError(t, err)

	// gitCliExecutor runs git commands on real repos extracted from tarballs
	gitCliExecutor := new(gitTestExecutor)
	gitCliExecutor.schemeExecutor = scheme

	return vctrl.NewGitCommand(cwd, gitCliExecutor, scheme)
}

type gitTestExecutor struct {
	schemeExecutor schemeapi.Executor
}

func (e *gitTestExecutor) Start(ctx context.Context, cmd workspaceapi.Cmd) (workspaceapi.Pid, error) {
	return e.schemeExecutor.StartCommand(ctx, cmd)
}

func (e *gitTestExecutor) Signal(pid workspaceapi.Pid, signal syscall.Signal) error {
	return nil
}

func (e *gitTestExecutor) Close() error {
	return nil
}
