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

package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace/walkdir"
)

// ErrNotInstalled is returned when a package is not installed.
var ErrNotInstalled = errors.New("package is not installed")

// PkgManager provides a view of the packages installed.
type PkgManager struct {
	fs      workspaceapi.FileSystem
	cwd     workspaceapi.URI
	dataDir string
}

// NewPkgManager allocates storage for a new PkgManager and initializes it.
func NewPkgManager(
	dataDir string, cwd workspaceapi.URI, fs workspaceapi.FileSystem,
) *PkgManager {
	return &PkgManager{
		fs:      fs,
		cwd:     cwd,
		dataDir: dataDir,
	}
}

// LibDir returns an iterator to the lib directory of the given package.
// The paths returned by the iterator are always absolute.
func (m *PkgManager) LibDir(ctx context.Context, pkgID string) (
	iterator.Iterator[string], error,
) {
	libDir := makePackageLibDirname(m.dataDir, pkgID)

	_, err := os.Stat(libDir)
	if err != nil {
		return nil, ErrNotInstalled
	}

	return getFiles(ctx, m.fs, m.cwd, libDir), nil
}

func getFiles(
	ctx context.Context, cwd workspaceapi.FileSystem,
	schemeURI workspaceapi.URI, libDir string,
) iterator.Iterator[string] {
	it, err := walkdir.ListFiles(ctx, cwd, libDir)
	if err != nil {
		return iterator.Error[string](fmt.Errorf("list files: %v", err))
	}
	// make paths absolute
	return iterator.Map(it, func(filename string) string {
		path, _ := workspaceapi.ExpandPathWithURI(filename, schemeURI)
		return path
	})
}

func makePackageLibDirname(
	dataDir, pkgID string,
) string {
	return filepath.Join(dataDir, "lib", pkgID)
}
