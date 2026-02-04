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

package browser

import (
	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/clipboard"
	tterm "unstable.build/go-tui/term"
)

// Keydump returns a Floating handler that prints the incoming events as rows.
func Keydump(
	clipboard clipboard.Register, notifications browserapi.Notifications,
) Floating {
	ret := new(keydump)
	ret.list.Init()
	cfg := component.StringResponsiveConfig{
		NoSplitWords: true,
		StringConfig: component.StringConfig{
			Alignment: component.AlignmentLeft,
		},
	}
	ret.clipboard = clipboard
	ret.placeholder = component.NewResponsiveString(
		"To exit press <ctrl-c><ctrl-d>", cfg)
	ret.notifications = notifications
	return ret
}

type keydump struct {
	notifications browserapi.Notifications
	clipboard     clipboard.Register
	placeholder   *component.ResponsiveString
	list          component.ResponsiveList
	selection     string
	exit          bool
}

func (k *keydump) Handle(ev term.Event) (exit, handled bool) {
	switch ev.Type {
	case term.EventMouse:
		if ev.Key != term.MouseLeft {
			return
		}
		selection, ok := k.list.ElementAt(term.Coordinates{X: ev.MouseX, Y: ev.MouseY})
		if ok {
			sel := selection.Value().(*component.ResponsiveString)
			k.selection = sel.String()
			handled = true
			err := k.clipboard.Copy(
				clipboard.DefaultRegisterID, clipboard.Data{Text: k.selection})
			if err == nil {
				_, _ = k.notifications.Notify(browserapi.LevelInfo,
					"%q copied to clipboard", k.selection)
			} else {
				_, _ = k.notifications.Notify(browserapi.LevelError,
					"%q copied to clipboard: %v", k.selection, err)
			}
		}
		return
	case term.EventKey:
	default:
		return
	}
	if ev.Type != term.EventKey {
		return
	}
	str := tterm.KeyCombString(ev.KeyComb())
	cfg := component.StringResponsiveConfig{
		NoSplitWords: true,
		StringConfig: component.StringConfig{
			Alignment: component.AlignmentLeft,
		},
	}
	k.list.PushFront(component.NewResponsiveString(str, cfg))
	handled = true
	switch {
	case ev.Ch == 'c' && ev.Mod == term.ModCtrl:
		k.exit = true
	case ev.Ch == 'd' && ev.Mod == term.ModCtrl && k.exit:
		exit = true
	}
	return
}

// Cursor should return the cursor coordinates, style and whether it should be shown at all.
func (k *keydump) Cursor() (c term.Coordinates, s term.CursorStyle, show bool) {
	return
}

// Selection returns the selected text and true if there's currently any, or
// an empty string and false if there's no text selected.
func (k *keydump) Selection() (string, bool) {
	return k.selection, k.selection != ""
}

func (k *keydump) Dimensions() (width int, height int) {
	return 60, 20
}

func (k *keydump) Resize(width, height int) {
	if k.list.Len() == 0 {
		k.placeholder.Resize(width, height)
	}
	k.list.Resize(width, height)
}

// Draw draws this component to the underlying Writer. It returns non-nil error
// if something went wrong in the process of writing or the writer returned
// an error.
func (k keydump) Draw(w term.Writer) {
	if k.list.Len() == 0 {
		k.placeholder.Draw(w)
		return
	}
	k.list.Draw(w)
}

func (k keydump) Close() error {
	return nil
}
