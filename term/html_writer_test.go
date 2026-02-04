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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/tcell/v3"
)

const defaultBackgroundCursorAtRoot = `<pre style="background:#000000;color:#FFFFFF;"><span style="background: red;">%s</span>%s</pre>`
const defaultBackgroundNoCursor = `<pre style="background:#000000;color:#FFFFFF;">%s</pre>`

func expectInnerHTMLWithCursor(t *testing.T, writer *HTMLWriter, expectedHTML string) {
	require.NoError(t, writer.Flush())
	assert.Equal(t, expectedHTML, writer.HTML())
}

func expectInnerHTML(t *testing.T, writer *HTMLWriter, onCursor string, html string) {
	expectedHTML := fmt.Sprintf(defaultBackgroundCursorAtRoot, onCursor, html)
	expectInnerHTMLWithCursor(t, writer, expectedHTML)
}

func newWriterNoCursor() *HTMLWriter {
	writer := NewHTMLWriter(1, 1)
	writer.cursor = -1
	return writer
}

func TestHTMLWriter(t *testing.T) {
	t.Run("writes a cell", func(t *testing.T) {
		writer := NewHTMLWriter(1, 1)
		writer.SetCell(term.Coordinates{}, term.Cell{Ch: 'a'})
		expectInnerHTML(t, writer, "a", "")
	})

	t.Run("overwrites cells", func(t *testing.T) {
		writer := NewHTMLWriter(1, 1)
		writer.SetCell(term.Coordinates{}, term.Cell{Ch: 'a'})
		writer.SetCell(term.Coordinates{}, term.Cell{Ch: 'b'})
		expectInnerHTML(t, writer, "b", "")
	})

	t.Run("ignores out-of-bounds writes", func(t *testing.T) {
		writer := NewHTMLWriter(1, 1)
		writer.SetCell(term.Coordinates{Y: 1}, term.Cell{Ch: 'X'})
		expectInnerHTML(t, writer, " ", "")
	})

	t.Run("escapes &,>,< runes", func(t *testing.T) {
		needEscape := []rune{'&', '<', '>'}
		writer := NewHTMLWriter(len(needEscape), 1)
		for i, c := range needEscape {
			writer.SetCell(term.Coordinates{X: i}, term.Cell{Ch: c})
		}
		expectInnerHTML(t, writer, "&amp", "&lt&gt")
	})

	t.Run("writes cell attributes in CSS", func(t *testing.T) {
		// NOTE: fix if webasm build is ever relevant again
		t.SkipNow()

		tsuite := []struct {
			attr        term.Attributes
			expectedCSS string
		}{
			{term.Attributes{}, "X"}, /* no style */
			{term.Attributes{Fg: tcell.ColorWhite, Bg: tcell.ColorWhite}, "<span style=\"background:#FFFFFF;\">X</span>"}, /*white is default foreground */
			{term.Attributes{Fg: tcell.ColorBlack, Bg: tcell.ColorBlack}, "<span style=\"color:#000000;\">X</span>"},      /* black is default background */
			{term.Attributes{Fg: tcell.ColorBlue, Bg: tcell.ColorBlue}, "<span style=\"background:#0000FF;color:#0000FF;\">X</span>"},
			{term.Attributes{Fg: tcell.ColorAqua, Bg: tcell.ColorAqua}, "<span style=\"background:#00FFFF;color:#00FFFF;\">X</span>"},
			{term.Attributes{Fg: tcell.ColorGreen, Bg: tcell.ColorGreen}, "<span style=\"background:#00FF00;color:#00FF00;\">X</span>"},
			{term.Attributes{Fg: tcell.ColorFuchsia, Bg: tcell.ColorFuchsia}, "<span style=\"background:#FF00FF;color:#FF00FF;\">X</span>"},
			{term.Attributes{Fg: tcell.ColorRed, Bg: tcell.ColorRed}, "<span style=\"background:#FF0000;color:#FF0000;\">X</span>"},
			{term.Attributes{Fg: tcell.ColorYellow, Bg: tcell.ColorYellow}, "<span style=\"background:#FFFF00;color:#FFFF00;\">X</span>"},
			{term.Attributes{Attrs: tcell.AttrBold}, "<span style=\"font-weight:bold;\">X</span>"},
			{term.Attributes{Attrs: tcell.AttrUnderline}, "<span style=\"text-decoration:underline;\">X</span>"},
			{term.Attributes{Attrs: tcell.AttrReverse}, "<span style=\"background:#FFFFFF;color:#000000;\">X</span>"},
			{term.Attributes{Attrs: tcell.AttrBold | tcell.AttrReverse | tcell.AttrUnderline},
				"<span style=\"font-weight:bold;text-decoration:underline;background:#FFFFFF;color:#000000;\">X</span>"},
			{term.Attributes{Fg: tcell.ColorAqua, Bg: tcell.ColorAqua, Attrs: tcell.AttrBold},
				"<span style=\"font-weight:bold;background:#00FFFF;color:#00FFFF;\">X</span>"},
		}

		for i, tcase := range tsuite {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				writer := newWriterNoCursor()
				writer.SetCell(term.Coordinates{}, term.Cell{Ch: 'X', Attributes: tcase.attr})
				expectedHTML := fmt.Sprintf(defaultBackgroundNoCursor, tcase.expectedCSS)
				expectInnerHTMLWithCursor(t, writer, expectedHTML)
			})
		}
	})
}

func TestHTMLWriterClear(t *testing.T) {
	writer := newWriterNoCursor()
	writer.Clear(term.Attributes{Bg: tcell.ColorAqua, Fg: tcell.ColorFuchsia})
	expectedHTML := `<pre style="background:#00FFFF;color:#FF00FF;"> </pre>`
	expectInnerHTMLWithCursor(t, writer, expectedHTML)
}

func TestHTMLWriterSetCursor(t *testing.T) {
	t.Run("sets cursor", func(t *testing.T) {
		writer := NewHTMLWriter(2, 2)
		writer.SetCursor(term.Coordinates{Y: 1, X: 1})
		expectedHTML := "<pre style=\"background:#000000;color:#FFFFFF;\">  \n <span style=\"background: red;\"> </span></pre>"
		expectInnerHTMLWithCursor(t, writer, expectedHTML)
	})

	t.Run("ignores out-of-bounds coordinates", func(t *testing.T) {
		writer := NewHTMLWriter(2, 2)
		writer.SetCursor(term.Coordinates{Y: 10, X: 1})
		expectedHTML := "<pre style=\"background:#000000;color:#FFFFFF;\">  \n  </pre>"
		expectInnerHTMLWithCursor(t, writer, expectedHTML)
	})

	t.Run("ignores negative coordinates", func(t *testing.T) {
		writer := NewHTMLWriter(2, 2)
		writer.SetCursor(term.Coordinates{X: -1})
		expectedHTML := "<pre style=\"background:#000000;color:#FFFFFF;\">  \n  </pre>"
		expectInnerHTMLWithCursor(t, writer, expectedHTML)
	})
}
