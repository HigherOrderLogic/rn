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
	"fmt"
	"path/filepath"

	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

func checkURIRelative(a, b workspaceapi.URI) error {
	if a.Scheme() != b.Scheme() {
		return fmt.Errorf("unexpected different schemes: %s vs %s",
			a.String(), b.String())
	}
	if a.Scheme() == FileScheme {
		return nil
	}
	if a.Host() != b.Host() {
		return fmt.Errorf("unexpected different hosts: %s vs %s",
			a.String(), b.String())
	}
	if a.User() != b.User() {
		return fmt.Errorf("unexpected different users: %s vs %s",
			a.String(), b.String())
	}
	return nil
}

// DefaultSwapFile returns a file's default swap directory in the
// local or remote workspace.
func DefaultSwapFile(swapDir workspaceapi.URI, file workspaceapi.URI) (workspaceapi.URI, error) {
	err := checkURIRelative(swapDir, file)
	if err != nil {
		return workspaceapi.URI{}, err
	}
	_, swapFilePath := swapFileName(swapDir.Path(), file.Path())
	return workspaceapi.WithPath(file, swapFilePath)
}

// DefaultSwapDirectory returns a file's default swap directory in the
// local or remote workspace.
func DefaultSwapDirectory(file workspaceapi.URI) (workspaceapi.URI, error) {
	swapDir, _ := swapFileName(filepath.Dir(file.Path()), file.Path())
	return workspaceapi.WithPath(file, swapDir)
}

// IsWorkspaceURI returns whether this uri can be managed by
// the given workspace. For Scheme implementations that
// do not support user and host/port, this method returns
// true if the URI schemes of the workspace and the supplied uri
// are the same.
func IsWorkspaceURI(workspace Workspace, uri workspaceapi.URI) (bool, error) {
	uriAtWorkspace, err := workspace.URI(uri.Path())
	if err != nil {
		return false, err
	}
	return uri.Scheme() == uriAtWorkspace.Scheme() &&
		uri.Hostname() == uriAtWorkspace.Hostname() &&
		uri.Port() == uriAtWorkspace.Port() &&
		uri.User() == uriAtWorkspace.User(), nil
}

// NewWorkspaceURI expands the given path with the given workspaceapi.URI
// and returns its corresponding URI. See ExpandPathWithURI for more details.
func NewWorkspaceURI(workspace workspaceapi.URI, path string) (workspaceapi.URI, error) {
	absPath, err := workspaceapi.ExpandPathWithURI(path, workspace)
	if err != nil {
		return workspaceapi.URI{}, err
	}

	var uriStr string
	if workspace.User() != "" {
		uriStr = fmt.Sprintf("%s://%s@%s%s", workspace.Scheme(),
			workspace.User(), workspace.Host(), absPath)
	} else {
		uriStr = fmt.Sprintf("%s://%s%s", workspace.Scheme(), workspace.Host(), absPath)
	}
	return workspaceapi.ParseURI(uriStr)
}
