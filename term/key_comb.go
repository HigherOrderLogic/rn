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

package term

import (
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// KeyCombString returns the long string representation of this term.KeyComb.
func KeyCombString(k term.KeyComb) string {
	if k.Ch != 0 && k.Key != term.KeySpace {
		ch, mod := processCharacter(k.Ch, k.Mod)
		// if Ch is set, then ignore key for representing this key comb
		switch mod {
		case term.ModMeta:
			return fmt.Sprintf("<meta-%s>", ch)
		case term.ModAlt:
			return fmt.Sprintf("<alt-%s>", ch)
		case term.ModShift:
			return fmt.Sprintf("<shift-%s>", ch)
		case term.ModCtrl:
			return fmt.Sprintf("<ctrl-%s>", ch)
		case term.ModCtrlShift:
			return fmt.Sprintf("<ctrl-shift-%s>", ch)
		case term.ModCtrlAlt:
			return fmt.Sprintf("<ctrl-alt-%s>", ch)
		case term.ModCtrlMeta:
			return fmt.Sprintf("<ctrl-meta-%s>", ch)
		case term.ModCtrlShiftAlt:
			return fmt.Sprintf("<ctrl-shift-alt-%s>", ch)
		case term.ModCtrlShiftMeta:
			return fmt.Sprintf("<ctrl-shift-meta-%s>", ch)
		case term.ModCtrlAltMeta:
			return fmt.Sprintf("<ctrl-alt-meta-%s>", ch)
		case term.ModShiftMeta:
			return fmt.Sprintf("<shift-meta-%s>", ch)
		case term.ModAltMeta:
			return fmt.Sprintf("<alt-meta-%s>", ch)
		case term.ModAltShiftMeta:
			return fmt.Sprintf("<alt-shift-meta-%s>", ch)
		case term.ModAltShift:
			return fmt.Sprintf("<alt-shift-%s>", ch)
		default:
			return string(ch)
		}
	}

	switch k {
	case term.KeyComb{Key: term.KeyF1}:
		return "<f1>"
	case term.KeyComb{Key: term.KeyF2}:
		return "<f2>"
	case term.KeyComb{Key: term.KeyF3}:
		return "<f3>"
	case term.KeyComb{Key: term.KeyF4}:
		return "<f4>"
	case term.KeyComb{Key: term.KeyF5}:
		return "<f5>"
	case term.KeyComb{Key: term.KeyF6}:
		return "<f6>"
	case term.KeyComb{Key: term.KeyF7}:
		return "<f7>"
	case term.KeyComb{Key: term.KeyF8}:
		return "<f8>"
	case term.KeyComb{Key: term.KeyF9}:
		return "<f9>"
	case term.KeyComb{Key: term.KeyF10}:
		return "<f10>"
	case term.KeyComb{Key: term.KeyF11}:
		return "<f11>"
	case term.KeyComb{Key: term.KeyF12}:
		return "<f12>"
	case term.KeyComb{Key: term.KeyInsert}:
		return "<insert>"
	case term.KeyComb{Key: term.KeyDelete}:
		return "<delete>"
	case term.KeyComb{Key: term.KeyHome}:
		return "<home>"
	case term.KeyComb{Key: term.KeyEnd}:
		return "<end>"
	case term.KeyComb{Key: term.KeyPgup}:
		return "<pgup>"
	case term.KeyComb{Key: term.KeyPgdn}:
		return "<pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp}:
		return "<up>"
	case term.KeyComb{Key: term.KeyArrowDown}:
		return "<down>"
	case term.KeyComb{Key: term.KeyArrowLeft}:
		return "<left>"
	case term.KeyComb{Key: term.KeyArrowRight}:
		return "<right>"
	case term.KeyComb{Key: term.MouseLeft}:
		return "<mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle}:
		return "<mouse-middle>"
	case term.KeyComb{Key: term.MouseRight}:
		return "<mouse-right>"
	case term.KeyComb{Key: term.MouseRelease}:
		return "<mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp}:
		return "<mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown}:
		return "<mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace}:
		return "<backspace>"
	case term.KeyComb{Key: term.KeyTab}:
		return "<tab>"
	case term.KeyComb{Key: term.KeyEnter}:
		return "<enter>"
	case term.KeyComb{Key: term.KeyEsc}:
		return "<esc>"
	case term.KeyComb{Key: term.KeySpace, Ch: ' '}, term.KeyComb{Key: term.KeySpace}:
		return "<space>"

	// meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModMeta}:
		return "<meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModMeta}:
		return "<meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModMeta}:
		return "<meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModMeta}:
		return "<meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModMeta}:
		return "<meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModMeta}:
		return "<meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModMeta}:
		return "<meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModMeta}:
		return "<meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModMeta}:
		return "<meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModMeta}:
		return "<meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModMeta}:
		return "<meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModMeta}:
		return "<meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModMeta}:
		return "<meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModMeta}:
		return "<meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModMeta}:
		return "<meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModMeta}:
		return "<meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModMeta}:
		return "<meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModMeta}:
		return "<meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModMeta}:
		return "<meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModMeta}:
		return "<meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModMeta}:
		return "<meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModMeta}:
		return "<meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModMeta}:
		return "<meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModMeta}:
		return "<meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModMeta}:
		return "<meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModMeta}:
		return "<meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModMeta}:
		return "<meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModMeta}:
		return "<meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModMeta}:
		return "<meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModMeta}:
		return "<meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModMeta}:
		return "<meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModMeta}:
		return "<meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModMeta}:
		return "<meta-space>"

	// alt
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAlt}:
		return "<alt-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAlt}:
		return "<alt-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAlt}:
		return "<alt-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAlt}:
		return "<alt-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAlt}:
		return "<alt-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAlt}:
		return "<alt-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAlt}:
		return "<alt-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAlt}:
		return "<alt-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAlt}:
		return "<alt-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAlt}:
		return "<alt-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAlt}:
		return "<alt-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAlt}:
		return "<alt-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAlt}:
		return "<alt-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAlt}:
		return "<alt-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAlt}:
		return "<alt-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAlt}:
		return "<alt-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAlt}:
		return "<alt-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAlt}:
		return "<alt-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAlt}:
		return "<alt-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAlt}:
		return "<alt-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAlt}:
		return "<alt-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAlt}:
		return "<alt-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAlt}:
		return "<alt-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAlt}:
		return "<alt-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAlt}:
		return "<alt-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAlt}:
		return "<alt-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAlt}:
		return "<alt-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAlt}:
		return "<alt-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAlt}:
		return "<alt-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAlt}:
		return "<alt-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAlt}:
		return "<alt-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAlt}:
		return "<alt-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAlt}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAlt}:
		return "<alt-space>"

	// shift
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModShift}:
		return "<shift-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModShift}:
		return "<shift-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModShift}:
		return "<shift-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModShift}:
		return "<shift-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModShift}:
		return "<shift-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModShift}:
		return "<shift-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModShift}:
		return "<shift-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModShift}:
		return "<shift-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModShift}:
		return "<shift-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModShift}:
		return "<shift-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModShift}:
		return "<shift-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModShift}:
		return "<shift-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModShift}:
		return "<shift-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModShift}:
		return "<shift-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModShift}:
		return "<shift-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModShift}:
		return "<shift-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModShift}:
		return "<shift-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModShift}:
		return "<shift-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModShift}:
		return "<shift-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModShift}:
		return "<shift-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModShift}:
		return "<shift-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModShift}:
		return "<shift-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModShift}:
		return "<shift-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModShift}:
		return "<shift-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModShift}:
		return "<shift-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModShift}:
		return "<shift-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModShift}:
		return "<shift-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModShift}:
		return "<shift-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModShift}:
		return "<shift-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModShift}:
		return "<shift-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModShift}:
		return "<shift-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModShift}:
		return "<shift-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModShift}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModShift}:
		return "<shift-space>"

	// ctrl
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrl}:
		return "<ctrl-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrl}:
		return "<ctrl-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrl}:
		return "<ctrl-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrl}:
		return "<ctrl-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrl}:
		return "<ctrl-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrl}:
		return "<ctrl-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrl}:
		return "<ctrl-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrl}:
		return "<ctrl-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrl}:
		return "<ctrl-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrl}:
		return "<ctrl-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrl}:
		return "<ctrl-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrl}:
		return "<ctrl-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrl}:
		return "<ctrl-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrl}:
		return "<ctrl-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrl}:
		return "<ctrl-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrl}:
		return "<ctrl-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrl}:
		return "<ctrl-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrl}:
		return "<ctrl-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrl}:
		return "<ctrl-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrl}:
		return "<ctrl-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrl}:
		return "<ctrl-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrl}:
		return "<ctrl-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrl}:
		return "<ctrl-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrl}:
		return "<ctrl-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrl}:
		return "<ctrl-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrl}:
		return "<ctrl-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrl}:
		return "<ctrl-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrl}:
		return "<ctrl-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrl}:
		return "<ctrl-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrl}:
		return "<ctrl-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrl}:
		return "<ctrl-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrl}:
		return "<ctrl-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrl}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrl}:
		return "<ctrl-space>"

	// ctrl+shift
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlShift}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlShift}:
		return "<ctrl-shift-space>"

	// ctrl+alt
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlAlt}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlAlt}:
		return "<ctrl-alt-space>"

	// ctrl+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlMeta}:
		return "<ctrl-meta-space>"

	// ctrl+shift+alt
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlShiftAlt}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt-space>"

	// ctrl+shift+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlShiftMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta-space>"

	// ctrl+alt+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlAltMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta-space>"

	// shift+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModShiftMeta}:
		return "<shift-meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModShiftMeta}:
		return "<shift-meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModShiftMeta}:
		return "<shift-meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModShiftMeta}:
		return "<shift-meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModShiftMeta}:
		return "<shift-meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModShiftMeta}:
		return "<shift-meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModShiftMeta}:
		return "<shift-meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModShiftMeta}:
		return "<shift-meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModShiftMeta}:
		return "<shift-meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModShiftMeta}:
		return "<shift-meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModShiftMeta}:
		return "<shift-meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModShiftMeta}:
		return "<shift-meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModShiftMeta}:
		return "<shift-meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModShiftMeta}:
		return "<shift-meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModShiftMeta}:
		return "<shift-meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModShiftMeta}:
		return "<shift-meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModShiftMeta}:
		return "<shift-meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModShiftMeta}:
		return "<shift-meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModShiftMeta}:
		return "<shift-meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModShiftMeta}:
		return "<shift-meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModShiftMeta}:
		return "<shift-meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModShiftMeta}:
		return "<shift-meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModShiftMeta}:
		return "<shift-meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModShiftMeta}:
		return "<shift-meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModShiftMeta}:
		return "<shift-meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModShiftMeta}:
		return "<shift-meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModShiftMeta}:
		return "<shift-meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModShiftMeta}:
		return "<shift-meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModShiftMeta}:
		return "<shift-meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModShiftMeta}:
		return "<shift-meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModShiftMeta}:
		return "<shift-meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModShiftMeta}:
		return "<shift-meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModShiftMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModShiftMeta}:
		return "<shift-meta-space>"

	// alt+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAltMeta}:
		return "<alt-meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAltMeta}:
		return "<alt-meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAltMeta}:
		return "<alt-meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAltMeta}:
		return "<alt-meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAltMeta}:
		return "<alt-meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAltMeta}:
		return "<alt-meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAltMeta}:
		return "<alt-meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAltMeta}:
		return "<alt-meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAltMeta}:
		return "<alt-meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAltMeta}:
		return "<alt-meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAltMeta}:
		return "<alt-meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAltMeta}:
		return "<alt-meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltMeta}:
		return "<alt-meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltMeta}:
		return "<alt-meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAltMeta}:
		return "<alt-meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltMeta}:
		return "<alt-meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltMeta}:
		return "<alt-meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltMeta}:
		return "<alt-meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltMeta}:
		return "<alt-meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltMeta}:
		return "<alt-meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltMeta}:
		return "<alt-meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltMeta}:
		return "<alt-meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltMeta}:
		return "<alt-meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltMeta}:
		return "<alt-meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAltMeta}:
		return "<alt-meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltMeta}:
		return "<alt-meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltMeta}:
		return "<alt-meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltMeta}:
		return "<alt-meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAltMeta}:
		return "<alt-meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAltMeta}:
		return "<alt-meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltMeta}:
		return "<alt-meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltMeta}:
		return "<alt-meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAltMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAltMeta}:
		return "<alt-meta-space>"

	// alt+shift+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAltShiftMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta-space>"

	// alt+shift
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAltShift}:
		return "<alt-shift-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAltShift}:
		return "<alt-shift-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAltShift}:
		return "<alt-shift-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAltShift}:
		return "<alt-shift-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAltShift}:
		return "<alt-shift-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAltShift}:
		return "<alt-shift-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAltShift}:
		return "<alt-shift-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAltShift}:
		return "<alt-shift-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAltShift}:
		return "<alt-shift-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAltShift}:
		return "<alt-shift-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAltShift}:
		return "<alt-shift-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAltShift}:
		return "<alt-shift-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltShift}:
		return "<alt-shift-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltShift}:
		return "<alt-shift-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAltShift}:
		return "<alt-shift-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltShift}:
		return "<alt-shift-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltShift}:
		return "<alt-shift-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltShift}:
		return "<alt-shift-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltShift}:
		return "<alt-shift-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltShift}:
		return "<alt-shift-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltShift}:
		return "<alt-shift-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltShift}:
		return "<alt-shift-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltShift}:
		return "<alt-shift-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltShift}:
		return "<alt-shift-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAltShift}:
		return "<alt-shift-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltShift}:
		return "<alt-shift-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltShift}:
		return "<alt-shift-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltShift}:
		return "<alt-shift-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAltShift}:
		return "<alt-shift-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAltShift}:
		return "<alt-shift-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltShift}:
		return "<alt-shift-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltShift}:
		return "<alt-shift-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAltShift}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAltShift}:
		return "<alt-shift-space>"

	// modifiers only
	case term.KeyComb{Mod: term.ModAlt}:
		return "<alt>"
	case term.KeyComb{Mod: term.ModShift}:
		return "<shift>"
	case term.KeyComb{Mod: term.ModMeta}:
		return "<meta>"
	case term.KeyComb{Mod: term.ModCtrl}:
		return "<ctrl>"
	case term.KeyComb{Mod: term.ModCtrlShift}:
		return "<ctrl-shift>"
	case term.KeyComb{Mod: term.ModCtrlAlt}:
		return "<ctrl-alt>"
	case term.KeyComb{Mod: term.ModCtrlMeta}:
		return "<ctrl-meta>"
	case term.KeyComb{Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt>"
	case term.KeyComb{Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta>"
	case term.KeyComb{Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta>"
	case term.KeyComb{Mod: term.ModShiftMeta}:
		return "<shift-meta>"
	case term.KeyComb{Mod: term.ModAltMeta}:
		return "<alt-meta>"
	case term.KeyComb{Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta>"
	case term.KeyComb{Mod: term.ModAltShift}:
		return "<alt-shift>"

	default:
		return "<INVALID>"
	}
}

// KeyCombShortString returns the shorthand of the long string representation
// of this term.KeyComb.
func KeyCombShortString(k term.KeyComb) string {
	if k.Ch != 0 && k.Key != term.KeySpace {
		ch, mod := processCharacter(k.Ch, k.Mod)
		// if Ch is set, then ignore key for representing this key comb
		switch mod {
		case term.ModMeta:
			return fmt.Sprintf("<m-%s>", ch)
		case term.ModAlt:
			return fmt.Sprintf("<a-%s>", ch)
		case term.ModShift:
			return fmt.Sprintf("<s-%s>", ch)
		case term.ModCtrl:
			return fmt.Sprintf("<c-%s>", ch)
		case term.ModCtrlShift:
			return fmt.Sprintf("<c-s-%s>", ch)
		case term.ModCtrlAlt:
			return fmt.Sprintf("<c-a-%s>", ch)
		case term.ModCtrlMeta:
			return fmt.Sprintf("<c-m-%s>", ch)
		case term.ModCtrlShiftAlt:
			return fmt.Sprintf("<c-s-a-%s>", ch)
		case term.ModCtrlShiftMeta:
			return fmt.Sprintf("<c-s-m-%s>", ch)
		case term.ModCtrlAltMeta:
			return fmt.Sprintf("<c-a-m-%s>", ch)
		case term.ModShiftMeta:
			return fmt.Sprintf("<s-m-%s>", ch)
		case term.ModAltMeta:
			return fmt.Sprintf("<a-m-%s>", ch)
		case term.ModAltShiftMeta:
			return fmt.Sprintf("<a-s-m-%s>", ch)
		case term.ModAltShift:
			return fmt.Sprintf("<a-s-%s>", ch)
		default:
			return string(ch)
		}
	}

	switch k {
	case term.KeyComb{Key: term.KeyF1}:
		return "<f1>"
	case term.KeyComb{Key: term.KeyF2}:
		return "<f2>"
	case term.KeyComb{Key: term.KeyF3}:
		return "<f3>"
	case term.KeyComb{Key: term.KeyF4}:
		return "<f4>"
	case term.KeyComb{Key: term.KeyF5}:
		return "<f5>"
	case term.KeyComb{Key: term.KeyF6}:
		return "<f6>"
	case term.KeyComb{Key: term.KeyF7}:
		return "<f7>"
	case term.KeyComb{Key: term.KeyF8}:
		return "<f8>"
	case term.KeyComb{Key: term.KeyF9}:
		return "<f9>"
	case term.KeyComb{Key: term.KeyF10}:
		return "<f10>"
	case term.KeyComb{Key: term.KeyF11}:
		return "<f11>"
	case term.KeyComb{Key: term.KeyF12}:
		return "<f12>"
	case term.KeyComb{Key: term.KeyInsert}:
		return "<insert>"
	case term.KeyComb{Key: term.KeyDelete}:
		return "<delete>"
	case term.KeyComb{Key: term.KeyHome}:
		return "<home>"
	case term.KeyComb{Key: term.KeyEnd}:
		return "<end>"
	case term.KeyComb{Key: term.KeyPgup}:
		return "<pgup>"
	case term.KeyComb{Key: term.KeyPgdn}:
		return "<pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp}:
		return "<up>"
	case term.KeyComb{Key: term.KeyArrowDown}:
		return "<down>"
	case term.KeyComb{Key: term.KeyArrowLeft}:
		return "<left>"
	case term.KeyComb{Key: term.KeyArrowRight}:
		return "<right>"
	case term.KeyComb{Key: term.MouseLeft}:
		return "<mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle}:
		return "<mouse-middle>"
	case term.KeyComb{Key: term.MouseRight}:
		return "<mouse-right>"
	case term.KeyComb{Key: term.MouseRelease}:
		return "<mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp}:
		return "<mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown}:
		return "<mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace}:
		return "<backspace>"
	case term.KeyComb{Key: term.KeyTab}:
		return "<tab>"
	case term.KeyComb{Key: term.KeyEnter}:
		return "<enter>"
	case term.KeyComb{Key: term.KeyEsc}:
		return "<esc>"
	case term.KeyComb{Key: term.KeySpace, Ch: ' '}, term.KeyComb{Key: term.KeySpace}:
		return "<space>"

	// meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModMeta}:
		return "<m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModMeta}:
		return "<m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModMeta}:
		return "<m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModMeta}:
		return "<m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModMeta}:
		return "<m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModMeta}:
		return "<m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModMeta}:
		return "<m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModMeta}:
		return "<m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModMeta}:
		return "<m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModMeta}:
		return "<m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModMeta}:
		return "<m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModMeta}:
		return "<m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModMeta}:
		return "<m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModMeta}:
		return "<m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModMeta}:
		return "<m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModMeta}:
		return "<m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModMeta}:
		return "<m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModMeta}:
		return "<m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModMeta}:
		return "<m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModMeta}:
		return "<m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModMeta}:
		return "<m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModMeta}:
		return "<m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModMeta}:
		return "<m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModMeta}:
		return "<m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModMeta}:
		return "<m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModMeta}:
		return "<m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModMeta}:
		return "<m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModMeta}:
		return "<m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModMeta}:
		return "<m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModMeta}:
		return "<m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModMeta}:
		return "<m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModMeta}:
		return "<m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModMeta}:
		return "<m-space>"

	// alt
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAlt}:
		return "<a-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAlt}:
		return "<a-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAlt}:
		return "<a-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAlt}:
		return "<a-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAlt}:
		return "<a-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAlt}:
		return "<a-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAlt}:
		return "<a-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAlt}:
		return "<a-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAlt}:
		return "<a-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAlt}:
		return "<a-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAlt}:
		return "<a-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAlt}:
		return "<a-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAlt}:
		return "<a-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAlt}:
		return "<a-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAlt}:
		return "<a-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAlt}:
		return "<a-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAlt}:
		return "<a-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAlt}:
		return "<a-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAlt}:
		return "<a-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAlt}:
		return "<a-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAlt}:
		return "<a-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAlt}:
		return "<a-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAlt}:
		return "<a-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAlt}:
		return "<a-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAlt}:
		return "<a-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAlt}:
		return "<a-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAlt}:
		return "<a-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAlt}:
		return "<a-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAlt}:
		return "<a-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAlt}:
		return "<a-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAlt}:
		return "<a-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAlt}:
		return "<a-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAlt}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAlt}:
		return "<a-space>"

	// shift
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModShift}:
		return "<s-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModShift}:
		return "<s-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModShift}:
		return "<s-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModShift}:
		return "<s-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModShift}:
		return "<s-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModShift}:
		return "<s-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModShift}:
		return "<s-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModShift}:
		return "<s-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModShift}:
		return "<s-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModShift}:
		return "<s-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModShift}:
		return "<s-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModShift}:
		return "<s-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModShift}:
		return "<s-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModShift}:
		return "<s-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModShift}:
		return "<s-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModShift}:
		return "<s-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModShift}:
		return "<s-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModShift}:
		return "<s-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModShift}:
		return "<s-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModShift}:
		return "<s-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModShift}:
		return "<s-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModShift}:
		return "<s-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModShift}:
		return "<s-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModShift}:
		return "<s-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModShift}:
		return "<s-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModShift}:
		return "<s-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModShift}:
		return "<s-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModShift}:
		return "<s-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModShift}:
		return "<s-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModShift}:
		return "<s-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModShift}:
		return "<s-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModShift}:
		return "<s-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModShift}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModShift}:
		return "<s-space>"

	// ctrl
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrl}:
		return "<c-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrl}:
		return "<c-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrl}:
		return "<c-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrl}:
		return "<c-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrl}:
		return "<c-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrl}:
		return "<c-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrl}:
		return "<c-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrl}:
		return "<c-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrl}:
		return "<c-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrl}:
		return "<c-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrl}:
		return "<c-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrl}:
		return "<c-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrl}:
		return "<c-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrl}:
		return "<c-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrl}:
		return "<c-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrl}:
		return "<c-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrl}:
		return "<c-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrl}:
		return "<c-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrl}:
		return "<c-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrl}:
		return "<c-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrl}:
		return "<c-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrl}:
		return "<c-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrl}:
		return "<c-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrl}:
		return "<c-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrl}:
		return "<c-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrl}:
		return "<c-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrl}:
		return "<c-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrl}:
		return "<c-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrl}:
		return "<c-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrl}:
		return "<c-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrl}:
		return "<c-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrl}:
		return "<c-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrl}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrl}:
		return "<c-space>"

	// ctrl+shift
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShift}:
		return "<c-s-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShift}:
		return "<c-s-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShift}:
		return "<c-s-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShift}:
		return "<c-s-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShift}:
		return "<c-s-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShift}:
		return "<c-s-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShift}:
		return "<c-s-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShift}:
		return "<c-s-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShift}:
		return "<c-s-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShift}:
		return "<c-s-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShift}:
		return "<c-s-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShift}:
		return "<c-s-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShift}:
		return "<c-s-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShift}:
		return "<c-s-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShift}:
		return "<c-s-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShift}:
		return "<c-s-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShift}:
		return "<c-s-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShift}:
		return "<c-s-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShift}:
		return "<c-s-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShift}:
		return "<c-s-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShift}:
		return "<c-s-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShift}:
		return "<c-s-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShift}:
		return "<c-s-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShift}:
		return "<c-s-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShift}:
		return "<c-s-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShift}:
		return "<c-s-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShift}:
		return "<c-s-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShift}:
		return "<c-s-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlShift}:
		return "<c-s-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlShift}:
		return "<c-s-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShift}:
		return "<c-s-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShift}:
		return "<c-s-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlShift}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlShift}:
		return "<c-s-space>"

	// ctrl+alt
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlAlt}:
		return "<c-a-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlAlt}:
		return "<c-a-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlAlt}:
		return "<c-a-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlAlt}:
		return "<c-a-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlAlt}:
		return "<c-a-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlAlt}:
		return "<c-a-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlAlt}:
		return "<c-a-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlAlt}:
		return "<c-a-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlAlt}:
		return "<c-a-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlAlt}:
		return "<c-a-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlAlt}:
		return "<c-a-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlAlt}:
		return "<c-a-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlAlt}:
		return "<c-a-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlAlt}:
		return "<c-a-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlAlt}:
		return "<c-a-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlAlt}:
		return "<c-a-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlAlt}:
		return "<c-a-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlAlt}:
		return "<c-a-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlAlt}:
		return "<c-a-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlAlt}:
		return "<c-a-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlAlt}:
		return "<c-a-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlAlt}:
		return "<c-a-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlAlt}:
		return "<c-a-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlAlt}:
		return "<c-a-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlAlt}:
		return "<c-a-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlAlt}:
		return "<c-a-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlAlt}:
		return "<c-a-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlAlt}:
		return "<c-a-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlAlt}:
		return "<c-a-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlAlt}:
		return "<c-a-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlAlt}:
		return "<c-a-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlAlt}:
		return "<c-a-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlAlt}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlAlt}:
		return "<c-a-space>"

	// ctrl+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlMeta}:
		return "<c-m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlMeta}:
		return "<c-m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlMeta}:
		return "<c-m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlMeta}:
		return "<c-m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlMeta}:
		return "<c-m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlMeta}:
		return "<c-m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlMeta}:
		return "<c-m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlMeta}:
		return "<c-m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlMeta}:
		return "<c-m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlMeta}:
		return "<c-m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlMeta}:
		return "<c-m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlMeta}:
		return "<c-m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlMeta}:
		return "<c-m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlMeta}:
		return "<c-m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlMeta}:
		return "<c-m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlMeta}:
		return "<c-m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlMeta}:
		return "<c-m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlMeta}:
		return "<c-m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlMeta}:
		return "<c-m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlMeta}:
		return "<c-m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlMeta}:
		return "<c-m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlMeta}:
		return "<c-m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlMeta}:
		return "<c-m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlMeta}:
		return "<c-m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlMeta}:
		return "<c-m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlMeta}:
		return "<c-m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlMeta}:
		return "<c-m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlMeta}:
		return "<c-m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlMeta}:
		return "<c-m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlMeta}:
		return "<c-m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlMeta}:
		return "<c-m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlMeta}:
		return "<c-m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlMeta}:
		return "<c-m-space>"

	// ctrl+shift+alt
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlShiftAlt}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlShiftAlt}:
		return "<c-s-a-space>"

	// ctrl+shift+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlShiftMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlShiftMeta}:
		return "<c-s-m-space>"

	// ctrl+alt+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModCtrlAltMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModCtrlAltMeta}:
		return "<c-a-m-space>"

	// shift+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModShiftMeta}:
		return "<s-m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModShiftMeta}:
		return "<s-m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModShiftMeta}:
		return "<s-m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModShiftMeta}:
		return "<s-m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModShiftMeta}:
		return "<s-m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModShiftMeta}:
		return "<s-m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModShiftMeta}:
		return "<s-m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModShiftMeta}:
		return "<s-m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModShiftMeta}:
		return "<s-m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModShiftMeta}:
		return "<s-m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModShiftMeta}:
		return "<s-m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModShiftMeta}:
		return "<s-m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModShiftMeta}:
		return "<s-m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModShiftMeta}:
		return "<s-m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModShiftMeta}:
		return "<s-m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModShiftMeta}:
		return "<s-m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModShiftMeta}:
		return "<s-m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModShiftMeta}:
		return "<s-m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModShiftMeta}:
		return "<s-m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModShiftMeta}:
		return "<s-m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModShiftMeta}:
		return "<s-m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModShiftMeta}:
		return "<s-m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModShiftMeta}:
		return "<s-m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModShiftMeta}:
		return "<s-m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModShiftMeta}:
		return "<s-m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModShiftMeta}:
		return "<s-m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModShiftMeta}:
		return "<s-m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModShiftMeta}:
		return "<s-m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModShiftMeta}:
		return "<s-m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModShiftMeta}:
		return "<s-m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModShiftMeta}:
		return "<s-m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModShiftMeta}:
		return "<s-m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModShiftMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModShiftMeta}:
		return "<s-m-space>"

	// alt+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAltMeta}:
		return "<a-m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAltMeta}:
		return "<a-m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAltMeta}:
		return "<a-m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAltMeta}:
		return "<a-m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAltMeta}:
		return "<a-m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAltMeta}:
		return "<a-m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAltMeta}:
		return "<a-m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAltMeta}:
		return "<a-m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAltMeta}:
		return "<a-m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAltMeta}:
		return "<a-m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAltMeta}:
		return "<a-m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAltMeta}:
		return "<a-m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltMeta}:
		return "<a-m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltMeta}:
		return "<a-m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAltMeta}:
		return "<a-m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltMeta}:
		return "<a-m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltMeta}:
		return "<a-m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltMeta}:
		return "<a-m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltMeta}:
		return "<a-m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltMeta}:
		return "<a-m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltMeta}:
		return "<a-m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltMeta}:
		return "<a-m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltMeta}:
		return "<a-m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltMeta}:
		return "<a-m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAltMeta}:
		return "<a-m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltMeta}:
		return "<a-m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltMeta}:
		return "<a-m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltMeta}:
		return "<a-m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAltMeta}:
		return "<a-m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAltMeta}:
		return "<a-m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltMeta}:
		return "<a-m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltMeta}:
		return "<a-m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAltMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAltMeta}:
		return "<a-m-space>"

	// alt+shift+meta
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAltShiftMeta}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAltShiftMeta}:
		return "<a-s-m-space>"

	// alt+shift
	case term.KeyComb{Key: term.KeyF1, Mod: term.ModAltShift}:
		return "<a-s-f1>"
	case term.KeyComb{Key: term.KeyF2, Mod: term.ModAltShift}:
		return "<a-s-f2>"
	case term.KeyComb{Key: term.KeyF3, Mod: term.ModAltShift}:
		return "<a-s-f3>"
	case term.KeyComb{Key: term.KeyF4, Mod: term.ModAltShift}:
		return "<a-s-f4>"
	case term.KeyComb{Key: term.KeyF5, Mod: term.ModAltShift}:
		return "<a-s-f5>"
	case term.KeyComb{Key: term.KeyF6, Mod: term.ModAltShift}:
		return "<a-s-f6>"
	case term.KeyComb{Key: term.KeyF7, Mod: term.ModAltShift}:
		return "<a-s-f7>"
	case term.KeyComb{Key: term.KeyF8, Mod: term.ModAltShift}:
		return "<a-s-f8>"
	case term.KeyComb{Key: term.KeyF9, Mod: term.ModAltShift}:
		return "<a-s-f9>"
	case term.KeyComb{Key: term.KeyF10, Mod: term.ModAltShift}:
		return "<a-s-f10>"
	case term.KeyComb{Key: term.KeyF11, Mod: term.ModAltShift}:
		return "<a-s-f11>"
	case term.KeyComb{Key: term.KeyF12, Mod: term.ModAltShift}:
		return "<a-s-f12>"
	case term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltShift}:
		return "<a-s-insert>"
	case term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltShift}:
		return "<a-s-delete>"
	case term.KeyComb{Key: term.KeyHome, Mod: term.ModAltShift}:
		return "<a-s-home>"
	case term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltShift}:
		return "<a-s-end>"
	case term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltShift}:
		return "<a-s-pgup>"
	case term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltShift}:
		return "<a-s-pgdn>"
	case term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltShift}:
		return "<a-s-up>"
	case term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltShift}:
		return "<a-s-down>"
	case term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltShift}:
		return "<a-s-left>"
	case term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltShift}:
		return "<a-s-right>"
	case term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltShift}:
		return "<a-s-mouse-left>"
	case term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltShift}:
		return "<a-s-mouse-middle>"
	case term.KeyComb{Key: term.MouseRight, Mod: term.ModAltShift}:
		return "<a-s-mouse-right>"
	case term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltShift}:
		return "<a-s-mouse-release>"
	case term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltShift}:
		return "<a-s-mouse-wheel-up>"
	case term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltShift}:
		return "<a-s-mouse-wheel-down>"
	case term.KeyComb{Key: term.KeyBackspace, Mod: term.ModAltShift}:
		return "<a-s-backspace>"
	case term.KeyComb{Key: term.KeyTab, Mod: term.ModAltShift}:
		return "<a-s-tab>"
	case term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltShift}:
		return "<a-s-enter>"
	case term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltShift}:
		return "<a-s-esc>"
	case term.KeyComb{Key: term.KeySpace, Mod: term.ModAltShift}, term.KeyComb{Ch: ' ', Key: term.KeySpace, Mod: term.ModAltShift}:
		return "<a-s-space>"

	// modifiers only
	case term.KeyComb{Mod: term.ModAlt}:
		return "<alt>"
	case term.KeyComb{Mod: term.ModShift}:
		return "<shift>"
	case term.KeyComb{Mod: term.ModMeta}:
		return "<meta>"
	case term.KeyComb{Mod: term.ModCtrl}:
		return "<ctrl>"
	case term.KeyComb{Mod: term.ModCtrlShift}:
		return "<ctrl-shift>"
	case term.KeyComb{Mod: term.ModCtrlAlt}:
		return "<ctrl-alt>"
	case term.KeyComb{Mod: term.ModCtrlMeta}:
		return "<ctrl-meta>"
	case term.KeyComb{Mod: term.ModCtrlShiftAlt}:
		return "<ctrl-shift-alt>"
	case term.KeyComb{Mod: term.ModCtrlShiftMeta}:
		return "<ctrl-shift-meta>"
	case term.KeyComb{Mod: term.ModCtrlAltMeta}:
		return "<ctrl-alt-meta>"
	case term.KeyComb{Mod: term.ModShiftMeta}:
		return "<shift-meta>"
	case term.KeyComb{Mod: term.ModAltMeta}:
		return "<alt-meta>"
	case term.KeyComb{Mod: term.ModAltShiftMeta}:
		return "<alt-shift-meta>"
	case term.KeyComb{Mod: term.ModAltShift}:
		return "<alt-shift>"
	default:
		return "<INVALID>"
	}
}

func processCharacter(ch rune, mod term.Modifier) (unshifCh string, shiftedMod term.Modifier) {
	switch mod {
	case term.ModAlt:
		shiftedMod = term.ModAltShift
	case term.ModMeta:
		shiftedMod = term.ModShiftMeta
	case term.ModCtrl:
		shiftedMod = term.ModCtrlShift
	case term.ModCtrlAlt:
		shiftedMod = term.ModCtrlShiftAlt
	case term.ModCtrlMeta:
		shiftedMod = term.ModCtrlShiftMeta
	case term.ModCtrlAltMeta:
		shiftedMod = term.ModCtrlShiftMeta // cannot use all the modifiers at once, drop alt
	case term.ModAltMeta:
		shiftedMod = term.ModAltShiftMeta
	case term.ModShift, term.ModCtrlShift,
		term.ModCtrlShiftAlt, term.ModCtrlShiftMeta,
		term.ModShiftMeta, term.ModAltShiftMeta,
		term.ModAltShift:
		shiftedMod = mod
	case 0:
		shiftedMod = term.ModShift
	default:
		shiftedMod = mod
	}

	switch ch {
	case 'A':
		return "a", shiftedMod
	case 'B':
		return "b", shiftedMod
	case 'C':
		return "c", shiftedMod
	case 'D':
		return "d", shiftedMod
	case 'E':
		return "e", shiftedMod
	case 'F':
		return "f", shiftedMod
	case 'G':
		return "g", shiftedMod
	case 'H':
		return "h", shiftedMod
	case 'I':
		return "i", shiftedMod
	case 'J':
		return "j", shiftedMod
	case 'K':
		return "k", shiftedMod
	case 'L':
		return "l", shiftedMod
	case 'M':
		return "m", shiftedMod
	case 'N':
		return "n", shiftedMod
	case 'O':
		return "o", shiftedMod
	case 'P':
		return "p", shiftedMod
	case 'Q':
		return "q", shiftedMod
	case 'R':
		return "r", shiftedMod
	case 'S':
		return "s", shiftedMod
	case 'T':
		return "t", shiftedMod
	case 'U':
		return "u", shiftedMod
	case 'V':
		return "v", shiftedMod
	case 'W':
		return "w", shiftedMod
	case 'X':
		return "x", shiftedMod
	case 'Y':
		return "y", shiftedMod
	case 'Z':
		return "z", shiftedMod
	case '_':
		return "-", shiftedMod
	case ')':
		return "0", shiftedMod
	case '!':
		return "1", shiftedMod
	case '@':
		return "2", shiftedMod
	case '#':
		return "3", shiftedMod
	case '$':
		return "4", shiftedMod
	case '%':
		return "5", shiftedMod
	case '^':
		return "6", shiftedMod
	case '&':
		return "7", shiftedMod
	case '*':
		return "8", shiftedMod
	case '(':
		return "9", shiftedMod
	case '+':
		return "=", shiftedMod
	case '<':
		return ",", shiftedMod
	case '{':
		return "[", shiftedMod
	case '}':
		return "]", shiftedMod
	case '~':
		return "`", shiftedMod
	case '?':
		return "/", shiftedMod
	case '|':
		return "\\\\", shiftedMod
	case '>':
		return ".", shiftedMod
	case '"':
		return "'", shiftedMod
	case ':':
		return ";", shiftedMod
	case '\\':
		return "\\\\", mod
	default:
		return string(ch), mod
	}
}
