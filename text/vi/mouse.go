package vi

import (
	"github.com/ernestrc/go-tui/component"
	"github.com/ernestrc/go-tui/term"
	"github.com/ernestrc/go-tui/text"
)

// satisfies text.MouseDelegate
type mouseDelegate struct {
	vi *viHandlerImpl
}

func newMouseDelegate(vi *viHandlerImpl) text.MouseDelegate {
	return &mouseDelegate{vi: vi}
}

func (d *mouseDelegate) scroll() *component.Scroll {
	return d.vi.less.Scroll()
}

func (d *mouseDelegate) OnAction(pos term.Coordinates, action text.MouseAction) bool {
	return false
}

func (d *mouseDelegate) ScrollUp(n int) (ok bool) {
	for i := 0; i < n; i++ {
		ok = d.scroll().SeekUp()
		if !ok {
			return
		}
	}
	return
}

func (d *mouseDelegate) ScrollDown(n int) (ok bool) {
	for i := 0; i < n; i++ {
		ok = d.scroll().SeekDown()
		if !ok {
			return
		}
	}
	return
}

func (d *mouseDelegate) SetSelectionStart(pos term.Coordinates) {
	if _, ok := d.vi.cursor.SelectionMode(); ok {
		d.vi.cursor.Unselect()
	}
	pos = d.vi.cursor.ScrollCoordinates(pos)
	d.vi.cursor.MoveToScroll(pos)
	d.vi.cursor.Select()
	d.vi.setVisualMode()
}

func (d *mouseDelegate) SetSelectionEnd(pos term.Coordinates) {
	if _, ok := d.vi.cursor.SelectionMode(); !ok {
		return
	}
	pos = d.vi.cursor.ScrollCoordinates(pos)
	d.vi.cursor.MoveToScroll(pos)
}

func (d *mouseDelegate) ClearSelection() {
	d.vi.setNormalMode()
	d.vi.cursor.Unselect()
}

func (d *mouseDelegate) SelectWordAt(pos term.Coordinates) {
	start, end, word := d.scroll().WordAt(pos)
	if word == "" {
		return
	}
	d.SetSelectionStart(start)
	d.SetSelectionEnd(end)
}

func (d *mouseDelegate) SelectLine(y int) {
	if _, ok := d.vi.cursor.SelectionMode(); ok {
		d.vi.cursor.Unselect()
	}
	pos := d.vi.cursor.ScrollCoordinates(term.Coordinates{Y: y})
	d.vi.cursor.MoveToScroll(pos)
	d.vi.cursor.SelectLine()
}

func (d *mouseDelegate) Width() int {
	return d.scroll().Width()
}

func (d *mouseDelegate) Height() int {
	return d.scroll().Height()
}
