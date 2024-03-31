package vte

import (
	"bytes"
	"errors"

	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui/term"
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

	// used just for briding tests with text.Cursor
	conflate()
	wrapLine()
	cursorCRLF()
}

var _ remote = ptyWriter{}

type ptyWriter struct {
	buf  *bytes.Buffer
	comp *Component
}

func ptyWriterRemote(comp *Component) ptyWriter {
	return ptyWriter{comp: comp, buf: new(bytes.Buffer)}
}

func (p ptyWriter) flush() (err error) {
	var clone bytes.Buffer
	_, err = p.buf.WriteTo(&clone)
	if err != nil {
		return
	}
	p.buf.Reset()
	scheduled := term.ScheduleNextTick(func() {
		_ = p.comp.WriteToPty(clone.Bytes())
	})
	if !scheduled {
		err = errors.New("could not schedule pty write: too much data")
	}
	return
}

func (p ptyWriter) writeToPty(data []byte) {
	p.buf.Write(data)
}

func (p ptyWriter) moveLeft() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'D'}
	} else {
		seq = []byte{0x1b, '[', 'D'}
	}
	p.writeToPty(seq)
}

func (p ptyWriter) moveRight() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'C'}
	} else {
		seq = []byte{0x1b, '[', 'C'}
	}
	p.writeToPty(seq)
}

func (p ptyWriter) moveStartOfLine() {
	// ctrl-a
	p.writeToPty([]byte{0x01})
}

func (p ptyWriter) eraseToEndOfLine() {
	// ctrl-k
	p.writeToPty([]byte{0x0B})
}

func (p ptyWriter) eraseToStartOfLine() {
	// ctrl-u
	p.writeToPty([]byte{0x15})
}

func (p ptyWriter) linefeed() {
	var seq []byte
	if p.comp.parserHandler.modeLineFeedNewLine {
		seq = []byte{0x0d, 0x0a}
	} else {
		seq = []byte{0x0d}
	}
	p.writeToPty(seq)
}

func (p ptyWriter) deleteChar() {
	p.writeToPty([]byte{0x1b, '[', '3', '~'})
}

func (p ptyWriter) formFeed() {
	// ctrl-l
	p.writeToPty([]byte{0x0c})
}

func (p ptyWriter) insertChar(ch rune) {
	p.writeToPty([]byte(string(ch)))
}

func (p ptyWriter) keyArrowDown() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'B'}
	} else {
		seq = []byte{0x1b, '[', 'B'}
	}
	p.writeToPty(seq)
}

func (p ptyWriter) keyArrowUp() {
	var seq []byte
	if p.comp.parserHandler.modeCursorKeys {
		seq = []byte{0x1b, 'O', 'A'}
	} else {
		seq = []byte{0x1b, '[', 'A'}
	}
	p.writeToPty(seq)
}

func (p ptyWriter) conflate() {
	// shell automatically conflates rows so no
	// need to do anything here.
	// This method is needed to bridge unit tests with
	// a real shell implementation.
}

func (p ptyWriter) wrapLine() {
	// shell automatically wraps rows so no
	// need to do anything here.
	// This method is needed to bridge unit tests with
	// a real shell implementation.
}

func (p ptyWriter) cursorCRLF() {
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

func (t loggingRemote) log(line string, params ...interface{}) {
	log.WithField(logging.KeyClass, "vte.loggingRemote").
		Tracef(line, params...)
}
