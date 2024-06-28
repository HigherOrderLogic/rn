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
	"context"
	"errors"
	"fmt"
	"image"
	"sync"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	"github.com/unstablebuild/tcell/v3"
	"unstable.build/go-tui"
	"unstable.build/go-tui/cell"
	"unstable.build/go-tui/term"
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
	// tps                         = int(1*time.Second/defaultKeyPressRepeat) + 1
	tps = 60
)

// GUI implements a graphical TUI runtime as an alternative runtime to what
// the tui packages provides.
type GUI struct {
	ctx              context.Context
	cancelCtx        func()
	mu               sync.Locker
	fontManager      *font.Manager
	updateChan       chan term.Event
	handler          tui.Handler
	writer           *cell.BufferWriter
	mouse            *mouse
	input            *input
	opacity          float32
	enableLigatures  bool
	renderOffset     image.Point
	cursorAttributes term.Attributes
	defaultAttr      term.Attributes
	renderer         *renderer

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
	iteration     int64
	deviceScale   float64
	activeHinter  int
}

// New allocates storage for a new GUI and initializes it with the given
// tui.Handler and options.
func New(handler tui.Handler, options ...Option) (*GUI, error) {
	ret := &GUI{
		mu:               new(sync.Mutex),
		handler:          handler,
		updateChan:       make(chan term.Event, 50),
		fontManager:      font.NewManager(),
		activeHinter:     -1,
		enableLigatures:  true,
		cursorAttributes: term.Attributes{Bg: tcell.ColorRed},
	}
	ret.input = newInput(ret.fontManager)
	ret.mouse = newMouse(ret.fontManager)
	ret.ctx = context.Background()
	ret.ctx, ret.cancelCtx = context.WithCancel(ret.ctx)

	for _, option := range options {
		if err := option(ret); err != nil {
			return nil, fmt.Errorf("option: %w", err)
		}
	}

	// initialze renderer, writer, etc.
	ret.resize(defaultWidth, defaultHeight, ret.fontManager.DeviceScale())

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

	go g.consumeEvents()

	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(defaultWidth, defaultHeight)

	var gameOpts ebiten.RunGameOptions
	gameOpts.SingleThread = true
	return ebiten.RunGameWithOptions(g, &gameOpts)
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
}

// Update satisfies ebiten.Game. It's called every time a new frame is to be scheduled.
func (g *GUI) Update() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	mouseEv, mouseOk := g.mouse.processMouse()
	keyEv, keyOk, needsResize := g.input.processEvents()
	needsDraw := needsResize || g.needsDraw

	if needsResize {
		g.resize(g.width, g.height, g.fontManager.DeviceScale())
	}
	if mouseOk {
		g.pendingEvents = append(g.pendingEvents, mouseEv)
	}
	if keyOk {
		g.pendingEvents = append(g.pendingEvents, keyEv)
	}

	// set context with default iteration
	for _, ev := range g.pendingEvents {
		switch ev.Type {
		case term.EventInterrupt:
			var ctx context.Context
			if ev.Raw != nil {
				if id, ok := tui.IterationFromRawBytes(ev.Raw); ok {
					// override writer context with a specific iteration ID
					ctx = tui.ContextWithIteration(g.ctx, id)
				} else {
					// override writer context with a user-payload
					ctx = term.ContextWithPayload(g.ctx, ev.Raw)
				}
			} else {
				// override writer context with no iteration id; instead reset to nil
				// so clients can differentiate between an interrupt
				// and a regular iteration loop.
				ctx = g.ctx
			}
			g.drawHandler(ctx)
			needsDraw = false
			continue
		case term.EventError, term.EventResize:
			/* not dispatched by GUI */
		default:
			needsDraw = true
			exit, _ := g.handler.Handle(ev)
			if exit {
				return ErrHandlerExited
			}
		}
	}

	if needsDraw {
		g.drawHandler(tui.ContextWithIteration(g.ctx, g.iteration))
	}

	g.pendingEvents = g.pendingEvents[:0]
	g.iteration++
	return nil
}

// Layout satisfies ebiten.Game. It provides the terminal gui size in pixels.
func (g *GUI) Layout(width, height int) (int, int) {
	s := g.fontManager.DeviceScale()

	// resize handler only if effective size has changed
	if g.width != width || g.height != height || g.deviceScale != s {
		g.resize(width, height, s)
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
	g.handler.Resize(cellsWidth, cellsHeight)
	g.mouse.resize(cellsWidth, cellsHeight)
	g.writer = cell.NewBufferWriter(g.ctx, cellsWidth, cellsHeight)
	g.renderer = newRenderer(g.width, g.height, g.deviceScale,
		g.fontManager, g.opacity, g.enableLigatures,
		g.cursorAttributes, g.defaultAttr)
	g.needsDraw = true
}

func (g *GUI) consumeEvents() {
	for {
		ev, ok := <-g.updateChan
		if !ok {
			return
		}
		// dispatch UserFunc outside of
		// input processing to avoid input keys
		// taking precedence over callbacks, thus
		// yielding in a laggy experience for
		// functionality that depends on UserFunc.
		if ev.UserFunc != nil {
			ev.UserFunc()
			continue
		}
		g.mu.Lock()
		g.pendingEvents = append(g.pendingEvents, ev)
		g.mu.Unlock()
	}
}
