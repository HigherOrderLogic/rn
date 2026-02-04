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

package ide

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/handler/handlertest"
)

func TestE2E(t *testing.T) {
	t.Parallel()
	t.Run("sed arg substitution and single quote grouping works", func(t *testing.T) {
		t.Parallel()
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		config, err := os.CreateTemp(dir, "bcd")
		require.NoError(t, err)

		file, err := os.Create(filepath.Join(dir, "e2e.go"))
		require.NoError(t, err)

		_, err = config.Seek(0, 0)
		require.NoError(t, err)
		_, err = config.WriteString(`
editor:
  mode: modal
command:
  key: ":"
  aliases:
    sed: "! gsed -i $1 %"
  key_bindings:
    <a-m>: windowclose
`)
		require.NoError(t, err)

		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		i, err := New(dir, config.Name(), dir)
		require.NoError(t, err)

		handler := i.Ready()
		i.workspaceHandler.mu.Lock()
		require.NoError(t, i.Open(uri))

		i.workspaceHandler.mu.Unlock()
		cases := []handlertest.SequenceTestCase{
			{"ia<space>bc<space>abc<space>ab<space>c<space>abc<esc>:write<enter>",
				`┌──────────────────┐
│o e2e.go          │
├──────────────────┤
│a bc abc ab c ab▐ │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
			{":sed<space>'s/c<space>a/C<space>A/g'<enter><a-m>:reloadfile!<enter>",
				`┌──────────────────┐
│o e2e.go          │
├──────────────────┤
│a bC AbC Ab C Ab▐ │
│                  │
│                  │
│                  │
│                  │
│            NORMAL│
└──────────────────┘`},
		}

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()
		handlertest.RunHandlerSequence(t, handler, 20, 10, cases)
	})
}
