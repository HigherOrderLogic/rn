package main

import (
	"math"
	"strings"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/handler/search"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
)

type commandListHandler struct {
	commandKey term.KeyComb
	overlayCfg text.CommandOverlayConfig
	callback   func(string, string) bool

	responsive component.Responsive
	buf        cell.Buffer
	list       search.List
	history    search.History

	// argsStartIdx is the position of the first space
	// that separates the 'command' from its args
	argsStartIdx int

	// used to signal across Handle calls that user
	// is cyclying through commands, in particular
	// when command key is a character that could be interpreted
	// as a character to be inserted in command input buffer
	prevCommandCycle bool
}

func newCommandListHandler(
	b browser.Storage, max int, overlayCfg text.CommandOverlayConfig,
	commandKey term.KeyComb, callback func(string, string) bool,
) *commandListHandler {
	ret := new(commandListHandler)
	ret.init(b, max, overlayCfg, commandKey, callback)
	return ret
}

func (h *commandListHandler) loadHistory() error {
	return h.history.Load()
}

func (h *commandListHandler) init(
	store browser.Storage, max int, overlayCfg text.CommandOverlayConfig,
	commandKey term.KeyComb, callback func(string, string) bool,
) {
	h.commandKey = commandKey
	h.overlayCfg = overlayCfg
	h.callback = callback

	h.history.Init(store, commandHistoryDocumentID, max)

	// overlay buffer over the search list so we can
	// stop the search for multiple argument commands
	// but we can display arguments
	h.buf.Init()
	h.responsive = component.BufferResponsive(&h.buf,
		component.StringConfig{})

	cfg := search.ListConfig{
		Algo:             search.FuzzyMatch,
		Interrupt:        term.Interrupt,
		CaseSensitive:    false,
		MatchedTextAttr:  &overlayCfg.MatchedTextAttr,
		CountAttr:        &overlayCfg.CountAttr,
		FocusElementAttr: &overlayCfg.FocusElementAttr,
		ElementAttr:      &overlayCfg.ElementAttr,
	}

	h.list.Init(cfg)
}

func (h *commandListHandler) commandOverlayDimensions() (width, height int) {
	width, height = h.overlayCfg.Width, h.overlayCfg.Height
	if h.overlayCfg.Frame && width > 2 && height > 2 {
		width -= 2
		height -= 2
	}
	return
}

func (h *commandListHandler) resizeCommandOverlay() {
	// propagate local cmd+args buffer height to
	// search list, which only has cmd, in case args alone span
	// multiple lines
	cmdWidth, cmdHeight := h.commandOverlayDimensions()
	height := h.responsive.Height(cmdWidth)
	// set to min 1, as it's being used as input field
	// and max to the height of the overlayed component
	height = int(math.Min(math.Max(1, float64(height)), float64(cmdHeight)))
	// the list is very short so waiting is not a significant
	// perf penalty and it makes tests easier to make deterministic
	h.list.Wait()
	h.list.SetMinInputHeight(height)
	h.responsive.Resize(cmdWidth, height)
}

func (h *commandListHandler) Resize(width, height int) {
	h.list.Resize(width, height)
}

func (h *commandListHandler) Draw(w term.Writer) {
	// resize on every draw because search.List uses a responsive
	// input so local buffer changes must consider potential resize
	// of search.List
	h.resizeCommandOverlay()
	h.list.Draw(w)
	h.responsive.Draw(w)
}

func (h *commandListHandler) writeLastCommandQuery() {
	cmd := h.history.Next()
	if cmd == "" {
		return
	}
	h.buf.Reset()
	h.buf.WriteString(cmd)

	listCmdArgs := strings.Split(cmd, " ")
	listCmd := listCmdArgs[0]
	if len(listCmd) > 0 {
		h.argsStartIdx = len(listCmd)
	} else {
		h.argsStartIdx = 0
	}
	h.list.Buffer().Reset()
	h.list.Buffer().WriteString(listCmd)
	h.list.Wait()
}

func (h *commandListHandler) Handle(ev term.Event) (quit, handled bool) {
	key := ev.KeyComb()
	if (key == h.commandKey && h.commandKey.Ch == 0) ||
		(key == h.commandKey && h.buf.Columns(0) == 0) ||
		(key == h.commandKey && h.prevCommandCycle) {
		h.writeLastCommandQuery()
		h.prevCommandCycle = true
		return false, true
	}

	h.prevCommandCycle = false
	handled = true
	switch ev.Key {
	case term.KeyEnter:
		// before selecting command wait for previous search to finish
		h.list.Wait()

		command, _ := h.list.Focus()
		bufStr := h.buf.String()
		quit := h.callback(string(command), bufStr)
		if bufStr != "" {
			_ = h.history.Add(bufStr)
		}
		if quit {
			// hack to signal exit process
			return true, false
		}
		return true, true
	case term.KeyEsc:
		return true, true
	case term.KeyArrowDown, term.KeyCtrlJ:
		h.list.FocusDown()
	case term.KeyArrowUp, term.KeyCtrlK:
		h.list.FocusUp()
	case term.KeySpace:
		ev.Ch = ' '
		handled = false
	case term.KeyBackspace, term.KeyBackspace2:
		cols := h.buf.Columns(0)
		if cols == 0 {
			return true, true
		}
		h.buf.DeleteCell(term.Coordinates{X: cols - 1})
		if h.argsStartIdx == h.buf.Size() ||
			h.argsStartIdx == 0 {
			h.list.Buffer().Reset()
			h.list.Buffer().WriteString(h.buf.String())
			h.argsStartIdx = 0
		}
	default:
		handled = false
	}

	if handled {
		return
	}

	if ev.Ch == 0 {
		return
	}

	handled = true
	if ev.Ch == ' ' && h.argsStartIdx == 0 {
		h.argsStartIdx = h.buf.Size()
	}
	h.buf.WriteString(string(ev.Ch))

	if h.argsStartIdx == 0 {
		h.list.Buffer().WriteString(string(ev.Ch))
	}
	return
}

func (h *commandListHandler) dataReset(commands []string) {
	h.list.DataReset()
	for _, cmd := range commands {
		h.list.PushSync([]byte(cmd))
	}
}

func (h *commandListHandler) reset() {
	h.buf.Reset()
	h.list.Buffer().Reset()
	h.list.Wait()
	h.argsStartIdx = 0
}

func (h *commandListHandler) Cursor() (term.Coordinates, bool) {
	var pos term.Coordinates
	cmdWidth, cmdHeight := h.commandOverlayDimensions()
	x := len(h.buf.String()) % cmdWidth
	y := len(h.buf.String()) / cmdWidth
	if y >= cmdHeight {
		pos.X += cmdWidth - 1
		pos.Y += cmdHeight - 1
	} else {
		pos.X += x
		pos.Y += y
	}
	return pos, true
}

func (h *commandListHandler) Man() tui.Manual {
	panic("TODO")
}

func (h *commandListHandler) Close() error {
	return h.list.Close()
}
