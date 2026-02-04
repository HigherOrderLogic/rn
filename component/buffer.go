// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
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

package component

import (
	"fmt"

	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
	"unstable.build/go-tui/cell"
)

// Buffer wraps a cell.Buffer and returns a tui.Component which satisfies
// Responsive. Note that this is not the most efficient implementation of tui.Component
// for a cell.Buffer. See component.Scroll for more details.
func Buffer(
	buf *cell.Buffer, cfg component.StringResponsiveConfig,
) component.Responsive {
	ret := &respBuf{
		buf: buf,
		cfg: cfg,
	}
	ret.ResponsiveString.Init(buf.RawCells(), cfg)
	return ret
}

var _ component.WithAttributes = (*respBuf)(nil)
var _ component.Scrollable = (*respBuf)(nil)
var _ component.Responsive = (*respBuf)(nil)
var _ fmt.Stringer = (*respBuf)(nil)

type respBuf struct {
	component.ResponsiveString
	cfg           component.StringResponsiveConfig
	buf           *cell.Buffer
	width, height int
}

func (b *respBuf) Height(width int) int {
	b.ResponsiveString.Reset(b.buf.RawCells(), b.cfg)
	return b.ResponsiveString.Height(width)
}

func (b *respBuf) Resize(width, height int) {
	b.width, b.height = width, height
	b.ResponsiveString.Resize(width, height)
}

func (b *respBuf) Draw(w term.Writer) {
	b.ResponsiveString.Reset(b.buf.RawCells(), b.cfg)
	b.ResponsiveString.Resize(b.width, b.height)
	b.ResponsiveString.Draw(w)
}

func (b *respBuf) MaxSeekOffset() int {
	return 0
}

func (b *respBuf) SeekDown() bool {
	return false
}

func (b *respBuf) SeekUp() bool {
	return false
}

func (b *respBuf) SeekOffset() int {
	return 0
}
