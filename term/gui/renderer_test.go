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
	"testing"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/gui/font"
)

func BenchmarkRenderLigaturesHD(b *testing.B) {
	benchmarkRenderLigatures(b, 1920, 1080)
}

func BenchmarkRenderLigaturesHD2(b *testing.B) {
	benchmarkRenderLigatures(b, 2560, 1440)
}

func BenchmarkRenderLigatures4k(b *testing.B) {
	benchmarkRenderLigatures(b, 3840, 2160)
}

func benchmarkRenderLigatures(b *testing.B, pixelsWidth, pixelsHeight int) {
	manager, err := font.NewManager()
	if err != nil {
		b.Logf("new manager: %v", err)
		b.FailNow()
	}
	width := manager.CellsWidth(pixelsWidth)
	height := manager.CellsHeight(pixelsHeight)

	cells := make([][]term.Cell, height)
	for i := 0; i < height; i++ {
		cells[i] = make([]term.Cell, width)
		for j := 0; j < height; j++ {
			if j%2 == 0 {
				cells[i][j].Ch = '='
			} else {
				cells[i][j].Ch = '>'
			}
		}
	}
	image := ebiten.NewImage(pixelsWidth, pixelsHeight)
	font := newFontFace(manager)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			handleLigatures(cells, 4, 10, font.Regular, color.Black, font, image)
		} else {
			handleLigatures(cells, 5, 10, font.Regular, color.Black, font, image)
		}
	}
}
