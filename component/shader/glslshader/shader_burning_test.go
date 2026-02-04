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

package glslshader

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"testing"

	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/component"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/component/shader/shadertest"
)

//go:embed test_logo.png
var testLogo []byte

func openTestLogo() image.Image {
	img, err := png.Decode(bytes.NewReader(testLogo))
	if err != nil {
		panic(fmt.Errorf("png decode: %v", err))
	}
	return img
}

func TestBurning(t *testing.T) {
	sh := Burning(
		DefaultBurningParams(openTestLogo(), component.FrameCharSetDefault()),
		term.Attributes{},
		4.0, 30,
	)
	shadertest.TestShader(t, sh)
}

func BenchmarkBurning(b *testing.B) {
	sh := Burning(
		DefaultBurningParams(openTestLogo(), component.FrameCharSetDefault()),
		term.Attributes{},
		4.0, 30,
	)
	width, height := 600, 400
	cfg := asciiart.DefaultConfig()
	cfg.Color = true
	cfg.MaintainAspectRatio = true
	cfg.DensityCharacters = "\u2009▓▓▓▓▓▓▓▓▓"
	image := asciiart.NewComponent(openTestLogo(), cfg)
	span := component.NewSpan(image, component.SpanConfig{
		PadHorizontalPerc: 0.4,
		PadVerticalPerc:   0.2,
		ContentAlignment:  component.AlignmentCentered,
	})
	writer := cell.NewBufferWriter(context.Background(), width, height)
	span.Resize(width, height)
	span.Draw(writer)
	shadertest.BenchmarkShader(b, sh, width, height, writer.RawCells())
}
