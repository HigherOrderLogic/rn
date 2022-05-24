package termutil

import (
	"fmt"
	"math"
	"strconv"

	"github.com/ernestrc/go-tui/term"
)

type Theme struct {
	Default term.Attributes
}

var (
	map4Bit = map[uint8]term.Attribute{
		30:  term.ColorBlack,
		31:  term.ColorRed,
		32:  term.ColorGreen,
		33:  term.ColorYellow,
		34:  term.ColorBlue,
		35:  term.ColorMagenta,
		36:  term.ColorCyan,
		37:  term.ColorWhite,
		90:  term.ColorBlack | term.AttrBold,
		91:  term.ColorRed | term.AttrBold,
		92:  term.ColorGreen | term.AttrBold,
		93:  term.ColorYellow | term.AttrBold,
		94:  term.ColorBlue | term.AttrBold,
		95:  term.ColorMagenta | term.AttrBold,
		96:  term.ColorCyan | term.AttrBold,
		97:  term.ColorWhite | term.AttrBold,
		40:  term.ColorBlack,
		41:  term.ColorRed,
		42:  term.ColorGreen,
		43:  term.ColorYellow,
		44:  term.ColorBlue,
		45:  term.ColorMagenta,
		46:  term.ColorCyan,
		47:  term.ColorWhite,
		100: term.ColorBlack | term.AttrBold,
		101: term.ColorRed | term.AttrBold,
		102: term.ColorGreen | term.AttrBold,
		103: term.ColorYellow | term.AttrBold,
		104: term.ColorBlue | term.AttrBold,
		105: term.ColorMagenta | term.AttrBold,
		106: term.ColorCyan | term.AttrBold,
		107: term.ColorWhite | term.AttrBold,
	}
)

func (t *Theme) ColourFrom4Bit(code uint8) term.Attribute {
	colour, ok := map4Bit[code]
	if !ok {
		return term.ColorDefault
	}
	return colour
}

func (t *Theme) ColourFrom8Bit(n string) (term.Attribute, error) {
	index, err := strconv.Atoi(n)
	if err != nil {
		return 0, err
	}

	return term.Attribute(index), nil
}

func (t *Theme) ColourFrom24Bit(r, g, b string) (term.Attribute, error) {
	ri, err := strconv.Atoi(r)
	if err != nil {
		return 0, err
	}
	gi, err := strconv.Atoi(g)
	if err != nil {
		return 0, err
	}
	bi, err := strconv.Atoi(b)
	if err != nil {
		return 0, err
	}

	return term.Attribute((int(math.Floor((float64(ri) / 32))) << 5) +
		(int(math.Floor((float64(gi) / 32))) << 2) +
		int(math.Floor((float64(bi) / 64)))), nil
}

func (t *Theme) ColourFromAnsi(ansi []string, bg bool) (term.Attribute, error) {
	if len(ansi) == 0 {
		return 0, fmt.Errorf("invalid ansi colour code")
	}

	switch ansi[0] {
	case "2":
		if len(ansi) != 4 {
			return 0, fmt.Errorf("invalid 24-bit ansi colour code")
		}
		return t.ColourFrom24Bit(ansi[1], ansi[2], ansi[3])
	case "5":
		if len(ansi) != 2 {
			return 0, fmt.Errorf("invalid 8-bit ansi colour code")
		}
		return t.ColourFrom8Bit(ansi[1])
	default:
		return 0, fmt.Errorf("invalid ansi colour code")
	}
}
