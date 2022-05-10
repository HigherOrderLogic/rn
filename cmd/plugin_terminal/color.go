package main

import (
	"image/color"
	"strconv"

	termutil "github.com/ernestrc/go-tui/cmd/plugin_terminal/util"
	"github.com/ernestrc/go-tui/term"
	log "github.com/sirupsen/logrus"
)

// Adjusting these does nothing in practice, they're just used
// as sample indexes for when mapping from RGBA to term.Attribute
var rgbaToTermutil = map[color.RGBA]termutil.Colour{
	{R: 0x1d, G: 0x1f, B: 0x21, A: 0xff}: termutil.ColourBlack,
	{R: 0xcd, G: 0x66, B: 0x66, A: 0xff}: termutil.ColourRed,
	{R: 0xb6, G: 0xbe, B: 0x68, A: 0xff}: termutil.ColourGreen,
	{R: 0xf1, G: 0xc7, B: 0x74, A: 0xff}: termutil.ColourYellow,
	{R: 0x82, G: 0xa3, B: 0xbf, A: 0xff}: termutil.ColourBlue,
	{R: 0xb3, G: 0x95, B: 0xbc, A: 0xff}: termutil.ColourMagenta,
	{R: 0x8b, G: 0xbf, B: 0xb8, A: 0xff}: termutil.ColourCyan,
	{R: 0xc6, G: 0xc9, B: 0xc7, A: 0xff}: termutil.ColourWhite,
	{R: 0x66, G: 0x66, B: 0x66, A: 0xff}: termutil.ColourBrightBlack,
	{R: 0xd6, G: 0x4e, B: 0x53, A: 0xff}: termutil.ColourBrightRed,
	{R: 0xba, G: 0xcb, B: 0x4a, A: 0xff}: termutil.ColourBrightGreen,
	{R: 0xe8, G: 0xc6, B: 0x47, A: 0xff}: termutil.ColourBrightYellow,
	{R: 0x7a, G: 0xa7, B: 0xdb, A: 0xff}: termutil.ColourBrightBlue,
	{R: 0xc4, G: 0x98, B: 0xd9, A: 0xff}: termutil.ColourBrightMagenta,
	{R: 0x70, G: 0xc1, B: 0xb2, A: 0xff}: termutil.ColourBrightCyan,
	{R: 0xeb, G: 0xeb, B: 0xeb, A: 0xff}: termutil.ColourBrightWhite,
	{R: 0x0, G: 0x0, B: 0x0, A: 0xff}:    termutil.ColourBackground,
	{R: 0xc6, G: 0xc9, B: 0xc7, A: 0xff}: termutil.ColourForeground,
	{R: 0x33, G: 0xab, B: 0x33, A: 0xff}: termutil.ColourSelectionBackground,
	{R: 0x1, G: 0x1, B: 0x1, A: 0xff}:    termutil.ColourSelectionForeground,
	{R: 0x1d, G: 0x1f, B: 0x21, A: 0xff}: termutil.ColourCursorForeground,
	{R: 0xc6, G: 0xc9, B: 0xc7, A: 0xff}: termutil.ColourCursorBackground,
}

type colorTheme struct {
	theme   *termutil.Theme
	mapping map[color.RGBA]term.Attribute
}

func newColorTheme() *colorTheme {
	mapping := make(map[color.RGBA]term.Attribute)
	f := termutil.NewThemeFactory()
	for k, c := range rgbaToTermutil {
		f = f.WithColour(c, k)
	}

	theme := f.Build()
	for i := 0; i <= 256; i++ {
		c, err := theme.ColourFrom8Bit(strconv.Itoa(i))
		if err != nil {
			log.Warning(err)
			continue
		}
		cc, ok := c.(color.RGBA)
		if !ok {
			log.Warningf("ColourFrom8Bit returned nil for %d", i)
			continue
		}
		mapping[cc] = term.Attribute(i + 1)
	}
	return &colorTheme{theme: theme, mapping: mapping}
}

func (t *colorTheme) to8Bit(c color.Color) (term.Attribute, bool) {
	r, g, b, a := c.RGBA()
	rgba := color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
	tcolor, ok := t.mapping[rgba]
	return tcolor, ok
}
