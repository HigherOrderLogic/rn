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

	blueauth "github.com/unstablebuild/blue/auth"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
)

// Extension represents an authenticated extension which is
// associated with some access to some resources.
type Extension struct {
	extensionapi.Metadata
}

// newAuthorizer returns an auth.Authorizer of Extension. It uses the permissions
// in the granted claims to authorize access to a resource.
func newAuthorizer() blueauth.Authorizer[Extension] {
	return authorizer{}
}

type authorizer struct{}

func (a authorizer) Authorize(
	ctx context.Context, claims blueauth.UserClaims[Extension], resource string,
) (err error) {
	var perm extensionapi.Permission
	switch resource {
	case "/workspace.Terminal/NewPty":
		perm = extensionapi.PermissionTerminal
	case "/workspace.Terminal/SetPtySize":
		perm = extensionapi.PermissionTerminal
	case "/workspace.Scheme/URI":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/OpenFile":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Open":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Create":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Remove":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Rename":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Stat":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/ReadLink": // in .proto is defined as ReadLink, not Readlink
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/ReadDir":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Root":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Join":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/TempFile":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Symlink":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/Chroot":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Scheme/MkdirAll":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Executor/StartCommand":
		perm = extensionapi.PermissionExecute
	case "/workspace.Executor/Signal":
		perm = extensionapi.PermissionExecute
	case "/workspace.Files/Sync":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Files/Truncate":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Files/Seek":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Files/Close":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Files/Read":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Files/Write":
		perm = extensionapi.PermissionFileSystem
	case "/workspace.Files/Stat":
		perm = extensionapi.PermissionFileSystem
	case "/browser.ResourceOpener/Open":
		perm = extensionapi.PermissionBrowserResourceOpener
	case "/browser.Notifications/Notify":
		perm = extensionapi.PermissionNotifications
	case "/browser.Notifications/NotifyOnce":
		perm = extensionapi.PermissionNotifications
	case "/browser.Notifications/UpdateNotificationProgress":
		perm = extensionapi.PermissionNotifications
	case "/browser.EventPublisher/Publish":
		perm = extensionapi.PermissionInterrupt
	case "/browser.WindowManager/Focus":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.WindowManager/Split":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.WindowManager/Bar":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.WindowManager/Floating":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.WindowManager/Tab":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.WindowManager/SetContent":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.WindowManager/CloseWindow":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/browser.Floating/Dimensions":
		perm = extensionapi.PermissionBrowserWindowManager
	case "/config.Config/Get":
		perm = extensionapi.PermissionConfig
	case "/text.Editor/Edit":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/SetCursor":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/Cursor":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/Editor":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/SetLocationList":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/MoveToNextLocation":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/MoveToPrevLocation":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/EditCell":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/RawCells":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/SetDefaultAttributes":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/SubscribeEvent":
		perm = extensionapi.PermissionEditor
	case "/text.Editor/SubscribeCommand":
		perm = extensionapi.PermissionCommands
	case "/proto.DocumentStore/Get":
		perm = extensionapi.PermissionStorage
	case "/proto.DocumentStore/Set":
		perm = extensionapi.PermissionStorage
	case "/proto.DocumentStore/Create":
		perm = extensionapi.PermissionStorage
	case "/proto.DocumentStore/Update":
		perm = extensionapi.PermissionStorage
	case "/proto.DocumentStore/Delete":
		perm = extensionapi.PermissionStorage
	case "/proto.DocumentStore/List":
		perm = extensionapi.PermissionStorage
	default:
		err = fmt.Errorf("extraneous rpc resource %s: %w",
			resource, blueauth.ErrForbidden)
		return
	}
	_, ok := claims.Extra.Permissions[perm]
	if !ok {
		err = blueauth.ErrForbidden
		return
	}
	return nil
}
