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

package ide

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/document"
	"go.uber.org/mock/gomock"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/texttest"
)

func TestHistory(t *testing.T) {
	testuri, err := workspaceapi.ParseURI("memory:///tmp")
	require.NoError(t, err)

	suite := []struct {
		description string
		sut         func(*testing.T, document.Service, *history)
		svc         document.Service // optional, default is fresh in memory service
	}{
		{
			description: "data is not nil after loading from cold cache",
			sut: func(t *testing.T, svc document.Service, h *history) {
				require.NotNil(t, h.cache)
			},
		},
		{
			description: "data is not nil even after error loading from storage",
			sut: func(t *testing.T, svc document.Service, h *history) {
				require.NotNil(t, h.cache)
			},
			svc: &mockService{err: errors.New("boom")},
		},
		{
			description: "subscribes to editor events",
			sut: func(t *testing.T, svc document.Service, h *history) {
				uri1, err := workspaceapi.ParseURI("memory:///tmp")
				require.NoError(t, err)

				ed := texttest.NopEditor()
				h.recordAddWorkspace(uri1, ed, true)

				ed.Edit(testuri, cell.NewBuffer())

				assertFilesInCache(t, h, uri1, 1)
			},
		},
		{
			description: "recordAddWorkspace clears cache if restore is set to false",
			sut: func(t *testing.T, svc document.Service, h *history) {
				uri1, err := workspaceapi.ParseURI("memory:///tmp")
				require.NoError(t, err)

				ed := texttest.NopEditor()
				h.recordAddWorkspace(uri1, ed, true)
				ed.Edit(testuri, cell.NewBuffer())
				h.recordCloseWorkspace(uri1)
				assert.Len(t, h.recordAddWorkspace(uri1, ed, false), 0)

				assertFilesInCache(t, h, uri1, 0)
			},
		},
		{
			description: "handles open file events even with errors in cache",
			sut: func(t *testing.T, svc document.Service, h *history) {
				uri1, err := workspaceapi.ParseURI("memory:///tmp")
				require.NoError(t, err)

				ed := texttest.NopEditor()
				h.recordAddWorkspace(uri1, ed, true)

				ed.Edit(testuri, cell.NewBuffer())

				assertFilesInCache(t, h, uri1, 1)
			},
			svc: &mockService{err: errors.New("boom")},
		},
		{
			description: "cleans cache when closing events",
			sut: func(t *testing.T, svc document.Service, h *history) {
				uri1, err := workspaceapi.ParseURI("memory:///tmp")
				require.NoError(t, err)

				ed := texttest.NopEditor()
				h.recordAddWorkspace(uri1, ed, true)
				ed.Edit(testuri, cell.NewBuffer())
				h.recordCloseWorkspace(uri1)

				assertFilesInStorage(t, h, uri1, 1)
				_, ok := h.cache[uri1.String()]
				require.False(t, ok)
			},
		},
		{
			description: "recordAddWorkspace returns the previous session's open files",
			sut: func(t *testing.T, svc document.Service, h *history) {
				uri1, err := workspaceapi.ParseURI("memory:///tmp")
				require.NoError(t, err)
				ed := texttest.NopEditor()

				h.recordAddWorkspace(uri1, ed, true)
				ed.Edit(testuri, cell.NewBuffer())
				h.recordCloseWorkspace(uri1)
				assert.Len(t, h.recordAddWorkspace(uri1, ed, true), 1)
			},
		},
	}

	for _, test := range suite {
		test := test
		t.Run(test.description, func(t *testing.T) {
			svc := test.svc
			if svc == nil {
				svc = document.NewInMemoryService()
			}
			h := newHistory(svc)
			test.sut(t, svc, h)
		})
	}

	t.Run("does not panic if storage is corrupted", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("memory:///tmp")
		require.NoError(t, err)
		svc := document.NewInMemoryService()
		require.NoError(t, svc.Set(context.Background(), uri.String(),
			map[string]any{"Files": "yikes"}))

		ed := texttest.NopEditor()
		h := newHistory(svc)
		assert.NotPanics(t, func() {
			h.recordAddWorkspace(uri, ed, true)
			ed.Edit(testuri, cell.NewBuffer())
		})
	})

	t.Run("does not assume file is open when EventTypeFlush is received", func(t *testing.T) {
		uri, err := workspaceapi.ParseURI("memory:///tmp")
		require.NoError(t, err)
		svc := document.NewInMemoryService()

		ctrl := gomock.NewController(t)
		ed := texttest.NewMockEditor(ctrl)
		h := newHistory(svc)

		var workspaceHandler text.EventHandler
		ed.EXPECT().SubscribeEvents(gomock.Any(), gomock.Any()).
			DoAndReturn(func(evs []textapi.EventType, sub text.EventHandler) bool {
				workspaceHandler = sub
				return false
			}).Times(2)
		h.recordAddWorkspace(uri, ed, true)

		workspaceHandler.Handle(context.Background(), textapi.Event{
			Type:    textapi.EventTypeFlush,
			URI:     testuri,
			Content: "abvc",
		})
		h.recordCloseWorkspace(uri)
		assert.Len(t, h.recordAddWorkspace(uri, ed, true), 0)
	})
}

type mockService struct {
	err error
	svc document.Service
}

func (s *mockService) Create(ctx context.Context, ID string, doc interface{}) error {
	if s.err != nil {
		return s.err
	}
	return s.svc.Create(ctx, ID, doc)
}
func (s *mockService) Set(ctx context.Context, ID string, doc interface{}) error {
	if s.err != nil {
		return s.err
	}
	return s.svc.Set(ctx, ID, doc)
}
func (s *mockService) Update(ctx context.Context, ID string,
	updates []document.Update, precond ...document.Precondition) error {
	if s.err != nil {
		return s.err
	}
	return s.svc.Update(ctx, ID, updates, precond...)
}
func (s *mockService) Get(ctx context.Context, ID string, doc interface{}) error {
	if s.err != nil {
		return s.err
	}
	return s.svc.Get(ctx, ID, doc)
}
func (s *mockService) Delete(ctx context.Context, ID string) error {
	if s.err != nil {
		return s.err
	}
	return s.svc.Delete(ctx, ID)
}
func (s *mockService) List(ctx context.Context, filters []document.Filter) (
	document.Iterator, error,
) {
	if s.err != nil {
		return nil, s.err
	}
	return s.svc.List(ctx, filters)
}
func (s *mockService) Close() error {
	if s.err != nil {
		return s.err
	}
	return s.svc.Close()
}

func assertFilesInCache(t *testing.T, h *history, wuri workspaceapi.URI, n int) {
	_, ok := h.cache[wuri.String()]
	require.True(t, ok)
	assert.Len(t, h.cache[wuri.String()].Files, n)

	if mock, ok := h.svc.(*mockService); ok {
		if mock.err != nil {
			return // do not test in storage
		}
	}
	assertFilesInStorage(t, h, wuri, n)
}

func assertFilesInStorage(t *testing.T, h *history, wuri workspaceapi.URI, n int) {
	var cache cache
	require.NoError(t, h.svc.Get(context.Background(), wuri.String(), &cache))
	assert.Len(t, cache.Files, n)
}
