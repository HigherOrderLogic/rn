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

// TODO rename to commandListHandler
type commandHandler struct {
	commandEvent term.Event
	overlayCfg   text.CommandOverlayConfig
	callback     func(string, string) bool

	component.Responsive
	cell.Buffer
	search.List
	search.History

	// argsStartIdx is the position of the first space
	// that separates the 'command' from its args
	argsStartIdx int
}

func newCommandHandler(
	b browser.Storage, max int, overlayCfg text.CommandOverlayConfig,
	commandEvent term.Event, callback func(string, string) bool,
) *commandHandler {
	ret := new(commandHandler)
	ret.init(b, max, overlayCfg, commandEvent, callback)
	return ret
}

func (h *commandHandler) loadHistory() error {
	return h.History.Load()
}

func (h *commandHandler) init(
	store browser.Storage, max int, overlayCfg text.CommandOverlayConfig,
	commandEvent term.Event, callback func(string, string) bool,
) {
	h.commandEvent = commandEvent
	h.overlayCfg = overlayCfg
	h.callback = callback

	h.History.Init(store, commandHistoryDocumentID, max)

	// overlay buffer over the search list so we can
	// stop the search for multiple argument commands
	// but we can display arguments
	h.Buffer.Init()
	responsive := component.BufferResponsive(&h.Buffer,
		component.StringConfig{})
	h.Responsive = responsive

	cfg := search.ListConfig{
		Algo:             search.FuzzyMatch,
		Interrupt:        term.Interrupt,
		CaseSensitive:    false,
		MatchedTextAttr:  &overlayCfg.MatchedTextAttr,
		CountAttr:        &overlayCfg.CountAttr,
		FocusElementAttr: &overlayCfg.FocusElementAttr,
		ElementAttr:      &overlayCfg.ElementAttr,
	}

	h.List.Init(cfg)
}

func (h *commandHandler) commandOverlayDimensions() (width, height int) {
	width, height = h.overlayCfg.Width, h.overlayCfg.Height
	if h.overlayCfg.Frame && width > 2 && height > 2 {
		width -= 2
		height -= 2
	}
	return
}

func (h *commandHandler) resizeCommandOverlay() {
	// propagate local cmd+args buffer height to
	// search list, which only has cmd, in case args alone span
	// multiple lines
	cmdWidth, cmdHeight := h.commandOverlayDimensions()
	height := h.Responsive.Height(cmdWidth)
	// set to min 1, as it's being used as input field
	// and max to the height of the overlayed component
	height = int(math.Min(math.Max(1, float64(height)), float64(cmdHeight)))
	// the list is very short so waiting is not a significant
	// perf penalty and it makes tests easier to make deterministic
	h.List.Wait()
	h.List.SetMinInputHeight(height)
	h.Responsive.Resize(cmdWidth, height)
}

func (h *commandHandler) Resize(width, height int) {
	h.List.Resize(width, height)
}

func (h *commandHandler) Draw(w term.Writer) {
	// resize on every draw because search.List uses a responsive
	// input so local buffer changes must consider potential resize
	// of search.List
	h.resizeCommandOverlay()
	h.List.Draw(w)
	h.Responsive.Draw(w)
}

func (h *commandHandler) writeLastCommandQuery() {
	cmd := h.History.Next()
	if cmd == "" {
		return
	}
	h.Buffer.Reset()
	h.Buffer.WriteString(cmd)

	listCmdArgs := strings.Split(cmd, " ")
	listCmd := listCmdArgs[0]
	if len(listCmd) > 0 {
		h.argsStartIdx = len(listCmd)
	} else {
		h.argsStartIdx = 0
	}
	h.List.Buffer().Reset()
	h.List.Buffer().WriteString(listCmd)
	h.List.Wait()
}

func (h *commandHandler) Handle(ev term.Event) (quit, handled bool) {
	handled = true

	if ev == h.commandEvent {
		h.writeLastCommandQuery()
		return
	}

	switch ev.Key {
	case term.KeyEnter:
		// before selecting command wait for previous search to finish
		h.List.Wait()

		command, _ := h.List.Focus()
		bufStr := h.Buffer.String()
		quit := h.callback(string(command), bufStr)
		if bufStr != "" {
			_ = h.History.Add(bufStr)
		}
		if quit {
			// hack to signal exit process
			return true, false
		}
		return true, true
	case term.KeyEsc:
		return true, true
	case term.KeyArrowDown:
		h.List.FocusDown()
	case term.KeyArrowUp:
		h.List.FocusUp()
	case term.KeySpace:
		ev.Ch = ' '
		handled = false
	case term.KeyBackspace, term.KeyBackspace2:
		cols := h.Buffer.Columns(0)
		if cols == 0 {
			return true, true
		}
		h.Buffer.DeleteCell(term.Coordinates{X: cols - 1})
		if h.argsStartIdx == h.Buffer.Size() ||
			h.argsStartIdx == 0 {
			h.List.Buffer().Reset()
			h.List.Buffer().WriteString(h.Buffer.String())
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
		h.argsStartIdx = h.Buffer.Size()
	}
	h.Buffer.WriteString(string(ev.Ch))

	if h.argsStartIdx == 0 {
		h.List.Buffer().WriteString(string(ev.Ch))
	}
	return
}

func (h *commandHandler) DataReset(commands []string) {
	h.List.DataReset()
	for _, cmd := range commands {
		h.List.PushSync([]byte(cmd))
	}
}

func (h *commandHandler) Reset() {
	h.Buffer.Reset()
	h.List.Buffer().Reset()
	h.List.Wait()
	h.argsStartIdx = 0
}

func (h *commandHandler) Cursor() (term.Coordinates, bool) {
	var pos term.Coordinates
	cmdWidth, cmdHeight := h.commandOverlayDimensions()
	x := len(h.Buffer.String()) % cmdWidth
	y := len(h.Buffer.String()) / cmdWidth
	if y >= cmdHeight {
		pos.X += cmdWidth - 1
		pos.Y += cmdHeight - 1
	} else {
		pos.X += x
		pos.Y += y
	}
	return pos, true
}

func (h *commandHandler) Man() tui.Manual {
	panic("TODO")
}

func (h *commandHandler) Close() error {
	return h.List.Close()
}
