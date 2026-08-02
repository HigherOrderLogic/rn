// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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
	"context"
	"os"

	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
)

var _ schemeapi.Scheme = (*remoteChroot)(nil)

// remoteChroot keeps a chrooted view on the parent's open-file registry.
// The SDK chroot client hands back raw files with their finalizers armed,
// so without this wrapper every descriptor opened through a chrooted view
// (gogit chroots the workspace for every diff) is closed by the GC behind
// our back, against a descriptor number the remote may have recycled.
type remoteChroot struct {
	schemeapi.Scheme
	parent     *remoteScheme
	generation uint64
}

func (c *remoteChroot) track(f workspaceapi.File, err error) (workspaceapi.File, error) {
	if err != nil {
		return nil, err
	}
	return c.parent.track(f, c.generation), nil
}

func (c *remoteChroot) Open(filename string) (workspaceapi.File, error) {
	return c.track(c.Scheme.Open(filename))
}

func (c *remoteChroot) Create(filename string) (workspaceapi.File, error) {
	return c.track(c.Scheme.Create(filename))
}

func (c *remoteChroot) OpenFile(filename string, flag int, perm os.FileMode) (
	workspaceapi.File, error,
) {
	return c.track(c.Scheme.OpenFile(filename, flag, perm))
}

func (c *remoteChroot) TempFile(dir, prefix string) (workspaceapi.File, error) {
	return c.track(c.Scheme.TempFile(dir, prefix))
}

func (c *remoteChroot) NewPty(ctx context.Context) (workspaceapi.Pty, error) {
	pty, err := c.Scheme.NewPty(ctx)
	if err == nil {
		pty.Master = c.parent.track(pty.Master, c.generation)
		pty.Slave = c.parent.track(pty.Slave, c.generation)
	}
	return pty, err
}

func (c *remoteChroot) NewFile(fd uintptr, filename string) workspaceapi.File {
	return c.parent.lookupFile(c.generation, fd, filename)
}

func (c *remoteChroot) Chroot(path string) (schemeapi.Scheme, error) {
	sub, err := c.Scheme.Chroot(path)
	if err != nil {
		return nil, err
	}
	return &remoteChroot{Scheme: sub, parent: c.parent, generation: c.generation}, nil
}
