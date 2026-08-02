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

package ide

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/schemeapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/workspace"
)

// asyncTerminalResizeQueue only has to hold the one resize that the
// worker has not picked up yet: anything older is already superseded
// by the newest size, so SetPtySize drops it rather than wait.
const asyncTerminalResizeQueue = 1

// errInvalidMasterPtyFd is the message workspacerpc.Server.SetPtySize
// reports once the master descriptor is gone. gRPC flattens it to an
// untyped status error, so the message is all the client can match on.
const errInvalidMasterPtyFd = "invalid master pty fd"

type ptyResize struct {
	pty           workspaceapi.Pty
	width, height int
}

type asyncTerminal struct {
	schemeapi.Terminal
	notifications browserapi.Notifications

	resizes chan ptyResize
	stop    chan struct{}
	// closing serializes Close so a double teardown cannot close
	// stop twice.
	closing chan struct{}
}

func newAsyncTerminal(
	t schemeapi.Terminal, n browserapi.Notifications,
) *asyncTerminal {
	ret := &asyncTerminal{
		Terminal:      t,
		notifications: n,
		resizes:       make(chan ptyResize, asyncTerminalResizeQueue),
		stop:          make(chan struct{}),
		closing:       make(chan struct{}, 1),
	}
	go debug.CapturePanicReport(ret.run)
	return ret
}

// SetPtySize queues the resize and returns nil immediately. Transport
// failures surface later, as a notification raised by the worker.
func (t *asyncTerminal) SetPtySize(
	p workspaceapi.Pty, width, height int,
) error {
	req := ptyResize{pty: p, width: width, height: height}
	if t.offer(req) {
		return nil
	}
	// Whatever is queued is superseded by this request, so drop it
	// instead of stalling the event loop behind the transport.
	select {
	case dropped := <-t.resizes:
		t.log(log.DebugLevel, "dropped superseded pty resize %dx%d",
			dropped.width, dropped.height)
	default:
	}
	if !t.offer(req) {
		t.log(log.WarnLevel, "dropped pty resize %dx%d: queue is saturated",
			width, height)
	}
	return nil
}

func (t *asyncTerminal) offer(req ptyResize) bool {
	select {
	case t.resizes <- req:
		return true
	default:
		return false
	}
}

// Close stops the resize worker. It does not wait for an in-flight
// transport RPC; closing the underlying workspace unwinds that.
func (t *asyncTerminal) Close() {
	select {
	case t.closing <- struct{}{}:
		close(t.stop)
	default:
	}
}

func (t *asyncTerminal) run() {
	for {
		// Checked before every dispatch so a resize queued before
		// Close is not delivered once a blocked RPC unwinds.
		select {
		case <-t.stop:
			return
		default:
		}
		select {
		case <-t.stop:
			return
		case req := <-t.resizes:
			err := t.Terminal.SetPtySize(req.pty, req.width, req.height)
			if err != nil {
				t.log(log.ErrorLevel, "set pty size %dx%d: %v",
					req.width, req.height, err)
				// A pty that is already gone cannot be resized, and
				// the terminal is tearing down anyway, so the toast
				// would only be noise.
				if strings.Contains(err.Error(), errInvalidMasterPtyFd) {
					continue
				}
				_, _ = t.notifications.Notify(browserapi.LevelError,
					"resize terminal to %dx%d: %v",
					req.width, req.height, err)
			}
		}
	}
}

// OnDisconnect forwards [workspace.RemoteScheme.OnDisconnect] when the
// wrapped terminal is remote. Without it the decorator would hide the
// optional interface from vtereservoir.New's type assertion and warm
// VTEs would survive a dead transport.
func (t *asyncTerminal) OnDisconnect() <-chan struct{} {
	if rs, ok := t.Terminal.(workspace.RemoteScheme); ok {
		return rs.OnDisconnect()
	}
	return nil
}

// WaitConnected forwards [workspace.RemoteScheme.WaitConnected] when
// the wrapped terminal is remote, and is a no-op otherwise.
func (t *asyncTerminal) WaitConnected(ctx context.Context) error {
	if rs, ok := t.Terminal.(workspace.RemoteScheme); ok {
		return rs.WaitConnected(ctx)
	}
	return nil
}

func (t *asyncTerminal) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithField(logging.KeyClass, "ide.asyncTerminal").
		Logf(level, msg, args...)
}
