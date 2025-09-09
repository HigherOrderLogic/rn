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

package extension

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/unstablebuild/blue/bluectx"
	"unstable.build/go-tui/api/extensionapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacerpc"
)

type workspaceResourceServer struct {
	b workspace.Workspace
	p extensionapi.Permission
}

func newWorkspaceResourceServer(
	b workspace.Workspace, p extensionapi.Permission,
) *workspaceResourceServer {
	ret := new(workspaceResourceServer)
	ret.p = p
	ret.b = b
	return ret
}

func (s *workspaceResourceServer) Register(
	registrar rpc.ServiceRegistrar, lock sync.Locker,
) (io.Closer, error) {
	// create a closer able to close all processes created by grantee
	// without closing workspace.Workspace, which we cannot assume about
	// its lifecycle
	w := &trackingWorkspace{
		Workspace: s.b,
	}
	w.ctx, w.cancelCtx = context.WithCancel(context.Background())
	server := workspacerpc.NewServer(w, lock)
	switch s.p {
	case extensionapi.PermissionFileSystem:
		if !rpc.IsRegistered(registrar, workspacerpc.Files_ServiceDesc) {
			workspacerpc.RegisterFilesServer(registrar, server)
		}
		workspacerpc.RegisterSchemeServer(registrar, server)
	case extensionapi.PermissionTerminal:
		if !rpc.IsRegistered(registrar, workspacerpc.Files_ServiceDesc) {
			workspacerpc.RegisterFilesServer(registrar, server)
		}
		workspacerpc.RegisterTerminalServer(registrar, server)
	case extensionapi.PermissionExecute:
		workspacerpc.RegisterExecutorServer(registrar, server)
	}
	w.server = server
	return w, nil
}

// WorkspaceResources returns a map of Permission to a ResourceServer
// capable of serving each of the b Workspace's resources.
func WorkspaceResources(b workspace.Workspace) map[extensionapi.Permission]ResourceRegistrar {
	return map[extensionapi.Permission]ResourceRegistrar{
		extensionapi.PermissionFileSystem: newWorkspaceResourceServer(
			b, extensionapi.PermissionFileSystem),
		extensionapi.PermissionTerminal: newWorkspaceResourceServer(
			b, extensionapi.PermissionTerminal),
		extensionapi.PermissionExecute: newWorkspaceResourceServer(
			b, extensionapi.PermissionExecute),
	}
}

type trackingWorkspace struct {
	workspace.Workspace
	ctx       context.Context
	cancelCtx func()
	server    *workspacerpc.Server
}

func (w *trackingWorkspace) Command(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	var cancelFn func()
	ctx, cancelFn = context.WithCancel(ctx)
	cmd.Watcher = newWrapWatcher(cmd.Watcher, cancelFn)
	ctx = bluectx.First(w.ctx, ctx)
	return w.Workspace.StartCommand(ctx, cmd)
}

func (w *trackingWorkspace) NewPty(ctx context.Context) (
	workspaceapi.Pty, error,
) {
	// FIXME this temporarily leaks a goroutine, once session is closed
	// but ctx or s.ctx have not been canceled yet (workspace is still active,
	// or extension is still active).
	// Since pty capability might be removed from a scheme, once sysprocattr
	// is enabled or if we decide to just remove it, it's ok to leave it
	// like this for now.
	ctx = bluectx.First(w.ctx, ctx)
	return w.Workspace.NewPty(ctx)
}

func (w *trackingWorkspace) Close() error {
	if w.cancelCtx != nil {
		cancelCtx := w.cancelCtx
		w.cancelCtx = nil
		cancelCtx()
		return w.server.Stop()
	}
	return nil
}

// wrap watcher to ensure that one of bluectx.First ctxs gets canceled
type wrapWatcher struct {
	watcher workspaceapi.ProcessWatcher
	ch      chan error
}

func newWrapWatcher(watcher workspaceapi.ProcessWatcher, cancelFn func()) wrapWatcher {
	ret := wrapWatcher{
		watcher: watcher,
		ch:      make(chan error),
	}
	go debug.CapturePanicReport(func() {
		err := <-ret.ch
		cancelFn()
		if ret.watcher != nil && ret.watcher.WatchProcess() != nil {
			t := time.After(2 * time.Minute) // in case watcher is unresponsive
			select {
			case ret.watcher.WatchProcess() <- err:
			case <-t:
			}
		}
	})
	return ret
}

func (w wrapWatcher) WatchProcess() chan error {
	return w.ch
}
