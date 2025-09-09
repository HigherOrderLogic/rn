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

package component

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"unstable.build/go-tui"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
)

const (
	defaultFPS = 30
	lenPrefix  = 8
)

var _ (tui.Component) = (*Animation)(nil)

// ProgressAnimationFrames returns the frames and sequence numbers of the default
// progress animation. It only needs 2 term.Cells in terms of width and 1 cell in height.
func ProgressAnimationFrames() ([]string, []int) {
	return []string{"⠃", "⠅", "⠆", "⠘", "⠨", "⠰", "⠉", "⠒", "⠤", "⠑", "⠡", "⠢", "⠊", "⠌", "⠔", "⠇", "⠸", "⠎", "⠱", "⠣", "⠜", "⠪", "⠕", "⠋", "⠙", "⠓", "⠚", "⠍", "⠩", "⠥", "⠬", "⠖", "⠲", "⠦", "⠴", "⠏", "⠹", "⠧", "⠼", "⠫", "⠝", "⠮", "⠵", "⠺", "⠗", "⠞", "⠳", "⠛", "⠭", "⠶", "⠟", "⠻", "⠷", "⠾", "⠯", "⠽", "⠿"},
		[]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35,
			36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56}
}

// SpinningAnimationFrames returns the frames and sequence numbers of the default
// progress animation. It only needs 2 term.Cells in terms of width and 1 cell in height.
func SpinningAnimationFrames() ([]string, []int) {
	return []string{"◦", "◯", "◴", "◵", "◶", "◷", "◌", "◎"},
		[]int{0, 1, 2, 3, 4, 5, 6, 7}
}

// CursorAnimationFrames returns the frames and sequence numbers of the default
// cursor animation. It only needs 1 term.Cells in terms of width and 1 cell in height.
func CursorAnimationFrames() ([]string, []int) {
	return []string{"▋", " "}, []int{0, 1}
}

// DecodeAnimation decodes the given raw compressed animation into
// an Animation tui.Component. If raw is empty, an empty animation is returned.
// This method returns an error if raw is somehow corrupted or is missing data.
func DecodeAnimation(
	raw []byte, fps int,
	interrupter term.Interrupter,
) (*Animation, error) {
	if len(raw) < lenPrefix {
		a := NewAnimation(interrupter,
			nil /* frames */, nil /* sequence */, fps)
		return a, nil
	}

	frames := make([]string, 0)
	sequence := make([]int, 0)

	for len(raw) != 0 {
		lenFrame, times := decodeLen(raw)
		if len(raw) < lenPrefix+int(lenFrame) {
			return nil, errors.New("corrupted animation data: missing frame data")
		}

		frame := raw[lenPrefix : lenPrefix+lenFrame]
		for i := 0; i < int(times); i++ {
			sequence = append(sequence, len(frames))
		}
		frames = append(frames, string(frame))

		raw = raw[lenPrefix+lenFrame:]
	}

	return NewAnimation(interrupter, frames, sequence, fps), nil
}

// EncodeAnimation encodes the given Animation as a compressed slice of bytes
// that can be stored or transferred. If the underlying Animation's frames
// are dynamic components, then this is lost and encoded as a static string component,
// encoded with the given width and height.
func EncodeAnimation(a *Animation, width, height int) []byte {
	padding := [lenPrefix]byte{0, 0, 0, 0, 0, 0, 0, 0}

	raw := make([]string, len(a.frames))
	for i, frame := range a.frames {
		if stringer, ok := frame.(fmt.Stringer); ok {
			raw[i] = stringer.String()
		} else {
			var writer term.StringWriter
			writer.Init(width, height)
			frame.Resize(width, height)
			frame.Draw(&writer)
			writer.Flush()
			raw[i] = writer.String()
		}
	}

	var ret bytes.Buffer
	var n int
	var times int32
	for i, sequenceID := range a.sequence {
		times++

		if i != len(a.sequence)-1 && a.sequence[i+1] == sequenceID {
			// do not flush until we change frames
			continue
		}

		frame := raw[sequenceID]
		lenFrame := len([]byte(frame))
		ret.Write(padding[:])
		encodeLen(ret.Bytes()[n:n+lenPrefix], int32(lenFrame), times)
		ret.WriteString(frame)

		n += (lenPrefix + lenFrame)
		times = 0
	}
	return ret.Bytes()
}

// Animation is a tui.Component that renders a series
// of frames on loop at the specified fps.
type Animation struct {
	interrupter   term.Interrupter
	frames        []tui.Component
	sequence      []int
	fps           int
	i             int
	ctx           context.Context
	cancelCtx     func()
	waitInterrupt chan struct{}
}

// NewAnimation allocates storage for a new Animation and initializes it.
// See Init for more details.
func NewAnimation(
	interrupter term.Interrupter,
	frames []string, sequence []int, fps int,
) *Animation {
	ret := new(Animation)
	ret.Init(interrupter, frames, sequence, fps)
	return ret
}

// Init initializes this animation with the given interrupter,
// frames, fps. It fires a goroutine which will call the
// given interrupter at the specified fps.
//
// Close should be called to cleanup resources and stop the
// interrupt goroutine.
func (a *Animation) Init(
	interrupter term.Interrupter,
	frames []string, sequence []int, fps int,
) {
	components := make([]tui.Component, len(frames))
	for i, frame := range frames {
		components[i] = NewStringWithConfig(frame, StringConfig{
			Alignment: SpanAlignmentCentered,
		})
	}
	a.InitWithComponents(context.Background(), interrupter, components, sequence, fps)
}

// InitWithComponents initializes this animation with the given interrupter,
// frames as tui.Components, and fps. The given context is passed in calls to
// interrupter.Interrupt. See Init for more details.
func (a *Animation) InitWithComponents(
	ctx context.Context, interrupter term.Interrupter,
	frames []tui.Component, sequence []int, fps int,
) {
	// assert frames and sequence are consistent with each other
	// so we panic on Init to indicate programmer error
	// and not halfway through the animation.
	for _, sequenceID := range sequence {
		if sequenceID > len(frames)-1 {
			panic("invalid sequence and frames pair")
		}
	}
	if fps == 0 {
		fps = defaultFPS
	}
	a.waitInterrupt = make(chan struct{})
	a.interrupter = interrupter
	a.fps = fps
	a.sequence = sequence
	a.frames = frames

	// do not use the given context for lifecycle monitoring
	a.ctx, a.cancelCtx = context.WithCancel(context.Background())
	go debug.CapturePanicReport(func() {
		a.interrupt(ctx)
	})
}

// Resize satisfies tui.Component.
func (a *Animation) Resize(width, height int) {
	for _, frame := range a.frames {
		frame.Resize(width, height)
	}
}

// Draw satisfies tui.Component.
func (a *Animation) Draw(w term.Writer) {
	if len(a.sequence) == 0 {
		return
	}
	sequenceIdx := a.i % len(a.sequence)
	sequenceID := a.sequence[sequenceIdx]
	a.frames[sequenceID].Draw(w)
	a.i++
}

// Close cleans all resources associated with this Animation.
func (a *Animation) Close() error {
	a.cancelCtx()
	<-a.waitInterrupt
	return nil
}

func (a *Animation) interrupt(ctx context.Context) {
	defer close(a.waitInterrupt)
	for a.interruptFullSequence(ctx) {
	}
}

func (a *Animation) interruptFullSequence(ctx context.Context) bool {
	cadence := time.Duration(int(time.Second) / a.fps)
	ticker := time.NewTicker(cadence)
	defer ticker.Stop()

	if len(a.sequence) == 0 {
		return false
	}

	for i := 0; i < len(a.sequence); i++ {
		select {
		case <-ticker.C:
			_ = a.interrupter.Interrupt(ctx)
		case <-a.ctx.Done():
			return false
		case <-ctx.Done():
			return false
		}
	}

	return true
}

func decodeLen(b []byte) (int32, int32) {
	return int32(b[3]) | int32(b[2])<<8 | int32(b[1])<<16 | int32(b[0])<<24,
		int32(b[7]) | int32(b[6])<<8 | int32(b[5])<<16 | int32(b[4])<<24
}

func encodeLen(b []byte, length, times int32) {
	b[0] = byte(length >> 24 & 0x00FF)
	b[1] = byte(length >> 16 & 0x00FF)
	b[2] = byte(length >> 8 & 0x00FF)
	b[3] = byte(length)
	b[4] = byte(times >> 24 & 0x00FF)
	b[5] = byte(times >> 16 & 0x00FF)
	b[6] = byte(times >> 8 & 0x00FF)
	b[7] = byte(times)
}
