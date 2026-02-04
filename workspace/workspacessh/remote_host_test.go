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
	"errors"
	"io"

	"os"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi/workspacerpc"
	"google.golang.org/grpc"
	"unstable.build/go-tui/workspace"
	tworkspacerpc "unstable.build/go-tui/workspace/workspacerpc"
)

var logger = log.New()

func init() {
	logger.Out = io.Discard
}

func TestReaderWriterListener(t *testing.T) {
	t.Run("one accept", func(t *testing.T) {
		inRead, inWrite, err := os.Pipe()
		require.NoError(t, err)

		outRead, outWrite, err := os.Pipe()
		require.NoError(t, err)

		l := newStdioListener(logger, inRead, outWrite, false, func() {})

		_, err = inWrite.WriteString("JJ")
		require.NoError(t, err)

		// sut
		conn, err := l.Accept()
		require.NoError(t, err)

		n, err := conn.Write([]byte("\n"))
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		var buf [3]byte
		n, err = io.ReadAtLeast(outRead, buf[:], 1)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		assert.Equal(t, "\n", string(buf[:1]))

		n, err = conn.Read(buf[:])
		require.NoError(t, err)
		require.Equal(t, 2, n)
		assert.Equal(t, "JJ", string(buf[:2]))

		err = conn.Close()
		require.NoError(t, err)

		t.Cleanup(func() {
			inRead.Close()
			inWrite.Close()
			outWrite.Close()
			outRead.Close()
		})
	})

	t.Run("close of stdio returns", func(t *testing.T) {
		grpcServer := grpc.NewServer()
		inRead, inWrite, err := os.Pipe()
		require.NoError(t, err)

		outRead, outWrite, err := os.Pipe()
		require.NoError(t, err)

		lis := newStdioListener(logger, inRead, outWrite, false, func() {
			go grpcServer.Stop()
		})
		uri, err := workspaceapi.ParseURI("memory:///")
		require.NoError(t, err)
		scheme, err := workspace.NewMemoryScheme(context.Background(), config.NopConfig(), uri)
		require.NoError(t, err)

		server := tworkspacerpc.NewServer(scheme, new(sync.Mutex))
		workspacerpc.RegisterSchemeServer(grpcServer, server)
		go func() {
			inWrite.Close()
		}()
		grpcServer.Serve(lis)

		t.Cleanup(func() {
			inRead.Close()
			inWrite.Close()
			outWrite.Close()
			outRead.Close()
		})
	})
}

type closer struct {
	mu     sync.Mutex
	closed bool
}

func (c *closer) Read(p []byte) (n int, err error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return 0, io.EOF
	}
	// timeout tests
	time.Sleep(300 * time.Minute)
	return 0, io.EOF
}

func (c *closer) Write(p []byte) (n int, err error) {
	return 0, errors.New("nope")
}

func (c *closer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}
