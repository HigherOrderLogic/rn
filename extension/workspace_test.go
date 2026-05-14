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

package extension

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi/workspacerpc"
	"google.golang.org/grpc"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/workspace"
	tworkspacerpc "unstable.build/go-tui/workspace/workspacerpc"
	"unstable.build/go-tui/workspace/workspacetest"
)

func TestWorkspaceResourcesUsesStartCommandAuthorizer(t *testing.T) {
	t.Parallel()

	scheme := &testWorkspace{NopScheme: &workspacetest.NopScheme{}}
	scheme.StartCommandFunc = func(context.Context, workspaceapi.Cmd) (workspaceapi.Pid, error) {
		return 1, nil
	}

	calls := 0
	resources := WorkspaceResources(scheme, tworkspacerpc.CommandAuthorizerFunc(
		func(ctx context.Context, cmd workspaceapi.Cmd) error {
			calls++
			assert.Equal(t, "echo", cmd.Path)
			assert.Equal(t, []string{"hello"}, cmd.Args)
			return nil
		}))

	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()
	closer, err := resources[extensionapi.PermissionExecute].Register(grpcServer, new(sync.Mutex))
	require.NoError(t, err)
	defer closer.Close()
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := workspacerpc.NewClient(context.Background(), conn)
	defer client.Close()

	pid, err := client.StartCommand(context.Background(), workspaceapi.Cmd{
		Path: "echo",
		Args: []string{"hello"},
	})
	require.NoError(t, err)
	require.NotZero(t, pid)
	assert.Equal(t, 1, calls)
}

type testWorkspace struct {
	*workspacetest.NopScheme
}

func (t *testWorkspace) Load(
	file workspaceapi.URI, buf *cell.Buffer, swapDir workspaceapi.URI, readOnly bool,
) (workspace.FlusherCloser, error) {
	return testFlusherCloser{}, nil
}

func (t *testWorkspace) Recover(
	file, swapFilePath workspaceapi.URI, buf *cell.Buffer, force bool,
) (workspace.FlusherCloser, error) {
	return testFlusherCloser{}, nil
}

var _ workspace.Workspace = (*testWorkspace)(nil)

type testFlusherCloser struct{}

func (testFlusherCloser) Flush(context.Context) (<-chan error, error) {
	return testFlusherCloserDone(), nil
}
func (testFlusherCloser) LastFlush() time.Time { return time.Time{} }
func (testFlusherCloser) ForceFlush(context.Context) (<-chan error, error) {
	return testFlusherCloserDone(), nil
}
func (testFlusherCloser) Reload(context.Context) (<-chan error, error) {
	return testFlusherCloserDone(), nil
}
func (testFlusherCloser) Close() error { return nil }

func testFlusherCloserDone() <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch
}

var _ io.Closer = testFlusherCloser{}
