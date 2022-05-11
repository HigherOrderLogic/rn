package plugin

import (
	"context"
	"io/ioutil"
	_ "net/http/pprof"
	"strings"
	"sync"
	"testing"

	"github.com/ernestrc/blue/datastore/document"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/proto"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
	"github.com/ernestrc/go-tui/workspace"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func assertClientMethodNoError(
	t *testing.T, resource interface{},
	wg *sync.WaitGroup, mu sync.Locker,
	method func(ifc interface{}) error,
) {
	defer wg.Done()
	mu.Lock()
	defer mu.Unlock()
	err := method(resource)
	require.NoError(t, err)
}

func TestIntegrationRace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWin := browser.NopWindow()
	h := browser.NewTestHandler()
	cache := document.NewInMemoryCache()
	broker := proto.NewDatastoreBroker(cache, nil)
	defer broker.Close()
	mock := browser.NewMockBrowser(ctrl)
	edMock := text.NewMockEditor(ctrl)
	edMock.EXPECT().SubscribeEditorEvents(gomock.Any(), gomock.Any()).Times(1)
	resources := MergeResourceMap(BrowserResources(mock), EditorResources(edMock))
	resources[PermissionClipboard] = NewClipboardManager()

	wpMock := workspace.NewMockWorkspace(ctrl)
	resources = MergeResourceMap(resources, WorkspaceResources(wpMock))

	uri, err := workspace.ParseURI("file:///tmp/test")
	require.NoError(t, err)

	// plugin manager serves each permission on a different grantID
	perms := map[Permission]uint32{
		PermissionBrowserWindowManager:  broker.NextId(),
		PermissionBrowserResourceOpener: broker.NextId(),
		PermissionBrowserMessenger:      broker.NextId(),
		PermissionBrowserEventPublisher: broker.NextId(),
		PermissionEditor:                broker.NextId(),
		PermissionBrowserStorage:        broker.NextId(),
		PermissionClipboard:             broker.NextId(),
		PermissionWorkspace:             broker.NextId(),
	}
	for perm, brokerID := range perms {
		resources[perm].Serve("caliu-plugins-ltd", brokerID,
			broker, nil, new(sync.Mutex))
	}

	tsuite := []struct {
		perm           Permission
		createResource func(uint32, proto.MuxBroker) (interface{}, error)
		expect         func(*workspace.MockWorkspaceMockRecorder, *text.MockEditorMockRecorder, *browser.MockBrowserMockRecorder) *gomock.Call
		method         func(ifc interface{}) error
	}{
		{PermissionBrowserWindowManager, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return WindowManager(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Focus().Return(mockWin, nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(browser.WindowManager).Focus()
			return err
		}},
		{PermissionBrowserWindowManager, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return WindowManager(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Split(gomock.Any(), gomock.Any()).Return(mockWin, nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(browser.WindowManager).Split(browser.OrientationBottom, h)
			return err
		}},
		{PermissionBrowserWindowManager, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return WindowManager(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Bar(gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.WindowManager).Bar(browser.OrientationBottom, h)
		}},
		{PermissionBrowserWindowManager, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return WindowManager(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Tab(gomock.Any(), gomock.Any(), gomock.Any()).Return(h, nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(browser.WindowManager).Tab(uri, "", h)
			return err
		}},
		{PermissionBrowserWindowManager, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return WindowManager(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Floating(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockWin, nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(browser.WindowManager).Floating(h, term.Coordinates{}, 1, 1)
			return err
		}},
		{PermissionBrowserResourceOpener, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return ResourceOpener(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Open(gomock.Any()).Return(h, nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(browser.ResourceOpener).Open(uri)
			return err
		}},
		{PermissionBrowserMessenger, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Messenger(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.SetMessage(gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.Messenger).SetMessage("")
		}},
		{PermissionBrowserEventPublisher, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return EventPublisher(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.PublishInterrupt().Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.EventPublisher).PublishInterrupt()
		}},
		{PermissionBrowserEventPublisher, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return EventPublisher(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.PublishEventNone().Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.EventPublisher).PublishEventNone()
		}},
		{PermissionEditor, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Editor(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return ed.Edit(gomock.Any(), gomock.Any()).Return(nil, nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(text.Editor).Edit(uri, cell.NewBuffer())
			return err
		}},
		{PermissionEditor, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Editor(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return ed.SubscribeEditorEvents(gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			h := text.FuncEventHandler(func(context.Context, text.Event) bool { return false })
			ev := []text.EventType{text.EventTypeFlush}
			return ifc.(text.Editor).SubscribeEditorEvents(ev, h)
		}},
		{PermissionEditor, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Editor(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			ed.Edit(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			return ed.SetCursor(gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			// force cache token
			h, err := ifc.(text.Editor).Edit(uri, cell.NewBuffer())
			if err != nil {
				return err
			}
			return ifc.(text.Editor).SetCursor(h, term.Coordinates{})
		}},
		{PermissionEditor, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Editor(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			ed.Edit(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			return ed.Cursor(gomock.Any()).Return(term.Coordinates{}, nil)
		}, func(ifc interface{}) error {
			// force cache token
			h, err := ifc.(text.Editor).Edit(uri, cell.NewBuffer())
			if err != nil {
				return err
			}
			_, err = ifc.(text.Editor).Cursor(h)
			return err
		}},
		{PermissionBrowserStorage, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Storage(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.Storage).Create(context.Background(), "", map[string]interface{}{"a": "b"})
		}},
		{PermissionBrowserStorage, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Storage(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			var recv map[string]interface{}
			return ifc.(browser.Storage).Get(context.Background(), "", &recv)
		}},
		{PermissionBrowserStorage, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Storage(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.Storage).Update(context.Background(), "", []document.Update{{FieldPath: []string{"a"}, Value: "b"}})
		}},
		{PermissionBrowserStorage, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Storage(token, broker)
		}, func(_ *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return mock.Delete(gomock.Any(), gomock.Any()).Return(nil)
		}, func(ifc interface{}) error {
			return ifc.(browser.Storage).Delete(context.Background(), "")
		}},
		{PermissionClipboard, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Clipboard(token, broker)
			// we test ClipboardManager directly
		}, nil, func(ifc interface{}) error {
			return ifc.(ClipboardSetter).SetRegister(text.DefaultRegisterID, nil)
		}},
		{PermissionWorkspace, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Workspace(token, broker)
		}, func(exec *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return exec.Command(gomock.Any(), gomock.Any()).Return(workspace.Pid(1), nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(workspace.Executor).Command("", "")
			return err
		}},
		{PermissionWorkspace, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Workspace(token, broker)
		}, func(exec *workspace.MockWorkspaceMockRecorder, ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			return exec.StdoutPipe(gomock.Any()).Return(ioutil.NopCloser(strings.NewReader(":")), nil)
		}, func(ifc interface{}) error {
			_, err := ifc.(workspace.Executor).StdoutPipe(workspace.Pid(0))
			return err
		}},
		/* FIXME: CI tests are failing due to mock.List not being called
		{PermissionBrowserStorage, func(token uint32, broker proto.MuxBroker) (interface{}, error) {
			return Storage(token, broker)
		}, func(ed *text.MockEditorMockRecorder, mock *browser.MockBrowserMockRecorder) *gomock.Call {
			m1, m2 := make(map[string]interface{}), make(map[string]interface{})
			return mock.List(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, filters []document.Filter) (document.Iterator, error) {
					// instantiate for every invokation
					// or else we run into data race. as mock expect data
					// is shared across different invokations
					return document.ListIterator(m1, m2), nil
				})
		}, func(ifc interface{}) error {
			_, err := ifc.(browser.Storage).List(context.Background(), nil)
			return err
		}},*/
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, tcase := range tsuite {
		brokerID := perms[tcase.perm]
		res1, err := tcase.createResource(brokerID, broker)
		require.NoError(t, err)

		res2, err := tcase.createResource(brokerID, broker)
		require.NoError(t, err)

		n := 5
		if tcase.expect != nil {
			tcase.expect(wpMock.EXPECT(), edMock.EXPECT(), mock.EXPECT()).Times(n * 2)
		}
		for i := 0; i < n; i++ {
			wg.Add(2)
			go assertClientMethodNoError(t, res1, &wg, &mu, tcase.method)
			go assertClientMethodNoError(t, res2, &wg, &mu, tcase.method)
		}

	}
	wg.Wait()
}
