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

package extutil

import (
	"context"

	"github.com/ernestrc/go-multierror"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
)

// MultiGrantee combines together a series of extension.Grantee, which will share
// Permissions and Grants.
func MultiGrantee(first extension.Grantee, extra ...extension.Grantee) extension.Grantee {
	children := make([]extension.Grantee, len(extra)+1)
	children[0] = first
	copy(children[1:], extra)
	return multiGrantee{children: children}
}

type multiGrantee struct {
	children []extension.Grantee
}

func (m multiGrantee) Connected(
	ctx context.Context, b rpc.MuxBroker, cfg config.Config,
) (ret error) {
	for _, child := range m.children {
		if err := child.Connected(ctx, b, cfg); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return nil
}

func (m multiGrantee) PermissionGranted(
	ctx context.Context, grants []extension.Grant,
) (ret error) {
	for _, child := range m.children {
		if err := child.PermissionGranted(ctx, grants); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return nil
}

func (m multiGrantee) PermissionDenied(
	ctx context.Context, perms []extensionapi.Permission,
) (ret error) {
	for _, child := range m.children {
		if err := child.PermissionDenied(ctx, perms); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return nil
}

func (m multiGrantee) Shutdown(ctx context.Context, reason string) (ret error) {
	for _, child := range m.children {
		if err := child.Shutdown(ctx, reason); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}

func (m multiGrantee) Health(ctx context.Context) (ret error) {
	for _, child := range m.children {
		if err := child.Health(ctx); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return
}
