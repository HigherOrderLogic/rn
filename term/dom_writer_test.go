package term

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	code int
	tpe  string
}

func (e testEvent) KeyCode() int {
	return e.code
}

func (e testEvent) Type() string {
	return e.tpe
}

type testEventEmitter struct {
	listeners map[string]domEventHandler
}

func (e *testEventEmitter) AddEventListener(ev string, h domEventHandler) {
	e.listeners[ev] = h
}

func (e *testEventEmitter) emit(tpe string, ev jsEvent) {
	e.listeners[tpe](ev)
}

type testJsValue struct {
	returnGet map[string]jsValue
	returnInt int

	interceptGet string
	interceptSet struct {
		name  string
		value interface{}
	}
}

func (v *testJsValue) Get(name string) jsValue {
	v.interceptGet = name
	r, ok := v.returnGet[name]
	if ok {
		return r
	}
	return nil
}

func (v *testJsValue) Int() int {
	return v.returnInt
}

func (v *testJsValue) Set(name string, value interface{}) {
	v.interceptSet.name = name
	v.interceptSet.value = value
}

func newBodyJsValue(width, height int) *testJsValue {
	return &testJsValue{
		returnGet: map[string]jsValue{
			domPropertyHeight: &testJsValue{
				returnInt: height,
			},
			domPropertyWidth: &testJsValue{
				returnInt: width,
			},
		},
	}
}

func newTermJsValue() *testJsValue {
	return &testJsValue{}
}

func newTestEventEmitter() *testEventEmitter {
	return &testEventEmitter{listeners: make(map[string]domEventHandler)}
}

func newTestDomWriter(widthInPixels, heightInPixels int) (
	w *domWriter, emitter *testEventEmitter,
	body, term *testJsValue,
) {
	emitter = newTestEventEmitter()
	body = newBodyJsValue(widthInPixels, heightInPixels)
	term = newTermJsValue()
	w = newDomWriter(emitter, body, term, "")
	return
}

func TestDomWriterSetAttr(t *testing.T) {
	w, _, _, _ := newTestDomWriter(1, 1)
	attr := Attributes{Bg: ColorBlack, Fg: ColorWhite | AttrBold}
	w.SetAttr(attr)
	assert.Equal(t, attr, w.Attr())
}

func TestDomWriterInterrupts(t *testing.T) {
	w, _, _, _ := newTestDomWriter(1, 1)

	evFns := []func(){w.SendNoneEvent, w.Interrupt}
	var evs []Event
	for _, evFn := range evFns {
		go evFn()
		ev := w.PollEvent()
		evs = append(evs, ev)
	}

	assert.Equal(t, EventNone, evs[0].Type)
	assert.Equal(t, EventInterrupt, evs[1].Type)
}

func TestDomWriterSize(t *testing.T) {
	tsuite := []struct {
		description    string
		heightInPixels int
		widthInPixels  int

		expectedWidthInCells  int
		expectedHeightInCells int
	}{
		{"converts pixels to cells", 100, 100, 16, 8},
		{"floors to guarantee that no cells are out of bounds", 95, 95, 15, 7},
		{"never returns 0 cells", 10, 10, 1, 1},
		{"does not panic on 0 height or width", 0, 0, 1, 1},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase
		t.Run(tcase.description, func(t *testing.T) {
			w, _, _, _ := newTestDomWriter(tcase.widthInPixels, tcase.heightInPixels)
			width, height := w.Size()
			assert.Equal(t, tcase.expectedWidthInCells, width)
			assert.Equal(t, tcase.expectedHeightInCells, height)
		})
	}
}

func TestDomWriterSetCell(t *testing.T) {
	w, _, _, _ := newTestDomWriter(0, 0)
	w.SetCell(Coordinates{}, Cell{Ch: 'X'})
	require.Equal(t, 'X', w.w.cellbuf[0].Ch)
}

func TestDomWriterSetCursor(t *testing.T) {
	w, _, _, _ := newTestDomWriter(100, 100)
	assert.Equal(t, 0, w.w.cursor)
	w.SetCursor(Coordinates{X: 1})
	assert.Equal(t, 1, w.w.cursor)
}

func TestDomWriterFlush(t *testing.T) {
	w, _, _, termElement := newTestDomWriter(50, 50)
	w.Flush()
	assert.Equal(t, "innerHTML", termElement.interceptSet.name)
	require.NotNil(t, termElement.interceptSet.value)
	str, ok := termElement.interceptSet.value.(string)
	require.True(t, ok)
	assert.NotZero(t, str)
}

func TestDomWriterClose(t *testing.T) {
	w, _, _, _ := newTestDomWriter(0, 0)
	go w.Interrupt()
	w.PollEvent()

	w.Close()
	assert.Panics(t, func() {
		w.Interrupt()
	})
	ev := w.PollEvent()
	assert.Equal(t, EventError, ev.Type)
	assert.Error(t, ev.Err)
}

func TestDomWriterEventDispatching(t *testing.T) {
	// FIXME: tcell model migration broke this test
	t.SkipNow()
	testHeight, testWidth := 100, 100
	tsuite := []struct {
		description string
		in          []testEvent
		out         []Event
	}{
		{"resize event", []testEvent{
			{tpe: domEventResize},
		}, []Event{
			{Type: EventResize, Width: 16, Height: 8},
		}},
		{"single character key press", []testEvent{
			{tpe: domEventKeyUp, code: 65},
			{tpe: domEventKeyDown, code: 65},
		}, []Event{
			{Type: EventKey, Ch: 'a'},
		}},
		{"multiple character key presses", []testEvent{
			{tpe: domEventKeyDown, code: 66},
			{tpe: domEventKeyUp, code: 66},
			{tpe: domEventKeyDown, code: 67},
			{tpe: domEventKeyUp, code: 67},
		}, []Event{
			{Type: EventKey, Ch: 'b'},
			{Type: EventKey, Ch: 'c'},
		}},
		{"uppercase character key presses", []testEvent{
			{tpe: domEventKeyDown, code: int(modShift)},
			{tpe: domEventKeyDown, code: 66},
			{tpe: domEventKeyUp, code: 66},
			{tpe: domEventKeyDown, code: 67},
			{tpe: domEventKeyUp, code: 67},
			{tpe: domEventKeyUp, code: int(modShift)},
		}, []Event{
			{Type: EventKey, Ch: 'B'},
			{Type: EventKey, Ch: 'C'},
		}},
		{"alt mod character key presses", []testEvent{
			{tpe: domEventKeyDown, code: int(ModAlt)},
			{tpe: domEventKeyDown, code: 66},
			{tpe: domEventKeyUp, code: 66},
			{tpe: domEventKeyDown, code: 67},
			{tpe: domEventKeyUp, code: 67},
			{tpe: domEventKeyUp, code: int(ModAlt)},
		}, []Event{
			{Type: EventKey, Ch: 'b', Mod: ModAlt},
			{Type: EventKey, Ch: 'c', Mod: ModAlt},
		}},
		{"number key press", []testEvent{
			{tpe: domEventKeyDown, code: 53},
			{tpe: domEventKeyUp, code: 53},
		}, []Event{
			{Type: EventKey, Ch: '5'},
		}},
		{"special key press", []testEvent{
			{tpe: domEventKeyDown, code: 192},
			{tpe: domEventKeyUp, code: 192},
		}, []Event{
			{Type: EventKey, Ch: '`'},
		}},
		{"key presses that the code is equal to the key constant", []testEvent{
			{tpe: domEventKeyDown, code: int(KeyF1)}, {tpe: domEventKeyUp, code: int(KeyF1)},
			{tpe: domEventKeyDown, code: int(KeyF2)}, {tpe: domEventKeyUp, code: int(KeyF2)},
			{tpe: domEventKeyDown, code: int(KeyF3)}, {tpe: domEventKeyUp, code: int(KeyF3)},
			{tpe: domEventKeyDown, code: int(KeyF4)}, {tpe: domEventKeyUp, code: int(KeyF4)},
			{tpe: domEventKeyDown, code: int(KeyF5)}, {tpe: domEventKeyUp, code: int(KeyF5)},
			{tpe: domEventKeyDown, code: int(KeyF6)}, {tpe: domEventKeyUp, code: int(KeyF6)},
			{tpe: domEventKeyDown, code: int(KeyF7)}, {tpe: domEventKeyUp, code: int(KeyF7)},
			{tpe: domEventKeyDown, code: int(KeyF8)}, {tpe: domEventKeyUp, code: int(KeyF8)},
			{tpe: domEventKeyDown, code: int(KeyF9)}, {tpe: domEventKeyUp, code: int(KeyF9)},
			{tpe: domEventKeyDown, code: int(KeyF10)}, {tpe: domEventKeyUp, code: int(KeyF10)},
			{tpe: domEventKeyDown, code: int(KeyF11)}, {tpe: domEventKeyUp, code: int(KeyF11)},
			{tpe: domEventKeyDown, code: int(KeyF12)}, {tpe: domEventKeyUp, code: int(KeyF12)},
			{tpe: domEventKeyDown, code: int(KeyInsert)}, {tpe: domEventKeyUp, code: int(KeyInsert)},
			{tpe: domEventKeyDown, code: int(KeyDelete)}, {tpe: domEventKeyUp, code: int(KeyDelete)},
			{tpe: domEventKeyDown, code: int(KeyHome)}, {tpe: domEventKeyUp, code: int(KeyHome)},
			{tpe: domEventKeyDown, code: int(KeyEnd)}, {tpe: domEventKeyUp, code: int(KeyEnd)},
			{tpe: domEventKeyDown, code: int(KeyPgup)}, {tpe: domEventKeyUp, code: int(KeyPgup)},
			{tpe: domEventKeyDown, code: int(KeyPgdn)}, {tpe: domEventKeyUp, code: int(KeyPgdn)},
			{tpe: domEventKeyDown, code: int(KeyArrowUp)}, {tpe: domEventKeyUp, code: int(KeyArrowUp)},
			{tpe: domEventKeyDown, code: int(KeyArrowDown)}, {tpe: domEventKeyUp, code: int(KeyArrowDown)},
			{tpe: domEventKeyDown, code: int(KeyArrowLeft)}, {tpe: domEventKeyUp, code: int(KeyArrowLeft)},
			{tpe: domEventKeyDown, code: int(KeyArrowRight)}, {tpe: domEventKeyUp, code: int(KeyArrowRight)},
			{tpe: domEventKeyDown, code: int(KeyBackspace)}, {tpe: domEventKeyUp, code: int(KeyBackspace)},
			{tpe: domEventKeyDown, code: int(KeyTab)}, {tpe: domEventKeyUp, code: int(KeyTab)},
			{tpe: domEventKeyDown, code: int(KeyEsc)}, {tpe: domEventKeyUp, code: int(KeyEsc)},
			{tpe: domEventKeyDown, code: int(KeyBackspace2)}, {tpe: domEventKeyUp, code: int(KeyBackspace2)},
			{tpe: domEventKeyDown, code: int(KeyEnter)}, {tpe: domEventKeyUp, code: int(KeyEnter)},
			{tpe: domEventKeyDown, code: int(KeySpace)}, {tpe: domEventKeyUp, code: int(KeySpace)},
		}, []Event{
			{Type: EventKey, Key: KeyF1},
			{Type: EventKey, Key: KeyF2},
			{Type: EventKey, Key: KeyF3},
			{Type: EventKey, Key: KeyF4},
			{Type: EventKey, Key: KeyF5},
			{Type: EventKey, Key: KeyF6},
			{Type: EventKey, Key: KeyF7},
			{Type: EventKey, Key: KeyF8},
			{Type: EventKey, Key: KeyF9},
			{Type: EventKey, Key: KeyF10},
			{Type: EventKey, Key: KeyF11},
			{Type: EventKey, Key: KeyF12},
			{Type: EventKey, Key: KeyInsert},
			{Type: EventKey, Key: KeyDelete},
			{Type: EventKey, Key: KeyHome},
			{Type: EventKey, Key: KeyEnd},
			{Type: EventKey, Key: KeyPgup},
			{Type: EventKey, Key: KeyPgdn},
			{Type: EventKey, Key: KeyArrowUp},
			{Type: EventKey, Key: KeyArrowDown},
			{Type: EventKey, Key: KeyArrowLeft},
			{Type: EventKey, Key: KeyArrowRight},
			{Type: EventKey, Key: KeyBackspace},
			{Type: EventKey, Key: KeyTab},
			{Type: EventKey, Key: KeyEsc},
			{Type: EventKey, Key: KeyBackspace2},
			{Type: EventKey, Key: KeyEnter},
			{Type: EventKey, Key: KeySpace},
		}},
		{"ctrl dedicated key press", []testEvent{
			{tpe: domEventKeyDown, code: int(modCtrl)},
			{tpe: domEventKeyDown, code: 192}, {tpe: domEventKeyUp, code: 192},
			{tpe: domEventKeyDown, code: 50}, {tpe: domEventKeyUp, code: 50},
			{tpe: domEventKeyDown, code: 32}, {tpe: domEventKeyUp, code: 32},
			{tpe: domEventKeyDown, code: 65}, {tpe: domEventKeyUp, code: 65},
			{tpe: domEventKeyDown, code: 66}, {tpe: domEventKeyUp, code: 66},
			{tpe: domEventKeyDown, code: 67}, {tpe: domEventKeyUp, code: 67},
			{tpe: domEventKeyDown, code: 68}, {tpe: domEventKeyUp, code: 68},
			{tpe: domEventKeyDown, code: 69}, {tpe: domEventKeyUp, code: 69},
			{tpe: domEventKeyDown, code: 70}, {tpe: domEventKeyUp, code: 70},
			{tpe: domEventKeyDown, code: 71}, {tpe: domEventKeyUp, code: 71},
			{tpe: domEventKeyDown, code: 72}, {tpe: domEventKeyUp, code: 72},
			{tpe: domEventKeyDown, code: 73}, {tpe: domEventKeyUp, code: 73},
			{tpe: domEventKeyDown, code: 74}, {tpe: domEventKeyUp, code: 74},
			{tpe: domEventKeyDown, code: 75}, {tpe: domEventKeyUp, code: 75},
			{tpe: domEventKeyDown, code: 76}, {tpe: domEventKeyUp, code: 76},
			{tpe: domEventKeyDown, code: 77}, {tpe: domEventKeyUp, code: 77},
			{tpe: domEventKeyDown, code: 78}, {tpe: domEventKeyUp, code: 78},
			{tpe: domEventKeyDown, code: 79}, {tpe: domEventKeyUp, code: 79},
			{tpe: domEventKeyDown, code: 80}, {tpe: domEventKeyUp, code: 80},
			{tpe: domEventKeyDown, code: 81}, {tpe: domEventKeyUp, code: 81},
			{tpe: domEventKeyDown, code: 82}, {tpe: domEventKeyUp, code: 82},
			{tpe: domEventKeyDown, code: 83}, {tpe: domEventKeyUp, code: 83},
			{tpe: domEventKeyDown, code: 84}, {tpe: domEventKeyUp, code: 84},
			{tpe: domEventKeyDown, code: 85}, {tpe: domEventKeyUp, code: 85},
			{tpe: domEventKeyDown, code: 86}, {tpe: domEventKeyUp, code: 86},
			{tpe: domEventKeyDown, code: 87}, {tpe: domEventKeyUp, code: 87},
			{tpe: domEventKeyDown, code: 88}, {tpe: domEventKeyUp, code: 88},
			{tpe: domEventKeyDown, code: 89}, {tpe: domEventKeyUp, code: 89},
			{tpe: domEventKeyDown, code: 90}, {tpe: domEventKeyUp, code: 90},
			{tpe: domEventKeyDown, code: 219}, {tpe: domEventKeyUp, code: 219},
			{tpe: domEventKeyDown, code: 51}, {tpe: domEventKeyUp, code: 51},
			{tpe: domEventKeyDown, code: 52}, {tpe: domEventKeyUp, code: 52},
			{tpe: domEventKeyDown, code: 220}, {tpe: domEventKeyUp, code: 220},
			{tpe: domEventKeyDown, code: 53}, {tpe: domEventKeyUp, code: 53},
			{tpe: domEventKeyDown, code: 221}, {tpe: domEventKeyUp, code: 221},
			{tpe: domEventKeyDown, code: 54}, {tpe: domEventKeyUp, code: 54},
			{tpe: domEventKeyDown, code: 55}, {tpe: domEventKeyUp, code: 55},
			{tpe: domEventKeyDown, code: 191}, {tpe: domEventKeyUp, code: 191},
			{tpe: domEventKeyDown, code: 56}, {tpe: domEventKeyUp, code: 56},
			{tpe: domEventKeyDown, code: 189}, {tpe: domEventKeyUp, code: 189},
			{tpe: domEventKeyUp, code: int(modCtrl)},
		}, []Event{
			{Type: EventKey, Key: KeyCtrlTilde},
			{Type: EventKey, Key: KeyCtrl2},
			{Type: EventKey, Key: KeyCtrlSpace},
			{Type: EventKey, Key: KeyCtrlA},
			{Type: EventKey, Key: KeyCtrlB},
			{Type: EventKey, Key: KeyCtrlC},
			{Type: EventKey, Key: KeyCtrlD},
			{Type: EventKey, Key: KeyCtrlE},
			{Type: EventKey, Key: KeyCtrlF},
			{Type: EventKey, Key: KeyCtrlG},
			{Type: EventKey, Key: KeyCtrlH},
			{Type: EventKey, Key: KeyCtrlI},
			{Type: EventKey, Key: KeyCtrlJ},
			{Type: EventKey, Key: KeyCtrlK},
			{Type: EventKey, Key: KeyCtrlL},
			{Type: EventKey, Key: KeyCtrlM},
			{Type: EventKey, Key: KeyCtrlN},
			{Type: EventKey, Key: KeyCtrlO},
			{Type: EventKey, Key: KeyCtrlP},
			{Type: EventKey, Key: KeyCtrlQ},
			{Type: EventKey, Key: KeyCtrlR},
			{Type: EventKey, Key: KeyCtrlS},
			{Type: EventKey, Key: KeyCtrlT},
			{Type: EventKey, Key: KeyCtrlU},
			{Type: EventKey, Key: KeyCtrlV},
			{Type: EventKey, Key: KeyCtrlW},
			{Type: EventKey, Key: KeyCtrlX},
			{Type: EventKey, Key: KeyCtrlY},
			{Type: EventKey, Key: KeyCtrlZ},
			{Type: EventKey, Key: KeyCtrlLsqBracket},
			{Type: EventKey, Key: KeyCtrl3},
			{Type: EventKey, Key: KeyCtrl4},
			{Type: EventKey, Key: KeyCtrlBackslash},
			{Type: EventKey, Key: KeyCtrl5},
			{Type: EventKey, Key: KeyCtrlRsqBracket},
			{Type: EventKey, Key: KeyCtrl6},
			{Type: EventKey, Key: KeyCtrl7},
			{Type: EventKey, Key: KeyCtrlSlash},
			{Type: EventKey, Key: KeyCtrl8},
			{Type: EventKey, Key: KeyCtrlUnderscore},
		}},
		{"alt+shift+ctrl is ignored but ctrl prevails", []testEvent{
			{tpe: domEventKeyDown, code: int(modCtrl)},
			{tpe: domEventKeyDown, code: int(ModAlt)},
			{tpe: domEventKeyDown, code: int(modShift)},
			{tpe: domEventKeyDown, code: 66},
			{tpe: domEventKeyUp, code: 66},
			{tpe: domEventKeyUp, code: int(modCtrl)},
			{tpe: domEventKeyUp, code: int(modShift)},
			{tpe: domEventKeyUp, code: int(ModAlt)},
			{tpe: domEventKeyDown, code: int(modShift)},
			{tpe: domEventKeyDown, code: int(ModAlt)},
			{tpe: domEventKeyDown, code: int(modCtrl)},
			{tpe: domEventKeyDown, code: 67},
			{tpe: domEventKeyUp, code: 67},
			{tpe: domEventKeyUp, code: int(modShift)},
			{tpe: domEventKeyUp, code: int(modCtrl)},
			{tpe: domEventKeyUp, code: int(ModAlt)},
		}, []Event{
			{Type: EventKey, Key: KeyCtrlB},
			{Type: EventKey, Key: KeyCtrlC},
		}},
		{"ctrl key down then up, cleans state properly", []testEvent{
			{tpe: domEventKeyDown, code: int(modCtrl)},
			{tpe: domEventKeyDown, code: 68},
			{tpe: domEventKeyUp, code: int(modCtrl)},
			{tpe: domEventKeyUp, code: 68},
			{tpe: domEventKeyDown, code: 68},
			{tpe: domEventKeyUp, code: 68},
		}, []Event{
			{Type: EventKey, Key: KeyCtrlD},
			{Type: EventKey, Ch: 'd'},
		}},
		{"shift key down then up, cleans state properly", []testEvent{
			{tpe: domEventKeyDown, code: int(modShift)},
			{tpe: domEventKeyDown, code: 68},
			{tpe: domEventKeyUp, code: 68},
			{tpe: domEventKeyUp, code: int(modShift)},
			{tpe: domEventKeyDown, code: 68},
			{tpe: domEventKeyUp, code: 68},
		}, []Event{
			{Type: EventKey, Ch: 'D'},
			{Type: EventKey, Ch: 'd'},
		}},
		{"alt key down then up, cleans state properly", []testEvent{
			{tpe: domEventKeyDown, code: int(ModAlt)},
			{tpe: domEventKeyDown, code: 68},
			{tpe: domEventKeyUp, code: 68},
			{tpe: domEventKeyUp, code: int(ModAlt)},
			{tpe: domEventKeyDown, code: 68},
			{tpe: domEventKeyUp, code: 68},
		}, []Event{
			{Type: EventKey, Ch: 'd', Mod: ModAlt},
			{Type: EventKey, Ch: 'd'},
		}},
		{"unknown ctrl code", []testEvent{
			{tpe: domEventKeyDown, code: int(modCtrl)},
			{tpe: domEventKeyDown, code: 187},
			{tpe: domEventKeyUp, code: int(modCtrl)},
			{tpe: domEventKeyUp, code: 187},
		}, []Event{
			{Type: EventKey, Ch: '='},
		}},
		{"unknown shift code", []testEvent{
			{tpe: domEventKeyDown, code: int(modShift)},
			{tpe: domEventKeyDown, code: 255},
			{tpe: domEventKeyUp, code: int(modShift)},
			{tpe: domEventKeyUp, code: 255},
		}, nil},
	}

	for _, _tcase := range tsuite {
		tcase := _tcase
		t.Run(tcase.description, func(t *testing.T) {
			w, emitter, _, _ := newTestDomWriter(testWidth, testHeight)

			go func() {
				for _, in := range tcase.in {
					emitter.emit(in.tpe, in)
				}

				// forces writer to emit interrupt
				// so we know it's done processing. This
				// avoids this test from hanging if number
				// of expected vs emitted events do not match.
				w.Interrupt()
			}()

			var out []Event
			for range tcase.out {
				ev := w.PollEvent()
				if ev.Type == EventInterrupt {
					break
				}
				out = append(out, ev)
			}

			assert.EqualValues(t, tcase.out, out)
		})
	}
}

func TestFuzzDomWriterEventMapping(t *testing.T) {
	w, emitter, _, _ := newTestDomWriter(10, 10)

	go func() {
		for i := 0; i < math.MaxInt16; i++ {
			ev := w.PollEvent()
			switch ev.Type {
			case EventError:
				t.Error(ev.Err)
				return
			case EventInterrupt:
				return
			default:
			}
		}
	}()

	// testing if it panics
	for i := 0; i < math.MaxInt16; i++ {
		emitter.emit("keydown", testEvent{tpe: "keydown", code: i})
	}
	emitter.emit("keydown", testEvent{tpe: "keydown", code: math.MaxInt32})
	emitter.emit("keydown", testEvent{tpe: "keydown", code: math.MaxInt64})
	w.Interrupt()
}

func BenchmarkDomWriterEventMapping(b *testing.B) {
	w, emitter, _, _ := newTestDomWriter(100000, 100000)
	events := []testEvent{
		{tpe: domEventKeyDown, code: int(modCtrl)},
		{tpe: domEventKeyDown, code: int(ModAlt)},
		{tpe: domEventKeyDown, code: int(modShift)},
		{tpe: domEventKeyDown, code: 66},
		{tpe: domEventKeyUp, code: 66},
		{tpe: domEventKeyUp, code: int(modCtrl)},
		{tpe: domEventKeyUp, code: int(modShift)},
		{tpe: domEventKeyUp, code: int(ModAlt)},
		{tpe: domEventKeyDown, code: int(modShift)},
		{tpe: domEventKeyDown, code: int(ModAlt)},
		{tpe: domEventKeyDown, code: int(modCtrl)},
		{tpe: domEventKeyDown, code: 67},
		{tpe: domEventKeyUp, code: 67},
		{tpe: domEventKeyUp, code: int(modShift)},
		{tpe: domEventKeyUp, code: int(modCtrl)},
		{tpe: domEventKeyUp, code: int(ModAlt)},
	}

	go func() {
		for i := 0; i < (b.N * len(events)); i++ {
			ev := w.PollEvent()
			switch ev.Type {
			case EventError:
				return
			case EventInterrupt:
				return
			default:
			}
		}
	}()

	// testing if it panics
	for i := 0; i < b.N; i++ {
		for _, ev := range events {
			emitter.emit(ev.tpe, ev)
		}
	}
	w.Interrupt()
}

func TestDomWriterUnload(t *testing.T) {
	w, emitter, _, _ := newTestDomWriter(1, 1)

	emitter.emit(domEventUnload, testEvent{tpe: domEventUnload})

	ev := w.PollEvent()
	assert.Equal(t, EventError, ev.Type)
	assert.Error(t, ev.Err)
}
