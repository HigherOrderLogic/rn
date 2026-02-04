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
	"errors"
	"io"
	"sync"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"unstable.build/go-tui/api/textapi/textext"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/text/textrpc"
)

// CommandEventHandler combines EventHandler with CommandHandler.
type CommandEventHandler interface {
	textapi.EventHandler
	textapi.CommandHandler
	io.Closer
}

// CommandEventHandlerFacility abstracts the ability to create new CommandEventHandler.
type CommandEventHandlerFacility func(context.Context, textapi.Editor, []extension.Grant,
	rpc.MuxBroker, config.Config) (CommandEventHandler, error)

// NewEditorEventHandler returns a extension.Grantee that simply responds to commands.
// It calls fn to build a CommandEventHandler, subcsribes it to events
// editor. Event and registers it as the CommandHandler of cmds.
// It also requests extraPerms, in addition to extensionapi.PermissionEditor.
// All granted permissions are returned in the fn callback. If one of the
// permissions is denied, the extension will exit with an error.
func NewEditorEventHandler(
	cmds []textapi.CommandManual,
	fn CommandEventHandlerFacility,
	events []textapi.EventType,
	extraPerms ...extensionapi.Permission,
) (extension.Grantee, []extensionapi.Permission) {
	perms := []extensionapi.Permission{
		extensionapi.Permission(extensionapi.PermissionEditor),
		extensionapi.Permission(extensionapi.PermissionCommands),
	}
	perms = append(perms, extraPerms...)
	s := &editorGrantee{evs: events, cmds: cmds, newHandler: fn}
	return s, perms
}

type editorGrantee struct {
	mu         sync.Mutex
	broker     rpc.MuxBroker
	ed         textapi.Editor
	handler    CommandEventHandler
	newHandler CommandEventHandlerFacility
	cmds       []textapi.CommandManual
	pconfig    config.Config
	evs        []textapi.EventType
}

func (t *editorGrantee) Connected(
	ctx context.Context, broker rpc.MuxBroker, config config.Config,
) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.log(log.DebugLevel, "extension connected")
	t.broker = broker
	t.pconfig = config
	return nil
}

func (t *editorGrantee) setNewHandler(
	ctx context.Context, grants []extension.Grant,
) (CommandEventHandler, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	h, err := t.newHandler(ctx, t.ed, grants, t.broker, t.pconfig)
	if err != nil {
		return nil, err
	}
	t.handler = h
	return h, nil
}

func (t *editorGrantee) subscribeToEvents(ctx context.Context, grants []extension.Grant) error {
	h, err := t.setNewHandler(ctx, grants)
	if err != nil {
		return err
	}

	err = t.ed.SubscribeEvents(t.evs, h)
	if err != nil {
		return err
	}

	for _, cmd := range t.cmds {
		// TODO refactor to use workspace api
		if err = t.ed.(*textrpc.Client).SubscribeCommand(cmd, h); err != nil {
			return err
		}
	}

	t.log(log.DebugLevel, "subscribed to events %v", t.evs)
	return nil
}

func (t *editorGrantee) PermissionGranted(ctx context.Context, grants []extension.Grant) (ret error) {
	t.log(log.DebugLevel, "permissions granted: %v", grants)

	for _, grant := range grants {
		var err error
		switch grant.Permission {
		case extensionapi.PermissionEditor:
			t.ed, err = textext.Editor(ctx, grant, t.broker)
			if err == nil {
				err = t.subscribeToEvents(ctx, grants)
			}
		}
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return ret
}

func (t *editorGrantee) PermissionDenied(ctx context.Context, perms []extensionapi.Permission) error {
	t.log(log.DebugLevel, "permissions denied: %v", perms)

	for _, perm := range perms {
		switch perm {
		case extensionapi.PermissionEditor:
			return errors.New("permission editor must be granted for this extension to work")
		}
	}
	return nil
}

func (t *editorGrantee) Shutdown(ctx context.Context, reason string) error {
	t.log(log.DebugLevel, "extension being shutdown: reason: %s", reason)

	t.mu.Lock()
	handler := t.handler
	t.mu.Unlock()

	if handler != nil {
		return handler.Close()
	}
	return nil
}

func (t *editorGrantee) Health(context.Context) error {
	return nil
}

func (t *editorGrantee) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "extutil.editorGrantee",
	}).Logf(level, msg, args...)
}
