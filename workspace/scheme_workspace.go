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
	"fmt"
	"os"

	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/cell"
)

// simple Scheme-backed Workspace implementation.
type schemeWorkspace struct {
	w workspaceapi.URI
	schemeapi.Scheme
}

// NewSchemeWorkspace wraps a schemeapi.Scheme and implements a workspace.Loader,
// effectively converting a schemeapi.Scheme into a workspace.Workspace.
func NewSchemeWorkspace(w workspaceapi.URI, p schemeapi.Scheme) Workspace {
	ret := new(schemeWorkspace)
	ret.Init(w, p)
	return ret
}

func (w *schemeWorkspace) Init(uri workspaceapi.URI, p schemeapi.Scheme) {
	w.w = uri
	w.Scheme = p
}

func (w *schemeWorkspace) Recover(
	uri, swapURI workspaceapi.URI, buf *cell.Buffer, force bool,
) (ret FlusherCloser, err error) {
	// force cleanup and expansion of URI paths
	// but first check if it's from this workspace
	is, err := IsWorkspaceURI(w, uri)
	if err != nil {
		err = fmt.Errorf("IsWorkspaceURI: %s", err)
		return
	}
	if !is {
		err = fmt.Errorf("invalid URI %q for workspace with URI %q", uri, w.w)
		return
	}
	is, err = IsWorkspaceURI(w, swapURI)
	if err != nil {
		err = fmt.Errorf("IsWorkspaceURI: %s", err)
		return
	}
	if !is {
		err = fmt.Errorf("invalid URI %q for workspace with URI %q", swapURI, w.w)
		return
	}

	// turn into relative if possible
	path := workspaceapi.RelPath(w.w, uri)
	swapPath := workspaceapi.RelPath(w.w, swapURI)

	ret, err = newFileRecover(w.Scheme, path, swapPath, buf, force)
	return
}

func (w *schemeWorkspace) Load(
	uri workspaceapi.URI, buf *cell.Buffer, swapDir workspaceapi.URI, readOnly bool,
) (ret FlusherCloser, err error) {
	// force cleanup and expansion of URI paths
	// but first check if it's from this workspace
	is, err := IsWorkspaceURI(w, uri)
	if err != nil {
		err = fmt.Errorf("IsWorkspaceURI: %s", err)
		return
	}
	if !is {
		err = fmt.Errorf("invalid file URI %q for workspace with URI %q", uri, w.w)
		return
	}
	is, err = IsWorkspaceURI(w, swapDir)
	if err != nil {
		err = fmt.Errorf("IsWorkspaceURI: %s", err)
		return
	}
	if !is {
		err = fmt.Errorf("invalid file URI %q for workspace with URI %q", swapDir, w.w)
		return
	}

	// turn into relative if possible
	path := workspaceapi.RelPath(w.w, uri)
	swapDirPath := workspaceapi.RelPath(w.w, swapDir)

	ret, err = newFile(w.Scheme, path, buf, swapDirPath, readOnly)
	if err == os.ErrNotExist {
		if readOnly {
			err = errors.New("cannot open file that doesn't exist in read-only")
		} else {
			err = errors.New("directory structure does not support creating file")
		}
	}
	return
}
