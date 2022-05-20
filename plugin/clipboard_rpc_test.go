package plugin

import (
	"net"
	"sync"
	"testing"

	"github.com/ernestrc/go-tui/proto"
	gomock "github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
)

func setupClipboardIntTest(
	t *testing.T, root Clipboard,
) (client Clipboard, closeFn func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	broker := proto.NewDialBroker()

	grpcServer := grpc.NewServer()
	srv := newClipboardServer(log.New(), broker, root, new(sync.Mutex))
	proto.RegisterClipboardServer(grpcServer, srv)
	proto.RegisterClipboardRegisterServer(grpcServer, srv.defaultRegisterServer)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	go grpcServer.Serve(lis)

	client = newClipboardClient(log.New(), broker, conn)
	closeFn = func() {
		client.(*clipboardClient).Close()
		grpcServer.Stop()
		srv.Close()
		lis.Close()
	}
	return
}

func TestClipboardIntegration(t *testing.T) {
	registerID := "Halston"

	t.Run("client/server calls underlying served ClipboardSetter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock := NewMockClipboard(ctrl)
		client, closeFn := setupClipboardIntTest(t, mock)
		defer closeFn()

		mock.EXPECT().SetRegister(gomock.Any(), gomock.Any()).Return(nil)

		err := client.SetRegister(registerID, nil)
		require.NoError(t, err)
	})

	t.Run("client/server call underlying ClipboardRegister", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock := NewMockClipboard(ctrl)
		client, closeFn := setupClipboardIntTest(t, mock)
		defer closeFn()

		mock.EXPECT().Copy(gomock.Eq("Montessori")).Return(nil)

		err := client.Copy("Montessori")
		require.NoError(t, err)

		mock.EXPECT().Paste().Return("Montessori", nil)
		paste, err := client.Paste()
		require.NoError(t, err)
		assert.Equal(t, "Montessori", paste)
	})

	t.Run("server closes does not leak overwritten client register", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock := NewMockClipboard(ctrl)
		client, closeFn := setupClipboardIntTest(t, mock)

		mock.EXPECT().SetRegister(gomock.Any(), gomock.Any()).Return(nil)

		err := client.SetRegister(registerID, nil)
		require.NoError(t, err)

		closeFn()
		ignoreOpenCensus := goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start")
		goleak.VerifyNone(t, ignoreOpenCensus)
	})
}
