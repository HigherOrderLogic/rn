package handler

import (
	"fmt"
	"math"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
)

// LessConfig holds configuration values for a Less instance.
type LessConfig struct {
	Debug   bool
	Wrap    bool
	ResAttr term.Attributes
	Handler func(LessEvent)
}

// DefaultLessConfig is a sane configuration defaults for Less.
func DefaultLessConfig() LessConfig {
	return LessConfig{
		Wrap: false,
		ResAttr: term.Attributes{
			Fg: term.AttrReverse,
			Bg: term.ColorDefault,
		},
	}
}

// Less is a clone of Unix' less program which implements
// the Handler and Component interfaces.
type Less struct {
	scroll       component.Scroll
	searchScroll component.Virtual
	msgAlt       component.Responsive
	msgAltVirt   component.Virtual
	msgAltWidth  int
	msg          component.Responsive
	msgVirt      component.Virtual
	mode         LessMode
	delEOF       bool
	cursorOffset int
	height       int
	width        int
	search       string
	config       LessConfig
}

// LessEventType represents a less event
type LessEventType uint8

const (
	// EOF is dispatched when user has reached end of buffer
	EOF LessEventType = iota
	// Search is dispatched when user has performed a text search
	// `Data` field in `Event` struct will be set to the search text
	Search
)

// LessMode represents one of the two modes of a less Handler.
// See Manual for more information on how to switch between modes.
type LessMode uint8

const (
	LessNormalMode LessMode = iota
	LessSearchMode
)

// LessEvent type represents a less event.
type LessEvent struct {
	Type LessEventType
	Data []byte
	Err  error
}

func (l *Less) sendEvent(ev LessEvent) {
	if l.config.Handler != nil {
		l.config.Handler(ev)
	}
}

func getBuffer(virtualScroll component.Virtual) *cell.Buffer {
	return virtualScroll.C.(*component.Scroll).Buffer()
}

// SetNormalMode sets the mode to normal.
func (l *Less) SetNormalMode() {
	l.cursorOffset = 1
	getBuffer(l.searchScroll).Reset()
	l.mode = LessNormalMode
}

// SetSearchMode sets the mode to search mode.
func (l *Less) SetSearchMode() {
	getBuffer(l.searchScroll).Reset()
	getBuffer(l.searchScroll).WriteString("/")
	l.mode = LessSearchMode
}

// SearchText returns the contents of the search buffer.
func (l *Less) SearchText() string {
	str := getBuffer(l.searchScroll).String()
	bytes := []byte(str)[1:]
	return string(bytes)
}

func (l *Less) searchHandleEvent(ev term.Event) (bool, bool) {
	switch ev.Key {
	case term.KeyBackspace:
		fallthrough
	case term.KeyBackspace2:
		if l.cursorOffset > 1 {
			l.cursorOffset--
			l.searchScroll.C.(*component.Scroll).Buffer().
				DeleteCell(term.Coordinates{X: l.cursorOffset, Y: 0})
		}
	case term.KeyEnter:
		l.search = l.SearchText()
		l.scroll.Search(l.search)
		l.SetNormalMode()
		l.scroll.SeekNextResult()
		l.sendEvent(LessEvent{Type: Search, Data: []byte(l.search)})

	case term.KeyEsc:
		l.SetNormalMode()

	case term.KeySpace:
		ev.Ch = ' '
		fallthrough

	default:
		l.cursorOffset++
		getBuffer(l.searchScroll).WriteString(string(ev.Ch))
	}

	return false, true
}

func (l *Less) normalHandleEvent(ev term.Event) (exit, handled bool) {
	switch ev.Type {
	case term.EventKey:
		handled = true
		switch ev.Ch {
		case 'q':
			exit = true
		case 'N':
			l.scroll.SeekPrevResult()
		case 'n':
			l.scroll.SeekNextResult()
		case '0':
			l.scroll.SeekStartLine()
		case '$':
			l.scroll.SeekEndLine()
		case 'g':
			l.scroll.SeekStartFile()
		case 'G':
			l.scroll.SeekEndFile()
		case 'j':
			l.scroll.SeekDown()
		case 'k':
			l.scroll.SeekUp()
		case 'h':
			l.scroll.SeekLeft()
		case 'l':
			l.scroll.SeekRight()
		case '/':
			l.SetSearchMode()
		default:
			switch ev.Key {
			case term.KeyEsc:
				exit = true
			default:
				handled = false
			}
		}
	}

	return
}

func (l *Less) setMessage(msg string) {
	l.msg = component.StringResponsive(msg, component.StringConfig{
		Alignment: component.SpanAlignmentRight,
	})
	l.msgVirt.C = l.msg
}

func (l *Less) setMessageAlt(msg string) {
	b := cell.CellsToBuffer(nil)
	b.WriteString(msg)
	l.msgAlt = component.BufferResponsive(b, component.StringConfig{
		Alignment: component.SpanAlignmentLeft,
	})
	l.msgAltVirt.C = l.msgAlt
	l.msgAltWidth = b.MaxColumns()
}

// SetMessage sets a message to be displayed on the bottom right corner.
func (l *Less) SetMessage(text string, args ...interface{}) {
	l.setMessage(fmt.Sprintf(text, args...))
	l.resize()
}

// SetMessageAlt sets a message to be displayed on the bottom left corner.
func (l *Less) SetMessageAlt(text string, args ...interface{}) {
	l.setMessageAlt(fmt.Sprintf(text, args...))
	l.resize()
}

// Mode returns the current LessMode.
func (l *Less) Mode() LessMode {
	return l.mode
}

// Cursor : Handler
func (l *Less) Cursor() (term.Coordinates, bool) {
	return term.Coordinates{X: l.cursorOffset, Y: l.height - 1}, true
}

// Draw : Component
func (l *Less) Draw(w term.Writer) {
	l.scroll.Draw(w)

	if !l.scroll.CanSeekDown() && !l.delEOF {
		l.delEOF = true
		l.sendEvent(LessEvent{Type: EOF})
	}

	l.msgAltVirt.Draw(w)
	l.msgVirt.Draw(w)
	l.searchScroll.Draw(w)
}

// Resize : Component
func (l *Less) Resize(width, height int) {
	l.width, l.height = width, height
	l.resize()
}

func (l *Less) resize() {
	cmdBarHeight := int(math.Max(float64(l.msg.Height(l.width)),
		float64(l.msgAlt.Height(l.width))))
	if l.height <= cmdBarHeight {
		cmdBarHeight = 1
	}
	contentHeight := l.height - cmdBarHeight
	l.scroll.Resize(l.width, contentHeight)

	l.searchScroll.Move(term.Coordinates{X: 0, Y: contentHeight})
	l.searchScroll.Resize(l.width, cmdBarHeight)

	msgAltWidth := int(math.Min(
		math.Min(float64(l.width), float64(l.msgAltWidth)),
		float64(l.width/2),
	))
	l.msgAltVirt.Move(term.Coordinates{X: 0, Y: contentHeight})
	l.msgAltVirt.Resize(msgAltWidth, cmdBarHeight)

	l.msgVirt.Move(term.Coordinates{X: msgAltWidth, Y: contentHeight})
	l.msgVirt.Resize(l.width-msgAltWidth, cmdBarHeight)
}

// Handle : Handler
func (l *Less) Handle(ev term.Event) (exit bool, handled bool) {
	switch ev.Type {
	case term.EventKey:
		switch l.mode {
		case LessNormalMode:
			exit, handled = l.normalHandleEvent(ev)
		case LessSearchMode:
			exit, handled = l.searchHandleEvent(ev)
		}
	}

	return
}

func (l *Less) setupScroll(w *component.Scroll) {
	w.ResultsAttr = l.config.ResAttr
	w.Wrap = l.config.Wrap
	w.Debug = l.config.Debug
}

// Buffer returns the internal scroll's Buffer.
func (l *Less) Buffer() *cell.Buffer {
	return l.scroll.Buffer()
}

// Scroll returns the internal scroll. Scroll's public properties
// should not be updated. Use LessConfig instead.
func (l *Less) Scroll() *component.Scroll {
	return &l.scroll
}

// Man : Handler
func (l *Less) Man() tui.Manual {
	return tui.Manual{
		Summary: "Less is a handler similar to Unix' less program, but simplified. It allows basic navigation with vi-style key bindings and text search.",
		Keys: tui.KeyMap{
			term.KeyComb{Ch: 'q'}: {
				ID:          "Normal.Exit",
				Description: "Exit handler.",
			},
			term.KeyComb{Ch: 'N'}: {
				ID:          "Normal.SeekPrevResult",
				Description: "Seek to previous search result. See 'SetSearchMode' for more info.",
			},
			term.KeyComb{Ch: 'n'}: {
				ID:          "Normal.SeekNextResult",
				Description: "Seek to next search result. See 'SetSearchMode' for more info.",
			},
			term.KeyComb{Ch: '0'}: {
				ID:          "Normal.SeekStartLine",
				Description: "Seek scroll enough columns to render start of the line.",
			},
			term.KeyComb{Ch: '$'}: {
				ID:          "Normal.SeekEndLine",
				Description: "Seek scroll enough columns to render the end of the line.",
			},
			term.KeyComb{Ch: 'g'}: {
				ID:          "Normal.SeekStartScroll",
				Description: "Seek to start of scroll",
			},
			term.KeyComb{Ch: 'G'}: {
				ID:          "Normal.SeekEndScroll",
				Description: "Seek to end of scroll.",
			},
			term.KeyComb{Ch: 'j'}: {
				ID:          "Normal.SeekDown",
				Description: "Seek scroll one row down.",
			},
			term.KeyComb{Ch: 'k'}: {
				ID:          "Normal.SeekUp",
				Description: "Seek scroll one row up.",
			},
			term.KeyComb{Ch: 'h'}: {
				ID:          "Normal.SeekLeft",
				Description: "Seek scroll one column to the left.",
			},
			term.KeyComb{Ch: 'l'}: {
				ID:          "Normal.SeekRight",
				Description: "Seek scroll one column to the right.",
			},
			term.KeyComb{Ch: '/'}: {
				ID:          "Normal.SetSearchMode",
				Description: "Enter search mode. After typing search text, press ENTER to perform a text-search or ESC to go back to normal mode.",
			},
			term.KeyComb{Key: term.KeyEsc}: {
				ID: "Search.SetNormalMode", Description: "Enter normal mode",
			},
			term.KeyComb{Key: term.KeyEnter}: {
				ID: "Search.Search", Description: "Perform text search with current search buffer.",
			},
		},
	}
}

// Init initializes this instance or resets it if already initialized.
func (l *Less) Init(cfg LessConfig) {
	l.InitWithBuffer(cell.NewBuffer(), cfg)
}

// InitWithBuffer initialzes this instance with the given Buffer and configuration.
// If config is nil, the default one is used.
func (l *Less) InitWithBuffer(buf *cell.Buffer, cfg LessConfig) {
	l.delEOF = false
	l.scroll.Init(buf)
	if cfg.ResAttr == (term.Attributes{}) {
		cfg.ResAttr = DefaultLessConfig().ResAttr
	}

	l.config = cfg
	l.searchScroll.C = component.NewScroll(cell.NewBuffer())

	// initialize message comps
	l.setMessage("")
	l.setMessageAlt("")

	l.setupScroll(l.searchScroll.C.(*component.Scroll))
	l.setupScroll(&l.scroll)

	l.SetNormalMode()

	return
}

// NewLess allocates storage and returns a new instance of Less.
func NewLess(cfg LessConfig) *Less {
	l := new(Less)
	l.Init(cfg)
	return l
}
