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

package extensionv2

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
)

// ServeLegacy adapts a v1 extension to run as a v2 extension.
func ServeLegacy(
	id, name, version string, grantee extension.Grantee,
	perms ...extensionapi.Permission,
) {
	shim, metadata := NewGranteeShim(id, name, version, grantee, perms...)
	err := extensionapi.ServeWorkspaceExtension(shim, metadata)
	if err != nil {
		log.Errorf("serve extension %q: %v", id, err)
	}
}

// NewGranteeShim wraps an extension.Grantee to satisfy extensionapi.WorkspaceExtension
// and builds a "builtin" extensionapi.Metadata, meaning it used an internal,
// hardcoded developer ID, key, and email.
func NewGranteeShim(
	id, name, version string, grantee extension.Grantee,
	perms ...extensionapi.Permission,
) (extensionapi.WorkspaceExtension, extensionapi.Metadata) {
	permissions := extensionapi.NewPermissions(perms...)
	cfg := extensionapi.Metadata{
		DeveloperID:      "ox.dev",
		DeveloperEmail:   "ernest@unstable.build",
		DeveloperKey:     "064D4ABCFA6D9338",
		ExtensionID:      id,
		ExtensionName:    name,
		ExtensionVersion: version,
		Permissions:      permissions,
	}

	// clean out duplicates
	perms = perms[:0]
	for perm := range permissions {
		perms = append(perms, perm)
	}

	return granteeShim{
		id:      id,
		perms:   perms,
		grantee: grantee,
	}, cfg
}

type granteeShim struct {
	id      string
	grantee extension.Grantee
	perms   []extensionapi.Permission
}

func (g granteeShim) ExtendWorkspace(
	ctx context.Context, w *extensionapi.Workspace, cfg config.Config,
) error {
	broker := newBroker(w.RawConn())
	m := make(map[string]any)
	cfg.Iterate(func(k string, v any) {
		m[k] = v
	})
	// allows legacy extensions to access data dir
	// this should be deprecated as soon as legacy extensions
	// are refactored.
	m["datadir"] = w.DataDir(ctx)
	cfg = config.MapConfig(m)
	err := g.grantee.Connected(ctx, broker, cfg)
	if err != nil {
		return fmt.Errorf("connected legacy callback: %w", err)
	}

	var grants []extension.Grant
	for _, perm := range g.perms {
		grants = append(grants, extension.Grant{
			Permission: perm,
			Context:    ctx,
		})
	}

	if err := g.grantee.PermissionGranted(ctx, grants); err != nil {
		return fmt.Errorf("permission granted legacy callback: %w", err)
	}

	return nil
}

type broker struct {
	conn grpc.ClientConnInterface
}

func newBroker(conn grpc.ClientConnInterface) rpc.MuxBroker {
	return &broker{conn: conn}
}

func (b *broker) DialChannel(context.Context, string, ...string) (grpc.ClientConnInterface, error) {
	return b.conn, nil
}

func (b *broker) Close() error {
	return nil
}
