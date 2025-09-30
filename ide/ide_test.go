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
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/extensionapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/term"
)

func TestIDEInitializationIntegration(t *testing.T) {
	t.Run("does not panic with sample config", func(t *testing.T) {
		configFile, file1 := makeTestFiles(t)
		file2, err := os.CreateTemp("", "six_ide_test")
		require.NoError(t, err)
		require.NoError(t, file2.Close())

		err = os.WriteFile(configFile.Name(), []byte(sampleConfig), 0666)
		require.NoError(t, err)

		cwdURI, err := workspaceapi.CurrentUserHostURI(".")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(cwdURI.String(), configFile.Name(), "",
			dir, []string{file1.Name(), file2.Name()},
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("does not panic with empty config", func(t *testing.T) {
		configFile, file1 := makeTestFiles(t)

		err := os.WriteFile(configFile.Name(), []byte("{}"), 0666)
		require.NoError(t, err)

		cwdURI, err := workspaceapi.CurrentUserHostURI(".")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(cwdURI.String(), configFile.Name(), "",
			dir, []string{file1.Name()},
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("takes a non-URI as a workspace", func(t *testing.T) {
		configFile, file1 := makeTestFiles(t)

		err := os.WriteFile(configFile.Name(), []byte("{}"), 0666)
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(".", configFile.Name(), "",
			dir, []string{file1.Name()},
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("does not publish an event before run is called", func(t *testing.T) {
		configFile, file1 := makeTestFiles(t)
		file2, err := os.CreateTemp("", "six_ide_test")
		require.NoError(t, err)
		require.NoError(t, file2.Close())

		err = os.WriteFile(configFile.Name(), []byte(sampleConfig), 0666)
		require.NoError(t, err)

		cwdURI, err := workspaceapi.CurrentUserHostURI(".")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		var published bool
		i := new(IDE)
		err = i.init(cwdURI.String(), configFile.Name(), "",
			dir, []string{file1.Name(), file2.Name()},
			WithPublishEvent(func(ev term.Event) bool {
				published = true
				return false
			}),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		assert.False(t, i.publishEvent(term.Event{}))
		assert.False(t, published)

		assert.NoError(t, i.closeResources())
	})

	t.Run("creates non-existing directories for log file", func(t *testing.T) {
		configFile, file1 := makeTestFiles(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		configData := fmt.Sprintf("log_path: %s/bla/bla/bla/debug.log", dir)
		err = os.WriteFile(configFile.Name(), []byte(configData), 0666)
		require.NoError(t, err)

		i := new(IDE)
		err = i.init(".", configFile.Name(), "",
			dir, []string{file1.Name()},
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("is able to initialize without a cwd", func(t *testing.T) {
		configFile, file1 := makeTestFiles(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init("", configFile.Name(), "",
			dir, []string{file1.Name()},
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.root)
		assert.NoError(t, i.closeResources())
	})

	t.Run("init shader is run when passed WithInitShader option", func(t *testing.T) {
		initShader := new(mockShader)
		i := new(IDE)
		err := i.init("", "", "", "datadir", []string{""},
			WithInitShader(
				func(_ term.Attributes) shader.Shader {
					return initShader
				},
				30, 1*time.Second,
			))
		require.NoError(t, err)

		i.initRunning()
		i.root.Draw(&term.NoopWriter{})

		assert.True(t, initShader.called)
		assert.NoError(t, i.closeResources())
	})
}

func TestOpen(t *testing.T) {
	assertURI := func(t *testing.T, i *IDE, expected workspaceapi.URI) {
		ex := i.workspaceHandler.exHandler(i.workspaceHandler.focusHandler())
		uri, _, ok := ex.handlerInFocus()
		require.True(t, ok)
		assert.Equal(t, expected, uri)
	}

	t.Run("empty workspace", func(t *testing.T) {
		i, err := New("", "", "datadir", nil)
		require.NoError(t, err)
		file, _ := makeTestFiles(t)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)
	})

	t.Run("a workspace", func(t *testing.T) {
		i, err := New(os.TempDir(), "", "datadir", nil)
		require.NoError(t, err)
		file, _ := makeTestFiles(t)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)
	})
}

type mockShader struct {
	called bool
	frames []int
}

func (s *mockShader) Shade(frame, total int, in [][]term.Cell) {
	s.called = true
	s.frames = append(s.frames, frame)
}

func makeTestFiles(t *testing.T) (*os.File, *os.File) {
	configFile, err := os.CreateTemp("", "six_ide_test")
	require.NoError(t, err)
	require.NoError(t, configFile.Close())

	file, err := os.CreateTemp("", "six_ide_test")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	t.Cleanup(func() {
		_ = os.Remove(configFile.Name())
		_ = os.Remove(file.Name())
	})

	return configFile, file
}

func testRunnerFn(
	uri workspaceapi.URI,
	res map[extensionapi.Permission]extension.ResourceRegistrar,
	dataDir string, n browser.Notifications) (extension.Runner, error) {
	return testRunner{}, nil
}

type testRunner struct {
}

func (r testRunner) Run(extensionID, path string, config config.Config) error {
	return nil
}

func (r testRunner) Close() error {
	return nil
}
