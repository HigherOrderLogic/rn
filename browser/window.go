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

package browser

import (
	"errors"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/handler"
)

type browserWindow struct {
	parent *Component
	win    handler.Window
}

func (w *browserWindow) Focus() (bool, error) {
	return w.win.Focus(), nil
}

func (w *browserWindow) WindowID() uint64 {
	return w.win.ID()
}

func (w *browserWindow) Closed() bool {
	return w.parent == nil || w.win.Closed()
}

func (w *browserWindow) IsFloating() bool {
	return w.win.IsFloating()
}

func (w *browserWindow) Content() (browserapi.Handler, error) {
	h := w.win.Content().(browserapi.Handler)
	t, ok := h.(*Tab)
	if !ok {
		bc, ok := h.(*browserContent)
		if ok {
			return bc.Handler, nil
		}
		fsc, ok := h.(*browserFloatingScrollableContent)
		if ok {
			return fsc.Handler, nil
		}
		fc, ok := h.(*browserFloatingContent)
		if ok {
			return fc.Handler, nil
		}
		return h.(*browserScrollableContent).Handler, nil
	}
	return t, nil
}

func (w *browserWindow) SetContent(h browserapi.Handler) error {
	if w.parent == nil {
		return errors.New("window is closing")
	}
	return w.parent.tryUpdateWindowContent(w, h, w.win.Content().(browserapi.Handler))
}

func (w *browserWindow) IsMinimized() (component.Alignment, bool) {
	return w.win.IsMinimized()
}

func (w *browserWindow) MinimizeUp(padding int) bool {
	return w.win.MinimizeUp(padding)
}

func (w *browserWindow) MinimizeDown(padding int) bool {
	return w.win.MinimizeDown(padding)
}

func (w *browserWindow) MinimizeLeft(padding int) bool {
	return w.win.MinimizeLeft(padding)
}

func (w *browserWindow) MinimizeRight(padding int) bool {
	return w.win.MinimizeRight(padding)
}

func (w *browserWindow) Unminimize() bool {
	return w.win.Unminimize()
}

func (w *browserWindow) SetFrameAttr(attr term.Attributes) (term.Attributes, bool) {
	return w.win.SetFrameAttr(attr)
}

func (w *browserWindow) Close() error {
	if w.parent == nil {
		return nil
	}

	parent := w.parent

	err := parent.closeWindow(w)
	if err != nil {
		return err
	}

	w.parent = nil

	return nil
}
