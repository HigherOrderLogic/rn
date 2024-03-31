package vte

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/ernestrc/blue/logging"
	"github.com/ernestrc/blue/retry"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/term/vte/screen"
	"unstable.build/go-tui/text/clipboard"
	"unstable.build/go-tui/text/vi"
)

var _ tui.Handler = (*viHandler)(nil)
var triggerBellRetryStrategy = retry.CombinedStrategy(
	retry.LimitStrategy(2),
	retry.ExponentialStrategy(10*time.Millisecond, 100*time.Millisecond),
)

const waitBellAtMostDuration = 100 * time.Millisecond

// viHandler serves both as a tui.Handler entrypoint to a limited vi tui.Handler
// implementation, and a cell.Editor, which intercepts user edits
// and transforms them into shell escape sequences to manipulate the content.
type viHandler struct {
	comp   parentComponent
	remote remote
	config Config
	width  int
	sync   struct {
		mu       sync.Locker
		vi       *vi.Vi
		editor   cell.Editor
		selector *cell.Buffer
	}
}

// for dependency injection purposes
type parentComponent interface {
	PrimaryScroll() *component.Scroll
	URI() workspaceapi.URI
	Locker() sync.Locker
	cursorAtScroll() term.Coordinates
	scheduleBellCallback(timeout time.Duration, callback func()) bool
}

func (v *viHandler) init(comp *Component, config Config) {
	v.doInit(comp, config)
	v.remote = ptyWriterRemote(comp)
	comp.waitParserHandler.useTrigger(v.triggerBell)
}

func (v *viHandler) doInit(comp parentComponent, config Config) {
	scroll := comp.PrimaryScroll()
	opts := []vi.Option{
		vi.WithBarHidden(true),
		vi.WithResAttr(config.SelectionAttributes),
		vi.WithAttr(config.Attributes),
		vi.WithWrap(false),
		vi.WithClipboard(config.Clipboard),
		vi.WithAutoSkipNullCells(false),
	}
	vi := new(vi.Vi)
	vi.InitWithScroll(scroll, comp.URI(), opts...)
	v.sync.vi = vi
	v.sync.selector = scroll.Buffer()
	v.sync.mu = comp.Locker()
	v.comp = comp
	v.config = config
	if log.IsLevelEnabled(log.TraceLevel) {
		v.remote = newLoggingRemote(v.remote)
	}
	v.sync.editor = scroll.Buffer().WithEditor(v)
}

func (v *viHandler) Handle(ev term.Event) (exit, handled bool) {
	exit, handled = v.handle(ev)
	if exit {
		v.scheduleAfterBell(func() {
			pos := v.trimToLastValidColumn(v.sync.vi.CursorAtScroll())
			if ev.Key != term.KeyEnter {
				v.moveRemoteTo(pos)
				v.remoteFlush()
			}
		})
		// no need to synchronize here as vi is not reading/writing
		// the underlying buffer for the following 2 calls
		v.sync.vi.Unselect()
		v.sync.vi.SetNormalMode()
	}
	return
}

func (v *viHandler) Resize(width, height int) {
	v.sync.mu.Lock()
	defer v.sync.mu.Unlock()

	v.width = width
	v.sync.vi.Resize(width, height)
}

func (v *viHandler) Draw(w term.Writer) {
	v.sync.mu.Lock()
	defer v.sync.mu.Unlock()

	v.sync.vi.Draw(w)
}

func (v *viHandler) Man() tui.Manual {
	panic("TODO")
}

func (v *viHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	v.sync.mu.Lock()
	defer v.sync.mu.Unlock()

	return v.sync.vi.Cursor()
}

// Edit implements the following behavior (B is content not allowed to edit,
// A is content allowed to edit, E is edit):
//
//	┌──────┐     ┌──────┐     ┌──────┐
//	│BBBBB │  ∩  │EEEE  │  =  │      │
//	│ $ AA │     │      │     │      │ -> bell
//	└──────┘     └──────┘     └──────┘
//	┌──────┐     ┌──────┐     ┌──────┐
//	│BBBBBB│  ∩  │      │  =  │      │
//	│ $ AA │     │ $ EE │     │ $ EE │
//	└──────┘     └──────┘     └──────┘
//	┌──────┐     ┌──────┐     ┌──────┐
//	│BBBBBB│  ∩  │EEEEEE│  =  │      │
//	│ $ AA │     │ $ EE │     │ $ EE │
//	└──────┘     └──────┘     └──────┘
//	┌──────┐     ┌──────┐     ┌──────┐
//	│ $ AAA│  ∩  │ $ EEE│  =  │ $ EEE│
//	│AA    │     │EEEEE │     │EEEEE │
//	└──────┘     └──────┘     └──────┘
//	┌──────┐     ┌──────┐     ┌──────┐
//	│ $ A  │  ∩  │ $ EEE│  =  │ $ EEE│
//	│      │     │EEEEE │     │EEEEE │
//	└──────┘     └──────┘     └──────┘
//	┌──────┐     ┌──────┐     ┌──────┐
//	│ $ AAA│  ∩  │      │  =  │ $ AAA│
//	│AAAA  │     │ EEEEE│     │AEEEEE│
//	└──────┘     └──────┘     └──────┘
//	┌──────┐     ┌──────┐     ┌──────┐
//	│ $ AA │  ∩  │      │  =  │      │ -> bell
//	│      │     │   EE │     │      │
//	└──────┘     └──────┘     └──────┘
func (v *viHandler) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	if screenContext := screen.IsScreenContext(ctx); screenContext {
		v.sync.vi.OnWillEdit(ctx, start, end, str)
		from, to, old = v.sync.editor.Edit(ctx, start, end, str)
		v.sync.vi.OnDidEdit(ctx, from, to, old)
		return
	}

	lastLineStart, lastLineEnd := v.lastPromptLine()
	var ok bool
	endForIntersection := end
	// inserts start==end so to calculate intersection we need to end.X++
	// otherwise there's never an intersection
	if start == end {
		endForIntersection.X++
		lastLineEnd.X++
	}
	intersectionStart, intersectionEnd, ok := term.CoordinatesIntersection(
		start, endForIntersection, lastLineStart, lastLineEnd)
	if !ok {
		from = start
		to = start
		v.log(log.TraceLevel, "edit not allowed: last line: [%+v, %+v), "+
			"start: %+v, end: %+v, endForIntersection: %+v, str: %q",
			lastLineStart, lastLineEnd, start, end, endForIntersection, str)
		v.config.RingBell()
		return
	}

	v.log(log.TraceLevel, "edit: "+
		"start: %+v, end: %+v, str: %q, last line: [%+v, %+v), intersection: [%+v, %+v)",
		start, end, str, lastLineStart, lastLineEnd, intersectionStart, intersectionEnd)

	oldStart := start
	insert := start == end
	if insert {
		start = intersectionStart
		end = start
	} else {
		start = intersectionStart
		end = intersectionEnd
	}

	// trim start to avoid editing prompt, only if for deletes
	start.X = v.moveRemoteTo(start)
	if insert {
		end.X = start.X
		endForIntersection.X = end.X + 1
	}
	from = start
	to = from

	if start == end && str == "" {
		v.log(log.TraceLevel, "edit not allowed after prompt correction: last line: [%+v, %+v), "+
			"start: %+v, end: %+v, endForIntersection: %+v, str: %q",
			lastLineStart, lastLineEnd, start, end, endForIntersection, str)
		v.config.RingBell()
		return
	}

	v.sync.vi.OnWillEdit(ctx, start, end, str)
	oldCells, _, ok := v.sync.selector.Select(start, end)
	if ok {
		old = cell.CellsToString(oldCells)
		if old != "" && strings.Count(old, " ") != len(old) {
			// Edit must maintain reversibility. Since shell
			// wraps lines automatically, we must remove newlines.
			old = strings.ReplaceAll(old, "\n", "")
			// copy user deletes to clipboard
			v.config.Clipboard.Copy(v.config.ClipboardRegister,
				clipboard.Data{Text: old})
		}
	}

	v.log(log.TraceLevel, "edit: effective start %+v end %+v", start, end)
	view := v.sync.vi.CellView()
	rows := view.Rows()
	rowsToDelete := end.Y - start.Y
	for i := 0; i <= rowsToDelete; i++ {
		first := i == 0
		last := i == rowsToDelete
		line := start.Y + i

		var endX int
		if last {
			endX = end.X
		} else if line < rows {
			endX = view.Columns(line)
		}

		var startX int
		if first {
			startX = start.X
			to.X = from.X
		} else {
			to.X = 0
		}

		for j := 0; j < endX-startX; j++ {
			v.remote.deleteChar()
			to.X++
		}
		if !last {
			v.remote.conflate()
			to.Y++
		}
	}

	defer func() {
		v.remoteFlush()
		v.sync.vi.OnDidEdit(ctx, from, to, old)
		v.log(log.TraceLevel, "edit return: from %+v to %+v", from, to)
	}()

	if str == "" {
		return
	}

	to.X = from.X
	to.Y = from.Y
	for _, ch := range str {
		// skip parts of str that were skipped before due to prompt start or not
		// in allowed range
		_, _, ok := term.CoordinatesIntersection(start, endForIntersection, oldStart,
			term.Coordinates{Y: oldStart.Y, X: oldStart.X + 1})
		if !ok {
			if ch == '\n' {
				oldStart.Y++
				oldStart.X = 0
			} else {
				oldStart.X++
			}
			continue
		}
		// entire buffer replaces via undo/redo try to replace
		// beyond last line by adding/removing newlines
		// but that causes shell to add unecessary newlines.
		if ch == '\n' && to.Y >= lastLineEnd.Y {
			return
		}
		v.remote.insertChar(ch)
		to.X++
		if to.X == v.width {
			v.remote.wrapLine()
			to.X = 0
			to.Y++
		}
	}
	return
}

func (v *viHandler) remoteFlush() {
	err := v.remote.flush()
	if err != nil {
		v.log(log.WarnLevel, "flush: %v", err)
	}
}

func (v *viHandler) handle(ev term.Event) (exit, handled bool) {
	if ev.Type != term.EventKey {
		return v.sync.vi.Handle(ev)
	}

	switch ev.Ch {
	case 'i':
		if !v.sync.vi.IsEditMode() {
			exit = true
			handled = true
			return
		}
	// ensure that repeat is not invoked, as underlying
	// buffer was not initialized with subscribe functionality,
	// which repeat is dependent upon
	case '.':
		return
	}
	handled = true
	switch ev.Key {
	case term.KeyEnter:
		// TODO except if in search mode
		v.remote.linefeed()
		v.remoteFlush()
		exit = true
		return
	case term.KeyCtrlK, term.KeyArrowUp:
		v.remote.keyArrowUp()
		v.remoteFlush()
		v.scheduleAfterBell(v.moveViToLastLineCharacter)
		return
	case term.KeyCtrlJ, term.KeyArrowDown:
		v.remote.keyArrowDown()
		v.remoteFlush()
		v.scheduleAfterBell(v.moveViToLastLineCharacter)
		return
	case term.KeyCtrlL:
		v.remote.formFeed()
		v.remoteFlush()
		return
	case term.KeyCtrlC:
		exit = true
		return
	default:
		handled = false
	}

	// schedule any potential edits after finding the start of the prompt:
	// this avoids race conditions when serializing handle with bell callbacks
	// and also prevents the main loop goroutine to not deadlock with the vte parser
	// goroutine. This cannot be performed during a call to Edit, because
	// the cursor logic heavily depends on the correct return values of Edit.
	v.scheduleAfterBell(func() {
		exit, _ = v.sync.vi.Handle(ev)
		// correct mouse coordinates beyond last line so
		// when moving through graphical windows doesn't
		// leave cursor in an non-useful coordinate.
		if ev.Ch != ' ' && !exit {
			// after edits, content might have changed
			// use bell to synchronize to the last state change
			// and then move cursor to bounds
			v.scheduleAfterBell(v.moveViToBounds)
		}
	})
	handled = true
	return exit, handled
}

func (v *viHandler) enterViMode(pos term.Coordinates) {
	pos.X = int(math.Max(float64(pos.X-1), float64(0)))
	v.sync.vi.SetCursorAtScroll(pos)
	v.scheduleAfterBell(v.moveViToBounds)
}

func (v *viHandler) log(level log.Level, line string, params ...interface{}) {
	log.WithField(logging.KeyClass, "vte.viHandler").
		Logf(level, line, params...)
}

func (v *viHandler) lastValidColumn(y int) int {
	view := v.sync.vi.CellView()
	cells := view.RawCells()
	if y >= len(cells) {
		// this is a guard against deleting last line
		// which calls Edit with y==len(rows).
		// Editor.Edit handles that but not
		// View.Columns below.
		return 0
	}
	for x := view.Columns(y) - 1; x > 0; x-- {
		c := cells[y][x]
		if c.Ch != screen.DefaultChar {
			return x
		}
	}
	return 0
}

func (v *viHandler) scheduleAfterBell(cb func()) {
	ok := v.comp.
		scheduleBellCallback(waitBellAtMostDuration, func() {
			// bell handler does not lock because bell
			// is usually implemented as synchronous I/O
			v.sync.mu.Lock()
			defer v.sync.mu.Unlock()

			// NOTE: this is scheduled as a user callback on the
			// next event loop tick.
			cb()
		})
	if !ok {
		v.log(log.WarnLevel, "could not schedule sync trigger: too many events")
	}
}

func (v *viHandler) triggerBell() {
	// the following sequence forces a bell from the underlying shell
	// tested with bash, sh and zsh. It is necessary to work around
	// shell prompt prefixes so comp.CursorAtScroll is correct,
	// or when we need to synchronize after the processing delay of a
	// remote sequence.
	//
	// Retry this otherwise we might end up not processing any events
	// as wait parser handler scheduler might overflow.
	retry.Retry(context.Background(), triggerBellRetryStrategy,
		func(ctx context.Context) (bool, error) {
			// NOTE: this is ALSO scheduled as a user callback
			// on the next event loop tick, so if event loop channel
			// gets filled up, the performance degrades substantially
			// but eventually it should catch up.
			v.remote.moveStartOfLine()
			v.remote.moveLeft()
			return true, v.remote.flush()
		})
}

func (v *viHandler) trimToLastValidColumn(pos term.Coordinates) term.Coordinates {
	_, lastLineEnd := v.lastPromptLine()
	return term.Coordinates{
		Y: int(math.Min(float64(pos.Y), float64(lastLineEnd.Y))),
		X: int(math.Min(float64(pos.X), float64(lastLineEnd.X))),
	}
}

func (v *viHandler) moveViToLineLastColumnOffset(offset int) {
	_, lastLineEnd := v.lastPromptLine()
	lastLineEnd.X += offset
	v.sync.vi.SetCursorAtScroll(lastLineEnd)
}

func (v *viHandler) moveRemoteTo(target term.Coordinates) (actual int) {
	start := v.comp.cursorAtScroll()

	if start.Y == target.Y {
		// use current cursor x position, to take $ or other
		// shell prefixes into consideration
		if start.X >= target.X {
			actual = start.X
			return
		}
		for i := start.X; i < target.X; i++ {
			v.remote.moveRight()
		}
		actual = target.X
		return
	}

	offset := start.X
	for i := start.Y; i < target.Y; i++ {
		for j := 0; j < v.width-offset; j++ {
			v.remote.moveRight()
		}
		v.remote.cursorCRLF()
		offset = 0
	}

	for i := 0; i < target.X; i++ {
		v.remote.moveRight()
	}
	actual = target.X

	return
}

func (v *viHandler) moveViToBounds() {
	/*promptStart := v.comp.cursorAtScroll()

	pos := v.sync.vi.CursorAtScroll()

	// correct past last line + 1
	_, lastLineEnd := v.lastPromptLine()
	if pos.Y > lastLineEnd.Y+1 {
		v.moveViToLineLastColumnOffset(1)
		return
	}

	// correct past last line + 1 to be at most x = 0
	if pos.Y == lastLineEnd.Y+1 && pos.X != 0 {
		pos = term.Coordinates{Y: lastLineEnd.Y + 1, X: 0}
		v.sync.vi.SetCursorAtScroll(pos)
	}

	// correct past last column, at last line, only if not in edit mode
	if !v.sync.vi.IsEditMode() {
		lastValidCol := int(math.Max(float64(lastLineEnd.X), float64(promptStart.X)))
		if pos.Y == lastLineEnd.Y && pos.X > lastValidCol {
			pos = term.Coordinates{Y: lastLineEnd.Y, X: lastValidCol}
			v.sync.vi.SetCursorAtScroll(pos)
			return
		}
	}

	// correct prior to prompt start
	if pos.Y == lastLineEnd.Y && pos.X < promptStart.X {
		pos = term.Coordinates{Y: lastLineEnd.Y, X: promptStart.X}
		v.sync.vi.SetCursorAtScroll(pos)
		return
	}
	*/
}

func (v *viHandler) moveViToLastLineCharacter() {
	pos := v.lastContentColumn()
	v.sync.vi.SetCursorAtScroll(pos)
}

// finds the last line column that's not a default
// character or a space.
func (v *viHandler) lastContentColumn() term.Coordinates {
	cells := v.sync.vi.CellView().RawCells()
	if len(cells) == 0 {
		return term.Coordinates{}
	}
	y := len(cells) - 1
	for x := len(cells[y]) - 1; x > 0; x-- {
		c := cells[y][x]
		if c.Ch != screen.DefaultChar && c.Ch != ' ' {
			return term.Coordinates{Y: y, X: x}
		}
	}
	return term.Coordinates{Y: y}
}

func (v *viHandler) lastPromptLine() (term.Coordinates, term.Coordinates) {
	return lastPromptLine(v.sync.vi.CellView(), v.width)
}
