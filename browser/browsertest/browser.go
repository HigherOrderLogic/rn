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

package browsertest

import (
	"context"
	"errors"
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/browser"
)

// BrowserFromAPIBrowser wraps a browserapi.Browser and returns
// a browser.Browser.
func BrowserFromAPIBrowser(b browserapi.Browser) browser.Browser {
	return toBrowser{b: b}
}

type toBrowser struct {
	b browserapi.Browser
}

func (b toBrowser) Focus() (browser.Window, error) {
	win, err := b.b.Focus()
	if err != nil {
		return nil, err
	}
	return WindowFromAPIWindow{Browser: b.b, Win: win}, nil
}

func (b toBrowser) SetTabName(uri workspaceapi.URI, name string, attr term.Attributes) error {
	return nil
}

func (b toBrowser) SetFocus(browser.Window) (browser.Window, error) {
	return nil, errors.New("unimplemented")
}

func (b toBrowser) Split(
	o browserapi.Orientation, win browser.Window, h browserapi.Handler,
) (browser.Window, error) {
	var inWin browserapi.Window
	if a, ok := win.(WindowFromAPIWindow); ok {
		inWin = a.Win
	} else {
		inWin = WindowToAPIWindow{Win: win}
	}
	retWin, err := b.b.Split(o, inWin, h)
	if err != nil {
		return nil, err
	}
	return WindowFromAPIWindow{Browser: b.b, Win: retWin}, nil
}

func (b toBrowser) Floating(
	h browser.Floating, cfg browserapi.FloatingConfig,
) (browser.Window, error) {
	retWin, err := b.b.Floating(h, cfg)
	if err != nil {
		return nil, err
	}
	return WindowFromAPIWindow{Browser: b.b, Win: retWin}, nil
}

func (b toBrowser) Bar(o browserapi.BarConfig, h tui.Handler) error {
	return b.b.Bar(o, h)
}

func (b toBrowser) Tab(
	uri workspaceapi.URI, icon rune, name string, h browserapi.Handler,
) (browserapi.Handler, error) {
	return b.b.Tab(uri, icon, name, h)
}

func (b toBrowser) Window(id uint64) (browser.Window, bool) {
	return NopWindow(), true
}

func (b toBrowser) Notify(level browserapi.NotificationLevel, msg string, args ...interface{}) (
	string, error,
) {
	return b.b.Notify(level, msg, args...)
}

func (b toBrowser) NotifyOnce(
	level browserapi.NotificationLevel, msg string, args ...interface{},
) (string, error) {
	return b.b.NotifyOnce(level, msg, args...)
}

func (b toBrowser) UpdateNotificationProgress(
	id, message string, progress, total int64,
) error {
	return b.b.UpdateNotificationProgress(id, message, progress, total)
}

func (b toBrowser) Open(resource workspaceapi.URI) (browserapi.Handler, error) {
	return b.b.Open(resource)
}

func (b toBrowser) Resource(u workspaceapi.URI) (browserapi.Handler, bool) {
	return NewTestHandler(), true
}

func (b toBrowser) PublishEvent(ev term.Event) error {
	switch ev.Type {
	case term.EventInterrupt:
		return b.b.Interrupt(context.Background())
	case term.EventNone:
		return b.b.PublishEventNone()
	}
	return fmt.Errorf("cannot publish event type: %v", ev.Type)
}

func (b toBrowser) Close() error {
	return b.b.Close()
}
