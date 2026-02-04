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

package workspace

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"os"
	"os/user"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

func newTestFileScheme(uri workspaceapi.URI) (*fileScheme, error) {
	ret := new(fileScheme)
	ret.osStat = func(path string) (os.FileInfo, error) {
		if strings.Contains(path, ".txt") || strings.Contains(path, ".md") {
			return testFileInfo{name: path, isDir: false}, nil
		}
		return testFileInfo{name: path, isDir: true}, nil
	}
	ret.getUser = func() (*user.User, error) {
		return &user.User{Username: "git", HomeDir: "/home/git"}, nil
	}
	ret.lookupUser = func(username string) (*user.User, error) {
		return &user.User{Username: username, HomeDir: fmt.Sprintf("/home/%s", username)}, nil
	}
	err := ret.init(config.NopConfig(), uri)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func TestNewScheme(t *testing.T) {
	tsuite := []struct {
		desc         string
		workspaceURI string
		expectedErr  string
	}{
		{"no port no user workspace absolute returns error", "file://ernest.photography", "invalid file URI"},
		{"port no user workspace absolute returns error", "file://ernest.photography:4222", "invalid file URI"},
		{"port user workspace absolute returns error", "file://ernie@ernest.photography:4222", "invalid file URI"},
		{"port user workspace absolute slash returns error", "file://ernie@ernest.photography:4222/", "invalid file URI"},
		{"no port user workspace absolute slash returns error", "file://ernie@ernest.photography/", "invalid file URI"},
		{"different scheme returns error", "ssh:///tmp", "invalid file URI"},
		{"no host regular folder success", "file:///tmp", ""},
		{"root", "file:///", ""},
		{"file uri should return error", "file:///tmp/file.txt",
			"workspaceapi.URI does not refer to a directory: file:///tmp/file.txt"}, // newTestFileScheme sets osStat based on file name
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			workspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			// sut
			_, err = newTestFileScheme(workspaceURI)
			if tcase.expectedErr != "" {
				assert.EqualError(t, err, tcase.expectedErr)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}

func TestFileSchemeURI(t *testing.T) {
	tsuite := []struct {
		desc         string
		workspaceURI string
		inPath       string
		expectedOut  string
		expectedErr  string
	}{
		{"absolute root", "file:///", "/",
			"file:///", ""},
		{"absolute root, non-root workspace", "file:///home/ernicles", "/",
			"file:///", ""},
		{"absolute root 2", "file:///var", "/tmp/file",
			"file:///tmp/file", ""},
		{"absolute folder is equal to workspace", "file:///home/ernicles", "/home/ernicles",
			"file:///home/ernicles", ""},
		{"absolute folder file", "file:///home/ernicles", "/home/ernicles/file.txt",
			"file:///home/ernicles/file.txt", ""},
		{"absolute other folder file", "file:///home/ernicles", "/home/git/file.txt",
			"file:///home/git/file.txt", ""},
		{"relative file", "file:///home/ernicles", "file.txt",
			"file:///home/ernicles/file.txt", ""},
		{"relative file to home, non workspace", "file:///home/src/blue", "~/file.txt",
			"file:///home/git/file.txt", ""},
		{"relative file to home, workspace", "file:///home/git/", "~/file.txt",
			"file:///home/git/file.txt", ""},
		{"relative upwards workspace tree", "file:///home/git/src/blue", "../../",
			"file:///home/git/src/blue/../../", ""},
		{"relative upwards workspace tree ending slash", "file:///home/git/src/blue/", "../../",
			"file:///home/git/src/blue/../../", ""},
		{"relative upwards workspace tree ./ path", "file:///home/git/src/blue", "./../../file.txt",
			"file:///home/git/src/blue/./../../file.txt", ""},
	}

	for _, tcase := range tsuite {
		t.Run(tcase.desc, func(t *testing.T) {
			workspaceURI, err := workspaceapi.ParseURI(tcase.workspaceURI)
			require.NoError(t, err)

			s, err := newTestFileScheme(workspaceURI)
			require.NoError(t, err)
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

func TestFileAssumptions(t *testing.T) {
	t.Run("Write overwrites data", func(t *testing.T) {
		f, err := os.CreateTemp("", "")
		name := f.Name()
		n, err := f.Write([]byte("12345"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		f, err = os.OpenFile(name, os.O_RDWR, 0)
		require.NoError(t, err)
		n, err = f.Write([]byte("ZZ"))
		require.NoError(t, err)
		assert.Equal(t, 2, n)

		nn, err := f.Seek(0, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(0), nn)

		data, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "ZZ345", string(data))
	})

	t.Run("Read uses write offset", func(t *testing.T) {
		f, err := os.CreateTemp("", "")
		name := f.Name()
		n, err := f.Write([]byte("12345"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		f, err = os.OpenFile(name, os.O_RDWR, 0)
		require.NoError(t, err)

		n, err = f.Write([]byte("ZZ"))
		require.NoError(t, err)
		assert.Equal(t, 2, n)

		data, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "345", string(data))
	})
}

func TestStartCommand(t *testing.T) {
	t.Run("Cmd.Dir makes command run on that directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		ch := make(chan error)
		ctx := context.Background()

		// If Dir is passed runs there
		cmd := workspaceapi.Cmd{
			Path:    "/bin/sh",
			Args:    []string{"-c", "pwd"},
			Dir:     "/bin",
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)

		err = <-ch
		assert.Equal(t, err, nil)

		assert.Equal(t, stderr.String(), "")
		assert.Equal(t, stdout.String(), "/bin\n")

		stderr.Reset()
		stdout.Reset()

	})

	t.Run("omitting Cmd.Dir makes command run on workspace dir", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "")
		require.NoError(t, err)

		uri, err := workspaceapi.ParseURI("file://" + tmpDir)
		require.NoError(t, err)

		s, err := newTestFileScheme(uri)
		require.NoError(t, err)

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		ch := make(chan error)
		ctx := context.Background()

		cmd := workspaceapi.Cmd{
			Path:    "/bin/sh",
			Args:    []string{"-c", "pwd"},
			Watcher: workspaceapi.ChanProcessWatcher(ch),
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		_, err = s.StartCommand(ctx, cmd)
		require.NoError(t, err)

		err = <-ch
		assert.Equal(t, err, nil)

		assert.Equal(t, stderr.String(), "")
		assert.Equal(t, stdout.String(), tmpDir+"\n")
	})
}
