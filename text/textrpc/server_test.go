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

package textrpc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi/textrpc"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/term/termrpc"
	gomock "go.uber.org/mock/gomock"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

const asyncResultsSleepDuration = 300 * time.Millisecond
const locID = "errors"

type nopLocker struct{}

func (l nopLocker) Lock()   {}
func (l nopLocker) Unlock() {}

func newTestServer(t *testing.T, ctrl *gomock.Controller) (*texttest.MockEditor, *Server) {
	ed := texttest.NewMockEditor(ctrl)
	s := NewServer(nopNotifications{}, ed, new(sync.Mutex))
	return ed, s
}

func expectEdit(t *testing.T, mock *texttest.MockEditor, resource workspaceapi.URI, content string, readOnly, recovered bool) {
	mock.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
		DoAndReturn(func(_uri workspaceapi.URI, buf *cell.Buffer, _readOnly, _recovered bool) (text.Handler, error) {
			assert.Equal(t, resource, _uri)
			assert.Equal(t, content, buf.String())
			assert.Equal(t, readOnly, _readOnly)
			assert.Equal(t, recovered, _recovered)
			return texttest.NewTestHandler(), nil
		})
}

func expectEditor(
	t *testing.T, ctrl *gomock.Controller,
	mock *texttest.MockEditor, resource workspaceapi.URI,
) *texttest.MockHandler {
	ret := texttest.NewMockHandler(ctrl)
	mock.EXPECT().Editor(gomock.Any()).AnyTimes().
		DoAndReturn(func(_uri workspaceapi.URI) (text.Handler, error) {
			assert.Equal(t, resource, _uri)
			return ret, nil
		})
	return ret
}

func callServerEdit(
	t *testing.T, ctx context.Context,
	s *Server, uri workspaceapi.URI, content string,
	readOnly, recovered bool,
) {
	buf := cell.NewBuffer()
	buf.WriteString(content)
	req := NewEditRequest(uri, buf, readOnly, recovered)

	res, err := s.Edit(ctx, &req)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestServerEdit(t *testing.T) {
	ctx := context.Background()
	resource, err := workspaceapi.ParseURI("file:///ULaptopNotLinux:@")
	require.NoError(t, err)
	bufContent1 := "ULaptopWillLinux:)"

	t.Run("Edit is propagated to underlying Editor", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock, s := newTestServer(t, ctrl)
		expectEdit(t, mock, resource, bufContent1, false, false)
		callServerEdit(t, ctx, s, resource, bufContent1, false, false)
	})

	t.Run("relative path is converted to absolute", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock, s := newTestServer(t, ctrl)
		require.NoError(t, err)
		expectEdit(t, mock, resource, bufContent1, false, false)
		callServerEdit(t, ctx, s, resource, bufContent1, false, false)
	})

	t.Run("bubbles up underlying's Editor Edit errors", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock, s := newTestServer(t, ctrl)

		mock.EXPECT().Edit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("NOLINUX")).
			Times(1)

		req := NewEditRequest(resource, cell.NewBuffer(), false, false)

		res, err := s.Edit(ctx, &req)
		require.Error(t, err)
		require.Nil(t, res)
	})
}

func assertEqualLocations(t *testing.T, loc, expected text.LocationList) {
	var locations, expectedLocations []textapi.Location
	for ok := true; ok; _, ok = loc.Prev() {

	}
	for ok := true; ok; _, ok = expected.Prev() {

	}
	for n, ok := loc.Current(); ok; n, ok = loc.Next() {
		locations = append(locations, n)
	}
	for n, ok := expected.Current(); ok; n, ok = expected.Next() {
		expectedLocations = append(expectedLocations, n)
	}
	assert.EqualValues(t, expectedLocations, locations)
}

func TestServerSetLocationList(t *testing.T) {
	resource, err := workspaceapi.ParseURI("file:///go-tui")
	require.NoError(t, err)

	t.Run("calls underlying editor SetLocationList", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, s := newTestServer(t, ctrl)

		content := "main"
		expectEdit(t, mock, resource, content, false, false)
		callServerEdit(t, ctx, s, resource, content, false, false)

		locs := text.LocationSlice([]textapi.Location{{Message: "wsb: hold AMC", To: term.Coordinates{X: 3}}})
		h := expectEditor(t, ctrl, mock, resource)
		h.EXPECT().SetLocationList(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(func(pri textapi.LocationPriority, ID string, l text.LocationList) {
				assert.Equal(t, textapi.LocationPriorityInfo, pri)
				assert.Equal(t, locID, ID)
			})

		req := makeLocationListRequest(resource, textapi.LocationPriorityInfo, locID, locs)
		res, err := s.SetLocationList(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
	})

	t.Run("is threadsafe", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ed := texttest.NopEditor()
		c, err := text.NewComponent(ed, &testLoader{}, text.Config{
			Config: browser.Config{
				Wallpaper: browser.NopWallpaper(),
			},
		})
		require.NoError(t, err)
		s := NewServer(nopNotifications{}, c, new(sync.Mutex))

		content := "main"
		callServerEdit(t, ctx, s, resource, content, false, false)

		locs := []textapi.Location{
			{From: term.Coordinates{X: 0, Y: 0}, To: term.Coordinates{X: 3, Y: 0}},
			{From: term.Coordinates{X: 1, Y: 4}, To: term.Coordinates{X: 2, Y: 4}},
			{From: term.Coordinates{X: 0, Y: 5}, To: term.Coordinates{X: 0, Y: 6}},
		}

		var wg sync.WaitGroup
		n := 100
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				l := text.LocationSlice(locs)

				req := makeLocationListRequest(resource, textapi.LocationPriorityWarning, locID, l)
				res, err := s.SetLocationList(ctx, &req)
				if !assert.NoError(t, err) {
					return
				}
				if !assert.NotNil(t, res) {
					return
				}
			}()
		}

		wg.Wait()
	})
}

func TestServerSetCursor(t *testing.T) {
	t.Run("calls underlying editor SetCursor", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, s := newTestServer(t, ctrl)

		resource, err := workspaceapi.ParseURI("file:///SetCursorer")
		require.NoError(t, err)
		content := "Oh my"
		expectEdit(t, mock, resource, content, true, true)
		callServerEdit(t, ctx, s, resource, content, true, true)

		pos := term.Coordinates{X: 4, Y: 5}
		h := expectEditor(t, ctrl, mock, resource)
		h.EXPECT().SetCursorAtScroll(gomock.Eq(pos)).Times(1)

		var protoPos termrpc.Coordinates
		protoPos.FromModel(pos)

		req := textrpc.SetCursorRequest{ResourceName: NewURI(resource), Pos: &protoPos}
		res, err := s.SetCursor(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)
	})
}

func TestServerCursor(t *testing.T) {
	t.Run("calls underlying editor Cursor", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mock, s := newTestServer(t, ctrl)

		resource, err := workspaceapi.ParseURI("file:///Cursorer")
		require.NoError(t, err)
		expectEdit(t, mock, resource, "", false, false)
		callServerEdit(t, ctx, s, resource, "", false, false)

		pos := term.Coordinates{X: 4, Y: 5}
		h := expectEditor(t, ctrl, mock, resource)
		h.EXPECT().CursorAtScroll().Return(pos).Times(1)

		req := textrpc.CursorRequest{ResourceName: NewURI(resource)}
		res, err := s.Cursor(ctx, &req)
		require.NoError(t, err)
		require.NotNil(t, res)

		assert.Equal(t, pos, res.GetPos().ToModel())
	})
}
