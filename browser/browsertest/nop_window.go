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
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/browser"
)

type noopWindow struct {
	content browserapi.Handler
}

func (w *noopWindow) Content() (browserapi.Handler, error)  { return w.content, nil }
func (w *noopWindow) SetContent(h browserapi.Handler) error { w.content = h; return nil }
func (w *noopWindow) Close() error {
	if w.content == nil {
		return nil
	}
	err := w.content.Close()
	w.content = nil
	return err
}
func (w *noopWindow) WindowID() uint64                         { return 1 }
func (w *noopWindow) Focus() (bool, error)                     { return false, nil }
func (w *noopWindow) Closed() bool                             { return false }
func (w *noopWindow) IsFloating() bool                         { return false }
func (w *noopWindow) IsMinimized() (component.Alignment, bool) { return 0, false }
func (w *noopWindow) MinimizeUp(padding int) bool              { return false }
func (w *noopWindow) MinimizeDown(padding int) bool            { return false }
func (w *noopWindow) MinimizeLeft(padding int) bool            { return false }
func (w *noopWindow) MinimizeRight(padding int) bool           { return false }
func (w *noopWindow) Unminimize() bool                         { return false }
func (w *noopWindow) SetFrameAttr(term.Attributes) (term.Attributes, bool) {
	return term.Attributes{}, false
}

// NopWindow returns a window that does nothing.
func NopWindow() browser.Window {
	return &noopWindow{}
}
