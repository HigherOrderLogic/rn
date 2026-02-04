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
	"errors"
	"fmt"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/term"
)

// ParseKeys parses the given sequence of key combinations.
func ParseKeys(sequence string) (ret []term.KeyComb, err error) {
	runes := []rune(sequence)
	for len(runes) != 0 {
		r := runes[0]
		switch r {
		// escape character
		case '\\':
			if len(runes) == 1 {
				err = errors.New("unterminated escape sequence: " +
					"Either '\\', '>' or '<' must follow a start of escape sequence '\\'")
				return
			}
			switch runes[1] {
			case '>', '<', '\\':
				ret = append(ret, term.KeyComb{Ch: runes[1]})
				runes = runes[2:]
			default:
				err = fmt.Errorf("invalid escape sequence: " +
					"Either '\\', '>' or '<' must follow a start of escape sequence '\\'")
				return
			}
		// invalid, space should be represented as <space>
		case ' ':
			err = errors.New("invalid escape sequence: " +
				"Space should be represented with '<space>' syntax")
			return

		case '>':
			err = errors.New("invalid escape sequence: unescaped, starting '>' character")
			return
		// start of key
		case '<':
			idxGt := strings.IndexRune(string(runes), '>')
			if idxGt < 0 {
				err = errors.New("unterminated key: '<' found but no matching '>' found")
				return
			}
			var key term.KeyComb
			key, err = ParseKey(string(runes[0 : idxGt+1]))
			if err != nil {
				return
			}
			ret = append(ret, key)
			runes = runes[idxGt+1:]
		default:
			var key term.KeyComb
			key, err = ParseKey(string(runes[0]))
			if err != nil {
				return
			}
			ret = append(ret, key)
			runes = runes[1:]
		}
	}
	return
}

// ParseKey parses str into a term.KeyComb or returns
// error if it fails to parse it.
func ParseKey(str string) (term.KeyComb, error) {
	str = strings.TrimSpace(str)
	switch len([]rune(str)) {
	case 0:
		return term.KeyComb{}, errors.New("invalid empty input")
	case 1:
		ch := []rune(str)[0]
		switch ch {
		case '>', '<', '\\':
			return term.KeyComb{}, errors.New("characters '>', '<' and '\\' must be escaped with '\\'")
		default:
			// characters that would return len(str) > 1 are processed in prev statement
			return term.KeyComb{Ch: ch}, nil
		}
	default:
		str = strings.ToLower(str)
		switch str {
		case "\\>":
			return term.KeyComb{Ch: '>'}, nil
		case "\\<":
			return term.KeyComb{Ch: '<'}, nil
		case "\\\\":
			return term.KeyComb{Ch: '\\'}, nil
		case "<f1>":
			return term.KeyComb{Key: term.KeyF1}, nil
		case "<f2>":
			return term.KeyComb{Key: term.KeyF2}, nil
		case "<f3>":
			return term.KeyComb{Key: term.KeyF3}, nil
		case "<f4>":
			return term.KeyComb{Key: term.KeyF4}, nil
		case "<f5>":
			return term.KeyComb{Key: term.KeyF5}, nil
		case "<f6>":
			return term.KeyComb{Key: term.KeyF6}, nil
		case "<f7>":
			return term.KeyComb{Key: term.KeyF7}, nil
		case "<f8>":
			return term.KeyComb{Key: term.KeyF8}, nil
		case "<f9>":
			return term.KeyComb{Key: term.KeyF9}, nil
		case "<f10>":
			return term.KeyComb{Key: term.KeyF10}, nil
		case "<f11>":
			return term.KeyComb{Key: term.KeyF11}, nil
		case "<f12>":
			return term.KeyComb{Key: term.KeyF12}, nil
		case "<insert>":
			return term.KeyComb{Key: term.KeyInsert}, nil
		case "<delete>":
			return term.KeyComb{Key: term.KeyDelete}, nil
		case "<home>":
			return term.KeyComb{Key: term.KeyHome}, nil
		case "<end>":
			return term.KeyComb{Key: term.KeyEnd}, nil
		case "<pgup>":
			return term.KeyComb{Key: term.KeyPgup}, nil
		case "<pgdn>":
			return term.KeyComb{Key: term.KeyPgdn}, nil
		case "<up>":
			return term.KeyComb{Key: term.KeyArrowUp}, nil
		case "<down>":
			return term.KeyComb{Key: term.KeyArrowDown}, nil
		case "<left>":
			return term.KeyComb{Key: term.KeyArrowLeft}, nil
		case "<right>":
			return term.KeyComb{Key: term.KeyArrowRight}, nil
		case "<mouse-left>":
			return term.KeyComb{Key: term.MouseLeft}, nil
		case "<mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle}, nil
		case "<mouse-right>":
			return term.KeyComb{Key: term.MouseRight}, nil
		case "<mouse-release>":
			return term.KeyComb{Key: term.MouseRelease}, nil
		case "<mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp}, nil
		case "<mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown}, nil
		case "<tab>":
			return term.KeyComb{Key: term.KeyTab}, nil
		case "<enter>":
			return term.KeyComb{Key: term.KeyEnter}, nil
		case "<esc>":
			return term.KeyComb{Key: term.KeyEsc}, nil
		case "<space>":
			return term.KeyComb{Key: term.KeySpace}, nil
		case "<backspace>":
			return term.KeyComb{Key: term.KeyBackspace}, nil

		// meta
		case "<m-f1>", "<meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModMeta}, nil
		case "<m-f2>", "<meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModMeta}, nil
		case "<m-f3>", "<meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModMeta}, nil
		case "<m-f4>", "<meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModMeta}, nil
		case "<m-f5>", "<meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModMeta}, nil
		case "<m-f6>", "<meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModMeta}, nil
		case "<m-f7>", "<meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModMeta}, nil
		case "<m-f8>", "<meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModMeta}, nil
		case "<m-f9>", "<meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModMeta}, nil
		case "<m-f10>", "<meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModMeta}, nil
		case "<m-f11>", "<meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModMeta}, nil
		case "<m-f12>", "<meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModMeta}, nil
		case "<m-insert>", "<meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModMeta}, nil
		case "<m-delete>", "<meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModMeta}, nil
		case "<m-home>", "<meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModMeta}, nil
		case "<m-end>", "<meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModMeta}, nil
		case "<m-pgup>", "<meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModMeta}, nil
		case "<m-pgdn>", "<meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModMeta}, nil
		case "<m-up>", "<meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModMeta}, nil
		case "<m-down>", "<meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModMeta}, nil
		case "<m-left>", "<meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModMeta}, nil
		case "<m-right>", "<meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModMeta}, nil
		case "<m-mouse-left>", "<meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModMeta}, nil
		case "<m-mouse-middle>", "<meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModMeta}, nil
		case "<m-mouse-right>", "<meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModMeta}, nil
		case "<m-mouse-release>", "<meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModMeta}, nil
		case "<m-mouse-wheel-up>", "<meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModMeta}, nil
		case "<m-mouse-wheel-down>", "<meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModMeta}, nil
		case "<m-enter>", "<meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModMeta}, nil
		case "<m-space>", "<meta-space>":
			return term.KeyComb{Mod: term.ModMeta, Key: term.KeySpace}, nil
		case "<m-backspace>", "<meta-backspace>":
			return term.KeyComb{Mod: term.ModMeta, Key: term.KeyBackspace}, nil
		case "<m-esc>", "<meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModMeta}, nil
		case "<m-1>", "<meta-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModMeta}, nil
		case "<m-2>", "<meta-2>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '2'}, nil
		case "<m-3>", "<meta-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModMeta}, nil
		case "<m-4>", "<meta-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModMeta}, nil
		case "<m-5>", "<meta-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModMeta}, nil
		case "<m-6>", "<meta-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModMeta}, nil
		case "<m-7>", "<meta-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModMeta}, nil
		case "<m-8>", "<meta-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModMeta}, nil
		case "<m-9>", "<meta-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModMeta}, nil
		case "<m-0>", "<meta-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModMeta}, nil
		case "<m-`>", "<meta-`>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '`'}, nil
		case "<m-a>", "<meta-a>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'a'}, nil
		case "<m-b>", "<meta-b>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'b'}, nil
		case "<m-c>", "<meta-c>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'c'}, nil
		case "<m-d>", "<meta-d>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'd'}, nil
		case "<m-e>", "<meta-e>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'e'}, nil
		case "<m-f>", "<meta-f>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'f'}, nil
		case "<m-g>", "<meta-g>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'g'}, nil
		case "<m-h>", "<meta-h>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'h'}, nil
		case "<m-tab>", "<meta-tab>":
			return term.KeyComb{Mod: term.ModMeta, Key: term.KeyTab}, nil
		case "<m-i>", "<meta-i>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'i'}, nil
		case "<m-j>", "<meta-j>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'j'}, nil
		case "<m-k>", "<meta-k>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'k'}, nil
		case "<m-l>", "<meta-l>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'l'}, nil
		case "<m-m>", "<meta-m>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'm'}, nil
		case "<m-n>", "<meta-n>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'n'}, nil
		case "<m-o>", "<meta-o>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'o'}, nil
		case "<m-p>", "<meta-p>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'p'}, nil
		case "<m-q>", "<meta-q>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'q'}, nil
		case "<m-r>", "<meta-r>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'r'}, nil
		case "<m-s>", "<meta-s>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 's'}, nil
		case "<m-t>", "<meta-t>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 't'}, nil
		case "<m-u>", "<meta-u>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'u'}, nil
		case "<m-v>", "<meta-v>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'v'}, nil
		case "<m-w>", "<meta-w>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'w'}, nil
		case "<m-x>", "<meta-x>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'x'}, nil
		case "<m-y>", "<meta-y>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'y'}, nil
		case "<m-z>", "<meta-z>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'z'}, nil
		case "<m-[>", "<meta-[>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '['}, nil
		case "<m-\\\\>", "<meta-\\\\>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '\\'}, nil
		case "<m-]>", "<meta-]>":
			return term.KeyComb{Mod: term.ModMeta, Ch: ']'}, nil
		case "<m-/>", "<meta-/>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '/'}, nil
		case "<m-_>", "<meta-_>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '_'}, nil
		case "<m-.>", "<meta-.>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '.'}, nil
		case "<m-,>", "<meta-,>":
			return term.KeyComb{Mod: term.ModMeta, Ch: ','}, nil
		case "<m-;>", "<meta-;>":
			return term.KeyComb{Mod: term.ModMeta, Ch: ';'}, nil
		case "<m-'>", "<meta-'>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '\''}, nil
		case "<m-=>", "<meta-=>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '='}, nil
		case "<m-->", "<meta-->":
			return term.KeyComb{Mod: term.ModMeta, Ch: '-'}, nil

		// alt
		case "<a-f1>", "<alt-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModAlt}, nil
		case "<a-f2>", "<alt-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModAlt}, nil
		case "<a-f3>", "<alt-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModAlt}, nil
		case "<a-f4>", "<alt-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModAlt}, nil
		case "<a-f5>", "<alt-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModAlt}, nil
		case "<a-f6>", "<alt-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModAlt}, nil
		case "<a-f7>", "<alt-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModAlt}, nil
		case "<a-f8>", "<alt-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModAlt}, nil
		case "<a-f9>", "<alt-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModAlt}, nil
		case "<a-f10>", "<alt-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModAlt}, nil
		case "<a-f11>", "<alt-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModAlt}, nil
		case "<a-f12>", "<alt-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModAlt}, nil
		case "<a-insert>", "<alt-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModAlt}, nil
		case "<a-delete>", "<alt-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModAlt}, nil
		case "<a-home>", "<alt-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModAlt}, nil
		case "<a-end>", "<alt-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModAlt}, nil
		case "<a-pgup>", "<alt-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModAlt}, nil
		case "<a-pgdn>", "<alt-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAlt}, nil
		case "<a-up>", "<alt-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAlt}, nil
		case "<a-down>", "<alt-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAlt}, nil
		case "<a-left>", "<alt-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAlt}, nil
		case "<a-right>", "<alt-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAlt}, nil
		case "<a-mouse-left>", "<alt-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModAlt}, nil
		case "<a-mouse-middle>", "<alt-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAlt}, nil
		case "<a-mouse-right>", "<alt-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModAlt}, nil
		case "<a-mouse-release>", "<alt-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModAlt}, nil
		case "<a-mouse-wheel-up>", "<alt-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAlt}, nil
		case "<a-mouse-wheel-down>", "<alt-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAlt}, nil
		case "<a-enter>", "<alt-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModAlt}, nil
		case "<a-space>", "<alt-space>":
			return term.KeyComb{Mod: term.ModAlt, Key: term.KeySpace}, nil
		case "<a-backspace>", "<alt-backspace>":
			return term.KeyComb{Mod: term.ModAlt, Key: term.KeyBackspace}, nil
		case "<a-esc>", "<alt-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModAlt}, nil
		case "<a-1>", "<alt-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModAlt}, nil
		case "<a-2>", "<alt-2>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '2'}, nil
		case "<a-3>", "<alt-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModAlt}, nil
		case "<a-4>", "<alt-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModAlt}, nil
		case "<a-5>", "<alt-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModAlt}, nil
		case "<a-6>", "<alt-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModAlt}, nil
		case "<a-7>", "<alt-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModAlt}, nil
		case "<a-8>", "<alt-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModAlt}, nil
		case "<a-9>", "<alt-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModAlt}, nil
		case "<a-0>", "<alt-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModAlt}, nil
		case "<a-`>", "<alt-`>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '`'}, nil
		case "<a-a>", "<alt-a>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'a'}, nil
		case "<a-b>", "<alt-b>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'b'}, nil
		case "<a-c>", "<alt-c>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'c'}, nil
		case "<a-d>", "<alt-d>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'd'}, nil
		case "<a-e>", "<alt-e>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'e'}, nil
		case "<a-f>", "<alt-f>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'f'}, nil
		case "<a-g>", "<alt-g>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'g'}, nil
		case "<a-h>", "<alt-h>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'h'}, nil
		case "<a-tab>", "<alt-tab>":
			return term.KeyComb{Mod: term.ModAlt, Key: term.KeyTab}, nil
		case "<a-i>", "<alt-i>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'i'}, nil
		case "<a-j>", "<alt-j>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'j'}, nil
		case "<a-k>", "<alt-k>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'k'}, nil
		case "<a-l>", "<alt-l>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'l'}, nil
		case "<a-m>", "<alt-m>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'm'}, nil
		case "<a-n>", "<alt-n>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'n'}, nil
		case "<a-o>", "<alt-o>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'o'}, nil
		case "<a-p>", "<alt-p>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'p'}, nil
		case "<a-q>", "<alt-q>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'q'}, nil
		case "<a-r>", "<alt-r>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'r'}, nil
		case "<a-s>", "<alt-s>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 's'}, nil
		case "<a-t>", "<alt-t>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 't'}, nil
		case "<a-u>", "<alt-u>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'u'}, nil
		case "<a-v>", "<alt-v>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'v'}, nil
		case "<a-w>", "<alt-w>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'w'}, nil
		case "<a-x>", "<alt-x>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'x'}, nil
		case "<a-y>", "<alt-y>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'y'}, nil
		case "<a-z>", "<alt-z>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'z'}, nil
		case "<a-[>", "<alt-[>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '['}, nil
		case "<a-\\\\>", "<alt-\\\\>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '\\'}, nil
		case "<a-]>", "<alt-]>":
			return term.KeyComb{Mod: term.ModAlt, Ch: ']'}, nil
		case "<a-/>", "<alt-/>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '/'}, nil
		case "<a-_>", "<alt-_>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '_'}, nil
		case "<a-.>", "<alt-.>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '.'}, nil
		case "<a-,>", "<alt-,>":
			return term.KeyComb{Mod: term.ModAlt, Ch: ','}, nil
		case "<a-;>", "<alt-;>":
			return term.KeyComb{Mod: term.ModAlt, Ch: ';'}, nil
		case "<a-'>", "<alt-'>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '\''}, nil
		case "<a-=>", "<alt-=>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '='}, nil
		case "<a-->", "<alt-->":
			return term.KeyComb{Mod: term.ModAlt, Ch: '-'}, nil

		// shift
		case "<s-f1>", "<shift-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModShift}, nil
		case "<s-f2>", "<shift-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModShift}, nil
		case "<s-f3>", "<shift-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModShift}, nil
		case "<s-f4>", "<shift-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModShift}, nil
		case "<s-f5>", "<shift-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModShift}, nil
		case "<s-f6>", "<shift-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModShift}, nil
		case "<s-f7>", "<shift-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModShift}, nil
		case "<s-f8>", "<shift-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModShift}, nil
		case "<s-f9>", "<shift-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModShift}, nil
		case "<s-f10>", "<shift-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModShift}, nil
		case "<s-f11>", "<shift-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModShift}, nil
		case "<s-f12>", "<shift-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModShift}, nil
		case "<s-insert>", "<shift-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModShift}, nil
		case "<s-delete>", "<shift-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModShift}, nil
		case "<s-home>", "<shift-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModShift}, nil
		case "<s-end>", "<shift-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModShift}, nil
		case "<s-pgup>", "<shift-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModShift}, nil
		case "<s-pgdn>", "<shift-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModShift}, nil
		case "<s-up>", "<shift-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModShift}, nil
		case "<s-down>", "<shift-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModShift}, nil
		case "<s-left>", "<shift-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModShift}, nil
		case "<s-right>", "<shift-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModShift}, nil
		case "<s-mouse-left>", "<shift-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModShift}, nil
		case "<s-mouse-middle>", "<shift-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModShift}, nil
		case "<s-mouse-right>", "<shift-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModShift}, nil
		case "<s-mouse-release>", "<shift-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModShift}, nil
		case "<s-mouse-wheel-up>", "<shift-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModShift}, nil
		case "<s-mouse-wheel-down>", "<shift-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModShift}, nil
		case "<s-enter>", "<shift-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModShift}, nil
		case "<s-space>", "<shift-space>":
			return term.KeyComb{Mod: term.ModShift, Key: term.KeySpace}, nil
		case "<s-backspace>", "<shift-backspace>":
			return term.KeyComb{Mod: term.ModShift, Key: term.KeyBackspace}, nil
		case "<s-esc>", "<shift-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModShift}, nil
		case "<s-1>", "<shift-1>":
			return term.KeyComb{Ch: '!'}, nil
		case "<s-2>", "<shift-2>":
			return term.KeyComb{Ch: '@'}, nil
		case "<s-3>", "<shift-3>":
			return term.KeyComb{Ch: '#'}, nil
		case "<s-4>", "<shift-4>":
			return term.KeyComb{Ch: '$'}, nil
		case "<s-5>", "<shift-5>":
			return term.KeyComb{Ch: '%'}, nil
		case "<s-6>", "<shift-6>":
			return term.KeyComb{Ch: '^'}, nil
		case "<s-7>", "<shift-7>":
			return term.KeyComb{Ch: '&'}, nil
		case "<s-8>", "<shift-8>":
			return term.KeyComb{Ch: '*'}, nil
		case "<s-9>", "<shift-9>":
			return term.KeyComb{Ch: '('}, nil
		case "<s-0>", "<shift-0>":
			return term.KeyComb{Ch: ')'}, nil
		case "<s-`>", "<shift-`>":
			return term.KeyComb{Ch: '~'}, nil
		case "<s-a>", "<shift-a>":
			return term.KeyComb{Ch: 'A'}, nil
		case "<s-b>", "<shift-b>":
			return term.KeyComb{Ch: 'B'}, nil
		case "<s-c>", "<shift-c>":
			return term.KeyComb{Ch: 'C'}, nil
		case "<s-d>", "<shift-d>":
			return term.KeyComb{Ch: 'D'}, nil
		case "<s-e>", "<shift-e>":
			return term.KeyComb{Ch: 'E'}, nil
		case "<s-f>", "<shift-f>":
			return term.KeyComb{Ch: 'F'}, nil
		case "<s-g>", "<shift-g>":
			return term.KeyComb{Ch: 'G'}, nil
		case "<s-h>", "<shift-h>":
			return term.KeyComb{Ch: 'H'}, nil
		case "<s-tab>", "<shift-tab>":
			return term.KeyComb{Mod: term.ModShift, Key: term.KeyTab}, nil
		case "<s-i>", "<shift-i>":
			return term.KeyComb{Ch: 'I'}, nil
		case "<s-j>", "<shift-j>":
			return term.KeyComb{Ch: 'J'}, nil
		case "<s-k>", "<shift-k>":
			return term.KeyComb{Ch: 'K'}, nil
		case "<s-l>", "<shift-l>":
			return term.KeyComb{Ch: 'L'}, nil
		case "<s-m>", "<shift-m>":
			return term.KeyComb{Ch: 'M'}, nil
		case "<s-n>", "<shift-n>":
			return term.KeyComb{Ch: 'N'}, nil
		case "<s-o>", "<shift-o>":
			return term.KeyComb{Ch: 'O'}, nil
		case "<s-p>", "<shift-p>":
			return term.KeyComb{Ch: 'P'}, nil
		case "<s-q>", "<shift-q>":
			return term.KeyComb{Ch: 'Q'}, nil
		case "<s-r>", "<shift-r>":
			return term.KeyComb{Ch: 'R'}, nil
		case "<s-s>", "<shift-s>":
			return term.KeyComb{Ch: 'S'}, nil
		case "<s-t>", "<shift-t>":
			return term.KeyComb{Ch: 'T'}, nil
		case "<s-u>", "<shift-u>":
			return term.KeyComb{Ch: 'U'}, nil
		case "<s-v>", "<shift-v>":
			return term.KeyComb{Ch: 'V'}, nil
		case "<s-w>", "<shift-w>":
			return term.KeyComb{Ch: 'W'}, nil
		case "<s-x>", "<shift-x>":
			return term.KeyComb{Ch: 'X'}, nil
		case "<s-y>", "<shift-y>":
			return term.KeyComb{Ch: 'Y'}, nil
		case "<s-z>", "<shift-z>":
			return term.KeyComb{Ch: 'Z'}, nil
		case "<s-[>", "<shift-[>":
			return term.KeyComb{Ch: '{'}, nil
		case "<s-\\\\>", "<shift-\\\\>":
			return term.KeyComb{Ch: '|'}, nil
		case "<s-]>", "<shift-]>":
			return term.KeyComb{Ch: '}'}, nil
		case "<s-/>", "<shift-/>":
			return term.KeyComb{Ch: '?'}, nil
		case "<s-_>", "<shift-_>":
			return term.KeyComb{Ch: '_'}, nil
		case "<s-.>", "<shift-.>":
			return term.KeyComb{Ch: '>'}, nil
		case "<s-,>", "<shift-,>":
			return term.KeyComb{Ch: '<'}, nil
		case "<s-;>", "<shift-;>":
			return term.KeyComb{Ch: ':'}, nil
		case "<s-'>", "<shift-'>":
			return term.KeyComb{Ch: '"'}, nil
		case "<s-=>", "<shift-=>":
			return term.KeyComb{Ch: '+'}, nil
		case "<s-+>", "<shift-+>":
			return term.KeyComb{Ch: '+'}, nil
		case "<s-->", "<shift-->":
			return term.KeyComb{Ch: '_'}, nil

		// ctrl
		case "<c-f1>", "<ctrl-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrl}, nil
		case "<c-f2>", "<ctrl-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrl}, nil
		case "<c-f3>", "<ctrl-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrl}, nil
		case "<c-f4>", "<ctrl-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrl}, nil
		case "<c-f5>", "<ctrl-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrl}, nil
		case "<c-f6>", "<ctrl-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrl}, nil
		case "<c-f7>", "<ctrl-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrl}, nil
		case "<c-f8>", "<ctrl-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrl}, nil
		case "<c-f9>", "<ctrl-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrl}, nil
		case "<c-f10>", "<ctrl-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrl}, nil
		case "<c-f11>", "<ctrl-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrl}, nil
		case "<c-f12>", "<ctrl-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrl}, nil
		case "<c-insert>", "<ctrl-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrl}, nil
		case "<c-delete>", "<ctrl-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrl}, nil
		case "<c-home>", "<ctrl-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrl}, nil
		case "<c-end>", "<ctrl-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrl}, nil
		case "<c-pgup>", "<ctrl-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrl}, nil
		case "<c-pgdn>", "<ctrl-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrl}, nil
		case "<c-up>", "<ctrl-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrl}, nil
		case "<c-down>", "<ctrl-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrl}, nil
		case "<c-left>", "<ctrl-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrl}, nil
		case "<c-right>", "<ctrl-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrl}, nil
		case "<c-mouse-left>", "<ctrl-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrl}, nil
		case "<c-mouse-middle>", "<ctrl-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrl}, nil
		case "<c-mouse-right>", "<ctrl-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrl}, nil
		case "<c-mouse-release>", "<ctrl-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrl}, nil
		case "<c-mouse-wheel-up>", "<ctrl-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrl}, nil
		case "<c-mouse-wheel-down>", "<ctrl-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrl}, nil
		case "<c-enter>", "<ctrl-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrl}, nil
		case "<c-space>", "<ctrl-space>":
			return term.KeyComb{Mod: term.ModCtrl, Key: term.KeySpace}, nil
		case "<c-backspace>", "<ctrl-backspace>":
			return term.KeyComb{Mod: term.ModCtrl, Key: term.KeyBackspace}, nil
		case "<c-esc>", "<ctrl-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrl}, nil
		case "<c-1>", "<ctrl-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModCtrl}, nil
		case "<c-2>", "<ctrl-2>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '2'}, nil
		case "<c-3>", "<ctrl-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModCtrl}, nil
		case "<c-4>", "<ctrl-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModCtrl}, nil
		case "<c-5>", "<ctrl-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModCtrl}, nil
		case "<c-6>", "<ctrl-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModCtrl}, nil
		case "<c-7>", "<ctrl-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModCtrl}, nil
		case "<c-8>", "<ctrl-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModCtrl}, nil
		case "<c-9>", "<ctrl-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModCtrl}, nil
		case "<c-0>", "<ctrl-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModCtrl}, nil
		case "<c-`>", "<ctrl-`>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '`'}, nil
		case "<c-a>", "<ctrl-a>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'a'}, nil
		case "<c-b>", "<ctrl-b>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'b'}, nil
		case "<c-c>", "<ctrl-c>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'c'}, nil
		case "<c-d>", "<ctrl-d>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'd'}, nil
		case "<c-e>", "<ctrl-e>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'e'}, nil
		case "<c-f>", "<ctrl-f>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'f'}, nil
		case "<c-g>", "<ctrl-g>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'g'}, nil
		case "<c-h>", "<ctrl-h>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'h'}, nil
		case "<c-tab>", "<ctrl-tab>":
			return term.KeyComb{Mod: term.ModCtrl, Key: term.KeyTab}, nil
		case "<c-i>", "<ctrl-i>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'i'}, nil
		case "<c-j>", "<ctrl-j>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'j'}, nil
		case "<c-k>", "<ctrl-k>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'k'}, nil
		case "<c-l>", "<ctrl-l>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'l'}, nil
		case "<c-m>", "<ctrl-m>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'm'}, nil
		case "<c-n>", "<ctrl-n>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'n'}, nil
		case "<c-o>", "<ctrl-o>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'o'}, nil
		case "<c-p>", "<ctrl-p>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'p'}, nil
		case "<c-q>", "<ctrl-q>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'q'}, nil
		case "<c-r>", "<ctrl-r>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'r'}, nil
		case "<c-s>", "<ctrl-s>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 's'}, nil
		case "<c-t>", "<ctrl-t>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 't'}, nil
		case "<c-u>", "<ctrl-u>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'u'}, nil
		case "<c-v>", "<ctrl-v>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'v'}, nil
		case "<c-w>", "<ctrl-w>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'w'}, nil
		case "<c-x>", "<ctrl-x>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'x'}, nil
		case "<c-y>", "<ctrl-y>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'y'}, nil
		case "<c-z>", "<ctrl-z>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'z'}, nil
		case "<c-[>", "<ctrl-[>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '['}, nil
		case "<c-\\\\>", "<ctrl-\\\\>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '\\'}, nil
		case "<c-]>", "<ctrl-]>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: ']'}, nil
		case "<c-/>", "<ctrl-/>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '/'}, nil
		case "<c-_>", "<ctrl-_>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '_'}, nil
		case "<c-.>", "<ctrl-.>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '.'}, nil
		case "<c-,>", "<ctrl-,>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: ','}, nil
		case "<c-;>", "<ctrl-;>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: ';'}, nil
		case "<c-'>", "<ctrl-'>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '\''}, nil
		case "<c-=>", "<ctrl-=>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '='}, nil
		case "<c-->", "<ctrl-->":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '-'}, nil

		// ctrl+shift
		case "<c-s-f1>", "<ctrl-shift-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShift}, nil
		case "<c-s-f2>", "<ctrl-shift-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShift}, nil
		case "<c-s-f3>", "<ctrl-shift-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShift}, nil
		case "<c-s-f4>", "<ctrl-shift-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShift}, nil
		case "<c-s-f5>", "<ctrl-shift-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShift}, nil
		case "<c-s-f6>", "<ctrl-shift-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShift}, nil
		case "<c-s-f7>", "<ctrl-shift-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShift}, nil
		case "<c-s-f8>", "<ctrl-shift-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShift}, nil
		case "<c-s-f9>", "<ctrl-shift-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShift}, nil
		case "<c-s-f10>", "<ctrl-shift-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShift}, nil
		case "<c-s-f11>", "<ctrl-shift-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShift}, nil
		case "<c-s-f12>", "<ctrl-shift-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShift}, nil
		case "<c-s-insert>", "<ctrl-shift-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShift}, nil
		case "<c-s-delete>", "<ctrl-shift-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShift}, nil
		case "<c-s-home>", "<ctrl-shift-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShift}, nil
		case "<c-s-end>", "<ctrl-shift-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShift}, nil
		case "<c-s-pgup>", "<ctrl-shift-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShift}, nil
		case "<c-s-pgdn>", "<ctrl-shift-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShift}, nil
		case "<c-s-up>", "<ctrl-shift-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShift}, nil
		case "<c-s-down>", "<ctrl-shift-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShift}, nil
		case "<c-s-left>", "<ctrl-shift-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShift}, nil
		case "<c-s-right>", "<ctrl-shift-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShift}, nil
		case "<c-s-mouse-left>", "<ctrl-shift-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShift}, nil
		case "<c-s-mouse-middle>", "<ctrl-shift-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShift}, nil
		case "<c-s-mouse-right>", "<ctrl-shift-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShift}, nil
		case "<c-s-mouse-release>", "<ctrl-shift-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShift}, nil
		case "<c-s-mouse-wheel-up>", "<ctrl-shift-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShift}, nil
		case "<c-s-mouse-wheel-down>", "<ctrl-shift-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShift}, nil
		case "<c-s-enter>", "<ctrl-shift-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShift}, nil
		case "<c-s-space>", "<ctrl-shift-space>":
			return term.KeyComb{Mod: term.ModCtrlShift, Key: term.KeySpace}, nil
		case "<c-s-backspace>", "<ctrl-shift-backspace>":
			return term.KeyComb{Mod: term.ModCtrlShift, Key: term.KeyBackspace}, nil
		case "<c-s-esc>", "<ctrl-shift-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShift}, nil
		case "<c-s-1>", "<ctrl-shift-1>":
			return term.KeyComb{Ch: '!', Mod: term.ModCtrl}, nil
		case "<c-s-2>", "<ctrl-shift-2>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '@'}, nil
		case "<c-s-3>", "<ctrl-shift-3>":
			return term.KeyComb{Ch: '#', Mod: term.ModCtrl}, nil
		case "<c-s-4>", "<ctrl-shift-4>":
			return term.KeyComb{Ch: '$', Mod: term.ModCtrl}, nil
		case "<c-s-5>", "<ctrl-shift-5>":
			return term.KeyComb{Ch: '%', Mod: term.ModCtrl}, nil
		case "<c-s-6>", "<ctrl-shift-6>":
			return term.KeyComb{Ch: '^', Mod: term.ModCtrl}, nil
		case "<c-s-7>", "<ctrl-shift-7>":
			return term.KeyComb{Ch: '&', Mod: term.ModCtrl}, nil
		case "<c-s-8>", "<ctrl-shift-8>":
			return term.KeyComb{Ch: '*', Mod: term.ModCtrl}, nil
		case "<c-s-9>", "<ctrl-shift-9>":
			return term.KeyComb{Ch: '(', Mod: term.ModCtrl}, nil
		case "<c-s-0>", "<ctrl-shift-0>":
			return term.KeyComb{Ch: ')', Mod: term.ModCtrl}, nil
		case "<c-s-`>", "<ctrl-shift-`>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '~'}, nil
		case "<c-s-a>", "<ctrl-shift-a>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'A'}, nil
		case "<c-s-b>", "<ctrl-shift-b>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'B'}, nil
		case "<c-s-c>", "<ctrl-shift-c>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'C'}, nil
		case "<c-s-d>", "<ctrl-shift-d>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'D'}, nil
		case "<c-s-e>", "<ctrl-shift-e>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'E'}, nil
		case "<c-s-f>", "<ctrl-shift-f>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'F'}, nil
		case "<c-s-g>", "<ctrl-shift-g>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'G'}, nil
		case "<c-s-h>", "<ctrl-shift-h>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'H'}, nil
		case "<c-s-tab>", "<ctrl-shift-tab>":
			return term.KeyComb{Mod: term.ModCtrlShift, Key: term.KeyTab}, nil
		case "<c-s-i>", "<ctrl-shift-i>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'I'}, nil
		case "<c-s-j>", "<ctrl-shift-j>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'J'}, nil
		case "<c-s-k>", "<ctrl-shift-k>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'K'}, nil
		case "<c-s-l>", "<ctrl-shift-l>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'L'}, nil
		case "<c-s-m>", "<ctrl-shift-m>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'M'}, nil
		case "<c-s-n>", "<ctrl-shift-n>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'N'}, nil
		case "<c-s-o>", "<ctrl-shift-o>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'O'}, nil
		case "<c-s-p>", "<ctrl-shift-p>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'P'}, nil
		case "<c-s-q>", "<ctrl-shift-q>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'Q'}, nil
		case "<c-s-r>", "<ctrl-shift-r>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'R'}, nil
		case "<c-s-s>", "<ctrl-shift-s>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'S'}, nil
		case "<c-s-t>", "<ctrl-shift-t>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'T'}, nil
		case "<c-s-u>", "<ctrl-shift-u>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'U'}, nil
		case "<c-s-v>", "<ctrl-shift-v>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'V'}, nil
		case "<c-s-w>", "<ctrl-shift-w>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'W'}, nil
		case "<c-s-x>", "<ctrl-shift-x>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'X'}, nil
		case "<c-s-y>", "<ctrl-shift-y>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'Y'}, nil
		case "<c-s-z>", "<ctrl-shift-z>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: 'Z'}, nil
		case "<c-s-[>", "<ctrl-shift-[>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '{'}, nil
		case "<c-s-\\\\>", "<ctrl-shift-\\\\>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '|'}, nil
		case "<c-s-]>", "<ctrl-shift-]>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '}'}, nil
		case "<c-s-/>", "<ctrl-shift-/>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '?'}, nil
		case "<c-s-_>", "<ctrl-shift-_>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '_'}, nil
		case "<c-s-.>", "<ctrl-shift-.>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '>'}, nil
		case "<c-s-,>", "<ctrl-shift-,>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '<'}, nil
		case "<c-s-;>", "<ctrl-shift-;>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: ':'}, nil
		case "<c-s-'>", "<ctrl-shift-'>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '"'}, nil
		case "<c-s-=>", "<ctrl-shift-=>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '+'}, nil
		case "<c-s-+>", "<ctrl-shift-+>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '+'}, nil
		case "<c-+>", "<ctrl-+>":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '+'}, nil
		case "<c-s-->", "<ctrl-shift-->":
			return term.KeyComb{Mod: term.ModCtrl, Ch: '_'}, nil

		// ctrl+alt
		case "<c-a-f1>", "<ctrl-alt-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f2>", "<ctrl-alt-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f3>", "<ctrl-alt-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f4>", "<ctrl-alt-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f5>", "<ctrl-alt-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f6>", "<ctrl-alt-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f7>", "<ctrl-alt-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f8>", "<ctrl-alt-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f9>", "<ctrl-alt-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f10>", "<ctrl-alt-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f11>", "<ctrl-alt-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlAlt}, nil
		case "<c-a-f12>", "<ctrl-alt-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlAlt}, nil
		case "<c-a-insert>", "<ctrl-alt-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlAlt}, nil
		case "<c-a-delete>", "<ctrl-alt-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlAlt}, nil
		case "<c-a-home>", "<ctrl-alt-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlAlt}, nil
		case "<c-a-end>", "<ctrl-alt-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlAlt}, nil
		case "<c-a-pgup>", "<ctrl-alt-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlAlt}, nil
		case "<c-a-pgdn>", "<ctrl-alt-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlAlt}, nil
		case "<c-a-up>", "<ctrl-alt-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlAlt}, nil
		case "<c-a-down>", "<ctrl-alt-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlAlt}, nil
		case "<c-a-left>", "<ctrl-alt-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlAlt}, nil
		case "<c-a-right>", "<ctrl-alt-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlAlt}, nil
		case "<c-a-mouse-left>", "<ctrl-alt-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlAlt}, nil
		case "<c-a-mouse-middle>", "<ctrl-alt-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlAlt}, nil
		case "<c-a-mouse-right>", "<ctrl-alt-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlAlt}, nil
		case "<c-a-mouse-release>", "<ctrl-alt-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlAlt}, nil
		case "<c-a-mouse-wheel-up>", "<ctrl-alt-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlAlt}, nil
		case "<c-a-mouse-wheel-down>", "<ctrl-alt-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlAlt}, nil
		case "<c-a-enter>", "<ctrl-alt-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlAlt}, nil
		case "<c-a-space>", "<ctrl-alt-space>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Key: term.KeySpace}, nil
		case "<c-a-backspace>", "<ctrl-alt-backspace>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Key: term.KeyBackspace}, nil
		case "<c-a-esc>", "<ctrl-alt-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlAlt}, nil
		case "<c-a-1>", "<ctrl-alt-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModCtrlAlt}, nil
		case "<c-a-2>", "<ctrl-alt-2>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '2'}, nil
		case "<c-a-3>", "<ctrl-alt-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModCtrlAlt}, nil
		case "<c-a-4>", "<ctrl-alt-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModCtrlAlt}, nil
		case "<c-a-5>", "<ctrl-alt-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModCtrlAlt}, nil
		case "<c-a-6>", "<ctrl-alt-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModCtrlAlt}, nil
		case "<c-a-7>", "<ctrl-alt-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModCtrlAlt}, nil
		case "<c-a-8>", "<ctrl-alt-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModCtrlAlt}, nil
		case "<c-a-9>", "<ctrl-alt-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModCtrlAlt}, nil
		case "<c-a-0>", "<ctrl-alt-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModCtrlAlt}, nil
		case "<c-a-`>", "<ctrl-alt-`>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '`'}, nil
		case "<c-a-a>", "<ctrl-alt-a>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'a'}, nil
		case "<c-a-b>", "<ctrl-alt-b>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'b'}, nil
		case "<c-a-c>", "<ctrl-alt-c>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'c'}, nil
		case "<c-a-d>", "<ctrl-alt-d>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'd'}, nil
		case "<c-a-e>", "<ctrl-alt-e>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'e'}, nil
		case "<c-a-f>", "<ctrl-alt-f>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'f'}, nil
		case "<c-a-g>", "<ctrl-alt-g>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'g'}, nil
		case "<c-a-h>", "<ctrl-alt-h>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'h'}, nil
		case "<c-a-tab>", "<ctrl-alt-tab>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Key: term.KeyTab}, nil
		case "<c-a-i>", "<ctrl-alt-i>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'i'}, nil
		case "<c-a-j>", "<ctrl-alt-j>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'j'}, nil
		case "<c-a-k>", "<ctrl-alt-k>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'k'}, nil
		case "<c-a-l>", "<ctrl-alt-l>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'l'}, nil
		case "<c-a-m>", "<ctrl-alt-m>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'm'}, nil
		case "<c-a-n>", "<ctrl-alt-n>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'n'}, nil
		case "<c-a-o>", "<ctrl-alt-o>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'o'}, nil
		case "<c-a-p>", "<ctrl-alt-p>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'p'}, nil
		case "<c-a-q>", "<ctrl-alt-q>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'q'}, nil
		case "<c-a-r>", "<ctrl-alt-r>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'r'}, nil
		case "<c-a-s>", "<ctrl-alt-s>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 's'}, nil
		case "<c-a-t>", "<ctrl-alt-t>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 't'}, nil
		case "<c-a-u>", "<ctrl-alt-u>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'u'}, nil
		case "<c-a-v>", "<ctrl-alt-v>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'v'}, nil
		case "<c-a-w>", "<ctrl-alt-w>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'w'}, nil
		case "<c-a-x>", "<ctrl-alt-x>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'x'}, nil
		case "<c-a-y>", "<ctrl-alt-y>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'y'}, nil
		case "<c-a-z>", "<ctrl-alt-z>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'z'}, nil
		case "<c-a-[>", "<ctrl-alt-[>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '['}, nil
		case "<c-a-\\\\>", "<ctrl-alt-\\\\>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '\\'}, nil
		case "<c-a-]>", "<ctrl-alt-]>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: ']'}, nil
		case "<c-a-/>", "<ctrl-alt-/>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '/'}, nil
		case "<c-a-_>", "<ctrl-alt-_>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '_'}, nil
		case "<c-a-.>", "<ctrl-alt-.>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '.'}, nil
		case "<c-a-,>", "<ctrl-alt-,>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: ','}, nil
		case "<c-a-;>", "<ctrl-alt-;>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: ';'}, nil
		case "<c-a-'>", "<ctrl-alt-'>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '\''}, nil
		case "<c-a-=>", "<ctrl-alt-=>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '='}, nil
		case "<c-a-->", "<ctrl-alt-->":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '-'}, nil

		// ctrl+meta
		case "<c-m-f1>", "<ctrl-meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f2>", "<ctrl-meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f3>", "<ctrl-meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f4>", "<ctrl-meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f5>", "<ctrl-meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f6>", "<ctrl-meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f7>", "<ctrl-meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f8>", "<ctrl-meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f9>", "<ctrl-meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f10>", "<ctrl-meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f11>", "<ctrl-meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlMeta}, nil
		case "<c-m-f12>", "<ctrl-meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlMeta}, nil
		case "<c-m-insert>", "<ctrl-meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlMeta}, nil
		case "<c-m-delete>", "<ctrl-meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlMeta}, nil
		case "<c-m-home>", "<ctrl-meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlMeta}, nil
		case "<c-m-end>", "<ctrl-meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlMeta}, nil
		case "<c-m-pgup>", "<ctrl-meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlMeta}, nil
		case "<c-m-pgdn>", "<ctrl-meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlMeta}, nil
		case "<c-m-up>", "<ctrl-meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlMeta}, nil
		case "<c-m-down>", "<ctrl-meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlMeta}, nil
		case "<c-m-left>", "<ctrl-meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlMeta}, nil
		case "<c-m-right>", "<ctrl-meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlMeta}, nil
		case "<c-m-mouse-left>", "<ctrl-meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlMeta}, nil
		case "<c-m-mouse-middle>", "<ctrl-meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlMeta}, nil
		case "<c-m-mouse-right>", "<ctrl-meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlMeta}, nil
		case "<c-m-mouse-release>", "<ctrl-meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlMeta}, nil
		case "<c-m-mouse-wheel-up>", "<ctrl-meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlMeta}, nil
		case "<c-m-mouse-wheel-down>", "<ctrl-meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlMeta}, nil
		case "<c-m-enter>", "<ctrl-meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlMeta}, nil
		case "<c-m-space>", "<ctrl-meta-space>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Key: term.KeySpace}, nil
		case "<c-m-backspace>", "<ctrl-meta-backspace>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Key: term.KeyBackspace}, nil
		case "<c-m-esc>", "<ctrl-meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlMeta}, nil
		case "<c-m-1>", "<ctrl-meta-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModCtrlMeta}, nil
		case "<c-m-2>", "<ctrl-meta-2>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '2'}, nil
		case "<c-m-3>", "<ctrl-meta-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModCtrlMeta}, nil
		case "<c-m-4>", "<ctrl-meta-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModCtrlMeta}, nil
		case "<c-m-5>", "<ctrl-meta-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModCtrlMeta}, nil
		case "<c-m-6>", "<ctrl-meta-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModCtrlMeta}, nil
		case "<c-m-7>", "<ctrl-meta-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModCtrlMeta}, nil
		case "<c-m-8>", "<ctrl-meta-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModCtrlMeta}, nil
		case "<c-m-9>", "<ctrl-meta-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModCtrlMeta}, nil
		case "<c-m-0>", "<ctrl-meta-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModCtrlMeta}, nil
		case "<c-m-`>", "<ctrl-meta-`>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '`'}, nil
		case "<c-m-a>", "<ctrl-meta-a>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'a'}, nil
		case "<c-m-b>", "<ctrl-meta-b>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'b'}, nil
		case "<c-m-c>", "<ctrl-meta-c>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'c'}, nil
		case "<c-m-d>", "<ctrl-meta-d>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'd'}, nil
		case "<c-m-e>", "<ctrl-meta-e>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'e'}, nil
		case "<c-m-f>", "<ctrl-meta-f>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'f'}, nil
		case "<c-m-g>", "<ctrl-meta-g>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'g'}, nil
		case "<c-m-h>", "<ctrl-meta-h>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'h'}, nil
		case "<c-m-tab>", "<ctrl-meta-tab>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Key: term.KeyTab}, nil
		case "<c-m-i>", "<ctrl-meta-i>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'i'}, nil
		case "<c-m-j>", "<ctrl-meta-j>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'j'}, nil
		case "<c-m-k>", "<ctrl-meta-k>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'k'}, nil
		case "<c-m-l>", "<ctrl-meta-l>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'l'}, nil
		case "<c-m-m>", "<ctrl-meta-m>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'm'}, nil
		case "<c-m-n>", "<ctrl-meta-n>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'n'}, nil
		case "<c-m-o>", "<ctrl-meta-o>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'o'}, nil
		case "<c-m-p>", "<ctrl-meta-p>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'p'}, nil
		case "<c-m-q>", "<ctrl-meta-q>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'q'}, nil
		case "<c-m-r>", "<ctrl-meta-r>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'r'}, nil
		case "<c-m-s>", "<ctrl-meta-s>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 's'}, nil
		case "<c-m-t>", "<ctrl-meta-t>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 't'}, nil
		case "<c-m-u>", "<ctrl-meta-u>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'u'}, nil
		case "<c-m-v>", "<ctrl-meta-v>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'v'}, nil
		case "<c-m-w>", "<ctrl-meta-w>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'w'}, nil
		case "<c-m-x>", "<ctrl-meta-x>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'x'}, nil
		case "<c-m-y>", "<ctrl-meta-y>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'y'}, nil
		case "<c-m-z>", "<ctrl-meta-z>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'z'}, nil
		case "<c-m-[>", "<ctrl-meta-[>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '['}, nil
		case "<c-m-\\\\>", "<ctrl-meta-\\\\>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '\\'}, nil
		case "<c-m-]>", "<ctrl-meta-]>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: ']'}, nil
		case "<c-m-/>", "<ctrl-meta-/>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '/'}, nil
		case "<c-m-_>", "<ctrl-meta-_>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '_'}, nil
		case "<c-m-.>", "<ctrl-meta-.>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '.'}, nil
		case "<c-m-,>", "<ctrl-meta-,>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: ','}, nil
		case "<c-m-;>", "<ctrl-meta-;>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: ';'}, nil
		case "<c-m-'>", "<ctrl-meta-'>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '\''}, nil
		case "<c-m-=>", "<ctrl-meta-=>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '='}, nil
		case "<c-m-->", "<ctrl-meta-->":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '-'}, nil

		// ctrl+shift+alt
		case "<c-s-a-f1>", "<ctrl-shift-alt-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f2>", "<ctrl-shift-alt-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f3>", "<ctrl-shift-alt-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f4>", "<ctrl-shift-alt-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f5>", "<ctrl-shift-alt-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f6>", "<ctrl-shift-alt-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f7>", "<ctrl-shift-alt-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f8>", "<ctrl-shift-alt-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f9>", "<ctrl-shift-alt-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f10>", "<ctrl-shift-alt-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f11>", "<ctrl-shift-alt-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-f12>", "<ctrl-shift-alt-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-insert>", "<ctrl-shift-alt-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-delete>", "<ctrl-shift-alt-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-home>", "<ctrl-shift-alt-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-end>", "<ctrl-shift-alt-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-pgup>", "<ctrl-shift-alt-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-pgdn>", "<ctrl-shift-alt-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-up>", "<ctrl-shift-alt-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-down>", "<ctrl-shift-alt-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-left>", "<ctrl-shift-alt-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-right>", "<ctrl-shift-alt-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-mouse-left>", "<ctrl-shift-alt-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-mouse-middle>", "<ctrl-shift-alt-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-mouse-right>", "<ctrl-shift-alt-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-mouse-release>", "<ctrl-shift-alt-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-mouse-wheel-up>", "<ctrl-shift-alt-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-mouse-wheel-down>", "<ctrl-shift-alt-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-enter>", "<ctrl-shift-alt-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-space>", "<ctrl-shift-alt-space>":
			return term.KeyComb{Mod: term.ModCtrlShiftAlt, Key: term.KeySpace}, nil
		case "<c-s-a-backspace>", "<ctrl-shift-alt-backspace>":
			return term.KeyComb{Mod: term.ModCtrlShiftAlt, Key: term.KeyBackspace}, nil
		case "<c-s-a-esc>", "<ctrl-shift-alt-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShiftAlt}, nil
		case "<c-s-a-1>", "<ctrl-shift-alt-1>":
			return term.KeyComb{Ch: '!', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-2>", "<ctrl-shift-alt-2>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '@'}, nil
		case "<c-s-a-3>", "<ctrl-shift-alt-3>":
			return term.KeyComb{Ch: '#', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-4>", "<ctrl-shift-alt-4>":
			return term.KeyComb{Ch: '$', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-5>", "<ctrl-shift-alt-5>":
			return term.KeyComb{Ch: '%', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-6>", "<ctrl-shift-alt-6>":
			return term.KeyComb{Ch: '^', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-7>", "<ctrl-shift-alt-7>":
			return term.KeyComb{Ch: '&', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-8>", "<ctrl-shift-alt-8>":
			return term.KeyComb{Ch: '*', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-9>", "<ctrl-shift-alt-9>":
			return term.KeyComb{Ch: '(', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-0>", "<ctrl-shift-alt-0>":
			return term.KeyComb{Ch: ')', Mod: term.ModCtrlAlt}, nil
		case "<c-s-a-`>", "<ctrl-shift-alt-`>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '~'}, nil
		case "<c-s-a-a>", "<ctrl-shift-alt-a>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'A'}, nil
		case "<c-s-a-b>", "<ctrl-shift-alt-b>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'B'}, nil
		case "<c-s-a-c>", "<ctrl-shift-alt-c>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'C'}, nil
		case "<c-s-a-d>", "<ctrl-shift-alt-d>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'D'}, nil
		case "<c-s-a-e>", "<ctrl-shift-alt-e>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'E'}, nil
		case "<c-s-a-f>", "<ctrl-shift-alt-f>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'F'}, nil
		case "<c-s-a-g>", "<ctrl-shift-alt-g>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'G'}, nil
		case "<c-s-a-h>", "<ctrl-shift-alt-h>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'H'}, nil
		case "<c-s-a-tab>", "<ctrl-shift-alt-tab>":
			return term.KeyComb{Mod: term.ModCtrlShiftAlt, Key: term.KeyTab}, nil
		case "<c-s-a-i>", "<ctrl-shift-alt-i>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'I'}, nil
		case "<c-s-a-j>", "<ctrl-shift-alt-j>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'J'}, nil
		case "<c-s-a-k>", "<ctrl-shift-alt-k>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'K'}, nil
		case "<c-s-a-l>", "<ctrl-shift-alt-l>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'L'}, nil
		case "<c-s-a-m>", "<ctrl-shift-alt-m>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'M'}, nil
		case "<c-s-a-n>", "<ctrl-shift-alt-n>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'N'}, nil
		case "<c-s-a-o>", "<ctrl-shift-alt-o>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'O'}, nil
		case "<c-s-a-p>", "<ctrl-shift-alt-p>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'P'}, nil
		case "<c-s-a-q>", "<ctrl-shift-alt-q>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'Q'}, nil
		case "<c-s-a-r>", "<ctrl-shift-alt-r>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'R'}, nil
		case "<c-s-a-s>", "<ctrl-shift-alt-s>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'S'}, nil
		case "<c-s-a-t>", "<ctrl-shift-alt-t>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'T'}, nil
		case "<c-s-a-u>", "<ctrl-shift-alt-u>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'U'}, nil
		case "<c-s-a-v>", "<ctrl-shift-alt-v>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'V'}, nil
		case "<c-s-a-w>", "<ctrl-shift-alt-w>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'W'}, nil
		case "<c-s-a-x>", "<ctrl-shift-alt-x>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'X'}, nil
		case "<c-s-a-y>", "<ctrl-shift-alt-y>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'Y'}, nil
		case "<c-s-a-z>", "<ctrl-shift-alt-z>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: 'Z'}, nil
		case "<c-s-a-[>", "<ctrl-shift-alt-[>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '{'}, nil
		case "<c-s-a-\\\\>", "<ctrl-shift-alt-\\\\>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '|'}, nil
		case "<c-s-a-]>", "<ctrl-shift-alt-]>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '}'}, nil
		case "<c-s-a-/>", "<ctrl-shift-alt-/>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '?'}, nil
		case "<c-s-a-_>", "<ctrl-shift-alt-_>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '_'}, nil
		case "<c-s-a-.>", "<ctrl-shift-alt-.>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '>'}, nil
		case "<c-s-a-,>", "<ctrl-shift-alt-,>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '<'}, nil
		case "<c-s-a-;>", "<ctrl-shift-alt-;>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: ':'}, nil
		case "<c-s-a-'>", "<ctrl-shift-alt-'>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '"'}, nil
		case "<c-s-a-=>", "<ctrl-shift-alt-=>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '+'}, nil
		case "<c-s-a-+>", "<ctrl-shift-alt-+>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '+'}, nil
		case "<c-a-+>", "<ctrl-alt-+>":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '+'}, nil
		case "<c-s-a-->", "<ctrl-shift-alt-->":
			return term.KeyComb{Mod: term.ModCtrlAlt, Ch: '_'}, nil

		// ctrl+shift+meta
		case "<c-s-m-f1>", "<ctrl-shift-meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f2>", "<ctrl-shift-meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f3>", "<ctrl-shift-meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f4>", "<ctrl-shift-meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f5>", "<ctrl-shift-meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f6>", "<ctrl-shift-meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f7>", "<ctrl-shift-meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f8>", "<ctrl-shift-meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f9>", "<ctrl-shift-meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f10>", "<ctrl-shift-meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f11>", "<ctrl-shift-meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-f12>", "<ctrl-shift-meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-insert>", "<ctrl-shift-meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-delete>", "<ctrl-shift-meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-home>", "<ctrl-shift-meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-end>", "<ctrl-shift-meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-pgup>", "<ctrl-shift-meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-pgdn>", "<ctrl-shift-meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-up>", "<ctrl-shift-meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-down>", "<ctrl-shift-meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-left>", "<ctrl-shift-meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-right>", "<ctrl-shift-meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-mouse-left>", "<ctrl-shift-meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-mouse-middle>", "<ctrl-shift-meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-mouse-right>", "<ctrl-shift-meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-mouse-release>", "<ctrl-shift-meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-mouse-wheel-up>", "<ctrl-shift-meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-mouse-wheel-down>", "<ctrl-shift-meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-enter>", "<ctrl-shift-meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-space>", "<ctrl-shift-meta-space>":
			return term.KeyComb{Mod: term.ModCtrlShiftMeta, Key: term.KeySpace}, nil
		case "<c-s-m-backspace>", "<ctrl-shift-meta-backspace>":
			return term.KeyComb{Mod: term.ModCtrlShiftMeta, Key: term.KeyBackspace}, nil
		case "<c-s-m-esc>", "<ctrl-shift-meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlShiftMeta}, nil
		case "<c-s-m-1>", "<ctrl-shift-meta-1>":
			return term.KeyComb{Ch: '!', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-2>", "<ctrl-shift-meta-2>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '@'}, nil
		case "<c-s-m-3>", "<ctrl-shift-meta-3>":
			return term.KeyComb{Ch: '#', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-4>", "<ctrl-shift-meta-4>":
			return term.KeyComb{Ch: '$', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-5>", "<ctrl-shift-meta-5>":
			return term.KeyComb{Ch: '%', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-6>", "<ctrl-shift-meta-6>":
			return term.KeyComb{Ch: '^', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-7>", "<ctrl-shift-meta-7>":
			return term.KeyComb{Ch: '&', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-8>", "<ctrl-shift-meta-8>":
			return term.KeyComb{Ch: '*', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-9>", "<ctrl-shift-meta-9>":
			return term.KeyComb{Ch: '(', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-0>", "<ctrl-shift-meta-0>":
			return term.KeyComb{Ch: ')', Mod: term.ModCtrlMeta}, nil
		case "<c-s-m-`>", "<ctrl-shift-meta-`>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '~'}, nil
		case "<c-s-m-a>", "<ctrl-shift-meta-a>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'A'}, nil
		case "<c-s-m-b>", "<ctrl-shift-meta-b>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'B'}, nil
		case "<c-s-m-c>", "<ctrl-shift-meta-c>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'C'}, nil
		case "<c-s-m-d>", "<ctrl-shift-meta-d>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'D'}, nil
		case "<c-s-m-e>", "<ctrl-shift-meta-e>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'E'}, nil
		case "<c-s-m-f>", "<ctrl-shift-meta-f>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'F'}, nil
		case "<c-s-m-g>", "<ctrl-shift-meta-g>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'G'}, nil
		case "<c-s-m-h>", "<ctrl-shift-meta-h>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'H'}, nil
		case "<c-s-m-tab>", "<ctrl-shift-meta-tab>":
			return term.KeyComb{Mod: term.ModCtrlShiftMeta, Key: term.KeyTab}, nil
		case "<c-s-m-i>", "<ctrl-shift-meta-i>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'I'}, nil
		case "<c-s-m-j>", "<ctrl-shift-meta-j>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'J'}, nil
		case "<c-s-m-k>", "<ctrl-shift-meta-k>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'K'}, nil
		case "<c-s-m-l>", "<ctrl-shift-meta-l>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'L'}, nil
		case "<c-s-m-m>", "<ctrl-shift-meta-m>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'M'}, nil
		case "<c-s-m-n>", "<ctrl-shift-meta-n>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'N'}, nil
		case "<c-s-m-o>", "<ctrl-shift-meta-o>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'O'}, nil
		case "<c-s-m-p>", "<ctrl-shift-meta-p>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'P'}, nil
		case "<c-s-m-q>", "<ctrl-shift-meta-q>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'Q'}, nil
		case "<c-s-m-r>", "<ctrl-shift-meta-r>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'R'}, nil
		case "<c-s-m-s>", "<ctrl-shift-meta-s>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'S'}, nil
		case "<c-s-m-t>", "<ctrl-shift-meta-t>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'T'}, nil
		case "<c-s-m-u>", "<ctrl-shift-meta-u>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'U'}, nil
		case "<c-s-m-v>", "<ctrl-shift-meta-v>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'V'}, nil
		case "<c-s-m-w>", "<ctrl-shift-meta-w>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'W'}, nil
		case "<c-s-m-x>", "<ctrl-shift-meta-x>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'X'}, nil
		case "<c-s-m-y>", "<ctrl-shift-meta-y>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'Y'}, nil
		case "<c-s-m-z>", "<ctrl-shift-meta-z>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: 'Z'}, nil
		case "<c-s-m-[>", "<ctrl-shift-meta-[>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '{'}, nil
		case "<c-s-m-\\\\>", "<ctrl-shift-meta-\\\\>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '|'}, nil
		case "<c-s-m-]>", "<ctrl-shift-meta-]>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '}'}, nil
		case "<c-s-m-/>", "<ctrl-shift-meta-/>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '?'}, nil
		case "<c-s-m-_>", "<ctrl-shift-meta-_>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '_'}, nil
		case "<c-s-m-.>", "<ctrl-shift-meta-.>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '>'}, nil
		case "<c-s-m-,>", "<ctrl-shift-meta-,>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '<'}, nil
		case "<c-s-m-;>", "<ctrl-shift-meta-;>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: ':'}, nil
		case "<c-s-m-'>", "<ctrl-shift-meta-'>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '"'}, nil
		case "<c-s-m-=>", "<ctrl-shift-meta-=>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '+'}, nil
		case "<c-s-m-+>", "<ctrl-shift-meta-+>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '+'}, nil
		case "<c-m-+>", "<ctrl-meta-+>":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '+'}, nil
		case "<c-s-m-->", "<ctrl-shift-meta-->":
			return term.KeyComb{Mod: term.ModCtrlMeta, Ch: '_'}, nil

		// ctrl+alt+meta
		case "<c-a-m-f1>", "<ctrl-alt-meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f2>", "<ctrl-alt-meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f3>", "<ctrl-alt-meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f4>", "<ctrl-alt-meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f5>", "<ctrl-alt-meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f6>", "<ctrl-alt-meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f7>", "<ctrl-alt-meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f8>", "<ctrl-alt-meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f9>", "<ctrl-alt-meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f10>", "<ctrl-alt-meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f11>", "<ctrl-alt-meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-f12>", "<ctrl-alt-meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-insert>", "<ctrl-alt-meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-delete>", "<ctrl-alt-meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-home>", "<ctrl-alt-meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-end>", "<ctrl-alt-meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-pgup>", "<ctrl-alt-meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-pgdn>", "<ctrl-alt-meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-up>", "<ctrl-alt-meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-down>", "<ctrl-alt-meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-left>", "<ctrl-alt-meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-right>", "<ctrl-alt-meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-mouse-left>", "<ctrl-alt-meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-mouse-middle>", "<ctrl-alt-meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-mouse-right>", "<ctrl-alt-meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-mouse-release>", "<ctrl-alt-meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-mouse-wheel-up>", "<ctrl-alt-meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-mouse-wheel-down>", "<ctrl-alt-meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-enter>", "<ctrl-alt-meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-space>", "<ctrl-alt-meta-space>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Key: term.KeySpace}, nil
		case "<c-a-m-backspace>", "<ctrl-alt-meta-backspace>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Key: term.KeyBackspace}, nil
		case "<c-a-m-esc>", "<ctrl-alt-meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-1>", "<ctrl-alt-meta-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-2>", "<ctrl-alt-meta-2>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '2'}, nil
		case "<c-a-m-3>", "<ctrl-alt-meta-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-4>", "<ctrl-alt-meta-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-5>", "<ctrl-alt-meta-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-6>", "<ctrl-alt-meta-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-7>", "<ctrl-alt-meta-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-8>", "<ctrl-alt-meta-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-9>", "<ctrl-alt-meta-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-0>", "<ctrl-alt-meta-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModCtrlAltMeta}, nil
		case "<c-a-m-`>", "<ctrl-alt-meta-`>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '`'}, nil
		case "<c-a-m-a>", "<ctrl-alt-meta-a>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'a'}, nil
		case "<c-a-m-b>", "<ctrl-alt-meta-b>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'b'}, nil
		case "<c-a-m-c>", "<ctrl-alt-meta-c>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'c'}, nil
		case "<c-a-m-d>", "<ctrl-alt-meta-d>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'd'}, nil
		case "<c-a-m-e>", "<ctrl-alt-meta-e>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'e'}, nil
		case "<c-a-m-f>", "<ctrl-alt-meta-f>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'f'}, nil
		case "<c-a-m-g>", "<ctrl-alt-meta-g>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'g'}, nil
		case "<c-a-m-h>", "<ctrl-alt-meta-h>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'h'}, nil
		case "<c-a-m-tab>", "<ctrl-alt-meta-tab>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Key: term.KeyTab}, nil
		case "<c-a-m-i>", "<ctrl-alt-meta-i>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'i'}, nil
		case "<c-a-m-j>", "<ctrl-alt-meta-j>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'j'}, nil
		case "<c-a-m-k>", "<ctrl-alt-meta-k>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'k'}, nil
		case "<c-a-m-l>", "<ctrl-alt-meta-l>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'l'}, nil
		case "<c-a-m-m>", "<ctrl-alt-meta-m>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'm'}, nil
		case "<c-a-m-n>", "<ctrl-alt-meta-n>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'n'}, nil
		case "<c-a-m-o>", "<ctrl-alt-meta-o>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'o'}, nil
		case "<c-a-m-p>", "<ctrl-alt-meta-p>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'p'}, nil
		case "<c-a-m-q>", "<ctrl-alt-meta-q>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'q'}, nil
		case "<c-a-m-r>", "<ctrl-alt-meta-r>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'r'}, nil
		case "<c-a-m-s>", "<ctrl-alt-meta-s>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 's'}, nil
		case "<c-a-m-t>", "<ctrl-alt-meta-t>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 't'}, nil
		case "<c-a-m-u>", "<ctrl-alt-meta-u>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'u'}, nil
		case "<c-a-m-v>", "<ctrl-alt-meta-v>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'v'}, nil
		case "<c-a-m-w>", "<ctrl-alt-meta-w>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'w'}, nil
		case "<c-a-m-x>", "<ctrl-alt-meta-x>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'x'}, nil
		case "<c-a-m-y>", "<ctrl-alt-meta-y>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'y'}, nil
		case "<c-a-m-z>", "<ctrl-alt-meta-z>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: 'z'}, nil
		case "<c-a-m-[>", "<ctrl-alt-meta-[>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '['}, nil
		case "<c-a-m-\\\\>", "<ctrl-alt-meta-\\\\>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '\\'}, nil
		case "<c-a-m-]>", "<ctrl-alt-meta-]>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: ']'}, nil
		case "<c-a-m-/>", "<ctrl-alt-meta-/>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '/'}, nil
		case "<c-a-m-_>", "<ctrl-alt-meta-_>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '_'}, nil
		case "<c-a-m-.>", "<ctrl-alt-meta-.>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '.'}, nil
		case "<c-a-m-,>", "<ctrl-alt-meta-,>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: ','}, nil
		case "<c-a-m-;>", "<ctrl-alt-meta-;>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: ';'}, nil
		case "<c-a-m-'>", "<ctrl-alt-meta-'>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '\''}, nil
		case "<c-a-m-=>", "<ctrl-alt-meta-=>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '='}, nil
		case "<c-a-m-->", "<ctrl-alt-meta-->":
			return term.KeyComb{Mod: term.ModCtrlAltMeta, Ch: '-'}, nil

		// shift+meta
		case "<s-m-f1>", "<shift-meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModShiftMeta}, nil
		case "<s-m-f2>", "<shift-meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModShiftMeta}, nil
		case "<s-m-f3>", "<shift-meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModShiftMeta}, nil
		case "<s-m-f4>", "<shift-meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModShiftMeta}, nil
		case "<s-m-f5>", "<shift-meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModShiftMeta}, nil
		case "<s-m-f6>", "<shift-meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModShiftMeta}, nil
		case "<s-m-f7>", "<shift-meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModShiftMeta}, nil
		case "<s-m-f8>", "<shift-meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModShiftMeta}, nil
		case "<s-m-f9>", "<shift-meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModShiftMeta}, nil
		case "<s-m-f10>", "<shift-meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModShiftMeta}, nil
		case "<s-m-f11>", "<shift-meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModShiftMeta}, nil
		case "<s-m-f12>", "<shift-meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModShiftMeta}, nil
		case "<s-m-insert>", "<shift-meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModShiftMeta}, nil
		case "<s-m-delete>", "<shift-meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModShiftMeta}, nil
		case "<s-m-home>", "<shift-meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModShiftMeta}, nil
		case "<s-m-end>", "<shift-meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModShiftMeta}, nil
		case "<s-m-pgup>", "<shift-meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModShiftMeta}, nil
		case "<s-m-pgdn>", "<shift-meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModShiftMeta}, nil
		case "<s-m-up>", "<shift-meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModShiftMeta}, nil
		case "<s-m-down>", "<shift-meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModShiftMeta}, nil
		case "<s-m-left>", "<shift-meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModShiftMeta}, nil
		case "<s-m-right>", "<shift-meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModShiftMeta}, nil
		case "<s-m-mouse-left>", "<shift-meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModShiftMeta}, nil
		case "<s-m-mouse-middle>", "<shift-meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModShiftMeta}, nil
		case "<s-m-mouse-right>", "<shift-meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModShiftMeta}, nil
		case "<s-m-mouse-release>", "<shift-meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModShiftMeta}, nil
		case "<s-m-mouse-wheel-up>", "<shift-meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModShiftMeta}, nil
		case "<s-m-mouse-wheel-down>", "<shift-meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModShiftMeta}, nil
		case "<s-m-enter>", "<shift-meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModShiftMeta}, nil
		case "<s-m-space>", "<shift-meta-space>":
			return term.KeyComb{Mod: term.ModShiftMeta, Key: term.KeySpace}, nil
		case "<s-m-backspace>", "<shift-meta-backspace>":
			return term.KeyComb{Mod: term.ModShiftMeta, Key: term.KeyBackspace}, nil
		case "<s-m-esc>", "<shift-meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModShiftMeta}, nil
		case "<s-m-1>", "<shift-meta-1>":
			return term.KeyComb{Ch: '!', Mod: term.ModMeta}, nil
		case "<s-m-2>", "<shift-meta-2>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '@'}, nil
		case "<s-m-3>", "<shift-meta-3>":
			return term.KeyComb{Ch: '#', Mod: term.ModMeta}, nil
		case "<s-m-4>", "<shift-meta-4>":
			return term.KeyComb{Ch: '$', Mod: term.ModMeta}, nil
		case "<s-m-5>", "<shift-meta-5>":
			return term.KeyComb{Ch: '%', Mod: term.ModMeta}, nil
		case "<s-m-6>", "<shift-meta-6>":
			return term.KeyComb{Ch: '^', Mod: term.ModMeta}, nil
		case "<s-m-7>", "<shift-meta-7>":
			return term.KeyComb{Ch: '&', Mod: term.ModMeta}, nil
		case "<s-m-8>", "<shift-meta-8>":
			return term.KeyComb{Ch: '*', Mod: term.ModMeta}, nil
		case "<s-m-9>", "<shift-meta-9>":
			return term.KeyComb{Ch: '(', Mod: term.ModMeta}, nil
		case "<s-m-0>", "<shift-meta-0>":
			return term.KeyComb{Ch: ')', Mod: term.ModMeta}, nil
		case "<s-m-`>", "<shift-meta-`>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '~'}, nil
		case "<s-m-a>", "<shift-meta-a>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'A'}, nil
		case "<s-m-b>", "<shift-meta-b>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'B'}, nil
		case "<s-m-c>", "<shift-meta-c>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'C'}, nil
		case "<s-m-d>", "<shift-meta-d>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'D'}, nil
		case "<s-m-e>", "<shift-meta-e>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'E'}, nil
		case "<s-m-f>", "<shift-meta-f>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'F'}, nil
		case "<s-m-g>", "<shift-meta-g>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'G'}, nil
		case "<s-m-h>", "<shift-meta-h>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'H'}, nil
		case "<s-m-tab>", "<shift-meta-tab>":
			return term.KeyComb{Mod: term.ModShiftMeta, Key: term.KeyTab}, nil
		case "<s-m-i>", "<shift-meta-i>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'I'}, nil
		case "<s-m-j>", "<shift-meta-j>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'J'}, nil
		case "<s-m-k>", "<shift-meta-k>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'K'}, nil
		case "<s-m-l>", "<shift-meta-l>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'L'}, nil
		case "<s-m-m>", "<shift-meta-m>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'M'}, nil
		case "<s-m-n>", "<shift-meta-n>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'N'}, nil
		case "<s-m-o>", "<shift-meta-o>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'O'}, nil
		case "<s-m-p>", "<shift-meta-p>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'P'}, nil
		case "<s-m-q>", "<shift-meta-q>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'Q'}, nil
		case "<s-m-r>", "<shift-meta-r>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'R'}, nil
		case "<s-m-s>", "<shift-meta-s>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'S'}, nil
		case "<s-m-t>", "<shift-meta-t>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'T'}, nil
		case "<s-m-u>", "<shift-meta-u>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'U'}, nil
		case "<s-m-v>", "<shift-meta-v>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'V'}, nil
		case "<s-m-w>", "<shift-meta-w>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'W'}, nil
		case "<s-m-x>", "<shift-meta-x>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'X'}, nil
		case "<s-m-y>", "<shift-meta-y>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'Y'}, nil
		case "<s-m-z>", "<shift-meta-z>":
			return term.KeyComb{Mod: term.ModMeta, Ch: 'Z'}, nil
		case "<s-m-[>", "<shift-meta-[>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '{'}, nil
		case "<s-m-\\\\>", "<shift-meta-\\\\>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '|'}, nil
		case "<s-m-]>", "<shift-meta-]>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '}'}, nil
		case "<s-m-/>", "<shift-meta-/>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '?'}, nil
		case "<s-m-_>", "<shift-meta-_>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '_'}, nil
		case "<s-m-.>", "<shift-meta-.>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '>'}, nil
		case "<s-m-,>", "<shift-meta-,>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '<'}, nil
		case "<s-m-;>", "<shift-meta-;>":
			return term.KeyComb{Mod: term.ModMeta, Ch: ':'}, nil
		case "<s-m-'>", "<shift-meta-'>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '"'}, nil
		case "<s-m-=>", "<shift-meta-=>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '+'}, nil
		case "<s-m-+>", "<shift-meta-+>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '+'}, nil
		case "<m-+>", "<meta-+>":
			return term.KeyComb{Mod: term.ModMeta, Ch: '+'}, nil
		case "<s-m-->", "<shift-meta-->":
			return term.KeyComb{Mod: term.ModMeta, Ch: '_'}, nil

		// alt+meta
		case "<a-m-f1>", "<alt-meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModAltMeta}, nil
		case "<a-m-f2>", "<alt-meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModAltMeta}, nil
		case "<a-m-f3>", "<alt-meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModAltMeta}, nil
		case "<a-m-f4>", "<alt-meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModAltMeta}, nil
		case "<a-m-f5>", "<alt-meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModAltMeta}, nil
		case "<a-m-f6>", "<alt-meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModAltMeta}, nil
		case "<a-m-f7>", "<alt-meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModAltMeta}, nil
		case "<a-m-f8>", "<alt-meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModAltMeta}, nil
		case "<a-m-f9>", "<alt-meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModAltMeta}, nil
		case "<a-m-f10>", "<alt-meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModAltMeta}, nil
		case "<a-m-f11>", "<alt-meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModAltMeta}, nil
		case "<a-m-f12>", "<alt-meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModAltMeta}, nil
		case "<a-m-insert>", "<alt-meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltMeta}, nil
		case "<a-m-delete>", "<alt-meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltMeta}, nil
		case "<a-m-home>", "<alt-meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModAltMeta}, nil
		case "<a-m-end>", "<alt-meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltMeta}, nil
		case "<a-m-pgup>", "<alt-meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltMeta}, nil
		case "<a-m-pgdn>", "<alt-meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltMeta}, nil
		case "<a-m-up>", "<alt-meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltMeta}, nil
		case "<a-m-down>", "<alt-meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltMeta}, nil
		case "<a-m-left>", "<alt-meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltMeta}, nil
		case "<a-m-right>", "<alt-meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltMeta}, nil
		case "<a-m-mouse-left>", "<alt-meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltMeta}, nil
		case "<a-m-mouse-middle>", "<alt-meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltMeta}, nil
		case "<a-m-mouse-right>", "<alt-meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModAltMeta}, nil
		case "<a-m-mouse-release>", "<alt-meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltMeta}, nil
		case "<a-m-mouse-wheel-up>", "<alt-meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltMeta}, nil
		case "<a-m-mouse-wheel-down>", "<alt-meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltMeta}, nil
		case "<a-m-enter>", "<alt-meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltMeta}, nil
		case "<a-m-space>", "<alt-meta-space>":
			return term.KeyComb{Mod: term.ModAltMeta, Key: term.KeySpace}, nil
		case "<a-m-backspace>", "<alt-meta-backspace>":
			return term.KeyComb{Mod: term.ModAltMeta, Key: term.KeyBackspace}, nil
		case "<a-m-esc>", "<alt-meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltMeta}, nil
		case "<a-m-1>", "<alt-meta-1>":
			return term.KeyComb{Ch: '1', Mod: term.ModAltMeta}, nil
		case "<a-m-2>", "<alt-meta-2>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '2'}, nil
		case "<a-m-3>", "<alt-meta-3>":
			return term.KeyComb{Ch: '3', Mod: term.ModAltMeta}, nil
		case "<a-m-4>", "<alt-meta-4>":
			return term.KeyComb{Ch: '4', Mod: term.ModAltMeta}, nil
		case "<a-m-5>", "<alt-meta-5>":
			return term.KeyComb{Ch: '5', Mod: term.ModAltMeta}, nil
		case "<a-m-6>", "<alt-meta-6>":
			return term.KeyComb{Ch: '6', Mod: term.ModAltMeta}, nil
		case "<a-m-7>", "<alt-meta-7>":
			return term.KeyComb{Ch: '7', Mod: term.ModAltMeta}, nil
		case "<a-m-8>", "<alt-meta-8>":
			return term.KeyComb{Ch: '8', Mod: term.ModAltMeta}, nil
		case "<a-m-9>", "<alt-meta-9>":
			return term.KeyComb{Ch: '9', Mod: term.ModAltMeta}, nil
		case "<a-m-0>", "<alt-meta-0>":
			return term.KeyComb{Ch: '0', Mod: term.ModAltMeta}, nil
		case "<a-m-`>", "<alt-meta-`>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '`'}, nil
		case "<a-m-a>", "<alt-meta-a>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'a'}, nil
		case "<a-m-b>", "<alt-meta-b>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'b'}, nil
		case "<a-m-c>", "<alt-meta-c>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'c'}, nil
		case "<a-m-d>", "<alt-meta-d>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'd'}, nil
		case "<a-m-e>", "<alt-meta-e>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'e'}, nil
		case "<a-m-f>", "<alt-meta-f>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'f'}, nil
		case "<a-m-g>", "<alt-meta-g>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'g'}, nil
		case "<a-m-h>", "<alt-meta-h>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'h'}, nil
		case "<a-m-tab>", "<alt-meta-tab>":
			return term.KeyComb{Mod: term.ModAltMeta, Key: term.KeyTab}, nil
		case "<a-m-i>", "<alt-meta-i>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'i'}, nil
		case "<a-m-j>", "<alt-meta-j>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'j'}, nil
		case "<a-m-k>", "<alt-meta-k>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'k'}, nil
		case "<a-m-l>", "<alt-meta-l>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'l'}, nil
		case "<a-m-m>", "<alt-meta-m>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'm'}, nil
		case "<a-m-n>", "<alt-meta-n>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'n'}, nil
		case "<a-m-o>", "<alt-meta-o>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'o'}, nil
		case "<a-m-p>", "<alt-meta-p>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'p'}, nil
		case "<a-m-q>", "<alt-meta-q>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'q'}, nil
		case "<a-m-r>", "<alt-meta-r>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'r'}, nil
		case "<a-m-s>", "<alt-meta-s>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 's'}, nil
		case "<a-m-t>", "<alt-meta-t>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 't'}, nil
		case "<a-m-u>", "<alt-meta-u>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'u'}, nil
		case "<a-m-v>", "<alt-meta-v>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'v'}, nil
		case "<a-m-w>", "<alt-meta-w>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'w'}, nil
		case "<a-m-x>", "<alt-meta-x>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'x'}, nil
		case "<a-m-y>", "<alt-meta-y>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'y'}, nil
		case "<a-m-z>", "<alt-meta-z>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'z'}, nil
		case "<a-m-[>", "<alt-meta-[>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '['}, nil
		case "<a-m-\\\\>", "<alt-meta-\\\\>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '\\'}, nil
		case "<a-m-]>", "<alt-meta-]>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: ']'}, nil
		case "<a-m-/>", "<alt-meta-/>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '/'}, nil
		case "<a-m-_>", "<alt-meta-_>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '_'}, nil
		case "<a-m-.>", "<alt-meta-.>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '.'}, nil
		case "<a-m-,>", "<alt-meta-,>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: ','}, nil
		case "<a-m-;>", "<alt-meta-;>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: ';'}, nil
		case "<a-m-'>", "<alt-meta-'>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '\''}, nil
		case "<a-m-=>", "<alt-meta-=>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '='}, nil
		case "<a-m-->", "<alt-meta-->":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '-'}, nil

		// alt+shift+meta
		case "<a-s-m-f1>", "<alt-shift-meta-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f2>", "<alt-shift-meta-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f3>", "<alt-shift-meta-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f4>", "<alt-shift-meta-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f5>", "<alt-shift-meta-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f6>", "<alt-shift-meta-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f7>", "<alt-shift-meta-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f8>", "<alt-shift-meta-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f9>", "<alt-shift-meta-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f10>", "<alt-shift-meta-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f11>", "<alt-shift-meta-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-f12>", "<alt-shift-meta-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-insert>", "<alt-shift-meta-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-delete>", "<alt-shift-meta-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-home>", "<alt-shift-meta-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-end>", "<alt-shift-meta-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-pgup>", "<alt-shift-meta-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-pgdn>", "<alt-shift-meta-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-up>", "<alt-shift-meta-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-down>", "<alt-shift-meta-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-left>", "<alt-shift-meta-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-right>", "<alt-shift-meta-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-mouse-left>", "<alt-shift-meta-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-mouse-middle>", "<alt-shift-meta-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-mouse-right>", "<alt-shift-meta-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-mouse-release>", "<alt-shift-meta-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-mouse-wheel-up>", "<alt-shift-meta-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-mouse-wheel-down>", "<alt-shift-meta-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-enter>", "<alt-shift-meta-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-space>", "<alt-shift-meta-space>":
			return term.KeyComb{Mod: term.ModAltShiftMeta, Key: term.KeySpace}, nil
		case "<a-s-m-backspace>", "<alt-shift-meta-backspace>":
			return term.KeyComb{Mod: term.ModAltShiftMeta, Key: term.KeyBackspace}, nil
		case "<a-s-m-esc>", "<alt-shift-meta-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltShiftMeta}, nil
		case "<a-s-m-1>", "<alt-shift-meta-1>":
			return term.KeyComb{Ch: '!', Mod: term.ModAltMeta}, nil
		case "<a-s-m-2>", "<alt-shift-meta-2>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '@'}, nil
		case "<a-s-m-3>", "<alt-shift-meta-3>":
			return term.KeyComb{Ch: '#', Mod: term.ModAltMeta}, nil
		case "<a-s-m-4>", "<alt-shift-meta-4>":
			return term.KeyComb{Ch: '$', Mod: term.ModAltMeta}, nil
		case "<a-s-m-5>", "<alt-shift-meta-5>":
			return term.KeyComb{Ch: '%', Mod: term.ModAltMeta}, nil
		case "<a-s-m-6>", "<alt-shift-meta-6>":
			return term.KeyComb{Ch: '^', Mod: term.ModAltMeta}, nil
		case "<a-s-m-7>", "<alt-shift-meta-7>":
			return term.KeyComb{Ch: '&', Mod: term.ModAltMeta}, nil
		case "<a-s-m-8>", "<alt-shift-meta-8>":
			return term.KeyComb{Ch: '*', Mod: term.ModAltMeta}, nil
		case "<a-s-m-9>", "<alt-shift-meta-9>":
			return term.KeyComb{Ch: '(', Mod: term.ModAltMeta}, nil
		case "<a-s-m-0>", "<alt-shift-meta-0>":
			return term.KeyComb{Ch: ')', Mod: term.ModAltMeta}, nil
		case "<a-s-m-`>", "<alt-shift-meta-`>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '~'}, nil
		case "<a-s-m-a>", "<alt-shift-meta-a>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'A'}, nil
		case "<a-s-m-b>", "<alt-shift-meta-b>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'B'}, nil
		case "<a-s-m-c>", "<alt-shift-meta-c>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'C'}, nil
		case "<a-s-m-d>", "<alt-shift-meta-d>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'D'}, nil
		case "<a-s-m-e>", "<alt-shift-meta-e>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'E'}, nil
		case "<a-s-m-f>", "<alt-shift-meta-f>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'F'}, nil
		case "<a-s-m-g>", "<alt-shift-meta-g>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'G'}, nil
		case "<a-s-m-h>", "<alt-shift-meta-h>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'H'}, nil
		case "<a-s-m-tab>", "<alt-shift-meta-tab>":
			return term.KeyComb{Mod: term.ModAltShiftMeta, Key: term.KeyTab}, nil
		case "<a-s-m-i>", "<alt-shift-meta-i>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'I'}, nil
		case "<a-s-m-j>", "<alt-shift-meta-j>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'J'}, nil
		case "<a-s-m-k>", "<alt-shift-meta-k>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'K'}, nil
		case "<a-s-m-l>", "<alt-shift-meta-l>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'L'}, nil
		case "<a-s-m-m>", "<alt-shift-meta-m>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'M'}, nil
		case "<a-s-m-n>", "<alt-shift-meta-n>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'N'}, nil
		case "<a-s-m-o>", "<alt-shift-meta-o>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'O'}, nil
		case "<a-s-m-p>", "<alt-shift-meta-p>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'P'}, nil
		case "<a-s-m-q>", "<alt-shift-meta-q>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'Q'}, nil
		case "<a-s-m-r>", "<alt-shift-meta-r>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'R'}, nil
		case "<a-s-m-s>", "<alt-shift-meta-s>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'S'}, nil
		case "<a-s-m-t>", "<alt-shift-meta-t>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'T'}, nil
		case "<a-s-m-u>", "<alt-shift-meta-u>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'U'}, nil
		case "<a-s-m-v>", "<alt-shift-meta-v>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'V'}, nil
		case "<a-s-m-w>", "<alt-shift-meta-w>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'W'}, nil
		case "<a-s-m-x>", "<alt-shift-meta-x>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'X'}, nil
		case "<a-s-m-y>", "<alt-shift-meta-y>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'Y'}, nil
		case "<a-s-m-z>", "<alt-shift-meta-z>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: 'Z'}, nil
		case "<a-s-m-[>", "<alt-shift-meta-[>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '{'}, nil
		case "<a-s-m-\\\\>", "<alt-shift-meta-\\\\>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '|'}, nil
		case "<a-s-m-]>", "<alt-shift-meta-]>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '}'}, nil
		case "<a-s-m-/>", "<alt-shift-meta-/>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '?'}, nil
		case "<a-s-m-_>", "<alt-shift-meta-_>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '_'}, nil
		case "<a-s-m-.>", "<alt-shift-meta-.>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '>'}, nil
		case "<a-s-m-,>", "<alt-shift-meta-,>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '<'}, nil
		case "<a-s-m-;>", "<alt-shift-meta-;>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: ':'}, nil
		case "<a-s-m-'>", "<alt-shift-meta-'>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '"'}, nil
		case "<a-s-m-=>", "<alt-shift-meta-=>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '+'}, nil
		case "<a-s-m-+>", "<alt-shift-meta-+>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '+'}, nil
		case "<a-m-+>", "<alt-meta-+>":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '+'}, nil
		case "<a-s-m-->", "<alt-shift-meta-->":
			return term.KeyComb{Mod: term.ModAltMeta, Ch: '_'}, nil

		// alt+shift
		case "<a-s-f1>", "<alt-shift-f1>":
			return term.KeyComb{Key: term.KeyF1, Mod: term.ModAltShift}, nil
		case "<a-s-f2>", "<alt-shift-f2>":
			return term.KeyComb{Key: term.KeyF2, Mod: term.ModAltShift}, nil
		case "<a-s-f3>", "<alt-shift-f3>":
			return term.KeyComb{Key: term.KeyF3, Mod: term.ModAltShift}, nil
		case "<a-s-f4>", "<alt-shift-f4>":
			return term.KeyComb{Key: term.KeyF4, Mod: term.ModAltShift}, nil
		case "<a-s-f5>", "<alt-shift-f5>":
			return term.KeyComb{Key: term.KeyF5, Mod: term.ModAltShift}, nil
		case "<a-s-f6>", "<alt-shift-f6>":
			return term.KeyComb{Key: term.KeyF6, Mod: term.ModAltShift}, nil
		case "<a-s-f7>", "<alt-shift-f7>":
			return term.KeyComb{Key: term.KeyF7, Mod: term.ModAltShift}, nil
		case "<a-s-f8>", "<alt-shift-f8>":
			return term.KeyComb{Key: term.KeyF8, Mod: term.ModAltShift}, nil
		case "<a-s-f9>", "<alt-shift-f9>":
			return term.KeyComb{Key: term.KeyF9, Mod: term.ModAltShift}, nil
		case "<a-s-f10>", "<alt-shift-f10>":
			return term.KeyComb{Key: term.KeyF10, Mod: term.ModAltShift}, nil
		case "<a-s-f11>", "<alt-shift-f11>":
			return term.KeyComb{Key: term.KeyF11, Mod: term.ModAltShift}, nil
		case "<a-s-f12>", "<alt-shift-f12>":
			return term.KeyComb{Key: term.KeyF12, Mod: term.ModAltShift}, nil
		case "<a-s-insert>", "<alt-shift-insert>":
			return term.KeyComb{Key: term.KeyInsert, Mod: term.ModAltShift}, nil
		case "<a-s-delete>", "<alt-shift-delete>":
			return term.KeyComb{Key: term.KeyDelete, Mod: term.ModAltShift}, nil
		case "<a-s-home>", "<alt-shift-home>":
			return term.KeyComb{Key: term.KeyHome, Mod: term.ModAltShift}, nil
		case "<a-s-end>", "<alt-shift-end>":
			return term.KeyComb{Key: term.KeyEnd, Mod: term.ModAltShift}, nil
		case "<a-s-pgup>", "<alt-shift-pgup>":
			return term.KeyComb{Key: term.KeyPgup, Mod: term.ModAltShift}, nil
		case "<a-s-pgdn>", "<alt-shift-pgdn>":
			return term.KeyComb{Key: term.KeyPgdn, Mod: term.ModAltShift}, nil
		case "<a-s-up>", "<alt-shift-up>":
			return term.KeyComb{Key: term.KeyArrowUp, Mod: term.ModAltShift}, nil
		case "<a-s-down>", "<alt-shift-down>":
			return term.KeyComb{Key: term.KeyArrowDown, Mod: term.ModAltShift}, nil
		case "<a-s-left>", "<alt-shift-left>":
			return term.KeyComb{Key: term.KeyArrowLeft, Mod: term.ModAltShift}, nil
		case "<a-s-right>", "<alt-shift-right>":
			return term.KeyComb{Key: term.KeyArrowRight, Mod: term.ModAltShift}, nil
		case "<a-s-mouse-left>", "<alt-shift-mouse-left>":
			return term.KeyComb{Key: term.MouseLeft, Mod: term.ModAltShift}, nil
		case "<a-s-mouse-middle>", "<alt-shift-mouse-middle>":
			return term.KeyComb{Key: term.MouseMiddle, Mod: term.ModAltShift}, nil
		case "<a-s-mouse-right>", "<alt-shift-mouse-right>":
			return term.KeyComb{Key: term.MouseRight, Mod: term.ModAltShift}, nil
		case "<a-s-mouse-release>", "<alt-shift-mouse-release>":
			return term.KeyComb{Key: term.MouseRelease, Mod: term.ModAltShift}, nil
		case "<a-s-mouse-wheel-up>", "<alt-shift-mouse-wheel-up>":
			return term.KeyComb{Key: term.MouseWheelUp, Mod: term.ModAltShift}, nil
		case "<a-s-mouse-wheel-down>", "<alt-shift-mouse-wheel-down>":
			return term.KeyComb{Key: term.MouseWheelDown, Mod: term.ModAltShift}, nil
		case "<a-s-enter>", "<alt-shift-enter>":
			return term.KeyComb{Key: term.KeyEnter, Mod: term.ModAltShift}, nil
		case "<a-s-space>", "<alt-shift-space>":
			return term.KeyComb{Mod: term.ModAltShift, Key: term.KeySpace}, nil
		case "<a-s-backspace>", "<alt-shift-backspace>":
			return term.KeyComb{Mod: term.ModAltShift, Key: term.KeyBackspace}, nil
		case "<a-s-esc>", "<alt-shift-esc>":
			return term.KeyComb{Key: term.KeyEsc, Mod: term.ModAltShift}, nil
		case "<a-s-1>", "<alt-shift-1>":
			return term.KeyComb{Ch: '!', Mod: term.ModAlt}, nil
		case "<a-s-2>", "<alt-shift-2>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '@'}, nil
		case "<a-s-3>", "<alt-shift-3>":
			return term.KeyComb{Ch: '#', Mod: term.ModAlt}, nil
		case "<a-s-4>", "<alt-shift-4>":
			return term.KeyComb{Ch: '$', Mod: term.ModAlt}, nil
		case "<a-s-5>", "<alt-shift-5>":
			return term.KeyComb{Ch: '%', Mod: term.ModAlt}, nil
		case "<a-s-6>", "<alt-shift-6>":
			return term.KeyComb{Ch: '^', Mod: term.ModAlt}, nil
		case "<a-s-7>", "<alt-shift-7>":
			return term.KeyComb{Ch: '&', Mod: term.ModAlt}, nil
		case "<a-s-8>", "<alt-shift-8>":
			return term.KeyComb{Ch: '*', Mod: term.ModAlt}, nil
		case "<a-s-9>", "<alt-shift-9>":
			return term.KeyComb{Ch: '(', Mod: term.ModAlt}, nil
		case "<a-s-0>", "<alt-shift-0>":
			return term.KeyComb{Ch: ')', Mod: term.ModAlt}, nil
		case "<a-s-`>", "<alt-shift-`>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '~'}, nil
		case "<a-s-a>", "<alt-shift-a>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'A'}, nil
		case "<a-s-b>", "<alt-shift-b>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'B'}, nil
		case "<a-s-c>", "<alt-shift-c>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'C'}, nil
		case "<a-s-d>", "<alt-shift-d>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'D'}, nil
		case "<a-s-e>", "<alt-shift-e>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'E'}, nil
		case "<a-s-f>", "<alt-shift-f>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'F'}, nil
		case "<a-s-g>", "<alt-shift-g>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'G'}, nil
		case "<a-s-h>", "<alt-shift-h>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'H'}, nil
		case "<a-s-tab>", "<alt-shift-tab>":
			return term.KeyComb{Mod: term.ModAltShift, Key: term.KeyTab}, nil
		case "<a-s-i>", "<alt-shift-i>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'I'}, nil
		case "<a-s-j>", "<alt-shift-j>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'J'}, nil
		case "<a-s-k>", "<alt-shift-k>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'K'}, nil
		case "<a-s-l>", "<alt-shift-l>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'L'}, nil
		case "<a-s-m>", "<alt-shift-m>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'M'}, nil
		case "<a-s-n>", "<alt-shift-n>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'N'}, nil
		case "<a-s-o>", "<alt-shift-o>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'O'}, nil
		case "<a-s-p>", "<alt-shift-p>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'P'}, nil
		case "<a-s-q>", "<alt-shift-q>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'Q'}, nil
		case "<a-s-r>", "<alt-shift-r>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'R'}, nil
		case "<a-s-s>", "<alt-shift-s>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'S'}, nil
		case "<a-s-t>", "<alt-shift-t>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'T'}, nil
		case "<a-s-u>", "<alt-shift-u>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'U'}, nil
		case "<a-s-v>", "<alt-shift-v>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'V'}, nil
		case "<a-s-w>", "<alt-shift-w>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'W'}, nil
		case "<a-s-x>", "<alt-shift-x>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'X'}, nil
		case "<a-s-y>", "<alt-shift-y>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'Y'}, nil
		case "<a-s-z>", "<alt-shift-z>":
			return term.KeyComb{Mod: term.ModAlt, Ch: 'Z'}, nil
		case "<a-s-[>", "<alt-shift-[>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '{'}, nil
		case "<a-s-\\\\>", "<alt-shift-\\\\>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '|'}, nil
		case "<a-s-]>", "<alt-shift-]>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '}'}, nil
		case "<a-s-/>", "<alt-shift-/>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '?'}, nil
		case "<a-s-_>", "<alt-shift-_>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '_'}, nil
		case "<a-s-.>", "<alt-shift-.>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '>'}, nil
		case "<a-s-,>", "<alt-shift-,>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '<'}, nil
		case "<a-s-;>", "<alt-shift-;>":
			return term.KeyComb{Mod: term.ModAlt, Ch: ':'}, nil
		case "<a-s-'>", "<alt-shift-'>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '"'}, nil
		case "<a-s-=>", "<alt-shift-=>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '+'}, nil
		case "<a-s-+>", "<alt-shift-+>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '+'}, nil
		case "<a-+>", "<alt-+>":
			return term.KeyComb{Mod: term.ModAlt, Ch: '+'}, nil
		case "<a-s-->", "<alt-shift-->":
			return term.KeyComb{Mod: term.ModAlt, Ch: '_'}, nil

		case "<alt>":
			return term.KeyComb{Mod: term.ModAlt}, nil
		case "<shift>":
			return term.KeyComb{Mod: term.ModShift}, nil
		case "<meta>":
			return term.KeyComb{Mod: term.ModMeta}, nil
		case "<ctrl>":
			return term.KeyComb{Mod: term.ModCtrl}, nil
		case "<ctrl-shift>":
			return term.KeyComb{Mod: term.ModCtrlShift}, nil
		case "<ctrl-alt>":
			return term.KeyComb{Mod: term.ModCtrlAlt}, nil
		case "<ctrl-meta>":
			return term.KeyComb{Mod: term.ModCtrlMeta}, nil
		case "<ctrl-shift-alt>":
			return term.KeyComb{Mod: term.ModCtrlShiftAlt}, nil
		case "<ctrl-shift-meta>":
			return term.KeyComb{Mod: term.ModCtrlShiftMeta}, nil
		case "<ctrl-alt-meta>":
			return term.KeyComb{Mod: term.ModCtrlAltMeta}, nil
		case "<shift-meta>":
			return term.KeyComb{Mod: term.ModShiftMeta}, nil
		case "<alt-meta>":
			return term.KeyComb{Mod: term.ModAltMeta}, nil
		case "<alt-shift-meta>":
			return term.KeyComb{Mod: term.ModAltShiftMeta}, nil
		case "<alt-shift>":
			return term.KeyComb{Mod: term.ModAltShift}, nil

		default:
			return term.KeyComb{}, fmt.Errorf("invalid key: '%s'", str)
		}
	}
}
