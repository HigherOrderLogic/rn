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

package shader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/debug"
	"github.com/unstablebuild/rune-go-sdk/term"
)

// Component wraps a tui.Component with a Shader.
type Component struct {
	shader      Shader
	root        tui.Component
	interrupter term.Interrupter
	buf         *cell.BufferWriter
	total       int
	fps         int

	cancelCtx func()
	epoch     atomic.Int64
	done      atomic.Bool
}

// New wraps a tui.Component with the given shader and returns
// a Component that will use the give interrupter to animate it
// at the given fps. If fps is 0, a sane default is used.
func New(
	comp tui.Component, shader Shader,
	interrupter term.Interrupter, fps int,
	duration time.Duration,
) *Component {
	const defaultFPS = 30
	if fps == 0 {
		fps = defaultFPS
	}

	ctx, cancel := context.WithCancel(context.Background())
	ret := &Component{
		shader:      shader,
		root:        comp,
		interrupter: interrupter,
		buf:         cell.NewBufferWriter(context.Background(), 1, 1),
		total:       int(duration / time.Duration(int(time.Second)/fps)),
		fps:         fps,
		cancelCtx:   cancel,
	}

	go debug.CapturePanicReport(func() {
		ret.interrupt(ctx)
	})

	return ret
}

// Draw satisfies tui.Component.
func (c *Component) Draw(w term.Writer) {
	if c.done.Load() {
		c.root.Draw(w)
		return
	}

	c.buf.SetContext(w.Context())
	_ = c.buf.Clear(term.Attributes{})

	c.root.Draw(c.buf)

	cells := c.buf.RawCells()
	c.shader.Shade(int(c.epoch.Load()), c.total, c.buf.RawCells())

	for y, row := range cells {
		for x, cell := range row {
			w.SetCell(term.Coordinates{Y: y, X: x}, cell)
		}
	}
}

// Resize satisfies tui.Component.
func (c *Component) Resize(width, height int) {
	if !c.done.Load() {
		c.buf = cell.NewBufferWriter(context.Background(), width, height)
	}
	c.root.Resize(width, height)
}

// Close cleans all resources associated with this Component.
func (s *Component) Close() error {
	s.cancelCtx()
	return nil
}

func (c *Component) interrupt(ctx context.Context) {
	cadence := time.Duration(int(time.Second) / c.fps)
	ticker := time.NewTicker(cadence)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			exit := c.epoch.Add(1) == int64(c.total)
			if exit {
				c.done.Store(true)
			}
			// always interrupt even after done is set to true
			// so we ensure that the root component is drawn intact again.
			_ = c.interrupter.Interrupt(ctx)
			if exit {
				return
			}
		case <-ctx.Done():
			c.done.Store(true)
			return
		}
	}
}
