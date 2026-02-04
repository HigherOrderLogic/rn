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

package asciiart

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"

	"github.com/disintegration/imaging"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"github.com/unstablebuild/rune-go-sdk/term"
)

const (
	// DefaultDensityCharacters are the default characters used by Encode
	// as density encoding characters.
	DefaultDensityCharacters = "   `'_.,-*=+:;cba!?0123456789$W#@"
	// AlternateDensityCharacters is a set of more realistic encoding characters.
	AlternateDensityCharacters = "    .:░▒▓█"
	// HeightToWidthCellAspectRatio is a modifier to take height to width cell aspect ratio
	// into consideration.
	HeightToWidthCellAspectRatio float64 = 2.3
)

// DefaultScaler is the default draw.Scaler in DefaultConfig.
func DefaultScaler() draw.Scaler {
	return draw.NearestNeighbor
}

// DefaultConfig returns the default sane configuration for Encode.
func DefaultConfig() Config {
	return Config{
		Scaler:            DefaultScaler(),
		DensityCharacters: DefaultDensityCharacters,
		Color:             false,
		AdjustContrast:    0,
	}
}

// ResizeMaintainAspectRatio adjusts the given image height and width to
// dstHeight and dstWidth boundaries, respecting the original aspect ratio.
func ResizeMaintainAspectRatio(srcWidth, srcHeight, dstWidth, dstHeight int) (
	width, height int,
) {
	aspectRatio := float64(srcWidth) / float64(srcHeight) * HeightToWidthCellAspectRatio
	height = dstHeight
	for {
		width = int(float64(height) * aspectRatio)
		if width <= dstWidth {
			break
		}
		height--
	}
	return
}

// Config is used to configure Encode.
type Config struct {
	// DensityCharacter is used to encode the density of a pixel. It is
	// assumed that it's sorted by density in ascending order.
	DensityCharacters string
	// Whether the final ASCII image should be encoded in color or not.
	Color  bool
	Scaler draw.Scaler

	// AdjustContrast adjusts the contrast of the image.
	// It ranges from -100 (decrease contrst by 100% to
	// 100 (increase contrast by 100%).
	AdjustContrast float64

	// If MaintainAspectRatio is true, then the original image
	// aspect ratio will be maintained.
	MaintainAspectRatio bool
}

// Encode takes an image.Image and encodes it in ASCII representation
// into the given cell.Buffer. The arguments outputHeight and outputWidth
// are used to determined the desired output height and width in cells.
// The argument config provides the density characters, a scaler and whether
// the final image should be encoded in color.
// assumed that it's sorted by density in ascending order.
func Encode(
	output *cell.Buffer, outputWidth, outputHeight int,
	src image.Image, config Config,
) {
	density := []rune(config.DensityCharacters)
	bounds := src.Bounds()
	origWidth, origHeight := outputWidth, outputHeight
	var padX, padY int

	output.Reset()
	if config.MaintainAspectRatio {
		outputWidth, outputHeight = ResizeMaintainAspectRatio(
			bounds.Dx(), bounds.Dy(), outputWidth, outputHeight)
		if outputWidth < origWidth {
			padX = (origWidth - outputWidth) / 2
		}
		if outputHeight < origHeight {
			padY = (origHeight - outputHeight) / 2
		}
		output.Insert(term.Coordinates{
			X: outputWidth + padX + padX,
			Y: outputHeight + padY + padY,
		}, ' ')
	}

	var dst *image.NRGBA
	dst = image.NewNRGBA(image.Rect(0, 0, outputWidth, outputHeight))
	config.Scaler.Scale(dst, dst.Rect, src, bounds, draw.Over, nil)
	if config.AdjustContrast != 0 {
		dst = imaging.AdjustContrast(dst, config.AdjustContrast)
	}

	for y := 0; y < outputHeight; y++ {
		for x := 0; x < outputWidth; x++ {
			c := dst.At(x, y)
			rgba := c.(color.NRGBA)
			r, g, b := rgba.R, rgba.G, rgba.B
			avg := (float64(r) + float64(g) + float64(b)) / 3.0
			idx := int(math.Floor(mapValue(avg, 0, 255, 0, float64(len(density)-1))))
			character := density[idx]
			var attr term.Attributes
			if config.Color {
				fg := tcell.NewColor(int32(r), int32(g), int32(b))
				attr = term.Attributes{Fg: fg}
			}
			output.InsertWithAttr(term.Coordinates{X: x + padX, Y: y + padY}, character, attr)
		}
	}
}

func mapValue(value, inMin, inMax, outMin, outMax float64) float64 {
	return (value-inMin)*(outMax-outMin)/(inMax-inMin) + outMin
}
