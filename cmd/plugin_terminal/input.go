package main

import (
	"fmt"

	termutil "github.com/ernestrc/go-tui/cmd/plugin_terminal/util"
	"github.com/ernestrc/go-tui/term"
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
func mapKeyToEscapeSequence(buffer *termutil.Buffer, ev term.Event) ([]byte, bool) {
	switch ev.Key {
	case term.KeyArrowUp:
		if buffer.IsApplicationCursorKeysModeEnabled() {
			return []byte{0x1b, 'O', 'A'}, true
		}
		return []byte(fmt.Sprintf("\x1b[%sA", getModifierStr(ev))), true
	case term.KeyArrowDown:
		if buffer.IsApplicationCursorKeysModeEnabled() {
			return []byte{0x1b, 'O', 'B'}, true
		}
		return []byte(fmt.Sprintf("\x1b[%sB", getModifierStr(ev))), true
	case term.KeyArrowRight:
		if buffer.IsApplicationCursorKeysModeEnabled() {
			return []byte{0x1b, 'O', 'C'}, true
		}
		return []byte(fmt.Sprintf("\x1b[%sC", getModifierStr(ev))), true
	case term.KeyArrowLeft:
		if buffer.IsApplicationCursorKeysModeEnabled() {
			return []byte{0x1b, 'O', 'D'}, true
		}
		return []byte(fmt.Sprintf("\x1b[%sD", getModifierStr(ev))), true
	case term.KeyEnter:
		if buffer.IsNewLineMode() {
			return []byte{0x0d, 0x0a}, true
		}
		return []byte{0x0d}, true
	case term.KeyTab:
		return []byte{0x09}, true
	case term.KeyEsc:
		buffer.ClearSelection()
		buffer.ClearHighlight()
		return []byte{0x1b}, true
	case term.KeyBackspace, term.KeyBackspace2:
		if ev.Mod == term.ModAlt {
			return []byte{0x17}, true
		} else {
			return []byte{0x7f}, true
		}
	case term.KeyF1:
		return []byte("\x1bOP"), true
	case term.KeyF2:
		return []byte("\x1bOQ"), true
	case term.KeyF3:
		return []byte("\x1bOR"), true
	case term.KeyF4:
		return []byte("\x1bOS"), true
	case term.KeyF5:
		return []byte("\x1b[15~"), true
	case term.KeyF6:
		return []byte("\x1b[17~"), true
	case term.KeyF7:
		return []byte("\x1b[18~"), true
	case term.KeyF8:
		return []byte("\x1b[19~"), true
	case term.KeyF9:
		return []byte("\x1b[20~"), true
	case term.KeyF10:
		return []byte("\x1b[21~"), true
	case term.KeyF11:
		return []byte("\x1b[22~"), true
	case term.KeyF12:
		return []byte("\x1b[23~"), true
	case term.KeyInsert:
		return []byte("\x1b[2~"), true
	case term.KeyDelete:
		return []byte("\x1b[3~"), true
	case term.KeyHome:
		if buffer.IsApplicationCursorKeysModeEnabled() {
			return []byte(fmt.Sprintf("\x1b[1%s~", getModifierStr(ev))), true
		}
		return []byte("\x1b[H"), true
	case term.KeyEnd:
		return []byte(fmt.Sprintf("\x1b[4%s~", getModifierStr(ev))), true
	case term.KeyPgup:
		return []byte(fmt.Sprintf("\x1b[5%s~", getModifierStr(ev))), true
	case term.KeyPgdn:
		return []byte(fmt.Sprintf("\x1b[6%s~", getModifierStr(ev))), true
	default:
		return nil, false
	}
}
