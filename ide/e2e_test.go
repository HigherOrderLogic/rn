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

package ide_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extensionv2"
	"unstable.build/go-tui/handler/handlertest"
	"unstable.build/go-tui/ide"
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

		var mu sync.Mutex
		i, err := ide.New(dir, config.Name(), dir, ide.WithLocker(&mu))
		require.NoError(t, err)

		handler := i.Ready()
		mu.Lock()
		require.NoError(t, i.Open(uri))
		mu.Unlock()
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

		mu.Lock()
		defer mu.Unlock()
		handlertest.RunHandlerSequence(t, handler, 20, 10, cases)
	})

	t.Run("plugins executed via ! and !! get auth env vars", func(t *testing.T) {
		t.Parallel()
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		config, err := os.CreateTemp(dir, "bcd")
		require.NoError(t, err)

		_, err = config.Seek(0, 0)
		require.NoError(t, err)
		_, err = config.WriteString(`
editor:
  mode: modal
command:
  key: ":"
  key_bindings:
    <a-m>: windowclose
`)
		require.NoError(t, err)

		var mu sync.Mutex
		runner, err := extensionv2.NewRunner(context.Background(),
			&mu, extension.GrantAll(), dir,
			extensionv2.WithSocketEnv("IDETEST_SOCKET"),
			extensionv2.WithDataDirEnv("IDETEST_DATADIR"),
			extensionv2.WithAuthCertEnv("IDETEST_CERT"),
			extensionv2.WithAuthTokenEnv("IDETEST_TOKEN"),
		)
		require.NoError(t, err)
		i, err := ide.New(dir, config.Name(), dir,
			ide.WithExtensionsRunner(runner),
			ide.WithLocker(&mu),
		)
		require.NoError(t, err)

		handler := i.Ready()

		filename1, err := filepath.Abs(filepath.Join(dir, "ide.env"))
		require.NoError(t, err)
		file1, err := os.Create(filename1)
		require.NoError(t, err)
		require.NoError(t, file1.Close())

		filename2, err := filepath.Abs(filepath.Join(dir, "ide2.env"))
		require.NoError(t, err)
		file2, err := os.Create(filename2)
		require.NoError(t, err)
		require.NoError(t, file2.Close())

		// allow for extension servers to be ready
		time.Sleep(5 * time.Second)

		keys, err := term.ParseKeys(
			fmt.Sprintf(`:!<space>sh<space>-c<space>"env<space>|grep<space>IDETEST<space>|<space>tee<space>%s"<enter>`, filename1) +
				fmt.Sprintf(`:!!<space>sh<space>-c<space>"env<space>|grep<space>IDETEST<space>|<space>tee<space>%s"<enter>`, filename2),
		)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		for _, key := range keys {
			_, handled := handler.Handle(term.Event{Ch: key.Ch, Mod: key.Mod, Key: key.Key, Type: term.EventKey})
			require.True(t, handled, key.String())
		}

		assertAuthVarsPresent(t, filename1)
		assertAuthVarsPresent(t, filename2)
	})
}

func assertAuthVarsPresent(t *testing.T, filename string) {
	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	vars := strings.Split(strings.TrimSuffix(strings.Trim(string(data), " "), "\n"), "\n")
	assert.Equal(t, 4, len(vars))
	for _, v := range vars {
		kv := strings.Split(v, "=")
		switch kv[0] {
		case "IDETEST_SOCKET",
			"IDETEST_DATADIR",
			"IDETEST_TOKEN":
			require.Len(t, kv, 2)
			assert.NotZero(t, kv[1])
		case "IDETEST_CERT":
		default:
			t.Errorf("extraneous idetest env var: %q", kv[0])
		}
	}
}
