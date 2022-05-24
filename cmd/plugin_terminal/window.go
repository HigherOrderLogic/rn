package main

import (
	"github.com/ernestrc/go-tui/browser"
	termutil "github.com/ernestrc/go-tui/cmd/plugin_terminal/util"
)

var _ termutil.WindowManipulator = (*windowManipulator)(nil)

type windowManipulator struct {
	width, height int
	wm            browser.WindowManager
	m             browser.Messenger
	title         string
}

func newWindowManipulator(
	wm browser.WindowManager, m browser.Messenger,
) *windowManipulator {
	return &windowManipulator{wm: wm}
}

func (w *windowManipulator) State() termutil.WindowState {
	return termutil.StateUnknown
}

func (w *windowManipulator) Minimise() {
}

func (w *windowManipulator) Maximise() {
}

func (w *windowManipulator) Restore() {
}

func (w *windowManipulator) SetTitle(title string) {
	w.title = title
}

func (w *windowManipulator) Position() (int, int) {
	return 0, 0
}

func (w *windowManipulator) SizeInPixels() (int, int) {
	panic("cannot determine size in pixels")
}

func (w *windowManipulator) CellSizeInPixels() (int, int) {
	panic("cannot determine size in pixels")
}

func (w *windowManipulator) SizeInChars() (int, int) {
	return w.height, w.width
}

func (w *windowManipulator) ResizeInPixels(int, int) {
	panic("cannot resize")
}

func (w *windowManipulator) ResizeInChars(height, width int) {
	w.width = width
	w.height = height
}

func (w *windowManipulator) ScreenSizeInPixels() (int, int) {
	panic("cannot determine size in pixels")
}

func (w *windowManipulator) ScreenSizeInChars() (int, int) {
	panic("cannot determine size in pixels")
}

func (w *windowManipulator) Move(x, y int) {
	panic("cannot move window")
}

func (w *windowManipulator) IsFullscreen() bool {
	return false
}

func (w *windowManipulator) SetFullscreen(enabled bool) {
}

func (w *windowManipulator) GetTitle() string {
	return w.title
}

func (w *windowManipulator) SaveTitleToStack() {
}

func (w *windowManipulator) RestoreTitleFromStack() {
}

func (w *windowManipulator) ReportError(err error) {
	w.m.SetMessage("terminal: %s", err)
}
