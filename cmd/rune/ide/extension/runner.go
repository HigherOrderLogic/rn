// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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

package extension

import (
	"context"
	"fmt"
	"sync"
	"syscall"

	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/logging/trace"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/storageapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/ide"
)

// NewRunner returns an extension runner for Rune workspace extensions.
func NewRunner(
	ctx context.Context, locker sync.Locker, dataDir, arg0 string,
) (*Extensions, error) {
	extensionOpts := []extensionv2.Option{
		extensionv2.WithPackageName(debug.Package),
		extensionv2.WithPackageVersion(debug.Tag),
		extensionv2.WithSocketEnv("RUNE_SOCKET"),
		extensionv2.WithDataDirEnv("RUNE_DATADIR"),
		extensionv2.WithAuthCertEnv("RUNE_CERT"),
		extensionv2.WithAuthTokenEnv("RUNE_TOKEN"),
	}
	runner, err := extensionv2.NewRunner(ctx, locker,
		dataDir, extensionOpts...)
	if err != nil {
		return nil, fmt.Errorf("new extension runner: %v", err)
	}

	return &Extensions{
		runner: runner,
	}, nil
}

// Extensions is a ide.Extensions implementation.
type Extensions struct {
	runner ide.ExtensionsRunner
}

// WorkspaceExtensionsRunner satisfies ide.ExtensionsRunner.
func (p *Extensions) WorkspaceExtensionsRunner(
	uri workspaceapi.URI,
	res map[extensionapi.Permission]extension.ResourceRegistrar,
	dataDir string, notifications browser.Notifications,
	exec schemeapi.Executor,
	grantor extension.Grantor,
	promptOpener ide.ExtensionPromptOpener, storage storageapi.Service,
	scheduleNextTick func(func()) bool,
) (extension.Runner, error) {
	other, err := p.runner.WorkspaceExtensionsRunner(uri, res, dataDir, notifications, exec,
		grantor, promptOpener, storage, scheduleNextTick)
	if err != nil {
		return nil, err
	}
	return extensionsRunner{other: other}, nil
}

type extensionsRunner struct {
	other extension.Runner
}

func (p extensionsRunner) Run(extensionID, path string, config config.Config) (ret error) {
	const runCallType = "runExtension"
	traceID := trace.New()

	fields := []logging.Field{
		{Key: "extensionID", Value: extensionID},
		{Key: "path", Value: path},
	}

	attemptAt := logging.LogAttempt(traceID, runCallType, fields...)
	defer func() {
		logging.LogResult(ret, attemptAt, traceID, runCallType, fields...)
	}()

	return p.other.Run(extensionID, path, config)
}

func (p extensionsRunner) StartCommand(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	return p.other.(schemeapi.Executor).StartCommand(ctx, cmd)
}

func (p extensionsRunner) Signal(pid workspaceapi.Pid, sig syscall.Signal) error {
	return p.other.(schemeapi.Executor).Signal(pid, sig)
}

func (p extensionsRunner) Close() (ret error) {
	return p.other.Close()
}
