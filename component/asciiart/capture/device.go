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
	"fmt"

	"github.com/ernestrc/go-multierror"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"unstable.build/go-tui/component/asciiart"
)

var _ (tui.Component) = (*Device)(nil)

// Device is a helper structure to setup a Component and manage the lifecycle
// of it along with the video track. When the video track closes unexpectedly,
// the next call to draw renders an error message.
type Device struct {
	component *Component
	stream    mediadevices.MediaStream
	tracks    []mediadevices.Track
	closedErr tui.Component
}

// NewDevice creates a new video capturing device and exposes it as a tui.Component.
// The given interrupter will be used to invoke draws at the given fps frames per second.
// See more details here: https://github.com/pion/mediadevices/blob/master/examples/webrtc/main.go
func NewDevice(interrupter term.Interrupter, fps int, cfg asciiart.Config) (
	*Device, error,
) {
	/*
		const lowBitRate = 32000
		x264Params, err := x264.NewParams()
		if err != nil {
			panic(err)
		}
		x264Params.BitRate = lowBitRate
		x264Params.Preset = x264.PresetUltrafast

		vp8Params, err := vpx.NewVP8Params()
		if err != nil {
			panic(err)
		}
		vp8Params.BitRate = lowBitRate

		vp9Params, err := vpx.NewVP9Params()
		if err != nil {
			panic(err)
		}
		vp9Params.BitRate = lowBitRate
	*/

	opusParams, err := opus.NewParams()
	if err != nil {
		panic(err)
	}
	codecSelector := mediadevices.NewCodecSelector(
		// mediadevices.WithVideoEncoders(&vp8Params),
		//mediadevices.WithVideoEncoders(&vp9Params),
		//mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)
	s, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.FrameFormat = prop.FrameFormat(frame.FormatI420)
			c.Width = prop.Int(160)
			c.Height = prop.Int(120)
		},
		Audio: func(c *mediadevices.MediaTrackConstraints) {
		},
		Codec: codecSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("get user media: %v", err)
	}

	ret := new(Device)
	return ret, ret.Init(interrupter, fps, s, cfg)
}

// Init initializes this device with the given interrupter, fps and
// MediaStream.
func (d *Device) Init(
	interrupter term.Interrupter, fps int,
	stream mediadevices.MediaStream, cfg asciiart.Config,
) error {
	d.stream = stream
	d.tracks = stream.GetTracks()

	var vidReader video.Reader
	var trackID, streamID string
	for _, track := range d.tracks {
		track.OnEnded(d.onTrackEnded(track))
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			vidTrack := track.(*mediadevices.VideoTrack)
			vidReader = vidTrack.Broadcaster.NewReader(true /* copy frame */)
			trackID = track.ID()
			streamID = track.StreamID()
		}
		log.WithFields(log.Fields{
			"trackID":   track.ID(),
			"streamID":  track.StreamID(),
			"trackKind": track.Kind(),
		}).Debug("got track")
	}

	if vidReader != nil {
		d.component = NewComponent(trackID, streamID, interrupter, fps, vidReader, cfg)
	} else {
		d.closedErr = component.NewStringWithConfig("no video track",
			component.StringConfig{Alignment: component.AlignmentCentered})
	}
	return nil
}

// Draw satisfies tui.Component.
func (d *Device) Draw(writer term.Writer) {
	if d.closedErr != nil {
		d.closedErr.Draw(writer)
		return
	}
	d.component.Draw(writer)
}

// Resize satisfies tui.Component.
func (d *Device) Resize(width, height int) {
	if d.closedErr != nil {
		d.closedErr.Resize(width, height)
		return
	}
	d.component.Resize(width, height)
}

// Close releases all media tracks and resources
// associated with this device.
func (d *Device) Close() (ret error) {
	for _, track := range d.tracks {
		d.stream.RemoveTrack(track)
		if err := track.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	if d.component != nil {
		if err := d.component.Close(); err != nil {
			ret = multierror.Append(ret, err)
		}
	}
	return ret
}

func (d *Device) onTrackEnded(track mediadevices.Track) func(error) {
	return func(err error) {
		log.Errorf("Track (ID: %s) ended with error: %v\n", track.ID(), err)
		// TODO if audio, overlay error
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			d.closedErr = component.NewStringWithConfig(fmt.Sprintf("video track ended: %s", err),
				component.StringConfig{Alignment: component.AlignmentCentered})
		}
	}
}
