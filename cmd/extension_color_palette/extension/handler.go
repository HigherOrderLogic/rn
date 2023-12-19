package extension

import (
	"fmt"
	"strconv"

	"unstable.build/go-tui"
	browserapi "unstable.build/go-tui/api/browser"
	"unstable.build/go-tui/api/config"
	textapi "unstable.build/go-tui/api/text"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/extension"
	extutil "unstable.build/go-tui/extension/util"
	"unstable.build/go-tui/proto"
	"unstable.build/go-tui/term"
)

var colorPaletteCmd = textapi.CommandManual{
	Name: "colorPalette",
	Summary: "Opens a new window and displays all the color codes available " +
		"to customize the UI via configuration.",
}

// Greantee returns this extension's extension.Grantee, and it required permissions.
func Grantee() (extension.Grantee, []extension.Permission) {
	return extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationRight,
		Handler: func(_ textapi.Command, grants []extension.Grant, broker proto.MuxBroker,
			invokeWindow browserapi.Window, config config.Config) (browserapi.Handler, error) {
			return new(colorPaletteHandler), nil
		},
		Command: colorPaletteCmd,
	})
}

type colorPaletteHandler struct {
	width, height int
	grid          tui.Component
	dim           bool
	dirty         bool
}

func (h *colorPaletteHandler) Resize(width, height int) {
	if h.grid != nil {
		h.grid.Resize(width, height)
	}
	h.width = width
	h.height = height
}

func makeColorGrid(dim bool) tui.Component {
	ret := make([][]tui.Component, 16)
	var nameNum int
	for y := 0; y < 16; y++ {
		ret[y] = make([]tui.Component, 16)
		for x := 0; x < 16; x++ {
			nameNum++
			attr := term.Attribute(nameNum)
			var name string
			switch attr {
			case term.ColorDefault:
				name = "ColorDefault"
			case term.ColorBlack:
				name = "ColorBlack"
			case term.ColorRed:
				name = "ColorRed"
			case term.ColorGreen:
				name = "ColorGreen"
			case term.ColorYellow:
				name = "ColorYellow"
			case term.ColorBlue:
				name = "ColorBlue"
			case term.ColorMagenta:
				name = "ColorMagenta"
			case term.ColorCyan:
				name = "ColorCyan"
			case term.ColorWhite:
				name = "ColorWhite"
			default:
				name = strconv.Itoa(nameNum)
			}
			if dim {
				attr = term.DimAttr(attr)
				name = fmt.Sprintf("D%s", name)
			}
			ret[y][x] = component.NewStringWithConfig(name,
				component.StringConfig{
					Attributes:           term.Attributes{Bg: attr},
					BackgroundAttributes: term.Attributes{Bg: attr},
				},
			)
		}
	}
	return component.Grid(ret)
}

func (h *colorPaletteHandler) Draw(w term.Writer) {
	if h.grid == nil || h.dirty {
		h.dirty = false
		h.grid = makeColorGrid(h.dim)
		h.grid.Resize(h.width, h.height)
	}
	h.grid.Draw(w)
}

func (h *colorPaletteHandler) Handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventKey {
		return
	}
	if ev.Key == term.KeyCtrlD {
		h.dim = !h.dim
		h.dirty = true
		return
	}
	exit = ev.Key == term.KeyEsc
	return
}

func (h *colorPaletteHandler) Cursor() (pos term.Coordinates, style term.CursorStyle, show bool) {
	return
}

func (h *colorPaletteHandler) Man() tui.Manual {
	return tui.Manual{}
}

func (h *colorPaletteHandler) Close() error {
	return nil
}
