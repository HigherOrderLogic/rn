package editor

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func setupIntTest(
	t *testing.T, broker proto.MuxBroker, s *Server,
) (client *Client, closeFn func()) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	proto.RegisterEditorServer(grpcServer, s)

	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)

	client = NewClient(broker, conn)
	closeFn = func() {
		client.Close()
		grpcServer.Stop()
		lis.Close()
	}
	return
}

func expectInitialServerSubscribe(t *testing.T, mock *MockEditor) {
	mock.EXPECT().SubscribeEditor(gomock.Eq(EventTypeClose), gomock.Any()).Return(nil).AnyTimes()
	mock.EXPECT().SubscribeEditor(gomock.Eq(EventTypeOpen), gomock.Any()).Return(nil).AnyTimes()
}

func TestClientServerIntegration(t *testing.T) {
	t.Run("client through server calls underlying editor Edit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := proto.NewDialBroker()
		ed := NewMockEditor(ctrl)
		expectInitialServerSubscribe(t, ed)
		s := NewServer(b, ed, nopLocker{})

		client, closeFn := setupIntTest(t, b, s)
		defer closeFn()

		expectEdit(t, ed, "zion", "hero")
		buf := cell.NewBuffer()
		buf.WriteString("hero")

		_, err := client.Edit("zion", buf)
		require.NoError(t, err)
	})

	t.Run("underlying editor Edito errors bubble up to client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := proto.NewDialBroker()
		ed := NewMockEditor(ctrl)
		expectInitialServerSubscribe(t, ed)
		s := NewServer(b, ed, nopLocker{})

		client, closeFn := setupIntTest(t, b, s)
		defer closeFn()

		ed.EXPECT().Edit(gomock.Eq("babylon"), gomock.Any()).
			Return(nil, errors.New("The Upsetter")).
			Times(1)

		_, err := client.Edit("babylon", cell.NewBuffer())
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "The Upsetter"))
	})

	t.Run("client through server calls underlying editor Subscribe", func(t *testing.T) {
		str1 := "Granola Lola"
		tsuite := []struct {
			name       string
			evType     EventType
			trigger    func(t *testing.T, resourceName string, ed Editor, buf *cell.Buffer)
			start, end *term.Coordinates
			content    *string
		}{
			{
				"Edit->EventTypeOpen",
				EventTypeOpen,
				func(t *testing.T, resourceName string, ed Editor, buf *cell.Buffer) {
					ed.Edit(resourceName, buf)
				}, nil, nil, nil,
			},
			{
				"Edit->EventTypeInsert",
				EventTypeInsert,
				func(t *testing.T, resourceName string, ed Editor, buf *cell.Buffer) {
					ed.Edit(resourceName, buf)
					buf.WriteString(str1)
				}, &term.Coordinates{}, &term.Coordinates{X: 11}, &str1,
			},
			{
				"Edit->EventTypeDelete",
				EventTypeDelete,
				func(t *testing.T, resourceName string, ed Editor, buf *cell.Buffer) {
					buf.WriteString(str1)
					ed.Edit(resourceName, buf)
					buf.DeleteRow(0)
				}, &term.Coordinates{}, &term.Coordinates{X: 11}, &str1,
			},
		}

		for i, _tcase := range tsuite {
			tcase := _tcase
			t.Run(tcase.name, func(t *testing.T) {
				var wg sync.WaitGroup
				var mu sync.Mutex
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				b := proto.NewDialBroker()
				ed := &testEditor{}
				s := NewServer(b, ed, &mu)

				client, closeFn := setupIntTest(t, b, s)
				defer closeFn()

				err := client.SubscribeEditor(tcase.evType, FuncEventHandler(func(ev Event) bool {
					defer wg.Done()
					if tcase.start != nil {
						assert.Equal(t, *tcase.start, ev.Start)
					}
					if tcase.end != nil {
						assert.Equal(t, *tcase.end, ev.End)
					}
					if tcase.content != nil {
						assert.Equal(t, *tcase.content, ev.Content)
					}
					return false
				}))
				require.NoError(t, err)

				wg.Add(1)
				buf := cell.NewBuffer()
				// simulate runtime mutex
				mu.Lock()
				tcase.trigger(t, strconv.Itoa(i), ed, buf)
				mu.Unlock()
				wg.Wait()

				assert.NoError(t, s.Close())
				time.Sleep(asyncResultsSleepDuration)
			})
		}
	})

	t.Run("SetLocationList sets the location list of the remote editor", func(t *testing.T) {
		var wg sync.WaitGroup
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := proto.NewDialBroker()
		ed := NewMockEditor(ctrl)
		expectInitialServerSubscribe(t, ed)
		s := NewServer(b, ed, new(sync.Mutex))

		client, closeFn := setupIntTest(t, b, s)
		defer closeFn()

		expectEdit(t, ed, "Tais", "")
		h, err := client.Edit("Tais", cell.NewBuffer())
		require.NoError(t, err)

		l := LocationSlice([]Location{loc2})

		ed.EXPECT().SetLocationList(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(h Handler, id string, ll LocationList) error {
				defer wg.Done()
				assertLocation(t, ll, 0, loc2)
				assert.Equal(t, locID, id)
				assertLocationListLen(t, ll, 1)
				return nil
			}).Times(1)

		wg.Add(1)
		err = client.SetLocationList(h, locID, l)
		require.NoError(t, err)

		wg.Wait()
	})

	t.Run("Writer returns a Writer that is able to modify underlying buffer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := proto.NewDialBroker()
		ed := NewMockEditor(ctrl)
		expectInitialServerSubscribe(t, ed)
		s := NewServer(b, ed, nopLocker{})

		client, closeFn := setupIntTest(t, b, s)
		defer closeFn()

		buf := cell.NewBuffer()
		expectEdit(t, ed, "locotron", "")
		h, err := client.Edit("locotron", buf)
		require.NoError(t, err)

		ed.EXPECT().Writer(gomock.Any()).Return(CellWriter(buf.Writer())).Times(2)

		w := client.Writer(h)
		from, to, err := w.Insert(term.Coordinates{X: 1}, "el\nAridio")
		require.NoError(t, err)
		assert.Equal(t, term.Coordinates{}, from)
		assert.Equal(t, term.Coordinates{X: 5, Y: 1}, to)

		assert.Equal(t, " el\nAridio", buf.String())

		start, end, str, err := w.Delete(term.Coordinates{}, term.Coordinates{X: 3})
		require.NoError(t, err)
		assert.Equal(t, term.Coordinates{}, start)
		assert.Equal(t, term.Coordinates{X: 3}, end)
		assert.Equal(t, " el\n", str)

		assert.Equal(t, "Aridio", buf.String())
	})

	t.Run("Reader returns a Reader that is able to read underlying buffer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		b := proto.NewDialBroker()
		ed := NewMockEditor(ctrl)
		expectInitialServerSubscribe(t, ed)
		s := NewServer(b, ed, nopLocker{})

		client, closeFn := setupIntTest(t, b, s)
		defer closeFn()

		buf := cell.NewBuffer()
		buf.WriteString("guacamole")
		expectEdit(t, ed, "sup'App", "guacamole")
		h, err := client.Edit("sup'App", buf)
		require.NoError(t, err)

		ed.EXPECT().Reader(gomock.Any()).Return(CellReader(buf.Reader())).Times(2)

		r := client.Reader(h)
		cells, err := r.RawCells()
		require.NoError(t, err)
		assert.Equal(t, "guacamole", cell.CellsToString(cells))

		buf.WriteString("\npollos hermanos")

		cells, err = r.RawCells()
		require.NoError(t, err)
		assert.Equal(t, "guacamole\npollos hermanos", cell.CellsToString(cells))
	})
}

func TestRPCRegister(t *testing.T) {
	var closeFns []func()

	testRegister(t, func(ed Editor, mu *sync.Mutex, resName string) (*Component, Editor, error) {
		c, err := newTestComponent(ed)
		if err != nil {
			return nil, nil, err
		}

		if m, ok := ed.(*MockEditor); ok {
			expectInitialServerSubscribe(t, m)
		}

		b := proto.NewDialBroker()
		s := NewServer(b, c, mu)

		client, closeFn := setupIntTest(t, b, s)
		closeFns = append(closeFns, func() {
			s.Close()
			closeFn()
		})

		return c, client, err
	})

	for _, closeFn := range closeFns {
		closeFn()
	}
}
