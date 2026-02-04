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
	"errors"
	"fmt"
	"net"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi/workspacerpc"
	"google.golang.org/grpc"
	tworkspacerpc "unstable.build/go-tui/workspace/workspacerpc"
)

// StartSchemeServer installs server to handle incoming workspacerpc requests
// over the calling process' os.Stdin and sends responses over os.Stdout.
func StartSchemeServer(
	logger *log.Logger, server *tworkspacerpc.Server,
	grpcServer *grpc.Server,
) error {
	lis := newStdioListener(
		logger, nil /*reader*/, nil, /*writer*/
		true, /* use stdio instead of reader and writer */
		func() {
			logger.Debugf("connection closed unexpectedly")
			go grpcServer.Stop()
		})
	workspacerpc.RegisterSchemeServer(grpcServer, server)
	workspacerpc.RegisterFilesServer(grpcServer, server)
	workspacerpc.RegisterExecutorServer(grpcServer, server)
	workspacerpc.RegisterTerminalServer(grpcServer, server)
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

type stdioListener struct {
	writer   *os.File
	reader   *os.File
	acceptCh chan struct{}
	onlyConn net.Conn
	onClose  func()
	stdio    bool
	addr     net.Addr
	logger   *log.Logger
}

func newStdioListener(
	logger *log.Logger,
	reader, writer *os.File,
	stdio bool,
	onClose func(),
) *stdioListener {
	ret := new(stdioListener)
	ret.reader = reader
	ret.logger = logger
	ret.writer = writer
	ret.stdio = stdio
	ret.addr = newStdioAddr("localaddr")
	ret.acceptCh = make(chan struct{}, 1)
	ret.acceptCh <- struct{}{}
	ret.onClose = onClose
	return ret
}

func (lis *stdioListener) Accept() (net.Conn, error) {
	_, ok := <-lis.acceptCh
	if !ok {
		return nil, errors.New("closed listener")
	}

	conn, err := newStdConn(lis.logger, lis.reader, lis.writer,
		lis.stdio, lis.onClose)
	if err != nil {
		return nil, err
	}
	lis.onlyConn = conn

	return lis.onlyConn, nil
}

func (lis *stdioListener) Close() error {
	close(lis.acceptCh)
	return nil
}

func (lis *stdioListener) Addr() net.Addr {
	return lis.addr
}

type stdioAddr struct {
	s string
}

func newStdioAddr(s string) *stdioAddr {
	return &stdioAddr{s}
}

func (a *stdioAddr) Network() string {
	return "stdio"
}

func (a *stdioAddr) String() string {
	return a.s
}
