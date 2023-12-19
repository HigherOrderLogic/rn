package extension

import (
	"strconv"

	"unstable.build/go-tui"
	workspaceapi "unstable.build/go-tui/api/workspace"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/component"
	"unstable.build/go-tui/handler/search"
	"unstable.build/go-tui/term"
	"unstable.build/go-tui/text"
	"unstable.build/go-tui/text/clipboard"
)

type mode uint8

const (
	normalMode = iota
	logsMode
)

var (
	defaultTextAttr        = term.Attributes{}
	defaultPinAttr         = term.Attributes{Fg: term.AttrBold}
	defaultMatchedTextAttr = term.Attributes{Fg: term.ColorRed}
)

type logsHandler struct {
	mode
	ed             tui.Handler
	width, height  int
	moveMultiplier []rune

	l struct {
		*search.List
		component.Virtual
	}

	matchedTextAttr term.Attributes
	pinAttr         term.Attributes
	textAttr        term.Attributes

	hScroll        int
	pinned         []*component.Virtual
	mouse          text.Mouse
	selectionStart term.Coordinates
}

func newLogsHandler(
	l *search.List, textAttr, matchedTextAttr, pinAttr *term.Attributes,
) tui.Handler {
	ret := new(logsHandler)

	ret.l.List = l
	ret.l.C = ret.l.List
	if matchedTextAttr == nil {
		matchedTextAttr = &defaultMatchedTextAttr
	}
	ret.matchedTextAttr = *matchedTextAttr
	if pinAttr == nil {
		pinAttr = &defaultPinAttr
	}
	ret.pinAttr = *pinAttr
	if textAttr == nil {
		textAttr = &defaultTextAttr
	}
	ret.textAttr = *textAttr

	ret.resetEd()

	ret.l.C = ret.withBackground(ret.l.C)

	ret.mouse.Init(ret)

	return ret
}

func (s *logsHandler) Handle(ev term.Event) (exit, handled bool) {
	switch s.mode {
	case normalMode:
		return s.handleNormal(ev)
	case logsMode:
		return s.handleFilter(ev)
	default:
		panic("unknown mode")
	}
}

func (s *logsHandler) positionFocusPinned() {
	pos := term.Coordinates{Y: s.l.FocusOffset() - s.l.Offset()}
	for _, v := range s.pinned {
		v.Move(pos)
		pos.Y++
	}
}

func (s *logsHandler) setTokenAttr(
	match search.Match, offset int, attrSetter *component.AttrSetter, attr term.Attributes,
) {
	for _, idx := range match.Tokens() {
		attrSetter.SetAttrAt(term.Coordinates{X: idx - offset}, attr)
	}
}

func (s *logsHandler) newPinnedResponsive(match search.Match) {
	cfg := s.defaultStringConfig()

	str := component.StringResponsive(string(match.Data()), cfg)
	attrSetter := component.WithAttrSetter(str)
	s.setTokenAttr(match, 0, attrSetter, s.matchedTextAttr)
	comp := s.withBackground(attrSetter)
	height := str.Height(s.width)

	v := &component.Virtual{C: comp}
	v.Resize(s.width, height)

	s.pinned = s.pinned[:0]
	s.pinned = append(s.pinned, v)
}

func (s *logsHandler) withBackground(comp tui.Component) tui.Component {
	return component.WithBackground(comp,
		term.Cell{Ch: ' ', Fg: s.textAttr.Fg, Bg: s.textAttr.Bg})
}

func (s *logsHandler) setPinned(matches []search.Match) {
	cfg := s.defaultStringConfig()
	s.pinned = s.pinned[:0]

	for _, match := range matches {
		str := component.StringResponsive(string(match.Data()[s.hScroll:]), cfg)
		attrSetter := component.WithAttrSetter(str)
		s.setTokenAttr(match, s.hScroll, attrSetter, s.matchedTextAttr)
		comp := s.withBackground(attrSetter)
		height := s.l.ElementHeight()

		v := &component.Virtual{C: comp}
		v.Resize(s.width, height)

		s.pinned = append(s.pinned, v)
	}
}

func (s *logsHandler) defaultStringConfig() component.StringResponsiveConfig {
	return component.StringResponsiveConfig{
		StringConfig: component.StringConfig{
			Alignment:      component.SpanAlignmentLeft,
			BackgroundRune: ' ',
			Attributes:     s.pinAttr,
		},
	}
}

func (s *logsHandler) newScrolledRight(match search.Match) bool {
	data := match.Data()
	if len(data) < s.width || s.hScroll == len(data)-1-s.width {
		return false
	}
	s.hScroll++
	s.setPinned([]search.Match{match})
	s.positionFocusPinned()
	return true
}
func (s *logsHandler) newScrolledLeft(match search.Match) bool {
	if s.hScroll <= 1 {
		s.pinFocus()
		return false
	}
	s.hScroll--
	s.setPinned([]search.Match{match})
	s.positionFocusPinned()
	return true
}

func (s *logsHandler) scrollLeft() bool {
	data, ok := s.l.Focus()
	if ok {
		return s.newScrolledLeft(data)
	}
	return false
}

func (s *logsHandler) scrollRight() bool {
	data, ok := s.l.Focus()
	if ok {
		return s.newScrolledRight(data)
	}
	return false
}

func (s *logsHandler) pinFocus() {
	s.hScroll = 0
	match, ok := s.l.Focus()
	if !ok {
		return
	}
	s.setPinned([]search.Match{match})
	s.positionFocusPinned()
}

func (s *logsHandler) resetPinned() {
	s.hScroll = 0
	s.setPinned(nil)
}

func (s *logsHandler) setFilterMode() {
	s.mode = logsMode
	s.pinFocus()
}

func (s *logsHandler) moveMultiply() int {
	if len(s.moveMultiplier) == 0 {
		return 1
	}
	n, _ := strconv.Atoi(string(s.moveMultiplier))
	return n
}

func (s *logsHandler) focusUp() bool {
	ret := s.l.FocusUp()
	s.pinFocus()
	return ret
}

func (s *logsHandler) focusDown() bool {
	ret := s.l.FocusDown()
	s.pinFocus()
	return ret
}

func (s *logsHandler) focusStart() bool {
	ret := s.l.FocusStart()
	s.pinFocus()
	return ret
}

func (s *logsHandler) focusEnd() bool {
	ret := s.l.FocusEnd()
	s.pinFocus()
	return ret
}

func (s *logsHandler) handleNormalKey(ev term.Event) (exit, handled bool) {
	switch ev.Key {
	case term.KeyTab:
		s.toggleCaseSensitivity()
		handled = true
	case term.KeyArrowRight:
		handled = s.scrollRight()
	case term.KeyArrowLeft:
		handled = s.scrollLeft()
	case term.KeyEnter:
		s.l.Wait()
		data, ok := s.l.Focus()
		if ok {
			handled = true
			s.newPinnedResponsive(data)
			s.positionFocusPinned()
		}
	case term.KeyCtrlJ, term.KeyArrowDown:
		handled = s.focusDown()
	case term.KeyCtrlK, term.KeyArrowUp:
		handled = s.focusUp()
	default:
		multiplier := s.moveMultiply()
		switch ev.Ch {
		case '/':
			handled = true
			s.setFilterMode()
		case 'g':
			handled = s.focusStart()
		case 'G':
			handled = s.focusEnd()
		case 'l':
			handled = s.scrollRight()
		case 'h':
			handled = s.scrollLeft()
		case '0':
			for handled = true; handled; handled = s.scrollLeft() {
			}
		case '$':
			for handled = true; handled; handled = s.scrollRight() {
			}
		case 'k':
			ok := true
			for i := 0; ok && i < multiplier; i++ {
				ok = s.focusUp()
				if !ok {
					break
				}
				handled = true
			}
		case 'j':
			ok := true
			for i := 0; ok && i < multiplier; i++ {
				ok = s.focusDown()
				if !ok {
					break
				}
				handled = true
			}
		default:
			if ev.Ch >= '0' && ev.Ch <= '9' {
				s.moveMultiplier = append(s.moveMultiplier, ev.Ch)
				return
			}
		}
	}
	s.moveMultiplier = s.moveMultiplier[:0]

	return false, handled
}

func (s *logsHandler) handleNormal(ev term.Event) (exit, handled bool) {
	_, handled = s.mouse.Handle(ev)
	if handled {
		return
	}

	switch ev.Type {
	case term.EventKey:
		return s.handleNormalKey(ev)
	default:
		return
	}
}

func (s *logsHandler) toggleCaseSensitivity() {
	s.l.ToggleCaseSensitivity()
	s.resetPinned()
}

func (s *logsHandler) resetEd() {
	clipboard := clipboard.NewInMemory()
	s.ed, _ = text.DefaultSimpleEditor(clipboard).
		Edit(workspaceapi.RandomURI("logs"), s.l.Buffer())
}

func (s *logsHandler) handleFilter(ev term.Event) (exit, handled bool) {
	_, handled = s.mouse.Handle(ev)
	if handled {
		return
	}

	if ev.Type != term.EventKey {
		return
	}
	switch ev.Key {
	case term.KeyCtrlJ, term.KeyArrowDown:
		handled = s.focusDown()
	case term.KeyCtrlK, term.KeyArrowUp:
		handled = s.focusUp()
	case term.KeyEnter:
		s.mode = normalMode
		handled = true
	case term.KeyTab:
		s.toggleCaseSensitivity()
		handled = true
	case term.KeyBackspace, term.KeyBackspace2:
		_, handled = s.currentEd().Handle(ev)
		s.resetPinned()
		if handled {
			return
		}
		fallthrough
	case term.KeyEsc:
		handled = true
		s.mode = normalMode
		s.l.Buffer().Reset()
		s.resetEd()
	default:
		_, handled = s.currentEd().Handle(ev)
		s.resetPinned()
	}
	return
}

func (s *logsHandler) Cursor() (term.Coordinates, term.CursorStyle, bool) {
	if s.mode == normalMode {
		return term.Coordinates{}, 0, false
	}
	c, style, ok := s.currentEd().Cursor()
	// NOTE: assumes bottomSearchBar is true
	c.Y += (s.l.Height() - s.l.InputHeight())
	return c, style, ok
}

func (s *logsHandler) Man() tui.Manual {
	panic("TODO")
}

func (s *logsHandler) currentEd() tui.Handler {
	return s.ed
}

func (s *logsHandler) ScrollUp(n int) bool {
	ok := s.focusUp()
	for i := 1; ok && i < n; i++ {
		ok = s.focusUp()
	}
	return ok
}

func (s *logsHandler) ScrollDown(n int) bool {
	ok := s.focusDown()
	for i := 1; ok && i < n; i++ {
		ok = s.focusDown()
	}
	return ok
}

func (s *logsHandler) setSelection(to term.Coordinates) {
	s.pinned = s.pinned[:0]

	from, to := cell.SortFromTo(s.selectionStart, to)

	var first component.ListNode
	var matches []search.Match
	var poss []term.Coordinates
	for i := from.Y; i <= to.Y; i++ {
		pos := term.Coordinates{Y: i}
		match, node, ok := s.l.ElementAt(pos)
		if ok {
			if i == from.Y {
				first = node
			}
			matches = append(matches, match)
			poss = append(poss, pos)
		}
	}
	s.setPinned(matches)

	for i, v := range s.pinned {
		v.Move(poss[i])
	}

	if len(matches) != 0 {
		s.l.SetFocus(first)
	}
}

func (s *logsHandler) SetSelectionEnd(pos term.Coordinates) {
	s.setSelection(pos)
}

func (s *logsHandler) SetSelectionStart(pos term.Coordinates) {
	s.selectionStart = pos
	s.setSelection(pos)
}

func (s *logsHandler) ClearSelection() {
	s.pinFocus()
}

func (s *logsHandler) SelectWordAt(pos term.Coordinates) {
	s.SetSelectionStart(pos)
}

func (s *logsHandler) SelectLine(y int) {
	s.SetSelectionStart(term.Coordinates{Y: y})
}

func (s *logsHandler) Width() int {
	return s.width
}

func (s *logsHandler) Height() int {
	return s.height
}

func (s *logsHandler) OnAction(pos term.Coordinates, action text.MouseAction) bool {
	return false
}

// Resize satisfies tui.Component
func (s *logsHandler) Resize(width, height int) {
	s.width, s.height = width, height
	s.l.Virtual.Resize(width, height)
	inputHeight := s.l.InputHeight()
	s.ed.Resize(width, inputHeight)
}

// Draw satisfies tui.Component
func (s *logsHandler) Draw(w term.Writer) {
	s.l.Virtual.Draw(w)
	// overwrite with selection/pinned
	for _, v := range s.pinned {
		v.Draw(w)
	}
}
