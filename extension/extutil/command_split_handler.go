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
	"fmt"
	"sync"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"unstable.build/go-tui/api/browserapi"
	"unstable.build/go-tui/api/browserapi/browserext"
	"unstable.build/go-tui/api/config"
	"unstable.build/go-tui/api/extensionapi"
	"unstable.build/go-tui/api/textapi"
	"unstable.build/go-tui/api/textapi/textext"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/rpc"
	"unstable.build/go-tui/text/textrpc"
)

// CommandSplitHandlerConfig provides the configuration required to
// use CommandSplitHandler extension.Grantee helper. See CommandSplitHandler
// for more details.
type CommandSplitHandlerConfig struct {
	// Command that triggers Split
	Command textapi.CommandManual

	// SplitOrientatio of the new split window.
	SplitOrientation browserapi.Orientation

	// Handler is the constructor used to install a handler
	// on the split window. The focus argument represents
	// the window in focus when cmd event was fired.
	// If returned Handler satisfies io.Closer, then Close will be called
	// when split window is closed.
	Handler func(context.Context, textapi.Command, []extension.Grant,
		rpc.MuxBroker, browserapi.Window, config.Config) (RedispatchHandler, error)

	// Permissions to be requested for Handler.
	Permissions []extensionapi.Permission
}

// NewCommandSplitHandler returns a extension.Grantee that opens a split window
// with a new handler when cmd event is fired or command called.
// This function never returns.
func NewCommandSplitHandler(config CommandSplitHandlerConfig) (
	extension.Grantee, []extensionapi.Permission,
) {
	if config.Handler == nil || config.Command.Name == "" {
		panic(fmt.Sprintf("invalid cmd split handler configuration: "+
			"Handler and Command must be set: %#v", config))
	}

	perms := []extensionapi.Permission{
		extensionapi.Permission(extensionapi.PermissionBrowserWindowManager),
		extensionapi.Permission(extensionapi.PermissionEditor),
		extensionapi.Permission(extensionapi.PermissionCommands),
	}
	perms = append(perms, config.Permissions...)
	return &cmdSplitHandler{config: config}, perms
}

// RedispatchHandler is a handler that is also interested
// in receiving calls to Redispatch, which is invoked
// when a command has been dispatched after the split
// window has already been open.
type RedispatchHandler interface {
	browserapi.Handler
	Redispatch(context.Context, textapi.Command) error
}

// NopRedispatchHandler returns a RedispatchHandler that does nothing
// when Redispatch is called.
func NopRedispatchHandler(h browserapi.Handler) RedispatchHandler {
	return fnHandler{Handler: h}
}

// FuncRedispatchHandler returns a RedispatchHandler that calls fn
// when Redispatch is called.
func FuncRedispatchHandler(
	h browserapi.Handler,
	fn func(context.Context, textapi.Command) error,
) RedispatchHandler {
	return fnHandler{Handler: h, fn: fn}
}

type fnHandler struct {
	browserapi.Handler
	fn func(context.Context, textapi.Command) error
}

func (n fnHandler) Redispatch(ctx context.Context, cmd textapi.Command) error {
	if n.fn != nil {
		return n.fn(ctx, cmd)
	}
	return nil
}

type cmdSplitHandler struct {
	mu     sync.Mutex
	config CommandSplitHandlerConfig

	broker  rpc.MuxBroker
	wm      browserapi.WindowManager
	ed      textapi.Editor
	pconfig config.Config
	grants  []extension.Grant
	h       RedispatchHandler
	win     browserapi.Window
}

func (t *cmdSplitHandler) Connected(
	ctx context.Context, broker rpc.MuxBroker, config config.Config,
) error {
	t.log(log.DebugLevel, "extension connected")
	t.broker = broker
	t.pconfig = config
	return nil
}

func (t *cmdSplitHandler) cleanWindow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.win == nil {
		return false
	}

	if err := t.wm.CloseWindow(t.win); err != nil {
		t.log(log.DebugLevel, "close window: %v", err)
	}
	t.win = nil

	return true
}

func (t *cmdSplitHandler) exitClean() {
	t.log(log.DebugLevel, "received exit signal; cleaning resources...")

	if t.cleanWindow() {
		t.log(log.DebugLevel, "cleaned window")
	}
}

func (t *cmdSplitHandler) openSplitWindow(ctx context.Context, cmd textapi.Command) error {
	focusWin := cmd.Window
	t.mu.Lock()
	win := t.win
	wm := t.wm
	t.mu.Unlock()

	if win != nil {
		return t.h.Redispatch(ctx, cmd)
	}

	h, err := t.config.Handler(ctx, cmd, t.grants, t.broker, focusWin, t.pconfig)
	if err != nil {
		err = fmt.Errorf("config.Handler: %w", err)
		return err
	}

	cleaningHandler := browserapi.FuncHandler(h, func() error {
		t.exitClean()
		return h.Close()
	})
	splitWin, err := wm.Split(t.config.SplitOrientation, focusWin, cleaningHandler)
	if err != nil {
		err = fmt.Errorf("wm.Split: %w", err)
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.h = h
	t.win = splitWin
	return nil
}

func (t *cmdSplitHandler) Complete(ctx context.Context, name string, args []string) (
	iterator.Iterator[string], error,
) {
	return iterator.FromSlice[string](nil), nil
}

func (t *cmdSplitHandler) HandleCommand(ctx context.Context, cmd textapi.Command) (err error) {
	if cmd.Name == t.config.Command.Name {
		return t.openSplitWindow(ctx, cmd)
	}
	return nil
}

func (t *cmdSplitHandler) PermissionGranted(ctx context.Context, grants []extension.Grant) (ret error) {
	t.log(log.DebugLevel, "permissions granted: %+v", grants)

	t.mu.Lock()
	defer t.mu.Unlock()

	var err error
	for _, g := range grants {
		switch g.Permission {
		case extensionapi.PermissionBrowserWindowManager:
			t.wm, err = browserext.WindowManager(ctx, g, t.broker)
		case extensionapi.PermissionEditor:
			t.ed, err = textext.Editor(ctx, g, t.broker)
			if err == nil {
				// TODO refactor to use v2 workspace api
				err = t.ed.(*textrpc.Client).SubscribeCommand(t.config.Command, t)
			}
		}
		if err != nil {
			ret = multierror.Append(ret, err)
		}
	}

	t.grants = grants
	return ret
}

func (t *cmdSplitHandler) PermissionDenied(
	ctx context.Context, perms []extensionapi.Permission,
) error {
	t.log(log.DebugLevel, "permission denied: %v", perms)

	for _, perm := range perms {
		switch perm {
		case extensionapi.PermissionBrowserWindowManager, extensionapi.PermissionEditor:
			return errors.New("permission window manager and permission " +
				"editor must be granted for this extension to work")
		}
	}
	return nil
}

func (t *cmdSplitHandler) Shutdown(ctx context.Context, reason string) error {
	t.log(log.DebugLevel, "extension being shutdown: %s", reason)
	t.cleanWindow()
	return nil
}

func (t *cmdSplitHandler) Health(ctx context.Context) error {
	return nil
}

func (t *cmdSplitHandler) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "extutil.cmdSplitHandler",
	}).Logf(level, msg, args...)
}
