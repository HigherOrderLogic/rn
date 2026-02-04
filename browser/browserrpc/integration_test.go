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

package browserrpc

import (
	context "context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi/browserrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/browser/browsertest"
)

func newClientServerIntegration(
	t *testing.T, h browser.Browser,
) (*browserrpc.Client, func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	mutex := new(sync.Mutex)

	grpcServer := grpc.NewServer()
	rpcServer := NewServer(h, mutex)
	rpcServer.SetSyncMode()
	browserrpc.RegisterWindowManagerServer(grpcServer, rpcServer)
	browserrpc.RegisterResourceOpenerServer(grpcServer, rpcServer)
	browserrpc.RegisterNotificationsServer(grpcServer, rpcServer)
	browserrpc.RegisterEventPublisherServer(grpcServer, rpcServer)

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	client := browserrpc.NewClient(context.Background(), conn)

	closeFn := func() {
		client.Close()
		rpcServer.browser.Lock()
		rpcServer.Stop()
		rpcServer.browser.Unlock()
		grpcServer.Stop()
		conn.Close()
	}

	return client, closeFn
}

func TestClientServerIntegrationSplit(t *testing.T) {
	tsuite := []browserapi.Orientation{
		browserapi.OrientationDefault,
		browserapi.OrientationTop,
		browserapi.OrientationBottom,
		browserapi.OrientationLeft,
		browserapi.OrientationRight,
	}

	for _, _tcase := range tsuite {
		tcase := _tcase
		t.Run(fmt.Sprintf("%+v", tcase), func(t *testing.T) {
			defer goleak.VerifyNone(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := browsertest.NewMockBrowser(ctrl)
			client, cleanup := newClientServerIntegration(t, mock)
			defer cleanup()

			win0 := browsertest.NopWindow()
			win1 := browsertest.NopWindow()

			mock.EXPECT().Window(gomock.Any()).Return(win1, true).AnyTimes()
			mock.EXPECT().Split(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					o browserapi.Orientation, win browserapi.Window, h browserapi.Handler,
				) (browser.Window, error) {
					assert.Equal(t, tcase, o)
					return win1, nil
				})
			resWin1, err := client.Split(tcase, win0, browsertest.NewTestHandler())
			require.NoError(t, err)

			mock.EXPECT().Window(gomock.Any()).Return(win1, true).AnyTimes()
			require.NoError(t, client.SetWindowContent(resWin1, browsertest.NewTestHandler()))
			require.NoError(t, client.CloseWindow(resWin1))
		})
	}
}

func TestClientServerIntegrationBar(t *testing.T) {
	tsuite := []browserapi.BarConfig{
		{Orientation: browserapi.OrientationDefault, Frame: browserapi.BarFrameAlways, Size: 1},
		{Orientation: browserapi.OrientationTop, Frame: browserapi.BarFrameAlways, Size: 1},
		{Orientation: browserapi.OrientationBottom, Frame: browserapi.BarFrameAlways, Size: 1},
		{Orientation: browserapi.OrientationLeft, Frame: browserapi.BarFrameAlways, Size: 1},
		{Orientation: browserapi.OrientationRight, Frame: browserapi.BarFrameAlways, Size: 1},
		{Orientation: browserapi.OrientationDefault, Frame: browserapi.BarFrameDefault, Size: 1},
		{Orientation: browserapi.OrientationTop, Frame: browserapi.BarFrameDefault, Size: 1},
		{Orientation: browserapi.OrientationBottom, Frame: browserapi.BarFrameDefault, Size: 1},
		{Orientation: browserapi.OrientationLeft, Frame: browserapi.BarFrameDefault, Size: 1},
		{Orientation: browserapi.OrientationRight, Frame: browserapi.BarFrameDefault, Size: 1},
		{Orientation: browserapi.OrientationDefault, Frame: browserapi.BarFrameNever, Size: 1},
		{Orientation: browserapi.OrientationTop, Frame: browserapi.BarFrameNever, Size: 1},
		{Orientation: browserapi.OrientationBottom, Frame: browserapi.BarFrameNever, Size: 1},
		{Orientation: browserapi.OrientationLeft, Frame: browserapi.BarFrameNever, Size: 1},
		{Orientation: browserapi.OrientationRight, Frame: browserapi.BarFrameNever, Size: 1},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase
		t.Run(fmt.Sprintf("%+v", tcase), func(t *testing.T) {
			defer goleak.VerifyNone(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := browsertest.NewMockBrowser(ctrl)
			client, cleanup := newClientServerIntegration(t, mock)
			defer cleanup()

			win1 := browsertest.NopWindow()

			mock.EXPECT().Window(gomock.Any()).Return(win1, true).AnyTimes()
			mock.EXPECT().Bar(gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					config browserapi.BarConfig, h tui.Handler,
				) error {
					assert.Equal(t, tcase.Orientation, config.Orientation)
					assert.Equal(t, tcase.Frame, config.Frame)
					assert.Equal(t, tcase.Size, config.Size)
					return nil
				})
			err := client.Bar(tcase, browsertest.NewTestHandler())
			require.NoError(t, err)
		})
	}
}

func TestClientServerIntegrationTab(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := browsertest.NewMockBrowser(ctrl)
	client, cleanup := newClientServerIntegration(t, mock)
	defer cleanup()

	expectedURI, err := workspaceapi.ParseURI("ABV://SanCarlos@2017/BlueLime")
	require.NoError(t, err)
	expectedIcon := 'X'
	expectedName := "Linduro"

	mock.EXPECT().Resource(gomock.Any()).Return(browsertest.NewTestHandler(), true)
	mock.EXPECT().Tab(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			uri workspaceapi.URI, icon rune, name string, h browserapi.Handler,
		) (browserapi.Handler, error) {
			assert.Equal(t, expectedURI, uri)
			assert.Equal(t, expectedIcon, icon)
			assert.Equal(t, expectedName, name)
			return browsertest.NewTestHandler(), nil
		})
	tab, err := client.Tab(expectedURI, expectedIcon,
		expectedName, browsertest.NewTestHandler())
	require.NoError(t, err)

	win1 := browsertest.NopWindow()
	mock.EXPECT().Focus().Return(win1, nil)

	win, err := client.Focus()
	require.NoError(t, err)

	mock.EXPECT().Window(gomock.Any()).Return(win1, true).AnyTimes()
	require.NoError(t, client.SetWindowContent(win, tab))
}
