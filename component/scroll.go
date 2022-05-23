package component

import (
	"io"
	"strings"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/term"
)

// Scroll adds Draw to a Buffer along with
// scrolling, searching and wrap-around capabilities.
type Scroll struct {
	buf           *cell.Buffer
	searcher      cell.SubscriberSearcher
	wrapsLen      int
	wraps         map[int]int // int representing absolute y position
	width, height int
	searchText    []rune
	offset        term.Coordinates
	subs          []ScrollSubscriber

	disablePublishing   bool
	lastPublishedOffset term.Coordinates

	// Sets the search result attributes upon matching.
	ResultsAttr term.Attributes
	Attributes  term.Attributes

	// Debug enables seeing visualizing term cells.
	Debug bool

	// lines longer than the width of the scroll wrap around and
	// are rendered in the next line if Wrap is set to true.
	// Wrap invalidates Debug.
	Wrap bool
}

// NewScroll allocates storage for a Scroll and initializes it.
func NewScroll(buf *cell.Buffer) (s *Scroll) {
	s = new(Scroll)
	s.Init(buf)
	return
}

func (s *Scroll) initBuffer(buf *cell.Buffer) {
	s.buf = buf

	searcher := cell.NewSimpleSearcher(s.buf)
	attrSearcher := cell.AttrSearcher(searcher, s.buf, s.ResultsAttr)

	s.searcher = attrSearcher
	s.buf.Subscribe(s.searcher)
}

// Init initializes this scroll with buf.
func (s *Scroll) Init(buf *cell.Buffer) {
	if s.ResultsAttr == (term.Attributes{}) {
		s.ResultsAttr.Fg, s.ResultsAttr.Bg = term.AttrReverse, term.AttrReverse
	}

	// Searcher that actually performs the text search
	s.initBuffer(buf)

	s.wraps = make(map[int]int, 0)
	s.searchText = nil
	s.offset = term.Coordinates{}
	s.searcher.Reset()
}

// CanSeekUp returns true if SeekUp would seek one row up.
func (s *Scroll) CanSeekUp() bool {
	return s.offset.Y > 0
}

// CanSeekDown returns true if SeekDown would seek one row down.
func (s *Scroll) CanSeekDown() bool {
	return s.offset.Y < s.getMaxYOffset()
}

// CanSeekLeft returns true if SeekLeft would seek one column left.
func (s *Scroll) CanSeekLeft() bool {
	return !s.Wrap && s.offset.X > 0
}

// CanSeekRight returns true if SeekRight would seek one column right.
func (s *Scroll) CanSeekRight() bool {
	return !s.Wrap && s.offset.X < s.getMaxXOffset()
}

// SeekUp shifts the contents of this scroll one row up.
func (s *Scroll) SeekUp() (ok bool) {
	if ok = s.CanSeekUp(); ok {
		s.offset.Y--
		s.dispatchSubscribers()
	}
	return ok
}

// SeekDown shifts the contents of this scroll one row down.
func (s *Scroll) SeekDown() (ok bool) {
	if ok = s.CanSeekDown(); ok {
		s.offset.Y++
		s.dispatchSubscribers()
	}
	return
}

// SeekLeft shifts the contents of this scroll one column left.
func (s *Scroll) SeekLeft() (ok bool) {
	if ok = s.CanSeekLeft(); ok {
		s.offset.X--
		s.dispatchSubscribers()
	}
	return
}

// SeekRight shifts the contents of this scroll one column right.
func (s *Scroll) SeekRight() (ok bool) {
	if ok = s.CanSeekRight(); ok {
		s.offset.X++
		s.dispatchSubscribers()
	}
	return
}

// SeekVertical shifts the contents of this scroll such that
// the vertical offset is y. If y is out of bounds the contents
// are shifted to the maximum possible y offset.
func (s *Scroll) SeekVertical(y int) (ok bool) {
	return s.seekVertical(y, true)
}

func (s *Scroll) seekVertical(y int, dispatch bool) (ok bool) {
	if max := s.buf.Rows() - 1; y > max {
		y = max
	}

	if y < 0 {
		y = 0
	}

	ok = s.offset.Y != y
	s.offset.Y = y

	if dispatch && ok {
		s.dispatchSubscribers()
	}

	return
}

// SeekHorizontal shifts the contents of this scroll such that
// the horizontal offset is x. If x is out of bounds the contents
// are shifted to the maximum possible x offset.
func (s *Scroll) SeekHorizontal(x int) (ok bool) {
	return s.seekHorizontal(x, true)
}

func (s *Scroll) seekHorizontal(x int, dispatch bool) (ok bool) {
	if max := s.getMaxXOffset(); x > max {
		x = max
	}
	if x < 0 {
		x = 0
	}

	ok = s.offset.X != x
	s.offset.X = x

	if dispatch && ok {
		s.dispatchSubscribers()
	}

	return
}

// SeekEndLine shifts the contents of this scroll to the maximum x offset.
func (s *Scroll) SeekEndLine() bool {
	return s.SeekHorizontal(s.getMaxXOffset())
}

// SeekStartLine shifts the contents of this scroll to the minimum x offset.
func (s *Scroll) SeekStartLine() bool {
	return s.SeekHorizontal(0)
}

// SeekEndFile shifts the contents of this scroll to the maximum y offset.
func (s *Scroll) SeekEndFile() bool {
	return s.SeekVertical(s.getMaxYOffset())
}

// SeekStartFile shifts the contents of this scroll to the minimum y offset.
func (s *Scroll) SeekStartFile() bool {
	return s.SeekVertical(0)
}

func (s *Scroll) seekTo(pos term.Coordinates, xpadding, ypadding int) bool {
	var yok, xok bool
	if max := s.getMaxXOffset(); xpadding > max {
		xpadding = max
	}
	if max := s.getMaxYOffset(); ypadding > max {
		ypadding = max
	}

	// ypadding < 0 is used to signal seek on the y axis with no padding
	if ypadding == -1 {
		yok = s.seekVertical(pos.Y, false)
	} else if pos.Y >= s.offset.Y+s.height-ypadding {
		yok = s.seekVertical(pos.Y-s.height+ypadding, false)
	} else if pos.Y < s.offset.Y-ypadding {
		yok = s.seekVertical(pos.Y-ypadding, false)
	}

	if pos.X >= s.offset.X+s.width-xpadding {
		xok = s.seekHorizontal(pos.X-s.width+xpadding, false)
	} else if pos.X < s.offset.X-xpadding {
		xok = s.seekHorizontal(pos.X-xpadding, false)
	}

	ok := yok || xok
	if ok {
		s.dispatchSubscribers()
	}
	return ok
}

// SeekTo shifts the contents of this scroll to make sure that pos is in
// range for the next call to Draw. It uses padding and if position is beyond
// the last seekable content, the max is used as the new seek position.
func (s *Scroll) SeekTo(pos term.Coordinates) bool {
	if pos.X < 0 || pos.Y < 0 {
		panic("invalid coordinates: negative")
	}
	return s.seekTo(pos, 2, 2)
}

// SetOffset sets the underlying offset of this scroll. It returns
// false if position is beyond the last seekable content.
func (s *Scroll) SetOffset(pos term.Coordinates) bool {
	if pos.X < 0 || pos.Y < 0 {
		panic("invalid coordinates: negative")
	}
	if max := s.getMaxXOffset(); pos.X > max {
		return false
	}
	if max := s.getMaxYOffset(); pos.Y > max {
		return false
	}

	ok := pos != s.offset
	if ok {
		s.offset = pos
		s.dispatchSubscribers()
	}
	return ok
}

// SeekNextResult shifts the contents of this scroll to visualize
// the next result in the result list.
func (s *Scroll) SeekNextResult() bool {
	pos, ok := s.NextResult()
	if !ok {
		return false
	}

	return s.seekTo(pos, len(s.searchText), -1)
}

// SeekPrevResult shifts the contents of this scroll to visualize
// the previous result in the result list.
func (s *Scroll) SeekPrevResult() bool {
	pos, ok := s.PrevResult()
	if !ok {
		return false
	}

	return s.seekTo(pos, len(s.searchText), -1)
}

// Resize resizes this scroll to fit inside given width and height.
func (s *Scroll) Resize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Scroll) getMaxXOffset() (x int) {
	if s.Wrap {
		return
	}

	columns := s.buf.MaxColumns()
	if columns >= s.width {
		x = columns - s.width + 1
	}
	return
}

func (s *Scroll) getMaxYOffset() (y int) {
	rows := s.buf.Rows() + s.wrapsLen
	if rows >= s.height {
		y = rows - s.height
	}
	return
}

func (s *Scroll) rawCellsOffset() [][]term.Cell {
	cells := s.buf.RawCells()
	if s.offset.Y < len(cells) {
		return cells[s.offset.Y:]
	}
	if len(cells) > 0 {
		return cells[len(cells)-1:]
	}
	return cells[:]
}

func (s *Scroll) drawFast(writer term.Writer) {
	xwindow := s.offset.X + s.width
	ywindow := s.height
	for y, r := range s.rawCellsOffset() {
		if y >= ywindow {
			break
		}
		for x, c := range r {
			if x >= xwindow {
				break
			}
			if c.Ch == 0 || x < s.offset.X {
				continue
			}
			xi := x - s.offset.X
			writer.SetCell(term.Coordinates{X: xi, Y: y}, c)
		}
	}
	return
}

func (s *Scroll) drawDebug(writer term.Writer) {
	xwindow := s.offset.X + s.width
	ywindow := s.height
	for y, r := range s.rawCellsOffset() {
		if y >= ywindow {
			break
		}
		var x int
		var c term.Cell
		for x, c = range r {
			if x >= xwindow {
				break
			}
			if c.Ch == 0 {
				c.Ch = '░'
			}
			if x < s.offset.X {
				continue
			}
			xi := x - s.offset.X
			if c.Bg == 0 {
				c.Bg = s.Attributes.Bg
			}
			if c.Fg == 0 {
				c.Fg = s.Attributes.Fg
			}
			writer.SetCell(term.Coordinates{X: xi, Y: y}, c)
		}
		if x >= xwindow {
			break
		}
		x = x - s.offset.X
		if x != 0 {
			x++
		}
		writer.SetCell(term.Coordinates{X: x, Y: y}, term.Cell{Ch: '¬'})
	}
	return
}

func (s *Scroll) draw(writer term.Writer) {
	xwindow := s.offset.X + s.width
	ywindow := s.height
	for y, r := range s.rawCellsOffset() {
		if y >= ywindow {
			break
		}
		for x, c := range r {
			if x >= xwindow {
				break
			}
			if c.Ch == 0 || x < s.offset.X {
				continue
			}
			xi := x - s.offset.X
			if c.Bg == 0 {
				c.Bg = s.Attributes.Bg
			}
			if c.Fg == 0 {
				c.Fg = s.Attributes.Fg
			}
			writer.SetCell(term.Coordinates{X: xi, Y: y}, c)
		}
	}
	return
}

func (s *Scroll) wrapdrawFast(writer term.Writer) {
	var xi, yi int
	xwindow := s.width
	ywindow := s.height
	s.wraps = make(map[int]int)
	s.wrapsLen = 0
	for y, r := range s.rawCellsOffset() {
		if y >= ywindow || xwindow <= 0 {
			break
		}
		for x, c := range r {
			if c.Ch == 0 {
				continue
			}
			xi = x
			if xi >= xwindow {
				for xi >= xwindow {
					xi -= xwindow
				}
				if xi == 0 {
					s.wraps[y] = s.wraps[y] + 1
					s.wrapsLen++
				}
			}
			yi = y + s.wrapsLen
			if yi >= ywindow {
				break
			}
			writer.SetCell(term.Coordinates{X: xi, Y: yi}, c)
		}
	}

}

func (s *Scroll) wrapdraw(writer term.Writer) {
	var xi, yi int
	xwindow := s.width
	ywindow := s.height
	s.wraps = make(map[int]int)
	s.wrapsLen = 0
	for y, r := range s.rawCellsOffset() {
		if y >= ywindow || xwindow <= 0 {
			break
		}
		for x, c := range r {
			if c.Ch == 0 {
				continue
			}
			xi = x
			if xi >= xwindow {
				for xi >= xwindow {
					xi -= xwindow
				}
				if xi == 0 {
					s.wraps[y] = s.wraps[y] + 1
					s.wrapsLen++
				}
			}
			yi = y + s.wrapsLen
			if yi >= ywindow {
				break
			}
			if c.Bg == 0 {
				c.Bg = s.Attributes.Bg
			}
			if c.Fg == 0 {
				c.Fg = s.Attributes.Fg
			}
			writer.SetCell(term.Coordinates{X: xi, Y: yi}, c)
		}
	}

}

// Draw draws the contents of this scroll to the given writer. If Wrap is set,
// lines that are too long wrap around and thus are rendered in the next line.
func (s *Scroll) Draw(writer term.Writer) {
	if s.Attributes == (term.Attributes{}) {
		if s.Wrap {
			s.wrapdrawFast(writer)
			return
		}

		if s.Debug {
			s.drawDebug(writer)
			return
		}

		s.drawFast(writer)
		return
	}
	if s.Wrap {
		s.wrapdraw(writer)
		return
	}

	if s.Debug {
		s.drawDebug(writer)
		return
	}

	s.draw(writer)
}

// WordAt returns the word at the given position or an empty string if token
// at the given position is not a word. See TokenAt for more details.
func (s *Scroll) WordAt(pos term.Coordinates) (
	term.Coordinates, term.Coordinates, string,
) {
	return s.TokenAt(pos, func(c rune) bool {
		return (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') || c == '_' ||
			(c >= '0' && c <= '9')
	})
}

// TokenAt returns the token that satisfies the isAllowed function
// and its start and end positions.
//
// It returns an empty string if the token at the given position does
// not satisfy isAllowed.
func (s *Scroll) TokenAt(pos term.Coordinates, isAllowed func(rune) bool) (
	term.Coordinates, term.Coordinates, string,
) {
	var b strings.Builder
	cells := s.buf.RawCells()
	rows := s.buf.Rows()

	start := pos
	for start.Y < rows && start.X < s.buf.Columns(start.Y) && start.X >= 0 {
		c := cells[start.Y][start.X]
		if isAllowed(c.Ch) {
			start.X--
			continue
		}
		break
	}

	if start == pos {
		return start, start, b.String()
	}

	start.X++
	end := start
	for end.Y < rows && end.X < s.buf.Columns(end.Y) && end.X >= 0 {
		c := cells[end.Y][end.X]
		if isAllowed(c.Ch) {
			end.X++
			b.WriteRune(c.Ch)
			continue
		}
		break
	}

	return start, end, b.String()
}

// Search performs a text search of text in the internal cell buffer. It populates
// a search list so SeekNextResult and SeekPreviousResult can be used to visualize results.
// It returns the number of matches found. Note that it also sets the attributes of
// the matching terms as defined by ResultsAttr.
func (s *Scroll) Search(text string) (n int) {
	s.searchText = []rune(text)
	return s.searcher.Search(text)
}

// PrevResult returns the coordinates of the previous result in the Search list.
func (s *Scroll) PrevResult() (pos term.Coordinates, ok bool) {
	return s.searcher.PrevResult()
}

// NextResult returns the coordinates of the next result in the Search list.
func (s *Scroll) NextResult() (pos term.Coordinates, ok bool) {
	return s.searcher.NextResult()
}

// Result returns the current search result's coordinates.
func (s *Scroll) Result() (pos term.Coordinates, ok bool) {
	return s.searcher.Result()
}

// Offset returns the scroll offset from the start of the content.
func (s *Scroll) Offset() term.Coordinates {
	return s.offset
}

// Width returns this scroll's width.
func (s *Scroll) Width() int {
	return s.width
}

// Height returns this scroll's height.
func (s *Scroll) Height() int {
	return s.height
}

// Wraps returns a the wraps visible in last call to Draw,
// indexed by their Y coordinate.
// If Draw has not been called yet, then this method returns an
// empty map.
func (s *Scroll) Wraps() map[int]int {
	ret := make(map[int]int, len(s.wraps))
	for y, wraps := range s.wraps {
		ret[y] = wraps
	}
	return ret
}

// Buffer returns s internal Buffer.
func (s *Scroll) Buffer() *cell.Buffer {
	return s.buf
}

// ReadFrom see cell.Buffer.ReadFrom.
func (s *Scroll) ReadFrom(r io.Reader) (n int64, err error) {
	return s.buf.ReadFrom(r)
}

// ScrollSubscriber is a subscriber of seek operations in a scroll.
type ScrollSubscriber interface {
	OnSeek(offset term.Coordinates)
}

type fnSubscriber func(term.Coordinates)

func (s fnSubscriber) OnSeek(offset term.Coordinates) {
	s(offset)
}

func (s *Scroll) dispatchSubscribers() {
	if s.disablePublishing {
		return
	}
	s.lastPublishedOffset = s.offset
	for _, sub := range s.subs {
		sub.OnSeek(s.offset)
	}
}

// Subscribe subscribes sub to seek operations.
func (s *Scroll) Subscribe(sub ScrollSubscriber) {
	s.subs = append(s.subs, sub)
}

// DisablePublishing disables dispatching OnSeek calls to subscribers.
// This is useful when clients of Scroll perform composite moves that
// would otherwise dispatch multiple calls rather than one.
func (s *Scroll) DisablePublishing() {
	s.disablePublishing = true
}

// EnablePublishing enables dispatching OnSeek calls after a call to DisablePublishing.
// It dispatches an OnSeek call to each subscriber if the offset has changed since
// last time publishing was disabled.
func (s *Scroll) EnablePublishing() {
	s.disablePublishing = false
	if s.lastPublishedOffset != s.Offset() {
		s.dispatchSubscribers()
	}
}

// PublishingEnabled returns whether publishing has been
// enabled with EnablePublishing, or disabled with DisablePublishing. By default
// it is enabled when Scroll is initialized.
func (s *Scroll) PublishingEnabled() bool {
	return !s.disablePublishing
}

// FuncScrollSubscriber wraps fn to satisfy ScrollSubscriber.
func FuncScrollSubscriber(fn func(term.Coordinates)) ScrollSubscriber {
	return fnSubscriber(fn)
}
