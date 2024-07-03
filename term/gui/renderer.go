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
package gui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/unstablebuild/tcell/v3"
	imagefont "golang.org/x/image/font"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/gui/drawrect"
	"unstable.build/go-tui/term/gui/drawtext"
	"unstable.build/go-tui/term/gui/font"
)

const (
	dimAlphaPerc    float32 = 0.7
	longestLigature int     = 2
)

var (
	colorBlack color.RGBA = color.RGBA{A: 255}
	colorWhite color.RGBA = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

var ligatures = map[string]rune{
	":=": '≔',
	"!=": '≠',
	"<=": '≤',
	">=": '≥',
	"=>": '⇒',
	"->": '→',
	"<-": '←',
	"<>": '≷',
}

type renderer struct {
	frame            *ebiten.Image
	fontManager      *font.Manager
	font             fontFace
	bgOpacity        float64
	fgOpacity        float64
	bgColor          color.RGBA
	fgColor          color.RGBA
	bgColors         *ebiten.Image
	enableLigatures  bool
	cursorBackground color.RGBA
	cursorForeground color.RGBA

	bufPath     drawrect.Path
	bufVertices []ebiten.Vertex
	bufIndices  []uint16
}

type fontFace struct {
	Regular    imagefont.Face
	Bold       imagefont.Face
	Italic     imagefont.Face
	BoldItalic imagefont.Face
	CellSize   font.CharSize
	OffsetY    float64
}

func newFontFace(fontManager *font.Manager) fontFace {
	return fontFace{
		Regular:    fontManager.RegularFontFace(),
		Bold:       fontManager.BoldFontFace(),
		Italic:     fontManager.ItalicFontFace(),
		BoldItalic: fontManager.BoldItalicFontFace(),
		CellSize:   fontManager.CharSize(),
		OffsetY:    fontManager.OffsetY(),
	}
}

func newRenderer(
	width, height int, deviceScale float64,
	fontManager *font.Manager, bgOpacity, fgOpacity float64, enableLigatures bool,
	cursorAttributes, defaultAttr term.Attributes,
) *renderer {
	imageWidth, imageHeight := fontManager.ImageWidth(width), fontManager.ImageHeight(height)
	var cursorForeground, cursorBackground color.RGBA
	if cursorAttributes.Bg.Valid() {
		rr, g, b := cursorAttributes.Bg.TrueColor().RGB()
		cursorBackground = color.RGBA{R: uint8(rr), G: uint8(g), B: uint8(b), A: 255}
	} else {
		cursorBackground = colorWhite
	}
	if cursorAttributes.Fg.Valid() {
		rr, g, b := cursorAttributes.Fg.TrueColor().RGB()
		cursorForeground = color.RGBA{R: uint8(rr), G: uint8(g), B: uint8(b), A: 255}
	} else {
		cursorForeground = colorBlack
	}

	bgColor := tcellToColor(defaultAttr.Bg, colorBlack, bgOpacity)
	bgColors := ebiten.NewImage(imageWidth, imageHeight)
	bgColors.Fill(bgColor)
	return &renderer{
		frame:            ebiten.NewImage(imageWidth, imageHeight),
		fontManager:      fontManager,
		bgColor:          bgColor,
		bgColors:         bgColors,
		fgColor:          tcellToColor(defaultAttr.Fg, colorWhite, fgOpacity),
		font:             newFontFace(fontManager),
		bgOpacity:        bgOpacity,
		fgOpacity:        fgOpacity,
		enableLigatures:  enableLigatures,
		cursorForeground: cursorForeground,
		cursorBackground: cursorBackground,
	}
}

func (r *renderer) Draw(
	screen *ebiten.Image, cells [][]term.Cell,
	drawCursor bool, cursorPos term.Coordinates,
	cursorStyle term.CursorStyle,
	offsetX, offsetY float64,
) {
	r.frame.Clear()

	r.renderContent(cells)
	if drawCursor {
		r.renderCursor(cells, cursorPos, cursorStyle)
	}

	screen.Clear()
	screen.DrawImage(r.bgColors, nil)

	opt := ebiten.DrawImageOptions{}
	opt.GeoM.Translate(offsetX, offsetY)
	// this blend set allows bgColors to fill background,
	// but disables blending color alpha
	opt.Blend = ebiten.Blend{
		BlendFactorSourceRGB:        ebiten.BlendFactorOne,
		BlendFactorSourceAlpha:      ebiten.BlendFactorOne,
		BlendFactorDestinationRGB:   ebiten.BlendFactorOneMinusSourceAlpha,
		BlendFactorDestinationAlpha: ebiten.BlendFactorOneMinusSourceAlpha,
		BlendOperationRGB:           ebiten.BlendOperationMax,
		BlendOperationAlpha:         ebiten.BlendOperationMax,
	}
	screen.DrawImage(r.frame, &opt)
}

func (r *renderer) renderContent(cells [][]term.Cell) {
	// draw base content for each row
	for viewY := len(cells) - 1; viewY >= 0; viewY-- {
		r.drawRow(cells, viewY, r.fgColor, r.bgColor)
	}
}

func (r *renderer) drawRow(
	cells [][]term.Cell, viewY int,
	defaultForegroundColor, defaultBackgroundColor color.RGBA,
) {
	row := cells[viewY]
	pixelY := float64(viewY) * r.font.CellSize.Y
	textPixelY := pixelY + r.font.OffsetY

	var useFace imagefont.Face
	useFace = r.font.Regular

	var temp color.RGBA
	var skipRunes int
	// draw text content of each cell in row
	for viewX := 0; viewX < len(row); viewX++ {
		cell := row[viewX]

		fg := tcellToColor(cell.Fg, defaultForegroundColor, r.fgOpacity)
		bg := tcellToColor(cell.Bg, defaultBackgroundColor, r.bgOpacity)
		pixelX := r.font.CellSize.X * float64(viewX)

		// reverse attr if AttrReverse
		if cell.Attrs&tcell.AttrReverse != 0 {
			temp = fg
			fg = bg
			bg = temp
		}

		// we don't need to draw empty cells, just draw background
		if cell.Ch == 0 || cell.Ch == '\t' {
			r.bufVertices, r.bufIndices = drawrect.DrawRect(&r.bufPath, r.bufVertices, r.bufIndices,
				r.frame, float32(pixelX), float32(pixelY),
				float32(r.font.CellSize.X), float32(r.font.CellSize.Y), bg, false)
			continue
		}

		isBold := cell.Attrs&tcell.AttrBold != 0
		isItalic := cell.Attrs&tcell.AttrItalic != 0

		var opts ebiten.DrawImageOptions
		opts.GeoM.Translate(pixelX, textPixelY)

		// pick a font face for the cell
		if !isBold && !isItalic {
			useFace = r.font.Regular
		} else if isBold && isItalic {
			useFace = r.font.BoldItalic
		} else if isBold {
			useFace = r.font.Bold
		} else if isItalic {
			useFace = r.font.Italic
		}
		cr, cg, cb, ca := fg.RGBA()
		opts.ColorScale.Scale(
			float32(cr)/0xffff,
			float32(cg)/0xffff,
			float32(cb)/0xffff,
			float32(ca)/0xffff,
		)

		// dim fg text if AttrDim
		if cell.Attrs&tcell.AttrDim != 0 {
			opts.ColorScale.ScaleAlpha(dimAlphaPerc)
		}

		if cell.Attrs&tcell.AttrUnderline != 0 {
			underlinePixelY := pixelY + r.font.CellSize.Y/2
			drawrect.DrawStroke(r.frame, float32(pixelX), float32(underlinePixelY),
				float32(pixelX+r.font.CellSize.X),
				float32(underlinePixelY), 2, fg, false)
		}

		if r.enableLigatures && skipRunes == 0 {
			skipRunes = r.handleLigatures(cells, viewX, viewY, useFace, fg)
		}

		if skipRunes > 0 {
			skipRunes--
			continue
		}

		// draw background
		r.bufVertices, r.bufIndices = drawrect.DrawRect(&r.bufPath, r.bufVertices, r.bufIndices,
			r.frame, float32(pixelX), float32(pixelY),
			float32(r.font.CellSize.X), float32(r.font.CellSize.Y), bg, false)

		// draw text
		drawtext.DrawWithOptions(r.frame, string(cell.Ch), useFace, &opts)
	}
}

func (r *renderer) handleLigatures(
	cells [][]term.Cell, sx, sy int, face imagefont.Face, color color.RGBA,
) (length int) {
	return handleLigatures(cells, sx, sy, face, color, r.font, r.frame)
}

func (r *renderer) renderCursor(
	cells [][]term.Cell, pos term.Coordinates, style term.CursorStyle,
) {
	cell := r.getCell(cells, pos)

	useFace := r.font.Regular
	isBold := cell.Attributes.Attrs&tcell.AttrBold != 0
	isItalic := cell.Attributes.Attrs&tcell.AttrItalic != 0
	if isBold && isItalic {
		useFace = r.font.BoldItalic
	} else if isBold {
		useFace = r.font.Bold
	} else if isItalic {
		useFace = r.font.Italic
	}

	pixelX := float64(pos.X) * r.font.CellSize.X
	pixelY := float64(pos.Y) * r.font.CellSize.Y
	textPixelY := pixelY + r.font.OffsetY
	pixelW, pixelH := r.font.CellSize.X, r.font.CellSize.Y

	// empty rect without focus
	if !ebiten.IsFocused() {
		r.bufVertices, r.bufIndices = drawrect.DrawRect(
			&r.bufPath, r.bufVertices, r.bufIndices,
			r.frame, float32(pixelX), float32(pixelY),
			float32(pixelW), float32(pixelH), r.cursorBackground, false)
		r.bufVertices, r.bufIndices = drawrect.DrawRect(
			&r.bufPath, r.bufVertices, r.bufIndices,
			r.frame, float32(pixelX+1), float32(pixelY+1),
			float32(pixelW-2), float32(pixelH-2), r.cursorForeground, false)
		return
	}

	// draw the cursor shape
	switch style {
	case term.CursorStyleBlinkingBar, term.CursorStyleSteadyBar:
		r.bufVertices, r.bufIndices = drawrect.DrawRect(
			&r.bufPath, r.bufVertices, r.bufIndices,
			r.frame, float32(pixelX), float32(pixelY), 2,
			float32(pixelH), r.cursorBackground, false)
	case term.CursorStyleBlinkingUnderline, term.CursorStyleSteadyUnderline:
		r.bufVertices, r.bufIndices = drawrect.DrawRect(
			&r.bufPath, r.bufVertices, r.bufIndices,
			r.frame, float32(pixelX), float32(pixelY+pixelH-2),
			float32(pixelW), 2, r.cursorBackground, false)
	default:
		r.bufVertices, r.bufIndices = drawrect.DrawRect(
			&r.bufPath, r.bufVertices, r.bufIndices,
			r.frame, float32(pixelX), float32(pixelY),
			float32(pixelW), float32(pixelH), r.cursorBackground, false)
		if cell.Ch != 0 {
			var opts ebiten.DrawImageOptions
			opts.GeoM.Translate(pixelX, textPixelY)
			cr, cg, cb, ca := r.cursorForeground.RGBA()
			opts.ColorScale.Scale(
				float32(cr)/0xffff,
				float32(cg)/0xffff,
				float32(cb)/0xffff,
				float32(ca)/0xffff,
			)
			drawtext.DrawWithOptions(r.frame, string(cell.Ch), useFace, &opts)
		}
	}
}

func (r *renderer) getCell(cells [][]term.Cell, pos term.Coordinates) (ret term.Cell) {
	if pos.Y >= len(cells) || pos.X >= len(cells[pos.Y]) {
		return
	}
	return cells[pos.Y][pos.X]
}

func tcellToColor(tcolor tcell.Color, def color.RGBA, opacity float64) color.RGBA {
	if !tcolor.Valid() || tcolor == tcell.ColorDefault {
		return def
	}
	r, g, b := tcolor.TrueColor().RGB()
	if opacity == 1 {
		return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	}
	alpha := uint8(float64(255) * opacity)
	return color.RGBA{
		R: uint8(float64(r) * opacity),
		G: uint8(float64(g) * opacity),
		B: uint8(float64(b) * opacity),
		A: alpha,
	}
}

func handleLigatures(
	cells [][]term.Cell, sx, sy int, face imagefont.Face, color color.RGBA,
	font fontFace, frame *ebiten.Image,
) (length int) {
	var c [longestLigature]rune
	candidate := c[:0]
	for i := 0; i < longestLigature; i++ {
		x := sx + i
		if sy >= len(cells) || x >= len(cells[sy]) || cells[sy][x].Ch == 0 {
			break
		}
		candidate = append(candidate, cells[sy][x].Ch)
	}

	for len(candidate) > 1 {
		if ru, ok := ligatures[string(candidate)]; ok {
			// draw ligature
			ligX := (float64(sx) * font.CellSize.X) + ((float64(len(candidate)-1) * font.CellSize.X) / 2)
			ligY := float64(sy)*font.CellSize.Y + font.OffsetY
			var opts ebiten.DrawImageOptions
			opts.GeoM.Translate(ligX, ligY)
			cr, cg, cb, ca := color.RGBA()
			opts.ColorScale.Scale(
				float32(cr)/0xffff,
				float32(cg)/0xffff,
				float32(cb)/0xffff,
				float32(ca)/0xffff,
			)
			drawtext.DrawWithOptions(frame, string(ru), face, &opts)
			return len(candidate)
		}
		candidate = candidate[:len(candidate)-1]
	}

	return 0
}
