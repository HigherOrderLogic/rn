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
	"os"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

// InvalidFile returns a File that always returns the given err.
// This can be used by Scheme implementations to satisfy NewFile
// when callers passed an invalid file descriptor.
func InvalidFile(fd uintptr, name string, err error) workspaceapi.File {
	return &invalidFile{fd: fd, name: name, err: err}
}

type invalidFile struct {
	fd   uintptr
	name string
	err  error
}

func (c *invalidFile) Read(p []byte) (n int, err error) {
	return 0, c.err
}

func (c *invalidFile) ReadAt(p []byte, offset int64) (n int, err error) {
	return 0, c.err
}

func (c *invalidFile) Write(p []byte) (n int, err error) {
	return 0, c.err
}

func (c *invalidFile) Name() string {
	return c.name
}

func (c *invalidFile) Stat() (os.FileInfo, error) {
	return nil, c.err
}

func (c *invalidFile) Sync() error {
	return c.err
}

func (c *invalidFile) Truncate(size int64) error {
	return c.err
}

func (c *invalidFile) Fd() uintptr {
	return c.fd
}

func (c *invalidFile) Seek(offset int64, whence int) (int64, error) {
	return 0, c.err
}

func (c *invalidFile) Close() error {
	return c.err
}
