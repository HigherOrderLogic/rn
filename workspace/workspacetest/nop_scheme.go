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

package workspacetest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// NewNopScheme returns a scheme that does nothing and workspace.Executor API panics.
func NewNopScheme(scheme string) schemeapi.SchemeFunc {
	return func(ctx context.Context, cfg config.Config, uri workspaceapi.URI) (schemeapi.Scheme, error) {
		scheme := &NopScheme{scheme: scheme}
		scheme.OpenFunc = func(name string, flag int, perm os.FileMode) (
			workspaceapi.File, error,
		) {
			return nopFile{}, nil
		}
		scheme.RemoveFunc = func(name string) error {
			return nil
		}
		scheme.RenameFunc = func(oldName, newName string) error {
			return nil
		}
		scheme.StatFunc = func(name string) (os.FileInfo, error) {
			// best effort
			return FileInfo{FileIsDir: !strings.Contains(name, ".")}, nil
		}
		scheme.LstatFunc = func(name string) (os.FileInfo, error) {
			return FileInfo{}, nil
		}
		scheme.NewPtyFunc = func(ctx context.Context) (workspaceapi.Pty, error) {
			return workspaceapi.Pty{
				Master: nopFile{},
				Slave:  nopFile{},
			}, nil
		}
		scheme.StartCommandFunc = func(context.Context, workspaceapi.Cmd) (workspaceapi.Pid, error) {
			return 0, nil
		}
		return scheme, nil
	}
}

// NopScheme is a scheme for testing.
type NopScheme struct {
	scheme           string
	OpenFunc         func(name string, flag int, perm os.FileMode) (workspaceapi.File, error)
	RemoveFunc       func(name string) error
	RenameFunc       func(oldName, newName string) error
	StatFunc         func(name string) (os.FileInfo, error)
	LstatFunc        func(name string) (os.FileInfo, error)
	NewPtyFunc       func(context.Context) (workspaceapi.Pty, error)
	StartCommandFunc func(context.Context, workspaceapi.Cmd) (workspaceapi.Pid, error)
}

// Command satisfies schemeapi.Scheme
func (t *NopScheme) Command(ctx context.Context, name string, arg ...string) (workspaceapi.Pid, error) {
	return t.StartCommandFunc(ctx, workspaceapi.Cmd{Path: name, Args: arg})
}

// StartCommand satisfies schemeapi.Scheme
func (t *NopScheme) StartCommand(ctx context.Context, cmd workspaceapi.Cmd) (
	workspaceapi.Pid, error,
) {
	return t.StartCommandFunc(ctx, cmd)
}

// Signal satisfies schemeapi.Scheme
func (t *NopScheme) Signal(workspaceapi.Pid, syscall.Signal) error {
	return nil
}

// NewFile satisfies schemeapi.Scheme
func (t *NopScheme) NewFile(fd uintptr, name string) workspaceapi.File {
	return nopFile{}
}

// URI satisfies schemeapi.Scheme
func (t *NopScheme) URI(path string) (workspaceapi.URI, error) {
	return workspaceapi.ParseURI(fmt.Sprintf("%s://%s", t.scheme, filepath.Join("/", path)))
}

// OpenFile satisfies schemeapi.Scheme
func (t *NopScheme) OpenFile(path string, flag int, perm os.FileMode) (workspaceapi.File, error) {
	return t.OpenFunc(path, flag, perm)
}

// Create satisfies schemeapi.Scheme
func (t *NopScheme) Create(filename string) (workspaceapi.File, error) {
	return t.OpenFunc(filename, os.O_CREATE|os.O_EXCL, 0)
}

// Open satisfies schemeapi.Scheme
func (t *NopScheme) Open(filename string) (workspaceapi.File, error) {
	return t.OpenFunc(filename, os.O_RDONLY, 0)
}

// Chroot satisfies schemeapi.Scheme
func (t *NopScheme) Chroot(path string) (schemeapi.Scheme, error) {
	return new(NopScheme), nil
}

// Root satisfies schemeapi.Scheme
func (t *NopScheme) Root() string {
	return ""
}

// Symlink satisfies schemeapi.Scheme
func (t *NopScheme) Symlink(target, link string) error {
	panic("unimplemented")
}

// TempFile satisfies schemeapi.Scheme
func (t *NopScheme) TempFile(dir, prefix string) (workspaceapi.File, error) {
	panic("unimplemented")
}

// Join satisfies schemeapi.Scheme
func (t *NopScheme) Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Remove satisfies schemeapi.Scheme
func (t *NopScheme) Remove(path string) error {
	return t.RemoveFunc(path)
}

// Rename satisfies schemeapi.Scheme
func (t *NopScheme) Rename(old, new string) error {
	return t.RenameFunc(old, new)
}

// Stat satisfies schemeapi.Scheme
func (t *NopScheme) Stat(path string) (os.FileInfo, error) {
	return t.StatFunc(path)
}

// NewPty satisfies schemeapi.Scheme
func (t *NopScheme) NewPty(ctx context.Context) (ret workspaceapi.Pty, err error) {
	return t.NewPtyFunc(ctx)
}

// SetPtySize satisfies schemeapi.Scheme
func (t *NopScheme) SetPtySize(p workspaceapi.Pty, width, height int) (err error) {
	return nil
}

// Lstat satisfies schemeapi.Scheme
func (t *NopScheme) Lstat(path string) (os.FileInfo, error) {
	return t.LstatFunc(path)
}

// Readlink satisfies schemeapi.Scheme
func (t *NopScheme) Readlink(path string) (string, error) {
	return path, nil
}

// ReadDir satisfies schemeapi.Scheme
func (t *NopScheme) ReadDir(string) (
	[]os.DirEntry, error,
) {
	panic("unimplemented")
}

// MkdirAll satisfies schemeapi.Scheme
func (t *NopScheme) MkdirAll(path string, perm os.FileMode) error {
	panic("unimplemented")
}

// Watch satisfies schemeapi.Scheme
func (t *NopScheme) Watch(
	path string, c chan<- schemeapi.EventInfo, events ...schemeapi.Event,
) (int, error) {
	panic("unimplemented")
}

// StopWatch satisfies schemeapi.Scheme
func (t *NopScheme) StopWatch(ID int) error {
	panic("unimplemented")
}

// Close satisfies schemeapi.Scheme
func (t *NopScheme) Close() error {
	return nil
}

// NewFile returns a workspaceapi.File that does nothing.
func NewFile() workspaceapi.File {
	return nopFile{}
}

type nopFile struct {
}

func (t nopFile) Name() string {
	return ""
}

func (t nopFile) Stat() (os.FileInfo, error) {
	return nopFileInfo{}, nil
}

func (t nopFile) ReadAt(b []byte, off int64) (int, error) {
	panic("unimplemented")
}

func (t nopFile) Sync() error {
	return nil
}

func (t nopFile) Fd() uintptr {
	return 0
}

func (t nopFile) Truncate(size int64) error {
	return nil
}

func (t nopFile) Seek(x int64, y int) (int64, error) {
	return 0, nil
}

func (t nopFile) Read(b []byte) (int, error) {
	return 0, io.EOF
}

func (t nopFile) Write(b []byte) (int, error) {
	return 0, nil
}

func (t nopFile) Close() error {
	return nil
}

// implements os.FileInfo
type nopFileInfo struct {
	name    string
	isDir   bool
	modTime time.Time
	size    int64
	mode    os.FileMode
}

func (t nopFileInfo) Name() string {
	return t.name
}
func (t nopFileInfo) Size() int64 {
	return t.size
}

func (t nopFileInfo) Mode() os.FileMode {
	return t.mode
}

func (t nopFileInfo) ModTime() time.Time {
	return t.modTime
}

func (t nopFileInfo) IsDir() bool {
	return t.isDir
}

func (t nopFileInfo) Sys() any {
	return nil
}
