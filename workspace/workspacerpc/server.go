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
	"io/fs"
	"os"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	grpc "google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"unstable.build/go-tui/api/schemeapi"
	"unstable.build/go-tui/api/workspaceapi"
	"unstable.build/go-tui/debug"
)

var (
	errInvalidFd = errors.New("invalid file descriptor")
)

// Server is a workspace server implementation which processes one request at a time.
type Server struct {
	UnimplementedSchemeServer
	UnimplementedExecutorServer
	UnimplementedTerminalServer
	UnimplementedFilesServer
	ctx       context.Context
	cancelCtx func()

	locker      sync.Locker
	s           schemeapi.Scheme
	watchpoints map[int]func()
}

// NewServer allocates storage for a new server and initializes it with wp.
func NewServer(wp schemeapi.Scheme, locker sync.Locker) *Server {
	ret := new(Server)
	ret.Init(wp, locker)
	return ret
}

// Init initializes this Server with the given workspace
func (s *Server) Init(scheme schemeapi.Scheme, locker sync.Locker) {
	s.s = scheme
	s.locker = locker
	s.ctx, s.cancelCtx = context.WithCancel(context.Background())
	s.s = scheme
	s.locker = locker
	s.watchpoints = make(map[int]func())
}

// StartCommand satisfies ExecutorServer
func (s *Server) StartCommand(stream Executor_StartCommandServer) error {
	var req CommandPayload
	err := stream.RecvMsg(&req)
	if err != nil {
		return fmt.Errorf("recv start command msg: %v", err)
	}
	if req.Type != CommandPayload_TypeStart || req.Start == nil {
		return fmt.Errorf("unexpected first stream message: %v", req.Type)
	}
	start := req.Start
	ctx, cancelCtx := context.WithCancel(s.ctx)
	streamer, err := newServerCommandStreamer(
		ctx, cancelCtx, stream, start.GetName(), start.GetDir(), start.GetArgs(), start.GetEnv(),
		start.GetStdin(), start.GetStdout(), start.GetStderr(),
		start.GetStdinFd(), start.GetStdoutFd(), start.GetStderrFd(),
		start.GetStdinName(), start.GetStdoutName(), start.GetStderrName(),
		start.GetSetsid(), start.GetSetctty(),
		s.s,
	)
	if err != nil {
		cancelCtx()
		return fmt.Errorf("new streamer: %v", err)
	}
	defer streamer.Close()

	s.locker.Lock()
	pid, err := s.s.StartCommand(ctx, streamer.command())
	s.locker.Unlock()
	if err != nil {
		cancelCtx()
		s.log(log.WarnLevel, "start command error: %v", err)
		return fmt.Errorf("start command: %v", err)
	}
	go debug.CapturePanicReport(streamer.receiveCommandData)
	err = streamer.sendCommandData(pid)
	s.log(log.DebugLevel, "send command data: err=%v", err)
	return err
}

// Signal satisfies ExecutorServer
func (s *Server) Signal(ctx context.Context, req *SignalRequest) (*SignalResponse, error) {
	pid := req.GetPid()
	signal := req.GetSig()
	s.locker.Lock()
	defer s.locker.Unlock()

	err := s.s.Signal(workspaceapi.Pid(pid), syscall.Signal(signal))
	if err != nil {
		return nil, err
	}
	resp := new(SignalResponse)
	return resp, nil
}

// URI satisfies SchemeServer
func (s *Server) URI(ctx context.Context, req *URIRequest) (
	*URIResponse, error,
) {
	s.locker.Lock()
	defer s.locker.Unlock()

	uri, err := s.s.URI(req.GetPath())
	if err != nil {
		return nil, err
	}
	resp := new(URIResponse)
	resp.Uri = uri.String()
	return resp, nil
}

// ReadDir satisfies SchemeServer.
func (s *Server) ReadDir(ctx context.Context, req *ReadDirRequest) (
	*ReadDirResponse, error,
) {
	root := req.GetRoot()
	s.locker.Lock()
	defer s.locker.Unlock()

	entries, err := s.s.ReadDir(root)
	if err != nil {
		isExist := errors.Is(err, os.ErrExist)
		isNotExist := errors.Is(err, os.ErrNotExist)
		isPermission := errors.Is(err, os.ErrPermission)
		is := isExist || isNotExist || isPermission
		if is {
			resp := new(ReadDirResponse)
			resp.IsExistErr = isExist
			resp.IsNotExistErr = isNotExist
			resp.IsPermissionErr = isPermission
			return resp, nil
		}
		return nil, err
	}
	resp := new(ReadDirResponse)
	rpcEntries := make([]*DirEntry, len(entries))
	for i, entry := range entries {
		rpcEntries[i] = &DirEntry{
			Name:  entry.Name(),
			Mode:  int32(entry.Type()),
			IsDir: entry.IsDir(),
		}
	}
	resp.Path = rpcEntries
	return resp, nil
}

// MkdirAll satisfies SchemeServer.
func (s *Server) MkdirAll(ctx context.Context, req *MkdirAllRequest) (
	*MkdirAllResponse, error,
) {
	path := req.GetPath()
	mode := req.GetMode()
	s.locker.Lock()
	defer s.locker.Unlock()

	err := s.s.MkdirAll(path, fs.FileMode(mode))
	if err != nil {
		isPermission := errors.Is(err, os.ErrPermission)
		if isPermission {
			resp := new(MkdirAllResponse)
			resp.IsPermissionErr = isPermission
			return resp, nil
		}
		return nil, err
	}
	resp := new(MkdirAllResponse)
	return resp, nil
}

// Open satisfies SchemeServer.
func (s *Server) Open(ctx context.Context, req *OpenRequest) (
	*OpenResponse, error,
) {
	filename := req.GetFilename()

	s.locker.Lock()
	defer s.locker.Unlock()

	f, werr := s.s.Open(filename, getOpenRequestFlag(req), os.FileMode(req.GetMode()))
	if werr != nil {
		if werr.IsExist || werr.IsNotExist || werr.IsPermission {
			resp := new(OpenResponse)
			resp.IsExistErr = werr.IsExist
			resp.IsNotExistErr = werr.IsNotExist
			resp.IsPermissionErr = werr.IsPermission
			return resp, nil
		}
		return nil, werr.ToError()
	}

	resp := new(OpenResponse)
	resp.Fd = uint32(f.Fd())
	resp.Filename = f.Name()
	return resp, nil
}

// Remove satisfies SchemeServer.
func (s *Server) Remove(ctx context.Context, req *RemoveRequest) (
	*RemoveResponse, error,
) {
	filename := req.GetFilename()
	s.locker.Lock()
	defer s.locker.Unlock()

	err := s.s.Remove(filename)
	if err != nil {
		isExist := errors.Is(err, os.ErrExist)
		isNotExist := errors.Is(err, os.ErrNotExist)
		isPermission := errors.Is(err, os.ErrPermission)
		is := isExist || isNotExist || isPermission
		if is {
			resp := new(RemoveResponse)
			resp.IsExistErr = isExist
			resp.IsNotExistErr = isNotExist
			resp.IsPermissionErr = isPermission
			return resp, nil
		}
		return nil, err
	}
	return new(RemoveResponse), nil
}

// Rename satisfies SchemeServer.
func (s *Server) Rename(ctx context.Context, req *RenameRequest) (
	*RenameResponse, error,
) {
	filename := req.GetFilename()
	s.locker.Lock()
	defer s.locker.Unlock()

	err := s.s.Rename(filename, req.GetNewfilename())
	if err != nil {
		isExist := errors.Is(err, os.ErrExist)
		isNotExist := errors.Is(err, os.ErrNotExist)
		isPermission := errors.Is(err, os.ErrPermission)
		is := isExist || isNotExist || isPermission
		if is {
			resp := new(RenameResponse)
			resp.IsExistErr = isExist
			resp.IsNotExistErr = isNotExist
			resp.IsPermissionErr = isPermission
			return resp, nil
		}
		return nil, err
	}
	return new(RenameResponse), nil
}

// ReadLink satisfies SchemeServer.
func (s *Server) ReadLink(ctx context.Context, req *ReadLinkRequest) (
	*ReadLinkResponse, error,
) {
	filename := req.GetFilename()
	s.locker.Lock()
	defer s.locker.Unlock()

	fil, err := s.s.ReadLink(filename)
	if err != nil {
		return nil, err
	}
	resp := new(ReadLinkResponse)
	resp.Filename = fil
	return resp, nil
}

// NewPty satisfies SchemeServer.
func (s *Server) NewPty(ctx context.Context, req *NewPtyRequest) (
	*NewPtyResponse, error,
) {
	s.locker.Lock()
	defer s.locker.Unlock()

	pty, err := s.s.NewPty(s.ctx)
	if err != nil {
		return nil, err
	}

	ret := &NewPtyResponse{
		Master:   pty.Master.Name(),
		MasterFd: uint32(pty.Master.Fd()),
		Slave:    pty.Slave.Name(),
		SlaveFd:  uint32(pty.Slave.Fd()),
	}
	return ret, nil
}

// SetPtySize satisfies SchemeServer.
func (s *Server) SetPtySize(ctx context.Context, req *SetPtySizeRequest) (
	*SetPtySizeResponse, error,
) {
	s.locker.Lock()
	defer s.locker.Unlock()

	master := s.s.NewFile(uintptr(req.GetMasterFd()), req.GetMaster())
	if master == nil {
		return nil, errors.New("invalid master pty fd")
	}
	pty := workspaceapi.Pty{
		Master: master,
		// no need to set slave, as it's not used for setting the pty size
	}
	err := s.s.SetPtySize(pty, int(req.GetWidth()), int(req.GetHeight()))
	if err != nil {
		return nil, err
	}
	return new(SetPtySizeResponse), nil
}

// Stop closes all resources associated with this server.
func (s *Server) Stop() error {
	s.cancelCtx()
	return nil
}

// Read satisfies FilesServer
func (s *Server) Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	s.locker.Lock()
	f := s.s.NewFile(uintptr(req.GetFd()), req.GetFilename())
	s.locker.Unlock()
	if f == nil {
		return nil, errInvalidFd
	}
	buf := make([]byte, req.GetN())
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read error: %s", err)
	}
	resp := new(ReadResponse)
	resp.Data = buf[:n]
	resp.N = int64(n)
	resp.IsEof = err == io.EOF
	s.log(log.TraceLevel, "file server read: req=%#v, resp: %#v", req, resp)
	return resp, nil
}

// Write satisfies FilesServer
func (s *Server) Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error) {
	s.locker.Lock()
	f := s.s.NewFile(uintptr(req.GetFd()), req.GetFilename())
	s.locker.Unlock()
	if f == nil {
		return nil, errInvalidFd
	}
	n, err := f.Write([]byte(req.GetData()))
	if err != nil {
		return nil, fmt.Errorf("write error: %s", err)
	}
	resp := new(WriteResponse)
	resp.N = int64(n)
	return resp, nil
}

// Close satisfies FilesServer
func (s *Server) Close(ctx context.Context, req *CloseFileRequest) (*CloseFileResponse, error) {
	s.locker.Lock()
	fd := uintptr(req.GetFd())
	f := s.s.NewFile(fd, req.GetFilename())
	if f == nil {
		// idempotent close
		s.locker.Unlock()
		return new(CloseFileResponse), nil
	}
	// unlock after close, which in some cases
	// might do some cleanups that require synchronization
	err := f.Close()
	s.locker.Unlock()
	if err != nil {
		return nil, fmt.Errorf("close error: %s", err)
	}
	return new(CloseFileResponse), nil
}

// Sync satisfies FilesServer.
func (s *Server) Sync(ctx context.Context, req *SyncRequest) (
	*SyncResponse, error,
) {
	s.locker.Lock()
	f := s.s.NewFile(uintptr(req.GetFd()), req.GetFilename())
	s.locker.Unlock()
	if f == nil {
		return nil, errInvalidFd
	}
	err := f.Sync()
	if err != nil {
		return nil, err
	}
	return new(SyncResponse), nil
}

// Truncate satisfies FilesServer.
func (s *Server) Truncate(ctx context.Context, req *TruncateRequest) (
	*TruncateResponse, error,
) {
	s.locker.Lock()
	f := s.s.NewFile(uintptr(req.GetFd()), req.GetFilename())
	s.locker.Unlock()
	if f == nil {
		return nil, errInvalidFd
	}
	err := f.Truncate(req.GetSize())
	if err != nil {
		return nil, err
	}
	return new(TruncateResponse), nil
}

// Seek satisfies FilesServer.
func (s *Server) Seek(ctx context.Context, req *SeekRequest) (
	*SeekResponse, error,
) {
	s.locker.Lock()
	f := s.s.NewFile(uintptr(req.GetFd()), req.GetFilename())
	s.locker.Unlock()
	if f == nil {
		return nil, errInvalidFd
	}
	newOffset, err := f.Seek(req.GetOffset(), int(req.GetWhence()))
	if err != nil {
		return nil, err
	}
	resp := new(SeekResponse)
	resp.NewOffset = newOffset
	return resp, nil
}

// Stat satisfies FilesServer.
func (s *Server) Stat(ctx context.Context, req *StatRequest) (
	*StatResponse, error,
) {
	var err error
	var fs os.FileInfo
	s.locker.Lock()
	if req.GetLstat() {
		if lstater, ok := s.s.(interface {
			Lstat(string) (os.FileInfo, error)
		}); ok {
			fs, err = lstater.Lstat(req.GetFilename())
		} else {
			err = errors.New("Lstat is not implemented")
		}
	} else {
		fs, err = s.s.Stat(req.GetFilename())
	}
	s.locker.Unlock()
	if err != nil {
		isExist := errors.Is(err, os.ErrExist)
		isNotExist := errors.Is(err, os.ErrNotExist)
		isPermission := errors.Is(err, os.ErrPermission)
		is := isExist || isNotExist || isPermission
		if is {
			resp := new(StatResponse)
			resp.IsExistErr = isExist
			resp.IsNotExistErr = isNotExist
			resp.IsPermissionErr = isPermission
			return resp, nil
		}
		return nil, err
	}
	resp := new(StatResponse)
	resp.Name = fs.Name()
	resp.Size = fs.Size()
	resp.Mode = int32(fs.Mode())
	resp.ModTime = timestamppb.New(fs.ModTime())
	resp.IsDir = fs.IsDir()

	return resp, nil
}

// Watch satisfies SchemeServer.
func (s *Server) Watch(
	req *WatchRequest, stream grpc.ServerStreamingServer[WatchMessage],
) error {
	if req.GetPath() == "" {
		return errors.New("path cannot be empty")
	}
	if len(req.GetEvents()) == 0 {
		return errors.New("events cannot be empty")
	}

	var events []workspaceapi.Event
	for _, ev := range req.GetEvents() {
		events = append(events, workspaceapi.Event(ev))
	}

	ch := make(chan workspaceapi.EventInfo)
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	s.locker.Lock()
	id, err := s.s.Watch(req.GetPath(), ch, events...)
	s.watchpoints[id] = cancel
	s.locker.Unlock()
	if err != nil {
		return err
	}

	defer func() {
		s.locker.Lock()
		defer s.locker.Unlock()
		_ = s.s.StopWatch(id)
	}()

	resp := WatchMessage{
		Type: WatchMessage_TypeResponse,
		Response: &WatchResponse{
			Id: int64(id),
		}}
	if err := stream.Send(&resp); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return nil
			}

			var protoEv Event
			switch ev.Event() {
			case workspaceapi.Create:
				protoEv = Event_Create
			case workspaceapi.Write:
				protoEv = Event_Write
			case workspaceapi.Rename:
				protoEv = Event_Rename
			case workspaceapi.Remove:
				protoEv = Event_Remove
			}
			// this can be an error only in windows
			isDir, _ := ev.IsDir()
			msg := WatchMessage{
				Type: WatchMessage_TypeData,
				Data: &WatchData{
					Uri:   ev.URI().String(),
					Event: protoEv,
					IsDir: isDir,
				}}
			if err := stream.Send(&msg); err != nil {
				return err
			}
		}
	}
}

// StopWatch satisfies SchemeServer.
func (s *Server) StopWatch(ctx context.Context, req *StopWatchRequest) (
	*StopWatchResponse, error,
) {
	s.locker.Lock()
	defer s.locker.Unlock()

	id := int(req.GetId())
	cancel, ok := s.watchpoints[id]
	if !ok {
		return nil, errors.New("watchpoint not found")
	}
	cancel()
	delete(s.watchpoints, id)

	return new(StopWatchResponse), nil
}

func (s *Server) log(
	level log.Level, msg string, args ...interface{},
) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "workspacerpc.Server").
		Logf(level, msg, args...)
}

func getOpenRequestFlag(req *OpenRequest) int {
	var flag int
	if req.O_RDONLY {
		flag = os.O_RDONLY
	} else if req.O_WRONLY {
		flag = os.O_WRONLY
	} else {
		flag = os.O_RDWR
	}

	if req.O_APPEND {
		flag |= os.O_APPEND
	}
	if req.O_CREATE {
		flag |= os.O_CREATE
	}
	if req.O_EXCL {
		flag |= os.O_EXCL
	}
	if req.O_SYNC {
		flag |= os.O_SYNC
	}
	if req.O_TRUNC {
		flag |= os.O_TRUNC
	}
	return flag
}
