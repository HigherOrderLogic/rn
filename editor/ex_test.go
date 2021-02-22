package editor

import (
	"sync"
	"testing"

	"github.com/ernestrc/go-tui"
	"github.com/ernestrc/go-tui/browser"
	"github.com/ernestrc/go-tui/cell"
	"github.com/ernestrc/go-tui/term"
	testutil "github.com/ernestrc/go-tui/util/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type browserConstructor func(ed Editor, opts ...Option) (tui.Handler, browser.Browser, error)

type testEditor struct {
	name string
	buf  *cell.Buffer
	subs map[EventType][]EventHandler
}

func (e *testEditor) Handle(ev Event) bool {
	e.dispatchEvent(ev)
	return false
}

func (e *testEditor) dispatchEvent(ev Event) {
	if len(e.subs) == 0 {
		return
	}
	subs, ok := e.subs[ev.Type]
	if !ok {
		return
	}

	remain := make([]EventHandler, 0, len(subs))
	for _, sub := range subs {
		exit := sub.Handle(ev)
		if !exit {
			remain = append(remain, sub)
		}
	}
	e.subs[ev.Type] = remain
}

type testEditorHandler struct {
	browser.TestHandler
	locationList LocationList
}

func (e *testEditor) Edit(name string, buf *cell.Buffer) (Handler, error) {
	e.name = name
	e.buf = buf

	h := &testEditorHandler{TestHandler: *browser.NewTestHandler()}
	e.dispatchEvent(Event{
		Type:         EventTypeOpen,
		ResourceName: name,
		Resource:     h,
		Content:      buf.String(),
	})

	subs := CellSubscriber(name, h, e)
	buf.Subscribe(subs)
	return h, nil
}

func (e *testEditor) SetLocationList(h Handler, id string, loc LocationList) error {
	h.(*testEditorHandler).locationList = loc
	return nil
}

func (e *testEditor) MoveToNextLocation(h Handler, ID string) error {
	return nil
}

func (e *testEditor) MoveToPrevLocation(h Handler, ID string) error {
	return nil
}

func (e *testEditor) SetCursor(h Handler, pos term.Coordinates) error {
	h.(*testEditorHandler).CursorPos = pos
	return nil
}

func (e *testEditor) Cursor(h Handler) (term.Coordinates, error) {
	return h.(*testEditorHandler).CursorPos, nil
}

func (e *testEditor) Writer(h Handler) Writer {
	return CellWriter(e.buf.Writer())
}

func (e *testEditor) Reader(h Handler) Reader {
	return CellReader(e.buf.Reader())
}

func (e *testEditor) Register(cmd string, h CommandHandler) error {
	return nil
}

func (e *testEditor) SubscribeEditor(ev EventType, sub EventHandler) error {
	if e.subs == nil {
		e.subs = make(map[EventType][]EventHandler)
	}
	if _, ok := e.subs[ev]; !ok {
		e.subs[ev] = []EventHandler{sub}
		return nil
	}
	e.subs[ev] = append(e.subs[ev], sub)
	return nil
}

type testFileBuffer struct {
	flushErr error
	closeErr error
}

func (t *testFileBuffer) Flush() error {
	return t.flushErr
}

func (t *testFileBuffer) Close() error {
	return t.closeErr
}

func openTestFile(filePath string, buf *cell.Buffer, swapDir string) (
	flusherCloser, error,
) {
	return &testFileBuffer{}, nil
}

func recoverTestFile(filePath, swapFilePath string, buf *cell.Buffer) (
	flusherCloser, error,
) {
	return openTestFile(filePath, buf, "")
}

func newTestBrowserHandler() *Ex {
	ret := new(Ex)
	ret.comp.openFileFn = openTestFile
	ret.comp.recoverFileFn = recoverTestFile
	return ret
}

func TestBrowserHandlerDraw(t *testing.T) {
	testBrowserHandlerDraw(t, func(ed Editor, opts ...Option) (tui.Handler, browser.Browser, error) {
		b := newTestBrowserHandler()
		err := b.Init(ed, opts...)
		if err != nil {
			return nil, nil, err
		}
		return b, b.Browser(), nil
	})
}

func testBrowserHandlerDraw(t *testing.T, constructor browserConstructor) {
	cases := []testutil.HandlerSequenceTestCase{
		{"asdf",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│                  │
└──────────────────┘`},
		{":",
			`┌──────────────────┐
│                  │
├──────────────────┤
│                  │
│                  │
│                  │
│                  │
│                  │
│▐                 │
└──────────────────┘`},
		{"e cabin.go>",
			`┌──────────────────┐
│cabin.go          │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"a",
			`┌──────────────────┐
│cabin.go          │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":e /tmp/other.go>",
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"#", // simulates ctrl-h
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{"#", // simulates ctrl-h
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"$", // simulates ctrl-l
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{"$", // simulates ctrl-l
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{":bclose>",
			`┌──────────────────┐
│cabin.go          │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":wq!^",
			`┌──────────────────┐
│cabin.go          │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│wq▐               │
└──────────────────┘`},
		{"^^^^",
			`┌──────────────────┐
│cabin.go          │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":<",
			`┌──────────────────┐
│cabin.go          │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":e other.go>1111",
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
		{"$$##",
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
	}

	// testutil.TestHandlerSequence maps ':' characters to the following event
	// this is to work around ex's assumptions on underlying handler.
	commandEvent := term.Event{Type: term.EventKey, Key: term.KeyCtrlBackslash}
	bh, b, err := constructor(&testEditor{},
		WithCommandEvent(commandEvent),
		WithCommandKeyBinding(term.Event{Type: term.EventKey, Ch: '4'}, "close"),
	)
	require.NoError(t, err)

	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	win, err := b.SplitVerticalLeft(browser.NewTestHandler())
	require.NoError(t, err)

	// test window ifc
	focus, err := b.Focus()
	require.NoError(t, err)

	h := browser.NewTestHandler()
	h.Ch = 'Z' // helps identify in tests

	err = b.Subscribe(term.Event{Type: term.EventKey, Ch: '&'},
		browser.FuncEventHandler(func(ev term.Event) bool {
			b.SplitHorizontalBelow(h)
			return false
		}))
	require.NoError(t, err)

	newMappings := map[term.Event]term.Event{
		{Type: term.EventKey, Ch: ')'}: {Type: term.EventKey, Key: term.KeyCtrlL},
		{Type: term.EventKey, Ch: '('}: {Type: term.EventKey, Key: term.KeyCtrlH},
	}
	require.NoError(t, b.MergeKeyMap(newMappings))

	cases = []testutil.HandlerSequenceTestCase{
		{"&_",
			`┌──────────────────┐
│cabin.go  other.go│
├────────┐┌────────┤
│BBBBBBBB││EEEEEEEE│
│BBBBBBBB││EEEEEEEE│
└────────┘│EEEEEEEE│
┌────────┐│EEEEEEEE│
│ZZZZZZZZ││EEEEEEEE│
│ZZZZZZZZ││EEEEEEEE│
└────────┘└────────┘`},
		{":<111111111_",
			`┌──────────────────┐
│cabin.go  other.go│
├────────┐┌────────┤
│BBBBBBBB││EEEEEEEE│
│BBBBBBBB││EEEEEEEE│
└────────┘│EEEEEEEE│
┌────────┐│EEEEEEEE│
│cccccccc││EEEEEEEE│
│cccccccc││EEEEEEEE│
└────────┘└────────┘`},
	}

	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	var unmounted int
	hx := browser.NewTestHandler()
	hx.Ch = '$'
	hx.OnUnmountCallback = func() error { unmounted++; return nil }
	require.NoError(t, focus.SetContent(hx))

	for i := 0; i < 10; i++ {
		content, err := focus.Content()
		require.NoError(t, err)
		require.NoError(t, focus.SetContent(content))
	}

	cases = []testutil.HandlerSequenceTestCase{
		{"_",
			`┌──────────────────┐
│cabin.go  other.go│
├────────┐┌────────┤
│$$$$$$$$││EEEEEEEE│
│$$$$$$$$││EEEEEEEE│
└────────┘│EEEEEEEE│
┌────────┐│EEEEEEEE│
│cccccccc││EEEEEEEE│
│cccccccc││EEEEEEEE│
└────────┘└────────┘`},
	}

	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	require.NoError(t, win.Close())
	require.NoError(t, focus.Close())

	cases = []testutil.HandlerSequenceTestCase{
		// test CommandKeyBindings
		{"4)))",
			`┌──────────────────┐
│cabin.go  other.go│
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":bcloseAll>:e other.go>bcde((((",
			`┌──────────────────┐
│other.go          │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
└──────────────────┘`},
	}
	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	cases = []testutil.HandlerSequenceTestCase{
		{"", `┌──┐
│..│
├EE┤
EEEE`},
	}
	testutil.TestHandlerSequence(t, bh, 4, 4, cases)

	require.NoError(t, b.SetMessage("wasup: %s", "Z"))
	cases = []testutil.HandlerSequenceTestCase{
		{"",
			`┌──────────────────┐
│other.go          │
├──────────────────┤
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│EEEEEEEEEEEEEEEEEE│
│wasup: Z          │
└──────────────────┘`},
	}
	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	nh, err := b.Open("bugz")
	require.NoError(t, err)
	focus, err = b.Focus()
	require.NoError(t, err)
	err = focus.SetContent(nh)
	require.NoError(t, err)

	cases = []testutil.HandlerSequenceTestCase{
		{"b___",
			`┌──────────────────┐
│other.go  bugz    │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":3>",
			`┌──────────────────┐
│other.go  bugz    │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│▐BBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{":0>",
			`┌──────────────────┐
│other.go  bugz    │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
	}
	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	floating1, err := b.FloatingWindow(browser.NewTestHandler(), term.Coordinates{X: 1, Y: 1}, 6, 4)
	require.NoError(t, err)

	// should not be able to split over a floating window, which is currently in focus
	_, err = b.SplitHorizontalAbove(browser.NewTestHandler())
	require.Error(t, err)
	cases = []testutil.HandlerSequenceTestCase{
		{"__",
			`┌──────────────────┐
│other.go  bugz    │
├──────────────────┤
│┌────┐BBBBBBBBBBBB│
││AAAA│BBBBBBBBBBBB│
││AAAA│BBBBBBBBBBBB│
│└────┘BBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
	}

	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	require.NoError(t, floating1.Close())

	cases = []testutil.HandlerSequenceTestCase{
		{"__",
			`┌──────────────────┐
│other.go  bugz    │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
	}
	testutil.TestHandlerSequence(t, bh, 20, 10, cases)

	assert.NoError(t, b.Close())
	assert.Equal(t, 11, unmounted)
}

func assertHandled(
	t *testing.T, h *browser.TestHandler, startingRune rune, exit, handled bool,
) {
	// test handler increments the character that it displays next
	// upon handling a new event
	require.False(t, exit)
	require.True(t, handled)
	assert.NotEqual(t, startingRune, h.Ch)
}

func newBrowserForSubscribeTest(t *testing.T, ev term.Event) (
	*Ex, *browser.TestHandler, rune,
) {
	b := newTestBrowserHandler()
	require.NoError(t, b.Init(&testEditor{}))

	h := browser.NewTestHandler()
	err := b.Browser().Subscribe(ev, browser.HandlerEventHandler(h))
	require.NoError(t, err)

	return b, h, h.Ch
}

func TestBrowserHandlerSubscribe(t *testing.T) {
	ev := term.Event{Type: term.EventKey, Ch: '*'}
	t.Run("proxies event to subscribed EventHandler", func(t *testing.T) {
		b, h, startingRune := newBrowserForSubscribeTest(t, ev)

		exit, handled := b.Handle(ev)
		assertHandled(t, h, startingRune, exit, handled)
	})

	t.Run("returns error on second event Subscribe", func(t *testing.T) {
		b, h, startingRune := newBrowserForSubscribeTest(t, ev)

		h2 := browser.NewTestHandler()
		err := b.Browser().Subscribe(ev, browser.HandlerEventHandler(h2))
		assert.Error(t, err)

		exit, handled := b.Handle(ev)
		assertHandled(t, h, startingRune, exit, handled)
	})

	t.Run("upon handler exit, it unsubscribes EventHandler", func(t *testing.T) {
		b, h, startingRune := newBrowserForSubscribeTest(t, ev)
		h.Exit = true

		exit, handled := b.Handle(ev)
		assertHandled(t, h, startingRune, exit, handled)

		nextRune := h.Ch
		exit, _ = b.Handle(ev)
		assert.False(t, exit)
		assert.Equal(t, nextRune, h.Ch)
	})
}

func TestBrowserHandlerPublishInterrupt(t *testing.T) {
	t.Run("calls interrupt handle asynchronously", func(t *testing.T) {
		var wg sync.WaitGroup
		browser := newTestBrowserHandler()
		require.NoError(t, browser.Init(&testEditor{}))
		browser.comp.interruptDraw = wg.Done

		wg.Add(1)
		browser.Browser().PublishInterrupt()

		wg.Wait()
	})
}

func TestMultipleFilesStartup(t *testing.T) {
	cases := []testutil.HandlerSequenceTestCase{
		{"",
			`┌──────────────────┐
│cabin.go  wi.go   │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
		{"a",
			`┌──────────────────┐
│cabin.go  wi.go   │
├──────────────────┤
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
│BBBBBBBBBBBBBBBBBB│
└──────────────────┘`},
		{"#", // simulates ctrl-h
			`┌──────────────────┐
│cabin.go  wi.go   │
├──────────────────┤
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
│AAAAAAAAAAAAAAAAAA│
└──────────────────┘`},
	}

	b := newTestBrowserHandler()
	err := b.Init(&testEditor{},
		WithFilepath("cabin.go"),
		WithFilepath("wi.go"),
	)
	require.NoError(t, err)

	testutil.TestHandlerSequence(t, b, 20, 10, cases)
}
