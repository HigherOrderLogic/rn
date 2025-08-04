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

package gui

import (
	"sync"

	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/term"
)

// Option allows configuring an instance of GUI.
type Option func(g *GUI) error

// WithFontFamily defines the opentype font family to use.
// See font.Manager.SetFontByFamilyName for more details.
func WithFontFamily(family string) Option {
	return func(g *GUI) error {
		return g.fontManager.SetFontByFamilyName(family)
	}
}

// WithOpacity sets the initial background opacity.
func WithOpacity(fg, bg float64) Option {
	return func(g *GUI) error {
		g.fgOpacity = fg
		g.bgOpacity = bg
		return nil
	}
}

// WithBackgroundBlur sets the initial background blur.
// It only takes effect if WithTransparentWindow is set to true,
// and WithOpacity has been used to set a non 1 background opacity.
func WithBackgroundBlur(radius int) Option {
	return func(g *GUI) error {
		g.bgBlurRadius = radius
		return nil
	}
}

// WithSize sets the initial width and height of the window in pixels.
func WithSize(width, height int) Option {
	return func(g *GUI) error {
		g.defaultWidth = width
		g.defaultHeight = height
		return nil
	}
}

// WithFontSize defines the size of the default font
// or the font set via WithFontFamily.
func WithFontSize(size float64) Option {
	return func(g *GUI) error {
		return g.fontManager.SetSize(size)
	}
}

// WithFontDPI sets the font DPI of the default font
// or the font set via WithFontFamily.
// If value is 0, the DPI is automatically calculated.
func WithFontDPI(dpi float64) Option {
	return func(g *GUI) error {
		return g.fontManager.SetDPI(dpi)
	}
}

// WithDeviceScale sets the device scale factor of the monitor.
// If value is 0, the device scale is automatically detected.
func WithDeviceScale(scale float64) Option {
	return func(g *GUI) error {
		g.fontManager.SetDeviceScale(scale)
		return nil
	}
}

// WithLigatures enables or disables font ligatures.
func WithLigatures(enable bool) Option {
	return func(g *GUI) error {
		g.enableLigatures = enable
		return nil
	}
}

// WithTransparentWindow enables or disables the ability to
// change the window foreground and background opacity.
func WithTransparentWindow(enable bool) Option {
	return func(g *GUI) error {
		g.enableTransparent = enable
		return nil
	}
}

// WithCursorAttributes defines the colors of the cursor.
func WithCursorAttributes(attr term.Attributes) Option {
	return func(g *GUI) error {
		g.cursorAttributes = attr
		return nil
	}
}

// WithPublishChannel defines the channel responsible for
// processing input events.
func WithPublishChannel(ch chan term.Event) Option {
	return func(g *GUI) error {
		g.updateChan = ch
		return nil
	}
}

// WithLocker defines the locker to be used to synchronize
// access to the GUI's root tui.Handler.
func WithLocker(mu sync.Locker) Option {
	return func(g *GUI) error {
		g.mu = mu
		return nil
	}
}

// WithLineHeightOffset defines positive or negative offset given
// to the font's default line height.
// See font.Manager.SetOffset for more details.
func WithLineHeightOffset(offset float64) Option {
	return func(g *GUI) error {
		return g.fontManager.SetOffset(0, offset)
	}
}

// WithRenderOffset defines the render offset in pixels.
// Default is no offset.
func WithRenderOffset(x, y int) Option {
	return func(g *GUI) error {
		g.renderOffset.X = x
		g.renderOffset.Y = y
		return nil
	}
}

// WithPrintFPS prints the current FPS in the resulting graphical screen.
func WithPrintFPS(print bool) Option {
	return func(g *GUI) error {
		g.printFPS = print
		return nil
	}
}

// Theme is a color theme which defines the default foreground and background colors
// as well as color mappings between colors. Tipically the initial 16-bit colors supported
// by XTERM/ECMA are mapped to arbitrary RGB colors.
type Theme struct {
	Foreground tcell.Color
	Background tcell.Color
	Cursor     tcell.Color
	Colors     map[tcell.Color]tcell.Color
}

// WithColorThemes defines the color themes available for later calls to GUI.SetTheme,
// and initial is used as the default theme.
func WithColorThemes(initial string, themes map[string]Theme) Option {
	return func(g *GUI) error {
		g.initialTheme = initial
		g.colorThemes = themes
		return nil
	}
}
