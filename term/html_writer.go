// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package term

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
)

const (
	defaultCursorStyle = "background: red;"
)

// HTMLWriter implements term.Writer by rendering an HTML representation.
type HTMLWriter struct {
	cellbuf       []term.Cell
	buffer        bytes.Buffer
	cursor        int
	width, height int
	defaultAttr   term.Attributes
	cursorStyle   string
}

// NewHTMLWriter allocates storage for a new HTMLWriter and initializes it.
func NewHTMLWriter(width, height int) (t *HTMLWriter) {
	t = new(HTMLWriter)
	t.defaultAttr = term.Attributes{Bg: tcell.ColorBlack, Fg: tcell.ColorWhite}
	t.Resize(width, height)
	t.cursorStyle = defaultCursorStyle
	return
}

// SetCursorStyle sets the CSS style of the cursor.
func (w *HTMLWriter) SetCursorStyle(style string) {
	w.cursorStyle = style
}

// Resize satisfies Writer.
func (w *HTMLWriter) Resize(width, height int) {
	w.width, w.height = width, height
	w.cellbuf = make([]term.Cell, width*height)
}

// SetCell satisfies Writer.
func (w *HTMLWriter) SetCell(pos term.Coordinates, cell term.Cell) {
	if outOfBounds(w.height, w.width, pos) {
		return
	}
	idx := pos.Y*w.width + pos.X
	w.cellbuf[idx] = cell
}

// UnionAttributes satisfies Writer.
func (w *HTMLWriter) UnionAttributes(pos term.Coordinates, attr term.Attributes) {
	if outOfBounds(w.height, w.width, pos) {
		return
	}
	idx := pos.Y*w.width + pos.X
	w.cellbuf[idx].Attributes = term.AttributesUnion(w.cellbuf[idx].Attributes, attr)
}

func (w *HTMLWriter) convertToCSS(attr term.Attributes, ignoreDefault bool) (
	css string, needsFg, needsBg bool,
) {
	var builder strings.Builder

	if attr.Attrs&tcell.AttrBold != 0 {
		attr.Attrs &^= tcell.AttrBold
		needsFg = true
		builder.WriteString("font-weight:bold;")
	}

	if attr.Attrs&tcell.AttrUnderline != 0 {
		attr.Attrs &^= tcell.AttrUnderline
		needsFg = true
		builder.WriteString("text-decoration:underline;")
	}

	fgReverse := attr.Attrs&tcell.AttrReverse != 0
	bgReverse := attr.Attrs&tcell.AttrReverse != 0
	if fgReverse || bgReverse {
		if fgReverse {
			attr.Attrs &^= tcell.AttrReverse
		}
		if bgReverse {
			attr.Attrs &^= tcell.AttrReverse
		}

		// at this point all attributes should be removed
		if attr.Fg == 0 {
			attr.Fg = w.defaultAttr.Fg
		}
		if attr.Bg == 0 {
			attr.Bg = w.defaultAttr.Bg
		}
		tempBg := attr.Bg
		attr.Bg = attr.Fg
		attr.Fg = tempBg
	}

	bgHex := attr.Bg.CSS()
	needsBg = attr.Bg != tcell.ColorDefault
	if !ignoreDefault || needsBg {
		builder.WriteString("background:")
		builder.WriteString(bgHex)
		builder.WriteString(";")
	}

	fgHex := attr.Fg.CSS()
	needsColorFg := attr.Fg != tcell.ColorDefault
	if !ignoreDefault || needsColorFg {
		builder.WriteString("color:")
		builder.WriteString(fgHex)
		builder.WriteString(";")
	}

	return builder.String(),
		needsFg || needsColorFg || !ignoreDefault,
		needsBg || !ignoreDefault
}

func (w *HTMLWriter) writeCellStyle(i int, c term.Cell) bool {
	if w.cursor == i {
		w.buffer.WriteString("<span style=\"")
		w.buffer.WriteString(w.cursorStyle)
		w.buffer.WriteString("\">")
		return true
	}

	styleStr, needsFg, needsBg := w.convertToCSS(
		term.Attributes{Bg: c.Attributes.Bg, Fg: c.Attributes.Fg}, true)
	if !needsFg && !needsBg {
		return false
	}

	if !needsBg && c.Ch == ' ' {
		return false
	}

	w.buffer.WriteString("<span style=\"")
	w.buffer.WriteString(styleStr)
	w.buffer.WriteString("\">")

	return true
}

func escapeRuneHTML(c rune) string {
	switch c {
	case '&':
		return "&amp"
	case '<':
		return "&lt"
	case '>':
		return "&gt"
	default:
		panic(fmt.Sprintf("unable to escape rune: %c", c))
	}
}

func (w *HTMLWriter) writeCells() {
	for i, c := range w.cellbuf {
		if i != 0 && i%w.width == 0 {
			w.buffer.WriteRune('\n')
		}
		switch c.Ch {
		case '<', '>', '&':
			ok := w.writeCellStyle(i, c)
			w.buffer.WriteString(escapeRuneHTML(c.Ch))
			if ok {
				w.buffer.WriteString("</span>")
			}
			continue
		case '\t', '\n', 0:
			c.Ch = ' '
		}

		// TODO we should try to optimize to conflate
		// the contingent spans with the same style.
		ok := w.writeCellStyle(i, c)
		w.buffer.WriteRune(c.Ch)
		if ok {
			w.buffer.WriteString("</span>")
		}
	}
}

// Flush satisfies Writer.
func (w *HTMLWriter) Flush() (err error) {
	defStyle, _, _ := w.convertToCSS(w.defaultAttr, false)
	w.buffer.WriteString("<pre style=\"")
	w.buffer.WriteString(defStyle)
	w.buffer.WriteString("\">")

	w.writeCells()
	w.buffer.WriteString("</pre>")
	return
}

// Clear satisfies Writer.
func (w *HTMLWriter) Clear(attr term.Attributes) error {
	w.cellbuf = make([]term.Cell, w.width*w.height)
	w.buffer.Reset()
	w.defaultAttr = attr
	return nil
}

// SetCursor satisfies Writer.
func (w *HTMLWriter) SetCursor(pos term.Coordinates) {
	i := pos.X + pos.Y*w.width
	if i < len(w.cellbuf) {
		w.cursor = i
	} else {
		w.cursor = -1
	}
}

// HTML returns the flushed contents of this writer in HTML.
func (w *HTMLWriter) HTML() string {
	return w.buffer.String()
}

func outOfBounds(height, width int, pos term.Coordinates) bool {
	return pos.X >= width || pos.Y >= height || pos.X < 0 || pos.Y < 0
}
