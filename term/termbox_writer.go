//go:build !js

package term

import (
	"context"

	"github.com/ernestrc/tcell/v3/termbox"
)

type termboxWriter struct {
	ctx context.Context
}

func newTermboxWriter() *termboxWriter {
	ret := new(termboxWriter)
	ret.ctx = context.Background()
	return ret
}

func (w *termboxWriter) SetCell(pos Coordinates, c Cell) {
	style := termbox.AttributeToStyle(termbox.Attribute(c.Fg), termbox.Attribute(c.Bg))
	termbox.Screen().SetContent(pos.X, pos.Y, c.Ch, c.Combining, c.Width, style)
	return
}

func (w *termboxWriter) Flush() error {
	return termbox.Flush()
}

func (w *termboxWriter) Clear(attr Attributes) (err error) {
	termbox.Clear(termbox.Attribute(attr.Fg), termbox.Attribute(attr.Bg))
	return
}

func (w *termboxWriter) SetCursor(pos Coordinates) {
	termbox.SetCursor(pos.X, pos.Y)
}

func (w *termboxWriter) Context() context.Context {
	return w.ctx
}

func (w *termboxWriter) SetContext(ctx context.Context) {
	w.ctx = ctx
}
