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

package workspace

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// A stale Close(fd, oldName) RPC must not be able to reach a pty that
// happened to inherit the recycled descriptor number.
func TestFileSchemeNewFileStaleNamePreservesPty(t *testing.T) {
	tmpDir := t.TempDir()
	uri, err := makeLocalURI(tmpDir)
	require.NoError(t, err)

	s, err := newTestFileScheme(uri)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	path := tmpDir + "/stale.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	stale, err := s.OpenFile(path, os.O_RDONLY, 0)
	require.NoError(t, err)
	staleFd, staleName := stale.Fd(), stale.Name()
	require.NoError(t, stale.Close())

	pty, err := s.NewPty(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = pty.Master.Close()
		_ = pty.Slave.Close()
	})

	var recycled workspaceapi.File
	switch staleFd {
	case pty.Master.Fd():
		recycled = pty.Master
	case pty.Slave.Fd():
		recycled = pty.Slave
	}
	require.NotNil(t, recycled, "expected the pty to reuse the closed descriptor")

	require.Nil(t, s.NewFile(staleFd, staleName),
		"stale close must not resolve to the pty that reused the descriptor")
	require.Equal(t, recycled, s.NewFile(staleFd, recycled.Name()))
}

func TestFileSchemeNewFileNameGate(t *testing.T) {
	tmpDir := t.TempDir()
	uri, err := makeLocalURI(tmpDir)
	require.NoError(t, err)

	s, err := newTestFileScheme(uri)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	path := tmpDir + "/a.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	f, err := s.OpenFile(path, os.O_RDONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	require.Equal(t, f, s.NewFile(f.Fd(), f.Name()))
	require.Nil(t, s.NewFile(f.Fd(), tmpDir+"/other.txt"))
}
