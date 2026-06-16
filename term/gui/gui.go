// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2023-2024 Unstable Build, All Rights Reserved.
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
	"context"
	"errors"
	"fmt"
	"image"
	"sync"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"github.com/unstablebuild/rune-go-sdk/term"
	"github.com/unstablebuild/rune-go-sdk/tui"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term/gui/drawrect"
	"unstable.build/go-tui/term/gui/font"
)

var (
	_ ebiten.Game = (*GUI)(nil)

	// ErrHandlerExited is returned by GUI.Run to indicate that
	// the root tui.Handler exited.
	ErrHandlerExited = errors.New("tui handler exited")
)

const (
	defaultWidth, defaultHeight = 800, 600
	// 90 strikes a good balance between key repeat smoothness and
	// not too taxing on the host's resources.
	tps = 90
)

// GUI implements a graphical TUI runtime as an alternative runtime to what
// the tui packages provides.
type GUI struct {
	ctx               context.Context
	cancelCtx         func()
	mu                sync.Locker
	fontManager       *font.Manager
	updateChan        chan term.Event
	handler           tui.Handler
	writer            *cell.BufferWriter
	mouse             *mouse
	input             *input
	theme             string
	bgOpacity         float64
	fgOpacity         float64
	defaultWidth      int
	defaultHeight     int
	explicitSize      bool
	startPositionX    int
	startPositionY    int
	printFPS          bool
	bgBlurRadius      int
	enableTransparent bool
	enableLigatures   bool
	renderOffset      image.Point
	cursorAttributes  term.Attributes
	defaultAttr       term.Attributes
	renderer          *renderer

	originalColorValues map[tcell.Color]int32
	originalValuesColor map[int32]tcell.Color
	initialTheme        string
	colorThemes         map[string]Theme

	cursor struct {
		pos   term.Coordinates
		style term.CursorStyle
		show  bool
	}

	pendingEvents []term.Event
	needsDraw     bool
	needsRender   bool
	width         int
	height        int
	lastPositionX int
	lastPositionY int
	iteration     int64
	deviceScale   float64
}

// New allocates storage for a new GUI and initializes it with the given
// tui.Handler and options.
func New(handler tui.Handler, options ...Option) (*GUI, error) {
	drawrect.Init()
	const (
		cellOverlapX = 0
		cellOverlapY = 0
	)
	fontManager, err := font.NewManager(cellOverlapX, cellOverlapY)
	if err != nil {
		return nil, fmt.Errorf("font manager: %v", err)
	}
	ret := &GUI{
		mu:               new(sync.Mutex),
		handler:          handler,
		updateChan:       make(chan term.Event, 50),
		bgOpacity:        1,
		fgOpacity:        1,
		fontManager:      fontManager,
		enableLigatures:  true,
		cursorAttributes: term.Attributes{Bg: term.FromTcellColor(tcell.ColorRed)},
		startPositionX:   0,
		startPositionY:   0,
		defaultWidth:     defaultWidth,
		defaultHeight:    defaultHeight,
	}
	ret.input = newInput(ret.fontManager)
	ret.mouse = newMouse(ret.fontManager)
	ret.ctx = context.Background()
	ret.ctx, ret.cancelCtx = context.WithCancel(ret.ctx)

	ret.defaultAttr.Fg = term.FromTcellColor(tcell.ColorWhite)
	ret.defaultAttr.Bg = term.FromTcellColor(tcell.ColorBlack)
	for _, option := range options {
		if err := option(ret); err != nil {
			return nil, fmt.Errorf("option: %w", err)
		}
	}

	// clone original values for restoration
	ret.originalColorValues = tcell.GetColorValues()
	ret.originalValuesColor = tcell.GetValuesColor()

	if ret.initialTheme != "" {
		ret.setTheme(ret.initialTheme, ret.colorThemes[ret.initialTheme])
	}

	// initialze renderer, writer, etc.
	ret.resize(ret.defaultWidth, ret.defaultHeight, ret.fontManager.DeviceScale())

	return ret, nil
}

// Run starts the main loop and runs the graphical TUI with the specified options.
func (g *GUI) Run(title string) error {
	defer g.cancelCtx()

	ebiten.SetScreenClearedEveryFrame(false)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetVsyncEnabled(true)
	ebiten.SetTPS(tps)

	ebiten.SetWindowPosition(g.startPositionX, g.startPositionY)
	width, height := g.defaultWidth, g.defaultHeight
	// On the first launch there is no stored size, so size the window to
	// the screen instead of the small built-in default.
	if !g.explicitSize {
		// ebiten.Monitor can be nil before the window is associated with a
		// monitor; only query its size when one is available.
		if m := ebiten.Monitor(); m != nil {
			if w, h := m.Size(); w > 0 && h > 0 {
				width, height = w, h
			}
		}
	}
	ebiten.SetWindowSize(width, height)

	if g.bgBlurRadius != 0 && g.enableTransparent {
		ebiten.SetWindowBackgroundBlur(g.bgBlurRadius)
	}
	ebiten.SetWindowDecorations(ebiten.DecorationsButtonsOnly)

	gameOpts := g.buildRunGameOptions()
	return ebiten.RunGameWithOptions(g, &gameOpts)
}

// x11WMClass is the ICCCM WM_CLASS instance/class reported by the window.
// It must match StartupWMClass in deploy/rune-linux/rune.desktop so X11
// desktop environments (e.g. KDE Plasma) group the running window under the
// pinned launcher instead of showing a second, generic-icon task.
const x11WMClass = "rune"

func (g *GUI) buildRunGameOptions() ebiten.RunGameOptions {
	return ebiten.RunGameOptions{
		SingleThread:      true,
		ScreenTransparent: g.enableTransparent,
		X11ClassName:      x11WMClass,
		X11InstanceName:   x11WMClass,
	}
}

// Draw satisfies ebiten.Game. It renders the terminal GUI to the ebtien window.
func (g *GUI) Draw(screen *ebiten.Image) {
	if !g.needsRender {
		return
	}
	screen.Clear()
	cells := g.writer.RawCells()
	g.renderer.Draw(screen, cells, g.cursor.show,
		g.cursor.pos, g.cursor.style, float64(g.renderOffset.X),
		float64(g.renderOffset.Y))
	g.needsRender = false
	if g.printFPS {
		ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f", ebiten.ActualFPS()))
	}
}

// Update satisfies ebiten.Game. It's called every time a new frame is to be scheduled.
func (g *GUI) Update() error {
	g.pendingEvents = append(g.pendingEvents, g.mouse.processMouse()...)
	g.pendingEvents = g.input.processEvents(g.pendingEvents)
	needsDraw := g.needsDraw

	var interruptPending bool
loop:
	for {
		select {
		case ev := <-g.updateChan:
			if ev.Type == term.EventInterrupt {
				// a client interrupt is defined as an interrupt that serves no
				// purpose other than to redraw content that was refreshed asynchronously
				clientInterrupt := ev.Raw == nil && ev.UserFunc == nil
				if clientInterrupt && interruptPending {
					continue
				}
				// Only ignore the next if we have already a client interrupt in the queue;
				// We cannot assume that an interrupt with a payload on ev.Raw
				// would be processed like a client interrupt, as it could be part of another
				// caching mechanism or state machine (i.e. handlerrpc's stream).
				interruptPending = interruptPending || clientInterrupt
			}
			g.pendingEvents = append(g.pendingEvents, ev)
		default:
			break loop
		}
	}

	// set context with default iteration
	ctx := tui.ContextWithIteration(g.ctx, g.iteration)

	g.mu.Lock()
	defer g.mu.Unlock()

	for _, ev := range g.pendingEvents {
		switch ev.Type {
		case term.EventInterrupt:
			if ev.UserFunc != nil {
				ev.UserFunc()
				needsDraw = true
				continue
			}
			var payloadCtx context.Context
			if ev.Raw == nil {
				payloadCtx = context.Background() // no iterationID
			} else {
				if id, ok := tui.IterationFromRawBytes(ev.Raw); ok {
					// override writer context with a specific iteration ID
					// that might not be this tick's iteration ID
					payloadCtx = tui.ContextWithIteration(g.ctx, id)
				} else {
					// override writer context with a user-payload
					payloadCtx = term.ContextWithPayload(g.ctx, ev.Raw)
				}
			}
			needsDraw = false
			g.drawHandler(payloadCtx)
		case term.EventError, term.EventResize:
			/* not dispatched by GUI */
		default:
			needsDraw = true
			exit, _ := g.handler.Handle(ev)
			if exit {
				if ebiten.IsFocused() {
					g.lastPositionX, g.lastPositionY = ebiten.WindowPosition()
				}
				return ErrHandlerExited
			}
		}
	}
	if needsDraw {
		g.drawHandler(ctx)
	}

	// Zero the slots before truncating: term.Event contains heap pointers
	// (Err, Raw, UserFunc, Context) and a bare [:0] reslice would otherwise
	// keep payloads from previously-buffered events reachable until overwritten.
	clear(g.pendingEvents)
	g.pendingEvents = g.pendingEvents[:0]
	g.iteration++
	return nil
}

// Layout satisfies ebiten.Game. It provides the terminal gui size in pixels.
func (g *GUI) Layout(width, height int) (int, int) {
	s := g.fontManager.DeviceScale()

	// resize handler only if effective size has changed
	if g.width != width || g.height != height || g.deviceScale != s {
		if g.deviceScale != s {
			g.log(log.DebugLevel, "reloading font due to "+
				"device scale change: %f vs %f", g.deviceScale, s)
			// reloading font shouldn't really fail
			_ = g.fontManager.ReloadFont()
		}
		g.mu.Lock()
		g.resize(width, height, s)
		g.mu.Unlock()
	}

	return int(float64(width) * s), int(float64(height) * s)
}

// MinimizeWindow minimizes the window.
func (g *GUI) MinimizeWindow() {
	ebiten.MinimizeWindow()
}

// MaximizeWindow maximizes the window.
func (g *GUI) MaximizeWindow() {
	ebiten.MaximizeWindow()
}

// RestoreWindow restores the window from its maximized or minimized state.
func (g *GUI) RestoreWindow() {
	ebiten.RestoreWindow()
}

// SetFullscreen changes the current mode to fullscreen or not.
//
// When on, the GUI screen is automatically enlarged
// to fit with the monitor. The current scale value is ignored.
//
// On desktops, Ebitengine uses 'windowed' fullscreen mode, which doesn't change
// your monitor's resolution.
//
// SetFullscreen does nothing on macOS when the window is fullscreened
// natively by the macOS desktop instead of SetFullscreen(true).
func (g *GUI) SetFullscreen(fullscreen bool) {
	ebiten.SetFullscreen(fullscreen)
}

// SetWindowPosition sets the window position.
// The position is an offset from the upper-left corner of the current monitor,
// in device-independent pixels.
// It sets the original window position in fullscreen mode.
func (g *GUI) SetWindowPosition(x, y int) {
	ebiten.SetWindowPosition(x, y)
}

// SetWindowSize sets the window size,
// even if the application is in fullscreen mode,
// it will set the original window size.
//
// SetWindowSize panics if width or height is not a positive number.
func (g *GUI) SetWindowSize(width, height int) {
	ebiten.SetWindowSize(width, height)
}

// IncreaseFontSize increases the size of the rendered font,
// making the interface appear bigger.
func (g *GUI) IncreaseFontSize() error {
	err := g.fontManager.IncreaseSize()
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// DecreaseFontSize increases the size of the rendered font,
// making the interface appear bigger.
func (g *GUI) DecreaseFontSize() error {
	err := g.fontManager.DecreaseSize()
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// IncreaseCellWidth increases the width of the rendered font cells.
func (g *GUI) IncreaseCellWidth() error {
	err := g.fontManager.IncreaseCellWidth()
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// DecreaseCellWidth decreases the width of the rendered font cells.
func (g *GUI) DecreaseCellWidth() error {
	err := g.fontManager.DecreaseCellWidth()
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// IncreaseLineHeight increases the size of the rendered font,
// making the interface appear bigger.
func (g *GUI) IncreaseLineHeight() error {
	err := g.fontManager.IncreaseLineHeight()
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// DecreaseLineHeight increases the size of the rendered font,
// making the interface appear bigger.
func (g *GUI) DecreaseLineHeight() error {
	err := g.fontManager.DecreaseLineHeight()
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// SetFont sets the font collection identified by the given family name.
// If family is set to an empty string, the default builtin font is used.
func (g *GUI) SetFont(family string) error {
	err := g.fontManager.SetFontByFamilyName(family)
	if err == nil {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	return err
}

// SetTheme sets the color theme to be used in the next render iteration.
// The theme must have been passed via WithColorThemes option before, otherwise
// this function returns an error.
func (g *GUI) SetTheme(name string) (Theme, error) {
	defer g.resize(g.width, g.height, g.fontManager.DeviceScale())

	g.resetTheme()
	if name == "" {
		return Theme{}, nil
	}

	theme, ok := g.colorThemes[name]
	if !ok {
		return Theme{}, fmt.Errorf("unkown theme '%s'", name)
	}
	g.setTheme(name, theme)
	return theme, nil
}

// Theme returns the current theme. An empty string
// indicates that no theme is set, so the default color scheme,
// is used.
func (g *GUI) Theme() string {
	return g.theme
}

// Themes returns the list of themes configured via WithColorThemes.
func (g *GUI) Themes() []string {
	var themes []string
	for name := range g.colorThemes {
		themes = append(themes, name)
	}
	return themes
}

// SetOpacity sets the background and foreground opacity.
// It has no effect if WithTransparentBackground has not
// been passed as an option to this GUI.
func (g *GUI) SetOpacity(background, foreground float64) {
	g.bgOpacity = background
	g.fgOpacity = foreground
	g.resize(g.width, g.height, g.fontManager.DeviceScale())
}

// SetBackgroundBlur sets the background blur of the window.
// It has no effect until the opacity is changed to be < 1.
// It has no effect if WithTransparentBackground has not
// been passed as an option to this GUI.
func (g *GUI) SetBackgroundBlur(radius int) {
	// if user is trying to remove blur, it will naturally
	// try to set it to 0, but radius 0 is interpreted as no-op
	// by ebiten.
	if radius == 0 {
		radius = 1
	}
	ebiten.SetWindowBackgroundBlur(radius)
	g.resize(g.width, g.height, g.fontManager.DeviceScale())
}

// AvailableFontFamilies returns an iterator with the available
// font families on the system.
func (g *GUI) AvailableFontFamilies() (iterator.Iterator[string], error) {
	return g.fontManager.AvailableFontFamilies()
}

// Size returns the current window width and height in pixels.
func (g *GUI) Size() (width, height int) {
	return g.width, g.height
}

// LastPosition returns the last window position in pixels offset, before
// GUI exits. If GUI is still running, this method returns 0, 0.
func (g *GUI) LastPosition() (x, y int) {
	return g.lastPositionX, g.lastPositionY
}

func (g *GUI) drawHandler(ctx context.Context) {
	g.writer.SetContext(ctx)
	_ = g.writer.Clear(g.defaultAttr)
	g.handler.Draw(g.writer)
	g.cursor.pos, g.cursor.style, g.cursor.show = g.handler.Cursor()
	g.needsDraw = false
	g.needsRender = true
}

func (g *GUI) resize(width, height int, deviceScale float64) {
	g.width = width
	g.height = height
	g.deviceScale = deviceScale

	cellsWidth := g.fontManager.CellsWidth(g.width)
	cellsHeight := g.fontManager.CellsHeight(g.height)
	g.log(log.DebugLevel, "resize: pixels width: %d, height: %d, "+
		"device scale %f; cells width: %d, height: %d",
		width, height, deviceScale, cellsWidth, cellsHeight)

	g.handler.Resize(cellsWidth, cellsHeight)
	g.mouse.resize(cellsWidth, cellsHeight)
	g.writer = cell.NewBufferWriter(g.ctx, cellsWidth, cellsHeight)
	g.renderer = newRenderer(g.width, g.height, g.deviceScale,
		g.fontManager, g.bgOpacity, g.fgOpacity, g.enableLigatures,
		g.cursorAttributes, g.defaultAttr)
	g.needsDraw = true
}

func (g *GUI) log(level log.Level, msg string, args ...any) {
	if !log.IsLevelEnabled(level) {
		return
	}
	log.WithFields(log.Fields{
		logging.KeyClass: "gui",
	}).Logf(level, msg, args...)
}

func (g *GUI) resetTheme() {
	tcell.SetColorValues(g.originalColorValues)

	g.defaultAttr.Fg = term.FromTcellColor(tcell.ColorWhite)
	g.defaultAttr.Bg = term.FromTcellColor(tcell.ColorBlack)
	g.cursorAttributes = term.Attributes{Bg: term.FromTcellColor(tcell.ColorRed)}
}

func (g *GUI) setTheme(name string, theme Theme) {
	m := make(map[tcell.Color]int32, len(theme.Colors))
	for color, value := range theme.Colors {
		m[color] = value.Hex()
	}
	tcell.MergeColorValues(m)

	g.theme = name
	g.defaultAttr.Fg = term.FromTcellColor(theme.Foreground)
	g.defaultAttr.Bg = term.FromTcellColor(theme.Background)
	g.cursorAttributes = term.Attributes{Bg: term.FromTcellColor(theme.Cursor)}
}
