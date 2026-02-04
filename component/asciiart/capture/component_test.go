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

package capture

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"unstable.build/go-tui/component/asciiart"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func TestComponent(t *testing.T) {
	ch := make(chan struct{})

	trackID, streamID := "1", "s:1"
	fps := 1
	interrupter := term.FuncInterrupter(func(context.Context) error {
		ch <- struct{}{}
		return nil
	})
	reader := testVideoReader{}
	cfg := asciiart.DefaultConfig()
	w := term.NewStringWriter(8, 4)

	expected := "1111    \n1111    \n    @@@@\n    @@@@"

	// sut
	c := NewComponent(trackID, streamID, interrupter, fps, reader, cfg)
	c.Resize(8, 4)

	// first one's resize might not have been processed yet
	<-ch
	<-ch
	c.Draw(w)

	require.NoError(t, w.Flush())
	assert.Equal(t, expected, w.String())

	require.NoError(t, c.Close())
	goleak.VerifyNone(t)
}

type testVideoReader struct {
}

func (t testVideoReader) Read() (image.Image, func(), error) {
	width := 80
	height := 40

	upLeft := image.Point{0, 0}
	lowRight := image.Point{width, height}

	img := image.NewRGBA(image.Rectangle{upLeft, lowRight})

	cyan := color.RGBA{100, 200, 200, 0xff}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			switch {
			case x < width/2 && y < height/2:
				img.Set(x, y, cyan)
			case x >= width/2 && y >= height/2:
				img.Set(x, y, color.White)
			default:
			}
		}
	}
	return img, func() {}, nil
}
