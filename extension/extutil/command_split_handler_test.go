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

package extutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"unstable.build/go-tui"
	"unstable.build/go-tui/api/browserapi"
	browserapitest "unstable.build/go-tui/api/browserapi/browsertest"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/browser/browsertest"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
)

var emptyConfig = config.MapConfig(make(map[string]interface{}))

type handlerCloser struct {
	tui.Handler
	calledClosed int
}

func (h *handlerCloser) Close() error {
	h.calledClosed++
	return nil
}

func expectInitialization(
	t *testing.T, ctrl *gomock.Controller, h *cmdSplitHandler,
) *rpc.MockMuxBroker {
	broker := rpc.NewMockMuxBroker(ctrl)
	h.Connected(context.Background(), broker, emptyConfig)
	h.Health(context.Background())
	return broker
}

type nopConn struct {
}

func (n nopConn) GetState() connectivity.State {
	return connectivity.Ready
}

func (n nopConn) WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool {
	return true
}

func (n nopConn) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	return nil
}

func (n nopConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}

func (n nopConn) Close() error {
	return nil
}

func TestCommandSplitHandlerEmpty(t *testing.T) {

	t.Run("panics if cmd configuration is missing", func(t *testing.T) {
		assert.Panics(t, func() {
			NewCommandSplitHandler(CommandSplitHandlerConfig{})
		})
	})

	t.Run("grantee initialization runs correctly", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := CommandSplitHandlerConfig{}
		h := &cmdSplitHandler{config: config}

		expectInitialization(t, ctrl, h)
	})

	t.Run("does nothing if shutdown is called when window not active", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := CommandSplitHandlerConfig{Command: testCommand("blah")}
		h := &cmdSplitHandler{config: config}
		expectInitialization(t, ctrl, h)
		h.Shutdown(context.Background(), "you are being naughty")
	})
}

func TestCommandSplitHandlerOpenWindow(t *testing.T) {
	t.Run("open a split window if command received", func(t *testing.T) {
		cmdName := "blah"
		config := CommandSplitHandlerConfig{
			Command:          testCommand(cmdName),
			SplitOrientation: browserapi.OrientationLeft,
		}

		testSplitWindow(t, config, func(h *cmdSplitHandler) {
			err := h.HandleCommand(context.Background(), textapi.Command{Name: cmdName})
			require.NoError(t, err)
		})
	})

	t.Run("calls Redispatch if command received subsequently", func(t *testing.T) {
		cmdName := "blah"
		var called int
		rh := FuncRedispatchHandler(browsertest.NewTestHandler(),
			func(ctx context.Context, cmd textapi.Command) error {
				called++
				return nil
			})
		config := CommandSplitHandlerConfig{
			Command:          testCommand(cmdName),
			SplitOrientation: browserapi.OrientationLeft,
			Handler: func(_ context.Context, _ textapi.Command, grants []extension.Grant, broker rpc.MuxBroker,
				focus browserapi.Window, c config.Config) (RedispatchHandler, error) {
				return rh, nil
			},
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockWm := browserapitest.NewMockWindowManager(ctrl)
		mockWm.EXPECT().
			Split(gomock.Eq(config.SplitOrientation), gomock.Any(), gomock.Any()).
			Return(browserapitest.NewMockWindow(ctrl), nil)
		h := &cmdSplitHandler{config: config, wm: mockWm}

		// once splits
		err := h.HandleCommand(context.Background(), textapi.Command{Name: cmdName})
		require.NoError(t, err)

		// second calls redispatch
		err = h.HandleCommand(context.Background(), textapi.Command{Name: cmdName})
		require.NoError(t, err)
		assert.Equal(t, 1, called)

		err = h.HandleCommand(context.Background(), textapi.Command{Name: cmdName})
		require.NoError(t, err)
		assert.Equal(t, 2, called)
	})

	t.Run("closes window when handler is closed", func(t *testing.T) {
		cmdName := "blah"
		handler := browsertest.NewTestHandler()
		handler.Exit = true
		rh := FuncRedispatchHandler(handler,
			func(ctx context.Context, cmd textapi.Command) error {
				return nil
			})
		config := CommandSplitHandlerConfig{
			Command:          testCommand(cmdName),
			SplitOrientation: browserapi.OrientationLeft,
			Handler: func(_ context.Context, _ textapi.Command, grants []extension.Grant, broker rpc.MuxBroker,
				focus browserapi.Window, c config.Config) (RedispatchHandler, error) {
				return rh, nil
			},
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		var h browserapi.Handler
		mockWm := browserapitest.NewMockWindowManager(ctrl)
		win := browserapitest.NewMockWindow(ctrl)
		mockWm.EXPECT().
			Split(gomock.Eq(config.SplitOrientation), gomock.Any(), gomock.Any()).
			DoAndReturn(func(
				_ browserapi.Orientation, _ browserapi.Window, _h browserapi.Handler,
			) (browserapi.Window, error) {
				h = _h
				return win, nil
			})
		splitHandler := cmdSplitHandler{config: config, wm: mockWm}

		err := splitHandler.HandleCommand(context.Background(), textapi.Command{Name: cmdName})
		require.NoError(t, err)

		mockWm.EXPECT().CloseWindow(gomock.Any())
		require.NoError(t, h.Close())
	})
}

func testSplitWindow(
	t *testing.T, cfg CommandSplitHandlerConfig,
	action func(*cmdSplitHandler),
) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg.Handler = func(_ context.Context, _ textapi.Command, grants []extension.Grant, broker rpc.MuxBroker,
		focus browserapi.Window, c config.Config) (RedispatchHandler, error) {
		return NopRedispatchHandler(browsertest.NewTestHandler()), nil
	}
	mockWm := browserapitest.NewMockWindowManager(ctrl)
	h := &cmdSplitHandler{config: cfg, wm: mockWm}
	mockWm.EXPECT().
		Split(gomock.Eq(cfg.SplitOrientation), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	action(h)
}

func testCommand(cmd string) textapi.CommandManual {
	return textapi.CommandManual{
		Name: cmd,
	}
}
