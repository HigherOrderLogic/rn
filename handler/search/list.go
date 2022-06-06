package search

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	fzf "github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

const (
	numElementsToReDrawAt    = 128
	maxNumElementsToReDrawAt = 1024 * 32
)

// internal representation of ListConfig
type listConfig struct {
	matchedTextAttr term.Attributes
	matchCountAttr  term.Attributes
	searchBaseAttr  term.Attributes
	textAttr        term.Attributes
	focusAttr       term.Attributes
	algo            fzf.Algo
	interrupt       func()
	caseSensitive   bool
}

// List is a collection of elements that can be interactively searched.
type List struct {
	mu           sync.Mutex
	quitChan     chan struct{}
	dataChan     chan []byte
	input        [][]byte
	searchCtx    context.Context
	cancelSearch func()
	height       int
	width        int

	cfg listConfig

	searchBar struct {
		component.Responsive
		component.Virtual
		minInputHeight int

		syncBuffer *cell.Buffer
		// this cannot be modified from this component
		// or else syncBuffer becomes out of sync.
		// All updates to the search buffer must
		// be performed via Buffer()
		internalRead *cell.Buffer
		dirty        bool
	}

	matchCountBar struct {
		width int
		cell.Buffer
		component.Scroll
		component.Virtual
	}
	list struct {
		component.Virtual
		component.FocusList
	}
}

type searchResultComponent struct {
	*component.AttrSetter
	Match
}

// NewList allocates storage for a new List and initializes it.
// See Init for more details.
func NewList(cfg ListConfig) *List {
	l := new(List)
	l.Init(cfg)
	return l
}

// Init initializes this config with cfg. Close must be called
// when this List is no longer to be used, or before Init
// is to be called again to reset the list.
func (l *List) Init(cfg ListConfig) {
	l.cfg = cfg.toInternal()

	l.searchBar.internalRead = cell.NewBuffer()
	l.searchBar.Responsive = component.BufferResponsive(
		l.searchBar.internalRead, component.StringConfig{
			Attributes:           l.cfg.textAttr,
			BackgroundAttributes: l.cfg.textAttr,
		})
	l.searchBar.C = l.searchBar.Responsive
	l.searchBar.dirty = true
	l.searchBar.syncBuffer = cell.NewBuffer()
	l.searchBar.minInputHeight = 1
	l.searchBar.syncBuffer.Subscribe(syncBuffer{
		parent: l,
		buf:    l.searchBar.internalRead,
	})

	l.matchCountBar.Buffer.Init()
	l.matchCountBar.Scroll.Init(&l.matchCountBar.Buffer)
	l.matchCountBar.C = &l.matchCountBar.Scroll

	l.list.FocusList.InitWithAttr(l.cfg.textAttr, l.cfg.focusAttr)
	l.list.C = &l.list.FocusList

	l.quitChan = make(chan struct{})
	l.dataChan = make(chan []byte)
	l.setFilesCount()

	go l.consumeAsyncElements(l.quitChan)
}

type syncBuffer struct {
	parent *List
	buf    *cell.Buffer
}

func (s syncBuffer) OnWillEdit(start, end term.Coordinates, str string) {
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	s.buf.Edit(start, end, str)
	s.parent.searchBar.dirty = true
	s.parent.asyncSearch()
}

func (s syncBuffer) OnDidEdit(from, to term.Coordinates, old string) {
}

func addMatch(
	list *component.FocusList, match Match, tokens *[]int,
	matchTextAttr term.Attributes,
) {
	matchText := match.data
	b := component.NewSpan(component.String(string(matchText)), component.SpanConfig{
		ContentAlignment: component.SpanAlignmentLeft,
	})

	comp := component.WithAttrSetter(b)
	if tokens != nil {
		for _, t := range *tokens {
			comp.SetAttrAt(term.Coordinates{X: t}, matchTextAttr)
		}
	}

	resComp := searchResultComponent{
		AttrSetter: comp,
		Match:      match,
	}

	list.PushBack(resComp)
}

func sortByResultScore(a, b component.WithAttributes) bool {
	ab := a.(searchResultComponent)
	bb := b.(searchResultComponent)
	if ab.Match.res.Score == bb.Match.res.Score {
		return len(ab.Match.data) < len(bb.Match.data)
	}
	return ab.Match.res.Score > bb.Match.res.Score
}

func (l *List) getSearchQuery() string {
	return l.searchBar.internalRead.String()
}

// MatchCount returns how many elements match the search query so far.
func (l *List) MatchCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.list.Len()
}

// TotalCount returns the total number of elements in the list, whether
// they are matches or not.
func (l *List) TotalCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.input)
}

func (l *List) setFilesCount() {
	l.matchCountBar.Reset()
	str := fmt.Sprintf("%d/%d", l.list.Len(), len(l.input))
	l.matchCountBar.InsertStringWithAttr(term.Coordinates{}, str, l.cfg.matchCountAttr)
}

func (l *List) sortMatchesList() {
	l.list.Sort(sortByResultScore)
	l.setFilesCount()
}

func (l *List) pushData(data []byte, slab *util.Slab, sortList bool) (matched bool) {
	linebuf := [1][]byte{nil}
	linebuf[0] = data

	l.mu.Lock()
	defer l.mu.Unlock()

	l.input = append(l.input, data)

	searchInput := l.getSearchQuery()
	if len(searchInput) == 0 {
		addMatch(&l.list.FocusList, Match{data: data}, nil, l.cfg.matchedTextAttr)
		matched = true
		if sortList {
			l.setFilesCount()
		}
		return
	}

	search(l.cfg.algo, linebuf[:], searchInput, slab, l.cfg.caseSensitive,
		func(match Match, tokens *[]int) bool {
			addMatch(&l.list.FocusList, match, tokens, l.cfg.matchedTextAttr)
			matched = true
			return false
		})

	if sortList {
		l.sortMatchesList()
	}

	return
}

func (l *List) consumeAsyncElements(quitChan chan struct{}) {
	// to handle very large searches, we redraw only every
	// numElementsToReDrawAt to start with, and we double this
	// number every time until we earch maxNumElementsToReDrawAt,
	// at which point we redraw every maxNumElementsToReDrawAt.
	redrawAt := numElementsToReDrawAt
	slab := makeSlab()
	for i := 0; ; i++ {
		select {
		case data, ok := <-l.dataChan:
			if !ok {
				l.mu.Lock()
				if len(l.getSearchQuery()) == 0 {
					l.setFilesCount()
				} else {
					l.sortMatchesList()
				}
				l.mu.Unlock()
				l.cfg.interrupt()
				return
			}
			l.mu.Lock()
			height := l.height
			l.mu.Unlock()
			redraw := i == height-1 || (i != 0 && i%redrawAt == 0)
			l.pushData(data, slab, redraw)
			if redraw {
				l.cfg.interrupt()
				if redrawAt < maxNumElementsToReDrawAt {
					redrawAt *= 2
				}
			}
		case <-quitChan:
			l.mu.Lock()
			defer l.mu.Unlock()
			l.input = l.input[:0]
			return
		}
	}
}

func (l *List) handleSearch(
	ctx context.Context, cancelFn func(), input [][]byte,
	searchInput string,
) {
	slab := makeSlab()
	search(l.cfg.algo, input, searchInput, slab, l.cfg.caseSensitive,
		func(match Match, tokens *[]int) bool {
			select {
			case <-ctx.Done():
				return false
			default:
				// NOTE: this creates a lot of contention when performing queries
				// on very large inputs that are still being collected via Push.
				// search list should be refactor to use on goroutine which takes
				// requests of either: new search (with query + all input), new data, or draw
				// that should be the only goroutine with access to l.list
				l.mu.Lock()
				defer l.mu.Unlock()
				addMatch(&l.list.FocusList, match, tokens, l.cfg.matchedTextAttr)
				return true
			}
		})

	select {
	case <-ctx.Done():
		return
	default:
	}

	l.mu.Lock()
	l.sortMatchesList()
	cancelFn()
	l.mu.Unlock()
	l.cfg.interrupt()
}

// Push returns a channel that can be used to push data to this list asynchronously.
// Clients can and should call close on the channel, once no more data is expected.
// See PushSync for more details.
func (l *List) Push() chan<- []byte {
	return l.dataChan
}

// PushSync pushes one element to this list and searches for a match on it.
// If data matches the search input, this element is appended to the list and
// this method returns true.
func (l *List) PushSync(b []byte) (match bool) {
	return l.pushData(b, nil, true)
}

// FocusUp moves the focus of the match list up.
func (l *List) FocusUp() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.FocusUp()
}

// FocusDown moves the focus of the match list down.
func (l *List) FocusDown() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.FocusDown()
}

// FocusStart moves the focus of the match list to the start.
func (l *List) FocusStart() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.FocusStart()
}

// FocusEnd moves the focus of the match list to the end.
func (l *List) FocusEnd() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.FocusEnd()
}

// Focus returns the match in the list currently in focus.
func (l *List) Focus() ([]byte, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	node, ok := l.list.Focus()
	if !ok {
		return nil, false
	}
	return node.Value().(searchResultComponent).data, true
}

func (l *List) asyncSearch() {
	if l.cancelSearch != nil {
		l.cancelSearch()
	}

	ctx := context.Background()
	l.searchCtx, l.cancelSearch = context.WithCancel(ctx)

	input := make([][]byte, len(l.input))
	copy(input, l.input)
	searchInput := l.getSearchQuery()
	l.list.Reset()
	go l.handleSearch(l.searchCtx, l.cancelSearch, input, searchInput)
}

// DataReset resets the current data list.
func (l *List) DataReset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.input = l.input[:0]
	l.list.Reset()
	l.setFilesCount()
}

// Wait waits for the current search to finish if any and returns.
func (l *List) Wait() {
	l.mu.Lock()
	ctx := l.searchCtx
	l.mu.Unlock()

	if ctx == nil {
		return
	}

	<-ctx.Done()
}

// Draw satisfies tui.Component
func (l *List) Draw(w term.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// if search bar buffer has been modified, re-calculate dimensions
	dirty := l.searchBar.dirty
	if dirty {
		l.resize(l.width, l.height)
	}

	l.list.Virtual.Draw(w)
	l.searchBar.Virtual.Draw(w)
	if l.matchCountBar.width != l.getMatchCountBarWidth() {
		l.resize(l.width, l.height)
	}
	l.matchCountBar.Virtual.Draw(w)
}

// Resize satisfies tui.Component
func (l *List) Resize(width, height int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resize(width, height)
}

func (l *List) resize(width, height int) {
	l.height = height
	l.width = width
	l.searchBar.dirty = false

	inputHeight := l.inputHeight()

	if height-inputHeight < 2 {
		l.list.Move(term.Coordinates{Y: 0})
		l.list.Virtual.Resize(width, height)
		l.searchBar.Virtual.Resize(0, 0)
		l.matchCountBar.Virtual.Resize(0, 0)
		return
	}

	l.list.Move(term.Coordinates{Y: inputHeight})
	l.list.Virtual.Resize(width, height-inputHeight)

	l.searchBar.Virtual.Move(term.Coordinates{})
	l.searchBar.Virtual.Resize(width, inputHeight)
	l.resizeMatchCountBar(inputHeight, width)
}

func (l *List) getMatchCountBarWidth() int {
	return l.matchCountBar.Buffer.Columns(0)
}

func (l *List) resizeMatchCountBar(inputHeight int, width int) {
	lenFilesCounter := l.getMatchCountBarWidth()
	if width-lenFilesCounter <= 0 {
		l.matchCountBar.Virtual.Resize(0, 0)
		return
	}

	l.matchCountBar.width = lenFilesCounter
	l.matchCountBar.Move(term.Coordinates{
		Y: inputHeight - 1,
		X: width - lenFilesCounter,
	})
	l.matchCountBar.Virtual.Resize(lenFilesCounter, 1)
}

func (l *List) inputHeight() int {
	// search bar is used as prompt so always return a min of 1,
	// even if buffer is empty
	return int(math.Max(float64(l.searchBar.minInputHeight),
		float64(l.searchBar.Responsive.Height(l.width))))
}

// InputHeight returns the component.Responsive Height of the input search bar.
func (l *List) InputHeight() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inputHeight()
}

// SetMinInputHeight sets the minimum search input field height.
func (l *List) SetMinInputHeight(height int) {
	if height < 1 {
		panic(fmt.Sprintf("invalid height: %d < 1", height))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.searchBar.minInputHeight = height
	l.resize(l.width, l.height)
}

// Buffer returns the search input buffer.
func (l *List) Buffer() *cell.Buffer {
	return l.searchBar.syncBuffer
}

// Close closes all the resources associated with this List.
func (l *List) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.quitChan == nil {
		return nil
	}
	if l.cancelSearch != nil {
		l.cancelSearch()
	}
	close(l.quitChan)
	l.quitChan = nil
	l.list.Reset()
	return nil
}
