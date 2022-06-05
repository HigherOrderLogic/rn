package term

type dimWriter struct {
	w Writer
}

func DimWriter(w Writer) Writer {
	return &dimWriter{w: w}
}

// DimAttr removes all attribute flags from attr and
// dims the color, except if color is already black,
// in which case it changes it to gray.
func DimAttr(attr Attribute) Attribute {
	// remove attributes
	attr &= 0x1ff

	switch attr {
	case 0:
		attr = 103
	case 1, 17, 233:
		/* cannot dim a black */
	case 53, 89, 125, 161, 197:
		attr -= 36
	case 18, 19, 20, 21, 22, 54, 55, 56, 57, 58,
		90, 91, 92, 93, 94, 126, 127, 128, 129, 130,
		162, 163, 164, 165, 166, 198, 199, 200, 201, 202,
		234, 235, 236, 237, 238, 239, 240, 241, 242, 243,
		244, 245, 246, 247, 248, 249, 250, 251, 252, 253,
		254, 255, 256:
		attr--
	case ColorRed:
		attr = 168
	case ColorGreen:
		attr = 71
	case ColorYellow:
		attr = 137
	case ColorBlue:
		attr = 238
	case 6:
		attr = 131
	case 7:
		attr = 112
	case 8:
		attr = 250
	case 9:
		attr = 238
	case 10:
		attr = ColorRed
	case 11:
		attr = ColorGreen
	case 12:
		attr = ColorYellow
	case 13:
		attr = ColorBlue
	case 14:
		attr = 6
	case 15:
		attr = 70
	case 16:
		attr = 8
	default:
		attr -= 6
	}
	return attr
}

func dimAttrs(attrs Attributes) Attributes {
	attrs.Fg = DimAttr(attrs.Fg)
	attrs.Bg = attrs.Bg
	return attrs

}

func (w *dimWriter) SetCell(pos Coordinates, c Cell) {
	attr := dimAttrs(Attributes{Fg: c.Fg, Bg: c.Bg})
	c.Fg = attr.Fg
	c.Bg = attr.Bg
	w.w.SetCell(pos, c)
}

func (w *dimWriter) Flush() error {
	return w.w.Flush()
}

func (w *dimWriter) Clear(attr Attributes) error {
	attr = dimAttrs(attr)
	return w.w.Clear(attr)
}

func (w *dimWriter) SetCursor(pos Coordinates) {
	w.w.SetCursor(pos)
}
