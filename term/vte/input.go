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

package vte

import (
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/term"
)

func getModifierStr(ev term.Event) string {
	switch ev.Mod {
	case term.ModAlt:
		return ";3"
	default:
		return ""
	}
}

// return an escape sequence for what requires state terminal, otherwise
// the raw bytes coming from the event are already correct
func mapKeyToEscapeSequence(comp *Component, ev term.Event) ([]byte, bool) {
	switch ev.Mod {
	case 0:
		switch ev.Key {
		case term.KeyArrowUp:
			if comp.IsApplicationCursorKeysMode() {
				return []byte{0x1b, 'O', 'A'}, true
			}
			return []byte(fmt.Sprintf("\x1b[%sA", getModifierStr(ev))), true
		case term.KeyArrowDown:
			if comp.IsApplicationCursorKeysMode() {
				return []byte{0x1b, 'O', 'B'}, true
			}
			return []byte(fmt.Sprintf("\x1b[%sB", getModifierStr(ev))), true
		case term.KeyArrowRight:
			if comp.IsApplicationCursorKeysMode() {
				return []byte{0x1b, 'O', 'C'}, true
			}
			return []byte(fmt.Sprintf("\x1b[%sC", getModifierStr(ev))), true
		case term.KeyArrowLeft:
			if comp.IsApplicationCursorKeysMode() {
				return []byte{0x1b, 'O', 'D'}, true
			}
			return []byte(fmt.Sprintf("\x1b[%sD", getModifierStr(ev))), true
		case term.KeyEnter:
			if ev.Mod == 0 {
				if comp.IsNewLineMode() {
					return []byte{0x0d, 0x0a}, true
				}
				return []byte{0x0d}, true
			}
			return nil, false
		case term.KeyHome:
			if comp.IsApplicationCursorKeysMode() {
				return []byte(fmt.Sprintf("\x1b[1%s~", getModifierStr(ev))), true
			}
			return []byte("\x1b[H"), true
		default:
			return nil, false
		}
	default:
		return nil, false
	}
}
