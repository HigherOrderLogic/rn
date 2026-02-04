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

package workspacessh

import (
	"context"
	"fmt"

	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/workspacetest"
)

type nopExecutor struct {
}

func (n nopExecutor) StartCommand(
	ctx context.Context, cmd workspaceapi.Cmd,
) (workspaceapi.Pid, error) {
	return 0, nil
}

func (n nopExecutor) Signal(workspaceapi.Pid, syscall.Signal) error {
	return nil
}
func (n nopExecutor) Close() error {
	return nil
}

type nopRemote struct {
}

func (n nopRemote) NewSession() (schemeapi.Executor, error) {
	return nopExecutor{}, nil
}

func (n nopRemote) Close() error {
	return nil
}

type testFileInfo struct {
	name string
}

func (t testFileInfo) Name() string {
	return t.name
}
func (t testFileInfo) Size() int64 {
	return 0
}

func (t testFileInfo) Mode() os.FileMode {
	return 0
}

func (t testFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (t testFileInfo) IsDir() bool {
	return !strings.Contains(t.name, ".")
}

func (t testFileInfo) Sys() interface{} {
	return nil
}

func newTestScheme(
	cfg config.Config, workspaceURI workspaceapi.URI,
	connectSchemeFn func(ctx context.Context,
		uri workspaceapi.URI, closeHook func(error)) (schemeapi.Scheme, error),
) (schemeapi.Scheme, error) {
	s := new(scheme)
	s.ctx, s.cancelCtx = context.WithCancel(context.Background())
	s.remoteFn = func(context.Context, sshConfig, workspaceapi.URI) (remote, error) {
		return nopRemote{}, nil
	}
	s.getUser = func() (*user.User, error) {
		return &user.User{Username: "git", HomeDir: "/home/git"}, nil
	}
	if connectSchemeFn == nil {
		connectSchemeFn = func(ctx context.Context, uri workspaceapi.URI, closeHook func(error)) (
			schemeapi.Scheme, error,
		) {
			return workspacetest.NewNopScheme("test")(ctx, config.NopConfig(), uri)
		}
	}
	s.connectSchemeFn = connectSchemeFn

	err := s.init(context.Background(), sshConfig{}, workspaceURI,
		func(_ schemeapi.Scheme, name string) (os.FileInfo, error) {
			return testFileInfo{name: name}, nil
		})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func newNopScheme(t *testing.T, workspaceURI workspaceapi.URI) *scheme {
	s, err := newTestScheme(config.NopConfig(), workspaceURI, nil)
	require.NoError(t, err)
	return s.(*scheme)
}

func TestNewScheme(t *testing.T) {
	tsuite := []struct {
		desc         string
		workspaceURI string
		expectedURI  string
		expectedErr  string
	}{
		{"no port no user workspace absolute", "ssh://ernest.photography", "", ""},
		{"port no user workspace absolute", "ssh://ernest.photography:4222", "", ""},
		{"port user workspace absolute", "ssh://ernie@ernest.photography:4222", "", ""},
		{"port user workspace absolute slash", "ssh://ernie@ernest.photography:4222/", "", ""},
		{"no port user workspace absolute slash", "ssh://ernie@ernest.photography/", "", ""},
		{"no port user workspace relative slash", "ssh://ernie@ernest.photography/~/src",
			"ssh://ernie@ernest.photography/home/ernie/src", ""},
		{"no host returns error", "ssh:///tmp", "", "could not parse ssh workspaceapi.URI: ssh scheme with empty host is invalid"},
		{"different scheme returns error", "file:///tmp", "", "invalid non-ssh scheme"},
		{"file URI returns error", "ssh://ernie@ernest.photography/tmp/file.txt", "",
			"workspaceapi.URI does not refer to a directory: ssh://ernie@ernest.photography/tmp/file.txt"},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			workspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			// sut
			ch := make(chan string, 1) // when uri is a file URI it gets called twice
			s, err := newTestScheme(config.NopConfig(), workspaceURI,
				func(ctx context.Context, uri workspaceapi.URI, closeHook func(error)) (schemeapi.Scheme, error) {
					go func() { ch <- uri.String() }()
					return workspacetest.NewNopScheme("test")(
						ctx, config.NopConfig(), uri)
				})
			if tcase.expectedErr != "" {
				assert.EqualError(t, err, tcase.expectedErr)
			} else {
				assert.NoError(t, err)
				expectedURI := tcase.expectedURI
				if expectedURI == "" {
					expectedURI = workspaceURI.String()
				}
				actualURI := <-ch
				assert.Equal(t, expectedURI, actualURI)
				assert.NoError(t, s.Close())
			}
		})
	}
}

func TestURI(t *testing.T) {
	tsuite := []struct {
		desc         string
		workspaceURI string
		inPath       string
		expectedOut  string
		expectedErr  string
	}{
		{"no port no user workspace absolute", "ssh://ernest.photography", "/",
			"ssh://ernest.photography/", ""},
		{"no port no user workspace home relative", "ssh://ernest.photography", "/~/",
			"ssh://ernest.photography/home/git", ""}, // newTestScheme sets git as default user
		{"port no user workspace home relative", "ssh://ernest.photography:455", "/~/",
			"ssh://ernest.photography:455/home/git", ""},
		{"port no user workspace absolute", "ssh://ernest.photography:455", "/tmp/var",
			"ssh://ernest.photography:455/tmp/var", ""},
		{"port user workspace absolute", "ssh://ernie@ernest.photography:455", "/tmp/var",
			"ssh://ernie@ernest.photography:455/tmp/var", ""},
		{"port user workspace home relative", "ssh://ernie@ernest.photography:455", "~/src/blue",
			"ssh://ernie@ernest.photography:455/home/ernie/src/blue", ""},
		{"relative common path implicit home folder", "ssh://ernest.photography/home/git/src/blue", "~/src/blue/fireplace.txt",
			"ssh://ernest.photography/home/git/src/blue/fireplace.txt", ""},
		{"relative common path explicit home folder", "ssh://ernest.photography/home/git/src/blue", "/home/git/src/blue/fireplace.txt",
			"ssh://ernest.photography/home/git/src/blue/fireplace.txt", ""},
		{"relative upwards workspace tree", "ssh://ernest.photography/home/git/src/blue", "../../",
			"ssh://ernest.photography/home/git", ""},
		{"relative upwards workspace tree ending slash", "ssh://ernest.photography/home/git/src/blue/", "../../",
			"ssh://ernest.photography/home/git", ""},
		{"relative upwards workspace tree ./ path", "ssh://ernest.photography/home/git/src/blue", "./../../file.txt",
			"ssh://ernest.photography/home/git/file.txt", ""},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			workspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			s := newNopScheme(t, workspaceURI)
			defer s.Close()

			// sut
			actualOut, actualErr := s.URI(tcase.inPath)
			if tcase.expectedErr != "" {
				assert.Error(t, actualErr)
				assert.Nil(t, actualOut)
			} else {
				assert.NoError(t, actualErr)

				expectedURI, err := workspaceapi.ParseURI(tcase.expectedOut)
				require.NoError(t, err)
				assert.Equal(t, expectedURI.String(), actualOut.String())
			}

		})
	}
}

func TestIntegrationIsWorkspaceURI(t *testing.T) {
	tsuite := []struct {
		workspaceURI string
		uri          string
		expectedOut  bool
	}{
		{"ssh://ernest.photography/", "ssh://ernest.photography/tmp", true},
		{"ssh://ernest.photography/", "file://ernest.photography/tmp", false},
		{"ssh://ernest.photography/", "ssh://git@ernest.photography/tmp", false},
		{"ssh://unstablebuild@ernest.photography/", "ssh://git@ernest.photography/tmp", false},
		{"ssh://git@ernest.photography/", "ssh://git@ernest.photography/tmp", true},
		{"ssh://unstablebuild@ernest.photography/", "ssh://git@ernest.photography/tmp", false},
		{"ssh://git@ernest.photography/", "ssh://git@ernest.photography:12222/tmp", false},
		{"ssh://git@ernest.photography:12222/", "ssh://git@ernest.photography/tmp", false},
		{"ssh://ernest.photography/tmp", "ssh://ernest.photography/tmp", true},
		{"ssh://ernest.photography:122/tmp", "ssh://ernest.photography:122/tmp", true},
		{"ssh://git@ernest.photography:122/tmp", "ssh://ernest.photography:122/tmp", false},
		{"ssh://ernest.photography:122/tmp", "ssh://git@ernest.photography:122/tmp", false},
		{"ssh://ernest.photography/var", "ssh://ernest.photography/tmp", true}, // diff tree, but scheme should be able to handle it
		{"ssh://ernest.photography/var", "ssh://ernest.photography/var/file.txt", true},
		{"ssh://ernest.photography/var", "ssh://ernest.photography/var/dir/dir/dir/file.txt", true},
		{"ssh://ernest.photography:22/var", "ssh://ernest.photography:22/var/dir/dir/dir/file.txt", true},
		{"ssh://ernest.photography/var/", "ssh://ernest.photography/var/file.txt", true},
		{"ssh://root@ernest.photography/var/", "ssh://root@ernest.photography/var/file.txt", true},
		{"ssh://root@ernest.photography:1999/var/", "ssh://root@ernest.photography:1999/var/file.txt", true},
	}

	for i, tcase := range tsuite {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			inURI, err := workspaceapi.ParseURI(tcase.uri)
			require.NoError(t, err)

			inWorkspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			fileScheme := newNopScheme(t, inWorkspaceURI)
			inWorkspace := workspace.NewSchemeWorkspace(inWorkspaceURI, fileScheme)

			// sut
			actual, err := workspace.IsWorkspaceURI(inWorkspace, inURI)
			require.NoError(t, err)
			assert.Equal(t, tcase.expectedOut, actual)
		})
	}
}

func TestIntegrationManagerIsWorkspaceFile(t *testing.T) {
	fileWorkspacePath, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	fileWorkspaceURI, err := workspaceapi.ParseURI(filepath.Join("file://", fileWorkspacePath))
	require.NoError(t, err)

	sshWorkspaceURI, err := workspaceapi.ParseURI("ssh://ernest.photography/~/")
	require.NoError(t, err)

	manager := workspace.NewManager(config.NopConfig())
	require.NoError(t, manager.RegisterScheme(workspace.FileScheme, workspace.NewFileScheme))
	require.NoError(t, manager.RegisterScheme(Scheme,
		func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (schemeapi.Scheme, error) {
			return newTestScheme(cfg, uri, nil)
		}))

	ctx := context.Background()

	fileWorkspace, err := manager.AddWorkspace(ctx, fileWorkspaceURI)
	require.NoError(t, err)

	sshWorkspace, err := manager.AddWorkspace(ctx, sshWorkspaceURI)
	require.NoError(t, err)

	tsuite := []struct {
		uri               string
		expectedWorkspace workspace.Workspace
		expectedFound     bool
	}{
		{"ssh://ernest.photography/~/hello.txt", sshWorkspace, true},
		{"ssh://unstable.build/~/hello.txt", nil, false},
		{"file:///hello.txt", fileWorkspace, true},
		{"file:///home/git/hello.txt", fileWorkspace, true},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.uri, func(t *testing.T) {
			inURI, err := workspaceapi.ParseURI(tcase.uri)
			require.NoError(t, err)

			// sut
			actual, ok, err := manager.Workspace(inURI)
			require.NoError(t, err)
			assert.Equal(t, ok, tcase.expectedFound)
			assert.Equal(t, tcase.expectedWorkspace, actual)
		})
	}
}

func TestSSHScheme(t *testing.T) {
	var cleanup []func() error
	t.Run("with memory scheme remote", func(t *testing.T) {
		workspacetest.TestWorkspaceSchemeFiles(t, func(t *testing.T) schemeapi.Scheme {
			workspaceURI, err := workspaceapi.ParseURI("ssh://test@host.com/")
			require.NoError(t, err)
			remoteURI, err := workspaceapi.ParseURI("memory:///")
			require.NoError(t, err)

			memScheme, err := workspace.NewMemoryScheme(
				context.Background(), config.NopConfig(), remoteURI)
			require.NoError(t, err)

			s, err := newTestScheme(config.NopConfig(), workspaceURI,
				func(ctx context.Context, uri workspaceapi.URI,
					closeHook func(error)) (schemeapi.Scheme, error) {
					return memScheme, nil
				})
			require.NoError(t, err)
			cleanup = append(cleanup, s.Close)
			return s
		})
	})

	t.Run("with file scheme remote", func(t *testing.T) {
		workspacetest.TestWorkspaceSchemeFiles(t, func(t *testing.T) schemeapi.Scheme {
			dir, err := os.MkdirTemp("", "ssh_scheme_suite")
			require.NoError(t, err)

			fileURI, err := workspaceapi.ParseURI("file://" + dir)
			require.NoError(t, err)

			workspaceURI, err := workspaceapi.ParseURI("ssh://host.com" + dir)
			require.NoError(t, err)

			fileScheme, err := workspace.NewFileScheme(
				context.Background(), config.NopConfig(), fileURI)
			require.NoError(t, err)

			s, err := newTestScheme(config.NopConfig(), workspaceURI,
				func(ctx context.Context, uri workspaceapi.URI,
					closeHook func(error)) (schemeapi.Scheme, error) {
					return fileScheme, nil
				})
			require.NoError(t, err)
			cleanup = append(cleanup, func() (ret error) {
				if err := s.Close(); err != nil {
					ret = multierr.Append(ret, err)
				}
				if err := os.RemoveAll(dir); err != nil {
					ret = multierr.Append(ret, err)
				}
				return ret
			})
			return s
		})
	})
	for _, clean := range cleanup {
		_ = clean()
	}
}
