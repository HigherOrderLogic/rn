package command

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ernestrc/blue/document"
	"github.com/ernestrc/blue/iterator"
	"github.com/ernestrc/blue/logging"
	log "github.com/sirupsen/logrus"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler"
	"unstable.build/go-tui/handler/search"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
)

// NewPrompt allocates storage for a new Prompt and initializes it.
func NewPrompt(
	storage document.Service, completer Completer,
	dispatcher Dispatcher, interrupter term.Interrupter,
	commands []text.CommandManual, config Config,
) *Prompt {
	ret := new(Prompt)
	ret.Init(storage, completer, dispatcher, interrupter, commands, config)
	return ret
}

// Prompt is a tui.Handler that presents a command browsing prompt
// with history scrolling and argument completion.
type Prompt struct {
	mode        commandPromptMode
	config      Config
	dispatcher  Dispatcher
	completer   Completer
	interrupter term.Interrupter

	height, width int
	// overlay buffer over the search list so we can
	// stop the search for multiple argument commands
	// but we can display arguments
	buf        cell.Buffer
	responsive component.Responsive
	list       search.List
	history    search.History

	commandAndArgs []string
	commandsBackup []text.CommandManual
	userScrolling  bool

	// used to signal across Handle calls that user
	// is cyclying through commands, in particular
	// when command key is a character that could be interpreted
	// as a character to be inserted in command input buffer
	prevCommandCycle bool

	inputString atomic.Value
	sync        bool
	mu          sync.Mutex
	animation   component.Virtual

	shownWidth         int
	manualComponent    component.Responsive
	showManual         bool
	resetManualTimeout chan struct{}
	ctx                context.Context
	cancelCtx          func()
	completionCtx      context.Context // children of ctx
	completionCancel   func()
}

var _ component.Floating = (*Prompt)(nil)

type commandPromptMode uint

const (
	modeCommandPromptCommand commandPromptMode = iota
	// modeCommandPromptArgs1
	// modeCommandPromptArgs2
	// ...
)

const (
	animationWidth          = 3
	defaultSeparatorHeight  = 1
	minWidthManualComponent = 100
	minListHeight           = 3
)

// Init initializes this handler with the given storage, completer,
// dispatcher, interrupter and config.
func (h *Prompt) Init(
	storage document.Service, completer Completer,
	dispatcher Dispatcher, interrupter term.Interrupter,
	commands []text.CommandManual, config Config,
) {
	cfg := search.ListConfig{
		Algo:             search.FuzzyMatch,
		Interrupter:      interrupter,
		CaseSensitive:    false,
		MatchedTextAttr:  &config.MatchedTextAttr,
		FocusElementAttr: &config.FocusElementAttr,
		ElementAttr:      &config.ElementAttr,
	}
	h.doInit(storage, completer, interrupter, dispatcher, commands, config, cfg)
}

func (h *Prompt) doInit(
	storage document.Service, completer Completer, interrupter term.Interrupter,
	dispatcher Dispatcher, commands []text.CommandManual, config Config,
	listCfg search.ListConfig,
) {
	h.mode = modeCommandPromptCommand
	h.config = config
	h.dispatcher = dispatcher
	h.completer = completer
	h.interrupter = interrupter
	h.animation.C = newNopAnimation(config)
	h.ctx, h.cancelCtx = context.WithCancel(context.Background())
	h.resetManualTimeout = make(chan struct{})

	h.buf.Init()
	h.inputString.Store("")
	h.responsive = component.Buffer(&h.buf,
		component.StringResponsiveConfig{
			StringConfig: component.StringConfig{
				Attributes:           config.ElementAttr,
				BackgroundAttributes: config.ElementAttr,
			},
		})

	h.list.Init(listCfg)

	for {
		h.history.Init(storage, config.DocumentID, config.MaxHistory)
		err := h.history.Load()
		if err == nil {
			break
		}
		h.log(log.ErrorLevel, "load history: %v", err)
		storage = document.NewInMemoryService()
	}

	// add a canceled cancelCtx so Wait never needs to check if cancelFn is nil
	ctx, cancel := context.WithCancel(h.ctx)
	cancel()
	h.completionCtx = ctx
	h.completionCancel = func() {}

	h.Reset(commands)
	h.startManualTimer()
}

func (h *Prompt) getCommandOverlayHeight(width int) int {
	leftWidgetWidth := h.width - animationWidth
	if leftWidgetWidth <= 0 {
		leftWidgetWidth = h.width
	}
	height := h.responsive.Height(leftWidgetWidth)
	// set to min 1, as it's being used as input field
	// and max to the height of the overlayed component
	height = int(math.Max(1, float64(height)))
	return height
}

// Resize satisfies tui.Handler
func (h *Prompt) Resize(width, height int) {
	h.width = width
	h.height = height
	if !h.showManual || h.manualComponent == nil {
		h.list.Resize(width, height)
		return
	}
	_, _, listHeight := h.calculateSplitHeights(width, height)
	h.list.Resize(width, listHeight)
}

func (h *Prompt) calculateSplitHeights(width, height int) (int, int, int) {
	separatorHeight := h.getSeparatorHeight()
	manHeight := h.manualComponent.Height(width)
	listHeight := height - manHeight - separatorHeight
	if manHeight < 0 || listHeight < minListHeight {
		return 0, 0, height
	}
	return manHeight, separatorHeight, listHeight
}

// Draw satisfies tui.Handler
func (h *Prompt) Draw(w term.Writer) {
	if h.showManual && h.manualComponent != nil {
		manHeight, separatorHeight, listHeight := h.calculateSplitHeights(h.width, h.height)
		if separatorHeight != 0 {
			separatorOffset := term.Coordinates{Y: listHeight}
			separatorWriter := component.VirtualWriter(w, separatorOffset, separatorHeight, h.width)
			comp := component.TestComponent{
				Ch:         h.config.FrameCharSet.HorizontalBottom,
				Attributes: h.config.FrameAttr,
			}
			separator := handler.Nop(&comp)
			separator.Resize(h.width, separatorHeight)
			separator.Draw(separatorWriter)
		}

		if manHeight != 0 {
			manOffset := term.Coordinates{Y: listHeight + separatorHeight}
			manWriter := component.VirtualWriter(w, manOffset, manHeight, h.width)
			h.manualComponent.Resize(h.width, manHeight)
			h.manualComponent.Draw(manWriter)
		}
	}

	h.drawPrompt(w)
}

func (h *Prompt) drawPrompt(w term.Writer) {
	// resize on every draw because search.List uses a responsive
	// input so local buffer changes must consider potential resize
	// of search.List
	bufHeight := h.getCommandOverlayHeight(h.width)

	// propagate local cmd+args buffer height to
	// search list, which only has cmd, in case args alone span
	// multiple lines
	h.list.SetMinInputHeight(bufHeight)

	leftWidgetWidth := h.width - animationWidth
	if leftWidgetWidth <= 0 {
		h.list.Draw(w)
		h.responsive.Resize(h.width, bufHeight)
		h.responsive.Draw(w)
		return
	}

	// needs to be dynamic because buffer can change height if prompt input
	// exceeds max width.
	var union component.FrameUnion
	union.Init(h.responsive)
	union.Frame = false

	// animation could finish any time
	h.mu.Lock()
	defer h.mu.Unlock()

	union.UnionRight(&h.animation, animationWidth)
	union.Resize(h.width, bufHeight)

	h.list.Draw(w)
	union.Draw(w)
}

func (h *Prompt) handleLastCommand() {
	cmd := h.history.Next()
	if cmd == "" {
		h.log(log.TraceLevel, "ignoring next command in history: empty")
		return
	}
	h.reset()
	for _, ch := range cmd {
		h.handle(term.Event{Type: term.EventKey, Ch: ch}, true)
	}
	h.mu.Lock()
	h.manualComponent = h.buildManualComponent(h.buf.String())
	h.mu.Unlock()
	h.log(log.TraceLevel, "done pushing history events")
}

func (h *Prompt) trimmedCommandAndArgs(cmd string, args ...string) []string {
	ret := make([]string, 0, len(args)+1)
	ret = append(ret, cmd)
	for _, argi := range args {
		if argi != "" {
			ret = append(ret, argi)
		}
	}
	return ret
}

func (h *Prompt) dispatchCommand() (
	quit, handled bool,
) {

	var commandAndArgsString string
	if len(h.commandAndArgs) != 0 {
		// add what's currently in the buffer; if user wanted to auto-complete
		// then Tab should be expected first
		if buf := h.list.Buffer().String(); buf != "" {
			h.commandAndArgs = append(h.commandAndArgs, buf)
		}
		// trim empty args (i.e. client added more spaces than required between args)
		h.commandAndArgs = h.trimmedCommandAndArgs(h.commandAndArgs[0], h.commandAndArgs[1:]...)
		commandAndArgsString = strings.Join(h.commandAndArgs, " ")

		h.log(log.TraceLevel, "dispatching command and args %#v", h.commandAndArgs)
		quit = h.dispatcher.Dispatch(h.commandAndArgs[0], h.commandAndArgs[1:]...)
	} else {
		match, _ := h.list.Focus()
		// if no args, then it means that we are in command mode, in which case
		// what's in the match list takes preference.
		commandAndArgsString = string(match.Data())
		// no match, use what's in buffer
		if commandAndArgsString == "" {
			commandAndArgsString = h.buf.String()
		}

		h.log(log.TraceLevel, "dispatching command %#v", commandAndArgsString)
		quit = h.dispatcher.Dispatch(commandAndArgsString)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if commandAndArgsString != "" {
		err := h.history.Add(commandAndArgsString)
		if err != nil {
			h.log(log.ErrorLevel, "add history %q: %v", commandAndArgsString, err)
		} else {
			h.log(log.TraceLevel, "add history %q: ok", commandAndArgsString)
		}
	}

	return true, true
}

// Handle satisfies tui.Handler
func (h *Prompt) Handle(ev term.Event) (quit, handled bool) {
	quit, handled = h.handle(ev, h.sync)
	if handled && !quit {
		select {
		// reset manual display timeout
		case h.resetManualTimeout <- struct{}{}:
		default:
		}
	}
	if handled {
		bufString := h.buf.String()
		h.inputString.Store(bufString)
		h.mu.Lock()
		h.manualComponent = h.buildManualComponent(bufString)
		h.mu.Unlock()
	}
	return
}

func (h *Prompt) handle(ev term.Event, sync bool) (quit, handled bool) {
	switch h.mode {
	case modeCommandPromptCommand:
		return h.handleCommand(ev, sync)
	default:
		return h.handleCompleteArgs(ev, sync)
	}
}

func (h *Prompt) handleCommon(ev *term.Event, sync bool) (quit, handled bool) {
	key := ev.KeyComb()
	if (key == h.config.HistoryKey && h.config.HistoryKey.Ch == 0) ||
		(key == h.config.HistoryKey && h.buf.Columns(0) == 0) ||
		(key == h.config.HistoryKey && h.prevCommandCycle) {
		h.handleLastCommand()
		h.prevCommandCycle = true
		return false, true
	}

	h.prevCommandCycle = false
	handled = true
	switch ev.Key {
	case term.KeyEnter:
		if h.userScrolling {
			h.incArgsCompleteMode(true, sync)
		}
		quit, handled = h.dispatchCommand()
		h.reset()
	case term.KeyEsc:
		quit = true
		h.Cancel()
	case term.KeyArrowDown, term.KeyCtrlJ:
		if h.userScrolling {
			h.list.FocusDown()
		} else {
			h.setUserScrolling(true)
		}
	case term.KeyArrowUp, term.KeyCtrlK:
		ok := h.list.FocusUp()
		if !ok {
			h.setUserScrolling(false)
		}
	case term.KeyCtrlC:
		h.cancelCompletionPush("received ctrl-c")
	case term.KeyTab:
		h.incArgsCompleteMode(true, sync)
	case term.KeySpace:
		ev.Ch = ' '
		handled = false
	default:
		handled = false
	}
	return
}

func (h *Prompt) handleCommand(ev term.Event, sync bool) (quit, handled bool) {
	quit, handled = h.handleCommon(&ev, sync)
	if handled {
		return
	}

	switch ev.Key {
	case term.KeyBackspace, term.KeyBackspace2:
		cols := h.buf.Columns(0)
		if cols == 0 {
			handled = true
			quit = true
			h.Cancel()
			return
		}
		h.buf.DeleteCell(term.Coordinates{X: cols - 1})
		h.list.Buffer().Replace(h.buf.String())
		handled = true
		return
	}

	if ev.Ch == 0 {
		return
	}

	handled = true
	h.buf.WriteString(string(ev.Ch))
	if ev.Ch == ' ' {
		// wait as commands are finite and muscle memory could beat
		// the completing logic
		h.Wait()
		h.incArgsCompleteMode(true, sync)
		return
	}
	h.list.Buffer().WriteString(string(ev.Ch))
	return
}

func (h *Prompt) handleCompleteArgs(ev term.Event, sync bool) (quit, handled bool) {
	quit, handled = h.handleCommon(&ev, sync)
	if handled {
		return
	}
	switch ev.Key {
	case term.KeyBackspace, term.KeyBackspace2:
		handled = true
		h.buf.DeleteCell(term.Coordinates{X: h.buf.Columns(0) - 1})
		if h.list.Buffer().Size() != 0 {
			h.list.Buffer().DeleteCell(
				term.Coordinates{X: h.list.Buffer().Columns(0) - 1},
			)
			h.setCompletionList(false, sync, h.commandAndArgs[0], h.completionArgs()...)
			return
		}
		if !h.decArgsCompleteMode(sync) {
			h.setCommandMode()
		}
		return
	}

	if ev.Ch == 0 {
		return
	}

	handled = true
	h.buf.WriteString(string(ev.Ch))
	if ev.Ch == ' ' {
		// do not wait here, as args are expected to be dynamic
		// and fuzzy search is a guide for user to complete
		h.incArgsCompleteMode(false, sync)
		return
	}
	h.list.Buffer().WriteString(string(ev.Ch))
	h.setCompletionList(false, sync, h.commandAndArgs[0], h.completionArgs()...)
	return
}

func (h *Prompt) completionArgs() []string {
	args := make([]string, 0, len(h.commandAndArgs))
	args = append(args, h.commandAndArgs[1:]...)
	return append(args, h.list.Buffer().String())
}

func (h *Prompt) completeTopList() bool {
	match, ok := h.list.Focus()
	if !ok {
		h.log(log.TraceLevel, "completeTopList: no matches on search list with %q",
			h.list.Buffer().String())
		return false
	}
	// could have completed multiple arguments
	parts := strings.Split(string(match.Data()), " ")
	h.commandAndArgs = append(h.commandAndArgs, parts...)
	newCmdAndArgs := strings.Join(h.commandAndArgs, " ")
	h.buf.Replace(newCmdAndArgs + " ")

	return true
}

func (h *Prompt) log(level log.Level, msg string, args ...interface{}) {
	log.WithField(logging.KeyClass, "command.Prompt").
		Logf(level, msg, args...)
}

func (h *Prompt) incArgsCompleteMode(complete bool, sync bool) {
	if !complete || !h.completeTopList() {
		h.commandAndArgs = append(h.commandAndArgs, h.list.Buffer().String())
	}
	// mode needs to be at least the number of command and arguments that have
	// been completed as per user request
	h.mu.Lock()
	for int(h.mode) < len(h.commandAndArgs) {
		h.mode++
	}
	h.mu.Unlock()
	h.list.Buffer().Reset()
	h.setCompletionList(true, sync, h.commandAndArgs[0], h.commandAndArgs[1:]...)
}

func (h *Prompt) decArgsCompleteMode(sync bool) bool {
	if h.mode == 1 {
		return false
	}
	h.mu.Lock()
	h.mode--
	h.mu.Unlock()
	lastIdx := len(h.commandAndArgs) - 1
	last := h.commandAndArgs[lastIdx]
	h.commandAndArgs = h.commandAndArgs[:lastIdx]
	h.list.Buffer().Replace(last)
	h.setCompletionList(true, sync, h.commandAndArgs[0], h.commandAndArgs[1:]...)
	return true
}

func (h *Prompt) setCommandMode() {
	h.mu.Lock()
	h.mode = modeCommandPromptCommand
	h.mu.Unlock()
	h.list.Buffer().Replace(h.buf.String())
	h.commandAndArgs = h.commandAndArgs[:0]
	h.resetListWith(h.commandsBackup)
	h.setUserScrolling(true)
}

func (h *Prompt) setCompletionList(
	persistLastArgUpdates, sync bool, cmd string, args ...string,
) {
	h.log(log.TraceLevel, "setCompletionList: %s %#v", cmd, args)

	ctx, cancel := context.WithCancel(h.ctx)

	// cancel prev if there's any
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cancelCompletionPush("re set completion list")
	h.completionCancel = cancel
	h.completionCtx = ctx
	h.setUserScrolling(false)

	// if user added any extra spaces, do not pass to completer
	cmdAndArgs := h.trimmedCommandAndArgs(cmd, args...)

	h.list.DataReset()
	it, newLastArg := h.completer.Complete(ctx, cmdAndArgs[0], cmdAndArgs[1:]...)
	if newLastArg != "" {
		newCmdAndArgs := make([]string, len(cmdAndArgs))
		copy(newCmdAndArgs, cmdAndArgs)
		h.log(log.TraceLevel, "completer returned updated last arg %v: %q -> %q",
			cmdAndArgs, newCmdAndArgs[len(newCmdAndArgs)-1], newLastArg)
		newCmdAndArgs[len(newCmdAndArgs)-1] = newLastArg
		// NOTE: this method is prone to expose errors if newLastArg is incorrect
		// ensure that all completer implementation use newLastArg sparingly
		// and at some point a Delete+Insert option should be explored
		h.buf.Replace(strings.Join(newCmdAndArgs, " "))
		if persistLastArgUpdates {
			h.commandAndArgs[len(h.commandAndArgs)-1] = newLastArg
		} else {
			h.list.Buffer().Replace(newLastArg)
		}
	}

	it, isEmpty := iterator.IsEmpty(it)
	if isEmpty {
		it = h.manualCompleter(ctx, cmdAndArgs[0], cmdAndArgs[1:]...)
	}

	if sync {
		h.pushCompletionListSync(ctx, cancel, cmdAndArgs, it)
	} else {
		ch := h.list.Push(ctx)
		go h.pushCompletionList(ctx, ch, cancel, cmdAndArgs, it)
	}
}

func (h *Prompt) pushCompletionListSync(
	ctx context.Context,
	cancel func(),
	cmdAndArgs []string,
	it iterator.Iterator[string],
) {
	defer cancel()
	var i int
	for ; ; i++ {
		next, ok := it.Next()
		if !ok {
			break
		}
		h.list.PushSync([]byte(next))
	}

	if i == 0 {
		// push args history if default completion iterator is empty
		it, ok := h.commandArgsHistoryIterator(cmdAndArgs)
		if ok {
			h.pushCompletionListSync(ctx, cancel, cmdAndArgs, it)
		}
	}
}

func (h *Prompt) commandArgsHistoryIterator(cmdAndArgs []string) (iterator.Iterator[string], bool) {
	history := h.history.Slice()
	it := iterator.FromSlice(history)

	// cmdAndArgs is not orthogonal to how we want to handle them here
	// essentially, we don't know by simply inspecting them, if we are
	// at the start of a new arg, or at the end of the previous command
	// as last space is handled ambigously.
	cmdAndArgs = strings.Split(strings.Join(cmdAndArgs, " "), " ")
	m := len(cmdAndArgs)
	if int(h.mode) < len(cmdAndArgs) {
		m = len(cmdAndArgs) - 1
	}
	queryMatch := strings.Join(cmdAndArgs[:m], " ")

	h.log(log.TraceLevel, "filtering data with mode %d and cmdAndArgs: %+v, len(%d), filter: %+v",
		h.mode, cmdAndArgs, len(cmdAndArgs), queryMatch)

	filterNoArgs := iterator.Filter(it, func(query string) bool {
		return strings.HasPrefix(query, queryMatch)
	})
	mapArgs := iterator.Map(filterNoArgs, func(query string) (args string) {
		storedAndArgs := strings.Split(query, " ")
		ret := strings.Join(storedAndArgs[m:], " ")
		return ret
	})
	seen := make(map[string]struct{})
	uniqueArgs := iterator.Filter(mapArgs, func(args string) bool {
		_, ok := seen[args]
		if !ok {
			seen[args] = struct{}{}
			return args != ""
		}
		return false
	})
	it, isEmpty := iterator.IsEmpty(uniqueArgs)
	return it, !isEmpty
}

func (h *Prompt) pushCompletionList(
	ctx context.Context,
	ch chan<- []byte, cancel func(),
	cmdAndArgs []string,
	it iterator.Iterator[string],
) {
	defer close(ch)
	defer cancel()

	// draw progress animation while iterator is still returning results
	frames, seq := component.ProgressAnimationFrames()
	animation := component.NewAnimation(h.interrupter, frames, seq, 10)
	defer func() {
		_ = animation.Close()
		h.mu.Lock()
		h.animation.C = newNopAnimation(h.config)
		h.mu.Unlock()
	}()

	h.mu.Lock()
	h.animation.C = animation
	h.mu.Unlock()

	var i int
	for ; ; i++ {
		next, ok := it.Next()
		if !ok {
			break
		}
		select {
		case <-ctx.Done():
			h.log(log.TraceLevel, "context canceled for ch %p before completed push", ch)
			return
		case ch <- []byte(next):
			h.log(log.TraceLevel, "pushed %q onto search list for ch %p", next, ch)
		}
	}

	err := it.Err()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			h.log(log.ErrorLevel, "completion iterator error: %v", err)
		}
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if i == 0 {
		it, ok := h.commandArgsHistoryIterator(cmdAndArgs)
		if ok {
			h.pushCompletionListSync(ctx, cancel, cmdAndArgs, it)
		}
	}
}

// Reset resets the commands listed in this Prompt.
// It should be called after initialization and every time
// new commands are available.
func (h *Prompt) Reset(commands []text.CommandManual) {
	h.setCommandMode()
	h.buf.Reset()
	h.list.Buffer().Reset()
	h.commandsBackup = commands
	h.resetListWith(commands)
}

// assumes holding lock
func (h *Prompt) cancelCompletionPush(reason string) {
	h.log(log.DebugLevel, "completion push to search list: %s", reason)
	h.completionCancel()
	h.list.Cancel()
}

func (h *Prompt) resetListWith(items []text.CommandManual) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cancelCompletionPush("reset list")

	h.list.DataReset()
	for _, item := range items {
		h.list.PushSync([]byte(item.Name))
	}
}

func (h *Prompt) reset() {
	h.Cancel()
	h.setCommandMode()
	h.buf.Reset()
	h.list.Buffer().Reset()
}

// Cursor satisfies tui.Handler
func (h *Prompt) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	if h.width == 0 {
		return term.Coordinates{}, 0, false
	}
	leftWidgetWidth := h.width - animationWidth
	if leftWidgetWidth <= 0 {
		leftWidgetWidth = h.width
	}
	var pos term.Coordinates
	x := len(h.buf.String()) % leftWidgetWidth
	y := len(h.buf.String()) / leftWidgetWidth
	if y >= h.height {
		pos.X += leftWidgetWidth - 1
		pos.Y += h.height - 1
	} else {
		pos.X += x
		pos.Y += y
	}
	return pos, term.CursorStyleBlinkingBar, true
}

// Man satisfies tui.Handler
func (h *Prompt) Man() tui.Manual {
	panic("TODO")
}

// Wait waits for any asynchronous completion
// or search to finish before it returns.
func (h *Prompt) Wait() {
	h.mu.Lock()
	completionCtx := h.completionCtx
	h.mu.Unlock()

	<-completionCtx.Done()
	h.list.Wait()
}

// Cancel cancels any asynchronous completion or search currently ongoing
// or does nothing if there's currently no ongoing completion or search.
func (h *Prompt) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cancelCompletionPush("cancel")
	h.list.Cancel()
}

// Dimensions satisfies component.Floating.
func (h *Prompt) Dimensions() (width, height int) {
	const (
		matchPadding = 1
		maxWidth     = 100
		maxHeight    = 20
	)

	h.mu.Lock()
	defer h.mu.Unlock()

	minWidth := 50
	if h.showManual && h.manualComponent != nil {
		minWidth = minWidthManualComponent
	}
	// always set min width if show manual was triggered
	// so user doesn't get confused if width goes back and forth
	// as its tipying.
	minWidth = int(math.Max(float64(minWidth), float64(h.shownWidth)))

	maxWidthItems := minWidth
	h.list.IterateVisible(func(m search.Match) {
		if mlen := len(m.Data()) + matchPadding; mlen > maxWidthItems {
			maxWidthItems = mlen
		}
	})
	width = int(math.Max(float64(h.list.Buffer().MaxColumns()), float64(maxWidthItems)))
	width = int(math.Min(float64(width), maxWidth))

	bufHeight := h.getCommandOverlayHeight(width)
	height = int(math.Min(math.Max(float64(h.list.MatchCount()+bufHeight), minListHeight), maxHeight))

	if h.showManual && h.manualComponent != nil {
		height += h.manualComponent.Height(width)
		height += h.getSeparatorHeight()
	}
	h.shownWidth = width
	return
}

// Close closes all resources associated with this Prompt.
func (h *Prompt) Close() error {
	defer h.cancelCtx()

	h.mu.Lock()
	defer h.mu.Unlock()

	h.cancelCompletionPush("close")
	return h.list.Close()
}

func (h *Prompt) setUserScrolling(scrolling bool) {
	h.userScrolling = scrolling
	if scrolling {
		h.list.SetFocusAttr(h.config.FocusElementAttr)
	} else {
		h.list.SetFocusAttr(h.config.ElementAttr)
	}

}

func (h *Prompt) startManualTimer() {
	if h.config.ShowManualAfter == 0 {
		h.manualComponent = h.buildManualComponent(h.buf.String())
		h.showManual = true
		return
	}

	timer := time.NewTimer(h.config.ShowManualAfter)
	h.log(log.DebugLevel, "showing manual for commands after %s",
		h.config.ShowManualAfter)

	go func() {
		defer timer.Stop()
		for {
			select {
			case <-h.resetManualTimeout:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(h.config.ShowManualAfter)
				h.log(log.TraceLevel, "reseting timeout for showing manual")
			case <-timer.C:
				h.log(log.DebugLevel, "showing manual for commands")
				h.mu.Lock()
				h.manualComponent = h.buildManualComponent(h.inputString.Load().(string))
				h.showManual = true
				h.mu.Unlock()
				h.interrupter.Interrupt()
				return
			case <-h.ctx.Done():
				return
			}
		}
	}()
}

func (h *Prompt) buildManualComponent(bufString string) component.Responsive {
	var man text.CommandManual
	var ok bool
	cmdAndArgs := strings.Split(strings.TrimSpace(bufString), " ")

	if len(cmdAndArgs) == 0 || (len(cmdAndArgs) == 1 && int(h.mode) < 1) {
		// if input is something like "ed" or "" then
		// find the manual of the top match of the search list.
		h.log(log.TraceLevel, "Length in words of input buffer is 0-1, "+
			"using top of the search list as desired command.")
		man, ok = h.getManualFromFocus()
	} else if len(cmdAndArgs) == 1 || (len(cmdAndArgs) == 2 && h.mode < 2) {
		// if input is something like "edit " or "edit m" or "edit myFile " then
		// find the manual of the first word in the input buffer
		cmd := cmdAndArgs[0]
		man, ok = h.getManualForCommand(cmd)
	} else {
		// if input is something like "edit myFile my" or "edit myFile myFile ..."
		// find the manual of the first word in the input buffer, then try to find
		// the manual of the last completed subcommand.
		h.log(log.TraceLevel, "Length in words of input buffer is >1, "+
			"using buffer to get man for command or sub-command.")
		cmd := cmdAndArgs[0]
		man, ok = h.getManualForCommand(cmd)
		if !ok {
			h.log(log.TraceLevel, "could not find manual for first "+
				"word in input buffer %q", cmd)
			return nil
		}
		// use mode to know if user has already completed
		// last arg or not.
		args := cmdAndArgs[1:]
		if int(h.mode) < len(cmdAndArgs) {
			// trim last argument, since it hasn't been completed yet
			args = args[:len(args)-1]
		}
		subcmd, foundSubcommand := getSubcommandManual(man, args)
		if foundSubcommand {
			man = subcmd
		} // else display parent command's manual
	}

	if ok {
		return makeManualComponent(man, h.config.FrameCharSet, h.config.ElementAttr)
	}
	return nil
}

func (h *Prompt) getManualFromFocus() (man text.CommandManual, ok bool) {
	h.list.Wait()
	m, ok := h.list.Focus()
	if !ok {
		h.log(log.TraceLevel, "could not get search list focus")
		return
	}
	cmd := m.Data()
	man, ok = h.getManualForCommand(string(cmd))
	if !ok {
		h.log(log.TraceLevel, "could not find manual for top of search list command %q", cmd)
	}
	return
}

func (h *Prompt) getManualForCommand(cmd string) (text.CommandManual, bool) {
	for _, man := range h.commandsBackup {
		if man.Name == cmd {
			return man, true
		}
	}
	return text.CommandManual{}, false
}
func (h *Prompt) manualCompleter(
	ctx context.Context, cmd string, args ...string,
) iterator.Iterator[string] {
	man, ok := h.getManualForCommand(cmd)
	if !ok || cmd == "" {
		return iterator.FromSlice[string](nil)
	}

	args = strings.Split(strings.TrimSpace(strings.Join(args, " ")), " ")
	// return iterator with submcommands,
	// if first command hasn't been fully typed yet
	if len(args) == 0 || (len(args) == 1 && h.mode < 2) {
		return iterator.Map(iterator.FromSlice(man.Commands), manualToName)
	}

	// use mode to know if user has already completed
	// last arg or not.
	cmdAndArgsLen := len(args) + 1
	if int(h.mode) < cmdAndArgsLen {
		// trim last argument, since it hasn't been completed yet
		args = args[:len(args)-1]
	}

	// if we find the last argument's subcommand manual,
	// then use that as the completion args, otherwise just
	// do not return any completion args.
	lastArg := args[len(args)-1]
	if man, ok := getSubcommandManual(man, args); ok && man.Name == lastArg {
		return iterator.Map(iterator.FromSlice(man.Commands), manualToName)
	}
	return iterator.FromSlice[string](nil)
}

func (h *Prompt) getSeparatorHeight() int {
	if h.config.FrameCharSet == (component.FrameCharSet{}) {
		return 0
	}
	return defaultSeparatorHeight
}

func newNopAnimation(cfg Config) tui.Component {
	return component.WithBackground(component.Nop(),
		term.Cell{Bg: cfg.ElementAttr.Bg, Fg: cfg.ElementAttr.Fg})
}
