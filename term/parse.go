package term

import (
	"errors"
	"fmt"
	"strings"
)

// ParseKey parses str into a KeyComb or returns
// error if it fails to parse it. Note that this function
// is not case sensitive.
func ParseKey(str string) (KeyComb, error) {
	str = strings.TrimSpace(strings.ToLower(str))
	switch len(str) {
	case 0:
		return KeyComb{}, errors.New("invalid empty input")
	case 1:
		return KeyComb{Ch: []rune(str)[0]}, nil
	default:
		switch str {
		case "<f1>":
			return KeyComb{Key: KeyF1}, nil
		case "<f2>":
			return KeyComb{Key: KeyF2}, nil
		case "<f3>":
			return KeyComb{Key: KeyF3}, nil
		case "<f4>":
			return KeyComb{Key: KeyF4}, nil
		case "<f5>":
			return KeyComb{Key: KeyF5}, nil
		case "<f6>":
			return KeyComb{Key: KeyF6}, nil
		case "<f7>":
			return KeyComb{Key: KeyF7}, nil
		case "<f8>":
			return KeyComb{Key: KeyF8}, nil
		case "<f9>":
			return KeyComb{Key: KeyF9}, nil
		case "<f10>":
			return KeyComb{Key: KeyF10}, nil
		case "<f11>":
			return KeyComb{Key: KeyF11}, nil
		case "<f12>":
			return KeyComb{Key: KeyF12}, nil
		case "<insert>":
			return KeyComb{Key: KeyInsert}, nil
		case "<delete>":
			return KeyComb{Key: KeyDelete}, nil
		case "<home>":
			return KeyComb{Key: KeyHome}, nil
		case "<end>":
			return KeyComb{Key: KeyEnd}, nil
		case "<pgup>":
			return KeyComb{Key: KeyPgup}, nil
		case "<pgdn>":
			return KeyComb{Key: KeyPgdn}, nil
		case "<up>":
			return KeyComb{Key: KeyArrowUp}, nil
		case "<down>":
			return KeyComb{Key: KeyArrowDown}, nil
		case "<left>":
			return KeyComb{Key: KeyArrowLeft}, nil
		case "<right>":
			return KeyComb{Key: KeyArrowRight}, nil
		case "<mouse-left>":
			return KeyComb{Key: MouseLeft}, nil
		case "<mouse-middle>":
			return KeyComb{Key: MouseMiddle}, nil
		case "<mouse-right>":
			return KeyComb{Key: MouseRight}, nil
		case "<mouse-release>":
			return KeyComb{Key: MouseRelease}, nil
		case "<mouse-wheel-up>":
			return KeyComb{Key: MouseWheelUp}, nil
		case "<mouse-wheel-down>":
			return KeyComb{Key: MouseWheelDown}, nil
		case "<c-`>", "<ctrl-`>":
			return KeyComb{Key: KeyCtrlTilde}, nil
		case "<c-2>", "<ctrl-2>":
			return KeyComb{Key: KeyCtrl2}, nil
		case "<c-space>", "<ctrl-space>":
			return KeyComb{Key: KeyCtrlSpace}, nil
		case "<c-a>", "<ctrl-a>":
			return KeyComb{Key: KeyCtrlA}, nil
		case "<c-b>", "<ctrl-b>":
			return KeyComb{Key: KeyCtrlB}, nil
		case "<c-c>", "<ctrl-c>":
			return KeyComb{Key: KeyCtrlC}, nil
		case "<c-d>", "<ctrl-d>":
			return KeyComb{Key: KeyCtrlD}, nil
		case "<c-e>", "<ctrl-e>":
			return KeyComb{Key: KeyCtrlE}, nil
		case "<c-f>", "<ctrl-f>":
			return KeyComb{Key: KeyCtrlF}, nil
		case "<c-g>", "<ctrl-g>":
			return KeyComb{Key: KeyCtrlG}, nil
		case "<c-backspace>", "<ctrl-backspace>":
			return KeyComb{Key: KeyBackspace}, nil
		case "<c-h>", "<ctrl-h>":
			return KeyComb{Key: KeyCtrlH}, nil
		case "<c-tab>", "<ctrl-tab>":
			return KeyComb{Key: KeyTab}, nil
		case "<c-i>", "<ctrl-i>":
			return KeyComb{Key: KeyCtrlI}, nil
		case "<c-j>", "<ctrl-j>":
			return KeyComb{Key: KeyCtrlJ}, nil
		case "<c-k>", "<ctrl-k>":
			return KeyComb{Key: KeyCtrlK}, nil
		case "<c-l>", "<ctrl-l>":
			return KeyComb{Key: KeyCtrlL}, nil
		case "<enter>":
			return KeyComb{Key: KeyEnter}, nil
		case "<c-m>", "<ctrl-m>":
			return KeyComb{Key: KeyCtrlM}, nil
		case "<c-n>", "<ctrl-n>":
			return KeyComb{Key: KeyCtrlN}, nil
		case "<c-o>", "<ctrl-o>":
			return KeyComb{Key: KeyCtrlO}, nil
		case "<c-p>", "<ctrl-p>":
			return KeyComb{Key: KeyCtrlP}, nil
		case "<c-q>", "<ctrl-q>":
			return KeyComb{Key: KeyCtrlQ}, nil
		case "<c-r>", "<ctrl-r>":
			return KeyComb{Key: KeyCtrlR}, nil
		case "<c-s>", "<ctrl-s>":
			return KeyComb{Key: KeyCtrlS}, nil
		case "<c-t>", "<ctrl-t>":
			return KeyComb{Key: KeyCtrlT}, nil
		case "<c-u>", "<ctrl-u>":
			return KeyComb{Key: KeyCtrlU}, nil
		case "<c-v>", "<ctrl-v>":
			return KeyComb{Key: KeyCtrlV}, nil
		case "<c-w>", "<ctrl-w>":
			return KeyComb{Key: KeyCtrlW}, nil
		case "<c-x>", "<ctrl-x>":
			return KeyComb{Key: KeyCtrlX}, nil
		case "<c-y>", "<ctrl-y>":
			return KeyComb{Key: KeyCtrlY}, nil
		case "<c-z>", "<ctrl-z>":
			return KeyComb{Key: KeyCtrlZ}, nil
		case "<esc>":
			return KeyComb{Key: KeyEsc}, nil
		case "<c-[>", "<ctrl-[>":
			return KeyComb{Key: KeyCtrlLsqBracket}, nil
		case "<c-3>", "<ctrl-3>":
			return KeyComb{Key: KeyCtrl3}, nil
		case "<c-4>", "<ctrl-4>":
			return KeyComb{Key: KeyCtrl4}, nil
		case "<c-\\>", "<ctrl-\\>":
			return KeyComb{Key: KeyCtrlBackslash}, nil
		case "<c-5>", "<ctrl-5>":
			return KeyComb{Key: KeyCtrl5}, nil
		case "<c-]>", "<ctrl-]>":
			return KeyComb{Key: KeyCtrlRsqBracket}, nil
		case "<c-6>", "<ctrl-6>":
			return KeyComb{Key: KeyCtrl6}, nil
		case "<c-7>", "<ctrl-7>":
			return KeyComb{Key: KeyCtrl7}, nil
		case "<c-/>", "<ctrl-/>":
			return KeyComb{Key: KeyCtrlSlash}, nil
		case "<c-_>", "<ctrl-_>":
			return KeyComb{Key: KeyCtrlUnderscore}, nil
		case "<space>":
			return KeyComb{Key: KeySpace}, nil
		case "<backspace>":
			return KeyComb{Key: KeyBackspace2}, nil
		case "<c-8>", "<ctrl-8>":
			return KeyComb{Key: KeyCtrl8}, nil
		case "<m-f1>":
			return KeyComb{Key: KeyF1, Mod: ModAlt}, nil
		case "<m-f2>":
			return KeyComb{Key: KeyF2, Mod: ModAlt}, nil
		case "<m-f3>":
			return KeyComb{Key: KeyF3, Mod: ModAlt}, nil
		case "<m-f4>":
			return KeyComb{Key: KeyF4, Mod: ModAlt}, nil
		case "<m-f5>":
			return KeyComb{Key: KeyF5, Mod: ModAlt}, nil
		case "<m-f6>":
			return KeyComb{Key: KeyF6, Mod: ModAlt}, nil
		case "<m-f7>":
			return KeyComb{Key: KeyF7, Mod: ModAlt}, nil
		case "<m-f8>":
			return KeyComb{Key: KeyF8, Mod: ModAlt}, nil
		case "<m-f9>":
			return KeyComb{Key: KeyF9, Mod: ModAlt}, nil
		case "<m-f10>":
			return KeyComb{Key: KeyF10, Mod: ModAlt}, nil
		case "<m-f11>":
			return KeyComb{Key: KeyF11, Mod: ModAlt}, nil
		case "<m-f12>":
			return KeyComb{Key: KeyF12, Mod: ModAlt}, nil
		case "<m-insert>":
			return KeyComb{Key: KeyInsert, Mod: ModAlt}, nil
		case "<m-delete>":
			return KeyComb{Key: KeyDelete, Mod: ModAlt}, nil
		case "<m-home>":
			return KeyComb{Key: KeyHome, Mod: ModAlt}, nil
		case "<m-end>":
			return KeyComb{Key: KeyEnd, Mod: ModAlt}, nil
		case "<m-pgup>":
			return KeyComb{Key: KeyPgup, Mod: ModAlt}, nil
		case "<m-pgdn>":
			return KeyComb{Key: KeyPgdn, Mod: ModAlt}, nil
		case "<m-up>":
			return KeyComb{Key: KeyArrowUp, Mod: ModAlt}, nil
		case "<m-down>":
			return KeyComb{Key: KeyArrowDown, Mod: ModAlt}, nil
		case "<m-left>":
			return KeyComb{Key: KeyArrowLeft, Mod: ModAlt}, nil
		case "<m-right>":
			return KeyComb{Key: KeyArrowRight, Mod: ModAlt}, nil
		case "<m-mouse-left>":
			return KeyComb{Key: MouseLeft, Mod: ModAlt}, nil
		case "<m-mouse-middle>":
			return KeyComb{Key: MouseMiddle, Mod: ModAlt}, nil
		case "<m-mouse-right>":
			return KeyComb{Key: MouseRight, Mod: ModAlt}, nil
		case "<m-mouse-release>":
			return KeyComb{Key: MouseRelease, Mod: ModAlt}, nil
		case "<m-mouse-wheel-up>":
			return KeyComb{Key: MouseWheelUp, Mod: ModAlt}, nil
		case "<m-mouse-wheel-down>":
			return KeyComb{Key: MouseWheelDown, Mod: ModAlt}, nil
		case "<m-c-`>", "<c-m-`>", "<mod-ctrl-`>", "<ctrl-mod-`>":
			return KeyComb{Key: KeyCtrlTilde, Mod: ModAlt}, nil
		case "<m-c-2>", "<c-m-2>", "<mod-ctrl-2>", "<ctrl-mod-2>":
			return KeyComb{Key: KeyCtrl2, Mod: ModAlt}, nil
		case "<m-c-space>", "<c-m-space>", "<mod-ctrl-space>", "<ctrl-mod-space>":
			return KeyComb{Key: KeyCtrlSpace, Mod: ModAlt}, nil
		case "<m-c-a>", "<c-m-a>", "<mod-ctrl-a>", "<ctrl-mod-a>":
			return KeyComb{Key: KeyCtrlA, Mod: ModAlt}, nil
		case "<m-c-b>", "<c-m-b>", "<mod-ctrl-b>", "<ctrl-mod-b>":
			return KeyComb{Key: KeyCtrlB, Mod: ModAlt}, nil
		case "<m-c-c>", "<c-m-c>", "<mod-ctrl-c>", "<ctrl-mod-c>":
			return KeyComb{Key: KeyCtrlC, Mod: ModAlt}, nil
		case "<m-c-d>", "<c-m-d>", "<mod-ctrl-d>", "<ctrl-mod-d>":
			return KeyComb{Key: KeyCtrlD, Mod: ModAlt}, nil
		case "<m-c-e>", "<c-m-e>", "<mod-ctrl-e>", "<ctrl-mod-e>":
			return KeyComb{Key: KeyCtrlE, Mod: ModAlt}, nil
		case "<m-c-f>", "<c-m-f>", "<mod-ctrl-f>", "<ctrl-mod-f>":
			return KeyComb{Key: KeyCtrlF, Mod: ModAlt}, nil
		case "<m-c-g>", "<c-m-g>", "<mod-ctrl-g>", "<ctrl-mod-g>":
			return KeyComb{Key: KeyCtrlG, Mod: ModAlt}, nil
		case "<m-c-backspace>", "<c-m-backspace>", "<mod-ctrl-backspace>", "<ctrl-mod-backspace>":
			return KeyComb{Key: KeyBackspace, Mod: ModAlt}, nil
		case "<m-c-h>", "<c-m-h>", "<mod-ctrl-h>", "<ctrl-mod-h>":
			return KeyComb{Key: KeyCtrlH, Mod: ModAlt}, nil
		case "<m-c-tab>", "<c-m-tab>", "<mod-ctrl-tab>", "<ctrl-mod-tab>":
			return KeyComb{Key: KeyTab, Mod: ModAlt}, nil
		case "<m-c-i>", "<c-m-i>", "<mod-ctrl-i>", "<ctrl-mod-i>":
			return KeyComb{Key: KeyCtrlI, Mod: ModAlt}, nil
		case "<m-c-j>", "<c-m-j>", "<mod-ctrl-j>", "<ctrl-mod-j>":
			return KeyComb{Key: KeyCtrlJ, Mod: ModAlt}, nil
		case "<m-c-k>", "<c-m-k>", "<mod-ctrl-k>", "<ctrl-mod-k>":
			return KeyComb{Key: KeyCtrlK, Mod: ModAlt}, nil
		case "<m-c-l>", "<c-m-l>", "<mod-ctrl-l>", "<ctrl-mod-l>":
			return KeyComb{Key: KeyCtrlL, Mod: ModAlt}, nil
		case "<m-enter>":
			return KeyComb{Key: KeyEnter, Mod: ModAlt}, nil
		case "<m-c-m>", "<c-m-m>", "<mod-ctrl-m>", "<ctrl-mod-m>":
			return KeyComb{Key: KeyCtrlM, Mod: ModAlt}, nil
		case "<m-c-n>", "<c-m-n>", "<mod-ctrl-n>", "<ctrl-mod-n>":
			return KeyComb{Key: KeyCtrlN, Mod: ModAlt}, nil
		case "<m-c-o>", "<c-m-o>", "<mod-ctrl-o>", "<ctrl-mod-o>":
			return KeyComb{Key: KeyCtrlO, Mod: ModAlt}, nil
		case "<m-c-p>", "<c-m-p>", "<mod-ctrl-p>", "<ctrl-mod-p>":
			return KeyComb{Key: KeyCtrlP, Mod: ModAlt}, nil
		case "<m-c-q>", "<c-m-q>", "<mod-ctrl-q>", "<ctrl-mod-q>":
			return KeyComb{Key: KeyCtrlQ, Mod: ModAlt}, nil
		case "<m-c-r>", "<c-m-r>", "<mod-ctrl-r>", "<ctrl-mod-r>":
			return KeyComb{Key: KeyCtrlR, Mod: ModAlt}, nil
		case "<m-c-s>", "<c-m-s>", "<mod-ctrl-s>", "<ctrl-mod-s>":
			return KeyComb{Key: KeyCtrlS, Mod: ModAlt}, nil
		case "<m-c-t>", "<c-m-t>", "<mod-ctrl-t>", "<ctrl-mod-t>":
			return KeyComb{Key: KeyCtrlT, Mod: ModAlt}, nil
		case "<m-c-u>", "<c-m-u>", "<mod-ctrl-u>", "<ctrl-mod-u>":
			return KeyComb{Key: KeyCtrlU, Mod: ModAlt}, nil
		case "<m-c-v>", "<c-m-v>", "<mod-ctrl-v>", "<ctrl-mod-v>":
			return KeyComb{Key: KeyCtrlV, Mod: ModAlt}, nil
		case "<m-c-w>", "<c-m-w>", "<mod-ctrl-w>", "<ctrl-mod-w>":
			return KeyComb{Key: KeyCtrlW, Mod: ModAlt}, nil
		case "<m-c-x>", "<c-m-x>", "<mod-ctrl-x>", "<ctrl-mod-x>":
			return KeyComb{Key: KeyCtrlX, Mod: ModAlt}, nil
		case "<m-c-y>", "<c-m-y>", "<mod-ctrl-y>", "<ctrl-mod-y>":
			return KeyComb{Key: KeyCtrlY, Mod: ModAlt}, nil
		case "<m-c-z>", "<c-m-z>", "<mod-ctrl-z>", "<ctrl-mod-z>":
			return KeyComb{Key: KeyCtrlZ, Mod: ModAlt}, nil
		case "<m-esc>":
			return KeyComb{Key: KeyEsc, Mod: ModAlt}, nil
		case "<m-c-[>", "<c-m-[>", "<mod-ctrl-[>", "<ctrl-mod-[>":
			return KeyComb{Key: KeyCtrlLsqBracket, Mod: ModAlt}, nil
		case "<m-c-3>", "<c-m-3>", "<mod-ctrl-3>", "<ctrl-mod-3>":
			return KeyComb{Key: KeyCtrl3, Mod: ModAlt}, nil
		case "<m-c-4>", "<c-m-4>", "<mod-ctrl-4>", "<ctrl-mod-4>":
			return KeyComb{Key: KeyCtrl4, Mod: ModAlt}, nil
		case "<m-c-\\>", "<c-m-\\>", "<mod-ctrl-\\>", "<ctrl-mod-\\>":
			return KeyComb{Key: KeyCtrlBackslash, Mod: ModAlt}, nil
		case "<m-c-5>", "<c-m-5>", "<mod-ctrl-5>", "<ctrl-mod-5>":
			return KeyComb{Key: KeyCtrl5, Mod: ModAlt}, nil
		case "<m-c-]>", "<c-m-]>", "<mod-ctrl-]>", "<ctrl-mod-]>":
			return KeyComb{Key: KeyCtrlRsqBracket, Mod: ModAlt}, nil
		case "<m-c-6>", "<c-m-6>", "<mod-ctrl-6>", "<ctrl-mod-6>":
			return KeyComb{Key: KeyCtrl6, Mod: ModAlt}, nil
		case "<m-c-7>", "<c-m-7>", "<mod-ctrl-7>", "<ctrl-mod-7>":
			return KeyComb{Key: KeyCtrl7, Mod: ModAlt}, nil
		case "<m-c-/>", "<c-m-/>", "<mod-ctrl-/>", "<ctrl-mod-/>":
			return KeyComb{Key: KeyCtrlSlash, Mod: ModAlt}, nil
		case "<m-c-_>", "<c-m-_>", "<mod-ctrl-_>", "<ctrl-mod-_>":
			return KeyComb{Key: KeyCtrlUnderscore, Mod: ModAlt}, nil
		case "<m-space>":
			return KeyComb{Key: KeySpace, Mod: ModAlt}, nil
		case "<m-backspace>":
			return KeyComb{Key: KeyBackspace2, Mod: ModAlt}, nil
		case "<m-c-8>", "<c-m-8>", "<mod-ctrl-8>", "<ctrl-mod-8>":
			return KeyComb{Key: KeyCtrl8, Mod: ModAlt}, nil
		default:
			return KeyComb{}, fmt.Errorf("invalid key: '%s'", str)
		}
	}
}
