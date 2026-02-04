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
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// memFile is an in-memory workspaceapi.File implementation.
type memFile struct {
	locker   sync.Locker
	m        *memoryScheme
	reader   memReader
	data     *[]byte // shared amongst symlinks
	filename string
	fd       uintptr
	modTime  time.Time
	mode     os.FileMode
	offset   int64
	link     string
}

// MemoryFileSys is returned in calls to a memory file's os.FileInfo's Sys.
type MemoryFileSys struct {
	Offset int64
}

// NewMemoryFile allocates storage for a new file and initializes it
// with the given filename, file descriptor, mode and initial data.
func NewMemoryFile(
	filename string, fd uintptr, mode fs.FileMode,
	data []byte, locker sync.Locker,
) workspaceapi.File {
	ptr := new([]byte)
	*ptr = data
	return &memFile{
		filename: filename,
		fd:       fd,
		mode:     mode,
		data:     ptr,
		reader:   memReader{s: ptr},
		locker:   locker,
	}
}

// Read satisfies workspaceapi.File.
func (c *memFile) Read(p []byte) (n int, err error) {
	// read could lock indefinetly, do not lock
	return c.reader.read(p)
}

func (c *memFile) ReadAt(p []byte, offset int64) (n int, err error) {
	return c.reader.readAt(p, offset)
}

// Write satisfies workspaceapi.File.
func (c *memFile) Write(p []byte) (n int, err error) {
	c.locker.Lock()
	defer c.locker.Unlock()

	if c.offset+int64(len(p)) > int64(len(*c.data)) {
		diff := c.offset + int64(len(p)) - int64(len(*c.data))
		*c.data = append(*c.data, make([]byte, diff)...)
		copy((*c.data)[diff:], *c.data)
	}
	copy((*c.data)[c.offset:], p)
	n = len(p)
	c.offset += int64(n)

	c.reader.i = 0
	_, err = c.reader.seek(c.offset, io.SeekStart)
	return
}

// Name satisfies workspaceapi.File.
func (c *memFile) Name() string {
	c.locker.Lock()
	defer c.locker.Unlock()

	return c.filename
}

// Stat satisfies workspaceapi.File.
func (c *memFile) Stat() (os.FileInfo, error) {
	c.locker.Lock()
	defer c.locker.Unlock()

	finfo := memFileInfo{
		bufLen:   int64(len(*c.data)),
		filename: filepath.Base(c.filename),
		modTime:  c.modTime,
		mode:     c.mode,
		offset:   c.offset,
	}
	return finfo, nil
}

// Sync satisfies workspaceapi.File.
func (c *memFile) Sync() error {
	c.locker.Lock()

	c.modTime = time.Now()
	if c.m == nil {
		c.locker.Unlock()
		return nil
	}

	watchpoints := c.m.watchpoints[schemeapi.Write]
	uri, _ := c.m.URI(c.filename)
	copied := make([]chan<- schemeapi.EventInfo, len(watchpoints))
	copy(copied, watchpoints)
	c.locker.Unlock()

	for _, wp := range copied {
		fi := watchFileInfo{
			event: schemeapi.Write,
			uri:   uri,
		}
		wp <- fi
	}
	return nil
}

// Truncate satisfies workspaceapi.File.
func (c *memFile) Truncate(size int64) error {
	c.locker.Lock()
	defer c.locker.Unlock()

	if size < 0 || size > int64(len(*c.data)) {
		panic("invalid truncate size")
	}
	*c.data = (*c.data)[:size]
	c.offset = 0
	c.reader.i = 0
	return nil
}

// Fd satisfies workspaceapi.File.
func (c *memFile) Fd() uintptr {
	c.locker.Lock()
	defer c.locker.Unlock()

	return c.fd
}

// Seek satisfies workspaceapi.File.
func (c *memFile) Seek(offset int64, whence int) (int64, error) {
	c.locker.Lock()
	defer c.locker.Unlock()

	var err error
	c.offset, err = c.reader.seek(offset, whence)
	if err != nil {
		return 0, err
	}
	return c.offset, nil
}

// Close satisfies workspaceapi.File.
func (c *memFile) Close() error {
	return nil
}

// memFileInfo is an os.FileInfo implementation
// for an memFile.
type memFileInfo struct {
	bufLen   int64
	filename string
	mode     os.FileMode
	modTime  time.Time
	isDir    bool
	offset   int64
}

// Size satisfies os.FileInfo.
func (t memFileInfo) Size() int64 {
	return t.bufLen
}

// Mode satisfies os.FileInfo.
func (t memFileInfo) Mode() os.FileMode {
	return t.mode
}

// ModTime satisfies os.FileInfo.
func (t memFileInfo) ModTime() time.Time {
	return t.modTime
}

// IsDir satisfies os.FileInfo.
func (t memFileInfo) IsDir() bool {
	return t.isDir
}

// Sys satisfies os.FileInfo.
func (t memFileInfo) Sys() interface{} {
	return MemoryFileSys{Offset: t.offset}
}

// Name satisfies os.FileInfo.
func (t memFileInfo) Name() string {
	return t.filename
}

// Type satisfies os.FileInfo.
func (t memFileInfo) Type() os.FileMode {
	return 0 // 0 is regular files
}

// Info satisfies os.FileInfo.
func (t memFileInfo) Info() (os.FileInfo, error) {
	return t, nil
}

type memReader struct {
	s *[]byte // shared amongst linked files
	i int64   // current reading index
}

func (r *memReader) read(b []byte) (n int, err error) {
	if r.i >= int64(len(*r.s)) {
		return 0, io.EOF
	}
	n = copy(b, (*r.s)[r.i:])
	r.i += int64(n)
	return
}

func (r *memReader) readAt(b []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= int64(len(*r.s)) {
		return 0, io.EOF
	}
	n = copy(b, (*r.s)[off:])
	if n < len(b) {
		err = io.EOF
	}
	return
}

func (r *memReader) seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.i + offset
	case io.SeekEnd:
		abs = int64(len(*r.s)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative position")
	}
	r.i = abs
	return abs, nil
}
