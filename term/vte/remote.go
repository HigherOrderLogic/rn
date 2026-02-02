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

package vte

import (
	"bytes"
	"context"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/blue/retry"
)

var triggerBellRetryStrategy = retry.CombinedStrategy(
	retry.LimitStrategy(2),
	retry.ExponentialStrategy(10*time.Millisecond, 100*time.Millisecond),
)

type remote interface {
	moveStartOfLine()
	keyArrowUp()
	keyArrowDown()
	deleteChar()
	insertChar(rune)
	linefeed()
	formFeed()
	moveLeft()
	moveRight()
	flush() error
	triggerBell() error

	// used just for briding tests with text.Cursor
	conflate()
	wrapLine()
	cursorCRLF()
}

var _ remote = (*ptyWriter)(nil)

type ptyWriter struct {
	buf              *bytes.Buffer
	comp             *Component
	scheduleCallback func(func()) bool
}

func ptyWriterRemote(comp *Component, scheduleCallback func(func()) bool) *ptyWriter {
	return &ptyWriter{
		comp:             comp,
		buf:              new(bytes.Buffer),
		scheduleCallback: scheduleCallback,
	}
}

func (p *ptyWriter) flush() (err error) {
	data := p.buf.String()
	p.buf.Reset()
	scheduled := p.scheduleCallback(func() {
		_ = p.comp.WriteToPty([]byte(data))
	})
	if !scheduled {
		err = errors.New("could not schedule pty write: too much data")
	}
	return
}

func (p *ptyWriter) writeToPty(data []byte) {
	p.buf.Write(data)
}

func (p *ptyWriter) moveLeft() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'D'}
	} else {
		seq = []byte{0x1b, '[', 'D'}
	}
	p.writeToPty(seq)
}

func (p *ptyWriter) moveRight() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'C'}
	} else {
		seq = []byte{0x1b, '[', 'C'}
	}
	p.writeToPty(seq)
}

func (p *ptyWriter) moveStartOfLine() {
	// ctrl-a
	p.writeToPty([]byte{0x01})
}

func (p *ptyWriter) linefeed() {
	var seq []byte
	if p.comp.parserHandler.modeLineFeedNewLine {
		seq = []byte{0x0d, 0x0a}
	} else {
		seq = []byte{0x0d}
	}
	p.writeToPty(seq)
}

func (p *ptyWriter) deleteChar() {
	p.writeToPty([]byte{0x1b, '[', '3', '~'})
}

func (p *ptyWriter) formFeed() {
	// ctrl-l
	p.writeToPty([]byte{0x0c})
}

func (p *ptyWriter) insertChar(ch rune) {
	p.writeToPty([]byte(string(ch)))
}

func (p *ptyWriter) keyArrowDown() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'B'}
	} else {
		seq = []byte{0x1b, '[', 'B'}
	}
	p.writeToPty(seq)
}

func (p *ptyWriter) keyArrowUp() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'A'}
	} else {
		seq = []byte{0x1b, '[', 'A'}
	}
	p.writeToPty(seq)
}

func (p *ptyWriter) triggerBell() error {
	// the following sequence forces a bell from the underlying shell
	// tested with bash, sh and zsh. It is necessary to work around
	// shell prompt prefixes so comp.CursorAtScroll is correct,
	// or when we need to synchronize after the processing delay of a
	// remote sequence.
	//
	// Retry this otherwise we might end up not processing any events
	// as wait parser handler scheduler might overflow.
	return retry.Retry(context.Background(), triggerBellRetryStrategy,
		func(ctx context.Context) (bool, error) {
			// NOTE: this is ALSO scheduled as a user callback
			// on the next event loop tick, so if event loop channel
			// gets filled up, the performance degrades substantially
			// but eventually it should catch up.
			var data []byte
			if len(p.comp.cfg.Bell) != 0 {
				data = append(data, p.comp.cfg.Bell...)
			} else {
				// ctrl-a, ctrl-g is the default
				data = []byte{0x01, 0x07}
			}
			scheduled := p.scheduleCallback(func() {
				_ = p.comp.WriteToPty(data)
			})
			if !scheduled {
				return true, errors.New("could not schedule pty write: too much data")
			}
			return true, nil
		})
}

func (p *ptyWriter) conflate() {
	// shell automatically conflates rows so no
	// need to do anything here.
	// This method is needed to bridge unit tests with
	// a real shell implementation.
}

func (p *ptyWriter) wrapLine() {
	// shell automatically wraps rows so no
	// need to do anything here.
	// This method is needed to bridge unit tests with
	// a real shell implementation.
}

func (p *ptyWriter) cursorCRLF() {
	// shell automatically moves cursor to start of next row so no
	// need to do anything here.
	// This method is needed to bridge unit tests with
	// a real shell implementation.
}

var _ remote = loggingRemote{}

type loggingRemote struct {
	r remote
}

func newLoggingRemote(remote remote) loggingRemote {
	return loggingRemote{remote}
}

func (p loggingRemote) moveLeft() {
	p.log("moveLeft")
	p.r.moveLeft()
}

func (p loggingRemote) moveRight() {
	p.log("moveRight")
	p.r.moveRight()
}

func (p loggingRemote) moveStartOfLine() {
	p.log("moveStartOfLine")
	p.r.moveStartOfLine()
}

func (p loggingRemote) linefeed() {
	p.log("linefeed")
	p.r.linefeed()
}

func (p loggingRemote) deleteChar() {
	p.log("deleteChar")
	p.r.deleteChar()
}

func (p loggingRemote) formFeed() {
	p.log("formFeed")
	p.r.formFeed()
}

func (p loggingRemote) keyArrowUp() {
	p.log("keyArrowUp")
	p.r.keyArrowUp()
}

func (p loggingRemote) keyArrowDown() {
	p.log("keyArrowDown")
	p.r.keyArrowDown()
}

func (p loggingRemote) insertChar(ch rune) {
	p.log("insertChar('%c')", ch)
	p.r.insertChar(ch)
}

func (p loggingRemote) flush() (err error) {
	err = p.r.flush()
	p.log("flush: %v", err)
	return
}

func (p loggingRemote) conflate() {
	p.log("conflate")
	p.r.conflate()
}

func (p loggingRemote) wrapLine() {
	p.log("wrapLine")
	p.r.wrapLine()
}

func (p loggingRemote) cursorCRLF() {
	p.log("cursorCRLF")
	p.r.cursorCRLF()
}
func (p loggingRemote) triggerBell() error {
	p.log("triggerBell")
	return p.r.triggerBell()
}

func (t loggingRemote) log(line string, params ...interface{}) {
	if !log.IsLevelEnabled(log.TraceLevel) {
		return
	}
	log.WithField(logging.KeyClass, "vte.loggingRemote").
		Tracef(line, params...)
}
