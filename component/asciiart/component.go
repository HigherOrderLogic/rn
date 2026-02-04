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

	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// NewComponent returns a tui.Component that renders the given image
// as ascii art.
func NewComponent(img image.Image, config Config) tui.Component {
	buf := new(cell.Buffer)
	buf.InitPerformance(100, 200, ' ')
	scroll := new(component.Scroll)
	scroll.InitPerformance(buf)
	return &imgComp{
		img:    img,
		config: config,
		scroll: scroll,
		dirty:  true,
	}
}

type imgComp struct {
	img           image.Image
	config        Config
	scroll        *component.Scroll
	dirty         bool
	width, height int
}

func (c *imgComp) Draw(w term.Writer) {
	if c.dirty {
		c.scroll.Buffer().Reset()
		Encode(c.scroll.Buffer(), c.width, c.height, c.img, c.config)
		c.dirty = false
	}
	c.scroll.Draw(w)
}

func (c *imgComp) Resize(width, height int) {
	c.dirty = true
	c.width = width
	c.height = height
	c.scroll.Resize(width, height)
}
