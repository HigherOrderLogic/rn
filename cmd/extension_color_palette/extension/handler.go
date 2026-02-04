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

package extension

import (
	"context"
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/api/browserapi"
	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/extensionapi"
	"github.com/unstablebuild/rune-go-sdk/api/textapi"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/extension/extutil"
	"unstable.build/go-tui/rpc"
)

// Grantee returns this extension's extension.Grantee, and it required permissions.
func Grantee() (extension.Grantee, []extensionapi.Permission) {
	return extutil.NewCommandSplitHandler(extutil.CommandSplitHandlerConfig{
		SplitOrientation: browserapi.OrientationRight,
		Handler: func(ctx context.Context, _ textapi.Command,
			grants []extension.Grant, broker rpc.MuxBroker,
			invokeWindow browserapi.Window, config config.Config) (extutil.RedispatchHandler, error) {
			return extutil.NopRedispatchHandler(new(colorPaletteHandler)), nil
		},
		Command: colorPaletteCmd,
	})
}

var colorPaletteCmd = textapi.CommandManual{
	Name: "colorpalette",
	Summary: "Opens a new window and displays all the color codes available " +
		"to customize the UI via configuration.",
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
			var attrs tcell.AttrMask
			color := tcell.PaletteColor(nameNum)
			name := color.Name(true)
			if dim {
				attrs = tcell.AttrDim
				name = fmt.Sprintf("D%s", name)
			}
			ret[y][x] = component.NewStringWithConfig(name,
				component.StringConfig{
					Attributes:           term.Attributes{Bg: color, Attrs: attrs},
					BackgroundAttributes: term.Attributes{Bg: color, Attrs: attrs},
				},
			)
			nameNum++
		}
	}
	nextGridOf := 12
	nextGrid := make([][]tui.Component, 0, nextGridOf)
	var i, x int
	y := -1
	for name := range tcell.ColorNames {
		if i%nextGridOf == 0 {
			y++
			x = 0
			nextGrid = append(nextGrid, make([]tui.Component, nextGridOf))
		}
		var attrs tcell.AttrMask
		color := tcell.GetColor(name)
		if dim {
			attrs = tcell.AttrDim
			name = fmt.Sprintf("D%s", name)
		}
		nextGrid[y][x] = component.NewStringWithConfig(name,
			component.StringConfig{
				Attributes:           term.Attributes{Bg: color, Attrs: attrs},
				BackgroundAttributes: term.Attributes{Bg: color, Attrs: attrs},
			},
		)
		i++
		x++
	}
	ret = append(ret, nextGrid...)
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
	if ev.Ch == 'd' && ev.Mod == term.ModCtrl {
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

func (h *colorPaletteHandler) Selection() (string, bool) {
	return "", false
}

func (h *colorPaletteHandler) Close() error {
	return nil
}
