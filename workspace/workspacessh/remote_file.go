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
	"os"
	"runtime"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

var (
	_ workspaceapi.File = (*remoteFile)(nil)

	errInvalidFd = errors.New("invalid file descriptor")
)

type remoteFile struct {
	fd       uintptr
	filename string
	scheme   *remoteScheme
}

func newRemoteFile(s *remoteScheme, fd uintptr, filename string) *remoteFile {
	ret := &remoteFile{scheme: s, fd: fd, filename: filename}
	runtime.SetFinalizer(ret, func(f *remoteFile) {
		f.Close()
	})
	return ret
}

func (c *remoteFile) newFile() (workspaceapi.File, error) {
	err, scheme := c.scheme.state()
	if err != nil {
		return nil, err
	}
	f := scheme.NewFile(c.fd, c.filename)
	if f == nil {
		return nil, errInvalidFd
	}
	// the returned file is transient so GC should
	// not close the remote file.
	runtime.SetFinalizer(f, nil)
	return f, nil
}

func (c *remoteFile) Read(p []byte) (n int, err error) {
	f, err := c.newFile()
	if err != nil {
		return 0, err
	}
	return f.Read(p)
}

func (c *remoteFile) ReadAt(p []byte, offset int64) (n int, err error) {
	f, err := c.newFile()
	if err != nil {
		return 0, err
	}
	return f.ReadAt(p, offset)
}

func (c *remoteFile) Write(p []byte) (n int, err error) {
	f, err := c.newFile()
	if err != nil {
		return 0, err
	}
	return f.Write(p)
}

func (c *remoteFile) Name() string {
	return c.filename
}

func (c *remoteFile) Stat() (os.FileInfo, error) {
	f, err := c.newFile()
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

func (c *remoteFile) Sync() error {
	f, err := c.newFile()
	if err != nil {
		return err
	}
	return f.Sync()
}

func (c *remoteFile) Truncate(size int64) error {
	f, err := c.newFile()
	if err != nil {
		return err
	}
	return f.Truncate(size)
}

func (c *remoteFile) Fd() uintptr {
	return c.fd
}

func (c *remoteFile) Seek(offset int64, whence int) (int64, error) {
	f, err := c.newFile()
	if err != nil {
		return 0, err
	}
	return f.Seek(offset, whence)
}

func (c *remoteFile) Close() error {
	f, err := c.newFile()
	if err != nil {
		return err
	}
	runtime.SetFinalizer(c, nil)
	c.scheme.files.Delete(c.Fd())
	return f.Close()
}
