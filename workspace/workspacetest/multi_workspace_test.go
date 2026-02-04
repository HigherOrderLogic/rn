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

package workspacetest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/workspace"
)

func TestMultiWorkspace(t *testing.T) {
	ctx := context.Background()

	tsuite := []struct {
		desc               string
		defURI             workspaceapi.URI
		fileURI            workspaceapi.URI
		recover            bool
		wantError          bool
		expectAddWorkspace workspaceapi.URI
	}{
		{"should load files in the default workspace",
			parseURI(t, "memory:///"), parseURI(t, "memory:///file.txt"), false, false, workspaceapi.URI{}},
		{"should recover files in the default workspace",
			parseURI(t, "memory:///"), parseURI(t, "memory:///file.txt"), true, false, workspaceapi.URI{}},
		{"should load files in a registered non-default workspace and should call AddWorkspace with dir URI",
			parseURI(t, "memory:///"), parseURI(t, "test:///file.txt"), false, false, parseURI(t, "test:///")},
		{"should recover files in a registered non-default workspace and should call AddWorkspace with dir URI",
			parseURI(t, "memory:///"), parseURI(t, "test:///file.txt"), true, false, parseURI(t, "test:///")},
		{"should not load files in a non-registered non-default workspace",
			parseURI(t, "memory:///"), parseURI(t, "nagging:///file.txt"), false, true, workspaceapi.URI{}},
		{"should not recover files in a non-registered non-default workspace",
			parseURI(t, "memory:///"), parseURI(t, "nagging:///file.txt"), true, true, workspaceapi.URI{}},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			memScheme, err := workspace.NewMemoryScheme(ctx, config.NopConfig(), tcase.defURI)
			require.NoError(t, err)
			cwd := workspace.NewSchemeWorkspace(tcase.defURI, memScheme)
			mockManager := &mockManager{}
			cwd = workspace.Multi(ctx, mockManager, cwd, tcase.defURI)

			if tcase.recover {
				swapFile := fmt.Sprintf("%s.swp", tcase.fileURI.Path())
				_, werr := memScheme.OpenFile(swapFile, os.O_CREATE, 0)
				require.Nil(t, werr)
				swapFileURI := parseURI(t, fmt.Sprintf("%s.swp", tcase.fileURI.String()))
				_, err = cwd.Recover(tcase.fileURI, swapFileURI, cell.NewBuffer(), false)
			} else {
				_, err = cwd.Load(tcase.fileURI, cell.NewBuffer(), workspaceapi.Dir(tcase.fileURI), false)
			}
			if tcase.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tcase.expectAddWorkspace != (workspaceapi.URI{}) {
				require.Len(t, mockManager.addWorkspace, 1)
				assert.Equal(t, mockManager.addWorkspace[0], tcase.expectAddWorkspace)
			}
		})
	}
}

type mockManager struct {
	addWorkspace []workspaceapi.URI
}

func (m *mockManager) RegisterScheme(string, schemeapi.SchemeFunc) error {
	panic("should not be called")
}
func (m *mockManager) UnregisterScheme(string) error {
	panic("should not be called")
}
func (m *mockManager) AddWorkspace(ctx context.Context, uri workspaceapi.URI) (
	workspace.Workspace, error,
) {
	if uri.Scheme() != "test" {
		return nil, errors.New("not registered")
	}
	m.addWorkspace = append(m.addWorkspace, uri)
	scheme, err := NewNopScheme(uri.Scheme())(ctx, config.NopConfig(), uri)
	if err != nil {
		return nil, err
	}
	return workspace.NewSchemeWorkspace(uri, scheme), nil
}
