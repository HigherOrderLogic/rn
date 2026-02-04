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
	"fmt"
	"sync"
	"time"

	"github.com/pion/mediadevices/pkg/io/video"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/component/asciiart"
	"unstable.build/go-tui/debug"
	"github.com/unstablebuild/rune-go-sdk/term"
)

const defaultFPS = 30

var _ (tui.Component) = (*Component)(nil)

// Component is a tui.Component that draws a video source
// upon calls to Draw.
type Component struct {
	trackID, streamID  string
	cfg                asciiart.Config
	interrupter        term.Interrupter
	reader             video.Reader
	ctx                context.Context
	cancelCtx          func()
	resize             chan resize
	consumeVideoActive chan struct{}

	rmu    sync.RWMutex
	buf    cell.Buffer
	scroll component.Scroll
}

// NewComponent allocates storage for a new Component and initializes it.
// See Init for more details.
func NewComponent(
	trackID, streamID string,
	interrupter term.Interrupter, fps int,
	reader video.Reader, cfg asciiart.Config,
) *Component {
	ret := new(Component)
	ret.Init(trackID, streamID, interrupter, fps, reader, cfg)
	return ret
}

// Init initializes this component with the given interrupter, fps
// video source and ASCII image encoding configuration.
func (c *Component) Init(
	trackID, streamID string,
	interrupter term.Interrupter, fps int, reader video.Reader,
	cfg asciiart.Config,
) {
	c.trackID, c.streamID = trackID, streamID
	c.cfg = cfg
	c.interrupter = interrupter
	c.reader = reader
	c.ctx, c.cancelCtx = context.WithCancel(context.Background())
	c.resize = make(chan resize)
	c.consumeVideoActive = make(chan struct{})

	c.buf.Init()
	c.scroll.Init(&c.buf)

	if fps == 0 {
		fps = defaultFPS
	}

	cadence := time.Duration(int(time.Second) / fps)
	go debug.CapturePanicReport(func() {
		c.consumeVideoSource(cadence)
	})
}

// Draw satisfies tui.Component.
func (c *Component) Draw(writer term.Writer) {
	c.rmu.RLock()
	defer c.rmu.RUnlock()

	c.scroll.Draw(writer)
}

// Resize satisfies tui.Component.
func (c *Component) Resize(width, height int) {
	c.log(log.DebugLevel, "resizing to width=%d height=%d", width, height)
	c.scroll.Resize(width, height)

	select {
	case c.resize <- resize{width, height}:
	case <-c.consumeVideoActive:
		// do not block main event loop if consumeVideoSource dies
		return
	}
}

// Close closes all resources associated with this Component.
func (c *Component) Close() error {
	c.cancelCtx()
	return nil
}

func (c *Component) consumeVideoSource(cadence time.Duration) {
	defer c.log(log.InfoLevel, "done consuming from video source")
	c.log(log.InfoLevel, "consuming from video source")

	ticker := time.NewTicker(cadence)
	defer ticker.Stop()
	defer close(c.consumeVideoActive)

	var width, height, frameWidth, frameHeight int
	var err error
	for {
		select {
		case resize := <-c.resize:
			width = resize.width
			height = resize.height
			// pre-calculate aspect ratio to avoid
			// Encode having to calculate it for each frame.
			if c.cfg.MaintainAspectRatio && frameWidth != 0 && frameHeight != 0 {
				width, height = asciiart.ResizeMaintainAspectRatio(
					frameWidth, frameHeight, width, height)
			}
			c.log(log.TraceLevel, "resize received. width=%d height=%d", width, height)
		case <-ticker.C:
			frameWidth, frameHeight, err = c.consumeFrame(width, height)
			c.log(log.TraceLevel, "read new frame "+
				"width=%v, height=%v, err=%s", width, height, err)
			if err != nil {
				continue
			}
			if err := c.interrupter.Interrupt(c.ctx); err != nil {
				c.log(log.WarnLevel, "interrupt error: %s", err)
				continue
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Component) consumeFrame(width, height int) (
	frameWidth, frameHeight int, err error,
) {
	img, release, err := c.reader.Read()
	if err != nil {
		err = fmt.Errorf("video read frame: %v", err)
		return
	}
	defer release()

	c.rmu.Lock()
	defer c.rmu.Unlock()

	// aspect ratio is pre-calculated on resize to avoid
	// Encode having to calculate it for each frame.
	cfg := c.cfg
	cfg.MaintainAspectRatio = false
	asciiart.Encode(&c.buf, width, height, img, cfg)

	frameWidth = img.Bounds().Dx()
	frameHeight = img.Bounds().Dy()
	return
}

func (c *Component) log(level log.Level, msg string, args ...interface{}) {
	log.WithFields(log.Fields{
		"trackID":  c.trackID,
		"streamID": c.streamID,
	}).Logf(level, msg, args...)
}

type resize struct {
	width, height int
}
