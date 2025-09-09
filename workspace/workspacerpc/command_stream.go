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

package workspacerpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"time"

	multierr "github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/bluenet"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
)

var readBufferSize = 1024 * 64

const watcherWaitTimeout = 2 * time.Minute

type serverCommandStreamer struct {
	cmd       workspaceapi.Cmd
	stream    Executor_StartCommandServer
	doneCh    chan error
	stdinCh   chan bluenet.ReadResult
	stdoutCh  chan bluenet.ReadResult
	stderrCh  chan bluenet.ReadResult
	stdinFd   uint32
	stdoutFd  uint32
	stderrFd  uint32
	ctx       context.Context
	cancelCtx func()
	closers   []io.Closer
}

func newServerCommandStreamer(
	ctx context.Context,
	cancelCtx func(),
	stream Executor_StartCommandServer,
	path, dir string, args, env []string,
	stdinSet, stdoutSet, stderrSet bool,
	stdinFd, stdoutFd, stderrFd uint32,
	stdinName, stdoutName, stderrName string,
	setsid, setctty bool,
	scheme schemeapi.Scheme,
) (*serverCommandStreamer, error) {
	doneCh := make(chan error)

	cmd := workspaceapi.Cmd{
		Path:    path,
		Dir:     dir,
		Args:    args,
		Env:     env,
		Watcher: workspaceapi.ChanProcessWatcher(doneCh),
	}
	if setsid || setctty {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: setsid, Setctty: setctty}
	}

	ret := new(serverCommandStreamer)

	// it's important that this channels are not buffered
	// so when Cmd.Wait returns, it means that all the data
	// has been drained from the connection
	stdinCh := make(chan bluenet.ReadResult)
	stdoutCh := make(chan bluenet.ReadResult)
	stderrCh := make(chan bluenet.ReadResult)

	// always set these channels so we don't need to worry
	// about nil conditions below
	ret.stdinCh = stdinCh
	ret.stdoutCh = stdoutCh
	ret.stderrCh = stderrCh

	if stdinSet {
		if stdinFd != 0 {
			cmd.Stdin = scheme.NewFile(uintptr(stdinFd), stdinName)
			if cmd.Stdin == nil {
				return nil, fmt.Errorf("invalid stdin file descriptor: %d", stdinFd)
			}
			ret.stdinFd = stdinFd
		} else {
			stdinOutCh := make(chan bluenet.ReadResult)
			// use ChanConn as io.Writer and io.Reader,
			// which means that addrs can be nil
			stdin := bluenet.ChanConn(nil, nil /* addrs */, stdinCh, stdinOutCh)
			cmd.Stdin = stdin
			ret.closers = append(ret.closers, stdin)
		}
	}

	if stdoutSet {
		if stdoutFd != 0 {
			cmd.Stdout = scheme.NewFile(uintptr(stdoutFd), stdoutName)
			if cmd.Stdout == nil {
				return nil, fmt.Errorf("invalid stdout file descriptor: %d", stdoutFd)
			}
			ret.stdoutFd = stdoutFd
		} else {
			stdoutInCh := make(chan bluenet.ReadResult)
			stdout := bluenet.ChanConn(nil, nil /* addrs */, stdoutInCh, stdoutCh)
			cmd.Stdout = stdout
			ret.closers = append(ret.closers, stdout)
		}
	}

	if stderrSet {
		if stderrFd != 0 {
			cmd.Stderr = scheme.NewFile(uintptr(stderrFd), stderrName)
			if cmd.Stderr == nil {
				return nil, fmt.Errorf("invalid stderr file descriptor: %d", stderrFd)
			}
			ret.stderrFd = stderrFd
		} else {
			stderrInCh := make(chan bluenet.ReadResult)
			stderr := bluenet.ChanConn(nil, nil /* addrs */, stderrInCh, stderrCh)
			cmd.Stderr = stderr
			ret.closers = append(ret.closers, stderr)
		}
	}

	ret.cmd = cmd
	ret.stream = stream
	ret.doneCh = doneCh

	ret.ctx = ctx
	ret.cancelCtx = cancelCtx

	return ret, nil
}

func (s *serverCommandStreamer) log(level log.Level, msg string, args ...interface{}) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{logging.KeyClass: "serverCommandStreamer"}).
		Logf(level, msg, args...)
}

func (s *serverCommandStreamer) receiveCommandData() {
	defer s.log(log.TraceLevel, "done receiving command data")
	defer close(s.stdinCh)

	// propagate context cancel to context passed
	// to command Start; either because client is closing
	// stream, or client context passed to Start canceled.
	go debug.CapturePanicReport(func() {
		<-s.stream.Context().Done()
		s.cancelCtx()
	})

	if s.stdinFd != 0 {
		s.log(log.DebugLevel, "not reading from stdin goroutine: remote file mode")
		return
	}

	for {
		var msg CommandPayload
		err := s.stream.RecvMsg(&msg)
		s.log(log.TraceLevel, "receive msg: err=%v", err)
		if err != nil {
			if err == io.EOF {
				return
			}
			if cerr := s.stream.Context().Err(); cerr != nil {
				return
			}
			select {
			case <-s.ctx.Done():
				return
			default:
				ack := make(chan struct{})
				select {
				case s.stdinCh <- bluenet.ReadResult{Error: err, Ch: ack}:
					<-ack
					continue
				case <-s.ctx.Done():
					return
				}
			}
		}

		var data []byte
		switch msg.Type {
		case CommandPayload_TypeIO:
			io := msg.GetIo()
			switch io.GetType() {
			case CommandPayload_IO_TypeStdin:
				data = io.GetData()
			default:
				err = fmt.Errorf("unexpected IO type received: %v", io.GetType())
			}
		default:
			err = fmt.Errorf("unexpected message type received: %v", msg.Type)
		}

		ack := make(chan struct{})
		select {
		case s.stdinCh <- bluenet.ReadResult{Error: err, Data: data, Ch: ack}:
			<-ack
			s.log(log.TraceLevel,
				"wrote to stdin: err=%v, data=%d", err, len(data))
			continue
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *serverCommandStreamer) streamReadResult(
	res bluenet.ReadResult, t CommandPayload_IO_Type,
) error {
	defer close(res.Ch)

	if res.Error != nil {
		err := s.stream.Send(&CommandPayload{
			Type:  CommandPayload_TypeError,
			Error: res.Error.Error(),
		})
		s.log(log.TraceLevel, "send error msg: err=%v", err)
		if err != nil {
			return fmt.Errorf("send error msg: %v", err)
		}
		return fmt.Errorf("error reading from standard io %v: %v", t, res.Error)
	}
	err := s.stream.Send(&CommandPayload{
		Type: CommandPayload_TypeIO,
		Io:   &CommandPayload_IO{Data: res.Data, Type: t},
	})
	s.log(log.TraceLevel, "send io msg: err=%v", err)
	if err != nil {
		return fmt.Errorf("send io msg: %v", err)
	}
	return nil
}

func (s *serverCommandStreamer) sendCommandData(pid workspaceapi.Pid) error {
	err := s.stream.Send(&CommandPayload{
		Type: CommandPayload_TypeStarted,
		Started: &CommandPayload_Started{
			Pid: int64(pid),
		},
	})
	s.log(log.TraceLevel, "send started msg: err=%v", err)
	if err != nil {
		return fmt.Errorf("send msg started: %v", err)
	}
	for {
		var err error
		select {
		case <-s.ctx.Done():
			err = s.ctx.Err()
		case res, ok := <-s.stdoutCh:
			if s.stdoutFd != 0 {
				s.log(log.ErrorLevel, "read from stdout but using file mode: ok=%v, err=%v, data=%d",
					ok, res.Error, len(res.Data))
				return errors.New("unexpected data in stdout chan")
			}
			s.log(log.TraceLevel, "read from stdout: ok=%v, err=%v, data=%d",
				ok, res.Error, len(res.Data))
			if !ok {
				return nil
			}
			err = s.streamReadResult(res, CommandPayload_IO_TypeStdout)
		case res, ok := <-s.stderrCh:
			if s.stderrFd != 0 {
				s.log(log.ErrorLevel, "read from stderr but using file mode: ok=%v, err=%v, data=%d",
					ok, res.Error, len(res.Data))
				return errors.New("unexpected data in stderr chan")
			}
			s.log(log.TraceLevel, "read from stderr: ok=%v, err=%v, data=%d",
				ok, res.Error, len(res.Data))
			if !ok {
				return nil
			}
			err = s.streamReadResult(res, CommandPayload_IO_TypeStderr)
		case doneErr := <-s.doneCh:
			var errStr string
			if doneErr != nil && doneErr != io.EOF {
				errStr = doneErr.Error()
			}
			err = s.stream.Send(&CommandPayload{
				Type: CommandPayload_TypeDone,
				Done: &CommandPayload_Done{
					ExitError: errStr,
				},
			})
			if err != nil {
				err = fmt.Errorf("send done msg: %v", err)
				s.log(log.WarnLevel, "%v", err)
			} else {
				s.log(log.TraceLevel, "send done msg: success")
			}
			// after receiving Done, client will disconnect
			// and RecvMsg in receiveCommandData will return with canceled error.
			return err
		}
		if err != nil {
			return err
		}
	}
}

func (s *serverCommandStreamer) command() workspaceapi.Cmd {
	return s.cmd
}

func (s *serverCommandStreamer) Close() (ret error) {
	for _, closer := range s.closers {
		if err := closer.Close(); err != nil {
			ret = multierr.Append(ret, err)
		}
	}
	s.cancelCtx()
	return
}

type clientCommandStreamer struct {
	req    *StartCommandRequest
	cmd    workspaceapi.Cmd
	stream Executor_StartCommandClient
	ctx    context.Context
	quitCh chan struct{}
}

func newClientCommandStreamer(
	ctx context.Context,
	req *StartCommandRequest,
	cmd workspaceapi.Cmd,
	stream Executor_StartCommandClient,
) *clientCommandStreamer {
	ret := new(clientCommandStreamer)
	ret.cmd = cmd
	ret.req = req
	ret.stream = stream
	ret.ctx = ctx
	ret.quitCh = make(chan struct{})
	return ret
}

func (s *clientCommandStreamer) waitForPid() (workspaceapi.Pid, error) {
	var msg CommandPayload
	err := s.stream.RecvMsg(&msg)
	s.log(log.TraceLevel, "receive first msg: err=%v", err)
	if err != nil {
		return 0, fmt.Errorf("stream receive msg: %v", err)
	}

	if msg.Type != CommandPayload_TypeStarted || msg.Started == nil {
		return 0, fmt.Errorf("expected stream started msg, found %v", msg.Type)
	}

	return workspaceapi.Pid(msg.Started.Pid), nil
}

func (s *clientCommandStreamer) streamStdin() {
	defer s.stream.CloseSend() // nolint:errcheck

	if s.cmd.Stdin == nil {
		s.log(log.TraceLevel, "no stdin set in cmd, skipping streaming stdin")
		return
	}

	if s.req.StdinFd != 0 {
		s.log(log.TraceLevel, "stdin is a file on the remote server. skipping streaming stdin")
		return
	}

	var err error
	buf := make([]byte, readBufferSize)
	for {
		select {
		case <-s.quitCh:
			return
		case <-s.ctx.Done():
			s.log(log.TraceLevel, "parent context is done")
			return
		default:
		}

		var n int
		n, err = s.cmd.Stdin.Read(buf)
		s.log(log.TraceLevel, "read from stdin: err=%v, data=%d", err, n)
		if err != nil && err != io.EOF {
			break
		}
		serr := s.stream.Send(&CommandPayload{
			Type: CommandPayload_TypeIO,
			Io: &CommandPayload_IO{
				Data: buf[:n],
				Type: CommandPayload_IO_TypeStdin,
			},
		})
		if serr != nil {
			err = fmt.Errorf("stream send: %v", serr)
			break
		}
		if err == io.EOF {
			err = nil
			break
		}
	}

	if err != nil {
		s.log(log.ErrorLevel, "stream stdin stopped with error: %v", err)
	}
}

func (s *clientCommandStreamer) log(level log.Level, msg string, args ...interface{}) {
	log.WithFields(log.Fields{logging.KeyClass: "clientCommandStreamer"}).Logf(level, msg, args...)
}

func (s *clientCommandStreamer) streamCommandData(cancelFn func()) {
	s.log(log.TraceLevel, "streaming command data")
	defer close(s.quitCh)
	defer cancelFn()

	go debug.CapturePanicReport(s.streamStdin)

	var err error
	for {
		var msg CommandPayload
		err = s.stream.RecvMsg(&msg)
		s.log(log.TraceLevel, "receive msg: err=%v", err)
		if err != nil {
			if err == io.EOF {
				break
			}
			err = fmt.Errorf("stream receive msg: %v", err)
			break
		}

		var n int
		switch msg.Type {
		case CommandPayload_TypeIO:
			switch msg.GetIo().GetType() {
			case CommandPayload_IO_TypeStdout:
				if s.cmd.Stdout == nil {
					s.log(log.TraceLevel, "no stdout set in cmd, dropping data")
					continue
				}
				if s.req.StdoutFd != 0 {
					s.log(log.ErrorLevel, "stdout is a file on the remote server but received data over the stream")
					return
				}
				if msg.GetIo().Data == nil {
					s.log(log.WarnLevel, "stdout type without stdout data")
					continue
				}
				_, err = s.cmd.Stdout.Write(msg.GetIo().GetData())
				s.log(log.TraceLevel, "wrote to stdout, err=%v, data=%d", err, len(msg.GetIo().GetData()))
				if err != nil {
					err = fmt.Errorf("stdout io.Writer write: %v", err)
				}
			case CommandPayload_IO_TypeStderr:
				if s.cmd.Stderr == nil {
					s.log(log.TraceLevel, "no stderr set in cmd, dropping data")
					continue
				}
				if s.req.StderrFd != 0 {
					s.log(log.ErrorLevel, "stderr is a file on the remote server but received data over the stream")
					return
				}
				if msg.GetIo().Data == nil {
					s.log(log.WarnLevel, "stderr type without stderr data")
					continue
				}
				n, err = s.cmd.Stderr.Write(msg.GetIo().GetData())
				s.log(log.TraceLevel, "wrote to stderr, err=%v, data=%d", err, n)
				if err != nil {
					err = fmt.Errorf("stderr io.Writer write: %v", err)
				}
			default:
				err = fmt.Errorf("unexpected io message received %v", msg.GetType())
			}
			if err == nil {
				continue
			}
		case CommandPayload_TypeError:
			err = fmt.Errorf("error reading command stdio: %v", msg.GetError())
		case CommandPayload_TypeDone:
			if msg.GetDone().GetExitError() != "" {
				err = errors.New(msg.GetDone().GetExitError())
			}
		default:
			err = fmt.Errorf("unexpected message received %v", msg.GetType())
		}
		break
	}

	// avoid buggy watchers to cause this goroutine to block forever,
	// so the timeout should be in the order of minutes.
	ctx, cancel := context.WithTimeout(context.Background(),
		watcherWaitTimeout)
	defer cancel()

	if s.cmd.Watcher != nil && s.cmd.Watcher.WatchProcess() != nil {
		select {
		case s.cmd.Watcher.WatchProcess() <- err:
		case <-ctx.Done():
			s.log(log.WarnLevel, "could not deliver error to watcher chan: "+
				"watcher not ready for too long")
		}
	}
	// emulate exec code; pipes should be closed to force EOF
	for _, fd := range [2]io.Writer{s.cmd.Stdout, s.cmd.Stderr} {
		if closer, ok := fd.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	s.log(log.TraceLevel, "done streaming command data: err=%v", err)
}
