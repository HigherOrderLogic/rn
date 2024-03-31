package handler

import (
	"fmt"
	"math"

	"github.com/ernestrc/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
)

// LessConfig holds configuration values for a Less instance.
type LessConfig struct {
	Debug      bool
	Wrap       bool
	ResAttr    term.Attributes
	BarAttr    term.Attributes
	Attributes term.Attributes
	Handler    func(LessEvent)
	NoBar      bool
}

// DefaultLessConfig is a sane configuration defaults for Less.
func DefaultLessConfig() LessConfig {
	return LessConfig{
		Wrap: false,
		ResAttr: term.Attributes{
			Attrs: tcell.AttrReverse,
		},
		NoBar: false,
	}
}

// Less is a clone of Unix' less program which implements
// the Handler and Component interfaces.
type Less struct {
	scroll           *component.Scroll
	searchScroll     component.Scroll
	searchScrollVirt component.Virtual
	msg              component.Responsive
	msgVirt          component.Virtual
	mode             LessMode
	delEOF           bool
	cursorOffset     int
	height           int
	width            int
	search           string
	config           LessConfig
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

// SetNormalMode sets the mode to normal.
func (l *Less) SetNormalMode() {
	l.cursorOffset = 1
	l.searchScroll.Buffer().Reset()
	l.mode = LessNormalMode
}

// SetSearchMode sets the mode to search mode.
func (l *Less) SetSearchMode() {
	buf := l.searchScroll.Buffer()
	buf.Reset()
	buf.WriteString("/")
	l.mode = LessSearchMode
	l.resizeSearchScroll(l.cmdBarHeight())
}

// SearchText returns the contents of the search buffer.
func (l *Less) SearchText() string {
	str := l.searchScroll.Buffer().String()
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
			l.searchScroll.Buffer().
				DeleteCell(term.Coordinates{X: l.cursorOffset, Y: 0})
			l.resizeSearchScroll(l.cmdBarHeight())
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
		l.searchScroll.Buffer().WriteString(string(ev.Ch))
		l.resizeSearchScroll(l.cmdBarHeight())
	}

	return false, true
}

func (l *Less) normalHandleEvent(ev term.Event) (exit, handled bool) {
	switch ev.Type {
	case term.EventKey:
		switch ev.Ch {
		case 'q':
			exit = true
			handled = true
		case 'N':
			handled = l.scroll.SeekPrevResult()
		case 'n':
			handled = l.scroll.SeekNextResult()
		case '0':
			handled = l.scroll.SeekStartLine()
		case '$':
			handled = l.scroll.SeekEndLine()
		case 'g':
			handled = l.scroll.SeekStartFile()
		case 'G':
			handled = l.scroll.SeekEndFile()
		case 'j':
			handled = l.scroll.SeekDown()
		case 'k':
			handled = l.scroll.SeekUp()
		case 'h':
			handled = l.scroll.SeekLeft()
		case 'l':
			handled = l.scroll.SeekRight()
		case '/':
			l.SetSearchMode()
			handled = true
		default:
			switch ev.Key {
			case term.KeyEsc:
				exit = true
				handled = true
			default:
				handled = false
			}
		}
	}

	return
}

func (l *Less) setMessage(msg string) bool {
	newMsg := component.NewResponsiveString(msg, component.StringResponsiveConfig{
		StringConfig: component.StringConfig{
			Alignment:            component.SpanAlignmentRight,
			Attributes:           l.config.BarAttr,
			BackgroundAttributes: l.config.BarAttr,
		},
	})
	shouldResize := l.msg == nil || l.msg.Height(l.width) != newMsg.Height(l.width)

	l.msg = newMsg
	l.msgVirt.C = l.msg
	return shouldResize
}

// SetMessage sets a message to be displayed on the bottom right corner.
func (l *Less) SetMessage(text string, args ...interface{}) {
	if (len(args) == 0 && l.setMessage(text)) || l.setMessage(fmt.Sprintf(text, args...)) {
		l.resize()
	} else {
		cmdBarHeight := l.cmdBarHeight()
		contentHeight := l.height - cmdBarHeight
		l.initMsg(cmdBarHeight, contentHeight)
	}
}

// Mode returns the current LessMode.
func (l *Less) Mode() LessMode {
	return l.mode
}

// Cursor : Handler
func (l *Less) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	ret := term.Coordinates{X: l.cursorOffset, Y: l.height - 1}
	return ret, term.CursorStyleDefault, true
}

// Draw : Component
func (l *Less) Draw(w term.Writer) {
	l.scroll.Draw(w)

	if !l.scroll.CanSeekDown() && !l.delEOF {
		l.delEOF = true
		l.sendEvent(LessEvent{Type: EOF})
	}

	l.msgVirt.Draw(w)
	l.searchScrollVirt.Draw(w)
}

// Resize : Component
func (l *Less) Resize(width, height int) {
	l.width, l.height = width, height
	l.resize()
}

// ShowCommandBar determines whether the command bar should be
// displayed or not.
func (l *Less) ShowCommandBar(show bool) {
	l.config.NoBar = !show
}

func (l *Less) cmdBarHeight() int {
	var cmdBarHeight int
	if !l.config.NoBar {
		cmdBarHeight = l.msg.Height(l.width)
		if l.height <= cmdBarHeight {
			cmdBarHeight = 1
		}
	}
	return cmdBarHeight
}

func (l *Less) resizeSearchScroll(cmdBarHeight int) {
	width := int(math.Min(float64(l.searchScroll.Buffer().MaxColumns()), float64(l.width)))
	l.searchScrollVirt.Resize(width, cmdBarHeight)
}

func (l *Less) resize() {
	cmdBarHeight := l.cmdBarHeight()
	contentHeight := l.height - cmdBarHeight
	l.scroll.Resize(l.width, contentHeight)

	l.searchScrollVirt.Move(term.Coordinates{X: 0, Y: contentHeight})
	l.resizeSearchScroll(cmdBarHeight)

	l.initMsg(cmdBarHeight, contentHeight)
}

func (l *Less) initMsg(cmdBarHeight int, contentHeight int) {
	l.msgVirt.Move(term.Coordinates{X: 0, Y: contentHeight})
	l.msgVirt.Resize(l.width, cmdBarHeight)
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

func (l *Less) setupScroll(w *component.Scroll, attr term.Attributes) {
	w.ResultsAttr = l.config.ResAttr
	w.Wrap = l.config.Wrap
	w.Debug = l.config.Debug
	w.Attributes = attr
}

// Buffer returns the internal scroll's Buffer.
func (l *Less) Buffer() *cell.Buffer {
	return l.scroll.Buffer()
}

// Scroll returns the internal scroll. Scroll's public properties
// should not be updated. Use LessConfig instead.
func (l *Less) Scroll() *component.Scroll {
	return l.scroll
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
	l.scroll = component.NewScroll(buf)
	l.initWithBuffer(buf, cfg)
}

// InitWithScroll initialzes this instance with the given main Scroll and configuration.
// If config is nil, the default one is used.
func (l *Less) InitWithScroll(scroll *component.Scroll, cfg LessConfig) {
	l.scroll = scroll
	l.initWithBuffer(scroll.Buffer(), cfg)
}

// NewLess allocates storage and returns a new instance of Less.
func NewLess(cfg LessConfig) *Less {
	l := new(Less)
	l.Init(cfg)
	return l
}

func (l *Less) initWithBuffer(buf *cell.Buffer, cfg LessConfig) {
	l.delEOF = false
	if cfg.ResAttr == (term.Attributes{}) {
		cfg.ResAttr = DefaultLessConfig().ResAttr
	}

	l.config = cfg
	l.searchScroll.Init(cell.NewBuffer())
	l.searchScrollVirt.C = &l.searchScroll

	// initialize message comps
	l.setMessage("")

	l.setupScroll(&l.searchScroll, l.config.BarAttr)
	l.setupScroll(l.scroll, l.config.Attributes)

	l.SetNormalMode()
}
