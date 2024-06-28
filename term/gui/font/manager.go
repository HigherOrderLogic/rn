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
package font

import (
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/ernestrc/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/fontinfo"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"unstable.build/go-tui/term/gui/font/builtinfont"
)

// Manager manages the underlying font.Face used to render
// characters on screen. It also exposes methods to help calculate
// the width and height in cells and pixels of the given screen.
//
// If SetFontByFamilyName is not called, a builtin font is used.
type Manager struct {
	family         string
	regularFace    font.Face
	boldFace       font.Face
	italicFace     font.Face
	boldItalicFace font.Face
	size           float64
	dpi            float64
	staticDevScale float64
	charSize       CharSize
	offset         fixed.Point26_6
	cellOffsetY    float64
}

// CharSize represent a character dimensions in pixels.
type CharSize struct {
	X float64
	Y float64
}

// NewManager allocates storage for a new Manager and initializes it
// with the default font and dpi.
func NewManager() *Manager {
	ret := &Manager{
		size: 16,
		dpi:  72,
	}
	// TODO use builtin glyphs for special characters that
	// need specific offsets. This is how alacritty always
	// gets pixel perfect frame borders.
	// We can achieve this by wrapping a font.Face
	// with a custom font.Face that provides builtin glyphs
	// for a known whitelist of characters, and uses the underlying
	// font's glyphs for the rest.
	ret.offset.Y = fixed.Int26_6(float64(1.0) * (1 << 6))
	// ret.offset.X = 58 // between 0.5 and 1.0
	return ret
}

// IncreaseSize increases the size of the font by 1.
func (m *Manager) IncreaseSize() error {
	return m.SetSize(m.size + 1)
}

// IncreaseSize decreases the size of the font by 1.
func (m *Manager) DecreaseSize() error {
	if m.size <= 1 {
		return nil
	}
	return m.SetSize(m.size - 1)
}

// DPI returns the configured DPI for the underlying font.
func (m *Manager) DPI() float64 {
	return m.dpi
}

// SetDPI sets the DPI of the configured font. It will
// reload the font with the new DPI and return
// an error if there was a problem reloading font.
func (m *Manager) SetDPI(dpi float64) error {
	if dpi <= 0 {
		panic(errors.New("DPI must be >0"))
	}
	m.dpi = dpi
	err := m.SetFontByFamilyName(m.family)
	if err != nil {
		return fmt.Errorf("reload font: %w", err)
	}
	return nil
}

// SetSize sets the size of the configured font.
// It will reload the font with the new DPI and return
// an error if there was a problem reloading the font.
func (m *Manager) SetSize(size float64) error {
	m.size = size
	// effectively reload fonts at new size
	if err := m.SetFontByFamilyName(m.family); err != nil {
		return fmt.Errorf("reload font: %w", err)
	}
	return nil
}

// SetFontByFamilyName finds the given font installed on the system and
// sets it as the configured font, or returns an error if there was
// a problem loading the given font.
func (m *Manager) SetFontByFamilyName(name string) error {
	m.family = name
	if name == "" {
		return m.loadDefaultFonts()
	}
	if name == "builtin" {
		return m.loadFallbackFont()
	}

	return m.findAndLoadFont(name)
}

// SetDeviceScale forces the device scale to the given value.
//
// Note that after this method is called, the device scale
// won't be adjusted dynamically. This is only meant to be run
// on systems where the device scale detector is not working properly.
func (m *Manager) SetDeviceScale(value float64) {
	m.staticDevScale = value
}

// DeviceScale returns the device scale factor of the current screen.
func (m *Manager) DeviceScale() float64 {
	if m.staticDevScale != 0 {
		return m.staticDevScale
	}
	// this cannot be cached otherwise moving window across screens with
	// different DPIs wouldn't adjust the device scale factor.
	return deviceScale()
}

// CharSize returns the character dimensions in pixels
// of the configured font.
func (m *Manager) CharSize() CharSize {
	m.ensureFontLoaded()
	return m.charSize
}

// OffsetY returns the y-offset for rendering a grid of character cells.
func (m *Manager) OffsetY() float64 {
	m.ensureFontLoaded()
	return m.cellOffsetY
}

// CellsWidth returns the total width in cells of the current screen.
func (m *Manager) CellsWidth(width int) int {
	m.ensureFontLoaded()
	return int(math.Max(1, math.Floor(m.cellsWidth(width))))
}

// CellsHeight returns the total height in cells of the current screen.
func (m *Manager) CellsHeight(height int) int {
	m.ensureFontLoaded()
	return int(math.Max(1, math.Floor(m.cellsHeight(height))))
}

// ImageWidth returns the total width in pixels of the current screen.
func (m *Manager) ImageWidth(width int) int {
	m.ensureFontLoaded()
	return int(math.Max(1, math.Floor(m.cellsWidth(width)*m.CharSize().X)))
}

// ImageHeight returns the total height in pixels of the current screen.
func (m *Manager) ImageHeight(height int) int {
	m.ensureFontLoaded()
	return int(math.Max(1, math.Floor(m.cellsHeight(height)*m.CharSize().Y)))
}

// RegularFontFace returns the configured regular font.Face.
func (m *Manager) RegularFontFace() font.Face {
	m.ensureFontLoaded()
	return m.regularFace
}

// BoldFontFace returns the configured bold font.Face or the fallback
// if no bold font face was found when loading the font.
func (m *Manager) BoldFontFace() font.Face {
	if m.boldFace == nil {
		return m.RegularFontFace()
	}
	return m.boldFace
}

// BoldFontFace returns the configured italic font.Face or the fallback
// if no italic font face was found when loading the font.
func (m *Manager) ItalicFontFace() font.Face {
	if m.italicFace == nil {
		return m.RegularFontFace()
	}
	return m.italicFace
}

// BoldFontFace returns the configured bold and italic font.Face or the fallback
// if no bold and italic font face was found when loading the font.
func (m *Manager) BoldItalicFontFace() font.Face {
	if m.boldItalicFace == nil {
		if m.boldFace == nil {
			return m.ItalicFontFace()
		}
		return m.BoldFontFace()
	}
	return m.boldItalicFace
}

// AvailableFontFamilies returns a list of available font families.
func (m *Manager) AvailableFontFamilies() (iterator.Iterator[string], error) {
	fonts, err := fontinfo.Match()
	if err != nil {
		return nil, fmt.Errorf("search fonts: %w", err)
	}

	var ret []string
	for _, font := range fonts {
		ret = append(ret, font.Family)
	}
	return iterator.FromSlice[string](ret), nil
}

func (m *Manager) ensureFontLoaded() {
	if m.regularFace == nil {
		err := m.loadDefaultFonts()
		if err != nil {
			panic(fmt.Sprintf("could not load the default fonts: %v", err))
		}
	}
}

func (m *Manager) cellsWidth(width int) float64 {
	return float64(width) * m.DeviceScale() / m.charSize.X
}

func (m *Manager) cellsHeight(height int) float64 {
	return float64(height) * m.DeviceScale() / m.charSize.Y
}

func (m *Manager) loadDefaultFonts() error {
	err := m.findAndLoadFont(defaultFamily())
	if err != nil {
		if lerr := m.loadFallbackFont(); lerr != nil {
			return multierror.Append(err, lerr)
		}
	}
	return nil
}

func (m *Manager) loadFallbackFont() error {
	regular, err := opentype.Parse(builtinfont.MesloLGMRegularTTF)
	if err != nil {
		return err
	}
	m.regularFace, err = m.createFace(regular)
	if err != nil {
		return err
	}

	bold, err := opentype.Parse(builtinfont.MesloLGMBoldTTF)
	if err != nil {
		return err
	}
	m.boldFace, err = m.createFace(bold)
	if err != nil {
		return err
	}

	italic, err := opentype.Parse(builtinfont.MesloLGMItalicTTF)
	if err != nil {
		return err
	}
	m.italicFace, err = m.createFace(italic)
	if err != nil {
		return err
	}

	boldItalic, err := opentype.Parse(builtinfont.MesloLGMBoldItalicTTF)
	if err != nil {
		return err
	}
	m.boldItalicFace, err = m.createFace(boldItalic)
	if err != nil {
		return err
	}

	return m.calcMetrics()
}

func (m *Manager) loadFontFace(path string) (err error) {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	col, err := opentype.ParseCollectionReaderAt(f)
	if err != nil {
		return fmt.Errorf("opentype parse collection: %w", err)
	}
	var buf sfnt.Buffer
	for i := 0; i < col.NumFonts(); i++ {
		font, err := col.Font(i)
		if err != nil {
			return fmt.Errorf("font %d: %w", i, err)
		}
		face, err := m.createFace(font)
		if err != nil {
			return fmt.Errorf("create %d opentype face: %w", i, err)
		}
		subfamily, err := font.Name(&buf, sfnt.NameIDSubfamily)
		if err != nil {
			return fmt.Errorf("read font %d subfamily: %w", i, err)
		}
		switch subfamily {
		case "Regular":
			m.regularFace = face
		case "Bold":
			m.boldFace = face
		case "Italic", "Oblique":
			m.italicFace = face
		case "Bold Italic", "Bold Oblique":
			m.boldItalicFace = face
		default:
			m.log(log.DebugLevel, "skipping subfamily: %q", subfamily)
		}
	}
	return nil
}

func (m *Manager) findAndLoadFont(name string) error {
	fonts, err := fontinfo.Match(fontinfo.MatchFamily(name))
	if err != nil {
		return fmt.Errorf("find font with family '%s': %w", name, err)
	}
	if len(fonts) == 0 {
		return fmt.Errorf("could not find font with family '%s'", name)
	}

	for _, fontMeta := range fonts {
		err := m.loadFontFace(fontMeta.Path)
		if err != nil {
			return err
		}
	}

	if m.regularFace == nil {
		return fmt.Errorf("could not find regular style for font family '%s'", name)
	}

	return m.calcMetrics()
}

func (m *Manager) createFace(f *sfnt.Font) (font.Face, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    m.size,
		DPI:     m.dpi,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("opentype new face: %w", err)
	}
	return face, nil
}

func (m *Manager) calcMetrics() error {
	face := m.regularFace
	bounds, advance, _ := face.GlyphBounds([]rune("█")[0])

	m.charSize.X = math.Max(0, float64((advance-bounds.Min.X+m.offset.X)/(1<<6)))
	m.charSize.Y = math.Max(0, float64((bounds.Max.Sub(bounds.Min).Y+m.offset.Y)/(1<<6)))
	m.cellOffsetY = float64(-bounds.Min.Y / (1 << 6))
	return nil
}

func (p *Manager) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "font.Manager",
	}).Logf(level, msg, args...)
}
