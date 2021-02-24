package editor

import (
	"context"
	"errors"
	"testing"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/proto"
	prototest "github.com/ernestrc/go-tui/proto/test"
	"github.com/ernestrc/go-tui/term"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func newTestClient(ctrl *gomock.Controller) (
	*proto.MockMuxBroker, *proto.MockClientConnInterface, *Client,
) {
	broker := proto.NewMockMuxBroker(ctrl)
	cc := proto.NewMockClientConnInterface(ctrl)
	c := NewClient(broker, cc)
	return broker, cc, c
}

func expectClientEdit(
	t *testing.T, mockCC *proto.MockClientConnInterface,
	expectedContent string,
) {
	mockCC.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq("/proto.Editor/Edit"),
			gomock.Any(),
			gomock.Any()).
		DoAndReturn(func(
			ctx context.Context, method string, args interface{},
			reply interface{}, opts ...grpc.CallOption) error {
			editReq, ok := args.(*proto.EditRequest)
			require.True(t, ok)

			buf := proto.EditRequestToBuffer(editReq)
			assert.Equal(t, expectedContent, buf.String())

			_, ok = reply.(*proto.EditResponse)
			assert.True(t, ok)
			return nil
		}).
		Times(1)
}

func TestClientEdit(t *testing.T) {
	resourceName1 := "54-46 Was My Number"
	bufContent1 := "The Maytals"

	t.Run("happy path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		_, cc, c := newTestClient(ctrl)

		buf := cell.NewBuffer()
		buf.WriteString(bufContent1)

		expectClientEdit(t, cc, bufContent1)

		h, err := c.Edit(resourceName1, buf)
		require.NoError(t, err)
		require.NotNil(t, h)
	})

	t.Run("handles underlying client error ", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		_, cc, c := newTestClient(ctrl)

		cc.EXPECT().
			Invoke(gomock.Any(),
				gomock.Eq("/proto.Editor/Edit"),
				gomock.Any(),
				gomock.Any()).
			Times(1).
			Return(errors.New("Would be a change of plan"))

		h, err := c.Edit(resourceName1, cell.NewBuffer())
		require.Error(t, err)
		require.Nil(t, h)
	})
}

func expectClientSubscribe(
	t *testing.T, mockCC *proto.MockClientConnInterface,
	expectedHandlerID uint32,
	expectedEventType EventType,
) {
	mockCC.EXPECT().
		Invoke(gomock.Any(),
			gomock.Eq("/proto.Editor/Subscribe"),
			gomock.Any(),
			gomock.Any()).
		DoAndReturn(func(
			ctx context.Context, method string, args interface{},
			reply interface{}, opts ...grpc.CallOption) error {
			req, ok := args.(*proto.EditorSubscribeRequest)
			require.True(t, ok)

			assert.Equal(t, expectedHandlerID, req.GetHandlerId())
			assert.Equal(t, Event{Type: expectedEventType}.protoType(), req.GetType())

			_, ok = reply.(*proto.EditorSubscribeResponse)
			assert.True(t, ok)
			return nil
		}).
		Times(1)
}

func TestClientSubscribe(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		broker, cc, c := newTestClient(ctrl)
		handler := NewMockEventHandler(ctrl)

		brokerID := uint32(22)
		evType := EventTypeFlush
		expectClientSubscribe(t, cc, brokerID, evType)

		prototest.ExpectBrokerServe(t, brokerID, broker)

		err := c.SubscribeEditor(evType, handler)
		require.NoError(t, err)

		assert.NoError(t, c.Close())
	})
}

func TestSetLocationListRequest(t *testing.T) {
	t.Run("non-nil zero slice", func(t *testing.T) {
		l := LocationSlice([]Location{})
		handlerID := uint32(23)
		expected := proto.SetLocationListRequest{
			HandlerId: handlerID,
			ListId:    locID,
			Locations: nil,
		}
		assert.Equal(t, expected, makeLocationListRequest(handlerID, locID, l))
	})

	t.Run("nil zero slice", func(t *testing.T) {
		l := LocationSlice(nil)
		handlerID := uint32(23)
		expected := proto.SetLocationListRequest{
			HandlerId: handlerID,
			ListId:    locID,
			Locations: nil,
		}
		assert.Equal(t, expected, makeLocationListRequest(handlerID, locID, l))
	})
	t.Run("non-zero slice", func(t *testing.T) {
		l := LocationSlice([]Location{
			loc1,
			loc2,
			loc3,
		})
		handlerID := uint32(23)
		expected := proto.SetLocationListRequest{
			HandlerId: handlerID,
			ListId:    locID,
			Locations: []*proto.SetLocationListRequest_Location{
				{
					From: &proto.Coordinates{},
					To:   &proto.Coordinates{X: 1, Y: 3},
					Attr: &proto.Attributes{Foreground: uint32(term.AttrBold)},
				},
				{
					To:   &proto.Coordinates{},
					From: &proto.Coordinates{X: 1, Y: 3},
					Attr: &proto.Attributes{
						Foreground: uint32(term.ColorBlack),
						Background: uint32(term.ColorGreen),
					},
					Msg: "wsb: hold BBBY",
				},
				{
					From: &proto.Coordinates{},
					To:   &proto.Coordinates{},
					Attr: &proto.Attributes{},
				},
			},
		}

		assert.Equal(t, expected, makeLocationListRequest(handlerID, locID, l))
	})
	t.Run("with message", func(t *testing.T) {
		myMsg := "wsb: HOLD GME"
		l := LocationSlice([]Location{
			{
				To:      term.Coordinates{X: 1, Y: 3},
				Attr:    term.Attributes{Fg: term.AttrBold},
				Message: myMsg,
			},
		})
		handlerID := uint32(23)
		expected := proto.SetLocationListRequest{
			HandlerId: handlerID,
			ListId:    locID,
			Locations: []*proto.SetLocationListRequest_Location{
				{
					From: &proto.Coordinates{},
					To:   &proto.Coordinates{X: 1, Y: 3},
					Attr: &proto.Attributes{Foreground: uint32(term.AttrBold)},
					Msg:  myMsg,
				},
			},
		}

		assert.Equal(t, expected, makeLocationListRequest(handlerID, locID, l))
	})
}

func benchmarkSetLocationListRequest(b *testing.B, n int) {
	l := make([]Location, n)
	for i := 0; i < n; i++ {
		l[i] = Location{
			From: term.Coordinates{X: i, Y: n},
			To:   term.Coordinates{X: n, Y: n},
			Attr: term.Attributes{Fg: term.AttrBold},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ll := LocationSlice(l)
		_ = makeLocationListRequest(45, locID, ll)
	}
}

func BenchmarkSetLocationListRequest10(b *testing.B) {
	benchmarkSetLocationListRequest(b, 10)
}

func BenchmarkSetLocationListRequest100(b *testing.B) {
	benchmarkSetLocationListRequest(b, 100)
}

func BenchmarkSetLocationListRequest1000(b *testing.B) {
	benchmarkSetLocationListRequest(b, 1000)
}

func BenchmarkSetLocationListRequest10000(b *testing.B) {
	benchmarkSetLocationListRequest(b, 10000)
}
