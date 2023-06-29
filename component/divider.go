package component

import (
	"strings"

	"unstable.build/go-tui/term"
)

// Divider returns a divider compatible with Responsive collections
// that will simply draw a line that will occupy perc of its width.
func Divider(perc float64, cfg StringConfig) Responsive {
	return &divider{perc: perc, cfg: cfg}
}

type divider struct {
	perc float64
	cfg  StringConfig
	comp Virtual
}

func (d *divider) Resize(width, height int) {
	totalWidth := int(float64(width) * d.perc)

	var builder strings.Builder
	for i := 0; i < totalWidth; i++ {
		builder.WriteRune(d.cfg.FrameCharSet.HorizontalTop)
	}

	d.comp.C = NewStringWithConfig(builder.String(), d.cfg)

	offsetX := int((float64(width) - float64(totalWidth)) / 2)
	offsetY := int(float64(height) / 2)
	d.comp.Resize(totalWidth, 1)
	d.comp.Move(term.Coordinates{X: offsetX, Y: offsetY})
}

func (d *divider) Draw(w term.Writer) {
	d.comp.Draw(w)
}

func (d *divider) Height(width int) int {
	return 1
}
