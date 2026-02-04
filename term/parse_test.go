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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestParseKeys(t *testing.T) {
	suite := []term.KeyComb{
		{Key: term.KeyF1},
		{Key: term.KeyF2},
		{Key: term.KeyF3},
		{Key: term.KeyF4},
		{Key: term.KeyF5},
		{Key: term.KeyF6},
		{Key: term.KeyF7},
		{Key: term.KeyF8},
		{Key: term.KeyF9},
		{Key: term.KeyF10},
		{Key: term.KeyF11},
		{Key: term.KeyF12},
		{Key: term.KeyInsert},
		{Key: term.KeyDelete},
		{Key: term.KeyHome},
		{Key: term.KeyEnd},
		{Key: term.KeyPgup},
		{Key: term.KeyPgdn},
		{Key: term.KeyArrowUp},
		{Key: term.KeyArrowDown},
		{Key: term.KeyArrowLeft},
		{Key: term.KeyArrowRight},
		{Key: term.MouseLeft},
		{Key: term.MouseMiddle},
		{Key: term.MouseRight},
		{Key: term.MouseRelease},
		{Key: term.MouseWheelUp},
		{Key: term.MouseWheelDown},
		{Key: term.KeyBackspace},
		{Key: term.KeyTab},
		{Key: term.KeyEnter},
		{Key: term.KeyEsc},
		{Key: term.KeySpace},

		{Key: term.KeyF1, Mod: term.ModAlt},
		{Key: term.KeyF2, Mod: term.ModAlt},
		{Key: term.KeyF3, Mod: term.ModAlt},
		{Key: term.KeyF4, Mod: term.ModAlt},
		{Key: term.KeyF5, Mod: term.ModAlt},
		{Key: term.KeyF6, Mod: term.ModAlt},
		{Key: term.KeyF7, Mod: term.ModAlt},
		{Key: term.KeyF8, Mod: term.ModAlt},
		{Key: term.KeyF9, Mod: term.ModAlt},
		{Key: term.KeyF10, Mod: term.ModAlt},
		{Key: term.KeyF11, Mod: term.ModAlt},
		{Key: term.KeyF12, Mod: term.ModAlt},
		{Key: term.KeyInsert, Mod: term.ModAlt},
		{Key: term.KeyDelete, Mod: term.ModAlt},
		{Key: term.KeyHome, Mod: term.ModAlt},
		{Key: term.KeyEnd, Mod: term.ModAlt},
		{Key: term.KeyPgup, Mod: term.ModAlt},
		{Key: term.KeyPgdn, Mod: term.ModAlt},
		{Key: term.KeyArrowUp, Mod: term.ModAlt},
		{Key: term.KeyArrowDown, Mod: term.ModAlt},
		{Key: term.KeyArrowLeft, Mod: term.ModAlt},
		{Key: term.KeyArrowRight, Mod: term.ModAlt},
		{Key: term.MouseLeft, Mod: term.ModAlt},
		{Key: term.MouseMiddle, Mod: term.ModAlt},
		{Key: term.MouseRight, Mod: term.ModAlt},
		{Key: term.MouseRelease, Mod: term.ModAlt},
		{Key: term.MouseWheelUp, Mod: term.ModAlt},
		{Key: term.MouseWheelDown, Mod: term.ModAlt},
		{Key: term.KeyBackspace, Mod: term.ModAlt},
		{Key: term.KeyTab, Mod: term.ModAlt},
		{Key: term.KeyEnter, Mod: term.ModAlt},
		{Key: term.KeyEsc, Mod: term.ModAlt},
		{Key: term.KeySpace, Mod: term.ModAlt},

		{Key: term.KeyF1, Mod: term.ModCtrl},
		{Key: term.KeyF2, Mod: term.ModCtrl},
		{Key: term.KeyF3, Mod: term.ModCtrl},
		{Key: term.KeyF4, Mod: term.ModCtrl},
		{Key: term.KeyF5, Mod: term.ModCtrl},
		{Key: term.KeyF6, Mod: term.ModCtrl},
		{Key: term.KeyF7, Mod: term.ModCtrl},
		{Key: term.KeyF8, Mod: term.ModCtrl},
		{Key: term.KeyF9, Mod: term.ModCtrl},
		{Key: term.KeyF10, Mod: term.ModCtrl},
		{Key: term.KeyF11, Mod: term.ModCtrl},
		{Key: term.KeyF12, Mod: term.ModCtrl},
		{Key: term.KeyInsert, Mod: term.ModCtrl},
		{Key: term.KeyDelete, Mod: term.ModCtrl},
		{Key: term.KeyHome, Mod: term.ModCtrl},
		{Key: term.KeyEnd, Mod: term.ModCtrl},
		{Key: term.KeyPgup, Mod: term.ModCtrl},
		{Key: term.KeyPgdn, Mod: term.ModCtrl},
		{Key: term.KeyArrowUp, Mod: term.ModCtrl},
		{Key: term.KeyArrowDown, Mod: term.ModCtrl},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrl},
		{Key: term.KeyArrowRight, Mod: term.ModCtrl},
		{Key: term.MouseLeft, Mod: term.ModCtrl},
		{Key: term.MouseMiddle, Mod: term.ModCtrl},
		{Key: term.MouseRight, Mod: term.ModCtrl},
		{Key: term.MouseRelease, Mod: term.ModCtrl},
		{Key: term.MouseWheelUp, Mod: term.ModCtrl},
		{Key: term.MouseWheelDown, Mod: term.ModCtrl},
		{Key: term.KeyBackspace, Mod: term.ModCtrl},
		{Key: term.KeyTab, Mod: term.ModCtrl},
		{Key: term.KeyEnter, Mod: term.ModCtrl},
		{Key: term.KeyEsc, Mod: term.ModCtrl},
		{Key: term.KeySpace, Mod: term.ModCtrl},

		{Key: term.KeyF1, Mod: term.ModShift},
		{Key: term.KeyF2, Mod: term.ModShift},
		{Key: term.KeyF3, Mod: term.ModShift},
		{Key: term.KeyF4, Mod: term.ModShift},
		{Key: term.KeyF5, Mod: term.ModShift},
		{Key: term.KeyF6, Mod: term.ModShift},
		{Key: term.KeyF7, Mod: term.ModShift},
		{Key: term.KeyF8, Mod: term.ModShift},
		{Key: term.KeyF9, Mod: term.ModShift},
		{Key: term.KeyF10, Mod: term.ModShift},
		{Key: term.KeyF11, Mod: term.ModShift},
		{Key: term.KeyF12, Mod: term.ModShift},
		{Key: term.KeyInsert, Mod: term.ModShift},
		{Key: term.KeyDelete, Mod: term.ModShift},
		{Key: term.KeyHome, Mod: term.ModShift},
		{Key: term.KeyEnd, Mod: term.ModShift},
		{Key: term.KeyPgup, Mod: term.ModShift},
		{Key: term.KeyPgdn, Mod: term.ModShift},
		{Key: term.KeyArrowUp, Mod: term.ModShift},
		{Key: term.KeyArrowDown, Mod: term.ModShift},
		{Key: term.KeyArrowLeft, Mod: term.ModShift},
		{Key: term.KeyArrowRight, Mod: term.ModShift},
		{Key: term.MouseLeft, Mod: term.ModShift},
		{Key: term.MouseMiddle, Mod: term.ModShift},
		{Key: term.MouseRight, Mod: term.ModShift},
		{Key: term.MouseRelease, Mod: term.ModShift},
		{Key: term.MouseWheelUp, Mod: term.ModShift},
		{Key: term.MouseWheelDown, Mod: term.ModShift},
		{Key: term.KeyBackspace, Mod: term.ModShift},
		{Key: term.KeyTab, Mod: term.ModShift},
		{Key: term.KeyEnter, Mod: term.ModShift},
		{Key: term.KeyEsc, Mod: term.ModShift},
		{Key: term.KeySpace, Mod: term.ModShift},

		{Key: term.KeyF1, Mod: term.ModMeta},
		{Key: term.KeyF2, Mod: term.ModMeta},
		{Key: term.KeyF3, Mod: term.ModMeta},
		{Key: term.KeyF4, Mod: term.ModMeta},
		{Key: term.KeyF5, Mod: term.ModMeta},
		{Key: term.KeyF6, Mod: term.ModMeta},
		{Key: term.KeyF7, Mod: term.ModMeta},
		{Key: term.KeyF8, Mod: term.ModMeta},
		{Key: term.KeyF9, Mod: term.ModMeta},
		{Key: term.KeyF10, Mod: term.ModMeta},
		{Key: term.KeyF11, Mod: term.ModMeta},
		{Key: term.KeyF12, Mod: term.ModMeta},
		{Key: term.KeyInsert, Mod: term.ModMeta},
		{Key: term.KeyDelete, Mod: term.ModMeta},
		{Key: term.KeyHome, Mod: term.ModMeta},
		{Key: term.KeyEnd, Mod: term.ModMeta},
		{Key: term.KeyPgup, Mod: term.ModMeta},
		{Key: term.KeyPgdn, Mod: term.ModMeta},
		{Key: term.KeyArrowUp, Mod: term.ModMeta},
		{Key: term.KeyArrowDown, Mod: term.ModMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModMeta},
		{Key: term.KeyArrowRight, Mod: term.ModMeta},
		{Key: term.MouseLeft, Mod: term.ModMeta},
		{Key: term.MouseMiddle, Mod: term.ModMeta},
		{Key: term.MouseRight, Mod: term.ModMeta},
		{Key: term.MouseRelease, Mod: term.ModMeta},
		{Key: term.MouseWheelUp, Mod: term.ModMeta},
		{Key: term.MouseWheelDown, Mod: term.ModMeta},
		{Key: term.KeyBackspace, Mod: term.ModMeta},
		{Key: term.KeyTab, Mod: term.ModMeta},
		{Key: term.KeyEnter, Mod: term.ModMeta},
		{Key: term.KeyEsc, Mod: term.ModMeta},
		{Key: term.KeySpace, Mod: term.ModMeta},

		{Key: term.KeyF1, Mod: term.ModCtrlShift},
		{Key: term.KeyF2, Mod: term.ModCtrlShift},
		{Key: term.KeyF3, Mod: term.ModCtrlShift},
		{Key: term.KeyF4, Mod: term.ModCtrlShift},
		{Key: term.KeyF5, Mod: term.ModCtrlShift},
		{Key: term.KeyF6, Mod: term.ModCtrlShift},
		{Key: term.KeyF7, Mod: term.ModCtrlShift},
		{Key: term.KeyF8, Mod: term.ModCtrlShift},
		{Key: term.KeyF9, Mod: term.ModCtrlShift},
		{Key: term.KeyF10, Mod: term.ModCtrlShift},
		{Key: term.KeyF11, Mod: term.ModCtrlShift},
		{Key: term.KeyF12, Mod: term.ModCtrlShift},
		{Key: term.KeyInsert, Mod: term.ModCtrlShift},
		{Key: term.KeyDelete, Mod: term.ModCtrlShift},
		{Key: term.KeyHome, Mod: term.ModCtrlShift},
		{Key: term.KeyEnd, Mod: term.ModCtrlShift},
		{Key: term.KeyPgup, Mod: term.ModCtrlShift},
		{Key: term.KeyPgdn, Mod: term.ModCtrlShift},
		{Key: term.KeyArrowUp, Mod: term.ModCtrlShift},
		{Key: term.KeyArrowDown, Mod: term.ModCtrlShift},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrlShift},
		{Key: term.KeyArrowRight, Mod: term.ModCtrlShift},
		{Key: term.MouseLeft, Mod: term.ModCtrlShift},
		{Key: term.MouseMiddle, Mod: term.ModCtrlShift},
		{Key: term.MouseRight, Mod: term.ModCtrlShift},
		{Key: term.MouseRelease, Mod: term.ModCtrlShift},
		{Key: term.MouseWheelUp, Mod: term.ModCtrlShift},
		{Key: term.MouseWheelDown, Mod: term.ModCtrlShift},
		{Key: term.KeyBackspace, Mod: term.ModCtrlShift},
		{Key: term.KeyTab, Mod: term.ModCtrlShift},
		{Key: term.KeyEnter, Mod: term.ModCtrlShift},
		{Key: term.KeyEsc, Mod: term.ModCtrlShift},
		{Key: term.KeySpace, Mod: term.ModCtrlShift},

		{Key: term.KeyF1, Mod: term.ModCtrlAlt},
		{Key: term.KeyF2, Mod: term.ModCtrlAlt},
		{Key: term.KeyF3, Mod: term.ModCtrlAlt},
		{Key: term.KeyF4, Mod: term.ModCtrlAlt},
		{Key: term.KeyF5, Mod: term.ModCtrlAlt},
		{Key: term.KeyF6, Mod: term.ModCtrlAlt},
		{Key: term.KeyF7, Mod: term.ModCtrlAlt},
		{Key: term.KeyF8, Mod: term.ModCtrlAlt},
		{Key: term.KeyF9, Mod: term.ModCtrlAlt},
		{Key: term.KeyF10, Mod: term.ModCtrlAlt},
		{Key: term.KeyF11, Mod: term.ModCtrlAlt},
		{Key: term.KeyF12, Mod: term.ModCtrlAlt},
		{Key: term.KeyInsert, Mod: term.ModCtrlAlt},
		{Key: term.KeyDelete, Mod: term.ModCtrlAlt},
		{Key: term.KeyHome, Mod: term.ModCtrlAlt},
		{Key: term.KeyEnd, Mod: term.ModCtrlAlt},
		{Key: term.KeyPgup, Mod: term.ModCtrlAlt},
		{Key: term.KeyPgdn, Mod: term.ModCtrlAlt},
		{Key: term.KeyArrowUp, Mod: term.ModCtrlAlt},
		{Key: term.KeyArrowDown, Mod: term.ModCtrlAlt},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrlAlt},
		{Key: term.KeyArrowRight, Mod: term.ModCtrlAlt},
		{Key: term.MouseLeft, Mod: term.ModCtrlAlt},
		{Key: term.MouseMiddle, Mod: term.ModCtrlAlt},
		{Key: term.MouseRight, Mod: term.ModCtrlAlt},
		{Key: term.MouseRelease, Mod: term.ModCtrlAlt},
		{Key: term.MouseWheelUp, Mod: term.ModCtrlAlt},
		{Key: term.MouseWheelDown, Mod: term.ModCtrlAlt},
		{Key: term.KeyBackspace, Mod: term.ModCtrlAlt},
		{Key: term.KeyTab, Mod: term.ModCtrlAlt},
		{Key: term.KeyEnter, Mod: term.ModCtrlAlt},
		{Key: term.KeyEsc, Mod: term.ModCtrlAlt},
		{Key: term.KeySpace, Mod: term.ModCtrlAlt},

		{Key: term.KeyF1, Mod: term.ModCtrlMeta},
		{Key: term.KeyF2, Mod: term.ModCtrlMeta},
		{Key: term.KeyF3, Mod: term.ModCtrlMeta},
		{Key: term.KeyF4, Mod: term.ModCtrlMeta},
		{Key: term.KeyF5, Mod: term.ModCtrlMeta},
		{Key: term.KeyF6, Mod: term.ModCtrlMeta},
		{Key: term.KeyF7, Mod: term.ModCtrlMeta},
		{Key: term.KeyF8, Mod: term.ModCtrlMeta},
		{Key: term.KeyF9, Mod: term.ModCtrlMeta},
		{Key: term.KeyF10, Mod: term.ModCtrlMeta},
		{Key: term.KeyF11, Mod: term.ModCtrlMeta},
		{Key: term.KeyF12, Mod: term.ModCtrlMeta},
		{Key: term.KeyInsert, Mod: term.ModCtrlMeta},
		{Key: term.KeyDelete, Mod: term.ModCtrlMeta},
		{Key: term.KeyHome, Mod: term.ModCtrlMeta},
		{Key: term.KeyEnd, Mod: term.ModCtrlMeta},
		{Key: term.KeyPgup, Mod: term.ModCtrlMeta},
		{Key: term.KeyPgdn, Mod: term.ModCtrlMeta},
		{Key: term.KeyArrowUp, Mod: term.ModCtrlMeta},
		{Key: term.KeyArrowDown, Mod: term.ModCtrlMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrlMeta},
		{Key: term.KeyArrowRight, Mod: term.ModCtrlMeta},
		{Key: term.MouseLeft, Mod: term.ModCtrlMeta},
		{Key: term.MouseMiddle, Mod: term.ModCtrlMeta},
		{Key: term.MouseRight, Mod: term.ModCtrlMeta},
		{Key: term.MouseRelease, Mod: term.ModCtrlMeta},
		{Key: term.MouseWheelUp, Mod: term.ModCtrlMeta},
		{Key: term.MouseWheelDown, Mod: term.ModCtrlMeta},
		{Key: term.KeyBackspace, Mod: term.ModCtrlMeta},
		{Key: term.KeyTab, Mod: term.ModCtrlMeta},
		{Key: term.KeyEnter, Mod: term.ModCtrlMeta},
		{Key: term.KeyEsc, Mod: term.ModCtrlMeta},
		{Key: term.KeySpace, Mod: term.ModCtrlMeta},

		{Key: term.KeyF1, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF2, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF3, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF4, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF5, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF6, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF7, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF8, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF9, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF10, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF11, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyF12, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyInsert, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyDelete, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyHome, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyEnd, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyPgup, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyPgdn, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftAlt},
		{Key: term.MouseLeft, Mod: term.ModCtrlShiftAlt},
		{Key: term.MouseMiddle, Mod: term.ModCtrlShiftAlt},
		{Key: term.MouseRight, Mod: term.ModCtrlShiftAlt},
		{Key: term.MouseRelease, Mod: term.ModCtrlShiftAlt},
		{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftAlt},
		{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyBackspace, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyTab, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyEnter, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeyEsc, Mod: term.ModCtrlShiftAlt},
		{Key: term.KeySpace, Mod: term.ModCtrlShiftAlt},

		{Key: term.KeyF1, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF2, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF3, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF4, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF5, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF6, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF7, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF8, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF9, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF10, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF11, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyF12, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyInsert, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyDelete, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyHome, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyEnd, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyPgup, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyPgdn, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftMeta},
		{Key: term.MouseLeft, Mod: term.ModCtrlShiftMeta},
		{Key: term.MouseMiddle, Mod: term.ModCtrlShiftMeta},
		{Key: term.MouseRight, Mod: term.ModCtrlShiftMeta},
		{Key: term.MouseRelease, Mod: term.ModCtrlShiftMeta},
		{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftMeta},
		{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyBackspace, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyTab, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyEnter, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeyEsc, Mod: term.ModCtrlShiftMeta},
		{Key: term.KeySpace, Mod: term.ModCtrlShiftMeta},

		{Key: term.KeyF1, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF2, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF3, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF4, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF5, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF6, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF7, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF8, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF9, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF10, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF11, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyF12, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyInsert, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyDelete, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyHome, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyEnd, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyPgup, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyPgdn, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyArrowUp, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyArrowDown, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyArrowRight, Mod: term.ModCtrlAltMeta},
		{Key: term.MouseLeft, Mod: term.ModCtrlAltMeta},
		{Key: term.MouseMiddle, Mod: term.ModCtrlAltMeta},
		{Key: term.MouseRight, Mod: term.ModCtrlAltMeta},
		{Key: term.MouseRelease, Mod: term.ModCtrlAltMeta},
		{Key: term.MouseWheelUp, Mod: term.ModCtrlAltMeta},
		{Key: term.MouseWheelDown, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyBackspace, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyTab, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyEnter, Mod: term.ModCtrlAltMeta},
		{Key: term.KeyEsc, Mod: term.ModCtrlAltMeta},
		{Key: term.KeySpace, Mod: term.ModCtrlAltMeta},

		{Key: term.KeyF1, Mod: term.ModShiftMeta},
		{Key: term.KeyF2, Mod: term.ModShiftMeta},
		{Key: term.KeyF3, Mod: term.ModShiftMeta},
		{Key: term.KeyF4, Mod: term.ModShiftMeta},
		{Key: term.KeyF5, Mod: term.ModShiftMeta},
		{Key: term.KeyF6, Mod: term.ModShiftMeta},
		{Key: term.KeyF7, Mod: term.ModShiftMeta},
		{Key: term.KeyF8, Mod: term.ModShiftMeta},
		{Key: term.KeyF9, Mod: term.ModShiftMeta},
		{Key: term.KeyF10, Mod: term.ModShiftMeta},
		{Key: term.KeyF11, Mod: term.ModShiftMeta},
		{Key: term.KeyF12, Mod: term.ModShiftMeta},
		{Key: term.KeyInsert, Mod: term.ModShiftMeta},
		{Key: term.KeyDelete, Mod: term.ModShiftMeta},
		{Key: term.KeyHome, Mod: term.ModShiftMeta},
		{Key: term.KeyEnd, Mod: term.ModShiftMeta},
		{Key: term.KeyPgup, Mod: term.ModShiftMeta},
		{Key: term.KeyPgdn, Mod: term.ModShiftMeta},
		{Key: term.KeyArrowUp, Mod: term.ModShiftMeta},
		{Key: term.KeyArrowDown, Mod: term.ModShiftMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModShiftMeta},
		{Key: term.KeyArrowRight, Mod: term.ModShiftMeta},
		{Key: term.MouseLeft, Mod: term.ModShiftMeta},
		{Key: term.MouseMiddle, Mod: term.ModShiftMeta},
		{Key: term.MouseRight, Mod: term.ModShiftMeta},
		{Key: term.MouseRelease, Mod: term.ModShiftMeta},
		{Key: term.MouseWheelUp, Mod: term.ModShiftMeta},
		{Key: term.MouseWheelDown, Mod: term.ModShiftMeta},
		{Key: term.KeyBackspace, Mod: term.ModShiftMeta},
		{Key: term.KeyTab, Mod: term.ModShiftMeta},
		{Key: term.KeyEnter, Mod: term.ModShiftMeta},
		{Key: term.KeyEsc, Mod: term.ModShiftMeta},
		{Key: term.KeySpace, Mod: term.ModShiftMeta},

		{Key: term.KeyF1, Mod: term.ModAltMeta},
		{Key: term.KeyF2, Mod: term.ModAltMeta},
		{Key: term.KeyF3, Mod: term.ModAltMeta},
		{Key: term.KeyF4, Mod: term.ModAltMeta},
		{Key: term.KeyF5, Mod: term.ModAltMeta},
		{Key: term.KeyF6, Mod: term.ModAltMeta},
		{Key: term.KeyF7, Mod: term.ModAltMeta},
		{Key: term.KeyF8, Mod: term.ModAltMeta},
		{Key: term.KeyF9, Mod: term.ModAltMeta},
		{Key: term.KeyF10, Mod: term.ModAltMeta},
		{Key: term.KeyF11, Mod: term.ModAltMeta},
		{Key: term.KeyF12, Mod: term.ModAltMeta},
		{Key: term.KeyInsert, Mod: term.ModAltMeta},
		{Key: term.KeyDelete, Mod: term.ModAltMeta},
		{Key: term.KeyHome, Mod: term.ModAltMeta},
		{Key: term.KeyEnd, Mod: term.ModAltMeta},
		{Key: term.KeyPgup, Mod: term.ModAltMeta},
		{Key: term.KeyPgdn, Mod: term.ModAltMeta},
		{Key: term.KeyArrowUp, Mod: term.ModAltMeta},
		{Key: term.KeyArrowDown, Mod: term.ModAltMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModAltMeta},
		{Key: term.KeyArrowRight, Mod: term.ModAltMeta},
		{Key: term.MouseLeft, Mod: term.ModAltMeta},
		{Key: term.MouseMiddle, Mod: term.ModAltMeta},
		{Key: term.MouseRight, Mod: term.ModAltMeta},
		{Key: term.MouseRelease, Mod: term.ModAltMeta},
		{Key: term.MouseWheelUp, Mod: term.ModAltMeta},
		{Key: term.MouseWheelDown, Mod: term.ModAltMeta},
		{Key: term.KeyBackspace, Mod: term.ModAltMeta},
		{Key: term.KeyTab, Mod: term.ModAltMeta},
		{Key: term.KeyEnter, Mod: term.ModAltMeta},
		{Key: term.KeyEsc, Mod: term.ModAltMeta},
		{Key: term.KeySpace, Mod: term.ModAltMeta},

		{Key: term.KeyF1, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF2, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF3, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF4, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF5, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF6, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF7, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF8, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF9, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF10, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF11, Mod: term.ModAltShiftMeta},
		{Key: term.KeyF12, Mod: term.ModAltShiftMeta},
		{Key: term.KeyInsert, Mod: term.ModAltShiftMeta},
		{Key: term.KeyDelete, Mod: term.ModAltShiftMeta},
		{Key: term.KeyHome, Mod: term.ModAltShiftMeta},
		{Key: term.KeyEnd, Mod: term.ModAltShiftMeta},
		{Key: term.KeyPgup, Mod: term.ModAltShiftMeta},
		{Key: term.KeyPgdn, Mod: term.ModAltShiftMeta},
		{Key: term.KeyArrowUp, Mod: term.ModAltShiftMeta},
		{Key: term.KeyArrowDown, Mod: term.ModAltShiftMeta},
		{Key: term.KeyArrowLeft, Mod: term.ModAltShiftMeta},
		{Key: term.KeyArrowRight, Mod: term.ModAltShiftMeta},
		{Key: term.MouseLeft, Mod: term.ModAltShiftMeta},
		{Key: term.MouseMiddle, Mod: term.ModAltShiftMeta},
		{Key: term.MouseRight, Mod: term.ModAltShiftMeta},
		{Key: term.MouseRelease, Mod: term.ModAltShiftMeta},
		{Key: term.MouseWheelUp, Mod: term.ModAltShiftMeta},
		{Key: term.MouseWheelDown, Mod: term.ModAltShiftMeta},
		{Key: term.KeyBackspace, Mod: term.ModAltShiftMeta},
		{Key: term.KeyTab, Mod: term.ModAltShiftMeta},
		{Key: term.KeyEnter, Mod: term.ModAltShiftMeta},
		{Key: term.KeyEsc, Mod: term.ModAltShiftMeta},
		{Key: term.KeySpace, Mod: term.ModAltShiftMeta},

		{Key: term.KeyF1, Mod: term.ModAltShift},
		{Key: term.KeyF2, Mod: term.ModAltShift},
		{Key: term.KeyF3, Mod: term.ModAltShift},
		{Key: term.KeyF4, Mod: term.ModAltShift},
		{Key: term.KeyF5, Mod: term.ModAltShift},
		{Key: term.KeyF6, Mod: term.ModAltShift},
		{Key: term.KeyF7, Mod: term.ModAltShift},
		{Key: term.KeyF8, Mod: term.ModAltShift},
		{Key: term.KeyF9, Mod: term.ModAltShift},
		{Key: term.KeyF10, Mod: term.ModAltShift},
		{Key: term.KeyF11, Mod: term.ModAltShift},
		{Key: term.KeyF12, Mod: term.ModAltShift},
		{Key: term.KeyInsert, Mod: term.ModAltShift},
		{Key: term.KeyDelete, Mod: term.ModAltShift},
		{Key: term.KeyHome, Mod: term.ModAltShift},
		{Key: term.KeyEnd, Mod: term.ModAltShift},
		{Key: term.KeyPgup, Mod: term.ModAltShift},
		{Key: term.KeyPgdn, Mod: term.ModAltShift},
		{Key: term.KeyArrowUp, Mod: term.ModAltShift},
		{Key: term.KeyArrowDown, Mod: term.ModAltShift},
		{Key: term.KeyArrowLeft, Mod: term.ModAltShift},
		{Key: term.KeyArrowRight, Mod: term.ModAltShift},
		{Key: term.MouseLeft, Mod: term.ModAltShift},
		{Key: term.MouseMiddle, Mod: term.ModAltShift},
		{Key: term.MouseRight, Mod: term.ModAltShift},
		{Key: term.MouseRelease, Mod: term.ModAltShift},
		{Key: term.MouseWheelUp, Mod: term.ModAltShift},
		{Key: term.MouseWheelDown, Mod: term.ModAltShift},
		{Key: term.KeyBackspace, Mod: term.ModAltShift},
		{Key: term.KeyTab, Mod: term.ModAltShift},
		{Key: term.KeyEnter, Mod: term.ModAltShift},
		{Key: term.KeyEsc, Mod: term.ModAltShift},
		{Key: term.KeySpace, Mod: term.ModAltShift},
		{Mod: term.ModAlt},
		{Mod: term.ModShift},
		{Mod: term.ModMeta},
		{Mod: term.ModCtrl},
		{Mod: term.ModCtrlShift},
		{Mod: term.ModCtrlAlt},
		{Mod: term.ModCtrlMeta},
		{Mod: term.ModCtrlShiftAlt},
		{Mod: term.ModCtrlShiftMeta},
		{Mod: term.ModCtrlAltMeta},
		{Mod: term.ModShiftMeta},
		{Mod: term.ModAltMeta},
		{Mod: term.ModAltShiftMeta},
		{Mod: term.ModAltShift},
	}

	for i, comb := range suite {
		t.Run(fmt.Sprintf("String, ParseKey twice: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

		t.Run(fmt.Sprintf("ShortString, ParseKey twice: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombShortString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombShortString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

		t.Run(fmt.Sprintf("ShortString, ParseKey, String, ParseKey: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombShortString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

		t.Run(fmt.Sprintf("String, ParseKey, ShortString, ParseKey: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombShortString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})
	}

	t.Run("combined sequence of keys", func(t *testing.T) {
		var builder strings.Builder
		for _, comb := range suite {
			builder.WriteString(KeyCombString(comb))
		}
		keys, err := ParseKeys(builder.String())
		require.NoError(t, err)
		assert.ElementsMatch(t, suite, keys)
	})
}

// test chars separately as equivalence must be tested via KeyComb.String()
// because Mod shift with unshifted characters is not emitted by tui or gui
// and so it's "invalid", but we should still test that it converts to the right
// KeyComb.
func TestParseCharKey(t *testing.T) {
	modifiers := []term.Modifier{0, term.ModAlt, term.ModShift, term.ModMeta,
		term.ModCtrl, term.ModCtrlShift, term.ModCtrlAlt, term.ModCtrlMeta,
		term.ModCtrlShiftAlt, term.ModCtrlShiftMeta,
		/* ModCtrlAltMeta skip since it cannot be fully upgraded/downgraded with shift */
		term.ModShiftMeta, term.ModAltMeta, term.ModAltShiftMeta, term.ModAltShift}
	charKeys := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQR" +
		"STUVWXYZ1234567890-=`[];',./~!@#$%^&*()_+{}\":?<>\\|"

	var suite []term.KeyComb
	for _, ch := range []rune(charKeys) {
		for _, mod := range modifiers {
			suite = append(suite, term.KeyComb{Ch: ch, Mod: mod})
		}
	}
	for i, comb := range suite {
		t.Run(fmt.Sprintf("String, ParseKey twice: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombString(comb))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c))

			c, err = ParseKey(KeyCombString(c))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)
		})

		t.Run(fmt.Sprintf("ShortString, ParseKey twice: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombShortString(comb))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)

			c, err = ParseKey(KeyCombShortString(c))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)
		})

		t.Run(fmt.Sprintf("ShortString, ParseKey, String, ParseKey: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombShortString(comb))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)

			c, err = ParseKey(KeyCombString(c))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)
		})

		t.Run(fmt.Sprintf("String, ParseKey, ShortString, ParseKey: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombString(comb))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)

			c, err = ParseKey(KeyCombShortString(c))
			require.NoError(t, err)
			assert.Equal(t, KeyCombString(comb), KeyCombString(c), comb)
		})

	}

	t.Run("combined sequence of characters", func(t *testing.T) {
		var builder strings.Builder
		for _, comb := range suite {
			builder.WriteString(KeyCombString(comb))
		}
		seq := builder.String()
		keys, err := ParseKeys(seq)
		require.NoError(t, err)

		builder.Reset()
		for _, comb := range keys {
			builder.WriteString(KeyCombString(comb))
		}
		assert.Equal(t, seq, builder.String())
	})

	t.Run("ParseKeyswith with mixed sequences", func(t *testing.T) {
		suite := []struct {
			in          string
			expectedOut []term.KeyComb
			expectedErr bool
		}{
			{
				in: "<esc>:edit<space>hello<enter>",
				expectedOut: []term.KeyComb{
					{Key: term.KeyEsc},
					{Ch: ':'},
					{Ch: 'e'},
					{Ch: 'd'},
					{Ch: 'i'},
					{Ch: 't'},
					{Key: term.KeySpace},
					{Ch: 'h'},
					{Ch: 'e'},
					{Ch: 'l'},
					{Ch: 'l'},
					{Ch: 'o'},
					{Key: term.KeyEnter},
				},
			},
			{
				in:          "<esc<:edit<space>",
				expectedErr: true,
			},
		}

		for _, test := range suite {
			actualOut, actualErr := ParseKeys(test.in)
			if test.expectedErr {
				require.Error(t, actualErr)
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, test.expectedOut, actualOut)
			}
		}
	})
}

func TestParseNonKeyChar(t *testing.T) {
	charKeys := "œ∑´®†¥¨ˆøπ“‘æ…¬˚∆˙©ƒ∂ßå≈ç√∫˜µ≤≥÷¡™£¢∞§¶•ªº–≠"

	var suite []term.KeyComb
	for _, ch := range []rune(charKeys) {
		suite = append(suite, term.KeyComb{Ch: ch})
	}
	for i, comb := range suite {
		t.Run(fmt.Sprintf("String, ParseKey twice: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c)

			c, err = ParseKey(KeyCombString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

		t.Run(fmt.Sprintf("ShortString, ParseKey twice: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombShortString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombShortString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

		t.Run(fmt.Sprintf("ShortString, ParseKey, String, ParseKey: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombShortString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

		t.Run(fmt.Sprintf("String, ParseKey, ShortString, ParseKey: %d: %#v", i, comb), func(t *testing.T) {
			c, err := ParseKey(KeyCombString(comb))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)

			c, err = ParseKey(KeyCombShortString(c))
			require.NoError(t, err)
			assert.Equal(t, comb, c, comb)
		})

	}

	t.Run("combined sequence of characters", func(t *testing.T) {
		var builder strings.Builder
		for _, comb := range suite {
			builder.WriteString(KeyCombString(comb))
		}
		keys, err := ParseKeys(builder.String())
		require.NoError(t, err)
		assert.ElementsMatch(t, suite, keys)
	})
}
