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

//revive:disable:exported
package browsertest

import (
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
)

// WindowToAPIWindow is a convenience method to work
// with browserapi.Window and browser.Window in the same package.
type WindowToAPIWindow struct {
	Win browser.Window
}

func (a WindowToAPIWindow) SetContent(h browserapi.Handler) error {
	return a.Win.SetContent(h)
}

func (a WindowToAPIWindow) Focus() (bool, error) {
	return a.Win.Focus()
}

func (a WindowToAPIWindow) Close() error {
	return a.Win.Close()
}

func (a WindowToAPIWindow) Content() (browserapi.Handler, error) {
	return a.Win.Content()
}

func (a WindowToAPIWindow) WindowID() uint64 {
	return a.Win.WindowID()
}

// WindowFromAPIWindow is a convenience method to work
// with browserapi.Window and browser.Window in the same package.
type WindowFromAPIWindow struct {
	Browser browserapi.Browser
	Win     browserapi.Window
}

func (a WindowFromAPIWindow) SetContent(h browserapi.Handler) error {
	return a.Browser.SetWindowContent(a.Win, h)
}

func (a WindowFromAPIWindow) Content() (browserapi.Handler, error) {
	h, err := a.Win.(interface {
		Content() (browserapi.Browser, error)
	}).Content()
	return h.(browserapi.Handler), err
}

func (a WindowFromAPIWindow) WindowID() uint64 {
	return a.Win.WindowID()
}

func (a WindowFromAPIWindow) Focus() (bool, error) {
	return a.Win.(interface{ Focus() (bool, error) }).Focus()
}

func (a WindowFromAPIWindow) Close() error {
	return a.Browser.CloseWindow(a.Win)
}

func (w WindowFromAPIWindow) Closed() bool {
	return w.Win.(interface{ Closed() bool }).Closed()
}

func (w WindowFromAPIWindow) IsFloating() bool {
	return w.Win.(interface{ IsFloating() bool }).IsFloating()
}

func (w WindowFromAPIWindow) SetFrameAttr(t term.Attributes) (term.Attributes, bool) {
	return w.Win.(interface {
		SetFrameAttr(t term.Attributes) (term.Attributes, bool)
	}).SetFrameAttr(t)
}

// IsMinimized returns true if this is a floating window and it's minimized.
func (w WindowFromAPIWindow) IsMinimized() (component.Alignment, bool) {
	return w.Win.(interface {
		IsMinimized() (component.Alignment, bool)
	}).IsMinimized()
}

// MinimizeUp minimizes this window and displays it above the window manager,
// if this window is a floating window.
func (w WindowFromAPIWindow) MinimizeUp(padding int) bool {
	return w.Win.(interface{ MinimizeUp(int) bool }).MinimizeUp(padding)
}

// MinimizeDown minimizes this window and displays it below the window manager,
// if this window is a floating window.
func (w WindowFromAPIWindow) MinimizeDown(padding int) bool {
	return w.Win.(interface{ MinimizeDown(int) bool }).MinimizeDown(padding)
}

// MinimizeLeft minimizes this window and displays it left of the window manager,
// if this window is a floating window.
func (w WindowFromAPIWindow) MinimizeLeft(padding int) bool {
	return w.Win.(interface{ MinimizeLeft(int) bool }).MinimizeLeft(padding)
}

// MinimizeRight minimizes this window and displays it left of the window manager,
// if this window is a floating window.
func (w WindowFromAPIWindow) MinimizeRight(padding int) bool {
	return w.Win.(interface{ MinimizeRight(int) bool }).MinimizeRight(padding)
}

// Unminimize un-minimizes this window and displays it at the back at the front.
func (w WindowFromAPIWindow) Unminimize() bool {
	return w.Win.(interface{ Unminimize() bool }).Unminimize()
}
