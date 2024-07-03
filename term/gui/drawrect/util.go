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
package drawrect

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

var (
	whiteImage    = ebiten.NewImage(3, 3)
	whiteSubImage = whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
)

func init() {
	b := whiteImage.Bounds()
	pix := make([]byte, 4*b.Dx()*b.Dy())
	for i := range pix {
		pix[i] = 0xff
	}
	// This is hacky, but WritePixels is better than Fill in term of automatic texture packing.
	whiteImage.WritePixels(pix)
}

func drawVerticesForUtil(
	dst *ebiten.Image, vs []ebiten.Vertex,
	is []uint16, clr color.RGBA, antialias bool,
) {
	r, g, b, a := clr.RGBA()
	for i := range vs {
		vs[i].SrcX = 1
		vs[i].SrcY = 1
		vs[i].ColorR = float32(r) / 0xffff
		vs[i].ColorG = float32(g) / 0xffff
		vs[i].ColorB = float32(b) / 0xffff
		vs[i].ColorA = float32(a) / 0xffff
	}

	var op ebiten.DrawTrianglesOptions
	op.ColorScaleMode = ebiten.ColorScaleModePremultipliedAlpha
	op.AntiAlias = antialias
	dst.DrawTriangles(vs, is, whiteSubImage, &op)
}

// StrokeLine strokes a line (x0, y0)-(x1, y1) with the specified width and color.
//
// clr has be to be a solid (non-transparent) color.
func DrawStroke(
	dst *ebiten.Image, x0, y0, x1, y1 float32,
	strokeWidth float32, clr color.RGBA, antialias bool,
) {
	var path Path
	path.MoveTo(x0, y0)
	path.LineTo(x1, y1)
	strokeOp := &StrokeOptions{}
	strokeOp.Width = strokeWidth
	vs, is := path.AppendVerticesAndIndicesForStroke(nil, nil, strokeOp)

	drawVerticesForUtil(dst, vs, is, clr, antialias)
}

// DrawFilledRect fills a rectangle with the specified width and color.
func DrawRect(
	path *Path, vs []ebiten.Vertex, is []uint16, dst *ebiten.Image,
	x, y, width, height float32, clr color.RGBA, antialias bool,
) ([]ebiten.Vertex, []uint16) {
	path.Reset()
	path.MoveTo(x, y)
	path.LineTo(x, y+height)
	path.LineTo(x+width, y+height)
	path.LineTo(x+width, y)

	vs = vs[:0]
	is = is[:0]
	vs, is = path.AppendVerticesAndIndicesForFilling(vs, is)

	drawVerticesForUtil(dst, vs, is, clr, antialias)
	return vs, is
}
