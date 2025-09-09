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

package search

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/junegunn/fzf/src/util"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/debug"
	"unstable.build/go-tui/term"
)

// List is a collection of elements that can be interactively searched.
type List struct {
	mu            sync.Mutex
	quitChan      chan struct{}
	input         [][]byte
	waitSearchCtx context.Context
	cancelSearch  func()
	waitPushCtx   context.Context
	cancelPush    func()
	height        int
	width         int

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

	matchCountBar matchCounter
	list          struct {
		component.Virtual
		component.FocusList
	}
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
	l.searchBar.Responsive = component.Buffer(
		l.searchBar.internalRead, component.StringResponsiveConfig{
			StringConfig: component.StringConfig{
				Attributes:           l.cfg.textAttr,
				BackgroundAttributes: l.cfg.textAttr,
			},
		})
	l.searchBar.C = l.searchBar.Responsive
	l.searchBar.dirty = true
	l.searchBar.syncBuffer = cell.NewBuffer()
	l.searchBar.minInputHeight = 1
	l.searchBar.syncBuffer.Subscribe(syncBuffer{
		parent: l,
		buf:    l.searchBar.internalRead,
	})

	l.matchCountBar.init()

	l.list.FocusList.InitWithAttr(l.cfg.textAttr, l.cfg.focusAttr)
	l.list.FocusList.Inverted = l.cfg.bottomSearchBar
	l.list.C = &l.list.FocusList

	l.setFilesCount()
	l.quitChan = make(chan struct{})
	var ctx context.Context
	ctx, l.cancelSearch = context.WithCancel(context.Background())
	l.waitSearchCtx = ctx
	ctx, l.cancelPush = context.WithCancel(context.Background())
	l.waitPushCtx = ctx
	l.cancelSearch()
	l.cancelPush()
}

// ToggleCaseSensitivity toggles whether the search should be case sensitive or not.
// It returns the previous setting and launches a new search asynchronously.
func (l *List) ToggleCaseSensitivity() bool {
	l.cancelSearch()
	<-l.waitSearchCtx.Done()

	l.mu.Lock()
	defer l.mu.Unlock()
	ret := l.cfg.caseSensitive
	l.cfg.caseSensitive = !l.cfg.caseSensitive
	l.asyncSearch(context.Background())
	return ret
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

// Push returns a channel that can be used to push data to this list asynchronously.
// Clients should call close on the channel if no more data is expected.
// See PushSync for more details.
//
// No more than one goroutine should be calling Push concurrently with the rest
// of methods of this List.
func (l *List) Push(ctx context.Context) chan<- []byte {
	l.cancelPush()
	<-l.waitPushCtx.Done()

	l.mu.Lock()
	defer l.mu.Unlock()

	// we want the cancel from outside to not cancel waitPushCtx
	// because we use that for ensuring that no more data will be pushed
	var cancelWait func()
	l.waitPushCtx, cancelWait = context.WithCancel(ctx)

	// cancel from outside though should cancel the internal one
	ctx, l.cancelPush = context.WithCancel(l.waitPushCtx)

	datachan := make(chan []byte)
	go debug.CapturePanicReport(func() {
		l.consumeAsyncElements(ctx, cancelWait, datachan, l.quitChan)
	})
	return datachan
}

// Pause hints to this list that no more data is expected, for now.
// This should be called when pushing an initial large amount of data
// from an unbound source.
//
// Note that this should NOT be called when there's no more data
// remaining, in which case closing the channel returned by Push is
// more appropiate.
//
// Resuming is as easy as pushing new data to the channel returned by Push.
func (l *List) Pause() {
	// this API is preferable over an automated timer in consumeAsyncElements
	// to avoid adding extra logic to an already contentious and hot path
	l.mu.Lock()
	l.sortMatchesList()
	l.setFilesCount()
	interrupter := l.cfg.interrupter
	l.mu.Unlock()
	_ = interrupter.Interrupt(context.Background())
}

// PushSync pushes one element to this list and searches for a match on it.
// If data matches the search input, this element is appended to the list and
// this method returns true.
func (l *List) PushSync(b []byte) bool {
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
func (l *List) Focus() (Match, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	node, ok := l.list.Focus()
	if !ok {
		return Match{}, false
	}
	comp := node.Value().(component.WithAttributes)
	return comp.(searchResultComponent).Match, true
}

// SetFocus sets the focus of this List to node.
func (l *List) SetFocus(node component.ListNode) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Note: due to performance reasons, Match has to be
	// stack allocated and therefore it canot contain
	// a pointer to its component.ListNode, thus forcing
	// this List's API to take both ListNode and Match
	// rendering it somewhat inconsistent.
	// TODO: benchcmp with heap allocated Match exclusively
	// vs stack allocated but also perform holistic benchmark
	// with GC on a real live session.
	l.list.SetFocus(node)
}

// IterateVisible iterates only the visible elements in l.
func (l *List) IterateVisible(fn func(Match)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list.IterateVisible(func(c component.WithAttributes) {
		fn(c.(searchResultComponent).Match)
	})
}

// DataReset resets the current data list.
func (l *List) DataReset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cancelSearch()
	l.input = l.input[:0]
	l.list.Reset()
	l.setFilesCount()
}

// RemoveFocus removes the item that is currently on focus.
func (l *List) RemoveFocus() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cancelSearch()

	listNode, ok := l.list.Focus()

	if !ok {
		return false
	}

	match := listNode.Value().(searchResultComponent)
	matchIdx := match.idx

	nextNode, _ := listNode.Next()

	// Remove the node on focus and shift focus if possible.
	l.list.Remove(&listNode)

	// Correct the rest of match results indices by shifting them, since we are
	// removing one.
	for nn := nextNode; ok; nn, ok = nn.Next() {
		nv := nn.Value()
		if nv == nil {
			break
		}
		match := nv.(searchResultComponent)
		match.idx--
		nn.SetValue(match)
	}

	// Align the `l.list` with the `l.input`.
	if len(l.input) == 1 {
		l.input = [][]byte{}
	} else {
		l.input = append(
			l.input[:matchIdx],
			l.input[matchIdx+1:]...,
		)
	}

	l.setFilesCount()
	return true
}

// Wait waits for the current search to finish if any and returns.
func (l *List) Wait() {
	l.mu.Lock()
	searchCtx := l.waitSearchCtx
	pushCtx := l.waitPushCtx
	l.mu.Unlock()

	<-searchCtx.Done()
	<-pushCtx.Done()
}

// Cancel cancels the current search if there's any.
//
// Cancel can be used before Wait to ensure that
// we don't wait indefinitely.
func (l *List) Cancel() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cancelSearch()
	l.cancelPush()
}

// Draw satisfies tui.Component
func (l *List) Draw(w term.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.drawList(w)
	l.drawSearchBar(w)
	l.drawMatchCounts(w)
}

// Resize satisfies tui.Component
func (l *List) Resize(width, height int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resize(width, height)
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

// Offset returns this list's current seek offset.
func (l *List) Offset() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.Offset()
}

// FocusOffset returns this list's focus index in the underlying list.
func (l *List) FocusOffset() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.FocusOffset()
}

// ElementHeight returns the height for each element of this list.
func (l *List) ElementHeight() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.list.ElementHeight()
}

// ElementAt returns the Match at the given position and true, if there's any
// or a zero-valued Match and false if there's none.
func (l *List) ElementAt(pos term.Coordinates) (Match, component.ListNode, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// NOTE returning a ListNode solely exist to enable usage of SetFocus. If SetFocus
	// ever uses a Match, this method should be removed.
	node, ok := l.list.ElementAt(pos)
	if !ok {
		return Match{}, component.ListNode{}, false
	}
	return node.Value().(searchResultComponent).Match, node, true
}

// SetFocusAttr sets the attributes of the focus nodes.
// The current focus node is changed and any future focused
// nodes will inherit the given attr.
func (l *List) SetFocusAttr(attr term.Attributes) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list.SetFocusAttr(attr)
}

// Close closes all the resources associated with this List.
func (l *List) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cancelSearch()
	l.list.Reset()
	if l.quitChan == nil {
		return nil
	}
	close(l.quitChan)
	l.quitChan = nil
	return nil
}

func (l *List) inputHeight() int {
	// search bar is used as prompt so always return a min of 1,
	// even if buffer is empty
	return int(math.Max(float64(l.searchBar.minInputHeight),
		float64(l.searchBar.Responsive.Height(l.width))))
}

func (l *List) setFilesCount() {
	doSetFilesCount(&l.matchCountBar, l.list.Len(), len(l.input), l.cfg.matchCountAttr)
}

func (l *List) sortMatchesList() {
	sortMatchesList(l.cfg.bottomSearchBar, &l.list.FocusList)
}

func (l *List) pushData(data []byte, slab *util.Slab, sortList bool) (matched bool) {
	linebuf := [1][]byte{nil}
	linebuf[0] = data

	l.mu.Lock()
	defer l.mu.Unlock()

	l.input = append(l.input, data)

	searchInput := l.getSearchQuery()
	if len(searchInput) == 0 {
		m := Match{data: data, idx: len(l.input) - 1}
		matched = true
		addMatch(&l.list.FocusList, m, l.cfg.textAttr, l.cfg.matchedTextAttr)
		if !sortList {
			return
		}
		if l.cfg.bottomSearchBar {
			l.sortMatchesList()
		}
		l.setFilesCount()
		return
	}

	search(l.cfg.algo, linebuf[:], searchInput, slab, l.cfg.caseSensitive,
		func(match Match) bool {
			match.idx = len(l.input) - 1
			matched = true
			addMatch(&l.list.FocusList, match, l.cfg.textAttr, l.cfg.matchedTextAttr)
			return false
		})

	if sortList {
		l.sortMatchesList()
		l.setFilesCount()
	}

	return
}

func (l *List) consumeAsyncElements(
	ctx context.Context, cancel func(),
	datachan chan []byte, quitChan chan struct{},
) {
	defer cancel()

	t := time.NewTicker(l.cfg.interruptEvery)
	tickerCh := make(chan struct{}) // need a way to signal from below
	defer close(tickerCh)
	defer func() {
		l.mu.Lock()
		if len(l.getSearchQuery()) != 0 {
			l.sortMatchesList()
		}
		l.setFilesCount()
		interrupter := l.cfg.interrupter
		l.mu.Unlock()
		_ = interrupter.Interrupt(context.Background())
	}()

	var dirty bool
	go debug.CapturePanicReport(func() {
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.mu.Lock()
				if !dirty {
					l.mu.Unlock()
					continue
				}
				interrupter := l.cfg.interrupter
				if len(l.getSearchQuery()) != 0 || l.cfg.bottomSearchBar {
					l.sortMatchesList()
				}
				l.setFilesCount()
				dirty = false
				l.mu.Unlock()
				_ = interrupter.Interrupt(context.Background())
			case <-ctx.Done():
				return
			case <-quitChan:
				return
			case <-tickerCh:
				return
			}
		}
	})

	slab := makeSlab()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-datachan:
			if !ok {
				return
			}
			l.pushData(data, slab, false)
			if i%l.cfg.setFileCountEvery == 0 {
				l.mu.Lock()
				l.setFilesCount()
				dirty = true
				l.mu.Unlock()
			}
		case <-quitChan:
			l.DataReset()
			return
		}
	}
}

func (l *List) getSearchQuery() string {
	return l.searchBar.internalRead.String()
}

func (l *List) handleSearch(
	ctx context.Context, cancelFn func(), input [][]byte,
	searchInput string,
) {
	// defer cancel search context so Wait can rely on searchCtx
	// to wait for search to be done. Cleanup resources
	// even if parent ctx.Done returns before the child ctx.
	defer cancelFn()

	// add matches to a goroutine-local focus list
	// to reduce lock contention while pushing matches to the list.
	// Also sort this temp list, rather than the final list,
	// which reduces contention.
	tempList := component.NewFocusList()
	slab := makeSlab()
	search(l.cfg.algo, input, searchInput, slab, l.cfg.caseSensitive,
		func(match Match) bool {
			addMatch(tempList, match, l.cfg.textAttr, l.cfg.matchedTextAttr)
			return true
		})
	sortMatchesList(l.cfg.bottomSearchBar, tempList)

	l.mu.Lock()
	defer l.cfg.interrupter.Interrupt(context.Background()) //nolint:errcheck

	// NOTE: there's a race condition to start pushing matches if we do not push
	// them in batch. Before we start pushing elements, ensure that the
	// context is still active.
	//
	// When a new search is triggered halfway through processing the input, the lock is
	// acquired, and a search is performed above in parallel with all the previosly consumed
	// items, as new items arrive, individual searches are performed by pushData on the
	// new items.
	select {
	case <-ctx.Done():
		// if search was canceled then we're done
		l.mu.Unlock()
		return
	default:
		// if we are holding the lock then nothing else is updating
		// the list or can cancel the search
	}

	for node, ok := tempList.Front(); ok; node, ok = node.Next() {
		l.list.PushBack(node.Value().(component.WithAttributes))
	}
	l.setFilesCount()
	l.mu.Unlock()

}

func (l *List) drawList(w term.Writer) {
	l.list.Virtual.Draw(w)
}

func (l *List) drawSearchBar(w term.Writer) {
	dirty := l.searchBar.dirty
	if dirty {
		l.resize(l.width, l.height)
	}
	l.searchBar.Virtual.Draw(w)
}

func (l *List) drawMatchCounts(w term.Writer) {
	if l.matchCountBar.width != getMatchCountBarWidth(&l.matchCountBar) {
		l.resize(l.width, l.height)
	}
	l.matchCountBar.Virtual.Draw(w)
}

func (l *List) asyncSearch(ctx context.Context) {
	// we want the cancel from outside to not cancel waitSearchCtx
	// because we use that for ensuring that no more data will be pushed
	var cancelWait func()
	l.waitSearchCtx, cancelWait = context.WithCancel(ctx)

	// cancel from outside though should cancel the internal one
	ctx, l.cancelSearch = context.WithCancel(l.waitSearchCtx)

	input := make([][]byte, len(l.input))
	copy(input, l.input)
	searchInput := l.getSearchQuery()
	// reset data now, so moving forward, all the prior input
	// is managed by the goroutine below, and the new items are
	// managed by the Push goroutine
	l.list.Reset()
	go debug.CapturePanicReport(func() {
		l.handleSearch(ctx, cancelWait, input, searchInput)
	})
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

	lenFilesCounter := getMatchCountBarWidth(&l.matchCountBar)
	listHeight := height - inputHeight
	if !l.cfg.bottomSearchBar {
		l.list.Move(term.Coordinates{Y: inputHeight})
		l.list.Virtual.Resize(width, listHeight)

		l.searchBar.Virtual.Move(term.Coordinates{})
		l.searchBar.Virtual.Resize(width, inputHeight)
		resizeMatchCountBar(&l.matchCountBar, inputHeight-1, lenFilesCounter, width)
	} else {
		l.list.Move(term.Coordinates{Y: 0})
		l.list.Virtual.Resize(width, listHeight)

		l.searchBar.Virtual.Move(term.Coordinates{Y: listHeight})
		l.searchBar.Virtual.Resize(width, inputHeight)
		resizeMatchCountBar(&l.matchCountBar, listHeight+inputHeight-1, lenFilesCounter, width)
	}
}

type searchResultComponent struct {
	*component.LazyBytes
	Match
}

type matchCounter struct {
	width int
	cell.Buffer
	component.Scroll
	component.Virtual
}

func (b *matchCounter) init() {
	b.Buffer.Init()
	b.Scroll.Init(&b.Buffer)
	b.C = &b.Scroll
}

type syncBuffer struct {
	parent *List
	buf    *cell.Buffer
}

func (s syncBuffer) OnWillEdit(
	ctx context.Context, start, end term.Coordinates, str string,
) {
	s.parent.cancelSearch()
	<-s.parent.waitSearchCtx.Done()

	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	s.buf.Edit(ctx, start, end, str)
	s.parent.searchBar.dirty = true
	s.parent.asyncSearch(ctx)
}

func (s syncBuffer) OnDidEdit(
	ctx context.Context, from, to term.Coordinates, old string,
) {
}

func sortMatchesList(bottomSearchBar bool, list *component.FocusList) {
	if bottomSearchBar {
		list.Sort(sortByResultScoreInverted)
	} else {
		list.Sort(sortByResultScore)
	}
}

func doSetFilesCount(matchCountBar *matchCounter, matches, total int, attr term.Attributes) {
	matchCountBar.Reset()
	matchCountBar.WriteStringWithAttr(strconv.Itoa(matches), attr)
	matchCountBar.WriteStringWithAttr("/", attr)
	matchCountBar.WriteStringWithAttr(strconv.Itoa(total), attr)
}

func getMatchCountBarWidth(matchCountBar *matchCounter) int {
	return matchCountBar.Buffer.Columns(0)
}

func resizeMatchCountBar(matchCountBar *matchCounter, y, lenFilesCounter, width int) {
	if width-lenFilesCounter <= 0 {
		matchCountBar.Virtual.Resize(0, 0)
		return
	}

	matchCountBar.width = lenFilesCounter
	matchCountBar.Move(term.Coordinates{
		Y: y,
		X: width - lenFilesCounter,
	})
	matchCountBar.Virtual.Resize(lenFilesCounter, 1)
}

func addMatch(
	list *component.FocusList, match Match,
	textAttr, matchTextAttr term.Attributes,
) {
	// this is a very hot path, performance critical
	// when loading large amounts of data into a search list.
	// Use LazyBytes, which defers all allocations until the next
	// call to Draw, this way all components that do not need to
	// be drawn barely imply any allocations (list uses a *Virtual
	// under the hood but that's about it).
	b := component.LazyBytes{Data: match.data, Attributes: textAttr}
	if match.tokens != nil {
		b.Tokens = *match.tokens
		b.TokenAttributes = matchTextAttr
	}
	list.PushBack(searchResultComponent{
		LazyBytes: &b,
		Match:     match,
	})
}

func sortByResultScore(a, b component.WithAttributes) bool {
	ab := a.(searchResultComponent)
	bb := b.(searchResultComponent)
	if ab.Match.res.Score == bb.Match.res.Score {
		return ab.Match.idx < bb.Match.idx
	}
	return ab.Match.res.Score > bb.Match.res.Score
}

func sortByResultScoreInverted(a, b component.WithAttributes) bool {
	ab := a.(searchResultComponent)
	bb := b.(searchResultComponent)
	if ab.Match.res.Score == bb.Match.res.Score {
		return ab.Match.idx < bb.Match.idx
	}
	return ab.Match.res.Score < bb.Match.res.Score
}
