package component

import (
	"math"

	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
)

// Responsive components implement a backpressure mechanism (Height) for
// aggregate components to dynamically resize children based on their contents.
// See Height for more details.
type Responsive interface {
	tui.Component
	// Height allows for children components to return a height hint
	// given a width so a parent component can compose accordingly.
	// The returned height can be overriden at the parent's discretion
	// (i.e. there's simply no height left on the screen)
	// so implementers should expect that on calls to Resize.
	Height(width int) int
}

type StringResponsiveConfig struct {
	// NoSplitWords instructs the underlying string responsive component
	// to attempt to not split words in half when possible.
	NoSplitWords bool
	StringConfig
}

// StringResponsive returns a Responsive implementation of
// a string tui.Component.
func StringResponsive(str string, cfg StringResponsiveConfig) Responsive {
	return Cells(cell.StringToCells(str, cfg.Tabspaces), cfg)
}

// Cells returns a Responsive implementation for a matrix of cells.
func Cells(cells [][]term.Cell, cfg StringResponsiveConfig) Responsive {
	return &respStr{
		cfg: cfg,
		in:  cells,
	}
}

// Buffer wraps a cell.Buffer and returns a tui.Component which satisfies
// Responsive. Note that this is not the most efficient implementation of tui.Component
// for a cell.Buffer. See component.Scroll for more details.
func Buffer(buf *cell.Buffer, cfg StringResponsiveConfig) Responsive {
	return &respBuf{buf: buf, respStr: respStr{cfg: cfg}}
}

// NopResponsive returns a Responsive tui.Component that draws nothing.
func NopResponsive() Responsive {
	return Buffer(cell.NewBuffer(), StringConfig{})
}

type respStr struct {
	cfg StringResponsiveConfig
	in  [][]term.Cell
	out tui.Component
}

type respBuf struct {
	buf           *cell.Buffer
	width, height int
	respStr
}

func (b *respBuf) Height(width int) int {
	b.respStr.in = b.buf.RawCells()
	return b.respStr.Height(width)
}

func (b *respBuf) Resize(width, height int) {
	b.width, b.height = width, height
}

func (b *respBuf) Draw(w term.Writer) {
	b.respStr.in = b.buf.RawCells()
	b.respStr.Resize(b.width, b.height)
	b.respStr.Draw(w)
}

// Height satisfies Responsive.
func (s *respStr) Height(width int) int {
	if width <= 0 {
		return 0
	}
	height := len(s.massageInput(width))
	height += s.cfg.PaddingVertical
	if s.cfg.FrameCharSet != (FrameCharSet{}) {
		height += 2
	}
	return height
}

// Resize satisfies tui.Component.
func (s *respStr) Resize(width, height int) {
	outRaw := s.massageInput(width)
	s.out = newStringComp(outRaw, s.cfg.Attributes, 0,
		s.cfg.BackgroundAttributes, s.cfg.FrameCharSet,
		s.cfg.PaddingHorizontal, s.cfg.PaddingVertical, s.cfg.Alignment, s.cfg.MinWidth)
	s.out.Resize(width, height)
}

// Draw satisfies tui.Component.
func (s *respStr) Draw(w term.Writer) {
	s.out.Draw(w)
}

func (s *respStr) massageInput(width int) [][]term.Cell {
	effectiveWidth := width
	if effectiveWidth > 2 && s.cfg.FrameCharSet != (FrameCharSet{}) {
		effectiveWidth -= 2
	}
	if effectiveWidth > s.cfg.PaddingHorizontal {
		effectiveWidth -= s.cfg.PaddingHorizontal
	}
	var outRaw [][]term.Cell
	for _, col := range s.in {
		if len(col) == 0 {
			outRaw = append(outRaw, col[:])
			continue
		}
		for len(col) > 0 {
			chunkLen := int(math.Min(float64(len(col)), float64(effectiveWidth)))
			if chunkLen == 0 {
				break
			}
			origChunkLen := chunkLen
			// do not split word in half
			for s.cfg.NoSplitWords && origChunkLen != len(col) && chunkLen > 1 && col[chunkLen-1].Ch != ' ' {
				chunkLen--
			}
			// word doesn't fit, split word
			if chunkLen == 1 {
				chunkLen = origChunkLen
			}
			outRaw = append(outRaw, col[:chunkLen])
			col = col[chunkLen:]
		}
	}
	return outRaw
}
