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

package handler

import (
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/rune-go-sdk/term"
)

var _ tui.Handler = (*Proxy)(nil)

// Proxy satisfies tui.Handler by taking a pointer to a tui.Handler
// and dereferencing on each method call. Caller is responsible for
// initializing Ptr correctly.
type Proxy struct {
	Target tui.Handler
}

// Resize satisfies tui.Handler.
func (i *Proxy) Resize(width, height int) {
	i.Target.Resize(width, height)
}

// Draw satisfies tui.Handler.
func (i *Proxy) Draw(w term.Writer) {
	i.Target.Draw(w)
}

// Handle satisfies tui.Handler.
func (i *Proxy) Handle(ev term.Event) (exit, handled bool) {
	return i.Target.Handle(ev)
}

// Cursor satisfies tui.Handler.
func (i *Proxy) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return i.Target.Cursor()
}

// Selection satisfies tui.Handler.
func (i *Proxy) Selection() (string, bool) {
	return i.Target.Selection()
}
