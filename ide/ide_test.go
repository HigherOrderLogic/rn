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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/blue/release"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
	"unstable.build/go-tui/component/shader"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/idepkg/idepkgtest"
)

func TestIDEInitializationIntegration(t *testing.T) {
	t.Run("does not panic with sample config", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)
		err := os.WriteFile(configFile.Name(), []byte(sampleConfig), 0666)
		require.NoError(t, err)

		cwdURI, err := workspaceapi.CurrentUserHostURI(".")
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(cwdURI.String(), configFile.Name(), dir,
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("does not panic with empty config", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

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
		err = i.init(cwdURI.String(), configFile.Name(),
			dir, WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("takes a non-URI as a workspace", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		err := os.WriteFile(configFile.Name(), []byte("{}"), 0666)
		require.NoError(t, err)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init(".", configFile.Name(), dir,
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("creates non-existing directories for log file", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		configData := fmt.Sprintf("log_path: %s/bla/bla/bla/debug.log", dir)
		err = os.WriteFile(configFile.Name(), []byte(configData), 0666)
		require.NoError(t, err)

		i := new(IDE)
		err = i.init(".", configFile.Name(), dir,
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.workspace)
		require.NotNil(t, i.clipboard)

		assert.NoError(t, i.closeResources())
	})

	t.Run("is able to initialize without a cwd", func(t *testing.T) {
		configFile, _ := makeTestFiles(t)

		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})

		i := new(IDE)
		err = i.init("", configFile.Name(), dir,
			WithPublishEvent(nopPublishEvent),
			WithExtensionsRunner(FuncExtensionsRunner(testRunnerFn)),
			WithLocker(new(sync.Mutex)))
		require.NoError(t, err)

		require.NotNil(t, i.root)
		assert.NoError(t, i.closeResources())
	})

	t.Run("init shader is run when passed WithInitShader option", func(t *testing.T) {
		_, config := makeTestFiles(t)
		initShader := new(mockShader)
		i := new(IDE)
		err := i.init("", "", filepath.Dir(config.Name()),
			WithInitShader(
				func(_ term.Attributes, _ component.FrameCharSet) shader.Shader {
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
	t.Parallel()
	assertURI := func(t *testing.T, i *IDE, expected workspaceapi.URI) {
		ex := i.workspaceHandler.exHandler(i.workspaceHandler.focusHandler())
		uri, _, ok := ex.handlerInFocus()
		require.True(t, ok)
		assert.Equal(t, expected, uri)
	}

	t.Run("empty workspace", func(t *testing.T) {
		t.Parallel()
		file, config := makeTestFiles(t)
		i, err := New("", config.Name(), filepath.Dir(config.Name()))
		require.NoError(t, err)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)
	})

	t.Run("a workspace", func(t *testing.T) {
		t.Parallel()
		file, config := makeTestFiles(t)
		i, err := New(os.TempDir(), config.Name(), filepath.Dir(config.Name()))
		require.NoError(t, err)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)
	})

	t.Run("syntax enabled, empty workspace", func(t *testing.T) {
		t.Parallel()
		pkgs := idepkgtest.MakePackages(
			release.Package{Name: "go", Latest: "3"},
		)
		bundles := idepkgtest.MakeBundles(
			[]release.Bundle{
				{Package: "go", Version: "3"},
			},
		)
		file, config := makeTestFiles(t)
		rm := idepkgtest.NewReleaseManager(pkgs, bundles)
		i, err := New("", config.Name(), filepath.Dir(config.Name()), WithReleaseManager(rm))
		require.NoError(t, err)
		uri, err := workspaceapi.CurrentUserHostURI(file.Name())
		require.NoError(t, err)

		logrus.SetLevel(logrus.TraceLevel)

		i.workspaceHandler.mu.Lock()
		defer i.workspaceHandler.mu.Unlock()

		require.NoError(t, i.Open(uri))
		assertURI(t, i, uri)

		// allow for syntax to unpack things
		time.Sleep(200 * time.Millisecond)
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
	configFile, err := os.CreateTemp("", "six_ide_test.*.yaml")
	require.NoError(t, err)

	_, err = configFile.WriteString("{}")
	require.NoError(t, err)

	require.NoError(t, configFile.Close())

	file, err := os.CreateTemp("", "six_ide_test.*.go")
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

func (r testRunner) StartCommand(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	return 0, nil
}

func (r testRunner) Signal(workspaceapi.Pid, syscall.Signal) error {
	return nil
}
